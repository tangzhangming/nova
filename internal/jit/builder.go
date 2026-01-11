package jit

import (
	"fmt"

	"github.com/tangzhangming/nova/internal/bytecode"
)

// ============================================================================
// 基本块和控制流图
// ============================================================================

// BasicBlock 基本块
type BasicBlock struct {
	ID          int        // 块 ID
	Name        string     // 块名称
	Insts       []IRInst   // 指令列表
	Preds       []*BasicBlock // 前驱块
	Succs       []*BasicBlock // 后继块
	StartIP     int        // 起始字节码 IP
	EndIP       int        // 结束字节码 IP
	IsEntry     bool       // 是否是入口块
	IsExit      bool       // 是否是出口块
	LiveIn      []int      // 入口活跃变量
	LiveOut     []int      // 出口活跃变量
	DomTree     *BasicBlock // 支配树父节点
	LoopDepth   int        // 循环嵌套深度
}

// NewBasicBlock 创建基本块
func NewBasicBlock(id int, name string) *BasicBlock {
	return &BasicBlock{
		ID:     id,
		Name:   name,
		Insts:  make([]IRInst, 0),
		Preds:  make([]*BasicBlock, 0),
		Succs:  make([]*BasicBlock, 0),
	}
}

// AddInst 添加指令
func (bb *BasicBlock) AddInst(inst IRInst) {
	bb.Insts = append(bb.Insts, inst)
}

// AddPred 添加前驱
func (bb *BasicBlock) AddPred(pred *BasicBlock) {
	bb.Preds = append(bb.Preds, pred)
}

// AddSucc 添加后继
func (bb *BasicBlock) AddSucc(succ *BasicBlock) {
	bb.Succs = append(bb.Succs, succ)
}

// IsTerminated 检查块是否已终止
func (bb *BasicBlock) IsTerminated() bool {
	if len(bb.Insts) == 0 {
		return false
	}
	last := bb.Insts[len(bb.Insts)-1]
	return last.IsTerminator()
}

// String 返回块的字符串表示
func (bb *BasicBlock) String() string {
	return fmt.Sprintf("BB%d(%s)", bb.ID, bb.Name)
}

// ============================================================================
// 控制流图
// ============================================================================

// CFG 控制流图
type CFG struct {
	Blocks     []*BasicBlock        // 所有基本块
	Entry      *BasicBlock          // 入口块
	Exit       *BasicBlock          // 出口块
	BlockMap   map[int]*BasicBlock  // IP -> 块映射
	Function   *bytecode.Function   // 关联的函数
}

// NewCFG 创建控制流图
func NewCFG(fn *bytecode.Function) *CFG {
	return &CFG{
		Blocks:   make([]*BasicBlock, 0),
		BlockMap: make(map[int]*BasicBlock),
		Function: fn,
	}
}

// AddBlock 添加基本块
func (cfg *CFG) AddBlock(bb *BasicBlock) {
	cfg.Blocks = append(cfg.Blocks, bb)
}

// GetBlock 获取指定 IP 的块
func (cfg *CFG) GetBlock(ip int) *BasicBlock {
	return cfg.BlockMap[ip]
}

// Connect 连接两个块
func (cfg *CFG) Connect(from, to *BasicBlock) {
	from.AddSucc(to)
	to.AddPred(from)
}

// ============================================================================
// IR 函数表示
// ============================================================================

// IRFunction IR 层的函数表示
type IRFunction struct {
	Name        string
	CFG         *CFG
	LocalCount  int
	ArgCount    int
	Constants   []bytecode.Value
	Registers   int  // 使用的寄存器数量
}

// NewIRFunction 创建 IR 函数
func NewIRFunction(name string) *IRFunction {
	return &IRFunction{
		Name:      name,
		Constants: make([]bytecode.Value, 0),
	}
}

// ============================================================================
// IR Builder
// ============================================================================

// FunctionBuilder 函数构建器
type FunctionBuilder struct {
	fn          *IRFunction
	cfg         *CFG
	currentBB   *BasicBlock
	blockCount  int
	valueCount  int
	
	// 字节码 -> IR 映射
	bcToBlock   map[int]*BasicBlock
	bcToValue   map[int]int  // bytecode IP -> value ID
	
	// 临时存储
	stack       []int  // 值栈
}

