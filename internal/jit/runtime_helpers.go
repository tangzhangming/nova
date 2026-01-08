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
