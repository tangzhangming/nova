package i18n

var messagesZH = map[string]string{
	// ========== 词法分析器 ==========
	ErrUnexpectedChar:      "意外字符 '%c'",
	ErrUnexpectedDoubleDot: "意外的 '..'",
	ErrUnterminatedComment: "未闭合的块注释",
	ErrUnterminatedString:  "未闭合的字符串",
	ErrUnterminatedInterp:  "未闭合的插值字符串",
	ErrExpectedVarName:     "'$' 后应该是变量名",
	ErrInvalidHexNumber:    "无效的十六进制数: %s",
	ErrInvalidBinaryNumber: "无效的二进制数: %s",
	ErrInvalidExponent:     "无效的数字: 需要指数部分",
	ErrInvalidFloat:        "无效的浮点数: %s",
	ErrInvalidInteger:      "无效的整数: %s",

	// ========== 语法分析器 ==========
	ErrExpectedType:         "需要类型",
	ErrVoidNotAllowed:       "'void' 不能作为返回类型，请省略返回类型",
	ErrExpectedToken:        "需要 %s",
	ErrUnexpectedToken:      "意外的符号: %s",
	ErrExpectedExpression:   "需要表达式",
	ErrExpectedStatement:    "需要语句",
	ErrExpectedClassName:    "需要类名",
	ErrExpectedMethodName:   "需要方法名",
	ErrExpectedPropertyName: "需要属性名",
	ErrExpectedParamName:    "需要参数名",
	ErrExpectedVarInUse:     "use 子句中需要变量",
	ErrExpectedCaseDefault:  "需要 'case' 或 'default'",
	ErrInvalidStaticAccess:  "无效的静态访问",
	ErrInvalidStaticMember:  "无效的静态成员",
	ErrChainedTypeCast:      "不允许链式类型断言",
	ErrInvalidAssignTarget:  "无效的赋值目标",

	// ========== 编译器 ==========
	ErrUnsupportedStmt:      "不支持的语句类型",
	ErrUnsupportedExpr:      "不支持的表达式类型",
	ErrBreakOutsideLoop:     "'break' 只能在循环内使用",
	ErrContinueOutsideLoop:  "'continue' 只能在循环内使用",
	ErrUndefinedVariable:    "未定义的变量 '$%s'（闭包中需使用 'use' 捕获外部变量）",
	ErrTooManyLocals:        "局部变量过多",
	ErrVariableRedeclared:   "变量在当前作用域已声明",
	ErrTypeMismatch:         "类型不匹配: 期望 %s 但得到 %s",
	ErrCannotAssign:         "不能将 %s 赋值给 %s 类型的变量",
	ErrReturnTypeMismatch:   "类型不匹配: 期望 %s 但得到 %s",
	ErrReturnCountMismatch:  "函数期望返回 %d 个值，但实际返回 %d 个",
	ErrNoReturnExpected:     "函数未声明返回类型，但返回了 %d 个值",
	ErrArraySizeNotConst:    "数组大小必须是编译时常量",
	ErrArraySizeNegative:    "数组大小必须是非负常量",
	ErrArrayTooManyElements: "数组初始化元素过多（最大 %d，实际 %d）",
	ErrCompoundAssignIndex:  "暂不支持数组元素的复合赋值",
	ErrInvalidStaticAccessC: "无效的静态访问",
	ErrInvalidBinaryOp:      "运算符 '%s' 不能用于 %s 和 %s 类型",
	ErrNativeFuncRestricted: "原生函数 '%s' 只能在标准库 (sola.*) 中调用",
	ErrMapKeyTypeMismatch:   "Map 键类型不一致: 期望 %s 但得到 %s",
	ErrMapValueTypeMismatch: "Map 值类型不一致: 期望 %s 但得到 %s",
	ErrCannotInferInterface: "无法推断接口 '%s' 的类型，需要显式声明类型",
	
	// 静态类型检查相关
	ErrTypeCannotInfer:      "无法推断表达式类型",
	ErrFunctionNotFound:     "未定义的函数 '%s'",
	ErrMethodNotFound:       "类型 '%s' 没有方法 '%s'（%d 个参数）",
	ErrUnionTypeMismatch:    "值类型 '%s' 不在联合类型 '%s' 中",
	ErrAllTypesMustBeKnown:  "静态类型检查要求所有类型必须在编译期确定",
	ErrPropertyNotFound:     "类型 '%s' 没有属性 '%s'",
	ErrVariableTypeUnknown:  "变量 '$%s' 的类型未知",
	ErrStaticMemberNotFound: "类 '%s' 没有静态成员 '%s'",
	ErrCannotInferVarType:   "无法推断变量 '$%s' 的类型，请显式声明类型",
	ErrIndexTargetUnknown:   "无法确定索引操作的目标类型",
	ErrUndeclaredVariable:   "变量 '$%s' 未声明，请使用 ':=' 声明变量",
	
	// 泛型相关
	ErrGenericTypeParamName:      "需要类型参数名",
	ErrGenericTypeArgCount:       "类型 '%s' 需要 %d 个类型参数，但提供了 %d 个",
	ErrGenericConstraintViolated: "类型 '%s' 不满足约束 '%s'",
	ErrGenericTypeRequired:       "泛型类型 '%s' 需要类型参数",
	ErrDuplicateTypeParam:        "重复的类型参数 '%s'",
	
	// 数组类型相关
	ErrSuperArrayNotCompatible:   "SuperArray（万能数组）与类型化数组 '%s' 不兼容，请使用类型化数组或显式转换",
	ErrArrayNotCompatible:        "类型化数组 '%s' 与 SuperArray（万能数组）不兼容，请使用 SuperArray 语法 [key => value]",
	
	// 穷尽性检查
	ErrSwitchNotExhaustive: "switch 语句未覆盖枚举 '%s' 的所有值，缺少: %s",
	
	// final 相关
	ErrCannotExtendFinalClass:    "不能继承 final 类 '%s'",
	ErrCannotOverrideFinalMethod: "不能重写类 '%s' 的 final 方法 '%s'",
	ErrFinalAndAbstractConflict:  "类不能同时是 final 和 abstract",
	ErrCannotAssignFinalProperty: "不能重新赋值 final 属性 '%s'",
	
	// 接口相关
	ErrInterfaceNotImplemented:      "类 '%s' 未实现接口 '%s'",
	ErrInterfaceMethodMissing:       "类 '%s' 未实现接口 '%s' 的方法 '%s'",
	ErrInterfaceMethodParamMismatch: "类 '%s' 的方法 '%s' 参数类型与接口 '%s' 不匹配（期望 %s，实际 %s）",
	ErrInterfaceMethodReturnMismatch: "类 '%s' 的方法 '%s' 返回类型与接口 '%s' 不兼容（期望 %s，实际 %s）",
	ErrInterfaceMethodStaticMismatch: "类 '%s' 的方法 '%s' 与接口 '%s' 的静态/实例属性不匹配",
	
	// 空安全检查相关
	ErrNullableAccess:          "不能访问可空类型 '%s' 的成员，请先检查 null 或使用安全调用 '?.'",
	ErrNullAssignment:          "不能将 null 赋值给非可空类型 '%s'，请使用可空类型 '%s|null'",
	ErrNullableArgument:        "将可空类型 '%s' 传递给非可空参数 '%s'，可能导致空指针错误",
	ErrNullableReturn:          "不能从返回类型为 '%s' 的函数返回 null",
	WarnUnreachableCode:        "检测到不可达代码",
	WarnUninitializedVariable:  "变量 '%s' 可能未初始化",
	
	// 类名解析相关
	ErrSelfOutsideClass: "不能在类外使用 self::class",

	// ========== 虚拟机 ==========
	ErrIPOutOfBounds:            "指令指针越界",
	ErrExecutionLimit:           "执行次数超限（可能是死循环？）",
	ErrUndefinedVar:             "未定义的变量 '%s'",
	ErrOperandMustBeNumber:      "操作数必须是数字",
	ErrDivisionByZero:           "除数不能为零",
	ErrModuloNotForFloats:       "浮点数不支持取模运算",
	ErrOperandsMustBeNumbers:    "操作数必须是数字",
	ErrOperandsMustBeComparable: "操作数必须可比较",
	ErrOnlyObjectsHaveFields:    "只有对象才有字段",
	ErrOnlyObjectsHaveMethods:   "只有对象才有方法",
	ErrUndefinedClass:           "未定义的类 '%s'",
	ErrArrayIndexOutOfBounds:    "数组索引 %d 越界（容量 %d）",
	ErrArrayIndexSimple:         "数组索引越界",
	ErrSubscriptRequiresArray:   "下标运算需要数组或 Map",
	ErrCanOnlyCallFunctions:     "只能调用函数",
	ErrArgumentCountMin:         "期望至少 %d 个参数，但只有 %d 个",
	ErrArgumentCountMax:         "期望最多 %d 个参数，但有 %d 个",
	ErrStackOverflow:            "栈溢出",
	ErrTypeError:                "类型错误: 期望 %s 但得到 %s",
	ErrCannotCast:               "不能将 %s 转换为 %s",
	ErrUnknownOpcode:            "未知操作码: %d",
	ErrForeachRequiresIterable:  "foreach 需要数组或 Map",
	ErrExpectedIterator:         "需要迭代器",
	ErrUndefinedMethod:          "未定义的方法 '%s'（%d 个参数）",
	ErrUndefinedStaticMethod:    "未定义的静态方法 '%s::%s'（%d 个参数）",
	ErrUndefinedEnumCase:        "未定义的枚举值 '%s::%s'",
	ErrLengthRequiresArray:      "length 需要数组",
	ErrLengthRequiresMap:        "length 需要 Map",
	ErrPushRequiresArray:        "push 需要数组",
	ErrHasRequiresArray:         "has 需要数组",
	ErrHasRequiresArrayOrMap:    "has() 需要数组或 Map",
	ErrSubscriptRequiresMap:     "下标运算需要 Map",
	ErrRuntimeError:             "运行时错误: %s",

	// ========== 运行时 ==========
	ErrFailedCreateLoader: "创建加载器失败: %v",
	ErrParseError:         "解析错误: %s",
	ErrParseFailed:        "解析失败",
	ErrParseFailedFor:     "%s 解析失败",
	ErrLoadFailed:         "加载 %s 失败: %v",
	ErrReadFailed:         "读取 %s 失败: %v",
	ErrCompileError:       "编译错误: %s",
	ErrCompileFailed:      "编译失败",
	ErrCompileFailedFor:   "%s 编译失败",

	// ========== 包加载器 ==========
	ErrGetExecutablePath:    "获取可执行文件路径失败: %v",
	ErrResolveSymlinks:      "解析符号链接失败: %v",
	ErrStdLibNotFound:       "标准库目录不存在: %s",
	ErrProjectConfigNotFound: "未找到项目配置文件 %s",
	ErrOpenProjectConfig:    "打开 %s 失败: %v",
	ErrReadProjectConfig:    "读取 %s 失败: %v",
	ErrStdLibNotConfigured:  "标准库未配置，无法导入: %s",
	ErrStdLibImportNotFound: "标准库模块未找到: %s（尝试路径: %s）",
	ErrImportNotFound:       "导入未找到: %s",

	// ========== 修复建议 ==========
	// 变量相关
	"suggestion.declare_variable":      "是否想要声明新变量？使用 `$%s := 值`",
	"suggestion.did_you_mean":          "是否想用 `$%s`？",
	"suggestion.use_assign_instead":    "如果要修改变量，使用 `$%s = 新值` 而不是 `:=`",
	"suggestion.rename_variable":       "考虑使用不同的变量名",
	"suggestion.use_clause":            "在闭包中使用外部变量需要 `use ($%s)`",

	// 类型相关
	"suggestion.convert_string_to_int":   "使用 `Str::toInt($var)` 将字符串转换为整数",
	"suggestion.convert_int_to_string":   "使用 `\"\" + $var` 或字符串插值 `\"${var}\"` 转换为字符串",
	"suggestion.convert_string_to_float": "使用 `Str::toFloat($var)` 将字符串转换为浮点数",
	"suggestion.convert_float_to_string": "使用 `\"\" + $var` 或字符串插值 `\"${var}\"` 转换为字符串",
	"suggestion.cast_float_to_int":       "使用 `(int)$var` 将浮点数转换为整数（会截断小数部分）",
	"suggestion.implicit_int_to_float":   "整数可以自动转换为浮点数",
	"suggestion.explicit_bool_check":     "使用显式的布尔表达式，如 `$var != 0` 或 `$var != \"\"`",
	"suggestion.wrap_in_array":           "使用 `[$var]` 将值包装成数组",
	"suggestion.explicit_type":           "为变量 `$%s` 添加显式类型声明",
	"suggestion.check_return_type":       "确保返回值类型为 `%s`",

	// 函数相关
	"suggestion.check_function_name":   "检查函数名 `%s` 是否拼写正确",
	"suggestion.did_you_mean_func":     "是否想用 `%s()`？",
	"suggestion.check_import":          "检查是否需要导入相关模块",
	"suggestion.add_arguments":         "添加 %d 个缺失的参数",
	"suggestion.remove_arguments":      "移除 %d 个多余的参数",
	"suggestion.break_only_in_loop":    "`break` 只能在 `for`、`while`、`foreach` 或 `switch` 中使用",
	"suggestion.use_return_instead":    "如果要退出函数，使用 `return`",
	"suggestion.continue_only_in_loop": "`continue` 只能在 `for`、`while` 或 `foreach` 循环中使用",

	// 类/对象相关
	"suggestion.check_method_name":   "检查方法名 `%s` 是否存在于类型 `%s`",
	"suggestion.did_you_mean_method": "是否想用 `%s()`？",
	"suggestion.check_property_name": "检查属性 `%s` 是否存在于类型 `%s`",

	// 运行时相关
	"suggestion.array_index_range":       "有效的索引范围是 [0, %d]",
	"suggestion.check_index_before_access": "访问数组前检查索引：`if ($i >= 0 && $i < len($arr))`",
	"suggestion.check_divisor":           "在除法运算前检查除数是否为零",
	"suggestion.check_type_before_cast":  "在类型转换前检查值类型：`%s` 不能转换为 `%s`",
	"suggestion.check_recursion":         "检查递归函数是否有正确的终止条件",
	"suggestion.add_base_case":           "确保递归函数有基本情况（base case）来终止递归",

	// ========== JIT 相关 ==========
	"jit.compilation_failed":       "JIT编译失败: %s",
	"jit.unsupported_instruction":  "JIT不支持的指令: %s",
	"jit.call_failed":              "JIT函数调用失败: %s",
	"jit.memory_allocation_failed": "JIT内存分配失败",
	"jit.execution_failed":         "JIT执行失败: %s",
	"jit.type_conversion_failed":   "JIT类型转换失败: 无法从 %s 转换为 %s",
	"jit.object_operation_failed":  "JIT对象操作失败: %s",
	"jit.inlining_failed":          "JIT内联失败: %s",
	
	// JIT 建议
	"suggestion.jit.disable":              "尝试使用 --no-jit 选项禁用JIT编译",
	"suggestion.jit.simplify_function":    "尝试简化函数逻辑或拆分为更小的函数",
	"suggestion.jit.check_types":          "确保值的类型在编译时是已知的",
	"suggestion.jit.avoid_dynamic":        "避免在JIT热点代码中使用动态类型",
	"suggestion.jit.add_type_hints":       "考虑添加显式类型标注",
	"suggestion.jit.check_null":           "确保对象不为null",
	"suggestion.jit.increase_memory":      "尝试增加JIT内存限制",
	"suggestion.jit.report_bug":           "这可能是JIT编译器的bug，请考虑报告问题",
}


