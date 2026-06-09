package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"labelhub-api/internal/httpx"
	"labelhub-api/internal/middleware"
	"labelhub-api/internal/model"
	"labelhub-api/internal/policy"
	"labelhub-api/internal/service/submission"
	"labelhub-api/internal/statemachine"
)

// LabelerHandler 封装 labeler 角色端点:任务广场、领单、作答(draft / submit)、我的提交。
type LabelerHandler struct {
	db *gorm.DB
}

func NewLabelerHandler(db *gorm.DB) LabelerHandler {
	return LabelerHandler{db: db}
}

func (h LabelerHandler) Register(api gin.IRouter) {
	api.GET("/labeler/tasks", middleware.RequireRoles("labeler"), h.ListPublishedTasks)
	api.POST("/tasks/:taskId/claim", middleware.RequireRoles("labeler"), h.ClaimItem)
	api.POST("/tasks/:taskId/claim-task", middleware.RequireRoles("labeler"), h.ClaimTask)
	api.GET("/tasks/:taskId/items/:itemId", middleware.RequireRoles("labeler", "reviewer", "admin"), h.GetItem)
	api.POST("/tasks/:taskId/items/:itemId/draft", middleware.RequireRoles("labeler"), h.SaveDraft)
	api.POST("/tasks/:taskId/items/:itemId/submit", middleware.RequireRoles("labeler"), h.SubmitItem)
	api.GET("/me/submissions", middleware.RequireRoles("labeler"), h.MySubmissions)
	api.GET("/me/tasks", middleware.RequireRoles("labeler"), h.MyTasks)
	api.GET("/tasks/:taskId/labeler/items", middleware.RequireRoles("labeler"), h.ListMyTaskItems)
}

// labelerTaskItem 是作答页"题目导航"里的一题:其在本任务中的序号、外部 ID,以及当前 labeler
// 对这一题的状态(available 待标 / claimed 进行中 / 或其 submission 状态;taken = 被他人领走)。
type labelerTaskItem struct {
	ItemID       uint64  `json:"itemId"`
	ExternalID   *string `json:"externalId"`
	Status       string  `json:"status"`
	Mine         bool    `json:"mine"`
	SubmissionID *uint64 `json:"submissionId"`
}

type labelerTaskItemsResponse struct {
	TaskID uint64            `json:"taskId"`
	Total  int               `json:"total"`
	Items  []labelerTaskItem `json:"items"`
	Counts map[string]int    `json:"counts"`
}

// ListMyTaskItems 返回某任务下全部题目及当前 labeler 对每题的状态,供作答页左侧"题目导航"
// 展示完整进度(已提交/草稿/打回/进行中/待标)与跳题。两次查询 + 内存合并,避免复杂联表。
func (h LabelerHandler) ListMyTaskItems(c *gin.Context) {
	task, ok := loadTask(h.db, c)
	if !ok {
		return
	}
	claims, _ := middleware.Claims(c)

	var items []model.TaskItem
	if err := h.db.Where("task_id = ?", task.ID).Order("priority DESC, id ASC").Find(&items).Error; err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list items")
		return
	}
	var subs []model.Submission
	if err := h.db.Where("task_id = ?", task.ID).Find(&subs).Error; err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list submissions")
		return
	}
	mineByItem := make(map[uint64]model.Submission, len(subs))
	for _, s := range subs {
		if s.LabelerID == claims.UserID {
			mineByItem[s.ItemID] = s
		}
	}

	out := make([]labelerTaskItem, 0, len(items))
	counts := map[string]int{}
	for _, it := range items {
		entry := labelerTaskItem{ItemID: it.ID}
		if it.ExternalID.Valid {
			ext := it.ExternalID.String
			entry.ExternalID = &ext
		}
		switch sub, has := mineByItem[it.ID]; {
		case has:
			entry.Status = sub.Status
			entry.Mine = true
			sid := sub.ID
			entry.SubmissionID = &sid
		case it.ClaimedBy != nil && *it.ClaimedBy == claims.UserID:
			entry.Status = "claimed"
			entry.Mine = true
		case it.Status != submission.ItemStatusAvailable:
			entry.Status = "taken"
		default:
			entry.Status = "available"
		}
		counts[entry.Status]++
		out = append(out, entry)
	}

	httpx.OK(c, labelerTaskItemsResponse{
		TaskID: task.ID,
		Total:  len(items),
		Items:  out,
		Counts: counts,
	})
}

