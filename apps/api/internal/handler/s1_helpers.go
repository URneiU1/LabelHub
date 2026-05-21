package handler

import (
	"encoding/json"

	"gorm.io/gorm"

	"labelhub-api/internal/model"
)

func hasRole(roles []string, target string) bool {
	for _, role := range roles {
		if role == target {
			return true
		}
	}
	return false
}

func createAuditLog(tx *gorm.DB, entityType string, entityID uint64, from string, to string, actorType string, actorID *uint64, event string, payload map[string]any) error {
	var payloadValue *string
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		value := string(raw)
		payloadValue = &value
	}

	audit := model.AuditLog{
		EntityType: entityType,
		EntityID:   entityID,
		FromState:  nullString(from),
		ToState:    to,
		ActorType:  actorType,
		ActorID:    actorID,
		Event:      event,
		Payload:    payloadValue,
	}
	return tx.Create(&audit).Error
}

func mustJSON(raw string) any {
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return raw
	}
	return value
}

func ptrInt(value int) *int {
	return &value
}

func aiReviewToMap(review model.AIReview) map[string]any {
	return map[string]any{
		"verdict":        review.Verdict,
		"overall_score":  review.OverallScore,
		"dimensions":     unwrapJSONPointer(review.Dimensions),
		"reason":         review.Reason,
		"prompt_version": review.PromptVersion,
		"created_at":     review.CreatedAt,
	}
}

func humanReviewToMap(review model.HumanReview) map[string]any {
	return map[string]any{
		"verdict":     review.Verdict,
		"reason":      review.Reason,
		"stage":       review.Stage,
		"reviewer_id": review.ReviewerID,
		"created_at":  review.CreatedAt,
	}
}

func unwrapJSONPointer(raw *string) any {
	if raw == nil {
		return nil
	}
	return mustJSON(*raw)
}
