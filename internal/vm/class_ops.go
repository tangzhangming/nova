package vm

import (
	"github.com/tangzhangming/nova/internal/bytecode"
)

// initializeObject 初始化对象实例属性
func (vm *VM) initializeObject(obj *bytecode.Object, class *bytecode.Class) {
	// 先初始化父类属性
	if class.Parent != nil {
		vm.initializeObject(obj, class.Parent)
	}
	
	// 复制类的属性默认值
	for name, value := range class.Properties {
		obj.Fields[name] = value
	}
}

// initObjectProperties 初始化对象属性（包括继承链）
func (vm *VM) initObjectProperties(obj *bytecode.Object, class *bytecode.Class) {
	vm.initializeObject(obj, class)
}

// lookupMethod 查找方法（包括继承链）
func (vm *VM) lookupMethod(class *bytecode.Class, name string) *bytecode.Method {
	// 当前类
	if method, ok := class.Methods[name]; ok {
		return method
	}
	
	// 父类
	if class.Parent != nil {
		return vm.lookupMethod(class.Parent, name)
	}
	
	return nil
}

// lookupConstant 查找常量（包括继承链）
func (vm *VM) lookupConstant(class *bytecode.Class, name string) (bytecode.Value, bool) {
	// 当前类
	if val, ok := class.Constants[name]; ok {
		return val, true
	}
	
	// 父类
	if class.Parent != nil {
		return vm.lookupConstant(class.Parent, name)
	}
	
	return bytecode.NullValue, false
}

// lookupStaticVar 查找静态变量（包括继承链）
func (vm *VM) lookupStaticVar(class *bytecode.Class, name string) (bytecode.Value, bool) {
	// 当前类
	if val, ok := class.StaticVars[name]; ok {
		return val, true
	}
	
	// 父类
	if class.Parent != nil {
		return vm.lookupStaticVar(class.Parent, name)
	}
	
	return bytecode.NullValue, false
}

// setStaticVar 设置静态变量
func (vm *VM) setStaticVar(class *bytecode.Class, name string, value bytecode.Value) bool {
	// 先在当前类查找
	if _, ok := class.StaticVars[name]; ok {
		class.StaticVars[name] = value
		return true
	}
	
	// 在父类查找
	if class.Parent != nil {
		return vm.setStaticVar(class.Parent, name, value)
	}
	
	// 没找到，创建在当前类
	class.StaticVars[name] = value
	return true
}

// isInstanceOf 检查对象是否是某个类的实例
func (vm *VM) isInstanceOf(obj *bytecode.Object, className string) bool {
	class := obj.Class
	for class != nil {
		if class.Name == className {
			return true
		}
		class = class.Parent
	}
	return false
}

// resolveParentClass 解析父类引用
func (vm *VM) resolveParentClass(class *bytecode.Class, parentName string) *bytecode.Class {
	if parent, ok := vm.classes[parentName]; ok {
		return parent
	}
	return nil
}

// CallParentMethod 调用父类方法
func (vm *VM) callParentMethod(class *bytecode.Class, methodName string, argCount int) InterpretResult {
	if class.Parent == nil {
		return vm.runtimeError("no parent class")
	}
	
	method := vm.lookupMethod(class.Parent, methodName)
	if method == nil {
		return vm.runtimeError("undefined method '%s' in parent class", methodName)
	}
	
	// 创建方法的闭包并调用
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

// CallConstructor 调用构造函数
func (vm *VM) callConstructor(obj *bytecode.Object, argCount int) InterpretResult {
	// 查找 __construct 方法
	method := vm.lookupMethod(obj.Class, "__construct")
	if method == nil {
		// 没有构造函数，检查参数
		if argCount != 0 {
			return vm.runtimeError("constructor takes no arguments")
		}
		return InterpretOK
	}
	
	// 调用构造函数
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

