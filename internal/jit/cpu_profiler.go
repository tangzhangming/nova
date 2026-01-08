// cpu_profiler.go - CPU 性能分析器
//
// 本文件实现基于采样的 CPU 性能分析。
// 通过定期采样当前执行的函数来统计 CPU 时间分布。
//
// 主要功能：
// 1. 定时采样当前执行栈
// 2. 聚合统计函数执行时间
// 3. 生成火焰图数据
// 4. 支持导出 pprof 格式

package jit

import (
	"fmt"
	"io"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// ============================================================================
// CPU 分析器
// ============================================================================

// CPUProfiler CPU 性能分析器
type CPUProfiler struct {
	mu sync.Mutex
	
	// 状态
	enabled    bool
	running    int32 // atomic
	sampleRate int   // 采样频率 (Hz)
	
	// 采样数据
	samples       []*CPUSample
	functionStats map[string]*FunctionStats
	
	// 控制
	stopChan chan struct{}
	doneChan chan struct{}
	
	// 回调
	stackSampler StackSampler
}

// CPUSample CPU 采样点
type CPUSample struct {
	Timestamp  time.Time
	Function   string
	StackTrace []string
	Duration   time.Duration // 采样间隔
}

// FunctionStats 函数统计信息
type FunctionStats struct {
	FunctionName string
	TotalSamples int64         // 总采样数
	SelfSamples  int64         // 自身采样数（不包括子调用）
	TotalTime    time.Duration // 总时间
	SelfTime     time.Duration // 自身时间
	CallCount    int64         // 调用次数（如果可统计）
}

// StackSampler 栈采样接口
type StackSampler interface {
	// SampleStack 采样当前执行栈
	// 返回从栈顶到栈底的函数名列表
	SampleStack() []string
}

// CPUProfile CPU 分析结果
type CPUProfile struct {
	StartTime     time.Time
	EndTime       time.Time
	Duration      time.Duration
	SampleCount   int
	SampleRate    int
	FunctionStats []*FunctionStats
	Samples       []*CPUSample
}

// NewCPUProfiler 创建 CPU 分析器
func NewCPUProfiler() *CPUProfiler {
	return &CPUProfiler{
		sampleRate:    100, // 默认 100Hz
		functionStats: make(map[string]*FunctionStats),
	}
}

// SetSampleRate 设置采样率（Hz）
func (p *CPUProfiler) SetSampleRate(rate int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	if rate > 0 && rate <= 1000 {
		p.sampleRate = rate
	}
}

// SetStackSampler 设置栈采样器
func (p *CPUProfiler) SetStackSampler(sampler StackSampler) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stackSampler = sampler
}

// IsRunning 检查是否正在运行
func (p *CPUProfiler) IsRunning() bool {
	return atomic.LoadInt32(&p.running) == 1
}

// Start 开始 CPU 分析
func (p *CPUProfiler) Start() error {
	if !atomic.CompareAndSwapInt32(&p.running, 0, 1) {
		return fmt.Errorf("profiler already running")
	}
	
	p.mu.Lock()
	p.enabled = true
	p.samples = make([]*CPUSample, 0, 10000)
	p.functionStats = make(map[string]*FunctionStats)
	p.stopChan = make(chan struct{})
	p.doneChan = make(chan struct{})
	sampleRate := p.sampleRate
	p.mu.Unlock()
	
	// 启动采样 goroutine
	go p.sampleLoop(sampleRate)
	
	return nil
}

// Stop 停止 CPU 分析并返回结果
func (p *CPUProfiler) Stop() (*CPUProfile, error) {
	if !atomic.CompareAndSwapInt32(&p.running, 1, 0) {
		return nil, fmt.Errorf("profiler not running")
	}
	
	// 停止采样
	close(p.stopChan)
	<-p.doneChan
	
	p.mu.Lock()
	defer p.mu.Unlock()
	
	p.enabled = false
	
	// 构建结果
	profile := &CPUProfile{
		SampleCount: len(p.samples),
		SampleRate:  p.sampleRate,
		Samples:     p.samples,
	}
	
	if len(p.samples) > 0 {
		profile.StartTime = p.samples[0].Timestamp
		profile.EndTime = p.samples[len(p.samples)-1].Timestamp
		profile.Duration = profile.EndTime.Sub(profile.StartTime)
	}
	
	// 收集函数统计
	profile.FunctionStats = make([]*FunctionStats, 0, len(p.functionStats))
	for _, stats := range p.functionStats {
		profile.FunctionStats = append(profile.FunctionStats, stats)
	}
	
	// 按总时间排序
	sort.Slice(profile.FunctionStats, func(i, j int) bool {
		return profile.FunctionStats[i].TotalTime > profile.FunctionStats[j].TotalTime
	})
	
	return profile, nil
}

