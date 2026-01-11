package vm

import (
	"reflect"

	"github.com/tangzhangming/nova/internal/bytecode"
)

// GCColor 三色标记
type GCColor byte

const (
	GCWhite GCColor = iota // 未访问（待回收）
	GCGray                 // 已发现但子对象未扫描
	GCBlack                // 已扫描完成（存活）
)

// GCGeneration 对象代
type GCGeneration byte

const (
	GenYoung GCGeneration = iota // 年轻代
	GenOld                       // 老年代
)

// GCPhase 增量 GC 阶段
type GCPhase byte

const (
	GCPhaseNone    GCPhase = iota // 无 GC 进行
	GCPhaseMarkYoung              // 标记年轻代
	GCPhaseMarkOld                // 标记老年代（Full GC）
	GCPhaseSweep                  // 清除阶段
)

// GCObject 可被 GC 管理的对象接口
type GCObject interface {
	GetGCColor() GCColor
	SetGCColor(GCColor)
	GetGCChildren() []GCObject // 返回引用的子对象
	GetGeneration() GCGeneration
	SetGeneration(GCGeneration)
	GetSurvivalCount() int
	IncrementSurvivalCount()
}

// GC 分代增量垃圾回收器
type GC struct {
	// ========== 分代堆 ==========
	youngGen []GCObject // 年轻代对象
	oldGen   []GCObject // 老年代对象

	// 对象注册表，用于根据指针快速找到对应的包装器
	objects map[uintptr]*GCObjectWrapper

	// ========== 增量标记 ==========
	grayList      []GCObject // 灰色对象队列（待扫描）
	currentPhase  GCPhase    // 当前 GC 阶段
	markWorkDone  int        // 已完成的标记工作量
	markWorkLimit int        // 每次增量标记的工作量限制

	// ========== 写屏障 ==========
	writeBarrierEnabled bool        // 写屏障是否启用
	rememberedSet       []GCObject  // 记忆集：老年代中指向年轻代的对象

	// ========== 晋升 ==========
	promotionThreshold int // 存活次数达到此值后晋升到老年代

	// ========== 统计信息 ==========
	totalAllocations   int64 // 总分配次数
	totalCollections   int64 // 总回收次数
	youngCollections   int64 // 年轻代回收次数
	oldCollections     int64 // 老年代回收次数（Full GC）
	totalFreed         int64 // 总释放对象数
	totalPromoted      int64 // 总晋升对象数
	avgPauseTimeUs     int64 // 平均停顿时间（微秒）
	maxPauseTimeUs     int64 // 最大停顿时间（微秒）

	// ========== 触发策略 ==========
	youngThreshold    int     // 年轻代触发阈值
	oldThreshold      int     // 老年代触发阈值
	youngGrowthFactor float64 // 年轻代增长因子
	oldGrowthFactor   float64 // 老年代增长因子
	allocSinceLastGC  int     // 自上次 GC 以来的分配数

	// ========== 自适应调整 ==========
	targetPauseTimeUs int64   // 目标停顿时间（微秒）
	adaptiveEnabled   bool    // 是否启用自适应调整

	// ========== 控制开关 ==========
	enabled bool // GC 是否启用
	debug   bool // 调试模式

	// ========== 内存泄漏检测 ==========
	leakDetection   bool                       // 是否启用泄漏检测
	allocationSites map[uintptr]AllocationInfo // 分配点信息
	leakReports     []LeakReport               // 泄漏报告
	cycleDetection  bool                       // 是否启用循环引用检测
	detectedCycles  []CycleInfo                // 检测到的循环引用
	cycleCheckFreq  int                        // 循环检测频率（每 N 次 Full GC 检测一次）
	cycleCheckCount int                        // 距离上次检测的 Full GC 次数

	// ========== 对象池 ==========
	arrayPool         *ObjectPool // 数组对象池
	stringBuilderPool *ObjectPool // StringBuilder 对象池
	argsPoolManager   *ArgsPoolManager // 参数数组池管理器

	// ========== GC 复用缓冲区 ==========
	youngAliveBuffer []GCObject // sweep 阶段复用的年轻代存活缓冲区
	oldAliveBuffer   []GCObject // sweep 阶段复用的老年代存活缓冲区

	// 兼容旧接口
	heap          []GCObject // 废弃：仅用于兼容
	threshold     int        // 废弃：使用 youngThreshold
	nextThreshold int        // 废弃
}

// AllocationInfo 分配点信息
type AllocationInfo struct {
	TypeName   string // 类型名称
	AllocTime  int64  // 分配时间（纳秒）
	StackTrace string // 分配时的调用栈（调试模式）
	Size       int    // 估计大小
}

// LeakReport 内存泄漏报告
type LeakReport struct {
	TypeName     string // 类型名称
	Count        int    // 泄漏数量
	TotalSize    int    // 总大小估计
	SampleTraces []string // 部分分配调用栈
}

// CycleInfo 循环引用信息
type CycleInfo struct {
	Objects []string  // 循环中的对象描述
	Path    []uintptr // 循环路径
}

// formatCycleInfo 格式化循环引用信息为字符串
func formatCycleInfo(cycle CycleInfo) string {
	if len(cycle.Objects) == 0 {
		return "<empty cycle>"
	}
	result := cycle.Objects[0]
	for i := 1; i < len(cycle.Objects); i++ {
		result += " -> " + cycle.Objects[i]
	}
	result += " -> " + cycle.Objects[0] // 闭合循环
	return result
}

// ObjectPool 对象池
type ObjectPool struct {
	pool      []interface{} // 可复用对象池
	maxSize   int           // 池最大容量
	newFunc   func() interface{} // 创建新对象的函数
	resetFunc func(interface{}) // 重置对象的函数
	
	// 统计信息
	hits   int64 // 命中次数（从池中获取）
	misses int64 // 未命中次数（需要新创建）
}

// NewObjectPool 创建对象池
func NewObjectPool(maxSize int, newFunc func() interface{}, resetFunc func(interface{})) *ObjectPool {
	return &ObjectPool{
		pool:      make([]interface{}, 0, maxSize),
		maxSize:   maxSize,
		newFunc:   newFunc,
		resetFunc: resetFunc,
	}
}

