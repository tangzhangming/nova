package compiler

import (
	"fmt"
	"strings"

	"github.com/tangzhangming/nova/internal/ast"
	"github.com/tangzhangming/nova/internal/bytecode"
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

	errors []Error
}

// Local 局部变量
type Local struct {
	Name     string
	Depth    int
	Index    int
	TypeName string // 变量类型名（用于类型检查）
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
		function:    fn,
		locals:      make([]Local, 256),
		classes:     make(map[string]*bytecode.Class),
		enums:       make(map[string]*bytecode.Enum),
		globalTypes: make(map[string]string),
		symbolTable: NewSymbolTable(),
	}
}

// NewWithSymbolTable 创建带符号表的编译器（用于多文件编译）
func NewWithSymbolTable(st *SymbolTable) *Compiler {
	fn := bytecode.NewFunction("<script>")
	return &Compiler{
		function:    fn,
		locals:      make([]Local, 256),
		classes:     make(map[string]*bytecode.Class),
		enums:       make(map[string]*bytecode.Enum),
		globalTypes: make(map[string]string),
		symbolTable: st,
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
	
	// 预留 slot 0 给调用者（与 CompileFunction 保持一致）
	c.addLocal("")

	// ========== Phase 2: 编译 ==========
	// 编译类、接口和枚举声明
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
		}
	}

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

	// 创建新函数
	c.function = bytecode.NewFunction(name)
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

	// 恢复状态
	c.function = prevFn
	c.locals = prevLocals
	c.localCount = prevLocalCount
	c.returnType = prevReturnType
	c.expectedReturns = prevExpectedReturns

	return fn
}

