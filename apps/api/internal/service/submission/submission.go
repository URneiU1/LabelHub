// Package submission 封装 labeler 写作答(draft / submit)的事务编排。
//
// 入口 Save() 是纯领域服务:
//   - 输入域对象(Task / TaskItem / Answer / UserID / Draft),不接 gin / httpx
//   - 输出更新后的 Submission 与一组分类错误,handler 层根据错误类型映射 HTTP 码
//   - 整段逻辑在 *单一事务* 内完成:findOrCreate submission + 写 revision +
//     状态机迁移 + outbox + audit log,保证一致性
//
// Phase 3 抽取自原 LabelerHandler.saveRevision,业务行为零改动。
package submission

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"labelhub-api/internal/model"
	"labelhub-api/internal/service/audit"
	"labelhub-api/internal/statemachine"
	"labelhub.local/reviewsampling"
)

// 业务错误分类。handler 把这些映射到 HTTP 码:
//
//	ErrInvalidSubmit / ErrDraftAfterSubmit → 409 / 422
//	其他(数据库错误)                      → 500
var (
	ErrInvalidSubmit     = errors.New("submission: cannot submit from current state")
	ErrDraftAfterSubmit  = errors.New("submission: draft can only be saved before first submit")
	ErrInvalidTransition = errors.New("submission: invalid state machine transition")
	ErrItemNotClaimed    = errors.New("submission: item is not claimed by current user")
	ErrInvalidAIPrompt   = errors.New("submission: active AI prompt is invalid")
	ErrTaskTemplate      = errors.New("submission: task template is not available")
)

// SaveInput:Save 调用所需要的全部领域数据。
// Now 由调用方注入便于测试(注入固定时间戳)。
type SaveInput struct {
	Task      model.Task
	Item      model.TaskItem
	AnswerRaw []byte // 已序列化的 JSON,handler 在校验阶段已 marshal 过
	UserID    uint64
	Draft     bool
}

