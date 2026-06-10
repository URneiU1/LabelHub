package submission

import (
	"encoding/json"
	"errors"
	"testing"
)

// decodeAnswer 模拟 handler:answer JSON 解码成 map[string]any(数字成为 float64)。
func decodeAnswer(t *testing.T, raw string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("bad answer json: %v", err)
	}
	return m
}

func TestValidateRequiredAnswer(t *testing.T) {
	// label 必填;comment 仅当 label == "reject" 时必填(requiredWhen);
	// extra 仅当 needExtra == true 时可见(visibleWhen),可见时必填。
	schema := `{"fields":[
		{"name":"label","widget":"Radio","required":true},
		{"name":"comment","widget":"TextArea","requiredWhen":{"field":"label","equals":"reject"}},
		{"name":"needExtra","widget":"Radio"},
		{"name":"extra","widget":"Input","required":true,"visibleWhen":{"field":"needExtra","notEmpty":true}}
	]}`

	cases := []struct {
		name    string
		answer  string
		wantErr bool
	}{
		{"完整答案可提交", `{"label":"pass"}`, false},
		{"缺必填 label 被拒", `{"label":""}`, true},
		{"必填 label 缺键被拒", `{}`, true},
		{"requiredWhen 命中但 comment 缺失被拒", `{"label":"reject"}`, true},
		{"requiredWhen 命中且 comment 已填通过", `{"label":"reject","comment":"理由"}`, false},
		{"requiredWhen 未命中时 comment 可空", `{"label":"pass"}`, false},
		{"不可见字段不强制必填", `{"label":"pass"}`, false},
		{"可见字段(visibleWhen 命中)缺必填被拒", `{"label":"pass","needExtra":"yes"}`, true},
		{"可见字段已填通过", `{"label":"pass","needExtra":"yes","extra":"x"}`, false},
		{"纯空白字符串视为空被拒", `{"label":"   "}`, true},
		{"空数组视为空被拒", `{"label":[]}`, true},
		{"非空数组视为已填通过", `{"label":["a"]}`, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateRequiredAnswer(schema, decodeAnswer(t, tc.answer))
			gotErr := errors.Is(err, ErrIncompleteAnswer)
			if gotErr != tc.wantErr {
				t.Fatalf("answer=%s err=%v wantErr=%v", tc.answer, err, tc.wantErr)
			}
		})
	}
}

func TestValidateRequiredAnswer_NestedGroupAndTabs(t *testing.T) {
	// Group 与 Tabs 内的必填字段必须被递归校验;ShowItem 不产出答案值,跳过。
	schema := `{"fields":[
		{"name":"show","widget":"ShowItem"},
		{"name":"g","widget":"Group","fields":[
			{"name":"inner","widget":"Input","required":true}
		]},
		{"name":"t","widget":"Tabs","tabs":[
			{"fields":[{"name":"tab1field","widget":"Input","required":true}]}
		]}
	]}`

	if err := validateRequiredAnswer(schema, decodeAnswer(t, `{"inner":"a","tab1field":"b"}`)); err != nil {
		t.Fatalf("complete nested answer should pass, got %v", err)
	}
	if err := validateRequiredAnswer(schema, decodeAnswer(t, `{"inner":"a"}`)); !errors.Is(err, ErrIncompleteAnswer) {
		t.Fatalf("missing nested Tabs field should be rejected, got %v", err)
	}
	if err := validateRequiredAnswer(schema, decodeAnswer(t, `{"tab1field":"b"}`)); !errors.Is(err, ErrIncompleteAnswer) {
		t.Fatalf("missing nested Group field should be rejected, got %v", err)
	}
}

func TestValidateRequiredAnswer_HiddenContainersSkipRequiredChildren(t *testing.T) {
	schema := `{"fields":[
		{"name":"show_group","widget":"Radio"},
		{"name":"hidden_group","widget":"Group","visibleWhen":{"field":"show_group","equals":"yes"},"fields":[
			{"name":"inner","widget":"Input","required":true}
		]},
		{"name":"show_tabs","widget":"Radio"},
		{"name":"hidden_tabs","widget":"Tabs","visibleWhen":{"field":"show_tabs","equals":"yes"},"tabs":[
			{"label":"A","fields":[{"name":"tab_inner","widget":"Input","required":true}]}
		]}
	]}`

	if err := validateRequiredAnswer(schema, decodeAnswer(t, `{"show_group":"no","show_tabs":"no"}`)); err != nil {
		t.Fatalf("hidden Group/Tabs children should not block submit, got %v", err)
	}
	if err := validateRequiredAnswer(schema, decodeAnswer(t, `{"show_group":"yes","show_tabs":"no"}`)); !errors.Is(err, ErrIncompleteAnswer) {
		t.Fatalf("visible Group required child should be rejected, got %v", err)
	}
	if err := validateRequiredAnswer(schema, decodeAnswer(t, `{"show_group":"no","show_tabs":"yes"}`)); !errors.Is(err, ErrIncompleteAnswer) {
		t.Fatalf("visible Tabs required child should be rejected, got %v", err)
	}
	if err := validateRequiredAnswer(schema, decodeAnswer(t, `{"show_group":"yes","inner":"ok","show_tabs":"yes","tab_inner":"ok"}`)); err != nil {
		t.Fatalf("filled visible container children should pass, got %v", err)
	}
}

