package customrule

import "testing"

func TestParse_RejectsUnsupportedSyntax(t *testing.T) {
	cases := []string{
		"len(value) >= 3 && len(value) <= 10", // && not supported (expr-eval uses `and`)
		"a || b",                              // || is concat in expr-eval, rejected here
		"2 ^ 3",                               // power not supported
		"foo(value)",                          // only len() allowed
		"value in answer.list",                // `in` not supported
		"value >",                             // trailing operator
		"len(value",                           // unbalanced paren
		"'unterminated",                       // unterminated string
		"value ? 1",                           // ternary missing ':'
		"@value",                              // unsupported char
	}
	for _, src := range cases {
		if _, err := Parse(src); err == nil {
			t.Errorf("Parse(%q) = nil error, want rejection", src)
		}
	}
}

func TestParse_AcceptsSupportedSyntax(t *testing.T) {
	cases := []string{
		"len(value) >= 3",
		"len(value) >= 3 and len(value) <= 10",
		"value >= 0 and value <= 100",
		"value != answer.other",
		"value == 'yes' or value == 'no'",
		"not (value == '')",
		"value > 0 ? value < 100 : true",
		"len(value) % 2 == 0",
		"-5 < value",
	}
	for _, src := range cases {
		if _, err := Parse(src); err != nil {
			t.Errorf("Parse(%q) unexpected error: %v", src, err)
		}
	}
}

func evalBool(t *testing.T, src string, scope Scope) bool {
	t.Helper()
	expr, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse(%q) error: %v", src, err)
	}
	ok, err := expr.Eval(scope)
	if err != nil {
		t.Fatalf("Eval(%q) error: %v", src, err)
	}
	return ok
}

func TestEval_LengthRules(t *testing.T) {
	if !evalBool(t, "len(value) >= 3", Scope{Value: "abcd"}) {
		t.Error("len('abcd') >= 3 should be true")
	}
	if evalBool(t, "len(value) >= 3", Scope{Value: "ab"}) {
		t.Error("len('ab') >= 3 should be false")
	}
	// 中文按 UTF-16 码元计数,与 JS String.length 一致(BMP 字符算 1)。
	if !evalBool(t, "len(value) == 2", Scope{Value: "你好"}) {
		t.Error("len('你好') should be 2")
	}
}

func TestEval_NumericBounds(t *testing.T) {
	scope := Scope{Value: float64(50)}
	if !evalBool(t, "value >= 0 and value <= 100", scope) {
		t.Error("50 in [0,100] should be true")
	}
	if evalBool(t, "value >= 0 and value <= 100", Scope{Value: float64(150)}) {
		t.Error("150 in [0,100] should be false")
	}
}

func TestEval_SiblingReference(t *testing.T) {
	scope := Scope{Value: "a", Answer: map[string]any{"other": "b"}}
	if !evalBool(t, "value != answer.other", scope) {
		t.Error("'a' != 'b' should be true")
	}
	// 裸标识符 other 解析为 answer["other"];value == other 时 "value != other" 为假。
	if evalBool(t, "value != other", Scope{Value: "a", Answer: map[string]any{"other": "a"}}) {
		t.Error("'a' != 'a' should be false")
	}
}

func TestEval_StringEqualityAndLogic(t *testing.T) {
	if !evalBool(t, "value == 'yes' or value == 'no'", Scope{Value: "no"}) {
		t.Error("'no' matches yes/no set")
	}
	if evalBool(t, "value == 'yes' or value == 'no'", Scope{Value: "maybe"}) {
		t.Error("'maybe' should not match yes/no set")
	}
}

func TestEval_Ternary(t *testing.T) {
	// 当 value > 10 时要求 < 100;否则恒真。
	if !evalBool(t, "value > 10 ? value < 100 : true", Scope{Value: float64(50)}) {
		t.Error("50 passes (10<50<100)")
	}
	if evalBool(t, "value > 10 ? value < 100 : true", Scope{Value: float64(150)}) {
		t.Error("150 fails (>100)")
	}
	if !evalBool(t, "value > 10 ? value < 100 : true", Scope{Value: float64(5)}) {
		t.Error("5 passes via else branch")
	}
}

func TestEval_Arithmetic(t *testing.T) {
	if !evalBool(t, "len(value) % 2 == 0", Scope{Value: "abcd"}) {
		t.Error("len 4 is even")
	}
	if evalBool(t, "len(value) % 2 == 0", Scope{Value: "abc"}) {
		t.Error("len 3 is odd")
	}
}

func TestEval_ErrorsTreatedByCaller(t *testing.T) {
	// 比较数字与字符串 → 求值错误,调用方应视为规则不通过。
	expr, err := Parse("value > 3")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, evalErr := expr.Eval(Scope{Value: "not-a-number"}); evalErr == nil {
		t.Error("comparing string with number should error")
	}
	// 除零 → 错误。
	expr2, _ := Parse("value / 0 > 1")
	if _, evalErr := expr2.Eval(Scope{Value: float64(5)}); evalErr == nil {
		t.Error("division by zero should error")
	}
}

func TestEval_Truthiness(t *testing.T) {
	// 裸表达式经 Boolean(...) 折算:非空字符串为真,空字符串为假。
	if !evalBool(t, "value", Scope{Value: "x"}) {
		t.Error("non-empty string is truthy")
	}
	if evalBool(t, "value", Scope{Value: ""}) {
		t.Error("empty string is falsy")
	}
	if !evalBool(t, "not value", Scope{Value: float64(0)}) {
		t.Error("not 0 is true")
	}
}
