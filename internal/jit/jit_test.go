package jit

import (
	"testing"

	"github.com/tangzhangming/nova/internal/bytecode"
)

// ============================================================================
// IR 构建测试
// ============================================================================

func TestIRBuilder_CreateConst(t *testing.T) {
	fn := &bytecode.Function{Name: "test"}
	builder := NewFunctionBuilder(fn)

	entry := builder.CreateEntryBlock()
	builder.SetInsertPoint(entry)

	// 创建常量
	v1 := builder.CreateConst(bytecode.NewInt(42))
	v2 := builder.CreateConst(bytecode.NewFloat(3.14))
	v3 := builder.CreateConst(bytecode.NewString("hello"))

	if v1 != 0 {
		t.Errorf("first value ID should be 0, got %d", v1)
	}
	if v2 != 1 {
		t.Errorf("second value ID should be 1, got %d", v2)
	}
	if v3 != 2 {
		t.Errorf("third value ID should be 2, got %d", v3)
	}

	irFn := builder.Build()
	if irFn.Registers != 3 {
		t.Errorf("expected 3 registers, got %d", irFn.Registers)
	}
}

func TestIRBuilder_CreateArithmetic(t *testing.T) {
	fn := &bytecode.Function{Name: "test"}
	builder := NewFunctionBuilder(fn)

	entry := builder.CreateEntryBlock()
	builder.SetInsertPoint(entry)

	a := builder.CreateConst(bytecode.NewInt(10))
	b := builder.CreateConst(bytecode.NewInt(20))

	sum := builder.CreateAdd(a, b)
	diff := builder.CreateSub(a, b)
	prod := builder.CreateMul(a, b)

	if sum != 2 || diff != 3 || prod != 4 {
		t.Errorf("unexpected value IDs: sum=%d, diff=%d, prod=%d", sum, diff, prod)
	}

	irFn := builder.Build()
	if len(irFn.CFG.Blocks) != 1 {
		t.Errorf("expected 1 block, got %d", len(irFn.CFG.Blocks))
	}
}

func TestIRBuilder_ControlFlow(t *testing.T) {
	fn := &bytecode.Function{Name: "test"}
	builder := NewFunctionBuilder(fn)

	entry := builder.CreateEntryBlock()
	trueBB := builder.CreateBlock("then")
	falseBB := builder.CreateBlock("else")
	mergeBB := builder.CreateBlock("merge")

	builder.SetInsertPoint(entry)
	cond := builder.CreateConst(bytecode.TrueValue)
	builder.CreateCondJump(cond, trueBB, falseBB)

	builder.SetInsertPoint(trueBB)
	builder.CreateJump(mergeBB)

	builder.SetInsertPoint(falseBB)
	builder.CreateJump(mergeBB)

	builder.SetInsertPoint(mergeBB)
	builder.CreateReturnVoid()

	irFn := builder.Build()

	if len(irFn.CFG.Blocks) != 4 {
		t.Errorf("expected 4 blocks, got %d", len(irFn.CFG.Blocks))
	}

	// 检查边
	if len(entry.Succs) != 2 {
		t.Errorf("entry should have 2 successors, got %d", len(entry.Succs))
	}
	if len(mergeBB.Preds) != 2 {
		t.Errorf("merge should have 2 predecessors, got %d", len(mergeBB.Preds))
	}
}

// ============================================================================
// 优化 Pass 测试
// ============================================================================

func TestConstantFolding(t *testing.T) {
	pass := NewConstantFoldingPass()

	// 创建测试块
	bb := NewBasicBlock(0, "test")
	bb.AddInst(IRInst{Op: IR_CONST, Value: bytecode.NewInt(10)})
	bb.AddInst(IRInst{Op: IR_CONST, Value: bytecode.NewInt(20)})
	bb.AddInst(IRInst{Op: IR_ADD})

	// 运行优化
	changed := pass.runOnBlock(bb)

	if !changed {
		t.Error("expected constant folding to change the block")
	}

	// 应该只剩一条 CONST 30 指令
	if len(bb.Insts) != 1 {
		t.Errorf("expected 1 instruction, got %d", len(bb.Insts))
	}

	if bb.Insts[0].Op != IR_CONST {
		t.Errorf("expected CONST, got %v", bb.Insts[0].Op)
	}

	if bb.Insts[0].Value.AsInt() != 30 {
		t.Errorf("expected 30, got %d", bb.Insts[0].Value.AsInt())
	}
}

func TestDeadCodeElimination(t *testing.T) {
	pass := NewDeadCodeEliminationPass()

	fn := NewIRFunction("test")
	fn.CFG = NewCFG(nil)

	// 创建带死代码的块
	bb := NewBasicBlock(0, "test")
	bb.AddInst(IRInst{Op: IR_CONST, Value: bytecode.NewInt(1)})
	bb.AddInst(IRInst{Op: IR_RETURN})
	bb.AddInst(IRInst{Op: IR_CONST, Value: bytecode.NewInt(2)}) // 死代码
	bb.AddInst(IRInst{Op: IR_ADD})                               // 死代码

	fn.CFG.AddBlock(bb)
	fn.CFG.Entry = bb

	changed := pass.Run(fn)

	if !changed {
		t.Error("expected DCE to change the function")
	}

	if len(bb.Insts) != 2 {
		t.Errorf("expected 2 instructions, got %d", len(bb.Insts))
	}
}

