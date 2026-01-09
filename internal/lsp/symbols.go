package lsp

import (
	"encoding/json"
	"strings"

	"github.com/tangzhangming/nova/internal/ast"
	"go.lsp.dev/protocol"
)

// handleDocumentSymbol 处理文档符号请求
func (s *Server) handleDocumentSymbol(id json.RawMessage, params json.RawMessage) {
	var p protocol.DocumentSymbolParams
	if err := json.Unmarshal(params, &p); err != nil {
		s.sendError(id, -32700, "Parse error")
		return
	}

	docURI := string(p.TextDocument.URI)
	doc := s.documents.Get(docURI)
	if doc == nil {
		s.sendResult(id, []protocol.DocumentSymbol{})
		return
	}

	// 获取文档符号
	symbols := s.getDocumentSymbols(doc)
	s.sendResult(id, symbols)
}

// getDocumentSymbols 获取文档符号列表
func (s *Server) getDocumentSymbols(doc *Document) []protocol.DocumentSymbol {
	var symbols []protocol.DocumentSymbol

	astFile := doc.GetAST()
	if astFile == nil {
		return symbols
	}

	// 处理命名空间
	if astFile.Namespace != nil {
		nsSymbol := protocol.DocumentSymbol{
			Name: astFile.Namespace.Name,
			Kind: protocol.SymbolKindNamespace,
			Range: protocol.Range{
				Start: protocol.Position{
					Line:      uint32(astFile.Namespace.NamespaceToken.Pos.Line - 1),
					Character: 0,
				},
				End: protocol.Position{
					Line:      uint32(astFile.Namespace.NamespaceToken.Pos.Line - 1),
					Character: uint32(len("namespace " + astFile.Namespace.Name)),
				},
			},
			SelectionRange: protocol.Range{
				Start: protocol.Position{
					Line:      uint32(astFile.Namespace.NamespaceToken.Pos.Line - 1),
					Character: uint32(astFile.Namespace.NamespaceToken.Pos.Column - 1),
				},
				End: protocol.Position{
					Line:      uint32(astFile.Namespace.NamespaceToken.Pos.Line - 1),
					Character: uint32(astFile.Namespace.NamespaceToken.Pos.Column - 1 + len(astFile.Namespace.Name)),
				},
			},
		}
		symbols = append(symbols, nsSymbol)
	}

	// 处理声明
	for _, decl := range astFile.Declarations {
		symbol := s.declarationToSymbol(decl)
		if symbol != nil {
			symbols = append(symbols, *symbol)
		}
	}

	return symbols
}

// declarationToSymbol 将声明转换为符号
func (s *Server) declarationToSymbol(decl ast.Declaration) *protocol.DocumentSymbol {
	switch d := decl.(type) {
	case *ast.ClassDecl:
		return s.classToSymbol(d)
	case *ast.InterfaceDecl:
		return s.interfaceToSymbol(d)
	case *ast.EnumDecl:
		return s.enumToSymbol(d)
	case *ast.TypeAliasDecl:
		return &protocol.DocumentSymbol{
			Name:   d.Name.Name,
			Kind:   protocol.SymbolKindTypeParameter,
			Detail: "type alias",
			Range: protocol.Range{
				Start: protocol.Position{
					Line:      uint32(d.TypeToken.Pos.Line - 1),
					Character: 0,
				},
				End: protocol.Position{
					Line:      uint32(d.TypeToken.Pos.Line - 1),
					Character: uint32(len(d.Name.Name) + 20),
				},
			},
			SelectionRange: protocol.Range{
				Start: protocol.Position{
					Line:      uint32(d.Name.Token.Pos.Line - 1),
					Character: uint32(d.Name.Token.Pos.Column - 1),
				},
				End: protocol.Position{
					Line:      uint32(d.Name.Token.Pos.Line - 1),
					Character: uint32(d.Name.Token.Pos.Column - 1 + len(d.Name.Name)),
				},
			},
		}
	case *ast.NewTypeDecl:
		return &protocol.DocumentSymbol{
			Name:   d.Name.Name,
			Kind:   protocol.SymbolKindTypeParameter,
			Detail: "new type",
			Range: protocol.Range{
				Start: protocol.Position{
					Line:      uint32(d.TypeToken.Pos.Line - 1),
					Character: 0,
				},
				End: protocol.Position{
					Line:      uint32(d.TypeToken.Pos.Line - 1),
					Character: uint32(len(d.Name.Name) + 20),
				},
			},
			SelectionRange: protocol.Range{
				Start: protocol.Position{
					Line:      uint32(d.Name.Token.Pos.Line - 1),
					Character: uint32(d.Name.Token.Pos.Column - 1),
				},
				End: protocol.Position{
					Line:      uint32(d.Name.Token.Pos.Line - 1),
					Character: uint32(d.Name.Token.Pos.Column - 1 + len(d.Name.Name)),
				},
			},
		}
	}
	return nil
}

