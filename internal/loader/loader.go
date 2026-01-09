package loader

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// 常量定义
const (
	SourceFileExtension = ".sola"      // 源码文件后缀
	ProjectConfigFile   = "sola.toml"  // 项目配置文件名
	StdLibPrefix        = "sola"       // 标准库导入前缀
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
	// 查找项目根目录（包含 sola.toml 的目录）
	rootDir, err := findProjectRoot(entryFile)
	if err != nil {
		// 没有 sola.toml，使用入口文件所在目录
		rootDir = filepath.Dir(entryFile)
	}

	// 获取标准库路径（在可执行文件同级目录）
	libDir, err := getStdLibPath()
	if err != nil {
		// 如果找不到标准库，返回错误但不阻止运行
		// 用户可能只是运行简单脚本，不需要标准库
		libDir = ""
	}

	loader := &Loader{
		rootDir:     rootDir,
		libDir:      libDir,
		loadedFiles: make(map[string]bool),
	}

	// 尝试加载项目配置
	configFile := filepath.Join(rootDir, ProjectConfigFile)
	if _, err := os.Stat(configFile); err == nil {
		config, err := loadProjectConfig(configFile)
		if err != nil {
			return nil, err
		}
		loader.config = config
	}

	return loader, nil
}

// getStdLibPath 获取标准库路径
// 标准库位于可执行文件同级的 lib/ 目录
func getStdLibPath() (string, error) {
	// 获取可执行文件路径
	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %w", err)
	}

	// 解析符号链接
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve symlinks: %w", err)
	}

	// 标准库在可执行文件同级的 lib/ 目录
	exeDir := filepath.Dir(exePath)
	libPath := filepath.Join(exeDir, "lib")

	// 检查是否存在
	if _, err := os.Stat(libPath); err != nil {
		return "", fmt.Errorf("standard library not found at %s", libPath)
	}

	return libPath, nil
}

// findProjectRoot 向上查找项目根目录
func findProjectRoot(startPath string) (string, error) {
	dir := filepath.Dir(startPath)
	for {
		configFile := filepath.Join(dir, ProjectConfigFile)
		if _, err := os.Stat(configFile); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("%s not found", ProjectConfigFile)
		}
		dir = parent
	}
}

// loadProjectConfig 加载项目配置（简化的 TOML 解析）
func loadProjectConfig(path string) (*ProjectConfig, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open %s: %w", ProjectConfigFile, err)
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
		return nil, fmt.Errorf("failed to read %s: %w", ProjectConfigFile, err)
	}
	
	return config, nil
}

// ResolveImport 解析导入路径，返回源文件路径
func (l *Loader) ResolveImport(importPath string) (string, error) {
	// 将点分隔路径转换为文件路径
	parts := strings.Split(importPath, ".")

	// sola 开头的是标准库
	if parts[0] == StdLibPrefix {
		if l.libDir == "" {
			return "", fmt.Errorf("standard library not configured, cannot import: %s", importPath)
		}
		// 标准库路径：去掉 sola 前缀，例如 sola.io.Console -> lib/io/Console.sola
		libPath := filepath.Join(l.libDir, filepath.Join(parts[1:]...) + SourceFileExtension)
		if _, err := os.Stat(libPath); err == nil {
			// 返回规范化的绝对路径
			absPath, _ := filepath.Abs(libPath)
			return absPath, nil
		}
		return "", fmt.Errorf("standard library not found: %s (tried %s)", importPath, libPath)
	}

	// 检查是否是当前项目的命名空间
	if l.config != nil && l.config.Namespace != "" && strings.HasPrefix(importPath, l.config.Namespace) {
		relativePath := strings.TrimPrefix(importPath, l.config.Namespace+".")
		pathParts := strings.Split(relativePath, ".")

		// 首先在 src/ 目录查找
		srcPath := filepath.Join(l.rootDir, "src", filepath.Join(pathParts...) + SourceFileExtension)
		if _, err := os.Stat(srcPath); err == nil {
			absPath, _ := filepath.Abs(srcPath)
			return absPath, nil
		}

		// 然后在项目根目录查找
		rootPath := filepath.Join(l.rootDir, filepath.Join(pathParts...) + SourceFileExtension)
		if _, err := os.Stat(rootPath); err == nil {
			absPath, _ := filepath.Abs(rootPath)
			return absPath, nil
		}
	}

	// 尝试在 src/ 目录查找
	srcPath := filepath.Join(l.rootDir, "src", filepath.Join(parts...) + SourceFileExtension)
	if _, err := os.Stat(srcPath); err == nil {
		absPath, _ := filepath.Abs(srcPath)
		return absPath, nil
	}

	// 尝试在项目根目录查找
	filePath := filepath.Join(l.rootDir, filepath.Join(parts...) + SourceFileExtension)
	if _, err := os.Stat(filePath); err == nil {
		absPath, _ := filepath.Abs(filePath)
		return absPath, nil
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
	// 规范化路径，确保一致性
	normalizedPath := normalizePath(path)
	l.loadedFiles[normalizedPath] = true
}

// IsLoaded 检查文件是否已加载
func (l *Loader) IsLoaded(path string) bool {
	normalizedPath := normalizePath(path)
	return l.loadedFiles[normalizedPath]
}

// normalizePath 规范化路径
func normalizePath(path string) string {
	// 获取绝对路径
	absPath, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	// 清理路径（解析 . 和 ..）
	cleanPath := filepath.Clean(absPath)
	// 转换为小写（Windows 不区分大小写）
	return strings.ToLower(cleanPath)
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

