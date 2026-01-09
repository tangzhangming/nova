package ast

import (
	"strings"

	"github.com/tangzhangming/nova/internal/token"
)

// Node 是所有 AST 节点的基接口
type Node interface {
	Pos() token.Position // 返回节点在源代码中的位置
	End() token.Position // 返回节点结束位置
	String() string      // 返回节点的字符串表示（用于调试）
}

// Expression 表示一个表达式节点
type Expression interface {
	Node
	exprNode()
}

// Statement 表示一个语句节点
type Statement interface {
	Node
	stmtNode()
}

// Declaration 表示一个声明节点
type Declaration interface {
	Node
	declNode()
}

// TypeNode 表示类型节点
type TypeNode interface {
	Node
	typeNode()
}

// ============================================================================
// 类型节点
// ============================================================================

// SimpleType 简单类型 (int, string, bool, etc.)
type SimpleType struct {
	Token token.Token // 类型 token
	Name  string      // 类型名称
}

func (t *SimpleType) Pos() token.Position { return t.Token.Pos }
func (t *SimpleType) End() token.Position { return t.Token.Pos }
func (t *SimpleType) String() string      { return t.Name }
func (t *SimpleType) typeNode()           {}

// NullableType 可空类型 (?Type)
type NullableType struct {
	Question token.Token // ? token
	Inner    TypeNode    // 内部类型
}

func (t *NullableType) Pos() token.Position { return t.Question.Pos }
func (t *NullableType) End() token.Position { return t.Inner.End() }
func (t *NullableType) String() string      { return "?" + t.Inner.String() }
func (t *NullableType) typeNode()           {}

// ArrayType 数组类型 (string[] 或 string[100])
type ArrayType struct {
	ElementType TypeNode    // 元素类型
	LBracket    token.Token // [ token
	Size        Expression  // 数组大小 (可为 nil，表示切片)
	RBracket    token.Token // ] token
}

func (t *ArrayType) Pos() token.Position { return t.ElementType.Pos() }
func (t *ArrayType) End() token.Position { return t.RBracket.Pos }
func (t *ArrayType) String() string {
	if t.Size != nil {
		return t.ElementType.String() + "[" + t.Size.String() + "]"
	}
	return t.ElementType.String() + "[]"
}
func (t *ArrayType) typeNode() {}

// MapType 映射类型 (map[KeyType]ValueType)
type MapType struct {
	MapToken  token.Token // map token
	KeyType   TypeNode    // 键类型
	ValueType TypeNode    // 值类型
}

func (t *MapType) Pos() token.Position { return t.MapToken.Pos }
func (t *MapType) End() token.Position { return t.ValueType.End() }
func (t *MapType) String() string {
	return "map[" + t.KeyType.String() + "]" + t.ValueType.String()
}
func (t *MapType) typeNode() {}

// FuncType 函数/闭包类型
type FuncType struct {
	FuncToken  token.Token // func token
	Params     []TypeNode  // 参数类型
	ReturnType TypeNode    // 返回类型 (可以是 TupleType)
}

func (t *FuncType) Pos() token.Position { return t.FuncToken.Pos }
func (t *FuncType) End() token.Position {
	if t.ReturnType != nil {
		return t.ReturnType.End()
	}
	return t.FuncToken.Pos
}
func (t *FuncType) String() string {
	var params []string
	for _, p := range t.Params {
		params = append(params, p.String())
	}
	result := "func(" + strings.Join(params, ", ") + ")"
	if t.ReturnType != nil {
		result += ": " + t.ReturnType.String()
	}
	return result
}
func (t *FuncType) typeNode() {}

// TupleType 多返回值类型 (int, string)
type TupleType struct {
	LParen token.Token // ( token
	Types  []TypeNode  // 类型列表
	RParen token.Token // ) token
}

func (t *TupleType) Pos() token.Position { return t.LParen.Pos }
func (t *TupleType) End() token.Position { return t.RParen.Pos }
func (t *TupleType) String() string {
	var types []string
	for _, typ := range t.Types {
		types = append(types, typ.String())
	}
	return "(" + strings.Join(types, ", ") + ")"
}
func (t *TupleType) typeNode() {}

// ClassType 类类型引用
type ClassType struct {
	Name token.Token // 类名
}

func (t *ClassType) Pos() token.Position { return t.Name.Pos }
func (t *ClassType) End() token.Position { return t.Name.Pos }
func (t *ClassType) String() string      { return t.Name.Literal }
func (t *ClassType) typeNode()           {}

// UnionType 联合类型 (Type1 | Type2)
type UnionType struct {
	Types []TypeNode // 联合的类型列表（至少2个）
}

func (t *UnionType) Pos() token.Position { return t.Types[0].Pos() }
func (t *UnionType) End() token.Position { return t.Types[len(t.Types)-1].End() }
func (t *UnionType) String() string {
	var parts []string
	for _, typ := range t.Types {
		parts = append(parts, typ.String())
	}
	return strings.Join(parts, " | ")
}
func (t *UnionType) typeNode() {}

// NullType null 类型（用于联合类型中表示 null）
type NullType struct {
	Token token.Token // null token (可选)
}

func (t *NullType) Pos() token.Position { return t.Token.Pos }
func (t *NullType) End() token.Position { return t.Token.Pos }
func (t *NullType) String() string      { return "null" }
func (t *NullType) typeNode()           {}

// TypeParameter 泛型类型参数 <T extends Comparable<T> implements IComparable, ISerializable>
type TypeParameter struct {
	Name            *Identifier // 类型参数名 (T, K, V 等)
	Constraint      TypeNode    // 约束类型 (extends 后的类型)，可为 nil
	ImplementsTypes []TypeNode // implements 接口列表，可为 nil
}

func (t *TypeParameter) Pos() token.Position { return t.Name.Pos() }
func (t *TypeParameter) End() token.Position {
	if len(t.ImplementsTypes) > 0 {
		return t.ImplementsTypes[len(t.ImplementsTypes)-1].End()
	}
	if t.Constraint != nil {
		return t.Constraint.End()
	}
	return t.Name.End()
}
func (t *TypeParameter) String() string {
	result := t.Name.String()
	if t.Constraint != nil {
		result += " extends " + t.Constraint.String()
	}
	if len(t.ImplementsTypes) > 0 {
		var ifaces []string
		for _, iface := range t.ImplementsTypes {
			ifaces = append(ifaces, iface.String())
		}
		result += " implements " + strings.Join(ifaces, ", ")
	}
	return result
}
func (t *TypeParameter) typeNode() {}

// GenericType 泛型类型实例化 List<int>, Map<string, User>
type GenericType struct {
	BaseType TypeNode    // 基础类型
	LAngle   token.Token // <
	TypeArgs []TypeNode  // 类型参数列表
	RAngle   token.Token // >
}

func (t *GenericType) Pos() token.Position { return t.BaseType.Pos() }
func (t *GenericType) End() token.Position { return t.RAngle.Pos }
func (t *GenericType) String() string {
	var args []string
	for _, arg := range t.TypeArgs {
		args = append(args, arg.String())
	}
	return t.BaseType.String() + "<" + strings.Join(args, ", ") + ">"
}
func (t *GenericType) typeNode() {}

// ============================================================================
// 表达式节点
// ============================================================================

// Identifier 标识符
type Identifier struct {
	Token token.Token
	Name  string
}

func (e *Identifier) Pos() token.Position { return e.Token.Pos }
func (e *Identifier) End() token.Position { return e.Token.Pos }
func (e *Identifier) String() string      { return e.Name }
func (e *Identifier) exprNode()           {}

// Variable 变量 ($name)
type Variable struct {
	Token token.Token
	Name  string // 不含 $ 前缀
}

func (e *Variable) Pos() token.Position { return e.Token.Pos }
func (e *Variable) End() token.Position { return e.Token.Pos }
func (e *Variable) String() string      { return "$" + e.Name }
func (e *Variable) exprNode()           {}

// ThisExpr $this
type ThisExpr struct {
	Token token.Token
}

func (e *ThisExpr) Pos() token.Position { return e.Token.Pos }
func (e *ThisExpr) End() token.Position { return e.Token.Pos }
func (e *ThisExpr) String() string      { return "$this" }
func (e *ThisExpr) exprNode()           {}

// IntegerLiteral 整数字面量
type IntegerLiteral struct {
	Token token.Token
	Value int64
}

func (e *IntegerLiteral) Pos() token.Position { return e.Token.Pos }
func (e *IntegerLiteral) End() token.Position { return e.Token.Pos }
func (e *IntegerLiteral) String() string      { return e.Token.Literal }
func (e *IntegerLiteral) exprNode()           {}

