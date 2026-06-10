package review

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"labelhub-api/internal/statemachine"
)

func newReviewMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, *sql.DB) {
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

func expectApproveCounts(mock sqlmock.Sqlmock, total int64, sameReviewer int64) {
	mock.ExpectQuery(`(?is)^SELECT count\(\*\) FROM .human_reviews. WHERE .+verdict = \?`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(total))
	mock.ExpectQuery(`(?is)^SELECT count\(\*\) FROM .human_reviews. WHERE .+reviewer_id = \?.+verdict = \?`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(sameReviewer))
}

// Decode:reviewer 的 verdict 字符串映射到状态机 event + 目标态 + human_verdict 字段。
// 单/复数严格匹配:"approved" 不等于 "approve"。
func TestDecodeMapping(t *testing.T) {
	tests := []struct {
		verdict     string
		wantEvent   string
		wantTo      string
		wantVerdict string
		wantOK      bool
	}{
		{"approve", statemachine.EventApprove, statemachine.StateApproved, "approve", true},
		{"reject", statemachine.EventReject, statemachine.StateRejected, "reject", true},
		{"revise", statemachine.EventRevise, statemachine.StateRevising, "revise", true},
		{"approved", "", "", "", false},
		{"", "", "", "", false},
		{"YES", "", "", "", false},
	}

	for _, tt := range tests {
		event, to, verdict, ok := Decode(tt.verdict)
		if ok != tt.wantOK || event != tt.wantEvent || to != tt.wantTo || verdict != tt.wantVerdict {
			t.Errorf("Decode(%q) = (%q, %q, %q, %v); want (%q, %q, %q, %v)",
				tt.verdict, event, to, verdict, ok, tt.wantEvent, tt.wantTo, tt.wantVerdict, tt.wantOK)
		}
	}
}

// UpdatesFor:approve 必须同时写 status=approved + approved_at;reject/revise 不写 approved_at。
// 防止 dashboard "approved_at 排序" 拿到 NULL 时序混进 reject/revise 行。
func TestUpdatesForApprovedAtSemantics(t *testing.T) {
	now := time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC)

	approve := UpdatesFor(statemachine.StateApproved, "approve", now)
	if approve["status"] != statemachine.StateApproved || approve["human_verdict"] != "approve" {
		t.Errorf("approve missing status/verdict: %+v", approve)
	}
	if approve["approved_at"] != now {
		t.Errorf("approve must stamp approved_at, got %v", approve["approved_at"])
	}

	reject := UpdatesFor(statemachine.StateRejected, "reject", now)
	if _, present := reject["approved_at"]; present {
		t.Errorf("reject must NOT touch approved_at, got %+v", reject)
	}

	revise := UpdatesFor(statemachine.StateRevising, "revise", now)
	if _, present := revise["approved_at"]; present {
		t.Errorf("revise must NOT touch approved_at, got %+v", revise)
	}
}

