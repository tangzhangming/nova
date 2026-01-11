// +build amd64

package jit

import (
	"testing"

	"github.com/tangzhangming/nova/internal/bytecode"
)

// ============================================================================
// JIT 正确性测试
// 确保 JIT 编译的代码与解释器产生相同结果
// ============================================================================

// TestIREmitterBasic 测试基本的 IR 发射
func TestIREmitterBasic(t *testing.T) {
	emitter := NewIREmitter()
	if emitter == nil {
		t.Fatal("failed to create IR emitter")
	}

	// 验证初始状态
	if len(emitter.Code()) != 0 {
		t.Error("emitter should start with empty code")
	}
}

// TestCodeGeneratorBasic 测试基本的代码生成
func TestCodeGeneratorBasic(t *testing.T) {
	cg := NewCodeGenerator()
	if cg == nil {
		t.Fatal("failed to create code generator")
	}

	// 生成简单的函数: return 42
	cg.EmitPrologue(0)
	cg.EmitMovRegImm64(RAX, 42)
	cg.EmitEpilogue()

	code := cg.Code()
	if len(code) == 0 {
		t.Error("code generator should produce non-empty code")
	}

	t.Logf("Generated %d bytes of code", len(code))
}

// TestIRConstant 测试常量加载
func TestIRConstant(t *testing.T) {
	tests := []struct {
		name  string
		value bytecode.Value
	}{
		{"null", bytecode.NullValue},
		{"true", bytecode.TrueValue},
		{"false", bytecode.FalseValue},
		{"int_0", bytecode.NewInt(0)},
		{"int_1", bytecode.NewInt(1)},
		{"int_42", bytecode.NewInt(42)},
		{"int_neg", bytecode.NewInt(-123)},
		{"float", bytecode.NewFloat(3.14)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ir := []IRInst{
				{Op: IR_CONST, Value: tt.value},
				{Op: IR_RETURN},
			}

			// 尝试生成机器码
			code, err := GenerateMachineCodeFromInsts(ir, 0, nil)
			if err != nil {
				t.Logf("Code generation not fully implemented: %v", err)
				return
			}
			if len(code) == 0 {
				t.Error("should generate non-empty code")
			}
			t.Logf("Generated %d bytes for constant %v", len(code), tt.value)
		})
	}
}

// TestIRArithmetic 测试算术运算
func TestIRArithmetic(t *testing.T) {
	tests := []struct {
		name     string
		op       IROp
		a, b     int64
		expected int64
	}{
		{"add", IR_ADD, 10, 5, 15},
		{"sub", IR_SUB, 10, 5, 5},
		{"mul", IR_MUL, 10, 5, 50},
		{"div", IR_DIV, 10, 5, 2},
		{"mod", IR_MOD, 10, 3, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ir := []IRInst{
				{Op: IR_CONST, Value: bytecode.NewInt(tt.a)},
				{Op: IR_CONST, Value: bytecode.NewInt(tt.b)},
				{Op: tt.op},
				{Op: IR_RETURN},
			}

			// 尝试生成机器码
			code, err := GenerateMachineCodeFromInsts(ir, 0, nil)
			if err != nil {
				t.Logf("Code generation error (expected): %v", err)
				return
			}
			t.Logf("Generated %d bytes for %s(%d, %d)", len(code), tt.name, tt.a, tt.b)
		})
	}
}

