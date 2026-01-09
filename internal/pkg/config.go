// Package pkg 实现 Sola 包管理相关功能
package pkg

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// 常量定义
const (
	ConfigFileName = "sola.toml" // 配置文件名
)

// PackageConfig 包配置
type PackageConfig struct {
	Package      PackageInfo       `toml:"package"`
	Dependencies map[string]string `toml:"dependencies"` // 包名 -> 版本号
}

// PackageInfo 包信息
type PackageInfo struct {
	// Name 包名（建议使用域名反转格式，如 com.example.myapp）
	Name string `toml:"name"`

	// Version 版本号（遵循语义化版本，如 1.0.0）
	Version string `toml:"version"`

	// Namespace 命名空间（用于 use 语句，如 my.app）
	Namespace string `toml:"namespace"`
}

// LoadConfig 从文件加载配置
func LoadConfig(path string) (*PackageConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config PackageConfig
	if err := toml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &config, nil
}

// Save 保存配置到文件
func (c *PackageConfig) Save(path string) error {
	// 生成带注释的配置文件内容
	content := generateConfigWithComments(c)

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// generateConfigWithComments 生成带注释的配置文件内容
func generateConfigWithComments(c *PackageConfig) string {
	var sb strings.Builder

	sb.WriteString("[package]\n")
	sb.WriteString("# 包名（建议使用有意义的名称）\n")
	sb.WriteString(fmt.Sprintf("name = %q\n\n", c.Package.Name))
	sb.WriteString("# 版本号（遵循语义化版本）\n")
	sb.WriteString(fmt.Sprintf("version = %q\n\n", c.Package.Version))
	sb.WriteString("# 命名空间（用于 use 语句导入）\n")
	sb.WriteString(fmt.Sprintf("namespace = %q\n", c.Package.Namespace))

	return sb.String()
}

// GenerateDefault 生成默认配置
// dir 是项目目录路径，用于生成默认的项目名
func GenerateDefault(dir string) *PackageConfig {
	// 从目录名生成默认名称
	baseName := filepath.Base(dir)
	if baseName == "" || baseName == "." || baseName == "/" {
		baseName = "my-app"
	}

	// 清理名称（移除特殊字符）
	name := sanitizeName(baseName)

	return &PackageConfig{
		Package: PackageInfo{
			Name:      name,
			Version:   "0.1.0",
			Namespace: "company.project", // 默认命名空间
		},
	}
}

// sanitizeName 清理包名
func sanitizeName(name string) string {
	// 转换为小写，替换空格和下划线为连字符
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, " ", "-")
	name = strings.ReplaceAll(name, "_", "-")

	// 移除非法字符
	var result strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '.' {
			result.WriteRune(r)
		}
	}

	s := result.String()
	if s == "" {
		return "my-app"
	}
	return s
}

// sanitizeNamespace 清理命名空间
func sanitizeNamespace(name string) string {
	// 转换为小写，替换连字符和空格为点
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, "-", ".")
	name = strings.ReplaceAll(name, "_", ".")
	name = strings.ReplaceAll(name, " ", ".")

	// 移除非法字符
	var result strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' {
			result.WriteRune(r)
		}
	}

	s := result.String()
	// 移除连续的点
	for strings.Contains(s, "..") {
		s = strings.ReplaceAll(s, "..", ".")
	}
	// 移除开头和结尾的点
	s = strings.Trim(s, ".")

	if s == "" {
		return "my.app"
	}
	return s
}

// FindConfigFile 从指定路径向上查找配置文件
// 返回配置文件的完整路径，如果找不到则返回空字符串
func FindConfigFile(startPath string) string {
	// 如果是文件，从其所在目录开始
	info, err := os.Stat(startPath)
	if err != nil {
		return ""
	}

	var dir string
	if info.IsDir() {
		dir = startPath
	} else {
		dir = filepath.Dir(startPath)
	}

	// 转换为绝对路径
	dir, err = filepath.Abs(dir)
	if err != nil {
		return ""
	}

	// 向上查找
	for {
		configPath := filepath.Join(dir, ConfigFileName)
		if _, err := os.Stat(configPath); err == nil {
			return configPath
		}

		// 获取父目录
		parent := filepath.Dir(dir)
		if parent == dir {
			// 已到达根目录
			return ""
		}
		dir = parent
	}
}

// GetProjectRoot 获取项目根目录（配置文件所在目录）
func GetProjectRoot(startPath string) string {
	configPath := FindConfigFile(startPath)
	if configPath == "" {
		return ""
	}
	return filepath.Dir(configPath)
}
