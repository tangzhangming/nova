// Package vm 实现了 Sola 编程语言的字节码虚拟机。
//
// # 概述
//
// 该虚拟机是一个基于栈的解释器，负责执行由编译器生成的字节码指令。
// 它是 Sola 语言运行时的核心组件，提供了完整的程序执行能力。
//
// # 架构设计
//
// 虚拟机采用经典的栈式架构，主要包含以下核心组件：
//
//   - 操作数栈 (stack): 用于存储操作数和中间计算结果
//   - 调用栈 (frames): 管理函数调用和返回
//   - 全局环境 (globals): 存储全局变量
//   - 类型系统 (classes/enums): 管理类和枚举定义
//   - 异常处理系统 (tryStack): 实现 try-catch-finally 机制
//
// # 性能优化
//
// 虚拟机集成了多项性能优化技术：
//
//   - JIT 编译器: 将热点代码编译为本机代码
//   - 内联缓存 (IC): 加速方法调用的类型检查
//   - 热点检测: 识别频繁执行的代码路径
//   - 垃圾回收 (GC): 自动内存管理，支持分代回收
//   - 尾调用优化: 消除尾递归的栈空间开销
//
// # 指令集
//
// 虚拟机支持完整的指令集，包括：
//   - 栈操作: OpPush, OpPop, OpDup, OpSwap 等
//   - 算术运算: OpAdd, OpSub, OpMul, OpDiv, OpMod, OpNeg
//   - 比较运算: OpEq, OpNe, OpLt, OpLe, OpGt, OpGe
//   - 逻辑运算: OpNot, OpBitAnd, OpBitOr 等
//   - 控制流: OpJump, OpJumpIfFalse, OpLoop, OpCall, OpReturn
//   - 对象操作: OpNewObject, OpGetField, OpSetField, OpCallMethod
//   - 异常处理: OpThrow, OpEnterTry, OpLeaveTry, OpEnterCatch
//
// # 线程安全
//
// 当前实现为单线程模型，每个 VM 实例不应在多个 goroutine 间共享。
// 如需并发执行，应为每个 goroutine 创建独立的 VM 实例。
//
// # 使用示例
//
//	// 创建虚拟机
//	vm := vm.New()
//
//	// 注册全局变量和类
//	vm.DefineGlobal("VERSION", bytecode.NewString("1.0"))
//	vm.DefineClass(myClass)
//
//	// 执行字节码
//	result := vm.Run(compiledFunction)
//	if result != vm.InterpretOK {
//	    fmt.Println("执行错误:", vm.GetError())
//	}
package vm

import (
	// 标准库
	"fmt"     // 格式化输出，用于错误信息和调试
	"strconv" // 字符串转换，用于类型转换操作
	"strings" // 字符串处理，用于类型名解析

	// 内部依赖
	"github.com/tangzhangming/nova/internal/bytecode" // 字节码定义：指令集、值类型、函数/类结构
	"github.com/tangzhangming/nova/internal/errors"   // 错误处理：增强的错误报告系统
	"github.com/tangzhangming/nova/internal/i18n"     // 国际化：多语言错误消息支持
	"github.com/tangzhangming/nova/internal/jit"      // JIT编译器：热点代码编译优化
)

// ============================================================================
// 虚拟机配置常量
// ============================================================================
//
// 这些常量定义了虚拟机的核心资源限制。
// 修改这些值会影响内存占用和程序的最大复杂度。

const (
	// StackMax 定义操作数栈的最大深度（256 个槽位）。
	//
	// 操作数栈用于存储：
	//   - 表达式计算的中间结果
	//   - 函数调用的参数
	//   - 局部变量
	//   - 临时对象引用
	//
	// 256 个槽位足以支持：
	//   - 深度嵌套的表达式（如 a + b * c / d - e ...）
	//   - 最多约 200 个参数的函数调用（需预留空间给返回值和临时值）
	//   - 复杂的闭包链
	//
	// 如果程序需要更深的栈，会触发栈溢出错误。
	// 注意：增大此值会增加每个 VM 实例的内存占用（每槽约 24 字节）。
	StackMax = 256

	// FramesMax 定义调用栈的最大深度（64 层）。
	//
	// 调用栈记录函数调用链，每个 CallFrame 包含：
	//   - 当前执行的闭包
	//   - 指令指针 (IP)
	//   - 栈基址 (BaseSlot)
	//
	// 64 层深度意味着：
	//   - 最多支持 64 层嵌套函数调用
	//   - 递归深度限制为 64 层（除非使用尾调用优化）
	//
	// 对于大多数程序，64 层已足够。如需更深递归：
	//   - 考虑使用尾递归（VM 会自动优化为循环）
	//   - 或将递归算法改写为迭代形式
	//
	// 注意：增大此值会增加每个 VM 实例的内存占用（每帧约 32 字节）。
	FramesMax = 64
)

// ============================================================================
// 执行结果类型
// ============================================================================

// InterpretResult 表示虚拟机执行字节码后的结果状态。
//
// 这是一个枚举类型，用于向调用者报告执行的最终状态。
// 调用者应根据返回的结果类型决定后续处理逻辑。
type InterpretResult int

const (
	// InterpretOK 表示程序执行成功完成。
	// 这是正常的退出状态，表示没有错误发生。
	InterpretOK InterpretResult = iota

	// InterpretCompileError 表示编译阶段发生错误。
	// 当前 VM 实现中较少使用此状态，因为编译通常在 VM 外部完成。
	// 保留此状态是为了与完整的编译-执行流程兼容。
	InterpretCompileError

	// InterpretRuntimeError 表示运行时发生未捕获的错误。
	// 可能的原因包括：
	//   - 类型错误（如对非数字类型执行算术运算）
	//   - 未定义的变量或方法
	//   - 数组越界
	//   - 除零错误
	//   - 栈溢出
	//   - 未捕获的异常
	// 调用 vm.GetError() 可获取详细错误信息。
	InterpretRuntimeError

	// InterpretExceptionHandled 表示异常已被 catch 块捕获处理。
	// 这是一个内部状态，用于指示执行循环需要：
	//   - 刷新当前帧 (frame) 引用
	//   - 刷新当前字节码块 (chunk) 引用
	// 因为异常处理可能导致栈展开，改变执行上下文。
	// 此状态通常不会返回给外部调用者。
	InterpretExceptionHandled
)

// ============================================================================
// 调用帧结构
// ============================================================================

// CallFrame 表示一个函数调用的执行上下文（调用帧/栈帧）。
//
// 每次函数调用都会创建一个新的 CallFrame，函数返回时销毁。
// CallFrame 记录了恢复调用者执行所需的所有信息。
//
// # 栈布局
//
// 假设函数 foo(a, b) 调用 bar(x, y, z)，栈布局如下：
//
//	                    ┌─────────────────┐
//	                    │ bar 的局部变量   │
//	                    ├─────────────────┤
//	                    │ z (参数)         │
//	                    │ y (参数)         │
//	                    │ x (参数)         │
//	bar.BaseSlot ─────► │ bar 闭包        │
//	                    ├─────────────────┤
//	                    │ foo 的局部变量   │
//	                    │ b (参数)         │
//	                    │ a (参数)         │
//	foo.BaseSlot ─────► │ foo 闭包        │
//	                    └─────────────────┘
//	                    栈底 (stackTop=0)
//
// # 字段说明
//
//   - Closure: 当前执行的闭包，包含函数代码和捕获的自由变量
//   - IP: 指令指针，指向下一条要执行的字节码指令
//   - BaseSlot: 栈基址，用于定位局部变量（local[i] = stack[BaseSlot + i]）
type CallFrame struct {
	// Closure 当前执行的闭包。
	// 闭包包含：
	//   - Function: 函数定义（字节码、参数信息、局部变量数等）
	//   - Upvalues: 捕获的外层变量（用于实现闭包）
	Closure *bytecode.Closure

	// IP (Instruction Pointer) 指令指针。
	// 指向当前字节码块中下一条要执行的指令的索引。
	// 执行时先读取 chunk.Code[IP]，然后 IP++。
	IP int

	// BaseSlot 栈基址（帧指针）。
	// 指向当前帧在操作数栈中的起始位置。
	// 局部变量通过相对于 BaseSlot 的偏移量访问：
	//   - slot 0: 函数/闭包自身（用于递归调用）
	//   - slot 1 ~ Arity: 函数参数
	//   - slot Arity+1 ~ LocalCount: 局部变量
	BaseSlot int
}

// ============================================================================
// 异常处理结构
// ============================================================================

// CatchHandlerInfo 描述一个 catch 子句的处理器信息。
//
// 一个 try 块可以有多个 catch 子句，每个 catch 捕获不同类型的异常。
// 当异常发生时，VM 按顺序检查每个 CatchHandlerInfo，
// 找到第一个类型匹配的处理器并跳转执行。
//
// # 示例
//
// 对于以下代码：
//
//	try {
//	    riskyOperation()
//	} catch (FileNotFoundException e) {  // handler[0]
//	    handleFileNotFound(e)
//	} catch (IOException e) {            // handler[1]
//	    handleIOError(e)
//	} catch (Exception e) {              // handler[2]
//	    handleGeneric(e)
//	}
//
// 将生成 3 个 CatchHandlerInfo，按声明顺序排列。
// 这确保了更具体的异常类型优先匹配。
type CatchHandlerInfo struct {
	// TypeName 是此 catch 子句捕获的异常类型名称。
	// 可以是：
	//   - 具体异常类名：如 "FileNotFoundException", "DivideByZeroException"
	//   - 基类异常名：如 "Exception", "Throwable"
	//   - 空字符串：捕获所有异常（catch-all）
	TypeName string

	// CatchOffset 是 catch 块代码相对于 OpEnterTry 指令的字节偏移量。
	// 当异常匹配此处理器时，VM 将 IP 设置为：
	//   IP = EnterTryIP + CatchOffset
	// 然后开始执行 catch 块的代码。
	CatchOffset int
}

// TryContext 表示一个 try-catch-finally 块的完整执行上下文。
//
// TryContext 在进入 try 块时创建并压入 tryStack，
// 在离开整个 try-catch-finally 结构时弹出。
// 它保存了异常处理所需的所有状态信息。
//
// # 异常处理流程
//
//  1. 进入 try 块时（OpEnterTry）：
//     - 创建 TryContext，记录当前帧数和栈顶
//     - 解析并存储所有 catch 处理器
//
//  2. try 块正常执行完毕（OpLeaveTry）：
//     - 如果有 finally，跳转执行 finally
//     - 如果没有 finally，移除 TryContext
//
//  3. 异常发生时（OpThrow 或运行时错误）：
//     - 展开栈到 try 块所在的帧
//     - 查找匹配的 catch 处理器
//     - 如果找到，跳转到 catch 块执行
//     - 如果没有匹配但有 finally，先执行 finally 再传播异常
//
//  4. catch 块执行完毕：
//     - 如果有 finally，跳转执行 finally
//
//  5. finally 块执行完毕（OpLeaveFinally）：
//     - 检查是否有挂起的异常，有则重新抛出
//     - 检查是否有挂起的返回值，有则执行返回
//     - 否则继续正常执行
//
// # 嵌套 try 块
//
// try 块可以嵌套，每个 try 块都有独立的 TryContext。
// tryStack 按进入顺序存储，异常发生时从栈顶开始查找处理器。
type TryContext struct {
	// EnterTryIP 记录 OpEnterTry 指令在字节码中的位置。
	// 用于计算 catch 块的绝对跳转地址：
	//   catchIP = EnterTryIP + handler.CatchOffset
	EnterTryIP int

	// CatchHandlers 存储所有 catch 子句的处理器信息。
	// 按源代码声明顺序排列，异常匹配时按此顺序检查。
	// 更具体的异常类型应该排在前面。
	CatchHandlers []CatchHandlerInfo

	// FinallyIP 是 finally 块的起始指令地址。
	// 值为 -1 表示没有 finally 块。
	// 无论 try/catch 如何退出，finally 块都会执行。
	FinallyIP int

	// FrameCount 记录进入 try 块时的调用帧数。
	// 异常发生时，需要展开调用栈到此帧数，
	// 确保栈帧状态与进入 try 时一致。
	FrameCount int

	// StackTop 记录进入 try 块时的操作数栈顶位置。
	// 异常发生时，恢复栈顶到此位置，
	// 清除 try 块执行期间压入的临时值。
	StackTop int

	// InCatch 标记当前是否正在执行 catch 块。
	// 如果 catch 块中再次发生异常：
	//   - 不能再次被同一 try 的其他 catch 捕获
	//   - 需要先执行 finally（如果有），然后向外传播
	InCatch bool

	// InFinally 标记当前是否正在执行 finally 块。
	// finally 块中发生的异常会覆盖原有异常。
	InFinally bool

	// PendingException 存储挂起的异常值。
	// 场景：catch 中发生新异常，需要先执行 finally，
	// finally 结束后检查此值决定是否重新抛出。
	PendingException bytecode.Value

	// HasPendingExc 标记是否有挂起的异常等待处理。
	// 与 PendingException 配合使用，因为零值异常也是有效值。
	HasPendingExc bool

	// PendingReturn 存储挂起的返回值。
	// 场景：try/catch 中执行 return，但需要先执行 finally，
	// finally 结束后使用此值完成返回。
	PendingReturn bytecode.Value

	// HasPendingReturn 标记是否有挂起的返回操作。
	// 与 PendingReturn 配合使用。
	HasPendingReturn bool
}

// ============================================================================
// 虚拟机核心结构
// ============================================================================

// VM 是 Sola 语言的字节码虚拟机，负责执行编译后的字节码程序。
//
// VM 是一个基于栈的解释器，采用经典的 fetch-decode-execute 循环：
//  1. Fetch: 从当前帧的字节码中读取下一条指令
//  2. Decode: 解析指令的操作码和操作数
//  3. Execute: 执行指令，可能修改栈、帧或全局状态
//
// # 内存布局
//
// VM 的内存主要分为以下区域：
//
//	┌─────────────────────────────────────────────────────────────┐
//	│                      操作数栈 (stack)                        │
//	│  存储计算的中间结果、函数参数、局部变量                          │
//	│  大小固定为 StackMax (256) 个槽位                            │
//	├─────────────────────────────────────────────────────────────┤
//	│                      调用栈 (frames)                         │
//	│  存储函数调用的上下文信息                                      │
//	│  大小固定为 FramesMax (64) 个帧                              │
//	├─────────────────────────────────────────────────────────────┤
//	│                      全局环境                                │
//	│  globals: 全局变量表                                         │
//	│  classes: 类定义表                                          │
//	│  enums: 枚举定义表                                           │
//	├─────────────────────────────────────────────────────────────┤
//	│                      异常处理栈                               │
//	│  tryStack: try-catch-finally 上下文栈                       │
//	├─────────────────────────────────────────────────────────────┤
//	│                      运行时子系统                             │
//	│  gc: 垃圾回收器                                              │
//	│  icManager: 内联缓存管理器                                   │
//	│  hotspotDetector: 热点检测器                                 │
//	│  jitCompiler: JIT 编译器                                    │
//	└─────────────────────────────────────────────────────────────┘
//
// # 生命周期
//
//  1. 创建: 使用 New() 或 NewWithConfig() 创建 VM 实例
//  2. 配置: 注册全局变量、类、枚举等
//  3. 执行: 调用 Run() 执行编译后的函数
//  4. 清理: VM 实例可被 GC 自动回收，无需手动销毁
//
// # 线程安全性
//
// VM 实例不是线程安全的，不应在多个 goroutine 间共享。
// 每个 goroutine 应创建独立的 VM 实例。
//
// # 性能考虑
//
// VM 结构体中的数组（frames、stack）使用固定大小而非切片，
// 这是为了：
//   - 避免切片扩容的内存分配
//   - 提高缓存局部性
//   - 简化边界检查逻辑
type VM struct {
	// =========================================================================
	// 调用栈 - 管理函数调用的上下文
	// =========================================================================

	// frames 是调用帧数组，存储嵌套的函数调用上下文。
	// 使用固定大小数组而非切片，避免动态扩容的开销。
	// 数组索引 0 到 frameCount-1 为有效帧。
	frames [FramesMax]CallFrame

	// frameCount 当前活动的调用帧数量。
	// 增加: 函数调用时 (OpCall)
	// 减少: 函数返回时 (OpReturn)
	// 为 0 时表示程序执行完毕。
	frameCount int

	// =========================================================================
	// 操作数栈 - 存储计算的中间值
	// =========================================================================

	// stack 是操作数栈，所有计算都通过此栈进行。
	// 使用固定大小数组，通过 stackTop 管理有效元素。
	// 栈增长方向：从低地址向高地址（stack[0] 是栈底）。
	stack [StackMax]bytecode.Value

	// stackTop 指向栈顶的下一个空闲位置。
	// push 操作: stack[stackTop] = value; stackTop++
	// pop 操作: stackTop--; return stack[stackTop]
	// 栈为空时 stackTop = 0。
	stackTop int

	// =========================================================================
	// 全局环境 - 存储全局定义
	// =========================================================================

	// globals 全局变量表，存储所有全局变量。
	// 键: 变量名
	// 值: 变量值（bytecode.Value）
	// 全局变量在整个程序生命周期内可见。
	globals map[string]bytecode.Value

	// classes 类定义表，存储所有已注册的类。
	// 键: 类名（包含命名空间时为完整路径）
	// 值: 类定义（包含方法、属性、继承信息等）
	// 对象创建时 (OpNewObject) 从此表查找类定义。
	classes map[string]*bytecode.Class

	// enums 枚举定义表，存储所有已注册的枚举。
	// 键: 枚举名
	// 值: 枚举定义（包含枚举成员及其值）
	// 静态访问时 (OpGetStatic) 会先检查此表。
	enums map[string]*bytecode.Enum

	// =========================================================================
	// 异常处理 - 实现 try-catch-finally 机制
	// =========================================================================

	// tryStack 是 try 块上下文栈，支持嵌套的异常处理。
	// 进入 try 块时压入新的 TryContext，离开时弹出。
	// 使用切片而非固定数组，因为嵌套深度通常较浅但理论上无限。
	tryStack []TryContext

	// tryDepth 是 try 块的嵌套深度计数器。
	// 这是一个优化：快速判断是否在 try 块中，
	// 避免每次都检查 len(tryStack) > 0。
	// 在异常处理的"快速路径"中特别有用。
	tryDepth int

	// exception 存储当前正在处理的异常值。
	// 当 hasException 为 true 时有效。
	exception bytecode.Value

	// hasException 标记是否有未处理的异常。
	// 与 exception 配合使用，因为零值也是有效的异常值。
	hasException bool

	// =========================================================================
	// 垃圾回收子系统
	// =========================================================================

	// gc 垃圾回收器，负责自动内存管理。
	// 功能包括：
	//   - 追踪堆上分配的对象
	//   - 标记-清除回收不可达对象
	//   - 分代回收优化（年轻代/老年代）
	//   - 写屏障支持
	gc *GC

	// =========================================================================
	// 性能优化子系统
	// =========================================================================

	// icManager 内联缓存管理器（Inline Cache Manager）。
	// 用于加速多态方法调用：
	//   - 缓存类型 -> 方法的映射
	//   - 避免每次方法调用都进行查找
	//   - 特别适合单态和少数多态的调用点
	icManager *ICManager

	// hotspotDetector 热点检测器。
	// 功能：
	//   - 统计函数调用和循环迭代次数
	//   - 识别"热点"代码（频繁执行的代码）
	//   - 触发 JIT 编译决策
	hotspotDetector *HotspotDetector

	// =========================================================================
	// JIT 编译子系统
	// =========================================================================

	// jitCompiler JIT 编译器实例。
	// 将热点字节码编译为本机代码以提升性能。
	// 可能为 nil（平台不支持或被禁用）。
	jitCompiler *jit.Compiler

	// jitEnabled 标记 JIT 执行是否启用。
	// 即使 jitCompiler 不为 nil，也可以动态禁用 JIT。
	// 用于调试或特定场景下强制使用解释执行。
	jitEnabled bool

	// =========================================================================
	// 协程支持
	// =========================================================================

	// scheduler 协程调度器
	// 管理所有协程的创建、调度和销毁
	scheduler *Scheduler

	// =========================================================================
	// 错误状态
	// =========================================================================

	// hadError 标记执行过程中是否发生过错误。
	// 用于在执行结束后检查是否成功。
	hadError bool

	// errorMessage 存储最后一次错误的详细信息。
	// 当 hadError 为 true 时，此字段包含错误描述。
	// 通过 GetError() 方法获取。
	errorMessage string
}

