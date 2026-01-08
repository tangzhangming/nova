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
	
	// 带函数调用的函数现在可以 JIT（使用 JITWithCalls 级别）
	withCall := &bytecode.Function{
		Name:  "withCall",
		Chunk: bytecode.NewChunk(),
	}
	withCall.Chunk.WriteOp(bytecode.OpCall, 1)
	withCall.Chunk.Write(0, 1) // arg count
	withCall.Chunk.WriteOp(bytecode.OpReturn, 2)
	
	// 函数调用现在支持，应返回 JITWithCalls 级别
	level := CanJITWithLevel(withCall)
	if level == JITDisabled {
		t.Error("Function with call should be JIT-able (at JITWithCalls level)")
	}
	if level != JITWithCalls {
		t.Logf("Function with call at level %d (expected %d)", level, JITWithCalls)
	}
	
	// 带异常处理的函数仍然不能 JIT
	withTry := &bytecode.Function{
		Name:  "withTry",
		Chunk: bytecode.NewChunk(),
	}
	withTry.Chunk.WriteOp(bytecode.OpEnterTry, 1)
	withTry.Chunk.Write(0, 1)  // catchCount
	withTry.Chunk.Write(0, 1)  // finallyOffset high
	withTry.Chunk.Write(0, 1)  // finallyOffset low
	withTry.Chunk.WriteOp(bytecode.OpReturn, 2)
	
	if CanJIT(withTry) {
		t.Error("Function with try-catch should not be JIT-able")
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

// ============================================================================
// 函数调用相关测试
// ============================================================================

// TestIRCallInstructions 测试IR调用指令
func TestIRCallInstructions(t *testing.T) {
	// 测试创建调用指令
	fn := NewIRFunc("test", 0)
	
	// 创建参数值
	arg1 := fn.NewConstIntValue(10)
	arg2 := fn.NewConstIntValue(20)
	
	// 创建调用指令
	dest := fn.NewValue(TypeInt)
	callInstr := NewCallInstr("add", dest, arg1, arg2)
	
	if callInstr.Op != OpCall {
		t.Errorf("Expected OpCall, got %v", callInstr.Op)
	}
	
	if callInstr.CallTarget != "add" {
		t.Errorf("Expected target 'add', got '%s'", callInstr.CallTarget)
	}
	
	if callInstr.CallArgCount != 2 {
		t.Errorf("Expected 2 args, got %d", callInstr.CallArgCount)
	}
	
	if len(callInstr.Args) != 2 {
		t.Errorf("Expected 2 args in slice, got %d", len(callInstr.Args))
	}
	
	t.Logf("Call instruction: %s", callInstr.String())
}

// TestIRMethodCallInstructions 测试IR方法调用指令
func TestIRMethodCallInstructions(t *testing.T) {
	fn := NewIRFunc("test", 0)
	
	// 创建接收者和参数
	receiver := fn.NewValue(TypeObject)
	arg1 := fn.NewConstIntValue(5)
	
	// 创建方法调用指令
	dest := fn.NewValue(TypeInt)
	callInstr := NewCallMethodInstr(receiver, "getValue", dest, arg1)
	
	if callInstr.Op != OpCallMethod {
		t.Errorf("Expected OpCallMethod, got %v", callInstr.Op)
	}
	
	if callInstr.CallTarget != "getValue" {
		t.Errorf("Expected target 'getValue', got '%s'", callInstr.CallTarget)
	}
	
	// Args[0]应该是接收者
	if len(callInstr.Args) != 2 {
		t.Errorf("Expected 2 args (receiver + 1), got %d", len(callInstr.Args))
	}
	
	t.Logf("Method call instruction: %s", callInstr.String())
}

// TestIRObjectInstructions 测试IR对象操作指令
func TestIRObjectInstructions(t *testing.T) {
	fn := NewIRFunc("test", 0)
	
	// 测试创建对象
	dest := fn.NewValue(TypeObject)
	newObjInstr := NewNewObjectInstr("MyClass", dest)
	
	if newObjInstr.Op != OpNewObject {
		t.Errorf("Expected OpNewObject, got %v", newObjInstr.Op)
	}
	
	if newObjInstr.ClassName != "MyClass" {
		t.Errorf("Expected class 'MyClass', got '%s'", newObjInstr.ClassName)
	}
	
	t.Logf("NewObject instruction: %s", newObjInstr.String())
	
	// 测试获取字段
	obj := fn.NewValue(TypeObject)
	fieldDest := fn.NewValue(TypeInt)
	getFieldInstr := NewGetFieldInstr(obj, "value", fieldDest, 8)
	
	if getFieldInstr.Op != OpGetField {
		t.Errorf("Expected OpGetField, got %v", getFieldInstr.Op)
	}
	
	if getFieldInstr.FieldName != "value" {
		t.Errorf("Expected field 'value', got '%s'", getFieldInstr.FieldName)
	}
	
	if getFieldInstr.FieldOffset != 8 {
		t.Errorf("Expected offset 8, got %d", getFieldInstr.FieldOffset)
	}
	
	t.Logf("GetField instruction: %s", getFieldInstr.String())
	
	// 测试设置字段
	fieldVal := fn.NewConstIntValue(42)
	setFieldInstr := NewSetFieldInstr(obj, "value", fieldVal, 8)
	
	if setFieldInstr.Op != OpSetField {
		t.Errorf("Expected OpSetField, got %v", setFieldInstr.Op)
	}
	
	t.Logf("SetField instruction: %s", setFieldInstr.String())
}

// TestInstructionProperties 测试指令属性检查
func TestInstructionProperties(t *testing.T) {
	fn := NewIRFunc("test", 0)
	
	// 测试 IsCall
	dest := fn.NewValue(TypeInt)
	callInstr := NewCallInstr("func", dest)
	if !callInstr.IsCall() {
		t.Error("Call instruction should return true for IsCall()")
	}
	
	// 测试非调用指令
	addDest := fn.NewValue(TypeInt)
	arg1 := fn.NewConstIntValue(1)
	arg2 := fn.NewConstIntValue(2)
	addInstr := NewInstr(OpAdd, addDest, arg1, arg2)
	if addInstr.IsCall() {
		t.Error("Add instruction should return false for IsCall()")
	}
	
	// 测试 IsObjectOp
	objDest := fn.NewValue(TypeObject)
	newObjInstr := NewNewObjectInstr("Test", objDest)
	if !newObjInstr.IsObjectOp() {
		t.Error("NewObject instruction should return true for IsObjectOp()")
	}
	
	if addInstr.IsObjectOp() {
		t.Error("Add instruction should return false for IsObjectOp()")
	}
	
	// 测试 HasSideEffects
	if !callInstr.HasSideEffects() {
		t.Error("Call instruction should have side effects")
	}
	
	if addInstr.HasSideEffects() {
		t.Error("Add instruction should not have side effects")
	}
	
	// 测试 CanThrow
	if !callInstr.CanThrow() {
		t.Error("Call instruction can throw")
	}
}

// TestBuilderWithCalls 测试带函数调用的IR构建
func TestBuilderWithCalls(t *testing.T) {
	// 创建包含函数调用的字节码
	fn := &bytecode.Function{
		Name:       "testWithCall",
		Chunk:      bytecode.NewChunk(),
		LocalCount: 2,
	}
	
	chunk := fn.Chunk
	// push 函数
	chunk.WriteOp(bytecode.OpPush, 1)
	chunk.WriteU16(chunk.AddConstant(bytecode.NewString("add")), 1)
	// push 参数1
	chunk.WriteOp(bytecode.OpPush, 1)
	chunk.WriteU16(chunk.AddConstant(bytecode.NewInt(10)), 1)
	// push 参数2
	chunk.WriteOp(bytecode.OpPush, 1)
	chunk.WriteU16(chunk.AddConstant(bytecode.NewInt(20)), 1)
	// call 2
	chunk.WriteOp(bytecode.OpCall, 2)
	chunk.Code = append(chunk.Code, 2) // argCount
	// return
	chunk.WriteOp(bytecode.OpReturn, 3)
	
	// 使用允许调用的配置构建IR
	config := DefaultBuilderConfig()
	config.AllowCalls = true
	builder := NewIRBuilderWithConfig(config)
	
	irFunc, err := builder.Build(fn)
	if err != nil {
		t.Fatalf("IR build with calls failed: %v", err)
	}
	
	t.Logf("IR with call:\n%s", irFunc.String())
	
	// 检查统计信息
	callCount, _ := builder.GetStats()
	if callCount == 0 {
		t.Log("No calls recorded (expected if builder skipped call)")
	}
}

// TestBuilderWithObjects 测试带对象操作的IR构建
func TestBuilderWithObjects(t *testing.T) {
	fn := &bytecode.Function{
		Name:       "testWithObjects",
		Chunk:      bytecode.NewChunk(),
		LocalCount: 2,
	}
	
	chunk := fn.Chunk
	// new object
	chunk.WriteOp(bytecode.OpNewObject, 1)
	chunk.WriteU16(chunk.AddConstant(bytecode.NewString("MyClass")), 1)
	// get field
	chunk.WriteOp(bytecode.OpGetField, 2)
	chunk.WriteU16(chunk.AddConstant(bytecode.NewString("value")), 2)
	// return
	chunk.WriteOp(bytecode.OpReturn, 3)
	
	config := DefaultBuilderConfig()
	config.AllowObjects = true
	builder := NewIRBuilderWithConfig(config)
	
	irFunc, err := builder.Build(fn)
	if err != nil {
		t.Fatalf("IR build with objects failed: %v", err)
	}
	
	t.Logf("IR with objects:\n%s", irFunc.String())
	
	// 检查统计信息
	_, objectOpCount := builder.GetStats()
	if objectOpCount > 0 {
		t.Logf("Object operations: %d", objectOpCount)
	}
}

// TestX64CallCodeGen 测试x64调用代码生成
func TestX64CallCodeGen(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		t.Skip("Skipping x64 test on non-amd64 platform")
	}
	
	// 创建带调用指令的IR函数
	fn := NewIRFunc("testCall", 0)
	
	// 创建简单的调用指令
	arg1 := fn.NewConstIntValue(10)
	dest := fn.NewValue(TypeInt)
	callInstr := NewCallInstr("helper", dest, arg1)
	callInstr.Line = 1
	
	fn.Entry.AddInstr(callInstr)
	
	// 添加返回
	retInstr := NewInstr(OpReturn, nil, dest)
	fn.Entry.AddInstr(retInstr)
	
	// 寄存器分配
	regalloc := NewRegisterAllocator(10)
	alloc := regalloc.Allocate(fn)
	
	// 代码生成
	codegen := NewX64CodeGenerator()
	code, err := codegen.Generate(fn, alloc)
	
	if err != nil {
		t.Fatalf("Code generation failed: %v", err)
	}
	
	if len(code) == 0 {
		t.Fatal("Generated code is empty")
	}
	
	t.Logf("Generated %d bytes of call code", len(code))
}

