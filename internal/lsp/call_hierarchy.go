package lsp

import (
	"encoding/json"

	"github.com/tangzhangming/nova/internal/ast"
	"go.lsp.dev/protocol"
)

// CallHierarchyItem 调用层次项
type CallHierarchyItem struct {
	Name           string                 `json:"name"`
	Kind           protocol.SymbolKind    `json:"kind"`
	Tags           []protocol.SymbolTag   `json:"tags,omitempty"`
	Detail         string                 `json:"detail,omitempty"`
	URI            protocol.DocumentURI   `json:"uri"`
	Range          protocol.Range         `json:"range"`
	SelectionRange protocol.Range         `json:"selectionRange"`
	Data           interface{}            `json:"data,omitempty"`
}

// CallHierarchyIncomingCall 调用来源
type CallHierarchyIncomingCall struct {
	From       CallHierarchyItem `json:"from"`
	FromRanges []protocol.Range  `json:"fromRanges"`
}

// CallHierarchyOutgoingCall 调用目标
type CallHierarchyOutgoingCall struct {
	To         CallHierarchyItem `json:"to"`
	FromRanges []protocol.Range  `json:"fromRanges"`
}

// CallHierarchyPrepareParams 准备调用层次参数
type CallHierarchyPrepareParams struct {
	TextDocument protocol.TextDocumentIdentifier `json:"textDocument"`
	Position     protocol.Position               `json:"position"`
}

// CallHierarchyIncomingCallsParams 调用来源参数
type CallHierarchyIncomingCallsParams struct {
	Item CallHierarchyItem `json:"item"`
}

// CallHierarchyOutgoingCallsParams 调用目标参数
type CallHierarchyOutgoingCallsParams struct {
	Item CallHierarchyItem `json:"item"`
}

// handleCallHierarchyPrepare 处理准备调用层次请求
func (s *Server) handleCallHierarchyPrepare(id json.RawMessage, params json.RawMessage) {
	var p CallHierarchyPrepareParams
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

	// 查找函数/方法定义
	item := s.findCallHierarchyItem(astFile, word, docURI, line, character)
	if item == nil {
		s.sendResult(id, nil)
		return
	}

	s.sendResult(id, []CallHierarchyItem{*item})
}

// handleCallHierarchyIncomingCalls 处理调用来源请求
func (s *Server) handleCallHierarchyIncomingCalls(id json.RawMessage, params json.RawMessage) {
	var p CallHierarchyIncomingCallsParams
	if err := json.Unmarshal(params, &p); err != nil {
		s.sendError(id, -32700, "Parse error")
		return
	}

	calls := s.findIncomingCalls(p.Item)
	s.sendResult(id, calls)
}

// handleCallHierarchyOutgoingCalls 处理调用目标请求
func (s *Server) handleCallHierarchyOutgoingCalls(id json.RawMessage, params json.RawMessage) {
	var p CallHierarchyOutgoingCallsParams
	if err := json.Unmarshal(params, &p); err != nil {
		s.sendError(id, -32700, "Parse error")
		return
	}

	calls := s.findOutgoingCalls(p.Item)
	s.sendResult(id, calls)
}

// findCallHierarchyItem 查找调用层次项
func (s *Server) findCallHierarchyItem(file *ast.File, name string, docURI string, line, character int) *CallHierarchyItem {
	// 查找函数定义
	for _, decl := range file.Declarations {
		switch d := decl.(type) {
		case *ast.ClassDecl:
			for _, method := range d.Methods {
				if method.Name.Name == name {
					return &CallHierarchyItem{
						Name:   method.Name.Name,
						Kind:   protocol.SymbolKindMethod,
						Detail: d.Name.Name,
						URI:    protocol.DocumentURI(docURI),
						Range: protocol.Range{
							Start: protocol.Position{
								Line:      uint32(method.FuncToken.Pos.Line - 1),
								Character: uint32(method.FuncToken.Pos.Column - 1),
							},
							End: protocol.Position{
								Line:      uint32(method.Body.RBrace.Pos.Line - 1),
								Character: uint32(method.Body.RBrace.Pos.Column),
							},
						},
						SelectionRange: protocol.Range{
							Start: protocol.Position{
								Line:      uint32(method.Name.Token.Pos.Line - 1),
								Character: uint32(method.Name.Token.Pos.Column - 1),
							},
							End: protocol.Position{
								Line:      uint32(method.Name.Token.Pos.Line - 1),
								Character: uint32(method.Name.Token.Pos.Column - 1 + len(method.Name.Name)),
							},
						},
						Data: map[string]interface{}{
							"className":  d.Name.Name,
							"methodName": method.Name.Name,
						},
					}
				}
			}
		}
	}

	return nil
}