// ============================================================================
// 构造函数
// ============================================================================

// New 创建一个使用默认配置的虚拟机实例。
//
// 默认配置包括：
//   - 启用 JIT 编译（如果平台支持）
//   - 启用垃圾回收
//   - 启用内联缓存
//   - 启用热点检测
//
// 这是创建 VM 最简单的方式，适合大多数使用场景。
//
// # 示例
//
//	vm := vm.New()
//	result := vm.Run(compiledFunction)
//
// # 返回值
//
// 返回初始化完成的 VM 实例，可立即用于执行字节码。
func New() *VM {
	return NewWithConfig(nil)
}

// NewWithConfig 创建一个使用指定配置的虚拟机实例。
//
// 此函数提供了对 VM 初始化的完全控制，特别是 JIT 编译器的配置。
//
// # 参数
//
//   - jitConfig: JIT 编译器配置。传入 nil 使用默认配置。
//     要完全禁用 JIT，传入 jit.InterpretOnlyConfig()。
//
// # JIT 配置选项
//
// JIT 配置通常包括：
//   - 热点阈值：函数被调用多少次后触发编译
//   - 编译级别：优化程度
//   - 内存限制：JIT 代码缓存大小
//
// # 初始化流程
//
//  1. 创建基础 VM 结构体
//  2. 初始化全局环境（globals、classes、enums）
//  3. 创建 GC 实例
//  4. 创建内联缓存管理器
//  5. 创建热点检测器
//  6. 初始化 JIT 编译器（如果启用）
//  7. 注册热点回调
//
// # 示例
//
//	// 使用默认 JIT 配置
//	vm1 := vm.NewWithConfig(nil)
//
//	// 禁用 JIT（纯解释执行）
//	vm2 := vm.NewWithConfig(jit.InterpretOnlyConfig())
//
//	// 自定义 JIT 配置
//	config := &jit.Config{HotThreshold: 100}
//	vm3 := vm.NewWithConfig(config)
func NewWithConfig(jitConfig *jit.Config) *VM {
	// 创建 VM 实例并初始化基础组件
	vm := &VM{
		// 全局环境初始化为空映射
		globals: make(map[string]bytecode.Value),
		classes: make(map[string]*bytecode.Class),
		enums:   make(map[string]*bytecode.Enum),

		// 创建运行时子系统
		gc:              NewGC(),              // 垃圾回收器
		icManager:       NewICManager(),       // 内联缓存管理器
		hotspotDetector: NewHotspotDetector(), // 热点检测器
		scheduler:       NewScheduler(),       // 协程调度器
	}

	// 初始化 JIT 编译器
	// jit.NewCompiler 会根据平台和配置决定是否实际创建编译器
	// 在不支持 JIT 的平台上，返回 nil
	vm.jitCompiler = jit.NewCompiler(jitConfig)
	vm.jitEnabled = vm.jitCompiler != nil && vm.jitCompiler.IsEnabled()

	// 设置热点检测回调
	// 当函数被检测为"热点"时，触发 JIT 编译
	if vm.jitEnabled {
		profiler := vm.jitCompiler.GetProfiler()
		if profiler != nil {
			// 注册回调：当函数变热时调用 onJITFunctionHot
			profiler.OnFunctionHot(vm.onJITFunctionHot)
		}
	}

	return vm
}

// ============================================================================
// JIT 编译相关方法
// ============================================================================

// onJITFunctionHot 是热点函数的回调处理器，用于触发 JIT 编译。
//
// 此方法由热点检测器在检测到函数达到"热点"阈值时调用。
// 它负责决定是否对函数进行 JIT 编译，以及执行编译过程。
//
// # JIT 编译决策流程
//
//  1. 检查 JIT 是否启用
//  2. 验证函数是否有效
//  3. 咨询 profiler 是否应该编译（可能已编译或正在编译）
//  4. 检查函数是否可以被 JIT（某些复杂指令不支持）
//  5. 执行编译
//  6. 更新编译状态
//
// # 不可编译的情况
//
// 以下类型的函数不能被 JIT 编译：
//   - 包含异常处理的函数
//   - 包含对象操作的函数
//   - 包含闭包操作的函数
//   - 包含全局变量访问的函数
//
// 对于不可编译的函数，会标记 3 次失败，以确保永远不再尝试编译。
//
// # 参数
//
//   - profile: 函数的运行时 profile 信息，包含调用次数等统计
func (vm *VM) onJITFunctionHot(profile *jit.FunctionProfile) {
	// 前置条件检查：JIT 必须启用
	if !vm.jitEnabled || vm.jitCompiler == nil {
		return
	}

	fn := profile.Function
	if fn == nil {
		return
	}

	// 检查是否应该编译
	// profiler 会追踪函数的编译状态，避免重复编译
	profiler := vm.jitCompiler.GetProfiler()
	if profiler != nil && !profiler.ShouldCompile(fn) {
		return
	}

	// 检查函数是否可以被 JIT 编译
	// 某些复杂操作（异常处理、对象操作等）目前不支持 JIT
	if !jit.CanJIT(fn) {
		// 不可编译的函数，标记失败以避免再次尝试
		// 标记 3 次是为了确保 profiler 的失败计数达到阈值，
		// 使得此函数永远不会再被考虑编译
		if profiler != nil {
			profiler.MarkCompileFailed(fn)
			profiler.MarkCompileFailed(fn)
			profiler.MarkCompileFailed(fn) // 标记3次，永远不再尝试
		}
		return
	}

	// 执行 JIT 编译
	// 编译器将字节码转换为本机代码
	compiled, err := vm.jitCompiler.Compile(fn)
	if err != nil {
		// 编译失败（可能是资源不足或内部错误）
		if profiler != nil {
			profiler.MarkCompileFailed(fn)
		}
		return
	}

	// 编译成功，标记函数为已编译状态
	if compiled != nil {
		if profiler != nil {
			profiler.MarkCompiled(fn)
		}
	}
}

// GetJITCompiler 获取 JIT 编译器实例。
//
// 返回值可能为 nil（如果 JIT 被禁用或平台不支持）。
// 可用于获取编译统计信息、手动触发编译等高级操作。
//
// # 返回值
//
// JIT 编译器实例，或 nil。
func (vm *VM) GetJITCompiler() *jit.Compiler {
	return vm.jitCompiler
}

// IsJITEnabled 检查 JIT 功能是否启用并可用。
//
// 返回 true 需要满足以下所有条件：
//   - jitEnabled 标志为 true
//   - jitCompiler 实例存在
//   - 编译器本身报告为启用状态
//
// # 返回值
//
// true 表示 JIT 功能完全可用，可以编译和执行本机代码。
func (vm *VM) IsJITEnabled() bool {
	return vm.jitEnabled && vm.jitCompiler != nil && vm.jitCompiler.IsEnabled()
}

// SetJITEnabled 动态启用或禁用 JIT 功能。
//
// # 启用 JIT
//
// 如果当前 JIT 被禁用且 jitCompiler 为 nil，此方法会：
//  1. 创建新的 JIT 编译器实例
//  2. 注册热点回调函数
//
// 注意：之前编译的代码不会被恢复，需要重新达到热点阈值。
//
// # 禁用 JIT
//
// 禁用时仅设置 jitEnabled 标志为 false：
//   - 已编译的代码会被保留（但不会被使用）
//   - 热点统计会继续（但不会触发编译）
//   - 所有执行将使用解释器
//
// # 参数
//
//   - enabled: true 启用 JIT，false 禁用 JIT
//
// # 使用场景
//
//   - 调试时临时禁用 JIT 以确保执行路径一致
//   - 性能测试对比 JIT 和解释执行
//   - 运行时检测到 JIT 问题后回退到解释执行
func (vm *VM) SetJITEnabled(enabled bool) {
	if enabled && vm.jitCompiler == nil {
		// 需要创建新的编译器
		vm.jitCompiler = jit.NewCompiler(nil)
		if vm.jitCompiler != nil && vm.jitCompiler.IsEnabled() {
			vm.jitEnabled = true
			// 重新注册热点回调
			profiler := vm.jitCompiler.GetProfiler()
			if profiler != nil {
				profiler.OnFunctionHot(vm.onJITFunctionHot)
			}
		}
	} else if !enabled {
		// 简单地禁用 JIT，保留编译器实例
		vm.jitEnabled = false
	}
}

// ============================================================================
// 垃圾回收相关方法
// ============================================================================

// GetGC 获取垃圾回收器实例。
//
// 返回 VM 使用的 GC 实例，可用于：
//   - 获取 GC 统计信息（如回收次数、堆大小）
//   - 手动配置 GC 参数
//   - 调试内存管理问题
//
// # 返回值
//
// GC 实例指针。此指针在 VM 生命周期内保持有效。
func (vm *VM) GetGC() *GC {
	return vm.gc
}

// SetGCEnabled 启用或禁用自动垃圾回收。
//
// 禁用 GC 时：
//   - 不会自动触发垃圾回收
//   - 内存会持续增长直到程序结束
//   - 可能导致内存耗尽
//
// # 使用场景
//
//   - 临时禁用：执行对延迟敏感的代码段
//   - 调试：排查 GC 相关问题
//   - 基准测试：测量纯执行性能
//
// # 参数
//
//   - enabled: true 启用 GC，false 禁用 GC
func (vm *VM) SetGCEnabled(enabled bool) {
	vm.gc.SetEnabled(enabled)
}

// SetGCDebug 设置 GC 调试模式。
//
// 启用调试模式后，GC 会输出详细的回收信息：
//   - 每次回收的触发原因
//   - 回收前后的堆大小
//   - 回收的对象数量
//   - 标记和清除阶段的耗时
//
// # 参数
//
//   - debug: true 启用调试输出，false 禁用
func (vm *VM) SetGCDebug(debug bool) {
	vm.gc.SetDebug(debug)
}

// ============================================================================
// 内联缓存相关方法
// ============================================================================

// GetICManager 获取内联缓存管理器实例。
//
// 内联缓存（Inline Cache）是一种方法调用优化技术：
//   - 缓存方法调用的目标类型和方法地址
//   - 避免每次调用都进行方法查找
//   - 特别适合单态调用点（总是调用同一类型的方法）
//
// # 返回值
//
// ICManager 实例指针。
func (vm *VM) GetICManager() *ICManager {
	return vm.icManager
}

// SetICEnabled 启用或禁用内联缓存优化。
//
// 内联缓存对方法密集的代码有显著性能提升，
// 但会占用额外内存来存储缓存条目。
//
// # 参数
//
//   - enabled: true 启用内联缓存，false 禁用
func (vm *VM) SetICEnabled(enabled bool) {
	vm.icManager.SetEnabled(enabled)
}

// ============================================================================
// 热点检测相关方法
// ============================================================================

// GetHotspotDetector 获取热点检测器实例。
//
// 热点检测器负责识别"热点"代码：
//   - 追踪函数调用次数
//   - 追踪循环迭代次数
//   - 当计数超过阈值时标记为热点
//   - 触发 JIT 编译决策
//
// # 返回值
//
// HotspotDetector 实例指针。
func (vm *VM) GetHotspotDetector() *HotspotDetector {
	return vm.hotspotDetector
}

// GetScheduler 获取协程调度器
//
// 调度器管理所有协程的创建、调度和销毁。
//
// # 返回值
//
// Scheduler 实例指针。
func (vm *VM) GetScheduler() *Scheduler {
	return vm.scheduler
}

// SetHotspotEnabled 启用或禁用热点检测。
//
// 禁用热点检测后：
//   - 不会追踪函数调用和循环次数
//   - 不会触发 JIT 编译
//   - 所有代码将始终使用解释执行
//
// # 参数
//
//   - enabled: true 启用热点检测，false 禁用
func (vm *VM) SetHotspotEnabled(enabled bool) {
	vm.hotspotDetector.SetEnabled(enabled)
}

// SetGCThreshold 设置 GC 触发阈值。
//
// 阈值定义了堆上对象数量达到多少时触发自动 GC。
// 较低的阈值会导致更频繁的 GC（更低的内存占用，更高的 CPU 开销）。
// 较高的阈值会减少 GC 频率（更高的内存占用，更低的 CPU 开销）。
//
// # 参数
//
//   - threshold: 触发 GC 的对象数量阈值
//
// # 调优建议
//
//   - 内存受限环境：使用较低阈值（如 100-500）
//   - 性能优先场景：使用较高阈值（如 10000+）
//   - 默认值通常适合大多数场景
func (vm *VM) SetGCThreshold(threshold int) {
	vm.gc.SetThreshold(threshold)
}

// CollectGarbage 手动触发一次完整的垃圾回收。
//
// 此方法会立即执行 GC，无论当前堆大小是否达到阈值。
// 回收过程包括：
//  1. 收集所有 GC 根对象
//  2. 从根对象开始标记所有可达对象
//  3. 清除所有不可达对象
//
// # 返回值
//
// 返回本次回收释放的对象数量。
//
// # 使用场景
//
//   - 在内存敏感操作前释放内存
//   - 程序空闲期间主动回收
//   - 调试和测试 GC 行为
func (vm *VM) CollectGarbage() int {
	roots := vm.collectRoots()
	return vm.gc.Collect(roots)
}

// collectRoots 收集所有 GC 根对象。
//
// GC 根是存活对象的起点，不从任何根可达的对象将被回收。
// 根对象包括：
//
// 1. 操作数栈上的值
//   - 当前正在计算的中间结果
//   - 函数参数和局部变量
//
// 2. 全局变量
//   - 所有通过 DefineGlobal 定义的变量
//
// 3. 调用帧中的闭包
//   - 当前调用链上的所有函数
//   - 闭包捕获的 upvalue
//
// # 根对象的重要性
//
// 根对象直接或间接引用的所有对象都被认为是"存活"的。
// 例如，如果栈上有一个数组，数组中的所有元素也是存活的。
//
// # 返回值
//
// 返回所有根对象的包装器切片，供 GC 进行标记。
func (vm *VM) collectRoots() []GCObject {
	var roots []GCObject

	// 1. 栈上的值
	// 遍历操作数栈，收集所有引用类型的值
	for i := 0; i < vm.stackTop; i++ {
		if w := vm.gc.GetWrapper(vm.stack[i]); w != nil {
			roots = append(roots, w)
		}
	}

	// 2. 全局变量
	// 遍历全局变量表，收集所有引用类型的值
	for _, v := range vm.globals {
		if w := vm.gc.GetWrapper(v); w != nil {
			roots = append(roots, w)
		}
	}

	// 3. 调用帧中的闭包
	// 遍历调用栈，收集所有活动的闭包
	for i := 0; i < vm.frameCount; i++ {
		closure := vm.frames[i].Closure
		if closure != nil {
			if w := vm.gc.GetWrapper(bytecode.NewClosure(closure)); w != nil {
				roots = append(roots, w)
			}
		}
	}

	return roots
}

// trackAllocation 用于在创建/返回堆对象时执行“分配追踪”。
//
// VM 中所有可能产生堆分配的指令（如创建对象、数组、Map、迭代器等）
// 都应在将值压入栈之前调用此方法（或等价逻辑），以便 GC 能正确追踪对象。
//
// 行为：
//   - 将 value 注册到 GC（仅引用类型会产生 wrapper）
//   - 如 GC 判定需要回收，则收集根并触发一次回收
//
// 设计注意：
//   - 这里是“慢路径”，用于少量但关键的分配点；
//   - 解释器的热路径（循环体等）通常使用 maybeGC 降低开销。
func (vm *VM) trackAllocation(v bytecode.Value) bytecode.Value {
	w := vm.gc.TrackValue(v)
	if w != nil && vm.gc.ShouldCollect() {
		roots := vm.collectRoots()
		vm.gc.Collect(roots)
	}
	return v
}

// maybeGC 是用于热点路径的轻量 GC 检查。
//
// 与 trackAllocation 的差异：
//   - maybeGC 不追踪具体某个分配，只是在“时间片/指令计数”维度上检查阈值；
//   - 适合放在执行循环中按固定间隔触发，以避免每次分配都做阈值判断。
//
// 触发策略由 vm.execute() 控制（例如每 N 条指令检查一次）。
func (vm *VM) maybeGC() {
	if vm.gc.ShouldCollect() {
		roots := vm.collectRoots()
		vm.gc.Collect(roots)
	}
}

// ============================================================================
// 字节码执行入口
// ============================================================================

// Run 执行一个“顶层函数”（通常是脚本/模块的入口函数）的字节码。
//
// 约定：
//   - fn 必须已经完成编译，且 fn.Chunk 里包含可执行字节码与常量表；
//   - VM 会为它创建一个顶层闭包并压入栈；
//   - 随后创建第 0 号调用帧，进入主解释循环 execute()。
//
// 栈状态（进入 execute 前）：
//   - stack[0] = 顶层闭包（作为 slot0，便于递归/自引用）
//   - stackTop = 1
//
// 注意：
//   - Run 不会清空 VM 的 globals/classes/enums；它们被视为“进程级/运行时级”环境。
//   - 若希望复用 VM 多次运行脚本，调用方需自行决定是否重置 globals 等状态。
func (vm *VM) Run(fn *bytecode.Function) InterpretResult {
	// 创建顶层闭包
	closure := &bytecode.Closure{Function: fn}
	
	// 压入闭包
	vm.push(vm.trackAllocation(bytecode.NewClosure(closure)))
	
	// 创建调用帧
	vm.frames[0] = CallFrame{
		Closure:  closure,
		IP:       0,
		BaseSlot: 0,
	}
	vm.frameCount = 1

	return vm.execute()
}

// ============================================================================
// 主解释循环（Fetch-Decode-Execute）
// ============================================================================

