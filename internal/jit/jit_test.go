// jit_test.go - JIT 编译器测试
//
// 这些测试验证 JIT 编译器的基本功能

package jit

import (
	"runtime"
	"testing"
	"unsafe"

	"github.com/tangzhangming/nova/internal/bytecode"
)

// TestNewCompiler 测试创建编译器
func TestNewCompiler(t *testing.T) {
	// 默认配置
	c := NewCompiler(nil)
	if c == nil {
		t.Fatal("NewCompiler returned nil")
	}
	
	// 检查是否启用（取决于平台）
	if runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64" {
		if !c.IsEnabled() {
			t.Log("JIT not enabled on supported platform")
		}
	}
	
	// 纯解释模式
	c = NewCompiler(InterpretOnlyConfig())
	if c.IsEnabled() {
		t.Error("Compiler should be disabled with InterpretOnlyConfig")
	}
}

// TestConfig 测试配置
func TestConfig(t *testing.T) {
	cfg := DefaultConfig()
	
	if !cfg.Enabled {
		t.Error("Default config should have JIT enabled")
	}
	
	if cfg.HotThreshold <= 0 {
		t.Error("Hot threshold should be positive")
	}
	
	if cfg.OptimizationLevel < 0 || cfg.OptimizationLevel > 3 {
		t.Error("Optimization level should be 0-3")
	}
}

// TestProfiler 测试热点检测器
func TestProfiler(t *testing.T) {
	p := NewProfiler(10, 5)
	
	if !p.IsEnabled() {
		t.Error("Profiler should be enabled by default")
	}
	
	// 创建测试函数
	fn := &bytecode.Function{
		Name:  "test",
		Chunk: bytecode.NewChunk(),
	}
	
	// 记录调用
	for i := 0; i < 20; i++ {
		p.RecordCall(fn)
	}
	
	count := p.GetCallCount(fn)
	if count != 20 {
		t.Errorf("Expected call count 20, got %d", count)
	}
	
	// 应该变热
	if !p.IsHot(fn) {
		t.Error("Function should be hot after 20 calls (threshold=10)")
	}
}

// TestIRBuilder 测试 IR 构建
func TestIRBuilder(t *testing.T) {
	// 创建简单函数：返回 42
	fn := &bytecode.Function{
		Name:       "returnFortyTwo",
		Chunk:      bytecode.NewChunk(),
		LocalCount: 1,
	}
	
	// 字节码：push 42, return
	chunk := fn.Chunk
	chunk.WriteOp(bytecode.OpPush, 1)
	constIdx := chunk.AddConstant(bytecode.NewInt(42))
	chunk.WriteU16(constIdx, 1)
	chunk.WriteOp(bytecode.OpReturn, 2)
	
	// 构建 IR
	builder := NewIRBuilder()
	irFunc, err := builder.Build(fn)
	if err != nil {
		t.Fatalf("IR build failed: %v", err)
	}
	
	if irFunc.Name != fn.Name {
		t.Errorf("IR function name mismatch: expected %s, got %s", fn.Name, irFunc.Name)
	}
	
	t.Logf("IR:\n%s", irFunc.String())
}

// TestOptimizer 测试优化器
func TestOptimizer(t *testing.T) {
	// 创建简单函数：返回 1 + 2
	fn := &bytecode.Function{
		Name:       "addConstants",
		Chunk:      bytecode.NewChunk(),
		LocalCount: 1,
	}
	
	chunk := fn.Chunk
	// push 1
	chunk.WriteOp(bytecode.OpPush, 1)
	chunk.WriteU16(chunk.AddConstant(bytecode.NewInt(1)), 1)
	// push 2
	chunk.WriteOp(bytecode.OpPush, 1)
	chunk.WriteU16(chunk.AddConstant(bytecode.NewInt(2)), 1)
	// add
	chunk.WriteOp(bytecode.OpAdd, 2)
	// return
	chunk.WriteOp(bytecode.OpReturn, 3)
	
	// 构建 IR
	builder := NewIRBuilder()
	irFunc, err := builder.Build(fn)
	if err != nil {
		t.Fatalf("IR build failed: %v", err)
	}
	
	t.Logf("Before optimization:\n%s", irFunc.String())
	
	// 优化
	opt := NewOptimizer(2)
	opt.Optimize(irFunc)
	
	t.Logf("After optimization:\n%s", irFunc.String())
}

