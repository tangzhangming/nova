package platform

import (
	"github.com/tangzhangming/nova/internal/bytecode"
	"github.com/tangzhangming/nova/internal/jit/types"
)

// ============================================================================
// ARM64 代码生成器
// ============================================================================

// ARM64CodeGenerator ARM64 代码生成器
type ARM64CodeGenerator struct {
	asm *ARM64Assembler
}

// NewARM64CodeGenerator 创建 ARM64 代码生成器
func NewARM64CodeGenerator() *ARM64CodeGenerator {
	return &ARM64CodeGenerator{
		asm: NewARM64Assembler(),
	}
}

// GenerateFunction 生成 ARM64 机器码
func (cg *ARM64CodeGenerator) GenerateFunction(fn *types.IRFunction, regAlloc *types.RegisterAllocation) ([]byte, error) {
	cg.asm.Reset()
	
	// 函数序言
	cg.emitPrologue(regAlloc)
	
	// 生成基本块代码
	for _, block := range fn.Blocks {
		cg.generateBlock(block, fn, regAlloc)
	}
	
	// 函数尾声
	cg.emitEpilogue(regAlloc)
	
	return cg.asm.Code(), nil
}

// emitPrologue 生成函数序言
func (cg *ARM64CodeGenerator) emitPrologue(regAlloc *types.RegisterAllocation) {
	// stp x29, x30, [sp, #-16]!
	cg.asm.STP_PRE(RegX29, RegX30, RegSP, -16)
	// mov x29, sp
	cg.asm.MOV_REG(RegX29, RegSP)
	// sub sp, sp, #stack_size
	stackSize := regAlloc.StackSize
	if stackSize > 0 {
		stackSize = (stackSize + 15) &^ 15
		cg.asm.SUB_IMM(RegSP, RegSP, uint32(stackSize))
	}
}

// emitEpilogue 生成函数尾声
func (cg *ARM64CodeGenerator) emitEpilogue(regAlloc *types.RegisterAllocation) {
	// mov sp, x29
	cg.asm.MOV_REG(RegSP, RegX29)
	// ldp x29, x30, [sp], #16
	cg.asm.LDP_POST(RegX29, RegX30, RegSP, 16)
	// ret
	cg.asm.RET()
}

// generateBlock 生成基本块代码
func (cg *ARM64CodeGenerator) generateBlock(block *types.IRBlock, fn *types.IRFunction, regAlloc *types.RegisterAllocation) {
	cg.asm.Label(block.ID)
	
	for _, instr := range block.Instrs {
		cg.generateInstruction(instr, fn, regAlloc)
	}
}

// generateInstruction 生成指令代码
func (cg *ARM64CodeGenerator) generateInstruction(instr *types.IRInstr, fn *types.IRFunction, regAlloc *types.RegisterAllocation) {
	switch instr.Op {
	case types.IRLoadLocal:
		cg.genLoadLocal(instr, regAlloc)
	case types.IRStoreLocal:
		cg.genStoreLocal(instr, regAlloc)
	case types.IRLoadConst:
		cg.genLoadConst(instr, fn, regAlloc)
	case types.IRAdd:
		cg.genAdd(instr, regAlloc)
	case types.IRSub:
		cg.genSub(instr, regAlloc)
	case types.IRMul:
		cg.genMul(instr, regAlloc)
	case types.IRDiv:
		cg.genDiv(instr, regAlloc)
	case types.IRReturn:
		cg.genReturn(instr, regAlloc)
	case types.IRBranch:
		cg.genBranch(instr)
	case types.IRBranchIf:
		cg.genBranchIf(instr, regAlloc)
	}
}

// genLoadLocal 生成加载局部变量代码
func (cg *ARM64CodeGenerator) genLoadLocal(instr *types.IRInstr, regAlloc *types.RegisterAllocation) {
	if len(instr.Args) == 0 {
		return
	}
	localIdx := instr.Args[0]
	dest := instr.Dest
	
	preg, spilled := cg.getReg(dest, regAlloc)
	offset := (localIdx + 1) * 8
	
	if spilled {
		cg.asm.LDR(RegX0, RegX29, -int32(offset))
		slot := regAlloc.Spilled[dest]
		cg.asm.STR(RegX0, RegSP, int32((slot+1)*8))
	} else if preg >= 0 {
		cg.asm.LDR(ARM64Register(preg), RegX29, -int32(offset))
	}
}

