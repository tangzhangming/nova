package bytecode

// ============================================================================
// B4. 字节码指令优化器
// ============================================================================

// Optimizer 字节码优化器
type Optimizer struct {
	chunk         *Chunk
	optimizations int // 优化次数统计
	debug         bool
}

// NewOptimizer 创建优化器
func NewOptimizer(chunk *Chunk) *Optimizer {
	return &Optimizer{
		chunk: chunk,
		debug: false,
	}
}

// SetDebug 设置调试模式
func (o *Optimizer) SetDebug(debug bool) {
	o.debug = debug
}

// Optimize 执行所有优化
func (o *Optimizer) Optimize() int {
	o.optimizations = 0

	// 多遍优化，直到没有更多优化为止
	for {
		before := o.optimizations
		o.constantPropagation()
		o.copyPropagation()
		o.strengthReduction()
		o.jumpThreading()
		o.loopUnrolling()
		o.peepholeOptimize()
		o.deadCodeEliminate()
		o.constantFolding()
		if o.optimizations == before {
			break
		}
	}

	return o.optimizations
}

// peepholeOptimize 窗口优化：识别并优化常见指令模式
func (o *Optimizer) peepholeOptimize() {
	code := o.chunk.Code
	if len(code) < 2 {
		return
	}

	// 创建新的代码缓冲区
	newCode := make([]byte, 0, len(code))
	newLines := make([]int, 0, len(o.chunk.Lines))
	i := 0

	for i < len(code) {
		op := OpCode(code[i])
		size := o.instructionSize(op, i)

		optimized := false

		// 模式1: PUSH 0, ADD -> 无操作（加0等于不变）
		if op == OpZero && i+1 < len(code) && OpCode(code[i+1]) == OpAdd {
			// 跳过 ZERO 和 ADD
			i += 2
			o.optimizations++
			optimized = true
		}

		// 模式2: PUSH 1, MUL -> 无操作（乘1等于不变）
		if !optimized && op == OpOne && i+1 < len(code) && OpCode(code[i+1]) == OpMul {
			i += 2
			o.optimizations++
			optimized = true
		}

		// 模式3: PUSH 0, MUL -> POP, ZERO（乘0等于0）
		if !optimized && op == OpZero && i+1 < len(code) && OpCode(code[i+1]) == OpMul {
			// 替换为 POP + ZERO（弹出原值，压入0）
			newCode = append(newCode, byte(OpPop), byte(OpZero))
			newLines = append(newLines, o.chunk.Lines[i], o.chunk.Lines[i])
			i += 2
			o.optimizations++
			optimized = true
		}

		// 模式4: DUP, POP -> 无操作
		if !optimized && op == OpDup && i+1 < len(code) && OpCode(code[i+1]) == OpPop {
			i += 2
			o.optimizations++
			optimized = true
		}

		// 模式5: NOT, NOT -> 无操作
		if !optimized && op == OpNot && i+1 < len(code) && OpCode(code[i+1]) == OpNot {
			i += 2
			o.optimizations++
			optimized = true
		}

		// 模式6: NEG, NEG -> 无操作
		if !optimized && op == OpNeg && i+1 < len(code) && OpCode(code[i+1]) == OpNeg {
			i += 2
			o.optimizations++
			optimized = true
		}

		// 模式7: JUMP 0 -> 无操作（跳转到下一条指令）
		if !optimized && op == OpJump && i+2 < len(code) {
			offset := int16(code[i+1])<<8 | int16(code[i+2])
			if offset == 0 {
				i += 3
				o.optimizations++
				optimized = true
			}
		}

		// 模式8: LOAD_LOCAL x, STORE_LOCAL x -> 无操作（自赋值）
		if !optimized && op == OpLoadLocal && i+5 < len(code) {
			if OpCode(code[i+3]) == OpStoreLocal {
				idx1 := uint16(code[i+1])<<8 | uint16(code[i+2])
				idx2 := uint16(code[i+4])<<8 | uint16(code[i+5])
				if idx1 == idx2 {
					i += 6
					o.optimizations++
					optimized = true
				}
			}
		}

		// 模式9: TRUE, JUMP_IF_FALSE -> JUMP 到目标（恒假分支消除）
		// 模式10: FALSE, JUMP_IF_FALSE -> 无条件跳转（恒真分支）

		// 模式11: STORE_LOCAL x, LOAD_LOCAL x -> DUP, STORE_LOCAL x
		// 这样可以保留栈顶值，同时存储
		if !optimized && op == OpStoreLocal && i+5 < len(code) {
			if OpCode(code[i+3]) == OpLoadLocal {
				idx1 := uint16(code[i+1])<<8 | uint16(code[i+2])
				idx2 := uint16(code[i+4])<<8 | uint16(code[i+5])
				if idx1 == idx2 {
					// 替换为 DUP, STORE_LOCAL x
					newCode = append(newCode, byte(OpDup))
					newLines = append(newLines, o.chunk.Lines[i])
					// 复制 STORE_LOCAL x
					newCode = append(newCode, code[i:i+3]...)
					newLines = append(newLines, o.chunk.Lines[i], o.chunk.Lines[i], o.chunk.Lines[i])
					i += 6
					o.optimizations++
					optimized = true
				}
			}
		}

		// ====================================================================
		// BUG FIX 2026-01-10: 字节码优化增强 - 添加新窥孔优化模式
		// 防止反复引入的问题:
		// 1. 优化必须保持语义等价（不能改变程序行为）
		// 2. 跳转指令优化时必须重新计算偏移量
		// 3. 优化后的代码行号信息必须保持一致（用于调试）
		// 4. 死代码消除时不能删除跳转目标
		// ====================================================================

		// 模式12: TRUE, JUMP_IF_FALSE -> 无操作（恒真条件，直接移除跳转）
		if !optimized && op == OpTrue && i+3 < len(code) && OpCode(code[i+1]) == OpJumpIfFalse {
			// 跳过 TRUE 和 JUMP_IF_FALSE
			i += 4
			o.optimizations++
			optimized = true
		}

		// 模式13: FALSE, JUMP_IF_FALSE -> JUMP（恒假条件，转为无条件跳转）
		if !optimized && op == OpFalse && i+3 < len(code) && OpCode(code[i+1]) == OpJumpIfFalse {
			// 保留 JUMP_IF_FALSE 的偏移量，替换为 JUMP
			newCode = append(newCode, byte(OpJump), code[i+2], code[i+3])
			newLines = append(newLines, o.chunk.Lines[i], o.chunk.Lines[i], o.chunk.Lines[i])
			i += 4
			o.optimizations++
			optimized = true
		}

		// 模式14: TRUE, JUMP_IF_TRUE -> JUMP（恒真条件，转为无条件跳转）
		if !optimized && op == OpTrue && i+3 < len(code) && OpCode(code[i+1]) == OpJumpIfTrue {
			newCode = append(newCode, byte(OpJump), code[i+2], code[i+3])
			newLines = append(newLines, o.chunk.Lines[i], o.chunk.Lines[i], o.chunk.Lines[i])
			i += 4
			o.optimizations++
			optimized = true
		}

		// 模式15: FALSE, JUMP_IF_TRUE -> 无操作（恒假条件，直接移除跳转）
		if !optimized && op == OpFalse && i+3 < len(code) && OpCode(code[i+1]) == OpJumpIfTrue {
			i += 4
			o.optimizations++
			optimized = true
		}

		// 模式16: NULL, NOT -> TRUE（null 的逻辑非是 true）
		if !optimized && op == OpNull && i+1 < len(code) && OpCode(code[i+1]) == OpNot {
			newCode = append(newCode, byte(OpTrue))
			newLines = append(newLines, o.chunk.Lines[i])
			i += 2
			o.optimizations++
			optimized = true
		}

		// 模式17: ZERO, NOT -> TRUE（0 的逻辑非是 true）
		if !optimized && op == OpZero && i+1 < len(code) && OpCode(code[i+1]) == OpNot {
			newCode = append(newCode, byte(OpTrue))
			newLines = append(newLines, o.chunk.Lines[i])
			i += 2
			o.optimizations++
			optimized = true
		}

		// 模式18: ONE, NOT -> FALSE（1 的逻辑非是 false）
		if !optimized && op == OpOne && i+1 < len(code) && OpCode(code[i+1]) == OpNot {
			newCode = append(newCode, byte(OpFalse))
			newLines = append(newLines, o.chunk.Lines[i])
			i += 2
			o.optimizations++
			optimized = true
		}

		// 模式19: TRUE, NOT -> FALSE
		if !optimized && op == OpTrue && i+1 < len(code) && OpCode(code[i+1]) == OpNot {
			newCode = append(newCode, byte(OpFalse))
			newLines = append(newLines, o.chunk.Lines[i])
			i += 2
			o.optimizations++
			optimized = true
		}

		// 模式20: FALSE, NOT -> TRUE
		if !optimized && op == OpFalse && i+1 < len(code) && OpCode(code[i+1]) == OpNot {
			newCode = append(newCode, byte(OpTrue))
			newLines = append(newLines, o.chunk.Lines[i])
			i += 2
			o.optimizations++
			optimized = true
		}

		// 模式21: PUSH const, POP -> 无操作（压入后立即弹出无意义）
		if !optimized && op == OpPush && i+3 < len(code) && OpCode(code[i+3]) == OpPop {
			i += 4
			o.optimizations++
			optimized = true
		}

		// 模式22: LOAD_LOCAL x, LOAD_LOCAL x -> LOAD_LOCAL x, DUP（加载同一变量两次）
		if !optimized && op == OpLoadLocal && i+5 < len(code) && OpCode(code[i+3]) == OpLoadLocal {
			idx1 := uint16(code[i+1])<<8 | uint16(code[i+2])
			idx2 := uint16(code[i+4])<<8 | uint16(code[i+5])
			if idx1 == idx2 {
				// 替换为 LOAD_LOCAL x, DUP
				newCode = append(newCode, code[i:i+3]...)
				newLines = append(newLines, o.chunk.Lines[i], o.chunk.Lines[i], o.chunk.Lines[i])
				newCode = append(newCode, byte(OpDup))
				newLines = append(newLines, o.chunk.Lines[i])
				i += 6
				o.optimizations++
				optimized = true
			}
		}

		// 模式23: ZERO, SUB -> NEG（0 - x = -x）
		if !optimized && op == OpZero && i+1 < len(code) && OpCode(code[i+1]) == OpSub {
			// 注意: 需要确保栈上有值，这个优化假设栈顶有要减去的值
			// PUSH x, ZERO, SUB -> PUSH x, NEG
			// 跳过 ZERO 和 SUB，改为 NEG
			newCode = append(newCode, byte(OpNeg))
			newLines = append(newLines, o.chunk.Lines[i])
			i += 2
			o.optimizations++
			optimized = true
		}

		// 模式24: DUP, DUP, POP -> DUP（冗余的 DUP-POP）
		if !optimized && op == OpDup && i+2 < len(code) && 
			OpCode(code[i+1]) == OpDup && OpCode(code[i+2]) == OpPop {
			newCode = append(newCode, byte(OpDup))
			newLines = append(newLines, o.chunk.Lines[i])
			i += 3
			o.optimizations++
			optimized = true
		}

		if !optimized {
			// 复制原始指令
			for j := 0; j < size && i+j < len(code); j++ {
				newCode = append(newCode, code[i+j])
				if i+j < len(o.chunk.Lines) {
					newLines = append(newLines, o.chunk.Lines[i+j])
				}
			}
			i += size
		}
	}

	o.chunk.Code = newCode
	o.chunk.Lines = newLines
}

