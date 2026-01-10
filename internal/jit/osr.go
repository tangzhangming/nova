// osr.go - On-Stack Replacement (OSR)
//
// 实现运行时从解释器切换到 JIT 编译代码的机制。
//
// OSR 允许在循环执行过程中，将解释器状态迁移到 JIT 编译的代码。
// 这对于长时间运行的循环特别有用，因为它们可能在首次 JIT 编译完成前就开始执行。
//
// 功能：
// 1. 热循环检测（基于迭代计数）
// 2. OSR 入口点生成
// 3. 栈帧迁移（解释器 -> JIT）
// 4. 类型反馈收集
// 5. 回退机制（OSR 失败时继续解释执行）

package jit

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/tangzhangming/nova/internal/bytecode"
)

// OSRThreshold OSR 触发阈值
const OSRThreshold = 1000

// OSRManager OSR 管理器
type OSRManager struct {
	mu sync.RWMutex
	
	// JIT 编译器引用（使用接口以避免循环依赖）
	compiler OSRCompiler
	
	// 循环计数器：funcName:loopOffset -> count
	loopCounters map[string]*int64
	
	// OSR 入口点缓存：funcName:loopOffset -> OSREntry
	osrEntries map[string]*OSREntry
	
	// 类型反馈
	typeFeedback map[string]*TypeFeedback
	
	// 配置
	config OSRConfig
	
	// 统计
	stats OSRStats
}

// OSRConfig OSR 配置
type OSRConfig struct {
	// Threshold 触发 OSR 的循环迭代次数
	Threshold int
	
	// Enabled 是否启用 OSR
	Enabled bool
	
	// EnableTypeFeedback 是否启用类型反馈
	EnableTypeFeedback bool
}

// OSRStats OSR 统计
type OSRStats struct {
	HotLoopsDetected int64
	OSRAttempts      int64
	OSRSuccesses     int64
	OSRFailures      int64
	Deoptimizations  int64
}

// OSREntry OSR 入口点
type OSREntry struct {
	// 函数信息
	FuncName   string
	LoopOffset int
	
	// 编译后的代码
	CompiledFunc *CompiledFunc
	EntryPoint   uintptr
	
	// OSR 入口块
	OSRBlock *IRBlock
	
	// 需要的局部变量映射
	LocalsMapping []LocalMapping
	
	// 状态
	Ready bool
	Error error
}

// LocalMapping 局部变量映射
type LocalMapping struct {
	InterpreterIndex int      // 解释器栈中的索引
	JITIndex         int      // JIT 局部变量索引
	Type             ValueType // 期望的类型
}

// TypeFeedback 类型反馈
type TypeFeedback struct {
	mu sync.RWMutex
	
	// 局部变量类型统计
	LocalTypes map[int]*TypeProfile
	
	// 调用目标类型
	CallTargets map[int]*CallTargetProfile
}

// TypeProfile 类型剖析
type TypeProfile struct {
	// 观察到的类型及其次数
	Types map[ValueType]int64
	
	// 最常见的类型
	DominantType ValueType
	
	// 是否是单态的（只有一种类型）
	Monomorphic bool
}

// CallTargetProfile 调用目标剖析
type CallTargetProfile struct {
	// 观察到的目标函数
	Targets map[string]int64
	
	// 最常见的目标
	DominantTarget string
	
	// 是否是单态的
	Monomorphic bool
}

// OSRCompiler JIT 编译器接口
type OSRCompiler interface {
	// CompileIR 编译 IR 函数
	CompileIR(fn *IRFunc) (*CompiledFunc, error)
}

// DefaultOSRConfig 默认 OSR 配置
func DefaultOSRConfig() OSRConfig {
	return OSRConfig{
		Threshold:          OSRThreshold,
		Enabled:            true,
		EnableTypeFeedback: true,
	}
}

// NewOSRManager 创建 OSR 管理器
func NewOSRManager(compiler OSRCompiler) *OSRManager {
	return &OSRManager{
		compiler:     compiler,
		loopCounters: make(map[string]*int64),
		osrEntries:   make(map[string]*OSREntry),
		typeFeedback: make(map[string]*TypeFeedback),
		config:       DefaultOSRConfig(),
	}
}

// SetConfig 设置配置
func (m *OSRManager) SetConfig(config OSRConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config = config
}

