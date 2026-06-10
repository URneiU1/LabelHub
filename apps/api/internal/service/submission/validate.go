package submission

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf16"

	"gorm.io/gorm"

	"labelhub-api/internal/customrule"
	"labelhub-api/internal/model"
)

// ErrIncompleteAnswer:真正 submit 时,模板里可见(visibleWhen 命中)的 required /
// requiredWhen 字段有空值。draft 保存不触发本校验。
//
// Server-side required-field validation: bypassing the frontend validator must not
// let an incomplete answer through.
var ErrIncompleteAnswer = errors.New("submission: required field is empty")

// AnswerValidationError:可见字段的值违反了模板规则(minLength / maxLength /
// regex / customRule)。Message 是面向用户的提示,已带字段标签,直接回前端展示。
//
// Backend value-rule validation with parity to renderer/validator.ts: a client that
// bypasses the frontend cannot submit an answer that violates length/regex/custom rules.
type AnswerValidationError struct {
	Message string
}

func (e *AnswerValidationError) Error() string { return e.Message }

// 校验用的最小 schema 视图。只取必填判定需要的字段,与前端
// renderer/validator.ts 的 required / requiredWhen / visibleWhen 规则对齐。
// 布局型 widget(ShowItem / Group / Tabs)不直接产出答案值,递归进其子字段。
type validateSchema struct {
	Fields []validateField `json:"fields"`
}

type validateField struct {
	Name         string              `json:"name"`
	Label        string              `json:"label"`
	Widget       string              `json:"widget"`
	Required     bool                `json:"required"`
	RequiredWhen *validateCondition  `json:"requiredWhen"`
	VisibleWhen  *validateCondition  `json:"visibleWhen"`
	MinLength    *int                `json:"minLength"`
	MaxLength    *int                `json:"maxLength"`
	Regex        string              `json:"regex"`
	CustomRule   *validateCustomRule `json:"customRule"`
	Fields       []validateField     `json:"fields"`
	Tabs         []validateTab       `json:"tabs"`
}

// validateCustomRule 复用前端 CustomRule 形状:expr 是 expr-eval 表达式,
// message 是校验不通过时展示给用户的文案。
type validateCustomRule struct {
	Expr    string `json:"expr"`
	Message string `json:"message"`
}

type validateTab struct {
	Fields []validateField `json:"fields"`
}

// validateCondition 复用前端 requiredWhen / visibleWhen 形状:
// 命中 = (有 equals 时 value == equals) 或 (notEmpty=true 且 value 非空)。
type validateCondition struct {
	Field    string          `json:"field"`
	Equals   json.RawMessage `json:"equals"`
	NotEmpty bool            `json:"notEmpty"`
}

// validateSubmitAnswer 在真正 submit 时执行服务端必填校验:
// 按 (task, templateVersion) 取模板 SchemaJSON,解析 answer 后校验可见的必填字段非空。
// 任务无关联模板时跳过(没有 schema 可校验)。
func validateSubmitAnswer(tx *gorm.DB, task model.Task, templateVersion int, answerRaw []byte) error {
	if task.TemplateID == nil {
		return nil
	}
	var template model.TaskTemplate
	if err := tx.Where("task_id = ? AND version = ?", task.ID, templateVersion).First(&template).Error; err != nil {
		// 找不到模板交给既有的模板缺失分支处理,这里不吞错。
		return err
	}
	var answer map[string]any
	if err := json.Unmarshal(answerRaw, &answer); err != nil {
		// answer 在 handler 已 marshal 自合法 map,这里几乎不会触发;真不合法则按空答案校验。
		answer = map[string]any{}
	}
	return validateRequiredAnswer(template.SchemaJSON, answer)
}

// validateRequiredAnswer 解析模板 SchemaJSON,对 answer 做服务端校验,与前端
// renderer/validator.ts 对齐:
//   - 命中 visibleWhen 的字段里,required / requiredWhen 命中者必须非空(否则 ErrIncompleteAnswer)。
//   - 可见字段的值还要满足 minLength / maxLength / regex / customRule(否则 AnswerValidationError)。
//
// 隐藏容器(visibleWhen 不命中的 Group / Tabs)整体跳过,其子字段不参与校验。
// schemaJSON 解析失败时静默放行(模板已在创建时校验过,这里不替模板纠错;且
// 解析错误若硬卡会误伤历史/外部数据)。
func validateRequiredAnswer(schemaJSON string, answer map[string]any) error {
	if strings.TrimSpace(schemaJSON) == "" {
		return nil
	}
	var schema validateSchema
	if err := json.Unmarshal([]byte(schemaJSON), &schema); err != nil {
		return nil
	}
	return validateAnswerFields(schema.Fields, answer)
}