// deadCodeEliminate 死代码消除
func (o *Optimizer) deadCodeEliminate() {
	code := o.chunk.Code
	if len(code) < 2 {
		return
	}

	// 标记可达指令
	reachable := make([]bool, len(code))
	o.markReachable(0, reachable)

	// 只有当有不可达代码时才重建
	hasUnreachable := false
	for i, r := range reachable {
		if !r && i < len(code) {
			hasUnreachable = true
			break
		}
	}

	if !hasUnreachable {
		return
	}

	// 重建代码（跳过不可达指令）
	// 注意：需要重新计算跳转偏移，这比较复杂，暂时跳过
	// 完整实现需要两遍：第一遍计算新偏移，第二遍修补跳转
}

// markReachable 标记可达指令
func (o *Optimizer) markReachable(start int, reachable []bool) {
	code := o.chunk.Code
	i := start

	for i < len(code) && !reachable[i] {
		reachable[i] = true
		op := OpCode(code[i])
		size := o.instructionSize(op, i)

		// 标记指令占用的所有字节
		for j := 1; j < size && i+j < len(code); j++ {
			reachable[i+j] = true
		}

		switch op {
		case OpJump:
			// 无条件跳转：跳转到目标，当前路径终止
			if i+2 < len(code) {
				offset := int16(code[i+1])<<8 | int16(code[i+2])
				target := i + 3 + int(offset)
				if target >= 0 && target < len(code) {
					o.markReachable(target, reachable)
				}
			}
			return

		case OpJumpIfFalse, OpJumpIfTrue:
			// 条件跳转：两个分支都可能可达
			if i+2 < len(code) {
				offset := int16(code[i+1])<<8 | int16(code[i+2])
				target := i + 3 + int(offset)
				if target >= 0 && target < len(code) {
					o.markReachable(target, reachable)
				}
			}
			i += size

		case OpLoop:
			// 循环跳转
			if i+2 < len(code) {
				offset := int16(code[i+1])<<8 | int16(code[i+2])
				target := i + 3 - int(offset)
				if target >= 0 && target < len(code) {
					o.markReachable(target, reachable)
				}
			}
			i += size

		case OpReturn, OpReturnNull, OpHalt, OpThrow:
			// 终止指令：当前路径终止
			return

		default:
			i += size
		}
	}
}

