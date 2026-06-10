package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"labelhub-api/internal/httpx"
	"labelhub-api/internal/middleware"
	"labelhub-api/internal/model"
	"labelhub-api/internal/service/acceptance"
	"labelhub-api/internal/statemachine"
)

// AcceptanceHandler 暴露 Owner 数据验收闭环的接口。审核「动作」仍归 Reviewer;
// 这里只做 Owner 视角的「质检 / 验收」:发起验收、抽检、验收通过 / 不通过。
type AcceptanceHandler struct {
	db *gorm.DB
}

func NewAcceptanceHandler(db *gorm.DB) AcceptanceHandler {
	return AcceptanceHandler{db: db}
}

func (h AcceptanceHandler) Register(api gin.IRouter) {
	api.GET("/tasks/:taskId/acceptance", middleware.RequireRoles("owner", "admin"), h.Status)
	api.POST("/tasks/:taskId/acceptance", middleware.RequireRoles("owner", "admin"), h.Start)
	api.POST("/tasks/:taskId/acceptance/spot-checks", middleware.RequireRoles("owner", "admin"), h.SpotCheck)
	api.POST("/tasks/:taskId/acceptance/accept", middleware.RequireRoles("owner", "admin"), h.Accept)
	api.POST("/tasks/:taskId/acceptance/reject", middleware.RequireRoles("owner", "admin"), h.Reject)
}

type spotCheckRequest struct {
	BatchID      uint64 `json:"batch_id"`
	SubmissionID uint64 `json:"submission_id"`
	Result       string `json:"result"`
	Note         string `json:"note"`
}

type decideRequest struct {
	BatchID uint64 `json:"batch_id"`
	Note    string `json:"note"`
}

// approvedSubmissionView 是验收看板里一条「已通过」提交的精简视图,带当前答案供 Owner 内联抽检。
type approvedSubmissionView struct {
	ID        uint64   `json:"id"`
	ItemID    uint64   `json:"itemId"`
	LabelerID uint64   `json:"labelerId"`
	AIVerdict *string  `json:"aiVerdict"`
	AIScore   *float64 `json:"aiScore"`
	Answer    string   `json:"answer"`
}

// loadApprovedSubmissions 列出该任务所有「已通过」提交及其当前答案,让 Owner 在验收面板内直接
// 查看内容并就地标记合格 / 不合格,而不必切到 Reviewer 视图或手输提交 ID。
func (h AcceptanceHandler) loadApprovedSubmissions(taskID uint64) ([]approvedSubmissionView, error) {
	rows := []approvedSubmissionView{}
	err := h.db.Table("submissions AS s").
		Select("s.id, s.item_id, s.labeler_id, s.ai_verdict, s.ai_score, r.answer").
		Joins("JOIN submission_revisions r ON r.id = s.current_revision_id").
		Where("s.task_id = ? AND s.status = ?", taskID, statemachine.StateApproved).
		Order("s.id ASC").
		Scan(&rows).Error
	return rows, err
}

// Status 返回该任务最近一个验收批次 + 其抽检记录 + 当前已通过数 + 已通过提交列表(供 Owner 验收看板)。
func (h AcceptanceHandler) Status(c *gin.Context) {
	task, ok := loadOwnedTask(h.db, c)
	if !ok {
		return
	}
	var approvedCount int64
	if err := h.db.Model(&model.Submission{}).
		Where("task_id = ? AND status = ?", task.ID, statemachine.StateApproved).
		Count(&approvedCount).Error; err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to count approved submissions")
		return
	}
	approved, err := h.loadApprovedSubmissions(task.ID)
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load approved submissions")
		return
	}
	var batch model.AcceptanceBatch
	err = h.db.Where("task_id = ?", task.ID).Order("id DESC").First(&batch).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		httpx.OK(c, gin.H{"batch": nil, "spotChecks": []model.AcceptanceSpotCheck{}, "approvedCount": approvedCount, "approvedSubmissions": approved})
		return
	}
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load acceptance batch")
		return
	}
	var checks []model.AcceptanceSpotCheck
	if err := h.db.Where("batch_id = ?", batch.ID).Order("id DESC").Find(&checks).Error; err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load spot checks")
		return
	}
	httpx.OK(c, gin.H{"batch": batch, "spotChecks": checks, "approvedCount": approvedCount, "approvedSubmissions": approved})
}

