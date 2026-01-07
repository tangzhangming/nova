package errors

import (
	"fmt"
	"strings"
)

// ============================================================================
// 错误标签
// ============================================================================

// Label 代码标签（用于标注错误位置）
type Label struct {
	Line    int    // 行号（1-based）
	Column  int    // 列号（1-based）
	Length  int    // 标注长度
	Message string // 标签消息
	Primary bool   // 是否为主要标签
}

// ============================================================================
// 编译错误
// ============================================================================

// CompileError 编译错误
type CompileError struct {
	Code      string   // 错误码 (E0200)
	Level     Level    // 错误级别
	Message   string   // 主消息
	File      string   // 文件路径
	Line      int      // 行号
	Column    int      // 列号
	EndColumn int      // 结束列
	Labels    []Label  // 代码标签
	Hints     []string // 修复建议
	Notes     []string // 附加说明
}

// Error 实现 error 接口
func (e *CompileError) Error() string {
	return fmt.Sprintf("%s:%d:%d: %s", e.File, e.Line, e.Column, e.Message)
}

// ============================================================================
// 运行时错误
// ============================================================================

// StackFrame 堆栈帧
type StackFrame struct {
	FunctionName string // 函数名
	ClassName    string // 类名（可选）
	FileName     string // 文件名
	LineNumber   int    // 行号
	SourceLine   string // 源代码行（可选）
}

// RuntimeError 运行时错误
type RuntimeError struct {
	Code       string                 // 错误码 (R0100)
	Level      Level                  // 错误级别
	Message    string                 // 主消息
	Context    map[string]interface{} // 上下文变量
	Frames     []StackFrame           // 堆栈帧
	Hints      []string               // 修复建议
	SourceLine string                 // 出错行源代码
	Column     int                    // 出错列
	Length     int                    // 标注长度
}

// Error 实现 error 接口
func (e *RuntimeError) Error() string {
	return e.Message
}

// ============================================================================
// 格式化器
// ============================================================================

// Formatter 错误格式化器
type Formatter struct {
	Colors      bool // 是否使用颜色
	ShowSource  bool // 是否显示源代码
	ShowHints   bool // 是否显示修复建议
	MaxContext  int  // 上下文行数（源代码前后行数）
	TabWidth    int  // Tab 宽度
}

// NewFormatter 创建默认格式化器
func NewFormatter() *Formatter {
	return &Formatter{
		Colors:     true,
		ShowSource: true,
		ShowHints:  true,
		MaxContext: 2,
		TabWidth:   4,
	}
}

// FormatCompileError 格式化编译错误
func (f *Formatter) FormatCompileError(err *CompileError, sourceLines []string) string {
	var sb strings.Builder

	// 错误头: error[E0200]: 类型不匹配
	levelStr := f.colorize(err.Level.String(), f.levelColor(err.Level))
	codeStr := f.colorize(fmt.Sprintf("[%s]", err.Code), f.levelColor(err.Level))
	sb.WriteString(fmt.Sprintf("%s%s: %s\n", levelStr, codeStr, err.Message))

	// 位置: --> file.sola:5:12
	arrow := f.colorize("-->", ColorCyan)
	location := f.colorize(fmt.Sprintf("%s:%d:%d", err.File, err.Line, err.Column), ColorCyan)
	sb.WriteString(fmt.Sprintf(" %s %s\n", arrow, location))

	// 显示源代码
	if f.ShowSource && len(sourceLines) > 0 && err.Line > 0 && err.Line <= len(sourceLines) {
		sb.WriteString(f.formatSourceContext(sourceLines, err.Line, err.Column, err.EndColumn, err.Labels))
	}

	// 修复建议
	if f.ShowHints {
		for _, hint := range err.Hints {
			hintLabel := f.colorize(" = help:", ColorCyan)
			sb.WriteString(fmt.Sprintf("%s %s\n", hintLabel, hint))
		}
	}

	// 附加说明
	for _, note := range err.Notes {
		noteLabel := f.colorize(" = note:", ColorCyan)
		sb.WriteString(fmt.Sprintf("%s %s\n", noteLabel, note))
	}

	return sb.String()
}

