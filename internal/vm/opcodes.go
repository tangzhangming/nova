package vm

import (
	"fmt"

	"github.com/tangzhangming/nova/internal/bytecode"
)

// ============================================================================
// 栈操作
// ============================================================================

// opPush 加载常量到栈
func opPush(vm *VM) {
	v := vm.readConstant()
	vm.push(v)
}

// opPop 弹出栈顶
func opPop(vm *VM) {
	vm.pop()
}

// opDup 复制栈顶
func opDup(vm *VM) {
	vm.push(vm.peek(0))
}

// ============================================================================
// 常量加载
// ============================================================================

// opNull 加载 null
func opNull(vm *VM) {
	vm.push(bytecode.NullValue)
}

// opTrue 加载 true
func opTrue(vm *VM) {
	vm.push(bytecode.TrueValue)
}

// opFalse 加载 false
func opFalse(vm *VM) {
	vm.push(bytecode.FalseValue)
}

// opZero 加载 0
func opZero(vm *VM) {
	vm.push(bytecode.ZeroValue)
}

// opOne 加载 1
func opOne(vm *VM) {
	vm.push(bytecode.OneValue)
}

// ============================================================================
// 算术运算 (带快速路径)
// ============================================================================

// opAdd 加法
func opAdd(vm *VM) {
	b := vm.pop()
	a := vm.pop()

	// 快速路径：整数加法 (最常见)
	if a.IsInt() && b.IsInt() {
		vm.push(bytecode.NewInt(a.AsInt() + b.AsInt()))
		return
	}

	// 浮点数
	if a.IsFloat() || b.IsFloat() {
		vm.push(bytecode.NewFloat(a.AsFloat() + b.AsFloat()))
		return
	}

	// 字符串拼接
	if a.IsString() || b.IsString() {
		vm.push(Helper_StringConcat(a, b))
		return
	}

	// 默认：调用 Helper
	vm.push(Helper_Add(a, b))
}

// opConcat 字符串拼接
func opConcat(vm *VM) {
	b := vm.pop()
	a := vm.pop()
	vm.push(Helper_StringConcat(a, b))
}

// opSub 减法
func opSub(vm *VM) {
	b := vm.pop()
	a := vm.pop()

	// 快速路径：整数
	if a.IsInt() && b.IsInt() {
		vm.push(bytecode.NewInt(a.AsInt() - b.AsInt()))
		return
	}

	// 浮点数
	if a.IsFloat() || b.IsFloat() {
		vm.push(bytecode.NewFloat(a.AsFloat() - b.AsFloat()))
		return
	}

	vm.push(Helper_Sub(a, b))
}

// opMul 乘法
func opMul(vm *VM) {
	b := vm.pop()
	a := vm.pop()

	// 快速路径：整数
	if a.IsInt() && b.IsInt() {
		vm.push(bytecode.NewInt(a.AsInt() * b.AsInt()))
		return
	}

	// 浮点数
	if a.IsFloat() || b.IsFloat() {
		vm.push(bytecode.NewFloat(a.AsFloat() * b.AsFloat()))
		return
	}

	vm.push(Helper_Mul(a, b))
}

// opDiv 除法
func opDiv(vm *VM) {
	b := vm.pop()
	a := vm.pop()

	// 整数除法
	if a.IsInt() && b.IsInt() {
		bv := b.AsInt()
		if bv == 0 {
			vm.runtimeError("division by zero")
			return
		}
		vm.push(bytecode.NewInt(a.AsInt() / bv))
		return
	}

	// 浮点数除法
	bf := b.AsFloat()
	if bf == 0 {
		vm.runtimeError("division by zero")
		return
	}
	vm.push(bytecode.NewFloat(a.AsFloat() / bf))
}

// opMod 取模
func opMod(vm *VM) {
	b := vm.pop()
	a := vm.pop()

	if a.IsInt() && b.IsInt() {
		bv := b.AsInt()
		if bv == 0 {
			vm.runtimeError("modulo by zero")
			return
		}
		vm.push(bytecode.NewInt(a.AsInt() % bv))
		return
	}

	vm.runtimeError("modulo requires integer operands")
}