// Start 对该任务当前已通过数据发起一次验收批次。
func (h AcceptanceHandler) Start(c *gin.Context) {
	task, ok := loadOwnedTask(h.db, c)
	if !ok {
		return
	}
	batch, err := acceptance.Start(h.db, task.ID, currentUserID(c))
	if err != nil {
		h.writeServiceError(c, err)
		return
	}
	httpx.OK(c, gin.H{"batch": batch})
}

// SpotCheck 记录对某条已通过提交的抽检结果(ok / flag)。
func (h AcceptanceHandler) SpotCheck(c *gin.Context) {
	task, ok := loadOwnedTask(h.db, c)
	if !ok {
		return
	}
	var req spotCheckRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid spot-check payload")
		return
	}
	if req.BatchID == 0 || req.SubmissionID == 0 {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "batch_id and submission_id are required")
		return
	}
	check, err := acceptance.RecordSpotCheck(h.db, task.ID, req.BatchID, req.SubmissionID, req.Result, req.Note, currentUserID(c))
	if err != nil {
		h.writeServiceError(c, err)
		return
	}
	httpx.OK(c, gin.H{"spotCheck": check})
}

// Accept 验收通过。
func (h AcceptanceHandler) Accept(c *gin.Context) {
	task, ok := loadOwnedTask(h.db, c)
	if !ok {
		return
	}
	var req decideRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.BatchID == 0 {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "batch_id is required")
		return
	}
	batch, err := acceptance.Accept(h.db, task.ID, req.BatchID, currentUserID(c), req.Note)
	if err != nil {
		h.writeServiceError(c, err)
		return
	}
	httpx.OK(c, gin.H{"batch": batch})
}

// Reject 验收不通过:把抽检 flag 的已通过提交打回人工复审。
func (h AcceptanceHandler) Reject(c *gin.Context) {
	task, ok := loadOwnedTask(h.db, c)
	if !ok {
		return
	}
	var req decideRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.BatchID == 0 {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "batch_id is required")
		return
	}
	result, err := acceptance.Reject(h.db, task.ID, req.BatchID, currentUserID(c), req.Note)
	if err != nil {
		h.writeServiceError(c, err)
		return
	}
	httpx.OK(c, gin.H{"batch": result.Batch, "reopenedCount": result.ReopenedCount})
}

// writeServiceError 把 acceptance service 的 sentinel error 映射到 HTTP。
func (h AcceptanceHandler) writeServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, acceptance.ErrActiveBatchExists):
		httpx.Error(c, http.StatusConflict, "CONFLICT", "this task already has a pending acceptance batch")
	case errors.Is(err, acceptance.ErrBatchNotPending):
		httpx.Error(c, http.StatusConflict, "CONFLICT", "acceptance batch is no longer pending")
	case errors.Is(err, acceptance.ErrConcurrentWrite):
		httpx.Error(c, http.StatusConflict, "CONFLICT", "acceptance batch changed concurrently, please reload")
	case errors.Is(err, acceptance.ErrBatchNotFound), errors.Is(err, acceptance.ErrBatchTaskMismatch):
		httpx.Error(c, http.StatusNotFound, "NOT_FOUND", "acceptance batch not found")
	case errors.Is(err, acceptance.ErrSubmissionNotEligible):
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "submission is not an approved item of this task")
	case errors.Is(err, acceptance.ErrInvalidResult):
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "result must be ok or flag")
	default:
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "acceptance operation failed")
	}
}
