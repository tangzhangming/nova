package compiler

import (
	"strings"

	"github.com/tangzhangming/nova/internal/ast"
)

// FunctionSignature 函数签名
type FunctionSignature struct {
	Name       string   // 函数名
	ParamTypes []string // 参数类型列表
	ReturnType string   // 返回类型 (可以是 "int", "string", "(int, string)" 等)
	MinArity   int      // 最小参数数量（考虑默认参数）
	IsVariadic bool     // 是否是可变参数
}

// MethodSignature 方法签名
type MethodSignature struct {
	ClassName  string   // 类名
	MethodName string   // 方法名
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

// SymbolTable 符号表
type SymbolTable struct {
	Functions       map[string]*FunctionSignature                      // 全局函数: 函数名 -> 签名
	ClassMethods    map[string]map[string][]*MethodSignature           // 类方法: 类名 -> 方法名 -> 签名列表（支持重载）
	ClassProperties map[string]map[string]*PropertySignature           // 类属性: 类名 -> 属性名 -> 签名
	GlobalVars      map[string]string                                  // 全局变量类型
	ClassParents    map[string]string                                  // 类继承关系: 子类名 -> 父类名
}

// NewSymbolTable 创建符号表
func NewSymbolTable() *SymbolTable {
	st := &SymbolTable{
		Functions:       make(map[string]*FunctionSignature),
		ClassMethods:    make(map[string]map[string][]*MethodSignature),
		ClassProperties: make(map[string]map[string]*PropertySignature),
		GlobalVars:      make(map[string]string),
		ClassParents:    make(map[string]string),
	}
	// 注册内置函数签名
	st.registerBuiltinFunctions()
	// 注册内置类型方法
	st.registerBuiltinTypeMethods()
	return st
}

// registerBuiltinFunctions 注册内置函数签名
func (st *SymbolTable) registerBuiltinFunctions() {
	// 核心内置函数
	st.Functions["print"] = &FunctionSignature{Name: "print", ParamTypes: []string{"any"}, ReturnType: "void", IsVariadic: true}
	st.Functions["echo"] = &FunctionSignature{Name: "echo", ParamTypes: []string{"any"}, ReturnType: "void"}
	st.Functions["len"] = &FunctionSignature{Name: "len", ParamTypes: []string{"any"}, ReturnType: "int"}
	st.Functions["type"] = &FunctionSignature{Name: "type", ParamTypes: []string{"any"}, ReturnType: "string"}
	st.Functions["isset"] = &FunctionSignature{Name: "isset", ParamTypes: []string{"any"}, ReturnType: "bool"}
	st.Functions["empty"] = &FunctionSignature{Name: "empty", ParamTypes: []string{"any"}, ReturnType: "bool"}
	st.Functions["unset"] = &FunctionSignature{Name: "unset", ParamTypes: []string{"any"}, ReturnType: "void"}
	st.Functions["var_dump"] = &FunctionSignature{Name: "var_dump", ParamTypes: []string{"any"}, ReturnType: "void"}
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

	// Base64 函数 (native_base64_*)
	st.Functions["native_base64_encode"] = &FunctionSignature{Name: "native_base64_encode", ParamTypes: []string{"string"}, ReturnType: "string"}
	st.Functions["native_base64_decode"] = &FunctionSignature{Name: "native_base64_decode", ParamTypes: []string{"string"}, ReturnType: "string"}
	st.Functions["native_base64_encode_bytes"] = &FunctionSignature{Name: "native_base64_encode_bytes", ParamTypes: []string{"byte[]"}, ReturnType: "string"}
	st.Functions["native_base64_decode_to_bytes"] = &FunctionSignature{Name: "native_base64_decode_to_bytes", ParamTypes: []string{"string"}, ReturnType: "byte[]"}

	// TCP 函数 (native_tcp_*)
	st.Functions["native_tcp_connect"] = &FunctionSignature{Name: "native_tcp_connect", ParamTypes: []string{"string", "int"}, ReturnType: "int"}
	st.Functions["native_tcp_close"] = &FunctionSignature{Name: "native_tcp_close", ParamTypes: []string{"int"}, ReturnType: "void"}
	st.Functions["native_tcp_write"] = &FunctionSignature{Name: "native_tcp_write", ParamTypes: []string{"int", "byte[]"}, ReturnType: "int"}
	st.Functions["native_tcp_read"] = &FunctionSignature{Name: "native_tcp_read", ParamTypes: []string{"int", "int"}, ReturnType: "byte[]"}
	st.Functions["native_tcp_read_line"] = &FunctionSignature{Name: "native_tcp_read_line", ParamTypes: []string{"int"}, ReturnType: "string"}
	st.Functions["native_tcp_set_timeout"] = &FunctionSignature{Name: "native_tcp_set_timeout", ParamTypes: []string{"int", "int"}, ReturnType: "void"}
	st.Functions["native_tcp_listen"] = &FunctionSignature{Name: "native_tcp_listen", ParamTypes: []string{"string", "int"}, ReturnType: "int"}
	st.Functions["native_tcp_accept"] = &FunctionSignature{Name: "native_tcp_accept", ParamTypes: []string{"int"}, ReturnType: "int"}
	st.Functions["native_tcp_get_remote_addr"] = &FunctionSignature{Name: "native_tcp_get_remote_addr", ParamTypes: []string{"int"}, ReturnType: "string"}
	st.Functions["native_tcp_get_local_addr"] = &FunctionSignature{Name: "native_tcp_get_local_addr", ParamTypes: []string{"int"}, ReturnType: "string"}

	// Bytes 函数 (native_bytes_*)
	st.Functions["native_bytes_new"] = &FunctionSignature{Name: "native_bytes_new", ParamTypes: []string{"int"}, ReturnType: "byte[]"}
	st.Functions["native_bytes_from_string"] = &FunctionSignature{Name: "native_bytes_from_string", ParamTypes: []string{"string"}, ReturnType: "byte[]"}
	st.Functions["native_bytes_to_string"] = &FunctionSignature{Name: "native_bytes_to_string", ParamTypes: []string{"byte[]"}, ReturnType: "string"}
	st.Functions["native_bytes_length"] = &FunctionSignature{Name: "native_bytes_length", ParamTypes: []string{"byte[]"}, ReturnType: "int"}
	st.Functions["native_bytes_slice"] = &FunctionSignature{Name: "native_bytes_slice", ParamTypes: []string{"byte[]", "int", "int"}, ReturnType: "byte[]"}
	st.Functions["native_bytes_concat"] = &FunctionSignature{Name: "native_bytes_concat", ParamTypes: []string{"byte[]", "byte[]"}, ReturnType: "byte[]"}
	st.Functions["native_bytes_copy"] = &FunctionSignature{Name: "native_bytes_copy", ParamTypes: []string{"byte[]"}, ReturnType: "byte[]"}
	st.Functions["native_bytes_equal"] = &FunctionSignature{Name: "native_bytes_equal", ParamTypes: []string{"byte[]", "byte[]"}, ReturnType: "bool"}
	st.Functions["native_bytes_index_of"] = &FunctionSignature{Name: "native_bytes_index_of", ParamTypes: []string{"byte[]", "byte[]"}, ReturnType: "int"}

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
	st.Functions["native_reflect_get_class"] = &FunctionSignature{Name: "native_reflect_get_class", ParamTypes: []string{"object"}, ReturnType: "string"}
	st.Functions["native_reflect_get_methods"] = &FunctionSignature{Name: "native_reflect_get_methods", ParamTypes: []string{"object"}, ReturnType: "string[]"}
	st.Functions["native_reflect_get_properties"] = &FunctionSignature{Name: "native_reflect_get_properties", ParamTypes: []string{"object"}, ReturnType: "string[]"}
	st.Functions["native_reflect_has_method"] = &FunctionSignature{Name: "native_reflect_has_method", ParamTypes: []string{"object", "string"}, ReturnType: "bool"}
	st.Functions["native_reflect_has_property"] = &FunctionSignature{Name: "native_reflect_has_property", ParamTypes: []string{"object", "string"}, ReturnType: "bool"}
	st.Functions["native_reflect_get_annotations"] = &FunctionSignature{Name: "native_reflect_get_annotations", ParamTypes: []string{"object"}, ReturnType: "map[string]any"}
	st.Functions["native_reflect_get_parent_class"] = &FunctionSignature{Name: "native_reflect_get_parent_class", ParamTypes: []string{"object"}, ReturnType: "string"}
	st.Functions["native_reflect_get_interfaces"] = &FunctionSignature{Name: "native_reflect_get_interfaces", ParamTypes: []string{"object"}, ReturnType: "string[]"}
	st.Functions["native_reflect_is_instance_of"] = &FunctionSignature{Name: "native_reflect_is_instance_of", ParamTypes: []string{"object", "string"}, ReturnType: "bool"}
	
	// typeof 函数
	st.Functions["typeof"] = &FunctionSignature{Name: "typeof", ParamTypes: []string{"any"}, ReturnType: "string"}
}

// registerBuiltinTypeMethods 注册内置类型的方法签名
func (st *SymbolTable) registerBuiltinTypeMethods() {
	// SuperArray 万能数组方法
	st.RegisterMethod(&MethodSignature{ClassName: "superarray", MethodName: "len", ParamTypes: []string{}, ReturnType: "int"})
	st.RegisterMethod(&MethodSignature{ClassName: "superarray", MethodName: "hasKey", ParamTypes: []string{"any"}, ReturnType: "bool"})
	st.RegisterMethod(&MethodSignature{ClassName: "superarray", MethodName: "get", ParamTypes: []string{"any"}, ReturnType: "any"})
	st.RegisterMethod(&MethodSignature{ClassName: "superarray", MethodName: "get", ParamTypes: []string{"any", "any"}, ReturnType: "any"})
	st.RegisterMethod(&MethodSignature{ClassName: "superarray", MethodName: "set", ParamTypes: []string{"any", "any"}, ReturnType: "superarray"})
	st.RegisterMethod(&MethodSignature{ClassName: "superarray", MethodName: "keys", ParamTypes: []string{}, ReturnType: "superarray"})
	st.RegisterMethod(&MethodSignature{ClassName: "superarray", MethodName: "values", ParamTypes: []string{}, ReturnType: "superarray"})
	st.RegisterMethod(&MethodSignature{ClassName: "superarray", MethodName: "push", ParamTypes: []string{"any"}, ReturnType: "superarray"})
	st.RegisterMethod(&MethodSignature{ClassName: "superarray", MethodName: "pop", ParamTypes: []string{}, ReturnType: "any"})
	st.RegisterMethod(&MethodSignature{ClassName: "superarray", MethodName: "shift", ParamTypes: []string{}, ReturnType: "any"})
	st.RegisterMethod(&MethodSignature{ClassName: "superarray", MethodName: "unshift", ParamTypes: []string{"any"}, ReturnType: "superarray"})
	st.RegisterMethod(&MethodSignature{ClassName: "superarray", MethodName: "merge", ParamTypes: []string{"superarray"}, ReturnType: "superarray"})
	st.RegisterMethod(&MethodSignature{ClassName: "superarray", MethodName: "slice", ParamTypes: []string{"int"}, ReturnType: "superarray"})
	st.RegisterMethod(&MethodSignature{ClassName: "superarray", MethodName: "slice", ParamTypes: []string{"int", "int"}, ReturnType: "superarray"})
	st.RegisterMethod(&MethodSignature{ClassName: "superarray", MethodName: "remove", ParamTypes: []string{"any"}, ReturnType: "bool"})
	st.RegisterMethod(&MethodSignature{ClassName: "superarray", MethodName: "clear", ParamTypes: []string{}, ReturnType: "void"})
	st.RegisterMethod(&MethodSignature{ClassName: "superarray", MethodName: "copy", ParamTypes: []string{}, ReturnType: "superarray"})
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
	return st.Functions[name]
}

// GetMethod 获取方法签名（按参数数量匹配）
func (st *SymbolTable) GetMethod(className, methodName string, arity int) *MethodSignature {
	// 首先在当前类中查找
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
	
	// 在父类中查找
	if parentName, ok := st.ClassParents[className]; ok && parentName != "" {
		return st.GetMethod(parentName, methodName, arity)
	}
	
	return nil
}

// GetProperty 获取属性签名
func (st *SymbolTable) GetProperty(className, propName string) *PropertySignature {
	// 首先在当前类中查找
	if props, ok := st.ClassProperties[className]; ok {
		if sig, ok := props[propName]; ok {
			return sig
		}
	}
	
	// 在父类中查找
	if parentName, ok := st.ClassParents[className]; ok && parentName != "" {
		return st.GetProperty(parentName, propName)
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
	for _, decl := range file.Declarations {
		switch d := decl.(type) {
		case *ast.ClassDecl:
			st.collectFromClass(d)
		case *ast.InterfaceDecl:
			st.collectFromInterface(d)
		}
	}
}

// collectFromClass 从类声明收集符号
func (st *SymbolTable) collectFromClass(decl *ast.ClassDecl) {
	className := decl.Name.Name
	
	// 注册继承关系
	if decl.Extends != nil {
		st.RegisterClassParent(className, decl.Extends.Name)
	}
	
	// 收集属性
	for _, prop := range decl.Properties {
		propType := "any"
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
		paramTypes := make([]string, len(method.Parameters))
		minArity := len(method.Parameters)
		
		for i, param := range method.Parameters {
			if param.Type != nil {
				paramTypes[i] = typeNodeToString(param.Type)
			} else {
				paramTypes[i] = "any"
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
			ParamTypes: paramTypes,
			ReturnType: returnType,
			MinArity:   minArity,
			IsStatic:   method.Static,
		})
	}
}

// collectFromInterface 从接口声明收集符号
func (st *SymbolTable) collectFromInterface(decl *ast.InterfaceDecl) {
	interfaceName := decl.Name.Name
	
	// 收集接口方法
	for _, method := range decl.Methods {
		paramTypes := make([]string, len(method.Parameters))
		for i, param := range method.Parameters {
			if param.Type != nil {
				paramTypes[i] = typeNodeToString(param.Type)
			} else {
				paramTypes[i] = "any"
			}
		}
		
		returnType := "void"
		if method.ReturnType != nil {
			returnType = typeNodeToString(method.ReturnType)
		}
		
		st.RegisterMethod(&MethodSignature{
			ClassName:  interfaceName,
			MethodName: method.Name.Name,
			ParamTypes: paramTypes,
			ReturnType: returnType,
			IsStatic:   method.Static,
		})
	}
}

// typeNodeToString 将类型节点转换为字符串
func typeNodeToString(t ast.TypeNode) string {
	if t == nil {
		return "any"
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
	default:
		return "any"
	}
}

