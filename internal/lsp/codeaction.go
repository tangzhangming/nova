package lsp

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/tangzhangming/nova/internal/ast"
	"go.lsp.dev/protocol"
)

// handleCodeAction 处理代码操作请求
func (s *Server) handleCodeAction(id json.RawMessage, params json.RawMessage) {
	var p protocol.CodeActionParams
	if err := json.Unmarshal(params, &p); err != nil {
		s.sendError(id, -32700, "Parse error")
		return
	}

	docURI := string(p.TextDocument.URI)
	doc := s.documents.Get(docURI)
	if doc == nil {
		s.sendResult(id, []protocol.CodeAction{})
		return
	}

	var actions []protocol.CodeAction

	// 从诊断中生成快速修复
	for _, diag := range p.Context.Diagnostics {
		fixes := s.getQuickFixesForDiagnostic(doc, diag)
		actions = append(actions, fixes...)
	}

	// 添加源代码操作
	sourceActions := s.getSourceActions(doc, p.Range)
	actions = append(actions, sourceActions...)

	s.sendResult(id, actions)
}

// getQuickFixesForDiagnostic 根据诊断生成快速修复
func (s *Server) getQuickFixesForDiagnostic(doc *Document, diag protocol.Diagnostic) []protocol.CodeAction {
	var actions []protocol.CodeAction
	msg := diag.Message

	// 缺失的符号修复
	if fix := s.getMissingTokenFix(doc, diag, msg); fix != nil {
		actions = append(actions, *fix)
	}

	// 类型相关修复
	if fix := s.getTypeFix(doc, diag, msg); fix != nil {
		actions = append(actions, *fix)
	}

	// 未定义变量修复
	if fix := s.getUndefinedVarFix(doc, diag, msg); fix != nil {
		actions = append(actions, *fix)
	}

	// 未定义函数/方法修复
	if fix := s.getUndefinedFuncFix(doc, diag, msg); fix != nil {
		actions = append(actions, *fix)
	}

	return actions
}

// getMissingTokenFix 获取缺失符号的修复
func (s *Server) getMissingTokenFix(doc *Document, diag protocol.Diagnostic, msg string) *protocol.CodeAction {
	docURI := protocol.DocumentURI(doc.URI)

	// 匹配 "expected 'X'" 或 "expected X" 格式
	patterns := []struct {
		pattern string
		insert  string
		title   string
	}{
		{`expected '\)'`, ")", "添加缺失的 ')'"},
		{`expected '\('`, "(", "添加缺失的 '('"},
		{`expected '\]'`, "]", "添加缺失的 ']'"},
		{`expected '\['`, "[", "添加缺失的 '['"},
		{`expected '\}'`, "}", "添加缺失的 '}'"},
		{`expected '\{'`, "{", "添加缺失的 '{'"},
		{`expected ';'`, ";", "添加缺失的 ';'"},
		{`expected ':'`, ":", "添加缺失的 ':'"},
		{`expected '=>'`, " => ", "添加缺失的 '=>'"},
		{`expected ':='`, " := ", "添加缺失的 ':='"},
		{`expected '>'`, ">", "添加缺失的 '>'"},
	}

	for _, p := range patterns {
		matched, _ := regexp.MatchString(p.pattern, msg)
		if matched {
			return &protocol.CodeAction{
				Title: p.title,
				Kind:  protocol.QuickFix,
				Diagnostics: []protocol.Diagnostic{diag},
				Edit: &protocol.WorkspaceEdit{
					Changes: map[protocol.DocumentURI][]protocol.TextEdit{
						docURI: {
							{
								Range:   diag.Range,
								NewText: p.insert,
							},
						},
					},
				},
			}
		}
	}

	return nil
}