// classToSymbol 将类声明转换为符号
func (s *Server) classToSymbol(d *ast.ClassDecl) *protocol.DocumentSymbol {
	var children []protocol.DocumentSymbol

	// 添加常量
	for _, c := range d.Constants {
		children = append(children, protocol.DocumentSymbol{
			Name:   c.Name.Name,
			Kind:   protocol.SymbolKindConstant,
			Detail: typeNodeToString(c.Type),
			Range: protocol.Range{
				Start: protocol.Position{
					Line:      uint32(c.ConstToken.Pos.Line - 1),
					Character: 0,
				},
				End: protocol.Position{
					Line:      uint32(c.Semicolon.Pos.Line - 1),
					Character: uint32(c.Semicolon.Pos.Column),
				},
			},
			SelectionRange: protocol.Range{
				Start: protocol.Position{
					Line:      uint32(c.Name.Token.Pos.Line - 1),
					Character: uint32(c.Name.Token.Pos.Column - 1),
				},
				End: protocol.Position{
					Line:      uint32(c.Name.Token.Pos.Line - 1),
					Character: uint32(c.Name.Token.Pos.Column - 1 + len(c.Name.Name)),
				},
			},
		})
	}

	// 添加属性
	for _, p := range d.Properties {
		kind := protocol.SymbolKindProperty
		if p.Static {
			kind = protocol.SymbolKindField
		}
		children = append(children, protocol.DocumentSymbol{
			Name:   "$" + p.Name.Name,
			Kind:   kind,
			Detail: typeNodeToString(p.Type),
			Range: protocol.Range{
				Start: protocol.Position{
					Line:      uint32(p.Name.Token.Pos.Line - 1),
					Character: 0,
				},
				End: protocol.Position{
					Line:      uint32(p.Semicolon.Pos.Line - 1),
					Character: uint32(p.Semicolon.Pos.Column),
				},
			},
			SelectionRange: protocol.Range{
				Start: protocol.Position{
					Line:      uint32(p.Name.Token.Pos.Line - 1),
					Character: uint32(p.Name.Token.Pos.Column - 1),
				},
				End: protocol.Position{
					Line:      uint32(p.Name.Token.Pos.Line - 1),
					Character: uint32(p.Name.Token.Pos.Column - 1 + len(p.Name.Name) + 1),
				},
			},
		})
	}

	// 添加方法
	for _, m := range d.Methods {
		kind := protocol.SymbolKindMethod
		if m.Name.Name == "__construct" {
			kind = protocol.SymbolKindConstructor
		}

		endLine := m.FuncToken.Pos.Line
		if m.Body != nil {
			endLine = m.Body.RBrace.Pos.Line
		}

		children = append(children, protocol.DocumentSymbol{
			Name:   m.Name.Name,
			Kind:   kind,
			Detail: formatMethodSignatureShort(m),
			Range: protocol.Range{
				Start: protocol.Position{
					Line:      uint32(m.FuncToken.Pos.Line - 1),
					Character: 0,
				},
				End: protocol.Position{
					Line:      uint32(endLine - 1),
					Character: 0,
				},
			},
			SelectionRange: protocol.Range{
				Start: protocol.Position{
					Line:      uint32(m.Name.Token.Pos.Line - 1),
					Character: uint32(m.Name.Token.Pos.Column - 1),
				},
				End: protocol.Position{
					Line:      uint32(m.Name.Token.Pos.Line - 1),
					Character: uint32(m.Name.Token.Pos.Column - 1 + len(m.Name.Name)),
				},
			},
		})
	}

	// 构建类符号
	detail := ""
	if d.Extends != nil {
		detail = "extends " + d.Extends.Name
	}

	return &protocol.DocumentSymbol{
		Name:     d.Name.Name,
		Kind:     protocol.SymbolKindClass,
		Detail:   detail,
		Children: children,
		Range: protocol.Range{
			Start: protocol.Position{
				Line:      uint32(d.ClassToken.Pos.Line - 1),
				Character: 0,
			},
			End: protocol.Position{
				Line:      uint32(d.RBrace.Pos.Line - 1),
				Character: uint32(d.RBrace.Pos.Column),
			},
		},
		SelectionRange: protocol.Range{
			Start: protocol.Position{
				Line:      uint32(d.Name.Token.Pos.Line - 1),
				Character: uint32(d.Name.Token.Pos.Column - 1),
			},
			End: protocol.Position{
				Line:      uint32(d.Name.Token.Pos.Line - 1),
				Character: uint32(d.Name.Token.Pos.Column - 1 + len(d.Name.Name)),
			},
		},
	}
}

