package lsp

import (
	"encoding/json"
	"strings"

	"github.com/tangzhangming/nova/internal/ast"
	"github.com/tangzhangming/nova/internal/compiler"
	"go.lsp.dev/protocol"
)

// handleCompletion 处理代码补全请求
func (s *Server) handleCompletion(id json.RawMessage, params json.RawMessage) {
	var p protocol.CompletionParams
	if err := json.Unmarshal(params, &p); err != nil {
		s.sendError(id, -32700, "Parse error")
		return
	}

	docURI := string(p.TextDocument.URI)
	doc := s.documents.Get(docURI)
	if doc == nil {
		s.sendResult(id, []protocol.CompletionItem{})
		return
	}

	line := int(p.Position.Line)
	character := int(p.Position.Character)

	// 获取补全项
	items := s.getCompletionItems(doc, line, character)

	s.sendResult(id, items)
}

// getCompletionItems 获取补全项
func (s *Server) getCompletionItems(doc *Document, line, character int) []protocol.CompletionItem {
	var items []protocol.CompletionItem

	lineText := doc.GetLine(line)
	if character > len(lineText) {
		character = len(lineText)
	}

	// 获取光标前的文本
	prefix := ""
	if character > 0 {
		prefix = lineText[:character]
	}

	// 检测触发上下文
	context := detectCompletionContext(prefix)

	switch context.Type {
	case contextMemberAccess:
		// 成员访问 $obj-> 或 $obj.
		items = s.getMemberCompletions(doc, context.ObjectName)
	case contextStaticAccess:
		// 静态访问 ClassName::
		items = s.getStaticCompletions(doc, context.ClassName)
	case contextVariable:
		// 变量补全 $
		items = s.getVariableCompletions(doc, line)
	case contextType:
		// 类型补全
		items = s.getTypeCompletions(doc)
	case contextNew:
		// new 后面的类名补全
		items = s.getClassCompletions(doc)
	default:
		// 通用补全：关键字 + 全局符号
		items = s.getGeneralCompletions(doc, line, character)
	}

	return items
}

// completionContextType 补全上下文类型
type completionContextType int

const (
	contextGeneral completionContextType = iota
	contextMemberAccess
	contextStaticAccess
	contextVariable
	contextType
	contextNew
)

// completionContext 补全上下文
type completionContext struct {
	Type       completionContextType
	ObjectName string
	ClassName  string
}

// detectCompletionContext 检测补全上下文
func detectCompletionContext(prefix string) completionContext {
	prefix = strings.TrimRight(prefix, " \t")

	// 检查是否是成员访问 $xxx-> 或 $xxx.
	if strings.HasSuffix(prefix, "->") || strings.HasSuffix(prefix, ".") {
		// 查找对象名
		sep := "->"
		if strings.HasSuffix(prefix, ".") {
			sep = "."
		}
		idx := strings.LastIndex(prefix, sep)
		if idx > 0 {
			objPart := strings.TrimRight(prefix[:idx], " \t")
			// 提取对象名（可能是 $var 或 $this）
			start := len(objPart) - 1
			for start >= 0 && (isWordChar(objPart[start]) || objPart[start] == '$') {
				start--
			}
			objName := objPart[start+1:]
			return completionContext{Type: contextMemberAccess, ObjectName: objName}
		}
	}

	// 检查是否是静态访问 ClassName:: 或 ClassName::$ 或 ClassName::xxx
	// 使用更宽松的检测：查找 :: 并检查其后的内容
	if idx := strings.LastIndex(prefix, "::"); idx > 0 {
		classPart := strings.TrimRight(prefix[:idx], " \t")
		start := len(classPart) - 1
		for start >= 0 && isWordChar(classPart[start]) {
			start--
		}
		className := classPart[start+1:]
		if className != "" {
			return completionContext{Type: contextStaticAccess, ClassName: className}
		}
	}

	// 检查是否是变量开始 $
	if strings.HasSuffix(prefix, "$") {
		return completionContext{Type: contextVariable}
	}

	// 检查是否在 new 后面
	trimmed := strings.TrimRight(prefix, " \t")
	if strings.HasSuffix(trimmed, "new ") || strings.HasSuffix(trimmed, "new") {
		return completionContext{Type: contextNew}
	}

	// 检查是否在类型位置（如参数类型、返回类型）- 但排除 ::
	if strings.HasSuffix(trimmed, ":") && !strings.HasSuffix(trimmed, "::") {
		return completionContext{Type: contextType}
	}
	if strings.HasSuffix(trimmed, "extends ") || strings.HasSuffix(trimmed, "implements ") {
		return completionContext{Type: contextType}
	}

	return completionContext{Type: contextGeneral}
}

// getMemberCompletions 获取成员补全（属性和方法）
func (s *Server) getMemberCompletions(doc *Document, objectName string) []protocol.CompletionItem {
	var items []protocol.CompletionItem

	symbols := doc.GetSymbols()
	astFile := doc.GetAST()

	// 如果是 $this，查找当前类的成员
	if objectName == "$this" || objectName == "this" {
		if astFile != nil {
			for _, decl := range astFile.Declarations {
				if classDecl, ok := decl.(*ast.ClassDecl); ok {
					// 添加属性
					for _, prop := range classDecl.Properties {
						items = append(items, protocol.CompletionItem{
							Label:  prop.Name.Name,
							Kind:   protocol.CompletionItemKindProperty,
							Detail: typeNodeToString(prop.Type),
						})
					}
					// 添加方法
					for _, method := range classDecl.Methods {
						items = append(items, protocol.CompletionItem{
							Label:      method.Name.Name,
							Kind:       protocol.CompletionItemKindMethod,
							Detail:     formatMethodSignatureShort(method),
							InsertText: method.Name.Name + "()",
						})
					}
				}
			}
		}
		return items
	}

	// 尝试推断变量的类型
	varName := strings.TrimPrefix(objectName, "$")
	varType := inferVariableType(astFile, varName)

	// 如果推断出了类型，获取该类的方法
	if varType != "" && varType != "dynamic" {
		// 从符号表获取类方法
		if symbols != nil {
			if methods, ok := symbols.ClassMethods[varType]; ok {
				for methodName, sigs := range methods {
					if len(sigs) > 0 && !sigs[0].IsStatic {
						items = append(items, protocol.CompletionItem{
							Label:      methodName,
							Kind:       protocol.CompletionItemKindMethod,
							Detail:     formatMethodSigShort(sigs[0]),
							InsertText: methodName + "()",
						})
					}
				}
			}
			// 获取类属性
			if props, ok := symbols.ClassProperties[varType]; ok {
				for propName, sig := range props {
					if !sig.IsStatic {
						items = append(items, protocol.CompletionItem{
							Label:  propName,
							Kind:   protocol.CompletionItemKindProperty,
							Detail: sig.Type,
						})
					}
				}
			}
		}

		// 从AST中查找类定义
		if astFile != nil {
			for _, decl := range astFile.Declarations {
				if classDecl, ok := decl.(*ast.ClassDecl); ok && classDecl.Name.Name == varType {
					for _, prop := range classDecl.Properties {
						if !prop.Static {
							items = append(items, protocol.CompletionItem{
								Label:  prop.Name.Name,
								Kind:   protocol.CompletionItemKindProperty,
								Detail: typeNodeToString(prop.Type),
							})
						}
					}
					for _, method := range classDecl.Methods {
						if !method.Static {
							items = append(items, protocol.CompletionItem{
								Label:      method.Name.Name,
								Kind:       protocol.CompletionItemKindMethod,
								Detail:     formatMethodSignatureShort(method),
								InsertText: method.Name.Name + "()",
							})
						}
					}
				}
			}
		}

		// 如果已经找到了类的方法，直接返回
		if len(items) > 0 {
			return items
		}

		// 从导入的文件中查找类定义（通过工作区索引）
		if s.workspace != nil && astFile != nil {
			items = s.getMemberCompletionsFromImports(astFile, varType)
			if len(items) > 0 {
				return items
			}
		}
	}

	// 添加内置数组/字符串方法
	builtinMethods := []struct {
		name   string
		detail string
	}{
		{"len", "(): int - 获取长度"},
		{"push", "($item): void - 添加元素"},
		{"pop", "(): mixed - 弹出最后一个元素"},
		{"slice", "($start, $end?): array - 切片"},
		{"keys", "(): array - 获取所有键"},
		{"values", "(): array - 获取所有值"},
		{"hasKey", "($key): bool - 检查键是否存在"},
		{"get", "($key, $default?): mixed - 获取值"},
		{"set", "($key, $value): self - 设置值"},
	}

	for _, m := range builtinMethods {
		items = append(items, protocol.CompletionItem{
			Label:      m.name,
			Kind:       protocol.CompletionItemKindMethod,
			Detail:     m.detail,
			InsertText: m.name + "()",
		})
	}

	return items
}

