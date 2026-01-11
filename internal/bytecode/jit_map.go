// jit_map.go - JIT 友好的哈希表实现
//
// 本文件实现了一个 JIT 友好的哈希表，使用开放寻址法（线性探测）。
// 与 Go 内置 map 不同，JITMap 使用固定内存布局，使得 JIT 生成的机器码
// 可以直接进行哈希查找，无需调用 Go 运行时。
//
// 内存布局:
//   JITMap 结构 (32 bytes):
//     偏移 0:  Capacity  (4 bytes) - 容量（必须是 2 的幂）
//     偏移 4:  Size      (4 bytes) - 当前元素数
//     偏移 8:  Keys      (8 bytes) - 键数组指针
//     偏移 16: Values    (8 bytes) - 值数组指针
//     偏移 24: States    (8 bytes) - 状态数组指针
//
//   状态值:
//     0 = 空槽
//     1 = 已占用
//     2 = 已删除（墓碑）

package bytecode

import (
	"sync"
	"unsafe"
)

// JITMap 状态常量
const (
	JITMapStateEmpty   byte = 0
	JITMapStateOccupied byte = 1
	JITMapStateDeleted byte = 2
)

// JITMap 默认容量
const (
	JITMapDefaultCapacity = 16
	JITMapLoadFactor      = 0.75
	JITMapGrowthFactor    = 2
)

// JITMap JIT 友好的哈希表
type JITMap struct {
	Capacity int32          // 容量（偏移 0）
	Size     int32          // 当前元素数（偏移 4）
	Keys     unsafe.Pointer // 键数组（偏移 8）
	Values   unsafe.Pointer // 值数组（偏移 16）
	States   unsafe.Pointer // 状态数组（偏移 24）
}

// JITMapEntry 哈希表条目（用于遍历）
type JITMapEntry struct {
	Key   int64
	Value int64
	Valid bool
}

// NewJITMap 创建新的 JIT 哈希表
func NewJITMap(capacity int32) *JITMap {
	if capacity < JITMapDefaultCapacity {
		capacity = JITMapDefaultCapacity
	}
	// 确保容量是 2 的幂
	capacity = nextPowerOfTwo(capacity)

	m := &JITMap{
		Capacity: capacity,
		Size:     0,
	}

	// 分配内存
	m.Keys = allocateInt64Array(int(capacity))
	m.Values = allocateInt64Array(int(capacity))
	m.States = allocateByteArray(int(capacity))

	return m
}

// 辅助函数：分配 int64 数组
func allocateInt64Array(size int) unsafe.Pointer {
	arr := make([]int64, size)
	return unsafe.Pointer(&arr[0])
}

// 辅助函数：分配 byte 数组
func allocateByteArray(size int) unsafe.Pointer {
	arr := make([]byte, size)
	return unsafe.Pointer(&arr[0])
}

// 辅助函数：计算下一个 2 的幂
func nextPowerOfTwo(n int32) int32 {
	n--
	n |= n >> 1
	n |= n >> 2
	n |= n >> 4
	n |= n >> 8
	n |= n >> 16
	n++
	return n
}

// Hash 计算哈希值（使用 FNV-1a 变体）
// 这个函数可以被 JIT 内联
func JITMapHash(key int64) uint64 {
	// 使用黄金比例乘法哈希
	const goldenRatio = 0x9E3779B97F4A7C15
	h := uint64(key) * goldenRatio
	// 高位混合
	h ^= h >> 33
	h *= 0xFF51AFD7ED558CCD
	h ^= h >> 33
	return h
}

// getKeyAt 获取指定索引的键
func (m *JITMap) getKeyAt(index int32) int64 {
	return *(*int64)(unsafe.Pointer(uintptr(m.Keys) + uintptr(index)*8))
}

// setKeyAt 设置指定索引的键
func (m *JITMap) setKeyAt(index int32, key int64) {
	*(*int64)(unsafe.Pointer(uintptr(m.Keys) + uintptr(index)*8)) = key
}

// getValueAt 获取指定索引的值
func (m *JITMap) getValueAt(index int32) int64 {
	return *(*int64)(unsafe.Pointer(uintptr(m.Values) + uintptr(index)*8))
}

// setValueAt 设置指定索引的值
func (m *JITMap) setValueAt(index int32, value int64) {
	*(*int64)(unsafe.Pointer(uintptr(m.Values) + uintptr(index)*8)) = value
}

// getStateAt 获取指定索引的状态
func (m *JITMap) getStateAt(index int32) byte {
	return *(*byte)(unsafe.Pointer(uintptr(m.States) + uintptr(index)))
}

