package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// Server LSP 服务器
type Server struct {
	// 文档管理
	documents *DocumentManager

	// 工作区索引
	workspace *WorkspaceIndex

	// 工作区根目录
	workspaceRoot string

	// 日志
	logFile *os.File
	logMu   sync.Mutex

	// 输入输出
	reader *bufio.Reader
	writer io.Writer
	mu     sync.Mutex

	// 服务器状态
	initialized bool
	shutdown    bool
}

// NewServer 创建 LSP 服务器
func NewServer(logPath string) *Server {
	s := &Server{
		documents: NewDocumentManager(),
		reader:    bufio.NewReader(os.Stdin),
		writer:    os.Stdout,
	}

	// 设置日志文件
	if logPath != "" {
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err == nil {
			s.logFile = f
		}
	}

	return s
}

// Run 启动 LSP 服务器主循环
func (s *Server) Run(ctx context.Context) error {
	s.log("Sola LSP Server started")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// 读取消息
		msg, err := s.readMessage()
		if err != nil {
			if err == io.EOF {
				s.log("Client disconnected")
				return nil
			}
			s.log("Error reading message: %v", err)
			continue
		}

		// 处理消息
		s.handleMessage(ctx, msg)

		// 如果收到 exit 通知，退出
		if s.shutdown {
			s.log("Server shutdown")
			return nil
		}
	}
}

// readMessage 读取 LSP 消息
func (s *Server) readMessage() ([]byte, error) {
	// 读取头部
	var contentLength int
	for {
		line, err := s.reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimSpace(line)

		if line == "" {
			// 头部结束
			break
		}

		if strings.HasPrefix(line, "Content-Length:") {
			lengthStr := strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:"))
			contentLength, err = strconv.Atoi(lengthStr)
			if err != nil {
				return nil, fmt.Errorf("invalid Content-Length: %s", lengthStr)
			}
		}
	}

	if contentLength == 0 {
		return nil, fmt.Errorf("missing Content-Length header")
	}

	// 读取内容
	content := make([]byte, contentLength)
	_, err := io.ReadFull(s.reader, content)
	if err != nil {
		return nil, err
	}

	s.log("Received: %s", string(content))
	return content, nil
}

// sendMessage 发送 LSP 消息
func (s *Server) sendMessage(msg interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	content, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(content))

	s.log("Sending: %s", string(content))

	_, err = s.writer.Write([]byte(header))
	if err != nil {
		return err
	}
	_, err = s.writer.Write(content)
	return err
}

