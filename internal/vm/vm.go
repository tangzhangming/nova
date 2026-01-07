package vm

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/tangzhangming/nova/internal/bytecode"
	"github.com/tangzhangming/nova/internal/errors"
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
	InterpretExceptionHandled // 异常已处理，需要刷新 frame/chunk
)

// CallFrame 调用帧
type CallFrame struct {
	Closure  *bytecode.Closure // 当前执行的闭包
	IP       int               // 指令指针
	BaseSlot int               // 栈基址
}

// CatchHandlerInfo catch 处理器信息
type CatchHandlerInfo struct {
	TypeName    string // 异常类型名
	CatchOffset int    // catch 块相对于 OpEnterTry 的偏移量
}

// TryContext 异常处理上下文
type TryContext struct {
	EnterTryIP       int                 // OpEnterTry 指令的位置
	CatchHandlers    []CatchHandlerInfo  // catch 处理器列表（按顺序）
	FinallyIP        int                 // finally 块的 IP (-1 表示没有)
	FrameCount       int                 // 进入 try 时的帧数
	StackTop         int                 // 进入 try 时的栈顶
	InCatch          bool                // 是否正在执行 catch 块
	InFinally        bool                // 是否正在执行 finally 块
	PendingException bytecode.Value      // 挂起的异常（finally 结束后处理）
	HasPendingExc    bool                // 是否有挂起的异常
	PendingReturn    bytecode.Value      // 挂起的返回值
	HasPendingReturn bool                // 是否有挂起的返回
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

	// 异常处理 - 优化：使用 tryDepth 快速判断是否在 try 块中
	tryStack     []TryContext
	tryDepth     int            // try 块嵌套深度，用于快速路径判断
	exception    bytecode.Value
	hasException bool

	// 垃圾回收
	gc *GC

	// 内联缓存（B1）
	icManager *ICManager

	// 热点检测（B2）
	hotspotDetector *HotspotDetector

	// 错误信息
	hadError     bool
	errorMessage string
}

