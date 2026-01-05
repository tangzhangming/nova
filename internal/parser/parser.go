package parser

import (
	"fmt"
	"strings"

	"github.com/tangzhangming/nova/internal/ast"
	"github.com/tangzhangming/nova/internal/i18n"
	"github.com/tangzhangming/nova/internal/lexer"
	"github.com/tangzhangming/nova/internal/token"
)

// Parser 语法分析器
type Parser struct {
	lexer    *lexer.Lexer
	tokens   []token.Token
	current  int
	errors   []Error
	filename string
}

// Error 语法分析错误
type Error struct {
	Pos     token.Position
	Message string
}

func (e Error) Error() string {
	return fmt.Sprintf("%s: %s", e.Pos, e.Message)
}

// New 创建一个新的语法分析器
func New(source, filename string) *Parser {
	l := lexer.New(source, filename)
	tokens := l.ScanTokens()

	return &Parser{
		lexer:    l,
		tokens:   tokens,
		current:  0,
		filename: filename,
	}
}

// Parse 解析源文件
func (p *Parser) Parse() *ast.File {
	file := &ast.File{
		Filename: p.filename,
	}

	// 解析命名空间
	if p.check(token.NAMESPACE) {
		file.Namespace = p.parseNamespace()
	}

	// 解析 use 声明
	for p.check(token.USE) {
		file.Uses = append(file.Uses, p.parseUse())
	}

	// 解析声明和语句
	for !p.isAtEnd() {
		if p.checkAny(token.CLASS, token.INTERFACE, token.ENUM, token.ABSTRACT, token.PUBLIC,
			token.PROTECTED, token.PRIVATE, token.AT) {
			decl := p.parseDeclaration()
			if decl != nil {
				file.Declarations = append(file.Declarations, decl)
			}
		} else {
			// 顶层语句 (入口文件)
			stmt := p.parseStatement()
			if stmt != nil {
				file.Statements = append(file.Statements, stmt)
			}
		}
	}

	return file
}

// Errors 返回所有语法错误
func (p *Parser) Errors() []Error {
	return p.errors
}

// HasErrors 检查是否有错误
func (p *Parser) HasErrors() bool {
	return len(p.errors) > 0
}

// ============================================================================
// 辅助方法
// ============================================================================

func (p *Parser) isAtEnd() bool {
	return p.peek().Type == token.EOF
}

func (p *Parser) peek() token.Token {
	return p.tokens[p.current]
}

func (p *Parser) previous() token.Token {
	return p.tokens[p.current-1]
}

func (p *Parser) advance() token.Token {
	if !p.isAtEnd() {
		p.current++
	}
	return p.previous()
}

func (p *Parser) check(t token.TokenType) bool {
	if p.isAtEnd() {
		return false
	}
	return p.peek().Type == t
}

func (p *Parser) checkAny(types ...token.TokenType) bool {
	for _, t := range types {
		if p.check(t) {
			return true
		}
	}
	return false
}

func (p *Parser) match(types ...token.TokenType) bool {
	for _, t := range types {
		if p.check(t) {
			p.advance()
			return true
		}
	}
	return false
}

func (p *Parser) consume(t token.TokenType, message string) token.Token {
	if p.check(t) {
		return p.advance()
	}
	p.error(message)
	return token.Token{}
}

func (p *Parser) error(message string) {
	p.errors = append(p.errors, Error{
		Pos:     p.peek().Pos,
		Message: message,
	})
}

func (p *Parser) synchronize() {
	p.advance()

	for !p.isAtEnd() {
		if p.previous().Type == token.SEMICOLON {
			return
		}

		switch p.peek().Type {
		case token.CLASS, token.INTERFACE, token.FUNCTION, token.IF, token.FOR,
			token.FOREACH, token.WHILE, token.RETURN, token.TRY:
			return
		}

		p.advance()
	}
}

// ============================================================================
// 类型解析
// ============================================================================

func (p *Parser) parseType() ast.TypeNode {
	// 解析第一个类型（可能是可空类型或基础类型）
	firstType := p.parseSingleType()
	if firstType == nil {
		return nil
	}

	// 检查是否是联合类型 (Type1 | Type2)
	if p.check(token.BIT_OR) {
		types := []ast.TypeNode{firstType}
		for p.match(token.BIT_OR) {
			nextType := p.parseSingleType()
			if nextType == nil {
				return nil
			}
			types = append(types, nextType)
		}
		return &ast.UnionType{Types: types}
	}

	return firstType
}

// parseSingleType 解析单个类型（可空类型或基础类型，包括数组）
func (p *Parser) parseSingleType() ast.TypeNode {
	// 可空类型 ?Type 转换为 Type | null
	if p.match(token.QUESTION) {
		inner := p.parseBaseType()
		if inner == nil {
			return nil
		}
		// 转换为联合类型: Type | null
		return &ast.UnionType{
			Types: []ast.TypeNode{inner, &ast.NullType{}},
		}
	}

	return p.parseBaseType()
}

func (p *Parser) parseBaseType() ast.TypeNode {
	// map 类型
	if p.match(token.MAP) {
		mapToken := p.previous()
		p.consume(token.LBRACKET, "expected '[' after 'map'")
		keyType := p.parseType()
		p.consume(token.RBRACKET, "expected ']' after map key type")
		valueType := p.parseType()
		return &ast.MapType{
			MapToken:  mapToken,
			KeyType:   keyType,
			ValueType: valueType,
		}
	}

	// func 类型
	if p.match(token.FUNC_TYPE) {
		return p.parseFuncType()
	}

	// 简单类型或类类型
	var baseType ast.TypeNode

	switch {
	case p.matchAny(token.INT_TYPE, token.I8_TYPE, token.I16_TYPE, token.I32_TYPE, token.I64_TYPE,
		token.UINT_TYPE, token.U8_TYPE, token.BYTE_TYPE, token.U16_TYPE, token.U32_TYPE, token.U64_TYPE,
		token.FLOAT_TYPE, token.F32_TYPE, token.F64_TYPE,
		token.BOOL_TYPE, token.STRING_TYPE, token.VOID, token.OBJECT):
		baseType = &ast.SimpleType{
			Token: p.previous(),
			Name:  p.previous().Literal,
		}
	case p.match(token.NULL):
		// null 类型（用于联合类型 Type | null）
		baseType = &ast.NullType{
			Token: p.previous(),
		}
	case p.check(token.IDENT):
		baseType = &ast.ClassType{
			Name: p.advance(),
		}
	default:
		p.error(i18n.T(i18n.ErrExpectedType))
		return nil
	}

	// 数组类型
	if p.check(token.LBRACKET) {
		lbracket := p.advance()
		var size ast.Expression
		if !p.check(token.RBRACKET) {
			size = p.parseExpression()
		}
		rbracket := p.consume(token.RBRACKET, "expected ']'")
		return &ast.ArrayType{
			ElementType: baseType,
			LBracket:    lbracket,
			Size:        size,
			RBracket:    rbracket,
		}
	}

	return baseType
}

func (p *Parser) parseFuncType() *ast.FuncType {
	funcToken := p.previous()
	p.consume(token.LPAREN, "expected '(' after 'func'")

	var params []ast.TypeNode
	if !p.check(token.RPAREN) {
		params = append(params, p.parseType())
		for p.match(token.COMMA) {
			params = append(params, p.parseType())
		}
	}
	p.consume(token.RPAREN, "expected ')'")

	var returnType ast.TypeNode
	if p.match(token.COLON) {
		returnType = p.parseReturnType()
	}

	return &ast.FuncType{
		FuncToken:  funcToken,
		Params:     params,
		ReturnType: returnType,
	}
}

func (p *Parser) parseReturnType() ast.TypeNode {
	// 检查是否是 void，如果是则报错
	if p.check(token.VOID) {
		p.error(i18n.T(i18n.ErrVoidNotAllowed))
		p.advance() // 跳过 void
		return nil
	}

	// 多返回值类型 (int, string)
	if p.match(token.LPAREN) {
		lparen := p.previous()
		var types []ast.TypeNode
		types = append(types, p.parseType())
		for p.match(token.COMMA) {
			types = append(types, p.parseType())
		}
		rparen := p.consume(token.RPAREN, "expected ')'")
		return &ast.TupleType{
			LParen: lparen,
			Types:  types,
			RParen: rparen,
		}
	}
	return p.parseType()
}

