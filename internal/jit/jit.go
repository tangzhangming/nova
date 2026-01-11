// Package jit 提供 JIT 编译支持
// 当前为 stub 实现，JIT 功能待后续实现
package jit

import (
	"github.com/tangzhangming/nova/internal/bytecode"
)

// Config JIT 配置
type Config struct {
	Enabled           bool // 是否启用 JIT
	OptimizationLevel int  // 优化级别 (0-3)
	InlineThreshold   int  // 内联阈值
	HotspotThreshold  int  // 热点检测阈值
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		Enabled:           false, // 默认禁用（待实现）
		OptimizationLevel: 1,
		InlineThreshold:   50,
		HotspotThreshold:  1000,
	}
}

// JIT JIT 编译器
type JIT struct {
	config *Config
}

// New 创建 JIT 编译器
func New(config *Config) *JIT {
	if config == nil {
		config = DefaultConfig()
	}
	return &JIT{config: config}
}

// Compile 编译函数（stub）
func (j *JIT) Compile(fn *bytecode.Function) error {
	// TODO: 实现 JIT 编译
	return nil
}

// Execute 执行编译后的代码（stub）
func (j *JIT) Execute(fn *bytecode.Function, args []bytecode.Value) (bytecode.Value, error) {
	// TODO: 实现 JIT 执行
	return bytecode.NullValue, nil
}

// IsCompiled 检查函数是否已编译
func (j *JIT) IsCompiled(fn *bytecode.Function) bool {
	return false
}

// Stats JIT 统计信息
type Stats struct {
	CompiledFunctions int
	TotalCompileTime  int64
	CacheHits         int64
	CacheMisses       int64
}

// GetStats 获取统计信息
func (j *JIT) GetStats() Stats {
	return Stats{}
}

// Reset 重置 JIT 状态
func (j *JIT) Reset() {
	// TODO: 清理编译缓存
}
