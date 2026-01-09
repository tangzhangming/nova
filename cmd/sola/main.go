package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tangzhangming/nova/internal/ast"
	"github.com/tangzhangming/nova/internal/bytecode"
	"github.com/tangzhangming/nova/internal/formatter"
	"github.com/tangzhangming/nova/internal/i18n"
	"github.com/tangzhangming/nova/internal/jvmgen"
	"github.com/tangzhangming/nova/internal/lexer"
	"github.com/tangzhangming/nova/internal/loader"
	"github.com/tangzhangming/nova/internal/parser"
	"github.com/tangzhangming/nova/internal/runtime"
)

const (
	Version = "0.1.0"
)

// 全局语言参数
var globalLang string

func main() {
	// 预扫描全局参数 --lang 或 -lang
	args := preprocessArgs(os.Args[1:])

	// 初始化语言
	InitLanguage(globalLang)

	// 同步设置内部模块语言
	switch GetLanguage() {
	case LangChinese:
		i18n.SetLanguage(i18n.LangChinese)
	default:
		i18n.SetLanguage(i18n.LangEnglish)
	}

	if len(args) < 1 {
		printUsage()
		os.Exit(0)
	}

	command := args[0]

	switch command {
	case "run":
		cmdRun(args[1:])
	case "build":
		cmdBuild(args[1:])
	case "jvm":
		cmdJvm(args[1:])
	case "check":
		cmdCheck(args[1:])
	case "init":
		cmdInit(args[1:])
	case "format", "fmt":
		cmdFormat(args[1:])
	case "version", "-v", "--version":
		cmdVersion()
	case "help", "-h", "--help":
		printUsage()
	default:
		// 兼容旧用法：直接运行文件
		if len(args) >= 1 && !isFlag(args[0]) {
			cmdRun(args)
		} else {
			fmt.Fprintf(os.Stderr, Msg().ErrUnknownCmd+"\n\n", command)
			printUsage()
			os.Exit(1)
		}
	}
}

// preprocessArgs 预处理参数，提取全局 --lang 参数
func preprocessArgs(args []string) []string {
	var result []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--lang" || arg == "-lang" {
			if i+1 < len(args) {
				globalLang = args[i+1]
				i++ // 跳过下一个参数
				continue
			}
		} else if strings.HasPrefix(arg, "--lang=") {
			globalLang = strings.TrimPrefix(arg, "--lang=")
			continue
		} else if strings.HasPrefix(arg, "-lang=") {
			globalLang = strings.TrimPrefix(arg, "-lang=")
			continue
		}
		result = append(result, arg)
	}
	return result
}

func isFlag(s string) bool {
	return len(s) > 0 && s[0] == '-'
}

func printUsage() {
	m := Msg()
	fmt.Printf(m.VersionTitle+"\n\n", Version)
	fmt.Println(m.HelpUsage)
	fmt.Println("  sola [--lang en|zh] <command> [options] [arguments]")
	fmt.Println()
	fmt.Println(m.HelpCommands)
	fmt.Printf("  init            %s\n", m.CmdInit)
	fmt.Printf("  run <file>      %s\n", m.CmdRun)
	fmt.Printf("  build <file>    %s\n", m.CmdBuild)
	fmt.Printf("  jvm <file>      %s\n", m.CmdJvm)
	fmt.Printf("  check <file>    %s\n", m.CmdCheck)
	fmt.Printf("  format <file>   %s\n", m.CmdFormat)
	fmt.Printf("  version         %s\n", m.CmdVersion)
	fmt.Printf("  help            %s\n", m.CmdHelp)
	fmt.Println()
	fmt.Println(m.HelpOptions)
	fmt.Printf("  -tokens         %s\n", m.OptTokens)
	fmt.Printf("  -ast            %s\n", m.OptAST)
	fmt.Printf("  -bytecode       %s\n", m.OptBytecode)
	fmt.Printf("  --jitless       %s\n", m.OptJitless)
	fmt.Printf("  -Xint           %s\n", m.OptJitless)
	fmt.Printf("  --lang <en|zh>  %s\n", m.OptLang)
	fmt.Println()
	fmt.Println(m.HelpExamples)
	fmt.Printf("  sola run main%s\n", loader.SourceFileExtension)
	fmt.Printf("  sola run -ast main%s\n", loader.SourceFileExtension)
	fmt.Printf("  sola check main%s\n", loader.SourceFileExtension)
	fmt.Printf("  sola format -w main%s\n", loader.SourceFileExtension)
	fmt.Printf("  sola --lang zh help\n")
}

