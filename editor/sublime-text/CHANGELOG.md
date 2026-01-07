# Sublime Text 插件更新日志

## 版本 2.0 (2026-01-08)

### 新增功能

#### 1. 一键注释支持 ✨
- 添加 `Comments.tmPreferences` 文件
- 支持标准快捷键：
  - `Ctrl + /` (Windows/Linux) 或 `Cmd + /` (macOS) - 单行注释
  - `Ctrl + Shift + /` (Windows/Linux) 或 `Cmd + Shift + /` (macOS) - 块注释
- 单行注释使用 `//`
- 块注释使用 `/* */`

#### 2. 完整语法支持

##### 新增关键字
- `match` - 模式匹配表达式
- `type` - 类型别名声明
- `get`, `set` - 属性访问器
- `is` - 类型检查运算符
- `as`, `as?` - 类型转换运算符

##### 新增类型
- `byte` - 字节类型 (u8 的别名)
- `superarray` - 动态类型万能数组

##### 新增语法元素
- 注解语法：`@Annotation`
- 文档注释标签：`@param`, `@return`, `@throws`, etc.
- 联合类型：`int|string`
- 可空类型：`?Type`
- 扩展运算符：`...`
- 成员访问点运算符：`.`
- 安全类型转换：`as?`

#### 3. 代码补全增强

##### Match 表达式
- `match` - 基本 match 表达式（值匹配）
- `matcht` - 类型匹配的 match 表达式
- `matchg` - 带守卫条件的 match 表达式

##### 属性访问器
- `autoprop` - 自动属性 `{ get; set; }`
- `propro` - 只读属性 `{ get; }`
- `propgs` - 公开读取、私有写入属性 `{ get; private set; }`
- `propget` - 完整属性访问器（带自定义 get/set 块）
- `propexpr` - 表达式体属性 `=> expression`

##### 函数和闭包
- `arrow` - 箭头函数
- `closure` - 闭包函数
- `closureuse` - 带 use 子句的闭包
- `generic` - 泛型函数

##### 变量和类型
- `arrlit` - 类型化数组字面量 `type{...}`
- `maplit` - Map 字面量
- `superarr` - SuperArray 声明
- `nullable` - 可空类型变量
- `union` - 联合类型变量
- `typealias` - 类型别名

##### 类型操作
- `is` - 类型检查模板
- `cast` - 类型转换
- `safecast` - 安全类型转换 `as?`

##### 注解
- `annotation` - 自定义注解
- `deprecated` - @Deprecated 注解

### 改进

#### 语法高亮
- 改进文档注释识别（必须在多行注释之前匹配）
- 添加文档注释标签高亮 (`@param`, `@return` 等)
- 改进运算符识别
- 添加注解语法高亮
- 支持联合类型分隔符高亮
- 优化属性访问器关键字识别

#### 文档
- 更新 README，添加所有新功能说明
- 添加详细的 scope 规则说明
- 更新安装说明，包含 `Comments.tmPreferences`
- 添加示例文件 `example.sola` 展示所有新特性

### 文件清单

```
editor/sublime-text/
├── Sola.sublime-syntax          # 语法定义（已更新）
├── Sola.sublime-completions     # 代码补全（已更新）
├── Comments.tmPreferences       # 注释配置（新增）
├── README.md                    # 使用说明（已更新）
├── CHANGELOG.md                 # 更新日志（新增）
└── example.sola                 # 示例文件（新增）
```

### 安装

请将以下文件复制到 Sublime Text 的 Packages 目录：
- `Sola.sublime-syntax`
- `Sola.sublime-completions`
- `Comments.tmPreferences` ⬅️ **新文件，必需！**

#### Windows
```powershell
$dest = "$env:APPDATA\Sublime Text\Packages\Sola"
New-Item -ItemType Directory -Force -Path $dest
Copy-Item "Sola.sublime-syntax" $dest
Copy-Item "Sola.sublime-completions" $dest
Copy-Item "Comments.tmPreferences" $dest
```

#### macOS
```bash
dest=~/Library/Application\ Support/Sublime\ Text/Packages/Sola
mkdir -p "$dest"
cp Sola.sublime-syntax Sola.sublime-completions Comments.tmPreferences "$dest/"
```

#### Linux
```bash
dest=~/.config/sublime-text/Packages/Sola
mkdir -p "$dest"
cp Sola.sublime-syntax Sola.sublime-completions Comments.tmPreferences "$dest/"
```

### 兼容性

- Sublime Text 3 及以上版本
- 完全支持 SOLA 语法指南 (SOLA_SYNTAX_GUIDE.md) 中的所有特性

### 已知问题

无

---

## 版本 1.0 (初始版本)

### 功能
- 基本语法高亮
- 基础代码补全
- 关键字识别
- 类型系统支持
- 函数和类定义识别

### 限制
- 不支持一键注释
- 缺少 match 表达式支持
- 缺少属性访问器语法
- 缺少注解语法
- 部分新类型未识别

