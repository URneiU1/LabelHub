package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mysqlerr "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"

	"labelhub-api/internal/auth"
)

func TestGoldenSamplesListCreateDeleteOwnerAndAdmin(t *testing.T) {
	t.Run("owner lists current task samples", func(t *testing.T) {
		db, mock, sqlDB := newMockDB(t)
		defer sqlDB.Close()

		mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status"}).
				AddRow(1, 7, "Task", "draft"))
		mock.ExpectQuery(`(?is)^SELECT.+FROM .golden_samples.+ORDER BY id DESC`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "payload", "payload_hash", "expected_answer", "expected_verdict", "created_by"}).
				AddRow(11, 1, `{"text":"a"}`, "hash", `{"label":"ok"}`, "pass", 7))

		r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner1", Roles: []string{"owner"}})
		registerAllHandlers(r, db)

		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, jsonRequest(http.MethodGet, "/tasks/1/golden-samples", nil))

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
		}
		data := responseData(t, rec)
		samples := data["samples"].([]any)
		if samples[0].(map[string]any)["id"] != float64(11) {
			t.Fatalf("sample id = %v, want 11", samples[0].(map[string]any)["id"])
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("expectations not met: %v", err)
		}
	})

	t.Run("owner creates sample for current task", func(t *testing.T) {
		db, mock, sqlDB := newMockDB(t)
		defer sqlDB.Close()

		mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status"}).
				AddRow(1, 7, "Task", "draft"))
		mock.ExpectQuery(`(?is)^SELECT count\(\*\) FROM .ai_prompt_configs.`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
		mock.ExpectBegin()
		mock.ExpectExec(`(?is)^INSERT INTO .golden_samples.`).
			WithArgs(1, uint64(21), `{"text":"a"}`, expectedPayloadHash(t, map[string]any{"text": "a"}), `{"label":"ok"}`, "pass", "baseline", 7, sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(31, 1))
		mock.ExpectCommit()

		r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner1", Roles: []string{"owner"}})
		registerAllHandlers(r, db)

		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/golden-samples", map[string]any{
			"payload":          map[string]any{"text": "a"},
			"expected_answer":  map[string]any{"label": "ok"},
			"expected_verdict": "pass",
			"notes":            "baseline",
			"ai_prompt_id":     21,
		}))

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
		}
		data := responseData(t, rec)
		sample := data["sample"].(map[string]any)
		if sample["id"] != float64(31) {
			t.Fatalf("sample id = %v, want 31", sample["id"])
		}
		if sample["payloadHash"] == "" {
			t.Fatalf("payloadHash missing in response: %s", rec.Body.String())
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("expectations not met: %v", err)
		}
	})

	t.Run("admin deletes current task sample", func(t *testing.T) {
		db, mock, sqlDB := newMockDB(t)
		defer sqlDB.Close()

		mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status"}).
				AddRow(1, 7, "Task", "draft"))
		mock.ExpectBegin()
		mock.ExpectExec(`(?is)^DELETE FROM .golden_samples. WHERE id = .+ AND task_id = .+`).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		r := newGinWithClaims(&auth.Claims{UserID: 99, Username: "admin", Roles: []string{"admin"}})
		registerAllHandlers(r, db)

		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, jsonRequest(http.MethodDelete, "/tasks/1/golden-samples/31", nil))

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("expectations not met: %v", err)
		}
	})
}

