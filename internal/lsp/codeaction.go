package lsp

import (
	"encoding/json"
	"regexp"
	"strings"

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

	return actions
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
