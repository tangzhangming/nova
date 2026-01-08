//go:build windows

package jit

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	MEM_COMMIT            = 0x1000
	MEM_RESERVE           = 0x2000
	PAGE_EXECUTE_READWRITE = 0x40
)

var (
	kernel32           = windows.NewLazySystemDLL("kernel32.dll")
	virtualAlloc      = kernel32.NewProc("VirtualAlloc")
	virtualFree       = kernel32.NewProc("VirtualFree")
	virtualProtect    = kernel32.NewProc("VirtualProtect")
)

const (
	MEM_RELEASE = 0x8000
)

// allocExecutable 分配可执行内存（Windows）
func allocExecutable(size int) ([]byte, error) {
	// 对齐到页面大小（4KB）
	pageSize := 4096
	alignedSize := (size + pageSize - 1) &^ (pageSize - 1)

	// 使用 VirtualAlloc 分配可执行内存
	addr, _, err := virtualAlloc.Call(
		0,
		uintptr(alignedSize),
		MEM_COMMIT|MEM_RESERVE,
		PAGE_EXECUTE_READWRITE,
	)

	if addr == 0 {
		return nil, err
	}

	// 将地址转换为 byte slice
	// 使用 unsafe 将指针转换为 slice
	slice := (*[1 << 30]byte)(unsafe.Pointer(addr))[:alignedSize:alignedSize]
	return slice, nil
}

// freeExecutable 释放可执行内存（Windows）
func freeExecutable(mem []byte) error {
	if len(mem) == 0 {
		return nil
	}

	addr := uintptr(unsafe.Pointer(&mem[0]))
	_, _, err := virtualFree.Call(addr, 0, MEM_RELEASE)
	return err
}