// FloatLiteral 浮点数字面量
type FloatLiteral struct {
	Token token.Token
	Value float64
}

func (e *FloatLiteral) Pos() token.Position { return e.Token.Pos }
func (e *FloatLiteral) End() token.Position { return e.Token.Pos }
func (e *FloatLiteral) String() string      { return e.Token.Literal }
func (e *FloatLiteral) exprNode()           {}

// StringLiteral 字符串字面量
type StringLiteral struct {
	Token token.Token
	Value string
}

func (e *StringLiteral) Pos() token.Position { return e.Token.Pos }
func (e *StringLiteral) End() token.Position { return e.Token.Pos }
func (e *StringLiteral) String() string      { return `"` + e.Value + `"` }
func (e *StringLiteral) exprNode()           {}

// InterpStringLiteral 插值字符串 #"..."
type InterpStringLiteral struct {
	Token token.Token
	Parts []Expression // 字符串部分和表达式混合
}

func (e *InterpStringLiteral) Pos() token.Position { return e.Token.Pos }
func (e *InterpStringLiteral) End() token.Position { return e.Token.Pos }
func (e *InterpStringLiteral) String() string      { return e.Token.Literal }
func (e *InterpStringLiteral) exprNode()           {}

// BoolLiteral 布尔字面量
type BoolLiteral struct {
	Token token.Token
	Value bool
}

func (e *BoolLiteral) Pos() token.Position { return e.Token.Pos }
func (e *BoolLiteral) End() token.Position { return e.Token.Pos }
func (e *BoolLiteral) String() string {
	if e.Value {
		return "true"
	}
	return "false"
}
func (e *BoolLiteral) exprNode() {}

// NullLiteral null 字面量
type NullLiteral struct {
	Token token.Token
}

func (e *NullLiteral) Pos() token.Position { return e.Token.Pos }
func (e *NullLiteral) End() token.Position { return e.Token.Pos }
func (e *NullLiteral) String() string      { return "null" }
func (e *NullLiteral) exprNode()           {}

// ArrayLiteral 数组字面量 int{1, 2, 3} 或 {1, 2, 3}（从上下文推断类型）
type ArrayLiteral struct {
	ElementType TypeNode    // 元素类型，可为 nil（从上下文推断）
	LBrace      token.Token // {
	Elements    []Expression
	RBrace      token.Token // }
}

func (e *ArrayLiteral) Pos() token.Position {
	if e.ElementType != nil {
		return e.ElementType.Pos()
	}
	return e.LBrace.Pos
}
func (e *ArrayLiteral) End() token.Position { return e.RBrace.Pos }
func (e *ArrayLiteral) String() string {
	var elems []string
	for _, elem := range e.Elements {
		elems = append(elems, elem.String())
	}
	typeStr := ""
	if e.ElementType != nil {
		typeStr = e.ElementType.String()
	}
	return typeStr + "{" + strings.Join(elems, ", ") + "}"
}
func (e *ArrayLiteral) exprNode() {}

// MapLiteral Map字面量 map[string]int{"key": value, ...} 或 {"key": value}（从上下文推断类型）
type MapLiteral struct {
	MapToken  token.Token // map 关键字，可为空 token
	KeyType   TypeNode    // 键类型，可为 nil
	ValueType TypeNode    // 值类型，可为 nil
	LBrace    token.Token // {
	Pairs     []MapPair
	RBrace    token.Token // }
}

type MapPair struct {
	Key   Expression
	Colon token.Token // : (Go风格)
	Value Expression
}

func (e *MapLiteral) Pos() token.Position {
	if e.MapToken.Type != 0 {
		return e.MapToken.Pos
	}
	return e.LBrace.Pos
}
func (e *MapLiteral) End() token.Position { return e.RBrace.Pos }
func (e *MapLiteral) String() string {
	var pairs []string
	for _, p := range e.Pairs {
		pairs = append(pairs, p.Key.String()+": "+p.Value.String())
	}
	typeStr := ""
	if e.KeyType != nil && e.ValueType != nil {
		typeStr = "map[" + e.KeyType.String() + "]" + e.ValueType.String()
	}
	return typeStr + "{" + strings.Join(pairs, ", ") + "}"
}
func (e *MapLiteral) exprNode() {}

// SuperArrayLiteral PHP风格万能数组字面量 [1, 2, "name" => "Sola"]
type SuperArrayLiteral struct {
	LBracket token.Token         // [
	Elements []SuperArrayElement // 元素列表
	RBracket token.Token         // ]
}

// SuperArrayElement 万能数组元素（可以是值或键值对）
type SuperArrayElement struct {
	Key   Expression  // 键，nil 表示自动索引
	Arrow token.Token // => (仅键值对时有值)
	Value Expression  // 值
}

func (e *SuperArrayLiteral) Pos() token.Position { return e.LBracket.Pos }
func (e *SuperArrayLiteral) End() token.Position { return e.RBracket.Pos }
func (e *SuperArrayLiteral) String() string {
	var elems []string
	for _, elem := range e.Elements {
		if elem.Key != nil {
			elems = append(elems, elem.Key.String()+" => "+elem.Value.String())
		} else {
			elems = append(elems, elem.Value.String())
		}
	}
	return "[" + strings.Join(elems, ", ") + "]"
}
func (e *SuperArrayLiteral) exprNode() {}

// UnaryExpr 一元表达式 (!x, -x, ++x, --x)
type UnaryExpr struct {
	Operator token.Token
	Operand  Expression
	Prefix   bool // true: !x, false: x++ (后缀)
}

func (e *UnaryExpr) Pos() token.Position {
	if e.Prefix {
		return e.Operator.Pos
	}
	return e.Operand.Pos()
}
func (e *UnaryExpr) End() token.Position {
	if e.Prefix {
		return e.Operand.End()
	}
	return e.Operator.Pos
}
func (e *UnaryExpr) String() string {
	if e.Prefix {
		return e.Operator.Literal + e.Operand.String()
	}
	return e.Operand.String() + e.Operator.Literal
}
func (e *UnaryExpr) exprNode() {}

// BinaryExpr 二元表达式 (a + b, a == b, etc.)
type BinaryExpr struct {
	Left     Expression
	Operator token.Token
	Right    Expression
}

func (e *BinaryExpr) Pos() token.Position { return e.Left.Pos() }
func (e *BinaryExpr) End() token.Position { return e.Right.End() }
func (e *BinaryExpr) String() string {
	return "(" + e.Left.String() + " " + e.Operator.Literal + " " + e.Right.String() + ")"
}
func (e *BinaryExpr) exprNode() {}

// IsExpr 类型检查表达式 ($x is string, $obj is MyClass)
// 用于类型收窄：在 if($x is string) 分支内，$x 的类型被收窄为 string
type IsExpr struct {
	Expr     Expression  // 被检查的表达式
	IsToken  token.Token // is 关键字
	Negated  bool        // 是否取反 (!is)
	TypeName TypeNode    // 目标类型
}

func (e *IsExpr) Pos() token.Position { return e.Expr.Pos() }
func (e *IsExpr) End() token.Position { return e.TypeName.End() }
func (e *IsExpr) String() string {
	if e.Negated {
		return e.Expr.String() + " !is " + e.TypeName.String()
	}
	return e.Expr.String() + " is " + e.TypeName.String()
}
func (e *IsExpr) exprNode() {}

// TernaryExpr 三元表达式 (cond ? then : else)
type TernaryExpr struct {
	Condition Expression
	Question  token.Token
	Then      Expression
	Colon     token.Token
	Else      Expression
}

func (e *TernaryExpr) Pos() token.Position { return e.Condition.Pos() }
func (e *TernaryExpr) End() token.Position { return e.Else.End() }
func (e *TernaryExpr) String() string {
	return "(" + e.Condition.String() + " ? " + e.Then.String() + " : " + e.Else.String() + ")"
}
func (e *TernaryExpr) exprNode() {}

// AssignExpr 赋值表达式 ($a = 1, $a += 1)
type AssignExpr struct {
	Left     Expression // 可以是 Variable, IndexExpr, PropertyAccess
	Operator token.Token
	Right    Expression
}

func (e *AssignExpr) Pos() token.Position { return e.Left.Pos() }
func (e *AssignExpr) End() token.Position { return e.Right.End() }
func (e *AssignExpr) String() string {
	return e.Left.String() + " " + e.Operator.Literal + " " + e.Right.String()
}
func (e *AssignExpr) exprNode() {}