func (p *Parser) matchAny(types ...token.TokenType) bool {
	for _, t := range types {
		if p.check(t) {
			p.advance()
			return true
		}
	}
	return false
}

// ============================================================================
// 表达式解析 (Pratt Parser / 优先级攀升)
// ============================================================================

// 运算符优先级
const (
	PREC_NONE       = iota
	PREC_ASSIGNMENT // =, +=, -=, ...
	PREC_TERNARY    // ?:
	PREC_OR         // ||
	PREC_AND        // &&
	PREC_BIT_OR     // |
	PREC_BIT_XOR    // ^
	PREC_BIT_AND    // &
	PREC_EQUALITY   // ==, !=
	PREC_COMPARISON // <, >, <=, >=
	PREC_CAST       // as, as?
	PREC_SHIFT      // <<, >>
	PREC_TERM       // +, -
	PREC_FACTOR     // *, /, %
	PREC_UNARY      // !, -, ~, ++, --
	PREC_POSTFIX    // ++, --, [], ->, ::, ()
	PREC_CALL       // ()
	PREC_PRIMARY
)

func (p *Parser) getPrecedence(t token.TokenType) int {
	switch t {
	case token.ASSIGN, token.DECLARE, token.PLUS_ASSIGN, token.MINUS_ASSIGN,
		token.STAR_ASSIGN, token.SLASH_ASSIGN, token.PERCENT_ASSIGN:
		return PREC_ASSIGNMENT
	case token.QUESTION:
		return PREC_TERNARY
	case token.OR:
		return PREC_OR
	case token.AND:
		return PREC_AND
	case token.BIT_OR:
		return PREC_BIT_OR
	case token.BIT_XOR:
		return PREC_BIT_XOR
	case token.BIT_AND:
		return PREC_BIT_AND
	case token.EQ, token.NE:
		return PREC_EQUALITY
	case token.LT, token.LE, token.GT, token.GE:
		return PREC_COMPARISON
	case token.AS, token.AS_SAFE:
		return PREC_CAST
	case token.LEFT_SHIFT, token.RIGHT_SHIFT:
		return PREC_SHIFT
	case token.PLUS, token.MINUS:
		return PREC_TERM
	case token.STAR, token.SLASH, token.PERCENT:
		return PREC_FACTOR
	case token.LBRACKET, token.ARROW, token.DOUBLE_COLON, token.LPAREN, token.DOT,
		token.INCREMENT, token.DECREMENT:
		return PREC_POSTFIX
	default:
		return PREC_NONE
	}
}

func (p *Parser) parseExpression() ast.Expression {
	return p.parsePrecedence(PREC_ASSIGNMENT)
}

func (p *Parser) parsePrecedence(precedence int) ast.Expression {
	left := p.parsePrefixExpr()
	if left == nil {
		return nil
	}

	for precedence <= p.getPrecedence(p.peek().Type) {
		left = p.parseInfixExpr(left)
		if left == nil {
			return nil
		}
	}

	return left
}

func (p *Parser) parsePrefixExpr() ast.Expression {
	switch p.peek().Type {
	case token.INT:
		tok := p.advance()
		return &ast.IntegerLiteral{
			Token: tok,
			Value: tok.Value.(int64),
		}
	case token.FLOAT:
		tok := p.advance()
		return &ast.FloatLiteral{
			Token: tok,
			Value: tok.Value.(float64),
		}
	case token.STRING:
		tok := p.advance()
		return &ast.StringLiteral{
			Token: tok,
			Value: tok.Value.(string),
		}
	case token.INTERP_STRING:
		tok := p.advance()
		parts := p.parseInterpStringParts(tok)
		return &ast.InterpStringLiteral{
			Token: tok,
			Parts: parts,
		}
	case token.TRUE:
		tok := p.advance()
		return &ast.BoolLiteral{Token: tok, Value: true}
	case token.FALSE:
		tok := p.advance()
		return &ast.BoolLiteral{Token: tok, Value: false}
	case token.NULL:
		tok := p.advance()
		return &ast.NullLiteral{Token: tok}
	case token.VARIABLE:
		tok := p.advance()
		name := tok.Literal[1:] // 去掉 $
		return &ast.Variable{Token: tok, Name: name}
	case token.THIS:
		tok := p.advance()
		return &ast.ThisExpr{Token: tok}
	case token.IDENT:
		// 检查是否是类型后跟 { (如 MyClass{})
		if p.lookAhead(1).Type == token.LBRACE {
			return p.parseTypedArrayLiteral()
		}
		tok := p.advance()
		return &ast.Identifier{Token: tok, Name: tok.Literal}
	case token.SELF:
		tok := p.advance()
		return &ast.SelfExpr{Token: tok}
	case token.PARENT:
		tok := p.advance()
		return &ast.ParentExpr{Token: tok}
	case token.LPAREN:
		return p.parseGroupOrArrowFunc()
	case token.LBRACKET:
		// 保留用于将来的万能数组，暂时报错
		p.error("'[' syntax reserved for super array (use Type{...} for arrays)")
		p.advance()
		return nil
	case token.NOT, token.MINUS, token.BIT_NOT:
		return p.parseUnaryExpr()
	case token.INCREMENT, token.DECREMENT:
		return p.parsePrefixIncDec()
	case token.NEW:
		return p.parseNewExpr()
	case token.FUNCTION:
		return p.parseClosureExpr()
	// Go 风格数组字面量: int{1, 2, 3}
	case token.INT_TYPE, token.I8_TYPE, token.I16_TYPE, token.I32_TYPE, token.I64_TYPE,
		token.UINT_TYPE, token.U8_TYPE, token.BYTE_TYPE, token.U16_TYPE, token.U32_TYPE, token.U64_TYPE,
		token.FLOAT_TYPE, token.F32_TYPE, token.F64_TYPE,
		token.BOOL_TYPE, token.STRING_TYPE, token.OBJECT:
		return p.parseTypedArrayLiteral()
	// Go 风格 Map 字面量: map[K]V{...}
	case token.MAP:
		return p.parseTypedMapLiteral()
	default:
		p.error(i18n.T(i18n.ErrUnexpectedToken, p.peek().Type))
		p.advance() // 跳过无效 token，防止无限循环
		return nil
	}
}

func (p *Parser) parseInfixExpr(left ast.Expression) ast.Expression {
	switch p.peek().Type {
	case token.PLUS, token.MINUS, token.STAR, token.SLASH, token.PERCENT,
		token.EQ, token.NE, token.LT, token.LE, token.GT, token.GE,
		token.AND, token.OR, token.BIT_AND, token.BIT_OR, token.BIT_XOR,
		token.LEFT_SHIFT, token.RIGHT_SHIFT:
		return p.parseBinaryExpr(left)
	case token.ASSIGN, token.PLUS_ASSIGN, token.MINUS_ASSIGN,
		token.STAR_ASSIGN, token.SLASH_ASSIGN, token.PERCENT_ASSIGN:
		return p.parseAssignExpr(left)
	case token.QUESTION:
		return p.parseTernaryExpr(left)
	case token.AS, token.AS_SAFE:
		return p.parseTypeCastExpr(left)
	case token.LBRACKET:
		return p.parseIndexExpr(left)
	case token.ARROW:
		return p.parsePropertyOrMethodAccess(left)
	case token.DOUBLE_COLON:
		return p.parseStaticAccess(left)
	case token.LPAREN:
		return p.parseCallExpr(left)
	case token.DOT:
		return p.parseDotAccess(left)
	case token.INCREMENT, token.DECREMENT:
		return p.parsePostfixIncDec(left)
	default:
		return left
	}
}

func (p *Parser) parseTypeCastExpr(left ast.Expression) ast.Expression {
	// 禁止链式类型断言: $x as A as B
	if _, ok := left.(*ast.TypeCastExpr); ok {
		p.error(i18n.T(i18n.ErrChainedTypeCast))
		return left
	}

	asToken := p.advance()
	safe := asToken.Type == token.AS_SAFE
	targetType := p.parseType() // 支持所有类型，包括 string[], map[K]V 等

	return &ast.TypeCastExpr{
		Expr:       left,
		AsToken:    asToken,
		Safe:       safe,
		TargetType: targetType,
	}
}

