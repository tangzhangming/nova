package token

import "fmt"

// ============================================================================
// Token 类型定义
// ============================================================================
//
// TokenType 使用 iota 自动编号，按类别分组：
// 1. 特殊标记（ILLEGAL, EOF, COMMENT）
// 2. 字面量（标识符、变量、数字、字符串）
// 3. 运算符（算术、比较、逻辑、位运算）
// 4. 分隔符（括号、逗号、分号等）
// 5. 关键字（类型、值、声明、控制流等）
//
// ============================================================================

// TokenType 表示 Token 的类型
type TokenType int

const (
	// ----------------------------------------------------------
	// 特殊标记
	// ----------------------------------------------------------
	ILLEGAL TokenType = iota // 非法字符
	EOF                      // 文件结束
	COMMENT                  // 注释

	// ----------------------------------------------------------
	// 字面量
	// ----------------------------------------------------------
	IDENT         // 标识符 (变量名、函数名等)
	VARIABLE      // 变量 ($开头)
	INT           // 整数字面量
	FLOAT         // 浮点数字面量
	STRING        // 字符串字面量
	INTERP_STRING // 插值字符串 #"..."

	// ----------------------------------------------------------
	// 算术运算符
	// ----------------------------------------------------------
	PLUS           // +
	MINUS          // -
	STAR           // *
	SLASH          // /
	PERCENT        // %
	ASSIGN         // =
	DECLARE        // :=
	PLUS_ASSIGN    // +=
	MINUS_ASSIGN   // -=
	STAR_ASSIGN    // *=
	SLASH_ASSIGN   // /=
	PERCENT_ASSIGN // %=
	INCREMENT      // ++
	DECREMENT      // --

	// ----------------------------------------------------------
	// 比较运算符
	// ----------------------------------------------------------
	EQ // ==
	NE // !=
	LT // <
	LE // <=
	GT // >
	GE // >=

	// ----------------------------------------------------------
	// 逻辑运算符
	// ----------------------------------------------------------
	AND // &&
	OR  // ||
	NOT // !

	// ----------------------------------------------------------
	// 位运算符
	// ----------------------------------------------------------
	BIT_AND     // &
	BIT_OR      // |
	BIT_XOR     // ^
	BIT_NOT     // ~
	LEFT_SHIFT  // <<
	RIGHT_SHIFT // >>

	// ----------------------------------------------------------
	// 分隔符
	// ----------------------------------------------------------
	LPAREN        // (
	RPAREN        // )
	LBRACE        // {
	RBRACE        // }
	LBRACKET      // [
	RBRACKET      // ]
	COMMA         // ,
	DOT           // .
	SEMICOLON     // ;
	COLON         // :
	QUESTION      // ?
	ARROW         // ->
	DOUBLE_ARROW  // =>
	DOUBLE_COLON  // ::
	AT            // @
	HASH          // #
	ELLIPSIS      // ...
	SAFE_DOT      // ?.
	NULL_COALESCE // ??

	// ----------------------------------------------------------
	// 关键字 - 类型
	// ----------------------------------------------------------
	keyword_beg // 关键字起始标记（不是实际 token）
	INT_TYPE    // int
	I8_TYPE     // i8
	I16_TYPE    // i16
	I32_TYPE    // i32
	I64_TYPE    // i64
	UINT_TYPE   // uint
	U8_TYPE     // u8
	BYTE_TYPE   // byte (与 u8 等价)
	U16_TYPE    // u16
	U32_TYPE    // u32
	U64_TYPE    // u64
	FLOAT_TYPE  // float
	F32_TYPE    // f32
	F64_TYPE    // f64
	BOOL_TYPE   // bool
	STRING_TYPE // string
	VOID        // void
	UNKNOWN     // unknown
	DYNAMIC     // dynamic
	FUNC_TYPE   // func

	// ----------------------------------------------------------
	// 关键字 - 值
	// ----------------------------------------------------------
	TRUE  // true
	FALSE // false
	NULL  // null

	// ----------------------------------------------------------
	// 关键字 - 声明
	// ----------------------------------------------------------
	CLASS      // class
	INTERFACE  // interface
	ABSTRACT   // abstract
	EXTENDS    // extends
	IMPLEMENTS // implements
	FUNCTION   // function
	CONST      // const
	STATIC     // static
	FINAL      // final
	ENUM       // enum

	// ----------------------------------------------------------
	// 关键字 - 访问控制
	// ----------------------------------------------------------
	PUBLIC    // public
	PROTECTED // protected
	PRIVATE   // private

	// ----------------------------------------------------------
	// 关键字 - 控制流
	// ----------------------------------------------------------
	IF       // if
	ELSE     // else
	ELSEIF   // elseif
	SWITCH   // switch
	CASE     // case
	DEFAULT  // default
	MATCH    // match (模式匹配)
	FOR      // for
	FOREACH  // foreach
	WHILE    // while
	DO       // do
	BREAK    // break
	CONTINUE // continue
	RETURN   // return

	// ----------------------------------------------------------
	// 关键字 - 异常处理
	// ----------------------------------------------------------
	TRY     // try
	CATCH   // catch
	FINALLY // finally
	THROW   // throw

	// ----------------------------------------------------------
	// 关键字 - 其他
	// ----------------------------------------------------------
	NEW       // new
	THIS      // $this (特殊处理)
	SELF      // self
	PARENT    // parent
	AS        // as
	AS_SAFE   // as? (安全类型断言)
	IS        // is (类型检查)
	NAMESPACE // namespace
	USE       // use
	MAP       // map
	ECHO      // echo
	WHERE     // where (泛型约束)
	TYPE      // type (类型别名)
	GET       // get (属性访问器)
	SET       // set (属性访问器)
	VALUE     // value (setter参数)
	keyword_end // 关键字结束标记（不是实际 token）
)

