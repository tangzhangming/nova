// jit_iterator.go - JIT 友好的迭代器实现
//
// 本文件实现了 JIT 友好的迭代器结构，使用固定内存布局。
// 支持数组、Map 和范围迭代，JIT 生成的机器码可以直接操作迭代器状态。
//
// 内存布局:
//   JITIterator 结构 (40 bytes):
//     偏移 0:  Type      (4 bytes) - 迭代器类型
//     偏移 4:  Length    (4 bytes) - 总长度
//     偏移 8:  Index     (4 bytes) - 当前索引
//     偏移 12: Flags     (4 bytes) - 标志位
//     偏移 16: DataPtr   (8 bytes) - 数据指针
//     偏移 24: KeyPtr    (8 bytes) - 当前键指针（Map 迭代）
//     偏移 32: ValuePtr  (8 bytes) - 当前值指针

package bytecode

import (
	"sync"
	"unsafe"
)

// JITIterType 迭代器类型
type JITIterType int32

const (
	JITIterTypeNone       JITIterType = 0
	JITIterTypeArray      JITIterType = 1 // 数组迭代
	JITIterTypeNativeArray JITIterType = 2 // NativeArray 迭代
	JITIterTypeMap        JITIterType = 3 // Map 迭代
	JITIterTypeJITMap     JITIterType = 4 // JITMap 迭代
	JITIterTypeRange      JITIterType = 5 // 范围迭代 (for i in 0..10)
	JITIterTypeString     JITIterType = 6 // 字符串迭代
)

// JITIterFlags 迭代器标志
type JITIterFlags int32

const (
	JITIterFlagNone     JITIterFlags = 0
	JITIterFlagHasValue JITIterFlags = 1 << 0 // 当前有有效值
	JITIterFlagFinished JITIterFlags = 1 << 1 // 迭代完成
	JITIterFlagReverse  JITIterFlags = 1 << 2 // 反向迭代
)

// JITIterator JIT 友好的迭代器
type JITIterator struct {
	Type     JITIterType  // 迭代器类型（偏移 0）
	Length   int32        // 总长度（偏移 4）
	Index    int32        // 当前索引（偏移 8）
	Flags    JITIterFlags // 标志位（偏移 12）
	DataPtr  uintptr      // 数据指针（偏移 16）
	KeyPtr   uintptr      // 当前键指针（偏移 24）
	ValuePtr uintptr      // 当前值指针（偏移 32）

	// 以下字段不在 JIT 布局中，仅供 Go 代码使用
	currentKey   int64 // 当前键缓存
	currentValue int64 // 当前值缓存

	// 范围迭代专用
	rangeStart int64
	rangeEnd   int64
	rangeStep  int64

	// Map 迭代专用
	mapKeyIndex int32   // 当前在 keys 数组中的位置
	mapKeys     []int64 // Map 的所有键（预取）
}

// JITIterator 内存偏移常量
const (
	JITIterOffsetType     = 0
	JITIterOffsetLength   = 4
	JITIterOffsetIndex    = 8
	JITIterOffsetFlags    = 12
	JITIterOffsetDataPtr  = 16
	JITIterOffsetKeyPtr   = 24
	JITIterOffsetValuePtr = 32
	JITIterStructSize     = 40
)

// NewJITIterator 创建新的迭代器
func NewJITIterator() *JITIterator {
	return &JITIterator{
		Type:  JITIterTypeNone,
		Index: -1, // 初始为 -1，第一次 Next 后变为 0
		Flags: JITIterFlagNone,
	}
}

// InitFromArray 从普通数组初始化
func (it *JITIterator) InitFromArray(arr []Value) {
	it.Type = JITIterTypeArray
	it.Length = int32(len(arr))
	it.Index = -1
	it.Flags = JITIterFlagNone
	if len(arr) > 0 {
		it.DataPtr = uintptr(unsafe.Pointer(&arr[0]))
	}
}

// InitFromNativeArray 从 NativeArray 初始化
func (it *JITIterator) InitFromNativeArray(arr *NativeArray) {
	it.Type = JITIterTypeNativeArray
	it.Length = arr.Length
	it.Index = -1
	it.Flags = JITIterFlagNone
	it.DataPtr = uintptr(arr.Data)
}

// InitFromJITMap 从 JITMap 初始化
func (it *JITIterator) InitFromJITMap(m *JITMap) {
	it.Type = JITIterTypeJITMap
	it.Length = m.Size
	it.Index = -1
	it.Flags = JITIterFlagNone
	it.DataPtr = uintptr(unsafe.Pointer(m))

	// 预取所有键
	it.mapKeys = m.GetKeys()
	it.mapKeyIndex = -1
}

