package errors

import (
	"os"
	"runtime"
	"strings"
)

// Color 终端颜色
type Color int

const (
	ColorReset Color = iota
	ColorRed
	ColorGreen
	ColorYellow
	ColorBlue
	ColorMagenta
	ColorCyan
	ColorWhite
	ColorBoldRed
	ColorBoldGreen
	ColorBoldYellow
	ColorBoldBlue
	ColorBoldMagenta
	ColorBoldCyan
	ColorBoldWhite
)

// ANSI 颜色代码
var ansiCodes = map[Color]string{
	ColorReset:       "\033[0m",
	ColorRed:         "\033[31m",
	ColorGreen:       "\033[32m",
	ColorYellow:      "\033[33m",
	ColorBlue:        "\033[34m",
	ColorMagenta:     "\033[35m",
	ColorCyan:        "\033[36m",
	ColorWhite:       "\033[37m",
	ColorBoldRed:     "\033[1;31m",
	ColorBoldGreen:   "\033[1;32m",
	ColorBoldYellow:  "\033[1;33m",
	ColorBoldBlue:    "\033[1;34m",
	ColorBoldMagenta: "\033[1;35m",
	ColorBoldCyan:    "\033[1;36m",
	ColorBoldWhite:   "\033[1;37m",
}

// colorsEnabled 是否启用颜色
var colorsEnabled = detectColorSupport()

// detectColorSupport 检测终端是否支持颜色
func detectColorSupport() bool {
	// Windows 需要特殊处理
	if runtime.GOOS == "windows" {
		// Windows 10 1511+ 支持 ANSI
		// 检查 TERM 环境变量
		term := os.Getenv("TERM")
		if term != "" && term != "dumb" {
			return true
		}
		// 检查 WT_SESSION（Windows Terminal）
		if os.Getenv("WT_SESSION") != "" {
			return true
		}
		// 检查 ConEmu
		if os.Getenv("ConEmuANSI") == "ON" {
			return true
		}
		// 检查 ANSICON
		if os.Getenv("ANSICON") != "" {
			return true
		}
		// 检查是否启用了虚拟终端处理
		// 默认在新版 Windows 上启用
		return true
	}

	// Unix-like 系统
	// 检查 NO_COLOR 环境变量
	if os.Getenv("NO_COLOR") != "" {
		return false
	}

	// 检查 TERM
	term := os.Getenv("TERM")
	if term == "dumb" {
		return false
	}

	// 检查是否为 TTY
	if fileInfo, err := os.Stdout.Stat(); err == nil {
		if (fileInfo.Mode() & os.ModeCharDevice) != 0 {
			return true
		}
	}

	// 检查 COLORTERM
	if os.Getenv("COLORTERM") != "" {
		return true
	}

	// 检查常见的支持颜色的终端
	colorTerms := []string{"xterm", "screen", "vt100", "linux", "ansi", "cygwin"}
	for _, ct := range colorTerms {
		if strings.Contains(strings.ToLower(term), ct) {
			return true
		}
	}

	return false
}

// EnableColors 启用颜色
func EnableColors() {
	colorsEnabled = true
}

// DisableColors 禁用颜色
func DisableColors() {
	colorsEnabled = false
}

// ColorsEnabled 检查颜色是否启用
func ColorsEnabled() bool {
	return colorsEnabled
}

// SetColorsEnabled 设置颜色启用状态
func SetColorsEnabled(enabled bool) {
	colorsEnabled = enabled
}

// Colorize 着色字符串
func Colorize(s string, color Color) string {
	if !colorsEnabled {
		return s
	}
	code, ok := ansiCodes[color]
	if !ok {
		return s
	}
	return code + s + ansiCodes[ColorReset]
}

// Red 红色
func Red(s string) string {
	return Colorize(s, ColorRed)
}

// Green 绿色
func Green(s string) string {
	return Colorize(s, ColorGreen)
}

// Yellow 黄色
func Yellow(s string) string {
	return Colorize(s, ColorYellow)
}

// Blue 蓝色
func Blue(s string) string {
	return Colorize(s, ColorBlue)
}

// Magenta 洋红色
func Magenta(s string) string {
	return Colorize(s, ColorMagenta)
}

// Cyan 青色
func Cyan(s string) string {
	return Colorize(s, ColorCyan)
}

// White 白色
func White(s string) string {
	return Colorize(s, ColorWhite)
}

// BoldRed 加粗红色
func BoldRed(s string) string {
	return Colorize(s, ColorBoldRed)
}

// BoldGreen 加粗绿色
func BoldGreen(s string) string {
	return Colorize(s, ColorBoldGreen)
}

// BoldYellow 加粗黄色
func BoldYellow(s string) string {
	return Colorize(s, ColorBoldYellow)
}

// BoldBlue 加粗蓝色
func BoldBlue(s string) string {
	return Colorize(s, ColorBoldBlue)
}

