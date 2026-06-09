package handler

import (
	"bytes"
	"crypto/rand"
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
	"labelhub-api/internal/envutil"
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
	api.GET("/uploads/:id", middleware.RequireRoles("labeler", "owner", "reviewer", "admin"), h.Download)
}

func (h UploadHandler) Upload(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, uploadMaxBytes+4096)
	file, err := c.FormFile("file")
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "file is required")
		return
	}
	taskID, err := strconv.ParseUint(c.PostForm("task_id"), 10, 64)
	if err != nil || taskID == 0 {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "task_id must be a positive integer")
		return
	}

	var task model.Task
	if err := h.db.First(&task, taskID).Error; err != nil {
		httpx.Error(c, http.StatusNotFound, "NOT_FOUND", "task not found")
		return
	}
	claims, ok := middleware.Claims(c)
	if !ok {
		httpx.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing auth context")
		return
	}
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
	uploadDir := envutil.Default("UPLOAD_DIR", defaultUploadBaseDir)
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
		// Reviewer 必须先被指派到该 task(task_reviewers),否则任意 reviewer 都能
		// 往任何处于 human_reviewing 的任务写文件(IDOR-write)。复用 canReviewTask
		// 做绑定校验,再确认该任务确实有进行中的人工审核提交。
		assigned, err := canReviewTask(h.db, claims, task)
		if err != nil {
			return false, err
		}
		if !assigned {
			return false, nil
		}
		var count int64
		err = h.db.Model(&model.Submission{}).
			Where("task_id = ? AND status = ?", task.ID, statemachine.StateHumanReviewing).
			Count(&count).Error
		return count > 0, err
	}
	return false, nil
}

// --- upload-internal helpers(被 s1_test.go 引用,故保留包级符号)---

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

func (h UploadHandler) Download(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil || id == 0 {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid upload id")
		return
	}

	var uploaded model.UploadedFile
	if err := h.db.Where("id = ? AND status <> ?", id, "deleted").First(&uploaded).Error; err != nil {
		httpx.Error(c, http.StatusNotFound, "NOT_FOUND", "upload not found")
		return
	}

	var task model.Task
	if err := h.db.First(&task, uploaded.TaskID).Error; err != nil {
		httpx.Error(c, http.StatusNotFound, "NOT_FOUND", "task not found")
		return
	}

	claims, ok := middleware.Claims(c)
	if !ok {
		httpx.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing auth context")
		return
	}
	allowed, err := h.canDownloadUpload(claims, task, uploaded)
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to check download access")
		return
	}
	if !allowed {
		httpx.Error(c, http.StatusForbidden, "FORBIDDEN", "download is not allowed for this task")
		return
	}

	uploadDir := envutil.Default("UPLOAD_DIR", defaultUploadBaseDir)
	dest := filepath.Join(uploadDir, strconv.FormatUint(uploaded.TaskID, 10), uploaded.StorageKey)
	c.FileAttachment(dest, uploaded.OriginalName)
}

func (h UploadHandler) canDownloadUpload(claims *auth.Claims, task model.Task, uploaded model.UploadedFile) (bool, error) {
	if policy.HasRole(claims, policy.RoleAdmin) || policy.IsTaskOwner(claims, task) || uploaded.CreatedBy == claims.UserID {
		return true, nil
	}
	if !policy.HasRole(claims, policy.RoleReviewer) || uploaded.SubmissionRevisionID == nil {
		return false, nil
	}
	allowed, err := canReviewTask(h.db, claims, task)
	if err != nil || !allowed {
		return false, err
	}
	var count int64
	err = h.db.Model(&model.SubmissionRevision{}).
		Joins("JOIN submissions ON submissions.id = submission_revisions.submission_id").
		Where("submission_revisions.id = ? AND submissions.task_id = ? AND submissions.status IN ?",
			*uploaded.SubmissionRevisionID,
			task.ID,
			// M-05:needs_arbitration 也要可下载,否则仲裁 reviewer 看不到冲突 submission 的证据附件。
			// task 级 reviewer 授权仍由上面的 canReviewTask 把关。
			[]string{statemachine.StateHumanReviewing, statemachine.StateNeedsArbitration, statemachine.StateApproved, statemachine.StateRejected},
		).
		Count(&count).Error
	return count > 0, err
}

func storageKey(name string) string {
	random := make([]byte, 32)
	if _, err := rand.Read(random); err == nil {
		return hex.EncodeToString(random)
	}
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", name, time.Now().UnixNano())))
	return hex.EncodeToString(hash[:])
}