func (p *Parser) parseBinaryExpr(left ast.Expression) ast.Expression {
	op := p.advance()
	prec := p.getPrecedence(op.Type)
	right := p.parsePrecedence(prec + 1)
	return &ast.BinaryExpr{
		Left:     left,
		Operator: op,
		Right:    right,
	}
}

func (p *Parser) parseAssignExpr(left ast.Expression) ast.Expression {
	// 检查左侧是否是有效的赋值目标
	if !p.isValidAssignTarget(left) {
		p.error(i18n.T(i18n.ErrInvalidAssignTarget))
	}

	op := p.advance()
	right := p.parsePrecedence(PREC_ASSIGNMENT)
	return &ast.AssignExpr{
		Left:     left,
		Operator: op,
		Right:    right,
	}
}

// isValidAssignTarget 检查表达式是否是有效的赋值目标
func (p *Parser) isValidAssignTarget(expr ast.Expression) bool {
	switch expr.(type) {
	case *ast.Variable:
		return true // $var = ...
	case *ast.IndexExpr:
		return true // $arr[0] = ...
	case *ast.PropertyAccess:
		return true // $obj.prop = ...
	case *ast.StaticAccess:
		return true // Class::$var = ...
	default:
		return false
	}
}

func (p *Parser) parseTernaryExpr(left ast.Expression) ast.Expression {
	question := p.advance()
	then := p.parseExpression()
	colon := p.consume(token.COLON, "expected ':' in ternary expression")
	elseExpr := p.parsePrecedence(PREC_TERNARY)
	return &ast.TernaryExpr{
		Condition: left,
		Question:  question,
		Then:      then,
		Colon:     colon,
		Else:      elseExpr,
	}
}

func (p *Parser) parseUnaryExpr() ast.Expression {
	op := p.advance()
	operand := p.parsePrecedence(PREC_UNARY)
	return &ast.UnaryExpr{
		Operator: op,
		Operand:  operand,
		Prefix:   true,
	}
}

func (p *Parser) parsePrefixIncDec() ast.Expression {
	op := p.advance()
	operand := p.parsePrecedence(PREC_UNARY)
	return &ast.UnaryExpr{
		Operator: op,
		Operand:  operand,
		Prefix:   true,
	}
}

func (p *Parser) parsePostfixIncDec(left ast.Expression) ast.Expression {
	op := p.advance()
	return &ast.UnaryExpr{
		Operator: op,
		Operand:  left,
		Prefix:   false,
	}
}

func (p *Parser) parseGroupOrArrowFunc() ast.Expression {
	lparen := p.advance() // 消费 (

	// 尝试判断是否是箭头函数
	// 箭头函数格式: (type $param, ...) : returnType => expr
	// 或: (type $param, ...) => expr

	// 保存当前位置用于回溯
	savedCurrent := p.current

	// 尝试解析为箭头函数参数
	if p.tryParseArrowFuncParams() {
		// 成功解析参数，检查是否有 =>
		if p.check(token.DOUBLE_ARROW) || p.check(token.COLON) {
			// 是箭头函数，回溯并正式解析
			p.current = savedCurrent
			return p.parseArrowFuncFromParams(lparen)
		}
	}

	// 不是箭头函数，回溯并解析为分组表达式
	p.current = savedCurrent

	expr := p.parseExpression()
	p.consume(token.RPAREN, "expected ')'")
	return expr
}

func (p *Parser) tryParseArrowFuncParams() bool {
	// 尝试解析参数列表，如果成功返回 true
	for !p.check(token.RPAREN) && !p.isAtEnd() {
		// 跳过类型
		if p.checkAny(token.INT_TYPE, token.STRING_TYPE, token.BOOL_TYPE, token.FLOAT_TYPE,
			token.VOID, token.OBJECT, token.IDENT, token.QUESTION) {
			p.advance()
			// 处理数组类型
			if p.check(token.LBRACKET) {
				p.advance()
				if !p.check(token.RBRACKET) {
					// 跳过数组大小
					for !p.check(token.RBRACKET) && !p.isAtEnd() {
						p.advance()
					}
				}
				if p.check(token.RBRACKET) {
					p.advance()
				}
			}
		}

		// 检查变量名
		if p.check(token.VARIABLE) {
			p.advance()
		} else {
			return false
		}

		// 检查默认值
		if p.check(token.ASSIGN) {
			p.advance()
			// 跳过默认值表达式（简单处理）
			depth := 0
			for !p.isAtEnd() {
				if p.check(token.LPAREN) || p.check(token.LBRACKET) {
					depth++
				} else if p.check(token.RPAREN) || p.check(token.RBRACKET) {
					if depth == 0 {
						break
					}
					depth--
				} else if p.check(token.COMMA) && depth == 0 {
					break
				}
				p.advance()
			}
		}

		if !p.match(token.COMMA) {
			break
		}
	}

	if !p.check(token.RPAREN) {
		return false
	}
	p.advance() // 消费 )
	return true
}

func (p *Parser) parseArrowFuncFromParams(lparen token.Token) ast.Expression {
	var params []*ast.Parameter

	if !p.check(token.RPAREN) {
		params = append(params, p.parseParameter())
		for p.match(token.COMMA) {
			params = append(params, p.parseParameter())
		}
	}

	rparen := p.consume(token.RPAREN, "expected ')'")

	var returnType ast.TypeNode
	if p.match(token.COLON) {
		returnType = p.parseReturnType()
	}

	arrow := p.consume(token.DOUBLE_ARROW, "expected '=>'")
	body := p.parseExpression()

	return &ast.ArrowFuncExpr{
		LParen:     lparen,
		Parameters: params,
		RParen:     rparen,
		ReturnType: returnType,
		Arrow:      arrow,
		Body:       body,
	}
}

// parseTypedArrayLiteral 解析 Go 风格数组字面量: int{1, 2, 3}
func (p *Parser) parseTypedArrayLiteral() ast.Expression {
	// 解析元素类型
	elementType := p.parseType()

	// 期望 {
	lbrace := p.consume(token.LBRACE, "expected '{' after type")

	// 解析元素
	var elements []ast.Expression
	if !p.check(token.RBRACE) {
		elements = append(elements, p.parseExpression())
		for p.match(token.COMMA) {
			if p.check(token.RBRACE) {
				break // 允许尾逗号
			}
			elements = append(elements, p.parseExpression())
		}
	}

	rbrace := p.consume(token.RBRACE, "expected '}'")
	return &ast.ArrayLiteral{
		ElementType: elementType,
		LBrace:      lbrace,
		Elements:    elements,
		RBrace:      rbrace,
	}
}

// parseTypedMapLiteral 解析 Go 风格 Map 字面量: map[K]V{...}
func (p *Parser) parseTypedMapLiteral() ast.Expression {
	mapToken := p.advance() // 消费 map

	// 解析 [KeyType]
	p.consume(token.LBRACKET, "expected '[' after 'map'")
	keyType := p.parseType()
	p.consume(token.RBRACKET, "expected ']' after map key type")

	// 解析 ValueType
	valueType := p.parseType()

	// 期望 {
	lbrace := p.consume(token.LBRACE, "expected '{' after map type")

	// 解析键值对
	var pairs []ast.MapPair
	if !p.check(token.RBRACE) {
		key := p.parseExpression()
		colon := p.consume(token.COLON, "expected ':' after map key")
		value := p.parseExpression()
		pairs = append(pairs, ast.MapPair{
			Key:   key,
			Colon: colon,
			Value: value,
		})

		for p.match(token.COMMA) {
			if p.check(token.RBRACE) {
				break // 允许尾逗号
			}
			key := p.parseExpression()
			colon := p.consume(token.COLON, "expected ':' after map key")
			value := p.parseExpression()
			pairs = append(pairs, ast.MapPair{
				Key:   key,
				Colon: colon,
				Value: value,
			})
		}
	}

	rbrace := p.consume(token.RBRACE, "expected '}'")
	return &ast.MapLiteral{
		MapToken:  mapToken,
		KeyType:   keyType,
		ValueType: valueType,
		LBrace:    lbrace,
		Pairs:     pairs,
		RBrace:    rbrace,
	}
}

