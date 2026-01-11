// object_layout.go - JIT 友好的对象内存布局
//
// 本文件定义了 JIT 编译器使用的固定内存布局对象结构。
// 与传统的 map[string]Value 字段存储不同，JITObject 使用固定偏移的内存槽，
// 使得 JIT 生成的机器码可以直接通过内存偏移访问字段，无需运行时查找。
//
// 内存布局:
//   偏移 0:  VTablePtr (8 bytes) - 虚表指针，用于方法分派
//   偏移 8:  ClassID   (4 bytes) - 类 ID，用于类型检查
//   偏移 12: Flags     (4 bytes) - 对象标志位
//   偏移 16: Fields    (N*8 bytes) - 字段槽数组

package bytecode

import (
	"sync"
	"unsafe"
)

// MaxJITFields JIT 对象支持的最大字段数
// 超过此数量的类将回退到传统对象模式
const MaxJITFields = 64

// JITObjectHeaderSize JIT 对象头部大小 (VTablePtr + ClassID + Flags)
const JITObjectHeaderSize = 16

// JITFieldSize 每个字段槽的大小 (统一 8 字节)
const JITFieldSize = 8

// JITObjectFlags 对象标志位
type JITObjectFlags uint32

const (
	JITObjFlagNone      JITObjectFlags = 0
	JITObjFlagImmutable JITObjectFlags = 1 << 0 // 不可变对象
	JITObjFlagFinalized JITObjectFlags = 1 << 1 // 已调用终结器
	JITObjFlagMarked    JITObjectFlags = 1 << 2 // GC 标记位
)

// JITObject JIT 友好的对象结构
// 使用固定内存布局，支持直接偏移访问
type JITObject struct {
	VTablePtr uintptr        // 虚表指针 (偏移 0)
	ClassID   int32          // 类 ID (偏移 8)
	Flags     JITObjectFlags // 标志位 (偏移 12)
	Fields    [MaxJITFields]int64 // 字段槽 (偏移 16)
}

// JITObjectSize 返回 JIT 对象的总大小
func JITObjectSize(fieldCount int) int {
	return JITObjectHeaderSize + fieldCount*JITFieldSize
}

// NewJITObject 创建新的 JIT 对象
func NewJITObject(classID int32, vtablePtr uintptr, fieldCount int) *JITObject {
	obj := &JITObject{
		VTablePtr: vtablePtr,
		ClassID:   classID,
		Flags:     JITObjFlagNone,
	}
	// 字段已经零初始化
	return obj
}

// GetFieldPtr 获取字段指针（用于 JIT 代码）
func (o *JITObject) GetFieldPtr(index int) unsafe.Pointer {
	if index < 0 || index >= MaxJITFields {
		return nil
	}
	return unsafe.Pointer(&o.Fields[index])
}

// GetField 获取字段值
func (o *JITObject) GetField(index int) int64 {
	if index < 0 || index >= MaxJITFields {
		return 0
	}
	return o.Fields[index]
}

// SetField 设置字段值
func (o *JITObject) SetField(index int, value int64) {
	if index >= 0 && index < MaxJITFields {
		o.Fields[index] = value
	}
}

// GetFieldAsFloat 获取字段值作为 float64
func (o *JITObject) GetFieldAsFloat(index int) float64 {
	bits := o.GetField(index)
	return *(*float64)(unsafe.Pointer(&bits))
}

// SetFieldAsFloat 设置字段值为 float64
func (o *JITObject) SetFieldAsFloat(index int, value float64) {
	bits := *(*int64)(unsafe.Pointer(&value))
	o.SetField(index, bits)
}

// GetFieldAsPtr 获取字段值作为指针
func (o *JITObject) GetFieldAsPtr(index int) uintptr {
	return uintptr(o.GetField(index))
}

// SetFieldAsPtr 设置字段值为指针
func (o *JITObject) SetFieldAsPtr(index int, ptr uintptr) {
	o.SetField(index, int64(ptr))
}

// ============================================================================
// ClassLayout - 类内存布局信息
// ============================================================================

// FieldInfo 字段信息
type FieldInfo struct {
	Name       string    // 字段名
	Index      int       // 在 Fields 数组中的索引
	Offset     int       // 相对于对象起始的字节偏移
	Type       ValueType // 字段类型
	IsPrivate  bool      // 是否私有
	IsReadonly bool      // 是否只读
}

// MethodInfo 方法信息（用于虚表）
type MethodInfo struct {
	Name        string  // 方法名
	VTableIndex int     // 在虚表中的索引
	IsVirtual   bool    // 是否虚方法
	IsStatic    bool    // 是否静态方法
	IsAbstract  bool    // 是否抽象方法
	EntryPoint  uintptr // JIT 编译后的入口点（0 表示未编译）
}

