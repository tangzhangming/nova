package runtime

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"path/filepath"
	"regexp"
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
// 文件流管理 (用于 native_stream_* 函数)
// ============================================================================

type fileStream struct {
	file   *os.File
	reader *bufio.Reader
	mode   string
}

var (
	fileStreamPool   = make(map[int]*fileStream)
	fileStreamNextID = 1
	fileStreamMutex  sync.Mutex
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

	// Native 文件操作函数 (仅供标准库使用)
	r.builtins["native_file_read"] = nativeFileRead
	r.builtins["native_file_write"] = nativeFileWrite
	r.builtins["native_file_append"] = nativeFileAppend
	r.builtins["native_file_exists"] = nativeFileExists
	r.builtins["native_file_delete"] = nativeFileDelete
	r.builtins["native_file_copy"] = nativeFileCopy
	r.builtins["native_file_rename"] = nativeFileRename
	r.builtins["native_is_file"] = nativeIsFile

	// Native 目录操作函数 (仅供标准库使用)
	r.builtins["native_dir_create"] = nativeDirCreate
	r.builtins["native_dir_create_all"] = nativeDirCreateAll
	r.builtins["native_dir_delete"] = nativeDirDelete
	r.builtins["native_dir_delete_all"] = nativeDirDeleteAll
	r.builtins["native_dir_list"] = nativeDirList
	r.builtins["native_is_dir"] = nativeIsDir

	// Native 文件信息函数 (仅供标准库使用)
	r.builtins["native_file_size"] = nativeFileSize
	r.builtins["native_file_mtime"] = nativeFileMtime
	r.builtins["native_file_atime"] = nativeFileAtime
	r.builtins["native_file_ctime"] = nativeFileCtime
	r.builtins["native_file_perms"] = nativeFilePerms
	r.builtins["native_is_readable"] = nativeIsReadable
	r.builtins["native_is_writable"] = nativeIsWritable
	r.builtins["native_is_executable"] = nativeIsExecutable
	r.builtins["native_is_link"] = nativeIsLink

	// Native 流操作函数 (仅供标准库使用)
	r.builtins["native_stream_open"] = r.nativeStreamOpen
	r.builtins["native_stream_read"] = r.nativeStreamRead
	r.builtins["native_stream_read_line"] = r.nativeStreamReadLine
	r.builtins["native_stream_write"] = r.nativeStreamWrite
	r.builtins["native_stream_seek"] = r.nativeStreamSeek
	r.builtins["native_stream_tell"] = r.nativeStreamTell
	r.builtins["native_stream_eof"] = r.nativeStreamEof
	r.builtins["native_stream_flush"] = r.nativeStreamFlush
	r.builtins["native_stream_close"] = r.nativeStreamClose

	// Native 正则表达式函数 (仅供标准库使用)
	r.builtins["native_regex_match"] = nativeRegexMatch
	r.builtins["native_regex_find"] = nativeRegexFind
	r.builtins["native_regex_find_index"] = nativeRegexFindIndex
	r.builtins["native_regex_find_all"] = nativeRegexFindAll
	r.builtins["native_regex_groups"] = nativeRegexGroups
	r.builtins["native_regex_find_all_groups"] = nativeRegexFindAllGroups
	r.builtins["native_regex_replace"] = nativeRegexReplace
	r.builtins["native_regex_replace_all"] = nativeRegexReplaceAll
	r.builtins["native_regex_split"] = nativeRegexSplit
	r.builtins["native_regex_escape"] = nativeRegexEscape

	// Native 时间函数 (仅供标准库使用)
	r.builtins["native_time_now"] = nativeTimeNow
	r.builtins["native_time_now_ms"] = nativeTimeNowMs
	r.builtins["native_time_now_nano"] = nativeTimeNowNano
	r.builtins["native_time_sleep"] = nativeTimeSleep
	r.builtins["native_time_parse"] = nativeTimeParse
	r.builtins["native_time_format"] = nativeTimeFormat
	r.builtins["native_time_year"] = nativeTimeYear
	r.builtins["native_time_month"] = nativeTimeMonth
	r.builtins["native_time_day"] = nativeTimeDay
	r.builtins["native_time_hour"] = nativeTimeHour
	r.builtins["native_time_minute"] = nativeTimeMinute
	r.builtins["native_time_second"] = nativeTimeSecond
	r.builtins["native_time_weekday"] = nativeTimeWeekday
	r.builtins["native_time_make"] = nativeTimeMake

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

// ============================================================================
// Native 文件操作函数
// ============================================================================

// nativeFileRead 读取文件全部内容
func nativeFileRead(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewString("")
	}
	path := args[0].AsString()
	data, err := os.ReadFile(path)
	if err != nil {
		return bytecode.NewString("")
	}
	return bytecode.NewString(string(data))
}

