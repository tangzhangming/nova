// runtime_helpers.go - JIT 运行时辅助函数
//
// 本文件提供 JIT 编译代码与 Go 运行时交互的辅助函数。
// 这些函数被 JIT 代码通过函数指针调用，处理复杂的运行时操作。
//
// 注意：这些函数的签名必须与 JIT 代码生成中的调用约定匹配。

package jit

import (
	"sync"
	"unsafe"

	"github.com/tangzhangming/nova/internal/bytecode"
)

// ============================================================================
// 数组操作辅助函数
// ============================================================================

// ArrayLenHelper 获取数组长度
// 参数：arr - 指向 bytecode.Value 的指针（Value.Type 应为 ValArray）
// 返回：数组长度，如果不是数组则返回 -1
//
//go:nosplit
func ArrayLenHelper(arrPtr uintptr) int64 {
	if arrPtr == 0 {
		return -1
	}

	arr := (*bytecode.Value)(unsafe.Pointer(arrPtr))
	
	switch arr.Type {
	case bytecode.ValArray:
		if arr.Data == nil {
			return 0
		}
		elements := arr.Data.([]bytecode.Value)
		return int64(len(elements))
		
	case bytecode.ValFixedArray:
		if arr.Data == nil {
			return 0
		}
		fa := arr.Data.(*bytecode.FixedArray)
		return int64(len(fa.Elements))
		
	case bytecode.ValString:
		if arr.Data == nil {
			return 0
		}
		s := arr.Data.(string)
		return int64(len(s))
		
	case bytecode.ValBytes:
		if arr.Data == nil {
			return 0
		}
		b := arr.Data.([]byte)
		return int64(len(b))
		
	default:
		return -1
	}
}

// ArrayGetHelper 获取数组元素
// 参数：
//   - arrPtr: 指向 bytecode.Value 的指针
//   - index: 数组索引
//
// 返回：
//   - value: 元素值（int64 表示）
//   - ok: 1 表示成功，0 表示失败（越界或类型错误）
//
//go:nosplit
func ArrayGetHelper(arrPtr uintptr, index int64) (value int64, ok int64) {
	if arrPtr == 0 {
		return 0, 0
	}

	arr := (*bytecode.Value)(unsafe.Pointer(arrPtr))
	
	switch arr.Type {
	case bytecode.ValArray:
		if arr.Data == nil {
			return 0, 0
		}
		elements := arr.Data.([]bytecode.Value)
		if index < 0 || index >= int64(len(elements)) {
			return 0, 0 // 越界
		}
		elem := elements[index]
		return valueToInt64(elem), 1
		
	case bytecode.ValFixedArray:
		if arr.Data == nil {
			return 0, 0
		}
		fa := arr.Data.(*bytecode.FixedArray)
		if index < 0 || index >= int64(len(fa.Elements)) {
			return 0, 0
		}
		elem := fa.Elements[index]
		return valueToInt64(elem), 1
		
	case bytecode.ValBytes:
		if arr.Data == nil {
			return 0, 0
		}
		b := arr.Data.([]byte)
		if index < 0 || index >= int64(len(b)) {
			return 0, 0
		}
		return int64(b[index]), 1
		
	default:
		return 0, 0
	}
}

// ArraySetHelper 设置数组元素
// 参数：
//   - arrPtr: 指向 bytecode.Value 的指针
//   - index: 数组索引
//   - value: 要设置的值（int64 表示）
//
// 返回：1 表示成功，0 表示失败
//
//go:nosplit
func ArraySetHelper(arrPtr uintptr, index int64, value int64) int64 {
	if arrPtr == 0 {
		return 0
	}

	arr := (*bytecode.Value)(unsafe.Pointer(arrPtr))
	
	switch arr.Type {
	case bytecode.ValArray:
		if arr.Data == nil {
			return 0
		}
		elements := arr.Data.([]bytecode.Value)
		if index < 0 || index >= int64(len(elements)) {
			return 0
		}
		// 保持原有类型，更新值
		elements[index] = bytecode.NewInt(value)
		return 1
		
	case bytecode.ValFixedArray:
		if arr.Data == nil {
			return 0
		}
		fa := arr.Data.(*bytecode.FixedArray)
		if index < 0 || index >= int64(len(fa.Elements)) {
			return 0
		}
		fa.Elements[index] = bytecode.NewInt(value)
		return 1
		
	case bytecode.ValBytes:
		if arr.Data == nil {
			return 0
		}
		b := arr.Data.([]byte)
		if index < 0 || index >= int64(len(b)) {
			return 0
		}
		b[index] = byte(value)
		return 1
		
	default:
		return 0
	}
}

// ============================================================================
// 辅助函数
// ============================================================================

