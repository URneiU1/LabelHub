package submission

import (
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"labelhub-api/internal/model"
	"labelhub-api/internal/statemachine"
)

const (
	ItemStatusAvailable = "available"
	ItemStatusClaimed   = "claimed"
	TaskStatusPublished = "published"

	DistributionFirstCome = "first_come"
	DistributionAssigned  = "assigned"
	DistributionQuota     = "quota"
)

var (
	ErrTaskNotFound     = errors.New("submission: task not found")
	ErrTaskNotPublished = errors.New("submission: task is not accepting new claims")
	ErrNoAvailableItem  = errors.New("submission: no available item")
	ErrClaimRaceLost    = errors.New("submission: claim race lost")
	ErrQuotaReached     = errors.New("submission: per-user quota reached")
	ErrNotAssigned      = errors.New("submission: labeler is not assigned to this task")
)

type ClaimInput struct {
	TaskID    uint64
	LabelerID uint64
}

type ClaimResult struct {
	Task       model.Task
	Item       model.TaskItem
	Submission model.Submission
}

// Claim locks the task, claims one item, and creates the draft submission
// in the same transaction. This freezes template_version at claim time.
func Claim(db *gorm.DB, input ClaimInput) (ClaimResult, error) {
	var result ClaimResult
	err := db.Transaction(func(tx *gorm.DB) error {
		var task model.Task
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&task, input.TaskID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTaskNotFound
			}
			return err
		}
		result.Task = task

		var item model.TaskItem
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("task_id = ? AND claimed_by = ? AND status = ?", task.ID, input.LabelerID, ItemStatusClaimed).
			Order("id").
			First(&item).Error
		if err == nil {
			result.Item = item
			sub, err := findOrCreateSubmission(tx, task, item, input.LabelerID)
			result.Submission = sub
			return err
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		if task.Status != TaskStatusPublished {
			return ErrTaskNotPublished
		}

		// 分发策略约束(仅作用于"领新题"路径;已认领的题目重新拉取不受限,见上方早返回)。
		if err := enforceDistribution(tx, task, input.LabelerID); err != nil {
			return err
		}

		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("task_id = ? AND status = ?", task.ID, ItemStatusAvailable).
			Order("id").
			First(&item).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNoAvailableItem
			}
			return err
		}

		res := tx.Model(&model.TaskItem{}).
			Where("id = ? AND status = ? AND claimed_by IS NULL", item.ID, ItemStatusAvailable).
			Updates(map[string]any{
				"status":     ItemStatusClaimed,
				"claimed_by": input.LabelerID,
				"claimed_at": NowUTC(),
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrClaimRaceLost
		}
		item.Status = ItemStatusClaimed
		item.ClaimedBy = &input.LabelerID
		result.Item = item

		version, err := templateVersionForTask(tx, task)
		if err != nil {
			return err
		}
		sub := model.Submission{
			TaskID:          task.ID,
			ItemID:          item.ID,
			TemplateVersion: version,
			LabelerID:       input.LabelerID,
			Status:          statemachine.StateDraft,
		}
		if err := tx.Create(&sub).Error; err != nil {
			return err
		}
		result.Submission = sub
		return nil
	})
	return result, err
}

// enforceDistribution 按任务分发策略决定 labeler 是否可领新题。
//
//	first_come: 不限制(默认)。
//	quota     : QuotaPerUser>0 时,该 labeler 在本任务的 submission 数已达上限则拒绝。
//	assigned  : 仅 task_assignees 里(task 级,item_id 为 NULL)登记的 labeler 可领。
func enforceDistribution(tx *gorm.DB, task model.Task, labelerID uint64) error {
	switch task.Distribution {
	case DistributionQuota:
		if task.QuotaPerUser <= 0 {
			return nil
		}
		var count int64
		if err := tx.Model(&model.Submission{}).
			Where("task_id = ? AND labeler_id = ?", task.ID, labelerID).
			Count(&count).Error; err != nil {
			return err
		}
		if count >= int64(task.QuotaPerUser) {
			return ErrQuotaReached
		}
		return nil
	case DistributionAssigned:
		var count int64
		if err := tx.Model(&model.TaskAssignee{}).
			Where("task_id = ? AND user_id = ? AND item_id IS NULL", task.ID, labelerID).
			Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return ErrNotAssigned
		}
		return nil
	default:
		return nil
	}
}