// genStoreLocal 生成存储局部变量代码
func (cg *ARM64CodeGenerator) genStoreLocal(instr *types.IRInstr, regAlloc *types.RegisterAllocation) {
	if len(instr.Args) < 2 {
		return
	}
	src := instr.Args[0]
	localIdx := instr.Args[1]
	
	srcReg, srcSpilled := cg.getReg(src, regAlloc)
	offset := (localIdx + 1) * 8
	
	if srcSpilled {
		slot := regAlloc.Spilled[src]
		cg.asm.LDR(RegX0, RegSP, int32((slot+1)*8))
		cg.asm.STR(RegX0, RegX29, -int32(offset))
	} else if srcReg >= 0 {
		cg.asm.STR(ARM64Register(srcReg), RegX29, -int32(offset))
	}
}

// genLoadConst 生成加载常量代码
func (cg *ARM64CodeGenerator) genLoadConst(instr *types.IRInstr, fn *types.IRFunction, regAlloc *types.RegisterAllocation) {
	if instr.Immediate == nil {
		return
	}
	
	dest := instr.Dest
	preg, spilled := cg.getReg(dest, regAlloc)
	
	var imm uint64
	if val, ok := instr.Immediate.(bytecode.Value); ok {
		switch val.Type {
		case bytecode.ValInt:
			imm = uint64(val.AsInt())
		case bytecode.ValBool:
			if val.AsBool() {
				imm = 1
			}
		}
	}
	
	if spilled {
		cg.asm.MOV_IMM(RegX0, imm)
		slot := regAlloc.Spilled[dest]
		cg.asm.STR(RegX0, RegSP, int32((slot+1)*8))
	} else if preg >= 0 {
		cg.asm.MOV_IMM(ARM64Register(preg), imm)
	}
}

// genAdd 生成加法代码
func (cg *ARM64CodeGenerator) genAdd(instr *types.IRInstr, regAlloc *types.RegisterAllocation) {
	if len(instr.Args) < 2 {
		return
	}
	left := instr.Args[0]
	right := instr.Args[1]
	dest := instr.Dest
	
	leftReg, leftSpilled := cg.getReg(left, regAlloc)
	rightReg, rightSpilled := cg.getReg(right, regAlloc)
	destReg, destSpilled := cg.getReg(dest, regAlloc)
	
	// 加载操作数
	if leftSpilled {
		slot := regAlloc.Spilled[left]
		cg.asm.LDR(RegX0, RegSP, int32((slot+1)*8))
	} else if leftReg >= 0 {
		cg.asm.MOV_REG(RegX0, ARM64Register(leftReg))
	}
	
	if rightSpilled {
		slot := regAlloc.Spilled[right]
		cg.asm.LDR(RegX1, RegSP, int32((slot+1)*8))
	} else if rightReg >= 0 {
		cg.asm.MOV_REG(RegX1, ARM64Register(rightReg))
	}
	
	// 加法
	cg.asm.ADD_REG(RegX0, RegX0, RegX1)
	
	// 存储结果
	if destSpilled {
		slot := regAlloc.Spilled[dest]
		cg.asm.STR(RegX0, RegSP, int32((slot+1)*8))
	} else if destReg >= 0 {
		cg.asm.MOV_REG(ARM64Register(destReg), RegX0)
	}
}

// genSub 生成减法代码
func (cg *ARM64CodeGenerator) genSub(instr *types.IRInstr, regAlloc *types.RegisterAllocation) {
	if len(instr.Args) < 2 {
		return
	}
	left := instr.Args[0]
	right := instr.Args[1]
	dest := instr.Dest
	
	leftReg, leftSpilled := cg.getReg(left, regAlloc)
	rightReg, rightSpilled := cg.getReg(right, regAlloc)
	destReg, destSpilled := cg.getReg(dest, regAlloc)
	
	if leftSpilled {
		slot := regAlloc.Spilled[left]
		cg.asm.LDR(RegX0, RegSP, int32((slot+1)*8))
	} else if leftReg >= 0 {
		cg.asm.MOV_REG(RegX0, ARM64Register(leftReg))
	}
	
	if rightSpilled {
		slot := regAlloc.Spilled[right]
		cg.asm.LDR(RegX1, RegSP, int32((slot+1)*8))
	} else if rightReg >= 0 {
		cg.asm.MOV_REG(RegX1, ARM64Register(rightReg))
	}
	
	cg.asm.SUB_REG(RegX0, RegX0, RegX1)
	
	if destSpilled {
		slot := regAlloc.Spilled[dest]
		cg.asm.STR(RegX0, RegSP, int32((slot+1)*8))
	} else if destReg >= 0 {
		cg.asm.MOV_REG(ARM64Register(destReg), RegX0)
	}
}

