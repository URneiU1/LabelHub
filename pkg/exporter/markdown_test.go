package exporter

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestEncodeMarkdown_HeaderSeparatorAndRows(t *testing.T) {
	var buf bytes.Buffer
	n, err := EncodeMarkdown(&buf, sampleCols(), sampleRows())
	if err != nil || n != 2 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("expected header + separator + 2 rows, got %d: %q", len(lines), buf.String())
	}
	// 表头用 Export 名。
	if lines[0] != "| 题目ID | 答案 | AI判定 |" {
		t.Fatalf("unexpected header: %q", lines[0])
	}
	// 分隔行每列一个 ---。
	if lines[1] != "| --- | --- | --- |" {
		t.Fatalf("unexpected separator: %q", lines[1])
	}
	if !strings.HasPrefix(lines[2], "| Q1 |") {
		t.Fatalf("unexpected first data row: %q", lines[2])
	}
	// 嵌套 map 应被 JSON 序列化进单元格。
	if !strings.Contains(lines[2], `{"label":"猫"}`) {
		t.Fatalf("nested value should be json-stringified, got %q", lines[2])
	}
	// json.Number 标量原样输出。
	if !strings.Contains(lines[3], "| 2 |") {
		t.Fatalf("json.Number cell not rendered, got %q", lines[3])
	}
}

func TestEncodeMarkdown_EscapesPipeAndNewline(t *testing.T) {
	cols := []Column{{Source: "answer", Export: "答|案"}}
	rows := []Row{
		{{Key: "answer", Value: "a|b\nc"}},
		{{Key: "answer", Value: "line1\r\nline2"}},
	}
	var buf bytes.Buffer
	n, err := EncodeMarkdown(&buf, cols, rows)
	if err != nil || n != 2 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	out := buf.String()
	// 表头里的 | 也要转义,否则破坏表格列数。
	if !strings.Contains(out, `| 答\|案 |`) {
		t.Fatalf("header pipe not escaped: %q", out)
	}
	// 单元格内 | 转 \|,换行转 <br>。
	if !strings.Contains(out, `| a\|b<br>c |`) {
		t.Fatalf("cell pipe/newline not escaped: %q", out)
	}
	// \r\n 与 \r 都折成单个 <br>,不留裸回车。
	if strings.Contains(out, "\r") {
		t.Fatalf("output still contains carriage return: %q", out)
	}
	if !strings.Contains(out, "| line1<br>line2 |") {
		t.Fatalf("crlf not folded to single <br>: %q", out)
	}
}

func TestEncodeMarkdown_EmptyRowsHeaderOnly(t *testing.T) {
	var buf bytes.Buffer
	n, err := EncodeMarkdown(&buf, sampleCols(), nil)
	if err != nil || n != 0 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected header + separator only, got %d: %q", len(lines), buf.String())
	}
}

func TestEncodeMarkdown_MultiColumnAndMissingCells(t *testing.T) {
	cols := []Column{{Source: "a", Export: "A"}, {Source: "b", Export: "B"}, {Source: "c", Export: "C"}}
	rows := []Row{
		{{Key: "a", Value: "1"}, {Key: "c", Value: json.Number("3")}}, // b 缺列 → 空单元格
	}
	var buf bytes.Buffer
	if _, err := EncodeMarkdown(&buf, cols, rows); err != nil {
		t.Fatalf("encode: %v", err)
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	// 缺列容忍为空,列数仍与表头一致。
	if lines[2] != "| 1 |  | 3 |" {
		t.Fatalf("missing column not tolerated as empty cell, got %q", lines[2])
	}
}

func TestEncode_RoutesMarkdown(t *testing.T) {
	var buf bytes.Buffer
	if _, err := Encode("md", &buf, sampleCols(), sampleRows()); err != nil {
		t.Fatalf("md format errored: %v", err)
	}
	if !strings.Contains(buf.String(), "| --- |") {
		t.Fatalf("Encode(md) did not route to markdown encoder: %q", buf.String())
	}
}

func TestMarkdownFileExtensionAndContentType(t *testing.T) {
	if !SupportedFormat("md") {
		t.Fatal("md should be a supported format")
	}
	if FileExtension("md") != ".md" {
		t.Fatalf("md ext = %s", FileExtension("md"))
	}
	if ContentType("md") != "text/markdown; charset=utf-8" {
		t.Fatalf("md content-type = %s", ContentType("md"))
	}
}
