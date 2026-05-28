package exporter

import (
	"errors"
	"strings"
	"testing"
)

type errWriter struct{}

func (errWriter) Write([]byte) (int, error) { return 0, errors.New("boom") }

func TestEncoders_PropagateWriteErrors(t *testing.T) {
	cols := sampleCols()
	rows := sampleRows()
	if _, err := EncodeJSON(errWriter{}, cols, rows); err == nil {
		t.Fatal("EncodeJSON should propagate write error")
	}
	if _, err := EncodeJSONL(errWriter{}, cols, rows); err == nil {
		t.Fatal("EncodeJSONL should propagate write error")
	}
	if _, err := EncodeCSV(errWriter{}, cols, rows); err == nil {
		t.Fatal("EncodeCSV should propagate write error (BOM)")
	}
	if _, err := EncodeXLSX(errWriter{}, cols, rows); err == nil {
		t.Fatal("EncodeXLSX should propagate write error")
	}
}

func TestStringifyCell_UnmarshalableFallsBackToFmt(t *testing.T) {
	got := stringifyCell(make(chan int))
	if !strings.Contains(got, "0x") && got == "" {
		t.Fatalf("unmarshalable value should fall back to fmt, got %q", got)
	}
}
