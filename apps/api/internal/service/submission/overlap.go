package submission

import (
	"encoding/json"

	"gorm.io/gorm"

	"labelhub-api/internal/model"
	"labelhub-api/internal/service/audit"
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

func decideOverlapOutcome(task model.Task, itemID uint64, priorAnswers []string, currentAnswer []byte, excludedFields []string) overlapOutcome {
	if len(priorAnswers)+1 < requiredOverlapForItem(task, itemID) {
		return overlapWaiting
	}
	current := canonicalConsensusAnswer(currentAnswer, excludedFields)
	for _, prior := range priorAnswers {
		if string(canonicalConsensusAnswer([]byte(prior), excludedFields)) != string(current) {
			return overlapNeedsArbitration
		}
	}
	return overlapConsensus
}

func canonicalConsensusAnswer(raw []byte, excludedFields []string) []byte {
	var answer map[string]any
	if err := json.Unmarshal(raw, &answer); err != nil {
		return raw
	}
	for _, field := range excludedFields {
		delete(answer, field)
	}
	normalized, err := json.Marshal(answer)
	if err != nil {
		return raw
	}
	return normalized
}

func transitionConsensusPeers(tx *gorm.DB, itemID uint64, currentSubmissionID uint64) error {
	var peerIDs []uint64
	if err := tx.Model(&model.Submission{}).
		Where("item_id = ? AND id <> ? AND status = ?", itemID, currentSubmissionID, statemachine.StateSubmitted).
		Order("id ASC").
		Pluck("id", &peerIDs).Error; err != nil {
		return err
	}
	for _, peerID := range peerIDs {
		if err := statemachine.Apply(statemachine.StateSubmitted, statemachine.EventConsensusEvidence, statemachine.StateConsensusEvidence); err != nil {
			return err
		}
		result := tx.Model(&model.Submission{}).
			Where("id = ? AND status = ?", peerID, statemachine.StateSubmitted).
			Update("status", statemachine.StateConsensusEvidence)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrClaimRaceLost
		}
		if err := audit.Write(tx, audit.LogEntry{
			EntityType: "submission",
			EntityID:   peerID,
			FromState:  statemachine.StateSubmitted,
			ToState:    statemachine.StateConsensusEvidence,
			ActorType:  "system",
			Event:      statemachine.EventConsensusEvidence,
			Payload:    map[string]any{"winner_submission_id": currentSubmissionID},
		}); err != nil {
			return err
		}
	}
	return nil
}

func transitionArbitrationPeers(tx *gorm.DB, itemID uint64, currentSubmissionID uint64) error {
	var peerIDs []uint64
	if err := tx.Model(&model.Submission{}).
		Where("item_id = ? AND id <> ? AND status = ?", itemID, currentSubmissionID, statemachine.StateSubmitted).
		Order("id ASC").
		Pluck("id", &peerIDs).Error; err != nil {
		return err
	}
	for _, peerID := range peerIDs {
		if err := statemachine.Apply(statemachine.StateSubmitted, statemachine.EventConsensusConflict, statemachine.StateNeedsArbitration); err != nil {
			return err
		}
		result := tx.Model(&model.Submission{}).
			Where("id = ? AND status = ?", peerID, statemachine.StateSubmitted).
			Update("status", statemachine.StateNeedsArbitration)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrClaimRaceLost
		}
		if err := audit.Write(tx, audit.LogEntry{
			EntityType: "submission",
			EntityID:   peerID,
			FromState:  statemachine.StateSubmitted,
			ToState:    statemachine.StateNeedsArbitration,
			ActorType:  "system",
			Event:      statemachine.EventConsensusConflict,
			Payload:    map[string]any{"conflict_submission_id": currentSubmissionID},
		}); err != nil {
			return err
		}
	}
	return nil
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