// NewFunctionBuilder 创建函数构建器
func NewFunctionBuilder(fn *bytecode.Function) *FunctionBuilder {
	irFn := NewIRFunction(fn.Name)
	cfg := NewCFG(fn)
	irFn.CFG = cfg
	
	return &FunctionBuilder{
		fn:        irFn,
		cfg:       cfg,
		bcToBlock: make(map[int]*BasicBlock),
		bcToValue: make(map[int]int),
		stack:     make([]int, 0),
	}
}

// ============================================================================
// 块操作
// ============================================================================

// CreateBlock 创建基本块
func (b *FunctionBuilder) CreateBlock(name string) *BasicBlock {
	bb := NewBasicBlock(b.blockCount, name)
	b.blockCount++
	b.cfg.AddBlock(bb)
	return bb
}

// CreateEntryBlock 创建入口块
func (b *FunctionBuilder) CreateEntryBlock() *BasicBlock {
	bb := b.CreateBlock("entry")
	bb.IsEntry = true
	b.cfg.Entry = bb
	return bb
}

// CreateExitBlock 创建出口块
func (b *FunctionBuilder) CreateExitBlock() *BasicBlock {
	bb := b.CreateBlock("exit")
	bb.IsExit = true
	b.cfg.Exit = bb
	return bb
}

// SetInsertPoint 设置插入点
func (b *FunctionBuilder) SetInsertPoint(bb *BasicBlock) {
	b.currentBB = bb
}

// CurrentBlock 获取当前块
func (b *FunctionBuilder) CurrentBlock() *BasicBlock {
	return b.currentBB
}

// ============================================================================
// 指令创建
// ============================================================================

// emit 发射指令到当前块
func (b *FunctionBuilder) emit(inst IRInst) int {
	if b.currentBB == nil {
		panic("no current block")
	}
	b.currentBB.AddInst(inst)
	return len(b.currentBB.Insts) - 1
}

// CreateConst 创建常量加载
func (b *FunctionBuilder) CreateConst(v bytecode.Value) int {
	id := b.valueCount
	b.valueCount++
	b.emit(IRInst{Op: IR_CONST, Value: v, Arg1: id})
	return id
}

// CreateAdd 创建加法
func (b *FunctionBuilder) CreateAdd(lhs, rhs int) int {
	id := b.valueCount
	b.valueCount++
	b.emit(IRInst{Op: IR_ADD, Arg1: lhs, Arg2: rhs})
	return id
}

// CreateSub 创建减法
func (b *FunctionBuilder) CreateSub(lhs, rhs int) int {
	id := b.valueCount
	b.valueCount++
	b.emit(IRInst{Op: IR_SUB, Arg1: lhs, Arg2: rhs})
	return id
}

// CreateMul 创建乘法
func (b *FunctionBuilder) CreateMul(lhs, rhs int) int {
	id := b.valueCount
	b.valueCount++
	b.emit(IRInst{Op: IR_MUL, Arg1: lhs, Arg2: rhs})
	return id
}

// CreateDiv 创建除法
func (b *FunctionBuilder) CreateDiv(lhs, rhs int) int {
	id := b.valueCount
	b.valueCount++
	b.emit(IRInst{Op: IR_DIV, Arg1: lhs, Arg2: rhs})
	return id
}

// CreateMod 创建取模
func (b *FunctionBuilder) CreateMod(lhs, rhs int) int {
	id := b.valueCount
	b.valueCount++
	b.emit(IRInst{Op: IR_MOD, Arg1: lhs, Arg2: rhs})
	return id
}

// CreateNeg 创建取负
func (b *FunctionBuilder) CreateNeg(val int) int {
	id := b.valueCount
	b.valueCount++
	b.emit(IRInst{Op: IR_NEG, Arg1: val})
	return id
}

// ============================================================================
// 比较指令
// ============================================================================

// CreateEq 创建相等比较
func (b *FunctionBuilder) CreateEq(lhs, rhs int) int {
	id := b.valueCount
	b.valueCount++
	b.emit(IRInst{Op: IR_EQ, Arg1: lhs, Arg2: rhs})
	return id
}

// CreateNe 创建不等比较
func (b *FunctionBuilder) CreateNe(lhs, rhs int) int {
	id := b.valueCount
	b.valueCount++
	b.emit(IRInst{Op: IR_NE, Arg1: lhs, Arg2: rhs})
	return id
}

// CreateLt 创建小于比较
func (b *FunctionBuilder) CreateLt(lhs, rhs int) int {
	id := b.valueCount
	b.valueCount++
	b.emit(IRInst{Op: IR_LT, Arg1: lhs, Arg2: rhs})
	return id
}

