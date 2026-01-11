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

	// Profile 配置
	profilingEnabled bool
	currentFunction  *bytecode.Function
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
	HotFunctionsDetected int    // 检测到的热点函数数
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
// 执行结果常量
// ============================================================================

const (
	InterpretOK          = 0 // 执行成功
	InterpretCompileError = 1 // 编译错误
	InterpretRuntimeError = 2 // 运行时错误
)

// ============================================================================
// 类和函数注册
// ============================================================================

// RegisterClass 注册类
func (vm *VM) RegisterClass(class *bytecode.Class) {
	// 注册完整名称
	vm.classes[class.FullName()] = class
	// 同时注册短名（用于简单引用）
	if class.Name != "" && class.Name != class.FullName() {
		vm.classes[class.Name] = class
	}
}

// DefineClass 定义类（RegisterClass 的别名，兼容旧 API）
func (vm *VM) DefineClass(class *bytecode.Class) {
	vm.RegisterClass(class)
}

// GetClass 获取类
func (vm *VM) GetClass(name string) *bytecode.Class {
	return vm.classes[name]
}

// listClasses 列出所有已注册的类
func (vm *VM) listClasses() []string {
	var names []string
	for name := range vm.classes {
		names = append(names, name)
	}
	return names
}

// RegisterFunction 注册函数
func (vm *VM) RegisterFunction(fn *bytecode.Function) {
	vm.functions[fn.Name] = fn
}

// GetFunction 获取函数
func (vm *VM) GetFunction(name string) *bytecode.Function {
	return vm.functions[name]
}

// RegisterBuiltin 注册内置函数
func (vm *VM) RegisterBuiltin(name string, fn *bytecode.Function) {
	vm.functions[name] = fn
}

// ============================================================================
// 枚举注册
// ============================================================================

// enums 枚举注册表（在 VM 结构体外定义，简化结构）
var vmEnums = make(map[string]*bytecode.Enum)

// DefineEnum 定义枚举
func (vm *VM) DefineEnum(enum *bytecode.Enum) {
	vmEnums[enum.Name] = enum
}

// GetEnum 获取枚举
func (vm *VM) GetEnum(name string) *bytecode.Enum {
	return vmEnums[name]
}

// ============================================================================
// 方法调用
// ============================================================================

// CallStaticMethod 调用静态方法
func (vm *VM) CallStaticMethod(class *bytecode.Class, methodName string, args []bytecode.Value) int {
	method := class.GetMethod(methodName)
	if method == nil {
		vm.runtimeError("undefined method: %s.%s", class.Name, methodName)
		return InterpretRuntimeError
	}

	// 设置参数到栈上
	argCount := 0
	if args != nil {
		for _, arg := range args {
			vm.push(arg)
		}
		argCount = len(args)
	}

	// 创建临时 Function 包装 Method 的 Chunk
	fn := &bytecode.Function{
		Name:  method.Name,
		Arity: method.Arity,
		Chunk: method.Chunk,
	}

	// 压入调用帧并执行
	vm.pushFrame(fn, vm.sp-argCount)
	result := vm.runLoop()

	// 检查执行结果
	if vm.hasError {
		return InterpretRuntimeError
	}
	_ = result // 忽略返回值
	return InterpretOK
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

// ============================================================================
// Profile 支持
// ============================================================================

// EnableProfiling 启用 Profile 收集
func (vm *VM) EnableProfiling() {
	vm.profilingEnabled = true
}

// DisableProfiling 禁用 Profile 收集
func (vm *VM) DisableProfiling() {
	vm.profilingEnabled = false
}

// IsProfilingEnabled 检查是否启用 Profile
func (vm *VM) IsProfilingEnabled() bool {
	return vm.profilingEnabled
}

// SetCurrentFunction 设置当前函数 (用于 Profile)
func (vm *VM) SetCurrentFunction(fn *bytecode.Function) {
	vm.currentFunction = fn
}

// GetCurrentFunction 获取当前函数
func (vm *VM) GetCurrentFunction() *bytecode.Function {
	return vm.currentFunction
}

// RecordTypeProfile 记录类型 Profile
func (vm *VM) RecordTypeProfile(ip int, t bytecode.ValueType) {
	if vm.profilingEnabled && vm.currentFunction != nil {
		RecordType(vm.currentFunction, ip, t)
	}
}

// RecordBranchProfile 记录分支 Profile
func (vm *VM) RecordBranchProfile(ip int, taken bool) {
	if vm.profilingEnabled && vm.currentFunction != nil {
		RecordBranch(vm.currentFunction, ip, taken)
	}
}

// CheckHotFunction 检查并记录热点函数
func (vm *VM) CheckHotFunction(fn *bytecode.Function) bool {
	if !vm.profilingEnabled {
		return false
	}
	if RecordExecution(fn) {
		vm.stats.HotFunctionsDetected++
		return true
	}
	return false
}
