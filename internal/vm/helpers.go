package vm

import (
	"strings"

	"github.com/tangzhangming/nova/internal/bytecode"
)

// ============================================================================
// 算术 Helper
// 处理混合类型的算术运算
// 所有 Helper 函数使用 //go:noinline 确保有稳定的函数地址供 JIT 调用
// ============================================================================

// Helper_Add 加法 (处理类型转换)
//
//go:noinline
func Helper_Add(a, b bytecode.Value) bytecode.Value {
	// 整数
	if a.IsInt() && b.IsInt() {
		return bytecode.NewInt(a.AsInt() + b.AsInt())
	}

	// 浮点数
	if a.IsFloat() || b.IsFloat() {
		return bytecode.NewFloat(a.AsFloat() + b.AsFloat())
	}

	// 字符串拼接
	if a.IsString() || b.IsString() {
		return Helper_StringConcat(a, b)
	}

	// 默认返回 0
	return bytecode.ZeroValue
}

// Helper_Sub 减法
func Helper_Sub(a, b bytecode.Value) bytecode.Value {
	if a.IsInt() && b.IsInt() {
		return bytecode.NewInt(a.AsInt() - b.AsInt())
	}
	return bytecode.NewFloat(a.AsFloat() - b.AsFloat())
}

// Helper_Mul 乘法
//
//go:noinline
func Helper_Mul(a, b bytecode.Value) bytecode.Value {
	if a.IsInt() && b.IsInt() {
		return bytecode.NewInt(a.AsInt() * b.AsInt())
	}
	return bytecode.NewFloat(a.AsFloat() * b.AsFloat())
}

// Helper_Div 除法
func Helper_Div(a, b bytecode.Value) bytecode.Value {
	// 整数除法
	if a.IsInt() && b.IsInt() {
		bv := b.AsInt()
		if bv == 0 {
			return bytecode.NullValue // 除零返回 null
		}
		return bytecode.NewInt(a.AsInt() / bv)
	}

	// 浮点数除法
	bf := b.AsFloat()
	if bf == 0 {
		return bytecode.NullValue
	}
	return bytecode.NewFloat(a.AsFloat() / bf)
}

// Helper_Mod 取模
//
//go:noinline
func Helper_Mod(a, b bytecode.Value) bytecode.Value {
	ai, bi := a.AsInt(), b.AsInt()
	if bi == 0 {
		return bytecode.NullValue
	}
	return bytecode.NewInt(ai % bi)
}

// Helper_Neg 取负
//
//go:noinline
func Helper_Neg(a bytecode.Value) bytecode.Value {
	if a.IsInt() {
		return bytecode.NewInt(-a.AsInt())
	}
	return bytecode.NewFloat(-a.AsFloat())
}

// ============================================================================
// 比较 Helper
// ============================================================================

// Helper_Less 小于比较
//
//go:noinline
func Helper_Less(a, b bytecode.Value) bytecode.Value {
	if a.IsInt() && b.IsInt() {
		return bytecode.NewBool(a.AsInt() < b.AsInt())
	}
	return bytecode.NewBool(a.AsFloat() < b.AsFloat())
}

// Helper_LessEqual 小于等于比较
//
//go:noinline
func Helper_LessEqual(a, b bytecode.Value) bytecode.Value {
	if a.IsInt() && b.IsInt() {
		return bytecode.NewBool(a.AsInt() <= b.AsInt())
	}
	return bytecode.NewBool(a.AsFloat() <= b.AsFloat())
}

// Helper_Greater 大于比较
//
//go:noinline
func Helper_Greater(a, b bytecode.Value) bytecode.Value {
	if a.IsInt() && b.IsInt() {
		return bytecode.NewBool(a.AsInt() > b.AsInt())
	}
	return bytecode.NewBool(a.AsFloat() > b.AsFloat())
}

// Helper_GreaterEqual 大于等于比较
//
//go:noinline
func Helper_GreaterEqual(a, b bytecode.Value) bytecode.Value {
	if a.IsInt() && b.IsInt() {
		return bytecode.NewBool(a.AsInt() >= b.AsInt())
	}
	return bytecode.NewBool(a.AsFloat() >= b.AsFloat())
}

// ============================================================================
// 字符串 Helper
// ============================================================================

// Helper_StringConcat 字符串拼接
//
//go:noinline
func Helper_StringConcat(a, b bytecode.Value) bytecode.Value {
	return bytecode.NewString(a.String() + b.String())
}

