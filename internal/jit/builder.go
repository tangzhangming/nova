// builder.go - IR 构建器
//
// 本文件实现了从字节码到 SSA IR 的转换。
//
// 转换过程：
// 1. 识别基本块边界（分支目标、跳转后的指令）
// 2. 模拟字节码虚拟机的栈操作，建立值的定义-使用关系
// 3. 在控制流合并点插入 Phi 节点
// 4. 生成 SSA 形式的 IR
//
// 栈模拟：
// 字节码虚拟机是基于栈的，而 IR 是基于寄存器的。
// 我们通过模拟栈操作来跟踪每个栈位置对应的 IR 值。

package jit

import (
	"fmt"
	"sort"

	"github.com/tangzhangming/nova/internal/bytecode"
)

// ============================================================================
// IR 构建器
// ============================================================================

// IRBuilder 从字节码构建 IR
type IRBuilder struct {
	fn          *IRFunc
	srcFunc     *bytecode.Function
	chunk       *bytecode.Chunk
	
	// 当前状态
	current     *IRBlock           // 当前基本块
	stack       []*IRValue         // 模拟栈
	locals      []*IRValue         // 局部变量的当前值
	
	// 基本块映射
	ipToBlock   map[int]*IRBlock   // 字节码偏移 -> 基本块
	blockStarts map[int]bool       // 基本块起始位置
	
	// Phi 节点处理
	incompletePhis map[*IRBlock]map[int]*IRInstr // block -> local -> phi
}

// NewIRBuilder 创建 IR 构建器
func NewIRBuilder() *IRBuilder {
	return &IRBuilder{
		ipToBlock:   make(map[int]*IRBlock),
		blockStarts: make(map[int]bool),
		incompletePhis: make(map[*IRBlock]map[int]*IRInstr),
	}
}

// Build 从字节码函数构建 IR
func (b *IRBuilder) Build(srcFunc *bytecode.Function) (*IRFunc, error) {
	b.srcFunc = srcFunc
	b.chunk = srcFunc.Chunk
	
	// 创建 IR 函数
	b.fn = NewIRFunc(srcFunc.Name, srcFunc.Arity)
	b.fn.SourceFunc = srcFunc
	b.fn.Constants = srcFunc.Chunk.Constants
	b.fn.LocalCount = srcFunc.LocalCount
	
	// 初始化局部变量
	b.locals = make([]*IRValue, srcFunc.LocalCount)
	
	// 第一步：识别基本块边界
	b.identifyBlocks()
	
	// 第二步：创建基本块
	b.createBlocks()
	
	// 第三步：转换指令
	err := b.convertInstructions()
	if err != nil {
		return nil, err
	}
	
	// 第四步：完成 Phi 节点
	b.completePhis()
	
	return b.fn, nil
}

// ============================================================================
// 基本块识别
// ============================================================================

// identifyBlocks 识别基本块边界
func (b *IRBuilder) identifyBlocks() {
	code := b.chunk.Code
	
	// 第一条指令总是块的开始
	b.blockStarts[0] = true
	
	// 扫描跳转指令
	ip := 0
	for ip < len(code) {
		op := bytecode.OpCode(code[ip])
		size := b.instrSize(op, ip)
		
		switch op {
		case bytecode.OpJump, bytecode.OpJumpIfFalse, bytecode.OpJumpIfTrue:
			// 跳转目标是块的开始
			if ip+2 < len(code) {
				offset := int(int16(code[ip+1])<<8 | int16(code[ip+2]))
				target := ip + 3 + offset
				if target >= 0 && target < len(code) {
					b.blockStarts[target] = true
				}
			}
			// 跳转后的下一条指令也是块的开始（fall-through）
			next := ip + size
			if next < len(code) {
				b.blockStarts[next] = true
			}
			
		case bytecode.OpLoop:
			// 回跳目标
			if ip+2 < len(code) {
				offset := int(code[ip+1])<<8 | int(code[ip+2])
				target := ip + 3 - offset
				if target >= 0 && target < len(code) {
					b.blockStarts[target] = true
				}
			}
			
		case bytecode.OpReturn, bytecode.OpReturnNull:
			// 返回后的指令是块的开始
			next := ip + size
			if next < len(code) {
				b.blockStarts[next] = true
			}
		}
		
		ip += size
	}
}

