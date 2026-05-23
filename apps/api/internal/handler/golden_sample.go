package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	mysqlerr "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"

	"labelhub-api/internal/httpx"
	"labelhub-api/internal/middleware"
	"labelhub-api/internal/model"
	"labelhub.local/llmreview"
)

type GoldenSampleHandler struct {
	db *gorm.DB
}

func NewGoldenSampleHandler(db *gorm.DB) GoldenSampleHandler {
	return GoldenSampleHandler{db: db}
}

func (h GoldenSampleHandler) Register(api gin.IRouter) {
	api.GET("/tasks/:taskId/golden-samples", middleware.RequireRoles("owner", "admin"), h.List)
	api.POST("/tasks/:taskId/golden-samples", middleware.RequireRoles("owner", "admin"), h.Create)
	api.DELETE("/tasks/:taskId/golden-samples/:sampleId", middleware.RequireRoles("owner", "admin"), h.Delete)
	api.POST("/tasks/:taskId/golden-samples/:sampleId/dry-run", middleware.RequireRoles("owner", "admin"), h.DryRun)
}

type goldenSampleRequest struct {
	Payload         any     `json:"payload"`
	ExpectedAnswer  any     `json:"expected_answer"`
	ExpectedVerdict string  `json:"expected_verdict"`
	Notes           string  `json:"notes"`
	AIPromptID      *uint64 `json:"ai_prompt_id"`
}

type goldenSampleDryRunRequest struct {
	AIPromptID *uint64 `json:"ai_prompt_id"`
}

func (h GoldenSampleHandler) List(c *gin.Context) {
	task, ok := loadOwnedTask(h.db, c)
	if !ok {
		return
	}
	var samples []model.GoldenSample
	if err := h.db.Where("task_id = ?", task.ID).Order("id DESC").Find(&samples).Error; err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list golden samples")
		return
	}
	httpx.OK(c, gin.H{"samples": samples})
}

func (h GoldenSampleHandler) Create(c *gin.Context) {
	task, ok := loadOwnedTask(h.db, c)
	if !ok {
		return
	}
	var req goldenSampleRequest
	if !bindLimitedJSON(c, &req, maxAIPromptBytes) {
		return
	}
	if req.Payload == nil || req.ExpectedAnswer == nil {
		httpx.Error(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "payload and expected_answer are required")
		return
	}
	if !validGoldenSampleVerdict(req.ExpectedVerdict) {
		httpx.Error(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "expected_verdict must be pass, reject, or uncertain")
		return
	}
	if req.AIPromptID != nil {
		var count int64
		if err := h.db.Model(&model.AIPromptConfig{}).Where("id = ? AND task_id = ?", *req.AIPromptID, task.ID).Count(&count).Error; err != nil {
			httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to validate ai prompt")
			return
		}
		if count == 0 {
			httpx.Error(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "ai_prompt_id must belong to task")
			return
		}
	}
	payloadJSON, err := json.Marshal(req.Payload)
	if err != nil {
		httpx.Error(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "payload is invalid")
		return
	}
	answerJSON, err := json.Marshal(req.ExpectedAnswer)
	if err != nil {
		httpx.Error(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "expected_answer is invalid")
		return
	}
	sample := model.GoldenSample{
		TaskID:          task.ID,
		AIPromptID:      req.AIPromptID,
		Payload:         string(payloadJSON),
		PayloadHash:     goldenSamplePayloadHash(payloadJSON),
		ExpectedAnswer:  string(answerJSON),
		ExpectedVerdict: req.ExpectedVerdict,
		Notes:           model.StringFrom(strings.TrimSpace(req.Notes)),
		CreatedBy:       currentUserID(c),
	}
	if err := h.db.Create(&sample).Error; err != nil {
		if isDuplicateGoldenSamplePayload(err) {
			httpx.Error(c, http.StatusConflict, "CONFLICT", "golden sample payload already exists")
			return
		}
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to create golden sample")
		return
	}
	httpx.OK(c, gin.H{"sample": sample})
}

func (h GoldenSampleHandler) Delete(c *gin.Context) {
	task, ok := loadOwnedTask(h.db, c)
	if !ok {
		return
	}
	sampleID, ok := parseIDParam(c, "sampleId")
	if !ok {
		return
	}
	result := h.db.Where("id = ? AND task_id = ?", sampleID, task.ID).Delete(&model.GoldenSample{})
	if result.Error != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to delete golden sample")
		return
	}
	if result.RowsAffected == 0 {
		httpx.Error(c, http.StatusNotFound, "NOT_FOUND", "golden sample not found")
		return
	}
	httpx.OK(c, gin.H{"deleted": true})
}