// NamedArgument 命名参数 (name: value)
type NamedArgument struct {
	Name  *Identifier
	Colon token.Token
	Value Expression
}

func (n *NamedArgument) Pos() token.Position { return n.Name.Pos() }
func (n *NamedArgument) End() token.Position { return n.Value.End() }
func (n *NamedArgument) String() string {
	return n.Name.String() + ": " + n.Value.String()
}

// CallExpr 函数/方法调用
type CallExpr struct {
	Function       Expression // 被调用的函数
	LParen         token.Token
	Arguments      []Expression     // 位置参数
	NamedArguments []*NamedArgument // 命名参数
	RParen         token.Token
}

func (e *CallExpr) Pos() token.Position { return e.Function.Pos() }
func (e *CallExpr) End() token.Position { return e.RParen.Pos }
func (e *CallExpr) String() string {
	var args []string
	for _, arg := range e.Arguments {
		args = append(args, arg.String())
	}
	for _, na := range e.NamedArguments {
		args = append(args, na.String())
	}
	return e.Function.String() + "(" + strings.Join(args, ", ") + ")"
}
func (e *CallExpr) exprNode() {}

// IndexExpr 索引访问 ($arr[0], $map["key"])
type IndexExpr struct {
	Object   Expression
	LBracket token.Token
	Index    Expression
	RBracket token.Token
}

func (e *IndexExpr) Pos() token.Position { return e.Object.Pos() }
func (e *IndexExpr) End() token.Position { return e.RBracket.Pos }
func (e *IndexExpr) String() string {
	return e.Object.String() + "[" + e.Index.String() + "]"
}
func (e *IndexExpr) exprNode() {}

// PropertyAccess 属性访问 ($obj->property)
type PropertyAccess struct {
	Object   Expression
	Arrow    token.Token
	Property *Identifier
}

func (e *PropertyAccess) Pos() token.Position { return e.Object.Pos() }
func (e *PropertyAccess) End() token.Position { return e.Property.End() }
func (e *PropertyAccess) String() string {
	return e.Object.String() + "->" + e.Property.String()
}
func (e *PropertyAccess) exprNode() {}

// MethodCall 方法调用 ($obj->method())
type MethodCall struct {
	Object         Expression
	Arrow          token.Token
	Method         *Identifier
	LParen         token.Token
	Arguments      []Expression     // 位置参数
	NamedArguments []*NamedArgument // 命名参数
	RParen         token.Token
}

func (e *MethodCall) Pos() token.Position { return e.Object.Pos() }
func (e *MethodCall) End() token.Position { return e.RParen.Pos }
func (e *MethodCall) String() string {
	var args []string
	for _, arg := range e.Arguments {
		args = append(args, arg.String())
	}
	for _, na := range e.NamedArguments {
		args = append(args, na.String())
	}
	return e.Object.String() + "->" + e.Method.String() + "(" + strings.Join(args, ", ") + ")"
}
func (e *MethodCall) exprNode() {}

// SafePropertyAccess 安全属性访问 ($obj?.prop)
type SafePropertyAccess struct {
	Object   Expression
	SafeDot  token.Token
	Property *Identifier
}

func (e *SafePropertyAccess) Pos() token.Position { return e.Object.Pos() }
func (e *SafePropertyAccess) End() token.Position { return e.Property.End() }
func (e *SafePropertyAccess) String() string {
	return e.Object.String() + "?." + e.Property.String()
}
func (e *SafePropertyAccess) exprNode() {}

// SafeMethodCall 安全方法调用 ($obj?.method())
type SafeMethodCall struct {
	Object         Expression
	SafeDot        token.Token
	Method         *Identifier
	LParen         token.Token
	Arguments      []Expression
	NamedArguments []*NamedArgument
	RParen         token.Token
}

func (e *SafeMethodCall) Pos() token.Position { return e.Object.Pos() }
func (e *SafeMethodCall) End() token.Position { return e.RParen.Pos }
func (e *SafeMethodCall) String() string {
	var args []string
	for _, arg := range e.Arguments {
		args = append(args, arg.String())
	}
	for _, na := range e.NamedArguments {
		args = append(args, na.String())
	}
	return e.Object.String() + "?." + e.Method.String() + "(" + strings.Join(args, ", ") + ")"
}
func (e *SafeMethodCall) exprNode() {}

// NullCoalesceExpr 空合并表达式 ($a ?? $b)
type NullCoalesceExpr struct {
	Left     Expression
	Operator token.Token
	Right    Expression
}

func (e *NullCoalesceExpr) Pos() token.Position { return e.Left.Pos() }
func (e *NullCoalesceExpr) End() token.Position { return e.Right.End() }
func (e *NullCoalesceExpr) String() string {
	return e.Left.String() + " ?? " + e.Right.String()
}
func (e *NullCoalesceExpr) exprNode() {}

// NonNullAssertExpr 非空断言表达式 (expr!!)
// BUG FIX 2026-01-10: 空安全系统完善 - 添加非空断言表达式
// 用于在确定值非空时移除可空类型，如果运行时值为null则抛出异常
type NonNullAssertExpr struct {
	Expr     Expression  // 被断言的表达式
	Operator token.Token // !! 操作符
}

func (e *NonNullAssertExpr) Pos() token.Position { return e.Expr.Pos() }
func (e *NonNullAssertExpr) End() token.Position { return e.Operator.Pos }
func (e *NonNullAssertExpr) String() string      { return e.Expr.String() + "!!" }
func (e *NonNullAssertExpr) exprNode()           {}

// StaticAccess 静态访问 (Class::CONST, Class::$prop, self::method())
type StaticAccess struct {
	Class       Expression // 可以是 Identifier (类名), SelfExpr, ParentExpr
	DoubleColon token.Token
	Member      Expression // Identifier, Variable, 或 CallExpr
}

func (e *StaticAccess) Pos() token.Position { return e.Class.Pos() }
func (e *StaticAccess) End() token.Position { return e.Member.End() }
func (e *StaticAccess) String() string {
	return e.Class.String() + "::" + e.Member.String()
}
func (e *StaticAccess) exprNode() {}

// SelfExpr self 关键字
type SelfExpr struct {
	Token token.Token
}

func (e *SelfExpr) Pos() token.Position { return e.Token.Pos }
func (e *SelfExpr) End() token.Position { return e.Token.Pos }
func (e *SelfExpr) String() string      { return "self" }
func (e *SelfExpr) exprNode()           {}

// ParentExpr parent 关键字
type ParentExpr struct {
	Token token.Token
}

func (e *ParentExpr) Pos() token.Position { return e.Token.Pos }
func (e *ParentExpr) End() token.Position { return e.Token.Pos }
func (e *ParentExpr) String() string      { return "parent" }
func (e *ParentExpr) exprNode()           {}

// NewExpr new 表达式 (new User() 或 new Box<int>())
type NewExpr struct {
	NewToken       token.Token
	ClassName      *Identifier
	TypeArgs       []TypeNode // 泛型类型参数 <int, string>
	LParen         token.Token
	Arguments      []Expression     // 位置参数
	NamedArguments []*NamedArgument // 命名参数
	RParen         token.Token
}

func (e *NewExpr) Pos() token.Position { return e.NewToken.Pos }
func (e *NewExpr) End() token.Position { return e.RParen.Pos }
func (e *NewExpr) String() string {
	var args []string
	for _, arg := range e.Arguments {
		args = append(args, arg.String())
	}
	for _, na := range e.NamedArguments {
		args = append(args, na.String())
	}
	typeArgsStr := ""
	if len(e.TypeArgs) > 0 {
		var typeArgStrs []string
		for _, ta := range e.TypeArgs {
			typeArgStrs = append(typeArgStrs, ta.String())
		}
		typeArgsStr = "<" + strings.Join(typeArgStrs, ", ") + ">"
	}
	return "new " + e.ClassName.String() + typeArgsStr + "(" + strings.Join(args, ", ") + ")"
}
func (e *NewExpr) exprNode() {}

// ClosureExpr 闭包表达式
type ClosureExpr struct {
	FuncToken  token.Token
	LParen     token.Token
	Parameters []*Parameter
	RParen     token.Token
	UseVars    []*Variable // use ($a, $b) 捕获的变量
	ReturnType TypeNode    // 可为 nil
	Body       *BlockStmt
}

func (e *ClosureExpr) Pos() token.Position { return e.FuncToken.Pos }
func (e *ClosureExpr) End() token.Position { return e.Body.End() }
func (e *ClosureExpr) String() string      { return "function(...) {...}" }
func (e *ClosureExpr) exprNode()           {}

