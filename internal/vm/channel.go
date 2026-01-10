// Package vm 实现了 Sola 编程语言的字节码虚拟机。
//
// 本文件实现通道（Channel）数据结构，用于协程间通信。
package vm

import (
	"sync"

	"github.com/tangzhangming/nova/internal/bytecode"
)

// ============================================================================
// 通道结构
// ============================================================================

// Channel 表示一个 Sola 通道
//
// 通道是协程间通信的主要机制，遵循 CSP（Communicating Sequential Processes）模型。
//
// 特性：
//   - 类型安全：编译期检查通道元素类型
//   - 阻塞语义：无缓冲通道发送/接收会阻塞
//   - 缓冲支持：有缓冲通道可减少阻塞
//   - 关闭语义：关闭后不能发送，但可以继续接收缓冲数据
type Channel struct {
	// =========================================================================
	// 类型信息
	// =========================================================================

	// ElementType 元素类型名
	// 用于运行时类型检查和调试
	ElementType string

	// =========================================================================
	// 缓冲区
	// =========================================================================

	// Capacity 缓冲区容量
	// 0 表示无缓冲通道
	Capacity int

	// Buffer 缓冲区
	// 使用环形缓冲区实现
	Buffer []bytecode.Value

	// head 缓冲区头指针（下一个读取位置）
	head int

	// tail 缓冲区尾指针（下一个写入位置）
	tail int

	// count 当前缓冲区元素数量
	count int

	// =========================================================================
	// 状态
	// =========================================================================

	// Closed 通道是否已关闭
	Closed bool

	// =========================================================================
	// 等待队列
	// =========================================================================

	// SendQueue 等待发送的协程队列
	SendQueue []*Goroutine

	// RecvQueue 等待接收的协程队列
	RecvQueue []*Goroutine

	// =========================================================================
	// 同步
	// =========================================================================

	// mu 保护通道状态
	mu sync.Mutex
}

// ============================================================================
// 通道创建
// ============================================================================

// NewChannel 创建通道
//
// 参数:
//   - elementType: 元素类型名（用于类型检查）
//   - capacity: 缓冲区容量（0 表示无缓冲）
func NewChannel(elementType string, capacity int) *Channel {
	if capacity < 0 {
		capacity = 0
	}

	ch := &Channel{
		ElementType: elementType,
		Capacity:    capacity,
		Closed:      false,
		SendQueue:   make([]*Goroutine, 0, 4),
		RecvQueue:   make([]*Goroutine, 0, 4),
	}

	if capacity > 0 {
		ch.Buffer = make([]bytecode.Value, capacity)
	}

	return ch
}

// checkValueType 检查值是否与通道的元素类型匹配
// 返回 true 表示类型匹配，false 表示不匹配
func (ch *Channel) checkValueType(value bytecode.Value) bool {
	// 如果没有指定类型或是 any 类型，接受任何值
	if ch.ElementType == "" || ch.ElementType == "any" {
		return true
	}
	
	// 根据值类型检查
	switch value.Type {
	case bytecode.ValNull:
		// null 可以赋值给任何引用类型
		return ch.ElementType == "null" || 
			ch.ElementType == "object" || 
			ch.ElementType == "string" ||
			ch.ElementType == "array"
	case bytecode.ValBool:
		return ch.ElementType == "bool" || ch.ElementType == "boolean"
	case bytecode.ValInt:
		return ch.ElementType == "int" || ch.ElementType == "integer" || ch.ElementType == "number"
	case bytecode.ValFloat:
		return ch.ElementType == "float" || ch.ElementType == "double" || ch.ElementType == "number"
	case bytecode.ValString:
		return ch.ElementType == "string"
	case bytecode.ValArray:
		return ch.ElementType == "array"
	case bytecode.ValObject:
		// 对于对象类型，需要检查具体的类名
		if ch.ElementType == "object" {
			return true // 接受任何对象
		}
		// 检查具体类型
		if obj := value.AsObject(); obj != nil && obj.Class != nil {
			return ch.isTypeCompatible(obj.Class.Name, ch.ElementType)
		}
		return false
	case bytecode.ValFunc, bytecode.ValClosure:
		return ch.ElementType == "function" || ch.ElementType == "callable"
	case bytecode.ValChannel:
		return ch.ElementType == "channel"
	default:
		return false
	}
}

// isTypeCompatible 检查类型兼容性（包括继承关系）
func (ch *Channel) isTypeCompatible(actualType, expectedType string) bool {
	// 直接匹配
	if actualType == expectedType {
		return true
	}
	
	// TODO: 如果需要支持继承关系检查，可以在这里添加
	// 需要访问类层次结构信息
	
	return false
}

