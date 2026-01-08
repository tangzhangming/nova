// x64_asm.go - x86-64 汇编器
//
// 本文件实现了 x86-64 机器码生成的底层汇编器。
// 提供了常用指令的编码方法，支持寄存器操作和内存操作。
//
// x86-64 指令编码格式：
// [前缀] [REX] [操作码] [ModR/M] [SIB] [位移] [立即数]
//
// REX 前缀：用于扩展寄存器和操作数大小
// - REX.W: 64 位操作数
// - REX.R: 扩展 ModR/M.reg 字段
// - REX.X: 扩展 SIB.index 字段
// - REX.B: 扩展 ModR/M.r/m 或 SIB.base 字段

package jit

import (
	"encoding/binary"
)

// ============================================================================
// x86-64 寄存器定义
// ============================================================================

// X64Reg x86-64 寄存器
type X64Reg int

const (
	// 通用寄存器（64 位）
	RAX X64Reg = iota
	RCX
	RDX
	RBX
	RSP
	RBP
	RSI
	RDI
	R8
	R9
	R10
	R11
	R12
	R13
	R14
	R15
	
	// 特殊标记
	RegNone X64Reg = -1 // 无寄存器
)

// String 返回寄存器名称
func (r X64Reg) String() string {
	names := []string{
		"rax", "rcx", "rdx", "rbx", "rsp", "rbp", "rsi", "rdi",
		"r8", "r9", "r10", "r11", "r12", "r13", "r14", "r15",
	}
	if r >= 0 && int(r) < len(names) {
		return names[r]
	}
	return "???"
}

// IsExtended 检查是否是扩展寄存器（需要 REX 前缀）
func (r X64Reg) IsExtended() bool {
	return r >= R8 && r <= R15
}

// LowBits 获取寄存器编码的低 3 位
func (r X64Reg) LowBits() byte {
	return byte(r) & 0x7
}

// ============================================================================
// x86-64 汇编器
// ============================================================================

// X64Assembler x86-64 汇编器
type X64Assembler struct {
	code   []byte            // 生成的机器码
	labels map[int]int       // 标签位置（块 ID -> 代码偏移）
	relocs []x64Reloc        // 重定位表
}

// x64Reloc 重定位条目
type x64Reloc struct {
	offset int  // 在代码中的偏移
	target int  // 目标块 ID
	size   int  // 偏移字段大小（1, 2, 4 字节）
}

// NewX64Assembler 创建 x86-64 汇编器
func NewX64Assembler() *X64Assembler {
	return &X64Assembler{
		code:   make([]byte, 0, 1024),
		labels: make(map[int]int),
	}
}

// Reset 重置汇编器状态
func (a *X64Assembler) Reset() {
	a.code = a.code[:0]
	a.labels = make(map[int]int)
	a.relocs = nil
}

// Code 获取生成的机器码
func (a *X64Assembler) Code() []byte {
	a.resolveRelocations()
	return a.code
}

// Len 返回当前代码长度
func (a *X64Assembler) Len() int {
	return len(a.code)
}

// ============================================================================
// 底层编码方法
// ============================================================================

// emit 写入字节
func (a *X64Assembler) emit(bytes ...byte) {
	a.code = append(a.code, bytes...)
}

// emitU32 写入 32 位值（小端序）
func (a *X64Assembler) emitU32(v uint32) {
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, v)
	a.code = append(a.code, buf...)
}

// emitU64 写入 64 位值（小端序）
func (a *X64Assembler) emitU64(v uint64) {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, v)
	a.code = append(a.code, buf...)
}

// rex 构造 REX 前缀
// w: 64 位操作数
// r: 扩展 ModR/M.reg
// x: 扩展 SIB.index
// b: 扩展 ModR/M.r/m 或 SIB.base
func rex(w, r, x, b bool) byte {
	var v byte = 0x40
	if w {
		v |= 0x08
	}
	if r {
		v |= 0x04
	}
	if x {
		v |= 0x02
	}
	if b {
		v |= 0x01
	}
	return v
}