// Get 从池中获取对象
func (p *ObjectPool) Get() interface{} {
	if len(p.pool) > 0 {
		// 从池尾取出对象
		obj := p.pool[len(p.pool)-1]
		p.pool = p.pool[:len(p.pool)-1]
		p.hits++
		return obj
	}
	// 池为空，创建新对象
	p.misses++
	return p.newFunc()
}

// Put 归还对象到池
func (p *ObjectPool) Put(obj interface{}) {
	if len(p.pool) < p.maxSize {
		// 重置对象状态
		if p.resetFunc != nil {
			p.resetFunc(obj)
		}
		p.pool = append(p.pool, obj)
	}
	// 池满，丢弃对象（让 GC 回收）
}

// Stats 获取池统计信息
func (p *ObjectPool) Stats() (hits, misses int64, poolSize int) {
	return p.hits, p.misses, len(p.pool)
}

// ============================================================================
// 参数数组池管理器 - 按大小分档管理参数数组，减少函数调用时的临时分配
// ============================================================================

// ArgsPoolManager 参数数组池管理器
// 按大小分档：0-4, 5-8, 9-16, 17-32, 33-64
type ArgsPoolManager struct {
	pools [5]*ObjectPool
	// 统计信息
	totalHits   int64
	totalMisses int64
}

// 参数数组池大小档位
var argPoolSizes = [5]int{4, 8, 16, 32, 64}

// NewArgsPoolManager 创建参数数组池管理器
func NewArgsPoolManager() *ArgsPoolManager {
	m := &ArgsPoolManager{}
	
	// 为每个档位创建对象池
	for i, size := range argPoolSizes {
		poolSize := size // capture for closure
		m.pools[i] = NewObjectPool(32,
			func() interface{} {
				return make([]bytecode.Value, 0, poolSize)
			},
			func(obj interface{}) {
				// 重置数组内容，避免内存泄漏
				arr := obj.([]bytecode.Value)
				for j := range arr {
					arr[j] = bytecode.NullValue
				}
			},
		)
	}
	
	return m
}

// getBucketIndex 根据需要的大小获取档位索引
func (m *ArgsPoolManager) getBucketIndex(size int) int {
	for i, s := range argPoolSizes {
		if size <= s {
			return i
		}
	}
	return -1 // 超出最大档位
}

// GetArgs 从池中获取指定大小的参数数组
func (m *ArgsPoolManager) GetArgs(size int) []bytecode.Value {
	idx := m.getBucketIndex(size)
	if idx < 0 {
		// 超出池大小，直接分配
		m.totalMisses++
		return make([]bytecode.Value, size)
	}
	
	arr := m.pools[idx].Get().([]bytecode.Value)
	m.totalHits++
	
	// 扩展到需要的大小
	if cap(arr) >= size {
		return arr[:size]
	}
	
	// 容量不足（不应该发生），创建新数组
	m.totalMisses++
	return make([]bytecode.Value, size)
}

// ReturnArgs 归还参数数组到池
func (m *ArgsPoolManager) ReturnArgs(arr []bytecode.Value) {
	if arr == nil {
		return
	}
	
	idx := m.getBucketIndex(cap(arr))
	if idx < 0 {
		// 超出池大小，让 GC 回收
		return
	}
	
	// 清理数组内容，避免内存泄漏
	for i := range arr {
		arr[i] = bytecode.NullValue
	}
	
	m.pools[idx].Put(arr[:0])
}

// Stats 获取统计信息
func (m *ArgsPoolManager) Stats() (hits, misses int64, poolSizes []int) {
	poolSizes = make([]int, len(m.pools))
	for i, p := range m.pools {
		h, mi, s := p.Stats()
		hits += h
		misses += mi
		poolSizes[i] = s
	}
	return hits, misses, poolSizes
}

// NewGC 创建分代增量垃圾回收器
func NewGC() *GC {
	gc := &GC{
		// 分代堆
		youngGen: make([]GCObject, 0, 128),
		oldGen:   make([]GCObject, 0, 64),
		objects:  make(map[uintptr]*GCObjectWrapper, 128),

		// 增量标记
		grayList:      make([]GCObject, 0, 64),
		currentPhase:  GCPhaseNone,
		markWorkLimit: 50, // 每次增量标记处理 50 个对象

		// 写屏障
		writeBarrierEnabled: false,
		rememberedSet:       make([]GCObject, 0, 32),

		// 晋升
		promotionThreshold: 3, // 存活 3 次后晋升

		// 触发策略
		youngThreshold:    64,  // 年轻代 64 个对象后触发
		oldThreshold:      256, // 老年代 256 个对象后触发 Full GC
		youngGrowthFactor: 1.5, // 年轻代动态增长因子
		oldGrowthFactor:   2.0, // 老年代动态增长因子
		allocSinceLastGC:  0,

		// 自适应调整
		targetPauseTimeUs: 1000, // 目标停顿时间 1ms
		adaptiveEnabled:   true,

		// 控制开关
		enabled: true,
		debug:   false,

		// 内存泄漏检测
		leakDetection:   false,
		allocationSites: make(map[uintptr]AllocationInfo),
		cycleCheckFreq:  5, // 每 5 次 Full GC 检测一次循环引用
		cycleCheckCount: 0,

		// 兼容旧接口
		heap:          make([]GCObject, 0, 64),
		threshold:     64,
		nextThreshold: 64,
	}

	// 初始化对象池
	gc.arrayPool = NewObjectPool(32,
		func() interface{} { return make([]bytecode.Value, 0, 8) },
		func(obj interface{}) {
			arr := obj.([]bytecode.Value)
			for i := range arr {
				arr[i] = bytecode.NullValue
			}
		},
	)

	gc.stringBuilderPool = NewObjectPool(16,
		func() interface{} { return bytecode.NewStringBuilder() },
		func(obj interface{}) {
			sb := obj.(*bytecode.StringBuilder)
			sb.Parts = sb.Parts[:0]
			sb.Len = 0
		},
	)

	// 初始化参数数组池管理器
	gc.argsPoolManager = NewArgsPoolManager()

	// 初始化 sweep 复用缓冲区
	gc.youngAliveBuffer = make([]GCObject, 0, 128)
	gc.oldAliveBuffer = make([]GCObject, 0, 64)

	return gc
}