// getMemberCompletionsFromImports 从导入的文件中获取成员补全
func (s *Server) getMemberCompletionsFromImports(currentFile *ast.File, className string) []protocol.CompletionItem {
	var items []protocol.CompletionItem

	if s.workspace == nil || currentFile == nil {
		return items
	}

	// 遍历 use 声明，查找类定义
	for _, use := range currentFile.Uses {
		if use == nil {
			continue
		}
		importedPath, err := s.workspace.ResolveImport(use.Path)
		if err != nil || importedPath == "" {
			continue
		}
		indexed := s.workspace.GetIndexedFile(importedPath)
		if indexed == nil || indexed.AST == nil {
			continue
		}

		// 在导入文件中查找类
		for _, decl := range indexed.AST.Declarations {
			if classDecl, ok := decl.(*ast.ClassDecl); ok && classDecl.Name.Name == className {
				// 添加属性
				for _, prop := range classDecl.Properties {
					if !prop.Static {
						items = append(items, protocol.CompletionItem{
							Label:  prop.Name.Name,
							Kind:   protocol.CompletionItemKindProperty,
							Detail: typeNodeToString(prop.Type),
						})
					}
				}
				// 添加方法
				for _, method := range classDecl.Methods {
					if !method.Static {
						items = append(items, protocol.CompletionItem{
							Label:      method.Name.Name,
							Kind:       protocol.CompletionItemKindMethod,
							Detail:     formatMethodSignatureShort(method),
							InsertText: method.Name.Name + "()",
						})
					}
				}
				return items
			}
		}
	}

	// 在全局符号索引中查找
	if indexed := s.workspace.FindSymbolFile(className); indexed != nil && indexed.AST != nil {
		for _, decl := range indexed.AST.Declarations {
			if classDecl, ok := decl.(*ast.ClassDecl); ok && classDecl.Name.Name == className {
				for _, prop := range classDecl.Properties {
					if !prop.Static {
						items = append(items, protocol.CompletionItem{
							Label:  prop.Name.Name,
							Kind:   protocol.CompletionItemKindProperty,
							Detail: typeNodeToString(prop.Type),
						})
					}
				}
				for _, method := range classDecl.Methods {
					if !method.Static {
						items = append(items, protocol.CompletionItem{
							Label:      method.Name.Name,
							Kind:       protocol.CompletionItemKindMethod,
							Detail:     formatMethodSignatureShort(method),
							InsertText: method.Name.Name + "()",
						})
					}
				}
				return items
			}
		}
	}

	return items
}

// inferVariableType 推断变量类型
func inferVariableType(file *ast.File, varName string) string {
	if file == nil {
		return ""
	}

	// 从顶层语句中查找变量声明
	for _, stmt := range file.Statements {
		if t := inferTypeFromStmt(stmt, varName); t != "" {
			return t
		}
	}

	// 从类方法中查找
	for _, decl := range file.Declarations {
		if classDecl, ok := decl.(*ast.ClassDecl); ok {
			for _, method := range classDecl.Methods {
				// 检查方法参数
				for _, param := range method.Parameters {
					if param.Name.Name == varName && param.Type != nil {
						return typeNodeToString(param.Type)
					}
				}
				// 检查方法体
				if method.Body != nil {
					if t := inferTypeFromStmt(method.Body, varName); t != "" {
						return t
					}
				}
			}
		}
	}

	return ""
}

// inferTypeFromStmt 从语句中推断变量类型
func inferTypeFromStmt(stmt ast.Statement, varName string) string {
	if stmt == nil {
		return ""
	}

	switch s := stmt.(type) {
	case *ast.VarDeclStmt:
		if s.Name.Name == varName {
			// 如果有显式类型
			if s.Type != nil {
				return typeNodeToString(s.Type)
			}
			// 从初始化表达式推断类型
			return inferTypeFromExpr(s.Value)
		}
	case *ast.BlockStmt:
		for _, inner := range s.Statements {
			if t := inferTypeFromStmt(inner, varName); t != "" {
				return t
			}
		}
	case *ast.IfStmt:
		if t := inferTypeFromStmt(s.Then, varName); t != "" {
			return t
		}
		if s.Else != nil {
			if t := inferTypeFromStmt(s.Else, varName); t != "" {
				return t
			}
		}
	case *ast.ForStmt:
		if s.Init != nil {
			if t := inferTypeFromStmt(s.Init, varName); t != "" {
				return t
			}
		}
		if s.Body != nil {
			if t := inferTypeFromStmt(s.Body, varName); t != "" {
				return t
			}
		}
	case *ast.ForeachStmt:
		if s.Body != nil {
			if t := inferTypeFromStmt(s.Body, varName); t != "" {
				return t
			}
		}
	}
	return ""
}

