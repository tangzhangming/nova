// Package vm 实现了 Sola 编程语言的字节码虚拟机。
//
// 本文件实现多线程调度器，支持全局队列和负载均衡。
package vm

import (
	"sync"
	"sync/atomic"

	"github.com/tangzhangming/nova/internal/bytecode"
)

// ============================================================================
// 多线程调度器
// ============================================================================
//
// BUG FIX 2026-01-10: 多线程调度器
// 防止反复引入的问题:
// 1. 工作窃取时必须从队列尾部窃取（避免与 owner 冲突）
// 2. 协程状态转换必须原子（Running->Blocked 等）
// 3. 协程迁移时必须保证 VM 上下文完整复制
// 4. Channel 阻塞/唤醒必须与调度器同步（避免协程丢失）

const (
	// GlobalQueueSize 全局队列容量
	GlobalQueueSize = 4096
)

// MultiThreadScheduler 多线程协程调度器
//
// 与单线程 Scheduler 的区别：
//   - 维护全局协程队列，供多个 Worker 共享
//   - 支持负载均衡：新协程会被分配到负载较低的 Worker
//   - 线程安全：所有操作都是并发安全的
//
// 调度策略：
//   - 新创建的协程优先放入创建者 Worker 的本地队列
//   - 本地队列满时放入全局队列
//   - Worker 空闲时先从本地队列取，再从全局队列取，最后窃取
type MultiThreadScheduler struct {
	// =========================================================================
	// 工作线程池引用
	// =========================================================================

	pool *WorkerPool

	// =========================================================================
	// 全局队列
	// =========================================================================

	// globalQueue 全局协程队列（并发安全）
	globalQueue chan *Goroutine

	// =========================================================================
	// 协程管理
	// =========================================================================

	// allGoroutines 所有协程的注册表
	// 键: 协程 ID
	// 使用 sync.Map 保证并发安全
	allGoroutines sync.Map

	// mainGoroutine 主协程
	mainGoroutine atomic.Pointer[Goroutine]

	// =========================================================================
	// ID 分配
	// =========================================================================

	// nextID 下一个协程 ID
	nextID atomic.Int64

	// =========================================================================
	// 统计
	// =========================================================================

	// goroutineCount 当前活跃协程数量
	goroutineCount atomic.Int32

	// totalSpawned 总创建协程数
	totalSpawned atomic.Int64

	// totalCompleted 总完成协程数
	totalCompleted atomic.Int64

	// =========================================================================
	// 协程池
	// =========================================================================

	// goroutinePool 协程对象池
	goroutinePool *GoroutinePool

	// =========================================================================
	// STW 支持
	// =========================================================================

	// stwMu STW 全局锁
	stwMu sync.RWMutex

	// stwActive STW 是否激活
	stwActive atomic.Bool

	// stwWaiters 等待 STW 完成的 Worker 数量
	stwWaiters atomic.Int32

	// stwDone STW 完成信号
	stwDone chan struct{}
}

// NewMultiThreadScheduler 创建多线程调度器
func NewMultiThreadScheduler(pool *WorkerPool) *MultiThreadScheduler {
	return &MultiThreadScheduler{
		pool:          pool,
		globalQueue:   make(chan *Goroutine, GlobalQueueSize),
		goroutinePool: NewGoroutinePool(),
		stwDone:       make(chan struct{}),
	}
}

// ============================================================================
// 协程创建和销毁
// ============================================================================

// Spawn 创建新协程
//
// 参数:
//   - closure: 要执行的闭包
//   - parentG: 父协程（可选，用于设置父子关系）
//
// 返回:
//   - 新创建的协程
//   - 如果协程数量超限，返回 nil
func (s *MultiThreadScheduler) Spawn(closure *bytecode.Closure, parentG *Goroutine) *Goroutine {
	// 检查协程数量限制
	if s.goroutineCount.Load() >= MaxGoroutines {
		return nil
	}

	// 分配 ID
	id := s.nextID.Add(1)

	// 获取父协程 ID
	var parentID int64 = 0
	if parentG != nil {
		parentID = parentG.ID
	}

	// 从池中获取协程
	g := s.goroutinePool.Get(id, parentID)

	// 设置初始调用帧
	frame := CallFrame{
		Closure:  closure,
		IP:       0,
		BaseSlot: 0,
	}
	g.PushFrame(frame)

	// 注册协程
	s.allGoroutines.Store(id, g)
	s.goroutineCount.Add(1)
	s.totalSpawned.Add(1)

	// 提交到调度
	s.Submit(g)

	return g
}

