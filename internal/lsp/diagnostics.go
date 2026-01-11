package lsp

import (
	"strings"

	"github.com/tangzhangming/nova/internal/ast"
	"github.com/tangzhangming/nova/internal/compiler"
	"github.com/tangzhangming/nova/internal/token"
	"go.lsp.dev/protocol"
)

// getDiagnostics 获取文档的诊断信息
func (s *Server) getDiagnostics(doc *Document) []protocol.Diagnostic {
	var diagnostics []protocol.Diagnostic

	// 确保文档已解析
	astFile := doc.GetAST()

	// 添加解析错误
	for _, err := range doc.ParseErrs {
		diag := protocol.Diagnostic{
			Range: protocol.Range{
				Start: protocol.Position{
					Line:      uint32(err.Pos.Line - 1), // LSP 行号从 0 开始
					Character: uint32(err.Pos.Column - 1),
				},
				End: protocol.Position{
					Line:      uint32(err.Pos.Line - 1),
					Character: uint32(err.Pos.Column + 10), // 估计错误范围
				},
			},
			Severity: protocol.DiagnosticSeverityError,
			Source:   "sola",
			Message:  err.Message,
		}
		diagnostics = append(diagnostics, diag)
	}

	// 类型检查 - 只有在没有解析错误时才进行
	if len(doc.ParseErrs) == 0 && astFile != nil && doc.Symbols != nil {
		typeErrors, typeWarnings := s.runTypeCheck(astFile, doc.Symbols)

		// 添加类型错误
		for _, err := range typeErrors {
			diag := typeErrorToDiagnostic(err)
			diagnostics = append(diagnostics, diag)
		}

		// 添加类型警告
		for _, warn := range typeWarnings {
			diag := typeWarningToDiagnostic(warn)
			diagnostics = append(diagnostics, diag)
		}
	}

	// 检查未使用的变量
	if astFile != nil && len(doc.ParseErrs) == 0 {
		unusedWarnings := s.checkUnusedVariables(astFile)
		diagnostics = append(diagnostics, unusedWarnings...)
	}

	// 检查未使用的导入
	if astFile != nil && len(doc.ParseErrs) == 0 {
		unusedImports := s.checkUnusedImports(astFile, doc)
		diagnostics = append(diagnostics, unusedImports...)
	}

	return diagnostics
}

// runTypeCheck 运行类型检查
func (s *Server) runTypeCheck(file *ast.File, symbols *compiler.SymbolTable) ([]compiler.TypeError, []compiler.TypeWarning) {
	if file == nil || symbols == nil {
		return nil, nil
	}

	typeChecker := compiler.NewTypeChecker(symbols)
	errors := typeChecker.Check(file)
	warnings := typeChecker.GetWarnings()

	return errors, warnings
}

// typeErrorToDiagnostic 将类型错误转换为诊断
func typeErrorToDiagnostic(err compiler.TypeError) protocol.Diagnostic {
	return protocol.Diagnostic{
		Range: protocol.Range{
			Start: protocol.Position{
				Line:      uint32(err.Pos.Line - 1),
				Character: uint32(err.Pos.Column - 1),
			},
			End: protocol.Position{
				Line:      uint32(err.Pos.Line - 1),
				Character: uint32(err.Pos.Column - 1 + 10),
			},
		},
		Severity: protocol.DiagnosticSeverityError,
		Code:     err.Code,
		Source:   "sola",
		Message:  err.Message,
	}
}

// typeWarningToDiagnostic 将类型警告转换为诊断
func typeWarningToDiagnostic(warn compiler.TypeWarning) protocol.Diagnostic {
	severity := protocol.DiagnosticSeverityWarning
	// 根据代码前缀调整严重性
	if strings.HasPrefix(warn.Code, "H") {
		severity = protocol.DiagnosticSeverityHint
	} else if strings.HasPrefix(warn.Code, "I") {
		severity = protocol.DiagnosticSeverityInformation
	}

	return protocol.Diagnostic{
		Range: protocol.Range{
			Start: protocol.Position{
				Line:      uint32(warn.Pos.Line - 1),
				Character: uint32(warn.Pos.Column - 1),
			},
			End: protocol.Position{
				Line:      uint32(warn.Pos.Line - 1),
				Character: uint32(warn.Pos.Column - 1 + 10),
			},
		},
		Severity: severity,
		Code:     warn.Code,
		Source:   "sola",
		Message:  warn.Message,
	}
}

