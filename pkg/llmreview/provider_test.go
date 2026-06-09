package llmreview_test

import (
	"context"
	"encoding/json"
	"fmt"
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

func TestOpenAICompatibleProviderRetriesRateLimit(t *testing.T) {
	t.Setenv("LLM_PROVIDER", "openai")
	t.Setenv("LLM_BASE_URL", "http://provider.test")
	t.Setenv("LLM_API_KEY", "test-key")
	t.Setenv("LLM_MODEL", "test-model")
	t.Setenv("LLM_RETRY_MAX_ATTEMPTS", "2")
	t.Setenv("LLM_RETRY_BACKOFF_MS", "0")
	t.Setenv("LLM_RETRY_MAX_BACKOFF_MS", "0")

	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Body:       io.NopCloser(strings.NewReader(`provider reflected sk-test-secret`)),
				Header:     http.Header{"Retry-After": []string{"0"}},
				Request:    r,
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(providerSuccessResponse(t))),
			Header:     make(http.Header),
			Request:    r,
		}, nil
	})}
	provider, _, err := llmreview.NewProviderFromEnv(client)
	if err != nil {
		t.Fatalf("NewProviderFromEnv returned error: %v", err)
	}

	result, err := provider.Evaluate(context.Background(), llmreview.PromptConfig{
		PromptTemplate: "review",
		Dimensions:     []llmreview.DimensionConfig{{Name: "相关性"}},
		PassThreshold:  80,
		UncertainMin:   60,
		Model:          "test-model",
	}, llmreview.EvaluationInput{PayloadJSON: `{}`, AnswerJSON: `{}`})
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if calls != 2 {
		t.Fatalf("rate limit should be retried once, got %d calls", calls)
	}
	if result.Verdict != llmreview.VerdictPass || result.OverallScore != 86 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestOpenAICompatibleProviderErrorDoesNotExposeResponseBody(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusUnauthorized} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Setenv("LLM_PROVIDER", "openai")
			t.Setenv("LLM_BASE_URL", "http://provider.test")
			t.Setenv("LLM_API_KEY", "test-key")
			t.Setenv("LLM_MODEL", "test-model")
			calls := 0
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
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
			if calls != 1 {
				t.Fatalf("non-retryable HTTP %d should not retry, got %d calls", status, calls)
			}
		})
	}
}

func TestConfigFromEnvRejectsInvalidRetryConfig(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{
			name: "zero attempts",
			env:  map[string]string{"LLM_RETRY_MAX_ATTEMPTS": "0"},
			want: "LLM_RETRY_MAX_ATTEMPTS",
		},
		{
			name: "too many attempts",
			env:  map[string]string{"LLM_RETRY_MAX_ATTEMPTS": "6"},
			want: "LLM_RETRY_MAX_ATTEMPTS",
		},
		{
			name: "negative base backoff",
			env:  map[string]string{"LLM_RETRY_BACKOFF_MS": "-1"},
			want: "LLM_RETRY_BACKOFF_MS",
		},
		{
			name: "max below base",
			env: map[string]string{
				"LLM_RETRY_BACKOFF_MS":     "300",
				"LLM_RETRY_MAX_BACKOFF_MS": "200",
			},
			want: "LLM_RETRY_MAX_BACKOFF_MS",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("LLM_PROVIDER", "openai")
			t.Setenv("LLM_BASE_URL", "http://provider.test")
			t.Setenv("LLM_API_KEY", "test-key")
			t.Setenv("LLM_MODEL", "test-model")
			for key, value := range tt.env {
				t.Setenv(key, value)
			}

			_, err := llmreview.ConfigFromEnv()
			if err == nil {
				t.Fatal("invalid retry config must be rejected")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error %q should mention %q", err.Error(), tt.want)
			}
		})
	}
}

func providerSuccessResponse(t *testing.T) string {
	t.Helper()
	args, err := json.Marshal(`{
		"verdict":"pass",
		"overall_score":86,
		"dimensions":[{"name":"相关性","score":86,"reason":"matches"}],
		"reason":"answer passes"
	}`)
	if err != nil {
		t.Fatalf("marshal tool arguments: %v", err)
	}
	return fmt.Sprintf(`{
		"model":"test-model",
		"choices":[{"message":{"tool_calls":[{"function":{"name":"submit_ai_review","arguments":%s}}]}}],
		"usage":{"prompt_tokens":12,"completion_tokens":7}
	}`, args)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
