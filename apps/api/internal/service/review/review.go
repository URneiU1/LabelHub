// Package review 封装 reviewer 审批(approve / reject / revise)的事务编排。
//
// Apply() 在单一事务内必须发生 5 件事(approve 路径):
//  1. INSERT human_reviews
//  2. UPDATE submissions(status + human_verdict + approved_at)
//  3. UPDATE task_items(status=finished + finished_at)— approve/reject 都做
//  4. UPDATE tasks(finished_items += 1)— 仅 approve
//  5. INSERT audit_logs
//
// reject 路径少 task.finished_items 那一步;revise 路径不动 task_items / tasks。
package review

import (
	"errors"
	"time"

	"gorm.io/gorm"

	"labelhub-api/internal/model"
	"labelhub-api/internal/service/audit"
	"labelhub-api/internal/statemachine"
)

// itemStatusFinished:跨包共用的字面值。原 handler 包 const,review service 自带一份避免反向依赖。
const itemStatusFinished = "finished"

var (
	// ErrSubmissionNotFound:submission_id 不存在。handler 映射 404。
	ErrSubmissionNotFound = errors.New("review: submission not found")
	// ErrInvalidTransition:状态机不允许此变更。handler 映射 422。
	ErrInvalidTransition = errors.New("review: invalid state machine transition")
	// ErrNoRevision:submission 还没有任何 revision,无法审。handler 映射 409。
	ErrNoRevision = errors.New("review: submission has no revision")
	// ErrInvalidVerdict:verdict 既不是 approve 也不是 reject 也不是 revise。handler 映射 400。
	ErrInvalidVerdict = errors.New("review: verdict must be approve, reject, or revise")
)

// NowUTC 包级时间源,便于将来注入固定时间。
var NowUTC = func() time.Time { return time.Now().UTC() }

// ApplyInput:Apply 需要的输入。
type ApplyInput struct {
	SubmissionID uint64
	Verdict      string // "approve" | "reject" | "revise"
	Reason       string
	ReviewerID   uint64
}

// ApplyResult:Apply 成功时返回给 handler 的快照(只含响应用得到的字段)。
type ApplyResult struct {
	SubmissionID uint64
	Status       string // 写入后的最终态:approved / rejected / revising
}

// Apply 跑完整事务。所有错误都是 sentinel error,handler 用 errors.Is 分类。
func Apply(db *gorm.DB, input ApplyInput) (ApplyResult, error) {
	event, to, humanVerdict, ok := decode(input.Verdict)
	if !ok {
		return ApplyResult{}, ErrInvalidVerdict
	}

	var submission model.Submission
	if err := db.First(&submission, input.SubmissionID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ApplyResult{}, ErrSubmissionNotFound
		}
		return ApplyResult{}, err
	}
	if err := statemachine.Apply(submission.Status, event, to); err != nil {
		return ApplyResult{}, ErrInvalidTransition
	}
	if submission.CurrentRevisionID == nil {
		return ApplyResult{}, ErrNoRevision
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		human := model.HumanReview{
			SubmissionID: submission.ID,
			RevisionID:   *submission.CurrentRevisionID,
			ReviewerID:   input.ReviewerID,
			Stage:        "first",
			Verdict:      humanVerdict,
			Reason:       model.StringFrom(input.Reason),
		}
		if err := tx.Create(&human).Error; err != nil {
			return err
		}
		now := NowUTC()
		updates := UpdatesFor(to, humanVerdict, now)
		if err := tx.Model(&model.Submission{}).Where("id = ?", submission.ID).Updates(updates).Error; err != nil {
			return err
		}
		if to == statemachine.StateApproved || to == statemachine.StateRejected {
			if err := tx.Model(&model.TaskItem{}).Where("id = ?", submission.ItemID).Updates(map[string]any{
				"status":      itemStatusFinished,
				"finished_at": now,
			}).Error; err != nil {
				return err
			}
			if to == statemachine.StateApproved {
				if err := tx.Model(&model.Task{}).Where("id = ?", submission.TaskID).Update("finished_items", gorm.Expr("finished_items + 1")).Error; err != nil {
					return err
				}
			}
		}
		actorID := input.ReviewerID
		return audit.Write(tx, audit.LogEntry{
			EntityType: "submission",
			EntityID:   submission.ID,
			FromState:  submission.Status,
			ToState:    to,
			ActorType:  "user",
			ActorID:    &actorID,
			Event:      event,
			Payload:    map[string]any{"reason": input.Reason},
		})
	})
	if err != nil {
		return ApplyResult{}, err
	}
	return ApplyResult{SubmissionID: submission.ID, Status: to}, nil
}

// UpdatesFor 给 submissions 表的 UPDATE map。approve 必须同时写 approved_at;
// reject / revise 必须 *不* 写 approved_at(保证 dashboard "approved_at 排序" 拿不到 NULL 时序混进来)。
// 导出以便单测复用。
func UpdatesFor(to string, humanVerdict string, now time.Time) map[string]any {
	updates := map[string]any{"status": to, "human_verdict": humanVerdict}
	if to == statemachine.StateApproved {
		updates["approved_at"] = now
	}
	return updates
}

// Decode 暴露 verdict → (event, to, humanVerdict) 的映射给单测。
func Decode(verdict string) (event string, to string, humanVerdict string, ok bool) {
	return decode(verdict)
}

func decode(verdict string) (event string, to string, humanVerdict string, ok bool) {
	switch verdict {
	case "approve":
		return statemachine.EventApprove, statemachine.StateApproved, "approve", true
	case "reject":
		return statemachine.EventReject, statemachine.StateRejected, "reject", true
	case "revise":
		return statemachine.EventRevise, statemachine.StateRevising, "revise", true
	default:
		return "", "", "", false
	}
}
