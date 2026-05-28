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

type aiDryRunPayload struct {
	RunID uint64 `json:"run_id"`
}

type aiDryRunInput struct {
	TaskID              uint64
	ExpectedVerdict     string
	PayloadJSON         string
	AnswerJSON          string
	BaselineDescription string
	Prompt              llmreview.PromptConfig
}

func (h workerHandlers) handleAIDryRun(ctx context.Context, t *asynq.Task) error {
	payload, err := parseAIDryRunPayload(t.Payload())
	if err != nil {
		h.logger.Warn("invalid ai dry-run payload", zap.Error(err), zap.ByteString("payload", t.Payload()))
		return nil
	}
	claimed, err := h.markAIDryRunRunning(ctx, payload.RunID)
	if err != nil {
		return err
	}
	if !claimed {
		h.logger.Info("ai dry-run already finalized or claimed elsewhere", zap.Uint64("run_id", payload.RunID))
		return nil
	}
	if err := h.circuit.check(); err != nil {
		h.logger.Warn("ai dry-run provider circuit open", zap.Uint64("run_id", payload.RunID), zap.Error(err))
		return h.failAIDryRun(ctx, payload.RunID, err)
	}

	reviewCtx, cancel := context.WithTimeout(ctx, aiReviewTimeout())
	defer cancel()
	input, err := h.loadAIDryRunInput(reviewCtx, payload.RunID)
	if err != nil {
		return h.failAIDryRun(ctx, payload.RunID, err)
	}
	provider, _, err := llmreview.NewProviderFromEnv(nil)
	var result llmreview.EvaluationResult
	if err == nil {
		result, err = provider.Evaluate(reviewCtx, input.Prompt, llmreview.EvaluationInput{
			TaskID:              input.TaskID,
			PromptConfigID:      input.Prompt.ID,
			PromptVersion:       input.Prompt.Version,
			PayloadJSON:         input.PayloadJSON,
			AnswerJSON:          input.AnswerJSON,
			BaselineDescription: input.BaselineDescription,
		})
	}
	if err == nil {
		err = llmreview.ValidateThresholdConsistency(result, input.Prompt)
	}
	if err != nil {
		h.circuit.recordProviderFailure(err)
		return h.failAIDryRun(ctx, payload.RunID, err)
	}
	h.circuit.recordProviderSuccess()
	if err := h.succeedAIDryRun(ctx, payload.RunID, input.ExpectedVerdict, result); err != nil {
		return err
	}
	h.logger.Info("ai dry-run completed",
		zap.Uint64("run_id", payload.RunID),
		zap.String("verdict", result.Verdict),
		zap.Float64("score", result.OverallScore),
	)
	return nil
}

func parseAIDryRunPayload(raw []byte) (aiDryRunPayload, error) {
	var payload aiDryRunPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return aiDryRunPayload{}, err
	}
	if payload.RunID == 0 {
		return aiDryRunPayload{}, errors.New("missing required ai dry-run payload fields")
	}
	return payload, nil
}

func (h workerHandlers) markAIDryRunRunning(ctx context.Context, runID uint64) (bool, error) {
	res, err := h.db.ExecContext(ctx,
		`UPDATE ai_dry_runs SET status = 'running' WHERE id = ? AND status = 'queued'`,
		runID,
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

func (h workerHandlers) loadAIDryRunInput(ctx context.Context, runID uint64) (aiDryRunInput, error) {
	var input aiDryRunInput
	var dimensionsRaw string
	err := h.db.QueryRowContext(ctx, `
SELECT dr.task_id, dr.ai_prompt_id, dr.prompt_version, COALESCE(dr.payload_snapshot, ''), COALESCE(dr.expected_answer_snapshot, ''), COALESCE(dr.expected_verdict, ''), COALESCE(t.baseline_description, ''), cfg.prompt_template, cfg.dimensions, cfg.pass_threshold, cfg.uncertain_min, cfg.model
FROM ai_dry_runs dr
JOIN tasks t ON t.id = dr.task_id
JOIN ai_prompt_configs cfg ON cfg.id = dr.ai_prompt_id AND cfg.task_id = dr.task_id AND cfg.version = dr.prompt_version
WHERE dr.id = ?`,
		runID,
	).Scan(
		&input.TaskID,
		&input.Prompt.ID,
		&input.Prompt.Version,
		&input.PayloadJSON,
		&input.AnswerJSON,
		&input.ExpectedVerdict,
		&input.BaselineDescription,
		&input.Prompt.PromptTemplate,
		&dimensionsRaw,
		&input.Prompt.PassThreshold,
		&input.Prompt.UncertainMin,
		&input.Prompt.Model,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return aiDryRunInput{}, errors.New("ai dry-run input not found")
		}
		return aiDryRunInput{}, err
	}
	if !llmreview.AllowedModelName(input.Prompt.Model) {
		return aiDryRunInput{}, errors.New("ai prompt model is not allowed")
	}
	dimensions, err := llmreview.ParseDimensions(dimensionsRaw)
	if err != nil {
		return aiDryRunInput{}, err
	}
	input.Prompt.Dimensions = dimensions
	return input, nil
}

func (h workerHandlers) failAIDryRun(ctx context.Context, runID uint64, cause error) error {
	_, err := h.db.ExecContext(ctx,
		`UPDATE ai_dry_runs SET status = 'failed', error_msg = ?, finished_at = ? WHERE id = ? AND status IN ('queued','running')`,
		llmreview.SafeErrorMessage(cause), time.Now().UTC(), runID,
	)
	return err
}

func (h workerHandlers) succeedAIDryRun(ctx context.Context, runID uint64, expectedVerdict string, result llmreview.EvaluationResult) error {
	raw, err := json.Marshal(result)
	if err != nil {
		return h.failAIDryRun(ctx, runID, errors.New("failed to serialize dry-run result"))
	}
	matchedExpected := result.Verdict == expectedVerdict
	_, err = h.db.ExecContext(ctx,
		`UPDATE ai_dry_runs SET status = 'succeeded', result = ?, actual_verdict = ?, matched_expected = ?, error_msg = NULL, finished_at = ? WHERE id = ? AND status = 'running'`,
		string(raw), result.Verdict, matchedExpected, time.Now().UTC(), runID,
	)
	return err
}
