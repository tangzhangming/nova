package bytecode

import (
	"fmt"
)

// ============================================================================
// B6. 操作数栈深度限制检查
// ============================================================================

// StackChecker 栈深度检查器
type StackChecker struct {
	chunk        *Chunk
	maxStackSize int // 编译时计算的最大栈深度
	errors       []string
}

// StackCheckResult 栈检查结果
type StackCheckResult struct {
	MaxDepth   int      // 最大栈深度
	IsValid    bool     // 是否有效
	Errors     []string // 错误信息
}

// DefaultMaxStackDepth 默认最大栈深度
const DefaultMaxStackDepth = 1024

// NewStackChecker 创建栈检查器
func NewStackChecker(chunk *Chunk) *StackChecker {
	return &StackChecker{
		chunk:        chunk,
		maxStackSize: 0,
		errors:       nil,
	}
}

// Check 执行栈深度检查
// 返回最大栈深度，如果超过限制返回错误
func (sc *StackChecker) Check(maxAllowed int) StackCheckResult {
	if maxAllowed <= 0 {
		maxAllowed = DefaultMaxStackDepth
	}

	sc.errors = nil
	sc.maxStackSize = 0

	// 使用数据流分析计算每个位置的栈深度
	code := sc.chunk.Code
	if len(code) == 0 {
		return StackCheckResult{MaxDepth: 0, IsValid: true}
	}

	// 每个位置的栈深度（-1 表示未访问）
	depths := make([]int, len(code))
	for i := range depths {
		depths[i] = -1
	}

	// 工作列表：(位置, 当前栈深度)
	type workItem struct {
		pos   int
		depth int
	}
	worklist := []workItem{{0, 0}}

	for len(worklist) > 0 {
		item := worklist[len(worklist)-1]
		worklist = worklist[:len(worklist)-1]

		pos := item.pos
		depth := item.depth

		// 遍历指令
		for pos < len(code) {
			// 检查是否已访问且深度一致
			if depths[pos] >= 0 {
				if depths[pos] != depth {
					sc.errors = append(sc.errors,
						fmt.Sprintf("栈深度不一致：位置 %d 有两个不同的深度 %d 和 %d",
							pos, depths[pos], depth))
				}
				break
			}

			depths[pos] = depth

			op := OpCode(code[pos])
			effect := sc.stackEffect(op, pos)
			size := sc.instructionSize(op, pos)

			// 计算新的栈深度
			newDepth := depth + effect
			if newDepth < 0 {
				sc.errors = append(sc.errors,
					fmt.Sprintf("栈下溢：位置 %d，指令 %s，深度从 %d 变为 %d",
						pos, op, depth, newDepth))
				newDepth = 0
			}

			if newDepth > sc.maxStackSize {
				sc.maxStackSize = newDepth
			}

			if newDepth > maxAllowed {
				sc.errors = append(sc.errors,
					fmt.Sprintf("栈溢出：位置 %d，深度 %d 超过限制 %d",
						pos, newDepth, maxAllowed))
			}

			// 处理控制流
			switch op {
			case OpJump:
				// 无条件跳转
				if pos+2 < len(code) {
					offset := int16(code[pos+1])<<8 | int16(code[pos+2])
					target := pos + 3 + int(offset)
					if target >= 0 && target < len(code) {
						worklist = append(worklist, workItem{target, newDepth})
					}
				}
				pos = len(code) // 终止当前路径

			case OpJumpIfFalse, OpJumpIfTrue:
				// 条件跳转：两个分支
				if pos+2 < len(code) {
					offset := int16(code[pos+1])<<8 | int16(code[pos+2])
					target := pos + 3 + int(offset)
					if target >= 0 && target < len(code) {
						worklist = append(worklist, workItem{target, newDepth})
					}
				}
				depth = newDepth
				pos += size

			case OpLoop:
				// 循环跳转
				if pos+2 < len(code) {
					offset := uint16(code[pos+1])<<8 | uint16(code[pos+2])
					target := pos + 3 - int(offset)
					if target >= 0 && target < len(code) {
						worklist = append(worklist, workItem{target, newDepth})
					}
				}
				depth = newDepth
				pos += size

			case OpReturn, OpReturnNull, OpHalt, OpThrow:
				// 终止指令
				pos = len(code)

			default:
				depth = newDepth
				pos += size
			}
		}
	}

	return StackCheckResult{
		MaxDepth: sc.maxStackSize,
		IsValid:  len(sc.errors) == 0,
		Errors:   sc.errors,
	}
}

