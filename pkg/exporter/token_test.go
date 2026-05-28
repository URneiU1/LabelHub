package exporter

import (
	"testing"
	"time"
)

func TestSignVerify_RoundTrip(t *testing.T) {
	secret := "super-secret-key"
	tok := SignDownloadToken(secret, 42, time.Now().Add(time.Minute))
	id, err := VerifyDownloadToken(secret, tok, time.Now())
	if err != nil {
		t.Fatalf("verify errored: %v", err)
	}
	if id != 42 {
		t.Fatalf("exportID = %d, want 42", id)
	}
}

func TestVerify_TamperedSignatureFails(t *testing.T) {
	secret := "super-secret-key"
	tok := SignDownloadToken(secret, 42, time.Now().Add(time.Minute))
	tampered := tok[:len(tok)-2] + "xy"
	if _, err := VerifyDownloadToken(secret, tampered, time.Now()); err != ErrTokenInvalid {
		t.Fatalf("expected ErrTokenInvalid, got %v", err)
	}
}

func TestVerify_ExpiredFails(t *testing.T) {
	secret := "super-secret-key"
	tok := SignDownloadToken(secret, 42, time.Now().Add(-time.Second))
	id, err := VerifyDownloadToken(secret, tok, time.Now())
	if err != ErrTokenExpired {
		t.Fatalf("expected ErrTokenExpired, got %v", err)
	}
	if id != 42 {
		t.Fatalf("expired token should still surface exportID, got %d", id)
	}
}

func TestVerify_GarbageFails(t *testing.T) {
	for _, g := range []string{"", "garbage", "a.b.c", "!!!.@@@"} {
		if _, err := VerifyDownloadToken("k", g, time.Now()); err != ErrTokenInvalid {
			t.Fatalf("garbage %q: expected ErrTokenInvalid, got %v", g, err)
		}
	}
}

func TestVerify_WrongSecretFails(t *testing.T) {
	tok := SignDownloadToken("right", 42, time.Now().Add(time.Minute))
	if _, err := VerifyDownloadToken("wrong", tok, time.Now()); err != ErrTokenInvalid {
		t.Fatalf("expected ErrTokenInvalid for wrong secret, got %v", err)
	}
}
