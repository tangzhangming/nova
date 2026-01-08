// ir.go - SSA 形式的中间表示 (Intermediate Representation)
//
// 本文件定义了 JIT 编译器使用的中间表示。采用 SSA (Static Single Assignment) 形式，
// 即每个变量只被赋值一次。这种形式便于进行数据流分析和优化。
//
// SSA 的优点：
// 1. 每个变量只有一个定义点，简化了 use-def 分析
// 2. 便于检测死代码和无用变量
// 3. 简化了常量传播和复写传播
// 4. 使控制流分析更加精确
//
// IR 结构：
// - IRFunc: 函数
// - IRBlock: 基本块（控制流图的节点）
// - IRInstr: 指令（操作的具体表示）
// - IRValue: 值（指令的操作数和结果）

package jit

import (
	"fmt"
	"strings"

	"github.com/tangzhangming/nova/internal/bytecode"
)

// ============================================================================
// IR 值类型
// ============================================================================

// ValueType IR 值的类型
type ValueType int

const (
	TypeVoid    ValueType = iota // 无类型（用于没有返回值的指令）
	TypeInt                      // 整数
	TypeFloat                    // 浮点数
	TypeBool                     // 布尔值
	TypePtr                      // 指针（用于对象引用）
	TypeUnknown                  // 未知类型（在类型推断前使用）
)

func (t ValueType) String() string {
	switch t {
	case TypeVoid:
		return "void"
	case TypeInt:
		return "int"
	case TypeFloat:
		return "float"
	case TypeBool:
		return "bool"
	case TypePtr:
		return "ptr"
	default:
		return "unknown"
	}
}

// ============================================================================
// IR 操作码
// ============================================================================

// Opcode IR 操作码
type Opcode int

const (
	// 常量加载
	OpConst Opcode = iota // 加载常量

	// 局部变量操作
	OpLoadLocal  // 加载局部变量
	OpStoreLocal // 存储局部变量

	// 算术运算
	OpAdd // 加法
	OpSub // 减法
	OpMul // 乘法
	OpDiv // 除法
	OpMod // 取模
	OpNeg // 取负

	// 比较运算
	OpEq  // 等于
	OpNe  // 不等于
	OpLt  // 小于
	OpLe  // 小于等于
	OpGt  // 大于
	OpGe  // 大于等于

	// 逻辑运算
	OpNot // 逻辑非
	OpAnd // 逻辑与
	OpOr  // 逻辑或

	// 位运算
	OpBitAnd // 位与
	OpBitOr  // 位或
	OpBitXor // 位异或
	OpBitNot // 位非
	OpShl    // 左移
	OpShr    // 右移

	// 控制流
	OpJump     // 无条件跳转
	OpBranch   // 条件跳转 (if cond goto then else goto else)
	OpReturn   // 返回
	OpPhi      // SSA Phi 函数

	// 函数调用（暂时回退到解释器）
	OpCall       // 函数调用
	OpCallMethod // 方法调用

	// 类型转换
	OpIntToFloat // int -> float
	OpFloatToInt // float -> int
	OpBoolToInt  // bool -> int

	// 数组操作
	OpArrayGet // 数组取元素
	OpArraySet // 数组设元素
	OpArrayLen // 数组长度

	// 标记指令
	OpNop // 空操作（占位符，优化后可能产生）
)

var opcodeNames = map[Opcode]string{
	OpConst:      "const",
	OpLoadLocal:  "load",
	OpStoreLocal: "store",
	OpAdd:        "add",
	OpSub:        "sub",
	OpMul:        "mul",
	OpDiv:        "div",
	OpMod:        "mod",
	OpNeg:        "neg",
	OpEq:         "eq",
	OpNe:         "ne",
	OpLt:         "lt",
	OpLe:         "le",
	OpGt:         "gt",
	OpGe:         "ge",
	OpNot:        "not",
	OpAnd:        "and",
	OpOr:         "or",
	OpBitAnd:     "band",
	OpBitOr:      "bor",
	OpBitXor:     "bxor",
	OpBitNot:     "bnot",
	OpShl:        "shl",
	OpShr:        "shr",
	OpJump:       "jump",
	OpBranch:     "branch",
	OpReturn:     "return",
	OpPhi:        "phi",
	OpCall:       "call",
	OpCallMethod: "callmethod",
	OpIntToFloat: "i2f",
	OpFloatToInt: "f2i",
	OpBoolToInt:  "b2i",
	OpArrayGet:   "aget",
	OpArraySet:   "aset",
	OpArrayLen:   "alen",
	OpNop:        "nop",
}

