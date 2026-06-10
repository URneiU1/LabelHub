package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"labelhub-api/internal/httpx"
	"labelhub-api/internal/model"
)

// --- 批量编辑 task_items 的 payload ---

type batchUpdateItem struct {
	ItemID  uint64          `json:"itemId" binding:"required"`
	Payload json.RawMessage `json:"payload" binding:"required"`
}

type batchUpdateItemsRequest struct {
	Items []batchUpdateItem `json:"items" binding:"required"`
}

// BatchUpdateItems 批量覆盖本任务下指定题目的 payload。只更新属于该任务的题目。
func (h TaskHandler) BatchUpdateItems(c *gin.Context) {
	task, ok := loadOwnedTask(h.db, c)
	if !ok {
		return
	}
	if !ensureTaskDraft(c, task) {
		return
	}
	var req batchUpdateItemsRequest
	if !bindLimitedJSON(c, &req, maxImportItemsBytes) {
		return
	}
	if len(req.Items) == 0 {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "items must not be empty")
		return
	}
	if len(req.Items) > maxImportItems {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "too many items in a single batch")
		return
	}
	for _, item := range req.Items {
		if !json.Valid(item.Payload) {
			httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "payload must be valid JSON")
			return
		}
	}

	var updated int64
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		for _, item := range req.Items {
			res := tx.Model(&model.TaskItem{}).
				Where("id = ? AND task_id = ?", item.ItemID, task.ID).
				Update("payload", string(item.Payload))
			if res.Error != nil {
				return res.Error
			}
			updated += res.RowsAffected
		}
		return nil
	}); err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to batch update items")
		return
	}
	httpx.OK(c, gin.H{"updated": updated, "requested": len(req.Items)})
}

// --- 任务指派(task_assignees)管理 ---

type addAssigneesRequest struct {
	UserIDs []uint64 `json:"userIds" binding:"required"`
}

type assigneeView struct {
	UserID     uint64         `json:"userId"`
	AssignedAt model.NullTime `json:"assignedAt"`
}

// ListAssignees 返回该任务的全部指派用户(task 级,item_id 为 NULL)。
func (h TaskHandler) ListAssignees(c *gin.Context) {
	task, ok := loadOwnedTask(h.db, c)
	if !ok {
		return
	}
	var assignees []model.TaskAssignee
	if err := h.db.Where("task_id = ? AND item_id IS NULL", task.ID).Order("user_id ASC").Find(&assignees).Error; err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list assignees")
		return
	}
	out := make([]assigneeView, 0, len(assignees))
	for _, a := range assignees {
		out = append(out, assigneeView{UserID: a.UserID, AssignedAt: a.AssignedAt})
	}
	httpx.OK(c, gin.H{"assignees": out})
}

// AddAssignees 把一组用户指派到任务(task 级)。重复指派幂等(FirstOrCreate)。
func (h TaskHandler) AddAssignees(c *gin.Context) {
	task, ok := loadOwnedTask(h.db, c)
	if !ok {
		return
	}
	var req addAssigneesRequest
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

	now := model.TimeFrom(taskNow())
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		for _, uid := range req.UserIDs {
			assignee := model.TaskAssignee{
				TaskID:     task.ID,
				UserID:     uid,
				AssignedAt: now,
			}
			if err := tx.Where("task_id = ? AND user_id = ? AND item_id IS NULL", task.ID, uid).
				FirstOrCreate(&assignee).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to add assignees")
		return
	}
	httpx.OK(c, gin.H{"added": len(req.UserIDs)})
}

// RemoveAssignee 取消某用户对任务的指派(task 级)。
func (h TaskHandler) RemoveAssignee(c *gin.Context) {
	task, ok := loadOwnedTask(h.db, c)
	if !ok {
		return
	}
	userID, ok := parseIDParam(c, "userId")
	if !ok {
		return
	}
	res := h.db.Where("task_id = ? AND user_id = ? AND item_id IS NULL", task.ID, userID).
		Delete(&model.TaskAssignee{})
	if res.Error != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to remove assignee")
		return
	}
	if res.RowsAffected == 0 {
		httpx.Error(c, http.StatusNotFound, "NOT_FOUND", "assignee not found")
		return
	}
	httpx.OK(c, gin.H{"removed": userID})
}

