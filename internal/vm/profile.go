package vm

import (
	"sync"
	"sync/atomic"

	"github.com/tangzhangming/nova/internal/bytecode"
)

// ============================================================================
// 类型 Profile
// ============================================================================

// TypeProfile 类型统计
type TypeProfile struct {
	IntCount    int64 // 整数类型次数
	FloatCount  int64 // 浮点数类型次数
	StringCount int64 // 字符串类型次数
	BoolCount   int64 // 布尔类型次数
	NullCount   int64 // null 类型次数
	ObjectCount int64 // 对象类型次数
	ArrayCount  int64 // 数组类型次数
	OtherCount  int64 // 其他类型次数
}

// Total 返回总次数
func (tp *TypeProfile) Total() int64 {
	return tp.IntCount + tp.FloatCount + tp.StringCount +
		tp.BoolCount + tp.NullCount + tp.ObjectCount +
		tp.ArrayCount + tp.OtherCount
}

// DominantType 返回占主导的类型
// 返回 ValNull 表示混合类型
func (tp *TypeProfile) DominantType() bytecode.ValueType {
	total := tp.Total()
	if total == 0 {
		return bytecode.ValNull
	}

	// 95% 以上认为是单一类型
	threshold := int64(float64(total) * 0.95)

	if tp.IntCount >= threshold {
		return bytecode.ValInt
	}
	if tp.FloatCount >= threshold {
		return bytecode.ValFloat
	}
	if tp.StringCount >= threshold {
		return bytecode.ValString
	}
	if tp.BoolCount >= threshold {
		return bytecode.ValBool
	}

	// 混合类型返回一个特殊值 (使用 255 表示未知)
	return bytecode.ValueType(255)
}

// IsMonomorphic 是否是单态 (95%+ 单一类型)
func (tp *TypeProfile) IsMonomorphic() bool {
	dt := tp.DominantType()
	return dt != bytecode.ValueType(255) && dt != bytecode.ValNull
}

// RecordType 记录类型
func (tp *TypeProfile) RecordType(t bytecode.ValueType) {
	switch t {
	case bytecode.ValInt:
		atomic.AddInt64(&tp.IntCount, 1)
	case bytecode.ValFloat:
		atomic.AddInt64(&tp.FloatCount, 1)
	case bytecode.ValString:
		atomic.AddInt64(&tp.StringCount, 1)
	case bytecode.ValBool:
		atomic.AddInt64(&tp.BoolCount, 1)
	case bytecode.ValNull:
		atomic.AddInt64(&tp.NullCount, 1)
	case bytecode.ValObject:
		atomic.AddInt64(&tp.ObjectCount, 1)
	case bytecode.ValArray, bytecode.ValSuperArray:
		atomic.AddInt64(&tp.ArrayCount, 1)
	default:
		atomic.AddInt64(&tp.OtherCount, 1)
	}
}

// ============================================================================
// 分支 Profile
// ============================================================================

// BranchProfile 分支统计
type BranchProfile struct {
	TakenCount    int64 // 跳转次数
	NotTakenCount int64 // 不跳转次数
}

// Total 返回总次数
func (bp *BranchProfile) Total() int64 {
	return bp.TakenCount + bp.NotTakenCount
}

// TakenRate 返回跳转率
func (bp *BranchProfile) TakenRate() float64 {
	total := bp.Total()
	if total == 0 {
		return 0.5
	}
	return float64(bp.TakenCount) / float64(total)
}

// IsBiased 是否有偏向 (90%+ 偏向一边)
func (bp *BranchProfile) IsBiased() bool {
	rate := bp.TakenRate()
	return rate > 0.9 || rate < 0.1
}

// RecordBranch 记录分支
func (bp *BranchProfile) RecordBranch(taken bool) {
	if taken {
		atomic.AddInt64(&bp.TakenCount, 1)
	} else {
		atomic.AddInt64(&bp.NotTakenCount, 1)
	}
}

