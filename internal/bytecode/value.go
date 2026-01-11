package bytecode

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unsafe"
)

// ValueType 值类型
type ValueType byte

const (
	ValNull ValueType = iota
	ValBool
	ValInt
	ValFloat
	ValString
	ValArray
	ValFixedArray   // 定长数组（旧版，兼容）
	ValNativeArray  // 原生定长数组（类型化存储，JIT 友好）
	ValBytes        // 字节数组类型
	ValMap
	ValSuperArray   // PHP风格万能数组
	ValObject
	ValFunc
	ValClosure
	ValClass
	ValMethod
	ValIterator
	ValEnum          // 枚举值
	ValException     // 异常值
	ValStringBuilder // 字符串构建器（用于高效拼接）
	ValChannel       // 通道
	ValGoroutine     // 协程引用
)

// FixedArray 定长数组（旧版，兼容）
type FixedArray struct {
	Elements []Value
	Capacity int
}

// ============================================================================
// NativeArray 原生定长数组（类型化存储，JIT 友好）
// ============================================================================

// NativeArrayElementSize 元素大小（字节）
// 所有类型统一 8 字节：int64, float64, 指针
const NativeArrayElementSize = 8

// NativeArray 原生定长数组
// 使用类型化原生存储，JIT 可直接通过指针操作内存
type NativeArray struct {
	ElementType ValueType      // 元素类型 (ValInt/ValFloat/ValBool/ValString/ValObject)
	Length      int32          // 数组长度（不可变）
	Data        unsafe.Pointer // 连续内存块，每元素 8 字节
}

// NewNativeArray 创建原生数组（元素为默认值）
func NewNativeArray(elemType ValueType, length int) *NativeArray {
	if length <= 0 {
		length = 0
	}
	arr := &NativeArray{
		ElementType: elemType,
		Length:      int32(length),
	}
	if length > 0 {
		// 分配 length * 8 字节的内存
		arr.Data = allocateNativeArrayMemory(length)
		// 填充默认值（全零）
		arr.fillDefaults()
	}
	return arr
}

// NewNativeArrayFromInts 从 int64 切片创建数组
func NewNativeArrayFromInts(values []int64) *NativeArray {
	length := len(values)
	arr := &NativeArray{
		ElementType: ValInt,
		Length:      int32(length),
	}
	if length > 0 {
		arr.Data = allocateNativeArrayMemory(length)
		for i, v := range values {
			arr.SetInt(i, v)
		}
	}
	return arr
}

// NewNativeArrayFromFloats 从 float64 切片创建数组
func NewNativeArrayFromFloats(values []float64) *NativeArray {
	length := len(values)
	arr := &NativeArray{
		ElementType: ValFloat,
		Length:      int32(length),
	}
	if length > 0 {
		arr.Data = allocateNativeArrayMemory(length)
		for i, v := range values {
			arr.SetFloat(i, v)
		}
	}
	return arr
}

// NewNativeArrayFromBools 从 bool 切片创建数组
func NewNativeArrayFromBools(values []bool) *NativeArray {
	length := len(values)
	arr := &NativeArray{
		ElementType: ValBool,
		Length:      int32(length),
	}
	if length > 0 {
		arr.Data = allocateNativeArrayMemory(length)
		for i, v := range values {
			var intVal int64
			if v {
				intVal = 1
			}
			arr.SetInt(i, intVal)
		}
	}
	return arr
}

// NewNativeArrayFromValues 从 Value 切片创建数组
func NewNativeArrayFromValues(elemType ValueType, values []Value) *NativeArray {
	length := len(values)
	arr := &NativeArray{
		ElementType: elemType,
		Length:      int32(length),
	}
	if length > 0 {
		arr.Data = allocateNativeArrayMemory(length)
		for i, v := range values {
			arr.Set(i, v)
		}
	}
	return arr
}

// allocateNativeArrayMemory 分配原生内存
func allocateNativeArrayMemory(length int) unsafe.Pointer {
	// 使用 Go 的 slice 底层数组作为内存存储
	// 这样可以让 GC 正确管理内存
	data := make([]int64, length)
	return unsafe.Pointer(&data[0])
}

// fillDefaults 填充默认值
func (arr *NativeArray) fillDefaults() {
	if arr.Data == nil || arr.Length == 0 {
		return
	}
	// 默认全为 0，int64 切片初始化已经是 0
	// 无需额外操作
}

// Len 获取长度
func (arr *NativeArray) Len() int {
	return int(arr.Length)
}

// GetInt 获取整数元素
func (arr *NativeArray) GetInt(index int) int64 {
	if index < 0 || index >= int(arr.Length) {
		return 0
	}
	ptr := unsafe.Pointer(uintptr(arr.Data) + uintptr(index)*NativeArrayElementSize)
	return *(*int64)(ptr)
}

// SetInt 设置整数元素
func (arr *NativeArray) SetInt(index int, value int64) {
	if index < 0 || index >= int(arr.Length) {
		return
	}
	ptr := unsafe.Pointer(uintptr(arr.Data) + uintptr(index)*NativeArrayElementSize)
	*(*int64)(ptr) = value
}

// GetFloat 获取浮点元素
func (arr *NativeArray) GetFloat(index int) float64 {
	if index < 0 || index >= int(arr.Length) {
		return 0
	}
	ptr := unsafe.Pointer(uintptr(arr.Data) + uintptr(index)*NativeArrayElementSize)
	return *(*float64)(ptr)
}

// SetFloat 设置浮点元素
func (arr *NativeArray) SetFloat(index int, value float64) {
	if index < 0 || index >= int(arr.Length) {
		return
	}
	ptr := unsafe.Pointer(uintptr(arr.Data) + uintptr(index)*NativeArrayElementSize)
	*(*float64)(ptr) = value
}

// GetBool 获取布尔元素
func (arr *NativeArray) GetBool(index int) bool {
	return arr.GetInt(index) != 0
}

// SetBool 设置布尔元素
func (arr *NativeArray) SetBool(index int, value bool) {
	var intVal int64
	if value {
		intVal = 1
	}
	arr.SetInt(index, intVal)
}

// GetPtr 获取指针元素（用于 string/object）
func (arr *NativeArray) GetPtr(index int) unsafe.Pointer {
	if index < 0 || index >= int(arr.Length) {
		return nil
	}
	ptr := unsafe.Pointer(uintptr(arr.Data) + uintptr(index)*NativeArrayElementSize)
	return *(*unsafe.Pointer)(ptr)
}

// SetPtr 设置指针元素
func (arr *NativeArray) SetPtr(index int, value unsafe.Pointer) {
	if index < 0 || index >= int(arr.Length) {
		return
	}
	ptr := unsafe.Pointer(uintptr(arr.Data) + uintptr(index)*NativeArrayElementSize)
	*(*unsafe.Pointer)(ptr) = value
}

