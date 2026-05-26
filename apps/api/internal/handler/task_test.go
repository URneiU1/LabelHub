package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"labelhub-api/internal/auth"
)

func TestPreviewItemReturnsFirstAvailableTaskItemForOwner(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status"}).
			AddRow(1, 7, "Task", "draft"))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_items.+task_id.+status.+ORDER BY id ASC`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "external_id", "payload", "status"}).
			AddRow(11, 1, "item-1", `{"prompt":"Preview prompt","model_answer":"Preview answer"}`, itemStatusAvailable))

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner1", Roles: []string{"owner"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodGet, "/tasks/1/item-preview", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	data := responseData(t, rec)
	item := data["item"].(map[string]any)
	if item["id"] != float64(11) {
		t.Fatalf("item id = %v, want 11", item["id"])
	}
	if _, ok := item["payload"].(map[string]any); !ok {
		t.Fatalf("payload = %v", item["payload"])
	}
	if _, ok := item["claimedBy"]; ok {
		t.Fatalf("response leaked claimedBy: %v", item)
	}
	if _, ok := item["status"]; ok {
		t.Fatalf("response leaked status: %v", item)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestPreviewItemOwnerOnly(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status"}).
			AddRow(1, 99, "Task", "draft"))

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner2", Roles: []string{"owner"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodGet, "/tasks/1/item-preview", nil))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestPreviewItemReturnsNullWhenNoAvailableItem(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status"}).
			AddRow(1, 7, "Task", "draft"))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_items.+task_id.+status.+ORDER BY id ASC`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "external_id", "payload", "status"}))

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner1", Roles: []string{"owner"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodGet, "/tasks/1/item-preview", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	data := responseData(t, rec)
	if data["item"] != nil {
		t.Fatalf("item = %v, want nil", data["item"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}
