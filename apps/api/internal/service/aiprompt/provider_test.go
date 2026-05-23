package aiprompt

import (
	"context"
	"io"
	"net/http"
	"strings"
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
	if !llmreview.IsNonRetryableEvaluationError(err) {
		t.Fatalf("invalid JSON must be non-retryable, got %T %v", err, err)
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
	if !llmreview.IsNonRetryableEvaluationError(err) {
		t.Fatalf("invalid schema must be non-retryable, got %T %v", err, err)
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
	if !llmreview.IsNonRetryableEvaluationError(err) {
		t.Fatalf("threshold mismatch must be non-retryable, got %T %v", err, err)
	}

	err = llmreview.ValidateThresholdConsistency(llmreview.EvaluationResult{
		Verdict:      "reject",
		OverallScore: 95,
	}, prompt)
	if err == nil {
		t.Fatal("reject with high score must be rejected")
	}
	if !llmreview.IsNonRetryableEvaluationError(err) {
		t.Fatalf("threshold mismatch must be non-retryable, got %T %v", err, err)
	}

	err = llmreview.ValidateThresholdConsistency(llmreview.EvaluationResult{
		Verdict:      "uncertain",
		OverallScore: 75,
	}, prompt)
	if err != nil {
		t.Fatalf("consistent uncertain result rejected: %v", err)
	}
}

func TestOpenAICompatibleProviderErrorDoesNotExposeResponseBody(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusUnauthorized} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Setenv("LLM_PROVIDER", "openai")
			t.Setenv("LLM_BASE_URL", "http://provider.test")
			t.Setenv("LLM_API_KEY", "test-key")
			t.Setenv("LLM_MODEL", "test-model")
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: status,
					Body:       io.NopCloser(strings.NewReader(`provider reflected sk-test-secret`)),
					Header:     make(http.Header),
					Request:    r,
				}, nil
			})}
			provider, _, err := llmreview.NewProviderFromEnv(client)
			if err != nil {
				t.Fatalf("NewProviderFromEnv returned error: %v", err)
			}

			_, err = provider.Evaluate(context.Background(), llmreview.PromptConfig{
				PromptTemplate: "review",
				Dimensions:     []llmreview.DimensionConfig{{Name: "相关性"}},
				PassThreshold:  80,
				UncertainMin:   60,
				Model:          "test-model",
			}, llmreview.EvaluationInput{PayloadJSON: `{}`, AnswerJSON: `{}`})
			if err == nil {
				t.Fatal("provider error must be returned")
			}
			if strings.Contains(err.Error(), "sk-test-secret") || strings.Contains(err.Error(), "provider reflected") {
				t.Fatalf("provider body leaked into error: %v", err)
			}
			if !strings.Contains(err.Error(), "HTTP") {
				t.Fatalf("sanitized error should retain status context, got %v", err)
			}
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