// ArrowFuncExpr 箭头函数 ((int $x): int => $x + 1)
type ArrowFuncExpr struct {
	LParen     token.Token
	Parameters []*Parameter
	RParen     token.Token
	ReturnType TypeNode // 可为 nil
	Arrow      token.Token
	Body       Expression
}

func (e *ArrowFuncExpr) Pos() token.Position { return e.LParen.Pos }
func (e *ArrowFuncExpr) End() token.Position { return e.Body.End() }
func (e *ArrowFuncExpr) String() string      { return "(...) => ..." }
func (e *ArrowFuncExpr) exprNode()           {}

// ClassAccessExpr 获取类名 ($obj::class)
type ClassAccessExpr struct {
	Object      Expression
	DoubleColon token.Token
	Class       token.Token
}

func (e *ClassAccessExpr) Pos() token.Position { return e.Object.Pos() }
func (e *ClassAccessExpr) End() token.Position { return e.Class.Pos }
func (e *ClassAccessExpr) String() string      { return e.Object.String() + "::class" }
func (e *ClassAccessExpr) exprNode()           {}

// TypeCastExpr 类型断言表达式 ($expr as Type / $expr as? Type)
type TypeCastExpr struct {
	Expr       Expression  // 被转换的表达式
	AsToken    token.Token // as 或 as? token
	Safe       bool        // true = as?, false = as
	TargetType TypeNode    // 目标类型
}

func (e *TypeCastExpr) Pos() token.Position { return e.Expr.Pos() }
func (e *TypeCastExpr) End() token.Position { return e.TargetType.End() }
func (e *TypeCastExpr) String() string {
	if e.Safe {
		return "(" + e.Expr.String() + " as? " + e.TargetType.String() + ")"
	}
	return "(" + e.Expr.String() + " as " + e.TargetType.String() + ")"
}
func (e *TypeCastExpr) exprNode() {}

// ============================================================================
// 语句节点
// ============================================================================

// ExprStmt 表达式语句
type ExprStmt struct {
	Expr      Expression
	Semicolon token.Token
}

func (s *ExprStmt) Pos() token.Position { return s.Expr.Pos() }
func (s *ExprStmt) End() token.Position { return s.Semicolon.Pos }
func (s *ExprStmt) String() string      { return s.Expr.String() + ";" }
func (s *ExprStmt) stmtNode()           {}

// VarDeclStmt 变量声明语句 (int $a = 1; 或 $a := 1;)
type VarDeclStmt struct {
	Type      TypeNode // 类型（如果是 := 则为 nil）
	Name      *Variable
	Operator  token.Token // = 或 :=
	Value     Expression  // 初始值（可为 nil）
	Semicolon token.Token
}

func (s *VarDeclStmt) Pos() token.Position {
	if s.Type != nil {
		return s.Type.Pos()
	}
	return s.Name.Pos()
}
func (s *VarDeclStmt) End() token.Position { return s.Semicolon.Pos }
func (s *VarDeclStmt) String() string {
	var sb strings.Builder
	if s.Type != nil {
		sb.WriteString(s.Type.String())
		sb.WriteString(" ")
	}
	sb.WriteString(s.Name.String())
	sb.WriteString(" ")
	sb.WriteString(s.Operator.Literal)
	if s.Value != nil {
		sb.WriteString(" ")
		sb.WriteString(s.Value.String())
	}
	sb.WriteString(";")
	return sb.String()
}
func (s *VarDeclStmt) stmtNode() {}

// MultiVarDeclStmt 多变量声明 ($a, $b := test();)
type MultiVarDeclStmt struct {
	Names     []*Variable
	Operator  token.Token // = 或 :=
	Value     Expression
	Semicolon token.Token
}

func (s *MultiVarDeclStmt) Pos() token.Position { return s.Names[0].Pos() }
func (s *MultiVarDeclStmt) End() token.Position { return s.Semicolon.Pos }
func (s *MultiVarDeclStmt) String() string {
	var names []string
	for _, n := range s.Names {
		names = append(names, n.String())
	}
	return strings.Join(names, ", ") + " " + s.Operator.Literal + " " + s.Value.String() + ";"
}
func (s *MultiVarDeclStmt) stmtNode() {}

// BlockStmt 代码块
type BlockStmt struct {
	LBrace     token.Token
	Statements []Statement
	RBrace     token.Token
}

func (s *BlockStmt) Pos() token.Position { return s.LBrace.Pos }
func (s *BlockStmt) End() token.Position { return s.RBrace.Pos }
func (s *BlockStmt) String() string {
	var stmts []string
	for _, stmt := range s.Statements {
		stmts = append(stmts, stmt.String())
	}
	return "{ " + strings.Join(stmts, " ") + " }"
}
func (s *BlockStmt) stmtNode() {}

// IfStmt if 语句
type IfStmt struct {
	IfToken   token.Token
	Condition Expression
	Then      *BlockStmt
	ElseIfs   []*ElseIfClause
	Else      *BlockStmt // 可为 nil
}

type ElseIfClause struct {
	ElseIfToken token.Token
	Condition   Expression
	Body        *BlockStmt
}

func (s *IfStmt) Pos() token.Position { return s.IfToken.Pos }
func (s *IfStmt) End() token.Position {
	if s.Else != nil {
		return s.Else.End()
	}
	if len(s.ElseIfs) > 0 {
		return s.ElseIfs[len(s.ElseIfs)-1].Body.End()
	}
	return s.Then.End()
}
func (s *IfStmt) String() string { return "if (...) {...}" }
func (s *IfStmt) stmtNode()      {}

// SwitchStmt switch 语句
// SwitchStmt switch 语句（支持多值和两种形式）
type SwitchStmt struct {
	SwitchToken token.Token
	LParen      token.Token
	Expr        Expression
	RParen      token.Token
	LBrace      token.Token
	Cases       []*SwitchCase
	Default     *SwitchDefaultCase // 可为 nil
	RBrace      token.Token
}

// SwitchExpr switch 表达式（返回值）
type SwitchExpr struct {
	SwitchToken token.Token
	LParen      token.Token
	Expr        Expression
	RParen      token.Token
	LBrace      token.Token
	Cases       []*SwitchCase
	Default     *SwitchDefaultCase // 可为 nil
	RBrace      token.Token
}

// SwitchCase case 子句（支持多值：case 1, 2, 3 和两种形式：=> 或 :）
type SwitchCase struct {
	CaseToken token.Token
	Values    []Expression // 多个值：case 1, 2, 3
	Arrow     token.Token  // => token（表达式形式）
	Colon     token.Token  // : token（语句形式）
	Body      interface{}  // Expression（=>形式）或 []Statement（:形式）
}

// SwitchDefaultCase default 子句（支持两种形式）
type SwitchDefaultCase struct {
	DefaultToken token.Token
	Arrow        token.Token // => token（表达式形式）
	Colon        token.Token // : token（语句形式）
	Body         interface{} // Expression（=>形式）或 []Statement（:形式）
}

// 保留旧类型别名以兼容现有代码（临时）
type CaseClause = SwitchCase
type DefaultClause = SwitchDefaultCase

func (s *SwitchStmt) Pos() token.Position { return s.SwitchToken.Pos }
func (s *SwitchStmt) End() token.Position { return s.RBrace.Pos }
func (s *SwitchStmt) String() string      { return "switch (...) {...}" }
func (s *SwitchStmt) stmtNode()           {}

func (e *SwitchExpr) Pos() token.Position { return e.SwitchToken.Pos }
func (e *SwitchExpr) End() token.Position { return e.RBrace.Pos }
func (e *SwitchExpr) String() string      { return "switch (...) {...}" }
func (e *SwitchExpr) exprNode()           {}

// ForStmt for 语句
type ForStmt struct {
	ForToken  token.Token
	Init      Statement  // 可为 nil
	Condition Expression // 可为 nil
	Post      Expression // 可为 nil
	Body      *BlockStmt
}

func (s *ForStmt) Pos() token.Position { return s.ForToken.Pos }
func (s *ForStmt) End() token.Position { return s.Body.End() }
func (s *ForStmt) String() string      { return "for (...) {...}" }
func (s *ForStmt) stmtNode()           {}

// ForeachStmt foreach 语句
type ForeachStmt struct {
	ForeachToken token.Token
	Iterable     Expression
	AsToken      token.Token
	Key          *Variable // 可为 nil
	Value        *Variable
	Body         *BlockStmt
}

func (s *ForeachStmt) Pos() token.Position { return s.ForeachToken.Pos }
func (s *ForeachStmt) End() token.Position { return s.Body.End() }
func (s *ForeachStmt) String() string      { return "foreach (...) {...}" }
func (s *ForeachStmt) stmtNode()           {}

