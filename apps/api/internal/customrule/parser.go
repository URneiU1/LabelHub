package customrule

import (
	"fmt"
	"strings"
	"unicode"
)

// tokenType 区分词法单元类别。
type tokenType int

const (
	tokenNumber tokenType = iota
	tokenString
	tokenIdent
	tokenOp
)

type token struct {
	typ  tokenType
	text string
	// value 仅对 number / string 字面量有效。
	value any
}

// tokenize 把 src 切成词法单元。无法识别的字符返回错误,使该表达式在
// Parse 阶段被拒绝(模板保存校验据此拒绝规则)。
func tokenize(src string) ([]token, error) {
	var tokens []token
	runes := []rune(src)
	i := 0
	for i < len(runes) {
		ch := runes[i]
		switch {
		case unicode.IsSpace(ch):
			i++
		case ch == '\'' || ch == '"':
			str, next, err := scanString(runes, i)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, token{typ: tokenString, text: str, value: str})
			i = next
		case unicode.IsDigit(ch) || (ch == '.' && i+1 < len(runes) && unicode.IsDigit(runes[i+1])):
			num, next := scanNumber(runes, i)
			value, err := numericLiteral(num)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, token{typ: tokenNumber, text: num, value: value})
			i = next
		case isIdentStart(ch):
			ident, next := scanIdent(runes, i)
			tokens = append(tokens, token{typ: tokenIdent, text: ident})
			i = next
		default:
			op, next, err := scanOperator(runes, i)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, token{typ: tokenOp, text: op})
			i = next
		}
	}
	return tokens, nil
}

func scanString(runes []rune, start int) (string, int, error) {
	quote := runes[start]
	var sb strings.Builder
	i := start + 1
	for i < len(runes) {
		ch := runes[i]
		if ch == '\\' {
			if i+1 >= len(runes) {
				return "", 0, fmt.Errorf("customrule: dangling escape in string literal")
			}
			next := runes[i+1]
			switch next {
			case 'n':
				sb.WriteRune('\n')
			case 't':
				sb.WriteRune('\t')
			case '\\', '\'', '"':
				sb.WriteRune(next)
			default:
				sb.WriteRune(next)
			}
			i += 2
			continue
		}
		if ch == quote {
			return sb.String(), i + 1, nil
		}
		sb.WriteRune(ch)
		i++
	}
	return "", 0, fmt.Errorf("customrule: unterminated string literal")
}

func scanNumber(runes []rune, start int) (string, int) {
	i := start
	seenDot := false
	for i < len(runes) {
		ch := runes[i]
		if unicode.IsDigit(ch) {
			i++
			continue
		}
		if ch == '.' && !seenDot {
			seenDot = true
			i++
			continue
		}
		break
	}
	return string(runes[start:i]), i
}

func scanIdent(runes []rune, start int) (string, int) {
	i := start
	for i < len(runes) && isIdentPart(runes[i]) {
		i++
	}
	return string(runes[start:i]), i
}

// scanOperator 识别多字符与单字符运算符。子集外的符号(^、&、|、~ 等)直接报错。
func scanOperator(runes []rune, start int) (string, int, error) {
	two := ""
	if start+1 < len(runes) {
		two = string(runes[start : start+2])
	}
	switch two {
	case "==", "!=", "<=", ">=":
		return two, start + 2, nil
	}
	switch runes[start] {
	case '(', ')', '.', ',', '?', ':', '<', '>', '+', '-', '*', '/', '%':
		return string(runes[start]), start + 1, nil
	case '!':
		// != 已由上面的双字符分支处理;落到这里只可能是前缀 ! ,
		// 而 expr-eval 里前缀 ! 是阶乘(数值)而非逻辑非,会与服务端逻辑非语义不一致,故拒绝。
		return "", 0, fmt.Errorf("customrule: '!' is not supported, use 'not' instead")
	}
	return "", 0, fmt.Errorf("customrule: unsupported operator %q", string(runes[start]))
}

func isIdentStart(ch rune) bool {
	return ch == '_' || unicode.IsLetter(ch)
}

func isIdentPart(ch rune) bool {
	return ch == '_' || unicode.IsLetter(ch) || unicode.IsDigit(ch)
}

// ---------------------------------------------------------------------------
// 递归下降解析器 / recursive-descent parser
// ---------------------------------------------------------------------------

type parser struct {
	tokens []token
	pos    int
}

func (p *parser) atEnd() bool    { return p.pos >= len(p.tokens) }
func (p *parser) peek() token    { return p.tokens[p.pos] }
func (p *parser) advance() token { t := p.tokens[p.pos]; p.pos++; return t }
func (p *parser) hasMore() bool  { return p.pos < len(p.tokens) }

func (p *parser) matchOp(ops ...string) (string, bool) {
	if !p.hasMore() {
		return "", false
	}
	t := p.peek()
	if t.typ != tokenOp {
		return "", false
	}
	for _, op := range ops {
		if t.text == op {
			p.pos++
			return op, true
		}
	}
	return "", false
}

func (p *parser) matchKeyword(words ...string) (string, bool) {
	if !p.hasMore() {
		return "", false
	}
	t := p.peek()
	if t.typ != tokenIdent {
		return "", false
	}
	for _, w := range words {
		if t.text == w {
			p.pos++
			return w, true
		}
	}
	return "", false
}

