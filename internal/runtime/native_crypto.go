package runtime

import (
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/des"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"hash"
	"io"
	"math/big"
	"sync"

	"github.com/tangzhangming/nova/internal/bytecode"
	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/hkdf"
	"golang.org/x/crypto/pbkdf2"
	"golang.org/x/crypto/scrypt"
)

// ============================================================================
// Hash Functions
// ============================================================================

// nativeCryptoMd5 计算MD5哈希
// native_crypto_md5(data string|byte[]) -> string (hex)
func nativeCryptoMd5(args []bytecode.Value) bytecode.Value {
	data := getBytes(args, 0)
	if data == nil {
		return bytecode.NewString("")
	}
	sum := md5.Sum(data)
	return bytecode.NewString(hex.EncodeToString(sum[:]))
}

// nativeCryptoMd5Bytes 计算MD5哈希（返回字节）
// native_crypto_md5_bytes(data string|byte[]) -> byte[]
func nativeCryptoMd5Bytes(args []bytecode.Value) bytecode.Value {
	data := getBytes(args, 0)
	if data == nil {
		return bytecode.NewBytes(nil)
	}
	sum := md5.Sum(data)
	return bytecode.NewBytes(sum[:])
}

// nativeCryptoSha1 计算SHA1哈希
// native_crypto_sha1(data string|byte[]) -> string (hex)
func nativeCryptoSha1(args []bytecode.Value) bytecode.Value {
	data := getBytes(args, 0)
	if data == nil {
		return bytecode.NewString("")
	}
	sum := sha1.Sum(data)
	return bytecode.NewString(hex.EncodeToString(sum[:]))
}

// nativeCryptoSha1Bytes 计算SHA1哈希（返回字节）
func nativeCryptoSha1Bytes(args []bytecode.Value) bytecode.Value {
	data := getBytes(args, 0)
	if data == nil {
		return bytecode.NewBytes(nil)
	}
	sum := sha1.Sum(data)
	return bytecode.NewBytes(sum[:])
}

// nativeCryptoSha256 计算SHA256哈希
// native_crypto_sha256(data string|byte[]) -> string (hex)
func nativeCryptoSha256(args []bytecode.Value) bytecode.Value {
	data := getBytes(args, 0)
	if data == nil {
		return bytecode.NewString("")
	}
	sum := sha256.Sum256(data)
	return bytecode.NewString(hex.EncodeToString(sum[:]))
}

// nativeCryptoSha256Bytes 计算SHA256哈希（返回字节）
func nativeCryptoSha256Bytes(args []bytecode.Value) bytecode.Value {
	data := getBytes(args, 0)
	if data == nil {
		return bytecode.NewBytes(nil)
	}
	sum := sha256.Sum256(data)
	return bytecode.NewBytes(sum[:])
}

// nativeCryptoSha384 计算SHA384哈希
func nativeCryptoSha384(args []bytecode.Value) bytecode.Value {
	data := getBytes(args, 0)
	if data == nil {
		return bytecode.NewString("")
	}
	sum := sha512.Sum384(data)
	return bytecode.NewString(hex.EncodeToString(sum[:]))
}

// nativeCryptoSha384Bytes 计算SHA384哈希（返回字节）
func nativeCryptoSha384Bytes(args []bytecode.Value) bytecode.Value {
	data := getBytes(args, 0)
	if data == nil {
		return bytecode.NewBytes(nil)
	}
	sum := sha512.Sum384(data)
	return bytecode.NewBytes(sum[:])
}

// nativeCryptoSha512 计算SHA512哈希
func nativeCryptoSha512(args []bytecode.Value) bytecode.Value {
	data := getBytes(args, 0)
	if data == nil {
		return bytecode.NewString("")
	}
	sum := sha512.Sum512(data)
	return bytecode.NewString(hex.EncodeToString(sum[:]))
}

// nativeCryptoSha512Bytes 计算SHA512哈希（返回字节）
func nativeCryptoSha512Bytes(args []bytecode.Value) bytecode.Value {
	data := getBytes(args, 0)
	if data == nil {
		return bytecode.NewBytes(nil)
	}
	sum := sha512.Sum512(data)
	return bytecode.NewBytes(sum[:])
}

// ============================================================================
// Streaming Hash Functions
// ============================================================================

var (
	hashMu     sync.Mutex
	hashMap    = make(map[int64]hash.Hash)
	hashNextID int64 = 1
)

// nativeCryptoHashCreate 创建流式哈希器
// native_crypto_hash_create(algorithm string) -> int (handle)
func nativeCryptoHashCreate(args []bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.NewInt(-1)
	}
	algo, ok := args[0].Data.(string)
	if !ok {
		return bytecode.NewInt(-1)
	}

	var h hash.Hash
	switch algo {
	case "md5":
		h = md5.New()
	case "sha1":
		h = sha1.New()
	case "sha256":
		h = sha256.New()
	case "sha384":
		h = sha512.New384()
	case "sha512":
		h = sha512.New()
	default:
		return bytecode.NewInt(-1)
	}

	hashMu.Lock()
	id := hashNextID
	hashNextID++
	hashMap[id] = h
	hashMu.Unlock()

	return bytecode.NewInt(id)
}

// nativeCryptoHashUpdate 更新哈希数据
// native_crypto_hash_update(handle int, data string|byte[]) -> bool
func nativeCryptoHashUpdate(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewBool(false)
	}
	handle, ok := args[0].Data.(int64)
	if !ok {
		return bytecode.NewBool(false)
	}
	data := getBytes(args, 1)
	if data == nil {
		return bytecode.NewBool(false)
	}

	hashMu.Lock()
	h, exists := hashMap[handle]
	hashMu.Unlock()
	if !exists {
		return bytecode.NewBool(false)
	}

	h.Write(data)
	return bytecode.NewBool(true)
}

// nativeCryptoHashFinalize 完成哈希并返回结果
// native_crypto_hash_finalize(handle int) -> string (hex)
func nativeCryptoHashFinalize(args []bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.NewString("")
	}
	handle, ok := args[0].Data.(int64)
	if !ok {
		return bytecode.NewString("")
	}

	hashMu.Lock()
	h, exists := hashMap[handle]
	if exists {
		delete(hashMap, handle)
	}
	hashMu.Unlock()
	if !exists {
		return bytecode.NewString("")
	}

	sum := h.Sum(nil)
	return bytecode.NewString(hex.EncodeToString(sum))
}

// nativeCryptoHashFinalizeBytes 完成哈希并返回字节结果
func nativeCryptoHashFinalizeBytes(args []bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.NewBytes(nil)
	}
	handle, ok := args[0].Data.(int64)
	if !ok {
		return bytecode.NewBytes(nil)
	}

	hashMu.Lock()
	h, exists := hashMap[handle]
	if exists {
		delete(hashMap, handle)
	}
	hashMu.Unlock()
	if !exists {
		return bytecode.NewBytes(nil)
	}

	return bytecode.NewBytes(h.Sum(nil))
}

// ============================================================================
// HMAC Functions
// ============================================================================

