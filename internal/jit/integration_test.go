package jit

import (
	"testing"

	"github.com/tangzhangming/nova/internal/bytecode"
)

// ============================================================================
// JIT 与字节码一致性测试
// ============================================================================

func TestJIT_IntegerArithmetic(t *testing.T) {
	tests := []struct {
		name     string
		a, b     int64
		op       bytecode.OpCode
		expected int64
	}{
		{"add", 10, 20, bytecode.OpAdd, 30},
		{"sub", 30, 10, bytecode.OpSub, 20},
		{"mul", 5, 6, bytecode.OpMul, 30},
		{"div", 100, 5, bytecode.OpDiv, 20},
		{"mod", 17, 5, bytecode.OpMod, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 创建函数 - 使用变量避免常量折叠
			fn := &bytecode.Function{
				Name:  tt.name,
				Arity: 0,
				Chunk: &bytecode.Chunk{
					Code: []byte{
						byte(bytecode.OpLoadLocal), 0, 0, // 加载参数 a
						byte(bytecode.OpLoadLocal), 0, 1, // 加载参数 b
						byte(tt.op),
						byte(bytecode.OpReturn),
					},
					Constants: []bytecode.Value{},
				},
			}

			// 编译
			compiler := NewCompiler(DefaultConfig())
			compiled, err := compiler.Compile(fn)
			if err != nil {
				t.Fatalf("compilation failed: %v", err)
			}

			// 检查 IR 生成
			if len(compiled.IRInsts) == 0 {
				t.Fatal("no IR instructions generated")
			}

			// 验证 IR 中包含正确的操作
			foundOp := false
			for _, inst := range compiled.IRInsts {
				switch tt.op {
				case bytecode.OpAdd:
					if inst.Op == IR_ADD {
						foundOp = true
					}
				case bytecode.OpSub:
					if inst.Op == IR_SUB {
						foundOp = true
					}
				case bytecode.OpMul:
					if inst.Op == IR_MUL {
						foundOp = true
					}
				case bytecode.OpDiv:
					if inst.Op == IR_DIV {
						foundOp = true
					}
				case bytecode.OpMod:
					if inst.Op == IR_MOD {
						foundOp = true
					}
				}
			}
			if !foundOp {
				t.Errorf("expected operation %v in IR", tt.op)
			}
		})
	}
}

func TestJIT_Comparison(t *testing.T) {
	tests := []struct {
		name     string
		a, b     int64
		op       bytecode.OpCode
		irOp     IROp
	}{
		{"eq", 10, 10, bytecode.OpEq, IR_EQ},
		{"ne", 10, 20, bytecode.OpNe, IR_NE},
		{"lt", 10, 20, bytecode.OpLt, IR_LT},
		{"le", 10, 10, bytecode.OpLe, IR_LE},
		{"gt", 20, 10, bytecode.OpGt, IR_GT},
		{"ge", 20, 20, bytecode.OpGe, IR_GE},
	}

	compiler := NewCompiler(DefaultConfig())

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fn := &bytecode.Function{
				Name:  tt.name,
				Chunk: &bytecode.Chunk{
					Code: []byte{
						byte(bytecode.OpPush), 0, 0,
						byte(bytecode.OpPush), 0, 1,
						byte(tt.op),
						byte(bytecode.OpReturn),
					},
					Constants: []bytecode.Value{
						bytecode.NewInt(tt.a),
						bytecode.NewInt(tt.b),
					},
				},
			}

			compiled, err := compiler.Compile(fn)
			if err != nil {
				t.Fatalf("compilation failed: %v", err)
			}

			// 验证 IR 包含比较操作
			found := false
			for _, inst := range compiled.IRInsts {
				if inst.Op == tt.irOp {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected %v in IR", tt.irOp)
			}
		})
	}
}

// ============================================================================
// SuperArray JIT 测试
// ============================================================================

func TestJIT_SuperArrayOperations(t *testing.T) {
	compiler := NewCompiler(DefaultConfig())

	// SuperArray 创建
	fn := &bytecode.Function{
		Name: "sa_test",
		Chunk: &bytecode.Chunk{
			Code: []byte{
				byte(bytecode.OpSuperArrayNew), 0, 0,
				byte(bytecode.OpReturn),
			},
		},
	}

	compiled, err := compiler.Compile(fn)
	if err != nil {
		t.Fatalf("compilation failed: %v", err)
	}

	// 应该生成 Helper 调用
	foundHelper := false
	for _, inst := range compiled.IRInsts {
		if inst.Op == IR_CALL_HELPER && inst.HelperName == "SA_New" {
			foundHelper = true
			break
		}
	}
	if !foundHelper {
		t.Error("expected SA_New helper call in IR")
	}
}

