package vm

import (
	"testing"

	"github.com/tangzhangming/nova/internal/bytecode"
)

// ============================================================================
// Value 创建基准测试
// 验证 Tagged Union 优化效果
// ============================================================================

func BenchmarkNewInt(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = bytecode.NewInt(int64(i))
	}
}

func BenchmarkNewInt_Small(b *testing.B) {
	// 测试小整数缓存
	for i := 0; i < b.N; i++ {
		_ = bytecode.NewInt(int64(i % 256))
	}
}

func BenchmarkNewFloat(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = bytecode.NewFloat(float64(i) * 0.1)
	}
}

func BenchmarkNewString(b *testing.B) {
	s := "hello world"
	for i := 0; i < b.N; i++ {
		_ = bytecode.NewString(s)
	}
}

func BenchmarkNewString_Empty(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = bytecode.NewString("")
	}
}

// ============================================================================
// Value 访问基准测试
// ============================================================================

func BenchmarkAsInt(b *testing.B) {
	v := bytecode.NewInt(42)
	for i := 0; i < b.N; i++ {
		_ = v.AsInt()
	}
}

func BenchmarkAsFloat(b *testing.B) {
	v := bytecode.NewFloat(3.14)
	for i := 0; i < b.N; i++ {
		_ = v.AsFloat()
	}
}

func BenchmarkAsString(b *testing.B) {
	v := bytecode.NewString("hello")
	for i := 0; i < b.N; i++ {
		_ = v.AsString()
	}
}

// ============================================================================
// 算术运算基准测试
// ============================================================================

func BenchmarkIntAdd(b *testing.B) {
	vm := New()
	a := bytecode.NewInt(100)
	c := bytecode.NewInt(1)
	for i := 0; i < b.N; i++ {
		vm.push(a)
		vm.push(c)
		opAdd(vm)
		vm.pop()
	}
}

func BenchmarkIntMul(b *testing.B) {
	vm := New()
	a := bytecode.NewInt(100)
	c := bytecode.NewInt(2)
	for i := 0; i < b.N; i++ {
		vm.push(a)
		vm.push(c)
		opMul(vm)
		vm.pop()
	}
}

func BenchmarkFloatAdd(b *testing.B) {
	vm := New()
	a := bytecode.NewFloat(100.5)
	c := bytecode.NewFloat(1.5)
	for i := 0; i < b.N; i++ {
		vm.push(a)
		vm.push(c)
		opAdd(vm)
		vm.pop()
	}
}

// ============================================================================
// 比较运算基准测试
// ============================================================================

func BenchmarkIntCompare(b *testing.B) {
	vm := New()
	a := bytecode.NewInt(100)
	c := bytecode.NewInt(50)
	for i := 0; i < b.N; i++ {
		vm.push(a)
		vm.push(c)
		opLt(vm)
		vm.pop()
	}
}

// ============================================================================
// 栈操作基准测试
// ============================================================================

func BenchmarkPushPop(b *testing.B) {
	vm := New()
	v := bytecode.NewInt(42)
	for i := 0; i < b.N; i++ {
		vm.push(v)
		vm.pop()
	}
}

func BenchmarkPushPop10(b *testing.B) {
	vm := New()
	v := bytecode.NewInt(42)
	for i := 0; i < b.N; i++ {
		for j := 0; j < 10; j++ {
			vm.push(v)
		}
		for j := 0; j < 10; j++ {
			vm.pop()
		}
	}
}

// ============================================================================
// SuperArray 基准测试
// ============================================================================

func BenchmarkSuperArray_SetInt(b *testing.B) {
	sa := Helper_SA_New()
	v := bytecode.NewInt(42)
	for i := 0; i < b.N; i++ {
		Helper_SA_SetInt(sa, int64(i%1000), v)
	}
}

func BenchmarkSuperArray_GetInt(b *testing.B) {
	sa := Helper_SA_New()
	v := bytecode.NewInt(42)
	for i := 0; i < 1000; i++ {
		Helper_SA_SetInt(sa, int64(i), v)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Helper_SA_GetInt(sa, int64(i%1000))
	}
}

func BenchmarkSuperArray_SetString(b *testing.B) {
	sa := Helper_SA_New()
	v := bytecode.NewInt(42)
	keys := make([]string, 1000)
	for i := range keys {
		keys[i] = string(rune('a' + i%26))
	}
	for i := 0; i < b.N; i++ {
		Helper_SA_SetString(sa, keys[i%1000], v)
	}
}

func BenchmarkSuperArray_GetString(b *testing.B) {
	sa := Helper_SA_New()
	v := bytecode.NewInt(42)
	keys := make([]string, 1000)
	for i := range keys {
		keys[i] = string(rune('a' + i%26))
		Helper_SA_SetString(sa, keys[i], v)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Helper_SA_GetString(sa, keys[i%1000])
	}
}

// ============================================================================
// 字符串操作基准测试
// ============================================================================

func BenchmarkStringConcat(b *testing.B) {
	a := bytecode.NewString("Hello, ")
	c := bytecode.NewString("World!")
	for i := 0; i < b.N; i++ {
		_ = Helper_StringConcat(a, c)
	}
}

// ============================================================================
// 模拟循环基准测试
// ============================================================================

func BenchmarkSimulatedLoop(b *testing.B) {
	// 模拟: for i := 0; i < 1000; i++ { sum += i }
	vm := New()
	
	for n := 0; n < b.N; n++ {
		sum := bytecode.NewInt(0)
		for i := int64(0); i < 1000; i++ {
			// sum = sum + i
			vm.push(sum)
			vm.push(bytecode.NewInt(i))
			opAdd(vm)
			sum = vm.pop()
		}
	}
}

func BenchmarkSimulatedLoop_NativeGo(b *testing.B) {
	// 对比: 纯 Go 实现
	for n := 0; n < b.N; n++ {
		sum := int64(0)
		for i := int64(0); i < 1000; i++ {
			sum += i
		}
		_ = sum
	}
}

// ============================================================================
// Value 类型检查基准测试
// ============================================================================

func BenchmarkIsInt(b *testing.B) {
	v := bytecode.NewInt(42)
	for i := 0; i < b.N; i++ {
		_ = v.IsInt()
	}
}

func BenchmarkTypeSwitch(b *testing.B) {
	v := bytecode.NewInt(42)
	for i := 0; i < b.N; i++ {
		switch v.Type() {
		case bytecode.ValInt:
			_ = v.AsInt()
		case bytecode.ValFloat:
			_ = v.AsFloat()
		}
	}
}
