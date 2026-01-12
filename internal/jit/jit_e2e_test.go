// +build amd64

package jit

import (
	"fmt"
	"runtime"
	"syscall"
	"testing"
	"unsafe"

	"github.com/tangzhangming/nova/internal/bytecode"
)

// ============================================================================
// JIT 端到端测试
// 验证完整的 JIT 编译和执行流程
// ============================================================================

// TestJIT_E2E_MemoryAlloc 测试可执行内存分配
func TestJIT_E2E_MemoryAlloc(t *testing.T) {
	// 测试可执行内存分配
	buf, err := NewExecutableBuffer(4096)
	if err != nil {
		t.Fatalf("Failed to allocate executable buffer: %v", err)
	}
	defer buf.Free()

	t.Logf("Buffer allocated at: %p, size=%d", unsafe.Pointer(buf.Addr()), buf.Size())

	// 写入最简单的代码：mov eax, 42; ret
	// 注意：不使用 REX.W 前缀，32 位操作会零扩展到 64 位
	simpleCode := []byte{
		0xB8, 0x2A, 0x00, 0x00, 0x00, // mov eax, 42
		0xC3,                         // ret
	}

	addr, err := buf.Write(simpleCode)
	if err != nil {
		t.Fatalf("Failed to write code: %v", err)
	}
	t.Logf("Code written at: %p", unsafe.Pointer(addr))

	// 打印代码字节
	t.Logf("Code bytes: %X", simpleCode)
}

// TestJIT_E2E_SimpleReturn 测试简单返回值
func TestJIT_E2E_SimpleReturn(t *testing.T) {
	// 直接使用 ExecutableBuffer 而不是 GlobalCodeCache
	buf, err := NewExecutableBuffer(4096)
	if err != nil {
		t.Fatalf("Failed to allocate executable buffer: %v", err)
	}
	defer buf.Free()

	// 生成最简单的代码：返回 42
	// mov eax, 42 (32位操作会零扩展)
	// ret
	simpleCode := []byte{
		0xB8, 0x2A, 0x00, 0x00, 0x00, // mov eax, 42
		0xC3,                         // ret
	}

	entry, err := buf.Write(simpleCode)
	if err != nil {
		t.Fatalf("Failed to write code: %v", err)
	}
	t.Logf("Code installed at: %p", unsafe.Pointer(entry))
	t.Logf("Code bytes: %X", simpleCode)

	// 调用生成的代码
	result := callJITCode0(entry)
	t.Logf("JIT call result: %d", result)

	if result != 42 {
		t.Errorf("Expected 42, got %d", result)
	}
}

// TestJIT_E2E_Addition 测试加法运算
func TestJIT_E2E_Addition(t *testing.T) {
	// 生成一个加法函数：返回 10 + 32 = 42
	cg := NewCodeGenerator()

	cg.EmitPrologue(0)

	// MOV RAX, 10
	cg.EmitMovRegImm64(RAX, 10)
	// MOV RBX, 32
	cg.EmitMovRegImm64(RBX, 32)
	// ADD RAX, RBX
	cg.EmitAddRegReg(RAX, RBX)

	cg.EmitEpilogue()

	code := cg.Code()
	entry, err := GlobalCodeCache.Install(code)
	if err != nil {
		t.Fatalf("Failed to install code: %v", err)
	}

	result := callJITCode0(entry)
	t.Logf("10 + 32 = %d", result)

	if result != 42 {
		t.Errorf("Expected 42, got %d", result)
	}
}

// TestJIT_E2E_Subtraction 测试减法运算
func TestJIT_E2E_Subtraction(t *testing.T) {
	cg := NewCodeGenerator()

	cg.EmitPrologue(0)

	// MOV RAX, 100
	cg.EmitMovRegImm64(RAX, 100)
	// MOV RBX, 58
	cg.EmitMovRegImm64(RBX, 58)
	// SUB RAX, RBX
	cg.EmitSubRegReg(RAX, RBX)

	cg.EmitEpilogue()

	code := cg.Code()
	entry, err := GlobalCodeCache.Install(code)
	if err != nil {
		t.Fatalf("Failed to install code: %v", err)
	}

	result := callJITCode0(entry)
	t.Logf("100 - 58 = %d", result)

	if result != 42 {
		t.Errorf("Expected 42, got %d", result)
	}
}

// TestJIT_E2E_Multiplication 测试乘法运算
func TestJIT_E2E_Multiplication(t *testing.T) {
	cg := NewCodeGenerator()

	cg.EmitPrologue(0)

	// MOV RAX, 6
	cg.EmitMovRegImm64(RAX, 6)
	// MOV RBX, 7
	cg.EmitMovRegImm64(RBX, 7)
	// IMUL RAX, RBX
	cg.EmitImulRegReg(RAX, RBX)

	cg.EmitEpilogue()

	code := cg.Code()
	entry, err := GlobalCodeCache.Install(code)
	if err != nil {
		t.Fatalf("Failed to install code: %v", err)
	}

	result := callJITCode0(entry)
	t.Logf("6 * 7 = %d", result)

	if result != 42 {
		t.Errorf("Expected 42, got %d", result)
	}
}