// getTypeFix 获取类型相关的修复
func (s *Server) getTypeFix(doc *Document, diag protocol.Diagnostic, msg string) *protocol.CodeAction {
	docURI := protocol.DocumentURI(doc.URI)

	// 缺失类型声明
	if strings.Contains(msg, "expected type") || strings.Contains(msg, "类型") {
		// 尝试获取变量名和建议的类型
		line := int(diag.Range.Start.Line)
		lineText := doc.GetLine(line)

		// 检查是否是变量声明，提供添加类型的建议
		if strings.Contains(lineText, ":=") {
			// 动态类型推断场景，建议添加显式类型
			return &protocol.CodeAction{
				Title:       "添加显式类型声明",
				Kind:        protocol.QuickFix,
				Diagnostics: []protocol.Diagnostic{diag},
				Edit: &protocol.WorkspaceEdit{
					Changes: map[protocol.DocumentURI][]protocol.TextEdit{
						docURI: {
							{
								Range: protocol.Range{
									Start: diag.Range.Start,
									End:   diag.Range.Start,
								},
								NewText: "dynamic ",
							},
						},
					},
				},
			}
		}
	}

	// void 类型不允许
	if strings.Contains(msg, "void") && strings.Contains(msg, "not allowed") {
		return &protocol.CodeAction{
			Title:       "将 void 改为有效类型",
			Kind:        protocol.QuickFix,
			Diagnostics: []protocol.Diagnostic{diag},
			// 这个需要用户手动选择类型，只提供提示
		}
	}

	return nil
}

// getUndefinedVarFix 获取未定义变量的修复
func (s *Server) getUndefinedVarFix(doc *Document, diag protocol.Diagnostic, msg string) *protocol.CodeAction {
	// 匹配未定义变量错误
	if !strings.Contains(msg, "undefined") && !strings.Contains(msg, "未定义") &&
		!strings.Contains(msg, "undeclared") && !strings.Contains(msg, "未声明") {
		return nil
	}

	// 提取变量名
	varName := extractVarName(msg)
	if varName == "" {
		return nil
	}

	docURI := protocol.DocumentURI(doc.URI)
	line := int(diag.Range.Start.Line)

	// 查找合适的声明位置（当前行之前）
	insertLine := line
	insertCol := 0

	// 找到行首的缩进
	lineText := doc.GetLine(line)
	indent := getIndent(lineText)

	// 生成变量声明
	declaration := indent + "$" + strings.TrimPrefix(varName, "$") + " := null\n"

	return &protocol.CodeAction{
		Title:       "声明变量 '" + varName + "'",
		Kind:        protocol.QuickFix,
		Diagnostics: []protocol.Diagnostic{diag},
		Edit: &protocol.WorkspaceEdit{
			Changes: map[protocol.DocumentURI][]protocol.TextEdit{
				docURI: {
					{
						Range: protocol.Range{
							Start: protocol.Position{Line: uint32(insertLine), Character: uint32(insertCol)},
							End:   protocol.Position{Line: uint32(insertLine), Character: uint32(insertCol)},
						},
						NewText: declaration,
					},
				},
			},
		},
	}
}

// getUndefinedFuncFix 获取未定义函数的修复
func (s *Server) getUndefinedFuncFix(doc *Document, diag protocol.Diagnostic, msg string) *protocol.CodeAction {
	// 匹配未定义函数/方法错误
	if !strings.Contains(msg, "function") && !strings.Contains(msg, "method") &&
		!strings.Contains(msg, "函数") && !strings.Contains(msg, "方法") {
		return nil
	}

	// 提取函数名
	funcName := extractFuncName(msg)
	if funcName == "" {
		return nil
	}

	docURI := protocol.DocumentURI(doc.URI)

	// 生成函数存根
	stub := "\n\nfunction " + funcName + "() {\n    // TODO: 实现此函数\n}\n"

	// 找到文件末尾
	lastLine := len(doc.Lines) - 1
	lastCol := len(doc.GetLine(lastLine))

	return &protocol.CodeAction{
		Title:       "创建函数 '" + funcName + "'",
		Kind:        protocol.QuickFix,
		Diagnostics: []protocol.Diagnostic{diag},
		Edit: &protocol.WorkspaceEdit{
			Changes: map[protocol.DocumentURI][]protocol.TextEdit{
				docURI: {
					{
						Range: protocol.Range{
							Start: protocol.Position{Line: uint32(lastLine), Character: uint32(lastCol)},
							End:   protocol.Position{Line: uint32(lastLine), Character: uint32(lastCol)},
						},
						NewText: stub,
					},
				},
			},
		},
	}
}

