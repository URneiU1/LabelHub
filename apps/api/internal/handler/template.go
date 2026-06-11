package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	mysqlerr "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"labelhub-api/internal/httpx"
	"labelhub-api/internal/middleware"
	"labelhub-api/internal/model"
	"labelhub-api/internal/schemadiff"
	"labelhub-api/internal/statemachine"
)

type TemplateHandler struct {
	db *gorm.DB
}

func NewTemplateHandler(db *gorm.DB) TemplateHandler {
	return TemplateHandler{db: db}
}

func (h TemplateHandler) Register(api gin.IRouter) {
	api.GET("/tasks/:taskId/templates", middleware.RequireRoles("owner", "admin"), h.ListTemplates)
	api.GET("/templates/:templateId", middleware.RequireRoles("owner", "admin", "labeler", "reviewer"), h.GetTemplate)
	api.POST("/tasks/:taskId/templates", middleware.RequireRoles("owner", "admin"), h.CreateTemplate)
	api.POST("/tasks/:taskId/templates/validate", middleware.RequireRoles("owner", "admin"), h.ValidateTemplate)
}

type templateSchemaRequest struct {
	Title  string           `json:"title"`
	Layout string           `json:"layout"`
	Fields []map[string]any `json:"fields" binding:"required"`
}

func (h TemplateHandler) ListTemplates(c *gin.Context) {
	task, ok := loadOwnedTask(h.db, c)
	if !ok {
		return
	}

	var templates []model.TaskTemplate
	if err := h.db.Where("task_id = ?", task.ID).Order("version DESC").Find(&templates).Error; err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list templates")
		return
	}
	httpx.OK(c, templates)
}

func (h TemplateHandler) GetTemplate(c *gin.Context) {
	templateID, ok := parseIDParam(c, "templateId")
	if !ok {
		return
	}

	var template model.TaskTemplate
	if err := h.db.First(&template, templateID).Error; err != nil {
		httpx.Error(c, http.StatusNotFound, "NOT_FOUND", "template not found")
		return
	}

	var task model.Task
	if err := h.db.First(&task, template.TaskID).Error; err != nil {
		httpx.Error(c, http.StatusNotFound, "NOT_FOUND", "task not found")
		return
	}
	if !enforceCanReadTask(c, task) {
		return
	}

	latest, err := currentTemplate(h.db, task.ID)
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load latest template")
		return
	}
	httpx.OK(c, gin.H{
		"template":         template,
		"isLatest":         latest.ID == template.ID,
		"latestTemplateId": latest.ID,
	})
}

func (h TemplateHandler) CreateTemplate(c *gin.Context) {
	task, ok := loadOwnedTask(h.db, c)
	if !ok {
		return
	}
	if statemachine.TaskTemplateFrozen(task.Status) {
		httpx.Error(c, http.StatusUnprocessableEntity, "INVALID_STATE", "published task schema is frozen; copy the task to create a new version")
		return
	}

	raw, ok := bindCanonicalTemplateSchema(c, task.Title)
	if !ok {
		return
	}
	if errs := validateTemplateSchema(string(raw)); len(errs) > 0 {
		httpx.ErrorWithDetails(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "template schema is invalid", errs)
		return
	}

	claims, _ := middleware.Claims(c)
	template, err := h.createTemplateVersion(task.ID, claims.UserID, raw)
	if err != nil {
		if errors.Is(err, errTaskPoliciesFrozen) {
			httpx.Error(c, http.StatusUnprocessableEntity, "INVALID_STATE", "published task schema is frozen; copy the task to create a new version")
			return
		}
		if errors.Is(err, errTemplateVersionConflict) {
			httpx.Error(c, http.StatusConflict, "CONFLICT", "template version already exists")
			return
		}
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to create template")
		return
	}
	httpx.OK(c, template)
}

