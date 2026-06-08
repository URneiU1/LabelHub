// Package acceptance 实现 Owner 数据验收的强闭环:对某任务当前「已通过(approved)」数据
// 做快照批次,Owner 抽检后「验收通过 / 不通过」。验收不通过会把抽检标记为 flag 且当前仍
// approved 的提交打回人工复审(approved -> human_reviewing),并同步回退其 task_item
// (finished -> claimed、finished_items - 1),让重审再通过能正常走完、统计也保持一致。
//
// 设计要点:
//   - 每任务至多一个 pending 批次(Start 时锁任务行串行化 + count 守卫)。
//   - 所有写操作在一个事务内完成,状态翻转走乐观锁(WHERE 旧态 + RowsAffected==1)。
//   - 验收只是状态标记,不闸导出(导出仍读 approved 数据)。
//   - AI 仍绝不自动通过:这里只在 Owner 显式验收不通过时打回,且走状态机校验。
package acceptance

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"labelhub-api/internal/model"
	"labelhub-api/internal/service/audit"
	"labelhub-api/internal/statemachine"
)

// 批次状态。
const (
	StatusPending  = "pending"
	StatusAccepted = "accepted"
	StatusRejected = "rejected"
)

// 抽检结果。
const (
	ResultOK   = "ok"
	ResultFlag = "flag"
)

// task_items.status 字面值。本包自带一份,避免反向依赖 handler / review 包。
const (
	itemStatusClaimed  = "claimed"
	itemStatusFinished = "finished"
)

// NowUTC 包级时间源,便于将来注入固定时间(与 review 包一致)。
var NowUTC = func() time.Time { return time.Now().UTC() }

var (
	// ErrActiveBatchExists:该任务已有 pending 批次,不能重复发起。handler 映射 409。
	ErrActiveBatchExists = errors.New("acceptance: a pending batch already exists for this task")
	// ErrBatchNotFound:批次不存在。handler 映射 404。
	ErrBatchNotFound = errors.New("acceptance: batch not found")
	// ErrBatchNotPending:批次已被验收/拒绝,不能再操作。handler 映射 409。
	ErrBatchNotPending = errors.New("acceptance: batch is not pending")
	// ErrBatchTaskMismatch:批次不属于当前任务(路径越权)。handler 映射 404。
	ErrBatchTaskMismatch = errors.New("acceptance: batch does not belong to task")
	// ErrSubmissionNotEligible:抽检目标不是该任务下的已通过提交。handler 映射 400/404。
	ErrSubmissionNotEligible = errors.New("acceptance: submission is not an approved item of this task")
	// ErrInvalidResult:抽检结果非法。handler 映射 400。
	ErrInvalidResult = errors.New("acceptance: spot-check result must be ok or flag")
	// ErrConcurrentWrite:并发写导致 RowsAffected 不符预期。handler 映射 409。
	ErrConcurrentWrite = errors.New("acceptance: concurrent write detected")
)

// RejectResult 是 Reject 的返回:刷新后的批次 + 实际打回重审的提交数。
type RejectResult struct {
	Batch         model.AcceptanceBatch
	ReopenedCount int
}

// Start 发起一次验收:对任务当前已通过数据建一个 pending 快照批次。
func Start(db *gorm.DB, taskID, ownerID uint64) (model.AcceptanceBatch, error) {
	var batch model.AcceptanceBatch
	err := db.Transaction(func(tx *gorm.DB) error {
		// 锁任务行,串行化同任务的并发 Start,避免两个 pending 批次。
		var task model.Task
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&task, taskID).Error; err != nil {
			return err
		}
		var pendingCount int64
		if err := tx.Model(&model.AcceptanceBatch{}).
			Where("task_id = ? AND status = ?", taskID, StatusPending).
			Count(&pendingCount).Error; err != nil {
			return err
		}
		if pendingCount > 0 {
			return ErrActiveBatchExists
		}
		var approvedCount int64
		if err := tx.Model(&model.Submission{}).
			Where("task_id = ? AND status = ?", taskID, statemachine.StateApproved).
			Count(&approvedCount).Error; err != nil {
			return err
		}
		batch = model.AcceptanceBatch{
			TaskID:        taskID,
			Status:        StatusPending,
			ApprovedCount: int(approvedCount),
			CreatedBy:     ownerID,
		}
		if err := tx.Create(&batch).Error; err != nil {
			return err
		}
		return audit.Write(tx, audit.LogEntry{
			EntityType: "acceptance_batch",
			EntityID:   batch.ID,
			ToState:    StatusPending,
			ActorType:  "owner",
			ActorID:    &ownerID,
			Event:      "acceptance_start",
			Payload:    map[string]any{"task_id": taskID, "approved_count": int(approvedCount)},
		})
	})
	return batch, err
}

