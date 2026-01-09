package ast

import (
	"github.com/tangzhangming/nova/internal/token"
)

// ============================================================================
// AST 节点工厂函数
// ============================================================================
//
// 工厂函数用于从 Arena 分配 AST 节点，提供以下优势：
// - 统一的节点创建方式，方便 Arena 分配
// - 减少手动字段初始化的错误
// - 提高代码可读性
//
// 使用方式：
//   arena := NewArena(64 * 1024)
//   node := arena.NewIntegerLiteral(tok, 42)
//
// PERF: 所有工厂函数都是内联友好的简单函数
//
// ============================================================================

// ============================================================================
// 类型节点工厂
// ============================================================================

// NewSimpleType 创建简单类型节点 (int, string, bool, etc.)
func (a *Arena) NewSimpleType(tok token.Token, name string) *SimpleType {
	node := AllocType[SimpleType](a)
	node.Token = tok
	node.Name = name
	return node
}

// NewNullableType 创建可空类型节点 (?Type)
func (a *Arena) NewNullableType(question token.Token, inner TypeNode) *NullableType {
	node := AllocType[NullableType](a)
	node.Question = question
	node.Inner = inner
	return node
}

// NewArrayType 创建数组类型节点 (string[] 或 string[100])
func (a *Arena) NewArrayType(elemType TypeNode, lbracket token.Token, size Expression, rbracket token.Token) *ArrayType {
	node := AllocType[ArrayType](a)
	node.ElementType = elemType
	node.LBracket = lbracket
	node.Size = size
	node.RBracket = rbracket
	return node
}

// NewMapType 创建映射类型节点 (map[K]V)
func (a *Arena) NewMapType(mapTok token.Token, keyType, valueType TypeNode) *MapType {
	node := AllocType[MapType](a)
	node.MapToken = mapTok
	node.KeyType = keyType
	node.ValueType = valueType
	return node
}

// NewFuncType 创建函数类型节点
func (a *Arena) NewFuncType(funcTok token.Token, params []TypeNode, returnType TypeNode) *FuncType {
	node := AllocType[FuncType](a)
	node.FuncToken = funcTok
	node.Params = params
	node.ReturnType = returnType
	return node
}

// NewTupleType 创建元组类型节点 (用于多返回值)
func (a *Arena) NewTupleType(lparen token.Token, types []TypeNode, rparen token.Token) *TupleType {
	node := AllocType[TupleType](a)
	node.LParen = lparen
	node.Types = types
	node.RParen = rparen
	return node
}

// NewClassType 创建类类型节点
func (a *Arena) NewClassType(name token.Token) *ClassType {
	node := AllocType[ClassType](a)
	node.Name = name
	return node
}

// NewUnionType 创建联合类型节点 (Type1 | Type2)
func (a *Arena) NewUnionType(types []TypeNode) *UnionType {
	node := AllocType[UnionType](a)
	node.Types = types
	return node
}

// NewNullType 创建 null 类型节点
func (a *Arena) NewNullType(tok token.Token) *NullType {
	node := AllocType[NullType](a)
	node.Token = tok
	return node
}

// NewTypeParameter 创建泛型类型参数节点
func (a *Arena) NewTypeParameter(name *Identifier, constraint TypeNode, implementsTypes []TypeNode) *TypeParameter {
	node := AllocType[TypeParameter](a)
	node.Name = name
	node.Constraint = constraint
	node.ImplementsTypes = implementsTypes
	return node
}

// NewGenericType 创建泛型类型实例化节点 (List<int>)
func (a *Arena) NewGenericType(baseType TypeNode, langle token.Token, typeArgs []TypeNode, rangle token.Token) *GenericType {
	node := AllocType[GenericType](a)
	node.BaseType = baseType
	node.LAngle = langle
	node.TypeArgs = typeArgs
	node.RAngle = rangle
	return node
}

// ============================================================================
// 表达式节点工厂
// ============================================================================

// NewIdentifier 创建标识符节点
func (a *Arena) NewIdentifier(tok token.Token, name string) *Identifier {
	node := AllocType[Identifier](a)
	node.Token = tok
	node.Name = name
	return node
}

// NewVariable 创建变量节点 ($name)
func (a *Arena) NewVariable(tok token.Token, name string) *Variable {
	node := AllocType[Variable](a)
	node.Token = tok
	node.Name = name
	return node
}

// NewThisExpr 创建 $this 表达式节点
func (a *Arena) NewThisExpr(tok token.Token) *ThisExpr {
	node := AllocType[ThisExpr](a)
	node.Token = tok
	return node
}

// NewIntegerLiteral 创建整数字面量节点
func (a *Arena) NewIntegerLiteral(tok token.Token, value int64) *IntegerLiteral {
	node := AllocType[IntegerLiteral](a)
	node.Token = tok
	node.Value = value
	return node
}

