package runtime

import (
	"bytes"
	"encoding/hex"

	"github.com/tangzhangming/nova/internal/bytecode"
)

// ============================================================================
// Native Bytes 函数 (仅供标准库使用)
// ============================================================================

// nativeBytesNew 创建指定大小的字节数组（初始化为0）
func nativeBytesNew(args []bytecode.Value) bytecode.Value {
	size := 0
	if len(args) > 0 {
		size = int(args[0].AsInt())
		if size < 0 {
			size = 0
		}
	}
	return bytecode.NewBytes(make([]byte, size))
}

// nativeBytesFromString 从字符串创建字节数组
func nativeBytesFromString(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewBytes([]byte{})
	}
	s := args[0].AsString()
	return bytecode.NewBytes([]byte(s))
}

// nativeBytesToString 将字节数组转换为字符串
func nativeBytesToString(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 || args[0].Type != bytecode.ValBytes {
		return bytecode.NewString("")
	}
	b := args[0].AsBytes()
	return bytecode.NewString(string(b))
}

// nativeBytesFromHex 从十六进制字符串创建字节数组
func nativeBytesFromHex(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewBytes([]byte{})
	}
	hexStr := args[0].AsString()
	b, err := hex.DecodeString(hexStr)
	if err != nil {
		return bytecode.NewException("FormatException", "invalid hex string: "+err.Error(), 0)
	}
	return bytecode.NewBytes(b)
}

// nativeBytesToHex 将字节数组转换为十六进制字符串
func nativeBytesToHex(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 || args[0].Type != bytecode.ValBytes {
		return bytecode.NewString("")
	}
	b := args[0].AsBytes()
	return bytecode.NewString(hex.EncodeToString(b))
}

// nativeBytesFromArray 从整数数组创建字节数组
func nativeBytesFromArray(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 || args[0].Type != bytecode.ValArray {
		return bytecode.NewBytes([]byte{})
	}
	arr := args[0].AsArray()
	b := make([]byte, len(arr))
	for i, v := range arr {
		b[i] = byte(v.AsInt() & 0xFF)
	}
	return bytecode.NewBytes(b)
}

// nativeBytesToArray 将字节数组转换为整数数组
func nativeBytesToArray(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 || args[0].Type != bytecode.ValBytes {
		return bytecode.NewArray([]bytecode.Value{})
	}
	b := args[0].AsBytes()
	arr := make([]bytecode.Value, len(b))
	for i, v := range b {
		arr[i] = bytecode.NewInt(int64(v))
	}
	return bytecode.NewArray(arr)
}

// nativeBytesLen 获取字节数组长度
func nativeBytesLen(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 || args[0].Type != bytecode.ValBytes {
		return bytecode.ZeroValue
	}
	return bytecode.NewInt(int64(len(args[0].AsBytes())))
}

// nativeBytesGet 获取指定索引的字节
func nativeBytesGet(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 || args[0].Type != bytecode.ValBytes {
		return bytecode.ZeroValue
	}
	b := args[0].AsBytes()
	i := int(args[1].AsInt())
	if i < 0 || i >= len(b) {
		return bytecode.NewException("ArrayIndexOutOfBoundsException", "byte index out of bounds", 0)
	}
	return bytecode.NewInt(int64(b[i]))
}

// nativeBytesSet 设置指定索引的字节
func nativeBytesSet(args []bytecode.Value) bytecode.Value {
	if len(args) < 3 || args[0].Type != bytecode.ValBytes {
		return bytecode.NullValue
	}
	b := args[0].AsBytes()
	i := int(args[1].AsInt())
	v := byte(args[2].AsInt() & 0xFF)
	if i < 0 || i >= len(b) {
		return bytecode.NewException("ArrayIndexOutOfBoundsException", "byte index out of bounds", 0)
	}
	b[i] = v
	return bytecode.NullValue
}

