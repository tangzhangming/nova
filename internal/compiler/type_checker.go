package compiler

import (
	"fmt"
	"strings"

	"github.com/tangzhangming/nova/internal/ast"
	"github.com/tangzhangming/nova/internal/i18n"
	"github.com/tangzhangming/nova/internal/token"
)

// TypeError 类型错误
type TypeError struct {
	Pos     token.Position
	Code    string
	Message string
}

func (e TypeError) Error() string {
	return fmt.Sprintf("%s: %s", e.Pos, e.Message)
}

// TypeWarning 类型警告
type TypeWarning struct {
	Pos     token.Position
	Code    string
	Message string
}

// TypeChecker 独立的类型检查器
type TypeChecker struct {
	symbolTable     *SymbolTable
	currentScope    *TypeScope
	errors          []TypeError
	warnings        []TypeWarning
	
	// 控制流上下文
	cfgBuilder      *CFGBuilder
	currentFunc     *FunctionContext
	
	// 空安全模式
	strictNullCheck bool
	
	// 当前文件
	currentFile     *ast.File
	
	// 当前类名（用于方法内类型推导）
	currentClassName string
}

// TypeScope 类型作用域
type TypeScope struct {
	parent     *TypeScope
	variables  map[string]*VarTypeInfo
	narrowings map[string]string // 类型收窄
}

// VarTypeInfo 变量类型信息
type VarTypeInfo struct {
	Name          string
	DeclaredType  string
	IsNullable    bool
	IsInitialized bool
	DefinedAt     token.Position
}

// FunctionContext 函数上下文
type FunctionContext struct {
	Name       string
	ReturnType string
	IsVoid     bool
	CFG        *CFG
}

// NewTypeChecker 创建类型检查器
func NewTypeChecker(symbolTable *SymbolTable) *TypeChecker {
	return &TypeChecker{
		symbolTable:     symbolTable,
		currentScope:    newTypeScope(nil),
		errors:          make([]TypeError, 0),
		warnings:        make([]TypeWarning, 0),
		strictNullCheck: true,
	}
}

// newTypeScope 创建新的类型作用域
func newTypeScope(parent *TypeScope) *TypeScope {
	return &TypeScope{
		parent:     parent,
		variables:  make(map[string]*VarTypeInfo),
		narrowings: make(map[string]string),
	}
}

// SetStrictNullCheck 设置严格空检查模式
func (tc *TypeChecker) SetStrictNullCheck(enabled bool) {
	tc.strictNullCheck = enabled
}

// Check 执行类型检查
func (tc *TypeChecker) Check(file *ast.File) []TypeError {
	tc.currentFile = file
	
	// 检查所有声明
	for _, decl := range file.Declarations {
		tc.checkDeclaration(decl)
	}
	
	// 检查顶层语句
	for _, stmt := range file.Statements {
		tc.checkStatement(stmt)
	}
	
	return tc.errors
}

// checkDeclaration 检查声明
func (tc *TypeChecker) checkDeclaration(decl ast.Declaration) {
	switch d := decl.(type) {
	case *ast.ClassDecl:
		tc.checkClassDecl(d)
	case *ast.InterfaceDecl:
		tc.checkInterfaceDecl(d)
	case *ast.EnumDecl:
		// 枚举不需要额外检查
	case *ast.TypeAliasDecl:
		// 类型别名不需要额外检查
	case *ast.NewTypeDecl:
		// 新类型不需要额外检查
	}
}

// checkClassDecl 检查类声明
func (tc *TypeChecker) checkClassDecl(decl *ast.ClassDecl) {
	prevClassName := tc.currentClassName
	tc.currentClassName = decl.Name.Name
	defer func() { tc.currentClassName = prevClassName }()
	
	// 检查方法
	for _, method := range decl.Methods {
		tc.checkMethodDecl(method, decl.Name.Name)
	}
}

// checkInterfaceDecl 检查接口声明
func (tc *TypeChecker) checkInterfaceDecl(decl *ast.InterfaceDecl) {
	// 接口方法只有签名，不需要检查实现
}