// FormatRuntimeError 格式化运行时错误
func (f *Formatter) FormatRuntimeError(err *RuntimeError, sourceCache map[string][]string) string {
	var sb strings.Builder

	// 异常类型和消息
	levelStr := f.colorize("RuntimeError", ColorRed)
	codeStr := f.colorize(fmt.Sprintf("[%s]", err.Code), ColorRed)
	sb.WriteString(fmt.Sprintf("%s%s: %s\n", levelStr, codeStr, err.Message))

	// 上下文信息
	if len(err.Context) > 0 {
		sb.WriteString("\n")
		for key, value := range err.Context {
			keyStr := f.colorize(fmt.Sprintf("  %s:", key), ColorYellow)
			sb.WriteString(fmt.Sprintf("%s %v\n", keyStr, value))
		}
	}

	// 堆栈跟踪
	if len(err.Frames) > 0 {
		sb.WriteString("\n")
		traceLabel := f.colorize("Stack trace:", ColorWhite)
		sb.WriteString(fmt.Sprintf("%s\n", traceLabel))

		for i, frame := range err.Frames {
			// 格式化堆栈帧
			funcName := frame.FunctionName
			if frame.ClassName != "" {
				funcName = frame.ClassName + "." + funcName
			}

			atStr := f.colorize("at", ColorWhite)
			funcStr := f.colorize(funcName, ColorYellow)

			if frame.FileName != "" {
				locStr := f.colorize(fmt.Sprintf("(%s:%d)", frame.FileName, frame.LineNumber), ColorCyan)
				sb.WriteString(fmt.Sprintf("    %s %s %s\n", atStr, funcStr, locStr))
			} else {
				locStr := f.colorize(fmt.Sprintf("(line %d)", frame.LineNumber), ColorCyan)
				sb.WriteString(fmt.Sprintf("    %s %s %s\n", atStr, funcStr, locStr))
			}

			// 显示第一帧的源代码
			if i == 0 && f.ShowSource && frame.FileName != "" {
				if lines, ok := sourceCache[frame.FileName]; ok && frame.LineNumber > 0 && frame.LineNumber <= len(lines) {
					sb.WriteString(f.formatSingleLine(lines[frame.LineNumber-1], frame.LineNumber, err.Column, err.Length))
				} else if err.SourceLine != "" {
					sb.WriteString(f.formatSingleLine(err.SourceLine, frame.LineNumber, err.Column, err.Length))
				}
			}
		}
	}

	// 修复建议
	if f.ShowHints && len(err.Hints) > 0 {
		sb.WriteString("\n")
		for _, hint := range err.Hints {
			hintLabel := f.colorize(" = help:", ColorCyan)
			sb.WriteString(fmt.Sprintf("%s %s\n", hintLabel, hint))
		}
	}

	return sb.String()
}

// formatSourceContext 格式化源代码上下文
func (f *Formatter) formatSourceContext(lines []string, errorLine, startCol, endCol int, labels []Label) string {
	var sb strings.Builder

	// 计算行号宽度
	maxLine := errorLine + f.MaxContext
	if maxLine > len(lines) {
		maxLine = len(lines)
	}
	lineNumWidth := len(fmt.Sprintf("%d", maxLine))

	// 空行分隔符
	separator := f.colorize(strings.Repeat(" ", lineNumWidth)+" |", ColorBlue)
	sb.WriteString(separator + "\n")

	// 显示错误行
	if errorLine > 0 && errorLine <= len(lines) {
		line := lines[errorLine-1]
		lineNum := f.colorize(fmt.Sprintf("%*d", lineNumWidth, errorLine), ColorBlue)
		pipe := f.colorize(" |", ColorBlue)
		sb.WriteString(fmt.Sprintf("%s%s %s\n", lineNum, pipe, f.expandTabs(line)))

		// 错误标注
		if endCol == 0 {
			endCol = startCol + 1
		}
		length := endCol - startCol
		if length < 1 {
			length = 1
		}

		// 计算实际的列位置（考虑 Tab）
		actualCol := f.calculateActualColumn(line, startCol)

		underline := strings.Repeat(" ", lineNumWidth+3+actualCol-1) +
			f.colorize(strings.Repeat("^", length), ColorRed)
		sb.WriteString(underline + "\n")
	}

	// 处理额外的标签
	for _, label := range labels {
		if label.Line != errorLine && label.Line > 0 && label.Line <= len(lines) {
			line := lines[label.Line-1]
			lineNum := f.colorize(fmt.Sprintf("%*d", lineNumWidth, label.Line), ColorBlue)
			pipe := f.colorize(" |", ColorBlue)
			sb.WriteString(fmt.Sprintf("%s%s %s\n", lineNum, pipe, f.expandTabs(line)))

			if label.Message != "" {
				actualCol := f.calculateActualColumn(line, label.Column)
				msgLine := strings.Repeat(" ", lineNumWidth+3+actualCol-1) +
					f.colorize(strings.Repeat("^", label.Length)+" "+label.Message, f.labelColor(label.Primary))
				sb.WriteString(msgLine + "\n")
			}
		}
	}

	return sb.String()
}

