package handler

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"labelhub-api/internal/httpx"
	"labelhub-api/internal/middleware"
	"labelhub-api/internal/model"
	"labelhub-api/internal/statemachine"
)

const (
	itemStatusAvailable = "available"
	itemStatusClaimed   = "claimed"
	itemStatusFinished  = "finished"
)

// 上传白名单与上限对齐 PLAN §3.1
const (
	uploadMaxBytes       = 10 * 1024 * 1024
	uploadImageMaxBytes  = 5 * 1024 * 1024
	defaultUploadBaseDir = "./data/uploads"
)

var (
	allowedUploadMIME = map[string]string{
		"image/png":        ".png",
		"image/jpeg":       ".jpg",
		"image/webp":       ".webp",
		"application/pdf":  ".pdf",
		"text/plain":       ".txt",
		"application/json": ".json",
	}
	imageMIMEs = map[string]struct{}{
		"image/png":  {},
		"image/jpeg": {},
		"image/webp": {},
	}
)

type S1Handler struct {
	db *gorm.DB
}

func NewS1Handler(db *gorm.DB) S1Handler {
	return S1Handler{db: db}
}

func (h S1Handler) Register(api gin.IRouter) {
	api.GET("/tasks", middleware.RequireRoles("owner", "admin"), h.ListTasks)
	api.POST("/tasks", middleware.RequireRoles("owner", "admin"), h.CreateTask)
	api.GET("/tasks/:taskId", middleware.RequireRoles("owner", "admin", "labeler", "reviewer"), h.GetTask)
	api.POST("/tasks/:taskId/items/import", middleware.RequireRoles("owner", "admin"), h.ImportItems)
	api.GET("/tasks/:taskId/export/json", middleware.RequireRoles("owner", "admin"), h.ExportJSON)

	api.GET("/labeler/tasks", middleware.RequireRoles("labeler"), h.ListPublishedTasks)
	api.POST("/tasks/:taskId/claim", middleware.RequireRoles("labeler"), h.ClaimItem)
	api.GET("/tasks/:taskId/items/:itemId", middleware.RequireRoles("labeler", "reviewer", "owner", "admin"), h.GetItem)
	api.POST("/tasks/:taskId/items/:itemId/draft", middleware.RequireRoles("labeler"), h.SaveDraft)
	api.POST("/tasks/:taskId/items/:itemId/submit", middleware.RequireRoles("labeler"), h.SubmitItem)
	api.GET("/me/submissions", middleware.RequireRoles("labeler"), h.MySubmissions)

	api.GET("/reviewer/submissions", middleware.RequireRoles("reviewer", "owner", "admin"), h.ReviewerQueue)
	api.POST("/submissions/:submissionId/review", middleware.RequireRoles("reviewer", "owner", "admin"), h.ReviewSubmission)

	api.POST("/llm/inline", middleware.RequireRoles("labeler", "owner", "reviewer", "admin"), h.InlineLLM)
	api.POST("/upload", middleware.RequireRoles("labeler", "owner", "reviewer", "admin"), h.Upload)
	api.POST("/uploads", middleware.RequireRoles("labeler", "owner", "reviewer", "admin"), h.Upload)
}

type createTaskRequest struct {
	Title               string `json:"title" binding:"required"`
	Description         string `json:"description"`
	BaselineDescription string `json:"baselineDescription"`
}

type importItemsRequest struct {
	Items []map[string]any `json:"items" binding:"required"`
}

type answerRequest struct {
	Answer map[string]any `json:"answer" binding:"required"`
}

type reviewRequest struct {
	Verdict string `json:"verdict" binding:"required"`
	Reason  string `json:"reason"`
}

type inlineLLMRequest struct {
	Prompt string         `json:"prompt"`
	Input  map[string]any `json:"input"`
}

func (h S1Handler) ListTasks(c *gin.Context) {
	claims, _ := middleware.Claims(c)
	query := h.db.Order("id DESC").Limit(httpx.CursorLimit(c))
	// admin 看全部;owner 只看自己创建的;混合角色按"admin 优先"放行
	if !hasRole(claims.Roles, "admin") {
		query = query.Where("owner_id = ?", claims.UserID)
	}
	var tasks []model.Task
	if err := query.Find(&tasks).Error; err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list tasks")
		return
	}
	httpx.PageOK(c, tasks, httpx.Page{})
}