// createBlocks 创建基本块
func (b *IRBuilder) createBlocks() {
	// 收集所有块起始位置并排序
	starts := make([]int, 0, len(b.blockStarts))
	for ip := range b.blockStarts {
		starts = append(starts, ip)
	}
	sort.Ints(starts)
	
	// 创建基本块
	for i, ip := range starts {
		var block *IRBlock
		if i == 0 {
			// 入口块已经创建
			block = b.fn.Entry
		} else {
			block = b.fn.NewBlock()
		}
		b.ipToBlock[ip] = block
	}
}

// ============================================================================
// 指令转换
// ============================================================================

// convertInstructions 转换所有指令
func (b *IRBuilder) convertInstructions() error {
	code := b.chunk.Code
	
	// 初始化：入口块，参数作为局部变量
	b.current = b.fn.Entry
	b.stack = make([]*IRValue, 0)
	
	// 加载参数
	for i := 0; i < b.srcFunc.Arity; i++ {
		arg := b.fn.NewValue(TypeUnknown)
		b.locals[i] = arg
	}
	
	// 转换每条指令
	ip := 0
	for ip < len(code) {
		// 检查是否需要切换基本块
		if block, ok := b.ipToBlock[ip]; ok && block != b.current {
			// 如果当前块没有终止指令，添加跳转
			if b.current != nil && (len(b.current.Instrs) == 0 || !b.current.LastInstr().IsTerminator()) {
				jump := NewInstr(OpJump, nil)
				jump.Targets = []*IRBlock{block}
				b.current.AddInstr(jump)
				b.current.AddSucc(block)
			}
			b.current = block
		}
		
		op := bytecode.OpCode(code[ip])
		size := b.instrSize(op, ip)
		line := 0
		if ip < len(b.chunk.Lines) {
			line = b.chunk.Lines[ip]
		}
		
		err := b.convertInstruction(op, ip, line)
		if err != nil {
			return err
		}
		
		ip += size
	}
	
	return nil
}

