package main

import (
	"os"
	"runtime"
	"strings"
)

// Language 语言类型
type Language string

const (
	LangEnglish Language = "en"
	LangChinese Language = "zh"
)

// Messages 消息结构
type Messages struct {
	// 版本信息
	VersionTitle string
	VersionDesc  string

	// 帮助信息
	HelpUsage    string
	HelpCommands string
	HelpOptions  string
	HelpExamples string

	// 命令描述
	CmdRun     string
	CmdBuild   string
	CmdJvm     string
	CmdCheck   string
	CmdFormat  string
	CmdVersion string
	CmdHelp    string

	// 运行选项
	OptTokens   string
	OptAST      string
	OptBytecode string
	OptJitless  string
	OptOutput   string
	OptVerbose  string
	OptLang     string

	// 格式化选项
	OptFormatWrite      string
	OptFormatCheck      string
	OptFormatIndent     string
	OptFormatIndentSize string
	OptFormatMaxLine    string

	// 错误信息
	ErrNoInput         string
	ErrReadFile        string
	ErrUnknownCmd      string
	ErrLexer           string
	ErrParser          string
	ErrRuntime         string
	ErrInvalidSourceFile string
	ErrCompileFailed   string
	ErrSerializeFailed string
	ErrWriteFile       string
	ErrInvalidCompiledFile string
	ErrDeserializeFailed   string
	ErrFormatFailed        string
	ErrFormatNotFormatted  string
	ErrJvmGenFailed        string

	// 成功信息
	SuccessSyntaxOK      string
	SuccessBuilding      string
	SuccessBuildComplete string
	SuccessFormatOK       string
	SuccessFormatComplete string
	SuccessJvmComplete    string

	// 其他
	NotImplemented string
	Namespace      string
	Uses           string
	Declarations   string
	Statements     string
}

// 英文消息
var messagesEN = Messages{
	VersionTitle: "Sola Programming Language v%s",
	VersionDesc:  "A statically-typed, compiled language with PHP-like syntax",

	HelpUsage:    "Usage:",
	HelpCommands: "Commands:",
	HelpOptions:  "Run Options:",
	HelpExamples: "Examples:",

	CmdRun:     "Run a Sola source file or compiled bytecode",
	CmdBuild:   "Compile to bytecode",
	CmdJvm:     "Compile to JVM bytecode (.class file)",
	CmdCheck:   "Check syntax without running",
	CmdFormat:  "Format source code",
	CmdVersion: "Show version information",
	CmdHelp:    "Show this help message",

	OptTokens:   "Show lexer tokens",
	OptAST:      "Show AST structure",
	OptBytecode: "Show compiled bytecode",
	OptJitless:  "Disable JIT compilation, interpret only (like JVM -Xint)",
	OptOutput:   "Output file path",
	OptVerbose:  "Verbose output",
	OptLang:     "Set language (en/zh)",

	OptFormatWrite:      "Write result to file instead of stdout",
	OptFormatCheck:      "Check if files are formatted (exit code 1 if not)",
	OptFormatIndent:     "Indent style: tabs or spaces",
	OptFormatIndentSize: "Indent size (when using spaces)",
	OptFormatMaxLine:    "Maximum line length",

	ErrNoInput:            "Error: no input file specified",
	ErrReadFile:           "Error reading file: %v",
	ErrUnknownCmd:         "Unknown command: %s",
	ErrLexer:              "Lexer errors:",
	ErrParser:             "Parser errors:",
	ErrRuntime:            "Error: %v",
	ErrInvalidSourceFile:  "Error: %s is not a valid source file (expected %s)",
	ErrCompileFailed:      "Compilation failed",
	ErrSerializeFailed:    "Serialization failed",
	ErrWriteFile:          "Error writing file",
	ErrInvalidCompiledFile: "Error: %s is not a valid compiled file (expected %s)",
	ErrDeserializeFailed:   "Failed to load compiled file",
	ErrFormatFailed:        "Formatting failed",
	ErrFormatNotFormatted:  "%s: not formatted",
	ErrJvmGenFailed:        "JVM bytecode generation failed",

	SuccessSyntaxOK:       "✓ %s: syntax OK",
	SuccessBuilding:       "Building %s...",
	SuccessBuildComplete:  "✓ Built %s (%d bytes)",
	SuccessFormatOK:       "✓ %s: already formatted",
	SuccessFormatComplete: "✓ Formatted: %s",
	SuccessJvmComplete:    "✓ Generated JVM class: %s (%d bytes)",

	NotImplemented: "Note: Build command is not yet implemented. Coming soon!",
	Namespace:      "Namespace",
	Uses:           "Uses",
	Declarations:   "Declarations",
	Statements:     "Statements",
}

