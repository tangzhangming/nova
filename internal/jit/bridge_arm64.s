//go:build arm64

// bridge_arm64.s - ARM64 平台的汇编桥接
//
// AAPCS64 调用约定：参数通过 X0-X7 传递，返回值在 X0

#include "textflag.h"

// func callNative0(funcPtr uintptr) int64
TEXT ·callNative0(SB), NOSPLIT, $0-16
    MOVD funcPtr+0(FP), R16   // 加载函数指针到临时寄存器
    BLR R16                    // 调用函数
    MOVD R0, ret+8(FP)        // 返回值
    RET

// func callNative1(funcPtr uintptr, arg0 int64) int64
TEXT ·callNative1(SB), NOSPLIT, $0-24
    MOVD funcPtr+0(FP), R16
    MOVD arg0+8(FP), R0       // 第一个参数
    BLR R16
    MOVD R0, ret+16(FP)
    RET

// func callNative2(funcPtr uintptr, arg0, arg1 int64) int64
TEXT ·callNative2(SB), NOSPLIT, $0-32
    MOVD funcPtr+0(FP), R16
    MOVD arg0+8(FP), R0
    MOVD arg1+16(FP), R1      // 第二个参数
    BLR R16
    MOVD R0, ret+24(FP)
    RET

// func callNative3(funcPtr uintptr, arg0, arg1, arg2 int64) int64
TEXT ·callNative3(SB), NOSPLIT, $0-40
    MOVD funcPtr+0(FP), R16
    MOVD arg0+8(FP), R0
    MOVD arg1+16(FP), R1
    MOVD arg2+24(FP), R2      // 第三个参数
    BLR R16
    MOVD R0, ret+32(FP)
    RET

// func callNative4(funcPtr uintptr, arg0, arg1, arg2, arg3 int64) int64
TEXT ·callNative4(SB), NOSPLIT, $0-48
    MOVD funcPtr+0(FP), R16
    MOVD arg0+8(FP), R0
    MOVD arg1+16(FP), R1
    MOVD arg2+24(FP), R2
    MOVD arg3+32(FP), R3      // 第四个参数
    BLR R16
    MOVD R0, ret+40(FP)
    RET
