package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"labelhub-api/internal/httpx"
	"labelhub-api/internal/middleware"
	"labelhub-api/internal/model"
	"labelhub-api/internal/policy"
	"labelhub-api/internal/service/audit"
	"labelhub-api/internal/statemachine"
)

// distribution 列的合法取值。default first_come。
var validDistributions = map[string]struct{}{
	"first_come": {},
	"assigned":   {},
	"quota":      {},
}

// errTaskStateRace:乐观更新影响 0 行,说明任务状态被并发改动。
var errTaskStateRace = errors.New("task status changed concurrently")

// taskNow 包级时间源,便于单测注入固定时间(发布时间戳)。
var taskNow = func() time.Time { return time.Now().UTC() }

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
	api.PUT("/tasks/:taskId", middleware.RequireRoles("owner", "admin"), h.UpdateTask)
	api.POST("/tasks/:taskId/baseline", middleware.RequireRoles("owner", "admin"), h.UpdateBaseline)
	api.GET("/tasks/:taskId/item-preview", middleware.RequireRoles("owner", "admin"), h.PreviewItem)
	api.GET("/tasks/:taskId/items", middleware.RequireRoles("owner", "admin"), h.ListItems)
	api.GET("/tasks/:taskId/review-results", middleware.RequireRoles("owner", "admin"), h.ListReviewResults)
	api.POST("/tasks/:taskId/items/import", middleware.RequireRoles("owner", "admin"), h.ImportItems)
	api.POST("/tasks/:taskId/items/import-file", middleware.RequireRoles("owner", "admin"), h.ImportItemsFile)
	api.POST("/tasks/:taskId/items/batch-update", middleware.RequireRoles("owner", "admin"), h.BatchUpdateItems)
	api.POST("/tasks/:taskId/publish", middleware.RequireRoles("owner", "admin"), h.PublishTask)
	api.POST("/tasks/:taskId/pause", middleware.RequireRoles("owner", "admin"), h.PauseTask)
	api.POST("/tasks/:taskId/resume", middleware.RequireRoles("owner", "admin"), h.ResumeTask)
	api.POST("/tasks/:taskId/end", middleware.RequireRoles("owner", "admin"), h.EndTask)
	api.GET("/tasks/:taskId/assignee-candidates", middleware.RequireRoles("owner", "admin"), h.ListLabelerCandidates)
	api.GET("/tasks/:taskId/assignees", middleware.RequireRoles("owner", "admin"), h.ListAssignees)
	api.POST("/tasks/:taskId/assignees", middleware.RequireRoles("owner", "admin"), h.AddAssignees)
	api.DELETE("/tasks/:taskId/assignees/:userId", middleware.RequireRoles("owner", "admin"), h.RemoveAssignee)
}

type createTaskRequest struct {
	Title               string           `json:"title" binding:"required"`
	Description         *string          `json:"description"`
	RichDescription     *json.RawMessage `json:"richDescription"`
	Tags                *json.RawMessage `json:"tags"`
	RewardConfig        *json.RawMessage `json:"rewardConfig"`
	BaselineDescription string           `json:"baselineDescription"`
	Distribution        *string          `json:"distribution"`
	QuotaPerUser        *int             `json:"quotaPerUser"`
	Deadline            *model.NullTime  `json:"deadline"`
}

// updateTaskRequest:基本信息编辑。所有字段可选,只更新出现的字段(指针/RawMessage 区分"未传"与"传 null")。
type updateTaskRequest struct {
	Title           *string          `json:"title"`
	Description     *string          `json:"description"`
	RichDescription *json.RawMessage `json:"richDescription"`
	Tags            *json.RawMessage `json:"tags"`
	RewardConfig    *json.RawMessage `json:"rewardConfig"`
	Distribution    *string          `json:"distribution"`
	QuotaPerUser    *int             `json:"quotaPerUser"`
	Deadline        *model.NullTime  `json:"deadline"`
}