// ============================================================================
// Token 类型名称映射
// ============================================================================

var tokenNames = map[TokenType]string{
	// 特殊标记
	ILLEGAL: "ILLEGAL",
	EOF:     "EOF",
	COMMENT: "COMMENT",

	// 字面量
	IDENT:         "IDENT",
	VARIABLE:      "VARIABLE",
	INT:           "INT",
	FLOAT:         "FLOAT",
	STRING:        "STRING",
	INTERP_STRING: "INTERP_STRING",

	// 算术运算符
	PLUS:           "+",
	MINUS:          "-",
	STAR:           "*",
	SLASH:          "/",
	PERCENT:        "%",
	ASSIGN:         "=",
	DECLARE:        ":=",
	PLUS_ASSIGN:    "+=",
	MINUS_ASSIGN:   "-=",
	STAR_ASSIGN:    "*=",
	SLASH_ASSIGN:   "/=",
	PERCENT_ASSIGN: "%=",
	INCREMENT:      "++",
	DECREMENT:      "--",

	// 比较运算符
	EQ: "==",
	NE: "!=",
	LT: "<",
	LE: "<=",
	GT: ">",
	GE: ">=",

	// 逻辑运算符
	AND: "&&",
	OR:  "||",
	NOT: "!",

	// 位运算符
	BIT_AND:     "&",
	BIT_OR:      "|",
	BIT_XOR:     "^",
	BIT_NOT:     "~",
	LEFT_SHIFT:  "<<",
	RIGHT_SHIFT: ">>",

	// 分隔符
	LPAREN:        "(",
	RPAREN:        ")",
	LBRACE:        "{",
	RBRACE:        "}",
	LBRACKET:      "[",
	RBRACKET:      "]",
	COMMA:         ",",
	DOT:           ".",
	SEMICOLON:     ";",
	COLON:         ":",
	QUESTION:      "?",
	ARROW:         "->",
	DOUBLE_ARROW:  "=>",
	DOUBLE_COLON:  "::",
	AT:            "@",
	HASH:          "#",
	ELLIPSIS:      "...",
	SAFE_DOT:      "?.",
	NULL_COALESCE: "??",

	// 类型关键字
	INT_TYPE:    "int",
	I8_TYPE:     "i8",
	I16_TYPE:    "i16",
	I32_TYPE:    "i32",
	I64_TYPE:    "i64",
	UINT_TYPE:   "uint",
	U8_TYPE:     "u8",
	BYTE_TYPE:   "byte",
	U16_TYPE:    "u16",
	U32_TYPE:    "u32",
	U64_TYPE:    "u64",
	FLOAT_TYPE:  "float",
	F32_TYPE:    "f32",
	F64_TYPE:    "f64",
	BOOL_TYPE:   "bool",
	STRING_TYPE: "string",
	VOID:        "void",
	UNKNOWN:     "unknown",
	DYNAMIC:     "dynamic",
	FUNC_TYPE:   "func",

	// 值关键字
	TRUE:  "true",
	FALSE: "false",
	NULL:  "null",

	// 声明关键字
	CLASS:      "class",
	INTERFACE:  "interface",
	ABSTRACT:   "abstract",
	EXTENDS:    "extends",
	IMPLEMENTS: "implements",
	FUNCTION:   "function",
	CONST:      "const",
	STATIC:     "static",
	FINAL:      "final",
	ENUM:       "enum",

	// 访问控制关键字
	PUBLIC:    "public",
	PROTECTED: "protected",
	PRIVATE:   "private",

	// 控制流关键字
	IF:       "if",
	ELSE:     "else",
	ELSEIF:   "elseif",
	SWITCH:   "switch",
	CASE:     "case",
	DEFAULT:  "default",
	MATCH:    "match",
	FOR:      "for",
	FOREACH:  "foreach",
	WHILE:    "while",
	DO:       "do",
	BREAK:    "break",
	CONTINUE: "continue",
	RETURN:   "return",

	// 异常处理关键字
	TRY:     "try",
	CATCH:   "catch",
	FINALLY: "finally",
	THROW:   "throw",

	// 其他关键字
	NEW:       "new",
	THIS:      "$this",
	SELF:      "self",
	PARENT:    "parent",
	AS:        "as",
	AS_SAFE:   "as?",
	IS:        "is",
	NAMESPACE: "namespace",
	USE:       "use",
	MAP:       "map",
	ECHO:      "echo",
	WHERE:     "where",
	TYPE:      "type",
	GET:       "get",
	SET:       "set",
	VALUE:     "value",
}

