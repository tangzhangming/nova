// bridge.go - VM-JIT 桥接
//
// 本文件提供了 VM 和 JIT 编译代码之间的桥接接口。
// 主要功能：
// 1. 将 Sola 运行时值转换为 JIT 期望的格式
// 2. 调用 JIT 编译的函数
// 3. 将 JIT 返回值转换回 Sola 运行时值
// 4. 从 JIT 代码调用 VM 函数
// 5. 处理对象操作
//
// 调用约定：
// - JIT 函数接收 int64 参数并返回 int64
// - 对于 float 类型，使用 IEEE 754 位表示传递
// - 对象和数组通过指针传递

package jit

import (
	"fmt"
	"math"
	"sync"
	"unsafe"

	"github.com/tangzhangming/nova/internal/bytecode"
)

// ExecuteResult JIT 执行结果
type ExecuteResult struct {
	Value   bytecode.Value // 返回值
	Success bool           // 是否成功执行
}

// JITCapability JIT能力级别
type JITCapability int

const (
	// JITDisabled JIT被禁用
	JITDisabled JITCapability = iota
	// JITBasic 基础JIT（算术、数组操作）
	JITBasic
	// JITWithCalls 支持函数调用的JIT
	JITWithCalls
	// JITWithObjects 支持对象操作的JIT
	JITWithObjects
	// JITFull 完全JIT支持
	JITFull
)

// CanJIT 检查函数是否可以被 JIT 编译
// 只有满足特定条件的函数才能被 JIT 编译
func CanJIT(fn *bytecode.Function) bool {
	return CanJITWithLevel(fn) != JITDisabled
}

// CanJITWithLevel 检查函数的 JIT 能力级别
func CanJITWithLevel(fn *bytecode.Function) JITCapability {
	if fn == nil || fn.Chunk == nil {
		return JITDisabled
	}
	
	// 不支持可变参数函数
	if fn.IsVariadic {
		return JITDisabled
	}
	
	// 不支持闭包（暂时）
	if fn.UpvalueCount > 0 {
		return JITDisabled
	}
	
	level := JITFull
	
	// 检查是否包含不支持的操作码
	code := fn.Chunk.Code
	ip := 0
	for ip < len(code) {
		op := bytecode.OpCode(code[ip])
		
		switch op {
		// 完全不支持的操作
		case bytecode.OpNewArray, // 创建数组需要复杂的内存分配
			bytecode.OpNewMap, bytecode.OpMapGet, bytecode.OpMapSet, bytecode.OpMapHas, bytecode.OpMapLen,
			bytecode.OpClosure, // 闭包需要复杂的环境捕获
			bytecode.OpThrow, bytecode.OpEnterTry, bytecode.OpLeaveTry, // 异常处理
			bytecode.OpEnterCatch, bytecode.OpEnterFinally, bytecode.OpLeaveFinally, bytecode.OpRethrow,
			bytecode.OpConcat, bytecode.OpStringBuilderNew, bytecode.OpStringBuilderAdd, bytecode.OpStringBuilderBuild,
			bytecode.OpIterInit, bytecode.OpIterNext, bytecode.OpIterKey, bytecode.OpIterValue,
			bytecode.OpSuperArrayNew, bytecode.OpSuperArrayGet, bytecode.OpSuperArraySet:
			return JITDisabled
		
		// 函数调用（需要 JITWithCalls 级别）
		case bytecode.OpCall, bytecode.OpTailCall, bytecode.OpCallMethod, bytecode.OpCallStatic:
			if level > JITWithCalls {
				level = JITWithCalls
			}
		
		// 对象操作（需要 JITWithObjects 级别）
		case bytecode.OpNewObject, bytecode.OpGetField, bytecode.OpSetField,
			bytecode.OpGetStatic, bytecode.OpSetStatic:
			if level > JITWithObjects {
				level = JITWithObjects
			}
		
		// 全局变量（需要 JITWithCalls 级别，因为需要运行时查找）
		case bytecode.OpLoadGlobal, bytecode.OpStoreGlobal:
			if level > JITWithCalls {
				level = JITWithCalls
			}
		
		// 基础支持的操作
		case bytecode.OpArrayGet, bytecode.OpArraySet, bytecode.OpArrayLen,
			bytecode.OpLoop, bytecode.OpJump, bytecode.OpJumpIfFalse, bytecode.OpJumpIfTrue:
			// 已实现支持
		}
		
		ip += instrSize(op, ip, code)
	}
	
	return level
}

