package lsp2

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"go.lsp.dev/protocol"
)

// Server LSP 服务器
type Server struct {
	// 核心组件
	docManager       *DocumentManager
	importResolver   *ImportResolver
	definitionProvider *DefinitionProvider
	memMonitor       *MemoryMonitor
	logger           *Logger

	// 工作区信息
	workspaceRoot string

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
	logger := NewLogger(logPath)

	s := &Server{
		logger: logger,
		reader: bufio.NewReader(os.Stdin),
		writer: os.Stdout,
	}

	// 创建核心组件
	s.docManager = NewDocumentManager(logger)
	s.importResolver = NewImportResolver(logger)
	s.definitionProvider = NewDefinitionProvider(s.docManager, s.importResolver, logger)
	s.memMonitor = NewMemoryMonitor(s, logger)

	return s
}

// Run 启动 LSP 服务器主循环
func (s *Server) Run(ctx context.Context) error {
	s.logger.Info("Sola LSP Server v2 started (debug=%v)", s.logger.IsEnabled())

	// 启动内存监控（在后台goroutine运行）
	go s.memMonitor.Start(ctx)

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
				s.logger.Info("Client disconnected")
				return nil
			}
			s.logger.Error("Error reading message: %v", err)
			continue
		}

		// 处理消息
		s.handleMessage(msg)

		// 如果收到 exit 通知，退出
		if s.shutdown {
			s.logger.Info("Server shutdown")
			s.logger.Close()
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

	s.logger.Debug("Received message: %d bytes", contentLength)
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

	s.logger.Debug("Sending message: %d bytes", len(content))

	_, err = s.writer.Write([]byte(header))
	if err != nil {
		return err
	}
	_, err = s.writer.Write(content)
	return err
}

// handleMessage 处理收到的消息
func (s *Server) handleMessage(msg []byte) {
	// 解析基础消息结构
	var baseMsg struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id,omitempty"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params,omitempty"`
	}

	if err := json.Unmarshal(msg, &baseMsg); err != nil {
		s.logger.Error("Error parsing message: %v", err)
		return
	}

	s.logger.Debug("Handling method: %s", baseMsg.Method)

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
	case "textDocument/definition":
		s.handleDefinition(baseMsg.ID, baseMsg.Params)
	default:
		s.logger.Debug("Unhandled method: %s", baseMsg.Method)
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

	s.logger.Info("Initialize: workspace=%s", s.workspaceRoot)

	// 返回服务器能力
	result := map[string]interface{}{
		"capabilities": map[string]interface{}{
			// 文档同步：完整同步
			"textDocumentSync": map[string]interface{}{
				"openClose": true,
				"change":    1, // Full sync
				"save": map[string]interface{}{
					"includeText": true,
				},
			},
			// 跳转定义
			"definitionProvider": true,
		},
		"serverInfo": map[string]interface{}{
			"name":    "solals2",
			"version": "0.2.0",
		},
	}

	s.sendResult(id, result)
}

// handleInitialized 处理初始化完成通知
func (s *Server) handleInitialized() {
	s.initialized = true
	s.logger.Info("Server initialized")
}

// handleShutdown 处理关闭请求
func (s *Server) handleShutdown(id json.RawMessage) {
	s.logger.Info("Shutdown requested")
	s.sendResult(id, nil)
}

// handleExit 处理退出通知
func (s *Server) handleExit() {
	s.shutdown = true
	s.logger.Info("Exit notification received")
}

// handleDidOpen 处理文档打开
func (s *Server) handleDidOpen(params json.RawMessage) {
	var p protocol.DidOpenTextDocumentParams
	if err := json.Unmarshal(params, &p); err != nil {
		s.logger.Error("Error parsing didOpen params: %v", err)
		return
	}

	docURI := string(p.TextDocument.URI)
	s.docManager.Open(docURI, p.TextDocument.Text, int(p.TextDocument.Version))

	// 强制GC（打开文档后）
	runtime.GC()
}

// handleDidChange 处理文档变更
func (s *Server) handleDidChange(params json.RawMessage) {
	var p protocol.DidChangeTextDocumentParams
	if err := json.Unmarshal(params, &p); err != nil {
		s.logger.Error("Error parsing didChange params: %v", err)
		return
	}

	docURI := string(p.TextDocument.URI)

	// 完整同步：使用第一个变更的文本内容
	if len(p.ContentChanges) > 0 {
		newContent := p.ContentChanges[0].Text
		s.docManager.Update(docURI, newContent, int(p.TextDocument.Version))
	}
}

// handleDidClose 处理文档关闭
func (s *Server) handleDidClose(params json.RawMessage) {
	var p protocol.DidCloseTextDocumentParams
	if err := json.Unmarshal(params, &p); err != nil {
		s.logger.Error("Error parsing didClose params: %v", err)
		return
	}

	docURI := string(p.TextDocument.URI)
	s.docManager.Close(docURI)

	// 文档关闭后强制GC
	runtime.GC()
}

// handleDidSave 处理文档保存
func (s *Server) handleDidSave(params json.RawMessage) {
	var p protocol.DidSaveTextDocumentParams
	if err := json.Unmarshal(params, &p); err != nil {
		s.logger.Error("Error parsing didSave params: %v", err)
		return
	}

	s.logger.Debug("Document saved: %s", p.TextDocument.URI)

	// 如果包含文本，更新文档
	if p.Text != "" {
		docURI := string(p.TextDocument.URI)
		doc := s.docManager.Get(docURI)
		if doc != nil {
			s.docManager.Update(docURI, p.Text, doc.Version+1)
		}
	}
}

// handleDefinition 处理跳转定义请求
func (s *Server) handleDefinition(id json.RawMessage, params json.RawMessage) {
	var p protocol.DefinitionParams
	if err := json.Unmarshal(params, &p); err != nil {
		s.sendError(id, -32700, "Parse error")
		return
	}

	docURI := string(p.TextDocument.URI)
	line := int(p.Position.Line)
	character := int(p.Position.Character)

	s.logger.Debug("Definition request: %s:%d:%d", docURI, line, character)

	// 查找定义
	location := s.definitionProvider.FindDefinition(docURI, line, character)

	if location != nil {
		s.logger.Info("Found definition: %s:%d:%d", location.URI, location.Range.Start.Line, location.Range.Start.Character)
	} else {
		s.logger.Debug("Definition not found")
	}

	s.sendResult(id, location)
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
