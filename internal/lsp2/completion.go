package lsp2

import (
	"encoding/json"
	"strings"

	"github.com/tangzhangming/nova/internal/ast"
	"go.lsp.dev/protocol"
)

// 补全上下文类型
const (
	CompletionMember   = 1 // $obj->
	CompletionStatic   = 2 // Class::
	CompletionVariable = 3 // $
	CompletionNew      = 4 // new
	CompletionGeneral  = 5 // 一般输入
)

// CompletionContext 补全上下文
type CompletionContext struct {
	Type       int
	ObjectName string // 用于 Member: $obj
	ClassName  string // 用于 Static: ClassName
}

// 最大补全项数量
const maxCompletionItems = 50

// handleCompletion 处理补全请求
func (s *Server) handleCompletion(id json.RawMessage, params json.RawMessage) {
	var p protocol.CompletionParams
	if err := json.Unmarshal(params, &p); err != nil {
		s.sendError(id, -32700, "Parse error")
		return
	}

	docURI := string(p.TextDocument.URI)
	doc := s.docManager.Get(docURI)
	if doc == nil {
		s.sendResult(id, nil)
		return
	}

	line := int(p.Position.Line)
	character := int(p.Position.Character)

	// 获取补全项
	items := s.getCompletionItems(doc, line, character)
	s.sendResult(id, items)
}

// getCompletionItems 获取补全项
func (s *Server) getCompletionItems(doc *Document, line, character int) []protocol.CompletionItem {
	if line < 0 || line >= len(doc.Lines) {
		return nil
	}

	lineText := doc.Lines[line]
	if character > len(lineText) {
		character = len(lineText)
	}

	// 获取光标前的文本
	prefix := lineText[:character]

	// 检测补全上下文
	ctx := detectCompletionContext(prefix)

	s.logger.Debug("Completion context: type=%d, obj=%s, class=%s", ctx.Type, ctx.ObjectName, ctx.ClassName)

	var items []protocol.CompletionItem

	switch ctx.Type {
	case CompletionMember:
		items = s.getMemberCompletions(doc, ctx.ObjectName, line)
	case CompletionStatic:
		items = s.getStaticCompletions(doc, ctx.ClassName)
	case CompletionVariable:
		items = s.getVariableCompletions(doc, line)
	case CompletionNew:
		items = s.getClassCompletions(doc)
	case CompletionGeneral:
		items = s.getGeneralCompletions(doc, prefix)
	}

	// 限制补全项数量
	if len(items) > maxCompletionItems {
		items = items[:maxCompletionItems]
	}

	return items
}

// detectCompletionContext 检测补全上下文
func detectCompletionContext(prefix string) CompletionContext {
	prefix = strings.TrimRight(prefix, " \t")

	// 检查 $obj->
	if strings.HasSuffix(prefix, "->") {
		// 提取对象名
		beforeArrow := prefix[:len(prefix)-2]
		objName := extractLastWord(beforeArrow)
		if strings.HasPrefix(objName, "$") {
			objName = objName[1:]
		}
		return CompletionContext{Type: CompletionMember, ObjectName: objName}
	}

	// 检查 Class::
	if strings.HasSuffix(prefix, "::") {
		beforeColons := prefix[:len(prefix)-2]
		className := extractLastWord(beforeColons)
		return CompletionContext{Type: CompletionStatic, ClassName: className}
	}

	// 检查 $
	if strings.HasSuffix(prefix, "$") || (len(prefix) > 0 && prefix[len(prefix)-1] == '$') {
		return CompletionContext{Type: CompletionVariable}
	}

	// 检查 new
	if strings.HasSuffix(prefix, "new ") || strings.HasSuffix(prefix, "new\t") {
		return CompletionContext{Type: CompletionNew}
	}

	// 检查是否在输入 $ 后的变量名
	lastWord := extractLastWord(prefix)
	if len(lastWord) > 0 {
		// 查找 lastWord 前面是否有 $
		wordStart := len(prefix) - len(lastWord)
		if wordStart > 0 && prefix[wordStart-1] == '$' {
			return CompletionContext{Type: CompletionVariable}
		}
	}

	return CompletionContext{Type: CompletionGeneral}
}

