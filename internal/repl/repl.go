// repl.go - Sola REPL (Read-Eval-Print Loop)
//
// 提供交互式命令行界面，支持：
// - 多行输入（检测未完成的表达式）
// - 历史记录
// - 特殊命令（:help, :quit, :reset, :load）
// - 自动打印表达式结果
// - 错误友好显示

package repl

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/tangzhangming/nova/internal/runtime"
)

// REPL 交互式解释器
type REPL struct {
	runtime     *runtime.Runtime
	reader      *bufio.Reader
	writer      io.Writer
	history     []string
	historyPos  int
	multiline   bool
	buffer      strings.Builder
	bracketDepth int
	parenDepth   int
	braceDepth   int
	inString     bool
	promptPrimary   string
	promptContinue  string
}

// Config REPL 配置
type Config struct {
	JITEnabled    bool
	PromptPrimary   string
	PromptContinue  string
}

// DefaultConfig 默认配置
func DefaultConfig() Config {
	return Config{
		JITEnabled:    true,
		PromptPrimary:   ">>> ",
		PromptContinue:  "... ",
	}
}

// New 创建 REPL
func New(config Config) *REPL {
	return &REPL{
		runtime: runtime.NewWithOptions(runtime.Options{
			JITEnabled: config.JITEnabled,
		}),
		reader:          bufio.NewReader(os.Stdin),
		writer:          os.Stdout,
		history:         make([]string, 0),
		promptPrimary:   config.PromptPrimary,
		promptContinue:  config.PromptContinue,
	}
}

// Run 运行 REPL
func (r *REPL) Run() {
	r.printWelcome()

	for {
		prompt := r.promptPrimary
		if r.multiline {
			prompt = r.promptContinue
		}
		fmt.Fprint(r.writer, prompt)

		line, err := r.reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				fmt.Fprintln(r.writer, "\nBye!")
				return
			}
			fmt.Fprintf(r.writer, "Error reading input: %v\n", err)
			continue
		}

		line = strings.TrimRight(line, "\r\n")

		// 处理特殊命令
		if !r.multiline && strings.HasPrefix(line, ":") {
			if r.handleCommand(line) {
				continue
			}
		}

		// 添加到缓冲区
		if r.multiline {
			r.buffer.WriteString("\n")
		}
		r.buffer.WriteString(line)

		// 检查是否需要继续输入
		if r.needsMoreInput(r.buffer.String()) {
			r.multiline = true
			continue
		}

		// 执行输入
		input := r.buffer.String()
		r.buffer.Reset()
		r.multiline = false

		if strings.TrimSpace(input) == "" {
			continue
		}

		r.addHistory(input)
		r.execute(input)
	}
}

// printWelcome 打印欢迎信息
func (r *REPL) printWelcome() {
	fmt.Fprintln(r.writer, "Sola REPL v0.1.0")
	fmt.Fprintln(r.writer, "Type :help for help, :quit to exit")
	fmt.Fprintln(r.writer)
}

// handleCommand 处理特殊命令
func (r *REPL) handleCommand(line string) bool {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return false
	}

	cmd := strings.ToLower(parts[0])
	args := parts[1:]

	switch cmd {
	case ":help", ":h", ":?":
		r.printHelp()
		return true

	case ":quit", ":q", ":exit":
		fmt.Fprintln(r.writer, "Bye!")
		os.Exit(0)
		return true

	case ":reset", ":clear":
		r.reset()
		fmt.Fprintln(r.writer, "Environment reset.")
		return true

	case ":load", ":l":
		if len(args) < 1 {
			fmt.Fprintln(r.writer, "Usage: :load <filename>")
			return true
		}
		r.loadFile(args[0])
		return true

	case ":history", ":hist":
		r.printHistory()
		return true

	case ":env":
		r.printEnv()
		return true

	default:
		fmt.Fprintf(r.writer, "Unknown command: %s\n", cmd)
		fmt.Fprintln(r.writer, "Type :help for available commands.")
		return true
	}
}

