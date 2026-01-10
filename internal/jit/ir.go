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
	TypeFunc                     // 函数指针
	TypeString                   // 字符串（指向字符串对象）
	TypeArray                    // 数组（指向数组对象）
	TypeObject                   // 对象引用
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
	case TypeFunc:
		return "func"
	case TypeString:
		return "string"
	case TypeArray:
		return "array"
	case TypeObject:
		return "object"
	default:
		return "unknown"
	}
}

// IsNumeric 检查是否为数值类型
func (t ValueType) IsNumeric() bool {
	return t == TypeInt || t == TypeFloat
}

// IsPointer 检查是否为指针类型（包括对象引用、数组等）
func (t ValueType) IsPointer() bool {
	return t == TypePtr || t == TypeObject || t == TypeArray || t == TypeString || t == TypeFunc
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
	
	// Upvalue 操作（闭包）
	OpLoadUpvalue  // 加载 upvalue（闭包捕获的变量）
	OpStoreUpvalue // 存储 upvalue

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

	// 函数调用
	OpCall         // 函数调用（通用）
	OpCallDirect   // 直接调用（编译时已知地址）
	OpCallIndirect // 间接调用（通过函数指针）
	OpCallBuiltin  // 内建函数调用
	OpCallMethod   // 方法调用
	OpCallVirtual  // 虚方法调用（通过虚表）
	OpTailCall     // 尾调用优化

	// 对象操作
	OpNewObject   // 创建对象
	OpGetField    // 读取字段
	OpSetField    // 写入字段
	OpGetFieldPtr // 获取字段指针（优化）
	OpLoadVTable  // 加载虚表

	// 类型转换
	OpIntToFloat // int -> float
	OpFloatToInt // float -> int
	OpBoolToInt  // bool -> int

	// 数组操作
	OpArrayGet         // 数组取元素
	OpArraySet         // 数组设元素
	OpArrayLen         // 数组长度
	OpArrayBoundsCheck // 数组边界检查（可被优化消除）

	// 标记指令
	OpNop // 空操作（占位符，优化后可能产生）
	
	// 异常处理
	OpExceptionFallback // 触发异常回退到解释器
)

