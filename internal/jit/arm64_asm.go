// arm64_asm.go - ARM64 汇编器
//
// 本文件实现了 ARM64 (AArch64) 机器码生成的底层汇编器。
//
// ARM64 指令特点：
// - 固定 32 位指令长度
// - 31 个通用寄存器 (X0-X30) + SP + ZR
// - 加载/存储架构（不支持内存直接运算）

package jit

import (
	"encoding/binary"
)

// ============================================================================
// ARM64 寄存器定义
// ============================================================================

// ARM64Reg ARM64 寄存器
type ARM64Reg int

const (
	// 通用寄存器 (64-bit: X0-X30)
	X0 ARM64Reg = iota
	X1
	X2
	X3
	X4
	X5
	X6
	X7
	X8
	X9
	X10
	X11
	X12
	X13
	X14
	X15
	X16 // IP0 - 过程内调用暂存器
	X17 // IP1
	X18 // 平台寄存器
	X19
	X20
	X21
	X22
	X23
	X24
	X25
	X26
	X27
	X28
	X29 // FP - 帧指针
	X30 // LR - 链接寄存器
	
	// 特殊寄存器
	XSP  ARM64Reg = 31 // 栈指针
	XZR  ARM64Reg = 31 // 零寄存器（与 SP 共享编码，由指令决定）
	
	ARM64RegNone ARM64Reg = -1
)

// String 返回寄存器名称
func (r ARM64Reg) String() string {
	if r >= X0 && r <= X28 {
		return "x" + string('0'+byte(r))
	}
	switch r {
	case X29:
		return "fp"
	case X30:
		return "lr"
	case XSP:
		return "sp"
	default:
		return "???"
	}
}

// Encode 获取寄存器编码
func (r ARM64Reg) Encode() uint32 {
	if r < 0 {
		return 31 // XZR
	}
	return uint32(r)
}

// ============================================================================
// ARM64 汇编器
// ============================================================================

// ARM64Assembler ARM64 汇编器
type ARM64Assembler struct {
	code   []byte
	labels map[int]int
	relocs []arm64Reloc
}

type arm64Reloc struct {
	offset int  // 代码中的偏移
	target int  // 目标块 ID
	kind   int  // 重定位类型
}

const (
	relocBranch   = 1 // B/BL 指令（26 位偏移）
	relocCondBr   = 2 // B.cond 指令（19 位偏移）
	relocCBZ      = 3 // CBZ/CBNZ 指令（19 位偏移）
)

// NewARM64Assembler 创建 ARM64 汇编器
func NewARM64Assembler() *ARM64Assembler {
	return &ARM64Assembler{
		code:   make([]byte, 0, 1024),
		labels: make(map[int]int),
	}
}

// Reset 重置汇编器
func (a *ARM64Assembler) Reset() {
	a.code = a.code[:0]
	a.labels = make(map[int]int)
	a.relocs = nil
}

// Code 获取生成的机器码
func (a *ARM64Assembler) Code() []byte {
	a.resolveRelocations()
	return a.code
}

// Len 返回当前代码长度
func (a *ARM64Assembler) Len() int {
	return len(a.code)
}

// emit 写入 32 位指令
func (a *ARM64Assembler) emit(instr uint32) {
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, instr)
	a.code = append(a.code, buf...)
}

// Label 定义标签
func (a *ARM64Assembler) Label(id int) {
	a.labels[id] = len(a.code)
}

// ============================================================================
// 数据移动指令
// ============================================================================

// MovRegReg 寄存器到寄存器: mov dst, src
func (a *ARM64Assembler) MovRegReg(dst, src ARM64Reg) {
	// ORR Xd, XZR, Xn (mov alias)
	instr := uint32(0xAA0003E0) | // ORR X
		(src.Encode() << 16) |
		dst.Encode()
	a.emit(instr)
}