// valueToInt64 将 bytecode.Value 转换为 int64
func valueToInt64(v bytecode.Value) int64 {
	switch v.Type {
	case bytecode.ValInt:
		return v.AsInt()
	case bytecode.ValFloat:
		return FloatBitsToInt64(v.AsFloat())
	case bytecode.ValBool:
		if v.AsBool() {
			return 1
		}
		return 0
	default:
		return 0
	}
}

// ============================================================================
// 函数指针获取（用于 JIT 代码生成）
// ============================================================================

// GetArrayLenHelperPtr 获取 ArrayLenHelper 的函数指针
func GetArrayLenHelperPtr() uintptr {
	return getFuncPtr(ArrayLenHelper)
}

// GetArrayGetHelperPtr 获取 ArrayGetHelper 的函数指针
func GetArrayGetHelperPtr() uintptr {
	return getFuncPtr(ArrayGetHelper)
}

// GetArraySetHelperPtr 获取 ArraySetHelper 的函数指针
func GetArraySetHelperPtr() uintptr {
	return getFuncPtr(ArraySetHelper)
}

// getFuncPtr 获取函数指针
// 使用 Go 的反射机制安全地获取函数地址
func getFuncPtr(fn interface{}) uintptr {
	// Go 函数值是一个指向函数描述符的指针
	// 函数描述符的第一个字段是函数代码的地址
	return *(*uintptr)((*[2]unsafe.Pointer)(unsafe.Pointer(&fn))[1])
}

// ============================================================================
// 函数调用辅助函数
// ============================================================================

// CallContext 调用上下文，用于在JIT和VM之间传递信息
type CallContext struct {
	VM           unsafe.Pointer  // VM实例指针
	FunctionName string          // 函数名
	ClassName    string          // 类名（用于静态方法）
	MethodName   string          // 方法名
	Args         []int64         // 参数列表
	ArgCount     int             // 参数数量
	ReturnValue  int64           // 返回值
	Error        error           // 错误信息
}

// 全局调用上下文（简化实现，生产环境应使用线程本地存储）
var globalCallContext CallContext
var callContextMu sync.Mutex

// VMCallbackFunc VM 回调函数类型
// 用于从 JIT 代码调用 VM 执行
type VMCallbackFunc func(funcName string, args []int64) (int64, error)

// 全局 VM 回调
var vmCallback VMCallbackFunc

// SetVMCallback 设置 VM 回调函数
func SetVMCallback(cb VMCallbackFunc) {
	vmCallback = cb
}

// CallHelper 通用函数调用辅助函数
// 从函数表中查找目标函数并调用
//
//go:nosplit
func CallHelper() int64 {
	callContextMu.Lock()
	ctx := globalCallContext
	callContextMu.Unlock()
	
	if ctx.FunctionName == "" {
		return 0
	}
	
	// 尝试从函数表获取已编译函数
	ft := GetFunctionTable()
	if addr := ft.GetAddress(ctx.FunctionName); addr != 0 {
		// 直接调用已编译函数
		return callCompiledFunc(addr, ctx.Args)
	}
	
	// 回退到 VM 执行
	if vmCallback != nil {
		result, err := vmCallback(ctx.FunctionName, ctx.Args)
		if err != nil {
			ctx.Error = err
			return 0
		}
		return result
	}
	
	return 0
}

// CallHelperWithName 带函数名的调用辅助函数
// 参数通过寄存器传递：RCX=funcNamePtr, RDX=arg0, R8=arg1, R9=arg2
// 栈上传递更多参数
func CallHelperWithName(funcNamePtr uintptr, args ...int64) int64 {
	if funcNamePtr == 0 {
		return 0
	}
	
	// 从指针恢复函数名
	funcName := *(*string)(unsafe.Pointer(funcNamePtr))
	
	// 尝试从函数表获取
	ft := GetFunctionTable()
	if addr := ft.GetAddress(funcName); addr != 0 {
		return callCompiledFunc(addr, args)
	}
	
	// 回退到 VM
	if vmCallback != nil {
		result, _ := vmCallback(funcName, args)
		return result
	}
	
	return 0
}

// CallHelperById4 通过函数 ID 调用（最多 4 个参数）
// 参数：RCX=funcID, RDX=argCount, R8=arg0, R9=arg1，arg2/arg3 通过栈传递
// 返回：RAX=返回值
func CallHelperById4(funcID int32, argCount int64, arg0, arg1, arg2 int64) int64 {
	if funcID == 0 {
		return 0
	}
	
	ft := GetFunctionTable()
	
	// 通过 ID 获取函数条目
	entry, ok := ft.GetEntryByID(funcID)
	if !ok {
		return 0
	}
	
	// 回退到 VM（目前始终使用 VM 执行函数调用）
	if vmCallback != nil {
		// 根据 argCount 传递参数
		var args []int64
		switch argCount {
		case 0:
			args = nil
		case 1:
			args = []int64{arg0}
		case 2:
			args = []int64{arg0, arg1}
		case 3:
			args = []int64{arg0, arg1, arg2}
		default:
			args = []int64{arg0, arg1, arg2}
		}
		result, err := vmCallback(entry.FullName, args)
		if err != nil {
			return 0
		}
		return result
	}
	
	// vmCallback 为空，说明没有设置
	return 0
}

