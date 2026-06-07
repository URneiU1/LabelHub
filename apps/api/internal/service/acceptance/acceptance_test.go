package acceptance

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

func init() {
	// 固定时间,避免 decided_at 影响断言。
	NowUTC = func() time.Time { return time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC) }
}

func TestStartRejectsWhenPendingBatchExists(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT \* FROM .tasks. WHERE .+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status"}).AddRow(1, "published"))
	mock.ExpectQuery(`(?is)^SELECT count\(\*\) FROM .acceptance_batches. WHERE task_id = \? AND status = \?`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectRollback()

	if _, err := Start(db, 1, 7); !errors.Is(err, ErrActiveBatchExists) {
		t.Fatalf("Start = %v, want ErrActiveBatchExists", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestStartSnapshotsApprovedCount(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT \* FROM .tasks. WHERE .+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status"}).AddRow(1, "published"))
	mock.ExpectQuery(`(?is)^SELECT count\(\*\) FROM .acceptance_batches.`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`(?is)^SELECT count\(\*\) FROM .submissions. WHERE task_id = \? AND status = \?`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(5))
	mock.ExpectExec(`(?is)^INSERT INTO .acceptance_batches.`).
		WillReturnResult(sqlmock.NewResult(42, 1))
	mock.ExpectExec(`(?is)^INSERT INTO .audit_logs.`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	batch, err := Start(db, 1, 7)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if batch.ApprovedCount != 5 {
		t.Errorf("ApprovedCount = %d, want 5", batch.ApprovedCount)
	}
	if batch.Status != StatusPending {
		t.Errorf("Status = %q, want pending", batch.Status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestAcceptMarksAccepted(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT \* FROM .acceptance_batches. WHERE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "status"}).AddRow(42, 1, StatusPending))
	mock.ExpectExec(`(?is)^UPDATE .acceptance_batches. SET .+ WHERE id = \? AND status = \?`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^INSERT INTO .audit_logs.`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery(`(?is)^SELECT \* FROM .acceptance_batches. WHERE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "status"}).AddRow(42, 1, StatusAccepted))
	mock.ExpectCommit()

	out, err := Accept(db, 1, 42, 7, "looks good")
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if out.Status != StatusAccepted {
		t.Errorf("Status = %q, want accepted", out.Status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestAcceptRejectsNonPendingBatch(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT \* FROM .acceptance_batches. WHERE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "status"}).AddRow(42, 1, StatusAccepted))
	mock.ExpectRollback()

	if _, err := Accept(db, 1, 42, 7, ""); !errors.Is(err, ErrBatchNotPending) {
		t.Fatalf("Accept = %v, want ErrBatchNotPending", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestAcceptRejectsTaskMismatch(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT \* FROM .acceptance_batches. WHERE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "status"}).AddRow(42, 999, StatusPending))
	mock.ExpectRollback()

	if _, err := Accept(db, 1, 42, 7, ""); !errors.Is(err, ErrBatchTaskMismatch) {
		t.Fatalf("Accept = %v, want ErrBatchTaskMismatch", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestRejectReopensFlaggedApprovedSubmission(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectBegin()
	// loadPendingBatch
	mock.ExpectQuery(`(?is)^SELECT \* FROM .acceptance_batches. WHERE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "status"}).AddRow(42, 1, StatusPending))
	// markDecided -> rejected
	mock.ExpectExec(`(?is)^UPDATE .acceptance_batches. SET .+ WHERE id = \? AND status = \?`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// pluck flagged submission ids
	mock.ExpectQuery(`(?is)^SELECT .submission_id. FROM .acceptance_spot_checks. WHERE batch_id = \? AND result = \?`).
		WillReturnRows(sqlmock.NewRows([]string{"submission_id"}).AddRow(101))
	// load the still-approved submission
	mock.ExpectQuery(`(?is)^SELECT \* FROM .submissions. WHERE id = \? AND task_id = \? AND status = \?`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "item_id", "status"}).AddRow(101, 1, 55, statemachine.StateApproved))
	// reopen submission approved -> human_reviewing
	mock.ExpectExec(`(?is)^UPDATE .submissions. SET .status.+ WHERE id = \? AND status = \?`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// revert task_item finished -> claimed
	mock.ExpectExec(`(?is)^UPDATE .task_items. SET .+ WHERE id = \? AND status = \?`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// finished_items - 1
	mock.ExpectExec(`(?is)^UPDATE .tasks. SET .finished_items.+ WHERE id = \? AND finished_items > 0`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// audit: submission reopen
	mock.ExpectExec(`(?is)^INSERT INTO .audit_logs.`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	// audit: batch reject
	mock.ExpectExec(`(?is)^INSERT INTO .audit_logs.`).
		WillReturnResult(sqlmock.NewResult(2, 1))
	// reload batch
	mock.ExpectQuery(`(?is)^SELECT \* FROM .acceptance_batches. WHERE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "status"}).AddRow(42, 1, StatusRejected))
	mock.ExpectCommit()

	result, err := Reject(db, 1, 42, 7, "sampled bad data")
	if err != nil {
		t.Fatalf("Reject: %v", err)
	}
	if result.ReopenedCount != 1 {
		t.Errorf("ReopenedCount = %d, want 1", result.ReopenedCount)
	}
	if result.Batch.Status != StatusRejected {
		t.Errorf("Batch.Status = %q, want rejected", result.Batch.Status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestRecordSpotCheckRejectsNonApprovedSubmission(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT \* FROM .acceptance_batches. WHERE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "status"}).AddRow(42, 1, StatusPending))
	mock.ExpectQuery(`(?is)^SELECT \* FROM .submissions. WHERE id = \? AND task_id = \?`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "item_id", "status"}).AddRow(101, 1, 55, statemachine.StateHumanReviewing))
	mock.ExpectRollback()

	if _, err := RecordSpotCheck(db, 1, 42, 101, ResultFlag, "", 7); !errors.Is(err, ErrSubmissionNotEligible) {
		t.Fatalf("RecordSpotCheck = %v, want ErrSubmissionNotEligible", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestRecordSpotCheckRejectsBadResult(t *testing.T) {
	db, _, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	if _, err := RecordSpotCheck(db, 1, 42, 101, "maybe", "", 7); !errors.Is(err, ErrInvalidResult) {
		t.Fatalf("RecordSpotCheck = %v, want ErrInvalidResult", err)
	}
}