// Helper_StringRepeat 字符串重复
func Helper_StringRepeat(s bytecode.Value, n bytecode.Value) bytecode.Value {
	str := s.AsString()
	count := int(n.AsInt())
	if count <= 0 {
		return bytecode.NewString("")
	}
	return bytecode.NewString(strings.Repeat(str, count))
}

// Helper_StringLen 字符串长度 (字节数)
func Helper_StringLen(s bytecode.Value) bytecode.Value {
	return bytecode.NewInt(int64(len(s.AsString())))
}

// Helper_StringRuneLen 字符串长度 (字符数)
func Helper_StringRuneLen(s bytecode.Value) bytecode.Value {
	return bytecode.NewInt(int64(len([]rune(s.AsString()))))
}

// Helper_StringSubstr 子字符串
func Helper_StringSubstr(s, start, length bytecode.Value) bytecode.Value {
	str := s.AsString()
	runes := []rune(str)
	
	startIdx := int(start.AsInt())
	subLen := int(length.AsInt())
	runeLen := len(runes)
	
	if startIdx < 0 {
		startIdx = 0
	}
	if startIdx >= runeLen {
		return bytecode.NewString("")
	}
	
	endIdx := startIdx + subLen
	if endIdx > runeLen {
		endIdx = runeLen
	}
	
	return bytecode.NewString(string(runes[startIdx:endIdx]))
}

// Helper_StringIndex 查找子串位置
func Helper_StringIndex(s, substr bytecode.Value) bytecode.Value {
	idx := strings.Index(s.AsString(), substr.AsString())
	return bytecode.NewInt(int64(idx))
}

// Helper_StringContains 检查是否包含子串
func Helper_StringContains(s, substr bytecode.Value) bytecode.Value {
	return bytecode.NewBool(strings.Contains(s.AsString(), substr.AsString()))
}

// Helper_StringUpper 转大写
func Helper_StringUpper(s bytecode.Value) bytecode.Value {
	return bytecode.NewString(strings.ToUpper(s.AsString()))
}

// Helper_StringLower 转小写
func Helper_StringLower(s bytecode.Value) bytecode.Value {
	return bytecode.NewString(strings.ToLower(s.AsString()))
}

// Helper_StringTrim 去除两端空白
func Helper_StringTrim(s bytecode.Value) bytecode.Value {
	return bytecode.NewString(strings.TrimSpace(s.AsString()))
}

// Helper_StringSplit 分割字符串
func Helper_StringSplit(s, sep bytecode.Value) bytecode.Value {
	parts := strings.Split(s.AsString(), sep.AsString())
	arr := make([]bytecode.Value, len(parts))
	for i, p := range parts {
		arr[i] = bytecode.NewString(p)
	}
	return bytecode.NewArray(arr)
}

// Helper_StringJoin 连接字符串数组
func Helper_StringJoin(arr, sep bytecode.Value) bytecode.Value {
	values := arr.AsArray()
	if values == nil {
		return bytecode.NewString("")
	}
	
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = v.String()
	}
	return bytecode.NewString(strings.Join(parts, sep.AsString()))
}

// ============================================================================
// SuperArray Helper
// ============================================================================

// Helper_SA_New 创建 SuperArray
//
//go:noinline
func Helper_SA_New() *bytecode.SuperArray {
	return bytecode.NewSuperArray()
}

// Helper_SA_Get 获取 SuperArray 元素
//
//go:noinline
func Helper_SA_Get(arr *bytecode.SuperArray, key bytecode.Value) bytecode.Value {
	if arr == nil {
		return bytecode.NullValue
	}
	val, _ := arr.Get(key)
	return val
}

// Helper_SA_GetInt 整数键获取 (快速路径)
func Helper_SA_GetInt(arr *bytecode.SuperArray, key int64) bytecode.Value {
	if arr == nil {
		return bytecode.NullValue
	}
	val, _ := arr.Get(bytecode.NewInt(key))
	return val
}

// Helper_SA_GetString 字符串键获取 (快速路径)
func Helper_SA_GetString(arr *bytecode.SuperArray, key string) bytecode.Value {
	if arr == nil {
		return bytecode.NullValue
	}
	val, _ := arr.Get(bytecode.NewString(key))
	return val
}

// Helper_SA_Set 设置 SuperArray 元素
//
//go:noinline
func Helper_SA_Set(arr *bytecode.SuperArray, key, val bytecode.Value) {
	if arr != nil {
		arr.Set(key, val)
	}
}

// Helper_SA_SetInt 整数键设置 (快速路径)
func Helper_SA_SetInt(arr *bytecode.SuperArray, key int64, val bytecode.Value) {
	if arr != nil {
		arr.Set(bytecode.NewInt(key), val)
	}
}