// checkMethodDecl 检查方法声明
func (tc *TypeChecker) checkMethodDecl(method *ast.MethodDecl, className string) {
	// 创建函数上下文
	returnTypeName := ""
	isVoid := true
	if method.ReturnType != nil {
		returnTypeName = tc.getTypeName(method.ReturnType)
		isVoid = false
	}
	
	tc.currentFunc = &FunctionContext{
		Name:       method.Name.Name,
		ReturnType: returnTypeName,
		IsVoid:     isVoid,
	}
	
	// 进入新作用域
	tc.enterScope()
	defer tc.exitScope()
	
	// 添加参数到作用域
	for _, param := range method.Parameters {
		paramType := "any"
		if param.Type != nil {
			paramType = tc.getTypeName(param.Type)
		}
		tc.declareVariable(param.Name.Name, paramType, param.Pos())
	}
	
	// 构建 CFG
	if method.Body != nil {
		tc.cfgBuilder = NewCFGBuilder()
		tc.currentFunc.CFG = tc.cfgBuilder.Build(method.Body)
		
		// 将函数参数标记为已初始化（在入口块）
		if tc.currentFunc.CFG.Entry != nil {
			for _, param := range method.Parameters {
				tc.currentFunc.CFG.Entry.VarsDefined[param.Name.Name] = true
			}
		}
		
		// 检查返回值完整性
		if !isVoid {
			rc := NewReturnChecker(tc.currentFunc.CFG, returnTypeName)
			if !rc.CheckAllPathsReturn() {
				tc.addError(method.Name.Pos(), i18n.ErrReturnTypeMismatch,
					i18n.T(i18n.ErrReturnTypeMismatch, returnTypeName, "void"))
			}
		}
		
		// 检查未初始化变量
		// TODO: 修复 UninitializedChecker 的数据流分析
		// uc := NewUninitializedChecker(tc.currentFunc.CFG)
		// uc.Check()
		// tc.errors = append(tc.errors, uc.errors...)
		
		// 检查不可达代码
		urc := NewUnreachableChecker(tc.currentFunc.CFG)
		urc.Check()
		tc.warnings = append(tc.warnings, urc.warnings...)
		
		// 检查方法体
		tc.checkBlockStmt(method.Body)
	}
	
	tc.currentFunc = nil
}

// checkStatement 检查语句
func (tc *TypeChecker) checkStatement(stmt ast.Statement) {
	switch s := stmt.(type) {
	case *ast.VarDeclStmt:
		tc.checkVarDeclStmt(s)
	case *ast.MultiVarDeclStmt:
		tc.checkMultiVarDeclStmt(s)
	case *ast.ExprStmt:
		tc.checkExpression(s.Expr)
	case *ast.BlockStmt:
		tc.checkBlockStmt(s)
	case *ast.IfStmt:
		tc.checkIfStmt(s)
	case *ast.WhileStmt:
		tc.checkWhileStmt(s)
	case *ast.ForStmt:
		tc.checkForStmt(s)
	case *ast.ForeachStmt:
		tc.checkForeachStmt(s)
	case *ast.SwitchStmt:
		tc.checkSwitchStmt(s)
	case *ast.ReturnStmt:
		tc.checkReturnStmt(s)
	case *ast.TryStmt:
		tc.checkTryStmt(s)
	case *ast.ThrowStmt:
		tc.checkExpression(s.Exception)
	case *ast.EchoStmt:
		tc.checkExpression(s.Value)
	case *ast.BreakStmt, *ast.ContinueStmt:
		// 不需要类型检查
	}
}

// checkVarDeclStmt 检查变量声明语句
func (tc *TypeChecker) checkVarDeclStmt(stmt *ast.VarDeclStmt) {
	var declaredType string
	
	if stmt.Type != nil {
		declaredType = tc.getTypeName(stmt.Type)
	}
	
	// 检查初始值
	if stmt.Value != nil {
		actualType := tc.checkExpression(stmt.Value)
		
		if declaredType == "" {
			// 类型推断
			declaredType = actualType
		} else {
			// 类型检查
			if !tc.isTypeCompatible(actualType, declaredType) {
				tc.addError(stmt.Value.Pos(), i18n.ErrCannotAssign,
					i18n.T(i18n.ErrCannotAssign, actualType, declaredType))
			}
		}
	}
	
	// 声明变量
	tc.declareVariable(stmt.Name.Name, declaredType, stmt.Name.Pos())
}

// checkMultiVarDeclStmt 检查多变量声明
func (tc *TypeChecker) checkMultiVarDeclStmt(stmt *ast.MultiVarDeclStmt) {
	valueType := tc.checkExpression(stmt.Value)
	
	// 值应该是数组或元组类型
	if !strings.HasSuffix(valueType, "[]") && !strings.HasPrefix(valueType, "(") {
		tc.addError(stmt.Value.Pos(), i18n.ErrTypeMismatch,
			fmt.Sprintf("multi-variable declaration requires array or tuple, got %s", valueType))
	}
	
	// 声明所有变量
	for _, name := range stmt.Names {
		tc.declareVariable(name.Name, "any", name.Pos())
	}
}

// checkBlockStmt 检查块语句
func (tc *TypeChecker) checkBlockStmt(stmt *ast.BlockStmt) {
	tc.enterScope()
	defer tc.exitScope()
	
	for _, s := range stmt.Statements {
		tc.checkStatement(s)
	}
}

