package platform

import (
	"encoding/binary"
)

// ============================================================================
// ARM64 汇编器
// ============================================================================

// ARM64Register ARM64 寄存器
type ARM64Register int

const (
	RegX0 ARM64Register = iota
	RegX1
	RegX2
	RegX3
	RegX4
	RegX5
	RegX6
	RegX7
	RegX8
	RegX9
	RegX10
	RegX11
	RegX12
	RegX13
	RegX14
	RegX15
	RegX16
	RegX17
	RegX18
	RegX19
	RegX20
	RegX21
	RegX22
	RegX23
	RegX24
	RegX25
	RegX26
	RegX27
	RegX28
	RegX29 // 帧指针 (FP)
	RegX30 // 链接寄存器 (LR)
	RegSP  // 栈指针
)

// ARM64Assembler ARM64 汇编器
type ARM64Assembler struct {
	code   []byte
	labels map[int]int
	relocs []arm64Reloc
}

type arm64Reloc struct {
	offset int
	target int
}

// NewARM64Assembler 创建汇编器
func NewARM64Assembler() *ARM64Assembler {
	return &ARM64Assembler{
		code:   make([]byte, 0, 1024),
		labels: make(map[int]int),
		relocs: make([]arm64Reloc, 0),
	}
}

// Reset 重置汇编器
func (a *ARM64Assembler) Reset() {
	a.code = a.code[:0]
	a.labels = make(map[int]int)
	a.relocs = a.relocs[:0]
}

// Code 获取生成的机器码
func (a *ARM64Assembler) Code() []byte {
	a.resolveRelocations()
	return a.code
}

func (a *ARM64Assembler) emit(instr uint32) {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, instr)
	a.code = append(a.code, b...)
}

// Label 标记标签
func (a *ARM64Assembler) Label(id int) {
	a.labels[id] = len(a.code)
}

// STP_PRE 存储寄存器对（预索引）
func (a *ARM64Assembler) STP_PRE(rt1, rt2 ARM64Register, base ARM64Register, offset int32) {
	// stp rt1, rt2, [base, #offset]!
	imm7 := uint32((offset / 8) & 0x7F)
	instr := uint32(0xA9800000) | // stp x, pre-index
		(imm7 << 15) |
		(uint32(rt2) << 10) |
		(uint32(base) << 5) |
		uint32(rt1)
	a.emit(instr)
}

// LDP_POST 加载寄存器对（后索引）
func (a *ARM64Assembler) LDP_POST(rt1, rt2 ARM64Register, base ARM64Register, offset int32) {
	// ldp rt1, rt2, [base], #offset
	imm7 := uint32((offset / 8) & 0x7F)
	instr := uint32(0xA8C00000) | // ldp x, post-index
		(imm7 << 15) |
		(uint32(rt2) << 10) |
		(uint32(base) << 5) |
		uint32(rt1)
	a.emit(instr)
}

// MOV_REG 寄存器到寄存器
func (a *ARM64Assembler) MOV_REG(rd, rm ARM64Register) {
	// mov rd, rm -> orr rd, xzr, rm
	instr := uint32(0xAA0003E0) |
		(uint32(rm) << 16) |
		uint32(rd)
	a.emit(instr)
}

// MOV_IMM 立即数到寄存器
func (a *ARM64Assembler) MOV_IMM(rd ARM64Register, imm uint64) {
	// movz rd, #imm16
	// movk rd, #imm16, lsl #16
	// movk rd, #imm16, lsl #32
	// movk rd, #imm16, lsl #48
	
	// 第一个 16 位
	instr := uint32(0xD2800000) | // movz x
		(uint32(imm&0xFFFF) << 5) |
		uint32(rd)
	a.emit(instr)
	
	// 后续 16 位（如果需要）
	if imm > 0xFFFF {
		instr = uint32(0xF2A00000) | // movk x, lsl #16
			(uint32((imm>>16)&0xFFFF) << 5) |
			uint32(rd)
		a.emit(instr)
	}
	if imm > 0xFFFFFFFF {
		instr = uint32(0xF2C00000) | // movk x, lsl #32
			(uint32((imm>>32)&0xFFFF) << 5) |
			uint32(rd)
		a.emit(instr)
	}
	if imm > 0xFFFFFFFFFFFF {
		instr = uint32(0xF2E00000) | // movk x, lsl #48
			(uint32((imm>>48)&0xFFFF) << 5) |
			uint32(rd)
		a.emit(instr)
	}
}

