package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"labelhub-api/internal/httpx"
	"labelhub-api/internal/middleware"
	"labelhub-api/internal/model"
)

const (
	itemStatusAvailable = "available"
	itemStatusClaimed   = "claimed"
	itemStatusFinished  = "finished"

	uploadMaxBytes       = 10 * 1024 * 1024
	uploadImageMaxBytes  = 5 * 1024 * 1024
	defaultUploadBaseDir = "./data/uploads"
)

var (
	allowedUploadMIME = map[string]string{
		"image/png":        ".png",
		"image/jpeg":       ".jpg",
		"image/webp":       ".webp",
		"application/pdf":  ".pdf",
		"text/plain":       ".txt",
		"application/json": ".json",
	}
	imageMIMEs = map[string]struct{}{
		"image/png":  {},
		"image/jpeg": {},
		"image/webp": {},
	}
)

type S1Handler struct {
	db *gorm.DB
}

func NewS1Handler(db *gorm.DB) S1Handler {
	return S1Handler{db: db}
}

func (h S1Handler) Register(api gin.IRouter) {
	api.GET("/tasks", middleware.RequireRoles("owner", "admin"), h.ListTasks)
	api.POST("/tasks", middleware.RequireRoles("owner", "admin"), h.CreateTask)
	api.GET("/tasks/:taskId", middleware.RequireRoles("owner", "admin", "labeler", "reviewer"), h.GetTask)
	api.POST("/tasks/:taskId/items/import", middleware.RequireRoles("owner", "admin"), h.ImportItems)
	api.GET("/tasks/:taskId/export/json", middleware.RequireRoles("owner", "admin"), h.ExportJSON)

	api.GET("/labeler/tasks", middleware.RequireRoles("labeler"), h.ListPublishedTasks)
	api.POST("/tasks/:taskId/claim", middleware.RequireRoles("labeler"), h.ClaimItem)
	api.GET("/tasks/:taskId/items/:itemId", middleware.RequireRoles("labeler", "reviewer", "owner", "admin"), h.GetItem)
	api.POST("/tasks/:taskId/items/:itemId/draft", middleware.RequireRoles("labeler"), h.SaveDraft)
	api.POST("/tasks/:taskId/items/:itemId/submit", middleware.RequireRoles("labeler"), h.SubmitItem)
	api.GET("/me/submissions", middleware.RequireRoles("labeler"), h.MySubmissions)

	api.GET("/reviewer/submissions", middleware.RequireRoles("reviewer", "owner", "admin"), h.ReviewerQueue)
	api.POST("/submissions/:submissionId/review", middleware.RequireRoles("reviewer", "owner", "admin"), h.ReviewSubmission)

	api.POST("/llm/inline", middleware.RequireRoles("labeler", "owner", "reviewer", "admin"), h.InlineLLM)
	api.POST("/uploads", middleware.RequireRoles("labeler", "owner", "reviewer", "admin"), h.Upload)
}

type createTaskRequest struct {
	Title               string `json:"title" binding:"required"`
	Description         string `json:"description"`
	BaselineDescription string `json:"baselineDescription"`
}

type importItemsRequest struct {
	Items []map[string]any `json:"items" binding:"required"`
}

type answerRequest struct {
	Answer map[string]any `json:"answer" binding:"required"`
}

type reviewRequest struct {
	Verdict string `json:"verdict" binding:"required"`
	Reason  string `json:"reason"`
}

type inlineLLMRequest struct {
	Prompt string         `json:"prompt"`
	Input  map[string]any `json:"input"`
}

func (h S1Handler) InlineLLM(c *gin.Context) {
	var req inlineLLMRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
		return
	}
	time.Sleep(800 * time.Millisecond)
	text := fmt.Sprintf("AI 预评分:建议重点检查相关性、准确性、格式合规与安全性。输入长度=%d。", len(req.Prompt)+len(fmt.Sprint(req.Input)))
	httpx.OK(c, gin.H{"text": text, "provider": "mock"})
}

func (h S1Handler) Upload(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "file is required")
		return
	}
	taskID, _ := strconv.ParseUint(c.PostForm("task_id"), 10, 64)
	if taskID == 0 {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "task_id is required")
		return
	}

	mime := file.Header.Get("Content-Type")
	ext, ok := allowedUploadMIME[mime]
	if !ok {
		httpx.ErrorWithDetails(c, http.StatusBadRequest, "VALIDATION_ERROR",
			"unsupported MIME type", gin.H{"mime": mime, "allowed": allowedMIMEKeys()})
		return
	}
	if _, isImage := imageMIMEs[mime]; isImage && file.Size > uploadImageMaxBytes {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "image must be <= 5MB")
		return
	}
	if file.Size > uploadMaxBytes {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "file must be <= 10MB")
		return
	}

	claims, _ := middleware.Claims(c)
	key := storageKey(file.Filename) + ext
	uploadDir := envOrDefault("UPLOAD_DIR", defaultUploadBaseDir)
	dest := filepath.Join(uploadDir, strconv.FormatUint(taskID, 10), key)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to prepare upload dir")
		return
	}
	if err := c.SaveUploadedFile(file, dest); err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to write upload")
		return
	}

	uploaded := model.UploadedFile{
		TaskID:       taskID,
		StorageKey:   key,
		OriginalName: filepath.Base(file.Filename),
		MimeType:     mime,
		SizeBytes:    uint64(file.Size),
		Status:       "temp",
		CreatedBy:    claims.UserID,
	}
	if err := h.db.Create(&uploaded).Error; err != nil {
		_ = os.Remove(dest)
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to save upload metadata")
		return
	}
	httpx.OK(c, uploaded)
}

func envOrDefault(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func allowedMIMEKeys() []string {
	keys := make([]string, 0, len(allowedUploadMIME))
	for k := range allowedUploadMIME {
		keys = append(keys, k)
	}
	return keys
}

func storageKey(name string) string {
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", name, time.Now().UnixNano())))
	return hex.EncodeToString(hash[:])
}
