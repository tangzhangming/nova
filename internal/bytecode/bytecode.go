package bytecode

import (
	"encoding/binary"
	"fmt"
	"strings"
	"sync"
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

	// NativeArray 原生数组操作（类型化存储，JIT 友好）
	OpNativeArrayNew  // 创建原生数组 (elemType: u8, length: u16) [stack: -> arr]
	OpNativeArrayInit // 用栈上元素初始化数组 (elemType: u8, count: u16) [stack: v1, v2, ... -> arr]
	OpNativeArrayGet  // 获取元素 [stack: arr, idx -> value]
	OpNativeArraySet  // 设置元素 [stack: arr, idx, value -> ]
	OpNativeArrayLen  // 获取长度 [stack: arr -> length]

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

	// =========================================================================
	// 协程操作 (OOP 风格)
	// =========================================================================

	// 协程创建和控制
	OpGo    // 启动协程 [stack: closure -> goroutine_id] (语法糖, 不返回 Coroutine 对象)
	OpYield // 让出执行权（协作式调度）

	OpCoroutineSpawn  // 创建协程 [stack: closure -> coroutine] (返回 Coroutine 对象)
	OpCoroutineAwait  // 等待协程完成 (hasTimeout: u8) [stack: coroutine [, timeout] -> result]
	OpCoroutineCancel // 取消协程 [stack: coroutine -> bool]

	// 协程状态查询
	OpCoroutineIsCompleted // 检查是否完成 [stack: coroutine -> bool]
	OpCoroutineIsCancelled // 检查是否已取消 [stack: coroutine -> bool]
	OpCoroutineGetResult   // 获取结果（非阻塞，未完成返回 null）[stack: coroutine -> value]
	OpCoroutineGetException // 获取异常 [stack: coroutine -> exception|null]
	OpCoroutineGetID       // 获取协程 ID [stack: coroutine -> int]

	// 协程组合操作
	OpCoroutineAll  // 等待所有协程 [stack: coroutine[] -> result[]]
	OpCoroutineAny  // 等待任一成功 [stack: coroutine[] -> result]
	OpCoroutineRace // 等待最快完成 [stack: coroutine[] -> result]

	// 延迟
	OpCoroutineDelay // 延迟执行 [stack: ms -> coroutine<void>]

	// =========================================================================
	// 通道操作 (OOP 风格)
	// =========================================================================

	OpChanMake    // 创建通道 (capacity: u16) [stack: -> channel]
	OpChanSend    // 发送到通道 [stack: channel, value -> ]
	OpChanRecv    // 从通道接收 [stack: channel -> value]
	OpChanClose   // 关闭通道 [stack: channel -> ]
	OpChanTrySend // 非阻塞发送 [stack: channel, value -> bool]
	OpChanTryRecv // 非阻塞接收 [stack: channel -> value, bool]

	// 通道状态查询
	OpChanLen      // 获取通道缓冲区长度 [stack: channel -> int]
	OpChanCap      // 获取通道容量 [stack: channel -> int]
	OpChanIsClosed // 检查通道是否已关闭 [stack: channel -> bool]

	// select 操作 (OOP 风格: Channel::select())
	OpSelectStart   // 开始 select (caseCount: u8)
	OpSelectCase    // 添加 case (isRecv: u8, jumpOffset: i16)
	OpSelectDefault // 添加 default (jumpOffset: i16)
	OpSelectWait    // 等待 select 完成，返回匹配的 case 索引

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
	OpNativeArrayNew:  "NATIVE_ARRAY_NEW",
	OpNativeArrayInit: "NATIVE_ARRAY_INIT",
	OpNativeArrayGet:  "NATIVE_ARRAY_GET",
	OpNativeArraySet:  "NATIVE_ARRAY_SET",
	OpNativeArrayLen:  "NATIVE_ARRAY_LEN",
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
	OpDebugPrint:    "DEBUG_PRINT",

	// 协程操作
	OpGo:                    "GO",
	OpYield:                 "YIELD",
	OpCoroutineSpawn:        "COROUTINE_SPAWN",
	OpCoroutineAwait:        "COROUTINE_AWAIT",
	OpCoroutineCancel:       "COROUTINE_CANCEL",
	OpCoroutineIsCompleted:  "COROUTINE_IS_COMPLETED",
	OpCoroutineIsCancelled:  "COROUTINE_IS_CANCELLED",
	OpCoroutineGetResult:    "COROUTINE_GET_RESULT",
	OpCoroutineGetException: "COROUTINE_GET_EXCEPTION",
	OpCoroutineGetID:        "COROUTINE_GET_ID",
	OpCoroutineAll:          "COROUTINE_ALL",
	OpCoroutineAny:          "COROUTINE_ANY",
	OpCoroutineRace:         "COROUTINE_RACE",
	OpCoroutineDelay:        "COROUTINE_DELAY",

	// 通道操作
	OpChanMake:      "CHAN_MAKE",
	OpChanSend:      "CHAN_SEND",
	OpChanRecv:      "CHAN_RECV",
	OpChanClose:     "CHAN_CLOSE",
	OpChanTrySend:   "CHAN_TRY_SEND",
	OpChanTryRecv:   "CHAN_TRY_RECV",
	OpChanLen:       "CHAN_LEN",
	OpChanCap:       "CHAN_CAP",
	OpChanIsClosed:  "CHAN_IS_CLOSED",
	OpSelectStart:   "SELECT_START",
	OpSelectCase:    "SELECT_CASE",
	OpSelectDefault: "SELECT_DEFAULT",
	OpSelectWait:    "SELECT_WAIT",

	OpHalt: "HALT",
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

// ============================================================================
// Chunk 对象池
// ============================================================================
// 使用 sync.Pool 复用 Chunk 对象，减少内存分配和 GC 压力
// 特别适合编译大量小函数/方法时的场景

var chunkPool = sync.Pool{
	New: func() interface{} {
		return &Chunk{
			Code:      make([]byte, 0, 256),
			Constants: make([]Value, 0, 64),
			Lines:     make([]int, 0, 256),
		}
	},
}

// NewChunk 创建新的字节码块
// 优化：从对象池获取，避免频繁分配
func NewChunk() *Chunk {
	chunk := chunkPool.Get().(*Chunk)
	// 确保切片是空的但保留容量
	chunk.Code = chunk.Code[:0]
	chunk.Constants = chunk.Constants[:0]
	chunk.Lines = chunk.Lines[:0]
	return chunk
}

// Release 将 Chunk 归还到对象池
// 调用者在确定不再使用 Chunk 时应调用此方法
// 注意：归还后不应再访问该 Chunk
func (c *Chunk) Release() {
	if c == nil {
		return
	}
	// 清空切片但保留底层数组
	c.Code = c.Code[:0]
	c.Constants = c.Constants[:0]
	c.Lines = c.Lines[:0]
	chunkPool.Put(c)
}

// Clone 深拷贝 Chunk
// 用于需要保留 Chunk 副本的场景
func (c *Chunk) Clone() *Chunk {
	clone := NewChunk()
	clone.Code = append(clone.Code, c.Code...)
	clone.Constants = append(clone.Constants, c.Constants...)
	clone.Lines = append(clone.Lines, c.Lines...)
	return clone
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
// 优化：使用 strings.Builder 避免字符串拼接开销
func (c *Chunk) Disassemble(name string) string {
	var sb strings.Builder
	// 预估大小：每条指令约 30 字节输出
	sb.Grow(len(c.Code) * 30)
	
	sb.WriteString("=== ")
	sb.WriteString(name)
	sb.WriteString(" ===\n")
	
	offset := 0
	for offset < len(c.Code) {
		offset = c.disassembleInstruction(&sb, offset)
	}
	
	return sb.String()
}

func (c *Chunk) disassembleInstruction(sb *strings.Builder, offset int) int {
	fmt.Fprintf(sb, "%04d ", offset)
	
	// 显示行号
	if offset > 0 && c.Lines[offset] == c.Lines[offset-1] {
		sb.WriteString("   | ")
	} else {
		fmt.Fprintf(sb, "%4d ", c.Lines[offset])
	}
	
	op := OpCode(c.Code[offset])
	
	switch op {
	case OpPush, OpLoadLocal, OpStoreLocal, OpLoadGlobal, OpStoreGlobal,
		OpNewObject, OpGetField, OpSetField, OpNewArray, OpNewFixedArray, OpNewMap,
		OpCheckType, OpCast, OpCastSafe, OpSuperArrayNew:
		return c.constantInstruction(sb, op, offset)
	case OpJump, OpJumpIfFalse, OpJumpIfTrue:
		return c.jumpInstruction(sb, op, 1, offset)
	case OpLoop:
		return c.jumpInstruction(sb, op, -1, offset)
	case OpCall, OpTailCall:
		return c.byteInstruction(sb, op, offset)
	case OpCallMethod, OpCallStatic:
		return c.invokeInstruction(sb, op, offset)
	case OpEnterTry:
		return c.enterTryInstruction(sb, offset)
	case OpEnterCatch:
		return c.constantInstruction(sb, op, offset)
	case OpClosure:
		return c.constantInstruction(sb, op, offset)
	default:
		fmt.Fprintf(sb, "%s\n", op)
		return offset + 1
	}
}

func (c *Chunk) constantInstruction(sb *strings.Builder, op OpCode, offset int) int {
	constant := c.ReadU16(offset + 1)
	fmt.Fprintf(sb, "%-16s %4d '", op, constant)
	if int(constant) < len(c.Constants) {
		sb.WriteString(c.Constants[constant].String())
	}
	sb.WriteString("'\n")
	return offset + 3
}

func (c *Chunk) jumpInstruction(sb *strings.Builder, op OpCode, sign int, offset int) int {
	jump := c.ReadI16(offset + 1)
	target := offset + 3 + int(jump)*sign
	fmt.Fprintf(sb, "%-16s %4d -> %d\n", op, jump, target)
	return offset + 3
}

func (c *Chunk) byteInstruction(sb *strings.Builder, op OpCode, offset int) int {
	slot := c.Code[offset+1]
	fmt.Fprintf(sb, "%-16s %4d\n", op, slot)
	return offset + 2
}

func (c *Chunk) invokeInstruction(sb *strings.Builder, op OpCode, offset int) int {
	nameIdx := c.ReadU16(offset + 1)
	argCount := c.Code[offset+3]
	fmt.Fprintf(sb, "%-16s %4d (%d args)\n", op, nameIdx, argCount)
	return offset + 4
}

func (c *Chunk) enterTryInstruction(sb *strings.Builder, offset int) int {
	catchCount := c.Code[offset+1]
	finallyOffset := c.ReadI16(offset + 2)
	
	fmt.Fprintf(sb, "ENTER_TRY       catches=%d, finally=%d\n", catchCount, finallyOffset)
	
	// 读取每个 catch 处理器信息
	pos := offset + 4
	for i := 0; i < int(catchCount); i++ {
		typeIdx := c.ReadU16(pos)
		catchOffset := c.ReadI16(pos + 2)
		typeName := ""
		if int(typeIdx) < len(c.Constants) {
			typeName = c.Constants[typeIdx].AsString()
		}
		fmt.Fprintf(sb, "    catch[%d]: type='%s' offset=%d\n", i, typeName, catchOffset)
		pos += 4
	}
	
	return pos
}
