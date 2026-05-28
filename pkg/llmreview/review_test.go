package llmreview

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestBuildMessagesPreservesLargeJSONNumberSpelling(t *testing.T) {
	messages := buildMessages(PromptConfig{
		PromptTemplate: "review {{payload.external_id}} {{answer.selected_id}}",
		Dimensions:     []DimensionConfig{{Name: "相关性"}},
		PassThreshold:  80,
		UncertainMin:   60,
	}, EvaluationInput{
		PayloadJSON: `{"external_id":9007199254740993123}`,
		AnswerJSON:  `{"selected_id":9007199254740993124}`,
	})

	if len(messages) < 2 {
		t.Fatalf("messages len = %d, want at least 2", len(messages))
	}
	content := messages[1].Content
	if !strings.Contains(content, "9007199254740993123") || !strings.Contains(content, "9007199254740993124") {
		t.Fatalf("message lost raw integer spelling: %s", content)
	}
	if strings.Contains(content, "9007199254740993000") || strings.Contains(content, "9.007199254740993") {
		t.Fatalf("message contains float64-rounded number: %s", content)
	}
}

func TestIsProviderHTTP5xxOnlyMatchesRetryableServerErrors(t *testing.T) {
	if !IsProviderHTTP5xx(retryableError{err: errors.New("bad gateway"), statusCode: http.StatusBadGateway}) {
		t.Fatal("HTTP 5xx retryable error should trip worker circuit")
	}
	if IsProviderHTTP5xx(retryableError{err: errors.New("rate limited"), statusCode: http.StatusTooManyRequests}) {
		t.Fatal("HTTP 429 should remain retryable without tripping the 5xx circuit")
	}
	if IsProviderHTTP5xx(errors.New("plain failure")) {
		t.Fatal("plain errors must not trip the 5xx circuit")
	}
}
