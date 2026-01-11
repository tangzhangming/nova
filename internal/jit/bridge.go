// +build amd64

package jit

import (
	"reflect"
	"unsafe"

	"github.com/tangzhangming/nova/internal/bytecode"
)

// ============================================================================
// JIT 桥接层
// 连接 JIT 编译的代码和 Go 运行时函数
// ============================================================================

// JITBridge JIT 桥接器
type JITBridge struct {
	// Helper 函数地址缓存
	helperAddrs map[string]uintptr

	// 编译器引用
	compiler *JITCompiler
}

// NewJITBridge 创建 JIT 桥接器
func NewJITBridge(compiler *JITCompiler) *JITBridge {
	bridge := &JITBridge{
		helperAddrs: make(map[string]uintptr),
		compiler:    compiler,
	}
	bridge.registerAllHelpers()
	return bridge
}

// registerAllHelpers 注册所有 Helper 函数
func (b *JITBridge) registerAllHelpers() {
	// 算术运算
	b.registerHelper("Add", jitHelperAdd)
	b.registerHelper("Sub", jitHelperSub)
	b.registerHelper("Mul", jitHelperMul)
	b.registerHelper("Div", jitHelperDiv)
	b.registerHelper("Mod", jitHelperMod)
	b.registerHelper("Neg", jitHelperNeg)

	// 比较运算
	b.registerHelper("Equal", jitHelperEqual)
	b.registerHelper("NotEqual", jitHelperNotEqual)
	b.registerHelper("Less", jitHelperLess)
	b.registerHelper("LessEqual", jitHelperLessEqual)
	b.registerHelper("Greater", jitHelperGreater)
	b.registerHelper("GreaterEqual", jitHelperGreaterEqual)

	// 字符串操作
	b.registerHelper("StringConcat", jitHelperStringConcat)

	// SuperArray 操作
	b.registerHelper("SA_New", jitHelperSANew)
	b.registerHelper("SA_Get", jitHelperSAGet)
	b.registerHelper("SA_Set", jitHelperSASet)
	b.registerHelper("SA_Len", jitHelperSALen)

	// 类型检查
	b.registerHelper("TypeCheck", jitHelperTypeCheck)
	b.registerHelper("IsTruthy", jitHelperIsTruthy)

	// 数组操作
	b.registerHelper("ArrayNew", jitHelperArrayNew)
	b.registerHelper("ArrayGet", jitHelperArrayGet)
	b.registerHelper("ArraySet", jitHelperArraySet)
	b.registerHelper("ArrayLen", jitHelperArrayLen)

	// 将地址注册到编译器
	for name, addr := range b.helperAddrs {
		b.compiler.RegisterHelper(name, addr)
	}
}

// registerHelper 注册单个 Helper 函数
func (b *JITBridge) registerHelper(name string, fn interface{}) {
	addr := getFuncPtr(fn)
	b.helperAddrs[name] = addr
}

// GetHelperAddr 获取 Helper 函数地址
func (b *JITBridge) GetHelperAddr(name string) uintptr {
	return b.helperAddrs[name]
}

// ============================================================================
// 函数指针工具
// ============================================================================

// getFuncPtr 获取函数指针
func getFuncPtr(fn interface{}) uintptr {
	return reflect.ValueOf(fn).Pointer()
}

// ============================================================================
// JIT Helper 函数实现
// 这些函数使用 Go 调用约定，可以直接从 JIT 代码调用
// ============================================================================

// 使用 //go:noinline 确保函数有稳定地址

//go:noinline
func jitHelperAdd(a, b bytecode.Value) bytecode.Value {
	// 快速路径：整数
	if a.IsInt() && b.IsInt() {
		return bytecode.NewInt(a.AsInt() + b.AsInt())
	}
	// 快速路径：浮点数
	if a.IsFloat() || b.IsFloat() {
		return bytecode.NewFloat(a.AsFloat() + b.AsFloat())
	}
	// 字符串拼接
	if a.IsString() || b.IsString() {
		return bytecode.NewString(a.String() + b.String())
	}
	return bytecode.ZeroValue
}