// LDR 从内存加载
func (a *ARM64Assembler) LDR(rt ARM64Register, base ARM64Register, offset int32) {
	// ldr rt, [base, #offset]
	imm12 := uint32((offset / 8) & 0xFFF)
	instr := uint32(0xF9400000) | // ldr x, unsigned offset
		(imm12 << 10) |
		(uint32(base) << 5) |
		uint32(rt)
	a.emit(instr)
}

// STR 存储到内存
func (a *ARM64Assembler) STR(rt ARM64Register, base ARM64Register, offset int32) {
	// str rt, [base, #offset]
	imm12 := uint32((offset / 8) & 0xFFF)
	instr := uint32(0xF9000000) | // str x, unsigned offset
		(imm12 << 10) |
		(uint32(base) << 5) |
		uint32(rt)
	a.emit(instr)
}

// ADD_REG 寄存器加法
func (a *ARM64Assembler) ADD_REG(rd, rn, rm ARM64Register) {
	// add rd, rn, rm
	instr := uint32(0x8B000000) | // add x
		(uint32(rm) << 16) |
		(uint32(rn) << 5) |
		uint32(rd)
	a.emit(instr)
}

// SUB_IMM 立即数减法
func (a *ARM64Assembler) SUB_IMM(rd, rn ARM64Register, imm uint32) {
	// sub rd, rn, #imm
	imm12 := imm & 0xFFF
	instr := uint32(0xD1000000) | // sub x, imm
		(imm12 << 10) |
		(uint32(rn) << 5) |
		uint32(rd)
	a.emit(instr)
}

// SUB_REG 寄存器减法
func (a *ARM64Assembler) SUB_REG(rd, rn, rm ARM64Register) {
	// sub rd, rn, rm
	instr := uint32(0xCB000000) | // sub x
		(uint32(rm) << 16) |
		(uint32(rn) << 5) |
		uint32(rd)
	a.emit(instr)
}

// MUL 乘法
func (a *ARM64Assembler) MUL(rd, rn, rm ARM64Register) {
	// mul rd, rn, rm -> madd rd, rn, rm, xzr
	instr := uint32(0x9B007C00) | // madd x, xzr
		(uint32(rm) << 16) |
		(uint32(rn) << 5) |
		uint32(rd)
	a.emit(instr)
}

// SDIV 有符号除法
func (a *ARM64Assembler) SDIV(rd, rn, rm ARM64Register) {
	// sdiv rd, rn, rm
	instr := uint32(0x9AC00C00) | // sdiv x
		(uint32(rm) << 16) |
		(uint32(rn) << 5) |
		uint32(rd)
	a.emit(instr)
}

// CBZ 比较并跳转（为零）
func (a *ARM64Assembler) CBZ(rt ARM64Register, labelID int) {
	// cbz rt, label
	a.relocs = append(a.relocs, arm64Reloc{
		offset: len(a.code),
		target: labelID,
	})
	instr := uint32(0xB4000000) | // cbz x
		uint32(rt)
	a.emit(instr)
}

// B 无条件跳转
func (a *ARM64Assembler) B(labelID int) {
	// b label
	a.relocs = append(a.relocs, arm64Reloc{
		offset: len(a.code),
		target: labelID,
	})
	a.emit(0x14000000) // b
}

// BL 带链接跳转（函数调用）
func (a *ARM64Assembler) BL(target uintptr) {
	// bl target
	a.emit(0x94000000) // bl (占位符)
}

// RET 返回
func (a *ARM64Assembler) RET() {
	// ret
	a.emit(0xD65F03C0) // ret x30
}

// resolveRelocations 解析重定位
func (a *ARM64Assembler) resolveRelocations() {
	for _, reloc := range a.relocs {
		if targetPos, ok := a.labels[reloc.target]; ok {
			// 计算相对偏移（以 4 字节为单位）
			offset := (targetPos - reloc.offset) / 4
			
			// 读取原始指令
			instr := binary.LittleEndian.Uint32(a.code[reloc.offset:])
			
			// 根据指令类型设置偏移
			if instr&0x7E000000 == 0x14000000 {
				// B 指令：26 位偏移
				instr = (instr &^ 0x03FFFFFF) | (uint32(offset) & 0x03FFFFFF)
			} else if instr&0x7E000000 == 0x34000000 {
				// CBZ 指令：19 位偏移
				instr = (instr &^ 0x00FFFFE0) | ((uint32(offset) & 0x7FFFF) << 5)
			}
			
			binary.LittleEndian.PutUint32(a.code[reloc.offset:], instr)
		}
	}
}
