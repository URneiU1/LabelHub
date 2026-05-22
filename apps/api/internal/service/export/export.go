// Package export 封装任务结果导出。
//
// Sprint 1 只做同步 JSON,Sprint 4 在此基础上扩 JSONL / CSV / XLSX(+ Markdown 加分),
// 字段映射 / include_reviews / pagination 是 Sprint 4 的扩展点。
package export

import (
	"gorm.io/gorm"

	"labelhub-api/internal/model"
	"labelhub-api/internal/service/audit"
	"labelhub-api/internal/statemachine"
)

// JSONInput:RunJSON 的入参。Task 已被 handler 做过 owner-check 才传进来。
type JSONInput struct {
	Task           model.Task
	CreatedBy      uint64
	IncludeReviews bool
}

// JSONResult:RunJSON 返回的视图,handler 直接序列化给前端。
type JSONResult struct {
	Task           model.Task
	Rows           []map[string]any
	IncludeReviews bool
}

// RunJSON 执行 JSON 同步导出全流程:join 数据 → 拼 DTO → 写 exports / audit_logs。
func RunJSON(db *gorm.DB, input JSONInput) (JSONResult, error) {
	rows, submissionIDs, err := loadApprovedRows(db, input.Task.ID)
	if err != nil {
		return JSONResult{}, err
	}

	var aiBySubmission map[uint64]model.AIReview
	var humanBySubmission map[uint64]model.HumanReview
	if input.IncludeReviews && len(submissionIDs) > 0 {
		aiBySubmission, err = latestAIReviewBySubmission(db, submissionIDs)
		if err != nil {
			return JSONResult{}, err
		}
		humanBySubmission, err = latestHumanReviewBySubmission(db, submissionIDs)
		if err != nil {
			return JSONResult{}, err
		}
	}

	exportRows := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		entry := map[string]any{
			"submission_id": row.SubmissionID,
			"item_id":       row.ItemID,
			"external_id":   nullStringJSON(row.ExternalID),
			"payload":       DecodeJSONFallback(row.Payload),
			"answer":        DecodeJSONFallback(row.Answer),
		}
		if input.IncludeReviews {
			if ai, ok := aiBySubmission[row.SubmissionID]; ok {
				entry["ai_review"] = AIReviewToMap(ai)
			} else {
				entry["ai_review"] = nil
			}
			if human, ok := humanBySubmission[row.SubmissionID]; ok {
				entry["human_review"] = HumanReviewToMap(human)
			} else {
				entry["human_review"] = nil
			}
		}
		exportRows = append(exportRows, entry)
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		rowCount := len(exportRows)
		export := model.Export{
			TaskID:         input.Task.ID,
			CreatedBy:      input.CreatedBy,
			Format:         "json",
			IncludeReviews: input.IncludeReviews,
			Status:         "succeeded",
			RowCount:       &rowCount,
		}
		if err := tx.Create(&export).Error; err != nil {
			return err
		}
		actorID := input.CreatedBy
		return audit.Write(tx, audit.LogEntry{
			EntityType: "export",
			EntityID:   export.ID,
			ToState:    "succeeded",
			ActorType:  "user",
			ActorID:    &actorID,
			Event:      "exported",
			Payload:    map[string]any{"task_id": input.Task.ID, "format": "json", "include_reviews": input.IncludeReviews},
		})
	})
	if err != nil {
		return JSONResult{}, err
	}

	return JSONResult{
		Task:           input.Task,
		Rows:           exportRows,
		IncludeReviews: input.IncludeReviews,
	}, nil
}

// approvedRow 是 join 三表后扁平化的行,只在本包内流转。
type approvedRow struct {
	SubmissionID uint64           `json:"submission_id"`
	ItemID       uint64           `json:"item_id"`
	ExternalID   model.NullString `json:"external_id"`
	Payload      string           `json:"payload"`
	Answer       string           `json:"answer"`
}

func loadApprovedRows(db *gorm.DB, taskID uint64) ([]approvedRow, []uint64, error) {
	var rows []approvedRow
	err := db.Table("submissions").
		Select("submissions.id AS submission_id, task_items.id AS item_id, task_items.external_id, task_items.payload, submission_revisions.answer").
		Joins("JOIN task_items ON task_items.id = submissions.item_id").
		Joins("JOIN submission_revisions ON submission_revisions.id = submissions.current_revision_id").
		Where("submissions.task_id = ? AND submissions.status = ?", taskID, statemachine.StateApproved).
		Scan(&rows).Error
	if err != nil {
		return nil, nil, err
	}
	ids := make([]uint64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.SubmissionID)
	}
	return rows, ids, nil
}

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
