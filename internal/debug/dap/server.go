// server.go - DAP 服务器
//
// 实现 Debug Adapter Protocol 服务器。
// 支持通过 stdio 或 TCP 与 IDE 通信。

package dap

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"

	"github.com/tangzhangming/nova/internal/debug"
)

// Server DAP 服务器
type Server struct {
	mu sync.RWMutex
	
	// 调试器
	debugger *debug.Debugger
	
	// 通信
	reader *bufio.Reader
	writer io.Writer
	
	// 序列号
	seq int32
	
	// 状态
	initialized bool
	launched    bool
	running     bool
	
	// 变量引用（用于变量查看）
	variableRefs     map[int]variableRef
	nextVariableRef  int
	
	// 配置
	config ServerConfig
}

// variableRef 变量引用
type variableRef struct {
	frameID  int
	scope    string // "locals", "globals"
}

// ServerConfig 服务器配置
type ServerConfig struct {
	// 工作目录
	WorkDir string
	
	// 程序路径
	Program string
	
	// 程序参数
	Args []string
}

// NewServer 创建 DAP 服务器
func NewServer(debugger *debug.Debugger) *Server {
	return &Server{
		debugger:     debugger,
		variableRefs: make(map[int]variableRef),
	}
}

// ServeStdio 通过 stdio 提供服务
func (s *Server) ServeStdio() error {
	s.reader = bufio.NewReader(os.Stdin)
	s.writer = os.Stdout
	
	return s.serve()
}

// ServeTCP 通过 TCP 提供服务
func (s *Server) ServeTCP(addr string) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer listener.Close()
	
	fmt.Fprintf(os.Stderr, "DAP server listening on %s\n", addr)
	
	for {
		conn, err := listener.Accept()
		if err != nil {
			return err
		}
		
		s.reader = bufio.NewReader(conn)
		s.writer = conn
		
		if err := s.serve(); err != nil {
			fmt.Fprintf(os.Stderr, "Session error: %v\n", err)
		}
		
		conn.Close()
	}
}

// serve 主服务循环
func (s *Server) serve() error {
	s.running = true
	
	// 启动事件处理
	go s.handleDebugEvents()
	
	for s.running {
		// 读取消息头
		header, err := s.readHeader()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		
		// 读取消息体
		contentLength, ok := header["Content-Length"]
		if !ok {
			return fmt.Errorf("missing Content-Length header")
		}
		
		body := make([]byte, contentLength)
		if _, err := io.ReadFull(s.reader, body); err != nil {
			return err
		}
		
		// 解析请求
		var req Request
		if err := json.Unmarshal(body, &req); err != nil {
			return err
		}
		
		// 处理请求
		s.handleRequest(&req)
	}
	
	return nil
}

// readHeader 读取消息头
func (s *Server) readHeader() (map[string]int, error) {
	header := make(map[string]int)
	
	for {
		line, err := s.reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		
		line = line[:len(line)-1]
		if len(line) > 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
		}
		
		if line == "" {
			break
		}
		
		var key string
		var value int
		if _, err := fmt.Sscanf(line, "%s %d", &key, &value); err == nil {
			if key == "Content-Length:" {
				header["Content-Length"] = value
			}
		}
	}
	
	return header, nil
}

// handleRequest 处理请求
func (s *Server) handleRequest(req *Request) {
	switch req.Command {
	case "initialize":
		s.handleInitialize(req)
	case "launch":
		s.handleLaunch(req)
	case "attach":
		s.handleAttach(req)
	case "configurationDone":
		s.handleConfigurationDone(req)
	case "setBreakpoints":
		s.handleSetBreakpoints(req)
	case "setFunctionBreakpoints":
		s.handleSetFunctionBreakpoints(req)
	case "continue":
		s.handleContinue(req)
	case "next":
		s.handleNext(req)
	case "stepIn":
		s.handleStepIn(req)
	case "stepOut":
		s.handleStepOut(req)
	case "pause":
		s.handlePause(req)
	case "stackTrace":
		s.handleStackTrace(req)
	case "scopes":
		s.handleScopes(req)
	case "variables":
		s.handleVariables(req)
	case "evaluate":
		s.handleEvaluate(req)
	case "threads":
		s.handleThreads(req)
	case "disconnect":
		s.handleDisconnect(req)
	case "terminate":
		s.handleTerminate(req)
	default:
		s.sendErrorResponse(req, fmt.Sprintf("Unknown command: %s", req.Command))
	}
}

// ============================================================================
// 请求处理器
// ============================================================================

func (s *Server) handleInitialize(req *Request) {
	capabilities := Capabilities{
		SupportsConfigurationDoneRequest:  true,
		SupportsFunctionBreakpoints:       false,
		SupportsConditionalBreakpoints:    true,
		SupportsHitConditionalBreakpoints: true,
		SupportsEvaluateForHovers:         true,
		SupportsSetVariable:               false,
		SupportsRestartRequest:            false,
		SupportsTerminateRequest:          true,
		SupportsLogPoints:                 true,
	}
	
	s.sendResponse(req, true, "", capabilities)
	
	// 发送 initialized 事件
	s.sendEvent("initialized", InitializedEventBody{})
	s.initialized = true
}

