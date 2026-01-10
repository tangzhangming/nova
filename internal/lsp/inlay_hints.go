package lsp

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tangzhangming/nova/internal/ast"
	"github.com/tangzhangming/nova/internal/compiler"
	"go.lsp.dev/protocol"
)

// InlayHintKind 内联提示类型
type InlayHintKind uint32

const (
	InlayHintKindType      InlayHintKind = 1
	InlayHintKindParameter InlayHintKind = 2
)

// InlayHintLabelPart 内联提示标签部分
type InlayHintLabelPart struct {
	Value string `json:"value"`
}

// InlayHint 内联提示
type InlayHint struct {
	Position     protocol.Position    `json:"position"`
	Label        []InlayHintLabelPart `json:"label"`
	Kind         InlayHintKind        `json:"kind,omitempty"`
	PaddingLeft  bool                 `json:"paddingLeft,omitempty"`
	PaddingRight bool                 `json:"paddingRight,omitempty"`
}

// InlayHintParams 内联提示请求参数
type InlayHintParams struct {
	TextDocument protocol.TextDocumentIdentifier `json:"textDocument"`
	Range        protocol.Range                  `json:"range"`
}

// InlayHintOptions 内联提示选项
type InlayHintOptions struct {
	ResolveProvider bool `json:"resolveProvider,omitempty"`
}

// handleInlayHints 处理内联提示请求
func (s *Server) handleInlayHints(id json.RawMessage, params json.RawMessage) {
	var p InlayHintParams
	if err := json.Unmarshal(params, &p); err != nil {
		s.sendError(id, -32700, "Parse error")
		return
	}

	docURI := string(p.TextDocument.URI)
	doc := s.documents.Get(docURI)
	if doc == nil {
		s.sendResult(id, []InlayHint{})
		return
	}

	hints := s.collectInlayHints(doc, p.Range)
	s.sendResult(id, hints)
}

// collectInlayHints 收集文档中的内联提示
func (s *Server) collectInlayHints(doc *Document, rang protocol.Range) []InlayHint {
	var hints []InlayHint

	astFile := doc.GetAST()
	if astFile == nil {
		return hints
	}

	symbols := doc.GetSymbols()

	// 收集语句中的提示
	for _, stmt := range astFile.Statements {
		hints = append(hints, s.collectStmtInlayHints(stmt, symbols, rang)...)
	}

	// 收集类方法中的提示
	for _, decl := range astFile.Declarations {
		if classDecl, ok := decl.(*ast.ClassDecl); ok {
			for _, method := range classDecl.Methods {
				if method.Body != nil {
					hints = append(hints, s.collectStmtInlayHints(method.Body, symbols, rang)...)
				}
			}
		}
	}

	return hints
}