// TestRegisterAllocator 测试寄存器分配
func TestRegisterAllocator(t *testing.T) {
	// 创建简单函数
	// LocalCount 必须至少为 Arity + 1（local[0] 预留给 this/调用者）
	fn := &bytecode.Function{
		Name:       "simpleAdd",
		Arity:      2,
		Chunk:      bytecode.NewChunk(),
		LocalCount: 3, // local[0] 预留 + local[1], local[2] 用于参数
	}
	
	chunk := fn.Chunk
	// load local 0
	chunk.WriteOp(bytecode.OpLoadLocal, 1)
	chunk.WriteU16(0, 1)
	// load local 1
	chunk.WriteOp(bytecode.OpLoadLocal, 1)
	chunk.WriteU16(1, 1)
	// add
	chunk.WriteOp(bytecode.OpAdd, 2)
	// return
	chunk.WriteOp(bytecode.OpReturn, 3)
	
	// 构建 IR
	builder := NewIRBuilder()
	irFunc, err := builder.Build(fn)
	if err != nil {
		t.Fatalf("IR build failed: %v", err)
	}
	
	// 寄存器分配
	regalloc := NewRegisterAllocator(10)
	alloc := regalloc.Allocate(irFunc)
	
	if alloc == nil {
		t.Fatal("Register allocation returned nil")
	}
	
	t.Logf("Stack size: %d bytes", alloc.StackSize)
	t.Logf("Value register mappings: %v", alloc.ValueRegs)
}

// TestX64Assembler 测试 x86-64 汇编器
func TestX64Assembler(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		t.Skip("Skipping x64 assembler test on non-amd64 platform")
	}
	
	asm := NewX64Assembler()
	
	// 生成简单函数：mov rax, 42; ret
	asm.MovRegImm64(RAX, 42)
	asm.Ret()
	
	code := asm.Code()
	if len(code) == 0 {
		t.Fatal("Generated code is empty")
	}
	
	t.Logf("Generated %d bytes of code", len(code))
	
	// 验证代码
	// mov rax, 42 = 48 B8 2A 00 00 00 00 00 00 00 (10 bytes)
	// ret = C3 (1 byte)
	expectedLen := 11
	if len(code) != expectedLen {
		t.Errorf("Expected %d bytes, got %d", expectedLen, len(code))
	}
}

// TestARM64Assembler 测试 ARM64 汇编器
func TestARM64Assembler(t *testing.T) {
	if runtime.GOARCH != "arm64" {
		t.Skip("Skipping ARM64 assembler test on non-arm64 platform")
	}
	
	asm := NewARM64Assembler()
	
	// 生成简单函数：mov x0, 42; ret
	asm.MovRegImm64(X0, 42)
	asm.Ret()
	
	code := asm.Code()
	if len(code) == 0 {
		t.Fatal("Generated code is empty")
	}
	
	// ARM64 指令是 4 字节对齐的
	if len(code)%4 != 0 {
		t.Errorf("Code length should be 4-byte aligned, got %d", len(code))
	}
	
	t.Logf("Generated %d bytes of code", len(code))
}