// findIncomingCalls 查找调用来源
func (s *Server) findIncomingCalls(item CallHierarchyItem) []CallHierarchyIncomingCall {
	var calls []CallHierarchyIncomingCall

	targetName := item.Name
	targetClass := ""
	if data, ok := item.Data.(map[string]interface{}); ok {
		if cn, ok := data["className"].(string); ok {
			targetClass = cn
		}
	}

	// 在所有打开的文档中查找调用
	for _, doc := range s.documents.GetAll() {
		astFile := doc.GetAST()
		if astFile == nil {
			continue
		}

		// 查找所有调用该方法/函数的地方
		for _, decl := range astFile.Declarations {
			if classDecl, ok := decl.(*ast.ClassDecl); ok {
				for _, method := range classDecl.Methods {
					if method.Body != nil {
						callRanges := findCallsInStmt(method.Body, targetName, targetClass)
						if len(callRanges) > 0 {
							callerItem := CallHierarchyItem{
								Name:   method.Name.Name,
								Kind:   protocol.SymbolKindMethod,
								Detail: classDecl.Name.Name,
								URI:    protocol.DocumentURI(doc.URI),
								Range: protocol.Range{
									Start: protocol.Position{
										Line:      uint32(method.FuncToken.Pos.Line - 1),
										Character: uint32(method.FuncToken.Pos.Column - 1),
									},
									End: protocol.Position{
										Line:      uint32(method.Body.RBrace.Pos.Line - 1),
										Character: uint32(method.Body.RBrace.Pos.Column),
									},
								},
								SelectionRange: protocol.Range{
									Start: protocol.Position{
										Line:      uint32(method.Name.Token.Pos.Line - 1),
										Character: uint32(method.Name.Token.Pos.Column - 1),
									},
									End: protocol.Position{
										Line:      uint32(method.Name.Token.Pos.Line - 1),
										Character: uint32(method.Name.Token.Pos.Column - 1 + len(method.Name.Name)),
									},
								},
							}
							calls = append(calls, CallHierarchyIncomingCall{
								From:       callerItem,
								FromRanges: callRanges,
							})
						}
					}
				}
			}
		}

		// 检查顶层语句中的调用
		for _, stmt := range astFile.Statements {
			callRanges := findCallsInStmt(stmt, targetName, targetClass)
			if len(callRanges) > 0 {
				// 顶层代码
				callerItem := CallHierarchyItem{
					Name:   "<main>",
					Kind:   protocol.SymbolKindFunction,
					Detail: "",
					URI:    protocol.DocumentURI(doc.URI),
					Range: protocol.Range{
						Start: protocol.Position{Line: 0, Character: 0},
						End:   protocol.Position{Line: uint32(len(doc.Lines) - 1), Character: 0},
					},
					SelectionRange: protocol.Range{
						Start: protocol.Position{Line: 0, Character: 0},
						End:   protocol.Position{Line: 0, Character: 0},
					},
				}
				calls = append(calls, CallHierarchyIncomingCall{
					From:       callerItem,
					FromRanges: callRanges,
				})
			}
		}
	}

	return calls
}

// findOutgoingCalls 查找调用目标
func (s *Server) findOutgoingCalls(item CallHierarchyItem) []CallHierarchyOutgoingCall {
	var calls []CallHierarchyOutgoingCall

	// 获取文档
	doc := s.documents.Get(string(item.URI))
	if doc == nil {
		return calls
	}

	astFile := doc.GetAST()
	if astFile == nil {
		return calls
	}

	// 查找方法体
	var methodBody *ast.BlockStmt
	for _, decl := range astFile.Declarations {
		if classDecl, ok := decl.(*ast.ClassDecl); ok {
			for _, method := range classDecl.Methods {
				if method.Name.Name == item.Name {
					methodBody = method.Body
					break
				}
			}
		}
	}

	if methodBody == nil {
		return calls
	}

	// 收集所有调用
	outgoingCalls := collectOutgoingCalls(methodBody)

	// 为每个调用创建层次项
	for callName, callInfo := range outgoingCalls {
		targetItem := CallHierarchyItem{
			Name: callName,
			Kind: protocol.SymbolKindFunction,
			URI:  item.URI, // 简化处理，使用同一URI
			Range: protocol.Range{
				Start: protocol.Position{Line: 0, Character: 0},
				End:   protocol.Position{Line: 0, Character: 0},
			},
			SelectionRange: protocol.Range{
				Start: protocol.Position{Line: 0, Character: 0},
				End:   protocol.Position{Line: 0, Character: 0},
			},
		}

		// 尝试查找调用目标的定义位置
		if found := s.findFunctionDefinition(callName, string(item.URI)); found != nil {
			targetItem = *found
		}

		calls = append(calls, CallHierarchyOutgoingCall{
			To:         targetItem,
			FromRanges: callInfo.ranges,
		})
	}

	return calls
}

