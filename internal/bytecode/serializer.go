package bytecode

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// Serializer 字节码序列化器
type Serializer struct {
	buf         *bytes.Buffer
	stringPool  []string
	stringIndex map[string]uint32
}

// NewSerializer 创建序列化器
func NewSerializer() *Serializer {
	return &Serializer{
		buf:         new(bytes.Buffer),
		stringPool:  make([]string, 0),
		stringIndex: make(map[string]uint32),
	}
}

// Serialize 序列化函数、类和枚举
func (s *Serializer) Serialize(fn *Function, classes map[string]*Class, enums map[string]*Enum) ([]byte, error) {
	// 第一遍：收集所有字符串到字符串池
	s.collectStrings(fn, classes, enums)

	// 创建临时缓冲区存储各个部分
	stringPoolBuf := s.serializeStringPool()
	funcBuf := s.serializeFunction(fn)
	classesBuf := s.serializeClasses(classes)
	enumsBuf := s.serializeEnums(enums)

	// 计算各部分偏移量
	headerSize := uint32(HeaderSize)
	stringPoolOffset := headerSize
	funcOffset := stringPoolOffset + uint32(len(stringPoolBuf))
	classOffset := funcOffset + uint32(len(funcBuf))
	enumOffset := classOffset + uint32(len(classesBuf))

	// 写入头部
	s.writeHeader(stringPoolOffset, funcOffset, classOffset, enumOffset)

	// 写入各部分
	s.buf.Write(stringPoolBuf)
	s.buf.Write(funcBuf)
	s.buf.Write(classesBuf)
	s.buf.Write(enumsBuf)

	return s.buf.Bytes(), nil
}

// writeHeader 写入文件头
func (s *Serializer) writeHeader(stringPoolOffset, funcOffset, classOffset, enumOffset uint32) {
	// Magic (4 bytes)
	binary.Write(s.buf, binary.BigEndian, MagicNumber)
	// Version (2 bytes)
	s.buf.WriteByte(MajorVersion)
	s.buf.WriteByte(MinorVersion)
	// Flags (2 bytes, reserved)
	binary.Write(s.buf, binary.BigEndian, uint16(0))
	// StringPoolOffset (4 bytes)
	binary.Write(s.buf, binary.BigEndian, stringPoolOffset)
	// FuncOffset (4 bytes)
	binary.Write(s.buf, binary.BigEndian, funcOffset)
	// ClassOffset (4 bytes)
	binary.Write(s.buf, binary.BigEndian, classOffset)
	// EnumOffset (4 bytes)
	binary.Write(s.buf, binary.BigEndian, enumOffset)
}

// addString 添加字符串到池，返回索引
func (s *Serializer) addString(str string) uint32 {
	if idx, ok := s.stringIndex[str]; ok {
		return idx
	}
	idx := uint32(len(s.stringPool))
	s.stringPool = append(s.stringPool, str)
	s.stringIndex[str] = idx
	return idx
}

// collectStrings 收集所有字符串
func (s *Serializer) collectStrings(fn *Function, classes map[string]*Class, enums map[string]*Enum) {
	// 收集主函数字符串
	s.collectFunctionStrings(fn)

	// 收集类字符串
	for name, class := range classes {
		s.addString(name)
		s.addString(class.Namespace)
		s.addString(class.ParentName)
		for _, iface := range class.Implements {
			s.addString(iface)
		}
		for propName := range class.Properties {
			s.addString(propName)
		}
		for constName := range class.Constants {
			s.addString(constName)
		}
		for staticName := range class.StaticVars {
			s.addString(staticName)
		}
		for _, methods := range class.Methods {
			for _, method := range methods {
				s.collectMethodStrings(method)
			}
		}
		// 收集类型参数
		for _, tp := range class.TypeParams {
			s.addString(tp.Name)
			s.addString(tp.Constraint)
			// 收集 implements 类型
			for _, implType := range tp.ImplementsTypes {
				s.addString(implType)
			}
		}
		// 收集注解
		s.collectAnnotations(class.Annotations)
		for _, anns := range class.PropAnnotations {
			s.collectAnnotations(anns)
		}
	}

	// 收集枚举字符串
	for name, enum := range enums {
		s.addString(name)
		for caseName := range enum.Cases {
			s.addString(caseName)
		}
	}
}

// collectFunctionStrings 收集函数中的字符串
func (s *Serializer) collectFunctionStrings(fn *Function) {
	if fn == nil {
		return
	}
	s.addString(fn.Name)
	s.addString(fn.ClassName)
	s.addString(fn.SourceFile)
	s.collectChunkStrings(fn.Chunk)
}