// stackEffect 计算指令对栈的影响（正数表示压入，负数表示弹出）
func (sc *StackChecker) stackEffect(op OpCode, offset int) int {
	switch op {
	// 压入操作：+1
	case OpPush, OpLoadLocal, OpLoadGlobal,
		OpNull, OpTrue, OpFalse, OpZero, OpOne,
		OpDup, OpIterKey, OpIterValue:
		return 1

	// 弹出操作：-1
	case OpPop, OpDebugPrint:
		return -1

	// 交换：0
	case OpSwap:
		return 0

	// 存储操作：-1（弹出值存储）
	case OpStoreLocal, OpStoreGlobal:
		return -1

	// 二元运算：弹出2个压入1个 = -1
	case OpAdd, OpSub, OpMul, OpDiv, OpMod,
		OpEq, OpNe, OpLt, OpLe, OpGt, OpGe,
		OpAnd, OpOr,
		OpBitAnd, OpBitOr, OpBitXor, OpShl, OpShr,
		OpConcat:
		return -1

	// 一元运算：弹出1个压入1个 = 0
	case OpNeg, OpNot, OpBitNot:
		return 0

	// 跳转：0
	case OpJump, OpLoop:
		return 0

	// 条件跳转：弹出条件 = -1
	case OpJumpIfFalse, OpJumpIfTrue:
		return -1

	// 函数调用：弹出函数和参数，压入返回值
	case OpCall, OpTailCall:
		if offset+1 < len(sc.chunk.Code) {
			argCount := int(sc.chunk.Code[offset+1])
			return -argCount // 弹出参数，函数已在栈上，返回值替换
		}
		return 0

	// 返回
	case OpReturn:
		return -1 // 弹出返回值
	case OpReturnNull:
		return 0

	// 对象操作
	case OpNewObject:
		return 1 // 压入新对象

	case OpGetField:
		return 0 // 弹出对象，压入字段值

	case OpSetField:
		return -2 // 弹出对象和值

	case OpCallMethod:
		if offset+3 < len(sc.chunk.Code) {
			argCount := int(sc.chunk.Code[offset+3])
			return -argCount // 弹出接收者和参数，压入返回值
		}
		return 0

	// 静态访问
	case OpGetStatic:
		return 1 // 压入静态成员值

	case OpSetStatic:
		return -1 // 弹出值

	case OpCallStatic:
		if offset+5 < len(sc.chunk.Code) {
			argCount := int(sc.chunk.Code[offset+5])
			return 1 - argCount // 压入返回值，弹出参数
		}
		return 0

	// 数组操作
	case OpNewArray:
		if offset+2 < len(sc.chunk.Code) {
			count := int(sc.chunk.Code[offset+1])<<8 | int(sc.chunk.Code[offset+2])
			return 1 - count // 弹出元素，压入数组
		}
		return 1

	case OpNewFixedArray:
		if offset+4 < len(sc.chunk.Code) {
			initLen := int(sc.chunk.Code[offset+3])<<8 | int(sc.chunk.Code[offset+4])
			return 1 - initLen // 弹出初始元素，压入数组
		}
		return 1

	case OpArrayGet, OpArrayGetUnchecked:
		return -1 // 弹出数组和索引，压入值

	case OpArraySet, OpArraySetUnchecked:
		return -3 // 弹出数组、索引和值

	case OpArrayLen:
		return 0 // 弹出数组，压入长度

	case OpArrayPush:
		return -2 // 弹出数组和值

	case OpArrayHas:
		return -1 // 弹出数组和值，压入布尔

	// Map 操作
	case OpNewMap:
		if offset+2 < len(sc.chunk.Code) {
			count := int(sc.chunk.Code[offset+1])<<8 | int(sc.chunk.Code[offset+2])
			return 1 - count*2 // 弹出键值对，压入 map
		}
		return 1

	case OpMapGet:
		return -1 // 弹出 map 和 key，压入值

	case OpMapSet:
		return -3 // 弹出 map、key 和值

	case OpMapHas:
		return -1 // 弹出 map 和 key，压入布尔

	case OpMapLen:
		return 0 // 弹出 map，压入长度

	// SuperArray
	case OpSuperArrayNew:
		// 可变长度
		if offset+2 < len(sc.chunk.Code) {
			count := int(sc.chunk.Code[offset+1])<<8 | int(sc.chunk.Code[offset+2])
			return 1 - count*2 // 大约
		}
		return 1

	case OpSuperArrayGet:
		return -1

	case OpSuperArraySet:
		return -2

	// 迭代器
	case OpIterInit:
		return 0 // 弹出可迭代对象，压入迭代器

	case OpIterNext:
		return 1 // 压入布尔

	// 字节数组
	case OpNewBytes:
		if offset+2 < len(sc.chunk.Code) {
			count := int(sc.chunk.Code[offset+1])<<8 | int(sc.chunk.Code[offset+2])
			return 1 - count
		}
		return 1

	case OpBytesGet:
		return -1

	case OpBytesSet:
		return -3

	case OpBytesLen:
		return 0

	case OpBytesSlice:
		return -2 // 弹出 bytes、start、end，压入新 bytes

	case OpBytesConcat:
		return -1 // 弹出两个 bytes，压入新 bytes

	// 类型操作
	case OpCheckType:
		return 0 // 检查栈顶类型

	case OpCast, OpCastSafe:
		return 0 // 转换栈顶值

	// 异常处理
	case OpThrow:
		return -1 // 弹出异常对象

	case OpEnterTry, OpLeaveTry, OpEnterCatch, OpEnterFinally, OpLeaveFinally, OpRethrow:
		return 0

	// 闭包
	case OpClosure:
		return 1 // 压入闭包

	// 字符串构建器
	case OpStringBuilderNew:
		return 1

	case OpStringBuilderAdd:
		return -1 // 弹出值添加到构建器

	case OpStringBuilderBuild:
		return 0 // 弹出构建器，压入字符串

	// 销毁
	case OpUnset:
		return -1

	case OpHalt:
		return 0

	default:
		return 0
	}
}

