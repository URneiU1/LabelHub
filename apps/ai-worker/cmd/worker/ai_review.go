package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"time"

	"github.com/hibiken/asynq"
	"go.uber.org/zap"
)

func (h workerHandlers) handleAIReview(ctx context.Context, t *asynq.Task) error {
	payload, err := parseAIReviewPayload(t.Payload())
	if err != nil {
		h.logger.Warn("invalid ai review payload", zap.Error(err), zap.ByteString("payload", t.Payload()))
		return nil
	}
	claimed, err := h.markRunning(ctx, payload)
	if err != nil {
		return err
	}
	if !claimed {
		h.logger.Info("ai review task already finalized or claimed elsewhere",
			zap.Uint64("submission_id", payload.SubmissionID),
			zap.Uint64("revision_id", payload.RevisionID),
			zap.String("idempotency_key", payload.IdempotencyKey),
		)
		return nil
	}

	reviewCtx, cancel := context.WithTimeout(ctx, aiReviewTimeout())
	defer cancel()
	answer, err := h.loadRevisionAnswer(reviewCtx, payload.RevisionID)
	if err != nil {
		if shouldFailover(ctx) {
			if failErr := h.failover(ctx, payload, err); failErr != nil {
				return failErr
			}
			return nil
		}
		_ = h.markFailed(ctx, payload, err)
		return err
	}
	result, err := h.evaluator.Evaluate(reviewCtx, payload, answer)
	if err != nil {
		if shouldFailover(ctx) {
			if failErr := h.failover(ctx, payload, err); failErr != nil {
				return failErr
			}
			return nil
		}
		_ = h.markFailed(ctx, payload, err)
		return err
	}
	if err := h.complete(ctx, payload, result); err != nil {
		return err
	}
	h.logger.Info("ai review completed",
		zap.Uint64("submission_id", payload.SubmissionID),
		zap.Uint64("revision_id", payload.RevisionID),
		zap.String("verdict", result.Verdict),
		zap.Float64("score", result.Score),
	)
	return nil
}

type aiReviewPayload struct {
	SubmissionID   uint64 `json:"submission_id"`
	RevisionID     uint64 `json:"revision_id"`
	PromptConfigID uint64 `json:"prompt_config_id"`
	PromptVersion  int    `json:"prompt_version"`
	IdempotencyKey string `json:"idempotency_key"`
}

type aiEvaluation struct {
	Verdict     string
	Score       float64
	Reason      string
	Dimensions  string
	RawResponse string
	LatencyMS   int
}

type aiEvaluator interface {
	Evaluate(ctx context.Context, payload aiReviewPayload, answer string) (aiEvaluation, error)
}

type deterministicEvaluator struct{}

func (deterministicEvaluator) Evaluate(ctx context.Context, payload aiReviewPayload, answer string) (aiEvaluation, error) {
	if os.Getenv("AI_WORKER_FORCE_FAIL") == "1" {
		return aiEvaluation{}, errors.New("forced ai worker failure")
	}
	start := time.Now()
	select {
	case <-ctx.Done():
		return aiEvaluation{}, ctx.Err()
	default:
	}
	score := 75.0
	verdict := "uncertain"
	if len(answer) <= 2 {
		score = 50
	}
	dimensions := `[{"name":"结构完整性","score":75,"reason":"deterministic precheck completed; route to human review for final decision"}]`
	raw, _ := json.Marshal(map[string]any{
		"mode":             "deterministic",
		"prompt_config_id": payload.PromptConfigID,
		"prompt_version":   payload.PromptVersion,
	})
	return aiEvaluation{
		Verdict:     verdict,
		Score:       score,
		Reason:      "AI precheck completed; human review required for final verdict.",
		Dimensions:  dimensions,
		RawResponse: string(raw),
		LatencyMS:   int(time.Since(start).Milliseconds()),
	}, nil
}

func parseAIReviewPayload(raw []byte) (aiReviewPayload, error) {
	var payload aiReviewPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return aiReviewPayload{}, err
	}
	if payload.SubmissionID == 0 || payload.RevisionID == 0 || payload.PromptConfigID == 0 || payload.PromptVersion <= 0 || payload.IdempotencyKey == "" {
		return aiReviewPayload{}, errors.New("missing required ai review payload fields")
	}
	return payload, nil
}

