package lsp

import (
	"encoding/json"
	"sort"

	"github.com/tangzhangming/nova/internal/ast"
	"go.lsp.dev/protocol"
)

// Semantic token types
const (
	TokenTypeNamespace = iota
	TokenTypeClass
	TokenTypeEnum
	TokenTypeInterface
	TokenTypeStruct
	TokenTypeTypeParameter
	TokenTypeType
	TokenTypeParameter
	TokenTypeVariable
	TokenTypeProperty
	TokenTypeEnumMember
	TokenTypeFunction
	TokenTypeMethod
	TokenTypeKeyword
	TokenTypeModifier
	TokenTypeComment
	TokenTypeString
	TokenTypeNumber
	TokenTypeRegexp
	TokenTypeOperator
)

// Semantic token modifiers
const (
	TokenModDeclaration = 1 << iota
	TokenModDefinition
	TokenModReadonly
	TokenModStatic
	TokenModDeprecated
	TokenModAbstract
	TokenModAsync
	TokenModModification
	TokenModDocumentation
	TokenModDefaultLibrary
)

// SemanticTokenTypes 语义token类型列表
var SemanticTokenTypes = []string{
	"namespace",
	"class",
	"enum",
	"interface",
	"struct",
	"typeParameter",
	"type",
	"parameter",
	"variable",
	"property",
	"enumMember",
	"function",
	"method",
	"keyword",
	"modifier",
	"comment",
	"string",
	"number",
	"regexp",
	"operator",
}

// SemanticTokenModifiers 语义token修饰符列表
var SemanticTokenModifiers = []string{
	"declaration",
	"definition",
	"readonly",
	"static",
	"deprecated",
	"abstract",
	"async",
	"modification",
	"documentation",
	"defaultLibrary",
}

// semanticToken 表示单个语义token
type semanticToken struct {
	Line      uint32
	StartChar uint32
	Length    uint32
	TokenType uint32
	Modifiers uint32
}

// handleSemanticTokensFull 处理全量语义tokens请求
func (s *Server) handleSemanticTokensFull(id json.RawMessage, params json.RawMessage) {
	// 检查是否启用语义高亮
	if s.configManager != nil && !s.configManager.Get().SemanticHighlighting.Enable {
		s.sendResult(id, protocol.SemanticTokens{})
		return
	}
	
	var p protocol.SemanticTokensParams
	if err := json.Unmarshal(params, &p); err != nil {
		s.sendError(id, -32700, "Parse error")
		return
	}

	docURI := string(p.TextDocument.URI)
	doc := s.documents.Get(docURI)
	if doc == nil {
		s.sendResult(id, protocol.SemanticTokens{})
		return
	}

	tokens := s.collectSemanticTokens(doc)
	data := encodeSemanticTokens(tokens)

	result := protocol.SemanticTokens{
		Data: data,
	}

	s.sendResult(id, result)
}

// handleSemanticTokensRange 处理范围语义tokens请求
func (s *Server) handleSemanticTokensRange(id json.RawMessage, params json.RawMessage) {
	var p protocol.SemanticTokensRangeParams
	if err := json.Unmarshal(params, &p); err != nil {
		s.sendError(id, -32700, "Parse error")
		return
	}

	docURI := string(p.TextDocument.URI)
	doc := s.documents.Get(docURI)
	if doc == nil {
		s.sendResult(id, protocol.SemanticTokens{})
		return
	}

	allTokens := s.collectSemanticTokens(doc)

	// 过滤范围内的tokens
	var filteredTokens []semanticToken
	startLine := p.Range.Start.Line
	endLine := p.Range.End.Line

	for _, tok := range allTokens {
		if tok.Line >= startLine && tok.Line <= endLine {
			filteredTokens = append(filteredTokens, tok)
		}
	}

	data := encodeSemanticTokens(filteredTokens)

	result := protocol.SemanticTokens{
		Data: data,
	}

	s.sendResult(id, result)
}