// extractLastWord 提取最后一个单词
func extractLastWord(s string) string {
	s = strings.TrimRight(s, " \t")
	if len(s) == 0 {
		return ""
	}

	end := len(s)
	start := end - 1
	for start >= 0 && (isWordCharByte(s[start]) || s[start] == '$') {
		start--
	}
	start++

	return s[start:end]
}

// getMemberCompletions 获取实例成员补全
func (s *Server) getMemberCompletions(doc *Document, objName string, line int) []protocol.CompletionItem {
	var items []protocol.CompletionItem

	// 推断变量类型
	var className string
	if objName == "this" {
		className = s.getCurrentClassName(doc)
	} else {
		className = s.definitionProvider.inferVariableType(doc, objName, line)
	}

	if className == "" || className == "dynamic" {
		return items
	}

	s.logger.Debug("Member completion for $%s (type: %s)", objName, className)

	// 在当前文档中查找类
	astFile := doc.GetAST()
	if astFile != nil {
		items = append(items, findClassMembers(astFile, className, false)...)
	}

	// 在导入的文件中查找
	imports := s.importResolver.ResolveImports(doc)
	for _, imported := range imports {
		if imported.AST != nil {
			items = append(items, findClassMembers(imported.AST, className, false)...)
		}
	}

	return items
}

// getStaticCompletions 获取静态成员补全
func (s *Server) getStaticCompletions(doc *Document, className string) []protocol.CompletionItem {
	var items []protocol.CompletionItem

	// 在当前文档中查找类
	astFile := doc.GetAST()
	if astFile != nil {
		items = append(items, findClassMembers(astFile, className, true)...)
		items = append(items, findEnumCases(astFile, className)...)
	}

	// 在导入的文件中查找
	imports := s.importResolver.ResolveImports(doc)
	for _, imported := range imports {
		if imported.AST != nil {
			items = append(items, findClassMembers(imported.AST, className, true)...)
			items = append(items, findEnumCases(imported.AST, className)...)
		}
	}

	return items
}

// findClassMembers 在AST中查找类成员
func findClassMembers(astFile *ast.File, className string, staticOnly bool) []protocol.CompletionItem {
	var items []protocol.CompletionItem

	for _, decl := range astFile.Declarations {
		if classDecl, ok := decl.(*ast.ClassDecl); ok && classDecl.Name.Name == className {
			// 添加方法
			for _, method := range classDecl.Methods {
				if staticOnly && !method.Static {
					continue
				}
				if !staticOnly && method.Static {
					continue
				}

				kind := protocol.CompletionItemKindMethod
				label := method.Name.Name

				// 构建详情
				var params []string
				for _, param := range method.Parameters {
					paramStr := "$" + param.Name.Name
					if param.Type != nil {
						paramStr = typeNodeToString(param.Type) + " " + paramStr
					}
					params = append(params, paramStr)
				}
				detail := "(" + strings.Join(params, ", ") + ")"
				if method.ReturnType != nil {
					detail += ": " + typeNodeToString(method.ReturnType)
				}

				items = append(items, protocol.CompletionItem{
					Label:  label,
					Kind:   kind,
					Detail: detail,
				})
			}

			// 添加属性
			for _, prop := range classDecl.Properties {
				if staticOnly && !prop.Static {
					continue
				}
				if !staticOnly && prop.Static {
					continue
				}

				kind := protocol.CompletionItemKindField
				label := prop.Name.Name

				// 静态属性需要$前缀
				if staticOnly {
					label = "$" + label
				}

				detail := ""
				if prop.Type != nil {
					detail = typeNodeToString(prop.Type)
				}

				items = append(items, protocol.CompletionItem{
					Label:  label,
					Kind:   kind,
					Detail: detail,
				})
			}

			// 如果是静态访问，添加常量
			if staticOnly {
				for _, c := range classDecl.Constants {
					items = append(items, protocol.CompletionItem{
						Label:  c.Name.Name,
						Kind:   protocol.CompletionItemKindConstant,
						Detail: "const",
					})
				}
			}
		}
	}

	return items
}

// findEnumCases 查找枚举值
func findEnumCases(astFile *ast.File, enumName string) []protocol.CompletionItem {
	var items []protocol.CompletionItem

	for _, decl := range astFile.Declarations {
		if enumDecl, ok := decl.(*ast.EnumDecl); ok && enumDecl.Name.Name == enumName {
			for _, c := range enumDecl.Cases {
				items = append(items, protocol.CompletionItem{
					Label:  c.Name.Name,
					Kind:   protocol.CompletionItemKindEnumMember,
					Detail: "enum case",
				})
			}
		}
	}

	return items
}

