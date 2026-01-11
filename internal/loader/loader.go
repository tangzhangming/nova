// Package loader 实现 Sola 语言的包加载和模块解析功能。
//
// 主要功能：
//   - 解析项目配置文件 (sola.toml)
//   - 管理标准库、项目文件和第三方依赖的加载
//   - 处理模块导入路径的解析
//
// 加载顺序：
//  1. 标准库 (sola.* 前缀) - 位于可执行文件上一级的 src/ 目录
//  2. 项目内模块 - 基于项目命名空间查找
//  3. 第三方依赖 - 从包仓库目录加载
package loader

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tangzhangming/nova/internal/i18n"
)

// ============================================================================
// 常量定义
// ============================================================================

const (
	// SourceFileExtension 是 Sola 源代码文件的扩展名
	SourceFileExtension = ".sola"

	// ProjectConfigFile 是项目配置文件名
	// 该文件定义项目名称、命名空间和依赖关系
	ProjectConfigFile = "sola.toml"

	// StdLibPrefix 是标准库导入的前缀
	// 所有以 "sola." 开头的导入都会从标准库目录查找
	StdLibPrefix = "sola"

	// StdLibDirName 是标准库目录名称
	// 标准库位于可执行文件上一级目录的 src/ 子目录中
	StdLibDirName = "src"

	// PackageRepoDirEnv 是包仓库路径的环境变量名
	// 用户可以通过设置此环境变量来自定义包仓库位置
	PackageRepoDirEnv = "SOLA_REPO_PATH"

	// PackageRepoDirName 是包仓库的默认子目录名
	// 默认位于用户主目录下的 .sola/packages 目录
	PackageRepoDirName = ".sola/packages"
)

// ============================================================================
// 包仓库管理
// ============================================================================

// getPackageRepoDir 获取第三方包仓库目录路径。
//
// 查找顺序：
//  1. 环境变量 SOLA_REPO_PATH（如果已设置）
//  2. 默认目录：
//     - Windows: C:\Users\{用户名}\.sola\packages
//     - Linux/Mac: ~/.sola/packages
//
// 返回包仓库的绝对路径。
func getPackageRepoDir() string {
	// 优先使用环境变量指定的路径
	if envPath := os.Getenv(PackageRepoDirEnv); envPath != "" {
		return envPath
	}

	// 使用用户主目录下的默认路径
	homeDir, err := os.UserHomeDir()
	if err != nil {
		// 如果无法获取主目录，回退到当前目录
		return filepath.Join(".", PackageRepoDirName)
	}
	return filepath.Join(homeDir, PackageRepoDirName)
}

// ============================================================================
// 配置结构体
// ============================================================================

// ProjectConfig 表示项目的配置信息，从 sola.toml 文件解析而来。
//
// 配置文件格式示例：
//
//	[package]
//	name = "myproject"
//	namespace = "com.example.myproject"
//
//	[dependencies]
//	some-package = "1.0.0"
type ProjectConfig struct {
	// Name 是项目名称
	Name string

	// Namespace 是项目的命名空间，用于模块导入路径的解析
	// 例如：namespace = "com.example" 时，导入 "com.example.utils.Helper"
	// 会在项目的 src/utils/Helper.sola 或 utils/Helper.sola 中查找
	Namespace string

	// Dependencies 存储项目的依赖关系
	// 键是包名，值是版本号
	Dependencies map[string]string
}

// DependencyInfo 存储已解析的依赖包信息。
type DependencyInfo struct {
	// Name 是包的名称（与 sola.toml 中的依赖名一致）
	Name string

	// Version 是包的版本号
	Version string

	// Namespace 是包声明的命名空间（从包的 sola.toml 读取）
	Namespace string

	// Path 是包在本地文件系统中的路径
	Path string
}

// ============================================================================
// 加载器主体
// ============================================================================

// Loader 是包加载器，负责解析和加载 Sola 模块。
//
// 使用方式：
//
//	loader, err := loader.New("main.sola")
//	if err != nil {
//	    // 处理错误
//	}
//	filePath, err := loader.ResolveImport("sola.io.Console")
type Loader struct {
	// rootDir 是项目根目录（包含 sola.toml 的目录，或入口文件所在目录）
	rootDir string

	// libDir 是标准库目录路径
	// 位于可执行文件上一级的 src/ 目录
	libDir string

	// config 是从 sola.toml 加载的项目配置（可能为 nil）
	config *ProjectConfig

	// loadedFiles 记录已加载的文件路径，避免重复加载
	loadedFiles map[string]bool

	// dependencies 存储已解析的依赖包信息
	// 键是包的命名空间，用于快速匹配导入路径
	dependencies map[string]*DependencyInfo

	// pathCache 路径规范化缓存
	// 优化：避免重复调用 filepath.Abs 和 strings.ToLower
	// 键是原始路径，值是规范化后的路径
	pathCache map[string]string
}

