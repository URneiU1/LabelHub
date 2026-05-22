package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

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
	task, ok := loadTask(h.db, c)
	if !ok {
		return
	}
	claims, _ := middleware.Claims(c)

	// Path A:已经 claim 中的 item 直接 resume。即使 task 后来被 pause/archive,
	// 也允许 labeler 把手上的活做完(否则会卡死他的 in-flight 工作)。
	var claimed model.TaskItem
	err := h.db.Where("task_id = ? AND claimed_by = ? AND status = ?", task.ID, claims.UserID, itemStatusClaimed).Order("id").First(&claimed).Error
	if err == nil {
		respondItem(h.db, c, task, claimed)
		return
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load claimed item")
		return
	}

	// Path B:尝试领新题前,task 必须是 published。draft/paused/archived 一律 409。
	if decision := policy.CanClaimNew(claims, task); !decision.Allowed {
		switch decision.Reason {
		case policy.ClaimDenyNotLabeler:
			httpx.Error(c, http.StatusForbidden, "FORBIDDEN", "only labeler can claim items")
		case policy.ClaimDenyTaskNotPublished:
			httpx.Error(c, http.StatusConflict, "CONFLICT",
				"task is not accepting new claims (status="+decision.TaskStatus+")")
		default:
			httpx.Error(c, http.StatusForbidden, "FORBIDDEN", "claim denied")
		}
		return
	}

	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("task_id = ? AND status = ?", task.ID, itemStatusAvailable).
			Order("id").
			First(&claimed).Error; err != nil {
			return err
		}
		now := time.Now().UTC()
		return tx.Model(&claimed).Updates(map[string]any{
			"status":     itemStatusClaimed,
			"claimed_by": claims.UserID,
			"claimed_at": now,
		}).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		httpx.Error(c, http.StatusConflict, "CONFLICT", "没有可领取的题目")
		return
	}
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to claim item")
		return
	}

	claimed.ClaimedBy = &claims.UserID
	claimed.Status = itemStatusClaimed
	respondItem(h.db, c, task, claimed)
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
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "answer is required")
		return
	}
	answerJSON, err := json.Marshal(req.Answer)
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "answer must be valid JSON")
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