func (h TemplateHandler) ValidateTemplate(c *gin.Context) {
	task, ok := loadOwnedTask(h.db, c)
	if !ok {
		return
	}

	raw, ok := bindCanonicalTemplateSchema(c, task.Title)
	if !ok {
		return
	}
	errs := validateTemplateSchema(string(raw))
	resp := gin.H{
		"valid":  len(errs) == 0,
		"errors": errs,
	}
	// 兼容性:schema 合法且任务已有历史模板版本时,比对最新版本,提示破坏性/警告级结构变更,
	// 让 Owner 在保存新版本前知道改动是否会让历史标注失效或丢数据。
	if len(errs) == 0 {
		var latest model.TaskTemplate
		if err := h.db.Where("task_id = ?", task.ID).Order("version DESC").First(&latest).Error; err == nil {
			if changes, derr := schemadiff.DetectChanges(latest.SchemaJSON, string(raw)); derr == nil {
				resp["compareVersion"] = latest.Version
				resp["changes"] = changes
				resp["compatibility"] = schemadiff.Summarize(changes)
			}
		}
	}
	httpx.OK(c, resp)
}

var (
	errTemplateVersionConflict = errors.New("template version conflict")
	errTaskPoliciesFrozen      = errors.New("published task policies are frozen")
)

func (h TemplateHandler) createTemplateVersion(taskID uint64, createdBy uint64, raw []byte) (model.TaskTemplate, error) {
	var created model.TaskTemplate
	err := h.db.Transaction(func(tx *gorm.DB) error {
		var task model.Task
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&task, taskID).Error; err != nil {
			return err
		}
		if statemachine.TaskTemplateFrozen(task.Status) {
			return errTaskPoliciesFrozen
		}

		var maxVersion int
		if err := tx.Model(&model.TaskTemplate{}).
			Where("task_id = ?", taskID).
			Select("COALESCE(MAX(version),0)").
			Scan(&maxVersion).Error; err != nil {
			return err
		}

		sum := sha256.Sum256(raw)
		created = model.TaskTemplate{
			TaskID:     taskID,
			Version:    maxVersion + 1,
			SchemaJSON: string(raw),
			SchemaHash: hex.EncodeToString(sum[:]),
			CreatedBy:  createdBy,
		}
		if err := tx.Create(&created).Error; err != nil {
			if isDuplicateTemplateVersion(err) {
				return errTemplateVersionConflict
			}
			return err
		}

		return tx.Model(&model.Task{}).Where("id = ?", taskID).Update("template_id", created.ID).Error
	})
	return created, err
}

func bindCanonicalTemplateSchema(c *gin.Context, defaultTitle string) ([]byte, bool) {
	var req map[string]any
	if !bindLimitedJSON(c, &req, maxTemplateSchemaBytes) {
		return nil, false
	}
	layout, _ := req["layout"].(string)
	if layout == "" {
		layout = "single_page"
	}
	if layout != "single_page" {
		httpx.Error(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "layout must be single_page")
		return nil, false
	}
	title, _ := req["title"].(string)
	title = strings.TrimSpace(title)
	if title == "" {
		title = defaultTitle
	}
	if len(title) > maxTemplateStringBytes {
		httpx.Error(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "title is too long")
		return nil, false
	}
	fields, _ := req["fields"].([]any)
	canonical := map[string]any{
		"title":  title,
		"layout": layout,
		"fields": fields,
	}
	if rawExportFields, has := req["export_fields"]; has {
		exportFields, ok := stringSliceProp(rawExportFields)
		if !ok {
			httpx.Error(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "export_fields must be string array")
			return nil, false
		}
		canonical["export_fields"] = exportFields
	}
	for key, value := range req {
		if strings.HasPrefix(key, "x-") {
			canonical[key] = value
		}
	}
	raw, err := json.Marshal(canonical)
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "template schema is invalid")
		return nil, false
	}
	return raw, true
}

func stringSliceProp(raw any) ([]string, bool) {
	values, ok := raw.([]any)
	if !ok {
		return nil, false
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		text, ok := value.(string)
		if !ok {
			return nil, false
		}
		if strings.TrimSpace(text) != "" {
			result = append(result, strings.TrimSpace(text))
		}
	}
	return result, true
}

func isDuplicateTemplateVersion(err error) bool {
	var mysqlErr *mysqlerr.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1062
	}
	return strings.Contains(strings.ToLower(err.Error()), "duplicate") &&
		strings.Contains(err.Error(), "uk_task_version")
}
