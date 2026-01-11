// +build amd64

package jit

import (
	"fmt"
	"unsafe"

	"github.com/tangzhangming/nova/internal/bytecode"
)

// ============================================================================
// IR 到机器码转换
// ============================================================================

// IREmitter IR 到机器码发射器
type IREmitter struct {
	cg *CodeGenerator

	// 标签位置映射
	labelOffsets map[string]int
	// 待修补的跳转
	pendingJumps []pendingJump

	// 局部变量偏移 (相对于 RBP)
	localOffsets []int32

	// Helper 函数地址
	helperAddrs map[string]uintptr

	// 栈深度跟踪 (用于调试)
	stackDepth int

	// 编译错误
	errors []error
}

// pendingJump 待修补的跳转
type pendingJump struct {
	codeOffset int    // 在机器码中的位置
	label      string // 目标标签
	isLoop     bool   // 是否是向后跳转 (loop)
}

// NewIREmitter 创建 IR 发射器
func NewIREmitter() *IREmitter {
	return &IREmitter{
		cg:           NewCodeGenerator(),
		labelOffsets: make(map[string]int),
		pendingJumps: make([]pendingJump, 0),
		helperAddrs:  make(map[string]uintptr),
	}
}

// SetHelperAddr 设置 Helper 函数地址
func (e *IREmitter) SetHelperAddr(name string, addr uintptr) {
	e.helperAddrs[name] = addr
}

// GetHelperAddr 获取 Helper 函数地址
func (e *IREmitter) GetHelperAddr(name string) uintptr {
	return e.helperAddrs[name]
}

// Reset 重置发射器
func (e *IREmitter) Reset() {
	e.cg.Reset()
	e.labelOffsets = make(map[string]int)
	e.pendingJumps = e.pendingJumps[:0]
	e.localOffsets = nil
	e.stackDepth = 0
	e.errors = nil
}

// Code 获取生成的机器码
func (e *IREmitter) Code() []byte {
	return e.cg.Code()
}

// Relocations 获取重定位列表
func (e *IREmitter) Relocations() []Relocation {
	return e.cg.Relocations()
}

// Errors 获取编译错误
func (e *IREmitter) Errors() []error {
	return e.errors
}

// ============================================================================
// 函数发射
// ============================================================================

// EmitFunction 发射整个函数
func (e *IREmitter) EmitFunction(fn *IRFunction) error {
	if fn == nil || fn.CFG == nil {
		return fmt.Errorf("invalid IR function")
	}

	// 计算局部变量偏移
	e.setupLocals(fn.LocalCount)

	// 计算需要的栈空间
	// 每个局部变量需要 24 字节 (Value 结构体大小)
	localSize := int32(fn.LocalCount * ValueSize)
	// 对齐到 16 字节
	localSize = (localSize + 15) & ^int32(15)
	// 额外留出一些栈空间用于临时存储
	stackSize := localSize + 128

	// 生成函数序言
	e.cg.EmitPrologue(stackSize)

	// 保存被调用者保存的寄存器
	e.emitSaveCalleeSaved()

	// 初始化局部变量为 null
	e.emitInitLocals(fn.LocalCount)

	// 遍历基本块生成代码
	for _, bb := range fn.CFG.Blocks {
		e.emitBasicBlock(bb)
	}

	// 修补所有跳转
	e.patchJumps()

	// 生成函数尾声
	e.emitRestoreCalleeSaved()
	e.cg.EmitEpilogue()

	if len(e.errors) > 0 {
		return e.errors[0]
	}
	return nil
}

// setupLocals 设置局部变量偏移
func (e *IREmitter) setupLocals(count int) {
	e.localOffsets = make([]int32, count)
	// 局部变量从 RBP-ValueSize 开始，向下增长
	for i := 0; i < count; i++ {
		e.localOffsets[i] = int32(-(i + 1) * ValueSize)
	}
}

// emitSaveCalleeSaved 保存被调用者保存的寄存器
func (e *IREmitter) emitSaveCalleeSaved() {
	// 保存 R12-R15
	e.cg.EmitPush(R12)
	e.cg.EmitPush(R13)
	e.cg.EmitPush(R14)
	e.cg.EmitPush(R15)
}

