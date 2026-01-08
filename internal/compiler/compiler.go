package compiler

import (
	"fmt"
	"strings"

	"github.com/tangzhangming/nova/internal/ast"
	"github.com/tangzhangming/nova/internal/bytecode"
	"github.com/tangzhangming/nova/internal/errors"
	"github.com/tangzhangming/nova/internal/i18n"
	"github.com/tangzhangming/nova/internal/token"
)

// Compiler 编译器
type Compiler struct {
	function   *bytecode.Function
	scopeDepth int
	locals     []Local
	localCount int

	// 循环上下文
	loopStart  int
	loopDepth  int
	breakJumps []int

	// 类
	classes map[string]*bytecode.Class
	
	// 枚举
	enums map[string]*bytecode.Enum
	
	// 闭包上下文 - 禁止直接访问全局变量
	inClosure bool
	
	// 返回值类型检查
	returnType       ast.TypeNode // 当前函数的返回类型
	expectedReturns  int          // 预期返回值数量 (0=void, 1=单值, >1=多值)
	
	// 类型检查
	globalTypes map[string]string // 全局变量类型表
	
	// 符号表 - 用于静态类型检查
	symbolTable *SymbolTable
	
	// 当前编译的类名（用于方法内类型推导）
	currentClassName string

	// 源文件信息
	sourceFile       string // 当前编译的源文件路径
	currentLine      int    // 当前编译的行号
	currentNamespace string // 当前命名空间
	
	// 类型收窄上下文：变量名 -> 收窄后的类型
	// 用于在 if 分支中收窄变量类型（如 if($x is string) 后 $x 的类型为 string）
	narrowedTypes map[string]string

	// CSE (公共子表达式消除) 相关字段
	cseEnabled    bool           // 是否启用 CSE
	exprCache     map[string]int // 表达式签名 -> 临时变量slot
	cseScopeDepth int            // CSE 作用域深度

	// LICM (循环不变式外提) 相关字段
	loopModifiedVars map[string]bool // 循环内被修改的变量
	loopHoistedExprs []hoistedExpr   // 待外提的表达式
	inLoopAnalysis   bool            // 是否在循环分析阶段

	// 函数内联相关字段
	compiledFunctions map[string]*bytecode.Function // 已编译函数缓存（用于内联）
	currentFuncName   string                        // 当前编译的函数名（防止自递归内联）

	// 数组访问边界检查优化相关字段
	boundsCheckedArrays map[string]bool // 在当前循环中已验证边界的数组变量
	loopBoundVar        string          // 当前循环的边界变量（用于 for 循环优化）
	inOptimizedLoop     bool            // 是否在可优化边界检查的循环中

	errors []Error
}

// Local 局部变量
type Local struct {
	Name     string
	Depth    int
	Index    int
	TypeName string // 变量类型名（用于类型检查）
}

// hoistedExpr 表示一个被外提的表达式
type hoistedExpr struct {
	Expr    ast.Expression
	TempVar string // 临时变量名
}

// Error 编译错误
type Error struct {
	Pos     token.Position
	Message string
}

func (e Error) Error() string {
	return fmt.Sprintf("%s: %s", e.Pos, e.Message)
}

// New 创建编译器
func New() *Compiler {
	fn := bytecode.NewFunction("<script>")
	return &Compiler{
		function:        fn,
		locals:          make([]Local, 256),
		classes:         make(map[string]*bytecode.Class),
		enums:           make(map[string]*bytecode.Enum),
		globalTypes:     make(map[string]string),
		symbolTable:     NewSymbolTable(),
		cseEnabled:      true, // 默认启用 CSE
		exprCache:       make(map[string]int),
		loopModifiedVars: make(map[string]bool),
		loopHoistedExprs: make([]hoistedExpr, 0),
	}
}

// NewWithSymbolTable 创建带符号表的编译器（用于多文件编译）
func NewWithSymbolTable(st *SymbolTable) *Compiler {
	fn := bytecode.NewFunction("<script>")
	return &Compiler{
		function:        fn,
		locals:          make([]Local, 256),
		classes:         make(map[string]*bytecode.Class),
		enums:           make(map[string]*bytecode.Enum),
		globalTypes:     make(map[string]string),
		symbolTable:     st,
		cseEnabled:      true, // 默认启用 CSE
		exprCache:       make(map[string]int),
		loopModifiedVars: make(map[string]bool),
		loopHoistedExprs: make([]hoistedExpr, 0),
		compiledFunctions: make(map[string]*bytecode.Function),
		boundsCheckedArrays: make(map[string]bool),
	}
}

// GetSymbolTable 获取符号表
func (c *Compiler) GetSymbolTable() *SymbolTable {
	return c.symbolTable
}

// Classes 返回编译的类
func (c *Compiler) Classes() map[string]*bytecode.Class {
	return c.classes
}

// Enums 返回编译的枚举
func (c *Compiler) Enums() map[string]*bytecode.Enum {
	return c.enums
}

// Compile 编译 AST
func (c *Compiler) Compile(file *ast.File) (*bytecode.Function, []Error) {
	// 设置源文件信息
	c.sourceFile = file.Filename
	c.function.SourceFile = file.Filename
	
	// 设置命名空间
	if file.Namespace != nil {
		c.currentNamespace = file.Namespace.Name
	}
	
	// ========== Phase 1: 符号收集 ==========
	// 收集所有类、接口、方法签名，用于静态类型检查
	c.symbolTable.CollectFromFile(file)
	
	// ========== Phase 1.5: 类型检查 ==========
	// 独立的类型检查器，在代码生成前进行静态分析
	tc := NewTypeChecker(c.symbolTable)
	tc.SetStrictNullCheck(true)
	typeErrors := tc.Check(file)
	
	// 将类型错误转换为编译错误
	for _, te := range typeErrors {
		c.errors = append(c.errors, Error{
			Pos:     te.Pos,
			Message: te.Message,
		})
	}
	
	// 如果有类型错误，仍然继续编译以收集更多错误
	// 但生成的字节码可能不正确，运行时可能出错
	
	// 预留 slot 0 给调用者（与 CompileFunction 保持一致）
	c.addLocal("")

	// ========== Phase 2: 编译 ==========
	// 编译类、接口和枚举声明
	// 注意：类型别名和新类型声明已在符号收集阶段处理（CollectFromFile），
	// 它们不产生运行时字节码，只需要在符号表中注册即可
	for _, decl := range file.Declarations {
		switch d := decl.(type) {
		case *ast.ClassDecl:
			class := c.CompileClass(d)
			c.classes[d.Name.Name] = class
		case *ast.InterfaceDecl:
			iface := c.CompileInterface(d)
			c.classes[d.Name.Name] = iface
		case *ast.EnumDecl:
			enum := c.CompileEnum(d)
			c.enums[d.Name.Name] = enum
		case *ast.TypeAliasDecl, *ast.NewTypeDecl:
			// 类型别名和新类型声明已在符号收集阶段处理（CollectFromFile）
			// 它们不产生运行时字节码，只是类型系统的声明
			// 在这里可以添加验证逻辑，但目前不需要额外处理
		}
	}
	
	// ========== Phase 2.5: Final 约束检查 ==========
	c.validateFinalConstraints(file)

	// 编译顶层语句
	for _, stmt := range file.Statements {
		c.compileStmt(stmt)
	}

	// 添加返回指令
	c.emit(bytecode.OpReturnNull)

	c.function.LocalCount = c.localCount

	return c.function, c.errors
}

// CompileFunction 编译函数（允许隐式返回值，用于箭头函数等）
func (c *Compiler) CompileFunction(name string, params []*ast.Parameter, body *ast.BlockStmt) *bytecode.Function {
	// 保存当前状态
	prevFn := c.function
	prevLocals := c.locals
	prevLocalCount := c.localCount
	prevReturnType := c.returnType
	prevExpectedReturns := c.expectedReturns
	prevFuncName := c.currentFuncName

	// 创建新函数
	c.function = bytecode.NewFunction(name)
	c.currentFuncName = name
	c.function.Arity = len(params)
	c.function.SourceFile = c.sourceFile // 继承源文件信息
	c.locals = make([]Local, 256)
	c.localCount = 0
	c.scopeDepth = 0
	
	// 对于 CompileFunction，不检查返回类型（允许隐式返回值，用于箭头函数）
	c.returnType = nil
	c.expectedReturns = -1 // -1 表示不检查

	// 计算最小参数数量和处理可变参数
	minArity := len(params)
	isVariadic := false
	var defaultValues []bytecode.Value
	
	for i, param := range params {
		if param.Variadic {
			isVariadic = true
			minArity = i // 可变参数之前的参数是必需的
			break
		}
		if param.Default != nil && minArity == len(params) {
			minArity = i
		}
	}
	
	c.function.MinArity = minArity
	c.function.IsVariadic = isVariadic

	// 预留 slot 0 给调用者（与方法的 this 对应）
	c.addLocal("")
	
	// 添加参数作为局部变量 (直接使用 addLocal，因为函数参数始终是局部的)
	for _, param := range params {
		typeName := ""
		if param.Type != nil {
			typeName = c.getTypeName(param.Type)
		}
		c.addLocalWithType(param.Name.Name, typeName)
	}
	
	// 静态类型检查：参数类型在调用点检查，函数体内不需要运行时检查

	// 为有默认值的参数生成检查代码
	for _, param := range params {
		if param.Default != nil && !param.Variadic {
			// 计算并存储默认值
			defaultVal := c.evaluateConstExpr(param.Default)
			defaultValues = append(defaultValues, defaultVal)
		}
	}
	c.function.DefaultValues = defaultValues

	// 编译函数体
	c.beginScope()
	for _, stmt := range body.Statements {
		c.compileStmt(stmt)
	}
	c.endScope()

	// 添加默认返回
	c.emit(bytecode.OpReturnNull)

	fn := c.function
	fn.LocalCount = c.localCount
	
	// 检查函数是否可内联
	fn.Inlinable = c.isInlinable(fn)
	
	// 缓存已编译的函数（用于内联）
	c.compiledFunctions[name] = fn

	// 恢复状态
	c.function = prevFn
	c.locals = prevLocals
	c.localCount = prevLocalCount
	c.returnType = prevReturnType
	c.expectedReturns = prevExpectedReturns
	c.currentFuncName = prevFuncName

	return fn
}

// countExpectedReturns 计算预期的返回值数量
func (c *Compiler) countExpectedReturns(returnType ast.TypeNode) int {
	if returnType == nil {
		return 0 // void
	}
	
	// 检查是否是多返回值类型（TupleType）
	if tuple, ok := returnType.(*ast.TupleType); ok {
		return len(tuple.Types)
	}
	
	// 单一返回值
	return 1
}

// evaluateConstExpr 计算常量表达式的值（用于默认参数和常量折叠）
func (c *Compiler) evaluateConstExpr(expr ast.Expression) bytecode.Value {
	switch e := expr.(type) {
	case *ast.IntegerLiteral:
		return bytecode.NewInt(e.Value)
	case *ast.FloatLiteral:
		return bytecode.NewFloat(e.Value)
	case *ast.StringLiteral:
		return bytecode.NewString(e.Value)
	case *ast.BoolLiteral:
		return bytecode.NewBool(e.Value)
	case *ast.NullLiteral:
		return bytecode.NullValue
	case *ast.ArrayLiteral:
		arr := make([]bytecode.Value, len(e.Elements))
		for i, elem := range e.Elements {
			arr[i] = c.evaluateConstExpr(elem)
		}
		return bytecode.NewArray(arr)
	case *ast.BinaryExpr:
		// 尝试折叠二元表达式
		return c.foldBinaryExpr(e)
	case *ast.UnaryExpr:
		// 尝试折叠一元表达式
		return c.foldUnaryExpr(e)
	default:
		// 非常量表达式，返回 null 标记
		return bytecode.NullValue
	}
}

// isConstExpr 检查表达式是否为常量表达式
func (c *Compiler) isConstExpr(expr ast.Expression) bool {
	val := c.evaluateConstExpr(expr)
	return !val.IsNull() || expr == nil // null 也是常量
}

// foldBinaryExpr 折叠二元常量表达式
func (c *Compiler) foldBinaryExpr(e *ast.BinaryExpr) bytecode.Value {
	leftVal := c.evaluateConstExpr(e.Left)
	rightVal := c.evaluateConstExpr(e.Right)
	
	// 如果任一操作数不是常量，返回 null 标记
	if leftVal.IsNull() && !isNullLiteral(e.Left) {
		return bytecode.NullValue
	}
	if rightVal.IsNull() && !isNullLiteral(e.Right) {
		return bytecode.NullValue
	}
	
	// 根据运算符类型进行常量折叠
	switch e.Operator.Type {
	case token.PLUS:
		// 加法：字符串拼接或数值相加
		if leftVal.Type == bytecode.ValString || rightVal.Type == bytecode.ValString {
			return bytecode.NewString(leftVal.String() + rightVal.String())
		}
		if leftVal.Type == bytecode.ValInt && rightVal.Type == bytecode.ValInt {
			return bytecode.NewInt(leftVal.AsInt() + rightVal.AsInt())
		}
		if leftVal.Type == bytecode.ValFloat || rightVal.Type == bytecode.ValFloat {
			leftFloat := toFloat(leftVal)
			rightFloat := toFloat(rightVal)
			return bytecode.NewFloat(leftFloat + rightFloat)
		}
	case token.MINUS:
		if leftVal.Type == bytecode.ValInt && rightVal.Type == bytecode.ValInt {
			return bytecode.NewInt(leftVal.AsInt() - rightVal.AsInt())
		}
		if leftVal.Type == bytecode.ValFloat || rightVal.Type == bytecode.ValFloat {
			leftFloat := toFloat(leftVal)
			rightFloat := toFloat(rightVal)
			return bytecode.NewFloat(leftFloat - rightFloat)
		}
	case token.STAR:
		if leftVal.Type == bytecode.ValInt && rightVal.Type == bytecode.ValInt {
			return bytecode.NewInt(leftVal.AsInt() * rightVal.AsInt())
		}
		if leftVal.Type == bytecode.ValFloat || rightVal.Type == bytecode.ValFloat {
			leftFloat := toFloat(leftVal)
			rightFloat := toFloat(rightVal)
			return bytecode.NewFloat(leftFloat * rightFloat)
		}
	case token.SLASH:
		if leftVal.Type == bytecode.ValFloat || rightVal.Type == bytecode.ValFloat {
			leftFloat := toFloat(leftVal)
			rightFloat := toFloat(rightVal)
			if rightFloat == 0 {
				// 除零错误，不折叠
				return bytecode.NullValue
			}
			return bytecode.NewFloat(leftFloat / rightFloat)
		}
		if leftVal.Type == bytecode.ValInt && rightVal.Type == bytecode.ValInt {
			rightInt := rightVal.AsInt()
			if rightInt == 0 {
				// 除零错误，不折叠
				return bytecode.NullValue
			}
			// 整数除法
			return bytecode.NewInt(leftVal.AsInt() / rightInt)
		}
	case token.PERCENT:
		if leftVal.Type == bytecode.ValInt && rightVal.Type == bytecode.ValInt {
			rightInt := rightVal.AsInt()
			if rightInt == 0 {
				// 模零错误，不折叠
				return bytecode.NullValue
			}
			return bytecode.NewInt(leftVal.AsInt() % rightInt)
		}
	case token.EQ:
		return bytecode.NewBool(leftVal.Equals(rightVal))
	case token.NE:
		return bytecode.NewBool(!leftVal.Equals(rightVal))
	case token.LT:
		return bytecode.NewBool(compareValues(leftVal, rightVal) < 0)
	case token.LE:
		return bytecode.NewBool(compareValues(leftVal, rightVal) <= 0)
	case token.GT:
		return bytecode.NewBool(compareValues(leftVal, rightVal) > 0)
	case token.GE:
		return bytecode.NewBool(compareValues(leftVal, rightVal) >= 0)
	case token.BIT_AND:
		if leftVal.Type == bytecode.ValInt && rightVal.Type == bytecode.ValInt {
			return bytecode.NewInt(leftVal.AsInt() & rightVal.AsInt())
		}
	case token.BIT_OR:
		if leftVal.Type == bytecode.ValInt && rightVal.Type == bytecode.ValInt {
			return bytecode.NewInt(leftVal.AsInt() | rightVal.AsInt())
		}
	case token.BIT_XOR:
		if leftVal.Type == bytecode.ValInt && rightVal.Type == bytecode.ValInt {
			return bytecode.NewInt(leftVal.AsInt() ^ rightVal.AsInt())
		}
	case token.LEFT_SHIFT:
		if leftVal.Type == bytecode.ValInt && rightVal.Type == bytecode.ValInt {
			return bytecode.NewInt(leftVal.AsInt() << rightVal.AsInt())
		}
	case token.RIGHT_SHIFT:
		if leftVal.Type == bytecode.ValInt && rightVal.Type == bytecode.ValInt {
			return bytecode.NewInt(leftVal.AsInt() >> rightVal.AsInt())
		}
	}
	
	// 无法折叠，返回 null 标记
	return bytecode.NullValue
}

// foldUnaryExpr 折叠一元常量表达式
func (c *Compiler) foldUnaryExpr(e *ast.UnaryExpr) bytecode.Value {
	operandVal := c.evaluateConstExpr(e.Operand)
	
	// 如果操作数不是常量，返回 null 标记
	if operandVal.IsNull() && !isNullLiteral(e.Operand) {
		return bytecode.NullValue
	}
	
	switch e.Operator.Type {
	case token.MINUS:
		// 取负
		if operandVal.Type == bytecode.ValInt {
			return bytecode.NewInt(-operandVal.AsInt())
		}
		if operandVal.Type == bytecode.ValFloat {
			return bytecode.NewFloat(-operandVal.AsFloat())
		}
	case token.NOT:
		// 逻辑非
		return bytecode.NewBool(!operandVal.IsTruthy())
	case token.BIT_NOT:
		// 位非
		if operandVal.Type == bytecode.ValInt {
			return bytecode.NewInt(^operandVal.AsInt())
		}
	}
	
	// 无法折叠，返回 null 标记
	return bytecode.NullValue
}

// isNullLiteral 检查表达式是否为 null 字面量
func isNullLiteral(expr ast.Expression) bool {
	_, ok := expr.(*ast.NullLiteral)
	return ok
}

// toFloat 将值转换为浮点数
func toFloat(v bytecode.Value) float64 {
	switch v.Type {
	case bytecode.ValInt:
		return float64(v.AsInt())
	case bytecode.ValFloat:
		return v.AsFloat()
	default:
		return 0
	}
}

// compareValues 比较两个值，返回 -1, 0, 1
func compareValues(a, b bytecode.Value) int {
	if a.Type == bytecode.ValInt && b.Type == bytecode.ValInt {
		aInt := a.AsInt()
		bInt := b.AsInt()
		if aInt < bInt {
			return -1
		}
		if aInt > bInt {
			return 1
		}
		return 0
	}
	if a.Type == bytecode.ValFloat || b.Type == bytecode.ValFloat {
		aFloat := toFloat(a)
		bFloat := toFloat(b)
		if aFloat < bFloat {
			return -1
		}
		if aFloat > bFloat {
			return 1
		}
		return 0
	}
	if a.Type == bytecode.ValString && b.Type == bytecode.ValString {
		aStr := a.AsString()
		bStr := b.AsString()
		if aStr < bStr {
			return -1
		}
		if aStr > bStr {
			return 1
		}
		return 0
	}
	return 0
}

// computeExprSignature 计算表达式的唯一签名，用于 CSE
// 返回空字符串表示该表达式不能缓存（有副作用或无法确定）
func (c *Compiler) computeExprSignature(expr ast.Expression) string {
	if expr == nil {
		return ""
	}

	switch e := expr.(type) {
	case *ast.IntegerLiteral:
		return fmt.Sprintf("int:%d", e.Value)
	case *ast.FloatLiteral:
		return fmt.Sprintf("float:%g", e.Value)
	case *ast.StringLiteral:
		return fmt.Sprintf("str:%s", e.Value)
	case *ast.BoolLiteral:
		if e.Value {
			return "bool:true"
		}
		return "bool:false"
	case *ast.NullLiteral:
		return "null"
	case *ast.Variable:
		// 变量加载本身非常快，不需要缓存
		// 缓存变量会导致 DUP 指令错误地复制栈顶值
		return ""
	case *ast.Identifier:
		// 标识符可能是函数或类，暂时不缓存
		return ""
	case *ast.BinaryExpr:
		leftSig := c.computeExprSignature(e.Left)
		rightSig := c.computeExprSignature(e.Right)
		if leftSig == "" || rightSig == "" {
			return ""
		}
		return fmt.Sprintf("bin:%s:%s:%s", e.Operator.Literal, leftSig, rightSig)
	case *ast.UnaryExpr:
		// 只处理没有副作用的运算符
		if e.Operator.Type == token.INCREMENT || e.Operator.Type == token.DECREMENT {
			return "" // 有副作用
		}
		operandSig := c.computeExprSignature(e.Operand)
		if operandSig == "" {
			return ""
		}
		return fmt.Sprintf("unary:%s:%s", e.Operator.Literal, operandSig)
	case *ast.IndexExpr:
		// 索引访问可能是数组或 map，保守策略：不缓存
		return ""
	case *ast.PropertyAccess:
		// 属性访问可能涉及 getter，保守策略：不缓存
		return ""
	case *ast.MethodCall:
		// 方法调用肯定有副作用
		return ""
	case *ast.CallExpr:
		// 函数调用可能有副作用
		return ""
	case *ast.AssignExpr:
		// 赋值有副作用
		return ""
	case *ast.NewExpr:
		// new 表达式有副作用
		return ""
	case *ast.ThisExpr:
		// $this 可能变化，不缓存
		return ""
	default:
		// 其他类型保守处理，不缓存
		return ""
	}
}

// hasSideEffect 检测表达式是否有副作用
func (c *Compiler) hasSideEffect(expr ast.Expression) bool {
	if expr == nil {
		return false
	}

	switch e := expr.(type) {
	case *ast.AssignExpr:
		return true
	case *ast.MethodCall:
		return true
	case *ast.CallExpr:
		return true // 保守估计，函数调用可能有副作用
	case *ast.NewExpr:
		return true
	case *ast.UnaryExpr:
		// 自增/自减有副作用
		if e.Operator.Type == token.INCREMENT || e.Operator.Type == token.DECREMENT {
			return true
		}
		return c.hasSideEffect(e.Operand)
	case *ast.BinaryExpr:
		// 二元表达式本身没有副作用，但操作数可能有
		return c.hasSideEffect(e.Left) || c.hasSideEffect(e.Right)
	case *ast.IndexExpr:
		// 索引访问可能调用 getter，保守估计有副作用
		return true
	case *ast.PropertyAccess:
		// 属性访问可能调用 getter，保守估计有副作用
		return true
	default:
		// 字面量、变量等通常没有副作用
		return false
	}
}