// MovRegImm16 加载 16 位立即数: movz dst, imm
func (a *ARM64Assembler) MovRegImm16(dst ARM64Reg, imm uint16, shift int) {
	// MOVZ Xd, #imm16, LSL #shift
	hw := uint32(shift / 16) // 0, 1, 2, 3
	instr := uint32(0xD2800000) | // MOVZ X
		(hw << 21) |
		(uint32(imm) << 5) |
		dst.Encode()
	a.emit(instr)
}

// MovkImm16 移动保持: movk dst, imm, lsl #shift
func (a *ARM64Assembler) MovkImm16(dst ARM64Reg, imm uint16, shift int) {
	hw := uint32(shift / 16)
	instr := uint32(0xF2800000) | // MOVK X
		(hw << 21) |
		(uint32(imm) << 5) |
		dst.Encode()
	a.emit(instr)
}

// MovRegImm64 加载 64 位立即数
func (a *ARM64Assembler) MovRegImm64(dst ARM64Reg, imm uint64) {
	// 使用 MOVZ + MOVK 序列
	a.MovRegImm16(dst, uint16(imm), 0)
	if imm > 0xFFFF {
		a.MovkImm16(dst, uint16(imm>>16), 16)
	}
	if imm > 0xFFFFFFFF {
		a.MovkImm16(dst, uint16(imm>>32), 32)
	}
	if imm > 0xFFFFFFFFFFFF {
		a.MovkImm16(dst, uint16(imm>>48), 48)
	}
}

// LdrRegMem 从内存加载: ldr dst, [base, #offset]
func (a *ARM64Assembler) LdrRegMem(dst, base ARM64Reg, offset int32) {
	if offset >= 0 && offset <= 32760 && offset%8 == 0 {
		// LDR Xt, [Xn, #imm12] (unsigned offset)
		imm12 := uint32(offset / 8)
		instr := uint32(0xF9400000) | // LDR X, unsigned offset
			(imm12 << 10) |
			(base.Encode() << 5) |
			dst.Encode()
		a.emit(instr)
	} else {
		// 需要使用带符号偏移的形式
		// LDR Xt, [Xn, #simm9]! 或 LDUR
		if offset >= -256 && offset <= 255 {
			imm9 := uint32(offset) & 0x1FF
			instr := uint32(0xF8400000) | // LDUR X
				(imm9 << 12) |
				(base.Encode() << 5) |
				dst.Encode()
			a.emit(instr)
		} else {
			// 偏移太大，需要使用临时寄存器
			a.MovRegImm64(X17, uint64(offset))
			a.AddRegReg(X17, base, X17)
			a.LdrRegMem(dst, X17, 0)
		}
	}
}

// StrRegMem 存储到内存: str src, [base, #offset]
func (a *ARM64Assembler) StrRegMem(src, base ARM64Reg, offset int32) {
	if offset >= 0 && offset <= 32760 && offset%8 == 0 {
		imm12 := uint32(offset / 8)
		instr := uint32(0xF9000000) | // STR X, unsigned offset
			(imm12 << 10) |
			(base.Encode() << 5) |
			src.Encode()
		a.emit(instr)
	} else {
		if offset >= -256 && offset <= 255 {
			imm9 := uint32(offset) & 0x1FF
			instr := uint32(0xF8000000) | // STUR X
				(imm9 << 12) |
				(base.Encode() << 5) |
				src.Encode()
			a.emit(instr)
		} else {
			a.MovRegImm64(X17, uint64(offset))
			a.AddRegReg(X17, base, X17)
			a.StrRegMem(src, X17, 0)
		}
	}
}

// ============================================================================
// 算术指令
// ============================================================================

// AddRegReg 加法: add dst, src1, src2
func (a *ARM64Assembler) AddRegReg(dst, src1, src2 ARM64Reg) {
	instr := uint32(0x8B000000) | // ADD X
		(src2.Encode() << 16) |
		(src1.Encode() << 5) |
		dst.Encode()
	a.emit(instr)
}

