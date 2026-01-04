package runtime

import (
	"fmt"
	"math"
	"strings"

	"github.com/tangzhangming/nova/internal/ast"
	"github.com/tangzhangming/nova/internal/bytecode"
	"github.com/tangzhangming/nova/internal/compiler"
	"github.com/tangzhangming/nova/internal/parser"
	"github.com/tangzhangming/nova/internal/vm"
)

// Runtime Nova 运行时
type Runtime struct {
	vm       *vm.VM
	builtins map[string]BuiltinFunc
}

// BuiltinFunc 内置函数类型
type BuiltinFunc func(args []bytecode.Value) bytecode.Value

// New 创建运行时
func New() *Runtime {
	r := &Runtime{
		vm:       vm.New(),
		builtins: make(map[string]BuiltinFunc),
	}
	r.registerBuiltins()
	return r
}

// Run 运行源代码
func (r *Runtime) Run(source, filename string) error {
	// 解析
	p := parser.New(source, filename)
	file := p.Parse()

	if p.HasErrors() {
		for _, e := range p.Errors() {
			fmt.Printf("Parse error: %s\n", e)
		}
		return fmt.Errorf("parse failed")
	}

	// 编译
	c := compiler.New()
	fn, errs := c.Compile(file)

	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Printf("Compile error: %s\n", e)
		}
		return fmt.Errorf("compile failed")
	}

	// 注册编译的类
	classes := c.Classes()
	for _, class := range classes {
		r.vm.DefineClass(class)
	}
	
	// 解析父类引用
	for _, class := range classes {
		if class.ParentName != "" && class.Parent == nil {
			if parent, ok := classes[class.ParentName]; ok {
				class.Parent = parent
			}
		}
	}

	// 注册编译的枚举
	enums := c.Enums()
	for _, enum := range enums {
		r.vm.DefineEnum(enum)
	}

	// 注册内置函数
	r.registerBuiltinsToVM()

	// 运行
	result := r.vm.Run(fn)
	if result != vm.InterpretOK {
		return fmt.Errorf("runtime error: %s", r.vm.GetError())
	}

	return nil
}

// RunFile 运行文件
func (r *Runtime) RunFile(filename string) error {
	// 由调用者读取文件内容
	return fmt.Errorf("use Run() with file content instead")
}

// CompileOnly 只编译，不运行
func (r *Runtime) CompileOnly(source, filename string) (*bytecode.Function, error) {
	p := parser.New(source, filename)
	file := p.Parse()

	if p.HasErrors() {
		return nil, fmt.Errorf("parse failed")
	}

	c := compiler.New()
	fn, errs := c.Compile(file)

	if len(errs) > 0 {
		return nil, fmt.Errorf("compile failed")
	}

	return fn, nil
}

// ParseOnly 只解析，返回 AST
func (r *Runtime) ParseOnly(source, filename string) (*ast.File, error) {
	p := parser.New(source, filename)
	file := p.Parse()

	if p.HasErrors() {
		return nil, fmt.Errorf("parse failed")
	}

	return file, nil
}

// Disassemble 反汇编
func (r *Runtime) Disassemble(source, filename string) (string, error) {
	// 解析
	p := parser.New(source, filename)
	file := p.Parse()
	if p.HasErrors() {
		return "", fmt.Errorf("parse failed")
	}

	// 编译
	c := compiler.New()
	fn, errs := c.Compile(file)
	if len(errs) > 0 {
		return "", fmt.Errorf("compile failed")
	}
	
	result := fn.Chunk.Disassemble(filename)
	
	// 也输出类方法的字节码
	for _, class := range c.Classes() {
		result += fmt.Sprintf("\n=== Class %s ===\n", class.Name)
		for name, methods := range class.Methods {
			for i, method := range methods {
				suffix := ""
				if len(methods) > 1 {
					suffix = fmt.Sprintf("#%d", i)
				}
				result += fmt.Sprintf("\n-- Method %s%s (arity=%d, locals=%d) --\n", name, suffix, method.Arity, method.LocalCount)
				if method.Chunk != nil {
					result += method.Chunk.Disassemble(name)
				}
			}
		}
	}
	
	return result, nil
}

// ============================================================================
// 内置函数
// ============================================================================

