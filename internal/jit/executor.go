// +build amd64

package jit

import (
	"fmt"
	"reflect"
	"sync"
	"unsafe"

	"github.com/tangzhangming/nova/internal/bytecode"
)

// ============================================================================
// 可执行内存管理
// ============================================================================

// ExecutableMemory 可执行内存块
type ExecutableMemory struct {
	addr   uintptr
	size   int
	used   int
	rwAddr uintptr // 读写地址 (用于写入代码)
}

// MemoryAllocator 内存分配器
type MemoryAllocator struct {
	blocks []*ExecutableMemory
	mu     sync.Mutex

	// 统计
	totalAllocated int
	totalUsed      int
}

// 页大小
const pageSize = 4096

// 默认块大小 (1MB)
const defaultBlockSize = 1024 * 1024

// NewMemoryAllocator 创建内存分配器
func NewMemoryAllocator() *MemoryAllocator {
	return &MemoryAllocator{
		blocks: make([]*ExecutableMemory, 0),
	}
}

// Allocate 分配可执行内存
func (ma *MemoryAllocator) Allocate(size int) (*ExecutableMemory, error) {
	ma.mu.Lock()
	defer ma.mu.Unlock()

	// 对齐到 16 字节
	size = (size + 15) & ^15

	// 尝试在现有块中分配
	for _, block := range ma.blocks {
		if block.size-block.used >= size {
			mem := &ExecutableMemory{
				addr: block.addr + uintptr(block.used),
				size: size,
			}
			block.used += size
			ma.totalUsed += size
			return mem, nil
		}
	}

	// 分配新块
	blockSize := defaultBlockSize
	if size > blockSize {
		blockSize = (size + pageSize - 1) & ^(pageSize - 1)
	}

	block, err := ma.allocateBlock(blockSize)
	if err != nil {
		return nil, err
	}

	ma.blocks = append(ma.blocks, block)
	ma.totalAllocated += blockSize

	// 从新块分配
	mem := &ExecutableMemory{
		addr: block.addr,
		size: size,
	}
	block.used = size
	ma.totalUsed += size

	return mem, nil
}

// allocateBlock 分配内存块 (平台相关)
// 注意: 这是一个简化实现，实际需要使用 mmap/VirtualAlloc
func (ma *MemoryAllocator) allocateBlock(size int) (*ExecutableMemory, error) {
	// 使用 Go 分配内存 (简化实现)
	// 实际生产环境需要使用系统调用分配可执行内存
	data := make([]byte, size)
	
	return &ExecutableMemory{
		addr:   uintptr(unsafe.Pointer(&data[0])),
		size:   size,
		rwAddr: uintptr(unsafe.Pointer(&data[0])),
	}, nil
}

// Free 释放所有内存
func (ma *MemoryAllocator) Free() {
	ma.mu.Lock()
	defer ma.mu.Unlock()

	ma.blocks = nil
	ma.totalAllocated = 0
	ma.totalUsed = 0
}

// Stats 返回统计信息
func (ma *MemoryAllocator) Stats() (allocated, used int) {
	ma.mu.Lock()
	defer ma.mu.Unlock()
	return ma.totalAllocated, ma.totalUsed
}

// ============================================================================
// 编译代码执行器
// ============================================================================

// Executor JIT 代码执行器
type Executor struct {
	allocator *MemoryAllocator
	compiler  *JITCompiler
	cache     sync.Map // *bytecode.Function -> *InstalledCode

	// 配置
	enabled bool
}

// InstalledCode 已安装的代码
type InstalledCode struct {
	Function  *bytecode.Function
	Code      *CompiledCode
	Memory    *ExecutableMemory
	EntryFunc func(args []bytecode.Value) bytecode.Value
}

// NewExecutor 创建执行器
func NewExecutor() *Executor {
	return &Executor{
		allocator: NewMemoryAllocator(),
		compiler:  NewCompiler(DefaultConfig()),
		enabled:   true,
	}
}

// SetEnabled 设置是否启用 JIT
func (e *Executor) SetEnabled(enabled bool) {
	e.enabled = enabled
}

// IsEnabled 是否启用 JIT
func (e *Executor) IsEnabled() bool {
	return e.enabled
}

