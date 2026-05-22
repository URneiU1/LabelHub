package handler

// 临时聚合文件:Phase 2 把原 S1Handler 上的方法转成包级函数,
// Phase 3 把跨 Sprint 复用的(createAuditLog / aiReviewToMap 等)搬到 service 层,
// Phase 4 把剩余 jsonx / hasRole / parseIDParam 等散到对应包,删掉本文件。

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"labelhub-api/internal/httpx"
	"labelhub-api/internal/middleware"
	"labelhub-api/internal/model"
	"labelhub-api/internal/policy"
)

// itemStatus* 是 task_items.status 的字面值,跨 handler 共用(labeler 领单 / 写完;reviewer 设 finished;task 导入设 available)。
const (
	itemStatusAvailable = "available"
	itemStatusClaimed   = "claimed"
	itemStatusFinished  = "finished"
)

// --- task helpers(原 S1Handler 方法,改包级)---

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

// enforceCanReadTask 对应原 S1Handler.canReadTask:policy 判可见 + 不可见时写 403。
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

// respondItem 用 task + item 拼标准化的"答题视图"(task / item / template / submission / revision)。
// labeler 的 ClaimItem / GetItem 两条路径都收敛到这里,保持 HTTP 响应结构一致。
func respondItem(db *gorm.DB, c *gin.Context, task model.Task, item model.TaskItem) {
	template, templateErr := currentTemplate(db, task.ID)
	if templateErr != nil && !errors.Is(templateErr, gorm.ErrRecordNotFound) {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load template")
		return
	}

	var submission model.Submission
	submissionErr := db.Where("item_id = ?", item.ID).First(&submission).Error
	if submissionErr != nil && !errors.Is(submissionErr, gorm.ErrRecordNotFound) {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load submission")
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
	// 已发布的 task 缺模板属于配置缺失,显式 warning 让前端能渲染 setup-incomplete 状态。
	if errors.Is(templateErr, gorm.ErrRecordNotFound) {
		payload["warnings"] = []string{"template_missing"}
	}
	httpx.OK(c, payload)
}

// --- audit log / json / param helpers(Phase 4 会拆到对应包)---

// hasRole 保留为兼容层,Phase 4 删;新代码请直接调 policy.HasRole。
func hasRole(roles []string, target string) bool {
	for _, role := range roles {
		if role == target {
			return true
		}
	}
	return false
}

func mustJSON(raw string) any {
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return raw
	}
	return value
}

func ptrInt(value int) *int {
	return &value
}

func aiReviewToMap(review model.AIReview) map[string]any {
	return map[string]any{
		"verdict":        review.Verdict,
		"overall_score":  review.OverallScore,
		"dimensions":     unwrapJSONPointer(review.Dimensions),
		"reason":         review.Reason,
		"prompt_version": review.PromptVersion,
		"created_at":     review.CreatedAt,
	}
}

func humanReviewToMap(review model.HumanReview) map[string]any {
	return map[string]any{
		"verdict":     review.Verdict,
		"reason":      review.Reason,
		"stage":       review.Stage,
		"reviewer_id": review.ReviewerID,
		"created_at":  review.CreatedAt,
	}
}

func unwrapJSONPointer(raw *string) any {
	if raw == nil {
		return nil
	}
	return mustJSON(*raw)
}

// nextRevisionFromMax 兼容垫片:s1_test.go 单测此契约,Phase 4 把测试搬到 service/submission
// 后删本垫片。生产路径已不走这里,改由 submission.Save 内部调 service 包私有同名函数。
func nextRevisionFromMax(valid bool, max int64) int {
	if !valid {
		return 1
	}
	return int(max) + 1
}

func parseIDParam(c *gin.Context, name string) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil || id == 0 {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", name+" must be a positive integer")
		return 0, false
	}
	return id, true
}

func nullString(value string) model.NullString {
	return model.StringFrom(value)
}

func nullStringJSON(value model.NullString) any {
	if !value.Valid {
		return nil
	}
	return value.String
}

// registerAllHandlers 把 6 个 handler 一次性注册到 router,供测试用。
// main.go 不走这里,而是显式列出 6 个 NewXHandler(db).Register(...) 让路由分组可见。
func registerAllHandlers(r gin.IRouter, db *gorm.DB) {
	NewTaskHandler(db).Register(r)
	NewLabelerHandler(db).Register(r)
	NewReviewerHandler(db).Register(r)
	NewUploadHandler(db).Register(r)
	NewLLMHandler().Register(r)
	NewExportHandler(db).Register(r)
}
