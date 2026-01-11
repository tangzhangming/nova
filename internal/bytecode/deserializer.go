package bytecode

import (
	"encoding/binary"
	"fmt"
	"math"
)

// Deserializer 字节码反序列化器
type Deserializer struct {
	data       []byte
	pos        int
	stringPool []string
}

// NewDeserializer 创建反序列化器
func NewDeserializer(data []byte) *Deserializer {
	return &Deserializer{
		data:       data,
		pos:        0,
		stringPool: make([]string, 0),
	}
}

// Deserialize 反序列化编译后的文件
func (d *Deserializer) Deserialize() (*CompiledFile, error) {
	// 读取并验证头部
	stringPoolOffset, funcOffset, classOffset, enumOffset, err := d.readHeader()
	if err != nil {
		return nil, err
	}

	// 读取字符串池
	d.pos = int(stringPoolOffset)
	if err := d.readStringPool(); err != nil {
		return nil, err
	}

	// 读取主函数
	d.pos = int(funcOffset)
	fn, err := d.readFunction()
	if err != nil {
		return nil, fmt.Errorf("failed to read main function: %w", err)
	}

	// 读取类
	d.pos = int(classOffset)
	classes, err := d.readClasses()
	if err != nil {
		return nil, fmt.Errorf("failed to read classes: %w", err)
	}

	// 读取枚举
	d.pos = int(enumOffset)
	enums, err := d.readEnums()
	if err != nil {
		return nil, fmt.Errorf("failed to read enums: %w", err)
	}

	// 字节码验证：验证主函数
	if err := VerifyFunction(fn); err != nil {
		return nil, fmt.Errorf("字节码验证失败: %w", err)
	}

	// 验证所有类的方法
	for _, class := range classes {
		for _, methods := range class.Methods {
			for _, method := range methods {
				if method.Chunk != nil {
					if err := VerifyChunk(method.Chunk); err != nil {
						return nil, fmt.Errorf("类 %s 的方法 %s 字节码验证失败: %w", class.Name, method.Name, err)
					}
				}
			}
		}
	}

	return &CompiledFile{
		MainFunction: fn,
		Classes:      classes,
		Enums:        enums,
	}, nil
}

// readHeader 读取文件头
func (d *Deserializer) readHeader() (stringPoolOffset, funcOffset, classOffset, enumOffset uint32, err error) {
	if len(d.data) < HeaderSize {
		return 0, 0, 0, 0, &FormatError{"file too small"}
	}

	// Magic
	magic := binary.BigEndian.Uint32(d.data[0:4])
	if magic != MagicNumber {
		return 0, 0, 0, 0, &FormatError{"invalid magic number, not a Sola compiled file"}
	}

	// Version
	major := d.data[4]
	minor := d.data[5]
	if major != MajorVersion {
		return 0, 0, 0, 0, &FormatError{fmt.Sprintf("incompatible version: file is v%d.%d, VM is v%d.%d", major, minor, MajorVersion, MinorVersion)}
	}

	// Flags (reserved)
	// _ = binary.BigEndian.Uint16(d.data[6:8])

	// Offsets
	stringPoolOffset = binary.BigEndian.Uint32(d.data[8:12])
	funcOffset = binary.BigEndian.Uint32(d.data[12:16])
	classOffset = binary.BigEndian.Uint32(d.data[16:20])
	enumOffset = binary.BigEndian.Uint32(d.data[20:24])

	d.pos = HeaderSize
	return stringPoolOffset, funcOffset, classOffset, enumOffset, nil
}

// readStringPool 读取字符串池
func (d *Deserializer) readStringPool() error {
	count, err := d.readU32()
	if err != nil {
		return err
	}

	d.stringPool = make([]string, count)
	for i := uint32(0); i < count; i++ {
		length, err := d.readU32()
		if err != nil {
			return err
		}
		if d.pos+int(length) > len(d.data) {
			return &FormatError{"string pool corrupted"}
		}
		d.stringPool[i] = string(d.data[d.pos : d.pos+int(length)])
		d.pos += int(length)
	}
	return nil
}

