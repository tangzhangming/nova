package vm

import (
	"github.com/tangzhangming/nova/internal/bytecode"
)

// ============================================================================
// B2. 热点检测框架 [JIT-PRE]
// ============================================================================

// HotspotThreshold 热点阈值
const (
	FunctionHotThreshold = 10000 // 函数调用 10000 次视为热点
	LoopHotThreshold     = 1000  // 循环迭代 1000 次视为热点
	BackedgeThreshold    = 100   // 回边执行 100 次开始计数
)

// HotspotState 热点状态
type HotspotState byte

const (
	HotspotCold     HotspotState = iota // 冷代码
	HotspotWarm                         // 温代码（接近热点）
	HotspotHot                          // 热点代码
	HotspotCompiled                     // 已编译（JIT）
)

// FunctionProfile 函数性能档案
type FunctionProfile struct {
	Function     *bytecode.Function // 函数对象
	CallCount    int64              // 调用次数
	TotalTime    int64              // 总执行时间（纳秒）
	State        HotspotState       // 热点状态
	LoopProfiles map[int]*LoopProfile // 循环档案（IP -> Profile）
	
	// 类型反馈（用于 JIT 优化）
	ArgTypes     [][]string // 每个参数见过的类型
	ReturnTypes  []string   // 返回值类型
}

// LoopProfile 循环性能档案
type LoopProfile struct {
	HeaderIP      int          // 循环头 IP
	BackedgeIP    int          // 回边 IP
	IterationCount int64       // 迭代次数
	State         HotspotState // 热点状态
	
	// 循环特征（用于优化）
	IsCountable   bool  // 是否是可计数循环
	EstimatedTrips int  // 估计的迭代次数
}

// HotspotDetector 热点检测器
type HotspotDetector struct {
	profiles        map[uintptr]*FunctionProfile // 函数档案
	loopCounters    map[loopKey]int64            // 循环计数器
	enabled         bool
	
	// 回调
	onFunctionHot   func(*FunctionProfile) // 函数变热时回调
	onLoopHot       func(*LoopProfile)     // 循环变热时回调
	
	// 统计
	totalFunctions  int64
	hotFunctions    int64
	totalLoops      int64
	hotLoops        int64
}

// loopKey 循环唯一标识
type loopKey struct {
	funcPtr  uintptr
	headerIP int
}

// NewHotspotDetector 创建热点检测器
func NewHotspotDetector() *HotspotDetector {
	return &HotspotDetector{
		profiles:     make(map[uintptr]*FunctionProfile),
		loopCounters: make(map[loopKey]int64),
		enabled:      true,
	}
}

// SetEnabled 启用/禁用热点检测
func (hd *HotspotDetector) SetEnabled(enabled bool) {
	hd.enabled = enabled
}

// OnFunctionHot 设置函数变热回调
func (hd *HotspotDetector) OnFunctionHot(callback func(*FunctionProfile)) {
	hd.onFunctionHot = callback
}

// OnLoopHot 设置循环变热回调
func (hd *HotspotDetector) OnLoopHot(callback func(*LoopProfile)) {
	hd.onLoopHot = callback
}

// RecordFunctionCall 记录函数调用
func (hd *HotspotDetector) RecordFunctionCall(fn *bytecode.Function) {
	if !hd.enabled || fn == nil {
		return
	}

	fnPtr := funcToPtr(fn)
	profile := hd.getOrCreateProfile(fnPtr, fn)
	profile.CallCount++

	// 检查是否变热
	if profile.State == HotspotCold && profile.CallCount >= FunctionHotThreshold/10 {
		profile.State = HotspotWarm
	}
	if profile.State == HotspotWarm && profile.CallCount >= FunctionHotThreshold {
		profile.State = HotspotHot
		hd.hotFunctions++
		if hd.onFunctionHot != nil {
			hd.onFunctionHot(profile)
		}
	}
}

