//go:build !windows

// memory_unix.go - Unix/Linux/macOS 平台可执行内存分配
//
// 使用 mmap/munmap 分配具有执行权限的内存

package jit

import (
	"syscall"
	"unsafe"
)

// allocExecutable 分配可执行内存（Unix）
func allocExecutable(size int) ([]byte, error) {
	if size <= 0 {
		size = 4096
	}
	
	// 对齐到页面大小
	pageSize := syscall.Getpagesize()
	alignedSize := (size + pageSize - 1) &^ (pageSize - 1)
	
	// 使用 mmap 分配
	// PROT_READ | PROT_WRITE | PROT_EXEC
	// MAP_PRIVATE | MAP_ANONYMOUS
	mem, err := syscall.Mmap(
		-1, // fd
		0,  // offset
		alignedSize,
		syscall.PROT_READ|syscall.PROT_WRITE|syscall.PROT_EXEC,
		syscall.MAP_PRIVATE|syscall.MAP_ANON,
	)
	
	if err != nil {
		return nil, err
	}
	
	return mem, nil
}

// freeExecutable 释放可执行内存（Unix）
func freeExecutable(mem []byte) error {
	if len(mem) == 0 {
		return nil
	}
	
	return syscall.Munmap(mem)
}

// 备用实现：直接使用 syscall
func allocExecutableAlt(size int) ([]byte, error) {
	pageSize := syscall.Getpagesize()
	alignedSize := (size + pageSize - 1) &^ (pageSize - 1)
	
	addr, _, errno := syscall.Syscall6(
		syscall.SYS_MMAP,
		0,
		uintptr(alignedSize),
		syscall.PROT_READ|syscall.PROT_WRITE|syscall.PROT_EXEC,
		syscall.MAP_PRIVATE|syscall.MAP_ANON,
		^uintptr(0), // -1
		0,
	)
	
	if errno != 0 {
		return nil, errno
	}
	
	slice := unsafe.Slice((*byte)(unsafe.Pointer(addr)), alignedSize)
	return slice, nil
}