// ============================================================================
// 函数 Profile
// ============================================================================

// FunctionProfile 函数级别的 Profile
type FunctionProfile struct {
	Name           string
	ExecutionCount int64 // 执行次数
	IsHot          bool  // 是否是热点

	// IP -> Profile 映射
	TypeProfiles   map[int]*TypeProfile   // 类型 Profile
	BranchProfiles map[int]*BranchProfile // 分支 Profile

	mu sync.RWMutex
}

// NewFunctionProfile 创建函数 Profile
func NewFunctionProfile(name string) *FunctionProfile {
	return &FunctionProfile{
		Name:           name,
		TypeProfiles:   make(map[int]*TypeProfile),
		BranchProfiles: make(map[int]*BranchProfile),
	}
}

// RecordExecution 记录执行
func (fp *FunctionProfile) RecordExecution() int64 {
	return atomic.AddInt64(&fp.ExecutionCount, 1)
}

// GetTypeProfile 获取指定 IP 的类型 Profile
func (fp *FunctionProfile) GetTypeProfile(ip int) *TypeProfile {
	fp.mu.RLock()
	tp := fp.TypeProfiles[ip]
	fp.mu.RUnlock()

	if tp == nil {
		fp.mu.Lock()
		// 双重检查
		if tp = fp.TypeProfiles[ip]; tp == nil {
			tp = &TypeProfile{}
			fp.TypeProfiles[ip] = tp
		}
		fp.mu.Unlock()
	}

	return tp
}

// GetBranchProfile 获取指定 IP 的分支 Profile
func (fp *FunctionProfile) GetBranchProfile(ip int) *BranchProfile {
	fp.mu.RLock()
	bp := fp.BranchProfiles[ip]
	fp.mu.RUnlock()

	if bp == nil {
		fp.mu.Lock()
		if bp = fp.BranchProfiles[ip]; bp == nil {
			bp = &BranchProfile{}
			fp.BranchProfiles[ip] = bp
		}
		fp.mu.Unlock()
	}

	return bp
}

// RecordType 记录类型
func (fp *FunctionProfile) RecordType(ip int, t bytecode.ValueType) {
	fp.GetTypeProfile(ip).RecordType(t)
}

// RecordBranch 记录分支
func (fp *FunctionProfile) RecordBranch(ip int, taken bool) {
	fp.GetBranchProfile(ip).RecordBranch(taken)
}

// ============================================================================
// Profile 管理器
// ============================================================================

// ProfileManager Profile 管理器
type ProfileManager struct {
	profiles map[*bytecode.Function]*FunctionProfile
	mu       sync.RWMutex

	// 配置
	hotThreshold int64 // 热点阈值
	enabled      bool  // 是否启用
}

// NewProfileManager 创建 Profile 管理器
func NewProfileManager() *ProfileManager {
	return &ProfileManager{
		profiles:     make(map[*bytecode.Function]*FunctionProfile),
		hotThreshold: 1000, // 默认 1000 次
		enabled:      true,
	}
}

// DefaultProfileManager 默认 Profile 管理器
var DefaultProfileManager = NewProfileManager()

// SetEnabled 设置是否启用
func (pm *ProfileManager) SetEnabled(enabled bool) {
	pm.enabled = enabled
}

// IsEnabled 是否启用
func (pm *ProfileManager) IsEnabled() bool {
	return pm.enabled
}

// SetHotThreshold 设置热点阈值
func (pm *ProfileManager) SetHotThreshold(threshold int64) {
	pm.hotThreshold = threshold
}

// GetProfile 获取函数 Profile
func (pm *ProfileManager) GetProfile(fn *bytecode.Function) *FunctionProfile {
	pm.mu.RLock()
	fp := pm.profiles[fn]
	pm.mu.RUnlock()

	if fp == nil {
		pm.mu.Lock()
		if fp = pm.profiles[fn]; fp == nil {
			fp = NewFunctionProfile(fn.Name)
			pm.profiles[fn] = fp
		}
		pm.mu.Unlock()
	}

	return fp
}