// modrm 构造 ModR/M 字节
// mod: 寻址模式 (0-3)
// reg: 寄存器操作数或操作码扩展
// rm: 寄存器/内存操作数
func modrm(mod, reg, rm byte) byte {
	return (mod << 6) | ((reg & 0x7) << 3) | (rm & 0x7)
}

// Label 定义标签
func (a *X64Assembler) Label(id int) {
	a.labels[id] = len(a.code)
}

// ============================================================================
// 数据移动指令
// ============================================================================

// MovRegReg 寄存器到寄存器: mov dst, src
func (a *X64Assembler) MovRegReg(dst, src X64Reg) {
	a.emit(rex(true, src.IsExtended(), false, dst.IsExtended()))
	a.emit(0x89)
	a.emit(modrm(3, src.LowBits(), dst.LowBits()))
}

// MovRegImm64 加载 64 位立即数: mov reg, imm64
func (a *X64Assembler) MovRegImm64(reg X64Reg, imm uint64) {
	a.emit(rex(true, false, false, reg.IsExtended()))
	a.emit(0xB8 + reg.LowBits())
	a.emitU64(imm)
}

// MovRegImm32 加载 32 位立即数（符号扩展）: mov reg, imm32
func (a *X64Assembler) MovRegImm32(reg X64Reg, imm int32) {
	a.emit(rex(true, false, false, reg.IsExtended()))
	a.emit(0xC7)
	a.emit(modrm(3, 0, reg.LowBits()))
	a.emitU32(uint32(imm))
}

// MovRegMem 从内存加载: mov reg, [base+offset]
func (a *X64Assembler) MovRegMem(dst X64Reg, base X64Reg, offset int32) {
	a.emit(rex(true, dst.IsExtended(), false, base.IsExtended()))
	a.emit(0x8B)
	a.emitMemOperand(dst.LowBits(), base, offset)
}

// MovMemReg 存储到内存: mov [base+offset], reg
func (a *X64Assembler) MovMemReg(base X64Reg, offset int32, src X64Reg) {
	a.emit(rex(true, src.IsExtended(), false, base.IsExtended()))
	a.emit(0x89)
	a.emitMemOperand(src.LowBits(), base, offset)
}

// emitMemOperand 生成内存操作数编码
func (a *X64Assembler) emitMemOperand(reg byte, base X64Reg, offset int32) {
	baseCode := base.LowBits()
	
	// RSP 需要 SIB 字节
	needSIB := base == RSP || base == R12
	
	if offset == 0 && base != RBP && base != R13 {
		// [base]
		if needSIB {
			a.emit(modrm(0, reg, 4)) // SIB 标记
			a.emit(0x24)              // SIB: scale=0, index=RSP, base=RSP
		} else {
			a.emit(modrm(0, reg, baseCode))
		}
	} else if offset >= -128 && offset <= 127 {
		// [base+disp8]
		if needSIB {
			a.emit(modrm(1, reg, 4))
			a.emit(0x24)
		} else {
			a.emit(modrm(1, reg, baseCode))
		}
		a.emit(byte(offset))
	} else {
		// [base+disp32]
		if needSIB {
			a.emit(modrm(2, reg, 4))
			a.emit(0x24)
		} else {
			a.emit(modrm(2, reg, baseCode))
		}
		a.emitU32(uint32(offset))
	}
}

// ============================================================================
// 算术指令
// ============================================================================

// AddRegReg 寄存器加法: add dst, src
func (a *X64Assembler) AddRegReg(dst, src X64Reg) {
	a.emit(rex(true, src.IsExtended(), false, dst.IsExtended()))
	a.emit(0x01)
	a.emit(modrm(3, src.LowBits(), dst.LowBits()))
}

