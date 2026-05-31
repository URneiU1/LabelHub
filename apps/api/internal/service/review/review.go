// Package review 封装 reviewer 审批(approve / reject / revise)的事务编排。
//
// 多级人工审核:approve 需要 RequiredHumanReviewLevels(=3)次,
// 依次落到 first(初审)/ second(复审)/ final(终审)三个 stage。
// 当前 stage 由"当前 revision 已记录的 approve 数"派生,不依赖额外的列:
// 0 → first,1 → second,2 → final。revise 让 labeler 重提产生新 revision,
// approve 计数随之归零(per current revision),故 stage 自动重置。
//
// Apply() 在单一事务内:
//   - 中间级 approve(第 1、2 次):INSERT human_reviews(stage)+ INSERT audit_logs,
//     submission 停留在 human_reviewing(乐观锁确认未被抢先终态化)。
//   - 终审 approve(第 3 次):INSERT human_reviews + UPDATE submissions(approved + approved_at)
//   - UPDATE task_items(finished)+ UPDATE tasks(finished_items += 1)+ INSERT audit_logs。
//   - reject(任意 stage):同终态路径但少 task.finished_items 那一步,落到 rejected。
//   - revise(任意 stage):INSERT human_reviews + UPDATE submissions(revising)+ INSERT audit_logs,
//     不动 task_items / tasks。
package review

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"labelhub-api/internal/auth"
	"labelhub-api/internal/model"
	"labelhub-api/internal/policy"
	"labelhub-api/internal/service/audit"
	"labelhub-api/internal/statemachine"
)

// task_items.status 字面值。review service 自带一份避免反向依赖 handler 包。
const (
	itemStatusClaimed          = "claimed"
	itemStatusFinished         = "finished"
	itemStatusNeedsArbitration = "needs_arbitration"
)

// RequiredHumanReviewLevels 多级人工审核的总级数:初审 → 复审 → 终审。
// 只有第 RequiredHumanReviewLevels 次 approve 才把 submission 推到 approved;
// 前面的 approve 只记录 human_reviews 行并停留在 human_reviewing。
const RequiredHumanReviewLevels = 3

// 三级 stage 字面值。HumanReview.Stage 列存这三个值之一。
const (
	StageFirst  = "first"  // 初审
	StageSecond = "second" // 复审
	StageFinal  = "final"  // 终审
)

// stageByApproveCount 把"当前 revision 已有的 approve 数"映射到本次 approve 落到的 stage 与 level。
// 0 → first(初审,level 1);1 → second(复审,level 2);2 及以上 → final(终审,level 3)。
var stageByApproveCount = []struct {
	stage string
	level int
}{
	{StageFirst, 1},
	{StageSecond, 2},
	{StageFinal, 3},
}

// StageForApproveCount 暴露 approveCount → (stage, level) 给 handler 与单测复用。
// approveCount 是当前 revision 已记录的 approve 数(即下一次 approve 将落到的级别)。
func StageForApproveCount(approveCount int) (stage string, level int) {
	if approveCount < 0 {
		approveCount = 0
	}
	if approveCount >= len(stageByApproveCount) {
		approveCount = len(stageByApproveCount) - 1
	}
	entry := stageByApproveCount[approveCount]
	return entry.stage, entry.level
}

var (
	// ErrSubmissionNotFound:submission_id 不存在。handler 映射 404。
	ErrSubmissionNotFound = errors.New("review: submission not found")
	// ErrInvalidTransition:状态机不允许此变更。handler 映射 422。
	ErrInvalidTransition = errors.New("review: invalid state machine transition")
	// ErrNoRevision:submission 还没有任何 revision,无法审。handler 映射 409。
	ErrNoRevision = errors.New("review: submission has no revision")
	// ErrInvalidVerdict:verdict 既不是 approve 也不是 reject 也不是 revise。handler 映射 400。
	ErrInvalidVerdict = errors.New("review: verdict must be approve, reject, or revise")
	// ErrForbidden:当前 reviewer 未被授权审核该 submission。handler 映射 403。
	ErrForbidden = errors.New("review: reviewer is not allowed for this task")
	// ErrConcurrentWrite:并发写导致 RowsAffected != 1。handler 映射 409。
	ErrConcurrentWrite = errors.New("review: concurrent write detected")
)

// NowUTC 包级时间源,便于将来注入固定时间。
var NowUTC = func() time.Time { return time.Now().UTC() }

// ApplyInput:Apply 需要的输入。
type ApplyInput struct {
	SubmissionID uint64
	Verdict      string // "approve" | "reject" | "revise"
	Reason       string
	ReviewerID   uint64
	Roles        []string
}

// ApplyResult:Apply 成功时返回给 handler 的快照(只含响应用得到的字段)。
type ApplyResult struct {
	SubmissionID uint64
	Status       string // 写入后的最终态:approved / rejected / revising / human_reviewing(中间级 approve)
	Stage        string // 本次 review 落到的 stage:first / second / final
}

