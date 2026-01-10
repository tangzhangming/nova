package lsp

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"

	"go.lsp.dev/protocol"
)

// DocumentLink 文档链接
type DocumentLink struct {
	Range   protocol.Range  `json:"range"`
	Target  string          `json:"target,omitempty"`
	Tooltip string          `json:"tooltip,omitempty"`
	Data    interface{}     `json:"data,omitempty"`
}

// handleDocumentLinks 处理文档链接请求
func (s *Server) handleDocumentLinks(id json.RawMessage, params json.RawMessage) {
	var p protocol.DocumentLinkParams
	if err := json.Unmarshal(params, &p); err != nil {
		s.sendError(id, -32700, "Parse error")
		return
	}

	docURI := string(p.TextDocument.URI)
	doc := s.documents.Get(docURI)
	if doc == nil {
		s.sendResult(id, []DocumentLink{})
		return
	}

	links := s.collectDocumentLinks(doc)
	s.sendResult(id, links)
}

// collectDocumentLinks 收集文档链接
func (s *Server) collectDocumentLinks(doc *Document) []DocumentLink {
	var links []DocumentLink

	astFile := doc.GetAST()

	// 收集 use 语句中的链接
	if astFile != nil {
		for _, use := range astFile.Uses {
			if use == nil {
				continue
			}

			// 尝试解析导入路径
			var target string
			if s.workspace != nil {
				resolved, err := s.workspace.ResolveImport(use.Path)
				if err == nil && resolved != "" {
					target = "file:///" + strings.ReplaceAll(resolved, "\\", "/")
				}
			}

			// 计算路径在行中的位置
			line := use.UseToken.Pos.Line - 1
			lineText := doc.GetLine(int(line))
			pathStart := strings.Index(lineText, use.Path)
			if pathStart < 0 {
				// 尝试查找带引号的路径
				pathStart = strings.Index(lineText, "\""+use.Path+"\"")
				if pathStart >= 0 {
					pathStart++ // 跳过开头引号
				}
			}

			if pathStart >= 0 {
				links = append(links, DocumentLink{
					Range: protocol.Range{
						Start: protocol.Position{
							Line:      uint32(line),
							Character: uint32(pathStart),
						},
						End: protocol.Position{
							Line:      uint32(line),
							Character: uint32(pathStart + len(use.Path)),
						},
					},
					Target:  target,
					Tooltip: "Go to " + use.Path,
				})
			}
		}
	}

	// 收集 URL 链接
	links = append(links, s.collectURLLinks(doc)...)

	return links
}

// collectURLLinks 收集 URL 链接
func (s *Server) collectURLLinks(doc *Document) []DocumentLink {
	var links []DocumentLink

	// URL 正则表达式
	urlPattern := regexp.MustCompile(`https?://[^\s"'<>\[\]{}|\\^` + "`" + `]+`)

	for lineNum, lineText := range doc.Lines {
		matches := urlPattern.FindAllStringIndex(lineText, -1)
		for _, match := range matches {
			start, end := match[0], match[1]
			url := lineText[start:end]

			// 移除尾部标点符号
			url = strings.TrimRight(url, ".,;:!?)")

			links = append(links, DocumentLink{
				Range: protocol.Range{
					Start: protocol.Position{
						Line:      uint32(lineNum),
						Character: uint32(start),
					},
					End: protocol.Position{
						Line:      uint32(lineNum),
						Character: uint32(start + len(url)),
					},
				},
				Target:  url,
				Tooltip: "Open " + url,
			})
		}
	}

	return links
}

// handleDocumentLinkResolve 处理文档链接解析请求
func (s *Server) handleDocumentLinkResolve(id json.RawMessage, params json.RawMessage) {
	var link DocumentLink
	if err := json.Unmarshal(params, &link); err != nil {
		s.sendError(id, -32700, "Parse error")
		return
	}

	// 如果已经有 target，直接返回
	if link.Target != "" {
		s.sendResult(id, link)
		return
	}

	// 尝试解析链接
	if data, ok := link.Data.(map[string]interface{}); ok {
		if path, ok := data["path"].(string); ok {
			// 解析导入路径
			if s.workspace != nil {
				resolved, err := s.workspace.ResolveImport(path)
				if err == nil && resolved != "" {
					link.Target = "file:///" + filepath.ToSlash(resolved)
				}
			}
		}
	}

	s.sendResult(id, link)
}
