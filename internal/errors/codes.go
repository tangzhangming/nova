// Package errors 提供 Sola 语言的错误处理系统
package errors

// ============================================================================
// 错误级别
// ============================================================================

// Level 错误级别
type Level int

const (
	LevelError   Level = iota // 错误
	LevelWarning              // 警告
	LevelNote                 // 提示
	LevelHelp                 // 帮助
)

func (l Level) String() string {
	switch l {
	case LevelError:
		return "error"
	case LevelWarning:
		return "warning"
	case LevelNote:
		return "note"
	case LevelHelp:
		return "help"
	default:
		return "unknown"
	}
}

// ============================================================================
// 编译器错误码 (E 开头)
// ============================================================================

// 编译器错误码常量
const (
	// E0001-E0099: 语法错误
	E0001 = "E0001" // 语法错误
	E0002 = "E0002" // 意外的字符
	E0003 = "E0003" // 未闭合的字符串
	E0004 = "E0004" // 未闭合的注释
	E0005 = "E0005" // 无效的数字
	E0006 = "E0006" // 期望的 token
	E0007 = "E0007" // 意外的 token

	// E0100-E0199: 变量错误
	E0100 = "E0100" // 未定义的变量
	E0101 = "E0101" // 变量重复声明
	E0102 = "E0102" // 变量未声明就使用
	E0103 = "E0103" // 局部变量过多
	E0104 = "E0104" // 闭包中未使用 use 捕获外部变量

	// E0200-E0299: 类型错误
	E0200 = "E0200" // 类型不匹配
	E0201 = "E0201" // 无法推断类型
	E0202 = "E0202" // 赋值类型不兼容
	E0203 = "E0203" // 返回类型不匹配
	E0204 = "E0204" // 返回值数量不匹配
	E0205 = "E0205" // 二元运算符类型不兼容
	E0206 = "E0206" // 联合类型不匹配
	E0207 = "E0207" // 无法确定索引目标类型

	// E0300-E0399: 函数错误
	E0300 = "E0300" // 未定义的函数
	E0301 = "E0301" // 参数数量错误（过少）
	E0302 = "E0302" // 参数数量错误（过多）
	E0303 = "E0303" // 参数类型错误
	E0304 = "E0304" // break 在循环外
	E0305 = "E0305" // continue 在循环外
	E0306 = "E0306" // 命名参数错误

	// E0400-E0499: 类/对象错误
	E0400 = "E0400" // 未定义的类
	E0401 = "E0401" // 未定义的方法
	E0402 = "E0402" // 未定义的属性
	E0403 = "E0403" // 未定义的静态成员
	E0404 = "E0404" // 无效的静态访问
	E0405 = "E0405" // self 在类外使用

	// E0500-E0599: 泛型错误
	E0500 = "E0500" // 泛型约束不满足
	E0501 = "E0501" // 泛型参数数量错误
	E0502 = "E0502" // 需要类型参数
	E0503 = "E0503" // 重复的类型参数

	// E0600-E0699: 数组/Map 错误
	E0600 = "E0600" // 数组大小必须是常量
	E0601 = "E0601" // 数组大小必须非负
	E0602 = "E0602" // 数组初始化元素过多
	E0603 = "E0603" // Map 键类型不一致
	E0604 = "E0604" // Map 值类型不一致

	// E0700-E0799: 原生函数错误
	E0700 = "E0700" // 原生函数受限
)

// ============================================================================
// 运行时错误码 (R 开头)
// ============================================================================

