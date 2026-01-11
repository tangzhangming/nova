// +build amd64

package jit

import (
	"encoding/binary"
	"unsafe"
)

// ============================================================================
// AMD64 寄存器定义
// ============================================================================

// Reg AMD64 寄存器
type Reg uint8

const (
	RAX Reg = iota
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
)

// 寄存器名称
var regNames = [16]string{
	"rax", "rcx", "rdx", "rbx", "rsp", "rbp", "rsi", "rdi",
	"r8", "r9", "r10", "r11", "r12", "r13", "r14", "r15",
}

func (r Reg) String() string {
	if r < 16 {
		return regNames[r]
	}
	return "unknown"
}

// isExtended 检查是否是扩展寄存器 (R8-R15)
func (r Reg) isExtended() bool {
	return r >= R8
}

// low3 返回寄存器编号的低3位
func (r Reg) low3() byte {
	return byte(r) & 0x07
}

// ============================================================================
// 重定位类型
// ============================================================================

// RelocType 重定位类型
type RelocType int

const (
	RelocAbsolute RelocType = iota // 绝对地址
	RelocRelative                  // 相对地址 (用于 CALL/JMP)
)

// Relocation 重定位条目
type Relocation struct {
	Offset int       // 在代码中的偏移
	Type   RelocType // 重定位类型
	Target uintptr   // 目标地址
	Addend int       // 附加值
}

// ============================================================================
// 代码生成器
// ============================================================================

// CodeGenerator AMD64 代码生成器
type CodeGenerator struct {
	code   []byte
	labels map[string]int
	relocs []Relocation

	// 寄存器分配状态
	usedRegs  uint16 // 位图：已使用的寄存器
	savedRegs []Reg  // 需要保存的寄存器
}

// NewCodeGenerator 创建代码生成器
func NewCodeGenerator() *CodeGenerator {
	return &CodeGenerator{
		code:   make([]byte, 0, 1024),
		labels: make(map[string]int),
		relocs: make([]Relocation, 0),
	}
}

// Code 获取生成的代码
func (g *CodeGenerator) Code() []byte {
	return g.code
}

// Size 获取代码大小
func (g *CodeGenerator) Size() int {
	return len(g.code)
}

// Relocations 获取重定位列表
func (g *CodeGenerator) Relocations() []Relocation {
	return g.relocs
}

// Reset 重置生成器
func (g *CodeGenerator) Reset() {
	g.code = g.code[:0]
	g.labels = make(map[string]int)
	g.relocs = g.relocs[:0]
	g.usedRegs = 0
	g.savedRegs = nil
}

// ============================================================================
// 基本发射方法
// ============================================================================

// emit 发射字节
func (g *CodeGenerator) emit(bytes ...byte) {
	g.code = append(g.code, bytes...)
}

// emit8 发射单字节
func (g *CodeGenerator) emit8(b byte) {
	g.code = append(g.code, b)
}

// emit32 发射32位立即数 (小端)
func (g *CodeGenerator) emit32(v uint32) {
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, v)
	g.code = append(g.code, buf...)
}

// emit64 发射64位立即数 (小端)
func (g *CodeGenerator) emit64(v uint64) {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, v)
	g.code = append(g.code, buf...)
}

// ============================================================================
// REX 前缀
// ============================================================================

// REX 前缀常量
const (
	rexBase = 0x40 // REX 基础
	rexW    = 0x08 // 64位操作数
	rexR    = 0x04 // ModR/M reg 字段扩展
	rexX    = 0x02 // SIB index 字段扩展
	rexB    = 0x01 // ModR/M r/m 或 SIB base 字段扩展
)

// emitREX 发射 REX 前缀
func (g *CodeGenerator) emitREX(w bool, r, x, b Reg) {
	rex := byte(rexBase)
	if w {
		rex |= rexW
	}
	if r.isExtended() {
		rex |= rexR
	}
	if x.isExtended() {
		rex |= rexX
	}
	if b.isExtended() {
		rex |= rexB
	}
	// 只在需要时发射 REX
	if rex != rexBase || w {
		g.emit8(rex)
	}
}

// emitREX64 发射 64 位 REX 前缀
func (g *CodeGenerator) emitREX64(r, b Reg) {
	g.emitREX(true, r, 0, b)
}

