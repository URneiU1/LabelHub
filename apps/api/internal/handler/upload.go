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

// UploadHandler 封装 /uploads 端点。
// 上传走两阶段:先落 temp 状态,绑定到 submission revision 时同事务标 attached;
// 24h 未 attached 的 temp 文件由 cron 清理(Sprint 4 加 cron)。
type UploadHandler struct {
	db *gorm.DB
}

func NewUploadHandler(db *gorm.DB) UploadHandler {
	return UploadHandler{db: db}
}

func (h UploadHandler) Register(api gin.IRouter) {
	api.POST("/uploads", middleware.RequireRoles("labeler", "owner", "reviewer", "admin"), h.Upload)
}

func (h UploadHandler) Upload(c *gin.Context) {
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

// --- upload-internal helpers(被 s1_test.go 引用,故保留包级符号)---

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