// TestJIT_E2E_WithArgument 测试带参数的函数
func TestJIT_E2E_WithArgument(t *testing.T) {
	// 生成函数：返回 arg + 1
	cg := NewCodeGenerator()

	cg.EmitPrologue(0)

	// Windows x64 调用约定：第一个参数在 RCX
	// MOV RAX, RCX (将参数复制到返回值寄存器)
	cg.EmitMovRegReg(RAX, RCX)
	// ADD RAX, 1
	cg.EmitAddRegImm32(RAX, 1)

	cg.EmitEpilogue()

	code := cg.Code()
	entry, err := GlobalCodeCache.Install(code)
	if err != nil {
		t.Fatalf("Failed to install code: %v", err)
	}

	result := callJITCode1(entry, 41)
	t.Logf("arg(41) + 1 = %d", result)

	if result != 42 {
		t.Errorf("Expected 42, got %d", result)
	}
}

// TestJIT_E2E_Loop 测试循环 (累加 1 到 N)
func TestJIT_E2E_Loop(t *testing.T) {
	// 生成简单循环：计算 1+2+3+...+N
	// 使用最少的寄存器和简单的跳转
	cg := NewCodeGenerator()

	// 简化：不使用 Prologue，直接操作寄存器
	// Windows x64: 参数 N 在 RCX，返回值在 RAX

	// MOV R8, RCX (保存 N 到 R8)
	cg.EmitMovRegReg(R8, RCX)
	// XOR RAX, RAX (sum = 0)
	cg.EmitXorRegReg(RAX, RAX)
	// XOR R9, R9 (i = 0)
	cg.EmitXorRegReg(R9, R9)

	// 循环开始
	loopStart := cg.CurrentOffset()

	// INC R9 (i++)
	cg.EmitAddRegImm32(R9, 1)

	// ADD RAX, R9 (sum += i)
	cg.EmitAddRegReg(RAX, R9)

	// CMP R9, R8 (i < N?)
	cg.EmitCmpRegReg(R9, R8)

	// JL loopStart (如果 i < N，继续循环)
	rel := int32(loopStart - (cg.CurrentOffset() + 6)) // 6 = JL 指令长度
	cg.EmitJl(rel)

	// RET
	cg.EmitRet()

	code := cg.Code()
	t.Logf("Generated code size: %d bytes", len(code))

	entry, err := GlobalCodeCache.Install(code)
	if err != nil {
		t.Fatalf("Failed to install code: %v", err)
	}

	// 测试 sum(10) = 55
	result := callJITCode1(entry, 10)
	t.Logf("sum(1..10) = %d", result)

	if result != 55 {
		t.Errorf("Expected 55, got %d", result)
	}

	// 测试 sum(100) = 5050
	result = callJITCode1(entry, 100)
	t.Logf("sum(1..100) = %d", result)

	if result != 5050 {
		t.Errorf("Expected 5050, got %d", result)
	}
}

// TestJIT_E2E_Comparison 测试比较运算
func TestJIT_E2E_Comparison(t *testing.T) {
	tests := []struct {
		name     string
		a, b     int64
		op       string
		expected int64
	}{
		{"10 < 20", 10, 20, "lt", 1},
		{"20 < 10", 20, 10, "lt", 0},
		{"10 == 10", 10, 10, "eq", 1},
		{"10 == 20", 10, 20, "eq", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cg := NewCodeGenerator()
			cg.EmitPrologue(0)

			// MOV RAX, a
			cg.EmitMovRegImm64(RAX, uint64(tt.a))
			// MOV RBX, b
			cg.EmitMovRegImm64(RBX, uint64(tt.b))
			// CMP RAX, RBX
			cg.EmitCmpRegReg(RAX, RBX)

			switch tt.op {
			case "lt":
				cg.EmitSetl(RAX)
			case "eq":
				cg.EmitSete(RAX)
			}

			// MOVZX RAX, AL
			cg.EmitMovzxByte(RAX, RAX)

			cg.EmitEpilogue()

			code := cg.Code()
			entry, err := GlobalCodeCache.Install(code)
			if err != nil {
				t.Fatalf("Failed to install code: %v", err)
			}

			result := callJITCode0(entry)
			if result != tt.expected {
				t.Errorf("Expected %d, got %d", tt.expected, result)
			}
		})
	}
}

