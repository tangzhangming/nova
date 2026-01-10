package lsp2

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tangzhangming/nova/internal/ast"
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
	doc := s.docManager.Get(docURI)
	if doc == nil {
		s.sendResult(id, nil)
		return
	}

	line := int(p.Position.Line)
	character := int(p.Position.Character)

	// 获取悬停信息
	hover := s.getHoverInfo(doc, line, character)
	s.sendResult(id, hover)
}

// getHoverInfo 获取悬停信息
func (s *Server) getHoverInfo(doc *Document, line, character int) *protocol.Hover {
	if line < 0 || line >= len(doc.Lines) {
		return nil
	}

	lineText := doc.Lines[line]
	if character > len(lineText) {
		return nil
	}

	// 获取当前位置的单词
	word, _, _ := GetWordAt(lineText, character)
	if word == "" {
		return nil
	}

	s.logger.Debug("Hover request for '%s' at %d:%d", word, line, character)

	// 检查是否是静态访问 (ClassName::methodName)
	if className, isStatic := CheckStaticCall(lineText, character); isStatic {
		if content := s.getStaticMemberHover(doc, className, word); content != "" {
			return &protocol.Hover{
				Contents: protocol.MarkupContent{
					Kind:  protocol.Markdown,
					Value: content,
				},
			}
		}
	}

	// 检查是否是实例访问 ($obj->method)
	if varName, isInstance := CheckInstanceCall(lineText, character); isInstance {
		if content := s.getInstanceMemberHover(doc, varName, word, line); content != "" {
			return &protocol.Hover{
				Contents: protocol.MarkupContent{
					Kind:  protocol.Markdown,
					Value: content,
				},
			}
		}
	}

	// 检查是否是变量（以 $ 开头）
	isVariable := false
	if character > 0 && character <= len(lineText) {
		start := character
		for start > 0 && isWordCharByte(lineText[start-1]) {
			start--
		}
		if start > 0 && lineText[start-1] == '$' {
			isVariable = true
		}
	}

	astFile := doc.GetAST()
	if astFile == nil {
		return nil
	}

	var content string

	if isVariable {
		// 变量悬停
		content = s.getVariableHover(doc, word, line)
	} else {
		// 类/接口/枚举悬停
		content = s.getSymbolHover(doc, word)
	}

	if content == "" {
		return nil
	}

	return &protocol.Hover{
		Contents: protocol.MarkupContent{
			Kind:  protocol.Markdown,
			Value: content,
		},
	}
}

// getStaticMemberHover 获取静态成员悬停信息
func (s *Server) getStaticMemberHover(doc *Document, className, memberName string) string {
	// 在当前文档中查找类
	astFile := doc.GetAST()
	if astFile != nil {
		if content := findStaticMemberInAST(astFile, className, memberName, doc.Lines); content != "" {
			return content
		}
	}

	// 在导入的文件中查找
	imports := s.importResolver.ResolveImports(doc)
	for _, imported := range imports {
		if imported.AST != nil {
			// 读取导入文件的源代码以获取注释
			lines := readFileLines(imported.Path)
			if content := findStaticMemberInAST(imported.AST, className, memberName, lines); content != "" {
				return content
			}
		}
	}

	return ""
}

// findStaticMemberInAST 在AST中查找静态成员
func findStaticMemberInAST(astFile *ast.File, className, memberName string, lines []string) string {
	for _, decl := range astFile.Declarations {
		switch d := decl.(type) {
		case *ast.ClassDecl:
			if d.Name.Name == className {
				// 查找静态方法
				for _, method := range d.Methods {
					if method.Static && method.Name.Name == memberName {
						comment := extractDocComment(lines, method.Name.Token.Pos.Line)
						return formatMethodHover(className, method, comment)
					}
				}
				// 查找静态属性
				for _, prop := range d.Properties {
					if prop.Static && prop.Name.Name == memberName {
						comment := extractDocComment(lines, prop.Name.Token.Pos.Line)
						return formatPropertyHover(className, prop, comment)
					}
				}
				// 查找常量
				for _, c := range d.Constants {
					if c.Name.Name == memberName {
						return fmt.Sprintf("```sola\nconst %s\n```\n\n常量来自: %s", memberName, className)
					}
				}
			}
		case *ast.EnumDecl:
			if d.Name.Name == className {
				for _, c := range d.Cases {
					if c.Name.Name == memberName {
						return fmt.Sprintf("```sola\n%s::%s\n```\n\n枚举值", className, memberName)
					}
				}
			}
		}
	}
	return ""
}