// Apply 跑完整事务。所有错误都是 sentinel error,handler 用 errors.Is 分类。
func Apply(db *gorm.DB, input ApplyInput) (ApplyResult, error) {
	event, to, humanVerdict, ok := decode(input.Verdict)
	if !ok {
		return ApplyResult{}, ErrInvalidVerdict
	}

	var result ApplyResult
	err := db.Transaction(func(tx *gorm.DB) error {
		var probe model.Submission
		if err := tx.First(&probe, input.SubmissionID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrSubmissionNotFound
			}
			return err
		}
		var task model.Task
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&task, probe.TaskID).Error; err != nil {
			return err
		}

		var submission model.Submission
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&submission, input.SubmissionID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrSubmissionNotFound
			}
			return err
		}
		isArbitration := submission.Status == statemachine.StateNeedsArbitration
		if submission.Status != statemachine.StateHumanReviewing && !isArbitration {
			return ErrInvalidTransition
		}
		if submission.CurrentRevisionID == nil {
			return ErrNoRevision
		}
		if ok, err := canReviewLocked(tx, input, task); err != nil {
			return err
		} else if !ok {
			return ErrForbidden
		}

		// 本次 review 落到哪个 stage 由"当前 revision 已有的 approve 数"决定。
		// approve 路径下,前 RequiredHumanReviewLevels-1 次只记录 human_reviews 并停在 human_reviewing,
		// 第 RequiredHumanReviewLevels 次才真正推到 approved。
		var approveCount int64
		if !isArbitration {
			if err := tx.Model(&model.HumanReview{}).
				Where("submission_id = ? AND revision_id = ? AND verdict = ?", submission.ID, *submission.CurrentRevisionID, "approve").
				Count(&approveCount).Error; err != nil {
				return err
			}
		}
		stage, _ := StageForApproveCount(int(approveCount))
		if isArbitration {
			stage = StageFinal
		}

		// 中间级 approve:advance 一级,不变更 submission 状态。
		isFinalApprove := humanVerdict == "approve" && (isArbitration || int(approveCount) >= RequiredHumanReviewLevels-1)
		isIntermediateApprove := humanVerdict == "approve" && !isFinalApprove

		// 只有真正发生状态变更的路径才校验状态机(中间级 approve 不变状态,跳过)。
		if !isIntermediateApprove {
			if err := statemachine.Apply(submission.Status, event, to); err != nil {
				return ErrInvalidTransition
			}
		}

		human := model.HumanReview{
			SubmissionID: submission.ID,
			RevisionID:   *submission.CurrentRevisionID,
			ReviewerID:   input.ReviewerID,
			Stage:        stage,
			Verdict:      humanVerdict,
			Reason:       model.StringFrom(input.Reason),
		}
		if err := tx.Create(&human).Error; err != nil {
			return err
		}
		now := NowUTC()
		actorID := input.ReviewerID

		if isIntermediateApprove {
			// 中间级 approve:只 advance stage,submission 停留在 human_reviewing。
			// 用乐观锁 WHERE status 保证并发安全(虽然此处不改 status,仍需确认未被其他 reviewer 抢先终态化)。
			res := tx.Model(&model.Submission{}).
				Where("id = ? AND status = ?", submission.ID, submission.Status).
				Update("updated_at", now)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected != 1 {
				return ErrConcurrentWrite
			}
			if err := audit.Write(tx, audit.LogEntry{
				EntityType: "submission",
				EntityID:   submission.ID,
				FromState:  submission.Status,
				ToState:    submission.Status,
				ActorType:  "user",
				ActorID:    &actorID,
				Event:      "human_approve_stage",
				Payload: map[string]any{
					"reason": input.Reason,
					"stage":  stage,
				},
			}); err != nil {
				return err
			}
			result = ApplyResult{SubmissionID: submission.ID, Status: submission.Status, Stage: stage}
			return nil
		}

		updates := UpdatesFor(to, humanVerdict, now)
		res := tx.Model(&model.Submission{}).
			Where("id = ? AND status = ?", submission.ID, submission.Status).
			Updates(updates)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrConcurrentWrite
		}
		if to == statemachine.StateApproved || to == statemachine.StateRejected {
			expectedItemStatus := itemStatusClaimed
			if isArbitration {
				expectedItemStatus = itemStatusNeedsArbitration
				if err := tx.Model(&model.Submission{}).
					Where("item_id = ? AND id <> ? AND status = ?", submission.ItemID, submission.ID, statemachine.StateNeedsArbitration).
					Updates(map[string]any{"status": statemachine.StateRejected, "human_verdict": "reject"}).Error; err != nil {
					return err
				}
			}
			res := tx.Model(&model.TaskItem{}).Where("id = ? AND status = ?", submission.ItemID, expectedItemStatus).Updates(map[string]any{
				"status":      itemStatusFinished,
				"finished_at": now,
			})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected != 1 {
				return ErrConcurrentWrite
			}
			if to == statemachine.StateApproved {
				res := tx.Model(&model.Task{}).Where("id = ?", submission.TaskID).Update("finished_items", gorm.Expr("finished_items + 1"))
				if res.Error != nil {
					return res.Error
				}
				if res.RowsAffected != 1 {
					return ErrConcurrentWrite
				}
			}
		}
		if err := audit.Write(tx, audit.LogEntry{
			EntityType: "submission",
			EntityID:   submission.ID,
			FromState:  submission.Status,
			ToState:    to,
			ActorType:  "user",
			ActorID:    &actorID,
			Event:      event,
			Payload:    map[string]any{"reason": input.Reason, "stage": stage},
		}); err != nil {
			return err
		}
		result = ApplyResult{SubmissionID: submission.ID, Status: to, Stage: stage}
		return nil
	})
	if err != nil {
		return ApplyResult{}, err
	}
	return result, nil
}

func canReviewLocked(tx *gorm.DB, input ApplyInput, task model.Task) (bool, error) {
	claims := &auth.Claims{UserID: input.ReviewerID, Roles: input.Roles}
	assigned := false
	if policy.HasRole(claims, policy.RoleReviewer) {
		var count int64
		if err := tx.Model(&model.TaskReviewer{}).
			Where("task_id = ? AND user_id = ?", task.ID, input.ReviewerID).
			Count(&count).Error; err != nil {
			return false, err
		}
		assigned = count > 0
	}
	return policy.CanReviewTask(claims, task, assigned), nil
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