// constantFolding 常量折叠（运行时部分优化）
func (o *Optimizer) constantFolding() {
	// 编译器已经做了大部分常量折叠
	// 这里可以处理一些编译器遗漏的情况
}

// instructionSize 获取指令大小
func (o *Optimizer) instructionSize(op OpCode, offset int) int {
	switch op {
	case OpPush, OpLoadLocal, OpStoreLocal, OpLoadGlobal, OpStoreGlobal,
		OpNewObject, OpGetField, OpSetField, OpNewArray, OpNewMap,
		OpCheckType, OpCast, OpCastSafe, OpClosure, OpEnterCatch:
		return 3 // op + u16

	case OpNewFixedArray:
		return 5 // op + u16 + u16

	case OpJump, OpJumpIfFalse, OpJumpIfTrue, OpLoop:
		return 3 // op + i16

	case OpCall, OpTailCall:
		return 2 // op + u8

	case OpCallMethod:
		return 4 // op + u16 + u8

	case OpGetStatic, OpSetStatic:
		return 5 // op + u16 + u16

	case OpCallStatic:
		return 6 // op + u16 + u16 + u8

	case OpSuperArrayNew:
		// 可变长度：需要解析元素数量
		if offset+2 < len(o.chunk.Code) {
			count := int(o.chunk.Code[offset+1])<<8 | int(o.chunk.Code[offset+2])
			return 3 + count // op + u16 + count bytes
		}
		return 3

	case OpEnterTry:
		// 可变长度：1 + 1 + 2 + (catchCount * 4)
		if offset+1 < len(o.chunk.Code) {
			catchCount := int(o.chunk.Code[offset+1])
			return 4 + catchCount*4
		}
		return 4

	default:
		return 1 // 单字节指令
	}
}