// printHelp 打印帮助信息
func (r *REPL) printHelp() {
	fmt.Fprintln(r.writer, "Available commands:")
	fmt.Fprintln(r.writer, "  :help, :h, :?     Show this help message")
	fmt.Fprintln(r.writer, "  :quit, :q, :exit  Exit the REPL")
	fmt.Fprintln(r.writer, "  :reset, :clear    Reset the environment")
	fmt.Fprintln(r.writer, "  :load <file>      Load and execute a file")
	fmt.Fprintln(r.writer, "  :history, :hist   Show command history")
	fmt.Fprintln(r.writer, "  :env              Show defined variables")
	fmt.Fprintln(r.writer)
	fmt.Fprintln(r.writer, "Multi-line input:")
	fmt.Fprintln(r.writer, "  Unfinished expressions (open brackets/parens)")
	fmt.Fprintln(r.writer, "  will continue on the next line.")
	fmt.Fprintln(r.writer)
	fmt.Fprintln(r.writer, "Examples:")
	fmt.Fprintln(r.writer, "  >>> let x = 10")
	fmt.Fprintln(r.writer, "  >>> print(x * 2)")
	fmt.Fprintln(r.writer, "  >>> fn add(a, b) {")
	fmt.Fprintln(r.writer, "  ...   return a + b")
	fmt.Fprintln(r.writer, "  ... }")
}

// reset 重置环境
func (r *REPL) reset() {
	r.runtime = runtime.NewWithOptions(runtime.Options{
		JITEnabled: true,
	})
	r.buffer.Reset()
	r.multiline = false
}

// loadFile 加载并执行文件
func (r *REPL) loadFile(filename string) {
	source, err := os.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(r.writer, "Error loading file: %v\n", err)
		return
	}

	if err := r.runtime.Run(string(source), filename); err != nil {
		fmt.Fprintf(r.writer, "Error: %v\n", err)
	} else {
		fmt.Fprintf(r.writer, "Loaded: %s\n", filename)
	}
}

// printHistory 打印历史记录
func (r *REPL) printHistory() {
	for i, cmd := range r.history {
		fmt.Fprintf(r.writer, "%4d  %s\n", i+1, cmd)
	}
}

// printEnv 打印环境变量
func (r *REPL) printEnv() {
	fmt.Fprintln(r.writer, "Environment variables are internal to the VM.")
	fmt.Fprintln(r.writer, "Use expressions to inspect values, e.g.: print(x)")
}

// addHistory 添加到历史记录
func (r *REPL) addHistory(input string) {
	// 不添加重复的历史记录
	if len(r.history) > 0 && r.history[len(r.history)-1] == input {
		return
	}
	r.history = append(r.history, input)
	// 限制历史记录大小
	if len(r.history) > 1000 {
		r.history = r.history[len(r.history)-1000:]
	}
}

// needsMoreInput 检查是否需要更多输入
func (r *REPL) needsMoreInput(input string) bool {
	// 统计括号深度
	braceDepth := 0  // {}
	parenDepth := 0  // ()
	bracketDepth := 0 // []
	inString := false
	stringChar := byte(0)
	escaped := false

	for i := 0; i < len(input); i++ {
		c := input[i]

		if escaped {
			escaped = false
			continue
		}

		if c == '\\' && inString {
			escaped = true
			continue
		}

		if inString {
			if c == stringChar {
				inString = false
			}
			continue
		}

		switch c {
		case '"', '\'', '`':
			inString = true
			stringChar = c
		case '{':
			braceDepth++
		case '}':
			braceDepth--
		case '(':
			parenDepth++
		case ')':
			parenDepth--
		case '[':
			bracketDepth++
		case ']':
			bracketDepth--
		}
	}

	// 如果括号未闭合或在字符串中，需要更多输入
	return braceDepth > 0 || parenDepth > 0 || bracketDepth > 0 || inString
}

// execute 执行输入
func (r *REPL) execute(input string) {
	// 尝试执行
	err := r.runtime.RunREPL(input, "<repl>")
	if err != nil {
		fmt.Fprintf(r.writer, "Error: %v\n", err)
	}
}

// GetCompletions 获取补全建议（供外部使用）
func (r *REPL) GetCompletions(prefix string) []string {
	completions := make([]string, 0)
	
	// 内置关键字
	keywords := []string{
		"let", "const", "fn", "return", "if", "else", "for", "while",
		"break", "continue", "class", "new", "this", "true", "false",
		"nil", "print", "println", "len", "type", "str", "int", "float",
		"import", "export", "async", "await", "try", "catch", "throw",
	}

	for _, kw := range keywords {
		if strings.HasPrefix(kw, prefix) {
			completions = append(completions, kw)
		}
	}

	// 特殊命令补全
	commands := []string{":help", ":quit", ":reset", ":load", ":history", ":env"}
	if strings.HasPrefix(prefix, ":") {
		for _, cmd := range commands {
			if strings.HasPrefix(cmd, prefix) {
				completions = append(completions, cmd)
			}
		}
	}

	return completions
}
