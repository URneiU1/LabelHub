package llmreview

import (
	"encoding/json"
	"strings"
	"testing"
)

func snapshotFixture() (PromptConfig, EvaluationInput) {
	prompt := PromptConfig{
		PromptTemplate: "评估这道题的标注质量 {{payload.prompt}}",
		Dimensions:     []DimensionConfig{{Name: "准确性"}, {Name: "完整性"}},
		PassThreshold:  80,
		UncertainMin:   60,
		Model:          "mock-model",
	}
	input := EvaluationInput{
		PayloadJSON:         `{"prompt":"1+1=?"}`,
		AnswerJSON:          `{"answer":"2"}`,
		BaselineDescription: "基线说明",
	}
	return prompt, input
}

func TestRenderPromptSnapshot_ValidMessagesWithRenderedData(t *testing.T) {
	prompt, input := snapshotFixture()
	snap, err := RenderPromptSnapshot(prompt, input)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	var msgs []chatMessage
	if err := json.Unmarshal([]byte(snap), &msgs); err != nil {
		t.Fatalf("snapshot not valid messages json: %v\n%s", err, snap)
	}
	if len(msgs) != 2 || msgs[0].Role != "system" || msgs[1].Role != "user" {
		t.Fatalf("unexpected messages shape: %v", msgs)
	}
	// user content 必须包含渲染进去的 payload / answer / baseline,证明快照即 AI 实际所见。
	for _, want := range []string{"1+1=?", `"answer":"2"`, "基线说明", "准确性"} {
		if !strings.Contains(msgs[1].Content, want) {
			t.Fatalf("snapshot user content missing %q:\n%s", want, msgs[1].Content)
		}
	}
}

func TestRenderPromptSnapshot_EqualsSerializedBuildMessages(t *testing.T) {
	prompt, input := snapshotFixture()
	snap, err := RenderPromptSnapshot(prompt, input)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	want, err := json.Marshal(buildMessages(prompt, input))
	if err != nil {
		t.Fatalf("marshal buildMessages: %v", err)
	}
	// 快照必须与真正发送的 buildMessages 字节级一致(snapshot == 真相)。
	if snap != string(want) {
		t.Fatalf("snapshot diverged from buildMessages\n got: %s\nwant: %s", snap, want)
	}
}