// CallHelperById1 通过函数 ID 调用（1 个参数）
func CallHelperById1(funcID int32, arg0 int64) int64 {
	if funcID == 0 {
		return 0
	}
	
	ft := GetFunctionTable()
	entry, ok := ft.GetEntryByID(funcID)
	if !ok {
		return 0
	}
	
	if vmCallback != nil {
		args := []int64{arg0}
		result, _ := vmCallback(entry.FullName, args)
		return result
	}
	
	return 0
}

// CallHelperById0 通过函数 ID 调用（无参数）
func CallHelperById0(funcID int32) int64 {
	if funcID == 0 {
		return 0
	}
	
	ft := GetFunctionTable()
	entry, ok := ft.GetEntryByID(funcID)
	if !ok {
		return 0
	}
	
	if vmCallback != nil {
		result, _ := vmCallback(entry.FullName, nil)
		return result
	}
	
	return 0
}

// GetCallHelperByIdPtr 获取 CallHelperById 的函数指针（根据参数数量）
func GetCallHelperByIdPtr() uintptr {
	return getFuncPtr(CallHelperById4)
}

// GetCallHelperById1Ptr 获取 1 参数版本
func GetCallHelperById1Ptr() uintptr {
	return getFuncPtr(CallHelperById1)
}

// GetCallHelperById0Ptr 获取无参数版本
func GetCallHelperById0Ptr() uintptr {
	return getFuncPtr(CallHelperById0)
}

// callCompiledFunc 调用已编译的函数
// 通过函数指针调用
func callCompiledFunc(addr uintptr, args []int64) int64 {
	if addr == 0 {
		return 0
	}
	
	// 根据参数数量选择调用方式
	switch len(args) {
	case 0:
		fn := *(*func() int64)(unsafe.Pointer(&addr))
		return fn()
	case 1:
		fn := *(*func(int64) int64)(unsafe.Pointer(&addr))
		return fn(args[0])
	case 2:
		fn := *(*func(int64, int64) int64)(unsafe.Pointer(&addr))
		return fn(args[0], args[1])
	case 3:
		fn := *(*func(int64, int64, int64) int64)(unsafe.Pointer(&addr))
		return fn(args[0], args[1], args[2])
	case 4:
		fn := *(*func(int64, int64, int64, int64) int64)(unsafe.Pointer(&addr))
		return fn(args[0], args[1], args[2], args[3])
	default:
		// 超过 4 个参数，使用通用调用
		// 这里简化处理，实际应该通过汇编实现
		fn := *(*func(int64, int64, int64, int64) int64)(unsafe.Pointer(&addr))
		return fn(args[0], args[1], args[2], args[3])
	}
}

// TailCallHelper 尾调用辅助函数
func TailCallHelper(funcNamePtr uintptr) int64 {
	if funcNamePtr == 0 {
		return 0
	}
	
	funcName := *(*string)(unsafe.Pointer(funcNamePtr))
	
	// 尾调用：尝试直接跳转到目标函数
	ft := GetFunctionTable()
	if addr := ft.GetAddress(funcName); addr != 0 {
		return callCompiledFunc(addr, nil)
	}
	
	return 0
}

// MethodCallHelper 方法调用辅助函数
// 参数：receiver - 对象指针
// 返回：方法返回值
func MethodCallHelper(receiver uintptr, methodNamePtr uintptr, args ...int64) int64 {
	if receiver == 0 || methodNamePtr == 0 {
		return 0
	}
	
	methodName := *(*string)(unsafe.Pointer(methodNamePtr))
	
	// 获取对象的类信息
	obj := (*bytecode.JITObject)(unsafe.Pointer(receiver))
	registry := bytecode.GetClassRegistry()
	
	layout, ok := registry.GetByID(obj.ClassID)
	if !ok {
		return 0
	}
	
	// 查找方法
	fullName := layout.ClassName + "::" + methodName
	ft := GetFunctionTable()
	
	if addr := ft.GetAddress(fullName); addr != 0 {
		// 将 receiver 作为第一个参数
		allArgs := make([]int64, len(args)+1)
		allArgs[0] = int64(receiver)
		copy(allArgs[1:], args)
		return callCompiledFunc(addr, allArgs)
	}
	
	// 尝试通过虚表调用
	if method, ok := layout.Methods[methodName]; ok && method.VTableIndex >= 0 {
		if method.VTableIndex < len(layout.VTable) && layout.VTable[method.VTableIndex] != 0 {
			allArgs := make([]int64, len(args)+1)
			allArgs[0] = int64(receiver)
			copy(allArgs[1:], args)
			return callCompiledFunc(layout.VTable[method.VTableIndex], allArgs)
		}
	}
	
	return 0
}

