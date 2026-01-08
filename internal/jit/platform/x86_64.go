package platform

import (
	"github.com/tangzhangming/nova/internal/bytecode"
	"github.com/tangzhangming/nova/internal/jit/types"
)

// ============================================================================
// x86-64 代码生成器
// ============================================================================

// X64CodeGenerator x86-64 代码生成器
type X64CodeGenerator struct {
	asm *X64Assembler
}

// NewX64CodeGenerator 创建 x86-64 代码生成器
func NewX64CodeGenerator() *X64CodeGenerator {
	return &X64CodeGenerator{
		asm: NewX64Assembler(),
	}
}

// GenerateFunction 生成 x86-64 机器码
func (cg *X64CodeGenerator) GenerateFunction(fn *types.IRFunction, regAlloc *types.RegisterAllocation) ([]byte, error) {
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
func (cg *X64CodeGenerator) emitPrologue(regAlloc *types.RegisterAllocation) {
	// push rbp
	cg.asm.PUSH(RegRBP)
	// mov rbp, rsp
	cg.asm.MOV_REG(RegRBP, RegRSP)
	// sub rsp, <stack_size>
	stackSize := regAlloc.StackSize
	if stackSize > 0 {
		// 对齐到 16 字节
		stackSize = (stackSize + 15) &^ 15
		cg.asm.SUB_IMM(RegRSP, uint32(stackSize))
	}
}

// emitEpilogue 生成函数尾声
func (cg *X64CodeGenerator) emitEpilogue(regAlloc *types.RegisterAllocation) {
	// mov rsp, rbp
	cg.asm.MOV_REG(RegRSP, RegRBP)
	// pop rbp
	cg.asm.POP(RegRBP)
	// ret
	cg.asm.RET()
}

// generateBlock 生成基本块代码
func (cg *X64CodeGenerator) generateBlock(block *types.IRBlock, fn *types.IRFunction, regAlloc *types.RegisterAllocation) {
	// 标记块开始
	cg.asm.Label(block.ID)
	
	// 生成块内指令
	for _, instr := range block.Instrs {
		cg.generateInstruction(instr, fn, regAlloc)
	}
}

// generateInstruction 生成指令代码
func (cg *X64CodeGenerator) generateInstruction(instr *types.IRInstr, fn *types.IRFunction, regAlloc *types.RegisterAllocation) {
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
	case types.IRCall:
		cg.genCall(instr, regAlloc)
	case types.IREq, types.IRNe, types.IRLt, types.IRLe, types.IRGt, types.IRGe:
		cg.genCompare(instr, regAlloc)
	}
}

// genLoadLocal 生成加载局部变量代码
func (cg *X64CodeGenerator) genLoadLocal(instr *types.IRInstr, regAlloc *types.RegisterAllocation) {
	if len(instr.Args) == 0 {
		return
	}
	localIdx := instr.Args[0]
	dest := instr.Dest
	
	preg, spilled := cg.getReg(dest, regAlloc)
	offset := (localIdx + 1) * 8 // 跳过返回地址
	
	if spilled {
		// 先加载到 RAX
		cg.asm.MOV_MEM_TO_REG(RegRAX, RegRBP, -int32(offset))
		// 再存储到栈
		slot := regAlloc.Spilled[dest]
		cg.asm.MOV_REG_TO_MEM(RegRBP, -int32((slot+1)*8+regAlloc.StackSize), RegRAX)
	} else if preg >= 0 {
		cg.asm.MOV_MEM_TO_REG(X64Register(preg), RegRBP, -int32(offset))
	}
}

// genStoreLocal 生成存储局部变量代码
func (cg *X64CodeGenerator) genStoreLocal(instr *types.IRInstr, regAlloc *types.RegisterAllocation) {
	if len(instr.Args) < 2 {
		return
	}
	src := instr.Args[0]
	localIdx := instr.Args[1]
	
	srcReg, srcSpilled := cg.getReg(src, regAlloc)
	offset := (localIdx + 1) * 8
	
	if srcSpilled {
		slot := regAlloc.Spilled[src]
		cg.asm.MOV_MEM_TO_REG(RegRAX, RegRBP, -int32((slot+1)*8+regAlloc.StackSize))
		cg.asm.MOV_REG_TO_MEM(RegRBP, -int32(offset), RegRAX)
	} else if srcReg >= 0 {
		cg.asm.MOV_REG_TO_MEM(RegRBP, -int32(offset), X64Register(srcReg))
	}
}