// getVariableCompletions 获取变量补全
func (s *Server) getVariableCompletions(doc *Document, line int) []protocol.CompletionItem {
	var items []protocol.CompletionItem
	seen := make(map[string]bool)

	// 添加常用关键字变量
	keywords := []string{"this"}
	for _, kw := range keywords {
		items = append(items, protocol.CompletionItem{
			Label: kw,
			Kind:  protocol.CompletionItemKindKeyword,
		})
		seen[kw] = true
	}

	astFile := doc.GetAST()
	if astFile == nil {
		return items
	}

	// 收集变量
	collectVariables(astFile, &items, seen, line)

	return items
}

// collectVariables 收集变量
func collectVariables(astFile *ast.File, items *[]protocol.CompletionItem, seen map[string]bool, currentLine int) {
	// 从语句中收集
	for _, stmt := range astFile.Statements {
		collectVariablesFromStmt(stmt, items, seen, currentLine)
	}

	// 从类中收集
	for _, decl := range astFile.Declarations {
		if classDecl, ok := decl.(*ast.ClassDecl); ok {
			// 类属性
			for _, prop := range classDecl.Properties {
				if !seen[prop.Name.Name] {
					seen[prop.Name.Name] = true
					detail := ""
					if prop.Type != nil {
						detail = typeNodeToString(prop.Type)
					}
					*items = append(*items, protocol.CompletionItem{
						Label:  prop.Name.Name,
						Kind:   protocol.CompletionItemKindField,
						Detail: detail,
					})
				}
			}

			// 方法参数
			for _, method := range classDecl.Methods {
				for _, param := range method.Parameters {
					if !seen[param.Name.Name] {
						seen[param.Name.Name] = true
						detail := ""
						if param.Type != nil {
							detail = typeNodeToString(param.Type)
						}
						*items = append(*items, protocol.CompletionItem{
							Label:  param.Name.Name,
							Kind:   protocol.CompletionItemKindVariable,
							Detail: detail,
						})
					}
				}

				// 方法体中的变量
				if method.Body != nil {
					collectVariablesFromStmt(method.Body, items, seen, currentLine)
				}
			}
		}
	}
}

// collectVariablesFromStmt 从语句中收集变量
func collectVariablesFromStmt(stmt ast.Statement, items *[]protocol.CompletionItem, seen map[string]bool, currentLine int) {
	switch s := stmt.(type) {
	case *ast.VarDeclStmt:
		if !seen[s.Name.Name] {
			seen[s.Name.Name] = true
			detail := ""
			if s.Type != nil {
				detail = typeNodeToString(s.Type)
			}
			*items = append(*items, protocol.CompletionItem{
				Label:  s.Name.Name,
				Kind:   protocol.CompletionItemKindVariable,
				Detail: detail,
			})
		}
	case *ast.BlockStmt:
		for _, inner := range s.Statements {
			collectVariablesFromStmt(inner, items, seen, currentLine)
		}
	case *ast.IfStmt:
		collectVariablesFromStmt(s.Then, items, seen, currentLine)
		if s.Else != nil {
			collectVariablesFromStmt(s.Else, items, seen, currentLine)
		}
	case *ast.ForStmt:
		if s.Init != nil {
			collectVariablesFromStmt(s.Init, items, seen, currentLine)
		}
		if s.Body != nil {
			collectVariablesFromStmt(s.Body, items, seen, currentLine)
		}
	case *ast.ForeachStmt:
		if s.Key != nil && !seen[s.Key.Name] {
			seen[s.Key.Name] = true
			*items = append(*items, protocol.CompletionItem{
				Label:  s.Key.Name,
				Kind:   protocol.CompletionItemKindVariable,
				Detail: "foreach key",
			})
		}
		if s.Value != nil && !seen[s.Value.Name] {
			seen[s.Value.Name] = true
			*items = append(*items, protocol.CompletionItem{
				Label:  s.Value.Name,
				Kind:   protocol.CompletionItemKindVariable,
				Detail: "foreach value",
			})
		}
		if s.Body != nil {
			collectVariablesFromStmt(s.Body, items, seen, currentLine)
		}
	}
}

