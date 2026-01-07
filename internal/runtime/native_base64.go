package runtime

import (
	"encoding/base64"

	"github.com/tangzhangming/nova/internal/bytecode"
)

// ============================================================================
// Native Base64 函数 (仅供标准库使用)
// ============================================================================

// nativeBase64Encode 标准 Base64 编码（带填充）
func nativeBase64Encode(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewString("")
	}
	data := []byte(args[0].AsString())
	encoded := base64.StdEncoding.EncodeToString(data)
	return bytecode.NewString(encoded)
}

// nativeBase64Decode 标准 Base64 解码
func nativeBase64Decode(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewString("")
	}
	encoded := args[0].AsString()
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return bytecode.NewException("FormatException", "invalid base64 string: "+err.Error(), 0)
	}
	return bytecode.NewString(string(decoded))
}

// nativeBase64EncodeURLSafe URL 安全 Base64 编码（带填充）
func nativeBase64EncodeURLSafe(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewString("")
	}
	data := []byte(args[0].AsString())
	encoded := base64.URLEncoding.EncodeToString(data)
	return bytecode.NewString(encoded)
}

// nativeBase64DecodeURLSafe URL 安全 Base64 解码
func nativeBase64DecodeURLSafe(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewString("")
	}
	encoded := args[0].AsString()
	decoded, err := base64.URLEncoding.DecodeString(encoded)
	if err != nil {
		return bytecode.NewException("FormatException", "invalid url-safe base64 string: "+err.Error(), 0)
	}
	return bytecode.NewString(string(decoded))
}

// nativeBase64EncodeRaw 标准 Base64 编码（无填充）
func nativeBase64EncodeRaw(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewString("")
	}
	data := []byte(args[0].AsString())
	encoded := base64.RawStdEncoding.EncodeToString(data)
	return bytecode.NewString(encoded)
}

// nativeBase64DecodeRaw 标准 Base64 解码（无填充）
func nativeBase64DecodeRaw(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewString("")
	}
	encoded := args[0].AsString()
	decoded, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil {
		return bytecode.NewException("FormatException", "invalid raw base64 string: "+err.Error(), 0)
	}
	return bytecode.NewString(string(decoded))
}

// nativeBase64EncodeRawURLSafe URL 安全 Base64 编码（无填充）
func nativeBase64EncodeRawURLSafe(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewString("")
	}
	data := []byte(args[0].AsString())
	encoded := base64.RawURLEncoding.EncodeToString(data)
	return bytecode.NewString(encoded)
}

// nativeBase64DecodeRawURLSafe URL 安全 Base64 解码（无填充）
func nativeBase64DecodeRawURLSafe(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewString("")
	}
	encoded := args[0].AsString()
	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return bytecode.NewException("FormatException", "invalid raw url-safe base64 string: "+err.Error(), 0)
	}
	return bytecode.NewString(string(decoded))
}

// nativeBase64DecodeStrict 严格模式 Base64 解码
// 严格模式要求输入必须严格符合 Base64 规范（不允许额外的空白字符等）
func nativeBase64DecodeStrict(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewString("")
	}
	encoded := args[0].AsString()
	decoded, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil {
		return bytecode.NewException("FormatException", "invalid base64 string (strict mode): "+err.Error(), 0)
	}
	return bytecode.NewString(string(decoded))
}

// nativeBase64DecodeStrictURLSafe URL 安全严格模式 Base64 解码
func nativeBase64DecodeStrictURLSafe(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewString("")
	}
	encoded := args[0].AsString()
	decoded, err := base64.URLEncoding.Strict().DecodeString(encoded)
	if err != nil {
		return bytecode.NewException("FormatException", "invalid url-safe base64 string (strict mode): "+err.Error(), 0)
	}
	return bytecode.NewString(string(decoded))
}

// nativeBase64DecodeStrictRaw 严格模式 Base64 解码（无填充）
func nativeBase64DecodeStrictRaw(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewString("")
	}
	encoded := args[0].AsString()
	decoded, err := base64.RawStdEncoding.Strict().DecodeString(encoded)
	if err != nil {
		return bytecode.NewException("FormatException", "invalid raw base64 string (strict mode): "+err.Error(), 0)
	}
	return bytecode.NewString(string(decoded))
}

// nativeBase64DecodeStrictRawURLSafe URL 安全严格模式 Base64 解码（无填充）
func nativeBase64DecodeStrictRawURLSafe(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewString("")
	}
	encoded := args[0].AsString()
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(encoded)
	if err != nil {
		return bytecode.NewException("FormatException", "invalid raw url-safe base64 string (strict mode): "+err.Error(), 0)
	}
	return bytecode.NewString(string(decoded))
}

// nativeBase64EncodedLen 计算编码后的长度
// 公式: (n + 2) / 3 * 4
func nativeBase64EncodedLen(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.ZeroValue
	}
	inputLen := int(args[0].AsInt())
	encodedLen := base64.StdEncoding.EncodedLen(inputLen)
	return bytecode.NewInt(int64(encodedLen))
}

// nativeBase64DecodedLen 计算解码后的长度（最大可能长度）
// 注意：实际解码长度可能更小（因为填充字符）
func nativeBase64DecodedLen(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.ZeroValue
	}
	encodedLen := int(args[0].AsInt())
	decodedLen := base64.StdEncoding.DecodedLen(encodedLen)
	return bytecode.NewInt(int64(decodedLen))
}

// nativeBase64EncodedLenRaw 计算无填充编码后的长度
func nativeBase64EncodedLenRaw(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.ZeroValue
	}
	inputLen := int(args[0].AsInt())
	encodedLen := base64.RawStdEncoding.EncodedLen(inputLen)
	return bytecode.NewInt(int64(encodedLen))
}

// nativeBase64DecodedLenRaw 计算无填充解码后的长度
func nativeBase64DecodedLenRaw(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.ZeroValue
	}
	encodedLen := int(args[0].AsInt())
	decodedLen := base64.RawStdEncoding.DecodedLen(encodedLen)
	return bytecode.NewInt(int64(decodedLen))
}

// nativeBase64IsValid 检查字符串是否是有效的 Base64 编码
func nativeBase64IsValid(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	encoded := args[0].AsString()
	_, err := base64.StdEncoding.DecodeString(encoded)
	return bytecode.NewBool(err == nil)
}

// nativeBase64IsValidURLSafe 检查字符串是否是有效的 URL 安全 Base64 编码
func nativeBase64IsValidURLSafe(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	encoded := args[0].AsString()
	_, err := base64.URLEncoding.DecodeString(encoded)
	return bytecode.NewBool(err == nil)
}

// nativeBase64IsValidRaw 检查字符串是否是有效的无填充 Base64 编码
func nativeBase64IsValidRaw(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	encoded := args[0].AsString()
	_, err := base64.RawStdEncoding.DecodeString(encoded)
	return bytecode.NewBool(err == nil)
}

// nativeBase64IsValidRawURLSafe 检查字符串是否是有效的无填充 URL 安全 Base64 编码
func nativeBase64IsValidRawURLSafe(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	encoded := args[0].AsString()
	_, err := base64.RawURLEncoding.DecodeString(encoded)
	return bytecode.NewBool(err == nil)
}







