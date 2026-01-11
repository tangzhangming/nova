package vm

import (
	"testing"

	"github.com/tangzhangming/nova/internal/bytecode"
)

// ============================================================================
// 基本测试
// ============================================================================

func TestNewVM(t *testing.T) {
	vm := New()
	if vm == nil {
		t.Fatal("New() returned nil")
	}
	if vm.sp != 0 {
		t.Errorf("Expected sp=0, got %d", vm.sp)
	}
	if vm.fp != 0 {
		t.Errorf("Expected fp=0, got %d", vm.fp)
	}
}

func TestPushPop(t *testing.T) {
	vm := New()

	// Push
	vm.push(bytecode.NewInt(42))
	if vm.sp != 1 {
		t.Errorf("Expected sp=1 after push, got %d", vm.sp)
	}

	// Peek
	v := vm.peek(0)
	if !v.IsInt() || v.AsInt() != 42 {
		t.Errorf("Expected 42, got %v", v)
	}

	// Pop
	v = vm.pop()
	if !v.IsInt() || v.AsInt() != 42 {
		t.Errorf("Expected 42, got %v", v)
	}
	if vm.sp != 0 {
		t.Errorf("Expected sp=0 after pop, got %d", vm.sp)
	}
}

// ============================================================================
// 算术运算测试
// ============================================================================

func TestIntegerArithmetic(t *testing.T) {
	tests := []struct {
		name string
		a, b int64
		op   func(*VM)
		want int64
	}{
		{"add", 10, 20, opAdd, 30},
		{"sub", 50, 20, opSub, 30},
		{"mul", 6, 7, opMul, 42},
		{"div", 100, 5, opDiv, 20},
		{"mod", 17, 5, opMod, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm := New()
			vm.push(bytecode.NewInt(tt.a))
			vm.push(bytecode.NewInt(tt.b))
			tt.op(vm)
			result := vm.pop()
			if !result.IsInt() || result.AsInt() != tt.want {
				t.Errorf("Expected %d, got %v", tt.want, result)
			}
		})
	}
}

func TestFloatArithmetic(t *testing.T) {
	vm := New()
	vm.push(bytecode.NewFloat(3.14))
	vm.push(bytecode.NewFloat(2.0))
	opAdd(vm)
	result := vm.pop()
	if !result.IsFloat() {
		t.Errorf("Expected float result, got %v", result.Type())
	}
	// 使用近似比较
	expected := 5.14
	if diff := result.AsFloat() - expected; diff < -0.001 || diff > 0.001 {
		t.Errorf("Expected ~%f, got %f", expected, result.AsFloat())
	}
}

func TestMixedArithmetic(t *testing.T) {
	vm := New()
	vm.push(bytecode.NewInt(10))
	vm.push(bytecode.NewFloat(2.5))
	opAdd(vm)
	result := vm.pop()
	if !result.IsFloat() {
		t.Errorf("Expected float result for mixed arithmetic")
	}
}

// ============================================================================
// 比较运算测试
// ============================================================================

func TestComparison(t *testing.T) {
	tests := []struct {
		name string
		a, b int64
		op   func(*VM)
		want bool
	}{
		{"eq_true", 10, 10, opEq, true},
		{"eq_false", 10, 20, opEq, false},
		{"ne_true", 10, 20, opNe, true},
		{"ne_false", 10, 10, opNe, false},
		{"lt_true", 10, 20, opLt, true},
		{"lt_false", 20, 10, opLt, false},
		{"le_true", 10, 10, opLe, true},
		{"gt_true", 20, 10, opGt, true},
		{"ge_true", 10, 10, opGe, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm := New()
			vm.push(bytecode.NewInt(tt.a))
			vm.push(bytecode.NewInt(tt.b))
			tt.op(vm)
			result := vm.pop()
			if result.AsBool() != tt.want {
				t.Errorf("Expected %v, got %v", tt.want, result.AsBool())
			}
		})
	}
}

