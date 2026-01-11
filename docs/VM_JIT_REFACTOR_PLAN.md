# VM + JIT 重构计划

## 1. 问题分析

### 1.1 当前性能问题

| 问题 | 原因 | 影响 |
|------|------|------|
| VM 解释执行慢 | 大量 interface{} 装箱/拆箱 | 每次操作都有堆分配 |
| JIT 生成代码效率低 | 未充分利用类型信息 | 生成的代码不够紧凑 |
| SuperArray 完全禁用 JIT | 动态类型无法特化 | 整个函数退化为解释执行 |
| Value 类型开销大 | `Data interface{}` 设计 | 基本类型也要堆分配 |

### 1.2 性能目标

```
目标：达到 Go 原生性能的 50%

基准参考（整数循环 1000 万次）:
- Go 原生:     ~10ms
- 当前 Sola:   ~500ms+ (50x 慢)
- 目标 Sola:   ~20ms (2x 慢)

SuperArray 性能目标:
- PHP 数组:    ~100ms
- 目标 Sola:   ~150ms (1.5x PHP)
```

### 1.3 关键认识：Go 能编译 SuperArray 的原因

```
Go 编译 SuperArray 实现代码:
┌─────────────────────────────────────────────────────┐
│ // Go 编译器知道所有类型                             │
│ type SuperArray struct {                            │
│     Entries []SuperArrayEntry  // 类型已知          │
│     Index   map[string]int     // 类型已知          │
│ }                                                   │
│                                                     │
│ func (sa *SuperArray) Get(key Value) Value {        │
│     // Go 编译器为这段代码生成高效机器码            │
│     // 因为它知道 Entries 是 slice，Index 是 map   │
│ }                                                   │
└─────────────────────────────────────────────────────┘

Sola JIT 编译 SuperArray 使用代码:
┌─────────────────────────────────────────────────────┐
│ $arr = [1, "hello", "key" => $obj];                 │
│ $x = $arr[0];  // JIT 不知道返回什么类型            │
│                                                     │
│ 字节码: OpSuperArrayGet                             │
│ JIT 看到的: "从某个数组取某个元素"                   │
│ JIT 不知道的: 键类型、值类型、数组内部结构          │
└─────────────────────────────────────────────────────┘
```

**核心洞察**: Go 编译的是"实现代码"（类型完整），JIT 编译的是"使用代码"（类型未知）。

**解决方案**: JIT 不需要"编译" SuperArray，只需要**调用**预编译好的 Go 函数。

---

## 2. 总体架构

### 2.1 新架构设计

```
┌─────────────────────────────────────────────────────────────────┐
│                         执行引擎架构                             │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌─────────────┐    ┌─────────────┐    ┌─────────────────────┐ │
│  │   解释器    │    │   Tier-1    │    │      Tier-2         │ │
│  │ (Baseline)  │ →  │   JIT       │ →  │   JIT (优化)        │ │
│  │ + Profile   │    │ (快速编译)  │    │  (类型特化)         │ │
│  └─────────────┘    └─────────────┘    └─────────────────────┘ │
│        ↓                  ↓                     ↓               │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │                    统一 Helper 层                           ││
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐   ││
│  │  │ 算术运算 │  │ 字符串   │  │SuperArray│  │ 对象操作 │   ││
│  │  │ Helper   │  │ Helper   │  │ Helper   │  │ Helper   │   ││
│  │  └──────────┘  └──────────┘  └──────────┘  └──────────┘   ││
│  └─────────────────────────────────────────────────────────────┘│
│                              ↓                                  │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │                    优化的 Value 表示                        ││
│  │  NaN Boxing / Tagged Union (基本类型无堆分配)              ││
│  └─────────────────────────────────────────────────────────────┘│
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### 2.2 核心原则

1. **基本类型零分配**: int/float/bool 直接存在栈上或寄存器中
2. **Helper 统一调用**: 复杂操作通过预编译的 Go 函数完成
3. **渐进式优化**: 解释 → 快速JIT → 优化JIT
4. **部分 JIT**: 函数内的 SuperArray 操作调用 Helper，其他部分仍可优化

---

## 3. 模块设计

### 3.1 模块一：Value 重构 (P0 优先级)

**目标**: 基本类型无堆分配

**当前设计问题**:
```go
type Value struct {
    Type ValueType
    Data interface{}  // 每次存 int 都要堆分配！
}
```

**新设计 - Tagged Pointer / NaN Boxing**:
```go
// 方案 A: Tagged Pointer (简单，64位系统)
type Value struct {
    bits uint64  // 低 3 位是 tag，高 61 位是数据或指针
}