// ============================================================================
// ModR/M 和 SIB
// ============================================================================

// emitModRM 发射 ModR/M 字节
// mod: 00=内存, 01=内存+disp8, 10=内存+disp32, 11=寄存器
func (g *CodeGenerator) emitModRM(mod byte, reg, rm Reg) {
	g.emit8((mod << 6) | (reg.low3() << 3) | rm.low3())
}

// emitModRMReg 发射寄存器到寄存器的 ModR/M
func (g *CodeGenerator) emitModRMReg(reg, rm Reg) {
	g.emitModRM(0b11, reg, rm)
}

// emitModRMDisp8 发射带 8 位偏移的 ModR/M
func (g *CodeGenerator) emitModRMDisp8(reg, base Reg, disp int8) {
	g.emitModRM(0b01, reg, base)
	if base == RSP || base == R12 {
		g.emit8(0x24) // SIB: base=RSP, index=none, scale=1
	}
	g.emit8(byte(disp))
}

// emitModRMDisp32 发射带 32 位偏移的 ModR/M
func (g *CodeGenerator) emitModRMDisp32(reg, base Reg, disp int32) {
	g.emitModRM(0b10, reg, base)
	if base == RSP || base == R12 {
		g.emit8(0x24) // SIB
	}
	g.emit32(uint32(disp))
}

// ============================================================================
// 标签和跳转
// ============================================================================

// Label 定义标签
func (g *CodeGenerator) Label(name string) {
	g.labels[name] = len(g.code)
}

// LabelOffset 获取标签偏移
func (g *CodeGenerator) LabelOffset(name string) (int, bool) {
	offset, ok := g.labels[name]
	return offset, ok
}

// ============================================================================
// 数据移动指令
// ============================================================================

// EmitMovRegReg MOV reg, reg (64位)
func (g *CodeGenerator) EmitMovRegReg(dst, src Reg) {
	g.emitREX64(src, dst)
	g.emit8(0x89) // MOV r/m64, r64
	g.emitModRMReg(src, dst)
}

// EmitMovRegImm64 MOV reg, imm64
func (g *CodeGenerator) EmitMovRegImm64(dst Reg, imm uint64) {
	g.emitREX64(0, dst)
	g.emit8(0xB8 + dst.low3()) // MOV r64, imm64
	g.emit64(imm)
}

// EmitMovRegImm32 MOV reg, imm32 (零扩展到 64 位)
func (g *CodeGenerator) EmitMovRegImm32(dst Reg, imm uint32) {
	if dst.isExtended() {
		g.emit8(rexBase | rexB)
	}
	g.emit8(0xB8 + dst.low3())
	g.emit32(imm)
}

// EmitMovRegMem MOV reg, [base+disp]
func (g *CodeGenerator) EmitMovRegMem(dst, base Reg, disp int32) {
	g.emitREX64(dst, base)
	g.emit8(0x8B) // MOV r64, r/m64
	if disp == 0 && base != RBP && base != R13 {
		g.emitModRM(0b00, dst, base)
		if base == RSP || base == R12 {
			g.emit8(0x24)
		}
	} else if disp >= -128 && disp <= 127 {
		g.emitModRMDisp8(dst, base, int8(disp))
	} else {
		g.emitModRMDisp32(dst, base, disp)
	}
}

// EmitMovMemReg MOV [base+disp], reg
func (g *CodeGenerator) EmitMovMemReg(base Reg, disp int32, src Reg) {
	g.emitREX64(src, base)
	g.emit8(0x89) // MOV r/m64, r64
	if disp == 0 && base != RBP && base != R13 {
		g.emitModRM(0b00, src, base)
		if base == RSP || base == R12 {
			g.emit8(0x24)
		}
	} else if disp >= -128 && disp <= 127 {
		g.emitModRMDisp8(src, base, int8(disp))
	} else {
		g.emitModRMDisp32(src, base, disp)
	}
}

// ============================================================================
// 算术指令
// ============================================================================

// EmitAddRegReg ADD dst, src
func (g *CodeGenerator) EmitAddRegReg(dst, src Reg) {
	g.emitREX64(src, dst)
	g.emit8(0x01) // ADD r/m64, r64
	g.emitModRMReg(src, dst)
}

