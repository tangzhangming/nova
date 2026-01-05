package token

import "fmt"

// TokenType 表示 Token 的类型
type TokenType int

const (
	// 特殊标记
	ILLEGAL TokenType = iota
	EOF
	COMMENT

	// 字面量
	IDENT        // 标识符 (变量名、函数名等)
	VARIABLE     // 变量 ($开头)
	INT          // 整数字面量
	FLOAT        // 浮点数字面量
	STRING       // 字符串字面量
	INTERP_STRING // 插值字符串 #"..."

	// 运算符
	PLUS          // +
	MINUS         // -
	STAR          // *
	SLASH         // /
	PERCENT       // %
	ASSIGN        // =
	DECLARE       // :=
	PLUS_ASSIGN   // +=
	MINUS_ASSIGN  // -=
	STAR_ASSIGN   // *=
	SLASH_ASSIGN  // /=
	PERCENT_ASSIGN // %=
	INCREMENT     // ++
	DECREMENT     // --

	// 比较运算符
	EQ     // ==
	NE     // !=
	LT     // <
	LE     // <=
	GT     // >
	GE     // >=

	// 逻辑运算符
	AND // &&
	OR  // ||
	NOT // !

	// 位运算符
	BIT_AND     // &
	BIT_OR      // |
	BIT_XOR     // ^
	BIT_NOT     // ~
	LEFT_SHIFT  // <<
	RIGHT_SHIFT // >>

	// 分隔符
	LPAREN    // (
	RPAREN    // )
	LBRACE    // {
	RBRACE    // }
	LBRACKET  // [
	RBRACKET  // ]
	COMMA     // ,
	DOT       // .
	SEMICOLON // ;
	COLON     // :
	QUESTION  // ?
	ARROW     // ->
	DOUBLE_ARROW // =>
	DOUBLE_COLON // ::
	AT        // @
	HASH      // #
	ELLIPSIS  // ...

	// 关键字 - 类型
	keyword_beg
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
	OBJECT      // object
	FUNC_TYPE   // func

	// 关键字 - 值
	TRUE  // true
	FALSE // false
	NULL  // null

	// 关键字 - 声明
	CLASS     // class
	INTERFACE // interface
	ABSTRACT  // abstract
	EXTENDS   // extends
	IMPLEMENTS // implements
	FUNCTION  // function
	CONST     // const
	STATIC    // static
	ENUM      // enum

	// 关键字 - 访问控制
	PUBLIC    // public
	PROTECTED // protected
	PRIVATE   // private

	// 关键字 - 控制流
	IF       // if
	ELSE     // else
	ELSEIF   // elseif
	SWITCH   // switch
	CASE     // case
	DEFAULT  // default
	FOR      // for
	FOREACH  // foreach
	WHILE    // while
	DO       // do
	BREAK    // break
	CONTINUE // continue
	RETURN   // return

	// 关键字 - 异常
	TRY     // try
	CATCH   // catch
	FINALLY // finally
	THROW   // throw

	// 关键字 - 其他
	NEW       // new
	THIS      // $this (特殊处理)
	SELF      // self
	PARENT    // parent
	AS        // as
	AS_SAFE   // as? (安全类型断言)
	NAMESPACE // namespace
	USE       // use
	MAP       // map
	ECHO      // echo
	keyword_end
)

var tokenNames = map[TokenType]string{
	ILLEGAL:        "ILLEGAL",
	EOF:            "EOF",
	COMMENT:        "COMMENT",
	IDENT:          "IDENT",
	VARIABLE:       "VARIABLE",
	INT:            "INT",
	FLOAT:          "FLOAT",
	STRING:         "STRING",
	INTERP_STRING:  "INTERP_STRING",
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
	EQ:             "==",
	NE:             "!=",
	LT:             "<",
	LE:             "<=",
	GT:             ">",
	GE:             ">=",
	AND:            "&&",
	OR:             "||",
	NOT:            "!",
	BIT_AND:        "&",
	BIT_OR:         "|",
	BIT_XOR:        "^",
	BIT_NOT:        "~",
	LEFT_SHIFT:     "<<",
	RIGHT_SHIFT:    ">>",
	LPAREN:         "(",
	RPAREN:         ")",
	LBRACE:         "{",
	RBRACE:         "}",
	LBRACKET:       "[",
	RBRACKET:       "]",
	COMMA:          ",",
	DOT:            ".",
	SEMICOLON:      ";",
	COLON:          ":",
	QUESTION:       "?",
	ARROW:          "->",
	DOUBLE_ARROW:   "=>",
	DOUBLE_COLON:   "::",
	AT:             "@",
	HASH:           "#",
	ELLIPSIS:       "...",
	INT_TYPE:       "int",
	I8_TYPE:        "i8",
	I16_TYPE:       "i16",
	I32_TYPE:       "i32",
	I64_TYPE:       "i64",
	UINT_TYPE:      "uint",
	U8_TYPE:        "u8",
	BYTE_TYPE:      "byte",
	U16_TYPE:       "u16",
	U32_TYPE:       "u32",
	U64_TYPE:       "u64",
	FLOAT_TYPE:     "float",
	F32_TYPE:       "f32",
	F64_TYPE:       "f64",
	BOOL_TYPE:      "bool",
	STRING_TYPE:    "string",
	VOID:           "void",
	OBJECT:         "object",
	FUNC_TYPE:      "func",
	TRUE:           "true",
	FALSE:          "false",
	NULL:           "null",
	CLASS:          "class",
	INTERFACE:      "interface",
	ABSTRACT:       "abstract",
	EXTENDS:        "extends",
	IMPLEMENTS:     "implements",
	FUNCTION:       "function",
	CONST:          "const",
	STATIC:         "static",
	ENUM:           "enum",
	PUBLIC:         "public",
	PROTECTED:      "protected",
	PRIVATE:        "private",
	IF:             "if",
	ELSE:           "else",
	ELSEIF:         "elseif",
	SWITCH:         "switch",
	CASE:           "case",
	DEFAULT:        "default",
	FOR:            "for",
	FOREACH:        "foreach",
	WHILE:          "while",
	DO:             "do",
	BREAK:          "break",
	CONTINUE:       "continue",
	RETURN:         "return",
	TRY:            "try",
	CATCH:          "catch",
	FINALLY:        "finally",
	THROW:          "throw",
	NEW:            "new",
	THIS:           "$this",
	SELF:           "self",
	PARENT:         "parent",
	AS:             "as",
	AS_SAFE:        "as?",
	NAMESPACE:      "namespace",
	USE:            "use",
	MAP:            "map",
	ECHO:           "echo",
}

