package aiprompt

import (
	"testing"

	"labelhub.local/llmreview"
)

func TestParseEvaluationArgumentsAcceptsStructuredResult(t *testing.T) {
	result, err := llmreview.ParseEvaluationArguments([]byte(`{
		"verdict":"pass",
		"overall_score":88,
		"dimensions":[{"name":"相关性","score":90,"reason":"matches the payload"}],
		"reason":"answer is complete"
	}`), []string{"相关性"})
	if err != nil {
		t.Fatalf("ParseEvaluationArguments returned error: %v", err)
	}
	if result.Verdict != "pass" || result.OverallScore != 88 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestParseEvaluationArgumentsRejectsInvalidJSON(t *testing.T) {
	_, err := llmreview.ParseEvaluationArguments([]byte(`{"verdict":`), []string{"相关性"})
	if err == nil {
		t.Fatal("invalid JSON must be rejected")
	}
}

func TestParseEvaluationArgumentsRejectsInvalidSchema(t *testing.T) {
	_, err := llmreview.ParseEvaluationArguments([]byte(`{
		"verdict":"approve",
		"overall_score":101,
		"dimensions":[],
		"reason":""
	}`), []string{"相关性"})
	if err == nil {
		t.Fatal("invalid enum/range/empty fields must be rejected")
	}
}

func TestParseEvaluationArgumentsRequiresEveryConfiguredDimensionOnce(t *testing.T) {
	_, err := llmreview.ParseEvaluationArguments([]byte(`{
		"verdict":"uncertain",
		"overall_score":75,
		"dimensions":[{"name":"相关性","score":75,"reason":"ok"}],
		"reason":"needs human review"
	}`), []string{"相关性", "准确性"})
	if err == nil {
		t.Fatal("missing configured dimension must be rejected")
	}

	_, err = llmreview.ParseEvaluationArguments([]byte(`{
		"verdict":"uncertain",
		"overall_score":75,
		"dimensions":[
			{"name":"相关性","score":75,"reason":"ok"},
			{"name":"相关性","score":75,"reason":"duplicate"}
		],
		"reason":"needs human review"
	}`), []string{"相关性"})
	if err == nil {
		t.Fatal("duplicate dimension must be rejected")
	}
}

func TestValidateThresholdConsistencyRejectsMismatchedVerdict(t *testing.T) {
	prompt := llmreview.PromptConfig{PassThreshold: 80, UncertainMin: 60}

	err := llmreview.ValidateThresholdConsistency(llmreview.EvaluationResult{
		Verdict:      "pass",
		OverallScore: 10,
	}, prompt)
	if err == nil {
		t.Fatal("pass with low score must be rejected")
	}

	err = llmreview.ValidateThresholdConsistency(llmreview.EvaluationResult{
		Verdict:      "reject",
		OverallScore: 95,
	}, prompt)
	if err == nil {
		t.Fatal("reject with high score must be rejected")
	}

	err = llmreview.ValidateThresholdConsistency(llmreview.EvaluationResult{
		Verdict:      "uncertain",
		OverallScore: 75,
	}, prompt)
	if err != nil {
		t.Fatalf("consistent uncertain result rejected: %v", err)
	}
}