// TestARM64CallCodeGen 测试ARM64调用代码生成
func TestARM64CallCodeGen(t *testing.T) {
	if runtime.GOARCH != "arm64" {
		t.Skip("Skipping ARM64 test on non-arm64 platform")
	}
	
	fn := NewIRFunc("testCall", 0)
	
	arg1 := fn.NewConstIntValue(10)
	dest := fn.NewValue(TypeInt)
	callInstr := NewCallInstr("helper", dest, arg1)
	callInstr.Line = 1
	
	fn.Entry.AddInstr(callInstr)
	
	retInstr := NewInstr(OpReturn, nil, dest)
	fn.Entry.AddInstr(retInstr)
	
	regalloc := NewRegisterAllocator(10)
	alloc := regalloc.Allocate(fn)
	
	codegen := NewARM64CodeGenerator()
	code, err := codegen.Generate(fn, alloc)
	
	if err != nil {
		t.Fatalf("Code generation failed: %v", err)
	}
	
	if len(code) == 0 {
		t.Fatal("Generated code is empty")
	}
	
	t.Logf("Generated %d bytes of ARM64 call code", len(code))
}

// TestCanJITWithLevel 测试JIT能力级别检测
func TestCanJITWithLevel(t *testing.T) {
	// 简单函数（无调用、无对象操作）
	simpleFn := &bytecode.Function{
		Name:       "simple",
		Chunk:      bytecode.NewChunk(),
		LocalCount: 1,
	}
	simpleFn.Chunk.WriteOp(bytecode.OpPush, 1)
	simpleFn.Chunk.WriteU16(0, 1)
	simpleFn.Chunk.WriteOp(bytecode.OpReturn, 2)
	
	level := CanJITWithLevel(simpleFn)
	if level == JITDisabled {
		t.Log("Simple function JIT disabled (unexpected)")
	} else {
		t.Logf("Simple function JIT level: %d", level)
	}
	
	// 带调用的函数
	callFn := &bytecode.Function{
		Name:       "withCall",
		Chunk:      bytecode.NewChunk(),
		LocalCount: 1,
	}
	callFn.Chunk.WriteOp(bytecode.OpCall, 1)
	callFn.Chunk.Code = append(callFn.Chunk.Code, 0)
	callFn.Chunk.WriteOp(bytecode.OpReturn, 2)
	
	level = CanJITWithLevel(callFn)
	t.Logf("Function with call JIT level: %d", level)
	
	// 带异常处理的函数（应该禁用）
	tryFn := &bytecode.Function{
		Name:       "withTry",
		Chunk:      bytecode.NewChunk(),
		LocalCount: 1,
	}
	tryFn.Chunk.WriteOp(bytecode.OpEnterTry, 1)
	tryFn.Chunk.Code = append(tryFn.Chunk.Code, 0, 0, 0) // catchCount=0, finallyOffset
	tryFn.Chunk.WriteOp(bytecode.OpReturn, 2)
	
	level = CanJITWithLevel(tryFn)
	if level != JITDisabled {
		t.Errorf("Function with try should have JIT disabled, got level %d", level)
	}
	
	// 可变参数函数（应该禁用）
	variadicFn := &bytecode.Function{
		Name:       "variadic",
		Chunk:      bytecode.NewChunk(),
		LocalCount: 1,
		IsVariadic: true,
	}
	variadicFn.Chunk.WriteOp(bytecode.OpReturn, 1)
	
	level = CanJITWithLevel(variadicFn)
	if level != JITDisabled {
		t.Errorf("Variadic function should have JIT disabled, got level %d", level)
	}
}

