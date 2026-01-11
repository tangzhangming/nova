package compiler

import (
	"fmt"
	"strings"
	"sync"

	"github.com/tangzhangming/nova/internal/ast"
)

// extractBaseTypeName 从泛型类型中提取基类名
// ConnectionPool<MysqlConnection> -> ConnectionPool
func extractBaseTypeName(typeName string) string {
	if idx := strings.Index(typeName, "<"); idx != -1 {
		return typeName[:idx]
	}
	return typeName
}

// FunctionSignature 函数签名
type FunctionSignature struct {
	Name       string   // 函数名
	TypeParams []string // 泛型类型参数 <T, K>
	ParamNames []string // 参数名称列表（用于命名参数）
	ParamTypes []string // 参数类型列表
	ReturnType string   // 返回类型 (可以是 "int", "string", "(int, string)" 等)
	MinArity   int      // 最小参数数量（考虑默认参数）
	IsVariadic bool     // 是否是可变参数
}

// MethodSignature 方法签名
type MethodSignature struct {
	ClassName  string   // 类名
	MethodName string   // 方法名
	TypeParams []string // 泛型类型参数 <T, K>
	ParamNames []string // 参数名称列表（用于命名参数）
	ParamTypes []string // 参数类型列表
	ReturnType string   // 返回类型
	MinArity   int      // 最小参数数量
	IsStatic   bool     // 是否是静态方法
}

// PropertySignature 属性签名
type PropertySignature struct {
	ClassName string // 类名
	PropName  string // 属性名
	Type      string // 属性类型
	IsStatic  bool   // 是否是静态属性
}

// TypeParamInfo 类型参数信息
type TypeParamInfo struct {
	Name            string   // 类型参数名 (T, K, V 等)
	ExtendsType     string   // extends 约束类型
	ImplementsTypes []string // implements 约束接口列表
}

// NewTypeInfo 新类型信息
// 新类型是与基础类型不兼容的独立类型，需要显式转换
type NewTypeInfo struct {
	Name     string // 新类型名
	BaseType string // 基础类型
	Distinct bool   // 是否是独立类型（不能隐式转换），总是 true
}

// ClassSignature 类签名 (用于泛型类)
type ClassSignature struct {
	Name       string           // 类名
	TypeParams []*TypeParamInfo // 泛型类型参数
}

// InterfaceSignature 接口签名 (用于泛型接口)
type InterfaceSignature struct {
	Name       string           // 接口名
	TypeParams []*TypeParamInfo // 泛型类型参数
}

// SymbolTable 符号表
type SymbolTable struct {
	Functions       map[string]*FunctionSignature            // 全局函数: 函数名 -> 签名
	ClassMethods    map[string]map[string][]*MethodSignature // 类方法: 类名 -> 方法名 -> 签名列表（支持重载）
	ClassProperties map[string]map[string]*PropertySignature // 类属性: 类名 -> 属性名 -> 签名
	GlobalVars      map[string]string                        // 全局变量类型
	ClassParents    map[string]string                        // 类继承关系: 子类名 -> 父类名
	ClassSignatures map[string]*ClassSignature               // 泛型类签名: 类名 -> 签名
	InterfaceSigs   map[string]*InterfaceSignature           // 泛型接口签名: 接口名 -> 签名
	TypeAliases     map[string]string                        // 类型别名: 别名 -> 目标类型
	NewTypes        map[string]*NewTypeInfo                  // 新类型: 类型名 -> 信息
	EnumValues      map[string][]string                      // 枚举值: 枚举名 -> 枚举值列表
	ClassInterfaces map[string][]string                      // 类实现的接口: 类名 -> 接口列表
}

// 全局共享的内置符号表（只初始化一次，避免内存暴涨）
var globalBuiltinSymbols *SymbolTable
var globalBuiltinOnce sync.Once

// getGlobalBuiltinSymbols 获取全局共享的内置符号表
func getGlobalBuiltinSymbols() *SymbolTable {
	globalBuiltinOnce.Do(func() {
		globalBuiltinSymbols = &SymbolTable{
			Functions:       make(map[string]*FunctionSignature),
			ClassMethods:    make(map[string]map[string][]*MethodSignature),
			ClassProperties: make(map[string]map[string]*PropertySignature),
			GlobalVars:      make(map[string]string),
			ClassParents:    make(map[string]string),
			ClassSignatures: make(map[string]*ClassSignature),
			InterfaceSigs:   make(map[string]*InterfaceSignature),
			TypeAliases:     make(map[string]string),
			NewTypes:        make(map[string]*NewTypeInfo),
			EnumValues:      make(map[string][]string),
			ClassInterfaces: make(map[string][]string),
		}
		globalBuiltinSymbols.registerBuiltinFunctions()
		globalBuiltinSymbols.registerBuiltinTypeMethods()
	})
	return globalBuiltinSymbols
}

// NewSymbolTable 创建符号表（不再重复注册231个内置符号）
func NewSymbolTable() *SymbolTable {
	return &SymbolTable{
		Functions:       make(map[string]*FunctionSignature),
		ClassMethods:    make(map[string]map[string][]*MethodSignature),
		ClassProperties: make(map[string]map[string]*PropertySignature),
		GlobalVars:      make(map[string]string),
		ClassParents:    make(map[string]string),
		ClassSignatures: make(map[string]*ClassSignature),
		InterfaceSigs:   make(map[string]*InterfaceSignature),
		TypeAliases:     make(map[string]string),
		NewTypes:        make(map[string]*NewTypeInfo),
		EnumValues:      make(map[string][]string),
		ClassInterfaces: make(map[string][]string),
	}
}