// inferTypeFromExpr 从表达式推断类型
func inferTypeFromExpr(expr ast.Expression) string {
	if expr == nil {
		return ""
	}

	switch e := expr.(type) {
	case *ast.NewExpr:
		return e.ClassName.Name
	case *ast.StringLiteral:
		return "string"
	case *ast.InterpStringLiteral:
		return "string"
	case *ast.IntegerLiteral:
		return "int"
	case *ast.FloatLiteral:
		return "float"
	case *ast.BoolLiteral:
		return "bool"
	case *ast.NullLiteral:
		return "null"
	case *ast.ArrayLiteral:
		if e.ElementType != nil {
			return typeNodeToString(e.ElementType) + "[]"
		}
		// 尝试从元素推断
		if len(e.Elements) > 0 {
			elemType := inferTypeFromExpr(e.Elements[0])
			if elemType != "" {
				return elemType + "[]"
			}
		}
		return "array"
	case *ast.MapLiteral:
		if e.KeyType != nil && e.ValueType != nil {
			return "map[" + typeNodeToString(e.KeyType) + "]" + typeNodeToString(e.ValueType)
		}
		return "map"
	case *ast.SuperArrayLiteral:
		return "SuperArray"
	case *ast.BinaryExpr:
		// 二元表达式 - 尝试推断
		leftType := inferTypeFromExpr(e.Left)
		rightType := inferTypeFromExpr(e.Right)
		op := e.Operator.Literal
		// 字符串连接
		if op == "." || op == "+" {
			if leftType == "string" || rightType == "string" {
				return "string"
			}
		}
		// 数值运算
		if op == "+" || op == "-" || op == "*" || op == "/" || op == "%" {
			if leftType == "float" || rightType == "float" {
				return "float"
			}
			if leftType == "int" && rightType == "int" {
				return "int"
			}
		}
		// 比较运算
		if op == "==" || op == "!=" || op == "<" || op == ">" || op == "<=" || op == ">=" || op == "&&" || op == "||" {
			return "bool"
		}
		return ""
	case *ast.TernaryExpr:
		// 三元表达式 - 返回then分支的类型
		return inferTypeFromExpr(e.Then)
	}
	return ""
}

// getStaticCompletions 获取静态成员补全
func (s *Server) getStaticCompletions(doc *Document, className string) []protocol.CompletionItem {
	var items []protocol.CompletionItem

	symbols := doc.GetSymbols()
	astFile := doc.GetAST()

	// 尝试解析类名（可能是简短名或完整名）
	fullClassName := resolveClassName(astFile, className)

	// 从符号表查找
	if symbols != nil {
		// 尝试多种类名形式
		classNames := []string{className, fullClassName}
		for _, cn := range classNames {
			if cn == "" {
				continue
			}
			// 查找类的静态方法
			if methods, ok := symbols.ClassMethods[cn]; ok {
				for methodName, sigs := range methods {
					if len(sigs) > 0 && sigs[0].IsStatic {
						items = append(items, protocol.CompletionItem{
							Label:      methodName,
							Kind:       protocol.CompletionItemKindMethod,
							Detail:     formatMethodSigShort(sigs[0]),
							InsertText: methodName + "()",
						})
					}
				}
			}

			// 查找类的静态属性
			if props, ok := symbols.ClassProperties[cn]; ok {
				for propName, sig := range props {
					if sig.IsStatic {
						items = append(items, protocol.CompletionItem{
							Label:  "$" + propName,
							Kind:   protocol.CompletionItemKindProperty,
							Detail: sig.Type,
						})
					}
				}
			}

			// 查找枚举值
			if values := symbols.GetEnumValues(cn); len(values) > 0 {
				for _, val := range values {
					items = append(items, protocol.CompletionItem{
						Label:  val,
						Kind:   protocol.CompletionItemKindEnumMember,
						Detail: cn + "::" + val,
					})
				}
			}
		}
	}

	// 从当前文件的AST中查找类定义
	if astFile != nil {
		for _, decl := range astFile.Declarations {
			switch d := decl.(type) {
			case *ast.ClassDecl:
				if d.Name.Name == className {
					// 添加静态属性
					for _, prop := range d.Properties {
						if prop.Static {
							items = append(items, protocol.CompletionItem{
								Label:  "$" + prop.Name.Name,
								Kind:   protocol.CompletionItemKindProperty,
								Detail: typeNodeToString(prop.Type),
							})
						}
					}
					// 添加静态方法
					for _, method := range d.Methods {
						if method.Static {
							items = append(items, protocol.CompletionItem{
								Label:      method.Name.Name,
								Kind:       protocol.CompletionItemKindMethod,
								Detail:     formatMethodSignatureShort(method),
								InsertText: method.Name.Name + "()",
							})
						}
					}
					// 添加常量
					for _, c := range d.Constants {
						items = append(items, protocol.CompletionItem{
							Label:  c.Name.Name,
							Kind:   protocol.CompletionItemKindConstant,
							Detail: typeNodeToString(c.Type),
						})
					}
				}
			case *ast.EnumDecl:
				if d.Name.Name == className {
					for _, c := range d.Cases {
						items = append(items, protocol.CompletionItem{
							Label:  c.Name.Name,
							Kind:   protocol.CompletionItemKindEnumMember,
							Detail: className + "::" + c.Name.Name,
						})
					}
				}
			}
		}
	}

	// 如果没有找到，从导入的文件中查找
	if len(items) == 0 && s.workspace != nil && astFile != nil {
		items = s.getStaticCompletionsFromImports(astFile, className)
	}

	return items
}

