//go:build !amd64

package jit

// 非amd64平台的存根实现

// ExecuteCompiled 执行已编译的函数（非amd64平台不支持）
func ExecuteCompiled(compiled *CompiledFunction, args []int64) (int64, bool) {
	// 非amd64平台暂不支持JIT执行
	return 0, false
}

