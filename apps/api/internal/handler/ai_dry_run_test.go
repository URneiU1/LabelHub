package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"labelhub-api/internal/auth"
)

func TestAIDryRunsListOwnerTaskHistory(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	matched := true

	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status"}).
			AddRow(1, 7, "Task", "draft"))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .ai_dry_runs.+task_id.+golden_sample_id.+ORDER BY created_at DESC, id DESC.+LIMIT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "task_id", "ai_prompt_id", "golden_sample_id", "prompt_version",
			"payload_snapshot", "expected_answer_snapshot", "expected_verdict", "actual_verdict", "matched_expected",
			"status", "result", "error_msg", "created_by", "created_at", "finished_at",
		}).AddRow(
			44, 1, 33, 11, 3,
			`{"text":"a"}`, `{"label":"ok"}`, "pass", "pass", matched,
			"succeeded", `{"verdict":"pass","overall_score":95}`, nil, 7, now, now,
		))

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner1", Roles: []string{"owner"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodGet, "/tasks/1/ai-dry-runs?golden_sample_id=11&limit=5", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	data := responseData(t, rec)
	runs := data["dryRuns"].([]any)
	if len(runs) != 1 {
		t.Fatalf("dryRuns len = %d, want 1", len(runs))
	}
	run := runs[0].(map[string]any)
	if run["id"] != float64(44) || run["goldenSampleId"] != float64(11) {
		t.Fatalf("unexpected run ids: %v", run)
	}
	if _, ok := run["payloadSnapshot"].(map[string]any); !ok {
		t.Fatalf("payloadSnapshot is %T, want JSON object: %s", run["payloadSnapshot"], rec.Body.String())
	}
	if _, ok := run["result"].(map[string]any); !ok {
		t.Fatalf("result is %T, want JSON object: %s", run["result"], rec.Body.String())
	}
	if run["matchedExpected"] != true {
		t.Fatalf("matchedExpected = %v, want true", run["matchedExpected"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestAIDryRunsListRejectsNonOwner(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status"}).
			AddRow(1, 99, "Task", "draft"))

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner1", Roles: []string{"owner"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodGet, "/tasks/1/ai-dry-runs", nil))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestAIDryRunsListRejectsInvalidGoldenSampleFilter(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status"}).
			AddRow(1, 7, "Task", "draft"))

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner1", Roles: []string{"owner"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodGet, "/tasks/1/ai-dry-runs?golden_sample_id=bad", nil))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}