// TestJIT_E2E_IRToMachineCode 测试 IR 到机器码的完整流程
func TestJIT_E2E_IRToMachineCode(t *testing.T) {
	// 创建 IR：10 + 20 = 30
	ir := []IRInst{
		{Op: IR_CONST, Value: bytecode.NewInt(10)},
		{Op: IR_CONST, Value: bytecode.NewInt(20)},
		{Op: IR_ADD},
		{Op: IR_RETURN},
	}

	// 生成机器码
	code, err := GenerateMachineCodeFromInsts(ir, 0, nil)
	if err != nil {
		t.Fatalf("Code generation failed: %v", err)
	}

	t.Logf("Generated %d bytes from IR", len(code))

	// 验证代码非空
	if len(code) == 0 {
		t.Error("Generated code is empty")
	}
}

// TestJIT_E2E_FullCompilation 测试完整的编译流程
func TestJIT_E2E_FullCompilation(t *testing.T) {
	compiler := NewCompiler(DefaultConfig())

	// 创建简单函数：返回常量 42
	fn := &bytecode.Function{
		Name:       "test",
		Arity:      0,
		LocalCount: 0,
		Chunk: &bytecode.Chunk{
			Code: []byte{
				byte(bytecode.OpPush), 0, 0, // 加载常量 0 (值 42)
				byte(bytecode.OpReturn),
			},
			Constants: []bytecode.Value{
				bytecode.NewInt(42),
			},
		},
	}

	// 编译
	compiled, err := compiler.Compile(fn)
	if err != nil {
		t.Fatalf("Compilation failed: %v", err)
	}

	t.Logf("Compiled: %d IR instructions, %d bytes machine code",
		len(compiled.IRInsts), len(compiled.Code))

	// 验证编译结果
	if len(compiled.IRInsts) == 0 {
		t.Error("No IR instructions generated")
	}
	if len(compiled.Code) == 0 {
		t.Error("No machine code generated")
	}
}

// TestJIT_E2E_ExecutorIntegration 测试执行器集成
func TestJIT_E2E_ExecutorIntegration(t *testing.T) {
	executor := NewExecutor()
	executor.SetEnabled(true)

	fn := &bytecode.Function{
		Name:       "integration_test",
		Arity:      0,
		LocalCount: 0,
		Chunk: &bytecode.Chunk{
			Code: []byte{
				byte(bytecode.OpPush), 0, 0,
				byte(bytecode.OpReturn),
			},
			Constants: []bytecode.Value{
				bytecode.NewInt(100),
			},
		},
	}

	// 编译
	installed, err := executor.Compile(fn)
	if err != nil {
		t.Fatalf("Compilation failed: %v", err)
	}

	// 验证已安装代码
	if installed == nil {
		t.Fatal("Installed code is nil")
	}
	if installed.Function != fn {
		t.Error("Function mismatch")
	}
	if installed.Code == nil {
		t.Error("Compiled code is nil")
	}

	t.Logf("Function compiled and installed successfully")
	t.Logf("Entry point: %p", unsafe.Pointer(installed.Code.Entry))
	t.Logf("Code size: %d bytes", len(installed.Code.Code))

	// 获取统计
	stats := executor.GetStats()
	t.Logf("Stats: compiled=%d, code_used=%d bytes",
		stats.CompiledFuncs, stats.CodeUsed)

	// 清理
	executor.Reset()
}

// TestJIT_E2E_CodeCacheStats 测试代码缓存统计
func TestJIT_E2E_CodeCacheStats(t *testing.T) {
	// 获取初始状态
	totalBefore, usedBefore := GlobalCodeCache.Stats()
	t.Logf("Before: total=%d, used=%d", totalBefore, usedBefore)

	// 安装一些代码
	cg := NewCodeGenerator()
	cg.EmitPrologue(0)
	cg.EmitMovRegImm64(RAX, 123)
	cg.EmitEpilogue()

	_, err := GlobalCodeCache.Install(cg.Code())
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// 获取最终状态
	totalAfter, usedAfter := GlobalCodeCache.Stats()
	t.Logf("After: total=%d, used=%d", totalAfter, usedAfter)

	// 验证使用量增加
	if usedAfter <= usedBefore {
		t.Error("Code cache usage should increase after installation")
	}
}

// TestJIT_E2E_MemoryProtection 测试内存保护
func TestJIT_E2E_MemoryProtection(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Log("Running on Windows with PAGE_EXECUTE_READWRITE")
	} else {
		t.Log("Running on Unix with mmap")
	}

	// 分配可执行内存
	buf, err := NewExecutableBuffer(4096)
	if err != nil {
		t.Fatalf("Failed to allocate executable buffer: %v", err)
	}
	defer buf.Free()

	t.Logf("Allocated buffer at %p, size=%d", unsafe.Pointer(buf.Addr()), buf.Size())

	// 写入简单代码
	simpleCode := []byte{
		0x48, 0xC7, 0xC0, 0x2A, 0x00, 0x00, 0x00, // MOV RAX, 42
		0xC3, // RET
	}

	addr, err := buf.Write(simpleCode)
	if err != nil {
		t.Fatalf("Failed to write code: %v", err)
	}

	t.Logf("Code written at %p", unsafe.Pointer(addr))

	// 尝试执行
	result := callJITCode0(addr)
	if result != 42 {
		t.Errorf("Expected 42, got %d", result)
	}
	t.Logf("Code execution successful, result=%d", result)
}