// ClassLayout 类内存布局
// 在编译期计算，运行时用于快速字段访问和方法分派
type ClassLayout struct {
	ClassName     string                // 类名
	ClassID       int32                 // 类 ID（全局唯一）
	ParentID      int32                 // 父类 ID（-1 表示无父类）
	FieldCount    int                   // 字段数量
	MethodCount   int                   // 方法数量
	ObjectSize    int                   // 对象实例大小
	Fields        map[string]*FieldInfo // 字段名 -> 字段信息
	FieldsByIndex []*FieldInfo          // 按索引排序的字段列表
	Methods       map[string]*MethodInfo // 方法名 -> 方法信息
	VTable        []uintptr             // 虚方法表
	VTablePtr     uintptr               // 虚表地址（用于快速访问）
	IsJITEnabled  bool                  // 是否启用 JIT 对象布局
}

// NewClassLayout 创建类布局
func NewClassLayout(className string, classID int32) *ClassLayout {
	return &ClassLayout{
		ClassName:     className,
		ClassID:       classID,
		ParentID:      -1,
		Fields:        make(map[string]*FieldInfo),
		FieldsByIndex: make([]*FieldInfo, 0),
		Methods:       make(map[string]*MethodInfo),
		VTable:        make([]uintptr, 0),
		IsJITEnabled:  true,
	}
}

// AddField 添加字段
func (cl *ClassLayout) AddField(name string, fieldType ValueType, isPrivate, isReadonly bool) int {
	if _, exists := cl.Fields[name]; exists {
		return -1 // 字段已存在
	}

	index := cl.FieldCount
	offset := JITObjectHeaderSize + index*JITFieldSize

	field := &FieldInfo{
		Name:       name,
		Index:      index,
		Offset:     offset,
		Type:       fieldType,
		IsPrivate:  isPrivate,
		IsReadonly: isReadonly,
	}

	cl.Fields[name] = field
	cl.FieldsByIndex = append(cl.FieldsByIndex, field)
	cl.FieldCount++
	cl.ObjectSize = JITObjectSize(cl.FieldCount)

	// 检查是否超过 JIT 限制
	if cl.FieldCount > MaxJITFields {
		cl.IsJITEnabled = false
	}

	return index
}

// GetFieldOffset 获取字段偏移（编译期使用）
func (cl *ClassLayout) GetFieldOffset(name string) int {
	if field, ok := cl.Fields[name]; ok {
		return field.Offset
	}
	return -1
}

// GetFieldIndex 获取字段索引
func (cl *ClassLayout) GetFieldIndex(name string) int {
	if field, ok := cl.Fields[name]; ok {
		return field.Index
	}
	return -1
}

// AddMethod 添加方法
func (cl *ClassLayout) AddMethod(name string, isVirtual, isStatic, isAbstract bool) int {
	vtableIndex := -1
	if isVirtual && !isStatic {
		vtableIndex = len(cl.VTable)
		cl.VTable = append(cl.VTable, 0) // 占位，稍后填充
	}

	method := &MethodInfo{
		Name:        name,
		VTableIndex: vtableIndex,
		IsVirtual:   isVirtual,
		IsStatic:    isStatic,
		IsAbstract:  isAbstract,
		EntryPoint:  0,
	}

	cl.Methods[name] = method
	cl.MethodCount++

	return vtableIndex
}

// SetMethodEntryPoint 设置方法入口点
func (cl *ClassLayout) SetMethodEntryPoint(name string, entryPoint uintptr) {
	if method, ok := cl.Methods[name]; ok {
		method.EntryPoint = entryPoint
		if method.VTableIndex >= 0 && method.VTableIndex < len(cl.VTable) {
			cl.VTable[method.VTableIndex] = entryPoint
		}
	}
}

// GetMethodVTableIndex 获取方法在虚表中的索引
func (cl *ClassLayout) GetMethodVTableIndex(name string) int {
	if method, ok := cl.Methods[name]; ok {
		return method.VTableIndex
	}
	return -1
}

// FinalizeVTable 完成虚表构建，返回虚表地址
func (cl *ClassLayout) FinalizeVTable() uintptr {
	if len(cl.VTable) == 0 {
		return 0
	}
	cl.VTablePtr = uintptr(unsafe.Pointer(&cl.VTable[0]))
	return cl.VTablePtr
}

// ============================================================================
// ClassLayoutRegistry - 类布局注册表
// ============================================================================

// ClassLayoutRegistry 全局类布局注册表
type ClassLayoutRegistry struct {
	mu       sync.RWMutex
	layouts  map[string]*ClassLayout // 类名 -> 布局
	byID     map[int32]*ClassLayout  // 类 ID -> 布局
	nextID   int32                   // 下一个可用的类 ID
}

// 全局类布局注册表实例
var globalClassRegistry *ClassLayoutRegistry
var classRegistryOnce sync.Once

// GetClassRegistry 获取全局类布局注册表
func GetClassRegistry() *ClassLayoutRegistry {
	classRegistryOnce.Do(func() {
		globalClassRegistry = &ClassLayoutRegistry{
			layouts: make(map[string]*ClassLayout),
			byID:    make(map[int32]*ClassLayout),
			nextID:  1, // 0 保留
		}
	})
	return globalClassRegistry
}

