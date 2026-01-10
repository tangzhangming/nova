package lsp

import (
	"encoding/json"

	"github.com/tangzhangming/nova/internal/ast"
	"go.lsp.dev/protocol"
)

// LinkedEditingRangeParams 链接编辑范围参数
type LinkedEditingRangeParams struct {
	TextDocument protocol.TextDocumentIdentifier `json:"textDocument"`
	Position     protocol.Position               `json:"position"`
}

// LinkedEditingRanges 链接编辑范围
type LinkedEditingRanges struct {
	Ranges      []protocol.Range `json:"ranges"`
	WordPattern string           `json:"wordPattern,omitempty"`
}

// handleLinkedEditingRange 处理链接编辑范围请求
func (s *Server) handleLinkedEditingRange(id json.RawMessage, params json.RawMessage) {
	var p LinkedEditingRangeParams
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

	// 获取当前位置的单词
	word := doc.GetWordAt(line, character)
	if word == "" {
		s.sendResult(id, nil)
		return
	}

	astFile := doc.GetAST()
	if astFile == nil {
		s.sendResult(id, nil)
		return
	}

	// 查找链接编辑范围
	ranges := s.findLinkedEditingRanges(astFile, doc, word, line, character)
	if len(ranges) < 2 {
		s.sendResult(id, nil)
		return
	}

	s.sendResult(id, LinkedEditingRanges{
		Ranges:      ranges,
		WordPattern: `[a-zA-Z_][a-zA-Z0-9_]*`,
	})
}

// findLinkedEditingRanges 查找链接编辑范围
func (s *Server) findLinkedEditingRanges(file *ast.File, doc *Document, word string, line, character int) []protocol.Range {
	var ranges []protocol.Range

	// 检查是否是变量名
	if len(word) > 0 && word[0] == '$' {
		// 变量名链接编辑
		ranges = s.findVariableRanges(file, doc, word)
	} else {
		// 检查是否在类/接口/枚举声明位置
		if declRange := s.findDeclarationRange(file, word, line+1); declRange != nil {
			ranges = append(ranges, *declRange)
			// 查找所有该类型的引用
			refRanges := s.findTypeReferenceRanges(file, word)
			ranges = append(ranges, refRanges...)
		}
	}

	return ranges
}

// findVariableRanges 查找变量的所有出现范围
func (s *Server) findVariableRanges(file *ast.File, doc *Document, varName string) []protocol.Range {
	var ranges []protocol.Range

	// 在所有语句中查找变量
	for _, stmt := range file.Statements {
		ranges = append(ranges, findVarInStmt(stmt, varName)...)
	}

	// 在所有类方法中查找变量
	for _, decl := range file.Declarations {
		if classDecl, ok := decl.(*ast.ClassDecl); ok {
			for _, method := range classDecl.Methods {
				// 检查参数
				for _, param := range method.Parameters {
					if "$"+param.Name.Name == varName {
						ranges = append(ranges, protocol.Range{
							Start: protocol.Position{
								Line:      uint32(param.Name.Token.Pos.Line - 1),
								Character: uint32(param.Name.Token.Pos.Column - 1),
							},
							End: protocol.Position{
								Line:      uint32(param.Name.Token.Pos.Line - 1),
								Character: uint32(param.Name.Token.Pos.Column - 1 + len(param.Name.Name)),
							},
						})
					}
				}
				// 检查方法体
				if method.Body != nil {
					ranges = append(ranges, findVarInStmt(method.Body, varName)...)
				}
			}
		}
	}

	return ranges
}

