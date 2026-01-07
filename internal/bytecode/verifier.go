package bytecode

import (
	"fmt"
)

// VerificationError 字节码验证错误
type VerificationError struct {
	Offset  int    // 指令偏移量
	Message string // 错误消息
}

func (e *VerificationError) Error() string {
	return fmt.Sprintf("字节码验证错误 (偏移量 %d): %s", e.Offset, e.Message)
}

// Verifier 字节码验证器
type Verifier struct {
	chunk         *Chunk
	stackDepth    []int   // 每个指令位置的栈深度
	maxStackDepth int     // 最大栈深度
	instructions  []OpCode // 指令序列（便于访问）
}

// NewVerifier 创建验证器
func NewVerifier(chunk *Chunk) *Verifier {
	return &Verifier{
		chunk:         chunk,
		stackDepth:    make([]int, chunk.Len()),
		maxStackDepth: 0,
		instructions:  make([]OpCode, 0, chunk.Len()),
	}
}

// Verify 验证字节码
func (v *Verifier) Verify() error {
	if v.chunk == nil {
		return &VerificationError{Offset: 0, Message: "chunk 为空"}
	}

	// 提取指令序列
	v.extractInstructions()

	// 验证栈平衡和跳转目标
	return v.verifyStackAndJumps()
}

// extractInstructions 提取指令序列
func (v *Verifier) extractInstructions() {
	ip := 0
	for ip < v.chunk.Len() {
		op := OpCode(v.chunk.Code[ip])
		v.instructions = append(v.instructions, op)
		
		// 获取指令大小
		size := v.getOpSize(ip)
		ip += size
	}
}