// checkIfStmt 检查 if 语句
func (tc *TypeChecker) checkIfStmt(stmt *ast.IfStmt) {
	// 检查条件
	tc.checkExpression(stmt.Condition)
	
	// 提取类型收窄信息
	narrowings := tc.extractTypeNarrowings(stmt.Condition, true)
	
	// 检查 then 分支（应用类型收窄）
	tc.enterScope()
	tc.applyNarrowings(narrowings)
	tc.checkStatement(stmt.Then)
	tc.exitScope()
	
	// 检查 elseif 分支
	for _, elseIf := range stmt.ElseIfs {
		tc.checkExpression(elseIf.Condition)
		elseIfNarrowings := tc.extractTypeNarrowings(elseIf.Condition, true)
		
		tc.enterScope()
		tc.applyNarrowings(elseIfNarrowings)
		tc.checkStatement(elseIf.Body)
		tc.exitScope()
	}
	
	// 检查 else 分支（应用反向收窄）
	if stmt.Else != nil {
		reverseNarrowings := tc.extractTypeNarrowings(stmt.Condition, false)
		
		tc.enterScope()
		tc.applyNarrowings(reverseNarrowings)
		tc.checkStatement(stmt.Else)
		tc.exitScope()
	}
}

// checkWhileStmt 检查 while 语句
func (tc *TypeChecker) checkWhileStmt(stmt *ast.WhileStmt) {
	tc.checkExpression(stmt.Condition)
	tc.checkStatement(stmt.Body)
}

// checkForStmt 检查 for 语句
func (tc *TypeChecker) checkForStmt(stmt *ast.ForStmt) {
	tc.enterScope()
	defer tc.exitScope()
	
	if stmt.Init != nil {
		tc.checkStatement(stmt.Init)
	}
	if stmt.Condition != nil {
		tc.checkExpression(stmt.Condition)
	}
	if stmt.Post != nil {
		tc.checkExpression(stmt.Post)
	}
	tc.checkStatement(stmt.Body)
}

// checkForeachStmt 检查 foreach 语句
func (tc *TypeChecker) checkForeachStmt(stmt *ast.ForeachStmt) {
	iterableType := tc.checkExpression(stmt.Iterable)
	
	tc.enterScope()
	defer tc.exitScope()
	
	// 推断 key 和 value 类型
	keyType := "any"
	valueType := "any"
	
	if strings.HasSuffix(iterableType, "[]") {
		keyType = "int"
		valueType = strings.TrimSuffix(iterableType, "[]")
	} else if strings.HasPrefix(iterableType, "map[") {
		// 从 map[K]V 提取类型
		if idx := strings.Index(iterableType, "]"); idx != -1 {
			keyType = iterableType[4:idx]
			valueType = iterableType[idx+1:]
		}
	}
	
	if stmt.Key != nil {
		tc.declareVariable(stmt.Key.Name, keyType, stmt.Key.Pos())
	}
	tc.declareVariable(stmt.Value.Name, valueType, stmt.Value.Pos())
	
	tc.checkStatement(stmt.Body)
}

// checkSwitchStmt 检查 switch 语句
func (tc *TypeChecker) checkSwitchStmt(stmt *ast.SwitchStmt) {
	exprType := tc.checkExpression(stmt.Expr)
	
	// 检查所有 case
	for _, caseClause := range stmt.Cases {
		caseType := tc.checkExpression(caseClause.Value)
		
		// case 值类型应该与 switch 表达式类型兼容
		if !tc.isTypeCompatible(caseType, exprType) && !tc.isTypeCompatible(exprType, caseType) {
			tc.addError(caseClause.Value.Pos(), i18n.ErrTypeMismatch,
				fmt.Sprintf("case type %s incompatible with switch type %s", caseType, exprType))
		}
		
		for _, s := range caseClause.Body {
			tc.checkStatement(s)
		}
	}
	
	// 检查 default
	if stmt.Default != nil {
		for _, s := range stmt.Default.Body {
			tc.checkStatement(s)
		}
	}
}

// checkReturnStmt 检查 return 语句
func (tc *TypeChecker) checkReturnStmt(stmt *ast.ReturnStmt) {
	if tc.currentFunc == nil {
		return
	}
	
	actualCount := len(stmt.Values)
	
	if tc.currentFunc.IsVoid {
		if actualCount > 0 {
			tc.addError(stmt.Pos(), i18n.ErrNoReturnExpected,
				i18n.T(i18n.ErrNoReturnExpected, actualCount))
		}
		return
	}
	
	if actualCount == 0 {
		tc.addError(stmt.Pos(), i18n.ErrReturnTypeMismatch,
			fmt.Sprintf("expected return type %s, got void", tc.currentFunc.ReturnType))
		return
	}
	
	// 检查返回值类型
	for _, val := range stmt.Values {
		actualType := tc.checkExpression(val)
		if !tc.isTypeCompatible(actualType, tc.currentFunc.ReturnType) {
			tc.addError(val.Pos(), i18n.ErrTypeMismatch,
				fmt.Sprintf("cannot return %s as %s", actualType, tc.currentFunc.ReturnType))
		}
	}
}

