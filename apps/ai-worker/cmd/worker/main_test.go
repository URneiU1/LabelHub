package main

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/hibiken/asynq"
	"go.uber.org/zap"
	"labelhub.local/llmreview"
)

func TestParseAIReviewPayloadRequiresAnchors(t *testing.T) {
	validKey := aiReviewIdempotencyKey(1, 2, 3, 1)
	_, err := parseAIReviewPayload([]byte(`{"submission_id":1,"revision_id":2,"prompt_config_id":3,"prompt_version":1,"idempotency_key":"` + validKey + `"}`))
	if err != nil {
		t.Fatalf("valid payload rejected: %v", err)
	}
	_, err = parseAIReviewPayload([]byte(`{"submission_id":1,"revision_id":2}`))
	if err == nil {
		t.Fatal("payload without prompt_config_id / idempotency_key must be rejected")
	}
	_, err = parseAIReviewPayload([]byte(`{"submission_id":1,"revision_id":2,"prompt_config_id":3,"prompt_version":1,"idempotency_key":"mismatch"}`))
	if err == nil {
		t.Fatal("mismatched idempotency key must be rejected")
	}
}

func TestHandleAIReviewRejectsMismatchedIdempotencyWithoutDBClaim(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	handler := workerHandlers{db: db, logger: zap.NewNop(), evaluator: failingEvaluator{}}
	rawPayload := []byte(`{"submission_id":42,"revision_id":901,"prompt_config_id":7,"prompt_version":2,"idempotency_key":"mismatch"}`)
	if err := handler.handleAIReview(context.Background(), newAsynqTask(rawPayload)); err != nil {
		t.Fatalf("handleAIReview returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unexpected DB call for invalid payload: %v", err)
	}
}

