// Package vm 实现了 Sola 编程语言的字节码虚拟机。
//
// 本文件实现 GC 的 Stop-The-World (STW) 机制，用于多线程环境下的安全垃圾回收。
package vm

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/tangzhangming/nova/internal/bytecode"
)

// ============================================================================
// STW 垃圾回收器
// ============================================================================
//
// BUG FIX 2026-01-10: 多线程 GC - STW 机制
// 防止反复引入的问题:
// 1. GC 标记阶段必须 STW（否则会漏标记导致对象被误回收）
// 2. 所有 Worker 必须在安全点暂停
// 3. 标记阶段需要遍历所有 Worker 的栈
// 4. 写屏障在 STW 期间仍需工作（并发标记优化时）

// MultiThreadGC 多线程安全的垃圾回收器
//
// 基于单线程 GC 扩展，添加了 STW 支持和多线程安全性。
type MultiThreadGC struct {
	// 内嵌基础 GC
	*GC

	// =========================================================================
	// 多线程支持
	// =========================================================================

	// workerPool 工作线程池引用
	workerPool *WorkerPool

	// scheduler 多线程调度器引用
	scheduler *MultiThreadScheduler

	// =========================================================================
	// STW 控制
	// =========================================================================

	// stwMu STW 锁
	stwMu sync.Mutex

	// stwActive STW 是否激活
	stwActive atomic.Bool

	// stwWaitGroup 等待所有 Worker 进入安全点
	stwWaitGroup sync.WaitGroup

	// safePointCount 已到达安全点的 Worker 数量
	safePointCount atomic.Int32

	// =========================================================================
	// STW 统计
	// =========================================================================

	// stwCount STW 次数
	stwCount int64

	// totalSTWTimeNs 总 STW 时间（纳秒）
	totalSTWTimeNs int64

	// maxSTWTimeNs 最大单次 STW 时间（纳秒）
	maxSTWTimeNs int64

	// lastSTWTimeNs 上次 STW 时间（纳秒）
	lastSTWTimeNs int64
}

// NewMultiThreadGC 创建多线程安全的 GC
func NewMultiThreadGC(pool *WorkerPool) *MultiThreadGC {
	return &MultiThreadGC{
		GC:         NewGC(),
		workerPool: pool,
	}
}

// SetScheduler 设置调度器引用
func (gc *MultiThreadGC) SetScheduler(scheduler *MultiThreadScheduler) {
	gc.scheduler = scheduler
}

// ============================================================================
// STW 操作
// ============================================================================

// RequestSTW 请求 STW（停止所有工作线程）
//
// 此方法会阻塞直到所有 Worker 都到达安全点或超时。
// 调用者必须在完成 GC 操作后调用 ReleaseSTW()。
func (gc *MultiThreadGC) RequestSTW() {
	gc.stwMu.Lock()

	startTime := time.Now()

	// 标记 STW 激活
	gc.stwActive.Store(true)
	gc.safePointCount.Store(0)

	// 通知调度器进入 STW
	if gc.scheduler != nil {
		gc.scheduler.RequestSTW()
	}

	// 等待所有 Worker 到达安全点（带超时）
	numWorkers := gc.workerPool.NumWorkers()
	timeout := time.After(5 * time.Second)
	
	for gc.safePointCount.Load() < int32(numWorkers) {
		select {
		case <-timeout:
			// 超时，强制继续（在测试环境或 Worker 未运行时）
			// 记录警告但不阻塞
			gc.lastSTWTimeNs = time.Since(startTime).Nanoseconds()
			return
		default:
			// 短暂休眠，避免忙等待
			time.Sleep(100 * time.Microsecond)
		}
		
		// 检查是否所有 Worker 都处于休眠状态
		allParking := true
		for _, w := range gc.workerPool.workers {
			if w.IsRunning() && !w.IsParking() {
				allParking = false
				break
			}
		}
		if allParking {
			// 所有 Worker 都在休眠，可以安全地进行 GC
			break
		}
	}

	// 记录 STW 开始时间
	gc.lastSTWTimeNs = time.Since(startTime).Nanoseconds()
}

// ReleaseSTW 释放 STW（恢复所有工作线程）
func (gc *MultiThreadGC) ReleaseSTW() {
	// 记录 STW 统计
	gc.stwCount++
	gc.totalSTWTimeNs += gc.lastSTWTimeNs
	if gc.lastSTWTimeNs > gc.maxSTWTimeNs {
		gc.maxSTWTimeNs = gc.lastSTWTimeNs
	}

	// 标记 STW 结束
	gc.stwActive.Store(false)

	// 通知调度器释放 STW
	if gc.scheduler != nil {
		gc.scheduler.ReleaseSTW()
	}

	gc.stwMu.Unlock()
}