// AddRegImm32 立即数加法: add reg, imm32
func (a *X64Assembler) AddRegImm32(reg X64Reg, imm int32) {
	a.emit(rex(true, false, false, reg.IsExtended()))
	if imm >= -128 && imm <= 127 {
		a.emit(0x83)
		a.emit(modrm(3, 0, reg.LowBits()))
		a.emit(byte(imm))
	} else {
		a.emit(0x81)
		a.emit(modrm(3, 0, reg.LowBits()))
		a.emitU32(uint32(imm))
	}
}

// SubRegReg 寄存器减法: sub dst, src
func (a *X64Assembler) SubRegReg(dst, src X64Reg) {
	a.emit(rex(true, src.IsExtended(), false, dst.IsExtended()))
	a.emit(0x29)
	a.emit(modrm(3, src.LowBits(), dst.LowBits()))
}

// SubRegImm32 立即数减法: sub reg, imm32
func (a *X64Assembler) SubRegImm32(reg X64Reg, imm int32) {
	a.emit(rex(true, false, false, reg.IsExtended()))
	if imm >= -128 && imm <= 127 {
		a.emit(0x83)
		a.emit(modrm(3, 5, reg.LowBits()))
		a.emit(byte(imm))
	} else {
		a.emit(0x81)
		a.emit(modrm(3, 5, reg.LowBits()))
		a.emitU32(uint32(imm))
	}
}

// IMulRegReg 有符号乘法: imul dst, src
func (a *X64Assembler) IMulRegReg(dst, src X64Reg) {
	a.emit(rex(true, dst.IsExtended(), false, src.IsExtended()))
	a.emit(0x0F, 0xAF)
	a.emit(modrm(3, dst.LowBits(), src.LowBits()))
}

// IMulRegImm32 立即数乘法: imul dst, src, imm32
func (a *X64Assembler) IMulRegImm32(dst, src X64Reg, imm int32) {
	a.emit(rex(true, dst.IsExtended(), false, src.IsExtended()))
	if imm >= -128 && imm <= 127 {
		a.emit(0x6B)
		a.emit(modrm(3, dst.LowBits(), src.LowBits()))
		a.emit(byte(imm))
	} else {
		a.emit(0x69)
		a.emit(modrm(3, dst.LowBits(), src.LowBits()))
		a.emitU32(uint32(imm))
	}
}

// Neg 取负: neg reg
func (a *X64Assembler) Neg(reg X64Reg) {
	a.emit(rex(true, false, false, reg.IsExtended()))
	a.emit(0xF7)
	a.emit(modrm(3, 3, reg.LowBits()))
}

// CQO 符号扩展 RAX -> RDX:RAX
func (a *X64Assembler) CQO() {
	a.emit(0x48, 0x99)
}

// IDivReg 有符号除法: idiv reg (RDX:RAX / reg -> RAX, 余数 -> RDX)
func (a *X64Assembler) IDivReg(reg X64Reg) {
	a.emit(rex(true, false, false, reg.IsExtended()))
	a.emit(0xF7)
	a.emit(modrm(3, 7, reg.LowBits()))
}

// ============================================================================
// 位运算指令
// ============================================================================

// AndRegReg 位与: and dst, src
func (a *X64Assembler) AndRegReg(dst, src X64Reg) {
	a.emit(rex(true, src.IsExtended(), false, dst.IsExtended()))
	a.emit(0x21)
	a.emit(modrm(3, src.LowBits(), dst.LowBits()))
}

// OrRegReg 位或: or dst, src
func (a *X64Assembler) OrRegReg(dst, src X64Reg) {
	a.emit(rex(true, src.IsExtended(), false, dst.IsExtended()))
	a.emit(0x09)
	a.emit(modrm(3, src.LowBits(), dst.LowBits()))
}

// XorRegReg 位异或: xor dst, src
func (a *X64Assembler) XorRegReg(dst, src X64Reg) {
	a.emit(rex(true, src.IsExtended(), false, dst.IsExtended()))
	a.emit(0x31)
	a.emit(modrm(3, src.LowBits(), dst.LowBits()))
}

// NotReg 位非: not reg
func (a *X64Assembler) NotReg(reg X64Reg) {
	a.emit(rex(true, false, false, reg.IsExtended()))
	a.emit(0xF7)
	a.emit(modrm(3, 2, reg.LowBits()))
}

