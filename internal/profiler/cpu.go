// cpu.go - CPU 性能分析
//
// 实现 CPU 性能分析功能：
// 1. 函数调用采样
// 2. 调用时间统计
// 3. 热点函数识别
// 4. 调用图生成

package profiler

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// CPUProfiler CPU 分析器
type CPUProfiler struct {
	mu sync.RWMutex
	
	// 配置
	config ProfileConfig
	
	// 状态
	running bool
	
	// 采样数据
	samples     []CPUSample
	sampleCount int64
	
	// 函数统计
	funcStats map[string]*FunctionStats
	
	// 调用栈
	callStack []CallStackEntry
	
	// 采样 ticker
	ticker   *time.Ticker
	stopChan chan struct{}
}

// CPUSample CPU 采样
type CPUSample struct {
	Timestamp time.Time
	Stack     []string
	Duration  time.Duration
}

// FunctionStats 函数统计
type FunctionStats struct {
	Name       string
	CallCount  int64
	TotalTime  time.Duration
	SelfTime   time.Duration
	MinTime    time.Duration
	MaxTime    time.Duration
	Callers    map[string]int64 // 调用者统计
	Callees    map[string]int64 // 被调用者统计
}

// CallStackEntry 调用栈条目
type CallStackEntry struct {
	FuncName  string
	StartTime time.Time
	File      string
	Line      int
}

// CPUProfile CPU 分析结果
type CPUProfile struct {
	StartTime   time.Time
	EndTime     time.Time
	Duration    time.Duration
	SampleCount int64
	Functions   []*FunctionStats
	TopFuncs    []*FunctionStats // 按时间排序的前 N 个函数
	CallGraph   *CallGraph
}

// CallGraph 调用图
type CallGraph struct {
	Nodes map[string]*CallGraphNode
	Edges []*CallGraphEdge
}

// CallGraphNode 调用图节点
type CallGraphNode struct {
	Name      string
	TotalTime time.Duration
	SelfTime  time.Duration
	Calls     int64
}

// CallGraphEdge 调用图边
type CallGraphEdge struct {
	From  string
	To    string
	Count int64
}

// NewCPUProfiler 创建 CPU 分析器
func NewCPUProfiler(config ProfileConfig) *CPUProfiler {
	return &CPUProfiler{
		config:    config,
		funcStats: make(map[string]*FunctionStats),
		callStack: make([]CallStackEntry, 0),
	}
}

// Start 开始采样
func (p *CPUProfiler) Start() {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	if p.running {
		return
	}
	
	p.running = true
	p.samples = make([]CPUSample, 0)
	p.stopChan = make(chan struct{})
	
	// 启动采样 goroutine
	p.ticker = time.NewTicker(p.config.CPUSampleInterval)
	go p.sampleLoop()
}

// Stop 停止采样
func (p *CPUProfiler) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	if !p.running {
		return
	}
	
	p.running = false
	if p.ticker != nil {
		p.ticker.Stop()
	}
	close(p.stopChan)
}

// sampleLoop 采样循环
func (p *CPUProfiler) sampleLoop() {
	for {
		select {
		case <-p.stopChan:
			return
		case <-p.ticker.C:
			p.takeSample()
		}
	}
}

// takeSample 进行一次采样
func (p *CPUProfiler) takeSample() {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	if len(p.callStack) == 0 {
		return
	}
	
	// 收集当前调用栈
	stack := make([]string, len(p.callStack))
	for i, entry := range p.callStack {
		stack[i] = entry.FuncName
	}
	
	sample := CPUSample{
		Timestamp: time.Now(),
		Stack:     stack,
	}
	
	p.samples = append(p.samples, sample)
	atomic.AddInt64(&p.sampleCount, 1)
}

