// incremental.go - 增量编译
//
// 实现智能增量编译，只重新编译变更的文件及其依赖者。
//
// 功能：
// 1. 依赖图构建和管理
// 2. 变更检测
// 3. 智能重编译决策
// 4. 编译顺序拓扑排序

package compiler

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/tangzhangming/nova/internal/ast"
	"github.com/tangzhangming/nova/internal/bytecode"
	"github.com/tangzhangming/nova/internal/parser"
)

// IncrementalCompiler 增量编译器
type IncrementalCompiler struct {
	mu sync.RWMutex
	
	// 缓存管理器
	cache *CacheManager
	
	// 依赖图
	depGraph *DependencyGraph
	
	// 文件状态
	fileStates map[string]*FileState
	
	// 工作目录
	workDir string
	
	// 统计信息
	stats IncrementalStats
}

// FileState 文件状态
type FileState struct {
	Path         string
	Hash         string
	ModTime      time.Time
	Dependencies []string
	Dependents   []string
	NeedsCompile bool
}

// IncrementalStats 增量编译统计
type IncrementalStats struct {
	TotalFiles      int
	CacheHits       int
	CacheMisses     int
	Recompiled      int
	SkippedUnchanged int
	TotalTime       time.Duration
}

// DependencyGraph 依赖图
type DependencyGraph struct {
	mu    sync.RWMutex
	nodes map[string]*DepNode
}

// DepNode 依赖节点
type DepNode struct {
	Path       string
	Imports    []string // 该文件导入的模块
	ImportedBy []string // 导入该文件的模块
}

// NewIncrementalCompiler 创建增量编译器
func NewIncrementalCompiler(workDir string) (*IncrementalCompiler, error) {
	cache, err := NewCacheManager(workDir)
	if err != nil {
		return nil, err
	}
	
	return &IncrementalCompiler{
		cache:      cache,
		depGraph:   NewDependencyGraph(),
		fileStates: make(map[string]*FileState),
		workDir:    workDir,
	}, nil
}

// NewDependencyGraph 创建依赖图
func NewDependencyGraph() *DependencyGraph {
	return &DependencyGraph{
		nodes: make(map[string]*DepNode),
	}
}

// Compile 增量编译文件
func (ic *IncrementalCompiler) Compile(sourcePath string) (*bytecode.CompiledFile, error) {
	startTime := time.Now()
	defer func() {
		ic.stats.TotalTime = time.Since(startTime)
	}()
	
	ic.stats = IncrementalStats{}
	
	// 解析源文件获取依赖
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read source file: %w", err)
	}
	
	// 分析依赖
	deps, err := ic.analyzeDependencies(sourcePath, string(source))
	if err != nil {
		return nil, err
	}
	
	// 更新依赖图
	ic.updateDependencyGraph(sourcePath, deps)
	
	// 确定需要重新编译的文件
	needsCompile := ic.determineRecompileSet(sourcePath)
	
	// 按拓扑顺序编译
	compiledFiles := make(map[string]*bytecode.CompiledFile)
	order := ic.topologicalSort(needsCompile)
	
	for _, path := range order {
		cf, err := ic.compileFile(path, compiledFiles)
		if err != nil {
			return nil, fmt.Errorf("failed to compile %s: %w", path, err)
		}
		compiledFiles[path] = cf
	}
	
	// 返回主文件的编译结果
	return compiledFiles[sourcePath], nil
}

// CompileChanged 编译所有变更的文件
func (ic *IncrementalCompiler) CompileChanged() (map[string]*bytecode.CompiledFile, error) {
	ic.mu.Lock()
	defer ic.mu.Unlock()
	
	// 检测变更
	changed := ic.detectChanges()
	if len(changed) == 0 {
		return nil, nil
	}
	
	// 确定受影响的文件
	affected := make(map[string]bool)
	for _, path := range changed {
		affected[path] = true
		dependents := ic.getDependents(path)
		for _, dep := range dependents {
			affected[dep] = true
		}
	}
	
	// 按拓扑顺序编译
	needsCompile := make([]string, 0, len(affected))
	for path := range affected {
		needsCompile = append(needsCompile, path)
	}
	
	compiledFiles := make(map[string]*bytecode.CompiledFile)
	order := ic.topologicalSort(needsCompile)
	
	for _, path := range order {
		cf, err := ic.compileFile(path, compiledFiles)
		if err != nil {
			return nil, fmt.Errorf("failed to compile %s: %w", path, err)
		}
		compiledFiles[path] = cf
		ic.stats.Recompiled++
	}
	
	return compiledFiles, nil
}

