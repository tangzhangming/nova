package errors

import (
	"strings"

	"github.com/tangzhangming/nova/internal/i18n"
)

// ============================================================================
// 修复建议生成器
// ============================================================================

// SuggestionGenerator 修复建议生成器
type SuggestionGenerator struct{}

// NewSuggestionGenerator 创建修复建议生成器
func NewSuggestionGenerator() *SuggestionGenerator {
	return &SuggestionGenerator{}
}

// GetSuggestions 根据错误码和上下文获取修复建议
func (g *SuggestionGenerator) GetSuggestions(code string, context map[string]interface{}) []string {
	switch code {
	// 变量错误
	case E0100, E0102:
		return g.undefinedVariableSuggestions(context)
	case E0101:
		return g.variableRedeclaredSuggestions(context)
	case E0104:
		return g.closureVariableSuggestions(context)

	// 类型错误
	case E0200, E0202:
		return g.typeMismatchSuggestions(context)
	case E0201:
		return g.typeInferenceSuggestions(context)
	case E0203:
		return g.returnTypeSuggestions(context)

	// 函数错误
	case E0300:
		return g.undefinedFunctionSuggestions(context)
	case E0301, E0302:
		return g.argumentCountSuggestions(context)
	case E0304:
		return g.breakOutsideLoopSuggestions()
	case E0305:
		return g.continueOutsideLoopSuggestions()

	// 类/对象错误
	case E0401:
		return g.undefinedMethodSuggestions(context)
	case E0402:
		return g.undefinedPropertySuggestions(context)

	// 运行时错误
	case R0100:
		return g.arrayIndexSuggestions(context)
	case R0200:
		return g.divisionByZeroSuggestions()
	case R0301:
		return g.typeCastSuggestions(context)
	case R0400:
		return g.stackOverflowSuggestions()

	// JIT 错误
	case R0600:
		return g.jitCompilationFailedSuggestions(context)
	case R0601:
		return g.jitUnsupportedInstructionSuggestions(context)
	case R0602:
		return g.jitCallFailedSuggestions(context)
	case R0603:
		return g.jitMemoryAllocationFailedSuggestions()
	case R0604:
		return g.jitExecutionFailedSuggestions(context)
	case R0605:
		return g.jitTypeConversionFailedSuggestions(context)
	case R0606:
		return g.jitObjectOperationFailedSuggestions(context)
	case R0607:
		return g.jitInliningFailedSuggestions(context)

	default:
		return nil
	}
}

// ============================================================================
// 具体建议生成
// ============================================================================

// undefinedVariableSuggestions 未定义变量的建议
func (g *SuggestionGenerator) undefinedVariableSuggestions(context map[string]interface{}) []string {
	var suggestions []string

	varName, _ := context["variable"].(string)

	suggestions = append(suggestions, i18n.T("suggestion.declare_variable", varName))

	// 检查是否是常见的拼写错误
	if similar, ok := context["similar"].(string); ok && similar != "" {
		suggestions = append(suggestions, i18n.T("suggestion.did_you_mean", similar))
	}

	return suggestions
}

// variableRedeclaredSuggestions 变量重复声明的建议
func (g *SuggestionGenerator) variableRedeclaredSuggestions(context map[string]interface{}) []string {
	varName, _ := context["variable"].(string)
	return []string{
		i18n.T("suggestion.use_assign_instead", varName),
		i18n.T("suggestion.rename_variable"),
	}
}

// closureVariableSuggestions 闭包变量捕获的建议
func (g *SuggestionGenerator) closureVariableSuggestions(context map[string]interface{}) []string {
	varName, _ := context["variable"].(string)
	return []string{
		i18n.T("suggestion.use_clause", varName),
	}
}

