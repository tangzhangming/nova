package lsp

import (
	"encoding/json"
	"strings"

	"go.lsp.dev/protocol"
)

// handleRename 处理重命名请求
func (s *Server) handleRename(id json.RawMessage, params json.RawMessage) {
	var p protocol.RenameParams
	if err := json.Unmarshal(params, &p); err != nil {
		s.sendError(id, -32700, "Parse error")
		return
	}

	docURI := string(p.TextDocument.URI)
	doc := s.documents.Get(docURI)
	if doc == nil {
		s.sendError(id, -32602, "Document not found")
		return
	}

	line := int(p.Position.Line)
	character := int(p.Position.Character)
	newName := p.NewName

	// 获取当前位置的单词
	word := doc.GetWordAt(line, character)
	if word == "" {
		s.sendError(id, -32602, "No symbol at position")
		return
	}

	// 检查是否是变量
	lineText := doc.GetLine(line)
	isVariable := false
	if character > 0 && character <= len(lineText) {
		start := character
		for start > 0 && isWordChar(lineText[start-1]) {
			start--
		}
		if start > 0 && lineText[start-1] == '$' {
			isVariable = true
		}
	}

	// 验证新名称
	if !isValidIdentifier(newName, isVariable) {
		s.sendError(id, -32602, "Invalid new name")
		return
	}

	// 执行重命名
	edit := s.performRename(doc, word, newName, isVariable)
	s.sendResult(id, edit)
}

// performRename 执行重命名
func (s *Server) performRename(doc *Document, oldName, newName string, isVariable bool) *protocol.WorkspaceEdit {
	changes := make(map[string][]protocol.TextEdit)

	// 在当前文档中查找所有引用
	edits := s.findRenameEdits(doc, oldName, newName, isVariable)
	if len(edits) > 0 {
		changes[doc.URI] = edits
	}

	// 如果不是变量，在其他文档中也查找
	if !isVariable {
		for _, otherDoc := range s.documents.GetAll() {
			if otherDoc.URI == doc.URI {
				continue
			}
			edits := s.findRenameEdits(otherDoc, oldName, newName, false)
			if len(edits) > 0 {
				changes[otherDoc.URI] = edits
			}
		}
	}

	// 转换为 DocumentChanges 格式
	var documentChanges []protocol.TextDocumentEdit
	for uri, textEdits := range changes {
		documentChanges = append(documentChanges, protocol.TextDocumentEdit{
			TextDocument: protocol.OptionalVersionedTextDocumentIdentifier{
				TextDocumentIdentifier: protocol.TextDocumentIdentifier{
					URI: protocol.DocumentURI(uri),
				},
			},
			Edits: convertToTextEditOrAnnotated(textEdits),
		})
	}

	return &protocol.WorkspaceEdit{
		DocumentChanges: documentChanges,
	}
}

// convertToTextEditOrAnnotated 转换为 TextEditOrAnnotatedTextEdit
func convertToTextEditOrAnnotated(edits []protocol.TextEdit) []protocol.TextEdit {
	return edits
}

// findRenameEdits 在文档中查找重命名编辑
func (s *Server) findRenameEdits(doc *Document, oldName, newName string, isVariable bool) []protocol.TextEdit {
	var edits []protocol.TextEdit

	for lineNum, lineText := range doc.Lines {
		if isVariable {
			// 查找变量引用 $name
			searchStr := "$" + oldName
			replaceStr := "$" + newName
			pos := 0
			for {
				idx := strings.Index(lineText[pos:], searchStr)
				if idx == -1 {
					break
				}
				actualPos := pos + idx

				// 确保是完整的变量名
				endPos := actualPos + len(searchStr)
				if endPos < len(lineText) && isWordChar(lineText[endPos]) {
					pos = actualPos + 1
					continue
				}

				edits = append(edits, protocol.TextEdit{
					Range: protocol.Range{
						Start: protocol.Position{
							Line:      uint32(lineNum),
							Character: uint32(actualPos),
						},
						End: protocol.Position{
							Line:      uint32(lineNum),
							Character: uint32(endPos),
						},
					},
					NewText: replaceStr,
				})
				pos = endPos
			}
		} else {
			// 查找标识符引用
			pos := 0
			for {
				idx := strings.Index(lineText[pos:], oldName)
				if idx == -1 {
					break
				}
				actualPos := pos + idx

				// 确保是完整的标识符
				if actualPos > 0 && isWordChar(lineText[actualPos-1]) {
					pos = actualPos + 1
					continue
				}
				endPos := actualPos + len(oldName)
				if endPos < len(lineText) && isWordChar(lineText[endPos]) {
					pos = actualPos + 1
					continue
				}

				// 排除 $ 前缀的变量
				if actualPos > 0 && lineText[actualPos-1] == '$' {
					pos = actualPos + 1
					continue
				}

				edits = append(edits, protocol.TextEdit{
					Range: protocol.Range{
						Start: protocol.Position{
							Line:      uint32(lineNum),
							Character: uint32(actualPos),
						},
						End: protocol.Position{
							Line:      uint32(lineNum),
							Character: uint32(endPos),
						},
					},
					NewText: newName,
				})
				pos = endPos
			}
		}
	}

	return edits
}

// isValidIdentifier 验证标识符是否有效
func isValidIdentifier(name string, isVariable bool) bool {
	if name == "" {
		return false
	}

	// 变量名不应包含 $（用户输入时不带 $）
	if isVariable && strings.HasPrefix(name, "$") {
		name = name[1:]
	}

	// 检查第一个字符
	first := name[0]
	if !((first >= 'a' && first <= 'z') || (first >= 'A' && first <= 'Z') || first == '_') {
		return false
	}

	// 检查其余字符
	for i := 1; i < len(name); i++ {
		c := name[i]
		if !isWordChar(c) {
			return false
		}
	}

	// 检查是否是关键字
	keywords := map[string]bool{
		"class": true, "interface": true, "enum": true, "type": true,
		"function": true, "public": true, "private": true, "protected": true,
		"static": true, "final": true, "abstract": true, "const": true,
		"extends": true, "implements": true, "new": true, "return": true,
		"if": true, "elseif": true, "else": true, "switch": true,
		"case": true, "default": true, "for": true, "foreach": true,
		"while": true, "do": true, "break": true, "continue": true,
		"try": true, "catch": true, "finally": true, "throw": true,
		"true": true, "false": true, "null": true, "this": true,
		"self": true, "parent": true, "echo": true, "use": true,
		"namespace": true, "as": true, "is": true, "go": true, "select": true,
	}

	return !keywords[name]
}
