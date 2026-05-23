package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"labelhub-api/internal/auth"
)

func TestCreateAIPromptRejectsNonOwner(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status"}).
			AddRow(1, 99, "Task", "draft"))

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner2", Roles: []string{"owner"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/ai-prompts", validAIPromptBody()))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestCreateAIPromptBumpsVersionAndUpdatesTaskPromptID(t *testing.T) {
	t.Setenv("LLM_ALLOWED_MODELS", "doubao-seed-2.0-lite")
	t.Setenv("LLM_MODEL", "doubao-seed-2.0-lite")
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status"}).
			AddRow(1, 7, "Task", "draft"))
	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status"}).
			AddRow(1, 7, "Task", "draft"))
	mock.ExpectQuery(`(?is)^SELECT COALESCE.+FROM .ai_prompt_configs.`).
		WillReturnRows(sqlmock.NewRows([]string{"COALESCE(MAX(version),0)"}).AddRow(2))
	mock.ExpectExec(`(?is)^INSERT INTO .ai_prompt_configs.`).
		WillReturnResult(sqlmock.NewResult(33, 1))
	mock.ExpectExec(`(?is)^UPDATE .tasks. SET .ai_prompt_id.`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner1", Roles: []string{"owner"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/ai-prompts", validAIPromptBody()))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	data := responseData(t, rec)
	prompt := data["prompt"].(map[string]any)
	if prompt["id"] != float64(33) {
		t.Fatalf("created id = %v, want 33", prompt["id"])
	}
	if prompt["version"] != float64(3) {
		t.Fatalf("version = %v, want 3", prompt["version"])
	}
	if prompt["model"] != "doubao-seed-2.0-lite" {
		t.Fatalf("model = %v, want env default", prompt["model"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestAIPromptDryRunUsesMockProviderWithoutMutatingSubmissionState(t *testing.T) {
	t.Setenv("LLM_PROVIDER", "mock")
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status"}).
			AddRow(1, 7, "Task", "draft"))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .ai_prompt_configs.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "version", "prompt_template", "dimensions", "pass_threshold", "uncertain_min", "model", "created_by"}).
			AddRow(33, 1, 3, "review {{answer.summary}}", `[{"name":"相关性"}]`, 80, 60, "mock-model", 7))
	mock.ExpectBegin()
	mock.ExpectExec(`(?is)^INSERT INTO .ai_dry_runs.`).
		WillReturnResult(sqlmock.NewResult(44, 1))
	mock.ExpectCommit()

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner1", Roles: []string{"owner"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/ai-prompts/33/dry-run", map[string]any{
		"payload": map[string]any{"prompt": "question"},
		"answer":  map[string]any{"summary": "ok"},
	}))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	data := responseData(t, rec)
	if data["provider"] != "mock" {
		t.Fatalf("provider = %v, want mock", data["provider"])
	}
	result := data["result"].(map[string]any)
	if result["verdict"] == "" || result["overall_score"] == nil {
		t.Fatalf("missing structured result: %v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestUpdateAIReviewSettingsRejectsNonOwner(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status"}).
			AddRow(1, 99, "Task", "draft"))

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner2", Roles: []string{"owner"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/ai-review-settings", map[string]any{"enabled": true}))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestUpdateAIReviewSettingsEnableRequiresActivePrompt(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status", "ai_prompt_id"}).
			AddRow(1, 7, "Task", "draft", nil))
	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status", "ai_prompt_id"}).
			AddRow(1, 7, "Task", "draft", nil))
	mock.ExpectRollback()

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner1", Roles: []string{"owner"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/ai-review-settings", map[string]any{"enabled": true}))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestUpdateAIReviewSettingsEnableWithActivePrompt(t *testing.T) {
	promptID := uint64(33)
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status", "ai_prompt_id", "ai_review_enabled"}).
			AddRow(1, 7, "Task", "draft", promptID, false))
	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status", "ai_prompt_id", "ai_review_enabled"}).
			AddRow(1, 7, "Task", "draft", promptID, false))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .ai_prompt_configs.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "version", "model"}).AddRow(promptID, 1, 3, "mock-model"))
	mock.ExpectExec(`(?is)^UPDATE .tasks. SET .ai_review_enabled.`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	r := newGinWithClaims(&auth.Claims{UserID: 999, Username: "admin", Roles: []string{"admin"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/ai-review-settings", map[string]any{"enabled": true}))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	data := responseData(t, rec)
	if data["aiReviewEnabled"] != true {
		t.Fatalf("aiReviewEnabled = %v, want true", data["aiReviewEnabled"])
	}
	if data["activePromptId"] != float64(promptID) {
		t.Fatalf("activePromptId = %v, want %d", data["activePromptId"], promptID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestUpdateAIReviewSettingsEnableRejectsDisallowedActiveModel(t *testing.T) {
	t.Setenv("LLM_ALLOWED_MODELS", "allowed-model")
	promptID := uint64(33)
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status", "ai_prompt_id", "ai_review_enabled"}).
			AddRow(1, 7, "Task", "draft", promptID, false))
	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status", "ai_prompt_id", "ai_review_enabled"}).
			AddRow(1, 7, "Task", "draft", promptID, false))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .ai_prompt_configs.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "version", "model"}).AddRow(promptID, 1, 3, "blocked-model"))
	mock.ExpectRollback()

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner1", Roles: []string{"owner"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/ai-review-settings", map[string]any{"enabled": true}))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "VALIDATION_ERROR") {
		t.Fatalf("expected VALIDATION_ERROR, body=%s", rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestUpdateAIReviewSettingsDisableKeepsActivePrompt(t *testing.T) {
	promptID := uint64(33)
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status", "ai_prompt_id", "ai_review_enabled"}).
			AddRow(1, 7, "Task", "draft", promptID, true))
	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status", "ai_prompt_id", "ai_review_enabled"}).
			AddRow(1, 7, "Task", "draft", promptID, true))
	mock.ExpectExec(`(?is)^UPDATE .tasks. SET .ai_review_enabled.`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner1", Roles: []string{"owner"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/ai-review-settings", map[string]any{"enabled": false}))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	data := responseData(t, rec)
	if data["aiReviewEnabled"] != false {
		t.Fatalf("aiReviewEnabled = %v, want false", data["aiReviewEnabled"])
	}
	if data["activePromptId"] != float64(promptID) {
		t.Fatalf("activePromptId = %v, want %d", data["activePromptId"], promptID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func validAIPromptBody() map[string]any {
	return map[string]any{
		"prompt_template": "请根据 {{payload.prompt}} 和 {{answer.summary}} 预审。",
		"dimensions": []map[string]any{
			{"name": "相关性", "description": "是否相关", "weight": 1},
		},
		"pass_threshold": 80,
		"uncertain_min":  60,
	}
}
