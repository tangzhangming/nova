package errors

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// ============================================================================
// 错误报告器
// ============================================================================

// Reporter 错误报告器
type Reporter struct {
	formatter   *Formatter
	sourceCache map[string][]string // 源代码缓存
	errors      []*CompileError
	warnings    []*CompileError
}

// NewReporter 创建错误报告器
func NewReporter() *Reporter {
	return &Reporter{
		formatter:   NewFormatter(),
		sourceCache: make(map[string][]string),
		errors:      nil,
		warnings:    nil,
	}
}

// SetFormatter 设置格式化器
func (r *Reporter) SetFormatter(f *Formatter) {
	r.formatter = f
}

// LoadSource 加载源文件
func (r *Reporter) LoadSource(filename string) error {
	if _, ok := r.sourceCache[filename]; ok {
		return nil // 已加载
	}

	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	r.sourceCache[filename] = lines
	return nil
}

// SetSource 设置源代码（用于测试或内存中的源代码）
func (r *Reporter) SetSource(filename string, content string) {
	lines := strings.Split(content, "\n")
	r.sourceCache[filename] = lines
}

// GetSourceLine 获取源代码行
func (r *Reporter) GetSourceLine(filename string, line int) string {
	if lines, ok := r.sourceCache[filename]; ok {
		if line > 0 && line <= len(lines) {
			return lines[line-1]
		}
	}
	return ""
}

// GetSourceLines 获取源代码行数组
func (r *Reporter) GetSourceLines(filename string) []string {
	return r.sourceCache[filename]
}

// ============================================================================
// 报告编译错误
// ============================================================================

// ReportError 报告编译错误
func (r *Reporter) ReportError(err *CompileError) {
	// 尝试加载源文件
	r.LoadSource(err.File)

	// 生成修复建议
	if len(err.Hints) == 0 {
		context := map[string]interface{}{
			"file": err.File,
			"line": err.Line,
		}
		err.Hints = GetSuggestions(err.Code, context)
	}

	r.errors = append(r.errors, err)

	// 输出格式化的错误
	lines := r.GetSourceLines(err.File)
	output := r.formatter.FormatCompileError(err, lines)
	fmt.Print(output)
}

// ReportWarning 报告警告
func (r *Reporter) ReportWarning(err *CompileError) {
	err.Level = LevelWarning
	r.warnings = append(r.warnings, err)

	lines := r.GetSourceLines(err.File)
	output := r.formatter.FormatCompileError(err, lines)
	fmt.Print(output)
}

// ReportSimple 报告简单错误（从现有错误信息转换）
func (r *Reporter) ReportSimple(file string, line, col int, message string) {
	// 尝试加载源文件
	r.LoadSource(file)

	err := &CompileError{
		Code:    E0001, // 默认语法错误
		Level:   LevelError,
		Message: message,
		File:    file,
		Line:    line,
		Column:  col,
	}

	// 尝试匹配错误码
	err.Code = r.inferErrorCode(message)

	r.ReportError(err)
}

// inferErrorCode 从错误消息推断错误码
func (r *Reporter) inferErrorCode(message string) string {
	msg := strings.ToLower(message)

	// 变量错误
	if strings.Contains(msg, "未定义的变量") || strings.Contains(msg, "undefined variable") {
		return E0100
	}
	if strings.Contains(msg, "已声明") || strings.Contains(msg, "already declared") {
		return E0101
	}
	if strings.Contains(msg, "未声明") || strings.Contains(msg, "not declared") {
		return E0102
	}

	// 类型错误
	if strings.Contains(msg, "类型不匹配") || strings.Contains(msg, "type mismatch") {
		return E0200
	}
	if strings.Contains(msg, "无法推断") || strings.Contains(msg, "cannot infer") {
		return E0201
	}
	if strings.Contains(msg, "不能将") || strings.Contains(msg, "cannot assign") {
		return E0202
	}

	// 函数错误
	if strings.Contains(msg, "未定义的函数") || strings.Contains(msg, "undefined function") {
		return E0300
	}
	if strings.Contains(msg, "至少") || strings.Contains(msg, "at least") {
		return E0301
	}
	if strings.Contains(msg, "最多") || strings.Contains(msg, "at most") {
		return E0302
	}
	if strings.Contains(msg, "break") {
		return E0304
	}
	if strings.Contains(msg, "continue") {
		return E0305
	}

	// 类错误
	if strings.Contains(msg, "未定义的类") || strings.Contains(msg, "undefined class") {
		return E0400
	}
	if strings.Contains(msg, "没有方法") || strings.Contains(msg, "no method") {
		return E0401
	}
	if strings.Contains(msg, "没有属性") || strings.Contains(msg, "no property") {
		return E0402
	}

	// 泛型错误
	if strings.Contains(msg, "约束") || strings.Contains(msg, "constraint") {
		return E0500
	}
	if strings.Contains(msg, "类型参数") || strings.Contains(msg, "type argument") {
		return E0501
	}

	return E0001 // 默认
}

