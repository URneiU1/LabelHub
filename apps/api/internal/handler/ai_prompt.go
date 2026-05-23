package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	mysqlerr "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"labelhub-api/internal/httpx"
	"labelhub-api/internal/middleware"
	"labelhub-api/internal/model"
	"labelhub.local/llmreview"
)

type AIPromptHandler struct {
	db *gorm.DB
}

func NewAIPromptHandler(db *gorm.DB) AIPromptHandler {
	return AIPromptHandler{db: db}
}

func (h AIPromptHandler) Register(api gin.IRouter) {
	api.GET("/tasks/:taskId/ai-prompts", middleware.RequireRoles("owner", "admin"), h.ListPrompts)
	api.POST("/tasks/:taskId/ai-prompts", middleware.RequireRoles("owner", "admin"), h.CreatePrompt)
	api.POST("/tasks/:taskId/ai-prompts/:promptId/dry-run", middleware.RequireRoles("owner", "admin"), h.DryRun)
	api.POST("/tasks/:taskId/ai-review-settings", middleware.RequireRoles("owner", "admin"), h.UpdateAIReviewSettings)
}

type aiPromptRequest struct {
	PromptTemplate string                      `json:"prompt_template"`
	Dimensions     []llmreview.DimensionConfig `json:"dimensions"`
	PassThreshold  float64                     `json:"pass_threshold"`
	UncertainMin   float64                     `json:"uncertain_min"`
	Model          string                      `json:"model"`
}

type normalizedAIPromptRequest struct {
	PromptTemplate string
	DimensionsJSON string
	PassThreshold  float64
	UncertainMin   float64
	Model          string
}

type aiDryRunRequest struct {
	Payload any `json:"payload"`
	Answer  any `json:"answer"`
}

type aiReviewSettingsRequest struct {
	Enabled *bool `json:"enabled"`
}

func (h AIPromptHandler) ListPrompts(c *gin.Context) {
	task, ok := loadOwnedTask(h.db, c)
	if !ok {
		return
	}
	var prompts []model.AIPromptConfig
	if err := h.db.Where("task_id = ?", task.ID).Order("version DESC").Find(&prompts).Error; err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list ai prompts")
		return
	}
	httpx.OK(c, gin.H{
		"prompts":         prompts,
		"activePromptId":  task.AIPromptID,
		"aiReviewEnabled": task.AIReviewEnabled,
	})
}

func (h AIPromptHandler) CreatePrompt(c *gin.Context) {
	task, ok := loadOwnedTask(h.db, c)
	if !ok {
		return
	}
	var req aiPromptRequest
	if !bindLimitedJSON(c, &req, maxAIPromptBytes) {
		return
	}
	normalized, err := normalizeAIPromptRequest(req)
	if err != nil {
		httpx.Error(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", err.Error())
		return
	}
	claims, _ := middleware.Claims(c)
	prompt, err := h.createPromptVersion(task.ID, claims.UserID, normalized)
	if err != nil {
		if errors.Is(err, errAIPromptVersionConflict) {
			httpx.Error(c, http.StatusConflict, "CONFLICT", "ai prompt version already exists")
			return
		}
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to create ai prompt")
		return
	}
	httpx.OK(c, gin.H{"prompt": prompt, "activePromptId": prompt.ID})
}

func (h AIPromptHandler) DryRun(c *gin.Context) {
	task, ok := loadOwnedTask(h.db, c)
	if !ok {
		return
	}
	promptID, ok := parseIDParam(c, "promptId")
	if !ok {
		return
	}
	var req aiDryRunRequest
	if !bindLimitedJSON(c, &req, maxAIPromptBytes) {
		return
	}
	if req.Payload == nil || req.Answer == nil {
		httpx.Error(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "payload and answer are required")
		return
	}

	var prompt model.AIPromptConfig
	if err := h.db.Where("id = ? AND task_id = ?", promptID, task.ID).First(&prompt).Error; err != nil {
		httpx.Error(c, http.StatusNotFound, "NOT_FOUND", "ai prompt not found")
		return
	}
	dimensions, err := llmreview.ParseDimensions(prompt.Dimensions)
	if err != nil {
		httpx.Error(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "ai prompt dimensions are invalid")
		return
	}
	if !llmreview.AllowedModelName(prompt.Model) {
		httpx.Error(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "ai prompt model is not allowed")
		return
	}
	payloadJSON, err := json.Marshal(req.Payload)
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "payload is invalid")
		return
	}
	answerJSON, err := json.Marshal(req.Answer)
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "answer is invalid")
		return
	}

	provider, _, err := llmreview.NewProviderFromEnv(nil)
	if err != nil {
		_ = h.recordDryRun(c, task.ID, prompt.ID, prompt.Version, "failed", nil, err)
		httpx.Error(c, http.StatusBadGateway, "LLM_PROVIDER_ERROR", "llm provider is not configured")
		return
	}
	result, err := provider.Evaluate(c.Request.Context(), llmreview.PromptConfig{
		ID:             prompt.ID,
		Version:        prompt.Version,
		PromptTemplate: prompt.PromptTemplate,
		Dimensions:     dimensions,
		PassThreshold:  prompt.PassThreshold,
		UncertainMin:   prompt.UncertainMin,
		Model:          prompt.Model,
	}, llmreview.EvaluationInput{
		TaskID:              task.ID,
		PromptConfigID:      prompt.ID,
		PromptVersion:       prompt.Version,
		PayloadJSON:         string(payloadJSON),
		AnswerJSON:          string(answerJSON),
		BaselineDescription: task.BaselineDescription.String,
	})
	if err != nil {
		_ = h.recordDryRun(c, task.ID, prompt.ID, prompt.Version, "failed", nil, err)
		httpx.Error(c, http.StatusBadGateway, "LLM_PROVIDER_ERROR", "ai dry-run failed")
		return
	}
	if err := llmreview.ValidateThresholdConsistency(result, llmreview.PromptConfig{PassThreshold: prompt.PassThreshold, UncertainMin: prompt.UncertainMin}); err != nil {
		_ = h.recordDryRun(c, task.ID, prompt.ID, prompt.Version, "failed", nil, err)
		httpx.Error(c, http.StatusBadGateway, "LLM_PROVIDER_ERROR", "ai dry-run failed")
		return
	}
	if err := h.recordDryRun(c, task.ID, prompt.ID, prompt.Version, "succeeded", &result, nil); err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to record ai dry-run")
		return
	}
	httpx.OK(c, gin.H{
		"provider": result.Provider,
		"result":   result,
	})
}

