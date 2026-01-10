package lsp

import (
	"encoding/json"

	"github.com/tangzhangming/nova/internal/ast"
	"go.lsp.dev/protocol"
)

// SelectionRange 选择范围
type SelectionRange struct {
	Range  protocol.Range   `json:"range"`
	Parent *SelectionRange  `json:"parent,omitempty"`
}

// SelectionRangeParams 选择范围参数
type SelectionRangeParams struct {
	TextDocument protocol.TextDocumentIdentifier `json:"textDocument"`
	Positions    []protocol.Position             `json:"positions"`
}

// handleSelectionRange 处理选择范围请求
func (s *Server) handleSelectionRange(id json.RawMessage, params json.RawMessage) {
	var p SelectionRangeParams
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

	astFile := doc.GetAST()
	if astFile == nil {
		s.sendResult(id, nil)
		return
	}

	var ranges []*SelectionRange
	for _, pos := range p.Positions {
		selRange := s.computeSelectionRange(astFile, doc, int(pos.Line), int(pos.Character))
		ranges = append(ranges, selRange)
	}

	s.sendResult(id, ranges)
}

// computeSelectionRange 计算选择范围
func (s *Server) computeSelectionRange(file *ast.File, doc *Document, line, character int) *SelectionRange {
	// 从最内层开始构建选择范围链
	var ranges []protocol.Range

	// 1. 当前单词
	word, startCol, endCol := doc.GetWordRangeAt(line, character)
	if word != "" {
		ranges = append(ranges, protocol.Range{
			Start: protocol.Position{Line: uint32(line), Character: uint32(startCol)},
			End:   protocol.Position{Line: uint32(line), Character: uint32(endCol)},
		})
	}

	// 2. 当前行
	lineText := doc.GetLine(line)
	if len(lineText) > 0 {
		ranges = append(ranges, protocol.Range{
			Start: protocol.Position{Line: uint32(line), Character: 0},
			End:   protocol.Position{Line: uint32(line), Character: uint32(len(lineText))},
		})
	}

	// 3. 查找包含位置的AST节点
	astRanges := findContainingRanges(file, line+1, character+1) // AST uses 1-based
	for _, r := range astRanges {
		ranges = append(ranges, r)
	}

	// 4. 整个文档
	if len(doc.Lines) > 0 {
		lastLine := len(doc.Lines) - 1
		lastChar := len(doc.Lines[lastLine])
		ranges = append(ranges, protocol.Range{
			Start: protocol.Position{Line: 0, Character: 0},
			End:   protocol.Position{Line: uint32(lastLine), Character: uint32(lastChar)},
		})
	}

	// 去重并构建链
	ranges = deduplicateRanges(ranges)

	// 构建选择范围链（从内到外）
	return buildSelectionRangeChain(ranges)
}

// findContainingRanges 查找包含指定位置的AST节点范围
func findContainingRanges(file *ast.File, line, col int) []protocol.Range {
	var ranges []protocol.Range

	// 检查声明
	for _, decl := range file.Declarations {
		ranges = append(ranges, findDeclRanges(decl, line, col)...)
	}

	// 检查语句
	for _, stmt := range file.Statements {
		ranges = append(ranges, findStmtRanges(stmt, line, col)...)
	}

	return ranges
}

// findDeclRanges 查找声明中包含位置的范围
func findDeclRanges(decl ast.Declaration, line, col int) []protocol.Range {
	var ranges []protocol.Range

	switch d := decl.(type) {
	case *ast.ClassDecl:
		// 整个类
		classRange := protocol.Range{
			Start: protocol.Position{
				Line:      uint32(d.ClassToken.Pos.Line - 1),
				Character: uint32(d.ClassToken.Pos.Column - 1),
			},
			End: protocol.Position{
				Line:      uint32(d.RBrace.Pos.Line - 1),
				Character: uint32(d.RBrace.Pos.Column),
			},
		}
		if containsPosition(classRange, line-1, col-1) {
			ranges = append(ranges, classRange)

			// 检查方法
			for _, method := range d.Methods {
				if method.Body != nil {
					methodRange := protocol.Range{
						Start: protocol.Position{
							Line:      uint32(method.FuncToken.Pos.Line - 1),
							Character: uint32(method.FuncToken.Pos.Column - 1),
						},
						End: protocol.Position{
							Line:      uint32(method.Body.RBrace.Pos.Line - 1),
							Character: uint32(method.Body.RBrace.Pos.Column),
						},
					}
					if containsPosition(methodRange, line-1, col-1) {
						ranges = append(ranges, methodRange)
						ranges = append(ranges, findStmtRanges(method.Body, line, col)...)
					}
				}
			}
		}

	case *ast.InterfaceDecl:
		ifaceRange := protocol.Range{
			Start: protocol.Position{
				Line:      uint32(d.InterfaceToken.Pos.Line - 1),
				Character: uint32(d.InterfaceToken.Pos.Column - 1),
			},
			End: protocol.Position{
				Line:      uint32(d.RBrace.Pos.Line - 1),
				Character: uint32(d.RBrace.Pos.Column),
			},
		}
		if containsPosition(ifaceRange, line-1, col-1) {
			ranges = append(ranges, ifaceRange)
		}

	case *ast.EnumDecl:
		enumRange := protocol.Range{
			Start: protocol.Position{
				Line:      uint32(d.EnumToken.Pos.Line - 1),
				Character: uint32(d.EnumToken.Pos.Column - 1),
			},
			End: protocol.Position{
				Line:      uint32(d.RBrace.Pos.Line - 1),
				Character: uint32(d.RBrace.Pos.Column),
			},
		}
		if containsPosition(enumRange, line-1, col-1) {
			ranges = append(ranges, enumRange)
		}
	}

	return ranges
}

