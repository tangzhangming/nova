package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/tangzhangming/nova/internal/ast"
	"github.com/tangzhangming/nova/internal/lexer"
	"github.com/tangzhangming/nova/internal/loader"
	"github.com/tangzhangming/nova/internal/parser"
	"github.com/tangzhangming/nova/internal/runtime"
)

const (
	Version = "0.1.0"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(0)
	}

	command := os.Args[1]

	switch command {
	case "run":
		cmdRun(os.Args[2:])
	case "build":
		cmdBuild(os.Args[2:])
	case "check":
		cmdCheck(os.Args[2:])
	case "version", "-v", "--version":
		cmdVersion()
	case "help", "-h", "--help":
		printUsage()
	default:
		// 兼容旧用法：直接运行文件
		if len(os.Args) >= 2 && !isFlag(os.Args[1]) {
			cmdRun(os.Args[1:])
		} else {
			fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", command)
			printUsage()
			os.Exit(1)
		}
	}
}

func isFlag(s string) bool {
	return len(s) > 0 && s[0] == '-'
}

func printUsage() {
	fmt.Printf("Sola Programming Language v%s\n\n", Version)
	fmt.Println("Usage:")
	fmt.Println("  sola <command> [options] [arguments]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  run <file>      Run a Sola source file")
	fmt.Println("  build <file>    Compile to bytecode (coming soon)")
	fmt.Println("  check <file>    Check syntax without running")
	fmt.Println("  version         Show version information")
	fmt.Println("  help            Show this help message")
	fmt.Println()
	fmt.Println("Run Options:")
	fmt.Println("  -tokens         Show lexer tokens")
	fmt.Println("  -ast            Show AST structure")
	fmt.Println("  -bytecode       Show compiled bytecode")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Printf("  sola run main%s\n", loader.SourceFileExtension)
	fmt.Printf("  sola run -ast main%s\n", loader.SourceFileExtension)
	fmt.Printf("  sola check main%s\n", loader.SourceFileExtension)
}

// cmdRun 运行 Sola 源文件
func cmdRun(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	showTokens := fs.Bool("tokens", false, "Show lexer tokens")
	showAST := fs.Bool("ast", false, "Show AST structure")
	showBytecode := fs.Bool("bytecode", false, "Show compiled bytecode")

	fs.Usage = func() {
		fmt.Println("Usage: sola run [options] <file>")
		fmt.Println()
		fmt.Println("Options:")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "Error: no input file specified")
		fs.Usage()
		os.Exit(1)
	}

	filename := fs.Arg(0)
	source, err := os.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
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
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// cmdBuild 编译为字节码（预留）
func cmdBuild(args []string) {
	fs := flag.NewFlagSet("build", flag.ExitOnError)
	output := fs.String("o", "", "Output file path")

	fs.Usage = func() {
		fmt.Println("Usage: sola build [options] <file>")
		fmt.Println()
		fmt.Println("Options:")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "Error: no input file specified")
		fs.Usage()
		os.Exit(1)
	}

	filename := fs.Arg(0)
	_ = output // 暂未使用

	fmt.Printf("Building %s...\n", filename)
	fmt.Println("Note: Build command is not yet implemented. Coming soon!")
}

// cmdCheck 语法检查
func cmdCheck(args []string) {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	verbose := fs.Bool("v", false, "Verbose output")

	fs.Usage = func() {
		fmt.Println("Usage: sola check [options] <file>")
		fmt.Println()
		fmt.Println("Options:")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "Error: no input file specified")
		fs.Usage()
		os.Exit(1)
	}

	filename := fs.Arg(0)
	source, err := os.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	runParser(string(source), filename, *verbose)
}

// cmdVersion 显示版本信息
func cmdVersion() {
	fmt.Printf("Sola Programming Language v%s\n", Version)
	fmt.Println("A statically-typed, compiled language with PHP-like syntax")
}

// runLexer 运行词法分析器
func runLexer(source, filename string) {
	l := lexer.New(source, filename)
	tokens := l.ScanTokens()

	fmt.Println("=== Tokens ===")
	for _, tok := range tokens {
		fmt.Printf("  %s\n", tok)
	}
	fmt.Println()

	if l.HasErrors() {
		fmt.Println("Lexer errors:")
		for _, e := range l.Errors() {
			fmt.Printf("  %s\n", e)
		}
		os.Exit(1)
	}
}

// runParser 运行解析器
func runParser(source, filename string, verbose bool) {
	p := parser.New(source, filename)
	file := p.Parse()

	if p.HasErrors() {
		fmt.Println("Parser errors:")
		for _, e := range p.Errors() {
			fmt.Printf("  %s\n", e)
		}
		os.Exit(1)
	}

	if verbose {
		fmt.Println("=== AST ===")
		printAST(file)
	}

	fmt.Printf("✓ %s: syntax OK\n", filename)
	if verbose {
		fmt.Printf("  Namespace: %s\n", getNamespace(file))
		fmt.Printf("  Uses: %d\n", len(file.Uses))
		fmt.Printf("  Declarations: %d\n", len(file.Declarations))
		fmt.Printf("  Statements: %d\n", len(file.Statements))
	}
}

// runDisassemble 运行反汇编
func runDisassemble(source, filename string) {
	r := runtime.New()
	bytecode, err := r.Disassemble(source, filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
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

