package lsp

import (
	"encoding/json"
	"fmt"

	"github.com/tangzhangming/nova/internal/ast"
	"go.lsp.dev/protocol"
)

// CodeLens 代码镜头
type CodeLens struct {
	Range   protocol.Range `json:"range"`
	Command *Command       `json:"command,omitempty"`
	Data    interface{}    `json:"data,omitempty"`
}

// Command 命令
type Command struct {
	Title     string        `json:"title"`
	Command   string        `json:"command"`
	Arguments []interface{} `json:"arguments,omitempty"`
}

// handleCodeLens 处理代码镜头请求
func (s *Server) handleCodeLens(id json.RawMessage, params json.RawMessage) {
	// 检查是否启用 Code Lens
	if s.configManager != nil && !s.configManager.GetCodeLens().Enable {
		s.sendResult(id, []CodeLens{})
		return
	}
	
	var p protocol.CodeLensParams
	if err := json.Unmarshal(params, &p); err != nil {
		s.sendError(id, -32700, "Parse error")
		return
	}

	docURI := string(p.TextDocument.URI)
	doc := s.documents.Get(docURI)
	if doc == nil {
		s.sendResult(id, []CodeLens{})
		return
	}

	lenses := s.collectCodeLenses(doc)
	s.sendResult(id, lenses)
}

// collectCodeLenses 收集代码镜头
func (s *Server) collectCodeLenses(doc *Document) []CodeLens {
	var lenses []CodeLens

	astFile := doc.GetAST()
	if astFile == nil {
		return lenses
	}

	// 添加测试相关的 Code Lenses（这是轻量级的，只检查当前文件）
	testLenses := s.GetTestCodeLenses(doc)
	lenses = append(lenses, testLenses...)

	// 注意：引用计数的 Code Lens 已被禁用以提高性能
	// 原因：countReferences 需要遍历所有文档的所有行，
	// 在每次 Code Lens 请求时（编辑器滚动、输入等都会触发）
	// 这会导致严重的性能问题和内存压力。
	//
	// 如需要引用计数功能，建议：
	// 1. 通过配置开关控制
	// 2. 使用缓存机制
	// 3. 使用延迟计算和防抖
	//
	// 用户仍可以通过 "Find All References" 功能查看引用

	// 只为接口添加实现计数（这个相对轻量）
	if s.configManager != nil && s.configManager.GetCodeLens().ShowImplementations {
		for _, decl := range astFile.Declarations {
			if ifaceDecl, ok := decl.(*ast.InterfaceDecl); ok {
				ifaceLens := s.createInterfaceCodeLens(ifaceDecl, doc.URI)
				if ifaceLens != nil {
					lenses = append(lenses, *ifaceLens)
				}
			}
		}
	}

	return lenses
}

// createClassCodeLens 创建类的代码镜头
func (s *Server) createClassCodeLens(classDecl *ast.ClassDecl, docURI string) *CodeLens {
	// 计算引用数量
	refCount := s.countReferences(classDecl.Name.Name, false)

	if refCount == 0 {
		return nil
	}

	return &CodeLens{
		Range: protocol.Range{
			Start: protocol.Position{
				Line:      uint32(classDecl.Name.Token.Pos.Line - 1),
				Character: uint32(classDecl.Name.Token.Pos.Column - 1),
			},
			End: protocol.Position{
				Line:      uint32(classDecl.Name.Token.Pos.Line - 1),
				Character: uint32(classDecl.Name.Token.Pos.Column - 1 + len(classDecl.Name.Name)),
			},
		},
		Command: &Command{
			Title:   fmt.Sprintf("%d references", refCount),
			Command: "sola.findReferences",
			Arguments: []interface{}{
				docURI,
				classDecl.Name.Token.Pos.Line - 1,
				classDecl.Name.Token.Pos.Column - 1,
			},
		},
	}
}

