package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/hibiken/asynq"
	"go.uber.org/zap"
	"labelhub.local/llmreview"
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
	input, err := h.loadReviewInput(reviewCtx, payload)
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
	result, err := h.evaluator.Evaluate(reviewCtx, payload, input)
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
	Verdict      string
	Score        float64
	Reason       string
	Dimensions   string
	RawResponse  string
	TokensInput  int
	TokensOutput int
	LatencyMS    int
}

type aiReviewInput struct {
	Prompt              llmreview.PromptConfig
	AnswerJSON          string
	PayloadJSON         string
	BaselineDescription string
}

type aiEvaluator interface {
	Evaluate(ctx context.Context, payload aiReviewPayload, input aiReviewInput) (aiEvaluation, error)
}

type deterministicEvaluator struct{}

func (deterministicEvaluator) Evaluate(ctx context.Context, payload aiReviewPayload, input aiReviewInput) (aiEvaluation, error) {
	if envOrDefault("AI_WORKER_FORCE_FAIL", "") == "1" {
		return aiEvaluation{}, errors.New("forced ai worker failure")
	}
	return evaluateWithProvider(ctx, llmreview.MockProvider{}, payload, input)
}

type providerEvaluator struct {
	provider llmreview.Provider
}

func (e providerEvaluator) Evaluate(ctx context.Context, payload aiReviewPayload, input aiReviewInput) (aiEvaluation, error) {
	return evaluateWithProvider(ctx, e.provider, payload, input)
}

func newEvaluatorFromEnv() (aiEvaluator, error) {
	provider, cfg, err := llmreview.NewProviderFromEnv(nil)
	if err != nil {
		return nil, err
	}
	if cfg.Provider == "mock" || cfg.Provider == "deterministic" {
		return deterministicEvaluator{}, nil
	}
	return providerEvaluator{provider: provider}, nil
}

func evaluateWithProvider(ctx context.Context, provider llmreview.Provider, payload aiReviewPayload, input aiReviewInput) (aiEvaluation, error) {
	result, err := provider.Evaluate(ctx, input.Prompt, llmreview.EvaluationInput{
		SubmissionID:        payload.SubmissionID,
		RevisionID:          payload.RevisionID,
		PromptConfigID:      payload.PromptConfigID,
		PromptVersion:       payload.PromptVersion,
		IdempotencyKey:      payload.IdempotencyKey,
		PayloadJSON:         input.PayloadJSON,
		AnswerJSON:          input.AnswerJSON,
		BaselineDescription: input.BaselineDescription,
	})
	if err != nil {
		return aiEvaluation{}, err
	}
	dimensions, err := json.Marshal(result.Dimensions)
	if err != nil {
		return aiEvaluation{}, err
	}
	return aiEvaluation{
		Verdict:      result.Verdict,
		Score:        result.OverallScore,
		Reason:       result.Reason,
		Dimensions:   string(dimensions),
		RawResponse:  result.RawResponse,
		TokensInput:  result.TokensInput,
		TokensOutput: result.TokensOutput,
		LatencyMS:    result.LatencyMS,
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

func (h workerHandlers) loadReviewInput(ctx context.Context, payload aiReviewPayload) (aiReviewInput, error) {
	var input aiReviewInput
	var dimensionsRaw string
	err := h.db.QueryRowContext(ctx, `
SELECT sr.answer, ti.payload, COALESCE(t.baseline_description,''), cfg.prompt_template, cfg.dimensions, cfg.pass_threshold, cfg.uncertain_min, cfg.model
FROM submission_revisions sr
JOIN submissions s ON s.id = sr.submission_id
JOIN task_items ti ON ti.id = s.item_id
JOIN tasks t ON t.id = s.task_id
JOIN ai_prompt_configs cfg ON cfg.id = ? AND cfg.task_id = t.id AND cfg.version = ?
WHERE sr.id = ? AND sr.submission_id = ?`,
		payload.PromptConfigID, payload.PromptVersion, payload.RevisionID, payload.SubmissionID,
	).Scan(
		&input.AnswerJSON,
		&input.PayloadJSON,
		&input.BaselineDescription,
		&input.Prompt.PromptTemplate,
		&dimensionsRaw,
		&input.Prompt.PassThreshold,
		&input.Prompt.UncertainMin,
		&input.Prompt.Model,
	)
	if err != nil {
		return aiReviewInput{}, err
	}
	dimensions, err := llmreview.ParseDimensions(dimensionsRaw)
	if err != nil {
		return aiReviewInput{}, err
	}
	input.Prompt.ID = payload.PromptConfigID
	input.Prompt.Version = payload.PromptVersion
	input.Prompt.Dimensions = dimensions
	return input, nil
}

func (h workerHandlers) complete(ctx context.Context, payload aiReviewPayload, result aiEvaluation) error {
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollbackUnlessCommitted(tx)

	reviewStatus, err := lockedAIReviewStatus(ctx, tx, payload.IdempotencyKey)
	if err != nil {
		return err
	}
	if !canFinalizeAIReview(reviewStatus) {
		return tx.Commit()
	}
	status, currentRevisionID, err := lockedSubmissionState(ctx, tx, payload.SubmissionID)
	if err != nil {
		return err
	}
	if status != "ai_reviewing" || !currentRevisionID.Valid || uint64(currentRevisionID.Int64) != payload.RevisionID {
		return tx.Commit()
	}

	now := time.Now().UTC()
	reviewRes, err := tx.ExecContext(ctx,
		`UPDATE ai_reviews SET status = 'succeeded', verdict = ?, overall_score = ?, dimensions = ?, reason = ?, raw_response = ?, tokens_input = ?, tokens_output = ?, latency_ms = ?, error_msg = NULL, finished_at = ? WHERE idempotency_key = ? AND status IN ('pending','running','failed')`,
		result.Verdict, result.Score, result.Dimensions, result.Reason, result.RawResponse, result.TokensInput, result.TokensOutput, result.LatencyMS, now, payload.IdempotencyKey,
	)
	if err != nil {
		return err
	}
	if rows, _ := reviewRes.RowsAffected(); rows != 1 {
		return errors.New("ai review completion lost review update race")
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

	reviewStatus, err := lockedAIReviewStatus(ctx, tx, payload.IdempotencyKey)
	if err != nil {
		return err
	}
	if !canFinalizeAIReview(reviewStatus) {
		return tx.Commit()
	}
	status, currentRevisionID, err := lockedSubmissionState(ctx, tx, payload.SubmissionID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	deadRes, err := tx.ExecContext(ctx,
		`UPDATE ai_reviews SET status = 'dead', error_msg = ?, finished_at = ? WHERE idempotency_key = ? AND status IN ('pending','running','failed')`,
		cause.Error(), now, payload.IdempotencyKey,
	)
	if err != nil {
		return err
	}
	if rows, _ := deadRes.RowsAffected(); rows != 1 {
		return tx.Commit()
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

func lockedAIReviewStatus(ctx context.Context, tx *sql.Tx, idempotencyKey string) (string, error) {
	var status string
	err := tx.QueryRowContext(ctx,
		`SELECT status FROM ai_reviews WHERE idempotency_key = ? FOR UPDATE`,
		idempotencyKey,
	).Scan(&status)
	return status, err
}

func canFinalizeAIReview(status string) bool {
	switch status {
	case "pending", "running", "failed":
		return true
	default:
		return false
	}
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
