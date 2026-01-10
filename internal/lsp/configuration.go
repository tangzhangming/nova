package lsp

import (
	"encoding/json"
	"sync"
)

// Configuration LSP 配置
type Configuration struct {
	// 格式化选项
	Formatting FormattingConfig `json:"formatting"`

	// 诊断选项
	Diagnostics DiagnosticsConfig `json:"diagnostics"`

	// 代码补全选项
	Completion CompletionConfig `json:"completion"`

	// 内联提示选项
	InlayHints InlayHintsConfig `json:"inlayHints"`

	// 代码镜头选项
	CodeLens CodeLensConfig `json:"codeLens"`

	// 语义高亮选项
	SemanticHighlighting SemanticHighlightingConfig `json:"semanticHighlighting"`

	// 工作区选项
	Workspace WorkspaceConfig `json:"workspace"`
}

// CompletionConfig 代码补全配置
type CompletionConfig struct {
	Enable               bool `json:"enable"`
	AutoImport           bool `json:"autoImport"`
	ShowDeprecated       bool `json:"showDeprecated"`
	ShowSnippets         bool `json:"showSnippets"`
	TriggerOnIdentifier  bool `json:"triggerOnIdentifier"`
}

// FormattingConfig 格式化配置
type FormattingConfig struct {
	TabSize               int  `json:"tabSize"`
	InsertSpaces          bool `json:"insertSpaces"`
	InsertFinalNewline    bool `json:"insertFinalNewline"`
	TrimFinalNewlines     bool `json:"trimFinalNewlines"`
	TrimTrailingWhitespace bool `json:"trimTrailingWhitespace"`
}

// DiagnosticsConfig 诊断配置
type DiagnosticsConfig struct {
	Enable              bool `json:"enable"`
	ShowWarnings        bool `json:"showWarnings"`
	ShowHints           bool `json:"showHints"`
	UnusedVariables     bool `json:"unusedVariables"`
	DeprecatedAPIs      bool `json:"deprecatedAPIs"`
	TypeMismatch        bool `json:"typeMismatch"`
}

// InlayHintsConfig 内联提示配置
type InlayHintsConfig struct {
	Enable           bool `json:"enable"`
	ParameterNames   bool `json:"parameterNames"`
	ParameterTypes   bool `json:"parameterTypes"`
	VariableTypes    bool `json:"variableTypes"`
	PropertyTypes    bool `json:"propertyTypes"`
	ReturnTypes      bool `json:"returnTypes"`
}

// CodeLensConfig 代码镜头配置
type CodeLensConfig struct {
	Enable          bool `json:"enable"`
	ShowReferences  bool `json:"showReferences"`
	ShowImplementations bool `json:"showImplementations"`
	ShowTests       bool `json:"showTests"`
}

// SemanticHighlightingConfig 语义高亮配置
type SemanticHighlightingConfig struct {
	Enable bool `json:"enable"`
}

// WorkspaceConfig 工作区配置
type WorkspaceConfig struct {
	MaxFileSize       int64    `json:"maxFileSize"`
	IndexOnStartup    bool     `json:"indexOnStartup"`
	IndexExclude      []string `json:"indexExclude"`
	WatchFileChanges  bool     `json:"watchFileChanges"`
}

// ConfigurationManager 配置管理器
type ConfigurationManager struct {
	config Configuration
	mu     sync.RWMutex
	server *Server
}

// NewConfigurationManager 创建配置管理器
func NewConfigurationManager(server *Server) *ConfigurationManager {
	return &ConfigurationManager{
		server: server,
		config: defaultConfiguration(),
	}
}

// defaultConfiguration 默认配置
func defaultConfiguration() Configuration {
	return Configuration{
		Formatting: FormattingConfig{
			TabSize:               4,
			InsertSpaces:          true,
			InsertFinalNewline:    true,
			TrimFinalNewlines:     true,
			TrimTrailingWhitespace: true,
		},
		Diagnostics: DiagnosticsConfig{
			Enable:          true,
			ShowWarnings:    true,
			ShowHints:       true,
			UnusedVariables: true,
			DeprecatedAPIs:  true,
			TypeMismatch:    true,
		},
		Completion: CompletionConfig{
			Enable:              true,
			AutoImport:          true,
			ShowDeprecated:      true,
			ShowSnippets:        true,
			TriggerOnIdentifier: true,
		},
		InlayHints: InlayHintsConfig{
			Enable:         true,
			ParameterNames: true,
			ParameterTypes: false,
			VariableTypes:  true,
			PropertyTypes:  false,
			ReturnTypes:    false,
		},
		CodeLens: CodeLensConfig{
			Enable:           true,
			ShowReferences:   true,
			ShowImplementations: true,
			ShowTests:        true,
		},
		SemanticHighlighting: SemanticHighlightingConfig{
			Enable: true,
		},
		Workspace: WorkspaceConfig{
			MaxFileSize:      5 * 1024 * 1024, // 5MB
			IndexOnStartup:   true,
			IndexExclude:     []string{"**/node_modules/**", "**/.git/**", "**/vendor/**"},
			WatchFileChanges: true,
		},
	}
}