// nativeCryptoHmac 计算HMAC
// native_crypto_hmac(algorithm string, message string|byte[], key string|byte[]) -> string (hex)
func nativeCryptoHmac(args []bytecode.Value) bytecode.Value {
	if len(args) < 3 {
		return bytecode.NewString("")
	}
	algo, ok := args[0].Data.(string)
	if !ok {
		return bytecode.NewString("")
	}
	message := getBytes(args, 1)
	key := getBytes(args, 2)
	if message == nil || key == nil {
		return bytecode.NewString("")
	}

	var h func() hash.Hash
	switch algo {
	case "md5":
		h = md5.New
	case "sha1":
		h = sha1.New
	case "sha256":
		h = sha256.New
	case "sha384":
		h = sha512.New384
	case "sha512":
		h = sha512.New
	default:
		return bytecode.NewString("")
	}

	mac := hmac.New(h, key)
	mac.Write(message)
	return bytecode.NewString(hex.EncodeToString(mac.Sum(nil)))
}

// nativeCryptoHmacBytes 计算HMAC（返回字节）
func nativeCryptoHmacBytes(args []bytecode.Value) bytecode.Value {
	if len(args) < 3 {
		return bytecode.NewBytes(nil)
	}
	algo, ok := args[0].Data.(string)
	if !ok {
		return bytecode.NewBytes(nil)
	}
	message := getBytes(args, 1)
	key := getBytes(args, 2)
	if message == nil || key == nil {
		return bytecode.NewBytes(nil)
	}

	var h func() hash.Hash
	switch algo {
	case "md5":
		h = md5.New
	case "sha1":
		h = sha1.New
	case "sha256":
		h = sha256.New
	case "sha384":
		h = sha512.New384
	case "sha512":
		h = sha512.New
	default:
		return bytecode.NewBytes(nil)
	}

	mac := hmac.New(h, key)
	mac.Write(message)
	return bytecode.NewBytes(mac.Sum(nil))
}

// nativeCryptoHmacVerify 验证HMAC
// native_crypto_hmac_verify(algorithm string, message string|byte[], key string|byte[], expected string) -> bool
func nativeCryptoHmacVerify(args []bytecode.Value) bytecode.Value {
	if len(args) < 4 {
		return bytecode.NewBool(false)
	}
	algo, ok := args[0].Data.(string)
	if !ok {
		return bytecode.NewBool(false)
	}
	message := getBytes(args, 1)
	key := getBytes(args, 2)
	expected, ok := args[3].Data.(string)
	if !ok || message == nil || key == nil {
		return bytecode.NewBool(false)
	}

	var h func() hash.Hash
	switch algo {
	case "md5":
		h = md5.New
	case "sha1":
		h = sha1.New
	case "sha256":
		h = sha256.New
	case "sha384":
		h = sha512.New384
	case "sha512":
		h = sha512.New
	default:
		return bytecode.NewBool(false)
	}

	mac := hmac.New(h, key)
	mac.Write(message)
	computed := hex.EncodeToString(mac.Sum(nil))
	return bytecode.NewBool(hmac.Equal([]byte(computed), []byte(expected)))
}

// ============================================================================
// Streaming HMAC Functions
// ============================================================================

var (
	hmacMu     sync.Mutex
	hmacMap    = make(map[int64]hash.Hash)
	hmacNextID int64 = 1
)

// nativeCryptoHmacCreate 创建流式HMAC
func nativeCryptoHmacCreate(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewInt(-1)
	}
	algo, ok := args[0].Data.(string)
	if !ok {
		return bytecode.NewInt(-1)
	}
	key := getBytes(args, 1)
	if key == nil {
		return bytecode.NewInt(-1)
	}

	var h func() hash.Hash
	switch algo {
	case "md5":
		h = md5.New
	case "sha1":
		h = sha1.New
	case "sha256":
		h = sha256.New
	case "sha384":
		h = sha512.New384
	case "sha512":
		h = sha512.New
	default:
		return bytecode.NewInt(-1)
	}

	mac := hmac.New(h, key)

	hmacMu.Lock()
	id := hmacNextID
	hmacNextID++
	hmacMap[id] = mac
	hmacMu.Unlock()

	return bytecode.NewInt(id)
}

// nativeCryptoHmacUpdate 更新HMAC数据
func nativeCryptoHmacUpdate(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewBool(false)
	}
	handle, ok := args[0].Data.(int64)
	if !ok {
		return bytecode.NewBool(false)
	}
	data := getBytes(args, 1)
	if data == nil {
		return bytecode.NewBool(false)
	}

	hmacMu.Lock()
	mac, exists := hmacMap[handle]
	hmacMu.Unlock()
	if !exists {
		return bytecode.NewBool(false)
	}

	mac.Write(data)
	return bytecode.NewBool(true)
}

// nativeCryptoHmacFinalize 完成HMAC
func nativeCryptoHmacFinalize(args []bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.NewString("")
	}
	handle, ok := args[0].Data.(int64)
	if !ok {
		return bytecode.NewString("")
	}

	hmacMu.Lock()
	mac, exists := hmacMap[handle]
	if exists {
		delete(hmacMap, handle)
	}
	hmacMu.Unlock()
	if !exists {
		return bytecode.NewString("")
	}

	return bytecode.NewString(hex.EncodeToString(mac.Sum(nil)))
}

// ============================================================================
// AES Functions
// ============================================================================

// nativeCryptoAesEncryptCbc AES-CBC加密
// native_crypto_aes_encrypt_cbc(plaintext byte[], key byte[], iv byte[]) -> byte[]
func nativeCryptoAesEncryptCbc(args []bytecode.Value) bytecode.Value {
	if len(args) < 3 {
		return bytecode.NewBytes(nil)
	}
	plaintext := getBytesArg(args, 0)
	key := getBytesArg(args, 1)
	iv := getBytesArg(args, 2)
	if plaintext == nil || key == nil || iv == nil {
		return bytecode.NewBytes(nil)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return bytecode.NewBytes(nil)
	}

	// PKCS7 padding
	blockSize := block.BlockSize()
	padding := blockSize - len(plaintext)%blockSize
	padtext := make([]byte, len(plaintext)+padding)
	copy(padtext, plaintext)
	for i := len(plaintext); i < len(padtext); i++ {
		padtext[i] = byte(padding)
	}

	ciphertext := make([]byte, len(padtext))
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(ciphertext, padtext)

	return bytecode.NewBytes(ciphertext)
}

// nativeCryptoAesDecryptCbc AES-CBC解密
func nativeCryptoAesDecryptCbc(args []bytecode.Value) bytecode.Value {
	if len(args) < 3 {
		return bytecode.NewBytes(nil)
	}
	ciphertext := getBytesArg(args, 0)
	key := getBytesArg(args, 1)
	iv := getBytesArg(args, 2)
	if ciphertext == nil || key == nil || iv == nil || len(ciphertext) == 0 {
		return bytecode.NewBytes(nil)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return bytecode.NewBytes(nil)
	}

	if len(ciphertext)%block.BlockSize() != 0 {
		return bytecode.NewBytes(nil)
	}

	plaintext := make([]byte, len(ciphertext))
	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(plaintext, ciphertext)

	// Remove PKCS7 padding
	if len(plaintext) == 0 {
		return bytecode.NewBytes(nil)
	}
	padding := int(plaintext[len(plaintext)-1])
	if padding > len(plaintext) || padding > block.BlockSize() {
		return bytecode.NewBytes(nil)
	}
	for i := len(plaintext) - padding; i < len(plaintext); i++ {
		if plaintext[i] != byte(padding) {
			return bytecode.NewBytes(nil)
		}
	}

	return bytecode.NewBytes(plaintext[:len(plaintext)-padding])
}

