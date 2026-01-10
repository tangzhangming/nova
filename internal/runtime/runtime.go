package runtime

import (
	"fmt"

	"github.com/tangzhangming/nova/internal/ast"
	"github.com/tangzhangming/nova/internal/bytecode"
	"github.com/tangzhangming/nova/internal/compiler"
	"github.com/tangzhangming/nova/internal/i18n"
	"github.com/tangzhangming/nova/internal/jit"
	"github.com/tangzhangming/nova/internal/loader"
	"github.com/tangzhangming/nova/internal/parser"
	"github.com/tangzhangming/nova/internal/vm"
)

// Runtime Sola 运行时
type Runtime struct {
	vm          *vm.VM
	builtins    map[string]BuiltinFunc
	loader      *loader.Loader
	classes     map[string]*bytecode.Class
	enums       map[string]*bytecode.Enum
	symbolTable *compiler.SymbolTable // 共享符号表
}

// BuiltinFunc 内置函数类型
type BuiltinFunc func(args []bytecode.Value) bytecode.Value

// Options 运行时选项
type Options struct {
	// JITEnabled 是否启用 JIT 编译
	// 设置为 false 等效于 --jitless 或 -Xint
	JITEnabled bool
}

// DefaultOptions 返回默认选项
func DefaultOptions() Options {
	return Options{
		JITEnabled: true,
	}
}

// New 创建运行时
func New() *Runtime {
	return NewWithOptions(DefaultOptions())
}

// NewWithOptions 创建带选项的运行时
func NewWithOptions(opts Options) *Runtime {
	var jitConfig *jit.Config
	if !opts.JITEnabled {
		jitConfig = jit.InterpretOnlyConfig()
	}
	
	r := &Runtime{
		vm:          vm.NewWithConfig(jitConfig),
		builtins:    make(map[string]BuiltinFunc),
		classes:     make(map[string]*bytecode.Class),
		enums:       make(map[string]*bytecode.Enum),
		symbolTable: compiler.NewSymbolTable(),
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

	// 处理 use 声明，加载依赖（使用共享符号表）
	for _, use := range file.Uses {
		if err := r.loadDependency(use.Path); err != nil {
			return fmt.Errorf(i18n.T(i18n.ErrLoadFailed, use.Path, err))
		}
	}

	// 编译入口文件（使用共享符号表，以便识别导入的类）
	c := compiler.NewWithSymbolTable(r.symbolTable)
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

	// 构建 VTable（为所有类构建接口虚表）
	bytecode.BuildAllVTables(r.classes)

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
		// VM 已经打印了详细的错误信息，这里返回空错误表示执行失败
		return fmt.Errorf("")
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

	// 解析（使用完整路径以便错误信息显示准确位置）
	p := parser.New(source, filePath)
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

	// 编译（使用共享符号表）
	c := compiler.NewWithSymbolTable(r.symbolTable)
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

// CompileToCompiledFile 编译为 CompiledFile（用于 build 命令）
func (r *Runtime) CompileToCompiledFile(source, filename string) (*bytecode.CompiledFile, error) {
	// 创建加载器
	var err error
	r.loader, err = loader.New(filename)
	if err != nil {
		return nil, fmt.Errorf(i18n.T(i18n.ErrFailedCreateLoader, err))
	}

	// 解析入口文件
	p := parser.New(source, filename)
	file := p.Parse()

	if p.HasErrors() {
		for _, e := range p.Errors() {
			fmt.Printf(i18n.T(i18n.ErrParseError, e) + "\n")
		}
		return nil, fmt.Errorf(i18n.T(i18n.ErrParseFailed))
	}

	// 处理 use 声明，加载依赖（使用共享符号表）
	for _, use := range file.Uses {
		if err := r.loadDependency(use.Path); err != nil {
			return nil, fmt.Errorf(i18n.T(i18n.ErrLoadFailed, use.Path, err))
		}
	}

	// 编译入口文件（使用共享符号表，以便识别导入的类）
	c := compiler.NewWithSymbolTable(r.symbolTable)
	fn, errs := c.Compile(file)

	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Printf(i18n.T(i18n.ErrCompileError, e) + "\n")
		}
		return nil, fmt.Errorf(i18n.T(i18n.ErrCompileFailed))
	}

	// 收集所有类和枚举（包括依赖项）
	allClasses := make(map[string]*bytecode.Class)
	for name, class := range c.Classes() {
		allClasses[name] = class
	}
	for name, class := range r.classes {
		allClasses[name] = class
	}

	allEnums := make(map[string]*bytecode.Enum)
	for name, enum := range c.Enums() {
		allEnums[name] = enum
	}
	for name, enum := range r.enums {
		allEnums[name] = enum
	}

	return &bytecode.CompiledFile{
		MainFunction: fn,
		Classes:      allClasses,
		Enums:        allEnums,
		SourceFile:   filename,
	}, nil
}

