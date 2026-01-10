package runtime

import (
	"github.com/tangzhangming/nova/internal/bytecode"
)

// ============================================================================
// 反射/注解函数
// ============================================================================

// get_class(object) - 获取对象的类名
func builtinGetClass(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 || args[0].Type != bytecode.ValObject {
		return bytecode.NullValue
	}
	obj := args[0].AsObject()
	return bytecode.NewString(obj.Class.Name)
}

// get_class_annotations(className) - 获取类的注解
func (r *Runtime) getClassAnnotations(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewArray(nil)
	}

	var className string
	if args[0].Type == bytecode.ValString {
		className = args[0].AsString()
	} else if args[0].Type == bytecode.ValObject {
		className = args[0].AsObject().Class.Name
	} else {
		return bytecode.NewArray(nil)
	}

	class := r.vm.GetClass(className)
	if class == nil {
		return bytecode.NewArray(nil)
	}

	return annotationsToArray(class.Annotations)
}

// get_method_annotations(className, methodName) - 获取方法的注解
func (r *Runtime) getMethodAnnotations(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewArray(nil)
	}

	var className string
	if args[0].Type == bytecode.ValString {
		className = args[0].AsString()
	} else if args[0].Type == bytecode.ValObject {
		className = args[0].AsObject().Class.Name
	} else {
		return bytecode.NewArray(nil)
	}

	methodName := args[1].AsString()

	class := r.vm.GetClass(className)
	if class == nil {
		return bytecode.NewArray(nil)
	}

	method := class.GetMethod(methodName)
	if method == nil {
		return bytecode.NewArray(nil)
	}

	return annotationsToArray(method.Annotations)
}

// has_annotation(className, annotationName) - 检查类是否有指定注解
func (r *Runtime) hasAnnotation(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.FalseValue
	}

	var className string
	if args[0].Type == bytecode.ValString {
		className = args[0].AsString()
	} else if args[0].Type == bytecode.ValObject {
		className = args[0].AsObject().Class.Name
	} else {
		return bytecode.FalseValue
	}

	annotationName := args[1].AsString()

	class := r.vm.GetClass(className)
	if class == nil {
		return bytecode.FalseValue
	}

	for _, ann := range class.Annotations {
		if ann.Name == annotationName {
			return bytecode.TrueValue
		}
	}

	return bytecode.FalseValue
}

// annotationsToArray 将注解列表转换为数组
func annotationsToArray(annotations []*bytecode.Annotation) bytecode.Value {
	if len(annotations) == 0 {
		return bytecode.NewArray(nil)
	}

	result := make([]bytecode.Value, len(annotations))
	for i, ann := range annotations {
		// 每个注解转换为 map: {name: "...", args: [...]}
		annMap := make(map[bytecode.Value]bytecode.Value)
		annMap[bytecode.NewString("name")] = bytecode.NewString(ann.Name)
		annMap[bytecode.NewString("args")] = bytecode.NewArray(ann.Args)
		result[i] = bytecode.NewMap(annMap)
	}

	return bytecode.NewArray(result)
}

// ============================================================================
// ORM 反射扩展函数
// ============================================================================

// native_reflect_set_property(object, propName, value) - 动态设置对象属性值
func builtinReflectSetProperty(args []bytecode.Value) bytecode.Value {
	if len(args) < 3 {
		return bytecode.FalseValue
	}

	// 第一个参数必须是对象
	if args[0].Type != bytecode.ValObject {
		return bytecode.FalseValue
	}

	obj := args[0].AsObject()

	// 第二个参数必须是字符串（属性名）
	if args[1].Type != bytecode.ValString {
		return bytecode.FalseValue
	}

	propName := args[1].AsString()
	value := args[2]

	// 设置属性值
	obj.SetField(propName, value)
	return bytecode.TrueValue
}

// native_reflect_get_property(object, propName) - 动态获取对象属性值
func builtinReflectGetProperty(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NullValue
	}

	// 第一个参数必须是对象
	if args[0].Type != bytecode.ValObject {
		return bytecode.NullValue
	}

	obj := args[0].AsObject()

	// 第二个参数必须是字符串（属性名）
	if args[1].Type != bytecode.ValString {
		return bytecode.NullValue
	}

	propName := args[1].AsString()

	// 获取属性值
	if value, ok := obj.GetField(propName); ok {
		return value
	}

	return bytecode.NullValue
}

// native_reflect_new_instance(className) - 根据类名动态创建实例
func (r *Runtime) reflectNewInstance(args []bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.NullValue
	}

	// 参数必须是字符串（类名）
	if args[0].Type != bytecode.ValString {
		return bytecode.NullValue
	}

	className := args[0].AsString()

	// 查找类定义
	class := r.vm.GetClass(className)
	if class == nil {
		// 尝试其他命名空间格式
		// 例如：User -> sola.database.orm.User
		return bytecode.NullValue
	}

	// 创建对象实例
	obj := bytecode.NewObjectInstance(class)

	// 初始化属性默认值
	for propName, defaultVal := range class.Properties {
		obj.Fields[propName] = defaultVal
	}

	return bytecode.NewObject(obj)
}

// native_reflect_get_property_annotations(object, propName) - 获取属性的注解列表
func (r *Runtime) reflectGetPropertyAnnotations(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewArray(nil)
	}

	var className string
	if args[0].Type == bytecode.ValString {
		className = args[0].AsString()
	} else if args[0].Type == bytecode.ValObject {
		className = args[0].AsObject().Class.Name
	} else {
		return bytecode.NewArray(nil)
	}

	// 第二个参数必须是字符串（属性名）
	if args[1].Type != bytecode.ValString {
		return bytecode.NewArray(nil)
	}

	propName := args[1].AsString()

	// 查找类定义
	class := r.vm.GetClass(className)
	if class == nil {
		return bytecode.NewArray(nil)
	}

	// 获取属性注解
	if annotations, ok := class.PropAnnotations[propName]; ok {
		return annotationsToArray(annotations)
	}

	return bytecode.NewArray(nil)
}

// native_reflect_get_properties(object|className) - 获取对象/类的所有属性名
// 支持传入对象实例或类名字符串
func (r *Runtime) reflectGetProperties(args []bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.NewArray(nil)
	}

	var class *bytecode.Class
	if args[0].Type == bytecode.ValObject {
		class = args[0].AsObject().Class
	} else if args[0].Type == bytecode.ValString {
		// 支持通过类名字符串查找
		className := args[0].AsString()
		class = r.vm.GetClass(className)
		if class == nil {
			return bytecode.NewArray(nil)
		}
	} else {
		return bytecode.NewArray(nil)
	}

	// 获取所有属性名
	props := make([]bytecode.Value, 0, len(class.Properties))
	for propName := range class.Properties {
		props = append(props, bytecode.NewString(propName))
	}

	return bytecode.NewArray(props)
}











