package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
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
	api.GET("/reviewer/submissions", middleware.RequireRoles("reviewer", "admin"), h.ReviewerQueue)
	api.GET("/reviewer/results", middleware.RequireRoles("reviewer", "admin"), h.ReviewerResults)
	api.GET("/reviewer/submissions/:submissionId", middleware.RequireRoles("reviewer", "admin"), h.ReviewerDetail)
	api.GET("/reviewer/tasks/:taskId/ai-prompts", middleware.RequireRoles("reviewer", "admin"), h.ReviewerAIPrompts)
	api.POST("/reviewer/submissions/:submissionId/ai-review/retry", middleware.RequireRoles("reviewer", "admin"), h.RetryAIReview)
	api.POST("/submissions/:submissionId/review", middleware.RequireRoles("reviewer", "admin"), h.ReviewSubmission)
	api.POST("/reviews/batch", middleware.RequireRoles("reviewer", "admin"), h.BatchReview)
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
	Stage        string  `json:"stage,omitempty"`
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
	statemachine.StateHumanReviewing:   {},
	statemachine.StateManualReview:     {},
	statemachine.StateNeedsArbitration: {},
}

// reviewStageInfo 暴露 submission 当前所处的人工审核级别给前端展示初审/复审/终审。
type reviewStageInfo struct {
	ReviewStage    string `json:"reviewStage"`    // first / second / final
	ReviewLevel    int    `json:"reviewLevel"`    // 1 / 2
	RequiredLevels int    `json:"requiredLevels"` // 固定 2(初审 → 终审)
}

// reviewerQueueItem:queue 里每条 submission 附带其当前 stage。
type reviewerQueueItem struct {
	model.Submission
	reviewStageInfo
}

// finalizedReviewStatuses:/reviewer/results 暴露的已定稿状态。
var finalizedReviewStatuses = []string{statemachine.StateApproved, statemachine.StateRejected}

// reviewerResultItem:review 结果列表的单行。
type reviewerResultItem struct {
	ID           uint64    `json:"id"`
	TaskID       uint64    `json:"taskId"`
	ItemID       uint64    `json:"itemId"`
	Status       string    `json:"status"`
	FinalVerdict *string   `json:"finalVerdict"`
	ReviewerID   *uint64   `json:"reviewerId"`
	AIScore      *float64  `json:"aiScore"`
	UpdatedAt    time.Time `json:"updatedAt"`
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
			"reviewer queue only exposes reviewable submissions",
			gin.H{"requested": status, "allowed": []string{statemachine.StateHumanReviewing, statemachine.StateManualReview, statemachine.StateNeedsArbitration}})
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
	if err := query.Order("submissions.updated_at ASC").Limit(200).Find(&submissions).Error; err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list review queue")
		return
	}
	approveCounts, err := h.approveCountsByRevision(submissions)
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to compute review stage")
		return
	}
	items := make([]reviewerQueueItem, 0, len(submissions))
	for _, submission := range submissions {
		items = append(items, reviewerQueueItem{
			Submission:      submission,
			reviewStageInfo: stageInfoFor(submission, approveCounts),
		})
	}
	httpx.PageOK(c, items, httpx.Page{})
}

// approveCountsByRevision 一次性查出 queue 内所有 submission 当前 revision 的 approve 数,避免 N+1。
// key = current_revision_id。
func (h ReviewerHandler) approveCountsByRevision(submissions []model.Submission) (map[uint64]int, error) {
	revisionIDs := make([]uint64, 0, len(submissions))
	for _, submission := range submissions {
		if submission.CurrentRevisionID != nil {
			revisionIDs = append(revisionIDs, *submission.CurrentRevisionID)
		}
	}
	counts := map[uint64]int{}
	if len(revisionIDs) == 0 {
		return counts, nil
	}
	type row struct {
		RevisionID uint64
		Total      int
	}
	var rows []row
	if err := h.db.Model(&model.HumanReview{}).
		Select("revision_id, COUNT(*) AS total").
		Where("revision_id IN ? AND verdict = ?", revisionIDs, "approve").
		Group("revision_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		counts[r.RevisionID] = r.Total
	}
	return counts, nil
}

// stageInfoFor 由 approve 计数派生 submission 当前所处 stage。无 revision 时退回初审。
func stageInfoFor(submission model.Submission, approveCounts map[uint64]int) reviewStageInfo {
	count := 0
	if submission.CurrentRevisionID != nil {
		count = approveCounts[*submission.CurrentRevisionID]
	}
	stage, level := review.StageForApproveCount(count)
	return reviewStageInfo{
		ReviewStage:    stage,
		ReviewLevel:    level,
		RequiredLevels: review.RequiredHumanReviewLevels,
	}
}

