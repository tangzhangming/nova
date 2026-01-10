package lsp

import (
	"encoding/json"

	"github.com/tangzhangming/nova/internal/ast"
	"go.lsp.dev/protocol"
)

// TypeHierarchyItem 类型层次项
type TypeHierarchyItem struct {
	Name           string               `json:"name"`
	Kind           protocol.SymbolKind  `json:"kind"`
	Tags           []protocol.SymbolTag `json:"tags,omitempty"`
	Detail         string               `json:"detail,omitempty"`
	URI            protocol.DocumentURI `json:"uri"`
	Range          protocol.Range       `json:"range"`
	SelectionRange protocol.Range       `json:"selectionRange"`
	Data           interface{}          `json:"data,omitempty"`
}

// TypeHierarchyPrepareParams 准备类型层次参数
type TypeHierarchyPrepareParams struct {
	TextDocument protocol.TextDocumentIdentifier `json:"textDocument"`
	Position     protocol.Position               `json:"position"`
}

// TypeHierarchySupertypesParams 父类型参数
type TypeHierarchySupertypesParams struct {
	Item TypeHierarchyItem `json:"item"`
}

// TypeHierarchySubtypesParams 子类型参数
type TypeHierarchySubtypesParams struct {
	Item TypeHierarchyItem `json:"item"`
}

// handleTypeHierarchyPrepare 处理准备类型层次请求
func (s *Server) handleTypeHierarchyPrepare(id json.RawMessage, params json.RawMessage) {
	var p TypeHierarchyPrepareParams
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

	// 获取当前位置的符号
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

	// 查找类型定义
	item := s.findTypeHierarchyItem(astFile, word, docURI)
	if item == nil {
		s.sendResult(id, nil)
		return
	}

	s.sendResult(id, []TypeHierarchyItem{*item})
}

// handleTypeHierarchySupertypes 处理父类型请求
func (s *Server) handleTypeHierarchySupertypes(id json.RawMessage, params json.RawMessage) {
	var p TypeHierarchySupertypesParams
	if err := json.Unmarshal(params, &p); err != nil {
		s.sendError(id, -32700, "Parse error")
		return
	}

	supertypes := s.findSupertypes(p.Item)
	s.sendResult(id, supertypes)
}

// handleTypeHierarchySubtypes 处理子类型请求
func (s *Server) handleTypeHierarchySubtypes(id json.RawMessage, params json.RawMessage) {
	var p TypeHierarchySubtypesParams
	if err := json.Unmarshal(params, &p); err != nil {
		s.sendError(id, -32700, "Parse error")
		return
	}

	subtypes := s.findSubtypes(p.Item)
	s.sendResult(id, subtypes)
}

