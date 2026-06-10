package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"labelhub-api/internal/auth"
)

// responseDataArray 解析 PageOK 返回的顶层数组 data。
func responseDataArray(t *testing.T, rec *httptest.ResponseRecorder) []any {
	t.Helper()
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	data, ok := resp["data"].([]any)
	if !ok {
		t.Fatalf("response data is %T, want array: %s", resp["data"], rec.Body.String())
	}
	return data
}

// adminClaims 复用:admin 跳过 task_reviewers 校验,简化多级审核 mock 序列。
func adminReviewerClaims() *auth.Claims {
	return &auth.Claims{UserID: 9, Username: "admin1", Roles: []string{"admin"}}
}

// expectReviewSubmissionLockSequence 铺设 review.Apply 进入事务后到 approve-count 之前的固定查询。
// admin 角色 → canReviewLocked 不查 task_reviewers。
func expectReviewSubmissionLockSequence(mock sqlmock.Sqlmock, revisionID uint64) {
	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .submissions.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "item_id", "status", "current_revision_id"}).
			AddRow(501, 1, 11, "human_reviewing", revisionID))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "status"}).AddRow(1, 7, "published"))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .submissions.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "item_id", "status", "current_revision_id"}).
			AddRow(501, 1, 11, "human_reviewing", revisionID))
}

func expectReviewApproveCounts(mock sqlmock.Sqlmock, total int64, sameReviewer int64) {
	mock.ExpectQuery(`(?is)^SELECT count\(\*\) FROM .human_reviews. WHERE .+verdict = \?`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(total))
	mock.ExpectQuery(`(?is)^SELECT count\(\*\) FROM .human_reviews. WHERE .+reviewer_id = \?.+verdict = \?`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(sameReviewer))
}

// 两级审核端到端:approve #1(初审)只 advance stage 并停留 human_reviewing 等终审。
func TestReviewSubmission_IntermediateApproveStaysHumanReviewing(t *testing.T) {
	cases := []struct {
		name          string
		existingCount int
		wantStage     string
	}{
		{"first approve (初审) stays human_reviewing", 0, "first"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, sqlDB := newMockDB(t)
			defer sqlDB.Close()
			revisionID := uint64(901)

			expectReviewSubmissionLockSequence(mock, revisionID)
			expectReviewApproveCounts(mock, int64(tc.existingCount), 0)
			mock.ExpectExec(`(?is)^INSERT INTO .human_reviews.`).
				WillReturnResult(sqlmock.NewResult(31, 1))
			// 中间级:只 UPDATE updated_at,不动 task_items / tasks。
			mock.ExpectExec(`(?is)^UPDATE .submissions. SET`).
				WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec(`(?is)^INSERT INTO .audit_logs.`).
				WillReturnResult(sqlmock.NewResult(41, 1))
			mock.ExpectCommit()

			r := newGinWithClaims(adminReviewerClaims())
			registerAllHandlers(r, db)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/submissions/501/review", map[string]any{
				"verdict": "approve",
			}))

			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
			}
			data := responseData(t, rec)
			if data["status"] != "human_reviewing" {
				t.Fatalf("status = %v, want human_reviewing", data["status"])
			}
			if data["stage"] != tc.wantStage {
				t.Fatalf("stage = %v, want %s", data["stage"], tc.wantStage)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("expectations not met: %v", err)
			}
		})
	}
}

