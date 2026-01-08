//go:build amd64

// bridge_amd64.go - AMD64 平台的 JIT 桥接
//
// 本文件实现了在 AMD64 平台上调用 JIT 编译代码的功能。
// 使用 Go 汇编实现底层的函数调用。

package jit

// callNative0 调用无参数的 JIT 函数
// funcPtr: 函数入口地址
// 返回: 函数返回值
func callNative0(funcPtr uintptr) int64

// callNative1 调用单参数的 JIT 函数
func callNative1(funcPtr uintptr, arg0 int64) int64

// callNative2 调用双参数的 JIT 函数
func callNative2(funcPtr uintptr, arg0, arg1 int64) int64

// callNative3 调用三参数的 JIT 函数
func callNative3(funcPtr uintptr, arg0, arg1, arg2 int64) int64

// callNative4 调用四参数的 JIT 函数
func callNative4(funcPtr uintptr, arg0, arg1, arg2, arg3 int64) int64

// CallNative 调用 JIT 编译的函数
func CallNative(funcPtr uintptr, args []int64) (int64, bool) {
	if funcPtr == 0 {
		return 0, false
	}
	
	switch len(args) {
	case 0:
		return callNative0(funcPtr), true
	case 1:
		return callNative1(funcPtr, args[0]), true
	case 2:
		return callNative2(funcPtr, args[0], args[1]), true
	case 3:
		return callNative3(funcPtr, args[0], args[1], args[2]), true
	case 4:
		return callNative4(funcPtr, args[0], args[1], args[2], args[3]), true
	default:
		// 超过 4 个参数，暂不支持
		return 0, false
	}
}