// opNeg 取负
func opNeg(vm *VM) {
	v := vm.pop()

	if v.IsInt() {
		vm.push(bytecode.NewInt(-v.AsInt()))
		return
	}

	if v.IsFloat() {
		vm.push(bytecode.NewFloat(-v.AsFloat()))
		return
	}

	vm.runtimeError("cannot negate non-numeric value")
}


// ============================================================================
// 位运算
// ============================================================================

// opBand 按位与
func opBand(vm *VM) {
	b := vm.pop()
	a := vm.pop()
	vm.push(bytecode.NewInt(a.AsInt() & b.AsInt()))
}

// opBor 按位或
func opBor(vm *VM) {
	b := vm.pop()
	a := vm.pop()
	vm.push(bytecode.NewInt(a.AsInt() | b.AsInt()))
}

// opBxor 按位异或
func opBxor(vm *VM) {
	b := vm.pop()
	a := vm.pop()
	vm.push(bytecode.NewInt(a.AsInt() ^ b.AsInt()))
}

// opBnot 按位取反
func opBnot(vm *VM) {
	v := vm.pop()
	vm.push(bytecode.NewInt(^v.AsInt()))
}

// opShl 左移
func opShl(vm *VM) {
	b := vm.pop()
	a := vm.pop()
	vm.push(bytecode.NewInt(a.AsInt() << uint(b.AsInt())))
}

// opShr 右移
func opShr(vm *VM) {
	b := vm.pop()
	a := vm.pop()
	vm.push(bytecode.NewInt(a.AsInt() >> uint(b.AsInt())))
}

// ============================================================================
// 比较运算
// ============================================================================

// opEq 相等
func opEq(vm *VM) {
	b := vm.pop()
	a := vm.pop()
	vm.push(bytecode.NewBool(a.Equals(b)))
}

// opNe 不等
func opNe(vm *VM) {
	b := vm.pop()
	a := vm.pop()
	vm.push(bytecode.NewBool(!a.Equals(b)))
}

// opLt 小于
func opLt(vm *VM) {
	b := vm.pop()
	a := vm.pop()

	// 快速路径：整数
	if a.IsInt() && b.IsInt() {
		vm.push(bytecode.NewBool(a.AsInt() < b.AsInt()))
		return
	}

	// 浮点数
	vm.push(bytecode.NewBool(a.AsFloat() < b.AsFloat()))
}

// opLe 小于等于
func opLe(vm *VM) {
	b := vm.pop()
	a := vm.pop()

	if a.IsInt() && b.IsInt() {
		vm.push(bytecode.NewBool(a.AsInt() <= b.AsInt()))
		return
	}

	vm.push(bytecode.NewBool(a.AsFloat() <= b.AsFloat()))
}

// opGt 大于
func opGt(vm *VM) {
	b := vm.pop()
	a := vm.pop()

	if a.IsInt() && b.IsInt() {
		vm.push(bytecode.NewBool(a.AsInt() > b.AsInt()))
		return
	}

	vm.push(bytecode.NewBool(a.AsFloat() > b.AsFloat()))
}

// opGe 大于等于
func opGe(vm *VM) {
	b := vm.pop()
	a := vm.pop()

	if a.IsInt() && b.IsInt() {
		vm.push(bytecode.NewBool(a.AsInt() >= b.AsInt()))
		return
	}

	vm.push(bytecode.NewBool(a.AsFloat() >= b.AsFloat()))
}

// ============================================================================
// 逻辑运算
// ============================================================================

// opNot 逻辑非
func opNot(vm *VM) {
	v := vm.pop()
	vm.push(bytecode.NewBool(!v.IsTruthy()))
}

// opAnd 逻辑与 (短路)
func opAnd(vm *VM) {
	b := vm.pop()
	a := vm.pop()
	if !a.IsTruthy() {
		vm.push(a)
	} else {
		vm.push(b)
	}
}