// RecordSpotCheck 记录/更新对某条已通过提交的抽检结果(batch_id+submission_id 唯一,重测覆盖)。
func RecordSpotCheck(db *gorm.DB, taskID, batchID, submissionID uint64, result, note string, checkerID uint64) (model.AcceptanceSpotCheck, error) {
	if result != ResultOK && result != ResultFlag {
		return model.AcceptanceSpotCheck{}, ErrInvalidResult
	}
	var check model.AcceptanceSpotCheck
	err := db.Transaction(func(tx *gorm.DB) error {
		batch, err := loadPendingBatch(tx, taskID, batchID)
		if err != nil {
			return err
		}
		// 提交必须属于该任务且当前已通过。
		var sub model.Submission
		if err := tx.Where("id = ? AND task_id = ?", submissionID, batch.TaskID).First(&sub).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrSubmissionNotEligible
			}
			return err
		}
		if sub.Status != statemachine.StateApproved {
			return ErrSubmissionNotEligible
		}
		var existing model.AcceptanceSpotCheck
		findErr := tx.Where("batch_id = ? AND submission_id = ?", batchID, submissionID).First(&existing).Error
		if findErr == nil {
			res := tx.Model(&model.AcceptanceSpotCheck{}).
				Where("id = ?", existing.ID).
				Updates(map[string]any{"result": result, "note": noteValue(note), "checked_by": checkerID})
			if res.Error != nil {
				return res.Error
			}
			return tx.First(&check, existing.ID).Error
		}
		if !errors.Is(findErr, gorm.ErrRecordNotFound) {
			return findErr
		}
		check = model.AcceptanceSpotCheck{
			BatchID:      batchID,
			SubmissionID: submissionID,
			Result:       result,
			Note:         noteValue(note),
			CheckedBy:    checkerID,
		}
		return tx.Create(&check).Error
	})
	return check, err
}

// Accept 验收通过:pending -> accepted。不改任何业务数据,导出不受影响。
func Accept(db *gorm.DB, taskID, batchID, ownerID uint64, note string) (model.AcceptanceBatch, error) {
	var out model.AcceptanceBatch
	err := db.Transaction(func(tx *gorm.DB) error {
		batch, err := loadPendingBatch(tx, taskID, batchID)
		if err != nil {
			return err
		}
		if err := markDecided(tx, batchID, ownerID, note, StatusAccepted); err != nil {
			return err
		}
		if err := audit.Write(tx, audit.LogEntry{
			EntityType: "acceptance_batch",
			EntityID:   batchID,
			FromState:  StatusPending,
			ToState:    StatusAccepted,
			ActorType:  "owner",
			ActorID:    &ownerID,
			Event:      "acceptance_accept",
			Payload:    map[string]any{"task_id": batch.TaskID},
		}); err != nil {
			return err
		}
		return tx.First(&out, batchID).Error
	})
	return out, err
}

