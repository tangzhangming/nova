package jit

import (
	"runtime"
	"testing"

	"github.com/tangzhangming/nova/internal/bytecode"
	"github.com/tangzhangming/nova/internal/jit/types"
)

// TestNewJITCompiler 测试 JIT 编译器创建
func TestNewJITCompiler(t *testing.T) {
	jc := NewJITCompiler()
	
	arch := runtime.GOARCH
	if arch != "amd64" && arch != "arm64" {
		if jc != nil {
			t.Error("Expected nil JIT compiler for unsupported platform")
		}
		return
	}
	
	if jc == nil {
		t.Error("Expected non-nil JIT compiler")
		return
	}
	
	if jc.GetPlatform() != arch {
		t.Errorf("Expected platform %s, got %s", arch, jc.GetPlatform())
	}
}

// TestIRBuilder 测试 IR 构建器
func TestIRBuilder(t *testing.T) {
	// 创建简单的字节码函数：返回 1 + 2
	chunk := bytecode.NewChunk()
	
	// PUSH 1
	chunk.WriteOp(bytecode.OpPush, 1)
	idx1 := chunk.AddConstant(bytecode.NewInt(1))
	chunk.WriteU16(idx1, 1)
	
	// PUSH 2
	chunk.WriteOp(bytecode.OpPush, 1)
	idx2 := chunk.AddConstant(bytecode.NewInt(2))
	chunk.WriteU16(idx2, 1)
	
	// ADD
	chunk.WriteOp(bytecode.OpAdd, 1)
	
	// RETURN
	chunk.WriteOp(bytecode.OpReturn, 1)
	
	fn := &bytecode.Function{
		Name:       "add",
		Arity:      0,
		LocalCount: 0,
		Chunk:      chunk,
	}
	
	builder := NewIRBuilder()
	irFn := builder.BuildFunction(fn)
	
	if irFn == nil {
		t.Fatal("Expected non-nil IR function")
	}
	
	if irFn.Name != "add" {
		t.Errorf("Expected function name 'add', got '%s'", irFn.Name)
	}
	
	if len(irFn.Blocks) == 0 {
		t.Error("Expected at least one basic block")
	}
	
	// 检查指令
	entry := irFn.Entry
	if entry == nil {
		t.Fatal("Expected non-nil entry block")
	}
	
	// 应该有：2 x LoadConst + Add + Return = 4 条指令
	if len(entry.Instrs) < 4 {
		t.Errorf("Expected at least 4 instructions, got %d", len(entry.Instrs))
	}
}

// TestRegisterAllocator 测试寄存器分配
func TestRegisterAllocator(t *testing.T) {
	// 创建一个简单的 IR 函数
	fn := &types.IRFunction{
		Name:     "test",
		NumVRegs: 5,
	}
	
	entry := &types.IRBlock{ID: 0, Entry: true}
	fn.Entry = entry
	fn.Blocks = []*types.IRBlock{entry}
	
	// 添加一些指令
	entry.Instrs = []*types.IRInstr{
		{Op: types.IRLoadConst, Dest: 0},
		{Op: types.IRLoadConst, Dest: 1},
		{Op: types.IRAdd, Dest: 2, Args: []int{0, 1}},
		{Op: types.IRReturn, Dest: -1, Args: []int{2}},
	}
	
	ra := NewRegisterAllocator(fn)
	ra.Allocate()
	
	alloc := ra.ToAllocation()
	
	// 检查分配结果
	if alloc == nil {
		t.Fatal("Expected non-nil allocation")
	}
	
	// 至少应该有一些寄存器被分配
	if len(alloc.Allocated) == 0 && len(alloc.Spilled) == 0 {
		t.Error("Expected some registers to be allocated or spilled")
	}
}

// TestOptimizer 测试优化器
func TestOptimizer(t *testing.T) {
	fn := &types.IRFunction{
		Name:     "test",
		NumVRegs: 3,
	}
	
	entry := &types.IRBlock{ID: 0, Entry: true}
	fn.Entry = entry
	fn.Blocks = []*types.IRBlock{entry}
	
	// 创建一些可优化的指令
	entry.Instrs = []*types.IRInstr{
		{Op: types.IRLoadConst, Dest: 0, Immediate: bytecode.NewInt(10)},
		{Op: types.IRLoadConst, Dest: 1, Immediate: bytecode.NewInt(20)},
		{Op: types.IRAdd, Dest: 2, Args: []int{0, 1}}, // 未被使用
		{Op: types.IRReturn, Dest: -1},
	}
	
	opt := NewOptimizer(fn)
	opt.Optimize()
	
	// 死代码消除应该移除未使用的 Add 指令
	// 但由于 Load 指令仍被引用，优化后可能仍有指令
	if len(entry.Instrs) == 0 {
		t.Error("Optimization should not remove all instructions")
	}
}

