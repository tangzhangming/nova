package lsp

import (
	"encoding/json"
	"strings"

	"github.com/tangzhangming/nova/internal/ast"
	"go.lsp.dev/protocol"
)

// handleDocumentHighlight 处理文档高亮请求
func (s *Server) handleDocumentHighlight(id json.RawMessage, params json.RawMessage) {
	var p protocol.DocumentHighlightParams
	if err := json.Unmarshal(params, &p); err != nil {
		s.sendError(id, -32700, "Parse error")
		return
	}

	docURI := string(p.TextDocument.URI)
	doc := s.documents.Get(docURI)
	if doc == nil {
		s.sendResult(id, []protocol.DocumentHighlight{})
		return
	}

	line := int(p.Position.Line)
	character := int(p.Position.Character)

	// 获取当前位置的单词
	word := doc.GetWordAt(line, character)
	if word == "" {
		s.sendResult(id, []protocol.DocumentHighlight{})
		return
	}

	// 检查是否是变量（以$开头）
	lineText := doc.GetLine(line)
	isVariable := false
	if character > 0 && character <= len(lineText) {
		start := character
		for start > 0 && isWordChar(lineText[start-1]) {
			start--
		}
		if start > 0 && lineText[start-1] == '$' {
			isVariable = true
		}
	}

	// 查找所有高亮位置
	highlights := s.findDocumentHighlights(doc, word, isVariable)
	s.sendResult(id, highlights)
}

// findDocumentHighlights 在文档中查找所有高亮位置
func (s *Server) findDocumentHighlights(doc *Document, name string, isVariable bool) []protocol.DocumentHighlight {
	var highlights []protocol.DocumentHighlight

	// 尝试使用AST进行精确高亮（可以区分读写）
	astFile := doc.GetAST()
	if astFile != nil {
		highlights = s.findHighlightsInAST(astFile, name, isVariable, doc.URI)
		if len(highlights) > 0 {
			return highlights
		}
	}

	// 回退到文本搜索
	highlights = s.findHighlightsInText(doc, name, isVariable)
	return highlights
}

// findHighlightsInAST 在AST中查找高亮位置（可以区分读写）
func (s *Server) findHighlightsInAST(file *ast.File, name string, isVariable bool, docURI string) []protocol.DocumentHighlight {
	var highlights []protocol.DocumentHighlight

	visitor := &highlightVisitor{
		name:       name,
		isVariable: isVariable,
		highlights: &highlights,
	}

	// 遍历所有声明
	for _, decl := range file.Declarations {
		visitDeclForHighlight(decl, visitor)
	}

	// 遍历所有语句
	for _, stmt := range file.Statements {
		visitStmtForHighlight(stmt, visitor)
	}

	return highlights
}

// highlightVisitor 高亮访问器
type highlightVisitor struct {
	name       string
	isVariable bool
	highlights *[]protocol.DocumentHighlight
}

// addHighlight 添加高亮
func (v *highlightVisitor) addHighlight(line, col, length int, kind protocol.DocumentHighlightKind) {
	*v.highlights = append(*v.highlights, protocol.DocumentHighlight{
		Range: protocol.Range{
			Start: protocol.Position{
				Line:      uint32(line - 1),
				Character: uint32(col - 1),
			},
			End: protocol.Position{
				Line:      uint32(line - 1),
				Character: uint32(col - 1 + length),
			},
		},
		Kind: kind,
	})
}