// collectStmtInlayHints 收集语句中的内联提示
func (s *Server) collectStmtInlayHints(stmt ast.Statement, symbols *compiler.SymbolTable, rang protocol.Range) []InlayHint {
	var hints []InlayHint

	if stmt == nil {
		return hints
	}

	switch st := stmt.(type) {
	case *ast.VarDeclStmt:
		// 类型提示：在变量声明没有显式类型时显示推断的类型
		if st.Type == nil && st.Value != nil {
			inferredType := inferExprType(st.Value, symbols)
			if inferredType != "" && inferredType != "dynamic" {
				// 在变量名后面显示类型
				pos := protocol.Position{
					Line:      uint32(st.Name.Token.Pos.Line - 1),
					Character: uint32(st.Name.Token.Pos.Column - 1 + len(st.Name.Name)),
				}
				if isInRange(pos, rang) {
					hints = append(hints, InlayHint{
						Position:    pos,
						Label:       []InlayHintLabelPart{{Value: ": " + inferredType}},
						Kind:        InlayHintKindType,
						PaddingLeft: true,
					})
				}
			}
		}
		// 值表达式中的提示
		if st.Value != nil {
			hints = append(hints, s.collectExprInlayHints(st.Value, symbols, rang)...)
		}

	case *ast.ExprStmt:
		hints = append(hints, s.collectExprInlayHints(st.Expr, symbols, rang)...)

	case *ast.BlockStmt:
		for _, inner := range st.Statements {
			hints = append(hints, s.collectStmtInlayHints(inner, symbols, rang)...)
		}

	case *ast.IfStmt:
		hints = append(hints, s.collectExprInlayHints(st.Condition, symbols, rang)...)
		hints = append(hints, s.collectStmtInlayHints(st.Then, symbols, rang)...)
		if st.Else != nil {
			hints = append(hints, s.collectStmtInlayHints(st.Else, symbols, rang)...)
		}

	case *ast.ForStmt:
		if st.Init != nil {
			hints = append(hints, s.collectStmtInlayHints(st.Init, symbols, rang)...)
		}
		if st.Condition != nil {
			hints = append(hints, s.collectExprInlayHints(st.Condition, symbols, rang)...)
		}
		if st.Post != nil {
			hints = append(hints, s.collectExprInlayHints(st.Post, symbols, rang)...)
		}
		hints = append(hints, s.collectStmtInlayHints(st.Body, symbols, rang)...)

	case *ast.ForeachStmt:
		// 显示迭代变量的类型
		iterType := inferExprType(st.Iterable, symbols)
		if st.Key != nil {
			keyType := "int"
			if strings.HasPrefix(iterType, "map[") {
				if idx := strings.Index(iterType, "]"); idx > 4 {
					keyType = iterType[4:idx]
				}
			}
			pos := protocol.Position{
				Line:      uint32(st.Key.Token.Pos.Line - 1),
				Character: uint32(st.Key.Token.Pos.Column - 1 + len(st.Key.Name)),
			}
			if isInRange(pos, rang) {
				hints = append(hints, InlayHint{
					Position:    pos,
					Label:       []InlayHintLabelPart{{Value: ": " + keyType}},
					Kind:        InlayHintKindType,
					PaddingLeft: true,
				})
			}
		}
		// value类型
		valueType := "dynamic"
		if strings.HasSuffix(iterType, "[]") {
			valueType = strings.TrimSuffix(iterType, "[]")
		} else if strings.HasPrefix(iterType, "map[") {
			if idx := strings.Index(iterType, "]"); idx > 0 && idx+1 < len(iterType) {
				valueType = iterType[idx+1:]
			}
		}
		pos := protocol.Position{
			Line:      uint32(st.Value.Token.Pos.Line - 1),
			Character: uint32(st.Value.Token.Pos.Column - 1 + len(st.Value.Name)),
		}
		if isInRange(pos, rang) && valueType != "dynamic" {
			hints = append(hints, InlayHint{
				Position:    pos,
				Label:       []InlayHintLabelPart{{Value: ": " + valueType}},
				Kind:        InlayHintKindType,
				PaddingLeft: true,
			})
		}
		hints = append(hints, s.collectStmtInlayHints(st.Body, symbols, rang)...)

	case *ast.WhileStmt:
		hints = append(hints, s.collectExprInlayHints(st.Condition, symbols, rang)...)
		hints = append(hints, s.collectStmtInlayHints(st.Body, symbols, rang)...)

	case *ast.ReturnStmt:
		for _, val := range st.Values {
			hints = append(hints, s.collectExprInlayHints(val, symbols, rang)...)
		}

	case *ast.TryStmt:
		hints = append(hints, s.collectStmtInlayHints(st.Try, symbols, rang)...)
		for _, catch := range st.Catches {
			hints = append(hints, s.collectStmtInlayHints(catch.Body, symbols, rang)...)
		}
	}

	return hints
}