var keywords = map[string]TokenType{
	"int":        INT_TYPE,
	"i8":         I8_TYPE,
	"i16":        I16_TYPE,
	"i32":        I32_TYPE,
	"i64":        I64_TYPE,
	"uint":       UINT_TYPE,
	"u8":         U8_TYPE,
	"byte":       BYTE_TYPE,
	"u16":        U16_TYPE,
	"u32":        U32_TYPE,
	"u64":        U64_TYPE,
	"float":      FLOAT_TYPE,
	"f32":        F32_TYPE,
	"f64":        F64_TYPE,
	"bool":       BOOL_TYPE,
	"string":     STRING_TYPE,
	"void":       VOID,
	"object":     OBJECT,
	"func":       FUNC_TYPE,
	"true":       TRUE,
	"false":      FALSE,
	"null":       NULL,
	"class":      CLASS,
	"interface":  INTERFACE,
	"abstract":   ABSTRACT,
	"extends":    EXTENDS,
	"implements": IMPLEMENTS,
	"function":   FUNCTION,
	"const":      CONST,
	"static":     STATIC,
	"enum":       ENUM,
	"public":     PUBLIC,
	"protected":  PROTECTED,
	"private":    PRIVATE,
	"if":         IF,
	"else":       ELSE,
	"elseif":     ELSEIF,
	"switch":     SWITCH,
	"case":       CASE,
	"default":    DEFAULT,
	"for":        FOR,
	"foreach":    FOREACH,
	"while":      WHILE,
	"do":         DO,
	"break":      BREAK,
	"continue":   CONTINUE,
	"return":     RETURN,
	"try":        TRY,
	"catch":      CATCH,
	"finally":    FINALLY,
	"throw":      THROW,
	"new":        NEW,
	"self":       SELF,
	"parent":     PARENT,
	"as":         AS,
	"namespace":  NAMESPACE,
	"use":        USE,
	"map":        MAP,
	"echo":       ECHO,
}

// LookupIdent 查找标识符是否为关键字
func LookupIdent(ident string) TokenType {
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

// Position 表示源代码中的位置
type Position struct {
	Filename string // 文件名
	Line     int    // 行号 (从1开始)
	Column   int    // 列号 (从1开始)
	Offset   int    // 字节偏移量 (从0开始)
}

func (p Position) String() string {
	if p.Filename != "" {
		return fmt.Sprintf("%s:%d:%d", p.Filename, p.Line, p.Column)
	}
	return fmt.Sprintf("%d:%d", p.Line, p.Column)
}

// Token 表示一个词法单元
type Token struct {
	Type    TokenType   // Token 类型
	Literal string      // 原始字面量
	Value   interface{} // 解析后的值 (用于数字、字符串等)
	Pos     Position    // 位置信息
}

func (t Token) String() string {
	switch t.Type {
	case IDENT, VARIABLE, INT, FLOAT, STRING, INTERP_STRING:
		return fmt.Sprintf("%s(%s) at %s", t.Type, t.Literal, t.Pos)
	default:
		return fmt.Sprintf("%s at %s", t.Type, t.Pos)
	}
}

// New 创建一个新的 Token
func New(tokenType TokenType, literal string, pos Position) Token {
	return Token{
		Type:    tokenType,
		Literal: literal,
		Pos:     pos,
	}
}

// NewWithValue 创建一个带值的 Token
func NewWithValue(tokenType TokenType, literal string, value interface{}, pos Position) Token {
	return Token{
		Type:    tokenType,
		Literal: literal,
		Value:   value,
		Pos:     pos,
	}
}