func (s *Server) handleLaunch(req *Request) {
	var args LaunchRequestArguments
	if err := s.unmarshalArguments(req, &args); err != nil {
		s.sendErrorResponse(req, err.Error())
		return
	}
	
	s.config.Program = args.Program
	s.config.Args = args.Args
	if args.Cwd != "" {
		s.config.WorkDir = args.Cwd
	}
	
	s.launched = true
	s.sendResponse(req, true, "", nil)
	
	// 如果配置了入口暂停
	if args.StopOnEntry {
		s.debugger.Pause()
	}
}

func (s *Server) handleAttach(req *Request) {
	s.sendResponse(req, true, "", nil)
}

func (s *Server) handleConfigurationDone(req *Request) {
	s.sendResponse(req, true, "", nil)
}

func (s *Server) handleSetBreakpoints(req *Request) {
	var args SetBreakpointsArguments
	if err := s.unmarshalArguments(req, &args); err != nil {
		s.sendErrorResponse(req, err.Error())
		return
	}
	
	// 获取文件路径
	file := args.Source.Path
	if file == "" {
		file = args.Source.Name
	}
	
	// 清除该文件的旧断点
	oldBreakpoints := s.debugger.GetBreakpointsForFile(file)
	for _, bp := range oldBreakpoints {
		s.debugger.RemoveBreakpoint(bp.ID)
	}
	
	// 设置新断点
	result := make([]Breakpoint, len(args.Breakpoints))
	for i, sbp := range args.Breakpoints {
		var bp *debug.Breakpoint
		var err error
		
		if sbp.Condition != "" {
			bp, err = s.debugger.SetConditionalBreakpoint(file, sbp.Line, sbp.Condition)
		} else {
			bp, err = s.debugger.SetBreakpoint(file, sbp.Line)
		}
		
		if err != nil {
			result[i] = Breakpoint{
				Verified: false,
				Message:  err.Error(),
				Line:     sbp.Line,
			}
		} else {
			result[i] = Breakpoint{
				Id:       bp.ID,
				Verified: true,
				Line:     bp.Line,
				Source:   &Source{Path: file},
			}
		}
	}
	
	s.sendResponse(req, true, "", SetBreakpointsResponseBody{
		Breakpoints: result,
	})
}

func (s *Server) handleSetFunctionBreakpoints(req *Request) {
	// 暂不支持函数断点
	s.sendResponse(req, true, "", SetBreakpointsResponseBody{
		Breakpoints: []Breakpoint{},
	})
}

func (s *Server) handleContinue(req *Request) {
	s.debugger.Continue()
	s.sendResponse(req, true, "", ContinueResponseBody{
		AllThreadsContinued: true,
	})
}

func (s *Server) handleNext(req *Request) {
	s.debugger.StepOver()
	s.sendResponse(req, true, "", nil)
}

func (s *Server) handleStepIn(req *Request) {
	s.debugger.StepIn()
	s.sendResponse(req, true, "", nil)
}

func (s *Server) handleStepOut(req *Request) {
	s.debugger.StepOut()
	s.sendResponse(req, true, "", nil)
}

func (s *Server) handlePause(req *Request) {
	s.debugger.Pause()
	s.sendResponse(req, true, "", nil)
}

func (s *Server) handleStackTrace(req *Request) {
	var args StackTraceArguments
	if err := s.unmarshalArguments(req, &args); err != nil {
		s.sendErrorResponse(req, err.Error())
		return
	}
	
	stack := s.debugger.GetCallStack()
	
	frames := make([]StackFrame, len(stack))
	for i, f := range stack {
		frames[i] = StackFrame{
			Id:     f.ID,
			Name:   f.Name,
			Line:   f.Line,
			Column: f.Column,
			Source: &Source{
				Path: f.File,
				Name: f.File,
			},
		}
	}
	
	s.sendResponse(req, true, "", StackTraceResponseBody{
		StackFrames: frames,
		TotalFrames: len(frames),
	})
}

func (s *Server) handleScopes(req *Request) {
	var args ScopesArguments
	if err := s.unmarshalArguments(req, &args); err != nil {
		s.sendErrorResponse(req, err.Error())
		return
	}
	
	// 创建变量引用
	s.mu.Lock()
	s.nextVariableRef++
	localsRef := s.nextVariableRef
	s.variableRefs[localsRef] = variableRef{frameID: args.FrameId, scope: "locals"}
	
	s.nextVariableRef++
	globalsRef := s.nextVariableRef
	s.variableRefs[globalsRef] = variableRef{frameID: args.FrameId, scope: "globals"}
	s.mu.Unlock()
	
	scopes := []Scope{
		{
			Name:               "Locals",
			PresentationHint:   "locals",
			VariablesReference: localsRef,
		},
		{
			Name:               "Globals",
			PresentationHint:   "globals",
			VariablesReference: globalsRef,
		},
	}
	
	s.sendResponse(req, true, "", ScopesResponseBody{Scopes: scopes})
}

