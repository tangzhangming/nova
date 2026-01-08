//go:build !amd64 && !arm64

// bridge_other.go - 不支持的平台
//
// 在不支持 JIT 的平台上，提供空实现

package jit

// CallNative 在不支持的平台上总是返回失败
func CallNative(funcPtr uintptr, args []int64) (int64, bool) {
	return 0, false
}