// ShlRegCL 左移: shl reg, cl
func (a *X64Assembler) ShlRegCL(reg X64Reg) {
	a.emit(rex(true, false, false, reg.IsExtended()))
	a.emit(0xD3)
	a.emit(modrm(3, 4, reg.LowBits()))
}

// ShlRegImm 左移立即数: shl reg, imm
func (a *X64Assembler) ShlRegImm(reg X64Reg, imm byte) {
	a.emit(rex(true, false, false, reg.IsExtended()))
	if imm == 1 {
		a.emit(0xD1)
		a.emit(modrm(3, 4, reg.LowBits()))
	} else {
		a.emit(0xC1)
		a.emit(modrm(3, 4, reg.LowBits()))
		a.emit(imm)
	}
}

// SarRegCL 算术右移: sar reg, cl
func (a *X64Assembler) SarRegCL(reg X64Reg) {
	a.emit(rex(true, false, false, reg.IsExtended()))
	a.emit(0xD3)
	a.emit(modrm(3, 7, reg.LowBits()))
}

// SarRegImm 算术右移立即数: sar reg, imm
func (a *X64Assembler) SarRegImm(reg X64Reg, imm byte) {
	a.emit(rex(true, false, false, reg.IsExtended()))
	if imm == 1 {
		a.emit(0xD1)
		a.emit(modrm(3, 7, reg.LowBits()))
	} else {
		a.emit(0xC1)
		a.emit(modrm(3, 7, reg.LowBits()))
		a.emit(imm)
	}
}

// ============================================================================
// 比较指令
// ============================================================================

// CmpRegReg 比较: cmp left, right
func (a *X64Assembler) CmpRegReg(left, right X64Reg) {
	a.emit(rex(true, right.IsExtended(), false, left.IsExtended()))
	a.emit(0x39)
	a.emit(modrm(3, right.LowBits(), left.LowBits()))
}

// CmpRegImm32 比较立即数: cmp reg, imm32
func (a *X64Assembler) CmpRegImm32(reg X64Reg, imm int32) {
	a.emit(rex(true, false, false, reg.IsExtended()))
	if imm >= -128 && imm <= 127 {
		a.emit(0x83)
		a.emit(modrm(3, 7, reg.LowBits()))
		a.emit(byte(imm))
	} else {
		a.emit(0x81)
		a.emit(modrm(3, 7, reg.LowBits()))
		a.emitU32(uint32(imm))
	}
}

// TestRegReg 测试: test reg1, reg2
func (a *X64Assembler) TestRegReg(reg1, reg2 X64Reg) {
	a.emit(rex(true, reg2.IsExtended(), false, reg1.IsExtended()))
	a.emit(0x85)
	a.emit(modrm(3, reg2.LowBits(), reg1.LowBits()))
}

// 条件设置指令（SETcc）

// SetE 设置等于: sete reg (ZF=1)
func (a *X64Assembler) SetE(reg X64Reg) {
	if reg.IsExtended() {
		a.emit(rex(false, false, false, true))
	}
	a.emit(0x0F, 0x94)
	a.emit(modrm(3, 0, reg.LowBits()))
}

// SetNE 设置不等于: setne reg (ZF=0)
func (a *X64Assembler) SetNE(reg X64Reg) {
	if reg.IsExtended() {
		a.emit(rex(false, false, false, true))
	}
	a.emit(0x0F, 0x95)
	a.emit(modrm(3, 0, reg.LowBits()))
}

// SetL 设置小于: setl reg (SF!=OF)
func (a *X64Assembler) SetL(reg X64Reg) {
	if reg.IsExtended() {
		a.emit(rex(false, false, false, true))
	}
	a.emit(0x0F, 0x9C)
	a.emit(modrm(3, 0, reg.LowBits()))
}