// Helper_SA_SetString 字符串键设置 (快速路径)
func Helper_SA_SetString(arr *bytecode.SuperArray, key string, val bytecode.Value) {
	if arr != nil {
		arr.Set(bytecode.NewString(key), val)
	}
}

// Helper_SA_Push 追加元素
func Helper_SA_Push(arr *bytecode.SuperArray, val bytecode.Value) {
	if arr != nil {
		arr.Push(val)
	}
}

// Helper_SA_Len 获取长度
//
//go:noinline
func Helper_SA_Len(arr *bytecode.SuperArray) int {
	if arr == nil {
		return 0
	}
	return arr.Len()
}

// Helper_SA_HasKey 检查键是否存在
func Helper_SA_HasKey(arr *bytecode.SuperArray, key bytecode.Value) bool {
	if arr == nil {
		return false
	}
	return arr.HasKey(key)
}

// Helper_SA_Remove 删除元素
func Helper_SA_Remove(arr *bytecode.SuperArray, key bytecode.Value) bool {
	if arr == nil {
		return false
	}
	return arr.Remove(key)
}

// Helper_SA_Keys 获取所有键
func Helper_SA_Keys(arr *bytecode.SuperArray) bytecode.Value {
	if arr == nil {
		return bytecode.NewArray(nil)
	}
	return bytecode.NewArray(arr.Keys())
}

// Helper_SA_Values 获取所有值
func Helper_SA_Values(arr *bytecode.SuperArray) bytecode.Value {
	if arr == nil {
		return bytecode.NewArray(nil)
	}
	return bytecode.NewArray(arr.Values())
}

// Helper_SA_Copy 复制 SuperArray
func Helper_SA_Copy(arr *bytecode.SuperArray) *bytecode.SuperArray {
	if arr == nil {
		return bytecode.NewSuperArray()
	}
	return arr.Copy()
}

// ============================================================================
// 比较 Helper
// ============================================================================

// Helper_Compare 通用比较 (返回 -1, 0, 1)
func Helper_Compare(a, b bytecode.Value) int {
	// 整数比较
	if a.IsInt() && b.IsInt() {
		ai, bi := a.AsInt(), b.AsInt()
		if ai < bi {
			return -1
		} else if ai > bi {
			return 1
		}
		return 0
	}

	// 浮点数比较
	af, bf := a.AsFloat(), b.AsFloat()
	if af < bf {
		return -1
	} else if af > bf {
		return 1
	}
	return 0
}

// ============================================================================
// 类型转换 Helper
// ============================================================================

// Helper_ToInt 转换为整数
func Helper_ToInt(v bytecode.Value) bytecode.Value {
	return bytecode.NewInt(v.AsInt())
}

// Helper_ToFloat 转换为浮点数
func Helper_ToFloat(v bytecode.Value) bytecode.Value {
	return bytecode.NewFloat(v.AsFloat())
}

// Helper_ToString 转换为字符串
func Helper_ToString(v bytecode.Value) bytecode.Value {
	return bytecode.NewString(v.String())
}

// Helper_ToBool 转换为布尔值
func Helper_ToBool(v bytecode.Value) bytecode.Value {
	return bytecode.NewBool(v.IsTruthy())
}

// ============================================================================
// 数组 Helper
// ============================================================================

// Helper_ArrayPush 数组追加
func Helper_ArrayPush(arr []bytecode.Value, val bytecode.Value) []bytecode.Value {
	return append(arr, val)
}

// Helper_ArrayPop 数组弹出
func Helper_ArrayPop(arr []bytecode.Value) ([]bytecode.Value, bytecode.Value) {
	if len(arr) == 0 {
		return arr, bytecode.NullValue
	}
	return arr[:len(arr)-1], arr[len(arr)-1]
}

// Helper_ArraySlice 数组切片
func Helper_ArraySlice(arr []bytecode.Value, start, end int) []bytecode.Value {
	if start < 0 {
		start = 0
	}
	if end > len(arr) {
		end = len(arr)
	}
	if start >= end {
		return nil
	}
	
	result := make([]bytecode.Value, end-start)
	copy(result, arr[start:end])
	return result
}

// Helper_ArrayConcat 数组拼接
func Helper_ArrayConcat(a, b []bytecode.Value) []bytecode.Value {
	result := make([]bytecode.Value, len(a)+len(b))
	copy(result, a)
	copy(result[len(a):], b)
	return result
}
