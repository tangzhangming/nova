# MySQL 数据库客户端

Sola 标准库提供的 MySQL 数据库客户端，支持连接池、查询、事务等功能。

## 快速开始

```sola
use sola.database.mysql.MysqlClient;

// 创建客户端
$client := new MysqlClient("127.0.0.1", 3306, "root", "password", "mydb");

// 查询
$result := $client->query("SELECT * FROM users WHERE id = ?", ["1"]);
$row := $result->first();
if ($row != null) {
    echo $row->get("name");
}

// 执行
$affected := $client->execute("INSERT INTO users (name) VALUES (?)", ["Alice"]);

// 关闭
$client->close();
```

## API 文档

### MysqlClient

#### 构造函数

```sola
public function __construct(
    string $host,
    int $port,
    string $user,
    string $password,
    string $database,
    PoolConfig|null $config = null
)
```

创建 MySQL 客户端实例。

**参数：**
- `$host`: 数据库主机地址
- `$port`: 数据库端口（默认 3306）
- `$user`: 用户名
- `$password`: 密码
- `$database`: 数据库名
- `$config`: 连接池配置（可选）

#### query()

执行查询 SQL，返回结果集。

```sola
public function query(string $sql, string[] $params = []): MysqlResult
```

**参数：**
- `$sql`: SQL 语句，支持 `?` 占位符
- `$params`: 参数数组

**返回：** `MysqlResult` 查询结果

**示例：**
```sola
$result := $client->query("SELECT * FROM users WHERE age > ?", ["18"]);
$rows := $result->all();
```

#### execute()

执行 SQL（INSERT/UPDATE/DELETE），返回受影响的行数。

```sola
public function execute(string $sql, string[] $params = []): int
```

**参数：**
- `$sql`: SQL 语句，支持 `?` 占位符
- `$params`: 参数数组

**返回：** 受影响的行数

**示例：**
```sola
$affected := $client->execute("UPDATE users SET name = ? WHERE id = ?", ["Alice", "1"]);
```

#### transaction()

执行事务。

```sola
public function transaction(callable $callback): mixed
```

**参数：**
- `$callback`: 事务回调函数，接收 `MysqlConnection` 参数

**返回：** 回调函数的返回值

**示例：**
```sola
$client->transaction(function($conn) {
    $conn->execute("INSERT INTO users (name) VALUES ('Alice')");
    $conn->execute("INSERT INTO users (name) VALUES ('Bob')");
    return true;
});
```

### MysqlResult

查询结果对象。

#### all()

获取所有行。

```sola
public function all(): MysqlRow[]
```

#### first()

获取第一行。

```sola
public function first(): MysqlRow|null
```

#### next()

移动到下一行。

```sola
public function next(): bool
```

#### row()

获取当前行。

```sola
public function row(): MysqlRow
```

### MysqlRow

行数据对象。

#### get()

获取指定列的值。

```sola
public function get(string $columnName): string|null
```

#### toMap()

转为映射（列名 => 值）。

```sola
public function toMap(): map[string]string
```

## 连接池配置

使用 `PoolConfig` 配置连接池：

```sola
use sola.database.PoolConfig;

$config := new PoolConfig();
$config->maxSize = 20;        // 最大连接数
$config->minSize = 5;          // 最小连接数
$config->maxIdleTime = 300;    // 最大空闲时间（秒）
$config->maxLifetime = 3600;    // 最大生存时间（秒）
$config->acquireTimeout = 30;  // 获取连接超时（秒）

$client := new MysqlClient("127.0.0.1", 3306, "root", "password", "mydb", $config);
```

## 高级用法

### 直接使用连接池

如果需要更细粒度的控制，可以直接使用连接池：

```sola
use sola.database.mysql.{MysqlPool, PoolConfig};

$config := new PoolConfig();
$pool := new MysqlPool($config, "127.0.0.1", 3306, "root", "password", "mydb");

$conn := $pool->get();
try {
    $result := $conn->query("SELECT * FROM users");
    // 使用连接
} finally {
    $pool->put($conn);
}

$pool->close();
```

### 连接池统计

```sola
$stats := $client->stats();
echo "Total: " + $stats->total + "\n";
echo "Active: " + $stats->active + "\n";
echo "Idle: " + $stats->idle + "\n";
echo "Usage: " + $stats->usage() + "\n";
```