// SetLE 设置小于等于: setle reg (ZF=1 or SF!=OF)
func (a *X64Assembler) SetLE(reg X64Reg) {
	if reg.IsExtended() {
		a.emit(rex(false, false, false, true))
	}
	a.emit(0x0F, 0x9E)
	a.emit(modrm(3, 0, reg.LowBits()))
}

// SetG 设置大于: setg reg (ZF=0 and SF=OF)
func (a *X64Assembler) SetG(reg X64Reg) {
	if reg.IsExtended() {
		a.emit(rex(false, false, false, true))
	}
	a.emit(0x0F, 0x9F)
	a.emit(modrm(3, 0, reg.LowBits()))
}

// SetGE 设置大于等于: setge reg (SF=OF)
func (a *X64Assembler) SetGE(reg X64Reg) {
	if reg.IsExtended() {
		a.emit(rex(false, false, false, true))
	}
	a.emit(0x0F, 0x9D)
	a.emit(modrm(3, 0, reg.LowBits()))
}

// SetB 设置低于（无符号）: setb reg (CF=1)
func (a *X64Assembler) SetB(reg X64Reg) {
	if reg.IsExtended() {
		a.emit(rex(false, false, false, true))
	}
	a.emit(0x0F, 0x92)
	a.emit(modrm(3, 0, reg.LowBits()))
}

// SetBE 设置低于等于（无符号）: setbe reg (CF=1 或 ZF=1)
func (a *X64Assembler) SetBE(reg X64Reg) {
	if reg.IsExtended() {
		a.emit(rex(false, false, false, true))
	}
	a.emit(0x0F, 0x96)
	a.emit(modrm(3, 0, reg.LowBits()))
}

// SetA 设置高于（无符号）: seta reg (CF=0 且 ZF=0)
func (a *X64Assembler) SetA(reg X64Reg) {
	if reg.IsExtended() {
		a.emit(rex(false, false, false, true))
	}
	a.emit(0x0F, 0x97)
	a.emit(modrm(3, 0, reg.LowBits()))
}

// SetAE 设置高于等于（无符号）: setae reg (CF=0)
func (a *X64Assembler) SetAE(reg X64Reg) {
	if reg.IsExtended() {
		a.emit(rex(false, false, false, true))
	}
	a.emit(0x0F, 0x93)
	a.emit(modrm(3, 0, reg.LowBits()))
}

// MovzxReg8 零扩展 8 位到 64 位: movzx dst, src (8-bit)
func (a *X64Assembler) MovzxReg8(dst, src X64Reg) {
	a.emit(rex(true, dst.IsExtended(), false, src.IsExtended()))
	a.emit(0x0F, 0xB6)
	a.emit(modrm(3, dst.LowBits(), src.LowBits()))
}

// ============================================================================
// 栈操作指令
// ============================================================================

// Push 压栈: push reg
func (a *X64Assembler) Push(reg X64Reg) {
	if reg.IsExtended() {
		a.emit(rex(false, false, false, true))
	}
	a.emit(0x50 + reg.LowBits())
}

// Pop 出栈: pop reg
func (a *X64Assembler) Pop(reg X64Reg) {
	if reg.IsExtended() {
		a.emit(rex(false, false, false, true))
	}
	a.emit(0x58 + reg.LowBits())
}

// ============================================================================
// 跳转指令
// ============================================================================

// Jmp 无条件跳转（相对）
func (a *X64Assembler) Jmp(blockID int) {
	a.emit(0xE9)
	a.relocs = append(a.relocs, x64Reloc{
		offset: len(a.code),
		target: blockID,
		size:   4,
	})
	a.emitU32(0) // 占位符
}

// JmpReg 间接跳转: jmp reg
func (a *X64Assembler) JmpReg(reg X64Reg) {
	if reg.IsExtended() {
		a.emit(rex(false, false, false, true))
	}
	a.emit(0xFF)
	a.emit(modrm(3, 4, reg.LowBits()))
}

// Je 相等跳转: je label (ZF=1)
func (a *X64Assembler) Je(blockID int) {
	a.emit(0x0F, 0x84)
	a.relocs = append(a.relocs, x64Reloc{
		offset: len(a.code),
		target: blockID,
		size:   4,
	})
	a.emitU32(0)
}

