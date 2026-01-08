// x64_codegen.go - x86-64 代码生成器
//
// 本文件实现了从 IR 到 x86-64 机器码的转换。
//
// 调用约定：Windows x64
// - 参数传递：RCX, RDX, R8, R9（前 4 个整数/指针参数）
// - 返回值：RAX
// - 调用者保存：RAX, RCX, RDX, R8-R11
// - 被调用者保存：RBX, RBP, RSI, RDI, R12-R15
// - 栈对齐：16 字节
// - Shadow space：调用前需要保留 32 字节
//
// 寄存器分配约定：
// - RAX: 返回值和临时寄存器
// - RCX, RDX, R8, R9: 参数寄存器
// - RBP: 帧指针
// - RSP: 栈指针
// - R10-R15: 通用寄存器（分配给 IR 值）

package jit

import (
	"fmt"
)

// ============================================================================
// x86-64 代码生成器
// ============================================================================

// X64CodeGenerator x86-64 代码生成器
type X64CodeGenerator struct {
	asm    *X64Assembler
	alloc  *RegAllocation
	fn     *IRFunc
	
	// 寄存器映射：虚拟寄存器编号 -> 物理寄存器
	// 使用 R10-R15 和 RBX 作为可分配寄存器
	physRegs []X64Reg
}

// NewX64CodeGenerator 创建代码生成器
func NewX64CodeGenerator() *X64CodeGenerator {
	return &X64CodeGenerator{
		asm: NewX64Assembler(),
		// 可分配的寄存器（避开参数寄存器和特殊寄存器）
		physRegs: []X64Reg{
			R10, R11, R12, R13, R14, R15, // 扩展寄存器
			RBX, RSI, RDI,                 // 基础寄存器
		},
	}
}

// NumRegisters 返回可用寄存器数量
func (cg *X64CodeGenerator) NumRegisters() int {
	return len(cg.physRegs)
}

// CallingConvention 返回调用约定
func (cg *X64CodeGenerator) CallingConvention() CallingConv {
	return CallingConv{
		ArgRegs:     []int{int(RCX), int(RDX), int(R8), int(R9)},
		RetReg:      int(RAX),
		CallerSaved: []int{int(RAX), int(RCX), int(RDX), int(R8), int(R9), int(R10), int(R11)},
		CalleeSaved: []int{int(RBX), int(RSI), int(RDI), int(R12), int(R13), int(R14), int(R15)},
	}
}

// Generate 生成机器码
func (cg *X64CodeGenerator) Generate(fn *IRFunc, alloc *RegAllocation) ([]byte, error) {
	cg.fn = fn
	cg.alloc = alloc
	cg.asm.Reset()
	
	// 生成函数序言
	cg.emitPrologue()
	
	// 生成基本块代码
	for _, block := range fn.Blocks {
		cg.emitBlock(block)
	}
	
	return cg.asm.Code(), nil
}

// ============================================================================
// 函数序言和尾声
// ============================================================================

// emitPrologue 生成函数序言
func (cg *X64CodeGenerator) emitPrologue() {
	// push rbp
	cg.asm.Push(RBP)
	// mov rbp, rsp
	cg.asm.MovRegReg(RBP, RSP)
	
	// 分配栈空间
	stackSize := cg.alloc.StackSize
	if stackSize > 0 {
		cg.asm.SubRegImm32(RSP, int32(stackSize))
	}
	
	// 保存被调用者保存的寄存器（如果使用了）
	// 简化实现：假设我们只使用 R10-R15，它们是调用者保存的
	
	// 将参数从参数寄存器保存到栈
	// Windows x64: RCX=arg0, RDX=arg1, R8=arg2, R9=arg3
	argRegs := []X64Reg{RCX, RDX, R8, R9}
	for i := 0; i < cg.fn.NumArgs && i < 4; i++ {
		offset := (i + 1) * -8
		cg.asm.MovMemReg(RBP, int32(offset), argRegs[i])
	}
}