// formatSingleLine 格式化单行源代码
func (f *Formatter) formatSingleLine(line string, lineNum, col, length int) string {
	var sb strings.Builder

	lineNumWidth := len(fmt.Sprintf("%d", lineNum))

	// 空行
	separator := f.colorize(strings.Repeat(" ", lineNumWidth+3)+" |", ColorBlue)
	sb.WriteString(separator + "\n")

	// 源代码行
	lineNumStr := f.colorize(fmt.Sprintf("%*d", lineNumWidth, lineNum), ColorBlue)
	pipe := f.colorize(" |", ColorBlue)
	sb.WriteString(fmt.Sprintf("    %s%s %s\n", lineNumStr, pipe, f.expandTabs(line)))

	// 标注
	if col > 0 {
		if length < 1 {
			length = 1
		}
		actualCol := f.calculateActualColumn(line, col)
		underline := strings.Repeat(" ", lineNumWidth+7+actualCol-1) +
			f.colorize(strings.Repeat("^", length), ColorRed)
		sb.WriteString(underline + "\n")
	}

	// 空行
	sb.WriteString(separator + "\n")

	return sb.String()
}

// expandTabs 展开 Tab 为空格
func (f *Formatter) expandTabs(s string) string {
	return strings.ReplaceAll(s, "\t", strings.Repeat(" ", f.TabWidth))
}

// calculateActualColumn 计算实际列位置（考虑 Tab）
func (f *Formatter) calculateActualColumn(line string, col int) int {
	if col <= 0 {
		return 0
	}
	actual := 0
	for i := 0; i < col-1 && i < len(line); i++ {
		if line[i] == '\t' {
			actual += f.TabWidth
		} else {
			actual++
		}
	}
	return actual
}

// levelColor 获取错误级别对应的颜色
func (f *Formatter) levelColor(level Level) Color {
	switch level {
	case LevelError:
		return ColorRed
	case LevelWarning:
		return ColorYellow
	case LevelNote:
		return ColorCyan
	case LevelHelp:
		return ColorGreen
	default:
		return ColorWhite
	}
}

// labelColor 获取标签颜色
func (f *Formatter) labelColor(primary bool) Color {
	if primary {
		return ColorRed
	}
	return ColorYellow
}

// colorize 着色字符串
func (f *Formatter) colorize(s string, color Color) string {
	if !f.Colors {
		return s
	}
	return Colorize(s, color)
}

// ============================================================================
// 简便方法
// ============================================================================

// FormatCompileErrors 格式化多个编译错误
func (f *Formatter) FormatCompileErrors(errors []*CompileError, sourceCache map[string][]string) string {
	var sb strings.Builder

	for i, err := range errors {
		if i > 0 {
			sb.WriteString("\n")
		}

		var lines []string
		if sourceCache != nil {
			lines = sourceCache[err.File]
		}
		sb.WriteString(f.FormatCompileError(err, lines))
	}

	// 错误计数
	if len(errors) > 0 {
		sb.WriteString("\n")
		countMsg := fmt.Sprintf("错误: 发现 %d 个错误", len(errors))
		if len(errors) == 1 {
			countMsg = "错误: 发现 1 个错误"
		}
		sb.WriteString(f.colorize(countMsg, ColorRed) + "\n")
	}

	return sb.String()
}

// ============================================================================
// 全局格式化器
// ============================================================================

var defaultFormatter = NewFormatter()

// SetDefaultFormatter 设置默认格式化器
func SetDefaultFormatter(f *Formatter) {
	defaultFormatter = f
}

// GetDefaultFormatter 获取默认格式化器
func GetDefaultFormatter() *Formatter {
	return defaultFormatter
}

// Format 使用默认格式化器格式化编译错误
func Format(err *CompileError, sourceLines []string) string {
	return defaultFormatter.FormatCompileError(err, sourceLines)
}

// FormatRuntime 使用默认格式化器格式化运行时错误
func FormatRuntime(err *RuntimeError, sourceCache map[string][]string) string {
	return defaultFormatter.FormatRuntimeError(err, sourceCache)
}