// cmdRun 运行 Sola 源文件或编译后的字节码
func cmdRun(args []string) {
	m := Msg()
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	showTokens := fs.Bool("tokens", false, m.OptTokens)
	showAST := fs.Bool("ast", false, m.OptAST)
	showBytecode := fs.Bool("bytecode", false, m.OptBytecode)
	
	// JIT 控制选项
	// --jitless 或 -Xint: 禁用 JIT 编译，仅使用解释器执行
	jitless := fs.Bool("jitless", false, m.OptJitless)
	xint := fs.Bool("Xint", false, m.OptJitless)

	fs.Usage = func() {
		fmt.Println(m.HelpUsage + " sola run [options] <file>")
		fmt.Println()
		fmt.Println(m.HelpOptions)
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	if fs.NArg() < 1 {
		fs.Usage()
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, m.ErrNoInput)
		os.Exit(1)
	}

	filename := fs.Arg(0)

	// 检查是否是编译后的文件
	if strings.HasSuffix(filename, bytecode.CompiledFileExtension) {
		runCompiledWithOptions(filename, *jitless || *xint)
		return
	}

	source, err := os.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, m.ErrReadFile+"\n", err)
		os.Exit(1)
	}

	// 词法分析模式
	if *showTokens {
		runLexer(string(source), filename)
		return
	}

	// AST 模式
	if *showAST {
		runParser(string(source), filename, true)
		return
	}

	// 字节码模式
	if *showBytecode {
		runDisassemble(string(source), filename)
		return
	}

	// 正常运行
	r := runtime.NewWithOptions(runtime.Options{
		JITEnabled: !(*jitless || *xint),
	})
	if err := r.Run(string(source), filename); err != nil {
		// 如果有非空错误消息则打印（VM 的异常信息已经打印过了）
		if err.Error() != "" {
			fmt.Fprintf(os.Stderr, m.ErrRuntime+"\n", err)
		}
		os.Exit(1)
	}
}

// runCompiled 运行编译后的字节码文件
func runCompiled(filename string) {
	runCompiledWithOptions(filename, false)
}

// runCompiledWithOptions 运行编译后的字节码文件（带选项）
func runCompiledWithOptions(filename string, jitless bool) {
	m := Msg()

	// 读取编译后的文件
	data, err := os.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, m.ErrReadFile+"\n", err)
		os.Exit(1)
	}

	// 验证文件头
	if err := bytecode.ValidateHeader(data); err != nil {
		fmt.Fprintf(os.Stderr, m.ErrDeserializeFailed+": %s\n", err)
		os.Exit(1)
	}

	// 反序列化
	cf, err := bytecode.DeserializeFromBytes(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, m.ErrDeserializeFailed+": %s\n", err)
		os.Exit(1)
	}

	// 设置源文件名（用于错误报告）
	cf.SourceFile = filepath.Base(filename)

	// 运行
	r := runtime.NewWithOptions(runtime.Options{
		JITEnabled: !jitless,
	})
	if err := r.RunCompiled(cf); err != nil {
		if err.Error() != "" {
			fmt.Fprintf(os.Stderr, m.ErrRuntime+"\n", err)
		}
		os.Exit(1)
	}
}

