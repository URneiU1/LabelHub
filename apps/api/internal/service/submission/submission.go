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

	"gorm.io/gorm"

	"labelhub-api/internal/model"
	"labelhub-api/internal/service/audit"
	"labelhub-api/internal/statemachine"
)

// 业务错误分类。handler 把这些映射到 HTTP 码:
//
//	ErrInvalidSubmit / ErrDraftAfterSubmit → 409 / 422
//	其他(数据库错误)                      → 500
var (
	ErrInvalidSubmit     = errors.New("submission: cannot submit from current state")
	ErrDraftAfterSubmit  = errors.New("submission: draft can only be saved before first submit")
	ErrInvalidTransition = errors.New("submission: invalid state machine transition")
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
		sub, err := findOrCreateSubmission(db, tx, input.Task, input.Item, input.UserID)
		if err != nil {
			return err
		}
		from := sub.Status

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

		to := from
		submitEvent := ""
		dispatchEvent := ""
		updates := map[string]any{"current_revision_id": revision.ID}
		if !input.Draft {
			if from != statemachine.StateDraft && from != statemachine.StateRevising {
				return ErrInvalidSubmit
			}
			if err := statemachine.Apply(from, statemachine.EventSubmit, statemachine.StateSubmitted); err != nil {
				return fmt.Errorf("%w: %s --submit--> submitted", ErrInvalidTransition, from)
			}
			submitEvent = statemachine.EventSubmit
			to = statemachine.StateSubmitted
			if input.Task.AIReviewEnabled {
				to = statemachine.StateAIReviewing
				dispatchEvent = statemachine.EventEnqueue
			} else {
				to = statemachine.StateHumanReviewing
				dispatchEvent = statemachine.EventSkipAI
			}
			if err := statemachine.Apply(statemachine.StateSubmitted, dispatchEvent, to); err != nil {
				return fmt.Errorf("%w: submitted --%s--> %s", ErrInvalidTransition, dispatchEvent, to)
			}
			for key, value := range ResubmitClearedFields(to, NowUTC()) {
				updates[key] = value
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
		if input.Task.AIReviewEnabled && !input.Draft {
			outbox := model.OutboxEvent{
				Topic:   "ai.review.requested",
				Payload: fmt.Sprintf(`{"submission_id":%d,"revision_id":%d}`, sub.ID, revision.ID),
				Status:  "pending",
			}
			if err := tx.Create(&outbox).Error; err != nil {
				return err
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
			if err := audit.Write(tx, audit.LogEntry{
				EntityType: "submission",
				EntityID:   sub.ID,
				FromState:  statemachine.StateSubmitted,
				ToState:    to,
				ActorType:  "user",
				ActorID:    &actorID,
				Event:      dispatchEvent,
			}); err != nil {
				return err
			}
		}
		return tx.First(&response, sub.ID).Error
	})
	return response, err
}