// readFunction 读取函数
func (d *Deserializer) readFunction() (*Function, error) {
	fn := &Function{}

	// 函数名
	nameIdx, err := d.readU32()
	if err != nil {
		return nil, err
	}
	fn.Name = d.getString(nameIdx)

	// 参数信息
	arity, err := d.readU16()
	if err != nil {
		return nil, err
	}
	fn.Arity = int(arity)

	minArity, err := d.readU16()
	if err != nil {
		return nil, err
	}
	fn.MinArity = int(minArity)

	localCount, err := d.readU16()
	if err != nil {
		return nil, err
	}
	fn.LocalCount = int(localCount)

	upvalueCount, err := d.readU16()
	if err != nil {
		return nil, err
	}
	fn.UpvalueCount = int(upvalueCount)

	// 标志位
	flags, err := d.readU8()
	if err != nil {
		return nil, err
	}
	fn.IsVariadic = (flags & FuncFlagVariadic) != 0
	fn.IsBuiltin = (flags & FuncFlagBuiltin) != 0

	// 字节码
	codeLen, err := d.readU32()
	if err != nil {
		return nil, err
	}
	if codeLen > 0 {
		fn.Chunk = NewChunk()
		if d.pos+int(codeLen) > len(d.data) {
			return nil, &FormatError{"bytecode corrupted"}
		}
		fn.Chunk.Code = make([]byte, codeLen)
		copy(fn.Chunk.Code, d.data[d.pos:d.pos+int(codeLen)])
		d.pos += int(codeLen)
	}

	// 行号（无论 codeLen 是否为 0 都要读取）
	lineCount, err := d.readU32()
	if err != nil {
		return nil, err
	}
	if lineCount > 0 {
		if fn.Chunk == nil {
			fn.Chunk = NewChunk()
		}
		fn.Chunk.Lines = make([]int, lineCount)
		for i := uint32(0); i < lineCount; i++ {
			line, err := d.readU32()
			if err != nil {
				return nil, err
			}
			fn.Chunk.Lines[i] = int(line)
		}
	}

	// 常量池（无论 codeLen 是否为 0 都要读取）
	constants, err := d.readConstants()
	if err != nil {
		return nil, err
	}
	if len(constants) > 0 {
		if fn.Chunk == nil {
			fn.Chunk = NewChunk()
		}
		fn.Chunk.Constants = constants
	}

	// 默认参数
	defaultCount, err := d.readU16()
	if err != nil {
		return nil, err
	}
	fn.DefaultValues = make([]Value, defaultCount)
	for i := uint16(0); i < defaultCount; i++ {
		val, err := d.readValue()
		if err != nil {
			return nil, err
		}
		fn.DefaultValues[i] = val
	}

	// 附加信息
	classNameIdx, err := d.readU32()
	if err != nil {
		return nil, err
	}
	fn.ClassName = d.getString(classNameIdx)

	sourceFileIdx, err := d.readU32()
	if err != nil {
		return nil, err
	}
	fn.SourceFile = d.getString(sourceFileIdx)

	return fn, nil
}

// readConstants 读取常量池
func (d *Deserializer) readConstants() ([]Value, error) {
	count, err := d.readU32()
	if err != nil {
		return nil, err
	}

	constants := make([]Value, count)
	for i := uint32(0); i < count; i++ {
		val, err := d.readValue()
		if err != nil {
			return nil, err
		}
		constants[i] = val
	}
	return constants, nil
}

// readValue 读取值
func (d *Deserializer) readValue() (Value, error) {
	typ, err := d.readU8()
	if err != nil {
		return NullValue, err
	}

	switch typ {
	case ConstNull:
		return NullValue, nil
	case ConstBool:
		b, err := d.readU8()
		if err != nil {
			return NullValue, err
		}
		return NewBool(b != 0), nil
	case ConstInt:
		i, err := d.readI64()
		if err != nil {
			return NullValue, err
		}
		return NewInt(i), nil
	case ConstFloat:
		f, err := d.readF64()
		if err != nil {
			return NullValue, err
		}
		return NewFloat(f), nil
	case ConstString:
		idx, err := d.readU32()
		if err != nil {
			return NullValue, err
		}
		return NewString(d.getString(idx)), nil
	default:
		return NullValue, &FormatError{fmt.Sprintf("unknown constant type: %d", typ)}
	}
}