// ============================================================================
// 关键字查找表
// ============================================================================
//
// keywords 将关键字字符串映射到对应的 TokenType。
// 用于在词法分析时区分标识符和关键字。
//
// ============================================================================

var keywords = map[string]TokenType{
	// 类型关键字
	"int":    INT_TYPE,
	"i8":     I8_TYPE,
	"i16":    I16_TYPE,
	"i32":    I32_TYPE,
	"i64":    I64_TYPE,
	"uint":   UINT_TYPE,
	"u8":     U8_TYPE,
	"byte":   BYTE_TYPE,
	"u16":    U16_TYPE,
	"u32":    U32_TYPE,
	"u64":    U64_TYPE,
	"float":  FLOAT_TYPE,
	"f32":    F32_TYPE,
	"f64":    F64_TYPE,
	"bool":   BOOL_TYPE,
	"string": STRING_TYPE,
	"void":   VOID,
	"unknown": UNKNOWN,
	"dynamic": DYNAMIC,
	"func":   FUNC_TYPE,

	// 值关键字
	"true":  TRUE,
	"false": FALSE,
	"null":  NULL,

	// 声明关键字
	"class":      CLASS,
	"interface":  INTERFACE,
	"abstract":   ABSTRACT,
	"extends":    EXTENDS,
	"implements": IMPLEMENTS,
	"function":   FUNCTION,
	"const":      CONST,
	"static":     STATIC,
	"final":      FINAL,
	"enum":       ENUM,

	// 访问控制关键字
	"public":    PUBLIC,
	"protected": PROTECTED,
	"private":   PRIVATE,

	// 控制流关键字
	"if":       IF,
	"else":     ELSE,
	"elseif":   ELSEIF,
	"switch":   SWITCH,
	"case":     CASE,
	"default":  DEFAULT,
	"match":    MATCH,
	"for":      FOR,
	"foreach":  FOREACH,
	"while":    WHILE,
	"do":       DO,
	"break":    BREAK,
	"continue": CONTINUE,
	"return":   RETURN,

	// 异常处理关键字
	"try":     TRY,
	"catch":   CATCH,
	"finally": FINALLY,
	"throw":   THROW,

	// 其他关键字
	"new":       NEW,
	"self":      SELF,
	"parent":    PARENT,
	"as":        AS,
	"is":        IS,
	"namespace": NAMESPACE,
	"use":       USE,
	"map":       MAP,
	"echo":      ECHO,
	"where":     WHERE,
	"type":      TYPE,
	"get":       GET,
	"set":       SET,
	"value":     VALUE,
}