// parseUntypedCollectionLiteral 解析无类型集合字面量 {...}（从上下文推断类型）
func (p *Parser) parseUntypedCollectionLiteral(expectedType ast.TypeNode) ast.Expression {
	lbrace := p.consume(token.LBRACE, "expected '{'")

	// 空集合
	if p.check(token.RBRACE) {
		rbrace := p.advance()
		// 根据期望类型决定是数组还是 Map
		if mapType, ok := expectedType.(*ast.MapType); ok {
			return &ast.MapLiteral{
				KeyType:   mapType.KeyType,
				ValueType: mapType.ValueType,
				LBrace:    lbrace,
				RBrace:    rbrace,
			}
		}
		// 默认是数组
		var elemType ast.TypeNode
		if arrType, ok := expectedType.(*ast.ArrayType); ok {
			elemType = arrType.ElementType
		}
		return &ast.ArrayLiteral{
			ElementType: elemType,
			LBrace:      lbrace,
			RBrace:      rbrace,
		}
	}

	// 解析第一个元素
	first := p.parseExpression()

	// 检查是否是 Map (key: value)
	if p.check(token.COLON) {
		return p.parseUntypedMapLiteralRest(lbrace, first, expectedType)
	}

	// 普通数组
	elements := []ast.Expression{first}
	for p.match(token.COMMA) {
		if p.check(token.RBRACE) {
			break // 允许尾逗号
		}
		elements = append(elements, p.parseExpression())
	}

	rbrace := p.consume(token.RBRACE, "expected '}'")

	var elemType ast.TypeNode
	if arrType, ok := expectedType.(*ast.ArrayType); ok {
		elemType = arrType.ElementType
	}
	return &ast.ArrayLiteral{
		ElementType: elemType,
		LBrace:      lbrace,
		Elements:    elements,
		RBrace:      rbrace,
	}
}

// parseUntypedMapLiteralRest 解析无类型 Map 字面量的剩余部分
func (p *Parser) parseUntypedMapLiteralRest(lbrace token.Token, firstKey ast.Expression, expectedType ast.TypeNode) ast.Expression {
	colon := p.advance() // 消费 :
	firstValue := p.parseExpression()

	pairs := []ast.MapPair{{
		Key:   firstKey,
		Colon: colon,
		Value: firstValue,
	}}

	for p.match(token.COMMA) {
		if p.check(token.RBRACE) {
			break // 允许尾逗号
		}
		key := p.parseExpression()
		colon := p.consume(token.COLON, "expected ':'")
		value := p.parseExpression()
		pairs = append(pairs, ast.MapPair{
			Key:   key,
			Colon: colon,
			Value: value,
		})
	}

	rbrace := p.consume(token.RBRACE, "expected '}'")

	var keyType, valueType ast.TypeNode
	if mapType, ok := expectedType.(*ast.MapType); ok {
		keyType = mapType.KeyType
		valueType = mapType.ValueType
	}
	return &ast.MapLiteral{
		KeyType:   keyType,
		ValueType: valueType,
		LBrace:    lbrace,
		Pairs:     pairs,
		RBrace:    rbrace,
	}
}

func (p *Parser) parseIndexExpr(left ast.Expression) ast.Expression {
	lbracket := p.advance()
	index := p.parseExpression()
	rbracket := p.consume(token.RBRACKET, "expected ']'")
	return &ast.IndexExpr{
		Object:   left,
		LBracket: lbracket,
		Index:    index,
		RBracket: rbracket,
	}
}

func (p *Parser) parsePropertyOrMethodAccess(left ast.Expression) ast.Expression {
	arrow := p.advance()
	property := p.consume(token.IDENT, "expected property name")

	if p.check(token.LPAREN) {
		// 方法调用
		lparen := p.advance()
		var args []ast.Expression
		if !p.check(token.RPAREN) {
			args = append(args, p.parseExpression())
			for p.match(token.COMMA) {
				args = append(args, p.parseExpression())
			}
		}
		rparen := p.consume(token.RPAREN, "expected ')'")
		return &ast.MethodCall{
			Object:    left,
			Arrow:     arrow,
			Method:    &ast.Identifier{Token: property, Name: property.Literal},
			LParen:    lparen,
			Arguments: args,
			RParen:    rparen,
		}
	}

	// 属性访问
	return &ast.PropertyAccess{
		Object:   left,
		Arrow:    arrow,
		Property: &ast.Identifier{Token: property, Name: property.Literal},
	}
}

func (p *Parser) parseDotAccess(left ast.Expression) ast.Expression {
	p.advance() // 消费 .
	property := p.consume(token.IDENT, "expected property name after '.'")

	// 目前 . 只用于属性访问（如 $arr.length, $map.has()）
	if p.check(token.LPAREN) {
		// 方法调用
		lparen := p.advance()
		var args []ast.Expression
		if !p.check(token.RPAREN) {
			args = append(args, p.parseExpression())
			for p.match(token.COMMA) {
				args = append(args, p.parseExpression())
			}
		}
		rparen := p.consume(token.RPAREN, "expected ')'")
		return &ast.MethodCall{
			Object:    left,
			Arrow:     token.Token{Type: token.DOT},
			Method:    &ast.Identifier{Token: property, Name: property.Literal},
			LParen:    lparen,
			Arguments: args,
			RParen:    rparen,
		}
	}

	return &ast.PropertyAccess{
		Object:   left,
		Arrow:    token.Token{Type: token.DOT},
		Property: &ast.Identifier{Token: property, Name: property.Literal},
	}
}

func (p *Parser) parseStaticAccess(left ast.Expression) ast.Expression {
	doubleColon := p.advance()

	// 检查是否是 ::class
	if p.check(token.CLASS) {
		classToken := p.advance()
		return &ast.ClassAccessExpr{
			Object:      left,
			DoubleColon: doubleColon,
			Class:       classToken,
		}
	}

	// 静态成员访问
	var member ast.Expression
	if p.check(token.VARIABLE) {
		tok := p.advance()
		member = &ast.Variable{Token: tok, Name: tok.Literal[1:]}
	} else if p.check(token.IDENT) {
		tok := p.advance()
		member = &ast.Identifier{Token: tok, Name: tok.Literal}

		// 检查是否是方法调用
		if p.check(token.LPAREN) {
			lparen := p.advance()
			var args []ast.Expression
			if !p.check(token.RPAREN) {
				args = append(args, p.parseExpression())
				for p.match(token.COMMA) {
					args = append(args, p.parseExpression())
				}
			}
			rparen := p.consume(token.RPAREN, "expected ')'")
			member = &ast.CallExpr{
				Function:  member,
				LParen:    lparen,
				Arguments: args,
				RParen:    rparen,
			}
		}
	} else {
		p.error(i18n.T(i18n.ErrInvalidStaticMember))
		return left
	}

	return &ast.StaticAccess{
		Class:       left,
		DoubleColon: doubleColon,
		Member:      member,
	}
}

func (p *Parser) parseCallExpr(left ast.Expression) ast.Expression {
	lparen := p.advance()
	var args []ast.Expression
	if !p.check(token.RPAREN) {
		args = append(args, p.parseExpression())
		for p.match(token.COMMA) {
			args = append(args, p.parseExpression())
		}
	}
	rparen := p.consume(token.RPAREN, "expected ')'")
	return &ast.CallExpr{
		Function:  left,
		LParen:    lparen,
		Arguments: args,
		RParen:    rparen,
	}
}

func (p *Parser) parseNewExpr() ast.Expression {
	newToken := p.advance()
	className := p.consume(token.IDENT, "expected class name after 'new'")

	lparen := p.consume(token.LPAREN, "expected '(' after class name")
	var args []ast.Expression
	if !p.check(token.RPAREN) {
		args = append(args, p.parseExpression())
		for p.match(token.COMMA) {
			args = append(args, p.parseExpression())
		}
	}
	rparen := p.consume(token.RPAREN, "expected ')'")

	return &ast.NewExpr{
		NewToken:  newToken,
		ClassName: &ast.Identifier{Token: className, Name: className.Literal},
		LParen:    lparen,
		Arguments: args,
		RParen:    rparen,
	}
}

