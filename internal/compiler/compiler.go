package compiler

import (
	"fmt"

	"github.com/tangzhangming/nova/internal/ast"
	"github.com/tangzhangming/nova/internal/bytecode"
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
	}
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
	// 预留 slot 0 给调用者（与 CompileFunction 保持一致）
	c.addLocal("")

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
	
	// 生成参数类型检查代码
	for i, param := range params {
		if param.Type != nil {
			typeName := c.getTypeName(param.Type)
			if typeName != "" && typeName != "unknown" && typeName != "mixed" && typeName != "any" {
				// 加载参数值
				c.emitU16(bytecode.OpLoadLocal, uint16(i+1)) // +1 因为 slot 0 是调用者
				// 发出类型检查指令
				typeIdx := c.makeConstant(bytecode.NewString(typeName))
				c.emitU16(bytecode.OpCheckType, typeIdx)
				// 弹出检查后的值（类型检查不消耗值）
				c.emit(bytecode.OpPop)
			}
		}
	}

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
	
	// 生成参数类型检查代码
	for i, param := range params {
		if param.Type != nil {
			typeName := c.getTypeName(param.Type)
			if typeName != "" && typeName != "unknown" && typeName != "mixed" && typeName != "any" {
				// 加载参数值
				c.emitU16(bytecode.OpLoadLocal, uint16(i+1)) // +1 因为 slot 0 是调用者
				// 发出类型检查指令
				typeIdx := c.makeConstant(bytecode.NewString(typeName))
				c.emitU16(bytecode.OpCheckType, typeIdx)
				// 弹出检查后的值
				c.emit(bytecode.OpPop)
			}
		}
	}
	
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
		c.error(stmt.Pos(), "unsupported statement type")
	}
}

