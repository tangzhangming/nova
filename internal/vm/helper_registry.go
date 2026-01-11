package vm

import (
	"reflect"
	"unsafe"

	"github.com/tangzhangming/nova/internal/bytecode"
)

// ============================================================================
// Helper 注册表
// 用于 JIT 编译器获取 Helper 函数地址
// ============================================================================

// HelperFunc Helper 函数类型
type HelperFunc func(args ...bytecode.Value) bytecode.Value

// helperRegistry Helper 注册表
var helperRegistry = make(map[string]HelperFunc)

// helperAddrs Helper 函数地址缓存
var helperAddrs = make(map[string]uintptr)

// RegisterHelper 注册 Helper 函数
func RegisterHelper(name string, fn HelperFunc) {
	helperRegistry[name] = fn
	// 缓存函数地址供 JIT 使用
	helperAddrs[name] = getFuncAddr(fn)
}

// GetHelper 获取 Helper 函数
func GetHelper(name string) HelperFunc {
	return helperRegistry[name]
}

// GetHelperAddr 获取 Helper 函数地址（供 JIT 使用）
func GetHelperAddr(name string) uintptr {
	return helperAddrs[name]
}

// ListHelpers 列出所有已注册的 Helper
func ListHelpers() []string {
	names := make([]string, 0, len(helperRegistry))
	for name := range helperRegistry {
		names = append(names, name)
	}
	return names
}

// getFuncAddr 获取函数地址
func getFuncAddr(fn interface{}) uintptr {
	return reflect.ValueOf(fn).Pointer()
}

// ============================================================================
// Helper 函数指针类型（供 JIT 直接调用）
// ============================================================================

// HelperFuncPtr Helper 函数指针
type HelperFuncPtr = unsafe.Pointer

// GetHelperPtr 获取 Helper 函数指针
func GetHelperPtr(name string) HelperFuncPtr {
	addr := GetHelperAddr(name)
	if addr == 0 {
		return nil
	}
	return unsafe.Pointer(addr)
}

// ============================================================================
// 初始化：注册所有内置 Helper
// ============================================================================

func init() {
	// 算术运算 Helper
	RegisterHelper("Add", helperAdd)
	RegisterHelper("Sub", helperSub)
	RegisterHelper("Mul", helperMul)
	RegisterHelper("Div", helperDiv)
	RegisterHelper("Mod", helperMod)
	RegisterHelper("Neg", helperNeg)

	// 比较运算 Helper
	RegisterHelper("Equal", helperEqual)
	RegisterHelper("NotEqual", helperNotEqual)
	RegisterHelper("Less", helperLess)
	RegisterHelper("LessEqual", helperLessEqual)
	RegisterHelper("Greater", helperGreater)
	RegisterHelper("GreaterEqual", helperGreaterEqual)

	// 字符串操作 Helper
	RegisterHelper("StringConcat", helperStringConcat)
	RegisterHelper("StringLen", helperStringLen)

	// SuperArray Helper
	RegisterHelper("SA_New", helperSANew)
	RegisterHelper("SA_Get", helperSAGet)
	RegisterHelper("SA_Set", helperSASet)
	RegisterHelper("SA_Len", helperSALen)
	RegisterHelper("SA_Push", helperSAPush)
	RegisterHelper("SA_Has", helperSAHas)
}

// ============================================================================
// 算术运算 Helper 包装
// ============================================================================

func helperAdd(args ...bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NullValue
	}
	return Helper_Add(args[0], args[1])
}

func helperSub(args ...bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NullValue
	}
	return Helper_Sub(args[0], args[1])
}

func helperMul(args ...bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NullValue
	}
	return Helper_Mul(args[0], args[1])
}

func helperDiv(args ...bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NullValue
	}
	return Helper_Div(args[0], args[1])
}

func helperMod(args ...bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NullValue
	}
	return Helper_Mod(args[0], args[1])
}

func helperNeg(args ...bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.NullValue
	}
	return Helper_Neg(args[0])
}

// ============================================================================
// 比较运算 Helper 包装
// ============================================================================

func helperEqual(args ...bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.FalseValue
	}
	return bytecode.NewBool(args[0].Equals(args[1]))
}

func helperNotEqual(args ...bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.TrueValue
	}
	return bytecode.NewBool(!args[0].Equals(args[1]))
}

func helperLess(args ...bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.FalseValue
	}
	return Helper_Less(args[0], args[1])
}

func helperLessEqual(args ...bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.FalseValue
	}
	return Helper_LessEqual(args[0], args[1])
}

func helperGreater(args ...bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.FalseValue
	}
	return Helper_Greater(args[0], args[1])
}

func helperGreaterEqual(args ...bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.FalseValue
	}
	return Helper_GreaterEqual(args[0], args[1])
}

// ============================================================================
// 字符串操作 Helper 包装
// ============================================================================

func helperStringConcat(args ...bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewString("")
	}
	return Helper_StringConcat(args[0], args[1])
}

func helperStringLen(args ...bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.ZeroValue
	}
	if args[0].IsString() {
		return bytecode.NewInt(int64(len(args[0].AsString())))
	}
	return bytecode.ZeroValue
}

// ============================================================================
// SuperArray Helper 包装
// ============================================================================

func helperSANew(args ...bytecode.Value) bytecode.Value {
	return bytecode.NewSuperArrayValue(Helper_SA_New())
}

func helperSAGet(args ...bytecode.Value) bytecode.Value {
	if len(args) < 2 || !args[0].IsSuperArray() {
		return bytecode.NullValue
	}
	return Helper_SA_Get(args[0].AsSuperArray(), args[1])
}

func helperSASet(args ...bytecode.Value) bytecode.Value {
	if len(args) < 3 || !args[0].IsSuperArray() {
		return bytecode.NullValue
	}
	Helper_SA_Set(args[0].AsSuperArray(), args[1], args[2])
	return bytecode.NullValue
}

func helperSALen(args ...bytecode.Value) bytecode.Value {
	if len(args) < 1 || !args[0].IsSuperArray() {
		return bytecode.ZeroValue
	}
	return bytecode.NewInt(int64(Helper_SA_Len(args[0].AsSuperArray())))
}

func helperSAPush(args ...bytecode.Value) bytecode.Value {
	if len(args) < 2 || !args[0].IsSuperArray() {
		return bytecode.NullValue
	}
	args[0].AsSuperArray().Push(args[1])
	return bytecode.NullValue
}

func helperSAHas(args ...bytecode.Value) bytecode.Value {
	if len(args) < 2 || !args[0].IsSuperArray() {
		return bytecode.FalseValue
	}
	return bytecode.NewBool(args[0].AsSuperArray().HasKey(args[1]))
}
