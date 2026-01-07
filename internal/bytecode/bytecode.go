package bytecode

import (
	"encoding/binary"
	"fmt"
)

// OpCode 操作码类型
type OpCode byte

const (
	// 栈操作
	OpPush       OpCode = iota // 压入常量 (index: u16)
	OpPop                      // 弹出栈顶
	OpDup                      // 复制栈顶
	OpSwap                     // 交换栈顶两个元素

	// 局部变量操作
	OpLoadLocal  // 加载局部变量 (index: u16)
	OpStoreLocal // 存储局部变量 (index: u16)

	// 全局变量操作
	OpLoadGlobal  // 加载全局变量 (index: u16)
	OpStoreGlobal // 存储全局变量 (index: u16)

	// 常量
	OpNull  // 压入 null
	OpTrue  // 压入 true
	OpFalse // 压入 false
	OpZero  // 压入 0
	OpOne   // 压入 1

	// 算术运算
	OpAdd // 加法
	OpSub // 减法
	OpMul // 乘法
	OpDiv // 除法
	OpMod // 取模
	OpNeg // 取负

	// 比较运算
	OpEq  // 等于
	OpNe  // 不等于
	OpLt  // 小于
	OpLe  // 小于等于
	OpGt  // 大于
	OpGe  // 大于等于

	// 逻辑运算
	OpNot // 逻辑非
	OpAnd // 逻辑与 (短路)
	OpOr  // 逻辑或 (短路)

	// 位运算
	OpBitAnd // 位与
	OpBitOr  // 位或
	OpBitXor // 位异或
	OpBitNot // 位非
	OpShl    // 左移
	OpShr    // 右移

	// 字符串操作
	OpConcat            // 字符串拼接
	OpStringBuilderNew  // 创建字符串构建器
	OpStringBuilderAdd  // 向构建器追加字符串
	OpStringBuilderBuild // 构建最终字符串

	// 跳转指令
	OpJump        // 无条件跳转 (offset: i16)
	OpJumpIfFalse // 条件为假时跳转 (offset: i16)
	OpJumpIfTrue  // 条件为真时跳转 (offset: i16)
	OpLoop        // 循环跳转 (向后跳转, offset: u16)

	// 函数调用
	OpCall       // 调用函数 (argCount: u8)
	OpTailCall   // 尾调用 (argCount: u8) - 复用当前栈帧
	OpReturn     // 返回
	OpReturnNull // 返回 null
	OpClosure    // 创建闭包 (upvalueCount: u16)

	// 对象操作
	OpNewObject  // 创建对象 (classIndex: u16)
	OpGetField   // 获取字段 (nameIndex: u16)
	OpSetField   // 设置字段 (nameIndex: u16)
	OpCallMethod // 调用方法 (nameIndex: u16, argCount: u8)

	// 静态成员
	OpGetStatic  // 获取静态成员 (classIndex: u16, nameIndex: u16)
	OpSetStatic  // 设置静态成员 (classIndex: u16, nameIndex: u16)
	OpCallStatic // 调用静态方法 (classIndex: u16, nameIndex: u16, argCount: u8)

	// 数组操作
	OpNewArray         // 创建数组 (length: u16)
	OpNewFixedArray    // 创建定长数组 (capacity: u16, initLength: u16)
	OpArrayGet         // 获取数组元素（带边界检查）
	OpArraySet         // 设置数组元素（带边界检查）
	OpArrayLen         // 获取数组长度
	OpArrayGetUnchecked // 获取数组元素（无边界检查，用于循环优化）
	OpArraySetUnchecked // 设置数组元素（无边界检查，用于循环优化）

	// Map 操作
	OpNewMap  // 创建 Map (size: u16)
	OpMapGet  // 获取 Map 值
	OpMapSet  // 设置 Map 值
	OpMapHas  // 检查 key 是否存在
	OpMapLen  // 获取 Map 大小

	// SuperArray 万能数组操作
	OpSuperArrayNew // 创建万能数组 (元素个数: u16, 键值对标记: bytes)
	OpSuperArrayGet // 获取万能数组元素 [stack: arr, key -> value]
	OpSuperArraySet // 设置万能数组元素 [stack: arr, key, value -> arr]

	// 迭代器操作
	OpIterInit  // 初始化迭代器
	OpIterNext  // 获取下一个元素 (返回 true/false)
	OpIterKey   // 获取当前 key
	OpIterValue // 获取当前 value

	// 数组追加操作
	OpArrayPush    // 追加元素到数组
	OpArrayHas     // 检查索引/值是否存在

	// 字节数组操作
	OpNewBytes    // 从栈上整数创建字节数组 (count: u16)
	OpBytesGet    // 获取字节 bytes[i] -> int
	OpBytesSet    // 设置字节 bytes[i] = v
	OpBytesLen    // 获取字节数组长度
	OpBytesSlice  // 字节数组切片
	OpBytesConcat // 拼接两个字节数组

	// 对象销毁
	OpUnset        // 销毁对象并调用析构函数

	// 类型操作
	OpCheckType // 类型检查 (typeIndex: u16)
	OpCast      // 类型转换 (typeIndex: u16) - 失败抛出异常
	OpCastSafe  // 安全类型转换 (typeIndex: u16) - 失败返回 null

	// 异常处理
	OpThrow        // 抛出异常
	OpEnterTry     // 进入 try 块 (catchCount: u8, finallyOffset: i16, [typeIdx: u16, catchOffset: i16]*)
	OpLeaveTry     // 离开 try 块
	OpEnterCatch   // 进入 catch 块 (typeIdx: u16 - 期望的异常类型)
	OpEnterFinally // 进入 finally 块
	OpLeaveFinally // 离开 finally 块
	OpRethrow      // 重新抛出挂起的异常

	// 调试
	OpDebugPrint // 调试打印

	// 终止
	OpHalt // 停止执行
)