func TestGoldenSamplesRejectNonOwner(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status"}).
			AddRow(1, 8, "Task", "draft"))

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner1", Roles: []string{"owner"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodGet, "/tasks/1/golden-samples", nil))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestGoldenSampleCreateRejectsPromptOutsideTask(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status"}).
			AddRow(1, 7, "Task", "draft"))
	mock.ExpectQuery(`(?is)^SELECT count\(\*\) FROM .ai_prompt_configs.`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner1", Roles: []string{"owner"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/golden-samples", map[string]any{
		"payload":          map[string]any{"text": "a"},
		"expected_answer":  map[string]any{"label": "ok"},
		"expected_verdict": "pass",
		"ai_prompt_id":     99,
	}))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestGoldenSampleCreateDuplicatePayloadReturns409(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status"}).
			AddRow(1, 7, "Task", "draft"))
	mock.ExpectBegin()
	mock.ExpectExec(`(?is)^INSERT INTO .golden_samples.`).
		WillReturnError(&mysqlerr.MySQLError{Number: 1062, Message: "Duplicate entry for key 'uk_task_payload_hash'"})
	mock.ExpectRollback()

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner1", Roles: []string{"owner"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/golden-samples", map[string]any{
		"payload":          map[string]any{"text": "a"},
		"expected_answer":  map[string]any{"label": "ok"},
		"expected_verdict": "pass",
	}))

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "CONFLICT") {
		t.Fatalf("expected CONFLICT body, got %s", rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestGoldenSampleDeleteCrossTaskReturns404(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status"}).
			AddRow(1, 7, "Task", "draft"))
	mock.ExpectBegin()
	mock.ExpectExec(`(?is)^DELETE FROM .golden_samples. WHERE id = .+ AND task_id = .+`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner1", Roles: []string{"owner"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodDelete, "/tasks/1/golden-samples/44", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestGoldenSampleDryRunRecordsMatchedExpected(t *testing.T) {
	for _, tc := range []struct {
		name          string
		expected      string
		wantMatched   bool
		expectedScore float64
	}{
		{name: "match", expected: "uncertain", wantMatched: true, expectedScore: 75},
		{name: "mismatch", expected: "pass", wantMatched: false, expectedScore: 75},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("LLM_PROVIDER", "mock")
			t.Setenv("LLM_ALLOWED_MODELS", "mock-model")
			db, mock, sqlDB := newMockDB(t)
			defer sqlDB.Close()

			mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
				WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status", "ai_prompt_id", "baseline_description"}).
					AddRow(1, 7, "Task", "draft", 33, "baseline"))
			mock.ExpectQuery(`(?is)^SELECT.+FROM .golden_samples.`).
				WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "ai_prompt_id", "payload", "payload_hash", "expected_answer", "expected_verdict", "created_by"}).
					AddRow(11, 1, 34, `{"text":"a"}`, "hash", `{"label":"ok"}`, tc.expected, 7))
			mock.ExpectQuery(`(?is)^SELECT.+FROM .ai_prompt_configs.`).
				WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "version", "prompt_template", "dimensions", "pass_threshold", "uncertain_min", "model", "created_by"}).
					AddRow(34, 1, 3, "review {{answer.label}}", `[{"name":"相关性"}]`, 80, 60, "mock-model", 7))
			mock.ExpectBegin()
			mock.ExpectExec(`(?is)^INSERT INTO .ai_dry_runs.`).
				WithArgs(1, uint64(34), uint64(11), 3, `{"text":"a"}`, `{"label":"ok"}`, tc.expected, "uncertain", tc.wantMatched, "succeeded", sqlmock.AnyArg(), nil, 7, sqlmock.AnyArg(), sqlmock.AnyArg()).
				WillReturnResult(sqlmock.NewResult(44, 1))
			mock.ExpectCommit()

			r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner1", Roles: []string{"owner"}})
			registerAllHandlers(r, db)

			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/golden-samples/11/dry-run", map[string]any{}))

			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
			}
			data := responseData(t, rec)
			if data["dryRunId"] != float64(44) {
				t.Fatalf("dryRunId = %v, want 44", data["dryRunId"])
			}
			if data["matchedExpected"] != tc.wantMatched {
				t.Fatalf("matchedExpected = %v, want %v", data["matchedExpected"], tc.wantMatched)
			}
			result := data["result"].(map[string]any)
			if result["overall_score"] != tc.expectedScore {
				t.Fatalf("overall_score = %v, want %v", result["overall_score"], tc.expectedScore)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("expectations not met: %v", err)
			}
		})
	}
}

