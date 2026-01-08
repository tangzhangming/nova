package platform

import (
	"encoding/binary"
)

// ============================================================================
// x86-64 汇编器
// ============================================================================

// X64Register x86-64 寄存器
type X64Register int

const (
	RegRAX X64Register = iota
	RegRCX
	RegRDX
	RegRBX
	RegRSP
	RegRBP
	RegRSI
	RegRDI
	RegR8
	RegR9
	RegR10
	RegR11
	RegR12
	RegR13
	RegR14
	RegR15
	RegAL  // 8位寄存器
)

// X64Assembler x86-64 汇编器
type X64Assembler struct {
	code   []byte
	labels map[int]int
	relocs []relocation
}

type relocation struct {
	offset int
	target int
	isRel  bool
}

// NewX64Assembler 创建汇编器
func NewX64Assembler() *X64Assembler {
	return &X64Assembler{
		code:   make([]byte, 0, 1024),
		labels: make(map[int]int),
		relocs: make([]relocation, 0),
	}
}

// Reset 重置汇编器
func (a *X64Assembler) Reset() {
	a.code = a.code[:0]
	a.labels = make(map[int]int)
	a.relocs = a.relocs[:0]
}

// Code 获取生成的机器码
func (a *X64Assembler) Code() []byte {
	a.resolveRelocations()
	return a.code
}

func (a *X64Assembler) emit(bytes ...byte) {
	a.code = append(a.code, bytes...)
}

func (a *X64Assembler) emitU32(v uint32) {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, v)
	a.code = append(a.code, b...)
}

func (a *X64Assembler) emitU64(v uint64) {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, v)
	a.code = append(a.code, b...)
}

// Label 标记标签
func (a *X64Assembler) Label(id int) {
	a.labels[id] = len(a.code)
}

// rex REX 前缀
func rex(w, r, x, b bool) byte {
	var v byte = 0x40
	if w { v |= 0x08 }
	if r { v |= 0x04 }
	if x { v |= 0x02 }
	if b { v |= 0x01 }
	return v
}

// modrm ModR/M 字节
func modrm(mod, reg, rm byte) byte {
	return (mod << 6) | (reg << 3) | rm
}

// needsREX 检查是否需要 REX 前缀
func needsREX(reg X64Register) bool {
	return reg >= RegR8 && reg <= RegR15
}

// regBits 获取寄存器编码（低3位）
func regBits(reg X64Register) byte {
	if reg >= RegR8 && reg <= RegR15 {
		return byte(reg - RegR8)
	}
	return byte(reg)
}

// PUSH 压栈
func (a *X64Assembler) PUSH(reg X64Register) {
	if needsREX(reg) {
		a.emit(rex(false, false, false, true))
	}
	a.emit(0x50 + regBits(reg))
}

// POP 出栈
func (a *X64Assembler) POP(reg X64Register) {
	if needsREX(reg) {
		a.emit(rex(false, false, false, true))
	}
	a.emit(0x58 + regBits(reg))
}

// MOV_REG 寄存器到寄存器
func (a *X64Assembler) MOV_REG(dst, src X64Register) {
	a.emit(rex(true, needsREX(src), false, needsREX(dst)))
	a.emit(0x89)
	a.emit(modrm(0xC0, regBits(src), regBits(dst)))
}

// MOV_IMM 立即数到寄存器
func (a *X64Assembler) MOV_IMM(reg X64Register, imm uint64) {
	a.emit(rex(true, false, false, needsREX(reg)))
	a.emit(0xB8 + regBits(reg))
	a.emitU64(imm)
}

// MOV_MEM_TO_REG 内存到寄存器 [base+offset]
func (a *X64Assembler) MOV_MEM_TO_REG(dst X64Register, base X64Register, offset int32) {
	a.emit(rex(true, needsREX(dst), false, needsREX(base)))
	a.emit(0x8B)
	
	if offset == 0 && base != RegRBP {
		a.emit(modrm(0x00, regBits(dst), regBits(base)))
	} else if offset >= -128 && offset <= 127 {
		a.emit(modrm(0x40, regBits(dst), regBits(base)))
		a.emit(byte(offset))
	} else {
		a.emit(modrm(0x80, regBits(dst), regBits(base)))
		a.emitU32(uint32(offset))
	}
}