var opcodeNames = map[Opcode]string{
	OpConst:        "const",
	OpLoadLocal:    "load",
	OpStoreLocal:   "store",
	OpLoadUpvalue:  "loadup",
	OpStoreUpvalue: "storeup",
	OpAdd:         "add",
	OpSub:         "sub",
	OpMul:         "mul",
	OpDiv:         "div",
	OpMod:         "mod",
	OpNeg:         "neg",
	OpEq:          "eq",
	OpNe:          "ne",
	OpLt:          "lt",
	OpLe:          "le",
	OpGt:          "gt",
	OpGe:          "ge",
	OpNot:         "not",
	OpAnd:         "and",
	OpOr:          "or",
	OpBitAnd:      "band",
	OpBitOr:       "bor",
	OpBitXor:      "bxor",
	OpBitNot:      "bnot",
	OpShl:         "shl",
	OpShr:         "shr",
	OpJump:        "jump",
	OpBranch:      "branch",
	OpReturn:      "return",
	OpPhi:         "phi",
	OpCall:        "call",
	OpCallDirect:  "call.direct",
	OpCallIndirect: "call.indirect",
	OpCallBuiltin: "call.builtin",
	OpCallMethod:  "call.method",
	OpCallVirtual: "call.virtual",
	OpTailCall:    "tailcall",
	OpNewObject:   "newobj",
	OpGetField:    "getfield",
	OpSetField:    "setfield",
	OpGetFieldPtr: "getfieldptr",
	OpLoadVTable:  "loadvtable",
	OpIntToFloat:  "i2f",
	OpFloatToInt:  "f2i",
	OpBoolToInt:   "b2i",
	OpArrayGet:         "aget",
	OpArraySet:         "aset",
	OpArrayLen:         "alen",
	OpArrayBoundsCheck: "boundscheck",
	OpNop:               "nop",
	OpExceptionFallback: "exception.fallback",
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
	
	// 是否是异常回退标记值
	IsFallback bool

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

// CallConvType 调用约定类型
type CallConvType int

const (
	CallConvDefault  CallConvType = iota // 默认调用约定
	CallConvSola                         // Sola 内部调用约定
	CallConvC                            // C 调用约定
	CallConvFast                         // 快速调用约定（尽可能用寄存器）
)

// IRInstr IR 指令
type IRInstr struct {
	Op     Opcode     // 操作码
	Dest   *IRValue   // 目标值（可能为 nil）
	Args   []*IRValue // 操作数列表
	Block  *IRBlock   // 所属基本块
	
	// 额外信息
	LocalIdx int        // 局部变量索引（用于 load/store）
	Targets  []*IRBlock // 跳转目标（用于 branch）
	
	// 函数调用相关字段
	CallTarget   string        // 调用目标（函数名或符号）
	CallArgCount int           // 调用参数数量
	CallConv     CallConvType  // 调用约定
	CallFunc     *IRFunc       // 被调用的IR函数（用于内联）
	IsVarArgs    bool          // 是否可变参数
	
	// 对象操作相关字段
	ClassName    string        // 类名
	FieldName    string        // 字段名
	FieldOffset  int           // 字段偏移（编译时计算）
	FieldType    ValueType     // 字段类型
	
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
	sb.WriteString(fmt.Sprintf("%-14s", instr.Op.String()))
	
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
	case OpCall, OpCallDirect, OpCallIndirect, OpCallBuiltin, OpTailCall:
		sb.WriteString(fmt.Sprintf(" %s(", instr.CallTarget))
		for i, arg := range instr.Args {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(arg.String())
		}
		sb.WriteString(")")
	case OpCallMethod, OpCallVirtual:
		if len(instr.Args) > 0 {
			sb.WriteString(fmt.Sprintf(" %s.%s(", instr.Args[0].String(), instr.CallTarget))
			for i := 1; i < len(instr.Args); i++ {
				if i > 1 {
					sb.WriteString(", ")
				}
				sb.WriteString(instr.Args[i].String())
			}
			sb.WriteString(")")
		}
	case OpNewObject:
		sb.WriteString(fmt.Sprintf(" %s", instr.ClassName))
	case OpGetField, OpSetField:
		if len(instr.Args) > 0 {
			sb.WriteString(fmt.Sprintf(" %s.%s", instr.Args[0].String(), instr.FieldName))
			if instr.FieldOffset >= 0 {
				sb.WriteString(fmt.Sprintf("[+%d]", instr.FieldOffset))
			}
		}
		if instr.Op == OpSetField && len(instr.Args) > 1 {
			sb.WriteString(fmt.Sprintf(" = %s", instr.Args[1].String()))
		}
	case OpGetFieldPtr:
		if len(instr.Args) > 0 {
			sb.WriteString(fmt.Sprintf(" &%s.%s", instr.Args[0].String(), instr.FieldName))
		}
	case OpLoadVTable:
		if len(instr.Args) > 0 {
			sb.WriteString(fmt.Sprintf(" %s", instr.Args[0].String()))
		}
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
	return instr.Op == OpJump || instr.Op == OpBranch || instr.Op == OpReturn || instr.Op == OpExceptionFallback
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

// ============================================================================
// 函数调用指令构造
// ============================================================================

// NewCallInstr 创建函数调用指令
//
// 参数：
//   - target: 被调用的函数名
//   - dest: 返回值（可为nil表示无返回值）
//   - args: 参数列表
//
// 返回：
//   - 调用指令
func NewCallInstr(target string, dest *IRValue, args ...*IRValue) *IRInstr {
	instr := NewInstr(OpCall, dest, args...)
	instr.CallTarget = target
	instr.CallArgCount = len(args)
	instr.CallConv = CallConvSola
	return instr
}

// NewCallDirectInstr 创建直接调用指令（编译时地址已知）
func NewCallDirectInstr(target string, dest *IRValue, args ...*IRValue) *IRInstr {
	instr := NewInstr(OpCallDirect, dest, args...)
	instr.CallTarget = target
	instr.CallArgCount = len(args)
	instr.CallConv = CallConvSola
	return instr
}

// NewCallIndirectInstr 创建间接调用指令（通过函数指针）
func NewCallIndirectInstr(funcPtr *IRValue, dest *IRValue, args ...*IRValue) *IRInstr {
	allArgs := make([]*IRValue, 0, len(args)+1)
	allArgs = append(allArgs, funcPtr)
	allArgs = append(allArgs, args...)
	
	instr := NewInstr(OpCallIndirect, dest, allArgs...)
	instr.CallArgCount = len(args)
	instr.CallConv = CallConvSola
	return instr
}

// NewCallBuiltinInstr 创建内建函数调用指令
func NewCallBuiltinInstr(builtinName string, dest *IRValue, args ...*IRValue) *IRInstr {
	instr := NewInstr(OpCallBuiltin, dest, args...)
	instr.CallTarget = builtinName
	instr.CallArgCount = len(args)
	instr.CallConv = CallConvC
	return instr
}

// NewCallMethodInstr 创建方法调用指令
//
// 参数：
//   - receiver: 接收者对象
//   - methodName: 方法名
//   - dest: 返回值
//   - args: 参数列表（不包括接收者）
func NewCallMethodInstr(receiver *IRValue, methodName string, dest *IRValue, args ...*IRValue) *IRInstr {
	allArgs := make([]*IRValue, 0, len(args)+1)
	allArgs = append(allArgs, receiver)
	allArgs = append(allArgs, args...)
	
	instr := NewInstr(OpCallMethod, dest, allArgs...)
	instr.CallTarget = methodName
	instr.CallArgCount = len(args)
	instr.CallConv = CallConvSola
	return instr
}

// NewCallVirtualInstr 创建虚方法调用指令
func NewCallVirtualInstr(receiver *IRValue, methodName string, dest *IRValue, args ...*IRValue) *IRInstr {
	allArgs := make([]*IRValue, 0, len(args)+1)
	allArgs = append(allArgs, receiver)
	allArgs = append(allArgs, args...)
	
	instr := NewInstr(OpCallVirtual, dest, allArgs...)
	instr.CallTarget = methodName
	instr.CallArgCount = len(args)
	instr.CallConv = CallConvSola
	return instr
}

// NewTailCallInstr 创建尾调用指令
func NewTailCallInstr(target string, args ...*IRValue) *IRInstr {
	instr := NewInstr(OpTailCall, nil, args...)
	instr.CallTarget = target
	instr.CallArgCount = len(args)
	instr.CallConv = CallConvSola
	return instr
}

// ============================================================================
// 对象操作指令构造
// ============================================================================

// NewNewObjectInstr 创建对象创建指令
func NewNewObjectInstr(className string, dest *IRValue) *IRInstr {
	instr := NewInstr(OpNewObject, dest)
	instr.ClassName = className
	return instr
}

// NewGetFieldInstr 创建字段读取指令
//
// 参数：
//   - obj: 对象引用
//   - fieldName: 字段名
//   - dest: 结果值
//   - fieldOffset: 字段偏移（-1表示运行时查找）
func NewGetFieldInstr(obj *IRValue, fieldName string, dest *IRValue, fieldOffset int) *IRInstr {
	instr := NewInstr(OpGetField, dest, obj)
	instr.FieldName = fieldName
	instr.FieldOffset = fieldOffset
	return instr
}

// NewSetFieldInstr 创建字段写入指令
//
// 参数：
//   - obj: 对象引用
//   - fieldName: 字段名
//   - value: 要写入的值
//   - fieldOffset: 字段偏移（-1表示运行时查找）
func NewSetFieldInstr(obj *IRValue, fieldName string, value *IRValue, fieldOffset int) *IRInstr {
	instr := NewInstr(OpSetField, nil, obj, value)
	instr.FieldName = fieldName
	instr.FieldOffset = fieldOffset
	return instr
}

// NewGetFieldPtrInstr 创建获取字段指针指令（用于优化）
func NewGetFieldPtrInstr(obj *IRValue, fieldName string, dest *IRValue, fieldOffset int) *IRInstr {
	instr := NewInstr(OpGetFieldPtr, dest, obj)
	instr.FieldName = fieldName
	instr.FieldOffset = fieldOffset
	return instr
}

// NewLoadVTableInstr 创建加载虚表指令
func NewLoadVTableInstr(obj *IRValue, dest *IRValue) *IRInstr {
	return NewInstr(OpLoadVTable, dest, obj)
}

// ============================================================================
// 指令属性查询
// ============================================================================

// IsCall 检查是否是调用指令
func (instr *IRInstr) IsCall() bool {
	switch instr.Op {
	case OpCall, OpCallDirect, OpCallIndirect, OpCallBuiltin, OpCallMethod, OpCallVirtual, OpTailCall:
		return true
	}
	return false
}

// IsObjectOp 检查是否是对象操作指令
func (instr *IRInstr) IsObjectOp() bool {
	switch instr.Op {
	case OpNewObject, OpGetField, OpSetField, OpGetFieldPtr, OpLoadVTable:
		return true
	}
	return false
}

// HasSideEffects 检查指令是否有副作用
func (instr *IRInstr) HasSideEffects() bool {
	switch instr.Op {
	case OpStoreLocal, OpSetField, OpArraySet,
		OpCall, OpCallDirect, OpCallIndirect, OpCallBuiltin, OpCallMethod, OpCallVirtual, OpTailCall,
		OpNewObject, OpReturn, OpJump, OpBranch:
		return true
	}
	return false
}

// CanThrow 检查指令是否可能抛出异常
func (instr *IRInstr) CanThrow() bool {
	switch instr.Op {
	case OpDiv, OpMod, // 除零异常
		OpArrayGet, OpArraySet, // 数组越界
		OpGetField, OpSetField, // 空指针
		OpCall, OpCallDirect, OpCallIndirect, OpCallBuiltin, OpCallMethod, OpCallVirtual: // 调用可能抛异常
		return true
	}
	return false
}
