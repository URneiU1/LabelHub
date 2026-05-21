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
	"labelhub-api/internal/statemachine"
)

func (h S1Handler) ListPublishedTasks(c *gin.Context) {
	var tasks []model.Task
	if err := h.db.Where("status = ?", "published").Order("id DESC").Find(&tasks).Error; err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list tasks")
		return
	}
	httpx.PageOK(c, tasks, httpx.Page{})
}

func (h S1Handler) ClaimItem(c *gin.Context) {
	task, ok := h.loadTask(c)
	if !ok {
		return
	}
	claims, _ := middleware.Claims(c)

	var claimed model.TaskItem
	err := h.db.Where("task_id = ? AND claimed_by = ? AND status = ?", task.ID, claims.UserID, itemStatusClaimed).Order("id").First(&claimed).Error
	if err == nil {
		h.respondItem(c, task, claimed)
		return
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load claimed item")
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
	h.respondItem(c, task, claimed)
}

func (h S1Handler) GetItem(c *gin.Context) {
	task, ok := h.loadTask(c)
	if !ok {
		return
	}
	if !h.canReadTask(c, task) {
		return
	}
	item, ok := h.loadItem(c, task.ID)
	if !ok {
		return
	}
	h.respondItem(c, task, item)
}

func (h S1Handler) SaveDraft(c *gin.Context) {
	h.saveRevision(c, true)
}

func (h S1Handler) SubmitItem(c *gin.Context) {
	h.saveRevision(c, false)
}

func (h S1Handler) MySubmissions(c *gin.Context) {
	claims, _ := middleware.Claims(c)
	var submissions []model.Submission
	if err := h.db.Where("labeler_id = ?", claims.UserID).Order("updated_at DESC").Find(&submissions).Error; err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list submissions")
		return
	}
	httpx.PageOK(c, submissions, httpx.Page{})
}

func (h S1Handler) respondItem(c *gin.Context, task model.Task, item model.TaskItem) {
	template, _ := h.currentTemplate(task.ID)
	var submission model.Submission
	_ = h.db.Where("item_id = ?", item.ID).First(&submission).Error
	var revision *model.SubmissionRevision
	if submission.ID != 0 && submission.CurrentRevisionID != nil {
		var current model.SubmissionRevision
		if err := h.db.First(&current, *submission.CurrentRevisionID).Error; err == nil {
			revision = &current
		}
	}
	httpx.OK(c, gin.H{"task": task, "item": item, "template": template, "submission": submission, "revision": revision})
}

func (h S1Handler) saveRevision(c *gin.Context, draft bool) {
	task, ok := h.loadTask(c)
	if !ok {
		return
	}
	item, ok := h.loadItem(c, task.ID)
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

func (h S1Handler) findOrCreateSubmission(tx *gorm.DB, task model.Task, item model.TaskItem, labelerID uint64) (model.Submission, error) {
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
		if template, err := h.currentTemplate(task.ID); err == nil {
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