// collectExprInlayHints 收集表达式中的内联提示
func (s *Server) collectExprInlayHints(expr ast.Expression, symbols *compiler.SymbolTable, rang protocol.Range) []InlayHint {
	var hints []InlayHint

	if expr == nil {
		return hints
	}

	switch e := expr.(type) {
	case *ast.CallExpr:
		// 参数名提示
		hints = append(hints, s.getCallArgHints(e, symbols, rang)...)
		// 递归处理参数
		for _, arg := range e.Arguments {
			hints = append(hints, s.collectExprInlayHints(arg, symbols, rang)...)
		}

	case *ast.MethodCall:
		// 方法调用的参数名提示
		hints = append(hints, s.getMethodCallArgHints(e, symbols, rang)...)
		hints = append(hints, s.collectExprInlayHints(e.Object, symbols, rang)...)
		for _, arg := range e.Arguments {
			hints = append(hints, s.collectExprInlayHints(arg, symbols, rang)...)
		}

	case *ast.NewExpr:
		// 构造函数参数提示
		hints = append(hints, s.getNewExprArgHints(e, symbols, rang)...)
		for _, arg := range e.Arguments {
			hints = append(hints, s.collectExprInlayHints(arg, symbols, rang)...)
		}

	case *ast.BinaryExpr:
		hints = append(hints, s.collectExprInlayHints(e.Left, symbols, rang)...)
		hints = append(hints, s.collectExprInlayHints(e.Right, symbols, rang)...)

	case *ast.UnaryExpr:
		hints = append(hints, s.collectExprInlayHints(e.Operand, symbols, rang)...)

	case *ast.AssignExpr:
		hints = append(hints, s.collectExprInlayHints(e.Left, symbols, rang)...)
		hints = append(hints, s.collectExprInlayHints(e.Right, symbols, rang)...)

	case *ast.TernaryExpr:
		hints = append(hints, s.collectExprInlayHints(e.Condition, symbols, rang)...)
		hints = append(hints, s.collectExprInlayHints(e.Then, symbols, rang)...)
		hints = append(hints, s.collectExprInlayHints(e.Else, symbols, rang)...)

	case *ast.ArrayLiteral:
		for _, elem := range e.Elements {
			hints = append(hints, s.collectExprInlayHints(elem, symbols, rang)...)
		}

	case *ast.MapLiteral:
		for _, pair := range e.Pairs {
			hints = append(hints, s.collectExprInlayHints(pair.Key, symbols, rang)...)
			hints = append(hints, s.collectExprInlayHints(pair.Value, symbols, rang)...)
		}
	}

	return hints
}

// getCallArgHints 获取函数调用的参数名提示
func (s *Server) getCallArgHints(call *ast.CallExpr, symbols *compiler.SymbolTable, rang protocol.Range) []InlayHint {
	var hints []InlayHint

	if symbols == nil || len(call.Arguments) == 0 {
		return hints
	}

	// 获取函数名
	funcName := ""
	if ident, ok := call.Function.(*ast.Identifier); ok {
		funcName = ident.Name
	}

	if funcName == "" {
		return hints
	}

	// 查找函数签名
	fn := symbols.GetFunction(funcName)
	if fn == nil || len(fn.ParamNames) == 0 {
		return hints
	}

	// 为每个参数添加名称提示
	for i, arg := range call.Arguments {
		if i >= len(fn.ParamNames) {
			break
		}

		paramName := fn.ParamNames[i]
		if paramName == "" {
			continue
		}

		// 跳过已经有明确变量名的参数（如果参数是变量且名称相同）
		if v, ok := arg.(*ast.Variable); ok && v.Name == paramName {
			continue
		}

		pos := protocol.Position{
			Line:      uint32(arg.Pos().Line - 1),
			Character: uint32(arg.Pos().Column - 1),
		}

		if isInRange(pos, rang) {
			hints = append(hints, InlayHint{
				Position:     pos,
				Label:        []InlayHintLabelPart{{Value: paramName + ":"}},
				Kind:         InlayHintKindParameter,
				PaddingRight: true,
			})
		}
	}

	return hints
}

// getMethodCallArgHints 获取方法调用的参数名提示
func (s *Server) getMethodCallArgHints(call *ast.MethodCall, symbols *compiler.SymbolTable, rang protocol.Range) []InlayHint {
	var hints []InlayHint

	if symbols == nil || len(call.Arguments) == 0 {
		return hints
	}

	// 获取对象类型
	objectType := inferExprType(call.Object, symbols)
	if objectType == "" || objectType == "dynamic" {
		return hints
	}

	// 提取基础类型
	baseType := objectType
	if strings.Contains(baseType, "|") {
		baseType = strings.Split(baseType, "|")[0]
	}

	// 查找方法签名
	method := symbols.GetMethod(baseType, call.Method.Name, len(call.Arguments))
	if method == nil || len(method.ParamNames) == 0 {
		return hints
	}

	// 为每个参数添加名称提示
	for i, arg := range call.Arguments {
		if i >= len(method.ParamNames) {
			break
		}

		paramName := method.ParamNames[i]
		if paramName == "" {
			continue
		}

		// 跳过已经有明确变量名的参数
		if v, ok := arg.(*ast.Variable); ok && v.Name == paramName {
			continue
		}

		pos := protocol.Position{
			Line:      uint32(arg.Pos().Line - 1),
			Character: uint32(arg.Pos().Column - 1),
		}

		if isInRange(pos, rang) {
			hints = append(hints, InlayHint{
				Position:     pos,
				Label:        []InlayHintLabelPart{{Value: paramName + ":"}},
				Kind:         InlayHintKindParameter,
				PaddingRight: true,
			})
		}
	}

	return hints
}

