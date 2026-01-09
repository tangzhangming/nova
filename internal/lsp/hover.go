package lsp

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tangzhangming/nova/internal/ast"
	"github.com/tangzhangming/nova/internal/compiler"
	"go.lsp.dev/protocol"
)

// handleHover 处理悬停请求
func (s *Server) handleHover(id json.RawMessage, params json.RawMessage) {
	var p protocol.HoverParams
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

	// 获取悬停信息
	hover := s.getHoverInfo(doc, line, character)
	if hover == nil {
		s.sendResult(id, nil)
		return
	}

	s.sendResult(id, hover)
}

// getHoverInfo 获取悬停信息
func (s *Server) getHoverInfo(doc *Document, line, character int) *protocol.Hover {
	// 获取当前行文本
	lineText := doc.GetLine(line)
	if lineText == "" || character > len(lineText) {
		return nil
	}

	// 获取当前位置的单词
	word := doc.GetWordAt(line, character)
	if word == "" {
		return nil
	}

	// 获取 AST 和符号表
	astFile := doc.GetAST()
	symbols := doc.GetSymbols()

	if astFile == nil {
		return nil
	}

	var content string
	var hoverRange *protocol.Range

	// 检查是否是静态访问 (ClassName::methodName)
	staticInfo := s.checkStaticAccess(lineText, character, astFile, symbols)
	if staticInfo != "" {
		return &protocol.Hover{
			Contents: protocol.MarkupContent{
				Kind:  protocol.Markdown,
				Value: staticInfo,
			},
			Range: hoverRange,
		}
	}

	// 检查是否是变量（以 $ 开头）
	isVariable := false
	if character > 0 && character <= len(lineText) {
		// 向前查找 $
		start := character
		for start > 0 && isWordChar(lineText[start-1]) {
			start--
		}
		if start > 0 && lineText[start-1] == '$' {
			word = "$" + word
			isVariable = true
		}
	}

	// 查找符号信息
	if isVariable {
		// 变量
		varName := word[1:]
		content = s.getVariableHoverInfo(astFile, varName, line+1) // AST 行号从 1 开始
	} else {
		// 尝试查找函数
		if symbols != nil {
			if fn := symbols.GetFunction(word); fn != nil {
				content = formatFunctionSignature(fn)
			}
		}
		if content == "" {
			// 尝试查找类
			if symbols != nil {
				if classSig := symbols.GetClassSignature(word); classSig != nil {
					content = formatClassSignature(classSig)
				}
			}
		}
		if content == "" {
			// 尝试在 AST 中查找声明
			content = s.findDeclarationInfo(astFile, word)
		}
	}

	if content == "" {
		return nil
	}

	return &protocol.Hover{
		Contents: protocol.MarkupContent{
			Kind:  protocol.Markdown,
			Value: content,
		},
		Range: hoverRange,
	}
}