// ============================================================================
// 关键字查找函数
// ============================================================================

// LookupIdent 查找标识符是否为关键字
//
// 优化说明:
//   - 对于短关键字（2-3字符），使用 switch 语句直接匹配
//   - 短字符串的 switch 比 map 查找更快，因为避免了哈希计算
//   - 较长的关键字仍使用 map 查找
//
// 参数:
//   - ident: 标识符字符串
//
// 返回:
//   - TokenType: 如果是关键字返回对应类型，否则返回 IDENT
func LookupIdent(ident string) TokenType {
	// ==========================================================
	// 优化：短关键字使用 switch 快速匹配
	// 2-3 字符的关键字非常常见（if, for, int, etc.）
	// switch 语句比 map 查找更快，因为：
	// 1. 避免了字符串哈希计算
	// 2. 编译器可以生成跳转表或二分查找
	// ==========================================================

	switch len(ident) {
	case 2:
		// 两字符关键字：if, do, as, is, i8, u8
		switch ident {
		case "if":
			return IF
		case "do":
			return DO
		case "as":
			return AS
		case "is":
			return IS
		case "i8":
			return I8_TYPE
		case "u8":
			return U8_TYPE
		}

	case 3:
		// 三字符关键字：for, int, new, try, use, get, set, i16, i32, i64, u16, u32, u64, f32, f64, map
		switch ident {
		case "for":
			return FOR
		case "int":
			return INT_TYPE
		case "new":
			return NEW
		case "try":
			return TRY
		case "use":
			return USE
		case "get":
			return GET
		case "set":
			return SET
		case "i16":
			return I16_TYPE
		case "i32":
			return I32_TYPE
		case "i64":
			return I64_TYPE
		case "u16":
			return U16_TYPE
		case "u32":
			return U32_TYPE
		case "u64":
			return U64_TYPE
		case "f32":
			return F32_TYPE
		case "f64":
			return F64_TYPE
		case "map":
			return MAP
		}

	case 4:
		// 四字符关键字：else, case, void, null, true, bool, func, self, enum, byte, echo, type
		switch ident {
		case "else":
			return ELSE
		case "case":
			return CASE
		case "void":
			return VOID
		case "null":
			return NULL
		case "true":
			return TRUE
		case "bool":
			return BOOL_TYPE
		case "func":
			return FUNC_TYPE
		case "self":
			return SELF
		case "enum":
			return ENUM
		case "byte":
			return BYTE_TYPE
		case "echo":
			return ECHO
		case "type":
			return TYPE
		case "uint":
			return UINT_TYPE
		}

	case 5:
		// 五字符关键字：while, break, catch, throw, class, const, final, float, false, match, where, value
		switch ident {
		case "while":
			return WHILE
		case "break":
			return BREAK
		case "catch":
			return CATCH
		case "throw":
			return THROW
		case "class":
			return CLASS
		case "const":
			return CONST
		case "final":
			return FINAL
		case "float":
			return FLOAT_TYPE
		case "false":
			return FALSE
		case "match":
			return MATCH
		case "where":
			return WHERE
		case "value":
			return VALUE
		}

	case 6:
		// 六字符关键字：return, switch, static, public, parent, object, elseif, string
		switch ident {
		case "return":
			return RETURN
		case "switch":
			return SWITCH
		case "static":
			return STATIC
		case "public":
			return PUBLIC
		case "parent":
			return PARENT
		case "unknown":
			return UNKNOWN
		case "dynamic":
			return DYNAMIC
		case "elseif":
			return ELSEIF
		case "string":
			return STRING_TYPE
		}
	}

	// ==========================================================
	// 较长的关键字使用 map 查找
	// 这些关键字不常见，map 查找的开销可以接受
	// ==========================================================
	if tok, ok := keywords[ident]; ok {
		return tok
	}

	return IDENT
}

