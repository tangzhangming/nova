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
	// 注意：Sola 字节码中 local[0] 预留给 this，参数从 local[1] 开始
	// 所以 arg0 保存到 [rbp-16] (local[1])，arg1 保存到 [rbp-24] (local[2])，以此类推
	argRegs := []X64Reg{RCX, RDX, R8, R9}
	for i := 0; i < cg.fn.NumArgs && i < 4; i++ {
		offset := (i + 2) * -8  // local[i+1] 的偏移
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
		if cg.isFloatOp(instr) {
			cg.emitFloatBinary(instr, cg.asm.AddsdRegReg)
		} else {
			cg.emitBinary(instr, cg.asm.AddRegReg)
		}
	case OpSub:
		if cg.isFloatOp(instr) {
			cg.emitFloatBinary(instr, cg.asm.SubsdRegReg)
		} else {
			cg.emitBinary(instr, cg.asm.SubRegReg)
		}
	case OpMul:
		if cg.isFloatOp(instr) {
			cg.emitFloatBinary(instr, cg.asm.MulsdRegReg)
		} else {
			cg.emitMul(instr)
		}
	case OpDiv:
		if cg.isFloatOp(instr) {
			cg.emitFloatBinary(instr, cg.asm.DivsdRegReg)
		} else {
			cg.emitDiv(instr)
		}
	case OpMod:
		cg.emitMod(instr)
	case OpNeg:
		cg.emitNeg(instr)
	case OpEq, OpNe, OpLt, OpLe, OpGt, OpGe:
		if cg.isFloatOp(instr) {
			cg.emitFloatCompare(instr)
		} else {
			cg.emitCompare(instr)
		}
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
	case OpIntToFloat:
		cg.emitIntToFloat(instr)
	case OpFloatToInt:
		cg.emitFloatToInt(instr)
	case OpArrayLen:
		cg.emitArrayLen(instr)
	case OpArrayGet:
		cg.emitArrayGet(instr)
	case OpArraySet:
		cg.emitArraySet(instr)
	case OpPhi:
		// Phi 节点在代码生成阶段已经通过寄存器分配处理，不需要额外代码
	case OpNop:
		// 空操作，不生成代码
	default:
		// 不支持的操作
		fmt.Printf("Warning: unsupported opcode: %s\n", instr.Op)
	}
}