// StaticCallHelper 静态方法调用辅助函数
func StaticCallHelper(classNamePtr, methodNamePtr uintptr, args ...int64) int64 {
	if classNamePtr == 0 || methodNamePtr == 0 {
		return 0
	}
	
	className := *(*string)(unsafe.Pointer(classNamePtr))
	methodName := *(*string)(unsafe.Pointer(methodNamePtr))
	
	fullName := className + "::" + methodName
	ft := GetFunctionTable()
	
	if addr := ft.GetAddress(fullName); addr != 0 {
		return callCompiledFunc(addr, args)
	}
	
	// 回退到 VM
	if vmCallback != nil {
		result, _ := vmCallback(fullName, args)
		return result
	}
	
	return 0
}

// BuiltinCallHelper 内建函数调用辅助函数
func BuiltinCallHelper(builtinID int64, args ...int64) int64 {
	// 内建函数通过 ID 直接调用
	// 这里需要维护一个内建函数表
	return 0
}

// GetCallHelperPtr 获取通用调用辅助函数指针
func GetCallHelperPtr() uintptr {
	return getFuncPtr(CallHelper)
}

// GetCallHelperWithNamePtr 获取带函数名的调用辅助函数指针
func GetCallHelperWithNamePtr() uintptr {
	return getFuncPtr(CallHelperWithName)
}

// GetTailCallHelperPtr 获取尾调用辅助函数指针
func GetTailCallHelperPtr(funcName string) uintptr {
	return getFuncPtr(TailCallHelper)
}

// GetMethodCallHelperPtr 获取方法调用辅助函数指针
func GetMethodCallHelperPtr(methodName string) uintptr {
	return getFuncPtr(MethodCallHelper)
}

// GetStaticCallHelperPtr 获取静态方法调用辅助函数指针
func GetStaticCallHelperPtr() uintptr {
	return getFuncPtr(StaticCallHelper)
}

// GetBuiltinCallHelperPtr 获取内建函数调用辅助函数指针
func GetBuiltinCallHelperPtr(builtinName string) uintptr {
	return getFuncPtr(BuiltinCallHelper)
}

// ============================================================================
// 对象操作辅助函数
// ============================================================================

// NewObjectHelper 创建新对象
// 参数：classNamePtr - 类名字符串指针
// 返回：JITObject 指针
func NewObjectHelper(classNamePtr uintptr) uintptr {
	if classNamePtr == 0 {
		return 0
	}
	
	className := *(*string)(unsafe.Pointer(classNamePtr))
	
	// 获取类布局
	registry := bytecode.GetClassRegistry()
	layout := registry.GetOrCreate(className)
	
	if !layout.IsJITEnabled {
		// 类字段太多，不能使用 JIT 对象
		return 0
	}
	
	// 创建 JITObject
	obj := bytecode.NewJITObject(layout.ClassID, layout.VTablePtr, layout.FieldCount)
	
	return uintptr(unsafe.Pointer(obj))
}

// NewObjectWithClassIDHelper 通过类 ID 创建对象（更快）
func NewObjectWithClassIDHelper(classID int32) uintptr {
	registry := bytecode.GetClassRegistry()
	layout, ok := registry.GetByID(classID)
	if !ok || !layout.IsJITEnabled {
		return 0
	}
	
	obj := bytecode.NewJITObject(classID, layout.VTablePtr, layout.FieldCount)
	return uintptr(unsafe.Pointer(obj))
}

// GetFieldHelper 获取对象字段值
// 参数：objPtr - JITObject 指针，fieldIndex - 字段索引
// 返回：字段值
func GetFieldHelper(objPtr uintptr, fieldIndex int32) int64 {
	if objPtr == 0 {
		return 0
	}
	
	obj := (*bytecode.JITObject)(unsafe.Pointer(objPtr))
	return obj.GetField(int(fieldIndex))
}

// GetFieldByNameHelper 通过字段名获取值
func GetFieldByNameHelper(objPtr, fieldNamePtr uintptr) int64 {
	if objPtr == 0 || fieldNamePtr == 0 {
		return 0
	}
	
	obj := (*bytecode.JITObject)(unsafe.Pointer(objPtr))
	fieldName := *(*string)(unsafe.Pointer(fieldNamePtr))
	
	// 获取类布局
	registry := bytecode.GetClassRegistry()
	layout, ok := registry.GetByID(obj.ClassID)
	if !ok {
		return 0
	}
	
	// 查找字段索引
	fieldIndex := layout.GetFieldIndex(fieldName)
	if fieldIndex < 0 {
		return 0
	}
	
	return obj.GetField(fieldIndex)
}

