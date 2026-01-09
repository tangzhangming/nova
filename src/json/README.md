# Sola JSON 库

JSON 库提供了完整的 JSON 编码/解码功能，支持 SuperArray、对象序列化、注解控制等特性。

## 快速开始

```sola
use sola.json.Json;

// 编码
$data := ["name" => "Alice", "age" => 25];
echo Json::encode($data);  // {"age":25,"name":"Alice"}

// 解码
$result := Json::decode('{"name":"Bob","age":30}');
echo $result["name"];  // Bob

// 美化输出
echo Json::encodePretty($data);
```

## 核心 API

### Json 类

#### encode(value) : string

将任意值编码为 JSON 字符串。

```sola
use sola.json.Json;

// 基本类型
echo Json::encode("hello");   // "hello"
echo Json::encode(42);        // 42
echo Json::encode(3.14);      // 3.14
echo Json::encode(true);      // true
echo Json::encode(null);      // null

// 数组（纯整数键 -> JSON数组）
$arr := [1, 2, 3];
echo Json::encode($arr);  // [1,2,3]

// 对象（字符串键 -> JSON对象）
$obj := ["name" => "Alice", "age" => 25];
echo Json::encode($obj);  // {"age":25,"name":"Alice"}
```

#### encodePretty(value, indent = "  ") : string

将值编码为格式化的 JSON 字符串。

```sola
$data := [
    "user" => ["name" => "Alice", "age" => 25],
    "active" => true
];

echo Json::encodePretty($data);
// {
//   "active": true,
//   "user": {
//     "age": 25,
//     "name": "Alice"
//   }
// }

// 自定义缩进
echo Json::encodePretty($data, "    ");  // 4空格缩进
echo Json::encodePretty($data, "\t");    // Tab缩进
```

#### decode(json) : mixed

将 JSON 字符串解码为 Sola 值。

```sola
// 解码对象
$data := Json::decode('{"name":"Alice","age":25}');
echo $data["name"];  // Alice
echo $data["age"];   // 25

// 解码数组
$arr := Json::decode('[1, 2, 3]');
echo $arr[0];  // 1

// 解码基本类型
echo Json::decode('"hello"');  // hello
echo Json::decode('42');       // 42
echo Json::decode('true');     // true
echo Json::decode('null');     // null
```

#### isValid(json) : bool

检查字符串是否为有效的 JSON。

```sola
echo Json::isValid('{"name":"test"}');  // true
echo Json::isValid('{invalid}');        // false
echo Json::isValid('[]');               // true
echo Json::isValid('');                 // false
```

#### get(json, key) : mixed

从 JSON 字符串中获取指定键的值。

```sola
$json := '{"name":"Alice","age":25}';
echo Json::get($json, "name");  // Alice
echo Json::get($json, "age");   // 25
```

## 类型映射

### 编码映射（Sola → JSON）

| Sola 类型 | JSON 输出 | 说明 |
|-----------|-----------|------|
| `int` | 数字 | `42` |
| `float` | 数字 | `3.14` |
| `string` | 字符串 | `"hello"` |
| `bool` | 布尔 | `true` / `false` |
| `null` | null | `null` |
| `SuperArray` (纯整数键) | 数组 | `[1, 2, 3]` |
| `SuperArray` (含字符串键) | 对象 | `{"key": "value"}` |
| `Object` | 对象 | 公开属性序列化 |

### 解码映射（JSON → Sola）

| JSON 类型 | Sola 结果 |
|-----------|-----------|
| 对象 `{...}` | `SuperArray` (字符串键) |
| 数组 `[...]` | `SuperArray` (整数键) |
| 字符串 | `string` |
| 整数 | `int` |
| 小数 | `float` |
| true/false | `bool` |
| null | `null` |

## 对象序列化

### 基本用法

类的公开属性会自动序列化：

```sola
use sola.json.Json;

class User {
    public string $name;
    public int $age;
    public bool $active;
    
    public function __construct(string $name, int $age) {
        $this->name = $name;
        $this->age = $age;
        $this->active = true;
    }
}

$user := new User("Alice", 25);
echo Json::encode($user);
// {"active":true,"age":25,"name":"Alice"}
```

### 使用注解控制序列化

#### @JsonProperty("name")

指定 JSON 字段名：

```sola
class User {
    @JsonProperty("user_name")
    public string $name;
    
    @JsonProperty("user_age")
    public int $age;
}

$user := new User();
$user->name = "Alice";
$user->age = 25;

echo Json::encode($user);
// {"user_age":25,"user_name":"Alice"}
```

#### @JsonIgnore

忽略字段，不参与序列化：

```sola
class User {
    public string $name;
    
    @JsonIgnore
    public string $password;
}

$user := new User();
$user->name = "Alice";
$user->password = "secret123";

echo Json::encode($user);
// {"name":"Alice"}
```

#### @JsonOmitEmpty

当值为空时不输出该字段：

```sola
class User {
    public string $name;
    
    @JsonOmitEmpty
    public string $nickname;
    
    @JsonOmitEmpty
    public int $score;
}

$user := new User();
$user->name = "Alice";
$user->nickname = "";  // 空字符串
$user->score = 0;       // 零值

echo Json::encode($user);
// {"name":"Alice"}
// nickname 和 score 因为是空值而被省略
```