func (h workerHandlers) markRunning(ctx context.Context, payload aiReviewPayload) (bool, error) {
	res, err := h.db.ExecContext(ctx,
		`UPDATE ai_reviews SET status = 'running', retry_count = retry_count + 1 WHERE idempotency_key = ? AND status IN ('pending','failed')`,
		payload.IdempotencyKey,
	)
	if err != nil {
		return false, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows == 1, nil
}

func (h workerHandlers) markFailed(ctx context.Context, payload aiReviewPayload, cause error) error {
	_, err := h.db.ExecContext(ctx,
		`UPDATE ai_reviews SET status = 'failed', error_msg = ? WHERE idempotency_key = ? AND status IN ('pending','running','failed')`,
		cause.Error(), payload.IdempotencyKey,
	)
	return err
}

func (h workerHandlers) loadRevisionAnswer(ctx context.Context, revisionID uint64) (string, error) {
	var answer string
	err := h.db.QueryRowContext(ctx, `SELECT answer FROM submission_revisions WHERE id = ?`, revisionID).Scan(&answer)
	return answer, err
}

func (h workerHandlers) complete(ctx context.Context, payload aiReviewPayload, result aiEvaluation) error {
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollbackUnlessCommitted(tx)

	status, currentRevisionID, err := lockedSubmissionState(ctx, tx, payload.SubmissionID)
	if err != nil {
		return err
	}
	if status != "ai_reviewing" || !currentRevisionID.Valid || uint64(currentRevisionID.Int64) != payload.RevisionID {
		return tx.Commit()
	}

	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx,
		`UPDATE ai_reviews SET status = 'succeeded', verdict = ?, overall_score = ?, dimensions = ?, reason = ?, raw_response = ?, latency_ms = ?, finished_at = ? WHERE idempotency_key = ?`,
		result.Verdict, result.Score, result.Dimensions, result.Reason, result.RawResponse, result.LatencyMS, now, payload.IdempotencyKey,
	); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx,
		`UPDATE submissions SET status = 'human_reviewing', ai_verdict = ?, ai_score = ? WHERE id = ? AND status = 'ai_reviewing' AND current_revision_id = ?`,
		result.Verdict, result.Score, payload.SubmissionID, payload.RevisionID,
	)
	if err != nil {
		return err
	}
	if rows, _ := res.RowsAffected(); rows != 1 {
		return errors.New("ai review completion lost submission update race")
	}
	auditPayload, _ := json.Marshal(map[string]any{"idempotency_key": payload.IdempotencyKey, "score": result.Score})
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO audit_logs (entity_type, entity_id, from_state, to_state, actor_type, event, payload, created_at) VALUES ('submission', ?, 'ai_reviewing', 'human_reviewing', 'system', 'ai_done', ?, ?)`,
		payload.SubmissionID, string(auditPayload), now,
	); err != nil {
		return err
	}
	return tx.Commit()
}

func (h workerHandlers) failover(ctx context.Context, payload aiReviewPayload, cause error) error {
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollbackUnlessCommitted(tx)

	status, currentRevisionID, err := lockedSubmissionState(ctx, tx, payload.SubmissionID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx,
		`UPDATE ai_reviews SET status = 'dead', error_msg = ?, finished_at = ? WHERE idempotency_key = ? AND status IN ('pending','running','failed')`,
		cause.Error(), now, payload.IdempotencyKey,
	); err != nil {
		return err
	}
	if status != "ai_reviewing" || !currentRevisionID.Valid || uint64(currentRevisionID.Int64) != payload.RevisionID {
		return tx.Commit()
	}
	res, err := tx.ExecContext(ctx,
		`UPDATE submissions SET status = 'human_reviewing', ai_verdict = 'uncertain' WHERE id = ? AND status = 'ai_reviewing' AND current_revision_id = ?`,
		payload.SubmissionID, payload.RevisionID,
	)
	if err != nil {
		return err
	}
	if rows, _ := res.RowsAffected(); rows != 1 {
		return errors.New("ai review failover lost submission update race")
	}
	auditPayload, _ := json.Marshal(map[string]any{"idempotency_key": payload.IdempotencyKey, "error": cause.Error()})
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO audit_logs (entity_type, entity_id, from_state, to_state, actor_type, event, payload, created_at) VALUES ('submission', ?, 'ai_reviewing', 'human_reviewing', 'system', 'ai_fail_max', ?, ?)`,
		payload.SubmissionID, string(auditPayload), now,
	); err != nil {
		return err
	}
	h.logger.Warn("ai review failed over to human review",
		zap.Uint64("submission_id", payload.SubmissionID),
		zap.Uint64("revision_id", payload.RevisionID),
		zap.Error(cause),
	)
	return tx.Commit()
}

func lockedSubmissionState(ctx context.Context, tx *sql.Tx, submissionID uint64) (string, sql.NullInt64, error) {
	var status string
	var currentRevisionID sql.NullInt64
	err := tx.QueryRowContext(ctx,
		`SELECT status, current_revision_id FROM submissions WHERE id = ? FOR UPDATE`,
		submissionID,
	).Scan(&status, &currentRevisionID)
	return status, currentRevisionID, err
}

func rollbackUnlessCommitted(tx *sql.Tx) {
	_ = tx.Rollback()
}

func shouldFailover(ctx context.Context) bool {
	retryCount, okRetry := asynq.GetRetryCount(ctx)
	maxRetry, okMax := asynq.GetMaxRetry(ctx)
	if !okRetry || !okMax {
		return true
	}
	return retryCount >= maxRetry
}