// getSourceActions 获取源代码操作
func (s *Server) getSourceActions(doc *Document, rang protocol.Range) []protocol.CodeAction {
	var actions []protocol.CodeAction

	// 组织 use 声明
	if organizeUse := s.getOrganizeUsesAction(doc); organizeUse != nil {
		actions = append(actions, *organizeUse)
	}

	// 移除未使用的变量（如果检测到）
	if removeUnused := s.getRemoveUnusedAction(doc, rang); removeUnused != nil {
		actions = append(actions, *removeUnused)
	}

	// 提取方法
	if extractMethod := s.getExtractMethodAction(doc, rang); extractMethod != nil {
		actions = append(actions, *extractMethod)
	}

	// 提取变量
	if extractVar := s.getExtractVariableAction(doc, rang); extractVar != nil {
		actions = append(actions, *extractVar)
	}

	// 生成构造函数
	if genConstructor := s.getGenerateConstructorAction(doc, rang); genConstructor != nil {
		actions = append(actions, *genConstructor)
	}

	// 生成Getter/Setter
	if genAccessors := s.getGenerateAccessorsAction(doc, rang); genAccessors != nil {
		actions = append(actions, genAccessors...)
	}

	// 实现接口
	if implInterface := s.getImplementInterfaceAction(doc, rang); implInterface != nil {
		actions = append(actions, *implInterface)
	}

	// 添加缺失的导入
	if addImport := s.getAddMissingImportAction(doc, rang); addImport != nil {
		actions = append(actions, *addImport)
	}

	return actions
}

// getExtractMethodAction 获取提取方法的操作
func (s *Server) getExtractMethodAction(doc *Document, rang protocol.Range) *protocol.CodeAction {
	// 检查是否选中了多行代码
	if rang.Start.Line == rang.End.Line && rang.Start.Character == rang.End.Character {
		return nil
	}

	// 检查选中的是否是有效的语句
	startLine := int(rang.Start.Line)
	endLine := int(rang.End.Line)

	if endLine-startLine < 1 {
		return nil
	}

	// 获取选中的代码
	var selectedLines []string
	for i := startLine; i <= endLine && i < len(doc.Lines); i++ {
		selectedLines = append(selectedLines, doc.Lines[i])
	}

	if len(selectedLines) == 0 {
		return nil
	}

	docURI := protocol.DocumentURI(doc.URI)

	// 获取缩进
	indent := getIndent(doc.GetLine(startLine))

	// 生成方法存根
	methodStub := "\n\n" + indent + "private function extractedMethod(): void {\n"
	for _, line := range selectedLines {
		methodStub += indent + "    " + strings.TrimLeft(line, " \t") + "\n"
	}
	methodStub += indent + "}\n"

	// 找到插入位置（当前类的末尾）
	insertLine := s.findClassEndLine(doc)
	if insertLine < 0 {
		return nil
	}

	return &protocol.CodeAction{
		Title: "提取方法 (Extract Method)",
		Kind:  protocol.RefactorExtract,
		Edit: &protocol.WorkspaceEdit{
			Changes: map[protocol.DocumentURI][]protocol.TextEdit{
				docURI: {
					// 替换选中区域为方法调用
					{
						Range: rang,
						NewText: indent + "$this->extractedMethod();",
					},
					// 添加新方法
					{
						Range: protocol.Range{
							Start: protocol.Position{Line: uint32(insertLine), Character: 0},
							End:   protocol.Position{Line: uint32(insertLine), Character: 0},
						},
						NewText: methodStub,
					},
				},
			},
		},
	}
}

