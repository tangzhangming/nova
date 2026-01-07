# 数据库客户端标准库

Sola 标准库提供的数据库客户端，支持 MySQL 和 PostgreSQL，包含连接池管理、查询、事务等功能。

## 支持的数据库

- **MySQL**: 完整支持，包括连接池、查询、事务
- **PostgreSQL**: 完整支持，包括连接池、查询、事务

## 架构设计

### 连接池

所有数据库客户端都基于统一的连接池架构：

```
ConnectionPool (抽象类)
├── MysqlPool (MySQL 实现)
└── PgPool (PostgreSQL 实现)
```

连接池的核心功能：
- 连接获取和归还
- 连接池大小管理
- 连接健康检查
- 连接过期清理
- 统计信息

### 客户端 API

提供两层 API：

1. **简化客户端**（推荐）：`MysqlClient` / `PgClient`
   - 自动管理连接池
   - 简洁的 API
   - 适合大多数场景

2. **连接池**（高级）：`MysqlPool` / `PgPool`
   - 手动管理连接
   - 更细粒度的控制
   - 适合特殊场景

## 快速开始

### MySQL

```sola
use sola.database.mysql.MysqlClient;

$client := new MysqlClient("127.0.0.1", 3306, "root", "password", "mydb");

// 查询
$result := $client->query("SELECT * FROM users WHERE id = ?", ["1"]);
$row := $result->first();

// 执行
$client->execute("INSERT INTO users (name) VALUES (?)", ["Alice"]);

$client->close();
```

### PostgreSQL

```sola
use sola.database.postgresql.PgClient;

$client := new PgClient("127.0.0.1", 5432, "root", "password", "mydb");

// 查询（注意：PostgreSQL 使用 $1, $2, ... 占位符）
$result := $client->query("SELECT * FROM users WHERE id = $1", ["1"]);
$row := $result->first();

// 执行
$client->execute("INSERT INTO users (name) VALUES ($1)", ["Alice"]);

$client->close();
```

## 连接池配置

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

## 事务

```sola
// MySQL
$client->transaction(function($conn) {
    $conn->execute("INSERT INTO users (name) VALUES ('Alice')");
    $conn->execute("INSERT INTO users (name) VALUES ('Bob')");
    return true;
});

// PostgreSQL
$client->transaction(function($conn) {
    $conn->execute("INSERT INTO users (name) VALUES ('Alice')");
    $conn->execute("INSERT INTO users (name) VALUES ('Bob')");
    return true;
});
```

## 连接池统计

```sola
$stats := $client->stats();
echo "Total connections: " + $stats->total + "\n";
echo "Active connections: " + $stats->active + "\n";
echo "Idle connections: " + $stats->idle + "\n";
echo "Pool usage: " + $stats->usage() + "\n";
```

## 目录结构

```
lib/database/
├── ConnectionPool.sola      # 通用连接池抽象类
├── PoolConfig.sola          # 连接池配置
├── PoolStats.sola           # 连接池统计
├── DatabaseException.sola   # 数据库异常
├── mysql/
│   ├── MysqlConnection.sola # MySQL 连接
│   ├── MysqlPool.sola       # MySQL 连接池
│   ├── MysqlClient.sola     # MySQL 客户端
│   └── README.md            # MySQL 文档
└── postgresql/
    ├── PgConnection.sola    # PostgreSQL 连接
    ├── PgPool.sola          # PostgreSQL 连接池
    ├── PgClient.sola        # PostgreSQL 客户端
    └── README.md            # PostgreSQL 文档
```

## 设计原则

1. **严格类型**：所有 API 都使用严格类型，不使用万能数组
2. **连接池共享**：MySQL 和 PostgreSQL 共享连接池核心逻辑
3. **简洁 API**：提供简洁易用的客户端 API
4. **灵活控制**：支持直接使用连接池进行细粒度控制

## 更多文档

- [MySQL 客户端文档](mysql/README.md)
- [PostgreSQL 客户端文档](postgresql/README.md)




