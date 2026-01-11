package compiler

import (
	"fmt"

	"github.com/tangzhangming/nova/internal/ast"
	"github.com/tangzhangming/nova/internal/bytecode"
	"github.com/tangzhangming/nova/internal/token"
)

// AnnotationTarget 注解目标类型
type AnnotationTarget int

const (
	TargetClass AnnotationTarget = iota
	TargetInterface
	TargetMethod
	TargetProperty
	TargetParameter
	TargetConstructor
)

// AnnotationValidator 注解验证器
// 用于在编译期验证注解的使用是否正确
type AnnotationValidator struct {
	compiler       *Compiler
	classes        map[string]*bytecode.Class
	strictMode     bool // 严格模式：注解必须先定义
	errors         []AnnotationError
}

// AnnotationError 注解错误
type AnnotationError struct {
	Pos     token.Position
	Message string
}

// NewAnnotationValidator 创建注解验证器
func NewAnnotationValidator(compiler *Compiler, classes map[string]*bytecode.Class, strict bool) *AnnotationValidator {
	return &AnnotationValidator{
		compiler:   compiler,
		classes:    classes,
		strictMode: strict,
		errors:     make([]AnnotationError, 0),
	}
}

// ValidateClassAnnotations 验证类上的注解
func (v *AnnotationValidator) ValidateClassAnnotations(decl *ast.ClassDecl) []AnnotationError {
	v.errors = nil
	v.validateAnnotations(decl.Annotations, TargetClass, decl.Name.Name)
	return v.errors
}

// ValidateInterfaceAnnotations 验证接口上的注解
func (v *AnnotationValidator) ValidateInterfaceAnnotations(decl *ast.InterfaceDecl) []AnnotationError {
	v.errors = nil
	v.validateAnnotations(decl.Annotations, TargetInterface, decl.Name.Name)
	return v.errors
}

// ValidateMethodAnnotations 验证方法上的注解
func (v *AnnotationValidator) ValidateMethodAnnotations(decl *ast.MethodDecl) []AnnotationError {
	v.errors = nil
	target := TargetMethod
	if decl.Name.Name == "__construct" {
		target = TargetConstructor
	}
	v.validateAnnotations(decl.Annotations, target, decl.Name.Name)
	return v.errors
}

// ValidatePropertyAnnotations 验证属性上的注解
func (v *AnnotationValidator) ValidatePropertyAnnotations(decl *ast.PropertyDecl) []AnnotationError {
	v.errors = nil
	v.validateAnnotations(decl.Annotations, TargetProperty, decl.Name.Name)
	return v.errors
}

// validateAnnotations 验证注解列表
func (v *AnnotationValidator) validateAnnotations(annotations []*ast.Annotation, target AnnotationTarget, elementName string) {
	// 记录已使用的注解（用于检查 @Repeatable）
	usedAnnotations := make(map[string]int)

	for _, ann := range annotations {
		annName := ann.Name.Name

		// 1. 检查注解类是否存在
		annClass := v.findAnnotationClass(annName)
		if annClass == nil {
			if v.strictMode {
				v.addError(ann.AtToken.Pos, fmt.Sprintf("undefined annotation: @%s", annName))
			}
			// 兼容模式：跳过未定义的注解（仅警告）
			continue
		}

		// 2. 检查类是否有 @Attribute 标记
		if !annClass.IsAttribute {
			v.addError(ann.AtToken.Pos, fmt.Sprintf("'%s' is not an annotation class (missing @Attribute)", annName))
			continue
		}

		// 3. 检查 @Target 是否匹配
		if !v.checkTarget(annClass, target, ann.AtToken.Pos, annName) {
			continue
		}

		// 4. 检查是否重复使用（非 @Repeatable）
		usedAnnotations[annName]++
		if usedAnnotations[annName] > 1 && !v.isRepeatable(annClass) {
			v.addError(ann.AtToken.Pos, fmt.Sprintf("@%s cannot be used multiple times (missing @Repeatable)", annName))
		}

		// 5. 验证注解参数（可选，需要构造函数信息）
		// 这里简化处理，只检查参数类型是否为编译期常量
	}
}

// findAnnotationClass 查找注解类
func (v *AnnotationValidator) findAnnotationClass(name string) *bytecode.Class {
	// 先查找完整名称
	if class, ok := v.classes[name]; ok {
		return class
	}

	// 尝试添加 sola.annotation. 前缀
	fullName := "sola.annotation." + name
	if class, ok := v.classes[fullName]; ok {
		return class
	}

	return nil
}