// ReviewerResults 列出已定稿(approved / rejected)的 submission,作为审核结果列表。
// 可见性与 ReviewerQueue 完全一致(admin 全量;reviewer 仅其被 task_reviewers 授权的 task)。
// 按 updated_at DESC 排序,id 作为游标键(updated_at 非唯一,以 id 兜底稳定翻页)。
func (h ReviewerHandler) ReviewerResults(c *gin.Context) {
	claims, _ := middleware.Claims(c)
	limit := httpx.CursorLimit(c)

	query := h.db.Model(&model.Submission{}).Where("submissions.status IN ?", finalizedReviewStatuses)
	var scoped bool
	query, scoped = applyReviewQueueScope(query, claims)
	if !scoped {
		httpx.Error(c, http.StatusForbidden, "FORBIDDEN", "review queue access denied")
		return
	}
	if cursor := strings.TrimSpace(c.Query("cursor")); cursor != "" {
		cursorID, err := strconv.ParseUint(cursor, 10, 64)
		if err != nil {
			httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "cursor must be a positive integer")
			return
		}
		query = query.Where("submissions.id < ?", cursorID)
	}

	var submissions []model.Submission
	// 多取一条用于判定 has_more。
	if err := query.Order("submissions.updated_at DESC, submissions.id DESC").Limit(limit + 1).Find(&submissions).Error; err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list review results")
		return
	}
	page := httpx.Page{}
	if len(submissions) > limit {
		submissions = submissions[:limit]
		page.HasMore = true
		page.NextCursor = strconv.FormatUint(submissions[len(submissions)-1].ID, 10)
	}
	finalReviewers, err := h.finalReviewerBySubmission(submissions)
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load review results")
		return
	}
	items := make([]reviewerResultItem, 0, len(submissions))
	for _, submission := range submissions {
		item := reviewerResultItem{
			ID:           submission.ID,
			TaskID:       submission.TaskID,
			ItemID:       submission.ItemID,
			Status:       submission.Status,
			FinalVerdict: submission.HumanVerdict,
			AIScore:      submission.AIScore,
			UpdatedAt:    submission.UpdatedAt,
		}
		if reviewerID, ok := finalReviewers[submission.ID]; ok {
			value := reviewerID
			item.ReviewerID = &value
		}
		items = append(items, item)
	}
	httpx.PageOK(c, items, page)
}

// finalReviewerBySubmission 取每条 submission 最近一次 human_review 的 reviewer_id
// (即作出终态决定的人)。一次查询拉回全部行后在内存里取每个 submission 的首行(已按时间倒序)。
func (h ReviewerHandler) finalReviewerBySubmission(submissions []model.Submission) (map[uint64]uint64, error) {
	out := map[uint64]uint64{}
	if len(submissions) == 0 {
		return out, nil
	}
	submissionIDs := make([]uint64, 0, len(submissions))
	for _, submission := range submissions {
		submissionIDs = append(submissionIDs, submission.ID)
	}
	var reviews []model.HumanReview
	if err := h.db.
		Where("submission_id IN ?", submissionIDs).
		Order("submission_id ASC, created_at DESC, id DESC").
		Find(&reviews).Error; err != nil {
		return nil, err
	}
	for _, r := range reviews {
		if _, seen := out[r.SubmissionID]; !seen {
			out[r.SubmissionID] = r.ReviewerID
		}
	}
	return out, nil
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

	response, err := reviewerAIPromptList(h.db, task)
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list ai prompts")
		return
	}
	httpx.OK(c, response)
}

