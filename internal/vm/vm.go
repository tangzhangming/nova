package vm

import (
	"fmt"

	"github.com/tangzhangming/nova/internal/bytecode"
)

const (
	StackMax  = 256  // 操作数栈最大深度
	FramesMax = 64   // 调用栈最大深度
)

// InterpretResult 执行结果
type InterpretResult int

const (
	InterpretOK InterpretResult = iota
	InterpretCompileError
	InterpretRuntimeError
)

// CallFrame 调用帧
type CallFrame struct {
	Closure  *bytecode.Closure // 当前执行的闭包
	IP       int               // 指令指针
	BaseSlot int               // 栈基址
}

// TryContext 异常处理上下文
type TryContext struct {
	CatchIP      int  // catch 块的 IP
	FinallyIP    int  // finally 块的 IP (-1 表示没有)
	FrameCount   int  // 进入 try 时的帧数
	StackTop     int  // 进入 try 时的栈顶
	ExceptionVar int  // 异常变量的 slot
}

// VM 虚拟机
type VM struct {
	frames     [FramesMax]CallFrame
	frameCount int

	stack    [StackMax]bytecode.Value
	stackTop int

	globals map[string]bytecode.Value
	classes map[string]*bytecode.Class
	enums   map[string]*bytecode.Enum

	// 异常处理
	tryStack    []TryContext
	exception   bytecode.Value
	hasException bool

	// 错误信息
	hadError     bool
	errorMessage string
}

// New 创建虚拟机
func New() *VM {
	return &VM{
		globals: make(map[string]bytecode.Value),
		classes: make(map[string]*bytecode.Class),
		enums:   make(map[string]*bytecode.Enum),
	}
}

// Run 执行字节码
func (vm *VM) Run(fn *bytecode.Function) InterpretResult {
	// 创建顶层闭包
	closure := &bytecode.Closure{Function: fn}
	
	// 压入闭包
	vm.push(bytecode.NewClosure(closure))
	
	// 创建调用帧
	vm.frames[0] = CallFrame{
		Closure:  closure,
		IP:       0,
		BaseSlot: 0,
	}
	vm.frameCount = 1

	return vm.execute()
}