// SetFieldHelper 设置对象字段值
// 参数：objPtr - JITObject 指针，fieldIndex - 字段索引，value - 值
func SetFieldHelper(objPtr uintptr, fieldIndex int32, value int64) {
	if objPtr == 0 {
		return
	}
	
	obj := (*bytecode.JITObject)(unsafe.Pointer(objPtr))
	obj.SetField(int(fieldIndex), value)
}

// SetFieldByNameHelper 通过字段名设置值
func SetFieldByNameHelper(objPtr, fieldNamePtr uintptr, value int64) {
	if objPtr == 0 || fieldNamePtr == 0 {
		return
	}
	
	obj := (*bytecode.JITObject)(unsafe.Pointer(objPtr))
	fieldName := *(*string)(unsafe.Pointer(fieldNamePtr))
	
	// 获取类布局
	registry := bytecode.GetClassRegistry()
	layout, ok := registry.GetByID(obj.ClassID)
	if !ok {
		return
	}
	
	// 查找字段索引
	fieldIndex := layout.GetFieldIndex(fieldName)
	if fieldIndex < 0 {
		return
	}
	
	obj.SetField(fieldIndex, value)
}

// GetNewObjectHelperPtr 获取对象创建辅助函数指针
func GetNewObjectHelperPtr(className string) uintptr {
	return getFuncPtr(NewObjectHelper)
}

// GetNewObjectWithClassIDHelperPtr 获取通过类ID创建对象的辅助函数指针
func GetNewObjectWithClassIDHelperPtr() uintptr {
	return getFuncPtr(NewObjectWithClassIDHelper)
}

// GetFieldHelperPtr 获取字段读取辅助函数指针
func GetFieldHelperPtr(fieldName string) uintptr {
	return getFuncPtr(GetFieldHelper)
}

// GetFieldByNameHelperPtr 获取通过字段名读取的辅助函数指针
func GetFieldByNameHelperPtr() uintptr {
	return getFuncPtr(GetFieldByNameHelper)
}

// GetSetFieldHelperPtr 获取字段写入辅助函数指针
func GetSetFieldHelperPtr(fieldName string) uintptr {
	return getFuncPtr(SetFieldHelper)
}

// GetSetFieldByNameHelperPtr 获取通过字段名写入的辅助函数指针
func GetSetFieldByNameHelperPtr() uintptr {
	return getFuncPtr(SetFieldByNameHelper)
}

// ============================================================================
// Map 操作辅助函数
// ============================================================================

// MapNewHelper 创建新的 JITMap
// 参数：capacity - 初始容量
// 返回：JITMap 指针
func MapNewHelper(capacity int64) uintptr {
	m := bytecode.NewJITMap(int32(capacity))
	return uintptr(unsafe.Pointer(m))
}

// MapGetHelper 获取 Map 中的值
// 参数：mapPtr - JITMap 指针，key - 键
// 返回：值（如果找到）或 0（如果未找到）
func MapGetHelper(mapPtr uintptr, key int64) int64 {
	if mapPtr == 0 {
		return 0
	}
	
	m := (*bytecode.JITMap)(unsafe.Pointer(mapPtr))
	if value, found := m.Get(key); found {
		return value
	}
	return 0
}

// MapGetWithFoundHelper 获取 Map 中的值，带找到标志
// 返回：高 32 位 = 是否找到 (1/0)，低 32 位 = 值
func MapGetWithFoundHelper(mapPtr uintptr, key int64) int64 {
	if mapPtr == 0 {
		return 0
	}
	
	m := (*bytecode.JITMap)(unsafe.Pointer(mapPtr))
	if value, found := m.Get(key); found {
		return value | int64(-1<<62) // 设置高两位为 11 表示找到
	}
	return 0
}

// MapSetHelper 设置 Map 中的值
// 参数：mapPtr - JITMap 指针，key - 键，value - 值
func MapSetHelper(mapPtr uintptr, key, value int64) {
	if mapPtr == 0 {
		return
	}
	
	m := (*bytecode.JITMap)(unsafe.Pointer(mapPtr))
	m.Set(key, value)
}

// MapHasHelper 检查 Map 中是否存在键
// 返回：1 存在，0 不存在
func MapHasHelper(mapPtr uintptr, key int64) int64 {
	if mapPtr == 0 {
		return 0
	}
	
	m := (*bytecode.JITMap)(unsafe.Pointer(mapPtr))
	if m.Has(key) {
		return 1
	}
	return 0
}

// MapLenHelper 获取 Map 长度
func MapLenHelper(mapPtr uintptr) int64 {
	if mapPtr == 0 {
		return 0
	}
	
	m := (*bytecode.JITMap)(unsafe.Pointer(mapPtr))
	return int64(m.Len())
}