// emitRestoreCalleeSaved 恢复被调用者保存的寄存器
func (e *IREmitter) emitRestoreCalleeSaved() {
	// 恢复 R15-R12 (逆序)
	e.cg.EmitPop(R15)
	e.cg.EmitPop(R14)
	e.cg.EmitPop(R13)
	e.cg.EmitPop(R12)
}

// emitInitLocals 初始化局部变量为 null
func (e *IREmitter) emitInitLocals(count int) {
	if count == 0 {
		return
	}

	// XOR RAX, RAX (设置为 0)
	e.cg.EmitXorRegReg(RAX, RAX)

	// 将所有局部变量初始化为 null (全 0)
	for i := 0; i < count; i++ {
		offset := e.localOffsets[i]
		// 存储 24 字节的 0 (Value 结构体)
		e.cg.EmitMovMemReg(RBP, offset, RAX)
		e.cg.EmitMovMemReg(RBP, offset+8, RAX)
		e.cg.EmitMovMemReg(RBP, offset+16, RAX)
	}
}

// ============================================================================
// 基本块发射
// ============================================================================

// emitBasicBlock 发射基本块
func (e *IREmitter) emitBasicBlock(bb *BasicBlock) {
	// 记录标签位置
	if bb.Name != "" {
		e.labelOffsets[bb.Name] = e.cg.CurrentOffset()
	}

	// 发射所有指令
	for _, inst := range bb.Insts {
		e.emitInstruction(inst)
	}
}

// emitInstruction 发射单条指令
func (e *IREmitter) emitInstruction(inst IRInst) {
	// 记录标签
	if inst.Op == IR_LABEL {
		e.labelOffsets[inst.Label] = e.cg.CurrentOffset()
		return
	}

	switch inst.Op {
	// 栈操作
	case IR_NOP:
		e.cg.EmitNop()
	case IR_CONST:
		e.emitConst(inst.Value)
	case IR_POP:
		e.emitPop()
	case IR_DUP:
		e.emitDup()

	// 算术运算
	case IR_ADD:
		e.emitBinaryOp(IR_ADD)
	case IR_SUB:
		e.emitBinaryOp(IR_SUB)
	case IR_MUL:
		e.emitBinaryOp(IR_MUL)
	case IR_DIV:
		e.emitBinaryOp(IR_DIV)
	case IR_MOD:
		e.emitBinaryOp(IR_MOD)
	case IR_NEG:
		e.emitUnaryOp(IR_NEG)

	// 位运算
	case IR_BAND:
		e.emitBinaryOp(IR_BAND)
	case IR_BOR:
		e.emitBinaryOp(IR_BOR)
	case IR_BXOR:
		e.emitBinaryOp(IR_BXOR)
	case IR_BNOT:
		e.emitUnaryOp(IR_BNOT)
	case IR_SHL:
		e.emitBinaryOp(IR_SHL)
	case IR_SHR:
		e.emitBinaryOp(IR_SHR)

	// 比较运算
	case IR_EQ:
		e.emitComparison(IR_EQ)
	case IR_NE:
		e.emitComparison(IR_NE)
	case IR_LT:
		e.emitComparison(IR_LT)
	case IR_LE:
		e.emitComparison(IR_LE)
	case IR_GT:
		e.emitComparison(IR_GT)
	case IR_GE:
		e.emitComparison(IR_GE)

	// 逻辑运算
	case IR_NOT:
		e.emitUnaryOp(IR_NOT)
	case IR_AND:
		e.emitLogicalOp(IR_AND)
	case IR_OR:
		e.emitLogicalOp(IR_OR)

	// 变量操作
	case IR_LOAD_LOCAL:
		e.emitLoadLocal(inst.Arg1)
	case IR_STORE_LOCAL:
		e.emitStoreLocal(inst.Arg1)
	case IR_LOAD_GLOBAL:
		e.emitLoadGlobal(inst.Arg1)
	case IR_STORE_GLOBAL:
		e.emitStoreGlobal(inst.Arg1)

	// 跳转
	case IR_JUMP:
		e.emitJump(inst)
	case IR_JUMP_TRUE:
		e.emitJumpTrue(inst)
	case IR_JUMP_FALSE:
		e.emitJumpFalse(inst)
	case IR_LOOP:
		e.emitLoop(inst)

	// 函数调用
	case IR_CALL:
		e.emitCall(inst.Arg1)
	case IR_CALL_HELPER:
		e.emitCallHelper(inst.HelperName, inst.Arg1)
	case IR_RETURN:
		e.emitReturn()

	// 复杂操作 - 通过 Helper 调用
	case IR_NEW_OBJECT, IR_GET_FIELD, IR_SET_FIELD, IR_INVOKE:
		e.emitObjectOp(inst)
	case IR_NEW_ARRAY, IR_ARRAY_GET, IR_ARRAY_SET, IR_ARRAY_LEN:
		e.emitArrayOp(inst)
	case IR_SA_NEW, IR_SA_GET, IR_SA_SET, IR_SA_LEN, IR_SA_PUSH, IR_SA_HAS:
		e.emitSuperArrayOp(inst)

	default:
		e.errors = append(e.errors, fmt.Errorf("unsupported IR op: %s", inst.Op))
	}
}

