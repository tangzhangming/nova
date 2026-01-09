// solaprof - Sola 性能分析工具
//
// 用法:
//   solaprof run [options] script.sola    # 运行并分析脚本
//   solaprof analyze profile.json         # 分析已有的性能数据
//   solaprof report profile.json          # 生成性能报告
//   solaprof flamegraph profile.json      # 生成火焰图数据
//   solaprof diff profile1.json profile2.json  # 对比两次分析

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
)

// 版本信息
const (
	Version = "1.0.0"
	Name    = "solaprof"
)

// 命令行选项
var (
	// 通用选项
	helpFlag    = flag.Bool("help", false, "显示帮助信息")
	versionFlag = flag.Bool("version", false, "显示版本信息")
	verboseFlag = flag.Bool("verbose", false, "详细输出")
	outputFlag  = flag.String("o", "", "输出文件")
	
	// CPU 分析选项
	cpuFlag     = flag.Bool("cpu", false, "启用 CPU 分析")
	sampleRate  = flag.Int("rate", 100, "CPU 采样率 (Hz)")
	
	// 内存分析选项
	memFlag     = flag.Bool("mem", false, "启用内存分析")
	allocsFlag  = flag.Bool("allocs", false, "只追踪分配")
	
	// 报告选项
	topN        = flag.Int("top", 20, "显示前 N 个热点")
	formatFlag  = flag.String("format", "text", "输出格式: text, json, html")
)

func main() {
	flag.Usage = usage
	flag.Parse()
	
	if *helpFlag {
		usage()
		os.Exit(0)
	}
	
	if *versionFlag {
		fmt.Printf("%s version %s\n", Name, Version)
		os.Exit(0)
	}
	
	args := flag.Args()
	if len(args) == 0 {
		usage()
		os.Exit(1)
	}
	
	cmd := args[0]
	cmdArgs := args[1:]
	
	var err error
	switch cmd {
	case "run":
		err = runProfile(cmdArgs)
	case "analyze":
		err = analyzeProfile(cmdArgs)
	case "report":
		err = generateReport(cmdArgs)
	case "flamegraph":
		err = generateFlameGraph(cmdArgs)
	case "diff":
		err = diffProfiles(cmdArgs)
	case "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "未知命令: %s\n", cmd)
		usage()
		os.Exit(1)
	}
	
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `%s - Sola 性能分析工具 v%s

用法:
  %s <命令> [选项] [参数]

命令:
  run       运行并分析 Sola 脚本
  analyze   分析已保存的性能数据
  report    生成性能报告
  flamegraph 生成火焰图数据
  diff      对比两次分析结果
  help      显示帮助信息

选项:
`, Name, Version, Name)
	flag.PrintDefaults()
	fmt.Fprintf(os.Stderr, `
示例:
  # 运行脚本并进行 CPU 分析
  %s run --cpu script.sola

  # 运行脚本并进行内存分析
  %s run --mem script.sola

  # 同时进行 CPU 和内存分析
  %s run --cpu --mem script.sola

  # 生成报告
  %s report --format=html profile.json -o report.html

  # 生成火焰图
  %s flamegraph profile.json -o flame.svg

  # 对比两次分析
  %s diff baseline.json current.json
`, Name, Name, Name, Name, Name, Name)
}

