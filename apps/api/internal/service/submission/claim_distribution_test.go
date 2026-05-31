package submission

import (
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"labelhub-api/internal/statemachine"
)

func TestClaim_ReleasesExpiredClaimsBeforeServingNextAvailableItem(t *testing.T) {
	db, mock, sqlDB := newSubmissionMockDB(t)
	defer sqlDB.Close()

	now := time.Date(2026, 6, 1, 4, 0, 0, 0, time.UTC)
	previousNow := NowUTC
	NowUTC = func() time.Time { return now }
	defer func() { NowUTC = previousNow }()

	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "lease_timeout_minutes"}).
			AddRow(1, "published", 30))
	mock.ExpectExec(`(?is)^UPDATE .task_items. SET .+ WHERE task_id = .+ AND status = .+ AND claimed_at IS NOT NULL AND claimed_at <`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_items.+claimed_by`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_items.+SKIP LOCKED`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "status"}).AddRow(7, 1, "available"))
	mock.ExpectExec(`(?is)^UPDATE .task_items. SET`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^INSERT INTO .submissions.`).
		WillReturnResult(sqlmock.NewResult(901, 1))
	mock.ExpectCommit()

	result, err := Claim(db, ClaimInput{TaskID: 1, LabelerID: 5})
	if err != nil {
		t.Fatalf("claim errored: %v", err)
	}
	if result.Item.ID != 7 {
		t.Fatalf("unexpected item: %+v", result.Item)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestClaim_DailySubmissionLimitBlocksWhenReached(t *testing.T) {
	db, mock, sqlDB := newSubmissionMockDB(t)
	defer sqlDB.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "daily_submission_limit_per_labeler"}).
			AddRow(1, "published", 2))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_items.+claimed_by`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery(`(?is)^SELECT count.+FROM .submissions.`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectRollback()

	_, err := Claim(db, ClaimInput{TaskID: 1, LabelerID: 5})
	if !errors.Is(err, ErrDailySubmissionLimitReached) {
		t.Fatalf("expected ErrDailySubmissionLimitReached, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestClaim_OverlapExcludesItemsAlreadySubmittedByLabeler(t *testing.T) {
	db, mock, sqlDB := newSubmissionMockDB(t)
	defer sqlDB.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "overlap_count", "overlap_coverage_pct"}).
			AddRow(1, "published", 2, 100))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_items.+claimed_by`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_items.+NOT EXISTS.+submissions.+MOD.+SKIP LOCKED`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "status"}).AddRow(8, 1, "available"))
	mock.ExpectExec(`(?is)^UPDATE .task_items. SET`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^INSERT INTO .submissions.`).
		WillReturnResult(sqlmock.NewResult(902, 1))
	mock.ExpectCommit()

	result, err := Claim(db, ClaimInput{TaskID: 1, LabelerID: 5})
	if err != nil {
		t.Fatalf("claim errored: %v", err)
	}
	if result.Item.ID != 8 {
		t.Fatalf("unexpected item: %+v", result.Item)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// TestClaim_QuotaBlocksWhenLimitReached:quota 分发下,labeler 在本任务的 submission 数
// 已达 QuotaPerUser 时拒绝领新题(ErrQuotaReached),不会再 SELECT available。
func TestClaim_QuotaBlocksWhenLimitReached(t *testing.T) {
	db, mock, sqlDB := newSubmissionMockDB(t)
	defer sqlDB.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "distribution", "quota_per_user"}).
			AddRow(1, "published", "quota", 2))
	// 无既有认领。
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_items.+claimed_by`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	// 配额计数:已有 2 条 submission == 上限。
	mock.ExpectQuery(`(?is)^SELECT count.+FROM .submissions.`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectRollback()

	_, err := Claim(db, ClaimInput{TaskID: 1, LabelerID: 5})
	if !errors.Is(err, ErrQuotaReached) {
		t.Fatalf("expected ErrQuotaReached, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// TestClaim_QuotaAllowsBelowLimit:quota 分发下,未达上限可正常领题。
func TestClaim_QuotaAllowsBelowLimit(t *testing.T) {
	db, mock, sqlDB := newSubmissionMockDB(t)
	defer sqlDB.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "distribution", "quota_per_user"}).
			AddRow(1, "published", "quota", 3))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_items.+claimed_by`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery(`(?is)^SELECT count.+FROM .submissions.`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_items.+SKIP LOCKED`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "status"}).AddRow(7, 1, "available"))
	mock.ExpectExec(`(?is)^UPDATE .task_items. SET`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^INSERT INTO .submissions.`).
		WillReturnResult(sqlmock.NewResult(901, 1))
	mock.ExpectCommit()

	result, err := Claim(db, ClaimInput{TaskID: 1, LabelerID: 5})
	if err != nil {
		t.Fatalf("claim errored: %v", err)
	}
	if result.Item.ID != 7 || result.Submission.Status != statemachine.StateDraft {
		t.Fatalf("unexpected result: %+v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// TestClaim_AssignedBlocksUnassignedLabeler:assigned 分发下,未被指派的 labeler
// 领新题被拒(ErrNotAssigned)。
func TestClaim_AssignedBlocksUnassignedLabeler(t *testing.T) {
	db, mock, sqlDB := newSubmissionMockDB(t)
	defer sqlDB.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "distribution", "quota_per_user"}).
			AddRow(1, "published", "assigned", 0))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_items.+claimed_by`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	// 指派计数:0 行 → 未被指派。
	mock.ExpectQuery(`(?is)^SELECT count.+FROM .task_assignees.`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectRollback()

	_, err := Claim(db, ClaimInput{TaskID: 1, LabelerID: 5})
	if !errors.Is(err, ErrNotAssigned) {
		t.Fatalf("expected ErrNotAssigned, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// TestClaim_AssignedAllowsAssignedLabeler:assigned 分发下,已被指派的 labeler 可领题。
func TestClaim_AssignedAllowsAssignedLabeler(t *testing.T) {
	db, mock, sqlDB := newSubmissionMockDB(t)
	defer sqlDB.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "distribution", "quota_per_user"}).
			AddRow(1, "published", "assigned", 0))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_items.+claimed_by`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery(`(?is)^SELECT count.+FROM .task_assignees.`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_items.+SKIP LOCKED`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "status"}).AddRow(7, 1, "available"))
	mock.ExpectExec(`(?is)^UPDATE .task_items. SET`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^INSERT INTO .submissions.`).
		WillReturnResult(sqlmock.NewResult(901, 1))
	mock.ExpectCommit()

	result, err := Claim(db, ClaimInput{TaskID: 1, LabelerID: 5})
	if err != nil {
		t.Fatalf("claim errored: %v", err)
	}
	if result.Item.ID != 7 {
		t.Fatalf("unexpected item: %+v", result.Item)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
