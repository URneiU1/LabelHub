package export

import (
	"encoding/json"
	"testing"

	"labelhub-api/internal/model"
)

// DecodeJSONFallback:坏 JSON 应原样返回字符串,而非 panic / 静默 nil。
// 导出场景:DB 里的 payload / answer / dimensions 偶尔出现历史脏数据时,
// 导出文件不能整批崩,必须带着原始字符串走完。
func TestDecodeJSONFallback(t *testing.T) {
	if got := DecodeJSONFallback(`{"a":1}`); !mapsEqual(got, map[string]any{"a": float64(1)}) {
		t.Errorf("valid JSON mismatch: %v", got)
	}
	if got := DecodeJSONFallback(`not json`); got != "not json" {
		t.Errorf("invalid JSON should fallback to raw, got %v", got)
	}
	if got := DecodeJSONFallback(`[1,2,3]`); !sliceLen(got, 3) {
		t.Errorf("array JSON unparsed: %v", got)
	}
}

func TestAIReviewToMapIncludesPromptConfigID(t *testing.T) {
	got := AIReviewToMap(model.AIReview{
		PromptConfigID: 33,
		PromptVersion:  3,
	})
	if got["prompt_config_id"] != uint64(33) {
		t.Fatalf("prompt_config_id = %v", got["prompt_config_id"])
	}
	if got["prompt_version"] != 3 {
		t.Fatalf("prompt_version = %v", got["prompt_version"])
	}
}

func mapsEqual(got any, want map[string]any) bool {
	gotMap, ok := got.(map[string]any)
	if !ok {
		return false
	}
	a, _ := json.Marshal(gotMap)
	b, _ := json.Marshal(want)
	return string(a) == string(b)
}

func sliceLen(value any, want int) bool {
	arr, ok := value.([]any)
	if !ok {
		return false
	}
	return len(arr) == want
}
