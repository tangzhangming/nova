package jit

import (
	"fmt"
	"strings"

	"github.com/tangzhangming/nova/internal/bytecode"
)

// ============================================================================
// IR 指令定义
// ============================================================================

// IROp IR 操作码
type IROp int

const (
	// 栈操作
	IR_NOP IROp = iota
	IR_CONST
	IR_POP
	IR_DUP

	// 算术运算
	IR_ADD
	IR_SUB
	IR_MUL
	IR_DIV
	IR_MOD
	IR_NEG

	// 位运算
	IR_BAND
	IR_BOR
	IR_BXOR
	IR_BNOT
	IR_SHL
	IR_SHR

	// 比较运算
	IR_EQ
	IR_NE
	IR_LT
	IR_LE
	IR_GT
	IR_GE

	// 逻辑运算
	IR_NOT
	IR_AND
	IR_OR

	// 变量操作
	IR_LOAD_LOCAL
	IR_STORE_LOCAL
	IR_LOAD_GLOBAL
	IR_STORE_GLOBAL

	// 跳转
	IR_JUMP
	IR_JUMP_TRUE
	IR_JUMP_FALSE
	IR_LOOP

	// 函数调用
	IR_CALL
	IR_CALL_HELPER
	IR_RETURN

	// 对象操作
	IR_NEW_OBJECT
	IR_GET_FIELD
	IR_SET_FIELD
	IR_INVOKE

	// 数组操作
	IR_NEW_ARRAY
	IR_ARRAY_GET
	IR_ARRAY_SET
	IR_ARRAY_LEN

	// SuperArray 操作
	IR_SA_NEW
	IR_SA_GET
	IR_SA_SET
	IR_SA_LEN
	IR_SA_PUSH
	IR_SA_HAS

	// 类型操作
	IR_TYPE_CHECK
	IR_TYPE_CAST

	// 标签
	IR_LABEL
)

// IRInst IR 指令
type IRInst struct {
	Op         IROp           // 操作码
	Value      bytecode.Value // 常量值 (用于 IR_CONST)
	Arg1       int            // 第一个参数
	Arg2       int            // 第二个参数
	HelperName string         // Helper 函数名 (用于 IR_CALL_HELPER)
	Label      string         // 标签名 (用于 IR_LABEL)
	BytecodeIP int            // 对应的字节码 IP (调试用)
}

// ============================================================================
// IR 字符串表示
// ============================================================================

