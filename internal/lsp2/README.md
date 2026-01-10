# Sola LSP Server v2

新一代Sola语言服务器，专注于**内存安全**和**核心功能**。

## 特性

### ✅ 已实现功能

1. **定义跳转** (textDocument/definition)
   - 类跳转 - 点击类名跳转到定义
   - 静态方法跳转 - `Class::method` 跳转到方法定义
   - 实例方法跳转 - `$obj->method` 跳转到方法定义
   - 自动推断变量类型

2. **悬停提示** (textDocument/hover)
   - 类名悬停 - 显示类签名和成员数量
   - 方法名悬停 - 显示方法签名和来源
   - 属性悬停 - 显示属性类型和来源
   - 静态/实例访问支持

3. **代码补全** (textDocument/completion)
   - 实例成员补全 - `$obj->` 触发方法/属性补全
   - 静态成员补全 - `Class::` 触发静态方法/常量补全
   - 变量补全 - `$` 触发变量名补全
   - 枚举成员补全 - `Enum::` 触发枚举值补全
   - `new` 补全 - 类名补全
   - 关键字补全

4. **签名帮助** (textDocument/signatureHelp)
   - 静态方法签名 - `Class::method(` 显示参数提示
   - 实例方法签名 - `$obj->method(` 显示参数提示
   - 当前参数高亮

5. **内存优化**
   - LRU缓存（最多10个文档）
   - 按需加载导入文件（缓存20个）
   - 文档关闭时立即释放内存
   - 定期内存检查和自动清理（60秒）
   - 验证：重复100次打开/关闭，内存增长 < 2MB ✅

5. **日志系统**
   - 环境变量控制：`SOLA_LSP_DEBUG=1` 启用
   - 简单文本格式：`[时间] [级别] 消息`
   - 默认关闭，方便生产使用

## 架构设计

```
internal/lsp2/
├── server.go          # LSP协议处理 + 请求分发
├── document.go        # 文档管理（LRU缓存）
├── definition.go      # 定义跳转核心逻辑
├── hover.go           # 悬停提示
├── completion.go      # 代码补全
├── signature_help.go  # 签名帮助
├── import_resolver.go # 导入解析（按需加载）
├── logger.go          # 可关闭的日志系统
├── memory.go          # 内存监控和清理
└── position.go        # 位置工具函数
```

## 编译

```bash
cd d:\workspace\go\src\nova
go build -o bin\solals2.exe cmd\solals2\main.go
```

## 使用

### 命令行

```bash
# 启动LSP服务器（默认关闭日志）
solals2.exe

# 启用调试日志
set SOLA_LSP_DEBUG=1
solals2.exe --log lsp2.log

# 查看版本
solals2.exe --version

# 查看帮助
solals2.exe --help
```

### VS Code 配置

在 `.vscode/settings.json` 中配置：

```json
{
  "sola.languageServer.path": "D:/workspace/go/src/nova/bin/solals2.exe",
  "sola.languageServer.args": ["--log", "D:/lsp2.log"]
}
```

启用调试日志（Windows）：

```json
{
  "sola.languageServer.env": {
    "SOLA_LSP_DEBUG": "1"
  }
}
```

## 运行测试

```bash
cd d:\workspace\go\src\nova

# 启用调试日志
set SOLA_LSP_DEBUG=1

# 定义跳转测试
node tests\lsp2\test_class_jump.js
node tests\lsp2\test_static_method.js
node tests\lsp2\test_instance_method.js

# 新功能测试
node tests\lsp2\test_hover.js
node tests\lsp2\test_completion.js
node tests\lsp2\test_signature.js

# 内存测试
node tests\lsp2\test_memory.js

# 运行所有测试
node tests\lsp2\run_all.js
```

## 测试结果

✅ **所有测试通过** (2026-01-11)

| 测试 | 状态 | 说明 |
|-----|------|------|
| 类跳转测试 | ✓ PASS | 4/4 场景通过 |
| 静态方法测试 | ✓ PASS | 3/3 场景通过 |
| 实例方法测试 | ✓ PASS | 核心功能正常 |
| 内存测试 | ✓ PASS | 增长 < 2MB |
| **Hover测试** | ✓ PASS | **6/6 场景通过** |
| **Completion测试** | ✓ PASS | **6/6 场景通过** |
| **SignatureHelp测试** | ✓ PASS | **5/5 场景通过** |

## 内存管理策略

### 平衡模式配置

- **文档缓存**：最多10个（LRU淘汰）
- **导入缓存**：最多20个文件（LRU淘汰）
- **文档大小限制**：500KB
- **自动GC**：
  - 文档关闭时立即GC
  - 每60秒检查内存
  - 超过100MB自动清理缓存

### 验证结果

- 重复打开/关闭100次：内存增长 1.99 MB ✅
- 打开10个文档后关闭：成功回收内存 ✅

## 与旧LSP对比

| 特性 | 旧LSP (internal/lsp) | 新LSP (internal/lsp2) |
|-----|---------------------|---------------------|
| 文档缓存 | 无限制 | 最多10个（LRU） |
| 工作区索引 | 全量预索引 | 按需加载 |
| 导入缓存 | 无限增长 | 最多20个 |
| 内存监控 | 有但不及时 | 60秒检查+自动清理 |
| 日志系统 | 总是启用 | 可一键关闭 |
| 定义跳转 | 支持 | ✓ 优化实现 |
| 其他功能 | 完整 | 聚焦核心功能 |
| 内存增长 | > 100MB | < 2MB ✅ |

## 支持的Sola语法

### 类跳转

```sola
use test.project.utils.Helper;  // Helper 可跳转

class Test {
    // ...
}

$h = new Helper();  // Helper 可跳转
$t = new Test();    // Test 可跳转
```

### 静态方法调用

```sola
Helper::greet("World");  // Helper跳转到类，greet跳转到方法
Str::trim($text);        // 标准库Str类的trim方法
Test::yy();              // 当前文件中的静态方法
```

### 实例方法调用

```sola
$helper = new Helper();
$helper->greet("World");  // 推断$helper类型为Helper，跳转到greet方法

$test = new Test();
$test->xx();  // 推断$test类型为Test，跳转到xx方法
```

## 性能指标

- 启动时间：< 1秒
- 跳转响应：< 100ms
- 内存占用：8-10 MB（正常使用）
- 内存增长：< 2 MB（100次操作）

## 未来计划

- [x] Hover提示 ✅
- [x] 代码补全 ✅
- [x] 签名帮助 ✅
- [ ] 查找引用
- [ ] 重命名
- [ ] 文档符号

## 开发者

- 架构：基于Plan模式设计
- 测试：JavaScript集成测试
- 日期：2026-01-11