// execute 是虚拟机的主解释循环。
//
// 该循环负责：
//   - 维护当前活动帧 frame 与其字节码 chunk 的引用；
//   - 不断 fetch（取指）-> decode（解码）-> execute（执行）；
//   - 在函数调用/返回、异常处理导致 frame 变化时刷新 frame/chunk；
//   - 在热点路径中按策略触发 GC；
//   - 在必要时进行安全保护（IP 越界、指令数上限、内存增长过快等）。
//
// 维护者须知：
//   - 任何可能改变当前 frame 的操作（如 OpCall/OpReturn/异常跳转）后，
//     都需要刷新本地变量 frame/chunk，否则会继续在旧 chunk 上读指令。
//   - 对于会“吞掉异常并跳转到 catch/finally”的路径，本实现用
//     InterpretExceptionHandled 作为信号让外层刷新 frame/chunk 并 continue。
func (vm *VM) execute() InterpretResult {
	frame := &vm.frames[vm.frameCount-1]
	chunk := frame.Closure.Function.Chunk

	// ------------------------------------------------------------------------
	// 安全保护：指令数上限（用于防止非预期的无限循环）
	// ------------------------------------------------------------------------
	maxInstructions := 500000000 // 5亿条指令上限（用于性能测试）
	instructionCount := 0
	
	// ------------------------------------------------------------------------
	// GC 策略：按固定间隔检查是否需要回收
	// ------------------------------------------------------------------------
	// 每执行 gcCheckInterval 条指令检查一次阈值。
	// 这样能避免“每次分配都判断”的开销，同时也能在长循环中及时回收。
	const gcCheckInterval = 100
	gcCheckCounter := 0
	
	// ------------------------------------------------------------------------
	// 内存增长保护：若堆对象数量短时间内剧增，则强制触发 GC
	// ------------------------------------------------------------------------
	// 这是一个粗粒度的“分配速率”保护，主要防止某些路径疯狂分配导致 OOM。
	const memoryCheckInterval = 50 // 每 50 条指令检查一次内存分配速率
	memoryCheckCounter := 0
	lastGCHeapSize := vm.gc.HeapSize()

	for {
		// --------------------------------------------------------------------
		// 安全检查：IP 越界
		// --------------------------------------------------------------------
		// 如果 IP 指向 chunk 末尾之外，说明字节码或跳转偏移有问题，
		// 或者 frame/chunk 未正确刷新。
		if frame.IP >= len(chunk.Code) {
			return vm.runtimeError(i18n.T(i18n.ErrIPOutOfBounds))
		}

		// --------------------------------------------------------------------
		// 安全检查：指令计数上限
		// --------------------------------------------------------------------
		// 用于防止死循环锁死解释器。该上限主要服务于性能测试与安全场景，
		// 正常生产配置可根据需求调节。
		instructionCount++
		if instructionCount > maxInstructions {
			return vm.runtimeError(i18n.T(i18n.ErrExecutionLimit))
		}
		
		// 周期性 GC 检查（轻量路径）
		gcCheckCounter++
		if gcCheckCounter >= gcCheckInterval {
			gcCheckCounter = 0
			vm.maybeGC()
		}
		
		// 内存增长保护：如果堆大小快速增长，强制触发 GC（更激进的慢路径）
		memoryCheckCounter++
		if memoryCheckCounter >= memoryCheckInterval {
			memoryCheckCounter = 0
			currentHeapSize := vm.gc.HeapSize()
			// 如果堆大小在短时间内增长超过 100 个对象，强制触发 GC
			if currentHeapSize > lastGCHeapSize+100 {
				roots := vm.collectRoots()
				vm.gc.Collect(roots)
				lastGCHeapSize = vm.gc.HeapSize()
			} else {
				lastGCHeapSize = currentHeapSize
			}
		}

		// --------------------------------------------------------------------
		// Fetch: 读取下一条指令并推进 IP
		// --------------------------------------------------------------------
		instruction := bytecode.OpCode(chunk.Code[frame.IP])
		frame.IP++

		switch instruction {
		// ----------------------------------------------------------------
		// 栈/常量加载：push/pop/dup/swap，以及常用常量
		// ----------------------------------------------------------------
		case bytecode.OpPush:
			constant := chunk.ReadU16(frame.IP)
			frame.IP += 2
			vm.push(chunk.Constants[constant])

		case bytecode.OpPop:
			vm.pop()

		case bytecode.OpDup:
			vm.push(vm.peek(0))

		case bytecode.OpSwap:
			a := vm.pop()
			b := vm.pop()
			vm.push(a)
			vm.push(b)

		case bytecode.OpNull:
			vm.push(bytecode.NullValue)

		case bytecode.OpTrue:
			vm.push(bytecode.TrueValue)

		case bytecode.OpFalse:
			vm.push(bytecode.FalseValue)

		case bytecode.OpZero:
			vm.push(bytecode.ZeroValue)

		case bytecode.OpOne:
			vm.push(bytecode.OneValue)

		// ----------------------------------------------------------------
		// 局部/全局变量访问
		// ----------------------------------------------------------------
		case bytecode.OpLoadLocal:
			slot := chunk.ReadU16(frame.IP)
			frame.IP += 2
			vm.push(vm.stack[frame.BaseSlot+int(slot)])

		case bytecode.OpStoreLocal:
			slot := chunk.ReadU16(frame.IP)
			frame.IP += 2
			vm.stack[frame.BaseSlot+int(slot)] = vm.peek(0)

		case bytecode.OpLoadGlobal:
			nameIdx := chunk.ReadU16(frame.IP)
			frame.IP += 2
			name := chunk.Constants[nameIdx].AsString()
			if value, ok := vm.globals[name]; ok {
				vm.push(value)
			} else {
				return vm.runtimeError(i18n.T(i18n.ErrUndefinedVar, name))
			}

		case bytecode.OpStoreGlobal:
			nameIdx := chunk.ReadU16(frame.IP)
			frame.IP += 2
			name := chunk.Constants[nameIdx].AsString()
			vm.globals[name] = vm.peek(0)

		// ----------------------------------------------------------------
		// 算术运算（可能触发异常：如除零、类型错误）
		// ----------------------------------------------------------------
		case bytecode.OpAdd:
			if result := vm.binaryOp(instruction); result != InterpretOK {
				if result == InterpretExceptionHandled {
					frame = &vm.frames[vm.frameCount-1]
					chunk = frame.Closure.Function.Chunk
					continue
				}
				return result
			}

		case bytecode.OpSub:
			if result := vm.binaryOp(instruction); result != InterpretOK {
				if result == InterpretExceptionHandled {
					frame = &vm.frames[vm.frameCount-1]
					chunk = frame.Closure.Function.Chunk
					continue
				}
				return result
			}

		case bytecode.OpMul:
			if result := vm.binaryOp(instruction); result != InterpretOK {
				if result == InterpretExceptionHandled {
					frame = &vm.frames[vm.frameCount-1]
					chunk = frame.Closure.Function.Chunk
					continue
				}
				return result
			}

		case bytecode.OpDiv:
			if result := vm.binaryOp(instruction); result != InterpretOK {
				if result == InterpretExceptionHandled {
					frame = &vm.frames[vm.frameCount-1]
					chunk = frame.Closure.Function.Chunk
					continue
				}
				return result
			}

		case bytecode.OpMod:
			if result := vm.binaryOp(instruction); result != InterpretOK {
				if result == InterpretExceptionHandled {
					frame = &vm.frames[vm.frameCount-1]
					chunk = frame.Closure.Function.Chunk
					continue
				}
				return result
			}

		case bytecode.OpNeg:
			v := vm.pop()
			switch v.Type {
			case bytecode.ValInt:
				vm.push(bytecode.NewInt(-v.AsInt()))
			case bytecode.ValFloat:
				vm.push(bytecode.NewFloat(-v.AsFloat()))
			default:
				return vm.runtimeError(i18n.T(i18n.ErrOperandMustBeNumber))
			}

		// ----------------------------------------------------------------
		// 比较运算
		// ----------------------------------------------------------------
		case bytecode.OpEq:
			b := vm.pop()
			a := vm.pop()
			vm.push(bytecode.NewBool(a.Equals(b)))

		case bytecode.OpNe:
			b := vm.pop()
			a := vm.pop()
			vm.push(bytecode.NewBool(!a.Equals(b)))

		case bytecode.OpLt:
			if result := vm.compareOp(instruction); result != InterpretOK {
				return result
			}

		case bytecode.OpLe:
			if result := vm.compareOp(instruction); result != InterpretOK {
				return result
			}

		case bytecode.OpGt:
			if result := vm.compareOp(instruction); result != InterpretOK {
				return result
			}

		case bytecode.OpGe:
			if result := vm.compareOp(instruction); result != InterpretOK {
				return result
			}

		// ----------------------------------------------------------------
		// 逻辑运算
		// ----------------------------------------------------------------
		case bytecode.OpNot:
			vm.push(bytecode.NewBool(!vm.pop().IsTruthy()))

		// ----------------------------------------------------------------
		// 位运算（按 int 语义）
		// ----------------------------------------------------------------
		case bytecode.OpBitAnd:
			b := vm.pop()
			a := vm.pop()
			vm.push(bytecode.NewInt(a.AsInt() & b.AsInt()))

		case bytecode.OpBitOr:
			b := vm.pop()
			a := vm.pop()
			vm.push(bytecode.NewInt(a.AsInt() | b.AsInt()))

		case bytecode.OpBitXor:
			b := vm.pop()
			a := vm.pop()
			vm.push(bytecode.NewInt(a.AsInt() ^ b.AsInt()))

		case bytecode.OpBitNot:
			a := vm.pop()
			vm.push(bytecode.NewInt(^a.AsInt()))

		case bytecode.OpShl:
			b := vm.pop()
			a := vm.pop()
			vm.push(bytecode.NewInt(a.AsInt() << uint(b.AsInt())))

		case bytecode.OpShr:
			b := vm.pop()
			a := vm.pop()
			vm.push(bytecode.NewInt(a.AsInt() >> uint(b.AsInt())))

		// ----------------------------------------------------------------
		// 字符串/字符串构建器
		// ----------------------------------------------------------------
		case bytecode.OpConcat:
			b := vm.pop()
			a := vm.pop()
			// 字符串在 Go 中是不可变的，直接创建新字符串
			result := bytecode.NewString(a.AsString() + b.AsString())
			vm.push(result)

		// 字符串构建器操作（用于高效多字符串拼接）
		case bytecode.OpStringBuilderNew:
			sb := bytecode.NewStringBuilder()
			vm.push(bytecode.NewStringBuilderValue(sb))

		case bytecode.OpStringBuilderAdd:
			value := vm.pop()
			sbVal := vm.pop()
			sb := sbVal.AsStringBuilder()
			if sb != nil {
				sb.AppendValue(value)
				vm.push(sbVal) // 返回构建器自身，支持链式调用
			} else {
				return vm.runtimeError("expected StringBuilder")
			}

		case bytecode.OpStringBuilderBuild:
			sbVal := vm.pop()
			sb := sbVal.AsStringBuilder()
			if sb != nil {
				result := sb.Build()
				vm.push(bytecode.NewString(result))
			} else {
				return vm.runtimeError("expected StringBuilder")
			}

		// ----------------------------------------------------------------
		// 控制流：跳转/条件跳转/循环回边
		// ----------------------------------------------------------------
		case bytecode.OpJump:
			offset := chunk.ReadI16(frame.IP)
			frame.IP += 2
			frame.IP += int(offset)

		case bytecode.OpJumpIfFalse:
			offset := chunk.ReadI16(frame.IP)
			frame.IP += 2
			if !vm.peek(0).IsTruthy() {
				frame.IP += int(offset)
			}

		case bytecode.OpJumpIfTrue:
			offset := chunk.ReadI16(frame.IP)
			frame.IP += 2
			if vm.peek(0).IsTruthy() {
				frame.IP += int(offset)
			}

		case bytecode.OpLoop:
			loopHeaderIP := frame.IP - 1 // 回边位置
			offset := chunk.ReadU16(frame.IP)
			frame.IP += 2
			targetIP := frame.IP - int(offset) // 循环头位置
			frame.IP = targetIP
			
			// 热点检测：记录循环迭代
			vm.hotspotDetector.RecordLoopIteration(frame.Closure.Function, targetIP, loopHeaderIP)

		// ----------------------------------------------------------------
		// 函数调用/尾调用/返回
		// ----------------------------------------------------------------
		case bytecode.OpCall:
			argCount := int(chunk.Code[frame.IP])
			frame.IP++
			if result := vm.callValue(vm.peek(argCount), argCount); result != InterpretOK {
				return result
			}
			frame = &vm.frames[vm.frameCount-1]
			chunk = frame.Closure.Function.Chunk

		case bytecode.OpTailCall:
			argCount := int(chunk.Code[frame.IP])
			frame.IP++
			if result := vm.tailCall(vm.peek(argCount), argCount); result != InterpretOK {
				return result
			}
			frame = &vm.frames[vm.frameCount-1]
			chunk = frame.Closure.Function.Chunk

		case bytecode.OpReturn:
			result := vm.pop()
			vm.frameCount--
			if vm.frameCount == 0 {
				vm.pop() // 弹出脚本函数
				return InterpretOK
			}
			vm.stackTop = frame.BaseSlot
			vm.push(result)
			frame = &vm.frames[vm.frameCount-1]
			chunk = frame.Closure.Function.Chunk

		case bytecode.OpClosure:
			upvalueCount := int(chunk.ReadU16(frame.IP))
			frame.IP += 2
			
			// 栈上：[function, upvalue1, upvalue2, ...]
			// 创建闭包并捕获 upvalues
			upvalues := make([]*bytecode.Upvalue, upvalueCount)
			for i := upvalueCount - 1; i >= 0; i-- {
				val := vm.pop()
				upvalues[i] = &bytecode.Upvalue{Closed: val, IsClosed: true}
			}
			fnVal := vm.pop()
			fn := fnVal.Data.(*bytecode.Function)
			closure := &bytecode.Closure{
				Function: fn,
				Upvalues: upvalues,
			}
			vm.push(vm.trackAllocation(bytecode.NewClosure(closure)))

		case bytecode.OpReturnNull:
			vm.frameCount--
			if vm.frameCount == 0 {
				// 程序结束，清理栈上的闭包（如果有）
				if vm.stackTop > 0 {
					vm.pop()
				}
				return InterpretOK
			}
			vm.stackTop = frame.BaseSlot
			vm.push(bytecode.NullValue)
			frame = &vm.frames[vm.frameCount-1]
			chunk = frame.Closure.Function.Chunk

		// ----------------------------------------------------------------
		// 对象/类/枚举/静态成员
		// ----------------------------------------------------------------
		case bytecode.OpNewObject:
			classIdx := chunk.ReadU16(frame.IP)
			frame.IP += 2
			className := chunk.Constants[classIdx].AsString()
			class, ok := vm.classes[className]
			if !ok {
				return vm.runtimeError(i18n.T(i18n.ErrUndefinedClass, className))
			}
			// 验证类约束（抽象类、接口实现）
			if err := vm.validateClass(class); err != nil {
				return vm.runtimeError("%v", err)
			}
			
			// 如果有泛型类型参数定义，检查是否有类型参数传入
			// 注意：由于类型擦除，运行时通常没有泛型类型参数信息
			// 这里主要是为未来的扩展做准备，当前实现基本验证框架
			var typeArgs []string
			if len(class.TypeParams) > 0 {
				// 如果有类型参数定义但运行时没有传入，这是正常的（类型擦除）
				// 未来如果需要运行时类型验证，可以在这里扩展
				// 目前仅验证编译时已有的类型信息
				// typeArgs 保持为 nil，表示没有运行时类型参数
			}
			
			var obj *bytecode.Object
			if typeArgs != nil {
				// 使用带类型参数的对象创建（如果有）
				obj = bytecode.NewObjectInstanceWithTypes(class, typeArgs)
				// 验证泛型约束
				if err := vm.validateGenericConstraints(typeArgs, class.TypeParams); err != nil {
					return vm.runtimeError("%v", err)
				}
			} else {
				// 普通对象创建
				obj = bytecode.NewObjectInstance(class)
			}
			
			// 初始化属性默认值
			vm.initObjectProperties(obj, class)
			vm.push(vm.trackAllocation(bytecode.NewObject(obj)))

		case bytecode.OpGetField:
			fieldIP := frame.IP - 1 // 保存字段访问点 IP（用于属性缓存）
			nameIdx := chunk.ReadU16(frame.IP)
			frame.IP += 2
			name := chunk.Constants[nameIdx].AsString()
			
			objVal := vm.pop()
			if objVal.Type != bytecode.ValObject {
				return vm.runtimeError(i18n.T(i18n.ErrOnlyObjectsHaveFields))
			}
			obj := objVal.AsObject()
			
			// 尝试使用属性缓存快速路径（仅用于直接字段访问，不包括 getter）
			useCache := false
			hasGetter := false
			if vm.icManager.IsEnabled() {
				pc := vm.icManager.GetPropertyCache(fieldIP)
				if pc != nil {
					if cached, cachedName := pc.Lookup(obj.Class, name); cached {
						if cachedName == name {
							// 缓存命中：直接字段访问
							useCache = true
							if err := vm.checkPropertyAccess(obj.Class, name); err != nil {
								return vm.runtimeError("%v", err)
							}
							if value, ok := obj.GetField(name); ok {
								vm.push(value)
							} else {
								vm.push(bytecode.NullValue)
							}
						}
					} else {
						// 缓存未命中，更新缓存（会在查找后更新）
					}
				}
			}
			
			if !useCache {
				// 检查是否有 getter 方法（属性访问器）
				getterName := "get_" + name
				if getter := vm.lookupMethod(obj.Class, getterName); getter != nil {
					hasGetter = true
					// 调用 getter 方法
					closure := &bytecode.Closure{
						Function: &bytecode.Function{
							Name:          getter.Name,
							ClassName:     getter.ClassName,
							SourceFile:    getter.SourceFile,
							Arity:         getter.Arity,
							MinArity:      getter.MinArity,
							Chunk:         getter.Chunk,
							LocalCount:    getter.LocalCount,
							DefaultValues: getter.DefaultValues,
						},
					}
					vm.push(objVal) // 将对象压回栈作为 receiver
					if result := vm.call(closure, 0); result != InterpretOK {
						return result
					}
					frame = &vm.frames[vm.frameCount-1]
					chunk = frame.Closure.Function.Chunk
					continue
				} else {
					// 普通字段访问
					// 检查访问权限
					if err := vm.checkPropertyAccess(obj.Class, name); err != nil {
						return vm.runtimeError("%v", err)
					}
					
					if value, ok := obj.GetField(name); ok {
						vm.push(value)
					} else {
						vm.push(bytecode.NullValue)
					}
					
					// 更新属性缓存（仅缓存直接字段访问，不缓存 getter）
					if vm.icManager.IsEnabled() && !hasGetter {
						pc := vm.icManager.GetPropertyCache(fieldIP)
						if pc != nil {
							pc.Update(obj.Class, name)
						}
					}
				}
			}

		case bytecode.OpSetField:
			fieldIP := frame.IP - 1 // 保存字段访问点 IP（用于属性缓存）
			nameIdx := chunk.ReadU16(frame.IP)
			frame.IP += 2
			name := chunk.Constants[nameIdx].AsString()
			
			// 栈布局: [value, object] -> 先 pop object，再 pop value
			objVal := vm.pop()
			value := vm.pop()
			if objVal.Type != bytecode.ValObject {
				return vm.runtimeError(i18n.T(i18n.ErrOnlyObjectsHaveFields))
			}
			obj := objVal.AsObject()
			
			// 尝试使用属性缓存快速路径（仅用于直接字段访问，不包括 setter）
			useCache := false
			hasSetter := false
			if vm.icManager.IsEnabled() {
				pc := vm.icManager.GetPropertyCache(fieldIP)
				if pc != nil {
					if cached, cachedName := pc.Lookup(obj.Class, name); cached {
						if cachedName == name {
							// 缓存命中：直接字段访问
							useCache = true
							// 检查访问权限
							if err := vm.checkPropertyAccess(obj.Class, name); err != nil {
								return vm.runtimeError("%v", err)
							}
							
							// 检查 final 属性 - 如果属性已有值且为 final，则不允许重新赋值
							// （第一次赋值在构造函数中是允许的）
							if obj.Class.PropFinal[name] {
								if _, exists := obj.GetField(name); exists {
									return vm.runtimeError(i18n.T(i18n.ErrCannotAssignFinalProperty, name))
								}
							}
							
							obj.SetField(name, value)
							
							// 写屏障：当老年代对象引用年轻代对象时，需要记录到记忆集
							vm.gc.WriteBarrierValue(objVal, value)
							vm.push(value)
						}
					}
				}
			}
			
			if !useCache {
				// 检查是否有 setter 方法（属性访问器）
				setterName := "set_" + name
				if setter := vm.lookupMethodByArity(obj.Class, setterName, 1); setter != nil {
					hasSetter = true
					// 调用 setter 方法
					closure := &bytecode.Closure{
						Function: &bytecode.Function{
							Name:          setter.Name,
							ClassName:     setter.ClassName,
							SourceFile:    setter.SourceFile,
							Arity:         setter.Arity,
							MinArity:      setter.MinArity,
							Chunk:         setter.Chunk,
							LocalCount:    setter.LocalCount,
							DefaultValues: setter.DefaultValues,
						},
					}
					vm.push(objVal) // 将对象压回栈作为 receiver
					vm.push(value)  // 将值压入栈作为参数
					if result := vm.call(closure, 1); result != InterpretOK {
						return result
					}
					frame = &vm.frames[vm.frameCount-1]
					chunk = frame.Closure.Function.Chunk
					// setter 可能返回 void，但我们需要返回设置的值
					vm.push(value)
				} else {
					// 普通字段访问
					// 检查访问权限
					if err := vm.checkPropertyAccess(obj.Class, name); err != nil {
						return vm.runtimeError("%v", err)
					}
					
					// 检查 final 属性 - 如果属性已有值且为 final，则不允许重新赋值
					// （第一次赋值在构造函数中是允许的）
					if obj.Class.PropFinal[name] {
						if _, exists := obj.GetField(name); exists {
							return vm.runtimeError(i18n.T(i18n.ErrCannotAssignFinalProperty, name))
						}
					}
					
					obj.SetField(name, value)
					
					// 写屏障：当老年代对象引用年轻代对象时，需要记录到记忆集
					vm.gc.WriteBarrierValue(objVal, value)
					vm.push(value)
					
					// 更新属性缓存（仅缓存直接字段访问，不缓存 setter）
					if vm.icManager.IsEnabled() && !hasSetter {
						pc := vm.icManager.GetPropertyCache(fieldIP)
						if pc != nil {
							pc.Update(obj.Class, name)
						}
					}
				}
			}

		case bytecode.OpCallMethod:
			callSiteIP := frame.IP - 1 // 保存调用点 IP（用于内联缓存）
			nameIdx := chunk.ReadU16(frame.IP)
			frame.IP += 2
			argCount := int(chunk.Code[frame.IP])
			frame.IP++
			name := chunk.Constants[nameIdx].AsString()
			
			// 特殊处理构造函数 - 如果不存在则跳过
			receiver := vm.peek(argCount)
			if receiver.Type == bytecode.ValObject {
				obj := receiver.AsObject()
				
				// 尝试内联缓存快速路径（验证参数数量匹配）
				if vm.icManager.IsEnabled() {
					ic := vm.icManager.GetMethodCache(frame.Closure.Function, callSiteIP)
					if ic != nil {
						if method, hit := ic.Lookup(obj.Class); hit {
							// 验证参数数量是否匹配（支持方法重载和默认参数）
							if argCount >= method.MinArity && argCount <= method.Arity {
								// 缓存命中且参数匹配：直接调用
								if result := vm.callMethodDirect(obj, method, argCount); result != InterpretOK {
									return result
								}
								frame = &vm.frames[vm.frameCount-1]
								chunk = frame.Closure.Function.Chunk
								continue
							}
							// 参数不匹配，清除缓存并重新查找
							ic.Reset()
						}
					}
				}
				
				// 快速检查构造函数是否存在（避免不必要的完整方法查找）
				if name == "__construct" {
					if methods, ok := obj.Class.Methods[name]; !ok || len(methods) == 0 {
						// 没有构造函数，跳过调用，只保留对象在栈上
						continue
					}
				}
			}
			
			// 使用 invokeMethod 进行完整的方法查找和调用（包含内联缓存更新）
			if result := vm.invokeMethod(name, argCount, callSiteIP); result != InterpretOK {
				return result
			}
			frame = &vm.frames[vm.frameCount-1]
			chunk = frame.Closure.Function.Chunk

		// 静态成员访问
		case bytecode.OpGetStatic:
			classIdx := chunk.ReadU16(frame.IP)
			frame.IP += 2
			nameIdx := chunk.ReadU16(frame.IP)
			frame.IP += 2
			className := chunk.Constants[classIdx].AsString()
			name := chunk.Constants[nameIdx].AsString()
			
			// 先检查是否是枚举
			if enum, ok := vm.enums[className]; ok {
				if val, ok := enum.Cases[name]; ok {
					vm.push(bytecode.NewEnumValue(className, name, val))
					continue
				}
				return vm.runtimeError(i18n.T(i18n.ErrUndefinedEnumCase, className, name))
			}
			
			// 检查是否有静态属性 getter
			class, err := vm.resolveClassName(className)
			if err != nil {
				return vm.runtimeError("%v", err)
			}
			
			getterName := "get_" + name
			if getter := vm.lookupMethod(class, getterName); getter != nil && getter.IsStatic {
				// 调用静态 getter 方法
				closure := &bytecode.Closure{
					Function: &bytecode.Function{
						Name:          getter.Name,
						ClassName:     getter.ClassName,
						SourceFile:    getter.SourceFile,
						Arity:         getter.Arity,
						MinArity:      getter.MinArity,
						Chunk:         getter.Chunk,
						LocalCount:    getter.LocalCount,
						DefaultValues: getter.DefaultValues,
					},
				}
				vm.push(bytecode.NullValue) // 静态方法使用 null 作为占位符
				if result := vm.call(closure, 0); result != InterpretOK {
					return result
				}
				frame = &vm.frames[vm.frameCount-1]
				chunk = frame.Closure.Function.Chunk
				continue
			}
			
			// 先尝试常量
			if val, ok := vm.lookupConstant(class, name); ok {
				vm.push(val)
			} else if val, ok := vm.lookupStaticVar(class, name); ok {
				// 再尝试静态变量
				vm.push(val)
			} else {
				vm.push(bytecode.NullValue)
			}

		case bytecode.OpSetStatic:
			classIdx := chunk.ReadU16(frame.IP)
			frame.IP += 2
			nameIdx := chunk.ReadU16(frame.IP)
			frame.IP += 2
			className := chunk.Constants[classIdx].AsString()
			name := chunk.Constants[nameIdx].AsString()
			value := vm.pop()
			
			class, err := vm.resolveClassName(className)
			if err != nil {
				return vm.runtimeError("%v", err)
			}
			
			// 检查是否有静态属性 setter
			setterName := "set_" + name
			if setter := vm.lookupMethodByArity(class, setterName, 1); setter != nil && setter.IsStatic {
				// 调用静态 setter 方法
				closure := &bytecode.Closure{
					Function: &bytecode.Function{
						Name:          setter.Name,
						ClassName:     setter.ClassName,
						SourceFile:    setter.SourceFile,
						Arity:         setter.Arity,
						MinArity:      setter.MinArity,
						Chunk:         setter.Chunk,
						LocalCount:    setter.LocalCount,
						DefaultValues: setter.DefaultValues,
					},
				}
				vm.push(bytecode.NullValue) // 静态方法使用 null 作为占位符
				vm.push(value)              // 将值压入栈作为参数
				if result := vm.call(closure, 1); result != InterpretOK {
					return result
				}
				frame = &vm.frames[vm.frameCount-1]
				chunk = frame.Closure.Function.Chunk
				vm.push(value) // setter 可能返回 void，但我们需要返回设置的值
			} else {
				// 普通静态字段访问
				vm.setStaticVar(class, name, value)
				vm.push(value)
			}

		case bytecode.OpCallStatic:
			classIdx := chunk.ReadU16(frame.IP)
			frame.IP += 2
			nameIdx := chunk.ReadU16(frame.IP)
			frame.IP += 2
			argCount := int(chunk.Code[frame.IP])
			frame.IP++
			className := chunk.Constants[classIdx].AsString()
			methodName := chunk.Constants[nameIdx].AsString()
			
			class, err := vm.resolveClassName(className)
			if err != nil {
				return vm.runtimeError("%v", err)
			}
			
			method := vm.lookupMethodByArity(class, methodName, argCount)
			if method == nil {
				return vm.runtimeError(i18n.T(i18n.ErrUndefinedStaticMethod, class.Name, methodName, argCount))
			}
			
			// 创建方法的闭包并调用
			closure := &bytecode.Closure{
				Function: &bytecode.Function{
					Name:          method.Name,
					ClassName:    method.ClassName, // 设置类名用于堆栈跟踪
					SourceFile:   method.SourceFile,
					Arity:        method.Arity,
					MinArity:     method.MinArity, // 支持默认参数
					Chunk:        method.Chunk,
					LocalCount:   method.LocalCount,
					DefaultValues: method.DefaultValues, // 包含默认参数值
				},
			}
			
			// 保存原始参数（使用对象池减少分配）
			args := vm.gc.GetArgsFromPool(argCount)
			for i := argCount - 1; i >= 0; i-- {
				args[i] = vm.pop()
			}
			
			// 对于 parent:: 和 self:: 调用非静态方法，需要传递当前的 $this
			// 对于真正的静态方法调用，使用 null
			if (className == "parent" || className == "self") && !method.IsStatic {
				// 传递当前的 $this
				thisValue := vm.stack[frame.BaseSlot]
				vm.push(thisValue)
			} else {
				// 静态方法，使用 null 作为占位符
				vm.push(bytecode.NullValue)
			}
			
			// 重新压入参数
			for i := 0; i < argCount; i++ {
				vm.push(args[i])
			}
			
			// 归还参数数组到池
			vm.gc.ReturnArgsToPool(args)
			
			if result := vm.callOptimized(closure, argCount); result != InterpretOK {
				return result
			}
			frame = &vm.frames[vm.frameCount-1]
			chunk = frame.Closure.Function.Chunk

		// ----------------------------------------------------------------
		// 数组/Map/SuperArray/迭代器/Bytes 等容器类型
		// ----------------------------------------------------------------
		case bytecode.OpNewArray:
			length := int(chunk.ReadU16(frame.IP))
			frame.IP += 2
			arr := make([]bytecode.Value, length)
			for i := length - 1; i >= 0; i-- {
				arr[i] = vm.pop()
			}
			vm.push(vm.trackAllocation(bytecode.NewArray(arr)))

		case bytecode.OpNewFixedArray:
			capacity := int(chunk.ReadU16(frame.IP))
			frame.IP += 2
			initLength := int(chunk.ReadU16(frame.IP))
			frame.IP += 2
			elements := make([]bytecode.Value, initLength)
			for i := initLength - 1; i >= 0; i-- {
				elements[i] = vm.pop()
			}
			vm.push(vm.trackAllocation(bytecode.NewFixedArrayWithElements(elements, capacity)))

		case bytecode.OpArrayGet:
			idx := vm.pop()
			arrVal := vm.pop()
			
			switch arrVal.Type {
			case bytecode.ValArray:
				arr := arrVal.AsArray()
				i := int(idx.AsInt())
				if i < 0 || i >= len(arr) {
					if result := vm.throwTypedException("ArrayIndexOutOfBoundsException", i18n.T(i18n.ErrArrayIndexSimple)); result == InterpretExceptionHandled {
						frame = &vm.frames[vm.frameCount-1]
						chunk = frame.Closure.Function.Chunk
						continue
					} else {
						return result
					}
				}
				vm.push(arr[i])
			case bytecode.ValFixedArray:
				fa := arrVal.AsFixedArray()
				i := int(idx.AsInt())
				if i < 0 || i >= fa.Capacity {
					if result := vm.throwTypedException("ArrayIndexOutOfBoundsException", i18n.T(i18n.ErrArrayIndexOutOfBounds, i, fa.Capacity)); result == InterpretExceptionHandled {
						frame = &vm.frames[vm.frameCount-1]
						chunk = frame.Closure.Function.Chunk
						continue
					} else {
						return result
					}
				}
				vm.push(fa.Elements[i])
			case bytecode.ValMap:
				// Map 索引支持
				m := arrVal.AsMap()
				if value, ok := m[idx]; ok {
					vm.push(value)
				} else {
					vm.push(bytecode.NullValue)
				}
			case bytecode.ValSuperArray:
				// SuperArray 索引支持
				sa := arrVal.AsSuperArray()
				if value, ok := sa.Get(idx); ok {
					vm.push(value)
				} else {
					vm.push(bytecode.NullValue)
				}
			default:
				return vm.runtimeError(i18n.T(i18n.ErrSubscriptRequiresArray))
			}

		case bytecode.OpArraySet:
			value := vm.pop()
			idx := vm.pop()
			arrVal := vm.pop()
			
			switch arrVal.Type {
			case bytecode.ValArray:
				arr := arrVal.AsArray()
				i := int(idx.AsInt())
				if i < 0 || i >= len(arr) {
					if result := vm.throwTypedException("ArrayIndexOutOfBoundsException", i18n.T(i18n.ErrArrayIndexSimple)); result == InterpretExceptionHandled {
						frame = &vm.frames[vm.frameCount-1]
						chunk = frame.Closure.Function.Chunk
						continue
					} else {
						return result
					}
				}
				arr[i] = value
				// 写屏障
				vm.gc.WriteBarrierValue(arrVal, value)
			case bytecode.ValFixedArray:
				fa := arrVal.AsFixedArray()
				i := int(idx.AsInt())
				if i < 0 || i >= fa.Capacity {
					if result := vm.throwTypedException("ArrayIndexOutOfBoundsException", i18n.T(i18n.ErrArrayIndexOutOfBounds, i, fa.Capacity)); result == InterpretExceptionHandled {
						frame = &vm.frames[vm.frameCount-1]
						chunk = frame.Closure.Function.Chunk
						continue
					} else {
						return result
					}
				}
				fa.Elements[i] = value
				// 写屏障
				vm.gc.WriteBarrierValue(arrVal, value)
			case bytecode.ValMap:
				// Map 设置
				m := arrVal.AsMap()
				m[idx] = value
				// 写屏障
				vm.gc.WriteBarrierValue(arrVal, value)
			case bytecode.ValSuperArray:
				// SuperArray 设置
				sa := arrVal.AsSuperArray()
				sa.Set(idx, value)
				// 写屏障
				vm.gc.WriteBarrierValue(arrVal, value)
			default:
				return vm.runtimeError(i18n.T(i18n.ErrSubscriptRequiresArray))
			}
			vm.push(value)

		case bytecode.OpArrayLen:
			arrVal := vm.pop()
			switch arrVal.Type {
			case bytecode.ValArray:
				vm.push(bytecode.NewInt(int64(len(arrVal.AsArray()))))
			case bytecode.ValFixedArray:
				vm.push(bytecode.NewInt(int64(arrVal.AsFixedArray().Capacity)))
			case bytecode.ValSuperArray:
				vm.push(bytecode.NewInt(int64(arrVal.AsSuperArray().Len())))
			default:
				return vm.runtimeError(i18n.T(i18n.ErrLengthRequiresArray))
			}

		// 无边界检查的数组访问（用于循环优化，边界检查已在循环外完成）
		case bytecode.OpArrayGetUnchecked:
			idx := vm.pop()
			arrVal := vm.pop()
			switch arrVal.Type {
			case bytecode.ValArray:
				arr := arrVal.AsArray()
				i := int(idx.AsInt())
				vm.push(arr[i])
			case bytecode.ValFixedArray:
				fa := arrVal.AsFixedArray()
				i := int(idx.AsInt())
				vm.push(fa.Elements[i])
			default:
				return vm.runtimeError(i18n.T(i18n.ErrSubscriptRequiresArray))
			}

		case bytecode.OpArraySetUnchecked:
			value := vm.pop()
			idx := vm.pop()
			arrVal := vm.pop()
			switch arrVal.Type {
			case bytecode.ValArray:
				arr := arrVal.AsArray()
				i := int(idx.AsInt())
				arr[i] = value
			case bytecode.ValFixedArray:
				fa := arrVal.AsFixedArray()
				i := int(idx.AsInt())
				fa.Elements[i] = value
			default:
				return vm.runtimeError(i18n.T(i18n.ErrSubscriptRequiresArray))
			}
			vm.push(value)

		// Map 操作
		case bytecode.OpNewMap:
			size := int(chunk.ReadU16(frame.IP))
			frame.IP += 2
			m := make(map[bytecode.Value]bytecode.Value)
			for i := 0; i < size; i++ {
				value := vm.pop()
				key := vm.pop()
				m[key] = value
			}
			vm.push(vm.trackAllocation(bytecode.NewMap(m)))

		case bytecode.OpMapGet:
			key := vm.pop()
			mapVal := vm.pop()
			if mapVal.Type != bytecode.ValMap {
				return vm.runtimeError(i18n.T(i18n.ErrSubscriptRequiresMap))
			}
			m := mapVal.AsMap()
			if value, ok := m[key]; ok {
				vm.push(value)
			} else {
				vm.push(bytecode.NullValue)
			}

		case bytecode.OpMapSet:
			value := vm.pop()
			key := vm.pop()
			mapVal := vm.pop()
			if mapVal.Type != bytecode.ValMap {
				return vm.runtimeError(i18n.T(i18n.ErrSubscriptRequiresMap))
			}
			m := mapVal.AsMap()
			m[key] = value
			vm.push(value)

		case bytecode.OpMapHas:
			key := vm.pop()
			container := vm.pop()
			switch container.Type {
			case bytecode.ValMap:
				m := container.AsMap()
				_, ok := m[key]
				vm.push(bytecode.NewBool(ok))
			case bytecode.ValArray:
				arr := container.AsArray()
				idx := int(key.AsInt())
				vm.push(bytecode.NewBool(idx >= 0 && idx < len(arr)))
			case bytecode.ValSuperArray:
				sa := container.AsSuperArray()
				vm.push(bytecode.NewBool(sa.HasKey(key)))
			default:
				return vm.runtimeError(i18n.T(i18n.ErrHasRequiresArrayOrMap))
			}

		case bytecode.OpMapLen:
			mapVal := vm.pop()
			if mapVal.Type != bytecode.ValMap {
				return vm.runtimeError(i18n.T(i18n.ErrLengthRequiresMap))
			}
			vm.push(bytecode.NewInt(int64(len(mapVal.AsMap()))))

		// SuperArray 万能数组操作
		case bytecode.OpSuperArrayNew:
			count := int(chunk.ReadU16(frame.IP))
			frame.IP += 2
			sa := bytecode.NewSuperArray()

			// 从栈上收集元素（按相反顺序）
			// 栈上结构: [value1, flag1, value2, flag2, ...] 或 [key, value, flag, ...]
			type elem struct {
				hasKey bool
				key    bytecode.Value
				value  bytecode.Value
			}
			elements := make([]elem, count)

			for i := count - 1; i >= 0; i-- {
				flag := vm.pop().AsInt()
				if flag == 1 {
					// 键值对
					value := vm.pop()
					key := vm.pop()
					elements[i] = elem{hasKey: true, key: key, value: value}
				} else {
					// 仅值
					value := vm.pop()
					elements[i] = elem{hasKey: false, value: value}
				}
			}

			// 按正确顺序填充 SuperArray
			for _, e := range elements {
				if e.hasKey {
					sa.Set(e.key, e.value)
				} else {
					sa.Push(e.value)
				}
			}

			vm.push(vm.trackAllocation(bytecode.NewSuperArrayValue(sa)))

		case bytecode.OpSuperArrayGet:
			key := vm.pop()
			arrVal := vm.pop()
			if arrVal.Type != bytecode.ValSuperArray {
				return vm.runtimeError("expected super array")
			}
			sa := arrVal.AsSuperArray()
			if value, ok := sa.Get(key); ok {
				vm.push(value)
			} else {
				vm.push(bytecode.NullValue)
			}

		case bytecode.OpSuperArraySet:
			value := vm.pop()
			key := vm.pop()
			arrVal := vm.pop()
			if arrVal.Type != bytecode.ValSuperArray {
				return vm.runtimeError("expected super array")
			}
			sa := arrVal.AsSuperArray()
			sa.Set(key, value)
			vm.push(value)

		// 迭代器操作
		case bytecode.OpIterInit:
			v := vm.pop()
			if v.Type != bytecode.ValArray && v.Type != bytecode.ValFixedArray && v.Type != bytecode.ValMap && v.Type != bytecode.ValSuperArray {
				return vm.runtimeError(i18n.T(i18n.ErrForeachRequiresIterable))
			}
			iter := bytecode.NewIterator(v)
			vm.push(vm.trackAllocation(bytecode.NewIteratorValue(iter)))

		case bytecode.OpIterNext:
			iterVal := vm.peek(0) // 只读取，不弹出
			iter := iterVal.AsIterator()
			if iter == nil {
				return vm.runtimeError(i18n.T(i18n.ErrExpectedIterator))
			}
			hasNext := iter.Next()
			vm.push(bytecode.NewBool(hasNext))

		case bytecode.OpIterKey:
			iterVal := vm.peek(0) // 只读取，不弹出
			iter := iterVal.AsIterator()
			if iter == nil {
				return vm.runtimeError(i18n.T(i18n.ErrExpectedIterator))
			}
			vm.push(iter.Key())

		case bytecode.OpIterValue:
			iterVal := vm.peek(0) // 只读取，不弹出
			iter := iterVal.AsIterator()
			if iter == nil {
				return vm.runtimeError(i18n.T(i18n.ErrExpectedIterator))
			}
			vm.push(iter.CurrentValue())

		// 数组操作扩展
		case bytecode.OpArrayPush:
			value := vm.pop()
			arrVal := vm.pop()
			switch arrVal.Type {
			case bytecode.ValArray:
				arr := arrVal.AsArray()
				arr = append(arr, value)
				vm.push(vm.trackAllocation(bytecode.NewArray(arr)))
			case bytecode.ValSuperArray:
				sa := arrVal.AsSuperArray()
				sa.Push(value)
				vm.push(arrVal) // SuperArray 是引用类型，直接返回
			default:
				return vm.runtimeError(i18n.T(i18n.ErrPushRequiresArray))
			}

		case bytecode.OpArrayHas:
			idx := vm.pop()
			arrVal := vm.pop()
			if arrVal.Type == bytecode.ValSuperArray {
				sa := arrVal.AsSuperArray()
				vm.push(bytecode.NewBool(sa.HasKey(idx)))
				continue
			}
			if arrVal.Type != bytecode.ValArray {
				return vm.runtimeError(i18n.T(i18n.ErrHasRequiresArray))
			}
			arr := arrVal.AsArray()
			i := int(idx.AsInt())
			vm.push(bytecode.NewBool(i >= 0 && i < len(arr)))

		// 字节数组操作
		case bytecode.OpNewBytes:
			count := int(chunk.ReadU16(frame.IP))
			frame.IP += 2
			bytes := make([]byte, count)
			for i := count - 1; i >= 0; i-- {
				bytes[i] = byte(vm.pop().AsInt() & 0xFF)
			}
			vm.push(vm.trackAllocation(bytecode.NewBytes(bytes)))

		case bytecode.OpBytesGet:
			idx := vm.pop()
			bytesVal := vm.pop()
			if bytesVal.Type != bytecode.ValBytes {
				return vm.runtimeError("expected bytes for index operation")
			}
			b := bytesVal.AsBytes()
			i := int(idx.AsInt())
			if i < 0 || i >= len(b) {
				if result := vm.throwTypedException("ArrayIndexOutOfBoundsException", 
					i18n.T(i18n.ErrArrayIndexOutOfBounds, i, len(b))); result == InterpretExceptionHandled {
					frame = &vm.frames[vm.frameCount-1]
					chunk = frame.Closure.Function.Chunk
					continue
				} else {
					return result
				}
			}
			vm.push(bytecode.NewInt(int64(b[i])))

		case bytecode.OpBytesSet:
			value := vm.pop()
			idx := vm.pop()
			bytesVal := vm.pop()
			if bytesVal.Type != bytecode.ValBytes {
				return vm.runtimeError("expected bytes for set operation")
			}
			b := bytesVal.AsBytes()
			i := int(idx.AsInt())
			if i < 0 || i >= len(b) {
				if result := vm.throwTypedException("ArrayIndexOutOfBoundsException", 
					i18n.T(i18n.ErrArrayIndexOutOfBounds, i, len(b))); result == InterpretExceptionHandled {
					frame = &vm.frames[vm.frameCount-1]
					chunk = frame.Closure.Function.Chunk
					continue
				} else {
					return result
				}
			}
			b[i] = byte(value.AsInt() & 0xFF)
			vm.push(bytesVal)

		case bytecode.OpBytesLen:
			bytesVal := vm.pop()
			if bytesVal.Type != bytecode.ValBytes {
				return vm.runtimeError("expected bytes for length operation")
			}
			vm.push(bytecode.NewInt(int64(len(bytesVal.AsBytes()))))

		case bytecode.OpBytesSlice:
			endVal := vm.pop()
			startVal := vm.pop()
			bytesVal := vm.pop()
			if bytesVal.Type != bytecode.ValBytes {
				return vm.runtimeError("expected bytes for slice operation")
			}
			b := bytesVal.AsBytes()
			start := int(startVal.AsInt())
			end := int(endVal.AsInt())
			if end < 0 {
				end = len(b)
			}
			if start < 0 {
				start = 0
			}
			if start > len(b) {
				start = len(b)
			}
			if end > len(b) {
				end = len(b)
			}
			if start > end {
				start = end
			}
			result := make([]byte, end-start)
			copy(result, b[start:end])
			vm.push(vm.trackAllocation(bytecode.NewBytes(result)))

		case bytecode.OpBytesConcat:
			b2Val := vm.pop()
			b1Val := vm.pop()
			if b1Val.Type != bytecode.ValBytes || b2Val.Type != bytecode.ValBytes {
				return vm.runtimeError("expected bytes for concat operation")
			}
			b1 := b1Val.AsBytes()
			b2 := b2Val.AsBytes()
			result := make([]byte, len(b1)+len(b2))
			copy(result, b1)
			copy(result[len(b1):], b2)
			vm.push(vm.trackAllocation(bytecode.NewBytes(result)))

		case bytecode.OpUnset:
			objVal := vm.pop()
			if objVal.Type == bytecode.ValObject {
				obj := objVal.AsObject()
				// 调用析构函数 __destruct
				if method := obj.Class.GetMethod("__destruct"); method != nil {
					// 压入对象作为 receiver
					vm.push(objVal)
					if result := vm.invokeDestructor(obj, method); result != InterpretOK {
						return result
					}
					// 恢复帧引用
					frame = &vm.frames[vm.frameCount-1]
					chunk = frame.Closure.Function.Chunk
				}
			}
			vm.push(bytecode.NullValue)

		// ----------------------------------------------------------------
		// 异常处理：throw / try-catch-finally / rethrow
		// ----------------------------------------------------------------
		case bytecode.OpThrow:
			exceptionVal := vm.pop()
			var exception bytecode.Value
			
			// 处理不同类型的异常
			switch exceptionVal.Type {
			case bytecode.ValString:
				// 字符串异常：自动转换为 Exception 对象
				exception = bytecode.NewException("Exception", exceptionVal.AsString(), 0)
			case bytecode.ValObject:
				// 对象异常：检查是否是 Throwable 的子类
				obj := exceptionVal.AsObject()
				if vm.isThrowable(obj.Class) {
					exception = bytecode.NewExceptionFromObject(obj)
				} else {
					return vm.runtimeError("cannot throw non-Throwable object: %s", obj.Class.Name)
				}
			case bytecode.ValException:
				// 已经是异常值
				exception = exceptionVal
			default:
				return vm.runtimeError("cannot throw value of type %v", exceptionVal.Type)
			}
			
			// 捕获调用栈信息
			if exc := exception.AsException(); exc != nil && len(exc.StackFrames) == 0 {
				exc.SetStackFrames(vm.captureStackTrace())
			}
			if !vm.handleException(exception) {
				if exc := exception.AsException(); exc != nil {
					return vm.runtimeErrorWithException(exc)
				}
				return vm.runtimeError("uncaught exception: %s", exception.String())
			}
			// 更新 frame 和 chunk 引用
			frame = &vm.frames[vm.frameCount-1]
			chunk = frame.Closure.Function.Chunk

		case bytecode.OpEnterTry:
			// 新格式: OpEnterTry catchCount:u8 finallyOffset:i16 [typeIdx:u16 catchOffset:i16]*catchCount
			enterTryIP := frame.IP - 1 // OpEnterTry 指令的位置
			
			catchCount := int(chunk.Code[frame.IP])
			frame.IP++
			
			finallyOffset := chunk.ReadI16(frame.IP)
			frame.IP += 2
			
			// 读取 catch 处理器信息
			var catchHandlers []CatchHandlerInfo
			for i := 0; i < catchCount; i++ {
				typeIdx := chunk.ReadU16(frame.IP)
				frame.IP += 2
				catchOffset := chunk.ReadI16(frame.IP)
				frame.IP += 2
				
				typeName := chunk.Constants[typeIdx].AsString()
				catchHandlers = append(catchHandlers, CatchHandlerInfo{
					TypeName:    typeName,
					CatchOffset: int(catchOffset),
				})
			}
			
			// 计算 finally 块的绝对地址 (-1 表示没有 finally)
			finallyIP := -1
			if finallyOffset != -1 {
				finallyIP = enterTryIP + int(finallyOffset)
			}
			
		vm.tryStack = append(vm.tryStack, TryContext{
			EnterTryIP:    enterTryIP,
			CatchHandlers: catchHandlers,
			FinallyIP:     finallyIP,
			FrameCount:    vm.frameCount,
			StackTop:      vm.stackTop,
		})
		vm.tryDepth++ // 更新 try 深度计数

		case bytecode.OpLeaveTry:
			// 离开 try 块（正常流程）- 零成本异常路径优化
			// 如果没有 finally 块，移除 TryContext
			// 如果有 finally 块，保留 TryContext 供 finally 使用
			if vm.tryDepth > 0 {
				tryCtx := &vm.tryStack[len(vm.tryStack)-1]
				if tryCtx.FinallyIP < 0 {
					// 没有 finally 块，移除 TryContext
					vm.tryStack = vm.tryStack[:len(vm.tryStack)-1]
					vm.tryDepth-- // 更新 try 深度计数
				}
				// 有 finally 块，保留 TryContext，等待 OpLeaveFinally 时移除
			}

		case bytecode.OpEnterCatch:
			// 新格式: OpEnterCatch typeIdx:u16
			// typeIdx 用于调试/日志，VM 在 handleException 中已经做了类型匹配
			frame.IP += 2 // 跳过 typeIdx
			// 异常值已经在栈上
			// 清除异常状态
			vm.hasException = false
			// 标记当前 TryContext 正在执行 catch 块
			if len(vm.tryStack) > 0 {
				vm.tryStack[len(vm.tryStack)-1].InCatch = true
			}

		case bytecode.OpEnterFinally:
			// 进入 finally 块
			// 设置 InFinally 标志，标记当前正在执行 finally 块
			if len(vm.tryStack) > 0 {
				vm.tryStack[len(vm.tryStack)-1].InFinally = true
			}

		case bytecode.OpLeaveFinally:
			// 离开 finally 块，检查是否有挂起的异常或返回值 - 零成本异常路径优化
			if vm.tryDepth > 0 {
				tryCtx := &vm.tryStack[len(vm.tryStack)-1]
				if tryCtx.InFinally {
					tryCtx.InFinally = false
					vm.tryStack = vm.tryStack[:len(vm.tryStack)-1]
					vm.tryDepth-- // 更新 try 深度计数
					
					// 如果有挂起的异常，重新抛出
					if tryCtx.HasPendingExc {
						if !vm.handleException(tryCtx.PendingException) {
							if exc := tryCtx.PendingException.AsException(); exc != nil {
								return vm.runtimeErrorWithException(exc)
							}
							return vm.runtimeError("uncaught exception: %s", tryCtx.PendingException.String())
						}
						frame = &vm.frames[vm.frameCount-1]
						chunk = frame.Closure.Function.Chunk
						continue
					}
					
					// 如果有挂起的返回值，执行返回
					if tryCtx.HasPendingReturn {
						result := tryCtx.PendingReturn
						vm.frameCount--
						if vm.frameCount == 0 {
							vm.pop()
							return InterpretOK
						}
						vm.stackTop = frame.BaseSlot
						vm.push(result)
						frame = &vm.frames[vm.frameCount-1]
						chunk = frame.Closure.Function.Chunk
						continue
					}
					// finally 块正常执行完毕，没有挂起的异常或返回值，继续执行后续代码
				}
			}

		case bytecode.OpRethrow:
			// 重新抛出当前异常
			if vm.hasException {
				if !vm.handleException(vm.exception) {
					if exc := vm.exception.AsException(); exc != nil {
						return vm.runtimeErrorWithException(exc)
					}
					return vm.runtimeError("uncaught exception: %s", vm.exception.String())
				}
				frame = &vm.frames[vm.frameCount-1]
				chunk = frame.Closure.Function.Chunk
			}

		// ----------------------------------------------------------------
		// 类型检查/转换（is/cast）
		// ----------------------------------------------------------------
		case bytecode.OpCheckType:
			typeIdx := chunk.ReadU16(frame.IP)
			frame.IP += 2
			targetType := chunk.Constants[typeIdx].AsString()
			value := vm.pop()
			
			// 执行类型检查，返回布尔结果
			result := vm.checkValueType(value, targetType)
			vm.push(bytecode.NewBool(result))
			
		case bytecode.OpCast:
			typeIdx := chunk.ReadU16(frame.IP)
			frame.IP += 2
			targetType := chunk.Constants[typeIdx].AsString()
			value := vm.pop()
			
			result, ok := vm.castValue(value, targetType)
			if !ok {
				actualType := vm.getValueTypeName(value)
				return vm.runtimeError(i18n.T(i18n.ErrCannotCast, actualType, targetType))
			}
			vm.push(result)

		case bytecode.OpCastSafe:
			typeIdx := chunk.ReadU16(frame.IP)
			frame.IP += 2
			targetType := chunk.Constants[typeIdx].AsString()
			value := vm.pop()
			
			result, ok := vm.castValue(value, targetType)
			if !ok {
				// 安全类型转换失败时返回 null
				vm.push(bytecode.NullValue)
			} else {
				vm.push(result)
			}

		// 调试
		case bytecode.OpDebugPrint:
			fmt.Println(vm.pop().String())

		// =====================================================================
		// 协程操作
		// =====================================================================

		case bytecode.OpGo:
			// 启动协程
			// 栈: [closure] -> [goroutine_id]
			closureVal := vm.pop()
			if closureVal.Type != bytecode.ValClosure {
				return vm.runtimeError("go: expected closure, got %s", closureVal.Type)
			}
			closure := closureVal.Data.(*bytecode.Closure)

			// 创建新协程
			g := vm.scheduler.Spawn(closure)
			if g == nil {
				return vm.runtimeError("go: too many goroutines")
			}

			// 压入协程 ID
			vm.push(bytecode.NewGoroutineValue(g.ID))

		case bytecode.OpYield:
			// 让出执行权（协作式调度点）
			// 当前实现：立即返回，由调度器选择下一个协程
			// 注意：这是简化实现，完整实现需要保存/恢复协程上下文
			vm.scheduler.Yield()

		// =====================================================================
		// 通道操作
		// =====================================================================

		case bytecode.OpChanMake:
			// 创建通道
			// 参数: capacity (u16)
			// 栈: [] -> [channel]
			capacity := int(chunk.ReadU16(frame.IP))
			frame.IP += 2
			ch := NewChannel("", capacity) // 元素类型在编译时已检查
			vm.push(bytecode.NewChannelValue(ch))

		case bytecode.OpChanSend:
			// 发送到通道
			// 栈: [channel, value] -> []
			value := vm.pop()
			chanVal := vm.pop()

			if !chanVal.IsChannel() {
				return vm.runtimeError("send: expected channel, got %s", chanVal.Type)
			}
			ch := chanVal.AsChannel().(*Channel)

			// 尝试发送
			g := vm.scheduler.Current()
			if g == nil {
				// 主协程直接发送（可能阻塞）
				ok, blocked := ch.Send(value, nil, vm.scheduler)
				if !ok {
					return vm.runtimeError("send on closed channel")
				}
				if blocked {
					// 主协程阻塞 - 这里简化处理，实际需要调度
					return vm.runtimeError("main goroutine blocked on channel send")
				}
			} else {
				ok, blocked := ch.Send(value, g, vm.scheduler)
				if !ok {
					return vm.runtimeError("send on closed channel")
				}
				if blocked {
					vm.scheduler.Block(g, ch, BlockSend)
				}
			}

		case bytecode.OpChanRecv:
			// 从通道接收
			// 栈: [channel] -> [value]
			chanVal := vm.pop()

			if !chanVal.IsChannel() {
				return vm.runtimeError("receive: expected channel, got %s", chanVal.Type)
			}
			ch := chanVal.AsChannel().(*Channel)

			// 尝试接收
			g := vm.scheduler.Current()
			if g == nil {
				// 主协程直接接收（可能阻塞）
				value, ok, blocked := ch.Receive(nil, vm.scheduler)
				if blocked {
					return vm.runtimeError("main goroutine blocked on channel receive")
				}
				if !ok {
					vm.push(bytecode.NullValue)
				} else {
					vm.push(value)
				}
			} else {
				value, ok, blocked := ch.Receive(g, vm.scheduler)
				if blocked {
					vm.scheduler.Block(g, ch, BlockRecv)
					// 阻塞后，接收值会在协程被唤醒时设置到 g.RecvValue
				} else if !ok {
					vm.push(bytecode.NullValue)
				} else {
					vm.push(value)
				}
			}

		case bytecode.OpChanClose:
			// 关闭通道
			// 栈: [channel] -> []
			chanVal := vm.pop()

			if !chanVal.IsChannel() {
				return vm.runtimeError("close: expected channel, got %s", chanVal.Type)
			}
			ch := chanVal.AsChannel().(*Channel)
			ch.Close(vm.scheduler)

		case bytecode.OpChanTrySend:
			// 非阻塞发送
			// 栈: [channel, value] -> [bool]
			value := vm.pop()
			chanVal := vm.pop()

			if !chanVal.IsChannel() {
				return vm.runtimeError("trySend: expected channel, got %s", chanVal.Type)
			}
			ch := chanVal.AsChannel().(*Channel)

			ok := ch.TrySend(value, vm.scheduler)
			vm.push(bytecode.NewBool(ok))

		case bytecode.OpChanTryRecv:
			// 非阻塞接收
			// 栈: [channel] -> [value, bool]
			chanVal := vm.pop()

			if !chanVal.IsChannel() {
				return vm.runtimeError("tryReceive: expected channel, got %s", chanVal.Type)
			}
			ch := chanVal.AsChannel().(*Channel)

			value, ok, _ := ch.TryReceive(vm.scheduler)
			vm.push(value)
			vm.push(bytecode.NewBool(ok))

		// =====================================================================
		// Select 操作（简化实现）
		// =====================================================================

		case bytecode.OpSelectStart:
			// 开始 select 语句
			// 参数: caseCount (u8)
			// 当前简化实现：不做特殊处理
			_ = chunk.Code[frame.IP] // caseCount
			frame.IP++

		case bytecode.OpSelectCase:
			// 添加 select case
			// 参数: isRecv (u8), jumpOffset (i16)
			_ = chunk.Code[frame.IP] // isRecv
			frame.IP++
			_ = chunk.ReadI16(frame.IP) // jumpOffset
			frame.IP += 2

		case bytecode.OpSelectDefault:
			// 添加 select default
			// 参数: jumpOffset (i16)
			_ = chunk.ReadI16(frame.IP) // jumpOffset
			frame.IP += 2

		case bytecode.OpSelectWait:
			// 等待 select 完成
			// 当前简化实现：总是执行 default 或第一个可用的 case
			// 完整实现需要配合 SelectStart/SelectCase 收集的信息
			vm.push(bytecode.NewInt(0)) // 返回选中的 case 索引

		case bytecode.OpHalt:
			return InterpretOK

		default:
			return vm.runtimeError(i18n.T(i18n.ErrUnknownOpcode, instruction))
		}
	}
}