// RecordLoopIteration 记录循环迭代（在回边处调用）
func (hd *HotspotDetector) RecordLoopIteration(fn *bytecode.Function, headerIP, backedgeIP int) {
	if !hd.enabled || fn == nil {
		return
	}

	fnPtr := funcToPtr(fn)
	key := loopKey{funcPtr: fnPtr, headerIP: headerIP}
	
	hd.loopCounters[key]++
	count := hd.loopCounters[key]

	// 只有达到一定次数才创建详细档案
	if count < BackedgeThreshold {
		return
	}

	// 获取或创建循环档案
	profile := hd.getOrCreateProfile(fnPtr, fn)
	loopProfile := hd.getOrCreateLoopProfile(profile, headerIP, backedgeIP)
	loopProfile.IterationCount = count

	// 检查是否变热
	if loopProfile.State == HotspotCold && count >= LoopHotThreshold/10 {
		loopProfile.State = HotspotWarm
	}
	if loopProfile.State == HotspotWarm && count >= LoopHotThreshold {
		loopProfile.State = HotspotHot
		hd.hotLoops++
		if hd.onLoopHot != nil {
			hd.onLoopHot(loopProfile)
		}
	}
}

// RecordArgType 记录参数类型（用于类型特化）
func (hd *HotspotDetector) RecordArgType(fn *bytecode.Function, argIndex int, typeName string) {
	if !hd.enabled || fn == nil {
		return
	}

	fnPtr := funcToPtr(fn)
	profile := hd.getOrCreateProfile(fnPtr, fn)

	// 确保数组足够大
	for len(profile.ArgTypes) <= argIndex {
		profile.ArgTypes = append(profile.ArgTypes, nil)
	}

	// 添加类型（如果不存在）
	for _, t := range profile.ArgTypes[argIndex] {
		if t == typeName {
			return
		}
	}
	profile.ArgTypes[argIndex] = append(profile.ArgTypes[argIndex], typeName)
}

// RecordReturnType 记录返回类型
func (hd *HotspotDetector) RecordReturnType(fn *bytecode.Function, typeName string) {
	if !hd.enabled || fn == nil {
		return
	}

	fnPtr := funcToPtr(fn)
	profile := hd.getOrCreateProfile(fnPtr, fn)

	// 添加类型（如果不存在）
	for _, t := range profile.ReturnTypes {
		if t == typeName {
			return
		}
	}
	profile.ReturnTypes = append(profile.ReturnTypes, typeName)
}

// getOrCreateProfile 获取或创建函数档案
func (hd *HotspotDetector) getOrCreateProfile(fnPtr uintptr, fn *bytecode.Function) *FunctionProfile {
	if profile, ok := hd.profiles[fnPtr]; ok {
		return profile
	}

	profile := &FunctionProfile{
		Function:     fn,
		State:        HotspotCold,
		LoopProfiles: make(map[int]*LoopProfile),
	}
	hd.profiles[fnPtr] = profile
	hd.totalFunctions++
	return profile
}

// getOrCreateLoopProfile 获取或创建循环档案
func (hd *HotspotDetector) getOrCreateLoopProfile(funcProfile *FunctionProfile, headerIP, backedgeIP int) *LoopProfile {
	if lp, ok := funcProfile.LoopProfiles[headerIP]; ok {
		return lp
	}

	lp := &LoopProfile{
		HeaderIP:   headerIP,
		BackedgeIP: backedgeIP,
		State:      HotspotCold,
	}
	funcProfile.LoopProfiles[headerIP] = lp
	hd.totalLoops++
	return lp
}

// GetHotFunctions 获取所有热点函数
func (hd *HotspotDetector) GetHotFunctions() []*FunctionProfile {
	var result []*FunctionProfile
	for _, profile := range hd.profiles {
		if profile.State == HotspotHot || profile.State == HotspotCompiled {
			result = append(result, profile)
		}
	}
	return result
}

// GetHotLoops 获取所有热点循环
func (hd *HotspotDetector) GetHotLoops() []*LoopProfile {
	var result []*LoopProfile
	for _, funcProfile := range hd.profiles {
		for _, loopProfile := range funcProfile.LoopProfiles {
			if loopProfile.State == HotspotHot || loopProfile.State == HotspotCompiled {
				result = append(result, loopProfile)
			}
		}
	}
	return result
}