// AddRegImm12 加法立即数: add dst, src, #imm12
func (a *ARM64Assembler) AddRegImm12(dst, src ARM64Reg, imm uint32) {
	if imm > 4095 {
		// 太大，使用临时寄存器
		a.MovRegImm64(X17, uint64(imm))
		a.AddRegReg(dst, src, X17)
		return
	}
	instr := uint32(0x91000000) | // ADD X, immediate
		(imm << 10) |
		(src.Encode() << 5) |
		dst.Encode()
	a.emit(instr)
}

// SubRegReg 减法: sub dst, src1, src2
func (a *ARM64Assembler) SubRegReg(dst, src1, src2 ARM64Reg) {
	instr := uint32(0xCB000000) | // SUB X
		(src2.Encode() << 16) |
		(src1.Encode() << 5) |
		dst.Encode()
	a.emit(instr)
}

// SubRegImm12 减法立即数: sub dst, src, #imm12
func (a *ARM64Assembler) SubRegImm12(dst, src ARM64Reg, imm uint32) {
	if imm > 4095 {
		a.MovRegImm64(X17, uint64(imm))
		a.SubRegReg(dst, src, X17)
		return
	}
	instr := uint32(0xD1000000) | // SUB X, immediate
		(imm << 10) |
		(src.Encode() << 5) |
		dst.Encode()
	a.emit(instr)
}

// MulReg 乘法: mul dst, src1, src2
func (a *ARM64Assembler) MulReg(dst, src1, src2 ARM64Reg) {
	// MADD Xd, Xn, Xm, XZR (mul alias)
	instr := uint32(0x9B007C00) | // MADD X
		(src2.Encode() << 16) |
		(src1.Encode() << 5) |
		dst.Encode()
	a.emit(instr)
}

// SdivReg 有符号除法: sdiv dst, src1, src2
func (a *ARM64Assembler) SdivReg(dst, src1, src2 ARM64Reg) {
	instr := uint32(0x9AC00C00) | // SDIV X
		(src2.Encode() << 16) |
		(src1.Encode() << 5) |
		dst.Encode()
	a.emit(instr)
}

// MsubReg 乘减（用于取模）: msub dst, mul1, mul2, sub
// dst = sub - mul1 * mul2
func (a *ARM64Assembler) MsubReg(dst, mul1, mul2, sub ARM64Reg) {
	instr := uint32(0x9B008000) | // MSUB X
		(mul2.Encode() << 16) |
		(sub.Encode() << 10) |
		(mul1.Encode() << 5) |
		dst.Encode()
	a.emit(instr)
}

// NegReg 取负: neg dst, src
func (a *ARM64Assembler) NegReg(dst, src ARM64Reg) {
	// SUB Xd, XZR, Xn (neg alias)
	a.SubRegReg(dst, XZR, src)
}

// ============================================================================
// 位运算指令
// ============================================================================

// AndRegReg 位与: and dst, src1, src2
func (a *ARM64Assembler) AndRegReg(dst, src1, src2 ARM64Reg) {
	instr := uint32(0x8A000000) | // AND X
		(src2.Encode() << 16) |
		(src1.Encode() << 5) |
		dst.Encode()
	a.emit(instr)
}

// OrrRegReg 位或: orr dst, src1, src2
func (a *ARM64Assembler) OrrRegReg(dst, src1, src2 ARM64Reg) {
	instr := uint32(0xAA000000) | // ORR X
		(src2.Encode() << 16) |
		(src1.Encode() << 5) |
		dst.Encode()
	a.emit(instr)
}

// EorRegReg 位异或: eor dst, src1, src2
func (a *ARM64Assembler) EorRegReg(dst, src1, src2 ARM64Reg) {
	instr := uint32(0xCA000000) | // EOR X
		(src2.Encode() << 16) |
		(src1.Encode() << 5) |
		dst.Encode()
	a.emit(instr)
}

