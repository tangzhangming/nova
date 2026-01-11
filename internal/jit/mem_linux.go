//go:build linux && amd64

package jit

import (
	"fmt"
	"syscall"
	"unsafe"
)

// Linux mmap 保护常量
const (
	PROT_READ  = 0x1
	PROT_WRITE = 0x2
	PROT_EXEC  = 0x4

	MAP_PRIVATE   = 0x2
	MAP_ANONYMOUS = 0x20
)

// allocateExecutableMemory 分配可执行内存 (Linux)
func allocateExecutableMemory(size int) (uintptr, error) {
	// 对齐到页边界
	alignedSize := (size + pageSize - 1) & ^(pageSize - 1)

	// 调用 mmap
	addr, _, errno := syscall.Syscall6(
		syscall.SYS_MMAP,
		0,                              // addr - 让系统选择
		uintptr(alignedSize),           // length
		PROT_READ|PROT_WRITE|PROT_EXEC, // prot
		MAP_PRIVATE|MAP_ANONYMOUS,      // flags
		^uintptr(0),                    // fd = -1
		0,                              // offset
	)

	if errno != 0 {
		return 0, fmt.Errorf("mmap failed: %v", errno)
	}

	return addr, nil
}

// freeExecutableMemory 释放可执行内存 (Linux)
func freeExecutableMemory(addr uintptr, size int) error {
	// 对齐大小
	alignedSize := (size + pageSize - 1) & ^(pageSize - 1)

	_, _, errno := syscall.Syscall(
		syscall.SYS_MUNMAP,
		addr,
		uintptr(alignedSize),
		0,
	)

	if errno != 0 {
		return fmt.Errorf("munmap failed: %v", errno)
	}

	return nil
}

// copyToExecutableMemory 复制代码到可执行内存
func copyToExecutableMemory(dst uintptr, src []byte) {
	dstSlice := (*[1 << 30]byte)(unsafe.Pointer(dst))[:len(src):len(src)]
	copy(dstSlice, src)
}

// makeExecutable 将内存设置为可执行
func makeExecutable(addr uintptr, size int) error {
	// Linux 在 mmap 时已经设置了 PROT_EXEC
	return nil
}

// ExecutableBuffer 可执行缓冲区
type ExecutableBuffer struct {
	addr uintptr
	size int
	used int
}

// NewExecutableBuffer 创建可执行缓冲区
func NewExecutableBuffer(size int) (*ExecutableBuffer, error) {
	addr, err := allocateExecutableMemory(size)
	if err != nil {
		return nil, err
	}

	return &ExecutableBuffer{
		addr: addr,
		size: size,
		used: 0,
	}, nil
}

// Write 写入代码
func (eb *ExecutableBuffer) Write(code []byte) (uintptr, error) {
	if eb.used+len(code) > eb.size {
		return 0, fmt.Errorf("executable buffer overflow")
	}

	writeAddr := eb.addr + uintptr(eb.used)
	copyToExecutableMemory(writeAddr, code)
	eb.used += len(code)
	eb.used = (eb.used + 15) & ^15

	return writeAddr, nil
}

// Free 释放缓冲区
func (eb *ExecutableBuffer) Free() error {
	if eb.addr != 0 {
		err := freeExecutableMemory(eb.addr, eb.size)
		eb.addr = 0
		eb.size = 0
		eb.used = 0
		return err
	}
	return nil
}

// Addr 获取基地址
func (eb *ExecutableBuffer) Addr() uintptr {
	return eb.addr
}

// Size 获取总大小
func (eb *ExecutableBuffer) Size() int {
	return eb.size
}

// Used 获取已使用大小
func (eb *ExecutableBuffer) Used() int {
	return eb.used
}

// Available 获取可用大小
func (eb *ExecutableBuffer) Available() int {
	return eb.size - eb.used
}

// ============================================================================
// 全局可执行内存池
// ============================================================================

// GlobalCodeCache 全局代码缓存
var GlobalCodeCache *CodeCache

// CodeCache 代码缓存管理器
type CodeCache struct {
	buffers    []*ExecutableBuffer
	current    *ExecutableBuffer
	bufferSize int
}

// NewCodeCache 创建代码缓存
func NewCodeCache(bufferSize int) *CodeCache {
	return &CodeCache{
		buffers:    make([]*ExecutableBuffer, 0),
		bufferSize: bufferSize,
	}
}

// Allocate 分配代码空间
func (cc *CodeCache) Allocate(size int) (uintptr, error) {
	if cc.current == nil || cc.current.Available() < size {
		allocSize := cc.bufferSize
		if size > allocSize {
			allocSize = size + pageSize
		}

		buf, err := NewExecutableBuffer(allocSize)
		if err != nil {
			return 0, err
		}

		cc.buffers = append(cc.buffers, buf)
		cc.current = buf
	}

	return cc.current.addr + uintptr(cc.current.used), nil
}

// Install 安装代码
func (cc *CodeCache) Install(code []byte) (uintptr, error) {
	if cc.current == nil || cc.current.Available() < len(code) {
		allocSize := cc.bufferSize
		if len(code) > allocSize {
			allocSize = len(code) + pageSize
		}

		buf, err := NewExecutableBuffer(allocSize)
		if err != nil {
			return 0, err
		}

		cc.buffers = append(cc.buffers, buf)
		cc.current = buf
	}

	return cc.current.Write(code)
}

// Free 释放所有缓冲区
func (cc *CodeCache) Free() error {
	var lastErr error
	for _, buf := range cc.buffers {
		if err := buf.Free(); err != nil {
			lastErr = err
		}
	}
	cc.buffers = nil
	cc.current = nil
	return lastErr
}

// Stats 返回统计信息
func (cc *CodeCache) Stats() (totalSize, usedSize int) {
	for _, buf := range cc.buffers {
		totalSize += buf.Size()
		usedSize += buf.Used()
	}
	return
}

func init() {
	GlobalCodeCache = NewCodeCache(defaultBlockSize)
}