// InitFromRange 从范围初始化
func (it *JITIterator) InitFromRange(start, end, step int64) {
	it.Type = JITIterTypeRange
	it.rangeStart = start
	it.rangeEnd = end
	it.rangeStep = step

	// 计算长度
	if step > 0 {
		it.Length = int32((end - start + step - 1) / step)
	} else if step < 0 {
		it.Length = int32((start - end - step - 1) / (-step))
	} else {
		it.Length = 0
	}

	it.Index = -1
	it.Flags = JITIterFlagNone
	it.currentKey = start - step // 第一次 Next 后会加 step
}

// InitFromString 从字符串初始化
func (it *JITIterator) InitFromString(s string) {
	runes := []rune(s)
	it.Type = JITIterTypeString
	it.Length = int32(len(runes))
	it.Index = -1
	it.Flags = JITIterFlagNone

	// 存储 runes
	if len(runes) > 0 {
		runeSlice := make([]int64, len(runes))
		for i, r := range runes {
			runeSlice[i] = int64(r)
		}
		it.DataPtr = uintptr(unsafe.Pointer(&runeSlice[0]))
	}
}

// Next 移动到下一个元素
// 返回是否还有更多元素
func (it *JITIterator) Next() bool {
	it.Index++

	if it.Index >= it.Length {
		it.Flags |= JITIterFlagFinished
		it.Flags &^= JITIterFlagHasValue
		return false
	}

	it.Flags |= JITIterFlagHasValue
	it.Flags &^= JITIterFlagFinished

	// 更新当前键值
	switch it.Type {
	case JITIterTypeArray:
		it.updateArrayValue()
	case JITIterTypeNativeArray:
		it.updateNativeArrayValue()
	case JITIterTypeJITMap:
		it.updateJITMapValue()
	case JITIterTypeRange:
		it.updateRangeValue()
	case JITIterTypeString:
		it.updateStringValue()
	}

	return true
}

func (it *JITIterator) updateArrayValue() {
	it.currentKey = int64(it.Index)
	if it.DataPtr != 0 {
		valuePtr := it.DataPtr + uintptr(it.Index)*unsafe.Sizeof(Value{})
		value := *(*Value)(unsafe.Pointer(valuePtr))
		it.currentValue = ValueToInt64(value)
	}
}

func (it *JITIterator) updateNativeArrayValue() {
	it.currentKey = int64(it.Index)
	if it.DataPtr != 0 {
		valuePtr := it.DataPtr + uintptr(it.Index)*8
		it.currentValue = *(*int64)(unsafe.Pointer(valuePtr))
	}
}

func (it *JITIterator) updateJITMapValue() {
	it.mapKeyIndex++
	if int(it.mapKeyIndex) < len(it.mapKeys) {
		it.currentKey = it.mapKeys[it.mapKeyIndex]
		m := (*JITMap)(unsafe.Pointer(it.DataPtr))
		if value, found := m.Get(it.currentKey); found {
			it.currentValue = value
		}
	}
}

func (it *JITIterator) updateRangeValue() {
	it.currentKey = it.rangeStart + int64(it.Index)*it.rangeStep
	it.currentValue = it.currentKey
}

func (it *JITIterator) updateStringValue() {
	it.currentKey = int64(it.Index)
	if it.DataPtr != 0 {
		runePtr := it.DataPtr + uintptr(it.Index)*8
		it.currentValue = *(*int64)(unsafe.Pointer(runePtr))
	}
}

// HasNext 检查是否还有更多元素
func (it *JITIterator) HasNext() bool {
	return it.Index+1 < it.Length
}

// Key 获取当前键
func (it *JITIterator) Key() int64 {
	return it.currentKey
}

// Value 获取当前值
func (it *JITIterator) Value() int64 {
	return it.currentValue
}

// KeyValue 获取当前键值对
func (it *JITIterator) KeyValue() (int64, int64) {
	return it.currentKey, it.currentValue
}

// Reset 重置迭代器
func (it *JITIterator) Reset() {
	it.Index = -1
	it.Flags = JITIterFlagNone
	it.mapKeyIndex = -1
	if it.Type == JITIterTypeRange {
		it.currentKey = it.rangeStart - it.rangeStep
	}
}

// IsFinished 检查是否完成
func (it *JITIterator) IsFinished() bool {
	return it.Flags&JITIterFlagFinished != 0
}

// HasValue 检查当前是否有有效值
func (it *JITIterator) HasValue() bool {
	return it.Flags&JITIterFlagHasValue != 0
}

// ============================================================================
// JIT 内联辅助函数
// ============================================================================

// JITIterNextInline JIT 内联版本的 Next
// 返回: 1 = 有更多元素，0 = 完成
func JITIterNextInline(it *JITIterator) int64 {
	it.Index++

	if it.Index >= it.Length {
		it.Flags |= JITIterFlagFinished
		return 0
	}

	it.Flags |= JITIterFlagHasValue
	return 1
}