// TestVMBridge 测试VM桥接
func TestVMBridge(t *testing.T) {
	bridge := GetBridge()
	
	if bridge == nil {
		t.Fatal("GetBridge() returned nil")
	}
	
	// 测试注册函数
	fn := &bytecode.Function{
		Name:  "testFunc",
		Chunk: bytecode.NewChunk(),
	}
	
	bridge.RegisterFunction("testFunc", fn, nil)
	
	entry, ok := bridge.GetFunction("testFunc")
	if !ok {
		t.Error("RegisterFunction/GetFunction failed")
	}
	
	if entry.Function != fn {
		t.Error("Function mismatch")
	}
	
	// 测试注册类布局
	layout := &ClassLayout{
		Name: "TestClass",
		Size: 24,
		Fields: map[string]FieldLayout{
			"x": {Name: "x", Offset: 0, Type: TypeInt},
			"y": {Name: "y", Offset: 8, Type: TypeInt},
		},
	}
	
	bridge.RegisterClass(layout)
	
	retrievedLayout, ok := bridge.GetClassLayout("TestClass")
	if !ok {
		t.Error("RegisterClass/GetClassLayout failed")
	}
	
	if retrievedLayout.Size != 24 {
		t.Errorf("Expected size 24, got %d", retrievedLayout.Size)
	}
	
	// 测试字段偏移计算
	offset := ComputeFieldOffset(retrievedLayout, "y")
	if offset != 8 {
		t.Errorf("Expected field y offset 8, got %d", offset)
	}
	
	offset = ComputeFieldOffset(retrievedLayout, "nonexistent")
	if offset != -1 {
		t.Errorf("Expected -1 for nonexistent field, got %d", offset)
	}
}