// New 创建一个新的包加载器。
//
// 参数 entryFile 是程序入口文件的路径，加载器会：
//  1. 向上查找包含 sola.toml 的目录作为项目根目录
//  2. 定位标准库目录
//  3. 加载项目配置和依赖信息
//
// 如果找不到 sola.toml，则使用入口文件所在目录作为项目根目录。
func New(entryFile string) (*Loader, error) {
	// 将入口文件路径转换为绝对路径，确保路径解析正确
	absEntryFile, err := filepath.Abs(entryFile)
	if err != nil {
		absEntryFile = entryFile
	}

	// 查找项目根目录（包含 sola.toml 的目录）
	rootDir, err := findProjectRoot(absEntryFile)
	if err != nil {
		// 没有 sola.toml 时，使用入口文件所在目录
		rootDir = filepath.Dir(absEntryFile)
	}

	// 获取标准库路径（在可执行文件上一级的 src/ 目录）
	libDir, err := getStdLibPath()
	if err != nil {
		// 找不到标准库时不阻止运行
		// 用户可能只是运行简单脚本，不需要标准库
		libDir = ""
	}

	loader := &Loader{
		rootDir:      rootDir,
		libDir:       libDir,
		loadedFiles:  make(map[string]bool),
		dependencies: make(map[string]*DependencyInfo),
		pathCache:    make(map[string]string, 64), // 预分配常用容量
	}

	// 尝试加载项目配置
	configFile := filepath.Join(rootDir, ProjectConfigFile)
	if _, err := os.Stat(configFile); err == nil {
		config, err := loadProjectConfig(configFile)
		if err != nil {
			return nil, err
		}
		loader.config = config

		// 加载依赖包信息
		if config.Dependencies != nil {
			loader.loadDependencies(config.Dependencies)
		}
	}

	return loader, nil
}

// loadDependencies 加载所有依赖包的信息。
//
// 依赖包存储在包仓库目录中，结构为：
//
//	{包仓库目录}/{包名}/{版本}/
//	  ├── sola.toml    # 包配置文件
//	  └── src/         # 源代码目录
func (l *Loader) loadDependencies(deps map[string]string) {
	pkgRepoDir := getPackageRepoDir()

	for pkgName, version := range deps {
		// 构建依赖包路径：{包仓库目录}/{包名}/{版本}/
		pkgPath := filepath.Join(pkgRepoDir, pkgName, version)
		configPath := filepath.Join(pkgPath, ProjectConfigFile)

		// 读取依赖包的 sola.toml 获取其命名空间
		if _, err := os.Stat(configPath); err == nil {
			pkgConfig, err := loadProjectConfig(configPath)
			if err != nil {
				continue
			}

			// 使用命名空间作为键，方便导入路径匹配
			if pkgConfig.Namespace != "" {
				l.dependencies[pkgConfig.Namespace] = &DependencyInfo{
					Name:      pkgName,
					Version:   version,
					Namespace: pkgConfig.Namespace,
					Path:      pkgPath,
				}
			}
		}
	}
}

// ============================================================================
// 标准库路径
// ============================================================================

// getStdLibPath 获取标准库的路径。
//
// 标准库位于可执行文件上一级目录的 src/ 子目录中。
// 例如，如果 sola.exe 位于 C:\sola\bin\sola.exe，
// 则标准库位于 C:\sola\src\
//
// 目录结构示例：
//
//	sola/
//	├── bin/
//	│   └── sola.exe      # 可执行文件
//	└── src/              # 标准库目录
//	    ├── io/
//	    │   └── Console.sola
//	    ├── collections/
//	    │   └── List.sola
//	    └── ...
func getStdLibPath() (string, error) {
	// 获取当前可执行文件的路径
	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf(i18n.T(i18n.ErrGetExecutablePath, err))
	}

	// 解析符号链接，获取真实路径
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return "", fmt.Errorf(i18n.T(i18n.ErrResolveSymlinks, err))
	}

	// 标准库在可执行文件上一级目录的 src/ 子目录
	// exePath: /path/to/bin/sola.exe
	// exeDir:  /path/to/bin
	// parent:  /path/to
	// libPath: /path/to/src
	exeDir := filepath.Dir(exePath)
	parentDir := filepath.Dir(exeDir)
	libPath := filepath.Join(parentDir, StdLibDirName)

	// 检查标准库目录是否存在
	if _, err := os.Stat(libPath); err != nil {
		return "", fmt.Errorf(i18n.T(i18n.ErrStdLibNotFound, libPath))
	}

	return libPath, nil
}