// nativeCryptoAesEncryptGcm AES-GCM加密
// native_crypto_aes_encrypt_gcm(plaintext byte[], key byte[], nonce byte[], aad byte[]) -> byte[] (ciphertext + tag)
func nativeCryptoAesEncryptGcm(args []bytecode.Value) bytecode.Value {
	if len(args) < 3 {
		return bytecode.NewBytes(nil)
	}
	plaintext := getBytesArg(args, 0)
	key := getBytesArg(args, 1)
	nonce := getBytesArg(args, 2)
	var aad []byte
	if len(args) >= 4 {
		aad = getBytesArg(args, 3)
	}
	if plaintext == nil || key == nil || nonce == nil {
		return bytecode.NewBytes(nil)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return bytecode.NewBytes(nil)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return bytecode.NewBytes(nil)
	}

	ciphertext := aesGCM.Seal(nil, nonce, plaintext, aad)
	return bytecode.NewBytes(ciphertext)
}

// nativeCryptoAesDecryptGcm AES-GCM解密
// native_crypto_aes_decrypt_gcm(ciphertext byte[], key byte[], nonce byte[], aad byte[]) -> byte[]
func nativeCryptoAesDecryptGcm(args []bytecode.Value) bytecode.Value {
	if len(args) < 3 {
		return bytecode.NewBytes(nil)
	}
	ciphertext := getBytesArg(args, 0)
	key := getBytesArg(args, 1)
	nonce := getBytesArg(args, 2)
	var aad []byte
	if len(args) >= 4 {
		aad = getBytesArg(args, 3)
	}
	if ciphertext == nil || key == nil || nonce == nil {
		return bytecode.NewBytes(nil)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return bytecode.NewBytes(nil)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return bytecode.NewBytes(nil)
	}

	plaintext, err := aesGCM.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return bytecode.NewBytes(nil)
	}

	return bytecode.NewBytes(plaintext)
}

// nativeCryptoAesEncryptCtr AES-CTR加密
func nativeCryptoAesEncryptCtr(args []bytecode.Value) bytecode.Value {
	if len(args) < 3 {
		return bytecode.NewBytes(nil)
	}
	plaintext := getBytesArg(args, 0)
	key := getBytesArg(args, 1)
	iv := getBytesArg(args, 2)
	if plaintext == nil || key == nil || iv == nil {
		return bytecode.NewBytes(nil)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return bytecode.NewBytes(nil)
	}

	ciphertext := make([]byte, len(plaintext))
	stream := cipher.NewCTR(block, iv)
	stream.XORKeyStream(ciphertext, plaintext)

	return bytecode.NewBytes(ciphertext)
}

// nativeCryptoAesDecryptCtr AES-CTR解密 (CTR模式加密解密相同)
func nativeCryptoAesDecryptCtr(args []bytecode.Value) bytecode.Value {
	return nativeCryptoAesEncryptCtr(args)
}

// ============================================================================
// DES/3DES Functions
// ============================================================================

// nativeCryptoDesEncrypt DES-CBC加密
func nativeCryptoDesEncrypt(args []bytecode.Value) bytecode.Value {
	if len(args) < 3 {
		return bytecode.NewBytes(nil)
	}
	plaintext := getBytesArg(args, 0)
	key := getBytesArg(args, 1)
	iv := getBytesArg(args, 2)
	if plaintext == nil || key == nil || iv == nil || len(key) != 8 {
		return bytecode.NewBytes(nil)
	}

	block, err := des.NewCipher(key)
	if err != nil {
		return bytecode.NewBytes(nil)
	}

	// PKCS7 padding
	blockSize := block.BlockSize()
	padding := blockSize - len(plaintext)%blockSize
	padtext := make([]byte, len(plaintext)+padding)
	copy(padtext, plaintext)
	for i := len(plaintext); i < len(padtext); i++ {
		padtext[i] = byte(padding)
	}

	ciphertext := make([]byte, len(padtext))
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(ciphertext, padtext)

	return bytecode.NewBytes(ciphertext)
}

// nativeCryptoDesDecrypt DES-CBC解密
func nativeCryptoDesDecrypt(args []bytecode.Value) bytecode.Value {
	if len(args) < 3 {
		return bytecode.NewBytes(nil)
	}
	ciphertext := getBytesArg(args, 0)
	key := getBytesArg(args, 1)
	iv := getBytesArg(args, 2)
	if ciphertext == nil || key == nil || iv == nil || len(key) != 8 {
		return bytecode.NewBytes(nil)
	}

	block, err := des.NewCipher(key)
	if err != nil {
		return bytecode.NewBytes(nil)
	}

	if len(ciphertext)%block.BlockSize() != 0 {
		return bytecode.NewBytes(nil)
	}

	plaintext := make([]byte, len(ciphertext))
	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(plaintext, ciphertext)

	// Remove PKCS7 padding
	if len(plaintext) == 0 {
		return bytecode.NewBytes(nil)
	}
	padding := int(plaintext[len(plaintext)-1])
	if padding > len(plaintext) || padding > block.BlockSize() {
		return bytecode.NewBytes(nil)
	}

	return bytecode.NewBytes(plaintext[:len(plaintext)-padding])
}

// nativeCryptoTripleDesEncrypt 3DES-CBC加密
func nativeCryptoTripleDesEncrypt(args []bytecode.Value) bytecode.Value {
	if len(args) < 3 {
		return bytecode.NewBytes(nil)
	}
	plaintext := getBytesArg(args, 0)
	key := getBytesArg(args, 1)
	iv := getBytesArg(args, 2)
	if plaintext == nil || key == nil || iv == nil || len(key) != 24 {
		return bytecode.NewBytes(nil)
	}

	block, err := des.NewTripleDESCipher(key)
	if err != nil {
		return bytecode.NewBytes(nil)
	}

	// PKCS7 padding
	blockSize := block.BlockSize()
	padding := blockSize - len(plaintext)%blockSize
	padtext := make([]byte, len(plaintext)+padding)
	copy(padtext, plaintext)
	for i := len(plaintext); i < len(padtext); i++ {
		padtext[i] = byte(padding)
	}

	ciphertext := make([]byte, len(padtext))
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(ciphertext, padtext)

	return bytecode.NewBytes(ciphertext)
}

// nativeCryptoTripleDesDecrypt 3DES-CBC解密
func nativeCryptoTripleDesDecrypt(args []bytecode.Value) bytecode.Value {
	if len(args) < 3 {
		return bytecode.NewBytes(nil)
	}
	ciphertext := getBytesArg(args, 0)
	key := getBytesArg(args, 1)
	iv := getBytesArg(args, 2)
	if ciphertext == nil || key == nil || iv == nil || len(key) != 24 {
		return bytecode.NewBytes(nil)
	}

	block, err := des.NewTripleDESCipher(key)
	if err != nil {
		return bytecode.NewBytes(nil)
	}

	if len(ciphertext)%block.BlockSize() != 0 {
		return bytecode.NewBytes(nil)
	}

	plaintext := make([]byte, len(ciphertext))
	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(plaintext, ciphertext)

	// Remove PKCS7 padding
	if len(plaintext) == 0 {
		return bytecode.NewBytes(nil)
	}
	padding := int(plaintext[len(plaintext)-1])
	if padding > len(plaintext) || padding > block.BlockSize() {
		return bytecode.NewBytes(nil)
	}

	return bytecode.NewBytes(plaintext[:len(plaintext)-padding])
}