// checkTryStmt 检查 try 语句
func (tc *TypeChecker) checkTryStmt(stmt *ast.TryStmt) {
	tc.checkStatement(stmt.Try)
	
	for _, catchClause := range stmt.Catches {
		tc.enterScope()
		exceptionType := tc.getTypeName(catchClause.Type)
		tc.declareVariable(catchClause.Variable.Name, exceptionType, catchClause.Variable.Pos())
		tc.checkStatement(catchClause.Body)
		tc.exitScope()
	}
	
	if stmt.Finally != nil {
		tc.checkStatement(stmt.Finally.Body)
	}
}

// checkExpression 检查表达式并返回类型
func (tc *TypeChecker) checkExpression(expr ast.Expression) string {
	if expr == nil {
		return "void"
	}
	
	switch e := expr.(type) {
	case *ast.IntegerLiteral:
		return "int"
	case *ast.FloatLiteral:
		return "float"
	case *ast.StringLiteral, *ast.InterpStringLiteral:
		return "string"
	case *ast.BoolLiteral:
		return "bool"
	case *ast.NullLiteral:
		return "null"
	case *ast.Variable:
		return tc.checkVariable(e)
	case *ast.BinaryExpr:
		return tc.checkBinaryExpr(e)
	case *ast.UnaryExpr:
		return tc.checkUnaryExpr(e)
	case *ast.AssignExpr:
		return tc.checkAssignExpr(e)
	case *ast.CallExpr:
		return tc.checkCallExpr(e)
	case *ast.PropertyAccess:
		return tc.checkPropertyAccess(e)
	case *ast.MethodCall:
		return tc.checkMethodCall(e)
	case *ast.IndexExpr:
		return tc.checkIndexExpr(e)
	case *ast.ArrayLiteral:
		return tc.checkArrayLiteral(e)
	case *ast.MapLiteral:
		return tc.checkMapLiteral(e)
	case *ast.NewExpr:
		return tc.checkNewExpr(e)
	case *ast.IsExpr:
		return "bool"
	case *ast.TypeCastExpr:
		return tc.getTypeName(e.TargetType)
	case *ast.TernaryExpr:
		return tc.checkTernaryExpr(e)
	case *ast.ThisExpr:
		return tc.currentClassName
	case *ast.StaticAccess:
		return tc.checkStaticAccess(e)
	case *ast.SafePropertyAccess:
		return tc.checkSafePropertyAccess(e)
	case *ast.SafeMethodCall:
		return tc.checkSafeMethodCall(e)
	case *ast.NullCoalesceExpr:
		return tc.checkNullCoalesceExpr(e)
	default:
		return "any"
	}
}

// checkVariable 检查变量
func (tc *TypeChecker) checkVariable(expr *ast.Variable) string {
	varInfo := tc.lookupVariable(expr.Name)
	if varInfo == nil {
		tc.addError(expr.Pos(), i18n.ErrUndefinedVariable,
			i18n.T(i18n.ErrUndefinedVariable, expr.Name))
		return "error"
	}
	
	// 检查是否已初始化
	if !varInfo.IsInitialized {
		tc.addError(expr.Pos(), "compiler.uninitialized_variable",
			fmt.Sprintf("variable '%s' may not have been initialized", expr.Name))
	}
	
	// 检查类型收窄
	if narrowedType, ok := tc.currentScope.narrowings[expr.Name]; ok {
		return narrowedType
	}
	
	return varInfo.DeclaredType
}

// checkBinaryExpr 检查二元表达式
func (tc *TypeChecker) checkBinaryExpr(expr *ast.BinaryExpr) string {
	leftType := tc.checkExpression(expr.Left)
	rightType := tc.checkExpression(expr.Right)
	
	switch expr.Operator.Type {
	case token.PLUS, token.MINUS, token.STAR, token.SLASH, token.PERCENT:
		// 算术运算
		if !tc.isNumericType(leftType) || !tc.isNumericType(rightType) {
			tc.addError(expr.Operator.Pos, i18n.ErrOperandsMustBeNumbers,
				i18n.T(i18n.ErrOperandsMustBeNumbers))
		}
		if leftType == "float" || rightType == "float" {
			return "float"
		}
		return "int"
		
	case token.EQ, token.NE, token.LT, token.LE, token.GT, token.GE:
		// 比较运算
		return "bool"
		
	case token.AND, token.OR:
		// 逻辑运算
		return "bool"
		
	case token.BIT_AND, token.BIT_OR, token.BIT_XOR, token.LEFT_SHIFT, token.RIGHT_SHIFT:
		// 位运算
		return "int"
		
	default:
		return "any"
	}
}