type answerRequest struct {
	Answer map[string]any `json:"answer" binding:"required"`
}

func (h LabelerHandler) ListPublishedTasks(c *gin.Context) {
	var tasks []model.Task
	if err := h.db.Where("status = ?", "published").Order("id DESC").Find(&tasks).Error; err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list tasks")
		return
	}
	httpx.PageOK(c, tasks, httpx.Page{})
}

func (h LabelerHandler) ClaimItem(c *gin.Context) {
	taskID, ok := parseIDParam(c, "taskId")
	if !ok {
		return
	}
	claims, _ := middleware.Claims(c)

	result, err := submission.Claim(h.db, submission.ClaimInput{
		TaskID:    taskID,
		LabelerID: claims.UserID,
	})
	switch {
	case err == nil:
		respondItem(h.db, c, result.Task, result.Item)
	case errors.Is(err, submission.ErrTaskNotFound):
		httpx.Error(c, http.StatusNotFound, "NOT_FOUND", "task not found")
	case errors.Is(err, submission.ErrTaskNotPublished):
		httpx.Error(c, http.StatusConflict, "CONFLICT", "task is not accepting new claims")
	case errors.Is(err, submission.ErrNoAvailableItem):
		httpx.Error(c, http.StatusConflict, "CONFLICT", "没有可领取的题目")
	case errors.Is(err, submission.ErrQuotaReached):
		httpx.Error(c, http.StatusConflict, "CONFLICT", "已达到本任务的领取配额")
	case errors.Is(err, submission.ErrDailySubmissionLimitReached):
		httpx.Error(c, http.StatusConflict, "CONFLICT", "已达到今日提交上限")
	case errors.Is(err, submission.ErrNotAssigned):
		httpx.Error(c, http.StatusForbidden, "FORBIDDEN", "你未被指派到该任务")
	case errors.Is(err, submission.ErrClaimRaceLost):
		httpx.Error(c, http.StatusConflict, "CONFLICT", "claim race lost, please retry")
	case errors.Is(err, submission.ErrTaskTemplate):
		httpx.Error(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "task template is not available; please contact the owner")
	default:
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to claim item")
	}
}

// ClaimTask 整体领取一个大任务(first_come / assigned 独占):该任务所有题一次性锁给当前 labeler。
func (h LabelerHandler) ClaimTask(c *gin.Context) {
	taskID, ok := parseIDParam(c, "taskId")
	if !ok {
		return
	}
	claims, _ := middleware.Claims(c)

	result, err := submission.ClaimTask(h.db, submission.ClaimInput{
		TaskID:    taskID,
		LabelerID: claims.UserID,
	})
	switch {
	case err == nil:
		httpx.OK(c, gin.H{"task": result.Task})
	case errors.Is(err, submission.ErrTaskNotFound):
		httpx.Error(c, http.StatusNotFound, "NOT_FOUND", "task not found")
	case errors.Is(err, submission.ErrTaskNotPublished):
		httpx.Error(c, http.StatusConflict, "CONFLICT", "task is not accepting new claims")
	case errors.Is(err, submission.ErrWrongClaimMode):
		httpx.Error(c, http.StatusConflict, "CONFLICT", "该任务按题领取，请逐题领取")
	case errors.Is(err, submission.ErrTaskTaken):
		httpx.Error(c, http.StatusConflict, "CONFLICT", "任务已被他人领取")
	case errors.Is(err, submission.ErrNotAssigned):
		httpx.Error(c, http.StatusForbidden, "FORBIDDEN", "你未被指派到该任务")
	default:
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to claim task")
	}
}