// isSimpleLiteral 检查表达式是否是简单字面量
// 简单字面量不需要 CSE 缓存，因为它们已经很高效
func (c *Compiler) isSimpleLiteral(expr ast.Expression) bool {
	switch expr.(type) {
	case *ast.IntegerLiteral, *ast.FloatLiteral, *ast.StringLiteral,
		*ast.BoolLiteral, *ast.NullLiteral:
		return true
	default:
		return false
	}
}

// CompileClosure 编译带 use 的闭包（无返回类型检查）
func (c *Compiler) CompileClosure(name string, params []*ast.Parameter, useVars []*ast.Variable, body *ast.BlockStmt) *bytecode.Function {
	return c.CompileClosureWithReturnType(name, params, useVars, body, nil)
}

// CompileClosureWithReturnType 编译带 use 的闭包（带返回类型检查）
func (c *Compiler) CompileClosureWithReturnType(name string, params []*ast.Parameter, useVars []*ast.Variable, body *ast.BlockStmt, returnType ast.TypeNode) *bytecode.Function {
	// 保存当前状态
	prevFn := c.function
	prevLocals := c.locals
	prevLocalCount := c.localCount
	prevInClosure := c.inClosure
	prevReturnType := c.returnType
	prevExpectedReturns := c.expectedReturns

	// 创建新函数
	c.function = bytecode.NewFunction(name)
	c.function.Arity = len(params)
	c.function.UpvalueCount = len(useVars)
	c.function.SourceFile = c.sourceFile // 继承源文件信息
	c.locals = make([]Local, 256)
	c.localCount = 0
	c.scopeDepth = 0
	c.inClosure = true // 标记在闭包中，禁止访问全局变量
	
	// 设置返回类型检查
	c.returnType = returnType
	c.expectedReturns = c.countExpectedReturns(returnType)

	// 计算最小参数数量和处理可变参数
	minArity := len(params)
	isVariadic := false
	var defaultValues []bytecode.Value
	
	for i, param := range params {
		if param.Variadic {
			isVariadic = true
			minArity = i
			break
		}
		if param.Default != nil && minArity == len(params) {
			minArity = i
		}
	}
	
	c.function.MinArity = minArity
	c.function.IsVariadic = isVariadic

	// 预留 slot 0 给调用者
	c.addLocal("")
	
	// 添加参数作为局部变量（包含类型信息）
	for _, param := range params {
		typeName := ""
		if param.Type != nil {
			typeName = c.getTypeName(param.Type)
		}
		c.addLocalWithType(param.Name.Name, typeName)
	}
	
	// 静态类型检查：参数类型在调用点检查，函数体内不需要运行时检查
	
	// 为有默认值的参数计算默认值
	for _, param := range params {
		if param.Default != nil && !param.Variadic {
			defaultVal := c.evaluateConstExpr(param.Default)
			defaultValues = append(defaultValues, defaultVal)
		}
	}
	c.function.DefaultValues = defaultValues
	
	// 添加 use 变量作为局部变量（它们会通过 upvalue 机制获取）
	for _, v := range useVars {
		c.addLocal(v.Name)
	}

	// 编译函数体
	c.beginScope()
	for _, stmt := range body.Statements {
		c.compileStmt(stmt)
	}
	c.endScope()

	// 添加默认返回
	c.emit(bytecode.OpReturnNull)

	fn := c.function
	fn.LocalCount = c.localCount

	// 恢复状态
	c.function = prevFn
	c.locals = prevLocals
	c.localCount = prevLocalCount
	c.inClosure = prevInClosure
	c.returnType = prevReturnType
	c.expectedReturns = prevExpectedReturns

	return fn
}

// ============================================================================
// 语句编译
// ============================================================================

func (c *Compiler) compileStmt(stmt ast.Statement) {
	// 更新当前行号
	c.currentLine = stmt.Pos().Line
	
	switch s := stmt.(type) {
	case *ast.ExprStmt:
		c.compileExpr(s.Expr)
		c.emit(bytecode.OpPop)

	case *ast.VarDeclStmt:
		c.compileVarDecl(s)

	case *ast.MultiVarDeclStmt:
		c.compileMultiVarDecl(s)

	case *ast.BlockStmt:
		c.beginScope()
		// 死代码消除：如果遇到 return，后续语句不编译
		for _, inner := range s.Statements {
			c.compileStmt(inner)
			// 检查当前语句是否为 return（简单检查，不分析控制流）
			if retStmt, ok := inner.(*ast.ReturnStmt); ok && retStmt != nil {
				// return 后的语句是死代码，跳过
				break
			}
		}
		c.endScope()

	case *ast.IfStmt:
		c.compileIfStmt(s)

	case *ast.WhileStmt:
		c.compileWhileStmt(s)

	case *ast.ForStmt:
		c.compileForStmt(s)

	case *ast.ForeachStmt:
		c.compileForeachStmt(s)

	case *ast.SwitchStmt:
		c.compileSwitchStmt(s)

	case *ast.BreakStmt:
		c.compileBreakStmt()

	case *ast.ContinueStmt:
		c.compileContinueStmt()

	case *ast.ReturnStmt:
		c.compileReturnStmt(s)

	case *ast.EchoStmt:
		c.compileExpr(s.Value)
		c.emit(bytecode.OpDebugPrint)

	case *ast.TryStmt:
		c.compileTryStmt(s)

	case *ast.ThrowStmt:
		c.compileExpr(s.Exception)
		c.emit(bytecode.OpThrow)

	default:
		c.error(stmt.Pos(), i18n.T(i18n.ErrUnsupportedStmt))
	}
}

func (c *Compiler) compileVarDecl(s *ast.VarDeclStmt) {
	// 获取声明的类型
	var declaredType string
	if s.Type != nil {
		declaredType = c.getTypeName(s.Type)
	}
	
	// 静态类型系统：如果是类型推断 (:=)，必须能推断出类型
	if s.Type == nil && s.Value != nil {
		inferredType := c.inferExprType(s.Value)
		if inferredType == "" || inferredType == "error" {
			// inferExprType 已经报过错了，这里只需设置标记
			declaredType = "error"
		} else {
			declaredType = inferredType
		}
	}
	
	// 静态类型检查：如果有显式类型和初始值，检查类型匹配
	if s.Type != nil && s.Value != nil {
		actualType := c.inferExprType(s.Value)
		// 静态类型系统：类型必须兼容（除非已报错）
		if actualType != "error" && declaredType != "error" {
			if !c.isTypeCompatible(actualType, declaredType) {
				c.error(s.Value.Pos(), i18n.T(i18n.ErrCannotAssign, actualType, declaredType))
			}
		}
	}
	
	// 如果是泛型类型，验证约束
	if genericType, ok := s.Type.(*ast.GenericType); ok {
		baseName := c.getTypeName(genericType.BaseType)
		c.validateGenericConstraints(baseName, genericType.TypeArgs)
	}
	
	// 检查是否是定长数组类型
	if arrType, ok := s.Type.(*ast.ArrayType); ok && arrType.Size != nil {
		// 获取数组大小（必须是常量整数）
		capacity := c.evalConstInt(arrType.Size)
		if capacity < 0 {
			c.error(arrType.Size.Pos(), i18n.T(i18n.ErrArraySizeNegative))
			return
		}
		
		if s.Value != nil {
			// 有初始值
			if arr, ok := s.Value.(*ast.ArrayLiteral); ok {
				// 数组字面量初始化
				if len(arr.Elements) > capacity {
					c.error(arr.Pos(), i18n.T(i18n.ErrArrayTooManyElements, capacity, len(arr.Elements)))
					return
				}
				for _, elem := range arr.Elements {
					c.compileExpr(elem)
				}
				// 创建定长数组
				c.emitU16(bytecode.OpNewFixedArray, uint16(capacity))
				c.currentChunk().WriteU16(uint16(len(arr.Elements)), c.currentLine)
			} else {
				// 非数组字面量初始化，创建空定长数组
				c.emitU16(bytecode.OpNewFixedArray, uint16(capacity))
				c.currentChunk().WriteU16(0, c.currentLine)
			}
		} else {
			// 无初始值，创建空定长数组
			c.emitU16(bytecode.OpNewFixedArray, uint16(capacity))
			c.currentChunk().WriteU16(0, c.currentLine)
		}
	} else if c.isBytesArrayType(s.Type) {
		// byte[] 类型特殊处理
		if s.Value != nil {
			if arr, ok := s.Value.(*ast.ArrayLiteral); ok {
				// 数组字面量编译为 byte[]
				for _, elem := range arr.Elements {
					c.compileExpr(elem)
				}
				c.emitU16(bytecode.OpNewBytes, uint16(len(arr.Elements)))
			} else {
				// 非数组字面量（可能是函数调用等），正常编译
				c.compileExpr(s.Value)
			}
		} else {
			// 无初始值，创建空 byte[]
			c.emitU16(bytecode.OpNewBytes, 0)
		}
	} else {
		// 普通变量或动态数组
		if s.Value != nil {
			c.compileExpr(s.Value)
		} else {
			c.emit(bytecode.OpNull)
		}
	}
	
	// 注意：类型推断 (:=) 已在函数开头处理

	// 声明并定义变量
	if c.scopeDepth > 0 {
		// 局部变量
		c.declareVariableWithType(s.Name.Name, declaredType)
		c.defineVariable()
	} else {
		// 全局变量 - 存储到全局变量表
		c.globalTypes[s.Name.Name] = declaredType
		idx := c.makeConstant(bytecode.NewString(s.Name.Name))
		c.emitU16(bytecode.OpStoreGlobal, idx)
		c.emit(bytecode.OpPop) // 弹出值
	}
}

func (c *Compiler) compileMultiVarDecl(s *ast.MultiVarDeclStmt) {
	// 编译右侧表达式（应返回数组）
	c.compileExpr(s.Value)
	
	if c.scopeDepth > 0 {
		// 局部变量：记住数组在栈上的位置
		arrSlot := c.localCount
		c.addLocal("") // 占位，数组就在这个位置
		
		// 多返回值解包：从数组中提取各个值
		for i, name := range s.Names {
			// 加载数组（从栈上的固定位置）
			c.emitU16(bytecode.OpLoadLocal, uint16(arrSlot))
			// 获取数组的第 i 个元素
			c.emitConstant(bytecode.NewInt(int64(i)))
			c.emit(bytecode.OpArrayGet)
			
			// 声明变量，值已在栈顶的正确位置
			c.declareVariable(name.Name)
			c.defineVariable()
		}
	} else {
		// 全局变量：从数组中提取每个值并存储到全局变量表
		for i, name := range s.Names {
			if i < len(s.Names)-1 {
				c.emit(bytecode.OpDup) // 复制数组给下一次使用
			}
			// 获取数组的第 i 个元素
			c.emitConstant(bytecode.NewInt(int64(i)))
			c.emit(bytecode.OpArrayGet)
			// 存储到全局变量
			idx := c.makeConstant(bytecode.NewString(name.Name))
			c.emitU16(bytecode.OpStoreGlobal, idx)
			c.emit(bytecode.OpPop)
		}
	}
}

func (c *Compiler) compileIfStmt(s *ast.IfStmt) {
	// 死代码消除：检查条件是否为常量
	if c.isConstExpr(s.Condition) {
		condVal := c.evaluateConstExpr(s.Condition)
		if condVal.Type == bytecode.ValBool {
			if !condVal.IsTruthy() {
				// if (false) - 只编译 else 分支
				if s.Else != nil {
					reverseNarrowings := c.extractTypeNarrowings(s.Condition, false)
					savedElseTypes := c.applyTypeNarrowings(reverseNarrowings)
					c.compileStmt(s.Else)
					c.restoreTypes(savedElseTypes)
				}
				// then 和 elseif 分支都是死代码，不编译
				return
			} else {
				// if (true) - 只编译 then 分支
				narrowings := c.extractTypeNarrowings(s.Condition, true)
				savedTypes := c.applyTypeNarrowings(narrowings)
				c.compileStmt(s.Then)
				c.restoreTypes(savedTypes)
				// else 和 elseif 分支都是死代码，不编译
				return
			}
		}
	}
	
	// 类型收窄：分析条件中的 is 表达式
	narrowings := c.extractTypeNarrowings(s.Condition, true)
	
	// 编译条件
	c.compileExpr(s.Condition)

	// 条件为假时跳转
	thenJump := c.emitJump(bytecode.OpJumpIfFalse)
	c.emit(bytecode.OpPop) // 弹出条件值

	// 应用类型收窄
	savedTypes := c.applyTypeNarrowings(narrowings)
	
	// 编译 then 分支
	c.compileStmt(s.Then)
	
	// 恢复类型
	c.restoreTypes(savedTypes)

	elseJump := c.emitJump(bytecode.OpJump)

	// 修补 then 跳转
	c.patchJump(thenJump)
	c.emit(bytecode.OpPop) // 弹出条件值

	// 编译 elseif 分支
	for _, elseIf := range s.ElseIfs {
		// 死代码消除：检查 elseif 条件是否为常量
		if c.isConstExpr(elseIf.Condition) {
			condVal := c.evaluateConstExpr(elseIf.Condition)
			if condVal.Type == bytecode.ValBool && !condVal.IsTruthy() {
				// elseif (false) - 跳过此分支
				continue
			}
		}
		
		// 类型收窄：分析 elseif 条件
		elseIfNarrowings := c.extractTypeNarrowings(elseIf.Condition, true)
		
		c.compileExpr(elseIf.Condition)
		nextJump := c.emitJump(bytecode.OpJumpIfFalse)
		c.emit(bytecode.OpPop)
		
		// 应用类型收窄
		savedElseIfTypes := c.applyTypeNarrowings(elseIfNarrowings)
		
		c.compileStmt(elseIf.Body)
		
		// 恢复类型
		c.restoreTypes(savedElseIfTypes)
		
		elseJump = c.emitJump(bytecode.OpJump)
		c.patchJump(nextJump)
		c.emit(bytecode.OpPop)
	}

	// 编译 else 分支
	if s.Else != nil {
		// 在 else 分支中，应用反向收窄（条件为假时的类型）
		reverseNarrowings := c.extractTypeNarrowings(s.Condition, false)
		savedElseTypes := c.applyTypeNarrowings(reverseNarrowings)
		
		c.compileStmt(s.Else)
		
		c.restoreTypes(savedElseTypes)
	}

	c.patchJump(elseJump)
}

// TypeNarrowing 表示一个类型收窄
type TypeNarrowing struct {
	VarName     string // 变量名
	NarrowedType string // 收窄后的类型
}

// extractTypeNarrowings 从条件表达式中提取类型收窄信息
// positive: true 表示条件为真时的收窄，false 表示条件为假时的收窄
func (c *Compiler) extractTypeNarrowings(cond ast.Expression, positive bool) []TypeNarrowing {
	var narrowings []TypeNarrowing
	
	switch e := cond.(type) {
	case *ast.IsExpr:
		// $x is string
		if v, ok := e.Expr.(*ast.Variable); ok {
			typeName := c.getTypeName(e.TypeName)
			// 如果是取反的 is 表达式，反转 positive 标志
			effectivePositive := positive
			if e.Negated {
				effectivePositive = !positive
			}
			if effectivePositive {
				narrowings = append(narrowings, TypeNarrowing{
					VarName:     v.Name,
					NarrowedType: typeName,
				})
			}
		}
	case *ast.BinaryExpr:
		// 处理 && 和 || 逻辑运算
		if e.Operator.Type == token.AND && positive {
			// a && b: 当条件为真时，两边都为真
			narrowings = append(narrowings, c.extractTypeNarrowings(e.Left, true)...)
			narrowings = append(narrowings, c.extractTypeNarrowings(e.Right, true)...)
		} else if e.Operator.Type == token.OR && !positive {
			// a || b: 当条件为假时，两边都为假
			narrowings = append(narrowings, c.extractTypeNarrowings(e.Left, false)...)
			narrowings = append(narrowings, c.extractTypeNarrowings(e.Right, false)...)
		}
		// 处理 !== null 检查
		if e.Operator.Type == token.NE {
			if v, ok := e.Left.(*ast.Variable); ok {
				if _, ok := e.Right.(*ast.NullLiteral); ok {
					if positive {
						// $x !== null: 在 then 分支中 $x 不是 null
						varType := c.getVariableType(v.Name)
						if strings.Contains(varType, "|null") {
							narrowedType := strings.Replace(varType, "|null", "", 1)
							narrowings = append(narrowings, TypeNarrowing{
								VarName:     v.Name,
								NarrowedType: narrowedType,
							})
						}
					}
				}
			}
		}
	case *ast.UnaryExpr:
		// !expr: 反转收窄方向
		if e.Operator.Type == token.NOT {
			narrowings = append(narrowings, c.extractTypeNarrowings(e.Operand, !positive)...)
		}
	}
	
	return narrowings
}

// applyTypeNarrowings 应用类型收窄，返回被覆盖的原始类型（用于恢复）
func (c *Compiler) applyTypeNarrowings(narrowings []TypeNarrowing) map[string]string {
	if len(narrowings) == 0 {
		return nil
	}
	
	if c.narrowedTypes == nil {
		c.narrowedTypes = make(map[string]string)
	}
	
	saved := make(map[string]string)
	for _, n := range narrowings {
		// 保存原始类型
		if original, exists := c.narrowedTypes[n.VarName]; exists {
			saved[n.VarName] = original
		} else {
			saved[n.VarName] = "" // 标记为之前不存在
		}
		// 应用收窄
		c.narrowedTypes[n.VarName] = n.NarrowedType
	}
	
	return saved
}

// restoreTypes 恢复类型收窄前的状态
func (c *Compiler) restoreTypes(saved map[string]string) {
	if saved == nil {
		return
	}
	
	for varName, originalType := range saved {
		if originalType == "" {
			delete(c.narrowedTypes, varName)
		} else {
			c.narrowedTypes[varName] = originalType
		}
	}
}

// collectModifiedVars 收集循环体中被修改的变量
func (c *Compiler) collectModifiedVars(body *ast.BlockStmt) map[string]bool {
	modified := make(map[string]bool)
	
	ast.Walk(body, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.AssignExpr:
			if v, ok := n.Left.(*ast.Variable); ok {
				modified[v.Name] = true
			}
		case *ast.UnaryExpr:
			if n.Operator.Type == token.INCREMENT || n.Operator.Type == token.DECREMENT {
				if v, ok := n.Operand.(*ast.Variable); ok {
					modified[v.Name] = true
				}
			}
		case *ast.VarDeclStmt:
			// 变量声明也视为修改
			modified[n.Name.Name] = true
		case *ast.MultiVarDeclStmt:
			// 多变量声明
			for _, name := range n.Names {
				modified[name.Name] = true
			}
		}
		return true
	})
	
	return modified
}

// isLoopInvariant 检测表达式是否为循环不变式
func (c *Compiler) isLoopInvariant(expr ast.Expression, modifiedVars map[string]bool) bool {
	if expr == nil {
		return true
	}

	switch e := expr.(type) {
	case *ast.IntegerLiteral, *ast.FloatLiteral, *ast.StringLiteral, *ast.BoolLiteral, *ast.NullLiteral:
		// 字面量是不变式
		return true
	case *ast.Variable:
		// 变量：不在修改列表中且不在循环中修改
		return !modifiedVars[e.Name] && !c.loopModifiedVars[e.Name]
	case *ast.BinaryExpr:
		// 二元表达式：左右两边都是不变式
		return c.isLoopInvariant(e.Left, modifiedVars) && c.isLoopInvariant(e.Right, modifiedVars)
	case *ast.UnaryExpr:
		// 一元表达式：操作数是不变式且没有副作用
		if e.Operator.Type == token.INCREMENT || e.Operator.Type == token.DECREMENT {
			return false
		}
		return c.isLoopInvariant(e.Operand, modifiedVars)
	case *ast.IndexExpr:
		// 索引访问：对象和索引都是不变式
		return c.isLoopInvariant(e.Object, modifiedVars) && c.isLoopInvariant(e.Index, modifiedVars)
	case *ast.PropertyAccess:
		// 属性访问：对象是不变式
		return c.isLoopInvariant(e.Object, modifiedVars)
	default:
		// 其他类型保守处理，认为不是不变式
		return false
	}
}

// hoistLoopInvariants 从循环体中提取不变式
func (c *Compiler) hoistLoopInvariants(body *ast.BlockStmt, modifiedVars map[string]bool) []hoistedExpr {
	hoisted := make([]hoistedExpr, 0)
	seen := make(map[string]bool) // 避免重复外提相同表达式
	
	// 遍历循环体中的所有表达式
	ast.Walk(body, func(node ast.Node) bool {
		var expr ast.Expression
		
		// 收集需要检查的表达式
		switch n := node.(type) {
		case *ast.BinaryExpr:
			// 检查整个二元表达式
			if c.isLoopInvariant(n, modifiedVars) {
				expr = n
			}
		case *ast.UnaryExpr:
			// 检查一元表达式（排除有副作用的）
			if c.isLoopInvariant(n, modifiedVars) {
				expr = n
			}
		case *ast.AssignExpr:
			// 检查赋值右侧
			if c.isLoopInvariant(n.Right, modifiedVars) {
				expr = n.Right
			}
		case *ast.VarDeclStmt:
			// 检查变量初始值
			if n.Value != nil && c.isLoopInvariant(n.Value, modifiedVars) {
				expr = n.Value
			}
		}
		
		if expr != nil {
			sig := c.computeExprSignature(expr)
			if sig != "" && !seen[sig] {
				// 只外提简单表达式，避免过度复杂
				if c.isSimpleInvariant(expr) {
					seen[sig] = true
					tempVar := fmt.Sprintf("$__licm_%d", len(hoisted))
					hoisted = append(hoisted, hoistedExpr{
						Expr:    expr,
						TempVar: tempVar,
					})
				}
			}
		}
		
		return true
	})
	
	return hoisted
}

// isSimpleInvariant 检查表达式是否适合外提（避免外提过于复杂的表达式）
func (c *Compiler) isSimpleInvariant(expr ast.Expression) bool {
	switch e := expr.(type) {
	case *ast.BinaryExpr:
		// 二元表达式：只有操作数都是变量或字面量才外提
		_, leftIsSimple := e.Left.(*ast.Variable)
		_, leftIsLiteral := e.Left.(*ast.IntegerLiteral)
		_, leftIsFloat := e.Left.(*ast.FloatLiteral)
		_, rightIsSimple := e.Right.(*ast.Variable)
		_, rightIsLiteral := e.Right.(*ast.IntegerLiteral)
		_, rightIsFloat := e.Right.(*ast.FloatLiteral)
		
		if (leftIsSimple || leftIsLiteral || leftIsFloat) && 
		   (rightIsSimple || rightIsLiteral || rightIsFloat) {
			return true
		}
		// 也允许嵌套一层
		if binLeft, ok := e.Left.(*ast.BinaryExpr); ok {
			return c.isSimpleInvariant(binLeft) && (rightIsSimple || rightIsLiteral || rightIsFloat)
		}
		if binRight, ok := e.Right.(*ast.BinaryExpr); ok {
			return (leftIsSimple || leftIsLiteral || leftIsFloat) && c.isSimpleInvariant(binRight)
		}
		return false
	case *ast.UnaryExpr:
		// 一元表达式：操作数是变量或字面量
		_, isSimple := e.Operand.(*ast.Variable)
		_, isLiteral := e.Operand.(*ast.IntegerLiteral)
		_, isFloat := e.Operand.(*ast.FloatLiteral)
		return isSimple || isLiteral || isFloat
	case *ast.Variable:
		return true
	case *ast.IntegerLiteral, *ast.FloatLiteral, *ast.StringLiteral, *ast.BoolLiteral:
		return true
	default:
		return false
	}
}