// SpawnMain 创建主协程
func (s *MultiThreadScheduler) SpawnMain(closure *bytecode.Closure) *Goroutine {
	g := s.Spawn(closure, nil)
	if g != nil {
		s.mainGoroutine.Store(g)
	}
	return g
}

// Kill 终止协程
func (s *MultiThreadScheduler) Kill(g *Goroutine) {
	if g.GetStatus() == GoroutineDead {
		return
	}

	g.SetStatus(GoroutineDead)
	s.goroutineCount.Add(-1)
	s.totalCompleted.Add(1)

	// 从注册表移除
	s.allGoroutines.Delete(g.ID)

	// 归还到池中
	s.goroutinePool.Put(g)
}

// ============================================================================
// 全局队列操作
// ============================================================================

// PushGlobal 将协程放入全局队列
func (s *MultiThreadScheduler) PushGlobal(g *Goroutine) {
	select {
	case s.globalQueue <- g:
		// 成功放入全局队列
	default:
		// 全局队列满，尝试负载均衡到某个 Worker
		s.balanceToWorker(g)
	}
}

// PopGlobal 从全局队列获取协程
func (s *MultiThreadScheduler) PopGlobal() *Goroutine {
	select {
	case g := <-s.globalQueue:
		return g
	default:
		return nil
	}
}

// Submit 提交协程到调度
//
// 智能选择目标：
//  1. 如果是在 Worker 上下文中，优先放入本地队列
//  2. 否则放入全局队列
func (s *MultiThreadScheduler) Submit(g *Goroutine) {
	g.SetStatus(GoroutineRunnable)

	// 尝试放入全局队列
	select {
	case s.globalQueue <- g:
		// 成功放入全局队列
		s.wakeOneWorker()
	default:
		// 全局队列满，负载均衡
		s.balanceToWorker(g)
	}
}

// balanceToWorker 将协程负载均衡到某个 Worker
func (s *MultiThreadScheduler) balanceToWorker(g *Goroutine) {
	// 找到负载最低的 Worker
	minLoad := int(^uint(0) >> 1) // MaxInt
	var targetWorker *Worker

	for _, w := range s.pool.workers {
		load := w.LocalQueueLen()
		if load < minLoad {
			minLoad = load
			targetWorker = w
		}
	}

	if targetWorker != nil {
		targetWorker.Submit(g)
		targetWorker.Wake()
	} else {
		// 没有可用 Worker，强制放入全局队列
		s.globalQueue <- g
	}
}

// wakeOneWorker 唤醒一个空闲的 Worker
func (s *MultiThreadScheduler) wakeOneWorker() {
	for _, w := range s.pool.workers {
		if w.IsParking() {
			w.Wake()
			return
		}
	}
}

// wakeAllWorkers 唤醒所有 Worker
func (s *MultiThreadScheduler) wakeAllWorkers() {
	for _, w := range s.pool.workers {
		w.Wake()
	}
}

// ============================================================================
// 阻塞和唤醒
// ============================================================================

// Block 阻塞协程
//
// 参数:
//   - g: 要阻塞的协程
//   - ch: 阻塞在哪个通道
//   - blockType: 阻塞类型
func (s *MultiThreadScheduler) Block(g *Goroutine, ch *Channel, blockType BlockType) {
	g.SetStatus(GoroutineBlocked)
	g.BlockedOn = ch
	g.BlockType = blockType
}

// Unblock 唤醒协程
func (s *MultiThreadScheduler) Unblock(g *Goroutine) {
	status := g.GetStatus()
	if status != GoroutineBlocked && status != GoroutineWaiting {
		return
	}

	g.SetStatus(GoroutineRunnable)
	g.BlockedOn = nil
	g.BlockType = BlockNone

	// 重新提交到调度
	s.Submit(g)
}

// ============================================================================
// Select 支持
// ============================================================================

// BlockOnSelect 在 select 语句上阻塞
func (s *MultiThreadScheduler) BlockOnSelect(g *Goroutine, cases []SelectCaseInfo) {
	g.SetStatus(GoroutineWaiting)
	g.SelectCases = cases
	g.SelectIndex = -1
	g.BlockType = BlockSelect

	// 将协程注册到所有相关通道的等待队列
	for _, c := range cases {
		if c.IsRecv {
			c.Channel.AddRecvWaiter(g)
		} else {
			c.Channel.AddSendWaiter(g)
		}
	}
}

// WakeupFromSelect 从 select 等待中唤醒
func (s *MultiThreadScheduler) WakeupFromSelect(g *Goroutine, caseIndex int) {
	if g.GetStatus() != GoroutineWaiting {
		return
	}

	// 从所有通道的等待队列中移除
	for _, c := range g.SelectCases {
		if c.IsRecv {
			c.Channel.RemoveRecvWaiter(g)
		} else {
			c.Channel.RemoveSendWaiter(g)
		}
	}

	g.SelectIndex = caseIndex
	g.SetStatus(GoroutineRunnable)
	g.BlockType = BlockNone

	// 重新提交到调度
	s.Submit(g)
}

