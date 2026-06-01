package submission

import (
	"database/sql"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"labelhub-api/internal/model"
	"labelhub-api/internal/statemachine"
)

// NowUTC 包级时间源,便于将来注入固定时间。Phase 3 暂用 time.Now;
// 单测时按需替换。
var NowUTC = func() time.Time { return time.Now().UTC() }

var (
	ErrLeaseExpired                = errors.New("submission: item lease expired")
	ErrDailySubmissionLimitReached = errors.New("submission: daily submission limit reached")
)

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

// findOrCreateSubmission 在 tx 内锁住 submission 行后再生成 revision_no。
// 新 claim 已经会创建 draft submission;这里保留 create 分支用于兼容历史数据。
func findOrCreateSubmission(tx *gorm.DB, task model.Task, item model.TaskItem, labelerID uint64) (model.Submission, error) {
	var submission model.Submission
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("item_id = ? AND labeler_id = ?", item.ID, labelerID).
		First(&submission).Error
	if err == nil {
		if submission.TaskID != task.ID || submission.ItemID != item.ID || submission.LabelerID != labelerID {
			return model.Submission{}, ErrItemNotClaimed
		}
		return submission, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return model.Submission{}, err
	}
	templateVersion, err := templateVersionForTask(tx, task)
	if err != nil {
		return model.Submission{}, err
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

func lockTask(tx *gorm.DB, taskID uint64) (model.Task, error) {
	var task model.Task
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&task, taskID).Error
	return task, err
}

func lockClaimedItem(tx *gorm.DB, task model.Task, itemID uint64, labelerID uint64) (model.TaskItem, error) {
	var item model.TaskItem
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND task_id = ?", itemID, task.ID).
		First(&item).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.TaskItem{}, ErrItemNotClaimed
		}
		return model.TaskItem{}, err
	}
	if item.Status != ItemStatusClaimed || item.ClaimedBy == nil || *item.ClaimedBy != labelerID {
		return model.TaskItem{}, ErrItemNotClaimed
	}
	if leaseExpired(item.ClaimedAt, task.LeaseTimeoutMinutes, NowUTC()) {
		return model.TaskItem{}, ErrLeaseExpired
	}
	return item, nil
}

// releaseExpiredClaims 只回收仍处于编辑态(draft / revising)的过期认领。
//
// 已提交进入审核管线的题目(submission 状态为 submitted / ai_reviewing /
// human_reviewing / needs_arbitration)绝不回收:它们的 claimed_at 在领取时就固定、
// 提交时不刷新,审核耗时一旦超过 lease_timeout_minutes 就会"过期",若一并回收会被
// 第二位标注员重新领取,而旧的 AI / 人审完成逻辑只按 item 仍为 claimed 终结,
// 会把后来重领的题目误判为 finished(见 H-01)。租约只服务"领了但没提交就撂挑子"的场景。
func releaseExpiredClaims(tx *gorm.DB, task model.Task) error {
	if task.LeaseTimeoutMinutes <= 0 {
		return nil
	}
	cutoff := NowUTC().Add(-time.Duration(task.LeaseTimeoutMinutes) * time.Minute)
	editableStates := []string{statemachine.StateDraft, statemachine.StateRevising}
	return tx.Model(&model.TaskItem{}).
		Where("task_id = ? AND status = ? AND claimed_at IS NOT NULL AND claimed_at < ? AND NOT EXISTS (SELECT 1 FROM submissions s WHERE s.item_id = task_items.id AND s.labeler_id = task_items.claimed_by AND s.status NOT IN ?)",
			task.ID, ItemStatusClaimed, cutoff, editableStates).
		Updates(map[string]any{
			"status":     ItemStatusAvailable,
			"claimed_by": nil,
			"claimed_at": nil,
		}).Error
}

func leaseExpired(claimedAt model.NullTime, timeoutMinutes int, now time.Time) bool {
	return timeoutMinutes > 0 && claimedAt.Valid &&
		claimedAt.Time.Before(now.Add(-time.Duration(timeoutMinutes)*time.Minute))
}

func enforceDailySubmissionLimit(tx *gorm.DB, task model.Task, labelerID uint64) error {
	if task.DailySubmissionLimitPerLabeler <= 0 {
		return nil
	}
	now := NowUTC()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	var count int64
	if err := tx.Model(&model.Submission{}).
		Where("task_id = ? AND labeler_id = ? AND submitted_at >= ?", task.ID, labelerID, dayStart).
		Count(&count).Error; err != nil {
		return err
	}
	if count >= int64(task.DailySubmissionLimitPerLabeler) {
		return ErrDailySubmissionLimitReached
	}
	return nil
}

func templateVersionForTask(tx *gorm.DB, task model.Task) (int, error) {
	if task.TemplateID == nil {
		return 1, nil
	}
	template, err := templateByID(tx, task.ID, *task.TemplateID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, ErrTaskTemplate
		}
		return 0, err
	}
	return template.Version, nil
}

func templateByID(tx *gorm.DB, taskID uint64, templateID uint64) (model.TaskTemplate, error) {
	var template model.TaskTemplate
	err := tx.Where("id = ? AND task_id = ?", templateID, taskID).First(&template).Error
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
