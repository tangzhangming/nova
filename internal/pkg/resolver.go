// resolver.go - 依赖解析器
//
// 实现依赖解析算法（最小版本选择 MVS）。
//
// MVS（Minimal Version Selection）算法：
// - 选择满足所有约束的最小版本
// - 避免版本冲突
// - 保证构建的可重复性

package pkg

import (
	"fmt"
	"sort"
	"sync"
)

// Resolver 依赖解析器
type Resolver struct {
	mu sync.RWMutex
	
	// 已解析的模块缓存
	resolved map[string]*ResolvedModule
	
	// 版本获取器
	versionFetcher VersionFetcher
}

// ResolvedModule 已解析的模块
type ResolvedModule struct {
	Path     string
	Version  string
	Deps     []Requirement
	Selected bool
}

// VersionFetcher 版本获取接口
type VersionFetcher interface {
	// GetVersions 获取模块所有可用版本
	GetVersions(modulePath string) ([]string, error)
	
	// GetDependencies 获取指定版本的依赖
	GetDependencies(modulePath, version string) ([]Requirement, error)
}

// NewResolver 创建解析器
func NewResolver() *Resolver {
	return &Resolver{
		resolved: make(map[string]*ResolvedModule),
	}
}

// SetVersionFetcher 设置版本获取器
func (r *Resolver) SetVersionFetcher(fetcher VersionFetcher) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.versionFetcher = fetcher
}

// Resolve 解析依赖
// 使用 MVS（Minimal Version Selection）算法
func (r *Resolver) Resolve(module *Module) ([]Requirement, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	// 清除缓存
	r.resolved = make(map[string]*ResolvedModule)
	
	// 构建依赖图
	graph := make(map[string]map[string]bool) // path -> version -> bool
	
	// 添加直接依赖
	for _, req := range module.Require {
		if graph[req.Path] == nil {
			graph[req.Path] = make(map[string]bool)
		}
		graph[req.Path][req.Version] = true
	}
	
	// 递归解析传递依赖
	for _, req := range module.Require {
		if err := r.resolveTransitive(req.Path, req.Version, graph); err != nil {
			return nil, err
		}
	}
	
	// 应用 MVS：为每个模块选择最高的必需版本
	result := make([]Requirement, 0)
	for path, versions := range graph {
		// 获取所有版本并排序
		versionList := make([]string, 0, len(versions))
		for v := range versions {
			versionList = append(versionList, v)
		}
		
		// 使用语义化版本排序，选择最高版本
		sortVersions(versionList)
		selectedVersion := versionList[len(versionList)-1]
		
		result = append(result, Requirement{
			Path:    path,
			Version: selectedVersion,
		})
	}
	
	// 按路径排序结果
	sort.Slice(result, func(i, j int) bool {
		return result[i].Path < result[j].Path
	})
	
	return result, nil
}

// resolveTransitive 递归解析传递依赖
func (r *Resolver) resolveTransitive(path, version string, graph map[string]map[string]bool) error {
	key := path + "@" + version
	
	// 检查是否已解析
	if _, ok := r.resolved[key]; ok {
		return nil
	}
	
	// 标记为正在解析
	r.resolved[key] = &ResolvedModule{
		Path:    path,
		Version: version,
	}
	
	// 获取依赖
	if r.versionFetcher != nil {
		deps, err := r.versionFetcher.GetDependencies(path, version)
		if err != nil {
			// 忽略获取失败，可能是本地模块
			return nil
		}
		
		r.resolved[key].Deps = deps
		
		// 递归处理依赖
		for _, dep := range deps {
			if graph[dep.Path] == nil {
				graph[dep.Path] = make(map[string]bool)
			}
			graph[dep.Path][dep.Version] = true
			
			if err := r.resolveTransitive(dep.Path, dep.Version, graph); err != nil {
				return err
			}
		}
	}
	
	return nil
}

// CheckCompatibility 检查版本兼容性
func (r *Resolver) CheckCompatibility(requirements []Requirement) error {
	// 检查是否有冲突的版本约束
	constraints := make(map[string][]string)
	
	for _, req := range requirements {
		constraints[req.Path] = append(constraints[req.Path], req.Version)
	}
	
	for path, versions := range constraints {
		if len(versions) > 1 {
			// 检查版本是否兼容
			sortVersions(versions)
			minVersion := versions[0]
			maxVersion := versions[len(versions)-1]
			
			// 检查主版本号是否相同
			minMajor := getMajorVersion(minVersion)
			maxMajor := getMajorVersion(maxVersion)
			
			if minMajor != maxMajor {
				return fmt.Errorf("incompatible version constraints for %s: %s and %s",
					path, minVersion, maxVersion)
			}
		}
	}
	
	return nil
}

// GetResolved 获取已解析的模块
func (r *Resolver) GetResolved() map[string]*ResolvedModule {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	result := make(map[string]*ResolvedModule)
	for k, v := range r.resolved {
		result[k] = v
	}
	return result
}

// BuildDependencyTree 构建依赖树
func (r *Resolver) BuildDependencyTree(module *Module) *DependencyTree {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	tree := &DependencyTree{
		Root: &DependencyNode{
			Path:    module.Path,
			Version: "",
		},
	}
	
	// 添加直接依赖
	for _, req := range module.Require {
		node := &DependencyNode{
			Path:    req.Path,
			Version: req.Version,
		}
		tree.Root.Children = append(tree.Root.Children, node)
		
		// 添加传递依赖
		r.buildSubTree(node, req.Path, req.Version)
	}
	
	return tree
}

// buildSubTree 构建子树
func (r *Resolver) buildSubTree(node *DependencyNode, path, version string) {
	key := path + "@" + version
	resolved, ok := r.resolved[key]
	if !ok {
		return
	}
	
	for _, dep := range resolved.Deps {
		child := &DependencyNode{
			Path:    dep.Path,
			Version: dep.Version,
		}
		node.Children = append(node.Children, child)
		r.buildSubTree(child, dep.Path, dep.Version)
	}
}

// DependencyTree 依赖树
type DependencyTree struct {
	Root *DependencyNode
}

// DependencyNode 依赖节点
type DependencyNode struct {
	Path     string
	Version  string
	Children []*DependencyNode
}

// String 格式化输出
func (n *DependencyNode) String() string {
	if n.Version == "" {
		return n.Path
	}
	return n.Path + "@" + n.Version
}

// Print 打印依赖树
func (t *DependencyTree) Print() string {
	return t.printNode(t.Root, "", true)
}

func (t *DependencyTree) printNode(node *DependencyNode, prefix string, isLast bool) string {
	result := ""
	
	// 打印当前节点
	connector := "├── "
	if isLast {
		connector = "└── "
	}
	if prefix == "" {
		result += node.String() + "\n"
	} else {
		result += prefix + connector + node.String() + "\n"
	}
	
	// 更新前缀
	childPrefix := prefix
	if prefix != "" {
		if isLast {
			childPrefix += "    "
		} else {
			childPrefix += "│   "
		}
	}
	
	// 打印子节点
	for i, child := range node.Children {
		isChildLast := i == len(node.Children)-1
		result += t.printNode(child, childPrefix, isChildLast)
	}
	
	return result
}