// ============================================================================
// 项目根目录查找
// ============================================================================

// findProjectRoot 向上查找项目根目录。
//
// 从 startPath 开始，逐级向上查找包含 sola.toml 的目录。
// 如果找到则返回该目录路径，否则返回错误。
func findProjectRoot(startPath string) (string, error) {
	dir := filepath.Dir(startPath)

	for {
		configFile := filepath.Join(dir, ProjectConfigFile)
		if _, err := os.Stat(configFile); err == nil {
			return dir, nil
		}

		// 获取父目录
		parent := filepath.Dir(dir)
		if parent == dir {
			// 已到达文件系统根目录，停止查找
			return "", fmt.Errorf(i18n.T(i18n.ErrProjectConfigNotFound, ProjectConfigFile))
		}
		dir = parent
	}
}

// ============================================================================
// 项目配置加载
// ============================================================================

// loadProjectConfig 加载并解析项目配置文件。
//
// 使用简化的 TOML 解析，支持以下格式：
//
//	[package]
//	name = "项目名"
//	namespace = "命名空间"
//
//	[dependencies]
//	包名 = "版本号"
//
// 注意：这是一个简化的解析器，不支持完整的 TOML 规范。
func loadProjectConfig(path string) (*ProjectConfig, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf(i18n.T(i18n.ErrOpenProjectConfig, ProjectConfigFile, err))
	}
	defer file.Close()

	config := &ProjectConfig{
		Dependencies: make(map[string]string),
	}

	// 当前正在解析的 section 名称
	currentSection := ""

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// 跳过空行和注释
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// 解析 section 头（如 [package]）
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentSection = strings.Trim(line, "[]")
			continue
		}

		// 解析键值对
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.Trim(strings.TrimSpace(parts[1]), "\"")

		// 根据当前 section 处理不同的配置项
		switch currentSection {
		case "package":
			switch key {
			case "name":
				config.Name = value
			case "namespace":
				config.Namespace = value
			}
		case "dependencies":
			// 依赖格式：包名 = "版本号"
			config.Dependencies[key] = value
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf(i18n.T(i18n.ErrReadProjectConfig, ProjectConfigFile, err))
	}

	return config, nil
}

// ============================================================================
// 导入路径解析
// ============================================================================

