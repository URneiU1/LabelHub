package submission

import (
	"database/sql"
	"errors"
	"time"

	"gorm.io/gorm"

	"labelhub-api/internal/model"
	"labelhub-api/internal/statemachine"
)

// NowUTC 包级时间源,便于将来注入固定时间。Phase 3 暂用 time.Now;
// 单测时按需替换。
var NowUTC = func() time.Time { return time.Now().UTC() }

// ResubmitClearedFields:revising → submit 时同事务必须把 ai_verdict / ai_score
// / human_verdict 三字段清空,否则旧 AI 判定会污染新一轮。同样 status / submitted_at 在这里设置。
// 导出以便单测复用(原 handler 包就在 s1_test.go 里测过这条契约)。
func ResubmitClearedFields(to string, now time.Time) map[string]any {
	return map[string]any{
		"status":        to,
		"submitted_at":  now,
		"ai_verdict":    nil,
		"ai_score":      nil,
		"human_verdict": nil,
	}
}

// findOrCreateSubmission 历史实现:模板版本查询走外层 db(非 tx),Phase 3 严格保持原行为。
// 将来若需要"刚 publish 的 task 立即领单"严格读 tx 内可见的 template,再统一改。
func findOrCreateSubmission(db *gorm.DB, tx *gorm.DB, task model.Task, item model.TaskItem, labelerID uint64) (model.Submission, error) {
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
		if template, err := templateByID(db, task.ID, *task.TemplateID); err == nil {
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

func templateByID(db *gorm.DB, taskID uint64, templateID uint64) (model.TaskTemplate, error) {
	var template model.TaskTemplate
	err := db.Where("id = ? AND task_id = ?", templateID, taskID).First(&template).Error
	return template, err
}

func nextRevisionNo(tx *gorm.DB, submissionID uint64) (int, error) {
	var maxRevision sql.NullInt64
	if err := tx.Model(&model.SubmissionRevision{}).Where("submission_id = ?", submissionID).Select("MAX(revision_no)").Scan(&maxRevision).Error; err != nil {
		return 0, err
	}
	return NextRevisionFromMax(maxRevision.Valid, maxRevision.Int64), nil
}

// NextRevisionFromMax:append-only revision_no 自增的纯函数。导出以便单测。
// 契约:无历史 → 1;有历史 max=N → N+1。命中 UK(submission_id, revision_no) 后由 DB 保证唯一。
func NextRevisionFromMax(valid bool, max int64) int {
	if !valid {
		return 1
	}
	return int(max) + 1
}