// findVarInStmt 在语句中查找变量
func findVarInStmt(stmt ast.Statement, varName string) []protocol.Range {
	var ranges []protocol.Range

	if stmt == nil {
		return ranges
	}

	switch s := stmt.(type) {
	case *ast.VarDeclStmt:
		if "$"+s.Name.Name == varName {
			ranges = append(ranges, protocol.Range{
				Start: protocol.Position{
					Line:      uint32(s.Name.Token.Pos.Line - 1),
					Character: uint32(s.Name.Token.Pos.Column - 1),
				},
				End: protocol.Position{
					Line:      uint32(s.Name.Token.Pos.Line - 1),
					Character: uint32(s.Name.Token.Pos.Column - 1 + len(s.Name.Name)),
				},
			})
		}
		if s.Value != nil {
			ranges = append(ranges, findVarInExpr(s.Value, varName)...)
		}
	case *ast.BlockStmt:
		for _, inner := range s.Statements {
			ranges = append(ranges, findVarInStmt(inner, varName)...)
		}
	case *ast.ExprStmt:
		ranges = append(ranges, findVarInExpr(s.Expr, varName)...)
	case *ast.IfStmt:
		ranges = append(ranges, findVarInExpr(s.Condition, varName)...)
		ranges = append(ranges, findVarInStmt(s.Then, varName)...)
		if s.Else != nil {
			ranges = append(ranges, findVarInStmt(s.Else, varName)...)
		}
	case *ast.ForStmt:
		if s.Init != nil {
			ranges = append(ranges, findVarInStmt(s.Init, varName)...)
		}
		if s.Condition != nil {
			ranges = append(ranges, findVarInExpr(s.Condition, varName)...)
		}
		if s.Post != nil {
			ranges = append(ranges, findVarInExpr(s.Post, varName)...)
		}
		ranges = append(ranges, findVarInStmt(s.Body, varName)...)
	case *ast.ForeachStmt:
		if s.Key != nil && "$"+s.Key.Name == varName {
			ranges = append(ranges, protocol.Range{
				Start: protocol.Position{
					Line:      uint32(s.Key.Token.Pos.Line - 1),
					Character: uint32(s.Key.Token.Pos.Column - 1),
				},
				End: protocol.Position{
					Line:      uint32(s.Key.Token.Pos.Line - 1),
					Character: uint32(s.Key.Token.Pos.Column - 1 + len(s.Key.Name)),
				},
			})
		}
		if s.Value != nil && "$"+s.Value.Name == varName {
			ranges = append(ranges, protocol.Range{
				Start: protocol.Position{
					Line:      uint32(s.Value.Token.Pos.Line - 1),
					Character: uint32(s.Value.Token.Pos.Column - 1),
				},
				End: protocol.Position{
					Line:      uint32(s.Value.Token.Pos.Line - 1),
					Character: uint32(s.Value.Token.Pos.Column - 1 + len(s.Value.Name)),
				},
			})
		}
		ranges = append(ranges, findVarInExpr(s.Iterable, varName)...)
		ranges = append(ranges, findVarInStmt(s.Body, varName)...)
	case *ast.WhileStmt:
		ranges = append(ranges, findVarInExpr(s.Condition, varName)...)
		ranges = append(ranges, findVarInStmt(s.Body, varName)...)
	case *ast.ReturnStmt:
		for _, val := range s.Values {
			ranges = append(ranges, findVarInExpr(val, varName)...)
		}
	}

	return ranges
}

// findVarInExpr 在表达式中查找变量
func findVarInExpr(expr ast.Expression, varName string) []protocol.Range {
	var ranges []protocol.Range

	if expr == nil {
		return ranges
	}

	switch e := expr.(type) {
	case *ast.Identifier:
		if "$"+e.Name == varName || e.Name == varName {
			ranges = append(ranges, protocol.Range{
				Start: protocol.Position{
					Line:      uint32(e.Token.Pos.Line - 1),
					Character: uint32(e.Token.Pos.Column - 1),
				},
				End: protocol.Position{
					Line:      uint32(e.Token.Pos.Line - 1),
					Character: uint32(e.Token.Pos.Column - 1 + len(e.Name)),
				},
			})
		}
	case *ast.BinaryExpr:
		ranges = append(ranges, findVarInExpr(e.Left, varName)...)
		ranges = append(ranges, findVarInExpr(e.Right, varName)...)
	case *ast.UnaryExpr:
		ranges = append(ranges, findVarInExpr(e.Operand, varName)...)
	case *ast.AssignExpr:
		ranges = append(ranges, findVarInExpr(e.Left, varName)...)
		ranges = append(ranges, findVarInExpr(e.Right, varName)...)
	case *ast.CallExpr:
		ranges = append(ranges, findVarInExpr(e.Function, varName)...)
		for _, arg := range e.Arguments {
			ranges = append(ranges, findVarInExpr(arg, varName)...)
		}
	case *ast.MethodCall:
		ranges = append(ranges, findVarInExpr(e.Object, varName)...)
		for _, arg := range e.Arguments {
			ranges = append(ranges, findVarInExpr(arg, varName)...)
		}
	case *ast.PropertyAccess:
		ranges = append(ranges, findVarInExpr(e.Object, varName)...)
	case *ast.IndexExpr:
		ranges = append(ranges, findVarInExpr(e.Object, varName)...)
		ranges = append(ranges, findVarInExpr(e.Index, varName)...)
	case *ast.TernaryExpr:
		ranges = append(ranges, findVarInExpr(e.Condition, varName)...)
		ranges = append(ranges, findVarInExpr(e.Then, varName)...)
		ranges = append(ranges, findVarInExpr(e.Else, varName)...)
	case *ast.ArrayLiteral:
		for _, elem := range e.Elements {
			ranges = append(ranges, findVarInExpr(elem, varName)...)
		}
	case *ast.MapLiteral:
		for _, pair := range e.Pairs {
			ranges = append(ranges, findVarInExpr(pair.Key, varName)...)
			ranges = append(ranges, findVarInExpr(pair.Value, varName)...)
		}
	case *ast.NewExpr:
		for _, arg := range e.Arguments {
			ranges = append(ranges, findVarInExpr(arg, varName)...)
		}
	}

	return ranges
}