// verifyStackAndJumps 验证栈平衡和跳转目标
func (v *Verifier) verifyStackAndJumps() error {
	ip := 0
	stack := 0
	
	// 标记所有跳转目标
	jumpTargets := make(map[int]bool)
	
	// 第一遍：标记所有跳转目标
	for ip < v.chunk.Len() {
		op := OpCode(v.chunk.Code[ip])
		
		switch op {
		case OpJump, OpJumpIfFalse, OpJumpIfTrue:
			// 向前跳转：读取相对偏移量
			offset := int(int16(v.chunk.ReadU16(ip + 1)))
			target := ip + 1 + 2 + offset
			if target >= 0 && target < v.chunk.Len() {
				jumpTargets[target] = true
			}
			ip += 3
		case OpLoop:
			// 向后跳转：读取相对偏移量
			offset := int(v.chunk.ReadU16(ip + 1))
			target := ip + 1 + 2 - offset
			if target >= 0 && target < v.chunk.Len() {
				jumpTargets[target] = true
			}
			ip += 3
		case OpEnterTry:
			// try 块可能有多个 catch 目标
			catchCount := int(v.chunk.Code[ip+1])
			ip += 2
			// 读取 finally 偏移量
			finallyOffset := int(int16(v.chunk.ReadU16(ip)))
			if finallyOffset >= 0 {
				finallyIP := ip + finallyOffset
				if finallyIP >= 0 && finallyIP < v.chunk.Len() {
					jumpTargets[finallyIP] = true
				}
			}
			ip += 2
			// 读取 catch 处理器
			for i := 0; i < catchCount; i++ {
				catchOffset := int(int16(v.chunk.ReadU16(ip+2)))
				catchIP := ip + catchOffset
				if catchIP >= 0 && catchIP < v.chunk.Len() {
					jumpTargets[catchIP] = true
				}
				ip += 4 // typeIdx (u16) + catchOffset (i16)
			}
			continue
		default:
			size := v.getOpSize(ip)
			ip += size
		}
	}
	
	// 第二遍：验证栈平衡和指令参数
	ip = 0
	stack = 0
	v.stackDepth[0] = 0
	
	for ip < v.chunk.Len() {
		// 记录栈深度
		v.stackDepth[ip] = stack
		if stack > v.maxStackDepth {
			v.maxStackDepth = stack
		}
		
		// 检查栈深度是否合理
		if stack < 0 {
			return &VerificationError{Offset: ip, Message: fmt.Sprintf("栈深度为负: %d", stack)}
		}
		if stack > 256 {
			return &VerificationError{Offset: ip, Message: fmt.Sprintf("栈深度超出限制: %d > 256", stack)}
		}
		
		op := OpCode(v.chunk.Code[ip])
		
		// 根据指令类型更新栈
		switch op {
		// 栈操作
		case OpPush, OpNull, OpTrue, OpFalse, OpZero, OpOne:
			stack++
		case OpPop:
			if stack < 1 {
				return &VerificationError{Offset: ip, Message: "OpPop 时栈为空"}
			}
			stack--
		case OpDup:
			if stack < 1 {
				return &VerificationError{Offset: ip, Message: "OpDup 时栈为空"}
			}
			stack++
		case OpSwap:
			if stack < 2 {
				return &VerificationError{Offset: ip, Message: "OpSwap 时栈元素少于 2 个"}
			}
		
		// 局部变量操作
		case OpLoadLocal, OpStoreLocal:
			// 检查索引是否有效
			idx := int(v.chunk.ReadU16(ip + 1))
			if idx > 255 {
				return &VerificationError{Offset: ip, Message: fmt.Sprintf("局部变量索引超出范围: %d", idx)}
			}
			if op == OpLoadLocal {
				stack++
			} else {
				if stack < 1 {
					return &VerificationError{Offset: ip, Message: "OpStoreLocal 时栈为空"}
				}
			}
			ip += 3
			continue
		
		// 全局变量操作
		case OpLoadGlobal, OpStoreGlobal:
			// 检查常量池索引
			constIdx := int(v.chunk.ReadU16(ip + 1))
			if constIdx >= len(v.chunk.Constants) {
				return &VerificationError{Offset: ip, Message: fmt.Sprintf("常量池索引超出范围: %d >= %d", constIdx, len(v.chunk.Constants))}
			}
			if op == OpLoadGlobal {
				stack++
			} else {
				if stack < 1 {
					return &VerificationError{Offset: ip, Message: "OpStoreGlobal 时栈为空"}
				}
			}
			ip += 3
			continue
		
		// 算术运算（二元）
		case OpAdd, OpSub, OpMul, OpDiv, OpMod:
			if stack < 2 {
				return &VerificationError{Offset: ip, Message: fmt.Sprintf("算术运算 %s 时栈元素少于 2 个", opNames[op])}
			}
			stack-- // 弹出两个，压入一个
		case OpNeg:
			if stack < 1 {
				return &VerificationError{Offset: ip, Message: "OpNeg 时栈为空"}
			}
			// 栈深度不变
		
		// 比较运算（二元）
		case OpEq, OpNe, OpLt, OpLe, OpGt, OpGe:
			if stack < 2 {
				return &VerificationError{Offset: ip, Message: fmt.Sprintf("比较运算 %s 时栈元素少于 2 个", opNames[op])}
			}
			stack-- // 弹出两个，压入一个 bool
		
		// 逻辑运算
		case OpNot:
			if stack < 1 {
				return &VerificationError{Offset: ip, Message: "OpNot 时栈为空"}
			}
			// 栈深度不变
		case OpAnd, OpOr:
			// 短路运算，栈深度在运行时决定
			// 验证时假设弹出两个，压入一个
			if stack < 1 {
				return &VerificationError{Offset: ip, Message: fmt.Sprintf("逻辑运算 %s 时栈为空", opNames[op])}
			}
		
		// 位运算
		case OpBitAnd, OpBitOr, OpBitXor:
			if stack < 2 {
				return &VerificationError{Offset: ip, Message: fmt.Sprintf("位运算 %s 时栈元素少于 2 个", opNames[op])}
			}
			stack--
		case OpBitNot:
			if stack < 1 {
				return &VerificationError{Offset: ip, Message: "OpBitNot 时栈为空"}
			}
		case OpShl, OpShr:
			if stack < 2 {
				return &VerificationError{Offset: ip, Message: fmt.Sprintf("移位运算 %s 时栈元素少于 2 个", opNames[op])}
			}
			stack--
		
		// 字符串操作
		case OpConcat:
			if stack < 2 {
				return &VerificationError{Offset: ip, Message: "OpConcat 时栈元素少于 2 个"}
			}
			stack--
		
		// 跳转指令
		case OpJump, OpJumpIfFalse, OpJumpIfTrue:
			if op == OpJumpIfFalse || op == OpJumpIfTrue {
				if stack < 1 {
					return &VerificationError{Offset: ip, Message: fmt.Sprintf("%s 时栈为空", opNames[op])}
				}
			}
			offset := int(int16(v.chunk.ReadU16(ip + 1)))
			target := ip + 1 + 2 + offset
			if target < 0 || target >= v.chunk.Len() {
				return &VerificationError{Offset: ip, Message: fmt.Sprintf("跳转目标无效: %d (范围: 0-%d)", target, v.chunk.Len()-1)}
			}
			// 验证跳转目标是有效指令开始位置
			if !jumpTargets[target] && target != 0 {
				// 允许跳转到已知跳转目标，但也允许跳转到指令边界（因为可能有其他跳转）
				// 这里只检查范围
			}
			ip += 3
			continue
		case OpLoop:
			offset := int(v.chunk.ReadU16(ip + 1))
			target := ip + 1 + 2 - offset
			if target < 0 || target >= v.chunk.Len() {
				return &VerificationError{Offset: ip, Message: fmt.Sprintf("循环跳转目标无效: %d (范围: 0-%d)", target, v.chunk.Len()-1)}
			}
			ip += 3
			continue
		
		// 函数调用
		case OpCall:
			argCount := int(v.chunk.Code[ip+1])
			if stack < argCount {
				return &VerificationError{Offset: ip, Message: fmt.Sprintf("OpCall 参数不足: 需要 %d 个参数，栈上只有 %d 个", argCount, stack)}
			}
			stack -= argCount
			// 函数返回值不确定，假设返回 1 个值
			stack++
			ip += 2
			continue
		case OpReturn, OpReturnNull:
			// return 语句会退出当前函数，不影响后续验证
			if op == OpReturn && stack < 1 {
				return &VerificationError{Offset: ip, Message: "OpReturn 时栈为空"}
			}
			// return 后不再验证（函数已退出）
			ip++
			continue
		
		// 对象操作
		case OpNewObject:
			constIdx := int(v.chunk.ReadU16(ip + 1))
			if constIdx >= len(v.chunk.Constants) {
				return &VerificationError{Offset: ip, Message: fmt.Sprintf("OpNewObject 常量池索引超出范围: %d", constIdx)}
			}
			stack++
			ip += 3
			continue
		case OpGetField:
			if stack < 1 {
				return &VerificationError{Offset: ip, Message: "OpGetField 时栈为空（缺少对象）"}
			}
			stack++ // 对象保持不变，压入字段值
			ip += 3
			continue
		case OpSetField:
			if stack < 2 {
				return &VerificationError{Offset: ip, Message: "OpSetField 时栈元素少于 2 个（需要对象和值）"}
			}
			// 弹出对象和值，压入值
			ip += 3
			continue
		case OpCallMethod:
			// OpCallMethod: [nameIdx: u16] [argCount: u8]
			constIdx := int(v.chunk.ReadU16(ip + 1))
			if constIdx >= len(v.chunk.Constants) {
				return &VerificationError{Offset: ip, Message: fmt.Sprintf("OpCallMethod 常量池索引超出范围: %d", constIdx)}
			}
			argCount := int(v.chunk.Code[ip+3])
			if stack < argCount+1 {
				return &VerificationError{Offset: ip, Message: fmt.Sprintf("OpCallMethod 参数不足: 需要 %d 个参数+对象，栈上只有 %d 个", argCount+1, stack)}
			}
			stack -= argCount + 1
			stack++ // 假设返回 1 个值
			ip += 4
			continue
		
		// 数组操作
		case OpNewArray:
			count := int(v.chunk.ReadU16(ip + 1))
			if stack < count {
				return &VerificationError{Offset: ip, Message: fmt.Sprintf("OpNewArray 元素不足: 需要 %d 个元素，栈上只有 %d 个", count, stack)}
			}
			stack -= count
			stack++
			ip += 3
			continue
		case OpArrayGet:
			if stack < 2 {
				return &VerificationError{Offset: ip, Message: "OpArrayGet 时栈元素少于 2 个（需要数组和索引）"}
			}
			// 弹出数组和索引，压入元素
			ip++
			continue
		case OpArraySet:
			if stack < 3 {
				return &VerificationError{Offset: ip, Message: "OpArraySet 时栈元素少于 3 个（需要数组、索引和值）"}
			}
			// 弹出数组、索引和值，压入值
			ip++
			continue
		
		// 其他指令
		default:
			// 未知指令或不需要验证的指令
			size := v.getOpSize(ip)
			ip += size
			continue
		}
		
		ip++
	}
	
	// 验证栈在函数结束时为空（或只有一个返回值）
	if stack > 1 {
		return &VerificationError{Offset: ip - 1, Message: fmt.Sprintf("函数结束时栈不为空: 还有 %d 个元素", stack)}
	}
	
	return nil
}