func hasRole(roles []string, target string) bool {
	for _, role := range roles {
		if role == target {
			return true
		}
	}
	return false
}

func (h S1Handler) CreateTask(c *gin.Context) {
	var req createTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "title is required")
		return
	}
	claims, _ := middleware.Claims(c)
	task := model.Task{
		OwnerID:             claims.UserID,
		Title:               req.Title,
		Status:              "draft",
		Description:         nullString(req.Description),
		BaselineDescription: nullString(req.BaselineDescription),
		Distribution:        "first_come",
		AIReviewEnabled:     false,
		HumanReviewEnabled:  true,
	}
	if err := h.db.Create(&task).Error; err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to create task")
		return
	}
	httpx.OK(c, task)
}

func (h S1Handler) GetTask(c *gin.Context) {
	task, ok := h.loadTask(c)
	if !ok {
		return
	}
	template, _ := h.currentTemplate(task.ID)
	httpx.OK(c, gin.H{
		"task":     task,
		"template": template,
	})
}

func (h S1Handler) ImportItems(c *gin.Context) {
	task, ok := h.loadTask(c)
	if !ok {
		return
	}
	var req importItemsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "items are required")
		return
	}

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		for _, payload := range req.Items {
			externalID, _ := payload["id"].(string)
			raw, err := json.Marshal(payload)
			if err != nil {
				return err
			}
			item := model.TaskItem{
				TaskID:     task.ID,
				ExternalID: nullString(externalID),
				Payload:    string(raw),
				Status:     itemStatusAvailable,
			}
			if err := tx.Where("task_id = ? AND external_id = ?", task.ID, externalID).FirstOrCreate(&item).Error; err != nil {
				return err
			}
		}
		return tx.Model(&model.Task{}).Where("id = ?", task.ID).Update("total_items", gorm.Expr("(SELECT COUNT(*) FROM task_items WHERE task_id = ?)", task.ID)).Error
	}); err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to import items")
		return
	}
	httpx.OK(c, gin.H{"imported": len(req.Items)})
}

func (h S1Handler) ListPublishedTasks(c *gin.Context) {
	var tasks []model.Task
	if err := h.db.Where("status = ?", "published").Order("id DESC").Find(&tasks).Error; err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list tasks")
		return
	}
	httpx.PageOK(c, tasks, httpx.Page{})
}

func (h S1Handler) ClaimItem(c *gin.Context) {
	task, ok := h.loadTask(c)
	if !ok {
		return
	}
	claims, _ := middleware.Claims(c)

	var claimed model.TaskItem
	err := h.db.Where("task_id = ? AND claimed_by = ? AND status = ?", task.ID, claims.UserID, itemStatusClaimed).Order("id").First(&claimed).Error
	if err == nil {
		h.respondItem(c, task, claimed)
		return
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load claimed item")
		return
	}

	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("task_id = ? AND status = ?", task.ID, itemStatusAvailable).
			Order("id").
			First(&claimed).Error; err != nil {
			return err
		}
		now := time.Now().UTC()
		return tx.Model(&claimed).Updates(map[string]any{
			"status":     itemStatusClaimed,
			"claimed_by": claims.UserID,
			"claimed_at": now,
		}).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		httpx.Error(c, http.StatusConflict, "CONFLICT", "没有可领取的题目")
		return
	}
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to claim item")
		return
	}

	claimed.ClaimedBy = &claims.UserID
	claimed.Status = itemStatusClaimed
	h.respondItem(c, task, claimed)
}

func (h S1Handler) GetItem(c *gin.Context) {
	task, ok := h.loadTask(c)
	if !ok {
		return
	}
	item, ok := h.loadItem(c, task.ID)
	if !ok {
		return
	}
	h.respondItem(c, task, item)
}

func (h S1Handler) SaveDraft(c *gin.Context) {
	h.saveRevision(c, true)
}

func (h S1Handler) SubmitItem(c *gin.Context) {
	h.saveRevision(c, false)
}

func (h S1Handler) MySubmissions(c *gin.Context) {
	claims, _ := middleware.Claims(c)
	var submissions []model.Submission
	if err := h.db.Where("labeler_id = ?", claims.UserID).Order("updated_at DESC").Find(&submissions).Error; err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list submissions")
		return
	}
	httpx.PageOK(c, submissions, httpx.Page{})
}

