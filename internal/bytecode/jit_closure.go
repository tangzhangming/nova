// jit_closure.go - JIT 友好的闭包实现
//
// 本文件定义了 JIT 编译器使用的闭包结构。
// 闭包包含函数指针和捕获的外部变量（upvalues）。
//
// 内存布局:
//   JITClosure 结构 (固定头部 + 可变 upvalue 数组):
//     偏移 0:  FuncPtr   (8 bytes) - 函数代码地址
//     偏移 8:  NumUpvals (4 bytes) - upvalue 数量
//     偏移 12: Flags     (4 bytes) - 标志位
//     偏移 16: Upvalues  (N*8 bytes) - upvalue 数组

package bytecode

import (
	"sync"
	"unsafe"
)

// JITClosure 常量
const (
	MaxJITUpvalues     = 16   // 最大 upvalue 数量
	JITClosureHeaderSize = 16 // 闭包头部大小
)

// JITClosureFlags 闭包标志
type JITClosureFlags uint32

const (
	JITClosureFlagNone     JITClosureFlags = 0
	JITClosureFlagCompiled JITClosureFlags = 1 << 0 // 已 JIT 编译
	JITClosureFlagVariadic JITClosureFlags = 1 << 1 // 可变参数
)

// JITClosure JIT 友好的闭包结构
type JITClosure struct {
	FuncPtr   uintptr         // 函数代码地址（偏移 0）
	NumUpvals int32           // upvalue 数量（偏移 8）
	Flags     JITClosureFlags // 标志位（偏移 12）
	Upvalues  [MaxJITUpvalues]int64 // upvalue 数组（偏移 16）
}

// JITUpvalue JIT upvalue 结构
type JITUpvalue struct {
	Value    int64  // 值（如果已关闭）
	Location *int64 // 指向栈上的位置（如果未关闭）
	IsClosed bool   // 是否已关闭
}

// NewJITClosure 创建新的 JIT 闭包
func NewJITClosure(funcPtr uintptr, numUpvals int) *JITClosure {
	if numUpvals > MaxJITUpvalues {
		numUpvals = MaxJITUpvalues
	}
	
	return &JITClosure{
		FuncPtr:   funcPtr,
		NumUpvals: int32(numUpvals),
		Flags:     JITClosureFlagNone,
	}
}

// GetUpvalue 获取 upvalue 值
func (c *JITClosure) GetUpvalue(index int) int64 {
	if index < 0 || index >= int(c.NumUpvals) {
		return 0
	}
	return c.Upvalues[index]
}

// SetUpvalue 设置 upvalue 值
func (c *JITClosure) SetUpvalue(index int, value int64) {
	if index >= 0 && index < int(c.NumUpvals) {
		c.Upvalues[index] = value
	}
}

// GetUpvaluePtr 获取 upvalue 指针（用于 JIT）
func (c *JITClosure) GetUpvaluePtr(index int) unsafe.Pointer {
	if index < 0 || index >= int(c.NumUpvals) {
		return nil
	}
	return unsafe.Pointer(&c.Upvalues[index])
}

// ============================================================================
// JITClosure 偏移常量（供 JIT 代码生成使用）
// ============================================================================

const (
	JITClosureOffsetFuncPtr   = 0
	JITClosureOffsetNumUpvals = 8
	JITClosureOffsetFlags     = 12
	JITClosureOffsetUpvalues  = 16
)

// GetJITUpvalueOffset 计算指定索引 upvalue 的偏移
func GetJITUpvalueOffset(index int) int {
	return JITClosureOffsetUpvalues + index*8
}

// ============================================================================
// 闭包池
// ============================================================================

var jitClosurePool = sync.Pool{
	New: func() interface{} {
		return &JITClosure{}
	},
}

// GetJITClosure 从池中获取闭包
func GetJITClosure() *JITClosure {
	c := jitClosurePool.Get().(*JITClosure)
	c.FuncPtr = 0
	c.NumUpvals = 0
	c.Flags = JITClosureFlagNone
	// 不需要清零 Upvalues，会被覆盖
	return c
}

// PutJITClosure 将闭包放回池
func PutJITClosure(c *JITClosure) {
	if c != nil {
		jitClosurePool.Put(c)
	}
}

// ============================================================================
// 从传统 Closure 转换
// ============================================================================

// ToJITClosure 将传统 Closure 转换为 JITClosure
func ToJITClosure(closure *Closure, funcPtr uintptr) *JITClosure {
	if closure == nil {
		return nil
	}
	
	numUpvals := len(closure.Upvalues)
	if numUpvals > MaxJITUpvalues {
		numUpvals = MaxJITUpvalues
	}
	
	jc := NewJITClosure(funcPtr, numUpvals)
	
	// 复制 upvalues
	for i := 0; i < numUpvals; i++ {
		upval := closure.Upvalues[i]
		if upval.IsClosed {
			jc.Upvalues[i] = ValueToInt64(upval.Closed)
		} else if upval.Location != nil {
			jc.Upvalues[i] = ValueToInt64(*upval.Location)
		}
	}
	
	return jc
}

// FromJITClosure 将 JITClosure 转换为传统 Closure
func FromJITClosure(jc *JITClosure, fn *Function) *Closure {
	if jc == nil || fn == nil {
		return nil
	}
	
	closure := &Closure{
		Function: fn,
		Upvalues: make([]*Upvalue, jc.NumUpvals),
	}
	
	for i := int32(0); i < jc.NumUpvals; i++ {
		closure.Upvalues[i] = &Upvalue{
			Closed:   NewInt(jc.Upvalues[i]),
			IsClosed: true,
		}
	}
	
	return closure
}