const (
    tagInt    = 0b001  // 整数 (61位有符号整数)
    tagFloat  = 0b010  // 浮点数 (使用 NaN boxing)
    tagBool   = 0b011  // 布尔值
    tagNull   = 0b100  // null
    tagPtr    = 0b000  // 指针 (对象、字符串、数组等)
)

func NewInt(n int64) Value {
    // 无堆分配！
    return Value{bits: uint64(n<<3) | tagInt}
}

func (v Value) IsInt() bool {
    return v.bits&0b111 == tagInt
}

func (v Value) AsInt() int64 {
    return int64(v.bits) >> 3  // 算术右移保留符号
}
```

**方案 B: 双字段 (更简单，兼容性好)**:
```go
type Value struct {
    tag  uint8   // 类型标签
    num  uint64  // 数值 (int64/float64 直接存这里)
    ptr  unsafe.Pointer  // 指针 (对象、字符串等)
}

// int/float/bool 使用 num 字段，无堆分配
// 对象/字符串/数组 使用 ptr 字段
```

**文件**: `internal/bytecode/value_v2.go`

**接口变更**:
```go
// 必须保持的接口 (向后兼容)
func NewInt(n int64) Value
func NewFloat(f float64) Value
func NewBool(b bool) Value
func NewString(s string) Value
func NewNull() Value

func (v Value) IsInt() bool
func (v Value) AsInt() int64
func (v Value) IsFloat() bool
func (v Value) AsFloat() float64
// ... 其他类型
```

---

### 3.2 模块二：VM 重构 (P0 优先级)

**目标**: 减少解释执行开销

**当前问题**:
```go
func (vm *VM) run() {
    for {
        op := vm.readOp()  // 每次都要检查边界
        switch op {
        case OpAdd:
            b := vm.pop()   // interface{} 操作
            a := vm.pop()   // interface{} 操作
            // 类型检查...
            vm.push(result) // interface{} 操作
        }
    }
}
```

**新设计 - 直接线程化 + 寄存器式**:
```go
// 使用固定大小的寄存器文件
type VMState struct {
    regs  [256]Value  // 寄存器 (栈顶缓存)
    stack []Value     // 溢出栈
    sp    int         // 栈指针
    ip    int         // 指令指针
    
    // 快速访问
    chunk  *Chunk
    code   []byte
    consts []Value
}

// 使用计算跳转表 (computed goto 模拟)
var dispatchTable = [256]func(*VMState){
    OpAdd:    execAdd,
    OpSub:    execSub,
    OpMul:    execMul,
    // ...
}

func (vm *VMState) run() {
    for vm.ip < len(vm.code) {
        op := vm.code[vm.ip]
        dispatchTable[op](vm)  // 直接调用，无 switch
    }
}

// 算术运算内联
func execAdd(vm *VMState) {
    vm.ip++
    // 假设都是 int (最常见情况)
    b := vm.regs[vm.sp-1]
    a := vm.regs[vm.sp-2]
    
    if a.IsInt() && b.IsInt() {
        // 快速路径：整数加法
        vm.regs[vm.sp-2] = NewInt(a.AsInt() + b.AsInt())
        vm.sp--
        return
    }
    
    // 慢速路径
    execAddSlow(vm, a, b)
}
```

**文件**: `internal/vm/vm_v2.go`

---

### 3.3 模块三：JIT 重构 (P1 优先级)

**目标**: 生成高效代码 + 支持 Helper 调用

#### 3.3.1 JIT 编译策略

```
┌───────────────────────────────────────────────────────────────┐
│                     JIT 编译流水线                            │
├───────────────────────────────────────────────────────────────┤
│                                                               │
│  字节码 → IR 构建 → 优化Pass → 寄存器分配 → 代码生成         │
│                                                               │
│  ┌─────────────────────────────────────────────────────────┐ │
│  │                    IR 优化 Pass                          │ │
│  │  1. 类型推断 (根据 Profile 信息)                        │ │
│  │  2. 常量折叠                                            │ │
│  │  3. 死代码消除                                          │ │
│  │  4. Helper 调用内联                                     │ │
│  │  5. 循环优化                                            │ │
│  └─────────────────────────────────────────────────────────┘ │
│                                                               │
└───────────────────────────────────────────────────────────────┘
```

#### 3.3.2 Helper 调用机制

**核心设计**: JIT 代码可以调用预编译的 Go 函数

```go
// Helper 函数注册表
type HelperRegistry struct {
    funcs map[string]uintptr  // 函数名 → 函数地址
}

