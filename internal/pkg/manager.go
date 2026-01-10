// manager.go - Sola 包管理器
//
// 实现模块和依赖管理功能。
//
// 功能：
// 1. sola.mod 文件解析和管理
// 2. 依赖解析和下载
// 3. 本地包缓存
// 4. 版本管理

package pkg

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

const (
	// ModFileName 模块定义文件名
	ModFileName = "sola.mod"
	
	// LockFileName 锁文件名
	LockFileName = "sola.lock"
	
	// DefaultCacheDir 默认缓存目录
	DefaultCacheDir = ".sola/pkg"
)

// Manager 包管理器
type Manager struct {
	mu sync.RWMutex
	
	// 当前模块
	module *Module
	
	// 缓存目录
	cacheDir string
	
	// 已加载的模块
	loaded map[string]*Module
	
	// 依赖解析器
	resolver *Resolver
	
	// HTTP 客户端
	client *http.Client
}

// Module 模块定义
type Module struct {
	// 模块路径（唯一标识）
	Path string `json:"module"`
	
	// Sola 版本要求
	SolaVersion string `json:"sola,omitempty"`
	
	// 依赖列表
	Require []Requirement `json:"require,omitempty"`
	
	// 替换规则
	Replace []Replacement `json:"replace,omitempty"`
	
	// 排除列表
	Exclude []string `json:"exclude,omitempty"`
}

// Requirement 依赖项
type Requirement struct {
	Path    string `json:"path"`
	Version string `json:"version"`
	// 间接依赖标记
	Indirect bool `json:"indirect,omitempty"`
}

// Replacement 替换规则
type Replacement struct {
	Old     string `json:"old"`
	New     string `json:"new"`
	Version string `json:"version,omitempty"`
}

// NewManager 创建包管理器
func NewManager() (*Manager, error) {
	// 获取缓存目录
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "."
	}
	cacheDir := filepath.Join(homeDir, DefaultCacheDir)
	
	// 确保缓存目录存在
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}
	
	return &Manager{
		cacheDir: cacheDir,
		loaded:   make(map[string]*Module),
		resolver: NewResolver(),
		client:   &http.Client{},
	}, nil
}

// Init 初始化新模块
func (m *Manager) Init(modulePath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// 检查是否已存在 sola.mod
	if _, err := os.Stat(ModFileName); err == nil {
		return fmt.Errorf("sola.mod already exists")
	}
	
	// 创建模块
	m.module = &Module{
		Path:        modulePath,
		SolaVersion: ">=0.1.0",
		Require:     []Requirement{},
	}
	
	// 写入文件
	return m.saveModFile()
}

// Load 加载当前目录的模块
func (m *Manager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// 读取 sola.mod
	data, err := os.ReadFile(ModFileName)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", ModFileName, err)
	}
	
	// 解析
	m.module = &Module{}
	if err := json.Unmarshal(data, m.module); err != nil {
		return fmt.Errorf("failed to parse %s: %w", ModFileName, err)
	}
	
	return nil
}

// Tidy 整理依赖（添加缺失的，移除未使用的）
func (m *Manager) Tidy() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if m.module == nil {
		return fmt.Errorf("no module loaded")
	}
	
	// 扫描所有源文件，收集导入
	imports, err := m.scanImports(".")
	if err != nil {
		return err
	}
	
	// 过滤出外部依赖
	external := m.filterExternalImports(imports)
	
	// 更新 require 列表
	newRequire := make([]Requirement, 0)
	for _, imp := range external {
		// 检查是否已在依赖列表中
		found := false
		for _, req := range m.module.Require {
			if req.Path == imp {
				newRequire = append(newRequire, req)
				found = true
				break
			}
		}
		if !found {
			// 添加新依赖（使用 latest 版本）
			version, err := m.fetchLatestVersion(imp)
			if err != nil {
				version = "latest"
			}
			newRequire = append(newRequire, Requirement{
				Path:    imp,
				Version: version,
			})
		}
	}
	
	m.module.Require = newRequire
	return m.saveModFile()
}