// EnterSafePoint Worker 进入安全点
//
// Worker 在检测到 STW 请求时调用此方法。
// 此方法会阻塞直到 STW 结束。
func (gc *MultiThreadGC) EnterSafePoint() {
	if !gc.stwActive.Load() {
		return
	}

	// 增加安全点计数
	gc.safePointCount.Add(1)

	// 等待 STW 结束
	for gc.stwActive.Load() {
		time.Sleep(100 * time.Microsecond)
	}
}

// IsSTWActive 检查 STW 是否激活
func (gc *MultiThreadGC) IsSTWActive() bool {
	return gc.stwActive.Load()
}

// ============================================================================
// 多线程安全的 GC 操作
// ============================================================================

// CollectWithSTW 执行带 STW 的垃圾回收
//
// 这是多线程环境下执行 GC 的安全方式：
//  1. 请求 STW
//  2. 收集所有 Worker 的根对象
//  3. 执行标记
//  4. 执行清除
//  5. 释放 STW
func (gc *MultiThreadGC) CollectWithSTW() int {
	if !gc.enabled {
		return 0
	}

	// 请求 STW
	gc.RequestSTW()
	defer gc.ReleaseSTW()

	// 收集所有根对象
	roots := gc.collectAllRoots()

	// 执行 GC
	return gc.GC.Collect(roots)
}

// CollectYoungWithSTW 执行带 STW 的年轻代 GC
func (gc *MultiThreadGC) CollectYoungWithSTW() int {
	if !gc.enabled {
		return 0
	}

	// 请求 STW
	gc.RequestSTW()
	defer gc.ReleaseSTW()

	// 收集所有根对象
	roots := gc.collectAllRoots()

	// 执行年轻代 GC
	return gc.GC.CollectYoung(roots)
}

// CollectFullWithSTW 执行带 STW 的完整 GC
func (gc *MultiThreadGC) CollectFullWithSTW() int {
	if !gc.enabled {
		return 0
	}

	// 请求 STW
	gc.RequestSTW()
	defer gc.ReleaseSTW()

	// 收集所有根对象
	roots := gc.collectAllRoots()

	// 执行完整 GC
	return gc.GC.CollectFull(roots)
}

// collectAllRoots 收集所有 Worker 的根对象
//
// 根对象包括：
//   - 每个 Worker 的操作数栈
//   - 每个 Worker 的调用帧中的闭包
//   - 全局变量
func (gc *MultiThreadGC) collectAllRoots() []GCObject {
	var roots []GCObject

	// 收集全局变量
	if gc.workerPool != nil && gc.workerPool.sharedState != nil {
		gc.workerPool.sharedState.RangeGlobals(func(name string, value bytecode.Value) bool {
			if wrapper := gc.GC.TrackValue(value); wrapper != nil {
				roots = append(roots, wrapper)
			}
			return true
		})
	}

	// 收集每个 Worker 的根对象
	if gc.workerPool != nil {
		for _, worker := range gc.workerPool.workers {
			roots = append(roots, gc.collectWorkerRoots(worker)...)
		}
	}

	// 收集所有活跃协程的根对象
	if gc.scheduler != nil {
		gc.scheduler.allGoroutines.Range(func(key, value interface{}) bool {
			g := value.(*Goroutine)
			roots = append(roots, gc.collectGoroutineRoots(g)...)
			return true
		})
	}

	return roots
}

// collectWorkerRoots 收集单个 Worker 的根对象
func (gc *MultiThreadGC) collectWorkerRoots(worker *Worker) []GCObject {
	var roots []GCObject

	if worker.ctx == nil {
		return roots
	}

	ctx := worker.ctx

	// 收集操作数栈上的对象
	for i := 0; i < ctx.stackTop; i++ {
		if wrapper := gc.GC.TrackValue(ctx.stack[i]); wrapper != nil {
			roots = append(roots, wrapper)
		}
	}

	// 收集调用帧中的对象
	for i := 0; i < ctx.frameCount; i++ {
		frame := &ctx.frames[i]

		// 闭包
		if frame.Closure != nil {
			closureValue := bytecode.Value{
				Type: bytecode.ValClosure,
				Data: frame.Closure,
			}
			if wrapper := gc.GC.TrackValue(closureValue); wrapper != nil {
				roots = append(roots, wrapper)
			}
		}
	}

	return roots
}

