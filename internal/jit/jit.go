// Package jit 实现 Sola 语言的即时编译器 (JIT Compiler)
//
// JIT 编译器将热点函数的字节码编译为本机机器码，以提升执行性能。
// 架构采用经典的编译器流水线：
//
//	字节码 → SSA IR → 优化 → 寄存器分配 → 机器码生成
//
// 支持的特性：
//   - 热点检测和自适应编译
//   - SSA 形式的中间表示
//   - 多种优化 Pass（常量传播、死代码消除、代数简化等）
//   - 线性扫描寄存器分配
//   - x86-64 和 ARM64 代码生成
//   - 可执行内存管理和代码缓存
//   - 支持通过 --jitless 或 -Xint 禁用 JIT
//
// 使用示例：
//
//	compiler := jit.NewCompiler(jit.DefaultConfig())
//	compiled, err := compiler.Compile(function)
//	if err == nil {
//	    result, ok := compiler.Execute(compiled, args)
//	}
package jit

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/tangzhangming/nova/internal/bytecode"
)

// ============================================================================
// JIT 配置
// ============================================================================

// Config JIT 编译器配置
// 可以通过命令行参数或环境变量来调整这些配置
type Config struct {
	// Enabled 是否启用 JIT 编译
	// 对应命令行: --jitless 或 -Xint 可以禁用 JIT
	Enabled bool

	// HotThreshold 函数调用多少次后视为热点
	// 降低此值可以更早触发 JIT 编译，但会增加编译开销
	HotThreshold int64

	// LoopThreshold 循环迭代多少次后触发 OSR
	LoopThreshold int64

	// OptimizationLevel 优化级别 (0-3)
	// 0: 不优化（最快编译）
	// 1: 基本优化（常量传播、死代码消除）
	// 2: 标准优化（包含代数简化、强度削减）
	// 3: 激进优化（包含内联、循环优化）
	OptimizationLevel int

	// MaxCodeCacheSize 代码缓存最大大小（字节）
	MaxCodeCacheSize int

	// EnableInlining 是否启用函数内联
	EnableInlining bool

	// MaxInlineDepth 最大内联深度
	MaxInlineDepth int

	// MaxInlineSize 可内联函数的最大字节码大小
	MaxInlineSize int

	// Debug 是否输出调试信息
	Debug bool

	// DumpIR 是否在编译时打印 IR
	DumpIR bool

	// DumpASM 是否在编译时打印生成的汇编
	DumpASM bool
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		Enabled:           true,
		HotThreshold:      100,   // 调用 100 次后编译
		LoopThreshold:     50,    // 循环 50 次后触发 OSR
		OptimizationLevel: 1,     // 基本优化（常量传播、死代码消除）
		MaxCodeCacheSize:  16 * 1024 * 1024, // 16MB 代码缓存
		EnableInlining:    true,
		MaxInlineDepth:    3,
		MaxInlineSize:     50,    // 50 字节码指令以内可内联
		Debug:             false,
		DumpIR:            false,
		DumpASM:           false,
	}
}

// InterpretOnlyConfig 返回仅解释执行的配置
// 等效于 JVM 的 -Xint 或 V8 的 --jitless
func InterpretOnlyConfig() *Config {
	return &Config{
		Enabled: false,
	}
}

// ============================================================================
// JIT 编译器
// ============================================================================

// Compiler JIT 编译器
// 线程安全，可以在多个 goroutine 中使用
type Compiler struct {
	config    *Config       // 编译器配置
	cache     *CodeCache    // 代码缓存
	profiler  *Profiler     // 性能分析器（热点检测）
	codegen   CodeGenerator // 代码生成器（平台相关）
	
	// 统计信息
	stats     CompilerStats
	
	// 并发控制
	mu        sync.RWMutex
}

// CompilerStats 编译器统计信息
type CompilerStats struct {
	CompileCount    int64   // 编译次数
	CompileTimeNs   int64   // 总编译时间（纳秒）
	ExecuteCount    int64   // JIT 执行次数
	FallbackCount   int64   // 回退到解释器次数
	CacheHits       int64   // 缓存命中次数
	CacheMisses     int64   // 缓存未命中次数
}

// NewCompiler 创建 JIT 编译器
func NewCompiler(config *Config) *Compiler {
	if config == nil {
		config = DefaultConfig()
	}

	// 如果 JIT 被禁用，返回一个最小化的编译器
	if !config.Enabled {
		return &Compiler{
			config: config,
		}
	}

	// 根据当前 CPU 架构选择代码生成器
	var codegen CodeGenerator
	switch runtime.GOARCH {
	case "amd64":
		codegen = NewX64CodeGenerator()
	case "arm64":
		codegen = NewARM64CodeGenerator()
	default:
		// 不支持的架构，禁用 JIT
		config.Enabled = false
		return &Compiler{config: config}
	}

	return &Compiler{
		config:   config,
		cache:    NewCodeCache(config.MaxCodeCacheSize),
		profiler: NewProfiler(config.HotThreshold, config.LoopThreshold),
		codegen:  codegen,
	}
}

// IsEnabled 检查 JIT 是否启用
func (c *Compiler) IsEnabled() bool {
	return c.config != nil && c.config.Enabled
}

// GetConfig 获取当前配置（只读）
func (c *Compiler) GetConfig() Config {
	if c.config == nil {
		return Config{}
	}
	return *c.config
}