func (p *Parser) parseClosureExpr() ast.Expression {
	funcToken := p.advance()
	lparen := p.consume(token.LPAREN, "expected '(' after 'function'")

	var params []*ast.Parameter
	if !p.check(token.RPAREN) {
		params = append(params, p.parseParameter())
		for p.match(token.COMMA) {
			params = append(params, p.parseParameter())
		}
	}
	rparen := p.consume(token.RPAREN, "expected ')'")

	// 解析 use 子句：use ($a, $b)
	var useVars []*ast.Variable
	if p.match(token.USE) {
		p.consume(token.LPAREN, "expected '(' after 'use'")
		if !p.check(token.RPAREN) {
			varTok := p.consume(token.VARIABLE, "expected variable in use clause")
			// 安全检查：防止空 token 导致 panic
			varNameStr := ""
			if len(varTok.Literal) > 0 {
				varNameStr = varTok.Literal[1:]
			}
			useVars = append(useVars, &ast.Variable{Token: varTok, Name: varNameStr})
			for p.match(token.COMMA) {
				varTok = p.consume(token.VARIABLE, "expected variable in use clause")
				varNameStr = ""
				if len(varTok.Literal) > 0 {
					varNameStr = varTok.Literal[1:]
				}
				useVars = append(useVars, &ast.Variable{Token: varTok, Name: varNameStr})
			}
		}
		p.consume(token.RPAREN, "expected ')' after use variables")
	}

	var returnType ast.TypeNode
	if p.match(token.COLON) {
		returnType = p.parseReturnType()
	}

	body := p.parseBlock()

	return &ast.ClosureExpr{
		FuncToken:  funcToken,
		LParen:     lparen,
		Parameters: params,
		RParen:     rparen,
		UseVars:    useVars,
		ReturnType: returnType,
		Body:       body,
	}
}

func (p *Parser) parseParameter() *ast.Parameter {
	param := &ast.Parameter{}

	// 可变参数前缀
	if p.match(token.ELLIPSIS) {
		param.Variadic = true
	}

	// 类型声明（必须）
	if !p.check(token.VARIABLE) {
		param.Type = p.parseType()
	} else {
		// 没有类型声明，报错
		p.error(i18n.T(i18n.ErrExpectedType) + " before parameter name")
	}

	// 可变参数（类型后面的 ...）
	if p.match(token.ELLIPSIS) {
		param.Variadic = true
	}

	// 变量名
	varToken := p.consume(token.VARIABLE, "expected parameter name")
	varName := ""
	if len(varToken.Literal) > 1 {
		varName = varToken.Literal[1:]
	}
	param.Name = &ast.Variable{Token: varToken, Name: varName}

	// 默认值
	if p.match(token.ASSIGN) {
		param.Default = p.parseExpression()
	}

	return param
}

// ============================================================================
// 语句解析
// ============================================================================

func (p *Parser) parseStatement() ast.Statement {
	switch p.peek().Type {
	case token.IF:
		return p.parseIfStmt()
	case token.SWITCH:
		return p.parseSwitchStmt()
	case token.FOR:
		return p.parseForStmt()
	case token.FOREACH:
		return p.parseForeachStmt()
	case token.WHILE:
		return p.parseWhileStmt()
	case token.DO:
		return p.parseDoWhileStmt()
	case token.BREAK:
		return p.parseBreakStmt()
	case token.CONTINUE:
		return p.parseContinueStmt()
	case token.RETURN:
		return p.parseReturnStmt()
	case token.TRY:
		return p.parseTryStmt()
	case token.THROW:
		return p.parseThrowStmt()
	case token.ECHO:
		return p.parseEchoStmt()
	case token.LBRACE:
		return p.parseBlock()
	default:
		return p.parseExprOrVarDeclStmt()
	}
}

func (p *Parser) parseExprOrVarDeclStmt() ast.Statement {
	// 尝试解析变量声明
	// 形式1: type $var = expr;
	// 形式2: $var := expr;
	// 形式3: $var1, $var2 := expr;

	// 检查是否以类型开始
	if p.isTypeStart() {
		return p.parseVarDeclWithType()
	}

	// 检查是否是变量开始（可能是多变量声明或表达式）
	if p.check(token.VARIABLE) {
		return p.parseVarDeclOrExprStmt()
	}

	// 普通表达式语句
	expr := p.parseExpression()
	semicolon := p.consume(token.SEMICOLON, "expected ';' after expression")
	return &ast.ExprStmt{
		Expr:      expr,
		Semicolon: semicolon,
	}
}

func (p *Parser) isTypeStart() bool {
	return p.checkAny(token.INT_TYPE, token.I8_TYPE, token.I16_TYPE, token.I32_TYPE, token.I64_TYPE,
		token.UINT_TYPE, token.U8_TYPE, token.BYTE_TYPE, token.U16_TYPE, token.U32_TYPE, token.U64_TYPE,
		token.FLOAT_TYPE, token.F32_TYPE, token.F64_TYPE,
		token.BOOL_TYPE, token.STRING_TYPE, token.VOID, token.OBJECT,
		token.MAP, token.FUNC_TYPE, token.QUESTION) ||
		(p.check(token.IDENT) && p.lookAhead(1).Type == token.VARIABLE)
}

func (p *Parser) lookAhead(n int) token.Token {
	if p.current+n >= len(p.tokens) {
		return p.tokens[len(p.tokens)-1]
	}
	return p.tokens[p.current+n]
}

func (p *Parser) parseVarDeclWithType() ast.Statement {
	varType := p.parseType()
	varToken := p.consume(token.VARIABLE, "expected variable name")
	// 安全检查：防止空 token 导致 panic
	varNameStr := ""
	if len(varToken.Literal) > 0 {
		varNameStr = varToken.Literal[1:]
	}
	varName := &ast.Variable{Token: varToken, Name: varNameStr}

	var value ast.Expression
	op := p.consume(token.ASSIGN, "expected '=' after variable name")
	if !p.check(token.SEMICOLON) {
		// 检查是否是无类型集合字面量 {...}
		if p.check(token.LBRACE) {
			value = p.parseUntypedCollectionLiteral(varType)
		} else {
			value = p.parseExpression()
		}
	}

	semicolon := p.consume(token.SEMICOLON, "expected ';' after variable declaration")

	return &ast.VarDeclStmt{
		Type:      varType,
		Name:      varName,
		Operator:  op,
		Value:     value,
		Semicolon: semicolon,
	}
}

func (p *Parser) parseVarDeclOrExprStmt() ast.Statement {
	// 收集变量名
	var vars []*ast.Variable
	firstVar := p.advance()
	vars = append(vars, &ast.Variable{Token: firstVar, Name: firstVar.Literal[1:]})

	// 检查是否是多变量
	for p.match(token.COMMA) {
		if !p.check(token.VARIABLE) {
			// 不是多变量声明，回溯
			// 实际上这种情况应该是语法错误
			p.error(i18n.T(i18n.ErrExpectedParamName))
			break
		}
		varToken := p.advance()
		vars = append(vars, &ast.Variable{Token: varToken, Name: varToken.Literal[1:]})
	}

	// 检查是 := 还是 = 还是其他
	if p.check(token.DECLARE) {
		op := p.advance()
		value := p.parseExpression()
		semicolon := p.consume(token.SEMICOLON, "expected ';'")

		if len(vars) > 1 {
			return &ast.MultiVarDeclStmt{
				Names:     vars,
				Operator:  op,
				Value:     value,
				Semicolon: semicolon,
			}
		}
		return &ast.VarDeclStmt{
			Name:      vars[0],
			Operator:  op,
			Value:     value,
			Semicolon: semicolon,
		}
	}

	// 不是声明，是表达式语句
	// 需要重新解析
	if len(vars) > 1 {
		// 多变量但不是 :=，这是错误
		p.error(i18n.T(i18n.ErrExpectedToken, "':='"))
	}

	// 将第一个变量作为表达式的开始
	left := ast.Expression(vars[0])

	// 继续解析中缀表达式
	for p.getPrecedence(p.peek().Type) > PREC_NONE {
		left = p.parseInfixExpr(left)
	}

	semicolon := p.consume(token.SEMICOLON, "expected ';' after expression")
	return &ast.ExprStmt{
		Expr:      left,
		Semicolon: semicolon,
	}
}

