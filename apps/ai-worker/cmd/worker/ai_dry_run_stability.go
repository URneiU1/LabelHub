package main

import (
	"math"

	"labelhub.local/llmreview"
)

// dryRunAttempt 是单次重复评测的精简快照(verdict + 总分),用于 Owner UI 的逐次表格。
type dryRunAttempt struct {
	Verdict string  `json:"verdict"`
	Score   float64 `json:"score"`
}

// dryRunStability 是稳定性 dry-run 的聚合指标,序列化进 ai_dry_runs.result.stability。
//
// verdict_agreement:最高频 verdict 次数 / 成功次数(1 = 完全一致)。
// score_stddev:总分的总体标准差。
// expected_match_rate:成功次数中 verdict == 期望 的占比(无期望时省略)。
// error_rate:失败次数 / 总尝试次数。
// dimension_stddev:每个维度分数的总体标准差(仅统计每次都出现的维度)。
type dryRunStability struct {
	RepeatCount       int                `json:"repeat_count"`
	SuccessCount      int                `json:"success_count"`
	ErrorCount        int                `json:"error_count"`
	ErrorRate         float64            `json:"error_rate"`
	VerdictAgreement  float64            `json:"verdict_agreement"`
	ScoreStddev       float64            `json:"score_stddev"`
	ExpectedMatchRate *float64           `json:"expected_match_rate,omitempty"`
	DimensionStddev   map[string]float64 `json:"dimension_stddev,omitempty"`
	VerdictCounts     map[string]int     `json:"verdict_counts"`
	Runs              []dryRunAttempt    `json:"runs"`
	Errors            []string           `json:"errors,omitempty"`
}

// computeStability 由成功结果集 + 失败信息聚合稳定性指标。
func computeStability(results []llmreview.EvaluationResult, expectedVerdict string, repeatCount int, attemptErrs []string) dryRunStability {
	successCount := len(results)
	errorCount := len(attemptErrs)
	total := successCount + errorCount

	stability := dryRunStability{
		RepeatCount:   repeatCount,
		SuccessCount:  successCount,
		ErrorCount:    errorCount,
		VerdictCounts: map[string]int{},
		Runs:          make([]dryRunAttempt, 0, successCount),
		Errors:        attemptErrs,
	}
	if total > 0 {
		stability.ErrorRate = float64(errorCount) / float64(total)
	}
	if successCount == 0 {
		return stability
	}

	scores := make([]float64, 0, successCount)
	dimensionScores := map[string][]float64{}
	maxVerdictCount := 0
	matched := 0
	for _, result := range results {
		stability.VerdictCounts[result.Verdict]++
		if stability.VerdictCounts[result.Verdict] > maxVerdictCount {
			maxVerdictCount = stability.VerdictCounts[result.Verdict]
		}
		scores = append(scores, result.OverallScore)
		stability.Runs = append(stability.Runs, dryRunAttempt{Verdict: result.Verdict, Score: result.OverallScore})
		if expectedVerdict != "" && result.Verdict == expectedVerdict {
			matched++
		}
		for _, dimension := range result.Dimensions {
			dimensionScores[dimension.Name] = append(dimensionScores[dimension.Name], dimension.Score)
		}
	}

	stability.VerdictAgreement = float64(maxVerdictCount) / float64(successCount)
	stability.ScoreStddev = populationStddev(scores)
	if expectedVerdict != "" {
		rate := float64(matched) / float64(successCount)
		stability.ExpectedMatchRate = &rate
	}

	dimensionStddev := map[string]float64{}
	for name, values := range dimensionScores {
		// 只统计每次评测都出现的维度,避免缺失维度拉低标准差的可信度。
		if len(values) == successCount {
			dimensionStddev[name] = populationStddev(values)
		}
	}
	if len(dimensionStddev) > 0 {
		stability.DimensionStddev = dimensionStddev
	}
	return stability
}

// populationStddev 返回总体标准差(除以 n)。少于 2 个样本时返回 0。
func populationStddev(values []float64) float64 {
	n := len(values)
	if n < 2 {
		return 0
	}
	var sum float64
	for _, v := range values {
		sum += v
	}
	mean := sum / float64(n)
	var variance float64
	for _, v := range values {
		delta := v - mean
		variance += delta * delta
	}
	variance /= float64(n)
	return math.Sqrt(variance)
}