// runProfile 运行并分析脚本
func runProfile(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("请指定要运行的脚本")
	}
	
	scriptPath := args[0]
	
	// 检查文件是否存在
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		return fmt.Errorf("脚本不存在: %s", scriptPath)
	}
	
	// 确定分析类型
	if !*cpuFlag && !*memFlag {
		// 默认启用 CPU 分析
		*cpuFlag = true
	}
	
	if *verboseFlag {
		fmt.Printf("分析脚本: %s\n", scriptPath)
		if *cpuFlag {
			fmt.Printf("  CPU 分析: 启用 (采样率: %d Hz)\n", *sampleRate)
		}
		if *memFlag {
			fmt.Printf("  内存分析: 启用\n")
		}
	}
	
	// 创建性能数据
	profile := &ProfileData{
		Version:    Version,
		ScriptPath: scriptPath,
		Options: ProfileOptions{
			CPUEnabled:  *cpuFlag,
			MemEnabled:  *memFlag,
			SampleRate:  *sampleRate,
		},
	}
	
	// TODO: 实际运行脚本并收集性能数据
	// 这需要集成到 Sola 运行时中
	
	fmt.Println("警告: solaprof run 命令尚未完全实现")
	fmt.Println("请使用 sola 命令运行脚本，并通过 --profile 选项收集性能数据")
	
	// 保存性能数据
	if *outputFlag != "" {
		if err := saveProfile(profile, *outputFlag); err != nil {
			return err
		}
		fmt.Printf("性能数据已保存到: %s\n", *outputFlag)
	}
	
	return nil
}

// analyzeProfile 分析性能数据
func analyzeProfile(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("请指定性能数据文件")
	}
	
	profilePath := args[0]
	profile, err := loadProfile(profilePath)
	if err != nil {
		return err
	}
	
	fmt.Printf("分析性能数据: %s\n\n", profilePath)
	
	// CPU 分析摘要
	if profile.CPUData != nil {
		printCPUSummary(profile.CPUData)
	}
	
	// 内存分析摘要
	if profile.MemData != nil {
		printMemorySummary(profile.MemData)
	}
	
	return nil
}

// generateReport 生成性能报告
func generateReport(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("请指定性能数据文件")
	}
	
	profilePath := args[0]
	profile, err := loadProfile(profilePath)
	if err != nil {
		return err
	}
	
	var output *os.File
	if *outputFlag != "" {
		output, err = os.Create(*outputFlag)
		if err != nil {
			return err
		}
		defer output.Close()
	} else {
		output = os.Stdout
	}
	
	switch *formatFlag {
	case "text":
		return generateTextReport(profile, output)
	case "json":
		return generateJSONReport(profile, output)
	case "html":
		return generateHTMLReport(profile, output)
	default:
		return fmt.Errorf("未知的输出格式: %s", *formatFlag)
	}
}

// generateFlameGraph 生成火焰图数据
func generateFlameGraph(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("请指定性能数据文件")
	}
	
	profilePath := args[0]
	profile, err := loadProfile(profilePath)
	if err != nil {
		return err
	}
	
	if profile.CPUData == nil || len(profile.CPUData.Stacks) == 0 {
		return fmt.Errorf("性能数据中没有 CPU 采样数据")
	}
	
	var output *os.File
	if *outputFlag != "" {
		output, err = os.Create(*outputFlag)
		if err != nil {
			return err
		}
		defer output.Close()
	} else {
		output = os.Stdout
	}
	
	// 输出折叠栈格式
	for _, stack := range profile.CPUData.Stacks {
		fmt.Fprintf(output, "%s %d\n", strings.Join(stack.Frames, ";"), stack.Count)
	}
	
	if *outputFlag != "" {
		fmt.Fprintf(os.Stderr, "火焰图数据已保存到: %s\n", *outputFlag)
		fmt.Fprintf(os.Stderr, "使用 flamegraph.pl 生成 SVG:\n")
		fmt.Fprintf(os.Stderr, "  flamegraph.pl %s > flame.svg\n", *outputFlag)
	}
	
	return nil
}