// isFloatOp 检查是否是浮点运算
func (cg *X64CodeGenerator) isFloatOp(instr *IRInstr) bool {
	// 检查目标类型
	if instr.Dest != nil && instr.Dest.Type == TypeFloat {
		return true
	}
	// 检查操作数类型
	for _, arg := range instr.Args {
		if arg != nil && arg.Type == TypeFloat {
			return true
		}
	}
	return false
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
	if instr.Dest == nil {
		return
	}
	
	localIdx := instr.LocalIdx
	dst := cg.getReg(instr.Dest.ID)
	
	if dst == RegNone {
		// 如果值被溢出到栈上，需要先加载到临时寄存器再存储
		dst = RAX
		offset := int32((localIdx + 1) * -8)
		cg.asm.MovRegMem(dst, RBP, offset)
		
		if cg.alloc.IsSpilled(instr.Dest.ID) {
			slot := cg.alloc.GetSpillSlot(instr.Dest.ID)
			spillOffset := cg.getSpillOffset(slot)
			cg.asm.MovMemReg(RBP, spillOffset, dst)
		}
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
			// 使用 IEEE 754 位表示保持精度
			imm = FloatBitsToInt64(v.ConstVal.AsFloat())
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

// ============================================================================
// 浮点运算指令生成
// ============================================================================

// emitFloatBinary 生成浮点二元运算
func (cg *X64CodeGenerator) emitFloatBinary(instr *IRInstr, op func(dst, src XMMReg)) {
	if instr.Dest == nil || len(instr.Args) < 2 {
		return
	}
	
	// 加载左操作数到 XMM0
	left := cg.loadValue(instr.Args[0], RAX)
	cg.asm.MovqXmmReg(XMM0, left)
	
	// 加载右操作数到 XMM1
	right := cg.loadValue(instr.Args[1], R11)
	cg.asm.MovqXmmReg(XMM1, right)
	
	// 执行运算 XMM0 = XMM0 op XMM1
	op(XMM0, XMM1)
	
	// 将结果移回通用寄存器
	dst := cg.getReg(instr.Dest.ID)
	if dst == RegNone {
		dst = RAX
	}
	cg.asm.MovqRegXmm(dst, XMM0)
	
	// 如果目标被溢出，存回栈
	if cg.alloc.IsSpilled(instr.Dest.ID) {
		slot := cg.alloc.GetSpillSlot(instr.Dest.ID)
		offset := cg.getSpillOffset(slot)
		cg.asm.MovMemReg(RBP, offset, dst)
	}
}

// emitFloatCompare 生成浮点比较
func (cg *X64CodeGenerator) emitFloatCompare(instr *IRInstr) {
	if instr.Dest == nil || len(instr.Args) < 2 {
		return
	}
	
	// 加载左操作数到 XMM0
	left := cg.loadValue(instr.Args[0], RAX)
	cg.asm.MovqXmmReg(XMM0, left)
	
	// 加载右操作数到 XMM1
	right := cg.loadValue(instr.Args[1], R11)
	cg.asm.MovqXmmReg(XMM1, right)
	
	// 比较 (设置标志位)
	cg.asm.UcomisdRegReg(XMM0, XMM1)
	
	// 根据比较类型设置结果
	dst := cg.getReg(instr.Dest.ID)
	if dst == RegNone {
		dst = RAX
	}
	
	switch instr.Op {
	case OpEq:
		// 浮点相等：需要检查 ZF=1 且 PF=0
		cg.asm.SetE(dst)
	case OpNe:
		cg.asm.SetNE(dst)
	case OpLt:
		// 浮点小于：CF=1 (ucomisd 后)
		cg.asm.SetB(dst)
	case OpLe:
		// 浮点小于等于：CF=1 或 ZF=1
		cg.asm.SetBE(dst)
	case OpGt:
		// 浮点大于：CF=0 且 ZF=0
		cg.asm.SetA(dst)
	case OpGe:
		// 浮点大于等于：CF=0
		cg.asm.SetAE(dst)
	}
	
	// 零扩展到 64 位
	cg.asm.MovzxReg8(dst, dst)
	
	if cg.alloc.IsSpilled(instr.Dest.ID) {
		slot := cg.alloc.GetSpillSlot(instr.Dest.ID)
		offset := cg.getSpillOffset(slot)
		cg.asm.MovMemReg(RBP, offset, dst)
	}
}

// emitIntToFloat 生成整数到浮点的转换
func (cg *X64CodeGenerator) emitIntToFloat(instr *IRInstr) {
	if instr.Dest == nil || len(instr.Args) == 0 {
		return
	}
	
	// 加载整数到通用寄存器
	src := cg.loadValue(instr.Args[0], RAX)
	
	// 转换为浮点 (XMM0 = cvtsi2sd(src))
	cg.asm.Cvtsi2sdRegReg(XMM0, src)
	
	// 将结果移回通用寄存器（作为 IEEE 754 位表示）
	dst := cg.getReg(instr.Dest.ID)
	if dst == RegNone {
		dst = RAX
	}
	cg.asm.MovqRegXmm(dst, XMM0)
	
	if cg.alloc.IsSpilled(instr.Dest.ID) {
		slot := cg.alloc.GetSpillSlot(instr.Dest.ID)
		offset := cg.getSpillOffset(slot)
		cg.asm.MovMemReg(RBP, offset, dst)
	}
}

// emitFloatToInt 生成浮点到整数的转换
func (cg *X64CodeGenerator) emitFloatToInt(instr *IRInstr) {
	if instr.Dest == nil || len(instr.Args) == 0 {
		return
	}
	
	// 加载浮点（IEEE 754 位表示）到通用寄存器
	src := cg.loadValue(instr.Args[0], RAX)
	
	// 先移到 XMM 寄存器
	cg.asm.MovqXmmReg(XMM0, src)
	
	// 转换为整数（截断）
	dst := cg.getReg(instr.Dest.ID)
	if dst == RegNone {
		dst = RAX
	}
	cg.asm.Cvttsd2siRegReg(dst, XMM0)
	
	if cg.alloc.IsSpilled(instr.Dest.ID) {
		slot := cg.alloc.GetSpillSlot(instr.Dest.ID)
		offset := cg.getSpillOffset(slot)
		cg.asm.MovMemReg(RBP, offset, dst)
	}
}

// ============================================================================
// 数组操作指令生成
// ============================================================================

// emitArrayLen 生成数组长度操作
// 调用 ArrayLenHelper(arrPtr) -> int64
func (cg *X64CodeGenerator) emitArrayLen(instr *IRInstr) {
	if instr.Dest == nil || len(instr.Args) == 0 {
		return
	}
	
	// 加载数组指针到 RCX（Windows x64 第一个参数）
	arr := cg.loadValue(instr.Args[0], RCX)
	if arr != RCX {
		cg.asm.MovRegReg(RCX, arr)
	}
	
	// 调用 ArrayLenHelper
	// 获取辅助函数地址
	helperAddr := GetArrayLenHelperPtr()
	cg.emitCallHelper(helperAddr)
	
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

// emitArrayGet 生成数组取元素操作
// 调用 ArrayGetHelper(arrPtr, index) -> (value, ok)
func (cg *X64CodeGenerator) emitArrayGet(instr *IRInstr) {
	if instr.Dest == nil || len(instr.Args) < 2 {
		return
	}
	
	// 加载数组指针到 RCX
	arr := cg.loadValue(instr.Args[0], RCX)
	if arr != RCX {
		cg.asm.MovRegReg(RCX, arr)
	}
	
	// 加载索引到 RDX
	index := cg.loadValue(instr.Args[1], RDX)
	if index != RDX {
		cg.asm.MovRegReg(RDX, index)
	}
	
	// 调用 ArrayGetHelper
	helperAddr := GetArrayGetHelperPtr()
	cg.emitCallHelper(helperAddr)
	
	// 结果在 RAX（值），RDX（成功标志）
	// 简化处理：只使用值，忽略成功标志
	dst := cg.getReg(instr.Dest.ID)
	if dst != RegNone && dst != RAX {
		cg.asm.MovRegReg(dst, RAX)
	} else if cg.alloc.IsSpilled(instr.Dest.ID) {
		slot := cg.alloc.GetSpillSlot(instr.Dest.ID)
		offset := cg.getSpillOffset(slot)
		cg.asm.MovMemReg(RBP, offset, RAX)
	}
}

// emitArraySet 生成数组设元素操作
// 调用 ArraySetHelper(arrPtr, index, value) -> ok
func (cg *X64CodeGenerator) emitArraySet(instr *IRInstr) {
	if len(instr.Args) < 3 {
		return
	}
	
	// 加载数组指针到 RCX
	arr := cg.loadValue(instr.Args[0], RCX)
	if arr != RCX {
		cg.asm.MovRegReg(RCX, arr)
	}
	
	// 加载索引到 RDX
	index := cg.loadValue(instr.Args[1], RDX)
	if index != RDX {
		cg.asm.MovRegReg(RDX, index)
	}
	
	// 加载值到 R8
	value := cg.loadValue(instr.Args[2], R8)
	if value != R8 {
		cg.asm.MovRegReg(R8, value)
	}
	
	// 调用 ArraySetHelper
	helperAddr := GetArraySetHelperPtr()
	cg.emitCallHelper(helperAddr)
	
	// 结果在 RAX（成功标志），可以忽略
}

// emitCallHelper 生成调用运行时辅助函数的代码
// Windows x64 调用约定：
// - 参数: RCX, RDX, R8, R9
// - 返回值: RAX
// - 需要 32 字节 shadow space
func (cg *X64CodeGenerator) emitCallHelper(addr uintptr) {
	// 分配 shadow space (32 字节) + 对齐
	// 确保栈 16 字节对齐
	cg.asm.SubRegImm32(RSP, 40) // 32 shadow + 8 对齐
	
	// 将地址加载到 RAX 并调用
	cg.asm.MovRegImm64(RAX, uint64(addr))
	cg.asm.Call(RAX)
	
	// 恢复栈
	cg.asm.AddRegImm32(RSP, 40)
}
