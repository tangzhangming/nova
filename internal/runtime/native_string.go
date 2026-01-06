package runtime

import (
	"strconv"
	"strings"

	"github.com/tangzhangming/nova/internal/bytecode"
)

// ============================================================================
// Native 字符串函数 (仅供标准库使用)
// ============================================================================

// nativeStrLen 获取字符串长度
func nativeStrLen(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.ZeroValue
	}
	return bytecode.NewInt(int64(len(args[0].AsString())))
}

// nativeStrSubstring 截取子串
// 参数：str, start, length(-1表示截取到末尾)
func nativeStrSubstring(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewString("")
	}
	s := args[0].AsString()
	start := int(args[1].AsInt())
	length := -1
	if len(args) > 2 {
		length = int(args[2].AsInt())
	}

	// 边界处理
	if start < 0 {
		start = 0
	}
	if start >= len(s) {
		return bytecode.NewString("")
	}

	// 计算结束位置
	var end int
	if length < 0 {
		end = len(s)
	} else {
		end = start + length
		if end > len(s) {
			end = len(s)
		}
	}

	return bytecode.NewString(s[start:end])
}

// nativeStrToUpper 转大写
func nativeStrToUpper(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewString("")
	}
	return bytecode.NewString(strings.ToUpper(args[0].AsString()))
}

// nativeStrToLower 转小写
func nativeStrToLower(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewString("")
	}
	return bytecode.NewString(strings.ToLower(args[0].AsString()))
}

// nativeStrTrim 去除首尾空白
func nativeStrTrim(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewString("")
	}
	return bytecode.NewString(strings.TrimSpace(args[0].AsString()))
}

// nativeStrReplace 替换字符串
// 参数：str, old, new
func nativeStrReplace(args []bytecode.Value) bytecode.Value {
	if len(args) < 3 {
		if len(args) > 0 {
			return args[0]
		}
		return bytecode.NewString("")
	}
	s := args[0].AsString()
	old := args[1].AsString()
	newStr := args[2].AsString()
	return bytecode.NewString(strings.ReplaceAll(s, old, newStr))
}

// nativeStrSplit 分割字符串
// 参数：str, delimiter
func nativeStrSplit(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewArray([]bytecode.Value{})
	}
	s := args[0].AsString()
	sep := args[1].AsString()
	parts := strings.Split(s, sep)
	result := make([]bytecode.Value, len(parts))
	for i, p := range parts {
		result[i] = bytecode.NewString(p)
	}
	return bytecode.NewArray(result)
}

// nativeStrJoin 连接数组
// 参数：arr, delimiter
func nativeStrJoin(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 || args[0].Type != bytecode.ValArray {
		return bytecode.NewString("")
	}
	arr := args[0].AsArray()
	sep := args[1].AsString()
	parts := make([]string, len(arr))
	for i, v := range arr {
		parts[i] = v.AsString()
	}
	return bytecode.NewString(strings.Join(parts, sep))
}

// nativeStrIndexOf 查找子串位置
// 参数：str, substr, fromIndex(可选，默认0)
func nativeStrIndexOf(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewInt(-1)
	}
	s := args[0].AsString()
	substr := args[1].AsString()
	fromIndex := 0
	if len(args) > 2 {
		fromIndex = int(args[2].AsInt())
	}

	// 边界处理
	if fromIndex < 0 {
		fromIndex = 0
	}
	if fromIndex >= len(s) {
		return bytecode.NewInt(-1)
	}

	// 从 fromIndex 开始查找
	idx := strings.Index(s[fromIndex:], substr)
	if idx == -1 {
		return bytecode.NewInt(-1)
	}
	return bytecode.NewInt(int64(fromIndex + idx))
}

// nativeStrLastIndexOf 从后往前查找
// 参数：str, substr
func nativeStrLastIndexOf(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewInt(-1)
	}
	s := args[0].AsString()
	substr := args[1].AsString()
	return bytecode.NewInt(int64(strings.LastIndex(s, substr)))
}

// nativeStrToInt 字符串转整数
func nativeStrToInt(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.ZeroValue
	}
	s := strings.TrimSpace(args[0].AsString())
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return bytecode.ZeroValue
	}
	return bytecode.NewInt(n)
}

// nativeStrToFloat 字符串转浮点数
func nativeStrToFloat(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewFloat(0)
	}
	s := strings.TrimSpace(args[0].AsString())
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return bytecode.NewFloat(0)
	}
	return bytecode.NewFloat(f)
}