// collectSemanticTokens 收集文档中的所有语义tokens
func (s *Server) collectSemanticTokens(doc *Document) []semanticToken {
	var tokens []semanticToken

	astFile := doc.GetAST()
	if astFile == nil {
		return tokens
	}

	// 收集所有声明的tokens
	for _, decl := range astFile.Declarations {
		tokens = append(tokens, collectDeclTokens(decl)...)
	}

	// 收集语句中的tokens
	for _, stmt := range astFile.Statements {
		tokens = append(tokens, collectStmtTokens(stmt)...)
	}

	// 收集use声明
	for _, use := range astFile.Uses {
		if use != nil {
			// use关键字
			tokens = append(tokens, semanticToken{
				Line:      uint32(use.UseToken.Pos.Line - 1),
				StartChar: uint32(use.UseToken.Pos.Column - 1),
				Length:    3, // "use"
				TokenType: TokenTypeKeyword,
				Modifiers: 0,
			})
		}
	}

	// 按位置排序
	sort.Slice(tokens, func(i, j int) bool {
		if tokens[i].Line != tokens[j].Line {
			return tokens[i].Line < tokens[j].Line
		}
		return tokens[i].StartChar < tokens[j].StartChar
	})

	return tokens
}

// collectDeclTokens 收集声明中的语义tokens
func collectDeclTokens(decl ast.Declaration) []semanticToken {
	var tokens []semanticToken

	switch d := decl.(type) {
	case *ast.ClassDecl:
		// class 关键字
		tokens = append(tokens, semanticToken{
			Line:      uint32(d.ClassToken.Pos.Line - 1),
			StartChar: uint32(d.ClassToken.Pos.Column - 1),
			Length:    5, // "class"
			TokenType: TokenTypeKeyword,
			Modifiers: 0,
		})

		// 类名
		modifiers := uint32(TokenModDeclaration | TokenModDefinition)
		if d.Abstract {
			modifiers |= TokenModAbstract
		}
		tokens = append(tokens, semanticToken{
			Line:      uint32(d.Name.Token.Pos.Line - 1),
			StartChar: uint32(d.Name.Token.Pos.Column - 1),
			Length:    uint32(len(d.Name.Name)),
			TokenType: TokenTypeClass,
			Modifiers: modifiers,
		})

		// extends
		if d.Extends != nil {
			tokens = append(tokens, semanticToken{
				Line:      uint32(d.Extends.Token.Pos.Line - 1),
				StartChar: uint32(d.Extends.Token.Pos.Column - 1),
				Length:    uint32(len(d.Extends.Name)),
				TokenType: TokenTypeClass,
				Modifiers: 0,
			})
		}

		// 泛型参数
		for _, tp := range d.TypeParams {
			tokens = append(tokens, semanticToken{
				Line:      uint32(tp.Name.Token.Pos.Line - 1),
				StartChar: uint32(tp.Name.Token.Pos.Column - 1),
				Length:    uint32(len(tp.Name.Name)),
				TokenType: TokenTypeTypeParameter,
				Modifiers: TokenModDeclaration,
			})
		}

		// 属性
		for _, prop := range d.Properties {
			modifiers := uint32(TokenModDeclaration)
			if prop.Static {
				modifiers |= TokenModStatic
			}
			if prop.Final {
				modifiers |= TokenModReadonly
			}
			tokens = append(tokens, semanticToken{
				Line:      uint32(prop.Name.Token.Pos.Line - 1),
				StartChar: uint32(prop.Name.Token.Pos.Column - 1),
				Length:    uint32(len(prop.Name.Name)),
				TokenType: TokenTypeProperty,
				Modifiers: modifiers,
			})

			// 属性类型
			tokens = append(tokens, collectTypeTokens(prop.Type)...)

			// 属性初值
			if prop.Value != nil {
				tokens = append(tokens, collectExprTokens(prop.Value)...)
			}
		}

		// 方法
		for _, method := range d.Methods {
			// function 关键字
			tokens = append(tokens, semanticToken{
				Line:      uint32(method.FuncToken.Pos.Line - 1),
				StartChar: uint32(method.FuncToken.Pos.Column - 1),
				Length:    8, // "function"
				TokenType: TokenTypeKeyword,
				Modifiers: 0,
			})

			modifiers := uint32(TokenModDeclaration | TokenModDefinition)
			if method.Static {
				modifiers |= TokenModStatic
			}
			if method.Abstract {
				modifiers |= TokenModAbstract
			}
			tokens = append(tokens, semanticToken{
				Line:      uint32(method.Name.Token.Pos.Line - 1),
				StartChar: uint32(method.Name.Token.Pos.Column - 1),
				Length:    uint32(len(method.Name.Name)),
				TokenType: TokenTypeMethod,
				Modifiers: modifiers,
			})

			// 参数
			for _, param := range method.Parameters {
				tokens = append(tokens, semanticToken{
					Line:      uint32(param.Name.Token.Pos.Line - 1),
					StartChar: uint32(param.Name.Token.Pos.Column - 1),
					Length:    uint32(len(param.Name.Name)),
					TokenType: TokenTypeParameter,
					Modifiers: TokenModDeclaration,
				})
				tokens = append(tokens, collectTypeTokens(param.Type)...)
			}

			// 返回类型
			tokens = append(tokens, collectTypeTokens(method.ReturnType)...)

			// 方法体
			if method.Body != nil {
				tokens = append(tokens, collectStmtTokens(method.Body)...)
			}
		}

		// 常量
		for _, c := range d.Constants {
			tokens = append(tokens, semanticToken{
				Line:      uint32(c.Name.Token.Pos.Line - 1),
				StartChar: uint32(c.Name.Token.Pos.Column - 1),
				Length:    uint32(len(c.Name.Name)),
				TokenType: TokenTypeProperty,
				Modifiers: TokenModDeclaration | TokenModReadonly | TokenModStatic,
			})
		}

	case *ast.InterfaceDecl:
		// interface 关键字
		tokens = append(tokens, semanticToken{
			Line:      uint32(d.InterfaceToken.Pos.Line - 1),
			StartChar: uint32(d.InterfaceToken.Pos.Column - 1),
			Length:    9, // "interface"
			TokenType: TokenTypeKeyword,
			Modifiers: 0,
		})

		// 接口名
		tokens = append(tokens, semanticToken{
			Line:      uint32(d.Name.Token.Pos.Line - 1),
			StartChar: uint32(d.Name.Token.Pos.Column - 1),
			Length:    uint32(len(d.Name.Name)),
			TokenType: TokenTypeInterface,
			Modifiers: TokenModDeclaration | TokenModDefinition,
		})

		// 方法签名
		for _, method := range d.Methods {
			tokens = append(tokens, semanticToken{
				Line:      uint32(method.Name.Token.Pos.Line - 1),
				StartChar: uint32(method.Name.Token.Pos.Column - 1),
				Length:    uint32(len(method.Name.Name)),
				TokenType: TokenTypeMethod,
				Modifiers: TokenModDeclaration | TokenModAbstract,
			})

			for _, param := range method.Parameters {
				tokens = append(tokens, semanticToken{
					Line:      uint32(param.Name.Token.Pos.Line - 1),
					StartChar: uint32(param.Name.Token.Pos.Column - 1),
					Length:    uint32(len(param.Name.Name)),
					TokenType: TokenTypeParameter,
					Modifiers: TokenModDeclaration,
				})
			}
		}

	case *ast.EnumDecl:
		// enum 关键字
		tokens = append(tokens, semanticToken{
			Line:      uint32(d.EnumToken.Pos.Line - 1),
			StartChar: uint32(d.EnumToken.Pos.Column - 1),
			Length:    4, // "enum"
			TokenType: TokenTypeKeyword,
			Modifiers: 0,
		})

		// 枚举名
		tokens = append(tokens, semanticToken{
			Line:      uint32(d.Name.Token.Pos.Line - 1),
			StartChar: uint32(d.Name.Token.Pos.Column - 1),
			Length:    uint32(len(d.Name.Name)),
			TokenType: TokenTypeEnum,
			Modifiers: TokenModDeclaration | TokenModDefinition,
		})

		// 枚举值
		for _, c := range d.Cases {
			tokens = append(tokens, semanticToken{
				Line:      uint32(c.Name.Token.Pos.Line - 1),
				StartChar: uint32(c.Name.Token.Pos.Column - 1),
				Length:    uint32(len(c.Name.Name)),
				TokenType: TokenTypeEnumMember,
				Modifiers: TokenModDeclaration | TokenModReadonly,
			})
		}

	case *ast.TypeAliasDecl:
		// type 关键字
		tokens = append(tokens, semanticToken{
			Line:      uint32(d.TypeToken.Pos.Line - 1),
			StartChar: uint32(d.TypeToken.Pos.Column - 1),
			Length:    4, // "type"
			TokenType: TokenTypeKeyword,
			Modifiers: 0,
		})

		// 类型名
		tokens = append(tokens, semanticToken{
			Line:      uint32(d.Name.Token.Pos.Line - 1),
			StartChar: uint32(d.Name.Token.Pos.Column - 1),
			Length:    uint32(len(d.Name.Name)),
			TokenType: TokenTypeType,
			Modifiers: TokenModDeclaration | TokenModDefinition,
		})

		tokens = append(tokens, collectTypeTokens(d.AliasType)...)
	}

	return tokens
}