// varInfo 变量信息
type varInfo struct {
	Name string
	Pos  token.Position
}

// checkUnusedVariables 检查未使用的变量
func (s *Server) checkUnusedVariables(file *ast.File) []protocol.Diagnostic {
	var diagnostics []protocol.Diagnostic

	// 收集所有声明的变量
	declared := make(map[string]*varInfo)
	// 收集所有使用的变量
	used := make(map[string]bool)

	// 遍历文件收集变量声明和使用
	for _, stmt := range file.Statements {
		collectVarDeclarationsAndUsages(stmt, declared, used)
	}

	// 遍历类方法
	for _, decl := range file.Declarations {
		if classDecl, ok := decl.(*ast.ClassDecl); ok {
			for _, method := range classDecl.Methods {
				// 方法内部的变量
				methodDeclared := make(map[string]*varInfo)
				methodUsed := make(map[string]bool)

				// 参数是声明的变量
				for _, param := range method.Parameters {
					methodDeclared[param.Name.Name] = &varInfo{
						Name: param.Name.Name,
						Pos:  param.Name.Token.Pos,
					}
				}

				if method.Body != nil {
					collectVarDeclarationsAndUsages(method.Body, methodDeclared, methodUsed)
				}

				// 检查方法内未使用的变量（排除参数）
				for name, info := range methodDeclared {
					if !methodUsed[name] {
						// 忽略以 _ 开头的变量（约定为故意不使用）
						if strings.HasPrefix(name, "_") {
							continue
						}
						diagnostics = append(diagnostics, protocol.Diagnostic{
							Range: protocol.Range{
								Start: protocol.Position{
									Line:      uint32(info.Pos.Line - 1),
									Character: uint32(info.Pos.Column - 1),
								},
								End: protocol.Position{
									Line:      uint32(info.Pos.Line - 1),
									Character: uint32(info.Pos.Column - 1 + len(name)),
								},
							},
							Severity: protocol.DiagnosticSeverityHint,
							Code:     "W002",
							Source:   "sola",
							Message:  "variable '" + name + "' is declared but never used",
							Tags:     []protocol.DiagnosticTag{protocol.DiagnosticTagUnnecessary},
						})
					}
				}
			}
		}
	}

	return diagnostics
}

// collectVarDeclarationsAndUsages 收集变量声明和使用
func collectVarDeclarationsAndUsages(stmt ast.Statement, declared map[string]*varInfo, used map[string]bool) {
	if stmt == nil {
		return
	}

	switch s := stmt.(type) {
	case *ast.VarDeclStmt:
		declared[s.Name.Name] = &varInfo{Name: s.Name.Name, Pos: s.Name.Token.Pos}
		if s.Value != nil {
			collectExprUsages(s.Value, used)
		}
	case *ast.MultiVarDeclStmt:
		for _, name := range s.Names {
			declared[name.Name] = &varInfo{Name: name.Name, Pos: name.Token.Pos}
		}
		collectExprUsages(s.Value, used)
	case *ast.ExprStmt:
		collectExprUsages(s.Expr, used)
	case *ast.BlockStmt:
		for _, inner := range s.Statements {
			collectVarDeclarationsAndUsages(inner, declared, used)
		}
	case *ast.IfStmt:
		collectExprUsages(s.Condition, used)
		collectVarDeclarationsAndUsages(s.Then, declared, used)
		if s.Else != nil {
			collectVarDeclarationsAndUsages(s.Else, declared, used)
		}
	case *ast.ForStmt:
		if s.Init != nil {
			collectVarDeclarationsAndUsages(s.Init, declared, used)
		}
		if s.Condition != nil {
			collectExprUsages(s.Condition, used)
		}
		if s.Post != nil {
			collectExprUsages(s.Post, used)
		}
		collectVarDeclarationsAndUsages(s.Body, declared, used)
	case *ast.ForeachStmt:
		if s.Key != nil {
			declared[s.Key.Name] = &varInfo{Name: s.Key.Name, Pos: s.Key.Token.Pos}
		}
		declared[s.Value.Name] = &varInfo{Name: s.Value.Name, Pos: s.Value.Token.Pos}
		collectExprUsages(s.Iterable, used)
		collectVarDeclarationsAndUsages(s.Body, declared, used)
	case *ast.WhileStmt:
		collectExprUsages(s.Condition, used)
		collectVarDeclarationsAndUsages(s.Body, declared, used)
	case *ast.ReturnStmt:
		for _, val := range s.Values {
			collectExprUsages(val, used)
		}
	case *ast.ThrowStmt:
		collectExprUsages(s.Exception, used)
	case *ast.TryStmt:
		collectVarDeclarationsAndUsages(s.Try, declared, used)
		for _, catch := range s.Catches {
			declared[catch.Variable.Name] = &varInfo{Name: catch.Variable.Name, Pos: catch.Variable.Token.Pos}
			collectVarDeclarationsAndUsages(catch.Body, declared, used)
		}
		if s.Finally != nil {
			collectVarDeclarationsAndUsages(s.Finally.Body, declared, used)
		}
	}
}