// Get 获取元素（通用方法，返回 Value）
func (arr *NativeArray) Get(index int) Value {
	if index < 0 || index >= int(arr.Length) {
		return NullValue
	}
	switch arr.ElementType {
	case ValInt:
		return NewInt(arr.GetInt(index))
	case ValFloat:
		return NewFloat(arr.GetFloat(index))
	case ValBool:
		return NewBool(arr.GetBool(index))
	case ValString:
		ptr := arr.GetPtr(index)
		if ptr == nil {
			return NewString("")
		}
		return NewString(*(*string)(ptr))
	case ValObject:
		ptr := arr.GetPtr(index)
		if ptr == nil {
			return NullValue
		}
		return NewObject((*Object)(ptr))
	default:
		return NullValue
	}
}

// Set 设置元素（通用方法）
func (arr *NativeArray) Set(index int, value Value) {
	if index < 0 || index >= int(arr.Length) {
		return
	}
	switch arr.ElementType {
	case ValInt:
		arr.SetInt(index, value.AsInt())
	case ValFloat:
		arr.SetFloat(index, value.AsFloat())
	case ValBool:
		arr.SetBool(index, value.AsBool())
	case ValString:
		s := value.AsString()
		arr.SetPtr(index, unsafe.Pointer(&s))
	case ValObject:
		if value.Type == ValObject {
			arr.SetPtr(index, unsafe.Pointer(value.AsObject()))
		} else {
			arr.SetPtr(index, nil)
		}
	}
}

// Copy 深拷贝数组
func (arr *NativeArray) Copy() *NativeArray {
	newArr := NewNativeArray(arr.ElementType, int(arr.Length))
	if arr.Length > 0 && arr.Data != nil {
		// 复制内存
		for i := 0; i < int(arr.Length); i++ {
			switch arr.ElementType {
			case ValInt, ValBool:
				newArr.SetInt(i, arr.GetInt(i))
			case ValFloat:
				newArr.SetFloat(i, arr.GetFloat(i))
			case ValString:
				// 字符串需要深拷贝
				ptr := arr.GetPtr(i)
				if ptr != nil {
					s := *(*string)(ptr)
					sCopy := s // Go 字符串是不可变的，直接复制引用即可
					newArr.SetPtr(i, unsafe.Pointer(&sCopy))
				}
			case ValObject:
				// 对象只拷贝引用
				newArr.SetPtr(i, arr.GetPtr(i))
			}
		}
	}
	return newArr
}

// ToValues 转换为 Value 切片
func (arr *NativeArray) ToValues() []Value {
	result := make([]Value, arr.Length)
	for i := 0; i < int(arr.Length); i++ {
		result[i] = arr.Get(i)
	}
	return result
}

// ToSuperArray 转换为 SuperArray
func (arr *NativeArray) ToSuperArray() *SuperArray {
	sa := NewSuperArray()
	for i := 0; i < int(arr.Length); i++ {
		sa.Push(arr.Get(i))
	}
	return sa
}

// GetElementPtr 获取元素指针（JIT 使用）
func (arr *NativeArray) GetElementPtr(index int) unsafe.Pointer {
	if index < 0 || index >= int(arr.Length) {
		return nil
	}
	return unsafe.Pointer(uintptr(arr.Data) + uintptr(index)*NativeArrayElementSize)
}

// ============================================================================
// NativeArray 语法糖方法
// ============================================================================

// IndexOf 查找元素第一次出现的索引
func (arr *NativeArray) IndexOf(value Value) int {
	for i := 0; i < int(arr.Length); i++ {
		if arr.Get(i).Equals(value) {
			return i
		}
	}
	return -1
}

// LastIndexOf 查找元素最后一次出现的索引
func (arr *NativeArray) LastIndexOf(value Value) int {
	for i := int(arr.Length) - 1; i >= 0; i-- {
		if arr.Get(i).Equals(value) {
			return i
		}
	}
	return -1
}

// Contains 检查是否包含元素
func (arr *NativeArray) Contains(value Value) bool {
	return arr.IndexOf(value) >= 0
}

// Reverse 原地反转数组
func (arr *NativeArray) Reverse() {
	n := int(arr.Length)
	for i := 0; i < n/2; i++ {
		j := n - 1 - i
		// 交换元素
		switch arr.ElementType {
		case ValInt, ValBool:
			a, b := arr.GetInt(i), arr.GetInt(j)
			arr.SetInt(i, b)
			arr.SetInt(j, a)
		case ValFloat:
			a, b := arr.GetFloat(i), arr.GetFloat(j)
			arr.SetFloat(i, b)
			arr.SetFloat(j, a)
		default:
			a, b := arr.GetPtr(i), arr.GetPtr(j)
			arr.SetPtr(i, b)
			arr.SetPtr(j, a)
		}
	}
}

// Sort 原地升序排序
func (arr *NativeArray) Sort() {
	n := int(arr.Length)
	switch arr.ElementType {
	case ValInt:
		// 提取值排序
		values := make([]int64, n)
		for i := 0; i < n; i++ {
			values[i] = arr.GetInt(i)
		}
		sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
		for i := 0; i < n; i++ {
			arr.SetInt(i, values[i])
		}
	case ValFloat:
		values := make([]float64, n)
		for i := 0; i < n; i++ {
			values[i] = arr.GetFloat(i)
		}
		sort.Float64s(values)
		for i := 0; i < n; i++ {
			arr.SetFloat(i, values[i])
		}
	case ValString:
		values := make([]string, n)
		for i := 0; i < n; i++ {
			ptr := arr.GetPtr(i)
			if ptr != nil {
				values[i] = *(*string)(ptr)
			}
		}
		sort.Strings(values)
		for i := 0; i < n; i++ {
			s := values[i]
			arr.SetPtr(i, unsafe.Pointer(&s))
		}
	}
}

// SortDesc 原地降序排序
func (arr *NativeArray) SortDesc() {
	arr.Sort()
	arr.Reverse()
}

// Slice 获取切片（返回新数组）
func (arr *NativeArray) Slice(start, end int) *NativeArray {
	n := int(arr.Length)
	if start < 0 {
		start = 0
	}
	if end > n {
		end = n
	}
	if start >= end {
		return NewNativeArray(arr.ElementType, 0)
	}
	
	length := end - start
	newArr := NewNativeArray(arr.ElementType, length)
	for i := 0; i < length; i++ {
		switch arr.ElementType {
		case ValInt, ValBool:
			newArr.SetInt(i, arr.GetInt(start+i))
		case ValFloat:
			newArr.SetFloat(i, arr.GetFloat(start+i))
		default:
			newArr.SetPtr(i, arr.GetPtr(start+i))
		}
	}
	return newArr
}

// Concat 连接另一个数组（返回新数组）
func (arr *NativeArray) Concat(other *NativeArray) *NativeArray {
	if other == nil || other.Length == 0 {
		return arr.Copy()
	}
	
	newLength := int(arr.Length) + int(other.Length)
	newArr := NewNativeArray(arr.ElementType, newLength)
	
	// 复制第一个数组
	for i := 0; i < int(arr.Length); i++ {
		switch arr.ElementType {
		case ValInt, ValBool:
			newArr.SetInt(i, arr.GetInt(i))
		case ValFloat:
			newArr.SetFloat(i, arr.GetFloat(i))
		default:
			newArr.SetPtr(i, arr.GetPtr(i))
		}
	}
	
	// 复制第二个数组
	offset := int(arr.Length)
	for i := 0; i < int(other.Length); i++ {
		switch arr.ElementType {
		case ValInt, ValBool:
			newArr.SetInt(offset+i, other.GetInt(i))
		case ValFloat:
			newArr.SetFloat(offset+i, other.GetFloat(i))
		default:
			newArr.SetPtr(offset+i, other.GetPtr(i))
		}
	}
	
	return newArr
}