// emitEpilogue 生成函数尾声
func (cg *X64CodeGenerator) emitEpilogue() {
	// mov rsp, rbp
	cg.asm.MovRegReg(RSP, RBP)
	// pop rbp
	cg.asm.Pop(RBP)
	// ret
	cg.asm.Ret()
}

// ============================================================================
// 基本块和指令生成
// ============================================================================

// emitBlock 生成基本块
func (cg *X64CodeGenerator) emitBlock(block *IRBlock) {
	// 定义块标签
	cg.asm.Label(block.ID)
	
	// 生成每条指令
	for _, instr := range block.Instrs {
		cg.emitInstr(instr)
	}
}

// emitInstr 生成单条指令
func (cg *X64CodeGenerator) emitInstr(instr *IRInstr) {
	switch instr.Op {
	case OpConst:
		cg.emitConst(instr)
	case OpLoadLocal:
		cg.emitLoadLocal(instr)
	case OpStoreLocal:
		cg.emitStoreLocal(instr)
	case OpAdd:
		cg.emitBinary(instr, cg.asm.AddRegReg)
	case OpSub:
		cg.emitBinary(instr, cg.asm.SubRegReg)
	case OpMul:
		cg.emitMul(instr)
	case OpDiv:
		cg.emitDiv(instr)
	case OpMod:
		cg.emitMod(instr)
	case OpNeg:
		cg.emitNeg(instr)
	case OpEq, OpNe, OpLt, OpLe, OpGt, OpGe:
		cg.emitCompare(instr)
	case OpNot:
		cg.emitNot(instr)
	case OpBitAnd:
		cg.emitBinary(instr, cg.asm.AndRegReg)
	case OpBitOr:
		cg.emitBinary(instr, cg.asm.OrRegReg)
	case OpBitXor:
		cg.emitBinary(instr, cg.asm.XorRegReg)
	case OpBitNot:
		cg.emitBitNot(instr)
	case OpShl:
		cg.emitShift(instr, true)
	case OpShr:
		cg.emitShift(instr, false)
	case OpJump:
		cg.emitJump(instr)
	case OpBranch:
		cg.emitBranch(instr)
	case OpReturn:
		cg.emitReturn(instr)
	case OpNop:
		// 空操作，不生成代码
	default:
		// 不支持的操作
		fmt.Printf("Warning: unsupported opcode: %s\n", instr.Op)
	}
}

// ============================================================================
// 指令生成方法
// ============================================================================

// emitConst 生成常量加载
func (cg *X64CodeGenerator) emitConst(instr *IRInstr) {
	if instr.Dest == nil {
		return
	}
	
	dst := cg.getReg(instr.Dest.ID)
	if dst == RegNone {
		return
	}
	
	var imm int64
	if instr.Dest.IsConst {
		switch instr.Dest.ConstVal.Type {
		case 2: // ValInt
			imm = instr.Dest.ConstVal.AsInt()
		case 3: // ValFloat
			// 浮点数需要特殊处理
			imm = 0
		case 1: // ValBool
			if instr.Dest.ConstVal.AsBool() {
				imm = 1
			}
		}
	}
	
	cg.asm.MovRegImm64(dst, uint64(imm))
}

// emitLoadLocal 生成局部变量加载
func (cg *X64CodeGenerator) emitLoadLocal(instr *IRInstr) {
	if instr.Dest == nil || len(instr.Args) == 0 {
		return
	}
	
	localIdx := instr.LocalIdx
	dst := cg.getReg(instr.Dest.ID)
	if dst == RegNone {
		return
	}
	
	// 局部变量在 [rbp - (idx+1)*8]
	offset := int32((localIdx + 1) * -8)
	cg.asm.MovRegMem(dst, RBP, offset)
}

// emitStoreLocal 生成局部变量存储
func (cg *X64CodeGenerator) emitStoreLocal(instr *IRInstr) {
	if len(instr.Args) == 0 {
		return
	}
	
	localIdx := instr.LocalIdx
	src := cg.loadValue(instr.Args[0], RAX)
	
	// 存储到 [rbp - (idx+1)*8]
	offset := int32((localIdx + 1) * -8)
	cg.asm.MovMemReg(RBP, offset, src)
}

