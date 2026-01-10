package lsp2

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/tangzhangming/nova/internal/ast"
	"go.lsp.dev/protocol"
)

// 补全上下文类型
const (
	CompletionMember    = 1 // $obj->
	CompletionStatic    = 2 // Class::
	CompletionVariable  = 3 // $
	CompletionNew       = 4 // new
	CompletionGeneral   = 5 // 一般输入
	CompletionNamespace = 6 // use xxx.yyy.
)

// CompletionContext 补全上下文
type CompletionContext struct {
	Type          int
	ObjectName    string // 用于 Member: $obj
	ClassName     string // 用于 Static: ClassName
	NamespacePath string // 用于 Namespace: use sola.collections.
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
	case CompletionNamespace:
		items = s.getNamespaceCompletions(doc, ctx.NamespacePath)
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
	// 检查 use xxx.yyy.（命名空间补全）- 在 TrimRight 之前检查
	if nsPath := extractNamespacePath(prefix); nsPath != "" {
		return CompletionContext{Type: CompletionNamespace, NamespacePath: nsPath}
	}

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

// extractNamespacePath 提取 use 语句中的命名空间路径
// 例如: "use sola.collections." -> "sola.collections"
// 例如: "use sola." -> "sola"
// 例如: "use " -> "" (顶级，返回空字符串但会被处理)
func extractNamespacePath(prefix string) string {
	// 查找 use 关键字
	trimmed := strings.TrimLeft(prefix, " \t")

	// 检查是否以 use 开头
	if !strings.HasPrefix(trimmed, "use ") && !strings.HasPrefix(trimmed, "use\t") {
		return ""
	}

	// 提取 use 后面的内容
	afterUse := strings.TrimPrefix(trimmed, "use")
	afterUse = strings.TrimLeft(afterUse, " \t")

	// 如果以 . 结尾，表示正在补全下一级
	if strings.HasSuffix(afterUse, ".") {
		// 返回 . 之前的部分
		return afterUse[:len(afterUse)-1]
	}

	// 如果没有任何内容，返回特殊标记表示顶级补全
	if afterUse == "" {
		return "."
	}

	// 如果正在输入某个命名空间部分（没有以 . 结尾）
	// 检查是否有 . 分隔符
	if lastDot := strings.LastIndex(afterUse, "."); lastDot != -1 {
		// 返回最后一个 . 之前的部分
		return afterUse[:lastDot]
	}

	// 正在输入顶级命名空间
	return "."
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

				// 设置插入文本：方法名+括号
				// 使用Snippet格式，$0表示光标最终位置
				var insertText string
				if len(method.Parameters) == 0 {
					insertText = label + "()$0"
				} else {
					insertText = label + "($0)"
				}

				items = append(items, protocol.CompletionItem{
					Label:            label,
					Kind:             kind,
					Detail:           detail,
					InsertText:       insertText,
					InsertTextFormat: protocol.InsertTextFormatSnippet,
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

// getNamespaceCompletions 获取命名空间补全
func (s *Server) getNamespaceCompletions(doc *Document, nsPath string) []protocol.CompletionItem {
	var items []protocol.CompletionItem
	seen := make(map[string]bool)

	s.logger.Debug("Namespace completion for: %s", nsPath)

	// 特殊处理：顶级补全（use 后面没有内容）
	if nsPath == "." {
		// 提示 sola 标准库
		items = append(items, protocol.CompletionItem{
			Label:  "sola",
			Kind:   protocol.CompletionItemKindModule,
			Detail: "标准库",
		})
		seen["sola"] = true

		// 如果有项目命名空间，也提示
		projectNs := s.getProjectNamespace(doc)
		if projectNs != "" {
			parts := strings.Split(projectNs, ".")
			if len(parts) > 0 && !seen[parts[0]] {
				items = append(items, protocol.CompletionItem{
					Label:  parts[0],
					Kind:   protocol.CompletionItemKindModule,
					Detail: "项目命名空间",
				})
			}
		}
		return items
	}

	parts := strings.Split(nsPath, ".")

	// 处理标准库命名空间 (sola.*)
	if parts[0] == "sola" {
		libDir := s.getStdLibDir()
		if libDir == "" {
			return items
		}

		// 构建目录路径
		var dirPath string
		if len(parts) == 1 {
			// sola. -> 列出标准库根目录
			dirPath = libDir
		} else {
			// sola.collections. -> 列出 src/collections/
			dirPath = filepath.Join(libDir, filepath.Join(parts[1:]...))
		}

		items = s.scanNamespaceDir(dirPath, seen)
	}

	// 处理项目命名空间
	projectNs := s.getProjectNamespace(doc)
	if projectNs != "" && strings.HasPrefix(nsPath, projectNs) {
		// 获取项目根目录
		projectRoot := s.getProjectRoot(doc)
		if projectRoot != "" {
			relativePath := strings.TrimPrefix(nsPath, projectNs)
			relativePath = strings.TrimPrefix(relativePath, ".")

			var dirPath string
			if relativePath == "" {
				dirPath = filepath.Join(projectRoot, "src")
			} else {
				pathParts := strings.Split(relativePath, ".")
				dirPath = filepath.Join(projectRoot, "src", filepath.Join(pathParts...))
			}

			projectItems := s.scanNamespaceDir(dirPath, seen)
			items = append(items, projectItems...)
		}
	}

	return items
}

// scanNamespaceDir 扫描目录获取命名空间补全项
func (s *Server) scanNamespaceDir(dirPath string, seen map[string]bool) []protocol.CompletionItem {
	var items []protocol.CompletionItem

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		s.logger.Debug("Failed to read dir %s: %v", dirPath, err)
		return items
	}

	for _, entry := range entries {
		name := entry.Name()

		// 跳过隐藏文件和已处理的名称
		if strings.HasPrefix(name, ".") {
			continue
		}

		if entry.IsDir() {
			// 目录 -> 子命名空间
			if !seen[name] {
				seen[name] = true
				items = append(items, protocol.CompletionItem{
					Label:  name,
					Kind:   protocol.CompletionItemKindModule,
					Detail: "命名空间",
				})
			}
		} else if strings.HasSuffix(name, ".sola") {
			// .sola 文件 -> 类名
			className := strings.TrimSuffix(name, ".sola")
			if !seen[className] && className != "README" {
				seen[className] = true
				items = append(items, protocol.CompletionItem{
					Label:  className,
					Kind:   protocol.CompletionItemKindClass,
					Detail: "类",
				})
			}
		}
	}

	return items
}

// getStdLibDir 获取标准库目录
func (s *Server) getStdLibDir() string {
	// 获取可执行文件路径
	exePath, err := os.Executable()
	if err != nil {
		return ""
	}

	// 解析符号链接
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return ""
	}

	// 标准库在可执行文件上一级目录的 src/ 子目录
	exeDir := filepath.Dir(exePath)
	parentDir := filepath.Dir(exeDir)
	libPath := filepath.Join(parentDir, "src")

	if _, err := os.Stat(libPath); err == nil {
		return libPath
	}

	return ""
}

// getProjectNamespace 获取项目命名空间
func (s *Server) getProjectNamespace(doc *Document) string {
	projectRoot := s.getProjectRoot(doc)
	if projectRoot == "" {
		return ""
	}

	configPath := filepath.Join(projectRoot, "sola.toml")
	content, err := os.ReadFile(configPath)
	if err != nil {
		return ""
	}

	// 简单解析 namespace
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "namespace") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				ns := strings.TrimSpace(parts[1])
				ns = strings.Trim(ns, "\"")
				return ns
			}
		}
	}

	return ""
}

// getProjectRoot 获取项目根目录
func (s *Server) getProjectRoot(doc *Document) string {
	docPath := uriToPath(doc.URI)
	dir := filepath.Dir(docPath)

	// 向上查找 sola.toml
	for {
		configPath := filepath.Join(dir, "sola.toml")
		if _, err := os.Stat(configPath); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return ""
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