// MvnReg 位非: mvn dst, src
func (a *ARM64Assembler) MvnReg(dst, src ARM64Reg) {
	// ORN Xd, XZR, Xn (mvn alias)
	instr := uint32(0xAA200000) | // ORN X
		(src.Encode() << 16) |
		(uint32(31) << 5) | // XZR
		dst.Encode()
	a.emit(instr)
}

// LslReg 逻辑左移: lsl dst, src, shift
func (a *ARM64Assembler) LslReg(dst, src, shift ARM64Reg) {
	// LSLV Xd, Xn, Xm
	instr := uint32(0x9AC02000) | // LSLV X
		(shift.Encode() << 16) |
		(src.Encode() << 5) |
		dst.Encode()
	a.emit(instr)
}

// LslImm 逻辑左移立即数: lsl dst, src, #shift
func (a *ARM64Assembler) LslImm(dst, src ARM64Reg, shift uint32) {
	// UBFM Xd, Xn, #(-shift mod 64), #(63-shift)
	immr := (64 - shift) & 0x3F
	imms := 63 - shift
	instr := uint32(0xD3400000) | // UBFM X (LSL alias)
		(immr << 16) |
		(imms << 10) |
		(src.Encode() << 5) |
		dst.Encode()
	a.emit(instr)
}

// AsrReg 算术右移: asr dst, src, shift
func (a *ARM64Assembler) AsrReg(dst, src, shift ARM64Reg) {
	// ASRV Xd, Xn, Xm
	instr := uint32(0x9AC02800) | // ASRV X
		(shift.Encode() << 16) |
		(src.Encode() << 5) |
		dst.Encode()
	a.emit(instr)
}

// AsrImm 算术右移立即数: asr dst, src, #shift
func (a *ARM64Assembler) AsrImm(dst, src ARM64Reg, shift uint32) {
	// SBFM Xd, Xn, #shift, #63 (ASR alias)
	instr := uint32(0x9340FC00) | // SBFM X (ASR alias)
		(shift << 16) |
		(src.Encode() << 5) |
		dst.Encode()
	a.emit(instr)
}

// ============================================================================
// 比较指令
// ============================================================================

// CmpRegReg 比较: cmp src1, src2
func (a *ARM64Assembler) CmpRegReg(src1, src2 ARM64Reg) {
	// SUBS XZR, Xn, Xm (cmp alias)
	instr := uint32(0xEB00001F) | // SUBS X -> XZR
		(src2.Encode() << 16) |
		(src1.Encode() << 5)
	a.emit(instr)
}

// CmpRegImm12 比较立即数: cmp src, #imm12
func (a *ARM64Assembler) CmpRegImm12(src ARM64Reg, imm uint32) {
	if imm > 4095 {
		a.MovRegImm64(X17, uint64(imm))
		a.CmpRegReg(src, X17)
		return
	}
	// SUBS XZR, Xn, #imm (cmp alias)
	instr := uint32(0xF100001F) | // SUBS X, immediate -> XZR
		(imm << 10) |
		(src.Encode() << 5)
	a.emit(instr)
}

// Cset 条件设置: cset dst, cond
// 条件为真时设为 1，否则为 0
func (a *ARM64Assembler) Cset(dst ARM64Reg, cond uint32) {
	// CSINC Xd, XZR, XZR, invert(cond)
	invertCond := cond ^ 1 // 反转条件
	instr := uint32(0x9A9F07E0) | // CSINC X
		(invertCond << 12) |
		dst.Encode()
	a.emit(instr)
}

// 条件码
const (
	CondEQ uint32 = 0x0 // 等于
	CondNE uint32 = 0x1 // 不等于
	CondLT uint32 = 0xB // 小于（有符号）
	CondLE uint32 = 0xD // 小于等于
	CondGT uint32 = 0xC // 大于
	CondGE uint32 = 0xA // 大于等于
)

// ============================================================================
// 跳转指令
// ============================================================================