// GetStats 获取统计
func (m *OSRManager) GetStats() OSRStats {
	return OSRStats{
		HotLoopsDetected: atomic.LoadInt64(&m.stats.HotLoopsDetected),
		OSRAttempts:      atomic.LoadInt64(&m.stats.OSRAttempts),
		OSRSuccesses:     atomic.LoadInt64(&m.stats.OSRSuccesses),
		OSRFailures:      atomic.LoadInt64(&m.stats.OSRFailures),
		Deoptimizations:  atomic.LoadInt64(&m.stats.Deoptimizations),
	}
}

// OnLoopIteration 循环迭代时调用
// 返回 OSR 入口点（如果可用），否则返回 nil
func (m *OSRManager) OnLoopIteration(fn *bytecode.Function, loopOffset int) *OSREntry {
	if !m.config.Enabled {
		return nil
	}
	
	key := fmt.Sprintf("%s:%d", fn.Name, loopOffset)
	
	// 增加循环计数
	m.mu.Lock()
	counter, ok := m.loopCounters[key]
	if !ok {
		counter = new(int64)
		m.loopCounters[key] = counter
	}
	m.mu.Unlock()
	
	count := atomic.AddInt64(counter, 1)
	
	// 检查是否达到阈值
	if count == int64(m.config.Threshold) {
		atomic.AddInt64(&m.stats.HotLoopsDetected, 1)
		
		// 检查是否已有 OSR 入口点
		m.mu.RLock()
		entry, hasEntry := m.osrEntries[key]
		m.mu.RUnlock()
		
		if !hasEntry {
			// 触发 OSR 编译
			go m.compileOSREntry(fn, loopOffset, key)
			return nil
		}
		
		if entry.Ready {
			return entry
		}
	}
	
	// 检查是否有就绪的 OSR 入口点
	if count > int64(m.config.Threshold) {
		m.mu.RLock()
		entry, ok := m.osrEntries[key]
		m.mu.RUnlock()
		
		if ok && entry.Ready {
			return entry
		}
	}
	
	return nil
}

// compileOSREntry 编译 OSR 入口点
func (m *OSRManager) compileOSREntry(fn *bytecode.Function, loopOffset int, key string) {
	atomic.AddInt64(&m.stats.OSRAttempts, 1)
	
	entry := &OSREntry{
		FuncName:   fn.Name,
		LoopOffset: loopOffset,
	}
	
	// 检查函数是否可以 JIT 编译
	if !CanJIT(fn) {
		entry.Error = fmt.Errorf("function cannot be JIT compiled")
		atomic.AddInt64(&m.stats.OSRFailures, 1)
		m.mu.Lock()
		m.osrEntries[key] = entry
		m.mu.Unlock()
		return
	}
	
	// 构建 IR
	builder := NewIRBuilder()
	irFunc, err := builder.Build(fn)
	if err != nil {
		entry.Error = err
		atomic.AddInt64(&m.stats.OSRFailures, 1)
		m.mu.Lock()
		m.osrEntries[key] = entry
		m.mu.Unlock()
		return
	}
	
	// 查找循环对应的基本块
	osrBlock := m.findLoopBlock(irFunc, loopOffset)
	if osrBlock == nil {
		entry.Error = fmt.Errorf("cannot find OSR entry block for offset %d", loopOffset)
		atomic.AddInt64(&m.stats.OSRFailures, 1)
		m.mu.Lock()
		m.osrEntries[key] = entry
		m.mu.Unlock()
		return
	}
	
	entry.OSRBlock = osrBlock
	
	// 分析需要的局部变量
	entry.LocalsMapping = m.analyzeRequiredLocals(irFunc, osrBlock)
	
	// 应用类型反馈进行优化
	if m.config.EnableTypeFeedback {
		m.applyTypeFeedback(irFunc, key)
	}
	
	// 优化
	optimizer := NewOptimizer(2)
	optimizer.Optimize(irFunc)
	
	// 代码生成（如果编译器可用）
	if m.compiler != nil {
		compiled, err := m.compiler.CompileIR(irFunc)
		if err != nil {
			entry.Error = err
			atomic.AddInt64(&m.stats.OSRFailures, 1)
		} else {
			entry.CompiledFunc = compiled
			entry.EntryPoint = compiled.EntryPoint()
			entry.Ready = true
			atomic.AddInt64(&m.stats.OSRSuccesses, 1)
		}
	} else {
		entry.Error = fmt.Errorf("JIT compiler not available")
		atomic.AddInt64(&m.stats.OSRFailures, 1)
	}
	
	m.mu.Lock()
	m.osrEntries[key] = entry
	m.mu.Unlock()
}