// checkTarget 检查 @Target 约束
func (v *AnnotationValidator) checkTarget(annClass *bytecode.Class, target AnnotationTarget, pos token.Position, annName string) bool {
	// 查找 @Target 注解
	targetAnn := v.findClassAnnotation(annClass, "Target")
	if targetAnn == nil {
		// 没有 @Target 注解，默认允许所有位置
		return true
	}

	// 获取 @Target 的 value 参数
	// Target 的参数格式: value = [ElementType::CLASS, ElementType::METHOD, ...]
	// 或位置参数: "0" -> [ElementType::CLASS, ...]
	var allowedTargets []string
	
	// 检查位置参数 "0" 或命名参数 "value"
	if val, ok := targetAnn.Args["0"]; ok {
		allowedTargets = v.parseElementTypes(val)
	} else if val, ok := targetAnn.Args["value"]; ok {
		allowedTargets = v.parseElementTypes(val)
	}

	if len(allowedTargets) == 0 {
		// 无法解析目标，默认允许
		return true
	}

	// 检查当前目标是否在允许列表中
	targetName := v.targetToString(target)
	for _, allowed := range allowedTargets {
		if allowed == "ALL" || allowed == targetName {
			return true
		}
	}

	v.addError(pos, fmt.Sprintf("@%s cannot be used on %s", annName, v.targetToReadable(target)))
	return false
}

// isRepeatable 检查注解是否可重复
func (v *AnnotationValidator) isRepeatable(annClass *bytecode.Class) bool {
	return v.findClassAnnotation(annClass, "Repeatable") != nil
}

// findClassAnnotation 在类上查找指定名称的注解
func (v *AnnotationValidator) findClassAnnotation(class *bytecode.Class, name string) *bytecode.Annotation {
	for _, ann := range class.Annotations {
		if ann.Name == name {
			return ann
		}
	}
	return nil
}

// parseElementTypes 解析 ElementType 数组
func (v *AnnotationValidator) parseElementTypes(val bytecode.Value) []string {
	var result []string
	
	// 如果是数组
	if val.Type() == bytecode.ValArray {
		arr := val.AsArray()
		for _, elem := range arr {
			if elem.Type() == bytecode.ValEnum {
				ev := elem.AsEnumValue()
				if ev != nil {
					result = append(result, ev.CaseName)
				}
			} else if elem.Type() == bytecode.ValString {
				result = append(result, elem.AsString())
			}
		}
	} else if val.Type() == bytecode.ValEnum {
		// 单个枚举值
		ev := val.AsEnumValue()
		if ev != nil {
			result = append(result, ev.CaseName)
		}
	} else if val.Type() == bytecode.ValString {
		// 字符串形式
		result = append(result, val.AsString())
	}
	
	return result
}

// targetToString 将目标类型转换为 ElementType 名称
func (v *AnnotationValidator) targetToString(target AnnotationTarget) string {
	switch target {
	case TargetClass:
		return "CLASS"
	case TargetInterface:
		return "INTERFACE"
	case TargetMethod:
		return "METHOD"
	case TargetProperty:
		return "PROPERTY"
	case TargetParameter:
		return "PARAMETER"
	case TargetConstructor:
		return "CONSTRUCTOR"
	default:
		return "UNKNOWN"
	}
}

// targetToReadable 将目标类型转换为可读描述
func (v *AnnotationValidator) targetToReadable(target AnnotationTarget) string {
	switch target {
	case TargetClass:
		return "a class"
	case TargetInterface:
		return "an interface"
	case TargetMethod:
		return "a method"
	case TargetProperty:
		return "a property"
	case TargetParameter:
		return "a parameter"
	case TargetConstructor:
		return "a constructor"
	default:
		return "this element"
	}
}

// addError 添加错误
func (v *AnnotationValidator) addError(pos token.Position, message string) {
	v.errors = append(v.errors, AnnotationError{
		Pos:     pos,
		Message: message,
	})
}

// HasErrors 检查是否有错误
func (v *AnnotationValidator) HasErrors() bool {
	return len(v.errors) > 0
}

// GetErrors 获取所有错误
func (v *AnnotationValidator) GetErrors() []AnnotationError {
	return v.errors
}
