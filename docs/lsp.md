# Sola 语言服务器 (LSP)

Sola LSP 是一个功能完备的语言服务器实现，支持 LSP 3.17 协议的绝大多数功能，为 Sola 语言提供专业级的 IDE 支持。

## 目录

- [功能列表](#功能列表)
- [编辑器集成指南](#编辑器集成指南)
- [配置选项](#配置选项)
- [性能调优](#性能调优)
- [故障排查](#故障排查)

## 功能列表

### 核心编辑功能

| 功能 | 状态 | 说明 |
|------|------|------|
| 代码补全 (Completion) | ✅ | 智能补全，支持类、方法、属性、变量等 |
| 补全项解析 (CompletionItem Resolve) | ✅ | 延迟加载补全项的详细文档 |
| 悬停提示 (Hover) | ✅ | 显示类型信息和文档注释 |
| 签名帮助 (Signature Help) | ✅ | 函数和方法调用时显示参数信息 |
| 跳转到定义 (Go to Definition) | ✅ | 跳转到符号定义位置 |
| 查找引用 (Find References) | ✅ | 查找符号的所有引用 |
| 重命名 (Rename) | ✅ | 智能重命名符号 |
| 准备重命名 (Prepare Rename) | ✅ | 验证重命名位置并返回范围 |

### 代码分析

| 功能 | 状态 | 说明 |
|------|------|------|
| 诊断 (Diagnostics) | ✅ | 语法错误、类型检查、代码质量警告 |
| 文档符号 (Document Symbols) | ✅ | 文件内符号大纲 |
| 工作区符号 (Workspace Symbols) | ✅ | 模糊搜索工作区所有符号 |
| 语义高亮 (Semantic Tokens) | ✅ | 基于语义的代码着色 |
| 代码操作 (Code Actions) | ✅ | 快速修复、重构操作 |

### 导航功能

| 功能 | 状态 | 说明 |
|------|------|------|
| 调用层次 (Call Hierarchy) | ✅ | 显示调用者和被调用者 |
| 类型层次 (Type Hierarchy) | ✅ | 显示类继承关系 |
| 文档高亮 (Document Highlight) | ✅ | 高亮同一符号的所有出现 |
| 文档链接 (Document Links) | ✅ | 识别 use/import 路径 |
| 折叠范围 (Folding Range) | ✅ | 代码块折叠 |
| 选择范围 (Selection Range) | ✅ | 智能扩展选择 |

### 编辑增强

| 功能 | 状态 | 说明 |
|------|------|------|
| 内联提示 (Inlay Hints) | ✅ | 显示参数名和类型推断 |
| 代码镜头 (Code Lens) | ✅ | 显示引用数量、运行测试按钮 |
| 文档格式化 (Formatting) | ✅ | 整个文档格式化 |
| 范围格式化 (Range Formatting) | ✅ | 选中区域格式化 |
| 链接编辑 (Linked Editing) | ✅ | 同时编辑关联内容 |
| 文档颜色 (Document Color) | ✅ | 颜色值预览和选择器 |

### 测试支持

| 功能 | 状态 | 说明 |
|------|------|------|
| 测试发现 | ✅ | 自动识别测试文件和方法 |
| 运行测试 Code Lens | ✅ | 在测试方法上显示运行按钮 |
| 调试测试 Code Lens | ✅ | 在测试方法上显示调试按钮 |
| 测试诊断 | ✅ | 检测空测试、无断言等问题 |

## 编辑器集成指南

### VS Code

1. **安装 Sola 扩展**（如果有）或手动配置：

在 `.vscode/settings.json` 中添加：

```json
{
  "sola.lsp.serverPath": "/path/to/solals",
  "sola.lsp.trace.server": "verbose"
}
```

2. **手动启动配置**

如果没有专用扩展，可以使用通用 LSP 客户端：

```json
{
  "languageserver": {
    "sola": {
      "command": "solals",
      "args": ["--log", "/tmp/solals.log"],
      "filetypes": ["sola"],
      "rootPatterns": ["go.mod", ".git"]
    }
  }
}
```

### Neovim (nvim-lspconfig)

```lua
local lspconfig = require('lspconfig')
local configs = require('lspconfig.configs')

if not configs.solals then
  configs.solals = {
    default_config = {
      cmd = { 'solals', '--log', '/tmp/solals.log' },
      filetypes = { 'sola' },
      root_dir = lspconfig.util.root_pattern('go.mod', '.git'),
      settings = {}
    }
  }
end

lspconfig.solals.setup{}
```

### Sublime Text

使用 LSP 包，在 `LSP.sublime-settings` 中添加：

```json
{
  "clients": {
    "solals": {
      "command": ["solals", "--log", "/tmp/solals.log"],
      "selector": "source.sola",
      "enabled": true
    }
  }
}
```

### Emacs (lsp-mode)

```elisp
(require 'lsp-mode)

(add-to-list 'lsp-language-id-configuration '(sola-mode . "sola"))

(lsp-register-client
 (make-lsp-client
  :new-connection (lsp-stdio-connection '("solals" "--log" "/tmp/solals.log"))
  :major-modes '(sola-mode)
  :server-id 'solals))

(add-hook 'sola-mode-hook #'lsp)
```

## 配置选项

LSP 服务器支持以下配置选项：

### 格式化 (formatting)

```json
{
  "formatting": {
    "tabSize": 4,
    "insertSpaces": true,
    "insertFinalNewline": true,
    "trimFinalNewlines": true,
    "trimTrailingWhitespace": true
  }
}
```

### 诊断 (diagnostics)

```json
{
  "diagnostics": {
    "enable": true,
    "showWarnings": true,
    "showHints": true,
    "unusedVariables": true,
    "deprecatedAPIs": true,
    "typeMismatch": true
  }
}
```

### 代码补全 (completion)

```json
{
  "completion": {
    "enable": true,
    "autoImport": true,
    "showDeprecated": true,
    "showSnippets": true,
    "triggerOnIdentifier": true
  }
}
```

### 内联提示 (inlayHints)

```json
{
  "inlayHints": {
    "enable": true,
    "parameterNames": true,
    "parameterTypes": false,
    "variableTypes": true,
    "propertyTypes": false,
    "returnTypes": false
  }
}
```

### 代码镜头 (codeLens)

```json
{
  "codeLens": {
    "enable": true,
    "showReferences": true,
    "showImplementations": true,
    "showTests": true
  }
}
```

### 工作区 (workspace)

```json
{
  "workspace": {
    "maxFileSize": 5242880,
    "indexOnStartup": true,
    "indexExclude": [
      "**/node_modules/**",
      "**/.git/**",
      "**/vendor/**"
    ],
    "watchFileChanges": true
  }
}
```

## 性能调优

### 大型项目优化

1. **排除不需要索引的目录**

```json
{
  "workspace": {
    "indexExclude": [
      "**/node_modules/**",
      "**/vendor/**",
      "**/build/**",
      "**/dist/**",
      "**/.git/**"
    ]
  }
}
```

2. **限制文件大小**

```json
{
  "workspace": {
    "maxFileSize": 5242880
  }
}
```

3. **禁用不需要的功能**

```json
{
  "inlayHints": {
    "enable": false
  },
  "codeLens": {
    "enable": false
  }
}
```

### 性能目标

| 操作 | 小文件 (<1000行) | 大文件 (>5000行) |
|------|------------------|------------------|
| 代码补全 | < 100ms | < 300ms |
| 诊断更新 | < 200ms | < 500ms |
| 跳转定义 | < 50ms | < 150ms |
| 查找引用 | < 200ms | < 1000ms |

### 内存使用

- 小项目 (<100 文件): ~50MB
- 中型项目 (100-500 文件): ~150MB
- 大型项目 (>1000 文件): ~500MB

## 故障排查

### 常见问题

#### 1. LSP 服务器无法启动

**症状**: 编辑器显示连接失败或无响应

**解决方案**:
1. 确认 `solals` 可执行文件在 PATH 中或路径正确
2. 检查日志文件（使用 `--log` 参数指定）
3. 验证权限：确保可执行文件有执行权限

```bash
# 测试服务器是否正常
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}' | solals
```

#### 2. 补全不工作

**症状**: 输入时没有补全提示

**解决方案**:
1. 确认文件扩展名是 `.sola`
2. 检查文件是否有语法错误（诊断信息）
3. 确认 `completion.enable` 为 `true`
4. 尝试手动触发补全（通常是 Ctrl+Space）

#### 3. 跳转定义失败

**症状**: Go to Definition 无响应或跳转错误

**解决方案**:
1. 确认工作区已正确索引
2. 检查符号是否在已打开或已索引的文件中定义
3. 对于标准库，确认标准库路径已正确配置

#### 4. 诊断信息延迟或不准确

**症状**: 错误提示延迟显示或显示错误的诊断

**解决方案**:
1. 保存文件以触发完整诊断
2. 检查增量同步是否正常工作
3. 大文件可能需要更长时间处理

#### 5. 高内存使用

**症状**: LSP 进程占用大量内存

**解决方案**:
1. 配置 `indexExclude` 排除不需要的目录
2. 减少 `maxFileSize` 限制
3. 关闭不需要的功能（如 inlayHints）
4. 重启 LSP 服务器

### 调试模式

启用详细日志：

```bash
solals --log /tmp/solals.log --log-level debug
```

日志格式：
```
[LSP] Received: {"jsonrpc":"2.0","method":"textDocument/completion",...}
[LSP] Sending: {"jsonrpc":"2.0","result":[...]}
```

### 报告问题

报告问题时，请提供：

1. **环境信息**
   - 操作系统和版本
   - 编辑器和版本
   - Sola LSP 版本 (`solals --version`)

2. **复现步骤**
   - 最小化的代码示例
   - 具体的操作步骤

3. **日志文件**
   - 启用 `--log` 和 `trace.server: verbose`
   - 提供相关的日志片段

4. **配置文件**
   - LSP 配置
   - 编辑器设置

## LSP 协议支持

### 已支持的方法

#### 生命周期
- `initialize`
- `initialized`
- `shutdown`
- `exit`

#### 文档同步
- `textDocument/didOpen`
- `textDocument/didChange`
- `textDocument/didClose`
- `textDocument/didSave`

#### 语言功能
- `textDocument/completion`
- `completionItem/resolve`
- `textDocument/hover`
- `textDocument/signatureHelp`
- `textDocument/definition`
- `textDocument/references`
- `textDocument/documentHighlight`
- `textDocument/documentSymbol`
- `textDocument/codeAction`
- `textDocument/codeLens`
- `textDocument/formatting`
- `textDocument/rangeFormatting`
- `textDocument/rename`
- `textDocument/prepareRename`
- `textDocument/foldingRange`
- `textDocument/selectionRange`
- `textDocument/documentLink`
- `textDocument/semanticTokens/full`
- `textDocument/semanticTokens/range`
- `textDocument/inlayHint`
- `textDocument/prepareCallHierarchy`
- `callHierarchy/incomingCalls`
- `callHierarchy/outgoingCalls`
- `textDocument/prepareTypeHierarchy`
- `typeHierarchy/supertypes`
- `typeHierarchy/subtypes`
- `textDocument/linkedEditingRange`
- `textDocument/documentColor`
- `textDocument/colorPresentation`
- `workspace/symbol`
- `workspace/didChangeConfiguration`

## 版本历史

### v0.1.0 (当前)
- 初始版本
- 支持 LSP 3.17 协议的 25+ 功能
- 完整的类型检查和诊断系统
- 语义高亮和内联提示
- 调用层次和类型层次
- 测试支持功能