// execute 执行循环
func (vm *VM) execute() InterpretResult {
	frame := &vm.frames[vm.frameCount-1]
	chunk := frame.Closure.Function.Chunk

	// 防止无限循环的安全计数器
	maxInstructions := 10000000 // 1000万条指令上限
	instructionCount := 0

	for {
		// 安全检查：IP 越界
		if frame.IP >= len(chunk.Code) {
			return vm.runtimeError("instruction pointer out of bounds")
		}

		// 安全检查：指令计数
		instructionCount++
		if instructionCount > maxInstructions {
			return vm.runtimeError("execution limit exceeded (infinite loop?)")
		}

		// 读取指令
		instruction := bytecode.OpCode(chunk.Code[frame.IP])
		frame.IP++

		switch instruction {
		case bytecode.OpPush:
			constant := chunk.ReadU16(frame.IP)
			frame.IP += 2
			vm.push(chunk.Constants[constant])

		case bytecode.OpPop:
			vm.pop()

		case bytecode.OpDup:
			vm.push(vm.peek(0))

		case bytecode.OpSwap:
			a := vm.pop()
			b := vm.pop()
			vm.push(a)
			vm.push(b)

		case bytecode.OpNull:
			vm.push(bytecode.NullValue)

		case bytecode.OpTrue:
			vm.push(bytecode.TrueValue)

		case bytecode.OpFalse:
			vm.push(bytecode.FalseValue)

		case bytecode.OpZero:
			vm.push(bytecode.ZeroValue)

		case bytecode.OpOne:
			vm.push(bytecode.OneValue)

		case bytecode.OpLoadLocal:
			slot := chunk.ReadU16(frame.IP)
			frame.IP += 2
			vm.push(vm.stack[frame.BaseSlot+int(slot)])

		case bytecode.OpStoreLocal:
			slot := chunk.ReadU16(frame.IP)
			frame.IP += 2
			vm.stack[frame.BaseSlot+int(slot)] = vm.peek(0)

		case bytecode.OpLoadGlobal:
			nameIdx := chunk.ReadU16(frame.IP)
			frame.IP += 2
			name := chunk.Constants[nameIdx].AsString()
			if value, ok := vm.globals[name]; ok {
				vm.push(value)
			} else {
				return vm.runtimeError("undefined variable '%s'", name)
			}

		case bytecode.OpStoreGlobal:
			nameIdx := chunk.ReadU16(frame.IP)
			frame.IP += 2
			name := chunk.Constants[nameIdx].AsString()
			vm.globals[name] = vm.peek(0)

		// 算术运算
		case bytecode.OpAdd:
			if result := vm.binaryOp(instruction); result != InterpretOK {
				return result
			}

		case bytecode.OpSub:
			if result := vm.binaryOp(instruction); result != InterpretOK {
				return result
			}

		case bytecode.OpMul:
			if result := vm.binaryOp(instruction); result != InterpretOK {
				return result
			}

		case bytecode.OpDiv:
			if result := vm.binaryOp(instruction); result != InterpretOK {
				return result
			}

		case bytecode.OpMod:
			if result := vm.binaryOp(instruction); result != InterpretOK {
				return result
			}

		case bytecode.OpNeg:
			v := vm.pop()
			switch v.Type {
			case bytecode.ValInt:
				vm.push(bytecode.NewInt(-v.AsInt()))
			case bytecode.ValFloat:
				vm.push(bytecode.NewFloat(-v.AsFloat()))
			default:
				return vm.runtimeError("operand must be a number")
			}

		// 比较运算
		case bytecode.OpEq:
			b := vm.pop()
			a := vm.pop()
			vm.push(bytecode.NewBool(a.Equals(b)))

		case bytecode.OpNe:
			b := vm.pop()
			a := vm.pop()
			vm.push(bytecode.NewBool(!a.Equals(b)))

		case bytecode.OpLt:
			if result := vm.compareOp(instruction); result != InterpretOK {
				return result
			}

		case bytecode.OpLe:
			if result := vm.compareOp(instruction); result != InterpretOK {
				return result
			}

		case bytecode.OpGt:
			if result := vm.compareOp(instruction); result != InterpretOK {
				return result
			}

		case bytecode.OpGe:
			if result := vm.compareOp(instruction); result != InterpretOK {
				return result
			}

		// 逻辑运算
		case bytecode.OpNot:
			vm.push(bytecode.NewBool(!vm.pop().IsTruthy()))

		// 位运算
		case bytecode.OpBitAnd:
			b := vm.pop()
			a := vm.pop()
			vm.push(bytecode.NewInt(a.AsInt() & b.AsInt()))

		case bytecode.OpBitOr:
			b := vm.pop()
			a := vm.pop()
			vm.push(bytecode.NewInt(a.AsInt() | b.AsInt()))

		case bytecode.OpBitXor:
			b := vm.pop()
			a := vm.pop()
			vm.push(bytecode.NewInt(a.AsInt() ^ b.AsInt()))

		case bytecode.OpBitNot:
			a := vm.pop()
			vm.push(bytecode.NewInt(^a.AsInt()))

		case bytecode.OpShl:
			b := vm.pop()
			a := vm.pop()
			vm.push(bytecode.NewInt(a.AsInt() << uint(b.AsInt())))

		case bytecode.OpShr:
			b := vm.pop()
			a := vm.pop()
			vm.push(bytecode.NewInt(a.AsInt() >> uint(b.AsInt())))

		// 字符串拼接
		case bytecode.OpConcat:
			b := vm.pop()
			a := vm.pop()
			vm.push(bytecode.NewString(a.AsString() + b.AsString()))

		// 跳转
		case bytecode.OpJump:
			offset := chunk.ReadI16(frame.IP)
			frame.IP += 2
			frame.IP += int(offset)

		case bytecode.OpJumpIfFalse:
			offset := chunk.ReadI16(frame.IP)
			frame.IP += 2
			if !vm.peek(0).IsTruthy() {
				frame.IP += int(offset)
			}

		case bytecode.OpJumpIfTrue:
			offset := chunk.ReadI16(frame.IP)
			frame.IP += 2
			if vm.peek(0).IsTruthy() {
				frame.IP += int(offset)
			}

		case bytecode.OpLoop:
			offset := chunk.ReadU16(frame.IP)
			frame.IP += 2
			frame.IP -= int(offset)

		// 函数调用
		case bytecode.OpCall:
			argCount := int(chunk.Code[frame.IP])
			frame.IP++
			if result := vm.callValue(vm.peek(argCount), argCount); result != InterpretOK {
				return result
			}
			frame = &vm.frames[vm.frameCount-1]
			chunk = frame.Closure.Function.Chunk

		case bytecode.OpReturn:
			result := vm.pop()
			vm.frameCount--
			if vm.frameCount == 0 {
				vm.pop() // 弹出脚本函数
				return InterpretOK
			}
			vm.stackTop = frame.BaseSlot
			vm.push(result)
			frame = &vm.frames[vm.frameCount-1]
			chunk = frame.Closure.Function.Chunk

		case bytecode.OpClosure:
			upvalueCount := int(chunk.ReadU16(frame.IP))
			frame.IP += 2
			
			// 栈上：[function, upvalue1, upvalue2, ...]
			// 创建闭包并捕获 upvalues
			upvalues := make([]*bytecode.Upvalue, upvalueCount)
			for i := upvalueCount - 1; i >= 0; i-- {
				val := vm.pop()
				upvalues[i] = &bytecode.Upvalue{Closed: val, IsClosed: true}
			}
			fnVal := vm.pop()
			fn := fnVal.Data.(*bytecode.Function)
			closure := &bytecode.Closure{
				Function: fn,
				Upvalues: upvalues,
			}
			vm.push(bytecode.NewClosure(closure))

		case bytecode.OpReturnNull:
			vm.frameCount--
			if vm.frameCount == 0 {
				vm.pop()
				return InterpretOK
			}
			vm.stackTop = frame.BaseSlot
			vm.push(bytecode.NullValue)
			frame = &vm.frames[vm.frameCount-1]
			chunk = frame.Closure.Function.Chunk

		// 对象操作
		case bytecode.OpNewObject:
			classIdx := chunk.ReadU16(frame.IP)
			frame.IP += 2
			className := chunk.Constants[classIdx].AsString()
			class, ok := vm.classes[className]
			if !ok {
				return vm.runtimeError("undefined class '%s'", className)
			}
			// 验证类约束（抽象类、接口实现）
			if err := vm.validateClass(class); err != nil {
				return vm.runtimeError("%v", err)
			}
			obj := bytecode.NewObjectInstance(class)
			// 初始化属性默认值
			vm.initObjectProperties(obj, class)
			vm.push(bytecode.NewObject(obj))

		case bytecode.OpGetField:
			nameIdx := chunk.ReadU16(frame.IP)
			frame.IP += 2
			name := chunk.Constants[nameIdx].AsString()
			
			objVal := vm.pop()
			if objVal.Type != bytecode.ValObject {
				return vm.runtimeError("only objects have fields")
			}
			obj := objVal.AsObject()
			
			// 检查访问权限
			if err := vm.checkPropertyAccess(obj.Class, name); err != nil {
				return vm.runtimeError("%v", err)
			}
			
			if value, ok := obj.GetField(name); ok {
				vm.push(value)
			} else {
				vm.push(bytecode.NullValue)
			}

		case bytecode.OpSetField:
			nameIdx := chunk.ReadU16(frame.IP)
			frame.IP += 2
			name := chunk.Constants[nameIdx].AsString()
			
			// 栈布局: [value, object] -> 先 pop object，再 pop value
			objVal := vm.pop()
			value := vm.pop()
			if objVal.Type != bytecode.ValObject {
				return vm.runtimeError("only objects have fields")
			}
			obj := objVal.AsObject()
			
			// 检查访问权限
			if err := vm.checkPropertyAccess(obj.Class, name); err != nil {
				return vm.runtimeError("%v", err)
			}
			
			obj.SetField(name, value)
			vm.push(value)

		case bytecode.OpCallMethod:
			nameIdx := chunk.ReadU16(frame.IP)
			frame.IP += 2
			argCount := int(chunk.Code[frame.IP])
			frame.IP++
			name := chunk.Constants[nameIdx].AsString()
			
			// 特殊处理构造函数 - 如果不存在则跳过
			receiver := vm.peek(argCount)
			if receiver.Type == bytecode.ValObject {
				obj := receiver.AsObject()
				method := obj.Class.GetMethod(name)
				if method == nil && name == "__construct" {
					// 没有构造函数，跳过调用，只保留对象在栈上
					continue
				}
			}
			
			if result := vm.invokeMethod(name, argCount); result != InterpretOK {
				return result
			}
			frame = &vm.frames[vm.frameCount-1]
			chunk = frame.Closure.Function.Chunk

		// 静态成员访问
		case bytecode.OpGetStatic:
			classIdx := chunk.ReadU16(frame.IP)
			frame.IP += 2
			nameIdx := chunk.ReadU16(frame.IP)
			frame.IP += 2
			className := chunk.Constants[classIdx].AsString()
			name := chunk.Constants[nameIdx].AsString()
			
			// 先检查是否是枚举
			if enum, ok := vm.enums[className]; ok {
				if val, ok := enum.Cases[name]; ok {
					vm.push(bytecode.NewEnumValue(className, name, val))
					continue
				}
				return vm.runtimeError("undefined enum case '%s::%s'", className, name)
			}
			
			class, err := vm.resolveClassName(className)
			if err != nil {
				return vm.runtimeError("%v", err)
			}
			
			// 先尝试常量
			if val, ok := vm.lookupConstant(class, name); ok {
				vm.push(val)
			} else if val, ok := vm.lookupStaticVar(class, name); ok {
				// 再尝试静态变量
				vm.push(val)
			} else {
				vm.push(bytecode.NullValue)
			}

		case bytecode.OpSetStatic:
			classIdx := chunk.ReadU16(frame.IP)
			frame.IP += 2
			nameIdx := chunk.ReadU16(frame.IP)
			frame.IP += 2
			className := chunk.Constants[classIdx].AsString()
			name := chunk.Constants[nameIdx].AsString()
			value := vm.pop()
			
			class, err := vm.resolveClassName(className)
			if err != nil {
				return vm.runtimeError("%v", err)
			}
			
			vm.setStaticVar(class, name, value)
			vm.push(value)

		case bytecode.OpCallStatic:
			classIdx := chunk.ReadU16(frame.IP)
			frame.IP += 2
			nameIdx := chunk.ReadU16(frame.IP)
			frame.IP += 2
			argCount := int(chunk.Code[frame.IP])
			frame.IP++
			className := chunk.Constants[classIdx].AsString()
			methodName := chunk.Constants[nameIdx].AsString()
			
			class, err := vm.resolveClassName(className)
			if err != nil {
				return vm.runtimeError("%v", err)
			}
			
			method := vm.lookupMethodByArity(class, methodName, argCount)
			if method == nil {
				return vm.runtimeError("undefined static method '%s::%s' with %d arguments", class.Name, methodName, argCount)
			}
			
			// 创建方法的闭包并调用
			closure := &bytecode.Closure{
				Function: &bytecode.Function{
					Name:       method.Name,
					Arity:      method.Arity,
					MinArity:   method.Arity, // 静态方法暂不支持默认参数
					Chunk:      method.Chunk,
					LocalCount: method.LocalCount,
				},
			}
			
			// 保存原始参数
			args := make([]bytecode.Value, argCount)
			for i := argCount - 1; i >= 0; i-- {
				args[i] = vm.pop()
			}
			
			// 对于 parent:: 和 self:: 调用非静态方法，需要传递当前的 $this
			// 对于真正的静态方法调用，使用 null
			if (className == "parent" || className == "self") && !method.IsStatic {
				// 传递当前的 $this
				thisValue := vm.stack[frame.BaseSlot]
				vm.push(thisValue)
			} else {
				// 静态方法，使用 null 作为占位符
				vm.push(bytecode.NullValue)
			}
			
			// 重新压入参数
			for i := 0; i < argCount; i++ {
				vm.push(args[i])
			}
			
			if result := vm.call(closure, argCount); result != InterpretOK {
				return result
			}
			frame = &vm.frames[vm.frameCount-1]
			chunk = frame.Closure.Function.Chunk

		// 数组操作
		case bytecode.OpNewArray:
			length := int(chunk.ReadU16(frame.IP))
			frame.IP += 2
			arr := make([]bytecode.Value, length)
			for i := length - 1; i >= 0; i-- {
				arr[i] = vm.pop()
			}
			vm.push(bytecode.NewArray(arr))

		case bytecode.OpNewFixedArray:
			capacity := int(chunk.ReadU16(frame.IP))
			frame.IP += 2
			initLength := int(chunk.ReadU16(frame.IP))
			frame.IP += 2
			elements := make([]bytecode.Value, initLength)
			for i := initLength - 1; i >= 0; i-- {
				elements[i] = vm.pop()
			}
			vm.push(bytecode.NewFixedArrayWithElements(elements, capacity))

		case bytecode.OpArrayGet:
			idx := vm.pop()
			arrVal := vm.pop()
			
			var arr []bytecode.Value
			var capacity int = -1
			
			switch arrVal.Type {
			case bytecode.ValArray:
				arr = arrVal.AsArray()
			case bytecode.ValFixedArray:
				fa := arrVal.AsFixedArray()
				arr = fa.Elements
				capacity = fa.Capacity
			default:
				return vm.runtimeError("subscript operator requires array")
			}
			
			i := int(idx.AsInt())
			if capacity > 0 {
				if i < 0 || i >= capacity {
					return vm.runtimeError("array index %d out of bounds (capacity %d)", i, capacity)
				}
			} else {
				if i < 0 || i >= len(arr) {
					return vm.runtimeError("array index out of bounds")
				}
			}
			vm.push(arr[i])

		case bytecode.OpArraySet:
			value := vm.pop()
			idx := vm.pop()
			arrVal := vm.pop()
			
			var arr []bytecode.Value
			var capacity int = -1
			
			switch arrVal.Type {
			case bytecode.ValArray:
				arr = arrVal.AsArray()
			case bytecode.ValFixedArray:
				fa := arrVal.AsFixedArray()
				arr = fa.Elements
				capacity = fa.Capacity
			default:
				return vm.runtimeError("subscript operator requires array")
			}
			
			i := int(idx.AsInt())
			if capacity > 0 {
				if i < 0 || i >= capacity {
					return vm.runtimeError("array index %d out of bounds (capacity %d)", i, capacity)
				}
			} else {
				if i < 0 || i >= len(arr) {
					return vm.runtimeError("array index out of bounds")
				}
			}
			arr[i] = value
			vm.push(value)

		case bytecode.OpArrayLen:
			arrVal := vm.pop()
			switch arrVal.Type {
			case bytecode.ValArray:
				vm.push(bytecode.NewInt(int64(len(arrVal.AsArray()))))
			case bytecode.ValFixedArray:
				vm.push(bytecode.NewInt(int64(arrVal.AsFixedArray().Capacity)))
			default:
				return vm.runtimeError("length requires array")
			}

		// Map 操作
		case bytecode.OpNewMap:
			size := int(chunk.ReadU16(frame.IP))
			frame.IP += 2
			m := make(map[bytecode.Value]bytecode.Value)
			for i := 0; i < size; i++ {
				value := vm.pop()
				key := vm.pop()
				m[key] = value
			}
			vm.push(bytecode.NewMap(m))

		case bytecode.OpMapGet:
			key := vm.pop()
			mapVal := vm.pop()
			if mapVal.Type != bytecode.ValMap {
				return vm.runtimeError("subscript operator requires map")
			}
			m := mapVal.AsMap()
			if value, ok := m[key]; ok {
				vm.push(value)
			} else {
				vm.push(bytecode.NullValue)
			}

		case bytecode.OpMapSet:
			value := vm.pop()
			key := vm.pop()
			mapVal := vm.pop()
			if mapVal.Type != bytecode.ValMap {
				return vm.runtimeError("subscript operator requires map")
			}
			m := mapVal.AsMap()
			m[key] = value
			vm.push(value)

		case bytecode.OpMapHas:
			key := vm.pop()
			container := vm.pop()
			switch container.Type {
			case bytecode.ValMap:
				m := container.AsMap()
				_, ok := m[key]
				vm.push(bytecode.NewBool(ok))
			case bytecode.ValArray:
				arr := container.AsArray()
				idx := int(key.AsInt())
				vm.push(bytecode.NewBool(idx >= 0 && idx < len(arr)))
			default:
				return vm.runtimeError("has() requires array or map")
			}

		case bytecode.OpMapLen:
			mapVal := vm.pop()
			if mapVal.Type != bytecode.ValMap {
				return vm.runtimeError("length requires map")
			}
			vm.push(bytecode.NewInt(int64(len(mapVal.AsMap()))))

		// 迭代器操作
		case bytecode.OpIterInit:
			v := vm.pop()
			if v.Type != bytecode.ValArray && v.Type != bytecode.ValFixedArray && v.Type != bytecode.ValMap {
				return vm.runtimeError("foreach requires array or map")
			}
			iter := bytecode.NewIterator(v)
			vm.push(bytecode.NewIteratorValue(iter))

		case bytecode.OpIterNext:
			iterVal := vm.peek(0) // 只读取，不弹出
			iter := iterVal.AsIterator()
			if iter == nil {
				return vm.runtimeError("expected iterator")
			}
			hasNext := iter.Next()
			vm.push(bytecode.NewBool(hasNext))

		case bytecode.OpIterKey:
			iterVal := vm.peek(0) // 只读取，不弹出
			iter := iterVal.AsIterator()
			if iter == nil {
				return vm.runtimeError("expected iterator")
			}
			vm.push(iter.Key())

		case bytecode.OpIterValue:
			iterVal := vm.peek(0) // 只读取，不弹出
			iter := iterVal.AsIterator()
			if iter == nil {
				return vm.runtimeError("expected iterator")
			}
			vm.push(iter.CurrentValue())

		// 数组操作扩展
		case bytecode.OpArrayPush:
			value := vm.pop()
			arrVal := vm.pop()
			if arrVal.Type != bytecode.ValArray {
				return vm.runtimeError("push requires array")
			}
			arr := arrVal.AsArray()
			arr = append(arr, value)
			vm.push(bytecode.NewArray(arr))

		case bytecode.OpArrayHas:
			idx := vm.pop()
			arrVal := vm.pop()
			if arrVal.Type != bytecode.ValArray {
				return vm.runtimeError("has requires array")
			}
			arr := arrVal.AsArray()
			i := int(idx.AsInt())
			vm.push(bytecode.NewBool(i >= 0 && i < len(arr)))

		case bytecode.OpUnset:
			objVal := vm.pop()
			if objVal.Type == bytecode.ValObject {
				obj := objVal.AsObject()
				// 调用析构函数 __destruct
				if method := obj.Class.GetMethod("__destruct"); method != nil {
					// 压入对象作为 receiver
					vm.push(objVal)
					if result := vm.invokeDestructor(obj, method); result != InterpretOK {
						return result
					}
					// 恢复帧引用
					frame = &vm.frames[vm.frameCount-1]
					chunk = frame.Closure.Function.Chunk
				}
			}
			vm.push(bytecode.NullValue)

		// 异常处理
		case bytecode.OpThrow:
			exception := vm.pop()
			// 如果抛出的是字符串，自动转换为 Exception 对象
			if exception.Type == bytecode.ValString {
				exception = bytecode.NewException("Exception", exception.AsString(), 0)
			}
			if !vm.handleException(exception) {
				return vm.runtimeError("uncaught exception: %s", exception.String())
			}
			// 更新 frame 和 chunk 引用
			frame = &vm.frames[vm.frameCount-1]
			chunk = frame.Closure.Function.Chunk

		case bytecode.OpEnterTry:
			// 记录偏移量开始的位置
			offsetStart := frame.IP
			catchOffset := chunk.ReadI16(frame.IP)
			frame.IP += 2
			_ = chunk.ReadI16(frame.IP) // finally 偏移量（暂不使用）
			frame.IP += 2
			
			// 计算 catch 块的绝对地址
			catchIP := offsetStart + int(catchOffset)
			
			vm.tryStack = append(vm.tryStack, TryContext{
				CatchIP:    catchIP,
				FinallyIP:  -1, // 暂时不支持 finally
				FrameCount: vm.frameCount,
				StackTop:   vm.stackTop,
			})

		case bytecode.OpLeaveTry:
			if len(vm.tryStack) > 0 {
				vm.tryStack = vm.tryStack[:len(vm.tryStack)-1]
			}

		case bytecode.OpEnterCatch:
			// 异常值已经在栈上
			// 清除异常状态
			vm.hasException = false

		// 调试
		case bytecode.OpDebugPrint:
			fmt.Println(vm.pop().String())

		case bytecode.OpHalt:
			return InterpretOK

		default:
			return vm.runtimeError("unknown opcode: %d", instruction)
		}
	}
}