// genLoadConst 生成加载常量代码
func (cg *X64CodeGenerator) genLoadConst(instr *types.IRInstr, fn *types.IRFunction, regAlloc *types.RegisterAllocation) {
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
			} else {
				imm = 0
			}
		case bytecode.ValFloat:
			// 浮点数需要特殊处理
			imm = 0
		}
	}
	
	if spilled {
		cg.asm.MOV_IMM(RegRAX, imm)
		slot := regAlloc.Spilled[dest]
		cg.asm.MOV_REG_TO_MEM(RegRBP, -int32((slot+1)*8+regAlloc.StackSize), RegRAX)
	} else if preg >= 0 {
		cg.asm.MOV_IMM(X64Register(preg), imm)
	}
}

// genAdd 生成加法代码
func (cg *X64CodeGenerator) genAdd(instr *types.IRInstr, regAlloc *types.RegisterAllocation) {
	if len(instr.Args) < 2 {
		return
	}
	left := instr.Args[0]
	right := instr.Args[1]
	dest := instr.Dest
	
	leftReg, leftSpilled := cg.getReg(left, regAlloc)
	rightReg, rightSpilled := cg.getReg(right, regAlloc)
	destReg, destSpilled := cg.getReg(dest, regAlloc)
	
	// 加载左操作数到 RAX
	if leftSpilled {
		slot := regAlloc.Spilled[left]
		cg.asm.MOV_MEM_TO_REG(RegRAX, RegRBP, -int32((slot+1)*8+regAlloc.StackSize))
	} else if leftReg >= 0 {
		cg.asm.MOV_REG(RegRAX, X64Register(leftReg))
	}
	
	// 加法
	if rightSpilled {
		slot := regAlloc.Spilled[right]
		cg.asm.ADD_MEM(RegRAX, RegRBP, -int32((slot+1)*8+regAlloc.StackSize))
	} else if rightReg >= 0 {
		cg.asm.ADD_REG(RegRAX, X64Register(rightReg))
	}
	
	// 存储结果
	if destSpilled {
		slot := regAlloc.Spilled[dest]
		cg.asm.MOV_REG_TO_MEM(RegRBP, -int32((slot+1)*8+regAlloc.StackSize), RegRAX)
	} else if destReg >= 0 {
		cg.asm.MOV_REG(X64Register(destReg), RegRAX)
	}
}

// genSub 生成减法代码
func (cg *X64CodeGenerator) genSub(instr *types.IRInstr, regAlloc *types.RegisterAllocation) {
	if len(instr.Args) < 2 {
		return
	}
	left := instr.Args[0]
	right := instr.Args[1]
	dest := instr.Dest
	
	leftReg, leftSpilled := cg.getReg(left, regAlloc)
	rightReg, rightSpilled := cg.getReg(right, regAlloc)
	destReg, destSpilled := cg.getReg(dest, regAlloc)
	
	// 加载左操作数到 RAX
	if leftSpilled {
		slot := regAlloc.Spilled[left]
		cg.asm.MOV_MEM_TO_REG(RegRAX, RegRBP, -int32((slot+1)*8+regAlloc.StackSize))
	} else if leftReg >= 0 {
		cg.asm.MOV_REG(RegRAX, X64Register(leftReg))
	}
	
	// 减法
	if rightSpilled {
		slot := regAlloc.Spilled[right]
		cg.asm.SUB_MEM(RegRAX, RegRBP, -int32((slot+1)*8+regAlloc.StackSize))
	} else if rightReg >= 0 {
		cg.asm.SUB_REG(RegRAX, X64Register(rightReg))
	}
	
	// 存储结果
	if destSpilled {
		slot := regAlloc.Spilled[dest]
		cg.asm.MOV_REG_TO_MEM(RegRBP, -int32((slot+1)*8+regAlloc.StackSize), RegRAX)
	} else if destReg >= 0 {
		cg.asm.MOV_REG(X64Register(destReg), RegRAX)
	}
}

