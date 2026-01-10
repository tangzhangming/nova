package lsp2

import (
	"strings"
	"unicode"
)

// isWordChar 判断字符是否是单词字符（字母、数字、下划线）
func isWordChar(c rune) bool {
	return unicode.IsLetter(c) || unicode.IsDigit(c) || c == '_'
}

// GetWordAt 获取指定位置的单词
// line: 行内容
// character: 字符位置（UTF-8字符索引）
// 返回: 单词内容、单词开始位置、单词结束位置
func GetWordAt(line string, character int) (word string, start int, end int) {
	runes := []rune(line)
	if character < 0 || character > len(runes) {
		return "", 0, 0
	}

	// 向前查找单词开始
	start = character
	for start > 0 && isWordChar(runes[start-1]) {
		start--
	}

	// 向后查找单词结束
	end = character
	for end < len(runes) && isWordChar(runes[end]) {
		end++
	}

	if start >= end {
		return "", 0, 0
	}

	return string(runes[start:end]), start, end
}

// GetContextBefore 获取指定位置之前的上下文
// 用于判断是否在特殊语法位置（如 :: 或 ->）
func GetContextBefore(line string, character int) string {
	runes := []rune(line)
	if character <= 0 || character > len(runes) {
		return ""
	}

	// 获取前面最多10个字符作为上下文
	contextStart := character - 10
	if contextStart < 0 {
		contextStart = 0
	}

	return string(runes[contextStart:character])
}

// CheckStaticCall 检查是否是静态调用 (Class::method)
// 返回: 类名, 是否是静态调用
func CheckStaticCall(line string, character int) (className string, isStatic bool) {
	context := GetContextBefore(line, character)
	
	// 查找 "::"
	idx := strings.LastIndex(context, "::")
	if idx < 0 {
		return "", false
	}

	// 提取类名（:: 之前的标识符）
	beforeDoubleColon := context[:idx]
	beforeDoubleColon = strings.TrimSpace(beforeDoubleColon)
	
	// 从后往前找到类名的开始
	classStart := len(beforeDoubleColon) - 1
	for classStart >= 0 && isWordChar(rune(beforeDoubleColon[classStart])) {
		classStart--
	}
	classStart++

	if classStart < len(beforeDoubleColon) {
		className = beforeDoubleColon[classStart:]
		return className, true
	}

	return "", false
}

// CheckInstanceCall 检查是否是实例方法调用 ($obj->method)
// 返回: 变量名（不含$）, 是否是实例调用
func CheckInstanceCall(line string, character int) (varName string, isInstance bool) {
	context := GetContextBefore(line, character)
	
	// 查找 "->"
	idx := strings.LastIndex(context, "->")
	if idx < 0 {
		return "", false
	}

	// 提取变量名（-> 之前的标识符，应该以 $ 开头）
	beforeArrow := context[:idx]
	beforeArrow = strings.TrimSpace(beforeArrow)
	
	// 从后往前找到变量名的开始（应该是 $）
	varStart := len(beforeArrow) - 1
	for varStart >= 0 && (isWordChar(rune(beforeArrow[varStart])) || beforeArrow[varStart] == '$') {
		varStart--
	}
	varStart++

	if varStart < len(beforeArrow) && beforeArrow[varStart] == '$' {
		// 去掉 $ 符号
		varName = beforeArrow[varStart+1:]
		return varName, true
	}

	return "", false
}

// uriToPath 将 URI 转换为文件路径
func uriToPath(uri string) string {
	// 处理 file:/// 前缀
	path := uri
	if strings.HasPrefix(path, "file:///") {
		path = strings.TrimPrefix(path, "file:///")
		// Windows 路径处理：file:///D:/path -> D:/path
		// Unix 路径处理：file:///path -> /path
		if len(path) > 2 && path[1] == ':' {
			// Windows 路径，已经正确
		} else {
			// Unix 路径，需要加回前导斜杠
			path = "/" + path
		}
	}
	
	// 将 URL 编码的斜杠转换回来
	path = strings.ReplaceAll(path, "%20", " ")
	path = strings.ReplaceAll(path, "%2F", "/")
	path = strings.ReplaceAll(path, "%5C", "\\")
	
	return path
}

// pathToURI 将文件路径转换为 URI
func pathToURI(path string) string {
	// 规范化路径分隔符
	path = strings.ReplaceAll(path, "\\", "/")
	
	// 如果没有 file:// 前缀，添加它
	if !strings.HasPrefix(path, "file://") {
		// Windows 路径：D:/path -> file:///D:/path
		// Unix 路径：/path -> file:///path
		if len(path) > 2 && path[1] == ':' {
			path = "file:///" + path
		} else if strings.HasPrefix(path, "/") {
			path = "file://" + path
		} else {
			path = "file:///" + path
		}
	}
	
	return path
}

// SplitLines 将内容按行分割
func SplitLines(content string) []string {
	// 处理不同的换行符
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	return strings.Split(content, "\n")
}