func (s *Server) handleVariables(req *Request) {
	var args VariablesArguments
	if err := s.unmarshalArguments(req, &args); err != nil {
		s.sendErrorResponse(req, err.Error())
		return
	}
	
	s.mu.RLock()
	ref, ok := s.variableRefs[args.VariablesReference]
	s.mu.RUnlock()
	
	if !ok {
		s.sendResponse(req, true, "", VariablesResponseBody{Variables: []Variable{}})
		return
	}
	
	var vars map[string]interface{}
	if ref.scope == "locals" {
		localVars := s.debugger.GetLocals(ref.frameID)
		vars = make(map[string]interface{})
		for k, v := range localVars {
			vars[k] = v
		}
	} else {
		globalVars := s.debugger.GetGlobals()
		vars = make(map[string]interface{})
		for k, v := range globalVars {
			vars[k] = v
		}
	}
	
	variables := make([]Variable, 0, len(vars))
	for name, value := range vars {
		variables = append(variables, Variable{
			Name:               name,
			Value:              fmt.Sprintf("%v", value),
			Type:               fmt.Sprintf("%T", value),
			VariablesReference: 0,
		})
	}
	
	s.sendResponse(req, true, "", VariablesResponseBody{Variables: variables})
}

func (s *Server) handleEvaluate(req *Request) {
	var args EvaluateArguments
	if err := s.unmarshalArguments(req, &args); err != nil {
		s.sendErrorResponse(req, err.Error())
		return
	}
	
	result, err := s.debugger.EvaluateExpression(args.Expression, args.FrameId)
	if err != nil {
		s.sendErrorResponse(req, err.Error())
		return
	}
	
	s.sendResponse(req, true, "", EvaluateResponseBody{
		Result:             fmt.Sprintf("%v", result),
		Type:               fmt.Sprintf("%T", result),
		VariablesReference: 0,
	})
}

func (s *Server) handleThreads(req *Request) {
	// 单线程实现
	threads := []Thread{
		{Id: 1, Name: "main"},
	}
	s.sendResponse(req, true, "", ThreadsResponseBody{Threads: threads})
}

func (s *Server) handleDisconnect(req *Request) {
	s.running = false
	s.sendResponse(req, true, "", nil)
}

func (s *Server) handleTerminate(req *Request) {
	s.debugger.Terminate()
	s.sendResponse(req, true, "", nil)
}

// ============================================================================
// 事件处理
// ============================================================================

func (s *Server) handleDebugEvents() {
	for event := range s.debugger.Events() {
		switch event.Type {
		case debug.EventStopped:
			s.sendEvent("stopped", StoppedEventBody{
				Reason:            event.Reason,
				ThreadId:          1,
				AllThreadsStopped: true,
			})
		case debug.EventContinued:
			s.sendEvent("continued", ContinuedEventBody{
				ThreadId:            1,
				AllThreadsContinued: true,
			})
		case debug.EventBreakpoint:
			s.sendEvent("stopped", StoppedEventBody{
				Reason:            "breakpoint",
				ThreadId:          1,
				AllThreadsStopped: true,
			})
		case debug.EventStep:
			s.sendEvent("stopped", StoppedEventBody{
				Reason:            "step",
				ThreadId:          1,
				AllThreadsStopped: true,
			})
		case debug.EventException:
			s.sendEvent("stopped", StoppedEventBody{
				Reason:            "exception",
				ThreadId:          1,
				AllThreadsStopped: true,
			})
		case debug.EventTerminated:
			s.sendEvent("terminated", TerminatedEventBody{})
		}
	}
}

// ============================================================================
// 响应发送
// ============================================================================

func (s *Server) sendResponse(req *Request, success bool, message string, body interface{}) {
	resp := Response{
		Message: Message{
			Seq:  int(atomic.AddInt32(&s.seq, 1)),
			Type: "response",
		},
		RequestSeq:   req.Seq,
		Success:      success,
		Command:      req.Command,
		ErrorMessage: message,
		Body:         body,
	}
	s.send(resp)
}

func (s *Server) sendErrorResponse(req *Request, message string) {
	s.sendResponse(req, false, message, nil)
}

func (s *Server) sendEvent(event string, body interface{}) {
	evt := Event{
		Message: Message{
			Seq:  int(atomic.AddInt32(&s.seq, 1)),
			Type: "event",
		},
		Event: event,
		Body:  body,
	}
	s.send(evt)
}

func (s *Server) send(msg interface{}) {
	body, err := json.Marshal(msg)
	if err != nil {
		return
	}
	
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	s.writer.Write([]byte(header))
	s.writer.Write(body)
}

func (s *Server) unmarshalArguments(req *Request, v interface{}) error {
	if req.Arguments == nil {
		return nil
	}
	
	data, err := json.Marshal(req.Arguments)
	if err != nil {
		return err
	}
	
	return json.Unmarshal(data, v)
}
