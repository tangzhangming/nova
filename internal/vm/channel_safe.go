// Package vm 实现了 Sola 编程语言的字节码虚拟机。
//
// 本文件提供 Channel 的并发安全性增强和验证。
package vm

import (
	"sync"
	"sync/atomic"

	"github.com/tangzhangming/nova/internal/bytecode"
)

// ============================================================================
// 并发安全 Channel 增强
// ============================================================================
//
// BUG FIX 2026-01-10: Channel 并发安全性验证
// 防止反复引入的问题:
// 1. send/recv 是复合操作，需要锁保护
// 2. 关闭通道时需要唤醒所有等待者
// 3. 多个 Worker 可能同时访问同一个 Channel
// 4. 避免死锁：不要在持有锁时调用外部方法

// SafeChannel 线程安全的通道实现
//
// 对基础 Channel 进行封装，添加额外的并发安全保证。
// 用于多线程 VM 环境。
type SafeChannel struct {
	*Channel

	// =========================================================================
	// 增强的同步原语
	// =========================================================================

	// opMu 操作锁（保护完整的 send/recv 操作）
	opMu sync.Mutex

	// stateMu 状态锁（保护状态查询）
	stateMu sync.RWMutex

	// =========================================================================
	// 统计信息
	// =========================================================================

	// sendCount 发送次数
	sendCount atomic.Int64

	// recvCount 接收次数
	recvCount atomic.Int64

	// blockCount 阻塞次数
	blockCount atomic.Int64
}

// NewSafeChannel 创建线程安全的通道
func NewSafeChannel(elementType string, capacity int) *SafeChannel {
	return &SafeChannel{
		Channel: NewChannel(elementType, capacity),
	}
}

// ============================================================================
// 线程安全的发送操作
// ============================================================================

// SendSafe 线程安全的发送操作
//
// 确保整个发送操作是原子的：
//  1. 检查通道状态
//  2. 尝试发送
//  3. 更新等待队列
func (ch *SafeChannel) SendSafe(value bytecode.Value, g *Goroutine, sched *MultiThreadScheduler) (ok bool, blocked bool) {
	ch.opMu.Lock()
	defer ch.opMu.Unlock()

	ch.sendCount.Add(1)

	// 通道已关闭，不能发送
	if ch.Closed {
		return false, false
	}

	// 检查是否有等待接收的协程
	if len(ch.RecvQueue) > 0 {
		// 直接传递给等待的接收者
		receiver := ch.RecvQueue[0]
		ch.RecvQueue = ch.RecvQueue[1:]

		receiver.RecvValue = value

		// 在释放锁后唤醒接收者
		go sched.Unblock(receiver)

		return true, false
	}

	// 有缓冲区且未满
	if ch.Capacity > 0 && ch.count < ch.Capacity {
		ch.Buffer[ch.tail] = value
		ch.tail = (ch.tail + 1) % ch.Capacity
		ch.count++
		return true, false
	}

	// 需要阻塞
	ch.blockCount.Add(1)
	g.SendValue = value
	ch.SendQueue = append(ch.SendQueue, g)
	return true, true
}

// ============================================================================
// 线程安全的接收操作
// ============================================================================

// ReceiveSafe 线程安全的接收操作
func (ch *SafeChannel) ReceiveSafe(g *Goroutine, sched *MultiThreadScheduler) (value bytecode.Value, ok bool, blocked bool) {
	ch.opMu.Lock()
	defer ch.opMu.Unlock()

	ch.recvCount.Add(1)

	// 缓冲区有数据
	if ch.count > 0 {
		value = ch.Buffer[ch.head]
		ch.head = (ch.head + 1) % ch.Capacity
		ch.count--

		// 如果有等待发送的协程，让它发送
		if len(ch.SendQueue) > 0 {
			sender := ch.SendQueue[0]
			ch.SendQueue = ch.SendQueue[1:]

			ch.Buffer[ch.tail] = sender.SendValue
			ch.tail = (ch.tail + 1) % ch.Capacity
			ch.count++

			// 在释放锁后唤醒发送者
			go sched.Unblock(sender)
		}

		return value, true, false
	}

	// 检查是否有等待发送的协程（无缓冲通道）
	if len(ch.SendQueue) > 0 {
		sender := ch.SendQueue[0]
		ch.SendQueue = ch.SendQueue[1:]

		value = sender.SendValue

		// 在释放锁后唤醒发送者
		go sched.Unblock(sender)

		return value, true, false
	}

	// 通道已关闭且为空
	if ch.Closed {
		return bytecode.NullValue, false, false
	}

	// 需要阻塞
	ch.blockCount.Add(1)
	ch.RecvQueue = append(ch.RecvQueue, g)
	return bytecode.NullValue, true, true
}

