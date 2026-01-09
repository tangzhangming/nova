package lsp

import (
	"encoding/json"
	"strings"

	"github.com/tangzhangming/nova/internal/ast"
	"go.lsp.dev/protocol"
)

// handleDefinition 处理跳转定义请求
func (s *Server) handleDefinition(id json.RawMessage, params json.RawMessage) {
	var p protocol.DefinitionParams
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

	// 获取定义位置
	location := s.findDefinition(doc, line, character)
	if location == nil {
		s.sendResult(id, nil)
		return
	}

	s.sendResult(id, location)
}

// findDefinition 查找定义位置
func (s *Server) findDefinition(doc *Document, line, character int) *protocol.Location {
	// 获取当前位置的单词
	word := doc.GetWordAt(line, character)
	if word == "" {
		return nil
	}

	// 检查是否是变量（以 $ 开头）
	lineText := doc.GetLine(line)
	isVariable := false
	if character > 0 && character <= len(lineText) {
		start := character
		for start > 0 && isWordChar(lineText[start-1]) {
			start--
		}
		if start > 0 && lineText[start-1] == '$' {
			word = word // 不包含 $
			isVariable = true
		}
	}

	// 获取 AST
	astFile := doc.GetAST()
	if astFile == nil {
		return nil
	}

	docURI := doc.URI

	// 查找定义
	if isVariable {
		// 查找变量定义
		if loc := s.findVariableDefinition(astFile, word, line+1, docURI); loc != nil {
			return loc
		}
	} else {
		// 查找类/接口/函数定义
		if loc := s.findSymbolDefinition(astFile, word, docURI); loc != nil {
			return loc
		}
	}

	// 在其他打开的文档中查找
	for _, otherDoc := range s.documents.GetAll() {
		if otherDoc.URI == docURI {
			continue
		}
		otherAST := otherDoc.GetAST()
		if otherAST == nil {
			continue
		}
		if loc := s.findSymbolDefinition(otherAST, word, otherDoc.URI); loc != nil {
			return loc
		}
	}

	return nil
}

// findVariableDefinition 查找变量定义
func (s *Server) findVariableDefinition(file *ast.File, varName string, currentLine int, docURI string) *protocol.Location {
	// 遍历语句查找变量声明
	for _, stmt := range file.Statements {
		if loc := findVarDeclInStatement(stmt, varName, currentLine, docURI); loc != nil {
			return loc
		}
	}

	// 遍历类声明查找属性
	for _, decl := range file.Declarations {
		if classDecl, ok := decl.(*ast.ClassDecl); ok {
			for _, prop := range classDecl.Properties {
				if prop.Name.Name == varName {
					return &protocol.Location{
						URI: protocol.DocumentURI(docURI),
						Range: protocol.Range{
							Start: protocol.Position{
								Line:      uint32(prop.Name.Token.Pos.Line - 1),
								Character: uint32(prop.Name.Token.Pos.Column - 1),
							},
							End: protocol.Position{
								Line:      uint32(prop.Name.Token.Pos.Line - 1),
								Character: uint32(prop.Name.Token.Pos.Column - 1 + len(varName)),
							},
						},
					}
				}
			}
			// 检查方法参数
			for _, method := range classDecl.Methods {
				for _, param := range method.Parameters {
					if param.Name.Name == varName && method.Body != nil {
						// 检查当前行是否在方法体内
						methodStart := method.LParen.Pos.Line
						methodEnd := method.Body.RBrace.Pos.Line
						if currentLine >= methodStart && currentLine <= methodEnd {
							return &protocol.Location{
								URI: protocol.DocumentURI(docURI),
								Range: protocol.Range{
									Start: protocol.Position{
										Line:      uint32(param.Name.Token.Pos.Line - 1),
										Character: uint32(param.Name.Token.Pos.Column - 1),
									},
									End: protocol.Position{
										Line:      uint32(param.Name.Token.Pos.Line - 1),
										Character: uint32(param.Name.Token.Pos.Column - 1 + len(varName)),
									},
								},
							}
						}
					}
				}
			}
		}
	}

	return nil
}

