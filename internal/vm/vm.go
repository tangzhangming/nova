package vm

import (
	"fmt"

	"github.com/tangzhangming/nova/internal/bytecode"
	"github.com/tangzhangming/nova/internal/i18n"
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
	CatchIP          int             // catch 块的 IP
	FinallyIP        int             // finally 块的 IP (-1 表示没有)
	AfterFinallyIP   int             // finally 块之后的 IP
	FrameCount       int             // 进入 try 时的帧数
	StackTop         int             // 进入 try 时的栈顶
	ExceptionVar     int             // 异常变量的 slot
	InFinally        bool            // 是否正在执行 finally 块
	PendingException bytecode.Value  // 挂起的异常（finally 结束后处理）
	HasPendingExc    bool            // 是否有挂起的异常
	PendingReturn    bytecode.Value  // 挂起的返回值
	HasPendingReturn bool            // 是否有挂起的返回
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

	// 垃圾回收
	gc *GC

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
		gc:      NewGC(),
	}
}

// GetGC 获取垃圾回收器
func (vm *VM) GetGC() *GC {
	return vm.gc
}

// SetGCEnabled 启用/禁用 GC
func (vm *VM) SetGCEnabled(enabled bool) {
	vm.gc.SetEnabled(enabled)
}

// SetGCDebug 设置 GC 调试模式
func (vm *VM) SetGCDebug(debug bool) {
	vm.gc.SetDebug(debug)
}

// SetGCThreshold 设置 GC 触发阈值
func (vm *VM) SetGCThreshold(threshold int) {
	vm.gc.SetThreshold(threshold)
}

// CollectGarbage 手动触发垃圾回收
func (vm *VM) CollectGarbage() int {
	roots := vm.collectRoots()
	return vm.gc.Collect(roots)
}

// collectRoots 收集 GC 根对象
func (vm *VM) collectRoots() []GCObject {
	var roots []GCObject

	// 1. 栈上的值
	for i := 0; i < vm.stackTop; i++ {
		if w := vm.gc.GetWrapper(vm.stack[i]); w != nil {
			roots = append(roots, w)
		}
	}

	// 2. 全局变量
	for _, v := range vm.globals {
		if w := vm.gc.GetWrapper(v); w != nil {
			roots = append(roots, w)
		}
	}

	// 3. 调用帧中的闭包
	for i := 0; i < vm.frameCount; i++ {
		closure := vm.frames[i].Closure
		if closure != nil {
			if w := vm.gc.GetWrapper(bytecode.NewClosure(closure)); w != nil {
				roots = append(roots, w)
			}
		}
	}

	return roots
}

// trackAllocation 追踪堆分配，必要时触发 GC
func (vm *VM) trackAllocation(v bytecode.Value) bytecode.Value {
	w := vm.gc.TrackValue(v)
	if w != nil && vm.gc.ShouldCollect() {
		roots := vm.collectRoots()
		vm.gc.Collect(roots)
	}
	return v
}

// maybeGC 检查并执行 GC（用于循环等热点路径）
func (vm *VM) maybeGC() {
	if vm.gc.ShouldCollect() {
		roots := vm.collectRoots()
		vm.gc.Collect(roots)
	}
}