// genMul 生成乘法代码
func (cg *ARM64CodeGenerator) genMul(instr *types.IRInstr, regAlloc *types.RegisterAllocation) {
	if len(instr.Args) < 2 {
		return
	}
	left := instr.Args[0]
	right := instr.Args[1]
	dest := instr.Dest
	
	leftReg, leftSpilled := cg.getReg(left, regAlloc)
	rightReg, rightSpilled := cg.getReg(right, regAlloc)
	destReg, destSpilled := cg.getReg(dest, regAlloc)
	
	if leftSpilled {
		slot := regAlloc.Spilled[left]
		cg.asm.LDR(RegX0, RegSP, int32((slot+1)*8))
	} else if leftReg >= 0 {
		cg.asm.MOV_REG(RegX0, ARM64Register(leftReg))
	}
	
	if rightSpilled {
		slot := regAlloc.Spilled[right]
		cg.asm.LDR(RegX1, RegSP, int32((slot+1)*8))
	} else if rightReg >= 0 {
		cg.asm.MOV_REG(RegX1, ARM64Register(rightReg))
	}
	
	cg.asm.MUL(RegX0, RegX0, RegX1)
	
	if destSpilled {
		slot := regAlloc.Spilled[dest]
		cg.asm.STR(RegX0, RegSP, int32((slot+1)*8))
	} else if destReg >= 0 {
		cg.asm.MOV_REG(ARM64Register(destReg), RegX0)
	}
}

// genDiv 生成除法代码
func (cg *ARM64CodeGenerator) genDiv(instr *types.IRInstr, regAlloc *types.RegisterAllocation) {
	if len(instr.Args) < 2 {
		return
	}
	left := instr.Args[0]
	right := instr.Args[1]
	dest := instr.Dest
	
	leftReg, leftSpilled := cg.getReg(left, regAlloc)
	rightReg, rightSpilled := cg.getReg(right, regAlloc)
	destReg, destSpilled := cg.getReg(dest, regAlloc)
	
	if leftSpilled {
		slot := regAlloc.Spilled[left]
		cg.asm.LDR(RegX0, RegSP, int32((slot+1)*8))
	} else if leftReg >= 0 {
		cg.asm.MOV_REG(RegX0, ARM64Register(leftReg))
	}
	
	if rightSpilled {
		slot := regAlloc.Spilled[right]
		cg.asm.LDR(RegX1, RegSP, int32((slot+1)*8))
	} else if rightReg >= 0 {
		cg.asm.MOV_REG(RegX1, ARM64Register(rightReg))
	}
	
	cg.asm.SDIV(RegX0, RegX0, RegX1)
	
	if destSpilled {
		slot := regAlloc.Spilled[dest]
		cg.asm.STR(RegX0, RegSP, int32((slot+1)*8))
	} else if destReg >= 0 {
		cg.asm.MOV_REG(ARM64Register(destReg), RegX0)
	}
}

// genReturn 生成返回代码
func (cg *ARM64CodeGenerator) genReturn(instr *types.IRInstr, regAlloc *types.RegisterAllocation) {
	if len(instr.Args) > 0 {
		ret := instr.Args[0]
		retReg, retSpilled := cg.getReg(ret, regAlloc)
		
		if retSpilled {
			slot := regAlloc.Spilled[ret]
			cg.asm.LDR(RegX0, RegSP, int32((slot+1)*8))
		} else if retReg >= 0 && retReg != int(RegX0) {
			cg.asm.MOV_REG(RegX0, ARM64Register(retReg))
		}
	}
}

// genBranch 生成无条件跳转
func (cg *ARM64CodeGenerator) genBranch(instr *types.IRInstr) {
	if block, ok := instr.Immediate.(*types.IRBlock); ok {
		cg.asm.B(block.ID)
	}
}

// genBranchIf 生成条件跳转
func (cg *ARM64CodeGenerator) genBranchIf(instr *types.IRInstr, regAlloc *types.RegisterAllocation) {
	if len(instr.Args) == 0 {
		return
	}
	
	cond := instr.Args[0]
	condReg, condSpilled := cg.getReg(cond, regAlloc)
	
	if condSpilled {
		slot := regAlloc.Spilled[cond]
		cg.asm.LDR(RegX0, RegSP, int32((slot+1)*8))
	} else if condReg >= 0 {
		cg.asm.MOV_REG(RegX0, ARM64Register(condReg))
	}
	
	if targets, ok := instr.Immediate.([]*types.IRBlock); ok && len(targets) >= 2 {
		cg.asm.CBZ(RegX0, targets[0].ID)
		cg.asm.B(targets[1].ID)
	}
}

// getReg 获取虚拟寄存器对应的物理寄存器
func (cg *ARM64CodeGenerator) getReg(vreg int, regAlloc *types.RegisterAllocation) (int, bool) {
	if preg, ok := regAlloc.Allocated[vreg]; ok {
		return preg, false
	}
	if _, ok := regAlloc.Spilled[vreg]; ok {
		return -1, true
	}
	return -1, false
}