// MapDeleteHelper 删除 Map 中的键
// 返回：1 删除成功，0 键不存在
func MapDeleteHelper(mapPtr uintptr, key int64) int64 {
	if mapPtr == 0 {
		return 0
	}
	
	m := (*bytecode.JITMap)(unsafe.Pointer(mapPtr))
	if m.Delete(key) {
		return 1
	}
	return 0
}

// GetMapNewHelperPtr 获取 Map 创建辅助函数指针
func GetMapNewHelperPtr() uintptr {
	return getFuncPtr(MapNewHelper)
}

// GetMapGetHelperPtr 获取 Map 读取辅助函数指针
func GetMapGetHelperPtr() uintptr {
	return getFuncPtr(MapGetHelper)
}

// GetMapGetWithFoundHelperPtr 获取带找到标志的 Map 读取辅助函数指针
func GetMapGetWithFoundHelperPtr() uintptr {
	return getFuncPtr(MapGetWithFoundHelper)
}

// GetMapSetHelperPtr 获取 Map 写入辅助函数指针
func GetMapSetHelperPtr() uintptr {
	return getFuncPtr(MapSetHelper)
}

// GetMapHasHelperPtr 获取 Map 存在检查辅助函数指针
func GetMapHasHelperPtr() uintptr {
	return getFuncPtr(MapHasHelper)
}

// GetMapLenHelperPtr 获取 Map 长度辅助函数指针
func GetMapLenHelperPtr() uintptr {
	return getFuncPtr(MapLenHelper)
}

// GetMapDeleteHelperPtr 获取 Map 删除辅助函数指针
func GetMapDeleteHelperPtr() uintptr {
	return getFuncPtr(MapDeleteHelper)
}

// ============================================================================
// 迭代器操作辅助函数
// ============================================================================

// IterInitHelper 初始化迭代器
// 参数：valuePtr - 指向要迭代的值（数组/Map/字符串）
// 返回：JITIterator 指针
func IterInitHelper(valuePtr uintptr) uintptr {
	if valuePtr == 0 {
		return 0
	}
	
	value := (*bytecode.Value)(unsafe.Pointer(valuePtr))
	iter := bytecode.JITIteratorFromValue(*value)
	return uintptr(unsafe.Pointer(iter))
}

// IterInitFromNativeArrayHelper 从 NativeArray 初始化迭代器
func IterInitFromNativeArrayHelper(arrPtr uintptr) uintptr {
	if arrPtr == 0 {
		return 0
	}
	
	arr := (*bytecode.NativeArray)(unsafe.Pointer(arrPtr))
	iter := bytecode.GetJITIterator()
	iter.InitFromNativeArray(arr)
	return uintptr(unsafe.Pointer(iter))
}

// IterInitFromJITMapHelper 从 JITMap 初始化迭代器
func IterInitFromJITMapHelper(mapPtr uintptr) uintptr {
	if mapPtr == 0 {
		return 0
	}
	
	m := (*bytecode.JITMap)(unsafe.Pointer(mapPtr))
	iter := bytecode.GetJITIterator()
	iter.InitFromJITMap(m)
	return uintptr(unsafe.Pointer(iter))
}

// IterNextHelper 移动到下一个元素
// 返回：1 有更多元素，0 完成
func IterNextHelper(iterPtr uintptr) int64 {
	if iterPtr == 0 {
		return 0
	}
	
	iter := (*bytecode.JITIterator)(unsafe.Pointer(iterPtr))
	if iter.Next() {
		return 1
	}
	return 0
}

// IterKeyHelper 获取当前键
func IterKeyHelper(iterPtr uintptr) int64 {
	if iterPtr == 0 {
		return 0
	}
	
	iter := (*bytecode.JITIterator)(unsafe.Pointer(iterPtr))
	return iter.Key()
}

// IterValueHelper 获取当前值
func IterValueHelper(iterPtr uintptr) int64 {
	if iterPtr == 0 {
		return 0
	}
	
	iter := (*bytecode.JITIterator)(unsafe.Pointer(iterPtr))
	return iter.Value()
}

// IterResetHelper 重置迭代器
func IterResetHelper(iterPtr uintptr) {
	if iterPtr == 0 {
		return
	}
	
	iter := (*bytecode.JITIterator)(unsafe.Pointer(iterPtr))
	iter.Reset()
}

// IterReleaseHelper 释放迭代器（放回池）
func IterReleaseHelper(iterPtr uintptr) {
	if iterPtr == 0 {
		return
	}
	
	iter := (*bytecode.JITIterator)(unsafe.Pointer(iterPtr))
	bytecode.PutJITIterator(iter)
}

// GetIterInitHelperPtr 获取迭代器初始化辅助函数指针
func GetIterInitHelperPtr() uintptr {
	return getFuncPtr(IterInitHelper)
}