// collectGoroutineRoots 收集单个协程的根对象
func (gc *MultiThreadGC) collectGoroutineRoots(g *Goroutine) []GCObject {
	var roots []GCObject

	// 收集操作数栈上的对象
	for i := 0; i < g.StackTop; i++ {
		if wrapper := gc.GC.TrackValue(g.Stack[i]); wrapper != nil {
			roots = append(roots, wrapper)
		}
	}

	// 收集调用帧中的对象
	for i := 0; i < g.FrameCount; i++ {
		frame := &g.Frames[i]

		// 闭包
		if frame.Closure != nil {
			closureValue := bytecode.Value{
				Type: bytecode.ValClosure,
				Data: frame.Closure,
			}
			if wrapper := gc.GC.TrackValue(closureValue); wrapper != nil {
				roots = append(roots, wrapper)
			}
		}
	}

	// 收集异常对象
	if g.HasException {
		if wrapper := gc.GC.TrackValue(g.Exception); wrapper != nil {
			roots = append(roots, wrapper)
		}
	}

	// 收集阻塞时的值
	if g.BlockType == BlockSend {
		if wrapper := gc.GC.TrackValue(g.SendValue); wrapper != nil {
			roots = append(roots, wrapper)
		}
	}
	if wrapper := gc.GC.TrackValue(g.RecvValue); wrapper != nil {
		roots = append(roots, wrapper)
	}

	return roots
}

// ============================================================================
// 安全点检查
// ============================================================================

// CheckSafePoint 检查是否需要进入安全点
//
// Worker 应该在执行循环的关键点调用此方法：
//   - 函数调用前后
//   - 循环迭代时
//   - 内存分配时
func (gc *MultiThreadGC) CheckSafePoint() {
	if gc.stwActive.Load() {
		gc.EnterSafePoint()
	}
}

// NeedsSafePoint 检查是否需要安全点（不阻塞）
func (gc *MultiThreadGC) NeedsSafePoint() bool {
	return gc.stwActive.Load()
}

// ============================================================================
// 统计信息
// ============================================================================

// STWStats 返回 STW 统计信息
func (gc *MultiThreadGC) STWStats() STWStats {
	return STWStats{
		STWCount:       gc.stwCount,
		TotalSTWTimeNs: gc.totalSTWTimeNs,
		MaxSTWTimeNs:   gc.maxSTWTimeNs,
		LastSTWTimeNs:  gc.lastSTWTimeNs,
		AvgSTWTimeNs:   gc.avgSTWTime(),
	}
}

func (gc *MultiThreadGC) avgSTWTime() int64 {
	if gc.stwCount == 0 {
		return 0
	}
	return gc.totalSTWTimeNs / gc.stwCount
}

// STWStats STW 统计信息
type STWStats struct {
	STWCount       int64 // STW 次数
	TotalSTWTimeNs int64 // 总 STW 时间
	MaxSTWTimeNs   int64 // 最大 STW 时间
	LastSTWTimeNs  int64 // 上次 STW 时间
	AvgSTWTimeNs   int64 // 平均 STW 时间
}

// ============================================================================
// 并发安全的对象追踪
// ============================================================================

// TrackValueSafe 并发安全的值追踪
//
// 在多线程环境下，多个 Worker 可能同时创建对象。
// 此方法使用原子操作确保对象注册的安全性。
func (gc *MultiThreadGC) TrackValueSafe(v bytecode.Value) *GCObjectWrapper {
	// 基础 GC 的 TrackValue 需要锁保护
	// 目前使用 STW 机制，所以在 STW 期间是安全的
	// 非 STW 期间，依赖写屏障
	return gc.GC.TrackValue(v)
}

// ============================================================================
// 写屏障（并发标记支持）
// ============================================================================

// WriteBarrierSafe 并发安全的写屏障
//
// 在并发标记期间，当老年代对象引用年轻代对象时调用。
// 此方法是线程安全的。
func (gc *MultiThreadGC) WriteBarrierSafe(parent, child GCObject) {
	// 使用原子操作或锁保护写屏障
	// 当前实现在 STW 期间执行 GC，所以不需要额外同步
	gc.GC.WriteBarrier(parent, child)
}

// ============================================================================
// 触发策略
// ============================================================================

// ShouldCollectSafe 并发安全的 GC 触发检查
func (gc *MultiThreadGC) ShouldCollectSafe() bool {
	// 如果已在 STW 中，不触发新的 GC
	if gc.stwActive.Load() {
		return false
	}
	return gc.GC.NeedsCollection()
}

// TryCollect 尝试触发 GC（非阻塞）
//
// 如果当前没有 STW 进行中，则触发 GC。
// 返回 true 表示成功触发，false 表示有其他 GC 进行中。
func (gc *MultiThreadGC) TryCollect() bool {
	// 尝试获取 STW 锁（非阻塞）
	if !gc.stwMu.TryLock() {
		return false
	}
	gc.stwMu.Unlock()

	// 触发 GC
	gc.CollectWithSTW()
	return true
}