var opNames = map[OpCode]string{
	OpPush:        "PUSH",
	OpPop:         "POP",
	OpDup:         "DUP",
	OpSwap:        "SWAP",
	OpLoadLocal:   "LOAD_LOCAL",
	OpStoreLocal:  "STORE_LOCAL",
	OpLoadGlobal:  "LOAD_GLOBAL",
	OpStoreGlobal: "STORE_GLOBAL",
	OpNull:        "NULL",
	OpTrue:        "TRUE",
	OpFalse:       "FALSE",
	OpZero:        "ZERO",
	OpOne:         "ONE",
	OpAdd:         "ADD",
	OpSub:         "SUB",
	OpMul:         "MUL",
	OpDiv:         "DIV",
	OpMod:         "MOD",
	OpNeg:         "NEG",
	OpEq:          "EQ",
	OpNe:          "NE",
	OpLt:          "LT",
	OpLe:          "LE",
	OpGt:          "GT",
	OpGe:          "GE",
	OpNot:         "NOT",
	OpAnd:         "AND",
	OpOr:          "OR",
	OpBitAnd:      "BIT_AND",
	OpBitOr:       "BIT_OR",
	OpBitXor:      "BIT_XOR",
	OpBitNot:      "BIT_NOT",
	OpShl:         "SHL",
	OpShr:                "SHR",
	OpConcat:             "CONCAT",
	OpStringBuilderNew:   "STRING_BUILDER_NEW",
	OpStringBuilderAdd:   "STRING_BUILDER_ADD",
	OpStringBuilderBuild: "STRING_BUILDER_BUILD",
	OpJump:               "JUMP",
	OpJumpIfFalse: "JUMP_IF_FALSE",
	OpJumpIfTrue:  "JUMP_IF_TRUE",
	OpLoop:        "LOOP",
	OpCall:        "CALL",
	OpTailCall:    "TAIL_CALL",
	OpReturn:      "RETURN",
	OpReturnNull:  "RETURN_NULL",
	OpClosure:     "CLOSURE",
	OpNewObject:         "NEW_OBJECT",
	OpArrayGetUnchecked: "ARRAY_GET_UNCHECKED",
	OpArraySetUnchecked: "ARRAY_SET_UNCHECKED",
	OpGetField:    "GET_FIELD",
	OpSetField:    "SET_FIELD",
	OpCallMethod:  "CALL_METHOD",
	OpGetStatic:   "GET_STATIC",
	OpSetStatic:   "SET_STATIC",
	OpCallStatic:  "CALL_STATIC",
	OpNewArray:      "NEW_ARRAY",
	OpNewFixedArray: "NEW_FIXED_ARRAY",
	OpArrayGet:      "ARRAY_GET",
	OpArraySet:    "ARRAY_SET",
	OpArrayLen:    "ARRAY_LEN",
	OpNewMap:      "NEW_MAP",
	OpMapGet:         "MAP_GET",
	OpMapSet:         "MAP_SET",
	OpMapHas:         "MAP_HAS",
	OpMapLen:         "MAP_LEN",
	OpSuperArrayNew:  "SUPER_ARRAY_NEW",
	OpSuperArrayGet:  "SUPER_ARRAY_GET",
	OpSuperArraySet:  "SUPER_ARRAY_SET",
	OpIterInit:       "ITER_INIT",
	OpIterNext:    "ITER_NEXT",
	OpIterKey:     "ITER_KEY",
	OpIterValue:   "ITER_VALUE",
	OpArrayPush:   "ARRAY_PUSH",
	OpArrayHas:    "ARRAY_HAS",
	OpNewBytes:    "NEW_BYTES",
	OpBytesGet:    "BYTES_GET",
	OpBytesSet:    "BYTES_SET",
	OpBytesLen:    "BYTES_LEN",
	OpBytesSlice:  "BYTES_SLICE",
	OpBytesConcat: "BYTES_CONCAT",
	OpUnset:       "UNSET",
	OpCheckType:   "CHECK_TYPE",
	OpCast:        "CAST",
	OpCastSafe:    "CAST_SAFE",
	OpThrow:        "THROW",
	OpEnterTry:     "ENTER_TRY",
	OpLeaveTry:     "LEAVE_TRY",
	OpEnterCatch:   "ENTER_CATCH",
	OpEnterFinally: "ENTER_FINALLY",
	OpLeaveFinally: "LEAVE_FINALLY",
	OpRethrow:      "RETHROW",
	OpDebugPrint:  "DEBUG_PRINT",
	OpHalt:        "HALT",
}