// EmitAddRegImm32 ADD reg, imm32
func (g *CodeGenerator) EmitAddRegImm32(dst Reg, imm int32) {
	g.emitREX64(0, dst)
	if imm >= -128 && imm <= 127 {
		g.emit8(0x83) // ADD r/m64, imm8
		g.emitModRMReg(0, dst)
		g.emit8(byte(imm))
	} else {
		g.emit8(0x81) // ADD r/m64, imm32
		g.emitModRMReg(0, dst)
		g.emit32(uint32(imm))
	}
}

// EmitSubRegReg SUB dst, src
func (g *CodeGenerator) EmitSubRegReg(dst, src Reg) {
	g.emitREX64(src, dst)
	g.emit8(0x29) // SUB r/m64, r64
	g.emitModRMReg(src, dst)
}

// EmitSubRegImm32 SUB reg, imm32
func (g *CodeGenerator) EmitSubRegImm32(dst Reg, imm int32) {
	g.emitREX64(0, dst)
	if imm >= -128 && imm <= 127 {
		g.emit8(0x83) // SUB r/m64, imm8
		g.emitModRMReg(5, dst) // /5
		g.emit8(byte(imm))
	} else {
		g.emit8(0x81) // SUB r/m64, imm32
		g.emitModRMReg(5, dst)
		g.emit32(uint32(imm))
	}
}

// EmitImulRegReg IMUL dst, src
func (g *CodeGenerator) EmitImulRegReg(dst, src Reg) {
	g.emitREX64(dst, src)
	g.emit(0x0F, 0xAF) // IMUL r64, r/m64
	g.emitModRMReg(dst, src)
}

// EmitNegReg NEG reg
func (g *CodeGenerator) EmitNegReg(reg Reg) {
	g.emitREX64(0, reg)
	g.emit8(0xF7) // NEG r/m64
	g.emitModRMReg(3, reg)
}

// ============================================================================
// 位运算指令
// ============================================================================

// EmitAndRegReg AND dst, src
func (g *CodeGenerator) EmitAndRegReg(dst, src Reg) {
	g.emitREX64(src, dst)
	g.emit8(0x21) // AND r/m64, r64
	g.emitModRMReg(src, dst)
}

// EmitOrRegReg OR dst, src
func (g *CodeGenerator) EmitOrRegReg(dst, src Reg) {
	g.emitREX64(src, dst)
	g.emit8(0x09) // OR r/m64, r64
	g.emitModRMReg(src, dst)
}

// EmitXorRegReg XOR dst, src
func (g *CodeGenerator) EmitXorRegReg(dst, src Reg) {
	g.emitREX64(src, dst)
	g.emit8(0x31) // XOR r/m64, r64
	g.emitModRMReg(src, dst)
}

// EmitNotReg NOT reg
func (g *CodeGenerator) EmitNotReg(reg Reg) {
	g.emitREX64(0, reg)
	g.emit8(0xF7) // NOT r/m64
	g.emitModRMReg(2, reg)
}

// EmitShlRegCL SHL reg, cl
func (g *CodeGenerator) EmitShlRegCL(reg Reg) {
	g.emitREX64(0, reg)
	g.emit8(0xD3) // SHL r/m64, cl
	g.emitModRMReg(4, reg)
}

// EmitShrRegCL SHR reg, cl
func (g *CodeGenerator) EmitShrRegCL(reg Reg) {
	g.emitREX64(0, reg)
	g.emit8(0xD3) // SHR r/m64, cl
	g.emitModRMReg(5, reg)
}

// ============================================================================
// 比较和跳转
// ============================================================================

// EmitCmpRegReg CMP dst, src
func (g *CodeGenerator) EmitCmpRegReg(dst, src Reg) {
	g.emitREX64(src, dst)
	g.emit8(0x39) // CMP r/m64, r64
	g.emitModRMReg(src, dst)
}

// EmitCmpRegImm32 CMP reg, imm32
func (g *CodeGenerator) EmitCmpRegImm32(reg Reg, imm int32) {
	g.emitREX64(0, reg)
	if imm >= -128 && imm <= 127 {
		g.emit8(0x83) // CMP r/m64, imm8
		g.emitModRMReg(7, reg)
		g.emit8(byte(imm))
	} else {
		g.emit8(0x81) // CMP r/m64, imm32
		g.emitModRMReg(7, reg)
		g.emit32(uint32(imm))
	}
}

