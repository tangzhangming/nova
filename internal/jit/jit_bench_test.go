package jit

import (
	"runtime"
	"testing"

	"github.com/tangzhangming/nova/internal/bytecode"
)

// ============================================================================
// JIT 基准测试
// ============================================================================
//
// 运行基准测试：
//   go test -bench=. -benchmem -vet=off ./internal/jit/...
//
// 运行特定测试：
//   go test -bench=BenchmarkJIT -benchmem -vet=off ./internal/jit/...
//
// ============================================================================

// createSimpleAddFunc 创建简单加法函数: func add(a, b int) int { return a + b }
func createSimpleAddFunc() *bytecode.Function {
	fn := &bytecode.Function{
		Name:       "simpleAdd",
		Arity:      2,
		MinArity:   2,
		Chunk:      bytecode.NewChunk(),
		LocalCount: 3,
	}

	chunk := fn.Chunk
	// load local 1 (first arg)
	chunk.WriteOp(bytecode.OpLoadLocal, 1)
	chunk.WriteU16(1, 1)
	// load local 2 (second arg)
	chunk.WriteOp(bytecode.OpLoadLocal, 1)
	chunk.WriteU16(2, 1)
	// add
	chunk.WriteOp(bytecode.OpAdd, 2)
	// return
	chunk.WriteOp(bytecode.OpReturn, 3)

	return fn
}

// createArithmeticFunc 创建复杂算术函数
// func compute(a, b int) int { return (a + b) * (a - b) }
func createArithmeticFunc() *bytecode.Function {
	fn := &bytecode.Function{
		Name:       "compute",
		Arity:      2,
		MinArity:   2,
		Chunk:      bytecode.NewChunk(),
		LocalCount: 3,
	}

	chunk := fn.Chunk
	// load a
	chunk.WriteOp(bytecode.OpLoadLocal, 1)
	chunk.WriteU16(1, 1)
	// load b
	chunk.WriteOp(bytecode.OpLoadLocal, 1)
	chunk.WriteU16(2, 1)
	// a + b
	chunk.WriteOp(bytecode.OpAdd, 2)

	// load a again
	chunk.WriteOp(bytecode.OpLoadLocal, 1)
	chunk.WriteU16(1, 1)
	// load b again
	chunk.WriteOp(bytecode.OpLoadLocal, 1)
	chunk.WriteU16(2, 1)
	// a - b
	chunk.WriteOp(bytecode.OpSub, 2)

	// (a+b) * (a-b)
	chunk.WriteOp(bytecode.OpMul, 3)
	// return
	chunk.WriteOp(bytecode.OpReturn, 4)

	return fn
}

// createConstantFoldingFunc 创建可以进行常量折叠的函数
// func constants() int { return 1 + 2 + 3 + 4 + 5 }
func createConstantFoldingFunc() *bytecode.Function {
	fn := &bytecode.Function{
		Name:       "constants",
		Arity:      0,
		MinArity:   0,
		Chunk:      bytecode.NewChunk(),
		LocalCount: 1,
	}

	chunk := fn.Chunk
	// 添加常量到常量池
	chunk.AddConstant(bytecode.NewInt(1))
	chunk.AddConstant(bytecode.NewInt(2))
	chunk.AddConstant(bytecode.NewInt(3))
	chunk.AddConstant(bytecode.NewInt(4))
	chunk.AddConstant(bytecode.NewInt(5))

	// push 1
	chunk.WriteOp(bytecode.OpPush, 1)
	chunk.WriteU16(0, 1)
	// push 2
	chunk.WriteOp(bytecode.OpPush, 1)
	chunk.WriteU16(1, 1)
	// add
	chunk.WriteOp(bytecode.OpAdd, 2)
	// push 3
	chunk.WriteOp(bytecode.OpPush, 1)
	chunk.WriteU16(2, 1)
	// add
	chunk.WriteOp(bytecode.OpAdd, 2)
	// push 4
	chunk.WriteOp(bytecode.OpPush, 1)
	chunk.WriteU16(3, 1)
	// add
	chunk.WriteOp(bytecode.OpAdd, 2)
	// push 5
	chunk.WriteOp(bytecode.OpPush, 1)
	chunk.WriteU16(4, 1)
	// add
	chunk.WriteOp(bytecode.OpAdd, 2)
	// return
	chunk.WriteOp(bytecode.OpReturn, 3)

	return fn
}

// BenchmarkJITCompile 测试 JIT 编译性能
func BenchmarkJITCompile(b *testing.B) {
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		b.Skip("JIT not supported on this architecture")
	}

	fn := createSimpleAddFunc()
	config := DefaultConfig()
	config.OptimizationLevel = 1

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		compiler := NewCompiler(config)
		_, err := compiler.Compile(fn)
		if err != nil {
			b.Fatalf("compilation failed: %v", err)
		}
	}
}

