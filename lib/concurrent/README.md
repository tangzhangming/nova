# Sola 并发库

本包提供 Sola 语言的并发编程支持，采用现代 OOP 风格 API。

## 核心组件

### Coroutine\<T\> - 协程

协程是 Sola 的核心并发原语，提供类型安全的异步编程支持。

```sola
use sola.concurrent.Coroutine;

// 创建并启动协程
Coroutine<int> $task = Coroutine::spawn((): int => {
    return heavyComputation();
});

// 等待结果
int $result = $task->await();

// 带超时等待
try {
    $result := $task->await(5000);  // 5 秒超时
} catch (TimeoutException $e) {
    echo "操作超时";
    $task->cancel();
}

// 状态检查
if ($task->isCompleted) {
    echo "已完成";
}

// 语法糖：go 关键字（fire-and-forget）
go doSomethingAsync();  // 不关心返回值时使用
```

### 组合多个协程

```sola
// 并行执行，等待全部完成
$tasks := Coroutine<int>[
    Coroutine::spawn(task1),
    Coroutine::spawn(task2),
    Coroutine::spawn(task3),
];
int[] $results = Coroutine::all($tasks)->await();

// 竞速：取最快的结果
$fastest := Coroutine::race([
    Coroutine::spawn(fetchFromServer1),
    Coroutine::spawn(fetchFromServer2),
])->await();

// 任一成功即可
$result := Coroutine::any([
    Coroutine::spawn(tryMethod1),
    Coroutine::spawn(tryMethod2),
])->await();
```

### 链式操作

```sola
Coroutine::spawn(fetchUserId)
    ->then((int $id): User => fetchUser($id))
    ->then((User $user): string => $user->name)
    ->catch((Exception $e): string => "Unknown")
    ->finally((): void => echo "完成")
    ->await();
```

### Channel\<T\> - 通道

通道是协程间通信的主要机制，遵循 CSP 模型。

```sola
use sola.concurrent.Channel;

// 创建通道
Channel<int> $ch = new Channel<int>();       // 无缓冲
Channel<string> $buf = new Channel<string>(10);  // 有缓冲

// 发送和接收
$ch->send(42);
int $value = $ch->receive();

// 非阻塞操作
if ($ch->trySend(100)) {
    echo "发送成功";
}
$msg := $ch->tryReceive();

// 关闭通道
$ch->close();
```

### Select 多路复用（OOP 风格）

```sola
use sola.concurrent.{Channel, SelectCase};

Channel<int> $ch1 = new Channel<int>();
Channel<string> $ch2 = new Channel<string>();

// OOP 风格（推荐）
Channel::select([
    SelectCase::recv($ch1, (int $v): void => {
        echo "收到整数: " + $v;
    }),
    SelectCase::recv($ch2, (string $s): void => {
        echo "收到字符串: " + $s;
    }),
    SelectCase::timeout(5000, (): void => {
        echo "等待超时";
    }),
]);

// 旧语法仍然支持（但不推荐）
select {
    case $num := <-$ch1:
        echo "Received: " + $num;
    default:
        echo "No data";
}
```

### WaitGroup - 等待组

等待组用于等待一组协程完成。

```sola
use sola.concurrent.{Coroutine, WaitGroup};

$wg := new WaitGroup();

for ($i := 0; $i < 10; $i++) {
    $wg->add();
    Coroutine::spawn((): void use ($i, $wg) => {
        processTask($i);
        $wg->done();
    });
}

$wg->wait();
echo "All tasks done!";
```

## 异常类

- `ChannelException` - 通道异常基类
- `ChannelClosedException` - 通道已关闭异常
- `ChannelFullException` - 通道已满异常
- `ChannelEmptyException` - 通道为空异常
- `TimeoutException` - 超时异常

## 使用示例

### 生产者-消费者模式

```sola
use sola.concurrent.{Channel, Coroutine};

Channel<int> $jobs = new Channel<int>(100);
Channel<int> $results = new Channel<int>(100);

// 启动 3 个工作协程
for ($w := 0; $w < 3; $w++) {
    Coroutine::spawn((): void => {
        foreach ($jobs as $job) {
            $results->send($job * 2);
        }
    });
}

// 发送任务
for ($i := 0; $i < 10; $i++) {
    $jobs->send($i);
}
$jobs->close();

// 收集结果
for ($i := 0; $i < 10; $i++) {
    echo $results->receive();
}
```

### 并行 HTTP 请求

```sola
use sola.concurrent.Coroutine;
use sola.net.http.HttpClient;

function fetchAllUsers(int[] $userIds): User[] {
    $tasks := Coroutine<User>[];
    
    foreach ($userIds as $id) {
        $tasks[] = Coroutine::spawn((): User use ($id) => {
            $client := new HttpClient();
            $response := $client->get("/api/users/" + $id);
            return User::fromJson($response->body());
        });
    }
    
    return Coroutine::all($tasks)->await();
}

// 使用
$users := fetchAllUsers(int{1, 2, 3, 4, 5});
```

### 超时和降级

```sola
use sola.concurrent.Coroutine;
use sola.net.TimeoutException;

function fetchWithFallback(): Data {
    try {
        // 尝试主服务，3 秒超时
        return Coroutine::spawn(fetchFromPrimary)->await(3000);
    } catch (TimeoutException $e) {
        // 降级到备用服务
        return Coroutine::spawn(fetchFromBackup)->await();
    }
}
```

## API 总览

### Coroutine\<T\>

| 方法 | 说明 |
|------|------|
| `Coroutine::spawn(fn)` | 创建并启动协程 |
| `Coroutine::resolved(value)` | 创建已完成的协程 |
| `Coroutine::rejected(error)` | 创建已失败的协程 |
| `Coroutine::delay(ms)` | 延迟执行 |
| `Coroutine::all(tasks)` | 等待全部完成 |
| `Coroutine::any(tasks)` | 等待任一成功 |
| `Coroutine::race(tasks)` | 等待最快完成 |
| `await()` | 等待并获取结果 |
| `await(timeout)` | 带超时等待 |
| `cancel()` | 请求取消 |
| `then(fn)` | 链式转换 |
| `catch(fn)` | 错误处理 |
| `finally(fn)` | 最终回调 |

### Channel\<T\>

| 方法 | 说明 |
|------|------|
| `new Channel<T>(capacity)` | 创建通道 |
| `send(value)` | 发送（阻塞） |
| `receive()` | 接收（阻塞） |
| `trySend(value)` | 非阻塞发送 |
| `tryReceive()` | 非阻塞接收 |
| `close()` | 关闭通道 |
| `Channel::select(cases)` | 多路复用 |
| `Channel::merge(channels)` | 合并通道 |