func TestFailoverMovesAIReviewingSubmissionToHumanReview(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	payload := aiReviewPayload{
		SubmissionID:   42,
		RevisionID:     901,
		PromptConfigID: 7,
		PromptVersion:  2,
		IdempotencyKey: "idem",
	}
	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT status FROM ai_reviews.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("running"))
	mock.ExpectQuery(`(?is)^SELECT status, current_revision_id FROM submissions.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"status", "current_revision_id"}).AddRow("ai_reviewing", 901))
	mock.ExpectExec(`(?is)^UPDATE ai_reviews SET status = 'dead'`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^UPDATE submissions SET status = 'human_reviewing'`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^INSERT INTO audit_logs`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	handler := workerHandlers{db: db, logger: zap.NewNop(), evaluator: deterministicEvaluator{}}
	if err := handler.failover(context.Background(), payload, errors.New("schema parse failed")); err != nil {
		t.Fatalf("failover returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestMarkRunningReturnsFalseForFinalizedDuplicateTask(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	payload := aiReviewPayload{
		SubmissionID:   42,
		RevisionID:     901,
		PromptConfigID: 7,
		PromptVersion:  2,
		IdempotencyKey: "idem",
	}
	mock.ExpectExec(`(?is)^UPDATE ai_reviews SET status = 'running'`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	handler := workerHandlers{db: db, logger: zap.NewNop(), evaluator: deterministicEvaluator{}}
	claimed, err := handler.markRunning(context.Background(), payload)
	if err != nil {
		t.Fatalf("markRunning returned error: %v", err)
	}
	if claimed {
		t.Fatal("duplicate finalized task must not be claimed")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestMarkFailedMovesRunningReviewBackToFailedForRetry(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	payload := aiReviewPayload{
		SubmissionID:   42,
		RevisionID:     901,
		PromptConfigID: 7,
		PromptVersion:  2,
		IdempotencyKey: "idem",
	}
	mock.ExpectExec(`(?is)^UPDATE ai_reviews SET status = 'failed'.+submission_id = \?.+revision_id = \?.+prompt_version = \?.+status IN \('pending','running','failed'\)`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	handler := workerHandlers{db: db, logger: zap.NewNop(), evaluator: deterministicEvaluator{}}
	if err := handler.markFailed(context.Background(), payload, errors.New("provider failed")); err != nil {
		t.Fatalf("markFailed returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestFailoverDoesNotMarkSucceededReviewDead(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	payload := aiReviewPayload{
		SubmissionID:   42,
		RevisionID:     901,
		PromptConfigID: 7,
		PromptVersion:  2,
		IdempotencyKey: "idem",
	}
	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT status FROM ai_reviews.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("succeeded"))
	mock.ExpectCommit()

	handler := workerHandlers{db: db, logger: zap.NewNop(), evaluator: deterministicEvaluator{}}
	if err := handler.failover(context.Background(), payload, errors.New("late replay failure")); err != nil {
		t.Fatalf("failover returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestCompleteMovesSubmissionToHumanReviewWithAIVerdict(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	payload := aiReviewPayload{
		SubmissionID:   42,
		RevisionID:     901,
		PromptConfigID: 7,
		PromptVersion:  2,
		IdempotencyKey: "idem",
	}
	result := aiEvaluation{
		Verdict:     "uncertain",
		Score:       75,
		Reason:      "needs human",
		Dimensions:  `[]`,
		RawResponse: `{}`,
		LatencyMS:   1,
	}
	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT status FROM ai_reviews.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("running"))
	mock.ExpectQuery(`(?is)^SELECT status, current_revision_id FROM submissions.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"status", "current_revision_id"}).AddRow("ai_reviewing", 901))
	mock.ExpectExec(`(?is)^UPDATE ai_reviews SET status = 'succeeded'.+error_msg = NULL`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^UPDATE submissions SET status = 'human_reviewing'`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^INSERT INTO audit_logs`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	handler := workerHandlers{db: db, logger: zap.NewNop(), evaluator: deterministicEvaluator{}}
	if err := handler.complete(context.Background(), payload, result); err != nil {
		t.Fatalf("complete returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestHandleAIReviewUsesProviderResultAndRecordsUsage(t *testing.T) {
	t.Setenv("LLM_ALLOWED_MODELS", "test-model")
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	rawPayload := validAIReviewPayloadJSON()
	mock.ExpectExec(`(?is)^UPDATE ai_reviews SET status = 'running'`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?is)^SELECT sr.answer, ti.payload, COALESCE\(t.baseline_description,''\), cfg.prompt_template, cfg.dimensions, cfg.pass_threshold, cfg.uncertain_min, cfg.model`).
		WillReturnRows(sqlmock.NewRows([]string{"answer", "payload", "baseline_description", "prompt_template", "dimensions", "pass_threshold", "uncertain_min", "model"}).
			AddRow(`{"summary":"ok"}`, `{"prompt":"question"}`, "baseline", "review {{answer.summary}}", `[{"name":"相关性"}]`, 80, 60, "test-model"))
	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT status FROM ai_reviews.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("running"))
	mock.ExpectQuery(`(?is)^SELECT status, current_revision_id FROM submissions.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"status", "current_revision_id"}).AddRow("ai_reviewing", 901))
	mock.ExpectExec(`(?is)^UPDATE ai_reviews SET status = 'succeeded'.+tokens_input = \?, tokens_output = \?.+error_msg = NULL`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^UPDATE submissions SET status = 'human_reviewing'`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^INSERT INTO audit_logs`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	handler := workerHandlers{db: db, logger: zap.NewNop(), evaluator: staticEvaluator{}}
	if err := handler.handleAIReview(context.Background(), newAsynqTask(rawPayload)); err != nil {
		t.Fatalf("handleAIReview returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestCompleteClearsPreviousErrorMessageOnRetrySuccess(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	payload := aiReviewPayload{
		SubmissionID:   42,
		RevisionID:     901,
		PromptConfigID: 7,
		PromptVersion:  2,
		IdempotencyKey: "idem",
	}
	result := aiEvaluation{
		Verdict:      "pass",
		Score:        88,
		Reason:       "retry succeeded",
		Dimensions:   `[{"name":"相关性","score":88,"reason":"ok"}]`,
		RawResponse:  `{"provider":"mock"}`,
		TokensInput:  12,
		TokensOutput: 8,
		LatencyMS:    3,
	}
	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT status FROM ai_reviews.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("failed"))
	mock.ExpectQuery(`(?is)^SELECT status, current_revision_id FROM submissions.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"status", "current_revision_id"}).AddRow("ai_reviewing", 901))
	mock.ExpectExec(`(?is)^UPDATE ai_reviews SET status = 'succeeded'.+error_msg = NULL`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^UPDATE submissions SET status = 'human_reviewing'`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^INSERT INTO audit_logs`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	handler := workerHandlers{db: db, logger: zap.NewNop(), evaluator: deterministicEvaluator{}}
	if err := handler.complete(context.Background(), payload, result); err != nil {
		t.Fatalf("complete returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestHandleAIReviewFinalizedDuplicateDoesNotCallProvider(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectExec(`(?is)^UPDATE ai_reviews SET status = 'running'`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	handler := workerHandlers{db: db, logger: zap.NewNop(), evaluator: failingEvaluator{}}
	rawPayload := validAIReviewPayloadJSON()
	if err := handler.handleAIReview(context.Background(), newAsynqTask(rawPayload)); err != nil {
		t.Fatalf("handleAIReview returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestHandleAIReviewProviderFailureFailsOverOnFinalAttempt(t *testing.T) {
	t.Setenv("LLM_ALLOWED_MODELS", "test-model")
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	rawPayload := validAIReviewPayloadJSON()
	mock.ExpectExec(`(?is)^UPDATE ai_reviews SET status = 'running'`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?is)^SELECT sr.answer, ti.payload, COALESCE\(t.baseline_description,''\), cfg.prompt_template, cfg.dimensions, cfg.pass_threshold, cfg.uncertain_min, cfg.model`).
		WillReturnRows(sqlmock.NewRows([]string{"answer", "payload", "baseline_description", "prompt_template", "dimensions", "pass_threshold", "uncertain_min", "model"}).
			AddRow(`{"summary":"ok"}`, `{"prompt":"question"}`, "baseline", "review {{answer.summary}}", `[{"name":"相关性"}]`, 80, 60, "test-model"))
	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT status FROM ai_reviews.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("running"))
	mock.ExpectQuery(`(?is)^SELECT status, current_revision_id FROM submissions.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"status", "current_revision_id"}).AddRow("ai_reviewing", 901))
	mock.ExpectExec(`(?is)^UPDATE ai_reviews SET status = 'dead'`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^UPDATE submissions SET status = 'human_reviewing'`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^INSERT INTO audit_logs`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	handler := workerHandlers{db: db, logger: zap.NewNop(), evaluator: failingEvaluator{}}
	if err := handler.handleAIReview(context.Background(), newAsynqTask(rawPayload)); err != nil {
		t.Fatalf("handleAIReview returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestHandleAIReviewNonRetryableEvaluationErrorFailsOverImmediately(t *testing.T) {
	t.Setenv("LLM_ALLOWED_MODELS", "test-model")
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	rawPayload := validAIReviewPayloadJSON()
	mock.ExpectExec(`(?is)^UPDATE ai_reviews SET status = 'running'`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?is)^SELECT sr.answer, ti.payload, COALESCE\(t.baseline_description,''\), cfg.prompt_template, cfg.dimensions, cfg.pass_threshold, cfg.uncertain_min, cfg.model`).
		WillReturnRows(sqlmock.NewRows([]string{"answer", "payload", "baseline_description", "prompt_template", "dimensions", "pass_threshold", "uncertain_min", "model"}).
			AddRow(`{"summary":"ok"}`, `{"prompt":"question"}`, "baseline", "review {{answer.summary}}", `[{"name":"相关性"}]`, 80, 60, "test-model"))
	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT status FROM ai_reviews.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("running"))
	mock.ExpectQuery(`(?is)^SELECT status, current_revision_id FROM submissions.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"status", "current_revision_id"}).AddRow("ai_reviewing", 901))
	mock.ExpectExec(`(?is)^UPDATE ai_reviews SET status = 'dead'`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^UPDATE submissions SET status = 'human_reviewing'`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^INSERT INTO audit_logs`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	handler := workerHandlers{db: db, logger: zap.NewNop(), evaluator: nonRetryableEvaluator{}}
	if err := handler.handleAIReview(context.Background(), newAsynqTask(rawPayload)); err != nil {
		t.Fatalf("handleAIReview returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

type staticEvaluator struct{}

func (staticEvaluator) Evaluate(ctx context.Context, payload aiReviewPayload, input aiReviewInput) (aiEvaluation, error) {
	return aiEvaluation{
		Verdict:      "pass",
		Score:        88,
		Reason:       "structured result",
		Dimensions:   `[{"name":"相关性","score":88,"reason":"ok"}]`,
		RawResponse:  `{"provider":"mock"}`,
		TokensInput:  12,
		TokensOutput: 8,
		LatencyMS:    3,
	}, nil
}

type failingEvaluator struct{}

func (failingEvaluator) Evaluate(ctx context.Context, payload aiReviewPayload, input aiReviewInput) (aiEvaluation, error) {
	return aiEvaluation{}, errors.New("provider should not be called")
}

type nonRetryableEvaluator struct{}

func (nonRetryableEvaluator) Evaluate(ctx context.Context, payload aiReviewPayload, input aiReviewInput) (aiEvaluation, error) {
	return aiEvaluation{}, llmreview.ValidateThresholdConsistency(llmreview.EvaluationResult{
		Verdict:      "pass",
		OverallScore: 10,
	}, input.Prompt)
}

func newAsynqTask(payload []byte) *asynq.Task {
	return asynq.NewTask("ai:review", payload)
}

func validAIReviewPayloadJSON() []byte {
	return []byte(`{"submission_id":42,"revision_id":901,"prompt_config_id":7,"prompt_version":2,"idempotency_key":"` + aiReviewIdempotencyKey(42, 901, 7, 2) + `"}`)
}