// RecordCall 记录函数调用
func (p *CPUProfiler) RecordCall(funcName string, file string, line int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	// 压入调用栈
	entry := CallStackEntry{
		FuncName:  funcName,
		StartTime: time.Now(),
		File:      file,
		Line:      line,
	}
	p.callStack = append(p.callStack, entry)
	
	// 更新统计
	stats := p.getOrCreateStats(funcName)
	stats.CallCount++
	
	// 记录调用关系
	if len(p.callStack) > 1 {
		caller := p.callStack[len(p.callStack)-2].FuncName
		if stats.Callers == nil {
			stats.Callers = make(map[string]int64)
		}
		stats.Callers[caller]++
		
		callerStats := p.getOrCreateStats(caller)
		if callerStats.Callees == nil {
			callerStats.Callees = make(map[string]int64)
		}
		callerStats.Callees[funcName]++
	}
}

// RecordReturn 记录函数返回
func (p *CPUProfiler) RecordReturn(funcName string, duration time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	if len(p.callStack) == 0 {
		return
	}
	
	// 弹出调用栈
	entry := p.callStack[len(p.callStack)-1]
	p.callStack = p.callStack[:len(p.callStack)-1]
	
	// 计算实际持续时间
	if duration == 0 {
		duration = time.Since(entry.StartTime)
	}
	
	// 更新统计
	stats := p.getOrCreateStats(funcName)
	stats.TotalTime += duration
	
	// 计算 self time（不包括子函数调用的时间）
	// 这里简化处理，使用采样来估计
	if stats.MinTime == 0 || duration < stats.MinTime {
		stats.MinTime = duration
	}
	if duration > stats.MaxTime {
		stats.MaxTime = duration
	}
}

// getOrCreateStats 获取或创建函数统计
func (p *CPUProfiler) getOrCreateStats(funcName string) *FunctionStats {
	stats, ok := p.funcStats[funcName]
	if !ok {
		stats = &FunctionStats{
			Name:    funcName,
			Callers: make(map[string]int64),
			Callees: make(map[string]int64),
		}
		p.funcStats[funcName] = stats
	}
	return stats
}

// SampleCount 获取采样数量
func (p *CPUProfiler) SampleCount() int64 {
	return atomic.LoadInt64(&p.sampleCount)
}

// GetProfile 获取分析结果
func (p *CPUProfiler) GetProfile() *CPUProfile {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	profile := &CPUProfile{
		SampleCount: p.sampleCount,
		Functions:   make([]*FunctionStats, 0, len(p.funcStats)),
	}
	
	for _, stats := range p.funcStats {
		statsCopy := *stats
		profile.Functions = append(profile.Functions, &statsCopy)
	}
	
	// 按总时间排序
	sort.Slice(profile.Functions, func(i, j int) bool {
		return profile.Functions[i].TotalTime > profile.Functions[j].TotalTime
	})
	
	// 前 10 个热点函数
	topN := 10
	if len(profile.Functions) < topN {
		topN = len(profile.Functions)
	}
	profile.TopFuncs = profile.Functions[:topN]
	
	// 构建调用图
	profile.CallGraph = p.buildCallGraph()
	
	return profile
}

// buildCallGraph 构建调用图
func (p *CPUProfiler) buildCallGraph() *CallGraph {
	graph := &CallGraph{
		Nodes: make(map[string]*CallGraphNode),
		Edges: make([]*CallGraphEdge, 0),
	}
	
	for name, stats := range p.funcStats {
		graph.Nodes[name] = &CallGraphNode{
			Name:      name,
			TotalTime: stats.TotalTime,
			SelfTime:  stats.SelfTime,
			Calls:     stats.CallCount,
		}
		
		for callee, count := range stats.Callees {
			graph.Edges = append(graph.Edges, &CallGraphEdge{
				From:  name,
				To:    callee,
				Count: count,
			})
		}
	}
	
	return graph
}

// WriteProfile 写入分析报告
func (p *CPUProfiler) WriteProfile(w io.Writer, format OutputFormat) error {
	profile := p.GetProfile()
	
	switch format {
	case FormatText:
		return p.writeTextProfile(w, profile)
	case FormatJSON:
		return p.writeJSONProfile(w, profile)
	case FormatFlamegraph:
		return p.writeFlamegraphProfile(w, profile)
	case FormatPprof:
		return p.writePprofProfile(w, profile)
	default:
		return p.writeTextProfile(w, profile)
	}
}

