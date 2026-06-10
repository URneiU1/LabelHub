package aireview

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func newSweeperMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, *sql.DB) {
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

func TestSweeperMovesStalledAIReviewToHumanReview(t *testing.T) {
	db, mock, sqlDB := newSweeperMockDB(t)
	defer sqlDB.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT ar.id, ar.submission_id, ar.revision_id, ar.idempotency_key.+ar.status = \? AND ar.created_at < \?.+ar.status = \? AND COALESCE\(ar.started_at, ar.created_at\) < \?.+FOR UPDATE SKIP LOCKED`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "submission_id", "revision_id", "idempotency_key"}).
			AddRow(9, 42, 901, "idem"))
	mock.ExpectExec(`(?is)^UPDATE ai_reviews SET status = 'dead'.+status IN`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^UPDATE submissions SET status = \?, ai_verdict = 'uncertain'`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^INSERT INTO .audit_logs.`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	sweeper := NewSweeper(db, zap.NewNop(), time.Minute, time.Minute, 20)
	moved, err := sweeper.SweepOnce(context.Background())
	if err != nil {
		t.Fatalf("SweepOnce returned error: %v", err)
	}
	if moved != 1 {
		t.Fatalf("moved = %d, want 1", moved)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestSweeperNoopsWhenNoStalledReviews(t *testing.T) {
	db, mock, sqlDB := newSweeperMockDB(t)
	defer sqlDB.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT ar.id, ar.submission_id, ar.revision_id, ar.idempotency_key.+FOR UPDATE SKIP LOCKED`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "submission_id", "revision_id", "idempotency_key"}))
	mock.ExpectCommit()

	sweeper := NewSweeper(db, zap.NewNop(), time.Minute, time.Minute, 20)
	moved, err := sweeper.SweepOnce(context.Background())
	if err != nil {
		t.Fatalf("SweepOnce returned error: %v", err)
	}
	if moved != 0 {
		t.Fatalf("moved = %d, want 0", moved)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}
