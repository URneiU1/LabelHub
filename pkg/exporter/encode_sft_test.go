package exporter

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// parseJSONL 把 NDJSON 输出拆成逐行对象(SFT/DPO 测试共用)。
func parseJSONL(t *testing.T, raw []byte) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			t.Fatalf("line not valid json: %v\n%s", err, line)
		}
		out = append(out, obj)
	}
	return out
}

func sftMessagesOf(t *testing.T, rec map[string]any) (string, string) {
	t.Helper()
	msgs, ok := rec["messages"].([]any)
	if !ok || len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %v", rec["messages"])
	}
	user := msgs[0].(map[string]any)
	asst := msgs[1].(map[string]any)
	if user["role"] != "user" || asst["role"] != "assistant" {
		t.Fatalf("roles = %v / %v", user["role"], asst["role"])
	}
	return user["content"].(string), asst["content"].(string)
}

func TestEncodeSFT_PromptFromPayloadCompletionFromCorrectedAnswer(t *testing.T) {
	var buf bytes.Buffer
	rows := []Row{{
		{Key: "submission_id", Value: uint64(101)},
		{Key: "item_id", Value: uint64(1)},
		{Key: "external_id", Value: "Q1"},
		{Key: "payload", Value: map[string]any{"prompt": "光合作用发生在哪？", "model_answer": "叶绿体"}},
		{Key: "answer", Value: map[string]any{"corrected_answer": "叶绿体的类囊体薄膜上", "summary": "基本正确"}},
	}}
	n, err := EncodeSFT(&buf, nil, rows)
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	recs := parseJSONL(t, buf.Bytes())
	user, asst := sftMessagesOf(t, recs[0])
	if user != "光合作用发生在哪？" {
		t.Fatalf("user content = %q", user)
	}
	// corrected_answer 优先于 summary。
	if asst != "叶绿体的类囊体薄膜上" {
		t.Fatalf("assistant content = %q", asst)
	}
	meta := recs[0]["metadata"].(map[string]any)
	if meta["submission_id"] != float64(101) || meta["item_id"] != float64(1) || meta["external_id"] != "Q1" {
		t.Fatalf("metadata = %v", meta)
	}
}

func TestEncodeSFT_CompletionFallsBackThroughCandidates(t *testing.T) {
	var buf bytes.Buffer
	rows := []Row{{
		{Key: "payload", Value: map[string]any{"prompt": "P"}},
		// 无 corrected_answer/output → 回退到 summary。
		{Key: "answer", Value: map[string]any{"corrected_answer": "", "summary": "结论"}},
	}}
	if _, err := EncodeSFT(&buf, nil, rows); err != nil {
		t.Fatalf("err=%v", err)
	}
	_, asst := sftMessagesOf(t, parseJSONL(t, buf.Bytes())[0])
	if asst != "结论" {
		t.Fatalf("expected fallback to summary, got %q", asst)
	}
}

func TestEncodeSFT_WholeObjectFallbackWhenNoCandidateKey(t *testing.T) {
	var buf bytes.Buffer
	rows := []Row{{
		{Key: "payload", Value: map[string]any{"foo": "bar"}},
		{Key: "answer", Value: map[string]any{"baz": 1}},
	}}
	if _, err := EncodeSFT(&buf, nil, rows); err != nil {
		t.Fatalf("err=%v", err)
	}
	user, asst := sftMessagesOf(t, parseJSONL(t, buf.Bytes())[0])
	if !strings.Contains(user, `"foo":"bar"`) {
		t.Fatalf("expected whole-object payload, got %q", user)
	}
	if !strings.Contains(asst, `"baz":1`) {
		t.Fatalf("expected whole-object answer, got %q", asst)
	}
}

func TestEncodeSFT_QualityProvenanceWhenReviewsIncluded(t *testing.T) {
	var buf bytes.Buffer
	rows := []Row{{
		{Key: "submission_id", Value: uint64(7)},
		{Key: "payload", Value: map[string]any{"prompt": "P"}},
		{Key: "answer", Value: map[string]any{"summary": "A"}},
		{Key: "ai_review.overall_score", Value: 88.5},
		{Key: "human_review.verdict", Value: "approved"},
	}}
	if _, err := EncodeSFT(&buf, nil, rows); err != nil {
		t.Fatalf("err=%v", err)
	}
	meta := parseJSONL(t, buf.Bytes())[0]["metadata"].(map[string]any)
	if meta["ai_overall_score"] != float64(88.5) {
		t.Fatalf("ai_overall_score = %v", meta["ai_overall_score"])
	}
	if meta["human_verdict"] != "approved" {
		t.Fatalf("human_verdict = %v", meta["human_verdict"])
	}
}

func TestEncodeSFT_StringPayloadAndNoHTMLEscape(t *testing.T) {
	var buf bytes.Buffer
	rows := []Row{{
		{Key: "payload", Value: "<b>原文</b>"},
		{Key: "answer", Value: "<i>答</i>"},
	}}
	if _, err := EncodeSFT(&buf, nil, rows); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(buf.String(), "<b>原文</b>") || !strings.Contains(buf.String(), "<i>答</i>") {
		t.Fatalf("html should not be escaped: %s", buf.String())
	}
	user, asst := sftMessagesOf(t, parseJSONL(t, buf.Bytes())[0])
	if user != "<b>原文</b>" || asst != "<i>答</i>" {
		t.Fatalf("string payload/answer should pass through, got %q / %q", user, asst)
	}
}

func TestEncode_RoutesSFTAndExtAndContentType(t *testing.T) {
	var buf bytes.Buffer
	n, err := Encode("sft", &buf, sampleCols(), sampleRows())
	if err != nil || n != 2 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if !SupportedFormat("sft") {
		t.Fatal("sft should be supported")
	}
	if FileExtension("sft") != ".sft.jsonl" {
		t.Fatalf("ext = %s", FileExtension("sft"))
	}
	if ContentType("sft") != "application/x-ndjson; charset=utf-8" {
		t.Fatalf("content-type = %s", ContentType("sft"))
	}
}

func TestEncodeSFT_EmptyRowsYieldsEmptyOutput(t *testing.T) {
	var buf bytes.Buffer
	n, err := EncodeSFT(&buf, nil, nil)
	if err != nil || n != 0 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if strings.TrimSpace(buf.String()) != "" {
		t.Fatalf("expected empty output, got %q", buf.String())
	}
}
