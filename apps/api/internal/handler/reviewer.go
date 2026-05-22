package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"labelhub-api/internal/httpx"
	"labelhub-api/internal/middleware"
	"labelhub-api/internal/model"
	"labelhub-api/internal/service/review"
	"labelhub-api/internal/statemachine"
)

// ReviewerHandler 封装 reviewer 审核相关端点。
type ReviewerHandler struct {
	db *gorm.DB
}

func NewReviewerHandler(db *gorm.DB) ReviewerHandler {
	return ReviewerHandler{db: db}
}

func (h ReviewerHandler) Register(api gin.IRouter) {
	api.GET("/reviewer/submissions", middleware.RequireRoles("reviewer", "owner", "admin"), h.ReviewerQueue)
	api.POST("/submissions/:submissionId/review", middleware.RequireRoles("reviewer", "owner", "admin"), h.ReviewSubmission)
}

type reviewRequest struct {
	Verdict string `json:"verdict" binding:"required"`
	Reason  string `json:"reason"`
}

var reviewerQueueAllowedStatuses = map[string]struct{}{
	statemachine.StateHumanReviewing: {},
}

func (h ReviewerHandler) ReviewerQueue(c *gin.Context) {
	status := c.DefaultQuery("status", statemachine.StateHumanReviewing)
	if _, ok := reviewerQueueAllowedStatuses[status]; !ok {
		httpx.ErrorWithDetails(c, http.StatusForbidden, "FORBIDDEN",
			"reviewer queue only exposes human_reviewing",
			gin.H{"requested": status, "allowed": []string{statemachine.StateHumanReviewing}})
		return
	}
	var submissions []model.Submission
	if err := h.db.Where("status = ?", status).Order("updated_at ASC").Find(&submissions).Error; err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list review queue")
		return
	}
	httpx.PageOK(c, submissions, httpx.Page{})
}

func (h ReviewerHandler) ReviewSubmission(c *gin.Context) {
	submissionID, ok := parseIDParam(c, "submissionId")
	if !ok {
		return
	}
	var req reviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "verdict is required")
		return
	}
	if (req.Verdict == "reject" || req.Verdict == "revise") && len(strings.TrimSpace(req.Reason)) < 5 {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "reject/revise 必填详细理由(至少 5 个字符)")
		return
	}
	claims, _ := middleware.Claims(c)

	result, err := review.Apply(h.db, review.ApplyInput{
		SubmissionID: submissionID,
		Verdict:      req.Verdict,
		Reason:       req.Reason,
		ReviewerID:   claims.UserID,
	})
	if err != nil {
		switch {
		case errors.Is(err, review.ErrInvalidVerdict):
			httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "verdict must be approve, reject, or revise")
		case errors.Is(err, review.ErrSubmissionNotFound):
			httpx.Error(c, http.StatusNotFound, "NOT_FOUND", "submission not found")
		case errors.Is(err, review.ErrInvalidTransition):
			httpx.Error(c, http.StatusUnprocessableEntity, "INVALID_STATE", err.Error())
		case errors.Is(err, review.ErrNoRevision):
			httpx.Error(c, http.StatusConflict, "CONFLICT", "submission has no revision")
		default:
			httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to review submission")
		}
		return
	}

	httpx.OK(c, gin.H{"submission_id": result.SubmissionID, "status": result.Status})
}