func (h LabelerHandler) GetItem(c *gin.Context) {
	task, ok := loadTask(h.db, c)
	if !ok {
		return
	}
	item, ok := loadItem(h.db, c, task.ID)
	if !ok {
		return
	}

	// 查 submission(可能不存在);policy.CanReadItem 需要它来判定 reviewer 是否有权限看。
	var submissionPtr *model.Submission
	var submission model.Submission
	if err := submissionQueryForItem(h.db, c, item.ID).First(&submission).Error; err == nil {
		submissionPtr = &submission
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load submission")
		return
	}

	claims, _ := middleware.Claims(c)
	if policy.HasRole(claims, policy.RoleReviewer) && !policy.HasRole(claims, policy.RoleAdmin) && !policy.IsTaskOwner(claims, task) {
		allowed, err := canReviewTask(h.db, claims, task)
		if err != nil {
			httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to check review access")
			return
		}
		if !allowed {
			httpx.Error(c, http.StatusForbidden, "FORBIDDEN", "reviewer is not assigned to this task")
			return
		}
	}
	if !policy.CanReadItem(claims, task, item, submissionPtr) {
		httpx.Error(c, http.StatusForbidden, "FORBIDDEN", "item is not visible to current user")
		return
	}
	respondItem(h.db, c, task, item)
}

func (h LabelerHandler) SaveDraft(c *gin.Context) {
	h.saveRevision(c, true)
}

func (h LabelerHandler) SubmitItem(c *gin.Context) {
	h.saveRevision(c, false)
}

func (h LabelerHandler) MySubmissions(c *gin.Context) {
	claims, _ := middleware.Claims(c)
	var submissions []model.Submission
	if err := h.db.Where("labeler_id = ?", claims.UserID).Order("updated_at DESC").Find(&submissions).Error; err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list submissions")
		return
	}
	httpx.PageOK(c, submissions, httpx.Page{})
}

// myTaskView 是「已领取的任务」里的一项:大任务本身 + 当前 labeler 在该任务里的进度
// (各状态计数、总数、进行中数量)与可恢复目标(最近一条 draft/revising 的 itemId)。
type myTaskView struct {
	Task         model.Task     `json:"task"`
	MyCounts     map[string]int `json:"myCounts"`
	MyTotal      int            `json:"myTotal"`
	MyInProgress int            `json:"myInProgress"`
	ResumeItemID *uint64        `json:"resumeItemId"`
}

type myTasksResponse struct {
	Tasks []myTaskView `json:"tasks"`
}

// MyTasks 返回当前 labeler 已领取(有提交)的大任务,按最近活跃排序,供任务广场「已领取的任务」
// 区块与工作台大任务切换器使用。两次查询(我的提交 + 这些任务) + 内存聚合,避免复杂联表。
func (h LabelerHandler) MyTasks(c *gin.Context) {
	claims, _ := middleware.Claims(c)

	var subs []model.Submission
	if err := h.db.Where("labeler_id = ?", claims.UserID).Order("updated_at DESC").Find(&subs).Error; err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list submissions")
		return
	}
	// 我认领的题(整体领取 first_come/assigned 后会一次性认领多题,可能尚未作答 → 没有 submission)。
	var claimedItems []model.TaskItem
	if err := h.db.Where("claimed_by = ?", claims.UserID).Order("priority DESC, id ASC").Find(&claimedItems).Error; err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list claimed items")
		return
	}
	if len(subs) == 0 && len(claimedItems) == 0 {
		httpx.OK(c, myTasksResponse{Tasks: []myTaskView{}})
		return
	}

	type taskAgg struct {
		counts        map[string]int
		total         int
		inProgress    int
		resumeItemID  *uint64
		subItemIDs    map[uint64]bool
		claimedResume *uint64
	}
	order := make([]uint64, 0)
	byTask := make(map[uint64]*taskAgg)
	get := func(taskID uint64) *taskAgg {
		agg, ok := byTask[taskID]
		if !ok {
			agg = &taskAgg{counts: map[string]int{}, subItemIDs: map[uint64]bool{}}
			byTask[taskID] = agg
			order = append(order, taskID)
		}
		return agg
	}
	for _, s := range subs {
		agg := get(s.TaskID) // subs 已按 updated_at DESC,首次出现即最近活跃
		agg.total++
		agg.counts[s.Status]++
		agg.subItemIDs[s.ItemID] = true
		if s.Status == statemachine.StateDraft || s.Status == statemachine.StateRevising {
			agg.inProgress++
			if agg.resumeItemID == nil {
				id := s.ItemID
				agg.resumeItemID = &id // 首个进行中(最近)作为恢复目标
			}
		}
	}
	for _, it := range claimedItems {
		agg := get(it.TaskID)
		if !agg.subItemIDs[it.ID] {
			// 认领但还没作答的题 → 计入进行中(待做),并作为兜底恢复目标。
			agg.inProgress++
			if agg.claimedResume == nil {
				id := it.ID
				agg.claimedResume = &id
			}
		}
	}
	for _, agg := range byTask {
		if agg.resumeItemID == nil {
			agg.resumeItemID = agg.claimedResume
		}
	}

	var tasks []model.Task
	if err := h.db.Where("id IN ?", order).Find(&tasks).Error; err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load tasks")
		return
	}
	taskByID := make(map[uint64]model.Task, len(tasks))
	for _, t := range tasks {
		taskByID[t.ID] = t
	}

	views := make([]myTaskView, 0, len(order))
	for _, id := range order {
		agg := byTask[id]
		task, ok := taskByID[id]
		if !ok {
			task = model.Task{ID: id} // 任务可能已不可见;占位 id,前端仍可显示「任务 #id」
		}
		views = append(views, myTaskView{
			Task:         task,
			MyCounts:     agg.counts,
			MyTotal:      agg.total,
			MyInProgress: agg.inProgress,
			ResumeItemID: agg.resumeItemID,
		})
	}

	httpx.OK(c, myTasksResponse{Tasks: views})
}

