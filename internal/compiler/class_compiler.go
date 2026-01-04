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

	// 处理父类
	if decl.Extends != nil {
		class.ParentName = decl.Extends.Name
	}

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
		
		if prop.Static {
			class.StaticVars[prop.Name.Name] = value
		} else {
			class.Properties[prop.Name.Name] = value
		}
	}

	// 编译方法
	for _, method := range decl.Methods {
		m := c.compileMethod(class, method)
		class.Methods[method.Name.Name] = m
	}

	return class
}

// compileMethod 编译方法
func (c *Compiler) compileMethod(class *bytecode.Class, decl *ast.MethodDecl) *bytecode.Method {
	method := &bytecode.Method{
		Name:     decl.Name.Name,
		Arity:    len(decl.Parameters),
		IsStatic: decl.Static,
		Chunk:    bytecode.NewChunk(),
	}

	// 如果是抽象方法，不编译方法体
	if decl.Abstract || decl.Body == nil {
		return method
	}

	// 保存当前状态
	prevFn := c.function
	prevLocals := c.locals
	prevLocalCount := c.localCount
	prevScopeDepth := c.scopeDepth

	// 创建方法的编译环境
	c.function = &bytecode.Function{
		Name:  decl.Name.Name,
		Arity: len(decl.Parameters),
		Chunk: method.Chunk,
	}
	c.locals = make([]Local, 256)
	c.localCount = 0
	c.scopeDepth = 0

	// 添加隐式 this 参数 (非静态方法)
	if !decl.Static {
		c.addLocal("this")
	}

	// 添加参数作为局部变量 (直接使用 addLocal，因为方法参数始终是局部的)
	for _, param := range decl.Parameters {
		c.addLocal(param.Name.Name)
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

	return method
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
	// 接口在 Nova 中作为特殊的类处理
	class := bytecode.NewClass(decl.Name.Name)
	
	// 接口的方法都是抽象的
	for _, method := range decl.Methods {
		m := &bytecode.Method{
			Name:     method.Name.Name,
			Arity:    len(method.Parameters),
			IsStatic: false,
		}
		class.Methods[method.Name.Name] = m
	}

	return class
}

