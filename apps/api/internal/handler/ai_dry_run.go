package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"labelhub-api/internal/httpx"
	"labelhub-api/internal/middleware"
	"labelhub-api/internal/model"
)

type AIDryRunHandler struct {
	db *gorm.DB
}

func NewAIDryRunHandler(db *gorm.DB) AIDryRunHandler {
	return AIDryRunHandler{db: db}
}

func (h AIDryRunHandler) Register(api gin.IRouter) {
	api.GET("/tasks/:taskId/ai-dry-runs", middleware.RequireRoles("owner", "admin"), h.List)
}

type aiDryRunHistoryResponse struct {
	ID                     uint64           `json:"id"`
	TaskID                 uint64           `json:"taskId"`
	AIPromptID             uint64           `json:"aiPromptId"`
	GoldenSampleID         *uint64          `json:"goldenSampleId"`
	PromptVersion          int              `json:"promptVersion"`
	PayloadSnapshot        json.RawMessage  `json:"payloadSnapshot"`
	ExpectedAnswerSnapshot json.RawMessage  `json:"expectedAnswerSnapshot"`
	ExpectedVerdict        model.NullString `json:"expectedVerdict"`
	ActualVerdict          model.NullString `json:"actualVerdict"`
	MatchedExpected        *bool            `json:"matchedExpected"`
	Status                 string           `json:"status"`
	Result                 json.RawMessage  `json:"result"`
	ErrorMsg               model.NullString `json:"errorMsg"`
	CreatedBy              uint64           `json:"createdBy"`
	CreatedAt              time.Time        `json:"createdAt"`
	FinishedAt             model.NullTime   `json:"finishedAt"`
}

func (h AIDryRunHandler) List(c *gin.Context) {
	task, ok := loadOwnedTask(h.db, c)
	if !ok {
		return
	}
	goldenSampleID, ok := optionalPositiveUintQuery(c, "golden_sample_id")
	if !ok {
		return
	}
	limit := httpx.CursorLimit(c)

	query := h.db.Where("task_id = ?", task.ID)
	if goldenSampleID != nil {
		query = query.Where("golden_sample_id = ?", *goldenSampleID)
	}
	var runs []model.AIDryRun
	if err := query.Order("created_at DESC, id DESC").Limit(limit).Find(&runs).Error; err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list ai dry-runs")
		return
	}
	responses, err := aiDryRunHistoryResponses(runs)
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to serialize ai dry-runs")
		return
	}
	httpx.OK(c, gin.H{"dryRuns": responses})
}

func optionalPositiveUintQuery(c *gin.Context, name string) (*uint64, bool) {
	raw := strings.TrimSpace(c.Query(name))
	if raw == "" {
		return nil, true
	}
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || value == 0 {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", name+" must be a positive integer")
		return nil, false
	}
	return &value, true
}

func aiDryRunHistoryResponses(runs []model.AIDryRun) ([]aiDryRunHistoryResponse, error) {
	responses := make([]aiDryRunHistoryResponse, 0, len(runs))
	for _, run := range runs {
		response, err := aiDryRunHistoryResponseFromModel(run)
		if err != nil {
			return nil, err
		}
		responses = append(responses, response)
	}
	return responses, nil
}

func aiDryRunHistoryResponseFromModel(run model.AIDryRun) (aiDryRunHistoryResponse, error) {
	payloadSnapshot, err := optionalStoredJSON(run.PayloadSnapshot)
	if err != nil {
		return aiDryRunHistoryResponse{}, err
	}
	expectedAnswerSnapshot, err := optionalStoredJSON(run.ExpectedAnswerSnapshot)
	if err != nil {
		return aiDryRunHistoryResponse{}, err
	}
	result, err := optionalStoredJSON(run.Result)
	if err != nil {
		return aiDryRunHistoryResponse{}, err
	}
	return aiDryRunHistoryResponse{
		ID:                     run.ID,
		TaskID:                 run.TaskID,
		AIPromptID:             run.AIPromptID,
		GoldenSampleID:         run.GoldenSampleID,
		PromptVersion:          run.PromptVersion,
		PayloadSnapshot:        payloadSnapshot,
		ExpectedAnswerSnapshot: expectedAnswerSnapshot,
		ExpectedVerdict:        run.ExpectedVerdict,
		ActualVerdict:          run.ActualVerdict,
		MatchedExpected:        run.MatchedExpected,
		Status:                 run.Status,
		Result:                 result,
		ErrorMsg:               run.ErrorMsg,
		CreatedBy:              run.CreatedBy,
		CreatedAt:              run.CreatedAt,
		FinishedAt:             run.FinishedAt,
	}, nil
}

func optionalStoredJSON(raw *string) (json.RawMessage, error) {
	if raw == nil {
		return nil, nil
	}
	value := json.RawMessage(*raw)
	if !json.Valid(value) {
		return nil, errInvalidStoredJSON
	}
	return value, nil
}

var errInvalidStoredJSON = errors.New("stored JSON is invalid")