// Save 执行 draft / submit 全流程。返回写入后的 submission 快照。
func Save(db *gorm.DB, input SaveInput) (model.Submission, error) {
	var response model.Submission
	err := db.Transaction(func(tx *gorm.DB) error {
		task, err := lockTask(tx, input.Task.ID)
		if err != nil {
			return err
		}
		item, err := lockClaimedItem(tx, task, input.Item.ID, input.UserID)
		if err != nil {
			return err
		}
		sub, err := findOrCreateSubmission(tx, task, item, input.UserID)
		if err != nil {
			return err
		}
		from := sub.Status
		if !input.Draft && from == statemachine.StateDraft {
			if err := enforceDailySubmissionLimit(tx, task, input.UserID); err != nil {
				return err
			}
		}

		revisionNo, err := nextRevisionNo(tx, sub.ID)
		if err != nil {
			return err
		}
		revision := model.SubmissionRevision{
			SubmissionID: sub.ID,
			RevisionNo:   revisionNo,
			Answer:       string(input.AnswerRaw),
			Draft:        input.Draft,
			CreatedBy:    input.UserID,
		}
		if err := tx.Create(&revision).Error; err != nil {
			return err
		}
		if !input.Draft {
			if err := attachUploadedFiles(tx, task, sub, revision, input.AnswerRaw, input.UserID); err != nil {
				return err
			}
		}
		overlap := overlapConsensus
		// overlap 共识是 labeler *首次提交* 的准入闸口,只在 from=draft 时判定。
		// 一旦该 submission 过了共识进入审核管线、被 reviewer 打回(revise),其 resubmit
		// (from=revising)不得再重跑 overlap——否则会把已归档的 consensus_evidence 同伴答案
		// 重新拉来比较,把单纯的"改完重审"误判成 needs_arbitration / overlapWaiting(F-1)。
		// revise 后的重提直接重走 AI/人工审核。
		overlapRequired := !input.Draft && from == statemachine.StateDraft && requiredOverlapForItem(task, item.ID) > 1
		if overlapRequired {
			excludedFields, err := loadFileUploadFieldNames(tx, task.ID, sub.TemplateVersion)
			if err != nil {
				return err
			}
			priorAnswers, err := priorOverlapAnswers(tx, item.ID, sub.ID)
			if err != nil {
				return err
			}
			overlap = decideOverlapOutcome(task, item.ID, priorAnswers, input.AnswerRaw, excludedFields)
		}
		aiPlan := aiReviewPlan{}
		if !input.Draft && overlap == overlapConsensus {
			var err error
			aiPlan, err = buildAIReviewPlan(tx, task, sub, revision)
			if err != nil {
				return err
			}
		}

		to := from
		submitEvent := ""
		dispatchEvent := ""
		now := NowUTC()
		updates := map[string]any{"current_revision_id": revision.ID}
		if !input.Draft {
			if from != statemachine.StateDraft && from != statemachine.StateRevising {
				return ErrInvalidSubmit
			}
			if err := validateSubmitAnswer(tx, task, sub.TemplateVersion, input.AnswerRaw); err != nil {
				return err
			}
			if err := statemachine.Apply(from, statemachine.EventSubmit, statemachine.StateSubmitted); err != nil {
				return fmt.Errorf("%w: %s --submit--> submitted", ErrInvalidTransition, from)
			}
			submitEvent = statemachine.EventSubmit
			to = statemachine.StateSubmitted
			for key, value := range ResubmitClearedFields(statemachine.StateSubmitted, now) {
				updates[key] = value
			}
			switch overlap {
			case overlapConsensus:
				if aiPlan.Enabled {
					to = statemachine.StateAIReviewing
					dispatchEvent = statemachine.EventEnqueue
				} else if !task.HumanReviewEnabled || !reviewsampling.ShouldReview(task.ID, item.ID, input.UserID, task.ReviewSamplingPct) {
					to = statemachine.StateApproved
					dispatchEvent = statemachine.EventSamplingAutoApproved
					updates["approved_at"] = now
				} else {
					to = statemachine.StateHumanReviewing
					dispatchEvent = statemachine.EventSkipAI
				}
				if err := statemachine.Apply(statemachine.StateSubmitted, dispatchEvent, to); err != nil {
					return fmt.Errorf("%w: submitted --%s--> %s", ErrInvalidTransition, dispatchEvent, to)
				}
				updates["status"] = to
			case overlapNeedsArbitration:
				to = statemachine.StateNeedsArbitration
				if err := statemachine.Apply(statemachine.StateSubmitted, statemachine.EventConsensusConflict, to); err != nil {
					return fmt.Errorf("%w: submitted --%s--> %s", ErrInvalidTransition, statemachine.EventConsensusConflict, to)
				}
				updates["status"] = to
			}
		} else if from != statemachine.StateDraft {
			return ErrDraftAfterSubmit
		}
		if input.Draft && from == statemachine.StateDraft {
			if err := statemachine.Apply(from, statemachine.EventSave, statemachine.StateDraft); err != nil {
				return fmt.Errorf("%w: %s --save--> draft", ErrInvalidTransition, from)
			}
		}

		if err := tx.Model(&model.Submission{}).Where("id = ?", sub.ID).Updates(updates).Error; err != nil {
			return err
		}
		if !input.Draft && overlap == overlapConsensus {
			if overlapRequired {
				if err := transitionConsensusPeers(tx, item.ID, sub.ID); err != nil {
					return err
				}
			}
			if err := createPendingAIReview(tx, sub, revision, aiPlan); err != nil {
				return err
			}
			if err := createAIReviewOutbox(tx, aiPlan); err != nil {
				return err
			}
		}
		if !input.Draft && overlap == overlapConsensus && to == statemachine.StateApproved {
			if err := finishAutoApprovedItem(tx, task.ID, item.ID, now); err != nil {
				return err
			}
		}
		if !input.Draft {
			switch overlap {
			case overlapWaiting:
				if err := releaseOverlapClaim(tx, item.ID, ItemStatusAvailable); err != nil {
					return err
				}
			case overlapNeedsArbitration:
				if err := tx.Model(&model.Submission{}).
					Where("item_id = ? AND id <> ? AND status = ?", item.ID, sub.ID, statemachine.StateSubmitted).
					Update("status", statemachine.StateNeedsArbitration).Error; err != nil {
					return err
				}
				if err := releaseOverlapClaim(tx, item.ID, ItemStatusNeedsArbitration); err != nil {
					return err
				}
			}
		}
		if !input.Draft {
			actorID := input.UserID
			if err := audit.Write(tx, audit.LogEntry{
				EntityType: "submission",
				EntityID:   sub.ID,
				FromState:  from,
				ToState:    statemachine.StateSubmitted,
				ActorType:  "user",
				ActorID:    &actorID,
				Event:      submitEvent,
			}); err != nil {
				return err
			}
			if dispatchEvent != "" {
				// 派发(enqueue / skip_ai)是系统按任务配置自动决策,不是用户动作,
				// 因此审计记为 system + 无 actor_id,避免审计轨迹误导。
				if err := audit.Write(tx, audit.LogEntry{
					EntityType: "submission",
					EntityID:   sub.ID,
					FromState:  statemachine.StateSubmitted,
					ToState:    to,
					ActorType:  "system",
					ActorID:    nil,
					Event:      dispatchEvent,
				}); err != nil {
					return err
				}
			}
			if overlap == overlapNeedsArbitration {
				if err := audit.Write(tx, audit.LogEntry{
					EntityType: "submission",
					EntityID:   sub.ID,
					FromState:  statemachine.StateSubmitted,
					ToState:    statemachine.StateNeedsArbitration,
					ActorType:  "system",
					ActorID:    nil,
					Event:      statemachine.EventConsensusConflict,
				}); err != nil {
					return err
				}
			}
		}
		return tx.First(&response, sub.ID).Error
	})
	return response, err
}

func finishAutoApprovedItem(tx *gorm.DB, taskID uint64, itemID uint64, now time.Time) error {
	result := tx.Model(&model.TaskItem{}).
		Where("id = ? AND status = ?", itemID, ItemStatusClaimed).
		Updates(map[string]any{"status": "finished", "finished_at": now})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrClaimRaceLost
	}
	result = tx.Model(&model.Task{}).Where("id = ?", taskID).Update("finished_items", gorm.Expr("finished_items + 1"))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrClaimRaceLost
	}
	return nil
}