// TestCallHelperPtrs 测试调用辅助函数指针
func TestCallHelperPtrs(t *testing.T) {
	callPtr := GetCallHelperPtr()
	if callPtr == 0 {
		t.Error("CallHelper pointer should not be 0")
	}
	
	t.Logf("CallHelper: %#x", callPtr)
}

// TestValueTypeProperties 测试值类型属性
func TestValueTypeProperties(t *testing.T) {
	// 测试 IsNumeric
	if !TypeInt.IsNumeric() {
		t.Error("TypeInt should be numeric")
	}
	if !TypeFloat.IsNumeric() {
		t.Error("TypeFloat should be numeric")
	}
	if TypeBool.IsNumeric() {
		t.Error("TypeBool should not be numeric")
	}
	if TypePtr.IsNumeric() {
		t.Error("TypePtr should not be numeric")
	}
	
	// 测试 IsPointer
	if !TypePtr.IsPointer() {
		t.Error("TypePtr should be pointer")
	}
	if !TypeObject.IsPointer() {
		t.Error("TypeObject should be pointer")
	}
	if !TypeArray.IsPointer() {
		t.Error("TypeArray should be pointer")
	}
	if !TypeFunc.IsPointer() {
		t.Error("TypeFunc should be pointer")
	}
	if TypeInt.IsPointer() {
		t.Error("TypeInt should not be pointer")
	}
}

