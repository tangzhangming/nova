# Sola LSP Server v2

新一代Sola语言服务器，专注于**内存安全**和**核心功能**。

## 特性

### ✅ 已实现功能

1. **类跳转** - 点击类名跳转到定义
   - 当前文件中的类
   - 导入文件中的类
   - `use` 语句中的类
   - `new ClassName()` 中的类

2. **静态方法跳转** - `Class::method` 跳转到方法定义
   - 当前文件中的静态方法
   - 导入类的静态方法
   - 标准库的静态方法（如 `Str::trim`）

3. **实例方法跳转** - `$obj->method` 跳转到方法定义
   - 自动推断变量类型
   - 支持当前文件和导入类的方法

4. **内存优化**
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
├── server.go          # LSP协议处理
├── document.go        # 文档管理（LRU缓存）
├── definition.go      # 定义跳转核心逻辑
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

# 运行单个测试
node tests\lsp2\test_class_jump.js
node tests\lsp2\test_static_method.js
node tests\lsp2\test_instance_method.js
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

- [ ] Hover提示
- [ ] 代码补全
- [ ] 查找引用
- [ ] 重命名
- [ ] 文档符号

## 开发者

- 架构：基于Plan模式设计
- 测试：JavaScript集成测试
- 日期：2026-01-11