// getClassCompletions 获取类名补全（用于 new）
func (s *Server) getClassCompletions(doc *Document) []protocol.CompletionItem {
	var items []protocol.CompletionItem
	seen := make(map[string]bool)

	// 当前文档的类
	astFile := doc.GetAST()
	if astFile != nil {
		for _, decl := range astFile.Declarations {
			if classDecl, ok := decl.(*ast.ClassDecl); ok {
				if !classDecl.Abstract && !seen[classDecl.Name.Name] {
					seen[classDecl.Name.Name] = true
					items = append(items, protocol.CompletionItem{
						Label: classDecl.Name.Name,
						Kind:  protocol.CompletionItemKindClass,
					})
				}
			}
		}
	}

	// 导入的类
	imports := s.importResolver.ResolveImports(doc)
	for _, imported := range imports {
		if imported.AST != nil {
			for _, decl := range imported.AST.Declarations {
				if classDecl, ok := decl.(*ast.ClassDecl); ok {
					if !classDecl.Abstract && !seen[classDecl.Name.Name] {
						seen[classDecl.Name.Name] = true
						items = append(items, protocol.CompletionItem{
							Label: classDecl.Name.Name,
							Kind:  protocol.CompletionItemKindClass,
						})
					}
				}
			}
		}
	}

	return items
}

// getGeneralCompletions 获取通用补全
func (s *Server) getGeneralCompletions(doc *Document, prefix string) []protocol.CompletionItem {
	var items []protocol.CompletionItem
	seen := make(map[string]bool)

	// 添加关键字
	keywords := []string{
		"class", "interface", "enum", "trait",
		"function", "static", "public", "private", "protected",
		"extends", "implements", "use",
		"if", "else", "elseif", "switch", "case", "default",
		"for", "foreach", "while", "do",
		"return", "break", "continue",
		"try", "catch", "finally", "throw",
		"new", "null", "true", "false",
		"const", "var", "final", "abstract",
	}

	for _, kw := range keywords {
		items = append(items, protocol.CompletionItem{
			Label: kw,
			Kind:  protocol.CompletionItemKindKeyword,
		})
		seen[kw] = true
	}

	// 添加当前文档的类和接口
	astFile := doc.GetAST()
	if astFile != nil {
		for _, decl := range astFile.Declarations {
			switch d := decl.(type) {
			case *ast.ClassDecl:
				if !seen[d.Name.Name] {
					seen[d.Name.Name] = true
					items = append(items, protocol.CompletionItem{
						Label: d.Name.Name,
						Kind:  protocol.CompletionItemKindClass,
					})
				}
			case *ast.InterfaceDecl:
				if !seen[d.Name.Name] {
					seen[d.Name.Name] = true
					items = append(items, protocol.CompletionItem{
						Label: d.Name.Name,
						Kind:  protocol.CompletionItemKindInterface,
					})
				}
			case *ast.EnumDecl:
				if !seen[d.Name.Name] {
					seen[d.Name.Name] = true
					items = append(items, protocol.CompletionItem{
						Label: d.Name.Name,
						Kind:  protocol.CompletionItemKindEnum,
					})
				}
			}
		}
	}

	// 添加导入的类
	imports := s.importResolver.ResolveImports(doc)
	for _, imported := range imports {
		if imported.AST != nil {
			for _, decl := range imported.AST.Declarations {
				switch d := decl.(type) {
				case *ast.ClassDecl:
					if !seen[d.Name.Name] {
						seen[d.Name.Name] = true
						items = append(items, protocol.CompletionItem{
							Label: d.Name.Name,
							Kind:  protocol.CompletionItemKindClass,
						})
					}
				case *ast.InterfaceDecl:
					if !seen[d.Name.Name] {
						seen[d.Name.Name] = true
						items = append(items, protocol.CompletionItem{
							Label: d.Name.Name,
							Kind:  protocol.CompletionItemKindInterface,
						})
					}
				case *ast.EnumDecl:
					if !seen[d.Name.Name] {
						seen[d.Name.Name] = true
						items = append(items, protocol.CompletionItem{
							Label: d.Name.Name,
							Kind:  protocol.CompletionItemKindEnum,
						})
					}
				}
			}
		}
	}

	return items
}