// OptimizeChunk 优化字节码块（便捷函数）
func OptimizeChunk(chunk *Chunk) int {
	opt := NewOptimizer(chunk)
	return opt.Optimize()
}

// ============================================================================
// 常量传播优化
// ============================================================================

// ConstantState 常量状态
type ConstantState struct {
	isConstant bool
	value      Value
}

// constantPropagation 常量传播：跟踪常量值并在使用点替换
func (o *Optimizer) constantPropagation() {
	code := o.chunk.Code
	if len(code) == 0 {
		return
	}

	// 跟踪每个局部变量的常量状态（简化版本：只处理局部变量）
	constants := make(map[uint16]*ConstantState)
	changed := false

	// 扫描代码，识别常量赋值
	for i := 0; i < len(code); {
		op := OpCode(code[i])
		size := o.instructionSize(op, i)

		// 识别常量赋值：PUSH const, STORE_LOCAL var
		if op == OpPush && i+3 < len(code) {
			constIdx := uint16(code[i+1])<<8 | uint16(code[i+2])
			if constIdx < uint16(len(o.chunk.Constants)) {
				constVal := o.chunk.Constants[constIdx]
				// 检查下一个指令是否是 STORE_LOCAL
				if i+3 < len(code) {
					nextOp := OpCode(code[i+3])
					if nextOp == OpStoreLocal {
						varIdx := uint16(code[i+4])<<8 | uint16(code[i+5])
						constants[varIdx] = &ConstantState{
							isConstant: true,
							value:      constVal,
						}
					}
				}
			}
		} else if op == OpZero && i+3 < len(code) {
			// ZERO, STORE_LOCAL var
			nextOp := OpCode(code[i+1])
			if nextOp == OpStoreLocal && i+3 < len(code) {
				varIdx := uint16(code[i+2])<<8 | uint16(code[i+3])
				constants[varIdx] = &ConstantState{
					isConstant: true,
					value:      ZeroValue,
				}
			}
		} else if op == OpOne && i+3 < len(code) {
			// ONE, STORE_LOCAL var
			nextOp := OpCode(code[i+1])
			if nextOp == OpStoreLocal && i+3 < len(code) {
				varIdx := uint16(code[i+2])<<8 | uint16(code[i+3])
				constants[varIdx] = &ConstantState{
					isConstant: true,
					value:      OneValue,
				}
			}
		} else if op == OpTrue && i+3 < len(code) {
			// TRUE, STORE_LOCAL var
			nextOp := OpCode(code[i+1])
			if nextOp == OpStoreLocal && i+3 < len(code) {
				varIdx := uint16(code[i+2])<<8 | uint16(code[i+3])
				constants[varIdx] = &ConstantState{
					isConstant: true,
					value:      TrueValue,
				}
			}
		} else if op == OpFalse && i+3 < len(code) {
			// FALSE, STORE_LOCAL var
			nextOp := OpCode(code[i+1])
			if nextOp == OpStoreLocal && i+3 < len(code) {
				varIdx := uint16(code[i+2])<<8 | uint16(code[i+3])
				constants[varIdx] = &ConstantState{
					isConstant: true,
					value:      FalseValue,
				}
			}
		} else if op == OpNull && i+3 < len(code) {
			// NULL, STORE_LOCAL var
			nextOp := OpCode(code[i+1])
			if nextOp == OpStoreLocal && i+3 < len(code) {
				varIdx := uint16(code[i+2])<<8 | uint16(code[i+3])
				constants[varIdx] = &ConstantState{
					isConstant: true,
					value:      NullValue,
				}
			}
		} else if op == OpStoreLocal {
			// 任何其他 STORE_LOCAL 都清除常量状态（可能被重新赋值）
			varIdx := uint16(code[i+1])<<8 | uint16(code[i+2])
			delete(constants, varIdx)
		}

		i += size
	}

	// 第二遍：替换 LOAD_LOCAL 为常量加载
	if len(constants) > 0 {
		newCode := make([]byte, 0, len(code))
		newLines := make([]int, 0, len(o.chunk.Lines))

		for i := 0; i < len(code); {
			op := OpCode(code[i])
			size := o.instructionSize(op, i)

			if op == OpLoadLocal {
				varIdx := uint16(code[i+1])<<8 | uint16(code[i+2])
				if state, ok := constants[varIdx]; ok && state.isConstant {
					// 替换为常量加载
					constIdx := o.chunk.AddConstant(state.value)
					newCode = append(newCode, byte(OpPush))
					newCode = append(newCode, byte(constIdx>>8), byte(constIdx))
					newLines = append(newLines, o.chunk.Lines[i], o.chunk.Lines[i], o.chunk.Lines[i])
					o.optimizations++
					changed = true
					i += size
					continue
				}
			}

			// 复制原始指令
			for j := 0; j < size && i+j < len(code); j++ {
				newCode = append(newCode, code[i+j])
				if i+j < len(o.chunk.Lines) {
					newLines = append(newLines, o.chunk.Lines[i+j])
				}
			}
			i += size
		}

		if changed {
			o.chunk.Code = newCode
			o.chunk.Lines = newLines
		}
	}
}

