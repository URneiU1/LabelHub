package aireview

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"labelhub-api/internal/service/audit"
	"labelhub-api/internal/statemachine"
)

const (
	reviewStatusPending = "pending"
	reviewStatusRunning = "running"
)

type Sweeper struct {
	db       *gorm.DB
	logger   *zap.Logger
	interval time.Duration
	timeout  time.Duration
	batch    int
}

type stalledReview struct {
	ReviewID       uint64
	SubmissionID   uint64
	RevisionID     uint64
	IdempotencyKey string
}

func NewSweeper(db *gorm.DB, logger *zap.Logger, interval time.Duration, timeout time.Duration, batch int) Sweeper {
	if interval <= 0 {
		interval = time.Minute
	}
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	if batch <= 0 {
		batch = 20
	}
	return Sweeper{db: db, logger: logger, interval: interval, timeout: timeout, batch: batch}
}

func (s Sweeper) Run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		if n, err := s.SweepOnce(ctx); err != nil && s.logger != nil {
			s.logger.Warn("ai review sweeper failed", zap.Error(err))
		} else if n > 0 && s.logger != nil {
			s.logger.Warn("ai review sweeper moved stalled reviews to human review", zap.Int("count", n))
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s Sweeper) SweepOnce(ctx context.Context) (int, error) {
	cutoff := time.Now().UTC().Add(-s.timeout)
	moved := 0
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		reviews, err := loadStalledReviews(tx, cutoff, s.batch)
		if err != nil {
			return err
		}
		for _, review := range reviews {
			now := time.Now().UTC()
			msg := fmt.Sprintf("ai review timed out before worker completion after %s", s.timeout)
			res := tx.Exec(
				`UPDATE ai_reviews SET status = 'dead', error_msg = ?, finished_at = ? WHERE id = ? AND status IN (?, ?)`,
				msg, now, review.ReviewID, reviewStatusPending, reviewStatusRunning,
			)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected != 1 {
				continue
			}
			res = tx.Exec(
				`UPDATE submissions SET status = ?, ai_verdict = 'uncertain' WHERE id = ? AND status = ? AND current_revision_id = ?`,
				statemachine.StateHumanReviewing, review.SubmissionID, statemachine.StateAIReviewing, review.RevisionID,
			)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected != 1 {
				continue
			}
			if err := audit.Write(tx, audit.LogEntry{
				EntityType: "submission",
				EntityID:   review.SubmissionID,
				FromState:  statemachine.StateAIReviewing,
				ToState:    statemachine.StateHumanReviewing,
				ActorType:  "system",
				Event:      statemachine.EventAIFailMax,
				Payload: map[string]any{
					"ai_review_id":    review.ReviewID,
					"revision_id":     review.RevisionID,
					"idempotency_key": review.IdempotencyKey,
					"reason":          "sweeper_timeout",
				},
			}); err != nil {
				return err
			}
			moved++
		}
		return nil
	})
	return moved, err
}

func loadStalledReviews(tx *gorm.DB, cutoff time.Time, batch int) ([]stalledReview, error) {
	rows, err := tx.Raw(
		`SELECT ar.id, ar.submission_id, ar.revision_id, ar.idempotency_key
FROM ai_reviews ar
JOIN submissions s ON s.id = ar.submission_id
WHERE s.status = ?
  AND s.current_revision_id = ar.revision_id
  AND ar.status IN (?, ?)
  AND ar.created_at < ?
ORDER BY ar.id ASC
LIMIT ? FOR UPDATE SKIP LOCKED`,
		statemachine.StateAIReviewing, reviewStatusPending, reviewStatusRunning, cutoff, batch,
	).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	reviews := make([]stalledReview, 0)
	for rows.Next() {
		var review stalledReview
		if err := rows.Scan(&review.ReviewID, &review.SubmissionID, &review.RevisionID, &review.IdempotencyKey); err != nil {
			return nil, err
		}
		reviews = append(reviews, review)
	}
	return reviews, rows.Err()
}