// reviewerQueueAllowedStatuses 限定审核队列只能看 human_reviewing / revising;
// 防止 reviewer 用 ?status=draft 或 ?status=approved 拉到不该看的中间态 / 终态记录
var reviewerQueueAllowedStatuses = map[string]struct{}{
	statemachine.StateHumanReviewing: {},
	statemachine.StateRevising:       {},
}

func (h S1Handler) ReviewerQueue(c *gin.Context) {
	status := c.DefaultQuery("status", statemachine.StateHumanReviewing)
	if _, ok := reviewerQueueAllowedStatuses[status]; !ok {
		httpx.ErrorWithDetails(c, http.StatusForbidden, "FORBIDDEN",
			"reviewer queue only exposes human_reviewing / revising",
			gin.H{"requested": status, "allowed": []string{statemachine.StateHumanReviewing, statemachine.StateRevising}})
		return
	}
	var submissions []model.Submission
	if err := h.db.Where("status = ?", status).Order("updated_at ASC").Find(&submissions).Error; err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list review queue")
		return
	}
	httpx.PageOK(c, submissions, httpx.Page{})
}

func (h S1Handler) ReviewSubmission(c *gin.Context) {
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
	// reject / revise 必须给原因 — labeler 需要看上一轮意见才能修订
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

func (h S1Handler) ExportJSON(c *gin.Context) {
	task, ok := h.loadTask(c)
	if !ok {
		return
	}
	claims, _ := middleware.Claims(c)
	includeReviews := c.DefaultQuery("include_reviews", "false") == "true"

	var rows []struct {
		SubmissionID uint64 `json:"submission_id"`
		ItemID       uint64 `json:"item_id"`
		ExternalID   string `json:"external_id"`
		Payload      string `json:"payload"`
		Answer       string `json:"answer"`
	}
	err := h.db.Table("submissions").
		Select("submissions.id AS submission_id, task_items.id AS item_id, task_items.external_id, task_items.payload, submission_revisions.answer").
		Joins("JOIN task_items ON task_items.id = submissions.item_id").
		Joins("JOIN submission_revisions ON submission_revisions.id = submissions.current_revision_id").
		Where("submissions.task_id = ? AND submissions.status = ?", task.ID, statemachine.StateApproved).
		Scan(&rows).Error
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to export task")
		return
	}

	// 收集 submission_id 一次性查 latest AI / human review,避免 N+1
	submissionIDs := make([]uint64, 0, len(rows))
	for _, row := range rows {
		submissionIDs = append(submissionIDs, row.SubmissionID)
	}

	var aiBySubmission map[uint64]model.AIReview
	var humanBySubmission map[uint64]model.HumanReview
	if includeReviews && len(submissionIDs) > 0 {
		aiBySubmission, err = h.latestAIReviewBySubmission(submissionIDs)
		if err != nil {
			httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load ai reviews")
			return
		}
		humanBySubmission, err = h.latestHumanReviewBySubmission(submissionIDs)
		if err != nil {
			httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load human reviews")
			return
		}
	}

	exportRows := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		entry := map[string]any{
			"submission_id": row.SubmissionID,
			"item_id":       row.ItemID,
			"external_id":   row.ExternalID,
			"payload":       mustJSON(row.Payload),
			"answer":        mustJSON(row.Answer),
		}
		if includeReviews {
			if ai, ok := aiBySubmission[row.SubmissionID]; ok {
				entry["ai_review"] = aiReviewToMap(ai)
			} else {
				entry["ai_review"] = nil
			}
			if human, ok := humanBySubmission[row.SubmissionID]; ok {
				entry["human_review"] = humanReviewToMap(human)
			} else {
				entry["human_review"] = nil
			}
		}
		exportRows = append(exportRows, entry)
	}

	_ = h.db.Transaction(func(tx *gorm.DB) error {
		export := model.Export{TaskID: task.ID, CreatedBy: claims.UserID, Format: "json", IncludeReviews: includeReviews, Status: "succeeded", RowCount: ptrInt(len(exportRows))}
		if err := tx.Create(&export).Error; err != nil {
			return err
		}
		return createAuditLog(tx, "export", export.ID, "", "succeeded", "user", &claims.UserID, "exported", map[string]any{"task_id": task.ID, "format": "json", "include_reviews": includeReviews})
	})

	httpx.OK(c, gin.H{"task": task, "rows": exportRows, "include_reviews": includeReviews})
}