func TestValidateRequiredAnswer_EqualsAcrossTypes(t *testing.T) {
	// requiredWhen.equals 为数字时,answer 中的 float64 也应正确比较。
	schema := `{"fields":[
		{"name":"score","widget":"Input"},
		{"name":"why","widget":"TextArea","requiredWhen":{"field":"score","equals":0}}
	]}`
	if err := validateRequiredAnswer(schema, decodeAnswer(t, `{"score":0}`)); !errors.Is(err, ErrIncompleteAnswer) {
		t.Fatalf("numeric equals should trigger requiredWhen, got %v", err)
	}
	if err := validateRequiredAnswer(schema, decodeAnswer(t, `{"score":1}`)); err != nil {
		t.Fatalf("non-matching numeric equals should not require why, got %v", err)
	}
	if err := validateRequiredAnswer(schema, decodeAnswer(t, `{"score":0,"why":"low"}`)); err != nil {
		t.Fatalf("filled why should pass, got %v", err)
	}
}

func TestValidateRequiredAnswer_TolerantOnBadOrEmptySchema(t *testing.T) {
	// 空 schema / 无法解析的 schema 静默放行,不替模板纠错。
	if err := validateRequiredAnswer("", decodeAnswer(t, `{}`)); err != nil {
		t.Fatalf("empty schema should pass, got %v", err)
	}
	if err := validateRequiredAnswer("not json", decodeAnswer(t, `{}`)); err != nil {
		t.Fatalf("unparsable schema should pass, got %v", err)
	}
}

// asAnswerValidationError 判断 err 是否为值校验违规(minLength/maxLength/regex/customRule)。
func asAnswerValidationError(err error) (*AnswerValidationError, bool) {
	var ave *AnswerValidationError
	if errors.As(err, &ave) {
		return ave, true
	}
	return nil, false
}

func TestValidateAnswer_LengthRules(t *testing.T) {
	// summary: maxLength 5;comment: minLength 3。与 validator.ts 一致:
	// minLength 先 trim 再数,maxLength 不 trim。
	schema := `{"fields":[
		{"name":"summary","widget":"Input","label":"总评","maxLength":5},
		{"name":"comment","widget":"TextArea","label":"评语","minLength":3}
	]}`

	if err := validateRequiredAnswer(schema, decodeAnswer(t, `{"summary":"ok","comment":"good"}`)); err != nil {
		t.Fatalf("valid lengths should pass, got %v", err)
	}
	if err := validateRequiredAnswer(schema, decodeAnswer(t, `{"summary":"toolong","comment":"good"}`)); func() bool { _, ok := asAnswerValidationError(err); return !ok }() {
		t.Fatalf("summary over maxLength should be rejected, got %v", err)
	}
	if err := validateRequiredAnswer(schema, decodeAnswer(t, `{"summary":"ok","comment":"ab"}`)); func() bool { _, ok := asAnswerValidationError(err); return !ok }() {
		t.Fatalf("comment under minLength should be rejected, got %v", err)
	}
	// minLength 对 trim 后计数:"  a  " trim 成 "a" 长度 1 < 3。
	if _, ok := asAnswerValidationError(validateRequiredAnswer(schema, decodeAnswer(t, `{"summary":"ok","comment":"  a  "}`))); !ok {
		t.Fatal("minLength should count trimmed length")
	}
}

func TestValidateAnswer_Regex(t *testing.T) {
	schema := `{"fields":[
		{"name":"code","widget":"Input","label":"编号","regex":"^[A-Z]{3}$"}
	]}`
	if err := validateRequiredAnswer(schema, decodeAnswer(t, `{"code":"ABC"}`)); err != nil {
		t.Fatalf("matching regex should pass, got %v", err)
	}
	if _, ok := asAnswerValidationError(validateRequiredAnswer(schema, decodeAnswer(t, `{"code":"abc"}`))); !ok {
		t.Fatal("non-matching regex should be rejected")
	}
	// regex 只校验非空值;空值(可选字段)跳过 regex。
	if err := validateRequiredAnswer(schema, decodeAnswer(t, `{"code":""}`)); err != nil {
		t.Fatalf("empty optional value skips regex, got %v", err)
	}
}

func TestValidateAnswer_CustomRule(t *testing.T) {
	schema := `{"fields":[
		{"name":"reason","widget":"TextArea","label":"理由","customRule":{"expr":"len(value) >= 5","message":"理由至少 5 个字符"}}
	]}`
	if err := validateRequiredAnswer(schema, decodeAnswer(t, `{"reason":"够长的理由"}`)); err != nil {
		t.Fatalf("satisfying customRule should pass, got %v", err)
	}
	ave, ok := asAnswerValidationError(validateRequiredAnswer(schema, decodeAnswer(t, `{"reason":"短"}`)))
	if !ok {
		t.Fatal("violating customRule should be rejected")
	}
	if ave.Message != "理由至少 5 个字符" {
		t.Fatalf("customRule message should surface to user, got %q", ave.Message)
	}
}

func TestValidateAnswer_HiddenFieldValueRulesSkipped(t *testing.T) {
	// 隐藏字段(visibleWhen 不命中)的值不参与 minLength/regex 校验。
	schema := `{"fields":[
		{"name":"need","widget":"Radio"},
		{"name":"detail","widget":"Input","label":"详情","minLength":10,"visibleWhen":{"field":"need","equals":"yes"}}
	]}`
	// detail 隐藏且其值很短,也不应被拒。
	if err := validateRequiredAnswer(schema, decodeAnswer(t, `{"need":"no","detail":"x"}`)); err != nil {
		t.Fatalf("hidden field value rules must be skipped, got %v", err)
	}
	// detail 可见且太短 → 拒绝。
	if _, ok := asAnswerValidationError(validateRequiredAnswer(schema, decodeAnswer(t, `{"need":"yes","detail":"x"}`))); !ok {
		t.Fatal("visible field too short should be rejected")
	}
}