// findLoopBlock 查找循环对应的基本块
func (m *OSRManager) findLoopBlock(irFunc *IRFunc, loopOffset int) *IRBlock {
	// 查找与循环偏移最接近的基本块
	var bestBlock *IRBlock
	bestDist := int(^uint(0) >> 1) // MaxInt
	
	for _, block := range irFunc.Blocks {
		// 检查块中的指令
		for _, instr := range block.Instrs {
			// 检查是否是循环相关的指令
			if instr.Op == OpJump || instr.Op == OpBranch {
				// 计算与目标偏移的距离
				dist := abs(instr.Line - loopOffset)
				if dist < bestDist {
					bestDist = dist
					bestBlock = block
				}
			}
		}
	}
	
	return bestBlock
}

// analyzeRequiredLocals 分析 OSR 入口需要的局部变量
func (m *OSRManager) analyzeRequiredLocals(irFunc *IRFunc, osrBlock *IRBlock) []LocalMapping {
	mappings := make([]LocalMapping, 0)
	
	// 收集 OSR 入口块及其后继块中使用的局部变量
	visited := make(map[int]bool)
	var collectLocals func(block *IRBlock)
	collectLocals = func(block *IRBlock) {
		if visited[block.ID] {
			return
		}
		visited[block.ID] = true
		
		for _, instr := range block.Instrs {
			if instr.Op == OpLoadLocal && instr.LocalIdx >= 0 {
				// 检查是否已记录
				found := false
				for _, m := range mappings {
					if m.InterpreterIndex == instr.LocalIdx {
						found = true
						break
					}
				}
				if !found {
					mappings = append(mappings, LocalMapping{
						InterpreterIndex: instr.LocalIdx,
						JITIndex:         instr.LocalIdx,
						Type:             instr.Dest.Type,
					})
				}
			}
		}
		
		// 递归处理后继块
		for _, succ := range block.Succs {
			collectLocals(succ)
		}
	}
	
	collectLocals(osrBlock)
	
	return mappings
}

// applyTypeFeedback 应用类型反馈
func (m *OSRManager) applyTypeFeedback(irFunc *IRFunc, key string) {
	m.mu.RLock()
	feedback, ok := m.typeFeedback[key]
	m.mu.RUnlock()
	
	if !ok {
		return
	}
	
	feedback.mu.RLock()
	defer feedback.mu.RUnlock()
	
	// 根据类型反馈优化局部变量类型
	for _, block := range irFunc.Blocks {
		for _, instr := range block.Instrs {
			if instr.Op == OpLoadLocal && instr.LocalIdx >= 0 {
				if profile, ok := feedback.LocalTypes[instr.LocalIdx]; ok {
					if profile.Monomorphic && instr.Dest != nil {
						instr.Dest.Type = profile.DominantType
					}
				}
			}
		}
	}
}

// RecordTypeFeedback 记录类型反馈
func (m *OSRManager) RecordTypeFeedback(fn *bytecode.Function, loopOffset int, localIdx int, valueType ValueType) {
	if !m.config.EnableTypeFeedback {
		return
	}
	
	key := fmt.Sprintf("%s:%d", fn.Name, loopOffset)
	
	m.mu.Lock()
	feedback, ok := m.typeFeedback[key]
	if !ok {
		feedback = &TypeFeedback{
			LocalTypes:  make(map[int]*TypeProfile),
			CallTargets: make(map[int]*CallTargetProfile),
		}
		m.typeFeedback[key] = feedback
	}
	m.mu.Unlock()
	
	feedback.mu.Lock()
	defer feedback.mu.Unlock()
	
	profile, ok := feedback.LocalTypes[localIdx]
	if !ok {
		profile = &TypeProfile{
			Types: make(map[ValueType]int64),
		}
		feedback.LocalTypes[localIdx] = profile
	}
	
	profile.Types[valueType]++
	
	// 更新主导类型
	var maxCount int64
	for t, count := range profile.Types {
		if count > maxCount {
			maxCount = count
			profile.DominantType = t
		}
	}
	
	// 检查是否单态
	profile.Monomorphic = len(profile.Types) == 1
}