// Sum 求和（数值数组）
func (arr *NativeArray) Sum() Value {
	switch arr.ElementType {
	case ValInt:
		var sum int64
		for i := 0; i < int(arr.Length); i++ {
			sum += arr.GetInt(i)
		}
		return NewInt(sum)
	case ValFloat:
		var sum float64
		for i := 0; i < int(arr.Length); i++ {
			sum += arr.GetFloat(i)
		}
		return NewFloat(sum)
	default:
		return NewInt(0)
	}
}

// Max 最大值
func (arr *NativeArray) Max() Value {
	if arr.Length == 0 {
		return NullValue
	}
	switch arr.ElementType {
	case ValInt:
		max := arr.GetInt(0)
		for i := 1; i < int(arr.Length); i++ {
			if v := arr.GetInt(i); v > max {
				max = v
			}
		}
		return NewInt(max)
	case ValFloat:
		max := arr.GetFloat(0)
		for i := 1; i < int(arr.Length); i++ {
			if v := arr.GetFloat(i); v > max {
				max = v
			}
		}
		return NewFloat(max)
	default:
		return NullValue
	}
}

// Min 最小值
func (arr *NativeArray) Min() Value {
	if arr.Length == 0 {
		return NullValue
	}
	switch arr.ElementType {
	case ValInt:
		min := arr.GetInt(0)
		for i := 1; i < int(arr.Length); i++ {
			if v := arr.GetInt(i); v < min {
				min = v
			}
		}
		return NewInt(min)
	case ValFloat:
		min := arr.GetFloat(0)
		for i := 1; i < int(arr.Length); i++ {
			if v := arr.GetFloat(i); v < min {
				min = v
			}
		}
		return NewFloat(min)
	default:
		return NullValue
	}
}

// Average 平均值
func (arr *NativeArray) Average() Value {
	if arr.Length == 0 {
		return NewFloat(math.NaN())
	}
	switch arr.ElementType {
	case ValInt:
		var sum int64
		for i := 0; i < int(arr.Length); i++ {
			sum += arr.GetInt(i)
		}
		return NewFloat(float64(sum) / float64(arr.Length))
	case ValFloat:
		var sum float64
		for i := 0; i < int(arr.Length); i++ {
			sum += arr.GetFloat(i)
		}
		return NewFloat(sum / float64(arr.Length))
	default:
		return NewFloat(math.NaN())
	}
}

// Equals 比较两个数组是否相等
func (arr *NativeArray) Equals(other *NativeArray) bool {
	if other == nil {
		return false
	}
	if arr.ElementType != other.ElementType {
		return false
	}
	if arr.Length != other.Length {
		return false
	}
	for i := 0; i < int(arr.Length); i++ {
		if !arr.Get(i).Equals(other.Get(i)) {
			return false
		}
	}
	return true
}