// diffProfiles 对比两次分析
func diffProfiles(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("请指定两个性能数据文件")
	}
	
	baseline, err := loadProfile(args[0])
	if err != nil {
		return fmt.Errorf("加载基准数据失败: %w", err)
	}
	
	current, err := loadProfile(args[1])
	if err != nil {
		return fmt.Errorf("加载当前数据失败: %w", err)
	}
	
	fmt.Printf("性能对比:\n")
	fmt.Printf("  基准: %s\n", args[0])
	fmt.Printf("  当前: %s\n\n", args[1])
	
	// CPU 对比
	if baseline.CPUData != nil && current.CPUData != nil {
		fmt.Printf("CPU 采样:\n")
		fmt.Printf("  基准采样数: %d\n", baseline.CPUData.TotalSamples)
		fmt.Printf("  当前采样数: %d\n", current.CPUData.TotalSamples)
		
		// 计算差异
		diff := float64(current.CPUData.TotalSamples-baseline.CPUData.TotalSamples) / float64(baseline.CPUData.TotalSamples) * 100
		sign := "+"
		if diff < 0 {
			sign = ""
		}
		fmt.Printf("  变化: %s%.1f%%\n\n", sign, diff)
	}
	
	// 内存对比
	if baseline.MemData != nil && current.MemData != nil {
		fmt.Printf("内存分配:\n")
		fmt.Printf("  基准总分配: %d bytes\n", baseline.MemData.TotalAllocated)
		fmt.Printf("  当前总分配: %d bytes\n", current.MemData.TotalAllocated)
		
		diff := float64(current.MemData.TotalAllocated-baseline.MemData.TotalAllocated) / float64(baseline.MemData.TotalAllocated) * 100
		sign := "+"
		if diff < 0 {
			sign = ""
		}
		fmt.Printf("  变化: %s%.1f%%\n", sign, diff)
	}
	
	return nil
}

// ============================================================================
// 数据结构
// ============================================================================

// ProfileData 性能数据
type ProfileData struct {
	Version    string          `json:"version"`
	ScriptPath string          `json:"script_path"`
	Options    ProfileOptions  `json:"options"`
	CPUData    *CPUProfileData `json:"cpu_data,omitempty"`
	MemData    *MemProfileData `json:"mem_data,omitempty"`
}

// ProfileOptions 分析选项
type ProfileOptions struct {
	CPUEnabled  bool `json:"cpu_enabled"`
	MemEnabled  bool `json:"mem_enabled"`
	SampleRate  int  `json:"sample_rate"`
}

// CPUProfileData CPU 分析数据
type CPUProfileData struct {
	TotalSamples int64             `json:"total_samples"`
	Duration     int64             `json:"duration_ns"`
	Functions    []FunctionStat    `json:"functions"`
	Stacks       []StackSample     `json:"stacks"`
}

// FunctionStat 函数统计
type FunctionStat struct {
	Name       string  `json:"name"`
	TotalPct   float64 `json:"total_pct"`
	SelfPct    float64 `json:"self_pct"`
	TotalTime  int64   `json:"total_time_ns"`
	SelfTime   int64   `json:"self_time_ns"`
	CallCount  int64   `json:"call_count"`
}

// StackSample 栈采样
type StackSample struct {
	Frames []string `json:"frames"`
	Count  int64    `json:"count"`
}

// MemProfileData 内存分析数据
type MemProfileData struct {
	TotalAllocated int64            `json:"total_allocated"`
	TotalFreed     int64            `json:"total_freed"`
	CurrentLive    int64            `json:"current_live"`
	Allocations    []AllocationStat `json:"allocations"`
}

// AllocationStat 分配统计
type AllocationStat struct {
	Type         string `json:"type"`
	Site         string `json:"site"`
	TotalBytes   int64  `json:"total_bytes"`
	LiveBytes    int64  `json:"live_bytes"`
	TotalCount   int64  `json:"total_count"`
	LiveCount    int64  `json:"live_count"`
}

// ============================================================================
// 辅助函数
// ============================================================================

func loadProfile(path string) (*ProfileData, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	
	var profile ProfileData
	if err := json.Unmarshal(data, &profile); err != nil {
		return nil, err
	}
	
	return &profile, nil
}

func saveProfile(profile *ProfileData, path string) error {
	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return err
	}
	
	return os.WriteFile(path, data, 0644)
}

