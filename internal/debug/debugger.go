// debugger.go - Sola 调试器核心
//
// 提供源码级调试功能：
// 1. 断点管理（行断点、条件断点）
// 2. 单步执行（step in/out/over）
// 3. 变量查看
// 4. 调用栈查看
// 5. 表达式求值

package debug

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/tangzhangming/nova/internal/bytecode"
)

// DebugState 调试状态
type DebugState int

const (
	// StateRunning 运行中
	StateRunning DebugState = iota
	// StatePaused 已暂停
	StatePaused
	// StateStepping 单步执行中
	StateStepping
	// StateTerminated 已终止
	StateTerminated
)

// StepAction 单步操作
type StepAction int

const (
	// StepNone 无操作
	StepNone StepAction = iota
	// StepIn 步入
	StepIn
	// StepOver 步过
	StepOver
	// StepOut 步出
	StepOut
)

// Debugger 调试器
type Debugger struct {
	mu sync.RWMutex
	
	// 状态
	state       DebugState
	stepAction  StepAction
	stepDepth   int
	
	// 断点管理
	breakpoints *BreakpointManager
	
	// 当前执行位置
	currentFile  string
	currentLine  int
	currentFunc  string
	callStack    []StackFrame
	
	// 局部变量
	locals map[string]bytecode.Value
	
	// 全局变量
	globals map[string]bytecode.Value
	
	// 事件通道
	eventChan chan DebugEvent
	
	// 暂停信号
	pauseChan chan struct{}
	resumeChan chan struct{}
	
	// 序列号
	sequenceID int64
	
	// 配置
	config DebugConfig
}

// DebugConfig 调试配置
type DebugConfig struct {
	// StopOnEntry 启动时暂停
	StopOnEntry bool
	
	// MaxCallStackDepth 最大调用栈深度
	MaxCallStackDepth int
}

// StackFrame 栈帧
type StackFrame struct {
	ID       int
	Name     string
	File     string
	Line     int
	Column   int
	Locals   map[string]bytecode.Value
	ModuleID int
}

// DebugEvent 调试事件
type DebugEvent struct {
	Type    EventType
	Reason  string
	Data    interface{}
	SeqID   int64
}

// EventType 事件类型
type EventType int

const (
	// EventStopped 停止事件
	EventStopped EventType = iota
	// EventContinued 继续事件
	EventContinued
	// EventBreakpoint 断点命中
	EventBreakpoint
	// EventStep 单步完成
	EventStep
	// EventException 异常
	EventException
	// EventOutput 输出
	EventOutput
	// EventTerminated 终止
	EventTerminated
)

// DefaultDebugConfig 默认配置
func DefaultDebugConfig() DebugConfig {
	return DebugConfig{
		StopOnEntry:       false,
		MaxCallStackDepth: 100,
	}
}

// NewDebugger 创建调试器
func NewDebugger() *Debugger {
	return NewDebuggerWithConfig(DefaultDebugConfig())
}

// NewDebuggerWithConfig 创建带配置的调试器
func NewDebuggerWithConfig(config DebugConfig) *Debugger {
	return &Debugger{
		state:       StateRunning,
		breakpoints: NewBreakpointManager(),
		locals:      make(map[string]bytecode.Value),
		globals:     make(map[string]bytecode.Value),
		eventChan:   make(chan DebugEvent, 100),
		pauseChan:   make(chan struct{}),
		resumeChan:  make(chan struct{}),
		config:      config,
	}
}

// GetState 获取调试状态
func (d *Debugger) GetState() DebugState {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.state
}

// SetState 设置调试状态
func (d *Debugger) SetState(state DebugState) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.state = state
}

// Events 获取事件通道
func (d *Debugger) Events() <-chan DebugEvent {
	return d.eventChan
}

// ============================================================================
// 断点操作
// ============================================================================

// SetBreakpoint 设置断点
func (d *Debugger) SetBreakpoint(file string, line int) (*Breakpoint, error) {
	return d.breakpoints.Add(file, line)
}

// SetConditionalBreakpoint 设置条件断点
func (d *Debugger) SetConditionalBreakpoint(file string, line int, condition string) (*Breakpoint, error) {
	return d.breakpoints.AddConditional(file, line, condition)
}

// RemoveBreakpoint 移除断点
func (d *Debugger) RemoveBreakpoint(id int) error {
	return d.breakpoints.Remove(id)
}