空值判断规则：
- `null` → 空
- `""` (空字符串) → 空
- `0` (整数零) → 空
- `0.0` (浮点零) → 空
- `[]` (空数组) → 空

#### @JsonString

将数字类型作为字符串输出：

```sola
class Order {
    @JsonString
    public int $orderId;
    
    @JsonString
    public float $amount;
}

$order := new Order();
$order->orderId = 123456789;
$order->amount = 99.99;

echo Json::encode($order);
// {"amount":"99.99","orderId":"123456789"}
```

#### 组合使用注解

```sola
class ApiUser {
    @JsonProperty("user_id")
    @JsonString
    public int $id;
    
    @JsonProperty("user_name")
    public string $name;
    
    @JsonOmitEmpty
    public string $email;
    
    @JsonIgnore
    public string $passwordHash;
}
```

## 命名策略

### JsonNaming 类

提供字段名转换功能：

```sola
use sola.json.JsonNaming;

// 转换为 snake_case
echo JsonNaming::toSnakeCase("userName");     // user_name
echo JsonNaming::toSnakeCase("HTTPServer");   // h_t_t_p_server

// 转换为 camelCase
echo JsonNaming::toCamelCase("user_name");    // userName

// 转换为 PascalCase
echo JsonNaming::toPascalCase("user_name");   // UserName

// 转换为 kebab-case
echo JsonNaming::toKebabCase("userName");     // user-name
```

### 命名策略常量

```sola
JsonNaming::NONE        // 不转换
JsonNaming::SNAKE_CASE  // userName → user_name
JsonNaming::CAMEL_CASE  // user_name → userName
JsonNaming::PASCAL_CASE // user_name → UserName
JsonNaming::KEBAB_CASE  // userName → user-name
```

## 高级用法

### JsonOptions 选项类

```sola
use sola.json.{Json, JsonOptions, JsonNaming};

$options := new JsonOptions();
$options->prettyPrint = true;
$options->indent = "    ";
$options->namingStrategy = JsonNaming::SNAKE_CASE;

// 或使用链式调用
$options := (new JsonOptions())
    ->withPretty()
    ->withIndent("    ")
    ->withNaming(JsonNaming::SNAKE_CASE);
```

### 预设配置

```sola
// 控制台/日志输出配置
$options := JsonOptions::forConsole();

// API响应配置（紧凑 + snake_case）
$options := JsonOptions::forApi();

// 存储配置（紧凑 + 转义Unicode）
$options := JsonOptions::forStorage();
```

## 实际应用示例

### API 响应处理

```sola
use sola.json.Json;

// 构建 API 响应
$response := [
    "code" => 0,
    "message" => "success",
    "data" => [
        "users" => [
            ["id" => 1, "name" => "Alice", "role" => "admin"],
            ["id" => 2, "name" => "Bob", "role" => "user"]
        ],
        "pagination" => [
            "page" => 1,
            "pageSize" => 10,
            "total" => 2
        ]
    ]
];

echo Json::encodePretty($response);
```

### 配置文件读取

```sola
use sola.json.Json;
use sola.io.File;

// 读取配置
$configJson := File::read("config.json");
$config := Json::decode($configJson);

// 使用配置
$dbHost := $config["database"]["host"];
$dbPort := $config["database"]["port"];
```

### 数据验证

```sola
use sola.json.Json;

function parseUserInput(string $input): mixed {
    if (!Json::isValid($input)) {
        throw new JsonException("Invalid JSON input");
    }
    
    $data := Json::decode($input);
    
    // 验证必填字段
    if (!$data.hasKey("name")) {
        throw new ArgumentException("Missing required field: name");
    }
    
    return $data;
}
```

### 往返测试

```sola
use sola.json.Json;

$original := ["x" => 1, "y" => 2, "z" => 3];
$encoded := Json::encode($original);
$decoded := Json::decode($encoded);
$reencoded := Json::encode($decoded);

// 验证一致性
if ($encoded == $reencoded) {
    echo "Round-trip successful!";
}
```

## 错误处理

```sola
use sola.json.{Json, JsonException};

try {
    $data := Json::decode($invalidJson);
} catch (JsonException $e) {
    echo "JSON 解析错误: " + $e->getMessage();
}
```

## 与 Go JSON 库对照

| Go 功能 | Sola 实现 |
|--------|----------|
| `json:"name"` | `@JsonProperty("name")` |
| `json:"-"` | `@JsonIgnore` |
| `json:",omitempty"` | `@JsonOmitEmpty` |
| `json:",string"` | `@JsonString` |
| `json.Marshal()` | `Json::encode()` |
| `json.Unmarshal()` | `Json::decode()` |
| `json.MarshalIndent()` | `Json::encodePretty()` |
| `json.Valid()` | `Json::isValid()` |

## 注意事项

1. **SuperArray 输出格式**：纯整数键（0,1,2...）输出为 JSON 数组，否则输出为 JSON 对象
2. **字段顺序**：JSON 对象的字段按字母顺序排列
3. **特殊字符**：字符串中的引号、换行符等会自动转义
4. **Unicode**：默认不转义 Unicode 字符，中文等直接输出
5. **null 处理**：Sola 的 null 值会输出为 JSON 的 null