// CreateLe 创建小于等于比较
func (b *FunctionBuilder) CreateLe(lhs, rhs int) int {
	id := b.valueCount
	b.valueCount++
	b.emit(IRInst{Op: IR_LE, Arg1: lhs, Arg2: rhs})
	return id
}

// CreateGt 创建大于比较
func (b *FunctionBuilder) CreateGt(lhs, rhs int) int {
	id := b.valueCount
	b.valueCount++
	b.emit(IRInst{Op: IR_GT, Arg1: lhs, Arg2: rhs})
	return id
}

// CreateGe 创建大于等于比较
func (b *FunctionBuilder) CreateGe(lhs, rhs int) int {
	id := b.valueCount
	b.valueCount++
	b.emit(IRInst{Op: IR_GE, Arg1: lhs, Arg2: rhs})
	return id
}

// ============================================================================
// 逻辑指令
// ============================================================================

// CreateNot 创建逻辑非
func (b *FunctionBuilder) CreateNot(val int) int {
	id := b.valueCount
	b.valueCount++
	b.emit(IRInst{Op: IR_NOT, Arg1: val})
	return id
}

// CreateAnd 创建逻辑与
func (b *FunctionBuilder) CreateAnd(lhs, rhs int) int {
	id := b.valueCount
	b.valueCount++
	b.emit(IRInst{Op: IR_AND, Arg1: lhs, Arg2: rhs})
	return id
}

// CreateOr 创建逻辑或
func (b *FunctionBuilder) CreateOr(lhs, rhs int) int {
	id := b.valueCount
	b.valueCount++
	b.emit(IRInst{Op: IR_OR, Arg1: lhs, Arg2: rhs})
	return id
}

// ============================================================================
// 位运算指令
// ============================================================================

// CreateBand 创建按位与
func (b *FunctionBuilder) CreateBand(lhs, rhs int) int {
	id := b.valueCount
	b.valueCount++
	b.emit(IRInst{Op: IR_BAND, Arg1: lhs, Arg2: rhs})
	return id
}

// CreateBor 创建按位或
func (b *FunctionBuilder) CreateBor(lhs, rhs int) int {
	id := b.valueCount
	b.valueCount++
	b.emit(IRInst{Op: IR_BOR, Arg1: lhs, Arg2: rhs})
	return id
}

// CreateBxor 创建按位异或
func (b *FunctionBuilder) CreateBxor(lhs, rhs int) int {
	id := b.valueCount
	b.valueCount++
	b.emit(IRInst{Op: IR_BXOR, Arg1: lhs, Arg2: rhs})
	return id
}

// CreateBnot 创建按位取反
func (b *FunctionBuilder) CreateBnot(val int) int {
	id := b.valueCount
	b.valueCount++
	b.emit(IRInst{Op: IR_BNOT, Arg1: val})
	return id
}

// CreateShl 创建左移
func (b *FunctionBuilder) CreateShl(lhs, rhs int) int {
	id := b.valueCount
	b.valueCount++
	b.emit(IRInst{Op: IR_SHL, Arg1: lhs, Arg2: rhs})
	return id
}

// CreateShr 创建右移
func (b *FunctionBuilder) CreateShr(lhs, rhs int) int {
	id := b.valueCount
	b.valueCount++
	b.emit(IRInst{Op: IR_SHR, Arg1: lhs, Arg2: rhs})
	return id
}

// ============================================================================
// 变量操作
// ============================================================================

// CreateLoadLocal 创建加载局部变量
func (b *FunctionBuilder) CreateLoadLocal(slot int) int {
	id := b.valueCount
	b.valueCount++
	b.emit(IRInst{Op: IR_LOAD_LOCAL, Arg1: slot})
	return id
}

// CreateStoreLocal 创建存储局部变量
func (b *FunctionBuilder) CreateStoreLocal(slot int, val int) {
	b.emit(IRInst{Op: IR_STORE_LOCAL, Arg1: slot, Arg2: val})
}

// CreateLoadGlobal 创建加载全局变量
func (b *FunctionBuilder) CreateLoadGlobal(index int) int {
	id := b.valueCount
	b.valueCount++
	b.emit(IRInst{Op: IR_LOAD_GLOBAL, Arg1: index})
	return id
}

// CreateStoreGlobal 创建存储全局变量
func (b *FunctionBuilder) CreateStoreGlobal(index int, val int) {
	b.emit(IRInst{Op: IR_STORE_GLOBAL, Arg1: index, Arg2: val})
}

// ============================================================================
// 控制流
// ============================================================================