// collectExprUsages 收集表达式中使用的变量
func collectExprUsages(expr ast.Expression, used map[string]bool) {
	if expr == nil {
		return
	}

	switch e := expr.(type) {
	case *ast.Variable:
		used[e.Name] = true
	case *ast.BinaryExpr:
		collectExprUsages(e.Left, used)
		collectExprUsages(e.Right, used)
	case *ast.UnaryExpr:
		collectExprUsages(e.Operand, used)
	case *ast.AssignExpr:
		collectExprUsages(e.Left, used)
		collectExprUsages(e.Right, used)
	case *ast.CallExpr:
		collectExprUsages(e.Function, used)
		for _, arg := range e.Arguments {
			collectExprUsages(arg, used)
		}
	case *ast.MethodCall:
		collectExprUsages(e.Object, used)
		for _, arg := range e.Arguments {
			collectExprUsages(arg, used)
		}
	case *ast.PropertyAccess:
		collectExprUsages(e.Object, used)
	case *ast.IndexExpr:
		collectExprUsages(e.Object, used)
		collectExprUsages(e.Index, used)
	case *ast.ArrayLiteral:
		for _, elem := range e.Elements {
			collectExprUsages(elem, used)
		}
	case *ast.MapLiteral:
		for _, pair := range e.Pairs {
			collectExprUsages(pair.Key, used)
			collectExprUsages(pair.Value, used)
		}
	case *ast.NewExpr:
		for _, arg := range e.Arguments {
			collectExprUsages(arg, used)
		}
	case *ast.TernaryExpr:
		collectExprUsages(e.Condition, used)
		collectExprUsages(e.Then, used)
		collectExprUsages(e.Else, used)
	case *ast.IsExpr:
		collectExprUsages(e.Expr, used)
	case *ast.TypeCastExpr:
		collectExprUsages(e.Expr, used)
	case *ast.NullCoalesceExpr:
		collectExprUsages(e.Left, used)
		collectExprUsages(e.Right, used)
	case *ast.SafePropertyAccess:
		collectExprUsages(e.Object, used)
	case *ast.SafeMethodCall:
		collectExprUsages(e.Object, used)
		for _, arg := range e.Arguments {
			collectExprUsages(arg, used)
		}
	}
}

// checkUnusedImports 检查未使用的导入
func (s *Server) checkUnusedImports(file *ast.File, doc *Document) []protocol.Diagnostic {
	var diagnostics []protocol.Diagnostic

	if file == nil || len(file.Uses) == 0 {
		return diagnostics
	}

	// 收集所有导入的符号
	importedSymbols := make(map[string]*ast.UseDecl)
	for _, use := range file.Uses {
		if use == nil {
			continue
		}
		// 获取导入的名称（别名或路径最后一部分）
		name := ""
		if use.Alias != nil {
			name = use.Alias.Name
		} else {
			parts := strings.Split(use.Path, ".")
			if len(parts) > 0 {
				name = parts[len(parts)-1]
			}
		}
		if name != "" {
			importedSymbols[name] = use
		}
	}

	// 检查代码中是否使用了这些符号
	usedSymbols := make(map[string]bool)
	for _, stmt := range file.Statements {
		collectUsedTypeNames(stmt, usedSymbols)
	}
	for _, decl := range file.Declarations {
		collectUsedTypeNamesInDecl(decl, usedSymbols)
	}

	// 检查未使用的导入
	for name, use := range importedSymbols {
		if !usedSymbols[name] {
			diagnostics = append(diagnostics, protocol.Diagnostic{
				Range: protocol.Range{
					Start: protocol.Position{
						Line:      uint32(use.UseToken.Pos.Line - 1),
						Character: uint32(use.UseToken.Pos.Column - 1),
					},
					End: protocol.Position{
						Line:      uint32(use.UseToken.Pos.Line - 1),
						Character: uint32(use.UseToken.Pos.Column - 1 + len("use "+use.Path)),
					},
				},
				Severity: protocol.DiagnosticSeverityHint,
				Code:     "W003",
				Source:   "sola",
				Message:  "import '" + use.Path + "' is not used",
				Tags:     []protocol.DiagnosticTag{protocol.DiagnosticTagUnnecessary},
			})
		}
	}

	return diagnostics
}