// writeTextProfile 写入文本格式报告
func (p *CPUProfiler) writeTextProfile(w io.Writer, profile *CPUProfile) error {
	fmt.Fprintf(w, "CPU Profile Report\n")
	fmt.Fprintf(w, "==================\n\n")
	fmt.Fprintf(w, "Total Samples: %d\n", profile.SampleCount)
	fmt.Fprintf(w, "Functions: %d\n\n", len(profile.Functions))
	
	fmt.Fprintf(w, "Top Functions by Total Time:\n")
	fmt.Fprintf(w, "%-40s %10s %10s %10s\n", "Function", "Calls", "Total", "Avg")
	fmt.Fprintf(w, strings.Repeat("-", 72) + "\n")
	
	for _, stats := range profile.TopFuncs {
		avg := time.Duration(0)
		if stats.CallCount > 0 {
			avg = stats.TotalTime / time.Duration(stats.CallCount)
		}
		fmt.Fprintf(w, "%-40s %10d %10s %10s\n",
			truncateName(stats.Name, 40),
			stats.CallCount,
			stats.TotalTime,
			avg,
		)
	}
	
	return nil
}

// writeJSONProfile 写入 JSON 格式报告
func (p *CPUProfiler) writeJSONProfile(w io.Writer, profile *CPUProfile) error {
	fmt.Fprintf(w, "{\n")
	fmt.Fprintf(w, "  \"sampleCount\": %d,\n", profile.SampleCount)
	fmt.Fprintf(w, "  \"functions\": [\n")
	
	for i, stats := range profile.Functions {
		fmt.Fprintf(w, "    {\n")
		fmt.Fprintf(w, "      \"name\": \"%s\",\n", stats.Name)
		fmt.Fprintf(w, "      \"calls\": %d,\n", stats.CallCount)
		fmt.Fprintf(w, "      \"totalTime\": %d,\n", stats.TotalTime.Nanoseconds())
		fmt.Fprintf(w, "      \"selfTime\": %d\n", stats.SelfTime.Nanoseconds())
		if i < len(profile.Functions)-1 {
			fmt.Fprintf(w, "    },\n")
		} else {
			fmt.Fprintf(w, "    }\n")
		}
	}
	
	fmt.Fprintf(w, "  ]\n")
	fmt.Fprintf(w, "}\n")
	
	return nil
}

// writeFlamegraphProfile 写入 Flamegraph 格式
func (p *CPUProfiler) writeFlamegraphProfile(w io.Writer, profile *CPUProfile) error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	// Flamegraph 格式：stack;path count
	stackCounts := make(map[string]int)
	
	for _, sample := range p.samples {
		if len(sample.Stack) == 0 {
			continue
		}
		key := strings.Join(sample.Stack, ";")
		stackCounts[key]++
	}
	
	for stack, count := range stackCounts {
		fmt.Fprintf(w, "%s %d\n", stack, count)
	}
	
	return nil
}

// writePprofProfile 写入 pprof 格式
func (p *CPUProfiler) writePprofProfile(w io.Writer, profile *CPUProfile) error {
	// 简化的 pprof 文本格式
	fmt.Fprintf(w, "--- CPU Profile ---\n")
	fmt.Fprintf(w, "Period: %s\n", p.config.CPUSampleInterval)
	fmt.Fprintf(w, "\n")
	
	for _, stats := range profile.Functions {
		fmt.Fprintf(w, "%d %d: %s\n",
			stats.CallCount,
			stats.TotalTime.Nanoseconds(),
			stats.Name,
		)
	}
	
	return nil
}

// Reset 重置分析器
func (p *CPUProfiler) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	p.samples = nil
	p.sampleCount = 0
	p.funcStats = make(map[string]*FunctionStats)
	p.callStack = make([]CallStackEntry, 0)
}

// truncateName 截断函数名
func truncateName(name string, maxLen int) string {
	if len(name) <= maxLen {
		return name
	}
	return name[:maxLen-3] + "..."
}