// registerBuiltinFunctions 注册内置函数签名
func (st *SymbolTable) registerBuiltinFunctions() {
	// 核心内置函数
	st.Functions["print"] = &FunctionSignature{Name: "print", ParamTypes: []string{"dynamic"}, ReturnType: "void", IsVariadic: true}
	st.Functions["echo"] = &FunctionSignature{Name: "echo", ParamTypes: []string{"dynamic"}, ReturnType: "void"}
	st.Functions["len"] = &FunctionSignature{Name: "len", ParamTypes: []string{"dynamic"}, ReturnType: "int"}
	st.Functions["type"] = &FunctionSignature{Name: "type", ParamTypes: []string{"dynamic"}, ReturnType: "string"}
	st.Functions["isset"] = &FunctionSignature{Name: "isset", ParamTypes: []string{"dynamic"}, ReturnType: "bool"}
	st.Functions["empty"] = &FunctionSignature{Name: "empty", ParamTypes: []string{"dynamic"}, ReturnType: "bool"}
	st.Functions["unset"] = &FunctionSignature{Name: "unset", ParamTypes: []string{"dynamic"}, ReturnType: "void"}
	
	// 数组操作函数
	st.Functions["push"] = &FunctionSignature{Name: "push", ParamTypes: []string{"dynamic", "dynamic"}, ReturnType: "dynamic", IsVariadic: true}
	st.Functions["pop"] = &FunctionSignature{Name: "pop", ParamTypes: []string{"dynamic"}, ReturnType: "dynamic"}
	st.Functions["shift"] = &FunctionSignature{Name: "shift", ParamTypes: []string{"dynamic"}, ReturnType: "dynamic"}
	st.Functions["unshift"] = &FunctionSignature{Name: "unshift", ParamTypes: []string{"dynamic", "dynamic"}, ReturnType: "dynamic", IsVariadic: true}
	st.Functions["slice"] = &FunctionSignature{Name: "slice", ParamTypes: []string{"dynamic", "int", "int"}, ReturnType: "dynamic", MinArity: 2}
	st.Functions["concat"] = &FunctionSignature{Name: "concat", ParamTypes: []string{"dynamic", "dynamic"}, ReturnType: "dynamic"}
	st.Functions["reverse"] = &FunctionSignature{Name: "reverse", ParamTypes: []string{"dynamic"}, ReturnType: "dynamic"}
	st.Functions["contains"] = &FunctionSignature{Name: "contains", ParamTypes: []string{"dynamic", "dynamic"}, ReturnType: "bool"}
	st.Functions["index_of"] = &FunctionSignature{Name: "index_of", ParamTypes: []string{"dynamic", "dynamic"}, ReturnType: "int"}
	st.Functions["var_dump"] = &FunctionSignature{Name: "var_dump", ParamTypes: []string{"dynamic"}, ReturnType: "void"}
	st.Functions["die"] = &FunctionSignature{Name: "die", ParamTypes: []string{"string"}, ReturnType: "void", MinArity: 0}
	st.Functions["exit"] = &FunctionSignature{Name: "exit", ParamTypes: []string{"int"}, ReturnType: "void", MinArity: 0}

	// 数学函数
	st.Functions["abs"] = &FunctionSignature{Name: "abs", ParamTypes: []string{"float"}, ReturnType: "float"}
	st.Functions["ceil"] = &FunctionSignature{Name: "ceil", ParamTypes: []string{"float"}, ReturnType: "float"}
	st.Functions["floor"] = &FunctionSignature{Name: "floor", ParamTypes: []string{"float"}, ReturnType: "float"}
	st.Functions["round"] = &FunctionSignature{Name: "round", ParamTypes: []string{"float"}, ReturnType: "float"}
	st.Functions["sqrt"] = &FunctionSignature{Name: "sqrt", ParamTypes: []string{"float"}, ReturnType: "float"}
	st.Functions["pow"] = &FunctionSignature{Name: "pow", ParamTypes: []string{"float", "float"}, ReturnType: "float"}
	st.Functions["min"] = &FunctionSignature{Name: "min", ParamTypes: []string{"float", "float"}, ReturnType: "float"}
	st.Functions["max"] = &FunctionSignature{Name: "max", ParamTypes: []string{"float", "float"}, ReturnType: "float"}
	st.Functions["sin"] = &FunctionSignature{Name: "sin", ParamTypes: []string{"float"}, ReturnType: "float"}
	st.Functions["cos"] = &FunctionSignature{Name: "cos", ParamTypes: []string{"float"}, ReturnType: "float"}
	st.Functions["tan"] = &FunctionSignature{Name: "tan", ParamTypes: []string{"float"}, ReturnType: "float"}
	st.Functions["log"] = &FunctionSignature{Name: "log", ParamTypes: []string{"float"}, ReturnType: "float"}
	st.Functions["exp"] = &FunctionSignature{Name: "exp", ParamTypes: []string{"float"}, ReturnType: "float"}
	st.Functions["rand"] = &FunctionSignature{Name: "rand", ParamTypes: []string{"int", "int"}, ReturnType: "int", MinArity: 0}

	// 字符串函数 (native_str_*) - 实际注册的函数名
	st.Functions["native_str_len"] = &FunctionSignature{Name: "native_str_len", ParamTypes: []string{"string"}, ReturnType: "int"}
	st.Functions["native_str_substring"] = &FunctionSignature{Name: "native_str_substring", ParamTypes: []string{"string", "int", "int"}, ReturnType: "string", MinArity: 2}
	st.Functions["native_str_to_upper"] = &FunctionSignature{Name: "native_str_to_upper", ParamTypes: []string{"string"}, ReturnType: "string"}
	st.Functions["native_str_to_lower"] = &FunctionSignature{Name: "native_str_to_lower", ParamTypes: []string{"string"}, ReturnType: "string"}
	st.Functions["native_str_trim"] = &FunctionSignature{Name: "native_str_trim", ParamTypes: []string{"string"}, ReturnType: "string"}
	st.Functions["native_str_replace"] = &FunctionSignature{Name: "native_str_replace", ParamTypes: []string{"string", "string", "string"}, ReturnType: "string"}
	st.Functions["native_str_split"] = &FunctionSignature{Name: "native_str_split", ParamTypes: []string{"string", "string"}, ReturnType: "string[]"}
	st.Functions["native_str_join"] = &FunctionSignature{Name: "native_str_join", ParamTypes: []string{"string[]", "string"}, ReturnType: "string"}
	st.Functions["native_str_index_of"] = &FunctionSignature{Name: "native_str_index_of", ParamTypes: []string{"string", "string", "int"}, ReturnType: "int", MinArity: 2}
	st.Functions["native_str_last_index_of"] = &FunctionSignature{Name: "native_str_last_index_of", ParamTypes: []string{"string", "string", "int"}, ReturnType: "int", MinArity: 2}
	st.Functions["native_str_to_int"] = &FunctionSignature{Name: "native_str_to_int", ParamTypes: []string{"string"}, ReturnType: "int"}
	st.Functions["native_str_to_float"] = &FunctionSignature{Name: "native_str_to_float", ParamTypes: []string{"string"}, ReturnType: "float"}

	// 时间函数 (native_time_*)
	st.Functions["native_time_now"] = &FunctionSignature{Name: "native_time_now", ParamTypes: []string{}, ReturnType: "int"}
	st.Functions["native_time_now_ms"] = &FunctionSignature{Name: "native_time_now_ms", ParamTypes: []string{}, ReturnType: "int"}
	st.Functions["native_time_now_nano"] = &FunctionSignature{Name: "native_time_now_nano", ParamTypes: []string{}, ReturnType: "int"}
	st.Functions["native_time_now_milli"] = &FunctionSignature{Name: "native_time_now_milli", ParamTypes: []string{}, ReturnType: "int"}
	st.Functions["native_time_now_micro"] = &FunctionSignature{Name: "native_time_now_micro", ParamTypes: []string{}, ReturnType: "int"}
	st.Functions["native_time_sleep"] = &FunctionSignature{Name: "native_time_sleep", ParamTypes: []string{"int"}, ReturnType: "void"}
	st.Functions["native_time_format"] = &FunctionSignature{Name: "native_time_format", ParamTypes: []string{"int", "string"}, ReturnType: "string"}
	st.Functions["native_time_parse"] = &FunctionSignature{Name: "native_time_parse", ParamTypes: []string{"string", "string"}, ReturnType: "int"}

	// 文件函数 (native_file_*)
	st.Functions["native_file_read"] = &FunctionSignature{Name: "native_file_read", ParamTypes: []string{"string"}, ReturnType: "string"}
	st.Functions["native_file_write"] = &FunctionSignature{Name: "native_file_write", ParamTypes: []string{"string", "string"}, ReturnType: "bool"}
	st.Functions["native_file_append"] = &FunctionSignature{Name: "native_file_append", ParamTypes: []string{"string", "string"}, ReturnType: "bool"}
	st.Functions["native_file_exists"] = &FunctionSignature{Name: "native_file_exists", ParamTypes: []string{"string"}, ReturnType: "bool"}
	st.Functions["native_file_delete"] = &FunctionSignature{Name: "native_file_delete", ParamTypes: []string{"string"}, ReturnType: "bool"}
	st.Functions["native_file_copy"] = &FunctionSignature{Name: "native_file_copy", ParamTypes: []string{"string", "string"}, ReturnType: "bool"}
	st.Functions["native_file_move"] = &FunctionSignature{Name: "native_file_move", ParamTypes: []string{"string", "string"}, ReturnType: "bool"}
	st.Functions["native_file_size"] = &FunctionSignature{Name: "native_file_size", ParamTypes: []string{"string"}, ReturnType: "int"}
	st.Functions["native_file_is_file"] = &FunctionSignature{Name: "native_file_is_file", ParamTypes: []string{"string"}, ReturnType: "bool"}
	st.Functions["native_file_is_dir"] = &FunctionSignature{Name: "native_file_is_dir", ParamTypes: []string{"string"}, ReturnType: "bool"}
	st.Functions["native_file_read_lines"] = &FunctionSignature{Name: "native_file_read_lines", ParamTypes: []string{"string"}, ReturnType: "string[]"}
	st.Functions["native_file_write_lines"] = &FunctionSignature{Name: "native_file_write_lines", ParamTypes: []string{"string", "string[]"}, ReturnType: "bool"}
	st.Functions["native_dir_create"] = &FunctionSignature{Name: "native_dir_create", ParamTypes: []string{"string"}, ReturnType: "bool"}
	st.Functions["native_dir_create_all"] = &FunctionSignature{Name: "native_dir_create_all", ParamTypes: []string{"string"}, ReturnType: "bool"}
	st.Functions["native_dir_list"] = &FunctionSignature{Name: "native_dir_list", ParamTypes: []string{"string"}, ReturnType: "string[]"}
	st.Functions["native_dir_remove"] = &FunctionSignature{Name: "native_dir_remove", ParamTypes: []string{"string"}, ReturnType: "bool"}

	// 正则表达式函数 (native_regex_*)
	st.Functions["native_regex_match"] = &FunctionSignature{Name: "native_regex_match", ParamTypes: []string{"string", "string"}, ReturnType: "bool"}
	st.Functions["native_regex_find"] = &FunctionSignature{Name: "native_regex_find", ParamTypes: []string{"string", "string"}, ReturnType: "string"}
	st.Functions["native_regex_find_all"] = &FunctionSignature{Name: "native_regex_find_all", ParamTypes: []string{"string", "string"}, ReturnType: "string[]"}
	st.Functions["native_regex_replace"] = &FunctionSignature{Name: "native_regex_replace", ParamTypes: []string{"string", "string", "string"}, ReturnType: "string"}
	st.Functions["native_regex_split"] = &FunctionSignature{Name: "native_regex_split", ParamTypes: []string{"string", "string"}, ReturnType: "string[]"}

	// JSON 函数 (native_json_*)
	st.Functions["native_json_encode"] = &FunctionSignature{Name: "native_json_encode", ParamTypes: []string{"dynamic", "bool", "string"}, ReturnType: "string", MinArity: 1}
	st.Functions["native_json_decode"] = &FunctionSignature{Name: "native_json_decode", ParamTypes: []string{"string"}, ReturnType: "dynamic"}
	st.Functions["native_json_is_valid"] = &FunctionSignature{Name: "native_json_is_valid", ParamTypes: []string{"string"}, ReturnType: "bool"}
	st.Functions["native_json_encode_object"] = &FunctionSignature{Name: "native_json_encode_object", ParamTypes: []string{"unknown", "dynamic"}, ReturnType: "string", MinArity: 1}

	// Crypto 哈希函数 (native_crypto_*)
	st.Functions["native_crypto_md5"] = &FunctionSignature{Name: "native_crypto_md5", ParamTypes: []string{"dynamic"}, ReturnType: "string"}
	st.Functions["native_crypto_md5_bytes"] = &FunctionSignature{Name: "native_crypto_md5_bytes", ParamTypes: []string{"dynamic"}, ReturnType: "byte[]"}
	st.Functions["native_crypto_sha1"] = &FunctionSignature{Name: "native_crypto_sha1", ParamTypes: []string{"dynamic"}, ReturnType: "string"}
	st.Functions["native_crypto_sha1_bytes"] = &FunctionSignature{Name: "native_crypto_sha1_bytes", ParamTypes: []string{"dynamic"}, ReturnType: "byte[]"}
	st.Functions["native_crypto_sha256"] = &FunctionSignature{Name: "native_crypto_sha256", ParamTypes: []string{"dynamic"}, ReturnType: "string"}
	st.Functions["native_crypto_sha256_bytes"] = &FunctionSignature{Name: "native_crypto_sha256_bytes", ParamTypes: []string{"dynamic"}, ReturnType: "byte[]"}
	st.Functions["native_crypto_sha384"] = &FunctionSignature{Name: "native_crypto_sha384", ParamTypes: []string{"dynamic"}, ReturnType: "string"}
	st.Functions["native_crypto_sha384_bytes"] = &FunctionSignature{Name: "native_crypto_sha384_bytes", ParamTypes: []string{"dynamic"}, ReturnType: "byte[]"}
	st.Functions["native_crypto_sha512"] = &FunctionSignature{Name: "native_crypto_sha512", ParamTypes: []string{"dynamic"}, ReturnType: "string"}
	st.Functions["native_crypto_sha512_bytes"] = &FunctionSignature{Name: "native_crypto_sha512_bytes", ParamTypes: []string{"dynamic"}, ReturnType: "byte[]"}

	// Crypto 流式哈希函数
	st.Functions["native_crypto_hash_create"] = &FunctionSignature{Name: "native_crypto_hash_create", ParamTypes: []string{"string"}, ReturnType: "int"}
	st.Functions["native_crypto_hash_update"] = &FunctionSignature{Name: "native_crypto_hash_update", ParamTypes: []string{"int", "dynamic"}, ReturnType: "bool"}
	st.Functions["native_crypto_hash_finalize"] = &FunctionSignature{Name: "native_crypto_hash_finalize", ParamTypes: []string{"int"}, ReturnType: "string"}
	st.Functions["native_crypto_hash_finalize_bytes"] = &FunctionSignature{Name: "native_crypto_hash_finalize_bytes", ParamTypes: []string{"int"}, ReturnType: "byte[]"}

	// Crypto HMAC函数
	st.Functions["native_crypto_hmac"] = &FunctionSignature{Name: "native_crypto_hmac", ParamTypes: []string{"string", "dynamic", "dynamic"}, ReturnType: "string"}
	st.Functions["native_crypto_hmac_bytes"] = &FunctionSignature{Name: "native_crypto_hmac_bytes", ParamTypes: []string{"string", "dynamic", "dynamic"}, ReturnType: "byte[]"}
	st.Functions["native_crypto_hmac_verify"] = &FunctionSignature{Name: "native_crypto_hmac_verify", ParamTypes: []string{"string", "dynamic", "dynamic", "string"}, ReturnType: "bool"}
	st.Functions["native_crypto_hmac_create"] = &FunctionSignature{Name: "native_crypto_hmac_create", ParamTypes: []string{"string", "dynamic"}, ReturnType: "int"}
	st.Functions["native_crypto_hmac_update"] = &FunctionSignature{Name: "native_crypto_hmac_update", ParamTypes: []string{"int", "dynamic"}, ReturnType: "bool"}
	st.Functions["native_crypto_hmac_finalize"] = &FunctionSignature{Name: "native_crypto_hmac_finalize", ParamTypes: []string{"int"}, ReturnType: "string"}

	// Crypto AES函数
	st.Functions["native_crypto_aes_encrypt_cbc"] = &FunctionSignature{Name: "native_crypto_aes_encrypt_cbc", ParamTypes: []string{"byte[]", "byte[]", "byte[]"}, ReturnType: "byte[]"}
	st.Functions["native_crypto_aes_decrypt_cbc"] = &FunctionSignature{Name: "native_crypto_aes_decrypt_cbc", ParamTypes: []string{"byte[]", "byte[]", "byte[]"}, ReturnType: "byte[]"}
	st.Functions["native_crypto_aes_encrypt_gcm"] = &FunctionSignature{Name: "native_crypto_aes_encrypt_gcm", ParamTypes: []string{"byte[]", "byte[]", "byte[]", "byte[]"}, ReturnType: "byte[]", MinArity: 3}
	st.Functions["native_crypto_aes_decrypt_gcm"] = &FunctionSignature{Name: "native_crypto_aes_decrypt_gcm", ParamTypes: []string{"byte[]", "byte[]", "byte[]", "byte[]"}, ReturnType: "byte[]", MinArity: 3}
	st.Functions["native_crypto_aes_encrypt_ctr"] = &FunctionSignature{Name: "native_crypto_aes_encrypt_ctr", ParamTypes: []string{"byte[]", "byte[]", "byte[]"}, ReturnType: "byte[]"}
	st.Functions["native_crypto_aes_decrypt_ctr"] = &FunctionSignature{Name: "native_crypto_aes_decrypt_ctr", ParamTypes: []string{"byte[]", "byte[]", "byte[]"}, ReturnType: "byte[]"}

	// Crypto DES/3DES函数
	st.Functions["native_crypto_des_encrypt"] = &FunctionSignature{Name: "native_crypto_des_encrypt", ParamTypes: []string{"byte[]", "byte[]", "byte[]"}, ReturnType: "byte[]"}
	st.Functions["native_crypto_des_decrypt"] = &FunctionSignature{Name: "native_crypto_des_decrypt", ParamTypes: []string{"byte[]", "byte[]", "byte[]"}, ReturnType: "byte[]"}
	st.Functions["native_crypto_triple_des_encrypt"] = &FunctionSignature{Name: "native_crypto_triple_des_encrypt", ParamTypes: []string{"byte[]", "byte[]", "byte[]"}, ReturnType: "byte[]"}
	st.Functions["native_crypto_triple_des_decrypt"] = &FunctionSignature{Name: "native_crypto_triple_des_decrypt", ParamTypes: []string{"byte[]", "byte[]", "byte[]"}, ReturnType: "byte[]"}

	// Crypto RSA函数
	st.Functions["native_crypto_rsa_generate"] = &FunctionSignature{Name: "native_crypto_rsa_generate", ParamTypes: []string{"int"}, ReturnType: "dynamic", MinArity: 0}
	st.Functions["native_crypto_rsa_get_public_key_pem"] = &FunctionSignature{Name: "native_crypto_rsa_get_public_key_pem", ParamTypes: []string{"int"}, ReturnType: "string"}
	st.Functions["native_crypto_rsa_get_private_key_pem"] = &FunctionSignature{Name: "native_crypto_rsa_get_private_key_pem", ParamTypes: []string{"int"}, ReturnType: "string"}
	st.Functions["native_crypto_rsa_load_public_key"] = &FunctionSignature{Name: "native_crypto_rsa_load_public_key", ParamTypes: []string{"string"}, ReturnType: "int"}
	st.Functions["native_crypto_rsa_load_private_key"] = &FunctionSignature{Name: "native_crypto_rsa_load_private_key", ParamTypes: []string{"string"}, ReturnType: "int"}
	st.Functions["native_crypto_rsa_encrypt"] = &FunctionSignature{Name: "native_crypto_rsa_encrypt", ParamTypes: []string{"dynamic", "int"}, ReturnType: "byte[]"}
	st.Functions["native_crypto_rsa_decrypt"] = &FunctionSignature{Name: "native_crypto_rsa_decrypt", ParamTypes: []string{"byte[]", "int"}, ReturnType: "byte[]"}
	st.Functions["native_crypto_rsa_sign"] = &FunctionSignature{Name: "native_crypto_rsa_sign", ParamTypes: []string{"dynamic", "int", "string"}, ReturnType: "byte[]"}
	st.Functions["native_crypto_rsa_verify"] = &FunctionSignature{Name: "native_crypto_rsa_verify", ParamTypes: []string{"dynamic", "byte[]", "int", "string"}, ReturnType: "bool"}
	st.Functions["native_crypto_rsa_sign_pkcs1"] = &FunctionSignature{Name: "native_crypto_rsa_sign_pkcs1", ParamTypes: []string{"dynamic", "int", "string"}, ReturnType: "byte[]"}
	st.Functions["native_crypto_rsa_verify_pkcs1"] = &FunctionSignature{Name: "native_crypto_rsa_verify_pkcs1", ParamTypes: []string{"dynamic", "byte[]", "int", "string"}, ReturnType: "bool"}
	st.Functions["native_crypto_rsa_encrypt_pkcs1"] = &FunctionSignature{Name: "native_crypto_rsa_encrypt_pkcs1", ParamTypes: []string{"dynamic", "int"}, ReturnType: "byte[]"}
	st.Functions["native_crypto_rsa_decrypt_pkcs1"] = &FunctionSignature{Name: "native_crypto_rsa_decrypt_pkcs1", ParamTypes: []string{"byte[]", "int"}, ReturnType: "byte[]"}
	st.Functions["native_crypto_rsa_free"] = &FunctionSignature{Name: "native_crypto_rsa_free", ParamTypes: []string{"int"}, ReturnType: "bool"}

	// Crypto ECDSA函数
	st.Functions["native_crypto_ecdsa_generate"] = &FunctionSignature{Name: "native_crypto_ecdsa_generate", ParamTypes: []string{"string"}, ReturnType: "dynamic", MinArity: 0}
	st.Functions["native_crypto_ecdsa_sign"] = &FunctionSignature{Name: "native_crypto_ecdsa_sign", ParamTypes: []string{"dynamic", "int"}, ReturnType: "byte[]"}
	st.Functions["native_crypto_ecdsa_verify"] = &FunctionSignature{Name: "native_crypto_ecdsa_verify", ParamTypes: []string{"dynamic", "byte[]", "int"}, ReturnType: "bool"}
	st.Functions["native_crypto_ecdsa_get_public_key_pem"] = &FunctionSignature{Name: "native_crypto_ecdsa_get_public_key_pem", ParamTypes: []string{"int"}, ReturnType: "string"}
	st.Functions["native_crypto_ecdsa_get_private_key_pem"] = &FunctionSignature{Name: "native_crypto_ecdsa_get_private_key_pem", ParamTypes: []string{"int"}, ReturnType: "string"}
	st.Functions["native_crypto_ecdsa_load_public_key"] = &FunctionSignature{Name: "native_crypto_ecdsa_load_public_key", ParamTypes: []string{"string"}, ReturnType: "int"}
	st.Functions["native_crypto_ecdsa_load_private_key"] = &FunctionSignature{Name: "native_crypto_ecdsa_load_private_key", ParamTypes: []string{"string"}, ReturnType: "int"}
	st.Functions["native_crypto_ecdsa_free"] = &FunctionSignature{Name: "native_crypto_ecdsa_free", ParamTypes: []string{"int"}, ReturnType: "bool"}

	// Crypto Ed25519函数
	st.Functions["native_crypto_ed25519_generate"] = &FunctionSignature{Name: "native_crypto_ed25519_generate", ParamTypes: []string{}, ReturnType: "dynamic"}
	st.Functions["native_crypto_ed25519_sign"] = &FunctionSignature{Name: "native_crypto_ed25519_sign", ParamTypes: []string{"dynamic", "int"}, ReturnType: "byte[]"}
	st.Functions["native_crypto_ed25519_verify"] = &FunctionSignature{Name: "native_crypto_ed25519_verify", ParamTypes: []string{"dynamic", "byte[]", "int"}, ReturnType: "bool"}
	st.Functions["native_crypto_ed25519_get_public_key_bytes"] = &FunctionSignature{Name: "native_crypto_ed25519_get_public_key_bytes", ParamTypes: []string{"int"}, ReturnType: "byte[]"}
	st.Functions["native_crypto_ed25519_get_private_key_bytes"] = &FunctionSignature{Name: "native_crypto_ed25519_get_private_key_bytes", ParamTypes: []string{"int"}, ReturnType: "byte[]"}
	st.Functions["native_crypto_ed25519_load_public_key"] = &FunctionSignature{Name: "native_crypto_ed25519_load_public_key", ParamTypes: []string{"byte[]"}, ReturnType: "int"}
	st.Functions["native_crypto_ed25519_load_private_key"] = &FunctionSignature{Name: "native_crypto_ed25519_load_private_key", ParamTypes: []string{"byte[]"}, ReturnType: "int"}
	st.Functions["native_crypto_ed25519_free"] = &FunctionSignature{Name: "native_crypto_ed25519_free", ParamTypes: []string{"int"}, ReturnType: "bool"}

	// Crypto 密钥派生函数
	st.Functions["native_crypto_pbkdf2"] = &FunctionSignature{Name: "native_crypto_pbkdf2", ParamTypes: []string{"dynamic", "byte[]", "int", "int", "string"}, ReturnType: "byte[]"}
	st.Functions["native_crypto_hkdf"] = &FunctionSignature{Name: "native_crypto_hkdf", ParamTypes: []string{"byte[]", "byte[]", "byte[]", "int", "string"}, ReturnType: "byte[]"}
	st.Functions["native_crypto_scrypt"] = &FunctionSignature{Name: "native_crypto_scrypt", ParamTypes: []string{"dynamic", "byte[]", "int", "int", "int", "int"}, ReturnType: "byte[]"}
	st.Functions["native_crypto_argon2id"] = &FunctionSignature{Name: "native_crypto_argon2id", ParamTypes: []string{"dynamic", "byte[]", "int", "int", "int", "int"}, ReturnType: "byte[]"}
	st.Functions["native_crypto_argon2i"] = &FunctionSignature{Name: "native_crypto_argon2i", ParamTypes: []string{"dynamic", "byte[]", "int", "int", "int", "int"}, ReturnType: "byte[]"}

	// Crypto 随机数函数
	st.Functions["native_crypto_random_bytes"] = &FunctionSignature{Name: "native_crypto_random_bytes", ParamTypes: []string{"int"}, ReturnType: "byte[]"}
	st.Functions["native_crypto_random_int"] = &FunctionSignature{Name: "native_crypto_random_int", ParamTypes: []string{"int", "int"}, ReturnType: "int"}
	st.Functions["native_crypto_random_hex"] = &FunctionSignature{Name: "native_crypto_random_hex", ParamTypes: []string{"int"}, ReturnType: "string"}
	st.Functions["native_crypto_random_uuid"] = &FunctionSignature{Name: "native_crypto_random_uuid", ParamTypes: []string{}, ReturnType: "string"}

	// Crypto Hex函数
	st.Functions["native_crypto_hex_encode"] = &FunctionSignature{Name: "native_crypto_hex_encode", ParamTypes: []string{"byte[]"}, ReturnType: "string"}
	st.Functions["native_crypto_hex_decode"] = &FunctionSignature{Name: "native_crypto_hex_decode", ParamTypes: []string{"string"}, ReturnType: "byte[]"}
	st.Functions["native_crypto_hex_is_valid"] = &FunctionSignature{Name: "native_crypto_hex_is_valid", ParamTypes: []string{"string"}, ReturnType: "bool"}

	// Base64 函数 (native_base64_*)
	st.Functions["native_base64_encode"] = &FunctionSignature{Name: "native_base64_encode", ParamTypes: []string{"string"}, ReturnType: "string"}
	st.Functions["native_base64_decode"] = &FunctionSignature{Name: "native_base64_decode", ParamTypes: []string{"string"}, ReturnType: "string"}
	st.Functions["native_base64_encode_bytes"] = &FunctionSignature{Name: "native_base64_encode_bytes", ParamTypes: []string{"byte[]"}, ReturnType: "string"}
	st.Functions["native_base64_decode_to_bytes"] = &FunctionSignature{Name: "native_base64_decode_to_bytes", ParamTypes: []string{"string"}, ReturnType: "byte[]"}

	// TCP 函数 (native_tcp_*)
	st.Functions["native_tcp_connect"] = &FunctionSignature{Name: "native_tcp_connect", ParamTypes: []string{"string", "int"}, ReturnType: "int"}
	st.Functions["native_tcp_connect_timeout"] = &FunctionSignature{Name: "native_tcp_connect_timeout", ParamTypes: []string{"string", "int", "int"}, ReturnType: "int"}
	st.Functions["native_tcp_close"] = &FunctionSignature{Name: "native_tcp_close", ParamTypes: []string{"int"}, ReturnType: "bool"}
	st.Functions["native_tcp_is_connected"] = &FunctionSignature{Name: "native_tcp_is_connected", ParamTypes: []string{"int"}, ReturnType: "bool"}
	st.Functions["native_tcp_is_tls"] = &FunctionSignature{Name: "native_tcp_is_tls", ParamTypes: []string{"int"}, ReturnType: "bool"}
	st.Functions["native_tcp_write"] = &FunctionSignature{Name: "native_tcp_write", ParamTypes: []string{"int", "string"}, ReturnType: "int"}
	st.Functions["native_tcp_write_bytes"] = &FunctionSignature{Name: "native_tcp_write_bytes", ParamTypes: []string{"int", "byte[]"}, ReturnType: "int"}
	st.Functions["native_tcp_read"] = &FunctionSignature{Name: "native_tcp_read", ParamTypes: []string{"int", "int"}, ReturnType: "string"}
	st.Functions["native_tcp_read_bytes"] = &FunctionSignature{Name: "native_tcp_read_bytes", ParamTypes: []string{"int", "int"}, ReturnType: "byte[]"}
	st.Functions["native_tcp_read_exact"] = &FunctionSignature{Name: "native_tcp_read_exact", ParamTypes: []string{"int", "int"}, ReturnType: "byte[]"}
	st.Functions["native_tcp_read_line"] = &FunctionSignature{Name: "native_tcp_read_line", ParamTypes: []string{"int"}, ReturnType: "string"}
	st.Functions["native_tcp_read_until"] = &FunctionSignature{Name: "native_tcp_read_until", ParamTypes: []string{"int", "string"}, ReturnType: "string"}
	st.Functions["native_tcp_available"] = &FunctionSignature{Name: "native_tcp_available", ParamTypes: []string{"int"}, ReturnType: "int"}
	st.Functions["native_tcp_flush"] = &FunctionSignature{Name: "native_tcp_flush", ParamTypes: []string{"int"}, ReturnType: "bool"}
	st.Functions["native_tcp_set_timeout"] = &FunctionSignature{Name: "native_tcp_set_timeout", ParamTypes: []string{"int", "int"}, ReturnType: "bool"}
	st.Functions["native_tcp_set_timeout_ms"] = &FunctionSignature{Name: "native_tcp_set_timeout_ms", ParamTypes: []string{"int", "int"}, ReturnType: "bool"}
	st.Functions["native_tcp_set_read_timeout"] = &FunctionSignature{Name: "native_tcp_set_read_timeout", ParamTypes: []string{"int", "int"}, ReturnType: "bool"}
	st.Functions["native_tcp_set_write_timeout"] = &FunctionSignature{Name: "native_tcp_set_write_timeout", ParamTypes: []string{"int", "int"}, ReturnType: "bool"}
	st.Functions["native_tcp_clear_timeout"] = &FunctionSignature{Name: "native_tcp_clear_timeout", ParamTypes: []string{"int"}, ReturnType: "bool"}
	st.Functions["native_tcp_set_keepalive"] = &FunctionSignature{Name: "native_tcp_set_keepalive", ParamTypes: []string{"int", "bool"}, ReturnType: "bool"}
	st.Functions["native_tcp_set_nodelay"] = &FunctionSignature{Name: "native_tcp_set_nodelay", ParamTypes: []string{"int", "bool"}, ReturnType: "bool"}
	st.Functions["native_tcp_set_linger"] = &FunctionSignature{Name: "native_tcp_set_linger", ParamTypes: []string{"int", "int"}, ReturnType: "bool"}
	st.Functions["native_tcp_set_read_buffer"] = &FunctionSignature{Name: "native_tcp_set_read_buffer", ParamTypes: []string{"int", "int"}, ReturnType: "bool"}
	st.Functions["native_tcp_set_write_buffer"] = &FunctionSignature{Name: "native_tcp_set_write_buffer", ParamTypes: []string{"int", "int"}, ReturnType: "bool"}
	st.Functions["native_tcp_listen"] = &FunctionSignature{Name: "native_tcp_listen", ParamTypes: []string{"string", "int"}, ReturnType: "int"}
	st.Functions["native_tcp_accept"] = &FunctionSignature{Name: "native_tcp_accept", ParamTypes: []string{"int"}, ReturnType: "int"}
	st.Functions["native_tcp_accept_timeout"] = &FunctionSignature{Name: "native_tcp_accept_timeout", ParamTypes: []string{"int", "int"}, ReturnType: "int"}
	st.Functions["native_tcp_stop_listen"] = &FunctionSignature{Name: "native_tcp_stop_listen", ParamTypes: []string{"int"}, ReturnType: "bool"}
	st.Functions["native_tcp_listener_addr"] = &FunctionSignature{Name: "native_tcp_listener_addr", ParamTypes: []string{"int"}, ReturnType: "string"}
	st.Functions["native_tcp_listener_host"] = &FunctionSignature{Name: "native_tcp_listener_host", ParamTypes: []string{"int"}, ReturnType: "string"}
	st.Functions["native_tcp_listener_port"] = &FunctionSignature{Name: "native_tcp_listener_port", ParamTypes: []string{"int"}, ReturnType: "int"}
	st.Functions["native_tcp_listener_is_listening"] = &FunctionSignature{Name: "native_tcp_listener_is_listening", ParamTypes: []string{"int"}, ReturnType: "bool"}
	st.Functions["native_tcp_get_remote_addr"] = &FunctionSignature{Name: "native_tcp_get_remote_addr", ParamTypes: []string{"int"}, ReturnType: "string"}
	st.Functions["native_tcp_get_local_addr"] = &FunctionSignature{Name: "native_tcp_get_local_addr", ParamTypes: []string{"int"}, ReturnType: "string"}
	st.Functions["native_tcp_get_remote_host"] = &FunctionSignature{Name: "native_tcp_get_remote_host", ParamTypes: []string{"int"}, ReturnType: "string"}
	st.Functions["native_tcp_get_remote_port"] = &FunctionSignature{Name: "native_tcp_get_remote_port", ParamTypes: []string{"int"}, ReturnType: "int"}
	st.Functions["native_tcp_get_local_host"] = &FunctionSignature{Name: "native_tcp_get_local_host", ParamTypes: []string{"int"}, ReturnType: "string"}
	st.Functions["native_tcp_get_local_port"] = &FunctionSignature{Name: "native_tcp_get_local_port", ParamTypes: []string{"int"}, ReturnType: "int"}

	// Bytes 函数 (native_bytes_*)
	st.Functions["native_bytes_new"] = &FunctionSignature{Name: "native_bytes_new", ParamTypes: []string{"int"}, ReturnType: "byte[]"}
	st.Functions["native_bytes_from_string"] = &FunctionSignature{Name: "native_bytes_from_string", ParamTypes: []string{"string"}, ReturnType: "byte[]"}
	st.Functions["native_bytes_to_string"] = &FunctionSignature{Name: "native_bytes_to_string", ParamTypes: []string{"byte[]"}, ReturnType: "string"}
	st.Functions["native_bytes_from_hex"] = &FunctionSignature{Name: "native_bytes_from_hex", ParamTypes: []string{"string"}, ReturnType: "byte[]"}
	st.Functions["native_bytes_to_hex"] = &FunctionSignature{Name: "native_bytes_to_hex", ParamTypes: []string{"byte[]"}, ReturnType: "string"}
	st.Functions["native_bytes_from_array"] = &FunctionSignature{Name: "native_bytes_from_array", ParamTypes: []string{"int[]"}, ReturnType: "byte[]"}
	st.Functions["native_bytes_to_array"] = &FunctionSignature{Name: "native_bytes_to_array", ParamTypes: []string{"byte[]"}, ReturnType: "int[]"}
	st.Functions["native_bytes_len"] = &FunctionSignature{Name: "native_bytes_len", ParamTypes: []string{"byte[]"}, ReturnType: "int"}
	st.Functions["native_bytes_length"] = &FunctionSignature{Name: "native_bytes_length", ParamTypes: []string{"byte[]"}, ReturnType: "int"}
	st.Functions["native_bytes_get"] = &FunctionSignature{Name: "native_bytes_get", ParamTypes: []string{"byte[]", "int"}, ReturnType: "int"}
	st.Functions["native_bytes_set"] = &FunctionSignature{Name: "native_bytes_set", ParamTypes: []string{"byte[]", "int", "int"}, ReturnType: "void"}
	st.Functions["native_bytes_slice"] = &FunctionSignature{Name: "native_bytes_slice", ParamTypes: []string{"byte[]", "int", "int"}, ReturnType: "byte[]"}
	st.Functions["native_bytes_concat"] = &FunctionSignature{Name: "native_bytes_concat", ParamTypes: []string{"byte[]", "byte[]"}, ReturnType: "byte[]"}
	st.Functions["native_bytes_copy"] = &FunctionSignature{Name: "native_bytes_copy", ParamTypes: []string{"byte[]"}, ReturnType: "byte[]"}
	st.Functions["native_bytes_equal"] = &FunctionSignature{Name: "native_bytes_equal", ParamTypes: []string{"byte[]", "byte[]"}, ReturnType: "bool"}
	st.Functions["native_bytes_compare"] = &FunctionSignature{Name: "native_bytes_compare", ParamTypes: []string{"byte[]", "byte[]"}, ReturnType: "int"}
	st.Functions["native_bytes_index"] = &FunctionSignature{Name: "native_bytes_index", ParamTypes: []string{"byte[]", "byte[]"}, ReturnType: "int"}
	st.Functions["native_bytes_index_of"] = &FunctionSignature{Name: "native_bytes_index_of", ParamTypes: []string{"byte[]", "byte[]"}, ReturnType: "int"}
	st.Functions["native_bytes_contains"] = &FunctionSignature{Name: "native_bytes_contains", ParamTypes: []string{"byte[]", "byte[]"}, ReturnType: "bool"}
	st.Functions["native_bytes_fill"] = &FunctionSignature{Name: "native_bytes_fill", ParamTypes: []string{"byte[]", "int"}, ReturnType: "void"}
	st.Functions["native_bytes_zero"] = &FunctionSignature{Name: "native_bytes_zero", ParamTypes: []string{"byte[]"}, ReturnType: "void"}

	// 流操作函数 (native_stream_*)
	st.Functions["native_stream_open"] = &FunctionSignature{Name: "native_stream_open", ParamTypes: []string{"string", "string"}, ReturnType: "int"}
	st.Functions["native_stream_close"] = &FunctionSignature{Name: "native_stream_close", ParamTypes: []string{"int"}, ReturnType: "void"}
	st.Functions["native_stream_read"] = &FunctionSignature{Name: "native_stream_read", ParamTypes: []string{"int", "int"}, ReturnType: "byte[]"}
	st.Functions["native_stream_write"] = &FunctionSignature{Name: "native_stream_write", ParamTypes: []string{"int", "byte[]"}, ReturnType: "int"}
	st.Functions["native_stream_seek"] = &FunctionSignature{Name: "native_stream_seek", ParamTypes: []string{"int", "int", "int"}, ReturnType: "int"}
	st.Functions["native_stream_tell"] = &FunctionSignature{Name: "native_stream_tell", ParamTypes: []string{"int"}, ReturnType: "int"}
	st.Functions["native_stream_flush"] = &FunctionSignature{Name: "native_stream_flush", ParamTypes: []string{"int"}, ReturnType: "void"}
	st.Functions["native_stream_eof"] = &FunctionSignature{Name: "native_stream_eof", ParamTypes: []string{"int"}, ReturnType: "bool"}

	// 反射函数 (native_reflect_*)
	st.Functions["native_reflect_get_class"] = &FunctionSignature{Name: "native_reflect_get_class", ParamTypes: []string{"unknown"}, ReturnType: "string"}
	st.Functions["native_reflect_get_methods"] = &FunctionSignature{Name: "native_reflect_get_methods", ParamTypes: []string{"unknown"}, ReturnType: "string[]"}
	st.Functions["native_reflect_get_properties"] = &FunctionSignature{Name: "native_reflect_get_properties", ParamTypes: []string{"unknown"}, ReturnType: "string[]"}
	st.Functions["native_reflect_has_method"] = &FunctionSignature{Name: "native_reflect_has_method", ParamTypes: []string{"unknown", "string"}, ReturnType: "bool"}
	st.Functions["native_reflect_has_property"] = &FunctionSignature{Name: "native_reflect_has_property", ParamTypes: []string{"unknown", "string"}, ReturnType: "bool"}
	st.Functions["native_reflect_get_annotations"] = &FunctionSignature{Name: "native_reflect_get_annotations", ParamTypes: []string{"unknown"}, ReturnType: "map[string]any"}
	st.Functions["native_reflect_get_parent_class"] = &FunctionSignature{Name: "native_reflect_get_parent_class", ParamTypes: []string{"unknown"}, ReturnType: "string"}
	st.Functions["native_reflect_get_interfaces"] = &FunctionSignature{Name: "native_reflect_get_interfaces", ParamTypes: []string{"unknown"}, ReturnType: "string[]"}
	st.Functions["native_reflect_is_instance_of"] = &FunctionSignature{Name: "native_reflect_is_instance_of", ParamTypes: []string{"unknown", "string"}, ReturnType: "bool"}

	// ORM 反射扩展函数
	st.Functions["native_reflect_set_property"] = &FunctionSignature{Name: "native_reflect_set_property", ParamTypes: []string{"unknown", "string", "dynamic"}, ReturnType: "bool"}
	st.Functions["native_reflect_get_property"] = &FunctionSignature{Name: "native_reflect_get_property", ParamTypes: []string{"unknown", "string"}, ReturnType: "dynamic"}
	st.Functions["native_reflect_new_instance"] = &FunctionSignature{Name: "native_reflect_new_instance", ParamTypes: []string{"string"}, ReturnType: "dynamic"}
	st.Functions["native_reflect_get_property_annotations"] = &FunctionSignature{Name: "native_reflect_get_property_annotations", ParamTypes: []string{"unknown", "string"}, ReturnType: "array"}

	// 新增的注解反射函数
	st.Functions["native_reflect_get_class_annotations"] = &FunctionSignature{Name: "native_reflect_get_class_annotations", ParamTypes: []string{"string"}, ReturnType: "array"}
	st.Functions["native_reflect_get_method_annotations"] = &FunctionSignature{Name: "native_reflect_get_method_annotations", ParamTypes: []string{"string", "string"}, ReturnType: "array"}
	st.Functions["native_reflect_is_attribute"] = &FunctionSignature{Name: "native_reflect_is_attribute", ParamTypes: []string{"string"}, ReturnType: "bool"}
	st.Functions["native_reflect_get_parent"] = &FunctionSignature{Name: "native_reflect_get_parent", ParamTypes: []string{"string"}, ReturnType: "string"}
	
	// typeof 函数
	st.Functions["typeof"] = &FunctionSignature{Name: "typeof", ParamTypes: []string{"dynamic"}, ReturnType: "string"}
}

