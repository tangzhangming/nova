package lsp

import (
	"encoding/json"
	"strings"

	"github.com/tangzhangming/nova/internal/ast"
	"go.lsp.dev/protocol"
)

// handleReferences 处理查找引用请求
func (s *Server) handleReferences(id json.RawMessage, params json.RawMessage) {
	var p protocol.ReferenceParams
	if err := json.Unmarshal(params, &p); err != nil {
		s.sendError(id, -32700, "Parse error")
		return
	}

	docURI := string(p.TextDocument.URI)
	doc := s.documents.Get(docURI)
	if doc == nil {
		s.sendResult(id, []protocol.Location{})
		return
	}

	line := int(p.Position.Line)
	character := int(p.Position.Character)

	// 获取当前位置的单词
	word := doc.GetWordAt(line, character)
	if word == "" {
		s.sendResult(id, []protocol.Location{})
		return
	}

	// 检查是否是变量
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

	// 查找所有引用
	var locations []protocol.Location

	// 在当前文档中查找
	refs := s.findReferencesInDoc(doc, word, isVariable, p.Context.IncludeDeclaration)
	locations = append(locations, refs...)

	// 如果是类/接口/函数，在其他文档中也查找
	if !isVariable {
		for _, otherDoc := range s.documents.GetAll() {
			if otherDoc.URI == docURI {
				continue
			}
			refs := s.findReferencesInDoc(otherDoc, word, false, p.Context.IncludeDeclaration)
			locations = append(locations, refs...)
		}
	}

	s.sendResult(id, locations)
}

// findReferencesInDoc 在文档中查找引用
func (s *Server) findReferencesInDoc(doc *Document, name string, isVariable, includeDeclaration bool) []protocol.Location {
	var locations []protocol.Location

	// 逐行搜索
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

				// 确保是完整的变量名（后面不是单词字符）
				endPos := actualPos + len(searchStr)
				if endPos < len(lineText) && isWordChar(lineText[endPos]) {
					pos = actualPos + 1
					continue
				}

				locations = append(locations, protocol.Location{
					URI: protocol.DocumentURI(doc.URI),
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

				locations = append(locations, protocol.Location{
					URI: protocol.DocumentURI(doc.URI),
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
				})
				pos = endPos
			}
		}
	}

	return locations
}

// findReferencesInAST 在 AST 中查找引用（更精确的方法）
func (s *Server) findReferencesInAST(file *ast.File, name string, isVariable bool, docURI string) []protocol.Location {
	var locations []protocol.Location

	visitor := &referenceVisitor{
		name:       name,
		isVariable: isVariable,
		docURI:     docURI,
		locations:  &locations,
	}

	// 遍历所有声明
	for _, decl := range file.Declarations {
		visitDeclaration(decl, visitor)
	}

	// 遍历所有语句
	for _, stmt := range file.Statements {
		visitStatement(stmt, visitor)
	}

	return locations
}

// referenceVisitor 引用访问器
type referenceVisitor struct {
	name       string
	isVariable bool
	docURI     string
	locations  *[]protocol.Location
}

// visitDeclaration 遍历声明
func visitDeclaration(decl ast.Declaration, v *referenceVisitor) {
	switch d := decl.(type) {
	case *ast.ClassDecl:
		// 检查类名
		if !v.isVariable && d.Name.Name == v.name {
			*v.locations = append(*v.locations, makeLocation(v.docURI, d.Name.Token.Pos.Line, d.Name.Token.Pos.Column, len(v.name)))
		}
		// 检查方法
		for _, method := range d.Methods {
			if !v.isVariable && method.Name.Name == v.name {
				*v.locations = append(*v.locations, makeLocation(v.docURI, method.Name.Token.Pos.Line, method.Name.Token.Pos.Column, len(v.name)))
			}
			// 检查方法体
			if method.Body != nil {
				visitStatement(method.Body, v)
			}
		}
		// 检查属性
		for _, prop := range d.Properties {
			if v.isVariable && prop.Name.Name == v.name {
				*v.locations = append(*v.locations, makeLocation(v.docURI, prop.Name.Token.Pos.Line, prop.Name.Token.Pos.Column, len(v.name)))
			}
		}
	case *ast.InterfaceDecl:
		if !v.isVariable && d.Name.Name == v.name {
			*v.locations = append(*v.locations, makeLocation(v.docURI, d.Name.Token.Pos.Line, d.Name.Token.Pos.Column, len(v.name)))
		}
	case *ast.EnumDecl:
		if !v.isVariable && d.Name.Name == v.name {
			*v.locations = append(*v.locations, makeLocation(v.docURI, d.Name.Token.Pos.Line, d.Name.Token.Pos.Column, len(v.name)))
		}
	}
}