func (c *Compiler) compileWhileStmt(s *ast.WhileStmt) {
	// LICM: 分析循环体，收集被修改的变量
	modifiedVars := c.collectModifiedVars(s.Body)
	c.loopModifiedVars = modifiedVars
	c.inLoopAnalysis = true
	
	// LICM: 提取循环不变式
	// 注意：while 循环的条件通常不能外提，因为需要每次迭代检查
	// 这里主要外提循环体内的不变式
	hoistedExprs := c.hoistLoopInvariants(s.Body, modifiedVars)
	
	c.inLoopAnalysis = false
	
	// 编译外提的表达式并添加到 CSE 缓存
	for _, h := range hoistedExprs {
		c.compileExpr(h.Expr)
		c.declareVariableWithType(h.TempVar, "")
		c.defineVariable()
		// 将表达式签名添加到 CSE 缓存
		sig := c.computeExprSignature(h.Expr)
		if sig != "" {
			c.exprCache[sig] = c.localCount - 1
		}
		// 将临时变量标记为已修改，避免在循环中被优化掉
		c.loopModifiedVars[h.TempVar] = true
	}
	
	loopStart := c.currentChunk().Len()
	prevLoopStart := c.loopStart
	prevBreakJumps := c.breakJumps
	c.loopStart = loopStart
	c.breakJumps = nil
	c.loopDepth++

	// 无效分支消除：检查条件是否为常量
	if c.isConstExpr(s.Condition) {
		condVal := c.evaluateConstExpr(s.Condition)
		if !condVal.IsTruthy() {
			// while(false) - 不编译循环体，直接跳过
			c.loopStart = prevLoopStart
			c.breakJumps = prevBreakJumps
			c.loopDepth--
			c.loopModifiedVars = make(map[string]bool)
			return
		}
		// while(true) - 无限循环，不生成条件检查
		// 编译循环体
		c.compileStmt(s.Body)
		// 跳回循环开始（无限循环）
		c.emitLoop(loopStart)
		// 修补所有 break
		for _, jump := range c.breakJumps {
			c.patchJump(jump)
		}
		c.loopStart = prevLoopStart
		c.breakJumps = prevBreakJumps
		c.loopDepth--
		c.loopModifiedVars = make(map[string]bool)
		return
	}

	// 编译条件
	c.compileExpr(s.Condition)
	exitJump := c.emitJump(bytecode.OpJumpIfFalse)
	c.emit(bytecode.OpPop)

	// 编译循环体
	c.compileStmt(s.Body)

	// 跳回循环开始
	c.emitLoop(loopStart)

	// 修补退出跳转
	c.patchJump(exitJump)
	c.emit(bytecode.OpPop)

	// 修补所有 break
	for _, jump := range c.breakJumps {
		c.patchJump(jump)
	}

	c.loopStart = prevLoopStart
	c.breakJumps = prevBreakJumps
	c.loopDepth--
	
	// 清理循环修改变量记录
	c.loopModifiedVars = make(map[string]bool)
}

func (c *Compiler) compileForStmt(s *ast.ForStmt) {
	c.beginScope()

	// 初始化
	if s.Init != nil {
		c.compileStmt(s.Init)
	}

	// LICM: 分析循环体，收集被修改的变量
	modifiedVars := c.collectModifiedVars(s.Body)
	c.loopModifiedVars = modifiedVars
	c.inLoopAnalysis = true
	
	// LICM: 提取循环不变式
	hoistedExprs := c.hoistLoopInvariants(s.Body, modifiedVars)
	
	c.inLoopAnalysis = false
	
	// 编译外提的表达式并添加到 CSE 缓存
	for _, h := range hoistedExprs {
		c.compileExpr(h.Expr)
		c.declareVariableWithType(h.TempVar, "")
		c.defineVariable()
		// 将表达式签名添加到 CSE 缓存，这样循环体中遇到相同表达式时会自动复用
		sig := c.computeExprSignature(h.Expr)
		if sig != "" {
			c.exprCache[sig] = c.localCount - 1
		}
		// 将临时变量标记为已修改，避免在循环中被优化掉
		c.loopModifiedVars[h.TempVar] = true
	}

	loopStart := c.currentChunk().Len()
	prevLoopStart := c.loopStart
	prevBreakJumps := c.breakJumps
	c.loopStart = loopStart
	c.breakJumps = nil
	c.loopDepth++

	// 无效分支消除：检查条件是否为常量
	var exitJump int
	if s.Condition != nil {
		if c.isConstExpr(s.Condition) {
			condVal := c.evaluateConstExpr(s.Condition)
			if !condVal.IsTruthy() {
				// for(;;false) - 不编译循环体，直接跳过
				c.loopStart = prevLoopStart
				c.breakJumps = prevBreakJumps
				c.loopDepth--
				c.endScope()
				c.loopModifiedVars = make(map[string]bool)
				return
			}
			// for(;;true) - 无限循环，不生成条件检查
			// 编译循环体
			c.compileStmt(s.Body)
			// 后置表达式
			if s.Post != nil {
				c.compileExpr(s.Post)
				c.emit(bytecode.OpPop)
			}
			// 跳回循环开始（无限循环）
			c.emitLoop(loopStart)
			// 修补 break
			for _, jump := range c.breakJumps {
				c.patchJump(jump)
			}
			c.loopStart = prevLoopStart
			c.breakJumps = prevBreakJumps
			c.loopDepth--
			c.endScope()
			c.loopModifiedVars = make(map[string]bool)
			return
		}
		// 非常量条件，使用正常的跳转逻辑
		c.compileExpr(s.Condition)
		exitJump = c.emitJump(bytecode.OpJumpIfFalse)
		c.emit(bytecode.OpPop)
	}

	// 编译循环体
	// 使用 CSE 机制：外提的表达式已经在临时变量中，CSE 会自动识别并复用
	// 需要在编译时暂时禁用 CSE 缓存新的表达式，只使用已缓存的外提变量
	c.compileStmt(s.Body)

	// 后置表达式
	if s.Post != nil {
		c.compileExpr(s.Post)
		c.emit(bytecode.OpPop)
	}

	// 跳回循环开始
	c.emitLoop(loopStart)

	// 修补退出
	if s.Condition != nil {
		c.patchJump(exitJump)
		c.emit(bytecode.OpPop)
	}

	// 修补 break
	for _, jump := range c.breakJumps {
		c.patchJump(jump)
	}

	c.loopStart = prevLoopStart
	c.breakJumps = prevBreakJumps
	c.loopDepth--
	c.endScope()
	
	// 清理循环修改变量记录
	c.loopModifiedVars = make(map[string]bool)
}


func (c *Compiler) compileForeachStmt(s *ast.ForeachStmt) {
	c.beginScope()

	// LICM: 分析循环体，收集被修改的变量
	modifiedVars := c.collectModifiedVars(s.Body)
	// 添加循环变量到修改列表（它们在每次迭代中都会改变）
	if s.Key != nil {
		modifiedVars[s.Key.Name] = true
	}
	modifiedVars[s.Value.Name] = true
	c.loopModifiedVars = modifiedVars
	c.inLoopAnalysis = true
	
	// LICM: 提取循环不变式
	hoistedExprs := c.hoistLoopInvariants(s.Body, modifiedVars)
	
	c.inLoopAnalysis = false

	// 推断迭代对象类型，用于确定 key 和 value 的类型
	iterableType := c.inferExprType(s.Iterable)
	keyType := "dynamic"
	valueType := "dynamic"
	
	// 根据可迭代对象类型确定 key/value 类型
	if strings.HasSuffix(iterableType, "[]") {
		// 数组：key 是 int，value 是元素类型
		keyType = "int"
		valueType = strings.TrimSuffix(iterableType, "[]")
	} else if strings.HasPrefix(iterableType, "map[") {
		// Map：从 map[K]V 中提取 K 和 V
		if idx := strings.Index(iterableType, "]"); idx != -1 {
			keyType = iterableType[4:idx]
			valueType = iterableType[idx+1:]
		}
	}
	// SuperArray 和其他类型使用默认的 "dynamic"

	// 编译迭代对象并创建迭代器
	c.compileExpr(s.Iterable)
	c.emit(bytecode.OpIterInit) // 栈上: [iterator]
	
	// 迭代器变量（内部使用）- 迭代器已经在栈顶
	iterSlot := c.localCount
	c.addLocal("$__iter__")
	// 不需要 StoreLocal，迭代器已经在正确的栈位置
	
	// 声明 key 变量 (如果有)
	keySlot := -1
	if s.Key != nil {
		c.emit(bytecode.OpNull)
		keySlot = c.localCount
		c.addLocalWithType(s.Key.Name, keyType)
	}
	
	// 声明 value 变量
	c.emit(bytecode.OpNull)
	valueSlot := c.localCount
	c.addLocalWithType(s.Value.Name, valueType)

	// 编译外提的表达式并添加到 CSE 缓存
	for _, h := range hoistedExprs {
		c.compileExpr(h.Expr)
		c.declareVariableWithType(h.TempVar, "")
		c.defineVariable()
		// 将表达式签名添加到 CSE 缓存
		sig := c.computeExprSignature(h.Expr)
		if sig != "" {
			c.exprCache[sig] = c.localCount - 1
		}
		// 将临时变量标记为已修改，避免在循环中被优化掉
		c.loopModifiedVars[h.TempVar] = true
	}

	// 循环开始
	loopStart := c.currentChunk().Len()
	prevLoopStart := c.loopStart
	prevBreakJumps := c.breakJumps
	c.loopStart = loopStart
	c.breakJumps = nil
	c.loopDepth++

	// 检查迭代器是否还有元素
	// 迭代器在 stack[iterSlot]，ITER_NEXT 使用 peek(0) 读取它
	c.emitU16(bytecode.OpLoadLocal, uint16(iterSlot))
	c.emit(bytecode.OpIterNext)
	// 栈: [iterator, bool]
	exitJump := c.emitJump(bytecode.OpJumpIfFalse)
	c.emit(bytecode.OpPop) // 弹出 bool
	c.emit(bytecode.OpPop) // 弹出 iterator (从 LOAD_LOCAL 加载的)

	// 获取 key 和 value
	if s.Key != nil {
		c.emitU16(bytecode.OpLoadLocal, uint16(iterSlot))
		c.emit(bytecode.OpIterKey)
		// 栈: [iterator, key]
		c.emitU16(bytecode.OpStoreLocal, uint16(keySlot))
		c.emit(bytecode.OpPop) // 弹出 key
		c.emit(bytecode.OpPop) // 弹出 iterator
	}
	
	c.emitU16(bytecode.OpLoadLocal, uint16(iterSlot))
	c.emit(bytecode.OpIterValue)
	// 栈: [iterator, value]
	c.emitU16(bytecode.OpStoreLocal, uint16(valueSlot))
	c.emit(bytecode.OpPop) // 弹出 value
	c.emit(bytecode.OpPop) // 弹出 iterator

	// 循环体
	c.compileStmt(s.Body)

	// 跳回循环开始
	c.emitLoop(loopStart)

	// 修补退出跳转
	c.patchJump(exitJump)
	c.emit(bytecode.OpPop) // 弹出 bool
	c.emit(bytecode.OpPop) // 弹出 iterator

	// 修补所有 break
	for _, jump := range c.breakJumps {
		c.patchJump(jump)
	}

	c.loopStart = prevLoopStart
	c.breakJumps = prevBreakJumps
	c.loopDepth--
	c.endScope()
}

func (c *Compiler) compileSwitchStmt(s *ast.SwitchStmt) {
	// 穷尽性检查：如果 switch 表达式是枚举类型，检查是否覆盖所有值
	exprType := c.inferExprType(s.Expr)
	c.checkSwitchExhaustiveness(s, exprType)
	
	c.compileExpr(s.Expr)

	var endJumps []int

	for _, caseClause := range s.Cases {
		// 复制 switch 表达式值
		c.emit(bytecode.OpDup)
		c.compileExpr(caseClause.Value)
		c.emit(bytecode.OpEq)

		nextCase := c.emitJump(bytecode.OpJumpIfFalse)
		c.emit(bytecode.OpPop)

		// case 体
		for _, stmt := range caseClause.Body {
			c.compileStmt(stmt)
		}

		endJumps = append(endJumps, c.emitJump(bytecode.OpJump))
		c.patchJump(nextCase)
		c.emit(bytecode.OpPop)
	}

	// default
	if s.Default != nil {
		for _, stmt := range s.Default.Body {
			c.compileStmt(stmt)
		}
	}

	// 修补所有结束跳转
	for _, jump := range endJumps {
		c.patchJump(jump)
	}

	c.emit(bytecode.OpPop) // 弹出 switch 表达式
}

// checkSwitchExhaustiveness 检查 switch 语句的穷尽性
// 如果 switch 表达式是枚举类型且没有 default 分支，检查是否覆盖了所有枚举值
func (c *Compiler) checkSwitchExhaustiveness(s *ast.SwitchStmt, exprType string) {
	// 如果有 default 分支，无需检查穷尽性
	if s.Default != nil {
		return
	}
	
	// 检查是否是枚举类型
	enumValues := c.symbolTable.GetEnumValues(exprType)
	if len(enumValues) == 0 {
		return // 不是枚举类型，跳过检查
	}
	
	// 收集所有 case 覆盖的值
	coveredValues := make(map[string]bool)
	for _, caseClause := range s.Cases {
		// 尝试从 case 值中提取枚举成员名
		if sa, ok := caseClause.Value.(*ast.StaticAccess); ok {
			if member, ok := sa.Member.(*ast.Identifier); ok {
				coveredValues[member.Name] = true
			}
		}
	}
	
	// 检查是否所有枚举值都被覆盖
	var missingValues []string
	for _, val := range enumValues {
		if !coveredValues[val] {
			missingValues = append(missingValues, val)
		}
	}
	
	if len(missingValues) > 0 {
		// 发出警告而不是错误，允许代码继续编译
		c.error(s.Expr.Pos(), i18n.T(i18n.ErrSwitchNotExhaustive, exprType, strings.Join(missingValues, ", ")))
	}
}

func (c *Compiler) compileBreakStmt() {
	if c.loopDepth == 0 {
		c.error(token.Position{}, i18n.T(i18n.ErrBreakOutsideLoop))
		return
	}
	jump := c.emitJump(bytecode.OpJump)
	c.breakJumps = append(c.breakJumps, jump)
}

func (c *Compiler) compileContinueStmt() {
	if c.loopDepth == 0 {
		c.error(token.Position{}, i18n.T(i18n.ErrContinueOutsideLoop))
		return
	}
	c.emitLoop(c.loopStart)
}

func (c *Compiler) compileReturnStmt(s *ast.ReturnStmt) {
	actualReturns := len(s.Values)
	
	// expectedReturns == -1 表示不检查（用于箭头函数等）
	if c.expectedReturns >= 0 {
		// 检查返回值数量
		if c.expectedReturns == 0 {
			// 预期无返回值 (void 或省略)
			if actualReturns > 0 {
				c.error(s.Pos(), i18n.T(i18n.ErrNoReturnExpected, actualReturns))
			}
			c.emit(bytecode.OpReturnNull)
			return
		}
		
		if c.expectedReturns > 0 && actualReturns != c.expectedReturns {
			c.error(s.Pos(), i18n.T(i18n.ErrReturnCountMismatch, c.expectedReturns, actualReturns))
		}
		
		// 检查返回值类型
		if c.returnType != nil && actualReturns > 0 {
			if tuple, ok := c.returnType.(*ast.TupleType); ok {
				// 多返回值类型检查
				for i, val := range s.Values {
					if i < len(tuple.Types) {
						c.checkReturnType(val.Pos(), val, tuple.Types[i])
					}
				}
			} else if actualReturns == 1 {
				// 单返回值类型检查
				c.checkReturnType(s.Values[0].Pos(), s.Values[0], c.returnType)
			}
		}
	}
	
	if actualReturns == 0 {
		c.emit(bytecode.OpReturnNull)
	} else if actualReturns == 1 {
		// 单返回值 - 检查是否是尾调用
		if callExpr, ok := s.Values[0].(*ast.CallExpr); ok {
			// 检测尾调用：return fn(...)
			if c.isTailCallable(callExpr) {
				c.compileTailCall(callExpr)
				return
			}
		}
		// 非尾调用，正常编译
		c.compileExpr(s.Values[0])
		c.emit(bytecode.OpReturn)
	} else {
		// 多返回值：用数组包装
		for _, val := range s.Values {
			c.compileExpr(val)
		}
		c.emitU16(bytecode.OpNewArray, uint16(len(s.Values)))
		c.emit(bytecode.OpReturn)
	}
}

func (c *Compiler) compileTryStmt(s *ast.TryStmt) {
	hasFinally := s.Finally != nil
	catchCount := len(s.Catches)
	
	// 发出进入 try 块指令
	// 格式: OpEnterTry catchCount:u8 finallyOffset:i16 [typeIdx:u16 catchOffset:i16]*catchCount
	enterTryIP := c.currentChunk().Len() // OpEnterTry 指令的位置
	c.emit(bytecode.OpEnterTry)
	enterTryPos := c.currentChunk().Len() // OpEnterTry 之后的位置（用于定位参数）
	
	// 写入 catch 数量
	c.currentChunk().WriteU8(uint8(catchCount), c.currentLine)
	
	// finally 偏移量占位
	c.currentChunk().WriteI16(0, c.currentLine)
	
	// 为每个 catch 处理器预留空间 (typeIdx: u16, catchOffset: i16)
	catchHandlerPos := c.currentChunk().Len()
	for i := 0; i < catchCount; i++ {
		c.currentChunk().WriteU16(0, c.currentLine) // typeIdx 占位
		c.currentChunk().WriteI16(0, c.currentLine) // catchOffset 占位
	}
	
	// 编译 try 块
	c.compileStmt(s.Try)
	
	// 离开 try 块（正常流程）
	c.emit(bytecode.OpLeaveTry)
	
	// 如果有 finally，正常流程需要跳转到 finally；否则跳过所有 catch 块
	var normalExitJump int
	normalExitJump = c.emitJump(bytecode.OpJump)
	
	// 编译每个 catch 块，并记录偏移量
	var catchEndJumps []int
	for i, catch := range s.Catches {
		// 记录这个 catch 块的开始位置
		catchBlockStart := c.currentChunk().Len()
		
		// 获取异常类型名
		typeName := "Exception" // 默认
		if catch.Type != nil {
			typeName = c.getTypeName(catch.Type)
		}
		
		// 将类型名添加到常量池
		typeIdx := c.makeConstant(bytecode.NewString(typeName))
		
		// 计算从 enterTryIP 到 catch 块的偏移量
		catchOffset := int16(catchBlockStart - enterTryIP)
		
		// 修补 catch 处理器信息
		handlerOffset := catchHandlerPos + i*4
		c.currentChunk().Code[handlerOffset] = byte(typeIdx >> 8)
		c.currentChunk().Code[handlerOffset+1] = byte(typeIdx)
		c.currentChunk().Code[handlerOffset+2] = byte(catchOffset >> 8)
		c.currentChunk().Code[handlerOffset+3] = byte(catchOffset)
		
		// 发出 OpEnterCatch 指令，带上类型索引
		c.emitU16(bytecode.OpEnterCatch, typeIdx)
		
		c.beginScope()
		
		// 异常值已经在栈上
		if catch.Variable != nil {
			c.addLocal(catch.Variable.Name)
		} else {
			c.emit(bytecode.OpPop)
		}
		
		// 编译 catch 体
		c.compileStmt(catch.Body)
		c.endScope()
		
		// catch 块结束后跳转（跳过其他 catch 块，到 finally 或结束）
		catchEndJumps = append(catchEndJumps, c.emitJump(bytecode.OpJump))
	}
	
	// 如果没有 catch 块但有 finally，需要处理未捕获的异常
	if catchCount == 0 && hasFinally {
		// 这种情况下异常会直接传播到 finally
	}
	
	// finally 块
	if hasFinally {
		// 修补正常退出跳转
		c.patchJump(normalExitJump)
		
		// 修补所有 catch 块的结束跳转
		for _, jump := range catchEndJumps {
			c.patchJump(jump)
		}
		
		// finally 开始位置
		finallyStart := c.currentChunk().Len()
		
		// 修补 finally 偏移量（相对于 enterTryIP）
		finallyOffset := int16(finallyStart - enterTryIP)
		c.currentChunk().Code[enterTryPos+1] = byte(finallyOffset >> 8)
		c.currentChunk().Code[enterTryPos+2] = byte(finallyOffset)
		
		c.emit(bytecode.OpEnterFinally)
		
		// 编译 finally 块
		c.compileStmt(s.Finally.Body)
		
		c.emit(bytecode.OpLeaveFinally)
	} else {
		// 没有 finally
		// 修补正常退出跳转
		c.patchJump(normalExitJump)
		
		// 修补所有 catch 块的结束跳转
		for _, jump := range catchEndJumps {
			c.patchJump(jump)
		}
		
		// finally 偏移量设为 -1（没有 finally）
		c.currentChunk().Code[enterTryPos+1] = 0xFF
		c.currentChunk().Code[enterTryPos+2] = 0xFF
	}
}

// ============================================================================
// 表达式编译
// ============================================================================