// JITIterKeyInline JIT 内联版本获取当前索引/键
func JITIterKeyInline(it *JITIterator) int64 {
	switch it.Type {
	case JITIterTypeArray, JITIterTypeNativeArray, JITIterTypeString:
		return int64(it.Index)
	case JITIterTypeRange:
		return it.rangeStart + int64(it.Index)*it.rangeStep
	case JITIterTypeJITMap:
		if int(it.mapKeyIndex) < len(it.mapKeys) {
			return it.mapKeys[it.mapKeyIndex]
		}
		return 0
	default:
		return int64(it.Index)
	}
}

// JITIterValueInline JIT 内联版本获取当前值
// 对于 NativeArray，直接从内存读取
func JITIterValueInline(it *JITIterator) int64 {
	switch it.Type {
	case JITIterTypeNativeArray:
		if it.DataPtr != 0 {
			valuePtr := it.DataPtr + uintptr(it.Index)*8
			return *(*int64)(unsafe.Pointer(valuePtr))
		}
		return 0
	case JITIterTypeRange:
		return it.rangeStart + int64(it.Index)*it.rangeStep
	default:
		return it.currentValue
	}
}

// ============================================================================
// 从传统迭代器转换
// ============================================================================

// JITIteratorFromValue 从 Value 创建 JITIterator
func JITIteratorFromValue(v Value) *JITIterator {
	it := NewJITIterator()

	switch v.Type {
	case ValArray:
		arr := v.AsArray()
		it.InitFromArray(arr)

	case ValFixedArray:
		fa := v.AsFixedArray()
		it.InitFromArray(fa.Elements)

	case ValNativeArray:
		na := v.AsNativeArray()
		it.InitFromNativeArray(na)

	case ValString:
		s := v.AsString()
		it.InitFromString(s)

	case ValMap:
		// 转换为 JITMap
		jm := JITMapFromValue(v)
		it.InitFromJITMap(jm)

	default:
		// 不支持的类型
		it.Type = JITIterTypeNone
		it.Length = 0
	}

	return it
}

// ============================================================================
// 迭代器池
// ============================================================================

var jitIteratorPool = sync.Pool{
	New: func() interface{} {
		return NewJITIterator()
	},
}

// GetJITIterator 从池中获取迭代器
func GetJITIterator() *JITIterator {
	it := jitIteratorPool.Get().(*JITIterator)
	// 重置状态
	it.Type = JITIterTypeNone
	it.Length = 0
	it.Index = -1
	it.Flags = JITIterFlagNone
	it.DataPtr = 0
	it.KeyPtr = 0
	it.ValuePtr = 0
	it.mapKeys = nil
	it.mapKeyIndex = -1
	return it
}

// PutJITIterator 将迭代器放回池
func PutJITIterator(it *JITIterator) {
	if it != nil {
		it.mapKeys = nil // 释放 map keys
		jitIteratorPool.Put(it)
	}
}

// ============================================================================
// JIT 代码生成辅助
// ============================================================================

// getFuncPtrInternal 内部函数指针获取（使用反射）
func getFuncPtrInternal(fn interface{}) uintptr {
	return *(*uintptr)((*[2]unsafe.Pointer)(unsafe.Pointer(&fn))[1])
}

// GetJITIteratorNextFunc 返回 Next 函数地址
func GetJITIteratorNextFunc() uintptr {
	return getFuncPtrInternal(JITIterNextInline)
}

// GetJITIteratorKeyFunc 返回 Key 函数地址
func GetJITIteratorKeyFunc() uintptr {
	return getFuncPtrInternal(JITIterKeyInline)
}

// GetJITIteratorValueFunc 返回 Value 函数地址
func GetJITIteratorValueFunc() uintptr {
	return getFuncPtrInternal(JITIterValueInline)
}

// ============================================================================
// NativeArray 快速迭代 (完全内联)
// ============================================================================

// JITIterNativeArrayFast 专门用于 NativeArray 的快速迭代结构
// 这个结构足够简单，可以完全被 JIT 内联
type JITIterNativeArrayFast struct {
	Data   unsafe.Pointer // 数据指针
	Length int32          // 长度
	Index  int32          // 当前索引
}

// NewJITIterNativeArrayFast 创建 NativeArray 快速迭代器
func NewJITIterNativeArrayFast(arr *NativeArray) *JITIterNativeArrayFast {
	return &JITIterNativeArrayFast{
		Data:   arr.Data,
		Length: arr.Length,
		Index:  -1,
	}
}

// Next 移动到下一个（返回是否成功）
func (it *JITIterNativeArrayFast) Next() bool {
	it.Index++
	return it.Index < it.Length
}

// Value 获取当前值
func (it *JITIterNativeArrayFast) GetValue() int64 {
	return *(*int64)(unsafe.Pointer(uintptr(it.Data) + uintptr(it.Index)*8))
}

// JITIterNativeArrayFast 偏移常量
const (
	JITIterFastOffsetData   = 0
	JITIterFastOffsetLength = 8
	JITIterFastOffsetIndex  = 12
	JITIterFastStructSize   = 16
)
