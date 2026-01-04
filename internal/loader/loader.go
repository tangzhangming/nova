package loader

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ProjectConfig 项目配置
type ProjectConfig struct {
	Name      string
	Namespace string
}

// Loader 包加载器
type Loader struct {
	rootDir     string        // 项目根目录
	libDir      string        // 标准库目录
	config      *ProjectConfig
	loadedFiles map[string]bool
}

// New 创建加载器
func New(entryFile string) (*Loader, error) {
	// 查找项目根目录（包含 project.toml 的目录）
	rootDir, err := findProjectRoot(entryFile)
	if err != nil {
		// 没有 project.toml，使用入口文件所在目录
		rootDir = filepath.Dir(entryFile)
	}

	loader := &Loader{
		rootDir:     rootDir,
		libDir:      filepath.Join(rootDir, "lib"),
		loadedFiles: make(map[string]bool),
	}

	// 尝试加载项目配置
	configFile := filepath.Join(rootDir, "project.toml")
	if _, err := os.Stat(configFile); err == nil {
		config, err := loadProjectConfig(configFile)
		if err != nil {
			return nil, err
		}
		loader.config = config
	}

	return loader, nil
}

// findProjectRoot 向上查找项目根目录
func findProjectRoot(startPath string) (string, error) {
	dir := filepath.Dir(startPath)
	for {
		configFile := filepath.Join(dir, "project.toml")
		if _, err := os.Stat(configFile); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("project.toml not found")
		}
		dir = parent
	}
}

// loadProjectConfig 加载项目配置（简化的 TOML 解析）
func loadProjectConfig(path string) (*ProjectConfig, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open project.toml: %w", err)
	}
	defer file.Close()

	config := &ProjectConfig{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.Trim(strings.TrimSpace(parts[1]), "\"")
		
		switch key {
		case "name":
			config.Name = value
		case "namespace":
			config.Namespace = value
		}
	}
	
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read project.toml: %w", err)
	}
	
	return config, nil
}

// ResolveImport 解析导入路径，返回源文件路径
func (l *Loader) ResolveImport(importPath string) (string, error) {
	// 将点分隔路径转换为文件路径
	parts := strings.Split(importPath, ".")

	// nova 开头的是标准库
	if parts[0] == "nova" {
		// 标准库路径：lib/nova/math/Math.nova
		libPath := filepath.Join(l.libDir, filepath.Join(parts...) + ".nova")
		if _, err := os.Stat(libPath); err == nil {
			return libPath, nil
		}
		return "", fmt.Errorf("standard library not found: %s (tried %s)", importPath, libPath)
	}

	// 检查是否是当前项目的命名空间
	if l.config != nil && strings.HasPrefix(importPath, l.config.Namespace) {
		relativePath := strings.TrimPrefix(importPath, l.config.Namespace+".")
		parts := strings.Split(relativePath, ".")
		filePath := filepath.Join(l.rootDir, filepath.Join(parts...) + ".nova")
		if _, err := os.Stat(filePath); err == nil {
			return filePath, nil
		}
	}

	// 尝试在项目根目录查找
	filePath := filepath.Join(l.rootDir, filepath.Join(parts...) + ".nova")
	if _, err := os.Stat(filePath); err == nil {
		return filePath, nil
	}

	return "", fmt.Errorf("import not found: %s", importPath)
}

// LoadFile 加载源文件内容
func (l *Loader) LoadFile(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// MarkLoaded 标记文件已加载
func (l *Loader) MarkLoaded(path string) {
	l.loadedFiles[path] = true
}

// IsLoaded 检查文件是否已加载
func (l *Loader) IsLoaded(path string) bool {
	return l.loadedFiles[path]
}

// GetProjectNamespace 获取项目命名空间
func (l *Loader) GetProjectNamespace() string {
	if l.config != nil {
		return l.config.Namespace
	}
	return ""
}

// RootDir 获取项目根目录
func (l *Loader) RootDir() string {
	return l.rootDir
}

