package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/tangzhangming/nova/internal/ast"
	"github.com/tangzhangming/nova/internal/lexer"
	"github.com/tangzhangming/nova/internal/parser"
	"github.com/tangzhangming/nova/internal/runtime"
)

var (
	showTokens = flag.Bool("tokens", false, "Show lexer tokens")
	showAST    = flag.Bool("ast", false, "Show AST structure")
	showBytecode = flag.Bool("bytecode", false, "Show bytecode")
	parseOnly  = flag.Bool("parse", false, "Parse only, don't run")
)

func main() {
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Println("Nova Programming Language v0.1.0")
		fmt.Println()
		fmt.Println("Usage: nova [options] <filename.nova>")
		fmt.Println()
		fmt.Println("Options:")
		fmt.Println("  -tokens     Show lexer tokens")
		fmt.Println("  -ast        Show AST structure")
		fmt.Println("  -bytecode   Show compiled bytecode")
		fmt.Println("  -parse      Parse only, don't run")
		os.Exit(0)
	}

	filename := flag.Arg(0)
	source, err := os.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	// 词法分析
	if *showTokens {
		l := lexer.New(string(source), filename)
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
		return
	}

	// 只解析模式
	if *parseOnly || *showAST {
		p := parser.New(string(source), filename)
		file := p.Parse()

		if p.HasErrors() {
			fmt.Println("Parser errors:")
			for _, e := range p.Errors() {
				fmt.Printf("  %s\n", e)
			}
			os.Exit(1)
		}

		if *showAST {
			fmt.Println("=== AST ===")
			printAST(file)
		}

		fmt.Printf("Successfully parsed %s\n", filename)
		fmt.Printf("  Namespace: %s\n", getNamespace(file))
		fmt.Printf("  Uses: %d\n", len(file.Uses))
		fmt.Printf("  Declarations: %d\n", len(file.Declarations))
		fmt.Printf("  Statements: %d\n", len(file.Statements))
		return
	}

	// 显示字节码
	if *showBytecode {
		r := runtime.New()
		bytecode, err := r.Disassemble(string(source), filename)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(bytecode)
		return
	}

	// 运行
	r := runtime.New()
	if err := r.Run(string(source), filename); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func getNamespace(file *ast.File) string {
	if file.Namespace != nil {
		return file.Namespace.Name
	}
	return "(default)"
}

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