// opOr 逻辑或 (短路)
func opOr(vm *VM) {
	b := vm.pop()
	a := vm.pop()
	if a.IsTruthy() {
		vm.push(a)
	} else {
		vm.push(b)
	}
}

// ============================================================================
// 变量操作
// ============================================================================

// opLoadLocal 加载局部变量
func opLoadLocal(vm *VM) {
	slot := int(vm.readShort())
	val := vm.getLocal(slot)
	vm.push(val)
}

// opStoreLocal 存储局部变量
func opStoreLocal(vm *VM) {
	slot := int(vm.readShort())
	val := vm.peek(0)
	vm.setLocal(slot, val)
}

// opLoadGlobal 加载全局变量或全局函数
func opLoadGlobal(vm *VM) {
	index := int(vm.readShort())
	frame := vm.currentFrame()

	// 从常量池获取名称
	if index < len(frame.chunk.Constants) {
		c := frame.chunk.Constants[index]
		if c.IsString() {
			name := c.AsString()
			// 优先查找已注册的函数
			if fn := vm.GetFunction(name); fn != nil {
				vm.push(bytecode.NewFunc(fn))
				return
			}
			// 注意：类的查找需要其他方式处理，这里暂不支持
		}
	}

	// 回退到按索引查找全局变量
	val := vm.GetGlobal(index)
	vm.push(val)
}

// opStoreGlobal 存储全局变量
func opStoreGlobal(vm *VM) {
	index := int(vm.readShort())
	vm.SetGlobal(index, vm.peek(0))
}


// ============================================================================
// 跳转
// ============================================================================

// opJump 无条件跳转
func opJump(vm *VM) {
	offset := int(vm.readShort())
	frame := vm.currentFrame()
	frame.ip += offset
}

// opJumpIfFalse 条件为假时跳转
func opJumpIfFalse(vm *VM) {
	offset := int(vm.readShort())
	if !vm.peek(0).IsTruthy() {
		frame := vm.currentFrame()
		frame.ip += offset
	}
}

// opJumpIfTrue 条件为真时跳转
func opJumpIfTrue(vm *VM) {
	offset := int(vm.readShort())
	if vm.peek(0).IsTruthy() {
		frame := vm.currentFrame()
		frame.ip += offset
	}
}

// opLoop 循环跳转 (向后跳)
func opLoop(vm *VM) {
	offset := int(vm.readShort())
	frame := vm.currentFrame()
	frame.ip -= offset
}

// ============================================================================
// 函数调用
// ============================================================================

// opCall 调用函数
func opCall(vm *VM) {
	argCount := int(vm.readByte())

	// 获取被调用者
	callee := vm.stack[vm.sp-argCount-1]

	switch callee.Type() {
	case bytecode.ValFunc:
		fn := callee.AsFunc()
		if fn == nil {
			vm.runtimeError("cannot call nil function")
			return
		}
		vm.callFunction(fn, argCount)

	case bytecode.ValClosure:
		closure := callee.AsClosure()
		if closure == nil {
			vm.runtimeError("cannot call nil closure")
			return
		}
		vm.callClosure(closure, argCount)

	default:
		vm.runtimeError("can only call functions and closures")
	}
}

// callFunction 调用普通函数
func (vm *VM) callFunction(fn *bytecode.Function, argCount int) {
	// 处理内置函数
	if fn.IsBuiltin && fn.BuiltinFn != nil {
		// 收集参数
		args := make([]bytecode.Value, argCount)
		for i := argCount - 1; i >= 0; i-- {
			args[i] = vm.pop()
		}
		// 弹出函数本身
		vm.pop()
		// 调用内置函数
		result := fn.BuiltinFn(args)
		// 压入结果
		vm.push(result)
		return
	}

	// 处理参数数量不匹配
	if argCount < fn.MinArity {
		// 填充默认参数
		for i := argCount; i < fn.Arity; i++ {
			defIdx := i - fn.MinArity
			if defIdx >= 0 && defIdx < len(fn.DefaultValues) {
				vm.push(fn.DefaultValues[defIdx])
			} else {
				vm.push(bytecode.NullValue)
			}
		}
		argCount = fn.Arity
	}

	// 计算基指针
	bp := vm.sp - argCount

	// 压入调用帧
	vm.pushFrame(fn, bp)
}