// registerBuiltinTypeMethods 注册内置类型的方法签名
func (st *SymbolTable) registerBuiltinTypeMethods() {
	// SuperArray 万能数组方法
	st.RegisterMethod(&MethodSignature{ClassName: "SuperArray", MethodName: "len", ParamTypes: []string{}, ReturnType: "int"})
	st.RegisterMethod(&MethodSignature{ClassName: "SuperArray", MethodName: "hasKey", ParamTypes: []string{"dynamic"}, ReturnType: "bool"})
	st.RegisterMethod(&MethodSignature{ClassName: "SuperArray", MethodName: "get", ParamTypes: []string{"dynamic"}, ReturnType: "dynamic"})
	st.RegisterMethod(&MethodSignature{ClassName: "SuperArray", MethodName: "get", ParamTypes: []string{"dynamic", "dynamic"}, ReturnType: "dynamic"})
	st.RegisterMethod(&MethodSignature{ClassName: "SuperArray", MethodName: "set", ParamTypes: []string{"dynamic", "dynamic"}, ReturnType: "SuperArray"})
	st.RegisterMethod(&MethodSignature{ClassName: "SuperArray", MethodName: "keys", ParamTypes: []string{}, ReturnType: "SuperArray"})
	st.RegisterMethod(&MethodSignature{ClassName: "SuperArray", MethodName: "values", ParamTypes: []string{}, ReturnType: "SuperArray"})
	st.RegisterMethod(&MethodSignature{ClassName: "SuperArray", MethodName: "push", ParamTypes: []string{"dynamic"}, ReturnType: "SuperArray"})
	st.RegisterMethod(&MethodSignature{ClassName: "SuperArray", MethodName: "pop", ParamTypes: []string{}, ReturnType: "dynamic"})
	st.RegisterMethod(&MethodSignature{ClassName: "SuperArray", MethodName: "shift", ParamTypes: []string{}, ReturnType: "dynamic"})
	st.RegisterMethod(&MethodSignature{ClassName: "SuperArray", MethodName: "unshift", ParamTypes: []string{"dynamic"}, ReturnType: "SuperArray"})
	st.RegisterMethod(&MethodSignature{ClassName: "SuperArray", MethodName: "merge", ParamTypes: []string{"SuperArray"}, ReturnType: "SuperArray"})
	st.RegisterMethod(&MethodSignature{ClassName: "SuperArray", MethodName: "slice", ParamTypes: []string{"int"}, ReturnType: "SuperArray"})
	st.RegisterMethod(&MethodSignature{ClassName: "SuperArray", MethodName: "slice", ParamTypes: []string{"int", "int"}, ReturnType: "SuperArray"})
	st.RegisterMethod(&MethodSignature{ClassName: "SuperArray", MethodName: "remove", ParamTypes: []string{"dynamic"}, ReturnType: "bool"})
	st.RegisterMethod(&MethodSignature{ClassName: "SuperArray", MethodName: "clear", ParamTypes: []string{}, ReturnType: "void"})
	st.RegisterMethod(&MethodSignature{ClassName: "SuperArray", MethodName: "copy", ParamTypes: []string{}, ReturnType: "SuperArray"})
}