// convertInstruction 转换单条指令
func (b *IRBuilder) convertInstruction(op bytecode.OpCode, ip int, line int) error {
	switch op {
	// 常量加载
	case bytecode.OpPush:
		idx := b.chunk.ReadU16(ip + 1)
		if int(idx) < len(b.chunk.Constants) {
			val := b.chunk.Constants[idx]
			irVal := b.createConstValue(val)
			b.push(irVal)
		}
		
	case bytecode.OpNull:
		// null 处理为 0
		b.push(b.fn.NewConstIntValue(0))
		
	case bytecode.OpTrue:
		b.push(b.fn.NewConstBoolValue(true))
		
	case bytecode.OpFalse:
		b.push(b.fn.NewConstBoolValue(false))
		
	case bytecode.OpZero:
		b.push(b.fn.NewConstIntValue(0))
		
	case bytecode.OpOne:
		b.push(b.fn.NewConstIntValue(1))
		
	// 局部变量
	case bytecode.OpLoadLocal:
		idx := int(b.chunk.ReadU16(ip + 1))
		val := b.readLocal(idx)
		b.push(val)
		
	case bytecode.OpStoreLocal:
		idx := int(b.chunk.ReadU16(ip + 1))
		val := b.pop()
		b.writeLocal(idx, val)
		
	// 栈操作
	case bytecode.OpPop:
		b.pop()
		
	case bytecode.OpDup:
		if len(b.stack) > 0 {
			b.push(b.stack[len(b.stack)-1])
		}
		
	case bytecode.OpSwap:
		if len(b.stack) >= 2 {
			n := len(b.stack)
			b.stack[n-1], b.stack[n-2] = b.stack[n-2], b.stack[n-1]
		}
		
	// 算术运算
	case bytecode.OpAdd:
		b.emitBinary(OpAdd, line)
		
	case bytecode.OpSub:
		b.emitBinary(OpSub, line)
		
	case bytecode.OpMul:
		b.emitBinary(OpMul, line)
		
	case bytecode.OpDiv:
		b.emitBinary(OpDiv, line)
		
	case bytecode.OpMod:
		b.emitBinary(OpMod, line)
		
	case bytecode.OpNeg:
		b.emitUnary(OpNeg, line)
		
	// 比较运算
	case bytecode.OpEq:
		b.emitBinary(OpEq, line)
		
	case bytecode.OpNe:
		b.emitBinary(OpNe, line)
		
	case bytecode.OpLt:
		b.emitBinary(OpLt, line)
		
	case bytecode.OpLe:
		b.emitBinary(OpLe, line)
		
	case bytecode.OpGt:
		b.emitBinary(OpGt, line)
		
	case bytecode.OpGe:
		b.emitBinary(OpGe, line)
		
	// 逻辑运算
	case bytecode.OpNot:
		b.emitUnary(OpNot, line)
		
	// 位运算
	case bytecode.OpBitAnd:
		b.emitBinary(OpBitAnd, line)
		
	case bytecode.OpBitOr:
		b.emitBinary(OpBitOr, line)
		
	case bytecode.OpBitXor:
		b.emitBinary(OpBitXor, line)
		
	case bytecode.OpBitNot:
		b.emitUnary(OpBitNot, line)
		
	case bytecode.OpShl:
		b.emitBinary(OpShl, line)
		
	case bytecode.OpShr:
		b.emitBinary(OpShr, line)
		
	// 跳转
	case bytecode.OpJump:
		offset := int(int16(b.chunk.ReadU16(ip + 1)))
		target := ip + 3 + offset
		targetBlock := b.ipToBlock[target]
		if targetBlock != nil {
			jump := NewInstr(OpJump, nil)
			jump.Targets = []*IRBlock{targetBlock}
			jump.Line = line
			b.current.AddInstr(jump)
			b.current.AddSucc(targetBlock)
		}
		
	case bytecode.OpJumpIfFalse:
		cond := b.pop()
		offset := int(int16(b.chunk.ReadU16(ip + 1)))
		target := ip + 3 + offset
		targetBlock := b.ipToBlock[target]
		nextBlock := b.ipToBlock[ip+3]
		
		if targetBlock != nil && nextBlock != nil {
			branch := NewInstr(OpBranch, nil, cond)
			// 注意：JumpIfFalse 是条件为假时跳转
			// 所以 Targets[0] = then (不跳转), Targets[1] = else (跳转)
			branch.Targets = []*IRBlock{nextBlock, targetBlock}
			branch.Line = line
			b.current.AddInstr(branch)
			b.current.AddSucc(nextBlock)
			b.current.AddSucc(targetBlock)
		}
		
	case bytecode.OpJumpIfTrue:
		cond := b.pop()
		offset := int(int16(b.chunk.ReadU16(ip + 1)))
		target := ip + 3 + offset
		targetBlock := b.ipToBlock[target]
		nextBlock := b.ipToBlock[ip+3]
		
		if targetBlock != nil && nextBlock != nil {
			branch := NewInstr(OpBranch, nil, cond)
			// JumpIfTrue: Targets[0] = then (跳转), Targets[1] = else (不跳转)
			branch.Targets = []*IRBlock{targetBlock, nextBlock}
			branch.Line = line
			b.current.AddInstr(branch)
			b.current.AddSucc(targetBlock)
			b.current.AddSucc(nextBlock)
		}
		
	case bytecode.OpLoop:
		offset := int(b.chunk.ReadU16(ip + 1))
		target := ip + 3 - offset
		targetBlock := b.ipToBlock[target]
		if targetBlock != nil {
			jump := NewInstr(OpJump, nil)
			jump.Targets = []*IRBlock{targetBlock}
			jump.Line = line
			b.current.AddInstr(jump)
			b.current.AddSucc(targetBlock)
		}
		
	// 返回
	case bytecode.OpReturn:
		var retVal *IRValue
		if len(b.stack) > 0 {
			retVal = b.pop()
		}
		ret := NewInstr(OpReturn, nil, retVal)
		ret.Line = line
		b.current.AddInstr(ret)
		
	case bytecode.OpReturnNull:
		ret := NewInstr(OpReturn, nil, b.fn.NewConstIntValue(0))
		ret.Line = line
		b.current.AddInstr(ret)
		
	// 不支持的指令暂时跳过
	default:
		// 对于复杂指令，我们需要回退到解释器
		// 这里简单地返回错误
		return fmt.Errorf("unsupported opcode: %s at ip=%d", op.String(), ip)
	}
	
	return nil
}

