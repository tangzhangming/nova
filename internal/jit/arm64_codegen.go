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
	case OpArrayLen:
		cg.emitArrayLen(instr)
	case OpArrayGet:
		cg.emitArrayGet(instr)
	case OpArraySet:
		cg.emitArraySet(instr)
	case OpNop:
		// 空操作
	
	// 函数调用
	case OpCall, OpCallDirect:
		cg.emitCall(instr)
	case OpCallIndirect:
		cg.emitCallIndirect(instr)
	case OpCallBuiltin:
		cg.emitCallBuiltin(instr)
	case OpCallMethod, OpCallVirtual:
		cg.emitCallMethod(instr)
	case OpTailCall:
		cg.emitTailCall(instr)
	
	// 对象操作
	case OpNewObject:
		cg.emitNewObject(instr)
	case OpGetField:
		cg.emitGetField(instr)
	case OpSetField:
		cg.emitSetField(instr)
	case OpLoadVTable:
		cg.emitLoadVTable(instr)
	
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

// ============================================================================
// 数组操作指令生成
// ============================================================================

// emitArrayLen 生成数组长度操作
func (cg *ARM64CodeGenerator) emitArrayLen(instr *IRInstr) {
	if instr.Dest == nil || len(instr.Args) == 0 {
		return
	}
	
	// 加载数组指针到 X0（AAPCS64 第一个参数）
	arr := cg.loadValue(instr.Args[0], X0)
	if arr != X0 {
		cg.asm.MovRegReg(X0, arr)
	}
	
	// 调用 ArrayLenHelper
	helperAddr := GetArrayLenHelperPtr()
	cg.emitCallHelper(helperAddr)
	
	// 结果在 X0
	dst := cg.getReg(instr.Dest.ID)
	if dst != ARM64RegNone && dst != X0 {
		cg.asm.MovRegReg(dst, X0)
	} else if cg.alloc.IsSpilled(instr.Dest.ID) {
		slot := cg.alloc.GetSpillSlot(instr.Dest.ID)
		offset := cg.getSpillOffset(slot)
		cg.asm.StrRegMem(X0, X29, offset)
	}
}

// emitArrayGet 生成数组取元素操作
func (cg *ARM64CodeGenerator) emitArrayGet(instr *IRInstr) {
	if instr.Dest == nil || len(instr.Args) < 2 {
		return
	}
	
	// 加载数组指针到 X0
	arr := cg.loadValue(instr.Args[0], X0)
	if arr != X0 {
		cg.asm.MovRegReg(X0, arr)
	}
	
	// 加载索引到 X1
	index := cg.loadValue(instr.Args[1], X1)
	if index != X1 {
		cg.asm.MovRegReg(X1, index)
	}
	
	// 调用 ArrayGetHelper
	helperAddr := GetArrayGetHelperPtr()
	cg.emitCallHelper(helperAddr)
	
	// 结果在 X0（值），X1（成功标志）
	dst := cg.getReg(instr.Dest.ID)
	if dst != ARM64RegNone && dst != X0 {
		cg.asm.MovRegReg(dst, X0)
	} else if cg.alloc.IsSpilled(instr.Dest.ID) {
		slot := cg.alloc.GetSpillSlot(instr.Dest.ID)
		offset := cg.getSpillOffset(slot)
		cg.asm.StrRegMem(X0, X29, offset)
	}
}

// emitArraySet 生成数组设元素操作
func (cg *ARM64CodeGenerator) emitArraySet(instr *IRInstr) {
	if len(instr.Args) < 3 {
		return
	}
	
	// 加载数组指针到 X0
	arr := cg.loadValue(instr.Args[0], X0)
	if arr != X0 {
		cg.asm.MovRegReg(X0, arr)
	}
	
	// 加载索引到 X1
	index := cg.loadValue(instr.Args[1], X1)
	if index != X1 {
		cg.asm.MovRegReg(X1, index)
	}
	
	// 加载值到 X2
	value := cg.loadValue(instr.Args[2], X2)
	if value != X2 {
		cg.asm.MovRegReg(X2, value)
	}
	
	// 调用 ArraySetHelper
	helperAddr := GetArraySetHelperPtr()
	cg.emitCallHelper(helperAddr)
	
	// 结果在 X0（成功标志），可以忽略
}

// emitCallHelper 生成调用运行时辅助函数的代码
// AAPCS64 调用约定：
// - 参数: X0-X7
// - 返回值: X0, X1
// - 调用者保存: X0-X18
// BUG FIX: 添加调用者保存寄存器的保存/恢复，防止复杂调用链中寄存器被破坏导致崩溃
func (cg *ARM64CodeGenerator) emitCallHelper(addr uintptr) {
	// 保存调用者保存的寄存器
	cg.saveCallerSavedRegs()
	
	// 将地址加载到 X9（临时寄存器）并调用
	cg.asm.MovRegImm64(X9, uint64(addr))
	cg.asm.Blr(X9)
	
	// 恢复调用者保存的寄存器
	cg.restoreCallerSavedRegs()
}