const (
	// R0001-R0099: 通用运行时错误
	R0001 = "R0001" // 未捕获的异常
	R0002 = "R0002" // 未知操作码
	R0003 = "R0003" // 指令指针越界

	// R0100-R0199: 数组/集合错误
	R0100 = "R0100" // 数组索引越界
	R0101 = "R0101" // Map 键不存在
	R0102 = "R0102" // 需要数组或 Map
	R0103 = "R0103" // 需要迭代器

	// R0200-R0299: 数值错误
	R0200 = "R0200" // 除以零
	R0201 = "R0201" // 操作数必须是数字
	R0202 = "R0202" // 浮点数不支持取模

	// R0300-R0399: 类型/对象错误
	R0300 = "R0300" // 空引用
	R0301 = "R0301" // 类型转换失败
	R0302 = "R0302" // 只有对象才有字段
	R0303 = "R0303" // 只有对象才有方法
	R0304 = "R0304" // 未定义的类
	R0305 = "R0305" // 未定义的方法
	R0306 = "R0306" // 未定义的静态方法
	R0307 = "R0307" // 未定义的枚举值
	R0308 = "R0308" // 只能调用函数

	// R0400-R0499: 资源/限制错误
	R0400 = "R0400" // 栈溢出
	R0401 = "R0401" // 执行超时/死循环
	R0402 = "R0402" // 调用栈过深

	// R0500-R0599: 变量错误
	R0500 = "R0500" // 未定义的变量
	R0501 = "R0501" // 参数数量错误
)

// ============================================================================
// 错误码信息
// ============================================================================

// ErrorInfo 错误码信息
type ErrorInfo struct {
	Code        string // 错误码
	Level       Level  // 错误级别
	MessageID   string // i18n 消息 ID
	Category    string // 错误分类
	DocURL      string // 文档链接（可选）
}

// compilerErrors 编译器错误码信息表
var compilerErrors = map[string]ErrorInfo{
	// 语法错误
	E0001: {E0001, LevelError, "error.syntax", "syntax", ""},
	E0002: {E0002, LevelError, "lexer.unexpected_char", "syntax", ""},
	E0003: {E0003, LevelError, "lexer.unterminated_string", "syntax", ""},
	E0004: {E0004, LevelError, "lexer.unterminated_comment", "syntax", ""},
	E0005: {E0005, LevelError, "lexer.invalid_number", "syntax", ""},
	E0006: {E0006, LevelError, "parser.expected_token", "syntax", ""},
	E0007: {E0007, LevelError, "parser.unexpected_token", "syntax", ""},

	// 变量错误
	E0100: {E0100, LevelError, "compiler.undefined_variable", "variable", ""},
	E0101: {E0101, LevelError, "compiler.variable_redeclared", "variable", ""},
	E0102: {E0102, LevelError, "compiler.undeclared_variable", "variable", ""},
	E0103: {E0103, LevelError, "compiler.too_many_locals", "variable", ""},
	E0104: {E0104, LevelError, "compiler.undefined_variable", "variable", ""},

	// 类型错误
	E0200: {E0200, LevelError, "compiler.type_mismatch", "type", ""},
	E0201: {E0201, LevelError, "compiler.type_cannot_infer", "type", ""},
	E0202: {E0202, LevelError, "compiler.cannot_assign", "type", ""},
	E0203: {E0203, LevelError, "compiler.return_type_mismatch", "type", ""},
	E0204: {E0204, LevelError, "compiler.return_count_mismatch", "type", ""},
	E0205: {E0205, LevelError, "compiler.invalid_binary_op", "type", ""},
	E0206: {E0206, LevelError, "compiler.union_type_mismatch", "type", ""},
	E0207: {E0207, LevelError, "compiler.index_target_unknown", "type", ""},

	// 函数错误
	E0300: {E0300, LevelError, "compiler.function_not_found", "function", ""},
	E0301: {E0301, LevelError, "vm.argument_count_min", "function", ""},
	E0302: {E0302, LevelError, "vm.argument_count_max", "function", ""},
	E0303: {E0303, LevelError, "compiler.type_mismatch", "function", ""},
	E0304: {E0304, LevelError, "compiler.break_outside_loop", "function", ""},
	E0305: {E0305, LevelError, "compiler.continue_outside_loop", "function", ""},
	E0306: {E0306, LevelError, "compiler.named_param_error", "function", ""},

	// 类/对象错误
	E0400: {E0400, LevelError, "vm.undefined_class", "class", ""},
	E0401: {E0401, LevelError, "compiler.method_not_found", "class", ""},
	E0402: {E0402, LevelError, "compiler.property_not_found", "class", ""},
	E0403: {E0403, LevelError, "compiler.static_member_not_found", "class", ""},
	E0404: {E0404, LevelError, "compiler.invalid_static_access", "class", ""},
	E0405: {E0405, LevelError, "compiler.self_outside_class", "class", ""},

	// 泛型错误
	E0500: {E0500, LevelError, "compiler.generic_constraint_violated", "generic", ""},
	E0501: {E0501, LevelError, "compiler.generic_type_arg_count", "generic", ""},
	E0502: {E0502, LevelError, "compiler.generic_type_required", "generic", ""},
	E0503: {E0503, LevelError, "compiler.duplicate_type_param", "generic", ""},

	// 数组/Map 错误
	E0600: {E0600, LevelError, "compiler.array_size_not_const", "array", ""},
	E0601: {E0601, LevelError, "compiler.array_size_negative", "array", ""},
	E0602: {E0602, LevelError, "compiler.array_too_many_elements", "array", ""},
	E0603: {E0603, LevelError, "compiler.map_key_type_mismatch", "array", ""},
	E0604: {E0604, LevelError, "compiler.map_value_type_mismatch", "array", ""},

	// 原生函数错误
	E0700: {E0700, LevelError, "compiler.native_func_restricted", "native", ""},
}