// ============================================================================
// RSA Functions
// ============================================================================

var (
	rsaMu         sync.Mutex
	rsaPrivateMap = make(map[int64]*rsa.PrivateKey)
	rsaPublicMap  = make(map[int64]*rsa.PublicKey)
	rsaNextID     int64 = 1
)

// nativeCryptoRsaGenerate 生成RSA密钥对
// native_crypto_rsa_generate(bits int) -> [privateHandle int, publicHandle int]
func nativeCryptoRsaGenerate(args []bytecode.Value) bytecode.Value {
	bits := int64(2048)
	if len(args) >= 1 {
		if b, ok := args[0].Data.(int64); ok {
			bits = b
		}
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, int(bits))
	if err != nil {
		return bytecode.NewBytes(nil)
	}

	rsaMu.Lock()
	privID := rsaNextID
	rsaNextID++
	pubID := rsaNextID
	rsaNextID++
	rsaPrivateMap[privID] = privateKey
	rsaPublicMap[pubID] = &privateKey.PublicKey
	rsaMu.Unlock()

	// 返回两个handle
	arr := bytecode.NewSuperArray()
	arr.Set(bytecode.NewInt(0), bytecode.NewInt(privID))
	arr.Set(bytecode.NewInt(1), bytecode.NewInt(pubID))
	return bytecode.Value{Type: bytecode.ValSuperArray, Data: arr}
}

// nativeCryptoRsaGetPublicKeyPem 获取公钥PEM
func nativeCryptoRsaGetPublicKeyPem(args []bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.NewString("")
	}
	handle, ok := args[0].Data.(int64)
	if !ok {
		return bytecode.NewString("")
	}

	rsaMu.Lock()
	pubKey, exists := rsaPublicMap[handle]
	rsaMu.Unlock()
	if !exists {
		return bytecode.NewString("")
	}

	pubASN1, err := x509.MarshalPKIXPublicKey(pubKey)
	if err != nil {
		return bytecode.NewString("")
	}

	pubPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubASN1,
	})

	return bytecode.NewString(string(pubPEM))
}

// nativeCryptoRsaGetPrivateKeyPem 获取私钥PEM
func nativeCryptoRsaGetPrivateKeyPem(args []bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.NewString("")
	}
	handle, ok := args[0].Data.(int64)
	if !ok {
		return bytecode.NewString("")
	}

	rsaMu.Lock()
	privKey, exists := rsaPrivateMap[handle]
	rsaMu.Unlock()
	if !exists {
		return bytecode.NewString("")
	}

	privPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privKey),
	})

	return bytecode.NewString(string(privPEM))
}

// nativeCryptoRsaLoadPublicKey 从PEM加载公钥
func nativeCryptoRsaLoadPublicKey(args []bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.NewInt(-1)
	}
	pemData, ok := args[0].Data.(string)
	if !ok {
		return bytecode.NewInt(-1)
	}

	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		return bytecode.NewInt(-1)
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		// 尝试PKCS1格式
		pub, err = x509.ParsePKCS1PublicKey(block.Bytes)
		if err != nil {
			return bytecode.NewInt(-1)
		}
	}

	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return bytecode.NewInt(-1)
	}

	rsaMu.Lock()
	id := rsaNextID
	rsaNextID++
	rsaPublicMap[id] = rsaPub
	rsaMu.Unlock()

	return bytecode.NewInt(id)
}

// nativeCryptoRsaLoadPrivateKey 从PEM加载私钥
func nativeCryptoRsaLoadPrivateKey(args []bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.NewInt(-1)
	}
	pemData, ok := args[0].Data.(string)
	if !ok {
		return bytecode.NewInt(-1)
	}

	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		return bytecode.NewInt(-1)
	}

	privKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		// 尝试PKCS8格式
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return bytecode.NewInt(-1)
		}
		var ok bool
		privKey, ok = key.(*rsa.PrivateKey)
		if !ok {
			return bytecode.NewInt(-1)
		}
	}

	rsaMu.Lock()
	id := rsaNextID
	rsaNextID++
	rsaPrivateMap[id] = privKey
	rsaMu.Unlock()

	return bytecode.NewInt(id)
}

// nativeCryptoRsaEncrypt RSA-OAEP加密
func nativeCryptoRsaEncrypt(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewBytes(nil)
	}
	plaintext := getBytes(args, 0)
	pubHandle, ok := args[1].Data.(int64)
	if !ok || plaintext == nil {
		return bytecode.NewBytes(nil)
	}

	rsaMu.Lock()
	pubKey, exists := rsaPublicMap[pubHandle]
	rsaMu.Unlock()
	if !exists {
		return bytecode.NewBytes(nil)
	}

	ciphertext, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, pubKey, plaintext, nil)
	if err != nil {
		return bytecode.NewBytes(nil)
	}

	return bytecode.NewBytes(ciphertext)
}

// nativeCryptoRsaDecrypt RSA-OAEP解密
func nativeCryptoRsaDecrypt(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewBytes(nil)
	}
	ciphertext := getBytesArg(args, 0)
	privHandle, ok := args[1].Data.(int64)
	if !ok || ciphertext == nil {
		return bytecode.NewBytes(nil)
	}

	rsaMu.Lock()
	privKey, exists := rsaPrivateMap[privHandle]
	rsaMu.Unlock()
	if !exists {
		return bytecode.NewBytes(nil)
	}

	plaintext, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, privKey, ciphertext, nil)
	if err != nil {
		return bytecode.NewBytes(nil)
	}

	return bytecode.NewBytes(plaintext)
}

// nativeCryptoRsaSign RSA签名 (PSS)
func nativeCryptoRsaSign(args []bytecode.Value) bytecode.Value {
	if len(args) < 3 {
		return bytecode.NewBytes(nil)
	}
	message := getBytes(args, 0)
	privHandle, ok := args[1].Data.(int64)
	if !ok || message == nil {
		return bytecode.NewBytes(nil)
	}
	hashAlgo, ok := args[2].Data.(string)
	if !ok {
		hashAlgo = "sha256"
	}

	rsaMu.Lock()
	privKey, exists := rsaPrivateMap[privHandle]
	rsaMu.Unlock()
	if !exists {
		return bytecode.NewBytes(nil)
	}

	var h crypto.Hash
	var hashed []byte
	switch hashAlgo {
	case "sha1":
		h = crypto.SHA1
		sum := sha1.Sum(message)
		hashed = sum[:]
	case "sha256":
		h = crypto.SHA256
		sum := sha256.Sum256(message)
		hashed = sum[:]
	case "sha384":
		h = crypto.SHA384
		sum := sha512.Sum384(message)
		hashed = sum[:]
	case "sha512":
		h = crypto.SHA512
		sum := sha512.Sum512(message)
		hashed = sum[:]
	default:
		return bytecode.NewBytes(nil)
	}

	signature, err := rsa.SignPSS(rand.Reader, privKey, h, hashed, nil)
	if err != nil {
		return bytecode.NewBytes(nil)
	}

	return bytecode.NewBytes(signature)
}

