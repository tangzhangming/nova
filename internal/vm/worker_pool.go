// Package vm 实现了 Sola 编程语言的字节码虚拟机。
//
// 本文件实现多线程 VM 的工作线程池和工作窃取调度算法。
package vm

import (
	"runtime"
	"sync"
	"sync/atomic"
)

// ============================================================================
// 工作线程池配置
// ============================================================================
//
// BUG FIX 2026-01-10: 多线程 VM - 工作线程池
// 防止反复引入的问题:
// 1. 工作窃取时必须从队列尾部窃取（避免与 owner 冲突）
// 2. 协程状态转换必须原子（Running->Blocked 等）
// 3. 协程迁移时必须保证 VM 上下文完整复制
// 4. Channel 阻塞/唤醒必须与调度器同步（避免协程丢失）

const (
	// DefaultWorkerCount 默认工作线程数（等于 CPU 核心数）
	DefaultWorkerCount = 0 // 0 表示自动检测

	// LocalQueueSize 每个工作线程的本地队列大小
	LocalQueueSize = 256

	// StealBatchSize 每次窃取的协程数量
	StealBatchSize = 1
)

// ============================================================================
// 工作线程池
// ============================================================================

// WorkerPool 工作线程池
//
// 工作线程池管理多个工作线程（Worker），每个线程独立执行协程。
// 采用工作窃取（Work Stealing）算法实现负载均衡：
//   - 每个 Worker 有自己的本地队列
//   - 本地队列为空时，从其他 Worker 窃取任务
//   - 全局队列用于新创建的协程
type WorkerPool struct {
	// =========================================================================
	// 工作线程管理
	// =========================================================================

	// workers 所有工作线程
	workers []*Worker

	// numWorkers 工作线程数量
	numWorkers int

	// =========================================================================
	// 调度器引用
	// =========================================================================

	// scheduler 协程调度器
	scheduler *MultiThreadScheduler

	// =========================================================================
	// 共享状态
	// =========================================================================

	// sharedState 多线程共享的状态
	sharedState *SharedState

	// vm 主 VM 实例（用于获取配置和共享资源）
	vm *VM

	// =========================================================================
	// 生命周期控制
	// =========================================================================

	// running 线程池是否运行中
	running atomic.Bool

	// wg 等待所有工作线程结束
	wg sync.WaitGroup

	// stopCh 停止信号
	stopCh chan struct{}

	// =========================================================================
	// 统计信息
	// =========================================================================

	// stats 性能统计
	stats WorkerPoolStats
}

// WorkerPoolStats 工作线程池统计信息
type WorkerPoolStats struct {
	// TotalGoroutinesExecuted 总执行协程数
	TotalGoroutinesExecuted int64

	// TotalSteals 总窃取次数
	TotalSteals int64

	// TotalStealFailures 窃取失败次数
	TotalStealFailures int64
}

// NewWorkerPool 创建工作线程池
//
// 参数:
//   - vm: 主 VM 实例
//   - numWorkers: 工作线程数量（0 表示自动检测 CPU 核心数）
func NewWorkerPool(vm *VM, numWorkers int) *WorkerPool {
	if numWorkers <= 0 {
		numWorkers = runtime.NumCPU()
	}

	// 限制最大线程数
	if numWorkers > 256 {
		numWorkers = 256
	}

	pool := &WorkerPool{
		numWorkers:  numWorkers,
		workers:     make([]*Worker, numWorkers),
		sharedState: NewSharedState(),
		vm:          vm,
		stopCh:      make(chan struct{}),
	}

	// 创建多线程调度器
	pool.scheduler = NewMultiThreadScheduler(pool)

	// 创建工作线程
	for i := 0; i < numWorkers; i++ {
		pool.workers[i] = NewWorker(i, pool)
	}

	return pool
}

// Start 启动工作线程池
//
// 调用此方法后，所有工作线程开始运行并等待任务。
// 在调用此方法前，应该完成所有类和枚举的注册。
func (p *WorkerPool) Start() {
	if p.running.Load() {
		return
	}

	// 冻结共享状态，防止运行时修改
	p.sharedState.Freeze()

	p.running.Store(true)

	// 启动所有工作线程
	for _, w := range p.workers {
		p.wg.Add(1)
		go w.Run()
	}
}

// Stop 停止工作线程池
//
// 等待所有工作线程完成当前任务后停止。
func (p *WorkerPool) Stop() {
	if !p.running.Load() {
		return
	}

	p.running.Store(false)
	close(p.stopCh)

	// 唤醒所有可能在等待的工作线程
	for _, w := range p.workers {
		w.Wake()
	}

	// 等待所有工作线程结束
	p.wg.Wait()
}

// Submit 提交协程到线程池执行
//
// 参数:
//   - g: 要执行的协程
func (p *WorkerPool) Submit(g *Goroutine) {
	p.scheduler.Submit(g)
}

// GetWorker 获取指定索引的工作线程
func (p *WorkerPool) GetWorker(index int) *Worker {
	if index < 0 || index >= len(p.workers) {
		return nil
	}
	return p.workers[index]
}