// createInterfaceCodeLens 创建接口的代码镜头
func (s *Server) createInterfaceCodeLens(ifaceDecl *ast.InterfaceDecl, docURI string) *CodeLens {
	// 计算实现数量
	implCount := s.countImplementations(ifaceDecl.Name.Name)

	title := fmt.Sprintf("%d implementations", implCount)
	if implCount == 0 {
		title = "no implementations"
	} else if implCount == 1 {
		title = "1 implementation"
	}

	return &CodeLens{
		Range: protocol.Range{
			Start: protocol.Position{
				Line:      uint32(ifaceDecl.Name.Token.Pos.Line - 1),
				Character: uint32(ifaceDecl.Name.Token.Pos.Column - 1),
			},
			End: protocol.Position{
				Line:      uint32(ifaceDecl.Name.Token.Pos.Line - 1),
				Character: uint32(ifaceDecl.Name.Token.Pos.Column - 1 + len(ifaceDecl.Name.Name)),
			},
		},
		Command: &Command{
			Title:   title,
			Command: "sola.findImplementations",
			Arguments: []interface{}{
				docURI,
				ifaceDecl.Name.Token.Pos.Line - 1,
				ifaceDecl.Name.Token.Pos.Column - 1,
			},
		},
	}
}

// createMethodCodeLenses 创建方法的代码镜头
func (s *Server) createMethodCodeLenses(className string, method *ast.MethodDecl, docURI string) []CodeLens {
	var lenses []CodeLens

	if method.Body == nil {
		return lenses
	}

	// 引用数量
	refCount := s.countMethodReferences(className, method.Name.Name)

	if refCount > 0 {
		lenses = append(lenses, CodeLens{
			Range: protocol.Range{
				Start: protocol.Position{
					Line:      uint32(method.Name.Token.Pos.Line - 1),
					Character: uint32(method.Name.Token.Pos.Column - 1),
				},
				End: protocol.Position{
					Line:      uint32(method.Name.Token.Pos.Line - 1),
					Character: uint32(method.Name.Token.Pos.Column - 1 + len(method.Name.Name)),
				},
			},
			Command: &Command{
				Title:   fmt.Sprintf("%d references", refCount),
				Command: "sola.findReferences",
				Arguments: []interface{}{
					docURI,
					method.Name.Token.Pos.Line - 1,
					method.Name.Token.Pos.Column - 1,
				},
			},
		})
	}

	return lenses
}

// countReferences 计算符号的引用数量
func (s *Server) countReferences(name string, isVariable bool) int {
	count := 0

	for _, doc := range s.documents.GetAll() {
		refs := s.findReferencesInDoc(doc, name, isVariable, false)
		count += len(refs)
	}

	// 减去定义本身
	if count > 0 {
		count--
	}

	return count
}

// countMethodReferences 计算方法的引用数量
func (s *Server) countMethodReferences(className, methodName string) int {
	count := 0

	for _, doc := range s.documents.GetAll() {
		astFile := doc.GetAST()
		if astFile == nil {
			continue
		}

		// 在所有语句中查找方法调用
		for _, stmt := range astFile.Statements {
			count += countMethodCallsInStmt(stmt, methodName)
		}

		// 在所有类方法中查找
		for _, decl := range astFile.Declarations {
			if classDecl, ok := decl.(*ast.ClassDecl); ok {
				for _, method := range classDecl.Methods {
					if method.Body != nil {
						count += countMethodCallsInStmt(method.Body, methodName)
					}
				}
			}
		}
	}

	return count
}

