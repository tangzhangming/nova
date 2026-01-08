// arm64_codegen.go - ARM64 代码生成器
//
// 本文件实现了从 IR 到 ARM64 机器码的转换。
//
// ARM64 AAPCS64 调用约定：
// - 参数传递：X0-X7（前 8 个整数/指针参数）
// - 返回值：X0
// - 调用者保存：X0-X18
// - 被调用者保存：X19-X28, X29(FP), X30(LR)
// - 栈对齐：16 字节
//
// 寄存器分配：
// - X0: 返回值和第一个参数
// - X19-X28: 可分配给 IR 值
// - X29(FP): 帧指针
// - X30(LR): 返回地址

package jit

import (
	"fmt"
)

// ============================================================================
// ARM64 代码生成器
// ============================================================================

// ARM64CodeGenerator ARM64 代码生成器
type ARM64CodeGenerator struct {
	asm    *ARM64Assembler
	alloc  *RegAllocation
	fn     *IRFunc
	
	// 可分配寄存器
	physRegs []ARM64Reg
}

// NewARM64CodeGenerator 创建代码生成器
func NewARM64CodeGenerator() *ARM64CodeGenerator {
	return &ARM64CodeGenerator{
		asm: NewARM64Assembler(),
		// 使用 X19-X28 作为可分配寄存器（被调用者保存）
		physRegs: []ARM64Reg{
			X19, X20, X21, X22, X23,
			X24, X25, X26, X27, X28,
		},
	}
}

// NumRegisters 返回可用寄存器数量
func (cg *ARM64CodeGenerator) NumRegisters() int {
	return len(cg.physRegs)
}

// CallingConvention 返回调用约定
func (cg *ARM64CodeGenerator) CallingConvention() CallingConv {
	return CallingConv{
		ArgRegs:     []int{int(X0), int(X1), int(X2), int(X3), int(X4), int(X5), int(X6), int(X7)},
		RetReg:      int(X0),
		CallerSaved: []int{int(X0), int(X1), int(X2), int(X3), int(X4), int(X5), int(X6), int(X7), 
			int(X8), int(X9), int(X10), int(X11), int(X12), int(X13), int(X14), int(X15), int(X16), int(X17), int(X18)},
		CalleeSaved: []int{int(X19), int(X20), int(X21), int(X22), int(X23), 
			int(X24), int(X25), int(X26), int(X27), int(X28), int(X29), int(X30)},
	}
}

// Generate 生成机器码
func (cg *ARM64CodeGenerator) Generate(fn *IRFunc, alloc *RegAllocation) ([]byte, error) {
	cg.fn = fn
	cg.alloc = alloc
	cg.asm.Reset()
	
	// 生成函数序言
	cg.emitPrologue()
	
	// 生成基本块
	for _, block := range fn.Blocks {
		cg.emitBlock(block)
	}
	
	return cg.asm.Code(), nil
}

// ============================================================================
// 函数序言和尾声
// ============================================================================

// emitPrologue 生成函数序言
func (cg *ARM64CodeGenerator) emitPrologue() {
	// 保存 FP 和 LR
	// stp x29, x30, [sp, #-16]!
	cg.asm.StpPre(X29, X30, XSP, -16)
	
	// 设置帧指针
	// mov x29, sp
	cg.asm.MovRegReg(X29, XSP)
	
	// 分配栈空间
	stackSize := cg.alloc.StackSize
	if stackSize > 0 {
		// 对齐到 16 字节
		stackSize = (stackSize + 15) &^ 15
		cg.asm.SubRegImm12(XSP, XSP, uint32(stackSize))
	}
	
	// 保存参数到栈
	// AAPCS64: X0-X7 是参数寄存器
	argRegs := []ARM64Reg{X0, X1, X2, X3, X4, X5, X6, X7}
	for i := 0; i < cg.fn.NumArgs && i < 8; i++ {
		offset := int32((i + 1) * -8)
		cg.asm.StrRegMem(argRegs[i], X29, offset)
	}
}