// cmdBuild 编译为字节码
func cmdBuild(args []string) {
	m := Msg()
	fs := flag.NewFlagSet("build", flag.ExitOnError)
	output := fs.String("o", "", m.OptOutput)

	fs.Usage = func() {
		fmt.Println(m.HelpUsage + " sola build [options] <file>")
		fmt.Println()
		fmt.Println(m.HelpOptions)
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	if fs.NArg() < 1 {
		fs.Usage()
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, m.ErrNoInput)
		os.Exit(1)
	}

	filename := fs.Arg(0)

	// 检查文件后缀
	if !strings.HasSuffix(filename, loader.SourceFileExtension) {
		fmt.Fprintf(os.Stderr, m.ErrInvalidSourceFile+"\n", filename, loader.SourceFileExtension)
		os.Exit(1)
	}

	// 读取源文件
	source, err := os.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, m.ErrReadFile+"\n", err)
		os.Exit(1)
	}

	// 编译
	r := runtime.New()
	cf, err := r.CompileToCompiledFile(string(source), filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, m.ErrCompileFailed+": %s\n", err)
		os.Exit(1)
	}

	// 确定输出文件名
	outputFile := *output
	if outputFile == "" {
		// 默认：将 .sola 替换为 .solac
		base := strings.TrimSuffix(filename, loader.SourceFileExtension)
		outputFile = base + bytecode.CompiledFileExtension
	}

	// 序列化
	data, err := bytecode.SerializeToBytes(cf)
	if err != nil {
		fmt.Fprintf(os.Stderr, m.ErrSerializeFailed+": %s\n", err)
		os.Exit(1)
	}

	// 写入文件
	if err := os.WriteFile(outputFile, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, m.ErrWriteFile+": %s\n", err)
		os.Exit(1)
	}

	// 获取文件大小
	fi, _ := os.Stat(outputFile)
	fmt.Printf(m.SuccessBuildComplete+"\n", outputFile, fi.Size())
}

// cmdJvm 编译为 JVM 字节码
func cmdJvm(args []string) {
	m := Msg()
	fs := flag.NewFlagSet("jvm", flag.ExitOnError)
	output := fs.String("o", "", m.OptOutput)
	className := fs.String("class", "Main", "Output class name")

	fs.Usage = func() {
		fmt.Println(m.HelpUsage + " sola jvm [options] <file>")
		fmt.Println()
		fmt.Println(m.HelpOptions)
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	if fs.NArg() < 1 {
		fs.Usage()
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, m.ErrNoInput)
		os.Exit(1)
	}

	filename := fs.Arg(0)

	// 读取源文件
	source, err := os.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, m.ErrReadFile+"\n", err)
		os.Exit(1)
	}

	// 解析源代码
	p := parser.New(string(source), filename)
	file := p.Parse()

	if p.HasErrors() {
		fmt.Println(m.ErrParser)
		for _, e := range p.Errors() {
			fmt.Printf("  %s\n", e)
		}
		os.Exit(1)
	}

	// 生成 JVM 字节码
	gen := jvmgen.NewGenerator(*className)
	classData, err := gen.Generate(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, m.ErrJvmGenFailed+": %s\n", err)
		os.Exit(1)
	}

	// 确定输出文件名
	outputFile := *output
	if outputFile == "" {
		outputFile = *className + ".class"
	}

	// 写入文件
	if err := os.WriteFile(outputFile, classData, 0644); err != nil {
		fmt.Fprintf(os.Stderr, m.ErrWriteFile+": %s\n", err)
		os.Exit(1)
	}

	// 获取文件大小
	fi, _ := os.Stat(outputFile)
	fmt.Printf(m.SuccessJvmComplete+"\n", outputFile, fi.Size())
}

// cmdCheck 语法检查
func cmdCheck(args []string) {
	m := Msg()
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	verbose := fs.Bool("v", false, m.OptVerbose)

	fs.Usage = func() {
		fmt.Println(m.HelpUsage + " sola check [options] <file>")
		fmt.Println()
		fmt.Println(m.HelpOptions)
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	if fs.NArg() < 1 {
		fs.Usage()
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, m.ErrNoInput)
		os.Exit(1)
	}

	filename := fs.Arg(0)
	source, err := os.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, m.ErrReadFile+"\n", err)
		os.Exit(1)
	}

	runParser(string(source), filename, *verbose)
}