// 优先级(低→高):ternary → or → and → equality → comparison → additive → multiplicative → unary → primary

func (p *parser) parseExpression() (node, error) {
	return p.parseTernary()
}

func (p *parser) parseTernary() (node, error) {
	cond, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if _, ok := p.matchOp("?"); !ok {
		return cond, nil
	}
	then, err := p.parseExpression()
	if err != nil {
		return nil, err
	}
	if _, ok := p.matchOp(":"); !ok {
		return nil, fmt.Errorf("customrule: expected ':' in conditional")
	}
	els, err := p.parseExpression()
	if err != nil {
		return nil, err
	}
	return ternaryNode{cond: cond, then: then, els: els}, nil
}

func (p *parser) parseOr() (node, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for {
		if _, ok := p.matchKeyword("or"); !ok {
			return left, nil
		}
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = binaryNode{op: "or", left: left, right: right}
	}
}

func (p *parser) parseAnd() (node, error) {
	left, err := p.parseEquality()
	if err != nil {
		return nil, err
	}
	for {
		if _, ok := p.matchKeyword("and"); !ok {
			return left, nil
		}
		right, err := p.parseEquality()
		if err != nil {
			return nil, err
		}
		left = binaryNode{op: "and", left: left, right: right}
	}
}

func (p *parser) parseEquality() (node, error) {
	left, err := p.parseComparison()
	if err != nil {
		return nil, err
	}
	for {
		op, ok := p.matchOp("==", "!=")
		if !ok {
			return left, nil
		}
		right, err := p.parseComparison()
		if err != nil {
			return nil, err
		}
		left = binaryNode{op: op, left: left, right: right}
	}
}

func (p *parser) parseComparison() (node, error) {
	left, err := p.parseAdditive()
	if err != nil {
		return nil, err
	}
	for {
		op, ok := p.matchOp("<", "<=", ">", ">=")
		if !ok {
			return left, nil
		}
		right, err := p.parseAdditive()
		if err != nil {
			return nil, err
		}
		left = binaryNode{op: op, left: left, right: right}
	}
}

func (p *parser) parseAdditive() (node, error) {
	left, err := p.parseMultiplicative()
	if err != nil {
		return nil, err
	}
	for {
		op, ok := p.matchOp("+", "-")
		if !ok {
			return left, nil
		}
		right, err := p.parseMultiplicative()
		if err != nil {
			return nil, err
		}
		left = binaryNode{op: op, left: left, right: right}
	}
}

func (p *parser) parseMultiplicative() (node, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for {
		op, ok := p.matchOp("*", "/", "%")
		if !ok {
			return left, nil
		}
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		left = binaryNode{op: op, left: left, right: right}
	}
}

func (p *parser) parseUnary() (node, error) {
	if op, ok := p.matchOp("-"); ok {
		arg, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return unaryNode{op: op, arg: arg}, nil
	}
	if _, ok := p.matchKeyword("not"); ok {
		arg, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return unaryNode{op: "not", arg: arg}, nil
	}
	return p.parsePrimary()
}

func (p *parser) parsePrimary() (node, error) {
	if !p.hasMore() {
		return nil, fmt.Errorf("customrule: unexpected end of expression")
	}
	t := p.peek()
	switch t.typ {
	case tokenNumber:
		p.advance()
		return literalNode{value: t.value}, nil
	case tokenString:
		p.advance()
		return literalNode{value: t.value}, nil
	case tokenIdent:
		return p.parseIdentExpr()
	case tokenOp:
		if t.text == "(" {
			p.advance()
			inner, err := p.parseExpression()
			if err != nil {
				return nil, err
			}
			if _, ok := p.matchOp(")"); !ok {
				return nil, fmt.Errorf("customrule: expected ')'")
			}
			return inner, nil
		}
	}
	return nil, fmt.Errorf("customrule: unexpected token %q", t.text)
}

// parseIdentExpr 处理标识符:关键字字面量(true/false)、len(...) 调用、
// 成员访问 ident.field,以及裸标识符。其它函数调用(ident 后跟 '(' 且非 len)被拒绝。
func (p *parser) parseIdentExpr() (node, error) {
	t := p.advance()
	switch t.text {
	case "true":
		return literalNode{value: true}, nil
	case "false":
		return literalNode{value: false}, nil
	case "null":
		return literalNode{value: nil}, nil
	}

	// 函数调用 / function call.
	if p.hasMore() && p.peek().typ == tokenOp && p.peek().text == "(" {
		if t.text != "len" {
			return nil, fmt.Errorf("customrule: unsupported function %q", t.text)
		}
		p.advance() // consume '('
		arg, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		if _, ok := p.matchOp(")"); !ok {
			return nil, fmt.Errorf("customrule: expected ')' after len argument")
		}
		return lenNode{arg: arg}, nil
	}

	var current node = identNode{name: t.text}
	// 成员访问(仅一层深度由调用链自然限制;链式 a.b.c 会构造嵌套 memberNode)。
	for {
		if _, ok := p.matchOp("."); !ok {
			break
		}
		if !p.hasMore() || p.peek().typ != tokenIdent {
			return nil, fmt.Errorf("customrule: expected field name after '.'")
		}
		field := p.advance().text
		current = memberNode{object: current, field: field}
	}
	return current, nil
}