// NewFloatLiteral 创建浮点数字面量节点
func (a *Arena) NewFloatLiteral(tok token.Token, value float64) *FloatLiteral {
	node := AllocType[FloatLiteral](a)
	node.Token = tok
	node.Value = value
	return node
}

// NewStringLiteral 创建字符串字面量节点
func (a *Arena) NewStringLiteral(tok token.Token, value string) *StringLiteral {
	node := AllocType[StringLiteral](a)
	node.Token = tok
	node.Value = value
	return node
}

// NewInterpStringLiteral 创建插值字符串节点
func (a *Arena) NewInterpStringLiteral(tok token.Token, parts []Expression) *InterpStringLiteral {
	node := AllocType[InterpStringLiteral](a)
	node.Token = tok
	node.Parts = parts
	return node
}

// NewBoolLiteral 创建布尔字面量节点
func (a *Arena) NewBoolLiteral(tok token.Token, value bool) *BoolLiteral {
	node := AllocType[BoolLiteral](a)
	node.Token = tok
	node.Value = value
	return node
}

// NewNullLiteral 创建 null 字面量节点
func (a *Arena) NewNullLiteral(tok token.Token) *NullLiteral {
	node := AllocType[NullLiteral](a)
	node.Token = tok
	return node
}

// NewArrayLiteral 创建数组字面量节点
func (a *Arena) NewArrayLiteral(elemType TypeNode, lbrace token.Token, elements []Expression, rbrace token.Token) *ArrayLiteral {
	node := AllocType[ArrayLiteral](a)
	node.ElementType = elemType
	node.LBrace = lbrace
	node.Elements = elements
	node.RBrace = rbrace
	return node
}

// NewMapLiteral 创建 Map 字面量节点
func (a *Arena) NewMapLiteral(mapTok token.Token, keyType, valueType TypeNode, lbrace token.Token, pairs []MapPair, rbrace token.Token) *MapLiteral {
	node := AllocType[MapLiteral](a)
	node.MapToken = mapTok
	node.KeyType = keyType
	node.ValueType = valueType
	node.LBrace = lbrace
	node.Pairs = pairs
	node.RBrace = rbrace
	return node
}

// NewSuperArrayLiteral 创建 PHP 风格万能数组节点
func (a *Arena) NewSuperArrayLiteral(lbracket token.Token, elements []SuperArrayElement, rbracket token.Token) *SuperArrayLiteral {
	node := AllocType[SuperArrayLiteral](a)
	node.LBracket = lbracket
	node.Elements = elements
	node.RBracket = rbracket
	return node
}

// NewUnaryExpr 创建一元表达式节点
func (a *Arena) NewUnaryExpr(op token.Token, operand Expression, prefix bool) *UnaryExpr {
	node := AllocType[UnaryExpr](a)
	node.Operator = op
	node.Operand = operand
	node.Prefix = prefix
	return node
}

// NewBinaryExpr 创建二元表达式节点
func (a *Arena) NewBinaryExpr(left Expression, op token.Token, right Expression) *BinaryExpr {
	node := AllocType[BinaryExpr](a)
	node.Left = left
	node.Operator = op
	node.Right = right
	return node
}

// NewIsExpr 创建类型检查表达式节点 ($x is Type)
func (a *Arena) NewIsExpr(expr Expression, isTok token.Token, negated bool, typeName TypeNode) *IsExpr {
	node := AllocType[IsExpr](a)
	node.Expr = expr
	node.IsToken = isTok
	node.Negated = negated
	node.TypeName = typeName
	return node
}

// NewTernaryExpr 创建三元表达式节点
func (a *Arena) NewTernaryExpr(cond Expression, question token.Token, then Expression, colon token.Token, elseExpr Expression) *TernaryExpr {
	node := AllocType[TernaryExpr](a)
	node.Condition = cond
	node.Question = question
	node.Then = then
	node.Colon = colon
	node.Else = elseExpr
	return node
}

// NewAssignExpr 创建赋值表达式节点
func (a *Arena) NewAssignExpr(left Expression, op token.Token, right Expression) *AssignExpr {
	node := AllocType[AssignExpr](a)
	node.Left = left
	node.Operator = op
	node.Right = right
	return node
}

// NewNamedArgument 创建命名参数节点
func (a *Arena) NewNamedArgument(name *Identifier, colon token.Token, value Expression) *NamedArgument {
	node := AllocType[NamedArgument](a)
	node.Name = name
	node.Colon = colon
	node.Value = value
	return node
}

// NewCallExpr 创建函数调用表达式节点
func (a *Arena) NewCallExpr(function Expression, lparen token.Token, args []Expression, namedArgs []*NamedArgument, rparen token.Token) *CallExpr {
	node := AllocType[CallExpr](a)
	node.Function = function
	node.LParen = lparen
	node.Arguments = args
	node.NamedArguments = namedArgs
	node.RParen = rparen
	return node
}