// checkUnaryExpr 检查一元表达式
func (tc *TypeChecker) checkUnaryExpr(expr *ast.UnaryExpr) string {
	operandType := tc.checkExpression(expr.Operand)
	
	switch expr.Operator.Type {
	case token.MINUS, token.PLUS:
		if !tc.isNumericType(operandType) {
			tc.addError(expr.Operator.Pos, i18n.ErrOperandMustBeNumber,
				i18n.T(i18n.ErrOperandMustBeNumber))
		}
		return operandType
		
	case token.NOT:
		return "bool"
		
	case token.BIT_NOT:
		return "int"
		
	case token.INCREMENT, token.DECREMENT:
		return operandType
		
	default:
		return operandType
	}
}

// checkAssignExpr 检查赋值表达式
func (tc *TypeChecker) checkAssignExpr(expr *ast.AssignExpr) string {
	leftType := tc.checkExpression(expr.Left)
	rightType := tc.checkExpression(expr.Right)
	
	if !tc.isTypeCompatible(rightType, leftType) {
		tc.addError(expr.Right.Pos(), i18n.ErrCannotAssign,
			i18n.T(i18n.ErrCannotAssign, rightType, leftType))
	}
	
	// 标记变量为已初始化
	if v, ok := expr.Left.(*ast.Variable); ok {
		if varInfo := tc.lookupVariable(v.Name); varInfo != nil {
			varInfo.IsInitialized = true
		}
	}
	
	return leftType
}

// checkCallExpr 检查函数调用
func (tc *TypeChecker) checkCallExpr(expr *ast.CallExpr) string {
	// 检查参数
	for _, arg := range expr.Arguments {
		tc.checkExpression(arg)
	}
	
	// 查找函数签名
	if ident, ok := expr.Function.(*ast.Identifier); ok {
		if sig := tc.symbolTable.GetFunction(ident.Name); sig != nil {
			return sig.ReturnType
		}
	}
	
	return "any"
}

// checkPropertyAccess 检查属性访问
func (tc *TypeChecker) checkPropertyAccess(expr *ast.PropertyAccess) string {
	objectType := tc.checkExpression(expr.Object)
	
	// 空安全检查
	if tc.strictNullCheck && tc.isNullableType(objectType) {
		if !tc.isTypeNarrowed(expr.Object) {
			tc.addError(expr.Property.Pos(), "compiler.nullable_access",
				fmt.Sprintf("cannot access property of nullable type '%s'", objectType))
		}
	}
	
	// 查找属性类型
	baseType := tc.extractBaseType(objectType)
	if prop := tc.symbolTable.GetProperty(baseType, expr.Property.Name); prop != nil {
		return prop.Type
	}
	
	return "any"
}

// checkMethodCall 检查方法调用
func (tc *TypeChecker) checkMethodCall(expr *ast.MethodCall) string {
	objectType := tc.checkExpression(expr.Object)
	
	// 空安全检查
	if tc.strictNullCheck && tc.isNullableType(objectType) {
		if !tc.isTypeNarrowed(expr.Object) {
			tc.addError(expr.Method.Pos(), "compiler.nullable_access",
				fmt.Sprintf("cannot call method on nullable type '%s'", objectType))
		}
	}
	
	// 检查参数
	for _, arg := range expr.Arguments {
		tc.checkExpression(arg)
	}
	
	// 查找方法签名
	baseType := tc.extractBaseType(objectType)
	if method := tc.symbolTable.GetMethod(baseType, expr.Method.Name, len(expr.Arguments)); method != nil {
		return method.ReturnType
	}
	
	return "any"
}

// checkIndexExpr 检查索引表达式
func (tc *TypeChecker) checkIndexExpr(expr *ast.IndexExpr) string {
	objectType := tc.checkExpression(expr.Object)
	tc.checkExpression(expr.Index)
	
	// 数组类型
	if strings.HasSuffix(objectType, "[]") {
		return strings.TrimSuffix(objectType, "[]")
	}
	
	// Map 类型
	if strings.HasPrefix(objectType, "map[") {
		if idx := strings.Index(objectType, "]"); idx != -1 {
			return objectType[idx+1:]
		}
	}
	
	return "any"
}

// checkArrayLiteral 检查数组字面量
func (tc *TypeChecker) checkArrayLiteral(expr *ast.ArrayLiteral) string {
	if len(expr.Elements) == 0 {
		return "array"
	}
	
	// 检查所有元素并推断公共类型
	var elemTypes []string
	for _, elem := range expr.Elements {
		elemType := tc.checkExpression(elem)
		if elemType != "" && elemType != "error" {
			elemTypes = append(elemTypes, elemType)
		}
	}
	
	if len(elemTypes) == 0 {
		return "array"
	}
	
	// 使用第一个元素的类型
	return elemTypes[0] + "[]"
}

