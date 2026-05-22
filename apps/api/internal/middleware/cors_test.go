package middleware

import "testing"

func TestOriginAllowedRequiresExplicitOrigin(t *testing.T) {
	allowed := []string{"http://localhost:5173", "https://labelhub.example.com"}

	if !originAllowed("https://labelhub.example.com", allowed) {
		t.Fatal("explicit allowed origin should pass")
	}
	if originAllowed("https://evil.example.com", allowed) {
		t.Fatal("unlisted origin should be blocked")
	}
	if originAllowed("https://evil.example.com", []string{"*"}) {
		t.Fatal("wildcard origin must not be accepted")
	}
}
