package formatter

import (
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