// setStateAt 设置指定索引的状态
func (m *JITMap) setStateAt(index int32, state byte) {
	*(*byte)(unsafe.Pointer(uintptr(m.States) + uintptr(index))) = state
}

// findSlot 查找键的槽位
// 返回: (槽位索引, 是否找到)
func (m *JITMap) findSlot(key int64) (int32, bool) {
	hash := JITMapHash(key)
	mask := uint64(m.Capacity - 1)
	index := int32(hash & mask)

	// 线性探测
	for i := int32(0); i < m.Capacity; i++ {
		state := m.getStateAt(index)

		if state == JITMapStateEmpty {
			return index, false // 空槽，未找到
		}

		if state == JITMapStateOccupied && m.getKeyAt(index) == key {
			return index, true // 找到
		}

		// 继续探测
		index = int32((uint64(index) + 1) & mask)
	}

	return -1, false // 表满
}

// findInsertSlot 查找插入位置
func (m *JITMap) findInsertSlot(key int64) int32 {
	hash := JITMapHash(key)
	mask := uint64(m.Capacity - 1)
	index := int32(hash & mask)
	firstDeleted := int32(-1)

	for i := int32(0); i < m.Capacity; i++ {
		state := m.getStateAt(index)

		if state == JITMapStateEmpty {
			// 空槽，使用第一个删除槽或当前槽
			if firstDeleted >= 0 {
				return firstDeleted
			}
			return index
		}

		if state == JITMapStateDeleted && firstDeleted < 0 {
			firstDeleted = index
		}

		if state == JITMapStateOccupied && m.getKeyAt(index) == key {
			return index // 键已存在
		}

		index = int32((uint64(index) + 1) & mask)
	}

	if firstDeleted >= 0 {
		return firstDeleted
	}
	return -1 // 表满
}

// Get 获取键对应的值
func (m *JITMap) Get(key int64) (int64, bool) {
	if m.Size == 0 {
		return 0, false
	}

	index, found := m.findSlot(key)
	if !found {
		return 0, false
	}

	return m.getValueAt(index), true
}

// Set 设置键值对
func (m *JITMap) Set(key int64, value int64) {
	// 检查是否需要扩容
	if float64(m.Size+1) > float64(m.Capacity)*JITMapLoadFactor {
		m.grow()
	}

	index := m.findInsertSlot(key)
	if index < 0 {
		// 不应该发生，扩容后应该有空间
		return
	}

	if m.getStateAt(index) != JITMapStateOccupied {
		m.Size++
	}

	m.setKeyAt(index, key)
	m.setValueAt(index, value)
	m.setStateAt(index, JITMapStateOccupied)
}

// Has 检查键是否存在
func (m *JITMap) Has(key int64) bool {
	_, found := m.findSlot(key)
	return found
}

// Delete 删除键
func (m *JITMap) Delete(key int64) bool {
	index, found := m.findSlot(key)
	if !found {
		return false
	}

	m.setStateAt(index, JITMapStateDeleted)
	m.Size--
	return true
}

// Len 返回元素数量
func (m *JITMap) Len() int32 {
	return m.Size
}

// grow 扩容
func (m *JITMap) grow() {
	oldCapacity := m.Capacity
	oldKeys := m.Keys
	oldValues := m.Values
	oldStates := m.States

	// 新容量
	newCapacity := oldCapacity * JITMapGrowthFactor

	// 分配新内存
	m.Capacity = newCapacity
	m.Size = 0
	m.Keys = allocateInt64Array(int(newCapacity))
	m.Values = allocateInt64Array(int(newCapacity))
	m.States = allocateByteArray(int(newCapacity))

	// 重新插入所有元素
	for i := int32(0); i < oldCapacity; i++ {
		state := *(*byte)(unsafe.Pointer(uintptr(oldStates) + uintptr(i)))
		if state == JITMapStateOccupied {
			key := *(*int64)(unsafe.Pointer(uintptr(oldKeys) + uintptr(i)*8))
			value := *(*int64)(unsafe.Pointer(uintptr(oldValues) + uintptr(i)*8))
			m.Set(key, value)
		}
	}
}

// Clear 清空哈希表
func (m *JITMap) Clear() {
	for i := int32(0); i < m.Capacity; i++ {
		m.setStateAt(i, JITMapStateEmpty)
	}
	m.Size = 0
}

// Keys 获取所有键
func (m *JITMap) GetKeys() []int64 {
	keys := make([]int64, 0, m.Size)
	for i := int32(0); i < m.Capacity; i++ {
		if m.getStateAt(i) == JITMapStateOccupied {
			keys = append(keys, m.getKeyAt(i))
		}
	}
	return keys
}