// GetElementType 获取通道的元素类型
func (ch *Channel) GetElementType() string {
	return ch.ElementType
}

// TypeMismatchError 类型不匹配错误
type TypeMismatchError struct {
	Expected string
	Actual   string
}

func (e *TypeMismatchError) Error() string {
	return "channel type mismatch: expected " + e.Expected + ", got " + e.Actual
}

// ============================================================================
// 发送操作
// ============================================================================

// Send 发送值到通道（阻塞）
//
// 返回值:
//   - ok: true 表示发送成功，false 表示通道已关闭
//   - blocked: true 表示需要阻塞当前协程
//
// 调用者需要在 blocked 为 true 时调用调度器的 Block 方法
func (ch *Channel) Send(value bytecode.Value, g *Goroutine, sched *Scheduler) (ok bool, blocked bool) {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	// 通道已关闭，不能发送
	if ch.Closed {
		return false, false
	}
	
	// 运行时类型检查（如果通道指定了元素类型）
	if ch.ElementType != "" && ch.ElementType != "any" {
		if !ch.checkValueType(value) {
			return false, false // 类型不匹配
		}
	}

	// 检查是否有等待接收的协程
	if len(ch.RecvQueue) > 0 {
		// 直接传递给等待的接收者
		receiver := ch.RecvQueue[0]
		ch.RecvQueue = ch.RecvQueue[1:]

		receiver.RecvValue = value
		sched.Unblock(receiver)

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
	g.SendValue = value
	ch.SendQueue = append(ch.SendQueue, g)
	return true, true
}

// TrySend 尝试发送（非阻塞）
//
// 返回值:
//   - ok: true 表示发送成功
func (ch *Channel) TrySend(value bytecode.Value, sched *Scheduler) bool {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	if ch.Closed {
		return false
	}
	
	// 运行时类型检查（如果通道指定了元素类型）
	if ch.ElementType != "" && ch.ElementType != "any" {
		if !ch.checkValueType(value) {
			return false // 类型不匹配
		}
	}

	// 检查是否有等待接收的协程
	if len(ch.RecvQueue) > 0 {
		receiver := ch.RecvQueue[0]
		ch.RecvQueue = ch.RecvQueue[1:]

		receiver.RecvValue = value
		sched.Unblock(receiver)

		return true
	}

	// 有缓冲区且未满
	if ch.Capacity > 0 && ch.count < ch.Capacity {
		ch.Buffer[ch.tail] = value
		ch.tail = (ch.tail + 1) % ch.Capacity
		ch.count++
		return true
	}

	// 无法立即发送
	return false
}

// ============================================================================
// 接收操作
// ============================================================================

// Receive 从通道接收值（阻塞）
//
// 返回值:
//   - value: 接收到的值（如果成功）
//   - ok: true 表示接收成功，false 表示通道已关闭且为空
//   - blocked: true 表示需要阻塞当前协程
func (ch *Channel) Receive(g *Goroutine, sched *Scheduler) (value bytecode.Value, ok bool, blocked bool) {
	ch.mu.Lock()
	defer ch.mu.Unlock()

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

			sched.Unblock(sender)
		}

		return value, true, false
	}

	// 检查是否有等待发送的协程（无缓冲通道）
	if len(ch.SendQueue) > 0 {
		sender := ch.SendQueue[0]
		ch.SendQueue = ch.SendQueue[1:]

		value = sender.SendValue
		sched.Unblock(sender)

		return value, true, false
	}

	// 通道已关闭且为空
	if ch.Closed {
		return bytecode.NullValue, false, false
	}

	// 需要阻塞
	ch.RecvQueue = append(ch.RecvQueue, g)
	return bytecode.NullValue, true, true
}

// TryReceive 尝试接收（非阻塞）
//
// 返回值:
//   - value: 接收到的值
//   - ok: true 表示接收成功
//   - closed: true 表示通道已关闭
func (ch *Channel) TryReceive(sched *Scheduler) (value bytecode.Value, ok bool, closed bool) {
	ch.mu.Lock()
	defer ch.mu.Unlock()

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

			sched.Unblock(sender)
		}

		return value, true, ch.Closed
	}

	// 检查是否有等待发送的协程（无缓冲通道）
	if len(ch.SendQueue) > 0 {
		sender := ch.SendQueue[0]
		ch.SendQueue = ch.SendQueue[1:]

		value = sender.SendValue
		sched.Unblock(sender)

		return value, true, ch.Closed
	}

	return bytecode.NullValue, false, ch.Closed
}

// ============================================================================
// 关闭操作
// ============================================================================