func (p *Parser) parseBlock() *ast.BlockStmt {
	lbrace := p.consume(token.LBRACE, "expected '{'")

	var stmts []ast.Statement
	for !p.check(token.RBRACE) && !p.isAtEnd() {
		stmts = append(stmts, p.parseStatement())
	}

	rbrace := p.consume(token.RBRACE, "expected '}'")

	return &ast.BlockStmt{
		LBrace:     lbrace,
		Statements: stmts,
		RBrace:     rbrace,
	}
}

func (p *Parser) parseIfStmt() *ast.IfStmt {
	ifToken := p.advance()
	p.consume(token.LPAREN, "expected '(' after 'if'")
	condition := p.parseExpression()
	p.consume(token.RPAREN, "expected ')'")
	then := p.parseBlock()

	var elseIfs []*ast.ElseIfClause
	var elseBlock *ast.BlockStmt

	for p.check(token.ELSEIF) {
		elseIfToken := p.advance()
		p.consume(token.LPAREN, "expected '(' after 'elseif'")
		elseIfCond := p.parseExpression()
		p.consume(token.RPAREN, "expected ')'")
		elseIfBody := p.parseBlock()
		elseIfs = append(elseIfs, &ast.ElseIfClause{
			ElseIfToken: elseIfToken,
			Condition:   elseIfCond,
			Body:        elseIfBody,
		})
	}

	if p.match(token.ELSE) {
		elseBlock = p.parseBlock()
	}

	return &ast.IfStmt{
		IfToken:   ifToken,
		Condition: condition,
		Then:      then,
		ElseIfs:   elseIfs,
		Else:      elseBlock,
	}
}

func (p *Parser) parseSwitchStmt() *ast.SwitchStmt {
	switchToken := p.advance()
	p.consume(token.LPAREN, "expected '(' after 'switch'")
	expr := p.parseExpression()
	p.consume(token.RPAREN, "expected ')'")
	lbrace := p.consume(token.LBRACE, "expected '{'")

	var cases []*ast.CaseClause
	var defaultClause *ast.DefaultClause

	for !p.check(token.RBRACE) && !p.isAtEnd() {
		if p.check(token.CASE) {
			caseToken := p.advance()
			value := p.parseExpression()
			colon := p.consume(token.COLON, "expected ':'")

			var body []ast.Statement
			for !p.checkAny(token.CASE, token.DEFAULT, token.RBRACE) && !p.isAtEnd() {
				body = append(body, p.parseStatement())
			}

			cases = append(cases, &ast.CaseClause{
				CaseToken: caseToken,
				Value:     value,
				Colon:     colon,
				Body:      body,
			})
		} else if p.check(token.DEFAULT) {
			defaultToken := p.advance()
			colon := p.consume(token.COLON, "expected ':'")

			var body []ast.Statement
			for !p.checkAny(token.CASE, token.RBRACE) && !p.isAtEnd() {
				body = append(body, p.parseStatement())
			}

			defaultClause = &ast.DefaultClause{
				DefaultToken: defaultToken,
				Colon:        colon,
				Body:         body,
			}
		} else {
			p.error(i18n.T(i18n.ErrExpectedCaseDefault))
			break
		}
	}

	rbrace := p.consume(token.RBRACE, "expected '}'")

	return &ast.SwitchStmt{
		SwitchToken: switchToken,
		Expr:        expr,
		LBrace:      lbrace,
		Cases:       cases,
		Default:     defaultClause,
		RBrace:      rbrace,
	}
}

func (p *Parser) parseForStmt() *ast.ForStmt {
	forToken := p.advance()
	p.consume(token.LPAREN, "expected '(' after 'for'")

	// 初始化
	var init ast.Statement
	if !p.check(token.SEMICOLON) {
		init = p.parseExprOrVarDeclStmt()
	} else {
		p.advance() // 消费 ;
	}

	// 条件
	var condition ast.Expression
	if !p.check(token.SEMICOLON) {
		condition = p.parseExpression()
	}
	p.consume(token.SEMICOLON, "expected ';' after for condition")

	// 后置表达式
	var post ast.Expression
	if !p.check(token.RPAREN) {
		post = p.parseExpression()
	}
	p.consume(token.RPAREN, "expected ')'")

	body := p.parseBlock()

	return &ast.ForStmt{
		ForToken:  forToken,
		Init:      init,
		Condition: condition,
		Post:      post,
		Body:      body,
	}
}

func (p *Parser) parseForeachStmt() *ast.ForeachStmt {
	foreachToken := p.advance()
	p.consume(token.LPAREN, "expected '(' after 'foreach'")

	// 解析 iterable 表达式，但使用 PREC_CAST 优先级，避免把 as 当作类型转换
	// 使用 PREC_CAST 而不是 PREC_CAST - 1，因为 parsePrecedence 的条件是 precedence <= getPrecedence(token)
	// 所以使用 PREC_CAST 时，遇到 as token 会停止（因为 PREC_CAST <= PREC_CAST 是 true，但我们需要停止）
	// 实际上应该使用 PREC_CAST + 1，这样遇到 as 时会停止
	iterable := p.parsePrecedence(PREC_CAST + 1)
	asToken := p.consume(token.AS, "expected 'as'")

	var key *ast.Variable
	valueToken := p.consume(token.VARIABLE, "expected variable")
	// 安全检查：防止空 token 导致 panic
	valueNameStr := ""
	if len(valueToken.Literal) > 0 {
		valueNameStr = valueToken.Literal[1:]
	}
	value := &ast.Variable{Token: valueToken, Name: valueNameStr}

	// 检查是否有 key => value 形式
	if p.match(token.DOUBLE_ARROW) {
		key = value
		valueToken = p.consume(token.VARIABLE, "expected variable after '=>'")
		valueNameStr = ""
		if len(valueToken.Literal) > 0 {
			valueNameStr = valueToken.Literal[1:]
		}
		value = &ast.Variable{Token: valueToken, Name: valueNameStr}
	}

	p.consume(token.RPAREN, "expected ')'")
	body := p.parseBlock()

	return &ast.ForeachStmt{
		ForeachToken: foreachToken,
		Iterable:     iterable,
		AsToken:      asToken,
		Key:          key,
		Value:        value,
		Body:         body,
	}
}

func (p *Parser) parseWhileStmt() *ast.WhileStmt {
	whileToken := p.advance()
	p.consume(token.LPAREN, "expected '(' after 'while'")
	condition := p.parseExpression()
	p.consume(token.RPAREN, "expected ')'")
	body := p.parseBlock()

	return &ast.WhileStmt{
		WhileToken: whileToken,
		Condition:  condition,
		Body:       body,
	}
}

func (p *Parser) parseDoWhileStmt() *ast.DoWhileStmt {
	doToken := p.advance()
	body := p.parseBlock()
	whileToken := p.consume(token.WHILE, "expected 'while'")
	p.consume(token.LPAREN, "expected '('")
	condition := p.parseExpression()
	p.consume(token.RPAREN, "expected ')'")
	semicolon := p.consume(token.SEMICOLON, "expected ';'")

	return &ast.DoWhileStmt{
		DoToken:    doToken,
		Body:       body,
		WhileToken: whileToken,
		Condition:  condition,
		Semicolon:  semicolon,
	}
}

func (p *Parser) parseBreakStmt() *ast.BreakStmt {
	breakToken := p.advance()
	semicolon := p.consume(token.SEMICOLON, "expected ';'")
	return &ast.BreakStmt{
		BreakToken: breakToken,
		Semicolon:  semicolon,
	}
}

func (p *Parser) parseContinueStmt() *ast.ContinueStmt {
	continueToken := p.advance()
	semicolon := p.consume(token.SEMICOLON, "expected ';'")
	return &ast.ContinueStmt{
		ContinueToken: continueToken,
		Semicolon:     semicolon,
	}
}

func (p *Parser) parseReturnStmt() *ast.ReturnStmt {
	returnToken := p.advance()

	var values []ast.Expression
	if !p.check(token.SEMICOLON) {
		values = append(values, p.parseExpression())
		for p.match(token.COMMA) {
			values = append(values, p.parseExpression())
		}
	}

	semicolon := p.consume(token.SEMICOLON, "expected ';'")

	return &ast.ReturnStmt{
		ReturnToken: returnToken,
		Values:      values,
		Semicolon:   semicolon,
	}
}

