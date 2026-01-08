// profiler.go - 热点检测和性能分析
//
// 本文件实现了运行时的热点检测功能，用于决定哪些函数需要 JIT 编译。
//
// 热点检测策略：
// 1. 函数调用计数：当函数被调用超过阈值次数时，标记为热点
// 2. 循环回边计数：当循环执行超过阈值次数时，可以触发 OSR
// 3. 类型反馈：记录参数类型以支持类型特化（未来功能）
//
// 使用方式：
//   profiler.RecordCall(fn)           // 记录函数调用
//   if profiler.IsHot(fn) { ... }     // 检查是否是热点
//   profiler.RecordLoop(fn, ip)       // 记录循环迭代

package jit

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tangzhangming/nova/internal/bytecode"
)

// ============================================================================
// 热点状态
// ============================================================================

// HotState 热点状态
type HotState int32

const (
	StateCold     HotState = iota // 冷代码
	StateWarm                     // 温代码（接近热点）
	StateHot                      // 热点
	StateCompiled                 // 已编译
)

func (s HotState) String() string {
	switch s {
	case StateCold:
		return "cold"
	case StateWarm:
		return "warm"
	case StateHot:
		return "hot"
	case StateCompiled:
		return "compiled"
	default:
		return "unknown"
	}
}

// ============================================================================
// 函数档案
// ============================================================================

// FunctionProfile 函数性能档案
type FunctionProfile struct {
	Function     *bytecode.Function // 函数对象
	CallCount    int64              // 调用次数
	State        HotState           // 热点状态
	CompileFails int32              // 编译失败次数
	
	// 循环信息
	Loops map[int]*LoopProfile // IP -> 循环档案
	
	// 类型反馈（用于优化）
	ArgTypes    [][]string // 每个参数见过的类型
	ReturnTypes []string   // 返回值类型
}

// LoopProfile 循环档案
type LoopProfile struct {
	HeaderIP       int     // 循环头 IP
	BackedgeIP     int     // 回边 IP
	IterationCount int64   // 迭代次数
	State          HotState
}

// ============================================================================
// 热点检测器
// ============================================================================

// Profiler 热点检测器
type Profiler struct {
	// 阈值配置
	functionThreshold int64 // 函数热点阈值
	loopThreshold     int64 // 循环热点阈值
	
	// 函数档案
	profiles sync.Map // uintptr -> *FunctionProfile
	
	// 回调
	onFunctionHot func(*FunctionProfile)
	onLoopHot     func(*LoopProfile)
	
	// 统计
	totalCalls    int64
	hotFunctions  int64
	hotLoops      int64
	
	// 是否启用
	enabled int32
}

// NewProfiler 创建热点检测器
func NewProfiler(functionThreshold, loopThreshold int64) *Profiler {
	return &Profiler{
		functionThreshold: functionThreshold,
		loopThreshold:     loopThreshold,
		enabled:           1,
	}
}

// SetEnabled 启用/禁用热点检测
func (p *Profiler) SetEnabled(enabled bool) {
	if enabled {
		atomic.StoreInt32(&p.enabled, 1)
	} else {
		atomic.StoreInt32(&p.enabled, 0)
	}
}

// IsEnabled 检查是否启用
func (p *Profiler) IsEnabled() bool {
	return atomic.LoadInt32(&p.enabled) == 1
}

// OnFunctionHot 设置函数变热回调
func (p *Profiler) OnFunctionHot(callback func(*FunctionProfile)) {
	p.onFunctionHot = callback
}

// OnLoopHot 设置循环变热回调
func (p *Profiler) OnLoopHot(callback func(*LoopProfile)) {
	p.onLoopHot = callback
}

// ============================================================================
// 函数调用追踪
// ============================================================================

