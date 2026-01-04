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

// VM 虚拟机
type VM struct {
	frames     [FramesMax]CallFrame
	frameCount int

	stack    [StackMax]bytecode.Value
	stackTop int

	globals map[string]bytecode.Value
	classes map[string]*bytecode.Class

	// 错误信息
	hadError     bool
	errorMessage string
}

// New 创建虚拟机
func New() *VM {
	return &VM{
		globals: make(map[string]bytecode.Value),
		classes: make(map[string]*bytecode.Class),
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

		// 数组操作
		case bytecode.OpNewArray:
			length := int(chunk.ReadU16(frame.IP))
			frame.IP += 2
			arr := make([]bytecode.Value, length)
			for i := length - 1; i >= 0; i-- {
				arr[i] = vm.pop()
			}
			vm.push(bytecode.NewArray(arr))

		case bytecode.OpArrayGet:
			idx := vm.pop()
			arrVal := vm.pop()
			if arrVal.Type != bytecode.ValArray {
				return vm.runtimeError("subscript operator requires array")
			}
			arr := arrVal.AsArray()
			i := int(idx.AsInt())
			if i < 0 || i >= len(arr) {
				return vm.runtimeError("array index out of bounds")
			}
			vm.push(arr[i])

		case bytecode.OpArraySet:
			value := vm.pop()
			idx := vm.pop()
			arrVal := vm.pop()
			if arrVal.Type != bytecode.ValArray {
				return vm.runtimeError("subscript operator requires array")
			}
			arr := arrVal.AsArray()
			i := int(idx.AsInt())
			if i < 0 || i >= len(arr) {
				return vm.runtimeError("array index out of bounds")
			}
			arr[i] = value
			vm.push(value)

		case bytecode.OpArrayLen:
			arrVal := vm.pop()
			if arrVal.Type != bytecode.ValArray {
				return vm.runtimeError("length requires array")
			}
			vm.push(bytecode.NewInt(int64(len(arrVal.AsArray()))))

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
			mapVal := vm.pop()
			if mapVal.Type != bytecode.ValMap {
				return vm.runtimeError("has() requires map")
			}
			m := mapVal.AsMap()
			_, ok := m[key]
			vm.push(bytecode.NewBool(ok))

		case bytecode.OpMapLen:
			mapVal := vm.pop()
			if mapVal.Type != bytecode.ValMap {
				return vm.runtimeError("length requires map")
			}
			vm.push(bytecode.NewInt(int64(len(mapVal.AsMap()))))

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
		closure := &bytecode.Closure{Function: fn}
		return vm.call(closure, argCount)
	default:
		return vm.runtimeError("can only call functions")
	}
}

// 调用闭包
func (vm *VM) call(closure *bytecode.Closure, argCount int) InterpretResult {
	if argCount != closure.Function.Arity {
		return vm.runtimeError("expected %d arguments but got %d",
			closure.Function.Arity, argCount)
	}

	if vm.frameCount == FramesMax {
		return vm.runtimeError("stack overflow")
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
	method := obj.Class.GetMethod(name)
	if method == nil {
		return vm.runtimeError("undefined method '%s'", name)
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

// GetError 获取错误信息
func (vm *VM) GetError() string {
	return vm.errorMessage
}

