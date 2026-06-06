package main

import (
	"context"
	"errors"
	"testing"
	"time"

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
	mock.ExpectExec(`(?is)^UPDATE ai_reviews SET status = 'running'.+started_at = \?`).
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

func TestAIWorkerCircuitOpensAfterConsecutiveProvider5XX(t *testing.T) {
	now := time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC)
	circuit := newAIWorkerCircuit(2, time.Minute)
	circuit.now = func() time.Time { return now }

	circuit.recordProvider5XX()
	if err := circuit.check(); err != nil {
		t.Fatalf("circuit opened before threshold: %v", err)
	}
	circuit.recordProvider5XX()
	if err := circuit.check(); !errors.Is(err, errAIWorkerCircuitOpen) {
		t.Fatalf("circuit error = %v, want errAIWorkerCircuitOpen", err)
	}

	now = now.Add(time.Minute + time.Second)
	if err := circuit.check(); err != nil {
		t.Fatalf("circuit should close after cooldown: %v", err)
	}
}

func TestAIWorkerCircuitResetsConsecutive5XXAfterProviderSuccess(t *testing.T) {
	circuit := newAIWorkerCircuit(2, time.Minute)

	circuit.recordProvider5XX()
	circuit.recordProviderSuccess()
	circuit.recordProvider5XX()
	if err := circuit.check(); err != nil {
		t.Fatalf("single 5xx after success should not open circuit: %v", err)
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

// 可疑(uncertain)→ 转人工复核(manual_review),走专属初审入口分支,不再合流进 human_reviewing。
func TestCompleteMovesUncertainSubmissionToManualReview(t *testing.T) {
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
	mock.ExpectExec(`(?is)^UPDATE submissions SET status = \?, ai_verdict = \?, ai_score = \?`).
		WithArgs("manual_review", "uncertain", 75.0, payload.SubmissionID, payload.RevisionID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^INSERT INTO audit_logs`).
		WithArgs(payload.SubmissionID, "manual_review", "ai_uncertain", sqlmock.AnyArg(), sqlmock.AnyArg()).
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

// 明确不合格(reject)→ 直接打回标注员(revising),不进人审、不终结 item。
func TestCompleteRejectBouncesSubmissionToRevising(t *testing.T) {
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
		Verdict:     "reject",
		Score:       30,
		Reason:      "clearly fails",
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
	mock.ExpectExec(`(?is)^UPDATE submissions SET status = \?, ai_verdict = \?, ai_score = \?`).
		WithArgs("revising", "reject", 30.0, payload.SubmissionID, payload.RevisionID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^INSERT INTO audit_logs`).
		WithArgs(payload.SubmissionID, "revising", "ai_reject", sqlmock.AnyArg(), sqlmock.AnyArg()).
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

func TestHandleAIReviewVerdictStateMapping(t *testing.T) {
	t.Setenv("LLM_ALLOWED_MODELS", "test-model")
	// 对齐审核流程图三条独立分支:
	//   pass → 初审(human_reviewing);uncertain → 转人工复核(manual_review);reject → 直接打回标注员(revising)。
	// AI 不再自动入库、不抽检直通。
	tests := []struct {
		name        string
		verdict     string
		score       float64
		wantToState string
		wantEvent   string
	}{
		{name: "pass goes to first human review", verdict: "pass", score: 92, wantToState: "human_reviewing", wantEvent: "ai_done"},
		{name: "uncertain goes to manual review", verdict: "uncertain", score: 65, wantToState: "manual_review", wantEvent: "ai_uncertain"},
		{name: "reject bounces back to labeler", verdict: "reject", score: 35, wantToState: "revising", wantEvent: "ai_reject"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
			if err != nil {
				t.Fatalf("sqlmock.New: %v", err)
			}
			defer db.Close()

			payload := aiReviewPayload{SubmissionID: 42, RevisionID: 901, PromptConfigID: 7, PromptVersion: 2}
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
			mock.ExpectExec(`(?is)^UPDATE ai_reviews SET status = 'succeeded'.+error_msg = NULL`).
				WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec(`(?is)^UPDATE submissions SET status = \?, ai_verdict = \?, ai_score = \? WHERE id = \? AND status = 'ai_reviewing' AND current_revision_id = \?`).
				WithArgs(tt.wantToState, tt.verdict, tt.score, payload.SubmissionID, payload.RevisionID).
				WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec(`(?is)^INSERT INTO audit_logs`).
				WithArgs(payload.SubmissionID, tt.wantToState, tt.wantEvent, sqlmock.AnyArg(), sqlmock.AnyArg()).
				WillReturnResult(sqlmock.NewResult(1, 1))
			mock.ExpectCommit()

			handler := workerHandlers{db: db, logger: zap.NewNop(), evaluator: verdictEvaluator{verdict: tt.verdict, score: tt.score}}
			if err := handler.handleAIReview(context.Background(), newAsynqTask(rawPayload)); err != nil {
				t.Fatalf("handleAIReview returned error: %v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("expectations not met: %v", err)
			}
		})
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
	mock.ExpectExec(`(?is)^UPDATE submissions SET status = \?, ai_verdict = \?, ai_score = \?`).
		WithArgs("human_reviewing", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
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

func TestHandleAIReviewReturnsCompleteDBErrorWithoutFailover(t *testing.T) {
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
	mock.ExpectExec(`(?is)^UPDATE ai_reviews SET status = 'succeeded'`).
		WillReturnError(errors.New("db write failed"))
	mock.ExpectRollback()

	handler := workerHandlers{db: db, logger: zap.NewNop(), evaluator: staticEvaluator{}}
	if err := handler.handleAIReview(context.Background(), newAsynqTask(rawPayload)); err == nil {
		t.Fatal("handleAIReview should return complete DB error for retry")
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
	mock.ExpectExec(`(?is)^UPDATE submissions SET status = \?, ai_verdict = \?, ai_score = \?`).
		WithArgs("human_reviewing", "pass", 88.0, payload.SubmissionID, payload.RevisionID).
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

func TestHandleAIReviewCircuitOpenFailsOverImmediately(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	circuit := newAIWorkerCircuit(1, time.Minute)
	circuit.recordProvider5XX()

	rawPayload := validAIReviewPayloadJSON()
	mock.ExpectExec(`(?is)^UPDATE ai_reviews SET status = 'running'`).
		WillReturnResult(sqlmock.NewResult(0, 1))
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

	handler := workerHandlers{db: db, logger: zap.NewNop(), evaluator: failingEvaluator{}, circuit: circuit}
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

func TestParseAIDryRunPayloadRequiresRunID(t *testing.T) {
	payload, err := parseAIDryRunPayload([]byte(`{"run_id":44}`))
	if err != nil {
		t.Fatalf("valid payload rejected: %v", err)
	}
	if payload.RunID != 44 {
		t.Fatalf("runID = %d, want 44", payload.RunID)
	}
	if _, err := parseAIDryRunPayload([]byte(`{"run_id":0}`)); err == nil {
		t.Fatal("payload without positive run_id must be rejected")
	}
}

func TestHandleAIDryRunCompletesQueuedRun(t *testing.T) {
	t.Setenv("LLM_PROVIDER", "unsupported-provider")
	t.Setenv("LLM_ALLOWED_MODELS", "test-model")
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectExec(`(?is)^UPDATE ai_dry_runs SET status = 'running'`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?is)^SELECT dr.task_id, dr.ai_prompt_id, dr.prompt_version, COALESCE\(dr.payload_snapshot, ''\), COALESCE\(dr.expected_answer_snapshot, ''\), COALESCE\(dr.expected_verdict, ''\), COALESCE\(t.baseline_description, ''\), cfg.prompt_template, cfg.dimensions, cfg.pass_threshold, cfg.uncertain_min, cfg.model`).
		WillReturnRows(sqlmock.NewRows([]string{
			"task_id", "ai_prompt_id", "prompt_version", "payload_snapshot", "expected_answer_snapshot", "expected_verdict",
			"baseline_description", "prompt_template", "dimensions", "pass_threshold", "uncertain_min", "model",
		}).AddRow(
			1, 33, 3, `{"text":"a"}`, `{"label":"ok"}`, "uncertain",
			"baseline", "review {{answer.label}}", `[{"name":"相关性"}]`, 80, 60, "test-model",
		))
	mock.ExpectExec(`(?is)^UPDATE ai_dry_runs SET status = 'succeeded'.+actual_verdict = \?.+matched_expected = \?.+error_msg = NULL`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	handler := workerHandlers{db: db, logger: zap.NewNop(), evaluator: deterministicEvaluator{}, circuit: newAIWorkerCircuit(0, time.Minute)}
	if err := handler.handleAIDryRun(context.Background(), newAIDryRunTask([]byte(`{"run_id":44}`))); err != nil {
		t.Fatalf("handleAIDryRun returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestHandleAIDryRunFinalizedDuplicateDoesNotCallProvider(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectExec(`(?is)^UPDATE ai_dry_runs SET status = 'running'`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	handler := workerHandlers{db: db, logger: zap.NewNop(), evaluator: deterministicEvaluator{}, circuit: newAIWorkerCircuit(0, time.Minute)}
	if err := handler.handleAIDryRun(context.Background(), newAIDryRunTask([]byte(`{"run_id":44}`))); err != nil {
		t.Fatalf("handleAIDryRun returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestHandleAIDryRunCircuitOpenMarksRunFailed(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	circuit := newAIWorkerCircuit(1, time.Minute)
	circuit.recordProvider5XX()

	mock.ExpectExec(`(?is)^UPDATE ai_dry_runs SET status = 'running'`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^UPDATE ai_dry_runs SET status = 'failed'`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	handler := workerHandlers{db: db, logger: zap.NewNop(), evaluator: deterministicEvaluator{}, circuit: circuit}
	if err := handler.handleAIDryRun(context.Background(), newAIDryRunTask([]byte(`{"run_id":44}`))); err != nil {
		t.Fatalf("handleAIDryRun returned error: %v", err)
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

type verdictEvaluator struct {
	verdict string
	score   float64
}

func (e verdictEvaluator) Evaluate(ctx context.Context, payload aiReviewPayload, input aiReviewInput) (aiEvaluation, error) {
	return aiEvaluation{
		Verdict:      e.verdict,
		Score:        e.score,
		Reason:       "mapped verdict",
		Dimensions:   `[{"name":"相关性","score":88,"reason":"ok"}]`,
		RawResponse:  `{"provider":"test"}`,
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

func newAIDryRunTask(payload []byte) *asynq.Task {
	return asynq.NewTask("ai:dry-run", payload)
}

func validAIReviewPayloadJSON() []byte {
	return []byte(`{"submission_id":42,"revision_id":901,"prompt_config_id":7,"prompt_version":2,"idempotency_key":"` + aiReviewIdempotencyKey(42, 901, 7, 2) + `"}`)
}
