package lsp

import (
	"encoding/json"

	"github.com/tangzhangming/nova/internal/formatter"
	"go.lsp.dev/protocol"
)

// handleFormatting 处理文档格式化请求
func (s *Server) handleFormatting(id json.RawMessage, params json.RawMessage) {
	var p protocol.DocumentFormattingParams
	if err := json.Unmarshal(params, &p); err != nil {
		s.sendError(id, -32700, "Parse error")
		return
	}

	docURI := string(p.TextDocument.URI)
	doc := s.documents.Get(docURI)
	if doc == nil {
		s.sendResult(id, []protocol.TextEdit{})
		return
	}

	// 获取格式化选项
	options := formatter.DefaultOptions()

	// 从 LSP 选项转换
	if p.Options.TabSize > 0 {
		options.IndentSize = int(p.Options.TabSize)
	}
	if p.Options.InsertSpaces {
		options.IndentStyle = "spaces"
	} else {
		options.IndentStyle = "tabs"
	}

	// 执行格式化
	filename := uriToPath(docURI)
	formatted, err := formatter.Format(doc.Content, filename, options)
	if err != nil {
		s.log("Format error: %v", err)
		// 格式化失败时返回空编辑
		s.sendResult(id, []protocol.TextEdit{})
		return
	}

	// 如果内容没有变化，返回空编辑
	if formatted == doc.Content {
		s.sendResult(id, []protocol.TextEdit{})
		return
	}

	// 计算需要替换的范围（整个文档）
	lines := doc.Lines
	lastLine := len(lines) - 1
	if lastLine < 0 {
		lastLine = 0
	}
	lastChar := 0
	if lastLine < len(lines) {
		lastChar = len(lines[lastLine])
	}

	edit := protocol.TextEdit{
		Range: protocol.Range{
			Start: protocol.Position{
				Line:      0,
				Character: 0,
			},
			End: protocol.Position{
				Line:      uint32(lastLine),
				Character: uint32(lastChar),
			},
		},
		NewText: formatted,
	}

	s.sendResult(id, []protocol.TextEdit{edit})
}

// handleRangeFormatting 处理范围格式化请求
func (s *Server) handleRangeFormatting(id json.RawMessage, params json.RawMessage) {
	var p protocol.DocumentRangeFormattingParams
	if err := json.Unmarshal(params, &p); err != nil {
		s.sendError(id, -32700, "Parse error")
		return
	}

	docURI := string(p.TextDocument.URI)
	doc := s.documents.Get(docURI)
	if doc == nil {
		s.sendResult(id, []protocol.TextEdit{})
		return
	}

	// 获取格式化选项
	options := formatter.DefaultOptions()
	if p.Options.TabSize > 0 {
		options.IndentSize = int(p.Options.TabSize)
	}
	if p.Options.InsertSpaces {
		options.IndentStyle = "spaces"
	} else {
		options.IndentStyle = "tabs"
	}

	// 扩展范围到完整的语句/块
	startLine, endLine := expandRangeToStatements(doc, int(p.Range.Start.Line), int(p.Range.End.Line))

	// 提取选定范围的代码
	rangeText := extractRange(doc, startLine, endLine)
	if rangeText == "" {
		s.sendResult(id, []protocol.TextEdit{})
		return
	}

	// 计算原始缩进级别（保持相对缩进）
	baseIndent := getBaseIndent(doc, startLine)

	// 格式化选定范围的代码
	// 为了正确格式化部分代码，我们需要包装它
	filename := uriToPath(docURI)
	formatted, err := formatter.FormatPartial(rangeText, filename, options, baseIndent)
	if err != nil {
		// 如果部分格式化失败，尝试整体格式化后提取范围
		s.log("Partial format error: %v, falling back to full format", err)
		fullFormatted, fullErr := formatter.Format(doc.Content, filename, options)
		if fullErr != nil {
			s.sendResult(id, []protocol.TextEdit{})
			return
		}
		// 从完整格式化的文档中提取对应范围
		formattedDoc := &Document{Content: fullFormatted}
		formattedDoc.Lines = splitLines(fullFormatted)
		formatted = extractRange(formattedDoc, startLine, endLine)
	}

	// 如果内容没有变化，返回空编辑
	if formatted == rangeText {
		s.sendResult(id, []protocol.TextEdit{})
		return
	}

	// 计算范围的结束字符
	endChar := 0
	if endLine < len(doc.Lines) {
		endChar = len(doc.Lines[endLine])
	}

	edit := protocol.TextEdit{
		Range: protocol.Range{
			Start: protocol.Position{
				Line:      uint32(startLine),
				Character: 0,
			},
			End: protocol.Position{
				Line:      uint32(endLine),
				Character: uint32(endChar),
			},
		},
		NewText: formatted,
	}

	s.sendResult(id, []protocol.TextEdit{edit})
}