// findTypeHierarchyItem 查找类型层次项
func (s *Server) findTypeHierarchyItem(file *ast.File, name string, docURI string) *TypeHierarchyItem {
	for _, decl := range file.Declarations {
		switch d := decl.(type) {
		case *ast.ClassDecl:
			if d.Name.Name == name {
				return &TypeHierarchyItem{
					Name:   d.Name.Name,
					Kind:   protocol.SymbolKindClass,
					URI:    protocol.DocumentURI(docURI),
					Range: protocol.Range{
						Start: protocol.Position{
							Line:      uint32(d.ClassToken.Pos.Line - 1),
							Character: uint32(d.ClassToken.Pos.Column - 1),
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
					Data: map[string]interface{}{
						"type": "class",
					},
				}
			}
		case *ast.InterfaceDecl:
			if d.Name.Name == name {
				return &TypeHierarchyItem{
					Name:   d.Name.Name,
					Kind:   protocol.SymbolKindInterface,
					URI:    protocol.DocumentURI(docURI),
					Range: protocol.Range{
						Start: protocol.Position{
							Line:      uint32(d.InterfaceToken.Pos.Line - 1),
							Character: uint32(d.InterfaceToken.Pos.Column - 1),
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
					Data: map[string]interface{}{
						"type": "interface",
					},
				}
			}
		}
	}
	return nil
}

// findSupertypes 查找父类型
func (s *Server) findSupertypes(item TypeHierarchyItem) []TypeHierarchyItem {
	var supertypes []TypeHierarchyItem

	// 获取文档
	doc := s.documents.Get(string(item.URI))
	if doc == nil {
		return supertypes
	}

	astFile := doc.GetAST()
	if astFile == nil {
		return supertypes
	}

	// 查找类或接口
	for _, decl := range astFile.Declarations {
		switch d := decl.(type) {
		case *ast.ClassDecl:
			if d.Name.Name == item.Name {
				// 添加父类
				if d.Extends != nil {
					superItem := s.findTypeDefinition(d.Extends.Name)
					if superItem != nil {
						supertypes = append(supertypes, *superItem)
					} else {
						// 创建一个未解析的父类项
						supertypes = append(supertypes, TypeHierarchyItem{
							Name: d.Extends.Name,
							Kind: protocol.SymbolKindClass,
							URI:  item.URI,
							Range: protocol.Range{
								Start: protocol.Position{Line: 0, Character: 0},
								End:   protocol.Position{Line: 0, Character: 0},
							},
							SelectionRange: protocol.Range{
								Start: protocol.Position{Line: 0, Character: 0},
								End:   protocol.Position{Line: 0, Character: 0},
							},
						})
					}
				}

				// 添加实现的接口
				for _, impl := range d.Implements {
					implName := impl.String()
					if simpleType, ok := impl.(*ast.SimpleType); ok {
						implName = simpleType.Name
					}
					implItem := s.findTypeDefinition(implName)
					if implItem != nil {
						supertypes = append(supertypes, *implItem)
					} else {
						supertypes = append(supertypes, TypeHierarchyItem{
							Name: implName,
							Kind: protocol.SymbolKindInterface,
							URI:  item.URI,
							Range: protocol.Range{
								Start: protocol.Position{Line: 0, Character: 0},
								End:   protocol.Position{Line: 0, Character: 0},
							},
							SelectionRange: protocol.Range{
								Start: protocol.Position{Line: 0, Character: 0},
								End:   protocol.Position{Line: 0, Character: 0},
							},
						})
					}
				}
			}

		case *ast.InterfaceDecl:
			if d.Name.Name == item.Name {
				// 添加继承的接口
				for _, ext := range d.Extends {
					extName := ext.String()
					if simpleType, ok := ext.(*ast.SimpleType); ok {
						extName = simpleType.Name
					}
					extItem := s.findTypeDefinition(extName)
					if extItem != nil {
						supertypes = append(supertypes, *extItem)
					} else {
						supertypes = append(supertypes, TypeHierarchyItem{
							Name: extName,
							Kind: protocol.SymbolKindInterface,
							URI:  item.URI,
							Range: protocol.Range{
								Start: protocol.Position{Line: 0, Character: 0},
								End:   protocol.Position{Line: 0, Character: 0},
							},
							SelectionRange: protocol.Range{
								Start: protocol.Position{Line: 0, Character: 0},
								End:   protocol.Position{Line: 0, Character: 0},
							},
						})
					}
				}
			}
		}
	}

	return supertypes
}

// findSubtypes 查找子类型
func (s *Server) findSubtypes(item TypeHierarchyItem) []TypeHierarchyItem {
	var subtypes []TypeHierarchyItem

	// 在所有打开的文档中查找
	for _, doc := range s.documents.GetAll() {
		astFile := doc.GetAST()
		if astFile == nil {
			continue
		}

		for _, decl := range astFile.Declarations {
			switch d := decl.(type) {
			case *ast.ClassDecl:
				// 检查是否继承自目标类型
				if d.Extends != nil && d.Extends.Name == item.Name {
					subtypes = append(subtypes, TypeHierarchyItem{
						Name:   d.Name.Name,
						Kind:   protocol.SymbolKindClass,
						URI:    protocol.DocumentURI(doc.URI),
						Range: protocol.Range{
							Start: protocol.Position{
								Line:      uint32(d.ClassToken.Pos.Line - 1),
								Character: uint32(d.ClassToken.Pos.Column - 1),
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
					})
				}

				// 检查是否实现目标接口
				for _, impl := range d.Implements {
					implName := impl.String()
					if simpleType, ok := impl.(*ast.SimpleType); ok {
						implName = simpleType.Name
					}
					if implName == item.Name {
						subtypes = append(subtypes, TypeHierarchyItem{
							Name:   d.Name.Name,
							Kind:   protocol.SymbolKindClass,
							URI:    protocol.DocumentURI(doc.URI),
							Range: protocol.Range{
								Start: protocol.Position{
									Line:      uint32(d.ClassToken.Pos.Line - 1),
									Character: uint32(d.ClassToken.Pos.Column - 1),
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
						})
						break
					}
				}

			case *ast.InterfaceDecl:
				// 检查是否继承自目标接口
				for _, ext := range d.Extends {
					extName := ext.String()
					if simpleType, ok := ext.(*ast.SimpleType); ok {
						extName = simpleType.Name
					}
					if extName == item.Name {
						subtypes = append(subtypes, TypeHierarchyItem{
							Name:   d.Name.Name,
							Kind:   protocol.SymbolKindInterface,
							URI:    protocol.DocumentURI(doc.URI),
							Range: protocol.Range{
								Start: protocol.Position{
									Line:      uint32(d.InterfaceToken.Pos.Line - 1),
									Character: uint32(d.InterfaceToken.Pos.Column - 1),
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
						})
						break
					}
				}
			}
		}
	}

	return subtypes
}

// findTypeDefinition 查找类型定义
func (s *Server) findTypeDefinition(typeName string) *TypeHierarchyItem {
	for _, doc := range s.documents.GetAll() {
		astFile := doc.GetAST()
		if astFile == nil {
			continue
		}

		for _, decl := range astFile.Declarations {
			switch d := decl.(type) {
			case *ast.ClassDecl:
				if d.Name.Name == typeName {
					return &TypeHierarchyItem{
						Name:   d.Name.Name,
						Kind:   protocol.SymbolKindClass,
						URI:    protocol.DocumentURI(doc.URI),
						Range: protocol.Range{
							Start: protocol.Position{
								Line:      uint32(d.ClassToken.Pos.Line - 1),
								Character: uint32(d.ClassToken.Pos.Column - 1),
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
			case *ast.InterfaceDecl:
				if d.Name.Name == typeName {
					return &TypeHierarchyItem{
						Name:   d.Name.Name,
						Kind:   protocol.SymbolKindInterface,
						URI:    protocol.DocumentURI(doc.URI),
						Range: protocol.Range{
							Start: protocol.Position{
								Line:      uint32(d.InterfaceToken.Pos.Line - 1),
								Character: uint32(d.InterfaceToken.Pos.Column - 1),
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
			}
		}
	}

	return nil
}