// Run 执行字节码
func (vm *VM) Run(fn *bytecode.Function) InterpretResult {
	// 创建顶层闭包
	closure := &bytecode.Closure{Function: fn}
	
	// 压入闭包
	vm.push(vm.trackAllocation(bytecode.NewClosure(closure)))
	
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
	
	// GC 检查间隔（每执行 500 条指令检查一次）
	const gcCheckInterval = 500
	gcCheckCounter := 0

	for {
		// 安全检查：IP 越界
		if frame.IP >= len(chunk.Code) {
			return vm.runtimeError(i18n.T(i18n.ErrIPOutOfBounds))
		}

		// 安全检查：指令计数
		instructionCount++
		if instructionCount > maxInstructions {
			return vm.runtimeError(i18n.T(i18n.ErrExecutionLimit))
		}
		
		// 周期性 GC 检查
		gcCheckCounter++
		if gcCheckCounter >= gcCheckInterval {
			gcCheckCounter = 0
			vm.maybeGC()
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
				return vm.runtimeError(i18n.T(i18n.ErrUndefinedVar, name))
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
				return vm.runtimeError(i18n.T(i18n.ErrOperandMustBeNumber))
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
			// 字符串在 Go 中是不可变的，直接创建新字符串
			result := bytecode.NewString(a.AsString() + b.AsString())
			vm.push(result)

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
			vm.push(vm.trackAllocation(bytecode.NewClosure(closure)))

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
				return vm.runtimeError(i18n.T(i18n.ErrUndefinedClass, className))
			}
			// 验证类约束（抽象类、接口实现）
			if err := vm.validateClass(class); err != nil {
				return vm.runtimeError("%v", err)
			}
			obj := bytecode.NewObjectInstance(class)
			// 初始化属性默认值
			vm.initObjectProperties(obj, class)
			vm.push(vm.trackAllocation(bytecode.NewObject(obj)))

		case bytecode.OpGetField:
			nameIdx := chunk.ReadU16(frame.IP)
			frame.IP += 2
			name := chunk.Constants[nameIdx].AsString()
			
			objVal := vm.pop()
			if objVal.Type != bytecode.ValObject {
				return vm.runtimeError(i18n.T(i18n.ErrOnlyObjectsHaveFields))
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
				return vm.runtimeError(i18n.T(i18n.ErrOnlyObjectsHaveFields))
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
				return vm.runtimeError(i18n.T(i18n.ErrUndefinedEnumCase, className, name))
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
				return vm.runtimeError(i18n.T(i18n.ErrUndefinedStaticMethod, class.Name, methodName, argCount))
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
			vm.push(vm.trackAllocation(bytecode.NewArray(arr)))

		case bytecode.OpNewFixedArray:
			capacity := int(chunk.ReadU16(frame.IP))
			frame.IP += 2
			initLength := int(chunk.ReadU16(frame.IP))
			frame.IP += 2
			elements := make([]bytecode.Value, initLength)
			for i := initLength - 1; i >= 0; i-- {
				elements[i] = vm.pop()
			}
			vm.push(vm.trackAllocation(bytecode.NewFixedArrayWithElements(elements, capacity)))

		case bytecode.OpArrayGet:
			idx := vm.pop()
			arrVal := vm.pop()
			
			switch arrVal.Type {
			case bytecode.ValArray:
				arr := arrVal.AsArray()
				i := int(idx.AsInt())
				if i < 0 || i >= len(arr) {
					return vm.runtimeError(i18n.T(i18n.ErrArrayIndexSimple))
				}
				vm.push(arr[i])
			case bytecode.ValFixedArray:
				fa := arrVal.AsFixedArray()
				i := int(idx.AsInt())
				if i < 0 || i >= fa.Capacity {
					return vm.runtimeError(i18n.T(i18n.ErrArrayIndexOutOfBounds, i, fa.Capacity))
				}
				vm.push(fa.Elements[i])
			case bytecode.ValMap:
				// Map 索引支持
				m := arrVal.AsMap()
				if value, ok := m[idx]; ok {
					vm.push(value)
				} else {
					vm.push(bytecode.NullValue)
				}
			default:
				return vm.runtimeError(i18n.T(i18n.ErrSubscriptRequiresArray))
			}

		case bytecode.OpArraySet:
			value := vm.pop()
			idx := vm.pop()
			arrVal := vm.pop()
			
			switch arrVal.Type {
			case bytecode.ValArray:
				arr := arrVal.AsArray()
				i := int(idx.AsInt())
				if i < 0 || i >= len(arr) {
					return vm.runtimeError(i18n.T(i18n.ErrArrayIndexSimple))
				}
				arr[i] = value
			case bytecode.ValFixedArray:
				fa := arrVal.AsFixedArray()
				i := int(idx.AsInt())
				if i < 0 || i >= fa.Capacity {
					return vm.runtimeError(i18n.T(i18n.ErrArrayIndexOutOfBounds, i, fa.Capacity))
				}
				fa.Elements[i] = value
			case bytecode.ValMap:
				// Map 设置
				m := arrVal.AsMap()
				m[idx] = value
			default:
				return vm.runtimeError(i18n.T(i18n.ErrSubscriptRequiresArray))
			}
			vm.push(value)

		case bytecode.OpArrayLen:
			arrVal := vm.pop()
			switch arrVal.Type {
			case bytecode.ValArray:
				vm.push(bytecode.NewInt(int64(len(arrVal.AsArray()))))
			case bytecode.ValFixedArray:
				vm.push(bytecode.NewInt(int64(arrVal.AsFixedArray().Capacity)))
			default:
				return vm.runtimeError(i18n.T(i18n.ErrLengthRequiresArray))
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
			vm.push(vm.trackAllocation(bytecode.NewMap(m)))

		case bytecode.OpMapGet:
			key := vm.pop()
			mapVal := vm.pop()
			if mapVal.Type != bytecode.ValMap {
				return vm.runtimeError(i18n.T(i18n.ErrSubscriptRequiresMap))
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
				return vm.runtimeError(i18n.T(i18n.ErrSubscriptRequiresMap))
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
				return vm.runtimeError(i18n.T(i18n.ErrHasRequiresArrayOrMap))
			}

		case bytecode.OpMapLen:
			mapVal := vm.pop()
			if mapVal.Type != bytecode.ValMap {
				return vm.runtimeError(i18n.T(i18n.ErrLengthRequiresMap))
			}
			vm.push(bytecode.NewInt(int64(len(mapVal.AsMap()))))

		// 迭代器操作
		case bytecode.OpIterInit:
			v := vm.pop()
			if v.Type != bytecode.ValArray && v.Type != bytecode.ValFixedArray && v.Type != bytecode.ValMap {
				return vm.runtimeError(i18n.T(i18n.ErrForeachRequiresIterable))
			}
			iter := bytecode.NewIterator(v)
			vm.push(vm.trackAllocation(bytecode.NewIteratorValue(iter)))

		case bytecode.OpIterNext:
			iterVal := vm.peek(0) // 只读取，不弹出
			iter := iterVal.AsIterator()
			if iter == nil {
				return vm.runtimeError(i18n.T(i18n.ErrExpectedIterator))
			}
			hasNext := iter.Next()
			vm.push(bytecode.NewBool(hasNext))

		case bytecode.OpIterKey:
			iterVal := vm.peek(0) // 只读取，不弹出
			iter := iterVal.AsIterator()
			if iter == nil {
				return vm.runtimeError(i18n.T(i18n.ErrExpectedIterator))
			}
			vm.push(iter.Key())

		case bytecode.OpIterValue:
			iterVal := vm.peek(0) // 只读取，不弹出
			iter := iterVal.AsIterator()
			if iter == nil {
				return vm.runtimeError(i18n.T(i18n.ErrExpectedIterator))
			}
			vm.push(iter.CurrentValue())

		// 数组操作扩展
		case bytecode.OpArrayPush:
			value := vm.pop()
			arrVal := vm.pop()
			if arrVal.Type != bytecode.ValArray {
				return vm.runtimeError(i18n.T(i18n.ErrPushRequiresArray))
			}
			arr := arrVal.AsArray()
			arr = append(arr, value)
			vm.push(vm.trackAllocation(bytecode.NewArray(arr)))

		case bytecode.OpArrayHas:
			idx := vm.pop()
			arrVal := vm.pop()
			if arrVal.Type != bytecode.ValArray {
				return vm.runtimeError(i18n.T(i18n.ErrHasRequiresArray))
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
			// 捕获调用栈信息
			if exc := exception.AsException(); exc != nil && len(exc.Stack) == 0 {
				exc.Stack = vm.captureStackTrace()
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
			finallyOffset := chunk.ReadI16(frame.IP)
			frame.IP += 2
			
			// 计算 catch 块的绝对地址
			catchIP := offsetStart + int(catchOffset)
			
			// 计算 finally 块的绝对地址 (-1 表示没有 finally)
			finallyIP := -1
			if finallyOffset != -1 {
				finallyIP = offsetStart + int(finallyOffset)
			}
			
			vm.tryStack = append(vm.tryStack, TryContext{
				CatchIP:      catchIP,
				FinallyIP:    finallyIP,
				FrameCount:   vm.frameCount,
				StackTop:     vm.stackTop,
			})

		case bytecode.OpLeaveTry:
			if len(vm.tryStack) > 0 {
				vm.tryStack = vm.tryStack[:len(vm.tryStack)-1]
			}

		case bytecode.OpEnterCatch:
			// 异常值已经在栈上
			// 清除异常状态
			vm.hasException = false

		case bytecode.OpEnterFinally:
			// 进入 finally 块
			// 如果有挂起的异常或返回值，VM 会在 OpLeaveFinally 时处理
			// finally 块开始时不需要特殊处理

		case bytecode.OpLeaveFinally:
			// 离开 finally 块，检查是否有挂起的异常或返回值
			if len(vm.tryStack) > 0 {
				tryCtx := &vm.tryStack[len(vm.tryStack)-1]
				if tryCtx.InFinally {
					tryCtx.InFinally = false
					vm.tryStack = vm.tryStack[:len(vm.tryStack)-1]
					
					// 如果有挂起的异常，重新抛出
					if tryCtx.HasPendingExc {
						if !vm.handleException(tryCtx.PendingException) {
							return vm.runtimeError("uncaught exception: %s", tryCtx.PendingException.String())
						}
						frame = &vm.frames[vm.frameCount-1]
						chunk = frame.Closure.Function.Chunk
						continue
					}
					
					// 如果有挂起的返回值，执行返回
					if tryCtx.HasPendingReturn {
						result := tryCtx.PendingReturn
						vm.frameCount--
						if vm.frameCount == 0 {
							vm.pop()
							return InterpretOK
						}
						vm.stackTop = frame.BaseSlot
						vm.push(result)
						frame = &vm.frames[vm.frameCount-1]
						chunk = frame.Closure.Function.Chunk
						continue
					}
				}
			}

		case bytecode.OpRethrow:
			// 重新抛出当前异常
			if vm.hasException {
				if !vm.handleException(vm.exception) {
					return vm.runtimeError("uncaught exception: %s", vm.exception.String())
				}
				frame = &vm.frames[vm.frameCount-1]
				chunk = frame.Closure.Function.Chunk
			}

		// 类型检查
		case bytecode.OpCheckType:
			typeIdx := chunk.ReadU16(frame.IP)
			frame.IP += 2
			expectedType := chunk.Constants[typeIdx].AsString()
			value := vm.peek(0)
			
			if !vm.checkValueType(value, expectedType) {
				actualType := vm.getValueTypeName(value)
				return vm.runtimeError(i18n.T(i18n.ErrTypeError, expectedType, actualType))
			}
			
		case bytecode.OpCast:
			typeIdx := chunk.ReadU16(frame.IP)
			frame.IP += 2
			targetType := chunk.Constants[typeIdx].AsString()
			value := vm.pop()
			
			result, ok := vm.castValue(value, targetType)
			if !ok {
				actualType := vm.getValueTypeName(value)
				return vm.runtimeError(i18n.T(i18n.ErrCannotCast, actualType, targetType))
			}
			vm.push(result)

		// 调试
		case bytecode.OpDebugPrint:
			fmt.Println(vm.pop().String())

		case bytecode.OpHalt:
			return InterpretOK

		default:
			return vm.runtimeError(i18n.T(i18n.ErrUnknownOpcode, instruction))
		}
	}
}

// handleException 处理异常，返回是否成功处理
func (vm *VM) handleException(exception bytecode.Value) bool {
	vm.exception = exception
	vm.hasException = true
	
	// 为异常添加调用栈信息
	if exc := exception.AsException(); exc != nil && len(exc.Stack) == 0 {
		exc.Stack = vm.captureStackTrace()
	}
	
	// 查找最近的 try 块
	for len(vm.tryStack) > 0 {
		tryCtx := &vm.tryStack[len(vm.tryStack)-1]
		
		// 如果正在执行 finally 块中发生异常，记录但继续传播
		if tryCtx.InFinally {
			vm.tryStack = vm.tryStack[:len(vm.tryStack)-1]
			continue
		}
		
		// 展开调用栈到 try 块所在的帧
		for vm.frameCount > tryCtx.FrameCount {
			vm.frameCount--
		}
		
		if vm.frameCount > 0 {
			frame := &vm.frames[vm.frameCount-1]
			
			// 恢复栈状态
			vm.stackTop = tryCtx.StackTop
			
			// 如果有 finally 块，先执行 finally
			if tryCtx.FinallyIP >= 0 && tryCtx.CatchIP != tryCtx.FinallyIP {
				// 有 catch 块，先跳转到 catch
				frame.IP = tryCtx.CatchIP
				vm.push(exception)
				vm.hasException = false
				return true
			} else if tryCtx.FinallyIP >= 0 {
				// 只有 finally 块，没有 catch
				// 挂起异常，先执行 finally
				tryCtx.PendingException = exception
				tryCtx.HasPendingExc = true
				tryCtx.InFinally = true
				frame.IP = tryCtx.FinallyIP
				vm.hasException = false
				return true
			} else {
				// 只有 catch，跳转到 catch
				frame.IP = tryCtx.CatchIP
				vm.push(exception)
				vm.hasException = false
				vm.tryStack = vm.tryStack[:len(vm.tryStack)-1]
				return true
			}
		}
		
		vm.tryStack = vm.tryStack[:len(vm.tryStack)-1]
	}
	
	// 没有找到处理程序
	return false
}

// captureStackTrace 捕获当前调用栈信息
func (vm *VM) captureStackTrace() []string {
	var stack []string
	for i := vm.frameCount - 1; i >= 0; i-- {
		frame := &vm.frames[i]
		fn := frame.Closure.Function
		line := 0
		if frame.IP > 0 && frame.IP-1 < len(fn.Chunk.Lines) {
			line = fn.Chunk.Lines[frame.IP-1]
		}
		stack = append(stack, fmt.Sprintf("%s (line %d)", fn.Name, line))
	}
	return stack
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
				return vm.runtimeError(i18n.T(i18n.ErrDivisionByZero))
			}
			vm.push(bytecode.NewInt(ai / bi))
		case bytecode.OpMod:
			if bi == 0 {
				return vm.runtimeError(i18n.T(i18n.ErrDivisionByZero))
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
				return vm.runtimeError(i18n.T(i18n.ErrDivisionByZero))
			}
			vm.push(bytecode.NewFloat(af / bf))
		case bytecode.OpMod:
			return vm.runtimeError(i18n.T(i18n.ErrModuloNotForFloats))
		}
		return InterpretOK
	}

	return vm.runtimeError(i18n.T(i18n.ErrOperandsMustBeNumbers))
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

	return vm.runtimeError(i18n.T(i18n.ErrOperandsMustBeComparable))
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
			return vm.callBuiltin(fn, argCount)
		}
		closure := &bytecode.Closure{Function: fn}
		return vm.call(closure, argCount)
	default:
		return vm.runtimeError(i18n.T(i18n.ErrCanOnlyCallFunctions))
	}
}