func TestGoldenSampleDryRunRejectsCrossTaskSample(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status", "ai_prompt_id"}).
			AddRow(1, 7, "Task", "draft", 33))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .golden_samples.`).
		WillReturnError(gorm.ErrRecordNotFound)

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner1", Roles: []string{"owner"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/golden-samples/99/dry-run", map[string]any{}))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestGoldenSampleDryRunRejectsPromptOutsideTask(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status"}).
			AddRow(1, 7, "Task", "draft"))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .golden_samples.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "payload", "payload_hash", "expected_answer", "expected_verdict", "created_by"}).
			AddRow(11, 1, `{"text":"a"}`, "hash", `{"label":"ok"}`, "pass", 7))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .ai_prompt_configs.`).
		WillReturnError(gorm.ErrRecordNotFound)

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner1", Roles: []string{"owner"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/golden-samples/11/dry-run", map[string]any{
		"ai_prompt_id": 99,
	}))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestGoldenSampleDryRunRecordsSanitizedProviderFailure(t *testing.T) {
	secret := "sk-test-secret"
	t.Setenv("LLM_PROVIDER", "openai")
	t.Setenv("LLM_API_KEY", secret)
	t.Setenv("LLM_ALLOWED_MODELS", "mock-model")
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status", "ai_prompt_id"}).
			AddRow(1, 7, "Task", "draft", 33))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .golden_samples.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "payload", "payload_hash", "expected_answer", "expected_verdict", "created_by"}).
			AddRow(11, 1, `{"text":"a"}`, "hash", `{"label":"ok"}`, "pass", 7))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .ai_prompt_configs.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "version", "prompt_template", "dimensions", "pass_threshold", "uncertain_min", "model", "created_by"}).
			AddRow(33, 1, 3, "review {{answer.label}}", `[{"name":"相关性"}]`, 80, 60, "mock-model", 7))
	mock.ExpectBegin()
	mock.ExpectExec(`(?is)^INSERT INTO .ai_dry_runs.`).
		WithArgs(1, uint64(33), uint64(11), 3, `{"text":"a"}`, `{"label":"ok"}`, "pass", nil, nil, "failed", nil, "LLM_BASE_URL, LLM_API_KEY, and LLM_MODEL are required for real LLM providers", 7, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(44, 1))
	mock.ExpectCommit()

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner1", Roles: []string{"owner"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/golden-samples/11/dry-run", map[string]any{}))

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), secret) {
		t.Fatalf("provider secret leaked in response: %s", rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestGoldenSampleDryRunReturns500WhenFailureRecordCannotPersist(t *testing.T) {
	t.Setenv("LLM_PROVIDER", "openai")
	t.Setenv("LLM_API_KEY", "test-key")
	t.Setenv("LLM_ALLOWED_MODELS", "mock-model")
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status", "ai_prompt_id"}).
			AddRow(1, 7, "Task", "draft", 33))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .golden_samples.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "payload", "payload_hash", "expected_answer", "expected_verdict", "created_by"}).
			AddRow(11, 1, `{"text":"a"}`, "hash", `{"label":"ok"}`, "pass", 7))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .ai_prompt_configs.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "version", "prompt_template", "dimensions", "pass_threshold", "uncertain_min", "model", "created_by"}).
			AddRow(33, 1, 3, "review {{answer.label}}", `[{"name":"相关性"}]`, 80, 60, "mock-model", 7))
	mock.ExpectBegin()
	mock.ExpectExec(`(?is)^INSERT INTO .ai_dry_runs.`).
		WillReturnError(gorm.ErrInvalidDB)
	mock.ExpectRollback()

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner1", Roles: []string{"owner"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/golden-samples/11/dry-run", map[string]any{}))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func expectedPayloadHash(t *testing.T, payload any) string {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
