package compiler

import (
	"strings"

	"github.com/tangzhangming/nova/internal/ast"
	"github.com/tangzhangming/nova/internal/bytecode"
	"github.com/tangzhangming/nova/internal/i18n"
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
	
	// 保存并设置当前类名（用于类型推导）
	prevClassName := c.currentClassName
	// 如果有命名空间，添加命名空间前缀
	if c.currentNamespace != "" {
		c.currentClassName = c.currentNamespace + "\\" + decl.Name.Name
	} else {
		c.currentClassName = decl.Name.Name
	}
	defer func() { c.currentClassName = prevClassName }()
	
	// 设置命名空间
	class.Namespace = c.currentNamespace
	
	// 处理泛型类型参数（包括类型参数和 where 子句）
	var allTypeParams []*ast.TypeParameter
	allTypeParams = append(allTypeParams, decl.TypeParams...)
	allTypeParams = append(allTypeParams, decl.WhereClause...)
	
	if len(allTypeParams) > 0 {
		class.TypeParams = make([]*bytecode.TypeParamDef, len(allTypeParams))
		for i, tp := range allTypeParams {
			constraint := ""
			if tp.Constraint != nil {
				constraint = c.getTypeName(tp.Constraint)
			}
			var implementsTypes []string
			for _, implType := range tp.ImplementsTypes {
				implementsTypes = append(implementsTypes, c.getTypeName(implType))
			}
			class.TypeParams[i] = &bytecode.TypeParamDef{
				Name:            tp.Name.Name,
				Constraint:      constraint,
				ImplementsTypes: implementsTypes,
			}
		}
	}

	// 处理类注解
	class.Annotations = c.compileAnnotations(decl.Annotations)

	// 处理父类
	if decl.Extends != nil {
		class.ParentName = decl.Extends.Name
	}

	// 处理接口 - 支持泛型接口（类型擦除，只保存基础接口名）
	for _, iface := range decl.Implements {
		fullName := c.getTypeName(iface)
		baseName := c.extractBaseTypeName(fullName)
		class.Implements = append(class.Implements, baseName)
	}
	
	// 抽象类标记
	class.IsAbstract = decl.Abstract
	
	// final 类标记
	class.IsFinal = decl.Final
	
	// final 和 abstract 不能同时存在
	if decl.Abstract && decl.Final {
		c.error(decl.ClassToken.Pos, i18n.T(i18n.ErrFinalAndAbstractConflict))
	}

	// 编译常量
	for _, constDecl := range decl.Constants {
		value := c.evaluateConstant(constDecl.Value)
		class.Constants[constDecl.Name.Name] = value
	}

	// 编译属性
	for _, prop := range decl.Properties {
		// 处理有访问器的属性（自动属性、完整属性、表达式体属性）
		if prop.Accessor != nil {
			// 自动属性或完整属性
			c.compilePropertyWithAccessor(class, prop)
		} else if prop.ExprBody != nil {
			// 表达式体只读属性
			c.compileExpressionBodiedProperty(class, prop)
		} else {
			// 普通字段
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
			
			// 保存属性 final 标记
			if prop.Final {
				class.PropFinal[prop.Name.Name] = true
			}
			
			// 保存属性注解
			if len(prop.Annotations) > 0 {
				class.PropAnnotations[prop.Name.Name] = c.compileAnnotations(prop.Annotations)
			}
		}
	}

	// 编译方法
	for _, method := range decl.Methods {
		m := c.compileMethod(class, method)
		class.AddMethod(m)
	}

	// 验证接口实现
	if len(decl.Implements) > 0 {
		c.validateInterfaceImplementations(decl)
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
		IsFinal:     decl.Final,
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
	prevCurrentClassName := c.currentClassName

	// 创建新函数
	c.function = bytecode.NewFunction(decl.Name.Name)
	c.function.Arity = len(decl.Parameters)
	c.function.SourceFile = c.sourceFile
	c.locals = make([]Local, 256)
	c.localCount = 0
	c.scopeDepth = 0
	
	// 设置返回类型检查
	c.returnType = decl.ReturnType
	c.expectedReturns = c.countExpectedReturns(decl.ReturnType)
	
	// 计算最小参数数量（考虑默认参数和可变参数）
	minArity = len(decl.Parameters)
	isVariadic := false
	for i, param := range decl.Parameters {
		if param.Variadic {
			isVariadic = true
			minArity = i
			break
		}
		if param.Default != nil && minArity == len(decl.Parameters) {
			minArity = i
		}
	}
	c.function.MinArity = minArity
	c.function.IsVariadic = isVariadic

	// 预留 slot 0 给 $this
	c.addLocal("")
	
	// 添加参数作为局部变量
	for _, param := range decl.Parameters {
		typeName := ""
		if param.Type != nil {
			typeName = c.getTypeName(param.Type)
		}
		c.addLocalWithType(param.Name.Name, typeName)
	}

	// 编译方法体
	c.beginScope()
	for _, stmt := range decl.Body.Statements {
		c.compileStmt(stmt)
	}
	c.endScope()

	// 添加默认返回
	c.emit(bytecode.OpReturnNull)

	method.LocalCount = c.localCount
	method.Chunk = c.function.Chunk
	
	// 恢复状态
	c.function = prevFn
	c.locals = prevLocals
	c.localCount = prevLocalCount
	c.scopeDepth = prevScopeDepth
	c.returnType = prevReturnType
	c.expectedReturns = prevExpectedReturns
	c.currentClassName = prevCurrentClassName
	
	return method
}

// compilePropertyWithAccessor 编译带访问器的属性（自动属性或完整属性）
func (c *Compiler) compilePropertyWithAccessor(class *bytecode.Class, prop *ast.PropertyDecl) {
	// 保存属性可见性
	vis := toByteVisibility(prop.Visibility)
	
	// 如果有 getter 访问器
	if prop.Accessor.Getter != nil {
		// 创建 getter 方法
		getterName := "get_" + prop.Name.Name
		getter := &bytecode.Method{
			Name:        getterName,
			ClassName:   class.Name,
			SourceFile:  c.sourceFile,
			Arity:       0,
			MinArity:    0,
			IsStatic:    prop.Static,
			Visibility:  vis,
			Chunk:       bytecode.NewChunk(),
		}
		
		// 编译 getter 方法体
		prevFn := c.function
		prevLocals := c.locals
		prevLocalCount := c.localCount
		prevScopeDepth := c.scopeDepth
		prevReturnType := c.returnType
		prevExpectedReturns := c.expectedReturns
		prevCurrentClassName := c.currentClassName
		
		c.function = bytecode.NewFunction(getterName)
		c.function.SourceFile = c.sourceFile
		c.locals = make([]Local, 256)
		c.localCount = 0
		c.scopeDepth = 0
		
		// 设置返回类型
		if prop.Type != nil {
			c.returnType = prop.Type
			c.expectedReturns = c.countExpectedReturns(prop.Type)
		} else {
			c.returnType = nil
			c.expectedReturns = 0
		}
		
		// 预留 slot 0 给 $this
		c.addLocal("")
		
		// 编译 getter 体
		c.beginScope()
		for _, stmt := range prop.Accessor.Getter.Statements {
			c.compileStmt(stmt)
		}
		c.endScope()
		
		// 如果没有显式返回，添加默认返回
		c.emit(bytecode.OpReturnNull)
		
		getter.LocalCount = c.localCount
		getter.Chunk = c.function.Chunk
		
		// 恢复状态
		c.function = prevFn
		c.locals = prevLocals
		c.localCount = prevLocalCount
		c.scopeDepth = prevScopeDepth
		c.returnType = prevReturnType
		c.expectedReturns = prevExpectedReturns
		c.currentClassName = prevCurrentClassName
		
		class.AddMethod(getter)
	}
	
	// 如果有 setter 访问器
	if prop.Accessor.Setter != nil {
		// 创建 setter 方法
		setterName := "set_" + prop.Name.Name
		setter := &bytecode.Method{
			Name:        setterName,
			ClassName:   class.Name,
			SourceFile:  c.sourceFile,
			Arity:       1,
			MinArity:    1,
			IsStatic:    prop.Static,
			Visibility:  vis,
			Chunk:       bytecode.NewChunk(),
		}
		
		// 编译 setter 方法体
		prevFn := c.function
		prevLocals := c.locals
		prevLocalCount := c.localCount
		prevScopeDepth := c.scopeDepth
		prevReturnType := c.returnType
		prevExpectedReturns := c.expectedReturns
		prevCurrentClassName := c.currentClassName
		
		c.function = bytecode.NewFunction(setterName)
		c.function.SourceFile = c.sourceFile
		c.locals = make([]Local, 256)
		c.localCount = 0
		c.scopeDepth = 0
		c.returnType = nil
		c.expectedReturns = 0
		
		// 预留 slot 0 给 $this
		c.addLocal("")
		
		// 添加参数作为局部变量
		if len(prop.Accessor.Setter.Parameters) > 0 {
			param := prop.Accessor.Setter.Parameters[0]
			typeName := ""
			if prop.Type != nil {
				typeName = c.getTypeName(prop.Type)
			}
			c.addLocalWithType(param.Name.Name, typeName)
		}
		
		// 编译 setter 体
		c.beginScope()
		for _, stmt := range prop.Accessor.Setter.Body.Statements {
			c.compileStmt(stmt)
		}
		c.endScope()
		
		// 添加默认返回
		c.emit(bytecode.OpReturnNull)
		
		setter.LocalCount = c.localCount
		setter.Chunk = c.function.Chunk
		
		// 恢复状态
		c.function = prevFn
		c.locals = prevLocals
		c.localCount = prevLocalCount
		c.scopeDepth = prevScopeDepth
		c.returnType = prevReturnType
		c.expectedReturns = prevExpectedReturns
		c.currentClassName = prevCurrentClassName
		
		class.AddMethod(setter)
	}
}

// compileExpressionBodiedProperty 编译表达式体属性
func (c *Compiler) compileExpressionBodiedProperty(class *bytecode.Class, prop *ast.PropertyDecl) {
	// 保存属性可见性
	vis := toByteVisibility(prop.Visibility)
	
	// 创建 getter 方法
	getterName := "get_" + prop.Name.Name
	getter := &bytecode.Method{
		Name:        getterName,
		ClassName:   class.Name,
		SourceFile:  c.sourceFile,
		Arity:       0,
		MinArity:    0,
		IsStatic:    prop.Static,
		Visibility:  vis,
		Chunk:       bytecode.NewChunk(),
	}
	
	// 编译 getter 方法体
	prevFn := c.function
	prevLocals := c.locals
	prevLocalCount := c.localCount
	prevScopeDepth := c.scopeDepth
	prevReturnType := c.returnType
	prevExpectedReturns := c.expectedReturns
	prevCurrentClassName := c.currentClassName
	
	c.function = bytecode.NewFunction(getterName)
	c.function.SourceFile = c.sourceFile
	c.locals = make([]Local, 256)
	c.localCount = 0
	c.scopeDepth = 0
	
	// 设置返回类型
	if prop.Type != nil {
		c.returnType = prop.Type
		c.expectedReturns = c.countExpectedReturns(prop.Type)
	} else {
		c.returnType = nil
		c.expectedReturns = 0
	}
	
	// 预留 slot 0 给 $this
	c.addLocal("")
	
	// 编译表达式体
	c.beginScope()
	c.compileExpr(prop.ExprBody)
	c.emit(bytecode.OpReturn)
	c.endScope()
	
	getter.LocalCount = c.localCount
	getter.Chunk = c.function.Chunk
	
	// 恢复状态
	c.function = prevFn
	c.locals = prevLocals
	c.localCount = prevLocalCount
	c.scopeDepth = prevScopeDepth
	c.returnType = prevReturnType
	c.expectedReturns = prevExpectedReturns
	c.currentClassName = prevCurrentClassName
	
	class.AddMethod(getter)
}

// validateInterfaceImplementations 验证类是否实现了所有声明的接口
func (c *Compiler) validateInterfaceImplementations(decl *ast.ClassDecl) {
	className := c.currentClassName
	if className == "" {
		className = decl.Name.Name
	}
	
	for _, iface := range decl.Implements {
		fullName := c.getTypeName(iface)
		baseName := c.extractBaseTypeName(fullName)
		
		// 验证接口实现
		err := c.symbolTable.ValidateImplements(className, baseName)
		if err != nil {
			// 构造详细的错误信息
			c.error(iface.Pos(), i18n.T(i18n.ErrInterfaceNotImplemented, className, baseName)+": "+err.Error())
			continue
		}
		
		// 进一步验证：检查所有接口方法是否都有正确的签名
		interfaceMethods, ok := c.symbolTable.ClassMethods[baseName]
		if !ok {
			continue
		}
		
		classMethods, ok := c.symbolTable.ClassMethods[className]
		if !ok {
			c.error(iface.Pos(), i18n.T(i18n.ErrInterfaceNotImplemented, className, baseName))
			continue
		}
		
		// 对每个接口方法，验证类中有匹配的实现
		for methodName, interfaceMethodSigs := range interfaceMethods {
			classMethodSigs, hasMethod := classMethods[methodName]
			if !hasMethod || len(classMethodSigs) == 0 {
				c.error(iface.Pos(), i18n.T(i18n.ErrInterfaceMethodMissing, className, baseName, methodName))
				continue
			}
			
			// 检查是否有匹配的方法签名
			for _, interfaceSig := range interfaceMethodSigs {
				matchFound := false
				for _, classSig := range classMethodSigs {
					if c.symbolTable.compareMethodSignatures(interfaceSig, classSig) {
						matchFound = true
						break
					}
				}
				
				if !matchFound {
					// 构造详细的错误信息
					interfaceSig := interfaceMethodSigs[0]
					classSig := classMethodSigs[0]
					
					// 检查参数类型不匹配
					if len(interfaceSig.ParamTypes) != len(classSig.ParamTypes) ||
						!c.paramsMatch(interfaceSig.ParamTypes, classSig.ParamTypes) {
						c.error(iface.Pos(), i18n.T(i18n.ErrInterfaceMethodParamMismatch,
							className, methodName, baseName,
							c.formatParamTypes(interfaceSig.ParamTypes),
							c.formatParamTypes(classSig.ParamTypes)))
						continue
					}
					
					// 检查返回类型不匹配
					if interfaceSig.ReturnType != classSig.ReturnType &&
						!c.symbolTable.IsTypeCompatible(classSig.ReturnType, interfaceSig.ReturnType) {
						c.error(iface.Pos(), i18n.T(i18n.ErrInterfaceMethodReturnMismatch,
							className, methodName, baseName,
							interfaceSig.ReturnType, classSig.ReturnType))
						continue
					}
					
					// 检查静态/实例不匹配
					if interfaceSig.IsStatic != classSig.IsStatic {
						c.error(iface.Pos(), i18n.T(i18n.ErrInterfaceMethodStaticMismatch,
							className, methodName, baseName))
						continue
					}
				}
			}
		}
	}
}

// paramsMatch 检查参数类型是否匹配
func (c *Compiler) paramsMatch(interfaceParams, classParams []string) bool {
	if len(interfaceParams) != len(classParams) {
		return false
	}
	for i := 0; i < len(interfaceParams); i++ {
		// 接口参数类型应该是类参数类型的超类型（逆变）
		if !c.symbolTable.IsTypeCompatible(interfaceParams[i], classParams[i]) {
			// 对于基本类型，需要严格匹配
			if interfaceParams[i] != classParams[i] {
				return false
			}
		}
	}
	return true
}

// formatParamTypes 格式化参数类型列表为字符串
func (c *Compiler) formatParamTypes(paramTypes []string) string {
	if len(paramTypes) == 0 {
		return "()"
	}
	return "(" + strings.Join(paramTypes, ", ") + ")"
}