// emitEpilogue 生成函数尾声
func (cg *ARM64CodeGenerator) emitEpilogue() {
	// 恢复 SP
	// mov sp, x29
	cg.asm.MovRegReg(XSP, X29)
	
	// 恢复 FP 和 LR
	// ldp x29, x30, [sp], #16
	cg.asm.LdpPost(X29, X30, XSP, 16)
	
	// 返回
	cg.asm.Ret()
}

// ============================================================================
// 基本块和指令生成
// ============================================================================

// emitBlock 生成基本块
func (cg *ARM64CodeGenerator) emitBlock(block *IRBlock) {
	cg.asm.Label(block.ID)
	
	for _, instr := range block.Instrs {
		cg.emitInstr(instr)
	}
}

// emitInstr 生成单条指令
func (cg *ARM64CodeGenerator) emitInstr(instr *IRInstr) {
	switch instr.Op {
	case OpConst:
		cg.emitConst(instr)
	case OpLoadLocal:
		cg.emitLoadLocal(instr)
	case OpStoreLocal:
		cg.emitStoreLocal(instr)
	case OpAdd:
		cg.emitAdd(instr)
	case OpSub:
		cg.emitSub(instr)
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
		cg.emitBitAnd(instr)
	case OpBitOr:
		cg.emitBitOr(instr)
	case OpBitXor:
		cg.emitBitXor(instr)
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
		// 空操作
	default:
		fmt.Printf("Warning: unsupported ARM64 opcode: %s\n", instr.Op)
	}
}

// ============================================================================
// 指令生成方法
// ============================================================================

// emitConst 生成常量加载
func (cg *ARM64CodeGenerator) emitConst(instr *IRInstr) {
	if instr.Dest == nil {
		return
	}
	
	dst := cg.getReg(instr.Dest.ID)
	if dst == ARM64RegNone {
		dst = X0
	}
	
	var imm int64
	if instr.Dest.IsConst {
		switch instr.Dest.ConstVal.Type {
		case 2: // ValInt
			imm = instr.Dest.ConstVal.AsInt()
		case 1: // ValBool
			if instr.Dest.ConstVal.AsBool() {
				imm = 1
			}
		}
	}
	
	cg.asm.MovRegImm64(dst, uint64(imm))
	
	if cg.alloc.IsSpilled(instr.Dest.ID) {
		slot := cg.alloc.GetSpillSlot(instr.Dest.ID)
		offset := cg.getSpillOffset(slot)
		cg.asm.StrRegMem(dst, X29, offset)
	}
}

// emitLoadLocal 生成局部变量加载
func (cg *ARM64CodeGenerator) emitLoadLocal(instr *IRInstr) {
	if instr.Dest == nil {
		return
	}
	
	localIdx := instr.LocalIdx
	dst := cg.getReg(instr.Dest.ID)
	if dst == ARM64RegNone {
		dst = X0
	}
	
	offset := int32((localIdx + 1) * -8)
	cg.asm.LdrRegMem(dst, X29, offset)
	
	if cg.alloc.IsSpilled(instr.Dest.ID) {
		slot := cg.alloc.GetSpillSlot(instr.Dest.ID)
		spillOffset := cg.getSpillOffset(slot)
		cg.asm.StrRegMem(dst, X29, spillOffset)
	}
}

// emitStoreLocal 生成局部变量存储
func (cg *ARM64CodeGenerator) emitStoreLocal(instr *IRInstr) {
	if len(instr.Args) == 0 {
		return
	}
	
	localIdx := instr.LocalIdx
	src := cg.loadValue(instr.Args[0], X0)
	
	offset := int32((localIdx + 1) * -8)
	cg.asm.StrRegMem(src, X29, offset)
}

// emitAdd 生成加法
func (cg *ARM64CodeGenerator) emitAdd(instr *IRInstr) {
	if instr.Dest == nil || len(instr.Args) < 2 {
		return
	}
	
	dst := cg.getReg(instr.Dest.ID)
	if dst == ARM64RegNone {
		dst = X0
	}
	
	left := cg.loadValue(instr.Args[0], X16)
	right := cg.loadValue(instr.Args[1], X17)
	
	cg.asm.AddRegReg(dst, left, right)
	
	if cg.alloc.IsSpilled(instr.Dest.ID) {
		slot := cg.alloc.GetSpillSlot(instr.Dest.ID)
		offset := cg.getSpillOffset(slot)
		cg.asm.StrRegMem(dst, X29, offset)
	}
}

