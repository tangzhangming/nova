// bridge.go - VM-JIT 桥接
//
// 本文件提供了 VM 和 JIT 编译代码之间的桥接接口。
// 主要功能：
// 1. 将 Sola 运行时值转换为 JIT 期望的格式
// 2. 调用 JIT 编译的函数
// 3. 将 JIT 返回值转换回 Sola 运行时值
//
// 调用约定：
// - JIT 函数接收 int64 参数并返回 int64
// - 对于 float 类型，使用 IEEE 754 位表示传递
// - 复杂类型（对象、数组等）暂不支持，会回退到解释器

package jit

import (
	"math"

	"github.com/tangzhangming/nova/internal/bytecode"
)

// ExecuteResult JIT 执行结果
type ExecuteResult struct {
	Value   bytecode.Value // 返回值
	Success bool           // 是否成功执行
}

// CanJIT 检查函数是否可以被 JIT 编译
// 只有满足特定条件的函数才能被 JIT 编译
func CanJIT(fn *bytecode.Function) bool {
	if fn == nil || fn.Chunk == nil {
		return false
	}
	
	// 不支持可变参数函数
	if fn.IsVariadic {
		return false
	}
	
	// 不支持闭包
	if fn.UpvalueCount > 0 {
		return false
	}
	
	// 检查是否包含不支持的操作码
	code := fn.Chunk.Code
	ip := 0
	for ip < len(code) {
		op := bytecode.OpCode(code[ip])
		
		// 不支持的操作（复杂运行时操作需要回退到解释器）
		switch op {
		case bytecode.OpCall, bytecode.OpTailCall,
			bytecode.OpCallMethod, bytecode.OpCallStatic,
			bytecode.OpNewObject, bytecode.OpGetField, bytecode.OpSetField,
			bytecode.OpNewArray, // 创建数组需要内存分配，暂不支持
			bytecode.OpNewMap, bytecode.OpMapGet, bytecode.OpMapSet,
			bytecode.OpClosure,
			bytecode.OpThrow, bytecode.OpEnterTry, bytecode.OpLeaveTry,
			bytecode.OpConcat,
			bytecode.OpLoadGlobal, bytecode.OpStoreGlobal:
			return false
		// 数组操作现在已支持（通过运行时辅助函数）
		case bytecode.OpArrayGet, bytecode.OpArraySet, bytecode.OpArrayLen:
			// 已实现支持
		// 控制流指令现在已支持
		case bytecode.OpLoop, bytecode.OpJump, bytecode.OpJumpIfFalse, bytecode.OpJumpIfTrue:
			// 已实现支持
		}
		
		ip += instrSize(op, ip, code)
	}
	
	return true
}

// instrSize 获取指令大小
func instrSize(op bytecode.OpCode, ip int, code []byte) int {
	switch op {
	case bytecode.OpPush, bytecode.OpLoadLocal, bytecode.OpStoreLocal,
		bytecode.OpLoadGlobal, bytecode.OpStoreGlobal,
		bytecode.OpNewObject, bytecode.OpGetField, bytecode.OpSetField,
		bytecode.OpNewArray, bytecode.OpNewMap,
		bytecode.OpCheckType, bytecode.OpCast, bytecode.OpCastSafe,
		bytecode.OpSuperArrayNew, bytecode.OpClosure:
		return 3
	case bytecode.OpNewFixedArray:
		return 5
	case bytecode.OpJump, bytecode.OpJumpIfFalse, bytecode.OpJumpIfTrue, bytecode.OpLoop:
		return 3
	case bytecode.OpCall, bytecode.OpTailCall:
		return 2
	case bytecode.OpCallMethod:
		return 4
	case bytecode.OpGetStatic, bytecode.OpSetStatic:
		return 5
	case bytecode.OpCallStatic:
		return 6
	case bytecode.OpEnterTry:
		if ip+1 < len(code) {
			catchCount := int(code[ip+1])
			return 4 + catchCount*4
		}
		return 4
	case bytecode.OpEnterCatch:
		return 3
	default:
		return 1
	}
}

// ============================================================================
// 值转换函数
// ============================================================================

// ValueToInt64 将 Sola 值转换为 int64
// 对于 float 类型，使用 IEEE 754 双精度位表示以保持精度
func ValueToInt64(v bytecode.Value) int64 {
	switch v.Type {
	case bytecode.ValInt:
		return v.AsInt()
	case bytecode.ValFloat:
		// 使用 IEEE 754 位表示保持精度
		return int64(math.Float64bits(v.AsFloat()))
	case bytecode.ValBool:
		if v.AsBool() {
			return 1
		}
		return 0
	case bytecode.ValNull:
		return 0
	default:
		return 0
	}
}

// Int64ToValue 将 int64 转换回 Sola 值
// 默认转换为整数类型
func Int64ToValue(v int64) bytecode.Value {
	return bytecode.NewInt(v)
}

// Int64ToFloatValue 将 int64（IEEE 754 位表示）转换回 float 值
func Int64ToFloatValue(v int64) bytecode.Value {
	return bytecode.NewFloat(math.Float64frombits(uint64(v)))
}

// FloatBitsToInt64 将 float64 转换为 IEEE 754 位表示的 int64
func FloatBitsToInt64(f float64) int64 {
	return int64(math.Float64bits(f))
}

// Int64ToFloatBits 将 IEEE 754 位表示的 int64 转换为 float64
func Int64ToFloatBits(v int64) float64 {
	return math.Float64frombits(uint64(v))
}