// NewIndexExpr 创建索引访问表达式节点
func (a *Arena) NewIndexExpr(object Expression, lbracket token.Token, index Expression, rbracket token.Token) *IndexExpr {
	node := AllocType[IndexExpr](a)
	node.Object = object
	node.LBracket = lbracket
	node.Index = index
	node.RBracket = rbracket
	return node
}

// NewPropertyAccess 创建属性访问表达式节点
func (a *Arena) NewPropertyAccess(object Expression, arrow token.Token, property *Identifier) *PropertyAccess {
	node := AllocType[PropertyAccess](a)
	node.Object = object
	node.Arrow = arrow
	node.Property = property
	return node
}

// NewMethodCall 创建方法调用表达式节点
func (a *Arena) NewMethodCall(object Expression, arrow token.Token, method *Identifier, lparen token.Token, args []Expression, namedArgs []*NamedArgument, rparen token.Token) *MethodCall {
	node := AllocType[MethodCall](a)
	node.Object = object
	node.Arrow = arrow
	node.Method = method
	node.LParen = lparen
	node.Arguments = args
	node.NamedArguments = namedArgs
	node.RParen = rparen
	return node
}

// NewSafePropertyAccess 创建安全属性访问表达式节点 (?.)
func (a *Arena) NewSafePropertyAccess(object Expression, safeDot token.Token, property *Identifier) *SafePropertyAccess {
	node := AllocType[SafePropertyAccess](a)
	node.Object = object
	node.SafeDot = safeDot
	node.Property = property
	return node
}

// NewSafeMethodCall 创建安全方法调用表达式节点 (?.)
func (a *Arena) NewSafeMethodCall(object Expression, safeDot token.Token, method *Identifier, lparen token.Token, args []Expression, namedArgs []*NamedArgument, rparen token.Token) *SafeMethodCall {
	node := AllocType[SafeMethodCall](a)
	node.Object = object
	node.SafeDot = safeDot
	node.Method = method
	node.LParen = lparen
	node.Arguments = args
	node.NamedArguments = namedArgs
	node.RParen = rparen
	return node
}

// NewNullCoalesceExpr 创建空合并表达式节点 (??)
func (a *Arena) NewNullCoalesceExpr(left Expression, op token.Token, right Expression) *NullCoalesceExpr {
	node := AllocType[NullCoalesceExpr](a)
	node.Left = left
	node.Operator = op
	node.Right = right
	return node
}

// NewStaticAccess 创建静态访问表达式节点 (::)
func (a *Arena) NewStaticAccess(class Expression, doubleColon token.Token, member Expression) *StaticAccess {
	node := AllocType[StaticAccess](a)
	node.Class = class
	node.DoubleColon = doubleColon
	node.Member = member
	return node
}

// NewSelfExpr 创建 self 表达式节点
func (a *Arena) NewSelfExpr(tok token.Token) *SelfExpr {
	node := AllocType[SelfExpr](a)
	node.Token = tok
	return node
}

// NewParentExpr 创建 parent 表达式节点
func (a *Arena) NewParentExpr(tok token.Token) *ParentExpr {
	node := AllocType[ParentExpr](a)
	node.Token = tok
	return node
}

// NewNewExpr 创建 new 表达式节点
func (a *Arena) NewNewExpr(newTok token.Token, className *Identifier, typeArgs []TypeNode, lparen token.Token, args []Expression, namedArgs []*NamedArgument, rparen token.Token) *NewExpr {
	node := AllocType[NewExpr](a)
	node.NewToken = newTok
	node.ClassName = className
	node.TypeArgs = typeArgs
	node.LParen = lparen
	node.Arguments = args
	node.NamedArguments = namedArgs
	node.RParen = rparen
	return node
}

// NewClosureExpr 创建闭包表达式节点
func (a *Arena) NewClosureExpr(funcTok token.Token, lparen token.Token, params []*Parameter, rparen token.Token, useVars []*Variable, returnType TypeNode, body *BlockStmt) *ClosureExpr {
	node := AllocType[ClosureExpr](a)
	node.FuncToken = funcTok
	node.LParen = lparen
	node.Parameters = params
	node.RParen = rparen
	node.UseVars = useVars
	node.ReturnType = returnType
	node.Body = body
	return node
}

// NewArrowFuncExpr 创建箭头函数表达式节点
func (a *Arena) NewArrowFuncExpr(lparen token.Token, params []*Parameter, rparen token.Token, returnType TypeNode, arrow token.Token, body Expression) *ArrowFuncExpr {
	node := AllocType[ArrowFuncExpr](a)
	node.LParen = lparen
	node.Parameters = params
	node.RParen = rparen
	node.ReturnType = returnType
	node.Arrow = arrow
	node.Body = body
	return node
}