// ============================================================================
// 线程安全的关闭操作
// ============================================================================

// CloseSafe 线程安全的关闭操作
func (ch *SafeChannel) CloseSafe(sched *MultiThreadScheduler) {
	ch.opMu.Lock()

	if ch.Closed {
		ch.opMu.Unlock()
		return
	}

	ch.Closed = true

	// 收集需要唤醒的协程
	recvWaiters := make([]*Goroutine, len(ch.RecvQueue))
	copy(recvWaiters, ch.RecvQueue)
	ch.RecvQueue = nil

	sendWaiters := make([]*Goroutine, len(ch.SendQueue))
	copy(sendWaiters, ch.SendQueue)
	ch.SendQueue = nil

	ch.opMu.Unlock()

	// 在释放锁后唤醒所有等待者
	for _, g := range recvWaiters {
		g.RecvValue = bytecode.NullValue
		sched.Unblock(g)
	}

	for _, g := range sendWaiters {
		sched.Unblock(g)
	}
}

// ============================================================================
// 状态查询（线程安全）
// ============================================================================

// IsClosedSafe 线程安全地检查通道是否已关闭
func (ch *SafeChannel) IsClosedSafe() bool {
	ch.stateMu.RLock()
	defer ch.stateMu.RUnlock()
	return ch.Closed
}

// LenSafe 线程安全地获取缓冲区元素数量
func (ch *SafeChannel) LenSafe() int {
	ch.stateMu.RLock()
	defer ch.stateMu.RUnlock()
	return ch.count
}

// ============================================================================
// 统计信息
// ============================================================================

// Stats 获取通道统计信息
func (ch *SafeChannel) Stats() ChannelStats {
	return ChannelStats{
		SendCount:  ch.sendCount.Load(),
		RecvCount:  ch.recvCount.Load(),
		BlockCount: ch.blockCount.Load(),
		BufferLen:  ch.LenSafe(),
		BufferCap:  ch.Capacity,
		IsClosed:   ch.IsClosedSafe(),
	}
}

// ChannelStats 通道统计信息
type ChannelStats struct {
	SendCount  int64 // 发送次数
	RecvCount  int64 // 接收次数
	BlockCount int64 // 阻塞次数
	BufferLen  int   // 当前缓冲区大小
	BufferCap  int   // 缓冲区容量
	IsClosed   bool  // 是否已关闭
}

// ============================================================================
// Channel 并发安全性验证器
// ============================================================================

// ChannelVerifier Channel 并发安全性验证器
//
// 用于在测试和调试模式下验证 Channel 的并发安全性。
type ChannelVerifier struct {
	// enabled 是否启用验证
	enabled atomic.Bool

	// violations 检测到的违规
	violations []ChannelViolation

	// mu 保护 violations
	mu sync.Mutex
}

// ChannelViolation 并发违规记录
type ChannelViolation struct {
	Type      string // 违规类型
	ChannelID uintptr
	Message   string
	Timestamp int64
}

// NewChannelVerifier 创建验证器
func NewChannelVerifier() *ChannelVerifier {
	return &ChannelVerifier{}
}

// Enable 启用验证
func (v *ChannelVerifier) Enable() {
	v.enabled.Store(true)
}

// Disable 禁用验证
func (v *ChannelVerifier) Disable() {
	v.enabled.Store(false)
}

// IsEnabled 检查是否启用
func (v *ChannelVerifier) IsEnabled() bool {
	return v.enabled.Load()
}

// RecordViolation 记录违规
func (v *ChannelVerifier) RecordViolation(violation ChannelViolation) {
	if !v.enabled.Load() {
		return
	}

	v.mu.Lock()
	defer v.mu.Unlock()
	v.violations = append(v.violations, violation)
}

// GetViolations 获取所有违规记录
func (v *ChannelVerifier) GetViolations() []ChannelViolation {
	v.mu.Lock()
	defer v.mu.Unlock()

	result := make([]ChannelViolation, len(v.violations))
	copy(result, v.violations)
	return result
}

// ClearViolations 清除违规记录
func (v *ChannelVerifier) ClearViolations() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.violations = nil
}

// HasViolations 检查是否有违规
func (v *ChannelVerifier) HasViolations() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return len(v.violations) > 0
}
