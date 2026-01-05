package compiler

import (
	"github.com/tangzhangming/nova/internal/ast"
	"github.com/tangzhangming/nova/internal/bytecode"
	"github.com/tangzhangming/nova/internal/token"
)

// ClassCompiler 类编译器
type ClassCompiler struct {
	enclosing   *ClassCompiler
	class       *bytecode.Class
	hasSuperclass bool
}

// CompileClass 编译类声明
func (c *Compiler) CompileClass(decl *ast.ClassDecl) *bytecode.Class {
	class := bytecode.NewClass(decl.Name.Name)
	
	// 设置命名空间
	class.Namespace = c.currentNamespace

	// 处理类注解
	class.Annotations = c.compileAnnotations(decl.Annotations)

	// 处理父类
	if decl.Extends != nil {
		class.ParentName = decl.Extends.Name
	}

	// 处理接口
	for _, iface := range decl.Implements {
		class.Implements = append(class.Implements, iface.Name)
	}
	
	// 抽象类标记
	class.IsAbstract = decl.Abstract

	// 编译常量
	for _, constDecl := range decl.Constants {
		value := c.evaluateConstant(constDecl.Value)
		class.Constants[constDecl.Name.Name] = value
	}

	// 编译属性
	for _, prop := range decl.Properties {
		var value bytecode.Value
		if prop.Value != nil {
			value = c.evaluateConstant(prop.Value)
		} else {
			value = bytecode.NullValue
		}
		
		// 保存属性可见性
		vis := toByteVisibility(prop.Visibility)
		
		if prop.Static {
			class.StaticVars[prop.Name.Name] = value
		} else {
			class.Properties[prop.Name.Name] = value
			class.PropVisibility[prop.Name.Name] = vis
		}
		
		// 保存属性注解
		if len(prop.Annotations) > 0 {
			class.PropAnnotations[prop.Name.Name] = c.compileAnnotations(prop.Annotations)
		}
	}

	// 编译方法
	for _, method := range decl.Methods {
		m := c.compileMethod(class, method)
		class.AddMethod(m)
	}

	return class
}

// compileAnnotations 编译注解列表
func (c *Compiler) compileAnnotations(annotations []*ast.Annotation) []*bytecode.Annotation {
	if len(annotations) == 0 {
		return nil
	}
	result := make([]*bytecode.Annotation, len(annotations))
	for i, ann := range annotations {
		result[i] = &bytecode.Annotation{
			Name: ann.Name.Name,
			Args: c.evaluateAnnotationArgs(ann.Args),
		}
	}
	return result
}

// evaluateAnnotationArgs 计算注解参数
func (c *Compiler) evaluateAnnotationArgs(args []ast.Expression) []bytecode.Value {
	if len(args) == 0 {
		return nil
	}
	result := make([]bytecode.Value, len(args))
	for i, arg := range args {
		result[i] = c.evaluateConstant(arg)
	}
	return result
}

// toByteVisibility 转换 AST 可见性到字节码可见性
func toByteVisibility(v ast.Visibility) bytecode.Visibility {
	switch v {
	case ast.VisibilityPublic:
		return bytecode.VisPublic
	case ast.VisibilityProtected:
		return bytecode.VisProtected
	case ast.VisibilityPrivate:
		return bytecode.VisPrivate
	default:
		return bytecode.VisPublic
	}
}

