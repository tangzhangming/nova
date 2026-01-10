package lsp

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/tangzhangming/nova/internal/ast"
	"github.com/tangzhangming/nova/internal/compiler"
	"github.com/tangzhangming/nova/internal/loader"
	"github.com/tangzhangming/nova/internal/parser"
	"go.lsp.dev/protocol"
)

// WorkspaceIndex 工作区索引，管理所有文件的符号信息
type WorkspaceIndex struct {
	// 加载器
	loader *loader.Loader

	// 标准库目录
	stdLibDir string

	// 工作区根目录
	workspaceRoot string

	// 已索引的文件 (path -> IndexedFile)
	files map[string]*IndexedFile
	mu    sync.RWMutex

	// 全局符号表（类名/接口名/函数名 -> 文件路径）
	symbolLocations map[string]string
	symbolMu        sync.RWMutex

	// 日志函数
	log func(format string, args ...interface{})
	
	// 内存管理
	maxIndexedFiles int  // 最大索引文件数量
	indexingEnabled bool // 是否启用索引
}

// IndexedFile 已索引的文件信息
type IndexedFile struct {
	Path      string
	URI       string
	AST       *ast.File
	Symbols   *compiler.SymbolTable
	ParseErrs []parser.Error
	ModTime   int64 // 文件修改时间
}