// Jne 不相等跳转: jne label (ZF=0)
func (a *X64Assembler) Jne(blockID int) {
	a.emit(0x0F, 0x85)
	a.relocs = append(a.relocs, x64Reloc{
		offset: len(a.code),
		target: blockID,
		size:   4,
	})
	a.emitU32(0)
}

// Jl 小于跳转: jl label (SF!=OF)
func (a *X64Assembler) Jl(blockID int) {
	a.emit(0x0F, 0x8C)
	a.relocs = append(a.relocs, x64Reloc{
		offset: len(a.code),
		target: blockID,
		size:   4,
	})
	a.emitU32(0)
}

// Jle 小于等于跳转: jle label
func (a *X64Assembler) Jle(blockID int) {
	a.emit(0x0F, 0x8E)
	a.relocs = append(a.relocs, x64Reloc{
		offset: len(a.code),
		target: blockID,
		size:   4,
	})
	a.emitU32(0)
}

// Jg 大于跳转: jg label
func (a *X64Assembler) Jg(blockID int) {
	a.emit(0x0F, 0x8F)
	a.relocs = append(a.relocs, x64Reloc{
		offset: len(a.code),
		target: blockID,
		size:   4,
	})
	a.emitU32(0)
}

// Jge 大于等于跳转: jge label
func (a *X64Assembler) Jge(blockID int) {
	a.emit(0x0F, 0x8D)
	a.relocs = append(a.relocs, x64Reloc{
		offset: len(a.code),
		target: blockID,
		size:   4,
	})
	a.emitU32(0)
}

// ============================================================================
// 函数调用指令
// ============================================================================

// Call 函数调用
func (a *X64Assembler) Call(reg X64Reg) {
	if reg.IsExtended() {
		a.emit(rex(false, false, false, true))
	}
	a.emit(0xFF)
	a.emit(modrm(3, 2, reg.LowBits()))
}

// Ret 返回
func (a *X64Assembler) Ret() {
	a.emit(0xC3)
}

// ============================================================================
// 重定位解析
// ============================================================================

// resolveRelocations 解析所有重定位
func (a *X64Assembler) resolveRelocations() {
	for _, reloc := range a.relocs {
		if target, ok := a.labels[reloc.target]; ok {
			// 计算相对偏移（从指令结束位置开始）
			offset := int32(target - (reloc.offset + reloc.size))
			binary.LittleEndian.PutUint32(a.code[reloc.offset:], uint32(offset))
		}
	}
}

// ============================================================================
// XMM 寄存器定义
// ============================================================================

// XMMReg XMM 寄存器（用于浮点运算）
type XMMReg int

const (
	XMM0 XMMReg = iota
	XMM1
	XMM2
	XMM3
	XMM4
	XMM5
	XMM6
	XMM7
	XMM8
	XMM9
	XMM10
	XMM11
	XMM12
	XMM13
	XMM14
	XMM15
)

// String 返回寄存器名称
func (r XMMReg) String() string {
	if r >= XMM0 && r <= XMM15 {
		return "xmm" + string('0'+byte(r))
	}
	return "xmm?"
}

// IsExtended 检查是否是扩展寄存器（XMM8-XMM15）
func (r XMMReg) IsExtended() bool {
	return r >= XMM8
}

// LowBits 获取寄存器编码的低 3 位
func (r XMMReg) LowBits() byte {
	return byte(r) & 0x7
}

// ============================================================================
// 浮点指令（SSE2）
// ============================================================================

// MovsdRegReg 移动双精度浮点: movsd dst, src
func (a *X64Assembler) MovsdRegReg(dst, src XMMReg) {
	// F2 0F 10 /r (MOVSD xmm1, xmm2/m64)
	a.emit(0xF2)
	if dst.IsExtended() || src.IsExtended() {
		a.emit(rex(false, dst.IsExtended(), false, src.IsExtended()))
	}
	a.emit(0x0F, 0x10)
	a.emit(modrm(3, dst.LowBits(), src.LowBits()))
}