// getStaticCompletionsFromImports 从导入的文件中获取静态成员补全
func (s *Server) getStaticCompletionsFromImports(currentFile *ast.File, className string) []protocol.CompletionItem {
	var items []protocol.CompletionItem

	if s.workspace == nil || currentFile == nil {
		return items
	}

	// 遍历 use 声明
	for _, use := range currentFile.Uses {
		if use == nil {
			continue
		}
		importedPath, err := s.workspace.ResolveImport(use.Path)
		if err != nil || importedPath == "" {
			continue
		}
		indexed := s.workspace.GetIndexedFile(importedPath)
		if indexed == nil || indexed.AST == nil {
			continue
		}

		for _, decl := range indexed.AST.Declarations {
			switch d := decl.(type) {
			case *ast.ClassDecl:
				if d.Name.Name == className {
					for _, prop := range d.Properties {
						if prop.Static {
							items = append(items, protocol.CompletionItem{
								Label:  "$" + prop.Name.Name,
								Kind:   protocol.CompletionItemKindProperty,
								Detail: typeNodeToString(prop.Type),
							})
						}
					}
					for _, method := range d.Methods {
						if method.Static {
							items = append(items, protocol.CompletionItem{
								Label:      method.Name.Name,
								Kind:       protocol.CompletionItemKindMethod,
								Detail:     formatMethodSignatureShort(method),
								InsertText: method.Name.Name + "()",
							})
						}
					}
					for _, c := range d.Constants {
						items = append(items, protocol.CompletionItem{
							Label:  c.Name.Name,
							Kind:   protocol.CompletionItemKindConstant,
							Detail: typeNodeToString(c.Type),
						})
					}
					return items
				}
			case *ast.EnumDecl:
				if d.Name.Name == className {
					for _, c := range d.Cases {
						items = append(items, protocol.CompletionItem{
							Label:  c.Name.Name,
							Kind:   protocol.CompletionItemKindEnumMember,
							Detail: className + "::" + c.Name.Name,
						})
					}
					return items
				}
			}
		}
	}

	// 在全局符号索引中查找
	if indexed := s.workspace.FindSymbolFile(className); indexed != nil && indexed.AST != nil {
		for _, decl := range indexed.AST.Declarations {
			switch d := decl.(type) {
			case *ast.ClassDecl:
				if d.Name.Name == className {
					for _, prop := range d.Properties {
						if prop.Static {
							items = append(items, protocol.CompletionItem{
								Label:  "$" + prop.Name.Name,
								Kind:   protocol.CompletionItemKindProperty,
								Detail: typeNodeToString(prop.Type),
							})
						}
					}
					for _, method := range d.Methods {
						if method.Static {
							items = append(items, protocol.CompletionItem{
								Label:      method.Name.Name,
								Kind:       protocol.CompletionItemKindMethod,
								Detail:     formatMethodSignatureShort(method),
								InsertText: method.Name.Name + "()",
							})
						}
					}
					return items
				}
			case *ast.EnumDecl:
				if d.Name.Name == className {
					for _, c := range d.Cases {
						items = append(items, protocol.CompletionItem{
							Label:  c.Name.Name,
							Kind:   protocol.CompletionItemKindEnumMember,
							Detail: className + "::" + c.Name.Name,
						})
					}
					return items
				}
			}
		}
	}

	return items
}

// resolveClassName 解析类名（从use声明中查找完整名）
func resolveClassName(file *ast.File, shortName string) string {
	if file == nil {
		return shortName
	}

	// 从 use 声明中查找
	for _, use := range file.Uses {
		// 检查是否有别名
		if use.Alias != nil && use.Alias.Name == shortName {
			return use.Path
		}
		// 检查路径的最后一部分是否匹配
		parts := strings.Split(use.Path, ".")
		if len(parts) > 0 && parts[len(parts)-1] == shortName {
			return use.Path
		}
	}

	return shortName
}

// getVariableCompletions 获取变量补全
func (s *Server) getVariableCompletions(doc *Document, line int) []protocol.CompletionItem {
	var items []protocol.CompletionItem

	// 添加预定义变量
	predefinedVars := []string{"this", "self", "parent"}
	for _, v := range predefinedVars {
		items = append(items, protocol.CompletionItem{
			Label: v,
			Kind:  protocol.CompletionItemKindVariable,
		})
	}

	// 从文档中收集已声明的变量
	astFile := doc.GetAST()
	if astFile != nil {
		vars := collectVariables(astFile, line+1)
		for varName := range vars {
			items = append(items, protocol.CompletionItem{
				Label: varName,
				Kind:  protocol.CompletionItemKindVariable,
			})
		}
	}

	return items
}

// collectVariables 收集变量声明
func collectVariables(file *ast.File, beforeLine int) map[string]bool {
	vars := make(map[string]bool)

	for _, stmt := range file.Statements {
		collectVarsFromStmt(stmt, beforeLine, vars)
	}

	// 收集类属性
	for _, decl := range file.Declarations {
		if classDecl, ok := decl.(*ast.ClassDecl); ok {
			for _, prop := range classDecl.Properties {
				vars[prop.Name.Name] = true
			}
			// 收集方法参数
			for _, method := range classDecl.Methods {
				for _, param := range method.Parameters {
					vars[param.Name.Name] = true
				}
			}
		}
	}

	return vars
}

// collectVarsFromStmt 从语句中收集变量
func collectVarsFromStmt(stmt ast.Statement, beforeLine int, vars map[string]bool) {
	if stmt == nil {
		return
	}

	switch s := stmt.(type) {
	case *ast.VarDeclStmt:
		if s.Name.Token.Pos.Line <= beforeLine {
			vars[s.Name.Name] = true
		}
	case *ast.MultiVarDeclStmt:
		for _, v := range s.Names {
			if v.Token.Pos.Line <= beforeLine {
				vars[v.Name] = true
			}
		}
	case *ast.BlockStmt:
		for _, inner := range s.Statements {
			collectVarsFromStmt(inner, beforeLine, vars)
		}
	case *ast.IfStmt:
		collectVarsFromStmt(s.Then, beforeLine, vars)
		if s.Else != nil {
			collectVarsFromStmt(s.Else, beforeLine, vars)
		}
	case *ast.ForStmt:
		if s.Init != nil {
			collectVarsFromStmt(s.Init, beforeLine, vars)
		}
		collectVarsFromStmt(s.Body, beforeLine, vars)
	case *ast.ForeachStmt:
		if s.Key != nil && s.ForeachToken.Pos.Line <= beforeLine {
			vars[s.Key.Name] = true
		}
		if s.Value != nil && s.ForeachToken.Pos.Line <= beforeLine {
			vars[s.Value.Name] = true
		}
		collectVarsFromStmt(s.Body, beforeLine, vars)
	}
}

// getTypeCompletions 获取类型补全
func (s *Server) getTypeCompletions(doc *Document) []protocol.CompletionItem {
	var items []protocol.CompletionItem

	// 基础类型
	baseTypes := []string{
		"int", "i8", "i16", "i32", "i64",
		"uint", "u8", "u16", "u32", "u64", "byte",
		"float", "f32", "f64",
		"bool", "string", "void", "dynamic", "unknown",
	}

	for _, t := range baseTypes {
		items = append(items, protocol.CompletionItem{
			Label: t,
			Kind:  protocol.CompletionItemKindKeyword,
		})
	}

	// 添加已知的类和接口
	items = append(items, s.getClassCompletions(doc)...)

	return items
}

