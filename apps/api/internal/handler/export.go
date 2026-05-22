package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"labelhub-api/internal/httpx"
	"labelhub-api/internal/middleware"
	"labelhub-api/internal/model"
	"labelhub-api/internal/statemachine"
)

// ExportHandler 封装 /tasks/:taskId/export/* 端点。
// Sprint 1 只做同步 JSON 导出,Sprint 4 扩 JSONL / CSV / XLSX(+Markdown 加分)+ 异步队列 + 字段映射 UI。
type ExportHandler struct {
	db *gorm.DB
}

func NewExportHandler(db *gorm.DB) ExportHandler {
	return ExportHandler{db: db}
}

func (h ExportHandler) Register(api gin.IRouter) {
	api.GET("/tasks/:taskId/export/json", middleware.RequireRoles("owner", "admin"), h.ExportJSON)
}

func (h ExportHandler) ExportJSON(c *gin.Context) {
	task, ok := loadOwnedTask(h.db, c)
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
		aiBySubmission, err = latestAIReviewBySubmission(h.db, submissionIDs)
		if err != nil {
			httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load ai reviews")
			return
		}
		humanBySubmission, err = latestHumanReviewBySubmission(h.db, submissionIDs)
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

// --- export helpers(包级,便于将来给 Sprint 4 多格式导出器复用)---

func latestAIReviewBySubmission(db *gorm.DB, submissionIDs []uint64) (map[uint64]model.AIReview, error) {
	var reviews []model.AIReview
	if err := db.Where("submission_id IN ?", submissionIDs).Order("id DESC").Find(&reviews).Error; err != nil {
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

func latestHumanReviewBySubmission(db *gorm.DB, submissionIDs []uint64) (map[uint64]model.HumanReview, error) {
	var reviews []model.HumanReview
	if err := db.Where("submission_id IN ?", submissionIDs).Order("id DESC").Find(&reviews).Error; err != nil {
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