// emitBinary 生成二元运算
func (cg *X64CodeGenerator) emitBinary(instr *IRInstr, op func(dst, src X64Reg)) {
	if instr.Dest == nil || len(instr.Args) < 2 {
		return
	}
	
	dst := cg.getReg(instr.Dest.ID)
	if dst == RegNone {
		dst = RAX // 如果目标被溢出，使用 RAX
	}
	
	// 加载左操作数
	left := cg.loadValue(instr.Args[0], dst)
	if left != dst {
		cg.asm.MovRegReg(dst, left)
	}
	
	// 加载右操作数
	right := cg.loadValue(instr.Args[1], R11)
	
	// 执行运算
	op(dst, right)
	
	// 如果目标被溢出，存回栈
	if cg.alloc.IsSpilled(instr.Dest.ID) {
		slot := cg.alloc.GetSpillSlot(instr.Dest.ID)
		offset := cg.getSpillOffset(slot)
		cg.asm.MovMemReg(RBP, offset, dst)
	}
}

// emitMul 生成乘法
func (cg *X64CodeGenerator) emitMul(instr *IRInstr) {
	if instr.Dest == nil || len(instr.Args) < 2 {
		return
	}
	
	dst := cg.getReg(instr.Dest.ID)
	if dst == RegNone {
		dst = RAX
	}
	
	left := cg.loadValue(instr.Args[0], dst)
	if left != dst {
		cg.asm.MovRegReg(dst, left)
	}
	
	right := cg.loadValue(instr.Args[1], R11)
	cg.asm.IMulRegReg(dst, right)
	
	if cg.alloc.IsSpilled(instr.Dest.ID) {
		slot := cg.alloc.GetSpillSlot(instr.Dest.ID)
		offset := cg.getSpillOffset(slot)
		cg.asm.MovMemReg(RBP, offset, dst)
	}
}

// emitDiv 生成除法
func (cg *X64CodeGenerator) emitDiv(instr *IRInstr) {
	if instr.Dest == nil || len(instr.Args) < 2 {
		return
	}
	
	// 除法使用 RDX:RAX / src -> RAX (商), RDX (余数)
	left := cg.loadValue(instr.Args[0], RAX)
	if left != RAX {
		cg.asm.MovRegReg(RAX, left)
	}
	
	right := cg.loadValue(instr.Args[1], R11)
	
	// 符号扩展
	cg.asm.CQO()
	// 除法
	cg.asm.IDivReg(right)
	
	// 结果在 RAX
	dst := cg.getReg(instr.Dest.ID)
	if dst != RegNone && dst != RAX {
		cg.asm.MovRegReg(dst, RAX)
	} else if cg.alloc.IsSpilled(instr.Dest.ID) {
		slot := cg.alloc.GetSpillSlot(instr.Dest.ID)
		offset := cg.getSpillOffset(slot)
		cg.asm.MovMemReg(RBP, offset, RAX)
	}
}

// emitMod 生成取模
func (cg *X64CodeGenerator) emitMod(instr *IRInstr) {
	if instr.Dest == nil || len(instr.Args) < 2 {
		return
	}
	
	left := cg.loadValue(instr.Args[0], RAX)
	if left != RAX {
		cg.asm.MovRegReg(RAX, left)
	}
	
	right := cg.loadValue(instr.Args[1], R11)
	
	cg.asm.CQO()
	cg.asm.IDivReg(right)
	
	// 余数在 RDX
	dst := cg.getReg(instr.Dest.ID)
	if dst != RegNone {
		cg.asm.MovRegReg(dst, RDX)
	} else if cg.alloc.IsSpilled(instr.Dest.ID) {
		slot := cg.alloc.GetSpillSlot(instr.Dest.ID)
		offset := cg.getSpillOffset(slot)
		cg.asm.MovMemReg(RBP, offset, RDX)
	}
}

