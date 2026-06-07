package handler

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"labelhub-api/internal/auth"
	"labelhub-api/internal/middleware"
)

// --- 共用脚手架 ---

func newMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	gormDB, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	return gormDB, mock, sqlDB
}

func newGinWithClaims(claims *auth.Claims) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("request_id", "test-req-id")
		if claims != nil {
			c.Set(middleware.ClaimsContextKey, claims)
		}
		c.Next()
	})
	return r
}

func jsonRequest(method, path string, body any) *http.Request {
	buf, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	return req
}

// --- Test 1: ClaimItem happy path ---
//
// PLAN §4.5 "first_come 抢单":确保事务内
//  1. SELECT current-user 已 claim 的 item(找不到)
//  2. BEGIN
//  3. SELECT ... FOR UPDATE SKIP LOCKED 拿一条 available
//  4. UPDATE task_items 标 claimed_by + status='claimed'
//  5. COMMIT
//  6. SELECT template / submission / revision 给 respondItem
func TestClaimItem_HappyPath(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	claims := &auth.Claims{UserID: 7, Username: "labeler1", Roles: []string{"labeler"}}

	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "title", "status", "template_id"}).
			AddRow(1, 1, "qa_quality", "published", 101))

	// 已 claimed 的 item 查找 — ErrRecordNotFound 走"没有正在干的活"分支
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_items.+claimed_by.+FOR UPDATE`).
		WillReturnError(gorm.ErrRecordNotFound)

	// PLAN §4.5 核心:SKIP LOCKED 必须真的出现在 SQL 里
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_items.+FOR UPDATE SKIP LOCKED`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "status"}).
			AddRow(11, 1, itemStatusAvailable))
	mock.ExpectExec(`(?is)^UPDATE .task_items. SET`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// findOrCreateSubmission 先查 (item,labeler) 既有 submission(无)→ 再按冻结模板版本新建。
	mock.ExpectQuery(`(?is)^SELECT.+FROM .submissions. WHERE item_id`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_templates.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "version", "schema_json"}).
			AddRow(101, 1, 2, `{}`))
	mock.ExpectExec(`(?is)^INSERT INTO .submissions.`).
		WillReturnResult(sqlmock.NewResult(42, 1))
	mock.ExpectCommit()

	// respondItem 后续:claim 时已经创建 draft submission,按 frozen template_version 渲染。
	mock.ExpectQuery(`(?is)^SELECT.+FROM .submissions.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "item_id", "template_version", "labeler_id", "status"}).
			AddRow(42, 1, 11, 2, 7, "draft"))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_templates.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "version", "schema_json"}).
			AddRow(101, 1, 2, `{}`))

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

// --- Test 2: ReviewApprove 整事务 5 mutation + finished_items+1 ---
//
// PLAN §2 关键易忘点 + §4.3:approve 路径在同一事务内必须发生 5 件事:
//
//	INSERT human_reviews
//	UPDATE submissions(status=approved + human_verdict + approved_at)
//	UPDATE task_items(status=finished + finished_at)
//	UPDATE tasks(finished_items += 1)
//	INSERT audit_logs
func TestReviewApprove_TransactionFullSequence(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	claims := &auth.Claims{UserID: 5, Username: "reviewer1", Roles: []string{"reviewer"}}

	revisionID := uint64(901)
	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .submissions.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "item_id", "status", "current_revision_id"}).
			AddRow(42, 1, 11, "human_reviewing", revisionID))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id"}).AddRow(1, 1))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .submissions.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "item_id", "status", "current_revision_id"}).
			AddRow(42, 1, 11, "human_reviewing", revisionID))
	mock.ExpectQuery(`(?is)^SELECT count\(\*\) FROM .task_reviewers.`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	// 已有 2 条 approve → 本次是终审(第 3 次),推到 approved。
	expectReviewApproveCounts(mock, 2, 0)
	mock.ExpectExec(`(?is)^INSERT INTO .human_reviews.`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`(?is)^UPDATE .submissions. SET`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^UPDATE .task_items. SET`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// 关键:approve 必须 bump tasks.finished_items —— PLAN §2 dashboard 数字依赖这条
	mock.ExpectExec(`(?is)^UPDATE .tasks. SET.+finished_items \+ 1`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^INSERT INTO .audit_logs.`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	r := newGinWithClaims(claims)
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/submissions/42/review", map[string]any{
		"verdict": "approve",
		"reason":  "",
	}))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}

	// 解析响应,确认状态转到了 approved
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	data, _ := resp["data"].(map[string]any)
	if data["status"] != "approved" {
		t.Errorf("expected status=approved, got %v", data["status"])
	}
}

// --- Test 3: SubmitFromRevising 跨态写 2 条 audit_log ---
//
// PLAN §4.3 + §11"audit_log 覆盖所有迁移":revising → submit 路径要写 2 条 audit_log
// (revising→submitted 的 submit 事件 + submitted→human_reviewing 的 skip_ai 事件)
func TestSubmitFromRevising_WritesTwoAuditLogs(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	claims := &auth.Claims{UserID: 7, Username: "labeler1", Roles: []string{"labeler"}}

	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "status", "ai_review_enabled", "human_review_enabled", "review_sampling_pct"}).
			AddRow(1, 1, "published", false, true, 100))

	claimedBy := uint64(7)
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_items.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "claimed_by", "status"}).
			AddRow(11, 1, claimedBy, itemStatusClaimed))

	mock.ExpectBegin()

	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "status", "ai_review_enabled", "human_review_enabled", "review_sampling_pct"}).
			AddRow(1, 1, "published", false, true, 100))

	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_items.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "claimed_by", "status"}).
			AddRow(11, 1, claimedBy, itemStatusClaimed))

	// findOrCreateSubmission → 已存在,status='revising'
	mock.ExpectQuery(`(?is)^SELECT.+FROM .submissions.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "item_id", "labeler_id", "status"}).
			AddRow(42, 1, 11, 7, "revising"))

	// nextRevisionNo:MAX(revision_no)=2 → 下一条 3
	mock.ExpectQuery(`(?is)^SELECT MAX.+FROM .submission_revisions.`).
		WillReturnRows(sqlmock.NewRows([]string{"max"}).AddRow(2))

	mock.ExpectExec(`(?is)^INSERT INTO .submission_revisions.`).
		WillReturnResult(sqlmock.NewResult(903, 1))

	mock.ExpectExec(`(?is)^UPDATE .submissions. SET`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	// B4 关键断言:跨态 2 条 audit_log(revising→submitted + submitted→human_reviewing)
	mock.ExpectExec(`(?is)^INSERT INTO .audit_logs.`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`(?is)^INSERT INTO .audit_logs.`).
		WillReturnResult(sqlmock.NewResult(2, 1))

	mock.ExpectQuery(`(?is)^SELECT.+FROM .submissions.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status"}).AddRow(42, "human_reviewing"))

	mock.ExpectCommit()

	r := newGinWithClaims(claims)
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/items/11/submit", map[string]any{
		"answer": map[string]any{"summary": "ok"},
	}))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

// 简单的 regexp sanity check,避免我自己写错 — 验证 sqlmock 真用了 regexp 匹配
func TestRegexpMatcherSanity(t *testing.T) {
	want := regexp.MustCompile(`^UPDATE .tasks. SET .finished_items. = finished_items \+ 1`)
	sample := "UPDATE `tasks` SET `finished_items` = finished_items + 1 WHERE id = ?"
	if !want.MatchString(sample) {
		t.Fatalf("regexp should match GORM-style UPDATE; sample=%q", sample)
	}
}
