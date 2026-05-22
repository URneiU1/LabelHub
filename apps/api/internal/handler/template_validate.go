package handler

import (
	"encoding/json"
	"fmt"
)

// allowedWidgets: S2 v1 锁定的 9 个核心物料,与 spec §3.5 widgetPrefixMap 严格一致。
var allowedWidgets = map[string]struct{}{
	"ShowItem": {}, "Input": {}, "TextArea": {}, "Radio": {}, "Tags": {},
	"RichText": {}, "JSONEditor": {}, "FileUpload": {}, "LLMTrigger": {},
}

// ValidationError 是 POST /templates/:id/validate 响应里 errors[] 的单条结构。
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// validateTemplateSchema 实现 spec §2 + §6 的"最小校验":
//   - JSON 解析(失败 → field="$")
//   - fields 必填非空数组
//   - name 在 fields[] 内唯一
//   - widget ∈ allowedWidgets
//   - required 必须是 bool(或缺省)
//   - 同字段 minLength 和 maxLength 都必须是非负整数 且 min <= max(若两者都给)
//
// requiredWhen / regex 等高级校验推到 S3(spec §12)。
func validateTemplateSchema(raw string) []ValidationError {
	var parsed struct {
		Fields []map[string]any `json:"fields"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return []ValidationError{{Field: "$", Message: "invalid JSON: " + err.Error()}}
	}
	var errs []ValidationError
	if len(parsed.Fields) == 0 {
		errs = append(errs, ValidationError{Field: "fields", Message: "fields must be non-empty"})
		return errs
	}
	seenNames := make(map[string]int, len(parsed.Fields))
	for i, f := range parsed.Fields {
		path := fmt.Sprintf("fields[%d]", i)
		name, _ := f["name"].(string)
		if name == "" {
			errs = append(errs, ValidationError{Field: path + ".name", Message: "name is required"})
		} else if _, dup := seenNames[name]; dup {
			errs = append(errs, ValidationError{Field: path + ".name", Message: "duplicate name " + name})
		} else {
			seenNames[name] = i
		}
		widget, _ := f["widget"].(string)
		if _, ok := allowedWidgets[widget]; !ok {
			errs = append(errs, ValidationError{Field: path + ".widget", Message: "widget not in enum: " + widget})
		}
		if reqRaw, has := f["required"]; has {
			if _, ok := reqRaw.(bool); !ok {
				errs = append(errs, ValidationError{Field: path + ".required", Message: "required must be bool"})
			}
		}
		minLen, hasMin := numericField(f, "minLength")
		maxLen, hasMax := numericField(f, "maxLength")
		if hasMin && minLen < 0 {
			errs = append(errs, ValidationError{Field: path + ".minLength", Message: "minLength must be >= 0"})
		}
		if hasMax && maxLen < 0 {
			errs = append(errs, ValidationError{Field: path + ".maxLength", Message: "maxLength must be >= 0"})
		}
		if hasMin && hasMax && minLen > maxLen {
			errs = append(errs, ValidationError{Field: path + ".maxLength", Message: "min > max"})
		}
	}
	return errs
}

func numericField(f map[string]any, key string) (int, bool) {
	raw, ok := f[key]
	if !ok {
		return 0, false
	}
	switch v := raw.(type) {
	case float64:
		return int(v), true
	case int:
		return v, true
	default:
		return 0, false
	}
}