// instructionSize 获取指令大小
func (sc *StackChecker) instructionSize(op OpCode, offset int) int {
	switch op {
	case OpPush, OpLoadLocal, OpStoreLocal, OpLoadGlobal, OpStoreGlobal,
		OpNewObject, OpGetField, OpSetField, OpNewArray, OpNewMap,
		OpCheckType, OpCast, OpCastSafe, OpClosure, OpEnterCatch,
		OpNewBytes:
		return 3

	case OpNewFixedArray:
		return 5

	case OpJump, OpJumpIfFalse, OpJumpIfTrue, OpLoop:
		return 3

	case OpCall, OpTailCall:
		return 2

	case OpCallMethod:
		return 4

	case OpGetStatic, OpSetStatic:
		return 5

	case OpCallStatic:
		return 6

	case OpSuperArrayNew:
		if offset+2 < len(sc.chunk.Code) {
			count := int(sc.chunk.Code[offset+1])<<8 | int(sc.chunk.Code[offset+2])
			return 3 + count
		}
		return 3

	case OpEnterTry:
		if offset+1 < len(sc.chunk.Code) {
			catchCount := int(sc.chunk.Code[offset+1])
			return 4 + catchCount*4
		}
		return 4

	default:
		return 1
	}
}

// CheckChunk 检查字节码块的栈深度（便捷函数）
func CheckChunk(chunk *Chunk, maxDepth int) StackCheckResult {
	checker := NewStackChecker(chunk)
	return checker.Check(maxDepth)
}