// RecordCall 记录函数调用
func (p *Profiler) RecordCall(fn *bytecode.Function) {
	if !p.IsEnabled() || fn == nil {
		return
	}
	
	atomic.AddInt64(&p.totalCalls, 1)
	
	// 获取或创建档案
	profile := p.getOrCreateProfile(fn)
	
	// 增加调用计数
	count := atomic.AddInt64(&profile.CallCount, 1)
	
	// 检查状态转换
	currentState := HotState(atomic.LoadInt32((*int32)(&profile.State)))
	
	switch currentState {
	case StateCold:
		if count >= p.functionThreshold/10 {
			atomic.CompareAndSwapInt32((*int32)(&profile.State), int32(StateCold), int32(StateWarm))
		}
		
	case StateWarm:
		if count >= p.functionThreshold {
			if atomic.CompareAndSwapInt32((*int32)(&profile.State), int32(StateWarm), int32(StateHot)) {
				atomic.AddInt64(&p.hotFunctions, 1)
				if p.onFunctionHot != nil {
					p.onFunctionHot(profile)
				}
			}
		}
	}
}

// IsHot 检查函数是否是热点
func (p *Profiler) IsHot(fn *bytecode.Function) bool {
	if fn == nil {
		return false
	}
	
	key := fnKey(fn)
	if val, ok := p.profiles.Load(key); ok {
		profile := val.(*FunctionProfile)
		state := HotState(atomic.LoadInt32((*int32)(&profile.State)))
		return state >= StateHot
	}
	return false
}

// IsCompiled 检查函数是否已编译
func (p *Profiler) IsCompiled(fn *bytecode.Function) bool {
	if fn == nil {
		return false
	}
	
	key := fnKey(fn)
	if val, ok := p.profiles.Load(key); ok {
		profile := val.(*FunctionProfile)
		state := HotState(atomic.LoadInt32((*int32)(&profile.State)))
		return state == StateCompiled
	}
	return false
}

// MarkCompiled 标记函数为已编译
func (p *Profiler) MarkCompiled(fn *bytecode.Function) {
	if fn == nil {
		return
	}
	
	key := fnKey(fn)
	if val, ok := p.profiles.Load(key); ok {
		profile := val.(*FunctionProfile)
		atomic.StoreInt32((*int32)(&profile.State), int32(StateCompiled))
	}
}

// MarkCompileFailed 标记函数编译失败
func (p *Profiler) MarkCompileFailed(fn *bytecode.Function) {
	if fn == nil {
		return
	}
	
	key := fnKey(fn)
	if val, ok := p.profiles.Load(key); ok {
		profile := val.(*FunctionProfile)
		atomic.AddInt32(&profile.CompileFails, 1)
	}
}

// ShouldCompile 检查是否应该尝试编译
// 如果编译失败超过3次，不再尝试
func (p *Profiler) ShouldCompile(fn *bytecode.Function) bool {
	if fn == nil {
		return false
	}
	
	key := fnKey(fn)
	if val, ok := p.profiles.Load(key); ok {
		profile := val.(*FunctionProfile)
		state := HotState(atomic.LoadInt32((*int32)(&profile.State)))
		fails := atomic.LoadInt32(&profile.CompileFails)
		// 只有热点函数且编译失败不超过3次才编译
		return state == StateHot && fails < 3
	}
	return false
}

// GetCallCount 获取函数调用次数
func (p *Profiler) GetCallCount(fn *bytecode.Function) int64 {
	if fn == nil {
		return 0
	}
	
	key := fnKey(fn)
	if val, ok := p.profiles.Load(key); ok {
		profile := val.(*FunctionProfile)
		return atomic.LoadInt64(&profile.CallCount)
	}
	return 0
}

// ============================================================================
// 循环追踪
// ============================================================================

