package lexer

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/tangzhangming/nova/internal/i18n"
	"github.com/tangzhangming/nova/internal/token"
)

// Lexer 词法分析器
type Lexer struct {
	source   string   // 源代码
	filename string   // 文件名
	tokens   []token.Token // 已扫描的 tokens

	start   int // 当前 token 的起始位置
	current int // 当前扫描位置
	line    int // 当前行号
	column  int // 当前列号
	lineStart int // 当前行的起始偏移

	errors []Error // 词法错误
}

// Error 表示词法分析错误
type Error struct {
	Pos     token.Position
	Message string
}

func (e Error) Error() string {
	return fmt.Sprintf("%s: %s", e.Pos, e.Message)
}

// New 创建一个新的词法分析器
func New(source, filename string) *Lexer {
	return &Lexer{
		source:   source,
		filename: filename,
		line:     1,
		column:   1,
	}
}

// ScanTokens 扫描所有 tokens
func (l *Lexer) ScanTokens() []token.Token {
	for !l.isAtEnd() {
		l.start = l.current
		l.scanToken()
	}

	// 添加 EOF token
	l.tokens = append(l.tokens, token.Token{
		Type: token.EOF,
		Pos:  l.currentPos(),
	})

	return l.tokens
}

// Errors 返回所有词法错误
func (l *Lexer) Errors() []Error {
	return l.errors
}

// HasErrors 检查是否有错误
func (l *Lexer) HasErrors() bool {
	return len(l.errors) > 0
}

// scanToken 扫描单个 token
func (l *Lexer) scanToken() {
	ch := l.advance()

	switch ch {
	// 单字符 tokens
	case '(':
		l.addToken(token.LPAREN)
	case ')':
		l.addToken(token.RPAREN)
	case '{':
		l.addToken(token.LBRACE)
	case '}':
		l.addToken(token.RBRACE)
	case '[':
		l.addToken(token.LBRACKET)
	case ']':
		l.addToken(token.RBRACKET)
	case ',':
		l.addToken(token.COMMA)
	case ';':
		l.addToken(token.SEMICOLON)
	case '~':
		l.addToken(token.BIT_NOT)
	case '?':
		if l.match('.') {
			l.addToken(token.SAFE_DOT)
		} else if l.match('?') {
			l.addToken(token.NULL_COALESCE)
		} else {
			l.addToken(token.QUESTION)
		}
	case '@':
		l.addToken(token.AT)

	// 可能是多字符的 tokens
	case '+':
		if l.match('+') {
			l.addToken(token.INCREMENT)
		} else if l.match('=') {
			l.addToken(token.PLUS_ASSIGN)
		} else {
			l.addToken(token.PLUS)
		}
	case '-':
		if l.match('-') {
			l.addToken(token.DECREMENT)
		} else if l.match('=') {
			l.addToken(token.MINUS_ASSIGN)
		} else if l.match('>') {
			l.addToken(token.ARROW)
		} else {
			l.addToken(token.MINUS)
		}
	case '*':
		if l.match('=') {
			l.addToken(token.STAR_ASSIGN)
		} else {
			l.addToken(token.STAR)
		}
	case '%':
		if l.match('=') {
			l.addToken(token.PERCENT_ASSIGN)
		} else {
			l.addToken(token.PERCENT)
		}
	case '!':
		if l.match('=') {
			l.addToken(token.NE)
		} else {
			l.addToken(token.NOT)
		}
	case '=':
		if l.match('=') {
			l.addToken(token.EQ)
		} else if l.match('>') {
			l.addToken(token.DOUBLE_ARROW)
		} else {
			l.addToken(token.ASSIGN)
		}
	case '<':
		if l.match('=') {
			l.addToken(token.LE)
		} else if l.match('<') {
			l.addToken(token.LEFT_SHIFT)
		} else {
			l.addToken(token.LT)
		}
	case '>':
		if l.match('=') {
			l.addToken(token.GE)
		} else if l.match('>') {
			l.addToken(token.RIGHT_SHIFT)
		} else {
			l.addToken(token.GT)
		}
	case '&':
		if l.match('&') {
			l.addToken(token.AND)
		} else {
			l.addToken(token.BIT_AND)
		}
	case '|':
		if l.match('|') {
			l.addToken(token.OR)
		} else {
			l.addToken(token.BIT_OR)
		}
	case '^':
		l.addToken(token.BIT_XOR)
	case ':':
		if l.match('=') {
			l.addToken(token.DECLARE)
		} else if l.match(':') {
			l.addToken(token.DOUBLE_COLON)
		} else {
			l.addToken(token.COLON)
		}
	case '.':
		if l.match('.') {
			if l.match('.') {
				l.addToken(token.ELLIPSIS)
			} else {
				l.error(i18n.T(i18n.ErrUnexpectedDoubleDot))
			}
		} else {
			l.addToken(token.DOT)
		}

	// 斜杠（注释或除法）
	case '/':
		if l.match('/') {
			// 单行注释
			l.lineComment()
		} else if l.match('*') {
			// 多行注释
			l.blockComment()
		} else if l.match('=') {
			l.addToken(token.SLASH_ASSIGN)
		} else {
			l.addToken(token.SLASH)
		}

	// 字符串
	case '"':
		l.string('"')
	case '\'':
		l.string('\'')

	// 插值字符串
	case '#':
		if l.match('"') {
			l.interpString()
		} else {
			l.addToken(token.HASH)
		}

	// 变量 ($开头)
	case '$':
		l.variable()

	// 空白字符
	case ' ', '\r', '\t':
		// 忽略
	case '\n':
		l.newLine()

	default:
		if isDigit(ch) {
			l.number()
		} else if isAlpha(ch) {
			l.identifier()
		} else {
			l.error(i18n.T(i18n.ErrUnexpectedChar, ch))
		}
	}
}