// callClosure 调用闭包
func (vm *VM) callClosure(closure *bytecode.Closure, argCount int) {
	fn := closure.Function

	// 处理参数数量不匹配
	if argCount < fn.MinArity {
		for i := argCount; i < fn.Arity; i++ {
			defIdx := i - fn.MinArity
			if defIdx >= 0 && defIdx < len(fn.DefaultValues) {
				vm.push(fn.DefaultValues[defIdx])
			} else {
				vm.push(bytecode.NullValue)
			}
		}
		argCount = fn.Arity
	}

	bp := vm.sp - argCount
	vm.pushClosureFrame(closure, bp)
}

// opReturn 函数返回
func opReturn(vm *VM) {
	// 获取返回值
	result := vm.pop()

	// 弹出当前帧
	frame := vm.popFrame()

	// 清理栈上的局部变量和参数
	vm.sp = frame.bp

	// 弹出被调用者 (如果有)
	// 注意：静态方法调用没有被调用者，不需要弹出
	if !frame.isStaticCall && vm.sp > 0 {
		vm.sp--
	}

	// 压入返回值
	vm.push(result)
}

// opReturnNull 返回 null
func opReturnNull(vm *VM) {
	// 弹出当前帧
	frame := vm.popFrame()

	// 清理栈上的局部变量和参数
	vm.sp = frame.bp

	// 弹出被调用者 (如果有)
	// 注意：静态方法调用没有被调用者，不需要弹出
	if !frame.isStaticCall && vm.sp > 0 {
		vm.sp--
	}

	// 压入 null 作为返回值
	vm.push(bytecode.NullValue)
}

// opCallStatic 调用静态方法
// 字节码格式: OpCallStatic + classIdx(u16) + methodIdx(u16) + argCount(u8)
func opCallStatic(vm *VM) {
	// 读取类索引和方法索引
	classIndex := vm.readShort()
	methodIndex := vm.readShort()
	argCount := int(vm.readByte())

	frame := vm.currentFrame()
	
	if int(classIndex) >= len(frame.chunk.Constants) {
		vm.runtimeError("invalid class index: %d", classIndex)
		return
	}
	
	classNameVal := frame.chunk.Constants[classIndex]
	if !classNameVal.IsString() {
		vm.runtimeError("class name must be string, got %s", classNameVal.Type())
		return
	}
	
	className := classNameVal.AsString()
	
	if int(methodIndex) >= len(frame.chunk.Constants) {
		vm.runtimeError("invalid method index: %d", methodIndex)
		return
	}
	
	methodNameVal := frame.chunk.Constants[methodIndex]
	if !methodNameVal.IsString() {
		vm.runtimeError("method name must be string")
		return
	}
	
	methodName := methodNameVal.AsString()
	
	// 查找类
	class := vm.GetClass(className)
	if class == nil {
		vm.runtimeError("undefined class: %s", className)
		return
	}
	
	// 查找方法
	method := class.GetMethod(methodName)
	if method == nil {
		vm.runtimeError("undefined method: %s.%s", className, methodName)
		return
	}
	
	// 使用方法上的缓存 Function
	fn := method.CachedFunction
	if fn == nil {
		// 首次访问，创建并缓存
		fn = &bytecode.Function{
			Name:       method.Name,
			ClassName:  method.ClassName,
			SourceFile: method.SourceFile,
			Arity:      method.Arity,
			MinArity:   method.MinArity,
			Chunk:      method.Chunk,
			LocalCount: method.LocalCount,
		}
		method.CachedFunction = fn
	}
	
	// 计算基指针（参数已经在栈上了）
	bp := vm.sp - argCount
	
	// 直接使用缓存的 Function
	vm.pushStaticFrame(fn, bp)
}

