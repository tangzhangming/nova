package bytecode

// ============================================================================
// Sola 编译产物文件格式定义
// ============================================================================

const (
	// CompiledFileExtension 编译产物文件后缀
	CompiledFileExtension = ".solac"

	// MagicNumber 文件魔数 "SOLA" in ASCII
	MagicNumber uint32 = 0x534F4C41

	// 版本号
	MajorVersion uint8 = 1
	MinorVersion uint8 = 0
)

// 常量池类型标记
const (
	ConstNull   uint8 = 0
	ConstBool   uint8 = 1
	ConstInt    uint8 = 2
	ConstFloat  uint8 = 3
	ConstString uint8 = 4
)

// 函数标志位
const (
	FuncFlagVariadic uint8 = 1 << 0 // 可变参数
	FuncFlagBuiltin  uint8 = 1 << 1 // 内置函数
)

// 类标志位
const (
	ClassFlagAbstract  uint8 = 1 << 0 // 抽象类
	ClassFlagInterface uint8 = 1 << 1 // 接口
)

// 方法标志位
const (
	MethodFlagStatic    uint8 = 1 << 0 // 静态方法
	MethodFlagPublic    uint8 = 1 << 1 // public
	MethodFlagProtected uint8 = 1 << 2 // protected
	MethodFlagPrivate   uint8 = 1 << 3 // private
)

// 属性标志位
const (
	PropFlagPublic    uint8 = 1 << 0 // public
	PropFlagProtected uint8 = 1 << 1 // protected
	PropFlagPrivate   uint8 = 1 << 2 // private
)

// 文件头结构大小
const HeaderSize = 24