// collectMethodStrings 收集方法中的字符串
func (s *Serializer) collectMethodStrings(method *Method) {
	if method == nil {
		return
	}
	s.addString(method.Name)
	s.addString(method.ClassName)
	s.addString(method.SourceFile)
	s.collectChunkStrings(method.Chunk)
	s.collectAnnotations(method.Annotations)
}

// collectChunkStrings 收集 Chunk 中的字符串
func (s *Serializer) collectChunkStrings(chunk *Chunk) {
	if chunk == nil {
		return
	}
	for _, val := range chunk.Constants {
		if val.Type == ValString {
			s.addString(val.Data.(string))
		}
	}
}

// collectAnnotations 收集注解中的字符串
func (s *Serializer) collectAnnotations(annotations []*Annotation) {
	for _, ann := range annotations {
		s.addString(ann.Name)
		for _, arg := range ann.Args {
			if arg.Type == ValString {
				s.addString(arg.Data.(string))
			}
		}
	}
}

// serializeStringPool 序列化字符串池
func (s *Serializer) serializeStringPool() []byte {
	buf := new(bytes.Buffer)
	// 字符串数量
	binary.Write(buf, binary.BigEndian, uint32(len(s.stringPool)))
	// 每个字符串：长度 + 数据
	for _, str := range s.stringPool {
		data := []byte(str)
		binary.Write(buf, binary.BigEndian, uint32(len(data)))
		buf.Write(data)
	}
	return buf.Bytes()
}

// serializeFunction 序列化函数
func (s *Serializer) serializeFunction(fn *Function) []byte {
	buf := new(bytes.Buffer)
	s.writeFunctionTo(buf, fn)
	return buf.Bytes()
}

// writeFunctionTo 写入函数到缓冲区
func (s *Serializer) writeFunctionTo(buf *bytes.Buffer, fn *Function) {
	if fn == nil {
		// 写入空函数标记
		binary.Write(buf, binary.BigEndian, uint32(0)) // name index = 0 (空字符串)
		binary.Write(buf, binary.BigEndian, uint16(0)) // arity
		binary.Write(buf, binary.BigEndian, uint16(0)) // minArity
		binary.Write(buf, binary.BigEndian, uint16(0)) // localCount
		binary.Write(buf, binary.BigEndian, uint16(0)) // upvalueCount
		buf.WriteByte(0)                               // flags
		binary.Write(buf, binary.BigEndian, uint32(0)) // codeLen
		binary.Write(buf, binary.BigEndian, uint32(0)) // lineCount
		binary.Write(buf, binary.BigEndian, uint32(0)) // constCount
		binary.Write(buf, binary.BigEndian, uint16(0)) // defaultCount
		binary.Write(buf, binary.BigEndian, uint32(0)) // className index
		binary.Write(buf, binary.BigEndian, uint32(0)) // sourceFile index
		return
	}

	// 函数名
	binary.Write(buf, binary.BigEndian, s.addString(fn.Name))
	// 参数信息
	binary.Write(buf, binary.BigEndian, uint16(fn.Arity))
	binary.Write(buf, binary.BigEndian, uint16(fn.MinArity))
	binary.Write(buf, binary.BigEndian, uint16(fn.LocalCount))
	binary.Write(buf, binary.BigEndian, uint16(fn.UpvalueCount))
	// 标志位
	var flags uint8
	if fn.IsVariadic {
		flags |= FuncFlagVariadic
	}
	if fn.IsBuiltin {
		flags |= FuncFlagBuiltin
	}
	buf.WriteByte(flags)

	// 字节码
	if fn.Chunk != nil {
		binary.Write(buf, binary.BigEndian, uint32(len(fn.Chunk.Code)))
		buf.Write(fn.Chunk.Code)
		// 行号
		binary.Write(buf, binary.BigEndian, uint32(len(fn.Chunk.Lines)))
		for _, line := range fn.Chunk.Lines {
			binary.Write(buf, binary.BigEndian, uint32(line))
		}
		// 常量池
		s.writeConstants(buf, fn.Chunk.Constants)
	} else {
		binary.Write(buf, binary.BigEndian, uint32(0)) // codeLen
		binary.Write(buf, binary.BigEndian, uint32(0)) // lineCount
		binary.Write(buf, binary.BigEndian, uint32(0)) // constCount
	}

	// 默认参数
	binary.Write(buf, binary.BigEndian, uint16(len(fn.DefaultValues)))
	for _, def := range fn.DefaultValues {
		s.writeValue(buf, def)
	}

	// 附加信息
	binary.Write(buf, binary.BigEndian, s.addString(fn.ClassName))
	binary.Write(buf, binary.BigEndian, s.addString(fn.SourceFile))
}