// genMul 生成乘法代码
func (cg *X64CodeGenerator) genMul(instr *types.IRInstr, regAlloc *types.RegisterAllocation) {
	if len(instr.Args) < 2 {
		return
	}
	left := instr.Args[0]
	right := instr.Args[1]
	dest := instr.Dest
	
	leftReg, leftSpilled := cg.getReg(left, regAlloc)
	rightReg, rightSpilled := cg.getReg(right, regAlloc)
	destReg, destSpilled := cg.getReg(dest, regAlloc)
	
	// 加载左操作数到 RAX
	if leftSpilled {
		slot := regAlloc.Spilled[left]
		cg.asm.MOV_MEM_TO_REG(RegRAX, RegRBP, -int32((slot+1)*8+regAlloc.StackSize))
	} else if leftReg >= 0 {
		cg.asm.MOV_REG(RegRAX, X64Register(leftReg))
	}
	
	// 乘法
	if rightSpilled {
		slot := regAlloc.Spilled[right]
		cg.asm.IMUL_MEM(RegRAX, RegRBP, -int32((slot+1)*8+regAlloc.StackSize))
	} else if rightReg >= 0 {
		cg.asm.IMUL(RegRAX, X64Register(rightReg))
	}
	
	// 存储结果
	if destSpilled {
		slot := regAlloc.Spilled[dest]
		cg.asm.MOV_REG_TO_MEM(RegRBP, -int32((slot+1)*8+regAlloc.StackSize), RegRAX)
	} else if destReg >= 0 {
		cg.asm.MOV_REG(X64Register(destReg), RegRAX)
	}
}

// genDiv 生成除法代码
func (cg *X64CodeGenerator) genDiv(instr *types.IRInstr, regAlloc *types.RegisterAllocation) {
	if len(instr.Args) < 2 {
		return
	}
	left := instr.Args[0]
	right := instr.Args[1]
	dest := instr.Dest
	
	leftReg, leftSpilled := cg.getReg(left, regAlloc)
	rightReg, rightSpilled := cg.getReg(right, regAlloc)
	destReg, destSpilled := cg.getReg(dest, regAlloc)
	
	// 加载被除数到 RAX
	if leftSpilled {
		slot := regAlloc.Spilled[left]
		cg.asm.MOV_MEM_TO_REG(RegRAX, RegRBP, -int32((slot+1)*8+regAlloc.StackSize))
	} else if leftReg >= 0 {
		cg.asm.MOV_REG(RegRAX, X64Register(leftReg))
	}
	
	// 符号扩展 RAX -> RDX:RAX
	cg.asm.CQO()
	
	// 除法
	if rightSpilled {
		slot := regAlloc.Spilled[right]
		cg.asm.IDIV_MEM(RegRBP, -int32((slot+1)*8+regAlloc.StackSize))
	} else if rightReg >= 0 {
		cg.asm.IDIV(X64Register(rightReg))
	}
	
	// 结果在 RAX
	if destSpilled {
		slot := regAlloc.Spilled[dest]
		cg.asm.MOV_REG_TO_MEM(RegRBP, -int32((slot+1)*8+regAlloc.StackSize), RegRAX)
	} else if destReg >= 0 {
		cg.asm.MOV_REG(X64Register(destReg), RegRAX)
	}
}

// genReturn 生成返回代码
func (cg *X64CodeGenerator) genReturn(instr *types.IRInstr, regAlloc *types.RegisterAllocation) {
	if len(instr.Args) > 0 {
		ret := instr.Args[0]
		retReg, retSpilled := cg.getReg(ret, regAlloc)
		
		if retSpilled {
			slot := regAlloc.Spilled[ret]
			cg.asm.MOV_MEM_TO_REG(RegRAX, RegRBP, -int32((slot+1)*8+regAlloc.StackSize))
		} else if retReg >= 0 && retReg != int(RegRAX) {
			cg.asm.MOV_REG(RegRAX, X64Register(retReg))
		}
	}
	// epilogue 会处理实际返回
}