// nativeBytesSlice 切片操作
func nativeBytesSlice(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 || args[0].Type != bytecode.ValBytes {
		return bytecode.NewBytes([]byte{})
	}
	b := args[0].AsBytes()
	start := int(args[1].AsInt())
	end := len(b)
	if len(args) > 2 {
		end = int(args[2].AsInt())
		if end < 0 {
			end = len(b)
		}
	}
	
	// 边界检查
	if start < 0 {
		start = 0
	}
	if start > len(b) {
		start = len(b)
	}
	if end > len(b) {
		end = len(b)
	}
	if start > end {
		start = end
	}
	
	result := make([]byte, end-start)
	copy(result, b[start:end])
	return bytecode.NewBytes(result)
}

// nativeBytesConcat 拼接两个字节数组
func nativeBytesConcat(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		if len(args) == 1 && args[0].Type == bytecode.ValBytes {
			// 复制并返回
			b := args[0].AsBytes()
			result := make([]byte, len(b))
			copy(result, b)
			return bytecode.NewBytes(result)
		}
		return bytecode.NewBytes([]byte{})
	}
	if args[0].Type != bytecode.ValBytes || args[1].Type != bytecode.ValBytes {
		return bytecode.NewBytes([]byte{})
	}
	b1 := args[0].AsBytes()
	b2 := args[1].AsBytes()
	result := make([]byte, len(b1)+len(b2))
	copy(result, b1)
	copy(result[len(b1):], b2)
	return bytecode.NewBytes(result)
}

// nativeBytesCopy 复制字节数组
func nativeBytesCopy(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 || args[0].Type != bytecode.ValBytes {
		return bytecode.NewBytes([]byte{})
	}
	b := args[0].AsBytes()
	result := make([]byte, len(b))
	copy(result, b)
	return bytecode.NewBytes(result)
}

// nativeBytesEqual 比较两个字节数组是否相等
func nativeBytesEqual(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.FalseValue
	}
	if args[0].Type != bytecode.ValBytes || args[1].Type != bytecode.ValBytes {
		return bytecode.FalseValue
	}
	b1 := args[0].AsBytes()
	b2 := args[1].AsBytes()
	return bytecode.NewBool(bytes.Equal(b1, b2))
}

// nativeBytesCompare 字典序比较两个字节数组
func nativeBytesCompare(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.ZeroValue
	}
	if args[0].Type != bytecode.ValBytes || args[1].Type != bytecode.ValBytes {
		return bytecode.ZeroValue
	}
	b1 := args[0].AsBytes()
	b2 := args[1].AsBytes()
	return bytecode.NewInt(int64(bytes.Compare(b1, b2)))
}

// nativeBytesIndex 查找子序列位置
func nativeBytesIndex(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewInt(-1)
	}
	if args[0].Type != bytecode.ValBytes || args[1].Type != bytecode.ValBytes {
		return bytecode.NewInt(-1)
	}
	b := args[0].AsBytes()
	sub := args[1].AsBytes()
	return bytecode.NewInt(int64(bytes.Index(b, sub)))
}

// nativeBytesContains 检查是否包含子序列
func nativeBytesContains(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.FalseValue
	}
	if args[0].Type != bytecode.ValBytes || args[1].Type != bytecode.ValBytes {
		return bytecode.FalseValue
	}
	b := args[0].AsBytes()
	sub := args[1].AsBytes()
	return bytecode.NewBool(bytes.Contains(b, sub))
}

// nativeBytesFill 用指定值填充字节数组
func nativeBytesFill(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 || args[0].Type != bytecode.ValBytes {
		return bytecode.NullValue
	}
	b := args[0].AsBytes()
	v := byte(args[1].AsInt() & 0xFF)
	for i := range b {
		b[i] = v
	}
	return bytecode.NullValue
}

// nativeBytesZero 将字节数组清零
func nativeBytesZero(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 || args[0].Type != bytecode.ValBytes {
		return bytecode.NullValue
	}
	b := args[0].AsBytes()
	for i := range b {
		b[i] = 0
	}
	return bytecode.NullValue
}







