# VM + JIT 实现检查清单

此文档用于跟踪实现进度，确保多个对话之间的一致性。

---

## Phase 1: Value 重构

### 1.1 设计新 Value 结构

**文件**: `internal/bytecode/value_v2.go`

**选择的方案**: Tagged Union (双字段)

```go
type Value struct {
    tag uint8          // 类型标签
    _   [7]byte        // 填充对齐
    num uint64         // 数值类型直接存这里 (int64/float64/bool)
    ptr unsafe.Pointer // 指针类型存这里 (string/object/array)
}
```

**原因**: 
- 比 NaN Boxing 更简单易懂
- 调试友好
- 性能差异不大 (都是 16 字节)

### 1.2 实现清单

```
□ 定义 Value 结构体
□ 定义 ValueType 常量 (ValInt, ValFloat, ValBool, ValNull, ValString, ValObject, ValArray, ValSuperArray)

□ 实现 NewInt(n int64) Value
    - 直接存入 num 字段
    - 设置 tag = ValInt
    - 无堆分配

□ 实现 NewFloat(f float64) Value
    - 使用 math.Float64bits 转换为 uint64
    - 存入 num 字段
    - 设置 tag = ValFloat
    - 无堆分配

□ 实现 NewBool(b bool) Value
    - true 存为 1，false 存为 0
    - 设置 tag = ValBool
    - 无堆分配

□ 实现 NewNull() Value
    - 返回预定义常量
    - tag = ValNull
    - 无堆分配

□ 实现 NewString(s string) Value
    - 将 string header 指针存入 ptr 字段
    - 设置 tag = ValString
    - 注意: Go 的 string 本身就是指针+长度，这里存的是 *string

□ 实现 NewObject(obj *Object) Value
□ 实现 NewArray(arr *Array) Value  
□ 实现 NewSuperArray(arr *SuperArray) Value

□ 实现类型检查方法
    □ Type() ValueType
    □ IsInt() bool
    □ IsFloat() bool
    □ IsBool() bool
    □ IsNull() bool
    □ IsString() bool
    □ IsObject() bool
    □ IsArray() bool
    □ IsSuperArray() bool

□ 实现值提取方法
    □ AsInt() int64
    □ AsFloat() float64
    □ AsBool() bool
    □ AsString() string
    □ AsObject() *Object
    □ AsArray() *Array
    □ AsSuperArray() *SuperArray

□ 实现 String() 方法 (用于调试)
□ 实现 Equals(other Value) bool

□ 预定义常量
    □ NullValue
    □ TrueValue
    □ FalseValue
    □ ZeroIntValue
    □ ZeroFloatValue
```

### 1.3 迁移指南

旧代码:
```go
val := Value{Type: ValInt, Data: int64(42)}
n := val.Data.(int64)
```

新代码:
```go
val := NewInt(42)
n := val.AsInt()
```

**迁移步骤**:
1. 先实现新 Value，保持旧 Value 可用
2. 逐个模块迁移
3. 移除旧 Value

---

## Phase 2: Helper 基础设施

### 2.1 Helper 注册表

**文件**: `internal/jit/helper_registry.go`

```
□ 定义 HelperFunc 类型
□ 定义 HelperRegistry 结构
□ 实现 RegisterHelper(name string, fn interface{})
□ 实现 GetHelperAddr(name string) uintptr
□ 使用 reflect 获取函数地址
□ 处理方法 vs 函数的区别
```

### 2.2 SuperArray Helper

**文件**: `internal/jit/superarray_helper.go`

```
□ func Helper_SA_New() *SuperArray
    - 使用预分配容量 make([]Entry, 0, 8)
    - //go:noinline 确保有稳定地址

□ func Helper_SA_Get(arr *SuperArray, key Value) Value
    - 整数键快速路径
    - 字符串键查找
    - 返回 NullValue 如果不存在

□ func Helper_SA_Set(arr *SuperArray, key, value Value)
    - 整数键插入/更新
    - 字符串键插入/更新
    - 维护 Index 映射

□ func Helper_SA_Len(arr *SuperArray) int

□ func Helper_SA_Delete(arr *SuperArray, key Value) bool

□ func Helper_SA_Has(arr *SuperArray, key Value) bool
```

### 2.3 其他 Helper

```
□ Helper_Add(a, b Value) Value
□ Helper_Sub(a, b Value) Value
□ Helper_Mul(a, b Value) Value
□ Helper_Div(a, b Value) Value
□ Helper_Mod(a, b Value) Value
□ Helper_Neg(a Value) Value

□ Helper_StringConcat(a, b Value) Value
□ Helper_StringLen(s Value) int
□ Helper_StringIndex(s Value, i int) Value

□ Helper_Compare(a, b Value) int
□ Helper_Equal(a, b Value) bool
```

---

## Phase 3: VM 重构

### 3.1 VM 结构重构

**文件**: `internal/vm/vm_v2.go`

```
□ 定义新 VMState 结构
    □ regs [256]Value      // 寄存器文件
    □ stack []Value        // 溢出栈
    □ sp int               // 栈指针 (指向 regs)
    □ ip int               // 指令指针
    □ fp int               // 帧指针
    □ chunk *Chunk         // 当前字节码块
    □ globals []Value      // 全局变量
    □ upvalues []*Upvalue  // 闭包变量

□ 实现分派表
    □ var dispatchTable [256]func(*VMState)
    □ 初始化所有操作码处理函数

□ 实现主循环
    □ func (vm *VMState) Run()
    □ 使用分派表而非 switch
```

