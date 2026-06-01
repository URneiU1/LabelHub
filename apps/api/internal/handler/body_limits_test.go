package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"labelhub-api/internal/auth"
)

func TestJSONEntryPointsRejectOversizedBodies(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		maxBytes int64
		register func(*gin.Engine)
	}{
		{"login", "/auth/login", maxAuthJSONBytes, func(r *gin.Engine) { r.POST("/auth/login", (AuthHandler{}).Login) }},
		{"refresh", "/auth/refresh", maxAuthJSONBytes, func(r *gin.Engine) { r.POST("/auth/refresh", (AuthHandler{}).Refresh) }},
		{"inline llm", "/llm/inline", maxInlineLLMBytes, func(r *gin.Engine) { r.POST("/llm/inline", NewLLMHandler().InlineLLM) }},
		{"review submission", "/submissions/1/review", maxReviewJSONBytes, func(r *gin.Engine) {
			r.POST("/submissions/:submissionId/review", (ReviewerHandler{}).ReviewSubmission)
		}},
		{"batch review", "/reviews/batch", maxReviewJSONBytes, func(r *gin.Engine) { r.POST("/reviews/batch", (ReviewerHandler{}).BatchReview) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			tt.register(r)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, jsonRequest(http.MethodPost, tt.path, map[string]any{
				"blob": strings.Repeat("x", int(tt.maxBytes)+1),
			}))
			if rec.Code != http.StatusRequestEntityTooLarge {
				t.Fatalf("expected 413, got %d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestOwnedJSONEntryPointsRejectOversizedBodies(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		maxBytes int64
	}{
		{"async export", "/tasks/1/exports", maxExportConfigBytes},
		{"baseline", "/tasks/1/baseline", maxBaselineBytes},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, sqlDB := newMockDB(t)
			defer sqlDB.Close()
			expectOwnedTask(mock, "draft")

			r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner1", Roles: []string{"owner"}})
			registerAllHandlers(r, db)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, jsonRequest(http.MethodPost, tt.path, map[string]any{
				"blob": strings.Repeat("x", int(tt.maxBytes)+1),
			}))
			if rec.Code != http.StatusRequestEntityTooLarge {
				t.Fatalf("expected 413, got %d body=%s", rec.Code, rec.Body.String())
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("expectations: %v", err)
			}
		})
	}
}
