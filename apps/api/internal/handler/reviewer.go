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
	"labelhub-api/internal/policy"
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
	api.GET("/reviewer/submissions/:submissionId", middleware.RequireRoles("reviewer", "owner", "admin"), h.ReviewerDetail)
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
	claims, _ := middleware.Claims(c)
	var submissions []model.Submission
	query := h.db.Model(&model.Submission{}).Where("submissions.status = ?", status)
	var scoped bool
	query, scoped = applyReviewQueueScope(query, claims)
	if !scoped {
		httpx.Error(c, http.StatusForbidden, "FORBIDDEN", "review queue access denied")
		return
	}
	if err := query.Order("submissions.updated_at ASC").Find(&submissions).Error; err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list review queue")
		return
	}
	httpx.PageOK(c, submissions, httpx.Page{})
}

func (h ReviewerHandler) ReviewerDetail(c *gin.Context) {
	submissionID, ok := parseIDParam(c, "submissionId")
	if !ok {
		return
	}
	bundle, ok := h.loadReviewBundle(c, submissionID)
	if !ok {
		return
	}
	httpx.OK(c, bundle)
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
		Roles:        claims.Roles,
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
		case errors.Is(err, review.ErrForbidden):
			httpx.Error(c, http.StatusForbidden, "FORBIDDEN", "reviewer is not assigned to this task")
		case errors.Is(err, review.ErrConcurrentWrite):
			httpx.Error(c, http.StatusConflict, "CONFLICT", "submission changed during review, please reload")
		default:
			httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to review submission")
		}
		return
	}

	httpx.OK(c, gin.H{"submission_id": result.SubmissionID, "status": result.Status})
}

func (h ReviewerHandler) loadReviewBundle(c *gin.Context, submissionID uint64) (gin.H, bool) {
	claims, _ := middleware.Claims(c)
	var submission model.Submission
	if err := h.db.First(&submission, submissionID).Error; err != nil {
		httpx.Error(c, http.StatusNotFound, "NOT_FOUND", "submission not found")
		return nil, false
	}
	if !policy.CanReviewSubmissionStatus(submission.Status) {
		httpx.Error(c, http.StatusForbidden, "FORBIDDEN", "submission is not visible in reviewer detail")
		return nil, false
	}

	var task model.Task
	if err := h.db.First(&task, submission.TaskID).Error; err != nil {
		httpx.Error(c, http.StatusNotFound, "NOT_FOUND", "task not found")
		return nil, false
	}
	allowed, err := canReviewTask(h.db, claims, task)
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to check review access")
		return nil, false
	}
	if !allowed {
		httpx.Error(c, http.StatusForbidden, "FORBIDDEN", "reviewer is not assigned to this task")
		return nil, false
	}

	var item model.TaskItem
	if err := h.db.Where("id = ? AND task_id = ?", submission.ItemID, task.ID).First(&item).Error; err != nil {
		httpx.Error(c, http.StatusNotFound, "NOT_FOUND", "item not found")
		return nil, false
	}
	template, templateErr := templateForBundle(h.db, task, submission)
	if templateErr != nil && !errors.Is(templateErr, gorm.ErrRecordNotFound) {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load template")
		return nil, false
	}

	var revision *model.SubmissionRevision
	if submission.CurrentRevisionID != nil {
		var current model.SubmissionRevision
		if err := h.db.First(&current, *submission.CurrentRevisionID).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load revision")
				return nil, false
			}
		} else {
			revision = &current
		}
	}

	payload := gin.H{"task": task, "item": item, "template": template, "submission": submission, "revision": revision}
	if errors.Is(templateErr, gorm.ErrRecordNotFound) {
		payload["warnings"] = []string{"template_missing"}
	}
	return payload, true
}