### 3.2 操作码实现

```
□ 常量加载
    □ execPush
    □ execPushNull
    □ execPushTrue
    □ execPushFalse

□ 局部变量
    □ execLoadLocal
    □ execStoreLocal

□ 全局变量
    □ execLoadGlobal
    □ execStoreGlobal

□ 算术运算 (快速路径 + 慢速路径)
    □ execAdd
    □ execSub
    □ execMul
    □ execDiv
    □ execMod
    □ execNeg

□ 比较运算
    □ execEqual
    □ execNotEqual
    □ execLess
    □ execLessEqual
    □ execGreater
    □ execGreaterEqual

□ 逻辑运算
    □ execNot
    □ execAnd
    □ execOr

□ 跳转
    □ execJump
    □ execJumpIfFalse
    □ execJumpIfTrue
    □ execLoop

□ 函数调用
    □ execCall
    □ execReturn
    □ execReturnNull

□ SuperArray 操作
    □ execSuperArrayNew
    □ execSuperArrayGet
    □ execSuperArraySet

□ 对象操作
    □ execNewObject
    □ execGetField
    □ execSetField
    □ execCallMethod
```

---

## Phase 4: JIT 重构

### 4.1 移除 SuperArray 禁用

**文件**: `internal/jit/bridge.go`

```
□ 修改 CanJITWithLevel 函数
    □ 将 OpSuperArrayNew/Get/Set 从禁用列表移除
    □ 改为降级到 JITWithCalls 级别
```

### 4.2 IR 层修改

**文件**: `internal/jit/ir.go`

```
□ 添加 IR_CALL 指令类型
□ 添加 IR_CALL_HELPER 指令类型
□ 定义调用约定结构
```

### 4.3 IR Builder 修改

**文件**: `internal/jit/builder.go`

```
□ 实现 buildHelperCall(name string, args ...int) int
□ 实现 buildSuperArrayNew() int
□ 实现 buildSuperArrayGet(arr, key int) int
□ 实现 buildSuperArraySet(arr, key, value int)
□ 修改字节码转 IR 时处理 SuperArray 操作
```

### 4.4 代码生成修改

**文件**: `internal/jit/codegen_amd64.go`

```
□ 实现 emitHelperCall(name string, args []Reg) Reg
    □ 保存调用者保存寄存器
    □ 按 Go 调用约定准备参数
    □ 生成 CALL 指令
    □ 处理返回值
    □ 恢复寄存器

□ 处理 IR_CALL_HELPER 指令的代码生成
```

---

## Phase 5: 测试与优化

### 5.1 基准测试

**文件**: `benchmark/refactor_bench_test.go`

```
□ BenchmarkIntLoopGo         // Go 原生
□ BenchmarkIntLoopInterp     // Sola 解释器
□ BenchmarkIntLoopJIT        // Sola JIT

□ BenchmarkSuperArrayGo      // Go map 模拟
□ BenchmarkSuperArrayPHP     // PHP 参考 (如果可能)
□ BenchmarkSuperArraySola    // Sola SuperArray

□ BenchmarkMixedCodeJIT      // 混合代码 (循环 + SuperArray)
```

### 5.2 正确性测试

```
□ Value 类型测试
    □ TestValueInt
    □ TestValueFloat
    □ TestValueBool
    □ TestValueString
    □ TestValueNull
    □ TestValueObject
    □ TestValueArray
    □ TestValueSuperArray
    □ TestValueConversion

□ Helper 测试
    □ TestHelperAdd
    □ TestHelperSub
    □ TestHelperSuperArrayCRUD

□ VM 测试
    □ TestVMArithmetic
    □ TestVMComparison
    □ TestVMControlFlow
    □ TestVMFunctionCall
    □ TestVMSuperArray

□ JIT 测试
    □ TestJITSimpleFunction
    □ TestJITWithSuperArray
    □ TestJITHelperCall
    □ TestJITCorrectness (与解释器对比)
```

---

## 进度跟踪

| Phase | 状态 | 完成日期 | 备注 |
|-------|------|----------|------|
| 1.1 Value 设计 | 未开始 | | |
| 1.2 Value 实现 | 未开始 | | |
| 1.3 Value 迁移 | 未开始 | | |
| 2.1 Helper 注册 | 未开始 | | |
| 2.2 SA Helper | 未开始 | | |
| 3.1 VM 结构 | 未开始 | | |
| 3.2 VM 操作码 | 未开始 | | |
| 4.1 JIT 解禁 SA | 已完成 | 2026-01-12 | bridge.go 已修改 |
| 4.2 IR 修改 | 未开始 | | |
| 4.3 IR Builder | 未开始 | | |
| 4.4 代码生成 | 未开始 | | |
| 5.1 基准测试 | 未开始 | | |
| 5.2 正确性测试 | 未开始 | | |

---

## 已知问题

1. `superarray_jit.go` 中的类型 Profile 收集还未集成到解释器
2. Helper 函数地址获取的 `GetHelperFuncPtr` 返回 0 (占位符)
3. IR Builder 的 Helper 调用生成尚未实现

---

## 后续对话启动指令

在新对话中，可以使用以下指令启动工作：

```
请继续 VM+JIT 重构工作。参考文档：
- docs/VM_JIT_REFACTOR_PLAN.md (总体设计)
- docs/VM_JIT_IMPLEMENTATION_CHECKLIST.md (实现清单)

当前进度：[查看检查清单中的进度跟踪表]

下一步任务：[根据检查清单选择]
```