// visitDeclForHighlight 遍历声明查找高亮
func visitDeclForHighlight(decl ast.Declaration, v *highlightVisitor) {
	switch d := decl.(type) {
	case *ast.ClassDecl:
		// 类名是定义（写入）
		if !v.isVariable && d.Name.Name == v.name {
			v.addHighlight(d.Name.Token.Pos.Line, d.Name.Token.Pos.Column, len(v.name), protocol.DocumentHighlightKindWrite)
		}
		// 检查方法
		for _, method := range d.Methods {
			if !v.isVariable && method.Name.Name == v.name {
				v.addHighlight(method.Name.Token.Pos.Line, method.Name.Token.Pos.Column, len(v.name), protocol.DocumentHighlightKindWrite)
			}
			// 检查方法体
			if method.Body != nil {
				visitStmtForHighlight(method.Body, v)
			}
		}
		// 检查属性（定义）
		for _, prop := range d.Properties {
			if v.isVariable && prop.Name.Name == v.name {
				v.addHighlight(prop.Name.Token.Pos.Line, prop.Name.Token.Pos.Column, len(v.name), protocol.DocumentHighlightKindWrite)
			}
		}
	case *ast.InterfaceDecl:
		if !v.isVariable && d.Name.Name == v.name {
			v.addHighlight(d.Name.Token.Pos.Line, d.Name.Token.Pos.Column, len(v.name), protocol.DocumentHighlightKindWrite)
		}
	case *ast.EnumDecl:
		if !v.isVariable && d.Name.Name == v.name {
			v.addHighlight(d.Name.Token.Pos.Line, d.Name.Token.Pos.Column, len(v.name), protocol.DocumentHighlightKindWrite)
		}
	}
}

// visitStmtForHighlight 遍历语句查找高亮
func visitStmtForHighlight(stmt ast.Statement, v *highlightVisitor) {
	if stmt == nil {
		return
	}

	switch s := stmt.(type) {
	case *ast.VarDeclStmt:
		// 变量声明是写入
		if v.isVariable && s.Name.Name == v.name {
			v.addHighlight(s.Name.Token.Pos.Line, s.Name.Token.Pos.Column, len(v.name), protocol.DocumentHighlightKindWrite)
		}
		if s.Value != nil {
			visitExprForHighlight(s.Value, v, false)
		}
	case *ast.ExprStmt:
		visitExprForHighlight(s.Expr, v, false)
	case *ast.BlockStmt:
		for _, inner := range s.Statements {
			visitStmtForHighlight(inner, v)
		}
	case *ast.IfStmt:
		visitExprForHighlight(s.Condition, v, false)
		visitStmtForHighlight(s.Then, v)
		if s.Else != nil {
			visitStmtForHighlight(s.Else, v)
		}
	case *ast.ForStmt:
		if s.Init != nil {
			visitStmtForHighlight(s.Init, v)
		}
		if s.Condition != nil {
			visitExprForHighlight(s.Condition, v, false)
		}
		if s.Post != nil {
			visitExprForHighlight(s.Post, v, false)
		}
		visitStmtForHighlight(s.Body, v)
	case *ast.ForeachStmt:
		visitExprForHighlight(s.Iterable, v, false)
		// foreach的key和value是写入
		if s.Key != nil && v.isVariable && s.Key.Name == v.name {
			v.addHighlight(s.Key.Token.Pos.Line, s.Key.Token.Pos.Column, len(v.name), protocol.DocumentHighlightKindWrite)
		}
		if s.Value != nil && v.isVariable && s.Value.Name == v.name {
			v.addHighlight(s.Value.Token.Pos.Line, s.Value.Token.Pos.Column, len(v.name), protocol.DocumentHighlightKindWrite)
		}
		visitStmtForHighlight(s.Body, v)
	case *ast.WhileStmt:
		visitExprForHighlight(s.Condition, v, false)
		visitStmtForHighlight(s.Body, v)
	case *ast.ReturnStmt:
		for _, val := range s.Values {
			visitExprForHighlight(val, v, false)
		}
	case *ast.TryStmt:
		visitStmtForHighlight(s.Try, v)
		for _, catch := range s.Catches {
			if catch.Variable != nil && v.isVariable && catch.Variable.Name == v.name {
				v.addHighlight(catch.Variable.Token.Pos.Line, catch.Variable.Token.Pos.Column, len(v.name), protocol.DocumentHighlightKindWrite)
			}
			visitStmtForHighlight(catch.Body, v)
		}
		if s.Finally != nil && s.Finally.Body != nil {
			visitStmtForHighlight(s.Finally.Body, v)
		}
	case *ast.SwitchStmt:
		visitExprForHighlight(s.Expr, v, false)
		for _, c := range s.Cases {
			for _, val := range c.Values {
				visitExprForHighlight(val, v, false)
			}
			// Body 可能是 Expression 或 []Statement
			if stmts, ok := c.Body.([]ast.Statement); ok {
				for _, bodyStmt := range stmts {
					visitStmtForHighlight(bodyStmt, v)
				}
			} else if expr, ok := c.Body.(ast.Expression); ok {
				visitExprForHighlight(expr, v, false)
			}
		}
		if s.Default != nil {
			// Default.Body 也可能是 Expression 或 []Statement
			if stmts, ok := s.Default.Body.([]ast.Statement); ok {
				for _, bodyStmt := range stmts {
					visitStmtForHighlight(bodyStmt, v)
				}
			} else if expr, ok := s.Default.Body.(ast.Expression); ok {
				visitExprForHighlight(expr, v, false)
			}
		}
	}
}

