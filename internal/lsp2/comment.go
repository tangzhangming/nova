package lsp2

import (
	"strings"
)

// extractCommentAboveLine 从源代码行中提取指定行上方的注释
// 不依赖AST，直接从源代码文本读取
// targetLine 是1-indexed的行号（与AST中的行号一致）
func extractCommentAboveLine(lines []string, targetLine int) string {
	// 转换为0-indexed
	lineIndex := targetLine - 1
	if lineIndex <= 0 || lineIndex >= len(lines) {
		return ""
	}

	var commentLines []string
	inMultiLineComment := false
	multiLineContent := []string{}

	// 从目标行的上一行开始向上查找
	for i := lineIndex - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])

		// 空行终止注释收集（除非在多行注释中）
		if line == "" && !inMultiLineComment {
			break
		}

		// 检查多行注释结束 */
		if strings.HasSuffix(line, "*/") {
			inMultiLineComment = true
			// 提取 */ 前的内容
			content := strings.TrimSuffix(line, "*/")
			content = strings.TrimSpace(content)
			if content != "" && content != "*" {
				multiLineContent = append([]string{cleanCommentLine(content)}, multiLineContent...)
			}
			continue
		}

		// 在多行注释中
		if inMultiLineComment {
			// 检查多行注释开始 /*
			if strings.HasPrefix(line, "/*") {
				// 提取 /* 后的内容
				content := strings.TrimPrefix(line, "/*")
				content = strings.TrimSpace(content)
				if content != "" && content != "*" {
					multiLineContent = append([]string{cleanCommentLine(content)}, multiLineContent...)
				}
				// 将多行注释内容添加到结果
				commentLines = append(multiLineContent, commentLines...)
				inMultiLineComment = false
				multiLineContent = []string{}
				continue
			}

			// 多行注释中间行
			content := cleanCommentLine(line)
			if content != "" {
				multiLineContent = append([]string{content}, multiLineContent...)
			}
			continue
		}

		// 检查单行注释 //
		if strings.HasPrefix(line, "//") {
			content := strings.TrimPrefix(line, "//")
			content = strings.TrimSpace(content)
			commentLines = append([]string{content}, commentLines...)
			continue
		}

		// 不是注释行，停止
		break
	}

	if len(commentLines) == 0 {
		return ""
	}

	return strings.Join(commentLines, "\n")
}

// cleanCommentLine 清理注释行内容
// 移除多行注释中的 * 前缀
func cleanCommentLine(line string) string {
	line = strings.TrimSpace(line)

	// 移除开头的 * 
	if strings.HasPrefix(line, "*") {
		line = strings.TrimPrefix(line, "*")
		line = strings.TrimSpace(line)
	}

	return line
}

// extractDocComment 提取文档注释
// 用于方法和类的文档注释提取
// 返回格式化后的注释字符串
func extractDocComment(lines []string, targetLine int) string {
	comment := extractCommentAboveLine(lines, targetLine)
	if comment == "" {
		return ""
	}

	// 返回格式化的注释
	return comment
}
