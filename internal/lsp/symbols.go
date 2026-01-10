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

	// 从工作区索引中搜索
	if s.workspace != nil {
		for _, indexed := range s.workspace.GetAllFiles() {
			if indexed.AST == nil {
				continue
			}
			// 跳过已打开的文档（避免重复）
			if s.documents.Get(indexed.URI) != nil {
				continue
			}
			indexedSymbols := s.getSymbolsFromIndexedFile(indexed, query)
			symbols = append(symbols, indexedSymbols...)
		}
	}

	// 对结果进行排序：完全匹配 > 前缀匹配 > 模糊匹配
	sortSymbolsByRelevance(symbols, query)

	// 限制结果数量
	if len(symbols) > 100 {
		symbols = symbols[:100]
	}

	s.sendResult(id, symbols)
}

// getSymbolsFromIndexedFile 从已索引文件获取符号
func (s *Server) getSymbolsFromIndexedFile(indexed *IndexedFile, query string) []protocol.SymbolInformation {
	var symbols []protocol.SymbolInformation

	if indexed.AST == nil {
		return symbols
	}

	for _, decl := range indexed.AST.Declarations {
		switch d := decl.(type) {
		case *ast.ClassDecl:
			if matchSymbol(d.Name.Name, query) {
				symbols = append(symbols, protocol.SymbolInformation{
					Name: d.Name.Name,
					Kind: protocol.SymbolKindClass,
					Location: protocol.Location{
						URI: protocol.DocumentURI(indexed.URI),
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
				if matchSymbol(m.Name.Name, query) {
					symbols = append(symbols, protocol.SymbolInformation{
						Name:          m.Name.Name,
						Kind:          protocol.SymbolKindMethod,
						ContainerName: d.Name.Name,
						Location: protocol.Location{
							URI: protocol.DocumentURI(indexed.URI),
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
			if matchSymbol(d.Name.Name, query) {
				symbols = append(symbols, protocol.SymbolInformation{
					Name: d.Name.Name,
					Kind: protocol.SymbolKindInterface,
					Location: protocol.Location{
						URI: protocol.DocumentURI(indexed.URI),
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
			if matchSymbol(d.Name.Name, query) {
				symbols = append(symbols, protocol.SymbolInformation{
					Name: d.Name.Name,
					Kind: protocol.SymbolKindEnum,
					Location: protocol.Location{
						URI: protocol.DocumentURI(indexed.URI),
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

// matchSymbol 检查符号名是否匹配查询（支持模糊匹配）
func matchSymbol(name, query string) bool {
	if query == "" {
		return true
	}

	nameLower := strings.ToLower(name)
	queryLower := strings.ToLower(query)

	// 精确匹配
	if nameLower == queryLower {
		return true
	}

	// 前缀匹配
	if strings.HasPrefix(nameLower, queryLower) {
		return true
	}

	// 包含匹配
	if strings.Contains(nameLower, queryLower) {
		return true
	}

	// 模糊匹配（驼峰匹配）
	if fuzzyMatchCamelCase(name, query) {
		return true
	}

	// 模糊匹配（首字母匹配）
	if fuzzyMatchInitials(name, query) {
		return true
	}

	return false
}

// fuzzyMatchCamelCase 驼峰模糊匹配
// 例如：query "gua" 匹配 "getUserAccount"
func fuzzyMatchCamelCase(name, query string) bool {
	queryLower := strings.ToLower(query)
	nameLower := strings.ToLower(name)

	qi := 0
	for ni := 0; ni < len(nameLower) && qi < len(queryLower); ni++ {
		if nameLower[ni] == queryLower[qi] {
			qi++
		}
	}

	return qi == len(queryLower)
}

// fuzzyMatchInitials 首字母匹配
// 例如：query "gua" 匹配 "GetUserAccount"
func fuzzyMatchInitials(name, query string) bool {
	queryLower := strings.ToLower(query)

	// 提取驼峰命名的首字母
	var initials strings.Builder
	for i, c := range name {
		if i == 0 || (c >= 'A' && c <= 'Z') {
			initials.WriteRune(c)
		}
	}

	initialsLower := strings.ToLower(initials.String())
	return strings.HasPrefix(initialsLower, queryLower)
}

// sortSymbolsByRelevance 按相关性排序符号
func sortSymbolsByRelevance(symbols []protocol.SymbolInformation, query string) {
	if query == "" {
		return
	}

	queryLower := strings.ToLower(query)

	// 计算每个符号的得分
	scores := make([]int, len(symbols))
	for i, sym := range symbols {
		nameLower := strings.ToLower(sym.Name)

		if nameLower == queryLower {
			scores[i] = 100 // 完全匹配
		} else if strings.HasPrefix(nameLower, queryLower) {
			scores[i] = 80 // 前缀匹配
		} else if strings.Contains(nameLower, queryLower) {
			scores[i] = 60 // 包含匹配
		} else {
			scores[i] = 40 // 模糊匹配
		}

		// 类型加权：类 > 接口 > 方法 > 其他
		switch sym.Kind {
		case protocol.SymbolKindClass:
			scores[i] += 10
		case protocol.SymbolKindInterface:
			scores[i] += 8
		case protocol.SymbolKindMethod:
			scores[i] += 5
		}
	}

	// 冒泡排序（按得分降序）
	for i := 0; i < len(symbols)-1; i++ {
		for j := 0; j < len(symbols)-1-i; j++ {
			if scores[j] < scores[j+1] {
				symbols[j], symbols[j+1] = symbols[j+1], symbols[j]
				scores[j], scores[j+1] = scores[j+1], scores[j]
			}
		}
	}
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
			if matchSymbol(d.Name.Name, query) {
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
				if matchSymbol(m.Name.Name, query) {
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
			if matchSymbol(d.Name.Name, query) {
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
			if matchSymbol(d.Name.Name, query) {
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
