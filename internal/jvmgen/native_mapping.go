package jvmgen

// NativeMapping 定义 Sola native 函数到 JVM 的映射
type NativeMapping struct {
	// JavaClass 目标 Java 类（内部名称格式，如 java/nio/file/Files）
	JavaClass string
	// JavaMethod 目标 Java 方法名
	JavaMethod string
	// Descriptor 方法描述符
	Descriptor string
	// IsStatic 是否是静态方法
	IsStatic bool
	// NeedsWrapper 是否需要包装代码（如类型转换）
	NeedsWrapper bool
	// WrapperGen 自定义包装代码生成函数
	WrapperGen func(g *Generator) error
}

// NativeMappings Sola native 函数到 Java API 的映射表
var NativeMappings = map[string]*NativeMapping{
	// ============================================================
	// 文件操作 - 映射到 java.nio.file.Files
	// ============================================================
	"native_file_read": {
		// Files.readString(Path.of(path)) 需要包装
		NeedsWrapper: true,
		WrapperGen:   genFileRead,
	},
	"native_file_write": {
		NeedsWrapper: true,
		WrapperGen:   genFileWrite,
	},
	"native_file_exists": {
		NeedsWrapper: true,
		WrapperGen:   genFileExists,
	},
	"native_file_delete": {
		NeedsWrapper: true,
		WrapperGen:   genFileDelete,
	},

	// ============================================================
	// 时间操作 - 映射到 java.lang.System
	// ============================================================
	"native_time_now": {
		JavaClass:  "java/lang/System",
		JavaMethod: "currentTimeMillis",
		Descriptor: "()J",
		IsStatic:   true,
	},
	"native_time_nano": {
		JavaClass:  "java/lang/System",
		JavaMethod: "nanoTime",
		Descriptor: "()J",
		IsStatic:   true,
	},

	// ============================================================
	// 字符串操作 - 映射到 java.lang.String
	// ============================================================
	"native_str_length": {
		JavaClass:  "java/lang/String",
		JavaMethod: "length",
		Descriptor: "()I",
		IsStatic:   false,
	},
	"native_str_upper": {
		JavaClass:  "java/lang/String",
		JavaMethod: "toUpperCase",
		Descriptor: "()Ljava/lang/String;",
		IsStatic:   false,
	},
	"native_str_lower": {
		JavaClass:  "java/lang/String",
		JavaMethod: "toLowerCase",
		Descriptor: "()Ljava/lang/String;",
		IsStatic:   false,
	},
	"native_str_trim": {
		JavaClass:  "java/lang/String",
		JavaMethod: "trim",
		Descriptor: "()Ljava/lang/String;",
		IsStatic:   false,
	},
	"native_str_contains": {
		JavaClass:  "java/lang/String",
		JavaMethod: "contains",
		Descriptor: "(Ljava/lang/CharSequence;)Z",
		IsStatic:   false,
	},
	"native_str_starts_with": {
		JavaClass:  "java/lang/String",
		JavaMethod: "startsWith",
		Descriptor: "(Ljava/lang/String;)Z",
		IsStatic:   false,
	},
	"native_str_ends_with": {
		JavaClass:  "java/lang/String",
		JavaMethod: "endsWith",
		Descriptor: "(Ljava/lang/String;)Z",
		IsStatic:   false,
	},
	"native_str_replace": {
		JavaClass:  "java/lang/String",
		JavaMethod: "replace",
		Descriptor: "(Ljava/lang/CharSequence;Ljava/lang/CharSequence;)Ljava/lang/String;",
		IsStatic:   false,
	},
	"native_str_split": {
		JavaClass:  "java/lang/String",
		JavaMethod: "split",
		Descriptor: "(Ljava/lang/String;)[Ljava/lang/String;",
		IsStatic:   false,
	},

	// ============================================================
	// 数学操作 - 映射到 java.lang.Math
	// ============================================================
	"native_math_abs": {
		JavaClass:  "java/lang/Math",
		JavaMethod: "abs",
		Descriptor: "(D)D",
		IsStatic:   true,
	},
	"native_math_floor": {
		JavaClass:  "java/lang/Math",
		JavaMethod: "floor",
		Descriptor: "(D)D",
		IsStatic:   true,
	},
	"native_math_ceil": {
		JavaClass:  "java/lang/Math",
		JavaMethod: "ceil",
		Descriptor: "(D)D",
		IsStatic:   true,
	},
	"native_math_round": {
		JavaClass:  "java/lang/Math",
		JavaMethod: "round",
		Descriptor: "(D)J",
		IsStatic:   true,
	},
	"native_math_sqrt": {
		JavaClass:  "java/lang/Math",
		JavaMethod: "sqrt",
		Descriptor: "(D)D",
		IsStatic:   true,
	},
	"native_math_pow": {
		JavaClass:  "java/lang/Math",
		JavaMethod: "pow",
		Descriptor: "(DD)D",
		IsStatic:   true,
	},
	"native_math_sin": {
		JavaClass:  "java/lang/Math",
		JavaMethod: "sin",
		Descriptor: "(D)D",
		IsStatic:   true,
	},
	"native_math_cos": {
		JavaClass:  "java/lang/Math",
		JavaMethod: "cos",
		Descriptor: "(D)D",
		IsStatic:   true,
	},
	"native_math_tan": {
		JavaClass:  "java/lang/Math",
		JavaMethod: "tan",
		Descriptor: "(D)D",
		IsStatic:   true,
	},
	"native_math_random": {
		JavaClass:  "java/lang/Math",
		JavaMethod: "random",
		Descriptor: "()D",
		IsStatic:   true,
	},
	"native_math_min": {
		JavaClass:  "java/lang/Math",
		JavaMethod: "min",
		Descriptor: "(DD)D",
		IsStatic:   true,
	},
	"native_math_max": {
		JavaClass:  "java/lang/Math",
		JavaMethod: "max",
		Descriptor: "(DD)D",
		IsStatic:   true,
	},

	// ============================================================
	// Base64 - 映射到 java.util.Base64
	// ============================================================
	"native_base64_encode": {
		NeedsWrapper: true,
		WrapperGen:   genBase64Encode,
	},
	"native_base64_decode": {
		NeedsWrapper: true,
		WrapperGen:   genBase64Decode,
	},

	// ============================================================
	// JSON - 需要运行时库或第三方库
	// ============================================================
	// native_json_encode, native_json_decode
	// 建议: 使用 Gson 或 Jackson，需要运行时依赖

	// ============================================================
	// 正则表达式 - 映射到 java.util.regex
	// ============================================================
	"native_regex_match": {
		NeedsWrapper: true,
		WrapperGen:   genRegexMatch,
	},

	// ============================================================
	// 加密 - 映射到 javax.crypto / java.security
	// ============================================================
	// native_hash_md5, native_hash_sha256 等
	// 需要包装 MessageDigest

	// ============================================================
	// 控制台 I/O
	// ============================================================
	"native_print": {
		JavaClass:  "java/io/PrintStream",
		JavaMethod: "print",
		Descriptor: "(Ljava/lang/String;)V",
		IsStatic:   false, // 需要先 getstatic System.out
	},
	"native_println": {
		JavaClass:  "java/io/PrintStream",
		JavaMethod: "println",
		Descriptor: "(Ljava/lang/String;)V",
		IsStatic:   false,
	},
}

