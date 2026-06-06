package main

import (
	"context"
	"database/sql/driver"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"go.uber.org/zap"
	"labelhub.local/llmreview"
)

func eval(verdict string, score float64, dims ...llmreview.DimensionResult) llmreview.EvaluationResult {
	return llmreview.EvaluationResult{Verdict: verdict, OverallScore: score, Dimensions: dims}
}

func TestComputeStability_AllAgree(t *testing.T) {
	results := []llmreview.EvaluationResult{eval("pass", 80), eval("pass", 80), eval("pass", 80)}
	s := computeStability(results, "pass", 3, nil)
	if s.VerdictAgreement != 1 {
		t.Fatalf("verdict_agreement = %v, want 1", s.VerdictAgreement)
	}
	if s.ScoreStddev != 0 {
		t.Fatalf("score_stddev = %v, want 0", s.ScoreStddev)
	}
	if s.ErrorRate != 0 {
		t.Fatalf("error_rate = %v, want 0", s.ErrorRate)
	}
	if s.ExpectedMatchRate == nil || *s.ExpectedMatchRate != 1 {
		t.Fatalf("expected_match_rate = %v, want 1", s.ExpectedMatchRate)
	}
	if len(s.Runs) != 3 {
		t.Fatalf("runs = %d, want 3", len(s.Runs))
	}
}

func TestComputeStability_MixedVerdicts(t *testing.T) {
	results := []llmreview.EvaluationResult{eval("pass", 90), eval("pass", 70), eval("reject", 40)}
	s := computeStability(results, "pass", 3, nil)
	// 最高频 verdict 是 pass(2 次)/ 3 = 0.667。
	if got := s.VerdictAgreement; got < 0.66 || got > 0.67 {
		t.Fatalf("verdict_agreement = %v, want ~0.667", got)
	}
	if s.ScoreStddev <= 0 {
		t.Fatalf("score_stddev = %v, want > 0", s.ScoreStddev)
	}
	if s.ExpectedMatchRate == nil || *s.ExpectedMatchRate < 0.66 || *s.ExpectedMatchRate > 0.67 {
		t.Fatalf("expected_match_rate = %v, want ~0.667", s.ExpectedMatchRate)
	}
	if s.VerdictCounts["pass"] != 2 || s.VerdictCounts["reject"] != 1 {
		t.Fatalf("verdict_counts = %v", s.VerdictCounts)
	}
}

func TestComputeStability_WithErrorsKeepsSuccesses(t *testing.T) {
	results := []llmreview.EvaluationResult{eval("pass", 80), eval("pass", 80)}
	s := computeStability(results, "pass", 4, []string{"timeout", "5xx"})
	if s.SuccessCount != 2 || s.ErrorCount != 2 {
		t.Fatalf("success/error = %d/%d, want 2/2", s.SuccessCount, s.ErrorCount)
	}
	if s.ErrorRate != 0.5 {
		t.Fatalf("error_rate = %v, want 0.5", s.ErrorRate)
	}
	// 成功结果仍参与一致率统计,不因部分错误而丢失。
	if s.VerdictAgreement != 1 {
		t.Fatalf("verdict_agreement = %v, want 1", s.VerdictAgreement)
	}
	if len(s.Errors) != 2 {
		t.Fatalf("errors = %v", s.Errors)
	}
}

func TestComputeStability_DimensionStddev(t *testing.T) {
	results := []llmreview.EvaluationResult{
		eval("pass", 80, llmreview.DimensionResult{Name: "相关性", Score: 90}),
		eval("pass", 80, llmreview.DimensionResult{Name: "相关性", Score: 70}),
	}
	s := computeStability(results, "", 2, nil)
	if _, ok := s.DimensionStddev["相关性"]; !ok {
		t.Fatalf("dimension_stddev missing 相关性: %v", s.DimensionStddev)
	}
	if s.DimensionStddev["相关性"] <= 0 {
		t.Fatalf("dimension stddev = %v, want > 0", s.DimensionStddev["相关性"])
	}
	// 无期望 verdict 时不产出 expected_match_rate。
	if s.ExpectedMatchRate != nil {
		t.Fatalf("expected_match_rate should be nil without expected verdict")
	}
}

func TestComputeStability_AllFailed(t *testing.T) {
	s := computeStability(nil, "pass", 3, []string{"a", "b", "c"})
	if s.SuccessCount != 0 || s.ErrorCount != 3 {
		t.Fatalf("success/error = %d/%d, want 0/3", s.SuccessCount, s.ErrorCount)
	}
	if s.ErrorRate != 1 {
		t.Fatalf("error_rate = %v, want 1", s.ErrorRate)
	}
}

// jsonContains 是 sqlmock.Argument:断言字符串参数包含某子串(用于检查 result JSON 里有稳定性指标)。
type jsonContains struct{ needle string }

func (m jsonContains) Match(v driver.Value) bool {
	s, ok := v.(string)
	return ok && strings.Contains(s, m.needle)
}

// cyclingEvaluator 每次调用返回列表里的下一个 (verdict, score),用于模拟重复评测的波动。
type cyclingEvaluator struct {
	calls    *int
	verdicts []string
	scores   []float64
}

func (e cyclingEvaluator) Evaluate(ctx context.Context, payload aiReviewPayload, input aiReviewInput) (aiEvaluation, error) {
	i := *e.calls
	*e.calls++
	idx := i % len(e.verdicts)
	return aiEvaluation{
		Verdict:     e.verdicts[idx],
		Score:       e.scores[idx],
		Dimensions:  `[]`,
		RawResponse: `{"provider":"mock","model":"test-model"}`,
		LatencyMS:   1,
	}, nil
}

func TestHandleAIDryRunRepeatedAggregatesStability(t *testing.T) {
	t.Setenv("LLM_ALLOWED_MODELS", "test-model")
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectExec(`(?is)^UPDATE ai_dry_runs SET status = 'running'`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?is)^SELECT dr.task_id, dr.ai_prompt_id, dr.prompt_version`).
		WillReturnRows(sqlmock.NewRows([]string{
			"task_id", "ai_prompt_id", "prompt_version", "payload_snapshot", "expected_answer_snapshot", "expected_verdict",
			"baseline_description", "prompt_template", "dimensions", "pass_threshold", "uncertain_min", "model",
		}).AddRow(
			1, 33, 3, `{"text":"a"}`, `{"label":"ok"}`, "pass",
			"baseline", "review {{answer.label}}", `[{"name":"相关性"}]`, 80, 60, "test-model",
		))
	// result JSON 必须带稳定性聚合;3 次评测里 2 次 pass、1 次 reject。
	mock.ExpectExec(`(?is)^UPDATE ai_dry_runs SET status = 'succeeded'.+actual_verdict = \?.+matched_expected = \?.+error_msg = NULL`).
		WithArgs(jsonContains{"verdict_agreement"}, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	calls := 0
	evaluator := cyclingEvaluator{calls: &calls, verdicts: []string{"pass", "reject", "pass"}, scores: []float64{85, 40, 80}}
	handler := workerHandlers{db: db, logger: zap.NewNop(), evaluator: evaluator, circuit: newAIWorkerCircuit(0, time.Minute)}
	if err := handler.handleAIDryRun(context.Background(), newAIDryRunTask([]byte(`{"run_id":44,"repeat_count":3}`))); err != nil {
		t.Fatalf("handleAIDryRun returned error: %v", err)
	}
	if calls != 3 {
		t.Fatalf("evaluator called %d times, want 3", calls)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}
