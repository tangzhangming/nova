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

// GCObject 可被 GC 管理的对象接口
type GCObject interface {
	GetGCColor() GCColor
	SetGCColor(GCColor)
	GetGCChildren() []GCObject // 返回引用的子对象
}

// GC 垃圾回收器
type GC struct {
	// 堆上所有对象
	heap []GCObject

	// 对象注册表，用于根据指针快速找到对应的包装器
	objects map[uintptr]*GCObjectWrapper

	// 灰色对象队列（待扫描）
	grayList []GCObject

	// 统计信息
	totalAllocations int64 // 总分配次数
	totalCollections int64 // 总回收次数
	totalFreed       int64 // 总释放对象数

	// GC 触发阈值
	threshold     int // 触发 GC 的对象数量阈值
	nextThreshold int // 下次 GC 阈值（动态调整）

	// 分配计数器（用于周期性触发 GC）
	allocSinceLastGC int // 自上次 GC 以来的分配数
	allocThreshold   int // 分配多少个对象后检查 GC

	// GC 是否启用
	enabled bool

	// 调试模式
	debug bool
}

// NewGC 创建垃圾回收器
func NewGC() *GC {
	return &GC{
		heap:             make([]GCObject, 0, 64),
		objects:          make(map[uintptr]*GCObjectWrapper, 64),
		grayList:         make([]GCObject, 0, 32),
		threshold:        32,   // 初始阈值：32 个对象后触发 GC
		nextThreshold:    32,
		allocSinceLastGC: 0,
		allocThreshold:   16,   // 每分配 16 个对象检查一次是否需要 GC
		enabled:          true,
		debug:            false,
	}
}

// SetEnabled 启用/禁用 GC
func (gc *GC) SetEnabled(enabled bool) {
	gc.enabled = enabled
}

// SetDebug 设置调试模式
func (gc *GC) SetDebug(debug bool) {
	gc.debug = debug
}

// SetThreshold 设置 GC 触发阈值
func (gc *GC) SetThreshold(threshold int) {
	gc.threshold = threshold
	gc.nextThreshold = threshold
}

// Track 将对象加入 GC 管理
func (gc *GC) Track(obj GCObject) {
	if obj == nil {
		return
	}
	gc.heap = append(gc.heap, obj)
	gc.totalAllocations++
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
	gc.heap = append(gc.heap, w)
	gc.totalAllocations++
	gc.allocSinceLastGC++
	return w
}

// NeedsCollection 检查是否应该触发 GC（基于分配计数）
func (gc *GC) NeedsCollection() bool {
	if !gc.enabled {
		return false
	}
	// 基于分配计数器检查
	if gc.allocSinceLastGC >= gc.allocThreshold {
		return gc.ShouldCollect()
	}
	return false
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
	return len(gc.heap)
}

// ShouldCollect 检查是否应该触发 GC
func (gc *GC) ShouldCollect() bool {
	return gc.enabled && len(gc.heap) >= gc.nextThreshold
}

// Collect 执行垃圾回收
// roots: 根集合（栈、全局变量等）
func (gc *GC) Collect(roots []GCObject) int {
	if !gc.enabled {
		return 0
	}

	gc.totalCollections++
	beforeSize := len(gc.heap)

	// 阶段1: 标记（Mark）
	gc.mark(roots)

	// 阶段2: 清除（Sweep）
	freed := gc.sweep()

	gc.totalFreed += int64(freed)

	// 重置分配计数器
	gc.allocSinceLastGC = 0

	// 动态调整下次 GC 阈值
	// 策略：下次阈值 = 当前存活对象数 * 2，但不低于初始阈值
	afterSize := len(gc.heap)
	gc.nextThreshold = afterSize * 2
	if gc.nextThreshold < gc.threshold {
		gc.nextThreshold = gc.threshold
	}

	if gc.debug {
		println("[GC] Collection #", gc.totalCollections,
			": before=", beforeSize,
			", after=", afterSize,
			", freed=", freed,
			", next_threshold=", gc.nextThreshold)
	}

	return freed
}

// mark 标记阶段：从根集合开始，标记所有可达对象
func (gc *GC) mark(roots []GCObject) {
	// 1. 将所有对象标记为白色
	for _, obj := range gc.heap {
		if obj != nil {
			obj.SetGCColor(GCWhite)
		}
	}

	// 2. 将根对象标记为灰色并加入灰色队列
	gc.grayList = gc.grayList[:0] // 清空灰色队列
	for _, root := range roots {
		if root != nil && root.GetGCColor() == GCWhite {
			root.SetGCColor(GCGray)
			gc.grayList = append(gc.grayList, root)
		}
	}

	// 3. 处理灰色队列直到为空
	for len(gc.grayList) > 0 {
		// 取出一个灰色对象
		obj := gc.grayList[len(gc.grayList)-1]
		gc.grayList = gc.grayList[:len(gc.grayList)-1]

		// 标记为黑色
		obj.SetGCColor(GCBlack)

		// 将其子对象标记为灰色
		for _, child := range obj.GetGCChildren() {
			if child != nil && child.GetGCColor() == GCWhite {
				child.SetGCColor(GCGray)
				gc.grayList = append(gc.grayList, child)
			}
		}
	}
}

// sweep 清除阶段：回收所有白色（未标记）对象
func (gc *GC) sweep() int {
	freed := 0
	alive := make([]GCObject, 0, len(gc.heap))

	for _, obj := range gc.heap {
		if obj == nil {
			continue
		}
		if obj.GetGCColor() == GCWhite {
			// 白色对象：不可达，回收
			freed++
			// 可以在这里调用析构函数等清理逻辑
			gc.finalize(obj)

			// 从注册表移除
			if w, ok := obj.(*GCObjectWrapper); ok {
				key := gc.keyOf(w.value)
				if key != 0 {
					delete(gc.objects, key)
				}
			}
		} else {
			// 黑色对象：存活，保留
			alive = append(alive, obj)
		}
	}

	gc.heap = alive
	return freed
}

// finalize 对象析构（可扩展）
func (gc *GC) finalize(obj GCObject) {
	// 目前只是让对象被 Go GC 回收
	// 未来可以在这里调用 __destruct 等清理方法
}

// Stats 返回 GC 统计信息
func (gc *GC) Stats() GCStats {
	return GCStats{
		HeapSize:         len(gc.heap),
		TotalAllocations: gc.totalAllocations,
		TotalCollections: gc.totalCollections,
		TotalFreed:       gc.totalFreed,
		NextThreshold:    gc.nextThreshold,
	}
}

// GCStats GC 统计信息
type GCStats struct {
	HeapSize         int
	TotalAllocations int64
	TotalCollections int64
	TotalFreed       int64
	NextThreshold    int
}

// ============================================================================
// GCObject 包装器 - 为 bytecode.Value 中的堆对象实现 GCObject 接口
// ============================================================================

// GCObjectWrapper 包装需要 GC 管理的对象
type GCObjectWrapper struct {
	color GCColor
	value bytecode.Value
	gc    *GC
}

// NewGCObjectWrapper 创建 GC 对象包装器
func NewGCObjectWrapper(v bytecode.Value, gc *GC) *GCObjectWrapper {
	return &GCObjectWrapper{
		color: GCWhite,
		value: v,
		gc:    gc,
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
	case bytecode.ValArray, bytecode.ValFixedArray, bytecode.ValMap,
		bytecode.ValObject, bytecode.ValClosure, bytecode.ValFunc:
		return true
	default:
		return false
	}
}