// callBuiltin 调用内置函数，支持异常捕获
func (vm *VM) callBuiltin(fn *bytecode.Function, argCount int) InterpretResult {
	// 收集参数
	args := make([]bytecode.Value, argCount)
	for i := argCount - 1; i >= 0; i-- {
		args[i] = vm.pop()
	}
	vm.pop() // 弹出函数本身
	
	// 使用 defer/recover 捕获 Go 原生 panic
	var result bytecode.Value
	var panicErr interface{}
	
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicErr = r
			}
		}()
		result = fn.BuiltinFn(args)
	}()
	
	// 如果发生 panic，转换为异常
	if panicErr != nil {
		var errMsg string
		switch e := panicErr.(type) {
		case error:
			errMsg = e.Error()
		case string:
			errMsg = e
		default:
			errMsg = fmt.Sprintf("%v", panicErr)
		}
		exception := bytecode.NewException("NativeException", errMsg, 0)
		if !vm.handleException(exception) {
			return vm.runtimeError("uncaught native exception: %s", errMsg)
		}
		return InterpretOK
	}
	
	// 检查返回值是否是异常
	if result.Type == bytecode.ValException {
		if !vm.handleException(result) {
			return vm.runtimeError("uncaught exception: %s", result.String())
		}
		return InterpretOK
	}
	
	vm.push(result)
	return InterpretOK
}