// ============================================================================
// 异常处理（try/catch/finally）
// ============================================================================

// handleException 处理一个“已经发生”的异常，并尝试将控制流转移到合适的处理器。
//
// 这是虚拟机异常机制的核心入口，可能由以下场景触发：
//   - OpThrow 指令显式抛出异常
//   - VM 内部检测到运行时错误并将其包装为异常（如数组越界、除零）
//   - 内置函数/宿主代码 panic 被捕获并转换为异常
//
// 行为（高层语义）：
//   1. 设置 vm.exception / vm.hasException
//   2. 若异常尚无堆栈信息，捕获当前调用栈（用于最终输出）
//   3. 从 tryStack 栈顶向下查找最近的 try 上下文：
//      - 若存在匹配的 catch：恢复栈/帧到 try 进入时的状态，跳转到 catch 入口，
//        并把异常对象/值压栈供 catch 代码读取
//      - 若没有匹配 catch 但有 finally：挂起异常，跳转执行 finally
//      - 若正在 catch/finally 内又发生异常：按规则先执行 finally（如存在），或继续向外传播
//   4. 若最终没有任何处理器，则返回 false（表示“未捕获异常”）
//
// 重要规则（维护者容易踩坑的点）：
//   - finally 中抛出的新异常会覆盖原异常（原异常会被丢弃/不再传播）
//   - catch 中发生新异常：如果存在 finally，必须先执行 finally，再传播新异常
//   - 进入 catch 时，VM 会把异常值（或异常对象）压入栈顶，供字节码加载到局部变量
//
// 返回值：
//   - true  表示 VM 已经将控制流转移到 catch/finally（调用方需要刷新 frame/chunk 后继续执行）
//   - false 表示异常未被捕获，调用方应将其作为未捕获异常输出并终止执行
func (vm *VM) handleException(exception bytecode.Value) bool {
	vm.exception = exception
	vm.hasException = true
	
	// 为异常添加调用栈信息
	if exc := exception.AsException(); exc != nil && len(exc.StackFrames) == 0 {
		exc.SetStackFrames(vm.captureStackTrace())
	}
	
	// 获取异常对象用于类型匹配
	exc := exception.AsException()
	
	// 查找最近的 try 块
	for len(vm.tryStack) > 0 {
		tryCtx := &vm.tryStack[len(vm.tryStack)-1]
		
		// 如果正在执行 finally 块中发生异常，记录但继续传播
		if tryCtx.InFinally {
			vm.tryStack = vm.tryStack[:len(vm.tryStack)-1]
			continue
		}
		
		// 如果正在执行 catch 块中发生异常，需要执行 finally（如果有）然后传播
		if tryCtx.InCatch {
			// 如果有 finally 块，先执行 finally
			if tryCtx.FinallyIP >= 0 {
				tryCtx.PendingException = exception
				tryCtx.HasPendingExc = true
				tryCtx.InFinally = true
				tryCtx.InCatch = false
				if vm.frameCount > 0 {
					frame := &vm.frames[vm.frameCount-1]
					frame.IP = tryCtx.FinallyIP
				}
				vm.hasException = false
				return true
			}
			// 没有 finally，移除此 TryContext 并继续传播
			vm.tryStack = vm.tryStack[:len(vm.tryStack)-1]
			continue
		}
		
		// 展开调用栈到 try 块所在的帧
		for vm.frameCount > tryCtx.FrameCount {
			vm.frameCount--
		}
		
		if vm.frameCount > 0 {
			frame := &vm.frames[vm.frameCount-1]
			
			// 恢复栈状态
			vm.stackTop = tryCtx.StackTop
			
			// 查找匹配的 catch 处理器
			matchedHandler := -1
			for i, handler := range tryCtx.CatchHandlers {
				if vm.exceptionMatchesType(exc, handler.TypeName) {
					matchedHandler = i
					break
				}
			}
			
			if matchedHandler >= 0 {
				// 找到匹配的 catch 处理器
				handler := tryCtx.CatchHandlers[matchedHandler]
				catchIP := tryCtx.EnterTryIP + handler.CatchOffset
				frame.IP = catchIP
				
				// 如果异常有关联的对象，推入对象；否则推入异常值
				if exc != nil && exc.Object != nil {
					vm.push(bytecode.NewObject(exc.Object))
				} else {
					vm.push(exception)
				}
				
				vm.hasException = false
				// 不要移除 tryCtx，因为 catch 块执行完后可能还需要执行 finally
				return true
			} else if tryCtx.FinallyIP >= 0 {
				// 没有匹配的 catch，但有 finally 块
				// 挂起异常，先执行 finally
				tryCtx.PendingException = exception
				tryCtx.HasPendingExc = true
				tryCtx.InFinally = true
				frame.IP = tryCtx.FinallyIP
				vm.hasException = false
				return true
			}
			// 没有匹配的处理器也没有 finally，继续向上传播
		}
		
		vm.tryStack = vm.tryStack[:len(vm.tryStack)-1]
	}
	
	// 没有找到处理程序
	return false
}

