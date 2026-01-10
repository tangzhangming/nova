package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/tangzhangming/nova/internal/lsp2"
)

const Version = "0.2.0"

func main() {
	// 解析命令行参数
	showVersion := flag.Bool("version", false, "显示版本信息")
	showHelp := flag.Bool("help", false, "显示帮助信息")
	logFile := flag.String("log", "", "日志文件路径（默认不记录日志，设置环境变量 SOLA_LSP_DEBUG=1 启用日志）")

	flag.Parse()

	if *showVersion {
		fmt.Printf("Sola Language Server v2 v%s\n", Version)
		os.Exit(0)
	}

	if *showHelp {
		printUsage()
		os.Exit(0)
	}

	// 创建并启动 LSP 服务器
	server := lsp2.NewServer(*logFile)
	ctx := context.Background()

	if err := server.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "LSP server error: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Sola Language Server v2 - 新一代 LSP 服务器")
	fmt.Println()
	fmt.Println("用法:")
	fmt.Println("  solals2 [options]")
	fmt.Println()
	fmt.Println("选项:")
	fmt.Println("  --version    显示版本信息")
	fmt.Println("  --help       显示帮助信息")
	fmt.Println("  --log <file> 日志文件路径")
	fmt.Println()
	fmt.Println("环境变量:")
	fmt.Println("  SOLA_LSP_DEBUG=1  启用调试日志（默认关闭）")
	fmt.Println()
	fmt.Println("特性:")
	fmt.Println("  - 类跳转：点击类名跳转到定义")
	fmt.Println("  - 静态方法跳转：Class::method 跳转到方法定义")
	fmt.Println("  - 实例方法跳转：$obj->method 跳转到方法定义")
	fmt.Println("  - 内存优化：LRU缓存，自动清理，防止内存泄漏")
	fmt.Println()
	fmt.Println("LSP 服务器通过标准输入输出 (stdio) 与编辑器通信。")
}