// findDeclarationRange 查找声明范围
func (s *Server) findDeclarationRange(file *ast.File, name string, line int) *protocol.Range {
	for _, decl := range file.Declarations {
		switch d := decl.(type) {
		case *ast.ClassDecl:
			if d.Name.Name == name && d.Name.Token.Pos.Line == line {
				return &protocol.Range{
					Start: protocol.Position{
						Line:      uint32(d.Name.Token.Pos.Line - 1),
						Character: uint32(d.Name.Token.Pos.Column - 1),
					},
					End: protocol.Position{
						Line:      uint32(d.Name.Token.Pos.Line - 1),
						Character: uint32(d.Name.Token.Pos.Column - 1 + len(d.Name.Name)),
					},
				}
			}
		case *ast.InterfaceDecl:
			if d.Name.Name == name && d.Name.Token.Pos.Line == line {
				return &protocol.Range{
					Start: protocol.Position{
						Line:      uint32(d.Name.Token.Pos.Line - 1),
						Character: uint32(d.Name.Token.Pos.Column - 1),
					},
					End: protocol.Position{
						Line:      uint32(d.Name.Token.Pos.Line - 1),
						Character: uint32(d.Name.Token.Pos.Column - 1 + len(d.Name.Name)),
					},
				}
			}
		case *ast.EnumDecl:
			if d.Name.Name == name && d.Name.Token.Pos.Line == line {
				return &protocol.Range{
					Start: protocol.Position{
						Line:      uint32(d.Name.Token.Pos.Line - 1),
						Character: uint32(d.Name.Token.Pos.Column - 1),
					},
					End: protocol.Position{
						Line:      uint32(d.Name.Token.Pos.Line - 1),
						Character: uint32(d.Name.Token.Pos.Column - 1 + len(d.Name.Name)),
					},
				}
			}
		}
	}
	return nil
}

// findTypeReferenceRanges 查找类型引用范围
func (s *Server) findTypeReferenceRanges(file *ast.File, typeName string) []protocol.Range {
	var ranges []protocol.Range

	for _, decl := range file.Declarations {
		switch d := decl.(type) {
		case *ast.ClassDecl:
			// 检查 extends
			if d.Extends != nil && d.Extends.Name == typeName {
				ranges = append(ranges, protocol.Range{
					Start: protocol.Position{
						Line:      uint32(d.Extends.Token.Pos.Line - 1),
						Character: uint32(d.Extends.Token.Pos.Column - 1),
					},
					End: protocol.Position{
						Line:      uint32(d.Extends.Token.Pos.Line - 1),
						Character: uint32(d.Extends.Token.Pos.Column - 1 + len(d.Extends.Name)),
					},
				})
			}
			// 检查 implements（简化处理）
			// 检查属性类型和方法参数/返回类型
		}
	}

	return ranges
}
