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

