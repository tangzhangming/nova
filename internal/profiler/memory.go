// memory.go - 内存性能分析
//
// 实现内存性能分析功能：
// 1. 对象分配追踪
// 2. 内存使用统计
// 3. 内存泄漏检测
// 4. 分配热点识别

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

// MemoryProfiler 内存分析器
type MemoryProfiler struct {
	mu sync.RWMutex
	
	// 配置
	config ProfileConfig
	
	// 状态
	running bool
	
	// 分配统计
	typeStats       map[string]*TypeAllocationStats
	allocationCount int64
	totalBytes      int64
	
	// 采样器
	sampler *Sampler
	
	// 分配记录（采样）
	allocRecords []AllocationRecord
}

// TypeAllocationStats 类型分配统计
type TypeAllocationStats struct {
	TypeName    string
	AllocCount  int64
	FreeCount   int64
	TotalBytes  int64
	FreedBytes  int64
	LiveCount   int64
	LiveBytes   int64
	MaxLive     int64
	MaxLiveBytes int64
	
	// 分配位置统计
	AllocSites map[string]*AllocSiteStats
}

// AllocSiteStats 分配位置统计
type AllocSiteStats struct {
	File       string
	Line       int
	Function   string
	AllocCount int64
	TotalBytes int64
}

// AllocationRecord 分配记录
type AllocationRecord struct {
	Timestamp time.Time
	TypeName  string
	Size      int64
	Stack     []StackFrame
	IsDealloc bool
}

// MemoryProfile 内存分析结果
type MemoryProfile struct {
	StartTime        time.Time
	EndTime          time.Time
	TotalAllocations int64
	TotalBytes       int64
	LiveObjects      int64
	LiveBytes        int64
	TypeStats        []*TypeAllocationStats
	TopAllocators    []*TypeAllocationStats // 按分配次数排序
	TopMemory        []*TypeAllocationStats // 按内存使用排序
}

// NewMemoryProfiler 创建内存分析器
func NewMemoryProfiler(config ProfileConfig) *MemoryProfiler {
	return &MemoryProfiler{
		config:    config,
		typeStats: make(map[string]*TypeAllocationStats),
		sampler:   NewSampler(int64(config.MemorySampleRate)),
	}
}

// Start 开始分析
func (p *MemoryProfiler) Start() {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	p.running = true
	p.allocRecords = make([]AllocationRecord, 0)
}

// Stop 停止分析
func (p *MemoryProfiler) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	p.running = false
}

// RecordAllocation 记录内存分配
func (p *MemoryProfiler) RecordAllocation(typeName string, size int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	atomic.AddInt64(&p.allocationCount, 1)
	atomic.AddInt64(&p.totalBytes, size)
	
	// 更新类型统计
	stats := p.getOrCreateTypeStats(typeName)
	stats.AllocCount++
	stats.TotalBytes += size
	stats.LiveCount++
	stats.LiveBytes += size
	
	if stats.LiveCount > stats.MaxLive {
		stats.MaxLive = stats.LiveCount
	}
	if stats.LiveBytes > stats.MaxLiveBytes {
		stats.MaxLiveBytes = stats.LiveBytes
	}
	
	// 采样记录
	if p.sampler.ShouldSample() {
		record := AllocationRecord{
			Timestamp: time.Now(),
			TypeName:  typeName,
			Size:      size,
			IsDealloc: false,
		}
		p.allocRecords = append(p.allocRecords, record)
	}
}

// RecordDeallocation 记录内存释放
func (p *MemoryProfiler) RecordDeallocation(typeName string, size int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	// 更新类型统计
	stats := p.getOrCreateTypeStats(typeName)
	stats.FreeCount++
	stats.FreedBytes += size
	stats.LiveCount--
	stats.LiveBytes -= size
	
	// 防止负数
	if stats.LiveCount < 0 {
		stats.LiveCount = 0
	}
	if stats.LiveBytes < 0 {
		stats.LiveBytes = 0
	}
}