func (op OpCode) String() string {
	if name, ok := opNames[op]; ok {
		return name
	}
	return fmt.Sprintf("UNKNOWN(%d)", op)
}

// CatchHandler 表示一个 catch 处理器
type CatchHandler struct {
	TypeName   string // 异常类型名 (如 "Exception", "RuntimeException")
	TypeIndex  uint16 // 类型名在常量池中的索引
	CatchOffset int   // catch 块的字节码偏移量
}

// Chunk 字节码块
type Chunk struct {
	Code      []byte     // 字节码
	Constants []Value    // 常量池
	Lines     []int      // 行号信息 (用于错误报告)
}

// NewChunk 创建新的字节码块
func NewChunk() *Chunk {
	return &Chunk{
		Code:      make([]byte, 0, 256),
		Constants: make([]Value, 0, 64),
		Lines:     make([]int, 0, 256),
	}
}

// Write 写入一个字节
func (c *Chunk) Write(b byte, line int) {
	c.Code = append(c.Code, b)
	c.Lines = append(c.Lines, line)
}

// WriteOp 写入操作码
func (c *Chunk) WriteOp(op OpCode, line int) {
	c.Write(byte(op), line)
}

// WriteU8 写入 uint8
func (c *Chunk) WriteU8(v uint8, line int) {
	c.Write(v, line)
}

// WriteU16 写入 uint16 (大端序)
func (c *Chunk) WriteU16(v uint16, line int) {
	c.Write(byte(v>>8), line)
	c.Write(byte(v), line)
}

// WriteI16 写入 int16 (大端序)
func (c *Chunk) WriteI16(v int16, line int) {
	c.WriteU16(uint16(v), line)
}

// AddConstant 添加常量，返回索引
func (c *Chunk) AddConstant(value Value) uint16 {
	c.Constants = append(c.Constants, value)
	return uint16(len(c.Constants) - 1)
}

// Len 返回字节码长度
func (c *Chunk) Len() int {
	return len(c.Code)
}

// ReadU16 从指定位置读取 uint16
func (c *Chunk) ReadU16(offset int) uint16 {
	return binary.BigEndian.Uint16(c.Code[offset:])
}

// ReadI16 从指定位置读取 int16
func (c *Chunk) ReadI16(offset int) int16 {
	return int16(c.ReadU16(offset))
}

