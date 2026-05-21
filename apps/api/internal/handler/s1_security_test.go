package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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

			mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
				WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "status"}).
					AddRow(1, 1, status))

			// Path A 未命中(此 labeler 没在干这个 task 的 item)
			mock.ExpectQuery(`(?is)^SELECT.+FROM .task_items.+claimed_by`).
				WillReturnError(gorm.ErrRecordNotFound)

			r := newGinWithClaims(claims)
			NewS1Handler(db).Register(r)

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
			// 关键:status=paused/archived 时禁止"新"领取,但事务不应该开。验证 ExpectBegin 没出现。
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
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "status"}).
			AddRow(1, 1, "paused"))

	// Path A 命中:该 labeler 在这个 paused task 上有正在干的 item
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_items.+claimed_by`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "claimed_by", "status"}).
			AddRow(11, 1, 7, itemStatusClaimed))

	// respondItem 后续查询
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_templates.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "version", "schema_json"}).
			AddRow(101, 1, 1, `{}`))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .submissions.`).
		WillReturnError(gorm.ErrRecordNotFound)

	r := newGinWithClaims(claims)
	NewS1Handler(db).Register(r)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/claim", map[string]any{}))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
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
	NewS1Handler(db).Register(r)

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

	r := newGinWithClaims(claims)
	NewS1Handler(db).Register(r)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tasks/1/items/11", nil))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d, body=%s", rec.Code, rec.Body.String())
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

	// respondItem:template 查询返回真 DB 错误(不是 NotFound)
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_templates.`).
		WillReturnError(gorm.ErrInvalidDB)

	r := newGinWithClaims(claims)
	NewS1Handler(db).Register(r)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequest(http.MethodPost, "/tasks/1/claim", map[string]any{}))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d, body=%s", rec.Code, rec.Body.String())
	}
}
