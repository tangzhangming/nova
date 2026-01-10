package lsp2

import (
	"context"
	"runtime"
	"time"
)

// MemoryMonitor 内存监控器
type MemoryMonitor struct {
	server  *Server
	logger  *Logger
	enabled bool
}

// NewMemoryMonitor 创建内存监控器
func NewMemoryMonitor(server *Server, logger *Logger) *MemoryMonitor {
	return &MemoryMonitor{
		server:  server,
		logger:  logger,
		enabled: true,
	}
}

// Start 启动内存监控
func (mm *MemoryMonitor) Start(ctx context.Context) {
	if !mm.enabled {
		return
	}

	mm.logger.Debug("Memory monitor started")

	ticker := time.NewTicker(60 * time.Second) // 每60秒检查一次
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			mm.logger.Debug("Memory monitor stopped")
			return
		case <-ticker.C:
			mm.checkMemory()
		}
	}
}

// checkMemory 检查内存使用情况
func (mm *MemoryMonitor) checkMemory() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	heapAllocMB := float64(m.HeapAlloc) / 1024 / 1024
	heapSysMB := float64(m.HeapSys) / 1024 / 1024
	numGoroutine := runtime.NumGoroutine()

	// 获取当前状态
	docCount := 0
	importCacheSize := 0

	if mm.server.docManager != nil {
		docCount = mm.server.docManager.Count()
	}
	if mm.server.importResolver != nil {
		importCacheSize = mm.server.importResolver.CacheSize()
	}

	mm.logger.Debug("Memory stats: HeapAlloc=%.2fMB, HeapSys=%.2fMB, Goroutines=%d, Docs=%d, ImportCache=%d",
		heapAllocMB, heapSysMB, numGoroutine, docCount, importCacheSize)

	// 检查内存使用是否过高
	const maxHeapMB = 100.0
	if heapAllocMB > maxHeapMB {
		mm.logger.Info("Memory usage high (%.2fMB > %.0fMB), cleaning caches", heapAllocMB, maxHeapMB)
		mm.cleanMemory()

		// 再次检查内存
		runtime.ReadMemStats(&m)
		newHeapMB := float64(m.HeapAlloc) / 1024 / 1024
		mm.logger.Info("Memory after cleanup: %.2fMB", newHeapMB)
	}

	// 检查 Goroutine 泄漏
	const maxGoroutines = 50
	if numGoroutine > maxGoroutines {
		mm.logger.Error("Goroutine count high: %d > %d (possible goroutine leak)", numGoroutine, maxGoroutines)
	}
}

// cleanMemory 清理内存
func (mm *MemoryMonitor) cleanMemory() {
	// 清理导入缓存
	if mm.server.importResolver != nil {
		mm.server.importResolver.ClearCache()
	}

	// 强制垃圾回收
	runtime.GC()

	mm.logger.Debug("Memory cleanup completed")
}

// GetMemoryStats 获取内存统计信息
func (mm *MemoryMonitor) GetMemoryStats() MemoryStats {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	stats := MemoryStats{
		HeapAllocMB:     float64(m.HeapAlloc) / 1024 / 1024,
		HeapSysMB:       float64(m.HeapSys) / 1024 / 1024,
		NumGoroutine:    runtime.NumGoroutine(),
		NumGC:           int(m.NumGC),
	}

	if mm.server.docManager != nil {
		stats.DocumentCount = mm.server.docManager.Count()
	}
	if mm.server.importResolver != nil {
		stats.ImportCacheSize = mm.server.importResolver.CacheSize()
	}

	return stats
}

// MemoryStats 内存统计信息
type MemoryStats struct {
	HeapAllocMB      float64
	HeapSysMB        float64
	NumGoroutine     int
	NumGC            int
	DocumentCount    int
	ImportCacheSize  int
}