// 每个 submission 取最新一条 AI review;append-only 表所以 ORDER BY id DESC 等价于 created_at DESC
func (h S1Handler) latestAIReviewBySubmission(submissionIDs []uint64) (map[uint64]model.AIReview, error) {
	var reviews []model.AIReview
	if err := h.db.Where("submission_id IN ?", submissionIDs).Order("id DESC").Find(&reviews).Error; err != nil {
		return nil, err
	}
	out := make(map[uint64]model.AIReview, len(submissionIDs))
	for _, review := range reviews {
		if _, seen := out[review.SubmissionID]; !seen {
			out[review.SubmissionID] = review
		}
	}
	return out, nil
}

func (h S1Handler) latestHumanReviewBySubmission(submissionIDs []uint64) (map[uint64]model.HumanReview, error) {
	var reviews []model.HumanReview
	if err := h.db.Where("submission_id IN ?", submissionIDs).Order("id DESC").Find(&reviews).Error; err != nil {
		return nil, err
	}
	out := make(map[uint64]model.HumanReview, len(submissionIDs))
	for _, review := range reviews {
		if _, seen := out[review.SubmissionID]; !seen {
			out[review.SubmissionID] = review
		}
	}
	return out, nil
}

func aiReviewToMap(review model.AIReview) map[string]any {
	return map[string]any{
		"verdict":        review.Verdict,
		"overall_score":  review.OverallScore,
		"dimensions":     unwrapJSONPointer(review.Dimensions),
		"reason":         review.Reason,
		"prompt_version": review.PromptVersion,
		"created_at":     review.CreatedAt,
	}
}

func humanReviewToMap(review model.HumanReview) map[string]any {
	return map[string]any{
		"verdict":      review.Verdict,
		"reason":       review.Reason,
		"stage":        review.Stage,
		"reviewer_id":  review.ReviewerID,
		"created_at":   review.CreatedAt,
	}
}

func unwrapJSONPointer(raw *string) any {
	if raw == nil {
		return nil
	}
	return mustJSON(*raw)
}

func (h S1Handler) InlineLLM(c *gin.Context) {
	var req inlineLLMRequest
	_ = c.ShouldBindJSON(&req)
	time.Sleep(800 * time.Millisecond)
	text := fmt.Sprintf("AI 预评分:建议重点检查相关性、准确性、格式合规与安全性。输入长度=%d。", len(req.Prompt)+len(fmt.Sprint(req.Input)))
	httpx.OK(c, gin.H{"text": text, "provider": "mock"})
}

func (h S1Handler) Upload(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "file is required")
		return
	}
	taskID, _ := strconv.ParseUint(c.PostForm("task_id"), 10, 64)
	if taskID == 0 {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "task_id is required")
		return
	}

	mime := file.Header.Get("Content-Type")
	ext, ok := allowedUploadMIME[mime]
	if !ok {
		httpx.ErrorWithDetails(c, http.StatusBadRequest, "VALIDATION_ERROR",
			"unsupported MIME type", gin.H{"mime": mime, "allowed": allowedMIMEKeys()})
		return
	}
	if _, isImage := imageMIMEs[mime]; isImage && file.Size > uploadImageMaxBytes {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "image must be <= 5MB")
		return
	}
	if file.Size > uploadMaxBytes {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "file must be <= 10MB")
		return
	}

	claims, _ := middleware.Claims(c)
	key := storageKey(file.Filename) + ext
	uploadDir := envOrDefault("UPLOAD_DIR", defaultUploadBaseDir)
	dest := filepath.Join(uploadDir, strconv.FormatUint(taskID, 10), key)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to prepare upload dir")
		return
	}
	if err := c.SaveUploadedFile(file, dest); err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to write upload")
		return
	}

	uploaded := model.UploadedFile{
		TaskID:       taskID,
		StorageKey:   key,
		OriginalName: filepath.Base(file.Filename),
		MimeType:     mime,
		SizeBytes:    uint64(file.Size),
		Status:       "temp",
		CreatedBy:    claims.UserID,
	}
	if err := h.db.Create(&uploaded).Error; err != nil {
		// 元数据写库失败时清掉已落盘文件,避免孤儿
		_ = os.Remove(dest)
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to save upload metadata")
		return
	}
	httpx.OK(c, uploaded)
}