// collectStmtTokens 收集语句中的语义tokens
func collectStmtTokens(stmt ast.Statement) []semanticToken {
	var tokens []semanticToken

	if stmt == nil {
		return tokens
	}

	switch s := stmt.(type) {
	case *ast.VarDeclStmt:
		// 变量名
		tokens = append(tokens, semanticToken{
			Line:      uint32(s.Name.Token.Pos.Line - 1),
			StartChar: uint32(s.Name.Token.Pos.Column - 1),
			Length:    uint32(len(s.Name.Name)),
			TokenType: TokenTypeVariable,
			Modifiers: TokenModDeclaration,
		})
		tokens = append(tokens, collectTypeTokens(s.Type)...)
		if s.Value != nil {
			tokens = append(tokens, collectExprTokens(s.Value)...)
		}

	case *ast.MultiVarDeclStmt:
		for _, name := range s.Names {
			tokens = append(tokens, semanticToken{
				Line:      uint32(name.Token.Pos.Line - 1),
				StartChar: uint32(name.Token.Pos.Column - 1),
				Length:    uint32(len(name.Name)),
				TokenType: TokenTypeVariable,
				Modifiers: TokenModDeclaration,
			})
		}
		tokens = append(tokens, collectExprTokens(s.Value)...)

	case *ast.ExprStmt:
		tokens = append(tokens, collectExprTokens(s.Expr)...)

	case *ast.BlockStmt:
		for _, inner := range s.Statements {
			tokens = append(tokens, collectStmtTokens(inner)...)
		}

	case *ast.IfStmt:
		// if关键字
		tokens = append(tokens, semanticToken{
			Line:      uint32(s.IfToken.Pos.Line - 1),
			StartChar: uint32(s.IfToken.Pos.Column - 1),
			Length:    2, // "if"
			TokenType: TokenTypeKeyword,
			Modifiers: 0,
		})
		tokens = append(tokens, collectExprTokens(s.Condition)...)
		tokens = append(tokens, collectStmtTokens(s.Then)...)
		if s.Else != nil {
			tokens = append(tokens, collectStmtTokens(s.Else)...)
		}

	case *ast.ForStmt:
		// for关键字
		tokens = append(tokens, semanticToken{
			Line:      uint32(s.ForToken.Pos.Line - 1),
			StartChar: uint32(s.ForToken.Pos.Column - 1),
			Length:    3, // "for"
			TokenType: TokenTypeKeyword,
			Modifiers: 0,
		})
		if s.Init != nil {
			tokens = append(tokens, collectStmtTokens(s.Init)...)
		}
		if s.Condition != nil {
			tokens = append(tokens, collectExprTokens(s.Condition)...)
		}
		if s.Post != nil {
			tokens = append(tokens, collectExprTokens(s.Post)...)
		}
		tokens = append(tokens, collectStmtTokens(s.Body)...)

	case *ast.ForeachStmt:
		// foreach关键字
		tokens = append(tokens, semanticToken{
			Line:      uint32(s.ForeachToken.Pos.Line - 1),
			StartChar: uint32(s.ForeachToken.Pos.Column - 1),
			Length:    7, // "foreach"
			TokenType: TokenTypeKeyword,
			Modifiers: 0,
		})
		if s.Key != nil {
			tokens = append(tokens, semanticToken{
				Line:      uint32(s.Key.Token.Pos.Line - 1),
				StartChar: uint32(s.Key.Token.Pos.Column - 1),
				Length:    uint32(len(s.Key.Name)),
				TokenType: TokenTypeVariable,
				Modifiers: TokenModDeclaration,
			})
		}
		tokens = append(tokens, semanticToken{
			Line:      uint32(s.Value.Token.Pos.Line - 1),
			StartChar: uint32(s.Value.Token.Pos.Column - 1),
			Length:    uint32(len(s.Value.Name)),
			TokenType: TokenTypeVariable,
			Modifiers: TokenModDeclaration,
		})
		tokens = append(tokens, collectExprTokens(s.Iterable)...)
		tokens = append(tokens, collectStmtTokens(s.Body)...)

	case *ast.WhileStmt:
		// while关键字
		tokens = append(tokens, semanticToken{
			Line:      uint32(s.WhileToken.Pos.Line - 1),
			StartChar: uint32(s.WhileToken.Pos.Column - 1),
			Length:    5, // "while"
			TokenType: TokenTypeKeyword,
			Modifiers: 0,
		})
		tokens = append(tokens, collectExprTokens(s.Condition)...)
		tokens = append(tokens, collectStmtTokens(s.Body)...)

	case *ast.ReturnStmt:
		// return关键字
		tokens = append(tokens, semanticToken{
			Line:      uint32(s.ReturnToken.Pos.Line - 1),
			StartChar: uint32(s.ReturnToken.Pos.Column - 1),
			Length:    6, // "return"
			TokenType: TokenTypeKeyword,
			Modifiers: 0,
		})
		for _, val := range s.Values {
			tokens = append(tokens, collectExprTokens(val)...)
		}

	case *ast.TryStmt:
		// try关键字
		tokens = append(tokens, semanticToken{
			Line:      uint32(s.TryToken.Pos.Line - 1),
			StartChar: uint32(s.TryToken.Pos.Column - 1),
			Length:    3, // "try"
			TokenType: TokenTypeKeyword,
			Modifiers: 0,
		})
		tokens = append(tokens, collectStmtTokens(s.Try)...)
		for _, catch := range s.Catches {
			tokens = append(tokens, semanticToken{
				Line:      uint32(catch.Variable.Token.Pos.Line - 1),
				StartChar: uint32(catch.Variable.Token.Pos.Column - 1),
				Length:    uint32(len(catch.Variable.Name)),
				TokenType: TokenTypeVariable,
				Modifiers: TokenModDeclaration,
			})
			tokens = append(tokens, collectTypeTokens(catch.Type)...)
			tokens = append(tokens, collectStmtTokens(catch.Body)...)
		}

	case *ast.ThrowStmt:
		// throw关键字
		tokens = append(tokens, semanticToken{
			Line:      uint32(s.ThrowToken.Pos.Line - 1),
			StartChar: uint32(s.ThrowToken.Pos.Column - 1),
			Length:    5, // "throw"
			TokenType: TokenTypeKeyword,
			Modifiers: 0,
		})
		tokens = append(tokens, collectExprTokens(s.Exception)...)

	case *ast.EchoStmt:
		// echo关键字
		tokens = append(tokens, semanticToken{
			Line:      uint32(s.EchoToken.Pos.Line - 1),
			StartChar: uint32(s.EchoToken.Pos.Column - 1),
			Length:    4, // "echo"
			TokenType: TokenTypeKeyword,
			Modifiers: 0,
		})
		tokens = append(tokens, collectExprTokens(s.Value)...)

	case *ast.BreakStmt:
		tokens = append(tokens, semanticToken{
			Line:      uint32(s.BreakToken.Pos.Line - 1),
			StartChar: uint32(s.BreakToken.Pos.Column - 1),
			Length:    5, // "break"
			TokenType: TokenTypeKeyword,
			Modifiers: 0,
		})

	case *ast.ContinueStmt:
		tokens = append(tokens, semanticToken{
			Line:      uint32(s.ContinueToken.Pos.Line - 1),
			StartChar: uint32(s.ContinueToken.Pos.Column - 1),
			Length:    8, // "continue"
			TokenType: TokenTypeKeyword,
			Modifiers: 0,
		})
	}

	return tokens
}