// getNewExprArgHints 获取new表达式的参数名提示
func (s *Server) getNewExprArgHints(expr *ast.NewExpr, symbols *compiler.SymbolTable, rang protocol.Range) []InlayHint {
	var hints []InlayHint

	if symbols == nil || len(expr.Arguments) == 0 {
		return hints
	}

	// 查找构造函数签名
	className := expr.ClassName.Name
	method := symbols.GetMethod(className, "__construct", len(expr.Arguments))
	if method == nil || len(method.ParamNames) == 0 {
		return hints
	}

	// 为每个参数添加名称提示
	for i, arg := range expr.Arguments {
		if i >= len(method.ParamNames) {
			break
		}

		paramName := method.ParamNames[i]
		if paramName == "" {
			continue
		}

		// 跳过已经有明确变量名的参数
		if v, ok := arg.(*ast.Variable); ok && v.Name == paramName {
			continue
		}

		pos := protocol.Position{
			Line:      uint32(arg.Pos().Line - 1),
			Character: uint32(arg.Pos().Column - 1),
		}

		if isInRange(pos, rang) {
			hints = append(hints, InlayHint{
				Position:     pos,
				Label:        []InlayHintLabelPart{{Value: paramName + ":"}},
				Kind:         InlayHintKindParameter,
				PaddingRight: true,
			})
		}
	}

	return hints
}

// inferExprType 推断表达式类型
func inferExprType(expr ast.Expression, symbols *compiler.SymbolTable) string {
	if expr == nil {
		return ""
	}

	switch e := expr.(type) {
	case *ast.IntegerLiteral:
		return "int"
	case *ast.FloatLiteral:
		return "float"
	case *ast.StringLiteral, *ast.InterpStringLiteral:
		return "string"
	case *ast.BoolLiteral:
		return "bool"
	case *ast.NullLiteral:
		return "null"
	case *ast.ArrayLiteral:
		if e.ElementType != nil {
			return typeNodeToString(e.ElementType) + "[]"
		}
		if len(e.Elements) > 0 {
			elemType := inferExprType(e.Elements[0], symbols)
			if elemType != "" {
				return elemType + "[]"
			}
		}
		return "array"
	case *ast.MapLiteral:
		if e.KeyType != nil && e.ValueType != nil {
			return fmt.Sprintf("map[%s]%s", typeNodeToString(e.KeyType), typeNodeToString(e.ValueType))
		}
		return "map"
	case *ast.NewExpr:
		return e.ClassName.Name
	case *ast.Variable:
		// 尝试从符号表获取变量类型
		if symbols != nil {
			// 这里简化处理，实际应该查找作用域
			return "dynamic"
		}
		return "dynamic"
	case *ast.CallExpr:
		if ident, ok := e.Function.(*ast.Identifier); ok && symbols != nil {
			if fn := symbols.GetFunction(ident.Name); fn != nil {
				return fn.ReturnType
			}
		}
		return "dynamic"
	case *ast.MethodCall:
		objType := inferExprType(e.Object, symbols)
		if objType != "" && symbols != nil {
			if method := symbols.GetMethod(objType, e.Method.Name, len(e.Arguments)); method != nil {
				return method.ReturnType
			}
		}
		return "dynamic"
	case *ast.BinaryExpr:
		leftType := inferExprType(e.Left, symbols)
		rightType := inferExprType(e.Right, symbols)
		// 简化的类型推断
		if leftType == "string" || rightType == "string" {
			return "string"
		}
		if leftType == "float" || rightType == "float" {
			return "float"
		}
		if leftType == "int" && rightType == "int" {
			return "int"
		}
		return "dynamic"
	case *ast.TernaryExpr:
		return inferExprType(e.Then, symbols)
	case *ast.ThisExpr:
		return "this"
	}

	return "dynamic"
}

// isInRange 检查位置是否在范围内
func isInRange(pos protocol.Position, rang protocol.Range) bool {
	if pos.Line < rang.Start.Line || pos.Line > rang.End.Line {
		return false
	}
	if pos.Line == rang.Start.Line && pos.Character < rang.Start.Character {
		return false
	}
	if pos.Line == rang.End.Line && pos.Character > rang.End.Character {
		return false
	}
	return true
}