// nativeFileWrite 写入文件（覆盖）
func nativeFileWrite(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.FalseValue
	}
	path := args[0].AsString()
	content := args[1].AsString()
	err := os.WriteFile(path, []byte(content), 0644)
	return bytecode.NewBool(err == nil)
}

// nativeFileAppend 追加内容到文件
func nativeFileAppend(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.FalseValue
	}
	path := args[0].AsString()
	content := args[1].AsString()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return bytecode.FalseValue
	}
	defer f.Close()
	_, err = f.WriteString(content)
	return bytecode.NewBool(err == nil)
}

// nativeFileExists 检查路径是否存在
func nativeFileExists(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	path := args[0].AsString()
	_, err := os.Stat(path)
	return bytecode.NewBool(err == nil)
}

// nativeFileDelete 删除文件
func nativeFileDelete(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	path := args[0].AsString()
	err := os.Remove(path)
	return bytecode.NewBool(err == nil)
}

// nativeFileCopy 复制文件
func nativeFileCopy(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.FalseValue
	}
	src := args[0].AsString()
	dst := args[1].AsString()

	srcFile, err := os.Open(src)
	if err != nil {
		return bytecode.FalseValue
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return bytecode.FalseValue
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return bytecode.NewBool(err == nil)
}

// nativeFileRename 重命名/移动文件
func nativeFileRename(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.FalseValue
	}
	oldPath := args[0].AsString()
	newPath := args[1].AsString()
	err := os.Rename(oldPath, newPath)
	return bytecode.NewBool(err == nil)
}

// nativeIsFile 检查是否是文件
func nativeIsFile(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	path := args[0].AsString()
	info, err := os.Stat(path)
	if err != nil {
		return bytecode.FalseValue
	}
	return bytecode.NewBool(!info.IsDir())
}

// ============================================================================
// Native 目录操作函数
// ============================================================================

// nativeDirCreate 创建单级目录
func nativeDirCreate(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	path := args[0].AsString()
	err := os.Mkdir(path, 0755)
	return bytecode.NewBool(err == nil)
}

// nativeDirCreateAll 递归创建目录
func nativeDirCreateAll(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	path := args[0].AsString()
	err := os.MkdirAll(path, 0755)
	return bytecode.NewBool(err == nil)
}

// nativeDirDelete 删除空目录
func nativeDirDelete(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	path := args[0].AsString()
	err := os.Remove(path)
	return bytecode.NewBool(err == nil)
}

// nativeDirDeleteAll 递归删除目录
func nativeDirDeleteAll(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	path := args[0].AsString()
	err := os.RemoveAll(path)
	return bytecode.NewBool(err == nil)
}

// nativeDirList 列出目录内容
func nativeDirList(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewArray([]bytecode.Value{})
	}
	path := args[0].AsString()
	entries, err := os.ReadDir(path)
	if err != nil {
		return bytecode.NewArray([]bytecode.Value{})
	}
	result := make([]bytecode.Value, len(entries))
	for i, entry := range entries {
		result[i] = bytecode.NewString(entry.Name())
	}
	return bytecode.NewArray(result)
}