type labelerCandidateView struct {
	UserID      uint64 `json:"userId"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
}

// ListLabelerCandidates 返回可被指派到任务的标注员(role=labeler 且 active),供 owner
// 在「指派」分发策略下从列表里选人,而不是手输用户 ID。只回最小字段。
func (h TaskHandler) ListLabelerCandidates(c *gin.Context) {
	if _, ok := loadOwnedTask(h.db, c); !ok {
		return
	}
	var users []model.User
	if err := h.db.
		Joins("JOIN user_roles ON user_roles.user_id = users.id AND user_roles.role = ?", "labeler").
		Where("users.status = ?", "active").
		Order("users.id ASC").
		Find(&users).Error; err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list labeler candidates")
		return
	}
	out := make([]labelerCandidateView, 0, len(users))
	for _, u := range users {
		out = append(out, labelerCandidateView{UserID: u.ID, Username: u.Username, DisplayName: u.DisplayName})
	}
	httpx.OK(c, gin.H{"candidates": out})
}

// ownerReviewResultItem:owner「审核结果」逐条质检反馈的单行。AI 预审判定 vs 人工判定
// 是否一致,供 owner 回看预审标准准不准、要不要调(只读,不做审核动作)。
type ownerReviewResultItem struct {
	ID           uint64    `json:"id"`
	ItemID       uint64    `json:"itemId"`
	Status       string    `json:"status"`
	AIVerdict    *string   `json:"aiVerdict"`
	AIScore      *float64  `json:"aiScore"`
	HumanVerdict *string   `json:"humanVerdict"`
	Agreed       *bool     `json:"agreed"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// aiHumanAgreed:AI 预审判定与人工终判是否一致。任一缺失返回 nil(无法比较)。
// pass↔approve、reject↔reject 视为一致;uncertain 或交叉视为不一致(值得 owner 回看)。
func aiHumanAgreed(aiVerdict, humanVerdict *string) *bool {
	if aiVerdict == nil || humanVerdict == nil {
		return nil
	}
	agreed := (*aiVerdict == "pass" && *humanVerdict == "approve") ||
		(*aiVerdict == "reject" && *humanVerdict == "reject")
	return &agreed
}

// ListReviewResults 列出该任务已定稿(approved/rejected)的提交,带 AI 判定 / 人工判定 /
// 是否一致,供 owner 在「审核结果」做只读质检回看。人工审核「动作」仍归 Reviewer。
func (h TaskHandler) ListReviewResults(c *gin.Context) {
	task, ok := loadOwnedTask(h.db, c)
	if !ok {
		return
	}
	limit := httpx.CursorLimit(c)
	query := h.db.Model(&model.Submission{}).
		Where("task_id = ? AND status IN ?", task.ID, finalizedReviewStatuses)
	if cursor := strings.TrimSpace(c.Query("cursor")); cursor != "" {
		cursorID, err := strconv.ParseUint(cursor, 10, 64)
		if err != nil {
			httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "cursor must be a positive integer")
			return
		}
		query = query.Where("id < ?", cursorID)
	}

	var submissions []model.Submission
	if err := query.Order("updated_at DESC, id DESC").Limit(limit + 1).Find(&submissions).Error; err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list review results")
		return
	}
	page := httpx.Page{}
	if len(submissions) > limit {
		submissions = submissions[:limit]
		page.HasMore = true
		page.NextCursor = strconv.FormatUint(submissions[len(submissions)-1].ID, 10)
	}
	items := make([]ownerReviewResultItem, 0, len(submissions))
	for _, s := range submissions {
		items = append(items, ownerReviewResultItem{
			ID:           s.ID,
			ItemID:       s.ItemID,
			Status:       s.Status,
			AIVerdict:    s.AIVerdict,
			AIScore:      s.AIScore,
			HumanVerdict: s.HumanVerdict,
			Agreed:       aiHumanAgreed(s.AIVerdict, s.HumanVerdict),
			UpdatedAt:    s.UpdatedAt,
		})
	}
	httpx.PageOK(c, items, page)
}