// String 字符串表示
func (arr *NativeArray) String() string {
	var parts []string
	for i := 0; i < int(arr.Length); i++ {
		parts = append(parts, arr.Get(i).String())
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// SuperArray PHP风格万能数组
// 特性: 有序存储、支持整数/字符串混合键、自动索引管理
type SuperArray struct {
	Entries []SuperArrayEntry // 保持插入顺序
	Index   map[string]int    // key字符串表示 -> entries下标，O(1)查找
	NextInt int64             // 下一个自动分配的整数索引
}

// SuperArrayEntry 万能数组条目
type SuperArrayEntry struct {
	Key   Value
	Value Value
}

// NewSuperArray 创建空的万能数组
func NewSuperArray() *SuperArray {
	return &SuperArray{
		Entries: make([]SuperArrayEntry, 0),
		Index:   make(map[string]int),
		NextInt: 0,
	}
}

// keyToString 将 key 转换为字符串用于索引
func (sa *SuperArray) keyToString(key Value) string {
	switch key.Type {
	case ValInt:
		return fmt.Sprintf("i:%d", key.Data.(int64))
	case ValString:
		return fmt.Sprintf("s:%s", key.Data.(string))
	default:
		return fmt.Sprintf("o:%v", key.Data)
	}
}

// Len 获取长度
func (sa *SuperArray) Len() int {
	return len(sa.Entries)
}

// Get 获取元素
func (sa *SuperArray) Get(key Value) (Value, bool) {
	keyStr := sa.keyToString(key)
	if idx, ok := sa.Index[keyStr]; ok {
		return sa.Entries[idx].Value, true
	}
	return NullValue, false
}

// Set 设置元素（如果存在则更新，否则追加）
func (sa *SuperArray) Set(key Value, value Value) {
	keyStr := sa.keyToString(key)
	if idx, ok := sa.Index[keyStr]; ok {
		// 更新现有元素
		sa.Entries[idx].Value = value
	} else {
		// 追加新元素
		sa.Index[keyStr] = len(sa.Entries)
		sa.Entries = append(sa.Entries, SuperArrayEntry{Key: key, Value: value})
		// 更新 nextInt
		if key.Type == ValInt {
			intKey := key.Data.(int64)
			if intKey >= sa.NextInt {
				sa.NextInt = intKey + 1
			}
		}
	}
}

// Push 追加元素（使用自动索引）
func (sa *SuperArray) Push(value Value) {
	key := NewInt(sa.NextInt)
	sa.Set(key, value)
}

// HasKey 检查 key 是否存在
func (sa *SuperArray) HasKey(key Value) bool {
	keyStr := sa.keyToString(key)
	_, ok := sa.Index[keyStr]
	return ok
}

// Remove 删除元素
func (sa *SuperArray) Remove(key Value) bool {
	keyStr := sa.keyToString(key)
	idx, ok := sa.Index[keyStr]
	if !ok {
		return false
	}

	// 从 entries 中删除
	sa.Entries = append(sa.Entries[:idx], sa.Entries[idx+1:]...)

	// 重建索引
	delete(sa.Index, keyStr)
	for i := idx; i < len(sa.Entries); i++ {
		sa.Index[sa.keyToString(sa.Entries[i].Key)] = i
	}

	return true
}

// Keys 获取所有 key
func (sa *SuperArray) Keys() []Value {
	keys := make([]Value, len(sa.Entries))
	for i, entry := range sa.Entries {
		keys[i] = entry.Key
	}
	return keys
}

// Values 获取所有 value
func (sa *SuperArray) Values() []Value {
	values := make([]Value, len(sa.Entries))
	for i, entry := range sa.Entries {
		values[i] = entry.Value
	}
	return values
}

// Copy 复制万能数组
func (sa *SuperArray) Copy() *SuperArray {
	newSa := &SuperArray{
		Entries: make([]SuperArrayEntry, len(sa.Entries)),
		Index:   make(map[string]int),
		NextInt: sa.NextInt,
	}
	copy(newSa.Entries, sa.Entries)
	for k, v := range sa.Index {
		newSa.Index[k] = v
	}
	return newSa
}

// StackFrame 堆栈帧信息（用于堆栈跟踪）
type StackFrame struct {
	FunctionName string // 函数/方法名
	FileName     string // 源文件名
	LineNumber   int    // 行号
	ClassName    string // 所属类名（可选，方法调用时有效）
}

// Exception 异常对象
// 支持两种模式：
// 1. 简单异常：只有 Type/Message/Code/Cause/StackFrames（用于原生异常或字符串异常）
// 2. 对象异常：包含一个 Sola Object（用于 throw new Exception(...) 创建的异常）
type Exception struct {
	Type        string       // 异常类型名 (如 "Exception", "RuntimeException")
	Message     string       // 异常消息
	Code        int64        // 异常代码
	Cause       *Exception   // 链式异常：导致此异常的原因
	StackFrames []StackFrame // 结构化的调用栈信息
	Object      *Object      // 关联的 Sola 对象（如果异常是从类实例化的）
	File        string       // 异常抛出的文件
	Line        int          // 异常抛出的行号
}

// NewException 创建异常值
func NewException(typeName, message string, code int64) Value {
	return Value{
		Type: ValException,
		Data: &Exception{
			Type:    typeName,
			Message: message,
			Code:    code,
		},
	}
}

// NewExceptionWithCause 创建带原因的异常值
func NewExceptionWithCause(typeName, message string, code int64, cause *Exception) Value {
	return Value{
		Type: ValException,
		Data: &Exception{
			Type:    typeName,
			Message: message,
			Code:    code,
			Cause:   cause,
		},
	}
}

// NewExceptionFromObject 从 Sola 对象创建异常值
// 对象必须是 Throwable 或其子类的实例
func NewExceptionFromObject(obj *Object) Value {
	// 从对象中提取异常信息
	message := ""
	code := int64(0)
	var cause *Exception

	if msgVal, ok := obj.Fields["message"]; ok {
		message = msgVal.AsString()
	}
	if codeVal, ok := obj.Fields["code"]; ok {
		code = codeVal.AsInt()
	}
	if prevVal, ok := obj.Fields["previous"]; ok && prevVal.Type == ValException {
		cause = prevVal.AsException()
	}

	return Value{
		Type: ValException,
		Data: &Exception{
			Type:    obj.Class.Name,
			Message: message,
			Code:    code,
			Cause:   cause,
			Object:  obj,
		},
	}
}

// GetExceptionObject 获取异常关联的 Sola 对象
func (e *Exception) GetExceptionObject() *Object {
	return e.Object
}

// IsObjectException 检查是否是对象异常
func (e *Exception) IsObjectException() bool {
	return e.Object != nil
}

// SetStackFrames 设置异常的调用栈
func (e *Exception) SetStackFrames(frames []StackFrame) {
	e.StackFrames = frames
	// 同时设置文件和行号（取第一帧）
	if len(frames) > 0 {
		e.File = frames[0].FileName
		e.Line = frames[0].LineNumber
	}
	
	// 如果有关联的 Sola 对象，同步更新其 stackTrace 字段
	if e.Object != nil {
		// 将 StackFrames 转换为 Sola 字符串数组
		arr := make([]Value, len(frames))
		for i, f := range frames {
			var frameStr string
			if f.ClassName != "" {
				frameStr = fmt.Sprintf("%s.%s (%s:%d)", f.ClassName, f.FunctionName, f.FileName, f.LineNumber)
			} else if f.FileName != "" {
				frameStr = fmt.Sprintf("%s (%s:%d)", f.FunctionName, f.FileName, f.LineNumber)
			} else {
				frameStr = fmt.Sprintf("%s (line %d)", f.FunctionName, f.LineNumber)
			}
			arr[i] = NewString(frameStr)
		}
		e.Object.Fields["stackTrace"] = NewArray(arr)
		
		// 同时设置 file 和 line 字段（如果存在）
		if len(frames) > 0 {
			e.Object.Fields["file"] = NewString(frames[0].FileName)
			e.Object.Fields["line"] = NewInt(int64(frames[0].LineNumber))
		}
	}
}

// GetFullMessage 获取包含异常链的完整消息
func (e *Exception) GetFullMessage() string {
	var result string
	current := e
	depth := 0
	for current != nil {
		if depth > 0 {
			result += "\nCaused by: "
		}
		
		// 如果是对象异常，尝试从对象获取最新的 message
		message := current.Message
		if current.Object != nil {
			if msgVal, ok := current.Object.Fields["message"]; ok {
				message = msgVal.AsString()
			}
		}
		
		result += fmt.Sprintf("%s: %s", current.Type, message)
		if len(current.StackFrames) > 0 {
			for _, frame := range current.StackFrames {
				if frame.ClassName != "" {
					result += fmt.Sprintf("\n    at %s.%s (%s:%d)", 
						frame.ClassName, frame.FunctionName, frame.FileName, frame.LineNumber)
				} else {
					result += fmt.Sprintf("\n    at %s (%s:%d)", 
						frame.FunctionName, frame.FileName, frame.LineNumber)
				}
			}
		}
		current = current.Cause
		depth++
		// 防止无限循环
		if depth > 10 {
			result += "\n... (exception chain too deep)"
			break
		}
	}
	return result
}

// GetStackTraceAsString 获取格式化的堆栈跟踪字符串
func (e *Exception) GetStackTraceAsString() string {
	var result string
	for i, frame := range e.StackFrames {
		if i > 0 {
			result += "\n"
		}
		if frame.ClassName != "" {
			result += fmt.Sprintf("    at %s.%s (%s:%d)", 
				frame.ClassName, frame.FunctionName, frame.FileName, frame.LineNumber)
		} else {
			result += fmt.Sprintf("    at %s (%s:%d)", 
				frame.FunctionName, frame.FileName, frame.LineNumber)
		}
	}
	return result
}

// Value 运行时值
type Value struct {
	Type ValueType
	Data interface{}
}

// 预定义常量值
var (
	NullValue  = Value{Type: ValNull}
	TrueValue  = Value{Type: ValBool, Data: true}
	FalseValue = Value{Type: ValBool, Data: false}
	ZeroValue  = Value{Type: ValInt, Data: int64(0)}
	OneValue   = Value{Type: ValInt, Data: int64(1)}
)

// NewNull 创建 null 值
func NewNull() Value {
	return NullValue
}

// NewBool 创建布尔值
func NewBool(b bool) Value {
	if b {
		return TrueValue
	}
	return FalseValue
}

// NewInt 创建整数值
func NewInt(n int64) Value {
	return Value{Type: ValInt, Data: n}
}

// NewFloat 创建浮点数值
func NewFloat(f float64) Value {
	return Value{Type: ValFloat, Data: f}
}

// NewString 创建字符串值
func NewString(s string) Value {
	return Value{Type: ValString, Data: s}
}

// StringBuilder 字符串构建器（用于高效拼接）
type StringBuilder struct {
	Parts []string // 待拼接的字符串片段
	Len   int      // 总长度（预计算，用于 strings.Builder 预分配）
}

// NewStringBuilder 创建字符串构建器
func NewStringBuilder() *StringBuilder {
	return &StringBuilder{
		Parts: make([]string, 0, 4),
		Len:   0,
	}
}

// Append 追加字符串
func (sb *StringBuilder) Append(s string) {
	sb.Parts = append(sb.Parts, s)
	sb.Len += len(s)
}

// AppendValue 追加值（转换为字符串）
func (sb *StringBuilder) AppendValue(v Value) {
	s := v.String()
	sb.Parts = append(sb.Parts, s)
	sb.Len += len(s)
}

// Build 构建最终字符串
func (sb *StringBuilder) Build() string {
	if len(sb.Parts) == 0 {
		return ""
	}
	if len(sb.Parts) == 1 {
		return sb.Parts[0]
	}
	// 使用 strings.Builder 高效拼接
	var builder strings.Builder
	builder.Grow(sb.Len)
	for _, part := range sb.Parts {
		builder.WriteString(part)
	}
	return builder.String()
}

// NewStringBuilderValue 创建字符串构建器值
func NewStringBuilderValue(sb *StringBuilder) Value {
	return Value{Type: ValStringBuilder, Data: sb}
}

// AsStringBuilder 获取字符串构建器
func (v Value) AsStringBuilder() *StringBuilder {
	if v.Type == ValStringBuilder {
		return v.Data.(*StringBuilder)
	}
	return nil
}

// NewChannelValue 创建通道值
// 注意: channel 数据结构在 vm 包中定义，这里只存储接口引用
func NewChannelValue(ch interface{}) Value {
	return Value{Type: ValChannel, Data: ch}
}

// AsChannel 获取通道（返回 interface{}，需要在 vm 包中断言）
func (v Value) AsChannel() interface{} {
	if v.Type == ValChannel {
		return v.Data
	}
	return nil
}

// IsChannel 检查是否为通道
func (v Value) IsChannel() bool {
	return v.Type == ValChannel
}

// ============================================================================
// Coroutine 协程对象 (OOP 风格)
// ============================================================================

// CoroutineStatus 协程状态
type CoroutineStatus int

const (
	CoroutinePending   CoroutineStatus = iota // 等待执行
	CoroutineRunning                          // 运行中
	CoroutineCompleted                        // 正常完成
	CoroutineFailed                           // 异常终止
	CoroutineCancelled                        // 已取消
)

// String 返回状态的字符串表示
func (s CoroutineStatus) String() string {
	switch s {
	case CoroutinePending:
		return "pending"
	case CoroutineRunning:
		return "running"
	case CoroutineCompleted:
		return "completed"
	case CoroutineFailed:
		return "failed"
	case CoroutineCancelled:
		return "cancelled"
	default:
		return "unknown"
	}
}

// CoroutineObject 协程对象（OOP 风格）
// 用于支持 Coroutine<T> API
type CoroutineObject struct {
	ID        int64           // 协程唯一标识符
	Status    CoroutineStatus // 当前状态
	Result    Value           // 完成时的返回值
	Exception Value           // 失败时的异常
	WaiterIDs []int64         // 等待此协程完成的协程 ID 列表
}

// NewCoroutineObject 创建协程对象
func NewCoroutineObject(id int64) *CoroutineObject {
	return &CoroutineObject{
		ID:        id,
		Status:    CoroutinePending,
		Result:    NullValue,
		Exception: NullValue,
		WaiterIDs: make([]int64, 0),
	}
}

// IsCompleted 检查协程是否已完成（成功、失败或取消）
func (c *CoroutineObject) IsCompleted() bool {
	return c.Status == CoroutineCompleted || c.Status == CoroutineFailed || c.Status == CoroutineCancelled
}

// IsSucceeded 检查协程是否成功完成
func (c *CoroutineObject) IsSucceeded() bool {
	return c.Status == CoroutineCompleted
}

// IsFailed 检查协程是否失败
func (c *CoroutineObject) IsFailed() bool {
	return c.Status == CoroutineFailed
}

// IsCancelled 检查协程是否已取消
func (c *CoroutineObject) IsCancelled() bool {
	return c.Status == CoroutineCancelled
}

// Complete 标记协程成功完成
func (c *CoroutineObject) Complete(result Value) {
	c.Status = CoroutineCompleted
	c.Result = result
}

// Fail 标记协程失败
func (c *CoroutineObject) Fail(exception Value) {
	c.Status = CoroutineFailed
	c.Exception = exception
}

// Cancel 标记协程取消
func (c *CoroutineObject) Cancel() {
	c.Status = CoroutineCancelled
}

// AddWaiter 添加等待者
func (c *CoroutineObject) AddWaiter(waiterID int64) {
	c.WaiterIDs = append(c.WaiterIDs, waiterID)
}

// NewCoroutineValue 创建协程值（OOP 风格）
func NewCoroutineValue(co *CoroutineObject) Value {
	return Value{Type: ValGoroutine, Data: co}
}

// AsCoroutine 获取协程对象
func (v Value) AsCoroutine() *CoroutineObject {
	if v.Type == ValGoroutine {
		if co, ok := v.Data.(*CoroutineObject); ok {
			return co
		}
		// 兼容旧格式（直接存储 ID）
		if id, ok := v.Data.(int64); ok {
			return NewCoroutineObject(id)
		}
	}
	return nil
}

// AsGoroutineID 获取协程 ID
func (v Value) AsGoroutineID() int64 {
	if v.Type == ValGoroutine {
		if co, ok := v.Data.(*CoroutineObject); ok {
			return co.ID
		}
		if id, ok := v.Data.(int64); ok {
			return id
		}
	}
	return -1
}

// IsGoroutine 检查是否为协程引用
func (v Value) IsGoroutine() bool {
	return v.Type == ValGoroutine
}

// IsCoroutine 检查是否为协程对象（别名）
func (v Value) IsCoroutine() bool {
	return v.Type == ValGoroutine
}

// NewArray 创建数组值
func NewArray(arr []Value) Value {
	return Value{Type: ValArray, Data: arr}
}

// NewFixedArray 创建定长数组值
func NewFixedArray(capacity int) Value {
	return Value{Type: ValFixedArray, Data: &FixedArray{
		Elements: make([]Value, capacity),
		Capacity: capacity,
	}}
}

// NewFixedArrayWithElements 创建带初始值的定长数组
func NewFixedArrayWithElements(elements []Value, capacity int) Value {
	arr := &FixedArray{
		Elements: make([]Value, capacity),
		Capacity: capacity,
	}
	// 复制初始元素
	copy(arr.Elements, elements)
	// 剩余位置填充 null
	for i := len(elements); i < capacity; i++ {
		arr.Elements[i] = NullValue
	}
	return Value{Type: ValFixedArray, Data: arr}
}

// NewNativeArrayValue 创建原生数组值
func NewNativeArrayValue(arr *NativeArray) Value {
	return Value{Type: ValNativeArray, Data: arr}
}

// NewNativeArrayFromValueSlice 从 Value 切片创建原生数组值
func NewNativeArrayFromValueSlice(elemType ValueType, values []Value) Value {
	return Value{Type: ValNativeArray, Data: NewNativeArrayFromValues(elemType, values)}
}

// NewMap 创建 Map 值
func NewMap(m map[Value]Value) Value {
	return Value{Type: ValMap, Data: m}
}

// NewSuperArrayValue 创建万能数组值
func NewSuperArrayValue(sa *SuperArray) Value {
	return Value{Type: ValSuperArray, Data: sa}
}

// NewEmptySuperArray 创建空万能数组值
func NewEmptySuperArray() Value {
	return Value{Type: ValSuperArray, Data: NewSuperArray()}
}

// NewBytes 创建字节数组值
func NewBytes(b []byte) Value {
	return Value{Type: ValBytes, Data: b}
}

// NewObject 创建对象值
func NewObject(obj *Object) Value {
	return Value{Type: ValObject, Data: obj}
}

// NewFunc 创建函数值
func NewFunc(fn *Function) Value {
	return Value{Type: ValFunc, Data: fn}
}

// NewClosure 创建闭包值
func NewClosure(closure *Closure) Value {
	return Value{Type: ValClosure, Data: closure}
}

// IsNull 检查是否为 null
func (v Value) IsNull() bool {
	return v.Type == ValNull
}

// IsTruthy 检查是否为真值
func (v Value) IsTruthy() bool {
	switch v.Type {
	case ValNull:
		return false
	case ValBool:
		return v.Data.(bool)
	case ValInt:
		return v.Data.(int64) != 0
	case ValFloat:
		return v.Data.(float64) != 0
	case ValString:
		return v.Data.(string) != ""
	case ValArray:
		return len(v.Data.([]Value)) > 0
	case ValFixedArray:
		return v.Data.(*FixedArray).Capacity > 0
	case ValNativeArray:
		return v.Data.(*NativeArray).Length > 0
	case ValMap:
		return len(v.Data.(map[Value]Value)) > 0
	case ValSuperArray:
		return v.Data.(*SuperArray).Len() > 0
	case ValBytes:
		return len(v.Data.([]byte)) > 0
	default:
		return true
	}
}

// AsBool 转换为布尔值
func (v Value) AsBool() bool {
	if v.Type == ValBool {
		return v.Data.(bool)
	}
	return v.IsTruthy()
}

// AsInt 转换为整数
func (v Value) AsInt() int64 {
	switch v.Type {
	case ValInt:
		return v.Data.(int64)
	case ValFloat:
		return int64(v.Data.(float64))
	case ValBool:
		if v.Data.(bool) {
			return 1
		}
		return 0
	default:
		return 0
	}
}

// AsFloat 转换为浮点数
func (v Value) AsFloat() float64 {
	switch v.Type {
	case ValFloat:
		return v.Data.(float64)
	case ValInt:
		return float64(v.Data.(int64))
	default:
		return 0
	}
}

// AsString 转换为字符串
func (v Value) AsString() string {
	if v.Type == ValString {
		return v.Data.(string)
	}
	return v.String()
}

// AsArray 获取数组
func (v Value) AsArray() []Value {
	if v.Type == ValArray {
		return v.Data.([]Value)
	}
	return nil
}

// AsFixedArray 获取定长数组
func (v Value) AsFixedArray() *FixedArray {
	if v.Type == ValFixedArray {
		return v.Data.(*FixedArray)
	}
	return nil
}

// AsNativeArray 获取原生数组
func (v Value) AsNativeArray() *NativeArray {
	if v.Type == ValNativeArray {
		return v.Data.(*NativeArray)
	}
	return nil
}

// IsNativeArray 检查是否为原生数组
func (v Value) IsNativeArray() bool {
	return v.Type == ValNativeArray
}

// AsMap 获取 Map
func (v Value) AsMap() map[Value]Value {
	if v.Type == ValMap {
		return v.Data.(map[Value]Value)
	}
	return nil
}

// AsSuperArray 获取万能数组
func (v Value) AsSuperArray() *SuperArray {
	if v.Type == ValSuperArray {
		return v.Data.(*SuperArray)
	}
	return nil
}

// IsSuperArray 检查是否为万能数组
func (v Value) IsSuperArray() bool {
	return v.Type == ValSuperArray
}

// AsBytes 获取字节数组
func (v Value) AsBytes() []byte {
	if v.Type == ValBytes {
		return v.Data.([]byte)
	}
	return nil
}

// IsBytesValue 检查是否为字节数组
func (v Value) IsBytesValue() bool {
	return v.Type == ValBytes
}

// AsObject 获取对象
func (v Value) AsObject() *Object {
	if v.Type == ValObject {
		return v.Data.(*Object)
	}
	return nil
}

// String 返回字符串表示
func (v Value) String() string {
	switch v.Type {
	case ValNull:
		return "null"
	case ValBool:
		if v.Data.(bool) {
			return "true"
		}
		return "false"
	case ValInt:
		return fmt.Sprintf("%d", v.Data.(int64))
	case ValFloat:
		// 与 Go 保持一致：直接显示浮点数，包括精度误差
		// Go 中 float64 变量运算也会显示精度误差，如：
		//   var d float64 = 3.14; fmt.Println(d + 1.0) // 输出 4.140000000000001
		// 字面量运算的精度问题由编译器常量折叠解决，不在这里处理
		return strconv.FormatFloat(v.Data.(float64), 'g', -1, 64)
	case ValString:
		return v.Data.(string)
	case ValArray:
		arr := v.Data.([]Value)
		var parts []string
		for _, elem := range arr {
			parts = append(parts, elem.String())
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case ValFixedArray:
		fa := v.Data.(*FixedArray)
		var parts []string
		for _, elem := range fa.Elements {
			parts = append(parts, elem.String())
		}
		return fmt.Sprintf("[%s](%d)", strings.Join(parts, ", "), fa.Capacity)
	case ValNativeArray:
		na := v.Data.(*NativeArray)
		return na.String()
	case ValMap:
		m := v.Data.(map[Value]Value)
		var parts []string
		for k, val := range m {
			parts = append(parts, k.String()+" => "+val.String())
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case ValSuperArray:
		sa := v.Data.(*SuperArray)
		var parts []string
		for _, entry := range sa.Entries {
			parts = append(parts, entry.Key.String()+" => "+entry.Value.String())
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case ValBytes:
		b := v.Data.([]byte)
		return fmt.Sprintf("<bytes len=%d>", len(b))
	case ValObject:
		obj := v.Data.(*Object)
		return fmt.Sprintf("<%s instance>", obj.Class.FullName())
	case ValFunc:
		fn := v.Data.(*Function)
		return fmt.Sprintf("<fn %s>", fn.Name)
	case ValClosure:
		closure := v.Data.(*Closure)
		return fmt.Sprintf("<closure %s>", closure.Function.Name)
	case ValEnum:
		ev := v.Data.(*EnumValue)
		return fmt.Sprintf("%s::%s", ev.EnumName, ev.CaseName)
	case ValException:
		ex := v.Data.(*Exception)
		// 如果是对象异常，获取最新的 message
		message := ex.Message
		if ex.Object != nil {
			if msgVal, ok := ex.Object.Fields["message"]; ok {
				message = msgVal.AsString()
			}
		}
		if ex.Cause != nil {
			return ex.GetFullMessage()
		}
		return fmt.Sprintf("%s: %s", ex.Type, message)
	case ValGoroutine:
		if co, ok := v.Data.(*CoroutineObject); ok {
			return fmt.Sprintf("<Coroutine#%d %s>", co.ID, co.Status)
		}
		if id, ok := v.Data.(int64); ok {
			return fmt.Sprintf("<Coroutine#%d>", id)
		}
		return "<Coroutine>"
	case ValChannel:
		return "<Channel>"
	default:
		return "<unknown>"
	}
}

// Equals 比较两个值是否相等
func (v Value) Equals(other Value) bool {
	if v.Type != other.Type {
		// 允许 int 和 float 比较
		if (v.Type == ValInt && other.Type == ValFloat) ||
			(v.Type == ValFloat && other.Type == ValInt) {
			return v.AsFloat() == other.AsFloat()
		}
		return false
	}

	switch v.Type {
	case ValNull:
		return true
	case ValBool:
		return v.Data.(bool) == other.Data.(bool)
	case ValInt:
		return v.Data.(int64) == other.Data.(int64)
	case ValFloat:
		return v.Data.(float64) == other.Data.(float64)
	case ValString:
		return v.Data.(string) == other.Data.(string)
	case ValArray:
		a1, a2 := v.Data.([]Value), other.Data.([]Value)
		if len(a1) != len(a2) {
			return false
		}
		for i := range a1 {
			if !a1[i].Equals(a2[i]) {
				return false
			}
		}
		return true
	case ValFixedArray:
		if other.Type != ValFixedArray {
			return false
		}
		fa1, fa2 := v.Data.(*FixedArray), other.Data.(*FixedArray)
		if fa1.Capacity != fa2.Capacity {
			return false
		}
		for i := range fa1.Elements {
			if !fa1.Elements[i].Equals(fa2.Elements[i]) {
				return false
			}
		}
		return true
	case ValNativeArray:
		if other.Type != ValNativeArray {
			return false
		}
		na1, na2 := v.Data.(*NativeArray), other.Data.(*NativeArray)
		return na1.Equals(na2)
	case ValBytes:
		b1, b2 := v.Data.([]byte), other.Data.([]byte)
		if len(b1) != len(b2) {
			return false
		}
		for i := range b1 {
			if b1[i] != b2[i] {
				return false
			}
		}
		return true
	case ValObject:
		return v.Data == other.Data // 引用比较
	default:
		return false
	}
}

// Hash 计算哈希值 (用于 Map key)
func (v Value) Hash() uint64 {
	switch v.Type {
	case ValNull:
		return 0
	case ValBool:
		if v.Data.(bool) {
			return 1
		}
		return 0
	case ValInt:
		return uint64(v.Data.(int64))
	case ValString:
		// FNV-1a hash
		h := uint64(14695981039346656037)
		for _, c := range v.Data.(string) {
			h ^= uint64(c)
			h *= 1099511628211
		}
		return h
	default:
		return 0
	}
}

// ============================================================================
// 运行时对象
// ============================================================================

// Object 对象实例
type Object struct {
	Class   *Class
	Fields  map[string]Value
	TypeArgs []string // 泛型类型参数（用于运行时类型验证）
}

// NewObjectInstance 创建对象实例
func NewObjectInstance(class *Class) *Object {
	return &Object{
		Class:    class,
		Fields:   make(map[string]Value),
		TypeArgs: nil,
	}
}

// NewObjectInstanceWithTypes 创建带泛型类型参数的对象实例
func NewObjectInstanceWithTypes(class *Class, typeArgs []string) *Object {
	return &Object{
		Class:    class,
		Fields:   make(map[string]Value),
		TypeArgs: typeArgs,
	}
}

// GetField 获取字段
func (o *Object) GetField(name string) (Value, bool) {
	v, ok := o.Fields[name]
	return v, ok
}

// SetField 设置字段
func (o *Object) SetField(name string, value Value) {
	o.Fields[name] = value
}

// Annotation 注解
// Args 使用 map 存储参数：
// - 位置参数使用数字字符串作为 key（如 "0", "1", "2"）
// - 命名参数使用参数名作为 key
type Annotation struct {
	Name string
	Args map[string]Value
}

// TypeParamDef 泛型类型参数定义
type TypeParamDef struct {
	Name            string   // 类型参数名 (T, K, V 等)
	Constraint      string   // extends 约束类型名
	ImplementsTypes []string // implements 约束接口列表
}

// Class 类定义
type Class struct {
	Name           string
	Namespace      string   // 命名空间（如 "sola.lang"）
	TypeParams     []*TypeParamDef // 泛型类型参数
	ParentName     string   // 父类名（用于运行时解析）
	Parent         *Class
	Implements     []string // 实现的接口名
	IsAbstract     bool     // 是否是抽象类
	IsFinal        bool     // 是否是 final 类（不能被继承）
	IsInterface    bool     // 是否是接口
	IsAttribute    bool     // 是否是注解类（有 @Attribute 标记）
	Annotations    []*Annotation         // 类注解
	Constants      map[string]Value
	StaticVars     map[string]Value
	Methods        map[string][]*Method  // 方法重载：同名不同参数数量
	Properties     map[string]Value      // 属性默认值
	PropVisibility map[string]Visibility // 属性可见性
	PropFinal      map[string]bool       // 属性是否 final（不能被重新赋值）
	PropAnnotations map[string][]*Annotation // 属性注解
	VTables        map[string]*VTable    // 接口 VTable 映射 (接口名 -> VTable)
}

// FullName 获取类的完整名称（包括命名空间）
func (c *Class) FullName() string {
	if c.Namespace != "" {
		return c.Namespace + "." + c.Name
	}
	return c.Name
}

// NewClass 创建类定义
func NewClass(name string) *Class {
	return &Class{
		Name:            name,
		Constants:       make(map[string]Value),
		StaticVars:      make(map[string]Value),
		Methods:         make(map[string][]*Method),
		Properties:      make(map[string]Value),
		PropVisibility:  make(map[string]Visibility),
		PropFinal:       make(map[string]bool),
		PropAnnotations: make(map[string][]*Annotation),
		VTables:         make(map[string]*VTable),
	}
}

// AddMethod 添加方法（支持重载）
func (c *Class) AddMethod(method *Method) {
	c.Methods[method.Name] = append(c.Methods[method.Name], method)
}

// GetMethod 获取方法（不区分参数数量，返回第一个）
func (c *Class) GetMethod(name string) *Method {
	if methods, ok := c.Methods[name]; ok && len(methods) > 0 {
		return methods[0]
	}
	if c.Parent != nil {
		return c.Parent.GetMethod(name)
	}
	return nil
}

// GetMethodByArity 根据参数数量获取方法（支持方法重载）
func (c *Class) GetMethodByArity(name string, arity int) *Method {
	if methods, ok := c.Methods[name]; ok {
		for _, m := range methods {
			if m.Arity == arity {
				return m
			}
		}
		// 如果没有精确匹配，返回第一个（可能有默认参数）
		if len(methods) > 0 {
			return methods[0]
		}
	}
	if c.Parent != nil {
		return c.Parent.GetMethodByArity(name, arity)
	}
	return nil
}

// GetAllMethods 获取指定名称的所有方法
func (c *Class) GetAllMethods(name string) []*Method {
	if methods, ok := c.Methods[name]; ok {
		return methods
	}
	if c.Parent != nil {
		return c.Parent.GetAllMethods(name)
	}
	return nil
}

// Method 方法定义
// Visibility 访问修饰符
type Visibility int

const (
	VisPublic    Visibility = 0
	VisProtected Visibility = 1
	VisPrivate   Visibility = 2
)

type Method struct {
	Name          string
	ClassName     string   // 所属类名（用于堆栈跟踪）
	SourceFile    string   // 源文件路径
	Arity         int      // 参数数量
	MinArity      int      // 最小参数数量（考虑默认参数后）
	IsStatic      bool
	IsFinal       bool       // 是否是 final 方法（不能被重写）
	Visibility    Visibility // 访问修饰符
	Annotations   []*Annotation
	Chunk         *Chunk
	LocalCount    int     // 局部变量数量
	DefaultValues []Value // 默认参数值（从第 MinArity 个参数开始）
}

// Function 函数定义
// BuiltinFn 内置函数类型
type BuiltinFn func(args []Value) Value

type Function struct {
	Name          string
	ClassName     string   // 所属类名（用于堆栈跟踪）
	SourceFile    string   // 源文件路径
	Arity         int
	MinArity      int      // 最小参数数量（考虑默认参数后）
	Chunk         *Chunk
	LocalCount    int
	UpvalueCount  int      // 捕获的外部变量数量
	IsVariadic    bool     // 是否是可变参数函数
	DefaultValues []Value  // 默认参数值（从第 MinArity 个参数开始）
	IsBuiltin     bool     // 是否是内置函数
	BuiltinFn     BuiltinFn // 内置函数实现
	Inlinable     bool     // 是否可内联（由编译器设置）
}

// NewFunction 创建函数
func NewFunction(name string) *Function {
	return &Function{
		Name:  name,
		Chunk: NewChunk(),
	}
}

// Closure 闭包
type Closure struct {
	Function *Function
	Upvalues []*Upvalue
}

// Upvalue 闭包捕获的变量
type Upvalue struct {
	Location *Value // 指向栈上的变量
	Closed   Value  // 闭包关闭后的值
	IsClosed bool
}

// Iterator 迭代器
type Iterator struct {
	Type       string // "array", "map" 或 "superarray"
	Array      []Value
	MapKeys    []Value
	Map        map[Value]Value
	SuperArray *SuperArray
	Index      int
	HasValue   bool
}

// NewIterator 创建迭代器
func NewIterator(v Value) *Iterator {
	iter := &Iterator{Index: -1}
	switch v.Type {
	case ValArray:
		iter.Type = "array"
		iter.Array = v.AsArray()
	case ValFixedArray:
		iter.Type = "array"
		iter.Array = v.AsFixedArray().Elements
	case ValNativeArray:
		iter.Type = "array"
		iter.Array = v.AsNativeArray().ToValues()
	case ValMap:
		iter.Type = "map"
		iter.Map = v.AsMap()
		iter.MapKeys = make([]Value, 0, len(iter.Map))
		for k := range iter.Map {
			iter.MapKeys = append(iter.MapKeys, k)
		}
	case ValSuperArray:
		iter.Type = "superarray"
		iter.SuperArray = v.AsSuperArray()
	}
	return iter
}

// Next 移动到下一个元素，返回是否成功
func (it *Iterator) Next() bool {
	it.Index++
	switch it.Type {
	case "array":
		it.HasValue = it.Index < len(it.Array)
	case "map":
		it.HasValue = it.Index < len(it.MapKeys)
	case "superarray":
		it.HasValue = it.Index < it.SuperArray.Len()
	}
	return it.HasValue
}

// Key 获取当前 key
func (it *Iterator) Key() Value {
	if !it.HasValue {
		return NullValue
	}
	switch it.Type {
	case "array":
		return NewInt(int64(it.Index))
	case "superarray":
		return it.SuperArray.Entries[it.Index].Key
	default:
		return it.MapKeys[it.Index]
	}
}

// Value 获取当前 value
func (it *Iterator) CurrentValue() Value {
	if !it.HasValue {
		return NullValue
	}
	switch it.Type {
	case "array":
		return it.Array[it.Index]
	case "superarray":
		return it.SuperArray.Entries[it.Index].Value
	default:
		return it.Map[it.MapKeys[it.Index]]
	}
}

// NewIteratorValue 创建迭代器值
func NewIteratorValue(iter *Iterator) Value {
	return Value{Type: ValIterator, Data: iter}
}

// AsIterator 获取迭代器
func (v Value) AsIterator() *Iterator {
	if v.Type == ValIterator {
		return v.Data.(*Iterator)
	}
	return nil
}

// Enum 枚举定义
type Enum struct {
	Name   string
	Cases  map[string]Value // 枚举成员名 -> 值
}

// NewEnum 创建枚举定义
func NewEnum(name string) *Enum {
	return &Enum{
		Name:  name,
		Cases: make(map[string]Value),
	}
}

// EnumValue 枚举值（运行时）
type EnumValue struct {
	EnumName  string // 枚举类型名
	CaseName  string // 成员名
	Value     Value  // 实际值
}

// NewEnumValue 创建枚举值
func NewEnumValue(enumName, caseName string, value Value) Value {
	return Value{
		Type: ValEnum,
		Data: &EnumValue{
			EnumName: enumName,
			CaseName: caseName,
			Value:    value,
		},
	}
}

// AsEnumValue 获取枚举值
func (v Value) AsEnumValue() *EnumValue {
	if v.Type == ValEnum {
		return v.Data.(*EnumValue)
	}
	return nil
}

// AsException 获取异常值
func (v Value) AsException() *Exception {
	if v.Type == ValException {
		return v.Data.(*Exception)
	}
	return nil
}

// IsException 检查是否是异常值
func (v Value) IsException() bool {
	return v.Type == ValException
}

// IsExceptionOfType 检查异常是否是指定类型（包括继承）
func (e *Exception) IsExceptionOfType(typeName string) bool {
	// 直接类型匹配
	if e.Type == typeName {
		return true
	}
	
	// 如果有关联对象，检查类继承链
	if e.Object != nil {
		return IsClassOrSubclass(e.Object.Class, typeName)
	}
	
	// 对于简单异常，使用硬编码的继承关系
	// Exception 继承 Throwable
	// RuntimeException 继承 Exception
	// Error 继承 Throwable
	switch e.Type {
	case "Exception":
		return typeName == "Throwable"
	case "RuntimeException":
		return typeName == "Exception" || typeName == "Throwable"
	case "Error":
		return typeName == "Throwable"
	default:
		// 其他类型（如 NativeException）默认认为继承 Exception
		if typeName == "Exception" || typeName == "Throwable" {
			return true
		}
	}
	
	return false
}

// IsClassOrSubclass 检查一个类是否是指定类名或其子类
func IsClassOrSubclass(class *Class, typeName string) bool {
	for c := class; c != nil; c = c.Parent {
		if c.Name == typeName {
			return true
		}
	}
	return false
}