// getInstanceMemberHover 获取实例成员悬停信息
func (s *Server) getInstanceMemberHover(doc *Document, varName, memberName string, line int) string {
	// 推断变量类型
	className := s.definitionProvider.inferVariableType(doc, varName, line)
	if className == "" || className == "dynamic" {
		return ""
	}

	// 在类中查找方法或属性
	astFile := doc.GetAST()
	if astFile != nil {
		if content := findInstanceMemberInAST(astFile, className, memberName, doc.Lines); content != "" {
			return content
		}
	}

	// 在导入的文件中查找
	imports := s.importResolver.ResolveImports(doc)
	for _, imported := range imports {
		if imported.AST != nil {
			// 读取导入文件的源代码以获取注释
			lines := readFileLines(imported.Path)
			if content := findInstanceMemberInAST(imported.AST, className, memberName, lines); content != "" {
				return content
			}
		}
	}

	return ""
}

// findInstanceMemberInAST 在AST中查找实例成员
func findInstanceMemberInAST(astFile *ast.File, className, memberName string, lines []string) string {
	for _, decl := range astFile.Declarations {
		if classDecl, ok := decl.(*ast.ClassDecl); ok && classDecl.Name.Name == className {
			// 查找方法
			for _, method := range classDecl.Methods {
				if method.Name.Name == memberName {
					comment := extractDocComment(lines, method.Name.Token.Pos.Line)
					return formatMethodHover(className, method, comment)
				}
			}
			// 查找属性
			for _, prop := range classDecl.Properties {
				if prop.Name.Name == memberName {
					comment := extractDocComment(lines, prop.Name.Token.Pos.Line)
					return formatPropertyHover(className, prop, comment)
				}
			}
		}
	}
	return ""
}

// getVariableHover 获取变量悬停信息
func (s *Server) getVariableHover(doc *Document, varName string, line int) string {
	astFile := doc.GetAST()
	if astFile == nil {
		return ""
	}

	// 在语句中查找变量声明
	for _, stmt := range astFile.Statements {
		if info := findVariableInStmt(stmt, varName); info != "" {
			return info
		}
	}

	// 在类中查找
	for _, decl := range astFile.Declarations {
		if classDecl, ok := decl.(*ast.ClassDecl); ok {
			// 查找属性
			for _, prop := range classDecl.Properties {
				if prop.Name.Name == varName {
					comment := extractDocComment(doc.Lines, prop.Name.Token.Pos.Line)
					return formatPropertyHover(classDecl.Name.Name, prop, comment)
				}
			}
			// 查找方法参数
			for _, method := range classDecl.Methods {
				for _, param := range method.Parameters {
					if param.Name.Name == varName {
						typeName := "dynamic"
						if param.Type != nil {
							typeName = typeNodeToString(param.Type)
						}
						return fmt.Sprintf("```sola\n%s $%s\n```\n\n方法参数", typeName, varName)
					}
				}
				// 查找方法体内的变量
				if method.Body != nil {
					if info := findVariableInStmt(method.Body, varName); info != "" {
						return info
					}
				}
			}
		}
	}

	return ""
}