// sampleLoop 采样循环
func (p *CPUProfiler) sampleLoop(sampleRate int) {
	defer close(p.doneChan)
	
	interval := time.Second / time.Duration(sampleRate)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	
	for {
		select {
		case <-p.stopChan:
			return
		case <-ticker.C:
			p.takeSample(interval)
		}
	}
}

// takeSample 执行一次采样
func (p *CPUProfiler) takeSample(duration time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	if !p.enabled {
		return
	}
	
	// 获取当前栈
	var stack []string
	if p.stackSampler != nil {
		stack = p.stackSampler.SampleStack()
	} else {
		// 使用默认栈（模拟）
		stack = []string{"<unknown>"}
	}
	
	if len(stack) == 0 {
		return
	}
	
	// 记录采样
	sample := &CPUSample{
		Timestamp:  time.Now(),
		Function:   stack[0],
		StackTrace: stack,
		Duration:   duration,
	}
	p.samples = append(p.samples, sample)
	
	// 更新统计
	for i, fn := range stack {
		stats, ok := p.functionStats[fn]
		if !ok {
			stats = &FunctionStats{FunctionName: fn}
			p.functionStats[fn] = stats
		}
		
		stats.TotalSamples++
		stats.TotalTime += duration
		
		// 只有栈顶函数计入自身时间
		if i == 0 {
			stats.SelfSamples++
			stats.SelfTime += duration
		}
	}
}

// RecordCall 记录函数调用（可选，用于更精确的统计）
func (p *CPUProfiler) RecordCall(funcName string) {
	if !p.IsRunning() {
		return
	}
	
	p.mu.Lock()
	defer p.mu.Unlock()
	
	stats, ok := p.functionStats[funcName]
	if !ok {
		stats = &FunctionStats{FunctionName: funcName}
		p.functionStats[funcName] = stats
	}
	stats.CallCount++
}

// ============================================================================
// 导出功能
// ============================================================================

// ExportPprof 导出为 pprof 格式
func (p *CPUProfile) ExportPprof(w io.Writer) error {
	// pprof 格式是 protobuf，这里简化为文本格式
	// 完整实现需要使用 google/pprof 库
	
	fmt.Fprintf(w, "--- CPU Profile ---\n")
	fmt.Fprintf(w, "Duration: %v\n", p.Duration)
	fmt.Fprintf(w, "Samples: %d\n", p.SampleCount)
	fmt.Fprintf(w, "Sample Rate: %d Hz\n\n", p.SampleRate)
	
	fmt.Fprintf(w, "%-40s %10s %10s %12s %12s\n",
		"Function", "Total%", "Self%", "TotalTime", "SelfTime")
	fmt.Fprintf(w, "%s\n", "-------------------------------------------------------------------------------------")
	
	for _, stats := range p.FunctionStats {
		totalPct := 0.0
		selfPct := 0.0
		if p.SampleCount > 0 {
			totalPct = float64(stats.TotalSamples) / float64(p.SampleCount) * 100
			selfPct = float64(stats.SelfSamples) / float64(p.SampleCount) * 100
		}
		
		fmt.Fprintf(w, "%-40s %9.1f%% %9.1f%% %12v %12v\n",
			truncateString(stats.FunctionName, 40),
			totalPct, selfPct,
			stats.TotalTime, stats.SelfTime)
	}
	
	return nil
}

// ExportFlameGraph 导出火焰图数据
func (p *CPUProfile) ExportFlameGraph(w io.Writer) error {
	// 聚合相同栈
	stackCounts := make(map[string]int64)
	
	for _, sample := range p.Samples {
		// 反转栈（火焰图需要从根到叶）
		reversed := make([]string, len(sample.StackTrace))
		for i, fn := range sample.StackTrace {
			reversed[len(sample.StackTrace)-1-i] = fn
		}
		
		key := ""
		for i, fn := range reversed {
			if i > 0 {
				key += ";"
			}
			key += fn
		}
		
		stackCounts[key]++
	}
	
	// 输出折叠栈格式（可被 flamegraph.pl 处理）
	for stack, count := range stackCounts {
		fmt.Fprintf(w, "%s %d\n", stack, count)
	}
	
	return nil
}

// ============================================================================
// 辅助函数
// ============================================================================

// truncateString 截断字符串
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// GetTopFunctions 获取 top N 热点函数
func (p *CPUProfile) GetTopFunctions(n int) []*FunctionStats {
	if n <= 0 || n > len(p.FunctionStats) {
		n = len(p.FunctionStats)
	}
	return p.FunctionStats[:n]
}

// GetFunctionStats 获取特定函数的统计
func (p *CPUProfile) GetFunctionStats(funcName string) *FunctionStats {
	for _, stats := range p.FunctionStats {
		if stats.FunctionName == funcName {
			return stats
		}
	}
	return nil
}