// nativeIsDir 检查是否是目录
func nativeIsDir(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	path := args[0].AsString()
	info, err := os.Stat(path)
	if err != nil {
		return bytecode.FalseValue
	}
	return bytecode.NewBool(info.IsDir())
}

// ============================================================================
// Native 文件信息函数
// ============================================================================

// nativeFileSize 获取文件大小
func nativeFileSize(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewInt(-1)
	}
	path := args[0].AsString()
	info, err := os.Stat(path)
	if err != nil {
		return bytecode.NewInt(-1)
	}
	return bytecode.NewInt(info.Size())
}

// nativeFileMtime 获取修改时间
func nativeFileMtime(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewInt(-1)
	}
	path := args[0].AsString()
	info, err := os.Stat(path)
	if err != nil {
		return bytecode.NewInt(-1)
	}
	return bytecode.NewInt(info.ModTime().Unix())
}

// nativeFileAtime 获取访问时间（在某些系统上可能与修改时间相同）
func nativeFileAtime(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewInt(-1)
	}
	path := args[0].AsString()
	info, err := os.Stat(path)
	if err != nil {
		return bytecode.NewInt(-1)
	}
	// Go 标准库不直接提供 atime，使用 mtime 作为后备
	return bytecode.NewInt(info.ModTime().Unix())
}

// nativeFileCtime 获取创建时间（在某些系统上可能与修改时间相同）
func nativeFileCtime(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewInt(-1)
	}
	path := args[0].AsString()
	info, err := os.Stat(path)
	if err != nil {
		return bytecode.NewInt(-1)
	}
	// Go 标准库不直接提供 ctime，使用 mtime 作为后备
	return bytecode.NewInt(info.ModTime().Unix())
}

// nativeFilePerms 获取文件权限
func nativeFilePerms(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewInt(0)
	}
	path := args[0].AsString()
	info, err := os.Stat(path)
	if err != nil {
		return bytecode.NewInt(0)
	}
	return bytecode.NewInt(int64(info.Mode().Perm()))
}

// nativeIsReadable 检查是否可读
func nativeIsReadable(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	path := args[0].AsString()
	f, err := os.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return bytecode.FalseValue
	}
	f.Close()
	return bytecode.TrueValue
}

// nativeIsWritable 检查是否可写
func nativeIsWritable(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	path := args[0].AsString()
	info, err := os.Stat(path)
	if err != nil {
		// 文件不存在，检查父目录是否可写
		dir := filepath.Dir(path)
		f, err := os.OpenFile(dir, os.O_WRONLY, 0)
		if err != nil {
			return bytecode.FalseValue
		}
		f.Close()
		return bytecode.TrueValue
	}
	// 文件存在，尝试以写模式打开
	if info.IsDir() {
		return bytecode.NewBool(info.Mode().Perm()&0200 != 0)
	}
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return bytecode.FalseValue
	}
	f.Close()
	return bytecode.TrueValue
}

// nativeIsExecutable 检查是否可执行
func nativeIsExecutable(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	path := args[0].AsString()
	info, err := os.Stat(path)
	if err != nil {
		return bytecode.FalseValue
	}
	return bytecode.NewBool(info.Mode().Perm()&0111 != 0)
}

// nativeIsLink 检查是否是符号链接
func nativeIsLink(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	path := args[0].AsString()
	info, err := os.Lstat(path)
	if err != nil {
		return bytecode.FalseValue
	}
	return bytecode.NewBool(info.Mode()&os.ModeSymlink != 0)
}

// ============================================================================
// Native 流操作函数
// ============================================================================