// GetStats 获取统计信息
func (ic *IncrementalCompiler) GetStats() IncrementalStats {
	ic.mu.RLock()
	defer ic.mu.RUnlock()
	return ic.stats
}

// ClearCache 清除缓存
func (ic *IncrementalCompiler) ClearCache() error {
	return ic.cache.Clear()
}

// ============================================================================
// 内部方法
// ============================================================================

// analyzeDependencies 分析文件依赖
func (ic *IncrementalCompiler) analyzeDependencies(sourcePath, source string) ([]string, error) {
	// 解析文件
	p := parser.New(source, sourcePath)
	file := p.Parse()
	
	if p.HasErrors() {
		return nil, fmt.Errorf("parse errors in %s", sourcePath)
	}
	
	// 收集导入
	deps := make([]string, 0)
	for _, use := range file.Uses {
		// 解析导入路径到文件路径
		depPath := ic.resolveImportPath(use.Path, sourcePath)
		if depPath != "" {
			deps = append(deps, depPath)
		}
	}
	
	return deps, nil
}

// resolveImportPath 解析导入路径
func (ic *IncrementalCompiler) resolveImportPath(importPath, fromFile string) string {
	// 简单实现：相对于工作目录
	baseDir := filepath.Dir(fromFile)
	
	// 尝试几种可能的路径
	candidates := []string{
		filepath.Join(baseDir, importPath+".sola"),
		filepath.Join(ic.workDir, importPath+".sola"),
		filepath.Join(ic.workDir, "lib", importPath+".sola"),
	}
	
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	
	return ""
}

// updateDependencyGraph 更新依赖图
func (ic *IncrementalCompiler) updateDependencyGraph(sourcePath string, deps []string) {
	ic.depGraph.mu.Lock()
	defer ic.depGraph.mu.Unlock()
	
	// 清除旧的依赖关系
	if oldNode, ok := ic.depGraph.nodes[sourcePath]; ok {
		for _, imp := range oldNode.Imports {
			if importedNode, ok := ic.depGraph.nodes[imp]; ok {
				// 从被导入节点的 ImportedBy 中移除
				newImportedBy := make([]string, 0)
				for _, by := range importedNode.ImportedBy {
					if by != sourcePath {
						newImportedBy = append(newImportedBy, by)
					}
				}
				importedNode.ImportedBy = newImportedBy
			}
		}
	}
	
	// 创建或更新节点
	node := &DepNode{
		Path:       sourcePath,
		Imports:    deps,
		ImportedBy: []string{},
	}
	
	// 保留旧的 ImportedBy
	if oldNode, ok := ic.depGraph.nodes[sourcePath]; ok {
		node.ImportedBy = oldNode.ImportedBy
	}
	
	ic.depGraph.nodes[sourcePath] = node
	
	// 更新被导入节点的 ImportedBy
	for _, dep := range deps {
		depNode, ok := ic.depGraph.nodes[dep]
		if !ok {
			depNode = &DepNode{
				Path:       dep,
				Imports:    []string{},
				ImportedBy: []string{},
			}
			ic.depGraph.nodes[dep] = depNode
		}
		depNode.ImportedBy = append(depNode.ImportedBy, sourcePath)
	}
}

// determineRecompileSet 确定需要重新编译的文件集
func (ic *IncrementalCompiler) determineRecompileSet(sourcePath string) []string {
	needsCompile := make(map[string]bool)
	needsCompile[sourcePath] = true
	
	// 检查缓存
	if _, ok := ic.cache.Get(sourcePath); !ok {
		ic.stats.CacheMisses++
	} else {
		ic.stats.CacheHits++
	}
	
	// 收集所有依赖（递归）
	visited := make(map[string]bool)
	var collectDeps func(path string)
	collectDeps = func(path string) {
		if visited[path] {
			return
		}
		visited[path] = true
		
		ic.depGraph.mu.RLock()
		node, ok := ic.depGraph.nodes[path]
		ic.depGraph.mu.RUnlock()
		
		if !ok {
			return
		}
		
		for _, dep := range node.Imports {
			needsCompile[dep] = true
			collectDeps(dep)
		}
	}
	collectDeps(sourcePath)
	
	// 转换为列表
	result := make([]string, 0, len(needsCompile))
	for path := range needsCompile {
		result = append(result, path)
	}
	
	ic.stats.TotalFiles = len(result)
	return result
}