func (p *Parser) parseTryStmt() *ast.TryStmt {
	tryToken := p.advance()
	tryBlock := p.parseBlock()

	var catches []*ast.CatchClause
	for p.check(token.CATCH) {
		catchToken := p.advance()
		p.consume(token.LPAREN, "expected '('")
		exType := p.parseType()
		varToken := p.consume(token.VARIABLE, "expected variable")
		// 安全检查：防止空 token 导致 panic
		varNameStr := ""
		if len(varToken.Literal) > 0 {
			varNameStr = varToken.Literal[1:]
		}
		varName := &ast.Variable{Token: varToken, Name: varNameStr}
		p.consume(token.RPAREN, "expected ')'")
		body := p.parseBlock()

		catches = append(catches, &ast.CatchClause{
			CatchToken: catchToken,
			Type:       exType,
			Variable:   varName,
			Body:       body,
		})
	}

	var finally *ast.FinallyClause
	if p.check(token.FINALLY) {
		finallyToken := p.advance()
		body := p.parseBlock()
		finally = &ast.FinallyClause{
			FinallyToken: finallyToken,
			Body:         body,
		}
	}

	return &ast.TryStmt{
		TryToken: tryToken,
		Try:      tryBlock,
		Catches:  catches,
		Finally:  finally,
	}
}

func (p *Parser) parseThrowStmt() *ast.ThrowStmt {
	throwToken := p.advance()
	exception := p.parseExpression()
	semicolon := p.consume(token.SEMICOLON, "expected ';'")

	return &ast.ThrowStmt{
		ThrowToken: throwToken,
		Exception:  exception,
		Semicolon:  semicolon,
	}
}

func (p *Parser) parseEchoStmt() *ast.EchoStmt {
	echoToken := p.advance()
	value := p.parseExpression()
	semicolon := p.consume(token.SEMICOLON, "expected ';'")

	return &ast.EchoStmt{
		EchoToken: echoToken,
		Value:     value,
		Semicolon: semicolon,
	}
}

// ============================================================================
// 声明解析
// ============================================================================

func (p *Parser) parseDeclaration() ast.Declaration {
	// 解析注解
	var annotations []*ast.Annotation
	for p.check(token.AT) {
		annotations = append(annotations, p.parseAnnotation())
	}

	// 解析访问修饰符
	visibility := ast.VisibilityDefault
	if p.match(token.PUBLIC) {
		visibility = ast.VisibilityPublic
	} else if p.match(token.PROTECTED) {
		visibility = ast.VisibilityProtected
	} else if p.match(token.PRIVATE) {
		visibility = ast.VisibilityPrivate
	}

	// 检查是否是抽象类
	isAbstract := p.match(token.ABSTRACT)

	switch p.peek().Type {
	case token.CLASS:
		return p.parseClass(annotations, visibility, isAbstract)
	case token.INTERFACE:
		return p.parseInterface(annotations, visibility)
	case token.ENUM:
		return p.parseEnum()
	default:
		p.error(i18n.T(i18n.ErrExpectedToken, "'class', 'interface' or 'enum'"))
		return nil
	}
}

func (p *Parser) parseAnnotation() *ast.Annotation {
	atToken := p.advance()
	nameToken := p.consume(token.IDENT, "expected annotation name")
	name := &ast.Identifier{Token: nameToken, Name: nameToken.Literal}

	var lparen, rparen token.Token
	var args []ast.Expression

	if p.check(token.LPAREN) {
		lparen = p.advance()
		if !p.check(token.RPAREN) {
			args = append(args, p.parseExpression())
			for p.match(token.COMMA) {
				args = append(args, p.parseExpression())
			}
		}
		rparen = p.consume(token.RPAREN, "expected ')'")
	}

	return &ast.Annotation{
		AtToken: atToken,
		Name:    name,
		LParen:  lparen,
		Args:    args,
		RParen:  rparen,
	}
}

func (p *Parser) parseClass(annotations []*ast.Annotation, visibility ast.Visibility, isAbstract bool) *ast.ClassDecl {
	classToken := p.advance()
	nameToken := p.consume(token.IDENT, "expected class name")
	name := &ast.Identifier{Token: nameToken, Name: nameToken.Literal}

	// extends
	var extends *ast.Identifier
	if p.match(token.EXTENDS) {
		extendsToken := p.consume(token.IDENT, "expected parent class name")
		extends = &ast.Identifier{Token: extendsToken, Name: extendsToken.Literal}
	}

	// implements
	var implements []*ast.Identifier
	if p.match(token.IMPLEMENTS) {
		implToken := p.consume(token.IDENT, "expected interface name")
		implements = append(implements, &ast.Identifier{Token: implToken, Name: implToken.Literal})
		for p.match(token.COMMA) {
			implToken = p.consume(token.IDENT, "expected interface name")
			implements = append(implements, &ast.Identifier{Token: implToken, Name: implToken.Literal})
		}
	}

	lbrace := p.consume(token.LBRACE, "expected '{'")

	var constants []*ast.ConstDecl
	var properties []*ast.PropertyDecl
	var methods []*ast.MethodDecl

	for !p.check(token.RBRACE) && !p.isAtEnd() {
		member := p.parseClassMember()
		switch m := member.(type) {
		case *ast.ConstDecl:
			constants = append(constants, m)
		case *ast.PropertyDecl:
			properties = append(properties, m)
		case *ast.MethodDecl:
			methods = append(methods, m)
		}
	}

	rbrace := p.consume(token.RBRACE, "expected '}'")

	return &ast.ClassDecl{
		Annotations: annotations,
		Visibility:  visibility,
		Abstract:    isAbstract,
		ClassToken:  classToken,
		Name:        name,
		Extends:     extends,
		Implements:  implements,
		LBrace:      lbrace,
		Constants:   constants,
		Properties:  properties,
		Methods:     methods,
		RBrace:      rbrace,
	}
}

func (p *Parser) parseClassMember() ast.Declaration {
	// 解析注解
	var annotations []*ast.Annotation
	for p.check(token.AT) {
		annotations = append(annotations, p.parseAnnotation())
	}

	// 解析访问修饰符
	visibility := ast.VisibilityDefault
	if p.match(token.PUBLIC) {
		visibility = ast.VisibilityPublic
	} else if p.match(token.PROTECTED) {
		visibility = ast.VisibilityProtected
	} else if p.match(token.PRIVATE) {
		visibility = ast.VisibilityPrivate
	}

	isAbstract := p.match(token.ABSTRACT)
	isStatic := p.match(token.STATIC)

	// 如果之前匹配了 static，再检查 abstract
	if !isAbstract {
		isAbstract = p.match(token.ABSTRACT)
	}

	// const
	if p.check(token.CONST) {
		return p.parseConstDecl(annotations, visibility)
	}

	// function
	if p.check(token.FUNCTION) {
		return p.parseMethodDecl(annotations, visibility, isStatic, isAbstract)
	}

	// property (类型 $变量名)
	return p.parsePropertyDecl(annotations, visibility, isStatic)
}

func (p *Parser) parseConstDecl(annotations []*ast.Annotation, visibility ast.Visibility) *ast.ConstDecl {
	constToken := p.advance()
	varType := p.parseType()
	nameToken := p.consume(token.IDENT, "expected constant name")
	name := &ast.Identifier{Token: nameToken, Name: nameToken.Literal}
	assign := p.consume(token.ASSIGN, "expected '='")
	value := p.parseExpression()
	semicolon := p.consume(token.SEMICOLON, "expected ';'")

	return &ast.ConstDecl{
		Annotations: annotations,
		Visibility:  visibility,
		ConstToken:  constToken,
		Type:        varType,
		Name:        name,
		Assign:      assign,
		Value:       value,
		Semicolon:   semicolon,
	}
}