// NewWorkspaceIndex 创建工作区索引
func NewWorkspaceIndex(workspaceRoot string, logFunc func(format string, args ...interface{})) *WorkspaceIndex {
	wi := &WorkspaceIndex{
		workspaceRoot:   workspaceRoot,
		files:           make(map[string]*IndexedFile),
		symbolLocations: make(map[string]string),
		log:             logFunc,
		maxIndexedFiles: 50, // 允许索引50个文件（按需索引）
		indexingEnabled: false, // 禁用自动扫描工作区
	}

	// 尝试创建 loader
	if workspaceRoot != "" {
		// 从 URI 转换为文件路径
		rootPath := workspaceRoot
		if strings.HasPrefix(rootPath, "file:///") {
			rootPath = strings.TrimPrefix(rootPath, "file:///")
			// Windows 路径处理
			if len(rootPath) > 2 && rootPath[1] == ':' {
				// 已经是正确的 Windows 路径格式
			} else if len(rootPath) > 0 && rootPath[0] != '/' {
				// Windows 路径，但缺少盘符后的内容
			}
		}

		// 查找一个入口文件来初始化 loader
		entryFile := wi.findEntryFile(rootPath)
		// #region agent log
		// 假设B1: 记录loader初始化
		appendLog := func() {
			f, _ := os.OpenFile("d:\\workspace\\go\\src\\nova\\.cursor\\debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if f != nil { defer f.Close(); fmt.Fprintf(f, "{\"sessionId\":\"debug-session\",\"hypothesisId\":\"B1\",\"location\":\"workspace.go:80\",\"message\":\"loader initialization attempt\",\"data\":{\"workspaceRoot\":\"%s\",\"rootPath\":\"%s\",\"entryFile\":\"%s\"},\"timestamp\":%d}\n", workspaceRoot, rootPath, entryFile, time.Now().UnixMilli()) }
		}
		appendLog()
		// #endregion
		if entryFile != "" {
			l, err := loader.New(entryFile)
			if err == nil {
				wi.loader = l
				// #region agent log
				// 假设B1: 记录loader初始化成功，包括namespace
				appendLog2 := func() {
					f, _ := os.OpenFile("d:\\workspace\\go\\src\\nova\\.cursor\\debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
					namespace := l.GetProjectNamespace()
					rootDir := l.RootDir()
					if f != nil { defer f.Close(); fmt.Fprintf(f, "{\"sessionId\":\"debug-session\",\"hypothesisId\":\"B1\",\"location\":\"workspace.go:88\",\"message\":\"loader initialized successfully\",\"data\":{\"entryFile\":\"%s\",\"namespace\":\"%s\",\"rootDir\":\"%s\"},\"timestamp\":%d}\n", entryFile, namespace, rootDir, time.Now().UnixMilli()) }
				}
				appendLog2()
				// #endregion
				wi.log("Loader initialized with entry: %s", entryFile)
			} else {
				// #region agent log
				// 假设B1: 记录loader初始化失败
				appendLog3 := func() {
					f, _ := os.OpenFile("d:\\workspace\\go\\src\\nova\\.cursor\\debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
					if f != nil { defer f.Close(); fmt.Fprintf(f, "{\"sessionId\":\"debug-session\",\"hypothesisId\":\"B1\",\"location\":\"workspace.go:97\",\"message\":\"loader initialization failed\",\"data\":{\"entryFile\":\"%s\",\"error\":\"%s\"},\"timestamp\":%d}\n", entryFile, err.Error(), time.Now().UnixMilli()) }
				}
				appendLog3()
				// #endregion
				wi.log("Failed to create loader: %v", err)
			}
		} else {
			// #region agent log
			// 假设B1: 记录找不到入口文件
			appendLog4 := func() {
				f, _ := os.OpenFile("d:\\workspace\\go\\src\\nova\\.cursor\\debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
				if f != nil { defer f.Close(); fmt.Fprintf(f, "{\"sessionId\":\"debug-session\",\"hypothesisId\":\"B1\",\"location\":\"workspace.go:107\",\"message\":\"no entry file found\",\"data\":{\"rootPath\":\"%s\"},\"timestamp\":%d}\n", rootPath, time.Now().UnixMilli()) }
			}
			appendLog4()
			// #endregion
		}
	}

	// 获取标准库路径
	wi.stdLibDir = wi.getStdLibPath()
	if wi.stdLibDir != "" {
		wi.log("Standard library found at: %s", wi.stdLibDir)
	}

	return wi
}

// findEntryFile 查找入口文件
func (wi *WorkspaceIndex) findEntryFile(rootPath string) string {
	// 优先查找 main.sola
	mainFile := filepath.Join(rootPath, "main.sola")
	if _, err := os.Stat(mainFile); err == nil {
		return mainFile
	}

	// 查找 src/main.sola
	srcMain := filepath.Join(rootPath, "src", "main.sola")
	if _, err := os.Stat(srcMain); err == nil {
		return srcMain
	}

	// 查找任意 .sola 文件
	var firstSola string
	filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && strings.HasSuffix(path, ".sola") {
			firstSola = path
			return filepath.SkipAll
		}
		return nil
	})

	return firstSola
}

// getStdLibPath 获取标准库路径
func (wi *WorkspaceIndex) getStdLibPath() string {
	// 获取可执行文件路径
	exePath, err := os.Executable()
	if err != nil {
		return ""
	}

	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return ""
	}

	// 标准库在可执行文件上一级目录的 src/ 子目录
	exeDir := filepath.Dir(exePath)
	parentDir := filepath.Dir(exeDir)
	libPath := filepath.Join(parentDir, "src")

	if _, err := os.Stat(libPath); err == nil {
		// #region agent log
		// 假设B3: 记录标准库路径查找结果
		appendLog := func() {
			f, _ := os.OpenFile("d:\\workspace\\go\\src\\nova\\.cursor\\debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if f != nil { defer f.Close(); fmt.Fprintf(f, "{\"sessionId\":\"debug-session\",\"hypothesisId\":\"B3\",\"location\":\"workspace.go:149\",\"message\":\"stdlib path found\",\"data\":{\"exePath\":\"%s\",\"libPath\":\"%s\"},\"timestamp\":%d}\n", exePath, libPath, time.Now().UnixMilli()) }
		}
		appendLog()
		// #endregion
		return libPath
	}
	
	// #region agent log
	// 假设B3: 记录标准库路径查找失败
	appendLog2 := func() {
		f, _ := os.OpenFile("d:\\workspace\\go\\src\\nova\\.cursor\\debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if f != nil { defer f.Close(); fmt.Fprintf(f, "{\"sessionId\":\"debug-session\",\"hypothesisId\":\"B3\",\"location\":\"workspace.go:159\",\"message\":\"stdlib path not found\",\"data\":{\"exePath\":\"%s\",\"attemptedLibPath\":\"%s\"},\"timestamp\":%d}\n", exePath, libPath, time.Now().UnixMilli()) }
	}
	appendLog2()
	// #endregion

	return ""
}