// countMethodCallsInStmt 在语句中计算方法调用数量
func countMethodCallsInStmt(stmt ast.Statement, methodName string) int {
	count := 0

	if stmt == nil {
		return count
	}

	switch s := stmt.(type) {
	case *ast.BlockStmt:
		for _, inner := range s.Statements {
			count += countMethodCallsInStmt(inner, methodName)
		}
	case *ast.ExprStmt:
		count += countMethodCallsInExpr(s.Expr, methodName)
	case *ast.VarDeclStmt:
		if s.Value != nil {
			count += countMethodCallsInExpr(s.Value, methodName)
		}
	case *ast.IfStmt:
		count += countMethodCallsInExpr(s.Condition, methodName)
		count += countMethodCallsInStmt(s.Then, methodName)
		if s.Else != nil {
			count += countMethodCallsInStmt(s.Else, methodName)
		}
	case *ast.ForStmt:
		if s.Init != nil {
			count += countMethodCallsInStmt(s.Init, methodName)
		}
		if s.Condition != nil {
			count += countMethodCallsInExpr(s.Condition, methodName)
		}
		count += countMethodCallsInStmt(s.Body, methodName)
	case *ast.ForeachStmt:
		count += countMethodCallsInExpr(s.Iterable, methodName)
		count += countMethodCallsInStmt(s.Body, methodName)
	case *ast.WhileStmt:
		count += countMethodCallsInExpr(s.Condition, methodName)
		count += countMethodCallsInStmt(s.Body, methodName)
	case *ast.ReturnStmt:
		for _, val := range s.Values {
			count += countMethodCallsInExpr(val, methodName)
		}
	}

	return count
}

// countMethodCallsInExpr 在表达式中计算方法调用数量
func countMethodCallsInExpr(expr ast.Expression, methodName string) int {
	count := 0

	if expr == nil {
		return count
	}

	switch e := expr.(type) {
	case *ast.MethodCall:
		if e.Method.Name == methodName {
			count++
		}
		count += countMethodCallsInExpr(e.Object, methodName)
		for _, arg := range e.Arguments {
			count += countMethodCallsInExpr(arg, methodName)
		}
	case *ast.CallExpr:
		if ident, ok := e.Function.(*ast.Identifier); ok {
			if ident.Name == methodName {
				count++
			}
		}
		for _, arg := range e.Arguments {
			count += countMethodCallsInExpr(arg, methodName)
		}
	case *ast.BinaryExpr:
		count += countMethodCallsInExpr(e.Left, methodName)
		count += countMethodCallsInExpr(e.Right, methodName)
	case *ast.UnaryExpr:
		count += countMethodCallsInExpr(e.Operand, methodName)
	case *ast.AssignExpr:
		count += countMethodCallsInExpr(e.Left, methodName)
		count += countMethodCallsInExpr(e.Right, methodName)
	case *ast.TernaryExpr:
		count += countMethodCallsInExpr(e.Condition, methodName)
		count += countMethodCallsInExpr(e.Then, methodName)
		count += countMethodCallsInExpr(e.Else, methodName)
	case *ast.NewExpr:
		for _, arg := range e.Arguments {
			count += countMethodCallsInExpr(arg, methodName)
		}
	case *ast.ArrayLiteral:
		for _, elem := range e.Elements {
			count += countMethodCallsInExpr(elem, methodName)
		}
	}

	return count
}

// countImplementations 计算接口的实现数量
// 注意：这是一个轻量级实现，只检查已打开的文档
// 不会遍历所有行，只检查类声明
func (s *Server) countImplementations(interfaceName string) int {
	count := 0

	// 限制检查的文档数量，避免性能问题
	docs := s.documents.GetAll()
	maxDocs := 50 // 最多检查50个文档
	if len(docs) > maxDocs {
		docs = docs[:maxDocs]
	}

	for _, doc := range docs {
		astFile := doc.GetAST()
		if astFile == nil {
			continue
		}

		for _, decl := range astFile.Declarations {
			if classDecl, ok := decl.(*ast.ClassDecl); ok {
				// 检查 implements
				for _, impl := range classDecl.Implements {
					// TypeNode 转换为字符串比较
					if impl.String() == interfaceName {
						count++
						break
					}
					// 也检查简单类型
					if simpleType, ok := impl.(*ast.SimpleType); ok {
						if simpleType.Name == interfaceName {
							count++
							break
						}
					}
				}
			}
		}
	}

	return count
}
