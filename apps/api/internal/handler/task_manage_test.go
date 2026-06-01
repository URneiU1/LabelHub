package handler

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/xuri/excelize/v2"

	"labelhub-api/internal/auth"
)

func ownerClaims() *auth.Claims {
	return &auth.Claims{UserID: 7, Username: "owner1", Roles: []string{"owner"}}
}

// expectOwnedTask:loadOwnedTask 先 SELECT 一行 owner_id=7 的 task。
func expectOwnedTask(mock sqlmock.Sqlmock, status string) {
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status", "template_id"}).
			AddRow(1, 7, "Task", status, 11))
}

// multipartImportRequest 构造 import-file 的 multipart 请求(file + 可选 format)。
func multipartImportRequest(path, filename, format string, content []byte) *http.Request {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	if format != "" {
		_ = writer.WriteField("format", format)
	}
	part, _ := writer.CreateFormFile("file", filename)
	_, _ = part.Write(content)
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

// ---------- CreateTask / UpdateTask ----------

func TestCreateTaskPersistsBasicInfoFields(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`(?is)^INSERT INTO .tasks.`).
		WillReturnResult(sqlmock.NewResult(42, 1))
	mock.ExpectCommit()

	r := newGinWithClaims(ownerClaims())
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks", map[string]any{
		"title":                          "标注任务",
		"description":                    "请仔细标注",
		"tags":                           []string{"nlp", "zh"},
		"rewardConfig":                   map[string]any{"perItem": 0.5},
		"distribution":                   "quota",
		"quotaPerUser":                   10,
		"overlapCount":                   3,
		"overlapCoveragePct":             25,
		"leaseTimeoutMinutes":            45,
		"reviewSamplingPct":              30,
		"dailySubmissionLimitPerLabeler": 80,
		"humanReviewEnabled":             false,
	}))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	data := responseData(t, rec)
	if data["distribution"] != "quota" {
		t.Fatalf("distribution = %v", data["distribution"])
	}
	if data["quotaPerUser"].(float64) != 10 {
		t.Fatalf("quotaPerUser = %v", data["quotaPerUser"])
	}
	if data["overlapCount"].(float64) != 3 || data["overlapCoveragePct"].(float64) != 25 {
		t.Fatalf("overlap policy = %v/%v", data["overlapCount"], data["overlapCoveragePct"])
	}
	if data["leaseTimeoutMinutes"].(float64) != 45 || data["reviewSamplingPct"].(float64) != 30 {
		t.Fatalf("lease/review policy = %v/%v", data["leaseTimeoutMinutes"], data["reviewSamplingPct"])
	}
	if data["dailySubmissionLimitPerLabeler"].(float64) != 80 {
		t.Fatalf("daily limit = %v", data["dailySubmissionLimitPerLabeler"])
	}
	if data["humanReviewEnabled"] != false {
		t.Fatalf("humanReviewEnabled = %v", data["humanReviewEnabled"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestUpdateTaskRejectsFrozenPolicyChangeAfterPublish(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	expectOwnedTask(mock, "published")

	r := newGinWithClaims(ownerClaims())
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPut, "/tasks/1", map[string]any{
		"reviewSamplingPct": 10,
	}))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestUpdateTaskRejectsFrozenPolicyWriteLostToConcurrentPublish(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	expectOwnedTask(mock, "draft")
	mock.ExpectBegin()
	mock.ExpectExec(`(?is)^UPDATE .tasks. SET .+ WHERE id = .+ AND status = .+`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	r := newGinWithClaims(ownerClaims())
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPut, "/tasks/1", map[string]any{
		"quotaPerUser": 20,
	}))
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestCreateTaskRejectsInvalidDistribution(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	r := newGinWithClaims(ownerClaims())
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks", map[string]any{
		"title":        "t",
		"distribution": "lottery",
	}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestUpdateTaskPersistsProvidedFields(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	expectOwnedTask(mock, "draft")
	mock.ExpectBegin()
	mock.ExpectExec(`(?is)^UPDATE .tasks.`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status", "distribution"}).
			AddRow(1, 7, "新标题", "draft", "assigned"))

	r := newGinWithClaims(ownerClaims())
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPut, "/tasks/1", map[string]any{
		"title":        "新标题",
		"distribution": "assigned",
		"tags":         []string{"a"},
	}))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	data := responseData(t, rec)
	task := data["task"].(map[string]any)
	if task["title"] != "新标题" || task["distribution"] != "assigned" {
		t.Fatalf("task = %v", task)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestUpdateTaskForbidsNonOwner(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	// task owner_id=99,当前 owner=7 → loadOwnedTask 403。
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status"}).
			AddRow(1, 99, "Task", "draft"))

	r := newGinWithClaims(ownerClaims())
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPut, "/tasks/1", map[string]any{"title": "x"}))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// ---------- Lifecycle ----------