// GetIterInitFromNativeArrayHelperPtr 获取从 NativeArray 初始化迭代器的辅助函数指针
func GetIterInitFromNativeArrayHelperPtr() uintptr {
	return getFuncPtr(IterInitFromNativeArrayHelper)
}

// GetIterInitFromJITMapHelperPtr 获取从 JITMap 初始化迭代器的辅助函数指针
func GetIterInitFromJITMapHelperPtr() uintptr {
	return getFuncPtr(IterInitFromJITMapHelper)
}

// GetIterNextHelperPtr 获取迭代器下一步辅助函数指针
func GetIterNextHelperPtr() uintptr {
	return getFuncPtr(IterNextHelper)
}

// GetIterKeyHelperPtr 获取迭代器键辅助函数指针
func GetIterKeyHelperPtr() uintptr {
	return getFuncPtr(IterKeyHelper)
}

// GetIterValueHelperPtr 获取迭代器值辅助函数指针
func GetIterValueHelperPtr() uintptr {
	return getFuncPtr(IterValueHelper)
}

// GetIterResetHelperPtr 获取迭代器重置辅助函数指针
func GetIterResetHelperPtr() uintptr {
	return getFuncPtr(IterResetHelper)
}

// GetIterReleaseHelperPtr 获取迭代器释放辅助函数指针
func GetIterReleaseHelperPtr() uintptr {
	return getFuncPtr(IterReleaseHelper)
}

// ============================================================================
// 闭包操作辅助函数
// ============================================================================

// ClosureNewHelper 创建新的闭包
// 参数：funcPtr - 函数地址，numUpvals - upvalue 数量
// 返回：JITClosure 指针
func ClosureNewHelper(funcPtr uintptr, numUpvals int32) uintptr {
	c := bytecode.NewJITClosure(funcPtr, int(numUpvals))
	return uintptr(unsafe.Pointer(c))
}

// ClosureGetUpvalueHelper 获取闭包的 upvalue
// 参数：closurePtr - JITClosure 指针，index - upvalue 索引
// 返回：upvalue 值
func ClosureGetUpvalueHelper(closurePtr uintptr, index int32) int64 {
	if closurePtr == 0 {
		return 0
	}
	
	c := (*bytecode.JITClosure)(unsafe.Pointer(closurePtr))
	return c.GetUpvalue(int(index))
}

// ClosureSetUpvalueHelper 设置闭包的 upvalue
// 参数：closurePtr - JITClosure 指针，index - upvalue 索引，value - 值
func ClosureSetUpvalueHelper(closurePtr uintptr, index int32, value int64) {
	if closurePtr == 0 {
		return
	}
	
	c := (*bytecode.JITClosure)(unsafe.Pointer(closurePtr))
	c.SetUpvalue(int(index), value)
}

// ClosureCallHelper 调用闭包
// 参数：closurePtr - JITClosure 指针，args - 参数
// 返回：返回值
func ClosureCallHelper(closurePtr uintptr, args ...int64) int64 {
	if closurePtr == 0 {
		return 0
	}
	
	c := (*bytecode.JITClosure)(unsafe.Pointer(closurePtr))
	if c.FuncPtr == 0 {
		return 0
	}
	
	// 调用闭包函数
	return callCompiledFunc(c.FuncPtr, args)
}

// GetClosureNewHelperPtr 获取闭包创建辅助函数指针
func GetClosureNewHelperPtr() uintptr {
	return getFuncPtr(ClosureNewHelper)
}

// GetClosureGetUpvalueHelperPtr 获取闭包 upvalue 读取辅助函数指针
func GetClosureGetUpvalueHelperPtr() uintptr {
	return getFuncPtr(ClosureGetUpvalueHelper)
}

// GetClosureSetUpvalueHelperPtr 获取闭包 upvalue 写入辅助函数指针
func GetClosureSetUpvalueHelperPtr() uintptr {
	return getFuncPtr(ClosureSetUpvalueHelper)
}

// GetClosureCallHelperPtr 获取闭包调用辅助函数指针
func GetClosureCallHelperPtr() uintptr {
	return getFuncPtr(ClosureCallHelper)
}

// ============================================================================
// 字符串操作辅助函数
// ============================================================================

// StringConcatHelper 字符串拼接
// 参数：aPtr, bPtr - 指向 bytecode.Value 的指针
// 返回：新字符串的 Value 指针
//
//go:nosplit
func StringConcatHelper(aPtr, bPtr uintptr) uintptr {
	if aPtr == 0 || bPtr == 0 {
		return 0
	}

	a := (*bytecode.Value)(unsafe.Pointer(aPtr))
	b := (*bytecode.Value)(unsafe.Pointer(bPtr))

	// 获取字符串内容
	var aStr, bStr string
	if a.Type == bytecode.ValString {
		aStr = a.AsString()
	} else {
		aStr = a.String()
	}
	if b.Type == bytecode.ValString {
		bStr = b.AsString()
	} else {
		bStr = b.String()
	}

	// 创建新字符串
	result := bytecode.NewString(aStr + bStr)
	return uintptr(unsafe.Pointer(&result))
}

