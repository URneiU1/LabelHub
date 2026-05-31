package submission

import (
	"encoding/json"

	"gorm.io/gorm"

	"labelhub-api/internal/model"
	"labelhub-api/internal/statemachine"
)

type overlapOutcome string

const (
	overlapConsensus        overlapOutcome = "consensus"
	overlapWaiting          overlapOutcome = "waiting"
	overlapNeedsArbitration overlapOutcome = "needs_arbitration"
)

func overlapEnabled(task model.Task) bool {
	return task.OverlapCount > 1 && task.OverlapCoveragePct > 0
}

func requiredOverlapForItem(task model.Task, itemID uint64) int {
	if !overlapEnabled(task) {
		return 1
	}
	if int(itemID%100) < task.OverlapCoveragePct {
		return task.OverlapCount
	}
	return 1
}

func decideOverlapOutcome(task model.Task, itemID uint64, priorAnswers []string, currentAnswer []byte) overlapOutcome {
	if len(priorAnswers)+1 < requiredOverlapForItem(task, itemID) {
		return overlapWaiting
	}
	current := canonicalAnswer(currentAnswer)
	for _, prior := range priorAnswers {
		if string(canonicalAnswer([]byte(prior))) != string(current) {
			return overlapNeedsArbitration
		}
	}
	return overlapConsensus
}

func canonicalAnswer(raw []byte) []byte {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return raw
	}
	normalized, err := json.Marshal(value)
	if err != nil {
		return raw
	}
	return normalized
}

func priorOverlapAnswers(tx *gorm.DB, itemID uint64, currentSubmissionID uint64) ([]string, error) {
	var answers []string
	err := tx.Table("submissions").
		Select("submission_revisions.answer").
		Joins("JOIN submission_revisions ON submission_revisions.id = submissions.current_revision_id").
		Where("submissions.item_id = ? AND submissions.id <> ? AND submissions.status <> ?", itemID, currentSubmissionID, statemachine.StateDraft).
		Order("submissions.id ASC").
		Scan(&answers).Error
	return answers, err
}

func releaseOverlapClaim(tx *gorm.DB, itemID uint64, status string) error {
	result := tx.Model(&model.TaskItem{}).
		Where("id = ? AND status = ?", itemID, ItemStatusClaimed).
		Updates(map[string]any{
			"status":     status,
			"claimed_by": nil,
			"claimed_at": nil,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrClaimRaceLost
	}
	return nil
}