// handleException 处理异常，返回是否成功处理
func (vm *VM) handleException(exception bytecode.Value) bool {
	vm.exception = exception
	vm.hasException = true
	
	// 查找最近的 try 块
	for len(vm.tryStack) > 0 {
		tryCtx := vm.tryStack[len(vm.tryStack)-1]
		vm.tryStack = vm.tryStack[:len(vm.tryStack)-1]
		
		// 展开调用栈到 try 块所在的帧
		for vm.frameCount > tryCtx.FrameCount {
			vm.frameCount--
		}
		
		if vm.frameCount > 0 {
			frame := &vm.frames[vm.frameCount-1]
			
			// 恢复栈状态：保持基础栈帧，只重置到 try 块开始时的状态
			// 但异常值需要作为一个新的局部变量
			vm.stackTop = tryCtx.StackTop
			
			frame.IP = tryCtx.CatchIP
			vm.push(exception) // 将异常值压入栈，它将成为 catch 块的第一个局部变量
			vm.hasException = false
			return true
		}
	}
	
	// 没有找到处理程序
	return false
}

// 栈操作
func (vm *VM) push(value bytecode.Value) {
	vm.stack[vm.stackTop] = value
	vm.stackTop++
}

func (vm *VM) pop() bytecode.Value {
	vm.stackTop--
	return vm.stack[vm.stackTop]
}