// EmitTestRegReg TEST reg, reg
func (g *CodeGenerator) EmitTestRegReg(dst, src Reg) {
	g.emitREX64(src, dst)
	g.emit8(0x85) // TEST r/m64, r64
	g.emitModRMReg(src, dst)
}

// EmitJmp JMP rel32
func (g *CodeGenerator) EmitJmp(offset int32) {
	g.emit8(0xE9)
	g.emit32(uint32(offset))
}

// EmitJmpLabel JMP 到标签 (后续需要修补)
func (g *CodeGenerator) EmitJmpLabel(label string) int {
	pos := len(g.code)
	g.emit8(0xE9)
	g.emit32(0) // 占位符
	return pos
}

// EmitJe JE/JZ rel32
func (g *CodeGenerator) EmitJe(offset int32) {
	g.emit(0x0F, 0x84)
	g.emit32(uint32(offset))
}

// EmitJne JNE/JNZ rel32
func (g *CodeGenerator) EmitJne(offset int32) {
	g.emit(0x0F, 0x85)
	g.emit32(uint32(offset))
}

// EmitJl JL rel32 (有符号小于)
func (g *CodeGenerator) EmitJl(offset int32) {
	g.emit(0x0F, 0x8C)
	g.emit32(uint32(offset))
}

// EmitJle JLE rel32 (有符号小于等于)
func (g *CodeGenerator) EmitJle(offset int32) {
	g.emit(0x0F, 0x8E)
	g.emit32(uint32(offset))
}

// EmitJg JG rel32 (有符号大于)
func (g *CodeGenerator) EmitJg(offset int32) {
	g.emit(0x0F, 0x8F)
	g.emit32(uint32(offset))
}

// EmitJge JGE rel32 (有符号大于等于)
func (g *CodeGenerator) EmitJge(offset int32) {
	g.emit(0x0F, 0x8D)
	g.emit32(uint32(offset))
}

// ============================================================================
// 函数调用
// ============================================================================

// EmitCall CALL rel32
func (g *CodeGenerator) EmitCall(offset int32) {
	g.emit8(0xE8)
	g.emit32(uint32(offset))
}

// EmitCallAbs CALL 绝对地址 (通过 R11)
func (g *CodeGenerator) EmitCallAbs(addr uintptr) {
	// MOV R11, addr
	g.EmitMovRegImm64(R11, uint64(addr))
	// CALL R11
	g.emitREX(false, 0, 0, R11)
	g.emit8(0xFF)
	g.emitModRMReg(2, R11)
}

// EmitCallAbsWithReloc CALL 绝对地址 (带重定位)
func (g *CodeGenerator) EmitCallAbsWithReloc(addr uintptr) {
	// 记录重定位
	g.relocs = append(g.relocs, Relocation{
		Offset: len(g.code) + 2, // MOV 指令的立即数偏移
		Type:   RelocAbsolute,
		Target: addr,
	})
	g.EmitCallAbs(addr)
}

// EmitRet RET
func (g *CodeGenerator) EmitRet() {
	g.emit8(0xC3)
}

// ============================================================================
// 栈操作
// ============================================================================

// EmitPush PUSH reg
func (g *CodeGenerator) EmitPush(reg Reg) {
	if reg.isExtended() {
		g.emit8(rexBase | rexB)
	}
	g.emit8(0x50 + reg.low3())
}

// EmitPop POP reg
func (g *CodeGenerator) EmitPop(reg Reg) {
	if reg.isExtended() {
		g.emit8(rexBase | rexB)
	}
	g.emit8(0x58 + reg.low3())
}

// ============================================================================
// 函数序言和尾声
// ============================================================================

// EmitPrologue 发射函数序言
func (g *CodeGenerator) EmitPrologue(localSize int32) {
	// PUSH RBP
	g.EmitPush(RBP)
	// MOV RBP, RSP
	g.EmitMovRegReg(RBP, RSP)
	// SUB RSP, localSize (对齐到 16 字节)
	if localSize > 0 {
		aligned := (localSize + 15) & ^int32(15)
		g.EmitSubRegImm32(RSP, aligned)
	}
}