// getExtractVariableAction 获取提取变量的操作
func (s *Server) getExtractVariableAction(doc *Document, rang protocol.Range) *protocol.CodeAction {
	// 检查是否选中了表达式
	if rang.Start.Line != rang.End.Line {
		return nil // 只支持单行表达式
	}

	line := int(rang.Start.Line)
	lineText := doc.GetLine(line)

	startChar := int(rang.Start.Character)
	endChar := int(rang.End.Character)

	if startChar >= endChar || endChar > len(lineText) {
		return nil
	}

	selectedText := lineText[startChar:endChar]
	if selectedText == "" || strings.TrimSpace(selectedText) == "" {
		return nil
	}

	// 检查是否是有效的表达式（简单检查）
	if strings.Contains(selectedText, "=") && !strings.Contains(selectedText, "==") {
		return nil // 不是表达式
	}

	docURI := protocol.DocumentURI(doc.URI)
	indent := getIndent(lineText)

	// 生成变量声明
	varDecl := indent + "$extractedVar := " + selectedText + "\n"

	return &protocol.CodeAction{
		Title: "提取变量 (Extract Variable)",
		Kind:  protocol.RefactorExtract,
		Edit: &protocol.WorkspaceEdit{
			Changes: map[protocol.DocumentURI][]protocol.TextEdit{
				docURI: {
					// 在当前行之前添加变量声明
					{
						Range: protocol.Range{
							Start: protocol.Position{Line: uint32(line), Character: 0},
							End:   protocol.Position{Line: uint32(line), Character: 0},
						},
						NewText: varDecl,
					},
					// 替换选中的表达式为变量
					{
						Range:   rang,
						NewText: "$extractedVar",
					},
				},
			},
		},
	}
}

// getGenerateConstructorAction 获取生成构造函数的操作
func (s *Server) getGenerateConstructorAction(doc *Document, rang protocol.Range) *protocol.CodeAction {
	astFile := doc.GetAST()
	if astFile == nil {
		return nil
	}

	// 查找当前位置所在的类
	line := int(rang.Start.Line) + 1 // AST 使用 1-based
	var targetClass *ast.ClassDecl

	for _, decl := range astFile.Declarations {
		if classDecl, ok := decl.(*ast.ClassDecl); ok {
			if classDecl.ClassToken.Pos.Line <= line && classDecl.RBrace.Pos.Line >= line {
				targetClass = classDecl
				break
			}
		}
	}

	if targetClass == nil {
		return nil
	}

	// 检查是否已有构造函数
	for _, method := range targetClass.Methods {
		if method.Name.Name == "__construct" {
			return nil
		}
	}

	// 如果没有属性，不生成构造函数
	if len(targetClass.Properties) == 0 {
		return nil
	}

	docURI := protocol.DocumentURI(doc.URI)

	// 生成构造函数
	var params []string
	var assignments []string
	for _, prop := range targetClass.Properties {
		if prop.Static {
			continue
		}
		paramType := "dynamic"
		if prop.Type != nil {
			paramType = typeNodeToString(prop.Type)
		}
		params = append(params, "$"+prop.Name.Name+": "+paramType)
		assignments = append(assignments, "        $this->"+prop.Name.Name+" = $"+prop.Name.Name+";")
	}

	if len(params) == 0 {
		return nil
	}

	constructor := "\n    public function __construct(" + strings.Join(params, ", ") + "): void {\n"
	constructor += strings.Join(assignments, "\n") + "\n"
	constructor += "    }\n"

	// 找到插入位置（类的第一个方法之前，或属性之后）
	insertLine := targetClass.RBrace.Pos.Line - 1

	return &protocol.CodeAction{
		Title: "生成构造函数 (Generate Constructor)",
		Kind:  protocol.SourceOrganizeImports,
		Edit: &protocol.WorkspaceEdit{
			Changes: map[protocol.DocumentURI][]protocol.TextEdit{
				docURI: {
					{
						Range: protocol.Range{
							Start: protocol.Position{Line: uint32(insertLine - 1), Character: 0},
							End:   protocol.Position{Line: uint32(insertLine - 1), Character: 0},
						},
						NewText: constructor,
					},
				},
			},
		},
	}
}

