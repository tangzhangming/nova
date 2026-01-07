# Sola HTTP 标准库

轻量级 HTTP/1.1 服务器实现，支持文件上传和 Session 管理。

## 目录

- [快速开始](#快速开始)
- [核心组件](#核心组件)
- [Request 对象](#request-对象)
- [Response 对象](#response-对象)
- [文件上传](#文件上传)
- [Session 管理](#session-管理)
- [完整示例](#完整示例)

---

## 快速开始

### 最简服务器

```sola
use sola.net.http.{HttpServer, Request, Response, HttpStatus};

$server := new HttpServer("0.0.0.0", 8080);

$server->handle(function(Request $req, Response $res) {
    $res->setStatus(HttpStatus::OK);
    $res->setHeader("Content-Type", "text/plain");
    $res->writeString("Hello, World!");
});

echo "Server running on http://localhost:8080";
$server->serve();
```

---

## 核心组件

### HttpServer

HTTP 服务器主类。

```sola
$server := new HttpServer("0.0.0.0", 8080);

// 配置（链式调用）
$server
    ->setReadTimeout(60000)      // 读取超时 60 秒
    ->setWriteTimeout(60000)     // 写入超时 60 秒
    ->setMaxHeaderBytes(1048576) // 最大头部 1MB
    ->setMaxBodyBytes(10485760); // 最大 Body 10MB

// 设置处理器
$server->handle(function(Request $req, Response $res) {
    // 处理请求
});

// 启动
$server->serve();

// 关闭
$server->shutdown();
```

### HttpMethod

HTTP 方法常量。

```sola
use sola.net.http.HttpMethod;

if ($req->getMethod() == HttpMethod::POST) {
    // 处理 POST 请求
}

// 检查方法
HttpMethod::isValid("GET");     // true
HttpMethod::isSafe("GET");      // true（不修改资源）
HttpMethod::isIdempotent("PUT"); // true（幂等）
```

### HttpStatus

HTTP 状态码常量。

```sola
use sola.net.http.HttpStatus;

$res->setStatus(HttpStatus::OK);           // 200
$res->setStatus(HttpStatus::CREATED);      // 201
$res->setStatus(HttpStatus::NOT_FOUND);    // 404

// 获取状态文本
HttpStatus::text(200);  // "OK"
HttpStatus::text(404);  // "Not Found"

// 状态判断
HttpStatus::isSuccess(200);     // true
HttpStatus::isClientError(404); // true
HttpStatus::isServerError(500); // true
```

### HttpException

HTTP 异常类。

```sola
use sola.net.http.HttpException;

throw new HttpException(404, "Resource not found");

// 快捷方法
throw HttpException::notFound("User not found");
throw HttpException::badRequest("Invalid input");
throw HttpException::unauthorized("Login required");
throw HttpException::forbidden("Access denied");
throw HttpException::internalServerError("Something went wrong");
```

---

## Request 对象

### 基本信息

```sola
$req->getMethod();      // "GET", "POST", etc.
$req->getPath();        // "/api/users"
$req->getRequestURI();  // "/api/users?page=1"
$req->getRawQuery();    // "page=1"
$req->getProtocol();    // "HTTP/1.1"
```

### 头部

```sola
$req->getHeader()->get("Content-Type");
$req->getHost();           // "localhost:8080"
$req->getContentLength();  // 1234
$req->getContentType();    // "application/json"
$req->getUserAgent();      // "Mozilla/5.0..."
$req->getReferer();        // 来源页面
```

### 查询参数

```sola
// GET /users?page=1&limit=10&active=true

$req->queryValue("page");          // "1"
$req->queryValue("sort", "id");    // "id"（默认值）
$req->queryInt("page", 1);         // 1
$req->queryBool("active", false);  // true
$req->hasQuery("page");            // true
$req->queryValues("tags");         // ["a", "b"]（多值）
```

### 表单数据

```sola
// POST Content-Type: application/x-www-form-urlencoded

$req->parseForm();                    // 解析表单
$req->formValue("username");          // "alice"
$req->formValue("email", "");         // 带默认值
$req->formInt("age", 0);              // 整数值
$req->hasForm("username");            // true
```

### POST 表单

```sola
// 仅获取 POST body 中的表单数据（不含 URL 查询参数）
$req->postFormValue("data");
```

### Cookie

```sola
$req->cookie("session_id");           // Cookie 对象
$req->cookie("session_id")->value;    // Cookie 值
$req->hasCookie("session_id");        // true
$req->getCookies();                   // 所有 Cookie
```

### 请求体

```sola
$req->getBody();        // byte[]
$req->getBodyString();  // string
```

### 连接信息

```sola
$req->getRemoteAddr();   // "192.168.1.100:54321"
$req->getRemoteHost();   // "192.168.1.100"
$req->getRemotePort();   // 54321
$req->isSecure();        // HTTPS?
```

### 请求类型判断

```sola
$req->isAjax();      // X-Requested-With: XMLHttpRequest
$req->isJson();      // Content-Type: application/json
$req->wantsJson();   // Accept: application/json
$req->accepts("text/html");
```

### 上下文存储

```sola
// 在请求中存储自定义数据
$req->set("user", $currentUser);
$req->get("user");
$req->has("user");
```

---

## Response 对象

### 状态码

```sola
$res->setStatus(HttpStatus::OK);        // 链式调用
$res->writeHeader(200);                 // Go 风格
$res->getStatusCode();                  // 获取状态码
```

### 头部

```sola
$res->setHeader("Content-Type", "application/json");  // 覆盖
$res->addHeader("Set-Cookie", "a=1");                 // 追加
$res->delHeader("X-Custom");                          // 删除
$res->getHeader()->get("Content-Type");               // 获取
```

### Cookie

```sola
use sola.net.http.Cookie;

// 设置 Cookie
$cookie := new Cookie("user_id", "12345");
$cookie
    ->withPath("/")
    ->withMaxAge(3600)
    ->withHttpOnly(true)
    ->withSecure(true)
    ->withSameSite("Strict");

$res->setCookie($cookie);

// 删除 Cookie
$res->deleteCookie("user_id");
```

### 写入内容

```sola
$res->writeString("Hello");           // 写字符串
$res->write($bytes);                  // 写字节数组
$res->flush();                        // 刷新缓冲区
```

### 状态查询

```sola
$res->isHeaderWritten();  // 头部是否已发送
$res->bytesWritten();     // 已写入字节数
```

---

## 文件上传

### 单文件上传

```sola
$server->handle(function(Request $req, Response $res) {
    if ($req->getMethod() == "POST") {
        // 解析 multipart 表单（最大 32MB 内存）
        if (!$req->parseMultipartForm(32 << 20)) {
            $res->setStatus(HttpStatus::BAD_REQUEST);
            $res->writeString("Failed to parse form");
            return;
        }
        
        // 获取上传文件
        $file := $req->formFile("avatar");
        
        // 验证
        if (!$file->isValid()) {
            $res->setStatus(HttpStatus::BAD_REQUEST);
            $res->writeString($file->getErrorMessage());
            return;
        }
        
        // 文件信息
        echo "Name: " + $file->getName();          // 原始文件名
        echo "Size: " + $file->getSize();          // 大小（字节）
        echo "Type: " + $file->getContentType();   // MIME 类型
        echo "Ext: " + $file->getExtension();      // 扩展名
        
        // 验证
        if (!$file->isImage()) {
            $res->setStatus(HttpStatus::BAD_REQUEST);
            $res->writeString("Only images allowed");
            return;
        }
        
        if (!$file->maxSize(5 << 20)) {  // 5MB
            $res->setStatus(HttpStatus::BAD_REQUEST);
            $res->writeString("File too large");
            return;
        }
        
        // 允许的 MIME 类型
        $allowedTypes := ["image/jpeg", "image/png", "image/gif"];
        if (!$file->isAllowedType($allowedTypes)) {
            $res->setStatus(HttpStatus::BAD_REQUEST);
            $res->writeString("Invalid file type");
            return;
        }
        
        // 保存文件
        $dest := "./uploads/" + $file->getSafeFileName();
        if ($file->moveTo($dest)) {
            $res->writeString("Uploaded: " + $dest);
        } else {
            $res->setStatus(HttpStatus::INTERNAL_SERVER_ERROR);
            $res->writeString("Failed to save");
        }
    }
});
```

### 多文件上传

```sola
// 获取同一字段的多个文件
$files := $req->formFiles("photos");
$count := len($files);

for ($i := 0; $i < $count; $i++) {
    $file := $files[$i];
    if ($file->isValid()) {
        $file->moveTo("./uploads/" + $file->generateUniqueName());
    }
}
```

### UploadedFile 方法

```sola
// 文件信息
$file->getName();           // 原始文件名
$file->getClientName();     // 客户端文件名（别名）
$file->getSize();           // 大小（字节）
$file->getContentType();    // MIME 类型
$file->getExtension();      // 扩展名
$file->getTmpPath();        // 临时文件路径

// 错误处理
$file->getError();          // 错误码
$file->getErrorMessage();   // 错误信息
$file->isValid();           // 是否上传成功
$file->hasError();          // 是否有错误

// 文件操作
$file->moveTo("/path/to/dest");     // 移动
$file->copyTo("/path/to/dest");     // 复制
$file->getContents();               // 读取内容 (byte[])
$file->getContentsString();         // 读取内容 (string)
$file->getStream();                 // 获取流
$file->delete();                    // 删除临时文件

// 验证
$file->isImage();                   // 是否图片
$file->isVideo();                   // 是否视频
$file->isAudio();                   // 是否音频
$file->isText();                    // 是否文本
$file->isAllowedType($types);       // 验证 MIME 类型
$file->isAllowedExtension($exts);   // 验证扩展名
$file->maxSize($bytes);             // 验证最大大小
$file->minSize($bytes);             // 验证最小大小

// 工具
$file->getSafeFileName();           // 安全文件名
$file->generateUniqueName();        // 生成唯一文件名
```

---

## Session 管理

Session 需要在 handler 中**手动开启**，这样可以按需使用，避免不必要的存储开销。

### 配置

```sola
use sola.net.http.session.{MemoryStore, FileStore};

$server := new HttpServer("0.0.0.0", 8080);

// 使用内存存储（开发环境）
$server->setSessionStore(new MemoryStore());

// 使用文件存储（生产环境）
$server->setSessionStore(new FileStore("./sessions"));

// 其他配置
$server->setSessionCookieName("MYSESSID");  // Cookie 名称
$server->setSessionLifetime(7200);          // 2 小时
```

### 基本使用

```sola
$server->handle(function(Request $req, Response $res) use ($server) {
    $path := $req->getPath();
    
    // 登录 - 需要 Session
    if ($path == "/login" && $req->getMethod() == "POST") {
        // 手动开启 Session
        $session := $server->startSession($req, $res);
        
        $username := $req->formValue("username");
        $session->set("user", $username);
        $session->set("login_time", native_time());
        
        // Flash 消息（只显示一次）
        $session->flash("message", "登录成功！");
        
        $session->save();
        
        $res->setStatus(302);
        $res->setHeader("Location", "/dashboard");
        return;
    }
    
    // 仪表板 - 需要 Session
    if ($path == "/dashboard") {
        $session := $server->startSession($req, $res);
        
        $user := $session->get("user");
        if ($user == null) {
            $res->setStatus(302);
            $res->setHeader("Location", "/login");
            return;
        }
        
        // 获取并消费 Flash 消息
        $message := $session->getFlash("message", "");
        
        $session->save();
        
        $res->setHeader("Content-Type", "text/html");
        $res->writeString("<h1>Welcome, " + $user + "</h1>");
        if ($message != "") {
            $res->writeString("<p class='success'>" + $message + "</p>");
        }
        return;
    }
    
    // 登出
    if ($path == "/logout") {
        $server->destroySession($req, $res);
        $res->setStatus(302);
        $res->setHeader("Location", "/");
        return;
    }
    
    // 首页 - 不需要 Session
    $res->writeString("<a href='/login'>Login</a>");
});
```

### Session 方法

```sola
// Session ID
$session->getId();          // 获取 ID
$session->regenerateId();   // 重新生成 ID（防止固定攻击）

// 数据操作
$session->set("key", $value);
$session->get("key");
$session->get("key", $default);
$session->has("key");
$session->delete("key");
$session->clear();
$session->all();

// Flash 消息
$session->flash("key", $value);     // 设置（下次请求可用）
$session->getFlash("key");          // 获取并消费
$session->hasFlash("key");          // 检查
$session->reflash();                // 保留所有到下次请求
$session->keep(["key1", "key2"]);   // 保留指定 Flash

// 元数据
$session->getCreatedAt();
$session->getLastAccessedAt();
$session->isExpired();

// 生命周期
$session->save();     // 保存
$session->destroy();  // 销毁
```

### 自定义存储

实现 `SessionStore` 接口：

```sola
use sola.net.http.session.{SessionStore, SessionData};

class RedisStore implements SessionStore {
    
    public function read(string $id): SessionData {
        // 从 Redis 读取
    }
    
    public function write(string $id, SessionData $data, int $lifetime): bool {
        // 写入 Redis
    }
    
    public function destroy(string $id): bool {
        // 删除
    }
    
    public function gc(int $maxLifetime): int {
        // 垃圾回收
    }
    
    public function exists(string $id): bool {
        // 检查存在
    }
}

$server->setSessionStore(new RedisStore());
```

---

## 完整示例

### RESTful API 服务器

```sola
use sola.net.http.{HttpServer, Request, Response, HttpStatus, HttpMethod};
use sola.json.Json;

$server := new HttpServer("0.0.0.0", 8080);

$server->handle(function(Request $req, Response $res) {
    $method := $req->getMethod();
    $path := $req->getPath();
    
    // 路由
    if ($path == "/api/users" && $method == HttpMethod::GET) {
        $users := [
            ["id" => 1, "name" => "Alice"],
            ["id" => 2, "name" => "Bob"]
        ];
        
        $res->setHeader("Content-Type", "application/json");
        $res->writeString(Json::encode($users));
        return;
    }
    
    if ($path == "/api/users" && $method == HttpMethod::POST) {
        $body := $req->getBodyString();
        $data := Json::decode($body);
        
        // 创建用户...
        
        $res->setStatus(HttpStatus::CREATED);
        $res->setHeader("Content-Type", "application/json");
        $res->writeString(Json::encode(["id" => 3, "name" => $data["name"]]));
        return;
    }
    
    $res->setStatus(HttpStatus::NOT_FOUND);
    $res->setHeader("Content-Type", "application/json");
    $res->writeString(Json::encode(["error" => "Not Found"]));
});

$server->serve();
```

### 带 Session 的登录系统

```sola
use sola.net.http.{HttpServer, Request, Response, HttpStatus};
use sola.net.http.session.FileStore;

$server := new HttpServer("0.0.0.0", 8080);

// 配置 Session
$server
    ->setSessionStore(new FileStore("./sessions"))
    ->setSessionLifetime(86400);  // 24 小时

$server->handle(function(Request $req, Response $res) use ($server) {
    $path := $req->getPath();
    $method := $req->getMethod();
    
    // === 公开页面 ===
    
    if ($path == "/" && $method == "GET") {
        $res->setHeader("Content-Type", "text/html");
        $res->writeString("
            <h1>Welcome</h1>
            <a href='/login'>Login</a> | 
            <a href='/dashboard'>Dashboard</a>
        ");
        return;
    }
    
    // === 登录页面 ===
    
    if ($path == "/login" && $method == "GET") {
        $res->setHeader("Content-Type", "text/html");
        $res->writeString("
            <form method='POST'>
                <input name='username' placeholder='Username'>
                <input name='password' type='password' placeholder='Password'>
                <button>Login</button>
            </form>
        ");
        return;
    }
    
    if ($path == "/login" && $method == "POST") {
        $req->parseForm();
        $username := $req->formValue("username");
        $password := $req->formValue("password");
        
        // 验证（示例）
        if ($username == "admin" && $password == "secret") {
            $session := $server->startSession($req, $res);
            $session->set("user", $username);
            $session->set("role", "admin");
            $session->flash("welcome", "Welcome back, " + $username + "!");
            $session->save();
            
            $res->setStatus(302);
            $res->setHeader("Location", "/dashboard");
        } else {
            $res->setHeader("Content-Type", "text/html");
            $res->writeString("<p style='color:red'>Invalid credentials</p>");
        }
        return;
    }
    
    // === 需要登录的页面 ===
    
    if ($path == "/dashboard") {
        $session := $server->startSession($req, $res);
        
        $user := $session->get("user");
        if ($user == null) {
            $res->setStatus(302);
            $res->setHeader("Location", "/login");
            return;
        }
        
        $welcome := $session->getFlash("welcome", "");
        $session->save();
        
        $res->setHeader("Content-Type", "text/html");
        $html := "<h1>Dashboard</h1>";
        if ($welcome != "") {
            $html = $html + "<p style='color:green'>" + $welcome + "</p>";
        }
        $html = $html + "<p>Logged in as: " + $user + "</p>";
        $html = $html + "<a href='/logout'>Logout</a>";
        $res->writeString($html);
        return;
    }
    
    // === 登出 ===
    
    if ($path == "/logout") {
        $server->destroySession($req, $res);
        $res->setStatus(302);
        $res->setHeader("Location", "/");
        return;
    }
    
    // === 404 ===
    
    $res->setStatus(HttpStatus::NOT_FOUND);
    $res->writeString("Not Found");
});

echo "Server running on http://localhost:8080";
$server->serve();
```

---

## 与 Go net/http 对照

| Go net/http | Sola HTTP |
|-------------|-----------|
| `http.Request.Method` | `Request::getMethod()` |
| `http.Request.URL.Path` | `Request::getPath()` |
| `http.Request.URL.RawQuery` | `Request::getRawQuery()` |
| `http.Request.Header` | `Request::getHeader()` |
| `http.Request.Body` | `Request::getBody()` |
| `http.Request.FormValue()` | `Request::formValue()` |
| `http.Request.PostFormValue()` | `Request::postFormValue()` |
| `http.Request.FormFile()` | `Request::formFile()` |
| `http.Request.ParseMultipartForm()` | `Request::parseMultipartForm()` |
| `http.Request.Cookie()` | `Request::cookie()` |
| `http.Request.RemoteAddr` | `Request::getRemoteAddr()` |
| `http.ResponseWriter.Header()` | `Response::getHeader()` |
| `http.ResponseWriter.WriteHeader()` | `Response::writeHeader()` |
| `http.ResponseWriter.Write()` | `Response::write()` |
| `http.SetCookie()` | `Response::setCookie()` |
| `http.ListenAndServe()` | `HttpServer::serve()` |






