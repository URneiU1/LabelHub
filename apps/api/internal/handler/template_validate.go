package handler

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// allowedWidgets: S2 v1 锁定的核心物料,与前端 renderer/types.ts 保持一致。
var allowedWidgets = map[string]struct{}{
	"ShowItem": {}, "Group": {}, "Tabs": {}, "Input": {}, "TextArea": {}, "Radio": {}, "Tags": {},
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
func validateTemplateSchema(raw string) []ValidationError {
	var parsed struct {
		Fields []any `json:"fields"`
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

	state := newTemplateValidationState()
	state.validateFields("fields", parsed.Fields)
	for _, target := range state.llmTargets {
		if target.allowExternal {
			continue
		}
		if _, ok := state.fieldNames[target.target]; !ok {
			state.errs = append(state.errs, ValidationError{Field: target.path + ".target_field", Message: "target_field must reference an existing field"})
		}
	}
	for _, condition := range state.requiredWhenRefs {
		if _, ok := state.fieldNames[condition.field]; !ok {
			state.errs = append(state.errs, ValidationError{Field: condition.path + ".requiredWhen.field", Message: "requiredWhen.field must reference an existing field"})
		}
	}
	return state.errs
}

type templateValidationState struct {
	errs             []ValidationError
	seenNames        map[string]int
	fieldNames       map[string]struct{}
	fieldCount       int
	llmTargets       []templateLLMTarget
	requiredWhenRefs []templateRequiredWhenRef
}

type templateLLMTarget struct {
	path          string
	target        string
	allowExternal bool
}

type templateRequiredWhenRef struct {
	path  string
	field string
}

func newTemplateValidationState() *templateValidationState {
	return &templateValidationState{
		seenNames:  make(map[string]int),
		fieldNames: make(map[string]struct{}),
	}
}

func (state *templateValidationState) validateFields(path string, rawFields []any) {
	if len(rawFields) == 0 {
		state.errs = append(state.errs, ValidationError{Field: path, Message: "fields must be non-empty"})
		return
	}
	for i, raw := range rawFields {
		fieldPath := fmt.Sprintf("%s[%d]", path, i)
		field, ok := raw.(map[string]any)
		if !ok {
			state.errs = append(state.errs, ValidationError{Field: fieldPath, Message: "field must be an object"})
			continue
		}
		state.validateField(fieldPath, field)
	}
}

func (state *templateValidationState) validateField(path string, f map[string]any) {
	state.fieldCount++
	if state.fieldCount > maxTemplateFields {
		state.errs = append(state.errs, ValidationError{Field: "fields", Message: fmt.Sprintf("fields must be <= %d", maxTemplateFields)})
		return
	}
	nameRaw, _ := f["name"].(string)
	name := strings.TrimSpace(nameRaw)
	if name == "" {
		state.errs = append(state.errs, ValidationError{Field: path + ".name", Message: "name is required"})
	} else if _, dup := state.seenNames[name]; dup {
		state.errs = append(state.errs, ValidationError{Field: path + ".name", Message: "duplicate name " + name})
	} else {
		state.seenNames[name] = state.fieldCount
		state.fieldNames[name] = struct{}{}
	}
	widgetRaw, _ := f["widget"].(string)
	widget := strings.TrimSpace(widgetRaw)
	if _, ok := allowedWidgets[widget]; !ok {
		state.errs = append(state.errs, ValidationError{Field: path + ".widget", Message: "widget not in enum: " + widget})
	}
	state.errs = append(state.errs, validateTemplateFieldLimits(path, f, widget)...)
	if reqRaw, has := f["required"]; has {
		if _, ok := reqRaw.(bool); !ok {
			state.errs = append(state.errs, ValidationError{Field: path + ".required", Message: "required must be bool"})
		}
	}
	if regexRaw, has := f["regex"]; has {
		regex, ok := regexRaw.(string)
		if !ok {
			state.errs = append(state.errs, ValidationError{Field: path + ".regex", Message: "regex must be string"})
		} else if _, err := regexp.Compile(regex); err != nil {
			state.errs = append(state.errs, ValidationError{Field: path + ".regex", Message: "regex must be valid"})
		}
	}
	if requiredWhenRaw, has := f["requiredWhen"]; has {
		condition, ok := requiredWhenRaw.(map[string]any)
		if !ok {
			state.errs = append(state.errs, ValidationError{Field: path + ".requiredWhen", Message: "requiredWhen must be an object"})
		} else {
			state.validateRequiredWhen(path, condition)
		}
	}
	minLen, hasMin, minOK := numericField(f, "minLength")
	maxLen, hasMax, maxOK := numericField(f, "maxLength")
	if hasMin && !minOK {
		state.errs = append(state.errs, ValidationError{Field: path + ".minLength", Message: "minLength must be number"})
	}
	if hasMax && !maxOK {
		state.errs = append(state.errs, ValidationError{Field: path + ".maxLength", Message: "maxLength must be number"})
	}
	if hasMin && minOK && minLen < 0 {
		state.errs = append(state.errs, ValidationError{Field: path + ".minLength", Message: "minLength must be >= 0"})
	}
	if hasMax && maxOK && maxLen < 0 {
		state.errs = append(state.errs, ValidationError{Field: path + ".maxLength", Message: "maxLength must be >= 0"})
	}
	if hasMin && hasMax && minOK && maxOK && minLen > maxLen {
		state.errs = append(state.errs, ValidationError{Field: path + ".maxLength", Message: "min > max"})
	}
	if widget == "LLMTrigger" {
		targetRaw, hasTarget := f["target_field"]
		allowExternal, _ := f["x-allow-external-target"].(bool)
		target, targetOK := targetRaw.(string)
		target = strings.TrimSpace(target)
		if !hasTarget || !targetOK || target == "" {
			if !allowExternal {
				state.errs = append(state.errs, ValidationError{Field: path + ".target_field", Message: "target_field is required"})
			}
		} else {
			state.llmTargets = append(state.llmTargets, templateLLMTarget{path: path, target: target, allowExternal: allowExternal})
		}
	}
	if widget == "Group" {
		children, ok := fieldArrayProp(f, "fields")
		if !ok {
			state.errs = append(state.errs, ValidationError{Field: path + ".fields", Message: "fields must be an array"})
		} else {
			state.validateFields(path+".fields", children)
		}
	}
	if widget == "Tabs" {
		tabs, ok := fieldArrayProp(f, "tabs")
		if !ok {
			state.errs = append(state.errs, ValidationError{Field: path + ".tabs", Message: "tabs must be an array"})
		} else {
			state.validateTabs(path+".tabs", tabs)
		}
	}
}

func (state *templateValidationState) validateRequiredWhen(path string, condition map[string]any) {
	fieldRaw, hasField := condition["field"]
	field, fieldOK := fieldRaw.(string)
	field = strings.TrimSpace(field)
	if !hasField || !fieldOK || field == "" {
		state.errs = append(state.errs, ValidationError{Field: path + ".requiredWhen.field", Message: "field is required"})
	} else {
		state.requiredWhenRefs = append(state.requiredWhenRefs, templateRequiredWhenRef{path: path, field: field})
	}
	if notEmptyRaw, hasNotEmpty := condition["notEmpty"]; hasNotEmpty {
		if _, ok := notEmptyRaw.(bool); !ok {
			state.errs = append(state.errs, ValidationError{Field: path + ".requiredWhen.notEmpty", Message: "notEmpty must be bool"})
		}
	}
	if _, hasEquals := condition["equals"]; !hasEquals {
		notEmpty, _ := condition["notEmpty"].(bool)
		if !notEmpty {
			state.errs = append(state.errs, ValidationError{Field: path + ".requiredWhen", Message: "requiredWhen must set equals or notEmpty=true"})
		}
	}
}

func (state *templateValidationState) validateTabs(path string, rawTabs []any) {
	if len(rawTabs) == 0 {
		state.errs = append(state.errs, ValidationError{Field: path, Message: "tabs must be non-empty"})
		return
	}
	for i, raw := range rawTabs {
		tabPath := fmt.Sprintf("%s[%d]", path, i)
		tab, ok := raw.(map[string]any)
		if !ok {
			state.errs = append(state.errs, ValidationError{Field: tabPath, Message: "tab must be an object"})
			continue
		}
		label, _ := tab["label"].(string)
		if strings.TrimSpace(label) == "" {
			state.errs = append(state.errs, ValidationError{Field: tabPath + ".label", Message: "label is required"})
		} else if len(label) > maxTemplateStringBytes {
			state.errs = append(state.errs, ValidationError{Field: tabPath + ".label", Message: fmt.Sprintf("label must be <= %d bytes", maxTemplateStringBytes)})
		}
		fields, ok := fieldArrayProp(tab, "fields")
		if !ok {
			state.errs = append(state.errs, ValidationError{Field: tabPath + ".fields", Message: "fields must be an array"})
		} else {
			state.validateFields(tabPath+".fields", fields)
		}
	}
}

func fieldArrayProp(raw map[string]any, key string) ([]any, bool) {
	values, ok := raw[key].([]any)
	return values, ok
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
