package exporter

import (
	"bytes"
	"testing"
)

// prefRow 构造一条 preference_compare 标准行。
func prefRow(subID uint64, preferred string, withModels bool) Row {
	payload := map[string]any{
		"prompt":     "解释过拟合",
		"response_a": "A 的回答",
		"response_b": "B 的回答",
	}
	if withModels {
		payload["model_a"] = "doubao-pro"
		payload["model_b"] = "baseline-7b"
	}
	return Row{
		{Key: "submission_id", Value: subID},
		{Key: "item_id", Value: uint64(1)},
		{Key: "external_id", Value: "P0001"},
		{Key: "payload", Value: payload},
		{Key: "answer", Value: map[string]any{"preferred": preferred, "margin": "明显优于"}},
	}
}

func TestEncodeDPO_PreferredAMapsChosenRejected(t *testing.T) {
	var buf bytes.Buffer
	n, err := EncodeDPO(&buf, nil, []Row{prefRow(101, "A", true)})
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	rec := parseJSONL(t, buf.Bytes())[0]
	if rec["prompt"] != "解释过拟合" {
		t.Fatalf("prompt = %v", rec["prompt"])
	}
	if rec["chosen"] != "A 的回答" || rec["rejected"] != "B 的回答" {
		t.Fatalf("chosen/rejected = %v / %v", rec["chosen"], rec["rejected"])
	}
	meta := rec["metadata"].(map[string]any)
	if meta["margin"] != "明显优于" {
		t.Fatalf("margin = %v", meta["margin"])
	}
	if meta["chosen_model"] != "doubao-pro" || meta["rejected_model"] != "baseline-7b" {
		t.Fatalf("models = %v / %v", meta["chosen_model"], meta["rejected_model"])
	}
}

func TestEncodeDPO_PreferredBSwapsAndCaseInsensitive(t *testing.T) {
	var buf bytes.Buffer
	// 小写 "b" 也应识别。
	n, err := EncodeDPO(&buf, nil, []Row{prefRow(102, "b", true)})
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	rec := parseJSONL(t, buf.Bytes())[0]
	if rec["chosen"] != "B 的回答" || rec["rejected"] != "A 的回答" {
		t.Fatalf("preferred=B should swap, got chosen=%v rejected=%v", rec["chosen"], rec["rejected"])
	}
	meta := rec["metadata"].(map[string]any)
	if meta["chosen_model"] != "baseline-7b" || meta["rejected_model"] != "doubao-pro" {
		t.Fatalf("models should swap with B, got %v / %v", meta["chosen_model"], meta["rejected_model"])
	}
}

func TestEncodeDPO_TieAndUnknownSkipped(t *testing.T) {
	var buf bytes.Buffer
	rows := []Row{
		prefRow(101, "A", false),
		prefRow(102, "tie", false),
		prefRow(103, "garbage", false),
		prefRow(104, "B", false),
	}
	n, err := EncodeDPO(&buf, nil, rows)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	// 只有 A / B 两条产出有效偏好对,tie 与非法值跳过。
	if n != 2 {
		t.Fatalf("expected 2 valid pairs, got %d", n)
	}
	if got := len(parseJSONL(t, buf.Bytes())); got != 2 {
		t.Fatalf("expected 2 lines, got %d", got)
	}
}

func TestEncodeDPO_EmptyResponseSkipped(t *testing.T) {
	var buf bytes.Buffer
	row := Row{
		{Key: "submission_id", Value: uint64(1)},
		{Key: "payload", Value: map[string]any{"prompt": "p", "response_a": "A", "response_b": ""}},
		{Key: "answer", Value: map[string]any{"preferred": "A"}},
	}
	// preferred=A 但 rejected(response_b)为空 → 无法成对,跳过。
	n, err := EncodeDPO(&buf, nil, []Row{row})
	if err != nil || n != 0 {
		t.Fatalf("expected 0 (empty rejected skipped), n=%d err=%v", n, err)
	}
}

func TestEncodeDPO_NonPreferenceRowsSkippedGracefully(t *testing.T) {
	var buf bytes.Buffer
	// payload/answer 不是 map(非 preference 任务误用此格式)→ 跳过不报错。
	row := Row{
		{Key: "payload", Value: "raw string"},
		{Key: "answer", Value: "raw answer"},
	}
	n, err := EncodeDPO(&buf, nil, []Row{row})
	if err != nil || n != 0 {
		t.Fatalf("expected graceful skip, n=%d err=%v", n, err)
	}
}

func TestEncodeDPOWithConfig_CustomFieldMapping(t *testing.T) {
	var buf bytes.Buffer
	rows := []Row{{
		{Key: "submission_id", Value: uint64(1)},
		{Key: "payload", Value: map[string]any{"task": "问题Z", "ans1": "甲", "ans2": "乙"}},
		{Key: "answer", Value: map[string]any{"winner": "B"}},
	}}
	cfg := &DPOConfig{PromptField: "task", CandidateAField: "ans1", CandidateBField: "ans2", PreferredField: "winner"}
	n, err := EncodeDPOWithConfig(&buf, rows, cfg)
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	rec := parseJSONL(t, buf.Bytes())[0]
	if rec["prompt"] != "问题Z" {
		t.Fatalf("prompt should follow mapped field, got %v", rec["prompt"])
	}
	// winner=B → chosen=候选B(ans2=乙)、rejected=候选A(ans1=甲)
	if rec["chosen"] != "乙" || rec["rejected"] != "甲" {
		t.Fatalf("custom-mapped chosen/rejected = %v/%v", rec["chosen"], rec["rejected"])
	}
}

func TestEncodeDPOWithConfig_NilConfigUsesPreferenceConvention(t *testing.T) {
	var buf bytes.Buffer
	n, err := EncodeDPOWithConfig(&buf, []Row{prefRow(1, "A", false)}, nil)
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if rec := parseJSONL(t, buf.Bytes())[0]; rec["chosen"] != "A 的回答" {
		t.Fatalf("nil cfg should use preference_compare convention, got %v", rec["chosen"])
	}
}

func TestEncode_RoutesDPOAndExtAndContentType(t *testing.T) {
	var buf bytes.Buffer
	n, err := Encode("dpo", &buf, nil, []Row{prefRow(1, "A", false)})
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if !SupportedFormat("dpo") {
		t.Fatal("dpo should be supported")
	}
	if FileExtension("dpo") != ".dpo.jsonl" {
		t.Fatalf("ext = %s", FileExtension("dpo"))
	}
	if ContentType("dpo") != "application/x-ndjson; charset=utf-8" {
		t.Fatalf("content-type = %s", ContentType("dpo"))
	}
}
