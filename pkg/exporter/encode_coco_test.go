package exporter

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestEncodeCOCO_SkipsRowsWithoutItemID(t *testing.T) {
	var buf bytes.Buffer
	cols := []Column{{Source: "answer", Export: "答案"}}
	rows := []Row{
		{
			{Key: "submission_id", Value: uint64(201)},
			{Key: "item_id", Value: uint64(7)},
			{Key: "external_id", Value: "Q7"},
			{Key: "payload", Value: map[string]any{"text": "ok"}},
			{Key: "answer", Value: map[string]any{"label": "猫"}},
		},
		{
			// 缺 item_id → 必须整行跳过,绝不能产出 {"id":null} 的幽灵 image/annotation。
			{Key: "submission_id", Value: uint64(202)},
			{Key: "payload", Value: map[string]any{"text": "orphan"}},
			{Key: "answer", Value: map[string]any{"label": "狗"}},
		},
	}
	if _, err := EncodeCOCO(&buf, cols, rows); err != nil {
		t.Fatalf("err=%v", err)
	}
	doc := decodeCOCO(t, buf.Bytes())

	images := doc["images"].([]any)
	if len(images) != 1 {
		t.Fatalf("expected 1 image (item_id-less row skipped), got %d", len(images))
	}
	if images[0].(map[string]any)["id"] != float64(7) {
		t.Fatalf("image id = %v", images[0].(map[string]any)["id"])
	}
	for _, img := range images {
		if img.(map[string]any)["id"] == nil {
			t.Fatal("ghost image with id:null leaked into COCO output")
		}
	}
	annotations := doc["annotations"].([]any)
	if len(annotations) != 1 {
		t.Fatalf("expected 1 annotation (orphan row skipped), got %d", len(annotations))
	}
}

// cocoRows 模拟 LoadApprovedRows 的标准行:item 1 有两条 approved 提交(overlap),item 2 一条。
func cocoRows() []Row {
	return []Row{
		{
			{Key: "submission_id", Value: uint64(101)},
			{Key: "item_id", Value: uint64(1)},
			{Key: "external_id", Value: "Q1"},
			{Key: "payload", Value: map[string]any{"text": "<b>原文</b>"}},
			{Key: "answer", Value: map[string]any{"label": "猫"}},
		},
		{
			{Key: "submission_id", Value: uint64(102)},
			{Key: "item_id", Value: uint64(1)},
			{Key: "external_id", Value: "Q1"},
			{Key: "payload", Value: map[string]any{"text": "<b>原文</b>"}},
			{Key: "answer", Value: map[string]any{"label": "狗"}},
		},
		{
			{Key: "submission_id", Value: uint64(103)},
			{Key: "item_id", Value: uint64(2)},
			{Key: "external_id", Value: nil},
			{Key: "payload", Value: "raw"},
			{Key: "answer", Value: json.Number("9007199254740993")},
		},
	}
}

func decodeCOCO(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("output not valid json: %v\n%s", err, raw)
	}
	return doc
}

func TestEncodeCOCO_ImagesDedupedAnnotationsPerSubmission(t *testing.T) {
	var buf bytes.Buffer
	cols := []Column{{Source: "answer", Export: "答案"}}
	n, err := EncodeCOCO(&buf, cols, cocoRows())
	if err != nil || n != 3 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	doc := decodeCOCO(t, buf.Bytes())

	images := doc["images"].([]any)
	if len(images) != 2 {
		t.Fatalf("expected 2 deduped images, got %d", len(images))
	}
	first := images[0].(map[string]any)
	if first["file_name"] != "Q1" {
		t.Fatalf("file_name = %v", first["file_name"])
	}
	second := images[1].(map[string]any)
	if second["file_name"] != "item-2" {
		t.Fatalf("missing external_id should fall back to item-<id>, got %v", second["file_name"])
	}

	annotations := doc["annotations"].([]any)
	if len(annotations) != 3 {
		t.Fatalf("expected 3 annotations, got %d", len(annotations))
	}
	ann := annotations[0].(map[string]any)
	if ann["category_id"] != float64(1) || ann["image_id"] != float64(1) {
		t.Fatalf("annotation = %v", ann)
	}
	attrs := ann["attributes"].(map[string]any)
	if attrs["答案"].(map[string]any)["label"] != "猫" {
		t.Fatalf("attributes should apply field map, got %v", attrs)
	}

	categories := doc["categories"].([]any)
	if len(categories) != 1 {
		t.Fatalf("expected single generic category, got %v", categories)
	}
}

func TestEncodeCOCO_NoHTMLEscapeAndBigIntPreserved(t *testing.T) {
	var buf bytes.Buffer
	cols := []Column{{Source: "answer", Export: "answer"}}
	if _, err := EncodeCOCO(&buf, cols, cocoRows()); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(buf.String(), "<b>原文</b>") {
		t.Fatalf("expected unescaped html in payload, got %s", buf.String())
	}
	// json.Number 大整数不坍塌(沿用 S3 教训)。
	if !strings.Contains(buf.String(), "9007199254740993") {
		t.Fatalf("expected big int preserved, got %s", buf.String())
	}
}

func TestEncodeCOCO_EmptyRowsYieldsValidEmptyDoc(t *testing.T) {
	var buf bytes.Buffer
	n, err := EncodeCOCO(&buf, sampleCols(), nil)
	if err != nil || n != 0 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	doc := decodeCOCO(t, buf.Bytes())
	if len(doc["images"].([]any)) != 0 || len(doc["annotations"].([]any)) != 0 {
		t.Fatalf("expected empty arrays, got %s", buf.String())
	}
}

func TestEncode_RoutesCOCO(t *testing.T) {
	var buf bytes.Buffer
	n, err := Encode("coco", &buf, sampleCols(), sampleRows())
	if err != nil || n != 2 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if !SupportedFormat("coco") {
		t.Fatal("coco should be a supported format")
	}
}

func TestCOCOFileExtensionAndContentType(t *testing.T) {
	if FileExtension("coco") != ".coco.json" {
		t.Fatalf("ext = %s", FileExtension("coco"))
	}
	if ContentType("coco") != "application/json; charset=utf-8" {
		t.Fatalf("content-type = %s", ContentType("coco"))
	}
}