// visitStatement 遍历语句
func visitStatement(stmt ast.Statement, v *referenceVisitor) {
	if stmt == nil {
		return
	}

	switch s := stmt.(type) {
	case *ast.VarDeclStmt:
		if v.isVariable && s.Name.Name == v.name {
			*v.locations = append(*v.locations, makeLocation(v.docURI, s.Name.Token.Pos.Line, s.Name.Token.Pos.Column, len(v.name)))
		}
		if s.Value != nil {
			visitExpression(s.Value, v)
		}
	case *ast.ExprStmt:
		visitExpression(s.Expr, v)
	case *ast.BlockStmt:
		for _, inner := range s.Statements {
			visitStatement(inner, v)
		}
	case *ast.IfStmt:
		visitExpression(s.Condition, v)
		visitStatement(s.Then, v)
		if s.Else != nil {
			visitStatement(s.Else, v)
		}
	case *ast.ForStmt:
		if s.Init != nil {
			visitStatement(s.Init, v)
		}
		if s.Condition != nil {
			visitExpression(s.Condition, v)
		}
		if s.Post != nil {
			visitExpression(s.Post, v)
		}
		visitStatement(s.Body, v)
	case *ast.ForeachStmt:
		visitExpression(s.Iterable, v)
		if s.Key != nil && v.isVariable && s.Key.Name == v.name {
			*v.locations = append(*v.locations, makeLocation(v.docURI, s.Key.Token.Pos.Line, s.Key.Token.Pos.Column, len(v.name)))
		}
		if s.Value != nil && v.isVariable && s.Value.Name == v.name {
			*v.locations = append(*v.locations, makeLocation(v.docURI, s.Value.Token.Pos.Line, s.Value.Token.Pos.Column, len(v.name)))
		}
		visitStatement(s.Body, v)
	case *ast.WhileStmt:
		visitExpression(s.Condition, v)
		visitStatement(s.Body, v)
	case *ast.ReturnStmt:
		for _, val := range s.Values {
			visitExpression(val, v)
		}
	}
}

// visitExpression 遍历表达式
func visitExpression(expr ast.Expression, v *referenceVisitor) {
	if expr == nil {
		return
	}

	switch e := expr.(type) {
	case *ast.Variable:
		if v.isVariable && e.Name == v.name {
			*v.locations = append(*v.locations, makeLocation(v.docURI, e.Token.Pos.Line, e.Token.Pos.Column, len(v.name)+1)) // +1 for $
		}
	case *ast.Identifier:
		if !v.isVariable && e.Name == v.name {
			*v.locations = append(*v.locations, makeLocation(v.docURI, e.Token.Pos.Line, e.Token.Pos.Column, len(v.name)))
		}
	case *ast.BinaryExpr:
		visitExpression(e.Left, v)
		visitExpression(e.Right, v)
	case *ast.UnaryExpr:
		visitExpression(e.Operand, v)
	case *ast.CallExpr:
		visitExpression(e.Function, v)
		for _, arg := range e.Arguments {
			visitExpression(arg, v)
		}
	case *ast.MethodCall:
		visitExpression(e.Object, v)
		for _, arg := range e.Arguments {
			visitExpression(arg, v)
		}
	case *ast.PropertyAccess:
		visitExpression(e.Object, v)
	case *ast.IndexExpr:
		visitExpression(e.Object, v)
		visitExpression(e.Index, v)
	case *ast.AssignExpr:
		visitExpression(e.Left, v)
		visitExpression(e.Right, v)
	case *ast.NewExpr:
		if !v.isVariable && e.ClassName.Name == v.name {
			*v.locations = append(*v.locations, makeLocation(v.docURI, e.ClassName.Token.Pos.Line, e.ClassName.Token.Pos.Column, len(v.name)))
		}
		for _, arg := range e.Arguments {
			visitExpression(arg, v)
		}
	case *ast.ArrayLiteral:
		for _, elem := range e.Elements {
			visitExpression(elem, v)
		}
	case *ast.TernaryExpr:
		visitExpression(e.Condition, v)
		visitExpression(e.Then, v)
		visitExpression(e.Else, v)
	}
}

// makeLocation 创建位置
func makeLocation(docURI string, line, col, length int) protocol.Location {
	return protocol.Location{
		URI: protocol.DocumentURI(docURI),
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
	}
}
