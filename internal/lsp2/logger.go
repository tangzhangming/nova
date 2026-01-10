package lsp2

import (
	"fmt"
	"os"
	"sync"
	"time"
)

// Logger 日志记录器
type Logger struct {
	enabled bool      // 是否启用日志（通过环境变量 SOLA_LSP_DEBUG 控制）
	file    *os.File  // 日志文件句柄
	mu      sync.Mutex
}

// NewLogger 创建日志记录器
// logPath: 日志文件路径，如果为空则不输出到文件
func NewLogger(logPath string) *Logger {
	// 检查环境变量
	debug := os.Getenv("SOLA_LSP_DEBUG")
	enabled := debug == "1" || debug == "true" || debug == "on"

	logger := &Logger{
		enabled: enabled,
	}

	// 如果指定了日志文件路径且启用了日志，则打开文件
	if enabled && logPath != "" {
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			// 如果打开文件失败，仍然可以继续，只是不输出到文件
			fmt.Fprintf(os.Stderr, "Failed to open log file %s: %v\n", logPath, err)
		} else {
			logger.file = f
		}
	}

	return logger
}

// Close 关闭日志记录器
func (l *Logger) Close() {
	if l.file != nil {
		l.file.Close()
		l.file = nil
	}
}

// Debug 记录调试信息（可被关闭）
func (l *Logger) Debug(format string, args ...interface{}) {
	if !l.enabled {
		return
	}
	l.log("DEBUG", format, args...)
}

// Info 记录一般信息（可被关闭）
func (l *Logger) Info(format string, args ...interface{}) {
	if !l.enabled {
		return
	}
	l.log("INFO", format, args...)
}

// Error 记录错误信息（始终输出）
func (l *Logger) Error(format string, args ...interface{}) {
	l.log("ERROR", format, args...)
}

// log 内部日志记录函数
func (l *Logger) log(level, format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// 格式化时间
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	
	// 格式化消息
	message := fmt.Sprintf(format, args...)
	
	// 构建日志行：[时间] [级别] 消息
	logLine := fmt.Sprintf("[%s] [%s] %s\n", timestamp, level, message)

	// 输出到文件
	if l.file != nil {
		l.file.WriteString(logLine)
	}

	// Error 级别也输出到 stderr
	if level == "ERROR" {
		fmt.Fprint(os.Stderr, logLine)
	}
}

// IsEnabled 返回日志是否启用
func (l *Logger) IsEnabled() bool {
	return l.enabled
}
