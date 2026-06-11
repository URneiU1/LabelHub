package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"labelhub-api/internal/httpx"
	"labelhub-api/internal/middleware"
	"labelhub-api/internal/model"
)

const aiReviewQueueDefaultLimit = 50
const aiReviewQueueMaxLimit = 200

// aiReviewRow 是「AI 审核队列」只读视图里的一条:AI 预审本身(状态 / 结论 / 维度分 / 耗时 / 重试 /
// 幂等键 / 原始 Prompt 与阈值)+ 它所属提交 / 任务 / 题目的上下文。把整条 AI Agent 流水线可视化。
type aiReviewRow struct {
	ID             uint64           `json:"id"`
	Status         string           `json:"status"`
	Verdict        *string          `json:"verdict"`
	OverallScore   *float64         `json:"overallScore"`
	Dimensions     json.RawMessage  `json:"dimensions"`
	Reason         model.NullString `json:"reason"`
	PromptVersion  int              `json:"promptVersion"`
	Model          string           `json:"model"`
	PromptTemplate string           `json:"promptTemplate"`
	PassThreshold  float64          `json:"passThreshold"`
	UncertainMin   float64          `json:"uncertainMin"`
	// PromptSnapshot 是本次预审真正发给 LLM 的消息全文(AI 实际看到的 prompt);PromptHash 是其
	// sha256 指纹。PromptDrift 为真表示该预审所用 prompt 配置已不再是任务当前生效版本(配置已变更),
	// ActivePromptVersion 是任务当前生效的 prompt 版本号,供审核员判断结论是否基于旧配置。
	PromptSnapshot      *string          `json:"promptSnapshot"`
	PromptHash          *string          `json:"promptHash"`
	PromptDrift         bool             `json:"promptDrift"`
	ActivePromptVersion int              `json:"activePromptVersion"`
	TokensInput         int              `json:"tokensInput"`
	TokensOutput        int              `json:"tokensOutput"`
	LatencyMs           int              `json:"latencyMs"`
	RetryCount          int              `json:"retryCount"`
	IdempotencyKey      string           `json:"idempotencyKey"`
	ErrorMsg            model.NullString `json:"errorMsg"`
	CreatedAt           time.Time        `json:"createdAt"`
	StartedAt           model.NullTime   `json:"startedAt"`
	FinishedAt          model.NullTime   `json:"finishedAt"`
	SubmissionID        uint64           `json:"submissionId"`
	TaskID              uint64           `json:"taskId"`
	TaskTitle           string           `json:"taskTitle"`
	ItemID              uint64           `json:"itemId"`
}

type aiReviewQueueResponse struct {
	Items      []aiReviewRow `json:"items"`
	NextBefore *uint64       `json:"nextBefore"`
}

