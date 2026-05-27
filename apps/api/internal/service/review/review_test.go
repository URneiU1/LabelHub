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

func TestApplyApproveWritesHumanReviewAndFinishesItem(t *testing.T) {
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
	if got.SubmissionID != 42 || got.Status != statemachine.StateApproved {
		t.Fatalf("Apply result = %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestApplyRejectFinishesItemWithoutIncrementingFinishedItems(t *testing.T) {
	db, mock, sqlDB := newReviewMockDB(t)
	defer sqlDB.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT .+FROM .submissions.`).WillReturnRows(reviewSubmissionRows("human_reviewing"))
	mock.ExpectQuery(`(?is)^SELECT .+FROM .tasks.`).WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "status"}).AddRow(1, 7, "published"))
	mock.ExpectQuery(`(?is)^SELECT .+FROM .submissions.+FOR UPDATE`).WillReturnRows(reviewSubmissionRows("human_reviewing"))
	mock.ExpectExec(`(?is)^INSERT INTO .human_reviews.`).WillReturnResult(sqlmock.NewResult(31, 1))
	mock.ExpectExec(`(?is)^UPDATE .submissions. SET .+ WHERE id = .+ AND status = .+`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^UPDATE .task_items. SET .+ WHERE id = .+ AND status = .+`).WillReturnResult(sqlmock.NewResult(0, 1))
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

func TestApplyReviseDoesNotFinishItem(t *testing.T) {
	db, mock, sqlDB := newReviewMockDB(t)
	defer sqlDB.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT .+FROM .submissions.`).WillReturnRows(reviewSubmissionRows("human_reviewing"))
	mock.ExpectQuery(`(?is)^SELECT .+FROM .tasks.`).WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "status"}).AddRow(1, 7, "published"))
	mock.ExpectQuery(`(?is)^SELECT .+FROM .submissions.+FOR UPDATE`).WillReturnRows(reviewSubmissionRows("human_reviewing"))
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

func reviewSubmissionRows(status string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "task_id", "item_id", "current_revision_id", "status"}).
		AddRow(42, 1, 11, 901, status)
}