// getGenerateAccessorsAction 获取生成Getter/Setter的操作
func (s *Server) getGenerateAccessorsAction(doc *Document, rang protocol.Range) []protocol.CodeAction {
	var actions []protocol.CodeAction

	astFile := doc.GetAST()
	if astFile == nil {
		return actions
	}

	line := int(rang.Start.Line) + 1
	var targetClass *ast.ClassDecl
	var targetProp *ast.PropertyDecl

	for _, decl := range astFile.Declarations {
		if classDecl, ok := decl.(*ast.ClassDecl); ok {
			for _, prop := range classDecl.Properties {
				if prop.Name.Token.Pos.Line == line {
					targetClass = classDecl
					targetProp = prop
					break
				}
			}
		}
	}

	if targetClass == nil || targetProp == nil || targetProp.Static {
		return actions
	}

	docURI := protocol.DocumentURI(doc.URI)
	propName := targetProp.Name.Name
	propType := "dynamic"
	if targetProp.Type != nil {
		propType = typeNodeToString(targetProp.Type)
	}

	// 首字母大写
	capitalName := strings.ToUpper(propName[:1]) + propName[1:]

	// 检查是否已有 getter/setter
	hasGetter := false
	hasSetter := false
	for _, method := range targetClass.Methods {
		if method.Name.Name == "get"+capitalName {
			hasGetter = true
		}
		if method.Name.Name == "set"+capitalName {
			hasSetter = true
		}
	}

	insertLine := targetClass.RBrace.Pos.Line - 1

	// 生成 Getter
	if !hasGetter {
		getter := "\n    public function get" + capitalName + "(): " + propType + " {\n"
		getter += "        return $this->" + propName + ";\n"
		getter += "    }\n"

		actions = append(actions, protocol.CodeAction{
			Title: "生成 Getter: get" + capitalName + "()",
			Kind:  protocol.SourceOrganizeImports,
			Edit: &protocol.WorkspaceEdit{
				Changes: map[protocol.DocumentURI][]protocol.TextEdit{
					docURI: {
						{
							Range: protocol.Range{
								Start: protocol.Position{Line: uint32(insertLine - 1), Character: 0},
								End:   protocol.Position{Line: uint32(insertLine - 1), Character: 0},
							},
							NewText: getter,
						},
					},
				},
			},
		})
	}

	// 生成 Setter
	if !hasSetter {
		setter := "\n    public function set" + capitalName + "($" + propName + ": " + propType + "): void {\n"
		setter += "        $this->" + propName + " = $" + propName + ";\n"
		setter += "    }\n"

		actions = append(actions, protocol.CodeAction{
			Title: "生成 Setter: set" + capitalName + "()",
			Kind:  protocol.SourceOrganizeImports,
			Edit: &protocol.WorkspaceEdit{
				Changes: map[protocol.DocumentURI][]protocol.TextEdit{
					docURI: {
						{
							Range: protocol.Range{
								Start: protocol.Position{Line: uint32(insertLine - 1), Character: 0},
								End:   protocol.Position{Line: uint32(insertLine - 1), Character: 0},
							},
							NewText: setter,
						},
					},
				},
			},
		})
	}

	return actions
}