// TestCodeGeneration 测试代码生成（仅在支持的平台）
func TestCodeGeneration(t *testing.T) {
	jc := NewJITCompiler()
	if jc == nil {
		t.Skip("JIT not supported on this platform")
		return
	}
	
	// 创建简单函数
	chunk := bytecode.NewChunk()
	
	// PUSH 42
	chunk.WriteOp(bytecode.OpPush, 1)
	idx := chunk.AddConstant(bytecode.NewInt(42))
	chunk.WriteU16(idx, 1)
	
	// RETURN
	chunk.WriteOp(bytecode.OpReturn, 1)
	
	fn := &bytecode.Function{
		Name:       "returnFortyTwo",
		Arity:      0,
		LocalCount: 0,
		Chunk:      chunk,
	}
	
	compiled, err := jc.CompileFunction(fn)
	if err != nil {
		t.Fatalf("Compilation failed: %v", err)
	}
	
	if compiled == nil {
		t.Fatal("Expected non-nil compiled function")
	}
	
	if len(compiled.MachineCode) == 0 {
		t.Error("Expected non-empty machine code")
	}
	
	t.Logf("Generated %d bytes of machine code", len(compiled.MachineCode))
}

// TestArithmeticCodeGen 测试算术运算代码生成
func TestArithmeticCodeGen(t *testing.T) {
	jc := NewJITCompiler()
	if jc == nil {
		t.Skip("JIT not supported on this platform")
		return
	}
	
	// 创建函数：计算 (10 + 20) * 2
	chunk := bytecode.NewChunk()
	
	// PUSH 10
	chunk.WriteOp(bytecode.OpPush, 1)
	idx1 := chunk.AddConstant(bytecode.NewInt(10))
	chunk.WriteU16(idx1, 1)
	
	// PUSH 20
	chunk.WriteOp(bytecode.OpPush, 1)
	idx2 := chunk.AddConstant(bytecode.NewInt(20))
	chunk.WriteU16(idx2, 1)
	
	// ADD
	chunk.WriteOp(bytecode.OpAdd, 1)
	
	// PUSH 2
	chunk.WriteOp(bytecode.OpPush, 1)
	idx3 := chunk.AddConstant(bytecode.NewInt(2))
	chunk.WriteU16(idx3, 1)
	
	// MUL
	chunk.WriteOp(bytecode.OpMul, 1)
	
	// RETURN
	chunk.WriteOp(bytecode.OpReturn, 1)
	
	fn := &bytecode.Function{
		Name:       "arithmetic",
		Arity:      0,
		LocalCount: 0,
		Chunk:      chunk,
	}
	
	compiled, err := jc.CompileFunction(fn)
	if err != nil {
		t.Fatalf("Compilation failed: %v", err)
	}
	
	if len(compiled.MachineCode) == 0 {
		t.Error("Expected non-empty machine code")
	}
	
	t.Logf("Arithmetic function: %d bytes of machine code", len(compiled.MachineCode))
}

// TestCodeCache 测试代码缓存
func TestCodeCache(t *testing.T) {
	cache := NewCodeCache(1024)
	
	// 分配一些内存
	mem1, err := cache.AllocateExecutable(100)
	if err != nil {
		t.Fatalf("Failed to allocate: %v", err)
	}
	
	if len(mem1) != 100 {
		t.Errorf("Expected 100 bytes, got %d", len(mem1))
	}
	
	// 存储和获取函数
	compiled := &CompiledFunction{
		Name:        "test",
		MachineCode: mem1,
		StackSize:   16,
	}
	
	cache.Put("test", compiled)
	
	retrieved, ok := cache.Get("test")
	if !ok {
		t.Error("Expected to find cached function")
	}
	
	if retrieved.Name != "test" {
		t.Errorf("Expected name 'test', got '%s'", retrieved.Name)
	}
	
	// 测试缓存满
	_, err = cache.AllocateExecutable(2000) // 超过缓存大小
	if err == nil {
		t.Error("Expected error when cache is full")
	}
}

// BenchmarkIRBuild 基准测试：IR 构建
func BenchmarkIRBuild(b *testing.B) {
	chunk := bytecode.NewChunk()
	
	for i := 0; i < 10; i++ {
		chunk.WriteOp(bytecode.OpPush, 1)
		idx := chunk.AddConstant(bytecode.NewInt(int64(i)))
		chunk.WriteU16(idx, 1)
	}
	
	for i := 0; i < 9; i++ {
		chunk.WriteOp(bytecode.OpAdd, 1)
	}
	
	chunk.WriteOp(bytecode.OpReturn, 1)
	
	fn := &bytecode.Function{
		Name:       "benchmark",
		Arity:      0,
		LocalCount: 0,
		Chunk:      chunk,
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		builder := NewIRBuilder()
		builder.BuildFunction(fn)
	}
}

// BenchmarkJITCompile 基准测试：JIT 编译
func BenchmarkJITCompile(b *testing.B) {
	jc := NewJITCompiler()
	if jc == nil {
		b.Skip("JIT not supported on this platform")
		return
	}
	
	chunk := bytecode.NewChunk()
	
	chunk.WriteOp(bytecode.OpPush, 1)
	idx := chunk.AddConstant(bytecode.NewInt(42))
	chunk.WriteU16(idx, 1)
	chunk.WriteOp(bytecode.OpReturn, 1)
	
	fn := &bytecode.Function{
		Name:       "benchmark",
		Arity:      0,
		LocalCount: 0,
		Chunk:      chunk,
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		// 每次创建新的编译器以避免缓存影响
		jc := NewJITCompiler()
		jc.CompileFunction(fn)
	}
}

