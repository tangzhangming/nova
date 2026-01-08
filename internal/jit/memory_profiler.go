// memory_profiler.go - 内存性能分析器
//
// 本文件实现内存分配追踪和分析。
// 记录对象分配的位置、类型和大小，帮助定位内存热点。
//
// 主要功能：
// 1. 记录内存分配和释放
// 2. 追踪分配点（调用栈）
// 3. 按类型和位置聚合统计
// 4. 支持检测内存泄漏

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
// 内存分析器
// ============================================================================

// MemoryProfiler 内存性能分析器
type MemoryProfiler struct {
	mu sync.Mutex
	
	// 状态
	enabled bool
	running int32 // atomic
	
	// 分配数据
	allocations     []*Allocation
	allocationStats map[string]*AllocationStats
	
	// 计数器
	totalAllocated   int64
	totalFreed       int64
	currentLive      int64
	allocationCount  int64
	
	// 回调
	stackSampler StackSampler
}

// Allocation 单次内存分配记录
type Allocation struct {
	ID         int64
	Timestamp  time.Time
	Function   string     // 分配位置
	Type       string     // 对象类型
	Size       int64      // 分配大小
	StackTrace []string   // 分配时的调用栈
	Freed      bool       // 是否已释放
	FreedAt    time.Time  // 释放时间
}

// AllocationStats 分配统计
type AllocationStats struct {
	ObjectType       string
	AllocSite        string        // 分配点
	TotalAllocations int64
	TotalBytes       int64
	LiveObjects      int64
	LiveBytes        int64
	AvgSize          float64
	MaxSize          int64
}

// MemoryProfile 内存分析结果
type MemoryProfile struct {
	StartTime        time.Time
	EndTime          time.Time
	Duration         time.Duration
	TotalAllocated   int64
	TotalFreed       int64
	CurrentLive      int64
	AllocationCount  int64
	AllocationStats  []*AllocationStats
	TopAllocators    []*AllocationStats
}

// NewMemoryProfiler 创建内存分析器
func NewMemoryProfiler() *MemoryProfiler {
	return &MemoryProfiler{
		allocationStats: make(map[string]*AllocationStats),
	}
}

// SetStackSampler 设置栈采样器
func (p *MemoryProfiler) SetStackSampler(sampler StackSampler) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stackSampler = sampler
}

// IsRunning 检查是否正在运行
func (p *MemoryProfiler) IsRunning() bool {
	return atomic.LoadInt32(&p.running) == 1
}

// Start 开始内存分析
func (p *MemoryProfiler) Start() error {
	if !atomic.CompareAndSwapInt32(&p.running, 0, 1) {
		return fmt.Errorf("profiler already running")
	}
	
	p.mu.Lock()
	defer p.mu.Unlock()
	
	p.enabled = true
	p.allocations = make([]*Allocation, 0, 10000)
	p.allocationStats = make(map[string]*AllocationStats)
	p.totalAllocated = 0
	p.totalFreed = 0
	p.currentLive = 0
	p.allocationCount = 0
	
	return nil
}

// Stop 停止内存分析并返回结果
func (p *MemoryProfiler) Stop() (*MemoryProfile, error) {
	if !atomic.CompareAndSwapInt32(&p.running, 1, 0) {
		return nil, fmt.Errorf("profiler not running")
	}
	
	p.mu.Lock()
	defer p.mu.Unlock()
	
	p.enabled = false
	
	// 构建结果
	profile := &MemoryProfile{
		TotalAllocated:  p.totalAllocated,
		TotalFreed:      p.totalFreed,
		CurrentLive:     p.currentLive,
		AllocationCount: p.allocationCount,
	}
	
	if len(p.allocations) > 0 {
		profile.StartTime = p.allocations[0].Timestamp
		profile.EndTime = p.allocations[len(p.allocations)-1].Timestamp
		profile.Duration = profile.EndTime.Sub(profile.StartTime)
	}
	
	// 收集分配统计
	profile.AllocationStats = make([]*AllocationStats, 0, len(p.allocationStats))
	for _, stats := range p.allocationStats {
		profile.AllocationStats = append(profile.AllocationStats, stats)
	}
	
	// 按总字节数排序
	sort.Slice(profile.AllocationStats, func(i, j int) bool {
		return profile.AllocationStats[i].TotalBytes > profile.AllocationStats[j].TotalBytes
	})
	
	// Top 分配者
	if len(profile.AllocationStats) > 10 {
		profile.TopAllocators = profile.AllocationStats[:10]
	} else {
		profile.TopAllocators = profile.AllocationStats
	}
	
	return profile, nil
}

// RecordAllocation 记录内存分配
func (p *MemoryProfiler) RecordAllocation(fn string, typ string, size int64) int64 {
	if !p.IsRunning() {
		return 0
	}
	
	p.mu.Lock()
	defer p.mu.Unlock()
	
	if !p.enabled {
		return 0
	}
	
	// 获取调用栈
	var stack []string
	if p.stackSampler != nil {
		stack = p.stackSampler.SampleStack()
	}
	
	// 创建分配记录
	id := p.allocationCount + 1
	p.allocationCount = id
	
	alloc := &Allocation{
		ID:         id,
		Timestamp:  time.Now(),
		Function:   fn,
		Type:       typ,
		Size:       size,
		StackTrace: stack,
	}
	p.allocations = append(p.allocations, alloc)
	
	// 更新计数
	p.totalAllocated += size
	p.currentLive += size
	
	// 更新统计
	key := fmt.Sprintf("%s@%s", typ, fn)
	stats, ok := p.allocationStats[key]
	if !ok {
		stats = &AllocationStats{
			ObjectType: typ,
			AllocSite:  fn,
		}
		p.allocationStats[key] = stats
	}
	
	stats.TotalAllocations++
	stats.TotalBytes += size
	stats.LiveObjects++
	stats.LiveBytes += size
	
	if size > stats.MaxSize {
		stats.MaxSize = size
	}
	stats.AvgSize = float64(stats.TotalBytes) / float64(stats.TotalAllocations)
	
	return id
}