// MOV_REG_TO_MEM 寄存器到内存 [base+offset]
func (a *X64Assembler) MOV_REG_TO_MEM(base X64Register, offset int32, src X64Register) {
	a.emit(rex(true, needsREX(src), false, needsREX(base)))
	a.emit(0x89)
	
	if offset == 0 && base != RegRBP {
		a.emit(modrm(0x00, regBits(src), regBits(base)))
	} else if offset >= -128 && offset <= 127 {
		a.emit(modrm(0x40, regBits(src), regBits(base)))
		a.emit(byte(offset))
	} else {
		a.emit(modrm(0x80, regBits(src), regBits(base)))
		a.emitU32(uint32(offset))
	}
}

// ADD_REG 寄存器加法
func (a *X64Assembler) ADD_REG(dst, src X64Register) {
	a.emit(rex(true, needsREX(src), false, needsREX(dst)))
	a.emit(0x01)
	a.emit(modrm(0xC0, regBits(src), regBits(dst)))
}

// ADD_MEM 内存加法
func (a *X64Assembler) ADD_MEM(dst X64Register, base X64Register, offset int32) {
	a.emit(rex(true, needsREX(dst), false, needsREX(base)))
	a.emit(0x03)
	
	if offset >= -128 && offset <= 127 {
		a.emit(modrm(0x40, regBits(dst), regBits(base)))
		a.emit(byte(offset))
	} else {
		a.emit(modrm(0x80, regBits(dst), regBits(base)))
		a.emitU32(uint32(offset))
	}
}

// SUB_IMM 立即数减法
func (a *X64Assembler) SUB_IMM(dst X64Register, imm uint32) {
	a.emit(rex(true, false, false, needsREX(dst)))
	a.emit(0x81)
	a.emit(modrm(0xC0, 5, regBits(dst))) // /5 = SUB
	a.emitU32(imm)
}

// SUB_REG 寄存器减法
func (a *X64Assembler) SUB_REG(dst, src X64Register) {
	a.emit(rex(true, needsREX(src), false, needsREX(dst)))
	a.emit(0x29)
	a.emit(modrm(0xC0, regBits(src), regBits(dst)))
}

// SUB_MEM 内存减法
func (a *X64Assembler) SUB_MEM(dst X64Register, base X64Register, offset int32) {
	a.emit(rex(true, needsREX(dst), false, needsREX(base)))
	a.emit(0x2B)
	
	if offset >= -128 && offset <= 127 {
		a.emit(modrm(0x40, regBits(dst), regBits(base)))
		a.emit(byte(offset))
	} else {
		a.emit(modrm(0x80, regBits(dst), regBits(base)))
		a.emitU32(uint32(offset))
	}
}

// IMUL 有符号乘法
func (a *X64Assembler) IMUL(dst, src X64Register) {
	a.emit(rex(true, needsREX(dst), false, needsREX(src)))
	a.emit(0x0F, 0xAF)
	a.emit(modrm(0xC0, regBits(dst), regBits(src)))
}

// IMUL_MEM 内存乘法
func (a *X64Assembler) IMUL_MEM(dst X64Register, base X64Register, offset int32) {
	a.emit(rex(true, needsREX(dst), false, needsREX(base)))
	a.emit(0x0F, 0xAF)
	
	if offset >= -128 && offset <= 127 {
		a.emit(modrm(0x40, regBits(dst), regBits(base)))
		a.emit(byte(offset))
	} else {
		a.emit(modrm(0x80, regBits(dst), regBits(base)))
		a.emitU32(uint32(offset))
	}
}

// CQO 符号扩展 RAX -> RDX:RAX
func (a *X64Assembler) CQO() {
	a.emit(0x48, 0x99)
}

// IDIV 有符号除法
func (a *X64Assembler) IDIV(src X64Register) {
	a.emit(rex(true, false, false, needsREX(src)))
	a.emit(0xF7)
	a.emit(modrm(0xC0, 7, regBits(src))) // /7 = IDIV
}

