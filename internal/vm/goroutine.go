// Package vm 实现了 Sola 编程语言的字节码虚拟机。
//
// 本文件实现协程（Goroutine）数据结构，用于支持 Sola 的并发编程模型。
package vm

import (
	"sync/atomic"

	"github.com/tangzhangming/nova/internal/bytecode"
)

// ============================================================================
// 协程状态
// ============================================================================

// GoroutineStatus 协程状态
type GoroutineStatus int32

const (
	// GoroutineRunnable 可运行状态
	// 协程已准备好执行，在调度器的运行队列中等待
	GoroutineRunnable GoroutineStatus = iota

	// GoroutineRunning 运行中状态
	// 协程正在被调度器执行
	GoroutineRunning

	// GoroutineBlocked 阻塞状态
	// 协程因通道操作（发送/接收）而阻塞
	GoroutineBlocked

	// GoroutineWaiting 等待状态
	// 协程在 select 语句中等待多个通道
	GoroutineWaiting

	// GoroutineDead 死亡状态
	// 协程执行完毕或因异常终止
	GoroutineDead
)

// String 返回状态的字符串表示
func (s GoroutineStatus) String() string {
	switch s {
	case GoroutineRunnable:
		return "runnable"
	case GoroutineRunning:
		return "running"
	case GoroutineBlocked:
		return "blocked"
	case GoroutineWaiting:
		return "waiting"
	case GoroutineDead:
		return "dead"
	default:
		return "unknown"
	}
}

// ============================================================================
// 阻塞类型
// ============================================================================

// BlockType 阻塞类型
type BlockType int

const (
	// BlockNone 未阻塞
	BlockNone BlockType = iota

	// BlockSend 因发送操作阻塞
	BlockSend

	// BlockRecv 因接收操作阻塞
	BlockRecv

	// BlockSelect 因 select 操作阻塞
	BlockSelect
)

// ============================================================================
// 协程结构
// ============================================================================

// Goroutine 表示一个 Sola 协程
//
// 每个协程拥有独立的执行上下文：
//   - 操作数栈：存储计算中间结果
//   - 调用栈：管理函数调用链
//   - 程序计数器：当前执行位置
//
// 协程之间通过通道（Channel）进行通信，遵循 CSP 模型。
type Goroutine struct {
	// =========================================================================
	// 标识信息
	// =========================================================================

	// ID 协程的唯一标识符
	// 由调度器分配，单调递增
	ID int64

	// Status 当前状态
	// 使用原子操作保证并发安全
	Status GoroutineStatus

	// =========================================================================
	// 执行上下文 - 每个协程独立拥有
	// =========================================================================

	// Stack 操作数栈
	// 存储表达式计算的中间结果、函数参数等
	Stack [StackMax]bytecode.Value

	// StackTop 栈顶指针
	// 指向下一个空闲位置
	StackTop int

	// Frames 调用栈
	// 存储函数调用的上下文信息
	Frames [FramesMax]CallFrame

	// FrameCount 当前调用帧数量
	FrameCount int

	// =========================================================================
	// 阻塞信息
	// =========================================================================

	// BlockedOn 阻塞在哪个通道上
	// 当 Status 为 GoroutineBlocked 时有效
	BlockedOn *Channel

	// BlockType 阻塞类型（发送/接收）
	BlockType BlockType

	// SendValue 待发送的值
	// 当 BlockType 为 BlockSend 时有效
	SendValue bytecode.Value

	// RecvValue 接收到的值
	// 当从阻塞状态唤醒时，接收操作的结果存储在这里
	RecvValue bytecode.Value

	// =========================================================================
	// Select 相关
	// =========================================================================

	// SelectCases 当前 select 语句的所有 case
	// 当 Status 为 GoroutineWaiting 时有效
	SelectCases []SelectCaseInfo

	// SelectIndex 被选中的 case 索引
	// -1 表示没有 case 被选中（将执行 default 或继续等待）
	SelectIndex int

	// =========================================================================
	// 异常处理
	// =========================================================================

	// Exception 当前异常
	Exception bytecode.Value

	// HasException 是否有未处理的异常
	HasException bool

	// TryStack try 块上下文栈
	TryStack []TryContext

	// TryDepth try 块嵌套深度
	TryDepth int

	// =========================================================================
	// 父子关系（可选，用于调试）
	// =========================================================================

	// ParentID 父协程 ID
	// 主协程的 ParentID 为 0
	ParentID int64
}

// SelectCaseInfo 存储 select case 的信息
type SelectCaseInfo struct {
	// Channel 关联的通道
	Channel *Channel

	// IsRecv true 表示接收操作，false 表示发送操作
	IsRecv bool

	// SendValue 发送操作的值（IsRecv 为 false 时有效）
	SendValue bytecode.Value

	// JumpAddr 匹配时跳转的地址
	JumpAddr int
}

