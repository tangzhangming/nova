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

### 快速安装（Windows）

```powershell
# 复制到 Sublime Text Packages 目录
$dest = "$env:APPDATA\Sublime Text\Packages\Sola"
New-Item -ItemType Directory -Force -Path $dest
Copy-Item "Sola.sublime-syntax" $dest
Copy-Item "Sola.sublime-completions" $dest
```

### 快速安装（macOS）

```bash
dest=~/Library/Application\ Support/Sublime\ Text/Packages/Sola
mkdir -p "$dest"
cp Sola.sublime-syntax Sola.sublime-completions "$dest/"
```

### 快速安装（Linux）

```bash
dest=~/.config/sublime-text/Packages/Sola
mkdir -p "$dest"
cp Sola.sublime-syntax Sola.sublime-completions "$dest/"
```

## 功能

### 语法高亮

- 关键字 (if, for, class, function, etc.)
- 类型 (int, string, bool, etc.)
- 变量 ($variable)
- 字符串 (普通字符串和插值字符串 #"...")
- 注释 (单行 // 和多行 /* */)
- 数字 (整数、浮点数、十六进制、二进制)
- 运算符
- 类和函数定义
- 异常类 (Throwable, Exception, RuntimeException, Error)

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
- `try` → try-catch 块
- `trycf` → try-catch-finally 块

#### 声明
- `fn` → 函数定义
- `pfn` → public 函数
- `sfn` → static 函数
- `class` → 类定义
- `classext` → 继承类
- `interface` → 接口定义
- `enum` → 枚举定义
- `construct` → 构造函数
- `destruct` → 析构函数

#### 变量
- `var` → 带类型变量声明
- `vardecl` → 类型推导变量声明
- `arr` → 数组声明
- `map` → Map 声明

#### 内置函数
- `print`, `echo`, `len`, `typeof`
- `push`, `pop`, `strlen`, `substr`
- `str_split`, `str_join`, `str_replace`
- `to_int`, `to_float`, `to_string`, `to_bool`

#### 其他
- `namespace` → 命名空间
- `use` → 导入语句
- `useex` → 导入异常类
- `getset` → getter/setter 模板

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

- `source.sola` - Sola 源代码
- `keyword.control.sola` - 控制流关键字
- `keyword.declaration.sola` - 声明关键字
- `storage.type.sola` - 类型
- `storage.modifier.sola` - 修饰符
- `variable.other.sola` - 变量
- `entity.name.function.sola` - 函数名
- `entity.name.class.sola` - 类名
- `support.class.exception.sola` - 异常类
- `string.quoted.double.interpolated.sola` - 插值字符串

## License

MIT License






