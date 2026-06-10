package handler

import (
	"labelhub-api/internal/auth"
	"labelhub-api/internal/model"
	"labelhub-api/internal/policy"

	"gorm.io/gorm"
)

func canReviewTask(db *gorm.DB, claims *auth.Claims, task model.Task) (bool, error) {
	assigned := false
	if policy.HasRole(claims, policy.RoleReviewer) {
		var count int64
		if err := db.Model(&model.TaskReviewer{}).
			Where("task_id = ? AND user_id = ?", task.ID, claims.UserID).
			Count(&count).Error; err != nil {
			return false, err
		}
		assigned = count > 0
	}
	return policy.CanReviewTask(claims, task, assigned), nil
}

func applyReviewQueueScope(query *gorm.DB, claims *auth.Claims) (*gorm.DB, bool) {
	if policy.HasRole(claims, policy.RoleAdmin) {
		return query, true
	}
	if !policy.HasRole(claims, policy.RoleReviewer) {
		return query, false
	}

	query = query.Joins("LEFT JOIN task_reviewers ON task_reviewers.task_id = submissions.task_id AND task_reviewers.user_id = ?", claims.UserID)
	return query.Where("task_reviewers.user_id IS NOT NULL"), true
}