// findVarDeclInStatement 在语句中查找变量声明
func findVarDeclInStatement(stmt ast.Statement, varName string, currentLine int, docURI string) *protocol.Location {
	switch s := stmt.(type) {
	case *ast.VarDeclStmt:
		if s.Name.Name == varName {
			return &protocol.Location{
				URI: protocol.DocumentURI(docURI),
				Range: protocol.Range{
					Start: protocol.Position{
						Line:      uint32(s.Name.Token.Pos.Line - 1),
						Character: uint32(s.Name.Token.Pos.Column - 1),
					},
					End: protocol.Position{
						Line:      uint32(s.Name.Token.Pos.Line - 1),
						Character: uint32(s.Name.Token.Pos.Column - 1 + len(varName)),
					},
				},
			}
		}
	case *ast.MultiVarDeclStmt:
		for _, v := range s.Names {
			if v.Name == varName {
				return &protocol.Location{
					URI: protocol.DocumentURI(docURI),
					Range: protocol.Range{
						Start: protocol.Position{
							Line:      uint32(v.Token.Pos.Line - 1),
							Character: uint32(v.Token.Pos.Column - 1),
						},
						End: protocol.Position{
							Line:      uint32(v.Token.Pos.Line - 1),
							Character: uint32(v.Token.Pos.Column - 1 + len(varName)),
						},
					},
				}
			}
		}
	case *ast.BlockStmt:
		for _, inner := range s.Statements {
			if loc := findVarDeclInStatement(inner, varName, currentLine, docURI); loc != nil {
				return loc
			}
		}
	case *ast.IfStmt:
		if loc := findVarDeclInStatement(s.Then, varName, currentLine, docURI); loc != nil {
			return loc
		}
		if s.Else != nil {
			if loc := findVarDeclInStatement(s.Else, varName, currentLine, docURI); loc != nil {
				return loc
			}
		}
	case *ast.ForStmt:
		if s.Init != nil {
			if loc := findVarDeclInStatement(s.Init, varName, currentLine, docURI); loc != nil {
				return loc
			}
		}
		if s.Body != nil {
			if loc := findVarDeclInStatement(s.Body, varName, currentLine, docURI); loc != nil {
				return loc
			}
		}
	case *ast.ForeachStmt:
		if s.Key != nil && s.Key.Name == varName {
			return &protocol.Location{
				URI: protocol.DocumentURI(docURI),
				Range: protocol.Range{
					Start: protocol.Position{
						Line:      uint32(s.Key.Token.Pos.Line - 1),
						Character: uint32(s.Key.Token.Pos.Column - 1),
					},
					End: protocol.Position{
						Line:      uint32(s.Key.Token.Pos.Line - 1),
						Character: uint32(s.Key.Token.Pos.Column - 1 + len(varName)),
					},
				},
			}
		}
		if s.Value != nil && s.Value.Name == varName {
			return &protocol.Location{
				URI: protocol.DocumentURI(docURI),
				Range: protocol.Range{
					Start: protocol.Position{
						Line:      uint32(s.Value.Token.Pos.Line - 1),
						Character: uint32(s.Value.Token.Pos.Column - 1),
					},
					End: protocol.Position{
						Line:      uint32(s.Value.Token.Pos.Line - 1),
						Character: uint32(s.Value.Token.Pos.Column - 1 + len(varName)),
					},
				},
			}
		}
		if s.Body != nil {
			if loc := findVarDeclInStatement(s.Body, varName, currentLine, docURI); loc != nil {
				return loc
			}
		}
	case *ast.WhileStmt:
		if s.Body != nil {
			if loc := findVarDeclInStatement(s.Body, varName, currentLine, docURI); loc != nil {
				return loc
			}
		}
	case *ast.TryStmt:
		if s.Try != nil {
			if loc := findVarDeclInStatement(s.Try, varName, currentLine, docURI); loc != nil {
				return loc
			}
		}
		for _, catch := range s.Catches {
			if catch.Variable.Name == varName {
				return &protocol.Location{
					URI: protocol.DocumentURI(docURI),
					Range: protocol.Range{
						Start: protocol.Position{
							Line:      uint32(catch.Variable.Token.Pos.Line - 1),
							Character: uint32(catch.Variable.Token.Pos.Column - 1),
						},
						End: protocol.Position{
							Line:      uint32(catch.Variable.Token.Pos.Line - 1),
							Character: uint32(catch.Variable.Token.Pos.Column - 1 + len(varName)),
						},
					},
				}
			}
			if catch.Body != nil {
				if loc := findVarDeclInStatement(catch.Body, varName, currentLine, docURI); loc != nil {
					return loc
				}
			}
		}
	}
	return nil
}