func (c *Compiler) compileExpr(expr ast.Expression) {
	// 更新当前行号
	c.currentLine = expr.Pos().Line
	
	// CSE: 检查是否可以复用已计算的表达式
	// 注意：简单字面量不需要 CSE，因为它们已经很高效
	if c.cseEnabled && !c.hasSideEffect(expr) && c.loopDepth > 0 && !c.isSimpleLiteral(expr) {
		sig := c.computeExprSignature(expr)
		if sig != "" {
			if slot, exists := c.exprCache[sig]; exists {
				// 复用缓存值
				c.emitU16(bytecode.OpLoadLocal, uint16(slot))
				return
			}
		}
	}
	
	// 记录是否需要缓存（在表达式编译完成后）
	// 注意：简单字面量不需要缓存，只缓存复杂表达式
	var needCache bool
	var cacheSig string
	if c.cseEnabled && !c.hasSideEffect(expr) && c.loopDepth > 0 && !c.isSimpleLiteral(expr) {
		cacheSig = c.computeExprSignature(expr)
		if cacheSig != "" {
			needCache = true
		}
	}
	
	switch e := expr.(type) {
	case *ast.IntegerLiteral:
		c.emitConstant(bytecode.NewInt(e.Value))

	case *ast.FloatLiteral:
		c.emitConstant(bytecode.NewFloat(e.Value))

	case *ast.StringLiteral:
		c.emitConstant(bytecode.NewString(e.Value))

	case *ast.InterpStringLiteral:
		// 编译插值字符串的每个部分并拼接
		if len(e.Parts) == 0 {
			c.emitConstant(bytecode.NewString(""))
		} else {
			for i, part := range e.Parts {
				c.compileExpr(part)
				if i > 0 {
					c.emit(bytecode.OpConcat)
				}
			}
		}

	case *ast.BoolLiteral:
		if e.Value {
			c.emit(bytecode.OpTrue)
		} else {
			c.emit(bytecode.OpFalse)
		}

	case *ast.NullLiteral:
		c.emit(bytecode.OpNull)

	case *ast.Variable:
		c.compileVariable(e)

	case *ast.ThisExpr:
		c.compileThis()

	case *ast.Identifier:
		c.compileIdentifier(e)

	case *ast.ArrayLiteral:
		for _, elem := range e.Elements {
			c.compileExpr(elem)
		}
		c.emitU16(bytecode.OpNewArray, uint16(len(e.Elements)))

	case *ast.MapLiteral:
		// Map 类型一致性检查
		if len(e.Pairs) > 0 {
			// 从第一个元素推断类型
			firstKeyType := c.inferExprType(e.Pairs[0].Key)
			firstValueType := c.inferExprType(e.Pairs[0].Value)
			
			// 检查 interface 类型不允许推导
			if c.isInterfaceType(firstKeyType) {
				c.error(e.Pairs[0].Key.Pos(), i18n.T(i18n.ErrCannotInferInterface, firstKeyType))
			}
			if c.isInterfaceType(firstValueType) {
				c.error(e.Pairs[0].Value.Pos(), i18n.T(i18n.ErrCannotInferInterface, firstValueType))
			}
			
			// 检查后续元素类型一致性（只有当类型已知时才检查）
			for i := 1; i < len(e.Pairs); i++ {
				keyType := c.inferExprType(e.Pairs[i].Key)
				valueType := c.inferExprType(e.Pairs[i].Value)
				
				if keyType != "" && firstKeyType != "" && keyType != firstKeyType {
					c.error(e.Pairs[i].Key.Pos(), i18n.T(i18n.ErrMapKeyTypeMismatch, firstKeyType, keyType))
				}
				if valueType != "" && firstValueType != "" && valueType != firstValueType {
					c.error(e.Pairs[i].Value.Pos(), i18n.T(i18n.ErrMapValueTypeMismatch, firstValueType, valueType))
				}
			}
		}
		
		// 继续正常编译
		for _, pair := range e.Pairs {
			c.compileExpr(pair.Key)
			c.compileExpr(pair.Value)
		}
		c.emitU16(bytecode.OpNewMap, uint16(len(e.Pairs)))

	case *ast.SuperArrayLiteral:
		c.compileSuperArrayLiteral(e)

	case *ast.UnaryExpr:
		c.compileUnaryExpr(e)

	case *ast.BinaryExpr:
		c.compileBinaryExpr(e)

	case *ast.TernaryExpr:
		c.compileTernaryExpr(e)

	case *ast.AssignExpr:
		c.compileAssignExpr(e)

	case *ast.CallExpr:
		c.compileCallExpr(e)

	case *ast.IndexExpr:
		c.compileIndexExpr(e)

	case *ast.PropertyAccess:
		c.compilePropertyAccess(e)

	case *ast.MethodCall:
		c.compileMethodCall(e)

	case *ast.StaticAccess:
		c.compileStaticAccess(e)

	case *ast.ClassAccessExpr:
		c.compileClassAccessExpr(e)

	case *ast.NewExpr:
		c.compileNewExpr(e)

	case *ast.ClosureExpr:
		c.compileClosureExpr(e)

	case *ast.ArrowFuncExpr:
		c.compileArrowFuncExpr(e)

	case *ast.TypeCastExpr:
		c.compileTypeCastExpr(e)
	
	case *ast.IsExpr:
		c.compileIsExpr(e)

	case *ast.MatchExpr:
		c.compileMatchExpr(e)

	default:
		c.error(expr.Pos(), i18n.T(i18n.ErrUnsupportedExpr))
	}
	
	// CSE: 缓存表达式结果
	if needCache {
		c.cacheExprResult(cacheSig)
	}
}

func (c *Compiler) compileVariable(v *ast.Variable) {
	// 查找局部变量
	if idx := c.resolveLocal(v.Name); idx != -1 {
		c.emitU16(bytecode.OpLoadLocal, uint16(idx))
		return
	}

	// 在闭包中不能访问全局变量（必须通过 use 引入）
	if c.inClosure {
		c.error(v.Pos(), i18n.T(i18n.ErrUndefinedVariable, v.Name))
		return
	}

	// 全局变量
	idx := c.makeConstant(bytecode.NewString(v.Name))
	c.emitU16(bytecode.OpLoadGlobal, idx)
}

func (c *Compiler) compileThis() {
	// $this 是第一个局部变量 (在方法中)
	c.emitU16(bytecode.OpLoadLocal, 0)
}

func (c *Compiler) compileIdentifier(id *ast.Identifier) {
	// 可能是类名或全局函数
	idx := c.makeConstant(bytecode.NewString(id.Name))
	c.emitU16(bytecode.OpLoadGlobal, idx)
}

func (c *Compiler) compileUnaryExpr(e *ast.UnaryExpr) {
	// 常量折叠优化：尝试折叠常量表达式
	if c.isConstExpr(e.Operand) {
		foldedVal := c.foldUnaryExpr(e)
		if !foldedVal.IsNull() || isNullLiteral(e.Operand) {
			// 成功折叠，直接输出常量值
			c.emitConstant(foldedVal)
			return
		}
	}
	
	c.compileExpr(e.Operand)

	switch e.Operator.Type {
	case token.MINUS:
		c.emit(bytecode.OpNeg)
	case token.NOT:
		c.emit(bytecode.OpNot)
	case token.BIT_NOT:
		c.emit(bytecode.OpBitNot)
	case token.INCREMENT:
		if e.Prefix {
			// ++$x: 先加1，再使用
			// 栈: [value] -> [value+1] -> [value+1, value+1] -> 存储 -> [value+1]
			c.emit(bytecode.OpOne)
			c.emit(bytecode.OpAdd)
			c.emit(bytecode.OpDup)
			c.compileAssignTarget(e.Operand)
			c.emit(bytecode.OpPop) // 弹出存储后的多余值
		} else {
			// $x++: 先使用旧值，再加1
			// 栈: [value] -> [value, value] -> [value, value+1] -> 存储 -> [value, value+1] -> pop -> [value]
			c.emit(bytecode.OpDup)    // 复制旧值用于返回
			c.emit(bytecode.OpOne)
			c.emit(bytecode.OpAdd)
			c.compileAssignTarget(e.Operand)
			c.emit(bytecode.OpPop) // 弹出新值，保留旧值作为表达式结果
		}
	case token.DECREMENT:
		if e.Prefix {
			// --$x: 先减1，再使用
			c.emit(bytecode.OpOne)
			c.emit(bytecode.OpSub)
			c.emit(bytecode.OpDup)
			c.compileAssignTarget(e.Operand)
			c.emit(bytecode.OpPop)
		} else {
			// $x--: 先使用旧值，再减1
			c.emit(bytecode.OpDup)
			c.emit(bytecode.OpOne)
			c.emit(bytecode.OpSub)
			c.compileAssignTarget(e.Operand)
			c.emit(bytecode.OpPop)
		}
	}
}

func (c *Compiler) compileBinaryExpr(e *ast.BinaryExpr) {
	// 短路运算
	if e.Operator.Type == token.AND {
		c.compileExpr(e.Left)
		endJump := c.emitJump(bytecode.OpJumpIfFalse)
		c.emit(bytecode.OpPop)
		c.compileExpr(e.Right)
		c.patchJump(endJump)
		return
	}

	if e.Operator.Type == token.OR {
		c.compileExpr(e.Left)
		elseJump := c.emitJump(bytecode.OpJumpIfFalse)
		endJump := c.emitJump(bytecode.OpJump)
		c.patchJump(elseJump)
		c.emit(bytecode.OpPop)
		c.compileExpr(e.Right)
		c.patchJump(endJump)
		return
	}

	// 常量折叠优化：尝试折叠常量表达式
	if c.isConstExpr(e.Left) && c.isConstExpr(e.Right) {
		foldedVal := c.foldBinaryExpr(e)
		if !foldedVal.IsNull() || (isNullLiteral(e.Left) && isNullLiteral(e.Right)) {
			// 成功折叠，直接输出常量值
			c.emitConstant(foldedVal)
			return
		}
	}

	// 类型检查：对于算术运算符，检查操作数类型是否兼容
	leftType := c.inferExprType(e.Left)
	rightType := c.inferExprType(e.Right)
	
	// 只有当两边类型都已知时才进行检查（空字符串表示无法推断）
	if leftType != "" && rightType != "" {
		c.checkBinaryOpTypes(e.Operator, leftType, rightType)
	}

	c.compileExpr(e.Left)
	c.compileExpr(e.Right)

	switch e.Operator.Type {
	case token.PLUS:
		c.emit(bytecode.OpAdd)
	case token.MINUS:
		c.emit(bytecode.OpSub)
	case token.STAR:
		c.emit(bytecode.OpMul)
	case token.SLASH:
		c.emit(bytecode.OpDiv)
	case token.PERCENT:
		c.emit(bytecode.OpMod)
	case token.EQ:
		c.emit(bytecode.OpEq)
	case token.NE:
		c.emit(bytecode.OpNe)
	case token.LT:
		c.emit(bytecode.OpLt)
	case token.LE:
		c.emit(bytecode.OpLe)
	case token.GT:
		c.emit(bytecode.OpGt)
	case token.GE:
		c.emit(bytecode.OpGe)
	case token.BIT_AND:
		c.emit(bytecode.OpBitAnd)
	case token.BIT_OR:
		c.emit(bytecode.OpBitOr)
	case token.BIT_XOR:
		c.emit(bytecode.OpBitXor)
	case token.LEFT_SHIFT:
		c.emit(bytecode.OpShl)
	case token.RIGHT_SHIFT:
		c.emit(bytecode.OpShr)
	}
}

func (c *Compiler) compileTernaryExpr(e *ast.TernaryExpr) {
	// 无效分支消除：检查条件是否为常量
	if c.isConstExpr(e.Condition) {
		condVal := c.evaluateConstExpr(e.Condition)
		if condVal.IsTruthy() {
			// 条件为真，只编译 then 分支
			c.compileExpr(e.Then)
		} else {
			// 条件为假，只编译 else 分支
			c.compileExpr(e.Else)
		}
		return
	}
	
	// 非常量条件，使用正常的跳转逻辑
	c.compileExpr(e.Condition)
	elseJump := c.emitJump(bytecode.OpJumpIfFalse)
	c.emit(bytecode.OpPop)
	c.compileExpr(e.Then)
	endJump := c.emitJump(bytecode.OpJump)
	c.patchJump(elseJump)
	c.emit(bytecode.OpPop)
	c.compileExpr(e.Else)
	c.patchJump(endJump)
}

// compileSuperArrayLiteral 编译 PHP 风格万能数组字面量
func (c *Compiler) compileSuperArrayLiteral(e *ast.SuperArrayLiteral) {
	// 编译所有元素，标记键值对
	// 对于每个元素：先编译 key（如果有），再编译 value
	// 使用标志字节标记每个元素是否是键值对

	elementCount := len(e.Elements)

	// 编译每个元素
	for _, elem := range e.Elements {
		if elem.Key != nil {
			// 键值对: 先 key 后 value
			c.compileExpr(elem.Key)
			c.compileExpr(elem.Value)
			// 压入标志 1 表示是键值对
			c.emitConstant(bytecode.NewInt(1))
		} else {
			// 仅值: 自动索引
			c.compileExpr(elem.Value)
			// 压入标志 0 表示非键值对
			c.emitConstant(bytecode.NewInt(0))
		}
	}

	// 发射创建万能数组指令，携带元素数量
	c.emitU16(bytecode.OpSuperArrayNew, uint16(elementCount))
}

func (c *Compiler) compileAssignExpr(e *ast.AssignExpr) {
	// 静态类型检查：变量赋值
	if v, ok := e.Left.(*ast.Variable); ok {
		varType := c.getVariableType(v.Name)
		if varType != "" {
			rightType := c.inferExprType(e.Right)
			if rightType != "" && !c.isTypeCompatible(rightType, varType) {
				c.error(e.Right.Pos(), i18n.T(i18n.ErrCannotAssign, rightType, varType))
			}
		}
	}
	
	// 静态类型检查：属性赋值
	if prop, ok := e.Left.(*ast.PropertyAccess); ok {
		objType := c.inferExprType(prop.Object)
		if objType != "" {
			propSig := c.symbolTable.GetProperty(objType, prop.Property.Name)
			if propSig != nil && propSig.Type != "" && propSig.Type != "dynamic" {
				rightType := c.inferExprType(e.Right)
				if rightType != "" && !c.isTypeCompatible(rightType, propSig.Type) {
					c.error(e.Right.Pos(), i18n.T(i18n.ErrCannotAssign, rightType, propSig.Type))
				}
			}
		}
	}
	
	// 特殊处理数组索引赋值：OpArraySet 期望栈顺序为 [array, index, value] (底到顶)
	if idx, ok := e.Left.(*ast.IndexExpr); ok {
		c.compileExpr(idx.Object)  // array
		c.compileExpr(idx.Index)   // index
		c.compileExpr(e.Right)     // value
		
		// 复合赋值暂不支持索引操作
		if e.Operator.Type != token.ASSIGN {
			c.error(e.Pos(), i18n.T(i18n.ErrCompoundAssignIndex))
			return
		}
		
		// OpArraySet 会弹出 value, idx, array，然后 push value 作为表达式结果
		c.emit(bytecode.OpArraySet)
		return
	}
	
	// 其他情况的赋值
	// 复合赋值
	if e.Operator.Type != token.ASSIGN {
		c.compileExpr(e.Left)
		c.compileExpr(e.Right)
		switch e.Operator.Type {
		case token.PLUS_ASSIGN:
			c.emit(bytecode.OpAdd)
		case token.MINUS_ASSIGN:
			c.emit(bytecode.OpSub)
		case token.STAR_ASSIGN:
			c.emit(bytecode.OpMul)
		case token.SLASH_ASSIGN:
			c.emit(bytecode.OpDiv)
		case token.PERCENT_ASSIGN:
			c.emit(bytecode.OpMod)
		}
	} else {
		c.compileExpr(e.Right)
	}

	// 存储到目标 - OpStoreGlobal/OpStoreLocal 使用 peek 而不是 pop
	// 所以值会留在栈上作为表达式结果，不需要额外 DUP
	if _, ok := e.Left.(*ast.StaticAccess); ok {
		c.emit(bytecode.OpDup) // 静态变量需要 dup 因为 OpSetStatic 会弹出
		c.compileAssignTarget(e.Left)
		c.emit(bytecode.OpPop) // 弹出 OpSetStatic 返回的值
	} else {
		c.compileAssignTarget(e.Left)
	}
}

func (c *Compiler) compileAssignTarget(target ast.Expression) {
	switch t := target.(type) {
	case *ast.Variable:
		if idx := c.resolveLocal(t.Name); idx != -1 {
			c.emitU16(bytecode.OpStoreLocal, uint16(idx))
		} else if _, ok := c.globalTypes[t.Name]; ok {
			// 只有已声明的全局变量才能赋值
			idx := c.makeConstant(bytecode.NewString(t.Name))
			c.emitU16(bytecode.OpStoreGlobal, idx)
		} else {
			// 变量未声明，报错
			c.error(t.Pos(), i18n.T(i18n.ErrUndeclaredVariable, t.Name))
		}
	case *ast.IndexExpr:
		// 栈上现在有值，需要按 array, index, value 顺序排列
		// 当前栈：[..., value]
		c.compileExpr(t.Object)  // 栈：[..., value, array]
		c.compileExpr(t.Index)   // 栈：[..., value, array, index]
		// 交换顺序使其变为 [array, index, value]
		// 需要一个临时方法来重新排列栈，或者我们改变 OpArraySet 的期望
		// 简单方法：emit rotations
		// 更简单的方法：修改编译方式
		// 先弹出 value 保存，编译 array 和 index，然后把 value 放回
		// 但我们没有临时变量支持，所以改变 compileAssignExpr
		c.emit(bytecode.OpArraySet)
	case *ast.PropertyAccess:
		c.compileExpr(t.Object)
		idx := c.makeConstant(bytecode.NewString(t.Property.Name))
		c.emitU16(bytecode.OpSetField, idx)
	case *ast.StaticAccess:
		// 静态变量赋值
		var className string
		switch cls := t.Class.(type) {
		case *ast.Identifier:
			className = cls.Name
		case *ast.SelfExpr:
			className = "self"
		default:
			return
		}
		classIdx := c.makeConstant(bytecode.NewString(className))
		if v, ok := t.Member.(*ast.Variable); ok {
			nameIdx := c.makeConstant(bytecode.NewString(v.Name))
			c.emitU16(bytecode.OpSetStatic, classIdx)
			c.currentChunk().WriteU16(nameIdx, c.currentLine)
		}
	}
}

func (c *Compiler) compileCallExpr(e *ast.CallExpr) {
	// 特殊处理 unset() 函数
	if ident, ok := e.Function.(*ast.Identifier); ok && ident.Name == "unset" {
		if len(e.Arguments) != 1 && len(e.NamedArguments) != 0 {
			c.error(e.Pos(), "unset() requires exactly 1 argument")
			return
		}
		c.compileExpr(e.Arguments[0])
		c.emit(bytecode.OpUnset)
		return
	}
	
	// 检查 native_ 开头的函数只能在标准库中调用
	if ident, ok := e.Function.(*ast.Identifier); ok {
		if strings.HasPrefix(ident.Name, "native_") {
			// 标准库命名空间以 "sola." 开头
			if !strings.HasPrefix(c.currentNamespace, "sola.") {
				c.error(e.Pos(), i18n.T(i18n.ErrNativeFuncRestricted, ident.Name))
				return
			}
		}
	}
	
	// 静态类型检查：检查参数类型
	c.checkCallArgTypes(e)
	
	// 处理命名参数
	args := c.resolveNamedArguments(e)
	
	// 尝试内联：检查是否是函数标识符调用且可内联
	if ident, ok := e.Function.(*ast.Identifier); ok {
		if targetFn := c.compiledFunctions[ident.Name]; targetFn != nil && targetFn.Inlinable {
			// 防止自递归内联
			if ident.Name != c.currentFuncName {
				// 内联函数体
				c.inlineFunction(targetFn, args)
				return
			}
		}
	}
	
	// 非内联调用，使用正常调用
	c.compileExpr(e.Function)
	for _, arg := range args {
		c.compileExpr(arg)
	}
	c.emitByte(bytecode.OpCall, byte(len(args)))
}

// isTailCallable 检查调用表达式是否可以作为尾调用
// 尾调用条件：
// 1. 函数调用（Identifier 或 Variable）
// 2. 不是方法调用（PropertyAccess）
// 3. 不是闭包调用（ClosureExpr）
func (c *Compiler) isTailCallable(e *ast.CallExpr) bool {
	// 必须是函数调用，不能是方法调用
	switch e.Function.(type) {
	case *ast.Identifier, *ast.Variable:
		// 普通函数调用，可以尾调用
		return true
	case *ast.PropertyAccess:
		// 方法调用，不支持尾调用（需要 this 上下文）
		return false
	case *ast.ClosureExpr:
		// 闭包调用，不支持尾调用（可能有 upvalues）
		return false
	default:
		return false
	}
}

// compileTailCall 编译尾调用
// 尾调用使用 OpTailCall 指令，复用当前栈帧
func (c *Compiler) compileTailCall(e *ast.CallExpr) {
	// 处理命名参数
	args := c.resolveNamedArguments(e)
	
	// 编译函数和参数（与普通调用相同）
	c.compileExpr(e.Function)
	for _, arg := range args {
		c.compileExpr(arg)
	}
	
	// 使用 OpTailCall 而非 OpCall
	c.emitByte(bytecode.OpTailCall, byte(len(args)))
}

// isInlinable 检查函数是否可内联
// 内联启发式规则：
// 1. 函数体指令数 <= 20
// 2. 无闭包捕获（UpvalueCount == 0）
// 3. 非可变参数
// 4. 非内置函数
func (c *Compiler) isInlinable(fn *bytecode.Function) bool {
	// 内置函数不能内联
	if fn.IsBuiltin {
		return false
	}
	
	// 可变参数函数不能内联（参数处理复杂）
	if fn.IsVariadic {
		return false
	}
	
	// 有闭包捕获的函数不能内联（需要 upvalues）
	if fn.UpvalueCount > 0 {
		return false
	}
	
	// 函数体指令数限制（<= 20）
	if fn.Chunk.Len() > 20 {
		return false
	}
	
	return true
}