// getClassCompletions 获取类名补全
func (s *Server) getClassCompletions(doc *Document) []protocol.CompletionItem {
	var items []protocol.CompletionItem
	seen := make(map[string]bool) // 避免重复

	symbols := doc.GetSymbols()
	if symbols != nil {
		// 从符号表获取所有类
		for className := range symbols.ClassMethods {
			if !seen[className] {
				seen[className] = true
				items = append(items, protocol.CompletionItem{
					Label: className,
					Kind:  protocol.CompletionItemKindClass,
				})
			}
		}

		// 从符号表获取所有接口
		for interfaceName := range symbols.InterfaceSigs {
			if !seen[interfaceName] {
				seen[interfaceName] = true
				items = append(items, protocol.CompletionItem{
					Label: interfaceName,
					Kind:  protocol.CompletionItemKindInterface,
				})
			}
		}
	}

	// 从当前文件的 AST 获取类和接口
	astFile := doc.GetAST()
	if astFile != nil {
		for _, decl := range astFile.Declarations {
			switch d := decl.(type) {
			case *ast.ClassDecl:
				if !seen[d.Name.Name] {
					seen[d.Name.Name] = true
					items = append(items, protocol.CompletionItem{
						Label: d.Name.Name,
						Kind:  protocol.CompletionItemKindClass,
					})
				}
			case *ast.InterfaceDecl:
				if !seen[d.Name.Name] {
					seen[d.Name.Name] = true
					items = append(items, protocol.CompletionItem{
						Label: d.Name.Name,
						Kind:  protocol.CompletionItemKindInterface,
					})
				}
			case *ast.EnumDecl:
				if !seen[d.Name.Name] {
					seen[d.Name.Name] = true
					items = append(items, protocol.CompletionItem{
						Label: d.Name.Name,
						Kind:  protocol.CompletionItemKindEnum,
					})
				}
			}
		}

		// 从导入的文件中获取类/接口（通过 use 声明）
		if s.workspace != nil && astFile != nil {
			for _, use := range astFile.Uses {
				if use == nil {
					continue
				}
				importedPath, err := s.workspace.ResolveImport(use.Path)
				if err != nil || importedPath == "" {
					continue
				}
				indexed := s.workspace.GetIndexedFile(importedPath)
				if indexed == nil || indexed.AST == nil {
					continue
				}
				for _, decl := range indexed.AST.Declarations {
					switch d := decl.(type) {
					case *ast.ClassDecl:
						if !seen[d.Name.Name] {
							seen[d.Name.Name] = true
							items = append(items, protocol.CompletionItem{
								Label:  d.Name.Name,
								Kind:   protocol.CompletionItemKindClass,
								Detail: use.Path,
							})
						}
					case *ast.InterfaceDecl:
						if !seen[d.Name.Name] {
							seen[d.Name.Name] = true
							items = append(items, protocol.CompletionItem{
								Label:  d.Name.Name,
								Kind:   protocol.CompletionItemKindInterface,
								Detail: use.Path,
							})
						}
					case *ast.EnumDecl:
						if !seen[d.Name.Name] {
							seen[d.Name.Name] = true
							items = append(items, protocol.CompletionItem{
								Label:  d.Name.Name,
								Kind:   protocol.CompletionItemKindEnum,
								Detail: use.Path,
							})
						}
					}
				}
			}
		}
	}

	return items
}

// getGeneralCompletions 获取通用补全
func (s *Server) getGeneralCompletions(doc *Document, line, character int) []protocol.CompletionItem {
	var items []protocol.CompletionItem

	// 关键字
	keywords := []string{
		"class", "interface", "enum", "type",
		"function", "public", "private", "protected",
		"static", "final", "abstract", "const",
		"extends", "implements", "new", "return",
		"if", "elseif", "else", "switch", "case", "default",
		"for", "foreach", "while", "do", "break", "continue",
		"try", "catch", "finally", "throw",
		"true", "false", "null", "this", "self", "parent",
		"echo", "use", "namespace", "as", "is",
		"go", "select", "match",
	}

	for _, kw := range keywords {
		items = append(items, protocol.CompletionItem{
			Label: kw,
			Kind:  protocol.CompletionItemKindKeyword,
		})
	}

	// 内置函数
	symbols := doc.GetSymbols()
	if symbols != nil {
		for fnName, fn := range symbols.Functions {
			// 跳过以 native_ 开头的内部函数
			if strings.HasPrefix(fnName, "native_") {
				continue
			}
			items = append(items, protocol.CompletionItem{
				Label:      fnName,
				Kind:       protocol.CompletionItemKindFunction,
				Detail:     formatFunctionSigShort(fn),
				InsertText: fnName + "()",
			})
		}
	}

	// 变量补全
	varItems := s.getVariableCompletions(doc, line)
	for _, item := range varItems {
		item.InsertText = "$" + item.Label
		item.Label = "$" + item.Label
		items = append(items, item)
	}

	// 类名补全
	items = append(items, s.getClassCompletions(doc)...)

	return items
}

// formatMethodSignatureShort 格式化方法签名（短格式）
func formatMethodSignatureShort(m *ast.MethodDecl) string {
	var params []string
	for _, p := range m.Parameters {
		paramStr := ""
		if p.Type != nil {
			paramStr = typeNodeToString(p.Type) + " "
		}
		paramStr += "$" + p.Name.Name
		params = append(params, paramStr)
	}

	result := "(" + strings.Join(params, ", ") + ")"
	if m.ReturnType != nil {
		result += ": " + typeNodeToString(m.ReturnType)
	}
	return result
}

// formatMethodSigShort 格式化方法签名（从符号表）
func formatMethodSigShort(sig *compiler.MethodSignature) string {
	var params []string
	for i, pt := range sig.ParamTypes {
		paramName := ""
		if i < len(sig.ParamNames) {
			paramName = " $" + sig.ParamNames[i]
		}
		params = append(params, pt+paramName)
	}

	result := "(" + strings.Join(params, ", ") + ")"
	if sig.ReturnType != "" && sig.ReturnType != "void" {
		result += ": " + sig.ReturnType
	}
	return result
}

// formatFunctionSigShort 格式化函数签名（短格式）
func formatFunctionSigShort(fn *compiler.FunctionSignature) string {
	var params []string
	for i, pt := range fn.ParamTypes {
		paramName := ""
		if i < len(fn.ParamNames) {
			paramName = " $" + fn.ParamNames[i]
		}
		params = append(params, pt+paramName)
	}

	result := "(" + strings.Join(params, ", ") + ")"
	if fn.ReturnType != "" && fn.ReturnType != "void" {
		result += ": " + fn.ReturnType
	}
	return result
}

// handleCompletionResolve 处理补全项解析请求
func (s *Server) handleCompletionResolve(id json.RawMessage, params json.RawMessage) {
	var item protocol.CompletionItem
	if err := json.Unmarshal(params, &item); err != nil {
		s.sendError(id, -32700, "Parse error")
		return
	}

	// 解析补全项的详细信息
	resolvedItem := s.resolveCompletionItem(item)
	s.sendResult(id, resolvedItem)
}

// resolveCompletionItem 解析补全项的详细信息
func (s *Server) resolveCompletionItem(item protocol.CompletionItem) protocol.CompletionItem {
	// 获取补全项的详细文档
	switch item.Kind {
	case protocol.CompletionItemKindFunction:
		item.Documentation = s.getFunctionDocumentation(item.Label)
	case protocol.CompletionItemKindMethod:
		item.Documentation = s.getMethodDocumentation(item.Label, item.Detail)
	case protocol.CompletionItemKindClass:
		item.Documentation = s.getClassDocumentation(item.Label)
	case protocol.CompletionItemKindInterface:
		item.Documentation = s.getInterfaceDocumentation(item.Label)
	case protocol.CompletionItemKindProperty:
		item.Documentation = s.getPropertyDocumentation(item.Label, item.Detail)
	case protocol.CompletionItemKindVariable:
		item.Documentation = s.getVariableDocumentation(item.Label)
	case protocol.CompletionItemKindKeyword:
		item.Documentation = s.getKeywordDocumentation(item.Label)
	}

	return item
}

