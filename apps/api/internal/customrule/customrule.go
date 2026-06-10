// Package customrule 实现 customRule 表达式的服务端解析与求值。
//
// 前端 renderer/validator.ts 用 expr-eval 库在浏览器里执行 customRule.expr;
// 服务端必须对同样的可见答案字段做等价校验,否则绕过前端即可提交违规答案。
// 由于无法在 Go 里嵌入 JS,本包实现一个与 expr-eval「受支持子集」语义对齐的
// 递归下降求值器:模板保存时用 Parse 拒绝子集外语法(保证落库的规则运行时一定可
// 强制),提交时用 Eval 执行。
//
// This package re-implements the subset of expr-eval the Designer realistically
// emits so the backend can enforce customRule with parity to the frontend.
//
// 受支持子集 / Supported subset:
//   - 字面量:数字、单/双引号字符串、true / false
//   - 标识符:答案字段名、value(当前字段值)、answer(整张答案表)
//   - 成员访问:answer.field(一层)
//   - 函数:仅 len(x)
//   - 一元:-、not(! 不支持:expr-eval 里 ! 是阶乘而非逻辑非,会与服务端不一致,用 not)
//   - 算术:+ - * / %(+ 仅数值,两侧非数字得 NaN,对齐 expr-eval 的 Number(a)+Number(b);
//     除零按 IEEE-754 给 ±Inf / NaN,与 JS 一致;字符串拼接非本子集)
//   - 比较:== != < <= > >=
//   - 逻辑:and、or(仅关键字;不支持 && / ||,因为 expr-eval 里 || 是拼接而非或)
//   - 三元:cond ? a : b
//   - 括号
//
// 子集外语法(其它函数、深层成员链、^、in、&&、|| 等)在 Parse 阶段返回错误,
// 由模板保存校验拒绝,运行时永远不会遇到无法强制的规则。
package customrule

import (
	"fmt"
	"math"
	"strconv"
	"unicode/utf16"
)

// Expr 是一条已解析的 customRule 表达式。
type Expr struct {
	root node
}

// Scope 是求值上下文。Value 是当前字段的答案值;Answer 是整张答案表
// (含所有兄弟字段)。两者都对应前端 buildExprScope 注入的 value / answer。
type Scope struct {
	Value  any
	Answer map[string]any
}

// Parse 解析 src 为可求值的表达式。落在受支持子集之外或语法非法时返回错误,
// 调用方(模板保存校验)据此拒绝该规则。
func Parse(src string) (*Expr, error) {
	tokens, err := tokenize(src)
	if err != nil {
		return nil, err
	}
	p := &parser{tokens: tokens}
	root, err := p.parseExpression()
	if err != nil {
		return nil, err
	}
	if !p.atEnd() {
		return nil, fmt.Errorf("customrule: unexpected token %q", p.peek().text)
	}
	return &Expr{root: root}, nil
}

// Eval 在 scope 上求值,按前端 Boolean(...) 的真值规则返回布尔结果。
// 引用缺失字段、类型不匹配等运行期错误返回 error,调用方应将其视为「规则不通过」
// (与 validator.ts 的 catch → false 一致)。
func (e *Expr) Eval(scope Scope) (bool, error) {
	value, err := e.root.eval(&scope)
	if err != nil {
		return false, err
	}
	return truthy(value), nil
}

// ---------------------------------------------------------------------------
// AST
// ---------------------------------------------------------------------------

type node interface {
	eval(s *Scope) (any, error)
}

type literalNode struct{ value any }

func (n literalNode) eval(*Scope) (any, error) { return n.value, nil }

type identNode struct{ name string }

func (n identNode) eval(s *Scope) (any, error) {
	switch n.name {
	case "value":
		return s.Value, nil
	case "answer":
		return s.Answer, nil
	default:
		if s.Answer == nil {
			return nil, nil
		}
		return s.Answer[n.name], nil
	}
}

type memberNode struct {
	object node
	field  string
}

func (n memberNode) eval(s *Scope) (any, error) {
	obj, err := n.object.eval(s)
	if err != nil {
		return nil, err
	}
	m, ok := obj.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("customrule: cannot read field %q of non-object", n.field)
	}
	return m[n.field], nil
}

type lenNode struct{ arg node }

func (n lenNode) eval(s *Scope) (any, error) {
	v, err := n.arg.eval(s)
	if err != nil {
		return nil, err
	}
	switch t := v.(type) {
	case string:
		return float64(utf16Len(t)), nil
	case []any:
		return float64(len(t)), nil
	default:
		return float64(0), nil
	}
}

type unaryNode struct {
	op  string
	arg node
}

func (n unaryNode) eval(s *Scope) (any, error) {
	v, err := n.arg.eval(s)
	if err != nil {
		return nil, err
	}
	switch n.op {
	case "-":
		f, ok := toNumber(v)
		if !ok {
			return nil, fmt.Errorf("customrule: unary - on non-number")
		}
		return -f, nil
	case "not":
		return !truthy(v), nil
	}
	return nil, fmt.Errorf("customrule: unknown unary operator %q", n.op)
}

type binaryNode struct {
	op          string
	left, right node
}

