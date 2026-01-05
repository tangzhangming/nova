package runtime

import (
	"bufio"
	"fmt"
	"math"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tangzhangming/nova/internal/ast"
	"github.com/tangzhangming/nova/internal/bytecode"
	"github.com/tangzhangming/nova/internal/compiler"
	"github.com/tangzhangming/nova/internal/i18n"
	"github.com/tangzhangming/nova/internal/loader"
	"github.com/tangzhangming/nova/internal/parser"
	"github.com/tangzhangming/nova/internal/vm"
)

// ============================================================================
// TCP 连接池管理 (用于 native_tcp_* 函数)
// ============================================================================

type tcpConnection struct {
	conn   net.Conn
	reader *bufio.Reader
}

var (
	tcpConnections = make(map[int64]*tcpConnection)
	tcpConnMutex   sync.RWMutex
	nextConnID     int64 = 1
)

// Runtime Sola 运行时
type Runtime struct {
	vm       *vm.VM
	builtins map[string]BuiltinFunc
	loader   *loader.Loader
	classes  map[string]*bytecode.Class
	enums    map[string]*bytecode.Enum
}

// BuiltinFunc 内置函数类型
type BuiltinFunc func(args []bytecode.Value) bytecode.Value

// New 创建运行时
func New() *Runtime {
	r := &Runtime{
		vm:       vm.New(),
		builtins: make(map[string]BuiltinFunc),
		classes:  make(map[string]*bytecode.Class),
		enums:    make(map[string]*bytecode.Enum),
	}
	r.registerBuiltins()
	// 异常类现在通过 lib/lang/*.sola 文件定义，不再在这里内置
	return r
}

// Run 运行源代码
func (r *Runtime) Run(source, filename string) error {
	// 创建加载器
	var err error
	r.loader, err = loader.New(filename)
	if err != nil {
		return fmt.Errorf(i18n.T(i18n.ErrFailedCreateLoader, err))
	}

	// 解析入口文件
	p := parser.New(source, filename)
	file := p.Parse()

	if p.HasErrors() {
		for _, e := range p.Errors() {
			fmt.Printf(i18n.T(i18n.ErrParseError, e) + "\n")
		}
		return fmt.Errorf(i18n.T(i18n.ErrParseFailed))
	}

	// 处理 use 声明，加载依赖
	for _, use := range file.Uses {
		if err := r.loadDependency(use.Path); err != nil {
			return fmt.Errorf(i18n.T(i18n.ErrLoadFailed, use.Path, err))
		}
	}

	// 编译入口文件
	c := compiler.New()
	fn, errs := c.Compile(file)

	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Printf(i18n.T(i18n.ErrCompileError, e) + "\n")
		}
		return fmt.Errorf(i18n.T(i18n.ErrCompileFailed))
	}

	// 注册编译的类
	classes := c.Classes()
	for name, class := range classes {
		r.classes[name] = class
		r.vm.DefineClass(class)
	}
	
	// 解析父类引用（包括导入的类）
	for _, class := range r.classes {
		if class.ParentName != "" && class.Parent == nil {
			if parent, ok := r.classes[class.ParentName]; ok {
				class.Parent = parent
			}
		}
	}

	// 注册编译的枚举
	enums := c.Enums()
	for name, enum := range enums {
		r.enums[name] = enum
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

// loadDependency 加载依赖
func (r *Runtime) loadDependency(importPath string) error {
	// 解析导入路径
	filePath, err := r.loader.ResolveImport(importPath)
	if err != nil {
		return err
	}
	
	// 检查是否已加载
	if r.loader.IsLoaded(filePath) {
		return nil
	}
	r.loader.MarkLoaded(filePath)
	
	// 加载文件内容
	source, err := r.loader.LoadFile(filePath)
	if err != nil {
		return fmt.Errorf(i18n.T(i18n.ErrReadFailed, filePath, err))
	}
	
	// 解析
	p := parser.New(source, filepath.Base(filePath))
	file := p.Parse()
	if p.HasErrors() {
		for _, e := range p.Errors() {
			fmt.Printf(i18n.T(i18n.ErrParseError, e) + "\n")
		}
		return fmt.Errorf(i18n.T(i18n.ErrParseFailedFor, importPath))
	}
	
	// 递归加载依赖
	for _, use := range file.Uses {
		if err := r.loadDependency(use.Path); err != nil {
			return err
		}
	}
	
	// 编译
	c := compiler.New()
	_, errs := c.Compile(file)
	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Printf(i18n.T(i18n.ErrCompileError, e) + "\n")
		}
		return fmt.Errorf(i18n.T(i18n.ErrCompileFailedFor, importPath))
	}
	
	// 注册类
	for name, class := range c.Classes() {
		// 使用完整路径作为类名（命名空间.类名）
		fullName := name
		if file.Namespace != nil {
			fullName = file.Namespace.Name + "." + name
		}
		r.classes[fullName] = class
		r.classes[name] = class // 也用短名注册
		r.vm.DefineClass(class)
	}
	
	// 注册枚举
	for name, enum := range c.Enums() {
		r.enums[name] = enum
		r.vm.DefineEnum(enum)
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
		return nil, fmt.Errorf(i18n.T(i18n.ErrParseFailed))
	}

	c := compiler.New()
	fn, errs := c.Compile(file)

	if len(errs) > 0 {
		return nil, fmt.Errorf(i18n.T(i18n.ErrCompileFailed))
	}

	return fn, nil
}