// MovsdRegMem 从内存加载双精度浮点: movsd dst, [base+offset]
func (a *X64Assembler) MovsdRegMem(dst XMMReg, base X64Reg, offset int32) {
	// F2 0F 10 /r (MOVSD xmm1, xmm2/m64)
	a.emit(0xF2)
	if dst.IsExtended() || base.IsExtended() {
		a.emit(rex(false, dst.IsExtended(), false, base.IsExtended()))
	}
	a.emit(0x0F, 0x10)
	a.emitMemOp(dst.LowBits(), base, offset)
}

// MovsdMemReg 存储双精度浮点到内存: movsd [base+offset], src
func (a *X64Assembler) MovsdMemReg(base X64Reg, offset int32, src XMMReg) {
	// F2 0F 11 /r (MOVSD xmm2/m64, xmm1)
	a.emit(0xF2)
	if src.IsExtended() || base.IsExtended() {
		a.emit(rex(false, src.IsExtended(), false, base.IsExtended()))
	}
	a.emit(0x0F, 0x11)
	a.emitMemOp(src.LowBits(), base, offset)
}

// MovqRegXmm 从 XMM 移动到通用寄存器: movq reg, xmm
func (a *X64Assembler) MovqRegXmm(dst X64Reg, src XMMReg) {
	// 66 REX.W 0F 7E /r (MOVQ r/m64, xmm)
	a.emit(0x66)
	a.emit(rex(true, src.IsExtended(), false, dst.IsExtended()))
	a.emit(0x0F, 0x7E)
	a.emit(modrm(3, src.LowBits(), dst.LowBits()))
}

// MovqXmmReg 从通用寄存器移动到 XMM: movq xmm, reg
func (a *X64Assembler) MovqXmmReg(dst XMMReg, src X64Reg) {
	// 66 REX.W 0F 6E /r (MOVQ xmm, r/m64)
	a.emit(0x66)
	a.emit(rex(true, dst.IsExtended(), false, src.IsExtended()))
	a.emit(0x0F, 0x6E)
	a.emit(modrm(3, dst.LowBits(), src.LowBits()))
}

// AddsdRegReg 双精度加法: addsd dst, src
func (a *X64Assembler) AddsdRegReg(dst, src XMMReg) {
	// F2 0F 58 /r (ADDSD xmm1, xmm2/m64)
	a.emit(0xF2)
	if dst.IsExtended() || src.IsExtended() {
		a.emit(rex(false, dst.IsExtended(), false, src.IsExtended()))
	}
	a.emit(0x0F, 0x58)
	a.emit(modrm(3, dst.LowBits(), src.LowBits()))
}

// SubsdRegReg 双精度减法: subsd dst, src
func (a *X64Assembler) SubsdRegReg(dst, src XMMReg) {
	// F2 0F 5C /r (SUBSD xmm1, xmm2/m64)
	a.emit(0xF2)
	if dst.IsExtended() || src.IsExtended() {
		a.emit(rex(false, dst.IsExtended(), false, src.IsExtended()))
	}
	a.emit(0x0F, 0x5C)
	a.emit(modrm(3, dst.LowBits(), src.LowBits()))
}

// MulsdRegReg 双精度乘法: mulsd dst, src
func (a *X64Assembler) MulsdRegReg(dst, src XMMReg) {
	// F2 0F 59 /r (MULSD xmm1, xmm2/m64)
	a.emit(0xF2)
	if dst.IsExtended() || src.IsExtended() {
		a.emit(rex(false, dst.IsExtended(), false, src.IsExtended()))
	}
	a.emit(0x0F, 0x59)
	a.emit(modrm(3, dst.LowBits(), src.LowBits()))
}