// ============================================================================
// 栈操作
// ============================================================================

// emitConst 发射常量加载
func (e *IREmitter) emitConst(v bytecode.Value) {
	// 根据类型优化常量加载
	switch v.Type() {
	case bytecode.ValNull:
		// XOR RAX, RAX
		e.cg.EmitXorRegReg(RAX, RAX)
		e.cg.EmitPush(RAX) // typ = 0
		e.cg.EmitPush(RAX) // num = 0
		e.cg.EmitPush(RAX) // ptr = 0

	case bytecode.ValBool:
		e.cg.EmitXorRegReg(RAX, RAX)
		if v.AsBool() {
			e.cg.EmitMovRegImm32(RAX, 1)
		}
		// typ = 1 (bool)
		e.cg.EmitMovRegImm32(RBX, 1)
		e.cg.EmitPush(RBX)
		e.cg.EmitPush(RAX) // num = 0 or 1
		e.cg.EmitXorRegReg(RAX, RAX)
		e.cg.EmitPush(RAX) // ptr = 0

	case bytecode.ValInt:
		val := v.AsInt()
		// typ = 2 (int)
		e.cg.EmitMovRegImm32(RAX, 2)
		e.cg.EmitPush(RAX)
		// num = value
		e.cg.EmitMovRegImm64(RAX, uint64(val))
		e.cg.EmitPush(RAX)
		// ptr = 0
		e.cg.EmitXorRegReg(RAX, RAX)
		e.cg.EmitPush(RAX)

	case bytecode.ValFloat:
		val := v.AsFloat()
		// typ = 3 (float)
		e.cg.EmitMovRegImm32(RAX, 3)
		e.cg.EmitPush(RAX)
		// num = float bits (使用整数存储)
		bits := floatBits(val)
		e.cg.EmitMovRegImm64(RAX, bits)
		e.cg.EmitPush(RAX)
		// ptr = 0
		e.cg.EmitXorRegReg(RAX, RAX)
		e.cg.EmitPush(RAX)

	default:
		// 其他类型通过 Helper 加载
		e.emitCallHelper("LoadConst", 0)
	}

	e.stackDepth++
}

// emitPop 发射弹出操作
func (e *IREmitter) emitPop() {
	// 弹出 Value (24 字节)
	e.cg.EmitAddRegImm32(RSP, ValueSize)
	e.stackDepth--
}

// emitDup 发射复制栈顶
func (e *IREmitter) emitDup() {
	// 读取栈顶 Value
	e.cg.EmitMovRegMem(RAX, RSP, 0)
	e.cg.EmitMovRegMem(RBX, RSP, 8)
	e.cg.EmitMovRegMem(RCX, RSP, 16)
	// 压入复制
	e.cg.EmitPush(RCX)
	e.cg.EmitPush(RBX)
	e.cg.EmitPush(RAX)
	e.stackDepth++
}

// ============================================================================
// 算术运算 - 整数快速路径
// ============================================================================