// 注册 Helper
func init() {
    RegisterHelper("SuperArray_Get", reflect.ValueOf(SuperArrayGet).Pointer())
    RegisterHelper("SuperArray_Set", reflect.ValueOf(SuperArraySet).Pointer())
    RegisterHelper("String_Concat", reflect.ValueOf(StringConcat).Pointer())
    // ...
}

// JIT 生成调用代码
func (c *Codegen) emitHelperCall(name string, args ...Reg) Reg {
    addr := GetHelperAddr(name)
    // 生成: CALL addr
    // 按照 Go 调用约定传参
}
```

**调用约定**:
```
AMD64 Go 调用约定:
- 参数: 栈传递 (Go 1.17+ 可用寄存器)
- 返回值: AX (或栈)
- 调用者保存: 所有寄存器

JIT 调用 Helper 时:
1. 保存活跃寄存器到栈
2. 按 Go 约定传参
3. CALL helper_addr
4. 恢复寄存器
5. 处理返回值
```

**文件**: `internal/jit/helper.go`, `internal/jit/codegen_amd64.go`

---

### 3.4 模块四：SuperArray JIT 支持 (P1 优先级)

**目标**: SuperArray 操作通过 Helper 执行，不阻塞整个函数的 JIT

#### 3.4.1 设计原理

```
当前行为 (错误):
┌─────────────────────────────────────────────────────┐
│ function test() {                                   │
│     $sum = 0;                   // 可以 JIT        │
│     $arr = [1, 2, 3];           // SuperArray!     │
│     for ($i = 0; $i < 100; $i++) {                 │
│         $sum += $arr[$i % 3];   // SuperArray!     │
│     }                                              │
│     return $sum;                                    │
│ }                                                   │
│                                                     │
│ 结果: 整个函数禁用 JIT，全部解释执行               │
└─────────────────────────────────────────────────────┘

新行为 (正确):
┌─────────────────────────────────────────────────────┐
│ function test() {                                   │
│     $sum = 0;                   // JIT: MOV reg, 0 │
│     $arr = [1, 2, 3];           // JIT: CALL SA_New│
│     for ($i = 0; $i < 100; $i++) {  // JIT: 循环  │
│         $sum += $arr[$i % 3];   // JIT: CALL SA_Get│
│     }                                              │
│     return $sum;                 // JIT: RET      │
│ }                                                   │
│                                                     │
│ 结果: 整个函数 JIT，SuperArray 操作调用 Helper     │
└─────────────────────────────────────────────────────┘
```

#### 3.4.2 SuperArray Helper 函数

```go
// internal/jit/superarray_helper.go

// 这些函数由 Go 编译器编译，性能等同于 Go 原生代码
// JIT 代码直接 CALL 这些函数

//go:noinline  // 防止内联，确保有稳定的函数地址
func SA_New() *SuperArray {
    return &SuperArray{
        Entries: make([]Entry, 0, 8),
        Index:   make(map[string]int, 8),
    }
}

//go:noinline
func SA_Get(arr *SuperArray, key Value) Value {
    // 这是 Go 代码，编译器完全知道类型
    // 生成的代码与普通 Go 代码一样高效
    if key.IsInt() {
        idx := key.AsInt()
        if idx >= 0 && idx < int64(len(arr.Entries)) {
            return arr.Entries[idx].Value
        }
    }
    // ... 字符串键查找
    return NullValue
}

//go:noinline
func SA_Set(arr *SuperArray, key, value Value) {
    // 同样是高效的 Go 代码
}

//go:noinline
func SA_Len(arr *SuperArray) int {
    return len(arr.Entries)
}
```

#### 3.4.3 JIT 代码生成

```go
// 当 JIT 遇到 OpSuperArrayGet 时
func (b *IRBuilder) buildSuperArrayGet(arrReg, keyReg int) int {
    // 生成 Helper 调用
    resultReg := b.allocReg()
    
    // IR: result = CALL SA_Get(arr, key)
    b.emit(IR_CALL, "SA_Get", resultReg, arrReg, keyReg)
    
    return resultReg
}

