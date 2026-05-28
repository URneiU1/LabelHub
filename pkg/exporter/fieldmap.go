package exporter

import (
	"encoding/json"
	"fmt"
	"strings"
)

// FieldMap 描述导出的字段选择/重命名 + 是否含审核记录。
// 前端传形如 {"include_reviews":true,"columns":[{"source":"external_id","export":"题目ID"}]}。
type FieldMap struct {
	IncludeReviews bool     `json:"include_reviews"`
	Columns        []Column `json:"columns"` // 空 == 导出全部默认列
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