// Values 获取所有值
func (m *JITMap) GetValues() []int64 {
	values := make([]int64, 0, m.Size)
	for i := int32(0); i < m.Capacity; i++ {
		if m.getStateAt(i) == JITMapStateOccupied {
			values = append(values, m.getValueAt(i))
		}
	}
	return values
}

// Entries 获取所有条目
func (m *JITMap) Entries() []JITMapEntry {
	entries := make([]JITMapEntry, 0, m.Size)
	for i := int32(0); i < m.Capacity; i++ {
		if m.getStateAt(i) == JITMapStateOccupied {
			entries = append(entries, JITMapEntry{
				Key:   m.getKeyAt(i),
				Value: m.getValueAt(i),
				Valid: true,
			})
		}
	}
	return entries
}

// ============================================================================
// JITMap 偏移常量（供 JIT 代码生成使用）
// ============================================================================

const (
	JITMapOffsetCapacity = 0
	JITMapOffsetSize     = 4
	JITMapOffsetKeys     = 8
	JITMapOffsetValues   = 16
	JITMapOffsetStates   = 24
	JITMapStructSize     = 32
)

// ============================================================================
// JITMap 池（减少分配）
// ============================================================================

var jitMapPool = sync.Pool{
	New: func() interface{} {
		return NewJITMap(JITMapDefaultCapacity)
	},
}

// GetJITMap 从池中获取 JITMap
func GetJITMap() *JITMap {
	m := jitMapPool.Get().(*JITMap)
	m.Clear()
	return m
}

// PutJITMap 将 JITMap 放回池中
func PutJITMap(m *JITMap) {
	if m != nil && m.Capacity <= 1024 {
		jitMapPool.Put(m)
	}
}

// ============================================================================
// 与 Value 系统的集成
// ============================================================================

// JITMapFromValue 从传统 Map 创建 JITMap
func JITMapFromValue(v Value) *JITMap {
	if v.Type() != ValMap {
		return nil
	}

	goMap := v.AsMap()
	if goMap == nil {
		return NewJITMap(JITMapDefaultCapacity)
	}

	m := NewJITMap(int32(len(goMap) * 2))
	for k, val := range goMap {
		keyInt := ValueToInt64(k)
		valInt := ValueToInt64(val)
		m.Set(keyInt, valInt)
	}

	return m
}

// JITMapToValue 将 JITMap 转换为传统 Map Value
func JITMapToValue(m *JITMap) Value {
	if m == nil {
		return NewMap(nil)
	}

	goMap := make(map[Value]Value)
	for i := int32(0); i < m.Capacity; i++ {
		if m.getStateAt(i) == JITMapStateOccupied {
			key := NewInt(m.getKeyAt(i))
			val := NewInt(m.getValueAt(i))
			goMap[key] = val
		}
	}

	return NewMap(goMap)
}

// ============================================================================
// JIT 内联辅助函数
// ============================================================================

// JITMapGetInline JIT 内联版本的 Get（返回值和是否找到的组合）
// 返回: 最高位 = 是否找到 (1/0)，其余位 = 值（如果找到）
// 使用 -1 的最高位来标记找到状态（避免溢出）
func JITMapGetInline(m *JITMap, key int64) int64 {
	if m.Size == 0 {
		return 0
	}

	hash := JITMapHash(key)
	mask := uint64(m.Capacity - 1)
	index := int32(hash & mask)

	for i := int32(0); i < m.Capacity; i++ {
		statePtr := uintptr(m.States) + uintptr(index)
		state := *(*byte)(unsafe.Pointer(statePtr))

		if state == JITMapStateEmpty {
			return 0
		}

		if state == JITMapStateOccupied {
			keyPtr := uintptr(m.Keys) + uintptr(index)*8
			if *(*int64)(unsafe.Pointer(keyPtr)) == key {
				valuePtr := uintptr(m.Values) + uintptr(index)*8
				value := *(*int64)(unsafe.Pointer(valuePtr))
				// 设置最高位表示找到，使用负数的特性
				return value | int64(-1<<62) // 设置高两位为 11
			}
		}

		index = int32((uint64(index) + 1) & mask)
	}

	return 0
}

// JITMapSetInline JIT 内联版本的 Set
func JITMapSetInline(m *JITMap, key int64, value int64) {
	m.Set(key, value)
}

// getJITMapFuncPtr 内部函数指针获取
func getJITMapFuncPtr(fn interface{}) uintptr {
	return *(*uintptr)((*[2]unsafe.Pointer)(unsafe.Pointer(&fn))[1])
}

// GetJITMapHashFunction 返回哈希函数的地址（供 JIT 使用）
func GetJITMapHashFunction() uintptr {
	return getJITMapFuncPtr(JITMapHash)
}
