package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"labelhub-api/internal/auth"
)

// ---------- Reviewers (task_reviewers 指派) ----------

func TestListReviewers(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	expectOwnedTask(mock, "published")
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_reviewers.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "user_id"}).
			AddRow(1, 1, 5).
			AddRow(2, 1, 6))

	r := newGinWithClaims(ownerClaims())
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodGet, "/tasks/1/reviewers", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	reviewers := responseData(t, rec)["reviewers"].([]any)
	if len(reviewers) != 2 {
		t.Fatalf("expected 2 reviewers, got %d", len(reviewers))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestAddReviewersAssignsReviewerRoleUsers(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	expectOwnedTask(mock, "published")
	// 角色校验:请求两个用户(5,6),user_roles 命中两个去重 reviewer。
	mock.ExpectQuery(`(?is)^SELECT count.+FROM .user_roles.`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectBegin()
	// uid 5: 已存在 → SELECT 命中(FirstOrCreate 不 INSERT)
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_reviewers.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "user_id"}).AddRow(1, 1, 5))
	// uid 6: 不存在 → SELECT 空 → INSERT
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_reviewers.`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectExec(`(?is)^INSERT INTO .task_reviewers.`).
		WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectCommit()

	r := newGinWithClaims(ownerClaims())
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/reviewers", map[string]any{
		"userIds": []uint64{5, 6},
	}))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// 被指派用户里有人不具备 reviewer 角色:角色去重计数 < 请求去重数 → 422,不进事务。
func TestAddReviewersRejectsNonReviewerUser(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	expectOwnedTask(mock, "published")
	// 请求两个用户,但只有一个落在 reviewer 角色 → 计数 1 != 2。
	mock.ExpectQuery(`(?is)^SELECT count.+FROM .user_roles.`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	r := newGinWithClaims(ownerClaims())
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/reviewers", map[string]any{
		"userIds": []uint64{5, 6},
	}))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestRemoveReviewer(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	expectOwnedTask(mock, "published")
	mock.ExpectBegin()
	mock.ExpectExec(`(?is)^DELETE FROM .task_reviewers.`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	r := newGinWithClaims(ownerClaims())
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodDelete, "/tasks/1/reviewers/5", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestRemoveReviewerNotFound(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	expectOwnedTask(mock, "published")
	mock.ExpectBegin()
	mock.ExpectExec(`(?is)^DELETE FROM .task_reviewers.`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	r := newGinWithClaims(ownerClaims())
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodDelete, "/tasks/1/reviewers/5", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// 非该 task owner(owner_id 不匹配且非 admin)被 loadOwnedTask 拦在 403,不查 task_reviewers。
func TestAddReviewersForbidsNonOwner(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status"}).
			AddRow(1, 99, "Task", "published"))

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner1", Roles: []string{"owner"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/reviewers", map[string]any{
		"userIds": []uint64{5},
	}))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