// GetBreakpoints 获取所有断点
func (d *Debugger) GetBreakpoints() []*Breakpoint {
	return d.breakpoints.GetAll()
}

// GetBreakpointsForFile 获取文件的所有断点
func (d *Debugger) GetBreakpointsForFile(file string) []*Breakpoint {
	return d.breakpoints.GetForFile(file)
}

// EnableBreakpoint 启用断点
func (d *Debugger) EnableBreakpoint(id int) error {
	return d.breakpoints.Enable(id)
}

// DisableBreakpoint 禁用断点
func (d *Debugger) DisableBreakpoint(id int) error {
	return d.breakpoints.Disable(id)
}

// ============================================================================
// 执行控制
// ============================================================================

// Continue 继续执行
func (d *Debugger) Continue() {
	d.mu.Lock()
	d.state = StateRunning
	d.stepAction = StepNone
	d.mu.Unlock()
	
	select {
	case d.resumeChan <- struct{}{}:
	default:
	}
	
	d.sendEvent(EventContinued, "continue", nil)
}

// Pause 暂停执行
func (d *Debugger) Pause() {
	d.mu.Lock()
	d.state = StatePaused
	d.mu.Unlock()
	
	d.sendEvent(EventStopped, "pause", nil)
}

// StepIn 步入
func (d *Debugger) StepIn() {
	d.mu.Lock()
	d.state = StateStepping
	d.stepAction = StepIn
	d.stepDepth = len(d.callStack)
	d.mu.Unlock()
	
	select {
	case d.resumeChan <- struct{}{}:
	default:
	}
}

// StepOver 步过
func (d *Debugger) StepOver() {
	d.mu.Lock()
	d.state = StateStepping
	d.stepAction = StepOver
	d.stepDepth = len(d.callStack)
	d.mu.Unlock()
	
	select {
	case d.resumeChan <- struct{}{}:
	default:
	}
}

// StepOut 步出
func (d *Debugger) StepOut() {
	d.mu.Lock()
	d.state = StateStepping
	d.stepAction = StepOut
	d.stepDepth = len(d.callStack)
	d.mu.Unlock()
	
	select {
	case d.resumeChan <- struct{}{}:
	default:
	}
}

// Terminate 终止调试
func (d *Debugger) Terminate() {
	d.mu.Lock()
	d.state = StateTerminated
	d.mu.Unlock()
	
	d.sendEvent(EventTerminated, "terminated", nil)
}

// ============================================================================
// VM 钩子（供 VM 调用）
// ============================================================================

// OnInstruction VM 执行指令前调用
// 返回 true 表示应该暂停执行
func (d *Debugger) OnInstruction(file string, line int, funcName string) bool {
	d.mu.Lock()
	d.currentFile = file
	d.currentLine = line
	d.currentFunc = funcName
	state := d.state
	stepAction := d.stepAction
	stepDepth := d.stepDepth
	callStackLen := len(d.callStack)
	d.mu.Unlock()
	
	// 检查是否终止
	if state == StateTerminated {
		return true
	}
	
	// 检查断点
	if d.breakpoints.ShouldBreak(file, line) {
		d.mu.Lock()
		d.state = StatePaused
		d.mu.Unlock()
		
		d.sendEvent(EventBreakpoint, "breakpoint", map[string]interface{}{
			"file": file,
			"line": line,
		})
		
		// 等待继续信号
		<-d.resumeChan
		return false
	}
	
	// 检查单步
	if state == StateStepping {
		shouldStop := false
		
		switch stepAction {
		case StepIn:
			// 每条指令都停
			shouldStop = true
		case StepOver:
			// 同级或更浅的深度停
			shouldStop = callStackLen <= stepDepth
		case StepOut:
			// 更浅的深度停
			shouldStop = callStackLen < stepDepth
		}
		
		if shouldStop {
			d.mu.Lock()
			d.state = StatePaused
			d.mu.Unlock()
			
			d.sendEvent(EventStep, "step", map[string]interface{}{
				"file": file,
				"line": line,
			})
			
			// 等待继续信号
			<-d.resumeChan
		}
	}
	
	// 检查暂停状态
	if state == StatePaused {
		<-d.resumeChan
	}
	
	return false
}

// OnFunctionEnter VM 进入函数时调用
func (d *Debugger) OnFunctionEnter(funcName string, file string, line int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	
	frame := StackFrame{
		ID:     len(d.callStack),
		Name:   funcName,
		File:   file,
		Line:   line,
		Locals: make(map[string]bytecode.Value),
	}
	
	d.callStack = append(d.callStack, frame)
}