// GetArrayFromPool 从池中获取数组
func (gc *GC) GetArrayFromPool() []bytecode.Value {
	return gc.arrayPool.Get().([]bytecode.Value)
}

// ReturnArrayToPool 归还数组到池
func (gc *GC) ReturnArrayToPool(arr []bytecode.Value) {
	// 只归还小数组
	if cap(arr) <= 64 {
		gc.arrayPool.Put(arr[:0])
	}
}

// GetStringBuilderFromPool 从池中获取 StringBuilder
func (gc *GC) GetStringBuilderFromPool() *bytecode.StringBuilder {
	return gc.stringBuilderPool.Get().(*bytecode.StringBuilder)
}

// ReturnStringBuilderToPool 归还 StringBuilder 到池
func (gc *GC) ReturnStringBuilderToPool(sb *bytecode.StringBuilder) {
	gc.stringBuilderPool.Put(sb)
}

// GetPoolStats 获取对象池统计信息
func (gc *GC) GetPoolStats() map[string]map[string]int64 {
	stats := make(map[string]map[string]int64)
	
	arrayHits, arrayMisses, arraySize := gc.arrayPool.Stats()
	stats["array"] = map[string]int64{
		"hits":   arrayHits,
		"misses": arrayMisses,
		"size":   int64(arraySize),
	}
	
	sbHits, sbMisses, sbSize := gc.stringBuilderPool.Stats()
	stats["stringBuilder"] = map[string]int64{
		"hits":   sbHits,
		"misses": sbMisses,
		"size":   int64(sbSize),
	}

	// 参数数组池统计
	if gc.argsPoolManager != nil {
		argsHits, argsMisses, _ := gc.argsPoolManager.Stats()
		stats["args"] = map[string]int64{
			"hits":   argsHits,
			"misses": argsMisses,
		}
	}
	
	return stats
}

// GetArgsFromPool 从池中获取指定大小的参数数组
func (gc *GC) GetArgsFromPool(size int) []bytecode.Value {
	if gc.argsPoolManager == nil {
		return make([]bytecode.Value, size)
	}
	return gc.argsPoolManager.GetArgs(size)
}

// ReturnArgsToPool 归还参数数组到池
func (gc *GC) ReturnArgsToPool(arr []bytecode.Value) {
	if gc.argsPoolManager != nil {
		gc.argsPoolManager.ReturnArgs(arr)
	}
}

// SetEnabled 启用/禁用 GC
func (gc *GC) SetEnabled(enabled bool) {
	gc.enabled = enabled
}

// SetDebug 设置调试模式
// 调试模式下会自动启用循环引用检测和泄漏检测
func (gc *GC) SetDebug(debug bool) {
	gc.debug = debug
	if debug {
		// 调试模式下自动启用循环引用检测
		gc.cycleDetection = true
		gc.leakDetection = true
		if gc.allocationSites == nil {
			gc.allocationSites = make(map[uintptr]AllocationInfo)
		}
	}
}

// SetLeakDetection 设置泄漏检测模式
func (gc *GC) SetLeakDetection(enabled bool) {
	gc.leakDetection = enabled
	if enabled && gc.allocationSites == nil {
		gc.allocationSites = make(map[uintptr]AllocationInfo)
	}
}

// SetCycleDetection 设置循环引用检测模式
func (gc *GC) SetCycleDetection(enabled bool) {
	gc.cycleDetection = enabled
}

// SetCycleCheckFrequency 设置循环检测频率（每 N 次 Full GC 检测一次）
func (gc *GC) SetCycleCheckFrequency(freq int) {
	if freq < 1 {
		freq = 1
	}
	gc.cycleCheckFreq = freq
}

// GetDetectedCycles 获取检测到的循环引用
func (gc *GC) GetDetectedCycles() []CycleInfo {
	return gc.detectedCycles
}

// ClearDetectedCycles 清除检测到的循环引用
func (gc *GC) ClearDetectedCycles() {
	gc.detectedCycles = nil
}

// SetThreshold 设置 GC 触发阈值
func (gc *GC) SetThreshold(threshold int) {
	gc.threshold = threshold
	gc.nextThreshold = threshold
}

// Track 将对象加入 GC 管理（新对象进入年轻代）
func (gc *GC) Track(obj GCObject) {
	if obj == nil {
		return
	}
	obj.SetGeneration(GenYoung)
	gc.youngGen = append(gc.youngGen, obj)
	gc.heap = append(gc.heap, obj) // 兼容
	gc.totalAllocations++
	gc.allocSinceLastGC++
}

// TrackValue 将值包装为 GCObject 并追踪（如果需要）
func (gc *GC) TrackValue(v bytecode.Value) *GCObjectWrapper {
	if !isHeapValue(v) {
		return nil
	}
	key := gc.keyOf(v)
	if key == 0 {
		return nil
	}
	if exist, ok := gc.objects[key]; ok {
		return exist
	}
	w := NewGCObjectWrapper(v, gc)
	gc.objects[key] = w
	// 新对象进入年轻代
	w.generation = GenYoung
	gc.youngGen = append(gc.youngGen, w)
	gc.heap = append(gc.heap, w) // 兼容
	gc.totalAllocations++
	gc.allocSinceLastGC++
	return w
}

// WriteBarrier 写屏障：当老年代对象引用年轻代对象时调用
// 用于维护记忆集，确保年轻代 GC 时不会遗漏老年代的引用
func (gc *GC) WriteBarrier(parent, child GCObject) {
	if !gc.writeBarrierEnabled {
		return
	}
	// 如果父对象在老年代，子对象在年轻代，添加到记忆集
	if parent.GetGeneration() == GenOld && child.GetGeneration() == GenYoung {
		gc.rememberedSet = append(gc.rememberedSet, parent)
	}
}

// WriteBarrierValue 值类型的写屏障
func (gc *GC) WriteBarrierValue(parentVal, childVal bytecode.Value) {
	if !gc.writeBarrierEnabled {
		return
	}
	parent := gc.GetWrapper(parentVal)
	child := gc.GetWrapper(childVal)
	if parent != nil && child != nil {
		gc.WriteBarrier(parent, child)
	}
}

