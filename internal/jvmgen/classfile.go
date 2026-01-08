// Package jvmgen 实现 Sola 到 JVM 字节码的编译
package jvmgen

import (
	"bytes"
	"encoding/binary"
	"io"
)

// Class 文件常量
const (
	ClassFileMagic   = 0xCAFEBABE
	ClassMajorVersion = 52 // Java 8
	ClassMinorVersion = 0
)

// 常量池标签
const (
	ConstantUtf8               = 1
	ConstantInteger            = 3
	ConstantFloat              = 4
	ConstantLong               = 5
	ConstantDouble             = 6
	ConstantClass              = 7
	ConstantString             = 8
	ConstantFieldref           = 9
	ConstantMethodref          = 10
	ConstantInterfaceMethodref = 11
	ConstantNameAndType        = 12
	ConstantMethodHandle       = 15
	ConstantMethodType         = 16
	ConstantInvokeDynamic      = 18
)

// 访问标志
const (
	AccPublic     = 0x0001
	AccPrivate    = 0x0002
	AccProtected  = 0x0004
	AccStatic     = 0x0008
	AccFinal      = 0x0010
	AccSuper      = 0x0020
	AccVolatile   = 0x0040
	AccTransient  = 0x0080
	AccInterface  = 0x0200
	AccAbstract   = 0x0400
	AccSynthetic  = 0x1000
	AccAnnotation = 0x2000
	AccEnum       = 0x4000
)

// ClassFile JVM class 文件结构
type ClassFile struct {
	Magic             uint32
	MinorVersion      uint16
	MajorVersion      uint16
	ConstantPoolCount uint16
	ConstantPool      []ConstantPoolEntry
	AccessFlags       uint16
	ThisClass         uint16
	SuperClass        uint16
	InterfacesCount   uint16
	Interfaces        []uint16
	FieldsCount       uint16
	Fields            []FieldInfo
	MethodsCount      uint16
	Methods           []MethodInfo
	AttributesCount   uint16
	Attributes        []AttributeInfo
}

// ConstantPoolEntry 常量池条目
type ConstantPoolEntry interface {
	Tag() uint8
	Write(w io.Writer) error
}

// ConstantUtf8Info UTF8 字符串常量
type ConstantUtf8Info struct {
	Value string
}

func (c *ConstantUtf8Info) Tag() uint8 { return ConstantUtf8 }
func (c *ConstantUtf8Info) Write(w io.Writer) error {
	binary.Write(w, binary.BigEndian, c.Tag())
	binary.Write(w, binary.BigEndian, uint16(len(c.Value)))
	_, err := w.Write([]byte(c.Value))
	return err
}

// ConstantClassInfo 类引用常量
type ConstantClassInfo struct {
	NameIndex uint16
}

func (c *ConstantClassInfo) Tag() uint8 { return ConstantClass }
func (c *ConstantClassInfo) Write(w io.Writer) error {
	binary.Write(w, binary.BigEndian, c.Tag())
	return binary.Write(w, binary.BigEndian, c.NameIndex)
}

// ConstantStringInfo 字符串常量
type ConstantStringInfo struct {
	StringIndex uint16
}

func (c *ConstantStringInfo) Tag() uint8 { return ConstantString }
func (c *ConstantStringInfo) Write(w io.Writer) error {
	binary.Write(w, binary.BigEndian, c.Tag())
	return binary.Write(w, binary.BigEndian, c.StringIndex)
}

// ConstantFieldrefInfo 字段引用常量
type ConstantFieldrefInfo struct {
	ClassIndex       uint16
	NameAndTypeIndex uint16
}

func (c *ConstantFieldrefInfo) Tag() uint8 { return ConstantFieldref }
func (c *ConstantFieldrefInfo) Write(w io.Writer) error {
	binary.Write(w, binary.BigEndian, c.Tag())
	binary.Write(w, binary.BigEndian, c.ClassIndex)
	return binary.Write(w, binary.BigEndian, c.NameAndTypeIndex)
}

// ConstantMethodrefInfo 方法引用常量
type ConstantMethodrefInfo struct {
	ClassIndex       uint16
	NameAndTypeIndex uint16
}

func (c *ConstantMethodrefInfo) Tag() uint8 { return ConstantMethodref }
func (c *ConstantMethodrefInfo) Write(w io.Writer) error {
	binary.Write(w, binary.BigEndian, c.Tag())
	binary.Write(w, binary.BigEndian, c.ClassIndex)
	return binary.Write(w, binary.BigEndian, c.NameAndTypeIndex)
}

// ConstantNameAndTypeInfo 名称和类型描述符常量
type ConstantNameAndTypeInfo struct {
	NameIndex       uint16
	DescriptorIndex uint16
}

