# Sola TCP 网络编程指南

本文档介绍 Sola 语言的 TCP 网络编程功能，包括客户端和服务端的使用方法。

## 目录

- [概述](#概述)
- [TCP 客户端](#tcp-客户端)
  - [基本使用](#基本使用)
  - [连接配置](#连接配置)
  - [数据读写](#数据读写)
  - [超时设置](#超时设置)
  - [Socket 选项](#socket-选项)
- [TCP 服务端](#tcp-服务端)
  - [启动服务器](#启动服务器)
  - [接受连接](#接受连接)
  - [处理客户端](#处理客户端)
- [TLS/SSL 安全连接](#tlsssl-安全连接)
  - [TLS 客户端](#tls-客户端)
  - [TLS 服务端](#tls-服务端)
- [异常处理](#异常处理)
- [完整示例](#完整示例)

---

## 概述

Sola 提供了完整的 TCP 网络编程支持，主要包括以下类：

| 类名 | 命名空间 | 说明 |
|------|----------|------|
| `TcpClient` | `sola.net.tcp` | TCP 客户端，用于连接远程服务器 |
| `TcpServer` | `sola.net.tcp` | TCP 服务端，用于监听和接受连接 |
| `TcpConnection` | `sola.net.tcp` | TCP 连接，用于服务端处理客户端 |
| `TlsClient` | `sola.net.tcp` | TLS/SSL 安全客户端 |
| `TlsServer` | `sola.net.tcp` | TLS/SSL 安全服务端 |

异常类：

| 类名 | 命名空间 | 说明 |
|------|----------|------|
| `SocketException` | `sola.net` | 网络异常基类 |
| `ConnectionException` | `sola.net` | 连接异常 |
| `TimeoutException` | `sola.net` | 超时异常 |

---

## TCP 客户端

### 基本使用

```sola
use sola.net.tcp.TcpClient;

// 方式1：构造时直接连接
$client := new TcpClient("example.com", 80);

// 方式2：先创建后连接
$client := new TcpClient();
$client->connect("example.com", 80);

// 方式3：使用静态工厂方法
$client := TcpClient::create("example.com", 80);

// 检查连接状态
if ($client->isConnected()) {
    print("已连接到服务器");
}

// 使用完毕后关闭连接
$client->close();
```

### 连接配置

```sola
use sola.net.tcp.TcpClient;

$client := new TcpClient();

// 设置连接超时（毫秒），必须在 connect() 之前设置
$client->setConnectTimeout(5000);  // 5秒

// 使用带超时的工厂方法
$client := TcpClient::createWithTimeout("example.com", 80, 3000);
```

### 数据读写

```sola
use sola.net.tcp.TcpClient;
use sola.lang.Bytes;

$client := new TcpClient("example.com", 80);

// ===== 写入数据 =====

// 写入字符串
$bytesSent := $client->write("GET / HTTP/1.1\r\n");

// 写入一行（自动添加换行符）
$client->writeLine("Host: example.com");

// 写入字节数组
$data := Bytes::fromString("Hello");
$client->writeBytes($data);

// ===== 读取数据 =====

// 读取指定长度的字符串
$response := $client->read(1024);

// 读取一行
$line := $client->readLine();

// 读取直到遇到分隔符
$data := $client->readUntil("\r\n");

// 精确读取指定字节数
$exactData := $client->readExact(100);

// 读取字节数组
$bytes := $client->readBytes(1024);

// 查看缓冲区可读字节数
$available := $client->available();

$client->close();
```

### 超时设置

```sola
use sola.net.tcp.TcpClient;

$client := new TcpClient("example.com", 80);

// 设置读取超时（毫秒）
$client->setReadTimeout(10000);  // 10秒

// 设置写入超时（毫秒）
$client->setWriteTimeout(5000);  // 5秒

// 设置通用超时（秒）- 同时应用于读写
$client->setTimeout(30);  // 30秒

// 清除所有超时设置
$client->clearTimeout();

// 链式调用
$client->setReadTimeout(10000)
       ->setWriteTimeout(5000)
       ->setNoDelay(true);

$client->close();
```

### Socket 选项

```sola
use sola.net.tcp.TcpClient;

$client := new TcpClient("example.com", 80);

// KeepAlive - 保持连接存活
$client->setKeepAlive(true);           // 启用
$client->setKeepAlive(true, 30);       // 启用，30秒间隔

// NoDelay - 禁用 Nagle 算法，减少延迟
$client->setNoDelay(true);

// 缓冲区大小
$client->setSendBufferSize(65536);     // 发送缓冲区 64KB
$client->setReceiveBufferSize(65536);  // 接收缓冲区 64KB

// Linger - 关闭时的行为
$client->setLinger(5);   // 等待5秒发送剩余数据
$client->setLinger(-1);  // 禁用 Linger

$client->close();
```

### 地址信息

```sola
use sola.net.tcp.TcpClient;

$client := new TcpClient("example.com", 80);

// 远程地址（服务器）
$remoteHost := $client->getRemoteHost();    // "93.184.216.34"
$remotePort := $client->getRemotePort();    // 80
$remoteAddr := $client->getRemoteAddress(); // "93.184.216.34:80"

// 本地地址（客户端）
$localHost := $client->getLocalHost();      // "192.168.1.100"
$localPort := $client->getLocalPort();      // 54321
$localAddr := $client->getLocalAddress();   // "192.168.1.100:54321"

$client->close();
```

---

## TCP 服务端

### 启动服务器

```sola
use sola.net.tcp.TcpServer;

// 方式1：构造时直接启动
$server := new TcpServer("0.0.0.0", 8080);

// 方式2：先创建后启动
$server := new TcpServer();
$server->start("0.0.0.0", 8080);

// 方式3：使用静态工厂方法
$server := TcpServer::create(8080);              // 监听所有接口
$server := TcpServer::createLocal(8080);         // 只监听本地

// 检查服务器状态
if ($server->isListening()) {
    print("服务器正在监听: " + $server->getAddress());
}

// 获取服务器信息
$host := $server->getHost();     // "0.0.0.0"
$port := $server->getPort();     // 8080
$addr := $server->getAddress();  // "0.0.0.0:8080"

// 停止服务器
$server->stop();
```

### 接受连接

```sola
use sola.net.tcp.TcpServer;
use sola.net.tcp.TcpConnection;

$server := TcpServer::create(8080);

// 阻塞等待新连接
$conn := $server->accept();

// 带超时的等待（毫秒）
$conn := $server->acceptTimeout(5000);  // 5秒超时

// 设置默认接受超时
$server->setAcceptTimeout(10000);  // 10秒
$conn := $server->accept();  // 使用默认超时

if ($conn != null) {
    print("新连接来自: " + $conn->getRemoteAddress());
    // 处理连接...
    $conn->close();
}

$server->stop();
```

### 处理客户端

```sola
use sola.net.tcp.TcpServer;
use sola.net.tcp.TcpConnection;

$server := TcpServer::create(8080);

print("Echo 服务器已启动，监听端口 8080");

while ($server->isListening()) {
    $conn := $server->accept();
    
    if ($conn != null) {
        print("客户端连接: " + $conn->getRemoteAddress());
        
        // 设置连接选项
        $conn->setReadTimeout(30000);
        $conn->setNoDelay(true);
        
        // 读取数据
        $data := $conn->readLine();
        
        if ($data != "") {
            print("收到: " + $data);
            
            // 发送响应
            $conn->write("Echo: " + $data);
        }
        
        // 关闭连接
        $conn->close();
    }
}

$server->stop();
```

---

## TLS/SSL 安全连接

### TLS 客户端

```sola
use sola.net.tcp.TlsClient;

// 连接到 HTTPS 服务器
$client := new TlsClient("www.google.com", 443);

// 或使用工厂方法
$client := TlsClient::create("www.google.com", 443);
$client := TlsClient::createWithTimeout("www.google.com", 443, 5000);

// 发送 HTTPS 请求
$client->write("GET / HTTP/1.1\r\n");
$client->write("Host: www.google.com\r\n");
$client->write("Connection: close\r\n\r\n");

// 读取响应
$response := $client->read(4096);
print($response);

// 获取 TLS 信息
$version := $client->getTlsVersion();       // "TLS 1.3"
$cipher := $client->getCipherSuite();        // "TLS_AES_128_GCM_SHA256"
$serverName := $client->getServerName();     // "www.google.com"

$client->close();
```

**跳过证书验证（仅用于测试）：**

```sola
use sola.net.tcp.TlsClient;

// 方式1：设置后连接
$client := new TlsClient();
$client->setSkipVerify(true);
$client->connect("self-signed.example.com", 443);

// 方式2：使用工厂方法
$client := TlsClient::createInsecure("self-signed.example.com", 443);

// 注意：生产环境中不应跳过证书验证！
```

### TLS 服务端

```sola
use sola.net.tcp.TlsServer;
use sola.net.tcp.TcpConnection;

// 创建 TLS 服务器
$server := new TlsServer();

// 设置证书（必须）
$server->setCertificate("server.crt", "server.key");

// 启动服务器
$server->start("0.0.0.0", 443);

// 或使用工厂方法
$server := TlsServer::create(443, "server.crt", "server.key");

if ($server->isListening()) {
    print("HTTPS 服务器已启动");
    
    while (true) {
        $conn := $server->accept();
        
        if ($conn != null) {
            // 处理安全连接
            $request := $conn->read(4096);
            $conn->write("HTTP/1.1 200 OK\r\n\r\nSecure Hello!");
            $conn->close();
        }
    }
}

$server->stop();
```

**生成自签名证书（用于测试）：**

```bash
# 使用 OpenSSL 生成自签名证书
openssl req -x509 -newkey rsa:4096 -keyout server.key -out server.crt -days 365 -nodes
```

---

## 异常处理

```sola
use sola.net.tcp.TcpClient;
use sola.net.SocketException;
use sola.net.ConnectionException;
use sola.net.TimeoutException;

try {
    $client := new TcpClient();
    $client->setConnectTimeout(5000);
    $client->setReadTimeout(10000);
    
    $client->connect("example.com", 80);
    
    $client->write("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n");
    $response := $client->read(4096);
    
    print($response);
    $client->close();
    
} catch (ConnectionException $e) {
    print("连接失败: " + $e->getMessage());
    
} catch (TimeoutException $e) {
    print("操作超时: " + $e->getMessage());
    
} catch (SocketException $e) {
    print("网络错误: " + $e->getMessage());
}
```

---

## 完整示例

### Echo 服务器

```sola
/**
 * Echo 服务器示例
 * 接收客户端消息并原样返回
 */

use sola.net.tcp.TcpServer;
use sola.net.tcp.TcpConnection;

$port := 8080;
$server := TcpServer::create($port);

if (!$server->isListening()) {
    print("服务器启动失败！端口 " + $port + " 可能已被占用");
} else {
    print("========================================");
    print("  Echo 服务器已启动");
    print("  监听地址: " + $server->getAddress());
    print("  按 Ctrl+C 停止服务器");
    print("========================================");
    print("");
    
    while ($server->isListening()) {
        print("等待新连接...");
        $conn := $server->accept();
        
        if ($conn != null) {
            print("[" + $conn->getRemoteAddress() + "] 已连接");
            
            // 设置连接超时
            $conn->setReadTimeout(60000);
            
            // 读取消息
            $message := $conn->readLine();
            
            if ($message != "") {
                print("[" + $conn->getRemoteAddress() + "] 收到: " + $message);
                
                // 发送回复
                $response := "Echo: " + $message;
                $conn->write($response);
                print("[" + $conn->getRemoteAddress() + "] 已回复");
            }
            
            $conn->close();
            print("[" + $conn->getRemoteAddress() + "] 已断开");
            print("");
        }
    }
}
```

### HTTP 客户端

```sola
/**
 * 简单的 HTTP 客户端示例
 * 获取网页内容
 */

use sola.net.tcp.TcpClient;
use sola.lang.Str;

$host := "example.com";
$port := 80;
$path := "/";

$client := new TcpClient();
$client->setConnectTimeout(10000);
$client->setReadTimeout(30000);

print("正在连接 " + $host + ":" + $port + "...");

if ($client->connect($host, $port)) {
    print("连接成功！");
    print("");
    
    // 构建 HTTP 请求
    $request := "GET " + $path + " HTTP/1.1\r\n";
    $request = $request + "Host: " + $host + "\r\n";
    $request = $request + "Connection: close\r\n";
    $request = $request + "User-Agent: Sola/1.0\r\n";
    $request = $request + "\r\n";
    
    // 发送请求
    $client->write($request);
    
    // 读取响应
    print("======== HTTP 响应 ========");
    
    $response := "";
    while ($client->isConnected()) {
        $chunk := $client->read(4096);
        if ($chunk == "") {
            break;
        }
        $response = $response + $chunk;
    }
    
    print($response);
    print("============================");
    
    $client->close();
} else {
    print("连接失败！");
}
```

### HTTPS 客户端

```sola
/**
 * HTTPS 客户端示例
 * 安全连接到网站
 */

use sola.net.tcp.TlsClient;

$host := "www.google.com";
$port := 443;

print("正在建立 TLS 安全连接到 " + $host + "...");

$client := TlsClient::createWithTimeout($host, $port, 10000);

if ($client->isConnected()) {
    print("连接成功！");
    print("TLS 版本: " + $client->getTlsVersion());
    print("加密套件: " + $client->getCipherSuite());
    print("");
    
    // 发送 HTTPS 请求
    $request := "GET / HTTP/1.1\r\n";
    $request = $request + "Host: " + $host + "\r\n";
    $request = $request + "Connection: close\r\n";
    $request = $request + "\r\n";
    
    $client->write($request);
    
    // 读取响应头
    print("======== 响应头 ========");
    
    while (true) {
        $line := $client->readLine();
        if ($line == "" || $line == "\r\n") {
            break;
        }
        print($line);
    }
    
    print("========================");
    
    $client->close();
} else {
    print("连接失败！");
}
```

---

## API 参考

### TcpClient

| 方法 | 说明 |
|------|------|
| `connect(host, port)` | 连接到服务器 |
| `close()` | 关闭连接 |
| `isConnected()` | 检查连接状态 |
| `write(data)` | 发送字符串 |
| `writeBytes(data)` | 发送字节数组 |
| `writeLine(data)` | 发送一行 |
| `read(length)` | 读取字符串 |
| `readBytes(length)` | 读取字节数组 |
| `readExact(length)` | 精确读取 |
| `readLine()` | 读取一行 |
| `readUntil(delimiter)` | 读取直到分隔符 |
| `available()` | 可读字节数 |
| `setConnectTimeout(ms)` | 设置连接超时 |
| `setReadTimeout(ms)` | 设置读取超时 |
| `setWriteTimeout(ms)` | 设置写入超时 |
| `setKeepAlive(enabled, interval)` | 设置 KeepAlive |
| `setNoDelay(enabled)` | 设置 NoDelay |
| `setSendBufferSize(size)` | 设置发送缓冲区 |
| `setReceiveBufferSize(size)` | 设置接收缓冲区 |
| `getRemoteHost()` | 获取远程主机 |
| `getRemotePort()` | 获取远程端口 |
| `getLocalHost()` | 获取本地主机 |
| `getLocalPort()` | 获取本地端口 |

### TcpServer

| 方法 | 说明 |
|------|------|
| `start(host, port)` | 启动服务器 |
| `stop()` | 停止服务器 |
| `isListening()` | 检查监听状态 |
| `accept()` | 接受新连接（阻塞） |
| `acceptTimeout(ms)` | 带超时接受连接 |
| `setAcceptTimeout(ms)` | 设置默认接受超时 |
| `getHost()` | 获取监听主机 |
| `getPort()` | 获取监听端口 |
| `getAddress()` | 获取完整地址 |

### TcpConnection

| 方法 | 说明 |
|------|------|
| `close()` | 关闭连接 |
| `isConnected()` | 检查连接状态 |
| `isTLS()` | 是否为 TLS 连接 |
| `write(data)` | 发送字符串 |
| `writeBytes(data)` | 发送字节数组 |
| `read(length)` | 读取字符串 |
| `readBytes(length)` | 读取字节数组 |
| `readLine()` | 读取一行 |
| `setReadTimeout(ms)` | 设置读取超时 |
| `setWriteTimeout(ms)` | 设置写入超时 |
| `setKeepAlive(enabled, interval)` | 设置 KeepAlive |
| `setNoDelay(enabled)` | 设置 NoDelay |
| `getRemoteHost()` | 获取客户端地址 |
| `getRemotePort()` | 获取客户端端口 |

### TlsClient

继承 TcpClient 的所有方法，额外提供：

| 方法 | 说明 |
|------|------|
| `setSkipVerify(skip)` | 设置跳过证书验证 |
| `getTlsVersion()` | 获取 TLS 版本 |
| `getCipherSuite()` | 获取加密套件 |
| `getServerName()` | 获取服务器名称 |

### TlsServer

继承 TcpServer 的所有方法，额外提供：

| 方法 | 说明 |
|------|------|
| `setCertificate(certFile, keyFile)` | 设置证书文件 |
| `getCertFile()` | 获取证书路径 |
| `getKeyFile()` | 获取私钥路径 |