// Download 下载所有依赖
func (m *Manager) Download() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if m.module == nil {
		return fmt.Errorf("no module loaded")
	}
	
	// 解析依赖图
	deps, err := m.resolver.Resolve(m.module)
	if err != nil {
		return err
	}
	
	// 下载每个依赖
	for _, dep := range deps {
		if err := m.downloadModule(dep.Path, dep.Version); err != nil {
			return fmt.Errorf("failed to download %s@%s: %w", dep.Path, dep.Version, err)
		}
		fmt.Printf("Downloaded: %s@%s\n", dep.Path, dep.Version)
	}
	
	// 生成锁文件
	return m.saveLockFile(deps)
}

// Add 添加依赖
func (m *Manager) Add(modulePath, version string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if m.module == nil {
		return fmt.Errorf("no module loaded")
	}
	
	// 如果未指定版本，获取最新版本
	if version == "" {
		var err error
		version, err = m.fetchLatestVersion(modulePath)
		if err != nil {
			version = "latest"
		}
	}
	
	// 检查是否已存在
	for i, req := range m.module.Require {
		if req.Path == modulePath {
			// 更新版本
			m.module.Require[i].Version = version
			return m.saveModFile()
		}
	}
	
	// 添加新依赖
	m.module.Require = append(m.module.Require, Requirement{
		Path:    modulePath,
		Version: version,
	})
	
	return m.saveModFile()
}

// Remove 移除依赖
func (m *Manager) Remove(modulePath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if m.module == nil {
		return fmt.Errorf("no module loaded")
	}
	
	// 查找并移除
	newRequire := make([]Requirement, 0)
	found := false
	for _, req := range m.module.Require {
		if req.Path == modulePath {
			found = true
		} else {
			newRequire = append(newRequire, req)
		}
	}
	
	if !found {
		return fmt.Errorf("dependency not found: %s", modulePath)
	}
	
	m.module.Require = newRequire
	return m.saveModFile()
}

// GetModule 获取当前模块
func (m *Manager) GetModule() *Module {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.module
}

// GetCacheDir 获取缓存目录
func (m *Manager) GetCacheDir() string {
	return m.cacheDir
}

// GetModulePath 获取模块在缓存中的路径
func (m *Manager) GetModulePath(modulePath, version string) string {
	return filepath.Join(m.cacheDir, modulePath+"@"+version)
}

// ============================================================================
// 内部方法
// ============================================================================

// saveModFile 保存 sola.mod 文件
func (m *Manager) saveModFile() error {
	data, err := json.MarshalIndent(m.module, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(ModFileName, data, 0644)
}

// saveLockFile 保存 sola.lock 文件
func (m *Manager) saveLockFile(deps []Requirement) error {
	lock := &LockFile{
		Version:      "1",
		Dependencies: deps,
	}
	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(LockFileName, data, 0644)
}

// scanImports 扫描源文件中的导入
func (m *Manager) scanImports(dir string) ([]string, error) {
	imports := make(map[string]bool)
	
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		// 跳过隐藏目录和缓存
		if info.IsDir() && strings.HasPrefix(info.Name(), ".") {
			return filepath.SkipDir
		}
		
		// 只处理 .sola 文件
		if !strings.HasSuffix(path, ".sola") {
			return nil
		}
		
		// 读取并解析导入
		fileImports, err := m.extractImports(path)
		if err != nil {
			return nil // 忽略解析错误
		}
		
		for _, imp := range fileImports {
			imports[imp] = true
		}
		
		return nil
	})
	
	if err != nil {
		return nil, err
	}
	
	result := make([]string, 0, len(imports))
	for imp := range imports {
		result = append(result, imp)
	}
	return result, nil
}

// extractImports 从文件中提取导入
func (m *Manager) extractImports(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	
	// 简单的 use 语句解析
	imports := make([]string, 0)
	lines := strings.Split(string(data), "\n")
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "use ") {
			// 提取路径
			path := strings.TrimPrefix(line, "use ")
			path = strings.TrimSuffix(path, ";")
			path = strings.Trim(path, "\"'")
			if path != "" {
				imports = append(imports, path)
			}
		}
	}
	
	return imports, nil
}