// ============================================================
// 包装函数实现
// ============================================================

// genFileRead 生成 Files.readString(Path.of(path)) 的字节码
func genFileRead(g *Generator) error {
	// 栈: [path:String]
	// 目标: Files.readString(Path.of(path))

	// 1. 调用 Path.of(path)
	//    invokestatic java/nio/file/Path.of:(Ljava/lang/String;[Ljava/lang/String;)Ljava/nio/file/Path;

	// 2. 调用 Files.readString(path)
	//    invokestatic java/nio/file/Files.readString:(Ljava/nio/file/Path;)Ljava/lang/String;

	// TODO: 实现具体字节码生成
	return nil
}

// genFileWrite 生成 Files.writeString 的字节码
func genFileWrite(g *Generator) error {
	// TODO: 实现
	return nil
}

// genFileExists 生成 Files.exists 的字节码
func genFileExists(g *Generator) error {
	// TODO: 实现
	return nil
}

// genFileDelete 生成 Files.delete 的字节码
func genFileDelete(g *Generator) error {
	// TODO: 实现
	return nil
}

// genBase64Encode 生成 Base64.getEncoder().encodeToString() 的字节码
func genBase64Encode(g *Generator) error {
	// TODO: 实现
	return nil
}

// genBase64Decode 生成 Base64.getDecoder().decode() 的字节码
func genBase64Decode(g *Generator) error {
	// TODO: 实现
	return nil
}

// genRegexMatch 生成 Pattern.matches() 的字节码
func genRegexMatch(g *Generator) error {
	// TODO: 实现
	return nil
}

// GetNativeMapping 获取 native 函数的 JVM 映射
func GetNativeMapping(name string) *NativeMapping {
	return NativeMappings[name]
}

// HasNativeMapping 检查是否有 native 函数的 JVM 映射
func HasNativeMapping(name string) bool {
	_, ok := NativeMappings[name]
	return ok
}
