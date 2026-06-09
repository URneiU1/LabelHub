package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"labelhub-api/internal/auth"
)

// AIReviewQueue 列出 AI 预审 + 关联的提交/任务/Prompt 上下文,供「AI 审核队列」只读视图。
func TestAIReviewQueueListsWithContext(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectQuery(`(?is)^SELECT.+FROM .ai_reviews.+ORDER BY id DESC`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "submission_id", "prompt_config_id", "idempotency_key", "prompt_version",
			"verdict", "overall_score", "dimensions", "status", "retry_count",
			"tokens_input", "tokens_output", "latency_ms",
		}).AddRow(
			5, 10, 3, "abc", 12,
			"reject", 62.0, `[{"name":"相关性","score":78,"reason":"x"}]`, "succeeded", 1,
			1342, 200, 1420,
		))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .submissions.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "item_id"}).AddRow(10, 1, 11))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "title"}).AddRow(1, "QA 任务"))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .ai_prompt_configs.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "model", "prompt_template", "pass_threshold", "uncertain_min"}).
			AddRow(3, "doubao-pro-32k", "请审核商品标题", 80.0, 60.0))

	r := newGinWithClaims(&auth.Claims{UserID: 9, Username: "reviewer1", Roles: []string{"reviewer"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodGet, "/reviewer/ai-reviews", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	data := responseData(t, rec)
	items, _ := data["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d (body=%s)", len(items), rec.Body.String())
	}
	row := items[0].(map[string]any)
	if row["id"].(float64) != 5 {
		t.Fatalf("id = %v, want 5", row["id"])
	}
	if row["status"] != "succeeded" {
		t.Fatalf("status = %v, want succeeded", row["status"])
	}
	if row["verdict"] != "reject" {
		t.Fatalf("verdict = %v, want reject", row["verdict"])
	}
	if row["overallScore"].(float64) != 62 {
		t.Fatalf("overallScore = %v, want 62", row["overallScore"])
	}
	if row["model"] != "doubao-pro-32k" {
		t.Fatalf("model = %v, want doubao-pro-32k", row["model"])
	}
	if row["promptTemplate"] != "请审核商品标题" {
		t.Fatalf("promptTemplate = %v", row["promptTemplate"])
	}
	if row["taskId"].(float64) != 1 || row["itemId"].(float64) != 11 {
		t.Fatalf("task/item context wrong: taskId=%v itemId=%v", row["taskId"], row["itemId"])
	}
	if row["taskTitle"] != "QA 任务" {
		t.Fatalf("taskTitle = %v", row["taskTitle"])
	}
	dims, _ := row["dimensions"].([]any)
	if len(dims) != 1 {
		t.Fatalf("dimensions = %v, want 1 scored dim", row["dimensions"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