// writeConstants 写入常量池
func (s *Serializer) writeConstants(buf *bytes.Buffer, constants []Value) {
	binary.Write(buf, binary.BigEndian, uint32(len(constants)))
	for _, val := range constants {
		s.writeValue(buf, val)
	}
}

// writeValue 写入值
func (s *Serializer) writeValue(buf *bytes.Buffer, val Value) {
	switch val.Type {
	case ValNull:
		buf.WriteByte(ConstNull)
	case ValBool:
		buf.WriteByte(ConstBool)
		if val.Data.(bool) {
			buf.WriteByte(1)
		} else {
			buf.WriteByte(0)
		}
	case ValInt:
		buf.WriteByte(ConstInt)
		binary.Write(buf, binary.BigEndian, val.Data.(int64))
	case ValFloat:
		buf.WriteByte(ConstFloat)
		binary.Write(buf, binary.BigEndian, val.Data.(float64))
	case ValString:
		buf.WriteByte(ConstString)
		binary.Write(buf, binary.BigEndian, s.addString(val.Data.(string)))
	default:
		// 不支持的类型，写入 null
		buf.WriteByte(ConstNull)
	}
}

// serializeClasses 序列化所有类
func (s *Serializer) serializeClasses(classes map[string]*Class) []byte {
	buf := new(bytes.Buffer)
	// 类数量
	binary.Write(buf, binary.BigEndian, uint32(len(classes)))
	for _, class := range classes {
		s.writeClassTo(buf, class)
	}
	return buf.Bytes()
}

// writeClassTo 写入类
func (s *Serializer) writeClassTo(buf *bytes.Buffer, class *Class) {
	// 类名
	binary.Write(buf, binary.BigEndian, s.addString(class.Name))
	// 命名空间
	binary.Write(buf, binary.BigEndian, s.addString(class.Namespace))
	// 父类名
	binary.Write(buf, binary.BigEndian, s.addString(class.ParentName))
	// 标志位
	var flags uint8
	if class.IsAbstract {
		flags |= ClassFlagAbstract
	}
	if class.IsInterface {
		flags |= ClassFlagInterface
	}
	buf.WriteByte(flags)

	// 实现的接口
	binary.Write(buf, binary.BigEndian, uint16(len(class.Implements)))
	for _, iface := range class.Implements {
		binary.Write(buf, binary.BigEndian, s.addString(iface))
	}

	// 类型参数
	binary.Write(buf, binary.BigEndian, uint16(len(class.TypeParams)))
	for _, tp := range class.TypeParams {
		binary.Write(buf, binary.BigEndian, s.addString(tp.Name))
		binary.Write(buf, binary.BigEndian, s.addString(tp.Constraint))
		// 写入 implements 类型列表
		binary.Write(buf, binary.BigEndian, uint16(len(tp.ImplementsTypes)))
		for _, implType := range tp.ImplementsTypes {
			binary.Write(buf, binary.BigEndian, s.addString(implType))
		}
	}

	// 类注解
	s.writeAnnotations(buf, class.Annotations)

	// 属性
	binary.Write(buf, binary.BigEndian, uint16(len(class.Properties)))
	for name, val := range class.Properties {
		binary.Write(buf, binary.BigEndian, s.addString(name))
		s.writeValue(buf, val)
		// 可见性
		vis := class.PropVisibility[name]
		buf.WriteByte(uint8(vis))
		// 属性注解
		s.writeAnnotations(buf, class.PropAnnotations[name])
	}

	// 常量
	binary.Write(buf, binary.BigEndian, uint16(len(class.Constants)))
	for name, val := range class.Constants {
		binary.Write(buf, binary.BigEndian, s.addString(name))
		s.writeValue(buf, val)
	}

	// 静态变量
	binary.Write(buf, binary.BigEndian, uint16(len(class.StaticVars)))
	for name, val := range class.StaticVars {
		binary.Write(buf, binary.BigEndian, s.addString(name))
		s.writeValue(buf, val)
	}

	// 方法
	// 先统计总方法数
	totalMethods := 0
	for _, methods := range class.Methods {
		totalMethods += len(methods)
	}
	binary.Write(buf, binary.BigEndian, uint16(totalMethods))
	for _, methods := range class.Methods {
		for _, method := range methods {
			s.writeMethodTo(buf, method)
		}
	}
}