func (vm *VM) peek(distance int) bytecode.Value {
	return vm.stack[vm.stackTop-1-distance]
}

// 二元运算
func (vm *VM) binaryOp(op bytecode.OpCode) InterpretResult {
	b := vm.pop()
	a := vm.pop()

	// 字符串拼接
	if op == bytecode.OpAdd && (a.Type == bytecode.ValString || b.Type == bytecode.ValString) {
		vm.push(bytecode.NewString(a.AsString() + b.AsString()))
		return InterpretOK
	}

	// 数值运算
	if a.Type == bytecode.ValInt && b.Type == bytecode.ValInt {
		ai, bi := a.AsInt(), b.AsInt()
		switch op {
		case bytecode.OpAdd:
			vm.push(bytecode.NewInt(ai + bi))
		case bytecode.OpSub:
			vm.push(bytecode.NewInt(ai - bi))
		case bytecode.OpMul:
			vm.push(bytecode.NewInt(ai * bi))
		case bytecode.OpDiv:
			if bi == 0 {
				return vm.runtimeError("division by zero")
			}
			vm.push(bytecode.NewInt(ai / bi))
		case bytecode.OpMod:
			if bi == 0 {
				return vm.runtimeError("division by zero")
			}
			vm.push(bytecode.NewInt(ai % bi))
		}
		return InterpretOK
	}

	// 浮点运算
	if (a.Type == bytecode.ValInt || a.Type == bytecode.ValFloat) &&
		(b.Type == bytecode.ValInt || b.Type == bytecode.ValFloat) {
		af, bf := a.AsFloat(), b.AsFloat()
		switch op {
		case bytecode.OpAdd:
			vm.push(bytecode.NewFloat(af + bf))
		case bytecode.OpSub:
			vm.push(bytecode.NewFloat(af - bf))
		case bytecode.OpMul:
			vm.push(bytecode.NewFloat(af * bf))
		case bytecode.OpDiv:
			if bf == 0 {
				return vm.runtimeError("division by zero")
			}
			vm.push(bytecode.NewFloat(af / bf))
		case bytecode.OpMod:
			return vm.runtimeError("modulo not supported for floats")
		}
		return InterpretOK
	}

	return vm.runtimeError("operands must be numbers")
}