// typeMismatchSuggestions 类型不匹配的建议
func (g *SuggestionGenerator) typeMismatchSuggestions(context map[string]interface{}) []string {
	var suggestions []string

	expected, _ := context["expected"].(string)
	actual, _ := context["actual"].(string)

	// 字符串 <-> 数字 转换
	if expected == "int" && actual == "string" {
		suggestions = append(suggestions, i18n.T("suggestion.convert_string_to_int"))
	} else if expected == "string" && actual == "int" {
		suggestions = append(suggestions, i18n.T("suggestion.convert_int_to_string"))
	} else if expected == "float" && actual == "string" {
		suggestions = append(suggestions, i18n.T("suggestion.convert_string_to_float"))
	} else if expected == "string" && actual == "float" {
		suggestions = append(suggestions, i18n.T("suggestion.convert_float_to_string"))
	} else if expected == "int" && actual == "float" {
		suggestions = append(suggestions, i18n.T("suggestion.cast_float_to_int"))
	} else if expected == "float" && actual == "int" {
		suggestions = append(suggestions, i18n.T("suggestion.implicit_int_to_float"))
	} else if expected == "bool" && (actual == "int" || actual == "string") {
		suggestions = append(suggestions, i18n.T("suggestion.explicit_bool_check"))
	}

	// 数组类型
	if strings.HasSuffix(expected, "[]") && !strings.HasSuffix(actual, "[]") {
		suggestions = append(suggestions, i18n.T("suggestion.wrap_in_array"))
	}

	return suggestions
}

// typeInferenceSuggestions 类型推断失败的建议
func (g *SuggestionGenerator) typeInferenceSuggestions(context map[string]interface{}) []string {
	varName, _ := context["variable"].(string)
	return []string{
		i18n.T("suggestion.explicit_type", varName),
	}
}

// returnTypeSuggestions 返回类型不匹配的建议
func (g *SuggestionGenerator) returnTypeSuggestions(context map[string]interface{}) []string {
	expected, _ := context["expected"].(string)
	return []string{
		i18n.T("suggestion.check_return_type", expected),
	}
}

// undefinedFunctionSuggestions 未定义函数的建议
func (g *SuggestionGenerator) undefinedFunctionSuggestions(context map[string]interface{}) []string {
	var suggestions []string

	funcName, _ := context["function"].(string)

	suggestions = append(suggestions, i18n.T("suggestion.check_function_name", funcName))

	if similar, ok := context["similar"].(string); ok && similar != "" {
		suggestions = append(suggestions, i18n.T("suggestion.did_you_mean_func", similar))
	}

	suggestions = append(suggestions, i18n.T("suggestion.check_import"))

	return suggestions
}

// argumentCountSuggestions 参数数量错误的建议
func (g *SuggestionGenerator) argumentCountSuggestions(context map[string]interface{}) []string {
	expected, _ := context["expected"].(int)
	actual, _ := context["actual"].(int)

	if actual < expected {
		return []string{i18n.T("suggestion.add_arguments", expected-actual)}
	}
	return []string{i18n.T("suggestion.remove_arguments", actual-expected)}
}

// breakOutsideLoopSuggestions break 在循环外的建议
func (g *SuggestionGenerator) breakOutsideLoopSuggestions() []string {
	return []string{
		i18n.T("suggestion.break_only_in_loop"),
		i18n.T("suggestion.use_return_instead"),
	}
}

// continueOutsideLoopSuggestions continue 在循环外的建议
func (g *SuggestionGenerator) continueOutsideLoopSuggestions() []string {
	return []string{
		i18n.T("suggestion.continue_only_in_loop"),
	}
}

// undefinedMethodSuggestions 未定义方法的建议
func (g *SuggestionGenerator) undefinedMethodSuggestions(context map[string]interface{}) []string {
	var suggestions []string

	methodName, _ := context["method"].(string)
	typeName, _ := context["type"].(string)

	suggestions = append(suggestions, i18n.T("suggestion.check_method_name", methodName, typeName))

	if similar, ok := context["similar"].(string); ok && similar != "" {
		suggestions = append(suggestions, i18n.T("suggestion.did_you_mean_method", similar))
	}

	return suggestions
}

// undefinedPropertySuggestions 未定义属性的建议
func (g *SuggestionGenerator) undefinedPropertySuggestions(context map[string]interface{}) []string {
	propName, _ := context["property"].(string)
	typeName, _ := context["type"].(string)
	return []string{
		i18n.T("suggestion.check_property_name", propName, typeName),
	}
}