// writeMethodTo 写入方法
func (s *Serializer) writeMethodTo(buf *bytes.Buffer, method *Method) {
	// 方法名
	binary.Write(buf, binary.BigEndian, s.addString(method.Name))
	// 参数信息
	binary.Write(buf, binary.BigEndian, uint16(method.Arity))
	binary.Write(buf, binary.BigEndian, uint16(method.MinArity))
	binary.Write(buf, binary.BigEndian, uint16(method.LocalCount))
	// 标志位
	var flags uint8
	if method.IsStatic {
		flags |= MethodFlagStatic
	}
	switch method.Visibility {
	case VisPublic:
		flags |= MethodFlagPublic
	case VisProtected:
		flags |= MethodFlagProtected
	case VisPrivate:
		flags |= MethodFlagPrivate
	}
	buf.WriteByte(flags)

	// 注解
	s.writeAnnotations(buf, method.Annotations)

	// 字节码
	if method.Chunk != nil {
		binary.Write(buf, binary.BigEndian, uint32(len(method.Chunk.Code)))
		buf.Write(method.Chunk.Code)
		// 行号
		binary.Write(buf, binary.BigEndian, uint32(len(method.Chunk.Lines)))
		for _, line := range method.Chunk.Lines {
			binary.Write(buf, binary.BigEndian, uint32(line))
		}
		// 常量池
		s.writeConstants(buf, method.Chunk.Constants)
	} else {
		binary.Write(buf, binary.BigEndian, uint32(0)) // codeLen
		binary.Write(buf, binary.BigEndian, uint32(0)) // lineCount
		binary.Write(buf, binary.BigEndian, uint32(0)) // constCount
	}

	// 默认参数
	binary.Write(buf, binary.BigEndian, uint16(len(method.DefaultValues)))
	for _, def := range method.DefaultValues {
		s.writeValue(buf, def)
	}

	// 附加信息
	binary.Write(buf, binary.BigEndian, s.addString(method.ClassName))
	binary.Write(buf, binary.BigEndian, s.addString(method.SourceFile))
}

// writeAnnotations 写入注解
func (s *Serializer) writeAnnotations(buf *bytes.Buffer, annotations []*Annotation) {
	binary.Write(buf, binary.BigEndian, uint16(len(annotations)))
	for _, ann := range annotations {
		binary.Write(buf, binary.BigEndian, s.addString(ann.Name))
		binary.Write(buf, binary.BigEndian, uint16(len(ann.Args)))
		for _, arg := range ann.Args {
			s.writeValue(buf, arg)
		}
	}
}

// serializeEnums 序列化所有枚举
func (s *Serializer) serializeEnums(enums map[string]*Enum) []byte {
	buf := new(bytes.Buffer)
	// 枚举数量
	binary.Write(buf, binary.BigEndian, uint32(len(enums)))
	for _, enum := range enums {
		s.writeEnumTo(buf, enum)
	}
	return buf.Bytes()
}

// writeEnumTo 写入枚举
func (s *Serializer) writeEnumTo(buf *bytes.Buffer, enum *Enum) {
	// 枚举名
	binary.Write(buf, binary.BigEndian, s.addString(enum.Name))
	// 成员数量
	binary.Write(buf, binary.BigEndian, uint16(len(enum.Cases)))
	// 枚举成员
	for caseName, val := range enum.Cases {
		binary.Write(buf, binary.BigEndian, s.addString(caseName))
		s.writeValue(buf, val)
	}
}

// ============================================================================
// 辅助类型
// ============================================================================

// CompiledFile 编译后的文件
type CompiledFile struct {
	MainFunction *Function
	Classes      map[string]*Class
	Enums        map[string]*Enum
	SourceFile   string // 源文件名（用于错误提示）
}

// SerializeToBytes 将编译后的文件序列化为字节数组
func SerializeToBytes(cf *CompiledFile) ([]byte, error) {
	serializer := NewSerializer()
	return serializer.Serialize(cf.MainFunction, cf.Classes, cf.Enums)
}

// GetVersion 获取当前版本号
func GetVersion() (uint8, uint8) {
	return MajorVersion, MinorVersion
}

// FormatError 格式错误
type FormatError struct {
	Message string
}

func (e *FormatError) Error() string {
	return fmt.Sprintf("bytecode format error: %s", e.Message)
}






