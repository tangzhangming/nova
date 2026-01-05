package runtime

import (
	"fmt"
	"path/filepath"

	"github.com/tangzhangming/nova/internal/ast"
	"github.com/tangzhangming/nova/internal/bytecode"
	"github.com/tangzhangming/nova/internal/compiler"
	"github.com/tangzhangming/nova/internal/i18n"
	"github.com/tangzhangming/nova/internal/loader"
	"github.com/tangzhangming/nova/internal/parser"
	"github.com/tangzhangming/nova/internal/vm"
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
// 内置函数注册
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