// TestCodeGenerator 测试代码生成器
func TestCodeGenerator(t *testing.T) {
	// 创建简单函数
	fn := &bytecode.Function{
		Name:       "return42",
		Chunk:      bytecode.NewChunk(),
		LocalCount: 1,
	}
	
	chunk := fn.Chunk
	chunk.WriteOp(bytecode.OpPush, 1)
	chunk.WriteU16(chunk.AddConstant(bytecode.NewInt(42)), 1)
	chunk.WriteOp(bytecode.OpReturn, 2)
	
	// 构建 IR
	builder := NewIRBuilder()
	irFunc, err := builder.Build(fn)
	if err != nil {
		t.Fatalf("IR build failed: %v", err)
	}
	
	// 寄存器分配
	regalloc := NewRegisterAllocator(10)
	alloc := regalloc.Allocate(irFunc)
	
	// 代码生成
	var codegen CodeGenerator
	switch runtime.GOARCH {
	case "amd64":
		codegen = NewX64CodeGenerator()
	case "arm64":
		codegen = NewARM64CodeGenerator()
	default:
		t.Skip("Unsupported architecture")
	}
	
	code, err := codegen.Generate(irFunc, alloc)
	if err != nil {
		t.Fatalf("Code generation failed: %v", err)
	}
	
	if len(code) == 0 {
		t.Fatal("Generated code is empty")
	}
	
	t.Logf("Generated %d bytes of code", len(code))
}

// TestCanJIT 测试 JIT 适用性检查
func TestCanJIT(t *testing.T) {
	// 简单函数应该可以 JIT
	simple := &bytecode.Function{
		Name:  "simple",
		Chunk: bytecode.NewChunk(),
	}
	simple.Chunk.WriteOp(bytecode.OpZero, 1)
	simple.Chunk.WriteOp(bytecode.OpReturn, 2)
	
	if !CanJIT(simple) {
		t.Error("Simple function should be JIT-able")
	}
	
	// 带闭包的函数不能 JIT
	withUpvalues := &bytecode.Function{
		Name:         "withUpvalues",
		Chunk:        bytecode.NewChunk(),
		UpvalueCount: 1,
	}
	if CanJIT(withUpvalues) {
		t.Error("Function with upvalues should not be JIT-able")
	}
	
	// 带函数调用的函数不能 JIT
	withCall := &bytecode.Function{
		Name:  "withCall",
		Chunk: bytecode.NewChunk(),
	}
	withCall.Chunk.WriteOp(bytecode.OpCall, 1)
	withCall.Chunk.Write(0, 1) // arg count
	withCall.Chunk.WriteOp(bytecode.OpReturn, 2)
	
	if CanJIT(withCall) {
		t.Error("Function with call should not be JIT-able")
	}
}

// TestFullCompilePipeline 测试完整编译流程
func TestFullCompilePipeline(t *testing.T) {
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skip("Skipping on unsupported architecture")
	}
	
	// 创建编译器
	compiler := NewCompiler(nil)
	if compiler == nil || !compiler.IsEnabled() {
		t.Skip("JIT compiler not available")
	}
	
	// 创建简单函数：返回 42
	fn := &bytecode.Function{
		Name:       "return42",
		Chunk:      bytecode.NewChunk(),
		LocalCount: 1,
	}
	
	chunk := fn.Chunk
	chunk.WriteOp(bytecode.OpPush, 1)
	chunk.WriteU16(chunk.AddConstant(bytecode.NewInt(42)), 1)
	chunk.WriteOp(bytecode.OpReturn, 2)
	
	// 编译
	compiled, err := compiler.Compile(fn)
	if err != nil {
		t.Fatalf("Compilation failed: %v", err)
	}
	
	if compiled == nil {
		t.Fatal("Compiled function is nil")
	}
	
	t.Logf("Compiled function: %s (%d bytes)", compiled.Name, len(compiled.Code))
	
	// 验证入口点
	entry := compiled.EntryPoint()
	if entry == 0 {
		t.Error("Entry point is 0")
	}
}

// ============================================================================
// 数组操作测试
// ============================================================================

