// Package vm 实现了 Sola 编程语言的字节码虚拟机。
//
// 本文件实现协程调度器，负责管理和调度所有协程的执行。
package vm

import (
	"sync"
	"sync/atomic"

	"github.com/tangzhangming/nova/internal/bytecode"
)

// ============================================================================
// 调度器配置
// ============================================================================

const (
	// DefaultTimeSlice 默认时间片（指令数）
	// 每个协程最多执行这么多条指令后让出 CPU
	DefaultTimeSlice = 1000

	// MaxGoroutines 最大协程数量
	MaxGoroutines = 10000
)

// ============================================================================
// 调度器结构
// ============================================================================

// Scheduler 协程调度器
//
// 调度器采用协作式调度模型：
//   - 协程在执行一定数量的指令后主动让出
//   - 协程在通道操作阻塞时让出
//   - 协程在执行 select 语句时可能让出
//
// 调度策略：
//   - 使用简单的 FIFO 队列
//   - 阻塞的协程从运行队列移除
//   - 被唤醒的协程加入运行队列尾部
type Scheduler struct {
	// =========================================================================
	// 协程管理
	// =========================================================================

	// runQueue 可运行协程队列
	runQueue []*Goroutine

	// current 当前正在执行的协程
	current *Goroutine

	// allGoroutines 所有协程（包括阻塞和死亡的）
	// key: 协程 ID
	allGoroutines map[int64]*Goroutine

	// mainGoroutine 主协程
	// 主协程结束意味着程序结束
	mainGoroutine *Goroutine

	// =========================================================================
	// ID 分配
	// =========================================================================

	// nextID 下一个协程 ID
	nextID int64

	// =========================================================================
	// 配置
	// =========================================================================

	// timeSlice 时间片大小
	timeSlice int

	// =========================================================================
	// 状态
	// =========================================================================

	// running 调度器是否运行中
	running bool

	// goroutineCount 当前协程数量（不含已死亡的）
	goroutineCount int32

	// =========================================================================
	// 对象池
	// =========================================================================

	// pool 协程对象池
	pool *GoroutinePool

	// =========================================================================
	// 同步
	// =========================================================================

	// mu 保护调度器状态
	mu sync.Mutex
}

// NewScheduler 创建调度器
func NewScheduler() *Scheduler {
	return &Scheduler{
		runQueue:      make([]*Goroutine, 0, 64),
		allGoroutines: make(map[int64]*Goroutine),
		nextID:        1,
		timeSlice:     DefaultTimeSlice,
		pool:          NewGoroutinePool(),
	}
}

// ============================================================================
// 协程创建和销毁
// ============================================================================

// Spawn 创建新协程
//
// 参数:
//   - closure: 要执行的闭包
//
// 返回:
//   - 新创建的协程
//   - 如果协程数量超限，返回 nil
func (s *Scheduler) Spawn(closure *bytecode.Closure) *Goroutine {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 检查协程数量限制
	if atomic.LoadInt32(&s.goroutineCount) >= MaxGoroutines {
		return nil
	}

	// 分配 ID
	id := atomic.AddInt64(&s.nextID, 1) - 1

	// 获取父协程 ID
	var parentID int64 = 0
	if s.current != nil {
		parentID = s.current.ID
	}

	// 创建协程
	g := s.pool.Get(id, parentID)

	// 设置初始调用帧
	frame := CallFrame{
		Closure:  closure,
		IP:       0,
		BaseSlot: 0,
	}
	g.PushFrame(frame)

	// 加入调度
	s.allGoroutines[id] = g
	s.runQueue = append(s.runQueue, g)
	atomic.AddInt32(&s.goroutineCount, 1)

	return g
}

// SpawnMain 创建主协程
func (s *Scheduler) SpawnMain(closure *bytecode.Closure) *Goroutine {
	g := s.Spawn(closure)
	if g != nil {
		s.mainGoroutine = g
	}
	return g
}

// Kill 终止协程
func (s *Scheduler) Kill(g *Goroutine) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if g.GetStatus() == GoroutineDead {
		return
	}

	g.SetStatus(GoroutineDead)
	atomic.AddInt32(&s.goroutineCount, -1)

	// 从运行队列移除
	s.removeFromRunQueue(g)

	// 从阻塞的通道中移除
	if g.BlockedOn != nil {
		g.BlockedOn.RemoveWaiter(g)
		g.BlockedOn = nil
	}

	// 归还到池中
	s.pool.Put(g)
}

// ============================================================================
// 调度核心
// ============================================================================

// Schedule 选择下一个要执行的协程
//
// 调度策略：FIFO
// 返回 nil 表示没有可运行的协程
func (s *Scheduler) Schedule() *Goroutine {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.runQueue) == 0 {
		return nil
	}

	// 取出队首
	g := s.runQueue[0]
	s.runQueue = s.runQueue[1:]

	g.SetStatus(GoroutineRunning)
	s.current = g

	return g
}