// Compile 编译函数
func (e *Executor) Compile(fn *bytecode.Function) (*InstalledCode, error) {
	if !e.enabled {
		return nil, fmt.Errorf("JIT is disabled")
	}

	// 检查缓存
	if cached, ok := e.cache.Load(fn); ok {
		return cached.(*InstalledCode), nil
	}

	// 编译
	compiled, err := e.compiler.Compile(fn)
	if err != nil {
		return nil, err
	}

	// 安装代码
	installed, err := e.install(fn, compiled)
	if err != nil {
		return nil, err
	}

	// 缓存
	e.cache.Store(fn, installed)

	return installed, nil
}

// install 安装编译后的代码
func (e *Executor) install(fn *bytecode.Function, code *CompiledCode) (*InstalledCode, error) {
	if len(code.Code) == 0 {
		// 代码为空，使用 stub
		return &InstalledCode{
			Function: fn,
			Code:     code,
		}, nil
	}

	// 使用全局代码缓存安装代码
	entryAddr, err := GlobalCodeCache.Install(code.Code)
	if err != nil {
		return nil, err
	}

	// 创建一个简单的 ExecutableMemory 结构体来保持兼容性
	mem := &ExecutableMemory{
		addr: entryAddr,
		size: len(code.Code),
		used: len(code.Code),
	}

	// 应用重定位
	e.applyRelocations(mem, code)

	// 创建函数指针
	installed := &InstalledCode{
		Function: fn,
		Code:     code,
		Memory:   mem,
	}

	// 设置入口点
	code.Entry = entryAddr

	return installed, nil
}

// applyRelocations 应用重定位
func (e *Executor) applyRelocations(mem *ExecutableMemory, code *CompiledCode) {
	codeBase := mem.addr
	codePtr := unsafe.Slice((*byte)(unsafe.Pointer(mem.addr)), len(code.Code))

	for _, reloc := range code.HelperRefs {
		offset := reloc.Offset
		target := reloc.Addr

		switch {
		case offset+8 <= len(codePtr):
			// 64 位绝对地址
			*(*uint64)(unsafe.Pointer(&codePtr[offset])) = uint64(target)
		case offset+4 <= len(codePtr):
			// 32 位相对地址
			rel := int32(target - (codeBase + uintptr(offset) + 4))
			*(*int32)(unsafe.Pointer(&codePtr[offset])) = rel
		}
	}
}

// Execute 执行已编译的函数
func (e *Executor) Execute(installed *InstalledCode, args []bytecode.Value) (bytecode.Value, error) {
	if installed == nil || installed.Code == nil {
		return bytecode.NullValue, fmt.Errorf("no compiled code")
	}

	if installed.EntryFunc != nil {
		return installed.EntryFunc(args), nil
	}

	// 没有可执行代码，返回错误
	return bytecode.NullValue, fmt.Errorf("code not executable")
}

// Invalidate 使函数缓存失效
func (e *Executor) Invalidate(fn *bytecode.Function) {
	e.cache.Delete(fn)
	e.compiler.Invalidate(fn)
}

// Reset 重置执行器
func (e *Executor) Reset() {
	e.cache = sync.Map{}
	e.compiler.Reset()
	e.allocator.Free()
}

// GetStats 获取统计信息
func (e *Executor) GetStats() ExecutorStats {
	allocated, used := e.allocator.Stats()
	compilerStats := e.compiler.GetStats()

	return ExecutorStats{
		CodeAllocated:   allocated,
		CodeUsed:        used,
		CompiledFuncs:   int(compilerStats.TotalCompiled),
		TotalIRInsts:    int(compilerStats.TotalIRInsts),
		CacheHits:       int(compilerStats.CacheHits),
		CacheMisses:     int(compilerStats.CacheMisses),
		HotspotCompiles: int(compilerStats.HotspotCompiles),
	}
}

// ExecutorStats 执行器统计
type ExecutorStats struct {
	CodeAllocated   int
	CodeUsed        int
	CompiledFuncs   int
	TotalIRInsts    int
	CacheHits       int
	CacheMisses     int
	HotspotCompiles int
}

// ============================================================================
// 全局执行器
// ============================================================================

// DefaultExecutor 默认执行器
var DefaultExecutor = NewExecutor()