// String 返回 IR 操作码的字符串表示
func (op IROp) String() string {
	switch op {
	case IR_NOP:
		return "NOP"
	case IR_CONST:
		return "CONST"
	case IR_POP:
		return "POP"
	case IR_DUP:
		return "DUP"
	case IR_ADD:
		return "ADD"
	case IR_SUB:
		return "SUB"
	case IR_MUL:
		return "MUL"
	case IR_DIV:
		return "DIV"
	case IR_MOD:
		return "MOD"
	case IR_NEG:
		return "NEG"
	case IR_BAND:
		return "BAND"
	case IR_BOR:
		return "BOR"
	case IR_BXOR:
		return "BXOR"
	case IR_BNOT:
		return "BNOT"
	case IR_SHL:
		return "SHL"
	case IR_SHR:
		return "SHR"
	case IR_EQ:
		return "EQ"
	case IR_NE:
		return "NE"
	case IR_LT:
		return "LT"
	case IR_LE:
		return "LE"
	case IR_GT:
		return "GT"
	case IR_GE:
		return "GE"
	case IR_NOT:
		return "NOT"
	case IR_AND:
		return "AND"
	case IR_OR:
		return "OR"
	case IR_LOAD_LOCAL:
		return "LOAD_LOCAL"
	case IR_STORE_LOCAL:
		return "STORE_LOCAL"
	case IR_LOAD_GLOBAL:
		return "LOAD_GLOBAL"
	case IR_STORE_GLOBAL:
		return "STORE_GLOBAL"
	case IR_JUMP:
		return "JUMP"
	case IR_JUMP_TRUE:
		return "JUMP_TRUE"
	case IR_JUMP_FALSE:
		return "JUMP_FALSE"
	case IR_LOOP:
		return "LOOP"
	case IR_CALL:
		return "CALL"
	case IR_CALL_HELPER:
		return "CALL_HELPER"
	case IR_RETURN:
		return "RETURN"
	case IR_NEW_OBJECT:
		return "NEW_OBJECT"
	case IR_GET_FIELD:
		return "GET_FIELD"
	case IR_SET_FIELD:
		return "SET_FIELD"
	case IR_INVOKE:
		return "INVOKE"
	case IR_NEW_ARRAY:
		return "NEW_ARRAY"
	case IR_ARRAY_GET:
		return "ARRAY_GET"
	case IR_ARRAY_SET:
		return "ARRAY_SET"
	case IR_ARRAY_LEN:
		return "ARRAY_LEN"
	case IR_SA_NEW:
		return "SA_NEW"
	case IR_SA_GET:
		return "SA_GET"
	case IR_SA_SET:
		return "SA_SET"
	case IR_SA_LEN:
		return "SA_LEN"
	case IR_SA_PUSH:
		return "SA_PUSH"
	case IR_SA_HAS:
		return "SA_HAS"
	case IR_TYPE_CHECK:
		return "TYPE_CHECK"
	case IR_TYPE_CAST:
		return "TYPE_CAST"
	case IR_LABEL:
		return "LABEL"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", op)
	}
}

// String 返回 IR 指令的字符串表示
func (inst IRInst) String() string {
	var sb strings.Builder
	sb.WriteString(inst.Op.String())

	switch inst.Op {
	case IR_CONST:
		sb.WriteString(fmt.Sprintf(" %v", inst.Value))
	case IR_LOAD_LOCAL, IR_STORE_LOCAL:
		sb.WriteString(fmt.Sprintf(" slot=%d", inst.Arg1))
	case IR_LOAD_GLOBAL, IR_STORE_GLOBAL:
		sb.WriteString(fmt.Sprintf(" idx=%d", inst.Arg1))
	case IR_JUMP, IR_JUMP_TRUE, IR_JUMP_FALSE, IR_LOOP:
		sb.WriteString(fmt.Sprintf(" offset=%d", inst.Arg1))
	case IR_CALL:
		sb.WriteString(fmt.Sprintf(" argc=%d", inst.Arg1))
	case IR_CALL_HELPER:
		sb.WriteString(fmt.Sprintf(" %s argc=%d", inst.HelperName, inst.Arg1))
	case IR_LABEL:
		sb.WriteString(fmt.Sprintf(" %s", inst.Label))
	}

	if inst.BytecodeIP > 0 {
		sb.WriteString(fmt.Sprintf(" ; bc@%d", inst.BytecodeIP))
	}

	return sb.String()
}

// ============================================================================
// IR 辅助函数
// ============================================================================

// IsTerminator 检查指令是否是终止指令
func (inst IRInst) IsTerminator() bool {
	switch inst.Op {
	case IR_RETURN, IR_JUMP:
		return true
	default:
		return false
	}
}

// IsJump 检查指令是否是跳转指令
func (inst IRInst) IsJump() bool {
	switch inst.Op {
	case IR_JUMP, IR_JUMP_TRUE, IR_JUMP_FALSE, IR_LOOP:
		return true
	default:
		return false
	}
}

// IsBinaryOp 检查指令是否是二元操作
func (inst IRInst) IsBinaryOp() bool {
	switch inst.Op {
	case IR_ADD, IR_SUB, IR_MUL, IR_DIV, IR_MOD,
		IR_BAND, IR_BOR, IR_BXOR, IR_SHL, IR_SHR,
		IR_EQ, IR_NE, IR_LT, IR_LE, IR_GT, IR_GE,
		IR_AND, IR_OR:
		return true
	default:
		return false
	}
}