// compileMethod 编译方法
func (c *Compiler) compileMethod(class *bytecode.Class, decl *ast.MethodDecl) *bytecode.Method {
	// 更新当前行号
	c.currentLine = decl.Pos().Line
	
	// 计算最小参数数量（考虑默认参数）
	minArity := len(decl.Parameters)
	for i, param := range decl.Parameters {
		if param.Default != nil && minArity == len(decl.Parameters) {
			minArity = i
		}
	}

	method := &bytecode.Method{
		Name:        decl.Name.Name,
		ClassName:   class.Name, // 设置所属类名
		SourceFile:  c.sourceFile, // 继承源文件信息
		Arity:       len(decl.Parameters),
		MinArity:    minArity,
		IsStatic:    decl.Static,
		Visibility:  toByteVisibility(decl.Visibility),
		Annotations: c.compileAnnotations(decl.Annotations),
		Chunk:       bytecode.NewChunk(),
	}

	// 如果是抽象方法，不编译方法体
	if decl.Abstract || decl.Body == nil {
		return method
	}

	// 收集默认参数值
	var defaultValues []bytecode.Value
	for _, param := range decl.Parameters {
		if param.Default != nil {
			defaultVal := c.evaluateConstExpr(param.Default)
			defaultValues = append(defaultValues, defaultVal)
		}
	}
	method.DefaultValues = defaultValues

	// 保存当前状态
	prevFn := c.function
	prevLocals := c.locals
	prevLocalCount := c.localCount
	prevScopeDepth := c.scopeDepth
	prevReturnType := c.returnType
	prevExpectedReturns := c.expectedReturns

	// 创建方法的编译环境
	c.function = &bytecode.Function{
		Name:       decl.Name.Name,
		ClassName:  class.Name, // 设置所属类名
		Arity:      len(decl.Parameters),
		Chunk:      method.Chunk,
		SourceFile: c.sourceFile, // 继承源文件信息
	}
	c.locals = make([]Local, 256)
	c.localCount = 0
	c.scopeDepth = 0
	
	// 设置返回类型检查
	c.returnType = decl.ReturnType
	c.expectedReturns = c.countExpectedReturns(decl.ReturnType)

	// 添加隐式 this 参数 (slot 0)
	// 非静态方法有 $this，静态方法用空字符串占位
	if !decl.Static {
		c.addLocal("this")
	} else {
		c.addLocal("") // 静态方法 slot 0 占位符
	}

	// 添加参数作为局部变量 (直接使用 addLocal，因为方法参数始终是局部的)
	for _, param := range decl.Parameters {
		typeName := ""
		if param.Type != nil {
			typeName = c.getTypeName(param.Type)
		}
		c.addLocalWithType(param.Name.Name, typeName)
	}
	
	// 生成参数类型检查代码
	for i, param := range decl.Parameters {
		if param.Type != nil {
			typeName := c.getTypeName(param.Type)
			if typeName != "" && typeName != "unknown" && typeName != "mixed" && typeName != "any" {
				// 加载参数值
				c.emitU16(bytecode.OpLoadLocal, uint16(i+1)) // +1 因为 slot 0 是 this 或占位符
				// 发出类型检查指令
				typeIdx := c.makeConstant(bytecode.NewString(typeName))
				c.emitU16(bytecode.OpCheckType, typeIdx)
				// 弹出检查后的值（类型检查不消耗值）
				c.emit(bytecode.OpPop)
			}
		}
	}

	// 编译方法体
	c.beginScope()
	for _, stmt := range decl.Body.Statements {
		c.compileStmt(stmt)
	}
	c.endScope()

	// 添加默认返回
	if decl.Name.Name == "__construct" {
		// 构造函数返回 this
		c.emitU16(bytecode.OpLoadLocal, 0) // 加载 this
		c.emit(bytecode.OpReturn)
	} else {
		c.emit(bytecode.OpReturnNull)
	}

	method.LocalCount = c.localCount
	method.Chunk = c.function.Chunk

	// 恢复状态
	c.function = prevFn
	c.locals = prevLocals
	c.localCount = prevLocalCount
	c.scopeDepth = prevScopeDepth
	c.returnType = prevReturnType
	c.expectedReturns = prevExpectedReturns

	return method
}