// lineComment 处理单行注释
func (l *Lexer) lineComment() {
	for l.peek() != '\n' && !l.isAtEnd() {
		l.advance()
	}
	// 可选：保存注释 token
	// l.addToken(token.COMMENT)
}

// blockComment 处理多行注释
func (l *Lexer) blockComment() {
	depth := 1 // 支持嵌套注释

	for depth > 0 && !l.isAtEnd() {
		if l.peek() == '\n' {
			l.newLine()
			l.advance()
		} else if l.peek() == '/' && l.peekNext() == '*' {
			l.advance()
			l.advance()
			depth++
		} else if l.peek() == '*' && l.peekNext() == '/' {
			l.advance()
			l.advance()
			depth--
		} else {
			l.advance()
		}
	}

	if depth > 0 {
		l.error(i18n.T(i18n.ErrUnterminatedComment))
	}
}

// string 处理字符串
func (l *Lexer) string(quote rune) {
	var sb strings.Builder

	for !l.isAtEnd() {
		ch := l.peek()
		if ch == rune(quote) {
			break
		}
		if ch == '\n' {
			l.error(i18n.T(i18n.ErrUnterminatedString))
			return
		}
		if ch == '\\' {
			l.advance() // 跳过反斜杠
			if l.isAtEnd() {
				l.error(i18n.T(i18n.ErrUnterminatedString))
				return
			}
			escaped := l.advance()
			switch escaped {
			case 'n':
				sb.WriteByte('\n')
			case 'r':
				sb.WriteByte('\r')
			case 't':
				sb.WriteByte('\t')
			case '\\':
				sb.WriteByte('\\')
			case '\'':
				sb.WriteByte('\'')
			case '"':
				sb.WriteByte('"')
			case '0':
				sb.WriteByte(0)
			default:
				sb.WriteRune(escaped)
			}
		} else {
			sb.WriteRune(l.advance())
		}
	}

	if l.isAtEnd() {
		l.error(i18n.T(i18n.ErrUnterminatedString))
		return
	}

	l.advance() // 跳过结束引号

	l.addTokenWithValue(token.STRING, sb.String())
}

// interpString 处理插值字符串 #"..."
func (l *Lexer) interpString() {
	var sb strings.Builder

	for !l.isAtEnd() {
		ch := l.peek()
		if ch == '"' {
			break
		}
		if ch == '\n' {
			l.error(i18n.T(i18n.ErrUnterminatedInterp))
			return
		}
		if ch == '\\' {
			l.advance()
			if l.isAtEnd() {
				l.error(i18n.T(i18n.ErrUnterminatedInterp))
				return
			}
			escaped := l.advance()
			switch escaped {
			case 'n':
				sb.WriteByte('\n')
			case 'r':
				sb.WriteByte('\r')
			case 't':
				sb.WriteByte('\t')
			case '\\':
				sb.WriteByte('\\')
			case '"':
				sb.WriteByte('"')
			case '{':
				sb.WriteByte('{')
			case '}':
				sb.WriteByte('}')
			default:
				sb.WriteRune(escaped)
			}
		} else {
			sb.WriteRune(l.advance())
		}
	}

	if l.isAtEnd() {
		l.error(i18n.T(i18n.ErrUnterminatedInterp))
		return
	}

	l.advance() // 跳过结束引号

	l.addTokenWithValue(token.INTERP_STRING, sb.String())
}

// variable 处理变量 ($开头)
func (l *Lexer) variable() {
	// 检查是否是 $this
	if l.matchSequence("this") && !isAlphaNumeric(l.peek()) {
		l.addToken(token.THIS)
		return
	}

	// 普通变量
	for isAlphaNumeric(l.peek()) {
		l.advance()
	}

	literal := l.source[l.start:l.current]
	if literal == "$" {
		l.error(i18n.T(i18n.ErrExpectedVarName))
		return
	}

	l.addToken(token.VARIABLE)
}