// GetFunctionProfile 获取函数档案
func (hd *HotspotDetector) GetFunctionProfile(fn *bytecode.Function) *FunctionProfile {
	if fn == nil {
		return nil
	}
	fnPtr := funcToPtr(fn)
	return hd.profiles[fnPtr]
}

// IsHot 检查函数是否是热点
func (hd *HotspotDetector) IsHot(fn *bytecode.Function) bool {
	profile := hd.GetFunctionProfile(fn)
	if profile == nil {
		return false
	}
	return profile.State == HotspotHot || profile.State == HotspotCompiled
}

// IsLoopHot 检查循环是否是热点
func (hd *HotspotDetector) IsLoopHot(fn *bytecode.Function, headerIP int) bool {
	profile := hd.GetFunctionProfile(fn)
	if profile == nil {
		return false
	}
	lp, ok := profile.LoopProfiles[headerIP]
	if !ok {
		return false
	}
	return lp.State == HotspotHot || lp.State == HotspotCompiled
}

// MarkCompiled 标记函数已编译
func (hd *HotspotDetector) MarkCompiled(fn *bytecode.Function) {
	profile := hd.GetFunctionProfile(fn)
	if profile != nil {
		profile.State = HotspotCompiled
	}
}

// MarkLoopCompiled 标记循环已编译
func (hd *HotspotDetector) MarkLoopCompiled(fn *bytecode.Function, headerIP int) {
	profile := hd.GetFunctionProfile(fn)
	if profile == nil {
		return
	}
	if lp, ok := profile.LoopProfiles[headerIP]; ok {
		lp.State = HotspotCompiled
	}
}

// Reset 重置所有档案
func (hd *HotspotDetector) Reset() {
	hd.profiles = make(map[uintptr]*FunctionProfile)
	hd.loopCounters = make(map[loopKey]int64)
	hd.totalFunctions = 0
	hd.hotFunctions = 0
	hd.totalLoops = 0
	hd.hotLoops = 0
}

// Stats 获取统计信息
func (hd *HotspotDetector) Stats() HotspotStats {
	return HotspotStats{
		TotalFunctions: hd.totalFunctions,
		HotFunctions:   hd.hotFunctions,
		TotalLoops:     hd.totalLoops,
		HotLoops:       hd.hotLoops,
		Enabled:        hd.enabled,
	}
}

// HotspotStats 热点统计
type HotspotStats struct {
	TotalFunctions int64
	HotFunctions   int64
	TotalLoops     int64
	HotLoops       int64
	Enabled        bool
}

// funcToPtr 将函数转换为指针
func funcToPtr(fn *bytecode.Function) uintptr {
	if fn == nil {
		return 0
	}
	// 使用函数名的哈希作为简化标识
	// 实际实现可以使用 unsafe.Pointer
	h := uintptr(0)
	for _, c := range fn.Name {
		h = h*31 + uintptr(c)
	}
	return h
}

// ============================================================================
// 类型反馈收集
// ============================================================================

// TypeFeedback 类型反馈
type TypeFeedback struct {
	SeenTypes map[string]int64 // 见过的类型 -> 出现次数
}

// NewTypeFeedback 创建类型反馈
func NewTypeFeedback() *TypeFeedback {
	return &TypeFeedback{
		SeenTypes: make(map[string]int64),
	}
}

// Record 记录类型
func (tf *TypeFeedback) Record(typeName string) {
	tf.SeenTypes[typeName]++
}

// IsMonomorphic 是否是单态（只有一种类型）
func (tf *TypeFeedback) IsMonomorphic() bool {
	return len(tf.SeenTypes) == 1
}

// IsBimorphic 是否是双态（两种类型）
func (tf *TypeFeedback) IsBimorphic() bool {
	return len(tf.SeenTypes) == 2
}

// DominantType 获取主导类型（出现最多的类型）
func (tf *TypeFeedback) DominantType() (string, float64) {
	if len(tf.SeenTypes) == 0 {
		return "", 0
	}

	var total int64
	var maxType string
	var maxCount int64

	for t, count := range tf.SeenTypes {
		total += count
		if count > maxCount {
			maxCount = count
			maxType = t
		}
	}

	return maxType, float64(maxCount) / float64(total)
}