// getFunctionDocumentation 获取函数文档
func (s *Server) getFunctionDocumentation(name string) *protocol.MarkupContent {
	// 内置函数文档
	builtinDocs := map[string]string{
		"print":   "输出内容到标准输出\n\n**语法**: `print(...$values)`\n\n**参数**: 可变参数，支持任意类型",
		"println": "输出内容到标准输出并换行\n\n**语法**: `println(...$values)`\n\n**参数**: 可变参数，支持任意类型",
		"len":     "获取字符串、数组或Map的长度\n\n**语法**: `len($value): int`\n\n**返回**: 长度值",
		"type":    "获取值的类型名称\n\n**语法**: `type($value): string`\n\n**返回**: 类型名称字符串",
		"time":    "获取当前Unix时间戳（毫秒）\n\n**语法**: `time(): int`\n\n**返回**: 时间戳",
		"sleep":   "暂停执行指定毫秒数\n\n**语法**: `sleep($ms: int): void`",
		"panic":   "抛出运行时错误\n\n**语法**: `panic($message: string): void`",
		"assert":  "断言条件为真，否则抛出错误\n\n**语法**: `assert($condition: bool, $message?: string): void`",
	}

	if doc, ok := builtinDocs[name]; ok {
		return &protocol.MarkupContent{
			Kind:  protocol.Markdown,
			Value: doc,
		}
	}

	// 从工作区索引查找函数
	if s.workspace != nil {
		for _, indexed := range s.workspace.GetAllFiles() {
			if indexed.Symbols != nil {
				if fn := indexed.Symbols.GetFunction(name); fn != nil {
					doc := formatFunctionDoc(name, fn)
					return &protocol.MarkupContent{
						Kind:  protocol.Markdown,
						Value: doc,
					}
				}
			}
		}
	}

	return nil
}

// getMethodDocumentation 获取方法文档
func (s *Server) getMethodDocumentation(methodName, detail string) *protocol.MarkupContent {
	// 解析类名（如果detail包含类名信息）
	className := ""
	if detail != "" {
		parts := strings.Split(detail, "::")
		if len(parts) > 0 {
			className = strings.TrimSpace(parts[0])
		}
	}

	// 从工作区查找方法定义
	if s.workspace != nil && className != "" {
		indexed := s.workspace.FindSymbolFile(className)
		if indexed != nil && indexed.AST != nil {
			for _, decl := range indexed.AST.Declarations {
				if classDecl, ok := decl.(*ast.ClassDecl); ok && classDecl.Name.Name == className {
					for _, method := range classDecl.Methods {
						if method.Name.Name == methodName {
							doc := formatMethodDoc(className, method)
							return &protocol.MarkupContent{
								Kind:  protocol.Markdown,
								Value: doc,
							}
						}
					}
				}
			}
		}
	}

	return nil
}

// getClassDocumentation 获取类文档
func (s *Server) getClassDocumentation(name string) *protocol.MarkupContent {
	if s.workspace != nil {
		indexed := s.workspace.FindSymbolFile(name)
		if indexed != nil && indexed.AST != nil {
			for _, decl := range indexed.AST.Declarations {
				if classDecl, ok := decl.(*ast.ClassDecl); ok && classDecl.Name.Name == name {
					doc := formatClassDoc(classDecl)
					return &protocol.MarkupContent{
						Kind:  protocol.Markdown,
						Value: doc,
					}
				}
			}
		}
	}
	return nil
}

// getInterfaceDocumentation 获取接口文档
func (s *Server) getInterfaceDocumentation(name string) *protocol.MarkupContent {
	if s.workspace != nil {
		indexed := s.workspace.FindSymbolFile(name)
		if indexed != nil && indexed.AST != nil {
			for _, decl := range indexed.AST.Declarations {
				if ifaceDecl, ok := decl.(*ast.InterfaceDecl); ok && ifaceDecl.Name.Name == name {
					doc := formatInterfaceDoc(ifaceDecl)
					return &protocol.MarkupContent{
						Kind:  protocol.Markdown,
						Value: doc,
					}
				}
			}
		}
	}
	return nil
}

// getPropertyDocumentation 获取属性文档
func (s *Server) getPropertyDocumentation(name, detail string) *protocol.MarkupContent {
	if detail != "" {
		return &protocol.MarkupContent{
			Kind:  protocol.Markdown,
			Value: "**类型**: `" + detail + "`",
		}
	}
	return nil
}

// getVariableDocumentation 获取变量文档
func (s *Server) getVariableDocumentation(name string) *protocol.MarkupContent {
	// 特殊变量文档
	specialVars := map[string]string{
		"$this":   "当前对象实例的引用",
		"$self":   "当前类的静态引用",
		"$parent": "父类的静态引用",
	}

	if doc, ok := specialVars[name]; ok {
		return &protocol.MarkupContent{
			Kind:  protocol.Markdown,
			Value: doc,
		}
	}
	return nil
}

// getKeywordDocumentation 获取关键字文档
func (s *Server) getKeywordDocumentation(keyword string) *protocol.MarkupContent {
	keywordDocs := map[string]string{
		"class":      "定义一个类\n\n```sola\nclass ClassName {\n    // 属性和方法\n}\n```",
		"interface":  "定义一个接口\n\n```sola\ninterface InterfaceName {\n    public function methodName(): ReturnType;\n}\n```",
		"enum":       "定义一个枚举\n\n```sola\nenum EnumName {\n    case Value1;\n    case Value2;\n}\n```",
		"function":   "定义一个方法\n\n```sola\npublic function name($param: Type): ReturnType {\n    // 方法体\n}\n```",
		"extends":    "继承一个类\n\n```sola\nclass Child extends Parent {\n}\n```",
		"implements": "实现一个或多个接口\n\n```sola\nclass MyClass implements Interface1, Interface2 {\n}\n```",
		"new":        "创建类的实例\n\n```sola\n$obj := new ClassName($arg1, $arg2)\n```",
		"return":     "从方法返回值\n\n```sola\nreturn $value\n```",
		"if":         "条件语句\n\n```sola\nif ($condition) {\n    // ...\n} elseif ($other) {\n    // ...\n} else {\n    // ...\n}\n```",
		"for":        "循环语句\n\n```sola\nfor ($i := 0; $i < 10; $i++) {\n    // ...\n}\n```",
		"foreach":    "遍历数组或Map\n\n```sola\nforeach ($array as $key => $value) {\n    // ...\n}\n```",
		"while":      "while循环\n\n```sola\nwhile ($condition) {\n    // ...\n}\n```",
		"try":        "异常处理\n\n```sola\ntry {\n    // ...\n} catch (Exception $e) {\n    // ...\n} finally {\n    // ...\n}\n```",
		"throw":      "抛出异常\n\n```sola\nthrow new Exception(\"error message\")\n```",
		"use":        "导入模块\n\n```sola\nuse \"module.path\"\n```",
		"static":     "声明静态成员，属于类本身而非实例",
		"public":     "公开访问修饰符，可从任何地方访问",
		"private":    "私有访问修饰符，仅在当前类中可访问",
		"protected":  "受保护访问修饰符，在当前类和子类中可访问",
		"const":      "定义常量\n\n```sola\nconst NAME: Type = value;\n```",
		"final":      "声明不可被重写的方法或不可被继承的类",
		"abstract":   "声明抽象类或抽象方法",
		"go":         "启动一个并发协程\n\n```sola\ngo functionCall()\n```",
		"select":     "多路通道选择\n\n```sola\nselect {\n    case $val := <- $ch1:\n        // ...\n    case $ch2 <- $val:\n        // ...\n}\n```",
		"match":      "模式匹配表达式\n\n```sola\nmatch ($value) {\n    1 => \"one\",\n    2 => \"two\",\n    _ => \"other\"\n}\n```",
	}

	if doc, ok := keywordDocs[keyword]; ok {
		return &protocol.MarkupContent{
			Kind:  protocol.Markdown,
			Value: doc,
		}
	}
	return nil
}