// nativeStreamOpen 打开文件流
func (r *Runtime) nativeStreamOpen(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewInt(-1)
	}
	path := args[0].AsString()
	mode := args[1].AsString()

	var flag int
	switch mode {
	case "r":
		flag = os.O_RDONLY
	case "w":
		flag = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	case "a":
		flag = os.O_WRONLY | os.O_CREATE | os.O_APPEND
	case "r+":
		flag = os.O_RDWR
	case "w+":
		flag = os.O_RDWR | os.O_CREATE | os.O_TRUNC
	case "a+":
		flag = os.O_RDWR | os.O_CREATE | os.O_APPEND
	default:
		return bytecode.NewInt(-1)
	}

	f, err := os.OpenFile(path, flag, 0644)
	if err != nil {
		return bytecode.NewInt(-1)
	}

	fileStreamMutex.Lock()
	id := fileStreamNextID
	fileStreamNextID++
	fileStreamPool[id] = &fileStream{
		file:   f,
		reader: bufio.NewReader(f),
		mode:   mode,
	}
	fileStreamMutex.Unlock()

	return bytecode.NewInt(int64(id))
}

// nativeStreamRead 读取指定长度
func (r *Runtime) nativeStreamRead(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewString("")
	}
	id := int(args[0].AsInt())
	length := int(args[1].AsInt())

	fileStreamMutex.Lock()
	stream, ok := fileStreamPool[id]
	fileStreamMutex.Unlock()
	if !ok || stream.file == nil {
		return bytecode.NewString("")
	}

	buf := make([]byte, length)
	n, err := stream.file.Read(buf)
	if err != nil && err != io.EOF {
		return bytecode.NewString("")
	}
	return bytecode.NewString(string(buf[:n]))
}

// nativeStreamReadLine 读取一行
func (r *Runtime) nativeStreamReadLine(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewString("")
	}
	id := int(args[0].AsInt())

	fileStreamMutex.Lock()
	stream, ok := fileStreamPool[id]
	fileStreamMutex.Unlock()
	if !ok || stream.file == nil {
		return bytecode.NewString("")
	}

	line, err := stream.reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return bytecode.NewString("")
	}
	return bytecode.NewString(strings.TrimRight(line, "\r\n"))
}

// nativeStreamWrite 写入内容
func (r *Runtime) nativeStreamWrite(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewInt(-1)
	}
	id := int(args[0].AsInt())
	content := args[1].AsString()

	fileStreamMutex.Lock()
	stream, ok := fileStreamPool[id]
	fileStreamMutex.Unlock()
	if !ok || stream.file == nil {
		return bytecode.NewInt(-1)
	}

	n, err := stream.file.WriteString(content)
	if err != nil {
		return bytecode.NewInt(-1)
	}
	return bytecode.NewInt(int64(n))
}

// nativeStreamSeek 移动文件指针
func (r *Runtime) nativeStreamSeek(args []bytecode.Value) bytecode.Value {
	if len(args) < 3 {
		return bytecode.FalseValue
	}
	id := int(args[0].AsInt())
	offset := args[1].AsInt()
	whence := int(args[2].AsInt())

	fileStreamMutex.Lock()
	stream, ok := fileStreamPool[id]
	fileStreamMutex.Unlock()
	if !ok || stream.file == nil {
		return bytecode.FalseValue
	}

	_, err := stream.file.Seek(offset, whence)
	if err != nil {
		return bytecode.FalseValue
	}
	// 重置 reader
	stream.reader.Reset(stream.file)
	return bytecode.TrueValue
}

// nativeStreamTell 获取当前位置
func (r *Runtime) nativeStreamTell(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewInt(-1)
	}
	id := int(args[0].AsInt())

	fileStreamMutex.Lock()
	stream, ok := fileStreamPool[id]
	fileStreamMutex.Unlock()
	if !ok || stream.file == nil {
		return bytecode.NewInt(-1)
	}

	pos, err := stream.file.Seek(0, io.SeekCurrent)
	if err != nil {
		return bytecode.NewInt(-1)
	}
	return bytecode.NewInt(pos)
}

