package exporter

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Cell 是导出行里的一个有序键值。
type Cell struct {
	Key   string
	Value any
}

// Row 是一条扁平导出记录:有序键值切片。用 []Cell 而非 map 以保证列顺序稳定
// (CSV/XLSX 必须列顺序确定;JSON/JSONL 也按该顺序序列化以便 diff)。
type Row []Cell

// Column 描述导出的一列:源字段名 + 导出名(重命名后)。
type Column struct {
	Source string `json:"source"`
	Export string `json:"export"`
}

// exportName 返回列的导出名;Export 为空时回退到 Source。
func (c Column) exportName() string {
	if c.Export != "" {
		return c.Export
	}
	return c.Source
}

// pick 按 key 取值;不存在返回 nil(导出场景容忍缺列)。
func pick(row Row, key string) any {
	for _, cell := range row {
		if cell.Key == key {
			return cell.Value
		}
	}
	return nil
}

// marshalValue 用关闭 HTML 转义的 encoder 序列化单个值(< > & 不转 \uXXXX),
// 数字若来自 json.Number 则原样输出,避免大整数精度坍塌(沿用 S3 教训)。
func marshalValue(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// stringifyCell 把任意值转成单元格字符串(CSV/XLSX 用):标量直出,
// 嵌套结构(map/array)JSON 序列化成字符串。
func stringifyCell(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case json.Number:
		return t.String()
	case bool:
		if t {
			return "true"
		}
		return "false"
	default:
		if b, err := marshalValue(v); err == nil {
			return string(b)
		}
		return fmt.Sprintf("%v", v)
	}
}

// appendRowObject 把一行按 cols 顺序写成 JSON 对象(键=Export 名,值=按 Source 取),
// 保序、关闭 HTML 转义。
func appendRowObject(dst []byte, cols []Column, row Row) ([]byte, error) {
	dst = append(dst, '{')
	for i, col := range cols {
		if i > 0 {
			dst = append(dst, ',')
		}
		key, err := marshalValue(col.exportName())
		if err != nil {
			return nil, err
		}
		dst = append(dst, key...)
		dst = append(dst, ':')
		val, err := marshalValue(pick(row, col.Source))
		if err != nil {
			return nil, err
		}
		dst = append(dst, val...)
	}
	return append(dst, '}'), nil
}
