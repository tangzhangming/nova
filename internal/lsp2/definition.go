package lsp2

import (
	"github.com/tangzhangming/nova/internal/ast"
	"go.lsp.dev/protocol"
)

// DefinitionProvider 提供定义跳转功能
type DefinitionProvider struct {
	docManager     *DocumentManager
	importResolver *ImportResolver
	logger         *Logger
}

// NewDefinitionProvider 创建定义跳转提供者
func NewDefinitionProvider(docManager *DocumentManager, importResolver *ImportResolver, logger *Logger) *DefinitionProvider {
	return &DefinitionProvider{
		docManager:     docManager,
		importResolver: importResolver,
		logger:         logger,
	}
}

// FindDefinition 查找定义
func (dp *DefinitionProvider) FindDefinition(uri string, line, character int) *protocol.Location {
	doc := dp.docManager.Get(uri)
	if doc == nil {
		dp.logger.Debug("Document not found: %s", uri)
		return nil
	}

	// 获取行内容
	if line < 0 || line >= len(doc.Lines) {
		return nil
	}
	lineText := doc.Lines[line]

	// 获取光标位置的单词
	word, _, _ := GetWordAt(lineText, character)
	if word == "" {
		return nil
	}

	dp.logger.Debug("Finding definition for '%s' at %s:%d:%d", word, uri, line, character)

	// 检查是否是静态方法调用 (Class::method)
	if className, isStatic := CheckStaticCall(lineText, character); isStatic {
		dp.logger.Debug("Detected static call: %s::%s", className, word)
		return dp.findStaticMethod(doc, className, word)
	}

	// 检查是否是实例方法调用 ($obj->method)
	if varName, isInstance := CheckInstanceCall(lineText, character); isInstance {
		dp.logger.Debug("Detected instance call: $%s->%s", varName, word)
		return dp.findInstanceMethod(doc, varName, word, line)
	}

	// 默认：查找类/接口/枚举定义
	return dp.findClassDefinition(doc, word)
}

// findClassDefinition 查找类/接口/枚举定义
func (dp *DefinitionProvider) findClassDefinition(doc *Document, className string) *protocol.Location {
	// 1. 在当前文档中查找
	if loc := dp.findSymbolInAST(doc.GetAST(), className, doc.URI); loc != nil {
		dp.logger.Debug("Found class '%s' in current document", className)
		return loc
	}

	// 2. 在导入的文件中查找
	imports := dp.importResolver.ResolveImports(doc)
	for importPath, imported := range imports {
		if imported.AST == nil {
			continue
		}

		if loc := dp.findSymbolInAST(imported.AST, className, imported.URI); loc != nil {
			dp.logger.Debug("Found class '%s' in import %s", className, importPath)
			return loc
		}
	}

	// 3. 尝试在标准库中查找
	if stdLibPath := ResolveStdLibImport("sola." + className); stdLibPath != "" {
		dp.logger.Debug("Trying to find '%s' in stdlib: %s", className, stdLibPath)
		imported := dp.importResolver.loadFile(stdLibPath)
		if imported != nil && imported.AST != nil {
			if loc := dp.findSymbolInAST(imported.AST, className, imported.URI); loc != nil {
				dp.logger.Debug("Found class '%s' in stdlib", className)
				return loc
			}
		}
	}

	dp.logger.Debug("Class '%s' not found", className)
	return nil
}

// findStaticMethod 查找静态方法
func (dp *DefinitionProvider) findStaticMethod(doc *Document, className, methodName string) *protocol.Location {
	// 首先找到类定义
	classLoc := dp.findClassDefinition(doc, className)
	if classLoc == nil {
		return nil
	}

	// 获取类定义所在的文档
	var classAST *ast.File

	if classLoc.URI == protocol.DocumentURI(doc.URI) {
		// 类在当前文档
		classAST = doc.GetAST()
	} else {
		// 类在其他文件，需要加载
		classPath := uriToPath(string(classLoc.URI))
		imported := dp.importResolver.loadFile(classPath)
		if imported == nil || imported.AST == nil {
			return nil
		}
		classAST = imported.AST
	}

	// 在类中查找静态方法
	if classAST != nil {
		for _, decl := range classAST.Declarations {
			if classDecl, ok := decl.(*ast.ClassDecl); ok {
				if classDecl.Name.Name == className {
					// 查找静态方法
					for _, method := range classDecl.Methods {
						if method.Static && method.Name.Name == methodName {
							dp.logger.Debug("Found static method %s::%s", className, methodName)
							return &protocol.Location{
								URI: classLoc.URI,
								Range: protocol.Range{
									Start: protocol.Position{
										Line:      uint32(method.Name.Token.Pos.Line - 1),
										Character: uint32(method.Name.Token.Pos.Column - 1),
									},
									End: protocol.Position{
										Line:      uint32(method.Name.Token.Pos.Line - 1),
										Character: uint32(method.Name.Token.Pos.Column - 1 + len(methodName)),
									},
								},
							}
						}
					}
				}
			}
		}
	}

	dp.logger.Debug("Static method %s::%s not found", className, methodName)
	return nil
}

