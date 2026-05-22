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
)

var (
	ErrTaskNotFound     = errors.New("submission: task not found")
	ErrTaskNotPublished = errors.New("submission: task is not accepting new claims")
	ErrNoAvailableItem  = errors.New("submission: no available item")
	ErrClaimRaceLost    = errors.New("submission: claim race lost")
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
			version = 1
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