func envOrDefault(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func allowedMIMEKeys() []string {
	keys := make([]string, 0, len(allowedUploadMIME))
	for k := range allowedUploadMIME {
		keys = append(keys, k)
	}
	return keys
}

func (h S1Handler) saveRevision(c *gin.Context, draft bool) {
	task, ok := h.loadTask(c)
	if !ok {
		return
	}
	item, ok := h.loadItem(c, task.ID)
	if !ok {
		return
	}
	claims, _ := middleware.Claims(c)
	if item.ClaimedBy == nil || *item.ClaimedBy != claims.UserID {
		httpx.Error(c, http.StatusForbidden, "FORBIDDEN", "item is not claimed by current user")
		return
	}
	var req answerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "answer is required")
		return
	}
	answerJSON, err := json.Marshal(req.Answer)
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "answer must be valid JSON")
		return
	}

	var response model.Submission
	err = h.db.Transaction(func(tx *gorm.DB) error {
		submission, err := h.findOrCreateSubmission(tx, task, item, claims.UserID)
		if err != nil {
			return err
		}
		from := submission.Status

		revisionNo, err := nextRevisionNo(tx, submission.ID)
		if err != nil {
			return err
		}
		revision := model.SubmissionRevision{
			SubmissionID: submission.ID,
			RevisionNo:   revisionNo,
			Answer:       string(answerJSON),
			Draft:        draft,
			CreatedBy:    claims.UserID,
		}
		if err := tx.Create(&revision).Error; err != nil {
			return err
		}

		to := from
		// submitEvent / dispatchEvent 仅在 !draft 路径有意义,留空字符串作为"未跨态"标记
		submitEvent := ""
		dispatchEvent := ""
		updates := map[string]any{"current_revision_id": revision.ID}
		if !draft {
			if from != statemachine.StateDraft && from != statemachine.StateRevising {
				return fmt.Errorf("submission cannot be submitted from %s", from)
			}
			if err := statemachine.Apply(from, statemachine.EventSubmit, statemachine.StateSubmitted); err != nil {
				return err
			}
			submitEvent = statemachine.EventSubmit
			to = statemachine.StateSubmitted
			if task.AIReviewEnabled {
				to = statemachine.StateAIReviewing
				dispatchEvent = statemachine.EventEnqueue
			} else {
				to = statemachine.StateHumanReviewing
				dispatchEvent = statemachine.EventSkipAI
			}
			if err := statemachine.Apply(statemachine.StateSubmitted, dispatchEvent, to); err != nil {
				return err
			}
			for key, value := range resubmitClearedFields(to, time.Now().UTC()) {
				updates[key] = value
			}
		} else if from != statemachine.StateDraft {
			return fmt.Errorf("draft can only be saved before first submit")
		}
		if draft && from == statemachine.StateDraft {
			if err := statemachine.Apply(from, statemachine.EventSave, statemachine.StateDraft); err != nil {
				return err
			}
		}

		if err := tx.Model(&model.Submission{}).Where("id = ?", submission.ID).Updates(updates).Error; err != nil {
			return err
		}
		if task.AIReviewEnabled && !draft {
			outbox := model.OutboxEvent{Topic: "ai.review.requested", Payload: fmt.Sprintf(`{"submission_id":%d,"revision_id":%d}`, submission.ID, revision.ID), Status: "pending"}
			if err := tx.Create(&outbox).Error; err != nil {
				return err
			}
		}
		if !draft {
			// PLAN §11"audit_log 覆盖所有迁移":跨态写 2 条 — submit 进 submitted、再 enqueue/skip_ai 进 ai/human_reviewing
			if err := createAuditLog(tx, "submission", submission.ID, from, statemachine.StateSubmitted, "user", &claims.UserID, submitEvent, nil); err != nil {
				return err
			}
			if err := createAuditLog(tx, "submission", submission.ID, statemachine.StateSubmitted, to, "user", &claims.UserID, dispatchEvent, nil); err != nil {
				return err
			}
		}
		return tx.First(&response, submission.ID).Error
	})
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to save answer")
		return
	}
	httpx.OK(c, response)
}

func (h S1Handler) findOrCreateSubmission(tx *gorm.DB, task model.Task, item model.TaskItem, labelerID uint64) (model.Submission, error) {
	var submission model.Submission
	err := tx.Where("item_id = ?", item.ID).First(&submission).Error
	if err == nil {
		return submission, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return model.Submission{}, err
	}
	templateVersion := 1
	if task.TemplateID != nil {
		if template, err := h.currentTemplate(task.ID); err == nil {
			templateVersion = template.Version
		}
	}
	submission = model.Submission{
		TaskID:          task.ID,
		ItemID:          item.ID,
		TemplateVersion: templateVersion,
		LabelerID:       labelerID,
		Status:          statemachine.StateDraft,
	}
	return submission, tx.Create(&submission).Error
}