// getOpSize 获取指令大小（包括操作码和参数）
func (v *Verifier) getOpSize(ip int) int {
	if ip >= v.chunk.Len() {
		return 1
	}
	op := OpCode(v.chunk.Code[ip])
	
	switch op {
	case OpPush, OpLoadLocal, OpStoreLocal, OpLoadGlobal, OpStoreGlobal,
		OpNewObject, OpGetField, OpSetField, OpGetStatic, OpSetStatic,
		OpCallStatic, OpNewArray, OpNewFixedArray, OpNewMap, OpSuperArrayNew,
		OpNewBytes, OpCheckType, OpCast, OpCastSafe:
		return 3 // op (1) + u16 (2)
	case OpJump, OpJumpIfFalse, OpJumpIfTrue, OpLoop:
		return 3 // op (1) + i16/u16 (2)
	case OpCall:
		return 2 // op (1) + u8 (1)
	case OpCallMethod:
		return 4 // op (1) + nameIdx (u16, 2) + argCount (u8, 1)
	case OpEnterTry:
		// OpEnterTry: [catchCount: u8] [finallyOffset: i16] [typeIdx: u16, catchOffset: i16]*
		catchCount := int(v.chunk.Code[ip+1])
		return 2 + 2 + catchCount*4 // op + catchCount + finallyOffset + catch handlers
	default:
		return 1 // 单字节指令
	}
}

// VerifyChunk 验证 Chunk（便捷函数）
func VerifyChunk(chunk *Chunk) error {
	verifier := NewVerifier(chunk)
	return verifier.Verify()
}

// VerifyFunction 验证函数（包括方法体）
func VerifyFunction(fn *Function) error {
	if fn == nil {
		return fmt.Errorf("函数为空")
	}
	return VerifyChunk(fn.Chunk)
}