// nativeCryptoRsaVerify RSA验签 (PSS)
func nativeCryptoRsaVerify(args []bytecode.Value) bytecode.Value {
	if len(args) < 4 {
		return bytecode.NewBool(false)
	}
	message := getBytes(args, 0)
	signature := getBytesArg(args, 1)
	pubHandle, ok := args[2].Data.(int64)
	if !ok || message == nil || signature == nil {
		return bytecode.NewBool(false)
	}
	hashAlgo, ok := args[3].Data.(string)
	if !ok {
		hashAlgo = "sha256"
	}

	rsaMu.Lock()
	pubKey, exists := rsaPublicMap[pubHandle]
	rsaMu.Unlock()
	if !exists {
		return bytecode.NewBool(false)
	}

	var h crypto.Hash
	var hashed []byte
	switch hashAlgo {
	case "sha1":
		h = crypto.SHA1
		sum := sha1.Sum(message)
		hashed = sum[:]
	case "sha256":
		h = crypto.SHA256
		sum := sha256.Sum256(message)
		hashed = sum[:]
	case "sha384":
		h = crypto.SHA384
		sum := sha512.Sum384(message)
		hashed = sum[:]
	case "sha512":
		h = crypto.SHA512
		sum := sha512.Sum512(message)
		hashed = sum[:]
	default:
		return bytecode.NewBool(false)
	}

	err := rsa.VerifyPSS(pubKey, h, hashed, signature, nil)
	return bytecode.NewBool(err == nil)
}

// nativeCryptoRsaSignPkcs1 RSA PKCS1v15签名
func nativeCryptoRsaSignPkcs1(args []bytecode.Value) bytecode.Value {
	if len(args) < 3 {
		return bytecode.NewBytes(nil)
	}
	message := getBytes(args, 0)
	privHandle, ok := args[1].Data.(int64)
	if !ok || message == nil {
		return bytecode.NewBytes(nil)
	}
	hashAlgo, ok := args[2].Data.(string)
	if !ok {
		hashAlgo = "sha256"
	}

	rsaMu.Lock()
	privKey, exists := rsaPrivateMap[privHandle]
	rsaMu.Unlock()
	if !exists {
		return bytecode.NewBytes(nil)
	}

	var h crypto.Hash
	var hashed []byte
	switch hashAlgo {
	case "sha1":
		h = crypto.SHA1
		sum := sha1.Sum(message)
		hashed = sum[:]
	case "sha256":
		h = crypto.SHA256
		sum := sha256.Sum256(message)
		hashed = sum[:]
	case "sha384":
		h = crypto.SHA384
		sum := sha512.Sum384(message)
		hashed = sum[:]
	case "sha512":
		h = crypto.SHA512
		sum := sha512.Sum512(message)
		hashed = sum[:]
	default:
		return bytecode.NewBytes(nil)
	}

	signature, err := rsa.SignPKCS1v15(rand.Reader, privKey, h, hashed)
	if err != nil {
		return bytecode.NewBytes(nil)
	}

	return bytecode.NewBytes(signature)
}

// nativeCryptoRsaVerifyPkcs1 RSA PKCS1v15验签
func nativeCryptoRsaVerifyPkcs1(args []bytecode.Value) bytecode.Value {
	if len(args) < 4 {
		return bytecode.NewBool(false)
	}
	message := getBytes(args, 0)
	signature := getBytesArg(args, 1)
	pubHandle, ok := args[2].Data.(int64)
	if !ok || message == nil || signature == nil {
		return bytecode.NewBool(false)
	}
	hashAlgo, ok := args[3].Data.(string)
	if !ok {
		hashAlgo = "sha256"
	}

	rsaMu.Lock()
	pubKey, exists := rsaPublicMap[pubHandle]
	rsaMu.Unlock()
	if !exists {
		return bytecode.NewBool(false)
	}

	var h crypto.Hash
	var hashed []byte
	switch hashAlgo {
	case "sha1":
		h = crypto.SHA1
		sum := sha1.Sum(message)
		hashed = sum[:]
	case "sha256":
		h = crypto.SHA256
		sum := sha256.Sum256(message)
		hashed = sum[:]
	case "sha384":
		h = crypto.SHA384
		sum := sha512.Sum384(message)
		hashed = sum[:]
	case "sha512":
		h = crypto.SHA512
		sum := sha512.Sum512(message)
		hashed = sum[:]
	default:
		return bytecode.NewBool(false)
	}

	err := rsa.VerifyPKCS1v15(pubKey, h, hashed, signature)
	return bytecode.NewBool(err == nil)
}

// nativeCryptoRsaEncryptPkcs1 RSA PKCS1v15加密
func nativeCryptoRsaEncryptPkcs1(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewBytes(nil)
	}
	plaintext := getBytes(args, 0)
	pubHandle, ok := args[1].Data.(int64)
	if !ok || plaintext == nil {
		return bytecode.NewBytes(nil)
	}

	rsaMu.Lock()
	pubKey, exists := rsaPublicMap[pubHandle]
	rsaMu.Unlock()
	if !exists {
		return bytecode.NewBytes(nil)
	}

	ciphertext, err := rsa.EncryptPKCS1v15(rand.Reader, pubKey, plaintext)
	if err != nil {
		return bytecode.NewBytes(nil)
	}

	return bytecode.NewBytes(ciphertext)
}

// nativeCryptoRsaDecryptPkcs1 RSA PKCS1v15解密
func nativeCryptoRsaDecryptPkcs1(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewBytes(nil)
	}
	ciphertext := getBytesArg(args, 0)
	privHandle, ok := args[1].Data.(int64)
	if !ok || ciphertext == nil {
		return bytecode.NewBytes(nil)
	}

	rsaMu.Lock()
	privKey, exists := rsaPrivateMap[privHandle]
	rsaMu.Unlock()
	if !exists {
		return bytecode.NewBytes(nil)
	}

	plaintext, err := rsa.DecryptPKCS1v15(rand.Reader, privKey, ciphertext)
	if err != nil {
		return bytecode.NewBytes(nil)
	}

	return bytecode.NewBytes(plaintext)
}

// nativeCryptoRsaFree 释放RSA密钥
func nativeCryptoRsaFree(args []bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.NewBool(false)
	}
	handle, ok := args[0].Data.(int64)
	if !ok {
		return bytecode.NewBool(false)
	}

	rsaMu.Lock()
	_, privExists := rsaPrivateMap[handle]
	_, pubExists := rsaPublicMap[handle]
	if privExists {
		delete(rsaPrivateMap, handle)
	}
	if pubExists {
		delete(rsaPublicMap, handle)
	}
	rsaMu.Unlock()

	return bytecode.NewBool(privExists || pubExists)
}

// ============================================================================
// ECDSA Functions
// ============================================================================

var (
	ecdsaMu         sync.Mutex
	ecdsaPrivateMap = make(map[int64]*ecdsa.PrivateKey)
	ecdsaPublicMap  = make(map[int64]*ecdsa.PublicKey)
	ecdsaNextID     int64 = 1
)