// ============================================================================
// 函数调用指令生成
// ============================================================================

// arm64CallerSavedRegs ARM64 调用者需要保存的寄存器
var arm64CallerSavedRegs = []ARM64Reg{X9, X10, X11, X12, X13, X14, X15}

// emitCall 生成函数调用指令
// AAPCS64 调用约定：
//   - 参数：X0-X7（前8个），其余通过栈传递
//   - 返回值：X0
//   - 栈对齐：16字节
func (cg *ARM64CodeGenerator) emitCall(instr *IRInstr) {
	argCount := len(instr.Args)
	argRegs := []ARM64Reg{X0, X1, X2, X3, X4, X5, X6, X7}
	
	// 保存调用者保存的寄存器
	cg.saveCallerSavedRegs()
	
	// 计算栈空间（用于超过8个参数的情况）
	stackSpace := int32(0)
	if argCount > 8 {
		stackSpace = int32((argCount - 8) * 8)
		// 对齐到16字节
		if (stackSpace % 16) != 0 {
			stackSpace += 8
		}
		cg.asm.SubRegImm12(XSP, XSP, uint32(stackSpace))
	}
	
	// 加载参数
	// 前8个参数放入寄存器
	for i := 0; i < argCount && i < 8; i++ {
		src := cg.loadValue(instr.Args[i], X16)
		if src != argRegs[i] {
			cg.asm.MovRegReg(argRegs[i], src)
		}
	}
	
	// 额外参数放入栈
	for i := 8; i < argCount; i++ {
		src := cg.loadValue(instr.Args[i], X16)
		offset := int32((i - 8) * 8)
		cg.asm.StrRegMem(src, XSP, offset)
	}
	
	// 获取函数地址并调用
	helperAddr := GetCallHelperPtr()
	cg.asm.MovRegImm64(X9, uint64(helperAddr))
	cg.asm.Blr(X9)
	
	// 恢复栈
	if stackSpace > 0 {
		cg.asm.AddRegImm12(XSP, XSP, uint32(stackSpace))
	}
	
	// 恢复调用者保存的寄存器
	cg.restoreCallerSavedRegs()
	
	// 处理返回值
	if instr.Dest != nil {
		dst := cg.getReg(instr.Dest.ID)
		if dst != ARM64RegNone && dst != X0 {
			cg.asm.MovRegReg(dst, X0)
		} else if cg.alloc.IsSpilled(instr.Dest.ID) {
			slot := cg.alloc.GetSpillSlot(instr.Dest.ID)
			offset := cg.getSpillOffset(slot)
			cg.asm.StrRegMem(X0, X29, offset)
		}
	}
}

// emitCallIndirect 生成间接调用指令
func (cg *ARM64CodeGenerator) emitCallIndirect(instr *IRInstr) {
	if len(instr.Args) == 0 {
		return
	}
	
	argRegs := []ARM64Reg{X0, X1, X2, X3, X4, X5, X6, X7}
	argCount := len(instr.Args) - 1 // 第一个是函数指针
	
	cg.saveCallerSavedRegs()
	
	// 加载函数指针到X9（避免被参数覆盖）
	funcPtr := cg.loadValue(instr.Args[0], X9)
	if funcPtr != X9 {
		cg.asm.MovRegReg(X9, funcPtr)
	}
	
	// 加载参数
	for i := 1; i <= argCount && i < 9; i++ {
		src := cg.loadValue(instr.Args[i], X16)
		if src != argRegs[i-1] {
			cg.asm.MovRegReg(argRegs[i-1], src)
		}
	}
	
	// 调用函数指针
	cg.asm.Blr(X9)
	
	cg.restoreCallerSavedRegs()
	
	if instr.Dest != nil {
		dst := cg.getReg(instr.Dest.ID)
		if dst != ARM64RegNone && dst != X0 {
			cg.asm.MovRegReg(dst, X0)
		} else if cg.alloc.IsSpilled(instr.Dest.ID) {
			slot := cg.alloc.GetSpillSlot(instr.Dest.ID)
			offset := cg.getSpillOffset(slot)
			cg.asm.StrRegMem(X0, X29, offset)
		}
	}
}