// ============================================================================
// JIT 调用桥接函数
// 使用 syscall 直接调用生成的机器码
// ============================================================================

// callJITCode0 调用无参数的 JIT 函数
//
//go:noinline
func callJITCode0(entry uintptr) int64 {
	if entry == 0 {
		return 0
	}
	// 使用 syscall.Syscall 直接调用原生代码
	// 在 Windows AMD64 上，syscall.Syscall 会设置好调用约定
	r1, _, _ := syscall.Syscall(entry, 0, 0, 0, 0)
	return int64(r1)
}

// callJITCode1 调用带一个整数参数的 JIT 函数
//
//go:noinline
func callJITCode1(entry uintptr, arg int64) int64 {
	if entry == 0 {
		return 0
	}
	r1, _, _ := syscall.Syscall(entry, 1, uintptr(arg), 0, 0)
	return int64(r1)
}

// callJITCode2 调用带两个整数参数的 JIT 函数
//
//go:noinline
func callJITCode2(entry uintptr, arg1, arg2 int64) int64 {
	if entry == 0 {
		return 0
	}
	r1, _, _ := syscall.Syscall(entry, 2, uintptr(arg1), uintptr(arg2), 0)
	return int64(r1)
}

// ============================================================================
// 性能基准测试
// ============================================================================

// BenchmarkJIT_NativeCall 基准测试：原生调用开销
func BenchmarkJIT_NativeCall(b *testing.B) {
	cg := NewCodeGenerator()
	cg.EmitPrologue(0)
	cg.EmitMovRegImm64(RAX, 42)
	cg.EmitEpilogue()

	entry, err := GlobalCodeCache.Install(cg.Code())
	if err != nil {
		b.Fatalf("Install failed: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		callJITCode0(entry)
	}
}

// BenchmarkJIT_Addition 基准测试：加法运算
func BenchmarkJIT_Addition(b *testing.B) {
	cg := NewCodeGenerator()
	cg.EmitPrologue(0)
	cg.EmitMovRegImm64(RAX, 10)
	cg.EmitMovRegImm64(RBX, 32)
	cg.EmitAddRegReg(RAX, RBX)
	cg.EmitEpilogue()

	entry, err := GlobalCodeCache.Install(cg.Code())
	if err != nil {
		b.Fatalf("Install failed: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		callJITCode0(entry)
	}
}

// BenchmarkJIT_Loop 基准测试：循环计算 sum(1..100)
func BenchmarkJIT_Loop(b *testing.B) {
	cg := NewCodeGenerator()

	// Windows x64: 参数 N 在 RCX，返回值在 RAX
	// MOV R8, RCX (保存 N)
	cg.EmitMovRegReg(R8, RCX)
	// XOR RAX, RAX (sum = 0)
	cg.EmitXorRegReg(RAX, RAX)
	// XOR R9, R9 (i = 0)
	cg.EmitXorRegReg(R9, R9)

	loopStart := cg.CurrentOffset()

	// INC R9
	cg.EmitAddRegImm32(R9, 1)
	// ADD RAX, R9
	cg.EmitAddRegReg(RAX, R9)
	// CMP R9, R8
	cg.EmitCmpRegReg(R9, R8)
	// JL loopStart
	rel := int32(loopStart - (cg.CurrentOffset() + 6))
	cg.EmitJl(rel)

	// RET
	cg.EmitRet()

	entry, err := GlobalCodeCache.Install(cg.Code())
	if err != nil {
		b.Fatalf("Install failed: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		callJITCode1(entry, 100)
	}
}

// BenchmarkGo_Loop 基准测试：Go 原生循环 sum(1..100)
func BenchmarkGo_Loop(b *testing.B) {
	for i := 0; i < b.N; i++ {
		goSum100()
	}
}

//go:noinline
func goSum100() int64 {
	var sum int64 = 0
	for i := int64(1); i <= 100; i++ {
		sum += i
	}
	return sum
}

// ============================================================================
// 辅助函数
// ============================================================================

// printCode 打印生成的机器码 (调试用)
func printCode(code []byte) {
	fmt.Printf("Code (%d bytes): ", len(code))
	for _, b := range code {
		fmt.Printf("%02X ", b)
	}
	fmt.Println()
}