// captureStackTrace 捕获当前调用栈信息（用于异常与运行时错误输出）。
//
// 该方法从当前帧向下遍历到 0 号帧，构造一个“从最近调用到最早调用”的堆栈帧列表。
// 注意：这里的顺序是为了更符合 Java/C# 风格的异常输出（顶部是最近的调用点）。
func (vm *VM) captureStackTrace() []bytecode.StackFrame {
	var frames []bytecode.StackFrame
	for i := vm.frameCount - 1; i >= 0; i-- {
		frame := &vm.frames[i]
		fn := frame.Closure.Function
		line := 0
		if frame.IP > 0 && frame.IP-1 < len(fn.Chunk.Lines) {
			line = fn.Chunk.Lines[frame.IP-1]
		}
		frames = append(frames, bytecode.StackFrame{
			FunctionName: fn.Name,
			ClassName:    fn.ClassName, // 设置类名用于堆栈跟踪
			FileName:     fn.SourceFile,
			LineNumber:   line,
		})
	}
	return frames
}

// exceptionMatchesType 检查异常是否匹配指定类型名。
//
// 匹配策略：
//   - 先使用 bytecode.Exception 内置的类型匹配（通常基于 exc.Type 字符串/命名空间）
//   - 若异常携带了对象实例，则进一步按类继承链判断（支持 Throwable 子类体系）
//
// 备注：这里的 typeName 是字节码中记录的 catch 类型名（可能为简单名或全名）。
func (vm *VM) exceptionMatchesType(exc *bytecode.Exception, typeName string) bool {
	if exc == nil {
		return false
	}
	
	// 使用 bytecode 包中的类型匹配逻辑
	if exc.IsExceptionOfType(typeName) {
		return true
	}
	
	// 如果有关联的类对象，检查 VM 中注册的类层次结构
	if exc.Object != nil {
		return vm.isInstanceOfType(exc.Object.Class, typeName)
	}
	
	return false
}