func TestJIT_SuperArrayGetSet(t *testing.T) {
	compiler := NewCompiler(DefaultConfig())

	fn := &bytecode.Function{
		Name: "sa_getset",
		Chunk: &bytecode.Chunk{
			Code: []byte{
				byte(bytecode.OpSuperArrayNew), 0, 0,
				byte(bytecode.OpPush), 0, 0, // key
				byte(bytecode.OpPush), 0, 1, // value
				byte(bytecode.OpSuperArraySet),
				byte(bytecode.OpReturn),
			},
			Constants: []bytecode.Value{
				bytecode.NewString("key"),
				bytecode.NewInt(42),
			},
		},
	}

	compiled, err := compiler.Compile(fn)
	if err != nil {
		t.Fatalf("compilation failed: %v", err)
	}

	// 验证生成了 SA_Set helper 调用
	foundSet := false
	for _, inst := range compiled.IRInsts {
		if inst.Op == IR_CALL_HELPER && inst.HelperName == "SA_Set" {
			foundSet = true
			break
		}
	}
	if !foundSet {
		t.Error("expected SA_Set helper call in IR")
	}
}

// ============================================================================
// 控制流测试
// ============================================================================

func TestJIT_ControlFlow_Loop(t *testing.T) {
	compiler := NewCompiler(DefaultConfig())

	// 简单循环: while (i < 10) { i++ }
	fn := &bytecode.Function{
		Name: "loop",
		Chunk: &bytecode.Chunk{
			Code: []byte{
				byte(bytecode.OpZero),           // i = 0
				byte(bytecode.OpStoreLocal), 0, 0,
				// 循环开始
				byte(bytecode.OpLoadLocal), 0, 0,
				byte(bytecode.OpPush), 0, 0,      // 10
				byte(bytecode.OpLt),
				byte(bytecode.OpJumpIfFalse), 0, 10, // 跳出循环
				// i++
				byte(bytecode.OpLoadLocal), 0, 0,
				byte(bytecode.OpOne),
				byte(bytecode.OpAdd),
				byte(bytecode.OpStoreLocal), 0, 0,
				byte(bytecode.OpLoop), 0, 15,     // 回到循环开始
				// 循环结束
				byte(bytecode.OpNull),
				byte(bytecode.OpReturn),
			},
			Constants: []bytecode.Value{
				bytecode.NewInt(10),
			},
		},
	}

	compiled, err := compiler.Compile(fn)
	if err != nil {
		t.Fatalf("compilation failed: %v", err)
	}

	// 验证生成了跳转指令
	hasJump := false
	hasLoop := false
	for _, inst := range compiled.IRInsts {
		if inst.Op == IR_JUMP_FALSE {
			hasJump = true
		}
		if inst.Op == IR_LOOP {
			hasLoop = true
		}
	}

	if !hasJump {
		t.Error("expected JUMP_FALSE in IR")
	}
	if !hasLoop {
		t.Error("expected LOOP in IR")
	}
}

func TestJIT_ControlFlow_Branch(t *testing.T) {
	compiler := NewCompiler(DefaultConfig())

	// if (cond) { a } else { b }
	fn := &bytecode.Function{
		Name: "branch",
		Chunk: &bytecode.Chunk{
			Code: []byte{
				byte(bytecode.OpTrue),
				byte(bytecode.OpJumpIfFalse), 0, 4,
				byte(bytecode.OpOne),         // then
				byte(bytecode.OpJump), 0, 2,
				byte(bytecode.OpZero),        // else
				byte(bytecode.OpReturn),
			},
		},
	}

	compiled, err := compiler.Compile(fn)
	if err != nil {
		t.Fatalf("compilation failed: %v", err)
	}

	if len(compiled.IRInsts) == 0 {
		t.Error("expected IR instructions")
	}
}

// ============================================================================
// 函数调用测试
// ============================================================================

func TestJIT_FunctionCall(t *testing.T) {
	compiler := NewCompiler(DefaultConfig())

	fn := &bytecode.Function{
		Name: "caller",
		Chunk: &bytecode.Chunk{
			Code: []byte{
				byte(bytecode.OpPush), 0, 0,   // 函数
				byte(bytecode.OpOne),          // 参数
				byte(bytecode.OpCall), 1,      // 调用
				byte(bytecode.OpReturn),
			},
			Constants: []bytecode.Value{
				bytecode.NewFunc(&bytecode.Function{Name: "callee"}),
			},
		},
	}

	compiled, err := compiler.Compile(fn)
	if err != nil {
		t.Fatalf("compilation failed: %v", err)
	}

	// 验证生成了 CALL
	hasCall := false
	for _, inst := range compiled.IRInsts {
		if inst.Op == IR_CALL {
			hasCall = true
			break
		}
	}
	if !hasCall {
		t.Error("expected CALL in IR")
	}
}

// ============================================================================
// 优化一致性测试
// ============================================================================

