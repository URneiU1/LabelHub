package exporter

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func sampleCols() []Column {
	return []Column{{Source: "external_id", Export: "题目ID"}, {Source: "answer", Export: "答案"}, {Source: "ai_review.verdict", Export: "AI判定"}}
}

func sampleRows() []Row {
	return []Row{
		{{Key: "external_id", Value: "Q1"}, {Key: "answer", Value: map[string]any{"label": "猫"}}, {Key: "ai_review.verdict", Value: "pass"}},
		{{Key: "external_id", Value: "Q2"}, {Key: "answer", Value: "<b>x</b>"}, {Key: "ai_review.verdict", Value: json.Number("2")}},
	}
}

func TestEncodeJSON_ArrayShapeAndOrder(t *testing.T) {
	var buf bytes.Buffer
	n, err := EncodeJSON(&buf, sampleCols(), sampleRows())
	if err != nil || n != 2 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	var arr []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &arr); err != nil {
		t.Fatalf("output not valid json array: %v\n%s", err, buf.String())
	}
	if len(arr) != 2 || arr[0]["题目ID"] != "Q1" || arr[0]["AI判定"] != "pass" {
		t.Fatalf("unexpected json: %s", buf.String())
	}
	// HTML 转义关闭: <b> 应原样而非 <
	if !strings.Contains(buf.String(), "<b>x</b>") {
		t.Fatalf("expected unescaped html, got %s", buf.String())
	}
	// 列顺序: 题目ID 在 答案 之前
	if strings.Index(buf.String(), "题目ID") > strings.Index(buf.String(), "答案") {
		t.Fatal("column order not preserved")
	}
}

func TestEncodeJSON_EmptyRowsYieldsEmptyArray(t *testing.T) {
	var buf bytes.Buffer
	n, err := EncodeJSON(&buf, sampleCols(), nil)
	if err != nil || n != 0 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if strings.TrimSpace(buf.String()) != "[]" {
		t.Fatalf("expected [], got %q", buf.String())
	}
}

func TestEncodeJSONL_OneObjectPerLineAndCount(t *testing.T) {
	var buf bytes.Buffer
	n, err := EncodeJSONL(&buf, sampleCols(), sampleRows())
	if err != nil || n != 2 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %q", len(lines), buf.String())
	}
	for _, line := range lines {
		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			t.Fatalf("line not valid json: %q", line)
		}
	}
	if !strings.HasSuffix(buf.String(), "\n") {
		t.Fatal("jsonl must end with newline")
	}
}

func TestEncodeCSV_HeaderBOMAndNestedStringified(t *testing.T) {
	var buf bytes.Buffer
	n, err := EncodeCSV(&buf, sampleCols(), sampleRows())
	if err != nil || n != 2 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if !bytes.HasPrefix(buf.Bytes(), []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatal("csv must start with UTF-8 BOM")
	}
	body := string(buf.Bytes()[3:])
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	if !strings.HasPrefix(lines[0], "题目ID,答案,AI判定") {
		t.Fatalf("header should use export names, got %q", lines[0])
	}
	// 嵌套 map 应被 JSON 序列化进单元格
	if !strings.Contains(body, `{""label"":""猫""}`) && !strings.Contains(body, `{"label":"猫"}`) {
		t.Fatalf("nested value should be json-stringified, got %q", body)
	}
}

func TestEncodeXLSX_OpensWithExcelizeAndHasHeader(t *testing.T) {
	var buf bytes.Buffer
	n, err := EncodeXLSX(&buf, sampleCols(), sampleRows())
	if err != nil || n != 2 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("excelize cannot open output: %v", err)
	}
	defer f.Close()
	got, err := f.GetRows("Sheet1")
	if err != nil {
		t.Fatalf("GetRows: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected header + 2 rows, got %d", len(got))
	}
	if got[0][0] != "题目ID" || got[0][2] != "AI判定" {
		t.Fatalf("unexpected header: %v", got[0])
	}
	if got[1][0] != "Q1" {
		t.Fatalf("unexpected first data row: %v", got[1])
	}
}

func TestEncodeXLSX_EmptyRowsHeaderOnly(t *testing.T) {
	var buf bytes.Buffer
	if _, err := EncodeXLSX(&buf, sampleCols(), nil); err != nil {
		t.Fatalf("err=%v", err)
	}
	f, _ := excelize.OpenReader(bytes.NewReader(buf.Bytes()))
	defer f.Close()
	got, _ := f.GetRows("Sheet1")
	if len(got) != 1 {
		t.Fatalf("expected header only, got %d rows", len(got))
	}
}

func TestEncode_RoutesByFormat(t *testing.T) {
	for _, format := range []string{"json", "jsonl", "csv", "xlsx"} {
		var buf bytes.Buffer
		if _, err := Encode(format, &buf, sampleCols(), sampleRows()); err != nil {
			t.Fatalf("format %s errored: %v", format, err)
		}
		if buf.Len() == 0 {
			t.Fatalf("format %s produced no output", format)
		}
	}
}

func TestEncode_UnknownFormatErrors(t *testing.T) {
	var buf bytes.Buffer
	if _, err := Encode("pdf", &buf, sampleCols(), sampleRows()); err != ErrUnsupportedFormat {
		t.Fatalf("expected ErrUnsupportedFormat, got %v", err)
	}
}

func TestFileExtensionAndContentType(t *testing.T) {
	cases := map[string][2]string{
		"json":  {".json", "application/json; charset=utf-8"},
		"jsonl": {".jsonl", "application/x-ndjson; charset=utf-8"},
		"csv":   {".csv", "text/csv; charset=utf-8"},
		"xlsx":  {".xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
	}
	for format, want := range cases {
		if FileExtension(format) != want[0] {
			t.Fatalf("%s ext = %s, want %s", format, FileExtension(format), want[0])
		}
		if ContentType(format) != want[1] {
			t.Fatalf("%s content-type = %s", format, ContentType(format))
		}
	}
}