// ============================================================================
// 报告运行时错误
// ============================================================================

// ReportRuntimeError 报告运行时错误
func (r *Reporter) ReportRuntimeError(err *RuntimeError) {
	// 尝试加载源文件
	for _, frame := range err.Frames {
		if frame.FileName != "" {
			r.LoadSource(frame.FileName)
		}
	}

	// 生成修复建议
	if len(err.Hints) == 0 {
		err.Hints = GetSuggestions(err.Code, err.Context)
	}

	// 输出格式化的错误
	output := r.formatter.FormatRuntimeError(err, r.sourceCache)
	fmt.Print(output)
}

// ReportRuntimeSimple 报告简单运行时错误
func (r *Reporter) ReportRuntimeSimple(message string, frames []StackFrame) {
	err := &RuntimeError{
		Code:    R0001,
		Level:   LevelError,
		Message: message,
		Frames:  frames,
	}

	// 推断错误码
	err.Code = r.inferRuntimeErrorCode(message)

	r.ReportRuntimeError(err)
}

// inferRuntimeErrorCode 从错误消息推断运行时错误码
func (r *Reporter) inferRuntimeErrorCode(message string) string {
	msg := strings.ToLower(message)

	// 数组错误
	if strings.Contains(msg, "索引") || strings.Contains(msg, "index") {
		return R0100
	}

	// 数值错误
	if strings.Contains(msg, "除") && strings.Contains(msg, "零") {
		return R0200
	}
	if strings.Contains(msg, "division") && strings.Contains(msg, "zero") {
		return R0200
	}
	if strings.Contains(msg, "数字") || strings.Contains(msg, "number") {
		return R0201
	}

	// 类型错误
	if strings.Contains(msg, "转换") || strings.Contains(msg, "cast") {
		return R0301
	}
	if strings.Contains(msg, "对象") || strings.Contains(msg, "object") {
		return R0302
	}

	// 资源错误
	if strings.Contains(msg, "栈溢出") || strings.Contains(msg, "stack overflow") {
		return R0400
	}
	if strings.Contains(msg, "死循环") || strings.Contains(msg, "infinite loop") {
		return R0401
	}

	return R0001
}

// ============================================================================
// 状态查询
// ============================================================================

// HasErrors 是否有错误
func (r *Reporter) HasErrors() bool {
	return len(r.errors) > 0
}

// HasWarnings 是否有警告
func (r *Reporter) HasWarnings() bool {
	return len(r.warnings) > 0
}

// ErrorCount 错误数量
func (r *Reporter) ErrorCount() int {
	return len(r.errors)
}

// WarningCount 警告数量
func (r *Reporter) WarningCount() int {
	return len(r.warnings)
}

// Errors 获取所有错误
func (r *Reporter) Errors() []*CompileError {
	return r.errors
}

// Warnings 获取所有警告
func (r *Reporter) Warnings() []*CompileError {
	return r.warnings
}

// Clear 清空错误和警告
func (r *Reporter) Clear() {
	r.errors = nil
	r.warnings = nil
}

// ============================================================================
// 全局报告器
// ============================================================================

var defaultReporter = NewReporter()

// GetDefaultReporter 获取默认报告器
func GetDefaultReporter() *Reporter {
	return defaultReporter
}

// SetDefaultReporter 设置默认报告器
func SetDefaultReporter(r *Reporter) {
	defaultReporter = r
}

// Report 使用默认报告器报告编译错误
func Report(err *CompileError) {
	defaultReporter.ReportError(err)
}

// ReportCompileError 报告编译错误（简化版）
func ReportCompileError(file string, line, col int, message string) {
	defaultReporter.ReportSimple(file, line, col, message)
}

// ReportRuntimeError 报告运行时错误
func ReportRuntimeErr(err *RuntimeError) {
	defaultReporter.ReportRuntimeError(err)
}