// nativeCryptoEcdsaGenerate 生成ECDSA密钥对
func nativeCryptoEcdsaGenerate(args []bytecode.Value) bytecode.Value {
	curve := "P-256"
	if len(args) >= 1 {
		if c, ok := args[0].Data.(string); ok {
			curve = c
		}
	}

	var ecCurve elliptic.Curve
	switch curve {
	case "P-256":
		ecCurve = elliptic.P256()
	case "P-384":
		ecCurve = elliptic.P384()
	case "P-521":
		ecCurve = elliptic.P521()
	default:
		return bytecode.NewBytes(nil)
	}

	privateKey, err := ecdsa.GenerateKey(ecCurve, rand.Reader)
	if err != nil {
		return bytecode.NewBytes(nil)
	}

	ecdsaMu.Lock()
	privID := ecdsaNextID
	ecdsaNextID++
	pubID := ecdsaNextID
	ecdsaNextID++
	ecdsaPrivateMap[privID] = privateKey
	ecdsaPublicMap[pubID] = &privateKey.PublicKey
	ecdsaMu.Unlock()

	arr := bytecode.NewSuperArray()
	arr.Set(bytecode.NewInt(0), bytecode.NewInt(privID))
	arr.Set(bytecode.NewInt(1), bytecode.NewInt(pubID))
	return bytecode.Value{Type: bytecode.ValSuperArray, Data: arr}
}

// nativeCryptoEcdsaSign ECDSA签名
func nativeCryptoEcdsaSign(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewBytes(nil)
	}
	message := getBytes(args, 0)
	privHandle, ok := args[1].Data.(int64)
	if !ok || message == nil {
		return bytecode.NewBytes(nil)
	}

	ecdsaMu.Lock()
	privKey, exists := ecdsaPrivateMap[privHandle]
	ecdsaMu.Unlock()
	if !exists {
		return bytecode.NewBytes(nil)
	}

	hash := sha256.Sum256(message)
	r, s, err := ecdsa.Sign(rand.Reader, privKey, hash[:])
	if err != nil {
		return bytecode.NewBytes(nil)
	}

	// 编码为DER格式
	signature := append(r.Bytes(), s.Bytes()...)
	return bytecode.NewBytes(signature)
}

// nativeCryptoEcdsaVerify ECDSA验签
func nativeCryptoEcdsaVerify(args []bytecode.Value) bytecode.Value {
	if len(args) < 3 {
		return bytecode.NewBool(false)
	}
	message := getBytes(args, 0)
	signature := getBytesArg(args, 1)
	pubHandle, ok := args[2].Data.(int64)
	if !ok || message == nil || signature == nil {
		return bytecode.NewBool(false)
	}

	ecdsaMu.Lock()
	pubKey, exists := ecdsaPublicMap[pubHandle]
	ecdsaMu.Unlock()
	if !exists {
		return bytecode.NewBool(false)
	}

	hash := sha256.Sum256(message)

	// 解码签名
	keySize := (pubKey.Curve.Params().BitSize + 7) / 8
	if len(signature) != 2*keySize {
		return bytecode.NewBool(false)
	}

	r := new(big.Int).SetBytes(signature[:keySize])
	s := new(big.Int).SetBytes(signature[keySize:])

	return bytecode.NewBool(ecdsa.Verify(pubKey, hash[:], r, s))
}

// nativeCryptoEcdsaGetPublicKeyPem 获取公钥PEM
func nativeCryptoEcdsaGetPublicKeyPem(args []bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.NewString("")
	}
	handle, ok := args[0].Data.(int64)
	if !ok {
		return bytecode.NewString("")
	}

	ecdsaMu.Lock()
	pubKey, exists := ecdsaPublicMap[handle]
	ecdsaMu.Unlock()
	if !exists {
		return bytecode.NewString("")
	}

	pubASN1, err := x509.MarshalPKIXPublicKey(pubKey)
	if err != nil {
		return bytecode.NewString("")
	}

	pubPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubASN1,
	})

	return bytecode.NewString(string(pubPEM))
}

// nativeCryptoEcdsaGetPrivateKeyPem 获取私钥PEM
func nativeCryptoEcdsaGetPrivateKeyPem(args []bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.NewString("")
	}
	handle, ok := args[0].Data.(int64)
	if !ok {
		return bytecode.NewString("")
	}

	ecdsaMu.Lock()
	privKey, exists := ecdsaPrivateMap[handle]
	ecdsaMu.Unlock()
	if !exists {
		return bytecode.NewString("")
	}

	privASN1, err := x509.MarshalECPrivateKey(privKey)
	if err != nil {
		return bytecode.NewString("")
	}

	privPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: privASN1,
	})

	return bytecode.NewString(string(privPEM))
}

// nativeCryptoEcdsaLoadPublicKey 从PEM加载公钥
func nativeCryptoEcdsaLoadPublicKey(args []bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.NewInt(-1)
	}
	pemData, ok := args[0].Data.(string)
	if !ok {
		return bytecode.NewInt(-1)
	}

	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		return bytecode.NewInt(-1)
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return bytecode.NewInt(-1)
	}

	ecdsaPub, ok := pub.(*ecdsa.PublicKey)
	if !ok {
		return bytecode.NewInt(-1)
	}

	ecdsaMu.Lock()
	id := ecdsaNextID
	ecdsaNextID++
	ecdsaPublicMap[id] = ecdsaPub
	ecdsaMu.Unlock()

	return bytecode.NewInt(id)
}

// nativeCryptoEcdsaLoadPrivateKey 从PEM加载私钥
func nativeCryptoEcdsaLoadPrivateKey(args []bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.NewInt(-1)
	}
	pemData, ok := args[0].Data.(string)
	if !ok {
		return bytecode.NewInt(-1)
	}

	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		return bytecode.NewInt(-1)
	}

	privKey, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		// 尝试PKCS8格式
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return bytecode.NewInt(-1)
		}
		var ok bool
		privKey, ok = key.(*ecdsa.PrivateKey)
		if !ok {
			return bytecode.NewInt(-1)
		}
	}

	ecdsaMu.Lock()
	id := ecdsaNextID
	ecdsaNextID++
	ecdsaPrivateMap[id] = privKey
	ecdsaMu.Unlock()

	return bytecode.NewInt(id)
}

// nativeCryptoEcdsaFree 释放ECDSA密钥
func nativeCryptoEcdsaFree(args []bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.NewBool(false)
	}
	handle, ok := args[0].Data.(int64)
	if !ok {
		return bytecode.NewBool(false)
	}

	ecdsaMu.Lock()
	_, privExists := ecdsaPrivateMap[handle]
	_, pubExists := ecdsaPublicMap[handle]
	if privExists {
		delete(ecdsaPrivateMap, handle)
	}
	if pubExists {
		delete(ecdsaPublicMap, handle)
	}
	ecdsaMu.Unlock()

	return bytecode.NewBool(privExists || pubExists)
}

// ============================================================================
// Ed25519 Functions
// ============================================================================

var (
	ed25519Mu         sync.Mutex
	ed25519PrivateMap = make(map[int64]ed25519.PrivateKey)
	ed25519PublicMap  = make(map[int64]ed25519.PublicKey)
	ed25519NextID     int64 = 1
)