// arrayIndexSuggestions 数组索引越界的建议
func (g *SuggestionGenerator) arrayIndexSuggestions(context map[string]interface{}) []string {
	length, _ := context["length"].(int)
	return []string{
		i18n.T("suggestion.array_index_range", length-1),
		i18n.T("suggestion.check_index_before_access"),
	}
}

// divisionByZeroSuggestions 除以零的建议
func (g *SuggestionGenerator) divisionByZeroSuggestions() []string {
	return []string{
		i18n.T("suggestion.check_divisor"),
	}
}

// typeCastSuggestions 类型转换失败的建议
func (g *SuggestionGenerator) typeCastSuggestions(context map[string]interface{}) []string {
	from, _ := context["from"].(string)
	to, _ := context["to"].(string)
	return []string{
		i18n.T("suggestion.check_type_before_cast", from, to),
	}
}

// stackOverflowSuggestions 栈溢出的建议
func (g *SuggestionGenerator) stackOverflowSuggestions() []string {
	return []string{
		i18n.T("suggestion.check_recursion"),
		i18n.T("suggestion.add_base_case"),
	}
}

// ============================================================================
// JIT 错误建议
// ============================================================================

// jitCompilationFailedSuggestions JIT 编译失败的建议
func (g *SuggestionGenerator) jitCompilationFailedSuggestions(context map[string]interface{}) []string {
	var suggestions []string
	
	// 检查具体的失败原因
	if reason, ok := context["reason"].(string); ok {
		if strings.Contains(reason, "unsupported") {
			suggestions = append(suggestions, "该函数包含JIT不支持的指令，将使用解释器执行")
		} else if strings.Contains(reason, "memory") {
			suggestions = append(suggestions, "JIT内存不足，可能需要增加内存限制或减少热点函数数量")
		}
	}
	
	suggestions = append(suggestions,
		"尝试禁用JIT编译运行程序（使用 --no-jit 选项）",
		"检查函数是否包含复杂的控制流或递归调用",
	)
	
	return suggestions
}

// jitUnsupportedInstructionSuggestions JIT 不支持指令的建议
func (g *SuggestionGenerator) jitUnsupportedInstructionSuggestions(context map[string]interface{}) []string {
	var suggestions []string
	
	if opcode, ok := context["opcode"].(string); ok {
		suggestions = append(suggestions, 
			"指令 "+opcode+" 目前不被JIT支持",
		)
	}
	
	suggestions = append(suggestions,
		"该函数将回退到解释器执行",
		"请考虑简化函数逻辑或拆分为更小的函数",
	)
	
	return suggestions
}

// jitCallFailedSuggestions JIT 函数调用失败的建议
func (g *SuggestionGenerator) jitCallFailedSuggestions(context map[string]interface{}) []string {
	var suggestions []string
	
	if funcName, ok := context["function"].(string); ok {
		suggestions = append(suggestions,
			"调用函数 "+funcName+" 失败",
		)
	}
	
	suggestions = append(suggestions,
		"检查函数参数类型是否正确",
		"确保被调用的函数存在且可访问",
		"JIT与解释器之间的互操作可能有类型转换问题",
	)
	
	return suggestions
}

// jitMemoryAllocationFailedSuggestions JIT 内存分配失败的建议
func (g *SuggestionGenerator) jitMemoryAllocationFailedSuggestions() []string {
	return []string{
		"JIT代码缓存内存不足",
		"尝试增加JIT内存限制（使用 --jit-memory 选项）",
		"减少热点函数数量或降低优化级别",
		"考虑使用解释器模式运行（--no-jit）",
	}
}

// jitExecutionFailedSuggestions JIT 执行失败的建议
func (g *SuggestionGenerator) jitExecutionFailedSuggestions(context map[string]interface{}) []string {
	var suggestions []string
	
	if funcName, ok := context["function"].(string); ok {
		suggestions = append(suggestions,
			"JIT编译的函数 "+funcName+" 执行时发生错误",
		)
	}
	
	suggestions = append(suggestions,
		"这可能是JIT编译器的bug，请考虑报告问题",
		"尝试禁用JIT运行以确认是否为JIT相关问题",
		"检查函数是否有未初始化的变量或空引用",
	)
	
	return suggestions
}