// checkStaticAccess 检查是否是静态访问并返回hover信息
func (s *Server) checkStaticAccess(lineText string, character int, astFile *ast.File, symbols *compiler.SymbolTable) string {
	// 查找 :: 的位置
	doubleColonIdx := strings.LastIndex(lineText[:min(character+20, len(lineText))], "::")
	if doubleColonIdx < 0 || doubleColonIdx >= character {
		return ""
	}

	// 检查光标是否在 :: 后面的部分
	afterDoubleColon := lineText[doubleColonIdx+2:]

	// 提取方法/属性名
	memberStart := 0
	memberEnd := 0
	for i, c := range afterDoubleColon {
		if isWordChar(byte(c)) || c == '$' {
			if memberStart == 0 && c != '$' {
				memberStart = i
			}
			memberEnd = i + 1
		} else if memberStart > 0 {
			break
		}
	}

	if memberEnd <= memberStart {
		return ""
	}

	memberName := afterDoubleColon[memberStart:memberEnd]

	// 提取类名
	classEnd := doubleColonIdx
	classStart := classEnd
	for classStart > 0 && isWordChar(lineText[classStart-1]) {
		classStart--
	}

	if classStart >= classEnd {
		return ""
	}

	className := lineText[classStart:classEnd]

	// 查找方法信息
	// 先从 AST 中查找
	if astFile != nil {
		for _, decl := range astFile.Declarations {
			if classDecl, ok := decl.(*ast.ClassDecl); ok && classDecl.Name.Name == className {
				// 查找静态方法
				for _, method := range classDecl.Methods {
					if method.Static && method.Name.Name == memberName {
						return formatMethodHover(className, method)
					}
				}
				// 查找静态属性
				for _, prop := range classDecl.Properties {
					if prop.Static && prop.Name.Name == strings.TrimPrefix(memberName, "$") {
						return formatPropertyHover(className, prop)
					}
				}
				// 查找常量
				for _, c := range classDecl.Constants {
					if c.Name.Name == memberName {
						return fmt.Sprintf("```sola\nconst %s %s\n```\n\n常量来自: %s", typeNodeToString(c.Type), memberName, className)
					}
				}
			}
		}
	}

	// 从符号表中查找
	if symbols != nil {
		// 查找方法
		if methods, ok := symbols.ClassMethods[className]; ok {
			if sigs, ok := methods[memberName]; ok && len(sigs) > 0 {
				sig := sigs[0]
				if sig.IsStatic {
					return formatMethodSigHover(className, sig)
				}
			}
		}
		// 查找属性
		propName := strings.TrimPrefix(memberName, "$")
		if props, ok := symbols.ClassProperties[className]; ok {
			if sig, ok := props[propName]; ok && sig.IsStatic {
				return fmt.Sprintf("```sola\nstatic %s $%s\n```\n\n属性来自: %s", sig.Type, propName, className)
			}
		}
	}

	return ""
}