func printCPUSummary(data *CPUProfileData) {
	fmt.Printf("=== CPU 分析摘要 ===\n")
	fmt.Printf("总采样数: %d\n", data.TotalSamples)
	fmt.Printf("分析时长: %.2f 秒\n\n", float64(data.Duration)/1e9)
	
	fmt.Printf("热点函数 (前 %d):\n", *topN)
	fmt.Printf("%-40s %10s %10s\n", "函数", "总时间%", "自身%")
	fmt.Printf("%s\n", strings.Repeat("-", 62))
	
	count := *topN
	if count > len(data.Functions) {
		count = len(data.Functions)
	}
	
	for i := 0; i < count; i++ {
		fn := data.Functions[i]
		fmt.Printf("%-40s %9.1f%% %9.1f%%\n",
			truncateName(fn.Name, 40), fn.TotalPct, fn.SelfPct)
	}
	fmt.Println()
}

func printMemorySummary(data *MemProfileData) {
	fmt.Printf("=== 内存分析摘要 ===\n")
	fmt.Printf("总分配: %s\n", formatBytes(data.TotalAllocated))
	fmt.Printf("总释放: %s\n", formatBytes(data.TotalFreed))
	fmt.Printf("当前存活: %s\n\n", formatBytes(data.CurrentLive))
	
	fmt.Printf("内存热点 (前 %d):\n", *topN)
	fmt.Printf("%-20s %-30s %12s %12s\n", "类型", "分配点", "总分配", "存活")
	fmt.Printf("%s\n", strings.Repeat("-", 76))
	
	count := *topN
	if count > len(data.Allocations) {
		count = len(data.Allocations)
	}
	
	for i := 0; i < count; i++ {
		alloc := data.Allocations[i]
		fmt.Printf("%-20s %-30s %12s %12s\n",
			truncateName(alloc.Type, 20),
			truncateName(alloc.Site, 30),
			formatBytes(alloc.TotalBytes),
			formatBytes(alloc.LiveBytes))
	}
	fmt.Println()
}

func generateTextReport(profile *ProfileData, output *os.File) error {
	fmt.Fprintf(output, "=== Sola 性能分析报告 ===\n\n")
	fmt.Fprintf(output, "脚本: %s\n", profile.ScriptPath)
	fmt.Fprintf(output, "版本: %s\n\n", profile.Version)
	
	if profile.CPUData != nil {
		fmt.Fprintf(output, "--- CPU 分析 ---\n\n")
		// 详细内容...
	}
	
	if profile.MemData != nil {
		fmt.Fprintf(output, "--- 内存分析 ---\n\n")
		// 详细内容...
	}
	
	return nil
}

func generateJSONReport(profile *ProfileData, output *os.File) error {
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	return encoder.Encode(profile)
}

func generateHTMLReport(profile *ProfileData, output *os.File) error {
	fmt.Fprintf(output, `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>Sola 性能分析报告</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; }
        h1 { color: #333; }
        table { border-collapse: collapse; width: 100%%; margin: 20px 0; }
        th, td { border: 1px solid #ddd; padding: 8px; text-align: left; }
        th { background-color: #4CAF50; color: white; }
        tr:nth-child(even) { background-color: #f2f2f2; }
        .section { margin: 30px 0; }
    </style>
</head>
<body>
    <h1>Sola 性能分析报告</h1>
    <p>脚本: %s</p>
`, profile.ScriptPath)

	if profile.CPUData != nil {
		fmt.Fprintf(output, `
    <div class="section">
        <h2>CPU 分析</h2>
        <p>总采样数: %d</p>
        <table>
            <tr><th>函数</th><th>总时间%%</th><th>自身%%</th></tr>
`, profile.CPUData.TotalSamples)
		
		for _, fn := range profile.CPUData.Functions {
			fmt.Fprintf(output, "            <tr><td>%s</td><td>%.1f%%</td><td>%.1f%%</td></tr>\n",
				fn.Name, fn.TotalPct, fn.SelfPct)
		}
		
		fmt.Fprintf(output, "        </table>\n    </div>\n")
	}
	
	fmt.Fprintf(output, "</body>\n</html>\n")
	return nil
}

func truncateName(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func formatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)
	
	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