// readClasses 读取所有类
func (d *Deserializer) readClasses() (map[string]*Class, error) {
	count, err := d.readU32()
	if err != nil {
		return nil, err
	}

	classes := make(map[string]*Class)
	for i := uint32(0); i < count; i++ {
		class, err := d.readClass()
		if err != nil {
			return nil, err
		}
		classes[class.Name] = class
	}
	return classes, nil
}

// readClass 读取类
func (d *Deserializer) readClass() (*Class, error) {
	class := NewClass("")

	// 类名
	nameIdx, err := d.readU32()
	if err != nil {
		return nil, err
	}
	class.Name = d.getString(nameIdx)

	// 命名空间
	nsIdx, err := d.readU32()
	if err != nil {
		return nil, err
	}
	class.Namespace = d.getString(nsIdx)

	// 父类名
	parentIdx, err := d.readU32()
	if err != nil {
		return nil, err
	}
	class.ParentName = d.getString(parentIdx)

	// 标志位
	flags, err := d.readU8()
	if err != nil {
		return nil, err
	}
	class.IsAbstract = (flags & ClassFlagAbstract) != 0
	class.IsInterface = (flags & ClassFlagInterface) != 0
	class.IsFinal = (flags & ClassFlagFinal) != 0
	class.IsAttribute = (flags & ClassFlagAttribute) != 0

	// 实现的接口
	implCount, err := d.readU16()
	if err != nil {
		return nil, err
	}
	class.Implements = make([]string, implCount)
	for i := uint16(0); i < implCount; i++ {
		idx, err := d.readU32()
		if err != nil {
			return nil, err
		}
		class.Implements[i] = d.getString(idx)
	}

	// 类型参数
	typeParamCount, err := d.readU16()
	if err != nil {
		return nil, err
	}
	class.TypeParams = make([]*TypeParamDef, typeParamCount)
	for i := uint16(0); i < typeParamCount; i++ {
		nameIdx, err := d.readU32()
		if err != nil {
			return nil, err
		}
		constraintIdx, err := d.readU32()
		if err != nil {
			return nil, err
		}
		// 读取 implements 类型列表
		implCount, err := d.readU16()
		if err != nil {
			return nil, err
		}
		var implementsTypes []string
		for j := uint16(0); j < implCount; j++ {
			implIdx, err := d.readU32()
			if err != nil {
				return nil, err
			}
			implementsTypes = append(implementsTypes, d.getString(implIdx))
		}
		class.TypeParams[i] = &TypeParamDef{
			Name:            d.getString(nameIdx),
			Constraint:      d.getString(constraintIdx),
			ImplementsTypes: implementsTypes,
		}
	}

	// 类注解
	annotations, err := d.readAnnotations()
	if err != nil {
		return nil, err
	}
	class.Annotations = annotations

	// 属性
	propCount, err := d.readU16()
	if err != nil {
		return nil, err
	}
	for i := uint16(0); i < propCount; i++ {
		nameIdx, err := d.readU32()
		if err != nil {
			return nil, err
		}
		propName := d.getString(nameIdx)
		val, err := d.readValue()
		if err != nil {
			return nil, err
		}
		class.Properties[propName] = val
		// 可见性
		vis, err := d.readU8()
		if err != nil {
			return nil, err
		}
		class.PropVisibility[propName] = Visibility(vis)
		// 属性注解
		propAnns, err := d.readAnnotations()
		if err != nil {
			return nil, err
		}
		class.PropAnnotations[propName] = propAnns
	}

	// 常量
	constCount, err := d.readU16()
	if err != nil {
		return nil, err
	}
	for i := uint16(0); i < constCount; i++ {
		nameIdx, err := d.readU32()
		if err != nil {
			return nil, err
		}
		val, err := d.readValue()
		if err != nil {
			return nil, err
		}
		class.Constants[d.getString(nameIdx)] = val
	}

	// 静态变量
	staticCount, err := d.readU16()
	if err != nil {
		return nil, err
	}
	for i := uint16(0); i < staticCount; i++ {
		nameIdx, err := d.readU32()
		if err != nil {
			return nil, err
		}
		val, err := d.readValue()
		if err != nil {
			return nil, err
		}
		class.StaticVars[d.getString(nameIdx)] = val
	}

	// 方法
	methodCount, err := d.readU16()
	if err != nil {
		return nil, err
	}
	for i := uint16(0); i < methodCount; i++ {
		method, err := d.readMethod()
		if err != nil {
			return nil, err
		}
		class.AddMethod(method)
	}

	return class, nil
}