// emitCallBuiltin 生成内建函数调用
func (cg *ARM64CodeGenerator) emitCallBuiltin(instr *IRInstr) {
	argRegs := []ARM64Reg{X0, X1, X2, X3, X4, X5, X6, X7}
	argCount := len(instr.Args)
	
	cg.saveCallerSavedRegs()
	
	for i := 0; i < argCount && i < 8; i++ {
		src := cg.loadValue(instr.Args[i], X16)
		if src != argRegs[i] {
			cg.asm.MovRegReg(argRegs[i], src)
		}
	}
	
	helperAddr := GetBuiltinCallHelperPtr(instr.CallTarget)
	cg.asm.MovRegImm64(X9, uint64(helperAddr))
	cg.asm.Blr(X9)
	
	cg.restoreCallerSavedRegs()
	
	if instr.Dest != nil {
		dst := cg.getReg(instr.Dest.ID)
		if dst != ARM64RegNone && dst != X0 {
			cg.asm.MovRegReg(dst, X0)
		} else if cg.alloc.IsSpilled(instr.Dest.ID) {
			slot := cg.alloc.GetSpillSlot(instr.Dest.ID)
			offset := cg.getSpillOffset(slot)
			cg.asm.StrRegMem(X0, X29, offset)
		}
	}
}

// emitCallMethod 生成方法调用指令
func (cg *ARM64CodeGenerator) emitCallMethod(instr *IRInstr) {
	if len(instr.Args) == 0 {
		return
	}
	
	argRegs := []ARM64Reg{X0, X1, X2, X3, X4, X5, X6, X7}
	argCount := len(instr.Args)
	
	cg.saveCallerSavedRegs()
	
	// 接收者作为第一个参数
	receiver := cg.loadValue(instr.Args[0], X0)
	if receiver != X0 {
		cg.asm.MovRegReg(X0, receiver)
	}
	
	// 其余参数
	for i := 1; i < argCount && i < 8; i++ {
		src := cg.loadValue(instr.Args[i], X16)
		if src != argRegs[i] {
			cg.asm.MovRegReg(argRegs[i], src)
		}
	}
	
	helperAddr := GetMethodCallHelperPtr(instr.CallTarget)
	cg.asm.MovRegImm64(X9, uint64(helperAddr))
	cg.asm.Blr(X9)
	
	cg.restoreCallerSavedRegs()
	
	if instr.Dest != nil {
		dst := cg.getReg(instr.Dest.ID)
		if dst != ARM64RegNone && dst != X0 {
			cg.asm.MovRegReg(dst, X0)
		} else if cg.alloc.IsSpilled(instr.Dest.ID) {
			slot := cg.alloc.GetSpillSlot(instr.Dest.ID)
			offset := cg.getSpillOffset(slot)
			cg.asm.StrRegMem(X0, X29, offset)
		}
	}
}

// emitTailCall 生成尾调用指令
func (cg *ARM64CodeGenerator) emitTailCall(instr *IRInstr) {
	argRegs := []ARM64Reg{X0, X1, X2, X3, X4, X5, X6, X7}
	argCount := len(instr.Args)
	
	// 加载参数到寄存器
	for i := 0; i < argCount && i < 8; i++ {
		src := cg.loadValue(instr.Args[i], X16)
		if src != argRegs[i] {
			cg.asm.MovRegReg(argRegs[i], src)
		}
	}
	
	// 如果参数超过8个，回退到普通调用
	if argCount > 8 {
		cg.emitCall(instr)
		cg.emitEpilogue()
		return
	}
	
	// 恢复栈帧
	cg.asm.MovRegReg(XSP, X29)
	cg.asm.LdpPost(X29, X30, XSP, 16)
	
	// 跳转到目标函数
	helperAddr := GetTailCallHelperPtr(instr.CallTarget)
	cg.asm.MovRegImm64(X9, uint64(helperAddr))
	cg.asm.Br(X9)
}

// ============================================================================
// 对象操作指令生成
// ============================================================================

// emitNewObject 生成创建对象指令
func (cg *ARM64CodeGenerator) emitNewObject(instr *IRInstr) {
	if instr.Dest == nil {
		return
	}
	
	cg.saveCallerSavedRegs()
	
	helperAddr := GetNewObjectHelperPtr(instr.ClassName)
	cg.asm.MovRegImm64(X9, uint64(helperAddr))
	cg.asm.Blr(X9)
	
	cg.restoreCallerSavedRegs()
	
	dst := cg.getReg(instr.Dest.ID)
	if dst != ARM64RegNone && dst != X0 {
		cg.asm.MovRegReg(dst, X0)
	} else if cg.alloc.IsSpilled(instr.Dest.ID) {
		slot := cg.alloc.GetSpillSlot(instr.Dest.ID)
		offset := cg.getSpillOffset(slot)
		cg.asm.StrRegMem(X0, X29, offset)
	}
}