// 比较运算
func (vm *VM) compareOp(op bytecode.OpCode) InterpretResult {
	b := vm.pop()
	a := vm.pop()

	// 数值比较
	if (a.Type == bytecode.ValInt || a.Type == bytecode.ValFloat) &&
		(b.Type == bytecode.ValInt || b.Type == bytecode.ValFloat) {
		af, bf := a.AsFloat(), b.AsFloat()
		var result bool
		switch op {
		case bytecode.OpLt:
			result = af < bf
		case bytecode.OpLe:
			result = af <= bf
		case bytecode.OpGt:
			result = af > bf
		case bytecode.OpGe:
			result = af >= bf
		}
		vm.push(bytecode.NewBool(result))
		return InterpretOK
	}

	// 字符串比较
	if a.Type == bytecode.ValString && b.Type == bytecode.ValString {
		as, bs := a.AsString(), b.AsString()
		var result bool
		switch op {
		case bytecode.OpLt:
			result = as < bs
		case bytecode.OpLe:
			result = as <= bs
		case bytecode.OpGt:
			result = as > bs
		case bytecode.OpGe:
			result = as >= bs
		}
		vm.push(bytecode.NewBool(result))
		return InterpretOK
	}

	return vm.runtimeError("operands must be comparable")
}

// 调用值
func (vm *VM) callValue(callee bytecode.Value, argCount int) InterpretResult {
	switch callee.Type {
	case bytecode.ValClosure:
		return vm.call(callee.Data.(*bytecode.Closure), argCount)
	case bytecode.ValFunc:
		fn := callee.Data.(*bytecode.Function)
		// 特殊处理内置函数
		if fn.IsBuiltin && fn.BuiltinFn != nil {
			// 收集参数
			args := make([]bytecode.Value, argCount)
			for i := argCount - 1; i >= 0; i-- {
				args[i] = vm.pop()
			}
			vm.pop() // 弹出函数本身
			// 调用内置函数
			result := fn.BuiltinFn(args)
			vm.push(result)
			return InterpretOK
		}
		closure := &bytecode.Closure{Function: fn}
		return vm.call(closure, argCount)
	default:
		return vm.runtimeError("can only call functions")
	}
}

