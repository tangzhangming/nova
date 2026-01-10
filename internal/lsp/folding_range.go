package lsp

import (
	"encoding/json"
	"strings"

	"github.com/tangzhangming/nova/internal/ast"
	"go.lsp.dev/protocol"
)

// FoldingRangeKind 折叠范围类型
type FoldingRangeKind string

const (
	FoldingRangeKindComment FoldingRangeKind = "comment"
	FoldingRangeKindImports FoldingRangeKind = "imports"
	FoldingRangeKindRegion  FoldingRangeKind = "region"
)

// FoldingRange 折叠范围
type FoldingRange struct {
	StartLine      uint32            `json:"startLine"`
	StartCharacter *uint32           `json:"startCharacter,omitempty"`
	EndLine        uint32            `json:"endLine"`
	EndCharacter   *uint32           `json:"endCharacter,omitempty"`
	Kind           *FoldingRangeKind `json:"kind,omitempty"`
}

// handleFoldingRange 处理折叠范围请求
func (s *Server) handleFoldingRange(id json.RawMessage, params json.RawMessage) {
	var p protocol.FoldingRangeParams
	if err := json.Unmarshal(params, &p); err != nil {
		s.sendError(id, -32700, "Parse error")
		return
	}

	docURI := string(p.TextDocument.URI)
	doc := s.documents.Get(docURI)
	if doc == nil {
		s.sendResult(id, []FoldingRange{})
		return
	}

	ranges := s.collectFoldingRanges(doc)
	s.sendResult(id, ranges)
}

// collectFoldingRanges 收集文档中的折叠范围
func (s *Server) collectFoldingRanges(doc *Document) []FoldingRange {
	var ranges []FoldingRange

	// 从 AST 收集折叠范围
	astFile := doc.GetAST()
	if astFile != nil {
		ranges = append(ranges, s.collectASTFoldingRanges(astFile)...)
	}

	// 从文本收集注释折叠范围
	ranges = append(ranges, s.collectCommentFoldingRanges(doc)...)

	// 收集 use/import 折叠范围
	ranges = append(ranges, s.collectImportFoldingRanges(doc, astFile)...)

	// 收集 region 折叠范围
	ranges = append(ranges, s.collectRegionFoldingRanges(doc)...)

	return ranges
}

// collectASTFoldingRanges 从 AST 收集折叠范围
func (s *Server) collectASTFoldingRanges(file *ast.File) []FoldingRange {
	var ranges []FoldingRange

	// 遍历声明
	for _, decl := range file.Declarations {
		ranges = append(ranges, collectDeclFoldingRange(decl)...)
	}

	// 遍历语句
	for _, stmt := range file.Statements {
		ranges = append(ranges, collectStmtFoldingRange(stmt)...)
	}

	return ranges
}

// collectDeclFoldingRange 收集声明的折叠范围
func collectDeclFoldingRange(decl ast.Declaration) []FoldingRange {
	var ranges []FoldingRange

	switch d := decl.(type) {
	case *ast.ClassDecl:
		// 类声明的折叠范围
		startLine := uint32(d.ClassToken.Pos.Line - 1)
		endLine := uint32(d.RBrace.Pos.Line - 1)
		if endLine > startLine {
			ranges = append(ranges, FoldingRange{
				StartLine: startLine,
				EndLine:   endLine,
			})
		}
		// 方法的折叠范围
		for _, method := range d.Methods {
			if method.Body != nil {
				mStartLine := uint32(method.Name.Token.Pos.Line - 1)
				mEndLine := uint32(method.Body.RBrace.Pos.Line - 1)
				if mEndLine > mStartLine {
					ranges = append(ranges, FoldingRange{
						StartLine: mStartLine,
						EndLine:   mEndLine,
					})
				}
			}
		}
	case *ast.InterfaceDecl:
		startLine := uint32(d.InterfaceToken.Pos.Line - 1)
		endLine := uint32(d.RBrace.Pos.Line - 1)
		if endLine > startLine {
			ranges = append(ranges, FoldingRange{
				StartLine: startLine,
				EndLine:   endLine,
			})
		}
	case *ast.EnumDecl:
		startLine := uint32(d.EnumToken.Pos.Line - 1)
		endLine := uint32(d.RBrace.Pos.Line - 1)
		if endLine > startLine {
			ranges = append(ranges, FoldingRange{
				StartLine: startLine,
				EndLine:   endLine,
			})
		}
	}

	return ranges
}