// callInfo 调用信息
type callInfo struct {
	ranges []protocol.Range
}

// collectOutgoingCalls 收集语句中的出站调用
func collectOutgoingCalls(stmt ast.Statement) map[string]*callInfo {
	calls := make(map[string]*callInfo)

	if stmt == nil {
		return calls
	}

	switch s := stmt.(type) {
	case *ast.BlockStmt:
		for _, inner := range s.Statements {
			innerCalls := collectOutgoingCalls(inner)
			mergeCalls(calls, innerCalls)
		}
	case *ast.ExprStmt:
		exprCalls := collectCallsFromExpr(s.Expr)
		mergeCalls(calls, exprCalls)
	case *ast.VarDeclStmt:
		if s.Value != nil {
			exprCalls := collectCallsFromExpr(s.Value)
			mergeCalls(calls, exprCalls)
		}
	case *ast.IfStmt:
		exprCalls := collectCallsFromExpr(s.Condition)
		mergeCalls(calls, exprCalls)
		thenCalls := collectOutgoingCalls(s.Then)
		mergeCalls(calls, thenCalls)
		if s.Else != nil {
			elseCalls := collectOutgoingCalls(s.Else)
			mergeCalls(calls, elseCalls)
		}
	case *ast.ForStmt:
		if s.Init != nil {
			initCalls := collectOutgoingCalls(s.Init)
			mergeCalls(calls, initCalls)
		}
		if s.Condition != nil {
			condCalls := collectCallsFromExpr(s.Condition)
			mergeCalls(calls, condCalls)
		}
		if s.Post != nil {
			postCalls := collectCallsFromExpr(s.Post)
			mergeCalls(calls, postCalls)
		}
		bodyCalls := collectOutgoingCalls(s.Body)
		mergeCalls(calls, bodyCalls)
	case *ast.ForeachStmt:
		iterCalls := collectCallsFromExpr(s.Iterable)
		mergeCalls(calls, iterCalls)
		bodyCalls := collectOutgoingCalls(s.Body)
		mergeCalls(calls, bodyCalls)
	case *ast.WhileStmt:
		condCalls := collectCallsFromExpr(s.Condition)
		mergeCalls(calls, condCalls)
		bodyCalls := collectOutgoingCalls(s.Body)
		mergeCalls(calls, bodyCalls)
	case *ast.ReturnStmt:
		for _, val := range s.Values {
			valCalls := collectCallsFromExpr(val)
			mergeCalls(calls, valCalls)
		}
	}

	return calls
}