// NeedsCollection 检查是否应该触发 GC（基于分配计数）
func (gc *GC) NeedsCollection() bool {
	if !gc.enabled {
		return false
	}
	return gc.ShouldCollectYoung() || gc.ShouldCollectOld()
}

// ShouldCollectYoung 检查是否应该触发年轻代 GC
func (gc *GC) ShouldCollectYoung() bool {
	return gc.enabled && len(gc.youngGen) >= gc.youngThreshold
}

// ShouldCollectOld 检查是否应该触发老年代 GC（Full GC）
func (gc *GC) ShouldCollectOld() bool {
	return gc.enabled && len(gc.oldGen) >= gc.oldThreshold
}

// ResetAllocCounter 重置分配计数器（在 GC 后调用）
func (gc *GC) ResetAllocCounter() {
	gc.allocSinceLastGC = 0
}

// GetWrapper 获取已追踪的包装器
func (gc *GC) GetWrapper(v bytecode.Value) *GCObjectWrapper {
	if !isHeapValue(v) {
		return nil
	}
	key := gc.keyOf(v)
	if key == 0 {
		return nil
	}
	return gc.objects[key]
}

// HeapSize 返回堆上对象数量
func (gc *GC) HeapSize() int {
	return len(gc.youngGen) + len(gc.oldGen)
}

// YoungGenSize 返回年轻代对象数量
func (gc *GC) YoungGenSize() int {
	return len(gc.youngGen)
}

// OldGenSize 返回老年代对象数量
func (gc *GC) OldGenSize() int {
	return len(gc.oldGen)
}

// ShouldCollect 检查是否应该触发 GC（兼容旧接口）
func (gc *GC) ShouldCollect() bool {
	return gc.ShouldCollectYoung()
}

// Collect 执行垃圾回收（兼容旧接口，执行年轻代 GC）
// roots: 根集合（栈、全局变量等）
func (gc *GC) Collect(roots []GCObject) int {
	return gc.CollectYoung(roots)
}

// CollectYoung 执行年轻代 GC（Minor GC）
// 只回收年轻代对象，速度快，停顿短
func (gc *GC) CollectYoung(roots []GCObject) int {
	if !gc.enabled {
		return 0
	}

	gc.totalCollections++
	gc.youngCollections++
	beforeSize := len(gc.youngGen)

	// 启用写屏障
	gc.writeBarrierEnabled = true

	// 阶段1: 标记年轻代
	gc.markYoung(roots)

	// 阶段2: 清除年轻代并晋升存活对象
	freed, promoted := gc.sweepYoung()

	gc.totalFreed += int64(freed)
	gc.totalPromoted += int64(promoted)

	// 重置分配计数器
	gc.allocSinceLastGC = 0

	// 清空记忆集
	gc.rememberedSet = gc.rememberedSet[:0]

	// 动态调整年轻代阈值
	gc.adjustYoungThreshold()

	// 禁用写屏障
	gc.writeBarrierEnabled = false

	if gc.debug {
		println("[GC] Minor Collection #", gc.youngCollections,
			": before=", beforeSize,
			", after=", len(gc.youngGen),
			", freed=", freed,
			", promoted=", promoted)
	}

	// 检查是否需要触发 Full GC
	if gc.ShouldCollectOld() {
		gc.CollectFull(roots)
	}

	return freed
}

// CollectFull 执行完整 GC（Major GC / Full GC）
// 回收所有代的对象，停顿较长但更彻底
func (gc *GC) CollectFull(roots []GCObject) int {
	if !gc.enabled {
		return 0
	}

	gc.oldCollections++
	beforeYoung := len(gc.youngGen)
	beforeOld := len(gc.oldGen)

	// 标记所有代
	gc.markAll(roots)

	// 清除所有代
	freedYoung, promotedYoung := gc.sweepYoung()
	freedOld := gc.sweepOld()

	totalFreed := freedYoung + freedOld
	gc.totalFreed += int64(totalFreed)
	gc.totalPromoted += int64(promotedYoung)

	// 动态调整阈值
	gc.adjustOldThreshold()

	if gc.debug {
		println("[GC] Major Collection #", gc.oldCollections,
			": young before=", beforeYoung, ", after=", len(gc.youngGen),
			", old before=", beforeOld, ", after=", len(gc.oldGen),
			", freed=", totalFreed)
	}

	// 自动循环引用检测（在调试模式或启用循环检测时）
	if gc.cycleDetection {
		gc.cycleCheckCount++
		if gc.cycleCheckCount >= gc.cycleCheckFreq {
			gc.cycleCheckCount = 0
			cycles := gc.DetectCycles()
			if len(cycles) > 0 && gc.debug {
				println("[GC] Detected", len(cycles), "potential circular references:")
				for i, cycle := range cycles {
					if i >= 5 { // 只显示前 5 个
						println("  ... and", len(cycles)-5, "more")
						break
					}
					println("  Cycle", i+1, ":", formatCycleInfo(cycle))
				}
			}
		}
	}

	// 更新兼容字段
	gc.heap = append(gc.youngGen[:0:0], gc.youngGen...)
	gc.heap = append(gc.heap, gc.oldGen...)
	gc.nextThreshold = gc.youngThreshold

	return totalFreed
}

// CollectIncremental 执行增量 GC
// 每次只做部分工作，减少单次停顿时间
func (gc *GC) CollectIncremental(roots []GCObject) bool {
	if !gc.enabled {
		return true // 完成
	}

	switch gc.currentPhase {
	case GCPhaseNone:
		// 开始新的 GC 周期
		if gc.ShouldCollectYoung() {
			gc.startIncrementalMark(roots, false)
			return false
		}
		return true

	case GCPhaseMarkYoung, GCPhaseMarkOld:
		// 继续增量标记
		done := gc.incrementalMark()
		if done {
			gc.currentPhase = GCPhaseSweep
		}
		return false

	case GCPhaseSweep:
		// 执行清除（清除阶段通常很快，一次完成）
		if gc.currentPhase == GCPhaseMarkOld {
			gc.sweepYoung()
			gc.sweepOld()
		} else {
			gc.sweepYoung()
		}
		gc.currentPhase = GCPhaseNone
		gc.allocSinceLastGC = 0
		return true
	}

	return true
}