// 调用闭包
func (vm *VM) call(closure *bytecode.Closure, argCount int) InterpretResult {
	fn := closure.Function
	
	// 检查参数数量
	if fn.IsVariadic {
		// 可变参数函数：至少需要 MinArity 个参数
		if argCount < fn.MinArity {
			return vm.runtimeError("expected at least %d arguments but got %d",
				fn.MinArity, argCount)
		}
	} else {
		// 普通函数：检查参数数量范围
		if argCount < fn.MinArity {
			return vm.runtimeError("expected at least %d arguments but got %d",
				fn.MinArity, argCount)
		}
		if argCount > fn.Arity {
			return vm.runtimeError("expected at most %d arguments but got %d",
				fn.Arity, argCount)
		}
	}

	if vm.frameCount == FramesMax {
		return vm.runtimeError("stack overflow")
	}

	// 处理默认参数：填充缺失的参数
	if !fn.IsVariadic && argCount < fn.Arity {
		defaultStart := fn.MinArity
		for i := argCount; i < fn.Arity; i++ {
			defaultIdx := i - defaultStart
			if defaultIdx >= 0 && defaultIdx < len(fn.DefaultValues) {
				vm.push(fn.DefaultValues[defaultIdx])
			} else {
				vm.push(bytecode.NullValue)
			}
		}
		argCount = fn.Arity
	}

	// 处理可变参数：将多余参数打包成数组
	if fn.IsVariadic {
		variadicCount := argCount - fn.MinArity
		if variadicCount > 0 {
			// 收集可变参数到数组
			varArgs := make([]bytecode.Value, variadicCount)
			for i := variadicCount - 1; i >= 0; i-- {
				varArgs[i] = vm.pop()
			}
			argCount = fn.MinArity
			vm.push(bytecode.NewArray(varArgs))
			argCount++ // 可变参数数组占一个 slot
		} else {
			// 没有可变参数，推入空数组
			vm.push(bytecode.NewArray([]bytecode.Value{}))
			argCount++
		}
	}

	frame := &vm.frames[vm.frameCount]
	vm.frameCount++
	frame.Closure = closure
	frame.IP = 0
	frame.BaseSlot = vm.stackTop - argCount - 1
	
	// 如果有 upvalues，将它们作为额外的局部变量
	// 布局：[caller, arg0, arg1, ..., upval0, upval1, ...]
	for _, upval := range closure.Upvalues {
		if upval.IsClosed {
			vm.push(upval.Closed)
		} else {
			vm.push(*upval.Location)
		}
	}

	return InterpretOK
}