// countExpectedReturns 计算预期返回值数量
func (c *Compiler) countExpectedReturns(returnType ast.TypeNode) int {
	if returnType == nil {
		return 0 // 无返回类型 = void
	}
	
	// 检查是否是 void 类型
	if simple, ok := returnType.(*ast.SimpleType); ok {
		if simple.Name == "void" {
			return 0
		}
	}
	
	// 检查是否是多返回值类型 (TupleType)
	if tuple, ok := returnType.(*ast.TupleType); ok {
		return len(tuple.Types)
	}
	
	// 单返回值
	return 1
}

// evaluateConstant 编译时求值常量表达式
func (c *Compiler) evaluateConstant(expr ast.Expression) bytecode.Value {
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
	case *ast.UnaryExpr:
		if e.Operator.Type == token.MINUS {
			inner := c.evaluateConstant(e.Operand)
			if inner.Type == bytecode.ValInt {
				return bytecode.NewInt(-inner.AsInt())
			}
			if inner.Type == bytecode.ValFloat {
				return bytecode.NewFloat(-inner.AsFloat())
			}
		}
	case *ast.BinaryExpr:
		left := c.evaluateConstant(e.Left)
		right := c.evaluateConstant(e.Right)
		return c.evalBinaryConstant(e.Operator.Type, left, right)
	}
	return bytecode.NullValue
}

func (c *Compiler) evalBinaryConstant(op token.TokenType, left, right bytecode.Value) bytecode.Value {
	// 字符串拼接
	if op == token.PLUS && (left.Type == bytecode.ValString || right.Type == bytecode.ValString) {
		return bytecode.NewString(left.AsString() + right.AsString())
	}

	// 整数运算
	if left.Type == bytecode.ValInt && right.Type == bytecode.ValInt {
		l, r := left.AsInt(), right.AsInt()
		switch op {
		case token.PLUS:
			return bytecode.NewInt(l + r)
		case token.MINUS:
			return bytecode.NewInt(l - r)
		case token.STAR:
			return bytecode.NewInt(l * r)
		case token.SLASH:
			if r != 0 {
				return bytecode.NewInt(l / r)
			}
		case token.PERCENT:
			if r != 0 {
				return bytecode.NewInt(l % r)
			}
		}
	}

	// 浮点运算
	if (left.Type == bytecode.ValInt || left.Type == bytecode.ValFloat) &&
		(right.Type == bytecode.ValInt || right.Type == bytecode.ValFloat) {
		l, r := left.AsFloat(), right.AsFloat()
		switch op {
		case token.PLUS:
			return bytecode.NewFloat(l + r)
		case token.MINUS:
			return bytecode.NewFloat(l - r)
		case token.STAR:
			return bytecode.NewFloat(l * r)
		case token.SLASH:
			if r != 0 {
				return bytecode.NewFloat(l / r)
			}
		}
	}

	return bytecode.NullValue
}

// CompileInterface 编译接口声明
func (c *Compiler) CompileInterface(decl *ast.InterfaceDecl) *bytecode.Class {
	// 接口在 Sola 中作为特殊的类处理
	class := bytecode.NewClass(decl.Name.Name)
	
	// 接口的方法都是抽象的
	for _, method := range decl.Methods {
		m := &bytecode.Method{
			Name:     method.Name.Name,
			Arity:    len(method.Parameters),
			IsStatic: false,
		}
		class.AddMethod(m)
	}

	return class
}

// CompileEnum 编译枚举声明
func (c *Compiler) CompileEnum(decl *ast.EnumDecl) *bytecode.Enum {
	enum := bytecode.NewEnum(decl.Name.Name)
	
	// 编译每个枚举成员
	for i, enumCase := range decl.Cases {
		var value bytecode.Value
		if enumCase.Value != nil {
			// 有显式值
			value = c.evaluateConstant(enumCase.Value)
		} else {
			// 默认值为索引
			value = bytecode.NewInt(int64(i))
		}
		enum.Cases[enumCase.Name.Name] = value
	}
	
	return enum
}

