# Sola Language Support for Sublime Text

为 Sublime Text 提供 Sola 语言支持，包括语法高亮和代码补全。

## 安装

### 手动安装

1. 打开 Sublime Text
2. 点击菜单 `Preferences` → `Browse Packages...`
3. 在打开的目录中创建 `Sola` 文件夹
4. 将以下文件复制到 `Sola` 文件夹：
   - `Sola.sublime-syntax`
   - `Sola.sublime-completions`
   - `Comments.tmPreferences`

### 快速安装（Windows）

```powershell
# 复制到 Sublime Text Packages 目录
$dest = "$env:APPDATA\Sublime Text\Packages\Sola"
New-Item -ItemType Directory -Force -Path $dest
Copy-Item "Sola.sublime-syntax" $dest
Copy-Item "Sola.sublime-completions" $dest
Copy-Item "Comments.tmPreferences" $dest
```

### 快速安装（macOS）

```bash
dest=~/Library/Application\ Support/Sublime\ Text/Packages/Sola
mkdir -p "$dest"
cp Sola.sublime-syntax Sola.sublime-completions Comments.tmPreferences "$dest/"
```

### 快速安装（Linux）

```bash
dest=~/.config/sublime-text/Packages/Sola
mkdir -p "$dest"
cp Sola.sublime-syntax Sola.sublime-completions Comments.tmPreferences "$dest/"
```

## 功能

### 语法高亮

- 关键字 (if, for, class, function, match, etc.)
- 类型 (int, string, bool, superarray, byte, etc.)
- 变量 ($variable)
- 字符串 (普通字符串和插值字符串 #"...")
- 注释 (单行 // 和多行 /* */ 和文档注释 /** */)
- 数字 (整数、浮点数、十六进制、二进制)
- 运算符 (包括类型运算符 is, as, as?)
- 类和函数定义
- 异常类 (Throwable, Exception, RuntimeException, Error)
- 注解语法 (@Annotation)
- 属性访问器 (get, set)
- 联合类型 (int|string)
- 可空类型 (?Type)

### 代码补全

输入以下触发词后按 Tab 键：

#### 控制流
- `if` → if 语句
- `ifelse` → if-else 语句
- `for` → for 循环
- `foreach` → foreach 循环
- `foreachkv` → foreach 带 key-value
- `while` → while 循环
- `switch` → switch 语句
- `match` → match 表达式（值匹配）
- `matcht` → match 表达式（类型匹配）
- `matchg` → match 表达式（带守卫条件）
- `try` → try-catch 块
- `trycf` → try-catch-finally 块

#### 声明
- `fn` → 函数定义
- `pfn` → public 函数
- `sfn` → static 函数
- `arrow` → 箭头函数
- `closure` → 闭包函数
- `closureuse` → 带 use 的闭包
- `generic` → 泛型函数
- `class` → 类定义
- `classext` → 继承类
- `interface` → 接口定义
- `enum` → 枚举定义
- `construct` → 构造函数
- `destruct` → 析构函数

#### 变量
- `var` → 带类型变量声明
- `vardecl` → 类型推导变量声明
- `arr` → 类型化数组声明
- `arrlit` → 数组字面量 (type{...})
- `map` → Map 声明
- `maplit` → Map 字面量
- `superarr` → SuperArray 声明
- `nullable` → 可空类型变量
- `union` → 联合类型变量

#### 内置函数
- `print`, `echo`, `len`, `typeof`
- `push`, `pop`, `strlen`, `substr`
- `str_split`, `str_join`, `str_replace`
- `to_int`, `to_float`, `to_string`, `to_bool`

#### 其他
- `namespace` → 命名空间
- `use` → 导入语句
- `useex` → 导入异常类
- `typealias` → 类型别名
- `getset` → getter/setter 模板
- `autoprop` → 自动属性 (get; set;)
- `propro` → 只读属性
- `propgs` → 公开读取、私有写入属性
- `propget` → 完整属性访问器
- `propexpr` → 表达式体属性
- `is` → 类型检查
- `cast` → 类型转换
- `safecast` → 安全类型转换
- `annotation` → 注解
- `deprecated` → @Deprecated 注解

### 一键注释

支持 Sublime Text 的标准注释快捷键：
- **Windows/Linux**: `Ctrl + /` (单行注释) 或 `Ctrl + Shift + /` (块注释)
- **macOS**: `Cmd + /` (单行注释) 或 `Cmd + Shift + /` (块注释)

单行注释使用 `//`，块注释使用 `/* */`

## 文件关联

插件自动将 `.sola` 文件关联为 Sola 语言。

## 示例

```sola
namespace myapp

use sola.lang.Exception;

public class Calculator {
    private int $result = 0;
    
    public function add(int $a, int $b): int {
        return $a + $b;
    }
    
    public function divide(int $a, int $b): int {
        if ($b == 0) {
            throw new Exception("除数不能为零");
        }
        return $a / $b;
    }
}

// 使用
$calc := new Calculator();
try {
    $result := $calc->divide(10, 0);
} catch (Exception $e) {
    echo "错误: " + $e->getMessage();
}
```

## 自定义

如需自定义颜色主题，可在 Sublime Text 的颜色主题文件中添加以下 scope 规则：

### 基础 Scopes
- `source.sola` - Sola 源代码
- `comment.line.double-slash.sola` - 单行注释
- `comment.block.sola` - 块注释
- `comment.block.documentation.sola` - 文档注释

### 关键字
- `keyword.control.sola` - 控制流关键字 (if, for, match, etc.)
- `keyword.declaration.sola` - 声明关键字 (class, function, type, etc.)
- `keyword.operator.type.sola` - 类型运算符 (is, as, as?)
- `keyword.other.sola` - 其他关键字

### 类型和修饰符
- `storage.type.sola` - 类型 (int, string, superarray, etc.)
- `storage.type.nullable.sola` - 可空类型标记 (?)
- `storage.modifier.sola` - 修饰符 (public, private, static, etc.)
- `storage.modifier.accessor.sola` - 访问器关键字 (get, set)
- `storage.type.annotation.sola` - 注解 (@Annotation)

### 字符串和数字
- `string.quoted.double.sola` - 双引号字符串
- `string.quoted.single.sola` - 单引号字符串
- `string.quoted.double.interpolated.sola` - 插值字符串 (#"...")
- `constant.numeric.integer.sola` - 整数
- `constant.numeric.float.sola` - 浮点数
- `constant.numeric.hex.sola` - 十六进制
- `constant.numeric.binary.sola` - 二进制

### 变量和函数
- `variable.other.sola` - 变量
- `variable.language.this.sola` - $this
- `variable.language.sola` - self, parent
- `entity.name.function.sola` - 函数名
- `entity.name.class.sola` - 类名
- `entity.other.inherited-class.sola` - 继承的类名

### 运算符
- `keyword.operator.assignment.sola` - 赋值运算符
- `keyword.operator.comparison.sola` - 比较运算符
- `keyword.operator.logical.sola` - 逻辑运算符
- `keyword.operator.arithmetic.sola` - 算术运算符
- `keyword.operator.bitwise.sola` - 位运算符
- `keyword.operator.accessor.sola` - 成员访问 (->, ::, .)
- `keyword.operator.spread.sola` - 扩展运算符 (...)
- `keyword.operator.type.union.sola` - 联合类型分隔符 (|)

### 其他
- `support.class.exception.sola` - 异常类
- `support.function.builtin.sola` - 内置函数
- `constant.language.boolean.sola` - 布尔值
- `constant.language.null.sola` - null

## License

MIT License












