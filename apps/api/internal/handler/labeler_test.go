package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"labelhub-api/internal/auth"
)

// 验证 ListMyTaskItems 把任务下题目与当前 labeler 的 submission 合并成 available/claimed/<sub状态>,
// 并正确计数。item10 无人动 -> available;item11 被我 claim 但无 submission -> claimed;
// item12 有我的 submission(submitted)-> submitted + mine + submissionId。
func TestListMyTaskItemsMergesPerItemStatus(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	const me = 5

	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "status"}).
			AddRow(1, 7, "published"))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_items.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "external_id", "status", "claimed_by"}).
			AddRow(10, 1, "Q10", "available", nil).
			AddRow(11, 1, "Q11", "claimed", me).
			AddRow(12, 1, "Q12", "claimed", me))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .submissions.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "item_id", "labeler_id", "status"}).
			AddRow(99, 1, 12, me, "submitted"))

	r := newGinWithClaims(&auth.Claims{UserID: me, Username: "labeler1", Roles: []string{"labeler"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodGet, "/tasks/1/labeler/items", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	data := responseData(t, rec)
	items, _ := data["items"].([]any)
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d (body=%s)", len(items), rec.Body.String())
	}
	statusByItem := map[float64]string{}
	for _, raw := range items {
		it := raw.(map[string]any)
		statusByItem[it["itemId"].(float64)] = it["status"].(string)
	}
	if statusByItem[10] != "available" {
		t.Fatalf("item10 status = %q, want available", statusByItem[10])
	}
	if statusByItem[11] != "claimed" {
		t.Fatalf("item11 status = %q, want claimed", statusByItem[11])
	}
	if statusByItem[12] != "submitted" {
		t.Fatalf("item12 status = %q, want submitted", statusByItem[12])
	}
	counts, _ := data["counts"].(map[string]any)
	if counts["available"].(float64) != 1 || counts["claimed"].(float64) != 1 || counts["submitted"].(float64) != 1 {
		t.Fatalf("counts = %v", counts)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

// 验证 MyTasks 把当前 labeler 的提交按大任务聚合:按最近活跃排序、附带我的各状态计数、
// 进行中数量与可恢复目标(最近 draft/revising 的 itemId)。
func TestMyTasksGroupsByBigTaskWithMyProgress(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	const me = 5

	// 我的提交按 updated_at DESC:任务2 的 draft 最近;任务1 有 submitted + approved。
	mock.ExpectQuery(`(?is)^SELECT.+FROM .submissions.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "item_id", "labeler_id", "status"}).
			AddRow(42, 2, 31, me, "draft").
			AddRow(10, 1, 11, me, "submitted").
			AddRow(11, 1, 12, me, "approved"))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_items.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "claimed_by"}))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "title", "status", "total_items", "finished_items"}).
			AddRow(1, "Task One", "published", 30, 7).
			AddRow(2, "Task Two", "published", 12, 0))

	r := newGinWithClaims(&auth.Claims{UserID: me, Username: "labeler1", Roles: []string{"labeler"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodGet, "/me/tasks", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	data := responseData(t, rec)
	tasks, _ := data["tasks"].([]any)
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d (body=%s)", len(tasks), rec.Body.String())
	}

	// 最近活跃的任务2 在前,带进行中草稿可恢复。
	first := tasks[0].(map[string]any)
	firstTask := first["task"].(map[string]any)
	if firstTask["id"].(float64) != 2 {
		t.Fatalf("first task id = %v, want 2 (most recent)", firstTask["id"])
	}
	if first["myInProgress"].(float64) != 1 {
		t.Fatalf("task2 myInProgress = %v, want 1", first["myInProgress"])
	}
	if first["resumeItemId"].(float64) != 31 {
		t.Fatalf("task2 resumeItemId = %v, want 31", first["resumeItemId"])
	}
	if first["myTotal"].(float64) != 1 {
		t.Fatalf("task2 myTotal = %v, want 1", first["myTotal"])
	}

	// 任务1:2 条提交,无进行中,无 resume。
	second := tasks[1].(map[string]any)
	secondTask := second["task"].(map[string]any)
	if secondTask["id"].(float64) != 1 {
		t.Fatalf("second task id = %v, want 1", secondTask["id"])
	}
	if second["myTotal"].(float64) != 2 {
		t.Fatalf("task1 myTotal = %v, want 2", second["myTotal"])
	}
	if second["myInProgress"].(float64) != 0 {
		t.Fatalf("task1 myInProgress = %v, want 0", second["myInProgress"])
	}
	if second["resumeItemId"] != nil {
		t.Fatalf("task1 resumeItemId = %v, want nil", second["resumeItemId"])
	}
	counts := second["myCounts"].(map[string]any)
	if counts["submitted"].(float64) != 1 || counts["approved"].(float64) != 1 {
		t.Fatalf("task1 myCounts = %v", counts)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

// ClaimTask 整体领取:first_come 任务无人占用 → 200,返回 task。
func TestClaimTask_FirstComeReturnsTask(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "distribution", "lease_timeout_minutes"}).
			AddRow(1, "published", "first_come", 0))
	mock.ExpectQuery(`(?is)^SELECT count.+FROM .task_items.`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`(?is)^SELECT count.+FROM .submissions.`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec(`(?is)^UPDATE .task_items. SET`).
		WillReturnResult(sqlmock.NewResult(0, 12))
	mock.ExpectCommit()

	r := newGinWithClaims(&auth.Claims{UserID: 5, Username: "labeler1", Roles: []string{"labeler"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/claim-task", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	data := responseData(t, rec)
	task, _ := data["task"].(map[string]any)
	if task["id"].(float64) != 1 {
		t.Fatalf("task id = %v, want 1", task["id"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

// ClaimTask 独占:被他人领取 → 409。
func TestClaimTask_RejectsWhenTaken(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "distribution", "lease_timeout_minutes"}).
			AddRow(1, "published", "first_come", 0))
	mock.ExpectQuery(`(?is)^SELECT count.+FROM .task_items.`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectQuery(`(?is)^SELECT count.+FROM .submissions.`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectRollback()

	r := newGinWithClaims(&auth.Claims{UserID: 5, Username: "labeler1", Roles: []string{"labeler"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/claim-task", nil))

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

// 没有任何提交时,MyTasks 返回空列表且不查 tasks。
func TestMyTasksEmptyWhenNoSubmissions(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectQuery(`(?is)^SELECT.+FROM .submissions.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "item_id", "labeler_id", "status"}))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_items.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "claimed_by"}))

	r := newGinWithClaims(&auth.Claims{UserID: 9, Username: "labeler1", Roles: []string{"labeler"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodGet, "/me/tasks", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	data := responseData(t, rec)
	if tasks, _ := data["tasks"].([]any); len(tasks) != 0 {
		t.Fatalf("expected 0 tasks, got %d", len(tasks))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

// MyTasks 把「整体领取后认领但还没作答」的任务也算进来:进行中 = 认领待做题数,resume 指向第一道待做题。
func TestMyTasksIncludesClaimedButUnansweredTask(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	const me = 5
	// 无任何提交,但在任务5 里认领了 3 道题(整体领取后尚未作答)。
	mock.ExpectQuery(`(?is)^SELECT.+FROM .submissions.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "item_id", "labeler_id", "status"}))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_items.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "claimed_by"}).
			AddRow(71, 5, me).
			AddRow(72, 5, me).
			AddRow(73, 5, me))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "title", "status", "total_items", "finished_items"}).
			AddRow(5, "Claimed Task", "published", 3, 0))

	r := newGinWithClaims(&auth.Claims{UserID: me, Username: "labeler1", Roles: []string{"labeler"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodGet, "/me/tasks", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	data := responseData(t, rec)
	tasks, _ := data["tasks"].([]any)
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d (body=%s)", len(tasks), rec.Body.String())
	}
	tv := tasks[0].(map[string]any)
	if tv["task"].(map[string]any)["id"].(float64) != 5 {
		t.Fatalf("task id = %v, want 5", tv["task"].(map[string]any)["id"])
	}
	if tv["myInProgress"].(float64) != 3 {
		t.Fatalf("myInProgress = %v, want 3 (claimed-unanswered)", tv["myInProgress"])
	}
	if tv["myTotal"].(float64) != 0 {
		t.Fatalf("myTotal = %v, want 0 (no submissions yet)", tv["myTotal"])
	}
	if tv["resumeItemId"].(float64) != 71 {
		t.Fatalf("resumeItemId = %v, want 71 (first claimed)", tv["resumeItemId"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestListMyTaskItemsKeepsReleasedOverlapItemAvailable(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "status"}).AddRow(1, 7, "published"))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_items.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "status"}).AddRow(10, 1, "available"))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .submissions.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "item_id", "labeler_id", "status"}).
			AddRow(99, 1, 10, 6, "submitted"))

	r := newGinWithClaims(&auth.Claims{UserID: 5, Username: "labeler1", Roles: []string{"labeler"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodGet, "/tasks/1/labeler/items", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	data := responseData(t, rec)
	items := data["items"].([]any)
	if got := items[0].(map[string]any)["status"]; got != "available" {
		t.Fatalf("status = %v, want available", got)
	}
}