// runtimeErrors 运行时错误码信息表
var runtimeErrors = map[string]ErrorInfo{
	// 通用错误
	R0001: {R0001, LevelError, "vm.uncaught_exception", "runtime", ""},
	R0002: {R0002, LevelError, "vm.unknown_opcode", "runtime", ""},
	R0003: {R0003, LevelError, "vm.ip_out_of_bounds", "runtime", ""},

	// 数组/集合错误
	R0100: {R0100, LevelError, "vm.array_index_out_of_bounds", "array", ""},
	R0101: {R0101, LevelError, "vm.map_key_not_found", "array", ""},
	R0102: {R0102, LevelError, "vm.subscript_requires_array", "array", ""},
	R0103: {R0103, LevelError, "vm.expected_iterator", "array", ""},

	// 数值错误
	R0200: {R0200, LevelError, "vm.division_by_zero", "numeric", ""},
	R0201: {R0201, LevelError, "vm.operand_must_be_number", "numeric", ""},
	R0202: {R0202, LevelError, "vm.modulo_not_for_floats", "numeric", ""},

	// 类型/对象错误
	R0300: {R0300, LevelError, "vm.null_reference", "type", ""},
	R0301: {R0301, LevelError, "vm.cannot_cast", "type", ""},
	R0302: {R0302, LevelError, "vm.only_objects_have_fields", "type", ""},
	R0303: {R0303, LevelError, "vm.only_objects_have_methods", "type", ""},
	R0304: {R0304, LevelError, "vm.undefined_class", "type", ""},
	R0305: {R0305, LevelError, "vm.undefined_method", "type", ""},
	R0306: {R0306, LevelError, "vm.undefined_static_method", "type", ""},
	R0307: {R0307, LevelError, "vm.undefined_enum_case", "type", ""},
	R0308: {R0308, LevelError, "vm.can_only_call_functions", "type", ""},

	// 资源/限制错误
	R0400: {R0400, LevelError, "vm.stack_overflow", "resource", ""},
	R0401: {R0401, LevelError, "vm.execution_limit", "resource", ""},
	R0402: {R0402, LevelError, "vm.call_stack_overflow", "resource", ""},

	// 变量错误
	R0500: {R0500, LevelError, "vm.undefined_var", "variable", ""},
	R0501: {R0501, LevelError, "vm.argument_count_min", "variable", ""},
}

// GetCompilerErrorInfo 获取编译器错误信息
func GetCompilerErrorInfo(code string) (ErrorInfo, bool) {
	info, ok := compilerErrors[code]
	return info, ok
}

// GetRuntimeErrorInfo 获取运行时错误信息
func GetRuntimeErrorInfo(code string) (ErrorInfo, bool) {
	info, ok := runtimeErrors[code]
	return info, ok
}

// IsCompilerError 检查是否为编译器错误码
func IsCompilerError(code string) bool {
	_, ok := compilerErrors[code]
	return ok
}

// IsRuntimeError 检查是否为运行时错误码
func IsRuntimeError(code string) bool {
	_, ok := runtimeErrors[code]
	return ok
}