// RegisterFunction 注册函数签名
func (st *SymbolTable) RegisterFunction(sig *FunctionSignature) {
	st.Functions[sig.Name] = sig
}

// RegisterMethod 注册方法签名
func (st *SymbolTable) RegisterMethod(sig *MethodSignature) {
	if st.ClassMethods[sig.ClassName] == nil {
		st.ClassMethods[sig.ClassName] = make(map[string][]*MethodSignature)
	}
	st.ClassMethods[sig.ClassName][sig.MethodName] = append(
		st.ClassMethods[sig.ClassName][sig.MethodName], sig,
	)
}

// RegisterProperty 注册属性签名
func (st *SymbolTable) RegisterProperty(sig *PropertySignature) {
	if st.ClassProperties[sig.ClassName] == nil {
		st.ClassProperties[sig.ClassName] = make(map[string]*PropertySignature)
	}
	st.ClassProperties[sig.ClassName][sig.PropName] = sig
}

// RegisterClassParent 注册类继承关系
func (st *SymbolTable) RegisterClassParent(className, parentName string) {
	st.ClassParents[className] = parentName
}

// GetFunction 获取函数签名
func (st *SymbolTable) GetFunction(name string) *FunctionSignature {
	if sig, ok := st.Functions[name]; ok {
		return sig
	}
	// 回退到全局内置符号表
	global := getGlobalBuiltinSymbols()
	if global != nil && global != st {
		return global.Functions[name]
	}
	return nil
}

