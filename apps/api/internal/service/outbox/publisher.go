package outbox

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"labelhub-api/internal/model"
)

const (
	StatusPending    = "pending"
	StatusProcessing = "processing"
	StatusPublished  = "published"
	StatusFailed     = "failed"

	processingTimeout = 5 * time.Minute
)

type enqueuer interface {
	EnqueueContext(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

type Publisher struct {
	db       *gorm.DB
	client   enqueuer
	logger   *zap.Logger
	interval time.Duration
	batch    int
}

func NewPublisher(db *gorm.DB, client *asynq.Client, logger *zap.Logger, interval time.Duration, batch int) Publisher {
	if interval <= 0 {
		interval = time.Second
	}
	if batch <= 0 {
		batch = 20
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return Publisher{db: db, client: client, logger: logger, interval: interval, batch: batch}
}

func (p Publisher) Run(ctx context.Context) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		if err := p.PublishOnce(ctx); err != nil && p.logger != nil {
			p.logger.Warn("outbox publish failed", zap.Error(err))
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (p Publisher) PublishOnce(ctx context.Context) error {
	now := time.Now().UTC()
	if err := p.resetStaleProcessing(ctx, now); err != nil {
		return err
	}
	events, err := p.claimPending(ctx, now)
	if err != nil {
		return err
	}

	for _, event := range events {
		p.publishEvent(ctx, event)
	}
	return nil
}

func (p Publisher) resetStaleProcessing(ctx context.Context, now time.Time) error {
	return p.db.WithContext(ctx).Model(&model.OutboxEvent{}).
		Where("status = ? AND published_at < ?", StatusProcessing, now.Add(-processingTimeout)).
		Updates(map[string]any{"status": StatusPending, "published_at": nil}).Error
}

func (p Publisher) claimPending(ctx context.Context, now time.Time) ([]model.OutboxEvent, error) {
	var events []model.OutboxEvent
	if err := p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status = ?", StatusPending).
			Order("id ASC").
			Limit(p.batch).
			Find(&events).Error; err != nil {
			return err
		}
		if len(events) == 0 {
			return nil
		}
		ids := make([]uint64, 0, len(events))
		for _, event := range events {
			ids = append(ids, event.ID)
		}
		res := tx.Model(&model.OutboxEvent{}).
			Where("id IN ? AND status = ?", ids, StatusPending).
			Updates(map[string]any{"status": StatusProcessing, "published_at": now})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != int64(len(events)) {
			return errors.New("outbox claim lost update race")
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return events, nil
}

func (p Publisher) publishEvent(ctx context.Context, event model.OutboxEvent) {
	task := asynq.NewTask(event.Topic, []byte(event.Payload))
	if _, err := p.client.EnqueueContext(ctx, task, asynq.MaxRetry(5), asynq.TaskID(outboxTaskID(event))); err != nil && !errors.Is(err, asynq.ErrTaskIDConflict) {
		retryCount := event.RetryCount + 1
		newStatus := StatusPending
		if retryCount >= 5 {
			newStatus = StatusFailed
		}
		if err := p.db.WithContext(ctx).Model(&model.OutboxEvent{}).
			Where("id = ? AND status = ?", event.ID, StatusProcessing).
			Updates(map[string]any{"status": newStatus, "retry_count": gorm.Expr("retry_count + 1"), "published_at": nil}).Error; err != nil {
			p.warn("failed to mark outbox event failed", zap.Uint64("id", event.ID), zap.Error(err))
		}
		return
	}
	if err := p.db.WithContext(ctx).Model(&model.OutboxEvent{}).
		Where("id = ? AND status = ?", event.ID, StatusProcessing).
		Updates(map[string]any{"status": StatusPublished, "published_at": time.Now().UTC()}).Error; err != nil {
		p.warn("failed to mark outbox event published", zap.Uint64("id", event.ID), zap.Error(err))
	}
}

func outboxTaskID(event model.OutboxEvent) string {
	return fmt.Sprintf("outbox:%d", event.ID)
}

func (p Publisher) warn(msg string, fields ...zap.Field) {
	if p.logger != nil {
		p.logger.Warn(msg, fields...)
	}
}