// ParseOnly 只解析，返回 AST
func (r *Runtime) ParseOnly(source, filename string) (*ast.File, error) {
	p := parser.New(source, filename)
	file := p.Parse()

	if p.HasErrors() {
		return nil, fmt.Errorf(i18n.T(i18n.ErrParseFailed))
	}

	return file, nil
}

// Disassemble 反汇编
func (r *Runtime) Disassemble(source, filename string) (string, error) {
	// 解析
	p := parser.New(source, filename)
	file := p.Parse()
	if p.HasErrors() {
		return "", fmt.Errorf(i18n.T(i18n.ErrParseFailed))
	}

	// 编译
	c := compiler.New()
	fn, errs := c.Compile(file)
	if len(errs) > 0 {
		return "", fmt.Errorf(i18n.T(i18n.ErrCompileFailed))
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

	// 转换函数 (使用 to_ 前缀避免与类型关键字冲突)
	r.builtins["to_int"] = builtinToInt
	r.builtins["to_float"] = builtinToFloat
	r.builtins["to_string"] = builtinToString
	r.builtins["to_bool"] = builtinToBool

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

	// Native 数学函数 (仅供标准库使用)
	r.builtins["native_math_abs"] = nativeMathAbs
	r.builtins["native_math_min"] = nativeMathMin
	r.builtins["native_math_max"] = nativeMathMax
	r.builtins["native_math_floor"] = nativeMathFloor
	r.builtins["native_math_ceil"] = nativeMathCeil
	r.builtins["native_math_round"] = nativeMathRound

	// Native 字符串函数 (仅供标准库使用)
	r.builtins["native_str_len"] = nativeStrLen
	r.builtins["native_str_substring"] = nativeStrSubstring
	r.builtins["native_str_to_upper"] = nativeStrToUpper
	r.builtins["native_str_to_lower"] = nativeStrToLower
	r.builtins["native_str_trim"] = nativeStrTrim
	r.builtins["native_str_replace"] = nativeStrReplace
	r.builtins["native_str_split"] = nativeStrSplit
	r.builtins["native_str_join"] = nativeStrJoin
	r.builtins["native_str_index_of"] = nativeStrIndexOf
	r.builtins["native_str_last_index_of"] = nativeStrLastIndexOf
	r.builtins["native_str_to_int"] = nativeStrToInt
	r.builtins["native_str_to_float"] = nativeStrToFloat

	// Native TCP 函数 (仅供标准库使用)
	r.builtins["native_tcp_connect"] = nativeTcpConnect
	r.builtins["native_tcp_write"] = nativeTcpWrite
	r.builtins["native_tcp_read"] = nativeTcpRead
	r.builtins["native_tcp_read_line"] = nativeTcpReadLine
	r.builtins["native_tcp_close"] = nativeTcpClose
	r.builtins["native_tcp_set_timeout"] = nativeTcpSetTimeout

	// 测试用：抛出原生异常的函数
	r.builtins["native_throw"] = func(args []bytecode.Value) bytecode.Value {
		msg := "Native exception"
		if len(args) > 0 {
			msg = args[0].AsString()
		}
		return bytecode.NewException("NativeException", msg, 0)
	}
	
	// 测试用：会 panic 的函数 (用于测试 panic/recover)
	r.builtins["native_panic"] = func(args []bytecode.Value) bytecode.Value {
		msg := "Native panic"
		if len(args) > 0 {
			msg = args[0].AsString()
		}
		panic(msg)
	}

	// GC 控制函数
	r.builtins["gc_collect"] = func(args []bytecode.Value) bytecode.Value {
		freed := r.vm.CollectGarbage()
		return bytecode.NewInt(int64(freed))
	}
	r.builtins["gc_enable"] = func(args []bytecode.Value) bytecode.Value {
		r.vm.SetGCEnabled(true)
		return bytecode.NullValue
	}
	r.builtins["gc_disable"] = func(args []bytecode.Value) bytecode.Value {
		r.vm.SetGCEnabled(false)
		return bytecode.NullValue
	}
	r.builtins["gc_stats"] = func(args []bytecode.Value) bytecode.Value {
		stats := r.vm.GetGC().Stats()
		m := make(map[bytecode.Value]bytecode.Value)
		m[bytecode.NewString("heap_size")] = bytecode.NewInt(int64(stats.HeapSize))
		m[bytecode.NewString("total_allocations")] = bytecode.NewInt(stats.TotalAllocations)
		m[bytecode.NewString("total_collections")] = bytecode.NewInt(stats.TotalCollections)
		m[bytecode.NewString("total_freed")] = bytecode.NewInt(stats.TotalFreed)
		m[bytecode.NewString("next_threshold")] = bytecode.NewInt(int64(stats.NextThreshold))
		return bytecode.NewMap(m)
	}
	r.builtins["gc_set_threshold"] = func(args []bytecode.Value) bytecode.Value {
		if len(args) > 0 {
			threshold := int(args[0].AsInt())
			if threshold > 0 {
				r.vm.SetGCThreshold(threshold)
			}
		}
		return bytecode.NullValue
	}
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
	v := args[0]
	if v.Type == bytecode.ValString {
		s := strings.TrimSpace(v.AsString())
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return bytecode.ZeroValue
		}
		return bytecode.NewInt(n)
	}
	return bytecode.NewInt(v.AsInt())
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

// Native 数学函数 (仅供标准库使用)

func nativeMathAbs(args []bytecode.Value) bytecode.Value {
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

func nativeMathMin(args []bytecode.Value) bytecode.Value {
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

func nativeMathMax(args []bytecode.Value) bytecode.Value {
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

func nativeMathFloor(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.ZeroValue
	}
	return bytecode.NewInt(int64(math.Floor(args[0].AsFloat())))
}

func nativeMathCeil(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.ZeroValue
	}
	return bytecode.NewInt(int64(math.Ceil(args[0].AsFloat())))
}

func nativeMathRound(args []bytecode.Value) bytecode.Value {
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

// ============================================================================
// Native TCP 函数实现 (仅供标准库使用)
// ============================================================================

func nativeTcpConnect(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewInt(-1)
	}
	host := args[0].AsString()
	port := args[1].AsInt()
	address := fmt.Sprintf("%s:%d", host, port)
	conn, err := net.DialTimeout("tcp", address, 10*time.Second)
	if err != nil {
		return bytecode.NewInt(-1)
	}
	tcpConnMutex.Lock()
	connID := nextConnID
	nextConnID++
	tcpConnections[connID] = &tcpConnection{conn: conn, reader: bufio.NewReader(conn)}
	tcpConnMutex.Unlock()
	return bytecode.NewInt(connID)
}

func nativeTcpWrite(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewInt(-1)
	}
	connID := args[0].AsInt()
	data := args[1].AsString()
	tcpConnMutex.RLock()
	tc, ok := tcpConnections[connID]
	tcpConnMutex.RUnlock()
	if !ok {
		return bytecode.NewInt(-1)
	}
	n, err := tc.conn.Write([]byte(data))
	if err != nil {
		return bytecode.NewInt(-1)
	}
	return bytecode.NewInt(int64(n))
}

func nativeTcpRead(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewString("")
	}
	connID := args[0].AsInt()
	length := int(args[1].AsInt())
	tcpConnMutex.RLock()
	tc, ok := tcpConnections[connID]
	tcpConnMutex.RUnlock()
	if !ok {
		return bytecode.NewString("")
	}
	buf := make([]byte, length)
	n, err := tc.reader.Read(buf)
	if err != nil {
		return bytecode.NewString("")
	}
	return bytecode.NewString(string(buf[:n]))
}

