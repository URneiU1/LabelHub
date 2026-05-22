package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"labelhub-api/internal/httpx"
	"labelhub-api/internal/middleware"
	"labelhub-api/internal/service/export"
)

// ExportHandler 封装 /tasks/:taskId/export/* 端点。
// Sprint 1 只做同步 JSON 导出,Sprint 4 扩 JSONL / CSV / XLSX(+Markdown 加分)+ 异步队列 + 字段映射 UI。
type ExportHandler struct {
	db *gorm.DB
}

func NewExportHandler(db *gorm.DB) ExportHandler {
	return ExportHandler{db: db}
}

func (h ExportHandler) Register(api gin.IRouter) {
	api.GET("/tasks/:taskId/export/json", middleware.RequireRoles("owner", "admin"), h.ExportJSON)
}

func (h ExportHandler) ExportJSON(c *gin.Context) {
	task, ok := loadOwnedTask(h.db, c)
	if !ok {
		return
	}
	claims, _ := middleware.Claims(c)
	includeReviews := c.DefaultQuery("include_reviews", "false") == "true"

	result, err := export.RunJSON(h.db, export.JSONInput{
		Task:           task,
		CreatedBy:      claims.UserID,
		IncludeReviews: includeReviews,
	})
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to export task")
		return
	}
	httpx.OK(c, gin.H{"task": result.Task, "rows": result.Rows, "include_reviews": result.IncludeReviews})
}
