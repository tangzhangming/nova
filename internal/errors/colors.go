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