// opClosure 创建闭包
func opClosure(vm *VM) {
	// 读取函数索引
	fnVal := vm.readConstant()
	fn := fnVal.AsFunc()
	if fn == nil {
		vm.runtimeError("closure requires function")
		return
	}

	// 创建闭包
	closure := &bytecode.Closure{
		Function: fn,
		Upvalues: make([]*bytecode.Upvalue, fn.UpvalueCount),
	}

	// 读取 upvalue 信息
	for i := 0; i < fn.UpvalueCount; i++ {
		isLocal := vm.readByte() == 1
		index := int(vm.readShort())

		if isLocal {
			// 捕获局部变量
			frame := vm.currentFrame()
			closure.Upvalues[i] = &bytecode.Upvalue{
				Location: &vm.stack[frame.bp+index],
			}
		} else {
			// 从外层闭包继承
			frame := vm.currentFrame()
			if frame.closure != nil && index < len(frame.closure.Upvalues) {
				closure.Upvalues[i] = frame.closure.Upvalues[index]
			}
		}
	}

	vm.push(bytecode.NewClosure(closure))
}

// ============================================================================
// 对象操作
// ============================================================================

// opNewInstance 创建对象实例
func opNewInstance(vm *VM) {
	classNameVal := vm.readConstant()
	className := classNameVal.AsString()

	class := vm.GetClass(className)
	if class == nil {
		vm.runtimeError("unknown class: %s", className)
		return
	}

	obj := bytecode.NewObjectInstance(class)
	vm.push(bytecode.NewObject(obj))
}

// opGetField 获取字段
func opGetField(vm *VM) {
	fieldNameVal := vm.readConstant()
	fieldName := fieldNameVal.AsString()

	objVal := vm.pop()
	if !objVal.IsObject() {
		vm.runtimeError("cannot get field of non-object")
		return
	}

	obj := objVal.AsObject()
	if val, ok := obj.GetField(fieldName); ok {
		vm.push(val)
	} else {
		vm.push(bytecode.NullValue)
	}
}

// opSetField 设置字段
func opSetField(vm *VM) {
	fieldNameVal := vm.readConstant()
	fieldName := fieldNameVal.AsString()

	val := vm.pop()
	objVal := vm.pop()

	if !objVal.IsObject() {
		vm.runtimeError("cannot set field of non-object")
		return
	}

	obj := objVal.AsObject()
	obj.SetField(fieldName, val)
	vm.push(val)
}

// opInvoke 调用方法
func opInvoke(vm *VM) {
	methodNameVal := vm.readConstant()
	methodName := methodNameVal.AsString()
	argCount := int(vm.readByte())

	// 获取对象
	receiver := vm.stack[vm.sp-argCount-1]
	if !receiver.IsObject() {
		vm.runtimeError("cannot invoke method on non-object")
		return
	}

	obj := receiver.AsObject()
	method := obj.Class.GetMethodByArity(methodName, argCount)
	if method == nil {
		vm.runtimeError("undefined method: %s", methodName)
		return
	}

	// 调用方法
	vm.callMethod(method, argCount)
}

// callMethod 调用方法
func (vm *VM) callMethod(method *bytecode.Method, argCount int) {
	// 处理参数
	if argCount < method.MinArity {
		for i := argCount; i < method.Arity; i++ {
			defIdx := i - method.MinArity
			if defIdx >= 0 && defIdx < len(method.DefaultValues) {
				vm.push(method.DefaultValues[defIdx])
			} else {
				vm.push(bytecode.NullValue)
			}
		}
		argCount = method.Arity
	}

	bp := vm.sp - argCount - 1 // -1 for receiver

	// 创建一个临时函数包装方法
	fn := &bytecode.Function{
		Name:       method.Name,
		Arity:      method.Arity,
		MinArity:   method.MinArity,
		Chunk:      method.Chunk,
		LocalCount: method.LocalCount,
	}

	vm.pushFrame(fn, bp)
}