// interfaceToSymbol 将接口声明转换为符号
func (s *Server) interfaceToSymbol(d *ast.InterfaceDecl) *protocol.DocumentSymbol {
	var children []protocol.DocumentSymbol

	// 添加方法
	for _, m := range d.Methods {
		children = append(children, protocol.DocumentSymbol{
			Name:   m.Name.Name,
			Kind:   protocol.SymbolKindMethod,
			Detail: formatMethodSignatureShort(m),
			Range: protocol.Range{
				Start: protocol.Position{
					Line:      uint32(m.FuncToken.Pos.Line - 1),
					Character: 0,
				},
				End: protocol.Position{
					Line:      uint32(m.FuncToken.Pos.Line - 1),
					Character: 100,
				},
			},
			SelectionRange: protocol.Range{
				Start: protocol.Position{
					Line:      uint32(m.Name.Token.Pos.Line - 1),
					Character: uint32(m.Name.Token.Pos.Column - 1),
				},
				End: protocol.Position{
					Line:      uint32(m.Name.Token.Pos.Line - 1),
					Character: uint32(m.Name.Token.Pos.Column - 1 + len(m.Name.Name)),
				},
			},
		})
	}

	return &protocol.DocumentSymbol{
		Name:     d.Name.Name,
		Kind:     protocol.SymbolKindInterface,
		Children: children,
		Range: protocol.Range{
			Start: protocol.Position{
				Line:      uint32(d.InterfaceToken.Pos.Line - 1),
				Character: 0,
			},
			End: protocol.Position{
				Line:      uint32(d.RBrace.Pos.Line - 1),
				Character: uint32(d.RBrace.Pos.Column),
			},
		},
		SelectionRange: protocol.Range{
			Start: protocol.Position{
				Line:      uint32(d.Name.Token.Pos.Line - 1),
				Character: uint32(d.Name.Token.Pos.Column - 1),
			},
			End: protocol.Position{
				Line:      uint32(d.Name.Token.Pos.Line - 1),
				Character: uint32(d.Name.Token.Pos.Column - 1 + len(d.Name.Name)),
			},
		},
	}
}

// enumToSymbol 将枚举声明转换为符号
func (s *Server) enumToSymbol(d *ast.EnumDecl) *protocol.DocumentSymbol {
	var children []protocol.DocumentSymbol

	// 添加枚举值
	for _, c := range d.Cases {
		children = append(children, protocol.DocumentSymbol{
			Name: c.Name.Name,
			Kind: protocol.SymbolKindEnumMember,
			Range: protocol.Range{
				Start: protocol.Position{
					Line:      uint32(c.Name.Token.Pos.Line - 1),
					Character: 0,
				},
				End: protocol.Position{
					Line:      uint32(c.Name.Token.Pos.Line - 1),
					Character: uint32(len("case " + c.Name.Name)),
				},
			},
			SelectionRange: protocol.Range{
				Start: protocol.Position{
					Line:      uint32(c.Name.Token.Pos.Line - 1),
					Character: uint32(c.Name.Token.Pos.Column - 1),
				},
				End: protocol.Position{
					Line:      uint32(c.Name.Token.Pos.Line - 1),
					Character: uint32(c.Name.Token.Pos.Column - 1 + len(c.Name.Name)),
				},
			},
		})
	}

	return &protocol.DocumentSymbol{
		Name:     d.Name.Name,
		Kind:     protocol.SymbolKindEnum,
		Children: children,
		Range: protocol.Range{
			Start: protocol.Position{
				Line:      uint32(d.EnumToken.Pos.Line - 1),
				Character: 0,
			},
			End: protocol.Position{
				Line:      uint32(d.RBrace.Pos.Line - 1),
				Character: uint32(d.RBrace.Pos.Column),
			},
		},
		SelectionRange: protocol.Range{
			Start: protocol.Position{
				Line:      uint32(d.Name.Token.Pos.Line - 1),
				Character: uint32(d.Name.Token.Pos.Column - 1),
			},
			End: protocol.Position{
				Line:      uint32(d.Name.Token.Pos.Line - 1),
				Character: uint32(d.Name.Token.Pos.Column - 1 + len(d.Name.Name)),
			},
		},
	}
}

