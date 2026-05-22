package main

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"go.uber.org/zap"
)

func TestParseAIReviewPayloadRequiresAnchors(t *testing.T) {
	_, err := parseAIReviewPayload([]byte(`{"submission_id":1,"revision_id":2,"prompt_config_id":3,"prompt_version":1,"idempotency_key":"k"}`))
	if err != nil {
		t.Fatalf("valid payload rejected: %v", err)
	}
	_, err = parseAIReviewPayload([]byte(`{"submission_id":1,"revision_id":2}`))
	if err == nil {
		t.Fatal("payload without prompt_config_id / idempotency_key must be rejected")
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
	mock.ExpectQuery(`(?is)^SELECT status, current_revision_id FROM submissions.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"status", "current_revision_id"}).AddRow("ai_reviewing", 901))
	mock.ExpectExec(`(?is)^UPDATE ai_reviews SET status = 'succeeded'`).
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