// readMethod 读取方法
func (d *Deserializer) readMethod() (*Method, error) {
	method := &Method{}

	// 方法名
	nameIdx, err := d.readU32()
	if err != nil {
		return nil, err
	}
	method.Name = d.getString(nameIdx)

	// 参数信息
	arity, err := d.readU16()
	if err != nil {
		return nil, err
	}
	method.Arity = int(arity)

	minArity, err := d.readU16()
	if err != nil {
		return nil, err
	}
	method.MinArity = int(minArity)

	localCount, err := d.readU16()
	if err != nil {
		return nil, err
	}
	method.LocalCount = int(localCount)

	// 标志位
	flags, err := d.readU8()
	if err != nil {
		return nil, err
	}
	method.IsStatic = (flags & MethodFlagStatic) != 0
	if (flags & MethodFlagProtected) != 0 {
		method.Visibility = VisProtected
	} else if (flags & MethodFlagPrivate) != 0 {
		method.Visibility = VisPrivate
	} else {
		method.Visibility = VisPublic
	}

	// 注解
	annotations, err := d.readAnnotations()
	if err != nil {
		return nil, err
	}
	method.Annotations = annotations

	// 字节码
	codeLen, err := d.readU32()
	if err != nil {
		return nil, err
	}
	if codeLen > 0 {
		method.Chunk = NewChunk()
		if d.pos+int(codeLen) > len(d.data) {
			return nil, &FormatError{"method bytecode corrupted"}
		}
		method.Chunk.Code = make([]byte, codeLen)
		copy(method.Chunk.Code, d.data[d.pos:d.pos+int(codeLen)])
		d.pos += int(codeLen)
	}

	// 行号（无论 codeLen 是否为 0 都要读取）
	lineCount, err := d.readU32()
	if err != nil {
		return nil, err
	}
	if lineCount > 0 {
		if method.Chunk == nil {
			method.Chunk = NewChunk()
		}
		method.Chunk.Lines = make([]int, lineCount)
		for i := uint32(0); i < lineCount; i++ {
			line, err := d.readU32()
			if err != nil {
				return nil, err
			}
			method.Chunk.Lines[i] = int(line)
		}
	}

	// 常量池（无论 codeLen 是否为 0 都要读取）
	constants, err := d.readConstants()
	if err != nil {
		return nil, err
	}
	if len(constants) > 0 {
		if method.Chunk == nil {
			method.Chunk = NewChunk()
		}
		method.Chunk.Constants = constants
	}

	// 默认参数
	defaultCount, err := d.readU16()
	if err != nil {
		return nil, err
	}
	method.DefaultValues = make([]Value, defaultCount)
	for i := uint16(0); i < defaultCount; i++ {
		val, err := d.readValue()
		if err != nil {
			return nil, err
		}
		method.DefaultValues[i] = val
	}

	// 附加信息
	classNameIdx, err := d.readU32()
	if err != nil {
		return nil, err
	}
	method.ClassName = d.getString(classNameIdx)

	sourceFileIdx, err := d.readU32()
	if err != nil {
		return nil, err
	}
	method.SourceFile = d.getString(sourceFileIdx)

	return method, nil
}

// readAnnotations 读取注解
func (d *Deserializer) readAnnotations() ([]*Annotation, error) {
	count, err := d.readU16()
	if err != nil {
		return nil, err
	}

	annotations := make([]*Annotation, count)
	for i := uint16(0); i < count; i++ {
		nameIdx, err := d.readU32()
		if err != nil {
			return nil, err
		}
		argCount, err := d.readU16()
		if err != nil {
			return nil, err
		}
		// 读取参数（map 格式：key-value 对）
		var args map[string]Value
		if argCount > 0 {
			args = make(map[string]Value, argCount)
			for j := uint16(0); j < argCount; j++ {
				keyIdx, err := d.readU32()
				if err != nil {
					return nil, err
				}
				val, err := d.readValue()
				if err != nil {
					return nil, err
				}
				args[d.getString(keyIdx)] = val
			}
		}
		annotations[i] = &Annotation{
			Name: d.getString(nameIdx),
			Args: args,
		}
	}
	return annotations, nil
}