// WhileStmt while 语句
type WhileStmt struct {
	WhileToken token.Token
	Condition  Expression
	Body       *BlockStmt
}

func (s *WhileStmt) Pos() token.Position { return s.WhileToken.Pos }
func (s *WhileStmt) End() token.Position { return s.Body.End() }
func (s *WhileStmt) String() string      { return "while (...) {...}" }
func (s *WhileStmt) stmtNode()           {}

// DoWhileStmt do-while 语句
type DoWhileStmt struct {
	DoToken    token.Token
	Body       *BlockStmt
	WhileToken token.Token
	Condition  Expression
	Semicolon  token.Token
}

func (s *DoWhileStmt) Pos() token.Position { return s.DoToken.Pos }
func (s *DoWhileStmt) End() token.Position { return s.Semicolon.Pos }
func (s *DoWhileStmt) String() string      { return "do {...} while (...);" }
func (s *DoWhileStmt) stmtNode()           {}

// BreakStmt break 语句
type BreakStmt struct {
	BreakToken token.Token
	Semicolon  token.Token
}

func (s *BreakStmt) Pos() token.Position { return s.BreakToken.Pos }
func (s *BreakStmt) End() token.Position { return s.Semicolon.Pos }
func (s *BreakStmt) String() string      { return "break;" }
func (s *BreakStmt) stmtNode()           {}

// ContinueStmt continue 语句
type ContinueStmt struct {
	ContinueToken token.Token
	Semicolon     token.Token
}

func (s *ContinueStmt) Pos() token.Position { return s.ContinueToken.Pos }
func (s *ContinueStmt) End() token.Position { return s.Semicolon.Pos }
func (s *ContinueStmt) String() string      { return "continue;" }
func (s *ContinueStmt) stmtNode()           {}

// ReturnStmt return 语句
type ReturnStmt struct {
	ReturnToken token.Token
	Values      []Expression // 支持多返回值
	Semicolon   token.Token
}

func (s *ReturnStmt) Pos() token.Position { return s.ReturnToken.Pos }
func (s *ReturnStmt) End() token.Position { return s.Semicolon.Pos }
func (s *ReturnStmt) String() string {
	if len(s.Values) == 0 {
		return "return;"
	}
	var vals []string
	for _, v := range s.Values {
		vals = append(vals, v.String())
	}
	return "return " + strings.Join(vals, ", ") + ";"
}
func (s *ReturnStmt) stmtNode() {}

// TryStmt try-catch-finally 语句
type TryStmt struct {
	TryToken token.Token
	Try      *BlockStmt
	Catches  []*CatchClause
	Finally  *FinallyClause // 可为 nil
}

type CatchClause struct {
	CatchToken token.Token
	Type       TypeNode
	Variable   *Variable
	Body       *BlockStmt
}

type FinallyClause struct {
	FinallyToken token.Token
	Body         *BlockStmt
}

func (s *TryStmt) Pos() token.Position { return s.TryToken.Pos }
func (s *TryStmt) End() token.Position {
	if s.Finally != nil {
		return s.Finally.Body.End()
	}
	return s.Catches[len(s.Catches)-1].Body.End()
}
func (s *TryStmt) String() string { return "try {...} catch (...) {...}" }
func (s *TryStmt) stmtNode()      {}

// ThrowStmt throw 语句
type ThrowStmt struct {
	ThrowToken token.Token
	Exception  Expression
	Semicolon  token.Token
}

func (s *ThrowStmt) Pos() token.Position { return s.ThrowToken.Pos }
func (s *ThrowStmt) End() token.Position { return s.Semicolon.Pos }
func (s *ThrowStmt) String() string      { return "throw ...;" }
func (s *ThrowStmt) stmtNode()           {}

// EchoStmt echo 语句
type EchoStmt struct {
	EchoToken token.Token
	Value     Expression
	Semicolon token.Token
}

func (s *EchoStmt) Pos() token.Position { return s.EchoToken.Pos }
func (s *EchoStmt) End() token.Position { return s.Semicolon.Pos }
func (s *EchoStmt) String() string      { return "echo " + s.Value.String() + ";" }
func (s *EchoStmt) stmtNode()           {}

// GoStmt go 语句 (启动协程)
type GoStmt struct {
	GoToken   token.Token
	Call      Expression // 必须是函数调用表达式
	Semicolon token.Token
}

func (s *GoStmt) Pos() token.Position { return s.GoToken.Pos }
func (s *GoStmt) End() token.Position { return s.Semicolon.Pos }
func (s *GoStmt) String() string      { return "go " + s.Call.String() + ";" }
func (s *GoStmt) stmtNode()           {}

// SelectStmt select 语句 (多路选择)
type SelectStmt struct {
	SelectToken token.Token
	LBrace      token.Token
	Cases       []*SelectCase
	Default     *SelectDefaultCase
	RBrace      token.Token
}

func (s *SelectStmt) Pos() token.Position { return s.SelectToken.Pos }
func (s *SelectStmt) End() token.Position { return s.RBrace.Pos }
func (s *SelectStmt) String() string      { return "select {...}" }
func (s *SelectStmt) stmtNode()           {}

// SelectCase select 分支
type SelectCase struct {
	CaseToken token.Token
	Comm      Expression  // 通道操作: $ch->send($v) 或 $ch->receive()
	Var       *Variable   // 接收操作的目标变量 (可为 nil，用于 $v := $ch->receive())
	Operator  token.Token // := 或 = (用于接收赋值，可为空)
	Colon     token.Token
	Body      []Statement
}

func (s *SelectCase) Pos() token.Position { return s.CaseToken.Pos }
func (s *SelectCase) End() token.Position {
	if len(s.Body) > 0 {
		return s.Body[len(s.Body)-1].End()
	}
	return s.Colon.Pos
}
func (s *SelectCase) String() string { return "case ...:" }

// SelectDefaultCase select 默认分支
type SelectDefaultCase struct {
	DefaultToken token.Token
	Colon        token.Token
	Body         []Statement
}

func (s *SelectDefaultCase) Pos() token.Position { return s.DefaultToken.Pos }
func (s *SelectDefaultCase) End() token.Position {
	if len(s.Body) > 0 {
		return s.Body[len(s.Body)-1].End()
	}
	return s.Colon.Pos
}
func (s *SelectDefaultCase) String() string { return "default:" }

// ============================================================================
// 协程 OOP 节点
// ============================================================================

// AwaitExpr await 表达式: $task->await() 或 $task->await(timeout)
// 用于等待协程完成并获取结果
type AwaitExpr struct {
	Coroutine Expression  // 协程对象表达式
	Arrow     token.Token // ->
	AwaitTok  token.Token // await 方法名 token
	LParen    token.Token
	Timeout   Expression  // 可选超时参数（毫秒）
	RParen    token.Token
}

func (e *AwaitExpr) Pos() token.Position { return e.Coroutine.Pos() }
func (e *AwaitExpr) End() token.Position { return e.RParen.Pos }
func (e *AwaitExpr) String() string {
	if e.Timeout != nil {
		return e.Coroutine.String() + "->await(" + e.Timeout.String() + ")"
	}
	return e.Coroutine.String() + "->await()"
}
func (e *AwaitExpr) exprNode() {}

// CoroutineSpawnExpr 协程创建表达式: Coroutine::spawn(fn)
type CoroutineSpawnExpr struct {
	CoroutineTok token.Token // Coroutine token
	DoubleColon  token.Token // ::
	SpawnTok     token.Token // spawn token
	LParen       token.Token
	Closure      Expression  // 要执行的闭包/函数
	RParen       token.Token
}

func (e *CoroutineSpawnExpr) Pos() token.Position { return e.CoroutineTok.Pos }
func (e *CoroutineSpawnExpr) End() token.Position { return e.RParen.Pos }
func (e *CoroutineSpawnExpr) String() string {
	return "Coroutine::spawn(" + e.Closure.String() + ")"
}
func (e *CoroutineSpawnExpr) exprNode() {}

// CoroutineAllExpr 等待所有协程: Coroutine::all(tasks)
type CoroutineAllExpr struct {
	CoroutineTok token.Token
	DoubleColon  token.Token
	AllTok       token.Token
	LParen       token.Token
	Tasks        Expression  // 协程数组
	RParen       token.Token
}

func (e *CoroutineAllExpr) Pos() token.Position { return e.CoroutineTok.Pos }
func (e *CoroutineAllExpr) End() token.Position { return e.RParen.Pos }
func (e *CoroutineAllExpr) String() string {
	return "Coroutine::all(" + e.Tasks.String() + ")"
}
func (e *CoroutineAllExpr) exprNode() {}

