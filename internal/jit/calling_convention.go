// calling_convention.go - JIT 调用约定定义
//
// 本文件定义了 Sola JIT 编译器使用的调用约定。
// 支持 Windows x64 和 System V AMD64 两种主流调用约定。
//
// Sola 统一调用约定:
// - 基于目标平台的原生调用约定
// - 参数通过寄存器和栈传递
// - 返回值通过 RAX 返回
// - 支持闭包（通过专用寄存器传递闭包指针）

package jit

import (
	"runtime"
)

// SolaCallingConvType 调用约定类型
type SolaCallingConvType int

const (
	// SolaCallingConvDefault Sola 默认调用约定（基于平台）
	SolaCallingConvDefault SolaCallingConvType = iota
	// SolaCallingConvWindowsX64 Windows x64 调用约定
	SolaCallingConvWindowsX64
	// SolaCallingConvSystemV System V AMD64 调用约定 (Linux/macOS)
	SolaCallingConvSystemV
	// SolaCallingConvFastCall 快速调用（最多 2 个寄存器参数）
	SolaCallingConvFastCall
)

// SolaCallingConv 调用约定详细信息
type SolaCallingConv struct {
	Type        SolaCallingConvType
	ArgRegs     []int  // 参数寄存器（按顺序）
	RetReg      int    // 返回值寄存器
	CallerSaved []int  // 调用者保存的寄存器
	CalleeSaved []int  // 被调用者保存的寄存器
	ShadowSpace int    // 阴影空间大小（字节）
	StackAlign  int    // 栈对齐要求（字节）
	
	// 闭包支持
	ClosureReg  int    // 闭包指针寄存器（-1 表示不使用）
	
	// 浮点支持
	FloatArgRegs []int // 浮点参数寄存器
	FloatRetReg  int   // 浮点返回值寄存器
}

// 寄存器编号常量（与 x64_asm.go 中的 X64Reg 对应）
const (
	RegRAX = 0
	RegRCX = 1
	RegRDX = 2
	RegRBX = 3
	RegRSP = 4
	RegRBP = 5
	RegRSI = 6
	RegRDI = 7
	RegR8  = 8
	RegR9  = 9
	RegR10 = 10
	RegR11 = 11
	RegR12 = 12
	RegR13 = 13
	RegR14 = 14
	RegR15 = 15
	
	// 浮点寄存器
	RegXMM0 = 16
	RegXMM1 = 17
	RegXMM2 = 18
	RegXMM3 = 19
	RegXMM4 = 20
	RegXMM5 = 21
	RegXMM6 = 22
	RegXMM7 = 23
)

// WindowsX64Conv Windows x64 调用约定
var WindowsX64Conv = SolaCallingConv{
	Type:        SolaCallingConvWindowsX64,
	ArgRegs:     []int{RegRCX, RegRDX, RegR8, RegR9},
	RetReg:      RegRAX,
	CallerSaved: []int{RegRAX, RegRCX, RegRDX, RegR8, RegR9, RegR10, RegR11},
	CalleeSaved: []int{RegRBX, RegRSI, RegRDI, RegR12, RegR13, RegR14, RegR15},
	ShadowSpace: 32,
	StackAlign:  16,
	ClosureReg:  RegR15,
	FloatArgRegs: []int{RegXMM0, RegXMM1, RegXMM2, RegXMM3},
	FloatRetReg: RegXMM0,
}

// SystemVConv System V AMD64 调用约定 (Linux/macOS)
var SystemVConv = SolaCallingConv{
	Type:        SolaCallingConvSystemV,
	ArgRegs:     []int{RegRDI, RegRSI, RegRDX, RegRCX, RegR8, RegR9},
	RetReg:      RegRAX,
	CallerSaved: []int{RegRAX, RegRCX, RegRDX, RegRSI, RegRDI, RegR8, RegR9, RegR10, RegR11},
	CalleeSaved: []int{RegRBX, RegR12, RegR13, RegR14, RegR15},
	ShadowSpace: 0,
	StackAlign:  16,
	ClosureReg:  RegR15,
	FloatArgRegs: []int{RegXMM0, RegXMM1, RegXMM2, RegXMM3, RegXMM4, RegXMM5, RegXMM6, RegXMM7},
	FloatRetReg: RegXMM0,
}

// GetNativeConv 获取当前平台的原生调用约定
func GetNativeConv() SolaCallingConv {
	if runtime.GOOS == "windows" {
		return WindowsX64Conv
	}
	return SystemVConv
}

// GetSolaConv 获取 Sola 调用约定（基于当前平台）
func GetSolaConv() SolaCallingConv {
	conv := GetNativeConv()
	conv.Type = SolaCallingConvDefault
	return conv
}

// ============================================================================
// 帧布局
// ============================================================================

// FrameLayout 栈帧布局
type FrameLayout struct {
	TotalSize      int
	LocalsSize     int
	SpillsSize     int
	ArgsOnStack    int
	CalleeSaveSize int
	
	ReturnAddrOffset int
	OldRBPOffset     int
	FirstLocalOffset int
	SpillAreaOffset  int
	
	NumArgs          int
	ArgsInRegs       int
	FirstStackArg    int
	
	LocalOffsets     []int
	
	HasClosure       bool
	ClosureOffset    int
	NumUpvalues      int
	UpvalueOffsets   []int
}

