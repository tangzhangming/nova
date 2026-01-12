package vm

import (
	"github.com/tangzhangming/nova/internal/bytecode"
)

// ============================================================================
// 分派表
// ============================================================================

// OpHandler 操作码处理函数类型
type OpHandler func(*VM)

// dispatchTable 分派表 (256 个操作码槽位)
var dispatchTable [256]OpHandler

func init() {
	// 初始化所有槽位为无效操作码处理器
	for i := range dispatchTable {
		dispatchTable[i] = opInvalid
	}

	// 栈操作
	dispatchTable[bytecode.OpPush] = opPush
	dispatchTable[bytecode.OpPop] = opPop
	dispatchTable[bytecode.OpDup] = opDup

	// 常量加载
	dispatchTable[bytecode.OpNull] = opNull
	dispatchTable[bytecode.OpTrue] = opTrue
	dispatchTable[bytecode.OpFalse] = opFalse
	dispatchTable[bytecode.OpZero] = opZero
	dispatchTable[bytecode.OpOne] = opOne

	// 算术运算
	dispatchTable[bytecode.OpAdd] = opAdd
	dispatchTable[bytecode.OpSub] = opSub
	dispatchTable[bytecode.OpMul] = opMul
	dispatchTable[bytecode.OpDiv] = opDiv
	dispatchTable[bytecode.OpMod] = opMod
	dispatchTable[bytecode.OpNeg] = opNeg

	// 位运算
	dispatchTable[bytecode.OpBitAnd] = opBand
	dispatchTable[bytecode.OpBitOr] = opBor
	dispatchTable[bytecode.OpBitXor] = opBxor
	dispatchTable[bytecode.OpBitNot] = opBnot
	dispatchTable[bytecode.OpShl] = opShl
	dispatchTable[bytecode.OpShr] = opShr

	// 字符串操作
	dispatchTable[bytecode.OpConcat] = opConcat

	// 比较运算
	dispatchTable[bytecode.OpEq] = opEq
	dispatchTable[bytecode.OpNe] = opNe
	dispatchTable[bytecode.OpLt] = opLt
	dispatchTable[bytecode.OpLe] = opLe
	dispatchTable[bytecode.OpGt] = opGt
	dispatchTable[bytecode.OpGe] = opGe

	// 逻辑运算
	dispatchTable[bytecode.OpNot] = opNot
	dispatchTable[bytecode.OpAnd] = opAnd
	dispatchTable[bytecode.OpOr] = opOr

	// 变量操作
	dispatchTable[bytecode.OpLoadLocal] = opLoadLocal
	dispatchTable[bytecode.OpStoreLocal] = opStoreLocal
	dispatchTable[bytecode.OpLoadGlobal] = opLoadGlobal
	dispatchTable[bytecode.OpStoreGlobal] = opStoreGlobal

	// 跳转
	dispatchTable[bytecode.OpJump] = opJump
	dispatchTable[bytecode.OpJumpIfFalse] = opJumpIfFalse
	dispatchTable[bytecode.OpJumpIfTrue] = opJumpIfTrue
	dispatchTable[bytecode.OpLoop] = opLoop

	// 函数调用
	dispatchTable[bytecode.OpCall] = opCall
	dispatchTable[bytecode.OpCallStatic] = opCallStatic
	dispatchTable[bytecode.OpReturn] = opReturn
	dispatchTable[bytecode.OpReturnNull] = opReturnNull
	dispatchTable[bytecode.OpClosure] = opClosure

	// 对象操作
	dispatchTable[bytecode.OpNewObject] = opNewInstance
	dispatchTable[bytecode.OpGetField] = opGetField
	dispatchTable[bytecode.OpSetField] = opSetField
	dispatchTable[bytecode.OpCallMethod] = opInvoke

	// 数组操作
	dispatchTable[bytecode.OpNewArray] = opNewArray
	dispatchTable[bytecode.OpArrayGet] = opArrayGet
	dispatchTable[bytecode.OpArraySet] = opArraySet
	dispatchTable[bytecode.OpArrayLen] = opArrayLen

	// SuperArray 操作
	dispatchTable[bytecode.OpSuperArrayNew] = opSuperArrayNew
	dispatchTable[bytecode.OpSuperArrayGet] = opSuperArrayGet
	dispatchTable[bytecode.OpSuperArraySet] = opSuperArraySet

	// 迭代器
	dispatchTable[bytecode.OpIterInit] = opIterNew
	dispatchTable[bytecode.OpIterNext] = opIterNext
	dispatchTable[bytecode.OpIterKey] = opIterKey
	dispatchTable[bytecode.OpIterValue] = opIterValue

	// 类型转换
	dispatchTable[bytecode.OpCast] = opCast
	dispatchTable[bytecode.OpCastSafe] = opCastSafe

	// 其他
	dispatchTable[bytecode.OpDebugPrint] = opPrint
	dispatchTable[bytecode.OpHalt] = opHalt
}

// ============================================================================
// 执行引擎
// ============================================================================

// Run 执行函数
func (vm *VM) Run(fn *bytecode.Function) bytecode.Value {
	// 设置初始帧
	vm.pushFrame(fn, 0)

	// 主执行循环
	for {
		if vm.hasError {
			return bytecode.NullValue
		}

		frame := vm.currentFrame()
		if frame.ip >= len(frame.chunk.Code) {
			break
		}

		op := frame.chunk.Code[frame.ip]
		frame.ip++
		vm.stats.InstructionsExecuted++

		// 通过分派表调用处理器
		dispatchTable[op](vm)

		// 检查是否执行完毕
		if vm.fp == 0 {
			break
		}
	}

	// 返回栈顶值
	if vm.sp > 0 {
		return vm.pop()
	}
	return bytecode.NullValue
}

// RunClosure 执行闭包
func (vm *VM) RunClosure(closure *bytecode.Closure, args []bytecode.Value) bytecode.Value {
	// 设置参数
	for _, arg := range args {
		vm.push(arg)
	}

	// 设置闭包帧
	vm.pushClosureFrame(closure, vm.sp-len(args))

	// 执行
	return vm.runLoop()
}

// runLoop 内部执行循环
func (vm *VM) runLoop() bytecode.Value {
	for {
		if vm.hasError {
			return bytecode.NullValue
		}

		frame := vm.currentFrame()
		if frame.ip >= len(frame.chunk.Code) {
			break
		}

		op := frame.chunk.Code[frame.ip]
		frame.ip++
		vm.stats.InstructionsExecuted++

		dispatchTable[op](vm)

		if vm.fp == 0 {
			break
		}
	}

	if vm.sp > 0 {
		return vm.pop()
	}
	return bytecode.NullValue
}

// ============================================================================
// 辅助方法
// ============================================================================

// opInvalid 无效操作码
func opInvalid(vm *VM) {
	frame := vm.currentFrame()
	op := frame.chunk.Code[frame.ip-1]
	vm.runtimeError("invalid opcode: %d", op)
}

// opNop 空操作
func opNop(vm *VM) {
	// 什么都不做
}

// opHalt 停止执行
func opHalt(vm *VM) {
	vm.fp = 0 // 清空调用栈
}