// findVariableInStmt 在语句中查找变量声明
func findVariableInStmt(stmt ast.Statement, varName string) string {
	switch s := stmt.(type) {
	case *ast.VarDeclStmt:
		if s.Name.Name == varName {
			typeName := "dynamic"
			if s.Type != nil {
				typeName = typeNodeToString(s.Type)
			} else if s.Value != nil {
				typeName = inferTypeFromExpr(s.Value)
				if typeName == "" {
					typeName = "dynamic"
				}
			}
			return fmt.Sprintf("```sola\n%s $%s\n```\n\n变量声明", typeName, varName)
		}
	case *ast.BlockStmt:
		for _, inner := range s.Statements {
			if info := findVariableInStmt(inner, varName); info != "" {
				return info
			}
		}
	case *ast.IfStmt:
		if info := findVariableInStmt(s.Then, varName); info != "" {
			return info
		}
		if s.Else != nil {
			if info := findVariableInStmt(s.Else, varName); info != "" {
				return info
			}
		}
	case *ast.ForStmt:
		if s.Init != nil {
			if info := findVariableInStmt(s.Init, varName); info != "" {
				return info
			}
		}
		if s.Body != nil {
			if info := findVariableInStmt(s.Body, varName); info != "" {
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
			if info := findVariableInStmt(s.Body, varName); info != "" {
				return info
			}
		}
	}
	return ""
}

// getSymbolHover 获取符号（类/接口/枚举）悬停信息
func (s *Server) getSymbolHover(doc *Document, name string) string {
	astFile := doc.GetAST()
	if astFile != nil {
		if content := findSymbolInAST(astFile, name, doc.Lines); content != "" {
			return content
		}
	}

	// 在导入的文件中查找
	imports := s.importResolver.ResolveImports(doc)
	for _, imported := range imports {
		if imported.AST != nil {
			// 读取导入文件的源代码以获取注释
			lines := readFileLines(imported.Path)
			if content := findSymbolInAST(imported.AST, name, lines); content != "" {
				return content
			}
		}
	}

	return ""
}

// findSymbolInAST 在AST中查找符号
func findSymbolInAST(astFile *ast.File, name string, lines []string) string {
	for _, decl := range astFile.Declarations {
		switch d := decl.(type) {
		case *ast.ClassDecl:
			if d.Name.Name == name {
				comment := extractDocComment(lines, d.Name.Token.Pos.Line)
				return formatClassHover(d, comment)
			}
			// 也检查方法名
			for _, method := range d.Methods {
				if method.Name.Name == name {
					comment := extractDocComment(lines, method.Name.Token.Pos.Line)
					return formatMethodHover(d.Name.Name, method, comment)
				}
			}
		case *ast.InterfaceDecl:
			if d.Name.Name == name {
				comment := extractDocComment(lines, d.Name.Token.Pos.Line)
				return formatInterfaceHover(d, comment)
			}
		case *ast.EnumDecl:
			if d.Name.Name == name {
				comment := extractDocComment(lines, d.Name.Token.Pos.Line)
				return formatEnumHover(d, comment)
			}
		}
	}
	return ""
}

// formatClassHover 格式化类悬停信息
func formatClassHover(d *ast.ClassDecl, comment string) string {
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

	// 添加注释
	if comment != "" {
		sb.WriteString("\n\n---\n\n")
		sb.WriteString(comment)
	}

	// 添加方法数量信息
	if len(d.Methods) > 0 || len(d.Properties) > 0 {
		sb.WriteString("\n\n")
		if len(d.Methods) > 0 {
			sb.WriteString(fmt.Sprintf("方法: %d个", len(d.Methods)))
		}
		if len(d.Properties) > 0 {
			if len(d.Methods) > 0 {
				sb.WriteString(" | ")
			}
			sb.WriteString(fmt.Sprintf("属性: %d个", len(d.Properties)))
		}
	}

	return sb.String()
}

// formatInterfaceHover 格式化接口悬停信息
func formatInterfaceHover(d *ast.InterfaceDecl, comment string) string {
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

	// 添加注释
	if comment != "" {
		sb.WriteString("\n\n---\n\n")
		sb.WriteString(comment)
	}

	if len(d.Methods) > 0 {
		sb.WriteString(fmt.Sprintf("\n\n方法: %d个", len(d.Methods)))
	}

	return sb.String()
}

// formatEnumHover 格式化枚举悬停信息
func formatEnumHover(d *ast.EnumDecl, comment string) string {
	var sb strings.Builder
	sb.WriteString("```sola\n")
	sb.WriteString("enum ")
	sb.WriteString(d.Name.Name)

	if d.Type != nil {
		sb.WriteString(": ")
		sb.WriteString(typeNodeToString(d.Type))
	}

	sb.WriteString("\n```")

	// 添加注释
	if comment != "" {
		sb.WriteString("\n\n---\n\n")
		sb.WriteString(comment)
	}

	sb.WriteString("\n\n枚举值: ")

	for i, c := range d.Cases {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(c.Name.Name)
	}

	return sb.String()
}

// formatMethodHover 格式化方法悬停信息
func formatMethodHover(className string, m *ast.MethodDecl, comment string) string {
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

	sb.WriteString("\n```")

	// 添加注释
	if comment != "" {
		sb.WriteString("\n\n---\n\n")
		sb.WriteString(comment)
	}

	sb.WriteString("\n\n方法来自: ")
	sb.WriteString(className)

	return sb.String()
}

// formatPropertyHover 格式化属性悬停信息
func formatPropertyHover(className string, p *ast.PropertyDecl, comment string) string {
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

	sb.WriteString("\n```")

	// 添加注释
	if comment != "" {
		sb.WriteString("\n\n---\n\n")
		sb.WriteString(comment)
	}

	sb.WriteString("\n\n属性来自: ")
	sb.WriteString(className)

	return sb.String()
}

// isWordCharByte 判断字节是否是单词字符
func isWordCharByte(c byte) bool {
	return (c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') ||
		c == '_'
}
