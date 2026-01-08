//go:build arm64

// bridge_arm64.go - ARM64 平台的 JIT 桥接

package jit

// callNative0 调用无参数的 JIT 函数
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
		return 0, false
	}
}