type importItemsRequest struct {
	Items []map[string]any `json:"items" binding:"required"`
}

type updateBaselineRequest struct {
	BaselineDescription *string `json:"baselineDescription"`
}

type previewItemResponse struct {
	ID         uint64          `json:"id"`
	ExternalID *string         `json:"externalId"`
	Payload    json.RawMessage `json:"payload"`
}

// taskItemRow 是 Owner 批量编辑列表里的一条题目:含状态/优先级,供前端展示与编辑定位。
type taskItemRow struct {
	ID         uint64          `json:"id"`
	ExternalID *string         `json:"externalId"`
	Status     string          `json:"status"`
	Priority   int             `json:"priority"`
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
	if !bindLimitedJSON(c, &req, maxTaskInfoBytes) {
		return
	}
	if req.Title == "" {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "title is required")
		return
	}

	distribution := "first_come"
	if req.Distribution != nil {
		if _, ok := validDistributions[*req.Distribution]; !ok {
			httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "distribution must be one of first_come, assigned, quota")
			return
		}
		distribution = *req.Distribution
	}

	tags, ok := validateOptionalJSON(c, req.Tags, "tags")
	if !ok {
		return
	}
	rewardConfig, ok := validateOptionalJSON(c, req.RewardConfig, "rewardConfig")
	if !ok {
		return
	}
	richDescription, ok := validateOptionalJSON(c, req.RichDescription, "richDescription")
	if !ok {
		return
	}

	claims, _ := middleware.Claims(c)
	task := model.Task{
		OwnerID:             claims.UserID,
		Title:               req.Title,
		Status:              statemachine.TaskDraft,
		BaselineDescription: model.StringFrom(req.BaselineDescription),
		Distribution:        distribution,
		RichDescription:     richDescription,
		Tags:                tags,
		RewardConfig:        rewardConfig,
		AIReviewEnabled:     false,
		HumanReviewEnabled:  true,
	}
	if req.Description != nil {
		task.Description = model.StringFrom(*req.Description)
	}
	if req.QuotaPerUser != nil {
		if *req.QuotaPerUser < 0 {
			httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "quotaPerUser must be >= 0")
			return
		}
		task.QuotaPerUser = *req.QuotaPerUser
	}
	if req.Deadline != nil {
		task.Deadline = *req.Deadline
	}
	if err := h.db.Create(&task).Error; err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to create task")
		return
	}
	httpx.OK(c, task)
}

// UpdateTask 编辑任务基本信息;只更新请求里出现的字段。
func (h TaskHandler) UpdateTask(c *gin.Context) {
	task, ok := loadOwnedTask(h.db, c)
	if !ok {
		return
	}
	var req updateTaskRequest
	if !bindLimitedJSON(c, &req, maxTaskInfoBytes) {
		return
	}

	updates := map[string]any{}
	if req.Title != nil {
		if *req.Title == "" {
			httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "title must not be empty")
			return
		}
		updates["title"] = *req.Title
	}
	if req.Description != nil {
		updates["description"] = model.StringFrom(*req.Description)
	}
	if req.Distribution != nil {
		if _, ok := validDistributions[*req.Distribution]; !ok {
			httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "distribution must be one of first_come, assigned, quota")
			return
		}
		updates["distribution"] = *req.Distribution
	}
	if req.QuotaPerUser != nil {
		if *req.QuotaPerUser < 0 {
			httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "quotaPerUser must be >= 0")
			return
		}
		updates["quota_per_user"] = *req.QuotaPerUser
	}
	if req.Deadline != nil {
		updates["deadline"] = *req.Deadline
	}
	if req.Tags != nil {
		tags, valid := validateOptionalJSON(c, req.Tags, "tags")
		if !valid {
			return
		}
		updates["tags"] = tags
	}
	if req.RewardConfig != nil {
		rewardConfig, valid := validateOptionalJSON(c, req.RewardConfig, "rewardConfig")
		if !valid {
			return
		}
		updates["reward_config"] = rewardConfig
	}
	if req.RichDescription != nil {
		richDescription, valid := validateOptionalJSON(c, req.RichDescription, "richDescription")
		if !valid {
			return
		}
		updates["rich_description"] = richDescription
	}

	if len(updates) == 0 {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "no fields to update")
		return
	}

	if err := h.db.Model(&model.Task{}).Where("id = ?", task.ID).Updates(updates).Error; err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to update task")
		return
	}
	if err := h.db.First(&task, task.ID).Error; err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to reload task")
		return
	}
	httpx.OK(c, gin.H{"task": task})
}