// instrSize 获取指令大小
func instrSize(op bytecode.OpCode, ip int, code []byte) int {
	switch op {
	case bytecode.OpPush, bytecode.OpLoadLocal, bytecode.OpStoreLocal,
		bytecode.OpLoadGlobal, bytecode.OpStoreGlobal,
		bytecode.OpNewObject, bytecode.OpGetField, bytecode.OpSetField,
		bytecode.OpNewArray, bytecode.OpNewMap,
		bytecode.OpCheckType, bytecode.OpCast, bytecode.OpCastSafe,
		bytecode.OpSuperArrayNew, bytecode.OpClosure:
		return 3
	case bytecode.OpNewFixedArray:
		return 5
	case bytecode.OpJump, bytecode.OpJumpIfFalse, bytecode.OpJumpIfTrue, bytecode.OpLoop:
		return 3
	case bytecode.OpCall, bytecode.OpTailCall:
		return 2
	case bytecode.OpCallMethod:
		return 4
	case bytecode.OpGetStatic, bytecode.OpSetStatic:
		return 5
	case bytecode.OpCallStatic:
		return 6
	case bytecode.OpEnterTry:
		if ip+1 < len(code) {
			catchCount := int(code[ip+1])
			return 4 + catchCount*4
		}
		return 4
	case bytecode.OpEnterCatch:
		return 3
	default:
		return 1
	}
}

// ============================================================================
// 值转换函数
// ============================================================================

// ValueToInt64 将 Sola 值转换为 int64
// 对于 float 类型，使用 IEEE 754 双精度位表示以保持精度
func ValueToInt64(v bytecode.Value) int64 {
	switch v.Type {
	case bytecode.ValInt:
		return v.AsInt()
	case bytecode.ValFloat:
		// 使用 IEEE 754 位表示保持精度
		return int64(math.Float64bits(v.AsFloat()))
	case bytecode.ValBool:
		if v.AsBool() {
			return 1
		}
		return 0
	case bytecode.ValNull:
		return 0
	default:
		return 0
	}
}

// Int64ToValue 将 int64 转换回 Sola 值
// 默认转换为整数类型
func Int64ToValue(v int64) bytecode.Value {
	return bytecode.NewInt(v)
}

// Int64ToFloatValue 将 int64（IEEE 754 位表示）转换回 float 值
func Int64ToFloatValue(v int64) bytecode.Value {
	return bytecode.NewFloat(math.Float64frombits(uint64(v)))
}

// FloatBitsToInt64 将 float64 转换为 IEEE 754 位表示的 int64
func FloatBitsToInt64(f float64) int64 {
	return int64(math.Float64bits(f))
}

// Int64ToFloatBits 将 IEEE 754 位表示的 int64 转换为 float64
func Int64ToFloatBits(v int64) float64 {
	return math.Float64frombits(uint64(v))
}

// ============================================================================
// VM 桥接接口
// ============================================================================

// VMBridge VM与JIT之间的桥接接口
// 这个结构体存储了JIT代码调用VM时需要的上下文
type VMBridge struct {
	mu sync.RWMutex
	
	// 函数解析器
	funcResolver FunctionResolver
	
	// 方法解析器
	methodResolver MethodResolver
	
	// 对象分配器
	objectAllocator ObjectAllocator
	
	// 字段访问器
	fieldAccessor FieldAccessor
	
	// 全局变量访问器
	globalAccessor GlobalAccessor
	
	// 已注册的函数表
	functions map[string]FunctionEntry
	
	// 类定义表
	classes map[string]*ClassLayout
}

// FunctionResolver 函数解析接口
type FunctionResolver interface {
	// ResolveFunction 解析函数名到函数定义
	ResolveFunction(name string) (*bytecode.Function, bool)
	
	// ResolveMethod 解析方法名到方法定义
	ResolveMethod(className, methodName string) (*bytecode.Function, bool)
}

// MethodResolver 方法调用接口
type MethodResolver interface {
	// CallMethod 调用对象方法
	CallMethod(receiver uintptr, methodName string, args []int64) (int64, error)
}

// ObjectAllocator 对象分配接口
type ObjectAllocator interface {
	// AllocateObject 分配新对象
	AllocateObject(className string) (uintptr, error)
}

