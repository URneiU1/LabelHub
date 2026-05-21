package handler

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"labelhub-api/internal/httpx"
	"labelhub-api/internal/middleware"
	"labelhub-api/internal/model"
	"labelhub-api/internal/policy"
	"labelhub-api/internal/statemachine"
)

func (h S1Handler) ListTasks(c *gin.Context) {
	claims, _ := middleware.Claims(c)
	query := h.db.Order("id DESC").Limit(httpx.CursorLimit(c))
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
	if !h.canReadTask(c, task) {
		return
	}
	template, _ := h.currentTemplate(task.ID)
	httpx.OK(c, gin.H{
		"task":     task,
		"template": template,
	})
}

func (h S1Handler) ImportItems(c *gin.Context) {
	task, ok := h.loadOwnedTask(c)
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
			// 缺 id 时用 payload 哈希兜底,避免每次重导致建出重复 task_items。
			// 前缀 "hash:" 让调用方一眼能区分"是我给的 ID"还是"系统兜底生成"。
			if externalID == "" {
				externalID = payloadHashExternalID(raw)
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

func (h S1Handler) ExportJSON(c *gin.Context) {
	task, ok := h.loadOwnedTask(c)
	if !ok {
		return
	}
	claims, _ := middleware.Claims(c)
	includeReviews := c.DefaultQuery("include_reviews", "false") == "true"

	var rows []struct {
		SubmissionID uint64           `json:"submission_id"`
		ItemID       uint64           `json:"item_id"`
		ExternalID   model.NullString `json:"external_id"`
		Payload      string           `json:"payload"`
		Answer       string           `json:"answer"`
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
			"external_id":   nullStringJSON(row.ExternalID),
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

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		export := model.Export{TaskID: task.ID, CreatedBy: claims.UserID, Format: "json", IncludeReviews: includeReviews, Status: "succeeded", RowCount: ptrInt(len(exportRows))}
		if err := tx.Create(&export).Error; err != nil {
			return err
		}
		return createAuditLog(tx, "export", export.ID, "", "succeeded", "user", &claims.UserID, "exported", map[string]any{"task_id": task.ID, "format": "json", "include_reviews": includeReviews})
	}); err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to record export")
		return
	}

	httpx.OK(c, gin.H{"task": task, "rows": exportRows, "include_reviews": includeReviews})
}

// --- task helpers ---

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

func (h S1Handler) loadOwnedTask(c *gin.Context) (model.Task, bool) {
	task, ok := h.loadTask(c)
	if !ok {
		return model.Task{}, false
	}
	claims, _ := middleware.Claims(c)
	if policy.IsTaskOwner(claims, task) {
		return task, true
	}
	httpx.Error(c, http.StatusForbidden, "FORBIDDEN", "task does not belong to current owner")
	return model.Task{}, false
}

func (h S1Handler) canReadTask(c *gin.Context, task model.Task) bool {
	claims, _ := middleware.Claims(c)
	if policy.CanReadTask(claims, task) {
		return true
	}
	httpx.Error(c, http.StatusForbidden, "FORBIDDEN", "task is not visible to current user")
	return false
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

// --- export helpers ---

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

func nextRevisionNo(tx *gorm.DB, submissionID uint64) (int, error) {
	var maxRevision sql.NullInt64
	if err := tx.Model(&model.SubmissionRevision{}).Where("submission_id = ?", submissionID).Select("MAX(revision_no)").Scan(&maxRevision).Error; err != nil {
		return 0, err
	}
	return nextRevisionFromMax(maxRevision.Valid, maxRevision.Int64), nil
}

func nextRevisionFromMax(valid bool, max int64) int {
	if !valid {
		return 1
	}
	return int(max) + 1
}

// payloadHashExternalID 用 payload JSON 的 sha256 前 16 字节(32 hex)生成内部 external_id,
// 确保同一 payload 重复导入命中 unique key (task_id, external_id) 而不是建出重复行。
func payloadHashExternalID(raw []byte) string {
	sum := sha256.Sum256(raw)
	return "hash:" + hex.EncodeToString(sum[:16])
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

func nullStringJSON(value model.NullString) any {
	if !value.Valid {
		return nil
	}
	return value.String
}
