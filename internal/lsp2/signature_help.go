package lsp2

import (
	"encoding/json"
	"strings"

	"github.com/tangzhangming/nova/internal/ast"
	"go.lsp.dev/protocol"
)

// handleSignatureHelp 处理签名帮助请求
func (s *Server) handleSignatureHelp(id json.RawMessage, params json.RawMessage) {
	var p protocol.SignatureHelpParams
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

	// 获取签名帮助
	help := s.getSignatureHelp(doc, line, character)
	s.sendResult(id, help)
}

// getSignatureHelp 获取签名帮助
func (s *Server) getSignatureHelp(doc *Document, line, character int) *protocol.SignatureHelp {
	if line < 0 || line >= len(doc.Lines) {
		return nil
	}

	lineText := doc.Lines[line]
	if character > len(lineText) {
		character = len(lineText)
	}

	// 获取光标前的文本
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
	for funcStart >= 0 && isWordCharByte(prefix[funcStart]) {
		funcStart--
	}
	funcStart++

	funcName := prefix[funcStart:funcEnd]
	if funcName == "" {
		return nil
	}

	s.logger.Debug("SignatureHelp for '%s' (param %d)", funcName, commaCount)

	// 检查是否是方法调用
	var className string
	if funcStart > 0 {
		beforeFunc := prefix[:funcStart]
		trimmed := strings.TrimRight(beforeFunc, " \t")

		// 检查 ->
		if strings.HasSuffix(trimmed, "->") {
			className = s.inferClassFromArrow(doc, trimmed[:len(trimmed)-2], line)
		} else if strings.HasSuffix(trimmed, "::") {
			// 检查 ::
			className = extractClassName(trimmed[:len(trimmed)-2])
		}
	}

	// 获取方法签名
	if className != "" {
		return s.getMethodSignatureHelp(doc, className, funcName, commaCount)
	}

	// 普通函数调用 - 检查当前文件中的函数定义
	return nil // 暂不支持全局函数
}

// inferClassFromArrow 从 -> 前的表达式推断类名
func (s *Server) inferClassFromArrow(doc *Document, expr string, line int) string {
	expr = strings.TrimRight(expr, " \t")

	// 如果是 $this
	if strings.HasSuffix(expr, "$this") || strings.HasSuffix(expr, "this") {
		return s.getCurrentClassName(doc)
	}

	// 提取变量名
	varEnd := len(expr)
	varStart := varEnd - 1
	for varStart >= 0 && (isWordCharByte(expr[varStart]) || expr[varStart] == '$') {
		varStart--
	}
	varStart++

	varName := expr[varStart:varEnd]
	if strings.HasPrefix(varName, "$") {
		varName = varName[1:]
	}

	if varName == "" {
		return ""
	}

	// 推断变量类型
	return s.definitionProvider.inferVariableType(doc, varName, line)
}

// extractClassName 从 :: 前的表达式提取类名
func extractClassName(expr string) string {
	expr = strings.TrimRight(expr, " \t")

	// 从后向前提取类名
	end := len(expr)
	start := end - 1
	for start >= 0 && isWordCharByte(expr[start]) {
		start--
	}
	start++

	return expr[start:end]
}

// getCurrentClassName 获取当前类名
func (s *Server) getCurrentClassName(doc *Document) string {
	astFile := doc.GetAST()
	if astFile == nil {
		return ""
	}

	for _, decl := range astFile.Declarations {
		if classDecl, ok := decl.(*ast.ClassDecl); ok {
			return classDecl.Name.Name
		}
	}

	return ""
}

// getMethodSignatureHelp 获取方法签名帮助
func (s *Server) getMethodSignatureHelp(doc *Document, className, methodName string, activeParam int) *protocol.SignatureHelp {
	// 在当前文档中查找
	astFile := doc.GetAST()
	if astFile != nil {
		if help := findMethodSignatureInAST(astFile, className, methodName, activeParam); help != nil {
			return help
		}
	}

	// 在导入的文件中查找
	imports := s.importResolver.ResolveImports(doc)
	for _, imported := range imports {
		if imported.AST != nil {
			if help := findMethodSignatureInAST(imported.AST, className, methodName, activeParam); help != nil {
				return help
			}
		}
	}

	return nil
}

// findMethodSignatureInAST 在AST中查找方法签名
func findMethodSignatureInAST(astFile *ast.File, className, methodName string, activeParam int) *protocol.SignatureHelp {
	for _, decl := range astFile.Declarations {
		if classDecl, ok := decl.(*ast.ClassDecl); ok && classDecl.Name.Name == className {
			for _, method := range classDecl.Methods {
				if method.Name.Name == methodName {
					return buildSignatureHelp(className, method, activeParam)
				}
			}
		}
	}
	return nil
}

// buildSignatureHelp 构建签名帮助
func buildSignatureHelp(className string, method *ast.MethodDecl, activeParam int) *protocol.SignatureHelp {
	var params []string
	var paramInfos []protocol.ParameterInformation

	for _, param := range method.Parameters {
		paramStr := "$" + param.Name.Name
		if param.Type != nil {
			paramStr = typeNodeToString(param.Type) + " " + paramStr
		}
		params = append(params, paramStr)
		paramInfos = append(paramInfos, protocol.ParameterInformation{
			Label: paramStr,
		})
	}

	// 构建签名标签
	sigLabel := className + "::" + method.Name.Name + "(" + strings.Join(params, ", ") + ")"
	if method.ReturnType != nil {
		sigLabel += ": " + typeNodeToString(method.ReturnType)
	}

	// 确保 activeParam 在有效范围内
	if activeParam >= len(paramInfos) {
		activeParam = len(paramInfos) - 1
	}
	if activeParam < 0 {
		activeParam = 0
	}

	return &protocol.SignatureHelp{
		Signatures: []protocol.SignatureInformation{
			{
				Label:      sigLabel,
				Parameters: paramInfos,
			},
		},
		ActiveSignature: 0,
		ActiveParameter: uint32(activeParam),
	}
}