func TestPublishTaskSucceedsWithTemplate(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	expectOwnedTask(mock, "draft") // template_id=11 set
	mock.ExpectBegin()
	mock.ExpectExec(`(?is)^UPDATE .tasks.`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^INSERT INTO .audit_logs.`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "status"}).AddRow(1, 7, "published"))

	r := newGinWithClaims(ownerClaims())
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/publish", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	data := responseData(t, rec)
	if data["task"].(map[string]any)["status"] != "published" {
		t.Fatalf("status = %v", data["task"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestPublishTaskRejectsWithoutTemplate(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	// template_id NULL → 422.
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "status", "template_id"}).
			AddRow(1, 7, "draft", nil))

	r := newGinWithClaims(ownerClaims())
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/publish", nil))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestPauseTaskRejectsIllegalTransition(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	// draft 不能 pause → 422.
	expectOwnedTask(mock, "draft")

	r := newGinWithClaims(ownerClaims())
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/pause", nil))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestEndTaskFromPaused(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	expectOwnedTask(mock, "paused")
	mock.ExpectBegin()
	mock.ExpectExec(`(?is)^UPDATE .tasks.`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^INSERT INTO .audit_logs.`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "status"}).AddRow(1, 7, "ended"))

	r := newGinWithClaims(ownerClaims())
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/end", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if responseData(t, rec)["task"].(map[string]any)["status"] != "ended" {
		t.Fatalf("status not ended")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// ---------- Import file ----------