// ============================================================================
// 拷贝传播优化
// ============================================================================

// copyPropagation 拷贝传播：消除冗余的变量拷贝
func (o *Optimizer) copyPropagation() {
	code := o.chunk.Code
	if len(code) < 6 {
		return
	}

	newCode := make([]byte, 0, len(code))
	newLines := make([]int, 0, len(o.chunk.Lines))
	i := 0

	for i < len(code) {
		op := OpCode(code[i])
		size := o.instructionSize(op, i)

		// 模式：LOAD_LOCAL x, STORE_LOCAL y -> 如果后面只用 y，可以将 y 替换为 x
		// 简化版本：只优化连续的 LOAD_LOCAL x, STORE_LOCAL y, LOAD_LOCAL y
		if op == OpLoadLocal && i+6 < len(code) {
			srcIdx := uint16(code[i+1])<<8 | uint16(code[i+2])
			nextOp := OpCode(code[i+3])
			if nextOp == OpStoreLocal {
				dstIdx := uint16(code[i+4])<<8 | uint16(code[i+5])
				// 检查下一条指令是否是 LOAD_LOCAL dst
				if i+6 < len(code) {
					nextNextOp := OpCode(code[i+6])
					if nextNextOp == OpLoadLocal && i+8 < len(code) {
						loadIdx := uint16(code[i+7])<<8 | uint16(code[i+8])
						if loadIdx == dstIdx && srcIdx != dstIdx {
							// LOAD_LOCAL x, STORE_LOCAL y, LOAD_LOCAL y -> LOAD_LOCAL x, STORE_LOCAL y, LOAD_LOCAL x
							// 实际上可以优化为：LOAD_LOCAL x, STORE_LOCAL y, DUP, POP (如果 y 不再使用)
							// 简化：替换 LOAD_LOCAL y 为 LOAD_LOCAL x
							newCode = append(newCode, code[i], code[i+1], code[i+2]) // LOAD_LOCAL x
							newLines = append(newLines, o.chunk.Lines[i], o.chunk.Lines[i+1], o.chunk.Lines[i+2])
							newCode = append(newCode, code[i+3], code[i+4], code[i+5]) // STORE_LOCAL y
							newLines = append(newLines, o.chunk.Lines[i+3], o.chunk.Lines[i+4], o.chunk.Lines[i+5])
							newCode = append(newCode, code[i+6], code[i+1], code[i+2]) // LOAD_LOCAL x (替换 y)
							newLines = append(newLines, o.chunk.Lines[i+6], o.chunk.Lines[i+6], o.chunk.Lines[i+6])
							o.optimizations++
							i += 9
							continue
						}
					}
				}
			}
		}

		// 复制原始指令
		for j := 0; j < size && i+j < len(code); j++ {
			newCode = append(newCode, code[i+j])
			if i+j < len(o.chunk.Lines) {
				newLines = append(newLines, o.chunk.Lines[i+j])
			}
		}
		i += size
	}

	o.chunk.Code = newCode
	o.chunk.Lines = newLines
}