func TestStrengthReduction(t *testing.T) {
	pass := NewStrengthReductionPass()

	bb := NewBasicBlock(0, "test")
	// x * 8 -> x << 3
	bb.AddInst(IRInst{Op: IR_CONST, Value: bytecode.NewInt(8)})
	bb.AddInst(IRInst{Op: IR_MUL})

	changed := pass.runOnBlock(bb)

	if !changed {
		t.Error("expected strength reduction to change the block")
	}

	if len(bb.Insts) != 2 {
		t.Errorf("expected 2 instructions, got %d", len(bb.Insts))
	}

	// 应该变成 CONST 3 + SHL
	if bb.Insts[0].Value.AsInt() != 3 {
		t.Errorf("expected shift amount 3, got %d", bb.Insts[0].Value.AsInt())
	}
	if bb.Insts[1].Op != IR_SHL {
		t.Errorf("expected SHL, got %v", bb.Insts[1].Op)
	}
}

func TestPeepholeOptimization(t *testing.T) {
	pass := NewPeepholePass()

	bb := NewBasicBlock(0, "test")
	// NEG + NEG = 无操作
	bb.AddInst(IRInst{Op: IR_NEG})
	bb.AddInst(IRInst{Op: IR_NEG})

	changed := pass.runOnBlock(bb)

	if !changed {
		t.Error("expected peephole to change the block")
	}

	if len(bb.Insts) != 0 {
		t.Errorf("expected 0 instructions, got %d", len(bb.Insts))
	}
}

// ============================================================================
// JIT 编译器测试
// ============================================================================

func TestJITCompiler_Compile(t *testing.T) {
	compiler := NewCompiler(DefaultConfig())

	// 创建简单函数
	fn := &bytecode.Function{
		Name:  "add",
		Arity: 2,
		Chunk: &bytecode.Chunk{
			Code: []byte{
				byte(bytecode.OpLoadLocal), 0, 0,
				byte(bytecode.OpLoadLocal), 0, 1,
				byte(bytecode.OpAdd),
				byte(bytecode.OpReturn),
			},
			Constants: []bytecode.Value{},
		},
	}

	compiled, err := compiler.Compile(fn)
	if err != nil {
		t.Fatalf("compilation failed: %v", err)
	}

	if compiled == nil {
		t.Fatal("compiled code is nil")
	}

	if len(compiled.IRInsts) == 0 {
		t.Error("expected IR instructions")
	}
}

func TestJITCompiler_CanCompile(t *testing.T) {
	compiler := NewCompiler(DefaultConfig())

	// 支持的函数
	fn := &bytecode.Function{
		Name:  "test",
		Chunk: &bytecode.Chunk{
			Code: []byte{
				byte(bytecode.OpNull),
				byte(bytecode.OpReturn),
			},
		},
	}

	if !compiler.CanCompile(fn) {
		t.Error("should be able to compile simple function")
	}

	// nil 函数
	if compiler.CanCompile(nil) {
		t.Error("should not compile nil function")
	}

	// 无 Chunk 函数
	fn2 := &bytecode.Function{Name: "empty"}
	if compiler.CanCompile(fn2) {
		t.Error("should not compile function without chunk")
	}
}

func TestJITCompiler_Cache(t *testing.T) {
	compiler := NewCompiler(DefaultConfig())

	fn := &bytecode.Function{
		Name:  "cached",
		Chunk: &bytecode.Chunk{
			Code: []byte{byte(bytecode.OpNull), byte(bytecode.OpReturn)},
		},
	}

	// 第一次编译
	code1, _ := compiler.Compile(fn)

	// 第二次应该命中缓存
	code2, _ := compiler.Compile(fn)

	if code1 != code2 {
		t.Error("expected cached result")
	}

	stats := compiler.GetStats()
	if stats.CacheHits != 1 {
		t.Errorf("expected 1 cache hit, got %d", stats.CacheHits)
	}
}

// ============================================================================
// 代码生成器测试
// ============================================================================

func TestCodeGenerator_BasicInstructions(t *testing.T) {
	g := NewCodeGenerator()

	// MOV RAX, RBX
	g.EmitMovRegReg(RAX, RBX)
	if len(g.Code()) == 0 {
		t.Error("expected code to be generated")
	}

	// ADD RAX, 10
	g.EmitAddRegImm32(RAX, 10)

	// SUB RCX, RDX
	g.EmitSubRegReg(RCX, RDX)

	// RET
	g.EmitRet()

	code := g.Code()
	if len(code) < 10 {
		t.Errorf("expected more code, got %d bytes", len(code))
	}
}