// 终审 approve(已存在 2 条 approve)推到 approved 并 finish item。
func TestApplyFinalApproveWritesHumanReviewAndFinishesItem(t *testing.T) {
	db, mock, sqlDB := newReviewMockDB(t)
	defer sqlDB.Close()
	now := time.Date(2026, 5, 28, 1, 0, 0, 0, time.UTC)
	oldNow := NowUTC
	NowUTC = func() time.Time { return now }
	defer func() { NowUTC = oldNow }()

	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT .+FROM .submissions. WHERE .submissions.\..id. = .+ORDER BY .submissions.\..id. LIMIT`).
		WillReturnRows(reviewSubmissionRows("human_reviewing"))
	mock.ExpectQuery(`(?is)^SELECT .+FROM .tasks. WHERE .tasks.\..id. = .+ORDER BY .tasks.\..id. LIMIT .+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "status"}).AddRow(1, 7, "published"))
	mock.ExpectQuery(`(?is)^SELECT .+FROM .submissions. WHERE .submissions.\..id. = .+ORDER BY .submissions.\..id. LIMIT .+FOR UPDATE`).
		WillReturnRows(reviewSubmissionRows("human_reviewing"))
	// approve count = 2 → 本次是第 3 次(终审),推到 approved。
	expectApproveCounts(mock, 2, 0)
	mock.ExpectExec(`(?is)^INSERT INTO .human_reviews.`).
		WillReturnResult(sqlmock.NewResult(31, 1))
	mock.ExpectExec(`(?is)^UPDATE .submissions. SET .+ WHERE id = .+ AND status = .+`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^UPDATE .task_items. SET .+ WHERE id = .+ AND status = .+`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^UPDATE .tasks. SET .+finished_items.=finished_items \+ 1.+ WHERE id = .+`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^INSERT INTO .audit_logs.`).
		WillReturnResult(sqlmock.NewResult(41, 1))
	mock.ExpectCommit()

	got, err := Apply(db, ApplyInput{
		SubmissionID: 42,
		Verdict:      "approve",
		Reason:       "looks good",
		ReviewerID:   9,
		Roles:        []string{"admin"},
	})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if got.SubmissionID != 42 || got.Status != statemachine.StateApproved || got.Stage != StageFinal {
		t.Fatalf("Apply result = %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestApplyArbitrationApproveFinishesItemInOneStep(t *testing.T) {
	db, mock, sqlDB := newReviewMockDB(t)
	defer sqlDB.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT .+FROM .submissions.`).WillReturnRows(reviewSubmissionRows("needs_arbitration"))
	mock.ExpectQuery(`(?is)^SELECT .+FROM .tasks.`).WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "status"}).AddRow(1, 7, "published"))
	mock.ExpectQuery(`(?is)^SELECT .+FROM .submissions.+FOR UPDATE`).WillReturnRows(reviewSubmissionRows("needs_arbitration"))
	mock.ExpectExec(`(?is)^INSERT INTO .human_reviews.`).WillReturnResult(sqlmock.NewResult(31, 1))
	mock.ExpectExec(`(?is)^UPDATE .submissions. SET .+ WHERE id = .+ AND status = .+`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?is)^SELECT .id. FROM .submissions. WHERE item_id = .+ AND id <> .+ AND status = .+ ORDER BY id ASC FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(43))
	mock.ExpectExec(`(?is)^UPDATE .submissions. SET .+ WHERE id IN .+ AND status = .+`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^INSERT INTO .audit_logs.`).WillReturnResult(sqlmock.NewResult(40, 1))
	mock.ExpectExec(`(?is)^UPDATE .task_items. SET .+ WHERE id = .+ AND status = .+`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^UPDATE .tasks. SET .+finished_items.=finished_items \+ 1.+ WHERE id = .+`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^INSERT INTO .audit_logs.`).WillReturnResult(sqlmock.NewResult(41, 1))
	mock.ExpectCommit()

	got, err := Apply(db, ApplyInput{SubmissionID: 42, Verdict: "approve", ReviewerID: 9, Roles: []string{"admin"}})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if got.Status != statemachine.StateApproved || got.Stage != StageFinal {
		t.Fatalf("Apply result = %+v, want approved/final", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

// 仲裁 reject 与 approve 对称:同样终结 item、批量打回 sibling、逐 sibling 写审计,
// 且 task.finished_items 也 +1(M-03:reject 也算已处理;M-04:每个 sibling 都要审计)。
// 若回归了这些,mock 期望会落空。
func TestApplyArbitrationRejectFinishesItemAndRejectsSiblings(t *testing.T) {
	db, mock, sqlDB := newReviewMockDB(t)
	defer sqlDB.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT .+FROM .submissions.`).WillReturnRows(reviewSubmissionRows("needs_arbitration"))
	mock.ExpectQuery(`(?is)^SELECT .+FROM .tasks.`).WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "status"}).AddRow(1, 7, "published"))
	mock.ExpectQuery(`(?is)^SELECT .+FROM .submissions.+FOR UPDATE`).WillReturnRows(reviewSubmissionRows("needs_arbitration"))
	mock.ExpectExec(`(?is)^INSERT INTO .human_reviews.`).WillReturnResult(sqlmock.NewResult(31, 1))
	mock.ExpectExec(`(?is)^UPDATE .submissions. SET .+ WHERE id = .+ AND status = .+`).WillReturnResult(sqlmock.NewResult(0, 1))
	// sibling 锁定 + 批量打回 + 逐 sibling 审计(M-04)。
	mock.ExpectQuery(`(?is)^SELECT .id. FROM .submissions. WHERE item_id = .+ AND id <> .+ AND status = .+ ORDER BY id ASC FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(43).AddRow(44))
	mock.ExpectExec(`(?is)^UPDATE .submissions. SET .+ WHERE id IN .+ AND status = .+`).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(`(?is)^INSERT INTO .audit_logs.`).WillReturnResult(sqlmock.NewResult(40, 1))
	mock.ExpectExec(`(?is)^INSERT INTO .audit_logs.`).WillReturnResult(sqlmock.NewResult(41, 1))
	mock.ExpectExec(`(?is)^UPDATE .task_items. SET .+ WHERE id = .+ AND status = .+`).WillReturnResult(sqlmock.NewResult(0, 1))
	// M-03:reject 也使 finished_items + 1。
	mock.ExpectExec(`(?is)^UPDATE .tasks. SET .+finished_items.=finished_items \+ 1.+ WHERE id = .+`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^INSERT INTO .audit_logs.`).WillReturnResult(sqlmock.NewResult(42, 1))
	mock.ExpectCommit()

	got, err := Apply(db, ApplyInput{SubmissionID: 42, Verdict: "reject", Reason: "都不达标,整题打回", ReviewerID: 9, Roles: []string{"admin"}})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if got.Status != statemachine.StateRejected || got.Stage != StageFinal {
		t.Fatalf("Apply result = %+v, want rejected/final", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

// 两级人审:第 1 次 approve(初审,count=0)只记录 human_reviews + advance,
// submission 停留 human_reviewing 等终审,不动 task_items / tasks。
func TestApplyFirstApproveStaysHumanReviewing(t *testing.T) {
	db, mock, sqlDB := newReviewMockDB(t)
	defer sqlDB.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT .+FROM .submissions.`).WillReturnRows(reviewSubmissionRows("human_reviewing"))
	mock.ExpectQuery(`(?is)^SELECT .+FROM .tasks.`).WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "status"}).AddRow(1, 7, "published"))
	mock.ExpectQuery(`(?is)^SELECT .+FROM .submissions.+FOR UPDATE`).WillReturnRows(reviewSubmissionRows("human_reviewing"))
	expectApproveCounts(mock, 0, 0)
	mock.ExpectExec(`(?is)^INSERT INTO .human_reviews.`).WillReturnResult(sqlmock.NewResult(31, 1))
	// 初审:只 advance(UPDATE updated_at),不改 status,不动 task_items / tasks。
	mock.ExpectExec(`(?is)^UPDATE .submissions. SET .+ WHERE id = .+ AND status = .+`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^INSERT INTO .audit_logs.`).WillReturnResult(sqlmock.NewResult(41, 1))
	mock.ExpectCommit()

	got, err := Apply(db, ApplyInput{SubmissionID: 42, Verdict: "approve", Reason: "looks good", ReviewerID: 9, Roles: []string{"admin"}})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if got.Status != statemachine.StateHumanReviewing || got.Stage != StageFirst {
		t.Fatalf("got %+v, want human_reviewing/first(初审)", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

// 两级人审:第 2 次 approve(终审,count=1)推到 approved,终结 item 并 finished_items+1。
func TestApplyFinalApproveFinishesItem(t *testing.T) {
	db, mock, sqlDB := newReviewMockDB(t)
	defer sqlDB.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT .+FROM .submissions.`).WillReturnRows(reviewSubmissionRows("human_reviewing"))
	mock.ExpectQuery(`(?is)^SELECT .+FROM .tasks.`).WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "status"}).AddRow(1, 7, "published"))
	mock.ExpectQuery(`(?is)^SELECT .+FROM .submissions.+FOR UPDATE`).WillReturnRows(reviewSubmissionRows("human_reviewing"))
	expectApproveCounts(mock, 1, 0) // 已有 1 次(初审)→ 本次是终审
	mock.ExpectExec(`(?is)^INSERT INTO .human_reviews.`).WillReturnResult(sqlmock.NewResult(32, 1))
	mock.ExpectExec(`(?is)^UPDATE .submissions. SET .+ WHERE id = .+ AND status = .+`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^UPDATE .task_items. SET .+ WHERE id = .+ AND status = .+`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^UPDATE .tasks. SET .+finished_items.=finished_items \+ 1.+ WHERE id = .+`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^INSERT INTO .audit_logs.`).WillReturnResult(sqlmock.NewResult(41, 1))
	mock.ExpectCommit()

	got, err := Apply(db, ApplyInput{SubmissionID: 42, Verdict: "approve", ReviewerID: 9, Roles: []string{"admin"}})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if got.Status != statemachine.StateApproved || got.Stage != StageFinal {
		t.Fatalf("got %+v, want approved/final(终审)", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

// manual_review(AI 可疑转人工复核)的 approve 等价初审通过:推进到 human_reviewing 等终审,
// 记一次 first-level approve,但不终结 item / tasks。
func TestApplyManualReviewApproveAdvancesToHumanReviewing(t *testing.T) {
	db, mock, sqlDB := newReviewMockDB(t)
	defer sqlDB.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT .+FROM .submissions.`).WillReturnRows(reviewSubmissionRows("manual_review"))
	mock.ExpectQuery(`(?is)^SELECT .+FROM .tasks.`).WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "status"}).AddRow(1, 7, "published"))
	mock.ExpectQuery(`(?is)^SELECT .+FROM .submissions.+FOR UPDATE`).WillReturnRows(reviewSubmissionRows("manual_review"))
	expectApproveCounts(mock, 0, 0)
	mock.ExpectExec(`(?is)^INSERT INTO .human_reviews.`).WillReturnResult(sqlmock.NewResult(31, 1))
	// 跨状态推进到 human_reviewing,但不动 task_items / tasks(尚未终审)。
	mock.ExpectExec(`(?is)^UPDATE .submissions. SET .+ WHERE id = .+ AND status = .+`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^INSERT INTO .audit_logs.`).WillReturnResult(sqlmock.NewResult(41, 1))
	mock.ExpectCommit()

	got, err := Apply(db, ApplyInput{SubmissionID: 42, Verdict: "approve", Reason: "manual ok", ReviewerID: 9, Roles: []string{"admin"}})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if got.Status != statemachine.StateHumanReviewing || got.Stage != StageFirst {
		t.Fatalf("got %+v, want human_reviewing/first(转人工复核初审通过)", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

// manual_review 的 reject 与 human_reviewing 一致:直接落 rejected 并终结 item。
func TestApplyManualReviewRejectGoesRejected(t *testing.T) {
	db, mock, sqlDB := newReviewMockDB(t)
	defer sqlDB.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT .+FROM .submissions.`).WillReturnRows(reviewSubmissionRows("manual_review"))
	mock.ExpectQuery(`(?is)^SELECT .+FROM .tasks.`).WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "status"}).AddRow(1, 7, "published"))
	mock.ExpectQuery(`(?is)^SELECT .+FROM .submissions.+FOR UPDATE`).WillReturnRows(reviewSubmissionRows("manual_review"))
	mock.ExpectQuery(`(?is)^SELECT count\(\*\) FROM .human_reviews.`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec(`(?is)^INSERT INTO .human_reviews.`).WillReturnResult(sqlmock.NewResult(31, 1))
	mock.ExpectExec(`(?is)^UPDATE .submissions. SET .+ WHERE id = .+ AND status = .+`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^UPDATE .task_items. SET .+ WHERE id = .+ AND status = .+`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^UPDATE .tasks. SET .+finished_items.=finished_items \+ 1.+ WHERE id = .+`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^INSERT INTO .audit_logs.`).WillReturnResult(sqlmock.NewResult(41, 1))
	mock.ExpectCommit()

	got, err := Apply(db, ApplyInput{SubmissionID: 42, Verdict: "reject", Reason: "manual reject reason", ReviewerID: 9, Roles: []string{"admin"}})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if got.Status != statemachine.StateRejected {
		t.Fatalf("status = %s, want rejected", got.Status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

// StageForApproveCount:approveCount → (stage, level) 映射。
func TestStageForApproveCount(t *testing.T) {
	tests := []struct {
		count     int
		wantStage string
		wantLevel int
	}{
		{0, StageFirst, 1},
		{1, StageFinal, 2},
		{2, StageFinal, 2}, // 越界 clamp 到 final
		{-1, StageFirst, 1},
	}
	for _, tt := range tests {
		stage, level := StageForApproveCount(tt.count)
		if stage != tt.wantStage || level != tt.wantLevel {
			t.Errorf("StageForApproveCount(%d) = (%q, %d); want (%q, %d)", tt.count, stage, level, tt.wantStage, tt.wantLevel)
		}
	}
}

func TestApplyRejectFinishesItemAndIncrementsFinishedItems(t *testing.T) {
	db, mock, sqlDB := newReviewMockDB(t)
	defer sqlDB.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT .+FROM .submissions.`).WillReturnRows(reviewSubmissionRows("human_reviewing"))
	mock.ExpectQuery(`(?is)^SELECT .+FROM .tasks.`).WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "status"}).AddRow(1, 7, "published"))
	mock.ExpectQuery(`(?is)^SELECT .+FROM .submissions.+FOR UPDATE`).WillReturnRows(reviewSubmissionRows("human_reviewing"))
	mock.ExpectQuery(`(?is)^SELECT count\(\*\) FROM .human_reviews.`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec(`(?is)^INSERT INTO .human_reviews.`).WillReturnResult(sqlmock.NewResult(31, 1))
	mock.ExpectExec(`(?is)^UPDATE .submissions. SET .+ WHERE id = .+ AND status = .+`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^UPDATE .task_items. SET .+ WHERE id = .+ AND status = .+`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^UPDATE .tasks. SET .+finished_items.=finished_items \+ 1.+ WHERE id = .+`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^INSERT INTO .audit_logs.`).WillReturnResult(sqlmock.NewResult(41, 1))
	mock.ExpectCommit()

	got, err := Apply(db, ApplyInput{SubmissionID: 42, Verdict: "reject", ReviewerID: 9, Roles: []string{"admin"}})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if got.Status != statemachine.StateRejected {
		t.Fatalf("status = %s, want rejected", got.Status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

// reject 在任意 stage(此处终审,已有 1 条 approve)都直接落 rejected 并 finish item。
func TestApplyRejectAtFinalStageGoesRejected(t *testing.T) {
	db, mock, sqlDB := newReviewMockDB(t)
	defer sqlDB.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT .+FROM .submissions.`).WillReturnRows(reviewSubmissionRows("human_reviewing"))
	mock.ExpectQuery(`(?is)^SELECT .+FROM .tasks.`).WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "status"}).AddRow(1, 7, "published"))
	mock.ExpectQuery(`(?is)^SELECT .+FROM .submissions.+FOR UPDATE`).WillReturnRows(reviewSubmissionRows("human_reviewing"))
	// 已有 1 条 approve(初审通过)→ 当前处于终审(final)。reject 仍直接终态化。
	mock.ExpectQuery(`(?is)^SELECT count\(\*\) FROM .human_reviews.`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectExec(`(?is)^INSERT INTO .human_reviews.`).WillReturnResult(sqlmock.NewResult(31, 1))
	mock.ExpectExec(`(?is)^UPDATE .submissions. SET .+ WHERE id = .+ AND status = .+`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^UPDATE .task_items. SET .+ WHERE id = .+ AND status = .+`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^UPDATE .tasks. SET .+finished_items.=finished_items \+ 1.+ WHERE id = .+`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^INSERT INTO .audit_logs.`).WillReturnResult(sqlmock.NewResult(41, 1))
	mock.ExpectCommit()

	got, err := Apply(db, ApplyInput{SubmissionID: 42, Verdict: "reject", Reason: "second-stage reject", ReviewerID: 9, Roles: []string{"admin"}})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if got.Status != statemachine.StateRejected || got.Stage != StageFinal {
		t.Fatalf("Apply result = %+v, want rejected/final", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestApplyReviseDoesNotFinishItem(t *testing.T) {
	db, mock, sqlDB := newReviewMockDB(t)
	defer sqlDB.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT .+FROM .submissions.`).WillReturnRows(reviewSubmissionRows("human_reviewing"))
	mock.ExpectQuery(`(?is)^SELECT .+FROM .tasks.`).WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "status"}).AddRow(1, 7, "published"))
	mock.ExpectQuery(`(?is)^SELECT .+FROM .submissions.+FOR UPDATE`).WillReturnRows(reviewSubmissionRows("human_reviewing"))
	mock.ExpectQuery(`(?is)^SELECT count\(\*\) FROM .human_reviews.`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec(`(?is)^INSERT INTO .human_reviews.`).WillReturnResult(sqlmock.NewResult(31, 1))
	mock.ExpectExec(`(?is)^UPDATE .submissions. SET .+ WHERE id = .+ AND status = .+`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^INSERT INTO .audit_logs.`).WillReturnResult(sqlmock.NewResult(41, 1))
	mock.ExpectCommit()

	got, err := Apply(db, ApplyInput{SubmissionID: 42, Verdict: "revise", Reason: "fix evidence", ReviewerID: 9, Roles: []string{"admin"}})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if got.Status != statemachine.StateRevising {
		t.Fatalf("status = %s, want revising", got.Status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestApplyRejectsInvalidState(t *testing.T) {
	db, mock, sqlDB := newReviewMockDB(t)
	defer sqlDB.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT .+FROM .submissions.`).WillReturnRows(reviewSubmissionRows("draft"))
	mock.ExpectQuery(`(?is)^SELECT .+FROM .tasks.`).WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "status"}).AddRow(1, 7, "published"))
	mock.ExpectQuery(`(?is)^SELECT .+FROM .submissions.+FOR UPDATE`).WillReturnRows(reviewSubmissionRows("draft"))
	mock.ExpectRollback()

	_, err := Apply(db, ApplyInput{SubmissionID: 42, Verdict: "approve", ReviewerID: 9, Roles: []string{"admin"}})
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("err = %v, want ErrInvalidTransition", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestApplyDetectsConcurrentSubmissionUpdate(t *testing.T) {
	db, mock, sqlDB := newReviewMockDB(t)
	defer sqlDB.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT .+FROM .submissions.`).WillReturnRows(reviewSubmissionRows("human_reviewing"))
	mock.ExpectQuery(`(?is)^SELECT .+FROM .tasks.`).WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "status"}).AddRow(1, 7, "published"))
	mock.ExpectQuery(`(?is)^SELECT .+FROM .submissions.+FOR UPDATE`).WillReturnRows(reviewSubmissionRows("human_reviewing"))
	expectApproveCounts(mock, 2, 0)
	mock.ExpectExec(`(?is)^INSERT INTO .human_reviews.`).WillReturnResult(sqlmock.NewResult(31, 1))
	mock.ExpectExec(`(?is)^UPDATE .submissions. SET .+ WHERE id = .+ AND status = .+`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	_, err := Apply(db, ApplyInput{SubmissionID: 42, Verdict: "approve", ReviewerID: 9, Roles: []string{"admin"}})
	if !errors.Is(err, ErrConcurrentWrite) {
		t.Fatalf("err = %v, want ErrConcurrentWrite", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestApplyRejectsDuplicateApproveBySameReviewer(t *testing.T) {
	db, mock, sqlDB := newReviewMockDB(t)
	defer sqlDB.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT .+FROM .submissions.`).WillReturnRows(reviewSubmissionRows("human_reviewing"))
	mock.ExpectQuery(`(?is)^SELECT .+FROM .tasks.`).WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "status"}).AddRow(1, 7, "published"))
	mock.ExpectQuery(`(?is)^SELECT .+FROM .submissions.+FOR UPDATE`).WillReturnRows(reviewSubmissionRows("human_reviewing"))
	expectApproveCounts(mock, 1, 1)
	mock.ExpectRollback()

	_, err := Apply(db, ApplyInput{SubmissionID: 42, Verdict: "approve", ReviewerID: 9, Roles: []string{"admin"}})
	if !errors.Is(err, ErrDuplicateReviewerApproval) {
		t.Fatalf("err = %v, want ErrDuplicateReviewerApproval", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func reviewSubmissionRows(status string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "task_id", "item_id", "current_revision_id", "status"}).
		AddRow(42, 1, 11, 901, status)
}