// RecordExecution 记录函数执行
// 返回是否是新的热点
func (pm *ProfileManager) RecordExecution(fn *bytecode.Function) bool {
	if !pm.enabled {
		return false
	}

	fp := pm.GetProfile(fn)
	count := fp.RecordExecution()

	// 检查是否达到热点阈值
	if !fp.IsHot && count >= pm.hotThreshold {
		fp.IsHot = true
		return true // 新的热点
	}

	return false
}

// RecordType 记录类型
func (pm *ProfileManager) RecordType(fn *bytecode.Function, ip int, t bytecode.ValueType) {
	if !pm.enabled {
		return
	}
	pm.GetProfile(fn).RecordType(ip, t)
}

// RecordBranch 记录分支
func (pm *ProfileManager) RecordBranch(fn *bytecode.Function, ip int, taken bool) {
	if !pm.enabled {
		return
	}
	pm.GetProfile(fn).RecordBranch(ip, taken)
}

// GetHotFunctions 获取所有热点函数
func (pm *ProfileManager) GetHotFunctions() []*FunctionProfile {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	var hot []*FunctionProfile
	for _, fp := range pm.profiles {
		if fp.IsHot {
			hot = append(hot, fp)
		}
	}
	return hot
}

// Reset 重置所有 Profile
func (pm *ProfileManager) Reset() {
	pm.mu.Lock()
	pm.profiles = make(map[*bytecode.Function]*FunctionProfile)
	pm.mu.Unlock()
}

// ============================================================================
// 全局函数 (便捷访问)
// ============================================================================

// RecordExecution 记录函数执行 (使用默认管理器)
func RecordExecution(fn *bytecode.Function) bool {
	return DefaultProfileManager.RecordExecution(fn)
}

// RecordType 记录类型 (使用默认管理器)
func RecordType(fn *bytecode.Function, ip int, t bytecode.ValueType) {
	DefaultProfileManager.RecordType(fn, ip, t)
}

// RecordBranch 记录分支 (使用默认管理器)
func RecordBranch(fn *bytecode.Function, ip int, taken bool) {
	DefaultProfileManager.RecordBranch(fn, ip, taken)
}

// GetProfile 获取函数 Profile (使用默认管理器)
func GetProfile(fn *bytecode.Function) *FunctionProfile {
	return DefaultProfileManager.GetProfile(fn)
}

// EnableProfiling 启用 Profile
func EnableProfiling() {
	DefaultProfileManager.SetEnabled(true)
}

// DisableProfiling 禁用 Profile
func DisableProfiling() {
	DefaultProfileManager.SetEnabled(false)
}

// ============================================================================
// Profile 辅助函数
// ============================================================================

// ShouldSpecializeForInt 是否应该为整数特化
func ShouldSpecializeForInt(fp *FunctionProfile, ip int) bool {
	tp := fp.TypeProfiles[ip]
	if tp == nil {
		return false
	}
	return tp.DominantType() == bytecode.ValInt
}

// ShouldSpecializeForFloat 是否应该为浮点数特化
func ShouldSpecializeForFloat(fp *FunctionProfile, ip int) bool {
	tp := fp.TypeProfiles[ip]
	if tp == nil {
		return false
	}
	return tp.DominantType() == bytecode.ValFloat
}

// GetBranchBias 获取分支偏向
// 返回: 1 = 偏向跳转, -1 = 偏向不跳转, 0 = 无偏向
func GetBranchBias(fp *FunctionProfile, ip int) int {
	bp := fp.BranchProfiles[ip]
	if bp == nil {
		return 0
	}

	rate := bp.TakenRate()
	if rate > 0.9 {
		return 1
	}
	if rate < 0.1 {
		return -1
	}
	return 0
}