// emitGetField 生成字段读取指令
func (cg *ARM64CodeGenerator) emitGetField(instr *IRInstr) {
	if instr.Dest == nil || len(instr.Args) == 0 {
		return
	}
	
	// 加载对象指针
	obj := cg.loadValue(instr.Args[0], X0)
	if obj != X0 {
		cg.asm.MovRegReg(X0, obj)
	}
	
	dst := cg.getReg(instr.Dest.ID)
	if dst == ARM64RegNone {
		dst = X0
	}
	
	// 如果有预计算的偏移，直接加载
	if instr.FieldOffset >= 0 {
		// ldr dst, [x0, #offset]
		cg.asm.LdrRegMem(dst, X0, int32(instr.FieldOffset))
	} else {
		// 调用运行时辅助函数
		cg.saveCallerSavedRegs()
		
		helperAddr := GetFieldHelperPtr(instr.FieldName)
		cg.asm.MovRegImm64(X9, uint64(helperAddr))
		cg.asm.Blr(X9)
		
		cg.restoreCallerSavedRegs()
		
		if dst != X0 {
			cg.asm.MovRegReg(dst, X0)
		}
	}
	
	if cg.alloc.IsSpilled(instr.Dest.ID) {
		slot := cg.alloc.GetSpillSlot(instr.Dest.ID)
		offset := cg.getSpillOffset(slot)
		cg.asm.StrRegMem(dst, X29, offset)
	}
}

// emitSetField 生成字段写入指令
func (cg *ARM64CodeGenerator) emitSetField(instr *IRInstr) {
	if len(instr.Args) < 2 {
		return
	}
	
	// 加载对象指针到X0
	obj := cg.loadValue(instr.Args[0], X0)
	if obj != X0 {
		cg.asm.MovRegReg(X0, obj)
	}
	
	// 加载值到X1
	value := cg.loadValue(instr.Args[1], X1)
	if value != X1 {
		cg.asm.MovRegReg(X1, value)
	}
	
	if instr.FieldOffset >= 0 {
		// str x1, [x0, #offset]
		cg.asm.StrRegMem(X1, X0, int32(instr.FieldOffset))
	} else {
		cg.saveCallerSavedRegs()
		
		helperAddr := GetSetFieldHelperPtr(instr.FieldName)
		cg.asm.MovRegImm64(X9, uint64(helperAddr))
		cg.asm.Blr(X9)
		
		cg.restoreCallerSavedRegs()
	}
}

// emitLoadVTable 生成加载虚表指令
func (cg *ARM64CodeGenerator) emitLoadVTable(instr *IRInstr) {
	if instr.Dest == nil || len(instr.Args) == 0 {
		return
	}
	
	obj := cg.loadValue(instr.Args[0], X0)
	
	dst := cg.getReg(instr.Dest.ID)
	if dst == ARM64RegNone {
		dst = X0
	}
	
	// 虚表通常在对象的第一个字段位置
	cg.asm.LdrRegMem(dst, obj, 0)
	
	if cg.alloc.IsSpilled(instr.Dest.ID) {
		slot := cg.alloc.GetSpillSlot(instr.Dest.ID)
		offset := cg.getSpillOffset(slot)
		cg.asm.StrRegMem(dst, X29, offset)
	}
}

// ============================================================================
// 寄存器保存和恢复
// ============================================================================

// saveCallerSavedRegs 保存调用者保存的寄存器
func (cg *ARM64CodeGenerator) saveCallerSavedRegs() {
	// 简化实现：成对保存以保持栈对齐
	for i := 0; i < len(arm64CallerSavedRegs)-1; i += 2 {
		cg.asm.StpPre(arm64CallerSavedRegs[i], arm64CallerSavedRegs[i+1], XSP, -16)
	}
	// 如果是奇数个寄存器，单独保存最后一个
	if len(arm64CallerSavedRegs)%2 == 1 {
		cg.asm.StrPre(arm64CallerSavedRegs[len(arm64CallerSavedRegs)-1], XSP, -16)
	}
}

// restoreCallerSavedRegs 恢复调用者保存的寄存器
func (cg *ARM64CodeGenerator) restoreCallerSavedRegs() {
	// 逆序恢复
	if len(arm64CallerSavedRegs)%2 == 1 {
		cg.asm.LdrPost(arm64CallerSavedRegs[len(arm64CallerSavedRegs)-1], XSP, 16)
	}
	for i := len(arm64CallerSavedRegs) - 2; i >= 0; i -= 2 {
		cg.asm.LdpPost(arm64CallerSavedRegs[i], arm64CallerSavedRegs[i+1], XSP, 16)
	}
}