// formatFunctionDoc 格式化函数文档
func formatFunctionDoc(name string, fn *compiler.FunctionSignature) string {
	var sb strings.Builder
	sb.WriteString("```sola\n")
	sb.WriteString("function " + name + "(")

	for i, pt := range fn.ParamTypes {
		if i > 0 {
			sb.WriteString(", ")
		}
		paramName := "$arg" + string(rune('0'+i))
		if i < len(fn.ParamNames) {
			paramName = "$" + fn.ParamNames[i]
		}
		sb.WriteString(paramName + ": " + pt)
	}
	sb.WriteString(")")

	if fn.ReturnType != "" && fn.ReturnType != "void" {
		sb.WriteString(": " + fn.ReturnType)
	}
	sb.WriteString("\n```")

	return sb.String()
}

// formatMethodDoc 格式化方法文档
func formatMethodDoc(className string, method *ast.MethodDecl) string {
	var sb strings.Builder
	sb.WriteString("```sola\n")

	// 访问修饰符
	if method.Static {
		sb.WriteString("static ")
	}

	sb.WriteString("function " + method.Name.Name + "(")

	for i, param := range method.Parameters {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString("$" + param.Name.Name)
		if param.Type != nil {
			sb.WriteString(": " + typeNodeToString(param.Type))
		}
	}
	sb.WriteString(")")

	if method.ReturnType != nil {
		sb.WriteString(": " + typeNodeToString(method.ReturnType))
	}
	sb.WriteString("\n```\n\n")
	sb.WriteString("**类**: `" + className + "`")

	return sb.String()
}

// formatClassDoc 格式化类文档
func formatClassDoc(classDecl *ast.ClassDecl) string {
	var sb strings.Builder
	sb.WriteString("```sola\n")
	sb.WriteString("class " + classDecl.Name.Name)

	if classDecl.Extends != nil {
		sb.WriteString(" extends " + classDecl.Extends.Name)
	}

	if len(classDecl.Implements) > 0 {
		sb.WriteString(" implements ")
		for i, impl := range classDecl.Implements {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(impl.String())
		}
	}

	sb.WriteString("\n```\n\n")

	// 方法列表
	if len(classDecl.Methods) > 0 {
		sb.WriteString("**方法**:\n")
		for _, method := range classDecl.Methods {
			sb.WriteString("- `" + method.Name.Name + formatMethodSignatureShort(method) + "`\n")
		}
	}

	// 属性列表
	if len(classDecl.Properties) > 0 {
		sb.WriteString("\n**属性**:\n")
		for _, prop := range classDecl.Properties {
			sb.WriteString("- `$" + prop.Name.Name)
			if prop.Type != nil {
				sb.WriteString(": " + typeNodeToString(prop.Type))
			}
			sb.WriteString("`\n")
		}
	}

	return sb.String()
}

// formatInterfaceDoc 格式化接口文档
func formatInterfaceDoc(ifaceDecl *ast.InterfaceDecl) string {
	var sb strings.Builder
	sb.WriteString("```sola\n")
	sb.WriteString("interface " + ifaceDecl.Name.Name)

	if len(ifaceDecl.Extends) > 0 {
		sb.WriteString(" extends ")
		for i, ext := range ifaceDecl.Extends {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(ext.String())
		}
	}

	sb.WriteString("\n```\n\n")

	// 方法列表
	if len(ifaceDecl.Methods) > 0 {
		sb.WriteString("**方法**:\n")
		for _, method := range ifaceDecl.Methods {
			sb.WriteString("- `" + method.Name.Name + formatMethodSignatureShort(method) + "`\n")
		}
	}

	return sb.String()
}

// handleSignatureHelp 处理签名帮助请求
func (s *Server) handleSignatureHelp(id json.RawMessage, params json.RawMessage) {
	var p protocol.SignatureHelpParams
	if err := json.Unmarshal(params, &p); err != nil {
		s.sendError(id, -32700, "Parse error")
		return
	}

	docURI := string(p.TextDocument.URI)
	doc := s.documents.Get(docURI)
	if doc == nil {
		s.sendResult(id, nil)
		return
	}

	line := int(p.Position.Line)
	character := int(p.Position.Character)

	// 获取签名帮助
	help := s.getSignatureHelp(doc, line, character)
	s.sendResult(id, help)
}

