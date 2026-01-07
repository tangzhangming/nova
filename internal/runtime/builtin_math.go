package runtime

import (
	"math"

	"github.com/tangzhangming/nova/internal/bytecode"
)

// ============================================================================
// Native 数学函数 (仅供标准库使用)
// ============================================================================

func nativeMathAbs(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.ZeroValue
	}
	v := args[0]
	switch v.Type {
	case bytecode.ValInt:
		n := v.AsInt()
		if n < 0 {
			return bytecode.NewInt(-n)
		}
		return v
	case bytecode.ValFloat:
		f := v.AsFloat()
		if f < 0 {
			return bytecode.NewFloat(-f)
		}
		return v
	default:
		return bytecode.ZeroValue
	}
}

func nativeMathMin(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NullValue
	}
	min := args[0]
	for _, v := range args[1:] {
		if v.AsFloat() < min.AsFloat() {
			min = v
		}
	}
	return min
}

func nativeMathMax(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NullValue
	}
	max := args[0]
	for _, v := range args[1:] {
		if v.AsFloat() > max.AsFloat() {
			max = v
		}
	}
	return max
}

func nativeMathFloor(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.ZeroValue
	}
	return bytecode.NewInt(int64(math.Floor(args[0].AsFloat())))
}

func nativeMathCeil(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.ZeroValue
	}
	return bytecode.NewInt(int64(math.Ceil(args[0].AsFloat())))
}

func nativeMathRound(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.ZeroValue
	}
	return bytecode.NewInt(int64(math.Round(args[0].AsFloat())))
}







