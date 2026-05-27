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
	StatusPending   = "pending"
	StatusPublished = "published"
	StatusFailed    = "failed"
)

type Publisher struct {
	db       *gorm.DB
	client   *asynq.Client
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
	var events []model.OutboxEvent
	if err := p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status = ?", StatusPending).
			Order("id ASC").
			Limit(p.batch).
			Find(&events).Error
	}); err != nil {
		return err
	}

	for _, event := range events {
		task := asynq.NewTask(event.Topic, []byte(event.Payload))
		if _, err := p.client.EnqueueContext(ctx, task, asynq.MaxRetry(5), asynq.TaskID(outboxTaskID(event))); err != nil && !errors.Is(err, asynq.ErrTaskIDConflict) {
			retryCount := event.RetryCount + 1
			newStatus := StatusPending
			if retryCount >= 5 {
				newStatus = StatusFailed
			}
			if err := p.db.WithContext(ctx).Model(&model.OutboxEvent{}).
				Where("id = ? AND status = ?", event.ID, StatusPending).
				Updates(map[string]any{"status": newStatus, "retry_count": retryCount}).Error; err != nil {
				p.warn("failed to mark outbox event failed", zap.Uint64("id", event.ID), zap.Error(err))
			}
			continue
		}
		if err := p.db.WithContext(ctx).Model(&model.OutboxEvent{}).
			Where("id = ? AND status = ?", event.ID, StatusPending).
			Updates(map[string]any{"status": StatusPublished, "published_at": time.Now().UTC()}).Error; err != nil {
			p.warn("failed to mark outbox event published", zap.Uint64("id", event.ID), zap.Error(err))
		}
	}
	return nil
}

func outboxTaskID(event model.OutboxEvent) string {
	return fmt.Sprintf("outbox:%d", event.ID)
}

func (p Publisher) warn(msg string, fields ...zap.Field) {
	if p.logger != nil {
		p.logger.Warn(msg, fields...)
	}
}