// CoroutineAnyExpr 等待任一协程: Coroutine::any(tasks)
type CoroutineAnyExpr struct {
	CoroutineTok token.Token
	DoubleColon  token.Token
	AnyTok       token.Token
	LParen       token.Token
	Tasks        Expression
	RParen       token.Token
}

func (e *CoroutineAnyExpr) Pos() token.Position { return e.CoroutineTok.Pos }
func (e *CoroutineAnyExpr) End() token.Position { return e.RParen.Pos }
func (e *CoroutineAnyExpr) String() string {
	return "Coroutine::any(" + e.Tasks.String() + ")"
}
func (e *CoroutineAnyExpr) exprNode() {}

// CoroutineRaceExpr 竞速协程: Coroutine::race(tasks)
type CoroutineRaceExpr struct {
	CoroutineTok token.Token
	DoubleColon  token.Token
	RaceTok      token.Token
	LParen       token.Token
	Tasks        Expression
	RParen       token.Token
}

func (e *CoroutineRaceExpr) Pos() token.Position { return e.CoroutineTok.Pos }
func (e *CoroutineRaceExpr) End() token.Position { return e.RParen.Pos }
func (e *CoroutineRaceExpr) String() string {
	return "Coroutine::race(" + e.Tasks.String() + ")"
}
func (e *CoroutineRaceExpr) exprNode() {}

// CoroutineDelayExpr 延迟: Coroutine::delay(ms)
type CoroutineDelayExpr struct {
	CoroutineTok token.Token
	DoubleColon  token.Token
	DelayTok     token.Token
	LParen       token.Token
	Milliseconds Expression
	RParen       token.Token
}

func (e *CoroutineDelayExpr) Pos() token.Position { return e.CoroutineTok.Pos }
func (e *CoroutineDelayExpr) End() token.Position { return e.RParen.Pos }
func (e *CoroutineDelayExpr) String() string {
	return "Coroutine::delay(" + e.Milliseconds.String() + ")"
}
func (e *CoroutineDelayExpr) exprNode() {}

// ChannelSelectExpr 通道选择: Channel::select(cases)
type ChannelSelectExpr struct {
	ChannelTok  token.Token
	DoubleColon token.Token
	SelectTok   token.Token
	LParen      token.Token
	Cases       Expression  // SelectCase 数组
	RParen      token.Token
}

func (e *ChannelSelectExpr) Pos() token.Position { return e.ChannelTok.Pos }
func (e *ChannelSelectExpr) End() token.Position { return e.RParen.Pos }
func (e *ChannelSelectExpr) String() string {
	return "Channel::select(" + e.Cases.String() + ")"
}
func (e *ChannelSelectExpr) exprNode() {}

// ============================================================================
// 声明节点
// ============================================================================

// Visibility 访问修饰符
type Visibility int

const (
	VisibilityDefault Visibility = iota
	VisibilityPublic
	VisibilityProtected
	VisibilityPrivate
)

func (v Visibility) String() string {
	switch v {
	case VisibilityPublic:
		return "public"
	case VisibilityProtected:
		return "protected"
	case VisibilityPrivate:
		return "private"
	default:
		return ""
	}
}

// Annotation 注解
type Annotation struct {
	AtToken token.Token
	Name    *Identifier
	LParen  token.Token  // 可选
	Args    []Expression // 可选
	RParen  token.Token  // 可选
}

// Parameter 函数参数
type Parameter struct {
	Type     TypeNode   // 类型
	Variadic bool       // 是否是可变参数 (...)
	Name     *Variable  // 参数名
	Default  Expression // 默认值 (可为 nil)
}

func (p *Parameter) Pos() token.Position {
	if p.Type != nil {
		return p.Type.Pos()
	}
	return p.Name.Pos()
}
func (p *Parameter) End() token.Position {
	if p.Default != nil {
		return p.Default.End()
	}
	return p.Name.End()
}
func (p *Parameter) String() string {
	var sb strings.Builder
	if p.Type != nil {
		sb.WriteString(p.Type.String())
		sb.WriteString(" ")
	}
	if p.Variadic {
		sb.WriteString("...")
	}
	sb.WriteString(p.Name.String())
	if p.Default != nil {
		sb.WriteString(" = ")
		sb.WriteString(p.Default.String())
	}
	return sb.String()
}

// ConstDecl 常量声明
type ConstDecl struct {
	Annotations []*Annotation
	Visibility  Visibility
	ConstToken  token.Token
	Type        TypeNode
	Name        *Identifier
	Assign      token.Token
	Value       Expression
	Semicolon   token.Token
}

func (d *ConstDecl) Pos() token.Position { return d.ConstToken.Pos }
func (d *ConstDecl) End() token.Position { return d.Semicolon.Pos }
func (d *ConstDecl) String() string      { return "const " + d.Name.String() }
func (d *ConstDecl) declNode()           {}

// PropertyAccessor 属性访问器（getter/setter）
type PropertyAccessor struct {
	GetToken    token.Token  // get 关键字
	SetToken    token.Token  // set 关键字（可选）
	GetVis      Visibility   // getter 可见性（默认与属性相同）
	SetVis      Visibility   // setter 可见性（默认与属性相同）
	GetBody     *BlockStmt   // getter 方法体（可选，用于完整属性）
	SetBody     *BlockStmt   // setter 方法体（可选，用于完整属性）
	GetExpr     Expression   // getter 表达式体（可选，用于表达式体属性）
	SetExpr     Expression   // setter 表达式体（可选，用于表达式体属性）
	LBrace      token.Token  // { token（访问器块）
	RBrace      token.Token  // } token（访问器块）
}

func (a *PropertyAccessor) Pos() token.Position {
	if a.LBrace.Type != 0 {
		return a.LBrace.Pos
	}
	if a.GetToken.Type != 0 {
		return a.GetToken.Pos
	}
	return a.SetToken.Pos
}

func (a *PropertyAccessor) End() token.Position {
	if a.RBrace.Type != 0 {
		return a.RBrace.Pos
	}
	if a.SetBody != nil {
		return a.SetBody.End()
	}
	if a.GetBody != nil {
		return a.GetBody.End()
	}
	if a.SetExpr != nil {
		return a.SetExpr.End()
	}
	if a.GetExpr != nil {
		return a.GetExpr.End()
	}
	if a.SetToken.Type != 0 {
		return a.SetToken.Pos
	}
	return a.GetToken.Pos
}

// PropertyDecl 属性声明
type PropertyDecl struct {
	Annotations []*Annotation
	Visibility  Visibility
	Static      bool
	Final       bool // final 属性不能被重新赋值（类似 readonly）
	Type        TypeNode
	Name        *Variable
	Assign      token.Token        // 可选（用于普通字段初始值）
	Value       Expression         // 可选（用于普通字段初始值）
	Accessor    *PropertyAccessor  // 可选（用于自动属性或完整属性）
	ExprBody    Expression         // 可选（用于表达式体只读属性）
	Arrow       token.Token        // => token（用于表达式体属性）
	Semicolon   token.Token
}

func (d *PropertyDecl) Pos() token.Position { return d.Type.Pos() }
func (d *PropertyDecl) End() token.Position {
	if d.Semicolon.Type != 0 {
		return d.Semicolon.Pos
	}
	if d.Accessor != nil {
		return d.Accessor.End()
	}
	if d.ExprBody != nil {
		return d.ExprBody.End()
	}
	return d.Name.End()
}
func (d *PropertyDecl) String() string      { return d.Name.String() }
func (d *PropertyDecl) declNode()           {}

// MethodDecl 方法声明
type MethodDecl struct {
	Annotations []*Annotation
	Visibility  Visibility
	Static      bool
	Abstract    bool
	Final       bool // final 方法不能被重写
	FuncToken   token.Token
	Name        *Identifier
	TypeParams  []*TypeParameter // 泛型类型参数 <T, K extends Comparable>
	LParen      token.Token
	Parameters  []*Parameter
	RParen      token.Token
	ReturnType  TypeNode   // 可为 nil (void) 或 TupleType (多返回值)
	Body        *BlockStmt // 抽象方法为 nil
}

func (d *MethodDecl) Pos() token.Position { return d.FuncToken.Pos }
func (d *MethodDecl) End() token.Position {
	if d.Body != nil {
		return d.Body.End()
	}
	if d.ReturnType != nil {
		return d.ReturnType.End()
	}
	return d.RParen.Pos
}
func (d *MethodDecl) String() string { return "function " + d.Name.String() + "(...)" }
func (d *MethodDecl) declNode()      {}

