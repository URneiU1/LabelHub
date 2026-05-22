package handler

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"labelhub-api/internal/auth"
	"labelhub-api/internal/httpx"
	"labelhub-api/internal/middleware"
	"labelhub-api/internal/model"
	"labelhub-api/internal/policy"
	"labelhub-api/internal/statemachine"
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

	var task model.Task
	if err := h.db.First(&task, taskID).Error; err != nil {
		httpx.Error(c, http.StatusNotFound, "NOT_FOUND", "task not found")
		return
	}
	claims, _ := middleware.Claims(c)
	allowed, err := h.canUploadToTask(claims, task)
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to check upload access")
		return
	}
	if !allowed {
		httpx.Error(c, http.StatusForbidden, "FORBIDDEN", "upload is not allowed for this task")
		return
	}

	if file.Size <= 0 {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "file must not be empty")
		return
	}
	declaredMIME := normalizeMIME(file.Header.Get("Content-Type"))
	ext, ok := allowedUploadMIME[declaredMIME]
	if !ok {
		httpx.ErrorWithDetails(c, http.StatusBadRequest, "VALIDATION_ERROR",
			"unsupported MIME type", gin.H{"mime": declaredMIME, "allowed": allowedMIMEKeys()})
		return
	}
	if _, isImage := imageMIMEs[declaredMIME]; isImage && file.Size > uploadImageMaxBytes {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "image must be <= 5MB")
		return
	}
	if file.Size > uploadMaxBytes {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "file must be <= 10MB")
		return
	}
	if !uploadContentMatches(file, declaredMIME) {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "file content does not match declared MIME type")
		return
	}
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
		MimeType:     declaredMIME,
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

func (h UploadHandler) canUploadToTask(claims *auth.Claims, task model.Task) (bool, error) {
	if policy.HasRole(claims, policy.RoleAdmin) || policy.IsTaskOwner(claims, task) {
		return true, nil
	}
	if policy.HasRole(claims, policy.RoleLabeler) {
		var count int64
		err := h.db.Model(&model.TaskItem{}).
			Where("task_id = ? AND claimed_by = ? AND status = ?", task.ID, claims.UserID, itemStatusClaimed).
			Count(&count).Error
		return count > 0, err
	}
	if policy.HasRole(claims, policy.RoleReviewer) {
		var count int64
		err := h.db.Model(&model.Submission{}).
			Where("task_id = ? AND status = ?", task.ID, statemachine.StateHumanReviewing).
			Count(&count).Error
		return count > 0, err
	}
	return false, nil
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

func normalizeMIME(value string) string {
	mediaType, _, err := mime.ParseMediaType(value)
	if err != nil {
		return strings.ToLower(strings.TrimSpace(value))
	}
	return strings.ToLower(strings.TrimSpace(mediaType))
}

func uploadContentMatches(file *multipart.FileHeader, declaredMIME string) bool {
	src, err := file.Open()
	if err != nil {
		return false
	}
	defer src.Close()

	content, err := io.ReadAll(io.LimitReader(src, uploadMaxBytes+1))
	if err != nil || int64(len(content)) > uploadMaxBytes {
		return false
	}
	return uploadSampleMatches(declaredMIME, content)
}

func uploadSampleMatches(declaredMIME string, sample []byte) bool {
	trimmed := bytes.TrimSpace(sample)
	if len(trimmed) == 0 {
		return false
	}
	detected := http.DetectContentType(sample)
	switch declaredMIME {
	case "image/png", "image/jpeg", "application/pdf":
		return detected == declaredMIME
	case "image/webp":
		return isWebPSample(sample)
	case "text/plain":
		return strings.HasPrefix(detected, "text/plain")
	case "application/json":
		return json.Valid(trimmed)
	default:
		return false
	}
}

func isWebPSample(sample []byte) bool {
	return len(sample) >= 12 &&
		string(sample[0:4]) == "RIFF" &&
		string(sample[8:12]) == "WEBP"
}

func storageKey(name string) string {
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", name, time.Now().UnixNano())))
	return hex.EncodeToString(hash[:])
}
