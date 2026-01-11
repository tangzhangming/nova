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
		fmt.Printf("%v: %s\n", arg.Type(), arg.String())
	}
	return bytecode.NullValue
}

func builtinTypeof(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewString("null")
	}
	switch args[0].Type() {
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
	case bytecode.ValSuperArray:
		return bytecode.NewString("SuperArray")
	case bytecode.ValObject:
		obj := args[0].AsObject()
		if obj != nil && obj.Class != nil {
			return bytecode.NewString(obj.Class.Name)
		}
		return bytecode.NewString("unknown")
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
	return bytecode.NewBool(args[0].Type() == bytecode.ValNull)
}

func builtinIsBool(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	return bytecode.NewBool(args[0].Type() == bytecode.ValBool)
}

func builtinIsInt(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	return bytecode.NewBool(args[0].Type() == bytecode.ValInt)
}

func builtinIsFloat(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	return bytecode.NewBool(args[0].Type() == bytecode.ValFloat)
}

func builtinIsString(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	return bytecode.NewBool(args[0].Type() == bytecode.ValString)
}

func builtinIsArray(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	return bytecode.NewBool(args[0].Type() == bytecode.ValArray)
}

func builtinIsMap(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	return bytecode.NewBool(args[0].Type() == bytecode.ValMap)
}

func builtinIsObject(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	return bytecode.NewBool(args[0].Type() == bytecode.ValObject)
}

func builtinToInt(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.ZeroValue
	}
	v := args[0]
	if v.Type() == bytecode.ValString {
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
	switch v.Type() {
	case bytecode.ValString:
		return bytecode.NewInt(int64(len(v.AsString())))
	case bytecode.ValArray:
		return bytecode.NewInt(int64(len(v.AsArray())))
	case bytecode.ValBytes:
		return bytecode.NewInt(int64(len(v.AsBytes())))
	case bytecode.ValMap:
		return bytecode.NewInt(int64(len(v.AsMap())))
	case bytecode.ValSuperArray:
		return bytecode.NewInt(int64(v.AsSuperArray().Len()))
	default:
		return bytecode.ZeroValue
	}
}