// NewClassAccessExpr 创建类访问表达式节点 (::class)
func (a *Arena) NewClassAccessExpr(object Expression, doubleColon token.Token, class token.Token) *ClassAccessExpr {
	node := AllocType[ClassAccessExpr](a)
	node.Object = object
	node.DoubleColon = doubleColon
	node.Class = class
	return node
}

// NewTypeCastExpr 创建类型断言表达式节点 (as / as?)
func (a *Arena) NewTypeCastExpr(expr Expression, asTok token.Token, safe bool, targetType TypeNode) *TypeCastExpr {
	node := AllocType[TypeCastExpr](a)
	node.Expr = expr
	node.AsToken = asTok
	node.Safe = safe
	node.TargetType = targetType
	return node
}

// NewMatchExpr 创建模式匹配表达式节点
func (a *Arena) NewMatchExpr(matchTok token.Token, lparen token.Token, expr Expression, rparen token.Token, lbrace token.Token, cases []*MatchCase, rbrace token.Token) *MatchExpr {
	node := AllocType[MatchExpr](a)
	node.MatchToken = matchTok
	node.LParen = lparen
	node.Expr = expr
	node.RParen = rparen
	node.LBrace = lbrace
	node.Cases = cases
	node.RBrace = rbrace
	return node
}

// ============================================================================
// 语句节点工厂
// ============================================================================

// NewExprStmt 创建表达式语句节点
func (a *Arena) NewExprStmt(expr Expression, semicolon token.Token) *ExprStmt {
	node := AllocType[ExprStmt](a)
	node.Expr = expr
	node.Semicolon = semicolon
	return node
}

// NewVarDeclStmt 创建变量声明语句节点
func (a *Arena) NewVarDeclStmt(varType TypeNode, name *Variable, op token.Token, value Expression, semicolon token.Token) *VarDeclStmt {
	node := AllocType[VarDeclStmt](a)
	node.Type = varType
	node.Name = name
	node.Operator = op
	node.Value = value
	node.Semicolon = semicolon
	return node
}

// NewMultiVarDeclStmt 创建多变量声明语句节点
func (a *Arena) NewMultiVarDeclStmt(names []*Variable, op token.Token, value Expression, semicolon token.Token) *MultiVarDeclStmt {
	node := AllocType[MultiVarDeclStmt](a)
	node.Names = names
	node.Operator = op
	node.Value = value
	node.Semicolon = semicolon
	return node
}

// NewBlockStmt 创建代码块语句节点
func (a *Arena) NewBlockStmt(lbrace token.Token, stmts []Statement, rbrace token.Token) *BlockStmt {
	node := AllocType[BlockStmt](a)
	node.LBrace = lbrace
	node.Statements = stmts
	node.RBrace = rbrace
	return node
}

// NewIfStmt 创建 if 语句节点
func (a *Arena) NewIfStmt(ifTok token.Token, cond Expression, then *BlockStmt, elseIfs []*ElseIfClause, elseBlock *BlockStmt) *IfStmt {
	node := AllocType[IfStmt](a)
	node.IfToken = ifTok
	node.Condition = cond
	node.Then = then
	node.ElseIfs = elseIfs
	node.Else = elseBlock
	return node
}

// NewElseIfClause 创建 elseif 子句节点
func (a *Arena) NewElseIfClause(elseIfTok token.Token, cond Expression, body *BlockStmt) *ElseIfClause {
	node := AllocType[ElseIfClause](a)
	node.ElseIfToken = elseIfTok
	node.Condition = cond
	node.Body = body
	return node
}

// NewSwitchStmt 创建 switch 语句节点
func (a *Arena) NewSwitchStmt(switchTok token.Token, expr Expression, lbrace token.Token, cases []*CaseClause, defaultClause *DefaultClause, rbrace token.Token) *SwitchStmt {
	node := AllocType[SwitchStmt](a)
	node.SwitchToken = switchTok
	node.Expr = expr
	node.LBrace = lbrace
	node.Cases = cases
	node.Default = defaultClause
	node.RBrace = rbrace
	return node
}

// NewSwitchCase 创建 switch case 子句节点（支持多值和两种形式）
func (a *Arena) NewSwitchCase(caseTok token.Token, values []Expression, arrow token.Token, colon token.Token, body interface{}) *SwitchCase {
	node := AllocType[SwitchCase](a)
	node.CaseToken = caseTok
	node.Values = values
	node.Arrow = arrow
	node.Colon = colon
	node.Body = body
	return node
}

// NewSwitchDefaultCase 创建 switch default 子句节点
func (a *Arena) NewSwitchDefaultCase(defaultTok token.Token, arrow token.Token, colon token.Token, body interface{}) *SwitchDefaultCase {
	node := AllocType[SwitchDefaultCase](a)
	node.DefaultToken = defaultTok
	node.Arrow = arrow
	node.Colon = colon
	node.Body = body
	return node
}