// emitBinaryOp 发射二元运算
func (e *IREmitter) emitBinaryOp(op IROp) {
	// 整数快速路径：
	// 假设两个操作数都是整数，直接进行运算
	// 实际运行时需要类型检查，这里先实现简化版本

	// 弹出第二个操作数的 num 到 RBX
	e.cg.EmitMovRegMem(RBX, RSP, 8) // b.num
	e.cg.EmitAddRegImm32(RSP, ValueSize)

	// 弹出第一个操作数的 num 到 RAX
	e.cg.EmitMovRegMem(RAX, RSP, 8) // a.num
	e.cg.EmitAddRegImm32(RSP, ValueSize)

	// 执行运算
	switch op {
	case IR_ADD:
		e.cg.EmitAddRegReg(RAX, RBX)
	case IR_SUB:
		e.cg.EmitSubRegReg(RAX, RBX)
	case IR_MUL:
		e.cg.EmitImulRegReg(RAX, RBX)
	case IR_DIV:
		// IDIV 需要 RDX:RAX / src
		// 先将 RAX 符号扩展到 RDX:RAX
		e.cg.emit8(0x48) // REX.W
		e.cg.emit8(0x99) // CQO (sign-extend RAX to RDX:RAX)
		// IDIV RBX
		e.cg.emitREX64(0, RBX)
		e.cg.emit8(0xF7)
		e.cg.emitModRMReg(7, RBX) // /7 = IDIV
	case IR_MOD:
		// IDIV 后余数在 RDX
		e.cg.emit8(0x48) // REX.W
		e.cg.emit8(0x99) // CQO
		e.cg.emitREX64(0, RBX)
		e.cg.emit8(0xF7)
		e.cg.emitModRMReg(7, RBX)
		// MOV RAX, RDX (余数)
		e.cg.EmitMovRegReg(RAX, RDX)
	case IR_BAND:
		e.cg.EmitAndRegReg(RAX, RBX)
	case IR_BOR:
		e.cg.EmitOrRegReg(RAX, RBX)
	case IR_BXOR:
		e.cg.EmitXorRegReg(RAX, RBX)
	case IR_SHL:
		// SHL RAX, CL
		e.cg.EmitMovRegReg(RCX, RBX)
		e.cg.EmitShlRegCL(RAX)
	case IR_SHR:
		// SHR RAX, CL
		e.cg.EmitMovRegReg(RCX, RBX)
		e.cg.EmitShrRegCL(RAX)
	}

	// 压入结果 (作为整数)
	// typ = 2 (int)
	e.cg.EmitMovRegImm32(RBX, 2)
	e.cg.EmitPush(RBX)
	// num = result
	e.cg.EmitPush(RAX)
	// ptr = 0
	e.cg.EmitXorRegReg(RBX, RBX)
	e.cg.EmitPush(RBX)

	e.stackDepth-- // 两个操作数变一个结果
}

// emitUnaryOp 发射一元运算
func (e *IREmitter) emitUnaryOp(op IROp) {
	// 读取栈顶的 num
	e.cg.EmitMovRegMem(RAX, RSP, 8) // value.num

	switch op {
	case IR_NEG:
		e.cg.EmitNegReg(RAX)
	case IR_BNOT:
		e.cg.EmitNotReg(RAX)
	case IR_NOT:
		// 逻辑非：如果值为 0，结果为 1；否则为 0
		e.cg.EmitTestRegReg(RAX, RAX)
		e.cg.EmitSete(RAX)
		e.cg.EmitMovzxByte(RAX, RAX)
	}

	// 更新栈顶的 num
	e.cg.EmitMovMemReg(RSP, 8, RAX)

	if op == IR_NOT {
		// 更新类型为 bool
		e.cg.EmitMovRegImm32(RAX, 1) // ValBool
		e.cg.EmitMovMemReg(RSP, 0, RAX)
	}
}

// ============================================================================
// 比较运算
// ============================================================================

// emitComparison 发射比较运算
func (e *IREmitter) emitComparison(op IROp) {
	// 弹出第二个操作数
	e.cg.EmitMovRegMem(RBX, RSP, 8) // b.num
	e.cg.EmitAddRegImm32(RSP, ValueSize)

	// 弹出第一个操作数
	e.cg.EmitMovRegMem(RAX, RSP, 8) // a.num
	e.cg.EmitAddRegImm32(RSP, ValueSize)

	// 比较
	e.cg.EmitCmpRegReg(RAX, RBX)

	// 设置结果
	switch op {
	case IR_EQ:
		e.cg.EmitSete(RAX)
	case IR_NE:
		e.cg.EmitSetne(RAX)
	case IR_LT:
		e.cg.EmitSetl(RAX)
	case IR_LE:
		e.cg.EmitSetle(RAX)
	case IR_GT:
		e.cg.EmitSetg(RAX)
	case IR_GE:
		e.cg.EmitSetge(RAX)
	}

	// 零扩展到 64 位
	e.cg.EmitMovzxByte(RAX, RAX)

	// 压入布尔结果
	e.cg.EmitMovRegImm32(RBX, 1) // typ = bool
	e.cg.EmitPush(RBX)
	e.cg.EmitPush(RAX) // num = 0 or 1
	e.cg.EmitXorRegReg(RBX, RBX)
	e.cg.EmitPush(RBX) // ptr = 0

	e.stackDepth--
}