// NumWorkers 获取工作线程数量
func (p *WorkerPool) NumWorkers() int {
	return p.numWorkers
}

// IsRunning 检查线程池是否运行中
func (p *WorkerPool) IsRunning() bool {
	return p.running.Load()
}

// GetSharedState 获取共享状态
func (p *WorkerPool) GetSharedState() *SharedState {
	return p.sharedState
}

// GetScheduler 获取调度器
func (p *WorkerPool) GetScheduler() *MultiThreadScheduler {
	return p.scheduler
}

// Stats 获取统计信息
func (p *WorkerPool) Stats() WorkerPoolStats {
	return WorkerPoolStats{
		TotalGoroutinesExecuted: atomic.LoadInt64(&p.stats.TotalGoroutinesExecuted),
		TotalSteals:             atomic.LoadInt64(&p.stats.TotalSteals),
		TotalStealFailures:      atomic.LoadInt64(&p.stats.TotalStealFailures),
	}
}

// ============================================================================
// 工作线程
// ============================================================================

// Worker 工作线程
//
// 每个工作线程拥有：
//   - 独立的 VMContext（执行上下文）
//   - 本地协程队列（双端队列）
//   - 窃取能力（从其他 Worker 获取任务）
type Worker struct {
	// =========================================================================
	// 标识
	// =========================================================================

	// id 工作线程 ID
	id int

	// pool 所属的工作线程池
	pool *WorkerPool

	// =========================================================================
	// 执行上下文
	// =========================================================================

	// ctx 执行上下文（独立的栈和帧）
	ctx *VMContext

	// =========================================================================
	// 本地队列（双端队列，支持工作窃取）
	// =========================================================================

	// localQueue 本地协程队列
	// 使用环形缓冲区实现
	localQueue [LocalQueueSize]*Goroutine

	// head 队列头部（Worker 自己从这里取）
	head atomic.Uint32

	// tail 队列尾部（新任务从这里加入，其他 Worker 从这里窃取）
	tail atomic.Uint32

	// =========================================================================
	// 状态
	// =========================================================================

	// running 是否运行中
	running atomic.Bool

	// parking 是否处于休眠状态
	parking atomic.Bool

	// =========================================================================
	// 同步
	// =========================================================================

	// wakeCh 唤醒通道
	wakeCh chan struct{}

	// =========================================================================
	// 统计
	// =========================================================================

	// executedCount 执行的协程数
	executedCount int64

	// stealCount 窃取成功次数
	stealCount int64

	// stealFailCount 窃取失败次数
	stealFailCount int64
}

// NewWorker 创建工作线程
func NewWorker(id int, pool *WorkerPool) *Worker {
	w := &Worker{
		id:     id,
		pool:   pool,
		wakeCh: make(chan struct{}, 1),
	}

	// 创建执行上下文
	w.ctx = NewVMContext(pool.vm)

	return w
}

// Run 工作线程主循环
//
// 此方法在独立的 goroutine 中运行，不断执行以下循环：
//  1. 从本地队列获取协程
//  2. 如果本地队列为空，尝试从全局队列获取
//  3. 如果全局队列为空，尝试从其他 Worker 窃取
//  4. 如果没有任务，进入休眠等待唤醒
func (w *Worker) Run() {
	defer w.pool.wg.Done()

	w.running.Store(true)

	for w.pool.running.Load() {
		// 尝试获取协程执行
		g := w.findWork()

		if g != nil {
			// 执行协程
			w.executeGoroutine(g)
			atomic.AddInt64(&w.executedCount, 1)
			atomic.AddInt64(&w.pool.stats.TotalGoroutinesExecuted, 1)
		} else {
			// 没有工作，进入休眠
			w.park()
		}
	}

	w.running.Store(false)
}

// findWork 查找要执行的协程
//
// 按以下顺序查找：
//  1. 本地队列
//  2. 全局队列
//  3. 从其他 Worker 窃取
func (w *Worker) findWork() *Goroutine {
	// 1. 先从本地队列取
	if g := w.popLocal(); g != nil {
		return g
	}

	// 2. 尝试从全局队列获取
	if g := w.pool.scheduler.PopGlobal(); g != nil {
		return g
	}

	// 3. 尝试从其他 Worker 窃取
	return w.steal()
}

// executeGoroutine 执行协程
func (w *Worker) executeGoroutine(g *Goroutine) {
	// 设置当前协程
	w.ctx.currentGoroutine = g
	g.SetStatus(GoroutineRunning)

	// 执行协程（使用 VMContext）
	// TODO: 实际的字节码执行逻辑
	// 这里需要与 VM.Run 的核心执行循环集成

	// 暂时使用简化版本
	w.runGoroutineSlice(g)

	// 清理
	w.ctx.currentGoroutine = nil
}

// runGoroutineSlice 执行协程的一个时间片
//
// 协程在以下情况下让出控制权：
//   - 执行完毕
//   - 达到时间片限制
//   - 遇到阻塞操作（channel、sleep 等）
func (w *Worker) runGoroutineSlice(g *Goroutine) {
	// TODO: 集成 VM 的核心执行循环
	// 当前为占位实现

	// 检查协程是否已完成
	if g.FrameCount == 0 {
		g.SetStatus(GoroutineDead)
		return
	}

	// 执行时间片（指令数限制）
	// 实际实现需要在 VM.runLoop 中添加中断点
}

