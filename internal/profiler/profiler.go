// profiler.go - Sola 性能分析器
//
// 提供 CPU 和内存性能分析功能。
//
// 功能：
// 1. CPU Profiling - 采样函数调用栈
// 2. 内存 Profiling - 追踪对象分配
// 3. 调用图生成（支持 flamegraph）
// 4. pprof 兼容格式导出

package profiler

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// ProfileType 分析类型
type ProfileType int

const (
	// ProfileCPU CPU 分析
	ProfileCPU ProfileType = iota
	// ProfileMemory 内存分析
	ProfileMemory
	// ProfileBlock 阻塞分析
	ProfileBlock
	// ProfileGoroutine Goroutine 分析
	ProfileGoroutine
)

// Profiler 性能分析器
type Profiler struct {
	mu sync.RWMutex
	
	// 状态
	enabled bool
	running bool
	
	// CPU 分析
	cpuProfiler *CPUProfiler
	
	// 内存分析
	memProfiler *MemoryProfiler
	
	// 配置
	config ProfileConfig
	
	// 统计
	stats ProfileStats
}

// ProfileConfig 分析器配置
type ProfileConfig struct {
	// CPU 采样间隔（毫秒）
	CPUSampleInterval time.Duration
	
	// 内存采样率（每 N 次分配采样一次）
	MemorySampleRate int
	
	// 最大栈深度
	MaxStackDepth int
	
	// 输出格式
	OutputFormat OutputFormat
}

// OutputFormat 输出格式
type OutputFormat int

const (
	// FormatPprof pprof 格式
	FormatPprof OutputFormat = iota
	// FormatJSON JSON 格式
	FormatJSON
	// FormatText 文本格式
	FormatText
	// FormatFlamegraph Flamegraph 格式
	FormatFlamegraph
)

// ProfileStats 分析统计
type ProfileStats struct {
	StartTime     time.Time
	EndTime       time.Time
	Duration      time.Duration
	CPUSamples    int64
	MemAllocations int64
	MemBytes      int64
}

// DefaultConfig 默认配置
func DefaultConfig() ProfileConfig {
	return ProfileConfig{
		CPUSampleInterval: 10 * time.Millisecond,
		MemorySampleRate:  512 * 1024, // 每 512KB 采样
		MaxStackDepth:     64,
		OutputFormat:      FormatPprof,
	}
}

// NewProfiler 创建分析器
func NewProfiler() *Profiler {
	config := DefaultConfig()
	return &Profiler{
		cpuProfiler: NewCPUProfiler(config),
		memProfiler: NewMemoryProfiler(config),
		config:      config,
	}
}

// NewProfilerWithConfig 创建带配置的分析器
func NewProfilerWithConfig(config ProfileConfig) *Profiler {
	return &Profiler{
		cpuProfiler: NewCPUProfiler(config),
		memProfiler: NewMemoryProfiler(config),
		config:      config,
	}
}

// Enable 启用分析器
func (p *Profiler) Enable() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.enabled = true
}

// Disable 禁用分析器
func (p *Profiler) Disable() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.enabled = false
}

// Start 开始分析
func (p *Profiler) Start(types ...ProfileType) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	if p.running {
		return fmt.Errorf("profiler already running")
	}
	
	p.running = true
	p.stats.StartTime = time.Now()
	
	// 默认启动所有类型
	if len(types) == 0 {
		types = []ProfileType{ProfileCPU, ProfileMemory}
	}
	
	for _, t := range types {
		switch t {
		case ProfileCPU:
			p.cpuProfiler.Start()
		case ProfileMemory:
			p.memProfiler.Start()
		}
	}
	
	return nil
}

// Stop 停止分析
func (p *Profiler) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	if !p.running {
		return fmt.Errorf("profiler not running")
	}
	
	p.cpuProfiler.Stop()
	p.memProfiler.Stop()
	
	p.running = false
	p.stats.EndTime = time.Now()
	p.stats.Duration = p.stats.EndTime.Sub(p.stats.StartTime)
	p.stats.CPUSamples = p.cpuProfiler.SampleCount()
	p.stats.MemAllocations = p.memProfiler.AllocationCount()
	p.stats.MemBytes = p.memProfiler.TotalBytes()
	
	return nil
}

// IsRunning 检查是否正在运行
func (p *Profiler) IsRunning() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.running
}

// RecordFunctionCall 记录函数调用（供 VM 调用）
func (p *Profiler) RecordFunctionCall(funcName string, file string, line int) {
	if !p.enabled || !p.running {
		return
	}
	p.cpuProfiler.RecordCall(funcName, file, line)
}

// RecordFunctionReturn 记录函数返回（供 VM 调用）
func (p *Profiler) RecordFunctionReturn(funcName string, duration time.Duration) {
	if !p.enabled || !p.running {
		return
	}
	p.cpuProfiler.RecordReturn(funcName, duration)
}

// RecordAllocation 记录内存分配（供 VM 调用）
func (p *Profiler) RecordAllocation(typeName string, size int64) {
	if !p.enabled || !p.running {
		return
	}
	p.memProfiler.RecordAllocation(typeName, size)
}