// inlineFunction 内联函数体到当前编译位置
// 将函数体的字节码复制到当前 chunk，并处理参数绑定
func (c *Compiler) inlineFunction(targetFn *bytecode.Function, args []ast.Expression) {
	// 编译参数到栈上（与函数调用相同）
	// 注意：内联时参数已经在栈上，我们需要将它们存储到局部变量槽中
	// 函数体的局部变量布局：[caller, arg0, arg1, ..., local0, local1, ...]
	
	// 保存当前局部变量状态
	savedLocals := make([]Local, len(c.locals))
	copy(savedLocals, c.locals)
	
	// 为参数创建局部变量槽（跳过 slot 0，它是调用者）
	paramSlots := make([]int, len(args))
	for i := 0; i < len(args); i++ {
		// 编译参数表达式
		c.compileExpr(args[i])
		// 创建局部变量槽
		slot := c.localCount
		c.addLocal(fmt.Sprintf("$__inline_param_%d", i))
		// 存储参数到局部变量
		c.emitU16(bytecode.OpStoreLocal, uint16(slot))
		paramSlots[i] = slot
	}
	
	// 复制函数体的字节码（跳过最后的 OpReturnNull）
	// 注意：需要调整局部变量引用和跳转偏移量
	sourceChunk := targetFn.Chunk
	targetChunk := c.currentChunk()
	
	// 计算需要调整的局部变量偏移量
	localOffset := c.localCount - 1 // 新局部变量的起始槽（-1 因为 slot 0 是调用者）
	
	// 复制字节码，调整局部变量引用
	offset := 0
	for offset < len(sourceChunk.Code) {
		op := bytecode.OpCode(sourceChunk.Code[offset])
		
		// 跳过最后的 OpReturnNull（内联时不需要返回）
		if op == bytecode.OpReturnNull && offset == len(sourceChunk.Code)-1 {
			break
		}
		
		// 处理需要调整局部变量引用的指令
		switch op {
		case bytecode.OpLoadLocal, bytecode.OpStoreLocal:
			// 调整局部变量索引：函数内的局部变量需要加上偏移量
			oldSlot := int(sourceChunk.ReadU16(offset + 1))
			// slot 0 是调用者，保持不变
			// slot 1..N 是参数，需要映射到我们创建的参数槽
			if oldSlot == 0 {
				// 调用者槽，保持不变
				targetChunk.Write(byte(op), c.currentLine)
				targetChunk.WriteU16(uint16(0), c.currentLine)
			} else if oldSlot <= len(args) {
				// 参数槽，映射到我们创建的参数槽
				targetChunk.Write(byte(op), c.currentLine)
				targetChunk.WriteU16(uint16(paramSlots[oldSlot-1]), c.currentLine)
			} else {
				// 函数内局部变量，加上偏移量
				newSlot := oldSlot + localOffset - len(args)
				targetChunk.Write(byte(op), c.currentLine)
				targetChunk.WriteU16(uint16(newSlot), c.currentLine)
			}
			offset += 3
		case bytecode.OpReturn:
			// 内联时，return 语句需要特殊处理
			// 如果函数有返回值，保留返回值在栈上
			// 如果没有返回值，移除 OpReturnNull（已在上面处理）
			// 对于 OpReturn，保留返回值在栈上，不发出返回指令
			// 实际上，我们需要保留返回值，所以这里不做任何操作
			// 但需要跳过 return 指令
			offset++
		default:
			// 其他指令直接复制
			line := sourceChunk.Lines[offset]
			targetChunk.Write(sourceChunk.Code[offset], line)
			offset++
		}
	}
	
	// 恢复局部变量状态（但保留内联函数创建的局部变量）
	// 实际上，内联后局部变量应该被清理，但为了简化，我们保留它们
	// 这可能会导致局部变量槽浪费，但不会影响正确性
}

// resolveNamedArguments 解析命名参数，返回按正确顺序排列的参数列表
func (c *Compiler) resolveNamedArguments(e *ast.CallExpr) []ast.Expression {
	// 如果没有命名参数，直接返回位置参数
	if len(e.NamedArguments) == 0 {
		return e.Arguments
	}
	
	// 获取函数签名
	var sig *FunctionSignature
	switch fn := e.Function.(type) {
	case *ast.Identifier:
		sig = c.symbolTable.GetFunction(fn.Name)
	case *ast.Variable:
		// 变量作为函数调用时不支持命名参数
		c.error(e.Pos(), "命名参数不能用于变量函数调用")
		return e.Arguments
	default:
		c.error(e.Pos(), "命名参数不能用于此类型的函数调用")
		return e.Arguments
	}
	
	if sig == nil || len(sig.ParamNames) == 0 {
		c.error(e.Pos(), "该函数不支持命名参数（未找到参数签名）")
		return e.Arguments
	}
	
	// 创建参数映射：参数名 -> 索引
	paramIndex := make(map[string]int)
	for i, name := range sig.ParamNames {
		paramIndex[name] = i
	}
	
	// 创建结果数组
	totalParams := len(sig.ParamNames)
	result := make([]ast.Expression, totalParams)
	filled := make([]bool, totalParams)
	
	// 首先填充位置参数
	for i, arg := range e.Arguments {
		if i >= totalParams {
			c.error(arg.Pos(), "参数数量超出限制")
			return e.Arguments
		}
		result[i] = arg
		filled[i] = true
	}
	
	// 然后填充命名参数
	for _, namedArg := range e.NamedArguments {
		paramName := namedArg.Name.Name
		idx, ok := paramIndex[paramName]
		if !ok {
			c.error(namedArg.Pos(), "未知的参数名: "+paramName)
			continue
		}
		if filled[idx] {
			c.error(namedArg.Pos(), "参数 "+paramName+" 已被赋值")
			continue
		}
		result[idx] = namedArg.Value
		filled[idx] = true
	}
	
	// 检查必需的参数是否都已提供
	for i := 0; i < sig.MinArity; i++ {
		if !filled[i] {
			c.error(e.Pos(), "缺少必需的参数: "+sig.ParamNames[i])
		}
	}
	
	// 对于可选参数，如果没有提供，编译时使用 null
	// 注意：真正的默认值在运行时由函数定义处理
	for i := sig.MinArity; i < totalParams; i++ {
		if !filled[i] {
			// 创建一个 null 字面量表达式
			result[i] = &ast.NullLiteral{}
		}
	}
	
	// 计算实际需要的参数数量（去掉尾部的可选参数）
	actualLen := totalParams
	for i := totalParams - 1; i >= sig.MinArity; i-- {
		if !filled[i] {
			actualLen = i
		} else {
			break
		}
	}
	
	return result[:actualLen]
}

// checkCallArgTypes 检查函数调用参数类型
// 静态类型系统：严格检查所有参数类型
func (c *Compiler) checkCallArgTypes(e *ast.CallExpr) {
	var sig *FunctionSignature
	var funcName string
	
	switch fn := e.Function.(type) {
	case *ast.Identifier:
		funcName = fn.Name
		sig = c.symbolTable.GetFunction(fn.Name)
	case *ast.Variable:
		funcName = fn.Name
		// 变量作为函数调用，从变量类型推断
		varType := c.getVariableType(fn.Name)
		if varType == "" {
			// 静态类型系统：变量类型必须明确（但不重复报错，inferExprType 会处理）
			return
		}
		// 变量类型是 func 时跳过详细检查
		return
	default:
		return
	}
	
	if sig == nil {
		// 静态类型系统：函数必须在符号表中定义（但不重复报错，inferCallExprType 会处理）
		return
	}
	
	// 检查参数数量
	if !sig.IsVariadic {
		if len(e.Arguments) < sig.MinArity {
			c.error(e.Pos(), i18n.T(i18n.ErrArgumentCountMin, sig.MinArity, len(e.Arguments)))
			return
		}
		if len(e.Arguments) > len(sig.ParamTypes) {
			c.error(e.Pos(), i18n.T(i18n.ErrArgumentCountMax, len(sig.ParamTypes), len(e.Arguments)))
			return
		}
	}
	
	// 检查每个参数类型
	for i, arg := range e.Arguments {
		if i >= len(sig.ParamTypes) {
			break // 可变参数情况
		}
		expectedType := sig.ParamTypes[i]
		if expectedType == "dynamic" || expectedType == "unknown" {
			continue // dynamic/unknown 类型接受任何值
		}
		
		actualType := c.inferExprType(arg)
		// 静态类型系统：error 类型表示已报错，跳过避免级联
		if actualType == "error" {
			continue
		}
		
		if !c.isTypeCompatible(actualType, expectedType) {
			c.error(arg.Pos(), i18n.T(i18n.ErrTypeMismatch, expectedType, actualType))
		}
	}
	_ = funcName // 避免未使用警告
}

func (c *Compiler) compileIndexExpr(e *ast.IndexExpr) {
	c.compileExpr(e.Object)
	c.compileExpr(e.Index)

	// 判断是数组还是 Map (运行时处理)
	// 数组访问边界检查优化：在已验证边界的循环中使用无检查版本
	if c.inOptimizedLoop && c.canSkipBoundsCheck(e) {
		c.emit(bytecode.OpArrayGetUnchecked)
	} else {
		c.emit(bytecode.OpArrayGet)
	}
}

// canSkipBoundsCheck 检查是否可以跳过边界检查
// 条件：索引是循环变量，且数组在循环外已验证边界
func (c *Compiler) canSkipBoundsCheck(e *ast.IndexExpr) bool {
	// 检查对象是否是已验证边界的数组变量
	if v, ok := e.Object.(*ast.Variable); ok {
		if c.boundsCheckedArrays[v.Name] {
			// 检查索引是否是安全的（循环变量或常量）
			if idx, ok := e.Index.(*ast.Variable); ok {
				if idx.Name == c.loopBoundVar {
					return true
				}
			}
		}
	}
	return false
}

func (c *Compiler) compilePropertyAccess(e *ast.PropertyAccess) {
	c.compileExpr(e.Object)

	// 特殊属性处理
	if e.Property.Name == "length" {
		c.emit(bytecode.OpArrayLen)
		return
	}

	idx := c.makeConstant(bytecode.NewString(e.Property.Name))
	c.emitU16(bytecode.OpGetField, idx)
}

func (c *Compiler) compileMethodCall(e *ast.MethodCall) {
	// 特殊方法处理
	switch e.Method.Name {
	case "has":
		// 数组/Map 的 has() 方法
		c.compileExpr(e.Object)
		if len(e.Arguments) > 0 {
			c.compileExpr(e.Arguments[0])
		} else {
			c.emit(bytecode.OpNull)
		}
		c.emit(bytecode.OpMapHas) // 通用的 has 检查（在 VM 中同时处理数组和 Map）
		return
	case "push":
		// 数组的 push() 方法
		c.compileExpr(e.Object)
		if len(e.Arguments) > 0 {
			c.compileExpr(e.Arguments[0])
		}
		c.emit(bytecode.OpArrayPush)
		return
	case "length", "len":
		// 获取长度
		c.compileExpr(e.Object)
		c.emit(bytecode.OpArrayLen)
		return
	}

	// 静态类型检查：检查方法参数类型
	c.checkMethodCallArgTypes(e)

	// 处理命名参数
	args := c.resolveMethodCallNamedArguments(e)

	c.compileExpr(e.Object)
	for _, arg := range args {
		c.compileExpr(arg)
	}
	idx := c.makeConstant(bytecode.NewString(e.Method.Name))
	c.emitU16(bytecode.OpCallMethod, idx)
	c.currentChunk().WriteU8(byte(len(args)), c.currentLine) // 参数数量
}

// resolveMethodCallNamedArguments 解析方法调用的命名参数
func (c *Compiler) resolveMethodCallNamedArguments(e *ast.MethodCall) []ast.Expression {
	// 如果没有命名参数，直接返回位置参数
	if len(e.NamedArguments) == 0 {
		return e.Arguments
	}
	
	// 获取对象类型
	objType := c.inferExprType(e.Object)
	if objType == "error" || objType == "" {
		return e.Arguments
	}
	
	// 获取方法签名
	sig := c.symbolTable.GetMethod(objType, e.Method.Name, len(e.Arguments)+len(e.NamedArguments))
	if sig == nil || len(sig.ParamNames) == 0 {
		c.error(e.Pos(), "该方法不支持命名参数（未找到参数签名）")
		return e.Arguments
	}
	
	// 创建参数映射：参数名 -> 索引
	paramIndex := make(map[string]int)
	for i, name := range sig.ParamNames {
		paramIndex[name] = i
	}
	
	// 创建结果数组
	totalParams := len(sig.ParamNames)
	result := make([]ast.Expression, totalParams)
	filled := make([]bool, totalParams)
	
	// 首先填充位置参数
	for i, arg := range e.Arguments {
		if i >= totalParams {
			c.error(arg.Pos(), "参数数量超出限制")
			return e.Arguments
		}
		result[i] = arg
		filled[i] = true
	}
	
	// 然后填充命名参数
	for _, namedArg := range e.NamedArguments {
		paramName := namedArg.Name.Name
		idx, ok := paramIndex[paramName]
		if !ok {
			c.error(namedArg.Pos(), "未知的参数名: "+paramName)
			continue
		}
		if filled[idx] {
			c.error(namedArg.Pos(), "参数 "+paramName+" 已被赋值")
			continue
		}
		result[idx] = namedArg.Value
		filled[idx] = true
	}
	
	// 检查必需的参数是否都已提供
	for i := 0; i < sig.MinArity; i++ {
		if !filled[i] {
			c.error(e.Pos(), "缺少必需的参数: "+sig.ParamNames[i])
		}
	}
	
	// 对于可选参数，如果没有提供，编译时使用 null
	for i := sig.MinArity; i < totalParams; i++ {
		if !filled[i] {
			result[i] = &ast.NullLiteral{}
		}
	}
	
	// 计算实际需要的参数数量
	actualLen := totalParams
	for i := totalParams - 1; i >= sig.MinArity; i-- {
		if !filled[i] {
			actualLen = i
		} else {
			break
		}
	}
	
	return result[:actualLen]
}

// checkMethodCallArgTypes 检查方法调用参数类型
// 静态类型系统：严格检查所有参数类型
func (c *Compiler) checkMethodCallArgTypes(e *ast.MethodCall) {
	// 获取对象类型
	objType := c.inferExprType(e.Object)
	// 静态类型系统：error 类型表示已报错，跳过避免级联
	if objType == "error" {
		return
	}
	if objType == "" {
		// inferExprType 应该已经报错了
		return
	}
	
	// 获取方法签名
	sig := c.symbolTable.GetMethod(objType, e.Method.Name, len(e.Arguments))
	if sig == nil {
		// 静态类型系统：方法必须存在（但不重复报错，inferMethodCallType 会处理）
		return
	}
	
	// 检查每个参数类型
	for i, arg := range e.Arguments {
		if i >= len(sig.ParamTypes) {
			break
		}
		expectedType := sig.ParamTypes[i]
		if expectedType == "dynamic" || expectedType == "unknown" {
			continue
		}
		
		actualType := c.inferExprType(arg)
		// 静态类型系统：error 类型表示已报错，跳过避免级联
		if actualType == "error" {
			continue
		}
		
		if !c.isTypeCompatible(actualType, expectedType) {
			c.error(arg.Pos(), i18n.T(i18n.ErrTypeMismatch, expectedType, actualType))
		}
	}
}

func (c *Compiler) compileStaticAccess(e *ast.StaticAccess) {
	// 获取类名
	var className string
	switch cls := e.Class.(type) {
	case *ast.Identifier:
		className = cls.Name
		// 如果类名不包含命名空间，且当前有命名空间，尝试添加命名空间前缀
		if !strings.Contains(className, "\\") && c.currentNamespace != "" {
			// 先尝试带命名空间的完整类名
			fullName := c.currentNamespace + "\\" + className
			// 检查符号表中是否存在（通过查找方法或属性）
			if _, ok := c.symbolTable.ClassMethods[fullName]; ok {
				className = fullName
			} else if _, ok := c.symbolTable.ClassProperties[fullName]; ok {
				className = fullName
			}
		}
	case *ast.SelfExpr:
		className = c.currentClassName // 使用当前类名
		if className == "" {
			className = "self"
		}
	case *ast.ParentExpr:
		// 获取父类名
		if c.currentClassName != "" {
			// 提取基类名
			baseClassName := c.extractBaseTypeName(c.currentClassName)
			if parent, ok := c.symbolTable.ClassParents[baseClassName]; ok {
				className = parent
			}
		}
		if className == "" {
			className = "parent"
		}
	default:
		c.error(e.Pos(), i18n.T(i18n.ErrInvalidStaticAccessC))
		return
	}
	
	// 为字节码使用的类名（可能需要特殊处理 self/parent）
	bytecodeClassName := className
	if cls, ok := e.Class.(*ast.SelfExpr); ok {
		_ = cls
		bytecodeClassName = "self"
	} else if cls, ok := e.Class.(*ast.ParentExpr); ok {
		_ = cls
		bytecodeClassName = "parent"
	}
	
	classIdx := c.makeConstant(bytecode.NewString(bytecodeClassName))
	
	// 处理成员访问
	switch member := e.Member.(type) {
	case *ast.Variable:
		// 静态属性访问: Class::$prop
		nameIdx := c.makeConstant(bytecode.NewString(member.Name))
		c.emitU16(bytecode.OpGetStatic, classIdx)
		c.currentChunk().WriteU16(nameIdx, c.currentLine)
		
	case *ast.Identifier:
		// 类常量访问: Class::CONST
		nameIdx := c.makeConstant(bytecode.NewString(member.Name))
		c.emitU16(bytecode.OpGetStatic, classIdx)
		c.currentChunk().WriteU16(nameIdx, c.currentLine)
		
	case *ast.CallExpr:
		// 静态方法调用: Class::method()
		if fn, ok := member.Function.(*ast.Identifier); ok {
			// 处理命名参数
			args := c.resolveStaticCallNamedArguments(className, fn.Name, member)
			
			// 静态类型检查：检查静态方法参数类型
			c.checkStaticMethodArgTypes(className, fn.Name, args)
			
			nameIdx := c.makeConstant(bytecode.NewString(fn.Name))
			// 编译参数
			for _, arg := range args {
				c.compileExpr(arg)
			}
			c.emitU16(bytecode.OpCallStatic, classIdx)
			c.currentChunk().WriteU16(nameIdx, c.currentLine)
			c.currentChunk().WriteU8(byte(len(args)), c.currentLine)
		}
	default:
		c.error(e.Pos(), i18n.T(i18n.ErrInvalidStaticMember))
	}
}

// resolveStaticCallNamedArguments 解析静态方法调用的命名参数
func (c *Compiler) resolveStaticCallNamedArguments(className, methodName string, e *ast.CallExpr) []ast.Expression {
	// 如果没有命名参数，直接返回位置参数
	if len(e.NamedArguments) == 0 {
		return e.Arguments
	}
	
	// 获取方法签名
	sig := c.symbolTable.GetMethod(className, methodName, len(e.Arguments)+len(e.NamedArguments))
	if sig == nil || len(sig.ParamNames) == 0 {
		c.error(e.Pos(), "该静态方法不支持命名参数（未找到参数签名）")
		return e.Arguments
	}
	
	// 创建参数映射：参数名 -> 索引
	paramIndex := make(map[string]int)
	for i, name := range sig.ParamNames {
		paramIndex[name] = i
	}
	
	// 创建结果数组
	totalParams := len(sig.ParamNames)
	result := make([]ast.Expression, totalParams)
	filled := make([]bool, totalParams)
	
	// 首先填充位置参数
	for i, arg := range e.Arguments {
		if i >= totalParams {
			c.error(arg.Pos(), "参数数量超出限制")
			return e.Arguments
		}
		result[i] = arg
		filled[i] = true
	}
	
	// 然后填充命名参数
	for _, namedArg := range e.NamedArguments {
		paramName := namedArg.Name.Name
		idx, ok := paramIndex[paramName]
		if !ok {
			c.error(namedArg.Pos(), "未知的参数名: "+paramName)
			continue
		}
		if filled[idx] {
			c.error(namedArg.Pos(), "参数 "+paramName+" 已被赋值")
			continue
		}
		result[idx] = namedArg.Value
		filled[idx] = true
	}
	
	// 检查必需的参数是否都已提供
	for i := 0; i < sig.MinArity; i++ {
		if !filled[i] {
			c.error(e.Pos(), "缺少必需的参数: "+sig.ParamNames[i])
		}
	}
	
	// 对于可选参数，如果没有提供，编译时使用 null
	for i := sig.MinArity; i < totalParams; i++ {
		if !filled[i] {
			result[i] = &ast.NullLiteral{}
		}
	}
	
	// 计算实际需要的参数数量
	actualLen := totalParams
	for i := totalParams - 1; i >= sig.MinArity; i-- {
		if !filled[i] {
			actualLen = i
		} else {
			break
		}
	}
	
	return result[:actualLen]
}

// checkStaticMethodArgTypes 检查静态方法参数类型
// 静态类型系统：严格检查所有参数类型
func (c *Compiler) checkStaticMethodArgTypes(className, methodName string, args []ast.Expression) {
	// 获取方法签名
	sig := c.symbolTable.GetMethod(className, methodName, len(args))
	if sig == nil {
		// 静态类型系统：方法必须存在（但不重复报错，inferStaticAccessType 会处理）
		return
	}
	
	// 检查每个参数类型
	for i, arg := range args {
		if i >= len(sig.ParamTypes) {
			break
		}
		expectedType := sig.ParamTypes[i]
		if expectedType == "dynamic" || expectedType == "unknown" {
			continue
		}
		
		actualType := c.inferExprType(arg)
		// 静态类型系统：error 类型表示已报错，跳过避免级联
		if actualType == "error" {
			continue
		}
		
		if !c.isTypeCompatible(actualType, expectedType) {
			c.error(arg.Pos(), i18n.T(i18n.ErrTypeMismatch, expectedType, actualType))
		}
	}
}

func (c *Compiler) compileNewExpr(e *ast.NewExpr) {
	// 静态类型检查：检查构造函数参数类型
	c.checkConstructorArgTypes(e)
	
	// 验证泛型约束（如果是泛型类型实例化）
	if len(e.TypeArgs) > 0 {
		className := e.ClassName.Name
		c.validateGenericConstraints(className, e.TypeArgs)
	}
	
	idx := c.makeConstant(bytecode.NewString(e.ClassName.Name))
	c.emitU16(bytecode.OpNewObject, idx)

	// 处理命名参数
	args := c.resolveNewExprNamedArguments(e)

	// 调用构造函数
	for _, arg := range args {
		c.compileExpr(arg)
	}
	constructorIdx := c.makeConstant(bytecode.NewString("__construct"))
	c.emitU16(bytecode.OpCallMethod, constructorIdx)
	c.currentChunk().WriteU8(byte(len(args)), c.currentLine) // 参数数量
}

// resolveNewExprNamedArguments 解析NewExpr的命名参数
func (c *Compiler) resolveNewExprNamedArguments(e *ast.NewExpr) []ast.Expression {
	// 如果没有命名参数，直接返回位置参数
	if len(e.NamedArguments) == 0 {
		return e.Arguments
	}
	
	className := e.ClassName.Name
	
	// 获取构造函数签名
	sig := c.symbolTable.GetMethod(className, "__construct", len(e.Arguments)+len(e.NamedArguments))
	if sig == nil || len(sig.ParamNames) == 0 {
		c.error(e.Pos(), "该构造函数不支持命名参数（未找到参数签名）")
		return e.Arguments
	}
	
	// 创建参数映射：参数名 -> 索引
	paramIndex := make(map[string]int)
	for i, name := range sig.ParamNames {
		paramIndex[name] = i
	}
	
	// 创建结果数组
	totalParams := len(sig.ParamNames)
	result := make([]ast.Expression, totalParams)
	filled := make([]bool, totalParams)
	
	// 首先填充位置参数
	for i, arg := range e.Arguments {
		if i >= totalParams {
			c.error(arg.Pos(), "参数数量超出限制")
			return e.Arguments
		}
		result[i] = arg
		filled[i] = true
	}
	
	// 然后填充命名参数
	for _, namedArg := range e.NamedArguments {
		paramName := namedArg.Name.Name
		idx, ok := paramIndex[paramName]
		if !ok {
			c.error(namedArg.Pos(), "未知的参数名: "+paramName)
			continue
		}
		if filled[idx] {
			c.error(namedArg.Pos(), "参数 "+paramName+" 已被赋值")
			continue
		}
		result[idx] = namedArg.Value
		filled[idx] = true
	}
	
	// 检查必需的参数是否都已提供
	for i := 0; i < sig.MinArity; i++ {
		if !filled[i] {
			c.error(e.Pos(), "缺少必需的参数: "+sig.ParamNames[i])
		}
	}
	
	// 对于可选参数，如果没有提供，编译时使用 null
	for i := sig.MinArity; i < totalParams; i++ {
		if !filled[i] {
			result[i] = &ast.NullLiteral{}
		}
	}
	
	// 计算实际需要的参数数量
	actualLen := totalParams
	for i := totalParams - 1; i >= sig.MinArity; i-- {
		if !filled[i] {
			actualLen = i
		} else {
			break
		}
	}
	
	return result[:actualLen]
}