// validateOptionalJSON:把可选的原始 JSON 字段转成 *string(json 列存储)。
// nil → 留空(不更新/不设置);JSON null → nil(显式清空);非法 JSON → 400。
func validateOptionalJSON(c *gin.Context, raw *json.RawMessage, field string) (*string, bool) {
	if raw == nil {
		return nil, true
	}
	if !json.Valid(*raw) {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", field+" must be valid JSON")
		return nil, false
	}
	str := string(*raw)
	if str == "null" {
		return nil, true
	}
	return &str, true
}

// PublishTask 草稿 → 发布中。发布要求已绑定模板。
func (h TaskHandler) PublishTask(c *gin.Context) {
	h.transitionTask(c, statemachine.TaskEventPublish, true)
}

// PauseTask 发布中 → 已暂停。
func (h TaskHandler) PauseTask(c *gin.Context) {
	h.transitionTask(c, statemachine.TaskEventPause, false)
}

// ResumeTask 已暂停 → 发布中。
func (h TaskHandler) ResumeTask(c *gin.Context) {
	h.transitionTask(c, statemachine.TaskEventResume, false)
}

// EndTask 发布中/已暂停 → 已结束。
func (h TaskHandler) EndTask(c *gin.Context) {
	h.transitionTask(c, statemachine.TaskEventEnd, false)
}

// transitionTask 统一处理任务生命周期迁移:状态机校验 + (发布时)模板校验 + 同事务写状态/PublishedAt/audit。
func (h TaskHandler) transitionTask(c *gin.Context, event string, requireTemplate bool) {
	task, ok := loadOwnedTask(h.db, c)
	if !ok {
		return
	}
	to, ok := statemachine.TaskTargetFor(task.Status, event)
	if !ok {
		httpx.Error(c, http.StatusUnprocessableEntity, "INVALID_STATE",
			"task cannot "+event+" from status "+task.Status)
		return
	}
	if requireTemplate && task.TemplateID == nil {
		httpx.Error(c, http.StatusUnprocessableEntity, "INVALID_STATE",
			"task must have a bound template before publishing")
		return
	}

	claims, _ := middleware.Claims(c)
	now := taskNow()
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		updates := map[string]any{"status": to}
		if event == statemachine.TaskEventPublish {
			updates["published_at"] = now
		}
		res := tx.Model(&model.Task{}).
			Where("id = ? AND status = ?", task.ID, task.Status).
			Updates(updates)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return errTaskStateRace
		}
		return audit.Write(tx, audit.LogEntry{
			EntityType: "task",
			EntityID:   task.ID,
			FromState:  task.Status,
			ToState:    to,
			ActorType:  "user",
			ActorID:    &claims.UserID,
			Event:      event,
		})
	}); err != nil {
		if errors.Is(err, errTaskStateRace) {
			httpx.Error(c, http.StatusConflict, "CONFLICT", "task status changed concurrently, please retry")
			return
		}
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to update task status")
		return
	}
	if err := h.db.First(&task, task.ID).Error; err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to reload task")
		return
	}
	httpx.OK(c, gin.H{"task": task})
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

