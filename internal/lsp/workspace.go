package lsp

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

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
		if entryFile != "" {
			l, err := loader.New(entryFile)
			if err == nil {
				wi.loader = l
				wi.log("Loader initialized with entry: %s", entryFile)
			} else {
				wi.log("Failed to create loader: %v", err)
			}
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
		return libPath
	}

	return ""
}

// IndexStandardLibrary 索引标准库
func (wi *WorkspaceIndex) IndexStandardLibrary() {
	if wi.stdLibDir == "" {
		wi.log("No standard library directory configured")
		return
	}

	wi.log("Indexing standard library: %s", wi.stdLibDir)

	count := 0
	filepath.Walk(wi.stdLibDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && strings.HasSuffix(path, ".sola") {
			wi.indexFile(path)
			count++
		}
		return nil
	})

	wi.log("Indexed %d standard library files", count)
}

// IndexWorkspace 索引工作区
func (wi *WorkspaceIndex) IndexWorkspace() {
	if wi.workspaceRoot == "" {
		return
	}

	rootPath := wi.workspaceRoot
	if strings.HasPrefix(rootPath, "file:///") {
		rootPath = strings.TrimPrefix(rootPath, "file:///")
	}

	wi.log("Indexing workspace: %s", rootPath)

	count := 0
	filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		// 跳过隐藏目录和 node_modules 等
		if info.IsDir() {
			name := info.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".sola") {
			wi.indexFile(path)
			count++
		}
		return nil
	})

	wi.log("Indexed %d workspace files", count)
}

// indexFile 索引单个文件
func (wi *WorkspaceIndex) indexFile(path string) *IndexedFile {
	// 检查文件是否存在
	info, err := os.Stat(path)
	if err != nil {
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
	wi.mu.RUnlock()

	// 如果文件已索引且未修改，跳过
	if exists && existing.ModTime == info.ModTime().Unix() {
		return existing
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

// updateSymbolLocations 更新符号位置索引
func (wi *WorkspaceIndex) updateSymbolLocations(path string, file *ast.File) {
	wi.symbolMu.Lock()
	defer wi.symbolMu.Unlock()

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
}

// ResolveImport 解析导入路径
func (wi *WorkspaceIndex) ResolveImport(importPath string) (string, error) {
	if wi.loader != nil {
		return wi.loader.ResolveImport(importPath)
	}

	// 备用解析：标准库
	parts := strings.Split(importPath, ".")
	if len(parts) > 0 && parts[0] == "sola" && wi.stdLibDir != "" {
		libPath := filepath.Join(wi.stdLibDir, filepath.Join(parts[1:]...)+".sola")
		if _, err := os.Stat(libPath); err == nil {
			absPath, _ := filepath.Abs(libPath)
			return absPath, nil
		}
	}

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
func (wi *WorkspaceIndex) FindDefinitionInImports(currentFile *ast.File, symbolName string) (*IndexedFile, *protocol.Location) {
	if currentFile == nil {
		return nil, nil
	}

	// 遍历 use 声明
	for _, use := range currentFile.Uses {
		if use == nil {
			continue
		}

		// 解析导入路径
		importedPath, err := wi.ResolveImport(use.Path)
		if err != nil || importedPath == "" {
			continue
		}

		// 获取导入文件的索引
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
func (wi *WorkspaceIndex) GetHoverInfoForSymbol(currentFile *ast.File, symbolName string) string {
	// 首先在导入的文件中查找
	indexed, _ := wi.FindDefinitionInImports(currentFile, symbolName)
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