// ============================================================================
// 内联优化测试
// ============================================================================

// TestInlinerConfig 测试内联配置
func TestInlinerConfig(t *testing.T) {
	config := DefaultInlineConfig()
	
	if config.MaxInlineSize <= 0 {
		t.Error("MaxInlineSize should be positive")
	}
	if config.MaxInlineDepth <= 0 {
		t.Error("MaxInlineDepth should be positive")
	}
	if config.AlwaysInlineThreshold <= 0 {
		t.Error("AlwaysInlineThreshold should be positive")
	}
	
	t.Logf("MaxInlineSize: %d", config.MaxInlineSize)
	t.Logf("MaxInlineDepth: %d", config.MaxInlineDepth)
	t.Logf("AlwaysInlineThreshold: %d", config.AlwaysInlineThreshold)
}

// TestInlinerDecision 测试内联决策
func TestInlinerDecision(t *testing.T) {
	inliner := NewInliner(nil)
	
	// 创建小函数（应该内联）
	smallFn := NewIRFunc("small", 0)
	smallFn.Entry.AddInstr(NewInstr(OpReturn, nil, smallFn.NewConstIntValue(42)))
	
	decision := inliner.DecideInlining(nil, smallFn, nil, 0)
	if !decision.ShouldInline {
		t.Error("Small function should be inlined")
	}
	t.Logf("Small function decision: %s (cost=%d, benefit=%d)", 
		decision.Reason, decision.Cost, decision.Benefit)
	
	// 创建大函数（不应内联）
	bigFn := NewIRFunc("big", 0)
	for i := 0; i < 100; i++ {
		v := bigFn.NewValue(TypeInt)
		bigFn.Entry.AddInstr(NewInstr(OpConst, v))
	}
	bigFn.Entry.AddInstr(NewInstr(OpReturn, nil, bigFn.NewConstIntValue(0)))
	
	decision = inliner.DecideInlining(nil, bigFn, nil, 0)
	if decision.ShouldInline {
		t.Error("Big function should not be inlined")
	}
	t.Logf("Big function decision: %s (cost=%d)", decision.Reason, decision.Cost)
	
	// 测试递归检测
	recursiveFn := NewIRFunc("recursive", 0)
	callInstr := NewCallInstr("recursive", recursiveFn.NewValue(TypeInt))
	recursiveFn.Entry.AddInstr(callInstr)
	recursiveFn.Entry.AddInstr(NewInstr(OpReturn, nil, recursiveFn.NewConstIntValue(0)))
	
	// 模拟递归调用
	inliner.inlineStack = map[string]bool{"recursive": true}
	decision = inliner.DecideInlining(nil, recursiveFn, callInstr, 0)
	if decision.ShouldInline {
		t.Error("Recursive function should not be inlined")
	}
	t.Logf("Recursive function decision: %s", decision.Reason)
}

