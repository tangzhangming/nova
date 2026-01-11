package vm

import (
	"github.com/tangzhangming/nova/internal/bytecode"
)

// ============================================================================
// VM 核心结构
// ============================================================================

// StackSize 默认操作数栈大小
const StackSize = 1024

// CallStackSize 默认调用栈大小
const CallStackSize = 256

// GlobalsSize 默认全局变量数量
const GlobalsSize = 1024

// VM 虚拟机
type VM struct {
	// 操作数栈
	stack [StackSize]bytecode.Value
	sp    int // 栈指针 (指向下一个空位)

	// 调用栈
	frames [CallStackSize]CallFrame
	fp     int // 帧指针 (当前帧索引)

	// 全局变量
	globals []bytecode.Value

	// 运行时状态
	chunk *bytecode.Chunk // 当前字节码
	ip    int             // 指令指针

	// 类和函数注册表
	classes   map[string]*bytecode.Class
	functions map[string]*bytecode.Function

	// 错误处理
	hasError bool
	errorMsg string

	// 统计信息
	stats VMStats
}

// CallFrame 调用帧
type CallFrame struct {
	function *bytecode.Function // 当前函数
	closure  *bytecode.Closure  // 当前闭包 (可能为 nil)
	ip       int                // 返回地址
	bp       int                // 基指针 (栈基址)
	chunk    *bytecode.Chunk    // 函数的字节码
}

// VMStats 虚拟机统计信息
type VMStats struct {
	InstructionsExecuted uint64 // 执行的指令数
	FunctionCalls        uint64 // 函数调用次数
	Allocations          uint64 // 分配次数
}

// ============================================================================
// VM 生命周期
// ============================================================================

// New 创建新的虚拟机
func New() *VM {
	vm := &VM{
		globals:   make([]bytecode.Value, GlobalsSize),
		classes:   make(map[string]*bytecode.Class),
		functions: make(map[string]*bytecode.Function),
	}
	return vm
}

// Reset 重置虚拟机状态 (用于复用)
func (vm *VM) Reset() {
	vm.sp = 0
	vm.fp = 0
	vm.ip = 0
	vm.chunk = nil
	vm.hasError = false
	vm.errorMsg = ""
	vm.stats = VMStats{}
}

// ============================================================================
// 栈操作 (内联友好)
// ============================================================================

// push 压栈
func (vm *VM) push(v bytecode.Value) {
	vm.stack[vm.sp] = v
	vm.sp++
}

// pop 弹栈
func (vm *VM) pop() bytecode.Value {
	vm.sp--
	return vm.stack[vm.sp]
}

// peek 查看栈顶 (不弹出)
func (vm *VM) peek(distance int) bytecode.Value {
	return vm.stack[vm.sp-1-distance]
}

// popN 弹出 n 个值
func (vm *VM) popN(n int) {
	vm.sp -= n
}

// ============================================================================
// 帧操作
// ============================================================================

// pushFrame 压入调用帧
func (vm *VM) pushFrame(fn *bytecode.Function, bp int) {
	frame := &vm.frames[vm.fp]
	frame.function = fn
	frame.closure = nil
	frame.chunk = fn.Chunk
	frame.ip = 0
	frame.bp = bp
	vm.fp++
	vm.stats.FunctionCalls++
}

// pushClosureFrame 压入闭包调用帧
func (vm *VM) pushClosureFrame(closure *bytecode.Closure, bp int) {
	frame := &vm.frames[vm.fp]
	frame.function = closure.Function
	frame.closure = closure
	frame.chunk = closure.Function.Chunk
	frame.ip = 0
	frame.bp = bp
	vm.fp++
	vm.stats.FunctionCalls++
}

// popFrame 弹出调用帧
func (vm *VM) popFrame() *CallFrame {
	vm.fp--
	return &vm.frames[vm.fp]
}

// currentFrame 获取当前帧
func (vm *VM) currentFrame() *CallFrame {
	return &vm.frames[vm.fp-1]
}

// ============================================================================
// 局部变量访问
// ============================================================================

// getLocal 获取局部变量
func (vm *VM) getLocal(slot int) bytecode.Value {
	frame := vm.currentFrame()
	return vm.stack[frame.bp+slot]
}

// setLocal 设置局部变量
func (vm *VM) setLocal(slot int, v bytecode.Value) {
	frame := vm.currentFrame()
	vm.stack[frame.bp+slot] = v
}

// ============================================================================
// 全局变量访问
// ============================================================================

// GetGlobal 获取全局变量
func (vm *VM) GetGlobal(index int) bytecode.Value {
	if index < 0 || index >= len(vm.globals) {
		return bytecode.NullValue
	}
	return vm.globals[index]
}

// SetGlobal 设置全局变量
func (vm *VM) SetGlobal(index int, v bytecode.Value) {
	if index >= 0 && index < len(vm.globals) {
		vm.globals[index] = v
	}
}

// ============================================================================
// 类和函数注册
// ============================================================================

// RegisterClass 注册类
func (vm *VM) RegisterClass(class *bytecode.Class) {
	vm.classes[class.FullName()] = class
}

// GetClass 获取类
func (vm *VM) GetClass(name string) *bytecode.Class {
	return vm.classes[name]
}

// RegisterFunction 注册函数
func (vm *VM) RegisterFunction(fn *bytecode.Function) {
	vm.functions[fn.Name] = fn
}

// GetFunction 获取函数
func (vm *VM) GetFunction(name string) *bytecode.Function {
	return vm.functions[name]
}

// ============================================================================
// 字节码读取
// ============================================================================

// readByte 读取单字节
func (vm *VM) readByte() byte {
	frame := vm.currentFrame()
	b := frame.chunk.Code[frame.ip]
	frame.ip++
	vm.stats.InstructionsExecuted++
	return b
}

// readShort 读取双字节 (big endian)
func (vm *VM) readShort() uint16 {
	frame := vm.currentFrame()
	hi := uint16(frame.chunk.Code[frame.ip])
	lo := uint16(frame.chunk.Code[frame.ip+1])
	frame.ip += 2
	return (hi << 8) | lo
}

// readConstant 读取常量
func (vm *VM) readConstant() bytecode.Value {
	frame := vm.currentFrame()
	index := vm.readShort()
	return frame.chunk.Constants[index]
}

// ============================================================================
// 错误处理
// ============================================================================

// runtimeError 设置运行时错误
func (vm *VM) runtimeError(format string, args ...interface{}) {
	vm.hasError = true
	vm.errorMsg = format
	// TODO: 格式化错误消息
}

// HasError 检查是否有错误
func (vm *VM) HasError() bool {
	return vm.hasError
}

// GetError 获取错误消息
func (vm *VM) GetError() string {
	return vm.errorMsg
}

// ============================================================================
// 统计信息
// ============================================================================

// Stats 获取统计信息
func (vm *VM) Stats() VMStats {
	return vm.stats
}

// ============================================================================
// 调试支持
// ============================================================================

// StackTop 获取栈顶元素 (调试用)
func (vm *VM) StackTop() bytecode.Value {
	if vm.sp == 0 {
		return bytecode.NullValue
	}
	return vm.stack[vm.sp-1]
}

// StackDepth 获取栈深度 (调试用)
func (vm *VM) StackDepth() int {
	return vm.sp
}

// CallDepth 获取调用深度 (调试用)
func (vm *VM) CallDepth() int {
	return vm.fp
}