// checkMapLiteral 检查 Map 字面量
func (tc *TypeChecker) checkMapLiteral(expr *ast.MapLiteral) string {
	if len(expr.Pairs) == 0 {
		return "map"
	}
	
	var keyTypes, valueTypes []string
	for _, pair := range expr.Pairs {
		keyType := tc.checkExpression(pair.Key)
		valueType := tc.checkExpression(pair.Value)
		
		if keyType != "" && keyType != "error" {
			keyTypes = append(keyTypes, keyType)
		}
		if valueType != "" && valueType != "error" {
			valueTypes = append(valueTypes, valueType)
		}
	}
	
	if len(keyTypes) == 0 || len(valueTypes) == 0 {
		return "map"
	}
	
	return fmt.Sprintf("map[%s]%s", keyTypes[0], valueTypes[0])
}

// checkNewExpr 检查 new 表达式
func (tc *TypeChecker) checkNewExpr(expr *ast.NewExpr) string {
	// 检查参数
	for _, arg := range expr.Arguments {
		tc.checkExpression(arg)
	}
	
	return expr.ClassName.Name
}

// checkTernaryExpr 检查三元表达式
func (tc *TypeChecker) checkTernaryExpr(expr *ast.TernaryExpr) string {
	tc.checkExpression(expr.Condition)
	thenType := tc.checkExpression(expr.Then)
	elseType := tc.checkExpression(expr.Else)
	
	// 返回公共类型
	if thenType == elseType {
		return thenType
	}
	
	// 如果类型不同，返回联合类型
	return fmt.Sprintf("%s|%s", thenType, elseType)
}

// checkStaticAccess 检查静态访问
func (tc *TypeChecker) checkStaticAccess(expr *ast.StaticAccess) string {
	// 获取类名
	className := ""
	switch c := expr.Class.(type) {
	case *ast.Identifier:
		className = c.Name
	case *ast.SelfExpr:
		className = tc.currentClassName
	case *ast.ParentExpr:
		// parent 访问暂不支持，返回 any
		return "any"
	}
	
	if className == "" {
		return "any"
	}
	
	// 检查成员类型
	switch m := expr.Member.(type) {
	case *ast.CallExpr:
		// 静态方法调用 Class::method()
		if ident, ok := m.Function.(*ast.Identifier); ok {
			methodName := ident.Name
			// 查找方法签名
			if method := tc.symbolTable.GetMethod(className, methodName, len(m.Arguments)); method != nil {
				return method.ReturnType
			}
		}
	case *ast.Identifier:
		// 静态常量访问 Class::CONST
		if prop := tc.symbolTable.GetProperty(className, m.Name); prop != nil {
			return prop.Type
		}
	case *ast.Variable:
		// 静态属性访问 Class::$prop
		if prop := tc.symbolTable.GetProperty(className, m.Name); prop != nil {
			return prop.Type
		}
	}
	
	return "any"
}

// checkSafePropertyAccess 检查安全属性访问 ($obj?.prop)
func (tc *TypeChecker) checkSafePropertyAccess(expr *ast.SafePropertyAccess) string {
	objectType := tc.checkExpression(expr.Object)
	
	// 安全访问返回可空类型
	baseType := tc.extractBaseType(objectType)
	if prop := tc.symbolTable.GetProperty(baseType, expr.Property.Name); prop != nil {
		// 如果属性类型不是可空的，返回值变为可空
		if !tc.isNullableType(prop.Type) {
			return prop.Type + "|null"
		}
		return prop.Type
	}
	
	return "any|null"
}

// checkSafeMethodCall 检查安全方法调用 ($obj?.method())
func (tc *TypeChecker) checkSafeMethodCall(expr *ast.SafeMethodCall) string {
	objectType := tc.checkExpression(expr.Object)
	
	// 检查参数
	for _, arg := range expr.Arguments {
		tc.checkExpression(arg)
	}
	
	// 安全调用返回可空类型
	baseType := tc.extractBaseType(objectType)
	if method := tc.symbolTable.GetMethod(baseType, expr.Method.Name, len(expr.Arguments)); method != nil {
		// 如果返回类型不是可空的，返回值变为可空
		if !tc.isNullableType(method.ReturnType) {
			return method.ReturnType + "|null"
		}
		return method.ReturnType
	}
	
	return "any|null"
}

// checkNullCoalesceExpr 检查空合并表达式 ($a ?? $b)
func (tc *TypeChecker) checkNullCoalesceExpr(expr *ast.NullCoalesceExpr) string {
	leftType := tc.checkExpression(expr.Left)
	rightType := tc.checkExpression(expr.Right)
	
	// 结果类型是左侧的非空类型或右侧类型
	leftNonNull := tc.removeNullFromType(leftType)
	
	// 如果左右类型相同（去除null后），返回非空类型
	if leftNonNull == rightType || leftNonNull == tc.removeNullFromType(rightType) {
		return leftNonNull
	}
	
	// 否则返回联合类型
	return fmt.Sprintf("%s|%s", leftNonNull, rightType)
}

