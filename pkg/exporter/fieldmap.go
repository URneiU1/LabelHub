package exporter

import (
	"encoding/json"
	"fmt"
	"strings"
)

// FieldMap 描述导出的字段选择/重命名 + 是否含审核记录,以及 SFT/DPO 训练格式的字段映射。
// 前端传形如 {"include_reviews":true,"columns":[{"source":"external_id","export":"题目ID"}]}。
type FieldMap struct {
	IncludeReviews bool       `json:"include_reviews"`
	Columns        []Column   `json:"columns"` // 空 == 导出全部默认列
	SFT            *SFTConfig `json:"sft,omitempty"`
	DPO            *DPOConfig `json:"dpo,omitempty"`
}

// SFTConfig 配置 SFT 导出从哪个字段取 user/assistant 文本(点号路径,相对 payload / answer 对象)。
// 字段为空时回退到候选键启发式,保证向后兼容。SystemPrompt 非空则额外加一条 system 消息。
type SFTConfig struct {
	PromptField     string `json:"prompt_field"`     // 相对 payload,如 "prompt" 或 "question.text"
	CompletionField string `json:"completion_field"` // 相对 answer,如 "corrected_answer"
	SystemPrompt    string `json:"system_prompt,omitempty"`
}

// DPOConfig 配置 DPO 导出的字段映射,使其适配任意 A/B 偏好任务(不止 preference_compare)。
// 任一字段为空时回退到 preference_compare 约定(prompt / response_a / response_b / preferred)。
type DPOConfig struct {
	PromptField     string `json:"prompt_field"`      // 相对 payload
	CandidateAField string `json:"candidate_a_field"` // 相对 payload
	CandidateBField string `json:"candidate_b_field"` // 相对 payload
	PreferredField  string `json:"preferred_field"`   // 相对 answer,值为 A/B/tie
}

// resolvePath 按点号路径从一个(可能嵌套的)对象取值;path 为空返回对象本身,中途非 map 返回 nil。
func resolvePath(obj any, path string) any {
	if path == "" {
		return obj
	}
	cur := obj
	for _, key := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[key]
	}
	return cur
}

// ParseFieldMap: nil / 空字符串 / "null" → 零值(全列默认导出);非法 JSON → error。
func ParseFieldMap(raw *string) (FieldMap, error) {
	if raw == nil {
		return FieldMap{}, nil
	}
	trimmed := strings.TrimSpace(*raw)
	if trimmed == "" || trimmed == "null" {
		return FieldMap{}, nil
	}
	var fm FieldMap
	if err := json.Unmarshal([]byte(trimmed), &fm); err != nil {
		return FieldMap{}, fmt.Errorf("exporter: invalid field_map json: %w", err)
	}
	return fm, nil
}

// Apply 把整行按 Columns 选取 + 重命名(键变 Export 名);Columns 空则原样返回;
// 缺失的 source 列填 nil(导出场景容忍缺列)。
func (fm FieldMap) Apply(full Row) Row {
	if len(fm.Columns) == 0 {
		return full
	}
	out := make(Row, len(fm.Columns))
	for i, col := range fm.Columns {
		out[i] = Cell{Key: col.exportName(), Value: pick(full, col.Source)}
	}
	return out
}

// ColumnsOrDefault: Columns 非空直接用(携带 Source→Export 映射,编码时按 Source 取值、按 Export 出表头);
// 为空时从 sample 推导全列顺序作为默认表头(identity 映射)。
func (fm FieldMap) ColumnsOrDefault(sample Row) []Column {
	if len(fm.Columns) > 0 {
		return fm.Columns
	}
	cols := make([]Column, len(sample))
	for i, cell := range sample {
		cols[i] = Column{Source: cell.Key, Export: cell.Key}
	}
	return cols
}
