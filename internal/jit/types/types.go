// Package types 定义 JIT 共享类型接口，用于避免循环导入
package types

import "github.com/tangzhangming/nova/internal/bytecode"

// ============================================================================
// IR 类型接口定义（用于避免循环导入）
// ============================================================================

// IROpCode IR 操作码
type IROpCode int

const (
	// 内存操作
	IRLoadLocal IROpCode = iota
	IRStoreLocal
	IRLoadGlobal
	IRStoreGlobal
	IRLoadField
	IRStoreField
	IRLoadConst

	// 算术运算
	IRAdd
	IRSub
	IRMul
	IRDiv
	IRMod
	IRNeg

	// 比较运算
	IREq
	IRNe
	IRLt
	IRLe
	IRGt
	IRGe

	// 逻辑运算
	IRAnd
	IROr
	IRNot

	// 位运算
	IRBitAnd
	IRBitOr
	IRBitXor
	IRBitNot
	IRShl
	IRShr

	// 控制流
	IRBranch
	IRBranchIf
	IRReturn
	IRCall
	IRCallMethod
	IRPhi

	// 对象操作
	IRNewObject
	IRNewArray
	IRArrayGet
	IRArraySet

	// 类型操作
	IRTypeGuard
	IRTypeCast
)

// TypeKind 类型种类
type TypeKind int

const (
	TypeVoid TypeKind = iota
	TypeInt
	TypeFloat
	TypeBool
	TypeString
	TypeObject
	TypeArray
	TypePointer
)

// IRType IR 类型
type IRType struct {
	Kind     TypeKind
	TypeName string
}

// IRInstr IR 指令
type IRInstr struct {
	Op        IROpCode
	Dest      int
	Args      []int
	Type      IRType
	Immediate interface{}
	TypeName  string
}

// IRBlock IR 基本块
type IRBlock struct {
	ID       int
	Instrs   []*IRInstr
	Succs    []*IRBlock
	Preds    []*IRBlock
	PhiNodes []*IRInstr
	Entry    bool
	Exit     bool
}

// IRFunction IR 函数
type IRFunction struct {
	Name       string
	Entry      *IRBlock
	Blocks     []*IRBlock
	NumVRegs   int
	Constants  []bytecode.Value
	LocalCount int
}

// LiveInterval 活跃区间
type LiveInterval struct {
	VReg  int
	Start int
	End   int
	PReg  int
	Spill int
}

// RegisterAllocation 寄存器分配结果
type RegisterAllocation struct {
	Allocated     map[int]int // vreg -> preg
	Spilled       map[int]int // vreg -> stack slot
	StackSize     int
	LiveIntervals []*LiveInterval
}

// CodeGenerator 代码生成器接口
type CodeGenerator interface {
	GenerateFunction(fn *IRFunction, regAlloc *RegisterAllocation) ([]byte, error)
}

