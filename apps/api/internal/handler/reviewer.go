package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"labelhub-api/internal/auth"
	"labelhub-api/internal/httpx"
	"labelhub-api/internal/middleware"
	"labelhub-api/internal/model"
	"labelhub-api/internal/policy"
	"labelhub-api/internal/service/audit"
	"labelhub-api/internal/service/review"
	"labelhub-api/internal/statemachine"
	"labelhub.local/llmreview"
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
	api.GET("/reviewer/tasks/:taskId/ai-prompts", middleware.RequireRoles("reviewer", "owner", "admin"), h.ReviewerAIPrompts)
	api.POST("/reviewer/submissions/:submissionId/ai-review/retry", middleware.RequireRoles("reviewer", "owner", "admin"), h.RetryAIReview)
	api.POST("/submissions/:submissionId/review", middleware.RequireRoles("reviewer", "owner", "admin"), h.ReviewSubmission)
	api.POST("/reviews/batch", middleware.RequireRoles("reviewer", "owner", "admin"), h.BatchReview)
}

type reviewRequest struct {
	Verdict string `json:"verdict" binding:"required"`
	Reason  string `json:"reason"`
}

type batchReviewRequest struct {
	SubmissionIDs []uint64 `json:"submission_ids"`
	Verdict       string   `json:"verdict" binding:"required"`
	Reason        string   `json:"reason"`
}

type batchReviewResult struct {
	SubmissionID uint64  `json:"submissionId"`
	Status       string  `json:"status,omitempty"`
	Error        *string `json:"error,omitempty"`
}