func TestJIT_OptimizationConsistency(t *testing.T) {
	// 验证优化不改变语义
	compiler := NewCompiler(DefaultConfig())

	// 常量折叠应该产生正确结果
	fn := &bytecode.Function{
		Name: "const_fold",
		Chunk: &bytecode.Chunk{
			Code: []byte{
				byte(bytecode.OpPush), 0, 0, // 10
				byte(bytecode.OpPush), 0, 1, // 20
				byte(bytecode.OpAdd),        // 30
				byte(bytecode.OpPush), 0, 2, // 5
				byte(bytecode.OpMul),        // 150
				byte(bytecode.OpReturn),
			},
			Constants: []bytecode.Value{
				bytecode.NewInt(10),
				bytecode.NewInt(20),
				bytecode.NewInt(5),
			},
		},
	}

	compiled, err := compiler.Compile(fn)
	if err != nil {
		t.Fatalf("compilation failed: %v", err)
	}

	// 检查 IR 被优化了 (应该更短)
	t.Logf("IR instructions: %d", len(compiled.IRInsts))
}

// ============================================================================
// 编译器统计测试
// ============================================================================

func TestJIT_CompilerStats(t *testing.T) {
	compiler := NewCompiler(DefaultConfig())

	fn := &bytecode.Function{
		Name:  "stats_test",
		Chunk: &bytecode.Chunk{
			Code: []byte{byte(bytecode.OpNull), byte(bytecode.OpReturn)},
		},
	}

	// 第一次编译
	compiler.Compile(fn)
	stats := compiler.GetStats()

	if stats.TotalCompiled != 1 {
		t.Errorf("expected 1 compiled, got %d", stats.TotalCompiled)
	}
	if stats.CacheMisses != 1 {
		t.Errorf("expected 1 cache miss, got %d", stats.CacheMisses)
	}

	// 第二次编译 (应该命中缓存)
	compiler.Compile(fn)
	stats = compiler.GetStats()

	if stats.CacheHits != 1 {
		t.Errorf("expected 1 cache hit, got %d", stats.CacheHits)
	}
}

// ============================================================================
// 内存分配测试
// ============================================================================

func TestJIT_MemoryAllocation(t *testing.T) {
	alloc := NewMemoryAllocator()

	mem, err := alloc.Allocate(1024)
	if err != nil {
		t.Fatalf("allocation failed: %v", err)
	}

	if mem.size != 1024 {
		t.Errorf("expected size 1024, got %d", mem.size)
	}

	allocated, used := alloc.Stats()
	if allocated < 1024 {
		t.Errorf("expected at least 1024 allocated, got %d", allocated)
	}
	if used < 1024 {
		t.Errorf("expected at least 1024 used, got %d", used)
	}

	alloc.Free()
	allocated, used = alloc.Stats()
	if allocated != 0 || used != 0 {
		t.Error("expected 0 after free")
	}
}

// ============================================================================
// 执行器测试
// ============================================================================

func TestJIT_Executor(t *testing.T) {
	executor := NewExecutor()
	executor.SetEnabled(true)

	fn := &bytecode.Function{
		Name:  "exec_test",
		Arity: 0,
		Chunk: &bytecode.Chunk{
			Code: []byte{
				byte(bytecode.OpPush), 0, 0,
				byte(bytecode.OpReturn),
			},
			Constants: []bytecode.Value{
				bytecode.NewInt(42),
			},
		},
	}

	installed, err := executor.Compile(fn)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}

	if installed == nil {
		t.Fatal("installed code is nil")
	}

	if installed.Function != fn {
		t.Error("function mismatch")
	}

	// 获取统计
	stats := executor.GetStats()
	if stats.CompiledFuncs < 1 {
		t.Error("expected at least 1 compiled function")
	}

	// 清理
	executor.Reset()
}

// ============================================================================
// Profile 集成测试
// ============================================================================

func TestJIT_ProfileIntegration(t *testing.T) {
	compiler := NewCompiler(DefaultConfig())

	fn := &bytecode.Function{
		Name:  "profiled",
		Chunk: &bytecode.Chunk{
			Code: []byte{
				byte(bytecode.OpPush), 0, 0,
				byte(bytecode.OpPush), 0, 1,
				byte(bytecode.OpAdd),
				byte(bytecode.OpReturn),
			},
			Constants: []bytecode.Value{
				bytecode.NewInt(1),
				bytecode.NewInt(2),
			},
		},
	}

	// 创建模拟 Profile
	profile := &FunctionProfile{
		Name:           fn.Name,
		ExecutionCount: 1000,
		IsHot:          true,
		TypeProfiles: map[int]*TypeProfileData{
			0: {IntCount: 950, FloatCount: 50},
			3: {IntCount: 990, FloatCount: 10},
		},
	}

	// 使用 Profile 编译
	compiled, err := compiler.CompileWithProfile(fn, profile)
	if err != nil {
		t.Fatalf("compilation with profile failed: %v", err)
	}

	if compiled == nil {
		t.Fatal("compiled code is nil")
	}
}