// GetMethod 获取方法签名（按参数数量匹配）
func (st *SymbolTable) GetMethod(className, methodName string, arity int) *MethodSignature {
	// 提取基类名（去除泛型参数）
	baseClassName := extractBaseTypeName(className)
	
	
	// 首先尝试直接查找
	if sig := st.getMethodDirect(baseClassName, methodName, arity); sig != nil {
		return sig
	}
	
	// 如果类名不包含命名空间分隔符，尝试在所有命名空间中查找
	if !strings.Contains(baseClassName, "\\") {
		suffix := "\\" + baseClassName
		for fullClassName := range st.ClassMethods {
			if strings.HasSuffix(fullClassName, suffix) || fullClassName == baseClassName {
				if sig := st.getMethodDirect(fullClassName, methodName, arity); sig != nil {
					return sig
				}
			}
		}
		// 同时尝试点分隔符（命名空间可能用点而不是反斜杠）
		suffix2 := "." + baseClassName
		for fullClassName := range st.ClassMethods {
			if strings.HasSuffix(fullClassName, suffix2) {
				if sig := st.getMethodDirect(fullClassName, methodName, arity); sig != nil {
					return sig
				}
			}
		}
	}
	
	// 回退到全局内置符号表
	global := getGlobalBuiltinSymbols()
	if global != nil && global != st {
		return global.GetMethod(className, methodName, arity)
	}
	
	return nil
}

