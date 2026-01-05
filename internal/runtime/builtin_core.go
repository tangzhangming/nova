package runtime

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/tangzhangming/nova/internal/bytecode"
)

// ============================================================================
// 核心内置函数实现
// ============================================================================

func builtinPrint(args []bytecode.Value) bytecode.Value {
	for i, arg := range args {
		if i > 0 {
			fmt.Print(" ")
		}
		fmt.Print(arg.String())
	}
	fmt.Println()
	return bytecode.NullValue
}

func builtinPrintR(args []bytecode.Value) bytecode.Value {
	for _, arg := range args {
		fmt.Printf("%v: %s\n", arg.Type, arg.String())
	}
	return bytecode.NullValue
}

func builtinTypeof(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewString("null")
	}
	switch args[0].Type {
	case bytecode.ValNull:
		return bytecode.NewString("null")
	case bytecode.ValBool:
		return bytecode.NewString("bool")
	case bytecode.ValInt:
		return bytecode.NewString("int")
	case bytecode.ValFloat:
		return bytecode.NewString("float")
	case bytecode.ValString:
		return bytecode.NewString("string")
	case bytecode.ValArray:
		return bytecode.NewString("array")
	case bytecode.ValMap:
		return bytecode.NewString("map")
	case bytecode.ValObject:
		return bytecode.NewString("object")
	case bytecode.ValFunc, bytecode.ValClosure:
		return bytecode.NewString("function")
	default:
		return bytecode.NewString("unknown")
	}
}

func builtinIsNull(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.TrueValue
	}
	return bytecode.NewBool(args[0].Type == bytecode.ValNull)
}

func builtinIsBool(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	return bytecode.NewBool(args[0].Type == bytecode.ValBool)
}

func builtinIsInt(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	return bytecode.NewBool(args[0].Type == bytecode.ValInt)
}

func builtinIsFloat(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	return bytecode.NewBool(args[0].Type == bytecode.ValFloat)
}

func builtinIsString(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	return bytecode.NewBool(args[0].Type == bytecode.ValString)
}

func builtinIsArray(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	return bytecode.NewBool(args[0].Type == bytecode.ValArray)
}

func builtinIsMap(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	return bytecode.NewBool(args[0].Type == bytecode.ValMap)
}

func builtinIsObject(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	return bytecode.NewBool(args[0].Type == bytecode.ValObject)
}

func builtinToInt(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.ZeroValue
	}
	v := args[0]
	if v.Type == bytecode.ValString {
		s := strings.TrimSpace(v.AsString())
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return bytecode.ZeroValue
		}
		return bytecode.NewInt(n)
	}
	return bytecode.NewInt(v.AsInt())
}

func builtinToFloat(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewFloat(0)
	}
	return bytecode.NewFloat(args[0].AsFloat())
}

func builtinToString(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewString("")
	}
	return bytecode.NewString(args[0].String())
}

func builtinToBool(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	return bytecode.NewBool(args[0].IsTruthy())
}

func builtinLen(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.ZeroValue
	}
	v := args[0]
	switch v.Type {
	case bytecode.ValString:
		return bytecode.NewInt(int64(len(v.AsString())))
	case bytecode.ValArray:
		return bytecode.NewInt(int64(len(v.AsArray())))
	case bytecode.ValMap:
		return bytecode.NewInt(int64(len(v.AsMap())))
	default:
		return bytecode.ZeroValue
	}
}

func builtinPush(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 || args[0].Type != bytecode.ValArray {
		return bytecode.NullValue
	}
	arr := args[0].AsArray()
	arr = append(arr, args[1:]...)
	return bytecode.NewArray(arr)
}

func builtinPop(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 || args[0].Type != bytecode.ValArray {
		return bytecode.NullValue
	}
	arr := args[0].AsArray()
	if len(arr) == 0 {
		return bytecode.NullValue
	}
	return arr[len(arr)-1]
}

func builtinShift(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 || args[0].Type != bytecode.ValArray {
		return bytecode.NullValue
	}
	arr := args[0].AsArray()
	if len(arr) == 0 {
		return bytecode.NullValue
	}
	return arr[0]
}

func builtinUnshift(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 || args[0].Type != bytecode.ValArray {
		return bytecode.NullValue
	}
	arr := args[0].AsArray()
	newArr := make([]bytecode.Value, len(args)-1+len(arr))
	copy(newArr, args[1:])
	copy(newArr[len(args)-1:], arr)
	return bytecode.NewArray(newArr)
}

func builtinSlice(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 || args[0].Type != bytecode.ValArray {
		return bytecode.NullValue
	}
	arr := args[0].AsArray()
	start := int(args[1].AsInt())
	end := len(arr)
	if len(args) > 2 {
		end = int(args[2].AsInt())
	}
	if start < 0 {
		start = 0
	}
	if end > len(arr) {
		end = len(arr)
	}
	if start >= end {
		return bytecode.NewArray([]bytecode.Value{})
	}
	return bytecode.NewArray(arr[start:end])
}

func builtinConcat(args []bytecode.Value) bytecode.Value {
	var result []bytecode.Value
	for _, arg := range args {
		if arg.Type == bytecode.ValArray {
			result = append(result, arg.AsArray()...)
		}
	}
	return bytecode.NewArray(result)
}

func builtinReverse(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 || args[0].Type != bytecode.ValArray {
		return bytecode.NullValue
	}
	arr := args[0].AsArray()
	result := make([]bytecode.Value, len(arr))
	for i, v := range arr {
		result[len(arr)-1-i] = v
	}
	return bytecode.NewArray(result)
}

func builtinContains(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 || args[0].Type != bytecode.ValArray {
		return bytecode.FalseValue
	}
	arr := args[0].AsArray()
	target := args[1]
	for _, v := range arr {
		if v.Equals(target) {
			return bytecode.TrueValue
		}
	}
	return bytecode.FalseValue
}

func builtinIndexOf(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 || args[0].Type != bytecode.ValArray {
		return bytecode.NewInt(-1)
	}
	arr := args[0].AsArray()
	target := args[1]
	for i, v := range arr {
		if v.Equals(target) {
			return bytecode.NewInt(int64(i))
		}
	}
	return bytecode.NewInt(-1)
}

