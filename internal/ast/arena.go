package ast

import (
	"unsafe"
)

// ============================================================================
// Arena 内存分配器
// ============================================================================
//
// Arena 是一个高性能的内存池分配器，专为 AST 节点分配设计。
//
// 设计目标：
// - 减少 GC 压力：所有 AST 节点从同一块内存分配，GC 只需追踪少量大块
// - 提高分配速度：简单的指针递增，无需复杂的内存管理
// - 批量释放：解析完成后一次性释放所有节点
//
// 性能优化说明：
// - 使用连续内存块，提高缓存局部性
// - 无锁设计（Parser 是单线程的）
// - 对齐分配确保 CPU 访问效率
//
// 使用方式：
//   arena := NewArena(64 * 1024)  // 64KB 初始块
//   defer arena.Free()
//   node := arena.NewIntegerLiteral(tok, 42)
//
// ============================================================================

// 默认内存块大小：64KB
// 这个大小经过权衡：
// - 太小会频繁分配新块
// - 太大会浪费内存
// - 64KB 通常足够解析中等大小的文件
const defaultChunkSize = 64 * 1024

// Arena 内存池分配器
//
// 内部维护一个内存块列表，分配时从当前块分配，
// 当前块不足时分配新块。
type Arena struct {
	chunks    [][]byte // 内存块列表
	current   []byte   // 当前内存块
	offset    int      // 当前块的分配偏移
	chunkSize int      // 每个块的标准大小
}

// NewArena 创建一个新的 Arena 分配器
//
// 参数:
//   - chunkSize: 每个内存块的大小（字节）
//     如果 <= 0，使用默认值 64KB
//
// 返回:
//   - *Arena: 新创建的分配器
//
// PERF: Arena 创建时会预分配第一个内存块
func NewArena(chunkSize int) *Arena {
	if chunkSize <= 0 {
		chunkSize = defaultChunkSize
	}

	a := &Arena{
		chunks:    make([][]byte, 0, 4), // 预分配 4 个块的容量
		chunkSize: chunkSize,
	}

	// 预分配第一个内存块
	a.grow(chunkSize)

	return a
}

// Alloc 从 Arena 分配指定大小的内存
//
// 参数:
//   - size: 需要分配的字节数
//   - align: 内存对齐要求（必须是 2 的幂，如 1, 2, 4, 8）
//
// 返回:
//   - unsafe.Pointer: 分配的内存指针
//
// PERF: 这是热路径，保持简单以便内联
func (a *Arena) Alloc(size, align int) unsafe.Pointer {
	// 对齐当前偏移
	// PERF: 使用位运算代替取模，因为 align 是 2 的幂
	offset := (a.offset + align - 1) &^ (align - 1)

	// 检查当前块是否有足够空间
	if offset+size > len(a.current) {
		// 当前块不足，分配新块
		a.grow(max(size, a.chunkSize))
		offset = 0
	}

	// 分配内存
	ptr := unsafe.Pointer(&a.current[offset])
	a.offset = offset + size

	return ptr
}

// AllocType 分配指定类型的内存（泛型版本）
//
// 这是一个便捷函数，自动计算大小和对齐。
//
// 用法:
//
//	node := AllocType[IntegerLiteral](arena)
//
// PERF: 使用泛型避免类型断言开销
func AllocType[T any](a *Arena) *T {
	var zero T
	size := int(unsafe.Sizeof(zero))
	align := int(unsafe.Alignof(zero))
	ptr := a.Alloc(size, align)
	return (*T)(ptr)
}

// grow 分配一个新的内存块
//
// 参数:
//   - size: 新块的最小大小
func (a *Arena) grow(size int) {
	// 确保新块足够大
	if size < a.chunkSize {
		size = a.chunkSize
	}

	// 分配新块
	chunk := make([]byte, size)
	a.chunks = append(a.chunks, chunk)
	a.current = chunk
	a.offset = 0
}

// Reset 重置 Arena，保留已分配的内存块以便复用
//
// 调用 Reset 后，之前分配的所有指针都将失效。
// 这比 Free 更高效，因为避免了内存重新分配。
//
// 典型用法：
//   - 在处理多个文件时，每个文件解析完后 Reset
//   - 保留内存块避免频繁 malloc
func (a *Arena) Reset() {
	// 只保留第一个块，释放其他块
	if len(a.chunks) > 1 {
		// 保留第一个块的大小作为新的 chunk
		firstSize := len(a.chunks[0])
		a.chunks = a.chunks[:1]
		a.current = a.chunks[0]

		// 如果第一个块太小，考虑扩大
		if firstSize < a.chunkSize {
			a.chunks[0] = make([]byte, a.chunkSize)
			a.current = a.chunks[0]
		}
	} else if len(a.chunks) == 1 {
		a.current = a.chunks[0]
	}

	a.offset = 0
}

// Free 完全释放 Arena 的所有内存
//
// 调用 Free 后，Arena 可以重新使用，但会重新分配内存。
func (a *Arena) Free() {
	a.chunks = nil
	a.current = nil
	a.offset = 0
}

// Stats 返回 Arena 的统计信息（用于调试和性能分析）
type ArenaStats struct {
	ChunkCount  int // 内存块数量
	TotalBytes  int // 总分配字节数
	UsedBytes   int // 已使用字节数
	WastedBytes int // 浪费的字节数（块末尾未使用部分）
}

// Stats 获取 Arena 的统计信息
func (a *Arena) Stats() ArenaStats {
	stats := ArenaStats{
		ChunkCount: len(a.chunks),
	}

	for i, chunk := range a.chunks {
		stats.TotalBytes += len(chunk)
		if i < len(a.chunks)-1 {
			// 之前的块已完全使用
			stats.UsedBytes += len(chunk)
		} else {
			// 当前块只使用了 offset 部分
			stats.UsedBytes += a.offset
			stats.WastedBytes += len(chunk) - a.offset
		}
	}

	return stats
}

// 注意：Go 1.21+ 已内置 max 函数，无需自定义