// RecordAllocationWithStack 记录带调用栈的内存分配
func (p *MemoryProfiler) RecordAllocationWithStack(typeName string, size int64, stack []StackFrame) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	atomic.AddInt64(&p.allocationCount, 1)
	atomic.AddInt64(&p.totalBytes, size)
	
	// 更新类型统计
	stats := p.getOrCreateTypeStats(typeName)
	stats.AllocCount++
	stats.TotalBytes += size
	stats.LiveCount++
	stats.LiveBytes += size
	
	if stats.LiveCount > stats.MaxLive {
		stats.MaxLive = stats.LiveCount
	}
	if stats.LiveBytes > stats.MaxLiveBytes {
		stats.MaxLiveBytes = stats.LiveBytes
	}
	
	// 更新分配位置统计
	if len(stack) > 0 {
		frame := stack[0]
		key := fmt.Sprintf("%s:%d", frame.File, frame.Line)
		site, ok := stats.AllocSites[key]
		if !ok {
			site = &AllocSiteStats{
				File:     frame.File,
				Line:     frame.Line,
				Function: frame.Function,
			}
			stats.AllocSites[key] = site
		}
		site.AllocCount++
		site.TotalBytes += size
	}
	
	// 记录
	record := AllocationRecord{
		Timestamp: time.Now(),
		TypeName:  typeName,
		Size:      size,
		Stack:     stack,
		IsDealloc: false,
	}
	p.allocRecords = append(p.allocRecords, record)
}

// getOrCreateTypeStats 获取或创建类型统计
func (p *MemoryProfiler) getOrCreateTypeStats(typeName string) *TypeAllocationStats {
	stats, ok := p.typeStats[typeName]
	if !ok {
		stats = &TypeAllocationStats{
			TypeName:   typeName,
			AllocSites: make(map[string]*AllocSiteStats),
		}
		p.typeStats[typeName] = stats
	}
	return stats
}

// AllocationCount 获取分配次数
func (p *MemoryProfiler) AllocationCount() int64 {
	return atomic.LoadInt64(&p.allocationCount)
}

// TotalBytes 获取总分配字节数
func (p *MemoryProfiler) TotalBytes() int64 {
	return atomic.LoadInt64(&p.totalBytes)
}

// GetProfile 获取分析结果
func (p *MemoryProfiler) GetProfile() *MemoryProfile {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	profile := &MemoryProfile{
		TotalAllocations: p.allocationCount,
		TotalBytes:       p.totalBytes,
		TypeStats:        make([]*TypeAllocationStats, 0, len(p.typeStats)),
	}
	
	var totalLive int64
	var totalLiveBytes int64
	
	for _, stats := range p.typeStats {
		statsCopy := *stats
		profile.TypeStats = append(profile.TypeStats, &statsCopy)
		totalLive += stats.LiveCount
		totalLiveBytes += stats.LiveBytes
	}
	
	profile.LiveObjects = totalLive
	profile.LiveBytes = totalLiveBytes
	
	// 按分配次数排序
	topByCount := make([]*TypeAllocationStats, len(profile.TypeStats))
	copy(topByCount, profile.TypeStats)
	sort.Slice(topByCount, func(i, j int) bool {
		return topByCount[i].AllocCount > topByCount[j].AllocCount
	})
	topN := 10
	if len(topByCount) < topN {
		topN = len(topByCount)
	}
	profile.TopAllocators = topByCount[:topN]
	
	// 按内存使用排序
	topByMem := make([]*TypeAllocationStats, len(profile.TypeStats))
	copy(topByMem, profile.TypeStats)
	sort.Slice(topByMem, func(i, j int) bool {
		return topByMem[i].LiveBytes > topByMem[j].LiveBytes
	})
	if len(topByMem) < topN {
		topN = len(topByMem)
	}
	profile.TopMemory = topByMem[:topN]
	
	return profile
}

// WriteProfile 写入分析报告
func (p *MemoryProfiler) WriteProfile(w io.Writer, format OutputFormat) error {
	profile := p.GetProfile()
	
	switch format {
	case FormatText:
		return p.writeTextProfile(w, profile)
	case FormatJSON:
		return p.writeJSONProfile(w, profile)
	case FormatPprof:
		return p.writePprofProfile(w, profile)
	default:
		return p.writeTextProfile(w, profile)
	}
}

