package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"go.uber.org/zap"
	"labelhub.local/llmreview"
)

// 稳定性 dry-run 的重复次数:默认 1(普通 smoke),最多 maxDryRunRepeatCount。
// 服务端常量(跨环境不变),并在 worker 侧再夹一次防止被构造的 payload 放大 LLM 调用成本。
const (
	defaultDryRunRepeatCount = 1
	maxDryRunRepeatCount     = 5
)

type aiDryRunPayload struct {
	RunID       uint64 `json:"run_id"`
	RepeatCount int    `json:"repeat_count,omitempty"`
}

func clampDryRunRepeatCount(n int) int {
	if n <= 0 {
		return defaultDryRunRepeatCount
	}
	if n > maxDryRunRepeatCount {
		return maxDryRunRepeatCount
	}
	return n
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

	// 稳定性评测:对同一 golden sample 重复评测 repeat 次,聚合 verdict 一致率 /
	// 分数标准差 / 期望命中率 / 错误率。每次用独立 idempotency key,避免上游网关按 key 去重
	// 掩盖真实波动。只要有一次成功就算成功(部分错误进 result.details),全失败才算失败。
	repeat := clampDryRunRepeatCount(payload.RepeatCount)
	var (
		results     []llmreview.EvaluationResult
		attemptErrs []string
	)
	for attempt := 0; attempt < repeat; attempt++ {
		evaluation, evalErr := h.evaluator.Evaluate(reviewCtx, dryRunAIReviewPayload(payload.RunID, attempt, input), aiReviewInput{
			Prompt:              input.Prompt,
			AnswerJSON:          input.AnswerJSON,
			PayloadJSON:         input.PayloadJSON,
			BaselineDescription: input.BaselineDescription,
		})
		if evalErr != nil {
			h.circuit.recordProviderFailure(evalErr)
			attemptErrs = append(attemptErrs, llmreview.SafeErrorMessage(evalErr))
			continue
		}
		result, convErr := evaluationResultFromAI(evaluation)
		if convErr != nil {
			attemptErrs = append(attemptErrs, llmreview.SafeErrorMessage(convErr))
			continue
		}
		h.circuit.recordProviderSuccess()
		results = append(results, result)
	}
	if len(results) == 0 {
		return h.failAIDryRun(ctx, payload.RunID, dryRunAllAttemptsFailed(attemptErrs))
	}

	var stability *dryRunStability
	if repeat > 1 {
		s := computeStability(results, input.ExpectedVerdict, repeat, attemptErrs)
		stability = &s
	}
	representative := results[0]
	if err := h.succeedAIDryRun(ctx, payload.RunID, input.ExpectedVerdict, representative, stability); err != nil {
		return err
	}
	h.logger.Info("ai dry-run completed",
		zap.Uint64("run_id", payload.RunID),
		zap.Int("repeat", repeat),
		zap.Int("succeeded", len(results)),
		zap.String("verdict", representative.Verdict),
		zap.Float64("score", representative.OverallScore),
	)
	return nil
}

func dryRunAllAttemptsFailed(errs []string) error {
	if len(errs) > 0 {
		return errors.New(errs[0])
	}
	return errors.New("ai dry-run produced no result")
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

func dryRunAIReviewPayload(runID uint64, attempt int, input aiDryRunInput) aiReviewPayload {
	return aiReviewPayload{
		TaskID:         input.TaskID,
		PromptConfigID: input.Prompt.ID,
		PromptVersion:  input.Prompt.Version,
		IdempotencyKey: fmt.Sprintf("dry-run:%d:%d", runID, attempt),
	}
}

func evaluationResultFromAI(evaluation aiEvaluation) (llmreview.EvaluationResult, error) {
	var dimensions []llmreview.DimensionResult
	if err := json.Unmarshal([]byte(evaluation.Dimensions), &dimensions); err != nil {
		return llmreview.EvaluationResult{}, err
	}
	result := llmreview.EvaluationResult{
		Verdict:      evaluation.Verdict,
		OverallScore: evaluation.Score,
		Dimensions:   dimensions,
		Reason:       evaluation.Reason,
		TokensInput:  evaluation.TokensInput,
		TokensOutput: evaluation.TokensOutput,
		LatencyMS:    evaluation.LatencyMS,
		RawResponse:  evaluation.RawResponse,
	}
	var raw struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
	}
	if err := json.Unmarshal([]byte(evaluation.RawResponse), &raw); err == nil {
		result.Provider = raw.Provider
		result.Model = raw.Model
	}
	return result, nil
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

// dryRunResultEnvelope 序列化进 ai_dry_runs.result:内嵌 EvaluationResult(字段提升到顶层,
// 保持单次 dry-run 的既有形状向后兼容),repeat>1 时附带 stability 聚合指标。
type dryRunResultEnvelope struct {
	llmreview.EvaluationResult
	Stability *dryRunStability `json:"stability,omitempty"`
}

func (h workerHandlers) succeedAIDryRun(ctx context.Context, runID uint64, expectedVerdict string, result llmreview.EvaluationResult, stability *dryRunStability) error {
	raw, err := json.Marshal(dryRunResultEnvelope{EvaluationResult: result, Stability: stability})
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
