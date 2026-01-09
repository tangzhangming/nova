package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tangzhangming/nova/internal/pkg"
)

// cmdInit 初始化新项目
func cmdInit(args []string) {
	m := Msg()
	fs := flag.NewFlagSet("init", flag.ExitOnError)

	// 可选参数
	name := fs.String("name", "", m.InitOptName)
	namespace := fs.String("namespace", "", m.InitOptNamespace)

	fs.Usage = func() {
		fmt.Println(m.HelpUsage + " sola init [options]")
		fmt.Println()
		fmt.Println(m.InitDesc)
		fmt.Println()
		fmt.Println(m.HelpOptions)
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	// 获取当前目录
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, m.ErrGetWorkDir+"\n", err)
		os.Exit(1)
	}

	// 检查是否已存在配置文件
	configPath := filepath.Join(dir, pkg.ConfigFileName)
	if _, err := os.Stat(configPath); err == nil {
		fmt.Fprintf(os.Stderr, m.ErrConfigExists+"\n", pkg.ConfigFileName)
		os.Exit(1)
	}

	// 生成默认配置
	config := pkg.GenerateDefault(dir)

	// 应用命令行参数
	if *name != "" {
		config.Package.Name = *name
	}
	if *namespace != "" {
		config.Package.Namespace = *namespace
	}

	// 保存配置文件
	fmt.Printf(m.InitCreating+"\n", pkg.ConfigFileName)
	if err := config.Save(configPath); err != nil {
		fmt.Fprintf(os.Stderr, m.ErrCreateConfig+"\n", err)
		os.Exit(1)
	}

	// 创建 src 目录
	srcDir := filepath.Join(dir, "src")
	if _, err := os.Stat(srcDir); os.IsNotExist(err) {
		fmt.Printf(m.InitCreating+"\n", "src/")
		if err := os.MkdirAll(srcDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, m.ErrCreateDir+"\n", err)
			os.Exit(1)
		}
	}

	// 创建 main.sola 模板
	mainPath := filepath.Join(srcDir, "main.sola")
	if _, err := os.Stat(mainPath); os.IsNotExist(err) {
		fmt.Printf(m.InitCreating+"\n", "src/main.sola")
		mainContent := generateMainTemplate(config.Package.Namespace)
		if err := os.WriteFile(mainPath, []byte(mainContent), 0644); err != nil {
			fmt.Fprintf(os.Stderr, m.ErrCreateFile+"\n", err)
			os.Exit(1)
		}
	}

	// 打印成功信息
	fmt.Println()
	fmt.Printf(m.InitSuccess+"\n", config.Package.Name)
	fmt.Println()
	fmt.Println(m.InitNextSteps)
	fmt.Printf("  sola run src/main.sola\n")
}

// generateMainTemplate 生成 main.sola 模板
func generateMainTemplate(namespace string) string {
	return fmt.Sprintf(`namespace %s

// 应用程序入口
echo "Hello, Sola!";
`, namespace)
}