//go:noinline
func jitHelperSub(a, b bytecode.Value) bytecode.Value {
	if a.IsInt() && b.IsInt() {
		return bytecode.NewInt(a.AsInt() - b.AsInt())
	}
	return bytecode.NewFloat(a.AsFloat() - b.AsFloat())
}

//go:noinline
func jitHelperMul(a, b bytecode.Value) bytecode.Value {
	if a.IsInt() && b.IsInt() {
		return bytecode.NewInt(a.AsInt() * b.AsInt())
	}
	return bytecode.NewFloat(a.AsFloat() * b.AsFloat())
}

//go:noinline
func jitHelperDiv(a, b bytecode.Value) bytecode.Value {
	if a.IsInt() && b.IsInt() {
		bi := b.AsInt()
		if bi == 0 {
			return bytecode.NullValue
		}
		return bytecode.NewInt(a.AsInt() / bi)
	}
	bf := b.AsFloat()
	if bf == 0 {
		return bytecode.NullValue
	}
	return bytecode.NewFloat(a.AsFloat() / bf)
}

//go:noinline
func jitHelperMod(a, b bytecode.Value) bytecode.Value {
	ai, bi := a.AsInt(), b.AsInt()
	if bi == 0 {
		return bytecode.NullValue
	}
	return bytecode.NewInt(ai % bi)
}

//go:noinline
func jitHelperNeg(a bytecode.Value) bytecode.Value {
	if a.IsInt() {
		return bytecode.NewInt(-a.AsInt())
	}
	return bytecode.NewFloat(-a.AsFloat())
}

//go:noinline
func jitHelperEqual(a, b bytecode.Value) bytecode.Value {
	return bytecode.NewBool(a.Equals(b))
}

//go:noinline
func jitHelperNotEqual(a, b bytecode.Value) bytecode.Value {
	return bytecode.NewBool(!a.Equals(b))
}

//go:noinline
func jitHelperLess(a, b bytecode.Value) bytecode.Value {
	if a.IsInt() && b.IsInt() {
		return bytecode.NewBool(a.AsInt() < b.AsInt())
	}
	return bytecode.NewBool(a.AsFloat() < b.AsFloat())
}

//go:noinline
func jitHelperLessEqual(a, b bytecode.Value) bytecode.Value {
	if a.IsInt() && b.IsInt() {
		return bytecode.NewBool(a.AsInt() <= b.AsInt())
	}
	return bytecode.NewBool(a.AsFloat() <= b.AsFloat())
}

//go:noinline
func jitHelperGreater(a, b bytecode.Value) bytecode.Value {
	if a.IsInt() && b.IsInt() {
		return bytecode.NewBool(a.AsInt() > b.AsInt())
	}
	return bytecode.NewBool(a.AsFloat() > b.AsFloat())
}

//go:noinline
func jitHelperGreaterEqual(a, b bytecode.Value) bytecode.Value {
	if a.IsInt() && b.IsInt() {
		return bytecode.NewBool(a.AsInt() >= b.AsInt())
	}
	return bytecode.NewBool(a.AsFloat() >= b.AsFloat())
}

//go:noinline
func jitHelperStringConcat(a, b bytecode.Value) bytecode.Value {
	return bytecode.NewString(a.String() + b.String())
}

//go:noinline
func jitHelperSANew() bytecode.Value {
	return bytecode.NewSuperArrayValue(bytecode.NewSuperArray())
}

//go:noinline
func jitHelperSAGet(sa, key bytecode.Value) bytecode.Value {
	if !sa.IsSuperArray() {
		return bytecode.NullValue
	}
	if val, ok := sa.AsSuperArray().Get(key); ok {
		return val
	}
	return bytecode.NullValue
}

//go:noinline
func jitHelperSASet(sa, key, val bytecode.Value) bytecode.Value {
	if !sa.IsSuperArray() {
		return bytecode.NullValue
	}
	sa.AsSuperArray().Set(key, val)
	return val
}