// ============================================================================
// 本地队列操作（Lock-Free）
// ============================================================================

// pushLocal 将协程添加到本地队列头部
//
// 此操作只由 Worker 自己调用，无需加锁。
func (w *Worker) pushLocal(g *Goroutine) bool {
	head := w.head.Load()
	tail := w.tail.Load()

	// 检查队列是否已满
	if head-tail >= LocalQueueSize {
		return false
	}

	w.localQueue[head%LocalQueueSize] = g
	w.head.Store(head + 1)
	return true
}

// popLocal 从本地队列头部取出协程
//
// 此操作只由 Worker 自己调用。
func (w *Worker) popLocal() *Goroutine {
	head := w.head.Load()
	tail := w.tail.Load()

	// 检查队列是否为空
	if head == tail {
		return nil
	}

	newHead := head - 1
	g := w.localQueue[newHead%LocalQueueSize]

	// CAS 更新 head
	if w.head.CompareAndSwap(head, newHead) {
		return g
	}

	// CAS 失败（可能被窃取），重试
	return w.popLocal()
}

// stealFrom 从指定 Worker 窃取协程
//
// 从目标 Worker 的队列尾部窃取（避免与 owner 冲突）。
func (w *Worker) stealFrom(victim *Worker) *Goroutine {
	tail := victim.tail.Load()
	head := victim.head.Load()

	// 检查是否有可窃取的任务
	if head <= tail {
		return nil
	}

	// 从尾部窃取
	g := victim.localQueue[tail%LocalQueueSize]

	// CAS 更新 tail
	if victim.tail.CompareAndSwap(tail, tail+1) {
		atomic.AddInt64(&w.stealCount, 1)
		atomic.AddInt64(&w.pool.stats.TotalSteals, 1)
		return g
	}

	// CAS 失败，窃取失败
	atomic.AddInt64(&w.stealFailCount, 1)
	atomic.AddInt64(&w.pool.stats.TotalStealFailures, 1)
	return nil
}

// steal 尝试从其他 Worker 窃取任务
func (w *Worker) steal() *Goroutine {
	numWorkers := len(w.pool.workers)
	if numWorkers <= 1 {
		return nil
	}

	// 随机选择起始点，避免总是从同一个 Worker 窃取
	start := int(w.executedCount) % numWorkers

	for i := 0; i < numWorkers; i++ {
		idx := (start + i) % numWorkers
		if idx == w.id {
			continue // 跳过自己
		}

		victim := w.pool.workers[idx]
		if g := w.stealFrom(victim); g != nil {
			return g
		}
	}

	return nil
}

// ============================================================================
// 休眠和唤醒
// ============================================================================

// park 休眠等待任务
func (w *Worker) park() {
	w.parking.Store(true)

	select {
	case <-w.wakeCh:
		// 被唤醒
	case <-w.pool.stopCh:
		// 线程池停止
	}

	w.parking.Store(false)
}

// Wake 唤醒工作线程
func (w *Worker) Wake() {
	if w.parking.Load() {
		select {
		case w.wakeCh <- struct{}{}:
		default:
			// 已经有唤醒信号
		}
	}
}

// ============================================================================
// 任务提交
// ============================================================================

// Submit 向此 Worker 提交协程
//
// 先尝试放入本地队列，失败则放入全局队列。
func (w *Worker) Submit(g *Goroutine) {
	if w.pushLocal(g) {
		return
	}

	// 本地队列满，放入全局队列
	w.pool.scheduler.PushGlobal(g)
}

// ============================================================================
// 状态查询
// ============================================================================

// ID 获取工作线程 ID
func (w *Worker) ID() int {
	return w.id
}

// IsRunning 检查是否运行中
func (w *Worker) IsRunning() bool {
	return w.running.Load()
}

// IsParking 检查是否休眠中
func (w *Worker) IsParking() bool {
	return w.parking.Load()
}

// LocalQueueSize 获取本地队列中的任务数
func (w *Worker) LocalQueueLen() int {
	head := w.head.Load()
	tail := w.tail.Load()
	if head >= tail {
		return int(head - tail)
	}
	return 0
}

// Stats 获取统计信息
func (w *Worker) Stats() WorkerStats {
	return WorkerStats{
		ID:             w.id,
		ExecutedCount:  atomic.LoadInt64(&w.executedCount),
		StealCount:     atomic.LoadInt64(&w.stealCount),
		StealFailCount: atomic.LoadInt64(&w.stealFailCount),
		LocalQueueLen:  w.LocalQueueLen(),
		IsParking:      w.parking.Load(),
	}
}

// WorkerStats 工作线程统计信息
type WorkerStats struct {
	ID             int   // 线程 ID
	ExecutedCount  int64 // 执行协程数
	StealCount     int64 // 窃取成功次数
	StealFailCount int64 // 窃取失败次数
	LocalQueueLen  int   // 本地队列长度
	IsParking      bool  // 是否休眠
}