// emitLogicalOp 发射逻辑运算
func (e *IREmitter) emitLogicalOp(op IROp) {
	// 弹出第二个操作数
	e.cg.EmitMovRegMem(RBX, RSP, 8) // b.num
	e.cg.EmitAddRegImm32(RSP, ValueSize)

	// 弹出第一个操作数
	e.cg.EmitMovRegMem(RAX, RSP, 8) // a.num
	e.cg.EmitAddRegImm32(RSP, ValueSize)

	switch op {
	case IR_AND:
		// 如果 a 为假，返回 a；否则返回 b
		e.cg.EmitTestRegReg(RAX, RAX)
		// 使用 CMOV 条件移动
		// CMOVNE RAX, RBX (如果 a != 0，则 RAX = RBX)
		e.cg.emitREX64(RAX, RBX)
		e.cg.emit(0x0F, 0x45) // CMOVNE
		e.cg.emitModRMReg(RAX, RBX)
	case IR_OR:
		// 如果 a 为真，返回 a；否则返回 b
		e.cg.EmitTestRegReg(RAX, RAX)
		// CMOVE RAX, RBX (如果 a == 0，则 RAX = RBX)
		e.cg.emitREX64(RAX, RBX)
		e.cg.emit(0x0F, 0x44) // CMOVE
		e.cg.emitModRMReg(RAX, RBX)
	}

	// 压入结果
	e.cg.EmitMovRegImm32(RBX, 2) // typ = int (简化处理)
	e.cg.EmitPush(RBX)
	e.cg.EmitPush(RAX)
	e.cg.EmitXorRegReg(RBX, RBX)
	e.cg.EmitPush(RBX)

	e.stackDepth--
}

// ============================================================================
// 变量操作
// ============================================================================

// emitLoadLocal 发射加载局部变量
func (e *IREmitter) emitLoadLocal(slot int) {
	if slot >= len(e.localOffsets) {
		e.errors = append(e.errors, fmt.Errorf("invalid local slot: %d", slot))
		return
	}

	offset := e.localOffsets[slot]

	// 读取 Value 的三个字段
	e.cg.EmitMovRegMem(RAX, RBP, offset)    // typ
	e.cg.EmitMovRegMem(RBX, RBP, offset+8)  // num
	e.cg.EmitMovRegMem(RCX, RBP, offset+16) // ptr

	// 压入栈
	e.cg.EmitPush(RAX)
	e.cg.EmitPush(RBX)
	e.cg.EmitPush(RCX)

	e.stackDepth++
}

// emitStoreLocal 发射存储局部变量
func (e *IREmitter) emitStoreLocal(slot int) {
	if slot >= len(e.localOffsets) {
		e.errors = append(e.errors, fmt.Errorf("invalid local slot: %d", slot))
		return
	}

	offset := e.localOffsets[slot]

	// 从栈顶读取 Value (不弹出，因为赋值表达式返回值)
	e.cg.EmitMovRegMem(RAX, RSP, 0)  // typ
	e.cg.EmitMovRegMem(RBX, RSP, 8)  // num
	e.cg.EmitMovRegMem(RCX, RSP, 16) // ptr

	// 存储到局部变量
	e.cg.EmitMovMemReg(RBP, offset, RAX)
	e.cg.EmitMovMemReg(RBP, offset+8, RBX)
	e.cg.EmitMovMemReg(RBP, offset+16, RCX)
}

// emitLoadGlobal 发射加载全局变量
func (e *IREmitter) emitLoadGlobal(idx int) {
	// 通过 Helper 函数加载全局变量
	e.cg.EmitMovRegImm32(RAX, uint32(idx))
	e.emitCallHelper("LoadGlobal", 1)
	e.stackDepth++
}