// findInstanceMethod 查找实例方法
func (dp *DefinitionProvider) findInstanceMethod(doc *Document, varName, methodName string, currentLine int) *protocol.Location {
	// 推断变量类型
	className := dp.inferVariableType(doc, varName, currentLine)
	if className == "" {
		dp.logger.Debug("Could not infer type of variable $%s", varName)
		return nil
	}

	dp.logger.Debug("Inferred type of $%s: %s", varName, className)

	// 找到类定义
	classLoc := dp.findClassDefinition(doc, className)
	if classLoc == nil {
		return nil
	}

	// 获取类定义所在的文档
	var classAST *ast.File

	if classLoc.URI == protocol.DocumentURI(doc.URI) {
		// 类在当前文档
		classAST = doc.GetAST()
	} else {
		// 类在其他文件，需要加载
		classPath := uriToPath(string(classLoc.URI))
		imported := dp.importResolver.loadFile(classPath)
		if imported == nil || imported.AST == nil {
			return nil
		}
		classAST = imported.AST
	}

	// 在类中查找方法（实例方法或静态方法）
	if classAST != nil {
		for _, decl := range classAST.Declarations {
			if classDecl, ok := decl.(*ast.ClassDecl); ok {
				if classDecl.Name.Name == className {
					// 查找方法
					for _, method := range classDecl.Methods {
						if method.Name.Name == methodName {
							dp.logger.Debug("Found instance method %s->%s", className, methodName)
							return &protocol.Location{
								URI: classLoc.URI,
								Range: protocol.Range{
									Start: protocol.Position{
										Line:      uint32(method.Name.Token.Pos.Line - 1),
										Character: uint32(method.Name.Token.Pos.Column - 1),
									},
									End: protocol.Position{
										Line:      uint32(method.Name.Token.Pos.Line - 1),
										Character: uint32(method.Name.Token.Pos.Column - 1 + len(methodName)),
									},
								},
							}
						}
					}
				}
			}
		}
	}

	dp.logger.Debug("Instance method %s->%s not found", className, methodName)
	return nil
}

// inferVariableType 推断变量类型
func (dp *DefinitionProvider) inferVariableType(doc *Document, varName string, currentLine int) string {
	astFile := doc.GetAST()
	if astFile == nil {
		return ""
	}

	// 1. 在当前文档的语句中查找变量声明
	for _, stmt := range astFile.Statements {
		if className := inferVarTypeFromStatement(stmt, varName, currentLine); className != "" {
			return className
		}
	}

	// 2. 在类方法中查找变量声明
	for _, decl := range astFile.Declarations {
		if classDecl, ok := decl.(*ast.ClassDecl); ok {
			for _, method := range classDecl.Methods {
				// 检查方法参数
				for _, param := range method.Parameters {
					if param.Name.Name == varName {
						if param.Type != nil {
							return typeNodeToString(param.Type)
						}
					}
				}

				// 检查方法体内的变量声明
				if method.Body != nil {
					if className := inferVarTypeFromStatement(method.Body, varName, currentLine); className != "" {
						return className
					}
				}
			}
		}
	}

	return ""
}