// NewCaseClause 创建 case 子句节点（兼容旧接口，单值版本）
// Deprecated: 请使用 NewSwitchCase
func (a *Arena) NewCaseClause(caseTok token.Token, value Expression, colon token.Token, body []Statement) *CaseClause {
	node := AllocType[CaseClause](a)
	node.CaseToken = caseTok
	node.Values = []Expression{value}
	node.Colon = colon
	node.Body = body
	return node
}

// NewDefaultClause 创建 default 子句节点（兼容旧接口）
// Deprecated: 请使用 NewSwitchDefaultCase
func (a *Arena) NewDefaultClause(defaultTok token.Token, colon token.Token, body []Statement) *DefaultClause {
	node := AllocType[DefaultClause](a)
	node.DefaultToken = defaultTok
	node.Colon = colon
	node.Body = body
	return node
}

// NewForStmt 创建 for 语句节点
func (a *Arena) NewForStmt(forTok token.Token, init Statement, cond Expression, post Expression, body *BlockStmt) *ForStmt {
	node := AllocType[ForStmt](a)
	node.ForToken = forTok
	node.Init = init
	node.Condition = cond
	node.Post = post
	node.Body = body
	return node
}

// NewForeachStmt 创建 foreach 语句节点
func (a *Arena) NewForeachStmt(foreachTok token.Token, iterable Expression, asTok token.Token, key *Variable, value *Variable, body *BlockStmt) *ForeachStmt {
	node := AllocType[ForeachStmt](a)
	node.ForeachToken = foreachTok
	node.Iterable = iterable
	node.AsToken = asTok
	node.Key = key
	node.Value = value
	node.Body = body
	return node
}

// NewWhileStmt 创建 while 语句节点
func (a *Arena) NewWhileStmt(whileTok token.Token, cond Expression, body *BlockStmt) *WhileStmt {
	node := AllocType[WhileStmt](a)
	node.WhileToken = whileTok
	node.Condition = cond
	node.Body = body
	return node
}

// NewDoWhileStmt 创建 do-while 语句节点
func (a *Arena) NewDoWhileStmt(doTok token.Token, body *BlockStmt, whileTok token.Token, cond Expression, semicolon token.Token) *DoWhileStmt {
	node := AllocType[DoWhileStmt](a)
	node.DoToken = doTok
	node.Body = body
	node.WhileToken = whileTok
	node.Condition = cond
	node.Semicolon = semicolon
	return node
}

// NewBreakStmt 创建 break 语句节点
func (a *Arena) NewBreakStmt(breakTok token.Token, semicolon token.Token) *BreakStmt {
	node := AllocType[BreakStmt](a)
	node.BreakToken = breakTok
	node.Semicolon = semicolon
	return node
}

// NewContinueStmt 创建 continue 语句节点
func (a *Arena) NewContinueStmt(continueTok token.Token, semicolon token.Token) *ContinueStmt {
	node := AllocType[ContinueStmt](a)
	node.ContinueToken = continueTok
	node.Semicolon = semicolon
	return node
}

// NewReturnStmt 创建 return 语句节点
func (a *Arena) NewReturnStmt(returnTok token.Token, values []Expression, semicolon token.Token) *ReturnStmt {
	node := AllocType[ReturnStmt](a)
	node.ReturnToken = returnTok
	node.Values = values
	node.Semicolon = semicolon
	return node
}

// NewTryStmt 创建 try-catch-finally 语句节点
func (a *Arena) NewTryStmt(tryTok token.Token, tryBlock *BlockStmt, catches []*CatchClause, finally *FinallyClause) *TryStmt {
	node := AllocType[TryStmt](a)
	node.TryToken = tryTok
	node.Try = tryBlock
	node.Catches = catches
	node.Finally = finally
	return node
}

// NewCatchClause 创建 catch 子句节点
func (a *Arena) NewCatchClause(catchTok token.Token, exType TypeNode, variable *Variable, body *BlockStmt) *CatchClause {
	node := AllocType[CatchClause](a)
	node.CatchToken = catchTok
	node.Type = exType
	node.Variable = variable
	node.Body = body
	return node
}

// NewFinallyClause 创建 finally 子句节点
func (a *Arena) NewFinallyClause(finallyTok token.Token, body *BlockStmt) *FinallyClause {
	node := AllocType[FinallyClause](a)
	node.FinallyToken = finallyTok
	node.Body = body
	return node
}

// NewThrowStmt 创建 throw 语句节点
func (a *Arena) NewThrowStmt(throwTok token.Token, exception Expression, semicolon token.Token) *ThrowStmt {
	node := AllocType[ThrowStmt](a)
	node.ThrowToken = throwTok
	node.Exception = exception
	node.Semicolon = semicolon
	return node
}