// TestCanJIT_ArrayOps 测试带数组操作的函数是否可以 JIT
func TestCanJIT_ArrayOps(t *testing.T) {
	// 带 ArrayLen 的函数应该可以 JIT
	withArrayLen := &bytecode.Function{
		Name:       "withArrayLen",
		Chunk:      bytecode.NewChunk(),
		LocalCount: 2,
	}
	chunk := withArrayLen.Chunk
	// load local 1 (假设是数组)
	chunk.WriteOp(bytecode.OpLoadLocal, 1)
	chunk.WriteU16(1, 1)
	// arraylen
	chunk.WriteOp(bytecode.OpArrayLen, 2)
	// return
	chunk.WriteOp(bytecode.OpReturn, 3)
	
	if !CanJIT(withArrayLen) {
		t.Error("Function with ArrayLen should be JIT-able")
	}
	
	// 带 ArrayGet 的函数应该可以 JIT
	withArrayGet := &bytecode.Function{
		Name:       "withArrayGet",
		Chunk:      bytecode.NewChunk(),
		LocalCount: 3,
	}
	chunk = withArrayGet.Chunk
	// load local 1 (数组)
	chunk.WriteOp(bytecode.OpLoadLocal, 1)
	chunk.WriteU16(1, 1)
	// load local 2 (索引)
	chunk.WriteOp(bytecode.OpLoadLocal, 1)
	chunk.WriteU16(2, 1)
	// arrayget
	chunk.WriteOp(bytecode.OpArrayGet, 2)
	// return
	chunk.WriteOp(bytecode.OpReturn, 3)
	
	if !CanJIT(withArrayGet) {
		t.Error("Function with ArrayGet should be JIT-able")
	}
	
	// 带 ArraySet 的函数应该可以 JIT
	withArraySet := &bytecode.Function{
		Name:       "withArraySet",
		Chunk:      bytecode.NewChunk(),
		LocalCount: 4,
	}
	chunk = withArraySet.Chunk
	// load local 1 (数组)
	chunk.WriteOp(bytecode.OpLoadLocal, 1)
	chunk.WriteU16(1, 1)
	// load local 2 (索引)
	chunk.WriteOp(bytecode.OpLoadLocal, 1)
	chunk.WriteU16(2, 1)
	// load local 3 (值)
	chunk.WriteOp(bytecode.OpLoadLocal, 1)
	chunk.WriteU16(3, 1)
	// arrayset
	chunk.WriteOp(bytecode.OpArraySet, 2)
	// pop (ArraySet 推回数组)
	chunk.WriteOp(bytecode.OpPop, 3)
	// return null
	chunk.WriteOp(bytecode.OpReturnNull, 4)
	
	if !CanJIT(withArraySet) {
		t.Error("Function with ArraySet should be JIT-able")
	}
	
	// 带 NewArray 的函数不能 JIT（需要内存分配）
	withNewArray := &bytecode.Function{
		Name:       "withNewArray",
		Chunk:      bytecode.NewChunk(),
		LocalCount: 1,
	}
	chunk = withNewArray.Chunk
	chunk.WriteOp(bytecode.OpNewArray, 1)
	chunk.WriteU16(5, 1) // 创建长度为 5 的数组
	chunk.WriteOp(bytecode.OpReturn, 2)
	
	if CanJIT(withNewArray) {
		t.Error("Function with NewArray should not be JIT-able")
	}
}

// TestArrayIRBuilder 测试数组操作的 IR 构建
func TestArrayIRBuilder(t *testing.T) {
	// 创建带 ArrayLen 的函数
	fn := &bytecode.Function{
		Name:       "getArrayLen",
		Arity:      1,
		Chunk:      bytecode.NewChunk(),
		LocalCount: 2,
	}
	
	chunk := fn.Chunk
	// load local 1 (数组参数)
	chunk.WriteOp(bytecode.OpLoadLocal, 1)
	chunk.WriteU16(1, 1)
	// arraylen
	chunk.WriteOp(bytecode.OpArrayLen, 2)
	// return
	chunk.WriteOp(bytecode.OpReturn, 3)
	
	// 构建 IR
	builder := NewIRBuilder()
	irFunc, err := builder.Build(fn)
	if err != nil {
		t.Fatalf("IR build failed: %v", err)
	}
	
	t.Logf("IR for getArrayLen:\n%s", irFunc.String())
	
	// 验证生成了 ArrayLen 指令
	hasArrayLen := false
	for _, block := range irFunc.Blocks {
		for _, instr := range block.Instrs {
			if instr.Op == OpArrayLen {
				hasArrayLen = true
				break
			}
		}
	}
	
	if !hasArrayLen {
		t.Error("IR should contain ArrayLen instruction")
	}
}