// NewFrameLayout 创建新的帧布局
func NewFrameLayout(numArgs, numLocals, numSpills int, conv SolaCallingConv) *FrameLayout {
	fl := &FrameLayout{
		NumArgs:      numArgs,
		LocalOffsets: make([]int, numLocals),
	}
	
	fl.ArgsInRegs = numArgs
	if fl.ArgsInRegs > len(conv.ArgRegs) {
		fl.ArgsInRegs = len(conv.ArgRegs)
	}
	
	if numArgs > len(conv.ArgRegs) {
		fl.ArgsOnStack = (numArgs - len(conv.ArgRegs)) * 8
	}
	
	fl.LocalsSize = numLocals * 8
	fl.SpillsSize = numSpills * 8
	
	fl.ReturnAddrOffset = 8
	fl.OldRBPOffset = 0
	fl.FirstLocalOffset = -8
	
	for i := 0; i < numLocals; i++ {
		fl.LocalOffsets[i] = fl.FirstLocalOffset - i*8
	}
	
	fl.SpillAreaOffset = fl.FirstLocalOffset - numLocals*8
	
	fl.TotalSize = fl.LocalsSize + fl.SpillsSize + conv.ShadowSpace
	fl.TotalSize = (fl.TotalSize + 15) & ^15
	
	return fl
}

// GetLocalOffset 获取局部变量偏移
func (fl *FrameLayout) GetLocalOffset(index int) int {
	if index >= 0 && index < len(fl.LocalOffsets) {
		return fl.LocalOffsets[index]
	}
	return fl.FirstLocalOffset - index*8
}

// GetSpillOffset 获取溢出槽偏移
func (fl *FrameLayout) GetSpillOffset(slot int) int {
	return fl.SpillAreaOffset - slot*8
}

// GetArgOffset 获取参数偏移
func (fl *FrameLayout) GetArgOffset(index int, conv SolaCallingConv) int {
	if index < len(conv.ArgRegs) {
		return fl.GetLocalOffset(index + 1)
	}
	stackIndex := index - len(conv.ArgRegs)
	return fl.ReturnAddrOffset + 8 + conv.ShadowSpace + stackIndex*8
}

// SetupClosure 设置闭包信息
func (fl *FrameLayout) SetupClosure(numUpvalues int) {
	fl.HasClosure = true
	fl.NumUpvalues = numUpvalues
	fl.ClosureOffset = fl.GetLocalOffset(0)
	
	fl.UpvalueOffsets = make([]int, numUpvalues)
	for i := 0; i < numUpvalues; i++ {
		fl.UpvalueOffsets[i] = 16 + i*8
	}
}

// ============================================================================
// 调用站点信息
// ============================================================================

// CallSiteInfo 调用站点信息
type CallSiteInfo struct {
	IsDirect    bool
	IsVirtual   bool
	IsTailCall  bool
	
	TargetName  string
	TargetAddr  uintptr
	VTableIndex int
	
	NumArgs     int
	ArgTypes    []ValueType
	
	HasReturn   bool
	ReturnType  ValueType
	
	CodeOffset  int
	PatchOffset int
}

// NewCallSiteInfo 创建调用站点信息
func NewCallSiteInfo(targetName string, numArgs int) *CallSiteInfo {
	return &CallSiteInfo{
		TargetName: targetName,
		NumArgs:    numArgs,
		ArgTypes:   make([]ValueType, numArgs),
		HasReturn:  true,
		ReturnType: TypeUnknown,
	}
}

// ============================================================================
// 辅助函数
// ============================================================================

// IsCallerSaved 检查寄存器是否是调用者保存的
func IsCallerSaved(reg int, conv SolaCallingConv) bool {
	for _, r := range conv.CallerSaved {
		if r == reg {
			return true
		}
	}
	return false
}

// IsCalleeSaved 检查寄存器是否是被调用者保存的
func IsCalleeSaved(reg int, conv SolaCallingConv) bool {
	for _, r := range conv.CalleeSaved {
		if r == reg {
			return true
		}
	}
	return false
}

// GetArgReg 获取参数寄存器
func GetArgReg(index int, conv SolaCallingConv) (int, bool) {
	if index >= 0 && index < len(conv.ArgRegs) {
		return conv.ArgRegs[index], true
	}
	return -1, false
}

// GetFloatArgReg 获取浮点参数寄存器
func GetFloatArgReg(index int, conv SolaCallingConv) (int, bool) {
	if index >= 0 && index < len(conv.FloatArgRegs) {
		return conv.FloatArgRegs[index], true
	}
	return -1, false
}

// CalcStackArgOffset 计算栈参数偏移
func CalcStackArgOffset(argIndex int, conv SolaCallingConv) int {
	if argIndex < len(conv.ArgRegs) {
		return -1
	}
	stackIndex := argIndex - len(conv.ArgRegs)
	return conv.ShadowSpace + stackIndex*8
}

// CalcCallStackSize 计算调用所需的栈空间
func CalcCallStackSize(numArgs int, conv SolaCallingConv) int {
	size := conv.ShadowSpace
	
	if numArgs > len(conv.ArgRegs) {
		size += (numArgs - len(conv.ArgRegs)) * 8
	}
	
	return (size + 15) & ^15
}