// visitExprForHighlight 遍历表达式查找高亮
// isWrite 表示当前表达式是否处于写入上下文
func visitExprForHighlight(expr ast.Expression, v *highlightVisitor, isWrite bool) {
	if expr == nil {
		return
	}

	switch e := expr.(type) {
	case *ast.Variable:
		if v.isVariable && e.Name == v.name {
			kind := protocol.DocumentHighlightKindRead
			if isWrite {
				kind = protocol.DocumentHighlightKindWrite
			}
			// +1 for $ prefix
			v.addHighlight(e.Token.Pos.Line, e.Token.Pos.Column, len(v.name)+1, kind)
		}
	case *ast.Identifier:
		if !v.isVariable && e.Name == v.name {
			kind := protocol.DocumentHighlightKindRead
			if isWrite {
				kind = protocol.DocumentHighlightKindWrite
			}
			v.addHighlight(e.Token.Pos.Line, e.Token.Pos.Column, len(v.name), kind)
		}
	case *ast.BinaryExpr:
		visitExprForHighlight(e.Left, v, false)
		visitExprForHighlight(e.Right, v, false)
	case *ast.UnaryExpr:
		visitExprForHighlight(e.Operand, v, false)
	case *ast.CallExpr:
		visitExprForHighlight(e.Function, v, false)
		for _, arg := range e.Arguments {
			visitExprForHighlight(arg, v, false)
		}
	case *ast.MethodCall:
		visitExprForHighlight(e.Object, v, false)
		// 方法名
		if !v.isVariable && e.Method.Name == v.name {
			v.addHighlight(e.Method.Token.Pos.Line, e.Method.Token.Pos.Column, len(v.name), protocol.DocumentHighlightKindRead)
		}
		for _, arg := range e.Arguments {
			visitExprForHighlight(arg, v, false)
		}
	case *ast.PropertyAccess:
		visitExprForHighlight(e.Object, v, false)
		// 属性名
		if v.isVariable && e.Property.Name == v.name {
			v.addHighlight(e.Property.Token.Pos.Line, e.Property.Token.Pos.Column, len(v.name), protocol.DocumentHighlightKindRead)
		}
	case *ast.IndexExpr:
		visitExprForHighlight(e.Object, v, false)
		visitExprForHighlight(e.Index, v, false)
	case *ast.AssignExpr:
		// 赋值左侧是写入
		visitExprForHighlight(e.Left, v, true)
		// 赋值右侧是读取
		visitExprForHighlight(e.Right, v, false)
	case *ast.NewExpr:
		// 类名是读取
		if !v.isVariable && e.ClassName.Name == v.name {
			v.addHighlight(e.ClassName.Token.Pos.Line, e.ClassName.Token.Pos.Column, len(v.name), protocol.DocumentHighlightKindRead)
		}
		for _, arg := range e.Arguments {
			visitExprForHighlight(arg, v, false)
		}
	case *ast.ArrayLiteral:
		for _, elem := range e.Elements {
			visitExprForHighlight(elem, v, false)
		}
	case *ast.MapLiteral:
		for _, pair := range e.Pairs {
			visitExprForHighlight(pair.Key, v, false)
			visitExprForHighlight(pair.Value, v, false)
		}
	case *ast.TernaryExpr:
		visitExprForHighlight(e.Condition, v, false)
		visitExprForHighlight(e.Then, v, false)
		visitExprForHighlight(e.Else, v, false)
	case *ast.ClosureExpr:
		// 闭包参数是写入
		for _, param := range e.Parameters {
			if v.isVariable && param.Name.Name == v.name {
				v.addHighlight(param.Name.Token.Pos.Line, param.Name.Token.Pos.Column, len(v.name), protocol.DocumentHighlightKindWrite)
			}
		}
		// use 变量
		for _, useVar := range e.UseVars {
			if v.isVariable && useVar.Name == v.name {
				v.addHighlight(useVar.Token.Pos.Line, useVar.Token.Pos.Column, len(v.name)+1, protocol.DocumentHighlightKindRead)
			}
		}
		if e.Body != nil {
			visitStmtForHighlight(e.Body, v)
		}
	case *ast.ArrowFuncExpr:
		// 箭头函数参数是写入
		for _, param := range e.Parameters {
			if v.isVariable && param.Name.Name == v.name {
				v.addHighlight(param.Name.Token.Pos.Line, param.Name.Token.Pos.Column, len(v.name), protocol.DocumentHighlightKindWrite)
			}
		}
		if e.Body != nil {
			visitExprForHighlight(e.Body, v, false)
		}
	case *ast.StaticAccess:
		// 静态访问的类名
		if ident, ok := e.Class.(*ast.Identifier); ok {
			if !v.isVariable && ident.Name == v.name {
				v.addHighlight(ident.Token.Pos.Line, ident.Token.Pos.Column, len(v.name), protocol.DocumentHighlightKindRead)
			}
		}
		// 成员
		visitExprForHighlight(e.Member, v, false)
	}
}