// emitNeg 生成取负
func (cg *X64CodeGenerator) emitNeg(instr *IRInstr) {
	if instr.Dest == nil || len(instr.Args) == 0 {
		return
	}
	
	dst := cg.getReg(instr.Dest.ID)
	if dst == RegNone {
		dst = RAX
	}
	
	src := cg.loadValue(instr.Args[0], dst)
	if src != dst {
		cg.asm.MovRegReg(dst, src)
	}
	
	cg.asm.Neg(dst)
	
	if cg.alloc.IsSpilled(instr.Dest.ID) {
		slot := cg.alloc.GetSpillSlot(instr.Dest.ID)
		offset := cg.getSpillOffset(slot)
		cg.asm.MovMemReg(RBP, offset, dst)
	}
}

// emitCompare 生成比较
func (cg *X64CodeGenerator) emitCompare(instr *IRInstr) {
	if instr.Dest == nil || len(instr.Args) < 2 {
		return
	}
	
	left := cg.loadValue(instr.Args[0], RAX)
	right := cg.loadValue(instr.Args[1], R11)
	
	cg.asm.CmpRegReg(left, right)
	
	// 使用 SETcc 设置结果
	dst := cg.getReg(instr.Dest.ID)
	if dst == RegNone {
		dst = RAX
	}
	
	switch instr.Op {
	case OpEq:
		cg.asm.SetE(dst)
	case OpNe:
		cg.asm.SetNE(dst)
	case OpLt:
		cg.asm.SetL(dst)
	case OpLe:
		cg.asm.SetLE(dst)
	case OpGt:
		cg.asm.SetG(dst)
	case OpGe:
		cg.asm.SetGE(dst)
	}
	
	// 零扩展到 64 位
	cg.asm.MovzxReg8(dst, dst)
	
	if cg.alloc.IsSpilled(instr.Dest.ID) {
		slot := cg.alloc.GetSpillSlot(instr.Dest.ID)
		offset := cg.getSpillOffset(slot)
		cg.asm.MovMemReg(RBP, offset, dst)
	}
}

// emitNot 生成逻辑非
func (cg *X64CodeGenerator) emitNot(instr *IRInstr) {
	if instr.Dest == nil || len(instr.Args) == 0 {
		return
	}
	
	dst := cg.getReg(instr.Dest.ID)
	if dst == RegNone {
		dst = RAX
	}
	
	src := cg.loadValue(instr.Args[0], dst)
	
	// 测试源值
	cg.asm.TestRegReg(src, src)
	// 如果为 0 则设置为 1
	cg.asm.SetE(dst)
	cg.asm.MovzxReg8(dst, dst)
	
	if cg.alloc.IsSpilled(instr.Dest.ID) {
		slot := cg.alloc.GetSpillSlot(instr.Dest.ID)
		offset := cg.getSpillOffset(slot)
		cg.asm.MovMemReg(RBP, offset, dst)
	}
}

// emitBitNot 生成位非
func (cg *X64CodeGenerator) emitBitNot(instr *IRInstr) {
	if instr.Dest == nil || len(instr.Args) == 0 {
		return
	}
	
	dst := cg.getReg(instr.Dest.ID)
	if dst == RegNone {
		dst = RAX
	}
	
	src := cg.loadValue(instr.Args[0], dst)
	if src != dst {
		cg.asm.MovRegReg(dst, src)
	}
	
	cg.asm.NotReg(dst)
	
	if cg.alloc.IsSpilled(instr.Dest.ID) {
		slot := cg.alloc.GetSpillSlot(instr.Dest.ID)
		offset := cg.getSpillOffset(slot)
		cg.asm.MovMemReg(RBP, offset, dst)
	}
}

