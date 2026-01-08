package i18n

var messagesEN = map[string]string{
	// ========== Lexer ==========
	ErrUnexpectedChar:      "unexpected character '%c'",
	ErrUnexpectedDoubleDot: "unexpected '..'",
	ErrUnterminatedComment: "unterminated block comment",
	ErrUnterminatedString:  "unterminated string",
	ErrUnterminatedInterp:  "unterminated interpolated string",
	ErrExpectedVarName:     "expected variable name after '$'",
	ErrInvalidHexNumber:    "invalid hex number: %s",
	ErrInvalidBinaryNumber: "invalid binary number: %s",
	ErrInvalidExponent:     "invalid number: expected exponent",
	ErrInvalidFloat:        "invalid float number: %s",
	ErrInvalidInteger:      "invalid integer: %s",

	// ========== Parser ==========
	ErrExpectedType:         "expected type",
	ErrVoidNotAllowed:       "'void' is not allowed as return type; omit the return type instead",
	ErrExpectedToken:        "expected %s",
	ErrUnexpectedToken:      "unexpected token: %s",
	ErrExpectedExpression:   "expected expression",
	ErrExpectedStatement:    "expected statement",
	ErrExpectedClassName:    "expected class name",
	ErrExpectedMethodName:   "expected method name",
	ErrExpectedPropertyName: "expected property name",
	ErrExpectedParamName:    "expected parameter name",
	ErrExpectedVarInUse:     "expected variable in use clause",
	ErrExpectedCaseDefault:  "expected 'case' or 'default'",
	ErrInvalidStaticAccess:  "invalid static access",
	ErrInvalidStaticMember:  "invalid static member",
	ErrChainedTypeCast:      "chained type cast is not allowed",
	ErrInvalidAssignTarget:  "invalid assignment target",

	// ========== Compiler ==========
	ErrUnsupportedStmt:      "unsupported statement type",
	ErrUnsupportedExpr:      "unsupported expression type",
	ErrBreakOutsideLoop:     "'break' outside of loop",
	ErrContinueOutsideLoop:  "'continue' outside of loop",
	ErrUndefinedVariable:    "undefined variable '$%s' (use 'use' to capture external variables in closures)",
	ErrTooManyLocals:        "too many local variables",
	ErrVariableRedeclared:   "variable already declared in this scope",
	ErrTypeMismatch:         "type mismatch: expected %s but got %s",
	ErrCannotAssign:         "cannot assign %s to variable of type %s",
	ErrReturnTypeMismatch:   "type mismatch: expected %s but got %s",
	ErrReturnCountMismatch:  "function expects %d return value(s) but got %d",
	ErrNoReturnExpected:     "function declared without return type but returns %d value(s)",
	ErrArraySizeNotConst:    "array size must be a compile-time constant",
	ErrArraySizeNegative:    "array size must be a non-negative constant",
	ErrArrayTooManyElements: "too many elements in array initializer (max %d, got %d)",
	ErrCompoundAssignIndex:  "compound assignment to array element not yet supported",
	ErrInvalidStaticAccessC: "invalid static access",
	ErrInvalidBinaryOp:      "operator '%s' cannot be applied to %s and %s",
	ErrNativeFuncRestricted: "native function '%s' can only be called from standard library (sola.*)",
	ErrMapKeyTypeMismatch:   "map key type mismatch: expected %s but got %s",
	ErrMapValueTypeMismatch: "map value type mismatch: expected %s but got %s",
	ErrCannotInferInterface: "cannot infer type for interface '%s', explicit type declaration required",
	
	// Static type checking
	ErrTypeCannotInfer:      "cannot infer type of expression",
	ErrFunctionNotFound:     "undefined function '%s'",
	ErrMethodNotFound:       "type '%s' has no method '%s' with %d arguments",
	ErrUnionTypeMismatch:    "value type '%s' is not a member of union type '%s'",
	ErrAllTypesMustBeKnown:  "static type checking requires all types to be known at compile time",
	ErrPropertyNotFound:     "type '%s' has no property '%s'",
	ErrVariableTypeUnknown:  "type of variable '$%s' is unknown",
	ErrStaticMemberNotFound: "class '%s' has no static member '%s'",
	ErrCannotInferVarType:   "cannot infer type of variable '$%s', explicit type declaration required",
	ErrIndexTargetUnknown:   "cannot determine type of index target",
	ErrUndeclaredVariable:   "variable '$%s' is not declared, use ':=' to declare a new variable",
	
	// Generics
	ErrGenericTypeParamName:      "expected type parameter name",
	ErrGenericTypeArgCount:       "type '%s' requires %d type argument(s), but %d provided",
	ErrGenericConstraintViolated: "type '%s' does not satisfy constraint '%s'",
	ErrGenericTypeRequired:       "generic type '%s' requires type arguments",
	ErrDuplicateTypeParam:        "duplicate type parameter '%s'",
	
	// Array types
	ErrSuperArrayNotCompatible:   "SuperArray is not compatible with typed array '%s', use typed array or explicit conversion",
	ErrArrayNotCompatible:        "typed array '%s' is not compatible with SuperArray, use SuperArray syntax [key => value]",
	
	// Exhaustiveness checking
	ErrSwitchNotExhaustive: "switch statement does not cover all values of enum '%s', missing: %s",
	
	// Final
	ErrCannotExtendFinalClass:    "cannot extend final class '%s'",
	ErrCannotOverrideFinalMethod: "cannot override final method '%s' in class '%s'",
	ErrFinalAndAbstractConflict:  "a class cannot be both final and abstract",
	ErrCannotAssignFinalProperty: "cannot reassign final property '%s'",
	
	// Interface
	ErrInterfaceNotImplemented:      "class '%s' does not implement interface '%s'",
	ErrInterfaceMethodMissing:       "class '%s' does not implement method '%s' from interface '%s'",
	ErrInterfaceMethodParamMismatch: "method '%s' in class '%s' has parameter type mismatch with interface '%s' (expected %s, got %s)",
	ErrInterfaceMethodReturnMismatch: "method '%s' in class '%s' has return type incompatible with interface '%s' (expected %s, got %s)",
	ErrInterfaceMethodStaticMismatch: "method '%s' in class '%s' has static/instance mismatch with interface '%s'",
	
	// Null safety checks
	ErrNullableAccess:          "cannot access member of nullable type '%s', check for null first or use safe call '?.'",
	ErrNullAssignment:          "cannot assign null to non-nullable type '%s', use nullable type '%s|null'",
	ErrNullableArgument:        "passing nullable type '%s' to non-nullable parameter '%s', may cause null pointer error",
	ErrNullableReturn:          "cannot return null from function with return type '%s'",
	WarnUnreachableCode:        "unreachable code detected",
	WarnUninitializedVariable:  "variable '%s' may not have been initialized",
	
	// Class name resolution
	ErrSelfOutsideClass: "cannot use self::class outside of class",

	// ========== VM ==========
	ErrIPOutOfBounds:            "instruction pointer out of bounds",
	ErrExecutionLimit:           "execution limit exceeded (infinite loop?)",
	ErrUndefinedVar:             "undefined variable '%s'",
	ErrOperandMustBeNumber:      "operand must be a number",
	ErrDivisionByZero:           "division by zero",
	ErrModuloNotForFloats:       "modulo not supported for floats",
	ErrOperandsMustBeNumbers:    "operands must be numbers",
	ErrOperandsMustBeComparable: "operands must be comparable",
	ErrOnlyObjectsHaveFields:    "only objects have fields",
	ErrOnlyObjectsHaveMethods:   "only objects have methods",
	ErrUndefinedClass:           "undefined class '%s'",
	ErrArrayIndexOutOfBounds:    "array index %d out of bounds (capacity %d)",
	ErrArrayIndexSimple:         "array index out of bounds",
	ErrSubscriptRequiresArray:   "subscript operator requires array or map",
	ErrCanOnlyCallFunctions:     "can only call functions",
	ErrArgumentCountMin:         "expected at least %d arguments but got %d",
	ErrArgumentCountMax:         "expected at most %d arguments but got %d",
	ErrStackOverflow:            "stack overflow",
	ErrTypeError:                "type error: expected %s but got %s",
	ErrCannotCast:               "cannot cast %s to %s",
	ErrUnknownOpcode:            "unknown opcode: %d",
	ErrForeachRequiresIterable:  "foreach requires array or map",
	ErrExpectedIterator:         "expected iterator",
	ErrUndefinedMethod:          "undefined method '%s' with %d arguments",
	ErrUndefinedStaticMethod:    "undefined static method '%s::%s' with %d arguments",
	ErrUndefinedEnumCase:        "undefined enum case '%s::%s'",
	ErrLengthRequiresArray:      "length requires array",
	ErrLengthRequiresMap:        "length requires map",
	ErrPushRequiresArray:        "push requires array",
	ErrHasRequiresArray:         "has requires array",
	ErrHasRequiresArrayOrMap:    "has() requires array or map",
	ErrSubscriptRequiresMap:     "subscript operator requires map",
	ErrRuntimeError:             "Runtime error: %s",

	// ========== Runtime ==========
	ErrFailedCreateLoader: "failed to create loader: %v",
	ErrParseError:         "Parse error: %s",
	ErrParseFailed:        "parse failed",
	ErrParseFailedFor:     "parse failed for %s",
	ErrLoadFailed:         "failed to load %s: %v",
	ErrReadFailed:         "failed to read %s: %v",
	ErrCompileError:       "Compile error: %s",
	ErrCompileFailed:      "compile failed",
	ErrCompileFailedFor:   "compile failed for %s",

	// ========== Suggestions ==========
	// Variable related
	"suggestion.declare_variable":      "Did you mean to declare a new variable? Use `$%s := value`",
	"suggestion.did_you_mean":          "Did you mean `$%s`?",
	"suggestion.use_assign_instead":    "To modify the variable, use `$%s = newValue` instead of `:=`",
	"suggestion.rename_variable":       "Consider using a different variable name",
	"suggestion.use_clause":            "To use external variables in a closure, add `use ($%s)`",

	// Type related
	"suggestion.convert_string_to_int":   "Use `Str::toInt($var)` to convert string to integer",
	"suggestion.convert_int_to_string":   "Use `\"\" + $var` or string interpolation `\"${var}\"` to convert to string",
	"suggestion.convert_string_to_float": "Use `Str::toFloat($var)` to convert string to float",
	"suggestion.convert_float_to_string": "Use `\"\" + $var` or string interpolation `\"${var}\"` to convert to string",
	"suggestion.cast_float_to_int":       "Use `(int)$var` to convert float to int (truncates decimal part)",
	"suggestion.implicit_int_to_float":   "Integer can be implicitly converted to float",
	"suggestion.explicit_bool_check":     "Use an explicit boolean expression like `$var != 0` or `$var != \"\"`",
	"suggestion.wrap_in_array":           "Use `[$var]` to wrap the value in an array",
	"suggestion.explicit_type":           "Add an explicit type declaration for variable `$%s`",
	"suggestion.check_return_type":       "Ensure the return value is of type `%s`",

	// Function related
	"suggestion.check_function_name":   "Check if function name `%s` is spelled correctly",
	"suggestion.did_you_mean_func":     "Did you mean `%s()`?",
	"suggestion.check_import":          "Check if you need to import the relevant module",
	"suggestion.add_arguments":         "Add %d missing argument(s)",
	"suggestion.remove_arguments":      "Remove %d extra argument(s)",
	"suggestion.break_only_in_loop":    "`break` can only be used inside `for`, `while`, `foreach`, or `switch`",
	"suggestion.use_return_instead":    "To exit a function, use `return`",
	"suggestion.continue_only_in_loop": "`continue` can only be used inside `for`, `while`, or `foreach` loops",

	// Class/Object related
	"suggestion.check_method_name":   "Check if method `%s` exists on type `%s`",
	"suggestion.did_you_mean_method": "Did you mean `%s()`?",
	"suggestion.check_property_name": "Check if property `%s` exists on type `%s`",

	// Runtime related
	"suggestion.array_index_range":       "Valid index range is [0, %d]",
	"suggestion.check_index_before_access": "Check index before accessing: `if ($i >= 0 && $i < len($arr))`",
	"suggestion.check_divisor":           "Check if divisor is zero before division",
	"suggestion.check_type_before_cast":  "Check value type before casting: `%s` cannot be converted to `%s`",
	"suggestion.check_recursion":         "Check if recursive function has proper termination condition",
	"suggestion.add_base_case":           "Ensure recursive function has a base case to terminate recursion",

	// ========== JIT Related ==========
	"jit.compilation_failed":       "JIT compilation failed: %s",
	"jit.unsupported_instruction":  "Unsupported JIT instruction: %s",
	"jit.call_failed":              "JIT function call failed: %s",
	"jit.memory_allocation_failed": "JIT memory allocation failed",
	"jit.execution_failed":         "JIT execution failed: %s",
	"jit.type_conversion_failed":   "JIT type conversion failed: cannot convert %s to %s",
	"jit.object_operation_failed":  "JIT object operation failed: %s",
	"jit.inlining_failed":          "JIT inlining failed: %s",
	
	// JIT Suggestions
	"suggestion.jit.disable":              "Try using --no-jit option to disable JIT compilation",
	"suggestion.jit.simplify_function":    "Try simplifying function logic or splitting into smaller functions",
	"suggestion.jit.check_types":          "Ensure value types are known at compile time",
	"suggestion.jit.avoid_dynamic":        "Avoid using dynamic types in JIT hot code paths",
	"suggestion.jit.add_type_hints":       "Consider adding explicit type annotations",
	"suggestion.jit.check_null":           "Ensure object is not null",
	"suggestion.jit.increase_memory":      "Try increasing JIT memory limit",
	"suggestion.jit.report_bug":           "This may be a JIT compiler bug, please consider reporting the issue",
}