// CreateJump 创建无条件跳转
func (b *FunctionBuilder) CreateJump(target *BasicBlock) {
	b.emit(IRInst{Op: IR_JUMP, Label: target.Name})
	b.cfg.Connect(b.currentBB, target)
}

// CreateCondJump 创建条件跳转
func (b *FunctionBuilder) CreateCondJump(cond int, trueBB, falseBB *BasicBlock) {
	b.emit(IRInst{Op: IR_JUMP_TRUE, Arg1: cond, Label: trueBB.Name})
	b.cfg.Connect(b.currentBB, trueBB)
	b.cfg.Connect(b.currentBB, falseBB)
}

// CreateReturn 创建返回
func (b *FunctionBuilder) CreateReturn(val int) {
	b.emit(IRInst{Op: IR_RETURN, Arg1: val})
}

// CreateReturnVoid 创建无返回值返回
func (b *FunctionBuilder) CreateReturnVoid() {
	b.emit(IRInst{Op: IR_RETURN, Arg1: -1})
}

// ============================================================================
// 函数调用
// ============================================================================

// CreateCall 创建函数调用
func (b *FunctionBuilder) CreateCall(argCount int) int {
	id := b.valueCount
	b.valueCount++
	b.emit(IRInst{Op: IR_CALL, Arg1: argCount})
	return id
}

// CreateHelperCall 创建 Helper 调用
func (b *FunctionBuilder) CreateHelperCall(name string, argCount int) int {
	id := b.valueCount
	b.valueCount++
	b.emit(IRInst{Op: IR_CALL_HELPER, HelperName: name, Arg1: argCount})
	return id
}

// ============================================================================
// SuperArray 操作
// ============================================================================

// CreateSANew 创建 SuperArray
func (b *FunctionBuilder) CreateSANew() int {
	id := b.valueCount
	b.valueCount++
	b.emit(IRInst{Op: IR_SA_NEW})
	return id
}

// CreateSAGet 获取 SuperArray 元素
func (b *FunctionBuilder) CreateSAGet(arr, key int) int {
	id := b.valueCount
	b.valueCount++
	b.emit(IRInst{Op: IR_SA_GET, Arg1: arr, Arg2: key})
	return id
}

// CreateSASet 设置 SuperArray 元素
func (b *FunctionBuilder) CreateSASet(arr, key, val int) {
	b.emit(IRInst{Op: IR_SA_SET, Arg1: arr, Arg2: key})
}

// CreateSALen 获取 SuperArray 长度
func (b *FunctionBuilder) CreateSALen(arr int) int {
	id := b.valueCount
	b.valueCount++
	b.emit(IRInst{Op: IR_SA_LEN, Arg1: arr})
	return id
}

// ============================================================================
// 构建完成
// ============================================================================

// Build 完成构建并返回 IR 函数
func (b *FunctionBuilder) Build() *IRFunction {
	b.fn.Registers = b.valueCount
	return b.fn
}

// ============================================================================
// 字节码转 IR (增强版)
// ============================================================================

// BuildFromBytecode 从字节码构建 IR
func (b *FunctionBuilder) BuildFromBytecode(fn *bytecode.Function) *IRFunction {
	if fn.Chunk == nil {
		return b.fn
	}

	// 1. 识别基本块边界
	leaders := b.findBlockLeaders(fn)
	
	// 2. 创建基本块
	b.createBlocks(fn, leaders)
	
	// 3. 转换每个基本块的指令
	b.translateBlocks(fn)
	
	// 4. 构建控制流边
	b.buildCFGEdges()
	
	return b.Build()
}

// findBlockLeaders 找出所有基本块的起始位置
func (b *FunctionBuilder) findBlockLeaders(fn *bytecode.Function) map[int]bool {
	leaders := make(map[int]bool)
	leaders[0] = true // 函数入口
	
	code := fn.Chunk.Code
	ip := 0
	
	for ip < len(code) {
		op := bytecode.OpCode(code[ip])
		ip++
		
		switch op {
		case bytecode.OpJump, bytecode.OpLoop:
			if ip+1 < len(code) {
				offset := int(code[ip])<<8 | int(code[ip+1])
				ip += 2
				target := ip + offset
				if op == bytecode.OpLoop {
					target = ip - offset
				}
				leaders[target] = true
				leaders[ip] = true // 跳转后的指令
			}
			
		case bytecode.OpJumpIfFalse, bytecode.OpJumpIfTrue:
			if ip+1 < len(code) {
				offset := int(code[ip])<<8 | int(code[ip+1])
				ip += 2
				target := ip + offset
				leaders[target] = true
				leaders[ip] = true
			}
			
		case bytecode.OpReturn:
			if ip < len(code) {
				leaders[ip] = true
			}
			
		default:
			// 跳过操作数
			ip += getOpcodeOperandSize(op)
		}
	}
	
	return leaders
}