// emitSub 生成减法
func (cg *ARM64CodeGenerator) emitSub(instr *IRInstr) {
	if instr.Dest == nil || len(instr.Args) < 2 {
		return
	}
	
	dst := cg.getReg(instr.Dest.ID)
	if dst == ARM64RegNone {
		dst = X0
	}
	
	left := cg.loadValue(instr.Args[0], X16)
	right := cg.loadValue(instr.Args[1], X17)
	
	cg.asm.SubRegReg(dst, left, right)
	
	if cg.alloc.IsSpilled(instr.Dest.ID) {
		slot := cg.alloc.GetSpillSlot(instr.Dest.ID)
		offset := cg.getSpillOffset(slot)
		cg.asm.StrRegMem(dst, X29, offset)
	}
}

// emitMul 生成乘法
func (cg *ARM64CodeGenerator) emitMul(instr *IRInstr) {
	if instr.Dest == nil || len(instr.Args) < 2 {
		return
	}
	
	dst := cg.getReg(instr.Dest.ID)
	if dst == ARM64RegNone {
		dst = X0
	}
	
	left := cg.loadValue(instr.Args[0], X16)
	right := cg.loadValue(instr.Args[1], X17)
	
	cg.asm.MulReg(dst, left, right)
	
	if cg.alloc.IsSpilled(instr.Dest.ID) {
		slot := cg.alloc.GetSpillSlot(instr.Dest.ID)
		offset := cg.getSpillOffset(slot)
		cg.asm.StrRegMem(dst, X29, offset)
	}
}

// emitDiv 生成除法
func (cg *ARM64CodeGenerator) emitDiv(instr *IRInstr) {
	if instr.Dest == nil || len(instr.Args) < 2 {
		return
	}
	
	dst := cg.getReg(instr.Dest.ID)
	if dst == ARM64RegNone {
		dst = X0
	}
	
	left := cg.loadValue(instr.Args[0], X16)
	right := cg.loadValue(instr.Args[1], X17)
	
	cg.asm.SdivReg(dst, left, right)
	
	if cg.alloc.IsSpilled(instr.Dest.ID) {
		slot := cg.alloc.GetSpillSlot(instr.Dest.ID)
		offset := cg.getSpillOffset(slot)
		cg.asm.StrRegMem(dst, X29, offset)
	}
}

// emitMod 生成取模
func (cg *ARM64CodeGenerator) emitMod(instr *IRInstr) {
	if instr.Dest == nil || len(instr.Args) < 2 {
		return
	}
	
	dst := cg.getReg(instr.Dest.ID)
	if dst == ARM64RegNone {
		dst = X0
	}
	
	left := cg.loadValue(instr.Args[0], X16)
	right := cg.loadValue(instr.Args[1], X17)
	
	// ARM64 没有直接的取模指令
	// a % b = a - (a / b) * b
	cg.asm.SdivReg(X15, left, right)   // X15 = a / b
	cg.asm.MsubReg(dst, X15, right, left) // dst = a - (a/b) * b
	
	if cg.alloc.IsSpilled(instr.Dest.ID) {
		slot := cg.alloc.GetSpillSlot(instr.Dest.ID)
		offset := cg.getSpillOffset(slot)
		cg.asm.StrRegMem(dst, X29, offset)
	}
}

// emitNeg 生成取负
func (cg *ARM64CodeGenerator) emitNeg(instr *IRInstr) {
	if instr.Dest == nil || len(instr.Args) == 0 {
		return
	}
	
	dst := cg.getReg(instr.Dest.ID)
	if dst == ARM64RegNone {
		dst = X0
	}
	
	src := cg.loadValue(instr.Args[0], X16)
	cg.asm.NegReg(dst, src)
	
	if cg.alloc.IsSpilled(instr.Dest.ID) {
		slot := cg.alloc.GetSpillSlot(instr.Dest.ID)
		offset := cg.getSpillOffset(slot)
		cg.asm.StrRegMem(dst, X29, offset)
	}
}