func (op Opcode) String() string {
	if name, ok := opcodeNames[op]; ok {
		return name
	}
	return fmt.Sprintf("op%d", op)
}

// ============================================================================
// IR 值
// ============================================================================

// IRValue IR 值
// 表示指令的操作数或结果
type IRValue struct {
	ID   int       // 唯一标识符（SSA 中每个值都有唯一 ID）
	Type ValueType // 值的类型
	
	// 如果是常量，这里存储常量值
	IsConst   bool
	ConstVal  bytecode.Value
	
	// 定义此值的指令（如果不是常量）
	Def       *IRInstr
	
	// 使用此值的所有指令（use-def 链）
	Uses      []*IRInstr
}

// NewValue 创建新的 IR 值
func NewValue(id int, typ ValueType) *IRValue {
	return &IRValue{
		ID:   id,
		Type: typ,
	}
}

// NewConstInt 创建整数常量
func NewConstInt(id int, val int64) *IRValue {
	return &IRValue{
		ID:       id,
		Type:     TypeInt,
		IsConst:  true,
		ConstVal: bytecode.NewInt(val),
	}
}

// NewConstFloat 创建浮点常量
func NewConstFloat(id int, val float64) *IRValue {
	return &IRValue{
		ID:       id,
		Type:     TypeFloat,
		IsConst:  true,
		ConstVal: bytecode.NewFloat(val),
	}
}

// NewConstBool 创建布尔常量
func NewConstBool(id int, val bool) *IRValue {
	return &IRValue{
		ID:       id,
		Type:     TypeBool,
		IsConst:  true,
		ConstVal: bytecode.NewBool(val),
	}
}

func (v *IRValue) String() string {
	if v == nil {
		return "nil"
	}
	if v.IsConst {
		return fmt.Sprintf("$%s", v.ConstVal.String())
	}
	return fmt.Sprintf("v%d", v.ID)
}

// AddUse 添加使用此值的指令
func (v *IRValue) AddUse(instr *IRInstr) {
	v.Uses = append(v.Uses, instr)
}

// RemoveUse 移除使用此值的指令
func (v *IRValue) RemoveUse(instr *IRInstr) {
	for i, u := range v.Uses {
		if u == instr {
			v.Uses = append(v.Uses[:i], v.Uses[i+1:]...)
			return
		}
	}
}

// HasUses 检查是否有指令使用此值
func (v *IRValue) HasUses() bool {
	return len(v.Uses) > 0
}

// ============================================================================
// IR 指令
// ============================================================================

// IRInstr IR 指令
type IRInstr struct {
	Op     Opcode     // 操作码
	Dest   *IRValue   // 目标值（可能为 nil）
	Args   []*IRValue // 操作数列表
	Block  *IRBlock   // 所属基本块
	
	// 额外信息
	LocalIdx int        // 局部变量索引（用于 load/store）
	Targets  []*IRBlock // 跳转目标（用于 branch）
	
	// 调试信息
	Line     int        // 源代码行号
}

// NewInstr 创建新指令
func NewInstr(op Opcode, dest *IRValue, args ...*IRValue) *IRInstr {
	instr := &IRInstr{
		Op:   op,
		Dest: dest,
		Args: args,
	}
	
	// 建立 def-use 关系
	if dest != nil {
		dest.Def = instr
	}
	for _, arg := range args {
		if arg != nil {
			arg.AddUse(instr)
		}
	}
	
	return instr
}

