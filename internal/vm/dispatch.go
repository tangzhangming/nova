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

	// 使用优化的执行循环
	return vm.runLoopOptimized()
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
	return vm.runLoopOptimized()
}

// runLoopOptimized 优化的执行循环
// 使用 switch 语句替代分派表调用，内联高频操作
func (vm *VM) runLoopOptimized() bytecode.Value {
	stack := &vm.stack
	sp := vm.sp // 本地化栈指针，减少内存访问

mainLoop:
	for {
		// 每次迭代获取当前frame（因为函数调用/返回会改变它）
		frame := &vm.frames[vm.fp-1]
		code := frame.chunk.Code
		constants := frame.chunk.Constants
		bp := frame.bp

		// 内部循环 - 在同一个frame内执行，避免重复获取frame
		for frame.ip < len(code) {
			op := bytecode.OpCode(code[frame.ip])
			frame.ip++

			// 使用 switch 替代分派表，Go 编译器可以更好地优化
			switch op {
			// ===== 高频操作内联 =====

			case bytecode.OpLoadLocal:
				// 内联局部变量加载 (Big Endian)
				slot := int(code[frame.ip])<<8 | int(code[frame.ip+1])
				frame.ip += 2
				stack[sp] = stack[bp+slot]
				sp++

			case bytecode.OpStoreLocal:
				// 内联局部变量存储 (Big Endian) - 使用peek不pop
				slot := int(code[frame.ip])<<8 | int(code[frame.ip+1])
				frame.ip += 2
				stack[bp+slot] = stack[sp-1]

			case bytecode.OpAdd:
				// 内联加法（整数快速路径，完全内联类型检查）
				b := stack[sp-1]
				a := stack[sp-2]
				if a.Type() == bytecode.ValInt && b.Type() == bytecode.ValInt {
					// 完全内联整数加法
					stack[sp-2] = bytecode.NewInt(int64(a.Raw()) + int64(b.Raw()))
				} else {
					stack[sp-2] = Helper_Add(a, b)
				}
				sp--

			case bytecode.OpSub:
				// 内联减法（整数快速路径）
				b := stack[sp-1]
				a := stack[sp-2]
				if a.Type() == bytecode.ValInt && b.Type() == bytecode.ValInt {
					stack[sp-2] = bytecode.NewInt(int64(a.Raw()) - int64(b.Raw()))
				} else {
					stack[sp-2] = Helper_Sub(a, b)
				}
				sp--

			case bytecode.OpMul:
				// 内联乘法（整数快速路径）
				b := stack[sp-1]
				a := stack[sp-2]
				if a.Type() == bytecode.ValInt && b.Type() == bytecode.ValInt {
					stack[sp-2] = bytecode.NewInt(int64(a.Raw()) * int64(b.Raw()))
				} else {
					stack[sp-2] = Helper_Mul(a, b)
				}
				sp--

			case bytecode.OpDiv:
				// 内联除法
				b := stack[sp-1]
				a := stack[sp-2]
				if a.Type() == bytecode.ValInt && b.Type() == bytecode.ValInt {
					bv := int64(b.Raw())
					if bv != 0 {
						stack[sp-2] = bytecode.NewInt(int64(a.Raw()) / bv)
					} else {
						stack[sp-2] = bytecode.NullValue
					}
				} else {
					stack[sp-2] = Helper_Div(a, b)
				}
				sp--

			case bytecode.OpMod:
				// 内联取模
				b := stack[sp-1]
				a := stack[sp-2]
				bi := int64(b.Raw())
				if bi != 0 {
					stack[sp-2] = bytecode.NewInt(int64(a.Raw()) % bi)
				} else {
					stack[sp-2] = bytecode.NullValue
				}
				sp--

			case bytecode.OpLt:
				// 内联小于比较（整数快速路径）
				b := stack[sp-1]
				a := stack[sp-2]
				if a.Type() == bytecode.ValInt && b.Type() == bytecode.ValInt {
					stack[sp-2] = bytecode.NewBool(int64(a.Raw()) < int64(b.Raw()))
				} else {
					stack[sp-2] = bytecode.NewBool(a.AsFloat() < b.AsFloat())
				}
				sp--

			case bytecode.OpLe:
				// 内联小于等于比较
				b := stack[sp-1]
				a := stack[sp-2]
				if a.Type() == bytecode.ValInt && b.Type() == bytecode.ValInt {
					stack[sp-2] = bytecode.NewBool(int64(a.Raw()) <= int64(b.Raw()))
				} else {
					stack[sp-2] = bytecode.NewBool(a.AsFloat() <= b.AsFloat())
				}
				sp--

			case bytecode.OpGt:
				// 内联大于比较
				b := stack[sp-1]
				a := stack[sp-2]
				if a.Type() == bytecode.ValInt && b.Type() == bytecode.ValInt {
					stack[sp-2] = bytecode.NewBool(int64(a.Raw()) > int64(b.Raw()))
				} else {
					stack[sp-2] = bytecode.NewBool(a.AsFloat() > b.AsFloat())
				}
				sp--

			case bytecode.OpGe:
				// 内联大于等于比较
				b := stack[sp-1]
				a := stack[sp-2]
				if a.Type() == bytecode.ValInt && b.Type() == bytecode.ValInt {
					stack[sp-2] = bytecode.NewBool(int64(a.Raw()) >= int64(b.Raw()))
				} else {
					stack[sp-2] = bytecode.NewBool(a.AsFloat() >= b.AsFloat())
				}
				sp--

			case bytecode.OpPush:
				// 内联常量加载 (Big Endian)
				idx := int(code[frame.ip])<<8 | int(code[frame.ip+1])
				frame.ip += 2
				stack[sp] = constants[idx]
				sp++

			case bytecode.OpPop:
				// 内联弹栈
				sp--

			case bytecode.OpDup:
				// 内联复制栈顶
				stack[sp] = stack[sp-1]
				sp++

			case bytecode.OpJump:
				// 内联无条件跳转 (Big Endian)
				offset := int(code[frame.ip])<<8 | int(code[frame.ip+1])
				frame.ip += 2 + offset

			case bytecode.OpJumpIfFalse:
				// 内联条件跳转 (Big Endian) - 使用peek不pop
				offset := int(code[frame.ip])<<8 | int(code[frame.ip+1])
				frame.ip += 2
				if !stack[sp-1].IsTruthy() {
					frame.ip += offset
				}

			case bytecode.OpJumpIfTrue:
				// 内联条件跳转 (Big Endian) - 使用peek不pop
				offset := int(code[frame.ip])<<8 | int(code[frame.ip+1])
				frame.ip += 2
				if stack[sp-1].IsTruthy() {
					frame.ip += offset
				}

			case bytecode.OpLoop:
				// 内联循环跳转（向后跳，Big Endian）
				offset := int(code[frame.ip])<<8 | int(code[frame.ip+1])
				frame.ip += 2
				frame.ip -= offset

			case bytecode.OpZero:
				// 内联压入0
				stack[sp] = bytecode.ZeroValue
				sp++

			case bytecode.OpOne:
				// 内联压入1
				stack[sp] = bytecode.OneValue
				sp++

			case bytecode.OpNull:
				// 内联压入null
				stack[sp] = bytecode.NullValue
				sp++

			case bytecode.OpTrue:
				// 内联压入true
				stack[sp] = bytecode.TrueValue
				sp++

			case bytecode.OpFalse:
				// 内联压入false
				stack[sp] = bytecode.FalseValue
				sp++

			case bytecode.OpEq:
				// 内联相等比较（整数快速路径）
				b := stack[sp-1]
				a := stack[sp-2]
				if a.Type() == bytecode.ValInt && b.Type() == bytecode.ValInt {
					stack[sp-2] = bytecode.NewBool(a.Raw() == b.Raw())
				} else {
					stack[sp-2] = bytecode.NewBool(a.Equals(b))
				}
				sp--

			case bytecode.OpNe:
				// 内联不等比较（整数快速路径）
				b := stack[sp-1]
				a := stack[sp-2]
				if a.Type() == bytecode.ValInt && b.Type() == bytecode.ValInt {
					stack[sp-2] = bytecode.NewBool(a.Raw() != b.Raw())
				} else {
					stack[sp-2] = bytecode.NewBool(!a.Equals(b))
				}
				sp--

			case bytecode.OpCall:
				// 内联函数调用（简单情况）
				argCount := int(code[frame.ip])
				frame.ip++
				callee := stack[sp-argCount-1]

				if callee.IsFunc() {
					fn := callee.AsFunc()
					if fn != nil && !fn.IsBuiltin {
						// 处理参数数量不匹配
						if argCount < fn.MinArity {
							for i := argCount; i < fn.Arity; i++ {
								defIdx := i - fn.MinArity
								if defIdx >= 0 && defIdx < len(fn.DefaultValues) {
									stack[sp] = fn.DefaultValues[defIdx]
								} else {
									stack[sp] = bytecode.NullValue
								}
								sp++
							}
							argCount = fn.Arity
						}
						// 保存当前sp到vm
						vm.sp = sp
						// 压入新帧
						newBp := sp - argCount
						vm.pushFrame(fn, newBp)
						// 跳转到外层循环获取新frame
						continue mainLoop
					}
					// 内置函数处理
					if fn != nil && fn.IsBuiltin && fn.BuiltinFn != nil {
						args := make([]bytecode.Value, argCount)
						for i := argCount - 1; i >= 0; i-- {
							sp--
							args[i] = stack[sp]
						}
						sp-- // 弹出函数本身
						result := fn.BuiltinFn(args)
						stack[sp] = result
						sp++
						continue
					}
				}
				// 闭包或其他复杂情况回退到分派表
				// 需要恢复ip，因为我们已经读取了argCount
				frame.ip--
				vm.sp = sp
				dispatchTable[byte(op)](vm)
				sp = vm.sp
				if vm.hasError {
					return bytecode.NullValue
				}
				continue mainLoop

			case bytecode.OpReturn:
				// 内联函数返回
				result := stack[sp-1]
				sp--

				// 弹出当前帧
				vm.fp--
				if vm.fp == 0 {
					// 主函数返回
					vm.sp = sp
					return result
				}

				// 清理栈上的局部变量和参数
				sp = bp
				// 弹出被调用者
				if !frame.isStaticCall && sp > 0 {
					sp--
				}
				// 压入返回值
				stack[sp] = result
				sp++
				// 跳转到外层循环获取新frame
				continue mainLoop

			case bytecode.OpReturnNull:
				// 内联null返回
				vm.fp--
				if vm.fp == 0 {
					vm.sp = sp
					return bytecode.NullValue
				}
				sp = bp
				if !frame.isStaticCall && sp > 0 {
					sp--
				}
				stack[sp] = bytecode.NullValue
				sp++
				continue mainLoop

			// ===== 其他操作使用分派表 =====
			default:
				vm.sp = sp
				dispatchTable[byte(op)](vm)
				sp = vm.sp
				if vm.hasError {
					return bytecode.NullValue
				}
				// 检查是否需要重新获取frame (函数调用/返回会改变)
				if vm.fp == 0 {
					break mainLoop
				}
				continue mainLoop
			}
		}

		// 当前frame执行完毕
		break
	}

	vm.sp = sp
	if sp > 0 {
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
