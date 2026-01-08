//go:build windows

// memory_windows.go - Windows 平台可执行内存分配
//
// 使用 VirtualAlloc/VirtualFree 分配具有执行权限的内存

package jit

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	memCommit            = 0x1000
	memReserve           = 0x2000
	memRelease           = 0x8000
	pageExecuteReadwrite = 0x40
)

var (
	kernel32       = windows.NewLazySystemDLL("kernel32.dll")
	virtualAlloc   = kernel32.NewProc("VirtualAlloc")
	virtualFree    = kernel32.NewProc("VirtualFree")
)

// allocExecutable 分配可执行内存（Windows）
func allocExecutable(size int) ([]byte, error) {
	if size <= 0 {
		size = 4096
	}
	
	// 对齐到页面大小（4KB）
	pageSize := 4096
	alignedSize := (size + pageSize - 1) &^ (pageSize - 1)
	
	// 调用 VirtualAlloc
	addr, _, err := virtualAlloc.Call(
		0,
		uintptr(alignedSize),
		memCommit|memReserve,
		pageExecuteReadwrite,
	)
	
	if addr == 0 {
		return nil, err
	}
	
	// 转换为 byte slice
	// 使用 unsafe 将地址转换为切片
	slice := unsafe.Slice((*byte)(unsafe.Pointer(addr)), alignedSize)
	return slice, nil
}

// freeExecutable 释放可执行内存（Windows）
func freeExecutable(mem []byte) error {
	if len(mem) == 0 {
		return nil
	}
	
	addr := uintptr(unsafe.Pointer(&mem[0]))
	_, _, err := virtualFree.Call(addr, 0, memRelease)
	
	// VirtualFree 成功时返回非零值
	// 这里简化处理，忽略错误
	_ = err
	return nil
}