// isInstanceOfType 判断 class 是否为 typeName，或是否继承自 typeName。
//
// 该方法主要用于运行时类型检查（异常匹配、is/cast 等）。
// 除了沿着 class.Parent 指针遍历外，还会在 Parent 尚未解析时回退到 vm.classes 做一次查找，
// 以处理“类先注册、继承关系后解析”的情况。
func (vm *VM) isInstanceOfType(class *bytecode.Class, typeName string) bool {
	// 遍历类继承链
	for c := class; c != nil; c = c.Parent {
		if c.Name == typeName {
			return true
		}
	}
	
	// 也检查 VM 中注册的类（用于处理 parent 尚未解析的情况）
	if class.ParentName != "" && class.Parent == nil {
		if parent := vm.classes[class.ParentName]; parent != nil {
			return vm.isInstanceOfType(parent, typeName)
		}
	}
	
	return false
}

// isThrowable 判断一个类是否属于 Throwable 体系。
//
// 语义：只有 Throwable（及其子类）才允许被 throw。
func (vm *VM) isThrowable(class *bytecode.Class) bool {
	return vm.isInstanceOfType(class, "Throwable")
}

// throwRuntimeException 抛出 RuntimeException（可被 try-catch 捕获）。
//
// 这是 VM 内部“运行时错误 -> 可捕获异常”的常用入口。
func (vm *VM) throwRuntimeException(message string) InterpretResult {
	return vm.throwTypedException("RuntimeException", message)
}

// throwTypedException 抛出指定类型的异常（可被 try-catch 捕获）。
//
// 参数：
//   - typeName: 异常类型名（如 "DivideByZeroException", "ArrayIndexOutOfBoundsException"）
//   - message:  异常消息
//
// 运行时类查找策略（按顺序尝试）：
//   1) 指定类型：先尝试标准库命名空间 "sola.lang."+typeName，再尝试简单名 typeName
//   2) 若指定类型不是 RuntimeException/Exception，则回退到 RuntimeException
//   3) 再回退到 Exception
//   4) 若依然找不到类定义，则构造一个“纯值异常”（bytecode.Exception）作为退化方案
//
// 设计目标：
//   - 让标准库异常体系可被用户 try-catch 捕获（基于类继承链匹配）
//   - 即使异常类未注册，也能保证有可输出的异常信息（不至于崩溃）
func (vm *VM) throwTypedException(typeName string, message string) InterpretResult {
	var exception bytecode.Value
	
	// 尝试按优先级查找异常类
	// 首先尝试完整命名空间名称（sola.lang.类名），然后尝试简单类名
	var classNames []string
	if typeName != "RuntimeException" && typeName != "Exception" {
		// 对于标准库异常，尝试完整命名空间名称
		classNames = []string{"sola.lang." + typeName, typeName, "sola.lang.RuntimeException", "RuntimeException", "sola.lang.Exception", "Exception"}
	} else {
		classNames = []string{"sola.lang." + typeName, typeName, "sola.lang.Exception", "Exception"}
	}
	var foundClass *bytecode.Class
	var foundTypeName string
	
	for _, name := range classNames {
		if class := vm.classes[name]; class != nil {
			foundClass = class
			foundTypeName = name
			break
		}
	}
	
	if foundClass != nil {
		// 找到异常类，创建对象实例
		obj := bytecode.NewObjectInstance(foundClass)
		obj.Fields["message"] = bytecode.NewString(message)
		obj.Fields["code"] = bytecode.NewInt(0)
		obj.Fields["previous"] = bytecode.NullValue
		obj.Fields["stackTrace"] = bytecode.NewArray([]bytecode.Value{})
		exception = bytecode.NewExceptionFromObject(obj)
	} else {
		// 没有异常类可用，使用简单异常值
		exception = bytecode.NewException(typeName, message, 0)
		foundTypeName = typeName
	}
	
	// 如果找到的不是请求的类型，但我们有消息，更新异常类型名
	if exc := exception.AsException(); exc != nil {
		if foundTypeName != typeName && foundClass != nil {
			// 即使使用了父类，也保留原始类型名用于显示
			exc.Type = typeName
		}
		if len(exc.StackFrames) == 0 {
			exc.SetStackFrames(vm.captureStackTrace())
		}
	}
	
	if vm.handleException(exception) {
		// 异常被捕获，返回特殊状态让调用者刷新 frame/chunk
		return InterpretExceptionHandled
	}
	// 未捕获的异常
	if exc := exception.AsException(); exc != nil {
		return vm.runtimeErrorWithException(exc)
	}
	return vm.runtimeError("uncaught exception: %s", exception.String())
}

// ============================================================================
// 栈操作（操作数栈）
// ============================================================================

// 说明：
//   - VM 使用固定大小数组 stack + stackTop 作为操作数栈；
//   - 这里的 push/pop/peek 都是“无边界检查”的快速实现；
//   - 栈溢出主要在函数调用（frames）或编译期/运行期约束中被避免。
//
// 维护者注意：
//   - 若未来引入更大的 StackMax 或动态栈，请同步检查所有 BaseSlot 相关计算；
//   - 如果在调试阶段需要更友好的错误，可在这些方法中加入断言，但不要在热路径开启。
func (vm *VM) push(value bytecode.Value) {
	vm.stack[vm.stackTop] = value
	vm.stackTop++
}

func (vm *VM) pop() bytecode.Value {
	vm.stackTop--
	return vm.stack[vm.stackTop]
}

func (vm *VM) peek(distance int) bytecode.Value {
	return vm.stack[vm.stackTop-1-distance]
}

// 二元运算
func (vm *VM) binaryOp(op bytecode.OpCode) InterpretResult {
	b := vm.pop()
	a := vm.pop()

	// 【重要】字符串拼接：只有两个操作数都是字符串时才允许拼接
	// 【警告】请勿将条件修改为 (a.Type == ValString || b.Type == ValString)
	// 否则会导致类型不安全的隐式转换问题：
	//   - "hello" + 123 会变成 "hello123"（应该报错）
	//   - 456 + "world" 会变成 "456world"（应该报错）
	// 正确的行为：只有 string + string 才能相加，其他情况应该报类型错误
	if op == bytecode.OpAdd && a.Type == bytecode.ValString && b.Type == bytecode.ValString {
		vm.push(bytecode.NewString(a.AsString() + b.AsString()))
		return InterpretOK
	}

	// 数值运算
	if a.Type == bytecode.ValInt && b.Type == bytecode.ValInt {
		ai, bi := a.AsInt(), b.AsInt()
		switch op {
		case bytecode.OpAdd:
			vm.push(bytecode.NewInt(ai + bi))
		case bytecode.OpSub:
			vm.push(bytecode.NewInt(ai - bi))
		case bytecode.OpMul:
			vm.push(bytecode.NewInt(ai * bi))
		case bytecode.OpDiv:
			if bi == 0 {
				return vm.throwTypedException("DivideByZeroException", i18n.T(i18n.ErrDivisionByZero))
			}
			vm.push(bytecode.NewInt(ai / bi))
		case bytecode.OpMod:
			if bi == 0 {
				return vm.throwTypedException("DivideByZeroException", i18n.T(i18n.ErrDivisionByZero))
			}
			vm.push(bytecode.NewInt(ai % bi))
		}
		return InterpretOK
	}

	// 浮点运算
	if (a.Type == bytecode.ValInt || a.Type == bytecode.ValFloat) &&
		(b.Type == bytecode.ValInt || b.Type == bytecode.ValFloat) {
		af, bf := a.AsFloat(), b.AsFloat()
		switch op {
		case bytecode.OpAdd:
			vm.push(bytecode.NewFloat(af + bf))
		case bytecode.OpSub:
			vm.push(bytecode.NewFloat(af - bf))
		case bytecode.OpMul:
			vm.push(bytecode.NewFloat(af * bf))
		case bytecode.OpDiv:
			if bf == 0 {
				return vm.throwTypedException("DivideByZeroException", i18n.T(i18n.ErrDivisionByZero))
			}
			vm.push(bytecode.NewFloat(af / bf))
		case bytecode.OpMod:
			return vm.throwRuntimeException(i18n.T(i18n.ErrModuloNotForFloats))
		}
		return InterpretOK
	}

	return vm.runtimeError(i18n.T(i18n.ErrOperandsMustBeNumbers))
}

// 比较运算
func (vm *VM) compareOp(op bytecode.OpCode) InterpretResult {
	b := vm.pop()
	a := vm.pop()

	// 数值比较
	if (a.Type == bytecode.ValInt || a.Type == bytecode.ValFloat) &&
		(b.Type == bytecode.ValInt || b.Type == bytecode.ValFloat) {
		af, bf := a.AsFloat(), b.AsFloat()
		var result bool
		switch op {
		case bytecode.OpLt:
			result = af < bf
		case bytecode.OpLe:
			result = af <= bf
		case bytecode.OpGt:
			result = af > bf
		case bytecode.OpGe:
			result = af >= bf
		}
		vm.push(bytecode.NewBool(result))
		return InterpretOK
	}

	// 字符串比较
	if a.Type == bytecode.ValString && b.Type == bytecode.ValString {
		as, bs := a.AsString(), b.AsString()
		var result bool
		switch op {
		case bytecode.OpLt:
			result = as < bs
		case bytecode.OpLe:
			result = as <= bs
		case bytecode.OpGt:
			result = as > bs
		case bytecode.OpGe:
			result = as >= bs
		}
		vm.push(bytecode.NewBool(result))
		return InterpretOK
	}

	return vm.runtimeError(i18n.T(i18n.ErrOperandsMustBeComparable))
}

// ============================================================================
// 调用与返回（函数/闭包/内置函数）
// ============================================================================

// callValue 对栈上的“可调用值”进行调用分发。
//
// callee 可能是：
//   - Closure：用户函数或方法闭包
//   - Func：函数对象（可能是内置函数，也可能是普通函数）
//
// 约定（调用点的栈布局）：
//   - 栈顶从上到下依次是 argN ... arg0, callee
//   - argCount 指参数个数（不含 callee 自身）
//
// 该方法不直接执行字节码；它只负责把调用“装配”为一个新的 CallFrame
// 或走内置函数的快速路径，然后由主循环继续执行。
func (vm *VM) callValue(callee bytecode.Value, argCount int) InterpretResult {
	switch callee.Type {
	case bytecode.ValClosure:
		// 使用优化的调用路径
		return vm.callOptimized(callee.Data.(*bytecode.Closure), argCount)
	case bytecode.ValFunc:
		fn := callee.Data.(*bytecode.Function)
		// 特殊处理内置函数（使用优化路径）
		if fn.IsBuiltin && fn.BuiltinFn != nil {
			return vm.callBuiltinOptimized(fn, argCount)
		}
		closure := &bytecode.Closure{Function: fn}
		return vm.callOptimized(closure, argCount)
	default:
		return vm.runtimeError(i18n.T(i18n.ErrCanOnlyCallFunctions))
	}
}