// GetStats 获取统计信息
func (c *Compiler) GetStats() CompilerStats {
	return CompilerStats{
		CompileCount:  atomic.LoadInt64(&c.stats.CompileCount),
		CompileTimeNs: atomic.LoadInt64(&c.stats.CompileTimeNs),
		ExecuteCount:  atomic.LoadInt64(&c.stats.ExecuteCount),
		FallbackCount: atomic.LoadInt64(&c.stats.FallbackCount),
		CacheHits:     atomic.LoadInt64(&c.stats.CacheHits),
		CacheMisses:   atomic.LoadInt64(&c.stats.CacheMisses),
	}
}

// GetProfiler 获取性能分析器
func (c *Compiler) GetProfiler() *Profiler {
	return c.profiler
}

// ============================================================================
// 编译流程
// ============================================================================

// Compile 编译函数
// 这是 JIT 编译的主入口点
func (c *Compiler) Compile(fn *bytecode.Function) (*CompiledFunc, error) {
	if !c.IsEnabled() {
		return nil, fmt.Errorf("JIT is disabled")
	}

	if fn == nil || fn.Chunk == nil {
		return nil, fmt.Errorf("invalid function")
	}

	// 检查缓存
	c.mu.RLock()
	if cached := c.cache.Get(fn.Name); cached != nil {
		c.mu.RUnlock()
		atomic.AddInt64(&c.stats.CacheHits, 1)
		return cached, nil
	}
	c.mu.RUnlock()
	atomic.AddInt64(&c.stats.CacheMisses, 1)

	// 开始编译
	c.mu.Lock()
	defer c.mu.Unlock()

	// 双重检查（可能其他 goroutine 已经编译）
	if cached := c.cache.Get(fn.Name); cached != nil {
		atomic.AddInt64(&c.stats.CacheHits, 1)
		return cached, nil
	}

	// 执行编译
	compiled, err := c.compileInternal(fn)
	if err != nil {
		return nil, err
	}

	// 存入缓存
	c.cache.Put(fn.Name, compiled)
	atomic.AddInt64(&c.stats.CompileCount, 1)

	return compiled, nil
}

// compileInternal 内部编译流程
func (c *Compiler) compileInternal(fn *bytecode.Function) (*CompiledFunc, error) {
	// 第一步：字节码 → SSA IR
	builder := NewIRBuilder()
	irFunc, err := builder.Build(fn)
	if err != nil {
		return nil, fmt.Errorf("IR build failed: %w", err)
	}

	if c.config.DumpIR {
		fmt.Printf("=== IR for %s (before optimization) ===\n%s\n", fn.Name, irFunc.String())
	}

	// 第二步：优化
	if c.config.OptimizationLevel > 0 {
		optimizer := NewOptimizer(c.config.OptimizationLevel)
		optimizer.Optimize(irFunc)

		if c.config.DumpIR {
			fmt.Printf("=== IR for %s (after optimization) ===\n%s\n", fn.Name, irFunc.String())
		}
	}

	// 第三步：寄存器分配
	regalloc := NewRegisterAllocator(c.codegen.NumRegisters())
	allocation := regalloc.Allocate(irFunc)

	// 第四步：代码生成
	code, err := c.codegen.Generate(irFunc, allocation)
	if err != nil {
		return nil, fmt.Errorf("code generation failed: %w", err)
	}

	if c.config.DumpASM {
		fmt.Printf("=== Generated code for %s (%d bytes) ===\n", fn.Name, len(code))
		dumpHex(code)
	}

	// 第五步：分配可执行内存并复制代码
	execMem, err := c.cache.AllocateExecutable(len(code))
	if err != nil {
		return nil, fmt.Errorf("failed to allocate executable memory: %w", err)
	}
	copy(execMem, code)

	return &CompiledFunc{
		Name:      fn.Name,
		Code:      execMem,
		StackSize: allocation.StackSize,
		NumArgs:   fn.Arity,
	}, nil
}

// GetCompiled 获取已编译的函数（如果存在）
func (c *Compiler) GetCompiled(name string) *CompiledFunc {
	if !c.IsEnabled() {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cache.Get(name)
}

// ============================================================================
// 已编译函数
// ============================================================================

// CompiledFunc 已编译的函数
type CompiledFunc struct {
	Name      string  // 函数名
	Code      []byte  // 可执行机器码
	StackSize int     // 栈帧大小
	NumArgs   int     // 参数数量
}

// EntryPoint 获取函数入口点
func (cf *CompiledFunc) EntryPoint() uintptr {
	if len(cf.Code) == 0 {
		return 0
	}
	return getCodePointer(cf.Code)
}

// ============================================================================
// 代码生成器接口
// ============================================================================

// CodeGenerator 代码生成器接口
// 不同的 CPU 架构需要实现不同的代码生成器
type CodeGenerator interface {
	// Generate 生成机器码
	Generate(fn *IRFunc, alloc *RegAllocation) ([]byte, error)
	
	// NumRegisters 返回可用的通用寄存器数量
	NumRegisters() int
	
	// CallingConvention 返回调用约定信息
	CallingConvention() CallingConv
}

// CallingConv 调用约定
type CallingConv struct {
	// ArgRegs 用于传递参数的寄存器
	ArgRegs []int
	// RetReg 返回值寄存器
	RetReg int
	// CallerSaved 调用者保存的寄存器
	CallerSaved []int
	// CalleeSaved 被调用者保存的寄存器
	CalleeSaved []int
}

// ============================================================================
// 辅助函数
// ============================================================================

// dumpHex 以十六进制格式打印字节数组
func dumpHex(code []byte) {
	for i := 0; i < len(code); i += 16 {
		fmt.Printf("%04x: ", i)
		end := i + 16
		if end > len(code) {
			end = len(code)
		}
		for j := i; j < end; j++ {
			fmt.Printf("%02x ", code[j])
		}
		fmt.Println()
	}
}