// startIncrementalMark 开始增量标记
func (gc *GC) startIncrementalMark(roots []GCObject, fullGC bool) {
	// 将所有对象标记为白色
	if fullGC {
		gc.currentPhase = GCPhaseMarkOld
		for _, obj := range gc.youngGen {
			if obj != nil {
				obj.SetGCColor(GCWhite)
			}
		}
		for _, obj := range gc.oldGen {
			if obj != nil {
				obj.SetGCColor(GCWhite)
			}
		}
	} else {
		gc.currentPhase = GCPhaseMarkYoung
		for _, obj := range gc.youngGen {
			if obj != nil {
				obj.SetGCColor(GCWhite)
			}
		}
	}

	// 启用写屏障
	gc.writeBarrierEnabled = true

	// 将根对象和记忆集加入灰色队列
	gc.grayList = gc.grayList[:0]
	for _, root := range roots {
		if root != nil && root.GetGCColor() == GCWhite {
			root.SetGCColor(GCGray)
			gc.grayList = append(gc.grayList, root)
		}
	}
	// 记忆集中的老年代对象也是根
	for _, obj := range gc.rememberedSet {
		if obj != nil && obj.GetGCColor() == GCWhite {
			obj.SetGCColor(GCGray)
			gc.grayList = append(gc.grayList, obj)
		}
	}

	gc.markWorkDone = 0
}

// incrementalMark 增量标记：每次处理有限数量的对象
func (gc *GC) incrementalMark() bool {
	workDone := 0

	for len(gc.grayList) > 0 && workDone < gc.markWorkLimit {
		// 取出一个灰色对象
		obj := gc.grayList[len(gc.grayList)-1]
		gc.grayList = gc.grayList[:len(gc.grayList)-1]

		// 标记为黑色
		obj.SetGCColor(GCBlack)
		workDone++

		// 将其子对象标记为灰色
		for _, child := range obj.GetGCChildren() {
			if child != nil && child.GetGCColor() == GCWhite {
				// 年轻代 GC 时只标记年轻代对象
				if gc.currentPhase == GCPhaseMarkYoung && child.GetGeneration() == GenOld {
					continue
				}
				child.SetGCColor(GCGray)
				gc.grayList = append(gc.grayList, child)
			}
		}
	}

	gc.markWorkDone += workDone
	return len(gc.grayList) == 0
}

// markYoung 标记年轻代对象
func (gc *GC) markYoung(roots []GCObject) {
	// 1. 将年轻代对象标记为白色
	for _, obj := range gc.youngGen {
		if obj != nil {
			obj.SetGCColor(GCWhite)
		}
	}

	// 2. 将根对象标记为灰色并加入灰色队列
	gc.grayList = gc.grayList[:0]
	for _, root := range roots {
		if root != nil && root.GetGCColor() == GCWhite {
			root.SetGCColor(GCGray)
			gc.grayList = append(gc.grayList, root)
		}
	}

	// 3. 记忆集中的老年代对象也是根（它们可能引用年轻代对象）
	for _, obj := range gc.rememberedSet {
		if obj != nil {
			// 老年代对象本身不需要标记，但要扫描它的子对象
			for _, child := range obj.GetGCChildren() {
				if child != nil && child.GetGeneration() == GenYoung && child.GetGCColor() == GCWhite {
					child.SetGCColor(GCGray)
					gc.grayList = append(gc.grayList, child)
				}
			}
		}
	}

	// 4. 处理灰色队列
	for len(gc.grayList) > 0 {
		obj := gc.grayList[len(gc.grayList)-1]
		gc.grayList = gc.grayList[:len(gc.grayList)-1]

		obj.SetGCColor(GCBlack)

		for _, child := range obj.GetGCChildren() {
			if child != nil && child.GetGCColor() == GCWhite {
				// 年轻代 GC 只追踪年轻代对象
				if child.GetGeneration() == GenYoung {
					child.SetGCColor(GCGray)
					gc.grayList = append(gc.grayList, child)
				}
			}
		}
	}
}

// markAll 标记所有代的对象（Full GC）
func (gc *GC) markAll(roots []GCObject) {
	// 1. 将所有对象标记为白色
	for _, obj := range gc.youngGen {
		if obj != nil {
			obj.SetGCColor(GCWhite)
		}
	}
	for _, obj := range gc.oldGen {
		if obj != nil {
			obj.SetGCColor(GCWhite)
		}
	}

	// 2. 将根对象标记为灰色
	gc.grayList = gc.grayList[:0]
	for _, root := range roots {
		if root != nil && root.GetGCColor() == GCWhite {
			root.SetGCColor(GCGray)
			gc.grayList = append(gc.grayList, root)
		}
	}

	// 3. 处理灰色队列
	for len(gc.grayList) > 0 {
		obj := gc.grayList[len(gc.grayList)-1]
		gc.grayList = gc.grayList[:len(gc.grayList)-1]

		obj.SetGCColor(GCBlack)

		for _, child := range obj.GetGCChildren() {
			if child != nil && child.GetGCColor() == GCWhite {
				child.SetGCColor(GCGray)
				gc.grayList = append(gc.grayList, child)
			}
		}
	}
}

// sweepYoung 清除年轻代，返回 (释放数, 晋升数)
func (gc *GC) sweepYoung() (freed, promoted int) {
	// 复用缓冲区，避免每次 GC 都分配新切片
	// 确保缓冲区容量足够
	if cap(gc.youngAliveBuffer) < len(gc.youngGen) {
		gc.youngAliveBuffer = make([]GCObject, 0, len(gc.youngGen)*2)
	}
	alive := gc.youngAliveBuffer[:0]

	for _, obj := range gc.youngGen {
		if obj == nil {
			continue
		}
		if obj.GetGCColor() == GCWhite {
			// 白色对象：不可达，回收
			freed++
			gc.finalize(obj)

			if w, ok := obj.(*GCObjectWrapper); ok {
				key := gc.keyOf(w.value)
				if key != 0 {
					delete(gc.objects, key)
				}
			}
		} else {
			// 黑色对象：存活
			obj.IncrementSurvivalCount()

			// 检查是否应该晋升到老年代
			if obj.GetSurvivalCount() >= gc.promotionThreshold {
				obj.SetGeneration(GenOld)
				gc.oldGen = append(gc.oldGen, obj)
				promoted++
			} else {
				alive = append(alive, obj)
			}
		}
	}

	// 交换切片：youngGen 使用 alive 的底层数组，youngAliveBuffer 复用 youngGen 的
	gc.youngAliveBuffer = gc.youngGen[:0]
	gc.youngGen = alive
	return
}