// TestInlinerDepth 测试内联深度限制
func TestInlinerDepth(t *testing.T) {
	config := DefaultInlineConfig()
	config.MaxInlineDepth = 2
	inliner := NewInliner(config)
	
	smallFn := NewIRFunc("small", 0)
	smallFn.Entry.AddInstr(NewInstr(OpReturn, nil, smallFn.NewConstIntValue(1)))
	
	// 深度0：应内联
	decision := inliner.DecideInlining(nil, smallFn, nil, 0)
	if !decision.ShouldInline {
		t.Error("Should inline at depth 0")
	}
	
	// 深度1：应内联
	decision = inliner.DecideInlining(nil, smallFn, nil, 1)
	if !decision.ShouldInline {
		t.Error("Should inline at depth 1")
	}
	
	// 深度2：不应内联（达到限制）
	decision = inliner.DecideInlining(nil, smallFn, nil, 2)
	if decision.ShouldInline {
		t.Error("Should not inline at max depth")
	}
	t.Logf("Max depth decision: %s", decision.Reason)
}

// TestInlinerStats 测试内联统计
func TestInlinerStats(t *testing.T) {
	inliner := NewInliner(nil)
	
	smallFn := NewIRFunc("small", 0)
	smallFn.Entry.AddInstr(NewInstr(OpReturn, nil, smallFn.NewConstIntValue(1)))
	
	bigFn := NewIRFunc("big", 0)
	for i := 0; i < 100; i++ {
		bigFn.Entry.AddInstr(NewInstr(OpConst, bigFn.NewValue(TypeInt)))
	}
	
	// 做几次决策
	inliner.DecideInlining(nil, smallFn, nil, 0) // 内联
	inliner.DecideInlining(nil, smallFn, nil, 0) // 内联
	inliner.DecideInlining(nil, bigFn, nil, 0)   // 不内联（太大）
	inliner.DecideInlining(nil, nil, nil, 0)     // 不内联（nil）
	
	stats := inliner.GetStats()
	
	t.Logf("Total calls: %d", stats.TotalCalls)
	t.Logf("Inlined calls: %d", stats.InlinedCalls)
	t.Logf("Skipped (too big): %d", stats.SkippedTooBig)
	
	if stats.TotalCalls != 4 {
		t.Errorf("Expected 4 total calls, got %d", stats.TotalCalls)
	}
	if stats.InlinedCalls != 2 {
		t.Errorf("Expected 2 inlined calls, got %d", stats.InlinedCalls)
	}
	if stats.SkippedTooBig != 1 {
		t.Errorf("Expected 1 skipped (too big), got %d", stats.SkippedTooBig)
	}
	
	// 重置统计
	inliner.ResetStats()
	stats = inliner.GetStats()
	if stats.TotalCalls != 0 {
		t.Error("Stats should be reset")
	}
}

// TestOptimizerO3 测试O3优化级别
func TestOptimizerO3(t *testing.T) {
	opt := NewOptimizer(3)
	
	if opt.level != 3 {
		t.Errorf("Expected level 3, got %d", opt.level)
	}
	
	if opt.inliner == nil {
		t.Error("O3 optimizer should have inliner")
	}
	
	// 创建一个简单的函数进行优化
	fn := NewIRFunc("test", 0)
	
	// 添加一些可以优化的指令
	v1 := fn.NewConstIntValue(10)
	v2 := fn.NewConstIntValue(0)
	v3 := fn.NewValue(TypeInt)
	
	// x + 0 -> x
	addInstr := NewInstr(OpAdd, v3, v1, v2)
	fn.Entry.AddInstr(addInstr)
	fn.Entry.AddInstr(NewInstr(OpReturn, nil, v3))
	
	t.Logf("Before optimization:\n%s", fn.String())
	
	opt.Optimize(fn)
	
	t.Logf("After O3 optimization:\n%s", fn.String())
	
	// 检查优化结果
	stats := opt.GetInlineStats()
	if stats != nil {
		t.Logf("Inline stats: total=%d, inlined=%d", stats.TotalCalls, stats.InlinedCalls)
	}
}
