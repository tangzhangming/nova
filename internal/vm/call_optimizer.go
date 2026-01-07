package vm

import (
	"github.com/tangzhangming/nova/internal/bytecode"
	"github.com/tangzhangming/nova/internal/i18n"
)

// ============================================================================
// B5. 函数参数传递优化
// ============================================================================

// CallStats 调用统计
type CallStats struct {
	TotalCalls      int64 // 总调用次数
	FastPathCalls   int64 // 快速路径调用次数
	SlowPathCalls   int64 // 慢速路径调用次数
	InlinedCalls    int64 // 内联调用次数
	DefaultArgFills int64 // 默认参数填充次数
	VariadicCalls   int64 // 可变参数调用次数
}

var globalCallStats CallStats

// GetCallStats 获取调用统计
func GetCallStats() CallStats {
	return globalCallStats
}

// ResetCallStats 重置调用统计
func ResetCallStats() {
	globalCallStats = CallStats{}
}

// callOptimized 优化的函数调用
// 针对不同参数数量使用不同的快速路径
func (vm *VM) callOptimized(closure *bytecode.Closure, argCount int) InterpretResult {
	fn := closure.Function
	globalCallStats.TotalCalls++

	// 快速路径：简单函数（无可变参数，参数数量匹配）
	if !fn.IsVariadic && argCount == fn.Arity && len(closure.Upvalues) == 0 {
		return vm.callFast(closure, argCount)
	}

	// 标准路径
	globalCallStats.SlowPathCalls++
	return vm.callStandard(closure, argCount)
}

// callFast 快速调用路径
// 适用于：参数数量完全匹配、无可变参数、无 upvalue 的简单函数
func (vm *VM) callFast(closure *bytecode.Closure, argCount int) InterpretResult {
	globalCallStats.FastPathCalls++

	if vm.frameCount == FramesMax {
		return vm.runtimeError(i18n.T(i18n.ErrStackOverflow))
	}

	// 直接创建帧，无需额外处理
	frame := &vm.frames[vm.frameCount]
	vm.frameCount++
	frame.Closure = closure
	frame.IP = 0
	frame.BaseSlot = vm.stackTop - argCount - 1

	return InterpretOK
}

// callStandard 标准调用路径（处理默认参数、可变参数等复杂情况）
func (vm *VM) callStandard(closure *bytecode.Closure, argCount int) InterpretResult {
	fn := closure.Function

	// 检查参数数量
	if fn.IsVariadic {
		if argCount < fn.MinArity {
			return vm.runtimeError(i18n.T(i18n.ErrArgumentCountMin, fn.MinArity, argCount))
		}
	} else {
		if argCount < fn.MinArity {
			return vm.runtimeError(i18n.T(i18n.ErrArgumentCountMin, fn.MinArity, argCount))
		}
		if argCount > fn.Arity {
			return vm.runtimeError(i18n.T(i18n.ErrArgumentCountMax, fn.Arity, argCount))
		}
	}

	if vm.frameCount == FramesMax {
		return vm.runtimeError(i18n.T(i18n.ErrStackOverflow))
	}

	// 处理默认参数
	if !fn.IsVariadic && argCount < fn.Arity {
		argCount = vm.fillDefaultArgs(fn, argCount)
	}

	// 处理可变参数
	if fn.IsVariadic {
		argCount = vm.packVariadicArgs(fn, argCount)
	}

	// 创建帧
	frame := &vm.frames[vm.frameCount]
	vm.frameCount++
	frame.Closure = closure
	frame.IP = 0
	frame.BaseSlot = vm.stackTop - argCount - 1

	// 处理 upvalues
	if len(closure.Upvalues) > 0 {
		vm.pushUpvalues(closure)
	}

	return InterpretOK
}

// fillDefaultArgs 填充默认参数（优化版）
func (vm *VM) fillDefaultArgs(fn *bytecode.Function, argCount int) int {
	globalCallStats.DefaultArgFills++
	defaultStart := fn.MinArity
	defaultCount := fn.Arity - argCount

	// 批量填充：预先计算需要填充的数量，减少循环开销
	for i := 0; i < defaultCount; i++ {
		defaultIdx := argCount - defaultStart + i
		if defaultIdx >= 0 && defaultIdx < len(fn.DefaultValues) {
			vm.stack[vm.stackTop] = fn.DefaultValues[defaultIdx]
		} else {
			vm.stack[vm.stackTop] = bytecode.NullValue
		}
		vm.stackTop++
	}

	return fn.Arity
}

// packVariadicArgs 打包可变参数（优化版）
func (vm *VM) packVariadicArgs(fn *bytecode.Function, argCount int) int {
	globalCallStats.VariadicCalls++
	variadicCount := argCount - fn.MinArity

	if variadicCount > 0 {
		// 直接从栈上切片创建数组，避免逐个 pop
		startIdx := vm.stackTop - variadicCount
		varArgs := make([]bytecode.Value, variadicCount)
		copy(varArgs, vm.stack[startIdx:vm.stackTop])
		vm.stackTop = startIdx

		vm.stack[vm.stackTop] = bytecode.NewArray(varArgs)
		vm.stackTop++
		return fn.MinArity + 1
	}

	// 没有可变参数，推入空数组
	vm.stack[vm.stackTop] = bytecode.NewArray([]bytecode.Value{})
	vm.stackTop++
	return fn.MinArity + 1
}

