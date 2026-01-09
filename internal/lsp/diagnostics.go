package lsp

import (
	"go.lsp.dev/protocol"
)

// getDiagnostics 获取文档的诊断信息
func (s *Server) getDiagnostics(doc *Document) []protocol.Diagnostic {
	var diagnostics []protocol.Diagnostic

	// 确保文档已解析
	_ = doc.GetAST()

	// 添加解析错误
	for _, err := range doc.ParseErrs {
		diag := protocol.Diagnostic{
			Range: protocol.Range{
				Start: protocol.Position{
					Line:      uint32(err.Pos.Line - 1), // LSP 行号从 0 开始
					Character: uint32(err.Pos.Column - 1),
				},
				End: protocol.Position{
					Line:      uint32(err.Pos.Line - 1),
					Character: uint32(err.Pos.Column + 10), // 估计错误范围
				},
			},
			Severity: protocol.DiagnosticSeverityError,
			Source:   "sola",
			Message:  err.Message,
		}
		diagnostics = append(diagnostics, diag)
	}

	// TODO: 添加类型检查错误
	// 需要调用 compiler.TypeCheck() 并收集错误

	return diagnostics
}

// DiagnosticSeverity 诊断严重程度
type DiagnosticSeverity int

const (
	SeverityError       DiagnosticSeverity = 1
	SeverityWarning     DiagnosticSeverity = 2
	SeverityInformation DiagnosticSeverity = 3
	SeverityHint        DiagnosticSeverity = 4
)

// ErrorCodeToDiagnostic 将错误码转换为诊断信息
func ErrorCodeToDiagnostic(code, message string, line, col int) protocol.Diagnostic {
	severity := protocol.DiagnosticSeverityError

	// 根据错误码前缀判断严重程度
	if len(code) > 0 {
		switch code[0] {
		case 'W': // Warning
			severity = protocol.DiagnosticSeverityWarning
		case 'I': // Info
			severity = protocol.DiagnosticSeverityInformation
		case 'H': // Hint
			severity = protocol.DiagnosticSeverityHint
		}
	}

	return protocol.Diagnostic{
		Range: protocol.Range{
			Start: protocol.Position{
				Line:      uint32(line - 1),
				Character: uint32(col - 1),
			},
			End: protocol.Position{
				Line:      uint32(line - 1),
				Character: uint32(col + 10),
			},
		},
		Severity: severity,
		Code:     code,
		Source:   "sola",
		Message:  message,
	}
}