// findSymbolDefinition 查找符号定义（类/接口/函数）
func (s *Server) findSymbolDefinition(file *ast.File, name string, docURI string) *protocol.Location {
	for _, decl := range file.Declarations {
		switch d := decl.(type) {
		case *ast.ClassDecl:
			if d.Name.Name == name {
				return &protocol.Location{
					URI: protocol.DocumentURI(docURI),
					Range: protocol.Range{
						Start: protocol.Position{
							Line:      uint32(d.Name.Token.Pos.Line - 1),
							Character: uint32(d.Name.Token.Pos.Column - 1),
						},
						End: protocol.Position{
							Line:      uint32(d.Name.Token.Pos.Line - 1),
							Character: uint32(d.Name.Token.Pos.Column - 1 + len(name)),
						},
					},
				}
			}
			// 检查方法
			for _, method := range d.Methods {
				if method.Name.Name == name {
					return &protocol.Location{
						URI: protocol.DocumentURI(docURI),
						Range: protocol.Range{
							Start: protocol.Position{
								Line:      uint32(method.Name.Token.Pos.Line - 1),
								Character: uint32(method.Name.Token.Pos.Column - 1),
							},
							End: protocol.Position{
								Line:      uint32(method.Name.Token.Pos.Line - 1),
								Character: uint32(method.Name.Token.Pos.Column - 1 + len(name)),
							},
						},
					}
				}
			}
		case *ast.InterfaceDecl:
			if d.Name.Name == name {
				return &protocol.Location{
					URI: protocol.DocumentURI(docURI),
					Range: protocol.Range{
						Start: protocol.Position{
							Line:      uint32(d.Name.Token.Pos.Line - 1),
							Character: uint32(d.Name.Token.Pos.Column - 1),
						},
						End: protocol.Position{
							Line:      uint32(d.Name.Token.Pos.Line - 1),
							Character: uint32(d.Name.Token.Pos.Column - 1 + len(name)),
						},
					},
				}
			}
		case *ast.EnumDecl:
			if d.Name.Name == name {
				return &protocol.Location{
					URI: protocol.DocumentURI(docURI),
					Range: protocol.Range{
						Start: protocol.Position{
							Line:      uint32(d.Name.Token.Pos.Line - 1),
							Character: uint32(d.Name.Token.Pos.Column - 1),
						},
						End: protocol.Position{
							Line:      uint32(d.Name.Token.Pos.Line - 1),
							Character: uint32(d.Name.Token.Pos.Column - 1 + len(name)),
						},
					},
				}
			}
			// 检查枚举值
			for _, c := range d.Cases {
				if c.Name.Name == name {
					return &protocol.Location{
						URI: protocol.DocumentURI(docURI),
						Range: protocol.Range{
							Start: protocol.Position{
								Line:      uint32(c.Name.Token.Pos.Line - 1),
								Character: uint32(c.Name.Token.Pos.Column - 1),
							},
							End: protocol.Position{
								Line:      uint32(c.Name.Token.Pos.Line - 1),
								Character: uint32(c.Name.Token.Pos.Column - 1 + len(name)),
							},
						},
					}
				}
			}
		case *ast.TypeAliasDecl:
			if d.Name.Name == name {
				return &protocol.Location{
					URI: protocol.DocumentURI(docURI),
					Range: protocol.Range{
						Start: protocol.Position{
							Line:      uint32(d.Name.Token.Pos.Line - 1),
							Character: uint32(d.Name.Token.Pos.Column - 1),
						},
						End: protocol.Position{
							Line:      uint32(d.Name.Token.Pos.Line - 1),
							Character: uint32(d.Name.Token.Pos.Column - 1 + len(name)),
						},
					},
				}
			}
		case *ast.NewTypeDecl:
			if d.Name.Name == name {
				return &protocol.Location{
					URI: protocol.DocumentURI(docURI),
					Range: protocol.Range{
						Start: protocol.Position{
							Line:      uint32(d.Name.Token.Pos.Line - 1),
							Character: uint32(d.Name.Token.Pos.Column - 1),
						},
						End: protocol.Position{
							Line:      uint32(d.Name.Token.Pos.Line - 1),
							Character: uint32(d.Name.Token.Pos.Column - 1 + len(name)),
						},
					},
				}
			}
		}
	}

	return nil
}

// getMethodOrPropertyContext 获取方法或属性的上下文（类名）
func getMethodOrPropertyContext(lineText string, character int) string {
	// 向前查找 -> 或 ::
	pos := character
	for pos > 1 {
		if lineText[pos-2:pos] == "->" || lineText[pos-2:pos] == "::" {
			// 继续向前查找对象/类名
			end := pos - 2
			start := end
			for start > 0 && (isWordChar(lineText[start-1]) || lineText[start-1] == '$') {
				start--
			}
			return strings.TrimPrefix(lineText[start:end], "$")
		}
		pos--
	}
	return ""
}