func (h AIPromptHandler) UpdateAIReviewSettings(c *gin.Context) {
	task, ok := loadOwnedTask(h.db, c)
	if !ok {
		return
	}
	var req aiReviewSettingsRequest
	if !bindLimitedJSON(c, &req, maxAIPromptBytes) {
		return
	}
	if req.Enabled == nil {
		httpx.Error(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "enabled is required")
		return
	}

	updated, err := h.updateAIReviewSettings(task.ID, *req.Enabled)
	if err != nil {
		if errors.Is(err, errAIReviewActivePromptRequired) {
			httpx.Error(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "active ai_prompt_id is required before enabling AI review")
			return
		}
		if errors.Is(err, errAIReviewModelNotAllowed) {
			httpx.Error(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "ai prompt model is not allowed")
			return
		}
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to update ai review settings")
		return
	}
	httpx.OK(c, gin.H{
		"aiReviewEnabled": updated.AIReviewEnabled,
		"activePromptId":  updated.AIPromptID,
	})
}

var errAIPromptVersionConflict = errors.New("ai prompt version conflict")
var errAIReviewActivePromptRequired = errors.New("ai review active prompt required")
var errAIReviewModelNotAllowed = errors.New("ai review model not allowed")

func (h AIPromptHandler) createPromptVersion(taskID uint64, createdBy uint64, req normalizedAIPromptRequest) (model.AIPromptConfig, error) {
	var created model.AIPromptConfig
	err := h.db.Transaction(func(tx *gorm.DB) error {
		var task model.Task
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&task, taskID).Error; err != nil {
			return err
		}
		var maxVersion int
		if err := tx.Model(&model.AIPromptConfig{}).
			Where("task_id = ?", taskID).
			Select("COALESCE(MAX(version),0)").
			Scan(&maxVersion).Error; err != nil {
			return err
		}
		created = model.AIPromptConfig{
			TaskID:         taskID,
			Version:        maxVersion + 1,
			PromptTemplate: req.PromptTemplate,
			Dimensions:     req.DimensionsJSON,
			PassThreshold:  req.PassThreshold,
			UncertainMin:   req.UncertainMin,
			Model:          req.Model,
			CreatedBy:      createdBy,
		}
		if err := tx.Create(&created).Error; err != nil {
			if isDuplicateAIPromptVersion(err) {
				return errAIPromptVersionConflict
			}
			return err
		}
		return tx.Model(&model.Task{}).Where("id = ?", taskID).Update("ai_prompt_id", created.ID).Error
	})
	return created, err
}

func (h AIPromptHandler) updateAIReviewSettings(taskID uint64, enabled bool) (model.Task, error) {
	var updated model.Task
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&updated, taskID).Error; err != nil {
			return err
		}
		if enabled {
			if updated.AIPromptID == nil {
				return errAIReviewActivePromptRequired
			}
			var prompt model.AIPromptConfig
			if err := tx.Where("id = ? AND task_id = ?", *updated.AIPromptID, taskID).First(&prompt).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return errAIReviewActivePromptRequired
				}
				return err
			}
			if !llmreview.AllowedModelName(prompt.Model) {
				return errAIReviewModelNotAllowed
			}
		}
		if err := tx.Model(&model.Task{}).Where("id = ?", taskID).Update("ai_review_enabled", enabled).Error; err != nil {
			return err
		}
		updated.AIReviewEnabled = enabled
		return nil
	})
	return updated, err
}