// emitStoreGlobal 发射存储全局变量
func (e *IREmitter) emitStoreGlobal(idx int) {
	// 通过 Helper 函数存储全局变量
	e.cg.EmitMovRegImm32(RAX, uint32(idx))
	e.emitCallHelper("StoreGlobal", 2)
}

// ============================================================================
// 跳转指令
// ============================================================================

// emitJump 发射无条件跳转
func (e *IREmitter) emitJump(inst IRInst) {
	label := fmt.Sprintf("L%d", inst.Arg1)

	// 检查目标标签是否已知
	if offset, ok := e.labelOffsets[label]; ok {
		// 向后跳转 (已知位置)
		rel := int32(offset - (e.cg.CurrentOffset() + 5))
		e.cg.EmitJmp(rel)
	} else {
		// 向前跳转 (需要后续修补)
		pos := e.cg.EmitJmpLabel(label)
		e.pendingJumps = append(e.pendingJumps, pendingJump{
			codeOffset: pos,
			label:      label,
		})
	}
}

// emitJumpTrue 发射条件为真时跳转
func (e *IREmitter) emitJumpTrue(inst IRInst) {
	label := fmt.Sprintf("L%d", inst.Arg1)

	// 读取栈顶值
	e.cg.EmitMovRegMem(RAX, RSP, 8) // num field

	// 测试是否为真
	e.cg.EmitTestRegReg(RAX, RAX)

	// 记录跳转位置
	pos := e.cg.CurrentOffset()
	e.cg.EmitJne(0) // 占位符

	e.pendingJumps = append(e.pendingJumps, pendingJump{
		codeOffset: pos,
		label:      label,
	})
}

// emitJumpFalse 发射条件为假时跳转
func (e *IREmitter) emitJumpFalse(inst IRInst) {
	label := fmt.Sprintf("L%d", inst.Arg1)

	// 读取栈顶值
	e.cg.EmitMovRegMem(RAX, RSP, 8) // num field

	// 测试是否为假
	e.cg.EmitTestRegReg(RAX, RAX)

	// 记录跳转位置
	pos := e.cg.CurrentOffset()
	e.cg.EmitJe(0) // 占位符

	e.pendingJumps = append(e.pendingJumps, pendingJump{
		codeOffset: pos,
		label:      label,
	})
}

// emitLoop 发射循环跳转 (向后跳)
func (e *IREmitter) emitLoop(inst IRInst) {
	label := fmt.Sprintf("L%d", inst.Arg1)

	if offset, ok := e.labelOffsets[label]; ok {
		rel := int32(offset - (e.cg.CurrentOffset() + 5))
		e.cg.EmitJmp(rel)
	} else {
		e.errors = append(e.errors, fmt.Errorf("loop target not found: %s", label))
	}
}

// patchJumps 修补所有待定跳转
func (e *IREmitter) patchJumps() {
	for _, pj := range e.pendingJumps {
		if offset, ok := e.labelOffsets[pj.label]; ok {
			e.cg.PatchJump(pj.codeOffset, offset)
		} else {
			e.errors = append(e.errors, fmt.Errorf("jump target not found: %s", pj.label))
		}
	}
}

// ============================================================================
// 函数调用
// ============================================================================

// emitCall 发射函数调用
func (e *IREmitter) emitCall(argCount int) {
	// 通过 Helper 函数调用
	e.cg.EmitMovRegImm32(RAX, uint32(argCount))
	e.emitCallHelper("Call", argCount+1) // +1 for function object
	e.stackDepth -= argCount
}

// emitCallHelper 发射 Helper 函数调用
func (e *IREmitter) emitCallHelper(name string, argCount int) {
	addr := e.helperAddrs[name]
	if addr == 0 {
		// 如果 Helper 未注册，记录错误但继续生成代码
		e.errors = append(e.errors, fmt.Errorf("helper not found: %s", name))
		return
	}

	// 保存调用者保存的寄存器
	e.cg.EmitPush(RCX)
	e.cg.EmitPush(RDX)
	e.cg.EmitPush(RSI)
	e.cg.EmitPush(RDI)

	// 调用 Helper
	e.cg.EmitCallAbs(addr)

	// 恢复寄存器
	e.cg.EmitPop(RDI)
	e.cg.EmitPop(RSI)
	e.cg.EmitPop(RDX)
	e.cg.EmitPop(RCX)
}

