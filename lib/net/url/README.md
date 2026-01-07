# Sola URL 标准库

URL 处理库提供了完整的 URL 解析、构建、编码解码等功能，类似 Go 的 `net/url` 包。

## 快速开始

```sola
use sola.net.url.Url;

// 解析 URL
$url := Url::parse("https://example.com:8080/path?key=value#section");
echo $url->getScheme();    // "https"
echo $url->getHost();      // "example.com:8080"
echo $url->getPath();      // "/path"

// 构建 URL
$url := new Url();
$url->setScheme("https")->setHost("example.com")->setPath("/api/users");
echo $url->toString();  // "https://example.com/api/users"
```

## 核心 API

### Url 类

#### parse(rawUrl) : Url

解析完整 URL 字符串。

```sola
$url := Url::parse("https://user:pass@example.com:8080/path?q=test#frag");

echo $url->getScheme();    // "https"
echo $url->getHost();      // "example.com:8080"
echo $url->getHostname();  // "example.com"
echo $url->getPort();      // 8080
echo $url->getPath();      // "/path"
echo $url->getRawQuery();  // "q=test"
echo $url->getFragment();  // "frag"
```

#### parseRequestURI(rawURI) : Url

解析请求 URI（仅 path?query#fragment，不含 scheme 和 host）。

```sola
$url := Url::parseRequestURI("/api/users?page=1#top");
echo $url->getPath();      // "/api/users"
echo $url->getRawQuery();  // "page=1"
echo $url->getFragment();  // "top"
```

#### 获取组件

```sola
$url := Url::parse("https://example.com/path?q=test");

$url->getScheme();     // "https"
$url->getHost();       // "example.com"
$url->getHostname();   // "example.com"（不含端口）
$url->getPort();       // -1（未指定）
$url->getPath();       // "/path"（解码后）
$url->getRawPath();    // "/path"（原始路径）
$url->getQuery();      // UrlValues 对象
$url->getRawQuery();   // "q=test"
$url->getFragment();   // ""
```

#### 设置组件

```sola
$url := new Url();
$url
    ->setScheme("https")
    ->setHost("example.com")
    ->setPath("/api/users")
    ->setQuery("page=1&limit=10")
    ->setFragment("top");

echo $url->toString();
// "https://example.com/api/users?page=1&limit=10#top"
```

#### toString() : string

转为完整 URL 字符串。

```sola
$url := Url::parse("https://example.com/path?q=test");
echo $url->toString();  // "https://example.com/path?q=test"
```

#### toRequestURI() : string

转为请求 URI（仅 path?query#fragment）。

```sola
$url := Url::parse("https://example.com/path?q=test#top");
echo $url->toRequestURI();  // "/path?q=test#top"
```

#### resolve(ref) : Url

解析相对路径。

```sola
$base := Url::parse("https://example.com/api/v1/users");
$ref := "../../v2/posts";
$resolved := $base->resolve($ref);

echo $resolved->toString();
// "https://example.com/api/v2/posts"
```

#### join(path) : Url

拼接路径。

```sola
$url := Url::parse("https://example.com/api");
$url->join("users");

echo $url->getPath();  // "/api/users"
```

#### 编码/解码方法

```sola
// 路径编码/解码
$encoded := Url::pathEscape("/path with spaces");
echo $encoded;  // "/path%20with%20spaces"

$decoded := Url::pathUnescape("/path%20with%20spaces");
echo $decoded;  // "/path with spaces"

// 查询参数编码/解码
$encoded := Url::queryEscape("hello world");
echo $encoded;  // "hello+world"

$decoded := Url::queryUnescape("hello+world");
echo $decoded;  // "hello world"
```

---

### UrlValues 类

#### parse(query) : UrlValues

从查询字符串解析。

```sola
use sola.net.url.UrlValues;

$values := UrlValues::parse("name=Alice&age=25&tags=web&tags=api");

echo $values->get("name");        // "Alice"
echo $values->get("age");         // "25"
echo $values->getInt("age");      // 25
echo $values->getAll("tags");     // ["web", "api"]
```

#### 获取值

