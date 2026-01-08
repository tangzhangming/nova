package jvmgen

import (
	"fmt"

	"github.com/tangzhangming/nova/internal/ast"
)

// Generator JVM 字节码生成器
type Generator struct {
	className    string
	classFile    *ClassFile
	constantPool []ConstantPoolEntry
	cpIndex      map[string]uint16 // 常量池索引缓存
	code         *ByteWriter       // 当前方法的字节码
	maxStack     uint16
	maxLocals    uint16
}

// NewGenerator 创建新的代码生成器
func NewGenerator(className string) *Generator {
	return &Generator{
		className: className,
		cpIndex:   make(map[string]uint16),
		code:      NewByteWriter(),
	}
}

// Generate 从 AST 生成 JVM class 文件
func (g *Generator) Generate(file *ast.File) ([]byte, error) {
	g.classFile = NewClassFile(g.className)

	// 构建常量池
	g.buildConstantPool()

	// 设置类和父类引用
	g.classFile.ThisClass = g.cpIndex["class:"+g.className]
	g.classFile.SuperClass = g.cpIndex["class:java/lang/Object"]

	// 生成 main 方法
	mainMethod, err := g.generateMainMethod(file)
	if err != nil {
		return nil, err
	}
	g.classFile.Methods = append(g.classFile.Methods, *mainMethod)

	// 生成默认构造函数
	initMethod := g.generateInitMethod()
	g.classFile.Methods = append(g.classFile.Methods, *initMethod)

	// 设置常量池
	g.classFile.ConstantPool = g.constantPool

	return g.classFile.ToBytes()
}

// buildConstantPool 构建基础常量池
func (g *Generator) buildConstantPool() {
	// 类名相关
	g.addUtf8(g.className)
	g.addUtf8("java/lang/Object")
	g.addUtf8("java/lang/System")
	g.addUtf8("java/io/PrintStream")

	g.addClass(g.className)
	g.addClass("java/lang/Object")
	g.addClass("java/lang/System")
	g.addClass("java/io/PrintStream")

	// main 方法相关
	g.addUtf8("main")
	g.addUtf8("([Ljava/lang/String;)V")
	g.addUtf8("Code")

	// 构造函数相关
	g.addUtf8("<init>")
	g.addUtf8("()V")

	// System.out 字段
	g.addUtf8("out")
	g.addUtf8("Ljava/io/PrintStream;")
	g.addNameAndType("out", "Ljava/io/PrintStream;")
	g.addFieldref("java/lang/System", "out", "Ljava/io/PrintStream;")

	// println 方法
	g.addUtf8("println")
	g.addUtf8("(Ljava/lang/String;)V")
	g.addNameAndType("println", "(Ljava/lang/String;)V")
	g.addMethodref("java/io/PrintStream", "println", "(Ljava/lang/String;)V")

	// Object 构造函数
	g.addNameAndType("<init>", "()V")
	g.addMethodref("java/lang/Object", "<init>", "()V")
}

// generateMainMethod 生成 main 方法
func (g *Generator) generateMainMethod(file *ast.File) (*MethodInfo, error) {
	g.code.Reset()
	g.maxStack = 2
	g.maxLocals = 1

	// 遍历所有语句
	for _, stmt := range file.Statements {
		if err := g.generateStatement(stmt); err != nil {
			return nil, err
		}
	}

	// return
	g.code.WriteByte(OpReturn)

	// 构建 Code 属性
	codeAttr := g.buildCodeAttribute()

	return &MethodInfo{
		AccessFlags:     AccPublic | AccStatic,
		NameIndex:       g.cpIndex["utf8:main"],
		DescriptorIndex: g.cpIndex["utf8:([Ljava/lang/String;)V"],
		Attributes:      []AttributeInfo{codeAttr},
	}, nil
}

// generateInitMethod 生成默认构造函数
func (g *Generator) generateInitMethod() *MethodInfo {
	initCode := NewByteWriter()

	// aload_0
	initCode.WriteByte(OpAload0)
	// invokespecial java/lang/Object.<init>:()V
	initCode.WriteByte(OpInvokespecial)
	initCode.WriteU16(g.cpIndex["methodref:java/lang/Object.<init>:()V"])
	// return
	initCode.WriteByte(OpReturn)

	// 构建 Code 属性
	codeBytes := initCode.Bytes()
	codeAttrData := NewByteWriter()
	codeAttrData.WriteU16(1) // max_stack
	codeAttrData.WriteU16(1) // max_locals
	codeAttrData.WriteU32(uint32(len(codeBytes)))
	codeAttrData.WriteBytes(codeBytes)
	codeAttrData.WriteU16(0) // exception_table_length
	codeAttrData.WriteU16(0) // attributes_count

	return &MethodInfo{
		AccessFlags:     AccPublic,
		NameIndex:       g.cpIndex["utf8:<init>"],
		DescriptorIndex: g.cpIndex["utf8:()V"],
		Attributes: []AttributeInfo{
			{
				NameIndex: g.cpIndex["utf8:Code"],
				Info:      codeAttrData.Bytes(),
			},
		},
	}
}