// NewEchoStmt 创建 echo 语句节点
func (a *Arena) NewEchoStmt(echoTok token.Token, value Expression, semicolon token.Token) *EchoStmt {
	node := AllocType[EchoStmt](a)
	node.EchoToken = echoTok
	node.Value = value
	node.Semicolon = semicolon
	return node
}

// NewGoStmt 创建 go 语句节点（启动协程）
func (a *Arena) NewGoStmt(goTok token.Token, call Expression, semicolon token.Token) *GoStmt {
	node := AllocType[GoStmt](a)
	node.GoToken = goTok
	node.Call = call
	node.Semicolon = semicolon
	return node
}

// NewSelectStmt 创建 select 语句节点（多路选择）
func (a *Arena) NewSelectStmt(selectTok token.Token, lbrace token.Token, cases []*SelectCase, defaultCase *SelectDefaultCase, rbrace token.Token) *SelectStmt {
	node := AllocType[SelectStmt](a)
	node.SelectToken = selectTok
	node.LBrace = lbrace
	node.Cases = cases
	node.Default = defaultCase
	node.RBrace = rbrace
	return node
}

// NewSelectCase 创建 select case 分支节点
func (a *Arena) NewSelectCase(caseTok token.Token, varNode *Variable, operator token.Token, comm Expression, colon token.Token, body []Statement) *SelectCase {
	node := AllocType[SelectCase](a)
	node.CaseToken = caseTok
	node.Var = varNode
	node.Operator = operator
	node.Comm = comm
	node.Colon = colon
	node.Body = body
	return node
}

// NewSelectDefaultCase 创建 select default 分支节点
func (a *Arena) NewSelectDefaultCase(defaultTok token.Token, colon token.Token, body []Statement) *SelectDefaultCase {
	node := AllocType[SelectDefaultCase](a)
	node.DefaultToken = defaultTok
	node.Colon = colon
	node.Body = body
	return node
}

// ============================================================================
// 协程 OOP 节点工厂
// ============================================================================

// NewAwaitExpr 创建 await 表达式节点
func (a *Arena) NewAwaitExpr(coroutine Expression, arrow token.Token, awaitTok token.Token, lparen token.Token, timeout Expression, rparen token.Token) *AwaitExpr {
	node := AllocType[AwaitExpr](a)
	node.Coroutine = coroutine
	node.Arrow = arrow
	node.AwaitTok = awaitTok
	node.LParen = lparen
	node.Timeout = timeout
	node.RParen = rparen
	return node
}

// NewCoroutineSpawnExpr 创建 Coroutine::spawn() 表达式节点
func (a *Arena) NewCoroutineSpawnExpr(coroutineTok token.Token, doubleColon token.Token, spawnTok token.Token, lparen token.Token, closure Expression, rparen token.Token) *CoroutineSpawnExpr {
	node := AllocType[CoroutineSpawnExpr](a)
	node.CoroutineTok = coroutineTok
	node.DoubleColon = doubleColon
	node.SpawnTok = spawnTok
	node.LParen = lparen
	node.Closure = closure
	node.RParen = rparen
	return node
}

// NewCoroutineAllExpr 创建 Coroutine::all() 表达式节点
func (a *Arena) NewCoroutineAllExpr(coroutineTok token.Token, doubleColon token.Token, allTok token.Token, lparen token.Token, tasks Expression, rparen token.Token) *CoroutineAllExpr {
	node := AllocType[CoroutineAllExpr](a)
	node.CoroutineTok = coroutineTok
	node.DoubleColon = doubleColon
	node.AllTok = allTok
	node.LParen = lparen
	node.Tasks = tasks
	node.RParen = rparen
	return node
}

// NewCoroutineAnyExpr 创建 Coroutine::any() 表达式节点
func (a *Arena) NewCoroutineAnyExpr(coroutineTok token.Token, doubleColon token.Token, anyTok token.Token, lparen token.Token, tasks Expression, rparen token.Token) *CoroutineAnyExpr {
	node := AllocType[CoroutineAnyExpr](a)
	node.CoroutineTok = coroutineTok
	node.DoubleColon = doubleColon
	node.AnyTok = anyTok
	node.LParen = lparen
	node.Tasks = tasks
	node.RParen = rparen
	return node
}

// NewCoroutineRaceExpr 创建 Coroutine::race() 表达式节点
func (a *Arena) NewCoroutineRaceExpr(coroutineTok token.Token, doubleColon token.Token, raceTok token.Token, lparen token.Token, tasks Expression, rparen token.Token) *CoroutineRaceExpr {
	node := AllocType[CoroutineRaceExpr](a)
	node.CoroutineTok = coroutineTok
	node.DoubleColon = doubleColon
	node.RaceTok = raceTok
	node.LParen = lparen
	node.Tasks = tasks
	node.RParen = rparen
	return node
}