// Helper methods

// enterScope 进入新作用域
func (tc *TypeChecker) enterScope() {
	tc.currentScope = newTypeScope(tc.currentScope)
}

// exitScope 退出当前作用域
func (tc *TypeChecker) exitScope() {
	if tc.currentScope.parent != nil {
		tc.currentScope = tc.currentScope.parent
	}
}

// declareVariable 声明变量
func (tc *TypeChecker) declareVariable(name, typeName string, pos token.Position) {
	tc.currentScope.variables[name] = &VarTypeInfo{
		Name:          name,
		DeclaredType:  typeName,
		IsNullable:    tc.isNullableType(typeName),
		IsInitialized: true, // 假设声明时已初始化
		DefinedAt:     pos,
	}
}

// lookupVariable 查找变量
func (tc *TypeChecker) lookupVariable(name string) *VarTypeInfo {
	scope := tc.currentScope
	for scope != nil {
		if varInfo, ok := scope.variables[name]; ok {
			return varInfo
		}
		scope = scope.parent
	}
	return nil
}

// applyNarrowings 应用类型收窄
func (tc *TypeChecker) applyNarrowings(narrowings map[string]string) {
	for varName, narrowedType := range narrowings {
		tc.currentScope.narrowings[varName] = narrowedType
	}
}

// extractTypeNarrowings 提取类型收窄信息
// 支持的模式:
// - $x is T (类型检查)
// - $x != null (非空检查)
// - $x == null (空检查，在else分支收窄)
// - $x is T && $x.prop > 0 (复合条件)
// - !($x is T) (否定类型检查)
// - $x is T || $y is U (仅在negative时合并)
func (tc *TypeChecker) extractTypeNarrowings(cond ast.Expression, positive bool) map[string]string {
	narrowings := make(map[string]string)
	
	switch e := cond.(type) {
	case *ast.IsExpr:
		// $x is T
		if v, ok := e.Expr.(*ast.Variable); ok {
			typeName := tc.getTypeName(e.TypeName)
			effectivePositive := positive
			if e.Negated {
				effectivePositive = !positive
			}
			if effectivePositive {
				narrowings[v.Name] = typeName
			}
		}
		
	case *ast.BinaryExpr:
		switch e.Operator.Type {
		case token.AND:
			if positive {
				// $x is T && $y is U: 两个条件都收窄
				left := tc.extractTypeNarrowings(e.Left, true)
				right := tc.extractTypeNarrowings(e.Right, true)
				for k, v := range left {
					narrowings[k] = v
				}
				for k, v := range right {
					narrowings[k] = v
				}
			}
			
		case token.OR:
			if !positive {
				// !($x is T || $y is U) => !($x is T) && !($y is U)
				left := tc.extractTypeNarrowings(e.Left, false)
				right := tc.extractTypeNarrowings(e.Right, false)
				for k, v := range left {
					narrowings[k] = v
				}
				for k, v := range right {
					narrowings[k] = v
				}
			}
			
		case token.NE:
			// $x != null
			if v, ok := e.Left.(*ast.Variable); ok {
				if _, ok := e.Right.(*ast.NullLiteral); ok && positive {
					if varInfo := tc.lookupVariable(v.Name); varInfo != nil {
						if tc.isNullableType(varInfo.DeclaredType) {
							narrowings[v.Name] = tc.removeNullFromType(varInfo.DeclaredType)
						}
					}
				}
			}
			// null != $x
			if v, ok := e.Right.(*ast.Variable); ok {
				if _, ok := e.Left.(*ast.NullLiteral); ok && positive {
					if varInfo := tc.lookupVariable(v.Name); varInfo != nil {
						if tc.isNullableType(varInfo.DeclaredType) {
							narrowings[v.Name] = tc.removeNullFromType(varInfo.DeclaredType)
						}
					}
				}
			}
			
		case token.EQ:
			// $x == null (在 else 分支收窄)
			if v, ok := e.Left.(*ast.Variable); ok {
				if _, ok := e.Right.(*ast.NullLiteral); ok && !positive {
					if varInfo := tc.lookupVariable(v.Name); varInfo != nil {
						if tc.isNullableType(varInfo.DeclaredType) {
							narrowings[v.Name] = tc.removeNullFromType(varInfo.DeclaredType)
						}
					}
				}
			}
			// null == $x
			if v, ok := e.Right.(*ast.Variable); ok {
				if _, ok := e.Left.(*ast.NullLiteral); ok && !positive {
					if varInfo := tc.lookupVariable(v.Name); varInfo != nil {
						if tc.isNullableType(varInfo.DeclaredType) {
							narrowings[v.Name] = tc.removeNullFromType(varInfo.DeclaredType)
						}
					}
				}
			}
		}
		
	case *ast.UnaryExpr:
		if e.Operator.Type == token.NOT {
			// !expr: 反转 positive
			return tc.extractTypeNarrowings(e.Operand, !positive)
		}
		
	case *ast.CallExpr:
		// 检查 typeof($x) 调用（如果支持）
		if ident, ok := e.Function.(*ast.Identifier); ok && ident.Name == "typeof" {
			// 需要与比较运算符一起使用，这里只是标记
		}
	}
	
	return narrowings
}

