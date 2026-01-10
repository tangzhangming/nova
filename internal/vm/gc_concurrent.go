// gc_concurrent.go - 并发垃圾回收
//
// 实现并发三色标记-清除垃圾回收算法。
//
// 特性：
// 1. 并发标记 - 与程序执行并行进行
// 2. 写屏障 - 保证标记正确性
// 3. 后台 GC goroutine
// 4. STW 时间最小化
// 5. 与现有 GC 共存

package vm

import (
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// ConcurrentGC 并发垃圾回收器
type ConcurrentGC struct {
	mu sync.RWMutex
	
	// 对象堆
	heap *ConcurrentHeap
	
	// 标记状态
	markPhase    int32 // 0=idle, 1=marking, 2=sweeping
	markComplete int32 // 标记完成标志
	
	// 工作队列
	workQueue     chan GCObject
	workQueueSize int
	
	// 后台 goroutine
	bgWorkers    int
	bgStopChan   chan struct{}
	bgDoneChan   chan struct{}
	bgRunning    int32
	
	// 写屏障
	writeBarrier *ConcurrentWriteBarrier
	
	// 统计
	stats ConcurrentGCStats
	
	// 配置
	config ConcurrentGCConfig
	
	// 根扫描器
	rootScanner RootScanner
}

// ConcurrentHeap 并发堆
type ConcurrentHeap struct {
	mu sync.RWMutex
	
	// 对象存储
	objects map[uintptr]*ConcurrentGCObject
	
	// 白/灰/黑集合
	whiteSet map[uintptr]*ConcurrentGCObject
	graySet  map[uintptr]*ConcurrentGCObject
	blackSet map[uintptr]*ConcurrentGCObject
	
	// 统计
	totalObjects int64
	totalBytes   int64
}

// ConcurrentGCObject 并发 GC 对象
type ConcurrentGCObject struct {
	ptr      uintptr
	size     int64
	color    int32 // 原子操作
	children []uintptr
	
	// 元数据
	typeName    string
	allocTime   time.Time
	generation  int32
}

// ConcurrentWriteBarrier 并发写屏障
type ConcurrentWriteBarrier struct {
	mu sync.RWMutex
	
	enabled int32 // 原子操作
	
	// 记录修改的对象
	modifiedObjects map[uintptr]bool
	
	// Dijkstra 插入屏障记录
	insertionBuffer []uintptr
	
	// Yuasa 删除屏障记录
	deletionBuffer []uintptr
}

// ConcurrentGCStats 并发 GC 统计
type ConcurrentGCStats struct {
	TotalCollections  int64
	TotalMarked       int64
	TotalSwept        int64
	TotalFreed        int64
	TotalSTWTime      time.Duration
	LastSTWTime       time.Duration
	MaxSTWTime        time.Duration
	ConcurrentMarkTime time.Duration
	ConcurrentSweepTime time.Duration
}

// ConcurrentGCConfig 并发 GC 配置
type ConcurrentGCConfig struct {
	// Enabled 是否启用并发 GC
	Enabled bool
	
	// WorkerCount 后台工作 goroutine 数量
	WorkerCount int
	
	// WorkQueueSize 工作队列大小
	WorkQueueSize int
	
	// MarkSliceSize 每次标记的对象数量
	MarkSliceSize int
	
	// SweepSliceSize 每次清除的对象数量
	SweepSliceSize int
	
	// TriggerRatio 触发 GC 的堆增长比例
	TriggerRatio float64
}

// RootScanner 根扫描器接口
type RootScanner interface {
	// ScanRoots 扫描根对象
	ScanRoots() []uintptr
}

// 并发 GC 阶段
const (
	gcPhaseIdle     int32 = iota
	gcPhaseMarking
	gcPhaseSweeping
)

// 对象颜色
const (
	colorWhite int32 = iota
	colorGray
	colorBlack
)

// DefaultConcurrentGCConfig 默认配置
func DefaultConcurrentGCConfig() ConcurrentGCConfig {
	return ConcurrentGCConfig{
		Enabled:        true,
		WorkerCount:    runtime.NumCPU() / 2,
		WorkQueueSize:  10000,
		MarkSliceSize:  100,
		SweepSliceSize: 100,
		TriggerRatio:   0.25, // 堆增长 25% 时触发
	}
}

// NewConcurrentGC 创建并发 GC
func NewConcurrentGC(config ConcurrentGCConfig) *ConcurrentGC {
	if config.WorkerCount < 1 {
		config.WorkerCount = 1
	}
	
	gc := &ConcurrentGC{
		heap: &ConcurrentHeap{
			objects:  make(map[uintptr]*ConcurrentGCObject),
			whiteSet: make(map[uintptr]*ConcurrentGCObject),
			graySet:  make(map[uintptr]*ConcurrentGCObject),
			blackSet: make(map[uintptr]*ConcurrentGCObject),
		},
		workQueue:     make(chan GCObject, config.WorkQueueSize),
		workQueueSize: config.WorkQueueSize,
		bgWorkers:     config.WorkerCount,
		bgStopChan:    make(chan struct{}),
		bgDoneChan:    make(chan struct{}),
		writeBarrier: &ConcurrentWriteBarrier{
			modifiedObjects: make(map[uintptr]bool),
			insertionBuffer: make([]uintptr, 0),
			deletionBuffer:  make([]uintptr, 0),
		},
		config: config,
	}
	
	return gc
}

// Start 启动并发 GC
func (gc *ConcurrentGC) Start() {
	if !gc.config.Enabled {
		return
	}
	
	if !atomic.CompareAndSwapInt32(&gc.bgRunning, 0, 1) {
		return // 已经在运行
	}
	
	// 启动后台工作 goroutine
	for i := 0; i < gc.bgWorkers; i++ {
		go gc.bgWorker(i)
	}
	
	// 启动 GC 协调器
	go gc.coordinator()
}

// Stop 停止并发 GC
func (gc *ConcurrentGC) Stop() {
	if !atomic.CompareAndSwapInt32(&gc.bgRunning, 1, 0) {
		return
	}
	
	close(gc.bgStopChan)
	
	// 等待所有工作完成
	for i := 0; i < gc.bgWorkers+1; i++ {
		<-gc.bgDoneChan
	}
}

// Trigger 触发一次 GC 周期
func (gc *ConcurrentGC) Trigger() {
	gc.mu.Lock()
	defer gc.mu.Unlock()
	
	if atomic.LoadInt32(&gc.markPhase) != gcPhaseIdle {
		return // GC 已在进行
	}
	
	atomic.AddInt64(&gc.stats.TotalCollections, 1)
	gc.startMarkPhase()
}

// ============================================================================
// GC 阶段实现
// ============================================================================

// startMarkPhase 开始标记阶段
func (gc *ConcurrentGC) startMarkPhase() {
	atomic.StoreInt32(&gc.markPhase, gcPhaseMarking)
	
	// 启用写屏障
	gc.writeBarrier.Enable()
	
	// STW: 扫描根对象
	stwStart := time.Now()
	gc.scanRoots()
	stwDuration := time.Since(stwStart)
	
	gc.stats.LastSTWTime = stwDuration
	gc.stats.TotalSTWTime += stwDuration
	if stwDuration > gc.stats.MaxSTWTime {
		gc.stats.MaxSTWTime = stwDuration
	}
	
	// 开始并发标记
	atomic.StoreInt32(&gc.markComplete, 0)
}

// scanRoots 扫描根对象
func (gc *ConcurrentGC) scanRoots() {
	gc.heap.mu.Lock()
	defer gc.heap.mu.Unlock()
	
	// 将所有对象标记为白色
	for ptr, obj := range gc.heap.objects {
		atomic.StoreInt32(&obj.color, colorWhite)
		gc.heap.whiteSet[ptr] = obj
	}
	gc.heap.graySet = make(map[uintptr]*ConcurrentGCObject)
	gc.heap.blackSet = make(map[uintptr]*ConcurrentGCObject)
	
	// 扫描根对象
	if gc.rootScanner != nil {
		roots := gc.rootScanner.ScanRoots()
		for _, ptr := range roots {
			if obj, ok := gc.heap.objects[ptr]; ok {
				gc.markGray(obj)
			}
		}
	}
}

// markGray 将对象标记为灰色
func (gc *ConcurrentGC) markGray(obj *ConcurrentGCObject) {
	if atomic.CompareAndSwapInt32(&obj.color, colorWhite, colorGray) {
		gc.heap.mu.Lock()
		delete(gc.heap.whiteSet, obj.ptr)
		gc.heap.graySet[obj.ptr] = obj
		gc.heap.mu.Unlock()
	}
}

// markBlack 将对象标记为黑色
func (gc *ConcurrentGC) markBlack(obj *ConcurrentGCObject) {
	if atomic.CompareAndSwapInt32(&obj.color, colorGray, colorBlack) {
		gc.heap.mu.Lock()
		delete(gc.heap.graySet, obj.ptr)
		gc.heap.blackSet[obj.ptr] = obj
		gc.heap.mu.Unlock()
		
		atomic.AddInt64(&gc.stats.TotalMarked, 1)
	}
}

// concurrentMark 并发标记
func (gc *ConcurrentGC) concurrentMark() bool {
	gc.heap.mu.Lock()
	if len(gc.heap.graySet) == 0 {
		gc.heap.mu.Unlock()
		return true // 标记完成
	}
	
	// 获取一批灰色对象
	batch := make([]*ConcurrentGCObject, 0, gc.config.MarkSliceSize)
	for ptr, obj := range gc.heap.graySet {
		batch = append(batch, obj)
		if len(batch) >= gc.config.MarkSliceSize {
			break
		}
		_ = ptr
	}
	gc.heap.mu.Unlock()
	
	// 处理灰色对象
	for _, obj := range batch {
		// 扫描子对象
		for _, childPtr := range obj.children {
			gc.heap.mu.RLock()
			child, ok := gc.heap.objects[childPtr]
			gc.heap.mu.RUnlock()
			
			if ok {
				gc.markGray(child)
			}
		}
		
		// 将对象标记为黑色
		gc.markBlack(obj)
	}
	
	return false
}

// startSweepPhase 开始清除阶段
func (gc *ConcurrentGC) startSweepPhase() {
	atomic.StoreInt32(&gc.markPhase, gcPhaseSweeping)
	
	// 处理写屏障缓冲区
	gc.processWriteBarrierBuffers()
	
	// 禁用写屏障
	gc.writeBarrier.Disable()
}

// concurrentSweep 并发清除
func (gc *ConcurrentGC) concurrentSweep() bool {
	gc.heap.mu.Lock()
	if len(gc.heap.whiteSet) == 0 {
		gc.heap.mu.Unlock()
		return true // 清除完成
	}
	
	// 获取一批白色对象（待回收）
	batch := make([]*ConcurrentGCObject, 0, gc.config.SweepSliceSize)
	for ptr, obj := range gc.heap.whiteSet {
		batch = append(batch, obj)
		delete(gc.heap.whiteSet, ptr)
		if len(batch) >= gc.config.SweepSliceSize {
			break
		}
	}
	gc.heap.mu.Unlock()
	
	// 回收对象
	var freedBytes int64
	for _, obj := range batch {
		freedBytes += obj.size
		
		gc.heap.mu.Lock()
		delete(gc.heap.objects, obj.ptr)
		gc.heap.mu.Unlock()
		
		atomic.AddInt64(&gc.stats.TotalSwept, 1)
		atomic.AddInt64(&gc.stats.TotalFreed, 1)
	}
	
	atomic.AddInt64(&gc.heap.totalBytes, -freedBytes)
	atomic.AddInt64(&gc.heap.totalObjects, -int64(len(batch)))
	
	return false
}

// finishCycle 完成 GC 周期
func (gc *ConcurrentGC) finishCycle() {
	atomic.StoreInt32(&gc.markPhase, gcPhaseIdle)
	
	// 将黑色对象重置为白色（为下一个周期做准备）
	gc.heap.mu.Lock()
	for ptr, obj := range gc.heap.blackSet {
		atomic.StoreInt32(&obj.color, colorWhite)
		gc.heap.whiteSet[ptr] = obj
	}
	gc.heap.blackSet = make(map[uintptr]*ConcurrentGCObject)
	gc.heap.mu.Unlock()
}

// ============================================================================
// 写屏障实现
// ============================================================================

// Enable 启用写屏障
func (wb *ConcurrentWriteBarrier) Enable() {
	atomic.StoreInt32(&wb.enabled, 1)
}

// Disable 禁用写屏障
func (wb *ConcurrentWriteBarrier) Disable() {
	atomic.StoreInt32(&wb.enabled, 0)
}

// IsEnabled 检查是否启用
func (wb *ConcurrentWriteBarrier) IsEnabled() bool {
	return atomic.LoadInt32(&wb.enabled) == 1
}

// WritePointer 写指针屏障（混合屏障）
// 在写指针时调用，记录旧值和新值
func (wb *ConcurrentWriteBarrier) WritePointer(slot *uintptr, oldVal, newVal uintptr) {
	if !wb.IsEnabled() {
		*slot = newVal
		return
	}
	
	wb.mu.Lock()
	// Dijkstra 插入屏障：标记新引用的对象
	if newVal != 0 {
		wb.insertionBuffer = append(wb.insertionBuffer, newVal)
	}
	// Yuasa 删除屏障：标记即将被删除引用的对象
	if oldVal != 0 {
		wb.deletionBuffer = append(wb.deletionBuffer, oldVal)
	}
	wb.mu.Unlock()
	
	*slot = newVal
}

// RecordModification 记录对象修改
func (wb *ConcurrentWriteBarrier) RecordModification(ptr uintptr) {
	if !wb.IsEnabled() {
		return
	}
	
	wb.mu.Lock()
	wb.modifiedObjects[ptr] = true
	wb.mu.Unlock()
}

// processWriteBarrierBuffers 处理写屏障缓冲区
func (gc *ConcurrentGC) processWriteBarrierBuffers() {
	gc.writeBarrier.mu.Lock()
	insertions := gc.writeBarrier.insertionBuffer
	deletions := gc.writeBarrier.deletionBuffer
	gc.writeBarrier.insertionBuffer = make([]uintptr, 0)
	gc.writeBarrier.deletionBuffer = make([]uintptr, 0)
	gc.writeBarrier.mu.Unlock()
	
	// 处理插入的对象
	for _, ptr := range insertions {
		gc.heap.mu.RLock()
		obj, ok := gc.heap.objects[ptr]
		gc.heap.mu.RUnlock()
		
		if ok && atomic.LoadInt32(&obj.color) == colorWhite {
			gc.markGray(obj)
		}
	}
	
	// 处理删除的对象
	for _, ptr := range deletions {
		gc.heap.mu.RLock()
		obj, ok := gc.heap.objects[ptr]
		gc.heap.mu.RUnlock()
		
		if ok && atomic.LoadInt32(&obj.color) == colorWhite {
			gc.markGray(obj)
		}
	}
}

// ============================================================================
// 后台 goroutine
// ============================================================================

// bgWorker 后台工作 goroutine
func (gc *ConcurrentGC) bgWorker(id int) {
	defer func() {
		gc.bgDoneChan <- struct{}{}
	}()
	
	for {
		select {
		case <-gc.bgStopChan:
			return
		default:
			phase := atomic.LoadInt32(&gc.markPhase)
			
			switch phase {
			case gcPhaseMarking:
				if gc.concurrentMark() {
					// 标记完成，转到清除阶段
					if atomic.CompareAndSwapInt32(&gc.markComplete, 0, 1) {
						gc.startSweepPhase()
					}
				}
			case gcPhaseSweeping:
				if gc.concurrentSweep() {
					// 清除完成
					gc.finishCycle()
				}
			default:
				// 空闲状态，短暂睡眠
				time.Sleep(time.Millisecond)
			}
		}
	}
}

// coordinator GC 协调器
func (gc *ConcurrentGC) coordinator() {
	defer func() {
		gc.bgDoneChan <- struct{}{}
	}()
	
	var lastHeapSize int64
	
	for {
		select {
		case <-gc.bgStopChan:
			return
		case <-time.After(100 * time.Millisecond):
			// 检查是否需要触发 GC
			currentSize := atomic.LoadInt64(&gc.heap.totalBytes)
			if lastHeapSize > 0 {
				growthRatio := float64(currentSize-lastHeapSize) / float64(lastHeapSize)
				if growthRatio >= gc.config.TriggerRatio {
					gc.Trigger()
					lastHeapSize = currentSize
				}
			} else {
				lastHeapSize = currentSize
			}
		}
	}
}

// ============================================================================
// 对象管理
// ============================================================================

// Allocate 分配对象
func (gc *ConcurrentGC) Allocate(ptr uintptr, size int64, typeName string) {
	obj := &ConcurrentGCObject{
		ptr:       ptr,
		size:      size,
		color:     colorWhite,
		typeName:  typeName,
		allocTime: time.Now(),
	}
	
	gc.heap.mu.Lock()
	gc.heap.objects[ptr] = obj
	gc.heap.whiteSet[ptr] = obj
	gc.heap.mu.Unlock()
	
	atomic.AddInt64(&gc.heap.totalObjects, 1)
	atomic.AddInt64(&gc.heap.totalBytes, size)
}

// AddReference 添加引用
func (gc *ConcurrentGC) AddReference(parentPtr, childPtr uintptr) {
	gc.heap.mu.Lock()
	parent, ok := gc.heap.objects[parentPtr]
	if ok {
		parent.children = append(parent.children, childPtr)
	}
	gc.heap.mu.Unlock()
	
	// 如果正在标记阶段，使用写屏障
	if atomic.LoadInt32(&gc.markPhase) == gcPhaseMarking {
		gc.writeBarrier.WritePointer(nil, 0, childPtr)
	}
}

// GetStats 获取统计信息
func (gc *ConcurrentGC) GetStats() ConcurrentGCStats {
	return ConcurrentGCStats{
		TotalCollections:    atomic.LoadInt64(&gc.stats.TotalCollections),
		TotalMarked:         atomic.LoadInt64(&gc.stats.TotalMarked),
		TotalSwept:          atomic.LoadInt64(&gc.stats.TotalSwept),
		TotalFreed:          atomic.LoadInt64(&gc.stats.TotalFreed),
		TotalSTWTime:        gc.stats.TotalSTWTime,
		LastSTWTime:         gc.stats.LastSTWTime,
		MaxSTWTime:          gc.stats.MaxSTWTime,
		ConcurrentMarkTime:  gc.stats.ConcurrentMarkTime,
		ConcurrentSweepTime: gc.stats.ConcurrentSweepTime,
	}
}

// SetRootScanner 设置根扫描器
func (gc *ConcurrentGC) SetRootScanner(scanner RootScanner) {
	gc.mu.Lock()
	defer gc.mu.Unlock()
	gc.rootScanner = scanner
}