func nativeTcpReadLine(args []bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.NewString("")
	}
	connID := args[0].AsInt()
	tcpConnMutex.RLock()
	tc, ok := tcpConnections[connID]
	tcpConnMutex.RUnlock()
	if !ok {
		return bytecode.NewString("")
	}
	line, err := tc.reader.ReadString('\n')
	if err != nil && len(line) == 0 {
		return bytecode.NewString("")
	}
	return bytecode.NewString(line)
}

func nativeTcpClose(args []bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.FalseValue
	}
	connID := args[0].AsInt()
	tcpConnMutex.Lock()
	tc, ok := tcpConnections[connID]
	if ok {
		tc.conn.Close()
		delete(tcpConnections, connID)
	}
	tcpConnMutex.Unlock()
	return bytecode.NewBool(ok)
}

func nativeTcpSetTimeout(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.FalseValue
	}
	connID := args[0].AsInt()
	seconds := args[1].AsInt()
	tcpConnMutex.RLock()
	tc, ok := tcpConnections[connID]
	tcpConnMutex.RUnlock()
	if !ok {
		return bytecode.FalseValue
	}
	deadline := time.Now().Add(time.Duration(seconds) * time.Second)
	err := tc.conn.SetDeadline(deadline)
	return bytecode.NewBool(err == nil)
}

