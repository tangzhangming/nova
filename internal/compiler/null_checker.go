package compiler

import (
	"fmt"

	"github.com/tangzhangming/nova/internal/ast"
	"github.com/tangzhangming/nova/internal/token"
)

// NullChecker 空安全检查器
type NullChecker struct {
	symbolTable *SymbolTable
	typeChecker *TypeChecker
	errors      []TypeError
	warnings    []TypeWarning
	
	// 当前作用域的类型收窄信息
	narrowings  map[string]string
}

// NewNullChecker 创建空安全检查器
func NewNullChecker(symbolTable *SymbolTable, typeChecker *TypeChecker) *NullChecker {
	return &NullChecker{
		symbolTable: symbolTable,
		typeChecker: typeChecker,
		errors:      make([]TypeError, 0),
		warnings:    make([]TypeWarning, 0),
		narrowings:  make(map[string]string),
	}
}

// CheckExpression 检查表达式的空安全性
func (nc *NullChecker) CheckExpression(expr ast.Expression, exprType string) {
	if expr == nil {
		return
	}
	
	switch e := expr.(type) {
	case *ast.PropertyAccess:
		nc.checkPropertyAccess(e, exprType)
		
	case *ast.MethodCall:
		nc.checkMethodCall(e, exprType)
		
	case *ast.IndexExpr:
		nc.checkIndexExpr(e, exprType)
		
	case *ast.BinaryExpr:
		// 递归检查子表达式
		leftType := nc.typeChecker.checkExpression(e.Left)
		rightType := nc.typeChecker.checkExpression(e.Right)
		nc.CheckExpression(e.Left, leftType)
		nc.CheckExpression(e.Right, rightType)
		
	case *ast.UnaryExpr:
		operandType := nc.typeChecker.checkExpression(e.Operand)
		nc.CheckExpression(e.Operand, operandType)
		
	case *ast.AssignExpr:
		leftType := nc.typeChecker.checkExpression(e.Left)
		rightType := nc.typeChecker.checkExpression(e.Right)
		nc.CheckExpression(e.Left, leftType)
		nc.CheckExpression(e.Right, rightType)
		
	case *ast.CallExpr:
		for _, arg := range e.Arguments {
			argType := nc.typeChecker.checkExpression(arg)
			nc.CheckExpression(arg, argType)
		}
		
	case *ast.TernaryExpr:
		condType := nc.typeChecker.checkExpression(e.Condition)
		thenType := nc.typeChecker.checkExpression(e.Then)
		elseType := nc.typeChecker.checkExpression(e.Else)
		nc.CheckExpression(e.Condition, condType)
		nc.CheckExpression(e.Then, thenType)
		nc.CheckExpression(e.Else, elseType)
		
	case *ast.ArrayLiteral:
		for _, elem := range e.Elements {
			elemType := nc.typeChecker.checkExpression(elem)
			nc.CheckExpression(elem, elemType)
		}
		
	case *ast.MapLiteral:
		for _, pair := range e.Pairs {
			keyType := nc.typeChecker.checkExpression(pair.Key)
			valueType := nc.typeChecker.checkExpression(pair.Value)
			nc.CheckExpression(pair.Key, keyType)
			nc.CheckExpression(pair.Value, valueType)
		}
		
	case *ast.NewExpr:
		for _, arg := range e.Arguments {
			argType := nc.typeChecker.checkExpression(arg)
			nc.CheckExpression(arg, argType)
		}
	}
}

// checkPropertyAccess 检查属性访问的空安全性
func (nc *NullChecker) checkPropertyAccess(expr *ast.PropertyAccess, exprType string) {
	objectType := nc.typeChecker.checkExpression(expr.Object)
	
	// 检查对象是否是可空类型
	if nc.isNullableType(objectType) {
		// 检查是否在类型收窄的上下文中
		if !nc.isTypeNarrowed(expr.Object) {
			nc.addError(expr.Property.Pos(), "compiler.nullable_access",
				fmt.Sprintf("cannot access property '%s' of nullable type '%s'. Use safe call operator '?.' or check for null first",
					expr.Property.Name, objectType))
		}
	}
	
	// 递归检查对象
	nc.CheckExpression(expr.Object, objectType)
}

// checkMethodCall 检查方法调用的空安全性
func (nc *NullChecker) checkMethodCall(expr *ast.MethodCall, exprType string) {
	objectType := nc.typeChecker.checkExpression(expr.Object)
	
	// 检查对象是否是可空类型
	if nc.isNullableType(objectType) {
		// 检查是否在类型收窄的上下文中
		if !nc.isTypeNarrowed(expr.Object) {
			nc.addError(expr.Method.Pos(), "compiler.nullable_access",
				fmt.Sprintf("cannot call method '%s' on nullable type '%s'. Use safe call operator '?.' or check for null first",
					expr.Method.Name, objectType))
		}
	}
	
	// 递归检查对象
	nc.CheckExpression(expr.Object, objectType)
	
	// 检查参数
	for _, arg := range expr.Arguments {
		argType := nc.typeChecker.checkExpression(arg)
		nc.CheckExpression(arg, argType)
	}
}

