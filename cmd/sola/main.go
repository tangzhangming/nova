package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/tangzhangming/nova/internal/ast"
	"github.com/tangzhangming/nova/internal/i18n"
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
	case "check":
		cmdCheck(args[1:])
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
	fmt.Printf("  run <file>      %s\n", m.CmdRun)
	fmt.Printf("  build <file>    %s\n", m.CmdBuild)
	fmt.Printf("  check <file>    %s\n", m.CmdCheck)
	fmt.Printf("  version         %s\n", m.CmdVersion)
	fmt.Printf("  help            %s\n", m.CmdHelp)
	fmt.Println()
	fmt.Println(m.HelpOptions)
	fmt.Printf("  -tokens         %s\n", m.OptTokens)
	fmt.Printf("  -ast            %s\n", m.OptAST)
	fmt.Printf("  -bytecode       %s\n", m.OptBytecode)
	fmt.Printf("  --lang <en|zh>  %s\n", m.OptLang)
	fmt.Println()
	fmt.Println(m.HelpExamples)
	fmt.Printf("  sola run main%s\n", loader.SourceFileExtension)
	fmt.Printf("  sola run -ast main%s\n", loader.SourceFileExtension)
	fmt.Printf("  sola check main%s\n", loader.SourceFileExtension)
	fmt.Printf("  sola --lang zh help\n")
}

// cmdRun 运行 Sola 源文件
func cmdRun(args []string) {
	m := Msg()
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	showTokens := fs.Bool("tokens", false, m.OptTokens)
	showAST := fs.Bool("ast", false, m.OptAST)
	showBytecode := fs.Bool("bytecode", false, m.OptBytecode)

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
	r := runtime.New()
	if err := r.Run(string(source), filename); err != nil {
		fmt.Fprintf(os.Stderr, m.ErrRuntime+"\n", err)
		os.Exit(1)
	}
}

// cmdBuild 编译为字节码（预留）
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
	_ = output // 暂未使用

	fmt.Printf(m.SuccessBuilding+"\n", filename)
	fmt.Println(m.NotImplemented)
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