// IndexStandardLibrary 索引标准库
func (wi *WorkspaceIndex) IndexStandardLibrary() {
	// 禁用标准库全量索引，改为按需加载，避免内存暴涨
	if wi.stdLibDir == "" {
		wi.log("No standard library directory configured")
		return
	}
	wi.log("Standard library lazy loading enabled: %s", wi.stdLibDir)
}

// IndexWorkspace 索引工作区
func (wi *WorkspaceIndex) IndexWorkspace() {
	// 紧急：完全禁用工作区索引，只索引打开的文件
	wi.log("Workspace indexing DISABLED to prevent memory leak")
	return
}

// indexFile 索引单个文件
func (wi *WorkspaceIndex) indexFile(path string) *IndexedFile {
	// 检查文件是否存在
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}

	// 检查文件大小限制（不索引超过 500KB 的文件）
	if info.Size() > 500*1024 {
		return nil
	}

	// 规范化路径
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}
	absPath = filepath.Clean(absPath)

	wi.mu.RLock()
	existing, exists := wi.files[absPath]
	fileCount := len(wi.files)
	wi.mu.RUnlock()

	// 如果文件已索引且未修改，跳过
	if exists && existing.ModTime == info.ModTime().Unix() {
		return existing
	}

	// 检查索引文件数量限制
	if !exists && fileCount >= wi.maxIndexedFiles {
		wi.log("Max indexed files limit reached (%d), skipping: %s", wi.maxIndexedFiles, absPath)
		return nil
	}

	// 读取文件内容
	content, err := os.ReadFile(absPath)
	if err != nil {
		return nil
	}

	// 解析文件
	p := parser.New(string(content), absPath)
	astFile := p.Parse()
	parseErrs := p.Errors()

	// 收集符号
	var symbols *compiler.SymbolTable
	if astFile != nil {
		symbols = compiler.NewSymbolTable()
		symbols.CollectFromFile(astFile)
	}

	// 创建 URI
	uri := "file:///" + strings.ReplaceAll(absPath, "\\", "/")

	indexed := &IndexedFile{
		Path:      absPath,
		URI:       uri,
		AST:       astFile,
		Symbols:   symbols,
		ParseErrs: parseErrs,
		ModTime:   info.ModTime().Unix(),
	}

	// 存储索引
	wi.mu.Lock()
	wi.files[absPath] = indexed
	wi.mu.Unlock()

	// 更新符号位置索引
	if astFile != nil {
		wi.updateSymbolLocations(absPath, astFile)
	}

	return indexed
}

// ClearIndex 清理索引缓存
func (wi *WorkspaceIndex) ClearIndex() {
	wi.mu.Lock()
	wi.files = make(map[string]*IndexedFile)
	wi.mu.Unlock()
	
	wi.symbolMu.Lock()
	wi.symbolLocations = make(map[string]string)
	wi.symbolMu.Unlock()
	
	wi.log("Workspace index cleared")
}

// RemoveFile 从索引中移除文件
func (wi *WorkspaceIndex) RemoveFile(path string) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}
	absPath = filepath.Clean(absPath)
	
	wi.mu.Lock()
	delete(wi.files, absPath)
	wi.mu.Unlock()
}