// emitShift 生成移位
func (cg *X64CodeGenerator) emitShift(instr *IRInstr, isLeft bool) {
	if instr.Dest == nil || len(instr.Args) < 2 {
		return
	}
	
	dst := cg.getReg(instr.Dest.ID)
	if dst == RegNone {
		dst = RAX
	}
	
	left := cg.loadValue(instr.Args[0], dst)
	if left != dst {
		cg.asm.MovRegReg(dst, left)
	}
	
	// 移位量
	if instr.Args[1].IsConst {
		shift := byte(instr.Args[1].ConstVal.AsInt())
		if isLeft {
			cg.asm.ShlRegImm(dst, shift)
		} else {
			cg.asm.SarRegImm(dst, shift)
		}
	} else {
		// 移位量在 CL 中
		right := cg.loadValue(instr.Args[1], RCX)
		if right != RCX {
			cg.asm.MovRegReg(RCX, right)
		}
		if isLeft {
			cg.asm.ShlRegCL(dst)
		} else {
			cg.asm.SarRegCL(dst)
		}
	}
	
	if cg.alloc.IsSpilled(instr.Dest.ID) {
		slot := cg.alloc.GetSpillSlot(instr.Dest.ID)
		offset := cg.getSpillOffset(slot)
		cg.asm.MovMemReg(RBP, offset, dst)
	}
}

// emitJump 生成无条件跳转
func (cg *X64CodeGenerator) emitJump(instr *IRInstr) {
	if len(instr.Targets) > 0 {
		cg.asm.Jmp(instr.Targets[0].ID)
	}
}

// emitBranch 生成条件跳转
func (cg *X64CodeGenerator) emitBranch(instr *IRInstr) {
	if len(instr.Args) == 0 || len(instr.Targets) < 2 {
		return
	}
	
	cond := cg.loadValue(instr.Args[0], RAX)
	
	// 测试条件
	cg.asm.TestRegReg(cond, cond)
	
	// 条件为真（非零）跳转到 Targets[0]，否则跳转到 Targets[1]
	cg.asm.Jne(instr.Targets[0].ID)
	cg.asm.Jmp(instr.Targets[1].ID)
}

// emitReturn 生成返回
func (cg *X64CodeGenerator) emitReturn(instr *IRInstr) {
	if len(instr.Args) > 0 && instr.Args[0] != nil {
		// 加载返回值到 RAX
		ret := cg.loadValue(instr.Args[0], RAX)
		if ret != RAX {
			cg.asm.MovRegReg(RAX, ret)
		}
	} else {
		// 无返回值，设置 RAX = 0
		cg.asm.XorRegReg(RAX, RAX)
	}
	
	cg.emitEpilogue()
}

// ============================================================================
// 辅助方法
// ============================================================================

// getReg 获取值对应的物理寄存器
func (cg *X64CodeGenerator) getReg(valueID int) X64Reg {
	regIdx := cg.alloc.GetReg(valueID)
	if regIdx < 0 || regIdx >= len(cg.physRegs) {
		return RegNone
	}
	return cg.physRegs[regIdx]
}

// loadValue 加载值到寄存器
func (cg *X64CodeGenerator) loadValue(v *IRValue, hint X64Reg) X64Reg {
	if v == nil {
		return hint
	}
	
	// 常量
	if v.IsConst {
		var imm int64
		switch v.Type {
		case TypeInt:
			imm = v.ConstVal.AsInt()
		case TypeBool:
			if v.ConstVal.AsBool() {
				imm = 1
			}
		case TypeFloat:
			// 简化：将浮点数转为整数
			imm = int64(v.ConstVal.AsFloat())
		}
		cg.asm.MovRegImm64(hint, uint64(imm))
		return hint
	}
	
	// 在寄存器中
	reg := cg.getReg(v.ID)
	if reg != RegNone {
		return reg
	}
	
	// 在栈上（溢出的值）
	if cg.alloc.IsSpilled(v.ID) {
		slot := cg.alloc.GetSpillSlot(v.ID)
		offset := cg.getSpillOffset(slot)
		cg.asm.MovRegMem(hint, RBP, offset)
		return hint
	}
	
	return hint
}

// getSpillOffset 获取溢出槽的栈偏移
func (cg *X64CodeGenerator) getSpillOffset(slot int) int32 {
	// 溢出槽在参数保存区之后
	// [rbp-8] 到 [rbp-32] 是参数保存区（4 个参数）
	// 溢出槽从 [rbp-40] 开始
	paramSpace := cg.fn.NumArgs * 8
	if paramSpace < 32 {
		paramSpace = 32
	}
	return int32(-(paramSpace + (slot+1)*8))
}
