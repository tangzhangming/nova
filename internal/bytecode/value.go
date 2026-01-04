package bytecode

import (
	"fmt"
	"strings"
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
	ValMap
	ValObject
	ValFunc
	ValClosure
	ValClass
	ValMethod
	ValIterator
)

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

// NewArray 创建数组值
func NewArray(arr []Value) Value {
	return Value{Type: ValArray, Data: arr}
}

// NewMap 创建 Map 值
func NewMap(m map[Value]Value) Value {
	return Value{Type: ValMap, Data: m}
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
	case ValMap:
		return len(v.Data.(map[Value]Value)) > 0
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

// AsMap 获取 Map
func (v Value) AsMap() map[Value]Value {
	if v.Type == ValMap {
		return v.Data.(map[Value]Value)
	}
	return nil
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
		return fmt.Sprintf("%g", v.Data.(float64))
	case ValString:
		return v.Data.(string)
	case ValArray:
		arr := v.Data.([]Value)
		var parts []string
		for _, elem := range arr {
			parts = append(parts, elem.String())
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case ValMap:
		m := v.Data.(map[Value]Value)
		var parts []string
		for k, val := range m {
			parts = append(parts, k.String()+" => "+val.String())
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case ValObject:
		obj := v.Data.(*Object)
		return fmt.Sprintf("<%s instance>", obj.Class.Name)
	case ValFunc:
		fn := v.Data.(*Function)
		return fmt.Sprintf("<fn %s>", fn.Name)
	case ValClosure:
		closure := v.Data.(*Closure)
		return fmt.Sprintf("<closure %s>", closure.Function.Name)
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
	Class  *Class
	Fields map[string]Value
}

// NewObjectInstance 创建对象实例
func NewObjectInstance(class *Class) *Object {
	return &Object{
		Class:  class,
		Fields: make(map[string]Value),
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

// Class 类定义
type Class struct {
	Name       string
	ParentName string  // 父类名（用于运行时解析）
	Parent     *Class
	Constants  map[string]Value
	StaticVars map[string]Value
	Methods    map[string]*Method
	Properties map[string]Value // 属性默认值
}

// NewClass 创建类定义
func NewClass(name string) *Class {
	return &Class{
		Name:       name,
		Constants:  make(map[string]Value),
		StaticVars: make(map[string]Value),
		Methods:    make(map[string]*Method),
		Properties: make(map[string]Value),
	}
}

// GetMethod 获取方法 (包括继承的)
func (c *Class) GetMethod(name string) *Method {
	if m, ok := c.Methods[name]; ok {
		return m
	}
	if c.Parent != nil {
		return c.Parent.GetMethod(name)
	}
	return nil
}

// Method 方法定义
type Method struct {
	Name       string
	Arity      int // 参数数量
	IsStatic   bool
	Chunk      *Chunk
	LocalCount int // 局部变量数量
}

// Function 函数定义
type Function struct {
	Name          string
	Arity         int
	MinArity      int      // 最小参数数量（考虑默认参数后）
	Chunk         *Chunk
	LocalCount    int
	UpvalueCount  int      // 捕获的外部变量数量
	IsVariadic    bool     // 是否是可变参数函数
	DefaultValues []Value  // 默认参数值（从第 MinArity 个参数开始）
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
	Type     string // "array" 或 "map"
	Array    []Value
	MapKeys  []Value
	Map      map[Value]Value
	Index    int
	HasValue bool
}

// NewIterator 创建迭代器
func NewIterator(v Value) *Iterator {
	iter := &Iterator{Index: -1}
	switch v.Type {
	case ValArray:
		iter.Type = "array"
		iter.Array = v.AsArray()
	case ValMap:
		iter.Type = "map"
		iter.Map = v.AsMap()
		iter.MapKeys = make([]Value, 0, len(iter.Map))
		for k := range iter.Map {
			iter.MapKeys = append(iter.MapKeys, k)
		}
	}
	return iter
}

// Next 移动到下一个元素，返回是否成功
func (it *Iterator) Next() bool {
	it.Index++
	if it.Type == "array" {
		it.HasValue = it.Index < len(it.Array)
	} else {
		it.HasValue = it.Index < len(it.MapKeys)
	}
	return it.HasValue
}

// Key 获取当前 key
func (it *Iterator) Key() Value {
	if !it.HasValue {
		return NullValue
	}
	if it.Type == "array" {
		return NewInt(int64(it.Index))
	}
	return it.MapKeys[it.Index]
}

// Value 获取当前 value
func (it *Iterator) CurrentValue() Value {
	if !it.HasValue {
		return NullValue
	}
	if it.Type == "array" {
		return it.Array[it.Index]
	}
	return it.Map[it.MapKeys[it.Index]]
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