// BoldCyan 加粗青色
func BoldCyan(s string) string {
	return Colorize(s, ColorBoldCyan)
}

// BoldWhite 加粗白色
func BoldWhite(s string) string {
	return Colorize(s, ColorBoldWhite)
}

// Strip 移除 ANSI 颜色代码
func Strip(s string) string {
	result := s
	for _, code := range ansiCodes {
		result = strings.ReplaceAll(result, code, "")
	}
	return result
}

// ============================================================================
// 代码语法高亮
// ============================================================================

// SyntaxHighlighter 代码语法高亮器
type SyntaxHighlighter struct {
	enabled bool
}

// NewSyntaxHighlighter 创建语法高亮器
func NewSyntaxHighlighter() *SyntaxHighlighter {
	return &SyntaxHighlighter{enabled: colorsEnabled}
}

// 关键字列表
var keywords = map[string]bool{
	"if": true, "else": true, "elseif": true, "while": true, "for": true, "foreach": true,
	"switch": true, "case": true, "default": true, "break": true, "continue": true, "return": true,
	"function": true, "class": true, "interface": true, "extends": true, "implements": true,
	"public": true, "private": true, "protected": true, "static": true, "final": true, "abstract": true,
	"new": true, "try": true, "catch": true, "finally": true, "throw": true,
	"use": true, "namespace": true, "const": true, "enum": true,
	"true": true, "false": true, "null": true, "self": true, "parent": true,
	"as": true, "is": true, "match": true, "where": true, "type": true,
}

// 类型关键字
var typeKeywords = map[string]bool{
	"int": true, "float": true, "string": true, "bool": true, "void": true,
	"i8": true, "i16": true, "i32": true, "i64": true,
	"u8": true, "u16": true, "u32": true, "u64": true,
	"f32": true, "f64": true, "byte": true, "uint": true,
	"object": true, "func": true, "map": true,
}

// HighlightLine 高亮代码行
func (h *SyntaxHighlighter) HighlightLine(line string) string {
	if !h.enabled || !colorsEnabled {
		return line
	}
	return h.highlightTokens(line)
}

// highlightTokens 对源代码行进行 token 级别的高亮
func (h *SyntaxHighlighter) highlightTokens(line string) string {
	var result strings.Builder
	i := 0
	n := len(line)

	for i < n {
		ch := line[i]

		// 跳过空白
		if ch == ' ' || ch == '\t' {
			result.WriteByte(ch)
			i++
			continue
		}

		// 字符串
		if ch == '"' || ch == '\'' {
			quote := ch
			start := i
			i++
			for i < n && line[i] != quote {
				if line[i] == '\\' && i+1 < n {
					i++
				}
				i++
			}
			if i < n {
				i++ // 包含结束引号
			}
			result.WriteString(Colorize(line[start:i], ColorGreen))
			continue
		}

		// 注释
		if ch == '/' && i+1 < n {
			if line[i+1] == '/' {
				result.WriteString(Colorize(line[i:], ColorWhite))
				break
			}
		}

		// 变量 $xxx
		if ch == '$' {
			start := i
			i++
			for i < n && (isAlphaNumeric(line[i]) || line[i] == '_') {
				i++
			}
			result.WriteString(Colorize(line[start:i], ColorCyan))
			continue
		}

		// 数字
		if isDigit(ch) {
			start := i
			for i < n && (isDigit(line[i]) || line[i] == '.' || line[i] == 'x' || line[i] == 'X' ||
				(line[i] >= 'a' && line[i] <= 'f') || (line[i] >= 'A' && line[i] <= 'F')) {
				i++
			}
			result.WriteString(Colorize(line[start:i], ColorMagenta))
			continue
		}

		// 标识符/关键字
		if isAlpha(ch) {
			start := i
			for i < n && (isAlphaNumeric(line[i]) || line[i] == '_') {
				i++
			}
			word := line[start:i]
			if keywords[word] {
				result.WriteString(Colorize(word, ColorYellow))
			} else if typeKeywords[word] {
				result.WriteString(Colorize(word, ColorBlue))
			} else {
				result.WriteString(word)
			}
			continue
		}

		// 运算符
		if isOperator(ch) {
			result.WriteString(Colorize(string(ch), ColorRed))
			i++
			continue
		}

		// 其他字符
		result.WriteByte(ch)
		i++
	}

	return result.String()
}

func isAlpha(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_'
}

func isDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

func isAlphaNumeric(ch byte) bool {
	return isAlpha(ch) || isDigit(ch)
}

func isOperator(ch byte) bool {
	return ch == '+' || ch == '-' || ch == '*' || ch == '/' || ch == '%' ||
		ch == '=' || ch == '<' || ch == '>' || ch == '!' || ch == '&' || ch == '|' ||
		ch == '^' || ch == '~'
}







