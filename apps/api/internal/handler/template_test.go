package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	mysqlerr "github.com/go-sql-driver/mysql"

	"labelhub-api/internal/auth"
)

func TestListTemplatesOwnerOnly(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status"}).
			AddRow(1, 9, "Task", "draft"))

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner2", Roles: []string{"owner"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodGet, "/tasks/1/templates", nil))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestListTemplatesReturnsVersionsDesc(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status"}).
			AddRow(1, 7, "Task", "draft"))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_templates.+ORDER BY version DESC`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "version", "schema_json", "schema_hash", "created_by"}).
			AddRow(12, 1, 2, `{"fields":[{"name":"b","widget":"Input"}]}`, "hash2", 7).
			AddRow(11, 1, 1, `{"fields":[{"name":"a","widget":"Input"}]}`, "hash1", 7))

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner1", Roles: []string{"owner"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodGet, "/tasks/1/templates", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	data := resp["data"].([]any)
	if got := data[0].(map[string]any)["version"]; got != float64(2) {
		t.Fatalf("first version = %v, want 2", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestGetTemplateReportsLatest(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_templates.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "version", "schema_json", "schema_hash", "created_by"}).
			AddRow(11, 1, 1, `{"fields":[{"name":"a","widget":"Input"}]}`, "hash1", 7))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status"}).
			AddRow(1, 7, "Task", "draft"))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_templates.+ORDER BY version DESC`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "version", "schema_json", "schema_hash", "created_by"}).
			AddRow(12, 1, 2, `{"fields":[{"name":"b","widget":"Input"}]}`, "hash2", 7))

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner1", Roles: []string{"owner"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodGet, "/templates/11", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	data := responseData(t, rec)
	if data["isLatest"] != false {
		t.Fatalf("isLatest = %v, want false", data["isLatest"])
	}
	if data["latestTemplateId"] != float64(12) {
		t.Fatalf("latestTemplateId = %v, want 12", data["latestTemplateId"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestCreateTemplateBumpsVersionAndUpdatesTaskTemplateID(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status"}).
			AddRow(1, 7, "Task", "draft"))
	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status"}).
			AddRow(1, 7, "Task", "draft"))
	mock.ExpectQuery(`(?is)^SELECT COALESCE.+FROM .task_templates.`).
		WillReturnRows(sqlmock.NewRows([]string{"COALESCE(MAX(version),0)"}).AddRow(1))
	mock.ExpectExec(`(?is)^INSERT INTO .task_templates.`).
		WillReturnResult(sqlmock.NewResult(22, 1))
	mock.ExpectExec(`(?is)^UPDATE .tasks. SET .template_id.`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner1", Roles: []string{"owner"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/templates", validTemplateBody()))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	data := responseData(t, rec)
	if data["id"] != float64(22) {
		t.Fatalf("created id = %v, want 22", data["id"])
	}
	if data["version"] != float64(2) {
		t.Fatalf("version = %v, want 2", data["version"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestCreateTemplateDuplicateKeyReturns409(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status"}).
			AddRow(1, 7, "Task", "draft"))
	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status"}).
			AddRow(1, 7, "Task", "draft"))
	mock.ExpectQuery(`(?is)^SELECT COALESCE.+FROM .task_templates.`).
		WillReturnRows(sqlmock.NewRows([]string{"COALESCE(MAX(version),0)"}).AddRow(1))
	mock.ExpectExec(`(?is)^INSERT INTO .task_templates.`).
		WillReturnError(&mysqlerr.MySQLError{Number: 1062, Message: "Duplicate entry '1-2' for key 'uk_task_version'"})
	mock.ExpectRollback()

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner1", Roles: []string{"owner"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/templates", validTemplateBody()))

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

func TestCreateTemplateRejectsPublishedTask(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status"}).
			AddRow(1, 7, "Task", "published"))

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner1", Roles: []string{"owner"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/templates", validTemplateBody()))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestValidateTemplateReportsSchemaErrors(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status"}).
			AddRow(1, 7, "Task", "draft"))

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner1", Roles: []string{"owner"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/templates/validate", map[string]any{
		"fields": []map[string]any{
			{"name": "a", "widget": "Input", "required": true, "minLength": 10, "maxLength": 5},
			{"name": "a", "widget": "Slider", "required": "yes"},
		},
	}))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	data := responseData(t, rec)
	if data["valid"] != false {
		t.Fatalf("valid = %v, want false", data["valid"])
	}
	fields := map[string]bool{}
	for _, raw := range data["errors"].([]any) {
		errObj := raw.(map[string]any)
		fields[errObj["field"].(string)] = true
	}
	for _, field := range []string{"fields[1].name", "fields[1].widget", "fields[1].required", "fields[0].maxLength"} {
		if !fields[field] {
			t.Fatalf("expected validation error for %s, got fields=%v", field, fields)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestCreateTemplateRejectsOversizedBody(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status"}).
			AddRow(1, 7, "Task", "draft"))

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner1", Roles: []string{"owner"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/templates", map[string]any{
		"title":  strings.Repeat("x", int(maxTemplateSchemaBytes)+1),
		"fields": []map[string]any{{"name": "a", "widget": "Input"}},
	}))

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestBindCanonicalTemplateSchemaPreservesExportFieldsAndExtensions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = jsonRequest(http.MethodPost, "/tasks/1/templates", map[string]any{
		"title":         "Custom",
		"layout":        "single_page",
		"export_fields": []string{"payload", "answer"},
		"x-owner-note":  map[string]any{"source": "designer"},
		"fields": []map[string]any{{
			"name": "summary", "widget": "Input", "x-field-note": "kept",
		}},
	})

	raw, ok := bindCanonicalTemplateSchema(c, "Fallback")
	if !ok {
		t.Fatalf("expected canonical schema, got response %d: %s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got["title"] != "Custom" {
		t.Fatalf("title = %v", got["title"])
	}
	if got["x-owner-note"] == nil {
		t.Fatalf("missing x-owner-note in %s", string(raw))
	}
	if _, ok := got["export_fields"].([]any); !ok {
		t.Fatalf("missing export_fields in %s", string(raw))
	}
	fields := got["fields"].([]any)
	if fields[0].(map[string]any)["x-field-note"] != "kept" {
		t.Fatalf("field extension lost: %s", string(raw))
	}
}

func validTemplateBody() map[string]any {
	return map[string]any{
		"layout": "single_page",
		"fields": []map[string]any{
			{"name": "summary", "widget": "Input", "label": "Summary", "required": true},
		},
	}
}

func responseData(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	data, ok := resp["data"].(map[string]any)
	if !ok {
		t.Fatalf("response data is %T, want object: %s", resp["data"], rec.Body.String())
	}
	return data
}