// TestArrayGetSetIRBuilder 测试 ArrayGet/ArraySet 的 IR 构建
func TestArrayGetSetIRBuilder(t *testing.T) {
	// 创建带 ArrayGet 的函数
	fn := &bytecode.Function{
		Name:       "getArrayElement",
		Arity:      2, // arr, index
		Chunk:      bytecode.NewChunk(),
		LocalCount: 3,
	}
	
	chunk := fn.Chunk
	// load local 1 (数组)
	chunk.WriteOp(bytecode.OpLoadLocal, 1)
	chunk.WriteU16(1, 1)
	// load local 2 (索引)
	chunk.WriteOp(bytecode.OpLoadLocal, 1)
	chunk.WriteU16(2, 1)
	// arrayget
	chunk.WriteOp(bytecode.OpArrayGet, 2)
	// return
	chunk.WriteOp(bytecode.OpReturn, 3)
	
	builder := NewIRBuilder()
	irFunc, err := builder.Build(fn)
	if err != nil {
		t.Fatalf("IR build failed: %v", err)
	}
	
	t.Logf("IR for getArrayElement:\n%s", irFunc.String())
	
	// 验证生成了 ArrayGet 指令
	hasArrayGet := false
	for _, block := range irFunc.Blocks {
		for _, instr := range block.Instrs {
			if instr.Op == OpArrayGet {
				hasArrayGet = true
				// 验证参数数量
				if len(instr.Args) != 2 {
					t.Errorf("ArrayGet should have 2 args, got %d", len(instr.Args))
				}
				break
			}
		}
	}
	
	if !hasArrayGet {
		t.Error("IR should contain ArrayGet instruction")
	}
}

// TestRuntimeHelpers 测试运行时辅助函数
func TestRuntimeHelpers(t *testing.T) {
	// 测试 ArrayLenHelper
	t.Run("ArrayLenHelper", func(t *testing.T) {
		// 创建测试数组
		arr := bytecode.Value{
			Type: bytecode.ValArray,
			Data: []bytecode.Value{
				bytecode.NewInt(1),
				bytecode.NewInt(2),
				bytecode.NewInt(3),
			},
		}
		
		// 获取数组指针
		arrPtr := uintptr(unsafe.Pointer(&arr))
		
		// 调用辅助函数
		length := ArrayLenHelper(arrPtr)
		
		if length != 3 {
			t.Errorf("Expected length 3, got %d", length)
		}
		
		// 测试空指针
		length = ArrayLenHelper(0)
		if length != -1 {
			t.Errorf("Expected -1 for nil pointer, got %d", length)
		}
		
		// 测试非数组类型
		notArr := bytecode.NewInt(42)
		notArrPtr := uintptr(unsafe.Pointer(&notArr))
		length = ArrayLenHelper(notArrPtr)
		if length != -1 {
			t.Errorf("Expected -1 for non-array, got %d", length)
		}
	})
	
	// 测试 ArrayGetHelper
	t.Run("ArrayGetHelper", func(t *testing.T) {
		arr := bytecode.Value{
			Type: bytecode.ValArray,
			Data: []bytecode.Value{
				bytecode.NewInt(10),
				bytecode.NewInt(20),
				bytecode.NewInt(30),
			},
		}
		arrPtr := uintptr(unsafe.Pointer(&arr))
		
		// 正常访问
		value, ok := ArrayGetHelper(arrPtr, 1)
		if ok != 1 {
			t.Error("ArrayGetHelper should return ok=1 for valid index")
		}
		if value != 20 {
			t.Errorf("Expected 20, got %d", value)
		}
		
		// 越界访问
		_, ok = ArrayGetHelper(arrPtr, 10)
		if ok != 0 {
			t.Error("ArrayGetHelper should return ok=0 for out of bounds")
		}
		
		// 负索引
		_, ok = ArrayGetHelper(arrPtr, -1)
		if ok != 0 {
			t.Error("ArrayGetHelper should return ok=0 for negative index")
		}
	})
	
	// 测试 ArraySetHelper
	t.Run("ArraySetHelper", func(t *testing.T) {
		arr := bytecode.Value{
			Type: bytecode.ValArray,
			Data: []bytecode.Value{
				bytecode.NewInt(10),
				bytecode.NewInt(20),
				bytecode.NewInt(30),
			},
		}
		arrPtr := uintptr(unsafe.Pointer(&arr))
		
		// 正常设置
		ok := ArraySetHelper(arrPtr, 1, 99)
		if ok != 1 {
			t.Error("ArraySetHelper should return 1 for valid index")
		}
		
		// 验证值已更改
		elements := arr.Data.([]bytecode.Value)
		if elements[1].AsInt() != 99 {
			t.Errorf("Expected 99, got %d", elements[1].AsInt())
		}
		
		// 越界设置
		ok = ArraySetHelper(arrPtr, 10, 100)
		if ok != 0 {
			t.Error("ArraySetHelper should return 0 for out of bounds")
		}
	})
}