func (h S1Handler) respondItem(c *gin.Context, task model.Task, item model.TaskItem) {
	template, _ := h.currentTemplate(task.ID)
	var submission model.Submission
	_ = h.db.Where("item_id = ?", item.ID).First(&submission).Error
	var revision *model.SubmissionRevision
	if submission.ID != 0 && submission.CurrentRevisionID != nil {
		var current model.SubmissionRevision
		if err := h.db.First(&current, *submission.CurrentRevisionID).Error; err == nil {
			revision = &current
		}
	}
	httpx.OK(c, gin.H{"task": task, "item": item, "template": template, "submission": submission, "revision": revision})
}

func (h S1Handler) loadTask(c *gin.Context) (model.Task, bool) {
	taskID, ok := parseIDParam(c, "taskId")
	if !ok {
		return model.Task{}, false
	}
	var task model.Task
	if err := h.db.First(&task, taskID).Error; err != nil {
		httpx.Error(c, http.StatusNotFound, "NOT_FOUND", "task not found")
		return model.Task{}, false
	}
	return task, true
}

func (h S1Handler) loadItem(c *gin.Context, taskID uint64) (model.TaskItem, bool) {
	itemID, ok := parseIDParam(c, "itemId")
	if !ok {
		return model.TaskItem{}, false
	}
	var item model.TaskItem
	if err := h.db.Where("id = ? AND task_id = ?", itemID, taskID).First(&item).Error; err != nil {
		httpx.Error(c, http.StatusNotFound, "NOT_FOUND", "item not found")
		return model.TaskItem{}, false
	}
	return item, true
}

func (h S1Handler) currentTemplate(taskID uint64) (model.TaskTemplate, error) {
	var template model.TaskTemplate
	err := h.db.Where("task_id = ?", taskID).Order("version DESC").First(&template).Error
	return template, err
}

func parseIDParam(c *gin.Context, name string) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil || id == 0 {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", name+" must be a positive integer")
		return 0, false
	}
	return id, true
}

func nullString(value string) model.NullString {
	return model.StringFrom(value)
}

func nextRevisionNo(tx *gorm.DB, submissionID uint64) (int, error) {
	var maxRevision sql.NullInt64
	if err := tx.Model(&model.SubmissionRevision{}).Where("submission_id = ?", submissionID).Select("MAX(revision_no)").Scan(&maxRevision).Error; err != nil {
		return 0, err
	}
	return nextRevisionFromMax(maxRevision.Valid, maxRevision.Int64), nil
}

// nextRevisionFromMax 是 nextRevisionNo 的纯逻辑部分,便于单测
func nextRevisionFromMax(valid bool, max int64) int {
	if !valid {
		return 1
	}
	return int(max) + 1
}

// resubmitClearedFields 返回 submission 重新提交时需要更新的字段
// PLAN §4.3:revising | submit → submitted(新 revision_no,清空 ai_verdict 重走);ai_score、human_verdict 同样要清
func resubmitClearedFields(to string, now time.Time) map[string]any {
	return map[string]any{
		"status":        to,
		"submitted_at":  now,
		"ai_verdict":    nil,
		"ai_score":      nil,
		"human_verdict": nil,
	}
}

// reviewUpdates 返回 reviewer 处理完一条提交后,submissions 表要 UPDATE 的字段
// PLAN §4.3:approve → approved + approved_at;reject → rejected;revise → revising
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

func createAuditLog(tx *gorm.DB, entityType string, entityID uint64, from string, to string, actorType string, actorID *uint64, event string, payload map[string]any) error {
	var payloadValue *string
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		value := string(raw)
		payloadValue = &value
	}

	audit := model.AuditLog{
		EntityType: entityType,
		EntityID:   entityID,
		FromState:  nullString(from),
		ToState:    to,
		ActorType:  actorType,
		ActorID:    actorID,
		Event:      event,
		Payload:    payloadValue,
	}
	return tx.Create(&audit).Error
}

func mustJSON(raw string) any {
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return raw
	}
	return value
}

func ptrInt(value int) *int {
	return &value
}

func storageKey(name string) string {
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", name, time.Now().UnixNano())))
	return hex.EncodeToString(hash[:])
}