// RecordDeallocation 记录内存释放（供 VM 调用）
func (p *Profiler) RecordDeallocation(typeName string, size int64) {
	if !p.enabled || !p.running {
		return
	}
	p.memProfiler.RecordDeallocation(typeName, size)
}

// WriteCPUProfile 写入 CPU 分析报告
func (p *Profiler) WriteCPUProfile(w io.Writer) error {
	return p.cpuProfiler.WriteProfile(w, p.config.OutputFormat)
}

// WriteMemoryProfile 写入内存分析报告
func (p *Profiler) WriteMemoryProfile(w io.Writer) error {
	return p.memProfiler.WriteProfile(w, p.config.OutputFormat)
}

// SaveCPUProfile 保存 CPU 分析到文件
func (p *Profiler) SaveCPUProfile(filename string) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()
	return p.WriteCPUProfile(f)
}

// SaveMemoryProfile 保存内存分析到文件
func (p *Profiler) SaveMemoryProfile(filename string) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()
	return p.WriteMemoryProfile(f)
}

// GetStats 获取统计信息
func (p *Profiler) GetStats() ProfileStats {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.stats
}

// GetCPUProfile 获取 CPU 分析数据
func (p *Profiler) GetCPUProfile() *CPUProfile {
	return p.cpuProfiler.GetProfile()
}

// GetMemoryProfile 获取内存分析数据
func (p *Profiler) GetMemoryProfile() *MemoryProfile {
	return p.memProfiler.GetProfile()
}

// Reset 重置分析器
func (p *Profiler) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	if p.running {
		p.cpuProfiler.Stop()
		p.memProfiler.Stop()
		p.running = false
	}
	
	p.cpuProfiler.Reset()
	p.memProfiler.Reset()
	p.stats = ProfileStats{}
}

// ============================================================================
// 全局分析器实例
// ============================================================================

var globalProfiler *Profiler
var profilerOnce sync.Once

// GetGlobalProfiler 获取全局分析器
func GetGlobalProfiler() *Profiler {
	profilerOnce.Do(func() {
		globalProfiler = NewProfiler()
	})
	return globalProfiler
}

// StartCPUProfile 启动 CPU 分析（便捷函数）
func StartCPUProfile() error {
	return GetGlobalProfiler().Start(ProfileCPU)
}

// StopCPUProfile 停止 CPU 分析（便捷函数）
func StopCPUProfile() error {
	return GetGlobalProfiler().Stop()
}

// StartMemoryProfile 启动内存分析（便捷函数）
func StartMemoryProfile() error {
	return GetGlobalProfiler().Start(ProfileMemory)
}

// StopMemoryProfile 停止内存分析（便捷函数）
func StopMemoryProfile() error {
	return GetGlobalProfiler().Stop()
}

// ============================================================================
// 辅助函数
// ============================================================================

// captureStack 捕获当前调用栈
func captureStack(maxDepth int) []uintptr {
	pcs := make([]uintptr, maxDepth)
	n := runtime.Callers(3, pcs) // 跳过 captureStack, 调用者, runtime.Callers
	return pcs[:n]
}

// formatStack 格式化调用栈
func formatStack(pcs []uintptr) []StackFrame {
	frames := make([]StackFrame, 0, len(pcs))
	runtimeFrames := runtime.CallersFrames(pcs)
	
	for {
		frame, more := runtimeFrames.Next()
		frames = append(frames, StackFrame{
			Function: frame.Function,
			File:     frame.File,
			Line:     frame.Line,
		})
		if !more {
			break
		}
	}
	
	return frames
}

// StackFrame 栈帧
type StackFrame struct {
	Function string
	File     string
	Line     int
}

func (f StackFrame) String() string {
	return fmt.Sprintf("%s (%s:%d)", f.Function, f.File, f.Line)
}

// ============================================================================
// 简单的采样器
// ============================================================================

// Sampler 采样器
type Sampler struct {
	rate    int64
	counter int64
}

// NewSampler 创建采样器
func NewSampler(rate int64) *Sampler {
	return &Sampler{rate: rate}
}

// ShouldSample 是否应该采样
func (s *Sampler) ShouldSample() bool {
	count := atomic.AddInt64(&s.counter, 1)
	return count%s.rate == 0
}

// Reset 重置计数器
func (s *Sampler) Reset() {
	atomic.StoreInt64(&s.counter, 0)
}

// ============================================================================
// 报告排序辅助
// ============================================================================

// sortByDuration 按持续时间排序
func sortByDuration(items []ProfileItem) {
	sort.Slice(items, func(i, j int) bool {
		return items[i].Duration > items[j].Duration
	})
}

// sortByCount 按计数排序
func sortByCount(items []ProfileItem) {
	sort.Slice(items, func(i, j int) bool {
		return items[i].Count > items[j].Count
	})
}

// sortByBytes 按字节数排序
func sortByBytes(items []ProfileItem) {
	sort.Slice(items, func(i, j int) bool {
		return items[i].Bytes > items[j].Bytes
	})
}

// ProfileItem 分析条目（用于排序）
type ProfileItem struct {
	Name     string
	Count    int64
	Duration time.Duration
	Bytes    int64
}