// collectExprTokens 收集表达式中的语义tokens
func collectExprTokens(expr ast.Expression) []semanticToken {
	var tokens []semanticToken

	if expr == nil {
		return tokens
	}

	switch e := expr.(type) {
	case *ast.Variable:
		tokens = append(tokens, semanticToken{
			Line:      uint32(e.Token.Pos.Line - 1),
			StartChar: uint32(e.Token.Pos.Column - 1),
			Length:    uint32(len(e.Name) + 1), // +1 for $
			TokenType: TokenTypeVariable,
			Modifiers: 0,
		})

	case *ast.Identifier:
		tokens = append(tokens, semanticToken{
			Line:      uint32(e.Token.Pos.Line - 1),
			StartChar: uint32(e.Token.Pos.Column - 1),
			Length:    uint32(len(e.Name)),
			TokenType: TokenTypeFunction,
			Modifiers: 0,
		})

	case *ast.IntegerLiteral:
		tokens = append(tokens, semanticToken{
			Line:      uint32(e.Token.Pos.Line - 1),
			StartChar: uint32(e.Token.Pos.Column - 1),
			Length:    uint32(len(e.Token.Literal)),
			TokenType: TokenTypeNumber,
			Modifiers: 0,
		})

	case *ast.FloatLiteral:
		tokens = append(tokens, semanticToken{
			Line:      uint32(e.Token.Pos.Line - 1),
			StartChar: uint32(e.Token.Pos.Column - 1),
			Length:    uint32(len(e.Token.Literal)),
			TokenType: TokenTypeNumber,
			Modifiers: 0,
		})

	case *ast.StringLiteral:
		tokens = append(tokens, semanticToken{
			Line:      uint32(e.Token.Pos.Line - 1),
			StartChar: uint32(e.Token.Pos.Column - 1),
			Length:    uint32(len(e.Value) + 2), // +2 for quotes
			TokenType: TokenTypeString,
			Modifiers: 0,
		})

	case *ast.BinaryExpr:
		tokens = append(tokens, collectExprTokens(e.Left)...)
		tokens = append(tokens, collectExprTokens(e.Right)...)

	case *ast.UnaryExpr:
		tokens = append(tokens, collectExprTokens(e.Operand)...)

	case *ast.AssignExpr:
		tokens = append(tokens, collectExprTokens(e.Left)...)
		tokens = append(tokens, collectExprTokens(e.Right)...)

	case *ast.CallExpr:
		tokens = append(tokens, collectExprTokens(e.Function)...)
		for _, arg := range e.Arguments {
			tokens = append(tokens, collectExprTokens(arg)...)
		}

	case *ast.MethodCall:
		tokens = append(tokens, collectExprTokens(e.Object)...)
		tokens = append(tokens, semanticToken{
			Line:      uint32(e.Method.Token.Pos.Line - 1),
			StartChar: uint32(e.Method.Token.Pos.Column - 1),
			Length:    uint32(len(e.Method.Name)),
			TokenType: TokenTypeMethod,
			Modifiers: 0,
		})
		for _, arg := range e.Arguments {
			tokens = append(tokens, collectExprTokens(arg)...)
		}

	case *ast.PropertyAccess:
		tokens = append(tokens, collectExprTokens(e.Object)...)
		tokens = append(tokens, semanticToken{
			Line:      uint32(e.Property.Token.Pos.Line - 1),
			StartChar: uint32(e.Property.Token.Pos.Column - 1),
			Length:    uint32(len(e.Property.Name)),
			TokenType: TokenTypeProperty,
			Modifiers: 0,
		})

	case *ast.IndexExpr:
		tokens = append(tokens, collectExprTokens(e.Object)...)
		tokens = append(tokens, collectExprTokens(e.Index)...)

	case *ast.NewExpr:
		// new关键字
		tokens = append(tokens, semanticToken{
			Line:      uint32(e.NewToken.Pos.Line - 1),
			StartChar: uint32(e.NewToken.Pos.Column - 1),
			Length:    3, // "new"
			TokenType: TokenTypeKeyword,
			Modifiers: 0,
		})
		tokens = append(tokens, semanticToken{
			Line:      uint32(e.ClassName.Token.Pos.Line - 1),
			StartChar: uint32(e.ClassName.Token.Pos.Column - 1),
			Length:    uint32(len(e.ClassName.Name)),
			TokenType: TokenTypeClass,
			Modifiers: 0,
		})
		for _, arg := range e.Arguments {
			tokens = append(tokens, collectExprTokens(arg)...)
		}

	case *ast.ArrayLiteral:
		for _, elem := range e.Elements {
			tokens = append(tokens, collectExprTokens(elem)...)
		}

	case *ast.MapLiteral:
		for _, pair := range e.Pairs {
			tokens = append(tokens, collectExprTokens(pair.Key)...)
			tokens = append(tokens, collectExprTokens(pair.Value)...)
		}

	case *ast.TernaryExpr:
		tokens = append(tokens, collectExprTokens(e.Condition)...)
		tokens = append(tokens, collectExprTokens(e.Then)...)
		tokens = append(tokens, collectExprTokens(e.Else)...)

	case *ast.IsExpr:
		tokens = append(tokens, collectExprTokens(e.Expr)...)
		tokens = append(tokens, collectTypeTokens(e.TypeName)...)

	case *ast.TypeCastExpr:
		tokens = append(tokens, collectTypeTokens(e.TargetType)...)
		tokens = append(tokens, collectExprTokens(e.Expr)...)

	case *ast.StaticAccess:
		if ident, ok := e.Class.(*ast.Identifier); ok {
			tokens = append(tokens, semanticToken{
				Line:      uint32(ident.Token.Pos.Line - 1),
				StartChar: uint32(ident.Token.Pos.Column - 1),
				Length:    uint32(len(ident.Name)),
				TokenType: TokenTypeClass,
				Modifiers: 0,
			})
		}

	case *ast.ThisExpr:
		tokens = append(tokens, semanticToken{
			Line:      uint32(e.Token.Pos.Line - 1),
			StartChar: uint32(e.Token.Pos.Column - 1),
			Length:    5, // "$this"
			TokenType: TokenTypeKeyword,
			Modifiers: 0,
		})

	case *ast.NullCoalesceExpr:
		tokens = append(tokens, collectExprTokens(e.Left)...)
		tokens = append(tokens, collectExprTokens(e.Right)...)

	case *ast.SafePropertyAccess:
		tokens = append(tokens, collectExprTokens(e.Object)...)
		tokens = append(tokens, semanticToken{
			Line:      uint32(e.Property.Token.Pos.Line - 1),
			StartChar: uint32(e.Property.Token.Pos.Column - 1),
			Length:    uint32(len(e.Property.Name)),
			TokenType: TokenTypeProperty,
			Modifiers: 0,
		})

	case *ast.SafeMethodCall:
		tokens = append(tokens, collectExprTokens(e.Object)...)
		tokens = append(tokens, semanticToken{
			Line:      uint32(e.Method.Token.Pos.Line - 1),
			StartChar: uint32(e.Method.Token.Pos.Column - 1),
			Length:    uint32(len(e.Method.Name)),
			TokenType: TokenTypeMethod,
			Modifiers: 0,
		})
		for _, arg := range e.Arguments {
			tokens = append(tokens, collectExprTokens(arg)...)
		}
	}

	return tokens
}