// filterExternalImports 过滤出外部导入
func (m *Manager) filterExternalImports(imports []string) []string {
	external := make([]string, 0)
	
	for _, imp := range imports {
		// 检查是否是标准库
		if strings.HasPrefix(imp, "std/") || strings.HasPrefix(imp, "lang/") {
			continue
		}
		
		// 检查是否是本地文件
		if strings.HasPrefix(imp, "./") || strings.HasPrefix(imp, "../") {
			continue
		}
		
		// 检查是否包含域名（外部包）
		if strings.Contains(imp, "/") && strings.Contains(imp, ".") {
			external = append(external, imp)
		}
	}
	
	return external
}

// fetchLatestVersion 获取最新版本
func (m *Manager) fetchLatestVersion(modulePath string) (string, error) {
	// 尝试从 Git 仓库获取最新标签
	if strings.HasPrefix(modulePath, "github.com/") {
		// 转换为 API URL
		parts := strings.Split(modulePath, "/")
		if len(parts) >= 3 {
			apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest",
				parts[1], parts[2])
			
			resp, err := m.client.Get(apiURL)
			if err != nil {
				return "latest", nil
			}
			defer resp.Body.Close()
			
			if resp.StatusCode == 200 {
				var release struct {
					TagName string `json:"tag_name"`
				}
				if err := json.NewDecoder(resp.Body).Decode(&release); err == nil {
					return release.TagName, nil
				}
			}
		}
	}
	
	return "latest", nil
}

// downloadModule 下载模块
func (m *Manager) downloadModule(modulePath, version string) error {
	destDir := m.GetModulePath(modulePath, version)
	
	// 检查是否已下载
	if _, err := os.Stat(destDir); err == nil {
		return nil // 已存在
	}
	
	// 确保目标目录存在
	if err := os.MkdirAll(filepath.Dir(destDir), 0755); err != nil {
		return err
	}
	
	// 根据模块路径确定下载方式
	if strings.HasPrefix(modulePath, "github.com/") {
		return m.downloadFromGit(modulePath, version, destDir)
	}
	
	// 尝试 HTTP 下载
	return m.downloadFromHTTP(modulePath, version, destDir)
}

// downloadFromGit 从 Git 仓库下载
func (m *Manager) downloadFromGit(modulePath, version, destDir string) error {
	// 构建 Git URL
	gitURL := "https://" + modulePath + ".git"
	
	// 克隆仓库
	cmd := exec.Command("git", "clone", "--depth", "1", "--branch", version, gitURL, destDir)
	if version == "latest" {
		cmd = exec.Command("git", "clone", "--depth", "1", gitURL, destDir)
	}
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git clone failed: %s", string(output))
	}
	
	// 删除 .git 目录以节省空间
	gitDir := filepath.Join(destDir, ".git")
	os.RemoveAll(gitDir)
	
	return nil
}

// downloadFromHTTP 从 HTTP 下载
func (m *Manager) downloadFromHTTP(modulePath, version, destDir string) error {
	// 构建下载 URL
	url := fmt.Sprintf("https://%s/archive/%s.tar.gz", modulePath, version)
	
	resp, err := m.client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	
	// 创建临时文件
	tmpFile, err := os.CreateTemp("", "sola-pkg-*.tar.gz")
	if err != nil {
		return err
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()
	
	// 下载内容
	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		return err
	}
	
	// 解压到目标目录
	return m.extractTarGz(tmpFile.Name(), destDir)
}

// extractTarGz 解压 tar.gz 文件
func (m *Manager) extractTarGz(srcFile, destDir string) error {
	// 使用系统命令解压
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}
	
	cmd := exec.Command("tar", "-xzf", srcFile, "-C", destDir, "--strip-components=1")
	return cmd.Run()
}

// LockFile 锁文件结构
type LockFile struct {
	Version      string        `json:"version"`
	Dependencies []Requirement `json:"dependencies"`
}

// LoadLockFile 加载锁文件
func (m *Manager) LoadLockFile() (*LockFile, error) {
	data, err := os.ReadFile(LockFileName)
	if err != nil {
		return nil, err
	}
	
	lock := &LockFile{}
	if err := json.Unmarshal(data, lock); err != nil {
		return nil, err
	}
	
	return lock, nil
}