// checkConstructorArgTypes 检查构造函数参数类型
// 静态类型系统：严格检查所有参数类型
func (c *Compiler) checkConstructorArgTypes(e *ast.NewExpr) {
	className := e.ClassName.Name
	
	// 获取构造函数签名
	sig := c.symbolTable.GetMethod(className, "__construct", len(e.Arguments))
	if sig == nil {
		return // 未找到构造函数，可能是无参数默认构造函数
	}
	
	// 检查每个参数类型
	for i, arg := range e.Arguments {
		if i >= len(sig.ParamTypes) {
			break
		}
		expectedType := sig.ParamTypes[i]
		if expectedType == "dynamic" || expectedType == "unknown" {
			continue
		}
		
		actualType := c.inferExprType(arg)
		// 静态类型系统：error 类型表示已报错，跳过避免级联
		if actualType == "error" {
			continue
		}
		
		if !c.isTypeCompatible(actualType, expectedType) {
			c.error(arg.Pos(), i18n.T(i18n.ErrTypeMismatch, expectedType, actualType))
		}
	}
}

func (c *Compiler) compileClosureExpr(e *ast.ClosureExpr) {
	// 编译闭包函数，传入 use 变量和返回类型
	fn := c.CompileClosureWithReturnType("<closure>", e.Parameters, e.UseVars, e.Body, e.ReturnType)
	
	// 如果有 use 变量，需要创建闭包并捕获值
	if len(e.UseVars) > 0 {
		// 先 push 闭包函数
		c.emitConstant(bytecode.NewFunc(fn))
		// 然后为每个 use 变量 push 其值
		for _, v := range e.UseVars {
			c.compileVariable(v)
		}
		// 发出创建闭包的指令
		c.emitU16(bytecode.OpClosure, uint16(len(e.UseVars)))
	} else {
		c.emitConstant(bytecode.NewFunc(fn))
	}
}

func (c *Compiler) compileArrowFuncExpr(e *ast.ArrowFuncExpr) {
	// 创建一个包含单个 return 语句的块
	body := &ast.BlockStmt{
		Statements: []ast.Statement{
			&ast.ReturnStmt{
				Values: []ast.Expression{e.Body},
			},
		},
	}
	fn := c.CompileFunction("<arrow>", e.Parameters, body)
	c.emitConstant(bytecode.NewFunc(fn))
}

func (c *Compiler) compileClassAccessExpr(e *ast.ClassAccessExpr) {
	// ::class 语法编译时解析为类名字符串
	// PHP 风格：支持 ClassName::class 和 self::class
	var className string
	
	switch obj := e.Object.(type) {
	case *ast.SelfExpr:
		// self::class - 返回当前类名
		if c.currentClassName == "" {
			c.error(e.Pos(), i18n.T(i18n.ErrSelfOutsideClass))
			return
		}
		className = c.currentClassName
		
	case *ast.Identifier:
		// ClassName::class - 返回指定类名
		className = obj.Name
		// 注意：不验证类是否存在，因为可能是前置引用
		// 运行时会自然报错如果类真的不存在
		
	default:
		c.error(e.Pos(), "::class 只能用于类名或 self")
		return
	}
	
	// 如果有命名空间，添加命名空间前缀
	if c.currentNamespace != "" {
		className = c.currentNamespace + "\\" + className
	}
	
	// 将类名作为字符串常量压入栈
	c.emitConstant(bytecode.NewString(className))
}

func (c *Compiler) compileTypeCastExpr(e *ast.TypeCastExpr) {
	// 编译被转换的表达式
	c.compileExpr(e.Expr)

	// 获取目标类型名称
	typeName := e.TargetType.String()
	typeIdx := c.makeConstant(bytecode.NewString(typeName))

	// 根据是否是安全转换选择不同的操作码
	if e.Safe {
		c.emitU16(bytecode.OpCastSafe, typeIdx)
	} else {
		c.emitU16(bytecode.OpCast, typeIdx)
	}
}

// compileMatchExpr 编译模式匹配表达式
func (c *Compiler) compileMatchExpr(e *ast.MatchExpr) {
	// 编译被匹配的表达式
	c.compileExpr(e.Expr)
	
	// 收集所有跳转到结束的位置
	var endJumps []int
	// 收集跳转到下一个 case 的位置
	var nextCaseJumps []int
	
	// 编译每个 case
	for i, case_ := range e.Cases {
		// 修补之前的跳转（跳转到当前 case）
		// 跳转过来时，栈上有 [matched_value, false]，需要弹出 false
		if len(nextCaseJumps) > 0 {
			for _, jump := range nextCaseJumps {
				if jump >= 0 {
					c.patchJump(jump)
				}
			}
			c.emit(bytecode.OpPop) // 弹出 false（之前的匹配失败）
			nextCaseJumps = nil
		}
		
		// 用于类型模式的变量绑定
		var boundVarName string
		var boundVarType string
		
		// 编译模式匹配
		switch p := case_.Pattern.(type) {
		case *ast.WildcardPattern:
			// 通配符：总是匹配，不需要检查
			// 栈上还有被匹配的值
			
		case *ast.ValuePattern:
			// 值模式：复制值，比较
			c.emit(bytecode.OpDup) // 复制被匹配的值
			c.compileExpr(p.Value) // 编译模式中的值
			c.emit(bytecode.OpEq)  // 比较，结果在栈顶
			// 如果不匹配，跳转到下一个 case
			nextJump := c.emitJump(bytecode.OpJumpIfFalse)
			c.emit(bytecode.OpPop) // 弹出 true（匹配成功）
			// 跳转目标需要弹出 false
			nextCaseJumps = append(nextCaseJumps, nextJump)
			
		case *ast.TypePattern:
			// 类型模式：复制值，检查类型
			c.emit(bytecode.OpDup) // 复制被匹配的值
			typeName := c.getTypeName(p.Type)
			typeIdx := c.makeConstant(bytecode.NewString(typeName))
			c.emitU16(bytecode.OpCheckType, typeIdx)
			// 如果类型不匹配，跳转到下一个 case
			nextJump := c.emitJump(bytecode.OpJumpIfFalse)
			c.emit(bytecode.OpPop) // 弹出 true（匹配成功）
			nextCaseJumps = append(nextCaseJumps, nextJump)
			
			// 如果有变量绑定，记录变量信息
			if p.Variable != nil {
				boundVarName = p.Variable.Name
				boundVarType = typeName
			}
		}
		
		// 如果有变量绑定，创建局部变量
		// 栈上现在是 [matched_value]，我们需要复制它并绑定到变量
		hasBinding := boundVarName != ""
		var prevLocalCount int
		if hasBinding {
			c.emit(bytecode.OpDup) // 复制被匹配的值，栈: [matched_value, matched_value_copy]
			// 直接添加局部变量（不使用 beginScope/endScope）
			prevLocalCount = c.localCount
			c.addLocalWithType(boundVarName, boundVarType)
			// 栈: [matched_value]，变量 $n 引用栈位置
		}
		
		// 检查守卫条件（如果有）
		// 守卫条件中可以使用绑定的变量
		hasGuard := case_.Guard != nil
		if hasGuard {
			c.compileExpr(case_.Guard)
			guardJump := c.emitJump(bytecode.OpJumpIfFalse)
			c.emit(bytecode.OpPop) // 弹出 true（守卫成功）
			
			// ====== 守卫成功路径 ======
			// 编译 body
			c.compileExpr(case_.Body)
			
			// 如果有变量绑定，手动弹出绑定的变量
			if hasBinding {
				c.emit(bytecode.OpSwap)
				c.emit(bytecode.OpPop)
				c.localCount = prevLocalCount
			}
			
			// 交换并弹出被匹配的值
			c.emit(bytecode.OpSwap)
			c.emit(bytecode.OpPop)
			
			// 跳转到 match 结束
			bodyEndJump := c.emitJump(bytecode.OpJump)
			endJumps = append(endJumps, bodyEndJump)
			
			// ====== 守卫失败路径 ======
			c.patchJump(guardJump)
			// 注意：此时栈上有 [matched_value, bound_var (如果有), false]
			// 需要清理到只剩 [matched_value, false]，这样下一个 case 可以统一处理
			
			// 如果有变量绑定，需要先交换 false 和 bound_var，然后弹出 bound_var
			if hasBinding {
				// 栈: [matched_value, bound_var, false]
				c.emit(bytecode.OpSwap) // [matched_value, false, bound_var]
				c.emit(bytecode.OpPop)  // [matched_value, false]
				c.localCount = prevLocalCount
			}
			// 现在栈: [matched_value, false]
			// 跳转到下一个 case
			guardFailJump := c.emitJump(bytecode.OpJump)
			nextCaseJumps = append(nextCaseJumps, guardFailJump)
			continue
		}
		
		// ====== 没有守卫条件的情况 ======
		// 编译 body
		c.compileExpr(case_.Body)
		
		// 如果有变量绑定，手动弹出绑定的变量
		if hasBinding {
			c.emit(bytecode.OpSwap)
			c.emit(bytecode.OpPop)
			c.localCount = prevLocalCount
		}
		
		// 交换栈顶：现在栈上是 [被匹配的值, body结果]
		// 需要变成 [body结果]
		c.emit(bytecode.OpSwap)
		c.emit(bytecode.OpPop) // 弹出被匹配的值
		
		// 跳转到结束（除了最后一个 case）
		if i < len(e.Cases)-1 {
			endJump := c.emitJump(bytecode.OpJump)
			endJumps = append(endJumps, endJump)
		}
	}
	
	// 修补之前的跳转（处理最后一个 case 失败的情况）
	for _, jump := range nextCaseJumps {
		if jump >= 0 {
			c.patchJump(jump)
		}
	}
	
	// 修补所有跳转到结束的跳转
	for _, jump := range endJumps {
		c.patchJump(jump)
	}
}


// checkMatchExhaustiveness 检查 match 表达式的穷尽性（可选）
func (c *Compiler) checkMatchExhaustiveness(e *ast.MatchExpr, exprType string) {
	// 简化实现：只检查是否有通配符
	// 如果所有 case 都有具体的模式（没有通配符），给出警告（不是错误）
	hasWildcard := false
	for _, case_ := range e.Cases {
		if _, ok := case_.Pattern.(*ast.WildcardPattern); ok {
			hasWildcard = true
			break
		}
	}
	
	// 如果有通配符，认为已经覆盖所有情况
	// 如果没有通配符，建议添加（但这不是错误，因为可能所有情况都已明确覆盖）
	if !hasWildcard && len(e.Cases) > 0 {
		// 可以选择发出警告或忽略
		// 这里我们不做任何操作，因为穷尽性检查是可选的
	}
}

// compileIsExpr 编译类型检查表达式 ($x is string)
// 在运行时检查表达式的类型是否与目标类型兼容
func (c *Compiler) compileIsExpr(e *ast.IsExpr) {
	// 编译被检查的表达式
	c.compileExpr(e.Expr)
	
	// 获取目标类型名称
	typeName := c.getTypeName(e.TypeName)
	typeIdx := c.makeConstant(bytecode.NewString(typeName))
	
	// 发射类型检查指令（复用 OpCheckType）
	c.emitU16(bytecode.OpCheckType, typeIdx)
	
	// 如果是取反的 is 表达式，添加 NOT 指令
	if e.Negated {
		c.emit(bytecode.OpNot)
	}
}

// ============================================================================
// 作用域管理
// ============================================================================

func (c *Compiler) beginScope() {
	c.scopeDepth++
	// CSE: 记录当前作用域深度
	if c.cseEnabled {
		c.cseScopeDepth = c.scopeDepth
	}
}

func (c *Compiler) endScope() {
	c.scopeDepth--

	// CSE: 清理当前作用域的缓存条目
	if c.cseEnabled {
		c.cleanupCSECache()
	}

	// 弹出当前作用域的局部变量
	for c.localCount > 0 && c.locals[c.localCount-1].Depth > c.scopeDepth {
		c.emit(bytecode.OpPop)
		c.localCount--
	}
}

// cleanupCSECache 清理当前作用域结束时的 CSE 缓存
func (c *Compiler) cleanupCSECache() {
	// 移除所有深度大于当前作用域的缓存条目
	for sig, slot := range c.exprCache {
		if slot >= c.localCount {
			delete(c.exprCache, sig)
			continue
		}
		// 检查变量是否在当前作用域
		if slot > 0 && c.locals[slot].Depth > c.scopeDepth {
			delete(c.exprCache, sig)
		}
	}
}

// cacheExprResult 缓存表达式结果到临时变量
// 调用时栈顶应该是表达式的结果值
func (c *Compiler) cacheExprResult(sig string) {
	if !c.cseEnabled || sig == "" {
		return
	}

	// 检查是否已经缓存
	if _, exists := c.exprCache[sig]; exists {
		return
	}

	// 创建临时变量名
	tempVarName := fmt.Sprintf("$__cse_%d", len(c.exprCache))
	
	// 复制栈顶值（因为要存储也要使用）
	c.emit(bytecode.OpDup)
	
	// 声明并定义临时变量
	c.declareVariableWithType(tempVarName, "")
	c.defineVariable()
	
	// 记录缓存（此时值已在栈上，defineVariable 已存储）
	c.exprCache[sig] = c.localCount - 1
}

func (c *Compiler) declareVariable(name string) {
	c.declareVariableWithType(name, "")
}

func (c *Compiler) declareVariableWithType(name string, typeName string) {
	if c.scopeDepth == 0 {
		return // 全局变量
	}

	// 检查当前作用域是否已有同名变量
	for i := c.localCount - 1; i >= 0; i-- {
		local := &c.locals[i]
		if local.Depth < c.scopeDepth {
			break
		}
		if local.Name == name {
			c.error(token.Position{}, i18n.T(i18n.ErrVariableRedeclared))
			return
		}
	}
	
	c.addLocalWithType(name, typeName)
}

func (c *Compiler) defineVariable() {
	if c.scopeDepth > 0 {
		return // 局部变量已在栈上
	}

	// 全局变量需要存储
	// 这里简化处理，实际应该记录变量名
}

func (c *Compiler) addLocal(name string) {
	c.addLocalWithType(name, "")
}

func (c *Compiler) addLocalWithType(name string, typeName string) {
	if c.localCount >= 256 {
		c.error(token.Position{}, i18n.T(i18n.ErrTooManyLocals))
		return
	}
	c.locals[c.localCount] = Local{
		Name:     name,
		Depth:    c.scopeDepth,
		Index:    c.localCount,
		TypeName: typeName,
	}
	c.localCount++
}

// getLocalType 获取局部变量的类型
func (c *Compiler) getLocalType(name string) string {
	for i := c.localCount - 1; i >= 0; i-- {
		if c.locals[i].Name == name {
			return c.locals[i].TypeName
		}
	}
	return ""
}

// setLocalType 设置局部变量的类型
func (c *Compiler) setLocalType(name string, typeName string) {
	for i := c.localCount - 1; i >= 0; i-- {
		if c.locals[i].Name == name {
			c.locals[i].TypeName = typeName
			return
		}
	}
}

// getVariableType 获取变量类型（局部或全局）
// 支持类型收窄：在 if 分支中，变量类型可能被收窄
func (c *Compiler) getVariableType(name string) string {
	// 首先检查类型收窄上下文（优先级最高）
	if c.narrowedTypes != nil {
		if narrowedType, ok := c.narrowedTypes[name]; ok {
			return narrowedType
		}
	}
	
	// 先查局部变量
	if t := c.getLocalType(name); t != "" {
		return t
	}
	// 再查全局变量
	if t, ok := c.globalTypes[name]; ok {
		return t
	}
	return ""
}

// setVariableType 设置变量类型（局部或全局）
func (c *Compiler) setVariableType(name string, typeName string) {
	// 如果是局部变量
	if c.resolveLocal(name) != -1 {
		c.setLocalType(name, typeName)
		return
	}
	// 否则是全局变量
	c.globalTypes[name] = typeName
}

func (c *Compiler) resolveLocal(name string) int {
	for i := c.localCount - 1; i >= 0; i-- {
		if c.locals[i].Name == name {
			return c.locals[i].Index
		}
	}
	return -1
}

// ============================================================================
// 字节码生成辅助
// ============================================================================

func (c *Compiler) currentChunk() *bytecode.Chunk {
	return c.function.Chunk
}

func (c *Compiler) emit(op bytecode.OpCode) {
	c.currentChunk().WriteOp(op, c.currentLine)
}

func (c *Compiler) emitByte(op bytecode.OpCode, b byte) {
	c.emit(op)
	c.currentChunk().WriteU8(b, c.currentLine)
}

func (c *Compiler) emitU16(op bytecode.OpCode, v uint16) {
	c.emit(op)
	c.currentChunk().WriteU16(v, c.currentLine)
}

func (c *Compiler) emitConstant(value bytecode.Value) {
	idx := c.makeConstant(value)
	c.emitU16(bytecode.OpPush, idx)
}

func (c *Compiler) makeConstant(value bytecode.Value) uint16 {
	return c.currentChunk().AddConstant(value)
}

func (c *Compiler) emitJump(op bytecode.OpCode) int {
	c.emit(op)
	c.currentChunk().WriteU16(0xFFFF, c.currentLine) // 占位
	return c.currentChunk().Len() - 2
}

func (c *Compiler) patchJump(offset int) {
	c.currentChunk().PatchJump(offset)
}

func (c *Compiler) emitLoop(loopStart int) {
	c.emit(bytecode.OpLoop)
	offset := c.currentChunk().Len() - loopStart + 2
	c.currentChunk().WriteU16(uint16(offset), c.currentLine)
}

func (c *Compiler) error(pos token.Position, message string, args ...interface{}) {
	formattedMsg := fmt.Sprintf(message, args...)
	c.errors = append(c.errors, Error{Pos: pos, Message: formattedMsg})
}

// errorWithCode 使用错误码报告错误
func (c *Compiler) errorWithCode(code string, pos token.Position, message string, context map[string]interface{}) {
	// 创建增强的错误对象
	err := &errors.CompileError{
		Code:      code,
		Level:     errors.LevelError,
		Message:   message,
		File:      c.sourceFile,
		Line:      pos.Line,
		Column:    pos.Column,
		EndColumn: pos.Column + 1,
	}

	// 获取修复建议
	if context == nil {
		context = make(map[string]interface{})
	}
	err.Hints = errors.GetSuggestions(code, context)

	// 添加到错误列表（保持兼容性）
	c.errors = append(c.errors, Error{Pos: pos, Message: message})

	// 使用新的错误报告器（如果启用）
	if useEnhancedErrors {
		reporter := errors.GetDefaultReporter()
		reporter.SetSource(c.sourceFile, c.getSourceContent())
		// 直接格式化输出（不通过 ReportError 以避免重复输出）
		_ = err // 错误已记录，格式化输出在 Compile 结束时统一处理
	}
}

// getSourceContent 获取当前源文件内容（用于错误报告）
func (c *Compiler) getSourceContent() string {
	// 这个方法需要在编译时保存源文件内容
	// 暂时返回空字符串，后续可以扩展
	return ""
}

// useEnhancedErrors 是否使用增强的错误报告（默认关闭，保持兼容性）
var useEnhancedErrors = false

// EnableEnhancedErrors 启用增强的错误报告
func EnableEnhancedErrors() {
	useEnhancedErrors = true
}

// DisableEnhancedErrors 禁用增强的错误报告
func DisableEnhancedErrors() {
	useEnhancedErrors = false
}