```sola
$values := UrlValues::parse("page=1&limit=10&active=true");

// 获取字符串值
$values->get("page");              // "1"
$values->get("page", "0");         // "1"（带默认值）
$values->get("missing", "default"); // "default"

// 获取整数值
$values->getInt("page");           // 1
$values->getInt("page", 0);        // 1（带默认值）
$values->getInt("missing", 0);      // 0

// 获取布尔值
$values->getBool("active");        // true
$values->getBool("active", false); // true（带默认值）

// 获取所有值（多值参数）
$values->getAll("tags");           // ["web", "api"]
```

#### 设置值

```sola
$values := new UrlValues();

// 设置值（覆盖所有）
$values->set("page", "1");
$values->set("limit", "10");

// 添加值（追加，支持多值）
$values->add("tags", "web");
$values->add("tags", "api");

// 删除
$values->del("page");
```

#### encode() : string

编码为查询字符串。

```sola
$values := new UrlValues();
$values->set("q", "search term")->set("page", "1");

echo $values->encode();
// "page=1&q=search+term"
```

#### 其他方法

```sola
$values->has("key");      // 检查是否存在
$values->isEmpty();       // 是否为空
$values->clear();         // 清空
$values->copy();          // 复制
$values->toMap();         // 转为单值映射
```

---

## 使用示例

### 在 HTTP 服务器中使用

```sola
use sola.net.http.{HttpServer, Request, Response};
use sola.net.url.Url;

$server->handle(function(Request $req, Response $res) {
    // 获取完整 URL
    $url := Url::parse("http://" + $req->getHost() + $req->getRequestURI());
    
    // 获取查询参数
    $query := $url->getQuery();
    $page := $query->getInt("page", 1);
    $limit := $query->getInt("limit", 10);
    
    // 构建下一页 URL
    $nextUrl := $url->copy();
    $nextUrl->getQuery()->set("page", ($page + 1)->toString());
    
    $res->setHeader("Link", '<' + $nextUrl->toString() + '>; rel="next"');
});
```

### URL 构建

```sola
use sola.net.url.Url;

$url := new Url();
$url
    ->setScheme("https")
    ->setHost("api.example.com")
    ->setPath("/v1/users")
    ->getQuery()
        ->set("page", "1")
        ->set("limit", "10");

echo $url->toString();
// "https://api.example.com/v1/users?limit=10&page=1"
```

### 相对路径解析

```sola
use sola.net.url.Url;

$base := Url::parse("https://example.com/api/v1/users/123");
$ref := "../../v2/posts/456";
$resolved := $base->resolve($ref);

echo $resolved->toString();
// "https://example.com/api/v2/posts/456"
```

### 查询参数处理

```sola
use sola.net.url.UrlValues;

// 解析
$values := UrlValues::parse("name=Alice&age=25&tags=web&tags=api");

// 修改
$values->set("age", "26");
$values->add("tags", "mobile");

// 编码
echo $values->encode();
// "age=26&name=Alice&tags=web&tags=api&tags=mobile"
```

---

## 与 Go net/url 对照

| Go net/url | Sola net/url | 说明 |
|------------|--------------|------|
| `url.Parse()` | `Url::parse()` | 解析 URL |
| `url.URL` | `Url` | URL 对象 |
| `url.Values` | `UrlValues` | 查询参数 |
| `url.QueryEscape()` | `Url::queryEscape()` | 查询编码 |
| `url.PathEscape()` | `Url::pathEscape()` | 路径编码 |
| `u.String()` | `$url->toString()` | 转为字符串 |
| `u.Hostname()` | `$url->getHostname()` | 获取主机名 |
| `u.Port()` | `$url->getPort()` | 获取端口 |

---

## 注意事项

1. **编码/解码实现**：当前使用简化实现，完整实现需要 native 函数支持
2. **IPv6 地址**：支持 IPv6 地址格式 `[::1]:8080`
3. **相对路径**：`resolve()` 方法支持 `../` 和 `./` 路径解析
4. **查询参数**：支持多值参数，使用 `add()` 方法添加多个值
5. **路径编码**：路径中的 `/` 不会被编码，其他特殊字符会被编码

---

## 异常处理

```sola
use sola.net.url.{Url, UrlException};

try {
    $url := Url::parse("invalid://url");
} catch (UrlException $e) {
    echo "URL 解析失败: " + $e->getMessage();
}
```





