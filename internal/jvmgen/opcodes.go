package jvmgen

// JVM 操作码常量
// 只定义最小原型需要的操作码
const (
	// 常量操作
	OpAconstNull = 0x01 // 将 null 压入栈
	OpIconstM1   = 0x02 // 将 -1 压入栈
	OpIconst0    = 0x03 // 将 0 压入栈
	OpIconst1    = 0x04 // 将 1 压入栈
	OpIconst2    = 0x05 // 将 2 压入栈
	OpIconst3    = 0x06 // 将 3 压入栈
	OpIconst4    = 0x07 // 将 4 压入栈
	OpIconst5    = 0x08 // 将 5 压入栈
	OpBipush     = 0x10 // 将单字节常量压入栈
	OpSipush     = 0x11 // 将短整型常量压入栈
	OpLdc        = 0x12 // 将常量池中的项压入栈

	// 加载操作
	OpAload0 = 0x2A // 将局部变量 0 (引用类型) 压入栈
	OpAload1 = 0x2B // 将局部变量 1 (引用类型) 压入栈
	OpAload2 = 0x2C // 将局部变量 2 (引用类型) 压入栈
	OpAload3 = 0x2D // 将局部变量 3 (引用类型) 压入栈

	// 存储操作
	OpAstore0 = 0x4B // 将栈顶引用存入局部变量 0
	OpAstore1 = 0x4C // 将栈顶引用存入局部变量 1
	OpAstore2 = 0x4D // 将栈顶引用存入局部变量 2
	OpAstore3 = 0x4E // 将栈顶引用存入局部变量 3

	// 栈操作
	OpPop  = 0x57 // 弹出栈顶元素
	OpDup  = 0x59 // 复制栈顶元素
	OpSwap = 0x5F // 交换栈顶两个元素

	// 算术操作
	OpIadd = 0x60 // int 加法
	OpIsub = 0x64 // int 减法
	OpImul = 0x68 // int 乘法
	OpIdiv = 0x6C // int 除法

	// 控制流
	OpReturn  = 0xB1 // void 返回
	OpIreturn = 0xAC // int 返回
	OpAreturn = 0xB0 // 引用返回

	// 字段操作
	OpGetstatic = 0xB2 // 获取静态字段
	OpPutstatic = 0xB3 // 设置静态字段
	OpGetfield  = 0xB4 // 获取实例字段
	OpPutfield  = 0xB5 // 设置实例字段

	// 方法调用
	OpInvokevirtual   = 0xB6 // 调用实例方法
	OpInvokespecial   = 0xB7 // 调用构造方法/父类方法/私有方法
	OpInvokestatic    = 0xB8 // 调用静态方法
	OpInvokeinterface = 0xB9 // 调用接口方法

	// 对象操作
	OpNew        = 0xBB // 创建对象
	OpNewarray   = 0xBC // 创建基本类型数组
	OpAnewarray  = 0xBD // 创建引用类型数组
	OpArraylength = 0xBE // 获取数组长度

	// 类型转换
	OpCheckcast  = 0xC0 // 类型检查转换
	OpInstanceof = 0xC1 // 类型检查

	// 其他
	OpAthrow = 0xBF // 抛出异常
	OpNop    = 0x00 // 空操作
)