// cmdFormat 格式化源代码
func cmdFormat(args []string) {
	m := Msg()
	fs := flag.NewFlagSet("format", flag.ExitOnError)
	write := fs.Bool("w", false, m.OptFormatWrite)
	check := fs.Bool("check", false, m.OptFormatCheck)
	indentStyle := fs.String("indent", "spaces", m.OptFormatIndent)
	indentSize := fs.Int("indent-size", 4, m.OptFormatIndentSize)
	maxLine := fs.Int("max-line", 100, m.OptFormatMaxLine)

	fs.Usage = func() {
		fmt.Println(m.HelpUsage + " sola format [options] <file>")
		fmt.Println()
		fmt.Println(m.HelpOptions)
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	if fs.NArg() < 1 {
		fs.Usage()
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, m.ErrNoInput)
		os.Exit(1)
	}

	filename := fs.Arg(0)

	// 读取源文件
	source, err := os.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, m.ErrReadFile+"\n", err)
		os.Exit(1)
	}

	// 设置格式化选项
	options := formatter.DefaultOptions()
	if *indentStyle == "tabs" {
		options.IndentStyle = "tabs"
	} else {
		options.IndentStyle = "spaces"
		options.IndentSize = *indentSize
	}
	options.MaxLineLength = *maxLine

	// 格式化
	formatted, err := formatter.Format(string(source), filename, options)
	if err != nil {
		fmt.Fprintf(os.Stderr, m.ErrFormatFailed+": %s\n", err)
		os.Exit(1)
	}

	// check 模式：检查是否已格式化
	if *check {
		if string(source) != formatted {
			fmt.Fprintf(os.Stderr, m.ErrFormatNotFormatted+"\n", filename)
			os.Exit(1)
		}
		fmt.Printf(m.SuccessFormatOK+"\n", filename)
		return
	}

	// 写入模式：覆盖原文件
	if *write {
		if err := os.WriteFile(filename, []byte(formatted), 0644); err != nil {
			fmt.Fprintf(os.Stderr, m.ErrWriteFile+": %s\n", err)
			os.Exit(1)
		}
		fmt.Printf(m.SuccessFormatComplete+"\n", filename)
		return
	}

	// 默认：输出到标准输出
	fmt.Print(formatted)
}

// cmdVersion 显示版本信息
func cmdVersion() {
	m := Msg()
	fmt.Printf(m.VersionTitle+"\n", Version)
	fmt.Println(m.VersionDesc)
}

// runLexer 运行词法分析器
func runLexer(source, filename string) {
	m := Msg()
	l := lexer.New(source, filename)
	tokens := l.ScanTokens()

	fmt.Println("=== Tokens ===")
	for _, tok := range tokens {
		fmt.Printf("  %s\n", tok)
	}
	fmt.Println()

	if l.HasErrors() {
		fmt.Println(m.ErrLexer)
		for _, e := range l.Errors() {
			fmt.Printf("  %s\n", e)
		}
		os.Exit(1)
	}
}

// runParser 运行解析器
func runParser(source, filename string, verbose bool) {
	m := Msg()
	p := parser.New(source, filename)
	file := p.Parse()

	if p.HasErrors() {
		fmt.Println(m.ErrParser)
		for _, e := range p.Errors() {
			fmt.Printf("  %s\n", e)
		}
		os.Exit(1)
	}

	if verbose {
		fmt.Println("=== AST ===")
		printAST(file)
	}

	fmt.Printf(m.SuccessSyntaxOK+"\n", filename)
	if verbose {
		fmt.Printf("  %s: %s\n", m.Namespace, getNamespace(file))
		fmt.Printf("  %s: %d\n", m.Uses, len(file.Uses))
		fmt.Printf("  %s: %d\n", m.Declarations, len(file.Declarations))
		fmt.Printf("  %s: %d\n", m.Statements, len(file.Statements))
	}
}

// runDisassemble 运行反汇编
func runDisassemble(source, filename string) {
	m := Msg()
	r := runtime.New()
	bytecode, err := r.Disassemble(source, filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, m.ErrRuntime+"\n", err)
		os.Exit(1)
	}
	fmt.Println(bytecode)
}

// getNamespace 获取命名空间
func getNamespace(file *ast.File) string {
	if file.Namespace != nil {
		return file.Namespace.Name
	}
	return "(default)"
}

// printAST 打印 AST
func printAST(file *ast.File) {
	if file.Namespace != nil {
		fmt.Printf("  Namespace: %s\n", file.Namespace.Name)
	}

	for _, use := range file.Uses {
		if use.Alias != nil {
			fmt.Printf("  Use: %s as %s\n", use.Path, use.Alias.Name)
		} else {
			fmt.Printf("  Use: %s\n", use.Path)
		}
	}

	for _, decl := range file.Declarations {
		fmt.Printf("  Declaration: %s\n", decl.String())
	}

	for i, stmt := range file.Statements {
		fmt.Printf("  Statement[%d]: %s\n", i, stmt.String())
	}
}
