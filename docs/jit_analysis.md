# JIT 性能问题分析报告

## 问题现象

| 测试 | Sola (--jitless) | Sola (JIT) | PHP 8.2 | Go |
|------|------------------|------------|---------|-----|
| 简单循环 | ~2400ms | ~2400ms | ~945ms | ~173ms |
| JIT 加速比 | - | 1.0x (无效) | - | - |

**结论：JIT 完全没有生效，Sola 解释器比 PHP 慢 2.5 倍**

---

## 根本原因分析

### 原因 1: 顶层代码永远不会被 JIT 编译

| 代码位置 | 是否被 JIT | 原因 |
|---------|-----------|------|
| **顶层脚本** | ❌ 永远不会 | 不是函数，不经过 `vm.call()` |
| **函数内部** | ⚠️ 理论上可以 | 需要满足热点条件 |

**关键代码 (`vm.go:4226-4234`)**:
```go
func (vm *VM) call(closure *bytecode.Closure, argCount int) InterpretResult {
    // 只有通过 call() 调用的函数才会被记录
    if vm.jitEnabled && vm.jitCompiler != nil {
        profiler := vm.jitCompiler.GetProfiler()
        if profiler != nil {
            profiler.RecordCall(fn)  // 顶层代码不会走这里
        }
    }
    ...
}
```

**测试代码全在顶层**:
```sola
// bench_simple.sola - 全部是顶层代码
int $sum = 0;
for (int $i = 0; $i < 10000000; $i++) {  // 这个循环不在函数内
    $sum = $sum + $i;
}
```

---

### 原因 2: 函数调用被禁用 JIT

| 操作码 | JIT 状态 | 影响 |
|--------|---------|------|
| `OpCall` | ❌ JITDisabled | 普通函数调用禁用 JIT |
| `OpCallStatic` | ❌ JITDisabled | 静态方法调用禁用 JIT |
| `OpCallMethod` | ❌ JITDisabled | 实例方法调用禁用 JIT |
| `OpTailCall` | ❌ JITDisabled | 尾调用禁用 JIT |

**关键代码 (`bridge.go`)**:
```go
case bytecode.OpCall, bytecode.OpTailCall, bytecode.OpCallMethod, bytecode.OpCallStatic:
    return JITDisabled  // 包含任何函数调用的函数都不会被 JIT
```

**后果**：即使写成函数形式，如果函数内部有任何函数调用，整个函数都不能 JIT。

---

### 原因 3: 热点检测阈值问题

| 参数 | 值 | 问题 |
|------|-----|------|
| `HotThreshold` | 100 | 函数需要被调用 100 次才会编译 |
| 测试脚本调用次数 | 1 | 每个函数只调用一次 |

即使把代码放进函数，单次运行也不会触发 JIT。

---

### 原因 4: 解释器本身性能差

| 语言 | 实现方式 | 性能特点 |
|------|---------|---------|
| **PHP 8.2** | C 实现 + OPcache | 高度优化的字节码解释器 |
| **Sola** | Go 实现 | Go 的函数调用/interface 开销大 |

**Go 解释器的固有开销**:
1. `interface{}` 类型断言开销
2. `map` 访问开销（全局变量、类查找）
3. 切片边界检查
4. GC 压力

---

## JIT 系统当前状态

### 已实现但无效的功能

| 组件 | 状态 | 问题 |
|------|------|------|
| x64 代码生成器 | ✅ 实现 | 没有代码被编译 |
| FunctionTable | ✅ 实现 | 没有函数注册 |
| CallHelperById | ✅ 实现 | 从未被调用 |
| JIT VM 回调 | ✅ 实现 | 回调机制有 bug |
| 热点检测 | ✅ 实现 | 阈值不合理 |

### 真正需要的改进

| 优先级 | 改进项 | 难度 | 效果 |
|--------|-------|------|------|
| **P0** | 支持顶层代码 JIT | 高 | 让基准测试能用 JIT |
| **P0** | 降低热点阈值 | 低 | 测试时更容易触发 |
| **P1** | 修复函数调用 JIT | 高 | 递归/嵌套函数可用 |
| **P2** | 优化解释器 | 中 | 提升基线性能 |

---

## 修复建议

### 短期：让 JIT 能被测试

```go
// 1. 添加强制 JIT 模式（跳过热点检测）
// jit/config.go
type Config struct {
    ForceCompile bool  // 强制编译所有函数
}

// 2. 支持顶层代码作为匿名函数编译
// 3. 将热点阈值降到 1（用于测试）
```

### 长期：完善 JIT 系统

1. **Tracing JIT**：跟踪热循环而不是热函数
2. **OSR (On-Stack Replacement)**：循环中途切换到 JIT
3. **函数内联**：消除函数调用开销
4. **类型特化**：为特定类型生成优化代码

---

## 结论

**JIT 没有任何加速效果的原因**:
1. 顶层代码不会被 JIT（设计问题）
2. 函数调用禁用了 JIT（实现不完整）
3. 热点阈值太高（测试场景不触发）
4. 解释器本身比 PHP 慢（Go 的固有开销）

**当前 JIT 代码是无用代码**，增加了编译时间但不产生任何性能收益。