// number 处理数字
func (l *Lexer) number() {
	// 检查是否是十六进制
	if l.source[l.start] == '0' && (l.peek() == 'x' || l.peek() == 'X') {
		l.advance() // 跳过 'x'
		for isHexDigit(l.peek()) {
			l.advance()
		}
		literal := l.source[l.start:l.current]
		value, err := strconv.ParseInt(literal, 0, 64)
		if err != nil {
			l.error(i18n.T(i18n.ErrInvalidHexNumber, literal))
			return
		}
		l.addTokenWithValue(token.INT, value)
		return
	}

	// 检查是否是二进制
	if l.source[l.start] == '0' && (l.peek() == 'b' || l.peek() == 'B') {
		l.advance() // 跳过 'b'
		for l.peek() == '0' || l.peek() == '1' {
			l.advance()
		}
		literal := l.source[l.start:l.current]
		value, err := strconv.ParseInt(literal, 0, 64)
		if err != nil {
			l.error(i18n.T(i18n.ErrInvalidBinaryNumber, literal))
			return
		}
		l.addTokenWithValue(token.INT, value)
		return
	}

	// 整数部分
	for isDigit(l.peek()) {
		l.advance()
	}

	// 检查小数部分
	isFloat := false
	if l.peek() == '.' && isDigit(l.peekNext()) {
		isFloat = true
		l.advance() // 跳过 '.'
		for isDigit(l.peek()) {
			l.advance()
		}
	}

	// 检查科学计数法
	if l.peek() == 'e' || l.peek() == 'E' {
		isFloat = true
		l.advance()
		if l.peek() == '+' || l.peek() == '-' {
			l.advance()
		}
		if !isDigit(l.peek()) {
			l.error(i18n.T(i18n.ErrInvalidExponent))
			return
		}
		for isDigit(l.peek()) {
			l.advance()
		}
	}

	literal := l.source[l.start:l.current]
	if isFloat {
		value, err := strconv.ParseFloat(literal, 64)
		if err != nil {
			l.error(i18n.T(i18n.ErrInvalidFloat, literal))
			return
		}
		l.addTokenWithValue(token.FLOAT, value)
	} else {
		value, err := strconv.ParseInt(literal, 10, 64)
		if err != nil {
			l.error(i18n.T(i18n.ErrInvalidInteger, literal))
			return
		}
		l.addTokenWithValue(token.INT, value)
	}
}

// identifier 处理标识符和关键字
func (l *Lexer) identifier() {
	for isAlphaNumeric(l.peek()) {
		l.advance()
	}

	text := l.source[l.start:l.current]
	tokenType := token.LookupIdent(text)

	// 特殊处理 as? (安全类型断言)
	if tokenType == token.AS && l.peek() == '?' {
		l.advance() // 消费 ?
		tokenType = token.AS_SAFE
	}

	l.addToken(tokenType)
}

// 辅助方法

func (l *Lexer) isAtEnd() bool {
	return l.current >= len(l.source)
}

func (l *Lexer) advance() rune {
	r, size := utf8.DecodeRuneInString(l.source[l.current:])
	l.current += size
	l.column++
	return r
}

func (l *Lexer) peek() rune {
	if l.isAtEnd() {
		return 0
	}
	r, _ := utf8.DecodeRuneInString(l.source[l.current:])
	return r
}

func (l *Lexer) peekNext() rune {
	if l.current+1 >= len(l.source) {
		return 0
	}
	_, size := utf8.DecodeRuneInString(l.source[l.current:])
	if l.current+size >= len(l.source) {
		return 0
	}
	r, _ := utf8.DecodeRuneInString(l.source[l.current+size:])
	return r
}

func (l *Lexer) match(expected rune) bool {
	if l.isAtEnd() {
		return false
	}
	r, size := utf8.DecodeRuneInString(l.source[l.current:])
	if r != expected {
		return false
	}
	l.current += size
	l.column++
	return true
}

func (l *Lexer) matchSequence(s string) bool {
	if l.current+len(s) > len(l.source) {
		return false
	}
	if l.source[l.current:l.current+len(s)] != s {
		return false
	}
	l.current += len(s)
	l.column += len(s)
	return true
}

func (l *Lexer) newLine() {
	l.line++
	l.column = 1
	l.lineStart = l.current + 1
}

func (l *Lexer) currentPos() token.Position {
	return token.Position{
		Filename: l.filename,
		Line:     l.line,
		Column:   l.column - (l.current - l.start),
		Offset:   l.start,
	}
}

func (l *Lexer) addToken(tokenType token.TokenType) {
	literal := l.source[l.start:l.current]
	l.tokens = append(l.tokens, token.Token{
		Type:    tokenType,
		Literal: literal,
		Pos:     l.currentPos(),
	})
}

func (l *Lexer) addTokenWithValue(tokenType token.TokenType, value interface{}) {
	literal := l.source[l.start:l.current]
	l.tokens = append(l.tokens, token.Token{
		Type:    tokenType,
		Literal: literal,
		Value:   value,
		Pos:     l.currentPos(),
	})
}

func (l *Lexer) error(message string) {
	l.errors = append(l.errors, Error{
		Pos:     l.currentPos(),
		Message: message,
	})
	l.addToken(token.ILLEGAL)
}

// 字符分类函数

func isDigit(ch rune) bool {
	return ch >= '0' && ch <= '9'
}

func isHexDigit(ch rune) bool {
	return isDigit(ch) || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')
}

func isAlpha(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_' || unicode.IsLetter(ch)
}

func isAlphaNumeric(ch rune) bool {
	return isAlpha(ch) || isDigit(ch)
}