// Yield 当前协程让出执行权
func (s *Scheduler) Yield() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.current == nil {
		return
	}

	g := s.current
	if g.GetStatus() == GoroutineRunning {
		g.SetStatus(GoroutineRunnable)
		s.runQueue = append(s.runQueue, g)
	}
	s.current = nil
}

// Block 阻塞当前协程
//
// 参数:
//   - ch: 阻塞在哪个通道
//   - blockType: 阻塞类型（发送/接收）
func (s *Scheduler) Block(g *Goroutine, ch *Channel, blockType BlockType) {
	s.mu.Lock()
	defer s.mu.Unlock()

	g.SetStatus(GoroutineBlocked)
	g.BlockedOn = ch
	g.BlockType = blockType

	// 从运行队列移除（应该已经不在队列中了）
	s.removeFromRunQueue(g)

	if s.current == g {
		s.current = nil
	}
}

// Unblock 唤醒阻塞的协程
func (s *Scheduler) Unblock(g *Goroutine) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if g.GetStatus() != GoroutineBlocked && g.GetStatus() != GoroutineWaiting {
		return
	}

	g.SetStatus(GoroutineRunnable)
	g.BlockedOn = nil
	g.BlockType = BlockNone

	s.runQueue = append(s.runQueue, g)
}

// ============================================================================
// Select 支持
// ============================================================================

// BlockOnSelect 在 select 语句上阻塞
func (s *Scheduler) BlockOnSelect(g *Goroutine, cases []SelectCaseInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()

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

	s.removeFromRunQueue(g)
	if s.current == g {
		s.current = nil
	}
}

// WakeupFromSelect 从 select 等待中唤醒
func (s *Scheduler) WakeupFromSelect(g *Goroutine, caseIndex int) {
	s.mu.Lock()
	defer s.mu.Unlock()

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

	s.runQueue = append(s.runQueue, g)
}

// ============================================================================
// 辅助方法
// ============================================================================

// removeFromRunQueue 从运行队列中移除协程
func (s *Scheduler) removeFromRunQueue(g *Goroutine) {
	for i, item := range s.runQueue {
		if item == g {
			s.runQueue = append(s.runQueue[:i], s.runQueue[i+1:]...)
			return
		}
	}
}

// Current 获取当前协程
func (s *Scheduler) Current() *Goroutine {
	return s.current
}

// SetCurrent 设置当前协程
func (s *Scheduler) SetCurrent(g *Goroutine) {
	s.current = g
}

// MainGoroutine 获取主协程
func (s *Scheduler) MainGoroutine() *Goroutine {
	return s.mainGoroutine
}

// GoroutineCount 获取协程数量
func (s *Scheduler) GoroutineCount() int32 {
	return atomic.LoadInt32(&s.goroutineCount)
}

// HasRunnable 是否有可运行的协程
func (s *Scheduler) HasRunnable() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.runQueue) > 0
}

// IsMainDead 主协程是否已终止
func (s *Scheduler) IsMainDead() bool {
	if s.mainGoroutine == nil {
		return true
	}
	return s.mainGoroutine.IsDead()
}

// AllDead 是否所有协程都已终止
func (s *Scheduler) AllDead() bool {
	return atomic.LoadInt32(&s.goroutineCount) == 0
}

// GetTimeSlice 获取时间片大小
func (s *Scheduler) GetTimeSlice() int {
	return s.timeSlice
}

// SetTimeSlice 设置时间片大小
func (s *Scheduler) SetTimeSlice(slice int) {
	if slice > 0 {
		s.timeSlice = slice
	}
}

// GetGoroutine 根据 ID 获取协程
func (s *Scheduler) GetGoroutine(id int64) *Goroutine {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.allGoroutines[id]
}

// NextID 获取下一个协程 ID（不创建协程）
func (s *Scheduler) NextID() int64 {
	return atomic.AddInt64(&s.nextID, 1) - 1
}

// ============================================================================
// 调试支持
// ============================================================================

// DumpState 输出调度器状态（用于调试）
func (s *Scheduler) DumpState() map[string]interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()

	var currentID int64 = -1
	if s.current != nil {
		currentID = s.current.ID
	}

	runQueueIDs := make([]int64, len(s.runQueue))
	for i, g := range s.runQueue {
		runQueueIDs[i] = g.ID
	}

	return map[string]interface{}{
		"current":         currentID,
		"runQueue":        runQueueIDs,
		"goroutineCount":  s.goroutineCount,
		"totalGoroutines": len(s.allGoroutines),
	}
}