// RecordDeallocation 记录内存释放
func (p *MemoryProfiler) RecordDeallocation(id int64, size int64) {
	if !p.IsRunning() {
		return
	}
	
	p.mu.Lock()
	defer p.mu.Unlock()
	
	if !p.enabled {
		return
	}
	
	// 更新计数
	p.totalFreed += size
	p.currentLive -= size
	
	// 查找并标记分配记录
	for _, alloc := range p.allocations {
		if alloc.ID == id && !alloc.Freed {
			alloc.Freed = true
			alloc.FreedAt = time.Now()
			
			// 更新统计
			key := fmt.Sprintf("%s@%s", alloc.Type, alloc.Function)
			if stats, ok := p.allocationStats[key]; ok {
				stats.LiveObjects--
				stats.LiveBytes -= alloc.Size
			}
			break
		}
	}
}

// RecordDeallocationByType 按类型记录释放
func (p *MemoryProfiler) RecordDeallocationByType(typ string, size int64) {
	if !p.IsRunning() {
		return
	}
	
	p.mu.Lock()
	defer p.mu.Unlock()
	
	if !p.enabled {
		return
	}
	
	// 更新计数
	p.totalFreed += size
	p.currentLive -= size
	
	// 更新统计（按类型匹配）
	for key, stats := range p.allocationStats {
		if stats.ObjectType == typ && stats.LiveObjects > 0 {
			stats.LiveObjects--
			stats.LiveBytes -= size
			if stats.LiveBytes < 0 {
				stats.LiveBytes = 0
			}
			break
		}
		_ = key
	}
}

// ============================================================================
// 导出功能
// ============================================================================

// ExportReport 导出内存分析报告
func (p *MemoryProfile) ExportReport(w io.Writer) error {
	fmt.Fprintf(w, "=== Memory Profile Report ===\n\n")
	fmt.Fprintf(w, "Duration: %v\n", p.Duration)
	fmt.Fprintf(w, "Total Allocated: %s (%d bytes)\n", formatBytes(p.TotalAllocated), p.TotalAllocated)
	fmt.Fprintf(w, "Total Freed: %s (%d bytes)\n", formatBytes(p.TotalFreed), p.TotalFreed)
	fmt.Fprintf(w, "Current Live: %s (%d bytes)\n", formatBytes(p.CurrentLive), p.CurrentLive)
	fmt.Fprintf(w, "Allocation Count: %d\n\n", p.AllocationCount)
	
	fmt.Fprintf(w, "=== Top Allocators ===\n\n")
	fmt.Fprintf(w, "%-20s %-30s %12s %12s %10s %10s\n",
		"Type", "AllocSite", "TotalBytes", "LiveBytes", "Count", "AvgSize")
	fmt.Fprintf(w, "%s\n", "--------------------------------------------------------------------------------------------")
	
	for _, stats := range p.TopAllocators {
		fmt.Fprintf(w, "%-20s %-30s %12s %12s %10d %10s\n",
			truncateString(stats.ObjectType, 20),
			truncateString(stats.AllocSite, 30),
			formatBytes(stats.TotalBytes),
			formatBytes(stats.LiveBytes),
			stats.TotalAllocations,
			formatBytes(int64(stats.AvgSize)))
	}
	
	return nil
}

// ExportHeapDump 导出堆转储
func (p *MemoryProfile) ExportHeapDump(w io.Writer) error {
	fmt.Fprintf(w, "# Heap Dump\n")
	fmt.Fprintf(w, "# Type,Site,LiveObjects,LiveBytes\n")
	
	for _, stats := range p.AllocationStats {
		if stats.LiveObjects > 0 {
			fmt.Fprintf(w, "%s,%s,%d,%d\n",
				stats.ObjectType, stats.AllocSite,
				stats.LiveObjects, stats.LiveBytes)
		}
	}
	
	return nil
}

// DetectLeaks 检测潜在内存泄漏
func (p *MemoryProfile) DetectLeaks(threshold float64) []*AllocationStats {
	var leaks []*AllocationStats
	
	for _, stats := range p.AllocationStats {
		// 如果存活率超过阈值，可能是泄漏
		if stats.TotalAllocations > 0 {
			liveRate := float64(stats.LiveObjects) / float64(stats.TotalAllocations)
			if liveRate >= threshold && stats.LiveObjects > 10 {
				leaks = append(leaks, stats)
			}
		}
	}
	
	// 按存活字节数排序
	sort.Slice(leaks, func(i, j int) bool {
		return leaks[i].LiveBytes > leaks[j].LiveBytes
	})
	
	return leaks
}

// ============================================================================
// 辅助函数
// ============================================================================

// formatBytes 格式化字节数
func formatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)
	
	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// GetTypeStats 获取特定类型的统计
func (p *MemoryProfile) GetTypeStats(typeName string) []*AllocationStats {
	var result []*AllocationStats
	
	for _, stats := range p.AllocationStats {
		if stats.ObjectType == typeName {
			result = append(result, stats)
		}
	}
	
	return result
}