// B 无条件跳转
func (a *ARM64Assembler) B(blockID int) {
	a.relocs = append(a.relocs, arm64Reloc{
		offset: len(a.code),
		target: blockID,
		kind:   relocBranch,
	})
	a.emit(0x14000000) // B (placeholder)
}

// Bcond 条件跳转: b.cond label
func (a *ARM64Assembler) Bcond(cond uint32, blockID int) {
	a.relocs = append(a.relocs, arm64Reloc{
		offset: len(a.code),
		target: blockID,
		kind:   relocCondBr,
	})
	instr := uint32(0x54000000) | cond // B.cond (placeholder)
	a.emit(instr)
}

// Cbz 比较为零跳转: cbz reg, label
func (a *ARM64Assembler) Cbz(reg ARM64Reg, blockID int) {
	a.relocs = append(a.relocs, arm64Reloc{
		offset: len(a.code),
		target: blockID,
		kind:   relocCBZ,
	})
	instr := uint32(0xB4000000) | reg.Encode() // CBZ X (placeholder)
	a.emit(instr)
}

// Cbnz 比较非零跳转: cbnz reg, label
func (a *ARM64Assembler) Cbnz(reg ARM64Reg, blockID int) {
	a.relocs = append(a.relocs, arm64Reloc{
		offset: len(a.code),
		target: blockID,
		kind:   relocCBZ,
	})
	instr := uint32(0xB5000000) | reg.Encode() // CBNZ X (placeholder)
	a.emit(instr)
}

// Blr 间接调用: blr reg
func (a *ARM64Assembler) Blr(reg ARM64Reg) {
	instr := uint32(0xD63F0000) | (reg.Encode() << 5) // BLR
	a.emit(instr)
}

// Ret 返回
func (a *ARM64Assembler) Ret() {
	// RET (uses X30/LR by default)
	a.emit(0xD65F03C0)
}

// ============================================================================
// 栈操作
// ============================================================================

// StpPre 存储寄存器对（预索引）: stp rt1, rt2, [base, #offset]!
func (a *ARM64Assembler) StpPre(rt1, rt2, base ARM64Reg, offset int32) {
	imm7 := uint32((offset / 8) & 0x7F)
	instr := uint32(0xA9800000) | // STP X, pre-index
		(imm7 << 15) |
		(rt2.Encode() << 10) |
		(base.Encode() << 5) |
		rt1.Encode()
	a.emit(instr)
}

// LdpPost 加载寄存器对（后索引）: ldp rt1, rt2, [base], #offset
func (a *ARM64Assembler) LdpPost(rt1, rt2, base ARM64Reg, offset int32) {
	imm7 := uint32((offset / 8) & 0x7F)
	instr := uint32(0xA8C00000) | // LDP X, post-index
		(imm7 << 15) |
		(rt2.Encode() << 10) |
		(base.Encode() << 5) |
		rt1.Encode()
	a.emit(instr)
}

// ============================================================================
// 重定位解析
// ============================================================================

func (a *ARM64Assembler) resolveRelocations() {
	for _, reloc := range a.relocs {
		targetPos, ok := a.labels[reloc.target]
		if !ok {
			continue
		}
		
		// 计算偏移（以指令为单位，4 字节）
		offset := (targetPos - reloc.offset) / 4
		
		// 读取原指令
		instr := binary.LittleEndian.Uint32(a.code[reloc.offset:])
		
		switch reloc.kind {
		case relocBranch:
			// B 指令：26 位偏移
			instr = (instr &^ 0x03FFFFFF) | (uint32(offset) & 0x03FFFFFF)
		case relocCondBr:
			// B.cond 指令：19 位偏移
			instr = (instr &^ 0x00FFFFE0) | ((uint32(offset) & 0x7FFFF) << 5)
		case relocCBZ:
			// CBZ/CBNZ 指令：19 位偏移
			instr = (instr &^ 0x00FFFFE0) | ((uint32(offset) & 0x7FFFF) << 5)
		}
		
		binary.LittleEndian.PutUint32(a.code[reloc.offset:], instr)
	}
}