// evaluateConstExpr 计算常量表达式的值（用于默认参数）
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
	default:
		// 非常量表达式，返回 null
		return bytecode.NullValue
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
		for _, inner := range s.Statements {
			c.compileStmt(inner)
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
	
	// 静态类型检查：如果有显式类型和初始值，检查类型匹配
	if s.Type != nil && s.Value != nil {
		actualType := c.inferExprType(s.Value)
		// 严格模式：类型必须兼容
		if actualType != "" && declaredType != "" {
			if !c.isTypeCompatible(actualType, declaredType) {
				c.error(s.Value.Pos(), i18n.T(i18n.ErrCannotAssign, actualType, declaredType))
			}
		}
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
	
	// 如果是类型推断 (:=)，从值推断类型
	if s.Type == nil && s.Value != nil {
		declaredType = c.inferExprType(s.Value)
	}

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
	// 编译条件
	c.compileExpr(s.Condition)

	// 条件为假时跳转
	thenJump := c.emitJump(bytecode.OpJumpIfFalse)
	c.emit(bytecode.OpPop) // 弹出条件值

	// 编译 then 分支
	c.compileStmt(s.Then)

	elseJump := c.emitJump(bytecode.OpJump)

	// 修补 then 跳转
	c.patchJump(thenJump)
	c.emit(bytecode.OpPop) // 弹出条件值

	// 编译 elseif 分支
	for _, elseIf := range s.ElseIfs {
		c.compileExpr(elseIf.Condition)
		nextJump := c.emitJump(bytecode.OpJumpIfFalse)
		c.emit(bytecode.OpPop)
		c.compileStmt(elseIf.Body)
		elseJump = c.emitJump(bytecode.OpJump)
		c.patchJump(nextJump)
		c.emit(bytecode.OpPop)
	}

	// 编译 else 分支
	if s.Else != nil {
		c.compileStmt(s.Else)
	}

	c.patchJump(elseJump)
}

func (c *Compiler) compileWhileStmt(s *ast.WhileStmt) {
	loopStart := c.currentChunk().Len()
	prevLoopStart := c.loopStart
	prevBreakJumps := c.breakJumps
	c.loopStart = loopStart
	c.breakJumps = nil
	c.loopDepth++

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
}

func (c *Compiler) compileForStmt(s *ast.ForStmt) {
	c.beginScope()

	// 初始化
	if s.Init != nil {
		c.compileStmt(s.Init)
	}

	loopStart := c.currentChunk().Len()
	prevLoopStart := c.loopStart
	prevBreakJumps := c.breakJumps
	c.loopStart = loopStart
	c.breakJumps = nil
	c.loopDepth++

	// 条件
	var exitJump int
	if s.Condition != nil {
		c.compileExpr(s.Condition)
		exitJump = c.emitJump(bytecode.OpJumpIfFalse)
		c.emit(bytecode.OpPop)
	}

	// 循环体
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
}

func (c *Compiler) compileForeachStmt(s *ast.ForeachStmt) {
	c.beginScope()

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
		c.addLocal(s.Key.Name)
	}
	
	// 声明 value 变量
	c.emit(bytecode.OpNull)
	valueSlot := c.localCount
	c.addLocal(s.Value.Name)

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
		// 单返回值
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

	case *ast.NewExpr:
		c.compileNewExpr(e)

	case *ast.ClosureExpr:
		c.compileClosureExpr(e)

	case *ast.ArrowFuncExpr:
		c.compileArrowFuncExpr(e)

	case *ast.TypeCastExpr:
		c.compileTypeCastExpr(e)

	default:
		c.error(expr.Pos(), i18n.T(i18n.ErrUnsupportedExpr))
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
			if propSig != nil && propSig.Type != "" && propSig.Type != "any" {
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
		} else {
			idx := c.makeConstant(bytecode.NewString(t.Name))
			c.emitU16(bytecode.OpStoreGlobal, idx)
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
		if len(e.Arguments) != 1 {
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
	
	c.compileExpr(e.Function)
	for _, arg := range e.Arguments {
		c.compileExpr(arg)
	}
	c.emitByte(bytecode.OpCall, byte(len(e.Arguments)))
}

// checkCallArgTypes 检查函数调用参数类型
func (c *Compiler) checkCallArgTypes(e *ast.CallExpr) {
	var sig *FunctionSignature
	
	switch fn := e.Function.(type) {
	case *ast.Identifier:
		sig = c.symbolTable.GetFunction(fn.Name)
	case *ast.Variable:
		// 变量作为函数调用，尝试从变量类型推断
		// 暂不严格检查
		return
	default:
		return
	}
	
	if sig == nil {
		return // 未知函数，跳过检查（内置函数或动态调用）
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
		if expectedType == "any" || expectedType == "mixed" {
			continue // any 类型接受任何值
		}
		
		actualType := c.inferExprType(arg)
		if actualType == "" {
			continue // 无法推断类型时跳过
		}
		
		if !c.isTypeCompatible(actualType, expectedType) {
			c.error(arg.Pos(), i18n.T(i18n.ErrTypeMismatch, expectedType, actualType))
		}
	}
}

func (c *Compiler) compileIndexExpr(e *ast.IndexExpr) {
	c.compileExpr(e.Object)
	c.compileExpr(e.Index)

	// 判断是数组还是 Map (运行时处理)
	c.emit(bytecode.OpArrayGet)
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

	c.compileExpr(e.Object)
	for _, arg := range e.Arguments {
		c.compileExpr(arg)
	}
	idx := c.makeConstant(bytecode.NewString(e.Method.Name))
	c.emitU16(bytecode.OpCallMethod, idx)
	c.currentChunk().WriteU8(byte(len(e.Arguments)), c.currentLine) // 参数数量
}

// checkMethodCallArgTypes 检查方法调用参数类型
func (c *Compiler) checkMethodCallArgTypes(e *ast.MethodCall) {
	// 获取对象类型
	objType := c.inferExprType(e.Object)
	if objType == "" {
		return // 无法推断对象类型，跳过检查
	}
	
	// 获取方法签名
	sig := c.symbolTable.GetMethod(objType, e.Method.Name, len(e.Arguments))
	if sig == nil {
		return // 未找到方法签名，跳过检查
	}
	
	// 检查每个参数类型
	for i, arg := range e.Arguments {
		if i >= len(sig.ParamTypes) {
			break
		}
		expectedType := sig.ParamTypes[i]
		if expectedType == "any" || expectedType == "mixed" {
			continue
		}
		
		actualType := c.inferExprType(arg)
		if actualType == "" {
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
	case *ast.SelfExpr:
		className = c.currentClassName // 使用当前类名
		if className == "" {
			className = "self"
		}
	case *ast.ParentExpr:
		// 获取父类名
		if c.currentClassName != "" {
			if parent, ok := c.symbolTable.ClassParents[c.currentClassName]; ok {
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
			// 静态类型检查：检查静态方法参数类型
			c.checkStaticMethodArgTypes(className, fn.Name, member.Arguments)
			
			nameIdx := c.makeConstant(bytecode.NewString(fn.Name))
			// 编译参数
			for _, arg := range member.Arguments {
				c.compileExpr(arg)
			}
			c.emitU16(bytecode.OpCallStatic, classIdx)
			c.currentChunk().WriteU16(nameIdx, c.currentLine)
			c.currentChunk().WriteU8(byte(len(member.Arguments)), c.currentLine)
		}
	default:
		c.error(e.Pos(), i18n.T(i18n.ErrInvalidStaticMember))
	}
}

// checkStaticMethodArgTypes 检查静态方法参数类型
func (c *Compiler) checkStaticMethodArgTypes(className, methodName string, args []ast.Expression) {
	// 获取方法签名
	sig := c.symbolTable.GetMethod(className, methodName, len(args))
	if sig == nil {
		return
	}
	
	// 检查每个参数类型
	for i, arg := range args {
		if i >= len(sig.ParamTypes) {
			break
		}
		expectedType := sig.ParamTypes[i]
		if expectedType == "any" || expectedType == "mixed" {
			continue
		}
		
		actualType := c.inferExprType(arg)
		if actualType == "" {
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
	
	idx := c.makeConstant(bytecode.NewString(e.ClassName.Name))
	c.emitU16(bytecode.OpNewObject, idx)

	// 调用构造函数
	for _, arg := range e.Arguments {
		c.compileExpr(arg)
	}
	constructorIdx := c.makeConstant(bytecode.NewString("__construct"))
	c.emitU16(bytecode.OpCallMethod, constructorIdx)
	c.currentChunk().WriteU8(byte(len(e.Arguments)), c.currentLine) // 参数数量
}

// checkConstructorArgTypes 检查构造函数参数类型
func (c *Compiler) checkConstructorArgTypes(e *ast.NewExpr) {
	className := e.ClassName.Name
	
	// 获取构造函数签名
	sig := c.symbolTable.GetMethod(className, "__construct", len(e.Arguments))
	if sig == nil {
		return // 未找到构造函数，跳过检查
	}
	
	// 检查每个参数类型
	for i, arg := range e.Arguments {
		if i >= len(sig.ParamTypes) {
			break
		}
		expectedType := sig.ParamTypes[i]
		if expectedType == "any" || expectedType == "mixed" {
			continue
		}
		
		actualType := c.inferExprType(arg)
		if actualType == "" {
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

// ============================================================================
// 作用域管理
// ============================================================================

func (c *Compiler) beginScope() {
	c.scopeDepth++
}

func (c *Compiler) endScope() {
	c.scopeDepth--

	// 弹出当前作用域的局部变量
	for c.localCount > 0 && c.locals[c.localCount-1].Depth > c.scopeDepth {
		c.emit(bytecode.OpPop)
		c.localCount--
	}
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
func (c *Compiler) getVariableType(name string) string {
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
	c.errors = append(c.errors, Error{Pos: pos, Message: fmt.Sprintf(message, args...)})
}

// inferExprType 推断表达式的类型名
func (c *Compiler) inferExprType(expr ast.Expression) string {
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
	case *ast.ArrayLiteral:
		// 尝试推断数组元素类型
		if len(e.Elements) > 0 {
			elemType := c.inferExprType(e.Elements[0])
			if elemType != "" && elemType != "any" {
				return elemType + "[]"
			}
		}
		return "array"
	case *ast.MapLiteral:
		// 尝试推断 Map 键值类型
		if len(e.Pairs) > 0 {
			keyType := c.inferExprType(e.Pairs[0].Key)
			valueType := c.inferExprType(e.Pairs[0].Value)
			if keyType != "" && valueType != "" {
				return "map[" + keyType + "]" + valueType
			}
		}
		return "map"
	case *ast.BinaryExpr:
		leftType := c.inferExprType(e.Left)
		rightType := c.inferExprType(e.Right)
		
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
		// 变量类型必须明确
		return ""
	case *ast.ThisExpr:
		// $this 的类型是当前类
		if c.currentClassName != "" {
			return c.currentClassName
		}
		return "object"
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
		return e.ClassName.Name
	case *ast.TernaryExpr:
		// 三元表达式：两个分支类型应该相同
		thenType := c.inferExprType(e.Then)
		elseType := c.inferExprType(e.Else)
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
		return "any"
	case *ast.PropertyAccess:
		// 属性访问：从符号表获取属性类型
		return c.inferPropertyAccessType(e)
	case *ast.TypeCastExpr:
		// 类型转换：返回目标类型
		return c.getTypeName(e.TargetType)
	case *ast.ClosureExpr:
		// 闭包表达式
		if e.ReturnType != nil {
			return "func(): " + c.getTypeName(e.ReturnType)
		}
		return "func"
	case *ast.ArrowFuncExpr:
		// 箭头函数
		bodyType := c.inferExprType(e.Body)
		return "func(): " + bodyType
	case *ast.Identifier:
		// 可能是类名、枚举等
		return e.Name
	default:
		return ""
	}
}

// inferCallExprType 推断函数调用的返回类型
func (c *Compiler) inferCallExprType(e *ast.CallExpr) string {
	switch fn := e.Function.(type) {
	case *ast.Identifier:
		// 普通函数调用
		if sig := c.symbolTable.GetFunction(fn.Name); sig != nil {
			return sig.ReturnType
		}
		// 可能是变量保存的闭包
		if t := c.getVariableType(fn.Name); t != "" {
			// 如果是 func 类型，尝试解析返回类型
			if strings.HasPrefix(t, "func") {
				if idx := strings.LastIndex(t, ": "); idx != -1 {
					return t[idx+2:]
				}
			}
			return t
		}
		return ""
	case *ast.Variable:
		// 变量作为函数调用
		if t := c.getVariableType(fn.Name); t != "" {
			if strings.HasPrefix(t, "func") {
				if idx := strings.LastIndex(t, ": "); idx != -1 {
					return t[idx+2:]
				}
			}
		}
		return ""
	default:
		return ""
	}
}

// inferMethodCallType 推断方法调用的返回类型
func (c *Compiler) inferMethodCallType(e *ast.MethodCall) string {
	objType := c.inferExprType(e.Object)
	if objType == "" {
		return ""
	}
	
	// 获取方法签名
	if sig := c.symbolTable.GetMethod(objType, e.Method.Name, len(e.Arguments)); sig != nil {
		return sig.ReturnType
	}
	
	return ""
}

// inferStaticAccessType 推断静态访问的类型
func (c *Compiler) inferStaticAccessType(e *ast.StaticAccess) string {
	var className string
	switch cls := e.Class.(type) {
	case *ast.Identifier:
		className = cls.Name
	case *ast.SelfExpr:
		className = c.currentClassName
	case *ast.ParentExpr:
		if c.currentClassName != "" {
			if parent, ok := c.symbolTable.ClassParents[c.currentClassName]; ok {
				className = parent
			}
		}
	default:
		return ""
	}
	
	if className == "" {
		return ""
	}
	
	switch member := e.Member.(type) {
	case *ast.Variable:
		// 静态属性
		if sig := c.symbolTable.GetProperty(className, member.Name); sig != nil {
			return sig.Type
		}
	case *ast.Identifier:
		// 类常量
		return "any" // 常量类型可能需要额外推断
	case *ast.CallExpr:
		// 静态方法调用
		if fn, ok := member.Function.(*ast.Identifier); ok {
			if sig := c.symbolTable.GetMethod(className, fn.Name, len(member.Arguments)); sig != nil {
				return sig.ReturnType
			}
		}
	}
	
	return ""
}

// inferPropertyAccessType 推断属性访问的类型
func (c *Compiler) inferPropertyAccessType(e *ast.PropertyAccess) string {
	objType := c.inferExprType(e.Object)
	if objType == "" {
		return ""
	}
	
	// 特殊属性
	if e.Property.Name == "length" {
		return "int"
	}
	
	// 从符号表获取属性类型
	if sig := c.symbolTable.GetProperty(objType, e.Property.Name); sig != nil {
		return sig.Type
	}
	
	return ""
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
		// + 运算符：两边都是数字，或者一边是字符串（字符串拼接会自动转换）
		if leftType == "string" || rightType == "string" {
			return // 字符串拼接是合法的（VM 会自动转换非字符串类型）
		}
		if isNumeric(leftType) && isNumeric(rightType) {
			return // 数字相加是合法的
		}
		// 其他组合都是错误的
		c.error(op.Pos, i18n.T(i18n.ErrInvalidBinaryOp, "+", leftType, rightType))

	case token.MINUS, token.STAR, token.SLASH, token.PERCENT:
		// 算术运算符：两边必须都是数字
		if !isNumeric(leftType) || !isNumeric(rightType) {
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
func (c *Compiler) checkReturnType(pos token.Position, expr ast.Expression, expectedType ast.TypeNode) {
	if expectedType == nil {
		return
	}
	
	actualType := c.inferExprType(expr)
	if actualType == "unknown" {
		return // 无法推断类型时跳过检查
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
func (c *Compiler) getTypeName(t ast.TypeNode) string {
	switch typ := t.(type) {
	case *ast.SimpleType:
		return typ.Name
	case *ast.ClassType:
		return typ.Name.Literal
	case *ast.ArrayType:
		return "array"
	case *ast.MapType:
		return "map"
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
func (c *Compiler) isTypeCompatible(actual, expected string) bool {
	// 空类型表示无法推断，暂时跳过检查
	if actual == "" || expected == "" {
		return true
	}
	
	if actual == expected {
		return true
	}
	
	// any/mixed 类型接受任何值
	if expected == "any" || expected == "mixed" {
		return true
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
	
	// 通用数组类型
	if expected == "array" && strings.HasSuffix(actual, "[]") {
		return true
	}
	// 反向：具体数组类型接受通用 array（运行时可能出错，但编译期允许）
	if actual == "array" && strings.HasSuffix(expected, "[]") {
		return true
	}
	
	// Map 类型兼容性
	if expected == "map" && strings.HasPrefix(actual, "map[") {
		return true
	}
	
	// 对象类型兼容性（子类可以赋给父类）
	if c.isSubclassOf(actual, expected) {
		return true
	}
	
	// object 类型可以接受任何对象
	if expected == "object" {
		if _, exists := c.symbolTable.ClassMethods[actual]; exists {
			return true
		}
	}
	
	return false
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