// sweepOld 清除老年代
func (gc *GC) sweepOld() int {
	freed := 0
	// 复用缓冲区，避免每次 GC 都分配新切片
	// 确保缓冲区容量足够
	if cap(gc.oldAliveBuffer) < len(gc.oldGen) {
		gc.oldAliveBuffer = make([]GCObject, 0, len(gc.oldGen)*2)
	}
	alive := gc.oldAliveBuffer[:0]

	for _, obj := range gc.oldGen {
		if obj == nil {
			continue
		}
		if obj.GetGCColor() == GCWhite {
			freed++
			gc.finalize(obj)

			if w, ok := obj.(*GCObjectWrapper); ok {
				key := gc.keyOf(w.value)
				if key != 0 {
					delete(gc.objects, key)
				}
			}
		} else {
			alive = append(alive, obj)
		}
	}

	// 交换切片：oldGen 使用 alive 的底层数组，oldAliveBuffer 复用 oldGen 的
	gc.oldAliveBuffer = gc.oldGen[:0]
	gc.oldGen = alive
	return freed
}

// mark 标记阶段（兼容旧接口）
func (gc *GC) mark(roots []GCObject) {
	gc.markAll(roots)
}

// sweep 清除阶段（兼容旧接口）
func (gc *GC) sweep() int {
	freedYoung, _ := gc.sweepYoung()
	freedOld := gc.sweepOld()
	return freedYoung + freedOld
}

// adjustYoungThreshold 动态调整年轻代阈值
func (gc *GC) adjustYoungThreshold() {
	// 策略：基于存活率调整
	// 存活对象多 -> 增大阈值（减少 GC 频率）
	// 存活对象少 -> 可以保持或减小阈值
	survivalRate := float64(len(gc.youngGen)) / float64(gc.youngThreshold)
	if survivalRate > 0.5 {
		// 存活率高，增大阈值
		gc.youngThreshold = int(float64(gc.youngThreshold) * gc.youngGrowthFactor)
		if gc.youngThreshold > 1024 {
			gc.youngThreshold = 1024 // 上限
		}
	}
	gc.nextThreshold = gc.youngThreshold // 兼容
}

// adjustOldThreshold 动态调整老年代阈值
func (gc *GC) adjustOldThreshold() {
	survivalRate := float64(len(gc.oldGen)) / float64(gc.oldThreshold)
	if survivalRate > 0.7 {
		gc.oldThreshold = int(float64(gc.oldThreshold) * gc.oldGrowthFactor)
		if gc.oldThreshold > 4096 {
			gc.oldThreshold = 4096 // 上限
		}
	}
}

// finalize 对象析构（可扩展）
func (gc *GC) finalize(obj GCObject) {
	// 目前只是让对象被 Go GC 回收
	// 未来可以在这里调用 __destruct 等清理方法
}

// Stats 返回 GC 统计信息
func (gc *GC) Stats() GCStats {
	return GCStats{
		HeapSize:         gc.HeapSize(),
		YoungGenSize:     len(gc.youngGen),
		OldGenSize:       len(gc.oldGen),
		TotalAllocations: gc.totalAllocations,
		TotalCollections: gc.totalCollections,
		YoungCollections: gc.youngCollections,
		OldCollections:   gc.oldCollections,
		TotalFreed:       gc.totalFreed,
		TotalPromoted:    gc.totalPromoted,
		NextThreshold:    gc.nextThreshold,
		YoungThreshold:   gc.youngThreshold,
		OldThreshold:     gc.oldThreshold,
		LeakReports:      gc.leakReports,
		DetectedCycles:   gc.detectedCycles,
	}
}

// ============================================================================
// 内存泄漏检测
// ============================================================================

// DetectLeaks 检测内存泄漏
// 应该在程序结束时调用，检测还存活但可能是泄漏的对象
func (gc *GC) DetectLeaks() []LeakReport {
	if !gc.leakDetection {
		return nil
	}

	gc.leakReports = nil
	typeCount := make(map[string]int)
	typeSamples := make(map[string][]string)

	// 检查年轻代
	for _, obj := range gc.youngGen {
		if obj == nil {
			continue
		}
		if w, ok := obj.(*GCObjectWrapper); ok {
			typeName := gc.getTypeName(w.value) + " (young)"
			typeCount[typeName]++

			key := gc.keyOf(w.value)
			if info, exists := gc.allocationSites[key]; exists {
				if len(typeSamples[typeName]) < 3 {
					typeSamples[typeName] = append(typeSamples[typeName], info.StackTrace)
				}
			}
		}
	}

	// 检查老年代
	for _, obj := range gc.oldGen {
		if obj == nil {
			continue
		}
		if w, ok := obj.(*GCObjectWrapper); ok {
			typeName := gc.getTypeName(w.value) + " (old)"
			typeCount[typeName]++

			key := gc.keyOf(w.value)
			if info, exists := gc.allocationSites[key]; exists {
				if len(typeSamples[typeName]) < 3 {
					typeSamples[typeName] = append(typeSamples[typeName], info.StackTrace)
				}
			}
		}
	}

	// 生成报告
	for typeName, count := range typeCount {
		if count > 0 {
			gc.leakReports = append(gc.leakReports, LeakReport{
				TypeName:     typeName,
				Count:        count,
				SampleTraces: typeSamples[typeName],
			})
		}
	}

	return gc.leakReports
}

