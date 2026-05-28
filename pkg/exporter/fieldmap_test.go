package exporter

import "testing"

func strptr(s string) *string { return &s }

func TestParseFieldMap_NilReturnsEmpty(t *testing.T) {
	fm, err := ParseFieldMap(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fm.IncludeReviews || len(fm.Columns) != 0 {
		t.Fatalf("expected zero-value FieldMap, got %+v", fm)
	}
	fm2, err := ParseFieldMap(strptr("   "))
	if err != nil || len(fm2.Columns) != 0 {
		t.Fatalf("blank string should yield empty fieldmap, got %+v err %v", fm2, err)
	}
}

func TestParseFieldMap_InvalidJSONErrors(t *testing.T) {
	if _, err := ParseFieldMap(strptr("{not json")); err == nil {
		t.Fatal("expected error for invalid json")
	}
}

func TestParseFieldMap_ParsesColumnsAndFlag(t *testing.T) {
	fm, err := ParseFieldMap(strptr(`{"include_reviews":true,"columns":[{"source":"external_id","export":"题目ID"}]}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fm.IncludeReviews || len(fm.Columns) != 1 || fm.Columns[0].Source != "external_id" || fm.Columns[0].Export != "题目ID" {
		t.Fatalf("unexpected fieldmap: %+v", fm)
	}
}

func TestApply_SelectsAndRenames(t *testing.T) {
	full := Row{{Key: "external_id", Value: "Q1"}, {Key: "answer", Value: "ok"}, {Key: "extra", Value: 9}}
	fm := FieldMap{Columns: []Column{{Source: "external_id", Export: "题目ID"}, {Source: "answer", Export: "答案"}}}
	out := fm.Apply(full)
	if len(out) != 2 {
		t.Fatalf("expected 2 cols, got %d", len(out))
	}
	if out[0].Key != "题目ID" || out[0].Value != "Q1" || out[1].Key != "答案" || out[1].Value != "ok" {
		t.Fatalf("unexpected applied row: %+v", out)
	}
}

func TestApply_EmptyColumnsReturnsFull(t *testing.T) {
	full := Row{{Key: "a", Value: 1}}
	out := FieldMap{}.Apply(full)
	if len(out) != 1 || out[0].Key != "a" {
		t.Fatalf("empty columns should return full row, got %+v", out)
	}
}

func TestApply_MissingSourceFillsNil(t *testing.T) {
	full := Row{{Key: "a", Value: 1}}
	fm := FieldMap{Columns: []Column{{Source: "missing", Export: "M"}}}
	out := fm.Apply(full)
	if len(out) != 1 || out[0].Key != "M" || out[0].Value != nil {
		t.Fatalf("missing source should fill nil, got %+v", out)
	}
}

func TestColumnsOrDefault_DerivesFromSample(t *testing.T) {
	sample := Row{{Key: "x"}, {Key: "y"}}
	cols := FieldMap{}.ColumnsOrDefault(sample)
	if len(cols) != 2 || cols[0].Source != "x" || cols[0].Export != "x" || cols[1].Source != "y" {
		t.Fatalf("unexpected default columns: %+v", cols)
	}
	explicit := FieldMap{Columns: []Column{{Source: "a", Export: "A"}}}
	if got := explicit.ColumnsOrDefault(sample); len(got) != 1 || got[0].Export != "A" {
		t.Fatalf("explicit columns should win: %+v", got)
	}
}