// Register 注册类布局
func (r *ClassLayoutRegistry) Register(layout *ClassLayout) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.layouts[layout.ClassName] = layout
	r.byID[layout.ClassID] = layout
}

// GetByName 通过类名获取布局
func (r *ClassLayoutRegistry) GetByName(name string) (*ClassLayout, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	layout, ok := r.layouts[name]
	return layout, ok
}

// GetByID 通过类 ID 获取布局
func (r *ClassLayoutRegistry) GetByID(id int32) (*ClassLayout, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	layout, ok := r.byID[id]
	return layout, ok
}

// AllocateClassID 分配新的类 ID
func (r *ClassLayoutRegistry) AllocateClassID() int32 {
	r.mu.Lock()
	defer r.mu.Unlock()

	id := r.nextID
	r.nextID++
	return id
}

// GetOrCreate 获取或创建类布局
func (r *ClassLayoutRegistry) GetOrCreate(className string) *ClassLayout {
	r.mu.Lock()
	defer r.mu.Unlock()

	if layout, ok := r.layouts[className]; ok {
		return layout
	}

	id := r.nextID
	r.nextID++

	layout := NewClassLayout(className, id)
	r.layouts[className] = layout
	r.byID[id] = layout

	return layout
}

// ============================================================================
// JITObject 与传统 Object 转换
// ============================================================================

// ToJITObject 将传统 Object 转换为 JITObject
func ToJITObject(obj *Object) *JITObject {
	if obj == nil || obj.Class == nil {
		return nil
	}

	registry := GetClassRegistry()
	layout, ok := registry.GetByName(obj.Class.Name)
	if !ok || !layout.IsJITEnabled {
		return nil
	}

	jitObj := NewJITObject(layout.ClassID, layout.VTablePtr, layout.FieldCount)

	// 复制字段
	for name, value := range obj.Fields {
		if field, ok := layout.Fields[name]; ok {
			jitObj.Fields[field.Index] = ValueToInt64(value)
		}
	}

	return jitObj
}

// FromJITObject 将 JITObject 转换为传统 Object
func FromJITObject(jitObj *JITObject) *Object {
	if jitObj == nil {
		return nil
	}

	registry := GetClassRegistry()
	layout, ok := registry.GetByID(jitObj.ClassID)
	if !ok {
		return nil
	}

	// 需要查找 Class 定义
	// 这里简化处理，实际需要从某处获取 Class 定义
	obj := &Object{
		Fields: make(map[string]Value),
	}

	// 复制字段
	for _, field := range layout.FieldsByIndex {
		value := Int64ToValue(jitObj.Fields[field.Index], field.Type)
		obj.Fields[field.Name] = value
	}

	return obj
}

// ValueToInt64 将 Value 转换为 int64（用于 JIT 存储）
func ValueToInt64(v Value) int64 {
	switch v.Type {
	case ValInt:
		return v.AsInt()
	case ValFloat:
		f := v.AsFloat()
		return *(*int64)(unsafe.Pointer(&f))
	case ValBool:
		if v.AsBool() {
			return 1
		}
		return 0
	case ValNull:
		return 0
	case ValString:
		// 字符串存储为指针
		s := v.AsString()
		return int64(uintptr(unsafe.Pointer(&s)))
	case ValObject:
		// 对象存储为指针
		obj := v.AsObject()
		return int64(uintptr(unsafe.Pointer(obj)))
	default:
		return 0
	}
}

// Int64ToValue 将 int64 转换回 Value
func Int64ToValue(v int64, vtype ValueType) Value {
	switch vtype {
	case ValInt:
		return NewInt(v)
	case ValFloat:
		f := *(*float64)(unsafe.Pointer(&v))
		return NewFloat(f)
	case ValBool:
		return NewBool(v != 0)
	case ValNull:
		return Value{Type: ValNull}
	case ValString:
		// 从指针恢复字符串
		if v == 0 {
			return NewString("")
		}
		s := *(*string)(unsafe.Pointer(uintptr(v)))
		return NewString(s)
	case ValObject:
		// 从指针恢复对象
		if v == 0 {
			return Value{Type: ValNull}
		}
		obj := (*Object)(unsafe.Pointer(uintptr(v)))
		return NewObject(obj)
	default:
		return Value{Type: ValNull}
	}
}

// ============================================================================
// 内存偏移常量（供 JIT 代码生成使用）
// ============================================================================

const (
	// JITObject 字段偏移
	JITObjectOffsetVTablePtr = 0
	JITObjectOffsetClassID   = 8
	JITObjectOffsetFlags     = 12
	JITObjectOffsetFields    = 16
)

// GetJITFieldOffset 计算指定索引字段的偏移
func GetJITFieldOffset(fieldIndex int) int {
	return JITObjectOffsetFields + fieldIndex*JITFieldSize
}