// EmitEpilogue 发射函数尾声
func (g *CodeGenerator) EmitEpilogue() {
	// MOV RSP, RBP
	g.EmitMovRegReg(RSP, RBP)
	// POP RBP
	g.EmitPop(RBP)
	// RET
	g.EmitRet()
}

// ============================================================================
// 条件设置
// ============================================================================

// EmitSete SETE reg (低 8 位)
func (g *CodeGenerator) EmitSete(reg Reg) {
	if reg.isExtended() || reg >= RSP {
		g.emit8(rexBase | (reg.low3() >> 3))
	}
	g.emit(0x0F, 0x94)
	g.emitModRMReg(0, reg)
}

// EmitSetne SETNE reg
func (g *CodeGenerator) EmitSetne(reg Reg) {
	if reg.isExtended() || reg >= RSP {
		g.emit8(rexBase | (reg.low3() >> 3))
	}
	g.emit(0x0F, 0x95)
	g.emitModRMReg(0, reg)
}

// EmitSetl SETL reg
func (g *CodeGenerator) EmitSetl(reg Reg) {
	if reg.isExtended() || reg >= RSP {
		g.emit8(rexBase | (reg.low3() >> 3))
	}
	g.emit(0x0F, 0x9C)
	g.emitModRMReg(0, reg)
}

// EmitSetle SETLE reg
func (g *CodeGenerator) EmitSetle(reg Reg) {
	if reg.isExtended() || reg >= RSP {
		g.emit8(rexBase | (reg.low3() >> 3))
	}
	g.emit(0x0F, 0x9E)
	g.emitModRMReg(0, reg)
}

// EmitSetg SETG reg
func (g *CodeGenerator) EmitSetg(reg Reg) {
	if reg.isExtended() || reg >= RSP {
		g.emit8(rexBase | (reg.low3() >> 3))
	}
	g.emit(0x0F, 0x9F)
	g.emitModRMReg(0, reg)
}

// EmitSetge SETGE reg
func (g *CodeGenerator) EmitSetge(reg Reg) {
	if reg.isExtended() || reg >= RSP {
		g.emit8(rexBase | (reg.low3() >> 3))
	}
	g.emit(0x0F, 0x9D)
	g.emitModRMReg(0, reg)
}

// ============================================================================
// 零扩展
// ============================================================================

// EmitMovzxByte MOVZX reg64, reg8 (零扩展字节到 64 位)
func (g *CodeGenerator) EmitMovzxByte(dst, src Reg) {
	g.emitREX64(dst, src)
	g.emit(0x0F, 0xB6)
	g.emitModRMReg(dst, src)
}

// ============================================================================
// NOP 填充
// ============================================================================

// EmitNop 发射 NOP
func (g *CodeGenerator) EmitNop() {
	g.emit8(0x90)
}

// EmitNopN 发射 N 字节的 NOP
func (g *CodeGenerator) EmitNopN(n int) {
	for n > 0 {
		switch {
		case n >= 9:
			// 9 字节 NOP: 66 0F 1F 84 00 00 00 00 00
			g.emit(0x66, 0x0F, 0x1F, 0x84, 0x00, 0x00, 0x00, 0x00, 0x00)
			n -= 9
		case n >= 8:
			// 8 字节 NOP: 0F 1F 84 00 00 00 00 00
			g.emit(0x0F, 0x1F, 0x84, 0x00, 0x00, 0x00, 0x00, 0x00)
			n -= 8
		case n >= 7:
			// 7 字节 NOP: 0F 1F 80 00 00 00 00
			g.emit(0x0F, 0x1F, 0x80, 0x00, 0x00, 0x00, 0x00)
			n -= 7
		case n >= 6:
			// 6 字节 NOP: 66 0F 1F 44 00 00
			g.emit(0x66, 0x0F, 0x1F, 0x44, 0x00, 0x00)
			n -= 6
		case n >= 5:
			// 5 字节 NOP: 0F 1F 44 00 00
			g.emit(0x0F, 0x1F, 0x44, 0x00, 0x00)
			n -= 5
		case n >= 4:
			// 4 字节 NOP: 0F 1F 40 00
			g.emit(0x0F, 0x1F, 0x40, 0x00)
			n -= 4
		case n >= 3:
			// 3 字节 NOP: 0F 1F 00
			g.emit(0x0F, 0x1F, 0x00)
			n -= 3
		case n >= 2:
			// 2 字节 NOP: 66 90
			g.emit(0x66, 0x90)
			n -= 2
		default:
			// 1 字节 NOP: 90
			g.emit8(0x90)
			n--
		}
	}
}