// 代码生成阶段
func (c *Codegen) emitCall(instr *IRInstr) {
    // 保存调用者保存寄存器
    c.saveCallerSaved()
    
    // 准备参数 (按 Go 调用约定)
    c.prepareArgs(instr.Args)
    
    // CALL helper_addr
    helperAddr := GetHelperAddr(instr.FuncName)
    c.emit(CALL, helperAddr)
    
    // 获取返回值
    c.moveResult(instr.Dst)
    
    // 恢复寄存器
    c.restoreCallerSaved()
}
```

---

### 3.5 模块五：Profile 引导优化 (P2 优先级)

**目标**: 收集运行时信息，指导优化

```go
// 类型 Profile
type TypeProfile struct {
    IntCount    int64
    FloatCount  int64
    StringCount int64
    ObjectCount int64
    NullCount   int64
}

// 在解释器中收集
func (vm *VM) execAdd() {
    a, b := vm.pop2()
    
    // 收集类型信息
    if vm.profiling {
        vm.recordType(vm.ip, a.Type, b.Type)
    }
    
    // 执行加法...
}

// JIT 使用 Profile 信息
func (b *IRBuilder) buildAdd(aReg, bReg int) int {
    profile := GetProfile(b.currentPC)
    
    if profile.IntCount > profile.Total() * 0.95 {
        // 95% 是整数，生成特化代码
        return b.buildIntAdd(aReg, bReg)
    }
    
    // 生成通用代码
    return b.buildGenericAdd(aReg, bReg)
}
```

---

## 4. 实现步骤

### Phase 1: 基础设施 (预计 2-3 天)

```
□ 1.1 Value 重构
    □ 设计新的 Value 结构 (Tagged Union)
    □ 实现基本类型构造函数 (NewInt, NewFloat, NewBool)
    □ 实现类型检查函数 (IsInt, IsFloat, IsBool)
    □ 实现值提取函数 (AsInt, AsFloat, AsBool)
    □ 迁移现有代码使用新 API
    □ 基准测试验证性能提升

□ 1.2 Helper 基础设施
    □ 实现 Helper 注册表
    □ 实现函数地址获取
    □ 实现 SuperArray Helper 函数
    □ 单元测试
```

### Phase 2: VM 重构 (预计 3-4 天)

```
□ 2.1 VM 核心重构
    □ 实现寄存器式 VM 结构
    □ 实现直接分派表
    □ 优化热路径 (算术运算)
    □ 基准测试

□ 2.2 集成 Profile
    □ 添加类型收集
    □ 添加分支统计
    □ 添加热点检测
```

### Phase 3: JIT 重构 (预计 5-7 天)

```
□ 3.1 IR 层重构
    □ 简化 IR 指令集
    □ 添加 Helper Call 指令
    □ 实现类型推断 Pass

□ 3.2 代码生成重构
    □ 实现 Helper 调用生成
    □ 优化寄存器分配
    □ 实现常见模式特化

□ 3.3 SuperArray 支持
    □ 移除 SuperArray 禁用检查
    □ 实现 SuperArray IR 生成
    □ 测试混合代码
```

### Phase 4: 优化与测试 (预计 2-3 天)

```
□ 4.1 性能优化
    □ Profile 引导优化
    □ 循环优化
    □ 内联优化

□ 4.2 测试
    □ 正确性测试
    □ 性能基准测试
    □ 与 Go 对比测试
    □ 与 PHP 对比测试 (SuperArray)
```

---

## 5. 接口定义

### 5.1 Value 接口

```go
// internal/bytecode/value_v2.go

type Value struct {
    // 内部实现（不对外暴露）
}

// 构造函数
func NewInt(n int64) Value
func NewFloat(f float64) Value
func NewBool(b bool) Value
func NewNull() Value
func NewString(s string) Value
func NewObject(obj *Object) Value
func NewArray(arr *Array) Value
func NewSuperArray(arr *SuperArray) Value

// 类型检查
func (v Value) Type() ValueType
func (v Value) IsInt() bool
func (v Value) IsFloat() bool
func (v Value) IsBool() bool
func (v Value) IsNull() bool
func (v Value) IsString() bool
func (v Value) IsObject() bool
func (v Value) IsArray() bool
func (v Value) IsSuperArray() bool

// 值提取
func (v Value) AsInt() int64          // panic if not int
func (v Value) AsFloat() float64      // panic if not float
func (v Value) AsBool() bool          // panic if not bool
func (v Value) AsString() string      // panic if not string
func (v Value) AsObject() *Object     // panic if not object
func (v Value) AsArray() *Array       // panic if not array
func (v Value) AsSuperArray() *SuperArray