// ============================================================================
// 数组操作
// ============================================================================

// opNewArray 创建数组
func opNewArray(vm *VM) {
	count := int(vm.readShort())

	arr := make([]bytecode.Value, count)
	for i := count - 1; i >= 0; i-- {
		arr[i] = vm.pop()
	}

	vm.push(bytecode.NewArray(arr))
}

// opArrayGet 数组取值
func opArrayGet(vm *VM) {
	index := vm.pop()
	arrVal := vm.pop()

	arr := arrVal.AsArray()
	if arr == nil {
		vm.runtimeError("cannot index non-array")
		return
	}

	idx := int(index.AsInt())
	if idx < 0 || idx >= len(arr) {
		vm.push(bytecode.NullValue)
		return
	}

	vm.push(arr[idx])
}

// opArraySet 数组赋值
func opArraySet(vm *VM) {
	val := vm.pop()
	index := vm.pop()
	arrVal := vm.pop()

	arr := arrVal.AsArray()
	if arr == nil {
		vm.runtimeError("cannot index non-array")
		return
	}

	idx := int(index.AsInt())
	if idx >= 0 && idx < len(arr) {
		arr[idx] = val
	}

	vm.push(val)
}

// opArrayLen 数组长度
func opArrayLen(vm *VM) {
	arrVal := vm.pop()
	arr := arrVal.AsArray()
	if arr == nil {
		vm.push(bytecode.ZeroValue)
		return
	}
	vm.push(bytecode.NewInt(int64(len(arr))))
}

// ============================================================================
// SuperArray 操作
// ============================================================================

// opSuperArrayNew 创建 SuperArray
func opSuperArrayNew(vm *VM) {
	count := int(vm.readShort())

	sa := bytecode.NewSuperArray()

	// 读取键值对
	for i := 0; i < count; i++ {
		val := vm.pop()
		key := vm.pop()
		sa.Set(key, val)
	}

	vm.push(bytecode.NewSuperArrayValue(sa))
}

// opSuperArrayGet 获取 SuperArray 元素
func opSuperArrayGet(vm *VM) {
	key := vm.pop()
	saVal := vm.pop()

	sa := saVal.AsSuperArray()
	if sa == nil {
		vm.push(bytecode.NullValue)
		return
	}

	if val, ok := sa.Get(key); ok {
		vm.push(val)
	} else {
		vm.push(bytecode.NullValue)
	}
}

// opSuperArraySet 设置 SuperArray 元素
func opSuperArraySet(vm *VM) {
	val := vm.pop()
	key := vm.pop()
	saVal := vm.pop()

	sa := saVal.AsSuperArray()
	if sa == nil {
		vm.runtimeError("cannot set value on non-SuperArray")
		return
	}

	sa.Set(key, val)
	vm.push(val)
}

// ============================================================================
// 迭代器
// ============================================================================

// opIterNew 创建迭代器
func opIterNew(vm *VM) {
	v := vm.pop()
	iter := bytecode.NewIterator(v)
	vm.push(bytecode.NewIteratorValue(iter))
}

// opIterNext 迭代器下一个
func opIterNext(vm *VM) {
	iterVal := vm.peek(0)
	iter := iterVal.AsIterator()
	if iter == nil {
		vm.push(bytecode.FalseValue)
		return
	}
	vm.push(bytecode.NewBool(iter.Next()))
}

// opIterKey 获取迭代器键
func opIterKey(vm *VM) {
	iterVal := vm.peek(0)
	iter := iterVal.AsIterator()
	if iter == nil {
		vm.push(bytecode.NullValue)
		return
	}
	vm.push(iter.Key())
}

