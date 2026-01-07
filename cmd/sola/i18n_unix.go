//go:build !windows

package main

// detectWindowsChinese Unix 系统不需要此函数，返回 false
func detectWindowsChinese() bool {
	return false
}

// getWindowsLocale Unix 系统不需要此函数，返回空字符串
func getWindowsLocale() string {
	return ""
}