// isTypeNarrowed 检查表达式是否在收窄上下文中
func (tc *TypeChecker) isTypeNarrowed(expr ast.Expression) bool {
	if v, ok := expr.(*ast.Variable); ok {
		_, narrowed := tc.currentScope.narrowings[v.Name]
		return narrowed
	}
	return false
}

// getTypeName 获取类型名称
func (tc *TypeChecker) getTypeName(typeNode ast.TypeNode) string {
	if typeNode == nil {
		return "any"
	}
	
	switch t := typeNode.(type) {
	case *ast.SimpleType:
		return t.Name
	case *ast.NullableType:
		return tc.getTypeName(t.Inner) + "|null"
	case *ast.ArrayType:
		return tc.getTypeName(t.ElementType) + "[]"
	case *ast.MapType:
		return fmt.Sprintf("map[%s]%s", tc.getTypeName(t.KeyType), tc.getTypeName(t.ValueType))
	case *ast.TupleType:
		var types []string
		for _, tt := range t.Types {
			types = append(types, tc.getTypeName(tt))
		}
		return "(" + strings.Join(types, ", ") + ")"
	case *ast.UnionType:
		var types []string
		for _, tt := range t.Types {
			types = append(types, tc.getTypeName(tt))
		}
		return strings.Join(types, "|")
	case *ast.ClassType:
		return t.Name.Literal
	default:
		return "any"
	}
}

// isTypeCompatible 检查类型兼容性
func (tc *TypeChecker) isTypeCompatible(actual, expected string) bool {
	if actual == expected {
		return true
	}
	
	if actual == "error" || expected == "error" {
		return true
	}
	
	if expected == "any" || expected == "mixed" {
		return true
	}
	
	// null 可以赋给可空类型
	if actual == "null" && strings.Contains(expected, "|null") {
		return true
	}
	
	// int 可以赋给 float
	if actual == "int" && expected == "float" {
		return true
	}
	
	// 数组类型
	if strings.HasSuffix(actual, "[]") && strings.HasSuffix(expected, "[]") {
		actualElem := strings.TrimSuffix(actual, "[]")
		expectedElem := strings.TrimSuffix(expected, "[]")
		return tc.isTypeCompatible(actualElem, expectedElem)
	}
	
	// 检查子类关系
	if tc.isSubclassOf(actual, expected) {
		return true
	}
	
	return false
}

// isSubclassOf 检查是否是子类
func (tc *TypeChecker) isSubclassOf(child, parent string) bool {
	current := child
	visited := make(map[string]bool)
	
	for current != "" && !visited[current] {
		visited[current] = true
		if current == parent {
			return true
		}
		current = tc.symbolTable.ClassParents[current]
	}
	
	return false
}

// isNullableType 检查是否是可空类型
func (tc *TypeChecker) isNullableType(typeName string) bool {
	return strings.Contains(typeName, "|null")
}

// removeNullFromType 从类型中移除 null
func (tc *TypeChecker) removeNullFromType(typeName string) string {
	return strings.Replace(typeName, "|null", "", 1)
}

// extractBaseType 提取基础类型（去除可空标记）
func (tc *TypeChecker) extractBaseType(typeName string) string {
	if strings.Contains(typeName, "|null") {
		return strings.Replace(typeName, "|null", "", 1)
	}
	return typeName
}

// isNumericType 检查是否是数值类型
func (tc *TypeChecker) isNumericType(typeName string) bool {
	numericTypes := map[string]bool{
		"int": true, "i8": true, "i16": true, "i32": true, "i64": true,
		"uint": true, "u8": true, "u16": true, "u32": true, "u64": true,
		"float": true, "f32": true, "f64": true,
		"byte": true,
	}
	return numericTypes[typeName]
}

// addError 添加错误
func (tc *TypeChecker) addError(pos token.Position, code, message string) {
	tc.errors = append(tc.errors, TypeError{
		Pos:     pos,
		Code:    code,
		Message: message,
	})
}

// addWarning 添加警告
func (tc *TypeChecker) addWarning(pos token.Position, code, message string) {
	tc.warnings = append(tc.warnings, TypeWarning{
		Pos:     pos,
		Code:    code,
		Message: message,
	})
}

