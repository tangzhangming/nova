# Nova 语言设计文档

Nova 是一门编译型静态类型语言，语法接近 PHP，运行于自定义虚拟机。

## 类
```
// 类可见性设计与C#相同
// 如果要公开导出，手动设置public
// 类名必须和文件名一致才可以导出，每个文件只能有一个与文件名相同的类
class Parents {

	// 类常量
	// 可见性 const 类型 常量名 = 值
	public const int MAX_VALUE = 100;

	// 静态属性
	public static int $count = 0;

	// 成员变量
	// 成员变量与变量都用$符号开头 与php一样
	public string $name;

	// 定义成员变量且初始化值
	public string $title = "this title";

    // 可空类型初始化为 null
    // 类型前面加?表示可空类型 与php一样
    public ?string $description = null;

	// 支持构造函数
    public function __construct() {

    }

	// 成员函数
	// 参数可以有默认值
	// 支持可变参数
	// 访问控制，可选public、protected、private
	// function关键字不可省略 与java不同
	public function getValue(string $name, int $var2=10000, int ...$args): int
	{
		// 调用类常量
		// 可以通过 self 或类名调用
		self::MAX_VALUE
		Parents::MAX_VALUE

		// 调用静态属性
		// 可以通过 self 或类名调用
		self::$count

		// 调用静态方法
		// 可以通过 self 或类名调用
		self::jingtai()

		// 调用属性
		// $this 指向当前对象
		$this->title

		// 调用方法
		$this->test();
	}

	// 成员函数支持重载
	public function getValue(string $name, string $test): int
	{
		return $var1, $var2;
	}

	/**
	 * 类方法支持多返回值
	 * 支持多行注释
	 */
	protected function test(string $data, string $name): (int, string)
	{
		string $title = "test";
		return 999, $title;
	}

	// 可以定义静态方法
	private static function jingtai(){

	}

	// 支持注解 和java一样用@符号
	@Service
	public function zhujie(){

	}


	// 析构函数
	// 不能有参数和返回值
    public function __destruct() {

    }
}

// 禁止类外部写执行代码
// 接口、抽象类、enum除外
```


## 继承
```
// 允许多层继承 不能多重继承
public class Sonclass extends Parents{
	// 静态属性 可以覆盖父类
	public static int $count = 0;

	protected function test(string $data, string $name): (int, string)
	{
		// parent::属性/方法/常量
		// parent是用于访问父类（基类）成员的特殊关键字
		return 999, parent::$count;
	}
}
```

## 接口
```
interface UserRepositoryInterface {
    // 参数和返回类型声明
    public function findById(int $id): User;

    public function findByEmail(string $email): User;

    public function save(User $user): bool;

    public function delete(int $id): bool;

    // 可空类型
    public function findByName(?string $name): ?array;
}
// 实现接口
class Template implements UserRepositoryInterface
{
	...
}
```


## 抽象类
```
// 抽象类
abstract class Shape {
    // 抽象方法 - 必须被子类实现
    abstract public function area(): float;

    // 抽象方法 - 带参数
    abstract public function scale(float $factor): void;

    // 具体方法 - 有实现
    public function describe(): string {
        return "我是一个形状，面积: " + $this->area();
    }
}

// 具体子类
class Circle extends Shape {

    // 实现抽象方法
    public function area(): float {
        return ...;
    }

    public function scale(float $factor) {
    	// 空return可以终止代码
    	return;
    }
}
```

## ENUM
```
语法接近php 按静态语言实现
```


## 类的使用
```
$obj := new User();
$obj->method();

// 调用静态方法、类常量、静态属性
User::abort()
User::PI
User::$languages

// 特殊用法 返回类的完整名称包括包名
$obj::class
```


## 变量
```
int $count = 999;       // 显式类型用 =
string $title;

$name := "abcd";        // 类型推导用 :=

// 多返回值接收
$a, $b := test();       // 声明新变量
$a, $b = test();        // 赋值给已存在的变量
$a, $newVar := test();  // 混合（至少一个新变量时可用 :=）
```

## 字符串
```
$name := "test";

// 字符串拼接 不使用PHP的.符号
$name + "abcd"

// 字符串插值 #号标识
#"my name is {$name}"
```

