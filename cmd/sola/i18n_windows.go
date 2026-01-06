//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

var (
	kernel32                   = syscall.NewLazyDLL("kernel32.dll")
	procGetUserDefaultUILanguage = kernel32.NewProc("GetUserDefaultUILanguage")
)

// detectWindowsChinese 使用 Windows API 检测是否为中文系统
func detectWindowsChinese() bool {
	// 调用 GetUserDefaultUILanguage 获取用户界面语言
	// 返回值是 LANGID (Language Identifier)
	ret, _, _ := procGetUserDefaultUILanguage.Call()
	langID := uint16(ret)

	// 提取主语言 ID (低 10 位)
	// LANG_CHINESE = 0x04
	primaryLangID := langID & 0x3FF

	// 中文语言 ID 是 0x04
	// 简体中文: 0x0804 (zh-CN)
	// 繁体中文: 0x0404 (zh-TW), 0x0C04 (zh-HK), 0x1404 (zh-MO)
	return primaryLangID == 0x04
}

// getWindowsLocale 获取 Windows 区域设置名称
func getWindowsLocale() string {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getUserDefaultLocaleName := kernel32.NewProc("GetUserDefaultLocaleName")

	buf := make([]uint16, 85) // LOCALE_NAME_MAX_LENGTH = 85
	ret, _, _ := getUserDefaultLocaleName.Call(
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
	)

	if ret == 0 {
		return ""
	}

	return syscall.UTF16ToString(buf)
}