// expandRangeToStatements 扩展范围到完整的语句/块边界
func expandRangeToStatements(doc *Document, startLine, endLine int) (int, int) {
	// 向前扩展：找到语句开始
	for startLine > 0 {
		line := doc.GetLine(startLine - 1)
		trimmed := trimLeft(line)
		// 如果上一行以 { 结尾或者是空行，停止
		if trimmed == "" || endsWithBrace(trimmed) || isStatementStart(trimmed) {
			break
		}
		// 如果当前行是语句继续（如链式调用），继续向前
		currentLine := doc.GetLine(startLine)
		if isStatementContinuation(currentLine) {
			startLine--
			continue
		}
		break
	}

	// 向后扩展：找到语句结束
	for endLine < len(doc.Lines)-1 {
		line := doc.GetLine(endLine)
		trimmed := trimRight(line)
		// 如果当前行以 ; 或 } 结尾，停止
		if endsWithSemicolon(trimmed) || endsWithBrace(trimmed) {
			break
		}
		// 如果下一行是语句继续，继续向后
		nextLine := doc.GetLine(endLine + 1)
		if isStatementContinuation(nextLine) {
			endLine++
			continue
		}
		break
	}

	return startLine, endLine
}

// extractRange 提取指定行范围的文本
func extractRange(doc *Document, startLine, endLine int) string {
	if startLine < 0 {
		startLine = 0
	}
	if endLine >= len(doc.Lines) {
		endLine = len(doc.Lines) - 1
	}
	if startLine > endLine {
		return ""
	}

	var result string
	for i := startLine; i <= endLine; i++ {
		if i > startLine {
			result += "\n"
		}
		result += doc.Lines[i]
	}
	return result
}

// getBaseIndent 获取基础缩进级别
func getBaseIndent(doc *Document, line int) int {
	if line < 0 || line >= len(doc.Lines) {
		return 0
	}
	lineText := doc.Lines[line]
	indent := 0
	for _, ch := range lineText {
		if ch == ' ' {
			indent++
		} else if ch == '\t' {
			indent += 4 // 假设 tab = 4 spaces
		} else {
			break
		}
	}
	return indent
}

// trimLeft 去除左侧空白
func trimLeft(s string) string {
	for i, ch := range s {
		if ch != ' ' && ch != '\t' {
			return s[i:]
		}
	}
	return ""
}

// trimRight 去除右侧空白
func trimRight(s string) string {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] != ' ' && s[i] != '\t' {
			return s[:i+1]
		}
	}
	return ""
}

// endsWithSemicolon 检查是否以分号结尾
func endsWithSemicolon(s string) bool {
	return len(s) > 0 && s[len(s)-1] == ';'
}

// endsWithBrace 检查是否以大括号结尾
func endsWithBrace(s string) bool {
	return len(s) > 0 && (s[len(s)-1] == '{' || s[len(s)-1] == '}')
}

// isStatementStart 检查是否是语句开始
func isStatementStart(s string) bool {
	keywords := []string{"class ", "interface ", "enum ", "function ", "if ", "for ", "foreach ", "while ", "switch ", "try ", "use ", "namespace "}
	for _, kw := range keywords {
		if len(s) >= len(kw) && s[:len(kw)] == kw {
			return true
		}
	}
	return false
}

// isStatementContinuation 检查是否是语句继续（如链式调用）
func isStatementContinuation(s string) bool {
	trimmed := trimLeft(s)
	if len(trimmed) == 0 {
		return false
	}
	// 以 -> 或 . 开头表示链式调用
	if len(trimmed) >= 2 && trimmed[:2] == "->" {
		return true
	}
	if trimmed[0] == '.' || trimmed[0] == '?' {
		return true
	}
	return false
}
