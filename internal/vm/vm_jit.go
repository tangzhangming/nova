package vm

import (
	"sync"
	"unsafe"

	"github.com/tangzhangming/nova/internal/bytecode"
)

// ============================================================================
// JIT 接口定义 (避免循环导入)
// ============================================================================

// JITCompilerInterface JIT 编译器接口
type JITCompilerInterface interface {
	Compile(fn *bytecode.Function) (JITCompiledCode, error)
	RegisterHelper(name string, addr uintptr)
}

// JITCompiledCode 编译后的代码接口
type JITCompiledCode interface {
	GetCode() []byte
	GetEntry() uintptr
}

// JITExecutorInterface JIT 执行器接口
type JITExecutorInterface interface {
	Compile(fn *bytecode.Function) (JITInstalledCode, error)
	Execute(code JITInstalledCode, args []bytecode.Value) (bytecode.Value, error)
}

// JITInstalledCode 已安装代码接口
type JITInstalledCode interface {
	GetFunction() *bytecode.Function
	GetEntry() uintptr
	HasCode() bool
}

// ============================================================================
// JIT 集成到 VM
// ============================================================================

// JITConfig JIT 配置
type JITConfig struct {
	// 是否启用 JIT
	Enabled bool

	// 热点阈值：函数执行多少次后触发 JIT 编译
	HotThreshold int

	// 是否异步编译
	AsyncCompile bool

	// 优化级别 (0-3)
	OptLevel int
}

// DefaultJITConfig 默认 JIT 配置
func DefaultJITConfig() *JITConfig {
	return &JITConfig{
		Enabled:      true,
		HotThreshold: 100,
		AsyncCompile: true,
		OptLevel:     2,
	}
}

// JITState VM 的 JIT 状态
type JITState struct {
	// 配置
	config *JITConfig

	// JIT 编译器 (通过接口)
	compiler JITCompilerInterface

	// JIT 执行器 (通过接口)
	executor JITExecutorInterface

	// 函数执行计数
	execCounts sync.Map // *bytecode.Function -> int

	// 已编译函数缓存
	compiledFuncs sync.Map // *bytecode.Function -> JITInstalledCode

	// 正在编译的函数
	compiling sync.Map // *bytecode.Function -> bool

	// 统计
	stats JITStats
	mu    sync.Mutex
}

// JITStats JIT 统计
type JITStats struct {
	FunctionsCompiled   int64
	CompilationTimeNs   int64
	ExecutionsJIT       int64
	ExecutionsInterp    int64
	HotFunctionsFound   int64
	CompilationFailures int64
}

// 全局 JIT 状态 (单例)
var globalJITState *JITState
var jitOnce sync.Once

// GetJITState 获取全局 JIT 状态
func GetJITState() *JITState {
	jitOnce.Do(func() {
		globalJITState = NewJITState(DefaultJITConfig())
	})
	return globalJITState
}

// NewJITState 创建 JIT 状态
func NewJITState(config *JITConfig) *JITState {
	if config == nil {
		config = DefaultJITConfig()
	}

	state := &JITState{
		config: config,
	}

	return state
}

// SetCompiler 设置 JIT 编译器
func (js *JITState) SetCompiler(compiler JITCompilerInterface) {
	js.compiler = compiler
}

// SetExecutor 设置 JIT 执行器
func (js *JITState) SetExecutor(executor JITExecutorInterface) {
	js.executor = executor
}

// ============================================================================
// 热点检测
// ============================================================================

// RecordExecution 记录函数执行
// 返回 true 如果函数变为热点
func (js *JITState) RecordExecution(fn *bytecode.Function) bool {
	if fn == nil || !js.config.Enabled {
		return false
	}

	// 增加计数
	countI, _ := js.execCounts.LoadOrStore(fn, 0)
	count := countI.(int) + 1
	js.execCounts.Store(fn, count)

	// 检查是否达到热点阈值
	if count == js.config.HotThreshold {
		js.mu.Lock()
		js.stats.HotFunctionsFound++
		js.mu.Unlock()
		return true
	}

	return false
}

// IsHot 检查函数是否是热点
func (js *JITState) IsHot(fn *bytecode.Function) bool {
	if fn == nil {
		return false
	}
	countI, ok := js.execCounts.Load(fn)
	if !ok {
		return false
	}
	return countI.(int) >= js.config.HotThreshold
}

// ============================================================================
// JIT 编译
// ============================================================================

// TryCompile 尝试编译函数
func (js *JITState) TryCompile(fn *bytecode.Function) JITInstalledCode {
	if fn == nil || !js.config.Enabled || js.executor == nil {
		return nil
	}

	// 检查是否已编译
	if cached, ok := js.compiledFuncs.Load(fn); ok {
		return cached.(JITInstalledCode)
	}

	// 检查是否正在编译
	if _, compiling := js.compiling.LoadOrStore(fn, true); compiling {
		return nil
	}
	defer js.compiling.Delete(fn)

	// 执行编译
	installed, err := js.executor.Compile(fn)
	if err != nil {
		js.mu.Lock()
		js.stats.CompilationFailures++
		js.mu.Unlock()
		return nil
	}

	// 缓存结果
	js.compiledFuncs.Store(fn, installed)

	js.mu.Lock()
	js.stats.FunctionsCompiled++
	js.mu.Unlock()

	return installed
}

// TryCompileAsync 异步编译函数
func (js *JITState) TryCompileAsync(fn *bytecode.Function) {
	if js.config.AsyncCompile {
		go js.TryCompile(fn)
	} else {
		js.TryCompile(fn)
	}
}

