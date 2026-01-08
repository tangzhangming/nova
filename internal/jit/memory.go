// memory.go - 可执行内存管理
//
// 本文件提供了跨平台的可执行内存分配接口。
// JIT 编译生成的机器码需要存储在可执行内存中才能被 CPU 执行。
//
// 安全注意事项：
// - 分配的内存同时具有读、写、执行权限（RWX）
// - 在生产环境中应考虑使用 W^X（写异或执行）策略
// - 先以 RW 权限写入代码，再更改为 RX 权限执行

package jit

import (
	"unsafe"
)

// getCodePointer 获取代码的函数指针
// 返回代码第一个字节的地址作为函数入口点
func getCodePointer(code []byte) uintptr {
	if len(code) == 0 {
		return 0
	}
	return uintptr(unsafe.Pointer(&code[0]))
}

// CodeCache 代码缓存
// 管理已编译函数的存储和查找
type CodeCache struct {
	maxSize   int                        // 最大缓存大小
	usedSize  int                        // 已使用大小
	entries   map[string]*CompiledFunc   // 函数名 -> 编译结果
	allocator executableAllocator        // 可执行内存分配器
}

// NewCodeCache 创建代码缓存
func NewCodeCache(maxSize int) *CodeCache {
	return &CodeCache{
		maxSize: maxSize,
		entries: make(map[string]*CompiledFunc),
	}
}

// Get 获取已编译的函数
func (cc *CodeCache) Get(name string) *CompiledFunc {
	return cc.entries[name]
}

// Put 存储编译结果
func (cc *CodeCache) Put(name string, compiled *CompiledFunc) {
	// 检查是否超过容量
	if cc.usedSize+len(compiled.Code) > cc.maxSize {
		// 简单策略：清除所有缓存
		cc.Clear()
	}
	
	cc.entries[name] = compiled
	cc.usedSize += len(compiled.Code)
}

// Clear 清除所有缓存
func (cc *CodeCache) Clear() {
	// 释放所有可执行内存
	for _, entry := range cc.entries {
		if len(entry.Code) > 0 {
			freeExecutable(entry.Code)
		}
	}
	cc.entries = make(map[string]*CompiledFunc)
	cc.usedSize = 0
}

// AllocateExecutable 分配可执行内存
func (cc *CodeCache) AllocateExecutable(size int) ([]byte, error) {
	return allocExecutable(size)
}

// executableAllocator 可执行内存分配器接口
type executableAllocator interface {
	Allocate(size int) ([]byte, error)
	Free(mem []byte) error
}
