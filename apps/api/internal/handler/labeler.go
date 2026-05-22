package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"labelhub-api/internal/httpx"
	"labelhub-api/internal/middleware"
	"labelhub-api/internal/model"
	"labelhub-api/internal/policy"
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

	var response model.Submission
	err = h.db.Transaction(func(tx *gorm.DB) error {
		submission, err := h.findOrCreateSubmission(tx, task, item, claims.UserID)
		if err != nil {
			return err
		}
		from := submission.Status

		revisionNo, err := nextRevisionNo(tx, submission.ID)
		if err != nil {
			return err
		}
		revision := model.SubmissionRevision{
			SubmissionID: submission.ID,
			RevisionNo:   revisionNo,
			Answer:       string(answerJSON),
			Draft:        draft,
			CreatedBy:    claims.UserID,
		}
		if err := tx.Create(&revision).Error; err != nil {
			return err
		}

		to := from
		submitEvent := ""
		dispatchEvent := ""
		updates := map[string]any{"current_revision_id": revision.ID}
		if !draft {
			if from != statemachine.StateDraft && from != statemachine.StateRevising {
				return errors.New("submission cannot be submitted from " + from)
			}
			if err := statemachine.Apply(from, statemachine.EventSubmit, statemachine.StateSubmitted); err != nil {
				return err
			}
			submitEvent = statemachine.EventSubmit
			to = statemachine.StateSubmitted
			if task.AIReviewEnabled {
				to = statemachine.StateAIReviewing
				dispatchEvent = statemachine.EventEnqueue
			} else {
				to = statemachine.StateHumanReviewing
				dispatchEvent = statemachine.EventSkipAI
			}
			if err := statemachine.Apply(statemachine.StateSubmitted, dispatchEvent, to); err != nil {
				return err
			}
			for key, value := range resubmitClearedFields(to, time.Now().UTC()) {
				updates[key] = value
			}
		} else if from != statemachine.StateDraft {
			return errors.New("draft can only be saved before first submit")
		}
		if draft && from == statemachine.StateDraft {
			if err := statemachine.Apply(from, statemachine.EventSave, statemachine.StateDraft); err != nil {
				return err
			}
		}

		if err := tx.Model(&model.Submission{}).Where("id = ?", submission.ID).Updates(updates).Error; err != nil {
			return err
		}
		if task.AIReviewEnabled && !draft {
			outbox := model.OutboxEvent{Topic: "ai.review.requested", Payload: fmt.Sprintf(`{"submission_id":%d,"revision_id":%d}`, submission.ID, revision.ID), Status: "pending"}
			if err := tx.Create(&outbox).Error; err != nil {
				return err
			}
		}
		if !draft {
			if err := createAuditLog(tx, "submission", submission.ID, from, statemachine.StateSubmitted, "user", &claims.UserID, submitEvent, nil); err != nil {
				return err
			}
			if err := createAuditLog(tx, "submission", submission.ID, statemachine.StateSubmitted, to, "user", &claims.UserID, dispatchEvent, nil); err != nil {
				return err
			}
		}
		return tx.First(&response, submission.ID).Error
	})
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to save answer")
		return
	}
	httpx.OK(c, response)
}

func (h LabelerHandler) findOrCreateSubmission(tx *gorm.DB, task model.Task, item model.TaskItem, labelerID uint64) (model.Submission, error) {
	var submission model.Submission
	err := tx.Where("item_id = ?", item.ID).First(&submission).Error
	if err == nil {
		return submission, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return model.Submission{}, err
	}
	templateVersion := 1
	if task.TemplateID != nil {
		// 历史实现:此查询走 h.db 而非传入的 tx,Phase 2 严格保持原行为不动。
		// Phase 3 抽 service 层时再统一 tx vs db 的语义。
		if template, err := currentTemplate(h.db, task.ID); err == nil {
			templateVersion = template.Version
		}
	}
	submission = model.Submission{
		TaskID:          task.ID,
		ItemID:          item.ID,
		TemplateVersion: templateVersion,
		LabelerID:       labelerID,
		Status:          statemachine.StateDraft,
	}
	return submission, tx.Create(&submission).Error
}

func resubmitClearedFields(to string, now time.Time) map[string]any {
	return map[string]any{
		"status":        to,
		"submitted_at":  now,
		"ai_verdict":    nil,
		"ai_score":      nil,
		"human_verdict": nil,
	}
}