// ClassDecl 类声明
type ClassDecl struct {
	Annotations []*Annotation
	Visibility  Visibility
	Abstract    bool
	Final       bool // final 类不能被继承
	ClassToken  token.Token
	Name        *Identifier
	TypeParams  []*TypeParameter // 泛型类型参数 <T, K extends Comparable>
	Extends     *Identifier      // 可为 nil
	Implements  []TypeNode       // 支持泛型接口 Container<T>
	WhereClause []*TypeParameter // where 子句约束，可为 nil
	LBrace      token.Token
	Constants   []*ConstDecl
	Properties  []*PropertyDecl
	Methods     []*MethodDecl
	RBrace      token.Token
}

func (d *ClassDecl) Pos() token.Position { return d.ClassToken.Pos }
func (d *ClassDecl) End() token.Position { return d.RBrace.Pos }
func (d *ClassDecl) String() string      { return "class " + d.Name.String() }
func (d *ClassDecl) declNode()           {}

// InterfaceDecl 接口声明
type InterfaceDecl struct {
	Annotations    []*Annotation
	Visibility     Visibility
	InterfaceToken token.Token
	Name           *Identifier
	TypeParams     []*TypeParameter // 泛型类型参数 <T, K extends Comparable>
	Extends        []TypeNode       // 支持泛型接口 Comparable<T>
	WhereClause    []*TypeParameter // where 子句约束，可为 nil
	LBrace         token.Token
	Methods        []*MethodDecl
	RBrace         token.Token
}

func (d *InterfaceDecl) Pos() token.Position { return d.InterfaceToken.Pos }
func (d *InterfaceDecl) End() token.Position { return d.RBrace.Pos }
func (d *InterfaceDecl) String() string      { return "interface " + d.Name.String() }
func (d *InterfaceDecl) declNode()           {}

// EnumDecl 枚举声明
type EnumDecl struct {
	EnumToken token.Token
	Name      *Identifier
	Type      TypeNode // 可选的基础类型 (int/string)
	LBrace    token.Token
	Cases     []*EnumCase
	RBrace    token.Token
}

func (d *EnumDecl) Pos() token.Position { return d.EnumToken.Pos }
func (d *EnumDecl) End() token.Position { return d.RBrace.Pos }
func (d *EnumDecl) String() string      { return "enum " + d.Name.String() }
func (d *EnumDecl) declNode()           {}

// EnumCase 枚举成员
type EnumCase struct {
	Name  *Identifier
	Value Expression // 可选的值
}

func (c *EnumCase) Pos() token.Position { return c.Name.Pos() }
func (c *EnumCase) End() token.Position { return c.Name.End() }
func (c *EnumCase) String() string      { return c.Name.String() }

// TypeAliasDecl 类型别名声明 (type StringList = string[])
// 类型别名创建与目标类型完全兼容的新名称
type TypeAliasDecl struct {
	TypeToken token.Token // type 关键字
	Name      *Identifier // 别名名称
	Equals    token.Token // = 符号
	AliasType TypeNode    // 目标类型
}

func (d *TypeAliasDecl) Pos() token.Position { return d.TypeToken.Pos }
func (d *TypeAliasDecl) End() token.Position { return d.AliasType.End() }
func (d *TypeAliasDecl) String() string {
	return "type " + d.Name.Name + " = " + d.AliasType.String()
}
func (d *TypeAliasDecl) declNode() {}

// NewTypeDecl 新类型声明 (type UserID int)
// 新类型创建与基础类型不兼容的独立类型，需要显式转换
type NewTypeDecl struct {
	TypeToken token.Token // type 关键字
	Name      *Identifier // 新类型名称
	BaseType  TypeNode    // 基础类型
}

func (d *NewTypeDecl) Pos() token.Position { return d.TypeToken.Pos }
func (d *NewTypeDecl) End() token.Position { return d.BaseType.End() }
func (d *NewTypeDecl) String() string {
	return "type " + d.Name.Name + " " + d.BaseType.String()
}
func (d *NewTypeDecl) declNode() {}

// NamespaceDecl 命名空间声明
type NamespaceDecl struct {
	NamespaceToken token.Token
	Name           string // 完整的命名空间路径 (e.g., "company.project")
}

func (d *NamespaceDecl) Pos() token.Position { return d.NamespaceToken.Pos }
func (d *NamespaceDecl) End() token.Position { return d.NamespaceToken.Pos }
func (d *NamespaceDecl) String() string      { return "namespace " + d.Name }
func (d *NamespaceDecl) declNode()           {}

// UseDecl use 声明
type UseDecl struct {
	UseToken token.Token
	Path     string      // 完整路径 (e.g., "company.project.models.User")
	Alias    *Identifier // 别名 (可为 nil)
}

func (d *UseDecl) Pos() token.Position { return d.UseToken.Pos }
func (d *UseDecl) End() token.Position { return d.UseToken.Pos }
func (d *UseDecl) String() string {
	if d.Alias != nil {
		return "use " + d.Path + " as " + d.Alias.String()
	}
	return "use " + d.Path
}
func (d *UseDecl) declNode() {}

// ============================================================================
// 文件/程序节点
// ============================================================================

// File 表示一个源文件
type File struct {
	Filename     string
	Namespace    *NamespaceDecl
	Uses         []*UseDecl
	Declarations []Declaration // 类、接口等声明
	Statements   []Statement   // 顶层语句 (入口文件)
}

func (f *File) Pos() token.Position {
	if f.Namespace != nil {
		return f.Namespace.Pos()
	}
	if len(f.Uses) > 0 {
		return f.Uses[0].Pos()
	}
	if len(f.Declarations) > 0 {
		return f.Declarations[0].Pos()
	}
	if len(f.Statements) > 0 {
		return f.Statements[0].Pos()
	}
	return token.Position{}
}

func (f *File) End() token.Position {
	if len(f.Statements) > 0 {
		return f.Statements[len(f.Statements)-1].End()
	}
	if len(f.Declarations) > 0 {
		return f.Declarations[len(f.Declarations)-1].End()
	}
	return token.Position{}
}

func (f *File) String() string {
	return f.Filename
}

// Program 表示整个程序
type Program struct {
	Files []*File
}

// Visitor 访问者函数类型
type Visitor func(node Node) bool