// emitCompare 生成比较
func (cg *ARM64CodeGenerator) emitCompare(instr *IRInstr) {
	if instr.Dest == nil || len(instr.Args) < 2 {
		return
	}
	
	dst := cg.getReg(instr.Dest.ID)
	if dst == ARM64RegNone {
		dst = X0
	}
	
	left := cg.loadValue(instr.Args[0], X16)
	right := cg.loadValue(instr.Args[1], X17)
	
	cg.asm.CmpRegReg(left, right)
	
	// 使用 CSET 设置结果
	var cond uint32
	switch instr.Op {
	case OpEq:
		cond = CondEQ
	case OpNe:
		cond = CondNE
	case OpLt:
		cond = CondLT
	case OpLe:
		cond = CondLE
	case OpGt:
		cond = CondGT
	case OpGe:
		cond = CondGE
	}
	
	cg.asm.Cset(dst, cond)
	
	if cg.alloc.IsSpilled(instr.Dest.ID) {
		slot := cg.alloc.GetSpillSlot(instr.Dest.ID)
		offset := cg.getSpillOffset(slot)
		cg.asm.StrRegMem(dst, X29, offset)
	}
}

// emitNot 生成逻辑非
func (cg *ARM64CodeGenerator) emitNot(instr *IRInstr) {
	if instr.Dest == nil || len(instr.Args) == 0 {
		return
	}
	
	dst := cg.getReg(instr.Dest.ID)
	if dst == ARM64RegNone {
		dst = X0
	}
	
	src := cg.loadValue(instr.Args[0], X16)
	
	// 比较 src 与 0
	cg.asm.CmpRegImm12(src, 0)
	// 如果等于 0，设置为 1
	cg.asm.Cset(dst, CondEQ)
	
	if cg.alloc.IsSpilled(instr.Dest.ID) {
		slot := cg.alloc.GetSpillSlot(instr.Dest.ID)
		offset := cg.getSpillOffset(slot)
		cg.asm.StrRegMem(dst, X29, offset)
	}
}

// emitBitAnd 生成位与
func (cg *ARM64CodeGenerator) emitBitAnd(instr *IRInstr) {
	cg.emitBitOp(instr, cg.asm.AndRegReg)
}

// emitBitOr 生成位或
func (cg *ARM64CodeGenerator) emitBitOr(instr *IRInstr) {
	cg.emitBitOp(instr, cg.asm.OrrRegReg)
}

// emitBitXor 生成位异或
func (cg *ARM64CodeGenerator) emitBitXor(instr *IRInstr) {
	cg.emitBitOp(instr, cg.asm.EorRegReg)
}

// emitBitOp 生成位运算
func (cg *ARM64CodeGenerator) emitBitOp(instr *IRInstr, op func(dst, src1, src2 ARM64Reg)) {
	if instr.Dest == nil || len(instr.Args) < 2 {
		return
	}
	
	dst := cg.getReg(instr.Dest.ID)
	if dst == ARM64RegNone {
		dst = X0
	}
	
	left := cg.loadValue(instr.Args[0], X16)
	right := cg.loadValue(instr.Args[1], X17)
	
	op(dst, left, right)
	
	if cg.alloc.IsSpilled(instr.Dest.ID) {
		slot := cg.alloc.GetSpillSlot(instr.Dest.ID)
		offset := cg.getSpillOffset(slot)
		cg.asm.StrRegMem(dst, X29, offset)
	}
}

// emitBitNot 生成位非
func (cg *ARM64CodeGenerator) emitBitNot(instr *IRInstr) {
	if instr.Dest == nil || len(instr.Args) == 0 {
		return
	}
	
	dst := cg.getReg(instr.Dest.ID)
	if dst == ARM64RegNone {
		dst = X0
	}
	
	src := cg.loadValue(instr.Args[0], X16)
	cg.asm.MvnReg(dst, src)
	
	if cg.alloc.IsSpilled(instr.Dest.ID) {
		slot := cg.alloc.GetSpillSlot(instr.Dest.ID)
		offset := cg.getSpillOffset(slot)
		cg.asm.StrRegMem(dst, X29, offset)
	}
}