func (instr *IRInstr) String() string {
	var sb strings.Builder
	
	// 目标
	if instr.Dest != nil {
		sb.WriteString(fmt.Sprintf("%-6s = ", instr.Dest.String()))
	} else {
		sb.WriteString("         ")
	}
	
	// 操作码
	sb.WriteString(fmt.Sprintf("%-10s", instr.Op.String()))
	
	// 参数
	switch instr.Op {
	case OpLoadLocal, OpStoreLocal:
		sb.WriteString(fmt.Sprintf(" local[%d]", instr.LocalIdx))
		for _, arg := range instr.Args {
			sb.WriteString(", ")
			sb.WriteString(arg.String())
		}
	case OpJump:
		if len(instr.Targets) > 0 {
			sb.WriteString(fmt.Sprintf(" -> bb%d", instr.Targets[0].ID))
		}
	case OpBranch:
		if len(instr.Args) > 0 {
			sb.WriteString(fmt.Sprintf(" %s", instr.Args[0].String()))
		}
		if len(instr.Targets) >= 2 {
			sb.WriteString(fmt.Sprintf(", then bb%d, else bb%d", 
				instr.Targets[0].ID, instr.Targets[1].ID))
		}
	case OpReturn:
		if len(instr.Args) > 0 {
			sb.WriteString(fmt.Sprintf(" %s", instr.Args[0].String()))
		}
	case OpPhi:
		sb.WriteString(" [")
		for i := 0; i < len(instr.Args); i++ {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(instr.Args[i].String())
			if i < len(instr.Targets) {
				sb.WriteString(fmt.Sprintf(":bb%d", instr.Targets[i].ID))
			}
		}
		sb.WriteString("]")
	default:
		for i, arg := range instr.Args {
			if i > 0 {
				sb.WriteString(",")
			}
			sb.WriteString(" ")
			sb.WriteString(arg.String())
		}
	}
	
	return sb.String()
}

// IsBranch 检查是否是分支指令
func (instr *IRInstr) IsBranch() bool {
	return instr.Op == OpJump || instr.Op == OpBranch || instr.Op == OpReturn
}

// IsTerminator 检查是否是终止指令
func (instr *IRInstr) IsTerminator() bool {
	return instr.IsBranch()
}

// ReplaceArg 替换操作数
func (instr *IRInstr) ReplaceArg(old, new *IRValue) {
	for i, arg := range instr.Args {
		if arg == old {
			old.RemoveUse(instr)
			instr.Args[i] = new
			new.AddUse(instr)
		}
	}
}

// ============================================================================
// IR 基本块
// ============================================================================

// IRBlock 基本块
// 基本块是控制流图的节点，包含一系列顺序执行的指令，
// 以一个终止指令（跳转、分支或返回）结束
type IRBlock struct {
	ID      int          // 块标识符
	Instrs  []*IRInstr   // 指令列表
	Preds   []*IRBlock   // 前驱块
	Succs   []*IRBlock   // 后继块
	
	// 循环信息
	LoopDepth int        // 循环嵌套深度
	
	// 支配信息（用于 SSA 构建和优化）
	IDom      *IRBlock   // 直接支配者
	DomFront  []*IRBlock // 支配边界
	
	// 活跃性分析结果
	LiveIn    map[int]bool // 块入口活跃的值
	LiveOut   map[int]bool // 块出口活跃的值
}

// NewBlock 创建新的基本块
func NewBlock(id int) *IRBlock {
	return &IRBlock{
		ID:       id,
		LiveIn:   make(map[int]bool),
		LiveOut:  make(map[int]bool),
	}
}

// AddInstr 添加指令到块末尾
func (b *IRBlock) AddInstr(instr *IRInstr) {
	instr.Block = b
	b.Instrs = append(b.Instrs, instr)
}

// InsertInstrBefore 在终止指令前插入指令
func (b *IRBlock) InsertInstrBefore(instr *IRInstr) {
	instr.Block = b
	if len(b.Instrs) == 0 {
		b.Instrs = append(b.Instrs, instr)
		return
	}
	
	// 找到终止指令
	lastIdx := len(b.Instrs) - 1
	if b.Instrs[lastIdx].IsTerminator() {
		// 在终止指令前插入
		b.Instrs = append(b.Instrs[:lastIdx], append([]*IRInstr{instr}, b.Instrs[lastIdx:]...)...)
	} else {
		b.Instrs = append(b.Instrs, instr)
	}
}

// RemoveInstr 移除指令
func (b *IRBlock) RemoveInstr(instr *IRInstr) {
	for i, inst := range b.Instrs {
		if inst == instr {
			b.Instrs = append(b.Instrs[:i], b.Instrs[i+1:]...)
			return
		}
	}
}

// LastInstr 获取最后一条指令
func (b *IRBlock) LastInstr() *IRInstr {
	if len(b.Instrs) == 0 {
		return nil
	}
	return b.Instrs[len(b.Instrs)-1]
}

