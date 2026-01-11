// runtime_helpers.go - JIT 运行时辅助函数
//
// 本文件提供 JIT 编译代码与 Go 运行时交互的辅助函数。
// 这些函数被 JIT 代码通过函数指针调用，处理复杂的运行时操作。
//
// 注意：这些函数的签名必须与 JIT 代码生成中的调用约定匹配。

package jit

import (
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
	ReturnValue  int64           // 返回值
	Error        error           // 错误信息
}

// 全局调用上下文（简化实现，生产环境应使用线程本地存储）
var globalCallContext CallContext

// CallHelper 通用函数调用辅助函数
// 这是一个占位实现，实际调用应通过VM进行
//
//go:nosplit
func CallHelper() int64 {
	// 实际实现需要：
	// 1. 从调用上下文获取函数信息
	// 2. 通过VM解析函数地址
	// 3. 执行调用
	// 4. 返回结果
	return 0
}

// TailCallHelper 尾调用辅助函数
//
//go:nosplit
func TailCallHelper(funcName string) int64 {
	// 尾调用优化：复用当前栈帧
	return 0
}

// MethodCallHelper 方法调用辅助函数
// 参数：receiver - 对象指针
// 返回：方法返回值
//
//go:nosplit
func MethodCallHelper(receiver uintptr) int64 {
	if receiver == 0 {
		return 0
	}
	
	// 实际实现需要：
	// 1. 获取对象的类型信息
	// 2. 查找方法
	// 3. 执行方法调用
	return 0
}

// BuiltinCallHelper 内建函数调用辅助函数
//
//go:nosplit
func BuiltinCallHelper() int64 {
	return 0
}

// GetCallHelperPtr 获取通用调用辅助函数指针
func GetCallHelperPtr() uintptr {
	return getFuncPtr(CallHelper)
}

// GetTailCallHelperPtr 获取尾调用辅助函数指针
func GetTailCallHelperPtr(funcName string) uintptr {
	return getFuncPtr(TailCallHelper)
}

// GetMethodCallHelperPtr 获取方法调用辅助函数指针
func GetMethodCallHelperPtr(methodName string) uintptr {
	return getFuncPtr(MethodCallHelper)
}

// GetBuiltinCallHelperPtr 获取内建函数调用辅助函数指针
func GetBuiltinCallHelperPtr(builtinName string) uintptr {
	return getFuncPtr(BuiltinCallHelper)
}

// ============================================================================
// 对象操作辅助函数
// ============================================================================

// NewObjectHelper 创建新对象
// 返回：对象指针
//
//go:nosplit
func NewObjectHelper(classNamePtr uintptr) uintptr {
	// 实际实现需要：
	// 1. 获取类定义
	// 2. 分配对象内存
	// 3. 初始化字段
	// 4. 返回对象指针
	return 0
}

// GetFieldHelper 获取对象字段值
// 参数：
//   - objPtr: 对象指针
//   - fieldNamePtr: 字段名指针
// 返回：字段值（int64表示）
//
//go:nosplit
func GetFieldHelper(objPtr uintptr) int64 {
	if objPtr == 0 {
		return 0
	}
	
	// 实际实现需要：
	// 1. 获取对象类型信息
	// 2. 查找字段偏移
	// 3. 读取字段值
	return 0
}

// SetFieldHelper 设置对象字段值
// 参数：
//   - objPtr: 对象指针
//   - fieldNamePtr: 字段名指针
//   - value: 要设置的值
// 返回：1成功，0失败
//
//go:nosplit
func SetFieldHelper(objPtr uintptr, value int64) int64 {
	if objPtr == 0 {
		return 0
	}
	
	// 实际实现需要：
	// 1. 获取对象类型信息
	// 2. 查找字段偏移
	// 3. 写入字段值
	return 1
}

// GetNewObjectHelperPtr 获取对象创建辅助函数指针
func GetNewObjectHelperPtr(className string) uintptr {
	return getFuncPtr(NewObjectHelper)
}

// GetFieldHelperPtr 获取字段读取辅助函数指针
func GetFieldHelperPtr(fieldName string) uintptr {
	return getFuncPtr(GetFieldHelper)
}

// GetSetFieldHelperPtr 获取字段写入辅助函数指针
func GetSetFieldHelperPtr(fieldName string) uintptr {
	return getFuncPtr(SetFieldHelper)
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
