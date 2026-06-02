package submission

import (
	"encoding/json"
	"errors"
	"strings"

	"gorm.io/gorm"

	"labelhub-api/internal/model"
)

// ErrIncompleteAnswer:真正 submit 时,模板里可见(visibleWhen 命中)的 required /
// requiredWhen 字段有空值。draft 保存不触发本校验。
//
// Server-side required-field validation: bypassing the frontend validator must not
// let an incomplete answer through.
var ErrIncompleteAnswer = errors.New("submission: required field is empty")

// 校验用的最小 schema 视图。只取必填判定需要的字段,与前端
// renderer/validator.ts 的 required / requiredWhen / visibleWhen 规则对齐。
// 布局型 widget(ShowItem / Group / Tabs)不直接产出答案值,递归进其子字段。
type validateSchema struct {
	Fields []validateField `json:"fields"`
}

type validateField struct {
	Name         string             `json:"name"`
	Widget       string             `json:"widget"`
	Required     bool               `json:"required"`
	RequiredWhen *validateCondition `json:"requiredWhen"`
	VisibleWhen  *validateCondition `json:"visibleWhen"`
	Fields       []validateField    `json:"fields"`
	Tabs         []validateTab      `json:"tabs"`
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

// validateRequiredAnswer 解析模板 SchemaJSON,对 answer 做服务端必填校验。
// 命中 visibleWhen 的字段里,required 或 requiredWhen 命中者必须非空,否则返回 ErrIncompleteAnswer。
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
	if missingRequired(schema.Fields, answer) {
		return ErrIncompleteAnswer
	}
	return nil
}

// missingRequired 深度优先遍历叶子字段(展开 Group.fields 与 Tabs.tabs[].fields),
// 返回是否存在「可见的必填字段为空」。
func missingRequired(fields []validateField, answer map[string]any) bool {
	for _, field := range fields {
		switch field.Widget {
		case widgetGroup:
			if missingRequired(field.Fields, answer) {
				return true
			}
			continue
		case widgetTabs:
			for _, tab := range field.Tabs {
				if missingRequired(tab.Fields, answer) {
					return true
				}
			}
			continue
		case "ShowItem":
			// 纯展示物料,无答案值。
			continue
		}
		if !conditionMatches(field.VisibleWhen, answer, true) {
			continue
		}
		required := field.Required || conditionMatches(field.RequiredWhen, answer, false)
		if required && answerIsEmpty(answer[field.Name]) {
			return true
		}
	}
	return false
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
