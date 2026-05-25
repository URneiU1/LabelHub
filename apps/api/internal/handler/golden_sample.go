package handler

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
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
	api.POST("/tasks/:taskId/golden-samples/dry-runs", middleware.RequireRoles("owner", "admin"), h.BatchDryRun)
	api.DELETE("/tasks/:taskId/golden-samples/:sampleId", middleware.RequireRoles("owner", "admin"), h.Delete)
	api.POST("/tasks/:taskId/golden-samples/:sampleId/dry-run", middleware.RequireRoles("owner", "admin"), h.DryRun)
}

type goldenSampleRequest struct {
	Payload         json.RawMessage `json:"payload"`
	ExpectedAnswer  json.RawMessage `json:"expected_answer"`
	ExpectedVerdict string          `json:"expected_verdict"`
	Notes           string          `json:"notes"`
	AIPromptID      *uint64         `json:"ai_prompt_id"`
}

type goldenSampleDryRunRequest struct {
	AIPromptID *uint64 `json:"ai_prompt_id"`
}

type goldenSampleBatchDryRunRequest struct {
	SampleIDs  []uint64 `json:"sample_ids"`
	AIPromptID *uint64  `json:"ai_prompt_id"`
}

type goldenSampleResponse struct {
	ID              uint64           `json:"id"`
	TaskID          uint64           `json:"taskId"`
	AIPromptID      *uint64          `json:"aiPromptId"`
	Payload         json.RawMessage  `json:"payload"`
	ExpectedAnswer  json.RawMessage  `json:"expectedAnswer"`
	ExpectedVerdict string           `json:"expectedVerdict"`
	Notes           model.NullString `json:"notes"`
	CreatedAt       time.Time        `json:"createdAt"`
}

type goldenSampleBatchDryRunResult struct {
	GoldenSampleID  uint64                      `json:"goldenSampleId"`
	Status          string                      `json:"status"`
	DryRunID        *uint64                     `json:"dryRunId,omitempty"`
	Provider        string                      `json:"provider,omitempty"`
	Result          *llmreview.EvaluationResult `json:"result,omitempty"`
	MatchedExpected *bool                       `json:"matchedExpected,omitempty"`
	Error           string                      `json:"error,omitempty"`
}

