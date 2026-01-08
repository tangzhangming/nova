//go:build amd64

#include "textflag.h"

// func callNativeCode(funcPtr uintptr, args []int64) int64
// 调用编译后的本机代码
// Windows x64 调用约定: RCX, RDX, R8, R9
// System V AMD64 调用约定: RDI, RSI, RDX, RCX, R8, R9
TEXT ·callNativeCode(SB), NOSPLIT, $0-40
    // 加载函数指针
    MOVQ funcPtr+0(FP), AX
    
    // 加载参数切片
    MOVQ args+8(FP), BX       // args.ptr
    MOVQ args+16(FP), CX      // args.len
    
    // 如果没有参数，直接调用
    CMPQ CX, $0
    JE   call_func
    
    // 加载第一个参数到 RCX (Windows) / RDI (Linux)
    // 这里使用Windows约定，因为我们在Windows上运行
    MOVQ 0(BX), CX
    
    // 检查是否有第二个参数
    CMPQ args+16(FP), $1
    JLE  call_func
    MOVQ 8(BX), DX
    
    // 检查是否有第三个参数
    CMPQ args+16(FP), $2
    JLE  call_func
    MOVQ 16(BX), R8
    
    // 检查是否有第四个参数
    CMPQ args+16(FP), $3
    JLE  call_func
    MOVQ 24(BX), R9

call_func:
    // 调用函数
    CALL AX
    
    // 返回值在 RAX
    MOVQ AX, ret+32(FP)
    RET

// func callNativeCodeSimple(funcPtr uintptr, arg0 int64) int64
// 简化版本：只有一个参数
TEXT ·callNativeCodeSimple(SB), NOSPLIT, $0-24
    MOVQ funcPtr+0(FP), AX
    MOVQ arg0+8(FP), CX       // Windows: RCX
    CALL AX
    MOVQ AX, ret+16(FP)
    RET

// func callNativeCodeNoArgs(funcPtr uintptr) int64
// 无参数版本
TEXT ·callNativeCodeNoArgs(SB), NOSPLIT, $0-16
    MOVQ funcPtr+0(FP), AX
    CALL AX
    MOVQ AX, ret+8(FP)
    RET