// emitReturn 发射返回
func (e *IREmitter) emitReturn() {
	// 返回值已经在栈顶
	// 将栈顶的 Value 移动到 RAX, RBX, RCX (Go 返回约定)
	if e.stackDepth > 0 {
		e.cg.EmitMovRegMem(RAX, RSP, 0)  // typ
		e.cg.EmitMovRegMem(RBX, RSP, 8)  // num
		e.cg.EmitMovRegMem(RCX, RSP, 16) // ptr
	} else {
		// 返回 null
		e.cg.EmitXorRegReg(RAX, RAX)
		e.cg.EmitXorRegReg(RBX, RBX)
		e.cg.EmitXorRegReg(RCX, RCX)
	}

	// 跳转到函数尾声
	// (实际上这里应该跳转到恢复寄存器的位置，但为了简单，直接生成尾声)
	e.emitRestoreCalleeSaved()
	e.cg.EmitEpilogue()
}

// ============================================================================
// 复杂操作 (通过 Helper)
// ============================================================================

// emitObjectOp 发射对象操作
func (e *IREmitter) emitObjectOp(inst IRInst) {
	switch inst.Op {
	case IR_NEW_OBJECT:
		e.emitCallHelper("NewObject", 1)
	case IR_GET_FIELD:
		e.emitCallHelper("GetField", 2)
		e.stackDepth--
	case IR_SET_FIELD:
		e.emitCallHelper("SetField", 3)
		e.stackDepth -= 2
	case IR_INVOKE:
		e.emitCallHelper("Invoke", inst.Arg1+1)
		e.stackDepth -= inst.Arg1
	}
}

// emitArrayOp 发射数组操作
func (e *IREmitter) emitArrayOp(inst IRInst) {
	switch inst.Op {
	case IR_NEW_ARRAY:
		e.emitCallHelper("NewArray", inst.Arg1)
		e.stackDepth -= inst.Arg1 - 1
	case IR_ARRAY_GET:
		e.emitCallHelper("ArrayGet", 2)
		e.stackDepth--
	case IR_ARRAY_SET:
		e.emitCallHelper("ArraySet", 3)
		e.stackDepth -= 2
	case IR_ARRAY_LEN:
		e.emitCallHelper("ArrayLen", 1)
	}
}

// emitSuperArrayOp 发射 SuperArray 操作
func (e *IREmitter) emitSuperArrayOp(inst IRInst) {
	switch inst.Op {
	case IR_SA_NEW:
		e.emitCallHelper("SA_New", inst.Arg1)
		e.stackDepth -= inst.Arg1 - 1
	case IR_SA_GET:
		e.emitCallHelper("SA_Get", 2)
		e.stackDepth--
	case IR_SA_SET:
		e.emitCallHelper("SA_Set", 3)
		e.stackDepth -= 2
	case IR_SA_LEN:
		e.emitCallHelper("SA_Len", 1)
	case IR_SA_PUSH:
		e.emitCallHelper("SA_Push", 2)
		e.stackDepth--
	case IR_SA_HAS:
		e.emitCallHelper("SA_Has", 2)
		e.stackDepth--
	}
}

// ============================================================================
// 辅助函数
// ============================================================================

// floatBits 获取浮点数的位表示
func floatBits(f float64) uint64 {
	return *(*uint64)(unsafe.Pointer(&f))
}

// ============================================================================
// 简化的机器码生成入口
// ============================================================================

// GenerateMachineCode 从 IR 生成机器码
func GenerateMachineCode(fn *IRFunction, helpers map[string]uintptr) ([]byte, error) {
	emitter := NewIREmitter()

	// 注册 Helper 函数
	for name, addr := range helpers {
		emitter.SetHelperAddr(name, addr)
	}

	// 生成代码
	if err := emitter.EmitFunction(fn); err != nil {
		return nil, err
	}

	return emitter.Code(), nil
}

// GenerateMachineCodeFromInsts 从 IR 指令列表生成机器码 (简化版)
func GenerateMachineCodeFromInsts(ir []IRInst, localCount int, helpers map[string]uintptr) ([]byte, error) {
	// 创建一个简单的 IRFunction
	fn := &IRFunction{
		LocalCount: localCount,
		CFG: &CFG{
			Blocks: []*BasicBlock{
				{
					ID:    0,
					Name:  "entry",
					Insts: ir,
				},
			},
		},
	}

	return GenerateMachineCode(fn, helpers)
}