// GetCompiled 获取已编译的函数
func (js *JITState) GetCompiled(fn *bytecode.Function) JITInstalledCode {
	if cached, ok := js.compiledFuncs.Load(fn); ok {
		return cached.(JITInstalledCode)
	}
	return nil
}

// ============================================================================
// JIT 执行
// ============================================================================

// ExecuteJIT 执行 JIT 编译的代码
func (js *JITState) ExecuteJIT(installed JITInstalledCode, args []bytecode.Value) (bytecode.Value, bool) {
	if installed == nil || !installed.HasCode() {
		return bytecode.NullValue, false
	}

	// 获取入口点
	entry := installed.GetEntry()
	if entry == 0 {
		return bytecode.NullValue, false
	}

	// 执行 JIT 代码
	result, err := js.callNative(entry, args)
	if err != nil {
		return bytecode.NullValue, false
	}

	js.mu.Lock()
	js.stats.ExecutionsJIT++
	js.mu.Unlock()

	return result, true
}

// callNative 调用原生代码
func (js *JITState) callNative(entry uintptr, args []bytecode.Value) (bytecode.Value, error) {
	// 这是一个简化的实现
	// 实际上需要设置正确的调用约定

	// 对于简单的无参数函数
	if len(args) == 0 {
		// 将入口点转换为函数指针并调用
		fn := *(*func() bytecode.Value)(unsafe.Pointer(&entry))
		return fn(), nil
	}

	// 对于有参数的函数，需要更复杂的处理
	// 这里返回 null，实际需要根据参数数量调用不同的函数类型
	return bytecode.NullValue, nil
}

// ============================================================================
// VM JIT 方法
// ============================================================================

// EnableJIT 启用 JIT
func (vm *VM) EnableJIT() {
	GetJITState().config.Enabled = true
}

// DisableJIT 禁用 JIT
func (vm *VM) DisableJIT() {
	GetJITState().config.Enabled = false
}

// IsJITEnabled JIT 是否启用
func (vm *VM) IsJITEnabled() bool {
	return GetJITState().config.Enabled
}

// SetJITThreshold 设置 JIT 热点阈值
func (vm *VM) SetJITThreshold(threshold int) {
	GetJITState().config.HotThreshold = threshold
}

// GetJITStats 获取 JIT 统计
func (vm *VM) GetJITStats() JITStats {
	js := GetJITState()
	js.mu.Lock()
	defer js.mu.Unlock()
	return js.stats
}

// TryJITExecution 尝试 JIT 执行
// 返回 (结果, 是否使用了JIT)
func (vm *VM) TryJITExecution(fn *bytecode.Function, args []bytecode.Value) (bytecode.Value, bool) {
	js := GetJITState()
	if !js.config.Enabled {
		return bytecode.NullValue, false
	}

	// 检查是否已编译
	installed := js.GetCompiled(fn)
	if installed != nil {
		return js.ExecuteJIT(installed, args)
	}

	// 记录执行，检查是否变为热点
	if js.RecordExecution(fn) {
		// 触发编译
		js.TryCompileAsync(fn)
	}

	return bytecode.NullValue, false
}

// ============================================================================
// 混合执行器
// ============================================================================

// HybridRunner 混合执行器（JIT + 解释器）
type HybridRunner struct {
	vm       *VM
	jitState *JITState
}

// NewHybridRunner 创建混合执行器
func NewHybridRunner(vm *VM) *HybridRunner {
	return &HybridRunner{
		vm:       vm,
		jitState: GetJITState(),
	}
}

// Run 运行函数
func (hr *HybridRunner) Run(fn *bytecode.Function, args []bytecode.Value) bytecode.Value {
	// 尝试 JIT 执行
	if result, ok := hr.vm.TryJITExecution(fn, args); ok {
		return result
	}

	// 回退到解释器
	hr.jitState.mu.Lock()
	hr.jitState.stats.ExecutionsInterp++
	hr.jitState.mu.Unlock()

	// 使用解释器执行
	return hr.runInterpreted(fn, args)
}

// runInterpreted 解释执行
func (hr *HybridRunner) runInterpreted(fn *bytecode.Function, args []bytecode.Value) bytecode.Value {
	// 保存当前状态
	savedSP := hr.vm.sp
	savedFP := hr.vm.fp

	// 压入参数
	for _, arg := range args {
		hr.vm.push(arg)
	}

	// 压入调用帧
	hr.vm.pushFrame(fn, savedSP)

	// 执行
	result := hr.vm.runLoop()

	// 恢复状态（如果需要）
	if hr.vm.hasError {
		hr.vm.sp = savedSP
		hr.vm.fp = savedFP
	}

	return result
}

// ============================================================================
// 性能测试辅助
// ============================================================================

// BenchmarkJIT 基准测试 JIT
func BenchmarkJIT(fn *bytecode.Function, iterations int) (jitTime, interpTime int64) {
	// 这个函数用于性能对比测试
	// 具体实现在 benchmark 包中
	return 0, 0
}

// WarmUpJIT 预热 JIT (强制编译)
func WarmUpJIT(fn *bytecode.Function) {
	js := GetJITState()
	// 直接触发编译，不等待热点
	js.TryCompile(fn)
}

// ResetJIT 重置 JIT 状态
func ResetJIT() {
	js := GetJITState()
	js.execCounts = sync.Map{}
	js.compiledFuncs = sync.Map{}
	js.compiling = sync.Map{}
	js.mu.Lock()
	js.stats = JITStats{}
	js.mu.Unlock()
}