//go:noinline
func jitHelperSALen(sa bytecode.Value) bytecode.Value {
	if !sa.IsSuperArray() {
		return bytecode.ZeroValue
	}
	return bytecode.NewInt(int64(sa.AsSuperArray().Len()))
}

//go:noinline
func jitHelperTypeCheck(v bytecode.Value, expectedType int) bytecode.Value {
	return bytecode.NewBool(int(v.Type()) == expectedType)
}

//go:noinline
func jitHelperIsTruthy(v bytecode.Value) bytecode.Value {
	return bytecode.NewBool(v.IsTruthy())
}

//go:noinline
func jitHelperArrayNew(count int) bytecode.Value {
	arr := make([]bytecode.Value, count)
	return bytecode.NewArray(arr)
}

//go:noinline
func jitHelperArrayGet(arr, idx bytecode.Value) bytecode.Value {
	if !arr.IsArray() {
		return bytecode.NullValue
	}
	a := arr.AsArray()
	i := int(idx.AsInt())
	if i < 0 || i >= len(a) {
		return bytecode.NullValue
	}
	return a[i]
}

//go:noinline
func jitHelperArraySet(arr, idx, val bytecode.Value) bytecode.Value {
	if !arr.IsArray() {
		return bytecode.NullValue
	}
	a := arr.AsArray()
	i := int(idx.AsInt())
	if i >= 0 && i < len(a) {
		a[i] = val
	}
	return val
}

//go:noinline
func jitHelperArrayLen(arr bytecode.Value) bytecode.Value {
	if !arr.IsArray() {
		return bytecode.ZeroValue
	}
	return bytecode.NewInt(int64(len(arr.AsArray())))
}

// ============================================================================
// JIT 调用约定
// ============================================================================

// JITCallFrame JIT 调用帧
type JITCallFrame struct {
	// 返回地址
	ReturnAddr uintptr

	// 保存的寄存器
	SavedRBP uintptr
	SavedRBX uintptr
	SavedR12 uintptr
	SavedR13 uintptr
	SavedR14 uintptr
	SavedR15 uintptr

	// 参数 (通过栈传递的 Value)
	Args []bytecode.Value

	// 返回值
	ReturnValue bytecode.Value
}

// ============================================================================
// 原生函数调用
// ============================================================================

// NativeFunc 原生函数类型
type NativeFunc func(args []bytecode.Value) bytecode.Value

// CallNative 调用原生函数
//
//go:noinline
func CallNative(fn NativeFunc, args []bytecode.Value) bytecode.Value {
	return fn(args)
}

// CallNativePtr 通过指针调用原生函数
func CallNativePtr(fnPtr uintptr, args []bytecode.Value) bytecode.Value {
	// 将指针转换为函数
	fn := *(*NativeFunc)(unsafe.Pointer(&fnPtr))
	return fn(args)
}

// ============================================================================
// Value 内存布局
// ============================================================================

// ValueLayout Value 在内存中的布局
// 与 bytecode.Value 的实际布局对应
type ValueLayout struct {
	Typ uint8     // 类型标记 (1 byte)
	_   [7]byte   // 填充 (7 bytes)
	Num int64     // 数值 (8 bytes)
	Ptr uintptr   // 指针 (8 bytes)
}

// ValueToLayout 将 Value 转换为内存布局
func ValueToLayout(v bytecode.Value) ValueLayout {
	return *(*ValueLayout)(unsafe.Pointer(&v))
}

// LayoutToValue 将内存布局转换为 Value
func LayoutToValue(l ValueLayout) bytecode.Value {
	return *(*bytecode.Value)(unsafe.Pointer(&l))
}

// ============================================================================
// 全局桥接器
// ============================================================================

// GlobalBridge 全局 JIT 桥接器
var GlobalBridge *JITBridge

// InitGlobalBridge 初始化全局桥接器
func InitGlobalBridge(compiler *JITCompiler) {
	GlobalBridge = NewJITBridge(compiler)
}

// GetGlobalHelperAddr 获取全局 Helper 地址
func GetGlobalHelperAddr(name string) uintptr {
	if GlobalBridge == nil {
		return 0
	}
	return GlobalBridge.GetHelperAddr(name)
}
