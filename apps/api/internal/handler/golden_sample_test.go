package handler

import (
	"bytes"
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
		sample := samples[0].(map[string]any)
		if sample["id"] != float64(11) {
			t.Fatalf("sample id = %v, want 11", sample["id"])
		}
		if _, ok := sample["payload"].(map[string]any); !ok {
			t.Fatalf("payload is %T, want JSON object: %s", sample["payload"], rec.Body.String())
		}
		if _, ok := sample["expectedAnswer"].(map[string]any); !ok {
			t.Fatalf("expectedAnswer is %T, want JSON object: %s", sample["expectedAnswer"], rec.Body.String())
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
		if _, ok := sample["payload"].(map[string]any); !ok {
			t.Fatalf("payload is %T, want JSON object: %s", sample["payload"], rec.Body.String())
		}
		if _, ok := sample["expectedAnswer"].(map[string]any); !ok {
			t.Fatalf("expectedAnswer is %T, want JSON object: %s", sample["expectedAnswer"], rec.Body.String())
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

func TestGoldenSampleCreatePreservesRawJSONNumbersAndCanonicalHash(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	payload := `{"nested":{"b":2.0,"a":1e0},"external_id":9007199254740993123}`
	expectedAnswer := `{"label":"ok","confidence":1}`
	canonicalPayload := `{"external_id":9007199254740993123,"nested":{"a":1,"b":2}}`
	canonicalAnswer := `{"confidence":1,"label":"ok"}`

	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status"}).
			AddRow(1, 7, "Task", "draft"))
	mock.ExpectBegin()
	mock.ExpectExec(`(?is)^INSERT INTO .golden_samples.`).
		WithArgs(1, nil, canonicalPayload, expectedPayloadHashRaw(t, canonicalPayload), canonicalAnswer, "pass", nil, 7, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(41, 1))
	mock.ExpectCommit()

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner1", Roles: []string{"owner"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, rawJSONRequest(http.MethodPost, "/tasks/1/golden-samples", `{
		"payload": `+payload+`,
		"expected_answer": `+expectedAnswer+`,
		"expected_verdict": "pass"
	}`))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			Sample struct {
				Payload        json.RawMessage `json:"payload"`
				ExpectedAnswer json.RawMessage `json:"expectedAnswer"`
			} `json:"sample"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if string(resp.Data.Sample.Payload) != canonicalPayload {
		t.Fatalf("payload = %s, want %s", resp.Data.Sample.Payload, canonicalPayload)
	}
	if !bytes.Contains(resp.Data.Sample.Payload, []byte("9007199254740993123")) {
		t.Fatalf("large integer lost precision: %s", resp.Data.Sample.Payload)
	}
	if string(resp.Data.Sample.ExpectedAnswer) != canonicalAnswer {
		t.Fatalf("expectedAnswer = %s, want %s", resp.Data.Sample.ExpectedAnswer, canonicalAnswer)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
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

func TestGoldenSampleDryRunQueuesAsyncRun(t *testing.T) {
	for _, tc := range []struct {
		name     string
		expected string
		promptID uint64
	}{
		{name: "sample prompt", expected: "uncertain", promptID: 34},
		{name: "active prompt fallback", expected: "pass", promptID: 33},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("LLM_ALLOWED_MODELS", "mock-model")
			db, mock, sqlDB := newMockDB(t)
			defer sqlDB.Close()

			mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
				WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status", "ai_prompt_id", "baseline_description"}).
					AddRow(1, 7, "Task", "draft", 33, "baseline"))
			mock.ExpectQuery(`(?is)^SELECT.+FROM .golden_samples.`).
				WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "ai_prompt_id", "payload", "payload_hash", "expected_answer", "expected_verdict", "created_by"}).
					AddRow(11, 1, nullableUint64(tc.promptID, tc.name == "sample prompt"), `{"text":"a"}`, "hash", `{"label":"ok"}`, tc.expected, 7))
			mock.ExpectQuery(`(?is)^SELECT.+FROM .ai_prompt_configs.`).
				WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "version", "prompt_template", "dimensions", "pass_threshold", "uncertain_min", "model", "created_by"}).
					AddRow(tc.promptID, 1, 3, "review {{answer.label}}", `[{"name":"相关性"}]`, 80, 60, "mock-model", 7))
			mock.ExpectBegin()
			mock.ExpectExec(`(?is)^INSERT INTO .ai_dry_runs.`).
				WithArgs(1, tc.promptID, uint64(11), 3, `{"text":"a"}`, `{"label":"ok"}`, tc.expected, nil, nil, "queued", nil, nil, 7, sqlmock.AnyArg(), nil).
				WillReturnResult(sqlmock.NewResult(44, 1))
			mock.ExpectExec(`(?is)^INSERT INTO .outbox_events.`).
				WillReturnResult(sqlmock.NewResult(81, 1))
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
			if data["status"] != "queued" {
				t.Fatalf("status = %v, want queued", data["status"])
			}
			if _, exists := data["result"]; exists {
				t.Fatalf("async queue response must not include provider result: %v", data)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("expectations not met: %v", err)
			}
		})
	}
}