// collectTypeTokens 收集类型节点中的语义tokens
func collectTypeTokens(t ast.TypeNode) []semanticToken {
	var tokens []semanticToken

	if t == nil {
		return tokens
	}

	switch typ := t.(type) {
	case *ast.SimpleType:
		tokenType := TokenTypeType
		if isBuiltinType(typ.Name) {
			tokenType = TokenTypeKeyword
		}
		tokens = append(tokens, semanticToken{
			Line:      uint32(typ.Token.Pos.Line - 1),
			StartChar: uint32(typ.Token.Pos.Column - 1),
			Length:    uint32(len(typ.Name)),
			TokenType: uint32(tokenType),
			Modifiers: 0,
		})

	case *ast.ClassType:
		tokens = append(tokens, semanticToken{
			Line:      uint32(typ.Name.Pos.Line - 1),
			StartChar: uint32(typ.Name.Pos.Column - 1),
			Length:    uint32(len(typ.Name.Literal)),
			TokenType: TokenTypeClass,
			Modifiers: 0,
		})

	case *ast.ArrayType:
		tokens = append(tokens, collectTypeTokens(typ.ElementType)...)

	case *ast.MapType:
		tokens = append(tokens, collectTypeTokens(typ.KeyType)...)
		tokens = append(tokens, collectTypeTokens(typ.ValueType)...)

	case *ast.NullableType:
		tokens = append(tokens, collectTypeTokens(typ.Inner)...)

	case *ast.UnionType:
		for _, inner := range typ.Types {
			tokens = append(tokens, collectTypeTokens(inner)...)
		}

	case *ast.GenericType:
		tokens = append(tokens, collectTypeTokens(typ.BaseType)...)
		for _, arg := range typ.TypeArgs {
			tokens = append(tokens, collectTypeTokens(arg)...)
		}
	}

	return tokens
}