// RecordLoop 记录循环迭代
func (p *Profiler) RecordLoop(fn *bytecode.Function, headerIP, backedgeIP int) {
	if !p.IsEnabled() || fn == nil {
		return
	}
	
	profile := p.getOrCreateProfile(fn)
	
	// 获取或创建循环档案
	if profile.Loops == nil {
		profile.Loops = make(map[int]*LoopProfile)
	}
	
	loopProfile, ok := profile.Loops[headerIP]
	if !ok {
		loopProfile = &LoopProfile{
			HeaderIP:   headerIP,
			BackedgeIP: backedgeIP,
		}
		profile.Loops[headerIP] = loopProfile
	}
	
	// 增加迭代计数
	count := atomic.AddInt64(&loopProfile.IterationCount, 1)
	
	// 检查状态转换
	currentState := HotState(atomic.LoadInt32((*int32)(&loopProfile.State)))
	
	switch currentState {
	case StateCold:
		if count >= p.loopThreshold/10 {
			atomic.CompareAndSwapInt32((*int32)(&loopProfile.State), int32(StateCold), int32(StateWarm))
		}
		
	case StateWarm:
		if count >= p.loopThreshold {
			if atomic.CompareAndSwapInt32((*int32)(&loopProfile.State), int32(StateWarm), int32(StateHot)) {
				atomic.AddInt64(&p.hotLoops, 1)
				if p.onLoopHot != nil {
					p.onLoopHot(loopProfile)
				}
			}
		}
	}
}

// ============================================================================
// 类型反馈
// ============================================================================

// RecordArgType 记录参数类型
func (p *Profiler) RecordArgType(fn *bytecode.Function, argIndex int, typeName string) {
	if !p.IsEnabled() || fn == nil {
		return
	}
	
	profile := p.getOrCreateProfile(fn)
	
	// 确保 ArgTypes 足够大
	for len(profile.ArgTypes) <= argIndex {
		profile.ArgTypes = append(profile.ArgTypes, nil)
	}
	
	// 记录类型（去重）
	types := profile.ArgTypes[argIndex]
	for _, t := range types {
		if t == typeName {
			return
		}
	}
	profile.ArgTypes[argIndex] = append(types, typeName)
}

// RecordReturnType 记录返回类型
func (p *Profiler) RecordReturnType(fn *bytecode.Function, typeName string) {
	if !p.IsEnabled() || fn == nil {
		return
	}
	
	profile := p.getOrCreateProfile(fn)
	
	// 记录类型（去重）
	for _, t := range profile.ReturnTypes {
		if t == typeName {
			return
		}
	}
	profile.ReturnTypes = append(profile.ReturnTypes, typeName)
}

// ============================================================================
// 统计信息
// ============================================================================

// ProfilerStats 检测器统计
type ProfilerStats struct {
	TotalCalls   int64
	HotFunctions int64
	HotLoops     int64
}

// GetStats 获取统计信息
func (p *Profiler) GetStats() ProfilerStats {
	return ProfilerStats{
		TotalCalls:   atomic.LoadInt64(&p.totalCalls),
		HotFunctions: atomic.LoadInt64(&p.hotFunctions),
		HotLoops:     atomic.LoadInt64(&p.hotLoops),
	}
}

// ============================================================================
// 辅助方法
// ============================================================================

// getOrCreateProfile 获取或创建函数档案
func (p *Profiler) getOrCreateProfile(fn *bytecode.Function) *FunctionProfile {
	key := fnKey(fn)
	
	// 快速路径：尝试加载
	if val, ok := p.profiles.Load(key); ok {
		return val.(*FunctionProfile)
	}
	
	// 慢路径：创建新档案
	profile := &FunctionProfile{
		Function: fn,
		State:    StateCold,
	}
	
	// 使用 LoadOrStore 避免竞态
	actual, _ := p.profiles.LoadOrStore(key, profile)
	return actual.(*FunctionProfile)
}

// fnKey 获取函数的唯一键
// 使用函数名和类名的组合作为key，避免每次创建新Function对象导致key不同
func fnKey(fn *bytecode.Function) string {
	if fn.ClassName != "" {
		return fn.ClassName + "::" + fn.Name
	}
	return fn.Name
}

// ============================================================================
// 统一性能分析接口
// ============================================================================

// ProfileType 分析类型
type ProfileType int

const (
	ProfileCPU ProfileType = 1 << iota
	ProfileMemory
	ProfileHotspot
	ProfileAll = ProfileCPU | ProfileMemory | ProfileHotspot
)

// UnifiedProfiler 统一性能分析器
// 集成热点检测、CPU分析和内存分析
type UnifiedProfiler struct {
	// 热点检测器
	*Profiler
	
	// CPU 分析器
	cpuProfiler *CPUProfiler
	
	// 内存分析器
	memoryProfiler *MemoryProfiler
	
	// 会话管理
	mu       sync.Mutex
	sessions map[string]*ProfileSession
}

