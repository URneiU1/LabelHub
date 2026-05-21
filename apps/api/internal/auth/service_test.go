package auth

import (
	"testing"
	"time"

	"labelhub-api/internal/model"
)

const testSecret = "labelhub-test-secret-32-characters-min"

func TestGenerateAndParseTokenPair(t *testing.T) {
	service := NewServiceForTest(testSecret, 2*time.Hour, 24*time.Hour)
	user := model.User{ID: 42, Username: "owner1"}
	roles := []string{"owner", "reviewer"}

	tokens, err := service.GenerateTokenPair(user, roles)
	if err != nil {
		t.Fatalf("GenerateTokenPair() error = %v", err)
	}

	accessClaims, err := service.Parse(tokens.AccessToken, TokenTypeAccess)
	if err != nil {
		t.Fatalf("Parse(access) error = %v", err)
	}
	if accessClaims.UserID != user.ID || accessClaims.Username != user.Username {
		t.Fatalf("access claims user mismatch: %+v", accessClaims)
	}

	if _, err := service.Parse(tokens.AccessToken, TokenTypeRefresh); err == nil {
		t.Fatal("Parse(access as refresh) expected error")
	}

	refreshClaims, err := service.Parse(tokens.RefreshToken, TokenTypeRefresh)
	if err != nil {
		t.Fatalf("Parse(refresh) error = %v", err)
	}
	if refreshClaims.TokenType != TokenTypeRefresh {
		t.Fatalf("refresh token type = %q", refreshClaims.TokenType)
	}
}

func TestExpiredTokenRejected(t *testing.T) {
	service := NewServiceForTest(testSecret, -1*time.Second, time.Hour)
	tokens, err := service.GenerateTokenPair(model.User{ID: 1, Username: "labeler1"}, []string{"labeler"})
	if err != nil {
		t.Fatalf("GenerateTokenPair() error = %v", err)
	}
	if _, err := service.Parse(tokens.AccessToken, TokenTypeAccess); err == nil {
		t.Fatal("expired token should be rejected")
	}
}

func TestWrongSecretRejected(t *testing.T) {
	signer := NewServiceForTest(testSecret, time.Hour, time.Hour)
	verifier := NewServiceForTest("different-secret-but-still-long-enough-1234", time.Hour, time.Hour)
	tokens, err := signer.GenerateTokenPair(model.User{ID: 1, Username: "owner1"}, []string{"owner"})
	if err != nil {
		t.Fatalf("GenerateTokenPair() error = %v", err)
	}
	if _, err := verifier.Parse(tokens.AccessToken, TokenTypeAccess); err == nil {
		t.Fatal("token signed by different secret should be rejected")
	}
}

func TestPasswordHash(t *testing.T) {
	hash, err := HashPassword("pass")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	if !CheckPassword(hash, "pass") {
		t.Fatal("CheckPassword() returned false for correct password")
	}
	if CheckPassword(hash, "wrong") {
		t.Fatal("CheckPassword() returned true for wrong password")
	}
}
