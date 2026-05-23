package submission

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"labelhub-api/internal/model"
	"labelhub.local/llmreview"
)

const aiReviewTopic = "ai:review"

type aiReviewPlan struct {
	Enabled        bool
	PromptID       uint64
	PromptVersion  int
	IdempotencyKey string
	Payload        string
}

type aiReviewTaskPayload struct {
	SubmissionID   uint64 `json:"submission_id"`
	RevisionID     uint64 `json:"revision_id"`
	PromptConfigID uint64 `json:"prompt_config_id"`
	PromptVersion  int    `json:"prompt_version"`
	IdempotencyKey string `json:"idempotency_key"`
}

func buildAIReviewPlan(tx *gorm.DB, task model.Task, sub model.Submission, revision model.SubmissionRevision) (aiReviewPlan, error) {
	if !task.AIReviewEnabled {
		return aiReviewPlan{}, nil
	}
	if task.AIPromptID == nil {
		return aiReviewPlan{}, ErrInvalidAIPrompt
	}
	var prompt model.AIPromptConfig
	err := tx.Where("id = ? AND task_id = ?", *task.AIPromptID, task.ID).First(&prompt).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return aiReviewPlan{}, ErrInvalidAIPrompt
		}
		return aiReviewPlan{}, err
	}
	if !llmreview.AllowedModelName(prompt.Model) {
		return aiReviewPlan{}, ErrInvalidAIPrompt
	}
	key := aiReviewIdempotencyKey(sub.ID, revision.ID, prompt.ID, prompt.Version)
	payload, err := json.Marshal(aiReviewTaskPayload{
		SubmissionID:   sub.ID,
		RevisionID:     revision.ID,
		PromptConfigID: prompt.ID,
		PromptVersion:  prompt.Version,
		IdempotencyKey: key,
	})
	if err != nil {
		return aiReviewPlan{}, err
	}
	return aiReviewPlan{
		Enabled:        true,
		PromptID:       prompt.ID,
		PromptVersion:  prompt.Version,
		IdempotencyKey: key,
		Payload:        string(payload),
	}, nil
}

func createPendingAIReview(tx *gorm.DB, sub model.Submission, revision model.SubmissionRevision, plan aiReviewPlan) error {
	if !plan.Enabled {
		return nil
	}
	review := model.AIReview{
		SubmissionID:   sub.ID,
		RevisionID:     revision.ID,
		IdempotencyKey: plan.IdempotencyKey,
		PromptVersion:  plan.PromptVersion,
		Status:         "pending",
		RetryCount:     0,
		TokensInput:    0,
		TokensOutput:   0,
		LatencyMS:      0,
		OverallScore:   nil,
		Dimensions:     nil,
		RawResponse:    nil,
		Verdict:        nil,
		ErrorMsg:       model.NullString{},
		Reason:         model.NullString{},
	}
	return tx.Where("idempotency_key = ?", plan.IdempotencyKey).FirstOrCreate(&review).Error
}

func createAIReviewOutbox(tx *gorm.DB, plan aiReviewPlan) error {
	if !plan.Enabled {
		return nil
	}
	return tx.Create(&model.OutboxEvent{
		Topic:   aiReviewTopic,
		Payload: plan.Payload,
		Status:  "pending",
	}).Error
}

func aiReviewIdempotencyKey(submissionID uint64, revisionID uint64, promptID uint64, promptVersion int) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d:%d:%d:%d", submissionID, revisionID, promptID, promptVersion)))
	return hex.EncodeToString(sum[:])
}
