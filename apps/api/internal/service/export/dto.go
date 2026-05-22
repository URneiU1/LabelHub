package export

import (
	"encoding/json"

	"labelhub-api/internal/model"
)

// AIReviewToMap / HumanReviewToMap:把 ai_reviews / human_reviews 行映射为前端 / 文件可读的扁平 map。
// 导出以便 Sprint 4 多格式导出器(JSONL / CSV / XLSX)复用同一份字段约定。
func AIReviewToMap(review model.AIReview) map[string]any {
	return map[string]any{
		"verdict":        review.Verdict,
		"overall_score":  review.OverallScore,
		"dimensions":     unwrapJSONPointer(review.Dimensions),
		"reason":         review.Reason,
		"prompt_version": review.PromptVersion,
		"created_at":     review.CreatedAt,
	}
}

func HumanReviewToMap(review model.HumanReview) map[string]any {
	return map[string]any{
		"verdict":     review.Verdict,
		"reason":      review.Reason,
		"stage":       review.Stage,
		"reviewer_id": review.ReviewerID,
		"created_at":  review.CreatedAt,
	}
}

// mustJSON / unwrapJSONPointer / nullStringJSON:
// Phase 3 暂在 export 包内私有一份,Phase 4 抽到 internal/jsonx 公共包后合并。
func mustJSON(raw string) any {
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return raw
	}
	return value
}

func unwrapJSONPointer(raw *string) any {
	if raw == nil {
		return nil
	}
	return mustJSON(*raw)
}

func nullStringJSON(value model.NullString) any {
	if !value.Valid {
		return nil
	}
	return value.String
}