// Compile 编译函数 (使用默认执行器)
func Compile(fn *bytecode.Function) (*InstalledCode, error) {
	return DefaultExecutor.Compile(fn)
}

// Execute 执行函数 (使用默认执行器)
func Execute(installed *InstalledCode, args []bytecode.Value) (bytecode.Value, error) {
	return DefaultExecutor.Execute(installed, args)
}

// ============================================================================
// 函数指针转换
// ============================================================================

// MakeFunc 创建可调用的函数
// 注意: 这是一个简化实现，实际需要处理调用约定
func MakeFunc(entry uintptr, argCount int) func([]bytecode.Value) bytecode.Value {
	if entry == 0 {
		return nil
	}

	// 使用 reflect 创建函数 (简化)
	// 实际实现需要处理参数传递
	return func(args []bytecode.Value) bytecode.Value {
		// 这里需要实际调用机器码
		// 当前返回 null 作为占位符
		return bytecode.NullValue
	}
}

// callNativeEntry 调用原生函数入口点
// 这是一个底层函数，用于实际调用 JIT 生成的代码
func callNativeEntry(entry uintptr, args ...uintptr) uintptr {
	if entry == 0 {
		return 0
	}

	// 使用 reflect 调用
	// 注意: 这只是一个示例，实际实现需要使用 asm
	fn := *(*func() uintptr)(unsafe.Pointer(&entry))
	return reflect.ValueOf(fn).Pointer()
}

// ============================================================================
// JIT 入口点
// ============================================================================

// TryJIT 尝试 JIT 编译并执行函数
// 如果编译失败或未启用，返回 false
func TryJIT(fn *bytecode.Function, args []bytecode.Value) (bytecode.Value, bool) {
	if !DefaultExecutor.enabled {
		return bytecode.NullValue, false
	}

	// 检查是否已编译
	if cached, ok := DefaultExecutor.cache.Load(fn); ok {
		installed := cached.(*InstalledCode)
		if result, err := DefaultExecutor.Execute(installed, args); err == nil {
			return result, true
		}
	}

	// 尝试编译
	installed, err := DefaultExecutor.Compile(fn)
	if err != nil {
		return bytecode.NullValue, false
	}

	// 执行
	result, err := DefaultExecutor.Execute(installed, args)
	if err != nil {
		return bytecode.NullValue, false
	}

	return result, true
}

// ============================================================================
// 混合执行模式
// ============================================================================

// HybridRunner 混合执行器
type HybridRunner struct {
	executor    *Executor
	interpreter func(*bytecode.Function, []bytecode.Value) bytecode.Value
	threshold   int64
	execCounts  sync.Map // *bytecode.Function -> *int64
}

// NewHybridRunner 创建混合执行器
func NewHybridRunner(interp func(*bytecode.Function, []bytecode.Value) bytecode.Value) *HybridRunner {
	return &HybridRunner{
		executor:    DefaultExecutor,
		interpreter: interp,
		threshold:   1000,
	}
}

// SetThreshold 设置编译阈值
func (hr *HybridRunner) SetThreshold(threshold int64) {
	hr.threshold = threshold
}

// Run 运行函数
func (hr *HybridRunner) Run(fn *bytecode.Function, args []bytecode.Value) bytecode.Value {
	// 获取或创建执行计数
	countPtr, _ := hr.execCounts.LoadOrStore(fn, new(int64))
	count := countPtr.(*int64)

	// 增加计数
	newCount := *count + 1
	*count = newCount

	// 检查是否已编译
	if cached, ok := hr.executor.cache.Load(fn); ok {
		installed := cached.(*InstalledCode)
		if result, err := hr.executor.Execute(installed, args); err == nil {
			return result
		}
		// JIT 执行失败，回退到解释器
	}

	// 检查是否达到编译阈值
	if newCount >= hr.threshold {
		// 尝试编译
		if installed, err := hr.executor.Compile(fn); err == nil {
			if result, err := hr.executor.Execute(installed, args); err == nil {
				return result
			}
		}
	}

	// 使用解释器
	if hr.interpreter != nil {
		return hr.interpreter(fn, args)
	}

	return bytecode.NullValue
}

// Reset 重置
func (hr *HybridRunner) Reset() {
	hr.execCounts = sync.Map{}
	hr.executor.Reset()
}
