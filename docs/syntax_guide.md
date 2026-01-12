# Sola 语言语法指南

> 本文档为 Sola 编程语言的完整语法参考，适合任何 AI 或开发者快速了解 Sola 的基础语法。

## 核心特性

- **纯面向对象**：代码通过类和方法组织
- **静态类型**：编译时类型检查，提供类型安全
- **泛型支持**：强大的泛型系统
- **现代语法**：模式匹配、属性访问器、箭头函数等
- **并发模型**：基于 CSP 的协程和通道
- **高性能**：JIT 编译和虚拟机优化

## 目录
1. [基本概念](#基本概念)
2. [类型系统](#类型系统)
   - [定长数组（NativeArray）](#定长数组nativearray)
   - [SuperArray（万能数组）](#superarray万能数组)
3. [变量与常量](#变量与常量)
4. [运算符](#运算符)
5. [控制结构](#控制结构)
   - [match 表达式（模式匹配）](#match-表达式模式匹配)
6. [函数](#函数)
7. [面向对象编程 (OOP)](#面向对象编程-oop)
   - [类声明](#类声明)
   - [属性访问器（C# 风格）](#属性访问器c-风格)
   - [对象创建与使用](#对象创建与使用)
   - [访问修饰符](#访问修饰符)
   - [继承](#继承)
   - [抽象类](#抽象类)
   - [接口](#接口)
8. [泛型](#泛型)
9. [异常处理](#异常处理)
10. [模块系统](#模块系统)
11. [并发编程](#并发编程)
12. [其他特性](#其他特性)

---

## 基本概念

### 程序组织

Sola 是一门**纯面向对象**的语言，程序通过类、接口和方法进行组织：

- ✅ **必须**：所有代码都组织在类中
- ❌ **不支持**：顶级语句（Sola 不支持在类外直接执行代码）
- 📦 **标准做法**：每个 `.sola` 文件包含一个主要的类定义
- 🚀 **入口点**：程序必须有 `public static function main()` 作为入口

### 程序执行

Sola 程序**强制要求**有 `main` 入口函数：

```sola
public class Application {
    public static function main() {
        Console::writeLine("Hello, Sola!");
    }
}
```

**重要说明**：
- ❌ **不支持顶级语句**：所有代码必须在类中
- ✅ **必须有 main 函数**：程序入口点必须是 `public static function main()`
- 📦 **一个文件一个类**：每个 `.sola` 文件包含一个主要的类定义

### 文件结构
```sola
namespace company.project      // 命名空间声明（可选）

use sola.collections.ArrayList; // 导入声明
use sola.lang.Exception;
use sola.io.Console;

// 类/接口/枚举声明
public class MyClass {
    public static function main() {
        // 程序入口点，必须是这个签名
        Console::writeLine("Hello, Sola!");
    }
}
```

> **重要**：Sola 强制要求 `main` 入口函数，不支持顶级语句。

### 注释
```sola
// 单行注释

/* 
 * 多行注释
 * 支持嵌套
 */

/**
 * 文档注释
 * @param name 参数说明
 * @return 返回值说明
 */
```

### 变量命名规则
- **变量必须以 `$` 开头**：`$name`, `$count`, `$userData`
- 标识符支持：字母、数字、下划线，首字符不能是数字
- 支持 Unicode 字符

---

## 类型系统

### 基本类型

| 类型 | 说明 | 示例 |
|------|------|------|
| `int` | 64位有符号整数 | `42`, `-100` |
| `i8`, `i16`, `i32`, `i64` | 固定宽度有符号整数 | `127i8` |
| `uint` | 64位无符号整数 | `100u` |
| `u8`, `u16`, `u32`, `u64` | 固定宽度无符号整数 | `255u8` |
| `byte` | 与 `u8` 等价 | - |
| `float` | 64位浮点数 | `3.14`, `1e10` |
| `f32`, `f64` | 固定宽度浮点数 | `3.14f32` |
| `bool` | 布尔值 | `true`, `false` |
| `string` | 字符串 | `"hello"`, `'world'` |
| `void` | 无返回值 | - |
| `null` | 空值 | `null` |

### 特殊类型说明

#### `unknown` 类型

`unknown` 是安全的顶类型，可以接收任何值，但**使用前必须进行类型检查**。

```sola
unknown $data = getData();

// ❌ 编译错误：unknown 类型不能直接使用
$data.name;
$data + 1;

// ✅ 必须先检查类型
if ($data is User) {
    print($data.name);  // OK，已收窄为 User
}

// ✅ 或使用 typeof 检查
if (typeof($data) == "string") {
    print($data.length);
}
```

**使用场景**：
- 处理外部数据（JSON、API 响应）
- 需要类型安全的动态类型处理

#### `dynamic` 类型

`dynamic` 跳过编译时类型检查，类似 C# 的 `dynamic`。**谨慎使用**。

```sola
dynamic $data = getExternalData();

// ⚠️ 编译通过，但运行时可能失败
$data.anyMethod();
$data.anyProperty;
```

**使用场景**：
- 快速原型开发
- 与外部动态系统交互
- 遗留代码迁移

#### `array` 类型

`array` 是无类型约束的动态数组，**不推荐在新代码中使用**。

```sola
// ⚠️ 不推荐：无类型安全
array $data = [1, 2, 3];

// ✅ 推荐：使用类型化数组
int[] $numbers = int{1, 2, 3};
string[] $names = string{"Alice", "Bob"};

// ✅ 推荐：使用泛型集合
ArrayList<int> $list = new ArrayList<int>();
```

**使用场景**：
- 与外部 API 交互（如 JSON 解析结果）
- 需要混合类型的场景（建议用 SuperArray 代替）

### 数字字面量
```sola
$decimal := 42;          // 十进制
$hex := 0xFF;            // 十六进制
$binary := 0b1010;       // 二进制
$float := 3.14;          // 浮点数
$scientific := 1.5e10;   // 科学计数法
```

### 字符串
```sola
$str1 := "双引号字符串";
$str2 := '单引号字符串';

// 转义字符
$escaped := "Hello\nWorld\t!";  // \n 换行, \t 制表符, \\ 反斜杠

// 插值字符串（使用 #"..."）
$name := "Sola";
$greeting := #"Hello, {$name}!";  // Hello, Sola!
```

### 复合类型

Sola 有两种完全不同的数组类型，**不能互相赋值**：

| 类型 | 语法 | 用途 |
|------|------|------|
| 类型化数组 `T[]` | `int{1, 2, 3}` | 静态类型安全，推荐使用 |
| 万能数组 `SuperArray` | `[1, 2, 3]` 或 `["k" => v]` | 动态类型，JSON/外部数据 |

> ⚠️ **重要**: `int[]` 和 `SuperArray` 是**完全不同的类型**，不能互相赋值！

#### 定长数组（NativeArray）

定长数组是 Sola 的原生数组类型，具有**固定长度**和**固定元素类型**。创建后长度不可变，但元素值可以修改。

##### 创建语法

```sola
// 方式1：new + 指定大小（元素为默认值）
int[] $arr1 = new int[5];           // 5个元素，默认值0
float[] $arr2 = new float[10];      // 10个元素，默认值0.0
string[] $arr3 = new string[3];     // 3个元素，默认值""

// 方式2：new + 初始化列表
int[] $arr4 = new int[] { 1, 2, 3, 4, 5 };
string[] $arr5 = new string[] { "Alice", "Bob", "Charlie" };

// 方式3：Go 风格简写（类型推断）
$arr6 := int{1, 2, 3};              // 推断为 int[]
$arr7 := string{"a", "b", "c"};     // 推断为 string[]

// 二维数组
int[][] $matrix = new int[][] {
    int{1, 2, 3},
    int{4, 5, 6},
    int{7, 8, 9}
};
```

##### 数组特性

| 特性 | 说明 |
|------|------|
| 定长 | 创建后长度不可变 |
| 类型化 | 元素类型在编译时确定，所有元素必须是同一类型 |
| 高性能 | 使用原生内存存储，JIT 可直接操作指针 |
| 值比较 | 两个数组使用 `==` 比较时，逐元素比较值 |
| 边界检查 | 访问越界会抛出 `ArrayIndexOutOfBoundsException` |

##### 数组访问

```sola
int[] $arr = new int[] { 10, 20, 30 };

// 索引访问
$first := $arr[0];  // 获取元素: 10
$arr[1] = 25;       // 修改元素

// 获取长度
$length := $arr.length;   // 3

// 遍历数组
foreach ($arr as $value) {
    Console::writeLine($value);
}

// 带索引遍历
foreach ($arr as $index => $value) {
    Console::writeLine(#"{$index}: {$value}");
}
```

##### 数组语法糖方法

定长数组提供丰富的内置方法，通过 `.` 语法糖调用：

```sola
int[] $arr = new int[] { 3, 1, 4, 1, 5, 9, 2, 6 };

// 查找方法
$arr.indexOf(4);        // 返回 2（第一次出现的索引）
$arr.lastIndexOf(1);    // 返回 3（最后一次出现的索引）
$arr.contains(5);       // 返回 true

// 排序方法（原地修改）
$arr.sort();            // 升序排序: [1, 1, 2, 3, 4, 5, 6, 9]
$arr.sortDesc();        // 降序排序: [9, 6, 5, 4, 3, 2, 1, 1]
$arr.reverse();         // 反转数组

// 切片方法（返回新数组）
int[] $sub = $arr.slice(2, 5);    // 获取索引2到4的元素
int[] $merged = $arr.concat($sub); // 连接两个数组

// 拷贝方法
int[] $copy = $arr.copy();        // 深拷贝
SuperArray $sa = $arr.toSuperArray(); // 转换为 SuperArray

// 数值方法（仅数值数组可用）
$arr.sum();             // 求和
$arr.max();             // 最大值
$arr.min();             // 最小值
$arr.average();         // 平均值
```

##### 数组比较

定长数组支持值比较：

```sola
int[] $a = new int[] { 1, 2, 3 };
int[] $b = new int[] { 1, 2, 3 };
int[] $c = new int[] { 1, 2, 4 };

$result1 := $a == $b;  // true（逐元素比较，值相等）
$result2 := $a == $c;  // false（第3个元素不同）
Console::writeLine($result1);
Console::writeLine($result2);
```

##### 数组与 SuperArray 的区别

| 特性 | 定长数组 `T[]` | SuperArray |
|------|---------------|------------|
| 长度 | 固定，创建后不可变 | 动态，可增删 |
| 元素类型 | 单一类型，编译时确定 | 动态类型，可混合 |
| 键类型 | 仅整数索引 | 整数/字符串混合键 |
| 性能 | 高（原生内存存储） | 较低（哈希表实现） |
| 用途 | 已知大小的数据集合 | JSON/外部数据/动态场景 |

```sola
// ✅ 正确：类型匹配
int[] $a = new int[] { 1, 2, 3 };
int[] $b = $a;  // OK

// ❌ 错误：类型不兼容
SuperArray $arr = [1, 2, 3];
int[] $c = $arr;  // 编译错误！SuperArray 不能赋给 int[]

// ✅ 正确：显式转换
int[] $d = new int[] { 1, 2, 3 };
SuperArray $e = $d.toSuperArray();  // OK
```

##### 底层原理（高级）

定长数组使用**类型化原生存储**：

- **内存布局**：连续内存块，每个元素 8 字节
- **元素存储**：
  - `int` → 直接存储 `int64`
  - `float` → 直接存储 `float64`
  - `bool` → 存储为 `int64`（0/1）
  - `string`/`object` → 存储指针
- **JIT 友好**：编译器可直接生成内存访问指令，无需类型拆装箱

#### Map 类型
```sola
// 类型化映射
map[string]int $ages = map[string]int{
    "Alice": 25,
    "Bob": 30
};
```

#### SuperArray（万能数组）

SuperArray 是 Sola 提供的**动态类型**数组，支持整数和字符串混合键。仅适用于与动态数据交互的场景（如 JSON 解析）。

```sola
// 自动索引
SuperArray $arr = [1, 2, 3];

// 关联数组（键值对）
SuperArray $data = [
    "name" => "Sola",
    "version" => 1,
    0 => "first"
];

// ✅ 正确：使用 SuperArray 类型
SuperArray $items = ["a", "b", "c"];

// ❌ 错误：不能把 SuperArray 当作类型化数组
string[] $names = ["a", "b", "c"];  // 编译错误！应使用 string{"a", "b", "c"}
```

> **何时使用 SuperArray**：
> - JSON 解析结果
> - 需要混合类型键（整数 + 字符串）
> - 与外部动态 API 交互
>
> **何时使用类型化数组**：
> - 所有其他情况（推荐默认选择）

```sola
// 访问
$value := $arr[0];         // 1
$name := $data["name"];    // Sola
Console::writeLine($value);
Console::writeLine($name);
```

> **静态类型建议**：对于已知结构的数据，推荐使用类型化数组 `T[]`、`map[K]V` 或自定义类，以获得更好的类型安全和IDE支持。

### 可空类型
```sola
?string $name = null;           // 可空字符串
?int $age = 25;                 // 可空整数

// 可空类型必须在使用前检查
if ($name != null) {
    Console::writeLine($name);
}
```

### 联合类型
```sola
// 函数参数或返回值可以是多种类型
public function process(int|string $value): int|null {
    // ...
}
```

### 类型别名与新类型

Sola 支持两种类型定义方式：

#### 类型别名（Type Alias）

类型别名创建与目标类型完全兼容的新名称，可以互相替换使用：

```sola
// 使用 = 符号定义类型别名
type StringList = string[];
type UserMap = map[string]User;
type Callback = func(int): bool;

// 使用类型别名
StringList $names = string{"Alice", "Bob"};
UserMap $users = map[string]User{};

// 类型别名与原类型完全兼容
string[] $arr = string{"a", "b"};
StringList $list = $arr;  // OK：类型别名与原类型兼容
```

#### 新类型（Distinct Type）

新类型创建与基础类型不兼容的独立类型，需要显式转换：

```sola
// 使用空格（无 = 符号）定义新类型
type UserID int;
type OrderID int;
type EmailAddress string;

// 新类型需要显式转换
UserID $userId = 1001 as UserID;
OrderID $orderId = 2001 as OrderID;

// ❌ 错误：新类型之间不能直接赋值
// $userId = $orderId;  // 编译错误！UserID 和 OrderID 是不同类型

// ✅ 正确：显式转换
$userId = $orderId as int as UserID;  // 先转为 int，再转为 UserID
```

#### 类型别名 vs 新类型

| 特性 | 类型别名 (`type X = Y`) | 新类型 (`type X Y`) |
|------|------------------------|---------------------|
| 与基础类型兼容 | ✅ 完全兼容 | ❌ 需要显式转换 |
| 类型安全 | 较低（只是重命名） | 较高（独立类型） |
| 使用场景 | 简化复杂类型名 | 防止类型混用 |

**使用建议**：
- 使用**类型别名**简化复杂类型（如 `map[string]func(int): bool`）
- 使用**新类型**区分语义不同但底层类型相同的值（如 `UserID` 和 `OrderID`）

---

## 变量与常量

### 变量声明

#### 显式类型声明
```sola
int $count = 0;
string $name = "Sola";
bool $active = true;
float $price = 19.99;
```

#### 类型推断（使用 `:=`）
```sola
$count := 0;          // 推断为 int
$name := "Sola";      // 推断为 string
$active := true;      // 推断为 bool
$price := 19.99;      // 推断为 float
```

#### 多变量声明
```sola
// 解构赋值
$a, $b := getValues();  // 函数返回多个值
```

### 常量
```sola
// 类内常量
public class Config {
    public const string VERSION = "1.0.0";
    private const int MAX_SIZE = 100;
    protected const float PI = 3.14159;
}

// 使用
Console::writeLine(Config::VERSION);
```

---

## 运算符

### 算术运算符
```sola
$a + $b     // 加法
$a - $b     // 减法
$a * $b     // 乘法
$a / $b     // 除法
$a % $b     // 取模
$a++        // 后置自增
++$a        // 前置自增
$a--        // 后置自减
--$a        // 前置自减
```

#### 算术运算符类型规则（严格类型检查）

Sola 是强类型语言，**不允许隐式类型转换**。算术运算符要求操作数类型完全匹配：

| 运算符 | 允许的类型组合 | 禁止的类型组合 |
|--------|---------------|---------------|
| `+` | `int + int`、`float + float`、`string + string` | `int + float`、`string + int` 等 |
| `-` `*` `/` `%` | `int op int`、`float op float` | `int op float` 等不同类型 |

```sola
// ✅ 正确：相同类型运算
int $a = 10;
int $b = 20;
Console::writeLine($a + $b);      // 30

float $x = 1.5;
float $y = 2.5;
Console::writeLine($x + $y);      // 4.0

string $s1 = "Hello";
string $s2 = " World";
Console::writeLine($s1 + $s2);    // "Hello World"

// ❌ 错误：不同类型不能运算
int $n = 10;
float $f = 3.14;
// Console::writeLine($n + $f);   // 编译错误：运算符 '+' 不能用于 int 和 float 类型

string $str = "100";
int $num = 50;
// Console::writeLine($str + $num);  // 编译错误：运算符 '+' 不能用于 string 和 int 类型
```

**如需混合类型运算，必须显式类型转换：**

```sola
int $n = 10;
float $f = 3.14;

// 方法1：将 int 转为 float
$result1 := ($n as float) + $f;   // 13.14
Console::writeLine($result1);

// 方法2：将 float 转为 int（截断小数）
$result2 := $n + ($f as int);     // 13
Console::writeLine($result2);
```

### 比较运算符
```sola
$a == $b    // 相等
$a != $b    // 不等
$a < $b     // 小于
$a <= $b    // 小于等于
$a > $b     // 大于
$a >= $b    // 大于等于
```

### 逻辑运算符
```sola
$a && $b    // 逻辑与
$a || $b    // 逻辑或
!$a         // 逻辑非
```

### 位运算符
```sola
$a & $b     // 按位与
$a | $b     // 按位或
$a ^ $b     // 按位异或
~$a         // 按位取反
$a << $b    // 左移
$a >> $b    // 右移
```

### 赋值运算符
```sola
$a = $b     // 赋值
$a += $b    // 加法赋值
$a -= $b    // 减法赋值
$a *= $b    // 乘法赋值
$a /= $b    // 除法赋值
$a %= $b    // 取模赋值
$a := $b    // 声明并赋值（类型推断）
```

### 三元运算符
```sola
$result := $condition ? $trueValue : $falseValue;
```

### 类型运算符
```sola
// 类型检查
if ($obj is User) {
    // $obj 在此分支内被视为 User 类型
}

// 类型断言（强制转换）
$user := $obj as User;           // 失败抛出异常
$user := $obj as? User;          // 安全转换，失败返回 null
```

---

## 控制结构

### if / elseif / else
```sola
if ($age < 18) {
    Console::writeLine("未成年");
} elseif ($age < 60) {
    Console::writeLine("成年人");
} else {
    Console::writeLine("老年人");
}

// 条件中的类型收窄
if ($value is string) {
    // $value 在此作用域内是 string 类型
    $length := $value.length;
}
```

### switch 语句与表达式

Sola 的 switch 支持两种形式：**语句形式**（执行操作）和**表达式形式**（返回值）。单个 case 支持多个值。

#### Switch 表达式（返回值）

switch 表达式可以赋值给变量，每个 case 使用 `=>` 返回值：

```sola
// 单值匹配
$dayName := switch ($day) {
    case 1 => "周一",
    case 2 => "周二",
    case 3 => "周三",
    default => "未知"
};

// 多值匹配
$category := switch ($day) {
    case 1, 2, 3, 4, 5 => "工作日",
    case 6, 7 => "周末",
    default => "无效"
};

// 复杂表达式
$price := switch ($level) {
    case 1 => calculateBasicPrice(),
    case 2, 3 => calculatePremiumPrice() * 0.9,
    case 4, 5 => getVipPrice(),
    default => throw new Exception("Invalid level")
};
```

#### Switch 语句（多行 body）

switch 语句用于执行多条语句，使用 `:` 和 `break`：

```sola
switch ($status) {
    case 0, 1:
        Console::writeLine("待处理");
        sendNotification();
        break;
    case 2, 3, 4:
        Console::writeLine("进行中");
        updateProgress();
        break;
    default:
        Console::writeLine("已完成");
}
```

#### Switch 语法规则

1. **多值 case**：单个 case 可以匹配多个值，用逗号分隔：`case 1, 2, 3`
2. **箭头形式 `=>`**：用于单行表达式，自动 break，不需要显式写 break
3. **冒号形式 `:`**：用于多行语句块，需要显式 break
4. **禁止混合**：同一个 switch 中所有 case 必须使用相同形式（全部 `=>` 或全部 `:`）
5. **Switch 表达式**：所有 case 使用 `=>` 时，switch 可作为表达式返回值，类型必须兼容
6. **穷尽性**：switch 表达式建议包含 default 分支以确保所有情况都被覆盖

#### Switch vs Match

| 特性 | switch | match |
|------|--------|-------|
| 用途 | 值匹配 | 值匹配 + 类型匹配 + 模式匹配 |
| 语法 | `case value =>` | `pattern =>` |
| 类型模式 | ❌ 不支持 | ✅ 支持 `int $n` |
| 守卫条件 | ❌ 不支持 | ✅ 支持 `if $n > 0` |
| 多值 | ✅ `case 1, 2, 3` | ❌ 需分开写 |
| 多行 body | ✅ 支持 `:` 形式 | ❌ 仅表达式 |

**使用建议**：
- 简单值匹配 + 需要多行操作：用 **switch**
- 需要类型匹配或守卫条件：用 **match**

### match 表达式（模式匹配）

`match` 是一个表达式，返回匹配分支的值。支持值匹配、类型匹配和守卫条件。

#### 值匹配
```sola
$day := 3;
$dayName := match ($day) {
    1 => "周一",
    2 => "周二",
    3 => "周三",
    4 => "周四",
    5 => "周五",
    6 => "周六",
    7 => "周日",
    _ => "未知"   // 通配符（默认分支）
};
Console::writeLine($dayName);  // 输出: 周三
```

#### 类型匹配（带变量绑定）
```sola
$value := 42;
$result := match ($value) {
    int $n => $n * 2,        // 类型匹配并绑定变量 $n
    string $s => $s.length,  // 匹配字符串类型
    _ => 0                   // 通配符
};
Console::writeLine($result);  // 输出: 84
```

#### 带守卫条件的匹配
```sola
$score := 85;
$grade := match ($score) {
    int $s if $s >= 90 => "A",   // 守卫条件: $s >= 90
    int $s if $s >= 80 => "B",
    int $s if $s >= 70 => "C",
    int $s if $s >= 60 => "D",
    _ => "F"
};
Console::writeLine($grade);  // 输出: B
```

#### match 语法规则

1. **match 是表达式**：必须返回一个值，可以赋值给变量
2. **按顺序匹配**：第一个匹配成功的分支执行
3. **通配符 `_`**：匹配任何值，通常放在最后作为默认分支
4. **值匹配**：精确匹配字面量值（整数、字符串、布尔等）
5. **类型模式**：`int $n` 匹配 int 类型并将值绑定到 `$n`
6. **守卫条件**：使用 `if` 关键字添加额外条件，绑定的变量可在守卫中使用

### for 循环
```sola
// 经典 for 循环
for ($i := 0; $i < 10; $i++) {
    Console::writeLine($i);
}

// 省略部分
for (; $i < 10; ) {
    $i++;
}
```

### foreach 循环
```sola
// 遍历数组
foreach ($items as $item) {
    Console::writeLine($item);
}

// 带索引/键遍历
foreach ($items as $index => $item) {
    Console::writeLine(#"{$index}: {$item}");
}

// 遍历 Map
foreach ($map as $key => $value) {
    Console::writeLine(#"{$key} = {$value}");
}
```

### while 循环
```sola
while ($count > 0) {
    Console::writeLine($count);
    $count--;
}
```

### do-while 循环
```sola
do {
    Console::writeLine($count);
    $count--;
} while ($count > 0);
```

### break 和 continue
```sola
for ($i := 0; $i < 10; $i++) {
    if ($i == 5) {
        break;      // 退出循环
    }
    if ($i % 2 == 0) {
        continue;   // 跳过本次迭代
    }
    Console::writeLine($i);
}
```

---

## 函数

### 函数声明
```sola
// 基本函数
function greet(string $name): string {
    return "Hello, " + $name;
}

// 无返回值
function printMessage(string $msg): void {
    Console::writeLine($msg);
}

// 带默认参数
function connect(string $host, int $port = 3306): bool {
    // ...
}

// 可变参数
function sum(int ...$numbers): int {
    $total := 0;
    foreach ($numbers as $n) {
        $total += $n;
    }
    return $total;
}
```

### 多返回值
```sola
function divide(int $a, int $b): (int, int) {
    return $a / $b, $a % $b;
}

// 调用
$quotient, $remainder := divide(10, 3);
```

### 闭包（匿名函数）
```sola
// 完整语法
$add := function(int $a, int $b): int {
    return $a + $b;
};

// 捕获外部变量
$multiplier := 2;
$double := function(int $n): int use ($multiplier) {
    return $n * $multiplier;
};
```

### 箭头函数
```sola
// 简洁的单表达式函数
$square := (int $x): int => $x * $x;

// 作为参数传递
$list->filter((int $x): bool => $x > 0);
$list->map((int $x): int => $x * 2);
```

### 函数类型参数
```sola
public function process(
    int[] $data,
    function(int $item): bool $predicate
): int[] {
    $result := int{};
    foreach ($data as $item) {
        if ($predicate($item)) {
            $result[] = $item;
        }
    }
    return $result;
}

// 泛型版本（更灵活）
public function filter<T>(
    T[] $data,
    function(T $item): bool $predicate
): T[] {
    $result := T{};
    foreach ($data as $item) {
        if ($predicate($item)) {
            $result[] = $item;
        }
    }
    return $result;
}
```

---

## 面向对象编程 (OOP)

### 类声明
```sola
public class User {
    // 普通属性
    private int $id;
    protected string $name;
    public string $email;
    
    // 静态属性
    private static int $count = 0;
    
    // 常量
    public const string DEFAULT_ROLE = "user";
    
    // 构造函数
    public function __construct(int $id, string $name, string $email) {
        $this->id = $id;
        $this->name = $name;
        $this->email = $email;
        self::$count++;
    }
    
    // 方法
    public function getName(): string {
        return $this->name;
    }
    
    public function setName(string $name): void {
        $this->name = $name;
    }
    
    // 静态方法
    public static function getCount(): int {
        return self::$count;
    }
    
    // 转字符串方法（普通方法，需显式调用）
    public function toString(): string {
        return #"User({$this->name})";
    }
}
```

### 属性访问器（C# 风格）

Sola 支持 C# 风格的属性访问器，提供更优雅的属性封装方式。

#### 自动属性（Auto-properties）

自动属性会自动生成后备字段和 getter/setter 方法：

```sola
public class User {
    // 可读写自动属性
    public string $name { get; set; }
    
    // 只读自动属性（只能在构造函数中设置）
    public string $id { get; }
    
    // 公开读取，私有写入
    public int $count { get; private set; }
    
    // 静态自动属性
    public static int $total { get; set; }
    
    public function __construct(string $id, string $name) {
        $this->id = $id;  // 只读属性可在构造函数中设置
        $this->name = $name;
    }
}

// 使用
$user := new User("001", "Alice");
Console::writeLine($user->name);  // 自动调用 getter
$user->name = "Bob";              // 自动调用 setter
// $user->id = "002";             // 错误：只读属性不能赋值
```

#### 表达式体属性（Expression-bodied properties）

表达式体属性提供简洁的计算属性语法：

```sola
public class Person {
    private string $firstName;
    private string $lastName;
    
    // 只读计算属性
    public string $fullName => $this->firstName + " " + $this->lastName;
    
    // 静态表达式体属性
    public static string $version => "1.0.0";
    
    public function __construct(string $firstName, string $lastName) {
        $this->firstName = $firstName;
        $this->lastName = $lastName;
    }
}

// 使用
$person := new Person("张", "三");
Console::writeLine($person->fullName);  // 输出: 张 三
```

#### 完整属性（Full properties）

完整属性允许自定义 getter 和 setter 逻辑：

```sola
public class User {
    private int $_age;
    
    // 完整属性（方法体）
    public int $age {
        get {
            return $this->_age;
        }
        set {
            if ($value < 0) {
                throw new ArgumentException("年龄不能为负数");
            }
            $this->_age = $value;
        }
    }
    
    // 表达式体 getter/setter
    public int $age2 {
        get => $this->_age;
        set => $this->_age = $value;
    }
    
    // 只读属性（只有 getter）
    public string $status {
        get {
            if ($this->_age < 18) {
                return "未成年";
            } else if ($this->_age < 60) {
                return "成年人";
            } else {
                return "老年人";
            }
        }
    }
    
    // 不同可见性的访问器
    public string $email {
        get;
        private set;  // 只有类内部可以设置
    }
}

// 使用
$user := new User();
$user->age = 25;        // 调用 setter，验证通过
// $user->age = -5;      // 抛出异常
Console::writeLine($user->age);     // 调用 getter
Console::writeLine($user->status);  // 调用 getter，返回计算值
```

#### 属性访问器特性

- **自动生成方法**：属性访问器会自动生成 `get_$name` 和 `set_$name` 方法
- **透明访问**：使用 `$obj->property` 语法时自动调用相应的访问器
- **后备字段**：自动属性会自动创建 `__prop_$name` 后备字段
- **setter 参数**：setter 中使用 `$value` 变量接收赋值
- **可见性控制**：getter 和 setter 可以有不同的可见性修饰符
- **静态支持**：支持静态属性访问器

#### 属性访问器 vs 普通方法

```sola
public class Example {
    // 使用属性访问器（推荐）
    public string $name { get; set; }
    
    // 等价于传统方法（不推荐）
    private string $_name;
    public function getName(): string {
        return $this->_name;
    }
    public function setName(string $name): void {
        $this->_name = $name;
    }
}

// 两种方式使用相同
$obj := new Example();
$obj->name = "Sola";              // 属性访问器
Console::writeLine($obj->name);   // 属性访问器

$obj->setName("Sola");            // 传统方法
Console::writeLine($obj->getName());  // 传统方法
```

### 对象创建与使用
```sola
// 创建对象
$user := new User(1, "Alice", "alice@example.com");

// 访问属性和方法
Console::writeLine($user->email);
$user->setName("Bob");

// 静态访问
Console::writeLine(User::getCount());
Console::writeLine(User::DEFAULT_ROLE);
```

### 访问修饰符

| 修饰符 | 说明 |
|--------|------|
| `public` | 任何地方可访问 |
| `protected` | 本类和子类可访问 |
| `private` | 仅本类可访问 |

### 类修饰符

| 修饰符 | 说明 |
|--------|------|
| `abstract` | 抽象类/方法，不能实例化 |
| `final` | 最终类/方法，不能被继承/重写 |
| `static` | 静态成员 |

### 继承
```sola
public class Animal {
    protected string $name;
    
    public function __construct(string $name) {
        $this->name = $name;
    }
    
    public function speak(): string {
        return "...";
    }
}

public class Dog extends Animal {
    private string $breed;
    
    public function __construct(string $name, string $breed) {
        parent::__construct($name);  // 调用父类构造函数
        $this->breed = $breed;
    }
    
    // 重写方法
    public function speak(): string {
        return "Woof!";
    }
    
    public function getBreed(): string {
        return $this->breed;
    }
}
```

### 抽象类
```sola
abstract class Shape {
    protected string $color;
    
    public function __construct(string $color) {
        $this->color = $color;
    }
    
    // 抽象方法（子类必须实现）
    abstract public function area(): float;
    
    // 普通方法
    public function getColor(): string {
        return $this->color;
    }
}

public class Circle extends Shape {
    private float $radius;
    
    public function __construct(string $color, float $radius) {
        parent::__construct($color);
        $this->radius = $radius;
    }
    
    public function area(): float {
        return 3.14159 * $this->radius * $this->radius;
    }
}
```

### 接口
```sola
public interface Drawable {
    public function draw(): void;
    public function resize(float $factor): void;
}

public interface Serializable {
    public function serialize(): string;
    public function deserialize(string $data): void;
}

// 实现多个接口
public class Canvas implements Drawable, Serializable {
    public function draw(): void {
        // 实现
    }
    
    public function resize(float $factor): void {
        // 实现
    }
    
    public function serialize(): string {
        // 实现
    }
    
    public function deserialize(string $data): void {
        // 实现
    }
}
```

### 接口继承
```sola
public interface ICollection<T> extends IIterable<T> {
    public function size(): int;
    public function isEmpty(): bool;
    public function add(T $element): bool;
}
```

### 特殊关键字
```sola
$this       // 当前对象实例
self        // 当前类（用于静态访问）
parent      // 父类（用于调用父类方法）

// 示例
public function example(): void {
    $this->method();           // 调用实例方法
    self::staticMethod();      // 调用当前类静态方法
    parent::__construct();     // 调用父类构造函数
}
```

### 特殊方法

Sola 支持以下特殊方法（由运行时自动调用）：

| 方法 | 说明 |
|------|------|
| `__construct` | 构造函数，创建对象时自动调用 |
| `__destruct` | 析构函数，对象被垃圾回收时调用 |

```sola
public class Resource {
    private int $handle;
    
    public function __construct() {
        $this->handle = openResource();
    }
    
    public function __destruct() {
        closeResource($this->handle);
    }
}
```

> **注意**：Sola 是静态类型语言，不支持 PHP 风格的魔术方法（如 `__toString`、`__get`、`__set` 等）。如需类似功能，请使用显式方法或属性访问器。

### final 类和方法
```sola
// 不能被继承的类
final class Singleton {
    // ...
}

// 不能被重写的方法
public class Base {
    final public function criticalMethod(): void {
        // 子类不能重写此方法
    }
}
```

---

## 泛型

### 泛型类
```sola
public class Box<T> {
    private T $value;
    
    public function __construct(T $value) {
        $this->value = $value;
    }
    
    public function get(): T {
        return $this->value;
    }
    
    public function set(T $value): void {
        $this->value = $value;
    }
}

// 使用
$intBox := new Box<int>(42);
$strBox := new Box<string>("hello");
```

### 泛型接口
```sola
public interface IComparable<T> {
    public function compareTo(T $other): int;
}

public class Integer implements IComparable<Integer> {
    private int $value;
    
    public function compareTo(Integer $other): int {
        return $this->value - $other->value;
    }
}
```

### 泛型方法
```sola
public class Utils {
    public static function swap<T>(T $a, T $b): (T, T) {
        return $b, $a;
    }
    
    public static function identity<T>(T $value): T {
        return $value;
    }
}
```

### 类型约束
```sola
// extends 约束
public class SortedList<T extends IComparable<T>> {
    public function add(T $item): void {
        // T 必须实现 IComparable<T>
    }
}

// implements 约束
public class Container<T implements ISerializable> {
    // T 必须实现 ISerializable 接口
}

// where 子句（复杂约束）
public class Repository<T> where T extends Entity implements IIdentifiable {
    // ...
}
```

### 多类型参数
```sola
public class Pair<K, V> {
    private K $key;
    private V $value;
    
    public function __construct(K $key, V $value) {
        $this->key = $key;
        $this->value = $value;
    }
    
    public function getKey(): K {
        return $this->key;
    }
    
    public function getValue(): V {
        return $this->value;
    }
}

public interface IMap<K, V> {
    public function get(K $key): V;
    public function put(K $key, V $value): V;
}
```

---

## 异常处理

### 异常层次结构
```
Throwable
├── Error          (不可恢复的错误)
└── Exception      (可捕获的异常)
    ├── RuntimeException
    ├── InvalidOperationException
    ├── ArgumentException
    └── ...
```

### try-catch-finally
```sola
try {
    $result := riskyOperation();
} catch (IOException $e) {
    Console::error("IO错误: " + $e->getMessage());
} catch (Exception $e) {
    Console::error("一般错误: " + $e->getMessage());
} finally {
    // 无论是否异常都会执行
    cleanup();
}
```

### 抛出异常
```sola
public function divide(int $a, int $b): int {
    if ($b == 0) {
        throw new DivideByZeroException("除数不能为零");
    }
    return $a / $b;
}
```

### 自定义异常
```sola
public class ValidationException extends Exception {
    private string[] $errors;
    
    public function __construct(string $message, string[] $errors) {
        parent::__construct($message);
        $this->errors = $errors;
    }
    
    public function getErrors(): string[] {
        return $this->errors;
    }
}
```

### 异常链
```sola
try {
    // ...
} catch (SQLException $e) {
    throw new DataAccessException("数据访问失败", 0, $e);
}
```

---

## 模块系统

### 命名空间
```sola
// 声明命名空间
namespace company.project.models

public class User {
    // ...
}
```

### 导入
```sola
// 导入单个类
use sola.collections.ArrayList;

// 导入多个类
use sola.lang.{Exception, RuntimeException, Str};

// 导入并起别名
use sola.collections.HashMap as Map;

// 使用
$list := new ArrayList<int>();
$map := new Map<string, int>();
```

### 完全限定名
```sola
// 不导入时使用完整路径
$list := new sola.collections.ArrayList<int>();
```

---

## 并发编程

Sola 采用 CSP（Communicating Sequential Processes）并发模型，通过协程和通道实现高效的并发编程。

### go 语句

使用 `go` 关键字启动协程：

```sola
// 启动协程执行函数
go processData();

// 启动协程执行方法
go $worker->run();

// 启动协程执行闭包
go function(): void {
    Console::writeLine("Hello from goroutine");
}();

// 带参数的闭包
$count := 100;
go function(): void use ($count) {
    Console::writeLine("Count: " + $count);
}();
```

### Channel\<T\> 通道

通道是协程间通信的主要机制：

```sola
use sola.concurrent.Channel;

// 创建无缓冲通道（同步通道）
$ch := new Channel<int>();

// 创建有缓冲通道
$buffered := new Channel<string>(10);

// 发送数据（可能阻塞）
$ch->send(42);

// 接收数据（可能阻塞）
$value := $ch->receive();

// 非阻塞发送
if ($ch->trySend(100)) {
    Console::writeLine("Sent successfully");
}

// 非阻塞接收
$msg := $ch->tryReceive();
if ($msg != null) {
    Console::writeLine("Received: " + $msg);
}

// 关闭通道
$ch->close();
```

### select 语句

`select` 用于多路通道选择：

```sola
use sola.concurrent.Channel;

$ch1 := new Channel<int>();
$ch2 := new Channel<string>();
$quit := new Channel<bool>();

select {
    case $num := $ch1->receive():
        Console::writeLine("Received number: " + $num);
        
    case $msg := $ch2->receive():
        Console::writeLine("Received message: " + $msg);
        
    case $quit->receive():
        Console::writeLine("Quit signal received");
        return;
        
    default:
        Console::writeLine("No channel ready");
}
```

### WaitGroup 等待组

等待一组协程完成：

```sola
use sola.concurrent.WaitGroup;

$wg := new WaitGroup();

for ($i := 0; $i < 10; $i++) {
    $wg->add();
    go function(): void use ($i, $wg) {
        processTask($i);
        $wg->done();
    }();
}

$wg->wait();
Console::writeLine("All tasks done!");
```

### 并发模式示例

#### 生产者-消费者

```sola
use sola.concurrent.Channel;

$jobs := new Channel<int>(100);
$results := new Channel<int>(100);

// 启动工作协程
for ($w := 0; $w < 3; $w++) {
    go function(): void use ($jobs, $results) {
        loop {
            $job := $jobs->tryReceive();
            if ($job == null) {
                break;
            }
            $results->send($job * 2);
        }
    }();
}

// 发送任务
for ($i := 0; $i < 10; $i++) {
    $jobs->send($i);
}
$jobs->close();

// 收集结果
for ($i := 0; $i < 10; $i++) {
    $result := $results->receive();
    Console::writeLine($result);
}
```

#### 超时控制

```sola
use sola.concurrent.Channel;

$result := new Channel<string>();

// 启动工作协程
go function(): void use ($result) {
    // 模拟耗时操作
    $data := fetchData();
    $result->send($data);
}();

// 使用 select 实现超时
select {
    case $data := $result->receive():
        Console::writeLine("Got result: " + $data);
        
    default:
        // 超时处理（实际超时需要配合定时器通道）
        Console::writeLine("Operation timed out");
}
```

---

## 其他特性

### 枚举
```sola
// 简单枚举
enum Color {
    RED,
    GREEN,
    BLUE
}

// 带值的枚举
enum Status: int {
    PENDING = 0,
    ACTIVE = 1,
    CLOSED = 2
}

// 字符串枚举
enum HttpMethod: string {
    GET = "GET",
    POST = "POST",
    PUT = "PUT",
    DELETE = "DELETE"
}

// 使用
$color := Color::RED;
$status := Status::ACTIVE;
```

### 注解

> **详细设计文档**: [annotation.md](./annotation.md)

Sola 使用**强类型注解**（PHP 8 风格），注解是普通类，用 `@Attribute` 标记。

#### 定义注解

```sola
namespace sola.orm.annotation

use sola.annotation.Attribute;
use sola.annotation.Target;
use sola.annotation.ElementType;

// 注解是普通类，用 @Attribute 标记
@Attribute
@Target([ElementType::CLASS])
public class Table {
    public string $name;
    public string $charset;
    
    public function __construct(string $name, string $charset = "utf8mb4") {
        $this->name = $name;
        $this->charset = $charset;
    }
}

@Attribute
@Target([ElementType::PROPERTY])
public class Column {
    public string $name;
    public bool $nullable;
    
    public function __construct(string $name = "", bool $nullable = true) {
        $this->name = $name;
        $this->nullable = $nullable;
    }
}
```

#### 使用注解

```sola
use sola.orm.annotation.Table;
use sola.orm.annotation.Column;

@Table("users")
public class User {
    
    @Column("id", nullable = false)
    public int $id;
    
    @Column  // 使用默认值
    public string $name;
}
```

#### 参数传递方式

```sola
// 命名参数（推荐）
@Column(name = "user_id", nullable = false)

// 位置参数（按构造函数顺序）
@Table("users")

// 混合使用（位置参数在前）
@Column("user_id", nullable = false)
```

#### 元注解

| 注解 | 作用 |
|------|------|
| `@Attribute` | 标记类为注解类 |
| `@Target` | 限制注解使用位置（CLASS, PROPERTY, METHOD 等）|
| `@Retention` | 注解保留策略（SOURCE, COMPILE, RUNTIME）|
| `@Inherited` | 子类继承父类注解 |
| `@Repeatable` | 允许重复使用 |

### 输出

Sola 使用 `sola.io.Console` 类进行所有输出操作：

```sola
use sola.io.Console;

Console::writeLine("Hello, World!");  // 输出并换行
Console::write("Hello");              // 输出不换行
Console::debug("Debug info");         // [DEBUG] Debug info
Console::info("Information");         // [INFO] Information
Console::warn("Warning");             // [WARN] Warning
Console::error("Error message");      // [ERROR] Error message
```

> **注意**：Sola 不支持 `echo` 语句或 `print()` 函数，所有输出必须通过 `Console` 类。

### 内置函数

Sola 提供以下内置函数：

```sola
len($array)          // 获取数组长度
typeof($value)       // 获取类型名称
isset($array[$key])  // 检查键是否存在
unset($array[$key])  // 删除数组元素
```

> **输出说明**：Sola 不提供 `echo` 或 `print()` 函数，所有输出必须使用 `sola.io.Console` 类。

### 原生函数 (native_)

`native_` 开头的函数是 Sola 运行时提供的底层原生函数，**只能在标准库（`sola.*` 命名空间）中调用**，用户代码无法直接使用。

```sola
// 这些函数只能在标准库代码中使用：
native_str_index_of($str, $search)      // 字符串查找
native_str_substring($str, $start, $end) // 字符串截取
native_crypto_sha1_bytes($data)          // SHA1 哈希
native_reflect_get_class($obj)           // 反射获取类名
native_reflect_set_property($obj, $name, $val)  // 反射设置属性
// ... 等等
```

如果需要使用这些功能，应通过标准库提供的封装类来调用，如 `sola.lang.Str`、`sola.crypto.Hash` 等。

### 链式调用
```sola
$client := new HttpClient();
$response := $client
    ->setTimeout(30000)
    ->setFollowRedirects(true)
    ->setUserAgent("MyApp/1.0")
    ->get("https://api.example.com");
```

---

## 代码风格约定

### 命名规范
- **类名**：PascalCase（`UserController`, `HttpClient`）
- **接口名**：以 `I` 开头（`ICollection`, `IComparable`）
- **方法名**：camelCase（`getUserById`, `setName`）
- **变量名**：以 `$` 开头 + camelCase（`$userName`, `$itemCount`）
- **常量名**：UPPER_SNAKE_CASE（`MAX_SIZE`, `DEFAULT_TIMEOUT`）
- **命名空间**：小写点分隔（`sola.collections`, `company.project`）

### 文件组织
- 一个文件通常包含一个主要的类/接口
- 文件名与类名对应（`ArrayList.sola` 包含 `ArrayList` 类）
- 相关的辅助类可以放在同一文件中

---

## 快速参考

### 常用标准库
```sola
// 输入输出
use sola.io.{Console, File, Dir};

// 集合
use sola.collections.{ArrayList, HashMap, HashSet};

// 字符串处理
use sola.lang.Str;

// 时间处理
use sola.time.{DateTime, Duration};

// JSON 处理
use sola.json.Json;

// HTTP 客户端
use sola.net.http.HttpClient;

// 正则表达式
use sola.regex.Regex;

// 并发编程
use sola.concurrent.{Channel, WaitGroup};
```

### 完整示例

以下是一个完整的 Sola 程序示例，展示了面向对象的代码组织方式：

```sola
namespace app.services

use sola.collections.ArrayList;
use sola.json.Json;
use sola.net.http.HttpClient;
use sola.io.Console;
use sola.lang.Exception;

/**
 * 用户实体类
 */
public class User {
    private int $id;
    private string $name;
    private string $email;

    public function __construct(int $id, string $name, string $email) {
        $this->id = $id;
        $this->name = $name;
        $this->email = $email;
    }

    public function getName(): string {
        return $this->name;
    }

    public function getEmail(): string {
        return $this->email;
    }

    public function isActive(): bool {
        return true; // 示例逻辑
    }
}

/**
 * 用户服务类
 */
public class UserService {
    private HttpClient $client;
    private string $baseUrl;
    
    public function __construct(string $baseUrl) {
        $this->baseUrl = $baseUrl;
        $this->client = new HttpClient();
        $this->client->setTimeout(30000);
    }
    
    /**
     * 获取所有用户
     */
    public function getUsers(): ArrayList<User> {
        $response := $this->client->get($this->baseUrl + "/users");
        
        if ($response->status() != 200) {
            throw new Exception("Failed to fetch users");
        }
        
        // JSON 解码返回 SuperArray（与外部数据交互的合理场景）
        $data := Json::decode($response->body());
        $users := new ArrayList<User>();
        
        // 将动态数据转换为强类型对象
        foreach ($data as $item) {
            $user := new User(
                $item["id"] as int,
                $item["name"] as string,
                $item["email"] as string
            );
            $users->add($user);
        }
        
        return $users;
    }
    
    /**
     * 根据条件过滤用户
     */
    public function filterUsers(
        ArrayList<User> $users,
        function(User $u): bool $predicate
    ): ArrayList<User> {
        return $users->filter($predicate);
    }
}

/**
 * 应用程序主类
 */
public class Application {
    /**
     * 程序入口点（必须是 main 函数）
     */
    public static function main() {
        try {
            // 创建服务实例
            $service := new UserService("https://api.example.com");

            // 获取所有用户
            Console::info("正在获取用户列表...");
            $users := $service->getUsers();
            Console::info(#"获取到 {$users->size()} 个用户");

            // 过滤活跃用户
            $activeUsers := $service->filterUsers(
                $users,
                (User $u): bool => $u->isActive()
            );

            // 遍历输出
            Console::writeLine("活跃用户列表:");
            foreach ($activeUsers as $user) {
                Console::writeLine(#"  - {$user->getName()} ({$user->getEmail()})");
            }

            Console::info("处理完成!");

        } catch (Exception $e) {
            Console::error(#"发生错误: {$e->getMessage()}");
        }
    }
}

// 程序从 Application::main() 开始执行
```

---

## 关键字和保留字

### 关键字列表

以下是 Sola 语言的所有关键字，不能用作标识符（变量名、函数名、类名等）：

#### 类型关键字
| 关键字 | 说明 |
|--------|------|
| `int`, `i8`, `i16`, `i32`, `i64` | 有符号整数类型 |
| `uint`, `u8`, `u16`, `u32`, `u64` | 无符号整数类型 |
| `byte` | 与 `u8` 等价 |
| `float`, `f32`, `f64` | 浮点数类型 |
| `bool` | 布尔类型 |
| `string` | 字符串类型 |
| `void` | 无返回值（可省略） |
| `unknown` | 安全的顶类型 |
| `dynamic` | 动态类型 |
| `func` | 函数类型 |
| `map` | 映射类型 |

#### 值关键字
| 关键字 | 说明 |
|--------|------|
| `true` | 布尔真值 |
| `false` | 布尔假值 |
| `null` | 空值 |

#### 声明关键字
| 关键字 | 说明 |
|--------|------|
| `class` | 类声明 |
| `interface` | 接口声明 |
| `abstract` | 抽象类/方法 |
| `extends` | 继承 |
| `implements` | 实现接口 |
| `function` | 函数声明 |
| `const` | 常量 |
| `static` | 静态成员 |
| `final` | 不可继承/重写 |
| `enum` | 枚举声明 |
| `type` | 类型别名/新类型 |
| `namespace` | 命名空间 |
| `use` | 导入 |

#### 访问控制关键字
| 关键字 | 说明 |
|--------|------|
| `public` | 公开访问 |
| `protected` | 保护访问 |
| `private` | 私有访问 |

#### 控制流关键字
| 关键字 | 说明 |
|--------|------|
| `if`, `else`, `elseif` | 条件分支 |
| `switch`, `case`, `default` | 多路分支 |
| `match` | 模式匹配 |
| `for`, `foreach`, `while`, `do` | 循环 |
| `break`, `continue` | 循环控制 |
| `return` | 返回 |

#### 并发关键字
| 关键字 | 说明 |
|--------|------|
| `go` | 启动协程 |
| `select` | 多路选择 |

#### 异常处理关键字
| 关键字 | 说明 |
|--------|------|
| `try`, `catch`, `finally` | 异常处理 |
| `throw` | 抛出异常 |

#### 其他关键字
| 关键字 | 说明 |
|--------|------|
| `new` | 创建实例 |
| `$this` | 当前对象 |
| `self` | 当前类 |
| `parent` | 父类 |
| `as` | 类型转换 |
| `as?` | 安全类型转换 |
| `is` | 类型检查 |
| `where` | 泛型约束 |

#### 内置函数
| 函数 | 说明 |
|------|------|
| `len()` | 获取长度 |
| `typeof()` | 获取类型 |
| `isset()` | 检查是否存在 |
| `unset()` | 删除元素 |

### 上下文关键字

以下是**上下文关键字**，仅在特定上下文中有特殊含义，其他地方可作为普通标识符使用：

| 关键字 | 上下文 | 说明 |
|--------|--------|------|
| `get` | 属性访问器 | 定义 getter |
| `set` | 属性访问器 | 定义 setter |
| `value` | setter 体内 | 表示设置的值 |

```sola
// get、set、value 作为上下文关键字
public class User {
    private string $name;
    
    // 属性访问器中，get/set 是关键字
    public string $Name {
        get { return $this->name; }
        set { $this->name = value; }  // value 是隐式参数
    }
}

// 在其他地方，get/set/value 可以作为普通标识符
public class Pool {
    public function get(): Connection {  // get 作为方法名
        return new Connection();
    }
}

public class Data {
    public string $value = "";  // value 作为属性名
}
```

### 函数返回类型

函数返回类型可以：
1. **省略**：表示无返回值
2. **使用 `void`**：显式声明无返回值（与省略等价）
3. **指定类型**：如 `string`、`int`、`User` 等

```sola
// 以下三种写法等价：
public function doSomething() { }           // 省略返回类型
public function doSomething(): void { }     // 显式 void（推荐省略）

// 有返回值
public function getName(): string {
    return $this->name;
}

// 多返回值
public function getName(): (int, string) {
    return $this->name;
}
```

---

*本文档基于 Sola 语言源码分析生成，如有更新请以官方文档为准。*

