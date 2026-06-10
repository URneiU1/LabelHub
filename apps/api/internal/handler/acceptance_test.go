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
	// loadApprovedSubmissions:已通过提交 + 当前答案,供 Owner 内联抽检。
	mock.ExpectQuery(`(?is)^SELECT.+FROM submissions AS s JOIN submission_revisions`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "item_id", "labeler_id", "ai_verdict", "ai_score", "answer"}).
			AddRow(1, 11, 3, "pass", 88.0, `{"relevance_score":5}`))
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
			ApprovedCount       int             `json:"approvedCount"`
			Batch               json.RawMessage `json:"batch"`
			ApprovedSubmissions []struct {
				ID     int    `json:"id"`
				ItemID int    `json:"itemId"`
				Answer string `json:"answer"`
			} `json:"approvedSubmissions"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.ApprovedCount != 5 {
		t.Errorf("approvedCount = %d, want 5", resp.Data.ApprovedCount)
	}
	if len(resp.Data.ApprovedSubmissions) != 1 || resp.Data.ApprovedSubmissions[0].ID != 1 || resp.Data.ApprovedSubmissions[0].ItemID != 11 {
		t.Fatalf("approvedSubmissions = %+v, want 1 row id=1 item=11", resp.Data.ApprovedSubmissions)
	}
	if resp.Data.ApprovedSubmissions[0].Answer != `{"relevance_score":5}` {
		t.Errorf("approvedSubmissions answer = %q", resp.Data.ApprovedSubmissions[0].Answer)
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