// getMethodDirect 直接在指定类中查找方法
func (st *SymbolTable) getMethodDirect(className, methodName string, arity int) *MethodSignature {
	// 在当前类中查找
	if methods, ok := st.ClassMethods[className]; ok {
		if sigs, ok := methods[methodName]; ok {
			// 先精确匹配参数数量
			for _, sig := range sigs {
				if len(sig.ParamTypes) == arity {
					return sig
				}
			}
			// 再考虑默认参数
			for _, sig := range sigs {
				if arity >= sig.MinArity && arity <= len(sig.ParamTypes) {
					return sig
				}
			}
			// 返回第一个
			if len(sigs) > 0 {
				return sigs[0]
			}
		}
	}
	
	// 在父类中查找 - 先直接查找
	if parentName, ok := st.ClassParents[className]; ok && parentName != "" {
		return st.GetMethod(parentName, methodName, arity)
	}
	
	// 如果类名包含命名空间分隔符，也尝试用基类名查找父类
	if strings.Contains(className, "\\") {
		// 提取基类名
		parts := strings.Split(className, "\\")
		baseName := parts[len(parts)-1]
		if parentName, ok := st.ClassParents[baseName]; ok && parentName != "" {
			return st.GetMethod(parentName, methodName, arity)
		}
	}
	
	return nil
}

// GetProperty 获取属性签名
func (st *SymbolTable) GetProperty(className, propName string) *PropertySignature {
	// 提取基类名（去除泛型参数）
	baseClassName := extractBaseTypeName(className)
	
	// 首先尝试直接查找
	if sig := st.getPropertyDirect(baseClassName, propName); sig != nil {
		return sig
	}
	
	// 如果类名不包含命名空间分隔符，尝试在所有命名空间中查找
	if !strings.Contains(baseClassName, "\\") {
		suffix := "\\" + baseClassName
		for fullClassName := range st.ClassProperties {
			if strings.HasSuffix(fullClassName, suffix) || fullClassName == baseClassName {
				if sig := st.getPropertyDirect(fullClassName, propName); sig != nil {
					return sig
				}
			}
		}
	}

	// 回退到全局内置符号表
	global := getGlobalBuiltinSymbols()
	if global != nil && global != st {
		return global.GetProperty(className, propName)
	}

	return nil
}

// getPropertyDirect 直接在指定类中查找属性
func (st *SymbolTable) getPropertyDirect(className, propName string) *PropertySignature {
	// 在当前类中查找
	if props, ok := st.ClassProperties[className]; ok {
		if sig, ok := props[propName]; ok {
			return sig
		}
	}
	
	// 在父类中查找 - 先直接查找
	if parentName, ok := st.ClassParents[className]; ok && parentName != "" {
		return st.GetProperty(parentName, propName)
	}
	
	// 如果类名包含命名空间分隔符，也尝试用基类名查找父类
	if strings.Contains(className, "\\") {
		parts := strings.Split(className, "\\")
		baseName := parts[len(parts)-1]
		if parentName, ok := st.ClassParents[baseName]; ok && parentName != "" {
			return st.GetProperty(parentName, propName)
		}
	}
	
	return nil
}

// GetMethodReturnType 获取方法返回类型
func (st *SymbolTable) GetMethodReturnType(className, methodName string, arity int) string {
	if sig := st.GetMethod(className, methodName, arity); sig != nil {
		return sig.ReturnType
	}
	return ""
}

