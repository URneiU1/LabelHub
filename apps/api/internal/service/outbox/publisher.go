package outbox

import (
	"context"
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
	return p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var events []model.OutboxEvent
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status = ?", StatusPending).
			Order("id ASC").
			Limit(p.batch).
			Find(&events).Error; err != nil {
			return err
		}
		for _, event := range events {
			task := asynq.NewTask(event.Topic, []byte(event.Payload))
			if _, err := p.client.EnqueueContext(ctx, task, asynq.MaxRetry(5)); err != nil {
				status := StatusPending
				retryCount := event.RetryCount + 1
				if retryCount >= 5 {
					status = StatusFailed
				}
				if err := tx.Model(&model.OutboxEvent{}).
					Where("id = ? AND status = ?", event.ID, StatusPending).
					Updates(map[string]any{"status": status, "retry_count": retryCount}).Error; err != nil {
					return err
				}
				continue
			}
			if err := tx.Model(&model.OutboxEvent{}).
				Where("id = ? AND status = ?", event.ID, StatusPending).
				Updates(map[string]any{"status": StatusPublished, "published_at": time.Now().UTC()}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
