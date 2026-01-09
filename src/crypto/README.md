# Sola Crypto 加密库

完整的加密标准库，提供哈希、HMAC、对称加密、非对称加密、密钥派生等功能。

## 功能概览

| 分类 | 算法 | 类 |
|------|------|-----|
| **哈希** | MD5, SHA1, SHA256, SHA384, SHA512 | `Hash` |
| **HMAC** | HMAC-MD5, HMAC-SHA1, HMAC-SHA256, HMAC-SHA384, HMAC-SHA512 | `Hmac` |
| **对称加密** | AES-CBC, AES-GCM, AES-CTR | `Aes` |
| **对称加密** | DES, 3DES | `Des` |
| **非对称加密** | RSA (OAEP, PKCS1v15) | `Rsa` |
| **数字签名** | RSA-PSS, RSA-PKCS1v15 | `Rsa` |
| **数字签名** | ECDSA (P-256, P-384, P-521) | `Ecdsa` |
| **数字签名** | Ed25519 | `Ed25519` |
| **密钥派生** | PBKDF2 | `Pbkdf2` |
| **密钥派生** | HKDF | `Hkdf` |
| **密钥派生** | Scrypt | `Scrypt` |
| **密钥派生** | Argon2id, Argon2i | `Argon2` |
| **随机数** | 加密安全随机数 | `Random` |
| **编码** | 十六进制 | `Hex` |

## 快速开始

### 哈希

```sola
use sola.crypto.Hash;

// 计算哈希
string $md5 = Hash::md5("hello");
string $sha256 = Hash::sha256("hello");
string $sha512 = Hash::sha512("hello");

// 流式哈希（大文件）
$hasher := Hash::createSha256();
$hasher->update("part1");
$hasher->update("part2");
string $hash = $hasher->finalize();

// 文件哈希
string $fileHash = Hash::file("sha256", "/path/to/file");
```

### HMAC

```sola
use sola.crypto.Hmac;

// 计算HMAC
string $mac = Hmac::sha256("message", "secret-key");

// 验证HMAC
bool $valid = Hmac::verify("sha256", "message", "secret-key", $mac);

// 流式HMAC
$hmac := Hmac::create("sha256", "secret-key");
$hmac->update("part1")->update("part2");
string $result = $hmac->finalize();
```

### AES加密

```sola
use sola.crypto.{Aes, Random};

// 简化API（推荐）
byte[] $encrypted = Aes::encrypt("secret message", "password");
byte[] $decrypted = Aes::decrypt($encrypted, "password");

// 或使用字符串
string $encStr = Aes::encryptToString("secret", "password");
string $decStr = Aes::decryptFromString($encStr, "password");

// AES-256-CBC
byte[] $key = Random::bytes(32);  // 256位密钥
byte[] $iv = Random::bytes(16);
byte[] $encrypted = Aes::encryptCbc($plaintext, $key, $iv);
byte[] $decrypted = Aes::decryptCbc($encrypted, $key, $iv);

// AES-256-GCM（带认证）
byte[] $nonce = Random::bytes(12);
AesGcmResult $result = Aes::encryptGcm($plaintext, $key, $nonce);
byte[] $decrypted = Aes::decryptGcm($result->ciphertext, $key, $nonce);
```

### RSA加密

```sola
use sola.crypto.Rsa;

// 生成密钥对
RsaKeyPair $keys = Rsa::generateKeyPair(2048);
string $publicPem = $keys->getPublicKeyPem();
string $privatePem = $keys->getPrivateKeyPem();

// 加密/解密
byte[] $encrypted = Rsa::encrypt("secret", $keys->getPublicKey());
byte[] $decrypted = Rsa::decrypt($encrypted, $keys->getPrivateKey());

// 签名/验签
byte[] $signature = Rsa::sign("message", $keys->getPrivateKey(), "sha256");
bool $valid = Rsa::verify("message", $signature, $keys->getPublicKey(), "sha256");

// 从PEM加载
RsaPublicKey $pub = Rsa::loadPublicKey($publicPem);
RsaPrivateKey $priv = Rsa::loadPrivateKey($privatePem);
```

### ECDSA签名

```sola
use sola.crypto.Ecdsa;

// 生成密钥对
EcdsaKeyPair $keys = Ecdsa::generateKeyPair("P-256");

// 签名
byte[] $signature = Ecdsa::sign("message", $keys->getPrivateKey());

// 验签
bool $valid = Ecdsa::verify("message", $signature, $keys->getPublicKey());
```

### Ed25519签名

```sola
use sola.crypto.Ed25519;

// 生成密钥对
Ed25519KeyPair $keys = Ed25519::generateKeyPair();

// 签名
byte[] $signature = Ed25519::sign("message", $keys->getPrivateKey());

// 验签
bool $valid = Ed25519::verify("message", $signature, $keys->getPublicKey());

// 导出/导入密钥
byte[] $pubBytes = $keys->getPublicKey()->toBytes();
Ed25519PublicKey $pub = Ed25519::loadPublicKey($pubBytes);
```