// handleWorkspaceSymbol 处理工作区符号请求
func (s *Server) handleWorkspaceSymbol(id json.RawMessage, params json.RawMessage) {
	var p protocol.WorkspaceSymbolParams
	if err := json.Unmarshal(params, &p); err != nil {
		s.sendError(id, -32700, "Parse error")
		return
	}

	query := strings.ToLower(p.Query)

	var symbols []protocol.SymbolInformation

	// 在所有打开的文档中搜索
	for _, doc := range s.documents.GetAll() {
		docSymbols := s.getWorkspaceSymbolsFromDoc(doc, query)
		symbols = append(symbols, docSymbols...)
	}

	s.sendResult(id, symbols)
}

// getWorkspaceSymbolsFromDoc 从文档获取工作区符号
func (s *Server) getWorkspaceSymbolsFromDoc(doc *Document, query string) []protocol.SymbolInformation {
	var symbols []protocol.SymbolInformation

	astFile := doc.GetAST()
	if astFile == nil {
		return symbols
	}

	for _, decl := range astFile.Declarations {
		switch d := decl.(type) {
		case *ast.ClassDecl:
			if query == "" || strings.Contains(strings.ToLower(d.Name.Name), query) {
				symbols = append(symbols, protocol.SymbolInformation{
					Name: d.Name.Name,
					Kind: protocol.SymbolKindClass,
					Location: protocol.Location{
						URI: protocol.DocumentURI(doc.URI),
						Range: protocol.Range{
							Start: protocol.Position{
								Line:      uint32(d.Name.Token.Pos.Line - 1),
								Character: uint32(d.Name.Token.Pos.Column - 1),
							},
							End: protocol.Position{
								Line:      uint32(d.Name.Token.Pos.Line - 1),
								Character: uint32(d.Name.Token.Pos.Column - 1 + len(d.Name.Name)),
							},
						},
					},
				})
			}
			// 搜索方法
			for _, m := range d.Methods {
				if query == "" || strings.Contains(strings.ToLower(m.Name.Name), query) {
					symbols = append(symbols, protocol.SymbolInformation{
						Name:          m.Name.Name,
						Kind:          protocol.SymbolKindMethod,
						ContainerName: d.Name.Name,
						Location: protocol.Location{
							URI: protocol.DocumentURI(doc.URI),
							Range: protocol.Range{
								Start: protocol.Position{
									Line:      uint32(m.Name.Token.Pos.Line - 1),
									Character: uint32(m.Name.Token.Pos.Column - 1),
								},
								End: protocol.Position{
									Line:      uint32(m.Name.Token.Pos.Line - 1),
									Character: uint32(m.Name.Token.Pos.Column - 1 + len(m.Name.Name)),
								},
							},
						},
					})
				}
			}
		case *ast.InterfaceDecl:
			if query == "" || strings.Contains(strings.ToLower(d.Name.Name), query) {
				symbols = append(symbols, protocol.SymbolInformation{
					Name: d.Name.Name,
					Kind: protocol.SymbolKindInterface,
					Location: protocol.Location{
						URI: protocol.DocumentURI(doc.URI),
						Range: protocol.Range{
							Start: protocol.Position{
								Line:      uint32(d.Name.Token.Pos.Line - 1),
								Character: uint32(d.Name.Token.Pos.Column - 1),
							},
							End: protocol.Position{
								Line:      uint32(d.Name.Token.Pos.Line - 1),
								Character: uint32(d.Name.Token.Pos.Column - 1 + len(d.Name.Name)),
							},
						},
					},
				})
			}
		case *ast.EnumDecl:
			if query == "" || strings.Contains(strings.ToLower(d.Name.Name), query) {
				symbols = append(symbols, protocol.SymbolInformation{
					Name: d.Name.Name,
					Kind: protocol.SymbolKindEnum,
					Location: protocol.Location{
						URI: protocol.DocumentURI(doc.URI),
						Range: protocol.Range{
							Start: protocol.Position{
								Line:      uint32(d.Name.Token.Pos.Line - 1),
								Character: uint32(d.Name.Token.Pos.Column - 1),
							},
							End: protocol.Position{
								Line:      uint32(d.Name.Token.Pos.Line - 1),
								Character: uint32(d.Name.Token.Pos.Column - 1 + len(d.Name.Name)),
							},
						},
					},
				})
			}
		}
	}

	return symbols
}
