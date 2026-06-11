package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"labelhub-api/internal/httpx"
	"labelhub-api/internal/middleware"
	"labelhub-api/internal/model"
	"labelhub-api/internal/policy"
)

// task_reviewer 收纳 Owner 视角对 task_reviewers 的指派管理:列出已指派/候选审核员、
// 指派、解除。仅 task owner / admin 可操作(走 loadOwnedTask),被指派者必须具备 reviewer 角色。

type addReviewersRequest struct {
	UserIDs []uint64 `json:"userIds" binding:"required"`
}

type reviewerView struct {
	UserID     uint64 `json:"userId"`
	AssignedAt string `json:"assignedAt"`
}

type reviewerCandidateView struct {
	UserID      uint64 `json:"userId"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
}

// RegisterReviewers 在 TaskHandler 既有路由组上追加审核员指派端点。
// 与 assignees 一致,只对 owner / admin 开放。
func (h TaskHandler) RegisterReviewers(api gin.IRouter) {
	api.GET("/tasks/:taskId/reviewers", middleware.RequireRoles("owner", "admin"), h.ListReviewers)
	api.GET("/tasks/:taskId/reviewer-candidates", middleware.RequireRoles("owner", "admin"), h.ListReviewerCandidates)
	api.POST("/tasks/:taskId/reviewers", middleware.RequireRoles("owner", "admin"), h.AddReviewers)
	api.DELETE("/tasks/:taskId/reviewers/:userId", middleware.RequireRoles("owner", "admin"), h.RemoveReviewer)
}

// ListReviewers 返回该任务已指派的审核员。
func (h TaskHandler) ListReviewers(c *gin.Context) {
	task, ok := loadOwnedTask(h.db, c)
	if !ok {
		return
	}
	var reviewers []model.TaskReviewer
	if err := h.db.Where("task_id = ?", task.ID).Order("user_id ASC").Find(&reviewers).Error; err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list reviewers")
		return
	}
	out := make([]reviewerView, 0, len(reviewers))
	for _, r := range reviewers {
		out = append(out, reviewerView{UserID: r.UserID, AssignedAt: r.AssignedAt.UTC().Format("2006-01-02T15:04:05Z07:00")})
	}
	httpx.OK(c, gin.H{"reviewers": out})
}

// ListReviewerCandidates 返回可被指派的审核员(role=reviewer 且 active),
// 供 owner 从列表里选人而不是手输用户 ID。
func (h TaskHandler) ListReviewerCandidates(c *gin.Context) {
	if _, ok := loadOwnedTask(h.db, c); !ok {
		return
	}
	users, err := reviewerCandidateUsers(h.db)
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list reviewer candidates")
		return
	}
	out := make([]reviewerCandidateView, 0, len(users))
	for _, u := range users {
		out = append(out, reviewerCandidateView{UserID: u.ID, Username: u.Username, DisplayName: u.DisplayName})
	}
	httpx.OK(c, gin.H{"candidates": out})
}

// AddReviewers 把一组用户指派为该任务的审核员。重复指派幂等(FirstOrCreate);
// 任一用户不具备 reviewer 角色则整批拒绝(422),避免把无审核权限的人放进 task_reviewers。
func (h TaskHandler) AddReviewers(c *gin.Context) {
	task, ok := loadOwnedTask(h.db, c)
	if !ok {
		return
	}
	var req addReviewersRequest
	if !bindLimitedJSON(c, &req, maxTaskInfoBytes) {
		return
	}
	if len(req.UserIDs) == 0 {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "userIds must not be empty")
		return
	}
	if len(req.UserIDs) > maxAssigneesPerRequest {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "too many userIds in a single request")
		return
	}
	for _, uid := range req.UserIDs {
		if uid == 0 {
			httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "userIds must be positive integers")
			return
		}
	}

	// 校验被指派用户确实具备 reviewer 角色:统计落在 reviewer 角色里的去重用户数,
	// 不等于请求里的去重用户数即有人不是 reviewer,整批拒绝。
	wantIDs := uniqueUint64(req.UserIDs)
	var reviewerCount int64
	if err := h.db.Model(&model.UserRole{}).
		Where("role = ? AND user_id IN ?", policy.RoleReviewer, wantIDs).
		Distinct("user_id").
		Count(&reviewerCount).Error; err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to verify reviewer roles")
		return
	}
	if int(reviewerCount) != len(wantIDs) {
		httpx.Error(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "all assignees must have the reviewer role")
		return
	}

	claims, _ := middleware.Claims(c)
	assignedBy := claims.UserID
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		for _, uid := range wantIDs {
			reviewer := model.TaskReviewer{
				TaskID:     task.ID,
				UserID:     uid,
				AssignedBy: &assignedBy,
			}
			if err := tx.Where("task_id = ? AND user_id = ?", task.ID, uid).
				FirstOrCreate(&reviewer).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to add reviewers")
		return
	}
	httpx.OK(c, gin.H{"added": len(wantIDs)})
}

// RemoveReviewer 解除某用户对任务的审核指派。
func (h TaskHandler) RemoveReviewer(c *gin.Context) {
	task, ok := loadOwnedTask(h.db, c)
	if !ok {
		return
	}
	userID, ok := parseIDParam(c, "userId")
	if !ok {
		return
	}
	res := h.db.Where("task_id = ? AND user_id = ?", task.ID, userID).
		Delete(&model.TaskReviewer{})
	if res.Error != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to remove reviewer")
		return
	}
	if res.RowsAffected == 0 {
		httpx.Error(c, http.StatusNotFound, "NOT_FOUND", "reviewer not found")
		return
	}
	httpx.OK(c, gin.H{"removed": userID})
}

// reviewerCandidateUsers 查 active 且具备 reviewer 角色的用户。
func reviewerCandidateUsers(db *gorm.DB) ([]model.User, error) {
	var users []model.User
	err := db.
		Joins("JOIN user_roles ON user_roles.user_id = users.id AND user_roles.role = ?", policy.RoleReviewer).
		Where("users.status = ?", "active").
		Order("users.id ASC").
		Find(&users).Error
	return users, err
}

// uniqueUint64 去重并保持原顺序。
func uniqueUint64(ids []uint64) []uint64 {
	seen := make(map[uint64]struct{}, len(ids))
	out := make([]uint64, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