func (h ReviewerHandler) ReviewSubmission(c *gin.Context) {
	submissionID, ok := parseIDParam(c, "submissionId")
	if !ok {
		return
	}
	var req reviewRequest
	if !bindLimitedJSON(c, &req, maxReviewJSONBytes) {
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

	httpx.OK(c, gin.H{"submission_id": result.SubmissionID, "status": result.Status, "stage": result.Stage})
}

// BatchReview 对一批 submission 逐个调用 review.Apply。
// 多级审核语义:batch approve 每次只把每条 submission advance 一级
// (初审→复审→终审),要凑满 RequiredHumanReviewLevels 级需要多次调用。
// reject / revise 仍是一次到终态。每条结果带回 status + stage。
func (h ReviewerHandler) BatchReview(c *gin.Context) {
	var req batchReviewRequest
	if !bindLimitedJSON(c, &req, maxReviewJSONBytes) {
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
		results = append(results, batchReviewResult{SubmissionID: result.SubmissionID, Status: result.Status, Stage: result.Stage})
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
		return "submission is not reviewable"
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

		var aiReviewRecord model.AIReview
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("submission_id = ? AND revision_id = ?", submission.ID, *submission.CurrentRevisionID).
			Order("created_at DESC, id DESC").
			First(&aiReviewRecord).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errAIRetryNoReview
			}
			return err
		}
		if aiReviewRecord.Status != "failed" && aiReviewRecord.Status != "dead" {
			return errAIRetryReviewState
		}

		var prompt model.AIPromptConfig
		if err := tx.Where("task_id = ? AND version = ?", task.ID, aiReviewRecord.PromptVersion).First(&prompt).Error; err != nil {
			return errAIRetryPromptInvalid
		}
		if !llmreview.AllowedModelName(prompt.Model) {
			return errAIRetryPromptInvalid
		}
		key := reviewerAIReviewIdempotencyKey(submission.ID, *submission.CurrentRevisionID, prompt.ID, prompt.Version)
		if key != aiReviewRecord.IdempotencyKey {
			return errAIRetryKeyMismatch
		}
		payload, err := reviewerAIReviewTaskPayload(submission.ID, *submission.CurrentRevisionID, prompt.ID, prompt.Version, key)
		if err != nil {
			return err
		}

		if err := tx.Model(&model.AIReview{}).
			Where("id = ? AND status IN ?", aiReviewRecord.ID, []string{"failed", "dead"}).
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
		result := tx.Model(&model.Submission{}).
			Where("id = ? AND status = ? AND current_revision_id = ?", submission.ID, statemachine.StateHumanReviewing, *submission.CurrentRevisionID).
			Updates(map[string]any{
				"status":     statemachine.StateAIReviewing,
				"ai_verdict": nil,
				"ai_score":   nil,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return review.ErrConcurrentWrite
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
				"ai_review_id":    aiReviewRecord.ID,
				"idempotency_key": key,
				"prompt_version":  prompt.Version,
			},
		}); err != nil {
			return err
		}

		aiReviewRecord.Status = "pending"
		aiReviewRecord.Verdict = nil
		aiReviewRecord.OverallScore = nil
		aiReviewRecord.Dimensions = nil
		aiReviewRecord.Reason = model.NullString{}
		aiReviewRecord.RawResponse = nil
		aiReviewRecord.TokensInput = 0
		aiReviewRecord.TokensOutput = 0
		aiReviewRecord.LatencyMS = 0
		aiReviewRecord.RetryCount = 0
		aiReviewRecord.ErrorMsg = model.NullString{}
		aiReviewRecord.FinishedAt = model.NullTime{}
		aiReview, err := reviewerAIReviewResponseFromModel(aiReviewRecord, &prompt)
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
	// manual_review(AI 可疑转人工复核)是 reviewer 的初审入口,与 policy 白名单里的状态一样可在详情查看。
	if !policy.CanReviewSubmissionStatus(submission.Status) && submission.Status != statemachine.StateManualReview {
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

	approveCounts, err := h.approveCountsByRevision([]model.Submission{submission})
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to compute review stage")
		return nil, false
	}
	stage := stageInfoFor(submission, approveCounts)

	payload := gin.H{"task": task, "item": item, "template": template, "submission": submission, "revision": revision, "auditLogs": auditLogs,
		"reviewStage": stage.ReviewStage, "reviewLevel": stage.ReviewLevel, "requiredLevels": stage.RequiredLevels}
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

func reviewerAIPromptList(db *gorm.DB, task model.Task) (reviewerAIPromptListResponse, error) {
	var prompts []model.AIPromptConfig
	if err := db.Where("task_id = ?", task.ID).Order("version DESC").Find(&prompts).Error; err != nil {
		return reviewerAIPromptListResponse{}, err
	}
	responses := make([]reviewerAIPromptResponse, 0, len(prompts))
	for _, prompt := range prompts {
		response, err := reviewerAIPromptResponseFromModel(prompt)
		if err != nil {
			return reviewerAIPromptListResponse{}, err
		}
		responses = append(responses, response)
	}
	return reviewerAIPromptListResponse{
		Prompts:         responses,
		ActivePromptID:  task.AIPromptID,
		AIReviewEnabled: task.AIReviewEnabled,
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