// collectCallsFromExpr 从表达式收集调用
func collectCallsFromExpr(expr ast.Expression) map[string]*callInfo {
	calls := make(map[string]*callInfo)

	if expr == nil {
		return calls
	}

	switch e := expr.(type) {
	case *ast.CallExpr:
		if ident, ok := e.Function.(*ast.Identifier); ok {
			name := ident.Name
			if calls[name] == nil {
				calls[name] = &callInfo{}
			}
			calls[name].ranges = append(calls[name].ranges, protocol.Range{
				Start: protocol.Position{
					Line:      uint32(ident.Token.Pos.Line - 1),
					Character: uint32(ident.Token.Pos.Column - 1),
				},
				End: protocol.Position{
					Line:      uint32(ident.Token.Pos.Line - 1),
					Character: uint32(ident.Token.Pos.Column - 1 + len(ident.Name)),
				},
			})
		}
		// 检查参数中的调用
		for _, arg := range e.Arguments {
			argCalls := collectCallsFromExpr(arg)
			mergeCalls(calls, argCalls)
		}
	case *ast.MethodCall:
		name := e.Method.Name
		if calls[name] == nil {
			calls[name] = &callInfo{}
		}
		calls[name].ranges = append(calls[name].ranges, protocol.Range{
			Start: protocol.Position{
				Line:      uint32(e.Method.Token.Pos.Line - 1),
				Character: uint32(e.Method.Token.Pos.Column - 1),
			},
			End: protocol.Position{
				Line:      uint32(e.Method.Token.Pos.Line - 1),
				Character: uint32(e.Method.Token.Pos.Column - 1 + len(name)),
			},
		})
		// 检查对象和参数中的调用
		objCalls := collectCallsFromExpr(e.Object)
		mergeCalls(calls, objCalls)
		for _, arg := range e.Arguments {
			argCalls := collectCallsFromExpr(arg)
			mergeCalls(calls, argCalls)
		}
	case *ast.BinaryExpr:
		leftCalls := collectCallsFromExpr(e.Left)
		rightCalls := collectCallsFromExpr(e.Right)
		mergeCalls(calls, leftCalls)
		mergeCalls(calls, rightCalls)
	case *ast.UnaryExpr:
		opCalls := collectCallsFromExpr(e.Operand)
		mergeCalls(calls, opCalls)
	case *ast.AssignExpr:
		leftCalls := collectCallsFromExpr(e.Left)
		rightCalls := collectCallsFromExpr(e.Right)
		mergeCalls(calls, leftCalls)
		mergeCalls(calls, rightCalls)
	case *ast.TernaryExpr:
		condCalls := collectCallsFromExpr(e.Condition)
		thenCalls := collectCallsFromExpr(e.Then)
		elseCalls := collectCallsFromExpr(e.Else)
		mergeCalls(calls, condCalls)
		mergeCalls(calls, thenCalls)
		mergeCalls(calls, elseCalls)
	case *ast.NewExpr:
		for _, arg := range e.Arguments {
			argCalls := collectCallsFromExpr(arg)
			mergeCalls(calls, argCalls)
		}
	case *ast.ArrayLiteral:
		for _, elem := range e.Elements {
			elemCalls := collectCallsFromExpr(elem)
			mergeCalls(calls, elemCalls)
		}
	case *ast.MapLiteral:
		for _, pair := range e.Pairs {
			keyCalls := collectCallsFromExpr(pair.Key)
			valCalls := collectCallsFromExpr(pair.Value)
			mergeCalls(calls, keyCalls)
			mergeCalls(calls, valCalls)
		}
	}

	return calls
}

// mergeCalls 合并调用信息
func mergeCalls(target, source map[string]*callInfo) {
	for name, info := range source {
		if target[name] == nil {
			target[name] = &callInfo{}
		}
		target[name].ranges = append(target[name].ranges, info.ranges...)
	}
}

// findCallsInStmt 在语句中查找特定函数/方法的调用
func findCallsInStmt(stmt ast.Statement, targetName, targetClass string) []protocol.Range {
	var ranges []protocol.Range

	if stmt == nil {
		return ranges
	}

	switch s := stmt.(type) {
	case *ast.BlockStmt:
		for _, inner := range s.Statements {
			ranges = append(ranges, findCallsInStmt(inner, targetName, targetClass)...)
		}
	case *ast.ExprStmt:
		ranges = append(ranges, findCallsInExpr(s.Expr, targetName, targetClass)...)
	case *ast.VarDeclStmt:
		if s.Value != nil {
			ranges = append(ranges, findCallsInExpr(s.Value, targetName, targetClass)...)
		}
	case *ast.IfStmt:
		ranges = append(ranges, findCallsInExpr(s.Condition, targetName, targetClass)...)
		ranges = append(ranges, findCallsInStmt(s.Then, targetName, targetClass)...)
		if s.Else != nil {
			ranges = append(ranges, findCallsInStmt(s.Else, targetName, targetClass)...)
		}
	case *ast.ForStmt:
		if s.Init != nil {
			ranges = append(ranges, findCallsInStmt(s.Init, targetName, targetClass)...)
		}
		if s.Condition != nil {
			ranges = append(ranges, findCallsInExpr(s.Condition, targetName, targetClass)...)
		}
		if s.Post != nil {
			ranges = append(ranges, findCallsInExpr(s.Post, targetName, targetClass)...)
		}
		ranges = append(ranges, findCallsInStmt(s.Body, targetName, targetClass)...)
	case *ast.ForeachStmt:
		ranges = append(ranges, findCallsInExpr(s.Iterable, targetName, targetClass)...)
		ranges = append(ranges, findCallsInStmt(s.Body, targetName, targetClass)...)
	case *ast.WhileStmt:
		ranges = append(ranges, findCallsInExpr(s.Condition, targetName, targetClass)...)
		ranges = append(ranges, findCallsInStmt(s.Body, targetName, targetClass)...)
	case *ast.ReturnStmt:
		for _, val := range s.Values {
			ranges = append(ranges, findCallsInExpr(val, targetName, targetClass)...)
		}
	case *ast.TryStmt:
		ranges = append(ranges, findCallsInStmt(s.Try, targetName, targetClass)...)
		for _, catch := range s.Catches {
			ranges = append(ranges, findCallsInStmt(catch.Body, targetName, targetClass)...)
		}
	}

	return ranges
}