func (r *Runtime) registerBuiltins() {
	// 输出函数
	r.builtins["print"] = builtinPrint
	r.builtins["print_r"] = builtinPrintR
	r.builtins["echo"] = builtinPrint

	// 类型函数
	r.builtins["typeof"] = builtinTypeof
	r.builtins["is_null"] = builtinIsNull
	r.builtins["is_bool"] = builtinIsBool
	r.builtins["is_int"] = builtinIsInt
	r.builtins["is_float"] = builtinIsFloat
	r.builtins["is_string"] = builtinIsString
	r.builtins["is_array"] = builtinIsArray
	r.builtins["is_map"] = builtinIsMap
	r.builtins["is_object"] = builtinIsObject

	// 转换函数
	r.builtins["int"] = builtinToInt
	r.builtins["float"] = builtinToFloat
	r.builtins["string"] = builtinToString
	r.builtins["bool"] = builtinToBool

	// 反射/注解函数
	r.builtins["get_class"] = builtinGetClass
	r.builtins["get_class_annotations"] = func(args []bytecode.Value) bytecode.Value {
		return r.getClassAnnotations(args)
	}
	r.builtins["get_method_annotations"] = func(args []bytecode.Value) bytecode.Value {
		return r.getMethodAnnotations(args)
	}
	r.builtins["has_annotation"] = func(args []bytecode.Value) bytecode.Value {
		return r.hasAnnotation(args)
	}

	// 数组函数
	r.builtins["len"] = builtinLen
	r.builtins["push"] = builtinPush
	r.builtins["pop"] = builtinPop
	r.builtins["shift"] = builtinShift
	r.builtins["unshift"] = builtinUnshift
	r.builtins["slice"] = builtinSlice
	r.builtins["concat"] = builtinConcat
	r.builtins["reverse"] = builtinReverse
	r.builtins["contains"] = builtinContains
	r.builtins["index_of"] = builtinIndexOf

	// 字符串函数
	r.builtins["strlen"] = builtinStrlen
	r.builtins["substr"] = builtinSubstr
	r.builtins["str_upper"] = builtinStrUpper
	r.builtins["str_lower"] = builtinStrLower
	r.builtins["str_trim"] = builtinStrTrim
	r.builtins["str_split"] = builtinStrSplit
	r.builtins["str_join"] = builtinStrJoin
	r.builtins["str_replace"] = builtinStrReplace
	r.builtins["str_contains"] = builtinStrContains
	r.builtins["str_starts_with"] = builtinStrStartsWith
	r.builtins["str_ends_with"] = builtinStrEndsWith

	// 数学函数
	r.builtins["abs"] = builtinAbs
	r.builtins["min"] = builtinMin
	r.builtins["max"] = builtinMax
	r.builtins["floor"] = builtinFloor
	r.builtins["ceil"] = builtinCeil
	r.builtins["round"] = builtinRound
}

func (r *Runtime) registerBuiltinsToVM() {
	for name, fn := range r.builtins {
		// 创建一个包装函数
		wrapper := createBuiltinWrapper(fn)
		r.vm.DefineGlobal(name, bytecode.NewFunc(wrapper))
	}
}

func createBuiltinWrapper(fn BuiltinFunc) *bytecode.Function {
	// 内置函数使用特殊标记
	f := bytecode.NewFunction("<builtin>")
	f.Arity = 255       // 最大参数数量
	f.MinArity = 0      // 最小参数数量
	f.IsVariadic = true // 标记为可变参数
	f.IsBuiltin = true  // 标记为内置函数
	f.BuiltinFn = bytecode.BuiltinFn(fn) // 保存内置函数实现
	return f
}

// ============================================================================
// 内置函数实现
// ============================================================================

func builtinPrint(args []bytecode.Value) bytecode.Value {
	for i, arg := range args {
		if i > 0 {
			fmt.Print(" ")
		}
		fmt.Print(arg.String())
	}
	fmt.Println()
	return bytecode.NullValue
}

func builtinPrintR(args []bytecode.Value) bytecode.Value {
	for _, arg := range args {
		fmt.Printf("%v: %s\n", arg.Type, arg.String())
	}
	return bytecode.NullValue
}

func builtinTypeof(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewString("null")
	}
	switch args[0].Type {
	case bytecode.ValNull:
		return bytecode.NewString("null")
	case bytecode.ValBool:
		return bytecode.NewString("bool")
	case bytecode.ValInt:
		return bytecode.NewString("int")
	case bytecode.ValFloat:
		return bytecode.NewString("float")
	case bytecode.ValString:
		return bytecode.NewString("string")
	case bytecode.ValArray:
		return bytecode.NewString("array")
	case bytecode.ValMap:
		return bytecode.NewString("map")
	case bytecode.ValObject:
		return bytecode.NewString("object")
	case bytecode.ValFunc, bytecode.ValClosure:
		return bytecode.NewString("function")
	default:
		return bytecode.NewString("unknown")
	}
}