// CollectFromFile 从 AST 文件收集符号
func (st *SymbolTable) CollectFromFile(file *ast.File) {
	namespace := ""
	if file.Namespace != nil {
		namespace = file.Namespace.Name
	}
	
	for _, decl := range file.Declarations {
		switch d := decl.(type) {
		case *ast.ClassDecl:
			st.collectFromClass(d, namespace)
		case *ast.InterfaceDecl:
			st.collectFromInterface(d, namespace)
		case *ast.EnumDecl:
			st.collectFromEnum(d, namespace)
		case *ast.TypeAliasDecl:
			st.collectFromTypeAlias(d, namespace)
		case *ast.NewTypeDecl:
			st.collectFromNewType(d, namespace)
		}
	}
}

// collectFromEnum 从枚举声明收集符号
func (st *SymbolTable) collectFromEnum(decl *ast.EnumDecl, namespace string) {
	enumName := decl.Name.Name
	if namespace != "" {
		enumName = namespace + "\\" + enumName
	}
	
	// 收集枚举值列表（用于穷尽性检查）
	values := make([]string, len(decl.Cases))
	for i, c := range decl.Cases {
		values[i] = c.Name.Name
	}
	st.EnumValues[enumName] = values
}

// collectFromTypeAlias 从类型别名声明收集符号
// 类型别名创建与目标类型完全兼容的新名称，可以互相替换使用
func (st *SymbolTable) collectFromTypeAlias(decl *ast.TypeAliasDecl, namespace string) {
	aliasName := decl.Name.Name
	if namespace != "" {
		aliasName = namespace + "\\" + aliasName
	}
	
	targetType := typeNodeToString(decl.AliasType)
	st.TypeAliases[aliasName] = targetType
}

// collectFromNewType 从新类型声明收集符号
// 新类型创建与基础类型不兼容的独立类型，需要显式转换
func (st *SymbolTable) collectFromNewType(decl *ast.NewTypeDecl, namespace string) {
	typeName := decl.Name.Name
	if namespace != "" {
		typeName = namespace + "\\" + typeName
	}
	
	baseType := typeNodeToString(decl.BaseType)
	st.NewTypes[typeName] = &NewTypeInfo{
		Name:     typeName,
		BaseType: baseType,
		Distinct: true, // 新类型总是独立的
	}
}

// ResolveTypeAlias 解析类型别名，返回实际类型
// 类型别名会递归解析到底层类型，但不会解析新类型（新类型保持独立）
func (st *SymbolTable) ResolveTypeAlias(typeName string) string {
	// 如果是新类型，不解析（新类型保持独立）
	if _, isNewType := st.NewTypes[typeName]; isNewType {
		return typeName
	}
	
	// 递归解析类型别名，防止循环引用（最多解析10层）
	for i := 0; i < 10; i++ {
		if resolved, ok := st.TypeAliases[typeName]; ok {
			typeName = resolved
		} else {
			break
		}
	}
	return typeName
}

// GetNewTypeInfo 获取新类型信息
func (st *SymbolTable) GetNewTypeInfo(typeName string) *NewTypeInfo {
	return st.NewTypes[typeName]
}

// IsNewType 判断类型是否是新类型（需要显式转换）
func (st *SymbolTable) IsNewType(typeName string) bool {
	_, ok := st.NewTypes[typeName]
	return ok
}

// ResolveToBaseType 解析新类型到底层基础类型（用于类型兼容性检查）
// 如果是新类型，返回其基础类型；如果是别名，递归解析；否则返回原类型
func (st *SymbolTable) ResolveToBaseType(typeName string) string {
	// 首先检查是否是新类型
	if newType, ok := st.NewTypes[typeName]; ok {
		// 新类型的基础类型可能也是别名，需要递归解析
		return st.ResolveTypeAlias(newType.BaseType)
	}
	
	// 否则解析别名
	return st.ResolveTypeAlias(typeName)
}

// GetEnumValues 获取枚举的所有值
func (st *SymbolTable) GetEnumValues(enumName string) []string {
	return st.EnumValues[enumName]
}

// IsEnumType 判断类型是否是枚举
func (st *SymbolTable) IsEnumType(typeName string) bool {
	_, ok := st.EnumValues[typeName]
	return ok
}

// collectFromClass 从类声明收集符号
func (st *SymbolTable) collectFromClass(decl *ast.ClassDecl, namespace string) {
	className := decl.Name.Name
	// 如果有命名空间，添加命名空间前缀
	if namespace != "" {
		className = namespace + "\\" + className
	}
	
	// 收集泛型类型参数（包括类型参数和 where 子句）
	var allTypeParams []*ast.TypeParameter
	allTypeParams = append(allTypeParams, decl.TypeParams...)
	allTypeParams = append(allTypeParams, decl.WhereClause...)
	
	if len(allTypeParams) > 0 {
		typeParams := make([]*TypeParamInfo, len(allTypeParams))
		for i, tp := range allTypeParams {
			extendsType := ""
			if tp.Constraint != nil {
				extendsType = typeNodeToString(tp.Constraint)
			}
			var implementsTypes []string
			for _, implType := range tp.ImplementsTypes {
				implementsTypes = append(implementsTypes, typeNodeToString(implType))
			}
			typeParams[i] = &TypeParamInfo{
				Name:            tp.Name.Name,
				ExtendsType:     extendsType,
				ImplementsTypes: implementsTypes,
			}
		}
		st.ClassSignatures[className] = &ClassSignature{
			Name:       className,
			TypeParams: typeParams,
		}
	}
	
	// 注册继承关系
	if decl.Extends != nil {
		// 提取基类名（去除泛型参数）
		parentBaseName := extractBaseTypeName(decl.Extends.Name)
		st.RegisterClassParent(className, parentBaseName)
	}
	
	// 收集属性
	for _, prop := range decl.Properties {
		propType := "dynamic"
		if prop.Type != nil {
			propType = typeNodeToString(prop.Type)
		}
		st.RegisterProperty(&PropertySignature{
			ClassName: className,
			PropName:  prop.Name.Name,
			Type:      propType,
			IsStatic:  prop.Static,
		})
	}
	
	// 收集方法
	for _, method := range decl.Methods {
		// 收集方法的泛型类型参数
		var methodTypeParams []string
		for _, tp := range method.TypeParams {
			methodTypeParams = append(methodTypeParams, tp.Name.Name)
		}
		
		paramNames := make([]string, len(method.Parameters))
		paramTypes := make([]string, len(method.Parameters))
		minArity := len(method.Parameters)
		
		for i, param := range method.Parameters {
			// 收集参数名称（去掉$前缀）
			paramNames[i] = param.Name.Name
			if param.Type != nil {
				paramTypes[i] = typeNodeToString(param.Type)
			} else {
				paramTypes[i] = "dynamic"
			}
			if param.Default != nil && minArity == len(method.Parameters) {
				minArity = i
			}
		}
		
		returnType := "void"
		if method.ReturnType != nil {
			returnType = typeNodeToString(method.ReturnType)
		}
		
		st.RegisterMethod(&MethodSignature{
			ClassName:  className,
			MethodName: method.Name.Name,
			TypeParams: methodTypeParams,
			ParamNames: paramNames,
			ParamTypes: paramTypes,
			ReturnType: returnType,
			MinArity:   minArity,
			IsStatic:   method.Static,
		})
	}
	
	// 收集类实现的接口（用于类型收窄和 is 检查）
	if len(decl.Implements) > 0 {
		interfaces := make([]string, len(decl.Implements))
		for i, iface := range decl.Implements {
			interfaces[i] = extractBaseTypeName(typeNodeToString(iface))
		}
		st.ClassInterfaces[className] = interfaces
	}
}

// collectFromInterface 从接口声明收集符号
func (st *SymbolTable) collectFromInterface(decl *ast.InterfaceDecl, namespace string) {
	interfaceName := decl.Name.Name
	// 如果有命名空间，添加命名空间前缀
	if namespace != "" {
		interfaceName = namespace + "\\" + interfaceName
	}
	
	// 收集泛型类型参数（包括类型参数和 where 子句）
	var allTypeParams []*ast.TypeParameter
	allTypeParams = append(allTypeParams, decl.TypeParams...)
	allTypeParams = append(allTypeParams, decl.WhereClause...)
	
	if len(allTypeParams) > 0 {
		typeParams := make([]*TypeParamInfo, len(allTypeParams))
		for i, tp := range allTypeParams {
			extendsType := ""
			if tp.Constraint != nil {
				extendsType = typeNodeToString(tp.Constraint)
			}
			var implementsTypes []string
			for _, implType := range tp.ImplementsTypes {
				implementsTypes = append(implementsTypes, typeNodeToString(implType))
			}
			typeParams[i] = &TypeParamInfo{
				Name:            tp.Name.Name,
				ExtendsType:     extendsType,
				ImplementsTypes: implementsTypes,
			}
		}
		st.InterfaceSigs[interfaceName] = &InterfaceSignature{
			Name:       interfaceName,
			TypeParams: typeParams,
		}
	}
	
	// 收集接口方法
	for _, method := range decl.Methods {
		// 收集方法的泛型类型参数
		var methodTypeParams []string
		for _, tp := range method.TypeParams {
			methodTypeParams = append(methodTypeParams, tp.Name.Name)
		}
		
		paramNames := make([]string, len(method.Parameters))
		paramTypes := make([]string, len(method.Parameters))
		for i, param := range method.Parameters {
			paramNames[i] = param.Name.Name
			if param.Type != nil {
				paramTypes[i] = typeNodeToString(param.Type)
			} else {
				paramTypes[i] = "dynamic"
			}
		}
		
		returnType := "void"
		if method.ReturnType != nil {
			returnType = typeNodeToString(method.ReturnType)
		}
		
		st.RegisterMethod(&MethodSignature{
			ClassName:  interfaceName,
			MethodName: method.Name.Name,
			TypeParams: methodTypeParams,
			ParamNames: paramNames,
			ParamTypes: paramTypes,
			ReturnType: returnType,
			IsStatic:   method.Static,
		})
	}
}

