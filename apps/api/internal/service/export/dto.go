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

// DecodeJSONFallback 把 JSON 字符串解码成 any;解码失败时**原样返回字符串**而非 panic。
// 用于导出场景:DB 里的 payload / answer / dimensions 这些 JSON 列正常都是合法 JSON,
// 但偶尔遇到历史脏数据或半截写入时,导出文件应该带着原始字符串继续走,而不是整批崩。
// 测试见 dto_test.go(原 handler/s1_test.go TestMustJSONFallback 搬到这里)。
func DecodeJSONFallback(raw string) any {
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
	return DecodeJSONFallback(*raw)
}

func nullStringJSON(value model.NullString) any {
	if !value.Valid {
		return nil
	}
	return value.String
}