// IsKeyword 判断 TokenType 是否为关键字
func IsKeyword(t TokenType) bool {
	return t > keyword_beg && t < keyword_end
}

// String 返回 TokenType 的字符串表示
func (t TokenType) String() string {
	if name, ok := tokenNames[t]; ok {
		return name
	}
	return fmt.Sprintf("TokenType(%d)", t)
}

// ============================================================================
// Position - 源代码位置
// ============================================================================

// Position 表示源代码中的位置
type Position struct {
	Filename string // 文件名
	Line     int    // 行号 (从1开始)
	Column   int    // 列号 (从1开始)
	Offset   int    // 字节偏移量 (从0开始)
}

// String 返回位置的字符串表示，格式为 "filename:line:column"
func (p Position) String() string {
	if p.Filename != "" {
		return fmt.Sprintf("%s:%d:%d", p.Filename, p.Line, p.Column)
	}
	return fmt.Sprintf("%d:%d", p.Line, p.Column)
}

// IsValid 检查位置是否有效
func (p Position) IsValid() bool {
	return p.Line > 0
}

// ============================================================================
// Span - 源代码范围
// ============================================================================

// Span 表示源代码中的一个范围（开始到结束）
//
// 用于错误报告和代码高亮，可以精确定位问题代码的起止位置。
type Span struct {
	Start Position // 开始位置
	End   Position // 结束位置
}

// NewSpan 创建新的 Span
func NewSpan(start, end Position) Span {
	return Span{Start: start, End: end}
}

// SpanFromToken 从 Token 创建 Span
//
// 计算 Token 的结束位置，创建覆盖整个 Token 的 Span。
func SpanFromToken(t Token) Span {
	endPos := t.Pos
	endPos.Column += len(t.Literal)
	endPos.Offset += len(t.Literal)
	return Span{Start: t.Pos, End: endPos}
}

// Length 返回 Span 的长度（仅在同一行有效）
func (s Span) Length() int {
	if s.Start.Line == s.End.Line {
		return s.End.Column - s.Start.Column
	}
	return 1 // 多行时返回 1
}

// String 返回 Span 的字符串表示
func (s Span) String() string {
	if s.Start.Line == s.End.Line {
		return fmt.Sprintf("%s:%d:%d-%d", s.Start.Filename, s.Start.Line, s.Start.Column, s.End.Column)
	}
	return fmt.Sprintf("%s:%d:%d-%d:%d", s.Start.Filename, s.Start.Line, s.Start.Column, s.End.Line, s.End.Column)
}

// ============================================================================
// Token - 词法单元
// ============================================================================

// Token 表示一个词法单元
//
// Token 是词法分析的产物，包含：
// - Type: token 类型（如 IDENT, INT, IF 等）
// - Literal: 原始字面量文本
// - Value: 解析后的值（数字、字符串等）
// - Pos: 在源代码中的位置
type Token struct {
	Type    TokenType   // Token 类型
	Literal string      // 原始字面量
	Value   interface{} // 解析后的值 (用于数字、字符串等)
	Pos     Position    // 位置信息
}

// String 返回 Token 的字符串表示（用于调试）
func (t Token) String() string {
	switch t.Type {
	case IDENT, VARIABLE, INT, FLOAT, STRING, INTERP_STRING:
		return fmt.Sprintf("%s(%s) at %s", t.Type, t.Literal, t.Pos)
	default:
		return fmt.Sprintf("%s at %s", t.Type, t.Pos)
	}
}

// ============================================================================
// Token 构造函数
// ============================================================================

// New 创建一个新的 Token
func New(tokenType TokenType, literal string, pos Position) Token {
	return Token{
		Type:    tokenType,
		Literal: literal,
		Pos:     pos,
	}
}

// NewWithValue 创建一个带值的 Token
//
// 用于数字和字符串字面量，value 参数存储解析后的实际值。
func NewWithValue(tokenType TokenType, literal string, value interface{}, pos Position) Token {
	return Token{
		Type:    tokenType,
		Literal: literal,
		Value:   value,
		Pos:     pos,
	}
}