// NewCoroutineDelayExpr 创建 Coroutine::delay() 表达式节点
func (a *Arena) NewCoroutineDelayExpr(coroutineTok token.Token, doubleColon token.Token, delayTok token.Token, lparen token.Token, milliseconds Expression, rparen token.Token) *CoroutineDelayExpr {
	node := AllocType[CoroutineDelayExpr](a)
	node.CoroutineTok = coroutineTok
	node.DoubleColon = doubleColon
	node.DelayTok = delayTok
	node.LParen = lparen
	node.Milliseconds = milliseconds
	node.RParen = rparen
	return node
}

// NewChannelSelectExpr 创建 Channel::select() 表达式节点
func (a *Arena) NewChannelSelectExpr(channelTok token.Token, doubleColon token.Token, selectTok token.Token, lparen token.Token, cases Expression, rparen token.Token) *ChannelSelectExpr {
	node := AllocType[ChannelSelectExpr](a)
	node.ChannelTok = channelTok
	node.DoubleColon = doubleColon
	node.SelectTok = selectTok
	node.LParen = lparen
	node.Cases = cases
	node.RParen = rparen
	return node
}

// ============================================================================
// 声明节点工厂
// ============================================================================

// NewAnnotation 创建注解节点
func (a *Arena) NewAnnotation(atTok token.Token, name *Identifier, lparen token.Token, args []Expression, rparen token.Token) *Annotation {
	node := AllocType[Annotation](a)
	node.AtToken = atTok
	node.Name = name
	node.LParen = lparen
	node.Args = args
	node.RParen = rparen
	return node
}

// NewParameter 创建函数参数节点
func (a *Arena) NewParameter(paramType TypeNode, variadic bool, name *Variable, defaultVal Expression) *Parameter {
	node := AllocType[Parameter](a)
	node.Type = paramType
	node.Variadic = variadic
	node.Name = name
	node.Default = defaultVal
	return node
}

// NewConstDecl 创建常量声明节点
func (a *Arena) NewConstDecl(annotations []*Annotation, visibility Visibility, constTok token.Token, constType TypeNode, name *Identifier, assign token.Token, value Expression, semicolon token.Token) *ConstDecl {
	node := AllocType[ConstDecl](a)
	node.Annotations = annotations
	node.Visibility = visibility
	node.ConstToken = constTok
	node.Type = constType
	node.Name = name
	node.Assign = assign
	node.Value = value
	node.Semicolon = semicolon
	return node
}

// NewPropertyAccessor 创建属性访问器节点
func (a *Arena) NewPropertyAccessor(getTok, setTok token.Token, getVis, setVis Visibility, getBody, setBody *BlockStmt, getExpr, setExpr Expression, lbrace, rbrace token.Token) *PropertyAccessor {
	node := AllocType[PropertyAccessor](a)
	node.GetToken = getTok
	node.SetToken = setTok
	node.GetVis = getVis
	node.SetVis = setVis
	node.GetBody = getBody
	node.SetBody = setBody
	node.GetExpr = getExpr
	node.SetExpr = setExpr
	node.LBrace = lbrace
	node.RBrace = rbrace
	return node
}

// NewPropertyDecl 创建属性声明节点
func (a *Arena) NewPropertyDecl(annotations []*Annotation, visibility Visibility, static, final bool, propType TypeNode, name *Variable, assign token.Token, value Expression, accessor *PropertyAccessor, exprBody Expression, arrow, semicolon token.Token) *PropertyDecl {
	node := AllocType[PropertyDecl](a)
	node.Annotations = annotations
	node.Visibility = visibility
	node.Static = static
	node.Final = final
	node.Type = propType
	node.Name = name
	node.Assign = assign
	node.Value = value
	node.Accessor = accessor
	node.ExprBody = exprBody
	node.Arrow = arrow
	node.Semicolon = semicolon
	return node
}

// NewMethodDecl 创建方法声明节点
func (a *Arena) NewMethodDecl(annotations []*Annotation, visibility Visibility, static, abstract, final bool, funcTok token.Token, name *Identifier, typeParams []*TypeParameter, lparen token.Token, params []*Parameter, rparen token.Token, returnType TypeNode, body *BlockStmt) *MethodDecl {
	node := AllocType[MethodDecl](a)
	node.Annotations = annotations
	node.Visibility = visibility
	node.Static = static
	node.Abstract = abstract
	node.Final = final
	node.FuncToken = funcTok
	node.Name = name
	node.TypeParams = typeParams
	node.LParen = lparen
	node.Parameters = params
	node.RParen = rparen
	node.ReturnType = returnType
	node.Body = body
	return node
}

