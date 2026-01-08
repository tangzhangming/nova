// jit_test.go - JIT 编译器测试
//
// 这些测试验证 JIT 编译器的基本功能

package jit

import (
	"runtime"
	"testing"

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
	fn := &bytecode.Function{
		Name:       "simpleAdd",
		Arity:      2,
		Chunk:      bytecode.NewChunk(),
		LocalCount: 2,
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