// ResolveImport 解析导入路径，返回对应的源文件路径。
//
// 导入解析顺序：
//  1. 标准库（sola.* 前缀）- 从标准库目录查找
//  2. 项目内模块 - 基于项目命名空间在项目目录查找
//  3. 依赖包 - 从包仓库目录查找
//  4. 相对路径 - 在项目的 src/ 或根目录查找
//
// 示例：
//
//	ResolveImport("sola.io.Console")           // 标准库
//	ResolveImport("com.example.utils.Helper")  // 项目内模块（假设命名空间为 com.example）
//	ResolveImport("utils.Helper")              // 相对路径
func (l *Loader) ResolveImport(importPath string) (string, error) {
	// 将点分隔的导入路径转换为路径段
	parts := strings.Split(importPath, ".")

	// ========================================
	// 1. 检查是否是标准库导入 (sola.*)
	// ========================================
	if parts[0] == StdLibPrefix {
		if l.libDir == "" {
			return "", fmt.Errorf(i18n.T(i18n.ErrStdLibNotConfigured, importPath))
		}

		// 标准库路径：去掉 sola 前缀
		// 例如：sola.io.Console -> src/io/Console.sola
		libPath := filepath.Join(l.libDir, filepath.Join(parts[1:]...)+SourceFileExtension)
		if _, err := os.Stat(libPath); err == nil {
			absPath, _ := filepath.Abs(libPath)
			return absPath, nil
		}
		return "", fmt.Errorf(i18n.T(i18n.ErrStdLibImportNotFound, importPath, libPath))
	}

	// ========================================
	// 2. 检查是否是当前项目的命名空间
	// ========================================
	if l.config != nil && l.config.Namespace != "" && strings.HasPrefix(importPath, l.config.Namespace) {
		// 计算相对于命名空间的路径
		relativePath := strings.TrimPrefix(importPath, l.config.Namespace+".")
		pathParts := strings.Split(relativePath, ".")

		// 首先在 src/ 目录查找
		srcPath := filepath.Join(l.rootDir, "src", filepath.Join(pathParts...)+SourceFileExtension)
		if _, err := os.Stat(srcPath); err == nil {
			absPath, _ := filepath.Abs(srcPath)
			return absPath, nil
		}

		// 然后在项目根目录查找
		rootPath := filepath.Join(l.rootDir, filepath.Join(pathParts...)+SourceFileExtension)
		if _, err := os.Stat(rootPath); err == nil {
			absPath, _ := filepath.Abs(rootPath)
			return absPath, nil
		}
	}

	// ========================================
	// 3. 检查是否是依赖包的命名空间
	// ========================================
	for ns, dep := range l.dependencies {
		if strings.HasPrefix(importPath, ns) {
			// 计算相对于依赖包命名空间的路径
			relativePath := strings.TrimPrefix(importPath, ns)
			if relativePath != "" && relativePath[0] == '.' {
				relativePath = relativePath[1:]
			}

			var filePath string
			if relativePath == "" {
				// 直接导入命名空间根，查找入口文件
				filePath = filepath.Join(dep.Path, "src", "lib"+SourceFileExtension)
				if _, err := os.Stat(filePath); err != nil {
					filePath = filepath.Join(dep.Path, "src", "index"+SourceFileExtension)
				}
			} else {
				pathParts := strings.Split(relativePath, ".")
				filePath = filepath.Join(dep.Path, "src", filepath.Join(pathParts...)+SourceFileExtension)
			}

			if _, err := os.Stat(filePath); err == nil {
				absPath, _ := filepath.Abs(filePath)
				return absPath, nil
			}
		}
	}

	// ========================================
	// 4. 尝试作为相对路径查找
	// ========================================

	// 在 src/ 目录查找
	srcPath := filepath.Join(l.rootDir, "src", filepath.Join(parts...)+SourceFileExtension)
	if _, err := os.Stat(srcPath); err == nil {
		absPath, _ := filepath.Abs(srcPath)
		return absPath, nil
	}

	// 在项目根目录查找
	filePath := filepath.Join(l.rootDir, filepath.Join(parts...)+SourceFileExtension)
	if _, err := os.Stat(filePath); err == nil {
		absPath, _ := filepath.Abs(filePath)
		return absPath, nil
	}

	return "", fmt.Errorf(i18n.T(i18n.ErrImportNotFound, importPath))
}

// ============================================================================
// 文件加载
// ============================================================================

// LoadFile 加载源文件内容。
//
// 参数 path 是源文件的绝对路径。
// 返回文件内容字符串和可能的错误。
func (l *Loader) LoadFile(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// ============================================================================
// 加载状态管理
// ============================================================================

// MarkLoaded 标记文件已加载。
//
// 用于避免重复加载同一文件，防止循环依赖导致的无限循环。
// 路径会被规范化后存储，确保不同写法的路径能正确匹配。
func (l *Loader) MarkLoaded(path string) {
	normalizedPath := l.normalizePath(path)
	l.loadedFiles[normalizedPath] = true
}

// IsLoaded 检查文件是否已加载。
//
// 返回 true 表示文件已被加载过，不需要再次加载。
func (l *Loader) IsLoaded(path string) bool {
	normalizedPath := l.normalizePath(path)
	return l.loadedFiles[normalizedPath]
}

// normalizePath 规范化文件路径。
//
// 处理步骤：
//  1. 转换为绝对路径
//  2. 清理路径（解析 . 和 ..）
//  3. 转换为小写（Windows 文件系统不区分大小写）
//
// 优化：使用缓存避免重复计算，filepath.Abs 和 strings.ToLower 在高频调用时开销较大
func (l *Loader) normalizePath(path string) string {
	// 快速路径：检查缓存
	if cached, ok := l.pathCache[path]; ok {
		return cached
	}

	// 获取绝对路径
	absPath, err := filepath.Abs(path)
	if err != nil {
		return path
	}

	// 清理路径（解析 . 和 ..）
	cleanPath := filepath.Clean(absPath)

	// 转换为小写（Windows 文件系统不区分大小写）
	normalized := strings.ToLower(cleanPath)

	// 存入缓存
	l.pathCache[path] = normalized
	return normalized
}

// ============================================================================
// 公共访问器
// ============================================================================

// GetProjectNamespace 获取项目的命名空间。
//
// 如果项目没有配置文件或未设置命名空间，返回空字符串。
func (l *Loader) GetProjectNamespace() string {
	if l.config != nil {
		return l.config.Namespace
	}
	return ""
}

// RootDir 获取项目根目录路径。
func (l *Loader) RootDir() string {
	return l.rootDir
}