// writeTextProfile 写入文本格式报告
func (p *MemoryProfiler) writeTextProfile(w io.Writer, profile *MemoryProfile) error {
	fmt.Fprintf(w, "Memory Profile Report\n")
	fmt.Fprintf(w, "=====================\n\n")
	fmt.Fprintf(w, "Total Allocations: %d\n", profile.TotalAllocations)
	fmt.Fprintf(w, "Total Bytes: %s\n", formatBytes(profile.TotalBytes))
	fmt.Fprintf(w, "Live Objects: %d\n", profile.LiveObjects)
	fmt.Fprintf(w, "Live Bytes: %s\n\n", formatBytes(profile.LiveBytes))
	
	fmt.Fprintf(w, "Top Allocators:\n")
	fmt.Fprintf(w, "%-30s %10s %15s %10s %15s\n", "Type", "Allocs", "Total", "Live", "Live Bytes")
	fmt.Fprintf(w, strings.Repeat("-", 85) + "\n")
	
	for _, stats := range profile.TopAllocators {
		fmt.Fprintf(w, "%-30s %10d %15s %10d %15s\n",
			truncateName(stats.TypeName, 30),
			stats.AllocCount,
			formatBytes(stats.TotalBytes),
			stats.LiveCount,
			formatBytes(stats.LiveBytes),
		)
	}
	
	fmt.Fprintf(w, "\nTop Memory Users:\n")
	fmt.Fprintf(w, "%-30s %10s %15s %10s %15s\n", "Type", "Live", "Live Bytes", "Max Live", "Max Bytes")
	fmt.Fprintf(w, strings.Repeat("-", 85) + "\n")
	
	for _, stats := range profile.TopMemory {
		fmt.Fprintf(w, "%-30s %10d %15s %10d %15s\n",
			truncateName(stats.TypeName, 30),
			stats.LiveCount,
			formatBytes(stats.LiveBytes),
			stats.MaxLive,
			formatBytes(stats.MaxLiveBytes),
		)
	}
	
	return nil
}

// writeJSONProfile 写入 JSON 格式报告
func (p *MemoryProfiler) writeJSONProfile(w io.Writer, profile *MemoryProfile) error {
	fmt.Fprintf(w, "{\n")
	fmt.Fprintf(w, "  \"totalAllocations\": %d,\n", profile.TotalAllocations)
	fmt.Fprintf(w, "  \"totalBytes\": %d,\n", profile.TotalBytes)
	fmt.Fprintf(w, "  \"liveObjects\": %d,\n", profile.LiveObjects)
	fmt.Fprintf(w, "  \"liveBytes\": %d,\n", profile.LiveBytes)
	fmt.Fprintf(w, "  \"types\": [\n")
	
	for i, stats := range profile.TypeStats {
		fmt.Fprintf(w, "    {\n")
		fmt.Fprintf(w, "      \"name\": \"%s\",\n", stats.TypeName)
		fmt.Fprintf(w, "      \"allocCount\": %d,\n", stats.AllocCount)
		fmt.Fprintf(w, "      \"totalBytes\": %d,\n", stats.TotalBytes)
		fmt.Fprintf(w, "      \"liveCount\": %d,\n", stats.LiveCount)
		fmt.Fprintf(w, "      \"liveBytes\": %d\n", stats.LiveBytes)
		if i < len(profile.TypeStats)-1 {
			fmt.Fprintf(w, "    },\n")
		} else {
			fmt.Fprintf(w, "    }\n")
		}
	}
	
	fmt.Fprintf(w, "  ]\n")
	fmt.Fprintf(w, "}\n")
	
	return nil
}

// writePprofProfile 写入 pprof 格式
func (p *MemoryProfiler) writePprofProfile(w io.Writer, profile *MemoryProfile) error {
	fmt.Fprintf(w, "--- Memory Profile ---\n")
	fmt.Fprintf(w, "heap profile: %d: %d [%d: %d] @ heapprofile\n",
		profile.LiveObjects,
		profile.LiveBytes,
		profile.TotalAllocations,
		profile.TotalBytes,
	)
	
	for _, stats := range profile.TypeStats {
		fmt.Fprintf(w, "%d: %d [%d: %d] @ %s\n",
			stats.LiveCount,
			stats.LiveBytes,
			stats.AllocCount,
			stats.TotalBytes,
			stats.TypeName,
		)
	}
	
	return nil
}

// Reset 重置分析器
func (p *MemoryProfiler) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	p.typeStats = make(map[string]*TypeAllocationStats)
	p.allocRecords = nil
	p.allocationCount = 0
	p.totalBytes = 0
	p.sampler.Reset()
}

// formatBytes 格式化字节数
func formatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
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