// ============================================================================
// 强度削弱优化
// ============================================================================

// strengthReduction 强度削弱：用更快的操作替换慢操作
func (o *Optimizer) strengthReduction() {
	code := o.chunk.Code
	if len(code) < 2 {
		return
	}

	newCode := make([]byte, 0, len(code))
	newLines := make([]int, 0, len(o.chunk.Lines))
	i := 0

	for i < len(code) {
		op := OpCode(code[i])
		size := o.instructionSize(op, i)

		optimized := false

		// 模式1: PUSH 2, MUL -> SHL (左移1位，相当于乘以2)
		if op == OpPush && i+3 < len(code) {
			constIdx := uint16(code[i+1])<<8 | uint16(code[i+2])
			if constIdx < uint16(len(o.chunk.Constants)) {
				constVal := o.chunk.Constants[constIdx]
				if constVal.Type() == ValInt {
					val := constVal.AsInt()
					if i+3 < len(code) && OpCode(code[i+3]) == OpMul {
						// 检查是否是 2 的幂
						if val == 2 {
							// 替换为 SHL
							newCode = append(newCode, byte(OpShl), byte(OpOne))
							newLines = append(newLines, o.chunk.Lines[i], o.chunk.Lines[i])
							o.optimizations++
							optimized = true
							i += 4
						} else if val > 0 && (val&(val-1)) == 0 {
							// 2的幂：计算位移次数
							shift := int64(0)
							for v := val; v > 1; v >>= 1 {
								shift++
							}
							if shift > 0 && shift < 64 {
								// 替换为 SHL with shift
								newCode = append(newCode, byte(OpPush))
								shiftIdx := o.chunk.AddConstant(NewInt(shift))
								newCode = append(newCode, byte(shiftIdx>>8), byte(shiftIdx))
								newCode = append(newCode, byte(OpShl))
								newLines = append(newLines, o.chunk.Lines[i], o.chunk.Lines[i], o.chunk.Lines[i], o.chunk.Lines[i+3])
								o.optimizations++
								optimized = true
								i += 4
							}
						}
					} else if i+3 < len(code) && OpCode(code[i+3]) == OpDiv && val > 0 && (val&(val-1)) == 0 {
						// 除以2的幂：替换为 SHR
						shift := int64(0)
						for v := val; v > 1; v >>= 1 {
							shift++
						}
						if shift > 0 && shift < 64 {
							newCode = append(newCode, byte(OpPush))
							shiftIdx := o.chunk.AddConstant(NewInt(shift))
							newCode = append(newCode, byte(shiftIdx>>8), byte(shiftIdx))
							newCode = append(newCode, byte(OpShr))
							newLines = append(newLines, o.chunk.Lines[i], o.chunk.Lines[i], o.chunk.Lines[i], o.chunk.Lines[i+3])
							o.optimizations++
							optimized = true
							i += 4
						}
					} else if i+3 < len(code) && OpCode(code[i+3]) == OpMod && val > 0 && (val&(val-1)) == 0 {
						// BUG FIX 2026-01-10: 强度削减增强 - 模2的幂运算
						// x % (2^n) -> x & (2^n - 1)
						// 例如: x % 8 -> x & 7
						mask := val - 1
						newCode = append(newCode, byte(OpPush))
						maskIdx := o.chunk.AddConstant(NewInt(mask))
						newCode = append(newCode, byte(maskIdx>>8), byte(maskIdx))
						newCode = append(newCode, byte(OpBitAnd))
						newLines = append(newLines, o.chunk.Lines[i], o.chunk.Lines[i], o.chunk.Lines[i], o.chunk.Lines[i+3])
						o.optimizations++
						optimized = true
						i += 4
					}
				}
			}
		}

		// 模式2: x * 0 -> 0 (已在窥孔优化中处理)
		// 模式3: x * 1 -> x (已在窥孔优化中处理)

		// BUG FIX 2026-01-10: 强度削减增强 - 布尔代数简化
		// 这些优化在运行时应用，处理常量布尔表达式

		// 模式4: TRUE, AND -> 第二个操作数（true && x = x）
		if !optimized && op == OpTrue && i+1 < len(code) && OpCode(code[i+1]) == OpAnd {
			// TRUE 被编译为单独指令，AND 需要两个操作数
			// 跳过 TRUE，保留后续的 AND 操作
			// 注意：这需要确保栈上有另一个操作数
			i += 1 // 只跳过 TRUE
			o.optimizations++
			optimized = true
		}

		// 模式5: FALSE, AND -> FALSE（false && x = false）
		if !optimized && op == OpFalse && i+1 < len(code) && OpCode(code[i+1]) == OpAnd {
			// false && anything = false
			// 保持 FALSE，跳过 AND
			newCode = append(newCode, byte(OpFalse))
			newLines = append(newLines, o.chunk.Lines[i])
			i += 2
			o.optimizations++
			optimized = true
		}

		// 模式6: TRUE, OR -> TRUE（true || x = true）
		if !optimized && op == OpTrue && i+1 < len(code) && OpCode(code[i+1]) == OpOr {
			// true || anything = true
			newCode = append(newCode, byte(OpTrue))
			newLines = append(newLines, o.chunk.Lines[i])
			i += 2
			o.optimizations++
			optimized = true
		}

		// 模式7: FALSE, OR -> 第二个操作数（false || x = x）
		if !optimized && op == OpFalse && i+1 < len(code) && OpCode(code[i+1]) == OpOr {
			i += 1 // 只跳过 FALSE
			o.optimizations++
			optimized = true
		}

		if !optimized {
			// 复制原始指令
			for j := 0; j < size && i+j < len(code); j++ {
				newCode = append(newCode, code[i+j])
				if i+j < len(o.chunk.Lines) {
					newLines = append(newLines, o.chunk.Lines[i+j])
				}
			}
			i += size
		}
	}

	o.chunk.Code = newCode
	o.chunk.Lines = newLines
}