// ProfileSession 分析会话
type ProfileSession struct {
	ID          string
	Types       ProfileType
	StartTime   time.Time
	EndTime     time.Time
	CPUProfile  *CPUProfile
	MemProfile  *MemoryProfile
	HotspotData *ProfilerStats
}

// NewUnifiedProfiler 创建统一分析器
func NewUnifiedProfiler(functionThreshold, loopThreshold int64) *UnifiedProfiler {
	return &UnifiedProfiler{
		Profiler:       NewProfiler(functionThreshold, loopThreshold),
		cpuProfiler:    NewCPUProfiler(),
		memoryProfiler: NewMemoryProfiler(),
		sessions:       make(map[string]*ProfileSession),
	}
}

// StartProfiling 开始分析会话
func (p *UnifiedProfiler) StartProfiling(types ProfileType) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	// 生成会话ID
	sessionID := fmt.Sprintf("session_%d", time.Now().UnixNano())
	
	session := &ProfileSession{
		ID:        sessionID,
		Types:     types,
		StartTime: time.Now(),
	}
	
	// 启动各个分析器
	if types&ProfileCPU != 0 {
		if err := p.cpuProfiler.Start(); err != nil {
			return "", fmt.Errorf("failed to start CPU profiler: %w", err)
		}
	}
	
	if types&ProfileMemory != 0 {
		if err := p.memoryProfiler.Start(); err != nil {
			// 停止已启动的分析器
			if types&ProfileCPU != 0 {
				p.cpuProfiler.Stop()
			}
			return "", fmt.Errorf("failed to start memory profiler: %w", err)
		}
	}
	
	p.sessions[sessionID] = session
	return sessionID, nil
}

// StopProfiling 停止分析会话并获取结果
func (p *UnifiedProfiler) StopProfiling(sessionID string) (*ProfileSession, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	session, ok := p.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	
	session.EndTime = time.Now()
	
	// 停止各个分析器并收集结果
	if session.Types&ProfileCPU != 0 {
		cpuProfile, err := p.cpuProfiler.Stop()
		if err != nil {
			return nil, fmt.Errorf("failed to stop CPU profiler: %w", err)
		}
		session.CPUProfile = cpuProfile
	}
	
	if session.Types&ProfileMemory != 0 {
		memProfile, err := p.memoryProfiler.Stop()
		if err != nil {
			return nil, fmt.Errorf("failed to stop memory profiler: %w", err)
		}
		session.MemProfile = memProfile
	}
	
	if session.Types&ProfileHotspot != 0 {
		stats := p.GetStats()
		session.HotspotData = &stats
	}
	
	// 从活动会话中移除
	delete(p.sessions, sessionID)
	
	return session, nil
}

// GetCPUProfiler 获取CPU分析器
func (p *UnifiedProfiler) GetCPUProfiler() *CPUProfiler {
	return p.cpuProfiler
}

// GetMemoryProfiler 获取内存分析器
func (p *UnifiedProfiler) GetMemoryProfiler() *MemoryProfiler {
	return p.memoryProfiler
}

// SetStackSampler 设置栈采样器
func (p *UnifiedProfiler) SetStackSampler(sampler StackSampler) {
	if p.cpuProfiler != nil {
		p.cpuProfiler.SetStackSampler(sampler)
	}
	if p.memoryProfiler != nil {
		p.memoryProfiler.SetStackSampler(sampler)
	}
}

// RecordAllocation 记录内存分配（委托给内存分析器）
func (p *UnifiedProfiler) RecordAllocation(fn string, typ string, size int64) int64 {
	if p.memoryProfiler != nil {
		return p.memoryProfiler.RecordAllocation(fn, typ, size)
	}
	return 0
}

// RecordDeallocation 记录内存释放
func (p *UnifiedProfiler) RecordDeallocation(id int64, size int64) {
	if p.memoryProfiler != nil {
		p.memoryProfiler.RecordDeallocation(id, size)
	}
}