// 安全提取
func (v Value) TryInt() (int64, bool)
func (v Value) TryFloat() (float64, bool)
// ...
```

### 5.2 Helper 接口

```go
// internal/jit/helper.go

// Helper 注册
func RegisterHelper(name string, fn interface{})
func GetHelperAddr(name string) uintptr

// 内置 Helper
func Helper_Add(a, b Value) Value
func Helper_Sub(a, b Value) Value
func Helper_Mul(a, b Value) Value
func Helper_Div(a, b Value) Value

func Helper_StringConcat(a, b Value) Value
func Helper_StringLen(s Value) int

func Helper_SA_New() *SuperArray
func Helper_SA_Get(arr *SuperArray, key Value) Value
func Helper_SA_Set(arr *SuperArray, key, value Value)
func Helper_SA_Len(arr *SuperArray) int

func Helper_ArrayGet(arr *Array, idx int) Value
func Helper_ArraySet(arr *Array, idx int, val Value)
func Helper_ArrayLen(arr *Array) int
```

### 5.3 JIT 编译接口

```go
// internal/jit/compiler.go

type JITCompiler struct {
    // ...
}

func NewJITCompiler() *JITCompiler
func (c *JITCompiler) Compile(fn *Function) (*CompiledCode, error)
func (c *JITCompiler) CompileWithProfile(fn *Function, profile *Profile) (*CompiledCode, error)

type CompiledCode struct {
    Code     []byte       // 机器码
    Entry    uintptr      // 入口地址
    Size     int          // 代码大小
    Metadata *CodeMetadata // 调试信息等
}

// 执行编译后的代码
func (code *CompiledCode) Execute(args ...Value) Value
```

---

## 6. 测试计划

### 6.1 基准测试

```go
// benchmark/perf_test.go

// 纯整数计算
func BenchmarkIntLoop(b *testing.B) {
    // Go 版本
    // Sola 解释器版本
    // Sola JIT 版本
}

// 字符串操作
func BenchmarkStringConcat(b *testing.B)

// SuperArray 操作
func BenchmarkSuperArrayGet(b *testing.B)
func BenchmarkSuperArraySet(b *testing.B)
func BenchmarkSuperArrayMixed(b *testing.B)

// 混合代码 (JIT + SuperArray)
func BenchmarkMixedCode(b *testing.B)
```

### 6.2 正确性测试

```
□ Value 类型转换测试
□ 算术运算边界测试
□ SuperArray CRUD 测试
□ JIT 与解释器结果一致性测试
□ 内存泄漏测试
□ 并发安全测试
```

---

## 7. 风险与缓解

| 风险 | 概率 | 影响 | 缓解措施 |
|------|------|------|----------|
| Value 重构破坏现有代码 | 高 | 高 | 保持 API 兼容，渐进迁移 |
| Helper 调用开销过大 | 中 | 中 | 优化调用约定，关键路径内联 |
| JIT 代码生成 bug | 高 | 高 | 充分测试，保留解释器回退 |
| 平台兼容性问题 | 中 | 中 | 先支持 AMD64，再扩展 |

---

## 8. 成功标准

```
□ 纯整数循环: 达到 Go 50% 性能 (目标 2x 慢)
□ SuperArray: 达到 PHP 同等性能 (目标 1.5x PHP)
□ 混合代码: SuperArray 不影响其他代码的 JIT
□ 内存: 基本类型零堆分配
□ 稳定性: 所有现有测试通过
```

---

## 9. 文档版本

| 版本 | 日期 | 修改内容 |
|------|------|----------|
| 1.0 | 2026-01-12 | 初始版本 |

---

## 10. 后续对话指引

在后续对话中，请参考本文档进行实现。关键点：

1. **Value 重构时**: 参考 5.1 节的接口定义，使用 Tagged Union 方案
2. **VM 重构时**: 使用寄存器式设计 + 分派表
3. **JIT 重构时**: 实现 Helper 调用机制，不要禁用 SuperArray
4. **SuperArray 支持**: 使用 Helper 函数，不需要在 JIT 中"编译" SuperArray 逻辑
5. **测试时**: 对比 Go 和 PHP 性能

**注意**: 本文档的设计决策是基于当前分析做出的，实现过程中如发现更好的方案，请更新文档。