func nullableUint64(value uint64, valid bool) any {
	if !valid {
		return nil
	}
	return value
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
		WithArgs(1, uint64(33), uint64(11), 3, `{"text":"a"}`, `{"label":"ok"}`, "pass", nil, nil, "queued", nil, nil, 7, sqlmock.AnyArg(), nil).
		WillReturnResult(sqlmock.NewResult(44, 1))
	mock.ExpectExec(`(?is)^INSERT INTO .outbox_events.`).
		WillReturnResult(sqlmock.NewResult(81, 1))
	mock.ExpectCommit()

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner1", Roles: []string{"owner"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/golden-samples/11/dry-run", map[string]any{}))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), secret) {
		t.Fatalf("provider secret leaked in response: %s", rec.Body.String())
	}
	data := responseData(t, rec)
	if data["dryRunId"] != float64(44) || data["status"] != "queued" {
		t.Fatalf("unexpected async dry-run response: %v", data)
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

func TestGoldenSampleBatchDryRunQueuesPerSample(t *testing.T) {
	t.Setenv("LLM_ALLOWED_MODELS", "mock-model")
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status", "ai_prompt_id", "baseline_description"}).
			AddRow(1, 7, "Task", "draft", nil, "baseline"))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .golden_samples.+task_id.+id IN`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "ai_prompt_id", "payload", "payload_hash", "expected_answer", "expected_verdict", "created_by"}).
			AddRow(11, 1, 34, `{"text":"a"}`, "hash-a", `{"label":"ok"}`, "uncertain", 7).
			AddRow(12, 1, nil, `{"text":"b"}`, "hash-b", `{"label":"ok"}`, "pass", 7))
	// 样本 11 可解析到 prompt 34 → 入队(同事务 INSERT ai_dry_runs(queued) + outbox_events)。
	mock.ExpectQuery(`(?is)^SELECT.+FROM .ai_prompt_configs.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "version", "prompt_template", "dimensions", "pass_threshold", "uncertain_min", "model", "created_by"}).
			AddRow(34, 1, 3, "review {{answer.label}}", `[{"name":"相关性"}]`, 80, 60, "mock-model", 7))
	mock.ExpectBegin()
	mock.ExpectExec(`(?is)^INSERT INTO .ai_dry_runs.`).
		WithArgs(1, uint64(34), uint64(11), 3, `{"text":"a"}`, `{"label":"ok"}`, "uncertain", nil, nil, "queued", nil, nil, 7, sqlmock.AnyArg(), nil).
		WillReturnResult(sqlmock.NewResult(44, 1))
	mock.ExpectExec(`(?is)^INSERT INTO .outbox_events.`).
		WillReturnResult(sqlmock.NewResult(81, 1))
	mock.ExpectCommit()
	// 样本 12 没有 prompt(自身与任务均无),解析失败,不触发任何 DB 写。

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner1", Roles: []string{"owner"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/golden-samples/dry-runs", map[string]any{
		"sample_ids": []uint64{11, 12},
	}))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	data := responseData(t, rec)
	summary := data["summary"].(map[string]any)
	if summary["total"] != float64(2) || summary["queued"] != float64(1) || summary["failed"] != float64(1) {
		t.Fatalf("unexpected summary: %v", summary)
	}
	results := data["results"].([]any)
	first := results[0].(map[string]any)
	if first["goldenSampleId"] != float64(11) || first["status"] != "queued" || first["dryRunId"] != float64(44) {
		t.Fatalf("unexpected first result: %v", first)
	}
	second := results[1].(map[string]any)
	if second["goldenSampleId"] != float64(12) || second["status"] != "failed" {
		t.Fatalf("unexpected second result: %v", second)
	}
	if second["error"] != "active ai_prompt_id is required" {
		t.Fatalf("second error = %v", second["error"])
	}
	if _, exists := second["dryRunId"]; exists {
		t.Fatalf("failed config result should not have dryRunId: %v", second)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestGoldenSampleBatchDryRunRejectsTooManySamples(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status"}).
			AddRow(1, 7, "Task", "draft"))

	sampleIDs := make([]uint64, 0, maxGoldenSampleBatchDryRunSamples+1)
	for sampleID := uint64(1); sampleID <= maxGoldenSampleBatchDryRunSamples+1; sampleID++ {
		sampleIDs = append(sampleIDs, sampleID)
	}
	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner1", Roles: []string{"owner"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/golden-samples/dry-runs", map[string]any{
		"sample_ids": sampleIDs,
	}))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestGoldenSampleBatchDryRunReturns429WhenQuotaWouldBeExceeded(t *testing.T) {
	t.Setenv(dryRunQuotaMaxRunsEnv, "4")
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status"}).
			AddRow(1, 7, "Task", "draft"))
	mock.ExpectQuery(`(?is)^SELECT count\(\*\) FROM .ai_dry_runs. WHERE task_id = .+ AND created_at >= .+`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner1", Roles: []string{"owner"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/golden-samples/dry-runs", map[string]any{
		"sample_ids":   []uint64{11, 12},
		"repeat_count": 2,
	}))

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "RATE_LIMITED") {
		t.Fatalf("expected RATE_LIMITED body, got %s", rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestGoldenSampleDryRunCountsRepeatCountAgainstQuota(t *testing.T) {
	t.Setenv(dryRunQuotaMaxRunsEnv, "3")
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status"}).
			AddRow(1, 7, "Task", "draft"))
	mock.ExpectQuery(`(?is)^SELECT count\(\*\) FROM .ai_dry_runs. WHERE task_id = .+ AND created_at >= .+`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner1", Roles: []string{"owner"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/golden-samples/11/dry-run", map[string]any{
		"repeat_count": 3,
	}))

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "RATE_LIMITED") {
		t.Fatalf("expected RATE_LIMITED body, got %s", rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestGoldenSampleDryRunReturns429WhenCircuitBreakerOpen(t *testing.T) {
	t.Setenv(dryRunCircuitMaxFailuresEnv, "2")
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status"}).
			AddRow(1, 7, "Task", "draft"))
	mock.ExpectQuery(`(?is)^SELECT count\(\*\) FROM .ai_dry_runs. WHERE task_id = .+ AND status = .+ AND created_at >= .+`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner1", Roles: []string{"owner"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/golden-samples/11/dry-run", map[string]any{}))

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "dry-run circuit breaker is open") {
		t.Fatalf("expected circuit breaker body, got %s", rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestGoldenSampleBatchDryRunRejectsNonOwner(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status"}).
			AddRow(1, 99, "Task", "draft"))

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner1", Roles: []string{"owner"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/golden-samples/dry-runs", map[string]any{
		"sample_ids": []uint64{11},
	}))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestGoldenSampleBatchDryRunRejectsMissingSample(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status"}).
			AddRow(1, 7, "Task", "draft"))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .golden_samples.+task_id.+id IN`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "payload", "payload_hash", "expected_answer", "expected_verdict", "created_by"}).
			AddRow(11, 1, `{"text":"a"}`, "hash-a", `{"label":"ok"}`, "pass", 7))

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner1", Roles: []string{"owner"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/golden-samples/dry-runs", map[string]any{
		"sample_ids": []uint64{11, 99},
	}))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d, body=%s", rec.Code, rec.Body.String())
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
	return expectedPayloadHashRaw(t, string(raw))
}

func expectedPayloadHashRaw(t *testing.T, raw string) string {
	t.Helper()
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func rawJSONRequest(method, path string, body string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}