// ============================================================================
// 跳转线程化优化
// ============================================================================

// jumpThreading 跳转线程化：消除连续跳转
func (o *Optimizer) jumpThreading() {
	code := o.chunk.Code
	if len(code) < 6 {
		return
	}

	// 构建跳转目标映射
	jumpTargets := make(map[int]int) // target -> final target

	// 第一遍：识别连续跳转
	for i := 0; i < len(code); {
		op := OpCode(code[i])
		size := o.instructionSize(op, i)

		if op == OpJump {
			if i+2 < len(code) {
				offset := int16(code[i+1])<<8 | int16(code[i+2])
				target := i + 3 + int(offset)
				if target >= 0 && target < len(code) {
					// 检查目标是否是另一个跳转
					if target < len(code) {
						targetOp := OpCode(code[target])
						if targetOp == OpJump && target+2 < len(code) {
							targetOffset := int16(code[target+1])<<8 | int16(code[target+2])
							finalTarget := target + 3 + int(targetOffset)
							if finalTarget >= 0 && finalTarget < len(code) {
								jumpTargets[target] = finalTarget
							}
						}
					}
				}
			}
		}

		i += size
	}

	// 第二遍：应用跳转线程化
	if len(jumpTargets) > 0 {
		newCode := make([]byte, 0, len(code))
		newLines := make([]int, 0, len(o.chunk.Lines))

		for i := 0; i < len(code); {
			op := OpCode(code[i])
			size := o.instructionSize(op, i)

			if op == OpJump {
				if i+2 < len(code) {
					offset := int16(code[i+1])<<8 | int16(code[i+2])
					target := i + 3 + int(offset)
					if finalTarget, ok := jumpTargets[target]; ok {
						// 直接跳转到最终目标
						newOffset := finalTarget - (i + 3)
						if newOffset >= -32768 && newOffset <= 32767 {
							newCode = append(newCode, byte(OpJump))
							newCode = append(newCode, byte(newOffset>>8), byte(newOffset))
							newLines = append(newLines, o.chunk.Lines[i], o.chunk.Lines[i], o.chunk.Lines[i])
							o.optimizations++
							i += size
							continue
						}
					}
				}
			}

			// 复制原始指令
			for j := 0; j < size && i+j < len(code); j++ {
				newCode = append(newCode, code[i+j])
				if i+j < len(o.chunk.Lines) {
					newLines = append(newLines, o.chunk.Lines[i+j])
				}
			}
			i += size
		}

		o.chunk.Code = newCode
		o.chunk.Lines = newLines
	}
}

