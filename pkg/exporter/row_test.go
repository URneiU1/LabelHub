package exporter

import (
	"encoding/json"
	"testing"
)

func TestStringifyCell_Variants(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{nil, ""},
		{"hi", "hi"},
		{json.Number("123456789012345678"), "123456789012345678"}, // 大整数不丢精度
		{true, "true"},
		{false, "false"},
		{map[string]any{"k": "v"}, `{"k":"v"}`},
		{[]any{1, 2}, "[1,2]"},
	}
	for _, c := range cases {
		if got := stringifyCell(c.in); got != c.want {
			t.Fatalf("stringifyCell(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestPick_MissingReturnsNil(t *testing.T) {
	row := Row{{Key: "a", Value: 1}}
	if pick(row, "a") != 1 {
		t.Fatal("pick a")
	}
	if pick(row, "nope") != nil {
		t.Fatal("missing key should be nil")
	}
}

func TestExtensionContentType_UnknownDefaults(t *testing.T) {
	if FileExtension("zzz") != ".bin" {
		t.Fatalf("unknown ext = %s", FileExtension("zzz"))
	}
	if ContentType("zzz") != "application/octet-stream" {
		t.Fatalf("unknown content-type = %s", ContentType("zzz"))
	}
	if SupportedFormat("zzz") || !SupportedFormat("csv") {
		t.Fatal("SupportedFormat mismatch")
	}
}