// genBranch 生成无条件跳转
func (cg *X64CodeGenerator) genBranch(instr *types.IRInstr) {
	if block, ok := instr.Immediate.(*types.IRBlock); ok {
		cg.asm.JMP(block.ID)
	}
}

// genBranchIf 生成条件跳转
func (cg *X64CodeGenerator) genBranchIf(instr *types.IRInstr, regAlloc *types.RegisterAllocation) {
	if len(instr.Args) == 0 {
		return
	}
	
	cond := instr.Args[0]
	condReg, condSpilled := cg.getReg(cond, regAlloc)
	
	// 测试条件
	if condSpilled {
		slot := regAlloc.Spilled[cond]
		cg.asm.MOV_MEM_TO_REG(RegRAX, RegRBP, -int32((slot+1)*8+regAlloc.StackSize))
		cg.asm.TEST(RegRAX, RegRAX)
	} else if condReg >= 0 {
		cg.asm.TEST(X64Register(condReg), X64Register(condReg))
	}
	
	// 跳转
	if targets, ok := instr.Immediate.([]*types.IRBlock); ok && len(targets) >= 2 {
		cg.asm.JZ(targets[0].ID)  // false 分支
		cg.asm.JMP(targets[1].ID) // true 分支
	}
}

// genCall 生成函数调用
func (cg *X64CodeGenerator) genCall(instr *types.IRInstr, regAlloc *types.RegisterAllocation) {
	// 简化：调用约定处理
	cg.asm.CALL(0) // 占位符
}

// genCompare 生成比较代码
func (cg *X64CodeGenerator) genCompare(instr *types.IRInstr, regAlloc *types.RegisterAllocation) {
	if len(instr.Args) < 2 {
		return
	}
	left := instr.Args[0]
	right := instr.Args[1]
	dest := instr.Dest
	
	leftReg, leftSpilled := cg.getReg(left, regAlloc)
	rightReg, rightSpilled := cg.getReg(right, regAlloc)
	destReg, destSpilled := cg.getReg(dest, regAlloc)
	
	// 加载左操作数
	if leftSpilled {
		slot := regAlloc.Spilled[left]
		cg.asm.MOV_MEM_TO_REG(RegRAX, RegRBP, -int32((slot+1)*8+regAlloc.StackSize))
	} else if leftReg >= 0 {
		cg.asm.MOV_REG(RegRAX, X64Register(leftReg))
	}
	
	// 比较
	if rightSpilled {
		slot := regAlloc.Spilled[right]
		cg.asm.CMP_MEM(RegRAX, RegRBP, -int32((slot+1)*8+regAlloc.StackSize))
	} else if rightReg >= 0 {
		cg.asm.CMP_REG(RegRAX, X64Register(rightReg))
	}
	
	// 设置结果
	switch instr.Op {
	case types.IREq:
		cg.asm.SETE(RegAL)
	case types.IRNe:
		cg.asm.SETNE(RegAL)
	case types.IRLt:
		cg.asm.SETL(RegAL)
	case types.IRLe:
		cg.asm.SETLE(RegAL)
	case types.IRGt:
		cg.asm.SETG(RegAL)
	case types.IRGe:
		cg.asm.SETGE(RegAL)
	}
	
	// 零扩展
	cg.asm.MOVZX(RegRAX, RegAL)
	
	// 存储结果
	if destSpilled {
		slot := regAlloc.Spilled[dest]
		cg.asm.MOV_REG_TO_MEM(RegRBP, -int32((slot+1)*8+regAlloc.StackSize), RegRAX)
	} else if destReg >= 0 {
		cg.asm.MOV_REG(X64Register(destReg), RegRAX)
	}
}

// getReg 获取虚拟寄存器对应的物理寄存器
func (cg *X64CodeGenerator) getReg(vreg int, regAlloc *types.RegisterAllocation) (int, bool) {
	if preg, ok := regAlloc.Allocated[vreg]; ok {
		return preg, false
	}
	if _, ok := regAlloc.Spilled[vreg]; ok {
		return -1, true
	}
	return -1, false
}