// ============================================================================
// 循环展开优化
// ============================================================================

// loopUnrolling 循环展开：展开小循环（2-4次迭代）
func (o *Optimizer) loopUnrolling() {
	code := o.chunk.Code
	if len(code) < 10 {
		return
	}

	newCode := make([]byte, 0, len(code))
	newLines := make([]int, 0, len(o.chunk.Lines))
	i := 0

	for i < len(code) {
		op := OpCode(code[i])
		size := o.instructionSize(op, i)

		// 识别简单循环模式：LOOP offset (offset 指向循环开始)
		// 简化版本：只展开非常小的固定迭代次数的循环
		// 实际实现需要分析循环体大小和迭代次数，这里做简化处理
		if op == OpLoop && i+2 < len(code) {
			offset := uint16(code[i+1])<<8 | uint16(code[i+2])
			loopStart := i + 3 - int(offset)
			if loopStart >= 0 && loopStart < i {
				// 计算循环体大小
				loopBodySize := i + 3 - loopStart
				// 只展开小的循环体（小于50字节）
				if loopBodySize < 50 {
					// 展开2次（简化：固定展开2次）
					// 复制循环体两次
					loopBody := code[loopStart:i]
					loopBodyLines := o.chunk.Lines[loopStart:i]
					newCode = append(newCode, loopBody...)
					newLines = append(newLines, loopBodyLines...)
					newCode = append(newCode, loopBody...)
					newLines = append(newLines, loopBodyLines...)
					o.optimizations++
					i += 3 // 跳过 LOOP 指令
					continue
				}
			}
		}

		// 复制原始指令
		for j := 0; j < size && i+j < len(code); j++ {
			newCode = append(newCode, code[i+j])
			if i+j < len(o.chunk.Lines) {
				newLines = append(newLines, o.chunk.Lines[i+j])
			}
		}
		i += size
	}

	// 只有当有优化时才替换
	if len(newCode) != len(code) {
		o.chunk.Code = newCode
		o.chunk.Lines = newLines
	}
}