// emitShift 生成移位
func (cg *ARM64CodeGenerator) emitShift(instr *IRInstr, isLeft bool) {
	if instr.Dest == nil || len(instr.Args) < 2 {
		return
	}
	
	dst := cg.getReg(instr.Dest.ID)
	if dst == ARM64RegNone {
		dst = X0
	}
	
	left := cg.loadValue(instr.Args[0], X16)
	
	if instr.Args[1].IsConst {
		shift := uint32(instr.Args[1].ConstVal.AsInt())
		if isLeft {
			cg.asm.LslImm(dst, left, shift)
		} else {
			cg.asm.AsrImm(dst, left, shift)
		}
	} else {
		right := cg.loadValue(instr.Args[1], X17)
		if isLeft {
			cg.asm.LslReg(dst, left, right)
		} else {
			cg.asm.AsrReg(dst, left, right)
		}
	}
	
	if cg.alloc.IsSpilled(instr.Dest.ID) {
		slot := cg.alloc.GetSpillSlot(instr.Dest.ID)
		offset := cg.getSpillOffset(slot)
		cg.asm.StrRegMem(dst, X29, offset)
	}
}

// emitJump 生成无条件跳转
func (cg *ARM64CodeGenerator) emitJump(instr *IRInstr) {
	if len(instr.Targets) > 0 {
		cg.asm.B(instr.Targets[0].ID)
	}
}

// emitBranch 生成条件跳转
func (cg *ARM64CodeGenerator) emitBranch(instr *IRInstr) {
	if len(instr.Args) == 0 || len(instr.Targets) < 2 {
		return
	}
	
	cond := cg.loadValue(instr.Args[0], X0)
	
	// 条件为真（非零）跳转到 Targets[0]
	cg.asm.Cbnz(cond, instr.Targets[0].ID)
	// 否则跳转到 Targets[1]
	cg.asm.B(instr.Targets[1].ID)
}

// emitReturn 生成返回
func (cg *ARM64CodeGenerator) emitReturn(instr *IRInstr) {
	if len(instr.Args) > 0 && instr.Args[0] != nil {
		ret := cg.loadValue(instr.Args[0], X0)
		if ret != X0 {
			cg.asm.MovRegReg(X0, ret)
		}
	} else {
		// 无返回值
		cg.asm.MovRegImm64(X0, 0)
	}
	
	cg.emitEpilogue()
}

// ============================================================================
// 辅助方法
// ============================================================================

// getReg 获取值对应的物理寄存器
func (cg *ARM64CodeGenerator) getReg(valueID int) ARM64Reg {
	regIdx := cg.alloc.GetReg(valueID)
	if regIdx < 0 || regIdx >= len(cg.physRegs) {
		return ARM64RegNone
	}
	return cg.physRegs[regIdx]
}

// loadValue 加载值到寄存器
func (cg *ARM64CodeGenerator) loadValue(v *IRValue, hint ARM64Reg) ARM64Reg {
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
			imm = int64(v.ConstVal.AsFloat())
		}
		cg.asm.MovRegImm64(hint, uint64(imm))
		return hint
	}
	
	// 在寄存器中
	reg := cg.getReg(v.ID)
	if reg != ARM64RegNone {
		return reg
	}
	
	// 在栈上
	if cg.alloc.IsSpilled(v.ID) {
		slot := cg.alloc.GetSpillSlot(v.ID)
		offset := cg.getSpillOffset(slot)
		cg.asm.LdrRegMem(hint, X29, offset)
		return hint
	}
	
	return hint
}

// getSpillOffset 获取溢出槽的栈偏移
func (cg *ARM64CodeGenerator) getSpillOffset(slot int) int32 {
	// 参数保存在 [fp-8] 到 [fp-64]（最多 8 个参数）
	paramSpace := cg.fn.NumArgs * 8
	if paramSpace < 64 {
		paramSpace = 64
	}
	return int32(-(paramSpace + (slot+1)*8))
}