// tailCall 执行尾调用（Tail Call）：复用当前栈帧而非创建新帧。
//
// 尾调用的目标：
//   - 消除尾递归导致的帧增长（避免 frames 溢出）
//   - 通过“就地替换当前帧”为被调用函数，达到等价于跳转的效果
//
// 与普通 call 的核心差异：
//   - call 会新增一帧（frameCount++），返回时再弹出；
//   - tailCall 不新增帧，而是：
//       1) 把新 callee/参数搬运到当前帧的 BaseSlot 起始位置
//       2) 重置 currentFrame.Closure / IP / BaseSlot
//       3) 继续在同一帧中解释执行新函数
//
// 栈约定（OpTailCall 调用点）：
//   - 与 OpCall 相同：栈顶为 argN...arg0, callee（callee 在参数之下）
//   - argCount 为参数数量
//
// 重要限制：
//   - 这里对“内置函数”的尾调用做了退化处理（模拟 return），原因是：
//       - 内置函数不经过字节码解释器，不易复用帧语义；
//       - 退化路径需要非常小心地恢复调用者栈状态。
func (vm *VM) tailCall(callee bytecode.Value, argCount int) InterpretResult {
	var closure *bytecode.Closure
	
	switch callee.Type {
	case bytecode.ValClosure:
		closure = callee.Data.(*bytecode.Closure)
	case bytecode.ValFunc:
		fn := callee.Data.(*bytecode.Function)
		// 内置函数不支持尾调用，退化为普通调用并处理返回
		if fn.IsBuiltin && fn.BuiltinFn != nil {
			// 将参数从栈中取出
			args := make([]bytecode.Value, argCount)
			for i := argCount - 1; i >= 0; i-- {
				args[i] = vm.pop()
			}
			// 弹出函数本身
			vm.pop()
			// 调用内置函数
			result := fn.BuiltinFn(args)
			
			// 尾调用场景：需要模拟返回逻辑
			// 但不能直接减少帧计数，因为调用者可能还需要继续执行
			// 正确的做法是把结果放到栈顶，让调用者处理
			// 注意：对于尾调用，调用者期望看到返回值在栈顶
			// 但在 OpTailCall 处理中，frame 会被重新获取
			// 所以这里只需要 push 结果，然后返回一个特殊状态表示需要执行返回
			
			// 简单方案：执行完内置函数后，模拟 OpReturn 的行为
			vm.frameCount--
			if vm.frameCount == 0 {
				// 程序结束
				vm.push(result)
				return InterpretOK
			}
			// 恢复到调用者的帧
			callerFrame := &vm.frames[vm.frameCount-1]
			vm.stackTop = callerFrame.BaseSlot + 1
			vm.stack[vm.stackTop-1] = result
			return InterpretOK
		}
		closure = &bytecode.Closure{Function: fn}
	default:
		return vm.runtimeError(i18n.T(i18n.ErrCanOnlyCallFunctions))
	}
	
	fn := closure.Function
	
	// 检查参数数量
	if fn.IsVariadic {
		if argCount < fn.MinArity {
			return vm.runtimeError(i18n.T(i18n.ErrArgumentCountMin, fn.MinArity, argCount))
		}
	} else {
		if argCount < fn.MinArity {
			return vm.runtimeError(i18n.T(i18n.ErrArgumentCountMin, fn.MinArity, argCount))
		}
		if argCount > fn.Arity {
			return vm.runtimeError(i18n.T(i18n.ErrArgumentCountMax, fn.Arity, argCount))
		}
	}
	
	// 获取当前帧
	currentFrame := &vm.frames[vm.frameCount-1]
	
	// 处理默认参数：填充缺失的参数
	if !fn.IsVariadic && argCount < fn.Arity {
		defaultStart := fn.MinArity
		for i := argCount; i < fn.Arity; i++ {
			defaultIdx := i - defaultStart
			if defaultIdx >= 0 && defaultIdx < len(fn.DefaultValues) {
				vm.push(fn.DefaultValues[defaultIdx])
			} else {
				vm.push(bytecode.NullValue)
			}
		}
		argCount = fn.Arity
	}
	
	// 处理可变参数：将多余参数打包成数组
	if fn.IsVariadic {
		variadicCount := argCount - fn.MinArity
		if variadicCount > 0 {
			// 收集可变参数到数组
			varArgs := make([]bytecode.Value, variadicCount)
			for i := variadicCount - 1; i >= 0; i-- {
				varArgs[i] = vm.pop()
			}
			argCount = fn.MinArity
			vm.push(bytecode.NewArray(varArgs))
			argCount++ // 可变参数数组占一个 slot
		} else {
			// 没有可变参数，推入空数组
			vm.push(bytecode.NewArray([]bytecode.Value{}))
			argCount++
		}
	}
	
	// 将参数从栈顶移动到当前帧的参数位置
	// 栈布局：[..., old_callee, old_arg0, old_arg1, ..., new_arg0, new_arg1, ...]
	// 需要移动到：[..., new_callee, new_arg0, new_arg1, ...]
	
	// 保存新参数（从栈顶开始，使用对象池减少分配）
	newArgs := vm.gc.GetArgsFromPool(argCount + 1) // +1 for callee
	for i := argCount; i >= 0; i-- {
		newArgs[i] = vm.pop()
	}
	
	// 调整栈：移除旧参数，保留调用者
	// 计算需要保留的栈大小（从调用者到旧参数之前）
	keepSize := currentFrame.BaseSlot
	vm.stackTop = keepSize
	
	// 将新参数和函数放回栈
	vm.push(bytecode.NewClosure(closure)) // 新函数
	for i := 0; i < argCount; i++ {
		vm.push(newArgs[i+1]) // 新参数
	}
	
	// 归还参数数组到池
	vm.gc.ReturnArgsToPool(newArgs)
	
	// 如果有 upvalues，将它们作为额外的局部变量
	for _, upval := range closure.Upvalues {
		if upval.IsClosed {
			vm.push(upval.Closed)
		} else {
			vm.push(*upval.Location)
		}
	}
	
	// 复用当前帧：更新闭包和 IP，重置 BaseSlot
	currentFrame.Closure = closure
	currentFrame.IP = 0
	currentFrame.BaseSlot = keepSize
	
	return InterpretOK
}

// callBuiltin 调用内置函数（宿主函数），并将 Go panic/异常值转换为 VM 可处理的异常。
//
// 栈约定：
//   - 调用前：argN...arg0, callee
//   - 本方法会 pop 掉所有参数与 callee，并将结果 push 回栈顶（或转移到异常处理流程）
//
// 异常语义：
//   - 若内置函数触发 Go panic：捕获并转换为 NativeException，然后走 handleException
//   - 若内置函数直接返回一个 ValException：同样走 handleException
//
// 注意：这个实现会为参数分配一个切片（make），若在极热路径建议使用对象池版本
//（本仓库里存在 callBuiltinOptimized，用于减少分配）。
func (vm *VM) callBuiltin(fn *bytecode.Function, argCount int) InterpretResult {
	// 收集参数
	args := make([]bytecode.Value, argCount)
	for i := argCount - 1; i >= 0; i-- {
		args[i] = vm.pop()
	}
	vm.pop() // 弹出函数本身
	
	// 使用 defer/recover 捕获 Go 原生 panic
	var result bytecode.Value
	var panicErr interface{}
	
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicErr = r
			}
		}()
		result = fn.BuiltinFn(args)
	}()
	
	// 如果发生 panic，转换为异常
	if panicErr != nil {
		var errMsg string
		switch e := panicErr.(type) {
		case error:
			errMsg = e.Error()
		case string:
			errMsg = e
		default:
			errMsg = fmt.Sprintf("%v", panicErr)
		}
		exception := bytecode.NewException("NativeException", errMsg, 0)
		if !vm.handleException(exception) {
			return vm.runtimeError("uncaught native exception: %s", errMsg)
		}
		return InterpretOK
	}
	
	// 检查返回值是否是异常
	if result.Type == bytecode.ValException {
		if !vm.handleException(result) {
			if exc := result.AsException(); exc != nil {
				return vm.runtimeErrorWithException(exc)
			}
			return vm.runtimeError("uncaught exception: %s", result.String())
		}
		return InterpretOK
	}
	
	vm.push(result)
	return InterpretOK
}

// call 调用一个闭包（用户函数/方法的字节码入口），为其创建新的调用帧。
//
// 栈约定：
//   - 调用点栈布局：argN...arg0, callee
//   - 进入被调函数后，frame.BaseSlot 指向 callee 的槽位
//     也即：slot0 是 callee（闭包本身），slot1.. 是参数
//
// 参数处理（非常关键，维护时请保持一致）：
//   - 默认参数：当 argCount < Arity 且 >= MinArity 时，按 DefaultValues 填充缺失参数
//   - 可变参数：当 IsVariadic 时，将超出 MinArity 的参数打包成数组并作为最后一个参数槽
//
// 帧创建：
//   - frame.BaseSlot = vm.stackTop - argCount - 1
//   - frame.IP 从 0 开始
//
// 备注：
//   - call 只负责“装配帧”；真正的执行仍由 vm.execute() 主循环驱动。
func (vm *VM) call(closure *bytecode.Closure, argCount int) InterpretResult {
	fn := closure.Function
	
	// 记录函数调用到profiler（用于热点检测）
	if vm.jitEnabled && vm.jitCompiler != nil {
		profiler := vm.jitCompiler.GetProfiler()
		if profiler != nil {
			profiler.RecordCall(fn)
		}
	}
	
	// 检查是否已 JIT 编译并可执行
	if vm.jitEnabled && vm.jitCompiler != nil {
		if compiled := vm.jitCompiler.GetCompiled(fn.Name); compiled != nil {
			// 尝试使用 JIT 编译的代码执行
			result, ok := vm.executeNative(compiled, closure, argCount)
			if ok {
				return result
			}
			// JIT 执行失败，回退到解释执行
		}
	}
	
	// 检查参数数量
	if fn.IsVariadic {
		// 可变参数函数：至少需要 MinArity 个参数
		if argCount < fn.MinArity {
			return vm.runtimeError(i18n.T(i18n.ErrArgumentCountMin, fn.MinArity, argCount))
		}
	} else {
		// 普通函数：检查参数数量范围
		if argCount < fn.MinArity {
			return vm.runtimeError(i18n.T(i18n.ErrArgumentCountMin, fn.MinArity, argCount))
		}
		if argCount > fn.Arity {
			return vm.runtimeError(i18n.T(i18n.ErrArgumentCountMax, fn.Arity, argCount))
		}
	}

	if vm.frameCount == FramesMax {
		return vm.runtimeError(i18n.T(i18n.ErrStackOverflow))
	}

	// 处理默认参数：填充缺失的参数
	if !fn.IsVariadic && argCount < fn.Arity {
		defaultStart := fn.MinArity
		for i := argCount; i < fn.Arity; i++ {
			defaultIdx := i - defaultStart
			if defaultIdx >= 0 && defaultIdx < len(fn.DefaultValues) {
				vm.push(fn.DefaultValues[defaultIdx])
			} else {
				vm.push(bytecode.NullValue)
			}
		}
		argCount = fn.Arity
	}

	// 处理可变参数：将多余参数打包成数组
	if fn.IsVariadic {
		variadicCount := argCount - fn.MinArity
		if variadicCount > 0 {
			// 收集可变参数到数组
			varArgs := make([]bytecode.Value, variadicCount)
			for i := variadicCount - 1; i >= 0; i-- {
				varArgs[i] = vm.pop()
			}
			argCount = fn.MinArity
			vm.push(bytecode.NewArray(varArgs))
			argCount++ // 可变参数数组占一个 slot
		} else {
			// 没有可变参数，推入空数组
			vm.push(bytecode.NewArray([]bytecode.Value{}))
			argCount++
		}
	}

	frame := &vm.frames[vm.frameCount]
	vm.frameCount++
	frame.Closure = closure
	frame.IP = 0
	frame.BaseSlot = vm.stackTop - argCount - 1
	
	// 如果有 upvalues，将它们作为额外的局部变量
	// 布局：[caller, arg0, arg1, ..., upval0, upval1, ...]
	for _, upval := range closure.Upvalues {
		if upval.IsClosed {
			vm.push(upval.Closed)
		} else {
			vm.push(*upval.Location)
		}
	}

	return InterpretOK
}

// jitCallCount 用于调试的 JIT 调用计数
var jitCallCount int64

// executeNative 执行 JIT 编译的本机代码
// 返回 (结果, 是否成功执行)
func (vm *VM) executeNative(compiled *jit.CompiledFunc, closure *bytecode.Closure, argCount int) (result InterpretResult, success bool) {
	fn := closure.Function
	
	// 只对简单的纯计算函数启用 JIT 执行
	// 复杂函数（如有闭包、可变参数等）暂不支持
	if fn.IsVariadic || len(closure.Upvalues) > 0 {
		return InterpretOK, false
	}
	
	// 最多支持 4 个参数（调用约定限制）
	if argCount > 4 {
		return InterpretOK, false
	}
	
	// 检查函数是否可以被 JIT 执行
	if !jit.CanJIT(fn) {
		return InterpretOK, false
	}
	
	// 准备参数
	args := make([]int64, argCount)
	baseSlot := vm.stackTop - argCount
	for i := 0; i < argCount; i++ {
		val := vm.stack[baseSlot+i]
		args[i] = jit.ValueToInt64(val)
	}
	
	// 获取函数入口点
	entryPoint := compiled.EntryPoint()
	if entryPoint == 0 {
		return InterpretOK, false
	}
	
	// 使用 defer/recover 捕获 JIT 代码中的崩溃
	defer func() {
		if r := recover(); r != nil {
			// JIT 执行崩溃，回退到解释执行
			// 记录统计信息（可选）
			if vm.jitCompiler != nil {
				// 可以在这里记录崩溃统计
			}
			result = InterpretOK
			success = false
		}
	}()
	
	// 调用本机代码
	nativeResult, ok := jit.CallNative(entryPoint, args)
	if !ok {
		// JIT 执行失败，回退到解释执行
		return InterpretOK, false
	}
	
	jitCallCount++
	
	// 弹出函数和参数
	vm.stackTop = baseSlot - 1
	
	// 将结果推入栈
	vm.push(jit.Int64ToValue(nativeResult))
	
	return InterpretOK, true
}

// GetJITCallCount 获取 JIT 调用次数（调试用）
func GetJITCallCount() int64 {
	return jitCallCount
}

// callMethodDirect 在内联缓存（IC）命中时直接调用已解析的方法实现。
//
// 该路径避免了 invokeMethod 中的：
//   - 方法查找（按类/父类/默认参数/接口 vtable 等）
//   - 某些动态分派开销
//
// 注意：这里仍然会将 *bytecode.Method 包装成 *bytecode.Function 再走 callOptimized，
// 这是为了复用统一的调用栈/参数处理逻辑（默认参数、可变参等）。
func (vm *VM) callMethodDirect(obj *bytecode.Object, method *bytecode.Method, argCount int) InterpretResult {
	// 热点检测：记录函数调用
	fn := &bytecode.Function{
		Name:          method.Name,
		ClassName:     method.ClassName,
		SourceFile:    method.SourceFile,
		Arity:         method.Arity,
		MinArity:      method.MinArity,
		Chunk:         method.Chunk,
		LocalCount:    method.LocalCount,
		DefaultValues: method.DefaultValues,
	}
	vm.hotspotDetector.RecordFunctionCall(fn)
	
	// 创建闭包并调用
	closure := &bytecode.Closure{Function: fn}
	return vm.callOptimized(closure, argCount)
}

// invokeMethod 以“动态分派”的方式调用对象方法。
//
// 栈约定：
//   - 调用点（OpCallMethod）：argN...arg0, receiver
//   - receiver 位于参数之下，因此通过 peek(argCount) 取得
//
// 分派流程：
//   1) 特判 SuperArray：其方法是 VM 内建的，不走类方法表
//   2) 确认 receiver 为对象，否则报错
//   3) 查找方法实现：
//      - 优先尝试接口 VTable（近似 O(1)）
//      - 回退到按继承链 + 默认参数范围匹配（findMethodWithDefaults）
//   4) 检查访问权限（public/protected/private 等）
//   5) 将方法包装为函数闭包并调用（call），复用参数填充/可变参逻辑
//
// 维护者注意：
//   - name+argCount 是分派的关键维度（支持重载/默认参数范围）
//   - 该函数自身不刷新 frame/chunk；调用方（解释循环）在 OpCallMethod 后会刷新。
//   - callSiteIP 用于内联缓存，-1 表示不使用缓存
func (vm *VM) invokeMethod(name string, argCount int, callSiteIP int) InterpretResult {
	receiver := vm.peek(argCount)

	// 处理 SuperArray 内置方法
	if receiver.Type == bytecode.ValSuperArray {
		return vm.invokeSuperArrayMethod(name, argCount)
	}

	if receiver.Type != bytecode.ValObject {
		return vm.runtimeError(i18n.T(i18n.ErrOnlyObjectsHaveMethods))
	}
	
	obj := receiver.AsObject()
	
	// 内联缓存快速路径：检查是否有缓存命中
	if vm.icManager.IsEnabled() && callSiteIP >= 0 && vm.frameCount > 0 {
		currentFunc := vm.frames[vm.frameCount-1].Closure.Function
		ic := vm.icManager.GetMethodCache(currentFunc, callSiteIP)
		if ic != nil {
			if method, hit := ic.Lookup(obj.Class); hit {
				// 缓存命中：直接调用已缓存的方法
				return vm.callMethodDirect(obj, method, argCount)
			}
		}
	}
	
	// 尝试使用 VTable 优化接口方法查找
	method := vm.findMethodWithVTable(obj.Class, name, argCount)
	if method == nil {
		// 回退到传统查找
		method = vm.findMethodWithDefaults(obj.Class, name, argCount)
		if method == nil {
			return vm.runtimeError(i18n.T(i18n.ErrUndefinedMethod, name, argCount))
		}
	}
	
	// 更新内联缓存
	if vm.icManager.IsEnabled() && callSiteIP >= 0 && vm.frameCount > 0 {
		currentFunc := vm.frames[vm.frameCount-1].Closure.Function
		ic := vm.icManager.GetMethodCache(currentFunc, callSiteIP)
		if ic != nil {
			ic.Update(obj.Class, method)
		}
	}

	// 热点检测：记录函数调用
	vm.hotspotDetector.RecordFunctionCall(&bytecode.Function{Name: method.Name, ClassName: method.ClassName})

	// 检查方法访问权限
	if err := vm.checkMethodAccess(obj.Class, method); err != nil {
		return vm.runtimeError("%v", err)
	}

	// 创建方法的闭包，包含默认参数信息
	closure := &bytecode.Closure{
		Function: &bytecode.Function{
			Name:          method.Name,
			ClassName:     method.ClassName, // 设置类名用于堆栈跟踪
			SourceFile:    method.SourceFile,
			Arity:         method.Arity,
			MinArity:      method.MinArity,
			Chunk:         method.Chunk,
			LocalCount:    method.LocalCount,
			DefaultValues: method.DefaultValues,
		},
	}

	return vm.call(closure, argCount)
}

// findMethodWithVTable 使用 VTable 查找接口方法（快速路径）。
//
// 背景：
//   - 当类实现了接口时，可预先构建“接口方法 -> 实现方法”的映射（vtable）
//   - 调用时无需沿继承链查找，尤其适合接口多态调用场景
//
// 当前实现说明：
//   - class.VTables 可能包含多个接口的 vtable
//   - 这里做了线性扫描（接口数量通常很小），找到匹配的方法名后再检查参数范围
func (vm *VM) findMethodWithVTable(class *bytecode.Class, name string, argCount int) *bytecode.Method {
	// 遍历类的所有 VTable（为所有实现的接口）
	for _, vtable := range class.VTables {
		// 在 VTable 中查找方法名
		for _, entry := range vtable.Methods {
			if entry.MethodName == name && entry.ImplMethod != nil {
				// 检查参数数量是否匹配
				m := entry.ImplMethod
				if argCount >= m.MinArity && argCount <= m.Arity {
					return m
				}
			}
		}
	}
	return nil
}

// findMethodWithDefaults 查找方法实现，考虑默认参数范围（MinArity..Arity）。
//
// 该函数用于支持：
//   - 方法重载（同名不同参数个数）
//   - 默认参数（调用时 argCount 可能小于 Arity）
//
// 匹配规则：
//   - 在继承链上，从子类到父类依次查找 name 对应的方法列表
//   - 对每个候选方法 m，若 argCount 落在 [m.MinArity, m.Arity] 之间，则匹配成功
func (vm *VM) findMethodWithDefaults(class *bytecode.Class, name string, argCount int) *bytecode.Method {
	for c := class; c != nil; c = c.Parent {
		if methods, ok := c.Methods[name]; ok {
			for _, m := range methods {
				// 检查参数数量是否在有效范围内
				if argCount >= m.MinArity && argCount <= m.Arity {
					return m
				}
			}
		}
	}
	return nil
}