// jitTypeConversionFailedSuggestions JIT 类型转换失败的建议
func (g *SuggestionGenerator) jitTypeConversionFailedSuggestions(context map[string]interface{}) []string {
	var suggestions []string
	
	from, _ := context["from"].(string)
	to, _ := context["to"].(string)
	
	if from != "" && to != "" {
		suggestions = append(suggestions,
			"无法在JIT中从 "+from+" 转换为 "+to,
		)
	}
	
	suggestions = append(suggestions,
		"确保值的类型在编译时是已知的",
		"避免在JIT热点代码中使用动态类型",
		"考虑添加显式类型标注",
	)
	
	return suggestions
}

// jitObjectOperationFailedSuggestions JIT 对象操作失败的建议
func (g *SuggestionGenerator) jitObjectOperationFailedSuggestions(context map[string]interface{}) []string {
	var suggestions []string
	
	if op, ok := context["operation"].(string); ok {
		suggestions = append(suggestions,
			"对象操作 "+op+" 在JIT执行时失败",
		)
	}
	
	if className, ok := context["class"].(string); ok {
		suggestions = append(suggestions,
			"类 "+className+" 的布局可能与JIT预期不符",
		)
	}
	
	suggestions = append(suggestions,
		"确保对象不为null",
		"检查字段名称是否正确",
		"JIT对象操作依赖于静态类型信息",
	)
	
	return suggestions
}

// jitInliningFailedSuggestions JIT 内联失败的建议
func (g *SuggestionGenerator) jitInliningFailedSuggestions(context map[string]interface{}) []string {
	var suggestions []string
	
	if funcName, ok := context["function"].(string); ok {
		suggestions = append(suggestions,
			"函数 "+funcName+" 无法被内联",
		)
	}
	
	if reason, ok := context["reason"].(string); ok {
		suggestions = append(suggestions,
			"原因: "+reason,
		)
	}
	
	suggestions = append(suggestions,
		"内联失败不影响正确性，仅影响性能",
		"可以尝试简化被调用函数以支持内联",
	)
	
	return suggestions
}

// ============================================================================
// 智能修复建议
// ============================================================================

// QuickFix 快速修复建议
type QuickFix struct {
	Message     string // 修复描述
	Replacement string // 替换文本
	Line        int    // 行号
	Column      int    // 起始列
	EndColumn   int    // 结束列
}

// GetQuickFixes 根据错误码和上下文获取快速修复建议
func GetQuickFixes(code string, context map[string]interface{}) []QuickFix {
	var fixes []QuickFix

	switch code {
	case E0100, E0102: // 未定义的变量
		if varName, ok := context["variable"].(string); ok {
			// 建议声明变量
			fixes = append(fixes, QuickFix{
				Message:     i18n.T("quickfix.declare_variable", varName),
				Replacement: varName + " := ",
			})
		}

	case E0101: // 变量重复声明
		if varName, ok := context["variable"].(string); ok {
			// 建议使用赋值
			fixes = append(fixes, QuickFix{
				Message:     i18n.T("quickfix.use_assignment", varName),
				Replacement: varName + " = ",
			})
		}

	case E0200, E0202: // 类型不匹配
		expected, _ := context["expected"].(string)
		actual, _ := context["actual"].(string)

		// 类型转换建议
		if expected == "int" && actual == "string" {
			fixes = append(fixes, QuickFix{
				Message:     i18n.T("quickfix.convert_to_int"),
				Replacement: "(int)",
			})
		} else if expected == "string" && actual == "int" {
			fixes = append(fixes, QuickFix{
				Message:     i18n.T("quickfix.convert_to_string"),
				Replacement: "(string)",
			})
		} else if expected == "float" && actual == "int" {
			fixes = append(fixes, QuickFix{
				Message:     i18n.T("quickfix.implicit_conversion"),
				Replacement: "", // 隐式转换
			})
		}

	case E0300: // 未定义的函数
		if funcName, ok := context["function"].(string); ok {
			if similar, ok := context["similar"].(string); ok && similar != "" {
				fixes = append(fixes, QuickFix{
					Message:     i18n.T("quickfix.replace_with", similar),
					Replacement: similar,
				})
			}
			// 建议导入
			fixes = append(fixes, QuickFix{
				Message:     i18n.T("quickfix.add_import", funcName),
				Replacement: "",
			})
		}

	case E0301: // 参数过少
		if expected, ok := context["expected"].(int); ok {
			if actual, ok := context["actual"].(int); ok {
				missing := expected - actual
				fixes = append(fixes, QuickFix{
					Message: i18n.T("quickfix.add_params", missing),
				})
			}
		}

	case E0302: // 参数过多
		if actual, ok := context["actual"].(int); ok {
			if expected, ok := context["expected"].(int); ok {
				extra := actual - expected
				fixes = append(fixes, QuickFix{
					Message: i18n.T("quickfix.remove_params", extra),
				})
			}
		}

	case E0401: // 未定义的方法
		if _, ok := context["method"].(string); ok {
			if similar, ok := context["similar"].(string); ok && similar != "" {
				fixes = append(fixes, QuickFix{
					Message:     i18n.T("quickfix.replace_with", similar),
					Replacement: similar,
				})
			}
		}

	case E0402: // 未定义的属性
		if _, ok := context["property"].(string); ok {
			if similar, ok := context["similar"].(string); ok && similar != "" {
				fixes = append(fixes, QuickFix{
					Message:     i18n.T("quickfix.replace_with", similar),
					Replacement: similar,
				})
			}
		}
	}

	return fixes
}

