package lsp

import (
	"encoding/json"

	"github.com/tangzhangming/nova/internal/formatter"
	"go.lsp.dev/protocol"
)

// handleFormatting 处理文档格式化请求
func (s *Server) handleFormatting(id json.RawMessage, params json.RawMessage) {
	var p protocol.DocumentFormattingParams
	if err := json.Unmarshal(params, &p); err != nil {
		s.sendError(id, -32700, "Parse error")
		return
	}

	docURI := string(p.TextDocument.URI)
	doc := s.documents.Get(docURI)
	if doc == nil {
		s.sendResult(id, []protocol.TextEdit{})
		return
	}

	// 获取格式化选项
	options := formatter.DefaultOptions()

	// 从 LSP 选项转换
	if p.Options.TabSize > 0 {
		options.IndentSize = int(p.Options.TabSize)
	}
	if p.Options.InsertSpaces {
		options.IndentStyle = "spaces"
	} else {
		options.IndentStyle = "tabs"
	}

	// 执行格式化
	filename := uriToPath(docURI)
	formatted, err := formatter.Format(doc.Content, filename, options)
	if err != nil {
		s.log("Format error: %v", err)
		// 格式化失败时返回空编辑
		s.sendResult(id, []protocol.TextEdit{})
		return
	}

	// 如果内容没有变化，返回空编辑
	if formatted == doc.Content {
		s.sendResult(id, []protocol.TextEdit{})
		return
	}

	// 计算需要替换的范围（整个文档）
	lines := doc.Lines
	lastLine := len(lines) - 1
	if lastLine < 0 {
		lastLine = 0
	}
	lastChar := 0
	if lastLine < len(lines) {
		lastChar = len(lines[lastLine])
	}

	edit := protocol.TextEdit{
		Range: protocol.Range{
			Start: protocol.Position{
				Line:      0,
				Character: 0,
			},
			End: protocol.Position{
				Line:      uint32(lastLine),
				Character: uint32(lastChar),
			},
		},
		NewText: formatted,
	}

	s.sendResult(id, []protocol.TextEdit{edit})
}

// handleRangeFormatting 处理范围格式化请求
func (s *Server) handleRangeFormatting(id json.RawMessage, params json.RawMessage) {
	var p protocol.DocumentRangeFormattingParams
	if err := json.Unmarshal(params, &p); err != nil {
		s.sendError(id, -32700, "Parse error")
		return
	}

	// 目前不支持范围格式化，返回空
	// TODO: 实现范围格式化
	s.sendResult(id, []protocol.TextEdit{})
}