// FieldAccessor 字段访问接口
type FieldAccessor interface {
	// GetField 获取对象字段
	GetField(obj uintptr, fieldName string) (int64, error)
	
	// SetField 设置对象字段
	SetField(obj uintptr, fieldName string, value int64) error
}

// GlobalAccessor 全局变量访问接口
type GlobalAccessor interface {
	// GetGlobal 获取全局变量
	GetGlobal(name string) (bytecode.Value, bool)
	
	// SetGlobal 设置全局变量
	SetGlobal(name string, value bytecode.Value)
}

// FunctionEntry 函数表条目
type FunctionEntry struct {
	Function    *bytecode.Function
	Compiled    *CompiledFunc
	EntryPoint  uintptr
}

// ClassLayout 类内存布局信息
type ClassLayout struct {
	Name       string
	Size       int
	Fields     map[string]FieldLayout
	VTablePtr  int  // 虚表指针偏移
	Methods    map[string]uintptr
}

// FieldLayout 字段布局
type FieldLayout struct {
	Name   string
	Offset int
	Type   ValueType
}

// 全局VM桥接实例
var globalBridge *VMBridge
var bridgeOnce sync.Once

// GetBridge 获取全局VM桥接实例
func GetBridge() *VMBridge {
	bridgeOnce.Do(func() {
		globalBridge = &VMBridge{
			functions: make(map[string]FunctionEntry),
			classes:   make(map[string]*ClassLayout),
		}
	})
	return globalBridge
}

// SetFunctionResolver 设置函数解析器
func (b *VMBridge) SetFunctionResolver(resolver FunctionResolver) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.funcResolver = resolver
}

// SetMethodResolver 设置方法解析器
func (b *VMBridge) SetMethodResolver(resolver MethodResolver) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.methodResolver = resolver
}

// SetObjectAllocator 设置对象分配器
func (b *VMBridge) SetObjectAllocator(allocator ObjectAllocator) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.objectAllocator = allocator
}

// SetFieldAccessor 设置字段访问器
func (b *VMBridge) SetFieldAccessor(accessor FieldAccessor) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.fieldAccessor = accessor
}

// SetGlobalAccessor 设置全局变量访问器
func (b *VMBridge) SetGlobalAccessor(accessor GlobalAccessor) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.globalAccessor = accessor
}

// RegisterFunction 注册函数
func (b *VMBridge) RegisterFunction(name string, fn *bytecode.Function, compiled *CompiledFunc) {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	entry := FunctionEntry{
		Function: fn,
		Compiled: compiled,
	}
	if compiled != nil {
		entry.EntryPoint = compiled.EntryPoint()
	}
	b.functions[name] = entry
}

// GetFunction 获取已注册的函数
func (b *VMBridge) GetFunction(name string) (FunctionEntry, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	entry, ok := b.functions[name]
	return entry, ok
}

// RegisterClass 注册类布局
func (b *VMBridge) RegisterClass(layout *ClassLayout) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.classes[layout.Name] = layout
}

// GetClassLayout 获取类布局
func (b *VMBridge) GetClassLayout(name string) (*ClassLayout, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	layout, ok := b.classes[name]
	return layout, ok
}

// ============================================================================
// JIT调用VM的辅助函数
// ============================================================================

// JITCallFunction 从JIT代码调用函数
// 这是JIT代码通过辅助函数间接调用的入口点
func JITCallFunction(funcName string, args []int64) (int64, error) {
	bridge := GetBridge()
	
	// 首先检查是否有已编译的版本
	if entry, ok := bridge.GetFunction(funcName); ok {
		if entry.Compiled != nil && entry.EntryPoint != 0 {
			// 直接调用JIT编译的代码
			return executeCompiledFunction(entry.EntryPoint, args)
		}
		// 回退到解释器
		return callInterpreted(entry.Function, args)
	}
	
	// 尝试通过函数解析器解析
	bridge.mu.RLock()
	resolver := bridge.funcResolver
	bridge.mu.RUnlock()
	
	if resolver != nil {
		if fn, ok := resolver.ResolveFunction(funcName); ok {
			return callInterpreted(fn, args)
		}
	}
	
	return 0, fmt.Errorf("function not found: %s", funcName)
}

// JITCallMethod 从JIT代码调用方法
func JITCallMethod(receiver uintptr, methodName string, args []int64) (int64, error) {
	bridge := GetBridge()
	
	bridge.mu.RLock()
	resolver := bridge.methodResolver
	bridge.mu.RUnlock()
	
	if resolver != nil {
		return resolver.CallMethod(receiver, methodName, args)
	}
	
	return 0, fmt.Errorf("method resolver not set")
}

