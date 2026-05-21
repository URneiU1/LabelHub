package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequestIDUsesIncomingHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID())
	router.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, c.GetString(RequestIDContextKey))
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-Id", "req-test")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Body.String() != "req-test" {
		t.Fatalf("body = %q", rec.Body.String())
	}
	if rec.Header().Get("X-Request-Id") != "req-test" {
		t.Fatalf("X-Request-Id = %q", rec.Header().Get("X-Request-Id"))
	}
}

func TestRequestIDGeneratesWhenMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID())
	router.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, c.GetString(RequestIDContextKey))
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Body.String() == "" {
		t.Fatal("generated request id is empty")
	}
	if rec.Header().Get("X-Request-Id") != rec.Body.String() {
		t.Fatalf("header/body mismatch: %q vs %q", rec.Header().Get("X-Request-Id"), rec.Body.String())
	}
}