// ============================================================================
// 逻辑运算测试
// ============================================================================

func TestLogicalNot(t *testing.T) {
	vm := New()
	vm.push(bytecode.TrueValue)
	opNot(vm)
	result := vm.pop()
	if result.AsBool() != false {
		t.Errorf("Expected false, got %v", result)
	}
}

// ============================================================================
// 位运算测试
// ============================================================================

func TestBitwiseOps(t *testing.T) {
	tests := []struct {
		name string
		a, b int64
		op   func(*VM)
		want int64
	}{
		{"band", 0b1100, 0b1010, opBand, 0b1000},
		{"bor", 0b1100, 0b1010, opBor, 0b1110},
		{"bxor", 0b1100, 0b1010, opBxor, 0b0110},
		{"shl", 1, 4, opShl, 16},
		{"shr", 16, 2, opShr, 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm := New()
			vm.push(bytecode.NewInt(tt.a))
			vm.push(bytecode.NewInt(tt.b))
			tt.op(vm)
			result := vm.pop()
			if result.AsInt() != tt.want {
				t.Errorf("Expected %d, got %d", tt.want, result.AsInt())
			}
		})
	}
}

// ============================================================================
// Helper 函数测试
// ============================================================================

func TestHelperStringConcat(t *testing.T) {
	result := Helper_StringConcat(
		bytecode.NewString("Hello, "),
		bytecode.NewString("World!"),
	)
	if result.AsString() != "Hello, World!" {
		t.Errorf("Expected 'Hello, World!', got '%s'", result.AsString())
	}
}

func TestHelperSuperArray(t *testing.T) {
	sa := Helper_SA_New()
	
	// 设置值
	Helper_SA_SetInt(sa, 0, bytecode.NewString("zero"))
	Helper_SA_SetString(sa, "key", bytecode.NewInt(42))
	
	// 获取值
	v0 := Helper_SA_GetInt(sa, 0)
	if v0.AsString() != "zero" {
		t.Errorf("Expected 'zero', got '%s'", v0.AsString())
	}
	
	vKey := Helper_SA_GetString(sa, "key")
	if vKey.AsInt() != 42 {
		t.Errorf("Expected 42, got %d", vKey.AsInt())
	}
	
	// 长度
	if Helper_SA_Len(sa) != 2 {
		t.Errorf("Expected len=2, got %d", Helper_SA_Len(sa))
	}
}

// ============================================================================
// Value 类型测试
// ============================================================================

func TestValueType(t *testing.T) {
	tests := []struct {
		name  string
		value bytecode.Value
		typ   bytecode.ValueType
	}{
		{"null", bytecode.NullValue, bytecode.ValNull},
		{"true", bytecode.TrueValue, bytecode.ValBool},
		{"false", bytecode.FalseValue, bytecode.ValBool},
		{"int", bytecode.NewInt(42), bytecode.ValInt},
		{"float", bytecode.NewFloat(3.14), bytecode.ValFloat},
		{"string", bytecode.NewString("hello"), bytecode.ValString},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value.Type() != tt.typ {
				t.Errorf("Expected type %v, got %v", tt.typ, tt.value.Type())
			}
		})
	}
}

func TestValueEquality(t *testing.T) {
	tests := []struct {
		name string
		a, b bytecode.Value
		want bool
	}{
		{"int_eq", bytecode.NewInt(42), bytecode.NewInt(42), true},
		{"int_ne", bytecode.NewInt(42), bytecode.NewInt(43), false},
		{"string_eq", bytecode.NewString("hello"), bytecode.NewString("hello"), true},
		{"string_ne", bytecode.NewString("hello"), bytecode.NewString("world"), false},
		{"int_float_eq", bytecode.NewInt(42), bytecode.NewFloat(42.0), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.a.Equals(tt.b) != tt.want {
				t.Errorf("Expected %v, got %v", tt.want, tt.a.Equals(tt.b))
			}
		})
	}
}