// ============================================================================
// 辅助方法
// ============================================================================

// PatchJump 修补跳转指令
func (g *CodeGenerator) PatchJump(pos int, target int) {
	offset := int32(target - (pos + 5)) // +5 是跳转指令的长度
	binary.LittleEndian.PutUint32(g.code[pos+1:], uint32(offset))
}

// CurrentOffset 获取当前代码偏移
func (g *CodeGenerator) CurrentOffset() int {
	return len(g.code)
}

// AllocReg 分配寄存器
func (g *CodeGenerator) AllocReg() Reg {
	// 优先使用调用者保存的寄存器
	callerSaved := []Reg{RAX, RCX, RDX, RSI, RDI, R8, R9, R10, R11}
	for _, r := range callerSaved {
		if g.usedRegs&(1<<r) == 0 {
			g.usedRegs |= 1 << r
			return r
		}
	}
	// 使用被调用者保存的寄存器
	calleeSaved := []Reg{RBX, R12, R13, R14, R15}
	for _, r := range calleeSaved {
		if g.usedRegs&(1<<r) == 0 {
			g.usedRegs |= 1 << r
			g.savedRegs = append(g.savedRegs, r)
			return r
		}
	}
	panic("out of registers")
}

// FreeReg 释放寄存器
func (g *CodeGenerator) FreeReg(r Reg) {
	g.usedRegs &^= 1 << r
}

// ============================================================================
// Value 操作辅助
// ============================================================================

// ValueSize Value 结构体大小 (24 字节: typ + padding + num + ptr)
const ValueSize = 24

// EmitLoadValueTag 加载 Value 的 tag 字段
func (g *CodeGenerator) EmitLoadValueTag(dst, base Reg, offset int32) {
	// tag 在 Value 结构体的第一个字节
	g.emitREX(false, dst, 0, base)
	g.emit8(0x0F)
	g.emit8(0xB6) // MOVZX r32, r/m8
	if offset == 0 && base != RBP && base != R13 {
		g.emitModRM(0b00, dst, base)
	} else if offset >= -128 && offset <= 127 {
		g.emitModRMDisp8(dst, base, int8(offset))
	} else {
		g.emitModRMDisp32(dst, base, offset)
	}
}

// EmitLoadValueNum 加载 Value 的 num 字段
func (g *CodeGenerator) EmitLoadValueNum(dst, base Reg, offset int32) {
	// num 在 Value 结构体的第 8 字节
	g.EmitMovRegMem(dst, base, offset+8)
}

// EmitStoreValueNum 存储 Value 的 num 字段
func (g *CodeGenerator) EmitStoreValueNum(base Reg, offset int32, src Reg) {
	g.EmitMovMemReg(base, offset+8, src)
}

// ============================================================================
// Go 调用约定
// ============================================================================

// Go 1.17+ 使用寄存器传参:
// 整数/指针参数: RAX, RBX, RCX, RDI, RSI, R8, R9, R10, R11
// 返回值: RAX, RBX (多返回值)
// 被调用者保存: RBP, R12-R15

// GoArgRegs Go 调用约定的参数寄存器
var GoArgRegs = []Reg{RAX, RBX, RCX, RDI, RSI, R8, R9, R10, R11}

// GoRetRegs Go 调用约定的返回值寄存器
var GoRetRegs = []Reg{RAX, RBX}

// CalleeSavedRegs 被调用者保存的寄存器
var CalleeSavedRegs = []Reg{RBP, R12, R13, R14, R15}

// CallerSavedRegs 调用者保存的寄存器
var CallerSavedRegs = []Reg{RAX, RCX, RDX, RSI, RDI, R8, R9, R10, R11}

// getPointerSize 获取指针大小
func getPointerSize() int {
	return int(unsafe.Sizeof(uintptr(0)))
}