// typeNodeToString 将类型节点转换为字符串
func typeNodeToString(t ast.TypeNode) string {
	if t == nil {
		return "dynamic"
	}
	
	switch typ := t.(type) {
	case *ast.SimpleType:
		return typ.Name
	case *ast.ClassType:
		return typ.Name.Literal
	case *ast.ArrayType:
		elemType := typeNodeToString(typ.ElementType)
		if typ.Size != nil {
			return elemType + "[N]" // 定长数组
		}
		return elemType + "[]"
	case *ast.MapType:
		keyType := typeNodeToString(typ.KeyType)
		valueType := typeNodeToString(typ.ValueType)
		return "map[" + keyType + "]" + valueType
	case *ast.NullableType:
		inner := typeNodeToString(typ.Inner)
		return inner + "|null"
	case *ast.UnionType:
		var parts []string
		for _, t := range typ.Types {
			parts = append(parts, typeNodeToString(t))
		}
		return strings.Join(parts, "|")
	case *ast.TupleType:
		var parts []string
		for _, t := range typ.Types {
			parts = append(parts, typeNodeToString(t))
		}
		return "(" + strings.Join(parts, ", ") + ")"
	case *ast.NullType:
		return "null"
	case *ast.FuncType:
		var params []string
		for _, p := range typ.Params {
			params = append(params, typeNodeToString(p))
		}
		ret := "void"
		if typ.ReturnType != nil {
			ret = typeNodeToString(typ.ReturnType)
		}
		return "func(" + strings.Join(params, ", ") + "): " + ret
	case *ast.GenericType:
		base := typeNodeToString(typ.BaseType)
		var args []string
		for _, arg := range typ.TypeArgs {
			args = append(args, typeNodeToString(arg))
		}
		return base + "<" + strings.Join(args, ", ") + ">"
	case *ast.TypeParameter:
		// 类型参数直接返回其名称
		return typ.Name.Name
	default:
		return "dynamic"
	}
}

// ValidateTypeConstraint 验证类型参数是否满足 extends 约束
func (st *SymbolTable) ValidateTypeConstraint(typeArg, constraint string) bool {
	if constraint == "" {
		return true // 无约束，任何类型都满足
	}
	
	// 提取基类名（去除泛型参数）
	baseTypeArg := extractBaseTypeName(typeArg)
	baseConstraint := extractBaseTypeName(constraint)
	
	// 完全匹配
	if baseTypeArg == baseConstraint {
		return true
	}
	
	// 检查继承关系：typeArg 是否是 constraint 的子类
	current := baseTypeArg
	for {
		parent, ok := st.ClassParents[current]
		if !ok || parent == "" {
			break
		}
		if parent == baseConstraint {
			return true
		}
		current = parent
	}
	
	return false
}

// CheckImplements 检查类型是否实现了指定接口（向后兼容方法）
func (st *SymbolTable) CheckImplements(typeName, interfaceName string) bool {
	err := st.ValidateImplements(typeName, interfaceName)
	return err == nil
}

// ValidateImplements 验证类型是否实现了指定接口，返回详细错误信息
func (st *SymbolTable) ValidateImplements(typeName, interfaceName string) error {
	// 提取基类名
	baseTypeName := extractBaseTypeName(typeName)
	baseInterfaceName := extractBaseTypeName(interfaceName)
	
	// 首先检查 ClassInterfaces 表（从类声明的 implements 收集）
	if interfaces, ok := st.ClassInterfaces[baseTypeName]; ok {
		for _, iface := range interfaces {
			if iface == baseInterfaceName {
				// 即使声明了实现，也要验证方法签名
				// 继续执行验证逻辑
				break
			}
		}
	}
	
	// 获取接口的所有方法签名
	interfaceMethods, ok := st.ClassMethods[baseInterfaceName]
	if !ok {
		// 接口不存在，返回错误
		return fmt.Errorf("interface '%s' not found", baseInterfaceName)
	}
	
	// 获取类的方法签名
	classMethods, ok := st.ClassMethods[baseTypeName]
	if !ok {
		return fmt.Errorf("class '%s' has no methods", baseTypeName)
	}
	
	// 验证接口的每个方法是否在类中都有实现，且签名匹配
	for methodName, interfaceMethodSigs := range interfaceMethods {
		classMethodSigs, hasMethod := classMethods[methodName]
		if !hasMethod || len(classMethodSigs) == 0 {
			return fmt.Errorf("method '%s' not implemented", methodName)
		}
		
		// 对每个接口方法签名，查找匹配的类方法签名
		for _, interfaceSig := range interfaceMethodSigs {
			matchFound := false
			for _, classSig := range classMethodSigs {
				if st.compareMethodSignatures(interfaceSig, classSig) {
					matchFound = true
					break
				}
			}
			if !matchFound {
				// 没有找到匹配的方法，返回详细错误
				return fmt.Errorf("method '%s' signature mismatch", methodName)
			}
		}
	}
	
	return nil
}

// compareMethodSignatures 比较两个方法签名是否兼容
// 用于验证类方法是否满足接口方法的签名要求
func (st *SymbolTable) compareMethodSignatures(interfaceMethod, classMethod *MethodSignature) bool {
	// 1. 检查静态/实例方法匹配
	if interfaceMethod.IsStatic != classMethod.IsStatic {
		return false
	}
	
	// 2. 检查参数数量
	if len(interfaceMethod.ParamTypes) != len(classMethod.ParamTypes) {
		// 如果接口方法有默认参数，允许类的参数数量在 MinArity 到 ParamTypes 长度之间
		if interfaceMethod.MinArity > 0 {
			classArity := len(classMethod.ParamTypes)
			if classArity < interfaceMethod.MinArity || classArity > len(interfaceMethod.ParamTypes) {
				return false
			}
		} else {
			return false
		}
	}
	
	// 3. 检查参数类型匹配
	// 类的参数类型必须与接口兼容（协变）
	minParams := len(interfaceMethod.ParamTypes)
	if len(classMethod.ParamTypes) < minParams {
		minParams = len(classMethod.ParamTypes)
	}
	for i := 0; i < minParams; i++ {
		// 接口参数类型应该是类参数类型的超类型（逆变）
		// 即：类方法参数应该可以接受接口方法参数
		if !st.IsTypeCompatible(interfaceMethod.ParamTypes[i], classMethod.ParamTypes[i]) {
			// 对于基本类型，需要严格匹配
			if interfaceMethod.ParamTypes[i] != classMethod.ParamTypes[i] {
				return false
			}
		}
	}
	
	// 4. 检查返回类型匹配（协变）
	// 类方法的返回类型必须是接口方法返回类型的子类型
	if interfaceMethod.ReturnType != "void" {
		if classMethod.ReturnType == "void" {
			return false
		}
		// 返回类型协变：类方法返回类型必须是接口方法返回类型的子类型
		if !st.IsTypeCompatible(classMethod.ReturnType, interfaceMethod.ReturnType) {
			return false
		}
	} else {
		// 接口方法返回 void，类方法也必须返回 void
		if classMethod.ReturnType != "void" {
			return false
		}
	}
	
	return true
}

// GetClassSignature 获取类的泛型签名
func (st *SymbolTable) GetClassSignature(className string) *ClassSignature {
	baseName := extractBaseTypeName(className)
	return st.ClassSignatures[baseName]
}

// IsTypeCompatible 检查 actualType 是否与 targetType 兼容
// 用于 is 表达式和类型收窄
func (st *SymbolTable) IsTypeCompatible(actualType, targetType string) bool {
	// 解析类型别名
	actualType = st.ResolveTypeAlias(actualType)
	targetType = st.ResolveTypeAlias(targetType)
	
	// 完全匹配
	if actualType == targetType {
		return true
	}
	
	// any 类型与任何类型兼容
	if actualType == "dynamic" || targetType == "dynamic" {
		return true
	}
	
	// null 类型检查
	if actualType == "null" && strings.Contains(targetType, "|null") {
		return true
	}
	
	// 联合类型检查：actualType 在 targetType 的联合类型中
	if strings.Contains(targetType, "|") {
		parts := strings.Split(targetType, "|")
		for _, p := range parts {
			if strings.TrimSpace(p) == actualType {
				return true
			}
		}
	}
	
	// 检查继承关系
	if st.ValidateTypeConstraint(actualType, targetType) {
		return true
	}
	
	// 检查接口实现
	if st.CheckImplements(actualType, targetType) {
		return true
	}
	
	return false
}

// GetClassInterfaces 获取类实现的接口列表
func (st *SymbolTable) GetClassInterfaces(className string) []string {
	baseName := extractBaseTypeName(className)
	return st.ClassInterfaces[baseName]
}

// NarrowType 类型收窄：根据类型检查条件收窄变量类型
// 返回收窄后的类型
func (st *SymbolTable) NarrowType(originalType, checkType string, positive bool) string {
	originalType = st.ResolveTypeAlias(originalType)
	checkType = st.ResolveTypeAlias(checkType)
	
	if positive {
		// 正向收窄：如果检查通过，类型变为检查类型
		// 例如：if ($x is string) { /* $x 是 string */ }
		if st.IsTypeCompatible(checkType, originalType) {
			return checkType
		}
		// 如果是联合类型，移除不兼容的部分
		if strings.Contains(originalType, "|") {
			parts := strings.Split(originalType, "|")
			var compatible []string
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if st.IsTypeCompatible(checkType, p) || p == checkType {
					compatible = append(compatible, checkType)
					break
				}
			}
			if len(compatible) == 1 {
				return compatible[0]
			}
		}
		return checkType
	} else {
		// 反向收窄：如果检查失败，从联合类型中移除检查类型
		// 例如：if (!($x is string)) { /* $x 不是 string */ }
		if strings.Contains(originalType, "|") {
			parts := strings.Split(originalType, "|")
			var remaining []string
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p != checkType {
					remaining = append(remaining, p)
				}
			}
			if len(remaining) == 1 {
				return remaining[0]
			}
			if len(remaining) > 1 {
				return strings.Join(remaining, "|")
			}
		}
		return originalType
	}
}