// Walk 遍历 AST 节点
func Walk(node Node, visitor Visitor) {
	if node == nil {
		return
	}
	
	if !visitor(node) {
		return
	}

	switch n := node.(type) {
	case *File:
		if n.Namespace != nil {
			Walk(n.Namespace, visitor)
		}
		for _, u := range n.Uses {
			Walk(u, visitor)
		}
		for _, d := range n.Declarations {
			Walk(d, visitor)
		}
		for _, s := range n.Statements {
			Walk(s, visitor)
		}

	case *BlockStmt:
		for _, stmt := range n.Statements {
			Walk(stmt, visitor)
		}

	case *IfStmt:
		Walk(n.Condition, visitor)
		Walk(n.Then, visitor)
		for _, elseIf := range n.ElseIfs {
			Walk(elseIf.Condition, visitor)
			Walk(elseIf.Body, visitor)
		}
		if n.Else != nil {
			Walk(n.Else, visitor)
		}

	case *ForStmt:
		if n.Init != nil {
			Walk(n.Init, visitor)
		}
		if n.Condition != nil {
			Walk(n.Condition, visitor)
		}
		if n.Post != nil {
			Walk(n.Post, visitor)
		}
		Walk(n.Body, visitor)

	case *WhileStmt:
		Walk(n.Condition, visitor)
		Walk(n.Body, visitor)

	case *ForeachStmt:
		Walk(n.Iterable, visitor)
		Walk(n.Body, visitor)

	case *DoWhileStmt:
		Walk(n.Body, visitor)
		Walk(n.Condition, visitor)

	case *BinaryExpr:
		Walk(n.Left, visitor)
		Walk(n.Right, visitor)

	case *UnaryExpr:
		Walk(n.Operand, visitor)

	case *AssignExpr:
		Walk(n.Left, visitor)
		Walk(n.Right, visitor)

	case *CallExpr:
		Walk(n.Function, visitor)
		for _, arg := range n.Arguments {
			Walk(arg, visitor)
		}
		for _, na := range n.NamedArguments {
			Walk(na.Value, visitor)
		}

	case *MethodCall:
		Walk(n.Object, visitor)
		for _, arg := range n.Arguments {
			Walk(arg, visitor)
		}
		for _, na := range n.NamedArguments {
			Walk(na.Value, visitor)
		}

	case *IndexExpr:
		Walk(n.Object, visitor)
		Walk(n.Index, visitor)

	case *PropertyAccess:
		Walk(n.Object, visitor)

	case *TernaryExpr:
		Walk(n.Condition, visitor)
		Walk(n.Then, visitor)
		Walk(n.Else, visitor)

	case *VarDeclStmt:
		if n.Value != nil {
			Walk(n.Value, visitor)
		}

	case *MultiVarDeclStmt:
		Walk(n.Value, visitor)

	case *ReturnStmt:
		for _, val := range n.Values {
			Walk(val, visitor)
		}

	case *ArrayLiteral:
		for _, elem := range n.Elements {
			Walk(elem, visitor)
		}

	case *MapLiteral:
		for _, pair := range n.Pairs {
			Walk(pair.Key, visitor)
			Walk(pair.Value, visitor)
		}

	case *SuperArrayLiteral:
		for _, elem := range n.Elements {
			if elem.Key != nil {
				Walk(elem.Key, visitor)
			}
			Walk(elem.Value, visitor)
		}

	case *NewExpr:
		for _, arg := range n.Arguments {
			Walk(arg, visitor)
		}
		for _, na := range n.NamedArguments {
			Walk(na.Value, visitor)
		}

	case *ClosureExpr:
		Walk(n.Body, visitor)

	case *ArrowFuncExpr:
		Walk(n.Body, visitor)

	case *TryStmt:
		Walk(n.Try, visitor)
		for _, catch := range n.Catches {
			Walk(catch.Body, visitor)
		}
		if n.Finally != nil {
			Walk(n.Finally.Body, visitor)
		}

	case *SwitchStmt:
		Walk(n.Expr, visitor)
		for _, switchCase := range n.Cases {
			// 遍历多个值
			for _, value := range switchCase.Values {
				Walk(value, visitor)
			}
			// Body 可能是 Expression 或 []Statement
			if expr, ok := switchCase.Body.(Expression); ok {
				Walk(expr, visitor)
			} else if stmts, ok := switchCase.Body.([]Statement); ok {
				for _, stmt := range stmts {
					Walk(stmt, visitor)
				}
			}
		}
		if n.Default != nil {
			if expr, ok := n.Default.Body.(Expression); ok {
				Walk(expr, visitor)
			} else if stmts, ok := n.Default.Body.([]Statement); ok {
				for _, stmt := range stmts {
					Walk(stmt, visitor)
				}
			}
		}

	case *SwitchExpr:
		Walk(n.Expr, visitor)
		for _, switchCase := range n.Cases {
			// 遍历多个值
			for _, value := range switchCase.Values {
				Walk(value, visitor)
			}
			// SwitchExpr 的 body 应该总是 Expression
			if expr, ok := switchCase.Body.(Expression); ok {
				Walk(expr, visitor)
			}
		}
		if n.Default != nil {
			if expr, ok := n.Default.Body.(Expression); ok {
				Walk(expr, visitor)
			}
		}

	case *ExprStmt:
		Walk(n.Expr, visitor)

	case *EchoStmt:
		Walk(n.Value, visitor)

	case *GoStmt:
		Walk(n.Call, visitor)

	case *SelectStmt:
		for _, selectCase := range n.Cases {
			if selectCase.Var != nil {
				Walk(selectCase.Var, visitor)
			}
			Walk(selectCase.Comm, visitor)
			for _, stmt := range selectCase.Body {
				Walk(stmt, visitor)
			}
		}
		if n.Default != nil {
			for _, stmt := range n.Default.Body {
				Walk(stmt, visitor)
			}
		}

	// 协程 OOP 节点
	case *AwaitExpr:
		Walk(n.Coroutine, visitor)
		if n.Timeout != nil {
			Walk(n.Timeout, visitor)
		}

	case *CoroutineSpawnExpr:
		Walk(n.Closure, visitor)

	case *CoroutineAllExpr:
		Walk(n.Tasks, visitor)

	case *CoroutineAnyExpr:
		Walk(n.Tasks, visitor)

	case *CoroutineRaceExpr:
		Walk(n.Tasks, visitor)

	case *CoroutineDelayExpr:
		Walk(n.Milliseconds, visitor)

	case *ChannelSelectExpr:
		Walk(n.Cases, visitor)

	case *ThrowStmt:
		Walk(n.Exception, visitor)

	case *IsExpr:
		Walk(n.Expr, visitor)

	case *TypeCastExpr:
		Walk(n.Expr, visitor)

	case *MatchExpr:
		Walk(n.Expr, visitor)
		for _, matchCase := range n.Cases {
			Walk(matchCase.Pattern, visitor)
			if matchCase.Guard != nil {
				Walk(matchCase.Guard, visitor)
			}
			Walk(matchCase.Body, visitor)
		}

	case *InterpStringLiteral:
		for _, part := range n.Parts {
			Walk(part, visitor)
		}

	case *StaticAccess:
		Walk(n.Class, visitor)
		Walk(n.Member, visitor)

	case *ClassAccessExpr:
		Walk(n.Object, visitor)

	case *ClassDecl:
		for _, prop := range n.Properties {
			if prop.Value != nil {
				Walk(prop.Value, visitor)
			}
			if prop.ExprBody != nil {
				Walk(prop.ExprBody, visitor)
			}
		}
		for _, method := range n.Methods {
			if method.Body != nil {
				Walk(method.Body, visitor)
			}
		}

	case *MethodDecl:
		if n.Body != nil {
			Walk(n.Body, visitor)
		}

	case *TypePattern:
		// 类型模式没有需要遍历的子节点

	case *ValuePattern:
		Walk(n.Value, visitor)
	}
}

// ============================================================================
// 模式匹配表达式
// ============================================================================

// MatchExpr 模式匹配表达式
type MatchExpr struct {
	MatchToken token.Token
	LParen     token.Token
	Expr       Expression // 被匹配的表达式
	RParen     token.Token
	LBrace     token.Token
	Cases      []*MatchCase
	RBrace     token.Token
}

func (e *MatchExpr) Pos() token.Position { return e.MatchToken.Pos }
func (e *MatchExpr) End() token.Position { return e.RBrace.Pos }
func (e *MatchExpr) String() string      { return "match (...) {...}" }
func (e *MatchExpr) exprNode()           {}

// MatchCase 匹配分支
type MatchCase struct {
	Pattern  Pattern     // 模式（类型模式、值模式或通配符）
	Guard    Expression  // 守卫条件（可选，nil 表示无守卫）
	IfToken  token.Token // if 关键字（可选）
	Arrow    token.Token // =>
	Body     Expression  // 表达式体（match 是表达式，返回一个值）
}

func (c *MatchCase) Pos() token.Position { return c.Pattern.Pos() }
func (c *MatchCase) End() token.Position { return c.Body.End() }
func (c *MatchCase) String() string {
	if c.Guard != nil {
		return c.Pattern.String() + " if " + c.Guard.String() + " => " + c.Body.String()
	}
	return c.Pattern.String() + " => " + c.Body.String()
}

// Pattern 模式接口
type Pattern interface {
	Node
	patternNode()
}

// TypePattern 类型模式 (User $u, int $n)
type TypePattern struct {
	Type     TypeNode  // 类型
	Variable *Variable // 绑定的变量（可为 nil，用于类型检查但不绑定）
}

func (p *TypePattern) Pos() token.Position {
	if p.Variable != nil {
		return p.Type.Pos()
	}
	return p.Type.Pos()
}
func (p *TypePattern) End() token.Position {
	if p.Variable != nil {
		return p.Variable.End()
	}
	return p.Type.End()
}
func (p *TypePattern) String() string {
	if p.Variable != nil {
		return p.Type.String() + " " + p.Variable.String()
	}
	return p.Type.String()
}
func (p *TypePattern) patternNode() {}

// ValuePattern 值模式 (1, "hello", true, null)
type ValuePattern struct {
	Value Expression
}

func (p *ValuePattern) Pos() token.Position { return p.Value.Pos() }
func (p *ValuePattern) End() token.Position { return p.Value.End() }
func (p *ValuePattern) String() string      { return p.Value.String() }
func (p *ValuePattern) patternNode()        {}

// WildcardPattern 通配符模式 (_)
type WildcardPattern struct {
	Underscore token.Token
}

func (p *WildcardPattern) Pos() token.Position { return p.Underscore.Pos }
func (p *WildcardPattern) End() token.Position { return p.Underscore.Pos }
func (p *WildcardPattern) String() string      { return "_" }
func (p *WildcardPattern) patternNode()        {}
