# Sola 语言核心状态（VM + JIT 生产级目标）

> 说明：本文件用于评估 **语言核心** 的工程状态，以支撑"VM + JIT 模式运行的生产级语言"目标。
> 范围：仅覆盖编译前端/语义与类型系统/字节码/VM/JIT/GC 等"语言核心"。
> 排除：`lib/` 标准库、`*_test.go` 测试、CLI/formatter/editor 等工具链与编辑器支持。

---

## 需要/可以完成实现的（缺失能力，但已有明显"半成品"基础）

| 领域 | 条目 | 现状（代码证据） | 备注/完成标准 |
|---|---|---|---|
| VM↔JIT | **启用 JIT 原生执行路径** | `internal/vm/vm.go` 中 JIT 执行分支被整体注释禁用，并明确标注"执行层暂时禁用" | 打通 `GetCompiled`→`executeNative`→失败回退解释器；配套崩溃隔离/回退与统计 |
| JIT 支持范围 | **支持控制流（Jump/Loop/分支）** | `internal/jit/bridge.go:CanJIT` 直接拒绝 `OpLoop/OpJump/OpJumpIf*`（注释写"暂时禁用循环"） | 先实现块/CFG 代码生成，再逐步放开 `CanJIT` 白名单 |
| JIT IR | **完成真正 SSA（Phi/rename）** | `internal/jit/builder.go` 的 `completePhis()` 是空实现并写明 TODO（支配边界/Phi/变量重命名） | SSA 完整后再逐步启用更高优化等级，避免错误优化导致错误码 |
| JIT 值模型 | **支持 float 与更一般的值传递** | `internal/jit/bridge.go` 采用"int64 参数/返回"的桥接，`ValueToInt64` 对 float 进行截断 | 至少区分 int/float 的调用约定（或 tagged value），否则数值语义不可靠 |
| VM 性能 | **把内联缓存真正接入方法调用路径** | `internal/vm/call_optimizer.go` 标注 `TODO: 集成内联缓存 (B1)` | 将 call-site cache 与 `OpCallMethod`/invoke 路径绑定；对 megamorphic 回退 |
| 静态分析 | **变量"确定赋值"数据流分析** | `internal/compiler/type_checker.go` 中 `UninitializedChecker` 被注释禁用，并写明 TODO | 编译期稳定给出"可能未初始化"错误/警告，减少运行期问题 |

---

## 待完善/修复的（已有实现，但存在明显缺陷/不一致/会误导用户）

| 领域 | 条目 | 现状（代码证据） | 影响 |
|---|---|---|---|
| 并发模型 | **VM 明确非线程安全，但语言层并发语义未形成闭环** | `internal/vm/vm.go` 注释说明 VM 实例不应跨 goroutine 共享 | 生产化需要明确并发原语与运行时约束，否则易出现误用 |

---

## 完全完备不需要再调整的（语言核心层面已闭环且实现一致性较高）

| 领域 | 条目 | 现状（代码证据） | 备注 |
|---|---|---|---|
| 前端 | **Token/Lexer 基础扫描链路** | `internal/token/`、`internal/lexer/` 结构完整，Parser 以 token 流驱动 | 主要短板在"错误恢复体验"，不是语法覆盖度 |
| 语法/AST | **AST 节点体系 + Arena/Factory** | `internal/ast/ast.go` + `internal/ast/arena.go`/`factory.go` | 节点构造集中，利于后续扩展与性能优化 |
| 语言特性（表达式） | **match 表达式的编译闭环** | `internal/compiler/compiler.go` 存在 `compileMatchExpr` 与返回类型推断逻辑 | 语义与字节码生成已落地 |
| OO/分派 | **接口 VTable 路径与回退查找** | `internal/vm/vm.go:findMethodWithVTable` 等 | 分派维度清晰（name+argCount，兼容默认参数范围） |
| 调用约定 | **默认参数/可变参数的栈布局与填充** | `internal/vm/vm.go:call` 对 DefaultValues 与 variadic 打包逻辑有详尽注释与实现 | 这是 VM 正确性的核心，当前实现"讲清楚且做到了" |
| 异常模型（解释器路径） | **try/catch/finally 的字节码结构与 VM 处理链路** | 编译器发出 `OpEnterTry` 等；VM 侧有 `handleException`/stack trace 相关逻辑 | JIT 暂不支持异常属于"优化缺失"，不影响解释器语义闭环 |
| 元数据 | **注解（语法→AST→字节码）闭环** | parser 支持 `@`，compiler/bytecode 支持 annotations 的编解码 | 不评价标准库反射 API，仅评价语言元数据链路 |
| JIT 准入规则 | **统一使用 `jit.CanJIT` 作为唯一判定** | 已删除冗余的 `isJITSafe`，统一由 `internal/jit/bridge.go:CanJIT` 负责 | 消除了 VM/JIT 两套规则不一致的维护陷阱 |
| 类型系统 | **严格禁止隐式类型转换** | `internal/compiler/type_checker.go:isTypeCompatible` 不再允许 `int→float`、整数族互转 | 与文档"严格类型"语义一致，必须使用显式 `as` 转换 |
| 类型检查 | **变量初始化状态正确追踪** | `declareVariable()` 根据是否有初值设置 `IsInitialized`；无初值的变量声明正确标记为未初始化 | 为后续"确定赋值"分析打好基础 |
| 解析器错误恢复 | **基于 panicMode 状态的错误恢复** | 移除 panic/recover，改用 `panicMode` 标志控制错误恢复流程；`synchronize()` 在顶层循环调用 | 避免 panic 开销，减少级联报错，更易调试 |
| 内联缓存 | **类型身份正确获取** | `internal/vm/inline_cache.go:unsafePointer` 使用 `unsafe.Pointer` 获取真实指针值 | IC 基础设施可用，后续可接入方法调用路径 |
