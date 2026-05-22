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
	api.GET("/tasks/:taskId/items/:itemId", middleware.RequireRoles("labeler", "reviewer", "owner", "admin"), h.GetItem)
	api.POST("/tasks/:taskId/items/:itemId/draft", middleware.RequireRoles("labeler"), h.SaveDraft)
	api.POST("/tasks/:taskId/items/:itemId/submit", middleware.RequireRoles("labeler"), h.SubmitItem)
	api.GET("/me/submissions", middleware.RequireRoles("labeler"), h.MySubmissions)
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
	case errors.Is(err, submission.ErrClaimRaceLost):
		httpx.Error(c, http.StatusConflict, "CONFLICT", "claim race lost, please retry")
	default:
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to claim item")
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
	if err := h.db.Where("item_id = ?", item.ID).First(&submission).Error; err == nil {
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
		switch {
		case errors.Is(err, submission.ErrInvalidSubmit):
			httpx.Error(c, http.StatusUnprocessableEntity, "INVALID_STATE", err.Error())
		case errors.Is(err, submission.ErrDraftAfterSubmit):
			httpx.Error(c, http.StatusConflict, "CONFLICT", err.Error())
		case errors.Is(err, submission.ErrInvalidTransition):
			httpx.Error(c, http.StatusUnprocessableEntity, "INVALID_STATE", err.Error())
		default:
			httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to save answer")
		}
		return
	}
	httpx.OK(c, response)
}