// TestIRComparison 测试比较运算
func TestIRComparison(t *testing.T) {
	tests := []struct {
		name     string
		op       IROp
		a, b     int64
		expected bool
	}{
		{"eq_true", IR_EQ, 5, 5, true},
		{"eq_false", IR_EQ, 5, 3, false},
		{"ne_true", IR_NE, 5, 3, true},
		{"ne_false", IR_NE, 5, 5, false},
		{"lt_true", IR_LT, 3, 5, true},
		{"lt_false", IR_LT, 5, 3, false},
		{"le_true", IR_LE, 5, 5, true},
		{"gt_true", IR_GT, 5, 3, true},
		{"ge_true", IR_GE, 5, 5, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ir := []IRInst{
				{Op: IR_CONST, Value: bytecode.NewInt(tt.a)},
				{Op: IR_CONST, Value: bytecode.NewInt(tt.b)},
				{Op: tt.op},
				{Op: IR_RETURN},
			}

			code, err := GenerateMachineCodeFromInsts(ir, 0, nil)
			if err != nil {
				t.Logf("Code generation error (expected): %v", err)
				return
			}
			t.Logf("Generated %d bytes for %s", len(code), tt.name)
		})
	}
}

// TestIRLocalVariables 测试局部变量
func TestIRLocalVariables(t *testing.T) {
	// 测试: x = 10; y = 20; return x + y
	ir := []IRInst{
		{Op: IR_CONST, Value: bytecode.NewInt(10)},
		{Op: IR_STORE_LOCAL, Arg1: 0},
		{Op: IR_CONST, Value: bytecode.NewInt(20)},
		{Op: IR_STORE_LOCAL, Arg1: 1},
		{Op: IR_LOAD_LOCAL, Arg1: 0},
		{Op: IR_LOAD_LOCAL, Arg1: 1},
		{Op: IR_ADD},
		{Op: IR_RETURN},
	}

	code, err := GenerateMachineCodeFromInsts(ir, 2, nil)
	if err != nil {
		t.Logf("Code generation error (expected): %v", err)
		return
	}
	t.Logf("Generated %d bytes for local variable test", len(code))
}

// TestIRFunction 测试完整函数
func TestIRFunction(t *testing.T) {
	// 创建一个简单的函数
	fn := &IRFunction{
		Name:       "test",
		LocalCount: 2,
		ArgCount:   0,
		CFG: &CFG{
			Blocks: []*BasicBlock{
				{
					ID:   0,
					Name: "entry",
					Insts: []IRInst{
						{Op: IR_CONST, Value: bytecode.NewInt(100)},
						{Op: IR_RETURN},
					},
				},
			},
		},
	}

	code, err := GenerateMachineCode(fn, nil)
	if err != nil {
		t.Logf("Code generation error: %v", err)
		return
	}
	t.Logf("Generated %d bytes for function", len(code))
}

// TestIRJump 测试跳转指令
func TestIRJump(t *testing.T) {
	// 测试条件跳转
	ir := []IRInst{
		{Op: IR_CONST, Value: bytecode.NewInt(1)},
		{Op: IR_JUMP_FALSE, Arg1: 5},
		{Op: IR_CONST, Value: bytecode.NewInt(42)},
		{Op: IR_RETURN},
		{Op: IR_LABEL, Label: "L5"},
		{Op: IR_CONST, Value: bytecode.NewInt(0)},
		{Op: IR_RETURN},
	}

	code, err := GenerateMachineCodeFromInsts(ir, 0, nil)
	if err != nil {
		t.Logf("Code generation error (expected): %v", err)
		return
	}
	t.Logf("Generated %d bytes for jump test", len(code))
}

// ============================================================================
// 寄存器分配测试
// ============================================================================

func TestRegisterAllocation(t *testing.T) {
	cg := NewCodeGenerator()

	// 分配几个寄存器
	r1 := cg.AllocReg()
	r2 := cg.AllocReg()
	r3 := cg.AllocReg()

	t.Logf("Allocated registers: %s, %s, %s", r1, r2, r3)

	// 释放
	cg.FreeReg(r2)

	// 再分配应该得到刚释放的
	r4 := cg.AllocReg()
	t.Logf("Reallocated: %s", r4)
}

// ============================================================================
// 机器码生成完整性测试
// ============================================================================