// nativeCryptoEd25519Generate 生成Ed25519密钥对
func nativeCryptoEd25519Generate(args []bytecode.Value) bytecode.Value {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return bytecode.NewBytes(nil)
	}

	ed25519Mu.Lock()
	privID := ed25519NextID
	ed25519NextID++
	pubID := ed25519NextID
	ed25519NextID++
	ed25519PrivateMap[privID] = privateKey
	ed25519PublicMap[pubID] = publicKey
	ed25519Mu.Unlock()

	arr := bytecode.NewSuperArray()
	arr.Set(bytecode.NewInt(0), bytecode.NewInt(privID))
	arr.Set(bytecode.NewInt(1), bytecode.NewInt(pubID))
	return bytecode.Value{Type: bytecode.ValSuperArray, Data: arr}
}

// nativeCryptoEd25519Sign Ed25519签名
func nativeCryptoEd25519Sign(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewBytes(nil)
	}
	message := getBytes(args, 0)
	privHandle, ok := args[1].Data.(int64)
	if !ok || message == nil {
		return bytecode.NewBytes(nil)
	}

	ed25519Mu.Lock()
	privKey, exists := ed25519PrivateMap[privHandle]
	ed25519Mu.Unlock()
	if !exists {
		return bytecode.NewBytes(nil)
	}

	signature := ed25519.Sign(privKey, message)
	return bytecode.NewBytes(signature)
}

// nativeCryptoEd25519Verify Ed25519验签
func nativeCryptoEd25519Verify(args []bytecode.Value) bytecode.Value {
	if len(args) < 3 {
		return bytecode.NewBool(false)
	}
	message := getBytes(args, 0)
	signature := getBytesArg(args, 1)
	pubHandle, ok := args[2].Data.(int64)
	if !ok || message == nil || signature == nil {
		return bytecode.NewBool(false)
	}

	ed25519Mu.Lock()
	pubKey, exists := ed25519PublicMap[pubHandle]
	ed25519Mu.Unlock()
	if !exists {
		return bytecode.NewBool(false)
	}

	return bytecode.NewBool(ed25519.Verify(pubKey, message, signature))
}

// nativeCryptoEd25519GetPublicKeyBytes 获取公钥字节
func nativeCryptoEd25519GetPublicKeyBytes(args []bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.NewBytes(nil)
	}
	handle, ok := args[0].Data.(int64)
	if !ok {
		return bytecode.NewBytes(nil)
	}

	ed25519Mu.Lock()
	pubKey, exists := ed25519PublicMap[handle]
	ed25519Mu.Unlock()
	if !exists {
		return bytecode.NewBytes(nil)
	}

	return bytecode.NewBytes(pubKey)
}

// nativeCryptoEd25519GetPrivateKeyBytes 获取私钥字节
func nativeCryptoEd25519GetPrivateKeyBytes(args []bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.NewBytes(nil)
	}
	handle, ok := args[0].Data.(int64)
	if !ok {
		return bytecode.NewBytes(nil)
	}

	ed25519Mu.Lock()
	privKey, exists := ed25519PrivateMap[handle]
	ed25519Mu.Unlock()
	if !exists {
		return bytecode.NewBytes(nil)
	}

	return bytecode.NewBytes(privKey)
}

// nativeCryptoEd25519LoadPublicKey 从字节加载公钥
func nativeCryptoEd25519LoadPublicKey(args []bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.NewInt(-1)
	}
	keyBytes := getBytesArg(args, 0)
	if keyBytes == nil || len(keyBytes) != ed25519.PublicKeySize {
		return bytecode.NewInt(-1)
	}

	ed25519Mu.Lock()
	id := ed25519NextID
	ed25519NextID++
	ed25519PublicMap[id] = ed25519.PublicKey(keyBytes)
	ed25519Mu.Unlock()

	return bytecode.NewInt(id)
}

// nativeCryptoEd25519LoadPrivateKey 从字节加载私钥
func nativeCryptoEd25519LoadPrivateKey(args []bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.NewInt(-1)
	}
	keyBytes := getBytesArg(args, 0)
	if keyBytes == nil || len(keyBytes) != ed25519.PrivateKeySize {
		return bytecode.NewInt(-1)
	}

	ed25519Mu.Lock()
	id := ed25519NextID
	ed25519NextID++
	ed25519PrivateMap[id] = ed25519.PrivateKey(keyBytes)
	ed25519Mu.Unlock()

	return bytecode.NewInt(id)
}

// nativeCryptoEd25519Free 释放Ed25519密钥
func nativeCryptoEd25519Free(args []bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.NewBool(false)
	}
	handle, ok := args[0].Data.(int64)
	if !ok {
		return bytecode.NewBool(false)
	}

	ed25519Mu.Lock()
	_, privExists := ed25519PrivateMap[handle]
	_, pubExists := ed25519PublicMap[handle]
	if privExists {
		delete(ed25519PrivateMap, handle)
	}
	if pubExists {
		delete(ed25519PublicMap, handle)
	}
	ed25519Mu.Unlock()

	return bytecode.NewBool(privExists || pubExists)
}

// ============================================================================
// Key Derivation Functions
// ============================================================================

// nativeCryptoPbkdf2 PBKDF2密钥派生
// native_crypto_pbkdf2(password string|byte[], salt byte[], iterations int, keyLen int, hash string) -> byte[]
func nativeCryptoPbkdf2(args []bytecode.Value) bytecode.Value {
	if len(args) < 5 {
		return bytecode.NewBytes(nil)
	}
	password := getBytes(args, 0)
	salt := getBytesArg(args, 1)
	iterations, ok := args[2].Data.(int64)
	if !ok || password == nil || salt == nil {
		return bytecode.NewBytes(nil)
	}
	keyLen, ok := args[3].Data.(int64)
	if !ok {
		return bytecode.NewBytes(nil)
	}
	hashAlgo, ok := args[4].Data.(string)
	if !ok {
		hashAlgo = "sha256"
	}

	var h func() hash.Hash
	switch hashAlgo {
	case "sha1":
		h = sha1.New
	case "sha256":
		h = sha256.New
	case "sha384":
		h = sha512.New384
	case "sha512":
		h = sha512.New
	default:
		return bytecode.NewBytes(nil)
	}

	key := pbkdf2.Key(password, salt, int(iterations), int(keyLen), h)
	return bytecode.NewBytes(key)
}

// nativeCryptoHkdf HKDF密钥派生
// native_crypto_hkdf(secret byte[], salt byte[], info byte[], keyLen int, hash string) -> byte[]
func nativeCryptoHkdf(args []bytecode.Value) bytecode.Value {
	if len(args) < 5 {
		return bytecode.NewBytes(nil)
	}
	secret := getBytesArg(args, 0)
	salt := getBytesArg(args, 1)
	info := getBytesArg(args, 2)
	keyLen, ok := args[3].Data.(int64)
	if !ok || secret == nil {
		return bytecode.NewBytes(nil)
	}
	hashAlgo, ok := args[4].Data.(string)
	if !ok {
		hashAlgo = "sha256"
	}

	var h func() hash.Hash
	switch hashAlgo {
	case "sha1":
		h = sha1.New
	case "sha256":
		h = sha256.New
	case "sha384":
		h = sha512.New384
	case "sha512":
		h = sha512.New
	default:
		return bytecode.NewBytes(nil)
	}

	reader := hkdf.New(h, secret, salt, info)
	key := make([]byte, keyLen)
	if _, err := io.ReadFull(reader, key); err != nil {
		return bytecode.NewBytes(nil)
	}

	return bytecode.NewBytes(key)
}