// findHighlightsInText 在文本中查找高亮位置（回退方法）
func (s *Server) findHighlightsInText(doc *Document, name string, isVariable bool) []protocol.DocumentHighlight {
	var highlights []protocol.DocumentHighlight

	for lineNum, lineText := range doc.Lines {
		if isVariable {
			// 查找变量引用 $name
			searchStr := "$" + name
			pos := 0
			for {
				idx := strings.Index(lineText[pos:], searchStr)
				if idx == -1 {
					break
				}
				actualPos := pos + idx

				// 确保是完整的变量名
				endPos := actualPos + len(searchStr)
				if endPos < len(lineText) && isWordChar(lineText[endPos]) {
					pos = actualPos + 1
					continue
				}

				highlights = append(highlights, protocol.DocumentHighlight{
					Range: protocol.Range{
						Start: protocol.Position{
							Line:      uint32(lineNum),
							Character: uint32(actualPos),
						},
						End: protocol.Position{
							Line:      uint32(lineNum),
							Character: uint32(endPos),
						},
					},
					Kind: protocol.DocumentHighlightKindText,
				})
				pos = endPos
			}
		} else {
			// 查找标识符引用
			pos := 0
			for {
				idx := strings.Index(lineText[pos:], name)
				if idx == -1 {
					break
				}
				actualPos := pos + idx

				// 确保是完整的标识符
				if actualPos > 0 && isWordChar(lineText[actualPos-1]) {
					pos = actualPos + 1
					continue
				}
				endPos := actualPos + len(name)
				if endPos < len(lineText) && isWordChar(lineText[endPos]) {
					pos = actualPos + 1
					continue
				}

				// 排除 $ 前缀的变量
				if actualPos > 0 && lineText[actualPos-1] == '$' {
					pos = actualPos + 1
					continue
				}

				highlights = append(highlights, protocol.DocumentHighlight{
					Range: protocol.Range{
						Start: protocol.Position{
							Line:      uint32(lineNum),
							Character: uint32(actualPos),
						},
						End: protocol.Position{
							Line:      uint32(lineNum),
							Character: uint32(endPos),
						},
					},
					Kind: protocol.DocumentHighlightKindText,
				})
				pos = endPos
			}
		}
	}

	return highlights
}