### 密码哈希

```sola
use sola.crypto.{Pbkdf2, Scrypt, Argon2};

// PBKDF2
string $hash = Pbkdf2::hash("password123");
bool $valid = Pbkdf2::verify("password123", $hash);

// Scrypt（更安全）
string $hash = Scrypt::hash("password123");
bool $valid = Scrypt::verify("password123", $hash);

// Argon2id（推荐）
string $hash = Argon2::hash("password123");
bool $valid = Argon2::verify("password123", $hash);
```

### 密钥派生

```sola
use sola.crypto.{Pbkdf2, Hkdf, Scrypt, Argon2, Random};

// PBKDF2
byte[] $salt = Random::bytes(16);
byte[] $key = Pbkdf2::derive("password", $salt, 100000, 32, "sha256");

// HKDF（从主密钥派生多个密钥）
byte[] $masterKey = Random::bytes(32);
byte[] $encKey = Hkdf::derive($masterKey, [], Bytes::fromString("encryption"), 32);
byte[] $macKey = Hkdf::derive($masterKey, [], Bytes::fromString("mac"), 32);

// Scrypt
byte[] $key = Scrypt::derive("password", $salt, 32768, 8, 1, 32);

// Argon2id
byte[] $key = Argon2::deriveId("password", $salt, 3, 65536, 4, 32);
```

### 随机数

```sola
use sola.crypto.Random;

// 随机字节
byte[] $key = Random::bytes(32);

// 随机整数
int $num = Random::int(1, 100);  // [1, 100)

// 随机十六进制
string $token = Random::hex(16);  // 32字符

// UUID
string $uuid = Random::uuid();

// 随机字符串
string $password = Random::string(16);
string $code = Random::string(6, "0123456789");

// 随机洗牌
array $shuffled = Random::shuffle($array);

// 随机选择
mixed $item = Random::choice($array);
```

### 十六进制编码

```sola
use sola.crypto.Hex;

// 编码
string $hex = Hex::encode($bytes);  // "48656c6c6f"

// 解码
byte[] $bytes = Hex::decode("48656c6c6f");

// 验证
bool $valid = Hex::isValid("48656c6c6f");  // true

// 字符串便捷方法
string $hex = Hex::encodeString("hello");
string $str = Hex::decodeToString($hex);
```

## 算法推荐

### 哈希算法选择

| 场景 | 推荐算法 |
|------|----------|
| 一般用途 | SHA-256 |
| 高安全需求 | SHA-512 |
| 兼容旧系统 | MD5（不推荐） |
| 文件校验 | SHA-256 |

### 对称加密选择

| 场景 | 推荐算法 |
|------|----------|
| 一般加密 | AES-256-GCM |
| 需要认证 | AES-256-GCM |
| 流式加密 | AES-256-CTR |
| 兼容旧系统 | AES-256-CBC |

### 非对称加密选择

| 场景 | 推荐算法 |
|------|----------|
| 密钥交换/加密 | RSA-2048 或 RSA-4096 |
| 数字签名 | Ed25519 或 ECDSA P-256 |
| 高性能签名 | Ed25519 |

### 密码存储选择

| 场景 | 推荐算法 |
|------|----------|
| 新项目 | Argon2id |
| 内存受限 | Scrypt |
| 兼容性需求 | PBKDF2 + SHA-256 |

## 安全建议

1. **密钥长度**
   - AES: 至少256位（32字节）
   - RSA: 至少2048位
   - ECDSA: 至少P-256

2. **迭代次数**
   - PBKDF2: 至少100,000次
   - Scrypt: N=32768, r=8, p=1
   - Argon2: t=3, m=64MB, p=4

3. **随机数**
   - 始终使用 `Random` 类生成密钥和IV
   - 不要重复使用IV/Nonce

4. **密钥管理**
   - 完成后调用 `free()` 释放密钥资源
   - 敏感密钥不要记录日志

## 错误处理

```sola
use sola.crypto.{Aes, CryptoException};

try {
    byte[] $decrypted = Aes::decrypt($data, "wrong-password");
} catch (CryptoException $e) {
    if ($e->isCryptoError()) {
        echo "解密失败: " + $e->getMessage();
    }
}
```

## 与Go crypto对比

| Go | Sola |
|----|------|
| `crypto/md5` | `Hash::md5()` |
| `crypto/sha256` | `Hash::sha256()` |
| `crypto/hmac` | `Hmac::sha256()` |
| `crypto/aes` + `cipher` | `Aes::encryptGcm()` |
| `crypto/rsa` | `Rsa::encrypt()` |
| `crypto/ecdsa` | `Ecdsa::sign()` |
| `crypto/ed25519` | `Ed25519::sign()` |
| `crypto/rand` | `Random::bytes()` |
| `golang.org/x/crypto/pbkdf2` | `Pbkdf2::derive()` |
| `golang.org/x/crypto/scrypt` | `Scrypt::derive()` |
| `golang.org/x/crypto/argon2` | `Argon2::deriveId()` |