// SetIndexingEnabled 设置是否启用索引
func (wi *WorkspaceIndex) SetIndexingEnabled(enabled bool) {
	wi.indexingEnabled = enabled
}

// updateSymbolLocations 更新符号位置索引
func (wi *WorkspaceIndex) updateSymbolLocations(path string, file *ast.File) {
	wi.symbolMu.Lock()
	defer wi.symbolMu.Unlock()
	
	oldCount := len(wi.symbolLocations)

	for _, decl := range file.Declarations {
		switch d := decl.(type) {
		case *ast.ClassDecl:
			wi.symbolLocations[d.Name.Name] = path
		case *ast.InterfaceDecl:
			wi.symbolLocations[d.Name.Name] = path
		case *ast.EnumDecl:
			wi.symbolLocations[d.Name.Name] = path
		case *ast.TypeAliasDecl:
			wi.symbolLocations[d.Name.Name] = path
		case *ast.NewTypeDecl:
			wi.symbolLocations[d.Name.Name] = path
		}
	}
	
	newCount := len(wi.symbolLocations)
	// #region agent log
	// 假设A3: 记录symbolLocations映射增长
	appendLog := func() {
		f, _ := os.OpenFile("d:\\workspace\\go\\src\\nova\\.cursor\\debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if f != nil { defer f.Close(); fmt.Fprintf(f, "{\"sessionId\":\"debug-session\",\"hypothesisId\":\"A3\",\"location\":\"workspace.go:336\",\"message\":\"symbolLocations updated\",\"data\":{\"path\":\"%s\",\"oldCount\":%d,\"newCount\":%d},\"timestamp\":%d}\n", path, oldCount, newCount, time.Now().UnixMilli()) }
	}
	appendLog()
	// #endregion
}

