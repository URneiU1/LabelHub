package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"labelhub-api/internal/auth"
)

func TestUpdateTaskBaselinePersistsOwnerEditableBaseline(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status", "baseline_description"}).
			AddRow(1, 7, "Task", "draft", "old baseline"))
	mock.ExpectBegin()
	mock.ExpectExec(`(?is)^UPDATE .tasks.`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status", "baseline_description"}).
			AddRow(1, 7, "Task", "draft", "new demo baseline"))

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner1", Roles: []string{"owner"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/baseline", map[string]any{
		"baselineDescription": "new demo baseline",
	}))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	data := responseData(t, rec)
	task := data["task"].(map[string]any)
	if task["baselineDescription"] != "new demo baseline" {
		t.Fatalf("baselineDescription = %v", task["baselineDescription"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}
