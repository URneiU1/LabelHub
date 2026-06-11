package main

import "testing"

func TestSha256Hex_Is64LowerHexAndStable(t *testing.T) {
	got := sha256Hex("labelhub")
	if len(got) != 64 {
		t.Fatalf("sha256Hex must be 64 hex chars to fit CHAR(64), got %d: %q", len(got), got)
	}
	for _, r := range got {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			t.Fatalf("sha256Hex must be lowercase hex, got %q", got)
		}
	}
	if sha256Hex("labelhub") != got {
		t.Fatal("sha256Hex must be deterministic for the same input")
	}
	if sha256Hex("labelhub2") == got {
		t.Fatal("sha256Hex must differ for different input")
	}
}
