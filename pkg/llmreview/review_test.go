package llmreview

import (
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