func (p *Parser) parsePropertyDecl(annotations []*ast.Annotation, visibility ast.Visibility, isStatic bool) *ast.PropertyDecl {
	varType := p.parseType()
	varToken := p.consume(token.VARIABLE, "expected variable name")
	// 安全检查：防止空 token 导致 panic
	nameStr := ""
	if len(varToken.Literal) > 0 {
		nameStr = varToken.Literal[1:]
	}
	name := &ast.Variable{Token: varToken, Name: nameStr}

	var assign token.Token
	var value ast.Expression
	if p.check(token.ASSIGN) {
		assign = p.advance()
		value = p.parseExpression()
	}

	semicolon := p.consume(token.SEMICOLON, "expected ';'")

	return &ast.PropertyDecl{
		Annotations: annotations,
		Visibility:  visibility,
		Static:      isStatic,
		Type:        varType,
		Name:        name,
		Assign:      assign,
		Value:       value,
		Semicolon:   semicolon,
	}
}

func (p *Parser) parseMethodDecl(annotations []*ast.Annotation, visibility ast.Visibility, isStatic, isAbstract bool) *ast.MethodDecl {
	funcToken := p.advance()
	nameToken := p.consume(token.IDENT, "expected method name")
	name := &ast.Identifier{Token: nameToken, Name: nameToken.Literal}

	lparen := p.consume(token.LPAREN, "expected '('")
	var params []*ast.Parameter
	if !p.check(token.RPAREN) {
		params = append(params, p.parseParameter())
		for p.match(token.COMMA) {
			params = append(params, p.parseParameter())
		}
	}
	rparen := p.consume(token.RPAREN, "expected ')'")

	var returnType ast.TypeNode
	if p.match(token.COLON) {
		returnType = p.parseReturnType()
	}

	var body *ast.BlockStmt
	if isAbstract {
		p.consume(token.SEMICOLON, "expected ';' after abstract method")
	} else {
		body = p.parseBlock()
	}

	return &ast.MethodDecl{
		Annotations: annotations,
		Visibility:  visibility,
		Static:      isStatic,
		Abstract:    isAbstract,
		FuncToken:   funcToken,
		Name:        name,
		LParen:      lparen,
		Parameters:  params,
		RParen:      rparen,
		ReturnType:  returnType,
		Body:        body,
	}
}

func (p *Parser) parseInterface(annotations []*ast.Annotation, visibility ast.Visibility) *ast.InterfaceDecl {
	interfaceToken := p.advance()
	nameToken := p.consume(token.IDENT, "expected interface name")
	name := &ast.Identifier{Token: nameToken, Name: nameToken.Literal}

	// extends (接口可以继承多个接口)
	var extends []*ast.Identifier
	if p.match(token.EXTENDS) {
		extToken := p.consume(token.IDENT, "expected interface name")
		extends = append(extends, &ast.Identifier{Token: extToken, Name: extToken.Literal})
		for p.match(token.COMMA) {
			extToken = p.consume(token.IDENT, "expected interface name")
			extends = append(extends, &ast.Identifier{Token: extToken, Name: extToken.Literal})
		}
	}

	lbrace := p.consume(token.LBRACE, "expected '{'")

	var methods []*ast.MethodDecl
	for !p.check(token.RBRACE) && !p.isAtEnd() {
		// 接口中的方法都是抽象的
		visibility := ast.VisibilityPublic
		if p.match(token.PUBLIC) {
			visibility = ast.VisibilityPublic
		}

		funcToken := p.consume(token.FUNCTION, "expected 'function'")
		nameToken := p.consume(token.IDENT, "expected method name")
		methodName := &ast.Identifier{Token: nameToken, Name: nameToken.Literal}

		lparen := p.consume(token.LPAREN, "expected '('")
		var params []*ast.Parameter
		if !p.check(token.RPAREN) {
			params = append(params, p.parseParameter())
			for p.match(token.COMMA) {
				params = append(params, p.parseParameter())
			}
		}
		rparen := p.consume(token.RPAREN, "expected ')'")

		var returnType ast.TypeNode
		if p.match(token.COLON) {
			returnType = p.parseReturnType()
		}

		p.consume(token.SEMICOLON, "expected ';'")

		methods = append(methods, &ast.MethodDecl{
			Visibility: visibility,
			Abstract:   true,
			FuncToken:  funcToken,
			Name:       methodName,
			LParen:     lparen,
			Parameters: params,
			RParen:     rparen,
			ReturnType: returnType,
		})
	}

	rbrace := p.consume(token.RBRACE, "expected '}'")

	return &ast.InterfaceDecl{
		Annotations:    annotations,
		Visibility:     visibility,
		InterfaceToken: interfaceToken,
		Name:           name,
		Extends:        extends,
		LBrace:         lbrace,
		Methods:        methods,
		RBrace:         rbrace,
	}
}

func (p *Parser) parseEnum() *ast.EnumDecl {
	enumToken := p.advance()
	nameToken := p.consume(token.IDENT, "expected enum name")
	name := &ast.Identifier{Token: nameToken, Name: nameToken.Literal}

	// 可选的基础类型 (: int 或 : string)
	var baseType ast.TypeNode
	if p.match(token.COLON) {
		baseType = p.parseType()
	}

	lbrace := p.consume(token.LBRACE, "expected '{'")

	var cases []*ast.EnumCase
	for !p.check(token.RBRACE) && !p.isAtEnd() {
		// 解析 case CaseName 或 case CaseName = value
		p.consume(token.CASE, "expected 'case'")
		caseNameToken := p.consume(token.IDENT, "expected case name")
		caseName := &ast.Identifier{Token: caseNameToken, Name: caseNameToken.Literal}

		var value ast.Expression
		if p.match(token.ASSIGN) {
			value = p.parseExpression()
		}

		cases = append(cases, &ast.EnumCase{
			Name:  caseName,
			Value: value,
		})

		// 分号是可选的
		p.match(token.SEMICOLON)
	}

	rbrace := p.consume(token.RBRACE, "expected '}'")

	return &ast.EnumDecl{
		EnumToken: enumToken,
		Name:      name,
		Type:      baseType,
		LBrace:    lbrace,
		Cases:     cases,
		RBrace:    rbrace,
	}
}

func (p *Parser) parseNamespace() *ast.NamespaceDecl {
	nsToken := p.advance()

	var parts []string
	parts = append(parts, p.consume(token.IDENT, "expected namespace name").Literal)
	for p.match(token.DOT) {
		parts = append(parts, p.consume(token.IDENT, "expected namespace name").Literal)
	}

	return &ast.NamespaceDecl{
		NamespaceToken: nsToken,
		Name:           strings.Join(parts, "."),
	}
}

func (p *Parser) parseUse() *ast.UseDecl {
	useToken := p.advance()

	var parts []string
	parts = append(parts, p.consume(token.IDENT, "expected import path").Literal)
	for p.match(token.DOT) {
		parts = append(parts, p.consume(token.IDENT, "expected import path").Literal)
	}

	var alias *ast.Identifier
	if p.match(token.AS) {
		aliasToken := p.consume(token.IDENT, "expected alias name")
		alias = &ast.Identifier{Token: aliasToken, Name: aliasToken.Literal}
	}

	p.consume(token.SEMICOLON, "expected ';'")

	return &ast.UseDecl{
		UseToken: useToken,
		Path:     strings.Join(parts, "."),
		Alias:    alias,
	}
}

// parseInterpStringParts 解析插值字符串的各个部分
func (p *Parser) parseInterpStringParts(tok token.Token) []ast.Expression {
	var parts []ast.Expression
	str := tok.Value.(string)

	i := 0
	start := 0
	for i < len(str) {
		if str[i] == '{' && i+1 < len(str) && str[i+1] == '$' {
			// 保存前面的普通文本部分
			if i > start {
				parts = append(parts, &ast.StringLiteral{
					Token: tok,
					Value: str[start:i],
				})
			}

			// 解析变量名 {$name}
			i += 2 // 跳过 {$
			varStart := i
			for i < len(str) && str[i] != '}' {
				i++
			}
			varName := str[varStart:i]
			if i < len(str) {
				i++ // 跳过 }
			}

			parts = append(parts, &ast.Variable{
				Token: tok,
				Name:  varName,
			})
			start = i
		} else {
			i++
		}
	}

	// 保存最后的普通文本部分
	if start < len(str) {
		parts = append(parts, &ast.StringLiteral{
			Token: tok,
			Value: str[start:],
		})
	}

	return parts
}