type goldenSampleBatchDryRunSummary struct {
	Total     int `json:"total"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
}

type resolvedGoldenSamplePrompt struct {
	prompt     model.AIPromptConfig
	dimensions []llmreview.DimensionConfig
}

const maxGoldenSampleBatchDryRunSamples = 20
const goldenSampleBatchDelayEnv = "LLM_BATCH_DRY_RUN_DELAY_MS"

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
	responses, err := goldenSampleResponses(samples)
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to serialize golden samples")
		return
	}
	httpx.OK(c, gin.H{"samples": responses})
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
	payloadJSON, ok := normalizeGoldenSampleJSON(c, req.Payload, "payload")
	if !ok {
		return
	}
	answerJSON, ok := normalizeGoldenSampleJSON(c, req.ExpectedAnswer, "expected_answer")
	if !ok {
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
	response, err := goldenSampleResponseFromModel(sample)
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to serialize golden sample")
		return
	}
	httpx.OK(c, gin.H{"sample": response})
}

func normalizeGoldenSampleJSON(c *gin.Context, raw json.RawMessage, field string) ([]byte, bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		httpx.Error(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "payload and expected_answer are required")
		return nil, false
	}
	canonical, err := canonicalGoldenSampleJSON(trimmed)
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", field+" is invalid")
		return nil, false
	}
	return canonical, true
}

func canonicalGoldenSampleJSON(raw []byte) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, errors.New("multiple JSON values")
		}
		return nil, err
	}
	var buf bytes.Buffer
	if err := writeCanonicalJSON(&buf, value); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeCanonicalJSON(buf *bytes.Buffer, value any) error {
	switch typed := value.(type) {
	case nil:
		buf.WriteString("null")
	case bool:
		if typed {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case string:
		bytes, err := json.Marshal(typed)
		if err != nil {
			return err
		}
		buf.Write(bytes)
	case json.Number:
		number, err := canonicalJSONNumber(typed.String())
		if err != nil {
			return err
		}
		buf.WriteString(number)
	case []any:
		buf.WriteByte('[')
		for index, item := range typed {
			if index > 0 {
				buf.WriteByte(',')
			}
			if err := writeCanonicalJSON(buf, item); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		buf.WriteByte('{')
		for index, key := range keys {
			if index > 0 {
				buf.WriteByte(',')
			}
			keyBytes, err := json.Marshal(key)
			if err != nil {
				return err
			}
			buf.Write(keyBytes)
			buf.WriteByte(':')
			if err := writeCanonicalJSON(buf, typed[key]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
	default:
		return errors.New("unsupported JSON value")
	}
	return nil
}

func canonicalJSONNumber(raw string) (string, error) {
	sign := ""
	if strings.HasPrefix(raw, "-") {
		sign = "-"
		raw = raw[1:]
	}
	exp := 0
	if index := strings.IndexAny(raw, "eE"); index >= 0 {
		parsed, err := strconv.Atoi(raw[index+1:])
		if err != nil {
			return "", err
		}
		exp = parsed
		raw = raw[:index]
	}
	fracLen := 0
	if index := strings.IndexByte(raw, '.'); index >= 0 {
		fracLen = len(raw) - index - 1
		raw = raw[:index] + raw[index+1:]
	}
	scale := fracLen - exp
	if scale < 0 {
		if -scale > int(maxAIPromptBytes) {
			return "", errors.New("number is too large")
		}
		raw += strings.Repeat("0", -scale)
		scale = 0
	}
	raw = strings.TrimLeft(raw, "0")
	if raw == "" {
		return "0", nil
	}
	for scale > 0 && strings.HasSuffix(raw, "0") {
		raw = strings.TrimSuffix(raw, "0")
		scale--
	}
	if len(raw)+scale > int(maxAIPromptBytes) {
		return "", errors.New("number is too large")
	}
	if scale == 0 {
		return sign + raw, nil
	}
	if len(raw) <= scale {
		return sign + "0." + strings.Repeat("0", scale-len(raw)) + raw, nil
	}
	split := len(raw) - scale
	return sign + raw[:split] + "." + raw[split:], nil
}

func goldenSampleResponses(samples []model.GoldenSample) ([]goldenSampleResponse, error) {
	responses := make([]goldenSampleResponse, 0, len(samples))
	for _, sample := range samples {
		response, err := goldenSampleResponseFromModel(sample)
		if err != nil {
			return nil, err
		}
		responses = append(responses, response)
	}
	return responses, nil
}

func goldenSampleResponseFromModel(sample model.GoldenSample) (goldenSampleResponse, error) {
	payload, err := storedGoldenSampleJSON(sample.Payload)
	if err != nil {
		return goldenSampleResponse{}, err
	}
	expectedAnswer, err := storedGoldenSampleJSON(sample.ExpectedAnswer)
	if err != nil {
		return goldenSampleResponse{}, err
	}
	return goldenSampleResponse{
		ID:              sample.ID,
		TaskID:          sample.TaskID,
		AIPromptID:      sample.AIPromptID,
		Payload:         payload,
		ExpectedAnswer:  expectedAnswer,
		ExpectedVerdict: sample.ExpectedVerdict,
		Notes:           sample.Notes,
		CreatedAt:       sample.CreatedAt,
	}, nil
}

func storedGoldenSampleJSON(value string) (json.RawMessage, error) {
	raw := json.RawMessage(value)
	if !json.Valid(raw) {
		return nil, errors.New("stored golden sample JSON is invalid")
	}
	return raw, nil
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

func (h GoldenSampleHandler) BatchDryRun(c *gin.Context) {
	task, ok := loadOwnedTask(h.db, c)
	if !ok {
		return
	}
	var req goldenSampleBatchDryRunRequest
	if !bindLimitedJSON(c, &req, maxAIPromptBytes) {
		return
	}
	sampleIDs, ok := normalizeGoldenSampleBatchIDs(c, req.SampleIDs)
	if !ok {
		return
	}

	var samples []model.GoldenSample
	if err := h.db.Where("task_id = ? AND id IN ?", task.ID, sampleIDs).Find(&samples).Error; err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list golden samples")
		return
	}
	if len(samples) != len(sampleIDs) {
		httpx.Error(c, http.StatusNotFound, "NOT_FOUND", "golden sample not found")
		return
	}
	sampleByID := make(map[uint64]model.GoldenSample, len(samples))
	for _, sample := range samples {
		sampleByID[sample.ID] = sample
	}

	provider, _, providerErr := llmreview.NewProviderFromEnv(nil)
	batchDelay, err := goldenSampleBatchDryRunDelayFromEnv()
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	promptCache := map[uint64]resolvedGoldenSamplePrompt{}
	results := make([]goldenSampleBatchDryRunResult, 0, len(sampleIDs))
	summary := goldenSampleBatchDryRunSummary{Total: len(sampleIDs)}
	for index, sampleID := range sampleIDs {
		if index > 0 && batchDelay > 0 {
			timer := time.NewTimer(batchDelay)
			select {
			case <-c.Request.Context().Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
		result, err := h.runGoldenSampleBatchDryRun(c, task, sampleByID[sampleID], req.AIPromptID, provider, providerErr, promptCache)
		if err != nil {
			httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to record ai dry-run")
			return
		}
		if result.Status == "succeeded" {
			summary.Succeeded++
		} else {
			summary.Failed++
		}
		results = append(results, result)
	}
	httpx.OK(c, gin.H{
		"results": results,
		"summary": summary,
	})
}

func normalizeGoldenSampleBatchIDs(c *gin.Context, sampleIDs []uint64) ([]uint64, bool) {
	if len(sampleIDs) == 0 {
		httpx.Error(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "sample_ids are required")
		return nil, false
	}
	if len(sampleIDs) > maxGoldenSampleBatchDryRunSamples {
		httpx.Error(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "sample_ids must contain at most 20 items")
		return nil, false
	}
	seen := map[uint64]struct{}{}
	for _, sampleID := range sampleIDs {
		if sampleID == 0 {
			httpx.Error(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "sample_ids must be positive integers")
			return nil, false
		}
		if _, exists := seen[sampleID]; exists {
			httpx.Error(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "sample_ids must be unique")
			return nil, false
		}
		seen[sampleID] = struct{}{}
	}
	return sampleIDs, true
}

func goldenSampleBatchDryRunDelayFromEnv() (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(goldenSampleBatchDelayEnv))
	if raw == "" {
		return 0, nil
	}
	ms, err := strconv.Atoi(raw)
	if err != nil || ms < 0 {
		return 0, errors.New(goldenSampleBatchDelayEnv + " must be a non-negative integer")
	}
	return time.Duration(ms) * time.Millisecond, nil
}

func (h GoldenSampleHandler) runGoldenSampleBatchDryRun(c *gin.Context, task model.Task, sample model.GoldenSample, overridePromptID *uint64, provider llmreview.Provider, providerErr error, promptCache map[uint64]resolvedGoldenSamplePrompt) (goldenSampleBatchDryRunResult, error) {
	resolved, message, ok := h.resolveGoldenSamplePrompt(task, sample, overridePromptID, promptCache)
	if !ok {
		return goldenSampleBatchDryRunResult{
			GoldenSampleID: sample.ID,
			Status:         "failed",
			Error:          message,
		}, nil
	}
	if providerErr != nil {
		dryRunID, err := h.recordGoldenSampleDryRun(c, task.ID, resolved.prompt, sample, "failed", nil, providerErr)
		if err != nil {
			return goldenSampleBatchDryRunResult{}, err
		}
		return goldenSampleBatchDryRunResult{
			GoldenSampleID: sample.ID,
			Status:         "failed",
			DryRunID:       &dryRunID,
			Error:          llmreview.SafeErrorMessage(providerErr),
		}, nil
	}

	result, err := provider.Evaluate(c.Request.Context(), llmreview.PromptConfig{
		ID:             resolved.prompt.ID,
		Version:        resolved.prompt.Version,
		PromptTemplate: resolved.prompt.PromptTemplate,
		Dimensions:     resolved.dimensions,
		PassThreshold:  resolved.prompt.PassThreshold,
		UncertainMin:   resolved.prompt.UncertainMin,
		Model:          resolved.prompt.Model,
	}, llmreview.EvaluationInput{
		TaskID:              task.ID,
		PromptConfigID:      resolved.prompt.ID,
		PromptVersion:       resolved.prompt.Version,
		PayloadJSON:         sample.Payload,
		AnswerJSON:          sample.ExpectedAnswer,
		BaselineDescription: task.BaselineDescription.String,
	})
	if err == nil {
		err = llmreview.ValidateThresholdConsistency(result, llmreview.PromptConfig{PassThreshold: resolved.prompt.PassThreshold, UncertainMin: resolved.prompt.UncertainMin})
	}
	if err != nil {
		dryRunID, recordErr := h.recordGoldenSampleDryRun(c, task.ID, resolved.prompt, sample, "failed", nil, err)
		if recordErr != nil {
			return goldenSampleBatchDryRunResult{}, recordErr
		}
		return goldenSampleBatchDryRunResult{
			GoldenSampleID: sample.ID,
			Status:         "failed",
			DryRunID:       &dryRunID,
			Error:          llmreview.SafeErrorMessage(err),
		}, nil
	}

	dryRunID, err := h.recordGoldenSampleDryRun(c, task.ID, resolved.prompt, sample, "succeeded", &result, nil)
	if err != nil {
		return goldenSampleBatchDryRunResult{}, err
	}
	matchedExpected := result.Verdict == sample.ExpectedVerdict
	return goldenSampleBatchDryRunResult{
		GoldenSampleID:  sample.ID,
		Status:          "succeeded",
		DryRunID:        &dryRunID,
		Provider:        result.Provider,
		Result:          &result,
		MatchedExpected: &matchedExpected,
	}, nil
}

func (h GoldenSampleHandler) resolveGoldenSamplePrompt(task model.Task, sample model.GoldenSample, overridePromptID *uint64, promptCache map[uint64]resolvedGoldenSamplePrompt) (resolvedGoldenSamplePrompt, string, bool) {
	promptID := overridePromptID
	if promptID == nil {
		promptID = sample.AIPromptID
	}
	if promptID == nil {
		promptID = task.AIPromptID
	}
	if promptID == nil {
		return resolvedGoldenSamplePrompt{}, "active ai_prompt_id is required", false
	}
	if cached, ok := promptCache[*promptID]; ok {
		return cached, "", true
	}

	var prompt model.AIPromptConfig
	if err := h.db.Where("id = ? AND task_id = ?", *promptID, task.ID).First(&prompt).Error; err != nil {
		return resolvedGoldenSamplePrompt{}, "ai_prompt_id must belong to task", false
	}
	dimensions, err := llmreview.ParseDimensions(prompt.Dimensions)
	if err != nil {
		return resolvedGoldenSamplePrompt{}, "ai prompt dimensions are invalid", false
	}
	if !llmreview.AllowedModelName(prompt.Model) {
		return resolvedGoldenSamplePrompt{}, "ai prompt model is not allowed", false
	}
	resolved := resolvedGoldenSamplePrompt{prompt: prompt, dimensions: dimensions}
	promptCache[*promptID] = resolved
	return resolved, "", true
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