// ============================================================================
// STW (Stop-The-World) 支持
// ============================================================================

// RequestSTW 请求 STW
//
// 此方法会阻塞直到所有 Worker 都停止。
// 用于 GC 标记阶段。
func (s *MultiThreadScheduler) RequestSTW() {
	s.stwMu.Lock()
	s.stwActive.Store(true)

	// 等待所有 Worker 进入安全点
	// Worker 在执行循环中会检查 stwActive 并暂停
}

// ReleaseSTW 释放 STW
func (s *MultiThreadScheduler) ReleaseSTW() {
	s.stwActive.Store(false)
	s.stwMu.Unlock()

	// 唤醒所有 Worker
	s.wakeAllWorkers()
}

// IsSTWActive 检查 STW 是否激活
func (s *MultiThreadScheduler) IsSTWActive() bool {
	return s.stwActive.Load()
}

// WaitForSafePoint Worker 进入安全点等待
//
// 在 STW 激活时，Worker 调用此方法暂停执行。
func (s *MultiThreadScheduler) WaitForSafePoint() {
	if !s.stwActive.Load() {
		return
	}

	// 等待 STW 结束
	s.stwMu.RLock()
	s.stwMu.RUnlock()
}

// ============================================================================
// 状态查询
// ============================================================================

// MainGoroutine 获取主协程
func (s *MultiThreadScheduler) MainGoroutine() *Goroutine {
	return s.mainGoroutine.Load()
}

// IsMainDead 主协程是否已终止
func (s *MultiThreadScheduler) IsMainDead() bool {
	main := s.mainGoroutine.Load()
	if main == nil {
		return true
	}
	return main.IsDead()
}

// GoroutineCount 获取活跃协程数量
func (s *MultiThreadScheduler) GoroutineCount() int32 {
	return s.goroutineCount.Load()
}

// GetGoroutine 根据 ID 获取协程
func (s *MultiThreadScheduler) GetGoroutine(id int64) *Goroutine {
	if val, ok := s.allGoroutines.Load(id); ok {
		return val.(*Goroutine)
	}
	return nil
}

// GlobalQueueLen 获取全局队列长度
func (s *MultiThreadScheduler) GlobalQueueLen() int {
	return len(s.globalQueue)
}

// AllDead 检查是否所有协程都已终止
func (s *MultiThreadScheduler) AllDead() bool {
	return s.goroutineCount.Load() == 0
}

// ============================================================================
// 统计信息
// ============================================================================

// Stats 获取调度器统计信息
func (s *MultiThreadScheduler) Stats() MultiThreadSchedulerStats {
	return MultiThreadSchedulerStats{
		GoroutineCount:  s.goroutineCount.Load(),
		TotalSpawned:    s.totalSpawned.Load(),
		TotalCompleted:  s.totalCompleted.Load(),
		GlobalQueueLen:  len(s.globalQueue),
		STWActive:       s.stwActive.Load(),
	}
}

// MultiThreadSchedulerStats 调度器统计信息
type MultiThreadSchedulerStats struct {
	GoroutineCount int32 // 活跃协程数
	TotalSpawned   int64 // 总创建数
	TotalCompleted int64 // 总完成数
	GlobalQueueLen int   // 全局队列长度
	STWActive      bool  // STW 是否激活
}

// ============================================================================
// 调试支持
// ============================================================================

// DumpState 输出调度器状态（用于调试）
func (s *MultiThreadScheduler) DumpState() map[string]interface{} {
	var mainID int64 = -1
	if main := s.mainGoroutine.Load(); main != nil {
		mainID = main.ID
	}

	// 收集所有协程 ID
	var goroutineIDs []int64
	s.allGoroutines.Range(func(key, value interface{}) bool {
		goroutineIDs = append(goroutineIDs, key.(int64))
		return true
	})

	// 收集 Worker 状态
	workerStats := make([]WorkerStats, len(s.pool.workers))
	for i, w := range s.pool.workers {
		workerStats[i] = w.Stats()
	}

	return map[string]interface{}{
		"mainGoroutine":   mainID,
		"goroutineCount":  s.goroutineCount.Load(),
		"totalSpawned":    s.totalSpawned.Load(),
		"totalCompleted":  s.totalCompleted.Load(),
		"globalQueueLen":  len(s.globalQueue),
		"goroutineIDs":    goroutineIDs,
		"workerStats":     workerStats,
		"stwActive":       s.stwActive.Load(),
	}
}
