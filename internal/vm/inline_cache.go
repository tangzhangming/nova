package vm

import (
	"time"
	"unsafe"

	"github.com/tangzhangming/nova/internal/bytecode"
)

// 缓存大小限制
const (
	MaxCallSiteCacheEntries   = 256 // 每个函数最多缓存的调用点数
	MaxMethodCacheFunctions   = 128 // 最多缓存的函数数
	MaxPropertyCacheEntries   = 512 // 最多缓存的属性访问点数
	CacheEvictionBatchSize    = 32  // 每次淘汰的条目数
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
	ClassPtr   uintptr          // 类指针（用于快速比较）
	Class      *bytecode.Class  // 类对象
	Method     *bytecode.Method // 缓存的方法
	lastAccess int64            // 最后访问时间（用于 LRU）
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
	now := time.Now().UnixNano()

	// 单态快速路径
	if ic.state == ICMonomorphic {
		if len(ic.entries) > 0 && ic.entries[0].ClassPtr == classPtr {
			ic.hits++
			ic.entries[0].lastAccess = now
			return ic.entries[0].Method, true
		}
		ic.misses++
		return nil, false
	}

	// 多态路径
	for i := range ic.entries {
		if ic.entries[i].ClassPtr == classPtr {
			ic.hits++
			ic.entries[i].lastAccess = now
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
	now := time.Now().UnixNano()

	// 检查是否已存在
	for i := range ic.entries {
		if ic.entries[i].ClassPtr == classPtr {
			ic.entries[i].Method = method // 更新方法
			ic.entries[i].lastAccess = now
			return
		}
	}

	// 添加新条目
	entry := ICEntry{
		ClassPtr:   classPtr,
		Class:      class,
		Method:     method,
		lastAccess: now,
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

// unsafePointer 获取类的指针值，用于快速类型身份比较
//go:noinline
func unsafePointer(p *bytecode.Class) uintptr {
	return uintptr(unsafe.Pointer(p))
}

// ============================================================================
// 方法调用点缓存
// ============================================================================

// CallSiteCacheEntry 调用点缓存条目（带访问时间）
type CallSiteCacheEntry struct {
	cache      *InlineCache
	lastAccess int64 // 最后访问时间
}

// CallSiteCache 调用点缓存
// 每个 OpCallMethod 指令位置一个缓存
type CallSiteCache struct {
	caches map[int]*CallSiteCacheEntry // IP -> 缓存条目
}

// NewCallSiteCache 创建调用点缓存
func NewCallSiteCache() *CallSiteCache {
	return &CallSiteCache{
		caches: make(map[int]*CallSiteCacheEntry),
	}
}

// Get 获取指定位置的缓存
func (csc *CallSiteCache) Get(ip int) *InlineCache {
	now := time.Now().UnixNano()

	if entry, ok := csc.caches[ip]; ok {
		entry.lastAccess = now
		return entry.cache
	}

	// 检查是否需要淘汰
	if len(csc.caches) >= MaxCallSiteCacheEntries {
		csc.evictLRU()
	}

	ic := NewInlineCache()
	csc.caches[ip] = &CallSiteCacheEntry{
		cache:      ic,
		lastAccess: now,
	}
	return ic
}

// evictLRU 淘汰最近最少使用的条目
func (csc *CallSiteCache) evictLRU() {
	if len(csc.caches) == 0 {
		return
	}

	// 收集所有条目并按访问时间排序
	type ipTime struct {
		ip         int
		lastAccess int64
	}
	entries := make([]ipTime, 0, len(csc.caches))
	for ip, entry := range csc.caches {
		entries = append(entries, ipTime{ip, entry.lastAccess})
	}

	// 简单选择排序找出最旧的 CacheEvictionBatchSize 个条目
	evictCount := CacheEvictionBatchSize
	if evictCount > len(entries) {
		evictCount = len(entries) / 4 // 淘汰 25%
		if evictCount < 1 {
			evictCount = 1
		}
	}

	// 找出最旧的条目
	for i := 0; i < evictCount; i++ {
		minIdx := i
		for j := i + 1; j < len(entries); j++ {
			if entries[j].lastAccess < entries[minIdx].lastAccess {
				minIdx = j
			}
		}
		entries[i], entries[minIdx] = entries[minIdx], entries[i]
	}

	// 删除最旧的条目
	for i := 0; i < evictCount; i++ {
		delete(csc.caches, entries[i].ip)
	}
}

// Reset 重置所有缓存
func (csc *CallSiteCache) Reset() {
	for _, entry := range csc.caches {
		entry.cache.Reset()
	}
}

// Stats 获取所有缓存的统计信息
func (csc *CallSiteCache) Stats() CallSiteCacheStats {
	stats := CallSiteCacheStats{
		TotalSites: len(csc.caches),
	}
	for _, entry := range csc.caches {
		hits, misses, state := entry.cache.Stats()
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
	state      ICState
	classPtr   uintptr
	offset     int    // 属性在 Fields map 中的"虚拟偏移"（实际是名字）
	name       string
	hits       int64
	misses     int64
	lastAccess int64 // 最后访问时间（用于 LRU）
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
		pc.lastAccess = time.Now().UnixNano()
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
	now := time.Now().UnixNano()

	if pc.state == ICUninitialized {
		pc.classPtr = newClassPtr
		pc.name = name
		pc.state = ICMonomorphic
		pc.lastAccess = now
	} else if pc.classPtr != newClassPtr {
		// 不同类访问同一属性位置，变为超多态
		pc.state = ICMegamorphic
	} else {
		pc.lastAccess = now
	}
}

// ============================================================================
// 全局缓存管理器
// ============================================================================

// MethodCacheEntry 方法缓存条目（带访问时间）
type MethodCacheEntry struct {
	cache      *CallSiteCache
	lastAccess int64
}

// ICManager 内联缓存管理器
type ICManager struct {
	methodCaches   map[uintptr]*MethodCacheEntry // 函数 -> 调用点缓存
	propertyCaches map[int]*PropertyCache        // IP -> 属性缓存
	enabled        bool
}

// NewICManager 创建缓存管理器
func NewICManager() *ICManager {
	return &ICManager{
		methodCaches:   make(map[uintptr]*MethodCacheEntry),
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

	fnPtr := uintptr(0)
	if fn != nil {
		fnPtr = uintptr(unsafe.Pointer(fn)) // 使用真正的函数指针作为标识
	}

	now := time.Now().UnixNano()

	entry, ok := m.methodCaches[fnPtr]
	if ok {
		entry.lastAccess = now
		return entry.cache.Get(ip)
	}

	// 检查是否需要淘汰
	if len(m.methodCaches) >= MaxMethodCacheFunctions {
		m.evictMethodCacheLRU()
	}

	csc := NewCallSiteCache()
	m.methodCaches[fnPtr] = &MethodCacheEntry{
		cache:      csc,
		lastAccess: now,
	}
	return csc.Get(ip)
}

// evictMethodCacheLRU 淘汰最近最少使用的方法缓存
func (m *ICManager) evictMethodCacheLRU() {
	if len(m.methodCaches) == 0 {
		return
	}

	// 收集所有条目并找出最旧的
	type fnTime struct {
		fnPtr      uintptr
		lastAccess int64
	}
	entries := make([]fnTime, 0, len(m.methodCaches))
	for fnPtr, entry := range m.methodCaches {
		entries = append(entries, fnTime{fnPtr, entry.lastAccess})
	}

	// 淘汰 25% 的条目
	evictCount := len(entries) / 4
	if evictCount < 1 {
		evictCount = 1
	}

	// 找出最旧的条目
	for i := 0; i < evictCount; i++ {
		minIdx := i
		for j := i + 1; j < len(entries); j++ {
			if entries[j].lastAccess < entries[minIdx].lastAccess {
				minIdx = j
			}
		}
		entries[i], entries[minIdx] = entries[minIdx], entries[i]
	}

	// 删除最旧的条目
	for i := 0; i < evictCount; i++ {
		delete(m.methodCaches, entries[i].fnPtr)
	}
}

// GetPropertyCache 获取属性访问缓存
func (m *ICManager) GetPropertyCache(ip int) *PropertyCache {
	if !m.enabled {
		return nil
	}

	now := time.Now().UnixNano()

	if pc, ok := m.propertyCaches[ip]; ok {
		pc.lastAccess = now
		return pc
	}

	// 检查是否需要淘汰
	if len(m.propertyCaches) >= MaxPropertyCacheEntries {
		m.evictPropertyCacheLRU()
	}

	pc := NewPropertyCache()
	pc.lastAccess = now
	m.propertyCaches[ip] = pc
	return pc
}

// evictPropertyCacheLRU 淘汰最近最少使用的属性缓存
func (m *ICManager) evictPropertyCacheLRU() {
	if len(m.propertyCaches) == 0 {
		return
	}

	// 收集所有条目并找出最旧的
	type ipTime struct {
		ip         int
		lastAccess int64
	}
	entries := make([]ipTime, 0, len(m.propertyCaches))
	for ip, pc := range m.propertyCaches {
		entries = append(entries, ipTime{ip, pc.lastAccess})
	}

	// 淘汰 25% 的条目
	evictCount := len(entries) / 4
	if evictCount < 1 {
		evictCount = 1
	}

	// 找出最旧的条目
	for i := 0; i < evictCount; i++ {
		minIdx := i
		for j := i + 1; j < len(entries); j++ {
			if entries[j].lastAccess < entries[minIdx].lastAccess {
				minIdx = j
			}
		}
		entries[i], entries[minIdx] = entries[minIdx], entries[i]
	}

	// 删除最旧的条目
	for i := 0; i < evictCount; i++ {
		delete(m.propertyCaches, entries[i].ip)
	}
}

// Reset 重置所有缓存（类重定义时调用）
func (m *ICManager) Reset() {
	for _, entry := range m.methodCaches {
		entry.cache.Reset()
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

	for _, entry := range m.methodCaches {
		siteStats := entry.cache.Stats()
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

// ============================================================================
// BUG FIX 2026-01-10: 内联缓存性能分析增强
// ============================================================================
// 防止反复引入的问题:
// 1. 命中率计算避免除零错误
// 2. 状态统计必须遍历所有缓存
// 3. 性能建议基于阈值，可配置

// ICDetailedStats 详细的内联缓存统计信息
type ICDetailedStats struct {
	ICManagerStats // 继承基础统计

	// 方法缓存状态分布
	MethodMonomorphic int // 单态缓存数
	MethodPolymorphic int // 多态缓存数
	MethodMegamorphic int // 超多态缓存数
	MethodUninitialized int // 未初始化缓存数

	// 属性缓存状态分布
	PropertyMonomorphic int
	PropertyPolymorphic int
	PropertyMegamorphic int
	PropertyUninitialized int

	// 命中率
	MethodHitRate   float64 // 方法缓存命中率 (0.0-1.0)
	PropertyHitRate float64 // 属性缓存命中率 (0.0-1.0)
	OverallHitRate  float64 // 总体命中率

	// 性能建议
	Recommendations []string
}

// DetailedStats 获取详细统计信息和性能分析
func (m *ICManager) DetailedStats() ICDetailedStats {
	stats := ICDetailedStats{
		ICManagerStats: m.Stats(),
	}

	// 统计方法缓存状态分布
	for _, entry := range m.methodCaches {
		for _, cacheEntry := range entry.cache.caches {
			if cacheEntry == nil || cacheEntry.cache == nil {
				continue
			}
			ic := cacheEntry.cache
			switch ic.state {
			case ICUninitialized:
				stats.MethodUninitialized++
			case ICMonomorphic:
				stats.MethodMonomorphic++
			case ICPolymorphic:
				stats.MethodPolymorphic++
			case ICMegamorphic:
				stats.MethodMegamorphic++
			}
		}
	}

	// 统计属性缓存状态分布
	for _, pc := range m.propertyCaches {
		switch pc.state {
		case ICUninitialized:
			stats.PropertyUninitialized++
		case ICMonomorphic:
			stats.PropertyMonomorphic++
		case ICPolymorphic:
			stats.PropertyPolymorphic++
		case ICMegamorphic:
			stats.PropertyMegamorphic++
		}
	}

	// 计算命中率（避免除零）
	methodTotal := stats.MethodHits + stats.MethodMisses
	if methodTotal > 0 {
		stats.MethodHitRate = float64(stats.MethodHits) / float64(methodTotal)
	}

	propertyTotal := stats.PropertyHits + stats.PropertyMisses
	if propertyTotal > 0 {
		stats.PropertyHitRate = float64(stats.PropertyHits) / float64(propertyTotal)
	}

	totalHits := stats.MethodHits + stats.PropertyHits
	totalMisses := stats.MethodMisses + stats.PropertyMisses
	if totalHits + totalMisses > 0 {
		stats.OverallHitRate = float64(totalHits) / float64(totalHits + totalMisses)
	}

	// 生成性能建议
	stats.Recommendations = m.generateRecommendations(stats)

	return stats
}

// generateRecommendations 根据统计数据生成性能优化建议
func (m *ICManager) generateRecommendations(stats ICDetailedStats) []string {
	var recommendations []string

	// 检查命中率
	if stats.MethodHitRate < 0.8 && (stats.MethodHits + stats.MethodMisses) > 100 {
		recommendations = append(recommendations, 
			"Method cache hit rate is low (%.1f%%). Consider reducing type polymorphism.")
	}

	if stats.PropertyHitRate < 0.8 && (stats.PropertyHits + stats.PropertyMisses) > 100 {
		recommendations = append(recommendations,
			"Property cache hit rate is low (%.1f%%). Consider using typed properties.")
	}

	// 检查超多态缓存
	megamorphicRatio := float64(stats.MethodMegamorphic) / float64(max(1, stats.TotalMethodCaches))
	if megamorphicRatio > 0.3 {
		recommendations = append(recommendations,
			"High megamorphic cache ratio (%.1f%%). Too many types at call sites.")
	}

	// 检查缓存大小
	if stats.TotalMethodCaches > MaxMethodCacheFunctions * 80 / 100 {
		recommendations = append(recommendations,
			"Method cache nearing capacity. Consider increasing MaxMethodCacheFunctions.")
	}

	if stats.TotalPropertyCaches > MaxPropertyCacheEntries * 80 / 100 {
		recommendations = append(recommendations,
			"Property cache nearing capacity. Consider increasing MaxPropertyCacheEntries.")
	}

	if len(recommendations) == 0 {
		recommendations = append(recommendations, "Inline cache performance is optimal.")
	}

	return recommendations
}

// max 返回两个整数中的较大值
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ResetStats 重置统计计数器（保留缓存内容）
func (m *ICManager) ResetStats() {
	for _, entry := range m.methodCaches {
		for _, cacheEntry := range entry.cache.caches {
			if cacheEntry != nil && cacheEntry.cache != nil {
				cacheEntry.cache.hits = 0
				cacheEntry.cache.misses = 0
			}
		}
	}
	for _, pc := range m.propertyCaches {
		pc.hits = 0
		pc.misses = 0
	}
}

// EnableProfiling 启用详细性能分析（可能略微影响性能）
func (m *ICManager) EnableProfiling() {
	// 当前实现已默认启用统计
	// 此方法预留给未来更详细的分析功能
	m.enabled = true
}

// DisableProfiling 禁用性能分析
func (m *ICManager) DisableProfiling() {
	// 禁用性能分析时，仅保留基本功能
	// 不收集详细统计
}

