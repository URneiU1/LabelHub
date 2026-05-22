package handler

import (
	"encoding/json"
	"fmt"
	"strings"
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
	if len(parsed.Fields) > maxTemplateFields {
		errs = append(errs, ValidationError{Field: "fields", Message: fmt.Sprintf("fields must be <= %d", maxTemplateFields)})
		return errs
	}
	seenNames := make(map[string]int, len(parsed.Fields))
	fieldNames := make(map[string]struct{}, len(parsed.Fields))
	type llmTarget struct {
		path          string
		target        string
		allowExternal bool
	}
	var llmTargets []llmTarget
	for i, f := range parsed.Fields {
		path := fmt.Sprintf("fields[%d]", i)
		nameRaw, _ := f["name"].(string)
		name := strings.TrimSpace(nameRaw)
		if name == "" {
			errs = append(errs, ValidationError{Field: path + ".name", Message: "name is required"})
		} else if _, dup := seenNames[name]; dup {
			errs = append(errs, ValidationError{Field: path + ".name", Message: "duplicate name " + name})
		} else {
			seenNames[name] = i
			fieldNames[name] = struct{}{}
		}
		widgetRaw, _ := f["widget"].(string)
		widget := strings.TrimSpace(widgetRaw)
		if _, ok := allowedWidgets[widget]; !ok {
			errs = append(errs, ValidationError{Field: path + ".widget", Message: "widget not in enum: " + widget})
		}
		errs = append(errs, validateTemplateFieldLimits(path, f, widget)...)
		if reqRaw, has := f["required"]; has {
			if _, ok := reqRaw.(bool); !ok {
				errs = append(errs, ValidationError{Field: path + ".required", Message: "required must be bool"})
			}
		}
		minLen, hasMin, minOK := numericField(f, "minLength")
		maxLen, hasMax, maxOK := numericField(f, "maxLength")
		if hasMin && !minOK {
			errs = append(errs, ValidationError{Field: path + ".minLength", Message: "minLength must be number"})
		}
		if hasMax && !maxOK {
			errs = append(errs, ValidationError{Field: path + ".maxLength", Message: "maxLength must be number"})
		}
		if hasMin && minOK && minLen < 0 {
			errs = append(errs, ValidationError{Field: path + ".minLength", Message: "minLength must be >= 0"})
		}
		if hasMax && maxOK && maxLen < 0 {
			errs = append(errs, ValidationError{Field: path + ".maxLength", Message: "maxLength must be >= 0"})
		}
		if hasMin && hasMax && minOK && maxOK && minLen > maxLen {
			errs = append(errs, ValidationError{Field: path + ".maxLength", Message: "min > max"})
		}
		if widget == "LLMTrigger" {
			targetRaw, hasTarget := f["target_field"]
			allowExternal, _ := f["x-allow-external-target"].(bool)
			target, targetOK := targetRaw.(string)
			target = strings.TrimSpace(target)
			if !hasTarget || !targetOK || target == "" {
				if !allowExternal {
					errs = append(errs, ValidationError{Field: path + ".target_field", Message: "target_field is required"})
				}
			} else {
				llmTargets = append(llmTargets, llmTarget{path: path, target: target, allowExternal: allowExternal})
			}
		}
	}
	for _, target := range llmTargets {
		if target.allowExternal {
			continue
		}
		if _, ok := fieldNames[target.target]; !ok {
			errs = append(errs, ValidationError{Field: target.path + ".target_field", Message: "target_field must reference an existing field"})
		}
	}
	return errs
}

func validateTemplateFieldLimits(path string, f map[string]any, widget string) []ValidationError {
	var errs []ValidationError
	for key, raw := range f {
		if value, ok := raw.(string); ok && len(value) > maxTemplateStringBytes {
			errs = append(errs, ValidationError{Field: path + "." + key, Message: fmt.Sprintf("%s must be <= %d bytes", key, maxTemplateStringBytes)})
		}
	}
	if rawOptions, hasOptions := f["options"]; hasOptions {
		options, ok := rawOptions.([]any)
		if !ok {
			errs = append(errs, ValidationError{Field: path + ".options", Message: "options must be an array"})
		} else {
			errs = append(errs, validateOptions(path, options)...)
		}
	} else if widget == "Radio" || widget == "Tags" {
		errs = append(errs, ValidationError{Field: path + ".options", Message: "options must be non-empty"})
	}
	if widget == "FileUpload" {
		maxFiles, hasMaxFiles, ok := numericField(f, "maxFiles")
		if hasMaxFiles && (!ok || maxFiles <= 0) {
			errs = append(errs, ValidationError{Field: path + ".maxFiles", Message: "maxFiles must be > 0"})
		}
	}
	return errs
}

func validateOptions(path string, options []any) []ValidationError {
	var errs []ValidationError
	if len(options) == 0 {
		return []ValidationError{{Field: path + ".options", Message: "options must be non-empty"}}
	}
	if len(options) > maxTemplateOptions {
		errs = append(errs, ValidationError{Field: path + ".options", Message: fmt.Sprintf("options must be <= %d", maxTemplateOptions)})
	}
	for i, option := range options {
		switch v := option.(type) {
		case string:
			if strings.TrimSpace(v) == "" {
				errs = append(errs, ValidationError{Field: fmt.Sprintf("%s.options[%d]", path, i), Message: "option must be non-empty string or number"})
			}
		case float64:
		case int:
		default:
			errs = append(errs, ValidationError{Field: fmt.Sprintf("%s.options[%d]", path, i), Message: "option must be string or number"})
		}
	}
	return errs
}

func numericField(f map[string]any, key string) (int, bool, bool) {
	raw, ok := f[key]
	if !ok {
		return 0, false, true
	}
	switch v := raw.(type) {
	case float64:
		if v != float64(int(v)) {
			return 0, true, false
		}
		return int(v), true, true
	case int:
		return v, true, true
	default:
		return 0, true, false
	}
}
