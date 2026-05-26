package handler

// request_helpers 收纳 handler 包内复用的"绑路径参数 → 查库 → 写 HTTP 错误"小工具。
// 跨业务但仅供 handler 自用,故保持 unexported。

import (
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
)

type humanReviewSummaryResponse struct {
	Verdict   string           `json:"verdict"`
	Reason    model.NullString `json:"reason"`
	CreatedAt time.Time        `json:"createdAt"`
}

func loadTask(db *gorm.DB, c *gin.Context) (model.Task, bool) {
	taskID, ok := parseIDParam(c, "taskId")
	if !ok {
		return model.Task{}, false
	}
	var task model.Task
	if err := db.First(&task, taskID).Error; err != nil {
		httpx.Error(c, http.StatusNotFound, "NOT_FOUND", "task not found")
		return model.Task{}, false
	}
	return task, true
}

func loadOwnedTask(db *gorm.DB, c *gin.Context) (model.Task, bool) {
	task, ok := loadTask(db, c)
	if !ok {
		return model.Task{}, false
	}
	claims, _ := middleware.Claims(c)
	if policy.IsTaskOwner(claims, task) {
		return task, true
	}
	httpx.Error(c, http.StatusForbidden, "FORBIDDEN", "task does not belong to current owner")
	return model.Task{}, false
}

// enforceCanReadTask:policy 判可见 + 不可见时写 403。
func enforceCanReadTask(c *gin.Context, task model.Task) bool {
	claims, _ := middleware.Claims(c)
	if policy.CanReadTask(claims, task) {
		return true
	}
	httpx.Error(c, http.StatusForbidden, "FORBIDDEN", "task is not visible to current user")
	return false
}

func loadItem(db *gorm.DB, c *gin.Context, taskID uint64) (model.TaskItem, bool) {
	itemID, ok := parseIDParam(c, "itemId")
	if !ok {
		return model.TaskItem{}, false
	}
	var item model.TaskItem
	if err := db.Where("id = ? AND task_id = ?", itemID, taskID).First(&item).Error; err != nil {
		httpx.Error(c, http.StatusNotFound, "NOT_FOUND", "item not found")
		return model.TaskItem{}, false
	}
	return item, true
}

func currentTemplate(db *gorm.DB, taskID uint64) (model.TaskTemplate, error) {
	var template model.TaskTemplate
	err := db.Where("task_id = ?", taskID).Order("version DESC").First(&template).Error
	return template, err
}

func templateForBundle(db *gorm.DB, task model.Task, submission model.Submission) (model.TaskTemplate, error) {
	var template model.TaskTemplate
	if submission.ID != 0 && submission.Status == "revising" {
		err := db.Where("task_id = ? AND version = ?", task.ID, submission.TemplateVersion).First(&template).Error
		return template, err
	}
	if task.TemplateID != nil {
		err := db.Where("id = ? AND task_id = ?", *task.TemplateID, task.ID).First(&template).Error
		return template, err
	}
	return currentTemplate(db, task.ID)
}

// respondItem 用 task + item 拼标准化的"答题视图"(task / item / template / submission / revision)。
// labeler 的 ClaimItem / GetItem 两条路径都收敛到这里,保持 HTTP 响应结构一致。
func respondItem(db *gorm.DB, c *gin.Context, task model.Task, item model.TaskItem) {
	var submission model.Submission
	submissionErr := db.Where("item_id = ?", item.ID).First(&submission).Error
	if submissionErr != nil && !errors.Is(submissionErr, gorm.ErrRecordNotFound) {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load submission")
		return
	}

	template, templateErr := templateForBundle(db, task, submission)
	if templateErr != nil && !errors.Is(templateErr, gorm.ErrRecordNotFound) {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load template")
		return
	}

	var revision *model.SubmissionRevision
	if submission.ID != 0 && submission.CurrentRevisionID != nil {
		var current model.SubmissionRevision
		if err := db.First(&current, *submission.CurrentRevisionID).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load revision")
				return
			}
		} else {
			revision = &current
		}
	}

	payload := gin.H{"task": task, "item": item, "template": template, "submission": submission, "revision": revision}
	if submission.ID != 0 && submission.Status == "revising" {
		var human model.HumanReview
		if err := db.Where("submission_id = ?", submission.ID).Order("created_at DESC, id DESC").First(&human).Error; err == nil {
			payload["latestHumanReview"] = humanReviewSummaryResponse{
				Verdict:   human.Verdict,
				Reason:    human.Reason,
				CreatedAt: human.CreatedAt,
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load latest human review")
			return
		}
	}
	// 已发布的 task 缺模板属于配置缺失,显式 warning 让前端能渲染 setup-incomplete 状态。
	if errors.Is(templateErr, gorm.ErrRecordNotFound) {
		payload["warnings"] = []string{"template_missing"}
	}
	httpx.OK(c, payload)
}

func parseIDParam(c *gin.Context, name string) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil || id == 0 {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", name+" must be a positive integer")
		return 0, false
	}
	return id, true
}