// collectUsedTypeNames 收集语句中使用的类型名称
func collectUsedTypeNames(stmt ast.Statement, used map[string]bool) {
	if stmt == nil {
		return
	}

	switch s := stmt.(type) {
	case *ast.VarDeclStmt:
		if s.Type != nil {
			collectTypeNames(s.Type, used)
		}
		if s.Value != nil {
			collectUsedTypeNamesInExpr(s.Value, used)
		}
	case *ast.ExprStmt:
		collectUsedTypeNamesInExpr(s.Expr, used)
	case *ast.BlockStmt:
		for _, inner := range s.Statements {
			collectUsedTypeNames(inner, used)
		}
	case *ast.IfStmt:
		collectUsedTypeNamesInExpr(s.Condition, used)
		collectUsedTypeNames(s.Then, used)
		if s.Else != nil {
			collectUsedTypeNames(s.Else, used)
		}
	case *ast.ForStmt:
		if s.Init != nil {
			collectUsedTypeNames(s.Init, used)
		}
		collectUsedTypeNames(s.Body, used)
	case *ast.ForeachStmt:
		collectUsedTypeNamesInExpr(s.Iterable, used)
		collectUsedTypeNames(s.Body, used)
	case *ast.ReturnStmt:
		for _, val := range s.Values {
			collectUsedTypeNamesInExpr(val, used)
		}
	case *ast.TryStmt:
		collectUsedTypeNames(s.Try, used)
		for _, catch := range s.Catches {
			if catch.Type != nil {
				collectTypeNames(catch.Type, used)
			}
			collectUsedTypeNames(catch.Body, used)
		}
	}
}

// collectUsedTypeNamesInExpr 收集表达式中使用的类型名称
func collectUsedTypeNamesInExpr(expr ast.Expression, used map[string]bool) {
	if expr == nil {
		return
	}

	switch e := expr.(type) {
	case *ast.NewExpr:
		used[e.ClassName.Name] = true
		for _, arg := range e.Arguments {
			collectUsedTypeNamesInExpr(arg, used)
		}
	case *ast.IsExpr:
		collectTypeNames(e.TypeName, used)
		collectUsedTypeNamesInExpr(e.Expr, used)
	case *ast.TypeCastExpr:
		collectTypeNames(e.TargetType, used)
		collectUsedTypeNamesInExpr(e.Expr, used)
	case *ast.StaticAccess:
		if ident, ok := e.Class.(*ast.Identifier); ok {
			used[ident.Name] = true
		}
	case *ast.BinaryExpr:
		collectUsedTypeNamesInExpr(e.Left, used)
		collectUsedTypeNamesInExpr(e.Right, used)
	case *ast.UnaryExpr:
		collectUsedTypeNamesInExpr(e.Operand, used)
	case *ast.CallExpr:
		collectUsedTypeNamesInExpr(e.Function, used)
		for _, arg := range e.Arguments {
			collectUsedTypeNamesInExpr(arg, used)
		}
	case *ast.MethodCall:
		collectUsedTypeNamesInExpr(e.Object, used)
		for _, arg := range e.Arguments {
			collectUsedTypeNamesInExpr(arg, used)
		}
	case *ast.PropertyAccess:
		collectUsedTypeNamesInExpr(e.Object, used)
	case *ast.IndexExpr:
		collectUsedTypeNamesInExpr(e.Object, used)
		collectUsedTypeNamesInExpr(e.Index, used)
	case *ast.TernaryExpr:
		collectUsedTypeNamesInExpr(e.Condition, used)
		collectUsedTypeNamesInExpr(e.Then, used)
		collectUsedTypeNamesInExpr(e.Else, used)
	case *ast.ArrayLiteral:
		if e.ElementType != nil {
			collectTypeNames(e.ElementType, used)
		}
		for _, elem := range e.Elements {
			collectUsedTypeNamesInExpr(elem, used)
		}
	case *ast.MapLiteral:
		if e.KeyType != nil {
			collectTypeNames(e.KeyType, used)
		}
		if e.ValueType != nil {
			collectTypeNames(e.ValueType, used)
		}
		for _, pair := range e.Pairs {
			collectUsedTypeNamesInExpr(pair.Key, used)
			collectUsedTypeNamesInExpr(pair.Value, used)
		}
	}
}