func (c *Compiler) compileVarDecl(s *ast.VarDeclStmt) {
	// 获取声明的类型
	var declaredType string
	if s.Type != nil {
		declaredType = c.getTypeName(s.Type)
	}
	
	// 类型检查：如果有显式类型和初始值，检查类型匹配
	if s.Type != nil && s.Value != nil {
		actualType := c.inferExprType(s.Value)
		if actualType != "unknown" && declaredType != "unknown" {
			if !c.isTypeCompatible(actualType, declaredType) {
				c.error(s.Value.Pos(), "cannot assign %s to variable of type %s", actualType, declaredType)
			}
		}
	}
	
	// 检查是否是定长数组类型
	if arrType, ok := s.Type.(*ast.ArrayType); ok && arrType.Size != nil {
		// 获取数组大小（必须是常量整数）
		capacity := c.evalConstInt(arrType.Size)
		if capacity < 0 {
			c.error(arrType.Size.Pos(), "array size must be a non-negative constant")
			return
		}
		
		if s.Value != nil {
			// 有初始值
			if arr, ok := s.Value.(*ast.ArrayLiteral); ok {
				// 数组字面量初始化
				if len(arr.Elements) > capacity {
					c.error(arr.Pos(), "too many elements in array initializer (max %d, got %d)", capacity, len(arr.Elements))
					return
				}
				for _, elem := range arr.Elements {
					c.compileExpr(elem)
				}
				// 创建定长数组
				c.emitU16(bytecode.OpNewFixedArray, uint16(capacity))
				c.currentChunk().WriteU16(uint16(len(arr.Elements)), 0)
			} else {
				// 非数组字面量初始化，创建空定长数组
				c.emitU16(bytecode.OpNewFixedArray, uint16(capacity))
				c.currentChunk().WriteU16(0, 0)
			}
		} else {
			// 无初始值，创建空定长数组
			c.emitU16(bytecode.OpNewFixedArray, uint16(capacity))
			c.currentChunk().WriteU16(0, 0)
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
		c.error(token.Position{}, "'break' outside of loop")
		return
	}
	jump := c.emitJump(bytecode.OpJump)
	c.breakJumps = append(c.breakJumps, jump)
}

func (c *Compiler) compileContinueStmt() {
	if c.loopDepth == 0 {
		c.error(token.Position{}, "'continue' outside of loop")
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
				c.error(s.Pos(), "function declared without return type but returns %d value(s)", actualReturns)
			}
			c.emit(bytecode.OpReturnNull)
			return
		}
		
		if c.expectedReturns > 0 && actualReturns != c.expectedReturns {
			c.error(s.Pos(), "function expects %d return value(s) but got %d", c.expectedReturns, actualReturns)
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
	hasCatch := len(s.Catches) > 0
	
	// 发出进入 try 块指令
	c.emit(bytecode.OpEnterTry)
	enterTryPos := c.currentChunk().Len()
	c.currentChunk().WriteI16(0, 0) // catch 偏移量占位
	c.currentChunk().WriteI16(0, 0) // finally 偏移量占位
	
	// 编译 try 块
	c.compileStmt(s.Try)
	
	// 离开 try 块（正常流程）
	c.emit(bytecode.OpLeaveTry)
	
	// 如果有 finally，正常流程需要跳转到 finally
	var normalToFinallyJump int
	var afterCatchJump int
	
	if hasFinally {
		normalToFinallyJump = c.emitJump(bytecode.OpJump)
	} else {
		// 没有 finally，跳过 catch 块
		afterCatchJump = c.emitJump(bytecode.OpJump)
	}
	
	// catch 块开始位置
	catchStart := c.currentChunk().Len()
	
	// 修补 catch 偏移量
	catchOffset := catchStart - enterTryPos
	c.currentChunk().Code[enterTryPos] = byte(int16(catchOffset) >> 8)
	c.currentChunk().Code[enterTryPos+1] = byte(int16(catchOffset))
	
	// 编译 catch 块
	if hasCatch {
		c.emit(bytecode.OpEnterCatch)
		
		for _, catch := range s.Catches {
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
		}
		
		// catch 执行完后，如果有 finally，跳转到 finally
		if hasFinally {
			// catch 之后也跳转到 finally
		} else {
			// 没有 finally，catch 结束后直接继续
		}
	} else {
		// 没有 catch 块，异常会直接传播到 finally
		// 但需要一个占位，因为 handleException 会跳转到这里
		c.emit(bytecode.OpEnterCatch)
		// 重新抛出异常（因为没有 catch 处理）
		if hasFinally {
			// 异常会在 finally 后重新抛出
		} else {
			c.emit(bytecode.OpRethrow)
		}
	}
	
	// finally 块
	if hasFinally {
		// 修补跳转到 finally
		c.patchJump(normalToFinallyJump)
		
		// finally 开始位置
		finallyStart := c.currentChunk().Len()
		
		// 修补 finally 偏移量
		finallyOffset := finallyStart - enterTryPos
		c.currentChunk().Code[enterTryPos+2] = byte(int16(finallyOffset) >> 8)
		c.currentChunk().Code[enterTryPos+3] = byte(int16(finallyOffset))
		
		c.emit(bytecode.OpEnterFinally)
		
		// 编译 finally 块
		c.compileStmt(s.Finally.Body)
		
		c.emit(bytecode.OpLeaveFinally)
	} else {
		// 没有 finally，修补 after-catch 跳转
		c.patchJump(afterCatchJump)
		
		// finally 偏移量设为 -1（没有 finally）
		c.currentChunk().Code[enterTryPos+2] = 0xFF
		c.currentChunk().Code[enterTryPos+3] = 0xFF
	}
}

// ============================================================================
// 表达式编译
// ============================================================================

func (c *Compiler) compileExpr(expr ast.Expression) {
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

	default:
		c.error(expr.Pos(), "unsupported expression type")
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
		c.error(v.Pos(), "undefined variable '$"+v.Name+"' (use 'use' to capture external variables in closures)")
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
	// 编译时类型检查
	if v, ok := e.Left.(*ast.Variable); ok {
		varType := c.getVariableType(v.Name)
		if varType != "" && varType != "unknown" {
			rightType := c.inferExprType(e.Right)
			if rightType != "unknown" && !c.isTypeCompatible(rightType, varType) {
				c.error(e.Right.Pos(), "cannot assign %s to variable of type %s", rightType, varType)
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
			c.error(e.Pos(), "compound assignment to array element not yet supported")
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
			c.currentChunk().WriteU16(nameIdx, 0)
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
	
	c.compileExpr(e.Function)
	for _, arg := range e.Arguments {
		c.compileExpr(arg)
	}
	c.emitByte(bytecode.OpCall, byte(len(e.Arguments)))
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

	c.compileExpr(e.Object)
	for _, arg := range e.Arguments {
		c.compileExpr(arg)
	}
	idx := c.makeConstant(bytecode.NewString(e.Method.Name))
	c.emitU16(bytecode.OpCallMethod, idx)
	c.currentChunk().WriteU8(byte(len(e.Arguments)), 0) // 参数数量
}

func (c *Compiler) compileStaticAccess(e *ast.StaticAccess) {
	// 获取类名
	var className string
	switch cls := e.Class.(type) {
	case *ast.Identifier:
		className = cls.Name
	case *ast.SelfExpr:
		className = "self" // 特殊处理
	case *ast.ParentExpr:
		className = "parent" // 特殊处理
	default:
		c.error(e.Pos(), "invalid static access")
		return
	}
	
	classIdx := c.makeConstant(bytecode.NewString(className))
	
	// 处理成员访问
	switch member := e.Member.(type) {
	case *ast.Variable:
		// 静态属性访问: Class::$prop
		nameIdx := c.makeConstant(bytecode.NewString(member.Name))
		c.emitU16(bytecode.OpGetStatic, classIdx)
		c.currentChunk().WriteU16(nameIdx, 0)
		
	case *ast.Identifier:
		// 类常量访问: Class::CONST
		nameIdx := c.makeConstant(bytecode.NewString(member.Name))
		c.emitU16(bytecode.OpGetStatic, classIdx)
		c.currentChunk().WriteU16(nameIdx, 0)
		
	case *ast.CallExpr:
		// 静态方法调用: Class::method()
		if fn, ok := member.Function.(*ast.Identifier); ok {
			nameIdx := c.makeConstant(bytecode.NewString(fn.Name))
			// 编译参数
			for _, arg := range member.Arguments {
				c.compileExpr(arg)
			}
			c.emitU16(bytecode.OpCallStatic, classIdx)
			c.currentChunk().WriteU16(nameIdx, 0)
			c.currentChunk().WriteU8(byte(len(member.Arguments)), 0)
		}
	default:
		c.error(e.Pos(), "invalid static member")
	}
}

func (c *Compiler) compileNewExpr(e *ast.NewExpr) {
	idx := c.makeConstant(bytecode.NewString(e.ClassName.Name))
	c.emitU16(bytecode.OpNewObject, idx)

	// 调用构造函数
	for _, arg := range e.Arguments {
		c.compileExpr(arg)
	}
	constructorIdx := c.makeConstant(bytecode.NewString("__construct"))
	c.emitU16(bytecode.OpCallMethod, constructorIdx)
	c.currentChunk().WriteU8(byte(len(e.Arguments)), 0) // 参数数量
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
			c.error(token.Position{}, "variable already declared in this scope")
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
		c.error(token.Position{}, "too many local variables")
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
	c.currentChunk().WriteOp(op, 0) // TODO: 行号
}

func (c *Compiler) emitByte(op bytecode.OpCode, b byte) {
	c.emit(op)
	c.currentChunk().WriteU8(b, 0)
}

func (c *Compiler) emitU16(op bytecode.OpCode, v uint16) {
	c.emit(op)
	c.currentChunk().WriteU16(v, 0)
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
	c.currentChunk().WriteU16(0xFFFF, 0) // 占位
	return c.currentChunk().Len() - 2
}

func (c *Compiler) patchJump(offset int) {
	c.currentChunk().PatchJump(offset)
}

func (c *Compiler) emitLoop(loopStart int) {
	c.emit(bytecode.OpLoop)
	offset := c.currentChunk().Len() - loopStart + 2
	c.currentChunk().WriteU16(uint16(offset), 0)
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
		return "array"
	case *ast.MapLiteral:
		return "map"
	case *ast.BinaryExpr:
		leftType := c.inferExprType(e.Left)
		rightType := c.inferExprType(e.Right)
		
		// 如果任一侧是 unknown，整个表达式也是 unknown
		if leftType == "unknown" || rightType == "unknown" {
			return "unknown"
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
		return "unknown"
	case *ast.CallExpr, *ast.MethodCall, *ast.StaticAccess:
		// 函数调用的返回类型需要查表，暂时返回 unknown
		return "unknown"
	case *ast.NewExpr:
		return e.ClassName.Name
	case *ast.TernaryExpr:
		return c.inferExprType(e.Then)
	default:
		return "unknown"
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
		c.error(pos, "type mismatch: expected %s but got %s", expectedTypeName, actualType)
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
	default:
		return "unknown"
	}
}

// isTypeCompatible 检查类型兼容性
func (c *Compiler) isTypeCompatible(actual, expected string) bool {
	if actual == expected {
		return true
	}
	// null 可以赋值给任何可空类型
	if actual == "null" {
		return true
	}
	// int 可以隐式转换为 float
	if actual == "int" && expected == "float" {
		return true
	}
	// 整数类型兼容
	intTypes := map[string]bool{"int": true, "i8": true, "i16": true, "i32": true, "i64": true}
	if intTypes[actual] && intTypes[expected] {
		return true
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
	c.error(expr.Pos(), "array size must be a compile-time constant")
	return -1
}

