package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"

	"labelhub-api/internal/auth"

	"labelhub.local/exporter"
)

func ownerTaskRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "owner_id", "title", "status"}).AddRow(1, 7, "Task", "draft")
}

func ownerGin() *gin.Engine {
	return newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner1", Roles: []string{"owner"}})
}

func TestCreateExport_QueuesAndReturnsID(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).WillReturnRows(ownerTaskRows())
	mock.ExpectBegin()
	mock.ExpectExec(`(?is)^INSERT INTO .exports.`).WillReturnResult(sqlmock.NewResult(99, 1))
	mock.ExpectExec(`(?is)^INSERT INTO .outbox_events.`).WillReturnResult(sqlmock.NewResult(81, 1))
	mock.ExpectExec(`(?is)^INSERT INTO .audit_logs.`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	r := ownerGin()
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/exports", map[string]any{"format": "csv", "include_reviews": true}))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	data := responseData(t, rec)
	if data["status"] != "queued" || data["id"] != float64(99) {
		t.Fatalf("unexpected response: %v", data)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestCreateExport_RejectsBadFormat(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).WillReturnRows(ownerTaskRows())

	r := ownerGin()
	registerAllHandlers(r, db)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/exports", map[string]any{"format": "pdf"}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestLegacySyncJSONExportRouteIsRemoved(t *testing.T) {
	db, _, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	r := ownerGin()
	registerAllHandlers(r, db)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tasks/1/export/json", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestListExports_ReturnsHistory(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).WillReturnRows(ownerTaskRows())
	mock.ExpectQuery(`(?is)^SELECT.+FROM .exports. WHERE task_id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "created_by", "format", "status"}).
			AddRow(99, 1, 7, "csv", "succeeded").
			AddRow(98, 1, 7, "json", "queued"))

	r := ownerGin()
	registerAllHandlers(r, db)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/tasks/1/exports", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	data := responseData(t, rec)
	if list, ok := data["exports"].([]any); !ok || len(list) != 2 {
		t.Fatalf("expected 2 exports, got %v", data["exports"])
	}
}

func TestDownloadURL_SignsForSucceededExport(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).WillReturnRows(ownerTaskRows())
	mock.ExpectQuery(`(?is)^SELECT.+FROM .exports. WHERE id = .+ AND task_id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "format", "status", "file_path"}).
			AddRow(99, 1, "csv", "succeeded", "/tmp/x.csv"))

	r := ownerGin()
	registerAllHandlers(r, db)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/tasks/1/exports/99/download-url", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	data := responseData(t, rec)
	if url, _ := data["url"].(string); url == "" {
		t.Fatalf("expected signed url, got %v", data)
	}
}

func TestDownload_ValidTokenStreamsFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("EXPORT_DIR", dir)
	taskDir := filepath.Join(dir, "1")
	_ = os.MkdirAll(taskDir, 0o755)
	filePath := filepath.Join(taskDir, "out.csv")
	content := []byte("col\nvalue\n")
	if err := os.WriteFile(filePath, content, 0o644); err != nil {
		t.Fatal(err)
	}

	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .exports. WHERE id = .+ AND status`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "format", "status", "file_path"}).
			AddRow(99, 1, "csv", "succeeded", filePath))

	r := ownerGin()
	registerAllHandlers(r, db)
	token := exporter.SignDownloadToken(testExportSecret, 99, time.Now().Add(time.Minute))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/exports/download?token="+token, nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != string(content) {
		t.Fatalf("body mismatch: %q", rec.Body.String())
	}
}

func TestDownload_ExpiredTokenReturns410(t *testing.T) {
	db, _, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	r := ownerGin()
	registerAllHandlers(r, db)
	token := exporter.SignDownloadToken(testExportSecret, 99, time.Now().Add(-time.Second))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/exports/download?token="+token, nil))
	if rec.Code != http.StatusGone {
		t.Fatalf("expected 410, got %d", rec.Code)
	}
}

func TestDownload_TamperedTokenReturns401(t *testing.T) {
	db, _, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	r := ownerGin()
	registerAllHandlers(r, db)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/exports/download?token=bogus.token", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestDownload_PathTraversalBlocked(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("EXPORT_DIR", dir)
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .exports. WHERE id = .+ AND status`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "format", "status", "file_path"}).
			AddRow(99, 1, "csv", "succeeded", "/etc/passwd"))

	r := ownerGin()
	registerAllHandlers(r, db)
	token := exporter.SignDownloadToken(testExportSecret, 99, time.Now().Add(time.Minute))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/exports/download?token="+token, nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
}
