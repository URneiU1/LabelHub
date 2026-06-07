package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/gorm"

	"labelhub-api/internal/auth"
)

// HIGH 1 回归:labeler 对 draft / paused / archived task 不能领新题。
// 已经在 claim 中的 item 走 Path A,这里只验证 Path B 的边界。
func TestClaimItem_BlockedOnNonPublishedTask(t *testing.T) {
	for _, status := range []string{"draft", "paused", "archived"} {
		t.Run(status, func(t *testing.T) {
			db, mock, sqlDB := newMockDB(t)
			defer sqlDB.Close()

			claims := &auth.Claims{UserID: 7, Username: "labeler1", Roles: []string{"labeler"}}

			mock.ExpectBegin()
			mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.+FOR UPDATE`).
				WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "status"}).
					AddRow(1, 1, status))

			// Path A 未命中(此 labeler 没在干这个 task 的 item)
			mock.ExpectQuery(`(?is)^SELECT.+FROM .task_items.+claimed_by.+FOR UPDATE`).
				WillReturnError(gorm.ErrRecordNotFound)
			mock.ExpectRollback()

			r := newGinWithClaims(claims)
			registerAllHandlers(r, db)

			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/claim", map[string]any{}))

			if rec.Code != http.StatusConflict {
				t.Fatalf("status=%s: expected 409, got %d, body=%s", status, rec.Code, rec.Body.String())
			}
			var resp map[string]any
			_ = json.Unmarshal(rec.Body.Bytes(), &resp)
			errObj, _ := resp["error"].(map[string]any)
			if errObj["code"] != "CONFLICT" {
				t.Errorf("status=%s: expected CONFLICT, got %v", status, errObj["code"])
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("status=%s: unexpected DB calls: %v", status, err)
			}
		})
	}
}

// HIGH 1 配套:即使 task 已经 paused,labeler 手上 in-flight 的 item 仍能 resume。
// 这是反向边界 —— policy 不能误伤已经在干的活,否则会把 labeler 卡死。
func TestClaimItem_ResumeWorksOnPausedTask(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	claims := &auth.Claims{UserID: 7, Username: "labeler1", Roles: []string{"labeler"}}

	// task 已经 paused
	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "status"}).
			AddRow(1, 1, "paused"))

	// Path A 命中:该 labeler 在这个 paused task 上有正在干的 item
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_items.+claimed_by.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "claimed_by", "status"}).
			AddRow(11, 1, 7, itemStatusClaimed))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .submissions.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "item_id", "template_version", "labeler_id", "status", "current_revision_id"}).
			AddRow(501, 1, 11, 1, 7, "draft", nil))
	mock.ExpectCommit()

	// respondItem 后续查询:已有 claim 必须已有 draft submission。
	mock.ExpectQuery(`(?is)^SELECT.+FROM .submissions.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "item_id", "template_version", "labeler_id", "status", "current_revision_id"}).
			AddRow(501, 1, 11, 1, 7, "draft", nil))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_templates.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "version", "schema_json"}).
			AddRow(101, 1, 1, `{}`))

	r := newGinWithClaims(claims)
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/claim", map[string]any{}))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestRespondItem_ExistingSubmissionUsesTemplateVersion(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	claims := &auth.Claims{UserID: 7, Username: "labeler1", Roles: []string{"labeler"}}

	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "status"}).
			AddRow(1, 1, "published"))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_items.+claimed_by.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "claimed_by", "status"}).
			AddRow(11, 1, 7, itemStatusClaimed))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .submissions.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "item_id", "template_version", "labeler_id", "status", "current_revision_id"}).
			AddRow(501, 1, 11, 1, 7, "human_reviewing", nil))
	mock.ExpectCommit()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .submissions.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "item_id", "template_version", "labeler_id", "status", "current_revision_id"}).
			AddRow(501, 1, 11, 1, 7, "human_reviewing", nil))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_templates. WHERE .*task_id.*AND.*version`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "version", "schema_json"}).
			AddRow(101, 1, 1, `{"title":"v1","fields":[{"name":"old","widget":"Input"}]}`))

	r := newGinWithClaims(claims)
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/claim", map[string]any{}))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	data := responseData(t, rec)
	template := data["template"].(map[string]any)
	if template["version"] != float64(1) {
		t.Fatalf("template version = %v, want historical version 1", template["version"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

// HIGH 2 回归:labeler A 不能通过 GetItem 偷看 labeler B 已 claim 的 item raw payload。
// 数据资产隔离的核心断言。
func TestGetItem_LabelerCannotReadPeerClaimedItem(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	claims := &auth.Claims{UserID: 7, Username: "labeler1", Roles: []string{"labeler"}}

	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "status"}).
			AddRow(1, 99, "published"))

	// item 被 labeler 8 claim 了,labeler 7 想读
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_items.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "claimed_by", "status"}).
			AddRow(11, 1, 8, itemStatusClaimed))

	// GetItem 内部会查 submission(可能没有)
	mock.ExpectQuery(`(?is)^SELECT.+FROM .submissions.`).
		WillReturnError(gorm.ErrRecordNotFound)

	r := newGinWithClaims(claims)
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tasks/1/items/11", nil))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d, body=%s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	errObj, _ := resp["error"].(map[string]any)
	if errObj["code"] != "FORBIDDEN" {
		t.Errorf("expected FORBIDDEN, got %v", errObj["code"])
	}
}

func TestGetItem_OwnerBlockedFromLabelerModule(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	r := newGinWithClaims(&auth.Claims{UserID: 10, Username: "owner1", Roles: []string{"owner"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tasks/1/items/11", nil))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

// HIGH 2 配套:reviewer 在没有 submission 的 item 上拿不到 raw payload。
func TestGetItem_ReviewerBlockedWithoutSubmission(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	claims := &auth.Claims{UserID: 5, Username: "reviewer1", Roles: []string{"reviewer"}}

	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "status"}).
			AddRow(1, 99, "published"))

	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_items.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "claimed_by", "status"}).
			AddRow(11, 1, nil, "available"))

	mock.ExpectQuery(`(?is)^SELECT.+FROM .submissions.`).
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectQuery(`(?is)^SELECT count\(\*\) FROM .task_reviewers.`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	r := newGinWithClaims(claims)
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tasks/1/items/11", nil))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestReviewerQueueRejectsOwnerRole(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner1", Roles: []string{"owner"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/reviewer/submissions", nil))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestReviewerDetailIncludesAIReviewAndAuditLogs(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	now := time.Date(2026, 5, 26, 13, 0, 0, 0, time.UTC)
	revisionID := uint64(901)
	verdict := "pass"
	score := 92.5

	mock.ExpectQuery(`(?is)^SELECT.+FROM .submissions.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "item_id", "template_version", "labeler_id", "status", "current_revision_id", "ai_verdict", "ai_score"}).
			AddRow(501, 1, 11, 3, 8, "human_reviewing", revisionID, verdict, score))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status"}).
			AddRow(1, 7, "商品标题清洗", "published"))
	mock.ExpectQuery(`(?is)^SELECT count\(\*\) FROM .task_reviewers.`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_items.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "payload", "status"}).
			AddRow(11, 1, `{"title":"raw"}`, "finished"))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_templates.+version`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "version", "schema_json"}).
			AddRow(101, 1, 3, `{"title":"v3","fields":[]}`))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .submission_revisions.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "submission_id", "revision_no", "answer", "draft", "created_by"}).
			AddRow(revisionID, 501, 1, `{"cleaned_title":"ok"}`, false, 8))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .ai_reviews.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "submission_id", "revision_id", "idempotency_key", "prompt_version", "verdict", "overall_score", "dimensions", "reason", "raw_response", "tokens_input", "tokens_output", "latency_ms", "status", "retry_count", "created_at"}).
			AddRow(31, 501, revisionID, "abc", 2, verdict, score, `[{"name":"相关性","score":92}]`, "looks good", `{"ok":true}`, 100, 20, 1420, "succeeded", 1, now))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .ai_prompt_configs.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "version", "prompt_template", "dimensions", "pass_threshold", "uncertain_min", "model"}).
			AddRow(41, 1, 2, "score this", `[{"name":"相关性","weight":1}]`, 80, 60, "doubao-pro-32k"))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .audit_logs.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "entity_type", "entity_id", "from_state", "to_state", "actor_type", "event", "payload", "created_at"}).
			AddRow(71, "submission", 501, "ai_reviewing", "human_reviewing", "ai_worker", "ai_done", `{"score":92.5}`, now))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .human_reviews.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "submission_id", "revision_id", "reviewer_id", "stage", "verdict", "reason", "created_at"}).
			AddRow(81, 501, revisionID, 7, "first", "revise", "上一轮意见", now))
	// 派生 reviewStage 的 approve 计数(当前 revision 已有 1 条 approve → 复审/second)。
	mock.ExpectQuery(`(?is)^SELECT revision_id, COUNT\(\*\) AS total FROM .human_reviews.`).
		WillReturnRows(sqlmock.NewRows([]string{"revision_id", "total"}).AddRow(revisionID, 1))

	r := newGinWithClaims(&auth.Claims{UserID: 5, Username: "reviewer1", Roles: []string{"reviewer"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/reviewer/submissions/501", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	data := responseData(t, rec)
	if data["reviewStage"] != "final" || data["reviewLevel"] != float64(2) || data["requiredLevels"] != float64(2) {
		t.Fatalf("reviewStage/level/required = %v/%v/%v", data["reviewStage"], data["reviewLevel"], data["requiredLevels"])
	}
	aiReview := data["aiReview"].(map[string]any)
	if aiReview["reason"] != "looks good" {
		t.Fatalf("aiReview.reason = %v", aiReview["reason"])
	}
	dimensions := aiReview["dimensions"].([]any)
	if dimensions[0].(map[string]any)["name"] != "相关性" {
		t.Fatalf("aiReview.dimensions = %v", dimensions)
	}
	prompt := aiReview["prompt"].(map[string]any)
	if prompt["promptTemplate"] != "score this" {
		t.Fatalf("promptTemplate = %v", prompt["promptTemplate"])
	}
	auditLogs := data["auditLogs"].([]any)
	if auditLogs[0].(map[string]any)["event"] != "ai_done" {
		t.Fatalf("auditLogs = %v", auditLogs)
	}
	latestHumanReview := data["latestHumanReview"].(map[string]any)
	if latestHumanReview["reason"] != "上一轮意见" {
		t.Fatalf("latestHumanReview = %v", latestHumanReview)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestReviewerAIPromptsScopedToAssignedReviewer(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	promptID := uint64(41)
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status", "ai_review_enabled", "ai_prompt_id"}).
			AddRow(1, 99, "商品标题清洗", "published", true, promptID))
	mock.ExpectQuery(`(?is)^SELECT count\(\*\) FROM .task_reviewers.`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .ai_prompt_configs.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "version", "prompt_template", "dimensions", "pass_threshold", "uncertain_min", "model"}).
			AddRow(promptID, 1, 2, "score this", `[{"name":"相关性","weight":1}]`, 80, 60, "mock-model"))

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "reviewer1", Roles: []string{"reviewer"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/reviewer/tasks/1/ai-prompts", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	data := responseData(t, rec)
	if data["activePromptId"] != float64(promptID) {
		t.Fatalf("activePromptId = %v", data["activePromptId"])
	}
	prompts := data["prompts"].([]any)
	prompt := prompts[0].(map[string]any)
	if prompt["promptTemplate"] != "score this" {
		t.Fatalf("promptTemplate = %v", prompt["promptTemplate"])
	}
	dimensions := prompt["dimensions"].([]any)
	if dimensions[0].(map[string]any)["name"] != "相关性" {
		t.Fatalf("dimensions = %v", dimensions)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestReviewerAIPromptsRejectsUnassignedReviewer(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status", "ai_review_enabled", "ai_prompt_id"}).
			AddRow(1, 99, "商品标题清洗", "published", true, uint64(41)))
	mock.ExpectQuery(`(?is)^SELECT count\(\*\) FROM .task_reviewers.`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "reviewer1", Roles: []string{"reviewer"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/reviewer/tasks/1/ai-prompts", nil))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestReviewerAIPromptActivateRouteIsNotRegistered(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "reviewer1", Roles: []string{"reviewer"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/reviewer/tasks/1/ai-prompts/41/activate", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestRetryAIReviewRequeuesFailedReview(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	revisionID := uint64(901)
	key := reviewerAIReviewIdempotencyKey(501, revisionID, 41, 2)

	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .submissions.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "item_id", "template_version", "labeler_id", "status", "current_revision_id", "ai_verdict", "ai_score"}).
			AddRow(501, 1, 11, 3, 8, "human_reviewing", revisionID, "uncertain", nil))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status"}).
			AddRow(1, 7, "商品标题清洗", "published"))
	mock.ExpectQuery(`(?is)^SELECT count\(\*\) FROM .task_reviewers.`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .ai_reviews.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "submission_id", "revision_id", "idempotency_key", "prompt_version", "status", "retry_count"}).
			AddRow(31, 501, revisionID, key, 2, "dead", 5))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .ai_prompt_configs.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "version", "prompt_template", "dimensions", "pass_threshold", "uncertain_min", "model"}).
			AddRow(41, 1, 2, "score this", `[{"name":"相关性","weight":1}]`, 80, 60, "mock-model"))
	mock.ExpectExec(`(?is)^UPDATE .ai_reviews. SET .+WHERE id = .+status IN`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^UPDATE .submissions. SET .+WHERE id = .+status = .+current_revision_id`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^INSERT INTO .outbox_events.`).
		WillReturnResult(sqlmock.NewResult(81, 1))
	mock.ExpectExec(`(?is)^INSERT INTO .audit_logs.`).
		WillReturnResult(sqlmock.NewResult(91, 1))
	mock.ExpectCommit()

	r := newGinWithClaims(&auth.Claims{UserID: 5, Username: "reviewer1", Roles: []string{"reviewer"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/reviewer/submissions/501/ai-review/retry", map[string]any{}))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	data := responseData(t, rec)
	if data["status"] != "ai_reviewing" {
		t.Fatalf("status = %v", data["status"])
	}
	aiReview := data["aiReview"].(map[string]any)
	if aiReview["status"] != "pending" {
		t.Fatalf("aiReview.status = %v", aiReview["status"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestReviewSubmissionOwnerCannotReviewOtherTask(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner1", Roles: []string{"owner"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/submissions/501/review", map[string]any{
		"verdict": "approve",
	}))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestBatchReviewAppliesApproveForSelectedSubmissions(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	revisionID := uint64(901)
	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .submissions.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "item_id", "status", "current_revision_id"}).
			AddRow(501, 1, 11, "human_reviewing", revisionID))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id"}).AddRow(1, 99))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .submissions.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "item_id", "status", "current_revision_id"}).
			AddRow(501, 1, 11, "human_reviewing", revisionID))
	mock.ExpectQuery(`(?is)^SELECT count\(\*\) FROM .task_reviewers.`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	// 已有 2 条 approve → batch approve 这一条触发终审,落到 approved。
	expectReviewApproveCounts(mock, 2, 0)
	mock.ExpectExec(`(?is)^INSERT INTO .human_reviews.`).
		WillReturnResult(sqlmock.NewResult(71, 1))
	mock.ExpectExec(`(?is)^UPDATE .submissions.`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^UPDATE .task_items.`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^UPDATE .tasks. SET .finished_items.`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^INSERT INTO .audit_logs.`).
		WillReturnResult(sqlmock.NewResult(81, 1))
	mock.ExpectCommit()

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "reviewer1", Roles: []string{"reviewer"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/reviews/batch", map[string]any{
		"submission_ids": []uint64{501},
		"verdict":        "approve",
		"reason":         "looks good",
	}))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	data := responseData(t, rec)
	summary := data["summary"].(map[string]any)
	if summary["succeeded"] != float64(1) || summary["failed"] != float64(0) {
		t.Fatalf("unexpected summary: %v", summary)
	}
	results := data["results"].([]any)
	first := results[0].(map[string]any)
	if first["submissionId"] != float64(501) || first["status"] != "approved" {
		t.Fatalf("unexpected first result: %v", first)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

// MEDIUM 3 回归:同样的 payload 两次 hash 必须一样(否则会建出重复 item)。
func TestPayloadHashExternalID_DeterministicSameInput(t *testing.T) {
	raw := []byte(`{"prompt":"什么是光合作用","model_answer":"植物利用阳光合成有机物"}`)
	h1 := payloadHashExternalID(raw)
	h2 := payloadHashExternalID(raw)
	if h1 != h2 {
		t.Errorf("hash mismatch on identical input: %s vs %s", h1, h2)
	}
	if h1[:5] != "hash:" {
		t.Errorf("expected hash: prefix, got %q", h1)
	}
	// "hash:" + 32 hex chars = 37
	if len(h1) != 37 {
		t.Errorf("expected length 37, got %d (%q)", len(h1), h1)
	}
}

// MEDIUM 3 配套:不同 payload 必须产出不同 hash(碰撞会让两条不同数据被 dedup 成一条)。
func TestPayloadHashExternalID_DifferentInputDifferentHash(t *testing.T) {
	h1 := payloadHashExternalID([]byte(`{"a":1}`))
	h2 := payloadHashExternalID([]byte(`{"a":2}`))
	if h1 == h2 {
		t.Fatalf("expected different hashes, both = %s", h1)
	}
}

// LOW 5 回归:respondItem 遇到非 NotFound 的 DB 错误必须 500,不能静默返回空模板。
func TestRespondItem_TemplateDBErrorReturns500(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	claims := &auth.Claims{UserID: 7, Username: "labeler1", Roles: []string{"labeler"}}

	// ClaimItem 走 Path A 命中
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "status"}).
			AddRow(1, 1, "published"))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_items.+claimed_by`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "claimed_by", "status"}).
			AddRow(11, 1, 7, itemStatusClaimed))

	// respondItem:submission 未命中后,template 查询返回真 DB 错误(不是 NotFound)
	mock.ExpectQuery(`(?is)^SELECT.+FROM .submissions.`).
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_templates.`).
		WillReturnError(gorm.ErrInvalidDB)

	r := newGinWithClaims(claims)
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/claim", map[string]any{}))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestSubmitItemRejectsOversizedAnswerBody(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	claims := &auth.Claims{UserID: 7, Username: "labeler1", Roles: []string{"labeler"}}
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "status"}).
			AddRow(1, 1, "published"))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_items.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "claimed_by", "status"}).
			AddRow(11, 1, 7, itemStatusClaimed))

	r := newGinWithClaims(claims)
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/items/11/submit", map[string]any{
		"answer": map[string]any{"blob": strings.Repeat("x", int(maxAnswerJSONBytes)+1)},
	}))

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestSubmitItemRechecksClaimOwnershipInsideTransaction(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	claims := &auth.Claims{UserID: 7, Username: "labeler1", Roles: []string{"labeler"}}
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "status"}).
			AddRow(1, 1, "published"))
	claimedBy := uint64(7)
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_items.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "claimed_by", "status"}).
			AddRow(11, 1, claimedBy, itemStatusClaimed))
	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "status"}).
			AddRow(1, 1, "published"))
	reassignedTo := uint64(8)
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_items.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "claimed_by", "status"}).
			AddRow(11, 1, reassignedTo, itemStatusClaimed))
	mock.ExpectRollback()

	r := newGinWithClaims(claims)
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/items/11/submit", map[string]any{
		"answer": map[string]any{"summary": "ok"},
	}))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestSubmitItemRejectsEnabledAIWithInvalidActivePrompt(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	claims := &auth.Claims{UserID: 7, Username: "labeler1", Roles: []string{"labeler"}}
	promptID := uint64(33)
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "status", "ai_review_enabled", "ai_prompt_id"}).
			AddRow(1, 1, "published", true, promptID))
	claimedBy := uint64(7)
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_items.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "claimed_by", "status"}).
			AddRow(11, 1, claimedBy, itemStatusClaimed))
	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "status", "ai_review_enabled", "ai_prompt_id"}).
			AddRow(1, 1, "published", true, promptID))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_items.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "claimed_by", "status"}).
			AddRow(11, 1, claimedBy, itemStatusClaimed))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .submissions.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "item_id", "labeler_id", "status", "template_version"}).
			AddRow(42, 1, 11, claimedBy, "draft", 1))
	mock.ExpectQuery(`(?is)^SELECT MAX.+FROM .submission_revisions.`).
		WillReturnRows(sqlmock.NewRows([]string{"max"}).AddRow(0))
	mock.ExpectExec(`(?is)^INSERT INTO .submission_revisions.`).
		WillReturnResult(sqlmock.NewResult(901, 1))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .ai_prompt_configs.`).
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectRollback()

	r := newGinWithClaims(claims)
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/items/11/submit", map[string]any{
		"answer": map[string]any{"summary": "ok"},
	}))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "VALIDATION_ERROR") {
		t.Fatalf("expected VALIDATION_ERROR, body=%s", rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}