func (n binaryNode) eval(s *Scope) (any, error) {
	// 逻辑运算符短路,且不要求两侧都能成功求值之外的类型。
	switch n.op {
	case "and":
		left, err := n.left.eval(s)
		if err != nil {
			return nil, err
		}
		if !truthy(left) {
			return false, nil
		}
		right, err := n.right.eval(s)
		if err != nil {
			return nil, err
		}
		return truthy(right), nil
	case "or":
		left, err := n.left.eval(s)
		if err != nil {
			return nil, err
		}
		if truthy(left) {
			return true, nil
		}
		right, err := n.right.eval(s)
		if err != nil {
			return nil, err
		}
		return truthy(right), nil
	}

	left, err := n.left.eval(s)
	if err != nil {
		return nil, err
	}
	right, err := n.right.eval(s)
	if err != nil {
		return nil, err
	}

	switch n.op {
	case "==":
		return jsEquals(left, right), nil
	case "!=":
		return !jsEquals(left, right), nil
	case "<", "<=", ">", ">=":
		return compare(n.op, left, right)
	case "+":
		return addNumbers(left, right)
	case "-", "*", "/", "%":
		return arithmetic(n.op, left, right)
	}
	return nil, fmt.Errorf("customrule: unknown operator %q", n.op)
}

type ternaryNode struct {
	cond, then, els node
}

func (n ternaryNode) eval(s *Scope) (any, error) {
	cond, err := n.cond.eval(s)
	if err != nil {
		return nil, err
	}
	if truthy(cond) {
		return n.then.eval(s)
	}
	return n.els.eval(s)
}

// ---------------------------------------------------------------------------
// 运行期辅助 / runtime helpers
// ---------------------------------------------------------------------------

// truthy 复刻 JS Boolean(v):false / 0 / NaN / "" / nil 为假,其余为真。
func truthy(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case float64:
		return t != 0 && !math.IsNaN(t)
	case string:
		return t != ""
	case []any:
		return true
	case map[string]any:
		return true
	default:
		return true
	}
}

func toNumber(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case int:
		return float64(t), true
	default:
		return 0, false
	}
}

func jsEquals(a, b any) bool {
	if an, aok := toNumber(a); aok {
		if bn, bok := toNumber(b); bok {
			return an == bn
		}
		return false
	}
	switch av := a.(type) {
	case string:
		bv, ok := b.(string)
		return ok && av == bv
	case bool:
		bv, ok := b.(bool)
		return ok && av == bv
	case nil:
		return b == nil
	}
	return false
}

func compare(op string, a, b any) (any, error) {
	if an, aok := toNumber(a); aok {
		bn, bok := toNumber(b)
		if !bok {
			return nil, fmt.Errorf("customrule: cannot compare number with non-number")
		}
		return numericCompare(op, an, bn), nil
	}
	as, aok := a.(string)
	bs, bok := b.(string)
	if aok && bok {
		return stringCompare(op, as, bs), nil
	}
	return nil, fmt.Errorf("customrule: cannot compare incompatible types")
}

func numericCompare(op string, a, b float64) bool {
	switch op {
	case "<":
		return a < b
	case "<=":
		return a <= b
	case ">":
		return a > b
	case ">=":
		return a >= b
	}
	return false
}

func stringCompare(op string, a, b string) bool {
	switch op {
	case "<":
		return a < b
	case "<=":
		return a <= b
	case ">":
		return a > b
	case ">=":
		return a >= b
	}
	return false
}

// addNumbers 实现 +,对齐 expr-eval 的 Number(a)+Number(b):两侧都是数字才相加,
// 否则返回 NaN(假值)。不做字符串拼接 —— expr-eval 的 + 对字符串同样走 Number() 得 NaN
// (拼接在 expr-eval 里是 ||,本子集不支持),故这里与前端保持一致地不拼接。
func addNumbers(a, b any) (any, error) {
	an, aok := toNumber(a)
	bn, bok := toNumber(b)
	if !aok || !bok {
		return math.NaN(), nil
	}
	return an + bn, nil
}

func arithmetic(op string, a, b any) (any, error) {
	an, aok := toNumber(a)
	bn, bok := toNumber(b)
	if !aok || !bok {
		return nil, fmt.Errorf("customrule: %s requires numbers", op)
	}
	switch op {
	case "-":
		return an - bn, nil
	case "*":
		return an * bn, nil
	case "/":
		// 除零交给 IEEE-754:x/0 → ±Inf,0/0 → NaN,与前端 expr-eval(JS)一致。
		return an / bn, nil
	case "%":
		// math.Mod(x, 0) → NaN,与 JS x % 0 一致。
		return math.Mod(an, bn), nil
	}
	return nil, fmt.Errorf("customrule: unknown operator %q", op)
}

// utf16Len 返回字符串的 UTF-16 码元数,与 JS String.length 对齐
// (BMP 字符算 1,星辰平面字符算 2),保证 len() 与前端一致。
func utf16Len(s string) int {
	return len(utf16.Encode([]rune(s)))
}

// numericLiteral 解析数字字面量。
func numericLiteral(text string) (any, error) {
	f, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return nil, fmt.Errorf("customrule: invalid number %q", text)
	}
	return f, nil
}