// nativeStreamEof 检查是否到达文件末尾
func (r *Runtime) nativeStreamEof(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.TrueValue
	}
	id := int(args[0].AsInt())

	fileStreamMutex.Lock()
	stream, ok := fileStreamPool[id]
	fileStreamMutex.Unlock()
	if !ok || stream.file == nil {
		return bytecode.TrueValue
	}

	// 尝试读取一个字节来检查是否到达末尾
	currentPos, _ := stream.file.Seek(0, io.SeekCurrent)
	buf := make([]byte, 1)
	_, err := stream.file.Read(buf)
	stream.file.Seek(currentPos, io.SeekStart)
	stream.reader.Reset(stream.file)

	return bytecode.NewBool(err == io.EOF)
}

// nativeStreamFlush 刷新缓冲区
func (r *Runtime) nativeStreamFlush(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	id := int(args[0].AsInt())

	fileStreamMutex.Lock()
	stream, ok := fileStreamPool[id]
	fileStreamMutex.Unlock()
	if !ok || stream.file == nil {
		return bytecode.FalseValue
	}

	err := stream.file.Sync()
	return bytecode.NewBool(err == nil)
}

// nativeStreamClose 关闭文件流
func (r *Runtime) nativeStreamClose(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	id := int(args[0].AsInt())

	fileStreamMutex.Lock()
	stream, ok := fileStreamPool[id]
	if ok {
		delete(fileStreamPool, id)
	}
	fileStreamMutex.Unlock()

	if !ok || stream.file == nil {
		return bytecode.FalseValue
	}

	err := stream.file.Close()
	return bytecode.NewBool(err == nil)
}

// ============================================================================
// Native 正则表达式函数
// ============================================================================

// nativeRegexMatch 检测是否匹配
func nativeRegexMatch(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.FalseValue
	}
	pattern := args[0].AsString()
	str := args[1].AsString()

	re, err := regexp.Compile(pattern)
	if err != nil {
		return bytecode.FalseValue
	}
	return bytecode.NewBool(re.MatchString(str))
}

// nativeRegexFind 查找第一个匹配
func nativeRegexFind(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewString("")
	}
	pattern := args[0].AsString()
	str := args[1].AsString()

	re, err := regexp.Compile(pattern)
	if err != nil {
		return bytecode.NewString("")
	}
	match := re.FindString(str)
	return bytecode.NewString(match)
}

// nativeRegexFindIndex 返回第一个匹配的位置 [start, end]
func nativeRegexFindIndex(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewArray([]bytecode.Value{})
	}
	pattern := args[0].AsString()
	str := args[1].AsString()

	re, err := regexp.Compile(pattern)
	if err != nil {
		return bytecode.NewArray([]bytecode.Value{})
	}
	loc := re.FindStringIndex(str)
	if loc == nil {
		return bytecode.NewArray([]bytecode.Value{})
	}
	return bytecode.NewArray([]bytecode.Value{
		bytecode.NewInt(int64(loc[0])),
		bytecode.NewInt(int64(loc[1])),
	})
}

// nativeRegexFindAll 查找所有匹配
func nativeRegexFindAll(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewArray([]bytecode.Value{})
	}
	pattern := args[0].AsString()
	str := args[1].AsString()

	re, err := regexp.Compile(pattern)
	if err != nil {
		return bytecode.NewArray([]bytecode.Value{})
	}
	matches := re.FindAllString(str, -1)
	result := make([]bytecode.Value, len(matches))
	for i, m := range matches {
		result[i] = bytecode.NewString(m)
	}
	return bytecode.NewArray(result)
}

// nativeRegexGroups 获取第一个匹配的捕获组
func nativeRegexGroups(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewArray([]bytecode.Value{})
	}
	pattern := args[0].AsString()
	str := args[1].AsString()

	re, err := regexp.Compile(pattern)
	if err != nil {
		return bytecode.NewArray([]bytecode.Value{})
	}
	groups := re.FindStringSubmatch(str)
	result := make([]bytecode.Value, len(groups))
	for i, g := range groups {
		result[i] = bytecode.NewString(g)
	}
	return bytecode.NewArray(result)
}