// ============================================================================
// 辅助方法
// ============================================================================

// push 压入栈
func (b *IRBuilder) push(v *IRValue) {
	b.stack = append(b.stack, v)
}

// pop 弹出栈
func (b *IRBuilder) pop() *IRValue {
	if len(b.stack) == 0 {
		// 返回一个未定义值
		return b.fn.NewConstIntValue(0)
	}
	v := b.stack[len(b.stack)-1]
	b.stack = b.stack[:len(b.stack)-1]
	return v
}

// readLocal 读取局部变量
func (b *IRBuilder) readLocal(idx int) *IRValue {
	if idx >= len(b.locals) {
		// 扩展 locals 数组
		newLocals := make([]*IRValue, idx+1)
		copy(newLocals, b.locals)
		b.locals = newLocals
	}
	
	if b.locals[idx] == nil {
		// 创建一个未定义值
		b.locals[idx] = b.fn.NewConstIntValue(0)
	}
	
	return b.locals[idx]
}

// writeLocal 写入局部变量
func (b *IRBuilder) writeLocal(idx int, val *IRValue) {
	if idx >= len(b.locals) {
		// 扩展 locals 数组
		newLocals := make([]*IRValue, idx+1)
		copy(newLocals, b.locals)
		b.locals = newLocals
	}
	
	b.locals[idx] = val
}

// createConstValue 从 bytecode.Value 创建 IR 常量
func (b *IRBuilder) createConstValue(val bytecode.Value) *IRValue {
	switch val.Type {
	case bytecode.ValInt:
		return b.fn.NewConstIntValue(val.AsInt())
	case bytecode.ValFloat:
		return b.fn.NewConstFloatValue(val.AsFloat())
	case bytecode.ValBool:
		return b.fn.NewConstBoolValue(val.AsBool())
	default:
		// 其他类型暂时返回 0
		return b.fn.NewConstIntValue(0)
	}
}

// emitBinary 生成二元运算指令
func (b *IRBuilder) emitBinary(op Opcode, line int) {
	right := b.pop()
	left := b.pop()
	
	// 确定结果类型
	resultType := b.inferBinaryType(op, left, right)
	dest := b.fn.NewValue(resultType)
	
	instr := NewInstr(op, dest, left, right)
	instr.Line = line
	b.current.AddInstr(instr)
	
	b.push(dest)
}

// emitUnary 生成一元运算指令
func (b *IRBuilder) emitUnary(op Opcode, line int) {
	operand := b.pop()
	
	// 确定结果类型
	resultType := operand.Type
	if op == OpNot {
		resultType = TypeBool
	}
	dest := b.fn.NewValue(resultType)
	
	instr := NewInstr(op, dest, operand)
	instr.Line = line
	b.current.AddInstr(instr)
	
	b.push(dest)
}

// inferBinaryType 推断二元运算结果类型
func (b *IRBuilder) inferBinaryType(op Opcode, left, right *IRValue) ValueType {
	// 比较运算返回布尔值
	switch op {
	case OpEq, OpNe, OpLt, OpLe, OpGt, OpGe:
		return TypeBool
	}
	
	// 如果任一操作数是浮点数，结果是浮点数
	if left.Type == TypeFloat || right.Type == TypeFloat {
		return TypeFloat
	}
	
	// 默认返回整数
	return TypeInt
}

// instrSize 获取指令大小
func (b *IRBuilder) instrSize(op bytecode.OpCode, ip int) int {
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
		// EnterTry 有变长的 catch 处理器
		catchCount := int(b.chunk.Code[ip+1])
		return 4 + catchCount*4
	case bytecode.OpEnterCatch:
		return 3
	default:
		return 1
	}
}

// completePhis 完成 Phi 节点的构建
// 在简化实现中，我们不生成完整的 SSA 形式，因此这里是空操作
func (b *IRBuilder) completePhis() {
	// TODO: 实现完整的 SSA 转换
	// 这需要：
	// 1. 计算支配边界
	// 2. 在合并点插入 Phi 节点
	// 3. 重命名变量
}
