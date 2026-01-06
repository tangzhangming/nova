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