type batchReviewSummary struct {
	Total     int `json:"total"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
}

type reviewerAIPromptResponse struct {
	ID             uint64          `json:"id"`
	Version        int             `json:"version"`
	Model          string          `json:"model"`
	PromptTemplate string          `json:"promptTemplate"`
	Dimensions     json.RawMessage `json:"dimensions"`
	PassThreshold  float64         `json:"passThreshold"`
	UncertainMin   float64         `json:"uncertainMin"`
}

type reviewerAIReviewResponse struct {
	ID             uint64                    `json:"id"`
	SubmissionID   uint64                    `json:"submissionId"`
	RevisionID     uint64                    `json:"revisionId"`
	IdempotencyKey string                    `json:"idempotencyKey"`
	PromptVersion  int                       `json:"promptVersion"`
	Verdict        *string                   `json:"verdict"`
	OverallScore   *float64                  `json:"overallScore"`
	Dimensions     json.RawMessage           `json:"dimensions"`
	Reason         *string                   `json:"reason"`
	RawResponse    json.RawMessage           `json:"rawResponse"`
	TokensInput    int                       `json:"tokensInput"`
	TokensOutput   int                       `json:"tokensOutput"`
	LatencyMS      int                       `json:"latencyMs"`
	Status         string                    `json:"status"`
	RetryCount     int                       `json:"retryCount"`
	ErrorMsg       *string                   `json:"errorMsg"`
	CreatedAt      time.Time                 `json:"createdAt"`
	FinishedAt     model.NullTime            `json:"finishedAt"`
	Prompt         *reviewerAIPromptResponse `json:"prompt"`
}

type reviewerAIPromptListResponse struct {
	Prompts         []reviewerAIPromptResponse `json:"prompts"`
	ActivePromptID  *uint64                    `json:"activePromptId"`
	AIReviewEnabled bool                       `json:"aiReviewEnabled"`
}

type reviewerAuditLogResponse struct {
	ID         uint64           `json:"id"`
	EntityType string           `json:"entityType"`
	EntityID   uint64           `json:"entityId"`
	FromState  model.NullString `json:"fromState"`
	ToState    string           `json:"toState"`
	ActorType  string           `json:"actorType"`
	ActorID    *uint64          `json:"actorId"`
	Event      string           `json:"event"`
	Payload    json.RawMessage  `json:"payload"`
	CreatedAt  time.Time        `json:"createdAt"`
}

type retryAIReviewResponse struct {
	SubmissionID uint64                   `json:"submissionId"`
	Status       string                   `json:"status"`
	AIReview     reviewerAIReviewResponse `json:"aiReview"`
}

var reviewerQueueAllowedStatuses = map[string]struct{}{
	statemachine.StateHumanReviewing: {},
}

var (
	errAIRetryForbidden       = errors.New("ai retry forbidden")
	errAIRetrySubmissionState = errors.New("ai retry requires human_reviewing submission")
	errAIRetryNoRevision      = errors.New("ai retry requires current revision")
	errAIRetryNoReview        = errors.New("ai review not found")
	errAIRetryReviewState     = errors.New("ai retry requires failed or dead review")
	errAIRetryPromptInvalid   = errors.New("ai retry prompt invalid")
	errAIRetryKeyMismatch     = errors.New("ai retry idempotency key mismatch")
)

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

func (h ReviewerHandler) ReviewerAIPrompts(c *gin.Context) {
	taskID, ok := parseIDParam(c, "taskId")
	if !ok {
		return
	}
	claims, _ := middleware.Claims(c)
	var task model.Task
	if err := h.db.First(&task, taskID).Error; err != nil {
		httpx.Error(c, http.StatusNotFound, "NOT_FOUND", "task not found")
		return
	}
	allowed, err := canReviewTask(h.db, claims, task)
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to check review access")
		return
	}
	if !allowed {
		httpx.Error(c, http.StatusForbidden, "FORBIDDEN", "reviewer is not assigned to this task")
		return
	}

	var prompts []model.AIPromptConfig
	if err := h.db.Where("task_id = ?", task.ID).Order("version DESC").Find(&prompts).Error; err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list ai prompts")
		return
	}
	responses := make([]reviewerAIPromptResponse, 0, len(prompts))
	for _, prompt := range prompts {
		response, err := reviewerAIPromptResponseFromModel(prompt)
		if err != nil {
			httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load ai prompt")
			return
		}
		responses = append(responses, response)
	}
	httpx.OK(c, reviewerAIPromptListResponse{
		Prompts:         responses,
		ActivePromptID:  task.AIPromptID,
		AIReviewEnabled: task.AIReviewEnabled,
	})
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

func (h ReviewerHandler) BatchReview(c *gin.Context) {
	var req batchReviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "submission_ids and verdict are required")
		return
	}
	submissionIDs, ok := normalizeBatchReviewIDs(c, req.SubmissionIDs)
	if !ok {
		return
	}
	if (req.Verdict == "reject" || req.Verdict == "revise") && len(strings.TrimSpace(req.Reason)) < 5 {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "reject/revise 必填详细理由(至少 5 个字符)")
		return
	}
	claims, _ := middleware.Claims(c)
	results := make([]batchReviewResult, 0, len(submissionIDs))
	summary := batchReviewSummary{Total: len(submissionIDs)}
	for _, submissionID := range submissionIDs {
		result, err := review.Apply(h.db, review.ApplyInput{
			SubmissionID: submissionID,
			Verdict:      req.Verdict,
			Reason:       req.Reason,
			ReviewerID:   claims.UserID,
			Roles:        claims.Roles,
		})
		if err != nil {
			message := batchReviewErrorMessage(err)
			results = append(results, batchReviewResult{SubmissionID: submissionID, Error: &message})
			summary.Failed++
			continue
		}
		results = append(results, batchReviewResult{SubmissionID: result.SubmissionID, Status: result.Status})
		summary.Succeeded++
	}
	httpx.OK(c, gin.H{"results": results, "summary": summary})
}

func normalizeBatchReviewIDs(c *gin.Context, submissionIDs []uint64) ([]uint64, bool) {
	if len(submissionIDs) == 0 {
		httpx.Error(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "submission_ids are required")
		return nil, false
	}
	if len(submissionIDs) > 50 {
		httpx.Error(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "submission_ids must contain at most 50 items")
		return nil, false
	}
	seen := map[uint64]struct{}{}
	for _, submissionID := range submissionIDs {
		if submissionID == 0 {
			httpx.Error(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "submission_ids must be positive integers")
			return nil, false
		}
		if _, exists := seen[submissionID]; exists {
			httpx.Error(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "submission_ids must be unique")
			return nil, false
		}
		seen[submissionID] = struct{}{}
	}
	return submissionIDs, true
}

func batchReviewErrorMessage(err error) string {
	switch {
	case errors.Is(err, review.ErrInvalidVerdict):
		return "verdict must be approve, reject, or revise"
	case errors.Is(err, review.ErrSubmissionNotFound):
		return "submission not found"
	case errors.Is(err, review.ErrInvalidTransition):
		return "submission is not human_reviewing"
	case errors.Is(err, review.ErrNoRevision):
		return "submission has no revision"
	case errors.Is(err, review.ErrForbidden):
		return "reviewer is not assigned to this task"
	case errors.Is(err, review.ErrConcurrentWrite):
		return "submission changed during review, please reload"
	default:
		return "failed to review submission"
	}
}

func (h ReviewerHandler) RetryAIReview(c *gin.Context) {
	submissionID, ok := parseIDParam(c, "submissionId")
	if !ok {
		return
	}
	claims, _ := middleware.Claims(c)
	response, err := h.retryAIReview(submissionID, claims)
	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound), errors.Is(err, errAIRetryNoReview):
			httpx.Error(c, http.StatusNotFound, "NOT_FOUND", "ai review not found")
		case errors.Is(err, errAIRetryForbidden):
			httpx.Error(c, http.StatusForbidden, "FORBIDDEN", "reviewer is not assigned to this task")
		case errors.Is(err, errAIRetrySubmissionState), errors.Is(err, errAIRetryNoRevision), errors.Is(err, errAIRetryReviewState), errors.Is(err, errAIRetryPromptInvalid), errors.Is(err, errAIRetryKeyMismatch):
			httpx.Error(c, http.StatusConflict, "CONFLICT", err.Error())
		default:
			httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to retry ai review")
		}
		return
	}
	httpx.OK(c, response)
}

func (h ReviewerHandler) retryAIReview(submissionID uint64, claims *auth.Claims) (retryAIReviewResponse, error) {
	var response retryAIReviewResponse
	err := h.db.Transaction(func(tx *gorm.DB) error {
		var submission model.Submission
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&submission, submissionID).Error; err != nil {
			return err
		}
		if submission.Status != statemachine.StateHumanReviewing {
			return errAIRetrySubmissionState
		}
		if submission.CurrentRevisionID == nil {
			return errAIRetryNoRevision
		}

		var task model.Task
		if err := tx.First(&task, submission.TaskID).Error; err != nil {
			return err
		}
		allowed, err := canReviewTask(tx, claims, task)
		if err != nil {
			return err
		}
		if !allowed {
			return errAIRetryForbidden
		}

		var review model.AIReview
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("submission_id = ? AND revision_id = ?", submission.ID, *submission.CurrentRevisionID).
			Order("created_at DESC, id DESC").
			First(&review).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errAIRetryNoReview
			}
			return err
		}
		if review.Status != "failed" && review.Status != "dead" {
			return errAIRetryReviewState
		}

		var prompt model.AIPromptConfig
		if err := tx.Where("task_id = ? AND version = ?", task.ID, review.PromptVersion).First(&prompt).Error; err != nil {
			return errAIRetryPromptInvalid
		}
		if !llmreview.AllowedModelName(prompt.Model) {
			return errAIRetryPromptInvalid
		}
		key := reviewerAIReviewIdempotencyKey(submission.ID, *submission.CurrentRevisionID, prompt.ID, prompt.Version)
		if key != review.IdempotencyKey {
			return errAIRetryKeyMismatch
		}
		payload, err := reviewerAIReviewTaskPayload(submission.ID, *submission.CurrentRevisionID, prompt.ID, prompt.Version, key)
		if err != nil {
			return err
		}

		if err := tx.Model(&model.AIReview{}).
			Where("id = ? AND status IN ?", review.ID, []string{"failed", "dead"}).
			Updates(map[string]any{
				"status":        "pending",
				"verdict":       nil,
				"overall_score": nil,
				"dimensions":    nil,
				"reason":        nil,
				"raw_response":  nil,
				"tokens_input":  0,
				"tokens_output": 0,
				"latency_ms":    0,
				"retry_count":   0,
				"error_msg":     nil,
				"finished_at":   nil,
			}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.Submission{}).
			Where("id = ? AND status = ? AND current_revision_id = ?", submission.ID, statemachine.StateHumanReviewing, *submission.CurrentRevisionID).
			Updates(map[string]any{
				"status":     statemachine.StateAIReviewing,
				"ai_verdict": nil,
				"ai_score":   nil,
			}).Error; err != nil {
			return err
		}
		if err := tx.Create(&model.OutboxEvent{
			Topic:   "ai:review",
			Payload: payload,
			Status:  "pending",
		}).Error; err != nil {
			return err
		}
		if err := audit.Write(tx, audit.LogEntry{
			EntityType: "submission",
			EntityID:   submission.ID,
			FromState:  statemachine.StateHumanReviewing,
			ToState:    statemachine.StateAIReviewing,
			ActorType:  "user",
			ActorID:    &claims.UserID,
			Event:      "ai_retry",
			Payload: map[string]any{
				"ai_review_id":    review.ID,
				"idempotency_key": key,
				"prompt_version":  prompt.Version,
			},
		}); err != nil {
			return err
		}

		review.Status = "pending"
		review.Verdict = nil
		review.OverallScore = nil
		review.Dimensions = nil
		review.Reason = model.NullString{}
		review.RawResponse = nil
		review.TokensInput = 0
		review.TokensOutput = 0
		review.LatencyMS = 0
		review.RetryCount = 0
		review.ErrorMsg = model.NullString{}
		review.FinishedAt = model.NullTime{}
		aiReview, err := reviewerAIReviewResponseFromModel(review, &prompt)
		if err != nil {
			return err
		}
		response = retryAIReviewResponse{
			SubmissionID: submission.ID,
			Status:       statemachine.StateAIReviewing,
			AIReview:     aiReview,
		}
		return nil
	})
	return response, err
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

	aiReview, ok, err := h.loadLatestAIReviewDetail(submission, task.ID)
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load ai review")
		return nil, false
	}
	auditLogs, err := h.loadSubmissionAuditLogs(submission.ID)
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load audit logs")
		return nil, false
	}
	latestHumanReview, hasHumanReview, err := h.loadLatestHumanReview(submission.ID)
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load latest human review")
		return nil, false
	}

	payload := gin.H{"task": task, "item": item, "template": template, "submission": submission, "revision": revision, "auditLogs": auditLogs}
	if ok {
		payload["aiReview"] = aiReview
	} else {
		payload["aiReview"] = nil
	}
	if hasHumanReview {
		payload["latestHumanReview"] = latestHumanReview
	}
	if errors.Is(templateErr, gorm.ErrRecordNotFound) {
		payload["warnings"] = []string{"template_missing"}
	}
	return payload, true
}

func (h ReviewerHandler) loadLatestHumanReview(submissionID uint64) (humanReviewSummaryResponse, bool, error) {
	var human model.HumanReview
	if err := h.db.Where("submission_id = ?", submissionID).Order("created_at DESC, id DESC").First(&human).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return humanReviewSummaryResponse{}, false, nil
		}
		return humanReviewSummaryResponse{}, false, err
	}
	return humanReviewSummaryResponse{
		Verdict:   human.Verdict,
		Reason:    human.Reason,
		CreatedAt: human.CreatedAt,
	}, true, nil
}

func (h ReviewerHandler) loadLatestAIReviewDetail(submission model.Submission, taskID uint64) (reviewerAIReviewResponse, bool, error) {
	var review model.AIReview
	query := h.db.Where("submission_id = ?", submission.ID)
	if submission.CurrentRevisionID != nil {
		query = query.Where("revision_id = ?", *submission.CurrentRevisionID)
	}
	if err := query.Order("created_at DESC, id DESC").First(&review).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return reviewerAIReviewResponse{}, false, nil
		}
		return reviewerAIReviewResponse{}, false, err
	}

	var prompt *model.AIPromptConfig
	var promptModel model.AIPromptConfig
	if err := h.db.Where("task_id = ? AND version = ?", taskID, review.PromptVersion).First(&promptModel).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return reviewerAIReviewResponse{}, false, err
		}
	} else {
		prompt = &promptModel
	}

	response, err := reviewerAIReviewResponseFromModel(review, prompt)
	if err != nil {
		return reviewerAIReviewResponse{}, false, err
	}
	return response, true, nil
}

func (h ReviewerHandler) loadSubmissionAuditLogs(submissionID uint64) ([]reviewerAuditLogResponse, error) {
	var logs []model.AuditLog
	if err := h.db.
		Where("entity_type = ? AND entity_id = ?", "submission", submissionID).
		Order("created_at ASC, id ASC").
		Limit(20).
		Find(&logs).Error; err != nil {
		return nil, err
	}
	responses := make([]reviewerAuditLogResponse, 0, len(logs))
	for _, log := range logs {
		response, err := reviewerAuditLogResponseFromModel(log)
		if err != nil {
			return nil, err
		}
		responses = append(responses, response)
	}
	return responses, nil
}

func reviewerAIReviewResponseFromModel(review model.AIReview, prompt *model.AIPromptConfig) (reviewerAIReviewResponse, error) {
	dimensions, err := optionalStoredJSON(review.Dimensions)
	if err != nil {
		return reviewerAIReviewResponse{}, err
	}
	rawResponse, err := optionalStoredJSON(review.RawResponse)
	if err != nil {
		return reviewerAIReviewResponse{}, err
	}
	var promptResponse *reviewerAIPromptResponse
	if prompt != nil {
		response, err := reviewerAIPromptResponseFromModel(*prompt)
		if err != nil {
			return reviewerAIReviewResponse{}, err
		}
		promptResponse = &response
	}
	return reviewerAIReviewResponse{
		ID:             review.ID,
		SubmissionID:   review.SubmissionID,
		RevisionID:     review.RevisionID,
		IdempotencyKey: review.IdempotencyKey,
		PromptVersion:  review.PromptVersion,
		Verdict:        review.Verdict,
		OverallScore:   review.OverallScore,
		Dimensions:     dimensions,
		Reason:         nullableStringPointer(review.Reason),
		RawResponse:    rawResponse,
		TokensInput:    review.TokensInput,
		TokensOutput:   review.TokensOutput,
		LatencyMS:      review.LatencyMS,
		Status:         review.Status,
		RetryCount:     review.RetryCount,
		ErrorMsg:       nullableStringPointer(review.ErrorMsg),
		CreatedAt:      review.CreatedAt,
		FinishedAt:     review.FinishedAt,
		Prompt:         promptResponse,
	}, nil
}

func reviewerAIPromptResponseFromModel(prompt model.AIPromptConfig) (reviewerAIPromptResponse, error) {
	dimensions, err := requiredStoredJSON(prompt.Dimensions)
	if err != nil {
		return reviewerAIPromptResponse{}, err
	}
	return reviewerAIPromptResponse{
		ID:             prompt.ID,
		Version:        prompt.Version,
		Model:          prompt.Model,
		PromptTemplate: prompt.PromptTemplate,
		Dimensions:     dimensions,
		PassThreshold:  prompt.PassThreshold,
		UncertainMin:   prompt.UncertainMin,
	}, nil
}

func reviewerAuditLogResponseFromModel(log model.AuditLog) (reviewerAuditLogResponse, error) {
	payload, err := optionalStoredJSON(log.Payload)
	if err != nil {
		return reviewerAuditLogResponse{}, err
	}
	return reviewerAuditLogResponse{
		ID:         log.ID,
		EntityType: log.EntityType,
		EntityID:   log.EntityID,
		FromState:  log.FromState,
		ToState:    log.ToState,
		ActorType:  log.ActorType,
		ActorID:    log.ActorID,
		Event:      log.Event,
		Payload:    payload,
		CreatedAt:  log.CreatedAt,
	}, nil
}

func requiredStoredJSON(raw string) (json.RawMessage, error) {
	value := json.RawMessage(raw)
	if !json.Valid(value) {
		return nil, errInvalidStoredJSON
	}
	return value, nil
}

func reviewerAIReviewTaskPayload(submissionID uint64, revisionID uint64, promptID uint64, promptVersion int, idempotencyKey string) (string, error) {
	raw, err := json.Marshal(gin.H{
		"submission_id":    submissionID,
		"revision_id":      revisionID,
		"prompt_config_id": promptID,
		"prompt_version":   promptVersion,
		"idempotency_key":  idempotencyKey,
	})
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func reviewerAIReviewIdempotencyKey(submissionID uint64, revisionID uint64, promptID uint64, promptVersion int) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d:%d:%d:%d", submissionID, revisionID, promptID, promptVersion)))
	return hex.EncodeToString(sum[:])
}

func nullableStringPointer(value model.NullString) *string {
	if !value.Valid {
		return nil
	}
	out := value.String
	return &out
}