// 调用闭包
func (vm *VM) call(closure *bytecode.Closure, argCount int) InterpretResult {
	fn := closure.Function
	
	// 检查参数数量
	if fn.IsVariadic {
		// 可变参数函数：至少需要 MinArity 个参数
		if argCount < fn.MinArity {
			return vm.runtimeError(i18n.T(i18n.ErrArgumentCountMin, fn.MinArity, argCount))
		}
	} else {
		// 普通函数：检查参数数量范围
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
		return vm.runtimeError(i18n.T(i18n.ErrOnlyObjectsHaveMethods))
	}
	
	obj := receiver.AsObject()
	// 使用参数数量查找重载方法（考虑默认参数）
	method := vm.findMethodWithDefaults(obj.Class, name, argCount)
	if method == nil {
		return vm.runtimeError(i18n.T(i18n.ErrUndefinedMethod, name, argCount))
	}

	// 检查方法访问权限
	if err := vm.checkMethodAccess(obj.Class, method); err != nil {
		return vm.runtimeError("%v", err)
	}

	// 创建方法的闭包，包含默认参数信息
	closure := &bytecode.Closure{
		Function: &bytecode.Function{
			Name:          method.Name,
			Arity:         method.Arity,
			MinArity:      method.MinArity,
			Chunk:         method.Chunk,
			LocalCount:    method.LocalCount,
			DefaultValues: method.DefaultValues,
		},
	}

	return vm.call(closure, argCount)
}