// getImplementInterfaceAction 获取实现接口的操作
func (s *Server) getImplementInterfaceAction(doc *Document, rang protocol.Range) *protocol.CodeAction {
	astFile := doc.GetAST()
	if astFile == nil {
		return nil
	}

	line := int(rang.Start.Line) + 1
	var targetClass *ast.ClassDecl

	for _, decl := range astFile.Declarations {
		if classDecl, ok := decl.(*ast.ClassDecl); ok {
			if classDecl.ClassToken.Pos.Line == line || classDecl.Name.Token.Pos.Line == line {
				targetClass = classDecl
				break
			}
		}
	}

	if targetClass == nil || len(targetClass.Implements) == 0 {
		return nil
	}

	// 收集需要实现的方法
	var missingMethods []string
	existingMethods := make(map[string]bool)
	for _, method := range targetClass.Methods {
		existingMethods[method.Name.Name] = true
	}

	for _, impl := range targetClass.Implements {
		implName := impl.String()
		if simpleType, ok := impl.(*ast.SimpleType); ok {
			implName = simpleType.Name
		}

		// 从工作区查找接口定义
		if s.workspace != nil {
			indexed := s.workspace.FindSymbolFile(implName)
			if indexed != nil && indexed.AST != nil {
				for _, decl := range indexed.AST.Declarations {
					if ifaceDecl, ok := decl.(*ast.InterfaceDecl); ok && ifaceDecl.Name.Name == implName {
						for _, method := range ifaceDecl.Methods {
							if !existingMethods[method.Name.Name] {
								missingMethods = append(missingMethods, formatMethodStub(method))
							}
						}
					}
				}
			}
		}

		// 从当前文件查找
		for _, decl := range astFile.Declarations {
			if ifaceDecl, ok := decl.(*ast.InterfaceDecl); ok && ifaceDecl.Name.Name == implName {
				for _, method := range ifaceDecl.Methods {
					if !existingMethods[method.Name.Name] {
						missingMethods = append(missingMethods, formatMethodStub(method))
					}
				}
			}
		}
	}

	if len(missingMethods) == 0 {
		return nil
	}

	docURI := protocol.DocumentURI(doc.URI)
	insertLine := targetClass.RBrace.Pos.Line - 1
	methodsCode := "\n" + strings.Join(missingMethods, "\n")

	return &protocol.CodeAction{
		Title: "实现接口方法 (Implement Interface Methods)",
		Kind:  protocol.QuickFix,
		Edit: &protocol.WorkspaceEdit{
			Changes: map[protocol.DocumentURI][]protocol.TextEdit{
				docURI: {
					{
						Range: protocol.Range{
							Start: protocol.Position{Line: uint32(insertLine - 1), Character: 0},
							End:   protocol.Position{Line: uint32(insertLine - 1), Character: 0},
						},
						NewText: methodsCode,
					},
				},
			},
		},
	}
}

// getAddMissingImportAction 获取添加缺失导入的操作
func (s *Server) getAddMissingImportAction(doc *Document, rang protocol.Range) *protocol.CodeAction {
	line := int(rang.Start.Line)
	lineText := doc.GetLine(line)

	// 查找可能的类名引用
	word := doc.GetWordAt(line, int(rang.Start.Character))
	if word == "" {
		return nil
	}

	// 检查是否已导入
	astFile := doc.GetAST()
	if astFile != nil {
		for _, use := range astFile.Uses {
			if use == nil {
				continue
			}
			// 检查路径的最后部分是否匹配
			parts := strings.Split(use.Path, ".")
			if len(parts) > 0 && parts[len(parts)-1] == word {
				return nil // 已导入
			}
			if use.Alias != nil && use.Alias.Name == word {
				return nil // 已通过别名导入
			}
		}
	}

	// 从工作区查找类定义
	if s.workspace == nil {
		return nil
	}

	indexed := s.workspace.FindSymbolFile(word)
	if indexed == nil || indexed.AST == nil {
		return nil
	}

	// 检查是否确实定义了该类/接口/枚举
	var importPath string
	for _, decl := range indexed.AST.Declarations {
		switch d := decl.(type) {
		case *ast.ClassDecl:
			if d.Name.Name == word {
				importPath = getImportPath(indexed.Path)
			}
		case *ast.InterfaceDecl:
			if d.Name.Name == word {
				importPath = getImportPath(indexed.Path)
			}
		case *ast.EnumDecl:
			if d.Name.Name == word {
				importPath = getImportPath(indexed.Path)
			}
		}
	}

	if importPath == "" {
		return nil
	}

	// 检查错误消息是否包含 "undefined" 或类似内容
	if !strings.Contains(strings.ToLower(lineText), word) {
		return nil
	}

	docURI := protocol.DocumentURI(doc.URI)

	// 找到插入位置（文件开头或已有use之后）
	insertLine := 0
	if astFile != nil && len(astFile.Uses) > 0 {
		lastUse := astFile.Uses[len(astFile.Uses)-1]
		insertLine = lastUse.UseToken.Pos.Line
	}

	importStmt := "use \"" + importPath + "\"\n"

	return &protocol.CodeAction{
		Title: "导入 " + word + " (Add Import)",
		Kind:  protocol.QuickFix,
		Edit: &protocol.WorkspaceEdit{
			Changes: map[protocol.DocumentURI][]protocol.TextEdit{
				docURI: {
					{
						Range: protocol.Range{
							Start: protocol.Position{Line: uint32(insertLine), Character: 0},
							End:   protocol.Position{Line: uint32(insertLine), Character: 0},
						},
						NewText: importStmt,
					},
				},
			},
		},
	}
}