func (h AIPromptHandler) recordDryRun(c *gin.Context, taskID uint64, promptID uint64, promptVersion int, status string, result *llmreview.EvaluationResult, cause error) error {
	var raw *string
	if result != nil {
		bytes, err := json.Marshal(result)
		if err != nil {
			return err
		}
		text := string(bytes)
		raw = &text
	}
	run := model.AIDryRun{
		TaskID:        taskID,
		AIPromptID:    promptID,
		PromptVersion: promptVersion,
		Status:        status,
		Result:        raw,
		CreatedBy:     currentUserID(c),
		FinishedAt:    model.TimeFrom(time.Now().UTC()),
	}
	if cause != nil {
		run.ErrorMsg = model.StringFrom(llmreview.SafeErrorMessage(cause))
	}
	return h.db.Create(&run).Error
}

func normalizeAIPromptRequest(req aiPromptRequest) (normalizedAIPromptRequest, error) {
	req.PromptTemplate = strings.TrimSpace(req.PromptTemplate)
	if req.PromptTemplate == "" {
		return normalizedAIPromptRequest{}, errors.New("prompt_template is required")
	}
	if len([]byte(req.PromptTemplate)) > maxAIPromptStringBytes {
		return normalizedAIPromptRequest{}, errors.New("prompt_template is too long")
	}
	if err := validatePromptPlaceholders(req.PromptTemplate); err != nil {
		return normalizedAIPromptRequest{}, err
	}
	if len(req.Dimensions) == 0 || len(req.Dimensions) > 20 {
		return normalizedAIPromptRequest{}, errors.New("dimensions must contain 1-20 items")
	}
	seen := map[string]struct{}{}
	for i := range req.Dimensions {
		req.Dimensions[i].Name = strings.TrimSpace(req.Dimensions[i].Name)
		req.Dimensions[i].Description = strings.TrimSpace(req.Dimensions[i].Description)
		if req.Dimensions[i].Name == "" || len([]rune(req.Dimensions[i].Name)) > 64 {
			return normalizedAIPromptRequest{}, errors.New("dimension name is required and must be at most 64 characters")
		}
		if len([]rune(req.Dimensions[i].Description)) > 512 {
			return normalizedAIPromptRequest{}, errors.New("dimension description is too long")
		}
		if req.Dimensions[i].Weight < 0 || req.Dimensions[i].Weight > 1 {
			return normalizedAIPromptRequest{}, errors.New("dimension weight must be between 0 and 1")
		}
		if _, exists := seen[req.Dimensions[i].Name]; exists {
			return normalizedAIPromptRequest{}, errors.New("dimension names must be unique")
		}
		seen[req.Dimensions[i].Name] = struct{}{}
	}
	if req.PassThreshold == 0 {
		req.PassThreshold = 80
	}
	if req.UncertainMin == 0 {
		req.UncertainMin = 60
	}
	if req.PassThreshold < 0 || req.PassThreshold > 100 || req.UncertainMin < 0 || req.UncertainMin > 100 || req.UncertainMin > req.PassThreshold {
		return normalizedAIPromptRequest{}, errors.New("thresholds must be between 0 and 100 and uncertain_min must be <= pass_threshold")
	}
	req.Model = strings.TrimSpace(req.Model)
	if req.Model == "" {
		req.Model = strings.TrimSpace(os.Getenv("LLM_MODEL"))
	}
	if req.Model == "" {
		req.Model = "mock-model"
	}
	if !allowedModelName(req.Model) {
		return normalizedAIPromptRequest{}, errors.New("model is not allowed")
	}
	dimensionsJSON, err := json.Marshal(req.Dimensions)
	if err != nil {
		return normalizedAIPromptRequest{}, err
	}
	return normalizedAIPromptRequest{
		PromptTemplate: req.PromptTemplate,
		DimensionsJSON: string(dimensionsJSON),
		PassThreshold:  req.PassThreshold,
		UncertainMin:   req.UncertainMin,
		Model:          req.Model,
	}, nil
}

var promptPlaceholderPattern = regexp.MustCompile(`\{\{\s*([^}]+?)\s*\}\}`)

func validatePromptPlaceholders(template string) error {
	for _, match := range promptPlaceholderPattern.FindAllStringSubmatch(template, -1) {
		name := strings.TrimSpace(match[1])
		if name == "task.baseline_description" || strings.HasPrefix(name, "payload.") || strings.HasPrefix(name, "answer.") {
			continue
		}
		return errors.New("prompt_template contains unsupported placeholder")
	}
	return nil
}

func allowedModelName(model string) bool {
	return llmreview.AllowedModelName(model)
}

func currentUserID(c *gin.Context) uint64 {
	claims, ok := middleware.Claims(c)
	if !ok {
		return 0
	}
	return claims.UserID
}

func isDuplicateAIPromptVersion(err error) bool {
	var mysqlErr *mysqlerr.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1062
	}
	return strings.Contains(strings.ToLower(err.Error()), "duplicate") &&
		strings.Contains(err.Error(), "uk_task_version")
}