// DetectCycles 检测循环引用
func (gc *GC) DetectCycles() []CycleInfo {
	if !gc.cycleDetection {
		return nil
	}

	gc.detectedCycles = nil
	visited := make(map[uintptr]bool)
	inStack := make(map[uintptr]bool)
	path := make([]uintptr, 0)

	// 检查年轻代
	for _, obj := range gc.youngGen {
		if obj == nil {
			continue
		}
		if w, ok := obj.(*GCObjectWrapper); ok {
			key := gc.keyOf(w.value)
			if !visited[key] {
				gc.detectCycleDFS(w, visited, inStack, path)
			}
		}
	}

	// 检查老年代
	for _, obj := range gc.oldGen {
		if obj == nil {
			continue
		}
		if w, ok := obj.(*GCObjectWrapper); ok {
			key := gc.keyOf(w.value)
			if !visited[key] {
				gc.detectCycleDFS(w, visited, inStack, path)
			}
		}
	}

	return gc.detectedCycles
}

// detectCycleDFS 使用 DFS 检测循环引用
func (gc *GC) detectCycleDFS(obj *GCObjectWrapper, visited, inStack map[uintptr]bool, path []uintptr) {
	key := gc.keyOf(obj.value)
	if key == 0 {
		return
	}

	visited[key] = true
	inStack[key] = true
	path = append(path, key)

	children := gc.getValueChildren(obj.value)
	for _, child := range children {
		if childW, ok := child.(*GCObjectWrapper); ok {
			childKey := gc.keyOf(childW.value)
			if childKey == 0 {
				continue
			}

			if inStack[childKey] {
				// 发现循环
				cycleStart := -1
				for i, p := range path {
					if p == childKey {
						cycleStart = i
						break
					}
				}
				if cycleStart >= 0 {
					cyclePath := make([]uintptr, len(path)-cycleStart)
					copy(cyclePath, path[cycleStart:])

					// 构建循环描述
					var objects []string
					for _, p := range cyclePath {
						if w, exists := gc.objects[p]; exists {
							objects = append(objects, gc.getTypeName(w.value))
						}
					}

					gc.detectedCycles = append(gc.detectedCycles, CycleInfo{
						Objects: objects,
						Path:    cyclePath,
					})
				}
			} else if !visited[childKey] {
				gc.detectCycleDFS(childW, visited, inStack, path)
			}
		}
	}

	inStack[key] = false
}

// getTypeName 获取值的类型名称
func (gc *GC) getTypeName(v bytecode.Value) string {
	switch v.Type {
	case bytecode.ValArray:
		return "array"
	case bytecode.ValFixedArray:
		return "fixed_array"
	case bytecode.ValNativeArray:
		return "native_array"
	case bytecode.ValMap:
		return "map"
	case bytecode.ValObject:
		if obj := v.AsObject(); obj != nil && obj.Class != nil {
			return obj.Class.Name
		}
		return "unknown"
	case bytecode.ValClosure:
		return "closure"
	case bytecode.ValFunc:
		if fn := v.Data.(*bytecode.Function); fn != nil {
			return "function:" + fn.Name
		}
		return "function"
	default:
		return gc.valueTypeName(v.Type)
	}
}

// valueTypeName 获取值类型名称
func (gc *GC) valueTypeName(t bytecode.ValueType) string {
	switch t {
	case bytecode.ValNull:
		return "null"
	case bytecode.ValBool:
		return "bool"
	case bytecode.ValInt:
		return "int"
	case bytecode.ValFloat:
		return "float"
	case bytecode.ValString:
		return "string"
	case bytecode.ValArray:
		return "array"
	case bytecode.ValFixedArray:
		return "fixed_array"
	case bytecode.ValNativeArray:
		return "native_array"
	case bytecode.ValMap:
		return "map"
	case bytecode.ValObject:
		return "unknown"
	case bytecode.ValFunc:
		return "function"
	case bytecode.ValClosure:
		return "closure"
	case bytecode.ValIterator:
		return "iterator"
	default:
		return "unknown"
	}
}

// PrintLeakReport 打印泄漏报告
func (gc *GC) PrintLeakReport() {
	reports := gc.DetectLeaks()
	if len(reports) == 0 {
		println("[GC] No memory leaks detected")
		return
	}

	println("[GC] Memory Leak Report:")
	println("========================")
	for _, report := range reports {
		println("  Type:", report.TypeName)
		println("    Count:", report.Count)
		if len(report.SampleTraces) > 0 {
			println("    Sample allocation traces:")
			for _, trace := range report.SampleTraces {
				if trace != "" {
					println("      -", trace)
				}
			}
		}
	}
}

// PrintCycleReport 打印循环引用报告
func (gc *GC) PrintCycleReport() {
	cycles := gc.DetectCycles()
	if len(cycles) == 0 {
		println("[GC] No circular references detected")
		return
	}

	println("[GC] Circular Reference Report:")
	println("================================")
	for i, cycle := range cycles {
		println("  Cycle", i+1, ":")
		for j, obj := range cycle.Objects {
			if j > 0 {
				print(" -> ")
			}
			print(obj)
		}
		println(" -> (back to start)")
	}
}

// DebugDump 输出完整的 GC 调试信息
func (gc *GC) DebugDump() {
	println("\n[GC] Debug Dump - Generational GC")
	println("==================================")
	println("Young Generation Size:", len(gc.youngGen))
	println("Old Generation Size:", len(gc.oldGen))
	println("Total Heap Size:", gc.HeapSize())
	println("")
	println("Young GC Collections:", gc.youngCollections)
	println("Full GC Collections:", gc.oldCollections)
	println("Total Allocations:", gc.totalAllocations)
	println("Total Freed:", gc.totalFreed)
	println("Total Promoted:", gc.totalPromoted)
	println("")
	println("Young Threshold:", gc.youngThreshold)
	println("Old Threshold:", gc.oldThreshold)
	println("Promotion Threshold:", gc.promotionThreshold, "survivals")
	println("")

	// 按类型和代统计对象
	youngTypeCounts := make(map[string]int)
	oldTypeCounts := make(map[string]int)

	for _, obj := range gc.youngGen {
		if obj == nil {
			continue
		}
		if w, ok := obj.(*GCObjectWrapper); ok {
			typeName := gc.getTypeName(w.value)
			youngTypeCounts[typeName]++
		}
	}

	for _, obj := range gc.oldGen {
		if obj == nil {
			continue
		}
		if w, ok := obj.(*GCObjectWrapper); ok {
			typeName := gc.getTypeName(w.value)
			oldTypeCounts[typeName]++
		}
	}

	println("Young Generation Objects by Type:")
	for typeName, count := range youngTypeCounts {
		println("  ", typeName, ":", count)
	}

	println("")
	println("Old Generation Objects by Type:")
	for typeName, count := range oldTypeCounts {
		println("  ", typeName, ":", count)
	}

	// 检测循环引用
	if gc.cycleDetection {
		println("")
		gc.PrintCycleReport()
	}

	// 检测泄漏
	if gc.leakDetection {
		println("")
		gc.PrintLeakReport()
	}
}