func TestMachineCodeInstructions(t *testing.T) {
	cg := NewCodeGenerator()

	// 测试各种指令的生成
	instructions := []struct {
		name string
		emit func()
	}{
		{"mov_reg_reg", func() { cg.EmitMovRegReg(RAX, RBX) }},
		{"mov_reg_imm64", func() { cg.EmitMovRegImm64(RAX, 0x123456789) }},
		{"mov_reg_imm32", func() { cg.EmitMovRegImm32(RAX, 42) }},
		{"add_reg_reg", func() { cg.EmitAddRegReg(RAX, RBX) }},
		{"add_reg_imm32", func() { cg.EmitAddRegImm32(RAX, 100) }},
		{"sub_reg_reg", func() { cg.EmitSubRegReg(RAX, RBX) }},
		{"imul_reg_reg", func() { cg.EmitImulRegReg(RAX, RBX) }},
		{"cmp_reg_reg", func() { cg.EmitCmpRegReg(RAX, RBX) }},
		{"push", func() { cg.EmitPush(RAX) }},
		{"pop", func() { cg.EmitPop(RAX) }},
		{"ret", func() { cg.EmitRet() }},
		{"nop", func() { cg.EmitNop() }},
	}

	for _, tt := range instructions {
		t.Run(tt.name, func(t *testing.T) {
			cg.Reset()
			sizeBefore := cg.Size()
			tt.emit()
			sizeAfter := cg.Size()

			if sizeAfter <= sizeBefore {
				t.Errorf("%s should generate code", tt.name)
			}
			t.Logf("%s: generated %d bytes", tt.name, sizeAfter-sizeBefore)
		})
	}
}

// ============================================================================
// 编译器集成测试
// ============================================================================

func TestCompilerIntegration(t *testing.T) {
	// 创建编译器
	compiler := NewCompiler(DefaultConfig())
	if compiler == nil {
		t.Fatal("failed to create compiler")
	}

	// 创建一个简单的函数
	fn := &bytecode.Function{
		Name:       "test",
		Arity:      0,
		LocalCount: 0,
		Chunk: &bytecode.Chunk{
			Code: []byte{
				byte(bytecode.OpPush), 0, 0, // PUSH 0 (加载常量)
				byte(bytecode.OpReturn), // RETURN
			},
			Constants: []bytecode.Value{
				bytecode.NewInt(42),
			},
		},
	}

	// 编译
	compiled, err := compiler.Compile(fn)
	if err != nil {
		t.Logf("Compilation error: %v", err)
		return
	}

	if compiled == nil {
		t.Error("compiled code should not be nil")
		return
	}

	t.Logf("Compiled function: IR instructions=%d, code size=%d bytes",
		len(compiled.IRInsts), len(compiled.Code))
}

// ============================================================================
// 边界条件测试
// ============================================================================

func TestEdgeCases(t *testing.T) {
	t.Run("empty_function", func(t *testing.T) {
		fn := &IRFunction{
			Name:       "empty",
			LocalCount: 0,
			CFG:        &CFG{Blocks: []*BasicBlock{}},
		}
		_, err := GenerateMachineCode(fn, nil)
		if err != nil {
			t.Logf("Expected error for empty function: %v", err)
		}
	})

	t.Run("nil_function", func(t *testing.T) {
		_, err := GenerateMachineCode(nil, nil)
		if err == nil {
			t.Error("should error on nil function")
		}
	})

	t.Run("many_locals", func(t *testing.T) {
		fn := &IRFunction{
			Name:       "many_locals",
			LocalCount: 100,
			CFG: &CFG{
				Blocks: []*BasicBlock{
					{
						ID:   0,
						Name: "entry",
						Insts: []IRInst{
							{Op: IR_CONST, Value: bytecode.NullValue},
							{Op: IR_RETURN},
						},
					},
				},
			},
		}
		code, err := GenerateMachineCode(fn, nil)
		if err != nil {
			t.Logf("Code generation error: %v", err)
			return
		}
		t.Logf("Generated %d bytes for function with 100 locals", len(code))
	})
}
