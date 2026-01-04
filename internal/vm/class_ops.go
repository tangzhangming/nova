package vm

import (
	"fmt"
	
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

// lookupMethod 查找方法（包括继承链），返回第一个匹配的方法
func (vm *VM) lookupMethod(class *bytecode.Class, name string) *bytecode.Method {
	// 当前类
	if methods, ok := class.Methods[name]; ok && len(methods) > 0 {
		return methods[0]
	}
	
	// 父类
	if class.Parent != nil {
		return vm.lookupMethod(class.Parent, name)
	}
	
	return nil
}

// lookupMethodByArity 根据参数数量查找方法（支持方法重载）
func (vm *VM) lookupMethodByArity(class *bytecode.Class, name string, arity int) *bytecode.Method {
	// 当前类
	if methods, ok := class.Methods[name]; ok {
		for _, m := range methods {
			if m.Arity == arity {
				return m
			}
		}
		// 没有精确匹配，返回第一个
		if len(methods) > 0 {
			return methods[0]
		}
	}
	
	// 父类
	if class.Parent != nil {
		return vm.lookupMethodByArity(class.Parent, name, arity)
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

// resolveClassName 解析类名，处理 self 和 parent
func (vm *VM) resolveClassName(className string) (*bytecode.Class, error) {
	if className == "self" || className == "parent" {
		// 从当前帧获取 $this 对象来确定当前类
		if vm.frameCount == 0 {
			return nil, vm.makeError("cannot use '%s' outside of a class", className)
		}
		
		frame := &vm.frames[vm.frameCount-1]
		thisValue := vm.stack[frame.BaseSlot]
		
		if thisValue.Type != bytecode.ValObject {
			return nil, vm.makeError("cannot use '%s' outside of a class method", className)
		}
		
		obj := thisValue.AsObject()
		if className == "self" {
			return obj.Class, nil
		}
		// parent
		if obj.Class.Parent == nil {
			return nil, vm.makeError("class '%s' has no parent class", obj.Class.Name)
		}
		return obj.Class.Parent, nil
	}
	
	// 普通类名
	class, ok := vm.classes[className]
	if !ok {
		return nil, vm.makeError("undefined class '%s'", className)
	}
	return class, nil
}

// makeError 创建错误（不设置 VM 状态）
func (vm *VM) makeError(format string, args ...interface{}) error {
	return fmt.Errorf(format, args...)
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

// invokeDestructor 调用析构函数
func (vm *VM) invokeDestructor(obj *bytecode.Object, method *bytecode.Method) InterpretResult {
	// 创建方法的闭包
	closure := &bytecode.Closure{
		Function: &bytecode.Function{
			Name:       method.Name,
			Arity:      method.Arity,
			Chunk:      method.Chunk,
			LocalCount: method.LocalCount,
		},
	}
	
	// 析构函数没有参数
	return vm.call(closure, 0)
}

// validateClass 验证类的接口实现和抽象类约束
func (vm *VM) validateClass(class *bytecode.Class) error {
	// 检查抽象类不能被实例化
	if class.IsAbstract {
		return vm.makeError("cannot instantiate abstract class '%s'", class.Name)
	}
	
	// 检查接口实现
	for _, ifaceName := range class.Implements {
		iface, ok := vm.classes[ifaceName]
		if !ok {
			return vm.makeError("interface '%s' not found", ifaceName)
		}
		
		// 检查类是否实现了接口的所有方法
		for methodName := range iface.Methods {
			if vm.lookupMethod(class, methodName) == nil {
				return vm.makeError("class '%s' does not implement method '%s' from interface '%s'", 
					class.Name, methodName, ifaceName)
			}
		}
	}
	
	return nil
}

// getCurrentClass 获取当前上下文的类（如果在方法中）
func (vm *VM) getCurrentClass() *bytecode.Class {
	if vm.frameCount == 0 {
		return nil
	}
	frame := &vm.frames[vm.frameCount-1]
	thisValue := vm.stack[frame.BaseSlot]
	if thisValue.Type == bytecode.ValObject {
		return thisValue.AsObject().Class
	}
	return nil
}

// checkPropertyAccess 检查属性访问权限
func (vm *VM) checkPropertyAccess(targetClass *bytecode.Class, propName string) error {
	vis, ok := targetClass.PropVisibility[propName]
	if !ok {
		// 没有可见性信息，默认 public
		return nil
	}
	
	if vis == bytecode.VisPublic {
		return nil
	}
	
	currentClass := vm.getCurrentClass()
	if currentClass == nil {
		// 不在类方法中，只能访问 public
		if vis != bytecode.VisPublic {
			return vm.makeError("cannot access %s property '%s' from outside class", 
				visibilityName(vis), propName)
		}
		return nil
	}
	
	// private: 只能在同一个类中访问
	if vis == bytecode.VisPrivate {
		if currentClass.Name != targetClass.Name {
			return vm.makeError("cannot access private property '%s' of class '%s'", 
				propName, targetClass.Name)
		}
		return nil
	}
	
	// protected: 可以在同一个类或子类中访问
	if vis == bytecode.VisProtected {
		if !vm.isClassOrSubclass(currentClass, targetClass) {
			return vm.makeError("cannot access protected property '%s' of class '%s'", 
				propName, targetClass.Name)
		}
	}
	
	return nil
}

// checkMethodAccess 检查方法访问权限
func (vm *VM) checkMethodAccess(targetClass *bytecode.Class, method *bytecode.Method) error {
	if method.Visibility == bytecode.VisPublic {
		return nil
	}
	
	currentClass := vm.getCurrentClass()
	if currentClass == nil {
		// 不在类方法中，只能访问 public
		return vm.makeError("cannot access %s method '%s' from outside class", 
			visibilityName(method.Visibility), method.Name)
	}
	
	// private: 只能在同一个类中访问
	if method.Visibility == bytecode.VisPrivate {
		if currentClass.Name != targetClass.Name {
			return vm.makeError("cannot access private method '%s' of class '%s'", 
				method.Name, targetClass.Name)
		}
		return nil
	}
	
	// protected: 可以在同一个类或子类中访问
	if method.Visibility == bytecode.VisProtected {
		if !vm.isClassOrSubclass(currentClass, targetClass) {
			return vm.makeError("cannot access protected method '%s' of class '%s'", 
				method.Name, targetClass.Name)
		}
	}
	
	return nil
}

// isClassOrSubclass 检查 current 是否是 target 或其子类
func (vm *VM) isClassOrSubclass(current, target *bytecode.Class) bool {
	for c := current; c != nil; c = c.Parent {
		if c.Name == target.Name {
			return true
		}
	}
	return false
}

// visibilityName 返回可见性名称
func visibilityName(v bytecode.Visibility) string {
	switch v {
	case bytecode.VisPublic:
		return "public"
	case bytecode.VisProtected:
		return "protected"
	case bytecode.VisPrivate:
		return "private"
	default:
		return "unknown"
	}
}

