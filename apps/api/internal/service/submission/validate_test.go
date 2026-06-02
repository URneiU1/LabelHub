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