// pushUpvalues 推入 upvalues（优化版）
func (vm *VM) pushUpvalues(closure *bytecode.Closure) {
	for _, upval := range closure.Upvalues {
		if upval.IsClosed {
			vm.stack[vm.stackTop] = upval.Closed
		} else {
			vm.stack[vm.stackTop] = *upval.Location
		}
		vm.stackTop++
	}
}

// ============================================================================
// 特化的小参数函数调用（进一步优化）
// ============================================================================

// callNoArgs 无参数函数调用（最快路径）
func (vm *VM) callNoArgs(closure *bytecode.Closure) InterpretResult {
	if vm.frameCount == FramesMax {
		return vm.runtimeError(i18n.T(i18n.ErrStackOverflow))
	}

	frame := &vm.frames[vm.frameCount]
	vm.frameCount++
	frame.Closure = closure
	frame.IP = 0
	frame.BaseSlot = vm.stackTop - 1 // 只有 caller

	return InterpretOK
}

// callOneArg 单参数函数调用
func (vm *VM) callOneArg(closure *bytecode.Closure) InterpretResult {
	if vm.frameCount == FramesMax {
		return vm.runtimeError(i18n.T(i18n.ErrStackOverflow))
	}

	frame := &vm.frames[vm.frameCount]
	vm.frameCount++
	frame.Closure = closure
	frame.IP = 0
	frame.BaseSlot = vm.stackTop - 2 // caller + 1 arg

	return InterpretOK
}

// callTwoArgs 双参数函数调用
func (vm *VM) callTwoArgs(closure *bytecode.Closure) InterpretResult {
	if vm.frameCount == FramesMax {
		return vm.runtimeError(i18n.T(i18n.ErrStackOverflow))
	}

	frame := &vm.frames[vm.frameCount]
	vm.frameCount++
	frame.Closure = closure
	frame.IP = 0
	frame.BaseSlot = vm.stackTop - 3 // caller + 2 args

	return InterpretOK
}

// ============================================================================
// 方法调用优化
// ============================================================================

// invokeMethodOptimized 优化的方法调用
func (vm *VM) invokeMethodOptimized(name string, argCount int) InterpretResult {
	receiver := vm.peek(argCount)

	// 处理 SuperArray 内置方法
	if receiver.Type == bytecode.ValSuperArray {
		return vm.invokeSuperArrayMethod(name, argCount)
	}

	if receiver.Type != bytecode.ValObject {
		return vm.runtimeError(i18n.T(i18n.ErrOnlyObjectsHaveMethods))
	}

	obj := receiver.AsObject()

	// 快速路径：使用内联缓存（如果已实现）
	// TODO: 集成内联缓存 (B1)

	// 查找方法
	method := vm.findMethodWithDefaults(obj.Class, name, argCount)
	if method == nil {
		return vm.runtimeError(i18n.T(i18n.ErrUndefinedMethod, name, argCount))
	}

	// 检查访问权限
	if err := vm.checkMethodAccess(obj.Class, method); err != nil {
		return vm.runtimeError("%v", err)
	}

	// 创建闭包并调用
	closure := &bytecode.Closure{
		Function: &bytecode.Function{
			Name:          method.Name,
			ClassName:     method.ClassName,
			SourceFile:    method.SourceFile,
			Arity:         method.Arity,
			MinArity:      method.MinArity,
			Chunk:         method.Chunk,
			LocalCount:    method.LocalCount,
			DefaultValues: method.DefaultValues,
		},
	}

	return vm.callOptimized(closure, argCount)
}

// ============================================================================
// 内置函数快速调用
// ============================================================================

// callBuiltinOptimized 优化的内置函数调用
func (vm *VM) callBuiltinOptimized(fn *bytecode.Function, argCount int) InterpretResult {
	globalCallStats.InlinedCalls++

	// 内置函数使用专门的快速路径
	if fn.BuiltinFn == nil {
		return vm.runtimeError("not a builtin function")
	}

	// 直接从栈上读取参数，避免创建切片
	switch argCount {
	case 0:
		result := fn.BuiltinFn(nil)
		return vm.handleBuiltinResult(result)

	case 1:
		args := []bytecode.Value{vm.stack[vm.stackTop-1]}
		vm.stackTop--
		result := fn.BuiltinFn(args)
		return vm.handleBuiltinResult(result)

	case 2:
		args := []bytecode.Value{
			vm.stack[vm.stackTop-2],
			vm.stack[vm.stackTop-1],
		}
		vm.stackTop -= 2
		result := fn.BuiltinFn(args)
		return vm.handleBuiltinResult(result)

	default:
		// 标准路径
		args := make([]bytecode.Value, argCount)
		for i := argCount - 1; i >= 0; i-- {
			vm.stackTop--
			args[i] = vm.stack[vm.stackTop]
		}
		result := fn.BuiltinFn(args)
		return vm.handleBuiltinResult(result)
	}
}

// handleBuiltinResult 处理内置函数返回值
func (vm *VM) handleBuiltinResult(result bytecode.Value) InterpretResult {
	// 弹出函数对象
	vm.stackTop--

	// 检查是否是异常
	if result.Type == bytecode.ValObject {
		if exc := result.AsException(); exc != nil {
			return vm.runtimeErrorWithException(exc)
		}
	}

	vm.stack[vm.stackTop] = result
	vm.stackTop++
	return InterpretOK
}