// inferExprType 推断表达式的类型名
// 静态类型系统：所有表达式必须有明确类型，无法推断时报编译错误并返回 "error"
func (c *Compiler) inferExprType(expr ast.Expression) string {
	switch e := expr.(type) {
	case *ast.IntegerLiteral:
		return "int"
	case *ast.FloatLiteral:
		return "float"
	case *ast.StringLiteral, *ast.InterpStringLiteral:
		return "string"
	case *ast.ClassAccessExpr:
		// ::class 语法返回 string 类型（类名）
		return "string"
	case *ast.BoolLiteral:
		return "bool"
	case *ast.NullLiteral:
		return "null"
	case *ast.ArrayLiteral:
		// 增强数组类型推导：支持嵌套数组和更复杂的元素类型推导
		// 例如：[[1, 2], [3, 4]] 应该推导为 int[][]
		if len(e.Elements) == 0 {
			return "array" // 空数组无法推断元素类型
		}
		
		// 收集所有元素的类型
		var elemTypes []string
		for _, elem := range e.Elements {
			elemType := c.inferExprType(elem)
			if elemType == "error" {
				return "error"
			}
			if elemType != "" && elemType != "dynamic" {
				elemTypes = append(elemTypes, elemType)
			}
		}
		
		if len(elemTypes) == 0 {
			return "array" // 无法推断元素类型
		}
		
		// 查找所有元素类型的公共类型
		commonType := c.findCommonArrayElementType(elemTypes)
		if commonType != "" {
			return commonType + "[]"
		}
		
		// 如果第一个元素是数组，尝试推断嵌套数组类型
		firstElemType := elemTypes[0]
		if strings.HasSuffix(firstElemType, "[]") {
			// 嵌套数组：检查所有元素是否都是数组
			allArrays := true
			for _, t := range elemTypes {
				if !strings.HasSuffix(t, "[]") {
					allArrays = false
					break
				}
			}
			if allArrays {
				// 递归推断嵌套数组的元素类型
				nestedElemTypes := make([]string, len(elemTypes))
				for i, t := range elemTypes {
					nestedElemTypes[i] = strings.TrimSuffix(t, "[]")
				}
				nestedCommonType := c.findCommonArrayElementType(nestedElemTypes)
				if nestedCommonType != "" {
					return nestedCommonType + "[][]"
				}
			}
		}
		
		// 使用第一个元素的类型
		return elemTypes[0] + "[]"
		
	case *ast.MapLiteral:
		// 增强 Map 类型推导：支持更准确的键值类型推断
		// 例如：["a" => 1, "b" => 2] 应该推导为 map[string]int
		if len(e.Pairs) == 0 {
			return "map" // 空 Map 无法推断类型
		}
		
		// 收集所有键和值的类型
		var keyTypes, valueTypes []string
		for _, pair := range e.Pairs {
			keyType := c.inferExprType(pair.Key)
			valueType := c.inferExprType(pair.Value)
			
			if keyType == "error" || valueType == "error" {
				return "error"
			}
			
			if keyType != "" && keyType != "dynamic" {
				keyTypes = append(keyTypes, keyType)
			}
			if valueType != "" && valueType != "dynamic" {
				valueTypes = append(valueTypes, valueType)
			}
		}
		
		if len(keyTypes) == 0 || len(valueTypes) == 0 {
			return "map" // 无法推断类型
		}
		
		// 查找公共类型
		commonKeyType := c.findCommonArrayElementType(keyTypes)
		commonValueType := c.findCommonArrayElementType(valueTypes)
		
		if commonKeyType != "" && commonValueType != "" {
			return "map[" + commonKeyType + "]" + commonValueType
		}
		
		// 使用第一个键值对的类型
		return "map[" + keyTypes[0] + "]" + valueTypes[0]
	case *ast.SuperArrayLiteral:
		// PHP 风格万能数组，类型固定为 superarray
		return "SuperArray"
	case *ast.BinaryExpr:
		leftType := c.inferExprType(e.Left)
		rightType := c.inferExprType(e.Right)
		
		// 如果任一操作数类型推断失败，传播错误
		if leftType == "error" || rightType == "error" {
			return "error"
		}
		
		// 字符串拼接
		if e.Operator.Type == token.PLUS {
			if leftType == "string" || rightType == "string" {
				return "string"
			}
		}
		// 比较运算符返回 bool
		switch e.Operator.Type {
		case token.EQ, token.NE, token.LT, token.LE, token.GT, token.GE:
			return "bool"
		case token.AND, token.OR:
			return "bool"
		}
		// 算术运算
		if leftType == "float" || rightType == "float" {
			return "float"
		}
		return "int"
	case *ast.UnaryExpr:
		if e.Operator.Type == token.NOT {
			return "bool"
		}
		return c.inferExprType(e.Operand)
	case *ast.Variable:
		// 从变量类型表中获取类型
		if t := c.getVariableType(e.Name); t != "" {
			return t
		}
		// 静态类型系统：变量类型必须明确
		c.error(e.Pos(), i18n.T(i18n.ErrVariableTypeUnknown, e.Name))
		return "error"
	case *ast.ThisExpr:
		// $this 的类型是当前类
		if c.currentClassName != "" {
			return c.currentClassName
		}
		return "unknown"
	case *ast.CallExpr:
		// 从符号表查询函数返回类型
		return c.inferCallExprType(e)
	case *ast.MethodCall:
		// 从符号表查询方法返回类型
		return c.inferMethodCallType(e)
	case *ast.StaticAccess:
		// 从符号表查询静态成员类型
		return c.inferStaticAccessType(e)
	case *ast.NewExpr:
		// 如果有泛型类型参数，返回完整的泛型类型
		if len(e.TypeArgs) > 0 {
			var args []string
			for _, arg := range e.TypeArgs {
				args = append(args, c.getTypeName(arg))
			}
			return e.ClassName.Name + "<" + strings.Join(args, ", ") + ">"
		}
		// 增强泛型推断：尝试从构造函数参数推断泛型类型参数
		return c.inferGenericNewExpr(e)
	case *ast.TernaryExpr:
		// 三元表达式：两个分支类型应该相同
		thenType := c.inferExprType(e.Then)
		elseType := c.inferExprType(e.Else)
		if thenType == "error" || elseType == "error" {
			return "error"
		}
		if thenType == elseType {
			return thenType
		}
		// 如果类型不同，返回联合类型
		if thenType != "" && elseType != "" {
			return thenType + "|" + elseType
		}
		return thenType
	case *ast.IndexExpr:
		// 数组/Map 索引访问
		objType := c.inferExprType(e.Object)
		if objType == "error" {
			return "error"
		}
		if strings.HasSuffix(objType, "[]") {
			// 数组元素类型
			return strings.TrimSuffix(objType, "[]")
		}
		if strings.HasPrefix(objType, "map[") {
			// Map 值类型
			if idx := strings.Index(objType, "]"); idx != -1 {
				return objType[idx+1:]
			}
		}
		// 静态类型系统：索引目标类型必须明确
		if objType == "" || objType == "dynamic" {
			c.error(e.Pos(), i18n.T(i18n.ErrIndexTargetUnknown))
			return "error"
		}
		return "dynamic"
	case *ast.PropertyAccess:
		// 属性访问：从符号表获取属性类型
		return c.inferPropertyAccessType(e)
	case *ast.TypeCastExpr:
		// 类型转换：返回目标类型
		return c.getTypeName(e.TargetType)
	case *ast.IsExpr:
		// is 表达式：返回 bool 类型
		return "bool"
	case *ast.ClosureExpr:
		// 闭包表达式
		// 如果有显式返回类型，使用显式类型
		if e.ReturnType != nil {
			returnType := c.getTypeName(e.ReturnType)
			// 构建参数类型字符串
			paramTypes := c.inferClosureParamTypes(e.Parameters)
			return "func(" + paramTypes + "): " + returnType
		}
		
		// 如果没有显式返回类型，尝试从函数体推断
		// 注意：闭包可能有多个 return 语句，这里只处理简单情况
		// 复杂的多返回语句情况需要控制流分析，这里暂时返回 func 类型
		if len(e.Body.Statements) > 0 {
			// 检查最后一个语句是否是 return 语句
			lastStmt := e.Body.Statements[len(e.Body.Statements)-1]
			if retStmt, ok := lastStmt.(*ast.ReturnStmt); ok && retStmt != nil {
				if len(retStmt.Values) > 0 && retStmt.Values[0] != nil {
					returnType := c.inferExprType(retStmt.Values[0])
					if returnType != "error" && returnType != "" {
						paramTypes := c.inferClosureParamTypes(e.Parameters)
						return "func(" + paramTypes + "): " + returnType
					}
				}
			}
		}
		
		paramTypes := c.inferClosureParamTypes(e.Parameters)
		if paramTypes != "" {
			return "func(" + paramTypes + ")"
		}
		return "func"
		
	case *ast.ArrowFuncExpr:
		// 箭头函数：从函数体表达式推断返回类型
		bodyType := c.inferExprType(e.Body)
		if bodyType == "error" {
			return "error"
		}
		
		// 构建参数类型字符串
		paramTypes := c.inferClosureParamTypes(e.Parameters)
		if paramTypes != "" {
			return "func(" + paramTypes + "): " + bodyType
		}
		return "func(): " + bodyType
	case *ast.MatchExpr:
		// match 表达式：返回所有 case body 的公共类型
		return c.inferMatchExprType(e)
	case *ast.Identifier:
		// 可能是类名、枚举等
		return e.Name
	default:
		// 静态类型系统：所有表达式必须有明确类型
		c.error(expr.Pos(), i18n.T(i18n.ErrTypeCannotInfer))
		return "error"
	}
}

// inferMatchExprType 推断 match 表达式的返回类型
func (c *Compiler) inferMatchExprType(e *ast.MatchExpr) string {
	if len(e.Cases) == 0 {
		c.error(e.Pos(), "match 表达式至少需要一个 case")
		return "error"
	}
	
	// 收集所有 case body 的类型
	var types []string
	for _, case_ := range e.Cases {
		bodyType := c.inferMatchCaseBodyType(case_)
		if bodyType == "error" {
			return "error"
		}
		if bodyType != "" {
			types = append(types, bodyType)
		}
	}
	
	if len(types) == 0 {
		c.error(e.Pos(), "无法推断 match 表达式的返回类型")
		return "error"
	}
	
	// 如果所有类型相同，返回该类型
	firstType := types[0]
	allSame := true
	for _, t := range types {
		if t != firstType {
			allSame = false
			break
		}
	}
	
	if allSame {
		return firstType
	}
	
	// 如果类型不同，检查是否兼容
	// 简化处理：返回第一个类型（未来可以改进为真正的联合类型推断）
	return firstType
}

// inferMatchCaseBodyType 推断 match case body 的类型（考虑类型收窄）
func (c *Compiler) inferMatchCaseBodyType(case_ *ast.MatchCase) string {
	// 如果是类型模式且绑定了变量，需要收窄变量类型
	var savedTypes map[string]string
	if tp, ok := case_.Pattern.(*ast.TypePattern); ok && tp.Variable != nil {
		// 收窄变量类型
		typeName := c.getTypeName(tp.Type)
		savedTypes = make(map[string]string)
		savedTypes[tp.Variable.Name] = c.getVariableType(tp.Variable.Name)
		c.narrowVariableType(tp.Variable.Name, typeName)
	}
	
	// 推断 body 类型
	bodyType := c.inferExprType(case_.Body)
	
	// 恢复变量类型
	if savedTypes != nil {
		for name, oldType := range savedTypes {
			c.restoreVariableType(name, oldType)
		}
	}
	
	return bodyType
}

// narrowVariableType 收窄变量类型
func (c *Compiler) narrowVariableType(name, newType string) {
	if c.narrowedTypes == nil {
		c.narrowedTypes = make(map[string]string)
	}
	c.narrowedTypes[name] = newType
}

// restoreVariableType 恢复变量类型
func (c *Compiler) restoreVariableType(name, oldType string) {
	if c.narrowedTypes == nil {
		return
	}
	if oldType == "" {
		delete(c.narrowedTypes, name)
	} else {
		c.narrowedTypes[name] = oldType
	}
}

// inferCallExprType 推断函数调用的返回类型
// 静态类型系统：函数必须在符号表中有签名
// inferCallExprType 推断函数调用的返回类型
// 增强：支持泛型函数类型参数推导
func (c *Compiler) inferCallExprType(e *ast.CallExpr) string {
	switch fn := e.Function.(type) {
	case *ast.Identifier:
		// 普通函数调用
		if sig := c.symbolTable.GetFunction(fn.Name); sig != nil {
			// 检查是否是泛型函数
			if len(sig.TypeParams) > 0 {
				// 尝试从参数推导泛型类型参数
				typeArgs := c.inferGenericTypeArgs(sig, e.Arguments)
				if len(typeArgs) > 0 {
					// 替换返回类型中的类型参数
					return c.substituteTypeParamsInString(sig.ReturnType, sig.TypeParams, typeArgs)
				}
			}
			return sig.ReturnType
		}
		// 可能是变量保存的闭包
		if t := c.getVariableType(fn.Name); t != "" {
			// 如果是 func 类型，尝试解析返回类型
			if strings.HasPrefix(t, "func") {
				if idx := strings.LastIndex(t, ": "); idx != -1 {
					return t[idx+2:]
				}
				// func 类型但无返回类型信息，返回 void
				return "void"
			}
			return t
		}
		// 静态类型系统：函数必须存在
		c.error(e.Pos(), i18n.T(i18n.ErrFunctionNotFound, fn.Name))
		return "error"
	case *ast.Variable:
		// 变量作为函数调用
		if t := c.getVariableType(fn.Name); t != "" {
			if strings.HasPrefix(t, "func") {
				if idx := strings.LastIndex(t, ": "); idx != -1 {
					return t[idx+2:]
				}
				return "void"
			}
			return t
		}
		// 静态类型系统：变量类型必须明确
		c.error(e.Pos(), i18n.T(i18n.ErrVariableTypeUnknown, fn.Name))
		return "error"
	default:
		c.error(e.Pos(), i18n.T(i18n.ErrTypeCannotInfer))
		return "error"
	}
}

// inferGenericTypeArgs 从函数调用参数推导泛型类型参数
// 例如：first<T>(T[] $arr) 被调用为 first([1, 2, 3]) 时，推导 T = int
func (c *Compiler) inferGenericTypeArgs(sig *FunctionSignature, args []ast.Expression) []string {
	if len(sig.TypeParams) == 0 {
		return nil
	}
	
	typeArgs := make([]string, len(sig.TypeParams))
	typeArgMap := make(map[string]string) // 类型参数名 -> 推导出的类型
	
	// 从参数类型推导类型参数
	for i, paramType := range sig.ParamTypes {
		if i >= len(args) {
			break
		}
		
		// 推断参数的实际类型
		argType := c.inferExprType(args[i])
		if argType == "error" || argType == "" {
			continue
		}
		
		// 尝试从参数类型匹配推导类型参数
		c.inferTypeParamFromArg(paramType, argType, sig.TypeParams, typeArgMap)
	}
	
	// 将推导结果转换为数组
	for i, paramName := range sig.TypeParams {
		if inferred, ok := typeArgMap[paramName]; ok {
			typeArgs[i] = inferred
		} else {
			// 无法推导，返回空数组（表示推导失败）
			return nil
		}
	}
	
	return typeArgs
}

// inferTypeParamFromArg 从参数类型推导类型参数
// 例如：从 T[] 和 int[] 推导 T = int
func (c *Compiler) inferTypeParamFromArg(paramType, argType string, typeParams []string, typeArgMap map[string]string) {
	// 检查参数类型中是否包含类型参数
	for _, typeParam := range typeParams {
		// 直接匹配：T 和 int
		if paramType == typeParam {
			if argType != "" && argType != "dynamic" && argType != "error" {
				typeArgMap[typeParam] = argType
			}
			continue
		}
		
		// 数组类型匹配：T[] 和 int[]
		if strings.HasSuffix(paramType, "[]") && strings.HasSuffix(argType, "[]") {
			paramElemType := strings.TrimSuffix(paramType, "[]")
			argElemType := strings.TrimSuffix(argType, "[]")
			if paramElemType == typeParam {
				if argElemType != "" && argElemType != "dynamic" && argElemType != "error" {
					typeArgMap[typeParam] = argElemType
				}
			}
		}
		
		// Map 类型匹配：map[K]V 和 map[string]int
		if strings.HasPrefix(paramType, "map[") && strings.HasPrefix(argType, "map[") {
			paramKeyType, paramValueType := c.parseMapType(paramType)
			argKeyType, argValueType := c.parseMapType(argType)
			
			if paramKeyType == typeParam && argKeyType != "" && argKeyType != "dynamic" {
				typeArgMap[typeParam] = argKeyType
			}
			if paramValueType == typeParam && argValueType != "" && argValueType != "dynamic" {
				typeArgMap[typeParam] = argValueType
			}
		}
		
		// 泛型类型匹配：List<T> 和 List<int>
		if strings.Contains(paramType, "<") && strings.Contains(argType, "<") {
			paramBase := strings.Split(paramType, "<")[0]
			argBase := strings.Split(argType, "<")[0]
			if paramBase == argBase {
				// 提取类型参数部分并递归推导
				paramArgs := c.extractGenericArgs(paramType)
				argArgs := c.extractGenericArgs(argType)
				if len(paramArgs) == len(argArgs) {
					for i, paramArg := range paramArgs {
						if i < len(argArgs) {
							c.inferTypeParamFromArg(paramArg, argArgs[i], typeParams, typeArgMap)
						}
					}
				}
			}
		}
	}
}

// parseMapType 解析 Map 类型字符串，返回键类型和值类型
// 例如：map[string]int -> (string, int)
func (c *Compiler) parseMapType(mapType string) (keyType, valueType string) {
	if !strings.HasPrefix(mapType, "map[") {
		return "", ""
	}
	
	// 找到第一个 ]
	idx := strings.Index(mapType, "]")
	if idx == -1 {
		return "", ""
	}
	
	keyType = mapType[4:idx] // map[ 后面到 ]
	valueType = mapType[idx+1:] // ] 后面
	
	return keyType, valueType
}

// extractGenericArgs 提取泛型类型参数
// 例如：List<int, string> -> [int, string]
func (c *Compiler) extractGenericArgs(genericType string) []string {
	if !strings.Contains(genericType, "<") {
		return nil
	}
	
	start := strings.Index(genericType, "<")
	end := strings.LastIndex(genericType, ">")
	if start == -1 || end == -1 || start >= end {
		return nil
	}
	
	argsStr := genericType[start+1 : end]
	args := strings.Split(argsStr, ",")
	
	// 清理空白
	for i, arg := range args {
		args[i] = strings.TrimSpace(arg)
	}
	
	return args
}

// substituteTypeParamsInString 在字符串中替换类型参数
// 例如：将 T[] 中的 T 替换为 int，得到 int[]
func (c *Compiler) substituteTypeParamsInString(typeStr string, typeParams []string, typeArgs []string) string {
	if len(typeParams) != len(typeArgs) {
		return typeStr
	}
	
	result := typeStr
	for i, param := range typeParams {
		if i < len(typeArgs) && typeArgs[i] != "" {
			// 替换完整的类型参数名（避免部分匹配）
			// 使用单词边界匹配，但考虑到类型参数可能是类型名的一部分
			// 我们进行简单的字符串替换
			result = strings.ReplaceAll(result, param, typeArgs[i])
		}
	}
	
	return result
}

// findCommonArrayElementType 查找数组元素的公共类型
// 如果所有元素类型相同，返回该类型
// 如果类型不同但兼容（如 int 和 float），返回更通用的类型
// 否则返回第一个类型（或空字符串表示无法确定）
func (c *Compiler) findCommonArrayElementType(elemTypes []string) string {
	if len(elemTypes) == 0 {
		return ""
	}
	
	if len(elemTypes) == 1 {
		return elemTypes[0]
	}
	
	// 检查所有类型是否相同
	firstType := elemTypes[0]
	allSame := true
	for _, t := range elemTypes[1:] {
		if t != firstType {
			allSame = false
			break
		}
	}
	if allSame {
		return firstType
	}
	
	// 检查类型兼容性：int 和 float 的混合应返回 float
	hasInt := false
	hasFloat := false
	hasString := false
	hasBool := false
	
	for _, t := range elemTypes {
		switch t {
		case "int", "i8", "i16", "i32", "i64", "byte", "u8", "u16", "u32", "u64", "uint":
			hasInt = true
		case "float", "f32", "f64":
			hasFloat = true
		case "string":
			hasString = true
		case "bool":
			hasBool = true
		}
	}
	
	// 如果包含 int 和 float，返回 float（更通用）
	if hasInt && hasFloat {
		return "float"
	}
	
	// 如果只有数字类型，返回 int（简化处理）
	if hasInt && !hasFloat && !hasString && !hasBool {
		return "int"
	}
	
	// 如果只有字符串
	if hasString && !hasInt && !hasFloat && !hasBool {
		return "string"
	}
	
	// 如果只有布尔
	if hasBool && !hasInt && !hasFloat && !hasString {
		return "bool"
	}
	
	// 类型差异太大，返回第一个类型（让调用者决定）
	return firstType
}

// inferClosureParamTypes 从闭包参数推断参数类型字符串
// 例如：参数列表 [int $x, string $y] -> "int, string"
func (c *Compiler) inferClosureParamTypes(params []*ast.Parameter) string {
	if len(params) == 0 {
		return ""
	}
	
	var paramTypes []string
	for _, param := range params {
		if param.Type != nil {
			paramTypes = append(paramTypes, c.getTypeName(param.Type))
		} else {
			// 如果没有显式类型，尝试从默认值推断（如果存在）
			if param.Default != nil {
				defaultType := c.inferExprType(param.Default)
				if defaultType != "error" && defaultType != "" {
					paramTypes = append(paramTypes, defaultType)
				} else {
					paramTypes = append(paramTypes, "dynamic")
				}
			} else {
				paramTypes = append(paramTypes, "dynamic")
			}
		}
	}
	
	return strings.Join(paramTypes, ", ")
}

// inferMethodCallType 推断方法调用的返回类型
// 静态类型系统：方法必须在符号表中有签名
// 增强泛型推断：支持从泛型对象类型替换方法返回类型中的类型参数
func (c *Compiler) inferMethodCallType(e *ast.MethodCall) string {
	objType := c.inferExprType(e.Object)
	if objType == "error" {
		return "error"
	}
	if objType == "" {
		c.error(e.Object.Pos(), i18n.T(i18n.ErrTypeCannotInfer))
		return "error"
	}
	
	// 从泛型类型中提取基类名和类型参数（Box<int> -> Box, [int]）
	baseType := c.extractBaseTypeName(objType)
	typeArgs := c.extractTypeArgs(objType)
	
	// 如果类型没有命名空间分隔符，尝试加上当前命名空间
	if !strings.Contains(baseType, "\\") && !strings.Contains(baseType, ".") && c.currentNamespace != "" {
		// 尝试反斜杠分隔符
		fullType := c.currentNamespace + "\\" + baseType
		if sig := c.symbolTable.GetMethod(fullType, e.Method.Name, len(e.Arguments)); sig != nil {
			return c.substituteTypeParams(sig.ReturnType, baseType, typeArgs)
		}
		// 尝试点分隔符（如果命名空间用点）
		fullType2 := strings.ReplaceAll(c.currentNamespace, ".", "\\") + "\\" + baseType
		if fullType2 != fullType {
			if sig := c.symbolTable.GetMethod(fullType2, e.Method.Name, len(e.Arguments)); sig != nil {
				return c.substituteTypeParams(sig.ReturnType, baseType, typeArgs)
			}
		}
	}
	
	// 获取方法签名
	if sig := c.symbolTable.GetMethod(baseType, e.Method.Name, len(e.Arguments)); sig != nil {
		// 增强泛型推断：替换返回类型中的类型参数
		return c.substituteTypeParams(sig.ReturnType, baseType, typeArgs)
	}
	
	// 静态类型系统：方法必须存在
	c.error(e.Pos(), i18n.T(i18n.ErrMethodNotFound, objType, e.Method.Name, len(e.Arguments)))
	return "error"
}