// New 创建虚拟机
func New() *VM {
	return &VM{
		globals:         make(map[string]bytecode.Value),
		classes:         make(map[string]*bytecode.Class),
		enums:           make(map[string]*bytecode.Enum),
		gc:              NewGC(),
		icManager:       NewICManager(),
		hotspotDetector: NewHotspotDetector(),
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

// GetICManager 获取内联缓存管理器
func (vm *VM) GetICManager() *ICManager {
	return vm.icManager
}

// SetICEnabled 启用/禁用内联缓存
func (vm *VM) SetICEnabled(enabled bool) {
	vm.icManager.SetEnabled(enabled)
}

// GetHotspotDetector 获取热点检测器
func (vm *VM) GetHotspotDetector() *HotspotDetector {
	return vm.hotspotDetector
}

// SetHotspotEnabled 启用/禁用热点检测
func (vm *VM) SetHotspotEnabled(enabled bool) {
	vm.hotspotDetector.SetEnabled(enabled)
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
	
	// GC 检查间隔（每执行 100 条指令检查一次，降低间隔以更快响应内存暴涨）
	const gcCheckInterval = 100
	gcCheckCounter := 0
	
	// 内存分配速率保护：记录最近的内存分配次数
	const memoryCheckInterval = 50 // 每 50 条指令检查一次内存分配速率
	memoryCheckCounter := 0
	lastGCHeapSize := vm.gc.HeapSize()

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
		
		// 内存分配速率保护：如果内存快速增长，强制触发 GC
		memoryCheckCounter++
		if memoryCheckCounter >= memoryCheckInterval {
			memoryCheckCounter = 0
			currentHeapSize := vm.gc.HeapSize()
			// 如果堆大小在短时间内增长超过 100 个对象，强制触发 GC
			if currentHeapSize > lastGCHeapSize+100 {
				roots := vm.collectRoots()
				vm.gc.Collect(roots)
				lastGCHeapSize = vm.gc.HeapSize()
			} else {
				lastGCHeapSize = currentHeapSize
			}
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
				if result == InterpretExceptionHandled {
					frame = &vm.frames[vm.frameCount-1]
					chunk = frame.Closure.Function.Chunk
					continue
				}
				return result
			}

		case bytecode.OpSub:
			if result := vm.binaryOp(instruction); result != InterpretOK {
				if result == InterpretExceptionHandled {
					frame = &vm.frames[vm.frameCount-1]
					chunk = frame.Closure.Function.Chunk
					continue
				}
				return result
			}

		case bytecode.OpMul:
			if result := vm.binaryOp(instruction); result != InterpretOK {
				if result == InterpretExceptionHandled {
					frame = &vm.frames[vm.frameCount-1]
					chunk = frame.Closure.Function.Chunk
					continue
				}
				return result
			}

		case bytecode.OpDiv:
			if result := vm.binaryOp(instruction); result != InterpretOK {
				if result == InterpretExceptionHandled {
					frame = &vm.frames[vm.frameCount-1]
					chunk = frame.Closure.Function.Chunk
					continue
				}
				return result
			}

		case bytecode.OpMod:
			if result := vm.binaryOp(instruction); result != InterpretOK {
				if result == InterpretExceptionHandled {
					frame = &vm.frames[vm.frameCount-1]
					chunk = frame.Closure.Function.Chunk
					continue
				}
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

		// 字符串构建器操作（用于高效多字符串拼接）
		case bytecode.OpStringBuilderNew:
			sb := bytecode.NewStringBuilder()
			vm.push(bytecode.NewStringBuilderValue(sb))

		case bytecode.OpStringBuilderAdd:
			value := vm.pop()
			sbVal := vm.pop()
			sb := sbVal.AsStringBuilder()
			if sb != nil {
				sb.AppendValue(value)
				vm.push(sbVal) // 返回构建器自身，支持链式调用
			} else {
				return vm.runtimeError("expected StringBuilder")
			}

		case bytecode.OpStringBuilderBuild:
			sbVal := vm.pop()
			sb := sbVal.AsStringBuilder()
			if sb != nil {
				result := sb.Build()
				vm.push(bytecode.NewString(result))
			} else {
				return vm.runtimeError("expected StringBuilder")
			}

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
			loopHeaderIP := frame.IP - 1 // 回边位置
			offset := chunk.ReadU16(frame.IP)
			frame.IP += 2
			targetIP := frame.IP - int(offset) // 循环头位置
			frame.IP = targetIP
			
			// 热点检测：记录循环迭代
			vm.hotspotDetector.RecordLoopIteration(frame.Closure.Function, targetIP, loopHeaderIP)

		// 函数调用
		case bytecode.OpCall:
			argCount := int(chunk.Code[frame.IP])
			frame.IP++
			if result := vm.callValue(vm.peek(argCount), argCount); result != InterpretOK {
				return result
			}
			frame = &vm.frames[vm.frameCount-1]
			chunk = frame.Closure.Function.Chunk

		case bytecode.OpTailCall:
			argCount := int(chunk.Code[frame.IP])
			frame.IP++
			if result := vm.tailCall(vm.peek(argCount), argCount); result != InterpretOK {
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
				// 程序结束，清理栈上的闭包（如果有）
				if vm.stackTop > 0 {
					vm.pop()
				}
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
			
			// 如果有泛型类型参数定义，检查是否有类型参数传入
			// 注意：由于类型擦除，运行时通常没有泛型类型参数信息
			// 这里主要是为未来的扩展做准备，当前实现基本验证框架
			var typeArgs []string = nil
			if len(class.TypeParams) > 0 {
				// 如果有类型参数定义但运行时没有传入，这是正常的（类型擦除）
				// 未来如果需要运行时类型验证，可以在这里扩展
				// 目前仅验证编译时已有的类型信息
			}
			
			var obj *bytecode.Object
			if typeArgs != nil {
				// 使用带类型参数的对象创建（如果有）
				obj = bytecode.NewObjectInstanceWithTypes(class, typeArgs)
				// 验证泛型约束
				if err := vm.validateGenericConstraints(typeArgs, class.TypeParams); err != nil {
					return vm.runtimeError("%v", err)
				}
			} else {
				// 普通对象创建
				obj = bytecode.NewObjectInstance(class)
			}
			
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
			
			// 检查是否有 getter 方法（属性访问器）
			getterName := "get_" + name
			if getter := vm.lookupMethod(obj.Class, getterName); getter != nil {
				// 调用 getter 方法
				closure := &bytecode.Closure{
					Function: &bytecode.Function{
						Name:          getter.Name,
						ClassName:     getter.ClassName,
						SourceFile:    getter.SourceFile,
						Arity:         getter.Arity,
						MinArity:      getter.MinArity,
						Chunk:         getter.Chunk,
						LocalCount:    getter.LocalCount,
						DefaultValues: getter.DefaultValues,
					},
				}
				vm.push(objVal) // 将对象压回栈作为 receiver
				if result := vm.call(closure, 0); result != InterpretOK {
					return result
				}
				frame = &vm.frames[vm.frameCount-1]
				chunk = frame.Closure.Function.Chunk
				continue
			} else {
				// 普通字段访问
				// 检查访问权限
				if err := vm.checkPropertyAccess(obj.Class, name); err != nil {
					return vm.runtimeError("%v", err)
				}
				
				if value, ok := obj.GetField(name); ok {
					vm.push(value)
				} else {
					vm.push(bytecode.NullValue)
				}
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
			
			// 检查是否有 setter 方法（属性访问器）
			setterName := "set_" + name
			if setter := vm.lookupMethodByArity(obj.Class, setterName, 1); setter != nil {
				// 调用 setter 方法
				closure := &bytecode.Closure{
					Function: &bytecode.Function{
						Name:          setter.Name,
						ClassName:     setter.ClassName,
						SourceFile:    setter.SourceFile,
						Arity:         setter.Arity,
						MinArity:      setter.MinArity,
						Chunk:         setter.Chunk,
						LocalCount:    setter.LocalCount,
						DefaultValues: setter.DefaultValues,
					},
				}
				vm.push(objVal) // 将对象压回栈作为 receiver
				vm.push(value)  // 将值压入栈作为参数
				if result := vm.call(closure, 1); result != InterpretOK {
					return result
				}
				frame = &vm.frames[vm.frameCount-1]
				chunk = frame.Closure.Function.Chunk
				// setter 可能返回 void，但我们需要返回设置的值
				vm.push(value)
			} else {
				// 普通字段访问
				// 检查访问权限
				if err := vm.checkPropertyAccess(obj.Class, name); err != nil {
					return vm.runtimeError("%v", err)
				}
				
				// 检查 final 属性 - 如果属性已有值且为 final，则不允许重新赋值
				// （第一次赋值在构造函数中是允许的）
				if obj.Class.PropFinal[name] {
					if _, exists := obj.GetField(name); exists {
						return vm.runtimeError(i18n.T(i18n.ErrCannotAssignFinalProperty, name))
					}
				}
				
				obj.SetField(name, value)
				vm.push(value)
			}

		case bytecode.OpCallMethod:
			callSiteIP := frame.IP - 1 // 保存调用点 IP（用于内联缓存）
			nameIdx := chunk.ReadU16(frame.IP)
			frame.IP += 2
			argCount := int(chunk.Code[frame.IP])
			frame.IP++
			name := chunk.Constants[nameIdx].AsString()
			
			// 特殊处理构造函数 - 如果不存在则跳过
			receiver := vm.peek(argCount)
			if receiver.Type == bytecode.ValObject {
				obj := receiver.AsObject()
				
				// 尝试内联缓存快速路径
				if vm.icManager.IsEnabled() {
					ic := vm.icManager.GetMethodCache(frame.Closure.Function, callSiteIP)
					if ic != nil {
						if method, hit := ic.Lookup(obj.Class); hit {
							// 缓存命中：直接调用
							if result := vm.callMethodDirect(obj, method, argCount); result != InterpretOK {
								return result
							}
							frame = &vm.frames[vm.frameCount-1]
							chunk = frame.Closure.Function.Chunk
							continue
						}
					}
				}
				
				method := obj.Class.GetMethod(name)
				if method == nil && name == "__construct" {
					// 没有构造函数，跳过调用，只保留对象在栈上
					continue
				}
				
				// 更新内联缓存
				if vm.icManager.IsEnabled() && method != nil {
					ic := vm.icManager.GetMethodCache(frame.Closure.Function, callSiteIP)
					if ic != nil {
						ic.Update(obj.Class, method)
					}
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
			
			// 检查是否有静态属性 getter
			class, err := vm.resolveClassName(className)
			if err != nil {
				return vm.runtimeError("%v", err)
			}
			
			getterName := "get_" + name
			if getter := vm.lookupMethod(class, getterName); getter != nil && getter.IsStatic {
				// 调用静态 getter 方法
				closure := &bytecode.Closure{
					Function: &bytecode.Function{
						Name:          getter.Name,
						ClassName:     getter.ClassName,
						SourceFile:    getter.SourceFile,
						Arity:         getter.Arity,
						MinArity:      getter.MinArity,
						Chunk:         getter.Chunk,
						LocalCount:    getter.LocalCount,
						DefaultValues: getter.DefaultValues,
					},
				}
				vm.push(bytecode.NullValue) // 静态方法使用 null 作为占位符
				if result := vm.call(closure, 0); result != InterpretOK {
					return result
				}
				frame = &vm.frames[vm.frameCount-1]
				chunk = frame.Closure.Function.Chunk
				continue
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
			
			// 检查是否有静态属性 setter
			setterName := "set_" + name
			if setter := vm.lookupMethodByArity(class, setterName, 1); setter != nil && setter.IsStatic {
				// 调用静态 setter 方法
				closure := &bytecode.Closure{
					Function: &bytecode.Function{
						Name:          setter.Name,
						ClassName:     setter.ClassName,
						SourceFile:    setter.SourceFile,
						Arity:         setter.Arity,
						MinArity:      setter.MinArity,
						Chunk:         setter.Chunk,
						LocalCount:    setter.LocalCount,
						DefaultValues: setter.DefaultValues,
					},
				}
				vm.push(bytecode.NullValue) // 静态方法使用 null 作为占位符
				vm.push(value)              // 将值压入栈作为参数
				if result := vm.call(closure, 1); result != InterpretOK {
					return result
				}
				frame = &vm.frames[vm.frameCount-1]
				chunk = frame.Closure.Function.Chunk
				vm.push(value) // setter 可能返回 void，但我们需要返回设置的值
			} else {
				// 普通静态字段访问
				vm.setStaticVar(class, name, value)
				vm.push(value)
			}

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
					Name:          method.Name,
					ClassName:    method.ClassName, // 设置类名用于堆栈跟踪
					SourceFile:   method.SourceFile,
					Arity:        method.Arity,
					MinArity:     method.MinArity, // 支持默认参数
					Chunk:        method.Chunk,
					LocalCount:   method.LocalCount,
					DefaultValues: method.DefaultValues, // 包含默认参数值
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
					if result := vm.throwTypedException("ArrayIndexOutOfBoundsException", i18n.T(i18n.ErrArrayIndexSimple)); result == InterpretExceptionHandled {
						frame = &vm.frames[vm.frameCount-1]
						chunk = frame.Closure.Function.Chunk
						continue
					} else {
						return result
					}
				}
				vm.push(arr[i])
			case bytecode.ValFixedArray:
				fa := arrVal.AsFixedArray()
				i := int(idx.AsInt())
				if i < 0 || i >= fa.Capacity {
					if result := vm.throwTypedException("ArrayIndexOutOfBoundsException", i18n.T(i18n.ErrArrayIndexOutOfBounds, i, fa.Capacity)); result == InterpretExceptionHandled {
						frame = &vm.frames[vm.frameCount-1]
						chunk = frame.Closure.Function.Chunk
						continue
					} else {
						return result
					}
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
			case bytecode.ValSuperArray:
				// SuperArray 索引支持
				sa := arrVal.AsSuperArray()
				if value, ok := sa.Get(idx); ok {
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
					if result := vm.throwTypedException("ArrayIndexOutOfBoundsException", i18n.T(i18n.ErrArrayIndexSimple)); result == InterpretExceptionHandled {
						frame = &vm.frames[vm.frameCount-1]
						chunk = frame.Closure.Function.Chunk
						continue
					} else {
						return result
					}
				}
				arr[i] = value
			case bytecode.ValFixedArray:
				fa := arrVal.AsFixedArray()
				i := int(idx.AsInt())
				if i < 0 || i >= fa.Capacity {
					if result := vm.throwTypedException("ArrayIndexOutOfBoundsException", i18n.T(i18n.ErrArrayIndexOutOfBounds, i, fa.Capacity)); result == InterpretExceptionHandled {
						frame = &vm.frames[vm.frameCount-1]
						chunk = frame.Closure.Function.Chunk
						continue
					} else {
						return result
					}
				}
				fa.Elements[i] = value
			case bytecode.ValMap:
				// Map 设置
				m := arrVal.AsMap()
				m[idx] = value
			case bytecode.ValSuperArray:
				// SuperArray 设置
				sa := arrVal.AsSuperArray()
				sa.Set(idx, value)
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
			case bytecode.ValSuperArray:
				vm.push(bytecode.NewInt(int64(arrVal.AsSuperArray().Len())))
			default:
				return vm.runtimeError(i18n.T(i18n.ErrLengthRequiresArray))
			}

		// 无边界检查的数组访问（用于循环优化，边界检查已在循环外完成）
		case bytecode.OpArrayGetUnchecked:
			idx := vm.pop()
			arrVal := vm.pop()
			switch arrVal.Type {
			case bytecode.ValArray:
				arr := arrVal.AsArray()
				i := int(idx.AsInt())
				vm.push(arr[i])
			case bytecode.ValFixedArray:
				fa := arrVal.AsFixedArray()
				i := int(idx.AsInt())
				vm.push(fa.Elements[i])
			default:
				return vm.runtimeError(i18n.T(i18n.ErrSubscriptRequiresArray))
			}

		case bytecode.OpArraySetUnchecked:
			value := vm.pop()
			idx := vm.pop()
			arrVal := vm.pop()
			switch arrVal.Type {
			case bytecode.ValArray:
				arr := arrVal.AsArray()
				i := int(idx.AsInt())
				arr[i] = value
			case bytecode.ValFixedArray:
				fa := arrVal.AsFixedArray()
				i := int(idx.AsInt())
				fa.Elements[i] = value
			default:
				return vm.runtimeError(i18n.T(i18n.ErrSubscriptRequiresArray))
			}
			vm.push(value)

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
			case bytecode.ValSuperArray:
				sa := container.AsSuperArray()
				vm.push(bytecode.NewBool(sa.HasKey(key)))
			default:
				return vm.runtimeError(i18n.T(i18n.ErrHasRequiresArrayOrMap))
			}

		case bytecode.OpMapLen:
			mapVal := vm.pop()
			if mapVal.Type != bytecode.ValMap {
				return vm.runtimeError(i18n.T(i18n.ErrLengthRequiresMap))
			}
			vm.push(bytecode.NewInt(int64(len(mapVal.AsMap()))))

		// SuperArray 万能数组操作
		case bytecode.OpSuperArrayNew:
			count := int(chunk.ReadU16(frame.IP))
			frame.IP += 2
			sa := bytecode.NewSuperArray()

			// 从栈上收集元素（按相反顺序）
			// 栈上结构: [value1, flag1, value2, flag2, ...] 或 [key, value, flag, ...]
			type elem struct {
				hasKey bool
				key    bytecode.Value
				value  bytecode.Value
			}
			elements := make([]elem, count)

			for i := count - 1; i >= 0; i-- {
				flag := vm.pop().AsInt()
				if flag == 1 {
					// 键值对
					value := vm.pop()
					key := vm.pop()
					elements[i] = elem{hasKey: true, key: key, value: value}
				} else {
					// 仅值
					value := vm.pop()
					elements[i] = elem{hasKey: false, value: value}
				}
			}

			// 按正确顺序填充 SuperArray
			for _, e := range elements {
				if e.hasKey {
					sa.Set(e.key, e.value)
				} else {
					sa.Push(e.value)
				}
			}

			vm.push(vm.trackAllocation(bytecode.NewSuperArrayValue(sa)))

		case bytecode.OpSuperArrayGet:
			key := vm.pop()
			arrVal := vm.pop()
			if arrVal.Type != bytecode.ValSuperArray {
				return vm.runtimeError("expected super array")
			}
			sa := arrVal.AsSuperArray()
			if value, ok := sa.Get(key); ok {
				vm.push(value)
			} else {
				vm.push(bytecode.NullValue)
			}

		case bytecode.OpSuperArraySet:
			value := vm.pop()
			key := vm.pop()
			arrVal := vm.pop()
			if arrVal.Type != bytecode.ValSuperArray {
				return vm.runtimeError("expected super array")
			}
			sa := arrVal.AsSuperArray()
			sa.Set(key, value)
			vm.push(value)

		// 迭代器操作
		case bytecode.OpIterInit:
			v := vm.pop()
			if v.Type != bytecode.ValArray && v.Type != bytecode.ValFixedArray && v.Type != bytecode.ValMap && v.Type != bytecode.ValSuperArray {
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
			switch arrVal.Type {
			case bytecode.ValArray:
				arr := arrVal.AsArray()
				arr = append(arr, value)
				vm.push(vm.trackAllocation(bytecode.NewArray(arr)))
			case bytecode.ValSuperArray:
				sa := arrVal.AsSuperArray()
				sa.Push(value)
				vm.push(arrVal) // SuperArray 是引用类型，直接返回
			default:
				return vm.runtimeError(i18n.T(i18n.ErrPushRequiresArray))
			}

		case bytecode.OpArrayHas:
			idx := vm.pop()
			arrVal := vm.pop()
			if arrVal.Type == bytecode.ValSuperArray {
				sa := arrVal.AsSuperArray()
				vm.push(bytecode.NewBool(sa.HasKey(idx)))
				continue
			}
			if arrVal.Type != bytecode.ValArray {
				return vm.runtimeError(i18n.T(i18n.ErrHasRequiresArray))
			}
			arr := arrVal.AsArray()
			i := int(idx.AsInt())
			vm.push(bytecode.NewBool(i >= 0 && i < len(arr)))

		// 字节数组操作
		case bytecode.OpNewBytes:
			count := int(chunk.ReadU16(frame.IP))
			frame.IP += 2
			bytes := make([]byte, count)
			for i := count - 1; i >= 0; i-- {
				bytes[i] = byte(vm.pop().AsInt() & 0xFF)
			}
			vm.push(vm.trackAllocation(bytecode.NewBytes(bytes)))

		case bytecode.OpBytesGet:
			idx := vm.pop()
			bytesVal := vm.pop()
			if bytesVal.Type != bytecode.ValBytes {
				return vm.runtimeError("expected bytes for index operation")
			}
			b := bytesVal.AsBytes()
			i := int(idx.AsInt())
			if i < 0 || i >= len(b) {
				if result := vm.throwTypedException("ArrayIndexOutOfBoundsException", 
					i18n.T(i18n.ErrArrayIndexOutOfBounds, i, len(b))); result == InterpretExceptionHandled {
					frame = &vm.frames[vm.frameCount-1]
					chunk = frame.Closure.Function.Chunk
					continue
				} else {
					return result
				}
			}
			vm.push(bytecode.NewInt(int64(b[i])))

		case bytecode.OpBytesSet:
			value := vm.pop()
			idx := vm.pop()
			bytesVal := vm.pop()
			if bytesVal.Type != bytecode.ValBytes {
				return vm.runtimeError("expected bytes for set operation")
			}
			b := bytesVal.AsBytes()
			i := int(idx.AsInt())
			if i < 0 || i >= len(b) {
				if result := vm.throwTypedException("ArrayIndexOutOfBoundsException", 
					i18n.T(i18n.ErrArrayIndexOutOfBounds, i, len(b))); result == InterpretExceptionHandled {
					frame = &vm.frames[vm.frameCount-1]
					chunk = frame.Closure.Function.Chunk
					continue
				} else {
					return result
				}
			}
			b[i] = byte(value.AsInt() & 0xFF)
			vm.push(bytesVal)

		case bytecode.OpBytesLen:
			bytesVal := vm.pop()
			if bytesVal.Type != bytecode.ValBytes {
				return vm.runtimeError("expected bytes for length operation")
			}
			vm.push(bytecode.NewInt(int64(len(bytesVal.AsBytes()))))

		case bytecode.OpBytesSlice:
			endVal := vm.pop()
			startVal := vm.pop()
			bytesVal := vm.pop()
			if bytesVal.Type != bytecode.ValBytes {
				return vm.runtimeError("expected bytes for slice operation")
			}
			b := bytesVal.AsBytes()
			start := int(startVal.AsInt())
			end := int(endVal.AsInt())
			if end < 0 {
				end = len(b)
			}
			if start < 0 {
				start = 0
			}
			if start > len(b) {
				start = len(b)
			}
			if end > len(b) {
				end = len(b)
			}
			if start > end {
				start = end
			}
			result := make([]byte, end-start)
			copy(result, b[start:end])
			vm.push(vm.trackAllocation(bytecode.NewBytes(result)))

		case bytecode.OpBytesConcat:
			b2Val := vm.pop()
			b1Val := vm.pop()
			if b1Val.Type != bytecode.ValBytes || b2Val.Type != bytecode.ValBytes {
				return vm.runtimeError("expected bytes for concat operation")
			}
			b1 := b1Val.AsBytes()
			b2 := b2Val.AsBytes()
			result := make([]byte, len(b1)+len(b2))
			copy(result, b1)
			copy(result[len(b1):], b2)
			vm.push(vm.trackAllocation(bytecode.NewBytes(result)))

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
			exceptionVal := vm.pop()
			var exception bytecode.Value
			
			// 处理不同类型的异常
			switch exceptionVal.Type {
			case bytecode.ValString:
				// 字符串异常：自动转换为 Exception 对象
				exception = bytecode.NewException("Exception", exceptionVal.AsString(), 0)
			case bytecode.ValObject:
				// 对象异常：检查是否是 Throwable 的子类
				obj := exceptionVal.AsObject()
				if vm.isThrowable(obj.Class) {
					exception = bytecode.NewExceptionFromObject(obj)
				} else {
					return vm.runtimeError("cannot throw non-Throwable object: %s", obj.Class.Name)
				}
			case bytecode.ValException:
				// 已经是异常值
				exception = exceptionVal
			default:
				return vm.runtimeError("cannot throw value of type %v", exceptionVal.Type)
			}
			
			// 捕获调用栈信息
			if exc := exception.AsException(); exc != nil && len(exc.StackFrames) == 0 {
				exc.SetStackFrames(vm.captureStackTrace())
			}
			if !vm.handleException(exception) {
				if exc := exception.AsException(); exc != nil {
					return vm.runtimeErrorWithException(exc)
				}
				return vm.runtimeError("uncaught exception: %s", exception.String())
			}
			// 更新 frame 和 chunk 引用
			frame = &vm.frames[vm.frameCount-1]
			chunk = frame.Closure.Function.Chunk

		case bytecode.OpEnterTry:
			// 新格式: OpEnterTry catchCount:u8 finallyOffset:i16 [typeIdx:u16 catchOffset:i16]*catchCount
			enterTryIP := frame.IP - 1 // OpEnterTry 指令的位置
			
			catchCount := int(chunk.Code[frame.IP])
			frame.IP++
			
			finallyOffset := chunk.ReadI16(frame.IP)
			frame.IP += 2
			
			// 读取 catch 处理器信息
			var catchHandlers []CatchHandlerInfo
			for i := 0; i < catchCount; i++ {
				typeIdx := chunk.ReadU16(frame.IP)
				frame.IP += 2
				catchOffset := chunk.ReadI16(frame.IP)
				frame.IP += 2
				
				typeName := chunk.Constants[typeIdx].AsString()
				catchHandlers = append(catchHandlers, CatchHandlerInfo{
					TypeName:    typeName,
					CatchOffset: int(catchOffset),
				})
			}
			
			// 计算 finally 块的绝对地址 (-1 表示没有 finally)
			finallyIP := -1
			if finallyOffset != -1 {
				finallyIP = enterTryIP + int(finallyOffset)
			}
			
		vm.tryStack = append(vm.tryStack, TryContext{
			EnterTryIP:    enterTryIP,
			CatchHandlers: catchHandlers,
			FinallyIP:     finallyIP,
			FrameCount:    vm.frameCount,
			StackTop:      vm.stackTop,
		})
		vm.tryDepth++ // 更新 try 深度计数

		case bytecode.OpLeaveTry:
			// 离开 try 块（正常流程）- 零成本异常路径优化
			// 如果没有 finally 块，移除 TryContext
			// 如果有 finally 块，保留 TryContext 供 finally 使用
			if vm.tryDepth > 0 {
				tryCtx := &vm.tryStack[len(vm.tryStack)-1]
				if tryCtx.FinallyIP < 0 {
					// 没有 finally 块，移除 TryContext
					vm.tryStack = vm.tryStack[:len(vm.tryStack)-1]
					vm.tryDepth-- // 更新 try 深度计数
				}
				// 有 finally 块，保留 TryContext，等待 OpLeaveFinally 时移除
			}

		case bytecode.OpEnterCatch:
			// 新格式: OpEnterCatch typeIdx:u16
			// typeIdx 用于调试/日志，VM 在 handleException 中已经做了类型匹配
			frame.IP += 2 // 跳过 typeIdx
			// 异常值已经在栈上
			// 清除异常状态
			vm.hasException = false
			// 标记当前 TryContext 正在执行 catch 块
			if len(vm.tryStack) > 0 {
				vm.tryStack[len(vm.tryStack)-1].InCatch = true
			}

		case bytecode.OpEnterFinally:
			// 进入 finally 块
			// 设置 InFinally 标志，标记当前正在执行 finally 块
			if len(vm.tryStack) > 0 {
				vm.tryStack[len(vm.tryStack)-1].InFinally = true
			}

		case bytecode.OpLeaveFinally:
			// 离开 finally 块，检查是否有挂起的异常或返回值 - 零成本异常路径优化
			if vm.tryDepth > 0 {
				tryCtx := &vm.tryStack[len(vm.tryStack)-1]
				if tryCtx.InFinally {
					tryCtx.InFinally = false
					vm.tryStack = vm.tryStack[:len(vm.tryStack)-1]
					vm.tryDepth-- // 更新 try 深度计数
					
					// 如果有挂起的异常，重新抛出
					if tryCtx.HasPendingExc {
						if !vm.handleException(tryCtx.PendingException) {
							if exc := tryCtx.PendingException.AsException(); exc != nil {
								return vm.runtimeErrorWithException(exc)
							}
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
					// finally 块正常执行完毕，没有挂起的异常或返回值，继续执行后续代码
				}
			}

		case bytecode.OpRethrow:
			// 重新抛出当前异常
			if vm.hasException {
				if !vm.handleException(vm.exception) {
					if exc := vm.exception.AsException(); exc != nil {
						return vm.runtimeErrorWithException(exc)
					}
					return vm.runtimeError("uncaught exception: %s", vm.exception.String())
				}
				frame = &vm.frames[vm.frameCount-1]
				chunk = frame.Closure.Function.Chunk
			}

		// 类型检查 - 用于 is 表达式的运行时类型检查
		case bytecode.OpCheckType:
			typeIdx := chunk.ReadU16(frame.IP)
			frame.IP += 2
			targetType := chunk.Constants[typeIdx].AsString()
			value := vm.pop()
			
			// 执行类型检查，返回布尔结果
			result := vm.checkValueType(value, targetType)
			vm.push(bytecode.NewBool(result))
			
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

		case bytecode.OpCastSafe:
			typeIdx := chunk.ReadU16(frame.IP)
			frame.IP += 2
			targetType := chunk.Constants[typeIdx].AsString()
			value := vm.pop()
			
			result, ok := vm.castValue(value, targetType)
			if !ok {
				// 安全类型转换失败时返回 null
				vm.push(bytecode.NullValue)
			} else {
				vm.push(result)
			}

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
	if exc := exception.AsException(); exc != nil && len(exc.StackFrames) == 0 {
		exc.SetStackFrames(vm.captureStackTrace())
	}
	
	// 获取异常对象用于类型匹配
	exc := exception.AsException()
	
	// 查找最近的 try 块
	for len(vm.tryStack) > 0 {
		tryCtx := &vm.tryStack[len(vm.tryStack)-1]
		
		// 如果正在执行 finally 块中发生异常，记录但继续传播
		if tryCtx.InFinally {
			vm.tryStack = vm.tryStack[:len(vm.tryStack)-1]
			continue
		}
		
		// 如果正在执行 catch 块中发生异常，需要执行 finally（如果有）然后传播
		if tryCtx.InCatch {
			// 如果有 finally 块，先执行 finally
			if tryCtx.FinallyIP >= 0 {
				tryCtx.PendingException = exception
				tryCtx.HasPendingExc = true
				tryCtx.InFinally = true
				tryCtx.InCatch = false
				if vm.frameCount > 0 {
					frame := &vm.frames[vm.frameCount-1]
					frame.IP = tryCtx.FinallyIP
				}
				vm.hasException = false
				return true
			}
			// 没有 finally，移除此 TryContext 并继续传播
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
			
			// 查找匹配的 catch 处理器
			matchedHandler := -1
			for i, handler := range tryCtx.CatchHandlers {
				if vm.exceptionMatchesType(exc, handler.TypeName) {
					matchedHandler = i
					break
				}
			}
			
			if matchedHandler >= 0 {
				// 找到匹配的 catch 处理器
				handler := tryCtx.CatchHandlers[matchedHandler]
				catchIP := tryCtx.EnterTryIP + handler.CatchOffset
				frame.IP = catchIP
				
				// 如果异常有关联的对象，推入对象；否则推入异常值
				if exc != nil && exc.Object != nil {
					vm.push(bytecode.NewObject(exc.Object))
				} else {
					vm.push(exception)
				}
				
				vm.hasException = false
				// 不要移除 tryCtx，因为 catch 块执行完后可能还需要执行 finally
				return true
			} else if tryCtx.FinallyIP >= 0 {
				// 没有匹配的 catch，但有 finally 块
				// 挂起异常，先执行 finally
				tryCtx.PendingException = exception
				tryCtx.HasPendingExc = true
				tryCtx.InFinally = true
				frame.IP = tryCtx.FinallyIP
				vm.hasException = false
				return true
			}
			// 没有匹配的处理器也没有 finally，继续向上传播
		}
		
		vm.tryStack = vm.tryStack[:len(vm.tryStack)-1]
	}
	
	// 没有找到处理程序
	return false
}

// captureStackTrace 捕获当前调用栈信息
func (vm *VM) captureStackTrace() []bytecode.StackFrame {
	var frames []bytecode.StackFrame
	for i := vm.frameCount - 1; i >= 0; i-- {
		frame := &vm.frames[i]
		fn := frame.Closure.Function
		line := 0
		if frame.IP > 0 && frame.IP-1 < len(fn.Chunk.Lines) {
			line = fn.Chunk.Lines[frame.IP-1]
		}
		frames = append(frames, bytecode.StackFrame{
			FunctionName: fn.Name,
			ClassName:    fn.ClassName, // 设置类名用于堆栈跟踪
			FileName:     fn.SourceFile,
			LineNumber:   line,
		})
	}
	return frames
}

// exceptionMatchesType 检查异常是否匹配指定类型
func (vm *VM) exceptionMatchesType(exc *bytecode.Exception, typeName string) bool {
	if exc == nil {
		return false
	}
	
	// 使用 bytecode 包中的类型匹配逻辑
	if exc.IsExceptionOfType(typeName) {
		return true
	}
	
	// 如果有关联的类对象，检查 VM 中注册的类层次结构
	if exc.Object != nil {
		return vm.isInstanceOfType(exc.Object.Class, typeName)
	}
	
	return false
}

// isInstanceOfType 检查一个类是否是指定类型或其子类
func (vm *VM) isInstanceOfType(class *bytecode.Class, typeName string) bool {
	// 遍历类继承链
	for c := class; c != nil; c = c.Parent {
		if c.Name == typeName {
			return true
		}
	}
	
	// 也检查 VM 中注册的类（用于处理 parent 尚未解析的情况）
	if class.ParentName != "" && class.Parent == nil {
		if parent := vm.classes[class.ParentName]; parent != nil {
			return vm.isInstanceOfType(parent, typeName)
		}
	}
	
	return false
}

// isThrowable 检查一个类是否是 Throwable 或其子类
func (vm *VM) isThrowable(class *bytecode.Class) bool {
	return vm.isInstanceOfType(class, "Throwable")
}

// throwRuntimeException 抛出一个运行时异常（可被 try-catch 捕获）
func (vm *VM) throwRuntimeException(message string) InterpretResult {
	return vm.throwTypedException("RuntimeException", message)
}

// throwTypedException 抛出指定类型的异常（可被 try-catch 捕获）
// typeName: 异常类型名（如 "DivideByZeroException", "ArrayIndexOutOfBoundsException"）
// 会按顺序尝试: 指定类型（完整名称和简单名称）-> RuntimeException -> Exception -> 简单异常
func (vm *VM) throwTypedException(typeName string, message string) InterpretResult {
	var exception bytecode.Value
	
	// 尝试按优先级查找异常类
	// 首先尝试完整命名空间名称（sola.lang.类名），然后尝试简单类名
	var classNames []string
	if typeName != "RuntimeException" && typeName != "Exception" {
		// 对于标准库异常，尝试完整命名空间名称
		classNames = []string{"sola.lang." + typeName, typeName, "sola.lang.RuntimeException", "RuntimeException", "sola.lang.Exception", "Exception"}
	} else {
		classNames = []string{"sola.lang." + typeName, typeName, "sola.lang.Exception", "Exception"}
	}
	var foundClass *bytecode.Class
	var foundTypeName string
	
	for _, name := range classNames {
		if class := vm.classes[name]; class != nil {
			foundClass = class
			foundTypeName = name
			break
		}
	}
	
	if foundClass != nil {
		// 找到异常类，创建对象实例
		obj := bytecode.NewObjectInstance(foundClass)
		obj.Fields["message"] = bytecode.NewString(message)
		obj.Fields["code"] = bytecode.NewInt(0)
		obj.Fields["previous"] = bytecode.NullValue
		obj.Fields["stackTrace"] = bytecode.NewArray([]bytecode.Value{})
		exception = bytecode.NewExceptionFromObject(obj)
	} else {
		// 没有异常类可用，使用简单异常值
		exception = bytecode.NewException(typeName, message, 0)
		foundTypeName = typeName
	}
	
	// 如果找到的不是请求的类型，但我们有消息，更新异常类型名
	if exc := exception.AsException(); exc != nil {
		if foundTypeName != typeName && foundClass != nil {
			// 即使使用了父类，也保留原始类型名用于显示
			exc.Type = typeName
		}
		if len(exc.StackFrames) == 0 {
			exc.SetStackFrames(vm.captureStackTrace())
		}
	}
	
	if vm.handleException(exception) {
		// 异常被捕获，返回特殊状态让调用者刷新 frame/chunk
		return InterpretExceptionHandled
	}
	// 未捕获的异常
	if exc := exception.AsException(); exc != nil {
		return vm.runtimeErrorWithException(exc)
	}
	return vm.runtimeError("uncaught exception: %s", exception.String())
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

	// 【重要】字符串拼接：只有两个操作数都是字符串时才允许拼接
	// 【警告】请勿将条件修改为 (a.Type == ValString || b.Type == ValString)
	// 否则会导致类型不安全的隐式转换问题：
	//   - "hello" + 123 会变成 "hello123"（应该报错）
	//   - 456 + "world" 会变成 "456world"（应该报错）
	// 正确的行为：只有 string + string 才能相加，其他情况应该报类型错误
	if op == bytecode.OpAdd && a.Type == bytecode.ValString && b.Type == bytecode.ValString {
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
				return vm.throwTypedException("DivideByZeroException", i18n.T(i18n.ErrDivisionByZero))
			}
			vm.push(bytecode.NewInt(ai / bi))
		case bytecode.OpMod:
			if bi == 0 {
				return vm.throwTypedException("DivideByZeroException", i18n.T(i18n.ErrDivisionByZero))
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
				return vm.throwTypedException("DivideByZeroException", i18n.T(i18n.ErrDivisionByZero))
			}
			vm.push(bytecode.NewFloat(af / bf))
		case bytecode.OpMod:
			return vm.throwRuntimeException(i18n.T(i18n.ErrModuloNotForFloats))
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
		// 使用优化的调用路径
		return vm.callOptimized(callee.Data.(*bytecode.Closure), argCount)
	case bytecode.ValFunc:
		fn := callee.Data.(*bytecode.Function)
		// 特殊处理内置函数（使用优化路径）
		if fn.IsBuiltin && fn.BuiltinFn != nil {
			return vm.callBuiltinOptimized(fn, argCount)
		}
		closure := &bytecode.Closure{Function: fn}
		return vm.callOptimized(closure, argCount)
	default:
		return vm.runtimeError(i18n.T(i18n.ErrCanOnlyCallFunctions))
	}
}

// tailCall 尾调用：复用当前栈帧而非创建新帧
func (vm *VM) tailCall(callee bytecode.Value, argCount int) InterpretResult {
	var closure *bytecode.Closure
	
	switch callee.Type {
	case bytecode.ValClosure:
		closure = callee.Data.(*bytecode.Closure)
	case bytecode.ValFunc:
		fn := callee.Data.(*bytecode.Function)
		// 内置函数不支持尾调用（需要返回值）
		if fn.IsBuiltin && fn.BuiltinFn != nil {
			return vm.runtimeError("tail call not supported for builtin functions")
		}
		closure = &bytecode.Closure{Function: fn}
	default:
		return vm.runtimeError(i18n.T(i18n.ErrCanOnlyCallFunctions))
	}
	
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
	
	// 获取当前帧
	currentFrame := &vm.frames[vm.frameCount-1]
	
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
	
	// 将参数从栈顶移动到当前帧的参数位置
	// 栈布局：[..., old_callee, old_arg0, old_arg1, ..., new_arg0, new_arg1, ...]
	// 需要移动到：[..., new_callee, new_arg0, new_arg1, ...]
	
	// 保存新参数（从栈顶开始）
	newArgs := make([]bytecode.Value, argCount+1) // +1 for callee
	for i := argCount; i >= 0; i-- {
		newArgs[i] = vm.pop()
	}
	
	// 调整栈：移除旧参数，保留调用者
	// 计算需要保留的栈大小（从调用者到旧参数之前）
	keepSize := currentFrame.BaseSlot
	vm.stackTop = keepSize
	
	// 将新参数和函数放回栈
	vm.push(bytecode.NewClosure(closure)) // 新函数
	for i := 0; i < argCount; i++ {
		vm.push(newArgs[i+1]) // 新参数
	}
	
	// 如果有 upvalues，将它们作为额外的局部变量
	for _, upval := range closure.Upvalues {
		if upval.IsClosed {
			vm.push(upval.Closed)
		} else {
			vm.push(*upval.Location)
		}
	}
	
	// 复用当前帧：更新闭包和 IP，重置 BaseSlot
	currentFrame.Closure = closure
	currentFrame.IP = 0
	currentFrame.BaseSlot = keepSize
	
	return InterpretOK
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
			if exc := result.AsException(); exc != nil {
				return vm.runtimeErrorWithException(exc)
			}
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

// callMethodDirect 直接调用方法（内联缓存命中时使用）
func (vm *VM) callMethodDirect(obj *bytecode.Object, method *bytecode.Method, argCount int) InterpretResult {
	// 热点检测：记录函数调用
	fn := &bytecode.Function{
		Name:          method.Name,
		ClassName:     method.ClassName,
		SourceFile:    method.SourceFile,
		Arity:         method.Arity,
		MinArity:      method.MinArity,
		Chunk:         method.Chunk,
		LocalCount:    method.LocalCount,
		DefaultValues: method.DefaultValues,
	}
	vm.hotspotDetector.RecordFunctionCall(fn)
	
	// 创建闭包并调用
	closure := &bytecode.Closure{Function: fn}
	return vm.callOptimized(closure, argCount)
}

// 调用方法
func (vm *VM) invokeMethod(name string, argCount int) InterpretResult {
	receiver := vm.peek(argCount)

	// 处理 SuperArray 内置方法
	if receiver.Type == bytecode.ValSuperArray {
		return vm.invokeSuperArrayMethod(name, argCount)
	}

	if receiver.Type != bytecode.ValObject {
		return vm.runtimeError(i18n.T(i18n.ErrOnlyObjectsHaveMethods))
	}
	
	obj := receiver.AsObject()
	// 使用参数数量查找重载方法（考虑默认参数）
	method := vm.findMethodWithDefaults(obj.Class, name, argCount)
	if method == nil {
		return vm.runtimeError(i18n.T(i18n.ErrUndefinedMethod, name, argCount))
	}

	// 热点检测：记录函数调用
	vm.hotspotDetector.RecordFunctionCall(&bytecode.Function{Name: method.Name, ClassName: method.ClassName})

	// 检查方法访问权限
	if err := vm.checkMethodAccess(obj.Class, method); err != nil {
		return vm.runtimeError("%v", err)
	}

	// 创建方法的闭包，包含默认参数信息
	closure := &bytecode.Closure{
		Function: &bytecode.Function{
			Name:          method.Name,
			ClassName:     method.ClassName, // 设置类名用于堆栈跟踪
			SourceFile:    method.SourceFile,
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

// invokeSuperArrayMethod 处理 SuperArray 的内置方法调用
func (vm *VM) invokeSuperArrayMethod(name string, argCount int) InterpretResult {
	// 收集参数（不包括 receiver）
	args := make([]bytecode.Value, argCount)
	for i := argCount - 1; i >= 0; i-- {
		args[i] = vm.pop()
	}
	receiver := vm.pop()
	sa := receiver.AsSuperArray()

	var result bytecode.Value

	switch name {
	case "len", "length":
		result = bytecode.NewInt(int64(sa.Len()))

	case "keys":
		keys := sa.Keys()
		newSa := bytecode.NewSuperArray()
		for _, k := range keys {
			newSa.Push(k)
		}
		result = bytecode.NewSuperArrayValue(newSa)

	case "values":
		values := sa.Values()
		newSa := bytecode.NewSuperArray()
		for _, v := range values {
			newSa.Push(v)
		}
		result = bytecode.NewSuperArrayValue(newSa)

	case "hasKey":
		if argCount < 1 {
			return vm.runtimeError("hasKey requires 1 argument")
		}
		result = bytecode.NewBool(sa.HasKey(args[0]))

	case "get":
		if argCount < 1 {
			return vm.runtimeError("get requires at least 1 argument")
		}
		if val, ok := sa.Get(args[0]); ok {
			result = val
		} else if argCount >= 2 {
			result = args[1] // default value
		} else {
			result = bytecode.NullValue
		}

	case "set":
		if argCount < 2 {
			return vm.runtimeError("set requires 2 arguments")
		}
		sa.Set(args[0], args[1])
		result = receiver // return self for chaining

	case "remove":
		if argCount < 1 {
			return vm.runtimeError("remove requires 1 argument")
		}
		result = bytecode.NewBool(sa.Remove(args[0]))

	case "push":
		if argCount < 1 {
			return vm.runtimeError("push requires 1 argument")
		}
		sa.Push(args[0])
		result = receiver // return self for chaining

	case "pop":
		if sa.Len() == 0 {
			result = bytecode.NullValue
		} else {
			lastIdx := sa.Len() - 1
			result = sa.Entries[lastIdx].Value
			// 移除最后一个元素
			key := sa.Entries[lastIdx].Key
			sa.Remove(key)
		}

	case "shift":
		if sa.Len() == 0 {
			result = bytecode.NullValue
		} else {
			result = sa.Entries[0].Value
			key := sa.Entries[0].Key
			sa.Remove(key)
		}

	case "unshift":
		if argCount < 1 {
			return vm.runtimeError("unshift requires 1 argument")
		}
		// 创建新数组，先添加新元素，再添加原有元素
		newSa := bytecode.NewSuperArray()
		newSa.Push(args[0])
		for _, entry := range sa.Entries {
			newSa.Set(entry.Key, entry.Value)
		}
		// 替换原数组内容
		sa.Entries = newSa.Entries
		sa.Index = newSa.Index
		sa.NextInt = newSa.NextInt
		result = receiver

	case "merge":
		if argCount < 1 || args[0].Type != bytecode.ValSuperArray {
			return vm.runtimeError("merge requires a SuperArray argument")
		}
		other := args[0].AsSuperArray()
		merged := sa.Copy()
		for _, entry := range other.Entries {
			merged.Set(entry.Key, entry.Value)
		}
		result = bytecode.NewSuperArrayValue(merged)

	case "slice":
		if argCount < 1 {
			return vm.runtimeError("slice requires at least 1 argument")
		}
		start := int(args[0].AsInt())
		end := sa.Len()
		if argCount >= 2 && args[1].AsInt() != -1 {
			end = int(args[1].AsInt())
		}
		if start < 0 {
			start = 0
		}
		if end > sa.Len() {
			end = sa.Len()
		}
		newSa := bytecode.NewSuperArray()
		for i := start; i < end; i++ {
			newSa.Push(sa.Entries[i].Value)
		}
		result = bytecode.NewSuperArrayValue(newSa)

	default:
		return vm.runtimeError("SuperArray has no method '%s'", name)
	}

	vm.push(result)
	return InterpretOK
}

// formatException 格式化异常为 Java/C# 风格的输出
func (vm *VM) formatException(exc *bytecode.Exception) string {
	var result string
	
	// 获取消息
	message := exc.Message
	if exc.Object != nil {
		if msgVal, ok := exc.Object.Fields["message"]; ok {
			message = msgVal.AsString()
		}
	}
	
	// 获取完整的异常类型名（包括命名空间）
	typeName := exc.Type
	if exc.Object != nil && exc.Object.Class != nil {
		typeName = exc.Object.Class.FullName()
	}
	
	// 异常类型和消息
	result = fmt.Sprintf("%s: %s\n", typeName, message)
	
	// 堆栈跟踪
	for _, frame := range exc.StackFrames {
		if frame.ClassName != "" {
			result += fmt.Sprintf("    at %s.%s (%s:%d)\n", 
				frame.ClassName, frame.FunctionName, frame.FileName, frame.LineNumber)
		} else if frame.FileName != "" {
			result += fmt.Sprintf("    at %s (%s:%d)\n", 
				frame.FunctionName, frame.FileName, frame.LineNumber)
		} else {
			result += fmt.Sprintf("    at %s (line %d)\n", 
				frame.FunctionName, frame.LineNumber)
		}
	}
	
	// 异常链
	if exc.Cause != nil {
		result += "\nCaused by: " + vm.formatException(exc.Cause)
	}
	
	return result
}

// runtimeErrorWithException 输出异常错误（Java/C# 风格）
func (vm *VM) runtimeErrorWithException(exc *bytecode.Exception) InterpretResult {
	vm.hadError = true
	vm.errorMessage = exc.Message
	
	// 输出格式化的异常信息
	fmt.Print(vm.formatException(exc))
	
	return InterpretRuntimeError
}

// 运行时错误
func (vm *VM) runtimeError(format string, args ...interface{}) InterpretResult {
	vm.hadError = true
	vm.errorMessage = fmt.Sprintf(format, args...)

	// 使用增强的错误报告（如果启用）
	if useEnhancedRuntimeErrors {
		frames := vm.captureStackTrace()
		vm.reportEnhancedError(vm.errorMessage, frames)
	} else {
		// 传统错误输出（Java/C# 风格）
		fmt.Printf("%s\n", vm.errorMessage)
		
		// 打印堆栈跟踪
		frames := vm.captureStackTrace()
		for _, frame := range frames {
			if frame.FileName != "" {
				fmt.Printf("    at %s (%s:%d)\n", frame.FunctionName, frame.FileName, frame.LineNumber)
			} else {
				fmt.Printf("    at %s (line %d)\n", frame.FunctionName, frame.LineNumber)
			}
		}
	}

	return InterpretRuntimeError
}

// reportEnhancedError 使用增强格式报告运行时错误
func (vm *VM) reportEnhancedError(message string, bcFrames []bytecode.StackFrame) {
	// 转换堆栈帧
	errFrames := make([]errors.StackFrame, len(bcFrames))
	for i, f := range bcFrames {
		errFrames[i] = errors.StackFrame{
			FunctionName: f.FunctionName,
			ClassName:    f.ClassName,
			FileName:     f.FileName,
			LineNumber:   f.LineNumber,
		}
	}

	// 创建运行时错误
	err := &errors.RuntimeError{
		Code:    errors.R0001, // 默认错误码，后续可以根据消息推断
		Level:   errors.LevelError,
		Message: message,
		Frames:  errFrames,
		Context: make(map[string]interface{}),
	}

	// 推断错误码
	err.Code = inferRuntimeErrorCode(message)

	// 使用报告器输出
	reporter := errors.GetDefaultReporter()
	reporter.ReportRuntimeError(err)
}

// inferRuntimeErrorCode 从消息推断错误码
func inferRuntimeErrorCode(message string) string {
	msg := strings.ToLower(message)

	if strings.Contains(msg, "索引") || strings.Contains(msg, "index") || strings.Contains(msg, "越界") {
		return errors.R0100
	}
	if strings.Contains(msg, "除") && strings.Contains(msg, "零") {
		return errors.R0200
	}
	if strings.Contains(msg, "division") && strings.Contains(msg, "zero") {
		return errors.R0200
	}
	if strings.Contains(msg, "数字") || strings.Contains(msg, "number") || strings.Contains(msg, "operand") {
		return errors.R0201
	}
	if strings.Contains(msg, "转换") || strings.Contains(msg, "cast") {
		return errors.R0301
	}
	if strings.Contains(msg, "栈溢出") || strings.Contains(msg, "stack overflow") {
		return errors.R0400
	}
	if strings.Contains(msg, "死循环") || strings.Contains(msg, "execution limit") {
		return errors.R0401
	}
	if strings.Contains(msg, "未定义的变量") || strings.Contains(msg, "undefined variable") {
		return errors.R0500
	}

	return errors.R0001
}

// useEnhancedRuntimeErrors 是否使用增强的运行时错误报告
var useEnhancedRuntimeErrors = false

// EnableEnhancedRuntimeErrors 启用增强的运行时错误报告
func EnableEnhancedRuntimeErrors() {
	useEnhancedRuntimeErrors = true
}

// DisableEnhancedRuntimeErrors 禁用增强的运行时错误报告
func DisableEnhancedRuntimeErrors() {
	useEnhancedRuntimeErrors = false
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
	case bytecode.ValBytes:
		return "bytes"
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
	
	// 联合类型检查 (type1|type2|type3)
	if strings.Contains(expectedType, "|") {
		expectedTypes := strings.Split(expectedType, "|")
		for _, t := range expectedTypes {
			if vm.checkValueType(v, strings.TrimSpace(t)) {
				return true
			}
		}
		return false
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
	case "array":
		// bytes 也被视为数组的兼容类型
		return actualType == "array" || actualType == "bytes"
	case "bytes":
		// bytes 类型也接受 array（用于兼容性）
		return actualType == "bytes" || actualType == "array"
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
			// 【重要】严格解析字符串为整数
			// 必须使用 strconv.ParseInt 进行严格验证，而不是 fmt.Sscanf
			// fmt.Sscanf("%d") 会解析到第一个非数字字符为止，导致以下错误行为：
			//   - "123abc" 会解析成功得到 123（应该失败，因为包含非数字字符）
			//   - "3.14" 会解析成功得到 3（应该失败，因为是浮点数）
			// strconv.ParseInt 会对整个字符串进行严格验证：
			//   - "123abc" -> 失败
			//   - "3.14" -> 失败
			//   - "123" -> 成功
			// 【警告】请勿将此修改回 fmt.Sscanf，否则会引入类型安全漏洞！
			s := strings.TrimSpace(v.AsString())
			i, err := strconv.ParseInt(s, 10, 64)
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