// JITAllocateObject 从JIT代码分配对象
func JITAllocateObject(className string) (uintptr, error) {
	bridge := GetBridge()
	
	bridge.mu.RLock()
	allocator := bridge.objectAllocator
	bridge.mu.RUnlock()
	
	if allocator != nil {
		return allocator.AllocateObject(className)
	}
	
	return 0, fmt.Errorf("object allocator not set")
}

// JITGetField 从JIT代码获取字段
func JITGetField(obj uintptr, fieldName string) (int64, error) {
	bridge := GetBridge()
	
	// 首先尝试使用缓存的偏移
	// TODO: 实现内联缓存
	
	bridge.mu.RLock()
	accessor := bridge.fieldAccessor
	bridge.mu.RUnlock()
	
	if accessor != nil {
		return accessor.GetField(obj, fieldName)
	}
	
	return 0, fmt.Errorf("field accessor not set")
}

// JITSetField 从JIT代码设置字段
func JITSetField(obj uintptr, fieldName string, value int64) error {
	bridge := GetBridge()
	
	bridge.mu.RLock()
	accessor := bridge.fieldAccessor
	bridge.mu.RUnlock()
	
	if accessor != nil {
		return accessor.SetField(obj, fieldName, value)
	}
	
	return fmt.Errorf("field accessor not set")
}

// JITGetGlobal 从JIT代码获取全局变量
func JITGetGlobal(name string) (int64, error) {
	bridge := GetBridge()
	
	bridge.mu.RLock()
	accessor := bridge.globalAccessor
	bridge.mu.RUnlock()
	
	if accessor != nil {
		if val, ok := accessor.GetGlobal(name); ok {
			return ValueToInt64(val), nil
		}
		return 0, fmt.Errorf("global variable not found: %s", name)
	}
	
	return 0, fmt.Errorf("global accessor not set")
}

// JITSetGlobal 从JIT代码设置全局变量
func JITSetGlobal(name string, value int64) error {
	bridge := GetBridge()
	
	bridge.mu.RLock()
	accessor := bridge.globalAccessor
	bridge.mu.RUnlock()
	
	if accessor != nil {
		accessor.SetGlobal(name, Int64ToValue(value))
		return nil
	}
	
	return fmt.Errorf("global accessor not set")
}

// ============================================================================
// 内部辅助函数
// ============================================================================

// executeCompiledFunction 执行已编译的函数
// 这里使用平台特定的调用代码（在 bridge_amd64.go 等文件中实现）
func executeCompiledFunction(entryPoint uintptr, args []int64) (int64, error) {
	// 这个函数的实际实现在平台特定的文件中
	// 这里提供一个占位实现
	_ = entryPoint
	_ = args
	return 0, fmt.Errorf("direct JIT execution not implemented on this platform")
}

// callInterpreted 调用解释执行的函数
// 这需要通过VM来执行
func callInterpreted(fn *bytecode.Function, args []int64) (int64, error) {
	// 这个函数需要VM的支持
	// 暂时返回错误
	_ = fn
	_ = args
	return 0, fmt.Errorf("interpreted execution not implemented")
}

// ============================================================================
// 字段偏移计算
// ============================================================================

// ComputeFieldOffset 计算字段在对象中的偏移
func ComputeFieldOffset(layout *ClassLayout, fieldName string) int {
	if layout == nil {
		return -1
	}
	if field, ok := layout.Fields[fieldName]; ok {
		return field.Offset
	}
	return -1
}

// GetOrComputeFieldOffset 获取或计算字段偏移
// 这个函数用于优化字段访问
func GetOrComputeFieldOffset(className, fieldName string) int {
	bridge := GetBridge()
	
	if layout, ok := bridge.GetClassLayout(className); ok {
		return ComputeFieldOffset(layout, fieldName)
	}
	
	return -1
}

// ============================================================================
// 值指针转换
// ============================================================================

// ValuePtrToUintptr 将bytecode.Value指针转换为uintptr
func ValuePtrToUintptr(v *bytecode.Value) uintptr {
	return uintptr(unsafe.Pointer(v))
}

// UintptrToValuePtr 将uintptr转换为bytecode.Value指针
func UintptrToValuePtr(ptr uintptr) *bytecode.Value {
	return (*bytecode.Value)(unsafe.Pointer(ptr))
}
