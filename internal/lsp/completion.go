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

	// 检查是否是静态访问 ClassName::
	if strings.HasSuffix(prefix, "::") {
		idx := strings.LastIndex(prefix, "::")
		if idx > 0 {
			classPart := strings.TrimRight(prefix[:idx], " \t")
			start := len(classPart) - 1
			for start >= 0 && isWordChar(classPart[start]) {
				start--
			}
			className := classPart[start+1:]
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

	// 检查是否在类型位置（如参数类型、返回类型）
	if strings.HasSuffix(trimmed, ":") || strings.HasSuffix(trimmed, "extends ") ||
		strings.HasSuffix(trimmed, "implements ") {
		return completionContext{Type: contextType}
	}

	return completionContext{Type: contextGeneral}
}

// getMemberCompletions 获取成员补全（属性和方法）
func (s *Server) getMemberCompletions(doc *Document, objectName string) []protocol.CompletionItem {
	var items []protocol.CompletionItem

	symbols := doc.GetSymbols()
	if symbols == nil {
		return items
	}

	// 如果是 $this，查找当前类的成员
	if objectName == "$this" || objectName == "this" {
		astFile := doc.GetAST()
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

	// 对于其他对象，尝试从符号表获取类型信息
	// 这里简化处理，返回所有已知类的公共成员
	for className, methods := range symbols.ClassMethods {
		for methodName, sigs := range methods {
			if len(sigs) > 0 {
				sig := sigs[0]
				if !sig.IsStatic {
					items = append(items, protocol.CompletionItem{
						Label:      methodName,
						Kind:       protocol.CompletionItemKindMethod,
						Detail:     className + "::" + methodName,
						InsertText: methodName + "()",
					})
				}
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

// getStaticCompletions 获取静态成员补全
func (s *Server) getStaticCompletions(doc *Document, className string) []protocol.CompletionItem {
	var items []protocol.CompletionItem

	symbols := doc.GetSymbols()
	if symbols == nil {
		return items
	}

	// 查找类的静态方法
	if methods, ok := symbols.ClassMethods[className]; ok {
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
	if props, ok := symbols.ClassProperties[className]; ok {
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
	if values := symbols.GetEnumValues(className); len(values) > 0 {
		for _, val := range values {
			items = append(items, protocol.CompletionItem{
				Label:  val,
				Kind:   protocol.CompletionItemKindEnumMember,
				Detail: className + "::" + val,
			})
		}
	}

	return items
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

	symbols := doc.GetSymbols()
	if symbols == nil {
		return items
	}

	// 从符号表获取所有类
	for className := range symbols.ClassMethods {
		items = append(items, protocol.CompletionItem{
			Label: className,
			Kind:  protocol.CompletionItemKindClass,
		})
	}

	// 从符号表获取所有接口
	for interfaceName := range symbols.InterfaceSigs {
		items = append(items, protocol.CompletionItem{
			Label: interfaceName,
			Kind:  protocol.CompletionItemKindInterface,
		})
	}

	// 从当前文件的 AST 获取类和接口
	astFile := doc.GetAST()
	if astFile != nil {
		for _, decl := range astFile.Declarations {
			switch d := decl.(type) {
			case *ast.ClassDecl:
				items = append(items, protocol.CompletionItem{
					Label: d.Name.Name,
					Kind:  protocol.CompletionItemKindClass,
				})
			case *ast.InterfaceDecl:
				items = append(items, protocol.CompletionItem{
					Label: d.Name.Name,
					Kind:  protocol.CompletionItemKindInterface,
				})
			case *ast.EnumDecl:
				items = append(items, protocol.CompletionItem{
					Label: d.Name.Name,
					Kind:  protocol.CompletionItemKindEnum,
				})
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

	// 提取函数名
	funcStart := funcEnd - 1
	for funcStart >= 0 && isWordChar(prefix[funcStart]) {
		funcStart--
	}
	funcStart++

	funcName := prefix[funcStart:funcEnd]
	if funcName == "" {
		return nil
	}

	// 查找函数签名
	symbols := doc.GetSymbols()
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