// expectImportInsert:insertImportedItems 的事务内对每条 payload 做 FirstOrCreate
// (SELECT 查无 → INSERT)+ 最后 UPDATE total_items。这里假定全部为新行。
func expectImportInsert(mock sqlmock.Sqlmock, rows int) {
	mock.ExpectBegin()
	for i := 0; i < rows; i++ {
		mock.ExpectQuery(`(?is)^SELECT.+FROM .task_items.`).
			WillReturnRows(sqlmock.NewRows([]string{"id"}))
		mock.ExpectExec(`(?is)^INSERT INTO .task_items.`).
			WillReturnResult(sqlmock.NewResult(int64(100+i), 1))
	}
	mock.ExpectExec(`(?is)^UPDATE .tasks. SET .total_items`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
}

func TestImportItemsFileJSONArray(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	expectOwnedTask(mock, "draft")
	expectImportInsert(mock, 2)

	r := newGinWithClaims(ownerClaims())
	registerAllHandlers(r, db)

	body := []byte(`[{"id":"a","text":"x"},{"id":"b","text":"y"}]`)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, multipartImportRequest("/tasks/1/items/import-file", "data.json", "json", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if responseData(t, rec)["imported"].(float64) != 2 {
		t.Fatalf("imported != 2: %s", rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestImportItemsFileJSONWrapper(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	expectOwnedTask(mock, "draft")
	expectImportInsert(mock, 1)

	r := newGinWithClaims(ownerClaims())
	registerAllHandlers(r, db)

	body := []byte(`{"items":[{"id":"only"}]}`)
	rec := httptest.NewRecorder()
	// format omitted → inferred from .json filename
	r.ServeHTTP(rec, multipartImportRequest("/tasks/1/items/import-file", "wrap.json", "", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestImportItemsFileJSONL(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	expectOwnedTask(mock, "draft")
	expectImportInsert(mock, 2)

	r := newGinWithClaims(ownerClaims())
	registerAllHandlers(r, db)

	body := []byte("{\"id\":\"1\"}\n\n{\"id\":\"2\"}\n")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, multipartImportRequest("/tasks/1/items/import-file", "data.jsonl", "jsonl", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if responseData(t, rec)["imported"].(float64) != 2 {
		t.Fatalf("imported != 2: %s", rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestImportItemsFileXLSX(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	expectOwnedTask(mock, "draft")
	expectImportInsert(mock, 2)

	// 构造一个真实 xlsx:首行表头 + 两条数据行。
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	_ = f.SetSheetRow(sheet, "A1", &[]any{"id", "text"})
	_ = f.SetSheetRow(sheet, "A2", &[]any{"r1", "hello"})
	_ = f.SetSheetRow(sheet, "A3", &[]any{"r2", "world"})
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("write xlsx: %v", err)
	}
	_ = f.Close()

	r := newGinWithClaims(ownerClaims())
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, multipartImportRequest("/tasks/1/items/import-file", "data.xlsx", "xlsx", buf.Bytes()))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if responseData(t, rec)["imported"].(float64) != 2 {
		t.Fatalf("imported != 2: %s", rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestImportItemsFileRejectsEmptyParse(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	expectOwnedTask(mock, "draft")

	r := newGinWithClaims(ownerClaims())
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, multipartImportRequest("/tasks/1/items/import-file", "data.json", "json", []byte(`[]`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// H-02:发布后不得再导入条目(题集冻结),import-file 返回 409,不读文件不进事务。
func TestImportItemsFileRejectedAfterPublish(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	expectOwnedTask(mock, "published")

	r := newGinWithClaims(ownerClaims())
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, multipartImportRequest("/tasks/1/items/import-file", "data.json", "json", []byte(`[{"id":"a"}]`)))
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// ---------- Batch update ----------

func TestBatchUpdateItemsOverwritesScopedToTask(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	expectOwnedTask(mock, "draft")
	mock.ExpectBegin()
	mock.ExpectExec(`(?is)^UPDATE .task_items. SET .payload`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^UPDATE .task_items. SET .payload`).
		WillReturnResult(sqlmock.NewResult(0, 0)) // 第二条不属于本任务 → 0 行
	mock.ExpectCommit()

	r := newGinWithClaims(ownerClaims())
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/items/batch-update", map[string]any{
		"items": []map[string]any{
			{"itemId": 10, "payload": map[string]any{"text": "new10"}},
			{"itemId": 999, "payload": map[string]any{"text": "alien"}},
		},
	}))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	data := responseData(t, rec)
	if data["updated"].(float64) != 1 || data["requested"].(float64) != 2 {
		t.Fatalf("updated/requested = %v / %v", data["updated"], data["requested"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// H-02:发布后题集冻结,批量覆盖 payload 必须被拒(409),不进事务、不改任何行。
func TestBatchUpdateItemsRejectedAfterPublish(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	expectOwnedTask(mock, "published")

	r := newGinWithClaims(ownerClaims())
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/items/batch-update", map[string]any{
		"items": []map[string]any{
			{"itemId": 10, "payload": map[string]any{"text": "new10"}},
		},
	}))
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// ---------- Assignees ----------

func TestListAssignees(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	expectOwnedTask(mock, "published")
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_assignees.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "user_id"}).
			AddRow(1, 1, 5).
			AddRow(2, 1, 6))

	r := newGinWithClaims(ownerClaims())
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodGet, "/tasks/1/assignees", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	assignees := responseData(t, rec)["assignees"].([]any)
	if len(assignees) != 2 {
		t.Fatalf("expected 2 assignees, got %d", len(assignees))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestAddAssigneesIsIdempotent(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	expectOwnedTask(mock, "published")
	mock.ExpectBegin()
	// uid 5: 已存在 → SELECT 命中(FirstOrCreate 不 INSERT)
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_assignees.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "user_id"}).AddRow(1, 1, 5))
	// uid 6: 不存在 → SELECT 空 → INSERT
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_assignees.`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectExec(`(?is)^INSERT INTO .task_assignees.`).
		WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectCommit()

	r := newGinWithClaims(ownerClaims())
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/assignees", map[string]any{
		"userIds": []uint64{5, 6},
	}))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestRemoveAssignee(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	expectOwnedTask(mock, "published")
	mock.ExpectBegin()
	mock.ExpectExec(`(?is)^DELETE FROM .task_assignees.`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	r := newGinWithClaims(ownerClaims())
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodDelete, "/tasks/1/assignees/5", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestRemoveAssigneeNotFound(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	expectOwnedTask(mock, "published")
	mock.ExpectBegin()
	mock.ExpectExec(`(?is)^DELETE FROM .task_assignees.`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	r := newGinWithClaims(ownerClaims())
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodDelete, "/tasks/1/assignees/5", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// ---------- ListItems (Owner 批量编辑列表) ----------

// 验证 GET /tasks/:id/items 把 task_items 映射成含 status/priority/payload 的行,
// 数量未超过 limit 时 has_more=false,external_id 为 NULL 时序列化为 null。
func TestListItemsReturnsTaskItemRows(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	expectOwnedTask(mock, "published")
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_items.+ORDER BY id ASC`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "external_id", "payload", "status", "priority"}).
			AddRow(10, 1, "Q10", `{"text":"a"}`, "available", 5).
			AddRow(11, 1, nil, `{"text":"b"}`, "claimed", 0))

	r := newGinWithClaims(ownerClaims())
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodGet, "/tasks/1/items", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	items := responseDataArray(t, rec)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d (body=%s)", len(items), rec.Body.String())
	}
	first := items[0].(map[string]any)
	if first["status"].(string) != "available" {
		t.Fatalf("item0 status = %v", first["status"])
	}
	if first["externalId"].(string) != "Q10" {
		t.Fatalf("item0 externalId = %v", first["externalId"])
	}
	if payload := first["payload"].(map[string]any); payload["text"].(string) != "a" {
		t.Fatalf("item0 payload = %v", payload)
	}
	if second := items[1].(map[string]any); second["externalId"] != nil {
		t.Fatalf("item1 externalId should be null, got %v", second["externalId"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

// 验证游标分页:?limit=2&cursor=10 → SQL 带 id > 过滤,多取一条后裁剪为 2 条,
// has_more=true 且 next_cursor 指向裁剪后最后一条 id。
func TestListItemsPaginatesWithCursor(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	expectOwnedTask(mock, "published")
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_items.+id > .+ORDER BY id ASC`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "external_id", "payload", "status", "priority"}).
			AddRow(11, 1, "Q11", `{}`, "available", 0).
			AddRow(12, 1, "Q12", `{}`, "available", 0).
			AddRow(13, 1, "Q13", `{}`, "available", 0))

	r := newGinWithClaims(ownerClaims())
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodGet, "/tasks/1/items?limit=2&cursor=10", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if items := resp["data"].([]any); len(items) != 2 {
		t.Fatalf("expected 2 items after trim, got %d (body=%s)", len(items), rec.Body.String())
	}
	page := resp["page"].(map[string]any)
	if page["has_more"].(bool) != true {
		t.Fatalf("expected has_more true, got %v", page["has_more"])
	}
	if page["next_cursor"].(string) != "12" {
		t.Fatalf("next_cursor = %v, want 12", page["next_cursor"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestListLabelerCandidatesReturnsActiveLabelers(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	expectOwnedTask(mock, "published")
	mock.ExpectQuery(`(?is)^SELECT.+FROM .users.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "display_name"}).
			AddRow(101, "labeler1", "标注员一号").
			AddRow(102, "labeler2", "标注员二号"))

	r := newGinWithClaims(ownerClaims())
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tasks/1/assignee-candidates", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	data := responseData(t, rec)
	candidates, ok := data["candidates"].([]any)
	if !ok || len(candidates) != 2 {
		t.Fatalf("candidates = %v", data["candidates"])
	}
	first := candidates[0].(map[string]any)
	if first["username"] != "labeler1" || first["userId"].(float64) != 101 || first["displayName"] != "标注员一号" {
		t.Fatalf("first candidate = %v", first)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestListReviewResultsReturnsAIVsHumanAgreement(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	expectOwnedTask(mock, "published")
	now := time.Now()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .submissions.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "item_id", "status", "ai_verdict", "ai_score", "human_verdict", "updated_at"}).
			AddRow(20, 200, "approved", "pass", 0.9, "approve", now).
			AddRow(19, 199, "rejected", "pass", 0.7, "reject", now))

	r := newGinWithClaims(ownerClaims())
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tasks/1/review-results", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	items := resp["data"].([]any)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d (body=%s)", len(items), rec.Body.String())
	}
	// AI pass + 人工 approve -> 一致
	first := items[0].(map[string]any)
	if first["aiVerdict"] != "pass" || first["humanVerdict"] != "approve" || first["agreed"] != true {
		t.Fatalf("first (pass/approve) should agree: %v", first)
	}
	// AI pass + 人工 reject -> 不一致(owner 该回看的信号)
	second := items[1].(map[string]any)
	if second["agreed"] != false {
		t.Fatalf("second (AI pass vs human reject) should disagree: %v", second)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}
