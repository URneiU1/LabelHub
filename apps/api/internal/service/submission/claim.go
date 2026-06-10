package submission

import (
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"labelhub-api/internal/model"
	"labelhub-api/internal/statemachine"
)

const (
	ItemStatusAvailable        = "available"
	ItemStatusClaimed          = "claimed"
	ItemStatusNeedsArbitration = "needs_arbitration"
	TaskStatusPublished        = "published"

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
	ErrTaskTaken        = errors.New("submission: task already claimed by another labeler")
	ErrWrongClaimMode   = errors.New("submission: task uses per-item claiming")
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
		if err := releaseExpiredClaims(tx, task); err != nil {
			return err
		}

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
		if err := enforceDailySubmissionLimit(tx, task, input.LabelerID); err != nil {
			return err
		}

		itemQuery := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("task_id = ? AND status = ?", task.ID, ItemStatusAvailable)
		if overlapEnabled(task) {
			itemQuery = itemQuery.
				Where("NOT EXISTS (SELECT 1 FROM submissions WHERE submissions.item_id = task_items.id AND submissions.labeler_id = ?)", input.LabelerID).
				Where("(SELECT COUNT(*) FROM submissions WHERE submissions.item_id = task_items.id AND submissions.status <> ?) < CASE WHEN MOD(task_items.id, 100) < ? THEN ? ELSE 1 END",
					statemachine.StateDraft, task.OverlapCoveragePct, task.OverlapCount)
		}
		if err := itemQuery.Order("id").First(&item).Error; err != nil {
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

		// 用 findOrCreateSubmission 而非裸 Create:releaseExpiredClaims 会把过期认领的 item 放回
		// available 却保留其草稿 submission;同一 labeler 再领到同一题时,裸 Create 会撞唯一键
		// (item_id, labeler_id) 报 1062 → 500。复用已有草稿让领取对 (item, labeler) 幂等。
		sub, err := findOrCreateSubmission(tx, task, item, input.LabelerID)
		if err != nil {
			return err
		}
		result.Submission = sub
		return nil
	})
	return result, err
}

// ClaimTask 整体领取一个大任务(first_come / assigned 独占模式):把该任务所有 available 题一次性
// 锁给该 labeler,使其在该任务里所有题全部解锁可做。不预建草稿(草稿在首次作答时按需创建)。
// quota 任务不走这里(按题抢单,用 Claim);该任务已被他人领取(有他人认领题或他人提交)则拒绝。
func ClaimTask(db *gorm.DB, input ClaimInput) (ClaimResult, error) {
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
		if task.Status != TaskStatusPublished {
			return ErrTaskNotPublished
		}
		if task.Distribution == DistributionQuota {
			return ErrWrongClaimMode
		}
		if task.Distribution == DistributionAssigned {
			var assigned int64
			if err := tx.Model(&model.TaskAssignee{}).
				Where("task_id = ? AND user_id = ? AND item_id IS NULL", task.ID, input.LabelerID).
				Count(&assigned).Error; err != nil {
				return err
			}
			if assigned == 0 {
				return ErrNotAssigned
			}
		}
		if err := releaseExpiredClaims(tx, task); err != nil {
			return err
		}

		// 独占校验:任意题被他人认领、或他人在本任务有提交 → 该大任务已被他人领取。
		var othersItems int64
		if err := tx.Model(&model.TaskItem{}).
			Where("task_id = ? AND claimed_by IS NOT NULL AND claimed_by <> ?", task.ID, input.LabelerID).
			Count(&othersItems).Error; err != nil {
			return err
		}
		var othersSubs int64
		if err := tx.Model(&model.Submission{}).
			Where("task_id = ? AND labeler_id <> ?", task.ID, input.LabelerID).
			Count(&othersSubs).Error; err != nil {
			return err
		}
		if othersItems > 0 || othersSubs > 0 {
			return ErrTaskTaken
		}

		// 把所有 available 题一次性锁给我。已是我认领/已提交的题不动。
		if err := tx.Model(&model.TaskItem{}).
			Where("task_id = ? AND status = ?", task.ID, ItemStatusAvailable).
			Updates(map[string]any{
				"status":     ItemStatusClaimed,
				"claimed_by": input.LabelerID,
				"claimed_at": NowUTC(),
			}).Error; err != nil {
			return err
		}
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
