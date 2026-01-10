# 已知 BUG 列表

本文档记录发现但暂未修复的 BUG。

---

## JIT 编译器运行时崩溃

**状态**: 未修复  
**发现日期**: 2026-01-11  
**严重程度**: 高

### 问题描述

在复杂类调用链中（如构造函数调用多层私有方法，且方法内部大量调用原生函数），JIT 编译会导致运行时崩溃。

### 错误信息

```
Exception 0xc0000005 0x8 0xc00002b2b8 0xc00002b2b8
PC=0xc00002b2b8
runtime: g 1 gp=0xc0000021c0: unknown pc 0xc00002b2b8
```

### 堆栈跟踪

```
jit.callNative2
jit.CallNative
vm.executeNative
```

### 复现场景

- 使用 `SimpleClient` 类连接 MySQL 时触发
- 涉及 `TcpClient.readExact()`、`Bytes::get()`、`native_crypto_sha1_bytes()` 等多个原生函数调用
- 在构造函数 -> `doHandshake()` -> `readPacket()` -> `scramblePassword()` 的调用链中发生

### 临时解决方案

使用 `--jitless` 参数禁用 JIT 编译，仅使用解释器：

```bash
sola run --jitless your_script.sola
```

### 影响范围

- 涉及大量原生函数调用的复杂类
- 深层嵌套的方法调用

### 待修复

需要检查 JIT 编译器（`internal/jit/`）中对原生函数调用的处理逻辑。

---

---

## 编译器内存泄露

**状态**: 未修复  
**发现日期**: 2026-01-11  
**严重程度**: 高

### 问题描述

编译复杂类（特别是包含 enum、多个类定义、或大量方法的文件）时，编译器可能耗尽内存并崩溃。

### 错误信息

```
fatal error: runtime: cannot allocate memory
```

### 复现场景

- 编译包含 enum 的类文件
- 编译包含多个类定义的文件

### 临时解决方案

简化代码结构，避免在单个文件中定义过多类型。

---

## push() 函数不修改原数组

**状态**: 设计问题  
**发现日期**: 2026-01-11  
**严重程度**: 中

### 问题描述

`push($arr, $val)` 函数返回新数组，但不修改原数组。作为语句调用时，结果被丢弃。

### 临时解决方案

使用索引赋值代替 push：

```sola
// 不工作
push($arr, "value");

// 正确方式
$arr[len($arr)] = "value";
```

---

## 十六进制字面量 0xff 编译问题

**状态**: 待调查  
**发现日期**: 2026-01-11  
**严重程度**: 中

### 问题描述

在复杂的条件分支中使用十六进制字面量 `0xff` 进行比较时，可能会导致运行时错误 "未知操作码: 255"。

### 临时解决方案

使用十进制 `255` 代替十六进制 `0xff`：

```sola
// 可能有问题
if ($byte == 0xff) { ... }

// 替代方案
$errCode := 255;
if ($byte == $errCode) { ... }
```

---

## 已修复的 BUG（记录）

### 1. 尾调用优化 BUG

**修复日期**: 2026-01-11

原生函数（`native_xxx`）被错误地当作尾调用优化，导致 `return native_xxx()` 返回 `$this` 而不是函数结果。

**修复位置**: `internal/compiler/compiler.go` - `isTailCallable()` 函数

### 2. `close` 内置函数与方法名冲突

**修复日期**: 2026-01-11

`close` 是内置函数（用于关闭 channel），与类方法名冲突会导致运行时错误。

**解决方案**: 类方法避免使用 `close` 作为方法名，改用 `disconnect` 等替代名称。
