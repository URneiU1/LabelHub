// Package policy centralizes RBAC + 数据资产隔离决策,handler 只调 policy 不写散落判断。
//
// 设计原则:
//   - 函数纯计算,不接 db / context,所有数据由调用方查好后传入
//   - 决定可见性的核心维度:角色 + 资源归属 + 资源生命周期状态
//   - 标注平台特殊性:labeler 之间互相隔离 raw payload(数据资产);reviewer 只看与他相关的 submission
package policy

import (
	"labelhub-api/internal/auth"
	"labelhub-api/internal/model"
	"labelhub-api/internal/statemachine"
)

const (
	RoleAdmin    = "admin"
	RoleOwner    = "owner"
	RoleLabeler  = "labeler"
	RoleReviewer = "reviewer"

	TaskStatusPublished = "published"

	ItemStatusClaimed = "claimed"
)

// reviewerReadableSubmissionStatuses:reviewer 角色允许查看的 submission 状态白名单。
// 草稿 / 提交中 / AI 审中 / 修订中 都属于 labeler 私域,reviewer 无权窥探 raw payload。
var reviewerReadableSubmissionStatuses = map[string]struct{}{
	statemachine.StateHumanReviewing: {},
	statemachine.StateApproved:       {},
	statemachine.StateRejected:       {},
}

// HasRole 判断 claims 是否携带指定角色。nil-safe。
func HasRole(claims *auth.Claims, role string) bool {
	if claims == nil {
		return false
	}
	for _, r := range claims.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// IsTaskOwner:admin 或 task.OwnerID 匹配的 owner。
func IsTaskOwner(claims *auth.Claims, task model.Task) bool {
	if HasRole(claims, RoleAdmin) {
		return true
	}
	return HasRole(claims, RoleOwner) && task.OwnerID == claims.UserID
}

// CanReadTask:GET /tasks/:id 可见性。
//   - admin:always
//   - owner:仅自己拥有的 task
//   - labeler / reviewer:仅 published task
func CanReadTask(claims *auth.Claims, task model.Task) bool {
	if HasRole(claims, RoleAdmin) {
		return true
	}
	if HasRole(claims, RoleOwner) && task.OwnerID == claims.UserID {
		return true
	}
	if (HasRole(claims, RoleLabeler) || HasRole(claims, RoleReviewer)) && task.Status == TaskStatusPublished {
		return true
	}
	return false
}

// ClaimDecision 描述 ClaimItem 路径 B(尝试领取新题)的判定结果。
type ClaimDecision struct {
	Allowed bool
	// HTTPStatus / Code / Message 用于直接构造错误响应。Allowed=true 时这三个字段为零值。
	HTTPStatus int
	Code       string
	Message    string
}

// CanClaimNew:labeler 尝试领取一道新 item 时的前置校验。
// 已经在干的 item(claimed_by=self)即使 task 暂停也允许 resume,那条路径不走这个函数。
func CanClaimNew(claims *auth.Claims, task model.Task) ClaimDecision {
	if !HasRole(claims, RoleLabeler) {
		return ClaimDecision{HTTPStatus: 403, Code: "FORBIDDEN", Message: "only labeler can claim items"}
	}
	if task.Status != TaskStatusPublished {
		return ClaimDecision{
			HTTPStatus: 409,
			Code:       "CONFLICT",
			Message:    "task is not accepting new claims (status=" + task.Status + ")",
		}
	}
	return ClaimDecision{Allowed: true}
}

// CanReadItem:GET /tasks/:id/items/:id 可见性。
// submission 可为 nil(item 尚未被任何人提交过)。
//   - admin:always
//   - owner:仅自己拥有的 task 下的 item
//   - labeler:仅自己 claim 中的 item(防止 labeler 互相查看 raw payload)
//   - reviewer:仅当存在 submission 且 status ∈ {human_reviewing, approved, rejected}
func CanReadItem(claims *auth.Claims, task model.Task, item model.TaskItem, submission *model.Submission) bool {
	if HasRole(claims, RoleAdmin) {
		return true
	}
	if HasRole(claims, RoleOwner) && task.OwnerID == claims.UserID {
		return true
	}
	if HasRole(claims, RoleLabeler) && item.ClaimedBy != nil && *item.ClaimedBy == claims.UserID {
		return true
	}
	if HasRole(claims, RoleReviewer) && submission != nil {
		if _, ok := reviewerReadableSubmissionStatuses[submission.Status]; ok {
			return true
		}
	}
	return false
}