// 两级审核端到端:approve #2(终审)推到 approved 并 finish item + bump finished_items。
func TestReviewSubmission_FinalApproveGoesApproved(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	revisionID := uint64(901)

	expectReviewSubmissionLockSequence(mock, revisionID)
	expectReviewApproveCounts(mock, 1, 0) // 已有 1 次(初审)→ 本次终审
	mock.ExpectExec(`(?is)^INSERT INTO .human_reviews.`).WillReturnResult(sqlmock.NewResult(31, 1))
	mock.ExpectExec(`(?is)^UPDATE .submissions. SET`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^UPDATE .task_items. SET`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^UPDATE .tasks. SET.+finished_items \+ 1`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^INSERT INTO .audit_logs.`).WillReturnResult(sqlmock.NewResult(41, 1))
	mock.ExpectCommit()

	r := newGinWithClaims(adminReviewerClaims())
	registerAllHandlers(r, db)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/submissions/501/review", map[string]any{
		"verdict": "approve",
	}))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	data := responseData(t, rec)
	if data["status"] != "approved" || data["stage"] != "final" {
		t.Fatalf("status/stage = %v/%v, want approved/final", data["status"], data["stage"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

// reject 在终审阶段(已有 1 条 approve)直接落 rejected。
func TestReviewSubmission_RejectAtFinalStageGoesRejected(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	revisionID := uint64(901)

	expectReviewSubmissionLockSequence(mock, revisionID)
	mock.ExpectQuery(`(?is)^SELECT count\(\*\) FROM .human_reviews.`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectExec(`(?is)^INSERT INTO .human_reviews.`).WillReturnResult(sqlmock.NewResult(31, 1))
	mock.ExpectExec(`(?is)^UPDATE .submissions. SET`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^UPDATE .task_items. SET`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^UPDATE .tasks. SET.+finished_items \+ 1`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^INSERT INTO .audit_logs.`).WillReturnResult(sqlmock.NewResult(41, 1))
	mock.ExpectCommit()

	r := newGinWithClaims(adminReviewerClaims())
	registerAllHandlers(r, db)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/submissions/501/review", map[string]any{
		"verdict": "reject",
		"reason":  "second-stage reject reason",
	}))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	data := responseData(t, rec)
	if data["status"] != "rejected" || data["stage"] != "final" {
		t.Fatalf("status/stage = %v/%v, want rejected/final", data["status"], data["stage"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

// ReviewerQueue 每条 submission 暴露当前 stage(初审/复审/终审)。
func TestReviewerQueue_ExposesReviewStage(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	rev501 := uint64(901)
	rev502 := uint64(902)

	mock.ExpectQuery(`(?is)^SELECT.+FROM .submissions.+LEFT JOIN task_reviewers`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "item_id", "status", "current_revision_id"}).
			AddRow(501, 1, 11, "human_reviewing", rev501).
			AddRow(502, 1, 12, "human_reviewing", rev502))
	// rev501 已有 1 条 approve → final(终审);rev502 无 approve → first(grouped 查询不返回 0 行)。
	mock.ExpectQuery(`(?is)^SELECT revision_id, COUNT\(\*\) AS total FROM .human_reviews.`).
		WillReturnRows(sqlmock.NewRows([]string{"revision_id", "total"}).AddRow(rev501, 1))

	r := newGinWithClaims(&auth.Claims{UserID: 5, Username: "reviewer1", Roles: []string{"reviewer"}})
	registerAllHandlers(r, db)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/reviewer/submissions", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	items := responseDataArray(t, rec)
	first := items[0].(map[string]any)
	if first["reviewStage"] != "final" || first["reviewLevel"] != float64(2) || first["requiredLevels"] != float64(2) {
		t.Fatalf("item0 stage = %v/%v/%v", first["reviewStage"], first["reviewLevel"], first["requiredLevels"])
	}
	second := items[1].(map[string]any)
	if second["reviewStage"] != "first" || second["reviewLevel"] != float64(1) {
		t.Fatalf("item1 stage = %v/%v", second["reviewStage"], second["reviewLevel"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestReviewerQueue_ExposesNeedsArbitration(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	revisionID := uint64(901)

	mock.ExpectQuery(`(?is)^SELECT.+FROM .submissions.+LEFT JOIN task_reviewers`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "item_id", "status", "current_revision_id"}).
			AddRow(501, 1, 11, "needs_arbitration", revisionID))
	mock.ExpectQuery(`(?is)^SELECT revision_id, COUNT\(\*\) AS total FROM .human_reviews.`).
		WillReturnRows(sqlmock.NewRows([]string{"revision_id", "total"}))

	r := newGinWithClaims(&auth.Claims{UserID: 5, Username: "reviewer1", Roles: []string{"reviewer"}})
	registerAllHandlers(r, db)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/reviewer/submissions?status=needs_arbitration", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	items := responseDataArray(t, rec)
	if len(items) != 1 || items[0].(map[string]any)["status"] != "needs_arbitration" {
		t.Fatalf("items = %v", items)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

// ReviewerQueue 用 ?status=manual_review 返回「转人工复核(AI 可疑)」分区的项。
func TestReviewerQueue_ExposesManualReview(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	revisionID := uint64(901)

	mock.ExpectQuery(`(?is)^SELECT.+FROM .submissions.+LEFT JOIN task_reviewers`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "item_id", "status", "current_revision_id"}).
			AddRow(503, 1, 13, "manual_review", revisionID))
	mock.ExpectQuery(`(?is)^SELECT revision_id, COUNT\(\*\) AS total FROM .human_reviews.`).
		WillReturnRows(sqlmock.NewRows([]string{"revision_id", "total"}))

	r := newGinWithClaims(&auth.Claims{UserID: 5, Username: "reviewer1", Roles: []string{"reviewer"}})
	registerAllHandlers(r, db)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/reviewer/submissions?status=manual_review", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	items := responseDataArray(t, rec)
	if len(items) != 1 || items[0].(map[string]any)["status"] != "manual_review" {
		t.Fatalf("items = %v", items)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

// /reviewer/results 列出已定稿 submission,带 finalVerdict / reviewerId / aiScore。
func TestReviewerResults_ListsFinalizedSubmissions(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	now := time.Date(2026, 5, 29, 9, 0, 0, 0, time.UTC)
	score := 88.0

	mock.ExpectQuery(`(?is)^SELECT.+FROM .submissions.+LEFT JOIN task_reviewers.+status IN`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "item_id", "status", "human_verdict", "ai_score", "updated_at"}).
			AddRow(501, 1, 11, "approved", "approve", score, now).
			AddRow(502, 1, 12, "rejected", "reject", nil, now))
	// finalReviewerBySubmission 一次性拉回所有 human_reviews。
	mock.ExpectQuery(`(?is)^SELECT.+FROM .human_reviews. WHERE submission_id IN`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "submission_id", "revision_id", "reviewer_id", "verdict", "created_at"}).
			AddRow(81, 501, 901, 5, "approve", now).
			AddRow(82, 502, 902, 6, "reject", now))

	r := newGinWithClaims(&auth.Claims{UserID: 5, Username: "reviewer1", Roles: []string{"reviewer"}})
	registerAllHandlers(r, db)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/reviewer/results", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	items := responseDataArray(t, rec)
	if len(items) != 2 {
		t.Fatalf("expected 2 results, got %d", len(items))
	}
	first := items[0].(map[string]any)
	if first["status"] != "approved" || first["finalVerdict"] != "approve" {
		t.Fatalf("first result = %v", first)
	}
	if first["reviewerId"] != float64(5) {
		t.Fatalf("first reviewerId = %v, want 5", first["reviewerId"])
	}
	if first["aiScore"] != score {
		t.Fatalf("first aiScore = %v, want %v", first["aiScore"], score)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

// /reviewer/results 拒绝非 reviewer/admin 角色。
func TestReviewerResults_RejectsOwnerRole(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner1", Roles: []string{"owner"}})
	registerAllHandlers(r, db)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/reviewer/results", nil))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}