// RunCompiled 运行编译后的字节码文件
func (r *Runtime) RunCompiled(cf *bytecode.CompiledFile) error {
	// 注册所有类
	for name, class := range cf.Classes {
		r.classes[name] = class
		r.vm.DefineClass(class)
	}

	// 解析父类引用
	for _, class := range r.classes {
		if class.ParentName != "" && class.Parent == nil {
			if parent, ok := r.classes[class.ParentName]; ok {
				class.Parent = parent
			}
		}
	}

	// 注册枚举
	for name, enum := range cf.Enums {
		r.enums[name] = enum
		r.vm.DefineEnum(enum)
	}

	// 注册内置函数
	r.registerBuiltinsToVM()

	// 运行
	result := r.vm.Run(cf.MainFunction)
	if result != vm.InterpretOK {
		return fmt.Errorf("")
	}

	return nil
}

// RunREPL 运行 REPL 输入
// 支持增量执行，保持环境状态
func (r *Runtime) RunREPL(source, filename string) error {
	// 解析输入
	p := parser.New(source, filename)
	file := p.Parse()

	if p.HasErrors() {
		for _, e := range p.Errors() {
			fmt.Printf(i18n.T(i18n.ErrParseError, e) + "\n")
		}
		return fmt.Errorf(i18n.T(i18n.ErrParseFailed))
	}

	// 处理 use 声明
	for _, use := range file.Uses {
		if r.loader == nil {
			var err error
			r.loader, err = loader.New(filename)
			if err != nil {
				return fmt.Errorf(i18n.T(i18n.ErrFailedCreateLoader, err))
			}
		}
		if err := r.loadDependency(use.Path); err != nil {
			return fmt.Errorf(i18n.T(i18n.ErrLoadFailed, use.Path, err))
		}
	}

	// 编译（使用共享符号表保持状态）
	c := compiler.NewWithSymbolTable(r.symbolTable)
	fn, errs := c.Compile(file)

	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Printf(i18n.T(i18n.ErrCompileError, e) + "\n")
		}
		return fmt.Errorf(i18n.T(i18n.ErrCompileFailed))
	}

	// 注册新的类
	for name, class := range c.Classes() {
		r.classes[name] = class
		r.vm.DefineClass(class)
	}

	// 注册新的枚举
	for name, enum := range c.Enums() {
		r.enums[name] = enum
		r.vm.DefineEnum(enum)
	}

	// 确保内置函数已注册
	r.registerBuiltinsToVM()

	// 运行
	result := r.vm.Run(fn)
	if result != vm.InterpretOK {
		return fmt.Errorf("")
	}

	return nil
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

	// Native TCP 函数 - 连接管理 (仅供标准库使用)
	r.builtins["native_tcp_connect"] = nativeTcpConnect
	r.builtins["native_tcp_connect_timeout"] = nativeTcpConnectTimeout
	r.builtins["native_tcp_close"] = nativeTcpClose
	r.builtins["native_tcp_is_connected"] = nativeTcpIsConnected

	// Native TCP 函数 - 数据读写
	r.builtins["native_tcp_write"] = nativeTcpWrite
	r.builtins["native_tcp_write_bytes"] = nativeTcpWriteBytes
	r.builtins["native_tcp_read"] = nativeTcpRead
	r.builtins["native_tcp_read_bytes"] = nativeTcpReadBytes
	r.builtins["native_tcp_read_exact"] = nativeTcpReadExact
	r.builtins["native_tcp_read_line"] = nativeTcpReadLine
	r.builtins["native_tcp_read_until"] = nativeTcpReadUntil
	r.builtins["native_tcp_available"] = nativeTcpAvailable
	r.builtins["native_tcp_flush"] = nativeTcpFlush

	// Native TCP 函数 - 超时配置
	r.builtins["native_tcp_set_timeout"] = nativeTcpSetTimeout
	r.builtins["native_tcp_set_timeout_ms"] = nativeTcpSetTimeoutMs
	r.builtins["native_tcp_set_read_timeout"] = nativeTcpSetReadTimeout
	r.builtins["native_tcp_set_write_timeout"] = nativeTcpSetWriteTimeout
	r.builtins["native_tcp_clear_timeout"] = nativeTcpClearTimeout

	// Native TCP 函数 - Socket选项
	r.builtins["native_tcp_set_keepalive"] = nativeTcpSetKeepAlive
	r.builtins["native_tcp_set_nodelay"] = nativeTcpSetNoDelay
	r.builtins["native_tcp_set_linger"] = nativeTcpSetLinger
	r.builtins["native_tcp_set_read_buffer"] = nativeTcpSetReadBuffer
	r.builtins["native_tcp_set_write_buffer"] = nativeTcpSetWriteBuffer

	// Native TCP 函数 - 地址信息
	r.builtins["native_tcp_get_local_addr"] = nativeTcpGetLocalAddr
	r.builtins["native_tcp_get_remote_addr"] = nativeTcpGetRemoteAddr
	r.builtins["native_tcp_get_local_host"] = nativeTcpGetLocalHost
	r.builtins["native_tcp_get_local_port"] = nativeTcpGetLocalPort
	r.builtins["native_tcp_get_remote_host"] = nativeTcpGetRemoteHost
	r.builtins["native_tcp_get_remote_port"] = nativeTcpGetRemotePort
	r.builtins["native_tcp_is_tls"] = nativeTcpIsTLS

	// Native TLS 函数 - SSL/TLS客户端支持
	r.builtins["native_tls_connect"] = nativeTlsConnect
	r.builtins["native_tls_connect_insecure"] = nativeTlsConnectInsecure
	r.builtins["native_tls_upgrade"] = nativeTlsUpgrade
	r.builtins["native_tls_get_version"] = nativeTlsGetVersion
	r.builtins["native_tls_get_cipher_suite"] = nativeTlsGetCipherSuite
	r.builtins["native_tls_get_server_name"] = nativeTlsGetServerName

	// Native TCP 函数 - 服务端监听
	r.builtins["native_tcp_listen"] = nativeTcpListen
	r.builtins["native_tcp_accept"] = nativeTcpAccept
	r.builtins["native_tcp_accept_timeout"] = nativeTcpAcceptTimeout
	r.builtins["native_tcp_stop_listen"] = nativeTcpStopListen
	r.builtins["native_tcp_listener_addr"] = nativeTcpListenerAddr
	r.builtins["native_tcp_listener_host"] = nativeTcpListenerHost
	r.builtins["native_tcp_listener_port"] = nativeTcpListenerPort
	r.builtins["native_tcp_listener_is_listening"] = nativeTcpListenerIsListening

	// Native TLS 函数 - SSL/TLS服务端支持
	r.builtins["native_tls_listen"] = nativeTlsListen
	r.builtins["native_tls_listener_is_tls"] = nativeTlsListenerIsTLS

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

	// Native JSON 函数 (仅供标准库使用)
	r.builtins["native_json_encode"] = nativeJsonEncode
	r.builtins["native_json_decode"] = nativeJsonDecode
	r.builtins["native_json_is_valid"] = nativeJsonIsValid
	r.builtins["native_json_encode_object"] = nativeJsonEncodeObject

	// Native Crypto 哈希函数 (仅供标准库使用)
	r.builtins["native_crypto_md5"] = nativeCryptoMd5
	r.builtins["native_crypto_md5_bytes"] = nativeCryptoMd5Bytes
	r.builtins["native_crypto_sha1"] = nativeCryptoSha1
	r.builtins["native_crypto_sha1_bytes"] = nativeCryptoSha1Bytes
	r.builtins["native_crypto_sha256"] = nativeCryptoSha256
	r.builtins["native_crypto_sha256_bytes"] = nativeCryptoSha256Bytes
	r.builtins["native_crypto_sha384"] = nativeCryptoSha384
	r.builtins["native_crypto_sha384_bytes"] = nativeCryptoSha384Bytes
	r.builtins["native_crypto_sha512"] = nativeCryptoSha512
	r.builtins["native_crypto_sha512_bytes"] = nativeCryptoSha512Bytes

	// Native Crypto 流式哈希函数
	r.builtins["native_crypto_hash_create"] = nativeCryptoHashCreate
	r.builtins["native_crypto_hash_update"] = nativeCryptoHashUpdate
	r.builtins["native_crypto_hash_finalize"] = nativeCryptoHashFinalize
	r.builtins["native_crypto_hash_finalize_bytes"] = nativeCryptoHashFinalizeBytes

	// Native Crypto HMAC函数
	r.builtins["native_crypto_hmac"] = nativeCryptoHmac
	r.builtins["native_crypto_hmac_bytes"] = nativeCryptoHmacBytes
	r.builtins["native_crypto_hmac_verify"] = nativeCryptoHmacVerify
	r.builtins["native_crypto_hmac_create"] = nativeCryptoHmacCreate
	r.builtins["native_crypto_hmac_update"] = nativeCryptoHmacUpdate
	r.builtins["native_crypto_hmac_finalize"] = nativeCryptoHmacFinalize

	// Native Crypto AES函数
	r.builtins["native_crypto_aes_encrypt_cbc"] = nativeCryptoAesEncryptCbc
	r.builtins["native_crypto_aes_decrypt_cbc"] = nativeCryptoAesDecryptCbc
	r.builtins["native_crypto_aes_encrypt_gcm"] = nativeCryptoAesEncryptGcm
	r.builtins["native_crypto_aes_decrypt_gcm"] = nativeCryptoAesDecryptGcm
	r.builtins["native_crypto_aes_encrypt_ctr"] = nativeCryptoAesEncryptCtr
	r.builtins["native_crypto_aes_decrypt_ctr"] = nativeCryptoAesDecryptCtr

	// Native Crypto DES/3DES函数
	r.builtins["native_crypto_des_encrypt"] = nativeCryptoDesEncrypt
	r.builtins["native_crypto_des_decrypt"] = nativeCryptoDesDecrypt
	r.builtins["native_crypto_triple_des_encrypt"] = nativeCryptoTripleDesEncrypt
	r.builtins["native_crypto_triple_des_decrypt"] = nativeCryptoTripleDesDecrypt

	// Native Crypto RSA函数
	r.builtins["native_crypto_rsa_generate"] = nativeCryptoRsaGenerate
	r.builtins["native_crypto_rsa_get_public_key_pem"] = nativeCryptoRsaGetPublicKeyPem
	r.builtins["native_crypto_rsa_get_private_key_pem"] = nativeCryptoRsaGetPrivateKeyPem
	r.builtins["native_crypto_rsa_load_public_key"] = nativeCryptoRsaLoadPublicKey
	r.builtins["native_crypto_rsa_load_private_key"] = nativeCryptoRsaLoadPrivateKey
	r.builtins["native_crypto_rsa_encrypt"] = nativeCryptoRsaEncrypt
	r.builtins["native_crypto_rsa_decrypt"] = nativeCryptoRsaDecrypt
	r.builtins["native_crypto_rsa_sign"] = nativeCryptoRsaSign
	r.builtins["native_crypto_rsa_verify"] = nativeCryptoRsaVerify
	r.builtins["native_crypto_rsa_sign_pkcs1"] = nativeCryptoRsaSignPkcs1
	r.builtins["native_crypto_rsa_verify_pkcs1"] = nativeCryptoRsaVerifyPkcs1
	r.builtins["native_crypto_rsa_encrypt_pkcs1"] = nativeCryptoRsaEncryptPkcs1
	r.builtins["native_crypto_rsa_decrypt_pkcs1"] = nativeCryptoRsaDecryptPkcs1
	r.builtins["native_crypto_rsa_free"] = nativeCryptoRsaFree

	// Native Crypto ECDSA函数
	r.builtins["native_crypto_ecdsa_generate"] = nativeCryptoEcdsaGenerate
	r.builtins["native_crypto_ecdsa_sign"] = nativeCryptoEcdsaSign
	r.builtins["native_crypto_ecdsa_verify"] = nativeCryptoEcdsaVerify
	r.builtins["native_crypto_ecdsa_get_public_key_pem"] = nativeCryptoEcdsaGetPublicKeyPem
	r.builtins["native_crypto_ecdsa_get_private_key_pem"] = nativeCryptoEcdsaGetPrivateKeyPem
	r.builtins["native_crypto_ecdsa_load_public_key"] = nativeCryptoEcdsaLoadPublicKey
	r.builtins["native_crypto_ecdsa_load_private_key"] = nativeCryptoEcdsaLoadPrivateKey
	r.builtins["native_crypto_ecdsa_free"] = nativeCryptoEcdsaFree

	// Native Crypto Ed25519函数
	r.builtins["native_crypto_ed25519_generate"] = nativeCryptoEd25519Generate
	r.builtins["native_crypto_ed25519_sign"] = nativeCryptoEd25519Sign
	r.builtins["native_crypto_ed25519_verify"] = nativeCryptoEd25519Verify
	r.builtins["native_crypto_ed25519_get_public_key_bytes"] = nativeCryptoEd25519GetPublicKeyBytes
	r.builtins["native_crypto_ed25519_get_private_key_bytes"] = nativeCryptoEd25519GetPrivateKeyBytes
	r.builtins["native_crypto_ed25519_load_public_key"] = nativeCryptoEd25519LoadPublicKey
	r.builtins["native_crypto_ed25519_load_private_key"] = nativeCryptoEd25519LoadPrivateKey
	r.builtins["native_crypto_ed25519_free"] = nativeCryptoEd25519Free

	// Native Crypto 密钥派生函数
	r.builtins["native_crypto_pbkdf2"] = nativeCryptoPbkdf2
	r.builtins["native_crypto_hkdf"] = nativeCryptoHkdf
	r.builtins["native_crypto_scrypt"] = nativeCryptoScrypt
	r.builtins["native_crypto_argon2id"] = nativeCryptoArgon2id
	r.builtins["native_crypto_argon2i"] = nativeCryptoArgon2i

	// Native Crypto 随机数函数
	r.builtins["native_crypto_random_bytes"] = nativeCryptoRandomBytes
	r.builtins["native_crypto_random_int"] = nativeCryptoRandomInt
	r.builtins["native_crypto_random_hex"] = nativeCryptoRandomHex
	r.builtins["native_crypto_random_uuid"] = nativeCryptoRandomUuid

	// Native Crypto Hex函数
	r.builtins["native_crypto_hex_encode"] = nativeCryptoHexEncode
	r.builtins["native_crypto_hex_decode"] = nativeCryptoHexDecode
	r.builtins["native_crypto_hex_is_valid"] = nativeCryptoHexIsValid

	// Native Base64 函数 (仅供标准库使用)
	r.builtins["native_base64_encode"] = nativeBase64Encode
	r.builtins["native_base64_decode"] = nativeBase64Decode
	r.builtins["native_base64_encode_url_safe"] = nativeBase64EncodeURLSafe
	r.builtins["native_base64_decode_url_safe"] = nativeBase64DecodeURLSafe
	r.builtins["native_base64_encode_raw"] = nativeBase64EncodeRaw
	r.builtins["native_base64_decode_raw"] = nativeBase64DecodeRaw
	r.builtins["native_base64_encode_raw_url_safe"] = nativeBase64EncodeRawURLSafe
	r.builtins["native_base64_decode_raw_url_safe"] = nativeBase64DecodeRawURLSafe
	r.builtins["native_base64_decode_strict"] = nativeBase64DecodeStrict
	r.builtins["native_base64_decode_strict_url_safe"] = nativeBase64DecodeStrictURLSafe
	r.builtins["native_base64_decode_strict_raw"] = nativeBase64DecodeStrictRaw
	r.builtins["native_base64_decode_strict_raw_url_safe"] = nativeBase64DecodeStrictRawURLSafe
	r.builtins["native_base64_encoded_len"] = nativeBase64EncodedLen
	r.builtins["native_base64_decoded_len"] = nativeBase64DecodedLen
	r.builtins["native_base64_encoded_len_raw"] = nativeBase64EncodedLenRaw
	r.builtins["native_base64_decoded_len_raw"] = nativeBase64DecodedLenRaw
	r.builtins["native_base64_is_valid"] = nativeBase64IsValid
	r.builtins["native_base64_is_valid_url_safe"] = nativeBase64IsValidURLSafe
	r.builtins["native_base64_is_valid_raw"] = nativeBase64IsValidRaw
	r.builtins["native_base64_is_valid_raw_url_safe"] = nativeBase64IsValidRawURLSafe

	// Native Bytes 函数 (仅供标准库使用)
	r.builtins["native_bytes_new"] = nativeBytesNew
	r.builtins["native_bytes_from_string"] = nativeBytesFromString
	r.builtins["native_bytes_to_string"] = nativeBytesToString
	r.builtins["native_bytes_from_hex"] = nativeBytesFromHex
	r.builtins["native_bytes_to_hex"] = nativeBytesToHex
	r.builtins["native_bytes_from_array"] = nativeBytesFromArray
	r.builtins["native_bytes_to_array"] = nativeBytesToArray
	r.builtins["native_bytes_len"] = nativeBytesLen
	r.builtins["native_bytes_get"] = nativeBytesGet
	r.builtins["native_bytes_set"] = nativeBytesSet
	r.builtins["native_bytes_slice"] = nativeBytesSlice
	r.builtins["native_bytes_concat"] = nativeBytesConcat
	r.builtins["native_bytes_copy"] = nativeBytesCopy
	r.builtins["native_bytes_equal"] = nativeBytesEqual
	r.builtins["native_bytes_compare"] = nativeBytesCompare
	r.builtins["native_bytes_index"] = nativeBytesIndex
	r.builtins["native_bytes_contains"] = nativeBytesContains
	r.builtins["native_bytes_fill"] = nativeBytesFill
	r.builtins["native_bytes_zero"] = nativeBytesZero

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