// collectStmtFoldingRange 收集语句的折叠范围
func collectStmtFoldingRange(stmt ast.Statement) []FoldingRange {
	var ranges []FoldingRange

	if stmt == nil {
		return ranges
	}

	switch s := stmt.(type) {
	case *ast.BlockStmt:
		startLine := uint32(s.LBrace.Pos.Line - 1)
		endLine := uint32(s.RBrace.Pos.Line - 1)
		if endLine > startLine {
			ranges = append(ranges, FoldingRange{
				StartLine: startLine,
				EndLine:   endLine,
			})
		}
		// 递归处理内部语句
		for _, inner := range s.Statements {
			ranges = append(ranges, collectStmtFoldingRange(inner)...)
		}
	case *ast.IfStmt:
		if s.Then != nil {
			ranges = append(ranges, collectStmtFoldingRange(s.Then)...)
		}
		if s.Else != nil {
			ranges = append(ranges, collectStmtFoldingRange(s.Else)...)
		}
	case *ast.ForStmt:
		ranges = append(ranges, collectStmtFoldingRange(s.Body)...)
	case *ast.ForeachStmt:
		ranges = append(ranges, collectStmtFoldingRange(s.Body)...)
	case *ast.WhileStmt:
		ranges = append(ranges, collectStmtFoldingRange(s.Body)...)
	case *ast.SwitchStmt:
		startLine := uint32(s.LBrace.Pos.Line - 1)
		endLine := uint32(s.RBrace.Pos.Line - 1)
		if endLine > startLine {
			ranges = append(ranges, FoldingRange{
				StartLine: startLine,
				EndLine:   endLine,
			})
		}
		// case 折叠
		for _, c := range s.Cases {
			if stmts, ok := c.Body.([]ast.Statement); ok {
				for _, caseStmt := range stmts {
					ranges = append(ranges, collectStmtFoldingRange(caseStmt)...)
				}
			}
		}
	case *ast.TryStmt:
		if s.Try != nil {
			ranges = append(ranges, collectStmtFoldingRange(s.Try)...)
		}
		for _, catch := range s.Catches {
			if catch.Body != nil {
				ranges = append(ranges, collectStmtFoldingRange(catch.Body)...)
			}
		}
		if s.Finally != nil && s.Finally.Body != nil {
			ranges = append(ranges, collectStmtFoldingRange(s.Finally.Body)...)
		}
	}

	return ranges
}

// collectCommentFoldingRanges 收集注释折叠范围
func (s *Server) collectCommentFoldingRanges(doc *Document) []FoldingRange {
	var ranges []FoldingRange
	kind := FoldingRangeKindComment

	inBlockComment := false
	blockStart := 0

	for i, line := range doc.Lines {
		trimmed := strings.TrimSpace(line)

		// 检测多行注释开始 /*
		if !inBlockComment && strings.HasPrefix(trimmed, "/*") {
			inBlockComment = true
			blockStart = i
			// 检查是否在同一行结束
			if strings.Contains(trimmed, "*/") && !strings.HasSuffix(trimmed, "/*") {
				inBlockComment = false
			}
		} else if inBlockComment {
			// 检测多行注释结束 */
			if strings.Contains(trimmed, "*/") {
				inBlockComment = false
				if i > blockStart {
					ranges = append(ranges, FoldingRange{
						StartLine: uint32(blockStart),
						EndLine:   uint32(i),
						Kind:      &kind,
					})
				}
			}
		}
	}

	// 收集连续的单行注释块
	commentBlockStart := -1
	for i, line := range doc.Lines {
		trimmed := strings.TrimSpace(line)
		isComment := strings.HasPrefix(trimmed, "//")

		if isComment {
			if commentBlockStart == -1 {
				commentBlockStart = i
			}
		} else {
			if commentBlockStart != -1 && i-1 > commentBlockStart {
				ranges = append(ranges, FoldingRange{
					StartLine: uint32(commentBlockStart),
					EndLine:   uint32(i - 1),
					Kind:      &kind,
				})
			}
			commentBlockStart = -1
		}
	}

	// 处理文件末尾的注释块
	if commentBlockStart != -1 && len(doc.Lines)-1 > commentBlockStart {
		ranges = append(ranges, FoldingRange{
			StartLine: uint32(commentBlockStart),
			EndLine:   uint32(len(doc.Lines) - 1),
			Kind:      &kind,
		})
	}

	return ranges
}

// collectImportFoldingRanges 收集 use/import 折叠范围
func (s *Server) collectImportFoldingRanges(doc *Document, file *ast.File) []FoldingRange {
	var ranges []FoldingRange

	if file == nil || len(file.Uses) < 2 {
		return ranges
	}

	// 找到 use 语句的范围
	startLine := file.Uses[0].UseToken.Pos.Line - 1
	endLine := startLine

	for _, use := range file.Uses {
		line := use.UseToken.Pos.Line - 1
		if line > endLine {
			endLine = line
		}
	}

	if endLine > startLine {
		kind := FoldingRangeKindImports
		ranges = append(ranges, FoldingRange{
			StartLine: uint32(startLine),
			EndLine:   uint32(endLine),
			Kind:      &kind,
		})
	}

	return ranges
}

// collectRegionFoldingRanges 收集 #region/#endregion 折叠范围
func (s *Server) collectRegionFoldingRanges(doc *Document) []FoldingRange {
	var ranges []FoldingRange
	kind := FoldingRangeKindRegion

	// 用栈来处理嵌套的 region
	var regionStack []int

	for i, line := range doc.Lines {
		trimmed := strings.TrimSpace(line)

		// 检测 #region 或 // region
		if strings.HasPrefix(trimmed, "#region") || strings.HasPrefix(trimmed, "// region") || strings.HasPrefix(trimmed, "//region") {
			regionStack = append(regionStack, i)
		}

		// 检测 #endregion 或 // endregion
		if strings.HasPrefix(trimmed, "#endregion") || strings.HasPrefix(trimmed, "// endregion") || strings.HasPrefix(trimmed, "//endregion") {
			if len(regionStack) > 0 {
				startLine := regionStack[len(regionStack)-1]
				regionStack = regionStack[:len(regionStack)-1]
				if i > startLine {
					ranges = append(ranges, FoldingRange{
						StartLine: uint32(startLine),
						EndLine:   uint32(i),
						Kind:      &kind,
					})
				}
			}
		}
	}

	return ranges
}
