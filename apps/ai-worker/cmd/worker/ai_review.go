package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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
	if err := h.circuit.check(); err != nil {
		h.logger.Warn("ai review provider circuit open",
			zap.Uint64("submission_id", payload.SubmissionID),
			zap.Uint64("revision_id", payload.RevisionID),
			zap.Error(err),
		)
		return h.retryOrFailover(ctx, payload, err)
	}

	reviewCtx, cancel := context.WithTimeout(ctx, aiReviewTimeout())
	defer cancel()
	input, err := h.loadReviewInput(reviewCtx, payload)
	if err != nil {
		return h.retryOrFailover(ctx, payload, err)
	}
	result, err := h.evaluator.Evaluate(reviewCtx, payload, input)
	if err != nil {
		h.circuit.recordProviderFailure(err)
		return h.retryOrFailover(ctx, payload, err)
	}
	h.circuit.recordProviderSuccess()
	if err := h.complete(ctx, payload, result); err != nil {
		return h.retryOrFailover(ctx, payload, err)
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
	if err := llmreview.ValidateThresholdConsistency(result, input.Prompt); err != nil {
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
	if payload.IdempotencyKey != aiReviewIdempotencyKey(payload.SubmissionID, payload.RevisionID, payload.PromptConfigID, payload.PromptVersion) {
		return aiReviewPayload{}, errors.New("ai review idempotency key does not match payload anchors")
	}
	return payload, nil
}

func (h workerHandlers) markRunning(ctx context.Context, payload aiReviewPayload) (bool, error) {
	// started_at 记为本次进入 running 的时间,sweeper 用它判断 running 是否真卡死(M-06)。
	// failed → running 的重试会刷新 started_at:每次重跑都应获得一个全新的超时窗口,
	// 而不是沿用上一次失败前的开始时间,故此处是有意覆写(非取首次开始时间)。
	res, err := h.db.ExecContext(ctx,
		`UPDATE ai_reviews SET status = 'running', retry_count = retry_count + 1, started_at = ? WHERE idempotency_key = ? AND submission_id = ? AND revision_id = ? AND prompt_version = ? AND status IN ('pending','failed')`,
		time.Now().UTC(), payload.IdempotencyKey, payload.SubmissionID, payload.RevisionID, payload.PromptVersion,
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
		`UPDATE ai_reviews SET status = 'failed', error_msg = ? WHERE idempotency_key = ? AND submission_id = ? AND revision_id = ? AND prompt_version = ? AND status IN ('pending','running','failed')`,
		llmreview.SafeErrorMessage(cause), payload.IdempotencyKey, payload.SubmissionID, payload.RevisionID, payload.PromptVersion,
	)
	return err
}

func (h workerHandlers) retryOrFailover(ctx context.Context, payload aiReviewPayload, cause error) error {
	if llmreview.IsNonRetryableEvaluationError(cause) || shouldFailover(ctx) {
		if failErr := h.failover(ctx, payload, cause); failErr != nil {
			return failErr
		}
		return nil
	}
	if err := h.markFailed(ctx, payload, cause); err != nil {
		return err
	}
	return cause
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
	if !llmreview.AllowedModelName(input.Prompt.Model) {
		return aiReviewInput{}, errors.New("ai prompt model is not allowed")
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

	reviewStatus, err := lockedAIReviewStatus(ctx, tx, payload)
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
		`UPDATE ai_reviews SET status = 'succeeded', verdict = ?, overall_score = ?, dimensions = ?, reason = ?, raw_response = ?, tokens_input = ?, tokens_output = ?, latency_ms = ?, error_msg = NULL, finished_at = ? WHERE idempotency_key = ? AND submission_id = ? AND revision_id = ? AND prompt_version = ? AND status IN ('pending','running','failed')`,
		result.Verdict, result.Score, result.Dimensions, result.Reason, result.RawResponse, result.TokensInput, result.TokensOutput, result.LatencyMS, now, payload.IdempotencyKey, payload.SubmissionID, payload.RevisionID, payload.PromptVersion,
	)
	if err != nil {
		return err
	}
	if rows, _ := reviewRes.RowsAffected(); rows != 1 {
		return errors.New("ai review completion lost review update race")
	}
	// 综合判定(对齐审核流程图,三条独立分支):
	//   明确不合格(reject) → 直接打回标注员(revising);标注员修改后重提会再次过 AI 评测。
	//   可疑(uncertain) → 转人工复核(manual_review),走专属的初审入口分支。
	//   通过(pass) → 进初审(human_reviewing),等终审定夺。
	// AI 不再自动入库 / 不抽检直通——是否入库只由人工终审决定。
	toState := "human_reviewing"
	event := "ai_done"
	switch result.Verdict {
	case "reject":
		toState = "revising"
		event = "ai_reject"
	case "uncertain":
		toState = "manual_review"
		event = "ai_uncertain"
	}
	submissionSQL := `UPDATE submissions SET status = ?, ai_verdict = ?, ai_score = ? WHERE id = ? AND status = 'ai_reviewing' AND current_revision_id = ?`
	res, err := tx.ExecContext(ctx, submissionSQL,
		toState, result.Verdict, result.Score, payload.SubmissionID, payload.RevisionID,
	)
	if err != nil {
		return err
	}
	if rows, _ := res.RowsAffected(); rows != 1 {
		return errors.New("ai review completion lost submission update race")
	}
	auditPayload, _ := json.Marshal(map[string]any{"idempotency_key": payload.IdempotencyKey, "score": result.Score})
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO audit_logs (entity_type, entity_id, from_state, to_state, actor_type, event, payload, created_at) VALUES ('submission', ?, 'ai_reviewing', ?, 'system', ?, ?, ?)`,
		payload.SubmissionID, toState, event, string(auditPayload), now,
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

	reviewStatus, err := lockedAIReviewStatus(ctx, tx, payload)
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
		`UPDATE ai_reviews SET status = 'dead', error_msg = ?, finished_at = ? WHERE idempotency_key = ? AND submission_id = ? AND revision_id = ? AND prompt_version = ? AND status IN ('pending','running','failed')`,
		llmreview.SafeErrorMessage(cause), now, payload.IdempotencyKey, payload.SubmissionID, payload.RevisionID, payload.PromptVersion,
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
	auditPayload, _ := json.Marshal(map[string]any{"idempotency_key": payload.IdempotencyKey, "error": llmreview.SafeErrorMessage(cause)})
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

func lockedAIReviewStatus(ctx context.Context, tx *sql.Tx, payload aiReviewPayload) (string, error) {
	var status string
	err := tx.QueryRowContext(ctx,
		`SELECT status FROM ai_reviews WHERE idempotency_key = ? AND submission_id = ? AND revision_id = ? AND prompt_version = ? FOR UPDATE`,
		payload.IdempotencyKey, payload.SubmissionID, payload.RevisionID, payload.PromptVersion,
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

func aiReviewIdempotencyKey(submissionID uint64, revisionID uint64, promptID uint64, promptVersion int) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d:%d:%d:%d", submissionID, revisionID, promptID, promptVersion)))
	return hex.EncodeToString(sum[:])
}