// DivsdRegReg 双精度除法: divsd dst, src
func (a *X64Assembler) DivsdRegReg(dst, src XMMReg) {
	// F2 0F 5E /r (DIVSD xmm1, xmm2/m64)
	a.emit(0xF2)
	if dst.IsExtended() || src.IsExtended() {
		a.emit(rex(false, dst.IsExtended(), false, src.IsExtended()))
	}
	a.emit(0x0F, 0x5E)
	a.emit(modrm(3, dst.LowBits(), src.LowBits()))
}

// UcomisdRegReg 双精度比较（设置标志位）: ucomisd dst, src
func (a *X64Assembler) UcomisdRegReg(dst, src XMMReg) {
	// 66 0F 2E /r (UCOMISD xmm1, xmm2/m64)
	a.emit(0x66)
	if dst.IsExtended() || src.IsExtended() {
		a.emit(rex(false, dst.IsExtended(), false, src.IsExtended()))
	}
	a.emit(0x0F, 0x2E)
	a.emit(modrm(3, dst.LowBits(), src.LowBits()))
}

// Cvtsi2sdRegReg 整数转双精度: cvtsi2sd dst, src
func (a *X64Assembler) Cvtsi2sdRegReg(dst XMMReg, src X64Reg) {
	// F2 REX.W 0F 2A /r (CVTSI2SD xmm1, r/m64)
	a.emit(0xF2)
	a.emit(rex(true, dst.IsExtended(), false, src.IsExtended()))
	a.emit(0x0F, 0x2A)
	a.emit(modrm(3, dst.LowBits(), src.LowBits()))
}

// Cvttsd2siRegReg 双精度转整数（截断）: cvttsd2si dst, src
func (a *X64Assembler) Cvttsd2siRegReg(dst X64Reg, src XMMReg) {
	// F2 REX.W 0F 2C /r (CVTTSD2SI r64, xmm1/m64)
	a.emit(0xF2)
	a.emit(rex(true, dst.IsExtended(), false, src.IsExtended()))
	a.emit(0x0F, 0x2C)
	a.emit(modrm(3, dst.LowBits(), src.LowBits()))
}

// XorpdRegReg 双精度异或（用于清零）: xorpd dst, src
func (a *X64Assembler) XorpdRegReg(dst, src XMMReg) {
	// 66 0F 57 /r (XORPD xmm1, xmm2/m128)
	a.emit(0x66)
	if dst.IsExtended() || src.IsExtended() {
		a.emit(rex(false, dst.IsExtended(), false, src.IsExtended()))
	}
	a.emit(0x0F, 0x57)
	a.emit(modrm(3, dst.LowBits(), src.LowBits()))
}

// emitMemOp 生成内存操作数编码
func (a *X64Assembler) emitMemOp(reg byte, base X64Reg, offset int32) {
	// 特殊处理 RSP 和 R12（需要 SIB 字节）
	if base == RSP || base == R12 {
		if offset == 0 {
			a.emit(modrm(0, reg, 4))
			a.emit(0x24) // SIB: scale=0, index=RSP, base=RSP/R12
		} else if offset >= -128 && offset <= 127 {
			a.emit(modrm(1, reg, 4))
			a.emit(0x24) // SIB
			a.emit(byte(offset))
		} else {
			a.emit(modrm(2, reg, 4))
			a.emit(0x24) // SIB
			a.emitU32(uint32(offset))
		}
		return
	}
	
	// 特殊处理 RBP 和 R13（不能用 mod=00）
	if base == RBP || base == R13 {
		if offset >= -128 && offset <= 127 {
			a.emit(modrm(1, reg, base.LowBits()))
			a.emit(byte(offset))
		} else {
			a.emit(modrm(2, reg, base.LowBits()))
			a.emitU32(uint32(offset))
		}
		return
	}
	
	// 普通情况
	if offset == 0 {
		a.emit(modrm(0, reg, base.LowBits()))
	} else if offset >= -128 && offset <= 127 {
		a.emit(modrm(1, reg, base.LowBits()))
		a.emit(byte(offset))
	} else {
		a.emit(modrm(2, reg, base.LowBits()))
		a.emitU32(uint32(offset))
	}
}