// BenchmarkJITCompileOptimized 测试带优化的 JIT 编译性能
func BenchmarkJITCompileOptimized(b *testing.B) {
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		b.Skip("JIT not supported on this architecture")
	}

	fn := createConstantFoldingFunc()
	config := DefaultConfig()
	config.OptimizationLevel = 2

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		compiler := NewCompiler(config)
		_, err := compiler.Compile(fn)
		if err != nil {
			b.Fatalf("compilation failed: %v", err)
		}
	}
}

// BenchmarkJITExecuteSimpleAdd 测试 JIT 执行简单加法
func BenchmarkJITExecuteSimpleAdd(b *testing.B) {
	if runtime.GOARCH != "amd64" {
		b.Skip("JIT execution only supported on amd64")
	}

	fn := createSimpleAddFunc()
	config := DefaultConfig()
	config.OptimizationLevel = 1

	compiler := NewCompiler(config)
	compiled, err := compiler.Compile(fn)
	if err != nil {
		b.Fatalf("compilation failed: %v", err)
	}

	entryPoint := compiled.EntryPoint()
	if entryPoint == 0 {
		b.Skip("Could not get entry point")
	}

	args := []int64{10, 20}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		result, ok := CallNative(entryPoint, args)
		if !ok {
			b.Fatal("JIT execution failed")
		}
		_ = result
	}
}

// BenchmarkJITExecuteArithmetic 测试 JIT 执行复杂算术
func BenchmarkJITExecuteArithmetic(b *testing.B) {
	if runtime.GOARCH != "amd64" {
		b.Skip("JIT execution only supported on amd64")
	}

	fn := createArithmeticFunc()
	config := DefaultConfig()
	config.OptimizationLevel = 1

	compiler := NewCompiler(config)
	compiled, err := compiler.Compile(fn)
	if err != nil {
		b.Fatalf("compilation failed: %v", err)
	}

	entryPoint := compiled.EntryPoint()
	if entryPoint == 0 {
		b.Skip("Could not get entry point")
	}

	args := []int64{100, 50}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		result, ok := CallNative(entryPoint, args)
		if !ok {
			b.Fatal("JIT execution failed")
		}
		_ = result
	}
}

// BenchmarkIRBuild 测试 IR 构建性能
func BenchmarkIRBuild(b *testing.B) {
	fn := createArithmeticFunc()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		builder := NewIRBuilder()
		_, err := builder.Build(fn)
		if err != nil {
			b.Fatalf("IR build failed: %v", err)
		}
	}
}

// BenchmarkOptimizer 测试优化器性能
func BenchmarkOptimizer(b *testing.B) {
	fn := createConstantFoldingFunc()
	builder := NewIRBuilder()
	irFunc, err := builder.Build(fn)
	if err != nil {
		b.Fatalf("IR build failed: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// 复制 IR 以避免重复优化
		optimizer := NewOptimizer(2)
		optimizer.Optimize(irFunc)
	}
}

// BenchmarkRegisterAllocation 测试寄存器分配性能
func BenchmarkRegisterAllocation(b *testing.B) {
	fn := createArithmeticFunc()
	builder := NewIRBuilder()
	irFunc, err := builder.Build(fn)
	if err != nil {
		b.Fatalf("IR build failed: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		regalloc := NewRegisterAllocator(9)
		_ = regalloc.Allocate(irFunc)
	}
}

// BenchmarkCodeGeneration 测试代码生成性能
func BenchmarkCodeGeneration(b *testing.B) {
	if runtime.GOARCH != "amd64" {
		b.Skip("x64 code generation only on amd64")
	}

	fn := createArithmeticFunc()
	builder := NewIRBuilder()
	irFunc, err := builder.Build(fn)
	if err != nil {
		b.Fatalf("IR build failed: %v", err)
	}

	regalloc := NewRegisterAllocator(9)
	allocation := regalloc.Allocate(irFunc)

	codegen := NewX64CodeGenerator()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := codegen.Generate(irFunc, allocation)
		if err != nil {
			b.Fatalf("Code generation failed: %v", err)
		}
	}
}

// BenchmarkX64Assembler 测试 x86-64 汇编器性能
func BenchmarkX64Assembler(b *testing.B) {
	if runtime.GOARCH != "amd64" {
		b.Skip("x64 assembler only on amd64")
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		asm := NewX64Assembler()

		// 模拟生成一些指令
		asm.Push(RBP)
		asm.MovRegReg(RBP, RSP)
		asm.SubRegImm32(RSP, 32)

		asm.MovRegImm64(RAX, 10)
		asm.MovRegImm64(RCX, 20)
		asm.AddRegReg(RAX, RCX)

		asm.MovRegReg(RSP, RBP)
		asm.Pop(RBP)
		asm.Ret()

		_ = asm.Code()
	}
}