// getSignatureHelp 获取签名帮助
func (s *Server) getSignatureHelp(doc *Document, line, character int) *protocol.SignatureHelp {
	lineText := doc.GetLine(line)
	if character > len(lineText) {
		return nil
	}

	// 向前查找函数调用
	prefix := lineText[:character]

	// 找到最近的未闭合的 (
	parenCount := 0
	commaCount := 0
	funcEnd := -1

	for i := len(prefix) - 1; i >= 0; i-- {
		switch prefix[i] {
		case ')':
			parenCount++
		case '(':
			if parenCount > 0 {
				parenCount--
			} else {
				funcEnd = i
			}
		case ',':
			if parenCount == 0 {
				commaCount++
			}
		}
		if funcEnd >= 0 {
			break
		}
	}

	if funcEnd < 0 {
		return nil
	}

	// 提取函数名或方法名
	funcStart := funcEnd - 1
	for funcStart >= 0 && isWordChar(prefix[funcStart]) {
		funcStart--
	}
	funcStart++

	funcName := prefix[funcStart:funcEnd]
	if funcName == "" {
		return nil
	}

	// 检查是否是方法调用 ($obj->method 或 $obj.method)
	isMethodCall := false
	objectName := ""
	className := ""

	if funcStart > 0 {
		beforeFunc := prefix[:funcStart]
		// 检查 -> 或 .
		if strings.HasSuffix(strings.TrimRight(beforeFunc, " \t"), "->") ||
			strings.HasSuffix(strings.TrimRight(beforeFunc, " \t"), ".") {
			isMethodCall = true
			// 提取对象名
			sep := "->"
			if strings.HasSuffix(strings.TrimRight(beforeFunc, " \t"), ".") {
				sep = "."
			}
			idx := strings.LastIndex(beforeFunc, sep)
			if idx >= 0 {
				objPart := strings.TrimRight(beforeFunc[:idx], " \t")
				start := len(objPart) - 1
				for start >= 0 && (isWordChar(objPart[start]) || objPart[start] == '$') {
					start--
				}
				objectName = objPart[start+1:]
			}
		} else if strings.HasSuffix(strings.TrimRight(beforeFunc, " \t"), "::") {
			// 静态方法调用
			isMethodCall = true
			idx := strings.LastIndex(beforeFunc, "::")
			if idx >= 0 {
				classPart := strings.TrimRight(beforeFunc[:idx], " \t")
				start := len(classPart) - 1
				for start >= 0 && isWordChar(classPart[start]) {
					start--
				}
				className = classPart[start+1:]
			}
		}
	}

	symbols := doc.GetSymbols()
	astFile := doc.GetAST()

	// 如果是方法调用，尝试获取方法签名
	if isMethodCall {
		// 推断对象类型
		if objectName != "" && className == "" {
			if objectName == "$this" || objectName == "this" {
				// 当前类的方法
				if astFile != nil {
					for _, decl := range astFile.Declarations {
						if classDecl, ok := decl.(*ast.ClassDecl); ok {
							className = classDecl.Name.Name
							break
						}
					}
				}
			} else {
				// 推断变量类型
				varName := strings.TrimPrefix(objectName, "$")
				className = inferVariableType(astFile, varName)
			}
		}

		// 从类中查找方法签名
		if className != "" {
			sig := s.getMethodSignature(className, funcName, astFile, symbols)
			if sig != nil {
				return sig
			}
		}
	}

	// 查找普通函数签名
	if symbols == nil {
		return nil
	}

	fn := symbols.GetFunction(funcName)
	if fn == nil {
		return nil
	}

	// 构建签名信息
	var params []string
	var paramInfos []protocol.ParameterInformation
	for i, pt := range fn.ParamTypes {
		paramName := ""
		if i < len(fn.ParamNames) {
			paramName = "$" + fn.ParamNames[i]
		} else {
			paramName = "$arg" + string(rune('0'+i))
		}
		paramStr := pt + " " + paramName
		params = append(params, paramStr)
		paramInfos = append(paramInfos, protocol.ParameterInformation{
			Label: paramStr,
		})
	}

	sigLabel := funcName + "(" + strings.Join(params, ", ") + ")"
	if fn.ReturnType != "" && fn.ReturnType != "void" {
		sigLabel += ": " + fn.ReturnType
	}

	return &protocol.SignatureHelp{
		Signatures: []protocol.SignatureInformation{
			{
				Label:      sigLabel,
				Parameters: paramInfos,
			},
		},
		ActiveSignature: 0,
		ActiveParameter: uint32(commaCount),
	}
}

// getMethodSignature 获取方法签名
func (s *Server) getMethodSignature(className, methodName string, astFile *ast.File, symbols *compiler.SymbolTable) *protocol.SignatureHelp {
	// 先从符号表查找
	if symbols != nil {
		if methods, ok := symbols.ClassMethods[className]; ok {
			if sigs, ok := methods[methodName]; ok && len(sigs) > 0 {
				sig := sigs[0]
				var params []string
				var paramInfos []protocol.ParameterInformation

				for i, pt := range sig.ParamTypes {
					paramName := "$arg" + string(rune('0'+i))
					if i < len(sig.ParamNames) {
						paramName = "$" + sig.ParamNames[i]
					}
					paramStr := pt + " " + paramName
					params = append(params, paramStr)
					paramInfos = append(paramInfos, protocol.ParameterInformation{
						Label: paramStr,
					})
				}

				sigLabel := className + "::" + methodName + "(" + strings.Join(params, ", ") + ")"
				if sig.ReturnType != "" && sig.ReturnType != "void" {
					sigLabel += ": " + sig.ReturnType
				}

				return &protocol.SignatureHelp{
					Signatures: []protocol.SignatureInformation{
						{
							Label:      sigLabel,
							Parameters: paramInfos,
						},
					},
					ActiveSignature: 0,
					ActiveParameter: 0,
				}
			}
		}
	}

	// 从当前文件的AST查找
	if astFile != nil {
		for _, decl := range astFile.Declarations {
			if classDecl, ok := decl.(*ast.ClassDecl); ok && classDecl.Name.Name == className {
				for _, method := range classDecl.Methods {
					if method.Name.Name == methodName {
						return s.methodToSignatureHelp(className, method)
					}
				}
			}
		}
	}

	// 从工作区索引查找
	if s.workspace != nil {
		indexed := s.workspace.FindSymbolFile(className)
		if indexed != nil && indexed.AST != nil {
			for _, decl := range indexed.AST.Declarations {
				if classDecl, ok := decl.(*ast.ClassDecl); ok && classDecl.Name.Name == className {
					for _, method := range classDecl.Methods {
						if method.Name.Name == methodName {
							return s.methodToSignatureHelp(className, method)
						}
					}
				}
			}
		}
	}

	return nil
}

// methodToSignatureHelp 将方法转换为签名帮助
func (s *Server) methodToSignatureHelp(className string, method *ast.MethodDecl) *protocol.SignatureHelp {
	var params []string
	var paramInfos []protocol.ParameterInformation

	for _, param := range method.Parameters {
		paramStr := "$" + param.Name.Name
		if param.Type != nil {
			paramStr = typeNodeToString(param.Type) + " " + paramStr
		}
		params = append(params, paramStr)

		paramDoc := ""
		if param.Type != nil {
			paramDoc = "类型: " + typeNodeToString(param.Type)
		}
		paramInfos = append(paramInfos, protocol.ParameterInformation{
			Label:         paramStr,
			Documentation: paramDoc,
		})
	}

	sigLabel := className + "::" + method.Name.Name + "(" + strings.Join(params, ", ") + ")"
	if method.ReturnType != nil {
		sigLabel += ": " + typeNodeToString(method.ReturnType)
	}

	return &protocol.SignatureHelp{
		Signatures: []protocol.SignatureInformation{
			{
				Label:      sigLabel,
				Parameters: paramInfos,
			},
		},
		ActiveSignature: 0,
		ActiveParameter: 0,
	}
}