// encodeSemanticTokens 将语义tokens编码为LSP格式
// LSP使用差值编码: [deltaLine, deltaStartChar, length, tokenType, tokenModifiers]
func encodeSemanticTokens(tokens []semanticToken) []uint32 {
	if len(tokens) == 0 {
		return []uint32{}
	}

	data := make([]uint32, 0, len(tokens)*5)

	var prevLine, prevChar uint32 = 0, 0

	for _, tok := range tokens {
		deltaLine := tok.Line - prevLine
		var deltaChar uint32
		if deltaLine == 0 {
			deltaChar = tok.StartChar - prevChar
		} else {
			deltaChar = tok.StartChar
		}

		data = append(data,
			deltaLine,
			deltaChar,
			tok.Length,
			tok.TokenType,
			tok.Modifiers,
		)

		prevLine = tok.Line
		prevChar = tok.StartChar
	}

	return data
}

// SemanticTokensProviderOptions 语义tokens提供者选项
type SemanticTokensProviderOptions struct {
	Legend protocol.SemanticTokensLegend `json:"legend"`
	Full   bool                          `json:"full,omitempty"`
	Range  bool                          `json:"range,omitempty"`
}

// getSemanticTokensLegend 获取语义tokens图例
func getSemanticTokensLegend() protocol.SemanticTokensLegend {
	tokenTypes := make([]protocol.SemanticTokenTypes, len(SemanticTokenTypes))
	for i, t := range SemanticTokenTypes {
		tokenTypes[i] = protocol.SemanticTokenTypes(t)
	}

	tokenModifiers := make([]protocol.SemanticTokenModifiers, len(SemanticTokenModifiers))
	for i, m := range SemanticTokenModifiers {
		tokenModifiers[i] = protocol.SemanticTokenModifiers(m)
	}

	return protocol.SemanticTokensLegend{
		TokenTypes:     tokenTypes,
		TokenModifiers: tokenModifiers,
	}
}

// getSemanticTokensProviderOptions 获取语义tokens提供者选项
func getSemanticTokensProviderOptions() SemanticTokensProviderOptions {
	return SemanticTokensProviderOptions{
		Legend: getSemanticTokensLegend(),
		Full:   true,
		Range:  true,
	}
}