// handleMessage 处理收到的消息
func (s *Server) handleMessage(ctx context.Context, msg []byte) {
	// 解析基础消息结构
	var baseMsg struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id,omitempty"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params,omitempty"`
	}

	if err := json.Unmarshal(msg, &baseMsg); err != nil {
		s.log("Error parsing message: %v", err)
		return
	}

	// 根据方法分发处理
	switch baseMsg.Method {
	case "initialize":
		s.handleInitialize(baseMsg.ID, baseMsg.Params)
	case "initialized":
		s.handleInitialized()
	case "shutdown":
		s.handleShutdown(baseMsg.ID)
	case "exit":
		s.handleExit()
	case "textDocument/didOpen":
		s.handleDidOpen(baseMsg.Params)
	case "textDocument/didChange":
		s.handleDidChange(baseMsg.Params)
	case "textDocument/didClose":
		s.handleDidClose(baseMsg.Params)
	case "textDocument/didSave":
		s.handleDidSave(baseMsg.Params)
	case "textDocument/hover":
		s.handleHover(baseMsg.ID, baseMsg.Params)
	case "textDocument/definition":
		s.handleDefinition(baseMsg.ID, baseMsg.Params)
	case "textDocument/references":
		s.handleReferences(baseMsg.ID, baseMsg.Params)
	case "textDocument/completion":
		s.handleCompletion(baseMsg.ID, baseMsg.Params)
	case "textDocument/formatting":
		s.handleFormatting(baseMsg.ID, baseMsg.Params)
	case "textDocument/rangeFormatting":
		s.handleRangeFormatting(baseMsg.ID, baseMsg.Params)
	case "textDocument/documentSymbol":
		s.handleDocumentSymbol(baseMsg.ID, baseMsg.Params)
	case "textDocument/rename":
		s.handleRename(baseMsg.ID, baseMsg.Params)
	case "textDocument/prepareRename":
		s.handlePrepareRename(baseMsg.ID, baseMsg.Params)
	case "textDocument/signatureHelp":
		s.handleSignatureHelp(baseMsg.ID, baseMsg.Params)
	case "textDocument/codeAction":
		s.handleCodeAction(baseMsg.ID, baseMsg.Params)
	case "workspace/symbol":
		s.handleWorkspaceSymbol(baseMsg.ID, baseMsg.Params)
	case "textDocument/semanticTokens/full":
		s.handleSemanticTokensFull(baseMsg.ID, baseMsg.Params)
	case "textDocument/semanticTokens/range":
		s.handleSemanticTokensRange(baseMsg.ID, baseMsg.Params)
	case "textDocument/inlayHint":
		s.handleInlayHints(baseMsg.ID, baseMsg.Params)
	case "textDocument/documentHighlight":
		s.handleDocumentHighlight(baseMsg.ID, baseMsg.Params)
	case "textDocument/foldingRange":
		s.handleFoldingRange(baseMsg.ID, baseMsg.Params)
	case "textDocument/selectionRange":
		s.handleSelectionRange(baseMsg.ID, baseMsg.Params)
	case "textDocument/documentLink":
		s.handleDocumentLinks(baseMsg.ID, baseMsg.Params)
	case "textDocument/prepareCallHierarchy":
		s.handleCallHierarchyPrepare(baseMsg.ID, baseMsg.Params)
	case "callHierarchy/incomingCalls":
		s.handleCallHierarchyIncomingCalls(baseMsg.ID, baseMsg.Params)
	case "callHierarchy/outgoingCalls":
		s.handleCallHierarchyOutgoingCalls(baseMsg.ID, baseMsg.Params)
	case "textDocument/codeLens":
		s.handleCodeLens(baseMsg.ID, baseMsg.Params)
	case "textDocument/prepareTypeHierarchy":
		s.handleTypeHierarchyPrepare(baseMsg.ID, baseMsg.Params)
	case "typeHierarchy/supertypes":
		s.handleTypeHierarchySupertypes(baseMsg.ID, baseMsg.Params)
	case "typeHierarchy/subtypes":
		s.handleTypeHierarchySubtypes(baseMsg.ID, baseMsg.Params)
	case "$/cancelRequest":
		// 忽略取消请求
	default:
		s.log("Unknown method: %s", baseMsg.Method)
		// 如果有 ID，返回方法未找到错误
		if baseMsg.ID != nil {
			s.sendError(baseMsg.ID, -32601, "Method not found: "+baseMsg.Method)
		}
	}
}

// handleInitialize 处理初始化请求
func (s *Server) handleInitialize(id json.RawMessage, params json.RawMessage) {
	var initParams protocol.InitializeParams
	if err := json.Unmarshal(params, &initParams); err != nil {
		s.sendError(id, -32700, "Parse error")
		return
	}

	// 保存工作区根目录
	if initParams.RootURI != "" {
		s.workspaceRoot = string(initParams.RootURI)
	}

	s.log("Initialize: workspace=%s", s.workspaceRoot)

	// 返回服务器能力
	result := map[string]interface{}{
		"capabilities": map[string]interface{}{
			// 文档同步：增量同步
			"textDocumentSync": map[string]interface{}{
				"openClose": true,
				"change":    2, // TextDocumentSyncKindIncremental
				"save": map[string]interface{}{
					"includeText": true,
				},
			},
			// 代码补全
			"completionProvider": map[string]interface{}{
				"triggerCharacters": []string{".", ">", ":", "$", "\\"},
				"resolveProvider":   false,
			},
			// 悬停提示
			"hoverProvider": true,
			// 签名帮助
			"signatureHelpProvider": map[string]interface{}{
				"triggerCharacters":   []string{"(", ","},
				"retriggerCharacters": []string{","},
			},
			// 跳转定义
			"definitionProvider": true,
			// 查找引用
			"referencesProvider": true,
			// 文档符号
			"documentSymbolProvider": true,
			// 工作区符号
			"workspaceSymbolProvider": true,
			// 代码格式化
			"documentFormattingProvider":      true,
			"documentRangeFormattingProvider": true,
			// 重命名
			"renameProvider": map[string]interface{}{
				"prepareProvider": true,
			},
			// 代码操作
			"codeActionProvider": map[string]interface{}{
				"codeActionKinds": []string{
					"quickfix",
					"source.organizeImports",
				},
			},
			// 语义高亮
			"semanticTokensProvider": getSemanticTokensProviderOptions(),
			// 内联提示
			"inlayHintProvider": true,
			// 文档高亮
			"documentHighlightProvider": true,
			// 折叠范围
			"foldingRangeProvider": true,
			// 选择范围
			"selectionRangeProvider": true,
			// 文档链接
			"documentLinkProvider": map[string]interface{}{
				"resolveProvider": false,
			},
			// 调用层次
			"callHierarchyProvider": true,
			// 代码镜头
			"codeLensProvider": map[string]interface{}{
				"resolveProvider": false,
			},
			// 类型层次
			"typeHierarchyProvider": true,
		},
		"serverInfo": map[string]interface{}{
			"name":    "solals",
			"version": "0.1.0",
		},
	}

	s.sendResult(id, result)
}

// handleInitialized 处理初始化完成通知
func (s *Server) handleInitialized() {
	s.initialized = true
	s.log("Server initialized")

	// 创建工作区索引
	s.workspace = NewWorkspaceIndex(s.workspaceRoot, s.log)

	// 异步索引标准库和工作区
	go func() {
		s.workspace.IndexStandardLibrary()
		s.workspace.IndexWorkspace()
		s.log("Workspace indexing complete")
	}()
}

// handleShutdown 处理关闭请求
func (s *Server) handleShutdown(id json.RawMessage) {
	s.log("Shutdown requested")
	s.sendResult(id, nil)
}

// handleExit 处理退出通知
func (s *Server) handleExit() {
	s.shutdown = true
	s.log("Exit notification received")
}

// handleDidOpen 处理文档打开
func (s *Server) handleDidOpen(params json.RawMessage) {
	var p protocol.DidOpenTextDocumentParams
	if err := json.Unmarshal(params, &p); err != nil {
		s.log("Error parsing didOpen params: %v", err)
		return
	}

	docURI := string(p.TextDocument.URI)
	s.log("Document opened: %s", docURI)

	// 添加文档
	s.documents.Open(docURI, p.TextDocument.Text, int(p.TextDocument.Version))

	// 发送诊断
	s.publishDiagnostics(docURI)
}

// handleDidChange 处理文档变更
func (s *Server) handleDidChange(params json.RawMessage) {
	var p protocol.DidChangeTextDocumentParams
	if err := json.Unmarshal(params, &p); err != nil {
		s.log("Error parsing didChange params: %v", err)
		return
	}

	docURI := string(p.TextDocument.URI)

	// 应用变更
	for _, change := range p.ContentChanges {
		s.documents.ApplyChange(docURI, change, int(p.TextDocument.Version))
	}

	// 发送诊断
	s.publishDiagnostics(docURI)
}

// handleDidClose 处理文档关闭
func (s *Server) handleDidClose(params json.RawMessage) {
	var p protocol.DidCloseTextDocumentParams
	if err := json.Unmarshal(params, &p); err != nil {
		s.log("Error parsing didClose params: %v", err)
		return
	}

	docURI := string(p.TextDocument.URI)
	s.log("Document closed: %s", docURI)

	// 移除文档
	s.documents.Close(docURI)

	// 清除诊断
	s.sendNotification("textDocument/publishDiagnostics", protocol.PublishDiagnosticsParams{
		URI:         p.TextDocument.URI,
		Diagnostics: []protocol.Diagnostic{},
	})
}

// handleDidSave 处理文档保存
func (s *Server) handleDidSave(params json.RawMessage) {
	var p protocol.DidSaveTextDocumentParams
	if err := json.Unmarshal(params, &p); err != nil {
		s.log("Error parsing didSave params: %v", err)
		return
	}

	docURI := string(p.TextDocument.URI)
	s.log("Document saved: %s", docURI)

	// 如果包含文本，更新文档
	if p.Text != "" {
		s.documents.UpdateContent(docURI, p.Text)
	}

	// 发送诊断
	s.publishDiagnostics(docURI)
}

// publishDiagnostics 发布诊断信息
func (s *Server) publishDiagnostics(docURI string) {
	doc := s.documents.Get(docURI)
	if doc == nil {
		return
	}

	diagnostics := s.getDiagnostics(doc)

	s.sendNotification("textDocument/publishDiagnostics", protocol.PublishDiagnosticsParams{
		URI:         protocol.DocumentURI(docURI),
		Version:     uint32(doc.Version),
		Diagnostics: diagnostics,
	})
}

// sendResult 发送成功响应
func (s *Server) sendResult(id json.RawMessage, result interface{}) {
	response := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	}
	s.sendMessage(response)
}

// sendError 发送错误响应
func (s *Server) sendError(id json.RawMessage, code int, message string) {
	response := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]interface{}{
			"code":    code,
			"message": message,
		},
	}
	s.sendMessage(response)
}

// sendNotification 发送通知
func (s *Server) sendNotification(method string, params interface{}) {
	notification := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	}
	s.sendMessage(notification)
}

// log 记录日志
func (s *Server) log(format string, args ...interface{}) {
	if s.logFile == nil {
		return
	}

	s.logMu.Lock()
	defer s.logMu.Unlock()

	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(s.logFile, "[%s] %s\n", "LSP", msg)
}

// uriToPath 将 URI 转换为文件路径
func uriToPath(docURI string) string {
	u, err := uri.Parse(docURI)
	if err != nil {
		return docURI
	}
	return u.Filename()
}