// ============================================================================
// 协程方法
// ============================================================================

// NewGoroutine 创建新协程
func NewGoroutine(id int64, parentID int64) *Goroutine {
	g := &Goroutine{
		ID:          id,
		ParentID:    parentID,
		Status:      GoroutineRunnable,
		StackTop:    0,
		FrameCount:  0,
		BlockType:   BlockNone,
		SelectIndex: -1,
		TryStack:    make([]TryContext, 0, 4),
	}
	return g
}

// SetStatus 原子设置状态
func (g *Goroutine) SetStatus(status GoroutineStatus) {
	atomic.StoreInt32((*int32)(&g.Status), int32(status))
}

// GetStatus 原子获取状态
func (g *Goroutine) GetStatus() GoroutineStatus {
	return GoroutineStatus(atomic.LoadInt32((*int32)(&g.Status)))
}

// IsRunnable 检查协程是否可运行
func (g *Goroutine) IsRunnable() bool {
	return g.GetStatus() == GoroutineRunnable
}

// IsBlocked 检查协程是否阻塞
func (g *Goroutine) IsBlocked() bool {
	status := g.GetStatus()
	return status == GoroutineBlocked || status == GoroutineWaiting
}

// IsDead 检查协程是否已终止
func (g *Goroutine) IsDead() bool {
	return g.GetStatus() == GoroutineDead
}

// Push 压栈
func (g *Goroutine) Push(value bytecode.Value) {
	if g.StackTop >= StackMax {
		panic("goroutine stack overflow")
	}
	g.Stack[g.StackTop] = value
	g.StackTop++
}

// Pop 出栈
func (g *Goroutine) Pop() bytecode.Value {
	if g.StackTop <= 0 {
		panic("goroutine stack underflow")
	}
	g.StackTop--
	return g.Stack[g.StackTop]
}

// Peek 查看栈顶
func (g *Goroutine) Peek() bytecode.Value {
	if g.StackTop <= 0 {
		panic("goroutine stack underflow")
	}
	return g.Stack[g.StackTop-1]
}

// PeekN 查看栈顶第 n 个元素（0 为栈顶）
func (g *Goroutine) PeekN(n int) bytecode.Value {
	if g.StackTop-n-1 < 0 {
		panic("goroutine stack underflow")
	}
	return g.Stack[g.StackTop-n-1]
}

// CurrentFrame 获取当前调用帧
func (g *Goroutine) CurrentFrame() *CallFrame {
	if g.FrameCount <= 0 {
		return nil
	}
	return &g.Frames[g.FrameCount-1]
}

// PushFrame 压入新调用帧
func (g *Goroutine) PushFrame(frame CallFrame) bool {
	if g.FrameCount >= FramesMax {
		return false
	}
	g.Frames[g.FrameCount] = frame
	g.FrameCount++
	return true
}

// PopFrame 弹出调用帧
func (g *Goroutine) PopFrame() *CallFrame {
	if g.FrameCount <= 0 {
		return nil
	}
	g.FrameCount--
	return &g.Frames[g.FrameCount]
}

// Reset 重置协程状态（用于复用）
func (g *Goroutine) Reset() {
	g.Status = GoroutineRunnable
	g.StackTop = 0
	g.FrameCount = 0
	g.BlockedOn = nil
	g.BlockType = BlockNone
	g.SendValue = bytecode.NullValue
	g.RecvValue = bytecode.NullValue
	g.SelectCases = nil
	g.SelectIndex = -1
	g.Exception = bytecode.NullValue
	g.HasException = false
	g.TryStack = g.TryStack[:0]
	g.TryDepth = 0
}

// ============================================================================
// 协程池（用于减少内存分配）
// ============================================================================

// GoroutinePool 协程对象池
type GoroutinePool struct {
	pool []*Goroutine
}

// NewGoroutinePool 创建协程池
func NewGoroutinePool() *GoroutinePool {
	return &GoroutinePool{
		pool: make([]*Goroutine, 0, 16),
	}
}

// Get 从池中获取协程
func (p *GoroutinePool) Get(id int64, parentID int64) *Goroutine {
	if len(p.pool) > 0 {
		g := p.pool[len(p.pool)-1]
		p.pool = p.pool[:len(p.pool)-1]
		g.ID = id
		g.ParentID = parentID
		g.Reset()
		return g
	}
	return NewGoroutine(id, parentID)
}

// Put 归还协程到池中
func (p *GoroutinePool) Put(g *Goroutine) {
	if len(p.pool) < 64 { // 限制池大小
		p.pool = append(p.pool, g)
	}
}