// opIterValue 获取迭代器值
func opIterValue(vm *VM) {
	iterVal := vm.peek(0)
	iter := iterVal.AsIterator()
	if iter == nil {
		vm.push(bytecode.NullValue)
		return
	}
	vm.push(iter.CurrentValue())
}

// ============================================================================
// 类型转换
// ============================================================================

// opCast 类型转换 (失败抛出异常)
func opCast(vm *VM) {
	typeNameVal := vm.readConstant()
	typeName := typeNameVal.AsString()
	val := vm.pop()

	result, ok := castValue(val, typeName)
	if !ok {
		vm.runtimeError("cannot cast %s to %s", val.Type(), typeName)
		return
	}
	vm.push(result)
}

// opCastSafe 安全类型转换 (失败返回 null)
func opCastSafe(vm *VM) {
	typeNameVal := vm.readConstant()
	typeName := typeNameVal.AsString()
	val := vm.pop()

	result, ok := castValue(val, typeName)
	if !ok {
		vm.push(bytecode.NullValue)
		return
	}
	vm.push(result)
}

// castValue 执行类型转换
func castValue(val bytecode.Value, targetType string) (bytecode.Value, bool) {
	switch targetType {
	case "int", "i64":
		return castToInt(val)
	case "float", "f64":
		return castToFloat(val)
	case "string":
		return castToString(val)
	case "bool":
		return castToBool(val)
	default:
		// 对象类型转换：检查类型是否匹配
		if val.IsObject() {
			obj := val.AsObject()
			if obj != nil && obj.Class != nil {
				if obj.Class.Name == targetType {
					return val, true
				}
			}
		}
		return bytecode.NullValue, false
	}
}

// castToInt 转换为 int
func castToInt(val bytecode.Value) (bytecode.Value, bool) {
	switch {
	case val.IsInt():
		return val, true
	case val.IsFloat():
		return bytecode.NewInt(int64(val.AsFloat())), true
	case val.IsBool():
		if val.AsBool() {
			return bytecode.OneValue, true
		}
		return bytecode.ZeroValue, true
	case val.IsString():
		s := val.AsString()
		var n int64
		_, err := fmt.Sscanf(s, "%d", &n)
		if err != nil {
			return bytecode.NullValue, false
		}
		return bytecode.NewInt(n), true
	default:
		return bytecode.NullValue, false
	}
}

// castToFloat 转换为 float
func castToFloat(val bytecode.Value) (bytecode.Value, bool) {
	switch {
	case val.IsFloat():
		return val, true
	case val.IsInt():
		return bytecode.NewFloat(float64(val.AsInt())), true
	case val.IsBool():
		if val.AsBool() {
			return bytecode.NewFloat(1.0), true
		}
		return bytecode.NewFloat(0.0), true
	case val.IsString():
		s := val.AsString()
		var f float64
		_, err := fmt.Sscanf(s, "%f", &f)
		if err != nil {
			return bytecode.NullValue, false
		}
		return bytecode.NewFloat(f), true
	default:
		return bytecode.NullValue, false
	}
}

// castToString 转换为 string
func castToString(val bytecode.Value) (bytecode.Value, bool) {
	switch {
	case val.IsString():
		return val, true
	case val.IsInt():
		return bytecode.NewString(fmt.Sprintf("%d", val.AsInt())), true
	case val.IsFloat():
		return bytecode.NewString(fmt.Sprintf("%g", val.AsFloat())), true
	case val.IsBool():
		if val.AsBool() {
			return bytecode.NewString("true"), true
		}
		return bytecode.NewString("false"), true
	case val.IsNull():
		return bytecode.NewString("null"), true
	default:
		// 对象类型使用 String() 方法
		return bytecode.NewString(val.String()), true
	}
}

// castToBool 转换为 bool
func castToBool(val bytecode.Value) (bytecode.Value, bool) {
	return bytecode.NewBool(val.IsTruthy()), true
}

// ============================================================================
// 其他
// ============================================================================

// opPrint 打印值
func opPrint(vm *VM) {
	v := vm.pop()
	fmt.Println(v.String())
}