// findStmtRanges 查找语句中包含位置的范围
func findStmtRanges(stmt ast.Statement, line, col int) []protocol.Range {
	var ranges []protocol.Range

	if stmt == nil {
		return ranges
	}

	switch s := stmt.(type) {
	case *ast.BlockStmt:
		blockRange := protocol.Range{
			Start: protocol.Position{
				Line:      uint32(s.LBrace.Pos.Line - 1),
				Character: uint32(s.LBrace.Pos.Column - 1),
			},
			End: protocol.Position{
				Line:      uint32(s.RBrace.Pos.Line - 1),
				Character: uint32(s.RBrace.Pos.Column),
			},
		}
		if containsPosition(blockRange, line-1, col-1) {
			ranges = append(ranges, blockRange)
			for _, inner := range s.Statements {
				ranges = append(ranges, findStmtRanges(inner, line, col)...)
			}
		}

	case *ast.IfStmt:
		ranges = append(ranges, findStmtRanges(s.Then, line, col)...)
		if s.Else != nil {
			ranges = append(ranges, findStmtRanges(s.Else, line, col)...)
		}

	case *ast.ForStmt:
		ranges = append(ranges, findStmtRanges(s.Body, line, col)...)

	case *ast.ForeachStmt:
		ranges = append(ranges, findStmtRanges(s.Body, line, col)...)

	case *ast.WhileStmt:
		ranges = append(ranges, findStmtRanges(s.Body, line, col)...)

	case *ast.TryStmt:
		ranges = append(ranges, findStmtRanges(s.Try, line, col)...)
		for _, catch := range s.Catches {
			ranges = append(ranges, findStmtRanges(catch.Body, line, col)...)
		}
		if s.Finally != nil && s.Finally.Body != nil {
			ranges = append(ranges, findStmtRanges(s.Finally.Body, line, col)...)
		}
	}

	return ranges
}

// containsPosition 检查范围是否包含位置
func containsPosition(r protocol.Range, line, col int) bool {
	if uint32(line) < r.Start.Line || uint32(line) > r.End.Line {
		return false
	}
	if uint32(line) == r.Start.Line && uint32(col) < r.Start.Character {
		return false
	}
	if uint32(line) == r.End.Line && uint32(col) > r.End.Character {
		return false
	}
	return true
}

// deduplicateRanges 去重范围
func deduplicateRanges(ranges []protocol.Range) []protocol.Range {
	seen := make(map[string]bool)
	var result []protocol.Range

	for _, r := range ranges {
		key := rangeKey(r)
		if !seen[key] {
			seen[key] = true
			result = append(result, r)
		}
	}

	return result
}

// rangeKey 生成范围的唯一键
func rangeKey(r protocol.Range) string {
	return string(rune(r.Start.Line)) + ":" + string(rune(r.Start.Character)) + "-" +
		string(rune(r.End.Line)) + ":" + string(rune(r.End.Character))
}

// buildSelectionRangeChain 构建选择范围链
func buildSelectionRangeChain(ranges []protocol.Range) *SelectionRange {
	if len(ranges) == 0 {
		return nil
	}

	// 按范围大小排序（从小到大）
	sortRangesBySize(ranges)

	// 构建链
	var current *SelectionRange
	for i := len(ranges) - 1; i >= 0; i-- {
		current = &SelectionRange{
			Range:  ranges[i],
			Parent: current,
		}
	}

	return current
}

// sortRangesBySize 按范围大小排序
func sortRangesBySize(ranges []protocol.Range) {
	// 简单冒泡排序
	for i := 0; i < len(ranges)-1; i++ {
		for j := 0; j < len(ranges)-1-i; j++ {
			if rangeSize(ranges[j]) > rangeSize(ranges[j+1]) {
				ranges[j], ranges[j+1] = ranges[j+1], ranges[j]
			}
		}
	}
}

// rangeSize 计算范围大小
func rangeSize(r protocol.Range) int {
	lines := int(r.End.Line - r.Start.Line)
	if lines == 0 {
		return int(r.End.Character - r.Start.Character)
	}
	return lines * 1000 + int(r.End.Character)
}
