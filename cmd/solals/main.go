package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/tangzhangming/nova/internal/lsp"
)

const Version = "0.1.0"

func main() {
	// 解析命令行参数
	showVersion := flag.Bool("version", false, "显示版本信息")
	showHelp := flag.Bool("help", false, "显示帮助信息")
	logFile := flag.String("log", "", "日志文件路径（默认不记录日志）")

	flag.Parse()

	if *showVersion {
		fmt.Printf("Sola Language Server v%s\n", Version)
		os.Exit(0)
	}

	if *showHelp {
		printUsage()
		os.Exit(0)
	}

	// 创建并启动 LSP 服务器
	server := lsp.NewServer(*logFile)
	ctx := context.Background()

	if err := server.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "LSP server error: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Sola Language Server - LSP 服务器")
	fmt.Println()
	fmt.Println("用法:")
	fmt.Println("  solals [options]")
	fmt.Println()
	fmt.Println("选项:")
	fmt.Println("  --version    显示版本信息")
	fmt.Println("  --help       显示帮助信息")
	fmt.Println("  --log <file> 日志文件路径")
	fmt.Println()
	fmt.Println("LSP 服务器通过标准输入输出 (stdio) 与编辑器通信。")
	fmt.Println()
	fmt.Println("支持的编辑器:")
	fmt.Println("  - VS Code (需要安装 Sola 扩展)")
	fmt.Println("  - Sublime Text (需要安装 LSP 插件)")
	fmt.Println("  - Neovim (内置 LSP 支持)")
	fmt.Println("  - 任何支持 LSP 协议的编辑器")
}
