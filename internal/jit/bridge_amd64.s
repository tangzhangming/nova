//go:build amd64

// bridge_amd64.s - AMD64 平台的汇编桥接
//
// 这些函数负责设置调用约定并调用 JIT 生成的代码
// Windows x64 调用约定：参数通过 RCX, RDX, R8, R9 传递

#include "textflag.h"

// func callNative0(funcPtr uintptr) int64
// 无参数调用
TEXT ·callNative0(SB), NOSPLIT, $0-16
    MOVQ funcPtr+0(FP), AX    // 加载函数指针
    
    // 为 Windows 调用约定预留 shadow space（32 字节）
    SUBQ $40, SP
    
    CALL AX                    // 调用函数
    
    ADDQ $40, SP
    MOVQ AX, ret+8(FP)        // 返回值
    RET

// func callNative1(funcPtr uintptr, arg0 int64) int64
TEXT ·callNative1(SB), NOSPLIT, $0-24
    MOVQ funcPtr+0(FP), AX
    MOVQ arg0+8(FP), CX       // 第一个参数 -> RCX
    
    SUBQ $40, SP
    CALL AX
    ADDQ $40, SP
    
    MOVQ AX, ret+16(FP)
    RET

// func callNative2(funcPtr uintptr, arg0, arg1 int64) int64
TEXT ·callNative2(SB), NOSPLIT, $0-32
    MOVQ funcPtr+0(FP), AX
    MOVQ arg0+8(FP), CX       // 第一个参数 -> RCX
    MOVQ arg1+16(FP), DX      // 第二个参数 -> RDX
    
    SUBQ $40, SP
    CALL AX
    ADDQ $40, SP
    
    MOVQ AX, ret+24(FP)
    RET

// func callNative3(funcPtr uintptr, arg0, arg1, arg2 int64) int64
TEXT ·callNative3(SB), NOSPLIT, $0-40
    MOVQ funcPtr+0(FP), AX
    MOVQ arg0+8(FP), CX
    MOVQ arg1+16(FP), DX
    MOVQ arg2+24(FP), R8      // 第三个参数 -> R8
    
    SUBQ $40, SP
    CALL AX
    ADDQ $40, SP
    
    MOVQ AX, ret+32(FP)
    RET

// func callNative4(funcPtr uintptr, arg0, arg1, arg2, arg3 int64) int64
TEXT ·callNative4(SB), NOSPLIT, $0-48
    MOVQ funcPtr+0(FP), AX
    MOVQ arg0+8(FP), CX
    MOVQ arg1+16(FP), DX
    MOVQ arg2+24(FP), R8
    MOVQ arg3+32(FP), R9      // 第四个参数 -> R9
    
    SUBQ $40, SP
    CALL AX
    ADDQ $40, SP
    
    MOVQ AX, ret+40(FP)
    RET
