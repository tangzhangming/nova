//go:build windows && amd64

package jit

import (
	"fmt"
	"syscall"
	"unsafe"
)

// Windows 内存保护常量
const (
	MEM_COMMIT      = 0x1000
	MEM_RESERVE     = 0x2000
	MEM_RELEASE     = 0x8000
	PAGE_EXECUTE_READWRITE = 0x40
	PAGE_READWRITE  = 0x04
)

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procVirtualAlloc = kernel32.NewProc("VirtualAlloc")
	procVirtualFree  = kernel32.NewProc("VirtualFree")
	procVirtualProtect = kernel32.NewProc("VirtualProtect")
)

// allocateExecutableMemory 分配可执行内存 (Windows)
func allocateExecutableMemory(size int) (uintptr, error) {
	// 对齐到页边界
	alignedSize := (size + pageSize - 1) & ^(pageSize - 1)

	// 调用 VirtualAlloc
	addr, _, err := procVirtualAlloc.Call(
		0,                             // lpAddress - 让系统选择地址
		uintptr(alignedSize),          // dwSize
		MEM_COMMIT|MEM_RESERVE,        // flAllocationType
		PAGE_EXECUTE_READWRITE,        // flProtect
	)

	if addr == 0 {
		return 0, fmt.Errorf("VirtualAlloc failed: %v", err)
	}

	return addr, nil
}

// freeExecutableMemory 释放可执行内存 (Windows)
func freeExecutableMemory(addr uintptr, size int) error {
	ret, _, err := procVirtualFree.Call(
		addr,
		0,           // dwSize - 使用 MEM_RELEASE 时必须为 0
		MEM_RELEASE, // dwFreeType
	)

	if ret == 0 {
		return fmt.Errorf("VirtualFree failed: %v", err)
	}

	return nil
}

// copyToExecutableMemory 复制代码到可执行内存
func copyToExecutableMemory(dst uintptr, src []byte) {
	// 直接复制 (内存已经是读写可执行的)
	dstSlice := (*[1 << 30]byte)(unsafe.Pointer(dst))[:len(src):len(src)]
	copy(dstSlice, src)
}

// makeExecutable 将内存设置为可执行 (如果需要)
func makeExecutable(addr uintptr, size int) error {
	// Windows 在 VirtualAlloc 时已经设置了 PAGE_EXECUTE_READWRITE
	// 这里不需要额外操作
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

	// 计算写入位置
	writeAddr := eb.addr + uintptr(eb.used)

	// 复制代码
	copyToExecutableMemory(writeAddr, code)

	// 更新已使用大小
	eb.used += len(code)

	// 对齐到 16 字节边界 (代码对齐)
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
	buffers []*ExecutableBuffer
	current *ExecutableBuffer
	
	// 配置
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
	// 检查当前缓冲区是否有足够空间
	if cc.current == nil || cc.current.Available() < size {
		// 创建新缓冲区
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
	// 检查当前缓冲区是否有足够空间
	if cc.current == nil || cc.current.Available() < len(code) {
		// 创建新缓冲区
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

// init 初始化全局代码缓存
func init() {
	GlobalCodeCache = NewCodeCache(defaultBlockSize)
}