// readEnums 读取所有枚举
func (d *Deserializer) readEnums() (map[string]*Enum, error) {
	count, err := d.readU32()
	if err != nil {
		return nil, err
	}

	enums := make(map[string]*Enum)
	for i := uint32(0); i < count; i++ {
		enum, err := d.readEnum()
		if err != nil {
			return nil, err
		}
		enums[enum.Name] = enum
	}
	return enums, nil
}

// readEnum 读取枚举
func (d *Deserializer) readEnum() (*Enum, error) {
	// 枚举名
	nameIdx, err := d.readU32()
	if err != nil {
		return nil, err
	}
	enum := NewEnum(d.getString(nameIdx))

	// 成员数量
	caseCount, err := d.readU16()
	if err != nil {
		return nil, err
	}

	// 枚举成员
	for i := uint16(0); i < caseCount; i++ {
		caseNameIdx, err := d.readU32()
		if err != nil {
			return nil, err
		}
		val, err := d.readValue()
		if err != nil {
			return nil, err
		}
		enum.Cases[d.getString(caseNameIdx)] = val
	}

	return enum, nil
}

// ============================================================================
// 辅助读取方法
// ============================================================================

func (d *Deserializer) readU8() (uint8, error) {
	if d.pos >= len(d.data) {
		return 0, &FormatError{"unexpected end of file"}
	}
	val := d.data[d.pos]
	d.pos++
	return val, nil
}

func (d *Deserializer) readU16() (uint16, error) {
	if d.pos+2 > len(d.data) {
		return 0, &FormatError{"unexpected end of file"}
	}
	val := binary.BigEndian.Uint16(d.data[d.pos:])
	d.pos += 2
	return val, nil
}

func (d *Deserializer) readU32() (uint32, error) {
	if d.pos+4 > len(d.data) {
		return 0, &FormatError{"unexpected end of file"}
	}
	val := binary.BigEndian.Uint32(d.data[d.pos:])
	d.pos += 4
	return val, nil
}

func (d *Deserializer) readI64() (int64, error) {
	if d.pos+8 > len(d.data) {
		return 0, &FormatError{"unexpected end of file"}
	}
	val := int64(binary.BigEndian.Uint64(d.data[d.pos:]))
	d.pos += 8
	return val, nil
}

func (d *Deserializer) readF64() (float64, error) {
	if d.pos+8 > len(d.data) {
		return 0, &FormatError{"unexpected end of file"}
	}
	bits := binary.BigEndian.Uint64(d.data[d.pos:])
	d.pos += 8
	return float64FromBits(bits), nil
}

func (d *Deserializer) getString(idx uint32) string {
	if int(idx) >= len(d.stringPool) {
		return ""
	}
	return d.stringPool[idx]
}

// float64FromBits 将 uint64 转换为 float64
func float64FromBits(bits uint64) float64 {
	return math.Float64frombits(bits)
}

// ============================================================================
// 公共 API
// ============================================================================

// DeserializeFromBytes 从字节数组反序列化
func DeserializeFromBytes(data []byte) (*CompiledFile, error) {
	deserializer := NewDeserializer(data)
	return deserializer.Deserialize()
}

// ValidateHeader 只验证头部，不完整反序列化
func ValidateHeader(data []byte) error {
	if len(data) < HeaderSize {
		return &FormatError{"file too small"}
	}

	magic := binary.BigEndian.Uint32(data[0:4])
	if magic != MagicNumber {
		return &FormatError{"invalid magic number, not a Sola compiled file"}
	}

	major := data[4]
	if major != MajorVersion {
		minor := data[5]
		return &FormatError{fmt.Sprintf("incompatible version: file is v%d.%d, VM is v%d.%d", major, minor, MajorVersion, MinorVersion)}
	}

	return nil
}