// OnFunctionExit VM 退出函数时调用
func (d *Debugger) OnFunctionExit(funcName string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	
	if len(d.callStack) > 0 {
		d.callStack = d.callStack[:len(d.callStack)-1]
	}
}

// OnException VM 发生异常时调用
func (d *Debugger) OnException(excType string, message string, file string, line int) {
	d.sendEvent(EventException, "exception", map[string]interface{}{
		"type":    excType,
		"message": message,
		"file":    file,
		"line":    line,
	})
	
	// 在异常处暂停
	d.mu.Lock()
	d.state = StatePaused
	d.mu.Unlock()
}

// SetLocal 设置局部变量（供 VM 调用）
func (d *Debugger) SetLocal(name string, value bytecode.Value) {
	d.mu.Lock()
	defer d.mu.Unlock()
	
	d.locals[name] = value
	
	// 更新当前栈帧的局部变量
	if len(d.callStack) > 0 {
		d.callStack[len(d.callStack)-1].Locals[name] = value
	}
}

// SetGlobal 设置全局变量（供 VM 调用）
func (d *Debugger) SetGlobal(name string, value bytecode.Value) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.globals[name] = value
}

// ============================================================================
// 查询操作
// ============================================================================

// GetCallStack 获取调用栈
func (d *Debugger) GetCallStack() []StackFrame {
	d.mu.RLock()
	defer d.mu.RUnlock()
	
	// 复制调用栈
	stack := make([]StackFrame, len(d.callStack))
	copy(stack, d.callStack)
	
	// 反转顺序（最近的在前）
	for i, j := 0, len(stack)-1; i < j; i, j = i+1, j-1 {
		stack[i], stack[j] = stack[j], stack[i]
	}
	
	return stack
}

// GetLocals 获取局部变量
func (d *Debugger) GetLocals(frameID int) map[string]bytecode.Value {
	d.mu.RLock()
	defer d.mu.RUnlock()
	
	// 从调用栈获取指定帧的局部变量
	if frameID >= 0 && frameID < len(d.callStack) {
		// 反转索引
		idx := len(d.callStack) - 1 - frameID
		return d.callStack[idx].Locals
	}
	
	return d.locals
}

// GetGlobals 获取全局变量
func (d *Debugger) GetGlobals() map[string]bytecode.Value {
	d.mu.RLock()
	defer d.mu.RUnlock()
	
	result := make(map[string]bytecode.Value)
	for k, v := range d.globals {
		result[k] = v
	}
	return result
}

// GetCurrentPosition 获取当前位置
func (d *Debugger) GetCurrentPosition() (file string, line int, funcName string) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.currentFile, d.currentLine, d.currentFunc
}

// EvaluateExpression 求值表达式
func (d *Debugger) EvaluateExpression(expr string, frameID int) (bytecode.Value, error) {
	// 简单实现：查找变量
	d.mu.RLock()
	defer d.mu.RUnlock()
	
	// 先在局部变量中查找
	if frameID >= 0 && frameID < len(d.callStack) {
		idx := len(d.callStack) - 1 - frameID
		if val, ok := d.callStack[idx].Locals[expr]; ok {
			return val, nil
		}
	} else if val, ok := d.locals[expr]; ok {
		return val, nil
	}
	
	// 再在全局变量中查找
	if val, ok := d.globals[expr]; ok {
		return val, nil
	}
	
	return bytecode.NullValue, fmt.Errorf("cannot evaluate expression: %s", expr)
}

// ============================================================================
// 内部方法
// ============================================================================

// sendEvent 发送事件
func (d *Debugger) sendEvent(eventType EventType, reason string, data interface{}) {
	event := DebugEvent{
		Type:   eventType,
		Reason: reason,
		Data:   data,
		SeqID:  atomic.AddInt64(&d.sequenceID, 1),
	}
	
	select {
	case d.eventChan <- event:
	default:
		// 丢弃事件（通道满）
	}
}

// Reset 重置调试器
func (d *Debugger) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	
	d.state = StateRunning
	d.stepAction = StepNone
	d.callStack = nil
	d.locals = make(map[string]bytecode.Value)
	d.currentFile = ""
	d.currentLine = 0
	d.currentFunc = ""
}
