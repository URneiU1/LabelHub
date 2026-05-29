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