// ResolveImport 解析导入路径
func (wi *WorkspaceIndex) ResolveImport(importPath string) (string, error) {
	var resolvedPath string
	var err error
	
	if wi.loader != nil {
		resolvedPath, err = wi.loader.ResolveImport(importPath)
		// #region agent log
		// 假设B1: 记录loader解析导入路径的结果
		appendLog := func() {
			f, _ := os.OpenFile("d:\\workspace\\go\\src\\nova\\.cursor\\debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			errStr := "nil"
			if err != nil { errStr = err.Error() }
			if f != nil { defer f.Close(); fmt.Fprintf(f, "{\"sessionId\":\"debug-session\",\"hypothesisId\":\"B1\",\"location\":\"workspace.go:343\",\"message\":\"loader ResolveImport\",\"data\":{\"importPath\":\"%s\",\"resolvedPath\":\"%s\",\"error\":\"%s\"},\"timestamp\":%d}\n", importPath, resolvedPath, errStr, time.Now().UnixMilli()) }
		}
		appendLog()
		// #endregion
		return resolvedPath, err
	}

	// 备用解析：标准库
	parts := strings.Split(importPath, ".")
	if len(parts) > 0 && parts[0] == "sola" && wi.stdLibDir != "" {
		libPath := filepath.Join(wi.stdLibDir, filepath.Join(parts[1:]...)+".sola")
		if _, err := os.Stat(libPath); err == nil {
			absPath, _ := filepath.Abs(libPath)
			// #region agent log
			// 假设B1, B3: 记录标准库解析成功
			appendLog2 := func() {
				f, _ := os.OpenFile("d:\\workspace\\go\\src\\nova\\.cursor\\debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
				if f != nil { defer f.Close(); fmt.Fprintf(f, "{\"sessionId\":\"debug-session\",\"hypothesisId\":\"B1_B3\",\"location\":\"workspace.go:362\",\"message\":\"stdlib resolved\",\"data\":{\"importPath\":\"%s\",\"libPath\":\"%s\",\"absPath\":\"%s\",\"stdLibDir\":\"%s\"},\"timestamp\":%d}\n", importPath, libPath, absPath, wi.stdLibDir, time.Now().UnixMilli()) }
			}
			appendLog2()
			// #endregion
			return absPath, nil
		}
	}
	
	// #region agent log
	// 假设B1, B3: 记录解析失败
	appendLog3 := func() {
		f, _ := os.OpenFile("d:\\workspace\\go\\src\\nova\\.cursor\\debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if f != nil { defer f.Close(); fmt.Fprintf(f, "{\"sessionId\":\"debug-session\",\"hypothesisId\":\"B1_B3\",\"location\":\"workspace.go:374\",\"message\":\"import resolve failed\",\"data\":{\"importPath\":\"%s\",\"stdLibDir\":\"%s\",\"hasLoader\":%v},\"timestamp\":%d}\n", importPath, wi.stdLibDir, wi.loader != nil, time.Now().UnixMilli()) }
	}
	appendLog3()
	// #endregion

	return "", nil
}

// GetIndexedFile 获取已索引的文件
func (wi *WorkspaceIndex) GetIndexedFile(path string) *IndexedFile {
	// 规范化路径
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}
	absPath = filepath.Clean(absPath)

	wi.mu.RLock()
	indexed := wi.files[absPath]
	wi.mu.RUnlock()

	if indexed != nil {
		return indexed
	}

	// 尝试索引文件
	return wi.indexFile(path)
}

// GetFileByURI 通过 URI 获取已索引的文件
func (wi *WorkspaceIndex) GetFileByURI(uri string) *IndexedFile {
	path := uriToPath(uri)
	return wi.GetIndexedFile(path)
}

// FindSymbolFile 查找符号所在的文件
func (wi *WorkspaceIndex) FindSymbolFile(symbolName string) *IndexedFile {
	wi.symbolMu.RLock()
	path, exists := wi.symbolLocations[symbolName]
	wi.symbolMu.RUnlock()

	if !exists {
		return nil
	}

	return wi.GetIndexedFile(path)
}

// FindDefinitionInImports 在导入的文件中查找定义
func (wi *WorkspaceIndex) FindDefinitionInImports(currentFile *ast.File, symbolName string, docLoader interface{}) (*IndexedFile, *protocol.Location) {
	if currentFile == nil {
		return nil, nil
	}
	
	// 优先使用文档专属的loader
	var loaderToUse interface {
		ResolveImport(importPath string) (string, error)
	}
	
	if docLoader != nil {
		if l, ok := docLoader.(interface {
			ResolveImport(importPath string) (string, error)
		}); ok {
			loaderToUse = l
		}
	}
	
	if loaderToUse == nil && wi.loader != nil {
		loaderToUse = wi.loader
	}

	// 遍历 use 声明
	for _, use := range currentFile.Uses {
		if use == nil {
			continue
		}

		// 解析导入路径 - 使用选定的loader
		var importedPath string
		var err error
		if loaderToUse != nil {
			importedPath, err = loaderToUse.ResolveImport(use.Path)
		} else {
			importedPath, err = wi.ResolveImport(use.Path)
		}
		
		if err != nil || importedPath == "" {
			continue
		}

		// 获取导入文件的索引（会按需索引）
		indexed := wi.GetIndexedFile(importedPath)
		if indexed == nil || indexed.AST == nil {
			continue
		}

		// 在导入文件中查找符号
		if loc := wi.findSymbolInFile(indexed, symbolName); loc != nil {
			return indexed, loc
		}
	}

	return nil, nil
}

// findSymbolInFile 在文件中查找符号定义
func (wi *WorkspaceIndex) findSymbolInFile(indexed *IndexedFile, name string) *protocol.Location {
	if indexed == nil || indexed.AST == nil {
		return nil
	}

	for _, decl := range indexed.AST.Declarations {
		switch d := decl.(type) {
		case *ast.ClassDecl:
			if d.Name.Name == name {
				return &protocol.Location{
					URI: protocol.DocumentURI(indexed.URI),
					Range: protocol.Range{
						Start: protocol.Position{
							Line:      uint32(d.Name.Token.Pos.Line - 1),
							Character: uint32(d.Name.Token.Pos.Column - 1),
						},
						End: protocol.Position{
							Line:      uint32(d.Name.Token.Pos.Line - 1),
							Character: uint32(d.Name.Token.Pos.Column - 1 + len(name)),
						},
					},
				}
			}
			// 检查方法
			for _, method := range d.Methods {
				if method.Name.Name == name {
					return &protocol.Location{
						URI: protocol.DocumentURI(indexed.URI),
						Range: protocol.Range{
							Start: protocol.Position{
								Line:      uint32(method.Name.Token.Pos.Line - 1),
								Character: uint32(method.Name.Token.Pos.Column - 1),
							},
							End: protocol.Position{
								Line:      uint32(method.Name.Token.Pos.Line - 1),
								Character: uint32(method.Name.Token.Pos.Column - 1 + len(name)),
							},
						},
					}
				}
			}
		case *ast.InterfaceDecl:
			if d.Name.Name == name {
				return &protocol.Location{
					URI: protocol.DocumentURI(indexed.URI),
					Range: protocol.Range{
						Start: protocol.Position{
							Line:      uint32(d.Name.Token.Pos.Line - 1),
							Character: uint32(d.Name.Token.Pos.Column - 1),
						},
						End: protocol.Position{
							Line:      uint32(d.Name.Token.Pos.Line - 1),
							Character: uint32(d.Name.Token.Pos.Column - 1 + len(name)),
						},
					},
				}
			}
		case *ast.EnumDecl:
			if d.Name.Name == name {
				return &protocol.Location{
					URI: protocol.DocumentURI(indexed.URI),
					Range: protocol.Range{
						Start: protocol.Position{
							Line:      uint32(d.Name.Token.Pos.Line - 1),
							Character: uint32(d.Name.Token.Pos.Column - 1),
						},
						End: protocol.Position{
							Line:      uint32(d.Name.Token.Pos.Line - 1),
							Character: uint32(d.Name.Token.Pos.Column - 1 + len(name)),
						},
					},
				}
			}
		}
	}

	return nil
}

// GetHoverInfoForSymbol 获取符号的悬停信息
func (wi *WorkspaceIndex) GetHoverInfoForSymbol(currentFile *ast.File, symbolName string, docLoader interface{}) string {
	// 首先在导入的文件中查找
	indexed, _ := wi.FindDefinitionInImports(currentFile, symbolName, docLoader)
	if indexed != nil && indexed.AST != nil {
		for _, decl := range indexed.AST.Declarations {
			switch d := decl.(type) {
			case *ast.ClassDecl:
				if d.Name.Name == symbolName {
					return formatClassDeclHover(d)
				}
			case *ast.InterfaceDecl:
				if d.Name.Name == symbolName {
					return formatInterfaceDeclHover(d)
				}
			case *ast.EnumDecl:
				if d.Name.Name == symbolName {
					return formatEnumDeclHover(d)
				}
			}
		}
	}

	// 在全局符号索引中查找
	symbolFile := wi.FindSymbolFile(symbolName)
	if symbolFile != nil && symbolFile.AST != nil {
		for _, decl := range symbolFile.AST.Declarations {
			switch d := decl.(type) {
			case *ast.ClassDecl:
				if d.Name.Name == symbolName {
					return formatClassDeclHover(d)
				}
			case *ast.InterfaceDecl:
				if d.Name.Name == symbolName {
					return formatInterfaceDeclHover(d)
				}
			case *ast.EnumDecl:
				if d.Name.Name == symbolName {
					return formatEnumDeclHover(d)
				}
			}
		}
	}

	return ""
}

// GetAllFiles 获取所有已索引的文件
func (wi *WorkspaceIndex) GetAllFiles() []*IndexedFile {
	wi.mu.RLock()
	defer wi.mu.RUnlock()

	files := make([]*IndexedFile, 0, len(wi.files))
	for _, f := range wi.files {
		files = append(files, f)
	}
	return files
}

// InitializeDocumentLoader 为文档初始化专属的loader
func (wi *WorkspaceIndex) InitializeDocumentLoader(doc interface{}) {
	// 类型断言获取Document
	type DocumentInterface interface {
		URI() string
	}
	
	// 获取文档路径
	var docPath string
	if d, ok := doc.(interface{ GetURI() string }); ok {
		docPath = uriToPath(d.GetURI())
	} else {
		// 使用反射获取URI字段
		docVal := reflect.ValueOf(doc)
		if docVal.Kind() == reflect.Ptr {
			docVal = docVal.Elem()
		}
		if docVal.Kind() == reflect.Struct {
			uriField := docVal.FieldByName("URI")
			if uriField.IsValid() && uriField.Kind() == reflect.String {
				docPath = uriToPath(uriField.String())
			}
		}
	}
	
	if docPath == "" {
		return
	}
	
	// #region agent log
	// 假设B1: 记录为文档创建loader
	appendLog := func() {
		f, _ := os.OpenFile("d:\\workspace\\go\\src\\nova\\.cursor\\debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if f != nil { defer f.Close(); fmt.Fprintf(f, "{\"sessionId\":\"debug-session\",\"hypothesisId\":\"B1\",\"location\":\"workspace.go:580\",\"message\":\"InitializeDocumentLoader called\",\"data\":{\"docPath\":\"%s\"},\"timestamp\":%d}\n", docPath, time.Now().UnixMilli()) }
	}
	appendLog()
	// #endregion
	
	// 为该文档创建loader
	l, err := loader.New(docPath)
	if err != nil {
		// #region agent log
		appendLog2 := func() {
			f, _ := os.OpenFile("d:\\workspace\\go\\src\\nova\\.cursor\\debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if f != nil { defer f.Close(); fmt.Fprintf(f, "{\"sessionId\":\"debug-session\",\"hypothesisId\":\"B1\",\"location\":\"workspace.go:592\",\"message\":\"document loader creation failed\",\"data\":{\"docPath\":\"%s\",\"error\":\"%s\"},\"timestamp\":%d}\n", docPath, err.Error(), time.Now().UnixMilli()) }
		}
		appendLog2()
		// #endregion
		wi.log("Failed to create loader for document %s: %v", docPath, err)
		return
	}
	
	// 设置文档的Loader字段
	docVal := reflect.ValueOf(doc)
	if docVal.Kind() == reflect.Ptr {
		docVal = docVal.Elem()
	}
	if docVal.Kind() == reflect.Struct {
		loaderField := docVal.FieldByName("Loader")
		if loaderField.IsValid() && loaderField.CanSet() {
			loaderField.Set(reflect.ValueOf(l))
			
			// #region agent log
			appendLog3 := func() {
				f, _ := os.OpenFile("d:\\workspace\\go\\src\\nova\\.cursor\\debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
				namespace := l.GetProjectNamespace()
				rootDir := l.RootDir()
				if f != nil { defer f.Close(); fmt.Fprintf(f, "{\"sessionId\":\"debug-session\",\"hypothesisId\":\"B1\",\"location\":\"workspace.go:613\",\"message\":\"document loader set successfully\",\"data\":{\"docPath\":\"%s\",\"namespace\":\"%s\",\"rootDir\":\"%s\"},\"timestamp\":%d}\n", docPath, namespace, rootDir, time.Now().UnixMilli()) }
			}
			appendLog3()
			// #endregion
			
			wi.log("Loader initialized for document: %s (namespace: %s, rootDir: %s)", docPath, l.GetProjectNamespace(), l.RootDir())
		}
	}
}