// nativeCryptoScrypt Scrypt密钥派生
// native_crypto_scrypt(password byte[], salt byte[], n int, r int, p int, keyLen int) -> byte[]
func nativeCryptoScrypt(args []bytecode.Value) bytecode.Value {
	if len(args) < 6 {
		return bytecode.NewBytes(nil)
	}
	password := getBytes(args, 0)
	salt := getBytesArg(args, 1)
	n, ok := args[2].Data.(int64)
	if !ok || password == nil || salt == nil {
		return bytecode.NewBytes(nil)
	}
	r, ok := args[3].Data.(int64)
	if !ok {
		return bytecode.NewBytes(nil)
	}
	p, ok := args[4].Data.(int64)
	if !ok {
		return bytecode.NewBytes(nil)
	}
	keyLen, ok := args[5].Data.(int64)
	if !ok {
		return bytecode.NewBytes(nil)
	}

	key, err := scrypt.Key(password, salt, int(n), int(r), int(p), int(keyLen))
	if err != nil {
		return bytecode.NewBytes(nil)
	}

	return bytecode.NewBytes(key)
}

// nativeCryptoArgon2id Argon2id密钥派生
// native_crypto_argon2id(password byte[], salt byte[], time int, memory int, threads int, keyLen int) -> byte[]
func nativeCryptoArgon2id(args []bytecode.Value) bytecode.Value {
	if len(args) < 6 {
		return bytecode.NewBytes(nil)
	}
	password := getBytes(args, 0)
	salt := getBytesArg(args, 1)
	time, ok := args[2].Data.(int64)
	if !ok || password == nil || salt == nil {
		return bytecode.NewBytes(nil)
	}
	memory, ok := args[3].Data.(int64)
	if !ok {
		return bytecode.NewBytes(nil)
	}
	threads, ok := args[4].Data.(int64)
	if !ok {
		return bytecode.NewBytes(nil)
	}
	keyLen, ok := args[5].Data.(int64)
	if !ok {
		return bytecode.NewBytes(nil)
	}

	key := argon2.IDKey(password, salt, uint32(time), uint32(memory), uint8(threads), uint32(keyLen))
	return bytecode.NewBytes(key)
}

// nativeCryptoArgon2i Argon2i密钥派生
func nativeCryptoArgon2i(args []bytecode.Value) bytecode.Value {
	if len(args) < 6 {
		return bytecode.NewBytes(nil)
	}
	password := getBytes(args, 0)
	salt := getBytesArg(args, 1)
	time, ok := args[2].Data.(int64)
	if !ok || password == nil || salt == nil {
		return bytecode.NewBytes(nil)
	}
	memory, ok := args[3].Data.(int64)
	if !ok {
		return bytecode.NewBytes(nil)
	}
	threads, ok := args[4].Data.(int64)
	if !ok {
		return bytecode.NewBytes(nil)
	}
	keyLen, ok := args[5].Data.(int64)
	if !ok {
		return bytecode.NewBytes(nil)
	}

	key := argon2.Key(password, salt, uint32(time), uint32(memory), uint8(threads), uint32(keyLen))
	return bytecode.NewBytes(key)
}

// ============================================================================
// Random Functions
// ============================================================================

// nativeCryptoRandomBytes 生成加密安全随机字节
func nativeCryptoRandomBytes(args []bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.NewBytes(nil)
	}
	n, ok := args[0].Data.(int64)
	if !ok || n <= 0 {
		return bytecode.NewBytes(nil)
	}

	bytes := make([]byte, n)
	if _, err := rand.Read(bytes); err != nil {
		return bytecode.NewBytes(nil)
	}

	return bytecode.NewBytes(bytes)
}

// nativeCryptoRandomInt 生成加密安全随机整数 [min, max)
func nativeCryptoRandomInt(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewInt(0)
	}
	min, ok := args[0].Data.(int64)
	if !ok {
		return bytecode.NewInt(0)
	}
	max, ok := args[1].Data.(int64)
	if !ok || max <= min {
		return bytecode.NewInt(0)
	}

	// 生成 [0, max-min) 的随机数，然后加上 min
	rangeVal := max - min
	n, err := rand.Int(rand.Reader, big.NewInt(rangeVal))
	if err != nil {
		return bytecode.NewInt(0)
	}

	return bytecode.NewInt(n.Int64() + min)
}

// nativeCryptoRandomHex 生成随机十六进制字符串
func nativeCryptoRandomHex(args []bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.NewString("")
	}
	n, ok := args[0].Data.(int64)
	if !ok || n <= 0 {
		return bytecode.NewString("")
	}

	bytes := make([]byte, n)
	if _, err := rand.Read(bytes); err != nil {
		return bytecode.NewString("")
	}

	return bytecode.NewString(hex.EncodeToString(bytes))
}

// nativeCryptoRandomUuid 生成UUID v4
func nativeCryptoRandomUuid(args []bytecode.Value) bytecode.Value {
	uuid := make([]byte, 16)
	if _, err := rand.Read(uuid); err != nil {
		return bytecode.NewString("")
	}

	// 设置版本位 (版本4)
	uuid[6] = (uuid[6] & 0x0f) | 0x40
	// 设置变体位
	uuid[8] = (uuid[8] & 0x3f) | 0x80

	return bytecode.NewString(fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16]))
}

// ============================================================================
// Hex Functions
// ============================================================================

// nativeCryptoHexEncode 十六进制编码
func nativeCryptoHexEncode(args []bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.NewString("")
	}
	data := getBytesArg(args, 0)
	if data == nil {
		return bytecode.NewString("")
	}
	return bytecode.NewString(hex.EncodeToString(data))
}

// nativeCryptoHexDecode 十六进制解码
func nativeCryptoHexDecode(args []bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.NewBytes(nil)
	}
	hexStr, ok := args[0].Data.(string)
	if !ok {
		return bytecode.NewBytes(nil)
	}
	data, err := hex.DecodeString(hexStr)
	if err != nil {
		return bytecode.NewBytes(nil)
	}
	return bytecode.NewBytes(data)
}

// nativeCryptoHexIsValid 检查是否有效十六进制
func nativeCryptoHexIsValid(args []bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.NewBool(false)
	}
	hexStr, ok := args[0].Data.(string)
	if !ok {
		return bytecode.NewBool(false)
	}
	_, err := hex.DecodeString(hexStr)
	return bytecode.NewBool(err == nil)
}

// ============================================================================
// Helper Functions
// ============================================================================

// getBytes 从参数获取字节数据（支持string和byte[]）
func getBytes(args []bytecode.Value, index int) []byte {
	if len(args) <= index {
		return nil
	}
	v := args[index]
	switch v.Type {
	case bytecode.ValString:
		return []byte(v.Data.(string))
	case bytecode.ValBytes:
		return v.Data.([]byte)
	default:
		return nil
	}
}

// getBytesArg 从参数获取字节数组（仅byte[]）
func getBytesArg(args []bytecode.Value, index int) []byte {
	if len(args) <= index {
		return nil
	}
	v := args[index]
	if v.Type == bytecode.ValBytes {
		return v.Data.([]byte)
	}
	if v.Type == bytecode.ValString {
		return []byte(v.Data.(string))
	}
	return nil
}