// nativeRegexFindAllGroups 获取所有匹配的捕获组
func nativeRegexFindAllGroups(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewArray([]bytecode.Value{})
	}
	pattern := args[0].AsString()
	str := args[1].AsString()

	re, err := regexp.Compile(pattern)
	if err != nil {
		return bytecode.NewArray([]bytecode.Value{})
	}
	allMatches := re.FindAllStringSubmatch(str, -1)
	result := make([]bytecode.Value, len(allMatches))
	for i, groups := range allMatches {
		groupValues := make([]bytecode.Value, len(groups))
		for j, g := range groups {
			groupValues[j] = bytecode.NewString(g)
		}
		result[i] = bytecode.NewArray(groupValues)
	}
	return bytecode.NewArray(result)
}

// nativeRegexReplace 替换第一个匹配
func nativeRegexReplace(args []bytecode.Value) bytecode.Value {
	if len(args) < 3 {
		return bytecode.NewString("")
	}
	pattern := args[0].AsString()
	str := args[1].AsString()
	replacement := args[2].AsString()

	re, err := regexp.Compile(pattern)
	if err != nil {
		return bytecode.NewString(str)
	}
	// Go regexp 没有直接的 ReplaceFirst，使用 ReplaceAllStringFunc 模拟
	replaced := false
	result := re.ReplaceAllStringFunc(str, func(match string) string {
		if !replaced {
			replaced = true
			return replacement
		}
		return match
	})
	return bytecode.NewString(result)
}

// nativeRegexReplaceAll 替换所有匹配
func nativeRegexReplaceAll(args []bytecode.Value) bytecode.Value {
	if len(args) < 3 {
		return bytecode.NewString("")
	}
	pattern := args[0].AsString()
	str := args[1].AsString()
	replacement := args[2].AsString()

	re, err := regexp.Compile(pattern)
	if err != nil {
		return bytecode.NewString(str)
	}
	return bytecode.NewString(re.ReplaceAllString(str, replacement))
}

// nativeRegexSplit 按模式分割字符串
func nativeRegexSplit(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewArray([]bytecode.Value{})
	}
	pattern := args[0].AsString()
	str := args[1].AsString()
	limit := -1
	if len(args) > 2 {
		limit = int(args[2].AsInt())
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return bytecode.NewArray([]bytecode.Value{bytecode.NewString(str)})
	}
	parts := re.Split(str, limit)
	result := make([]bytecode.Value, len(parts))
	for i, p := range parts {
		result[i] = bytecode.NewString(p)
	}
	return bytecode.NewArray(result)
}

// nativeRegexEscape 转义正则特殊字符
func nativeRegexEscape(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewString("")
	}
	str := args[0].AsString()
	return bytecode.NewString(regexp.QuoteMeta(str))
}

// ============================================================================
// Native 时间函数
// ============================================================================

// nativeTimeNow 获取当前 Unix 时间戳（秒）
func nativeTimeNow(args []bytecode.Value) bytecode.Value {
	return bytecode.NewInt(time.Now().Unix())
}

// nativeTimeNowMs 获取当前毫秒时间戳
func nativeTimeNowMs(args []bytecode.Value) bytecode.Value {
	return bytecode.NewInt(time.Now().UnixMilli())
}

// nativeTimeNowNano 获取当前纳秒时间戳
func nativeTimeNowNano(args []bytecode.Value) bytecode.Value {
	return bytecode.NewInt(time.Now().UnixNano())
}

// nativeTimeSleep 休眠指定毫秒
func nativeTimeSleep(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NullValue
	}
	ms := args[0].AsInt()
	time.Sleep(time.Duration(ms) * time.Millisecond)
	return bytecode.NullValue
}