// getOpcodeOperandSize 获取操作码的操作数大小
func getOpcodeOperandSize(op bytecode.OpCode) int {
	switch op {
	case bytecode.OpPush, bytecode.OpLoadLocal, bytecode.OpStoreLocal,
		bytecode.OpLoadGlobal, bytecode.OpStoreGlobal,
		bytecode.OpNewArray, bytecode.OpSuperArrayNew:
		return 2
	case bytecode.OpCall:
		return 1
	default:
		return 0
	}
}

// createBlocks 创建基本块
func (b *FunctionBuilder) createBlocks(fn *bytecode.Function, leaders map[int]bool) {
	// 排序 leaders
	var sorted []int
	for ip := range leaders {
		sorted = append(sorted, ip)
	}
	
	// 简单排序
	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	
	// 创建块
	for i, ip := range sorted {
		name := fmt.Sprintf("bb%d", i)
		if ip == 0 {
			name = "entry"
		}
		bb := b.CreateBlock(name)
		bb.StartIP = ip
		if i+1 < len(sorted) {
			bb.EndIP = sorted[i+1]
		} else {
			bb.EndIP = len(fn.Chunk.Code)
		}
		b.bcToBlock[ip] = bb
	}
	
	// 设置入口块
	if bb := b.bcToBlock[0]; bb != nil {
		bb.IsEntry = true
		b.cfg.Entry = bb
	}
}

// translateBlocks 转换每个基本块
func (b *FunctionBuilder) translateBlocks(fn *bytecode.Function) {
	for _, bb := range b.cfg.Blocks {
		b.SetInsertPoint(bb)
		b.translateBlock(fn, bb)
	}
}