// TestArrayCodeGeneration 测试数组操作的代码生成
func TestArrayCodeGeneration(t *testing.T) {
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skip("Skipping on unsupported architecture")
	}
	
	// 创建带 ArrayLen 的函数
	fn := &bytecode.Function{
		Name:       "testArrayLen",
		Arity:      1,
		Chunk:      bytecode.NewChunk(),
		LocalCount: 2,
	}
	
	chunk := fn.Chunk
	chunk.WriteOp(bytecode.OpLoadLocal, 1)
	chunk.WriteU16(1, 1)
	chunk.WriteOp(bytecode.OpArrayLen, 2)
	chunk.WriteOp(bytecode.OpReturn, 3)
	
	// 构建 IR
	builder := NewIRBuilder()
	irFunc, err := builder.Build(fn)
	if err != nil {
		t.Fatalf("IR build failed: %v", err)
	}
	
	// 寄存器分配
	regalloc := NewRegisterAllocator(10)
	alloc := regalloc.Allocate(irFunc)
	
	// 代码生成
	var codegen CodeGenerator
	switch runtime.GOARCH {
	case "amd64":
		codegen = NewX64CodeGenerator()
	case "arm64":
		codegen = NewARM64CodeGenerator()
	default:
		t.Skip("Unsupported architecture")
	}
	
	code, err := codegen.Generate(irFunc, alloc)
	if err != nil {
		t.Fatalf("Code generation failed: %v", err)
	}
	
	if len(code) == 0 {
		t.Fatal("Generated code is empty")
	}
	
	t.Logf("Generated %d bytes of code for array operation", len(code))
}

// TestGetHelperPtrs 测试获取辅助函数指针
func TestGetHelperPtrs(t *testing.T) {
	lenPtr := GetArrayLenHelperPtr()
	if lenPtr == 0 {
		t.Error("ArrayLenHelper pointer should not be 0")
	}
	
	getPtr := GetArrayGetHelperPtr()
	if getPtr == 0 {
		t.Error("ArrayGetHelper pointer should not be 0")
	}
	
	setPtr := GetArraySetHelperPtr()
	if setPtr == 0 {
		t.Error("ArraySetHelper pointer should not be 0")
	}
	
	// 三个指针应该不同
	if lenPtr == getPtr || getPtr == setPtr || lenPtr == setPtr {
		t.Error("Helper function pointers should be different")
	}
	
	t.Logf("ArrayLenHelper: %#x", lenPtr)
	t.Logf("ArrayGetHelper: %#x", getPtr)
	t.Logf("ArraySetHelper: %#x", setPtr)
}
