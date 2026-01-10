package lsp2

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/tangzhangming/nova/internal/ast"
	"github.com/tangzhangming/nova/internal/loader"
	"github.com/tangzhangming/nova/internal/parser"
)

// ImportedFile 表示一个导入的文件
type ImportedFile struct {
	Path    string
	URI     string
	AST     *ast.File
	ModTime int64 // 文件修改时间，用于缓存失效
}

// ImportResolver 导入解析器
type ImportResolver struct {
	cache      map[string]*ImportedFile // 路径 -> 导入文件
	cacheOrder []string                 // LRU 顺序
	maxCache   int                      // 最多缓存的文件数量
	mu         sync.Mutex
	logger     *Logger
}

// NewImportResolver 创建导入解析器
func NewImportResolver(logger *Logger) *ImportResolver {
	return &ImportResolver{
		cache:      make(map[string]*ImportedFile),
		cacheOrder: make([]string, 0, 20),
		maxCache:   20, // 最多缓存20个导入文件
		logger:     logger,
	}
}

// ResolveImports 解析文档的所有导入
// 返回导入路径 -> 导入文件的映射
func (ir *ImportResolver) ResolveImports(doc *Document) map[string]*ImportedFile {
	docAST := doc.GetAST()
	if docAST == nil {
		return nil
	}

	// 创建 loader
	docPath := uriToPath(doc.URI)
	l, err := loader.New(docPath)
	if err != nil {
		ir.logger.Debug("Failed to create loader for %s: %v", docPath, err)
		return nil
	}

	imports := make(map[string]*ImportedFile)

	// 遍历所有 use 声明
	for _, use := range docAST.Uses {
		if use == nil {
			continue
		}

		importPath := use.Path
		ir.logger.Debug("Resolving import: %s", importPath)

		// 解析导入路径
		resolvedPath, err := l.ResolveImport(importPath)
		if err != nil || resolvedPath == "" {
			ir.logger.Debug("Failed to resolve import %s: %v", importPath, err)
			continue
		}

		// 获取导入的文件
		importedFile := ir.loadFile(resolvedPath)
		if importedFile != nil {
			imports[importPath] = importedFile
			ir.logger.Debug("Resolved import %s -> %s", importPath, resolvedPath)
		}
	}

	return imports
}

// FindSymbolInImports 在导入的文件中查找符号
func (ir *ImportResolver) FindSymbolInImports(doc *Document, symbolName string) *ImportedFile {
	imports := ir.ResolveImports(doc)
	if imports == nil {
		return nil
	}

	// 在所有导入的文件中查找符号
	for _, imported := range imports {
		if imported.AST == nil {
			continue
		}

		// 检查是否定义了该符号
		for _, decl := range imported.AST.Declarations {
			switch d := decl.(type) {
			case *ast.ClassDecl:
				if d.Name.Name == symbolName {
					return imported
				}
			case *ast.InterfaceDecl:
				if d.Name.Name == symbolName {
					return imported
				}
			case *ast.EnumDecl:
				if d.Name.Name == symbolName {
					return imported
				}
			}
		}
	}

	return nil
}

// loadFile 加载并解析文件（带缓存）
func (ir *ImportResolver) loadFile(path string) *ImportedFile {
	ir.mu.Lock()
	defer ir.mu.Unlock()

	// 规范化路径
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}
	absPath = filepath.Clean(absPath)

	// 检查缓存
	if cached, exists := ir.cache[absPath]; exists {
		// 检查文件是否修改
		info, err := os.Stat(absPath)
		if err == nil && info.ModTime().Unix() == cached.ModTime {
			// 缓存有效，更新 LRU
			ir.updateLRU(absPath)
			return cached
		}
		// 文件已修改，删除缓存
		delete(ir.cache, absPath)
	}

	// 检查缓存大小限制
	if len(ir.cache) >= ir.maxCache {
		ir.evictOldest()
	}

	// 检查文件是否存在
	info, err := os.Stat(absPath)
	if err != nil {
		ir.logger.Debug("File not found: %s", absPath)
		return nil
	}

	// 检查文件大小限制（500KB）
	if info.Size() > 500*1024 {
		ir.logger.Debug("File too large: %s (%d bytes)", absPath, info.Size())
		return nil
	}

	// 读取文件内容
	content, err := os.ReadFile(absPath)
	if err != nil {
		ir.logger.Debug("Failed to read file %s: %v", absPath, err)
		return nil
	}

	// 解析文件
	p := parser.New(string(content), absPath)
	astFile := p.Parse()

	// 创建导入文件对象
	imported := &ImportedFile{
		Path:    absPath,
		URI:     pathToURI(absPath),
		AST:     astFile,
		ModTime: info.ModTime().Unix(),
	}

	// 添加到缓存
	ir.cache[absPath] = imported
	ir.cacheOrder = append(ir.cacheOrder, absPath)

	ir.logger.Debug("Loaded and cached file: %s", absPath)

	return imported
}

// updateLRU 更新 LRU 顺序（内部方法，调用者需持有锁）
func (ir *ImportResolver) updateLRU(path string) {
	// 从列表中移除
	for i, p := range ir.cacheOrder {
		if p == path {
			ir.cacheOrder = append(ir.cacheOrder[:i], ir.cacheOrder[i+1:]...)
			break
		}
	}
	// 添加到末尾（最近使用）
	ir.cacheOrder = append(ir.cacheOrder, path)
}

// evictOldest 淘汰最旧的缓存（内部方法，调用者需持有锁）
func (ir *ImportResolver) evictOldest() {
	if len(ir.cacheOrder) == 0 {
		return
	}

	// 淘汰最旧的文件（列表开头）
	oldestPath := ir.cacheOrder[0]
	imported := ir.cache[oldestPath]

	// 删除缓存
	delete(ir.cache, oldestPath)
	ir.cacheOrder = ir.cacheOrder[1:]

	// 清理 AST
	if imported != nil {
		imported.AST = nil
	}

	ir.logger.Debug("Evicted oldest import cache: %s", oldestPath)
}

// ClearCache 清空缓存
func (ir *ImportResolver) ClearCache() {
	ir.mu.Lock()
	defer ir.mu.Unlock()

	// 清理所有 AST
	for _, imported := range ir.cache {
		if imported != nil {
			imported.AST = nil
		}
	}

	ir.cache = make(map[string]*ImportedFile)
	ir.cacheOrder = make([]string, 0, 20)

	ir.logger.Info("Import cache cleared")
}

// CacheSize 返回当前缓存大小
func (ir *ImportResolver) CacheSize() int {
	ir.mu.Lock()
	defer ir.mu.Unlock()
	return len(ir.cache)
}

// GetStdLibPath 获取标准库路径
func GetStdLibPath() string {
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

// ResolveStdLibImport 解析标准库导入
// 例如: sola.lang.Str -> /path/to/stdlib/lang/Str.sola
func ResolveStdLibImport(importPath string) string {
	stdLibPath := GetStdLibPath()
	if stdLibPath == "" {
		return ""
	}

	// 解析导入路径
	// sola.lang.Str -> lang/Str.sola
	parts := strings.Split(importPath, ".")
	if len(parts) < 2 || parts[0] != "sola" {
		return ""
	}

	// 去掉 "sola" 前缀
	parts = parts[1:]

	// 构建文件路径
	filePath := filepath.Join(stdLibPath, filepath.Join(parts...)+".sola")

	if _, err := os.Stat(filePath); err == nil {
		return filePath
	}

	return ""
}
