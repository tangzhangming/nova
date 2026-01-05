package runtime

import (
	"regexp"

	"github.com/tangzhangming/nova/internal/bytecode"
)

// ============================================================================
// Native 正则表达式函数
// ============================================================================

// nativeRegexMatch 检测是否匹配
func nativeRegexMatch(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.FalseValue
	}
	pattern := args[0].AsString()
	str := args[1].AsString()

	re, err := regexp.Compile(pattern)
	if err != nil {
		return bytecode.FalseValue
	}
	return bytecode.NewBool(re.MatchString(str))
}

// nativeRegexFind 查找第一个匹配
func nativeRegexFind(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewString("")
	}
	pattern := args[0].AsString()
	str := args[1].AsString()

	re, err := regexp.Compile(pattern)
	if err != nil {
		return bytecode.NewString("")
	}
	match := re.FindString(str)
	return bytecode.NewString(match)
}

// nativeRegexFindIndex 返回第一个匹配的位置 [start, end]
func nativeRegexFindIndex(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewArray([]bytecode.Value{})
	}
	pattern := args[0].AsString()
	str := args[1].AsString()

	re, err := regexp.Compile(pattern)
	if err != nil {
		return bytecode.NewArray([]bytecode.Value{})
	}
	loc := re.FindStringIndex(str)
	if loc == nil {
		return bytecode.NewArray([]bytecode.Value{})
	}
	return bytecode.NewArray([]bytecode.Value{
		bytecode.NewInt(int64(loc[0])),
		bytecode.NewInt(int64(loc[1])),
	})
}

// nativeRegexFindAll 查找所有匹配
func nativeRegexFindAll(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewArray([]bytecode.Value{})
	}
	pattern := args[0].AsString()
	str := args[1].AsString()

	re, err := regexp.Compile(pattern)
	if err != nil {
		return bytecode.NewArray([]bytecode.Value{})
	}
	matches := re.FindAllString(str, -1)
	result := make([]bytecode.Value, len(matches))
	for i, m := range matches {
		result[i] = bytecode.NewString(m)
	}
	return bytecode.NewArray(result)
}

// nativeRegexGroups 获取第一个匹配的捕获组
func nativeRegexGroups(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewArray([]bytecode.Value{})
	}
	pattern := args[0].AsString()
	str := args[1].AsString()

	re, err := regexp.Compile(pattern)
	if err != nil {
		return bytecode.NewArray([]bytecode.Value{})
	}
	groups := re.FindStringSubmatch(str)
	result := make([]bytecode.Value, len(groups))
	for i, g := range groups {
		result[i] = bytecode.NewString(g)
	}
	return bytecode.NewArray(result)
}

// nativeRegexFindAllGroups 获取所有匹配的捕获组
func nativeRegexFindAllGroups(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewArray([]bytecode.Value{})
	}
	pattern := args[0].AsString()
	str := args[1].AsString()

	re, err := regexp.Compile(pattern)
	if err != nil {
		return bytecode.NewArray([]bytecode.Value{})
	}
	allMatches := re.FindAllStringSubmatch(str, -1)
	result := make([]bytecode.Value, len(allMatches))
	for i, groups := range allMatches {
		groupValues := make([]bytecode.Value, len(groups))
		for j, g := range groups {
			groupValues[j] = bytecode.NewString(g)
		}
		result[i] = bytecode.NewArray(groupValues)
	}
	return bytecode.NewArray(result)
}

// nativeRegexReplace 替换第一个匹配
func nativeRegexReplace(args []bytecode.Value) bytecode.Value {
	if len(args) < 3 {
		return bytecode.NewString("")
	}
	pattern := args[0].AsString()
	str := args[1].AsString()
	replacement := args[2].AsString()

	re, err := regexp.Compile(pattern)
	if err != nil {
		return bytecode.NewString(str)
	}
	// Go regexp 没有直接的 ReplaceFirst，使用 ReplaceAllStringFunc 模拟
	replaced := false
	result := re.ReplaceAllStringFunc(str, func(match string) string {
		if !replaced {
			replaced = true
			return replacement
		}
		return match
	})
	return bytecode.NewString(result)
}

// nativeRegexReplaceAll 替换所有匹配
func nativeRegexReplaceAll(args []bytecode.Value) bytecode.Value {
	if len(args) < 3 {
		return bytecode.NewString("")
	}
	pattern := args[0].AsString()
	str := args[1].AsString()
	replacement := args[2].AsString()

	re, err := regexp.Compile(pattern)
	if err != nil {
		return bytecode.NewString(str)
	}
	return bytecode.NewString(re.ReplaceAllString(str, replacement))
}

// nativeRegexSplit 按模式分割字符串
func nativeRegexSplit(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewArray([]bytecode.Value{})
	}
	pattern := args[0].AsString()
	str := args[1].AsString()
	limit := -1
	if len(args) > 2 {
		limit = int(args[2].AsInt())
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return bytecode.NewArray([]bytecode.Value{bytecode.NewString(str)})
	}
	parts := re.Split(str, limit)
	result := make([]bytecode.Value, len(parts))
	for i, p := range parts {
		result[i] = bytecode.NewString(p)
	}
	return bytecode.NewArray(result)
}

// nativeRegexEscape 转义正则特殊字符
func nativeRegexEscape(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewString("")
	}
	str := args[0].AsString()
	return bytecode.NewString(regexp.QuoteMeta(str))
}