func (h LabelerHandler) saveRevision(c *gin.Context, draft bool) {
	task, ok := loadTask(h.db, c)
	if !ok {
		return
	}
	item, ok := loadItem(h.db, c, task.ID)
	if !ok {
		return
	}
	claims, _ := middleware.Claims(c)
	if item.ClaimedBy == nil || *item.ClaimedBy != claims.UserID {
		httpx.Error(c, http.StatusForbidden, "FORBIDDEN", "item is not claimed by current user")
		return
	}
	var req answerRequest
	if !bindLimitedJSON(c, &req, maxAnswerJSONBytes) {
		return
	}
	answerJSON, err := json.Marshal(req.Answer)
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "answer must be valid JSON")
		return
	}
	if int64(len(answerJSON)) > maxAnswerJSONBytes {
		httpx.Error(c, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "answer is too large")
		return
	}

	response, err := submission.Save(h.db, submission.SaveInput{
		Task:      task,
		Item:      item,
		AnswerRaw: answerJSON,
		UserID:    claims.UserID,
		Draft:     draft,
	})
	if err != nil {
		var answerValidationErr *submission.AnswerValidationError
		switch {
		case errors.Is(err, submission.ErrItemNotClaimed):
			httpx.Error(c, http.StatusForbidden, "FORBIDDEN", "item is not claimed by current user")
		case errors.Is(err, submission.ErrLeaseExpired):
			httpx.Error(c, http.StatusConflict, "CONFLICT", "题目租约已过期，请重新领取")
		case errors.Is(err, submission.ErrDailySubmissionLimitReached):
			httpx.Error(c, http.StatusConflict, "CONFLICT", "已达到今日提交上限")
		case errors.Is(err, submission.ErrIncompleteAnswer):
			httpx.Error(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "答案不完整，请填写所有必填项 / Incomplete answer")
		case errors.As(err, &answerValidationErr):
			httpx.Error(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", answerValidationErr.Message)
		case errors.Is(err, submission.ErrInvalidSubmit):
			httpx.Error(c, http.StatusUnprocessableEntity, "INVALID_STATE", err.Error())
		case errors.Is(err, submission.ErrDraftAfterSubmit):
			httpx.Error(c, http.StatusConflict, "CONFLICT", err.Error())
		case errors.Is(err, submission.ErrInvalidTransition):
			httpx.Error(c, http.StatusUnprocessableEntity, "INVALID_STATE", err.Error())
		case errors.Is(err, submission.ErrInvalidUploadedFile):
			httpx.Error(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "answer contains invalid uploaded file reference")
		case errors.Is(err, submission.ErrInvalidAIPrompt):
			httpx.Error(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "active AI prompt is invalid")
		case errors.Is(err, submission.ErrTaskTemplate):
			httpx.Error(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "task template is not available; please contact the owner")
		default:
			httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to save answer")
		}
		return
	}
	httpx.OK(c, response)
}