// findMethodWithDefaults 查找方法，考虑默认参数
func (vm *VM) findMethodWithDefaults(class *bytecode.Class, name string, argCount int) *bytecode.Method {
	for c := class; c != nil; c = c.Parent {
		if methods, ok := c.Methods[name]; ok {
			for _, m := range methods {
				// 检查参数数量是否在有效范围内
				if argCount >= m.MinArity && argCount <= m.Arity {
					return m
				}
			}
		}
	}
	return nil
}

// 运行时错误
func (vm *VM) runtimeError(format string, args ...interface{}) InterpretResult {
	vm.hadError = true
	vm.errorMessage = fmt.Sprintf(format, args...)

	// 打印调用栈
	fmt.Printf(i18n.T(i18n.ErrRuntimeError, vm.errorMessage) + "\n")
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

// getValueTypeName 获取值的类型名称
func (vm *VM) getValueTypeName(v bytecode.Value) string {
	switch v.Type {
	case bytecode.ValNull:
		return "null"
	case bytecode.ValBool:
		return "bool"
	case bytecode.ValInt:
		return "int"
	case bytecode.ValFloat:
		return "float"
	case bytecode.ValString:
		return "string"
	case bytecode.ValArray:
		return "array"
	case bytecode.ValFixedArray:
		return "array"
	case bytecode.ValMap:
		return "map"
	case bytecode.ValObject:
		obj := v.AsObject()
		if obj != nil && obj.Class != nil {
			return obj.Class.Name
		}
		return "object"
	case bytecode.ValClosure:
		return "function"
	case bytecode.ValIterator:
		return "iterator"
	default:
		return "unknown"
	}
}

// checkValueType 检查值是否匹配指定类型
func (vm *VM) checkValueType(v bytecode.Value, expectedType string) bool {
	actualType := vm.getValueTypeName(v)
	
	// 直接匹配
	if actualType == expectedType {
		return true
	}
	
	// null 可以匹配任何可空类型（以 ? 开头的类型名会被处理掉 ?）
	if actualType == "null" {
		return true // null 可以赋值给任何类型（在运行时）
	}
	
	// 数值类型兼容性
	switch expectedType {
	case "int", "i8", "i16", "i32", "i64":
		return actualType == "int"
	case "float", "f32", "f64":
		return actualType == "float" || actualType == "int"
	case "number":
		return actualType == "int" || actualType == "float"
	case "mixed", "any":
		return true
	}
	
	// 对象类型检查（包括继承关系）
	if v.Type == bytecode.ValObject {
		obj := v.AsObject()
		if obj != nil && obj.Class != nil {
			// 检查是否是该类型或其子类
			return vm.checkClassHierarchy(obj.Class, expectedType)
		}
	}
	
	return false
}


// checkClassHierarchy 检查类是否匹配指定类型名（包括继承关系和接口）
func (vm *VM) checkClassHierarchy(class *bytecode.Class, typeName string) bool {
	if class == nil {
		return false
	}
	
	// 检查当前类
	if class.Name == typeName {
		return true
	}
	
	// 检查父类链
	for c := class; c != nil; c = c.Parent {
		if c.Name == typeName {
			return true
		}
		// 检查接口
		for _, iface := range c.Implements {
			if iface == typeName {
				return true
			}
		}
	}
	
	return false
}

// castValue 将值转换为指定类型
func (vm *VM) castValue(v bytecode.Value, targetType string) (bytecode.Value, bool) {
	switch targetType {
	case "int", "i8", "i16", "i32", "i64":
		switch v.Type {
		case bytecode.ValInt:
			return v, true
		case bytecode.ValFloat:
			return bytecode.NewInt(int64(v.AsFloat())), true
		case bytecode.ValString:
			// 尝试解析字符串为整数
			var i int64
			_, err := fmt.Sscanf(v.AsString(), "%d", &i)
			if err == nil {
				return bytecode.NewInt(i), true
			}
			return bytecode.Value{}, false
		case bytecode.ValBool:
			if v.AsBool() {
				return bytecode.NewInt(1), true
			}
			return bytecode.NewInt(0), true
		case bytecode.ValNull:
			return bytecode.NewInt(0), true
		}
		
	case "float", "f32", "f64":
		switch v.Type {
		case bytecode.ValFloat:
			return v, true
		case bytecode.ValInt:
			return bytecode.NewFloat(float64(v.AsInt())), true
		case bytecode.ValString:
			var f float64
			_, err := fmt.Sscanf(v.AsString(), "%f", &f)
			if err == nil {
				return bytecode.NewFloat(f), true
			}
			return bytecode.Value{}, false
		case bytecode.ValNull:
			return bytecode.NewFloat(0.0), true
		}
		
	case "string":
		return bytecode.NewString(v.String()), true
		
	case "bool":
		return bytecode.NewBool(v.IsTruthy()), true
		
	case "array":
		if v.Type == bytecode.ValArray || v.Type == bytecode.ValFixedArray {
			return v, true
		}
		// 将单个值包装为数组
		return bytecode.NewArray([]bytecode.Value{v}), true
	}
	
	return bytecode.Value{}, false
}