func TestCodeGenerator_PrologueEpilogue(t *testing.T) {
	g := NewCodeGenerator()

	g.EmitPrologue(32)
	g.EmitMovRegImm64(RAX, 42)
	g.EmitEpilogue()

	code := g.Code()
	if len(code) == 0 {
		t.Error("expected code to be generated")
	}

	// 检查 RET 在最后
	if code[len(code)-1] != 0xC3 {
		t.Error("expected RET at end")
	}
}

func TestCodeGenerator_Jumps(t *testing.T) {
	g := NewCodeGenerator()

	// JMP target
	jumpPos := g.EmitJmpLabel("target")

	// 一些代码
	g.EmitNop()
	g.EmitNop()

	// 目标
	target := g.CurrentOffset()
	g.Label("target")

	// 修补跳转
	g.PatchJump(jumpPos, target)

	code := g.Code()
	if len(code) == 0 {
		t.Error("expected code")
	}
}

// ============================================================================
// 基本块测试
// ============================================================================

func TestBasicBlock_Termination(t *testing.T) {
	bb := NewBasicBlock(0, "test")

	if bb.IsTerminated() {
		t.Error("empty block should not be terminated")
	}

	bb.AddInst(IRInst{Op: IR_CONST})
	if bb.IsTerminated() {
		t.Error("block with only CONST should not be terminated")
	}

	bb.AddInst(IRInst{Op: IR_RETURN})
	if !bb.IsTerminated() {
		t.Error("block with RETURN should be terminated")
	}
}

func TestCFG_Connection(t *testing.T) {
	cfg := NewCFG(nil)

	bb1 := NewBasicBlock(0, "bb1")
	bb2 := NewBasicBlock(1, "bb2")

	cfg.AddBlock(bb1)
	cfg.AddBlock(bb2)
	cfg.Connect(bb1, bb2)

	if len(bb1.Succs) != 1 || bb1.Succs[0] != bb2 {
		t.Error("bb1 should have bb2 as successor")
	}
	if len(bb2.Preds) != 1 || bb2.Preds[0] != bb1 {
		t.Error("bb2 should have bb1 as predecessor")
	}
}

// ============================================================================
// IR 指令测试
// ============================================================================

func TestIRInst_Properties(t *testing.T) {
	tests := []struct {
		inst         IRInst
		isTerminator bool
		isJump       bool
		isBinaryOp   bool
		isUnaryOp    bool
	}{
		{IRInst{Op: IR_RETURN}, true, false, false, false},
		{IRInst{Op: IR_JUMP}, true, true, false, false},
		{IRInst{Op: IR_JUMP_FALSE}, false, true, false, false},
		{IRInst{Op: IR_ADD}, false, false, true, false},
		{IRInst{Op: IR_NEG}, false, false, false, true},
		{IRInst{Op: IR_CONST}, false, false, false, false},
	}

	for _, tt := range tests {
		if tt.inst.IsTerminator() != tt.isTerminator {
			t.Errorf("%v: IsTerminator() = %v, want %v", tt.inst.Op, tt.inst.IsTerminator(), tt.isTerminator)
		}
		if tt.inst.IsJump() != tt.isJump {
			t.Errorf("%v: IsJump() = %v, want %v", tt.inst.Op, tt.inst.IsJump(), tt.isJump)
		}
		if tt.inst.IsBinaryOp() != tt.isBinaryOp {
			t.Errorf("%v: IsBinaryOp() = %v, want %v", tt.inst.Op, tt.inst.IsBinaryOp(), tt.isBinaryOp)
		}
		if tt.inst.IsUnaryOp() != tt.isUnaryOp {
			t.Errorf("%v: IsUnaryOp() = %v, want %v", tt.inst.Op, tt.inst.IsUnaryOp(), tt.isUnaryOp)
		}
	}
}

// ============================================================================
// Pass Manager 测试
// ============================================================================

func TestPassManager(t *testing.T) {
	pm := NewPassManager()

	pm.AddPass(NewConstantFoldingPass())
	pm.AddPass(NewDeadCodeEliminationPass())

	fn := NewIRFunction("test")
	fn.CFG = NewCFG(nil)

	bb := NewBasicBlock(0, "test")
	bb.AddInst(IRInst{Op: IR_CONST, Value: bytecode.NewInt(1)})
	bb.AddInst(IRInst{Op: IR_CONST, Value: bytecode.NewInt(2)})
	bb.AddInst(IRInst{Op: IR_ADD})
	bb.AddInst(IRInst{Op: IR_RETURN})

	fn.CFG.AddBlock(bb)
	fn.CFG.Entry = bb

	pm.Run(fn)

	stats := pm.Stats()
	if stats.PassesRun < 2 {
		t.Errorf("expected at least 2 passes run, got %d", stats.PassesRun)
	}
}

func TestStandardPipeline(t *testing.T) {
	pm := CreateStandardPipeline()

	fn := NewIRFunction("test")
	fn.CFG = NewCFG(nil)

	bb := NewBasicBlock(0, "test")
	bb.AddInst(IRInst{Op: IR_RETURN})
	fn.CFG.AddBlock(bb)
	fn.CFG.Entry = bb

	// 应该不报错
	pm.Run(fn)
}