// topologicalSort 拓扑排序
func (ic *IncrementalCompiler) topologicalSort(files []string) []string {
	ic.depGraph.mu.RLock()
	defer ic.depGraph.mu.RUnlock()
	
	// 构建入度表
	inDegree := make(map[string]int)
	fileSet := make(map[string]bool)
	for _, f := range files {
		fileSet[f] = true
		inDegree[f] = 0
	}
	
	// 计算入度（只考虑在编译集中的依赖）
	for _, f := range files {
		node, ok := ic.depGraph.nodes[f]
		if !ok {
			continue
		}
		for _, dep := range node.Imports {
			if fileSet[dep] {
				inDegree[f]++
			}
		}
	}
	
	// Kahn 算法
	queue := make([]string, 0)
	for f, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, f)
		}
	}
	
	result := make([]string, 0, len(files))
	for len(queue) > 0 {
		// 弹出
		f := queue[0]
		queue = queue[1:]
		result = append(result, f)
		
		// 减少被依赖者的入度
		node, ok := ic.depGraph.nodes[f]
		if !ok {
			continue
		}
		for _, dependent := range node.ImportedBy {
			if !fileSet[dependent] {
				continue
			}
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				queue = append(queue, dependent)
			}
		}
	}
	
	// 如果有循环依赖，添加剩余文件
	if len(result) < len(files) {
		for _, f := range files {
			found := false
			for _, r := range result {
				if r == f {
					found = true
					break
				}
			}
			if !found {
				result = append(result, f)
			}
		}
	}
	
	return result
}

// compileFile 编译单个文件
func (ic *IncrementalCompiler) compileFile(path string, compiled map[string]*bytecode.CompiledFile) (*bytecode.CompiledFile, error) {
	// 先尝试从缓存获取
	if cf, ok := ic.cache.Get(path); ok {
		ic.stats.CacheHits++
		return cf, nil
	}
	ic.stats.CacheMisses++
	
	// 读取源文件
	source, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	
	// 解析
	p := parser.New(string(source), path)
	file := p.Parse()
	if p.HasErrors() {
		return nil, fmt.Errorf("parse errors")
	}
	
	// 编译
	c := New()
	fn, errs := c.Compile(file)
	if len(errs) > 0 {
		return nil, fmt.Errorf("compile errors: %v", errs)
	}
	
	// 构建 CompiledFile
	cf := &bytecode.CompiledFile{
		MainFunction: fn,
		Classes:      c.Classes(),
		Enums:        c.Enums(),
		SourceFile:   path,
	}
	
	// 获取依赖列表
	deps := make([]string, 0)
	for _, use := range file.Uses {
		depPath := ic.resolveImportPath(use.Path, path)
		if depPath != "" {
			deps = append(deps, depPath)
		}
	}
	
	// 存入缓存
	ic.cache.Put(path, cf, deps)
	ic.stats.Recompiled++
	
	return cf, nil
}

// detectChanges 检测文件变更
func (ic *IncrementalCompiler) detectChanges() []string {
	changed := make([]string, 0)
	
	for path, state := range ic.fileStates {
		info, err := os.Stat(path)
		if err != nil {
			// 文件可能被删除
			changed = append(changed, path)
			continue
		}
		
		if info.ModTime().After(state.ModTime) {
			changed = append(changed, path)
		}
	}
	
	return changed
}

// getDependents 获取依赖指定文件的所有文件
func (ic *IncrementalCompiler) getDependents(path string) []string {
	ic.depGraph.mu.RLock()
	defer ic.depGraph.mu.RUnlock()
	
	result := make([]string, 0)
	visited := make(map[string]bool)
	
	var collect func(p string)
	collect = func(p string) {
		if visited[p] {
			return
		}
		visited[p] = true
		
		node, ok := ic.depGraph.nodes[p]
		if !ok {
			return
		}
		
		for _, dependent := range node.ImportedBy {
			result = append(result, dependent)
			collect(dependent)
		}
	}
	
	collect(path)
	return result
}

// WatchFile 添加文件到监视列表
func (ic *IncrementalCompiler) WatchFile(path string) error {
	ic.mu.Lock()
	defer ic.mu.Unlock()
	
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	
	hash := ""
	if data, err := os.ReadFile(path); err == nil {
		hash = ComputeContentHash(string(data))
	}
	
	ic.fileStates[path] = &FileState{
		Path:    path,
		Hash:    hash,
		ModTime: info.ModTime(),
	}
	
	return nil
}

// ============================================================================
// 依赖分析辅助函数
// ============================================================================

// ExtractImports 从 AST 提取导入
func ExtractImports(file *ast.File) []string {
	imports := make([]string, 0, len(file.Uses))
	for _, use := range file.Uses {
		imports = append(imports, use.Path)
	}
	return imports
}
