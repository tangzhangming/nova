package vm

import (
	"sync"

	"github.com/tangzhangming/nova/internal/bytecode"
)

// ============================================================================
// sync.Pool 优化高频临时对象
// ============================================================================
//
// 使用 Go 标准库的 sync.Pool 来优化高频分配的临时对象。
// sync.Pool 是并发安全的，且在 GC 时会自动清理未使用的对象。
//
// 适用场景：
// - 参数数组（函数调用时的临时存储）
// - 字节切片（字节操作时的临时缓冲区）
// - 字符串构建器切片
//
// 注意事项：
// - sync.Pool 中的对象可能在任意时刻被 GC 回收
// - 对象归还到池后不应该再使用
// - 池中对象可能被多个 goroutine 共享（如果启用并发）
//
// ============================================================================

// ========== 参数数组池 ==========

// 小参数数组池（容量 4）
var smallArgsPool = sync.Pool{
	New: func() interface{} {
		arr := make([]bytecode.Value, 0, 4)
		return &arr
	},
}

// 中参数数组池（容量 8）
var mediumArgsPool = sync.Pool{
	New: func() interface{} {
		arr := make([]bytecode.Value, 0, 8)
		return &arr
	},
}

// 大参数数组池（容量 16）
var largeArgsPool = sync.Pool{
	New: func() interface{} {
		arr := make([]bytecode.Value, 0, 16)
		return &arr
	},
}

// GetArgsSlice 从 sync.Pool 获取参数切片
func GetArgsSlice(size int) []bytecode.Value {
	var ptr *[]bytecode.Value
	
	switch {
	case size <= 4:
		ptr = smallArgsPool.Get().(*[]bytecode.Value)
	case size <= 8:
		ptr = mediumArgsPool.Get().(*[]bytecode.Value)
	case size <= 16:
		ptr = largeArgsPool.Get().(*[]bytecode.Value)
	default:
		// 超出池大小，直接分配
		return make([]bytecode.Value, size)
	}
	
	// 扩展到需要的大小
	arr := (*ptr)[:0]
	for i := 0; i < size; i++ {
		arr = append(arr, bytecode.NullValue)
	}
	*ptr = arr
	return *ptr
}

// PutArgsSlice 归还参数切片到 sync.Pool
func PutArgsSlice(arr []bytecode.Value) {
	if arr == nil {
		return
	}
	
	// 清空切片内容，避免内存泄漏
	for i := range arr {
		arr[i] = bytecode.NullValue
	}
	arr = arr[:0]
	
	cap := cap(arr)
	switch {
	case cap <= 4:
		smallArgsPool.Put(&arr)
	case cap <= 8:
		mediumArgsPool.Put(&arr)
	case cap <= 16:
		largeArgsPool.Put(&arr)
	}
	// 超出池大小的切片让 GC 回收
}

// ========== 字节切片池 ==========

// 小字节切片池（容量 64）
var smallBytesPool = sync.Pool{
	New: func() interface{} {
		b := make([]byte, 0, 64)
		return &b
	},
}

// 中字节切片池（容量 256）
var mediumBytesPool = sync.Pool{
	New: func() interface{} {
		b := make([]byte, 0, 256)
		return &b
	},
}

// 大字节切片池（容量 1024）
var largeBytesPool = sync.Pool{
	New: func() interface{} {
		b := make([]byte, 0, 1024)
		return &b
	},
}

// 超大字节切片池（容量 4096）
var xlargeBytesPool = sync.Pool{
	New: func() interface{} {
		b := make([]byte, 0, 4096)
		return &b
	},
}

// GetBytesSlice 从 sync.Pool 获取字节切片
func GetBytesSlice(size int) []byte {
	var ptr *[]byte
	
	switch {
	case size <= 64:
		ptr = smallBytesPool.Get().(*[]byte)
	case size <= 256:
		ptr = mediumBytesPool.Get().(*[]byte)
	case size <= 1024:
		ptr = largeBytesPool.Get().(*[]byte)
	case size <= 4096:
		ptr = xlargeBytesPool.Get().(*[]byte)
	default:
		// 超出池大小，直接分配
		return make([]byte, size)
	}
	
	// 扩展到需要的大小
	b := (*ptr)[:0]
	if cap(b) >= size {
		*ptr = b[:size]
		return *ptr
	}
	
	// 容量不足，分配新的
	return make([]byte, size)
}

// PutBytesSlice 归还字节切片到 sync.Pool
func PutBytesSlice(b []byte) {
	if b == nil {
		return
	}
	
	// 清空切片内容
	for i := range b {
		b[i] = 0
	}
	b = b[:0]
	
	cap := cap(b)
	switch {
	case cap <= 64:
		smallBytesPool.Put(&b)
	case cap <= 256:
		mediumBytesPool.Put(&b)
	case cap <= 1024:
		largeBytesPool.Put(&b)
	case cap <= 4096:
		xlargeBytesPool.Put(&b)
	}
	// 超出池大小的切片让 GC 回收
}

// ========== GCObject 切片池（用于 GC 内部）==========

// GCObject 切片池（容量 128）
var gcObjectSlicePool = sync.Pool{
	New: func() interface{} {
		s := make([]GCObject, 0, 128)
		return &s
	},
}

// GetGCObjectSlice 从 sync.Pool 获取 GCObject 切片
func GetGCObjectSlice(size int) []GCObject {
	if size > 128 {
		return make([]GCObject, 0, size)
	}
	
	ptr := gcObjectSlicePool.Get().(*[]GCObject)
	s := (*ptr)[:0]
	*ptr = s
	return *ptr
}

// PutGCObjectSlice 归还 GCObject 切片到 sync.Pool
func PutGCObjectSlice(s []GCObject) {
	if s == nil || cap(s) > 128 {
		return
	}
	
	// 清空引用，避免内存泄漏
	for i := range s {
		s[i] = nil
	}
	s = s[:0]
	
	gcObjectSlicePool.Put(&s)
}

// ========== 池统计信息 ==========

// PoolStats 池统计信息（用于调试）
type PoolStats struct {
	ArgsPoolSizes  []int // 各参数数组池的大小档位
	BytesPoolSizes []int // 各字节切片池的大小档位
}

// GetPoolSizes 获取池大小档位信息
func GetPoolSizes() PoolStats {
	return PoolStats{
		ArgsPoolSizes:  []int{4, 8, 16},
		BytesPoolSizes: []int{64, 256, 1024, 4096},
	}
}