func (h TaskHandler) UpdateBaseline(c *gin.Context) {
	task, ok := loadOwnedTask(h.db, c)
	if !ok {
		return
	}
	var req updateBaselineRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.BaselineDescription == nil {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "baselineDescription is required")
		return
	}
	if err := h.db.Model(&model.Task{}).Where("id = ?", task.ID).Update("baseline_description", *req.BaselineDescription).Error; err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to update baseline")
		return
	}
	if err := h.db.First(&task, task.ID).Error; err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to reload task")
		return
	}
	httpx.OK(c, gin.H{"task": task})
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

// ListItems 分页列出某任务的全部题目(payload + 状态),供 Owner 批量编辑加载完整列表。
// 按 id 升序游标分页:?limit= 控制每页(默认 20,最多 100),?cursor= 为上一页最后一条的 id。
func (h TaskHandler) ListItems(c *gin.Context) {
	task, ok := loadOwnedTask(h.db, c)
	if !ok {
		return
	}
	limit := httpx.CursorLimit(c)
	query := h.db.Where("task_id = ?", task.ID)
	if cursor := c.Query("cursor"); cursor != "" {
		afterID, err := strconv.ParseUint(cursor, 10, 64)
		if err != nil {
			httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "cursor must be a positive integer")
			return
		}
		query = query.Where("id > ?", afterID)
	}
	var items []model.TaskItem
	// 多取一条用于判断是否还有下一页,返回前裁掉。
	if err := query.Order("id ASC").Limit(limit + 1).Find(&items).Error; err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list items")
		return
	}
	page := httpx.Page{}
	if len(items) > limit {
		items = items[:limit]
		page.HasMore = true
		page.NextCursor = strconv.FormatUint(items[len(items)-1].ID, 10)
	}
	rows := make([]taskItemRow, 0, len(items))
	for _, it := range items {
		row := taskItemRow{
			ID:       it.ID,
			Status:   it.Status,
			Priority: it.Priority,
			Payload:  json.RawMessage(it.Payload),
		}
		if it.ExternalID.Valid {
			ext := it.ExternalID.String
			row.ExternalID = &ext
		}
		rows = append(rows, row)
	}
	httpx.PageOK(c, rows, page)
}

func (h TaskHandler) ImportItems(c *gin.Context) {
	task, ok := loadOwnedTask(h.db, c)
	if !ok {
		return
	}
	var req importItemsRequest
	// 批量导入限制请求体大小,避免无界 JSON 撑爆内存;再卡单批条数上限。
	if !bindLimitedJSON(c, &req, maxImportItemsBytes) {
		return
	}
	if len(req.Items) > maxImportItems {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "too many items in a single import")
		return
	}

	if err := insertImportedItems(h.db, task.ID, req.Items); err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to import items")
		return
	}
	httpx.OK(c, gin.H{"imported": len(req.Items)})
}

// insertImportedItems:在单事务里对每条 payload 做 FirstOrCreate 去重并重算 total_items。
// JSON / JSONL / Excel 三种导入都收敛到这里,确保 external_id 去重逻辑一致。
func insertImportedItems(db *gorm.DB, taskID uint64, items []map[string]any) error {
	return db.Transaction(func(tx *gorm.DB) error {
		for _, payload := range items {
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
				TaskID:     taskID,
				ExternalID: model.StringFrom(externalID),
				Payload:    string(raw),
				Status:     itemStatusAvailable,
			}
			if err := tx.Where("task_id = ? AND external_id = ?", taskID, externalID).FirstOrCreate(&item).Error; err != nil {
				return err
			}
		}
		return tx.Model(&model.Task{}).Where("id = ?", taskID).Update("total_items", gorm.Expr("(SELECT COUNT(*) FROM task_items WHERE task_id = ?)", taskID)).Error
	})
}

// payloadHashExternalID 用 payload JSON 的 sha256 前 16 字节(32 hex)生成内部 external_id,
// 确保同一 payload 重复导入命中 unique key (task_id, external_id) 而不是建出重复行。
func payloadHashExternalID(raw []byte) string {
	sum := sha256.Sum256(raw)
	return "hash:" + hex.EncodeToString(sum[:16])
}