// GCStats GC 统计信息
type GCStats struct {
	HeapSize         int   // 堆总大小
	YoungGenSize     int   // 年轻代大小
	OldGenSize       int   // 老年代大小
	TotalAllocations int64 // 总分配次数
	TotalCollections int64 // 总回收次数
	YoungCollections int64 // 年轻代回收次数
	OldCollections   int64 // 老年代回收次数（Full GC）
	TotalFreed       int64 // 总释放对象数
	TotalPromoted    int64 // 总晋升对象数
	NextThreshold    int   // 下次 GC 阈值（兼容）
	YoungThreshold   int   // 年轻代阈值
	OldThreshold     int   // 老年代阈值
	LeakReports      []LeakReport // 泄漏报告
	DetectedCycles   []CycleInfo  // 检测到的循环引用
}

// ============================================================================
// GCObject 包装器 - 为 bytecode.Value 中的堆对象实现 GCObject 接口
// ============================================================================

// GCObjectWrapper 包装需要 GC 管理的对象
type GCObjectWrapper struct {
	color         GCColor
	generation    GCGeneration
	survivalCount int // 存活次数（用于晋升决策）
	value         bytecode.Value
	gc            *GC
}

// NewGCObjectWrapper 创建 GC 对象包装器
func NewGCObjectWrapper(v bytecode.Value, gc *GC) *GCObjectWrapper {
	return &GCObjectWrapper{
		color:         GCWhite,
		generation:    GenYoung,
		survivalCount: 0,
		value:         v,
		gc:            gc,
	}
}

func (w *GCObjectWrapper) GetGCColor() GCColor {
	return w.color
}

func (w *GCObjectWrapper) SetGCColor(c GCColor) {
	w.color = c
}

func (w *GCObjectWrapper) GetGCChildren() []GCObject {
	return w.gc.getValueChildren(w.value)
}

func (w *GCObjectWrapper) GetGeneration() GCGeneration {
	return w.generation
}

func (w *GCObjectWrapper) SetGeneration(g GCGeneration) {
	w.generation = g
}

func (w *GCObjectWrapper) GetSurvivalCount() int {
	return w.survivalCount
}

func (w *GCObjectWrapper) IncrementSurvivalCount() {
	w.survivalCount++
}

func (w *GCObjectWrapper) GetValue() bytecode.Value {
	return w.value
}

// getValueChildren 获取值引用的子对象（使用已注册的包装器）
func (gc *GC) getValueChildren(v bytecode.Value) []GCObject {
	var children []GCObject

	switch v.Type {
	case bytecode.ValArray:
		arr := v.AsArray()
		for _, elem := range arr {
			if w := gc.TrackValue(elem); w != nil {
				children = append(children, w)
			}
		}

	case bytecode.ValFixedArray:
		fa := v.AsFixedArray()
		if fa != nil {
			for _, elem := range fa.Elements {
				if w := gc.TrackValue(elem); w != nil {
					children = append(children, w)
				}
			}
		}

	case bytecode.ValNativeArray:
		na := v.AsNativeArray()
		if na != nil {
			// 对于引用类型元素，需要追踪
			if na.ElementType == bytecode.ValString || na.ElementType == bytecode.ValObject {
				for i := 0; i < na.Len(); i++ {
					elem := na.Get(i)
					if w := gc.TrackValue(elem); w != nil {
						children = append(children, w)
					}
				}
			}
		}

	case bytecode.ValMap:
		m := v.AsMap()
		for k, val := range m {
			if w := gc.TrackValue(k); w != nil {
				children = append(children, w)
			}
			if w := gc.TrackValue(val); w != nil {
				children = append(children, w)
			}
		}

	case bytecode.ValObject:
		obj := v.AsObject()
		if obj != nil {
			for _, field := range obj.Fields {
				if w := gc.TrackValue(field); w != nil {
					children = append(children, w)
				}
			}
		}

	case bytecode.ValClosure:
		closure := v.Data.(*bytecode.Closure)
		if closure != nil {
			for _, upval := range closure.Upvalues {
				if upval != nil && upval.IsClosed {
					if w := gc.TrackValue(upval.Closed); w != nil {
						children = append(children, w)
					}
				}
			}
		}
	}

	return children
}

// keyOf 计算值的唯一标识（基于底层指针）
func (gc *GC) keyOf(v bytecode.Value) uintptr {
	switch v.Type {
	case bytecode.ValArray:
		return reflect.ValueOf(v.AsArray()).Pointer()
	case bytecode.ValFixedArray:
		return reflect.ValueOf(v.AsFixedArray()).Pointer()
	case bytecode.ValNativeArray:
		return reflect.ValueOf(v.AsNativeArray()).Pointer()
	case bytecode.ValMap:
		return reflect.ValueOf(v.AsMap()).Pointer()
	case bytecode.ValObject:
		return reflect.ValueOf(v.AsObject()).Pointer()
	case bytecode.ValClosure:
		return reflect.ValueOf(v.Data.(*bytecode.Closure)).Pointer()
	case bytecode.ValFunc:
		return reflect.ValueOf(v.Data.(*bytecode.Function)).Pointer()
	case bytecode.ValIterator:
		return reflect.ValueOf(v.AsIterator()).Pointer()
	default:
		return 0
	}
}

// isHeapValue 判断值是否是堆分配的（需要 GC 管理）
func isHeapValue(v bytecode.Value) bool {
	switch v.Type {
	case bytecode.ValArray, bytecode.ValFixedArray, bytecode.ValNativeArray, bytecode.ValMap,
		bytecode.ValObject, bytecode.ValClosure, bytecode.ValFunc:
		return true
	default:
		return false
	}
}