// StringBuilderNewHelper 创建新的字符串构建器
// 返回：StringBuilder 的 Value 指针
//
//go:nosplit
func StringBuilderNewHelper() uintptr {
	sb := bytecode.NewStringBuilder()
	result := bytecode.NewStringBuilderValue(sb)
	return uintptr(unsafe.Pointer(&result))
}

// StringBuilderAddHelper 向字符串构建器添加内容
// 参数：sbPtr - StringBuilder 的 Value 指针，valPtr - 要添加的值的指针
// 返回：StringBuilder 的 Value 指针（支持链式调用）
//
//go:nosplit
func StringBuilderAddHelper(sbPtr, valPtr uintptr) uintptr {
	if sbPtr == 0 || valPtr == 0 {
		return 0
	}

	sbVal := (*bytecode.Value)(unsafe.Pointer(sbPtr))
	val := (*bytecode.Value)(unsafe.Pointer(valPtr))

	sb := sbVal.AsStringBuilder()
	if sb != nil {
		sb.AppendValue(*val)
	}

	return sbPtr
}

// StringBuilderBuildHelper 构建最终字符串
// 参数：sbPtr - StringBuilder 的 Value 指针
// 返回：字符串的 Value 指针
//
//go:nosplit
func StringBuilderBuildHelper(sbPtr uintptr) uintptr {
	if sbPtr == 0 {
		return 0
	}

	sbVal := (*bytecode.Value)(unsafe.Pointer(sbPtr))
	sb := sbVal.AsStringBuilder()
	if sb == nil {
		return 0
	}

	result := bytecode.NewString(sb.Build())
	return uintptr(unsafe.Pointer(&result))
}

// GetStringConcatHelperPtr 获取字符串拼接辅助函数指针
func GetStringConcatHelperPtr() uintptr {
	return getFuncPtr(StringConcatHelper)
}

// GetStringBuilderNewHelperPtr 获取字符串构建器创建辅助函数指针
func GetStringBuilderNewHelperPtr() uintptr {
	return getFuncPtr(StringBuilderNewHelper)
}

// GetStringBuilderAddHelperPtr 获取字符串构建器添加辅助函数指针
func GetStringBuilderAddHelperPtr() uintptr {
	return getFuncPtr(StringBuilderAddHelper)
}

// GetStringBuilderBuildHelperPtr 获取字符串构建器构建辅助函数指针
func GetStringBuilderBuildHelperPtr() uintptr {
	return getFuncPtr(StringBuilderBuildHelper)
}

// ============================================================================
// 数组创建辅助函数
// ============================================================================

// NewArrayHelper 创建新数组
// 参数：length - 数组长度，stackPtr - 栈指针（元素从栈上读取）
// 返回：数组的 Value 指针
//
//go:nosplit
func NewArrayHelper(length int64, stackBase uintptr) uintptr {
	if length < 0 {
		return 0
	}

	// 从栈上读取元素
	elements := make([]bytecode.Value, length)
	for i := int64(0); i < length; i++ {
		// 每个 Value 大小为 24 字节 (Type + Data)
		elemPtr := stackBase + uintptr(i)*24
		elem := (*bytecode.Value)(unsafe.Pointer(elemPtr))
		elements[i] = *elem
	}

	result := bytecode.NewArray(elements)
	return uintptr(unsafe.Pointer(&result))
}

// NewFixedArrayHelper 创建定长数组
// 参数：capacity - 容量，length - 初始长度，stackBase - 栈指针
// 返回：数组的 Value 指针
//
//go:nosplit
func NewFixedArrayHelper(capacity, length int64, stackBase uintptr) uintptr {
	if capacity < 0 || length < 0 || length > capacity {
		return 0
	}

	// 从栈上读取元素
	elements := make([]bytecode.Value, length)
	for i := int64(0); i < length; i++ {
		elemPtr := stackBase + uintptr(i)*24
		elem := (*bytecode.Value)(unsafe.Pointer(elemPtr))
		elements[i] = *elem
	}

	result := bytecode.NewFixedArrayWithElements(elements, int(capacity))
	return uintptr(unsafe.Pointer(&result))
}

// GetNewArrayHelperPtr 获取数组创建辅助函数指针
func GetNewArrayHelperPtr() uintptr {
	return getFuncPtr(NewArrayHelper)
}

// GetNewFixedArrayHelperPtr 获取定长数组创建辅助函数指针
func GetNewFixedArrayHelperPtr() uintptr {
	return getFuncPtr(NewFixedArrayHelper)
}
