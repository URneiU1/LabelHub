package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"labelhub-api/internal/httpx"
	"labelhub-api/internal/middleware"
	"labelhub-api/internal/model"
	"labelhub-api/internal/service/export"

	"labelhub.local/exporter"
)

// ExportHandler 封装 /tasks/:taskId/export* 端点。
// S1 同步 JSON 保留;S4 加异步多格式导出(入队/历史/签名下载)。
type ExportHandler struct {
	db     *gorm.DB
	secret string
	ttl    time.Duration
}

func NewExportHandler(db *gorm.DB, downloadSecret string, ttl time.Duration) ExportHandler {
	return ExportHandler{db: db, secret: downloadSecret, ttl: ttl}
}

func (h ExportHandler) Register(api gin.IRouter) {
	api.GET("/tasks/:taskId/export/json", middleware.RequireRoles("owner", "admin"), h.ExportJSON)
	api.POST("/tasks/:taskId/exports", middleware.RequireRoles("owner", "admin"), h.CreateExport)
	api.GET("/tasks/:taskId/exports", middleware.RequireRoles("owner", "admin"), h.ListExports)
	api.GET("/tasks/:taskId/exports/:exportId/download-url", middleware.RequireRoles("owner", "admin"), h.DownloadURL)
}

// RegisterPublic 挂在未鉴权的 api 组(签名 token 即鉴权)。
func (h ExportHandler) RegisterPublic(api gin.IRouter) {
	api.GET("/exports/download", h.Download)
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

type createExportRequest struct {
	Format         string          `json:"format"`
	FieldMap       json.RawMessage `json:"field_map"` // 保留原始字节, 避免再 marshal 坍塌大整数(S3 教训)
	IncludeReviews bool            `json:"include_reviews"`
}

func (h ExportHandler) CreateExport(c *gin.Context) {
	task, ok := loadOwnedTask(h.db, c)
	if !ok {
		return
	}
	var req createExportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid export request body")
		return
	}
	if !exporter.SupportedFormat(req.Format) {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "format must be one of json, jsonl, csv, xlsx")
		return
	}
	var fieldMap *string
	if len(req.FieldMap) > 0 && string(req.FieldMap) != "null" {
		raw := string(req.FieldMap)
		fieldMap = &raw
	}

	record, err := export.Enqueue(h.db, export.EnqueueInput{
		Task:           task,
		CreatedBy:      currentUserID(c),
		Format:         req.Format,
		FieldMap:       fieldMap,
		IncludeReviews: req.IncludeReviews,
	})
	if err != nil {
		if errors.Is(err, export.ErrUnsupportedFormat) {
			httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "unsupported format")
			return
		}
		var jsonErr *json.SyntaxError
		if errors.As(err, &jsonErr) {
			httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid field_map json")
			return
		}
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to enqueue export")
		return
	}
	httpx.OK(c, gin.H{"id": record.ID, "status": record.Status})
}

func (h ExportHandler) ListExports(c *gin.Context) {
	task, ok := loadOwnedTask(h.db, c)
	if !ok {
		return
	}
	records, err := export.ListByTask(h.db, task.ID, 50)
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list exports")
		return
	}
	httpx.OK(c, gin.H{"exports": records})
}

// DownloadURL 为已完成的导出签发一个限时 HMAC 下载 URL(owner 边界)。
func (h ExportHandler) DownloadURL(c *gin.Context) {
	task, ok := loadOwnedTask(h.db, c)
	if !ok {
		return
	}
	exportID, ok := parseIDParam(c, "exportId")
	if !ok {
		return
	}
	if h.secret == "" {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "download secret not configured")
		return
	}
	var record model.Export
	err := h.db.Where("id = ? AND task_id = ? AND status = ?", exportID, task.ID, "succeeded").First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		httpx.Error(c, http.StatusNotFound, "NOT_FOUND", "export not found or not finished")
		return
	}
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load export")
		return
	}
	token := exporter.SignDownloadToken(h.secret, record.ID, time.Now().Add(h.ttl))
	httpx.OK(c, gin.H{
		"url":       "/api/v1/exports/download?token=" + url.QueryEscape(token),
		"expiresIn": int(h.ttl.Seconds()),
	})
}

// Download 校验签名 token 后流式返回导出文件(公开路由,token 即鉴权)。
func (h ExportHandler) Download(c *gin.Context) {
	if h.secret == "" {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "download secret not configured")
		return
	}
	exportID, err := exporter.VerifyDownloadToken(h.secret, c.Query("token"), time.Now())
	if errors.Is(err, exporter.ErrTokenExpired) {
		httpx.Error(c, http.StatusGone, "GONE", "download link expired")
		return
	}
	if err != nil {
		httpx.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid download token")
		return
	}
	var record model.Export
	if err := h.db.Where("id = ? AND status = ?", exportID, "succeeded").First(&record).Error; err != nil {
		httpx.Error(c, http.StatusNotFound, "NOT_FOUND", "export not found")
		return
	}
	if !record.FilePath.Valid {
		httpx.Error(c, http.StatusNotFound, "NOT_FOUND", "export file missing")
		return
	}
	absPath, ok := safeExportPath(record.FilePath.String)
	if !ok {
		httpx.Error(c, http.StatusForbidden, "FORBIDDEN", "invalid export file path")
		return
	}
	filename := fmt.Sprintf("task-%d-%s-%d%s", record.TaskID, record.Format, record.ID, exporter.FileExtension(record.Format))
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Header("Content-Type", exporter.ContentType(record.Format))
	c.File(absPath)
}

// safeExportPath 防路径穿越:确认 stored 路径 Clean 后落在 EXPORT_DIR 之内。
// 返回可供 c.File 使用的绝对路径。
func safeExportPath(stored string) (string, bool) {
	base := os.Getenv("EXPORT_DIR")
	if base == "" {
		base = "./data/exports"
	}
	absBase, err := filepath.Abs(base)
	if err != nil {
		return "", false
	}
	absFile, err := filepath.Abs(filepath.Clean(stored))
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(absBase, absFile)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return absFile, true
}
