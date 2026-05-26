package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"labelhub-api/internal/httpx"
	"labelhub-api/internal/middleware"
	"labelhub-api/internal/model"
	"labelhub-api/internal/policy"
)

// TaskHandler 封装 Owner 视角的 /tasks 端点。
type TaskHandler struct {
	db *gorm.DB
}

func NewTaskHandler(db *gorm.DB) TaskHandler {
	return TaskHandler{db: db}
}

func (h TaskHandler) Register(api gin.IRouter) {
	api.GET("/tasks", middleware.RequireRoles("owner", "admin"), h.ListTasks)
	api.POST("/tasks", middleware.RequireRoles("owner", "admin"), h.CreateTask)
	api.GET("/tasks/:taskId", middleware.RequireRoles("owner", "admin", "labeler", "reviewer"), h.GetTask)
	api.GET("/tasks/:taskId/item-preview", middleware.RequireRoles("owner", "admin"), h.PreviewItem)
	api.POST("/tasks/:taskId/items/import", middleware.RequireRoles("owner", "admin"), h.ImportItems)
}

type createTaskRequest struct {
	Title               string `json:"title" binding:"required"`
	Description         string `json:"description"`
	BaselineDescription string `json:"baselineDescription"`
}

type importItemsRequest struct {
	Items []map[string]any `json:"items" binding:"required"`
}

type previewItemResponse struct {
	ID         uint64          `json:"id"`
	ExternalID *string         `json:"externalId"`
	Payload    json.RawMessage `json:"payload"`
}

func (h TaskHandler) ListTasks(c *gin.Context) {
	claims, _ := middleware.Claims(c)
	query := h.db.Order("id DESC").Limit(httpx.CursorLimit(c))
	if !policy.HasRole(claims, policy.RoleAdmin) {
		query = query.Where("owner_id = ?", claims.UserID)
	}
	var tasks []model.Task
	if err := query.Find(&tasks).Error; err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list tasks")
		return
	}
	httpx.PageOK(c, tasks, httpx.Page{})
}

func (h TaskHandler) CreateTask(c *gin.Context) {
	var req createTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "title is required")
		return
	}
	claims, _ := middleware.Claims(c)
	task := model.Task{
		OwnerID:             claims.UserID,
		Title:               req.Title,
		Status:              "draft",
		Description:         model.StringFrom(req.Description),
		BaselineDescription: model.StringFrom(req.BaselineDescription),
		Distribution:        "first_come",
		AIReviewEnabled:     false,
		HumanReviewEnabled:  true,
	}
	if err := h.db.Create(&task).Error; err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to create task")
		return
	}
	httpx.OK(c, task)
}

func (h TaskHandler) GetTask(c *gin.Context) {
	task, ok := loadTask(h.db, c)
	if !ok {
		return
	}
	if !enforceCanReadTask(c, task) {
		return
	}
	template, _ := currentTemplate(h.db, task.ID)
	httpx.OK(c, gin.H{
		"task":     task,
		"template": template,
	})
}

func (h TaskHandler) PreviewItem(c *gin.Context) {
	task, ok := loadOwnedTask(h.db, c)
	if !ok {
		return
	}
	var item model.TaskItem
	if err := h.db.Where("task_id = ? AND status = ?", task.ID, itemStatusAvailable).Order("id ASC").First(&item).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			httpx.OK(c, gin.H{"item": nil})
			return
		}
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load preview item")
		return
	}
	var externalID *string
	if item.ExternalID.Valid {
		externalID = &item.ExternalID.String
	}
	httpx.OK(c, gin.H{"item": previewItemResponse{
		ID:         item.ID,
		ExternalID: externalID,
		Payload:    json.RawMessage(item.Payload),
	}})
}

func (h TaskHandler) ImportItems(c *gin.Context) {
	task, ok := loadOwnedTask(h.db, c)
	if !ok {
		return
	}
	var req importItemsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "items are required")
		return
	}

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		for _, payload := range req.Items {
			externalID, _ := payload["id"].(string)
			raw, err := json.Marshal(payload)
			if err != nil {
				return err
			}
			// 缺 id 时用 payload 哈希兜底,避免每次重导致建出重复 task_items。
			// 前缀 "hash:" 让调用方一眼能区分"是我给的 ID"还是"系统兜底生成"。
			if externalID == "" {
				externalID = payloadHashExternalID(raw)
			}
			item := model.TaskItem{
				TaskID:     task.ID,
				ExternalID: model.StringFrom(externalID),
				Payload:    string(raw),
				Status:     itemStatusAvailable,
			}
			if err := tx.Where("task_id = ? AND external_id = ?", task.ID, externalID).FirstOrCreate(&item).Error; err != nil {
				return err
			}
		}
		return tx.Model(&model.Task{}).Where("id = ?", task.ID).Update("total_items", gorm.Expr("(SELECT COUNT(*) FROM task_items WHERE task_id = ?)", task.ID)).Error
	}); err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to import items")
		return
	}
	httpx.OK(c, gin.H{"imported": len(req.Items)})
}

// payloadHashExternalID 用 payload JSON 的 sha256 前 16 字节(32 hex)生成内部 external_id,
// 确保同一 payload 重复导入命中 unique key (task_id, external_id) 而不是建出重复行。
func payloadHashExternalID(raw []byte) string {
	sum := sha256.Sum256(raw)
	return "hash:" + hex.EncodeToString(sum[:16])
}
