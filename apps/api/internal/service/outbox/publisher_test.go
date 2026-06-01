package outbox

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/hibiken/asynq"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"labelhub-api/internal/model"
)

func newOutboxMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, *sql.DB) {
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

func TestClaimPendingMarksEventsProcessingInsideTransaction(t *testing.T) {
	db, mock, sqlDB := newOutboxMockDB(t)
	defer sqlDB.Close()

	now := time.Date(2026, 5, 27, 9, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .outbox_events.+WHERE status = .+ORDER BY id ASC LIMIT .+FOR UPDATE SKIP LOCKED`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "topic", "payload", "status", "retry_count", "created_at"}).
			AddRow(11, "ai:review", `{}`, StatusPending, 0, now).
			AddRow(12, "ai:dry-run", `{}`, StatusPending, 1, now))
	mock.ExpectExec(`(?is)^UPDATE .outbox_events. SET .+ WHERE id IN .+ AND status = .+`).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()

	publisher := Publisher{db: db, batch: 2}
	events, err := publisher.claimPending(context.Background(), now)
	if err != nil {
		t.Fatalf("claimPending returned error: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events len = %d, want 2", len(events))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestClaimPendingDetectsLostUpdateRace(t *testing.T) {
	db, mock, sqlDB := newOutboxMockDB(t)
	defer sqlDB.Close()

	now := time.Date(2026, 5, 27, 9, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .outbox_events.+WHERE status = .+ORDER BY id ASC LIMIT .+FOR UPDATE SKIP LOCKED`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "topic", "payload", "status", "retry_count", "created_at"}).
			AddRow(11, "ai:review", `{}`, StatusPending, 0, now).
			AddRow(12, "ai:dry-run", `{}`, StatusPending, 1, now))
	mock.ExpectExec(`(?is)^UPDATE .outbox_events. SET .+ WHERE id IN .+ AND status = .+`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()

	publisher := Publisher{db: db, batch: 2}
	if _, err := publisher.claimPending(context.Background(), now); err == nil {
		t.Fatal("claimPending should fail when RowsAffected differs from claimed event count")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestResetStaleProcessingReturnsEventsToPending(t *testing.T) {
	db, mock, sqlDB := newOutboxMockDB(t)
	defer sqlDB.Close()

	now := time.Date(2026, 5, 27, 9, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectExec(`(?is)^UPDATE .outbox_events. SET .+ WHERE status = .+ AND published_at < .+`).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()

	publisher := Publisher{db: db}
	if err := publisher.resetStaleProcessing(context.Background(), now); err != nil {
		t.Fatalf("resetStaleProcessing returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

// H-04:冷却 failedRetryBackoff 后的 failed 事件应被重新置为 pending,
// 给 Redis 瞬时抖动导致的入队失败一条自动恢复路径,而不是永久卡死。
func TestRecoverFailedEventsRequeuesAfterBackoff(t *testing.T) {
	db, mock, sqlDB := newOutboxMockDB(t)
	defer sqlDB.Close()

	now := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectExec(`(?is)^UPDATE .outbox_events. SET .+WHERE status = .+published_at IS NULL OR published_at <`).
		WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectCommit()

	publisher := Publisher{db: db}
	if err := publisher.recoverFailedEvents(context.Background(), now); err != nil {
		t.Fatalf("recoverFailedEvents returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestPublishEventUsesDeterministicTaskIDAndProcessingStatus(t *testing.T) {
	db, mock, sqlDB := newOutboxMockDB(t)
	defer sqlDB.Close()

	client := &fakeEnqueuer{}
	// GORM 默认把裸 Updates() 包进隐式事务,所以这里必须配 Begin/Commit,
	// 否则 BEGIN 被 sqlmock 拒绝、Updates 报错被吞,ExpectExec 永远匹配不上。
	mock.ExpectBegin()
	mock.ExpectExec(`(?is)^UPDATE .outbox_events. SET .+WHERE id.+status.+`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), uint64(42), StatusProcessing).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	publisher := Publisher{db: db, client: client}
	publisher.publishEvent(context.Background(), model.OutboxEvent{ID: 42, Topic: "ai:review", Payload: `{}`})

	if client.taskID != "outbox:42" {
		t.Fatalf("task id = %q, want outbox:42", client.taskID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestPublishEventRetriesFailedEnqueueThenDeadLetters(t *testing.T) {
	tests := []struct {
		name       string
		retryCount int
		wantStatus string
	}{
		{name: "retry", retryCount: 3, wantStatus: StatusPending},
		{name: "dead-letter", retryCount: 4, wantStatus: StatusFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, sqlDB := newOutboxMockDB(t)
			defer sqlDB.Close()

			client := &fakeEnqueuer{err: errors.New("redis unavailable")}
			mock.ExpectBegin()
			mock.ExpectExec(`(?is)^UPDATE .outbox_events. SET .+ WHERE id = .+ AND status = .+`).
				WithArgs(sqlmock.AnyArg(), tt.wantStatus, uint64(42), StatusProcessing).
				WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectCommit()

			publisher := Publisher{db: db, client: client}
			publisher.publishEvent(context.Background(), model.OutboxEvent{
				ID:         42,
				Topic:      "ai:review",
				Payload:    `{}`,
				RetryCount: tt.retryCount,
			})

			if client.taskID != "outbox:42" {
				t.Fatalf("task id = %q, want outbox:42", client.taskID)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("expectations not met for status %s: %v", tt.wantStatus, err)
			}
		})
	}
}

func TestPublishEventTaskIDConflictIsSuccess(t *testing.T) {
	db, mock, sqlDB := newOutboxMockDB(t)
	defer sqlDB.Close()

	client := &fakeEnqueuer{err: asynq.ErrTaskIDConflict}
	mock.ExpectBegin()
	mock.ExpectExec(`(?is)^UPDATE .outbox_events. SET .+WHERE id.+status.+`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), uint64(42), StatusProcessing).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	publisher := Publisher{db: db, client: client}
	publisher.publishEvent(context.Background(), model.OutboxEvent{ID: 42, Topic: "ai:review", Payload: `{}`})

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

type fakeEnqueuer struct {
	taskID string
	err    error
}

func (f *fakeEnqueuer) EnqueueContext(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	for _, opt := range opts {
		if opt.Type() == asynq.TaskIDOpt {
			f.taskID, _ = opt.Value().(string)
		}
	}
	return nil, f.err
}