// 调用方法
func (vm *VM) invokeMethod(name string, argCount int) InterpretResult {
	receiver := vm.peek(argCount)
	if receiver.Type != bytecode.ValObject {
		return vm.runtimeError("only objects have methods")
	}
	
	obj := receiver.AsObject()
	// 使用参数数量查找重载方法
	method := obj.Class.GetMethodByArity(name, argCount)
	if method == nil {
		return vm.runtimeError("undefined method '%s' with %d arguments", name, argCount)
	}

	// 检查方法访问权限
	if err := vm.checkMethodAccess(obj.Class, method); err != nil {
		return vm.runtimeError("%v", err)
	}

	// 创建方法的闭包
	closure := &bytecode.Closure{
		Function: &bytecode.Function{
			Name:       method.Name,
			Arity:      method.Arity,
			Chunk:      method.Chunk,
			LocalCount: method.LocalCount,
		},
	}

	return vm.call(closure, argCount)
}

// 运行时错误
func (vm *VM) runtimeError(format string, args ...interface{}) InterpretResult {
	vm.hadError = true
	vm.errorMessage = fmt.Sprintf(format, args...)

	// 打印调用栈
	fmt.Printf("Runtime error: %s\n", vm.errorMessage)
	for i := vm.frameCount - 1; i >= 0; i-- {
		frame := &vm.frames[i]
		fn := frame.Closure.Function
		line := fn.Chunk.Lines[frame.IP-1]
		fmt.Printf("  [line %d] in %s\n", line, fn.Name)
	}

	return InterpretRuntimeError
}

// DefineGlobal 定义全局变量
func (vm *VM) DefineGlobal(name string, value bytecode.Value) {
	vm.globals[name] = value
}

// DefineClass 定义类
func (vm *VM) DefineClass(class *bytecode.Class) {
	vm.classes[class.Name] = class
}

// GetClass 获取类定义
func (vm *VM) GetClass(name string) *bytecode.Class {
	return vm.classes[name]
}

// DefineEnum 注册枚举
func (vm *VM) DefineEnum(enum *bytecode.Enum) {
	vm.enums[enum.Name] = enum
}

// GetError 获取错误信息
func (vm *VM) GetError() string {
	return vm.errorMessage
}

