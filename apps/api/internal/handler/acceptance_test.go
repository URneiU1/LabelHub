package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"labelhub-api/internal/auth"
)

func TestAcceptanceStatusReturnsApprovedCountWithNoBatch(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status"}).AddRow(1, 7, "Task", "published"))
	mock.ExpectQuery(`(?is)^SELECT count\(\*\) FROM .submissions. WHERE task_id = \? AND status = \?`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(5))
	mock.ExpectQuery(`(?is)^SELECT \* FROM .acceptance_batches. WHERE task_id = \?`).
		WillReturnRows(sqlmock.NewRows([]string{"id"})) // no batch yet

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner1", Roles: []string{"owner"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodGet, "/tasks/1/acceptance", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			ApprovedCount int             `json:"approvedCount"`
			Batch         json.RawMessage `json:"batch"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.ApprovedCount != 5 {
		t.Errorf("approvedCount = %d, want 5", resp.Data.ApprovedCount)
	}
	if string(resp.Data.Batch) != "null" {
		t.Errorf("batch = %s, want null", resp.Data.Batch)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestAcceptanceStartReturns409WhenActiveBatchExists(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	// loadOwnedTask
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status"}).AddRow(1, 7, "Task", "published"))
	// service Start tx
	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT \* FROM .tasks. WHERE .+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "status"}).AddRow(1, 7, "published"))
	mock.ExpectQuery(`(?is)^SELECT count\(\*\) FROM .acceptance_batches. WHERE task_id = \? AND status = \?`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectRollback()

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner1", Roles: []string{"owner"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/acceptance", nil))

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestAcceptanceRejectsNonOwner(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	// task owned by 8, current owner is 7 -> 403 before any service call.
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status"}).AddRow(1, 8, "Task", "published"))

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner1", Roles: []string{"owner"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/acceptance/accept", map[string]any{"batch_id": 42}))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestAcceptanceRequiresOwnerRole(t *testing.T) {
	db, _, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	// labeler role -> RequireRoles blocks before any DB access.
	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "labeler1", Roles: []string{"labeler"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodGet, "/tasks/1/acceptance", nil))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d, body=%s", rec.Code, rec.Body.String())
	}
}