// findCallsInExpr 在表达式中查找特定函数/方法的调用
func findCallsInExpr(expr ast.Expression, targetName, targetClass string) []protocol.Range {
	var ranges []protocol.Range

	if expr == nil {
		return ranges
	}

	switch e := expr.(type) {
	case *ast.CallExpr:
		if ident, ok := e.Function.(*ast.Identifier); ok {
			if ident.Name == targetName && targetClass == "" {
				ranges = append(ranges, protocol.Range{
					Start: protocol.Position{
						Line:      uint32(ident.Token.Pos.Line - 1),
						Character: uint32(ident.Token.Pos.Column - 1),
					},
					End: protocol.Position{
						Line:      uint32(ident.Token.Pos.Line - 1),
						Character: uint32(ident.Token.Pos.Column - 1 + len(ident.Name)),
					},
				})
			}
		}
		for _, arg := range e.Arguments {
			ranges = append(ranges, findCallsInExpr(arg, targetName, targetClass)...)
		}
	case *ast.MethodCall:
		if e.Method.Name == targetName {
			// 如果有targetClass，可以尝试检查对象类型
			ranges = append(ranges, protocol.Range{
				Start: protocol.Position{
					Line:      uint32(e.Method.Token.Pos.Line - 1),
					Character: uint32(e.Method.Token.Pos.Column - 1),
				},
				End: protocol.Position{
					Line:      uint32(e.Method.Token.Pos.Line - 1),
					Character: uint32(e.Method.Token.Pos.Column - 1 + len(e.Method.Name)),
				},
			})
		}
		ranges = append(ranges, findCallsInExpr(e.Object, targetName, targetClass)...)
		for _, arg := range e.Arguments {
			ranges = append(ranges, findCallsInExpr(arg, targetName, targetClass)...)
		}
	case *ast.StaticAccess:
		if member, ok := e.Member.(*ast.CallExpr); ok {
			if ident, ok := member.Function.(*ast.Identifier); ok {
				if ident.Name == targetName {
					// 检查类名是否匹配
					if classIdent, ok := e.Class.(*ast.Identifier); ok {
						if targetClass == "" || classIdent.Name == targetClass {
							ranges = append(ranges, protocol.Range{
								Start: protocol.Position{
									Line:      uint32(ident.Token.Pos.Line - 1),
									Character: uint32(ident.Token.Pos.Column - 1),
								},
								End: protocol.Position{
									Line:      uint32(ident.Token.Pos.Line - 1),
									Character: uint32(ident.Token.Pos.Column - 1 + len(ident.Name)),
								},
							})
						}
					}
				}
			}
		}
	case *ast.BinaryExpr:
		ranges = append(ranges, findCallsInExpr(e.Left, targetName, targetClass)...)
		ranges = append(ranges, findCallsInExpr(e.Right, targetName, targetClass)...)
	case *ast.UnaryExpr:
		ranges = append(ranges, findCallsInExpr(e.Operand, targetName, targetClass)...)
	case *ast.AssignExpr:
		ranges = append(ranges, findCallsInExpr(e.Left, targetName, targetClass)...)
		ranges = append(ranges, findCallsInExpr(e.Right, targetName, targetClass)...)
	case *ast.TernaryExpr:
		ranges = append(ranges, findCallsInExpr(e.Condition, targetName, targetClass)...)
		ranges = append(ranges, findCallsInExpr(e.Then, targetName, targetClass)...)
		ranges = append(ranges, findCallsInExpr(e.Else, targetName, targetClass)...)
	case *ast.NewExpr:
		for _, arg := range e.Arguments {
			ranges = append(ranges, findCallsInExpr(arg, targetName, targetClass)...)
		}
	case *ast.ArrayLiteral:
		for _, elem := range e.Elements {
			ranges = append(ranges, findCallsInExpr(elem, targetName, targetClass)...)
		}
	case *ast.MapLiteral:
		for _, pair := range e.Pairs {
			ranges = append(ranges, findCallsInExpr(pair.Key, targetName, targetClass)...)
			ranges = append(ranges, findCallsInExpr(pair.Value, targetName, targetClass)...)
		}
	}

	return ranges
}