// nativeTimeParse 解析时间字符串
// 参数：str, format（Go 格式，如 "2006-01-02 15:04:05"）
func nativeTimeParse(args []bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.NewInt(0)
	}
	str := args[0].AsString()
	format := "2006-01-02 15:04:05"
	if len(args) > 1 {
		format = args[1].AsString()
	}

	t, err := time.Parse(format, str)
	if err != nil {
		// 尝试常见格式
		formats := []string{
			"2006-01-02 15:04:05",
			"2006-01-02",
			"2006/01/02 15:04:05",
			"2006/01/02",
			time.RFC3339,
		}
		for _, f := range formats {
			if t, err = time.Parse(f, str); err == nil {
				break
			}
		}
		if err != nil {
			return bytecode.NewInt(0)
		}
	}
	return bytecode.NewInt(t.Unix())
}

// nativeTimeFormat 格式化时间戳
// 参数：timestamp, format（Go 格式）
func nativeTimeFormat(args []bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.NewString("")
	}
	timestamp := args[0].AsInt()
	format := "2006-01-02 15:04:05"
	if len(args) > 1 {
		format = args[1].AsString()
	}

	t := time.Unix(timestamp, 0)
	return bytecode.NewString(t.Format(format))
}

// nativeTimeYear 获取年份
func nativeTimeYear(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewInt(int64(time.Now().Year()))
	}
	timestamp := args[0].AsInt()
	t := time.Unix(timestamp, 0)
	return bytecode.NewInt(int64(t.Year()))
}

// nativeTimeMonth 获取月份
func nativeTimeMonth(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewInt(int64(time.Now().Month()))
	}
	timestamp := args[0].AsInt()
	t := time.Unix(timestamp, 0)
	return bytecode.NewInt(int64(t.Month()))
}

// nativeTimeDay 获取日
func nativeTimeDay(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewInt(int64(time.Now().Day()))
	}
	timestamp := args[0].AsInt()
	t := time.Unix(timestamp, 0)
	return bytecode.NewInt(int64(t.Day()))
}

// nativeTimeHour 获取时
func nativeTimeHour(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewInt(int64(time.Now().Hour()))
	}
	timestamp := args[0].AsInt()
	t := time.Unix(timestamp, 0)
	return bytecode.NewInt(int64(t.Hour()))
}

// nativeTimeMinute 获取分
func nativeTimeMinute(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewInt(int64(time.Now().Minute()))
	}
	timestamp := args[0].AsInt()
	t := time.Unix(timestamp, 0)
	return bytecode.NewInt(int64(t.Minute()))
}

// nativeTimeSecond 获取秒
func nativeTimeSecond(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewInt(int64(time.Now().Second()))
	}
	timestamp := args[0].AsInt()
	t := time.Unix(timestamp, 0)
	return bytecode.NewInt(int64(t.Second()))
}

// nativeTimeWeekday 获取星期几（0=周日，1=周一...）
func nativeTimeWeekday(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewInt(int64(time.Now().Weekday()))
	}
	timestamp := args[0].AsInt()
	t := time.Unix(timestamp, 0)
	return bytecode.NewInt(int64(t.Weekday()))
}

// nativeTimeMake 构建时间戳
// 参数：year, month, day, hour, minute, second
func nativeTimeMake(args []bytecode.Value) bytecode.Value {
	if len(args) < 3 {
		return bytecode.NewInt(0)
	}
	year := int(args[0].AsInt())
	month := time.Month(args[1].AsInt())
	day := int(args[2].AsInt())
	hour := 0
	minute := 0
	second := 0
	if len(args) > 3 {
		hour = int(args[3].AsInt())
	}
	if len(args) > 4 {
		minute = int(args[4].AsInt())
	}
	if len(args) > 5 {
		second = int(args[5].AsInt())
	}

	t := time.Date(year, month, day, hour, minute, second, 0, time.Local)
	return bytecode.NewInt(t.Unix())
}