// translateBlock 转换单个基本块
func (b *FunctionBuilder) translateBlock(fn *bytecode.Function, bb *BasicBlock) {
	code := fn.Chunk.Code
	consts := fn.Chunk.Constants
	ip := bb.StartIP
	
	for ip < bb.EndIP && ip < len(code) {
		op := bytecode.OpCode(code[ip])
		startIP := ip
		ip++
		
		switch op {
		case bytecode.OpPush:
			if ip+1 < len(code) {
				constIdx := int(code[ip])<<8 | int(code[ip+1])
				ip += 2
				if constIdx < len(consts) {
					b.CreateConst(consts[constIdx])
				}
			}
			
		case bytecode.OpNull:
			b.CreateConst(bytecode.NullValue)
			
		case bytecode.OpTrue:
			b.CreateConst(bytecode.TrueValue)
			
		case bytecode.OpFalse:
			b.CreateConst(bytecode.FalseValue)
			
		case bytecode.OpZero:
			b.CreateConst(bytecode.ZeroValue)
			
		case bytecode.OpOne:
			b.CreateConst(bytecode.OneValue)
			
		case bytecode.OpAdd:
			b.emit(IRInst{Op: IR_ADD, BytecodeIP: startIP})
			
		case bytecode.OpSub:
			b.emit(IRInst{Op: IR_SUB, BytecodeIP: startIP})
			
		case bytecode.OpMul:
			b.emit(IRInst{Op: IR_MUL, BytecodeIP: startIP})
			
		case bytecode.OpDiv:
			b.emit(IRInst{Op: IR_DIV, BytecodeIP: startIP})
			
		case bytecode.OpMod:
			b.emit(IRInst{Op: IR_MOD, BytecodeIP: startIP})
			
		case bytecode.OpNeg:
			b.emit(IRInst{Op: IR_NEG, BytecodeIP: startIP})
			
		case bytecode.OpEq:
			b.emit(IRInst{Op: IR_EQ, BytecodeIP: startIP})
			
		case bytecode.OpNe:
			b.emit(IRInst{Op: IR_NE, BytecodeIP: startIP})
			
		case bytecode.OpLt:
			b.emit(IRInst{Op: IR_LT, BytecodeIP: startIP})
			
		case bytecode.OpLe:
			b.emit(IRInst{Op: IR_LE, BytecodeIP: startIP})
			
		case bytecode.OpGt:
			b.emit(IRInst{Op: IR_GT, BytecodeIP: startIP})
			
		case bytecode.OpGe:
			b.emit(IRInst{Op: IR_GE, BytecodeIP: startIP})
			
		case bytecode.OpNot:
			b.emit(IRInst{Op: IR_NOT, BytecodeIP: startIP})
			
		case bytecode.OpLoadLocal:
			if ip+1 < len(code) {
				slot := int(code[ip])<<8 | int(code[ip+1])
				ip += 2
				b.emit(IRInst{Op: IR_LOAD_LOCAL, Arg1: slot, BytecodeIP: startIP})
			}
			
		case bytecode.OpStoreLocal:
			if ip+1 < len(code) {
				slot := int(code[ip])<<8 | int(code[ip+1])
				ip += 2
				b.emit(IRInst{Op: IR_STORE_LOCAL, Arg1: slot, BytecodeIP: startIP})
			}
			
		case bytecode.OpJump:
			if ip+1 < len(code) {
				offset := int(code[ip])<<8 | int(code[ip+1])
				ip += 2
				b.emit(IRInst{Op: IR_JUMP, Arg1: offset, BytecodeIP: startIP})
			}
			
		case bytecode.OpJumpIfFalse:
			if ip+1 < len(code) {
				offset := int(code[ip])<<8 | int(code[ip+1])
				ip += 2
				b.emit(IRInst{Op: IR_JUMP_FALSE, Arg1: offset, BytecodeIP: startIP})
			}
			
		case bytecode.OpJumpIfTrue:
			if ip+1 < len(code) {
				offset := int(code[ip])<<8 | int(code[ip+1])
				ip += 2
				b.emit(IRInst{Op: IR_JUMP_TRUE, Arg1: offset, BytecodeIP: startIP})
			}
			
		case bytecode.OpLoop:
			if ip+1 < len(code) {
				offset := int(code[ip])<<8 | int(code[ip+1])
				ip += 2
				b.emit(IRInst{Op: IR_LOOP, Arg1: offset, BytecodeIP: startIP})
			}
			
		case bytecode.OpCall:
			argCount := int(code[ip])
			ip++
			b.emit(IRInst{Op: IR_CALL, Arg1: argCount, BytecodeIP: startIP})
			
		case bytecode.OpReturn:
			b.emit(IRInst{Op: IR_RETURN, BytecodeIP: startIP})
			
		case bytecode.OpSuperArrayNew:
			if ip+1 < len(code) {
				count := int(code[ip])<<8 | int(code[ip+1])
				ip += 2
				b.emit(IRInst{Op: IR_CALL_HELPER, HelperName: "SA_New", Arg1: count, BytecodeIP: startIP})
			}
			
		case bytecode.OpSuperArrayGet:
			b.emit(IRInst{Op: IR_CALL_HELPER, HelperName: "SA_Get", BytecodeIP: startIP})
			
		case bytecode.OpSuperArraySet:
			b.emit(IRInst{Op: IR_CALL_HELPER, HelperName: "SA_Set", BytecodeIP: startIP})
			
		default:
			// 未处理的操作码
		}
	}
}

// buildCFGEdges 构建控制流边
func (b *FunctionBuilder) buildCFGEdges() {
	for _, bb := range b.cfg.Blocks {
		if len(bb.Insts) == 0 {
			continue
		}
		
		last := bb.Insts[len(bb.Insts)-1]
		
		switch last.Op {
		case IR_JUMP, IR_LOOP:
			// 无条件跳转
			target := bb.EndIP + last.Arg1
			if last.Op == IR_LOOP {
				target = bb.EndIP - last.Arg1
			}
			if targetBB := b.bcToBlock[target]; targetBB != nil {
				b.cfg.Connect(bb, targetBB)
			}
			
		case IR_JUMP_FALSE, IR_JUMP_TRUE:
			// 条件跳转
			target := bb.EndIP + last.Arg1
			if targetBB := b.bcToBlock[target]; targetBB != nil {
				b.cfg.Connect(bb, targetBB)
			}
			// 落入下一个块
			if nextBB := b.bcToBlock[bb.EndIP]; nextBB != nil {
				b.cfg.Connect(bb, nextBB)
			}
			
		case IR_RETURN:
			// 返回指令，连接到出口
			if b.cfg.Exit != nil {
				b.cfg.Connect(bb, b.cfg.Exit)
			}
			
		default:
			// 落入下一个块
			if nextBB := b.bcToBlock[bb.EndIP]; nextBB != nil {
				b.cfg.Connect(bb, nextBB)
			}
		}
	}
}