func (c *ConstantNameAndTypeInfo) Tag() uint8 { return ConstantNameAndType }
func (c *ConstantNameAndTypeInfo) Write(w io.Writer) error {
	binary.Write(w, binary.BigEndian, c.Tag())
	binary.Write(w, binary.BigEndian, c.NameIndex)
	return binary.Write(w, binary.BigEndian, c.DescriptorIndex)
}

// FieldInfo 字段信息
type FieldInfo struct {
	AccessFlags     uint16
	NameIndex       uint16
	DescriptorIndex uint16
	AttributesCount uint16
	Attributes      []AttributeInfo
}

// MethodInfo 方法信息
type MethodInfo struct {
	AccessFlags     uint16
	NameIndex       uint16
	DescriptorIndex uint16
	AttributesCount uint16
	Attributes      []AttributeInfo
}

// AttributeInfo 属性信息
type AttributeInfo struct {
	NameIndex uint16
	Info      []byte
}

// NewClassFile 创建新的 class 文件
func NewClassFile(className string) *ClassFile {
	cf := &ClassFile{
		Magic:        ClassFileMagic,
		MinorVersion: ClassMinorVersion,
		MajorVersion: ClassMajorVersion,
		AccessFlags:  AccPublic | AccSuper,
	}
	return cf
}

// Write 将 class 文件写入 io.Writer
func (cf *ClassFile) Write(w io.Writer) error {
	// Magic number
	if err := binary.Write(w, binary.BigEndian, cf.Magic); err != nil {
		return err
	}

	// Version
	if err := binary.Write(w, binary.BigEndian, cf.MinorVersion); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, cf.MajorVersion); err != nil {
		return err
	}

	// Constant pool
	if err := binary.Write(w, binary.BigEndian, uint16(len(cf.ConstantPool)+1)); err != nil {
		return err
	}
	for _, cp := range cf.ConstantPool {
		if err := cp.Write(w); err != nil {
			return err
		}
	}

	// Access flags, this class, super class
	if err := binary.Write(w, binary.BigEndian, cf.AccessFlags); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, cf.ThisClass); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, cf.SuperClass); err != nil {
		return err
	}

	// Interfaces
	if err := binary.Write(w, binary.BigEndian, uint16(len(cf.Interfaces))); err != nil {
		return err
	}
	for _, iface := range cf.Interfaces {
		if err := binary.Write(w, binary.BigEndian, iface); err != nil {
			return err
		}
	}

	// Fields
	if err := binary.Write(w, binary.BigEndian, uint16(len(cf.Fields))); err != nil {
		return err
	}
	for _, field := range cf.Fields {
		if err := writeFieldInfo(w, &field); err != nil {
			return err
		}
	}

	// Methods
	if err := binary.Write(w, binary.BigEndian, uint16(len(cf.Methods))); err != nil {
		return err
	}
	for _, method := range cf.Methods {
		if err := writeMethodInfo(w, &method); err != nil {
			return err
		}
	}

	// Attributes
	if err := binary.Write(w, binary.BigEndian, uint16(len(cf.Attributes))); err != nil {
		return err
	}
	for _, attr := range cf.Attributes {
		if err := writeAttributeInfo(w, &attr); err != nil {
			return err
		}
	}

	return nil
}

// ToBytes 将 class 文件转换为字节数组
func (cf *ClassFile) ToBytes() ([]byte, error) {
	var buf bytes.Buffer
	if err := cf.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeFieldInfo(w io.Writer, f *FieldInfo) error {
	binary.Write(w, binary.BigEndian, f.AccessFlags)
	binary.Write(w, binary.BigEndian, f.NameIndex)
	binary.Write(w, binary.BigEndian, f.DescriptorIndex)
	binary.Write(w, binary.BigEndian, uint16(len(f.Attributes)))
	for _, attr := range f.Attributes {
		if err := writeAttributeInfo(w, &attr); err != nil {
			return err
		}
	}
	return nil
}

func writeMethodInfo(w io.Writer, m *MethodInfo) error {
	binary.Write(w, binary.BigEndian, m.AccessFlags)
	binary.Write(w, binary.BigEndian, m.NameIndex)
	binary.Write(w, binary.BigEndian, m.DescriptorIndex)
	binary.Write(w, binary.BigEndian, uint16(len(m.Attributes)))
	for _, attr := range m.Attributes {
		if err := writeAttributeInfo(w, &attr); err != nil {
			return err
		}
	}
	return nil
}

func writeAttributeInfo(w io.Writer, a *AttributeInfo) error {
	binary.Write(w, binary.BigEndian, a.NameIndex)
	binary.Write(w, binary.BigEndian, uint32(len(a.Info)))
	_, err := w.Write(a.Info)
	return err
}