// inferGenericNewExpr 增强泛型推断：从构造函数参数推断泛型类型
func (c *Compiler) inferGenericNewExpr(e *ast.NewExpr) string {
	classSig := c.symbolTable.GetClassSignature(e.ClassName.Name)
	if classSig == nil || len(classSig.TypeParams) == 0 {
		return e.ClassName.Name
	}
	
	if len(e.Arguments) == 0 {
		return e.ClassName.Name
	}
	
	// 获取构造函数签名
	methods := c.symbolTable.GetMethod(e.ClassName.Name, "__construct", len(e.Arguments))
	if methods == nil {
		// 没有显式构造函数，尝试从第一个参数推断
		firstArgType := c.inferExprType(e.Arguments[0])
		if firstArgType != "" && firstArgType != "dynamic" && firstArgType != "error" {
			return e.ClassName.Name + "<" + firstArgType + ">"
		}
		return e.ClassName.Name
	}
	
	// 尝试从参数类型推断泛型类型参数
	inferredTypeArgs := make([]string, len(classSig.TypeParams))
	for i := range inferredTypeArgs {
		inferredTypeArgs[i] = "" // 初始化为空
	}
	
	// 遍历参数，尝试匹配类型参数
	for i, param := range methods.ParamTypes {
		if i >= len(e.Arguments) {
			break
		}
		
		argType := c.inferExprType(e.Arguments[i])
		if argType == "" || argType == "dynamic" || argType == "error" {
			continue
		}
		
		// 检查参数类型是否是类型参数
		for j, tp := range classSig.TypeParams {
			if param == tp.Name {
				// 找到匹配的类型参数
				if inferredTypeArgs[j] == "" {
					inferredTypeArgs[j] = argType
				}
				break
			}
		}
	}
	
	// 检查是否所有类型参数都被推断
	allInferred := true
	for _, t := range inferredTypeArgs {
		if t == "" {
			allInferred = false
			break
		}
	}
	
	if allInferred && len(inferredTypeArgs) > 0 {
		return e.ClassName.Name + "<" + strings.Join(inferredTypeArgs, ", ") + ">"
	}
	
	// 如果无法完全推断，至少尝试从第一个参数推断第一个类型参数
	if len(inferredTypeArgs) > 0 && inferredTypeArgs[0] != "" {
		return e.ClassName.Name + "<" + inferredTypeArgs[0] + ">"
	}
	
	return e.ClassName.Name
}

// extractTypeArgs 从泛型类型中提取类型参数列表
// 例如：Box<int, string> -> ["int", "string"]
func (c *Compiler) extractTypeArgs(typeName string) []string {
	start := strings.Index(typeName, "<")
	end := strings.LastIndex(typeName, ">")
	if start == -1 || end == -1 || end <= start {
		return nil
	}
	
	argsStr := typeName[start+1 : end]
	if argsStr == "" {
		return nil
	}
	
	// 简单分割（不处理嵌套泛型）
	args := strings.Split(argsStr, ",")
	result := make([]string, len(args))
	for i, arg := range args {
		result[i] = strings.TrimSpace(arg)
	}
	return result
}

// substituteTypeParams 替换返回类型中的类型参数
// 例如：返回类型是 T，类是 Box，类型参数是 [int]，则返回 int
func (c *Compiler) substituteTypeParams(returnType, className string, typeArgs []string) string {
	if len(typeArgs) == 0 {
		return returnType
	}
	
	// 获取类的类型参数定义
	classSig := c.symbolTable.GetClassSignature(className)
	if classSig == nil || len(classSig.TypeParams) == 0 {
		return returnType
	}
	
	// 构建类型参数映射
	for i, tp := range classSig.TypeParams {
		if i >= len(typeArgs) {
			break
		}
		// 替换类型参数
		if returnType == tp.Name {
			return typeArgs[i]
		}
		// 处理数组类型 T[]
		if returnType == tp.Name+"[]" {
			return typeArgs[i] + "[]"
		}
		// 处理 Map 类型中的类型参数
		if strings.Contains(returnType, tp.Name) {
			returnType = strings.ReplaceAll(returnType, tp.Name, typeArgs[i])
		}
	}
	
	return returnType
}

// inferStaticAccessType 推断静态访问的类型
// 静态类型系统：静态成员必须在符号表中有签名
func (c *Compiler) inferStaticAccessType(e *ast.StaticAccess) string {
	var className string
	switch cls := e.Class.(type) {
	case *ast.Identifier:
		className = cls.Name
		// 如果类名不包含命名空间，且当前有命名空间，尝试添加命名空间前缀
		if !strings.Contains(className, "\\") && c.currentNamespace != "" {
			// 先尝试带命名空间的完整类名
			fullName := c.currentNamespace + "\\" + className
			// 检查符号表中是否存在（通过查找方法或属性）
			if _, ok := c.symbolTable.ClassMethods[fullName]; ok {
				className = fullName
			} else if _, ok := c.symbolTable.ClassProperties[fullName]; ok {
				className = fullName
			}
		}
	case *ast.SelfExpr:
		className = c.currentClassName
	case *ast.ParentExpr:
		if c.currentClassName != "" {
			// 提取基类名
			baseClassName := c.extractBaseTypeName(c.currentClassName)
			if parent, ok := c.symbolTable.ClassParents[baseClassName]; ok {
				className = parent
			}
		}
	default:
		c.error(e.Pos(), i18n.T(i18n.ErrTypeCannotInfer))
		return "error"
	}
	
	if className == "" {
		c.error(e.Pos(), i18n.T(i18n.ErrTypeCannotInfer))
		return "error"
	}
	
	switch member := e.Member.(type) {
	case *ast.Variable:
		// 静态属性
		if sig := c.symbolTable.GetProperty(className, member.Name); sig != nil {
			return sig.Type
		}
		// 静态类型系统：静态属性必须存在
		c.error(e.Pos(), i18n.T(i18n.ErrStaticMemberNotFound, className, "$"+member.Name))
		return "error"
	case *ast.Identifier:
		// 类常量 - 暂时返回 dynamic，后续可以增强常量类型追踪
		return "dynamic"
	case *ast.CallExpr:
		// 静态方法调用
		if fn, ok := member.Function.(*ast.Identifier); ok {
			if sig := c.symbolTable.GetMethod(className, fn.Name, len(member.Arguments)); sig != nil {
				return sig.ReturnType
			}
			// 静态类型系统：静态方法必须存在
			c.error(e.Pos(), i18n.T(i18n.ErrMethodNotFound, className, fn.Name, len(member.Arguments)))
			return "error"
		}
		c.error(e.Pos(), i18n.T(i18n.ErrTypeCannotInfer))
		return "error"
	default:
		c.error(e.Pos(), i18n.T(i18n.ErrTypeCannotInfer))
		return "error"
	}
}

// inferPropertyAccessType 推断属性访问的类型
// 静态类型系统：属性必须在符号表中有签名
func (c *Compiler) inferPropertyAccessType(e *ast.PropertyAccess) string {
	objType := c.inferExprType(e.Object)
	if objType == "error" {
		return "error"
	}
	if objType == "" {
		c.error(e.Object.Pos(), i18n.T(i18n.ErrTypeCannotInfer))
		return "error"
	}
	
	// dynamic 类型允许任何属性访问
	if objType == "dynamic" || objType == "unknown" {
		return "dynamic"
	}
	
	// 特殊属性
	if e.Property.Name == "length" {
		return "int"
	}
	
	// 从符号表获取属性类型
	if sig := c.symbolTable.GetProperty(objType, e.Property.Name); sig != nil {
		return sig.Type
	}
	
	// 静态类型系统：属性必须存在
	c.error(e.Pos(), i18n.T(i18n.ErrPropertyNotFound, objType, e.Property.Name))
	return "error"
}

// isInterfaceType 检查类型名是否为已声明的接口
func (c *Compiler) isInterfaceType(typeName string) bool {
	if class, ok := c.classes[typeName]; ok {
		return class.IsInterface
	}
	return false
}

// checkBinaryOpTypes 检查二元运算符的操作数类型是否兼容
func (c *Compiler) checkBinaryOpTypes(op token.Token, leftType, rightType string) {
	// 判断是否是数字类型
	isNumeric := func(t string) bool {
		switch t {
		case "int", "float", "i8", "i16", "i32", "i64", "u8", "u16", "u32", "u64", "f32", "f64", "byte":
			return true
		}
		return false
	}

	// 判断是否是整数类型
	isInteger := func(t string) bool {
		switch t {
		case "int", "i8", "i16", "i32", "i64", "u8", "u16", "u32", "u64", "byte":
			return true
		}
		return false
	}

	switch op.Type {
	case token.PLUS:
		// + 运算符：严格类型检查，不允许隐式类型转换
		// 只允许: string + string, int + int, float + float
		if leftType == "string" && rightType == "string" {
			return // 字符串拼接
		}
		if leftType == rightType && isNumeric(leftType) {
			return // 相同数字类型相加
		}
		// 其他组合都是错误的（包括 int + float, string + int 等）
		c.error(op.Pos, i18n.T(i18n.ErrInvalidBinaryOp, "+", leftType, rightType))

	case token.MINUS, token.STAR, token.SLASH, token.PERCENT:
		// 算术运算符：严格类型检查，两边必须是相同的数字类型
		if leftType != rightType {
			c.error(op.Pos, i18n.T(i18n.ErrInvalidBinaryOp, op.Literal, leftType, rightType))
			return
		}
		if !isNumeric(leftType) {
			c.error(op.Pos, i18n.T(i18n.ErrInvalidBinaryOp, op.Literal, leftType, rightType))
		}

	case token.BIT_AND, token.BIT_OR, token.BIT_XOR, token.LEFT_SHIFT, token.RIGHT_SHIFT:
		// 位运算符：两边必须都是整数
		if !isInteger(leftType) || !isInteger(rightType) {
			c.error(op.Pos, i18n.T(i18n.ErrInvalidBinaryOp, op.Literal, leftType, rightType))
		}

	case token.LT, token.LE, token.GT, token.GE:
		// 比较运算符：两边必须是可比较的类型（都是数字或都是字符串）
		if isNumeric(leftType) && isNumeric(rightType) {
			return
		}
		if leftType == "string" && rightType == "string" {
			return
		}
		c.error(op.Pos, i18n.T(i18n.ErrInvalidBinaryOp, op.Literal, leftType, rightType))

	case token.EQ, token.NE:
		// == 和 != 需要两边类型兼容
		// 允许: 数字与数字比较, 字符串与字符串比较, bool与bool比较, null与任何类型比较
		if leftType == "null" || rightType == "null" {
			return // null 可以与任何类型比较
		}
		if isNumeric(leftType) && isNumeric(rightType) {
			return // 数字类型之间可以比较
		}
		if leftType == rightType {
			return // 相同类型可以比较
		}
		c.error(op.Pos, i18n.T(i18n.ErrInvalidBinaryOp, op.Literal, leftType, rightType))
	}
}

// checkReturnType 检查返回值类型是否匹配
// checkReturnType 检查返回值类型
// 静态类型系统：严格检查返回类型
func (c *Compiler) checkReturnType(pos token.Position, expr ast.Expression, expectedType ast.TypeNode) {
	if expectedType == nil {
		return
	}
	
	actualType := c.inferExprType(expr)
	// 静态类型系统：error 类型表示已报错，跳过避免级联
	if actualType == "error" {
		return
	}
	
	expectedTypeName := c.getTypeName(expectedType)
	if expectedTypeName == "" || expectedTypeName == "unknown" {
		return
	}
	
	// 类型兼容性检查
	if !c.isTypeCompatible(actualType, expectedTypeName) {
		c.error(pos, i18n.T(i18n.ErrTypeMismatch, expectedTypeName, actualType))
	}
}

// getTypeName 从 TypeNode 获取类型名
// 注意：这里返回的是类型名称，不会自动解析类型别名
// 如果需要解析别名，应调用 symbolTable.ResolveTypeAlias
func (c *Compiler) getTypeName(t ast.TypeNode) string {
	switch typ := t.(type) {
	case *ast.SimpleType:
		// 简单类型名（如 int, string）可能已经被解析为类型别名
		// 但这里我们返回原始名称，由调用者决定是否需要解析
		return typ.Name
	case *ast.ClassType:
		return typ.Name.Literal
	case *ast.ArrayType:
		// 类型化数组：获取元素类型 + []
		if typ.ElementType != nil {
			elemType := c.getTypeName(typ.ElementType)
			return elemType + "[]"
		}
		return "array"
	case *ast.MapType:
		keyType := c.getTypeName(typ.KeyType)
		valueType := c.getTypeName(typ.ValueType)
		return "map[" + keyType + "]" + valueType
	case *ast.NullableType:
		return c.getTypeName(typ.Inner)
	case *ast.TupleType:
		return "tuple"
	case *ast.UnionType:
		var names []string
		for _, t := range typ.Types {
			names = append(names, c.getTypeName(t))
		}
		return strings.Join(names, "|")
	case *ast.NullType:
		return "null"
	case *ast.GenericType:
		// 泛型类型: List<int> -> List<int>
		base := c.getTypeName(typ.BaseType)
		var args []string
		for _, arg := range typ.TypeArgs {
			args = append(args, c.getTypeName(arg))
		}
		return base + "<" + strings.Join(args, ", ") + ">"
	case *ast.TypeParameter:
		// 类型参数直接返回其名称
		return typ.Name.Name
	default:
		return "unknown"
	}
}

// isBytesArrayType 检查是否是 byte[] 类型
func (c *Compiler) isBytesArrayType(t ast.TypeNode) bool {
	arrType, ok := t.(*ast.ArrayType)
	if !ok {
		return false
	}
	// 检查元素类型是否是 byte 或 u8
	if simpleType, ok := arrType.ElementType.(*ast.SimpleType); ok {
		return simpleType.Name == "byte" || simpleType.Name == "u8"
	}
	return false
}

// isTypeCompatible 检查类型兼容性
// 静态类型系统：不再跳过空类型检查
func (c *Compiler) isTypeCompatible(actual, expected string) bool {
	// error 类型表示已报错，避免级联错误
	if actual == "error" || expected == "error" {
		return true
	}
	
	// 静态类型系统：空类型不应出现，视为不兼容
	if actual == "" || expected == "" {
		return false
	}
	
	// 解析类型别名：类型别名会解析到底层类型，但新类型保持独立
	resolvedActual := c.symbolTable.ResolveTypeAlias(actual)
	resolvedExpected := c.symbolTable.ResolveTypeAlias(expected)
	
	// 检查是否是新类型（新类型需要显式转换，不能隐式转换）
	if c.symbolTable.IsNewType(actual) && actual != expected {
		// 新类型只能与自身兼容，或者与基础类型兼容（需要显式转换）
		// 这里我们先实现基本检查，显式转换在类型转换表达式处处理
		newTypeInfo := c.symbolTable.GetNewTypeInfo(actual)
		if newTypeInfo != nil {
			// 检查是否可以转换为基础类型（显式转换）
			// 这里我们先不处理，等待类型转换表达式处理
		}
		return false
	}
	if c.symbolTable.IsNewType(expected) && actual != expected {
		return false
	}
	
	if resolvedActual == resolvedExpected {
		return true
	}
	
	// 更新 actual 和 expected 为解析后的类型（用于后续兼容性检查）
	actual = resolvedActual
	expected = resolvedExpected
	
	if actual == expected {
		return true
	}
	
	// dynamic/unknown 类型接受任何值
	if expected == "dynamic" || expected == "unknown" {
		return true
	}
	
	// 泛型类型参数（单个大写字母如 T, K, V, E, R）视为 dynamic 类型
	// 这实现了类型擦除：在编译时泛型参数可以接受任何类型
	if c.isTypeParameter(expected) {
		return true
	}
	if c.isTypeParameter(actual) {
		return true
	}
	
	// 泛型类型匹配: Box<int> 和 Box<int>
	// 只比较基础类型名，忽略类型参数（类型擦除）
	if strings.Contains(actual, "<") && strings.Contains(expected, "<") {
		actualBase := strings.Split(actual, "<")[0]
		expectedBase := strings.Split(expected, "<")[0]
		if actualBase == expectedBase {
			return true // 同一泛型类的不同实例化视为兼容（类型擦除）
		}
	}
	
	// 泛型类型赋给非泛型基类型: Box<int> 赋给 Box
	if strings.Contains(actual, "<") {
		actualBase := strings.Split(actual, "<")[0]
		if actualBase == expected {
			return true
		}
	}
	
	// 非泛型类型赋给泛型类型: Box 赋给 Box<int>（需要类型参数）
	if strings.Contains(expected, "<") {
		expectedBase := strings.Split(expected, "<")[0]
		if actual == expectedBase {
			return true // 类型擦除后兼容
		}
	}
	
	// null 可以赋值给可空类型或包含 null 的联合类型
	if actual == "null" {
		// 检查 expected 是否包含 null
		if strings.Contains(expected, "|") {
			expectedTypes := strings.Split(expected, "|")
			for _, t := range expectedTypes {
				if strings.TrimSpace(t) == "null" {
					return true
				}
			}
			return false // 联合类型不包含 null，不允许赋值 null
		}
		// 非联合类型，null 只能赋给 null 类型
		return false
	}
	
	// 检查 expected 是否是联合类型
	if strings.Contains(expected, "|") {
		expectedTypes := strings.Split(expected, "|")
		for _, t := range expectedTypes {
			trimmed := strings.TrimSpace(t)
			if c.isTypeCompatible(actual, trimmed) {
				return true
			}
		}
		return false
	}
	
	// 检查 actual 是否是联合类型（赋值给非联合类型时需要所有成员都兼容）
	if strings.Contains(actual, "|") {
		actualTypes := strings.Split(actual, "|")
		for _, t := range actualTypes {
			if !c.isTypeCompatible(strings.TrimSpace(t), expected) {
				return false
			}
		}
		return true
	}
	
	// int 可以隐式转换为 float
	if actual == "int" && expected == "float" {
		return true
	}
	
	// 数字类型兼容性
	intTypes := map[string]bool{"int": true, "i8": true, "i16": true, "i32": true, "i64": true, "byte": true}
	uintTypes := map[string]bool{"uint": true, "u8": true, "u16": true, "u32": true, "u64": true}
	floatTypes := map[string]bool{"float": true, "f32": true, "f64": true}
	
	// 整数类型之间兼容
	if intTypes[actual] && intTypes[expected] {
		return true
	}
	// 无符号整数类型之间兼容
	if uintTypes[actual] && uintTypes[expected] {
		return true
	}
	// 浮点类型之间兼容
	if floatTypes[actual] && floatTypes[expected] {
		return true
	}
	// 任意整数可以赋给 float
	if (intTypes[actual] || uintTypes[actual]) && floatTypes[expected] {
		return true
	}
	
	// 数组类型兼容性
	if strings.HasSuffix(actual, "[]") && strings.HasSuffix(expected, "[]") {
		actualElem := strings.TrimSuffix(actual, "[]")
		expectedElem := strings.TrimSuffix(expected, "[]")
		return c.isTypeCompatible(actualElem, expectedElem)
	}
	
	// 通用数组类型：T[] 可以赋给 array
	if expected == "array" && strings.HasSuffix(actual, "[]") {
		return true
	}
	// 反向：array 可以赋给 T[]（运行时可能出错，但编译期允许）
	if actual == "array" && strings.HasSuffix(expected, "[]") {
		return true
	}
	
	// 【严格分离】SuperArray 和类型化数组是完全不同的类型，不兼容
	// superarray 只能与 superarray 兼容
	// 不再允许 superarray 与 array/T[] 互相赋值
	
	// Map 类型兼容性
	if expected == "map" && strings.HasPrefix(actual, "map[") {
		return true
	}
	
	// 对象类型兼容性（子类可以赋给父类）
	if c.isSubclassOf(actual, expected) {
		return true
	}
	
	// unknown 类型可以接受任何值
	if expected == "unknown" {
		return true
	}
	
	// 命名空间匹配：sola.net.tcp\TcpClient 与 TcpClient 应匹配
	// 提取基类名进行比较
	actualBase := actual
	if idx := strings.LastIndex(actual, "\\"); idx != -1 {
		actualBase = actual[idx+1:]
	}
	expectedBase := expected
	if idx := strings.LastIndex(expected, "\\"); idx != -1 {
		expectedBase = expected[idx+1:]
	}
	if actualBase == expectedBase && actualBase != "" {
		return true
	}
	
	return false
}

// extractBaseTypeName 从泛型类型中提取基类名
// Box<int> -> Box, Map<string, int> -> Map
func (c *Compiler) extractBaseTypeName(typeName string) string {
	if idx := strings.Index(typeName, "<"); idx != -1 {
		return typeName[:idx]
	}
	return typeName
}

// isTypeParameter 检查类型名是否是泛型类型参数
// 类型参数通常是单个大写字母（T, K, V, E, R 等）
func (c *Compiler) isTypeParameter(typeName string) bool {
	if len(typeName) == 0 {
		return false
	}
	// 单个大写字母
	if len(typeName) == 1 && typeName[0] >= 'A' && typeName[0] <= 'Z' {
		return true
	}
	// 常见的多字符类型参数名
	commonTypeParams := map[string]bool{
		"TKey": true, "TValue": true, "TResult": true, "TElement": true,
		"Key": true, "Value": true, "Element": true,
	}
	return commonTypeParams[typeName]
}

// isSubclassOf 检查 child 是否是 parent 的子类
func (c *Compiler) isSubclassOf(child, parent string) bool {
	current := child
	visited := make(map[string]bool)
	
	for current != "" && !visited[current] {
		visited[current] = true
		if current == parent {
			return true
		}
		current = c.symbolTable.ClassParents[current]
	}
	
	return false
}

// evalConstInt 在编译时计算常量整数表达式
func (c *Compiler) evalConstInt(expr ast.Expression) int {
	switch e := expr.(type) {
	case *ast.IntegerLiteral:
		return int(e.Value)
	case *ast.UnaryExpr:
		if e.Operator.Type == token.MINUS {
			return -c.evalConstInt(e.Operand)
		}
	case *ast.BinaryExpr:
		left := c.evalConstInt(e.Left)
		right := c.evalConstInt(e.Right)
		switch e.Operator.Type {
		case token.PLUS:
			return left + right
		case token.MINUS:
			return left - right
		case token.STAR:
			return left * right
		case token.SLASH:
			if right != 0 {
				return left / right
			}
		}
	}
	c.error(expr.Pos(), i18n.T(i18n.ErrArraySizeNotConst))
	return -1
}

// validateGenericConstraints 验证泛型类型参数的约束
func (c *Compiler) validateGenericConstraints(className string, typeArgs []ast.TypeNode) {
	// 获取类的泛型签名
	classSig := c.symbolTable.GetClassSignature(className)
	if classSig == nil {
		return // 不是泛型类，无需验证
	}
	
	// 检查类型参数数量是否匹配
	if len(typeArgs) != len(classSig.TypeParams) {
		c.error(typeArgs[0].Pos(), i18n.T(i18n.ErrGenericTypeArgCount, className, len(classSig.TypeParams), len(typeArgs)))
		return
	}
	
	// 验证每个类型参数是否满足约束
	for i, typeArg := range typeArgs {
		if i >= len(classSig.TypeParams) {
			break
		}
		typeParam := classSig.TypeParams[i]
		typeArgName := c.getTypeName(typeArg)
		
		// 验证 extends 约束
		if typeParam.ExtendsType != "" {
			if !c.symbolTable.ValidateTypeConstraint(typeArgName, typeParam.ExtendsType) {
				c.error(typeArg.Pos(), i18n.T(i18n.ErrGenericConstraintViolated, typeArgName, typeParam.ExtendsType))
			}
		}
		
		// 验证 implements 约束
		for _, implType := range typeParam.ImplementsTypes {
			if !c.symbolTable.CheckImplements(typeArgName, implType) {
				c.error(typeArg.Pos(), i18n.T(i18n.ErrGenericConstraintViolated, typeArgName, implType))
			}
		}
	}
}