// PatchJump 修补跳转偏移量
func (c *Chunk) PatchJump(offset int) {
	// 计算跳转距离
	jump := c.Len() - offset - 2
	if jump > 32767 || jump < -32768 {
		panic("jump offset out of range")
	}
	binary.BigEndian.PutUint16(c.Code[offset:], uint16(int16(jump)))
}

// Disassemble 反汇编字节码
func (c *Chunk) Disassemble(name string) string {
	var result string
	result += fmt.Sprintf("=== %s ===\n", name)
	
	offset := 0
	for offset < len(c.Code) {
		offset = c.disassembleInstruction(&result, offset)
	}
	
	return result
}

func (c *Chunk) disassembleInstruction(result *string, offset int) int {
	*result += fmt.Sprintf("%04d ", offset)
	
	// 显示行号
	if offset > 0 && c.Lines[offset] == c.Lines[offset-1] {
		*result += "   | "
	} else {
		*result += fmt.Sprintf("%4d ", c.Lines[offset])
	}
	
	op := OpCode(c.Code[offset])
	
	switch op {
	case OpPush, OpLoadLocal, OpStoreLocal, OpLoadGlobal, OpStoreGlobal,
		OpNewObject, OpGetField, OpSetField, OpNewArray, OpNewFixedArray, OpNewMap,
		OpCheckType, OpCast, OpCastSafe, OpSuperArrayNew:
		return c.constantInstruction(result, op, offset)
	case OpJump, OpJumpIfFalse, OpJumpIfTrue:
		return c.jumpInstruction(result, op, 1, offset)
	case OpLoop:
		return c.jumpInstruction(result, op, -1, offset)
	case OpCall, OpTailCall:
		return c.byteInstruction(result, op, offset)
	case OpCallMethod, OpCallStatic:
		return c.invokeInstruction(result, op, offset)
	case OpEnterTry:
		return c.enterTryInstruction(result, offset)
	case OpEnterCatch:
		return c.constantInstruction(result, op, offset)
	case OpClosure:
		return c.constantInstruction(result, op, offset)
	default:
		*result += fmt.Sprintf("%s\n", op)
		return offset + 1
	}
}

func (c *Chunk) constantInstruction(result *string, op OpCode, offset int) int {
	constant := c.ReadU16(offset + 1)
	*result += fmt.Sprintf("%-16s %4d '", op, constant)
	if int(constant) < len(c.Constants) {
		*result += c.Constants[constant].String()
	}
	*result += "'\n"
	return offset + 3
}

func (c *Chunk) jumpInstruction(result *string, op OpCode, sign int, offset int) int {
	jump := c.ReadI16(offset + 1)
	target := offset + 3 + int(jump)*sign
	*result += fmt.Sprintf("%-16s %4d -> %d\n", op, jump, target)
	return offset + 3
}

func (c *Chunk) byteInstruction(result *string, op OpCode, offset int) int {
	slot := c.Code[offset+1]
	*result += fmt.Sprintf("%-16s %4d\n", op, slot)
	return offset + 2
}

func (c *Chunk) invokeInstruction(result *string, op OpCode, offset int) int {
	nameIdx := c.ReadU16(offset + 1)
	argCount := c.Code[offset+3]
	*result += fmt.Sprintf("%-16s %4d (%d args)\n", op, nameIdx, argCount)
	return offset + 4
}

func (c *Chunk) enterTryInstruction(result *string, offset int) int {
	catchCount := c.Code[offset+1]
	finallyOffset := c.ReadI16(offset + 2)
	
	*result += fmt.Sprintf("ENTER_TRY       catches=%d, finally=%d\n", catchCount, finallyOffset)
	
	// 读取每个 catch 处理器信息
	pos := offset + 4
	for i := 0; i < int(catchCount); i++ {
		typeIdx := c.ReadU16(pos)
		catchOffset := c.ReadI16(pos + 2)
		typeName := ""
		if int(typeIdx) < len(c.Constants) {
			typeName = c.Constants[typeIdx].AsString()
		}
		*result += fmt.Sprintf("    catch[%d]: type='%s' offset=%d\n", i, typeName, catchOffset)
		pos += 4
	}
	
	return pos
}