// generateStatement 生成语句的字节码
func (g *Generator) generateStatement(stmt ast.Statement) error {
	switch s := stmt.(type) {
	case *ast.EchoStmt:
		return g.generateEcho(s)
	default:
		// 其他语句暂时忽略
		return nil
	}
}

// generateEcho 生成 echo 语句的字节码
func (g *Generator) generateEcho(stmt *ast.EchoStmt) error {
	// getstatic java/lang/System.out:Ljava/io/PrintStream;
	g.code.WriteByte(OpGetstatic)
	g.code.WriteU16(g.cpIndex["fieldref:java/lang/System.out:Ljava/io/PrintStream;"])

	// 获取要打印的字符串
	strValue, err := g.getStringValue(stmt.Value)
	if err != nil {
		return err
	}

	// 添加字符串常量
	strIndex := g.addString(strValue)

	// ldc <string>
	g.code.WriteByte(OpLdc)
	g.code.WriteByte(byte(strIndex))

	// invokevirtual java/io/PrintStream.println:(Ljava/lang/String;)V
	g.code.WriteByte(OpInvokevirtual)
	g.code.WriteU16(g.cpIndex["methodref:java/io/PrintStream.println:(Ljava/lang/String;)V"])

	return nil
}

// getStringValue 从表达式获取字符串值
func (g *Generator) getStringValue(expr ast.Expression) (string, error) {
	switch e := expr.(type) {
	case *ast.StringLiteral:
		return e.Value, nil
	default:
		return "", fmt.Errorf("unsupported expression type for echo: %T", expr)
	}
}

// buildCodeAttribute 构建 Code 属性
func (g *Generator) buildCodeAttribute() AttributeInfo {
	codeBytes := g.code.Bytes()

	attrData := NewByteWriter()
	attrData.WriteU16(g.maxStack)  // max_stack
	attrData.WriteU16(g.maxLocals) // max_locals
	attrData.WriteU32(uint32(len(codeBytes)))
	attrData.WriteBytes(codeBytes)
	attrData.WriteU16(0) // exception_table_length
	attrData.WriteU16(0) // attributes_count

	return AttributeInfo{
		NameIndex: g.cpIndex["utf8:Code"],
		Info:      attrData.Bytes(),
	}
}

// 常量池辅助方法

func (g *Generator) addUtf8(value string) uint16 {
	key := "utf8:" + value
	if idx, ok := g.cpIndex[key]; ok {
		return idx
	}
	g.constantPool = append(g.constantPool, &ConstantUtf8Info{Value: value})
	idx := uint16(len(g.constantPool))
	g.cpIndex[key] = idx
	return idx
}

func (g *Generator) addClass(name string) uint16 {
	key := "class:" + name
	if idx, ok := g.cpIndex[key]; ok {
		return idx
	}
	nameIdx := g.addUtf8(name)
	g.constantPool = append(g.constantPool, &ConstantClassInfo{NameIndex: nameIdx})
	idx := uint16(len(g.constantPool))
	g.cpIndex[key] = idx
	return idx
}

func (g *Generator) addString(value string) uint16 {
	key := "string:" + value
	if idx, ok := g.cpIndex[key]; ok {
		return idx
	}
	utf8Idx := g.addUtf8(value)
	g.constantPool = append(g.constantPool, &ConstantStringInfo{StringIndex: utf8Idx})
	idx := uint16(len(g.constantPool))
	g.cpIndex[key] = idx
	return idx
}

func (g *Generator) addNameAndType(name, descriptor string) uint16 {
	key := "nameandtype:" + name + ":" + descriptor
	if idx, ok := g.cpIndex[key]; ok {
		return idx
	}
	nameIdx := g.addUtf8(name)
	descIdx := g.addUtf8(descriptor)
	g.constantPool = append(g.constantPool, &ConstantNameAndTypeInfo{
		NameIndex:       nameIdx,
		DescriptorIndex: descIdx,
	})
	idx := uint16(len(g.constantPool))
	g.cpIndex[key] = idx
	return idx
}

func (g *Generator) addFieldref(className, name, descriptor string) uint16 {
	key := "fieldref:" + className + "." + name + ":" + descriptor
	if idx, ok := g.cpIndex[key]; ok {
		return idx
	}
	classIdx := g.addClass(className)
	natIdx := g.addNameAndType(name, descriptor)
	g.constantPool = append(g.constantPool, &ConstantFieldrefInfo{
		ClassIndex:       classIdx,
		NameAndTypeIndex: natIdx,
	})
	idx := uint16(len(g.constantPool))
	g.cpIndex[key] = idx
	return idx
}

func (g *Generator) addMethodref(className, name, descriptor string) uint16 {
	key := "methodref:" + className + "." + name + ":" + descriptor
	if idx, ok := g.cpIndex[key]; ok {
		return idx
	}
	classIdx := g.addClass(className)
	natIdx := g.addNameAndType(name, descriptor)
	g.constantPool = append(g.constantPool, &ConstantMethodrefInfo{
		ClassIndex:       classIdx,
		NameAndTypeIndex: natIdx,
	})
	idx := uint16(len(g.constantPool))
	g.cpIndex[key] = idx
	return idx
}