// ============================================================================
// Native 字符串函数 (仅供标准库使用)
// ============================================================================

// nativeStrLen 获取字符串长度
func nativeStrLen(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.ZeroValue
	}
	return bytecode.NewInt(int64(len(args[0].AsString())))
}

// nativeStrSubstring 截取子串
// 参数：str, start, length(-1表示截取到末尾)
func nativeStrSubstring(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewString("")
	}
	s := args[0].AsString()
	start := int(args[1].AsInt())
	length := -1
	if len(args) > 2 {
		length = int(args[2].AsInt())
	}

	// 边界处理
	if start < 0 {
		start = 0
	}
	if start >= len(s) {
		return bytecode.NewString("")
	}

	// 计算结束位置
	var end int
	if length < 0 {
		end = len(s)
	} else {
		end = start + length
		if end > len(s) {
			end = len(s)
		}
	}

	return bytecode.NewString(s[start:end])
}

// nativeStrToUpper 转大写
func nativeStrToUpper(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewString("")
	}
	return bytecode.NewString(strings.ToUpper(args[0].AsString()))
}

// nativeStrToLower 转小写
func nativeStrToLower(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewString("")
	}
	return bytecode.NewString(strings.ToLower(args[0].AsString()))
}

// nativeStrTrim 去除首尾空白
func nativeStrTrim(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewString("")
	}
	return bytecode.NewString(strings.TrimSpace(args[0].AsString()))
}

// nativeStrReplace 替换字符串
// 参数：str, old, new
func nativeStrReplace(args []bytecode.Value) bytecode.Value {
	if len(args) < 3 {
		if len(args) > 0 {
			return args[0]
		}
		return bytecode.NewString("")
	}
	s := args[0].AsString()
	old := args[1].AsString()
	newStr := args[2].AsString()
	return bytecode.NewString(strings.ReplaceAll(s, old, newStr))
}

// nativeStrSplit 分割字符串
// 参数：str, delimiter
func nativeStrSplit(args []bytecode.Value) bytecode.Value {
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

// nativeStrJoin 连接数组
// 参数：arr, delimiter
func nativeStrJoin(args []bytecode.Value) bytecode.Value {
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

// nativeStrIndexOf 查找子串位置
// 参数：str, substr, fromIndex(可选，默认0)
func nativeStrIndexOf(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewInt(-1)
	}
	s := args[0].AsString()
	substr := args[1].AsString()
	fromIndex := 0
	if len(args) > 2 {
		fromIndex = int(args[2].AsInt())
	}

	// 边界处理
	if fromIndex < 0 {
		fromIndex = 0
	}
	if fromIndex >= len(s) {
		return bytecode.NewInt(-1)
	}

	// 从 fromIndex 开始查找
	idx := strings.Index(s[fromIndex:], substr)
	if idx == -1 {
		return bytecode.NewInt(-1)
	}
	return bytecode.NewInt(int64(fromIndex + idx))
}

// nativeStrLastIndexOf 从后往前查找
// 参数：str, substr
func nativeStrLastIndexOf(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewInt(-1)
	}
	s := args[0].AsString()
	substr := args[1].AsString()
	return bytecode.NewInt(int64(strings.LastIndex(s, substr)))
}

// nativeStrToInt 字符串转整数
func nativeStrToInt(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.ZeroValue
	}
	s := strings.TrimSpace(args[0].AsString())
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return bytecode.ZeroValue
	}
	return bytecode.NewInt(n)
}

// nativeStrToFloat 字符串转浮点数
func nativeStrToFloat(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewFloat(0)
	}
	s := strings.TrimSpace(args[0].AsString())
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return bytecode.NewFloat(0)
	}
	return bytecode.NewFloat(f)
}

// 异常类现在通过 lib/lang/*.sola 文件定义：
//   sola.lang.Throwable        // 所有错误和异常的基类
//   sola.lang.Exception        // 异常基类（可捕获的）
//   sola.lang.RuntimeException // 运行时异常
//   sola.lang.Error            // 错误基类（通常不应捕获）