// findFunctionDefinition 查找函数定义
func (s *Server) findFunctionDefinition(name, currentURI string) *CallHierarchyItem {
	// 首先在当前文档中查找
	doc := s.documents.Get(currentURI)
	if doc != nil {
		if item := findFunctionInDoc(doc, name); item != nil {
			return item
		}
	}

	// 在其他打开的文档中查找
	for _, otherDoc := range s.documents.GetAll() {
		if otherDoc.URI == currentURI {
			continue
		}
		if item := findFunctionInDoc(otherDoc, name); item != nil {
			return item
		}
	}

	// 在工作区索引中查找
	if s.workspace != nil {
		if indexed := s.workspace.FindSymbolFile(name); indexed != nil && indexed.AST != nil {
			for _, decl := range indexed.AST.Declarations {
				if classDecl, ok := decl.(*ast.ClassDecl); ok {
					for _, method := range classDecl.Methods {
						if method.Name.Name == name {
							return &CallHierarchyItem{
								Name:   method.Name.Name,
								Kind:   protocol.SymbolKindMethod,
								Detail: classDecl.Name.Name,
								URI:    protocol.DocumentURI(indexed.URI),
								Range: protocol.Range{
									Start: protocol.Position{
										Line:      uint32(method.FuncToken.Pos.Line - 1),
										Character: uint32(method.FuncToken.Pos.Column - 1),
									},
									End: protocol.Position{
										Line:      uint32(method.FuncToken.Pos.Line - 1),
										Character: uint32(method.FuncToken.Pos.Column + 10),
									},
								},
								SelectionRange: protocol.Range{
									Start: protocol.Position{
										Line:      uint32(method.Name.Token.Pos.Line - 1),
										Character: uint32(method.Name.Token.Pos.Column - 1),
									},
									End: protocol.Position{
										Line:      uint32(method.Name.Token.Pos.Line - 1),
										Character: uint32(method.Name.Token.Pos.Column - 1 + len(method.Name.Name)),
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

// findFunctionInDoc 在文档中查找函数
func findFunctionInDoc(doc *Document, name string) *CallHierarchyItem {
	astFile := doc.GetAST()
	if astFile == nil {
		return nil
	}

	for _, decl := range astFile.Declarations {
		if classDecl, ok := decl.(*ast.ClassDecl); ok {
			for _, method := range classDecl.Methods {
				if method.Name.Name == name && method.Body != nil {
					return &CallHierarchyItem{
						Name:   method.Name.Name,
						Kind:   protocol.SymbolKindMethod,
						Detail: classDecl.Name.Name,
						URI:    protocol.DocumentURI(doc.URI),
						Range: protocol.Range{
							Start: protocol.Position{
								Line:      uint32(method.FuncToken.Pos.Line - 1),
								Character: uint32(method.FuncToken.Pos.Column - 1),
							},
							End: protocol.Position{
								Line:      uint32(method.Body.RBrace.Pos.Line - 1),
								Character: uint32(method.Body.RBrace.Pos.Column),
							},
						},
						SelectionRange: protocol.Range{
							Start: protocol.Position{
								Line:      uint32(method.Name.Token.Pos.Line - 1),
								Character: uint32(method.Name.Token.Pos.Column - 1),
							},
							End: protocol.Position{
								Line:      uint32(method.Name.Token.Pos.Line - 1),
								Character: uint32(method.Name.Token.Pos.Column - 1 + len(method.Name.Name)),
							},
						},
					}
				}
			}
		}
	}

	return nil
}
