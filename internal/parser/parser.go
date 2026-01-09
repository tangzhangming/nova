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
	lexer      *lexer.Lexer
	tokens     []token.Token
	current    int
	errors     []Error
	filename   string
	panicMode  bool // 错误恢复模式标志，用于避免级联报错
	exprDepth  int  // 表达式解析深度，防止栈溢出
}

// maxExprDepth 最大表达式嵌套深度，防止栈溢出
const maxExprDepth = 200

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
		p.panicMode = false
		ns := p.parseNamespace()
		if p.panicMode {
			p.synchronize()
		} else if ns != nil {
			file.Namespace = ns
		}
	}

	// 解析 use 声明
	for p.check(token.USE) {
		p.panicMode = false
		use := p.parseUse()
		if p.panicMode {
			p.synchronize()
		} else if use != nil {
			file.Uses = append(file.Uses, use)
		}
	}

	// 解析声明和语句
	for !p.isAtEnd() {
		p.panicMode = false // 每次迭代重置 panicMode

		if p.checkAny(token.CLASS, token.INTERFACE, token.ENUM, token.ABSTRACT, token.FINAL, token.PUBLIC,
			token.PROTECTED, token.PRIVATE, token.AT, token.TYPE) {
			decl := p.parseDeclaration()
			if p.panicMode {
				p.synchronize()
				continue
			}
			if decl != nil {
				file.Declarations = append(file.Declarations, decl)
			}
		} else {
			// 顶层语句 (入口文件)
			stmt := p.parseStatement()
			if p.panicMode {
				p.synchronize()
				continue
			}
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

func (p *Parser) peekNext() token.Token {
	if p.current+1 >= len(p.tokens) {
		return p.tokens[len(p.tokens)-1] // 返回EOF
	}
	return p.tokens[p.current+1]
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
	p.panicMode = true
	return token.Token{} // 返回零值，调用方应检查 panicMode
}

// maxParseErrors 最大错误数量限制，防止错误爆炸
const maxParseErrors = 50

func (p *Parser) error(message string) {
	// panicMode 下跳过后续错误，避免级联报错
	if p.panicMode {
		return
	}

	pos := p.peek().Pos

	// 避免在同一位置重复报错
	if len(p.errors) > 0 {
		last := p.errors[len(p.errors)-1]
		if last.Pos.Line == pos.Line && last.Pos.Column == pos.Column {
			return
		}
	}

	// 检查是否超过最大错误数量
	if len(p.errors) >= maxParseErrors {
		p.errors = append(p.errors, Error{
			Pos:     pos,
			Message: "too many errors, aborting",
		})
		p.panicMode = true // 设置 panicMode 而不是 panic
		return
	}

	p.errors = append(p.errors, Error{
		Pos:     pos,
		Message: message,
	})
}

func (p *Parser) synchronize() {
	p.advance()

	for !p.isAtEnd() {
		// 分号后是安全点
		if p.previous().Type == token.SEMICOLON {
			return
		}
		// 右大括号后通常是安全点
		if p.previous().Type == token.RBRACE {
			return
		}

		// 新语句/声明的开始是安全的同步点
		switch p.peek().Type {
		case token.CLASS, token.INTERFACE, token.ENUM, token.TYPE,
			token.ABSTRACT, token.FINAL, token.PUBLIC, token.PROTECTED, token.PRIVATE,
			token.FUNCTION, token.IF, token.FOR, token.FOREACH, token.WHILE, token.DO,
			token.RETURN, token.TRY, token.THROW, token.BREAK, token.CONTINUE,
			token.NAMESPACE, token.USE, token.AT, token.ECHO, token.SWITCH:
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
		token.BOOL_TYPE, token.STRING_TYPE, token.VOID, token.UNKNOWN, token.DYNAMIC):
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
		nameToken := p.advance()
		baseType = &ast.ClassType{
			Name: nameToken,
		}
		// 检查是否是泛型类型 List<int>
		if p.check(token.LT) {
			langle := p.advance() // 消费 <
			var typeArgs []ast.TypeNode
			// 解析第一个类型参数
			arg := p.parseType()
			if arg != nil {
				typeArgs = append(typeArgs, arg)
			}
			// 解析剩余的类型参数
			for p.match(token.COMMA) {
				arg = p.parseType()
				if arg != nil {
					typeArgs = append(typeArgs, arg)
				}
			}
			rangle := p.consume(token.GT, "expected '>' after type arguments")
			baseType = &ast.GenericType{
				BaseType: &ast.ClassType{Name: nameToken},
				LAngle:   langle,
				TypeArgs: typeArgs,
				RAngle:   rangle,
			}
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

// parseTypeParameters 解析泛型类型参数声明 <T, K extends Comparable<K>>
// 用于类、接口、方法声明
func (p *Parser) parseTypeParameters() []*ast.TypeParameter {
	if !p.check(token.LT) {
		return nil
	}
	p.advance() // 消费 <

	var params []*ast.TypeParameter

	// 解析第一个类型参数
	param := p.parseTypeParameter()
	if param != nil {
		params = append(params, param)
	}

	// 解析剩余的类型参数
	for p.match(token.COMMA) {
		param = p.parseTypeParameter()
		if param != nil {
			params = append(params, param)
		}
	}

	p.consume(token.GT, "expected '>' after type parameters")
	return params
}

// parseTypeParameter 解析单个类型参数 T 或 T extends Comparable<T> implements IComparable, ISerializable
func (p *Parser) parseTypeParameter() *ast.TypeParameter {
	nameToken := p.consume(token.IDENT, "expected type parameter name")
	name := &ast.Identifier{Token: nameToken, Name: nameToken.Literal}

	var constraint ast.TypeNode
	// 检查是否有约束 extends
	if p.match(token.EXTENDS) {
		constraint = p.parseType()
	}

	var implementsTypes []ast.TypeNode
	// 检查是否有 implements 约束
	if p.match(token.IMPLEMENTS) {
		implType := p.parseType()
		if implType != nil {
			implementsTypes = append(implementsTypes, implType)
		}
		for p.match(token.COMMA) {
			implType = p.parseType()
			if implType != nil {
				implementsTypes = append(implementsTypes, implType)
			}
		}
	}

	return &ast.TypeParameter{
		Name:            name,
		Constraint:      constraint,
		ImplementsTypes: implementsTypes,
	}
}

// parseTypeArguments 解析泛型类型参数列表 <int, string>
// 用于泛型类型实例化
func (p *Parser) parseTypeArguments() []ast.TypeNode {
	if !p.check(token.LT) {
		return nil
	}
	p.advance() // 消费 <

	var args []ast.TypeNode

	// 解析第一个类型参数
	arg := p.parseType()
	if arg != nil {
		args = append(args, arg)
	}

	// 解析剩余的类型参数
	for p.match(token.COMMA) {
		arg = p.parseType()
		if arg != nil {
			args = append(args, arg)
		}
	}

	p.consume(token.GT, "expected '>' after type arguments")
	return args
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
	PREC_COALESCE   // ?? (空合并)
	PREC_OR         // ||
	PREC_AND        // &&
	PREC_BIT_OR     // |
	PREC_BIT_XOR    // ^
	PREC_BIT_AND    // &
	PREC_EQUALITY   // ==, !=
	PREC_COMPARISON // <, >, <=, >=
	PREC_IS         // is (类型检查)
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
	case token.IS:
		return PREC_IS
	case token.AS, token.AS_SAFE:
		return PREC_CAST
	case token.LEFT_SHIFT, token.RIGHT_SHIFT:
		return PREC_SHIFT
	case token.PLUS, token.MINUS:
		return PREC_TERM
	case token.STAR, token.SLASH, token.PERCENT:
		return PREC_FACTOR
	case token.NULL_COALESCE:
		return PREC_COALESCE
	case token.LBRACKET, token.ARROW, token.SAFE_DOT, token.DOUBLE_COLON, token.LPAREN, token.DOT,
		token.INCREMENT, token.DECREMENT, token.NON_NULL_ASSERT:
		// BUG FIX 2026-01-10: 空安全系统完善 - 添加 !! 非空断言操作符优先级
		return PREC_POSTFIX
	default:
		return PREC_NONE
	}
}

func (p *Parser) parseExpression() ast.Expression {
	// 检查递归深度，防止栈溢出
	p.exprDepth++
	if p.exprDepth > maxExprDepth {
		p.error("expression too deeply nested")
		p.panicMode = true
		p.exprDepth--
		return nil
	}
	defer func() { p.exprDepth-- }()

	return p.parsePrecedence(PREC_ASSIGNMENT)
}

func (p *Parser) parsePrecedence(precedence int) ast.Expression {
	left := p.parsePrefixExpr()
	if left == nil {
		return nil
	}

	for precedence <= p.getPrecedence(p.peek().Type) && !p.panicMode {
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
		// PHP 风格万能数组: [1, 2, "name" => "Sola"]
		return p.parseSuperArrayLiteral()
	case token.NOT, token.MINUS, token.BIT_NOT:
		return p.parseUnaryExpr()
	case token.INCREMENT, token.DECREMENT:
		return p.parsePrefixIncDec()
	case token.NEW:
		return p.parseNewExpr()
	case token.FUNCTION:
		return p.parseClosureExpr()
	case token.MATCH:
		return p.parseMatchExpr()
	case token.SWITCH:
		return p.parseSwitchExpr()
	// Go 风格数组字面量: int{1, 2, 3}
	case token.INT_TYPE, token.I8_TYPE, token.I16_TYPE, token.I32_TYPE, token.I64_TYPE,
		token.UINT_TYPE, token.U8_TYPE, token.BYTE_TYPE, token.U16_TYPE, token.U32_TYPE, token.U64_TYPE,
		token.FLOAT_TYPE, token.F32_TYPE, token.F64_TYPE,
		token.BOOL_TYPE, token.STRING_TYPE, token.UNKNOWN, token.DYNAMIC:
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
	case token.IS:
		return p.parseIsExpr(left)
	case token.AS, token.AS_SAFE:
		return p.parseTypeCastExpr(left)
	case token.LBRACKET:
		return p.parseIndexExpr(left)
	case token.ARROW:
		return p.parsePropertyOrMethodAccess(left)
	case token.SAFE_DOT:
		return p.parseSafePropertyOrMethodAccess(left)
	case token.NULL_COALESCE:
		return p.parseNullCoalesceExpr(left)
	case token.DOUBLE_COLON:
		return p.parseStaticAccess(left)
	case token.LPAREN:
		return p.parseCallExpr(left)
	case token.DOT:
		return p.parseDotAccess(left)
	case token.INCREMENT, token.DECREMENT:
		return p.parsePostfixIncDec(left)
	case token.NON_NULL_ASSERT:
		// BUG FIX 2026-01-10: 空安全系统完善 - 解析非空断言表达式 (expr!!)
		return p.parseNonNullAssertExpr(left)
	default:
		return left
	}
}

// parseIsExpr 解析类型检查表达式 ($x is string, $obj is MyClass)
// 用于类型收窄：在 if($x is string) 分支内，$x 的类型被收窄为 string
func (p *Parser) parseIsExpr(left ast.Expression) ast.Expression {
	isToken := p.advance() // 消费 is
	typeName := p.parseType()
	
	return &ast.IsExpr{
		Expr:     left,
		IsToken:  isToken,
		Negated:  false,
		TypeName: typeName,
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
			token.VOID, token.UNKNOWN, token.DYNAMIC, token.IDENT, token.QUESTION) {
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
// parseSuperArrayLiteral 解析 PHP 风格万能数组: [1, 2, "name" => "Sola"]
func (p *Parser) parseSuperArrayLiteral() ast.Expression {
	lbracket := p.advance() // 消费 [

	var elements []ast.SuperArrayElement

	// 解析元素
	if !p.check(token.RBRACKET) {
		elem := p.parseSuperArrayElement()
		elements = append(elements, elem)

		for p.match(token.COMMA) {
			if p.check(token.RBRACKET) {
				break // 允许尾逗号
			}
			elem = p.parseSuperArrayElement()
			elements = append(elements, elem)
		}
	}

	rbracket := p.consume(token.RBRACKET, "expected ']'")

	return &ast.SuperArrayLiteral{
		LBracket: lbracket,
		Elements: elements,
		RBracket: rbracket,
	}
}

// parseSuperArrayElement 解析万能数组元素（可能是值或键值对）
func (p *Parser) parseSuperArrayElement() ast.SuperArrayElement {
	// 先解析一个表达式
	expr := p.parseExpression()

	// 如果下一个是 =>，则这是一个键值对
	if p.match(token.DOUBLE_ARROW) {
		arrow := p.previous()
		value := p.parseExpression()
		return ast.SuperArrayElement{
			Key:   expr,
			Arrow: arrow,
			Value: value,
		}
	}

	// 否则是普通值（自动索引）
	return ast.SuperArrayElement{
		Key:   nil,
		Arrow: token.Token{},
		Value: expr,
	}
}

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
		var namedArgs []*ast.NamedArgument
		hasNamedArg := false
		if !p.check(token.RPAREN) {
			args, namedArgs, hasNamedArg = p.parseCallArgument(args, namedArgs, hasNamedArg)
			for p.match(token.COMMA) {
				args, namedArgs, hasNamedArg = p.parseCallArgument(args, namedArgs, hasNamedArg)
			}
		}
		rparen := p.consume(token.RPAREN, "expected ')'")
		return &ast.MethodCall{
			Object:         left,
			Arrow:          arrow,
			Method:         &ast.Identifier{Token: property, Name: property.Literal},
			LParen:         lparen,
			Arguments:      args,
			NamedArguments: namedArgs,
			RParen:         rparen,
		}
	}

	// 属性访问
	return &ast.PropertyAccess{
		Object:   left,
		Arrow:    arrow,
		Property: &ast.Identifier{Token: property, Name: property.Literal},
	}
}

// parseSafePropertyOrMethodAccess 解析安全属性/方法访问 ($obj?.prop, $obj?.method())
func (p *Parser) parseSafePropertyOrMethodAccess(left ast.Expression) ast.Expression {
	safeDot := p.advance() // 消费 ?.
	property := p.consume(token.IDENT, "expected property name after '?.'")

	if p.check(token.LPAREN) {
		// 安全方法调用
		lparen := p.advance()
		var args []ast.Expression
		var namedArgs []*ast.NamedArgument
		hasNamedArg := false
		if !p.check(token.RPAREN) {
			args, namedArgs, hasNamedArg = p.parseCallArgument(args, namedArgs, hasNamedArg)
			for p.match(token.COMMA) {
				args, namedArgs, hasNamedArg = p.parseCallArgument(args, namedArgs, hasNamedArg)
			}
		}
		rparen := p.consume(token.RPAREN, "expected ')'")
		return &ast.SafeMethodCall{
			Object:         left,
			SafeDot:        safeDot,
			Method:         &ast.Identifier{Token: property, Name: property.Literal},
			LParen:         lparen,
			Arguments:      args,
			NamedArguments: namedArgs,
			RParen:         rparen,
		}
	}

	// 安全属性访问
	return &ast.SafePropertyAccess{
		Object:   left,
		SafeDot:  safeDot,
		Property: &ast.Identifier{Token: property, Name: property.Literal},
	}
}

// parseNullCoalesceExpr 解析空合并表达式 ($a ?? $b)
func (p *Parser) parseNullCoalesceExpr(left ast.Expression) ast.Expression {
	op := p.advance() // 消费 ??
	right := p.parsePrecedence(PREC_COALESCE) // 右结合
	return &ast.NullCoalesceExpr{
		Left:     left,
		Operator: op,
		Right:    right,
	}
}

// parseNonNullAssertExpr 解析非空断言表达式 (expr!!)
// BUG FIX 2026-01-10: 空安全系统完善 - 添加非空断言解析
// 用于断言表达式非空，运行时如果为null则抛出异常
func (p *Parser) parseNonNullAssertExpr(left ast.Expression) ast.Expression {
	op := p.advance() // 消费 !!
	return &ast.NonNullAssertExpr{
		Expr:     left,
		Operator: op,
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
		var namedArgs []*ast.NamedArgument
		hasNamedArg := false
		if !p.check(token.RPAREN) {
			args, namedArgs, hasNamedArg = p.parseCallArgument(args, namedArgs, hasNamedArg)
			for p.match(token.COMMA) {
				args, namedArgs, hasNamedArg = p.parseCallArgument(args, namedArgs, hasNamedArg)
			}
		}
		rparen := p.consume(token.RPAREN, "expected ')'")
		return &ast.MethodCall{
			Object:         left,
			Arrow:          token.Token{Type: token.DOT},
			Method:         &ast.Identifier{Token: property, Name: property.Literal},
			LParen:         lparen,
			Arguments:      args,
			NamedArguments: namedArgs,
			RParen:         rparen,
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
			member = p.parseCallExpr(member)
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
	var namedArgs []*ast.NamedArgument
	hasNamedArg := false

	if !p.check(token.RPAREN) {
		// 解析第一个参数
		args, namedArgs, hasNamedArg = p.parseCallArgument(args, namedArgs, hasNamedArg)

		// 解析剩余参数
		for p.match(token.COMMA) {
			args, namedArgs, hasNamedArg = p.parseCallArgument(args, namedArgs, hasNamedArg)
		}
	}

	rparen := p.consume(token.RPAREN, "expected ')'")
	return &ast.CallExpr{
		Function:       left,
		LParen:         lparen,
		Arguments:      args,
		NamedArguments: namedArgs,
		RParen:         rparen,
	}
}

// parseCallArgument 解析单个调用参数（位置参数或命名参数）
func (p *Parser) parseCallArgument(args []ast.Expression, namedArgs []*ast.NamedArgument, hasNamedArg bool) ([]ast.Expression, []*ast.NamedArgument, bool) {
	// 检查是否是命名参数：IDENT + COLON
	if p.check(token.IDENT) && p.peekNext().Type == token.COLON {
		// 命名参数
		nameToken := p.advance()
		colon := p.advance() // 消费 :
		value := p.parseExpression()

		namedArgs = append(namedArgs, &ast.NamedArgument{
			Name:  &ast.Identifier{Token: nameToken, Name: nameToken.Literal},
			Colon: colon,
			Value: value,
		})
		hasNamedArg = true
	} else {
		// 位置参数
		if hasNamedArg {
			p.error("位置参数不能出现在命名参数之后")
		}
		args = append(args, p.parseExpression())
	}
	return args, namedArgs, hasNamedArg
}

func (p *Parser) parseNewExpr() ast.Expression {
	newToken := p.advance()
	className := p.consume(token.IDENT, "expected class name after 'new'")

	// 解析泛型类型参数 <T, K>
	var typeArgs []ast.TypeNode
	if p.check(token.LT) {
		p.advance() // 消费 <
		// 解析第一个类型参数
		arg := p.parseType()
		if arg != nil {
			typeArgs = append(typeArgs, arg)
		}
		// 解析剩余的类型参数
		for p.match(token.COMMA) {
			arg = p.parseType()
			if arg != nil {
				typeArgs = append(typeArgs, arg)
			}
		}
		p.consume(token.GT, "expected '>' after type arguments")
	}

	lparen := p.consume(token.LPAREN, "expected '(' after class name")
	var args []ast.Expression
	var namedArgs []*ast.NamedArgument
	hasNamedArg := false

	if !p.check(token.RPAREN) {
		args, namedArgs, hasNamedArg = p.parseCallArgument(args, namedArgs, hasNamedArg)
		for p.match(token.COMMA) {
			args, namedArgs, hasNamedArg = p.parseCallArgument(args, namedArgs, hasNamedArg)
		}
	}
	rparen := p.consume(token.RPAREN, "expected ')'")

	return &ast.NewExpr{
		NewToken:       newToken,
		ClassName:      &ast.Identifier{Token: className, Name: className.Literal},
		TypeArgs:       typeArgs,
		LParen:         lparen,
		Arguments:      args,
		NamedArguments: namedArgs,
		RParen:         rparen,
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
	case token.GO:
		return p.parseGoStmt()
	case token.SELECT:
		return p.parseSelectStmt()
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
	// 基础类型关键字
	if p.checkAny(token.INT_TYPE, token.I8_TYPE, token.I16_TYPE, token.I32_TYPE, token.I64_TYPE,
		token.UINT_TYPE, token.U8_TYPE, token.BYTE_TYPE, token.U16_TYPE, token.U32_TYPE, token.U64_TYPE,
		token.FLOAT_TYPE, token.F32_TYPE, token.F64_TYPE,
		token.BOOL_TYPE, token.STRING_TYPE, token.VOID, token.UNKNOWN, token.DYNAMIC,
		token.MAP, token.FUNC_TYPE, token.QUESTION) {
		return true
	}

	// 类名后跟变量: ClassName $var
	if p.check(token.IDENT) && p.lookAhead(1).Type == token.VARIABLE {
		return true
	}

	// 泛型类型: ClassName<...> $var
	// 需要检查 IDENT < 的情况，并且找到匹配的 > 后面是变量
	if p.check(token.IDENT) && p.lookAhead(1).Type == token.LT {
		return p.isGenericTypeStart()
	}

	return false
}

// isGenericTypeStart 检查是否是泛型类型的开始（如 Box<int> $var）
func (p *Parser) isGenericTypeStart() bool {
	// 从当前位置开始，查找匹配的 >
	depth := 0
	i := 2 // 跳过 IDENT 和 <
	for p.current+i < len(p.tokens) {
		tok := p.tokens[p.current+i]
		switch tok.Type {
		case token.LT:
			depth++
		case token.GT:
			if depth == 0 {
				// 找到匹配的 >，检查下一个 token 是否是变量或数组标记
				nextIdx := p.current + i + 1
				if nextIdx < len(p.tokens) {
					next := p.tokens[nextIdx]
					// > $var 或 >[] $var
					if next.Type == token.VARIABLE {
						return true
					}
					if next.Type == token.LBRACKET {
						// 检查是否是数组类型 >[]
						if nextIdx+1 < len(p.tokens) && p.tokens[nextIdx+1].Type == token.RBRACKET {
							if nextIdx+2 < len(p.tokens) && p.tokens[nextIdx+2].Type == token.VARIABLE {
								return true
							}
						}
					}
				}
				return false
			}
			depth--
		case token.EOF, token.SEMICOLON, token.LBRACE, token.RBRACE:
			// 遇到这些 token 说明不是泛型类型声明
			return false
		}
		i++
	}
	return false
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
	if p.panicMode {
		return nil
	}

	var stmts []ast.Statement
	for !p.check(token.RBRACE) && !p.isAtEnd() && !p.panicMode {
		stmt := p.parseStatement()
		if p.panicMode {
			// 块内错误恢复：跳到下一个语句或块结束
			for !p.check(token.RBRACE) && !p.isAtEnd() && !p.checkAny(
				token.IF, token.FOR, token.FOREACH, token.WHILE, token.DO,
				token.RETURN, token.TRY, token.THROW, token.BREAK, token.CONTINUE,
				token.ECHO, token.SWITCH, token.VARIABLE) {
				if p.previous().Type == token.SEMICOLON || p.previous().Type == token.RBRACE {
					break
				}
				p.advance()
			}
			p.panicMode = false
			continue
		}
		if stmt != nil {
			stmts = append(stmts, stmt)
		}
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
	if p.panicMode {
		return nil
	}
	condition := p.parseExpression()
	p.consume(token.RPAREN, "expected ')'")
	if p.panicMode {
		return nil
	}
	then := p.parseBlock()
	if p.panicMode {
		return nil
	}

	var elseIfs []*ast.ElseIfClause
	var elseBlock *ast.BlockStmt

	for p.check(token.ELSEIF) && !p.panicMode {
		elseIfToken := p.advance()
		p.consume(token.LPAREN, "expected '(' after 'elseif'")
		if p.panicMode {
			return nil
		}
		elseIfCond := p.parseExpression()
		p.consume(token.RPAREN, "expected ')'")
		if p.panicMode {
			return nil
		}
		elseIfBody := p.parseBlock()
		if p.panicMode {
			return nil
		}
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
	lparen := p.consume(token.LPAREN, "expected '(' after 'switch'")
	if p.panicMode {
		return nil
	}
	expr := p.parseExpression()
	rparen := p.consume(token.RPAREN, "expected ')'")
	if p.panicMode {
		return nil
	}
	lbrace := p.consume(token.LBRACE, "expected '{'")
	if p.panicMode {
		return nil
	}

	var cases []*ast.SwitchCase
	var defaultClause *ast.SwitchDefaultCase
	// 跟踪是表达式形式还是语句形式（nil=未确定, true=表达式, false=语句）
	var isExprForm *bool

	for !p.check(token.RBRACE) && !p.isAtEnd() && !p.panicMode {
		if p.check(token.CASE) {
			cases = append(cases, p.parseSwitchCase(&isExprForm))
		} else if p.check(token.DEFAULT) {
			defaultClause = p.parseSwitchDefaultCase(&isExprForm)
		} else {
			p.error(i18n.T(i18n.ErrExpectedCaseDefault))
			break
		}
	}

	rbrace := p.consume(token.RBRACE, "expected '}'")

	return &ast.SwitchStmt{
		SwitchToken: switchToken,
		LParen:      lparen,
		Expr:        expr,
		RParen:      rparen,
		LBrace:      lbrace,
		Cases:       cases,
		Default:     defaultClause,
		RBrace:      rbrace,
	}
}

// parseSwitchCase 解析 case 子句（支持多值和两种形式）
func (p *Parser) parseSwitchCase(isExprForm **bool) *ast.SwitchCase {
	caseToken := p.advance() // 消费 case

	// 解析多个值：case 1, 2, 3
	values := []ast.Expression{p.parseExpression()}
	for p.match(token.COMMA) {
		// 如果遇到 => 或 :，说明逗号是分隔case的，不是多值
		if p.check(token.DOUBLE_ARROW) || p.check(token.COLON) {
			break
		}
		values = append(values, p.parseExpression())
	}

	// 检查是 => 还是 :
	if p.check(token.DOUBLE_ARROW) {
		// 表达式形式（=>）
		if *isExprForm != nil && !**isExprForm {
			p.error("不能混合使用 => 和 : 形式，此switch已使用 : 形式")
			return nil
		}
		isTrue := true
		*isExprForm = &isTrue

		arrow := p.advance()
		body := p.parseExpression() // 单行表达式
		p.match(token.COMMA)        // 允许尾逗号

		return &ast.SwitchCase{
			CaseToken: caseToken,
			Values:    values,
			Arrow:     arrow,
			Body:      body, // Expression
		}
	} else {
		// 语句形式（:）
		if *isExprForm != nil && **isExprForm {
			p.error("不能混合使用 => 和 : 形式，此switch已使用 => 形式")
			return nil
		}
		isFalse := false
		*isExprForm = &isFalse

		colon := p.consume(token.COLON, "expected ':' or '=>' after case values")
		if p.panicMode {
			return nil
		}

		// 解析语句块直到遇到 case/default/}
		var body []ast.Statement
		for !p.checkAny(token.CASE, token.DEFAULT, token.RBRACE) && !p.isAtEnd() && !p.panicMode {
			body = append(body, p.parseStatement())
		}

		return &ast.SwitchCase{
			CaseToken: caseToken,
			Values:    values,
			Colon:     colon,
			Body:      body, // []Statement
		}
	}
}

// parseSwitchDefaultCase 解析 default 子句（支持两种形式）
func (p *Parser) parseSwitchDefaultCase(isExprForm **bool) *ast.SwitchDefaultCase {
	defaultToken := p.advance() // 消费 default

	// 检查是 => 还是 :
	if p.check(token.DOUBLE_ARROW) {
		// 表达式形式（=>）
		if *isExprForm != nil && !**isExprForm {
			p.error("不能混合使用 => 和 : 形式，此switch已使用 : 形式")
			return nil
		}
		isTrue := true
		*isExprForm = &isTrue

		arrow := p.advance()
		body := p.parseExpression()

		return &ast.SwitchDefaultCase{
			DefaultToken: defaultToken,
			Arrow:        arrow,
			Body:         body, // Expression
		}
	} else {
		// 语句形式（:）
		if *isExprForm != nil && **isExprForm {
			p.error("不能混合使用 => 和 : 形式，此switch已使用 => 形式")
			return nil
		}
		isFalse := false
		*isExprForm = &isFalse

		colon := p.consume(token.COLON, "expected ':' or '=>' after 'default'")
		if p.panicMode {
			return nil
		}

		// 解析语句块直到遇到 case/}
		var body []ast.Statement
		for !p.checkAny(token.CASE, token.RBRACE) && !p.isAtEnd() && !p.panicMode {
			body = append(body, p.parseStatement())
		}

		return &ast.SwitchDefaultCase{
			DefaultToken: defaultToken,
			Colon:        colon,
			Body:         body, // []Statement
		}
	}
}

// parseSwitchExpr 解析 switch 表达式（返回值形式）
func (p *Parser) parseSwitchExpr() ast.Expression {
	switchToken := p.advance()
	lparen := p.consume(token.LPAREN, "expected '(' after 'switch'")
	if p.panicMode {
		return nil
	}
	expr := p.parseExpression()
	rparen := p.consume(token.RPAREN, "expected ')'")
	if p.panicMode {
		return nil
	}
	lbrace := p.consume(token.LBRACE, "expected '{'")
	if p.panicMode {
		return nil
	}

	var cases []*ast.SwitchCase
	var defaultClause *ast.SwitchDefaultCase
	// switch表达式必须使用 => 形式
	var isExprForm *bool
	isTrue := true
	isExprForm = &isTrue

	for !p.check(token.RBRACE) && !p.isAtEnd() && !p.panicMode {
		if p.check(token.CASE) {
			switchCase := p.parseSwitchCase(&isExprForm)
			if switchCase != nil {
				// 验证是表达式形式
				if _, ok := switchCase.Body.(ast.Expression); !ok {
					p.error("switch表达式的case必须使用 => 形式")
					return nil
				}
				cases = append(cases, switchCase)
			}
		} else if p.check(token.DEFAULT) {
			defaultClause = p.parseSwitchDefaultCase(&isExprForm)
			if defaultClause != nil {
				// 验证是表达式形式
				if _, ok := defaultClause.Body.(ast.Expression); !ok {
					p.error("switch表达式的default必须使用 => 形式")
					return nil
				}
			}
		} else {
			p.error(i18n.T(i18n.ErrExpectedCaseDefault))
			break
		}
	}

	rbrace := p.consume(token.RBRACE, "expected '}'")

	return &ast.SwitchExpr{
		SwitchToken: switchToken,
		LParen:      lparen,
		Expr:        expr,
		RParen:      rparen,
		LBrace:      lbrace,
		Cases:       cases,
		Default:     defaultClause,
		RBrace:      rbrace,
	}
}

func (p *Parser) parseForStmt() *ast.ForStmt {
	forToken := p.advance()
	p.consume(token.LPAREN, "expected '(' after 'for'")
	if p.panicMode {
		return nil
	}

	// 初始化
	var init ast.Statement
	if !p.check(token.SEMICOLON) {
		init = p.parseExprOrVarDeclStmt()
		if p.panicMode {
			return nil
		}
	} else {
		p.advance() // 消费 ;
	}

	// 条件
	var condition ast.Expression
	if !p.check(token.SEMICOLON) {
		condition = p.parseExpression()
	}
	p.consume(token.SEMICOLON, "expected ';' after for condition")
	if p.panicMode {
		return nil
	}

	// 后置表达式
	var post ast.Expression
	if !p.check(token.RPAREN) {
		post = p.parseExpression()
	}
	p.consume(token.RPAREN, "expected ')'")
	if p.panicMode {
		return nil
	}

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
	if p.panicMode {
		return nil
	}

	// 解析 iterable 表达式，但使用 PREC_CAST 优先级，避免把 as 当作类型转换
	// 使用 PREC_CAST 而不是 PREC_CAST - 1，因为 parsePrecedence 的条件是 precedence <= getPrecedence(token)
	// 所以使用 PREC_CAST 时，遇到 as token 会停止（因为 PREC_CAST <= PREC_CAST 是 true，但我们需要停止）
	// 实际上应该使用 PREC_CAST + 1，这样遇到 as 时会停止
	iterable := p.parsePrecedence(PREC_CAST + 1)
	asToken := p.consume(token.AS, "expected 'as'")
	if p.panicMode {
		return nil
	}

	var key *ast.Variable
	valueToken := p.consume(token.VARIABLE, "expected variable")
	if p.panicMode {
		return nil
	}
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
		if p.panicMode {
			return nil
		}
		valueNameStr = ""
		if len(valueToken.Literal) > 0 {
			valueNameStr = valueToken.Literal[1:]
		}
		value = &ast.Variable{Token: valueToken, Name: valueNameStr}
	}

	p.consume(token.RPAREN, "expected ')'")
	if p.panicMode {
		return nil
	}
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
	if p.panicMode {
		return nil
	}
	condition := p.parseExpression()
	p.consume(token.RPAREN, "expected ')'")
	if p.panicMode {
		return nil
	}
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
	if p.panicMode {
		return nil
	}
	whileToken := p.consume(token.WHILE, "expected 'while'")
	if p.panicMode {
		return nil
	}
	p.consume(token.LPAREN, "expected '('")
	if p.panicMode {
		return nil
	}
	condition := p.parseExpression()
	p.consume(token.RPAREN, "expected ')'")
	if p.panicMode {
		return nil
	}
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
	if p.panicMode {
		return nil
	}

	var catches []*ast.CatchClause
	for p.check(token.CATCH) && !p.panicMode {
		catchToken := p.advance()
		p.consume(token.LPAREN, "expected '('")
		if p.panicMode {
			return nil
		}
		exType := p.parseType()
		varToken := p.consume(token.VARIABLE, "expected variable")
		if p.panicMode {
			return nil
		}
		// 安全检查：防止空 token 导致 panic
		varNameStr := ""
		if len(varToken.Literal) > 0 {
			varNameStr = varToken.Literal[1:]
		}
		varName := &ast.Variable{Token: varToken, Name: varNameStr}
		p.consume(token.RPAREN, "expected ')'")
		if p.panicMode {
			return nil
		}
		body := p.parseBlock()
		if p.panicMode {
			return nil
		}

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

// parseGoStmt 解析 go 语句
//
// 语法:
//
//	go <call_expression>;
//
// 示例:
//
//	go processData();
//	go $obj->method();
//	go function() { ... }();
func (p *Parser) parseGoStmt() *ast.GoStmt {
	goToken := p.advance() // 消费 'go'

	// 解析函数调用表达式
	call := p.parseExpression()

	// 验证必须是函数调用
	switch call.(type) {
	case *ast.CallExpr, *ast.MethodCall:
		// 有效的函数调用
	default:
		p.error("go statement requires a function call")
	}

	semicolon := p.consume(token.SEMICOLON, "expected ';' after go statement")

	return &ast.GoStmt{
		GoToken:   goToken,
		Call:      call,
		Semicolon: semicolon,
	}
}

// parseSelectStmt 解析 select 语句
//
// 语法:
//
//	select {
//	    case $v := $ch->receive():
//	        <statements>
//	    case $ch->send($value):
//	        <statements>
//	    default:
//	        <statements>
//	}
func (p *Parser) parseSelectStmt() *ast.SelectStmt {
	selectToken := p.advance() // 消费 'select'

	lbrace := p.consume(token.LBRACE, "expected '{' after select")

	var cases []*ast.SelectCase
	var defaultCase *ast.SelectDefaultCase

	for !p.check(token.RBRACE) && !p.isAtEnd() {
		if p.check(token.CASE) {
			selectCase := p.parseSelectCase()
			cases = append(cases, selectCase)
		} else if p.check(token.DEFAULT) {
			if defaultCase != nil {
				p.error("multiple default cases in select")
			}
			defaultCase = p.parseSelectDefaultCase()
		} else {
			p.error("expected 'case' or 'default' in select")
			p.synchronize()
			break
		}
	}

	rbrace := p.consume(token.RBRACE, "expected '}' after select")

	return &ast.SelectStmt{
		SelectToken: selectToken,
		LBrace:      lbrace,
		Cases:       cases,
		Default:     defaultCase,
		RBrace:      rbrace,
	}
}

// parseSelectCase 解析 select case 分支
//
// 语法:
//
//	case $v := $ch->receive():
//	    <statements>
//	case $ch->send($value):
//	    <statements>
func (p *Parser) parseSelectCase() *ast.SelectCase {
	caseToken := p.advance() // 消费 'case'

	var varNode *ast.Variable
	var operator token.Token
	var comm ast.Expression

	// 检查是否是接收赋值形式: $v := $ch->receive()
	if p.check(token.VARIABLE) {
		// 先尝试看是否是 $var := 形式
		savedPos := p.current
		tok := p.advance()
		varNode = &ast.Variable{Token: tok, Name: tok.Literal[1:]}

		if p.check(token.DECLARE) {
			operator = p.advance() // 消费 ':='
			comm = p.parseExpression()
		} else {
			// 不是赋值形式，回退并解析为普通表达式
			p.current = savedPos
			varNode = nil
			comm = p.parseExpression()
		}
	} else {
		// 解析通道操作表达式（如 $ch->send($value)）
		comm = p.parseExpression()
	}

	colon := p.consume(token.COLON, "expected ':' after case")

	// 解析 case 体
	var body []ast.Statement
	for !p.check(token.CASE) && !p.check(token.DEFAULT) && !p.check(token.RBRACE) && !p.isAtEnd() {
		stmt := p.parseStatement()
		body = append(body, stmt)
	}

	return &ast.SelectCase{
		CaseToken: caseToken,
		Var:       varNode,
		Operator:  operator,
		Comm:      comm,
		Colon:     colon,
		Body:      body,
	}
}

// parseSelectDefaultCase 解析 select default 分支
func (p *Parser) parseSelectDefaultCase() *ast.SelectDefaultCase {
	defaultToken := p.advance() // 消费 'default'

	colon := p.consume(token.COLON, "expected ':' after default")

	// 解析 default 体
	var body []ast.Statement
	for !p.check(token.CASE) && !p.check(token.RBRACE) && !p.isAtEnd() {
		stmt := p.parseStatement()
		body = append(body, stmt)
	}

	return &ast.SelectDefaultCase{
		DefaultToken: defaultToken,
		Colon:        colon,
		Body:         body,
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

	// 检查是否是抽象类或 final 类
	// abstract 和 final 互斥
	isAbstract := p.match(token.ABSTRACT)
	isFinal := false
	if !isAbstract {
		isFinal = p.match(token.FINAL)
	}

	switch p.peek().Type {
	case token.CLASS:
		return p.parseClass(annotations, visibility, isAbstract, isFinal)
	case token.INTERFACE:
		return p.parseInterface(annotations, visibility)
	case token.ENUM:
		return p.parseEnum()
	case token.TYPE:
		return p.parseTypeDeclaration()
	default:
		p.error(i18n.T(i18n.ErrExpectedToken, "'class', 'interface', 'enum' or 'type'"))
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

func (p *Parser) parseClass(annotations []*ast.Annotation, visibility ast.Visibility, isAbstract, isFinal bool) *ast.ClassDecl {
	classToken := p.advance()
	nameToken := p.consume(token.IDENT, "expected class name")
	if p.panicMode {
		return nil
	}
	name := &ast.Identifier{Token: nameToken, Name: nameToken.Literal}

	// 泛型类型参数 <T, K extends Comparable>
	typeParams := p.parseTypeParameters()

	// extends
	var extends *ast.Identifier
	if p.match(token.EXTENDS) {
		extendsToken := p.consume(token.IDENT, "expected parent class name")
		if p.panicMode {
			return nil
		}
		extends = &ast.Identifier{Token: extendsToken, Name: extendsToken.Literal}
	}

	// implements - 支持泛型接口
	var implements []ast.TypeNode
	if p.match(token.IMPLEMENTS) {
		implType := p.parseType()
		if implType != nil {
			implements = append(implements, implType)
		}
		for p.match(token.COMMA) {
			implType = p.parseType()
			if implType != nil {
				implements = append(implements, implType)
			}
		}
	}

	// where 子句 - 支持多重约束
	var whereClause []*ast.TypeParameter
	if p.match(token.WHERE) {
		whereParam := p.parseTypeParameter()
		if whereParam != nil {
			whereClause = append(whereClause, whereParam)
		}
		for p.match(token.COMMA) {
			whereParam = p.parseTypeParameter()
			if whereParam != nil {
				whereClause = append(whereClause, whereParam)
			}
		}
	}

	lbrace := p.consume(token.LBRACE, "expected '{'")
	if p.panicMode {
		return nil
	}

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
		Final:       isFinal,
		ClassToken:  classToken,
		Name:        name,
		TypeParams:  typeParams,
		Extends:     extends,
		Implements:  implements,
		WhereClause: whereClause,
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

	// 修饰符可以按任意顺序出现: abstract, static, final
	// 但 abstract 和 final 互斥
	isAbstract := false
	isStatic := false
	isFinal := false

	for p.checkAny(token.ABSTRACT, token.STATIC, token.FINAL) {
		if p.match(token.ABSTRACT) {
			isAbstract = true
		} else if p.match(token.STATIC) {
			isStatic = true
		} else if p.match(token.FINAL) {
			isFinal = true
		}
	}

	// const
	if p.check(token.CONST) {
		return p.parseConstDecl(annotations, visibility)
	}

	// function
	if p.check(token.FUNCTION) {
		return p.parseMethodDecl(annotations, visibility, isStatic, isAbstract, isFinal)
	}

	// property (类型 $变量名)
	return p.parsePropertyDecl(annotations, visibility, isStatic, isFinal)
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

func (p *Parser) parsePropertyDecl(annotations []*ast.Annotation, visibility ast.Visibility, isStatic, isFinal bool) *ast.PropertyDecl {
	varType := p.parseType()
	varToken := p.consume(token.VARIABLE, "expected variable name")
	// 安全检查：防止空 token 导致 panic
	nameStr := ""
	if len(varToken.Literal) > 0 {
		nameStr = varToken.Literal[1:]
	}
	name := &ast.Variable{Token: varToken, Name: nameStr}

	// 检查属性类型：
	// 1. { get; set; } 或 { get { ... } set { ... } } - 访问器属性
	// 2. => expression - 表达式体属性
	// 3. = expression; - 普通字段带初始值
	// 4. ; - 普通字段

	if p.check(token.LBRACE) {
		// 访问器属性
		accessor := p.parsePropertyAccessor(visibility)
		semicolon := p.consume(token.SEMICOLON, "expected ';' after property accessor")
		return &ast.PropertyDecl{
			Annotations: annotations,
			Visibility:  visibility,
			Static:      isStatic,
			Final:       isFinal,
			Type:        varType,
			Name:        name,
			Accessor:    accessor,
			Semicolon:   semicolon,
		}
	} else if p.check(token.DOUBLE_ARROW) {
		// 表达式体属性
		arrow := p.advance()
		exprBody := p.parseExpression()
		semicolon := p.consume(token.SEMICOLON, "expected ';' after expression-bodied property")
		return &ast.PropertyDecl{
			Annotations: annotations,
			Visibility:  visibility,
			Static:      isStatic,
			Final:       isFinal,
			Type:        varType,
			Name:        name,
			ExprBody:    exprBody,
			Arrow:       arrow,
			Semicolon:   semicolon,
		}
	} else {
		// 普通字段（可能带初始值）
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
			Final:       isFinal,
			Type:        varType,
			Name:        name,
			Assign:      assign,
			Value:       value,
			Semicolon:   semicolon,
		}
	}
}

// parsePropertyAccessor 解析属性访问器 { get; set; } 或 { get { ... } set { ... } }
func (p *Parser) parsePropertyAccessor(defaultVis ast.Visibility) *ast.PropertyAccessor {
	lbrace := p.consume(token.LBRACE, "expected '{' after property name")
	
	var getToken, setToken token.Token
	var getVis, setVis ast.Visibility = defaultVis, defaultVis
	var getBody, setBody *ast.BlockStmt
	var getExpr, setExpr ast.Expression

	// 解析 get 和 set
	for !p.check(token.RBRACE) && !p.isAtEnd() {
		if p.match(token.GET) {
			getToken = p.previous()
			
			// 检查是否有可见性修饰符
			if p.match(token.PUBLIC) {
				getVis = ast.VisibilityPublic
			} else if p.match(token.PROTECTED) {
				getVis = ast.VisibilityProtected
			} else if p.match(token.PRIVATE) {
				getVis = ast.VisibilityPrivate
			}
			
			// 检查是表达式体还是方法体
			if p.check(token.DOUBLE_ARROW) {
				// 表达式体 getter: get => expr;
				p.advance() // 消费 =>
				getExpr = p.parseExpression()
				p.consume(token.SEMICOLON, "expected ';' after getter expression")
			} else if p.check(token.LBRACE) {
				// 方法体 getter: get { ... }
				getBody = p.parseBlock()
			} else {
				// 自动属性: get;
				p.consume(token.SEMICOLON, "expected ';' after 'get'")
			}
		} else if p.match(token.SET) {
			setToken = p.previous()
			
			// 检查是否有可见性修饰符
			if p.match(token.PUBLIC) {
				setVis = ast.VisibilityPublic
			} else if p.match(token.PROTECTED) {
				setVis = ast.VisibilityProtected
			} else if p.match(token.PRIVATE) {
				setVis = ast.VisibilityPrivate
			}
			
			// 检查是表达式体还是方法体
			if p.check(token.DOUBLE_ARROW) {
				// 表达式体 setter: set => expr;
				p.advance() // 消费 =>
				setExpr = p.parseExpression()
				p.consume(token.SEMICOLON, "expected ';' after setter expression")
			} else if p.check(token.LBRACE) {
				// 方法体 setter: set { ... }
				setBody = p.parseBlock()
			} else {
				// 自动属性: set;
				p.consume(token.SEMICOLON, "expected ';' after 'set'")
			}
		} else {
			p.error("expected 'get' or 'set' in property accessor")
			break
		}
	}

	rbrace := p.consume(token.RBRACE, "expected '}' after property accessor")

	return &ast.PropertyAccessor{
		GetToken: getToken,
		SetToken: setToken,
		GetVis:   getVis,
		SetVis:   setVis,
		GetBody:  getBody,
		SetBody:  setBody,
		GetExpr:  getExpr,
		SetExpr:  setExpr,
		LBrace:   lbrace,
		RBrace:   rbrace,
	}
}

func (p *Parser) parseMethodDecl(annotations []*ast.Annotation, visibility ast.Visibility, isStatic, isAbstract, isFinal bool) *ast.MethodDecl {
	funcToken := p.advance()
	nameToken := p.consume(token.IDENT, "expected method name")
	name := &ast.Identifier{Token: nameToken, Name: nameToken.Literal}

	// 泛型类型参数 <T, K extends Comparable>
	typeParams := p.parseTypeParameters()

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
		Final:       isFinal,
		FuncToken:   funcToken,
		Name:        name,
		TypeParams:  typeParams,
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
	if p.panicMode {
		return nil
	}
	name := &ast.Identifier{Token: nameToken, Name: nameToken.Literal}

	// 泛型类型参数 <T, K extends Comparable>
	typeParams := p.parseTypeParameters()

	// extends (接口可以继承多个接口) - 支持泛型接口
	var extends []ast.TypeNode
	if p.match(token.EXTENDS) {
		extType := p.parseType()
		if extType != nil {
			extends = append(extends, extType)
		}
		for p.match(token.COMMA) {
			extType = p.parseType()
			if extType != nil {
				extends = append(extends, extType)
			}
		}
	}

	// where 子句 - 支持多重约束
	var whereClause []*ast.TypeParameter
	if p.match(token.WHERE) {
		whereParam := p.parseTypeParameter()
		if whereParam != nil {
			whereClause = append(whereClause, whereParam)
		}
		for p.match(token.COMMA) {
			whereParam = p.parseTypeParameter()
			if whereParam != nil {
				whereClause = append(whereClause, whereParam)
			}
		}
	}

	lbrace := p.consume(token.LBRACE, "expected '{'")
	if p.panicMode {
		return nil
	}

	var methods []*ast.MethodDecl
	for !p.check(token.RBRACE) && !p.isAtEnd() && !p.panicMode {
		// 接口中的方法都是抽象的
		visibility := ast.VisibilityPublic
		if p.match(token.PUBLIC) {
			visibility = ast.VisibilityPublic
		}

		funcToken := p.consume(token.FUNCTION, "expected 'function'")
		nameToken := p.consume(token.IDENT, "expected method name")
		methodName := &ast.Identifier{Token: nameToken, Name: nameToken.Literal}

		// 方法的泛型类型参数
		methodTypeParams := p.parseTypeParameters()

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
			TypeParams: methodTypeParams,
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
		TypeParams:     typeParams,
		Extends:        extends,
		WhereClause:    whereClause,
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

// parseTypeDeclaration 解析类型声明
// 支持两种语法：
// 1. 类型别名: type StringList = string[]  (与目标类型完全兼容)
// 2. 新类型:   type UserID int             (独立类型，需要显式转换)
func (p *Parser) parseTypeDeclaration() ast.Declaration {
	typeToken := p.advance() // 消费 type
	nameToken := p.consume(token.IDENT, "expected type name")
	name := &ast.Identifier{Token: nameToken, Name: nameToken.Literal}
	
	// 检查是否有 = 符号来区分别名和新类型
	if p.match(token.ASSIGN) {
		// 类型别名: type StringList = string[]
		equals := p.previous()
		aliasType := p.parseType()
		
		return &ast.TypeAliasDecl{
			TypeToken: typeToken,
			Name:      name,
			Equals:    equals,
			AliasType: aliasType,
		}
	} else {
		// 新类型: type UserID int
		baseType := p.parseType()
		
		return &ast.NewTypeDecl{
			TypeToken: typeToken,
			Name:      name,
			BaseType:  baseType,
		}
	}
}

// parseTypeAlias 解析类型别名声明 (保留向后兼容，内部调用 parseTypeDeclaration)
// 已废弃：请使用 parseTypeDeclaration
func (p *Parser) parseTypeAlias() *ast.TypeAliasDecl {
	decl := p.parseTypeDeclaration()
	if alias, ok := decl.(*ast.TypeAliasDecl); ok {
		return alias
	}
	// 如果不是类型别名，报错
	p.error("expected type alias (with '=')")
	return nil
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

// ============================================================================
// 模式匹配表达式解析
// ============================================================================

// parseMatchExpr 解析 match 表达式
// match ($expr) {
//     pattern [if guard] => body,
//     ...
// }
func (p *Parser) parseMatchExpr() *ast.MatchExpr {
	matchToken := p.advance()
	lparen := p.consume(token.LPAREN, "expected '(' after 'match'")
	expr := p.parseExpression()
	rparen := p.consume(token.RPAREN, "expected ')' after match expression")
	lbrace := p.consume(token.LBRACE, "expected '{' after match expression")

	var cases []*ast.MatchCase
	for !p.check(token.RBRACE) && !p.isAtEnd() {
		case_ := p.parseMatchCase()
		if case_ != nil {
			cases = append(cases, case_)
		}
		// 允许逗号分隔（可选）
		if p.match(token.COMMA) {
			continue
		}
	}

	rbrace := p.consume(token.RBRACE, "expected '}' after match cases")

	return &ast.MatchExpr{
		MatchToken: matchToken,
		LParen:     lparen,
		Expr:       expr,
		RParen:     rparen,
		LBrace:     lbrace,
		Cases:      cases,
		RBrace:     rbrace,
	}
}

// parseMatchCase 解析匹配分支
// pattern [if guard] => body
func (p *Parser) parseMatchCase() *ast.MatchCase {
	// 解析模式
	pattern := p.parsePattern()
	if pattern == nil {
		p.error(i18n.T(i18n.ErrExpectedExpression))
		return nil
	}

	// 检查是否有守卫条件
	var guard ast.Expression
	var ifToken token.Token
	if p.match(token.IF) {
		ifToken = p.previous()
		guard = p.parseExpression()
	}

	// 期望 =>
	arrow := p.consume(token.DOUBLE_ARROW, "expected '=>' after pattern")

	// 解析 body（表达式，不是语句）
	body := p.parseExpression()

	return &ast.MatchCase{
		Pattern: pattern,
		Guard:   guard,
		IfToken: ifToken,
		Arrow:   arrow,
		Body:    body,
	}
}

// parsePattern 解析模式（类型模式、值模式、通配符）
func (p *Parser) parsePattern() ast.Pattern {
	// 通配符 _
	if p.check(token.IDENT) && p.peek().Literal == "_" {
		underscore := p.advance()
		return &ast.WildcardPattern{
			Underscore: underscore,
		}
	}

	// 检查是否是类型开始（类型模式）
	if p.isTypeStart() {
		return p.parseTypePattern()
	}

	// 否则是值模式
	return p.parseValuePattern()
}

// parseTypePattern 解析类型模式 (User $u, int $n)
func (p *Parser) parseTypePattern() *ast.TypePattern {
	typeNode := p.parseType()

	// 检查是否有变量绑定
	var varNode *ast.Variable
	if p.check(token.VARIABLE) {
		varToken := p.advance()
		varName := varToken.Literal[1:] // 去掉 $
		varNode = &ast.Variable{
			Token: varToken,
			Name:  varName,
		}
	}

	return &ast.TypePattern{
		Type:     typeNode,
		Variable: varNode,
	}
}

// parseValuePattern 解析值模式 (1, "hello", true, null)
func (p *Parser) parseValuePattern() *ast.ValuePattern {
	// 只解析主表达式（字面量），不解析完整表达式
	// 避免把 => 当作表达式的一部分
	value := p.parsePrefixExpr()
	return &ast.ValuePattern{
		Value: value,
	}
}