// validateAnswerFields 深度优先遍历字段(展开 Group.fields 与 Tabs.tabs[].fields),
// 返回首个校验违规。隐藏容器整体跳过。
func validateAnswerFields(fields []validateField, answer map[string]any) error {
	for _, field := range fields {
		if !conditionMatches(field.VisibleWhen, answer, true) {
			continue
		}
		switch field.Widget {
		case widgetGroup:
			if err := validateAnswerFields(field.Fields, answer); err != nil {
				return err
			}
			continue
		case widgetTabs:
			for _, tab := range field.Tabs {
				if err := validateAnswerFields(tab.Fields, answer); err != nil {
					return err
				}
			}
			continue
		case "ShowItem":
			// 纯展示物料,无答案值。
			continue
		}
		value := answer[field.Name]
		required := field.Required || conditionMatches(field.RequiredWhen, answer, false)
		if required && answerIsEmpty(value) {
			return ErrIncompleteAnswer
		}
		if err := validateFieldValue(field, value, answer); err != nil {
			return err
		}
	}
	return nil
}

// validateFieldValue 对单个可见字段的值做 minLength / maxLength / regex / customRule
// 校验,规则与 validator.ts 逐条对齐:
//   - minLength:对字符串值,trim 后按 UTF-16 码元计数 < minLength 即违规。
//   - maxLength:对字符串值,不 trim,UTF-16 码元 > maxLength 即违规。
//   - regex:仅对非空字符串值;命中失败即违规。
//   - customRule:仅对非空值;表达式求值为假或出错即违规(对齐 validator.ts 的 catch→false)。
func validateFieldValue(field validateField, value any, answer map[string]any) error {
	label := fieldLabel(field)
	if s, ok := value.(string); ok {
		if field.MinLength != nil && utf16Len(strings.TrimSpace(s)) < *field.MinLength {
			return &AnswerValidationError{Message: fmt.Sprintf("%s must be at least %d characters", label, *field.MinLength)}
		}
		if field.MaxLength != nil && utf16Len(s) > *field.MaxLength {
			return &AnswerValidationError{Message: fmt.Sprintf("%s must be at most %d characters", label, *field.MaxLength)}
		}
		if field.Regex != "" && !answerIsEmpty(s) {
			// 模板保存时已校验 regex 可编译;若仍失败则跳过,不误伤用户。
			if re, err := regexp.Compile(field.Regex); err == nil && !re.MatchString(s) {
				return &AnswerValidationError{Message: fmt.Sprintf("%s format is invalid", label)}
			}
		}
	}
	if field.CustomRule != nil && strings.TrimSpace(field.CustomRule.Expr) != "" && !answerIsEmpty(value) {
		expr, err := customrule.Parse(field.CustomRule.Expr)
		if err != nil {
			// 落库的模板已通过 customrule.Parse 校验;此处解析失败意味着数据被篡改/损坏,按违规处理。
			return &AnswerValidationError{Message: customRuleMessage(field)}
		}
		ok, evalErr := expr.Eval(customrule.Scope{Value: value, Answer: answer})
		if evalErr != nil || !ok {
			return &AnswerValidationError{Message: customRuleMessage(field)}
		}
	}
	return nil
}

func fieldLabel(field validateField) string {
	if strings.TrimSpace(field.Label) != "" {
		return field.Label
	}
	return field.Name
}

func customRuleMessage(field validateField) string {
	if field.CustomRule != nil && strings.TrimSpace(field.CustomRule.Message) != "" {
		return field.CustomRule.Message
	}
	return fmt.Sprintf("%s is invalid", fieldLabel(field))
}

// utf16Len 返回字符串的 UTF-16 码元数,与 JS String.length 对齐,
// 保证 minLength / maxLength 计数与前端一致。
func utf16Len(s string) int {
	return len(utf16.Encode([]rune(s)))
}

// conditionMatches 求值 visibleWhen / requiredWhen。condition 为 nil 时:
// visibleWhen 默认可见(matchNil=true),requiredWhen 默认不触发(matchNil=false)。
func conditionMatches(condition *validateCondition, answer map[string]any, matchNil bool) bool {
	if condition == nil {
		return matchNil
	}
	value := answer[condition.Field]
	if len(condition.Equals) > 0 {
		return jsonEquals(condition.Equals, value)
	}
	return condition.NotEmpty && !answerIsEmpty(value)
}

// jsonEquals 比较模板里的 equals(原始 JSON)与答案值是否相等。
// 把答案值 marshal 回 JSON 再与 equals 的规范化形式逐字节比,
// 规避 float64 / json.Number / bool / string 的跨类型比较坑。
func jsonEquals(equalsRaw json.RawMessage, value any) bool {
	want, err := normalizeJSON([]byte(equalsRaw))
	if err != nil {
		return false
	}
	valueRaw, err := json.Marshal(value)
	if err != nil {
		return false
	}
	got, err := normalizeJSON(valueRaw)
	if err != nil {
		return false
	}
	return want == got
}

func normalizeJSON(raw []byte) (string, error) {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return "", err
	}
	out, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// answerIsEmpty 与前端 isEmpty 对齐:nil / 纯空白字符串 / 空数组为空;
// 其余(数字、bool、非空数组、对象)为非空。
func answerIsEmpty(value any) bool {
	switch v := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(v) == ""
	case []any:
		return len(v) == 0
	default:
		return false
	}
}