// formatMethodSigHover 格式化方法签名的hover信息
func formatMethodSigHover(className string, sig *compiler.MethodSignature) string {
	var sb strings.Builder
	sb.WriteString("```sola\n")

	if sig.IsStatic {
		sb.WriteString("static ")
	}
	sb.WriteString("function ")
	sb.WriteString(sig.MethodName)
	sb.WriteString("(")

	for i, pt := range sig.ParamTypes {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(pt)
		if i < len(sig.ParamNames) {
			sb.WriteString(" $")
			sb.WriteString(sig.ParamNames[i])
		}
	}

	sb.WriteString(")")

	if sig.ReturnType != "" && sig.ReturnType != "void" {
		sb.WriteString(": ")
		sb.WriteString(sig.ReturnType)
	}

	sb.WriteString("\n```\n\n方法来自: ")
	sb.WriteString(className)

	return sb.String()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// getVariableHoverInfo 获取变量悬停信息
func (s *Server) getVariableHoverInfo(file *ast.File, varName string, line int) string {
	// 首先遍历语句查找局部变量声明（优先级更高）
	for _, stmt := range file.Statements {
		if info := findVariableInStatement(stmt, varName, line); info != "" {
			return info
		}
	}

	// 遍历类方法查找局部变量
	for _, decl := range file.Declarations {
		if classDecl, ok := decl.(*ast.ClassDecl); ok {
			for _, method := range classDecl.Methods {
				// 检查方法参数
				for _, param := range method.Parameters {
					if param.Name.Name == varName {
						typeName := "dynamic"
						if param.Type != nil {
							typeName = typeNodeToString(param.Type)
						}
						return fmt.Sprintf("```sola\n%s $%s\n```\n\n方法参数", typeName, varName)
					}
				}
				// 检查方法体内的变量
				if method.Body != nil {
					if info := findVariableInStatement(method.Body, varName, line); info != "" {
						return info
					}
				}
			}
		}
	}

	// 最后才查找类属性（只有在没找到局部变量时）
	for _, decl := range file.Declarations {
		if classDecl, ok := decl.(*ast.ClassDecl); ok {
			for _, prop := range classDecl.Properties {
				if prop.Name.Name == varName {
					return formatPropertyHover(classDecl.Name.Name, prop)
				}
			}
		}
	}

	return ""
}

// findVariableInStatement 在语句中查找变量
func findVariableInStatement(stmt ast.Statement, varName string, line int) string {
	switch s := stmt.(type) {
	case *ast.VarDeclStmt:
		if s.Name.Name == varName {
			typeName := ""
			if s.Type != nil {
				typeName = typeNodeToString(s.Type)
			} else if s.Value != nil {
				// 从初始化表达式推断类型
				typeName = inferTypeFromExprForHover(s.Value)
			}
			if typeName == "" {
				typeName = "dynamic"
			}
			declType := "变量声明"
			if s.Operator.Literal == ":=" {
				declType = "短变量声明"
			}
			return fmt.Sprintf("```sola\n%s $%s\n```\n\n%s", typeName, varName, declType)
		}
	case *ast.MultiVarDeclStmt:
		for _, v := range s.Names {
			if v.Name == varName {
				return fmt.Sprintf("```sola\n$%s\n```\n\n多变量声明", varName)
			}
		}
	case *ast.BlockStmt:
		for _, inner := range s.Statements {
			if info := findVariableInStatement(inner, varName, line); info != "" {
				return info
			}
		}
	case *ast.IfStmt:
		if info := findVariableInStatement(s.Then, varName, line); info != "" {
			return info
		}
		if s.Else != nil {
			if info := findVariableInStatement(s.Else, varName, line); info != "" {
				return info
			}
		}
	case *ast.ForStmt:
		if s.Init != nil {
			if info := findVariableInStatement(s.Init, varName, line); info != "" {
				return info
			}
		}
		if s.Body != nil {
			if info := findVariableInStatement(s.Body, varName, line); info != "" {
				return info
			}
		}
	case *ast.ForeachStmt:
		if s.Key != nil && s.Key.Name == varName {
			// 尝试推断key的类型
			keyType := "dynamic"
			if s.Iterable != nil {
				iterType := inferTypeFromExprForHover(s.Iterable)
				if strings.HasPrefix(iterType, "map[") {
					// map[K]V -> K
					if idx := strings.Index(iterType, "]"); idx > 4 {
						keyType = iterType[4:idx]
					}
				} else if strings.HasSuffix(iterType, "[]") {
					keyType = "int"
				}
			}
			return fmt.Sprintf("```sola\n%s $%s\n```\n\nforeach 键变量", keyType, varName)
		}
		if s.Value != nil && s.Value.Name == varName {
			// 尝试推断value的类型
			valueType := "dynamic"
			if s.Iterable != nil {
				iterType := inferTypeFromExprForHover(s.Iterable)
				if strings.HasPrefix(iterType, "map[") {
					// map[K]V -> V
					if idx := strings.Index(iterType, "]"); idx > 0 && idx+1 < len(iterType) {
						valueType = iterType[idx+1:]
					}
				} else if strings.HasSuffix(iterType, "[]") {
					valueType = iterType[:len(iterType)-2]
				}
			}
			return fmt.Sprintf("```sola\n%s $%s\n```\n\nforeach 值变量", valueType, varName)
		}
		if s.Body != nil {
			if info := findVariableInStatement(s.Body, varName, line); info != "" {
				return info
			}
		}
	}
	return ""
}

// inferTypeFromExprForHover 从表达式推断类型（用于hover显示）
// 注意：这是 inferTypeFromExpr 的别名，保持一致性
func inferTypeFromExprForHover(expr ast.Expression) string {
	return inferTypeFromExpr(expr)
}

// findDeclarationInfo 在 AST 中查找声明信息
func (s *Server) findDeclarationInfo(file *ast.File, name string) string {
	for _, decl := range file.Declarations {
		switch d := decl.(type) {
		case *ast.ClassDecl:
			if d.Name.Name == name {
				return formatClassDeclHover(d)
			}
			// 检查方法
			for _, method := range d.Methods {
				if method.Name.Name == name {
					return formatMethodHover(d.Name.Name, method)
				}
			}
		case *ast.InterfaceDecl:
			if d.Name.Name == name {
				return formatInterfaceDeclHover(d)
			}
		case *ast.EnumDecl:
			if d.Name.Name == name {
				return formatEnumDeclHover(d)
			}
		case *ast.TypeAliasDecl:
			if d.Name.Name == name {
				return fmt.Sprintf("```sola\ntype %s = %s\n```\n\n类型别名",
					name, typeNodeToString(d.AliasType))
			}
		case *ast.NewTypeDecl:
			if d.Name.Name == name {
				return fmt.Sprintf("```sola\ntype %s %s\n```\n\n新类型定义",
					name, typeNodeToString(d.BaseType))
			}
		}
	}
	return ""
}

// formatFunctionSignature 格式化函数签名
func formatFunctionSignature(fn *compiler.FunctionSignature) string {
	var params []string
	for i, pt := range fn.ParamTypes {
		paramName := fmt.Sprintf("$arg%d", i)
		if i < len(fn.ParamNames) && fn.ParamNames[i] != "" {
			paramName = "$" + fn.ParamNames[i]
		}
		params = append(params, fmt.Sprintf("%s %s", pt, paramName))
	}

	signature := fmt.Sprintf("function %s(%s)", fn.Name, strings.Join(params, ", "))
	if fn.ReturnType != "" && fn.ReturnType != "void" {
		signature += ": " + fn.ReturnType
	}

	return fmt.Sprintf("```sola\n%s\n```\n\n内置函数", signature)
}

// formatClassSignature 格式化类签名
func formatClassSignature(sig *compiler.ClassSignature) string {
	var typeParams []string
	for _, tp := range sig.TypeParams {
		param := tp.Name
		if tp.ExtendsType != "" {
			param += " extends " + tp.ExtendsType
		}
		typeParams = append(typeParams, param)
	}

	signature := "class " + sig.Name
	if len(typeParams) > 0 {
		signature += "<" + strings.Join(typeParams, ", ") + ">"
	}

	return fmt.Sprintf("```sola\n%s\n```\n\n泛型类", signature)
}

// formatClassDeclHover 格式化类声明悬停信息
func formatClassDeclHover(d *ast.ClassDecl) string {
	var sb strings.Builder
	sb.WriteString("```sola\n")

	if d.Abstract {
		sb.WriteString("abstract ")
	}
	if d.Final {
		sb.WriteString("final ")
	}
	sb.WriteString("class ")
	sb.WriteString(d.Name.Name)

	if len(d.TypeParams) > 0 {
		sb.WriteString("<")
		for i, tp := range d.TypeParams {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(tp.Name.Name)
			if tp.Constraint != nil {
				sb.WriteString(" extends ")
				sb.WriteString(typeNodeToString(tp.Constraint))
			}
		}
		sb.WriteString(">")
	}

	if d.Extends != nil {
		sb.WriteString(" extends ")
		sb.WriteString(d.Extends.Name)
	}

	if len(d.Implements) > 0 {
		sb.WriteString(" implements ")
		for i, impl := range d.Implements {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(typeNodeToString(impl))
		}
	}

	sb.WriteString("\n```")
	return sb.String()
}

// formatInterfaceDeclHover 格式化接口声明悬停信息
func formatInterfaceDeclHover(d *ast.InterfaceDecl) string {
	var sb strings.Builder
	sb.WriteString("```sola\n")
	sb.WriteString("interface ")
	sb.WriteString(d.Name.Name)

	if len(d.TypeParams) > 0 {
		sb.WriteString("<")
		for i, tp := range d.TypeParams {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(tp.Name.Name)
		}
		sb.WriteString(">")
	}

	sb.WriteString("\n```")
	return sb.String()
}

// formatEnumDeclHover 格式化枚举声明悬停信息
func formatEnumDeclHover(d *ast.EnumDecl) string {
	var sb strings.Builder
	sb.WriteString("```sola\n")
	sb.WriteString("enum ")
	sb.WriteString(d.Name.Name)

	if d.Type != nil {
		sb.WriteString(": ")
		sb.WriteString(typeNodeToString(d.Type))
	}

	sb.WriteString("\n```\n\n枚举值: ")

	for i, c := range d.Cases {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(c.Name.Name)
	}

	return sb.String()
}

// formatMethodHover 格式化方法悬停信息
func formatMethodHover(className string, m *ast.MethodDecl) string {
	var sb strings.Builder
	sb.WriteString("```sola\n")

	if m.Static {
		sb.WriteString("static ")
	}
	sb.WriteString("function ")
	sb.WriteString(m.Name.Name)
	sb.WriteString("(")

	for i, param := range m.Parameters {
		if i > 0 {
			sb.WriteString(", ")
		}
		if param.Type != nil {
			sb.WriteString(typeNodeToString(param.Type))
			sb.WriteString(" ")
		}
		sb.WriteString("$")
		sb.WriteString(param.Name.Name)
	}

	sb.WriteString(")")

	if m.ReturnType != nil {
		sb.WriteString(": ")
		sb.WriteString(typeNodeToString(m.ReturnType))
	}

	sb.WriteString("\n```\n\n方法来自: ")
	sb.WriteString(className)

	return sb.String()
}

// formatPropertyHover 格式化属性悬停信息
func formatPropertyHover(className string, p *ast.PropertyDecl) string {
	var sb strings.Builder
	sb.WriteString("```sola\n")

	if p.Static {
		sb.WriteString("static ")
	}
	if p.Type != nil {
		sb.WriteString(typeNodeToString(p.Type))
		sb.WriteString(" ")
	}
	sb.WriteString("$")
	sb.WriteString(p.Name.Name)

	sb.WriteString("\n```\n\n属性来自: ")
	sb.WriteString(className)

	return sb.String()
}

// typeNodeToString 将类型节点转换为字符串
func typeNodeToString(t ast.TypeNode) string {
	if t == nil {
		return "dynamic"
	}

	switch typ := t.(type) {
	case *ast.SimpleType:
		return typ.Name
	case *ast.ClassType:
		return typ.Name.Literal
	case *ast.ArrayType:
		elemType := typeNodeToString(typ.ElementType)
		return elemType + "[]"
	case *ast.MapType:
		keyType := typeNodeToString(typ.KeyType)
		valueType := typeNodeToString(typ.ValueType)
		return "map[" + keyType + "]" + valueType
	case *ast.NullableType:
		inner := typeNodeToString(typ.Inner)
		return "?" + inner
	case *ast.UnionType:
		var parts []string
		for _, t := range typ.Types {
			parts = append(parts, typeNodeToString(t))
		}
		return strings.Join(parts, "|")
	case *ast.TupleType:
		var parts []string
		for _, t := range typ.Types {
			parts = append(parts, typeNodeToString(t))
		}
		return "(" + strings.Join(parts, ", ") + ")"
	case *ast.NullType:
		return "null"
	case *ast.FuncType:
		var params []string
		for _, p := range typ.Params {
			params = append(params, typeNodeToString(p))
		}
		ret := "void"
		if typ.ReturnType != nil {
			ret = typeNodeToString(typ.ReturnType)
		}
		return "func(" + strings.Join(params, ", ") + "): " + ret
	case *ast.GenericType:
		base := typeNodeToString(typ.BaseType)
		var args []string
		for _, arg := range typ.TypeArgs {
			args = append(args, typeNodeToString(arg))
		}
		return base + "<" + strings.Join(args, ", ") + ">"
	default:
		return "dynamic"
	}
}