// Close 关闭通道
//
// 关闭后：
//   - 不能再发送（会 panic）
//   - 可以继续接收缓冲区中的数据
//   - 缓冲区为空时，接收返回零值和 false
func (ch *Channel) Close(sched *Scheduler) {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	if ch.Closed {
		return
	}

	ch.Closed = true

	// 唤醒所有等待接收的协程（它们会收到零值和 closed=true）
	for _, g := range ch.RecvQueue {
		g.RecvValue = bytecode.NullValue
		sched.Unblock(g)
	}
	ch.RecvQueue = nil

	// 对于等待发送的协程，不做处理（它们的发送会失败）
	// 在实际语言中可能需要让它们 panic
	for _, g := range ch.SendQueue {
		sched.Unblock(g)
	}
	ch.SendQueue = nil
}

// IsClosed 检查通道是否已关闭
func (ch *Channel) IsClosed() bool {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	return ch.Closed
}

// ============================================================================
// 等待队列管理
// ============================================================================

// AddSendWaiter 添加发送等待者
func (ch *Channel) AddSendWaiter(g *Goroutine) {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	ch.SendQueue = append(ch.SendQueue, g)
}

// AddRecvWaiter 添加接收等待者
func (ch *Channel) AddRecvWaiter(g *Goroutine) {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	ch.RecvQueue = append(ch.RecvQueue, g)
}

// RemoveSendWaiter 移除发送等待者
func (ch *Channel) RemoveSendWaiter(g *Goroutine) {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	for i, waiter := range ch.SendQueue {
		if waiter == g {
			ch.SendQueue = append(ch.SendQueue[:i], ch.SendQueue[i+1:]...)
			return
		}
	}
}

// RemoveRecvWaiter 移除接收等待者
func (ch *Channel) RemoveRecvWaiter(g *Goroutine) {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	for i, waiter := range ch.RecvQueue {
		if waiter == g {
			ch.RecvQueue = append(ch.RecvQueue[:i], ch.RecvQueue[i+1:]...)
			return
		}
	}
}

// RemoveWaiter 从所有等待队列中移除
func (ch *Channel) RemoveWaiter(g *Goroutine) {
	ch.RemoveSendWaiter(g)
	ch.RemoveRecvWaiter(g)
}

// ============================================================================
// 状态查询
// ============================================================================

// Len 获取缓冲区当前元素数量
func (ch *Channel) Len() int {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	return ch.count
}

// Cap 获取缓冲区容量
func (ch *Channel) Cap() int {
	return ch.Capacity
}

// CanSend 检查是否可以立即发送（不阻塞）
func (ch *Channel) CanSend() bool {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	if ch.Closed {
		return false
	}

	// 有等待接收者或缓冲区未满
	return len(ch.RecvQueue) > 0 || (ch.Capacity > 0 && ch.count < ch.Capacity)
}

// CanReceive 检查是否可以立即接收（不阻塞）
func (ch *Channel) CanReceive() bool {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	// 缓冲区有数据或有等待发送者
	return ch.count > 0 || len(ch.SendQueue) > 0
}

// ============================================================================
// Select 支持
// ============================================================================

// TrySelect 尝试在 select 中选择此通道
//
// 参数:
//   - isRecv: true 表示接收操作，false 表示发送操作
//   - sendValue: 发送操作的值（isRecv 为 false 时有效）
//
// 返回值:
//   - ready: true 表示操作可以立即完成
//   - value: 接收到的值（isRecv 为 true 且 ready 为 true 时有效）
func (ch *Channel) TrySelect(isRecv bool, sendValue bytecode.Value, sched *Scheduler) (ready bool, value bytecode.Value) {
	if isRecv {
		val, ok, _ := ch.TryReceive(sched)
		return ok, val
	} else {
		ok := ch.TrySend(sendValue, sched)
		return ok, bytecode.NullValue
	}
}

// ============================================================================
// 调试支持
// ============================================================================

// DumpState 输出通道状态（用于调试）
func (ch *Channel) DumpState() map[string]interface{} {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	sendWaiters := make([]int64, len(ch.SendQueue))
	for i, g := range ch.SendQueue {
		sendWaiters[i] = g.ID
	}

	recvWaiters := make([]int64, len(ch.RecvQueue))
	for i, g := range ch.RecvQueue {
		recvWaiters[i] = g.ID
	}

	return map[string]interface{}{
		"elementType": ch.ElementType,
		"capacity":    ch.Capacity,
		"count":       ch.count,
		"closed":      ch.Closed,
		"sendWaiters": sendWaiters,
		"recvWaiters": recvWaiters,
	}
}
