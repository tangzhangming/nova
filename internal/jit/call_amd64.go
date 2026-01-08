//go:build amd64

package jit

// callNativeCode 调用编译后的本机代码（通过汇编实现）
// funcPtr: 机器码起始地址
// args: 参数列表（最多4个）
// 返回: 函数返回值
func callNativeCode(funcPtr uintptr, args []int64) int64

// callNativeCodeSimple 简化版本：只有一个参数
func callNativeCodeSimple(funcPtr uintptr, arg0 int64) int64

// callNativeCodeNoArgs 无参数版本
func callNativeCodeNoArgs(funcPtr uintptr) int64

// ExecuteCompiled 执行已编译的函数
// 这是VM调用JIT代码的入口点
func ExecuteCompiled(compiled *CompiledFunction, args []int64) (int64, bool) {
	if compiled == nil || compiled.FuncPtr == 0 {
		return 0, false
	}

	var result int64
	switch len(args) {
	case 0:
		result = callNativeCodeNoArgs(compiled.FuncPtr)
	case 1:
		result = callNativeCodeSimple(compiled.FuncPtr, args[0])
	default:
		result = callNativeCode(compiled.FuncPtr, args)
	}

	return result, true
}