// AIReviewQueue 只读列出 AI 预审队列(最新在前,可按 status 过滤、id 游标分页),
// 供「AI 审核队列」视图把流水线展示给审核员。四次查询(预审 + 提交 + 任务 + Prompt 配置)内存合并。
func (h ReviewerHandler) AIReviewQueue(c *gin.Context) {
	limit := aiReviewQueueDefaultLimit
	if raw := c.Query("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= aiReviewQueueMaxLimit {
			limit = n
		}
	}

	// Scope AI reviews to the caller's authorized tasks (admin sees all). Join through
	// submissions so the shared task_reviewers scope (keyed on submissions.task_id) applies —
	// otherwise any reviewer could enumerate every task's AI reviews and prompt config.
	claims, _ := middleware.Claims(c)
	query := h.db.Model(&model.AIReview{}).
		Joins("JOIN submissions ON submissions.id = ai_reviews.submission_id").
		Select("ai_reviews.*").
		Order("ai_reviews.id DESC").Limit(limit)
	var scoped bool
	query, scoped = applyReviewQueueScope(query, claims)
	if !scoped {
		httpx.Error(c, http.StatusForbidden, "FORBIDDEN", "review queue access denied")
		return
	}
	if status := c.Query("status"); status != "" {
		query = query.Where("ai_reviews.status = ?", status)
	}
	if before := c.Query("before"); before != "" {
		if n, err := strconv.ParseUint(before, 10, 64); err == nil {
			query = query.Where("ai_reviews.id < ?", n)
		}
	}

	var reviews []model.AIReview
	if err := query.Find(&reviews).Error; err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list ai reviews")
		return
	}
	if len(reviews) == 0 {
		httpx.OK(c, aiReviewQueueResponse{Items: []aiReviewRow{}})
		return
	}

	submissionIDs := make([]uint64, 0, len(reviews))
	promptIDs := make([]uint64, 0, len(reviews))
	for _, review := range reviews {
		submissionIDs = append(submissionIDs, review.SubmissionID)
		promptIDs = append(promptIDs, review.PromptConfigID)
	}

	var subs []model.Submission
	if err := h.db.Where("id IN ?", submissionIDs).Find(&subs).Error; err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load submissions")
		return
	}
	subByID := make(map[uint64]model.Submission, len(subs))
	taskIDs := make([]uint64, 0, len(subs))
	for _, sub := range subs {
		subByID[sub.ID] = sub
		taskIDs = append(taskIDs, sub.TaskID)
	}

	var tasks []model.Task
	if len(taskIDs) > 0 {
		if err := h.db.Where("id IN ?", taskIDs).Find(&tasks).Error; err != nil {
			httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load tasks")
			return
		}
	}
	taskByID := make(map[uint64]model.Task, len(tasks))
	for _, task := range tasks {
		taskByID[task.ID] = task
		// 把每个任务当前生效的 prompt 配置 id 也纳入待加载集合,用于漂移判定(取其版本号)。
		if task.AIPromptID != nil {
			promptIDs = append(promptIDs, *task.AIPromptID)
		}
	}

	// 去重:review 用的 prompt 与任务当前生效 prompt 常常相同(无漂移),避免 IN 子句重复值。
	promptIDs = uniqueUint64(promptIDs)

	var prompts []model.AIPromptConfig
	if err := h.db.Where("id IN ?", promptIDs).Find(&prompts).Error; err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load prompt configs")
		return
	}
	promptByID := make(map[uint64]model.AIPromptConfig, len(prompts))
	for _, prompt := range prompts {
		promptByID[prompt.ID] = prompt
	}

	rows := make([]aiReviewRow, 0, len(reviews))
	for _, review := range reviews {
		var dims json.RawMessage
		if review.Dimensions != nil {
			dims = json.RawMessage(*review.Dimensions)
		}
		sub := subByID[review.SubmissionID]
		prompt := promptByID[review.PromptConfigID]
		task := taskByID[sub.TaskID]
		promptDrift := false
		activePromptVersion := 0
		if task.AIPromptID != nil {
			promptDrift = *task.AIPromptID != review.PromptConfigID
			if active, ok := promptByID[*task.AIPromptID]; ok {
				activePromptVersion = active.Version
			}
		}
		rows = append(rows, aiReviewRow{
			ID:                  review.ID,
			Status:              review.Status,
			Verdict:             review.Verdict,
			OverallScore:        review.OverallScore,
			Dimensions:          dims,
			Reason:              review.Reason,
			PromptVersion:       review.PromptVersion,
			Model:               prompt.Model,
			PromptTemplate:      prompt.PromptTemplate,
			PassThreshold:       prompt.PassThreshold,
			UncertainMin:        prompt.UncertainMin,
			PromptSnapshot:      review.PromptSnapshot,
			PromptHash:          review.PromptHash,
			PromptDrift:         promptDrift,
			ActivePromptVersion: activePromptVersion,
			TokensInput:         review.TokensInput,
			TokensOutput:        review.TokensOutput,
			LatencyMs:           review.LatencyMS,
			RetryCount:          review.RetryCount,
			IdempotencyKey:      review.IdempotencyKey,
			ErrorMsg:            review.ErrorMsg,
			CreatedAt:           review.CreatedAt,
			StartedAt:           review.StartedAt,
			FinishedAt:          review.FinishedAt,
			SubmissionID:        review.SubmissionID,
			TaskID:              sub.TaskID,
			TaskTitle:           task.Title,
			ItemID:              sub.ItemID,
		})
	}

	var nextBefore *uint64
	if len(reviews) == limit {
		last := reviews[len(reviews)-1].ID
		nextBefore = &last
	}
	httpx.OK(c, aiReviewQueueResponse{Items: rows, NextBefore: nextBefore})
}