## 数组
```
// 数组 [类型:长度] 变量名 = 初始值;
string[100] $names = ["bob", "liu", "zhao"];

// 切片 不固定长度
string[] $names = ["bob", "liu", "zhao"];

// 自动推导数组 取第一个值的类型 后面类型不同就语法检查失败
$ids := [111, 222, 333, 444]

// 循环数组
foreach($names as $k => $v){
	$names[$k]
}

// 取值
$names[1];

// 赋值
$names[0] = "xxxx";

// 检查索引是否存在
if ($names.has(5)) {
	$names[5]...
}

// 获取数组长度
$ids.length
```


## map
```
map[string]int $names = [
	'china' => 100,
	'jp' => 99,
	'us' => 100,
]

// 取值
$names["china"];

// 赋值
$names["jp"] = 55;

// 检查键是否存在
if ($names.has("china")) {
	$names["china"]...
}

// 获取长度
$names.length
```

## 控制结构
```
	// if
	if(condition){
		// 条件为真时执行
	}

	if(condition){

	}else{

	}

	if ($t<"10") {
	    echo "Have a good morning!";
	} elseif ($t<"20") {
	    echo "Have a good day!";
	} else {
	    echo "Have a good night!";
	}

	// switch (自动break)
	switch (expression) {
	    case value1:
	        // 代码块1

	    case value2:
	        // 代码块2

	    // 更多的 case 语句
	    default:
	        // 如果没有匹配的值
	}

	// while
	while (条件)
	{
	    要执行的代码;
	}


	// for
	for (初始值; 条件; 增量)
	{
	    要执行的代码;
	}

	// 可以循环数组和map
	foreach ($array as $value)
	{
	    要执行代码;
	}
	foreach ($array as $key => $value)
	{
	    要执行代码;
	}
```


## 类型
```
| 关键字 | 用途 | 字节数 |
|--------|------|--------|
| `int` | 有符号整型 | 8 |
| `i8` | 8位有符号整型 | 1 |
| `i16` | 16位有符号整型 | 2 |
| `i32` | 32位有符号整型 | 4 |
| `i64` | 64位有符号整型 | 8 |
| `uint` | 无符号整型 | 8 |
| `u8` | 8位无符号整型 | 1 |
| `u16` | 16位无符号整型 | 2 |
| `u32` | 32位无符号整型 | 4 |
| `u64` | 64位无符号整型 | 8 |
| `float` | 浮点数 | 8 |
| `f32` | 32位浮点数 | 4 |
| `f64` | 64位浮点数 | 8 |
| `bool` | 布尔类型 | 1 |
| `string` | 字符串类型 |  |
| `object` | 对象 |  |
| `func` | 闭包类型 |  |
```


## 运算符
算术、比较、逻辑运算符、三元运算符



## 闭包与箭头函数
```
// 匿名闭包
$bibao = function(int $i): int {
	// 不能使用外部变量
	return $i * 3;
};
$bibao(10);

$x = 5;
$bibao2 = function(int $i) use ($x): int {
	// 外部变量可以use进来使用
	return $i * 3 * $x;
};
$bibao2(10);


// 箭头函数（自动捕获外部变量）
$y := 1;
$fn = (int $x): int => $x + $y;
$fn(100);
```


## 错误处理
```
try {
	100/0;

} catch (Exception $e) {
	$e->getMessage();

} finally {

}
```




## 全局工具函数
```
print($a)    // 向控制台打印信息

print_r($a)  // 打印类型和可读信息
```


## 入口类
```
// 与C#类似
print("Welcome!");
$app := new Application();
$app->run();

// 编译为：
// class Program {
//     public static function main() {
//         print("Welcome!");
//         $app := new Application();
//         $app->run();
//     }
// }
```

## 包管理
```
namespace company.project
// 声明namespace时可以像golang的包一样只有当前目录名

use overwatch.test.Test as Bieming;
use tencent.sdk.redis.Client;
use company.project.models.UserModel;

// 命名空间逻辑
// nova开头的，全是标准库，在lib目录下找

// 在项目根目录下定义project.toml，里面存在当前项目的命名空间，其原理与golang的go.mod相同
// 从入口文件一直向上找到project.toml
```


## 标准库
tcp
io
time
http
regex
redis、mysql
math
byte操作
email
TCP/udp/socket/web socket