func (h GoldenSampleHandler) DryRun(c *gin.Context) {
	task, ok := loadOwnedTask(h.db, c)
	if !ok {
		return
	}
	sampleID, ok := parseIDParam(c, "sampleId")
	if !ok {
		return
	}
	var req goldenSampleDryRunRequest
	if !bindLimitedJSON(c, &req, maxAIPromptBytes) {
		return
	}

	var sample model.GoldenSample
	if err := h.db.Where("id = ? AND task_id = ?", sampleID, task.ID).First(&sample).Error; err != nil {
		httpx.Error(c, http.StatusNotFound, "NOT_FOUND", "golden sample not found")
		return
	}

	promptID := req.AIPromptID
	if promptID == nil {
		promptID = sample.AIPromptID
	}
	if promptID == nil {
		promptID = task.AIPromptID
	}
	if promptID == nil {
		httpx.Error(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "active ai_prompt_id is required")
		return
	}

	var prompt model.AIPromptConfig
	if err := h.db.Where("id = ? AND task_id = ?", *promptID, task.ID).First(&prompt).Error; err != nil {
		httpx.Error(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "ai_prompt_id must belong to task")
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

	provider, _, err := llmreview.NewProviderFromEnv(nil)
	if err != nil {
		if _, recordErr := h.recordGoldenSampleDryRun(c, task.ID, prompt, sample, "failed", nil, err); recordErr != nil {
			httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to record ai dry-run")
			return
		}
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
		PayloadJSON:         sample.Payload,
		AnswerJSON:          sample.ExpectedAnswer,
		BaselineDescription: task.BaselineDescription.String,
	})
	if err != nil {
		if _, recordErr := h.recordGoldenSampleDryRun(c, task.ID, prompt, sample, "failed", nil, err); recordErr != nil {
			httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to record ai dry-run")
			return
		}
		httpx.Error(c, http.StatusBadGateway, "LLM_PROVIDER_ERROR", "ai golden sample dry-run failed")
		return
	}
	if err := llmreview.ValidateThresholdConsistency(result, llmreview.PromptConfig{PassThreshold: prompt.PassThreshold, UncertainMin: prompt.UncertainMin}); err != nil {
		if _, recordErr := h.recordGoldenSampleDryRun(c, task.ID, prompt, sample, "failed", nil, err); recordErr != nil {
			httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to record ai dry-run")
			return
		}
		httpx.Error(c, http.StatusBadGateway, "LLM_PROVIDER_ERROR", "ai golden sample dry-run failed")
		return
	}
	dryRunID, err := h.recordGoldenSampleDryRun(c, task.ID, prompt, sample, "succeeded", &result, nil)
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to record ai dry-run")
		return
	}
	matchedExpected := result.Verdict == sample.ExpectedVerdict
	httpx.OK(c, gin.H{
		"provider":        result.Provider,
		"result":          result,
		"matchedExpected": matchedExpected,
		"dryRunId":        dryRunID,
	})
}

func validGoldenSampleVerdict(verdict string) bool {
	switch verdict {
	case "pass", "reject", "uncertain":
		return true
	default:
		return false
	}
}

func (h GoldenSampleHandler) recordGoldenSampleDryRun(c *gin.Context, taskID uint64, prompt model.AIPromptConfig, sample model.GoldenSample, status string, result *llmreview.EvaluationResult, cause error) (uint64, error) {
	var raw *string
	var actualVerdict model.NullString
	var matchedExpected *bool
	if result != nil {
		bytes, err := json.Marshal(result)
		if err != nil {
			return 0, err
		}
		text := string(bytes)
		raw = &text
		actualVerdict = model.StringFrom(result.Verdict)
		matched := result.Verdict == sample.ExpectedVerdict
		matchedExpected = &matched
	}
	payloadSnapshot := sample.Payload
	expectedAnswerSnapshot := sample.ExpectedAnswer
	run := model.AIDryRun{
		TaskID:                 taskID,
		AIPromptID:             prompt.ID,
		GoldenSampleID:         &sample.ID,
		PromptVersion:          prompt.Version,
		PayloadSnapshot:        &payloadSnapshot,
		ExpectedAnswerSnapshot: &expectedAnswerSnapshot,
		ExpectedVerdict:        model.StringFrom(sample.ExpectedVerdict),
		ActualVerdict:          actualVerdict,
		MatchedExpected:        matchedExpected,
		Status:                 status,
		Result:                 raw,
		CreatedBy:              currentUserID(c),
		FinishedAt:             model.TimeFrom(time.Now().UTC()),
	}
	if cause != nil {
		run.ErrorMsg = model.StringFrom(llmreview.SafeErrorMessage(cause))
	}
	if err := h.db.Create(&run).Error; err != nil {
		return 0, err
	}
	return run.ID, nil
}

func goldenSamplePayloadHash(payloadJSON []byte) string {
	sum := sha256.Sum256(payloadJSON)
	return hex.EncodeToString(sum[:])
}

func isDuplicateGoldenSamplePayload(err error) bool {
	var mysqlErr *mysqlerr.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1062
	}
	return strings.Contains(strings.ToLower(err.Error()), "duplicate") &&
		strings.Contains(err.Error(), "uk_task_payload_hash")
}