// 中文消息
var messagesZH = Messages{
	VersionTitle: "Sola 编程语言 v%s",
	VersionDesc:  "一门静态类型、编译型语言，语法类似 PHP",

	HelpUsage:    "用法:",
	HelpCommands: "命令:",
	HelpOptions:  "运行选项:",
	HelpExamples: "示例:",

	CmdRun:     "运行 Sola 源文件或编译后的字节码",
	CmdBuild:   "编译为字节码",
	CmdJvm:     "编译为 JVM 字节码（.class 文件）",
	CmdCheck:   "检查语法，不运行",
	CmdFormat:  "格式化源代码",
	CmdVersion: "显示版本信息",
	CmdHelp:    "显示帮助信息",

	OptTokens:   "显示词法分析结果",
	OptAST:      "显示抽象语法树",
	OptBytecode: "显示编译后的字节码",
	OptJitless:  "禁用 JIT 编译，仅使用解释器（类似 JVM -Xint）",
	OptOutput:   "输出文件路径",
	OptVerbose:  "详细输出",
	OptLang:     "设置语言 (en/zh)",

	OptFormatWrite:      "将结果写入文件而不是标准输出",
	OptFormatCheck:      "检查文件是否已格式化（未格式化则退出码为 1）",
	OptFormatIndent:     "缩进风格：tabs 或 spaces",
	OptFormatIndentSize: "缩进大小（使用 spaces 时）",
	OptFormatMaxLine:    "最大行长度",

	ErrNoInput:            "错误: 未指定输入文件",
	ErrReadFile:           "读取文件错误: %v",
	ErrUnknownCmd:         "未知命令: %s",
	ErrLexer:              "词法分析错误:",
	ErrParser:             "语法分析错误:",
	ErrRuntime:            "运行时错误: %v",
	ErrInvalidSourceFile:  "错误: %s 不是有效的源文件（应为 %s）",
	ErrCompileFailed:      "编译失败",
	ErrSerializeFailed:    "序列化失败",
	ErrWriteFile:          "写入文件错误",
	ErrInvalidCompiledFile: "错误: %s 不是有效的编译文件（应为 %s）",
	ErrDeserializeFailed:   "加载编译文件失败",
	ErrFormatFailed:        "格式化失败",
	ErrFormatNotFormatted:  "%s: 未格式化",
	ErrJvmGenFailed:        "JVM 字节码生成失败",

	SuccessSyntaxOK:       "✓ %s: 语法正确",
	SuccessBuilding:       "正在编译 %s...",
	SuccessBuildComplete:  "✓ 编译完成 %s (%d 字节)",
	SuccessFormatOK:       "✓ %s: 已格式化",
	SuccessFormatComplete: "✓ 已格式化: %s",
	SuccessJvmComplete:    "✓ 已生成 JVM 类: %s (%d 字节)",

	NotImplemented: "提示: 编译功能尚未实现，敬请期待！",
	Namespace:      "命名空间",
	Uses:           "导入",
	Declarations:   "声明",
	Statements:     "语句",
}

// 当前消息
var msg = messagesEN

// 当前语言
var currentLang = LangEnglish

// InitLanguage 初始化语言设置
// 优先级: 命令行参数 > 环境变量 SOLA_LANG > 操作系统语言 > 默认英文
func InitLanguage(langOverride string) {
	// 1. 命令行参数优先
	if langOverride != "" {
		setLanguage(langOverride)
		return
	}

	// 2. 检查环境变量
	if envLang := os.Getenv("SOLA_LANG"); envLang != "" {
		setLanguage(envLang)
		return
	}

	// 3. 检测操作系统语言
	if detectChineseOS() {
		setLanguage("zh")
		return
	}

	// 4. 默认英文
	setLanguage("en")
}

// setLanguage 设置语言
func setLanguage(lang string) {
	lang = strings.ToLower(strings.TrimSpace(lang))
	switch lang {
	case "zh", "zh-cn", "zh-tw", "zh-hk", "chinese":
		currentLang = LangChinese
		msg = messagesZH
	default:
		currentLang = LangEnglish
		msg = messagesEN
	}
}

// detectChineseOS 检测操作系统是否为中文环境
func detectChineseOS() bool {
	// Windows 使用 API 检测
	if runtime.GOOS == "windows" {
		// 优先使用 Windows API
		if detectWindowsChinese() {
			return true
		}
		// 备用：检查 locale 名称
		locale := getWindowsLocale()
		if strings.HasPrefix(strings.ToLower(locale), "zh") {
			return true
		}
	}

	// Unix/Linux/Mac: 检查环境变量
	langVars := []string{"LANG", "LANGUAGE", "LC_ALL", "LC_MESSAGES"}
	for _, v := range langVars {
		if val := os.Getenv(v); val != "" {
			lower := strings.ToLower(val)
			if strings.Contains(lower, "zh") ||
				strings.Contains(lower, "chinese") {
				return true
			}
		}
	}

	return false
}

// GetLanguage 获取当前语言
func GetLanguage() Language {
	return currentLang
}

// Msg 获取当前消息对象
func Msg() *Messages {
	return &msg
}