func builtinIsNull(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.TrueValue
	}
	return bytecode.NewBool(args[0].Type == bytecode.ValNull)
}

func builtinIsBool(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	return bytecode.NewBool(args[0].Type == bytecode.ValBool)
}

func builtinIsInt(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	return bytecode.NewBool(args[0].Type == bytecode.ValInt)
}

func builtinIsFloat(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	return bytecode.NewBool(args[0].Type == bytecode.ValFloat)
}

func builtinIsString(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	return bytecode.NewBool(args[0].Type == bytecode.ValString)
}

func builtinIsArray(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	return bytecode.NewBool(args[0].Type == bytecode.ValArray)
}

func builtinIsMap(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	return bytecode.NewBool(args[0].Type == bytecode.ValMap)
}

func builtinIsObject(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	return bytecode.NewBool(args[0].Type == bytecode.ValObject)
}

func builtinToInt(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.ZeroValue
	}
	return bytecode.NewInt(args[0].AsInt())
}

func builtinToFloat(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewFloat(0)
	}
	return bytecode.NewFloat(args[0].AsFloat())
}

func builtinToString(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewString("")
	}
	return bytecode.NewString(args[0].String())
}

func builtinToBool(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	return bytecode.NewBool(args[0].IsTruthy())
}

func builtinLen(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.ZeroValue
	}
	v := args[0]
	switch v.Type {
	case bytecode.ValString:
		return bytecode.NewInt(int64(len(v.AsString())))
	case bytecode.ValArray:
		return bytecode.NewInt(int64(len(v.AsArray())))
	case bytecode.ValMap:
		return bytecode.NewInt(int64(len(v.AsMap())))
	default:
		return bytecode.ZeroValue
	}
}

func builtinPush(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 || args[0].Type != bytecode.ValArray {
		return bytecode.NullValue
	}
	arr := args[0].AsArray()
	arr = append(arr, args[1:]...)
	return bytecode.NewArray(arr)
}

func builtinPop(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 || args[0].Type != bytecode.ValArray {
		return bytecode.NullValue
	}
	arr := args[0].AsArray()
	if len(arr) == 0 {
		return bytecode.NullValue
	}
	return arr[len(arr)-1]
}

func builtinShift(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 || args[0].Type != bytecode.ValArray {
		return bytecode.NullValue
	}
	arr := args[0].AsArray()
	if len(arr) == 0 {
		return bytecode.NullValue
	}
	return arr[0]
}

func builtinUnshift(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 || args[0].Type != bytecode.ValArray {
		return bytecode.NullValue
	}
	arr := args[0].AsArray()
	newArr := make([]bytecode.Value, len(args)-1+len(arr))
	copy(newArr, args[1:])
	copy(newArr[len(args)-1:], arr)
	return bytecode.NewArray(newArr)
}

func builtinSlice(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 || args[0].Type != bytecode.ValArray {
		return bytecode.NullValue
	}
	arr := args[0].AsArray()
	start := int(args[1].AsInt())
	end := len(arr)
	if len(args) > 2 {
		end = int(args[2].AsInt())
	}
	if start < 0 {
		start = 0
	}
	if end > len(arr) {
		end = len(arr)
	}
	if start >= end {
		return bytecode.NewArray([]bytecode.Value{})
	}
	return bytecode.NewArray(arr[start:end])
}

func builtinConcat(args []bytecode.Value) bytecode.Value {
	var result []bytecode.Value
	for _, arg := range args {
		if arg.Type == bytecode.ValArray {
			result = append(result, arg.AsArray()...)
		}
	}
	return bytecode.NewArray(result)
}

func builtinReverse(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 || args[0].Type != bytecode.ValArray {
		return bytecode.NullValue
	}
	arr := args[0].AsArray()
	result := make([]bytecode.Value, len(arr))
	for i, v := range arr {
		result[len(arr)-1-i] = v
	}
	return bytecode.NewArray(result)
}

