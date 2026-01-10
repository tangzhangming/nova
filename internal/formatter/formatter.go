package formatter

import (
	"strings"

	"github.com/tangzhangming/nova/internal/parser"
)

// Format 格式化源代码
func Format(source, filename string, options *Options) (string, error) {
	// 解析源代码
	p := parser.New(source, filename)
	file := p.Parse()

	if p.HasErrors() {
		return "", p.Errors()[0]
	}

	// 使用打印器生成格式化的代码
	printer := NewPrinter(options)
	formatted := printer.Print(file)

	return formatted, nil
}

// FormatWithDefaultOptions 使用默认选项格式化
func FormatWithDefaultOptions(source, filename string) (string, error) {
	return Format(source, filename, DefaultOptions())
}

// FormatPartial 格式化部分代码
// baseIndent 是基础缩进级别（空格数）
func FormatPartial(source, filename string, options *Options, baseIndent int) (string, error) {
	// 首先尝试直接格式化
	formatted, err := Format(source, filename, options)
	if err == nil {
		return formatted, nil
	}

	// 如果直接格式化失败，尝试包装代码后格式化
	// 这样可以处理不完整的代码片段
	wrapped := wrapPartialCode(source)
	formatted, err = Format(wrapped, filename, options)
	if err != nil {
		// 如果仍然失败，返回原始错误
		return "", err
	}

	// 从格式化后的代码中提取原始部分
	extracted := extractFormattedPart(formatted, baseIndent, options)
	return extracted, nil
}

// wrapPartialCode 包装部分代码以便解析
func wrapPartialCode(source string) string {
	// 尝试用函数体包装
	return "function __wrapper__() {\n" + source + "\n}"
}

// extractFormattedPart 从包装的格式化代码中提取原始部分
func extractFormattedPart(formatted string, baseIndent int, options *Options) string {
	lines := strings.Split(formatted, "\n")
	if len(lines) < 3 {
		return formatted
	}

	// 跳过第一行（function __wrapper__() {）和最后一行（}）
	resultLines := lines[1 : len(lines)-1]

	// 移除包装函数添加的额外缩进
	indentStr := options.IndentString()
	var result []string
	for _, line := range resultLines {
		// 移除一级缩进（包装函数添加的）
		if strings.HasPrefix(line, indentStr) {
			line = line[len(indentStr):]
		}
		result = append(result, line)
	}

	return strings.Join(result, "\n")
}