// AnalyzeExpression 分析表达式并提供建议
func AnalyzeExpression(expr string, context map[string]interface{}) []string {
	var suggestions []string

	// 检测常见的错误模式
	if strings.Contains(expr, "==") && strings.Contains(expr, "=") {
		// 可能是赋值和比较混淆
		suggestions = append(suggestions, i18n.T("suggestion.check_assignment_vs_comparison"))
	}

	if strings.Contains(expr, "!=") && strings.Contains(expr, "!") {
		// 可能是逻辑非和不等于混淆
		suggestions = append(suggestions, i18n.T("suggestion.check_not_operator"))
	}

	return suggestions
}

// ============================================================================
// 相似名称查找
// ============================================================================

// FindSimilar 查找相似的名称
func FindSimilar(name string, candidates []string, maxDistance int) string {
	if len(candidates) == 0 {
		return ""
	}

	bestMatch := ""
	bestDistance := maxDistance + 1

	for _, candidate := range candidates {
		distance := levenshteinDistance(name, candidate)
		if distance < bestDistance {
			bestDistance = distance
			bestMatch = candidate
		}
	}

	if bestDistance <= maxDistance {
		return bestMatch
	}
	return ""
}

// levenshteinDistance 计算 Levenshtein 编辑距离
func levenshteinDistance(s1, s2 string) int {
	if len(s1) == 0 {
		return len(s2)
	}
	if len(s2) == 0 {
		return len(s1)
	}

	// 忽略大小写比较
	s1 = strings.ToLower(s1)
	s2 = strings.ToLower(s2)

	// 创建距离矩阵
	d := make([][]int, len(s1)+1)
	for i := range d {
		d[i] = make([]int, len(s2)+1)
	}

	// 初始化第一行和第一列
	for i := 0; i <= len(s1); i++ {
		d[i][0] = i
	}
	for j := 0; j <= len(s2); j++ {
		d[0][j] = j
	}

	// 填充矩阵
	for i := 1; i <= len(s1); i++ {
		for j := 1; j <= len(s2); j++ {
			cost := 0
			if s1[i-1] != s2[j-1] {
				cost = 1
			}
			d[i][j] = min(
				d[i-1][j]+1,      // 删除
				d[i][j-1]+1,      // 插入
				d[i-1][j-1]+cost, // 替换
			)
		}
	}

	return d[len(s1)][len(s2)]
}

func min(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

// ============================================================================
// 全局实例
// ============================================================================

var defaultSuggestionGenerator = NewSuggestionGenerator()

// GetSuggestions 使用默认生成器获取建议
func GetSuggestions(code string, context map[string]interface{}) []string {
	return defaultSuggestionGenerator.GetSuggestions(code, context)
}