// IDIV_MEM 内存除法
func (a *X64Assembler) IDIV_MEM(base X64Register, offset int32) {
	a.emit(rex(true, false, false, needsREX(base)))
	a.emit(0xF7)
	
	if offset >= -128 && offset <= 127 {
		a.emit(modrm(0x40, 7, regBits(base)))
		a.emit(byte(offset))
	} else {
		a.emit(modrm(0x80, 7, regBits(base)))
		a.emitU32(uint32(offset))
	}
}

// CMP_REG 寄存器比较
func (a *X64Assembler) CMP_REG(dst, src X64Register) {
	a.emit(rex(true, needsREX(src), false, needsREX(dst)))
	a.emit(0x39)
	a.emit(modrm(0xC0, regBits(src), regBits(dst)))
}

// CMP_MEM 内存比较
func (a *X64Assembler) CMP_MEM(dst X64Register, base X64Register, offset int32) {
	a.emit(rex(true, needsREX(dst), false, needsREX(base)))
	a.emit(0x3B)
	
	if offset >= -128 && offset <= 127 {
		a.emit(modrm(0x40, regBits(dst), regBits(base)))
		a.emit(byte(offset))
	} else {
		a.emit(modrm(0x80, regBits(dst), regBits(base)))
		a.emitU32(uint32(offset))
	}
}

// TEST 测试
func (a *X64Assembler) TEST(reg1, reg2 X64Register) {
	a.emit(rex(true, needsREX(reg2), false, needsREX(reg1)))
	a.emit(0x85)
	a.emit(modrm(0xC0, regBits(reg2), regBits(reg1)))
}

// SETcc 条件设置
func (a *X64Assembler) SETE(dst X64Register)  { a.emit(0x0F, 0x94, modrm(0xC0, 0, regBits(dst))) }
func (a *X64Assembler) SETNE(dst X64Register) { a.emit(0x0F, 0x95, modrm(0xC0, 0, regBits(dst))) }
func (a *X64Assembler) SETL(dst X64Register)  { a.emit(0x0F, 0x9C, modrm(0xC0, 0, regBits(dst))) }
func (a *X64Assembler) SETLE(dst X64Register) { a.emit(0x0F, 0x9E, modrm(0xC0, 0, regBits(dst))) }
func (a *X64Assembler) SETG(dst X64Register)  { a.emit(0x0F, 0x9F, modrm(0xC0, 0, regBits(dst))) }
func (a *X64Assembler) SETGE(dst X64Register) { a.emit(0x0F, 0x9D, modrm(0xC0, 0, regBits(dst))) }

// MOVZX 零扩展
func (a *X64Assembler) MOVZX(dst, src X64Register) {
	a.emit(rex(true, needsREX(dst), false, needsREX(src)))
	a.emit(0x0F, 0xB6)
	a.emit(modrm(0xC0, regBits(dst), regBits(src)))
}

// JMP 无条件跳转
func (a *X64Assembler) JMP(labelID int) {
	a.emit(0xE9)
	a.relocs = append(a.relocs, relocation{
		offset: len(a.code),
		target: labelID,
		isRel:  true,
	})
	a.emitU32(0) // 占位符
}

// JZ 为零跳转
func (a *X64Assembler) JZ(labelID int) {
	a.emit(0x0F, 0x84)
	a.relocs = append(a.relocs, relocation{
		offset: len(a.code),
		target: labelID,
		isRel:  true,
	})
	a.emitU32(0)
}

// JNZ 非零跳转
func (a *X64Assembler) JNZ(labelID int) {
	a.emit(0x0F, 0x85)
	a.relocs = append(a.relocs, relocation{
		offset: len(a.code),
		target: labelID,
		isRel:  true,
	})
	a.emitU32(0)
}

// CALL 函数调用
func (a *X64Assembler) CALL(target uintptr) {
	a.emit(0xE8)
	a.emitU32(0) // 相对偏移（需要运行时修正）
}

// RET 返回
func (a *X64Assembler) RET() {
	a.emit(0xC3)
}

// resolveRelocations 解析重定位
func (a *X64Assembler) resolveRelocations() {
	for _, reloc := range a.relocs {
		if reloc.isRel {
			if targetPos, ok := a.labels[reloc.target]; ok {
				// 计算相对偏移（从指令结束处计算）
				offset := int32(targetPos - (reloc.offset + 4))
				binary.LittleEndian.PutUint32(a.code[reloc.offset:], uint32(offset))
			}
		}
	}
}