// findClassEndLine 查找当前类的结束行
func (s *Server) findClassEndLine(doc *Document) int {
	astFile := doc.GetAST()
	if astFile == nil {
		return -1
	}

	for _, decl := range astFile.Declarations {
		if classDecl, ok := decl.(*ast.ClassDecl); ok {
			return classDecl.RBrace.Pos.Line - 1
		}
	}
	return -1
}

// formatMethodStub 格式化方法存根
func formatMethodStub(method *ast.MethodDecl) string {
	var params []string
	for _, param := range method.Parameters {
		paramStr := "$" + param.Name.Name
		if param.Type != nil {
			paramStr = typeNodeToString(param.Type) + " " + paramStr
		}
		params = append(params, paramStr)
	}

	returnType := "void"
	if method.ReturnType != nil {
		returnType = typeNodeToString(method.ReturnType)
	}

	return "    public function " + method.Name.Name + "(" + strings.Join(params, ", ") + "): " + returnType + " {\n        // TODO: 实现此方法\n    }\n"
}

// getImportPath 从文件路径获取导入路径
func getImportPath(filePath string) string {
	// 简化处理：使用文件名作为导入路径
	base := strings.TrimSuffix(filePath, ".sola")
	// 将路径分隔符转换为点
	base = strings.ReplaceAll(base, "\\", "/")
	parts := strings.Split(base, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return base
}


// getOrganizeUsesAction 获取组织 use 声明的操作
func (s *Server) getOrganizeUsesAction(doc *Document) *protocol.CodeAction {
	astFile := doc.GetAST()
	if astFile == nil || len(astFile.Uses) < 2 {
		return nil
	}

	// 收集所有 use 声明
	var useLines []struct {
		line    int
		endLine int
		text    string
		path    string
	}

	for _, use := range astFile.Uses {
		if use == nil {
			continue
		}
		startLine := use.UseToken.Pos.Line - 1 // 转换为 0-based
		text := doc.GetLine(startLine)

		// 提取路径用于排序
		path := use.Path

		useLines = append(useLines, struct {
			line    int
			endLine int
			text    string
			path    string
		}{
			line:    startLine,
			endLine: startLine,
			text:    text,
			path:    path,
		})
	}

	if len(useLines) < 2 {
		return nil
	}

	// 检查是否需要排序
	needSort := false
	for i := 1; i < len(useLines); i++ {
		if useLines[i].path < useLines[i-1].path {
			needSort = true
			break
		}
	}

	if !needSort {
		return nil
	}

	// 排序 use 声明
	sortedTexts := make([]string, len(useLines))
	copy(sortedTexts, extractTexts(useLines))
	sortStrings(sortedTexts)

	// 生成编辑
	docURI := protocol.DocumentURI(doc.URI)
	var edits []protocol.TextEdit

	for i, ul := range useLines {
		if ul.text != sortedTexts[i] {
			edits = append(edits, protocol.TextEdit{
				Range: protocol.Range{
					Start: protocol.Position{Line: uint32(ul.line), Character: 0},
					End:   protocol.Position{Line: uint32(ul.line), Character: uint32(len(ul.text))},
				},
				NewText: sortedTexts[i],
			})
		}
	}

	if len(edits) == 0 {
		return nil
	}

	return &protocol.CodeAction{
		Title: "整理 use 声明",
		Kind:  protocol.SourceOrganizeImports,
		Edit: &protocol.WorkspaceEdit{
			Changes: map[protocol.DocumentURI][]protocol.TextEdit{
				docURI: edits,
			},
		},
	}
}

// getRemoveUnusedAction 获取移除未使用变量的操作
func (s *Server) getRemoveUnusedAction(doc *Document, rang protocol.Range) *protocol.CodeAction {
	// 检查选中范围是否是变量声明
	line := int(rang.Start.Line)
	lineText := doc.GetLine(line)

	// 简单检测：如果是变量声明行
	if !strings.Contains(lineText, ":=") && !strings.Contains(lineText, "$") {
		return nil
	}

	// 检查变量是否在后续被使用（简化实现）
	// 这里只是提供操作选项，实际判断需要更复杂的分析
	docURI := protocol.DocumentURI(doc.URI)

	return &protocol.CodeAction{
		Title: "移除未使用的变量",
		Kind:  protocol.QuickFix,
		Edit: &protocol.WorkspaceEdit{
			Changes: map[protocol.DocumentURI][]protocol.TextEdit{
				docURI: {
					{
						Range: protocol.Range{
							Start: protocol.Position{Line: uint32(line), Character: 0},
							End:   protocol.Position{Line: uint32(line + 1), Character: 0},
						},
						NewText: "",
					},
				},
			},
		},
	}
}

// 辅助函数

// extractVarName 从错误消息中提取变量名
func extractVarName(msg string) string {
	// 尝试匹配 'varName' 或 "varName" 或 $varName
	patterns := []string{
		`'(\$?\w+)'`,
		`"(\$?\w+)"`,
		`variable (\$?\w+)`,
		`变量 (\$?\w+)`,
	}

	for _, p := range patterns {
		re := regexp.MustCompile(p)
		matches := re.FindStringSubmatch(msg)
		if len(matches) > 1 {
			return matches[1]
		}
	}
	return ""
}

// extractFuncName 从错误消息中提取函数名
func extractFuncName(msg string) string {
	patterns := []string{
		`function '(\w+)'`,
		`函数 '(\w+)'`,
		`method '(\w+)'`,
		`方法 '(\w+)'`,
		`'(\w+)' is not defined`,
	}

	for _, p := range patterns {
		re := regexp.MustCompile(p)
		matches := re.FindStringSubmatch(msg)
		if len(matches) > 1 {
			return matches[1]
		}
	}
	return ""
}

// getIndent 获取行的缩进
func getIndent(line string) string {
	indent := ""
	for _, c := range line {
		if c == ' ' || c == '\t' {
			indent += string(c)
		} else {
			break
		}
	}
	return indent
}

// extractTexts 从 use 行信息中提取文本
func extractTexts(useLines []struct {
	line    int
	endLine int
	text    string
	path    string
}) []string {
	texts := make([]string, len(useLines))
	for i, ul := range useLines {
		texts[i] = ul.text
	}
	return texts
}

// sortStrings 简单的字符串排序（冒泡排序）
func sortStrings(strs []string) {
	for i := 0; i < len(strs)-1; i++ {
		for j := 0; j < len(strs)-1-i; j++ {
			if strs[j] > strs[j+1] {
				strs[j], strs[j+1] = strs[j+1], strs[j]
			}
		}
	}
}