// IsUnaryOp 检查指令是否是一元操作
func (inst IRInst) IsUnaryOp() bool {
	switch inst.Op {
	case IR_NEG, IR_BNOT, IR_NOT:
		return true
	default:
		return false
	}
}

// ============================================================================
// IR 构建器
// ============================================================================

// IRBuilder IR 构建器
type IRBuilder struct {
	insts      []IRInst
	labelCount int
}

// NewIRBuilder 创建 IR 构建器
func NewIRBuilder() *IRBuilder {
	return &IRBuilder{
		insts: make([]IRInst, 0, 64),
	}
}

// Emit 发射指令
func (b *IRBuilder) Emit(inst IRInst) {
	b.insts = append(b.insts, inst)
}

// EmitConst 发射常量加载
func (b *IRBuilder) EmitConst(v bytecode.Value) {
	b.Emit(IRInst{Op: IR_CONST, Value: v})
}

// EmitBinaryOp 发射二元操作
func (b *IRBuilder) EmitBinaryOp(op IROp) {
	b.Emit(IRInst{Op: op})
}

// EmitUnaryOp 发射一元操作
func (b *IRBuilder) EmitUnaryOp(op IROp) {
	b.Emit(IRInst{Op: op})
}

// EmitLoadLocal 发射加载局部变量
func (b *IRBuilder) EmitLoadLocal(slot int) {
	b.Emit(IRInst{Op: IR_LOAD_LOCAL, Arg1: slot})
}

// EmitStoreLocal 发射存储局部变量
func (b *IRBuilder) EmitStoreLocal(slot int) {
	b.Emit(IRInst{Op: IR_STORE_LOCAL, Arg1: slot})
}

// EmitJump 发射跳转
func (b *IRBuilder) EmitJump(offset int) {
	b.Emit(IRInst{Op: IR_JUMP, Arg1: offset})
}

// EmitJumpIfFalse 发射条件跳转
func (b *IRBuilder) EmitJumpIfFalse(offset int) {
	b.Emit(IRInst{Op: IR_JUMP_FALSE, Arg1: offset})
}

// EmitCall 发射函数调用
func (b *IRBuilder) EmitCall(argCount int) {
	b.Emit(IRInst{Op: IR_CALL, Arg1: argCount})
}

// EmitCallHelper 发射 Helper 调用
func (b *IRBuilder) EmitCallHelper(name string, argCount int) {
	b.Emit(IRInst{Op: IR_CALL_HELPER, HelperName: name, Arg1: argCount})
}

// EmitReturn 发射返回
func (b *IRBuilder) EmitReturn() {
	b.Emit(IRInst{Op: IR_RETURN})
}

// EmitLabel 发射标签
func (b *IRBuilder) EmitLabel(label string) {
	b.Emit(IRInst{Op: IR_LABEL, Label: label})
}

// NewLabel 创建新标签
func (b *IRBuilder) NewLabel() string {
	label := fmt.Sprintf("L%d", b.labelCount)
	b.labelCount++
	return label
}

// Build 构建 IR
func (b *IRBuilder) Build() []IRInst {
	return b.insts
}

// Clear 清空构建器
func (b *IRBuilder) Clear() {
	b.insts = b.insts[:0]
	b.labelCount = 0
}

// ============================================================================
// IR 打印器
// ============================================================================

// PrintIR 打印 IR
func PrintIR(ir []IRInst) string {
	var sb strings.Builder
	for i, inst := range ir {
		sb.WriteString(fmt.Sprintf("%4d: %s\n", i, inst.String()))
	}
	return sb.String()
}

// PrintIRToConsole 打印 IR 到控制台
func PrintIRToConsole(ir []IRInst) {
	fmt.Print(PrintIR(ir))
}