// invokeSuperArrayMethod 处理 SuperArray 的内置方法调用（VM 内建）。
//
// SuperArray 是“万能数组/有序字典”的运行时结构，为了性能与易用性，
// 其常用方法（len/keys/values/get/set/push/pop...）由 VM 直接实现。
//
// 栈约定：
//   - argN...arg0, receiver（receiver 为 SuperArray）
//   - 本方法会 pop 参数与 receiver，并 push 返回值
//
// 备注：该路径绕过了普通对象方法分派与权限检查。
func (vm *VM) invokeSuperArrayMethod(name string, argCount int) InterpretResult {
	// 收集参数（不包括 receiver）
	args := make([]bytecode.Value, argCount)
	for i := argCount - 1; i >= 0; i-- {
		args[i] = vm.pop()
	}
	receiver := vm.pop()
	sa := receiver.AsSuperArray()

	var result bytecode.Value

	switch name {
	case "len", "length":
		result = bytecode.NewInt(int64(sa.Len()))

	case "keys":
		keys := sa.Keys()
		newSa := bytecode.NewSuperArray()
		for _, k := range keys {
			newSa.Push(k)
		}
		result = bytecode.NewSuperArrayValue(newSa)

	case "values":
		values := sa.Values()
		newSa := bytecode.NewSuperArray()
		for _, v := range values {
			newSa.Push(v)
		}
		result = bytecode.NewSuperArrayValue(newSa)

	case "hasKey":
		if argCount < 1 {
			return vm.runtimeError("hasKey requires 1 argument")
		}
		result = bytecode.NewBool(sa.HasKey(args[0]))

	case "get":
		if argCount < 1 {
			return vm.runtimeError("get requires at least 1 argument")
		}
		if val, ok := sa.Get(args[0]); ok {
			result = val
		} else if argCount >= 2 {
			result = args[1] // default value
		} else {
			result = bytecode.NullValue
		}

	case "set":
		if argCount < 2 {
			return vm.runtimeError("set requires 2 arguments")
		}
		sa.Set(args[0], args[1])
		result = receiver // return self for chaining

	case "remove":
		if argCount < 1 {
			return vm.runtimeError("remove requires 1 argument")
		}
		result = bytecode.NewBool(sa.Remove(args[0]))

	case "push":
		if argCount < 1 {
			return vm.runtimeError("push requires 1 argument")
		}
		sa.Push(args[0])
		result = receiver // return self for chaining

	case "pop":
		if sa.Len() == 0 {
			result = bytecode.NullValue
		} else {
			lastIdx := sa.Len() - 1
			result = sa.Entries[lastIdx].Value
			// 移除最后一个元素
			key := sa.Entries[lastIdx].Key
			sa.Remove(key)
		}

	case "shift":
		if sa.Len() == 0 {
			result = bytecode.NullValue
		} else {
			result = sa.Entries[0].Value
			key := sa.Entries[0].Key
			sa.Remove(key)
		}

	case "unshift":
		if argCount < 1 {
			return vm.runtimeError("unshift requires 1 argument")
		}
		// 创建新数组，先添加新元素，再添加原有元素
		newSa := bytecode.NewSuperArray()
		newSa.Push(args[0])
		for _, entry := range sa.Entries {
			newSa.Set(entry.Key, entry.Value)
		}
		// 替换原数组内容
		sa.Entries = newSa.Entries
		sa.Index = newSa.Index
		sa.NextInt = newSa.NextInt
		result = receiver

	case "merge":
		if argCount < 1 || args[0].Type != bytecode.ValSuperArray {
			return vm.runtimeError("merge requires a SuperArray argument")
		}
		other := args[0].AsSuperArray()
		merged := sa.Copy()
		for _, entry := range other.Entries {
			merged.Set(entry.Key, entry.Value)
		}
		result = bytecode.NewSuperArrayValue(merged)

	case "slice":
		if argCount < 1 {
			return vm.runtimeError("slice requires at least 1 argument")
		}
		start := int(args[0].AsInt())
		end := sa.Len()
		if argCount >= 2 && args[1].AsInt() != -1 {
			end = int(args[1].AsInt())
		}
		if start < 0 {
			start = 0
		}
		if end > sa.Len() {
			end = sa.Len()
		}
		newSa := bytecode.NewSuperArray()
		for i := start; i < end; i++ {
			newSa.Push(sa.Entries[i].Value)
		}
		result = bytecode.NewSuperArrayValue(newSa)

	default:
		return vm.runtimeError("SuperArray has no method '%s'", name)
	}

	vm.push(result)
	return InterpretOK
}

// ============================================================================
// 错误与异常输出
// ============================================================================

// formatException 将 VM 异常格式化为接近 Java/C# 的输出格式。
//
// 输出包含：
//   - 异常类型（尽可能使用带命名空间的完整类名）
//   - 异常消息
//   - 调用栈（StackFrames），每帧包含函数名/类名/文件/行号
//   - 异常链（Cause），以 "Caused by:" 递归输出
//
// 备注：
//   - exc.Message 与对象字段 message 可能不同；若异常携带对象实例，优先使用对象字段。
//   - 该方法只做格式化，不会改变 VM 状态。
func (vm *VM) formatException(exc *bytecode.Exception) string {
	var result string
	
	// 获取消息
	message := exc.Message
	if exc.Object != nil {
		if msgVal, ok := exc.Object.Fields["message"]; ok {
			message = msgVal.AsString()
		}
	}
	
	// 获取完整的异常类型名（包括命名空间）
	typeName := exc.Type
	if exc.Object != nil && exc.Object.Class != nil {
		typeName = exc.Object.Class.FullName()
	}
	
	// 异常类型和消息
	result = fmt.Sprintf("%s: %s\n", typeName, message)
	
	// 堆栈跟踪
	for _, frame := range exc.StackFrames {
		if frame.ClassName != "" {
			result += fmt.Sprintf("    at %s.%s (%s:%d)\n", 
				frame.ClassName, frame.FunctionName, frame.FileName, frame.LineNumber)
		} else if frame.FileName != "" {
			result += fmt.Sprintf("    at %s (%s:%d)\n", 
				frame.FunctionName, frame.FileName, frame.LineNumber)
		} else {
			result += fmt.Sprintf("    at %s (line %d)\n", 
				frame.FunctionName, frame.LineNumber)
		}
	}
	
	// 异常链
	if exc.Cause != nil {
		result += "\nCaused by: " + vm.formatException(exc.Cause)
	}
	
	return result
}

// runtimeErrorWithException 以“异常”形式报告运行时错误并终止执行。
//
// 与 runtimeError 的区别：
//   - runtimeErrorWithException 的输入已经是结构化异常（带类型、堆栈、cause 等）
//   - runtimeError 接受格式化字符串，通常用于非异常的运行时错误路径
//
// 副作用：
//   - 设置 vm.hadError / vm.errorMessage
//   - 输出异常信息到 stdout/stderr（当前实现使用 fmt.Print）
func (vm *VM) runtimeErrorWithException(exc *bytecode.Exception) InterpretResult {
	vm.hadError = true
	vm.errorMessage = exc.Message
	
	// 输出格式化的异常信息
	fmt.Print(vm.formatException(exc))
	
	return InterpretRuntimeError
}

// runtimeError 报告一个运行时错误并终止执行（非 try-catch 语义的直接错误路径）。
//
// 说明：
//   - 该方法用于“无法/不走 throwTypedException 的错误”或最终未捕获异常的输出路径。
//   - 输出形式支持两种模式：
//       1) 传统输出：Java/C# 风格消息 + 堆栈
//       2) 增强输出：通过 internal/errors 的 Reporter 结构化输出（可带错误码）
//
// 副作用：
//   - 设置 vm.hadError / vm.errorMessage
//   - 输出错误信息（取决于 useEnhancedRuntimeErrors）
func (vm *VM) runtimeError(format string, args ...interface{}) InterpretResult {
	vm.hadError = true
	vm.errorMessage = fmt.Sprintf(format, args...)

	// 使用增强的错误报告（如果启用）
	if useEnhancedRuntimeErrors {
		frames := vm.captureStackTrace()
		vm.reportEnhancedError(vm.errorMessage, frames)
	} else {
		// 传统错误输出（Java/C# 风格）
		fmt.Printf("%s\n", vm.errorMessage)
		
		// 打印堆栈跟踪
		frames := vm.captureStackTrace()
		for _, frame := range frames {
			if frame.FileName != "" {
				fmt.Printf("    at %s (%s:%d)\n", frame.FunctionName, frame.FileName, frame.LineNumber)
			} else {
				fmt.Printf("    at %s (line %d)\n", frame.FunctionName, frame.LineNumber)
			}
		}
	}

	return InterpretRuntimeError
}

// reportEnhancedError 使用增强格式报告运行时错误（结构化错误 + 错误码）。
//
// 该模式会把 bytecode.StackFrame 转换为 errors.StackFrame，
// 并交由默认 Reporter 输出。常用于 IDE/诊断工具集成。
func (vm *VM) reportEnhancedError(message string, bcFrames []bytecode.StackFrame) {
	// 转换堆栈帧
	errFrames := make([]errors.StackFrame, len(bcFrames))
	for i, f := range bcFrames {
		errFrames[i] = errors.StackFrame{
			FunctionName: f.FunctionName,
			ClassName:    f.ClassName,
			FileName:     f.FileName,
			LineNumber:   f.LineNumber,
		}
	}

	// 创建运行时错误
	err := &errors.RuntimeError{
		Code:    errors.R0001, // 默认错误码，后续可以根据消息推断
		Level:   errors.LevelError,
		Message: message,
		Frames:  errFrames,
		Context: make(map[string]interface{}),
	}

	// 推断错误码
	err.Code = inferRuntimeErrorCode(message)

	// 使用报告器输出
	reporter := errors.GetDefaultReporter()
	reporter.ReportRuntimeError(err)
}

// inferRuntimeErrorCode 从错误消息中启发式推断错误码。
//
// 注意：
//   - 这是“弱推断”：依赖字符串包含关系，可能误判；
//   - 更可靠的方式是让抛错点直接携带错误码（未来可演进）。
// 当前逻辑优先覆盖常见类别：索引越界、除零、类型错误、转换失败、栈溢出等。
func inferRuntimeErrorCode(message string) string {
	msg := strings.ToLower(message)

	if strings.Contains(msg, "索引") || strings.Contains(msg, "index") || strings.Contains(msg, "越界") {
		return errors.R0100
	}
	if strings.Contains(msg, "除") && strings.Contains(msg, "零") {
		return errors.R0200
	}
	if strings.Contains(msg, "division") && strings.Contains(msg, "zero") {
		return errors.R0200
	}
	if strings.Contains(msg, "数字") || strings.Contains(msg, "number") || strings.Contains(msg, "operand") {
		return errors.R0201
	}
	if strings.Contains(msg, "转换") || strings.Contains(msg, "cast") {
		return errors.R0301
	}
	if strings.Contains(msg, "栈溢出") || strings.Contains(msg, "stack overflow") {
		return errors.R0400
	}
	if strings.Contains(msg, "死循环") || strings.Contains(msg, "execution limit") {
		return errors.R0401
	}
	if strings.Contains(msg, "未定义的变量") || strings.Contains(msg, "undefined variable") {
		return errors.R0500
	}

	return errors.R0001
}

// useEnhancedRuntimeErrors 控制是否启用增强运行时错误报告（结构化 Reporter）。
//
// 默认 false，以保持传统输出格式与最小依赖。
var useEnhancedRuntimeErrors = false

// EnableEnhancedRuntimeErrors 启用增强的运行时错误报告。
//
// 启用后，runtimeError 将通过 internal/errors.Reporter 输出结构化错误，
// 并附带 inferRuntimeErrorCode 推断出的错误码。
func EnableEnhancedRuntimeErrors() {
	useEnhancedRuntimeErrors = true
}

// DisableEnhancedRuntimeErrors 禁用增强的运行时错误报告，恢复传统输出。
func DisableEnhancedRuntimeErrors() {
	useEnhancedRuntimeErrors = false
}

// DefineGlobal 定义全局变量
func (vm *VM) DefineGlobal(name string, value bytecode.Value) {
	vm.globals[name] = value
}

// DefineClass 定义类
func (vm *VM) DefineClass(class *bytecode.Class) {
	vm.classes[class.Name] = class
}

// GetClass 获取类定义
func (vm *VM) GetClass(name string) *bytecode.Class {
	return vm.classes[name]
}

// DefineEnum 注册枚举
func (vm *VM) DefineEnum(enum *bytecode.Enum) {
	vm.enums[enum.Name] = enum
}

// GetError 获取错误信息
func (vm *VM) GetError() string {
	return vm.errorMessage
}

// ============================================================================
// 运行时类型系统（is/cast/类型兼容）
// ============================================================================

// getValueTypeName 返回一个值在 Sola 语义下的“运行时类型名”。
//
// 约定：
//   - 基本类型返回固定名：int/float/string/bool/null 等
//   - 对象返回其 class.Name（用于 is/catch 类型匹配）
//   - 固定数组与普通数组在类型名上都视为 "array"（兼容性设计）
//
// 该函数主要用于：
//   - OpCheckType（is 表达式）
//   - castValue 失败时生成错误信息
//   - checkValueType 的快速路径判断
func (vm *VM) getValueTypeName(v bytecode.Value) string {
	switch v.Type {
	case bytecode.ValNull:
		return "null"
	case bytecode.ValBool:
		return "bool"
	case bytecode.ValInt:
		return "int"
	case bytecode.ValFloat:
		return "float"
	case bytecode.ValString:
		return "string"
	case bytecode.ValArray:
		return "array"
	case bytecode.ValFixedArray:
		return "array"
	case bytecode.ValBytes:
		return "bytes"
	case bytecode.ValMap:
		return "map"
	case bytecode.ValObject:
		obj := v.AsObject()
		if obj != nil && obj.Class != nil {
			return obj.Class.Name
		}
		return "unknown"
	case bytecode.ValClosure:
		return "function"
	case bytecode.ValIterator:
		return "iterator"
	default:
		return "unknown"
	}
}

// checkValueType 判断值 v 是否“可赋值/可视为” expectedType。
//
// 支持的类型表达式：
//   - 单一类型：int / string / MyClass
//   - 联合类型：A|B|C（递归检查任一分支）
//
// 兼容性规则（运行时宽松策略）：
//   - null 被视为可赋值给任何类型（运行时不做非空约束）
//   - float 兼容 int（数值提升）
//   - array 与 bytes 互相兼容（为历史/语义便利做的折中）
//   - dynamic/unknown 匹配所有类型
//
// 对象类型：
//   - 通过 checkClassHierarchy 检查继承链与接口实现关系
func (vm *VM) checkValueType(v bytecode.Value, expectedType string) bool {
	actualType := vm.getValueTypeName(v)
	
	// 直接匹配
	if actualType == expectedType {
		return true
	}
	
	// null 可以匹配任何可空类型（以 ? 开头的类型名会被处理掉 ?）
	if actualType == "null" {
		return true // null 可以赋值给任何类型（在运行时）
	}
	
	// 联合类型检查 (type1|type2|type3)
	if strings.Contains(expectedType, "|") {
		expectedTypes := strings.Split(expectedType, "|")
		for _, t := range expectedTypes {
			if vm.checkValueType(v, strings.TrimSpace(t)) {
				return true
			}
		}
		return false
	}
	
	// 数值类型兼容性
	switch expectedType {
	case "int", "i8", "i16", "i32", "i64":
		return actualType == "int"
	case "float", "f32", "f64":
		return actualType == "float" || actualType == "int"
	case "number":
		return actualType == "int" || actualType == "float"
	case "dynamic", "unknown":
		return true
	case "array":
		// bytes 也被视为数组的兼容类型
		return actualType == "array" || actualType == "bytes"
	case "bytes":
		// bytes 类型也接受 array（用于兼容性）
		return actualType == "bytes" || actualType == "array"
	}
	
	// 对象类型检查（包括继承关系）
	if v.Type == bytecode.ValObject {
		obj := v.AsObject()
		if obj != nil && obj.Class != nil {
			// 检查是否是该类型或其子类
			return vm.checkClassHierarchy(obj.Class, expectedType)
		}
	}
	
	return false
}


// checkClassHierarchy 判断 class 是否满足 typeName（包括继承与接口）。
//
// 规则：
//   - 若 class.Name == typeName：直接匹配
//   - 否则沿父类链向上查找
//   - 同时检查每层的 Implements 列表（接口名匹配）
//
// 注意：这里不处理泛型参数（运行时类型擦除）。
func (vm *VM) checkClassHierarchy(class *bytecode.Class, typeName string) bool {
	if class == nil {
		return false
	}
	
	// 检查当前类
	if class.Name == typeName {
		return true
	}
	
	// 检查父类链
	for c := class; c != nil; c = c.Parent {
		if c.Name == typeName {
			return true
		}
		// 检查接口
		for _, iface := range c.Implements {
			if iface == typeName {
				return true
			}
		}
	}
	
	return false
}

// castValue 尝试将值 v 转换为 targetType。
//
// 返回值：
//   - (converted, true)  表示转换成功
//   - (zero, false)      表示转换失败（调用方决定抛错或返回 null）
//
// 语义说明：
//   - castValue 是“运行时转换”，主要服务 OpCast / OpCastSafe。
//   - 这里的转换规则偏实用：例如 bool->int，null->0 等。
//   - 对 string->int 的转换要求严格（见下方 ParseInt 注释），以避免隐式截断造成类型安全问题。
func (vm *VM) castValue(v bytecode.Value, targetType string) (bytecode.Value, bool) {
	switch targetType {
	case "int", "i8", "i16", "i32", "i64":
		switch v.Type {
		case bytecode.ValInt:
			return v, true
		case bytecode.ValFloat:
			return bytecode.NewInt(int64(v.AsFloat())), true
		case bytecode.ValString:
			// 【重要】严格解析字符串为整数
			// 必须使用 strconv.ParseInt 进行严格验证，而不是 fmt.Sscanf
			// fmt.Sscanf("%d") 会解析到第一个非数字字符为止，导致以下错误行为：
			//   - "123abc" 会解析成功得到 123（应该失败，因为包含非数字字符）
			//   - "3.14" 会解析成功得到 3（应该失败，因为是浮点数）
			// strconv.ParseInt 会对整个字符串进行严格验证：
			//   - "123abc" -> 失败
			//   - "3.14" -> 失败
			//   - "123" -> 成功
			// 【警告】请勿将此修改回 fmt.Sscanf，否则会引入类型安全漏洞！
			s := strings.TrimSpace(v.AsString())
			i, err := strconv.ParseInt(s, 10, 64)
			if err == nil {
				return bytecode.NewInt(i), true
			}
			return bytecode.Value{}, false
		case bytecode.ValBool:
			if v.AsBool() {
				return bytecode.NewInt(1), true
			}
			return bytecode.NewInt(0), true
		case bytecode.ValNull:
			return bytecode.NewInt(0), true
		}
		
	case "float", "f32", "f64":
		switch v.Type {
		case bytecode.ValFloat:
			return v, true
		case bytecode.ValInt:
			return bytecode.NewFloat(float64(v.AsInt())), true
		case bytecode.ValString:
			var f float64
			_, err := fmt.Sscanf(v.AsString(), "%f", &f)
			if err == nil {
				return bytecode.NewFloat(f), true
			}
			return bytecode.Value{}, false
		case bytecode.ValNull:
			return bytecode.NewFloat(0.0), true
		}
		
	case "string":
		return bytecode.NewString(v.String()), true
		
	case "bool":
		return bytecode.NewBool(v.IsTruthy()), true
		
	case "array":
		if v.Type == bytecode.ValArray || v.Type == bytecode.ValFixedArray {
			return v, true
		}
		// 将单个值包装为数组
		return bytecode.NewArray([]bytecode.Value{v}), true
	}
	
	return bytecode.Value{}, false
}