// checkIndexExpr 检查索引表达式的空安全性
func (nc *NullChecker) checkIndexExpr(expr *ast.IndexExpr, exprType string) {
	objectType := nc.typeChecker.checkExpression(expr.Object)
	
	// 检查对象是否是可空类型
	if nc.isNullableType(objectType) {
		if !nc.isTypeNarrowed(expr.Object) {
			nc.addError(expr.Index.Pos(), "compiler.nullable_access",
				fmt.Sprintf("cannot index into nullable type '%s'. Check for null first", objectType))
		}
	}
	
	// 递归检查对象和索引
	nc.CheckExpression(expr.Object, objectType)
	indexType := nc.typeChecker.checkExpression(expr.Index)
	nc.CheckExpression(expr.Index, indexType)
}

// CheckNullAssignment 检查 null 赋值的合法性
func (nc *NullChecker) CheckNullAssignment(targetType string, valueExpr ast.Expression, pos token.Position) {
	// 如果值是 null
	if _, isNull := valueExpr.(*ast.NullLiteral); isNull {
		// 目标类型必须是可空的
		if !nc.isNullableType(targetType) {
			nc.addError(pos, "compiler.null_assignment",
				fmt.Sprintf("cannot assign null to non-nullable type '%s'. Use nullable type '%s|null'",
					targetType, targetType))
		}
	}
}

// ApplyNarrowings 应用类型收窄
func (nc *NullChecker) ApplyNarrowings(narrowings map[string]string) {
	nc.narrowings = narrowings
}

// ClearNarrowings 清除类型收窄
func (nc *NullChecker) ClearNarrowings() {
	nc.narrowings = make(map[string]string)
}

// isNullableType 检查是否是可空类型
func (nc *NullChecker) isNullableType(typeName string) bool {
	return nc.typeChecker.isNullableType(typeName)
}

// isTypeNarrowed 检查表达式是否在收窄上下文中
func (nc *NullChecker) isTypeNarrowed(expr ast.Expression) bool {
	if v, ok := expr.(*ast.Variable); ok {
		_, narrowed := nc.narrowings[v.Name]
		return narrowed
	}
	return false
}

// addError 添加错误
func (nc *NullChecker) addError(pos token.Position, code, message string) {
	nc.errors = append(nc.errors, TypeError{
		Pos:     pos,
		Code:    code,
		Message: message,
	})
}

// addWarning 添加警告
func (nc *NullChecker) addWarning(pos token.Position, code, message string) {
	nc.warnings = append(nc.warnings, TypeWarning{
		Pos:     pos,
		Code:    code,
		Message: message,
	})
}

// GetErrors 获取错误列表
func (nc *NullChecker) GetErrors() []TypeError {
	return nc.errors
}

// GetWarnings 获取警告列表
func (nc *NullChecker) GetWarnings() []TypeWarning {
	return nc.warnings
}

// CheckNullableReturn 检查可空返回值
func (nc *NullChecker) CheckNullableReturn(returnType string, returnValues []ast.Expression, pos token.Position) {
	// 如果返回类型不可空，但返回了 null
	if !nc.isNullableType(returnType) {
		for _, val := range returnValues {
			if _, isNull := val.(*ast.NullLiteral); isNull {
				nc.addError(pos, "compiler.null_return",
					fmt.Sprintf("cannot return null from function with non-nullable return type '%s'", returnType))
			}
		}
	}
}

// CheckNullableParameter 检查可空参数传递
func (nc *NullChecker) CheckNullableParameter(paramType string, argExpr ast.Expression, argType string, pos token.Position) {
	// 如果参数类型不可空，但传入了可空类型
	if !nc.isNullableType(paramType) && nc.isNullableType(argType) {
		nc.addWarning(pos, "compiler.nullable_argument",
			fmt.Sprintf("passing nullable type '%s' to non-nullable parameter '%s'. This may cause null pointer errors",
				argType, paramType))
	}
}

// SuggestSafeCall 建议使用安全调用
func (nc *NullChecker) SuggestSafeCall(expr ast.Expression, pos token.Position) {
	nc.addWarning(pos, "compiler.suggest_safe_call",
		"consider using safe call operator '?.' to handle null values")
}

// SuggestNullCoalescing 建议使用空合并运算符
func (nc *NullChecker) SuggestNullCoalescing(expr ast.Expression, pos token.Position) {
	nc.addWarning(pos, "compiler.suggest_null_coalescing",
		"consider using null coalescing operator '??' to provide a default value")
}