// NewClassDecl 创建类声明节点
func (a *Arena) NewClassDecl(annotations []*Annotation, visibility Visibility, abstract, final bool, classTok token.Token, name *Identifier, typeParams []*TypeParameter, extends *Identifier, implements []TypeNode, whereClause []*TypeParameter, lbrace token.Token, constants []*ConstDecl, properties []*PropertyDecl, methods []*MethodDecl, rbrace token.Token) *ClassDecl {
	node := AllocType[ClassDecl](a)
	node.Annotations = annotations
	node.Visibility = visibility
	node.Abstract = abstract
	node.Final = final
	node.ClassToken = classTok
	node.Name = name
	node.TypeParams = typeParams
	node.Extends = extends
	node.Implements = implements
	node.WhereClause = whereClause
	node.LBrace = lbrace
	node.Constants = constants
	node.Properties = properties
	node.Methods = methods
	node.RBrace = rbrace
	return node
}

// NewInterfaceDecl 创建接口声明节点
func (a *Arena) NewInterfaceDecl(annotations []*Annotation, visibility Visibility, interfaceTok token.Token, name *Identifier, typeParams []*TypeParameter, extends []TypeNode, whereClause []*TypeParameter, lbrace token.Token, methods []*MethodDecl, rbrace token.Token) *InterfaceDecl {
	node := AllocType[InterfaceDecl](a)
	node.Annotations = annotations
	node.Visibility = visibility
	node.InterfaceToken = interfaceTok
	node.Name = name
	node.TypeParams = typeParams
	node.Extends = extends
	node.WhereClause = whereClause
	node.LBrace = lbrace
	node.Methods = methods
	node.RBrace = rbrace
	return node
}

// NewEnumDecl 创建枚举声明节点
func (a *Arena) NewEnumDecl(enumTok token.Token, name *Identifier, baseType TypeNode, lbrace token.Token, cases []*EnumCase, rbrace token.Token) *EnumDecl {
	node := AllocType[EnumDecl](a)
	node.EnumToken = enumTok
	node.Name = name
	node.Type = baseType
	node.LBrace = lbrace
	node.Cases = cases
	node.RBrace = rbrace
	return node
}

// NewEnumCase 创建枚举成员节点
func (a *Arena) NewEnumCase(name *Identifier, value Expression) *EnumCase {
	node := AllocType[EnumCase](a)
	node.Name = name
	node.Value = value
	return node
}

// NewTypeAliasDecl 创建类型别名声明节点
func (a *Arena) NewTypeAliasDecl(typeTok token.Token, name *Identifier, equals token.Token, aliasType TypeNode) *TypeAliasDecl {
	node := AllocType[TypeAliasDecl](a)
	node.TypeToken = typeTok
	node.Name = name
	node.Equals = equals
	node.AliasType = aliasType
	return node
}

// NewNewTypeDecl 创建新类型声明节点
func (a *Arena) NewNewTypeDecl(typeTok token.Token, name *Identifier, baseType TypeNode) *NewTypeDecl {
	node := AllocType[NewTypeDecl](a)
	node.TypeToken = typeTok
	node.Name = name
	node.BaseType = baseType
	return node
}

// NewNamespaceDecl 创建命名空间声明节点
func (a *Arena) NewNamespaceDecl(nsTok token.Token, name string) *NamespaceDecl {
	node := AllocType[NamespaceDecl](a)
	node.NamespaceToken = nsTok
	node.Name = name
	return node
}

// NewUseDecl 创建 use 声明节点
func (a *Arena) NewUseDecl(useTok token.Token, path string, alias *Identifier) *UseDecl {
	node := AllocType[UseDecl](a)
	node.UseToken = useTok
	node.Path = path
	node.Alias = alias
	return node
}

// NewFile 创建文件节点
func (a *Arena) NewFile(filename string, ns *NamespaceDecl, uses []*UseDecl, decls []Declaration, stmts []Statement) *File {
	node := AllocType[File](a)
	node.Filename = filename
	node.Namespace = ns
	node.Uses = uses
	node.Declarations = decls
	node.Statements = stmts
	return node
}

// ============================================================================
// 模式匹配节点工厂
// ============================================================================

// NewMatchCase 创建模式匹配分支节点
func (a *Arena) NewMatchCase(pattern Pattern, guard Expression, ifTok token.Token, arrow token.Token, body Expression) *MatchCase {
	node := AllocType[MatchCase](a)
	node.Pattern = pattern
	node.Guard = guard
	node.IfToken = ifTok
	node.Arrow = arrow
	node.Body = body
	return node
}

// NewTypePattern 创建类型模式节点
func (a *Arena) NewTypePattern(typeNode TypeNode, variable *Variable) *TypePattern {
	node := AllocType[TypePattern](a)
	node.Type = typeNode
	node.Variable = variable
	return node
}

// NewValuePattern 创建值模式节点
func (a *Arena) NewValuePattern(value Expression) *ValuePattern {
	node := AllocType[ValuePattern](a)
	node.Value = value
	return node
}

// NewWildcardPattern 创建通配符模式节点
func (a *Arena) NewWildcardPattern(underscore token.Token) *WildcardPattern {
	node := AllocType[WildcardPattern](a)
	node.Underscore = underscore
	return node
}