// inferVarTypeFromStatement 从语句中推断变量类型
func inferVarTypeFromStatement(stmt ast.Statement, varName string, currentLine int) string {
	switch s := stmt.(type) {
	case *ast.VarDeclStmt:
		if s.Name.Name == varName {
			// 有显式类型声明
			if s.Type != nil {
				return typeNodeToString(s.Type)
			}
			// 从初始化表达式推断类型
			if s.Value != nil {
				return inferTypeFromExpr(s.Value)
			}
		}

	case *ast.BlockStmt:
		for _, inner := range s.Statements {
			if className := inferVarTypeFromStatement(inner, varName, currentLine); className != "" {
				return className
			}
		}

	case *ast.IfStmt:
		if className := inferVarTypeFromStatement(s.Then, varName, currentLine); className != "" {
			return className
		}
		if s.Else != nil {
			if className := inferVarTypeFromStatement(s.Else, varName, currentLine); className != "" {
				return className
			}
		}

	case *ast.ForStmt:
		if s.Init != nil {
			if className := inferVarTypeFromStatement(s.Init, varName, currentLine); className != "" {
				return className
			}
		}
		if s.Body != nil {
			if className := inferVarTypeFromStatement(s.Body, varName, currentLine); className != "" {
				return className
			}
		}

	case *ast.ForeachStmt:
		if s.Value != nil && s.Value.Name == varName {
			// foreach 的值变量，需要推断迭代对象的元素类型
			// 这里简化处理，返回 dynamic
			return "dynamic"
		}
		if s.Body != nil {
			if className := inferVarTypeFromStatement(s.Body, varName, currentLine); className != "" {
				return className
			}
		}
	}

	return ""
}

// inferTypeFromExpr 从表达式推断类型
func inferTypeFromExpr(expr ast.Expression) string {
	switch e := expr.(type) {
	case *ast.NewExpr:
		// new ClassName() -> ClassName
		if e.ClassName != nil {
			return e.ClassName.Name
		}

	case *ast.CallExpr:
		// 函数调用，暂时返回 dynamic
		return "dynamic"

	case *ast.IntegerLiteral:
		return "int"

	case *ast.FloatLiteral:
		return "float"

	case *ast.StringLiteral:
		return "string"

	case *ast.BoolLiteral:
		return "bool"

	case *ast.NullLiteral:
		return "null"
	}

	return ""
}

// findSymbolInAST 在 AST 中查找符号定义
func (dp *DefinitionProvider) findSymbolInAST(astFile *ast.File, symbolName, uri string) *protocol.Location {
	if astFile == nil {
		return nil
	}

	for _, decl := range astFile.Declarations {
		switch d := decl.(type) {
		case *ast.ClassDecl:
			if d.Name.Name == symbolName {
				return &protocol.Location{
					URI: protocol.DocumentURI(uri),
					Range: protocol.Range{
						Start: protocol.Position{
							Line:      uint32(d.Name.Token.Pos.Line - 1),
							Character: uint32(d.Name.Token.Pos.Column - 1),
						},
						End: protocol.Position{
							Line:      uint32(d.Name.Token.Pos.Line - 1),
							Character: uint32(d.Name.Token.Pos.Column - 1 + len(symbolName)),
						},
					},
				}
			}

		case *ast.InterfaceDecl:
			if d.Name.Name == symbolName {
				return &protocol.Location{
					URI: protocol.DocumentURI(uri),
					Range: protocol.Range{
						Start: protocol.Position{
							Line:      uint32(d.Name.Token.Pos.Line - 1),
							Character: uint32(d.Name.Token.Pos.Column - 1),
						},
						End: protocol.Position{
							Line:      uint32(d.Name.Token.Pos.Line - 1),
							Character: uint32(d.Name.Token.Pos.Column - 1 + len(symbolName)),
						},
					},
				}
			}

		case *ast.EnumDecl:
			if d.Name.Name == symbolName {
				return &protocol.Location{
					URI: protocol.DocumentURI(uri),
					Range: protocol.Range{
						Start: protocol.Position{
							Line:      uint32(d.Name.Token.Pos.Line - 1),
							Character: uint32(d.Name.Token.Pos.Column - 1),
						},
						End: protocol.Position{
							Line:      uint32(d.Name.Token.Pos.Line - 1),
							Character: uint32(d.Name.Token.Pos.Column - 1 + len(symbolName)),
						},
					},
				}
			}
		}
	}

	return nil
}

// typeNodeToString 将类型节点转换为字符串
func typeNodeToString(t ast.TypeNode) string {
	if t == nil {
		return ""
	}

	switch typ := t.(type) {
	case *ast.SimpleType:
		return typ.Name
	case *ast.ClassType:
		return typ.Name.Literal
	case *ast.ArrayType:
		return typeNodeToString(typ.ElementType) + "[]"
	default:
		return ""
	}
}
