package exporter

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestLoadApprovedRows_NoReviews(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectQuery(`(?is)^\s*SELECT s.id.+FROM submissions`).WithArgs(uint64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"sid", "iid", "external_id", "payload", "answer"}).
			AddRow(1, 11, "Q1", `{"x":1}`, `{"label":"cat"}`))

	rows, err := LoadApprovedRows(context.Background(), db, 5, false)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	if pick(rows[0], "external_id") != "Q1" {
		t.Fatalf("external_id = %v", pick(rows[0], "external_id"))
	}
	// 无 review 列
	if pick(rows[0], "ai_review.verdict") != nil {
		t.Fatal("should not have review columns when includeReviews=false")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestLoadApprovedRows_WithReviews(t *testing.T) {
	db, mock := newMock(t)
	now := time.Date(2026, 5, 27, 9, 0, 0, 0, time.UTC)
	mock.ExpectQuery(`(?is)^\s*SELECT s.id.+FROM submissions`).WithArgs(uint64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"sid", "iid", "external_id", "payload", "answer"}).
			AddRow(1, 11, "Q1", `{"x":1}`, `{"label":"cat"}`).
			AddRow(2, 12, nil, `{}`, `{}`))
	mock.ExpectQuery(`(?is)^SELECT submission_id, verdict, overall_score, dimensions, reason, prompt_version, created_at\s+FROM ai_reviews WHERE submission_id IN`).
		WillReturnRows(sqlmock.NewRows([]string{"submission_id", "verdict", "overall_score", "dimensions", "reason", "prompt_version", "created_at"}).
			AddRow(1, "pass", 8.5, `[{"name":"相关性","score":9}]`, "looks good", 3, now))
	mock.ExpectQuery(`(?is)^SELECT submission_id, verdict, reason, stage, reviewer_id, created_at\s+FROM human_reviews WHERE submission_id IN`).
		WillReturnRows(sqlmock.NewRows([]string{"submission_id", "verdict", "reason", "stage", "reviewer_id", "created_at"}).
			AddRow(1, "approve", "ok", "first", 2, now))

	rows, err := LoadApprovedRows(context.Background(), db, 5, true)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("want 2 rows, got %d", len(rows))
	}
	// sub 1 有 ai/human review
	if pick(rows[0], "ai_review.verdict") != "pass" {
		t.Fatalf("ai_review.verdict = %v", pick(rows[0], "ai_review.verdict"))
	}
	if pick(rows[0], "human_review.verdict") != "approve" {
		t.Fatalf("human_review.verdict = %v", pick(rows[0], "human_review.verdict"))
	}
	// sub 2 无 review → 列在但值为 nil(保列稳定)
	if pick(rows[1], "ai_review.verdict") != nil {
		t.Fatalf("sub2 ai_review.verdict should be nil, got %v", pick(rows[1], "ai_review.verdict"))
	}
	hasKey := false
	for _, c := range rows[1] {
		if c.Key == "ai_review.verdict" {
			hasKey = true
		}
	}
	if !hasKey {
		t.Fatal("review columns must exist on all rows for column stability")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
