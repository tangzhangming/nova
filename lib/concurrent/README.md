# Sola 并发库

本包提供 Sola 语言的并发编程支持，包括协程通信和同步原语。

## 核心组件

### Channel\<T\>

通道是协程间通信的主要机制，遵循 CSP（Communicating Sequential Processes）模型。

```sola
use sola.concurrent.Channel;

// 创建无缓冲通道
$ch := new Channel<int>();

// 创建有缓冲通道
$buffered := new Channel<string>(10);

// 发送数据
$ch->send(42);

// 接收数据
$value := $ch->receive();

// 非阻塞操作
if ($ch->trySend(100)) {
    echo "Sent successfully";
}

$msg := $ch->tryReceive();
if ($msg != null) {
    echo "Received: " + $msg;
}

// 关闭通道
$ch->close();
```

### WaitGroup

等待组用于等待一组协程完成。

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
echo "All tasks done!";
```

## 异常类

- `ChannelException` - 通道异常基类
- `ChannelClosedException` - 通道已关闭异常
- `ChannelFullException` - 通道已满异常
- `ChannelEmptyException` - 通道为空异常

## 使用示例

### 生产者-消费者模式

```sola
use sola.concurrent.Channel;

$jobs := new Channel<int>(100);
$results := new Channel<int>(100);

// 启动 3 个工作协程
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
    echo $results->receive();
}
```

### Select 多路复用

```sola
use sola.concurrent.Channel;

$ch1 := new Channel<int>();
$ch2 := new Channel<string>();

select {
    case $num := $ch1->receive():
        echo "Received number: " + $num;
        
    case $msg := $ch2->receive():
        echo "Received message: " + $msg;
        
    default:
        echo "No data available";
}
```