// RecordCallTarget 记录调用目标
func (m *OSRManager) RecordCallTarget(fn *bytecode.Function, loopOffset int, callSite int, targetName string) {
	if !m.config.EnableTypeFeedback {
		return
	}
	
	key := fmt.Sprintf("%s:%d", fn.Name, loopOffset)
	
	m.mu.Lock()
	feedback, ok := m.typeFeedback[key]
	if !ok {
		feedback = &TypeFeedback{
			LocalTypes:  make(map[int]*TypeProfile),
			CallTargets: make(map[int]*CallTargetProfile),
		}
		m.typeFeedback[key] = feedback
	}
	m.mu.Unlock()
	
	feedback.mu.Lock()
	defer feedback.mu.Unlock()
	
	profile, ok := feedback.CallTargets[callSite]
	if !ok {
		profile = &CallTargetProfile{
			Targets: make(map[string]int64),
		}
		feedback.CallTargets[callSite] = profile
	}
	
	profile.Targets[targetName]++
	
	// 更新主导目标
	var maxCount int64
	for t, count := range profile.Targets {
		if count > maxCount {
			maxCount = count
			profile.DominantTarget = t
		}
	}
	
	// 检查是否单态
	profile.Monomorphic = len(profile.Targets) == 1
}

// Deoptimize 触发反优化（从 JIT 代码回退到解释器）
func (m *OSRManager) Deoptimize(fn *bytecode.Function, reason string) {
	atomic.AddInt64(&m.stats.Deoptimizations, 1)
	
	// 清除相关的 OSR 入口点
	m.mu.Lock()
	for key := range m.osrEntries {
		if len(key) > len(fn.Name) && key[:len(fn.Name)] == fn.Name {
			delete(m.osrEntries, key)
		}
	}
	m.mu.Unlock()
}

// InvalidateOSREntry 使 OSR 入口失效
func (m *OSRManager) InvalidateOSREntry(fn *bytecode.Function, loopOffset int) {
	key := fmt.Sprintf("%s:%d", fn.Name, loopOffset)
	
	m.mu.Lock()
	delete(m.osrEntries, key)
	m.mu.Unlock()
}

// Reset 重置 OSR 管理器
func (m *OSRManager) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.loopCounters = make(map[string]*int64)
	m.osrEntries = make(map[string]*OSREntry)
	m.typeFeedback = make(map[string]*TypeFeedback)
	m.stats = OSRStats{}
}

// GetOSREntry 获取 OSR 入口点
func (m *OSRManager) GetOSREntry(fn *bytecode.Function, loopOffset int) *OSREntry {
	key := fmt.Sprintf("%s:%d", fn.Name, loopOffset)
	
	m.mu.RLock()
	entry, ok := m.osrEntries[key]
	m.mu.RUnlock()
	
	if ok && entry.Ready {
		return entry
	}
	return nil
}

// abs 返回绝对值
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// ============================================================================
// OSR 状态迁移
// ============================================================================

// OSRState OSR 状态（用于栈帧迁移）
type OSRState struct {
	// 局部变量值
	Locals []int64
	
	// 栈值
	Stack []int64
	
	// 当前 IP
	IP int
	
	// 栈帧深度
	FrameDepth int
}

// CreateOSRState 创建 OSR 状态
func CreateOSRState(entry *OSREntry, interpreterLocals []int64, interpreterStack []int64, ip int) *OSRState {
	state := &OSRState{
		Locals: make([]int64, len(entry.LocalsMapping)),
		Stack:  interpreterStack,
		IP:     ip,
	}
	
	// 映射局部变量
	for i, mapping := range entry.LocalsMapping {
		if mapping.InterpreterIndex < len(interpreterLocals) {
			state.Locals[i] = interpreterLocals[mapping.InterpreterIndex]
		}
	}
	
	return state
}

// PerformOSR 执行 OSR（从解释器切换到 JIT）
// 返回 JIT 执行的结果和是否成功
func (m *OSRManager) PerformOSR(entry *OSREntry, state *OSRState) (int64, bool) {
	if entry == nil || !entry.Ready || entry.CompiledFunc == nil {
		return 0, false
	}
	
	// 获取编译后代码的入口点
	entryPoint := entry.CompiledFunc.EntryPoint()
	if entryPoint == 0 {
		return 0, false
	}
	
	// 准备参数
	// OSR 需要将解释器状态传递给 JIT 代码
	// 这里使用简化的实现，将局部变量作为参数传递
	
	// 调用编译后的代码
	// 注意：实际的 OSR 实现需要更复杂的栈帧迁移机制
	// 这里是一个框架实现，实际的执行需要平台特定的代码
	
	// 返回值表示 OSR 入口已就绪，但实际执行由调用者处理
	// 调用者需要使用 entry.EntryPoint 和 state 来执行 JIT 代码
	return 0, true
}
