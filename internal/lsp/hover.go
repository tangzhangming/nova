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
	// 获取当前位置的单词
	word := doc.GetWordAt(line, character)
	if word == "" {
		return nil
	}

	// 检查是否是变量（以 $ 开头）
	lineText := doc.GetLine(line)
	if character > 0 && character <= len(lineText) {
		// 向前查找 $
		start := character
		for start > 0 && isWordChar(lineText[start-1]) {
			start--
		}
		if start > 0 && lineText[start-1] == '$' {
			word = "$" + word
		}
	}

	// 获取 AST 和符号表
	astFile := doc.GetAST()
	symbols := doc.GetSymbols()

	if astFile == nil || symbols == nil {
		return nil
	}

	var content string
	var hoverRange *protocol.Range

	// 查找符号信息
	if strings.HasPrefix(word, "$") {
		// 变量
		varName := word[1:]
		content = s.getVariableHoverInfo(astFile, varName, line+1) // AST 行号从 1 开始
	} else {
		// 尝试查找函数
		if fn := symbols.GetFunction(word); fn != nil {
			content = formatFunctionSignature(fn)
		} else {
			// 尝试查找类
			if classSig := symbols.GetClassSignature(word); classSig != nil {
				content = formatClassSignature(classSig)
			} else {
				// 尝试在 AST 中查找声明
				content = s.findDeclarationInfo(astFile, word)
			}
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

// getVariableHoverInfo 获取变量悬停信息
func (s *Server) getVariableHoverInfo(file *ast.File, varName string, line int) string {
	// 遍历语句查找变量声明
	for _, stmt := range file.Statements {
		if info := findVariableInStatement(stmt, varName, line); info != "" {
			return info
		}
	}

	// 遍历类声明查找属性
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
			typeName := "dynamic"
			if s.Type != nil {
				typeName = typeNodeToString(s.Type)
			}
			return fmt.Sprintf("```sola\n%s $%s\n```\n\n变量声明", typeName, varName)
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
			return fmt.Sprintf("```sola\n$%s\n```\n\nforeach 键变量", varName)
		}
		if s.Value != nil && s.Value.Name == varName {
			return fmt.Sprintf("```sola\n$%s\n```\n\nforeach 值变量", varName)
		}
		if s.Body != nil {
			if info := findVariableInStatement(s.Body, varName, line); info != "" {
				return info
			}
		}
	}
	return ""
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
