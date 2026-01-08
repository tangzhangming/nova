//go:build !windows

package jit

import (
	"syscall"
	"unsafe"
)

const (
	PROT_READ   = 0x1
	PROT_WRITE  = 0x2
	PROT_EXEC   = 0x4
	MAP_PRIVATE = 0x02
	MAP_ANON    = 0x1000
)

// allocExecutable 分配可执行内存（Unix/Linux/macOS）
func allocExecutable(size int) ([]byte, error) {
	// 对齐到页面大小
	pageSize := syscall.Getpagesize()
	alignedSize := (size + pageSize - 1) &^ (pageSize - 1)

	// 使用 mmap 分配可执行内存
	addr, _, errno := syscall.Syscall6(
		syscall.SYS_MMAP,
		0,
		uintptr(alignedSize),
		PROT_READ|PROT_WRITE|PROT_EXEC,
		MAP_PRIVATE|MAP_ANON,
		-1,
		0,
	)

	if errno != 0 {
		return nil, errno
	}

	// 将地址转换为 byte slice
	slice := (*[1 << 30]byte)(unsafe.Pointer(addr))[:alignedSize:alignedSize]
	return slice, nil
}

// freeExecutable 释放可执行内存（Unix/Linux/macOS）
func freeExecutable(mem []byte) error {
	if len(mem) == 0 {
		return nil
	}

	addr := uintptr(unsafe.Pointer(&mem[0]))
	_, _, errno := syscall.Syscall(syscall.SYS_MUNMAP, addr, uintptr(len(mem)), 0)
	if errno != 0 {
		return errno
	}
	return nil
}