// collectUsedTypeNamesInDecl 收集声明中使用的类型名称
func collectUsedTypeNamesInDecl(decl ast.Declaration, used map[string]bool) {
	switch d := decl.(type) {
	case *ast.ClassDecl:
		if d.Extends != nil {
			used[d.Extends.Name] = true
		}
		for _, impl := range d.Implements {
			collectTypeNames(impl, used)
		}
		for _, prop := range d.Properties {
			if prop.Type != nil {
				collectTypeNames(prop.Type, used)
			}
			if prop.Value != nil {
				collectUsedTypeNamesInExpr(prop.Value, used)
			}
		}
		for _, method := range d.Methods {
			for _, param := range method.Parameters {
				if param.Type != nil {
					collectTypeNames(param.Type, used)
				}
			}
			if method.ReturnType != nil {
				collectTypeNames(method.ReturnType, used)
			}
			if method.Body != nil {
				collectUsedTypeNames(method.Body, used)
			}
		}
	case *ast.InterfaceDecl:
		for _, method := range d.Methods {
			for _, param := range method.Parameters {
				if param.Type != nil {
					collectTypeNames(param.Type, used)
				}
			}
			if method.ReturnType != nil {
				collectTypeNames(method.ReturnType, used)
			}
		}
	case *ast.TypeAliasDecl:
		collectTypeNames(d.AliasType, used)
	case *ast.NewTypeDecl:
		collectTypeNames(d.BaseType, used)
	}
}

// collectTypeNames 收集类型节点中的类型名称
func collectTypeNames(t ast.TypeNode, used map[string]bool) {
	if t == nil {
		return
	}

	switch typ := t.(type) {
	case *ast.SimpleType:
		// 排除基本类型
		if !isBuiltinType(typ.Name) {
			used[typ.Name] = true
		}
	case *ast.ClassType:
		used[typ.Name.Literal] = true
	case *ast.ArrayType:
		collectTypeNames(typ.ElementType, used)
	case *ast.MapType:
		collectTypeNames(typ.KeyType, used)
		collectTypeNames(typ.ValueType, used)
	case *ast.NullableType:
		collectTypeNames(typ.Inner, used)
	case *ast.UnionType:
		for _, inner := range typ.Types {
			collectTypeNames(inner, used)
		}
	case *ast.GenericType:
		collectTypeNames(typ.BaseType, used)
		for _, arg := range typ.TypeArgs {
			collectTypeNames(arg, used)
		}
	}
}

// isBuiltinType 检查是否是内置类型
func isBuiltinType(name string) bool {
	builtins := map[string]bool{
		"int": true, "i8": true, "i16": true, "i32": true, "i64": true,
		"uint": true, "u8": true, "u16": true, "u32": true, "u64": true,
		"float": true, "f32": true, "f64": true,
		"bool": true, "string": true, "byte": true,
		"void": true, "null": true, "dynamic": true, "unknown": true,
		"array": true, "map": true,
	}
	return builtins[name]
}

// DiagnosticSeverity 诊断严重程度
type DiagnosticSeverity int

const (
	SeverityError       DiagnosticSeverity = 1
	SeverityWarning     DiagnosticSeverity = 2
	SeverityInformation DiagnosticSeverity = 3
	SeverityHint        DiagnosticSeverity = 4
)

// ErrorCodeToDiagnostic 将错误码转换为诊断信息
func ErrorCodeToDiagnostic(code, message string, line, col int) protocol.Diagnostic {
	severity := protocol.DiagnosticSeverityError

	// 根据错误码前缀判断严重程度
	if len(code) > 0 {
		switch code[0] {
		case 'W': // Warning
			severity = protocol.DiagnosticSeverityWarning
		case 'I': // Info
			severity = protocol.DiagnosticSeverityInformation
		case 'H': // Hint
			severity = protocol.DiagnosticSeverityHint
		}
	}

	return protocol.Diagnostic{
		Range: protocol.Range{
			Start: protocol.Position{
				Line:      uint32(line - 1),
				Character: uint32(col - 1),
			},
			End: protocol.Position{
				Line:      uint32(line - 1),
				Character: uint32(col + 10),
			},
		},
		Severity: severity,
		Code:     code,
		Source:   "sola",
		Message:  message,
	}
}