// Reject 验收不通过:pending -> rejected,并把抽检 flag 且当前仍 approved 的提交打回人工复审。
func Reject(db *gorm.DB, taskID, batchID, ownerID uint64, note string) (RejectResult, error) {
	var result RejectResult
	err := db.Transaction(func(tx *gorm.DB) error {
		batch, err := loadPendingBatch(tx, taskID, batchID)
		if err != nil {
			return err
		}
		if err := markDecided(tx, batchID, ownerID, note, StatusRejected); err != nil {
			return err
		}

		var flaggedIDs []uint64
		if err := tx.Model(&model.AcceptanceSpotCheck{}).
			Where("batch_id = ? AND result = ?", batchID, ResultFlag).
			Order("submission_id ASC").
			Pluck("submission_id", &flaggedIDs).Error; err != nil {
			return err
		}

		reopened := 0
		for _, sid := range flaggedIDs {
			var sub model.Submission
			loadErr := tx.Where("id = ? AND task_id = ? AND status = ?", sid, batch.TaskID, statemachine.StateApproved).First(&sub).Error
			if errors.Is(loadErr, gorm.ErrRecordNotFound) {
				// 已不再 approved(并发改动),跳过,不打回。
				continue
			}
			if loadErr != nil {
				return loadErr
			}
			if err := statemachine.Apply(statemachine.StateApproved, statemachine.EventAcceptanceReopen, statemachine.StateHumanReviewing); err != nil {
				return err
			}
			subRes := tx.Model(&model.Submission{}).
				Where("id = ? AND status = ?", sub.ID, statemachine.StateApproved).
				Update("status", statemachine.StateHumanReviewing)
			if subRes.Error != nil {
				return subRes.Error
			}
			if subRes.RowsAffected != 1 {
				return ErrConcurrentWrite
			}
			// 独立重审:作废该提交当前 revision 的历史人工审核记录,使打回项必须重新走完整的初审+终审,
			// 而非凭历史 approve 计数被一次复确认即终结。审计历史仍完整保留在 audit_logs。
			if sub.CurrentRevisionID != nil {
				if err := tx.Model(&model.HumanReview{}).Where("submission_id = ? AND revision_id = ? AND superseded_at IS NULL", sub.ID, *sub.CurrentRevisionID).Update("superseded_at", NowUTC()).Error; err != nil {
					return err
				}
			}
			// 同步回退 task_item:finished -> claimed,清 finished_at,finished_items - 1,
			// 否则重审再通过时 review.Apply 的 item 守卫(WHERE status=claimed)会匹配 0 行而失败,且统计虚高。
			itemRes := tx.Model(&model.TaskItem{}).
				Where("id = ? AND status = ?", sub.ItemID, itemStatusFinished).
				Updates(map[string]any{"status": itemStatusClaimed, "finished_at": nil})
			if itemRes.Error != nil {
				return itemRes.Error
			}
			if itemRes.RowsAffected != 1 {
				return ErrConcurrentWrite
			}
			taskRes := tx.Model(&model.Task{}).
				Where("id = ? AND finished_items > 0", batch.TaskID).
				Update("finished_items", gorm.Expr("finished_items - 1"))
			if taskRes.Error != nil {
				return taskRes.Error
			}
			if taskRes.RowsAffected != 1 {
				return ErrConcurrentWrite
			}
			if err := audit.Write(tx, audit.LogEntry{
				EntityType: "submission",
				EntityID:   sub.ID,
				FromState:  statemachine.StateApproved,
				ToState:    statemachine.StateHumanReviewing,
				ActorType:  "owner",
				ActorID:    &ownerID,
				Event:      statemachine.EventAcceptanceReopen,
				Payload:    map[string]any{"acceptance_batch_id": batchID},
			}); err != nil {
				return err
			}
			reopened++
		}
		result.ReopenedCount = reopened

		if err := audit.Write(tx, audit.LogEntry{
			EntityType: "acceptance_batch",
			EntityID:   batchID,
			FromState:  StatusPending,
			ToState:    StatusRejected,
			ActorType:  "owner",
			ActorID:    &ownerID,
			Event:      "acceptance_reject",
			Payload:    map[string]any{"task_id": batch.TaskID, "reopened_count": reopened},
		}); err != nil {
			return err
		}
		return tx.First(&result.Batch, batchID).Error
	})
	return result, err
}

// loadPendingBatch 读批次并校验:存在、属于该任务、且仍 pending。
func loadPendingBatch(tx *gorm.DB, taskID, batchID uint64) (model.AcceptanceBatch, error) {
	var batch model.AcceptanceBatch
	if err := tx.First(&batch, batchID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return batch, ErrBatchNotFound
		}
		return batch, err
	}
	if batch.TaskID != taskID {
		return batch, ErrBatchTaskMismatch
	}
	if batch.Status != StatusPending {
		return batch, ErrBatchNotPending
	}
	return batch, nil
}

// markDecided 用乐观锁把 pending 批次翻到终态(accepted / rejected)。
func markDecided(tx *gorm.DB, batchID, ownerID uint64, note, status string) error {
	res := tx.Model(&model.AcceptanceBatch{}).
		Where("id = ? AND status = ?", batchID, StatusPending).
		Updates(map[string]any{
			"status":     status,
			"decided_by": ownerID,
			"decided_at": NowUTC(),
			"note":       noteValue(note),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return ErrConcurrentWrite
	}
	return nil
}

// noteValue 把可空备注转成 NullString:空白 -> NULL。
func noteValue(note string) model.NullString {
	if strings.TrimSpace(note) == "" {
		return model.NullString{}
	}
	return model.StringFrom(note)
}