// GetDefaultConfiguration 获取默认配置（公开方法，用于测试）
func GetDefaultConfiguration() Configuration {
	return defaultConfiguration()
}

// Get 获取配置
func (cm *ConfigurationManager) Get() Configuration {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.config
}

// Update 更新配置
func (cm *ConfigurationManager) Update(newConfig Configuration) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.config = newConfig
}

// UpdateFromJSON 从 JSON 更新配置
func (cm *ConfigurationManager) UpdateFromJSON(data json.RawMessage) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// 先用默认值
	cm.config = defaultConfiguration()

	// 覆盖传入的配置
	if err := json.Unmarshal(data, &cm.config); err != nil {
		return err
	}

	return nil
}

// GetFormatting 获取格式化配置
func (cm *ConfigurationManager) GetFormatting() FormattingConfig {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.config.Formatting
}

// GetDiagnostics 获取诊断配置
func (cm *ConfigurationManager) GetDiagnostics() DiagnosticsConfig {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.config.Diagnostics
}

// GetInlayHints 获取内联提示配置
func (cm *ConfigurationManager) GetInlayHints() InlayHintsConfig {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.config.InlayHints
}

// GetCodeLens 获取代码镜头配置
func (cm *ConfigurationManager) GetCodeLens() CodeLensConfig {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.config.CodeLens
}

// GetWorkspace 获取工作区配置
func (cm *ConfigurationManager) GetWorkspace() WorkspaceConfig {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.config.Workspace
}

// ShouldExclude 检查文件是否应被排除
func (cm *ConfigurationManager) ShouldExclude(path string) bool {
	cm.mu.RLock()
	excludes := cm.config.Workspace.IndexExclude
	cm.mu.RUnlock()

	for _, pattern := range excludes {
		if matchGlob(pattern, path) {
			return true
		}
	}
	return false
}

// matchGlob 简单的 glob 匹配
func matchGlob(pattern, path string) bool {
	// 简化实现：只支持 **/ 和 * 通配符
	if pattern == "" {
		return false
	}

	// 检查是否是 **/xxx/** 模式
	if len(pattern) > 4 && pattern[:3] == "**/" {
		// 检查路径是否包含模式中的目录名
		dirName := pattern[3:]
		if len(dirName) > 3 && dirName[len(dirName)-3:] == "/**" {
			dirName = dirName[:len(dirName)-3]
		}
		return containsDir(path, dirName)
	}

	return false
}

// containsDir 检查路径是否包含指定目录
func containsDir(path, dir string) bool {
	// 简单检查
	return len(path) >= len(dir) && (path == dir ||
		(len(path) > len(dir) && (path[len(path)-len(dir)-1] == '/' || path[len(path)-len(dir)-1] == '\\') && path[len(path)-len(dir):] == dir) ||
		containsSubstring(path, "/"+dir+"/") ||
		containsSubstring(path, "\\"+dir+"\\"))
}

// containsSubstring 检查字符串是否包含子串
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr) >= 0
}

// findSubstring 查找子串位置
func findSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// handleConfigurationChanged 处理配置变更通知
func (s *Server) handleConfigurationChanged(params json.RawMessage) {
	if s.configManager == nil {
		return
	}

	var settings struct {
		Settings json.RawMessage `json:"settings"`
	}

	if err := json.Unmarshal(params, &settings); err != nil {
		s.log("Error parsing configuration: %v", err)
		return
	}

	if err := s.configManager.UpdateFromJSON(settings.Settings); err != nil {
		s.log("Error updating configuration: %v", err)
		return
	}

	s.log("Configuration updated")

	// 触发重新诊断所有打开的文档
	for _, doc := range s.documents.GetAll() {
		s.publishDiagnostics(doc.URI)
	}
}
