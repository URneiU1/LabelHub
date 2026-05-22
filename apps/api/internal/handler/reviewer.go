package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"labelhub-api/internal/httpx"
	"labelhub-api/internal/middleware"
	"labelhub-api/internal/model"
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
	event, to, humanVerdict, ok := reviewDecision(req.Verdict)
	if !ok {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "verdict must be approve, reject, or revise")
		return
	}
	if (req.Verdict == "reject" || req.Verdict == "revise") && len(strings.TrimSpace(req.Reason)) < 5 {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "reject/revise 必填详细理由(至少 5 个字符)")
		return
	}
	claims, _ := middleware.Claims(c)

	var submission model.Submission
	if err := h.db.First(&submission, submissionID).Error; err != nil {
		httpx.Error(c, http.StatusNotFound, "NOT_FOUND", "submission not found")
		return
	}
	if err := statemachine.Apply(submission.Status, event, to); err != nil {
		httpx.Error(c, http.StatusUnprocessableEntity, "INVALID_STATE", err.Error())
		return
	}
	if submission.CurrentRevisionID == nil {
		httpx.Error(c, http.StatusConflict, "CONFLICT", "submission has no revision")
		return
	}

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		review := model.HumanReview{
			SubmissionID: submission.ID,
			RevisionID:   *submission.CurrentRevisionID,
			ReviewerID:   claims.UserID,
			Stage:        "first",
			Verdict:      humanVerdict,
			Reason:       nullString(req.Reason),
		}
		if err := tx.Create(&review).Error; err != nil {
			return err
		}
		updates := reviewUpdates(to, humanVerdict, time.Now().UTC())
		if err := tx.Model(&model.Submission{}).Where("id = ?", submission.ID).Updates(updates).Error; err != nil {
			return err
		}
		if to == statemachine.StateApproved || to == statemachine.StateRejected {
			if err := tx.Model(&model.TaskItem{}).Where("id = ?", submission.ItemID).Updates(map[string]any{
				"status":      itemStatusFinished,
				"finished_at": time.Now().UTC(),
			}).Error; err != nil {
				return err
			}
			if to == statemachine.StateApproved {
				if err := tx.Model(&model.Task{}).Where("id = ?", submission.TaskID).Update("finished_items", gorm.Expr("finished_items + 1")).Error; err != nil {
					return err
				}
			}
		}
		return createAuditLog(tx, "submission", submission.ID, submission.Status, to, "user", &claims.UserID, event, map[string]any{"reason": req.Reason})
	}); err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to review submission")
		return
	}

	httpx.OK(c, gin.H{"submission_id": submission.ID, "status": to})
}

func reviewUpdates(to string, humanVerdict string, now time.Time) map[string]any {
	updates := map[string]any{"status": to, "human_verdict": humanVerdict}
	if to == statemachine.StateApproved {
		updates["approved_at"] = now
	}
	return updates
}

func reviewDecision(verdict string) (event string, to string, humanVerdict string, ok bool) {
	switch verdict {
	case "approve":
		return statemachine.EventApprove, statemachine.StateApproved, "approve", true
	case "reject":
		return statemachine.EventReject, statemachine.StateRejected, "reject", true
	case "revise":
		return statemachine.EventRevise, statemachine.StateRevising, "revise", true
	default:
		return "", "", "", false
	}
}