// AddSucc 添加后继块
func (b *IRBlock) AddSucc(succ *IRBlock) {
	b.Succs = append(b.Succs, succ)
	succ.Preds = append(succ.Preds, b)
}

func (b *IRBlock) String() string {
	var sb strings.Builder
	
	// 块头
	sb.WriteString(fmt.Sprintf("bb%d:", b.ID))
	
	// 前驱
	if len(b.Preds) > 0 {
		sb.WriteString(" ; preds = ")
		for i, pred := range b.Preds {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(fmt.Sprintf("bb%d", pred.ID))
		}
	}
	sb.WriteString("\n")
	
	// 指令
	for _, instr := range b.Instrs {
		sb.WriteString("    ")
		sb.WriteString(instr.String())
		sb.WriteString("\n")
	}
	
	return sb.String()
}

// ============================================================================
// IR 函数
// ============================================================================

// IRFunc IR 函数
type IRFunc struct {
	Name       string       // 函数名
	NumArgs    int          // 参数数量
	Entry      *IRBlock     // 入口块
	Blocks     []*IRBlock   // 所有基本块
	Values     []*IRValue   // 所有值
	
	// 源函数信息
	SourceFunc *bytecode.Function
	Constants  []bytecode.Value // 常量池
	LocalCount int              // 局部变量数量
	
	// ID 计数器
	nextValueID int
	nextBlockID int
}

// NewIRFunc 创建新的 IR 函数
func NewIRFunc(name string, numArgs int) *IRFunc {
	fn := &IRFunc{
		Name:    name,
		NumArgs: numArgs,
	}
	
	// 创建入口块
	fn.Entry = fn.NewBlock()
	
	return fn
}

// NewBlock 创建新的基本块
func (fn *IRFunc) NewBlock() *IRBlock {
	block := NewBlock(fn.nextBlockID)
	fn.nextBlockID++
	fn.Blocks = append(fn.Blocks, block)
	return block
}

// NewValue 创建新的值
func (fn *IRFunc) NewValue(typ ValueType) *IRValue {
	v := NewValue(fn.nextValueID, typ)
	fn.nextValueID++
	fn.Values = append(fn.Values, v)
	return v
}

// NewConstIntValue 创建整数常量值
func (fn *IRFunc) NewConstIntValue(val int64) *IRValue {
	v := NewConstInt(fn.nextValueID, val)
	fn.nextValueID++
	fn.Values = append(fn.Values, v)
	return v
}

// NewConstFloatValue 创建浮点常量值
func (fn *IRFunc) NewConstFloatValue(val float64) *IRValue {
	v := NewConstFloat(fn.nextValueID, val)
	fn.nextValueID++
	fn.Values = append(fn.Values, v)
	return v
}

// NewConstBoolValue 创建布尔常量值
func (fn *IRFunc) NewConstBoolValue(val bool) *IRValue {
	v := NewConstBool(fn.nextValueID, val)
	fn.nextValueID++
	fn.Values = append(fn.Values, v)
	return v
}

func (fn *IRFunc) String() string {
	var sb strings.Builder
	
	sb.WriteString(fmt.Sprintf("func %s(args: %d, locals: %d) {\n", 
		fn.Name, fn.NumArgs, fn.LocalCount))
	
	for _, block := range fn.Blocks {
		sb.WriteString(block.String())
	}
	
	sb.WriteString("}\n")
	return sb.String()
}

// ComputeBlockOrder 计算基本块的逆后序遍历顺序
// 这个顺序对于很多数据流分析算法是必需的
func (fn *IRFunc) ComputeBlockOrder() []*IRBlock {
	visited := make(map[*IRBlock]bool)
	order := make([]*IRBlock, 0, len(fn.Blocks))
	
	var postorder func(*IRBlock)
	postorder = func(b *IRBlock) {
		if visited[b] {
			return
		}
		visited[b] = true
		for _, succ := range b.Succs {
			postorder(succ)
		}
		order = append(order, b)
	}
	
	postorder(fn.Entry)
	
	// 反转得到逆后序
	for i, j := 0, len(order)-1; i < j; i, j = i+1, j-1 {
		order[i], order[j] = order[j], order[i]
	}
	
	return order
}