func builtinContains(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 || args[0].Type != bytecode.ValArray {
		return bytecode.FalseValue
	}
	arr := args[0].AsArray()
	target := args[1]
	for _, v := range arr {
		if v.Equals(target) {
			return bytecode.TrueValue
		}
	}
	return bytecode.FalseValue
}

func builtinIndexOf(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 || args[0].Type != bytecode.ValArray {
		return bytecode.NewInt(-1)
	}
	arr := args[0].AsArray()
	target := args[1]
	for i, v := range arr {
		if v.Equals(target) {
			return bytecode.NewInt(int64(i))
		}
	}
	return bytecode.NewInt(-1)
}

// 字符串函数

func builtinStrlen(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.ZeroValue
	}
	return bytecode.NewInt(int64(len(args[0].AsString())))
}

func builtinSubstr(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewString("")
	}
	s := args[0].AsString()
	start := int(args[1].AsInt())
	length := len(s) - start
	if len(args) > 2 {
		length = int(args[2].AsInt())
	}
	if start < 0 {
		start = 0
	}
	if start >= len(s) {
		return bytecode.NewString("")
	}
	end := start + length
	if end > len(s) {
		end = len(s)
	}
	return bytecode.NewString(s[start:end])
}

func builtinStrUpper(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewString("")
	}
	return bytecode.NewString(strings.ToUpper(args[0].AsString()))
}

func builtinStrLower(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewString("")
	}
	return bytecode.NewString(strings.ToLower(args[0].AsString()))
}

func builtinStrTrim(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewString("")
	}
	return bytecode.NewString(strings.TrimSpace(args[0].AsString()))
}

func builtinStrSplit(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewArray([]bytecode.Value{})
	}
	s := args[0].AsString()
	sep := args[1].AsString()
	parts := strings.Split(s, sep)
	result := make([]bytecode.Value, len(parts))
	for i, p := range parts {
		result[i] = bytecode.NewString(p)
	}
	return bytecode.NewArray(result)
}

func builtinStrJoin(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 || args[0].Type != bytecode.ValArray {
		return bytecode.NewString("")
	}
	arr := args[0].AsArray()
	sep := args[1].AsString()
	parts := make([]string, len(arr))
	for i, v := range arr {
		parts[i] = v.AsString()
	}
	return bytecode.NewString(strings.Join(parts, sep))
}

func builtinStrReplace(args []bytecode.Value) bytecode.Value {
	if len(args) < 3 {
		return bytecode.NewString("")
	}
	s := args[0].AsString()
	old := args[1].AsString()
	new := args[2].AsString()
	return bytecode.NewString(strings.ReplaceAll(s, old, new))
}

func builtinStrContains(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.FalseValue
	}
	s := args[0].AsString()
	sub := args[1].AsString()
	return bytecode.NewBool(strings.Contains(s, sub))
}

func builtinStrStartsWith(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.FalseValue
	}
	s := args[0].AsString()
	prefix := args[1].AsString()
	return bytecode.NewBool(strings.HasPrefix(s, prefix))
}

func builtinStrEndsWith(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.FalseValue
	}
	s := args[0].AsString()
	suffix := args[1].AsString()
	return bytecode.NewBool(strings.HasSuffix(s, suffix))
}

// 数学函数

func builtinAbs(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.ZeroValue
	}
	v := args[0]
	switch v.Type {
	case bytecode.ValInt:
		n := v.AsInt()
		if n < 0 {
			return bytecode.NewInt(-n)
		}
		return v
	case bytecode.ValFloat:
		f := v.AsFloat()
		if f < 0 {
			return bytecode.NewFloat(-f)
		}
		return v
	default:
		return bytecode.ZeroValue
	}
}

func builtinMin(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NullValue
	}
	min := args[0]
	for _, v := range args[1:] {
		if v.AsFloat() < min.AsFloat() {
			min = v
		}
	}
	return min
}

func builtinMax(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NullValue
	}
	max := args[0]
	for _, v := range args[1:] {
		if v.AsFloat() > max.AsFloat() {
			max = v
		}
	}
	return max
}

func builtinFloor(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.ZeroValue
	}
	return bytecode.NewInt(int64(math.Floor(args[0].AsFloat())))
}

func builtinCeil(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.ZeroValue
	}
	return bytecode.NewInt(int64(math.Ceil(args[0].AsFloat())))
}

func builtinRound(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.ZeroValue
	}
	return bytecode.NewInt(int64(math.Round(args[0].AsFloat())))
}

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

