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

// ============================================================================
// Lexer - 词法分析器
// ============================================================================
//
// 词法分析器负责将源代码字符串转换为 Token 序列。
//
// 性能优化说明：
// 1. ASCII 快速路径：大多数源代码字符是 ASCII，避免不必要的 UTF-8 解码
// 2. Token 切片预分配：根据源码长度预估 token 数量，减少切片扩容
// 3. 空白字符批量跳过：一次性跳过连续空白，减少循环次数
// 4. 字符串快速路径：无转义字符时直接切片，避免逐字符拷贝
// 5. 小整数快速解析：单位数整数直接计算，避免 strconv 调用
// 6. Switch 分支优化：按字符出现频率排序，提高分支预测命中率
//
// ============================================================================

// Lexer 词法分析器结构体
type Lexer struct {
	source   string        // 源代码字符串
	filename string        // 源文件名（用于错误报告）
	tokens   []token.Token // 已扫描的 Token 列表

	start     int // 当前 Token 的起始位置（字节偏移）
	current   int // 当前扫描位置（字节偏移）
	line      int // 当前行号（从1开始）
	column    int // 当前列号（从1开始）
	lineStart int // 当前行的起始偏移（用于计算列号）

	errors []Error // 词法错误列表
}

// Error 表示词法分析错误
type Error struct {
	Pos     token.Position // 错误位置
	Message string         // 错误信息
}

func (e Error) Error() string {
	return fmt.Sprintf("%s: %s", e.Pos, e.Message)
}

// ============================================================================
// 构造函数
// ============================================================================

// New 创建一个新的词法分析器
//
// 参数:
//   - source: 源代码字符串
//   - filename: 源文件名（用于错误报告）
//
// 返回:
//   - *Lexer: 词法分析器实例
//
// 优化说明:
//   - 预分配 tokens 切片容量，经验值为 源码长度/5
//   - 这可以显著减少扫描过程中的切片扩容次数
func New(source, filename string) *Lexer {
	// 预估 token 数量：源码长度 / 5 是一个经验值
	// 实际代码中平均每 5 个字符产生一个 token（包括空白）
	estimatedTokens := len(source) / 5
	if estimatedTokens < 16 {
		estimatedTokens = 16 // 最小预分配 16 个
	}

	return &Lexer{
		source:   source,
		filename: filename,
		tokens:   make([]token.Token, 0, estimatedTokens),
		line:     1,
		column:   1,
	}
}

// ============================================================================
// 公共方法
// ============================================================================

// ScanTokens 扫描所有 tokens
//
// 这是词法分析的主入口，会扫描整个源代码并返回 Token 序列。
// 最后一个 Token 总是 EOF，表示文件结束。
//
// 返回:
//   - []token.Token: 扫描得到的 Token 序列
func (l *Lexer) ScanTokens() []token.Token {
	for !l.isAtEnd() {
		// 记录当前 token 的起始位置
		l.start = l.current
		l.scanToken()
	}

	// 添加 EOF token 标记文件结束
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

// ============================================================================
// 核心扫描逻辑
// ============================================================================

// scanToken 扫描单个 token
//
// 这是词法分析的核心函数，根据当前字符决定如何处理。
//
// 优化说明:
//   - switch 分支按字符出现频率排序
//   - 空白字符最常见，放在最前面
//   - 标识符和数字次之
//   - 运算符和分隔符再次
func (l *Lexer) scanToken() {
	ch := l.advance()

	// ==========================================================
	// 优化：按字符出现频率排序 switch 分支
	// 频率排序：空白 > 标识符 > 数字 > 常用符号 > 其他
	// ==========================================================

	switch ch {

	// ----------------------------------------------------------
	// 高频：空白字符（代码中最常见）
	// ----------------------------------------------------------
	case ' ', '\t', '\r':
		// 优化：批量跳过连续空白字符
		// 源代码中经常有连续的空格（如缩进），一次性跳过更高效
		l.skipWhitespace()

	case '\n':
		// 换行需要更新行号
		l.newLine()
		// 继续跳过后续空白（如下一行的缩进）
		l.skipWhitespace()

	// ----------------------------------------------------------
	// 高频：常用分隔符
	// ----------------------------------------------------------
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

	// ----------------------------------------------------------
	// 高频：常用运算符（可能是多字符）
	// ----------------------------------------------------------
	case '=':
		// = 或 == 或 =>
		if l.match('=') {
			l.addToken(token.EQ)
		} else if l.match('>') {
			l.addToken(token.DOUBLE_ARROW)
		} else {
			l.addToken(token.ASSIGN)
		}

	case ':':
		// : 或 := 或 ::
		if l.match('=') {
			l.addToken(token.DECLARE)
		} else if l.match(':') {
			l.addToken(token.DOUBLE_COLON)
		} else {
			l.addToken(token.COLON)
		}

	case '.':
		// . 或 ...
		if l.match('.') {
			if l.match('.') {
				l.addToken(token.ELLIPSIS)
			} else {
				l.error(i18n.T(i18n.ErrUnexpectedDoubleDot))
			}
		} else {
			l.addToken(token.DOT)
		}

	case '+':
		// + 或 ++ 或 +=
		if l.match('+') {
			l.addToken(token.INCREMENT)
		} else if l.match('=') {
			l.addToken(token.PLUS_ASSIGN)
		} else {
			l.addToken(token.PLUS)
		}

	case '-':
		// - 或 -- 或 -= 或 ->
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
		// * 或 *=
		if l.match('=') {
			l.addToken(token.STAR_ASSIGN)
		} else {
			l.addToken(token.STAR)
		}

	case '/':
		// / 或 /= 或 // 注释 或 /* 块注释
		if l.match('/') {
			l.lineComment()
		} else if l.match('*') {
			l.blockComment()
		} else if l.match('=') {
			l.addToken(token.SLASH_ASSIGN)
		} else {
			l.addToken(token.SLASH)
		}

	case '%':
		// % 或 %=
		if l.match('=') {
			l.addToken(token.PERCENT_ASSIGN)
		} else {
			l.addToken(token.PERCENT)
		}

	// ----------------------------------------------------------
	// 中频：比较和逻辑运算符
	// ----------------------------------------------------------
	case '!':
		// ! 或 !=
		if l.match('=') {
			l.addToken(token.NE)
		} else {
			l.addToken(token.NOT)
		}

	case '<':
		// < 或 <= 或 <<
		if l.match('=') {
			l.addToken(token.LE)
		} else if l.match('<') {
			l.addToken(token.LEFT_SHIFT)
		} else {
			l.addToken(token.LT)
		}

	case '>':
		// > 或 >= 或 >>
		if l.match('=') {
			l.addToken(token.GE)
		} else if l.match('>') {
			l.addToken(token.RIGHT_SHIFT)
		} else {
			l.addToken(token.GT)
		}

	case '&':
		// & 或 &&
		if l.match('&') {
			l.addToken(token.AND)
		} else {
			l.addToken(token.BIT_AND)
		}

	case '|':
		// | 或 ||
		if l.match('|') {
			l.addToken(token.OR)
		} else {
			l.addToken(token.BIT_OR)
		}

	case '?':
		// ? 或 ?. 或 ??
		if l.match('.') {
			l.addToken(token.SAFE_DOT)
		} else if l.match('?') {
			l.addToken(token.NULL_COALESCE)
		} else {
			l.addToken(token.QUESTION)
		}

	// ----------------------------------------------------------
	// 低频：单字符运算符
	// ----------------------------------------------------------
	case '^':
		l.addToken(token.BIT_XOR)
	case '~':
		l.addToken(token.BIT_NOT)
	case '@':
		l.addToken(token.AT)

	// ----------------------------------------------------------
	// 字符串字面量
	// ----------------------------------------------------------
	case '"':
		l.string('"')
	case '\'':
		l.string('\'')

	// ----------------------------------------------------------
	// 插值字符串 #"..."
	// ----------------------------------------------------------
	case '#':
		if l.match('"') {
			l.interpString()
		} else {
			l.addToken(token.HASH)
		}

	// ----------------------------------------------------------
	// 变量 ($开头)
	// ----------------------------------------------------------
	case '$':
		l.variable()

	// ----------------------------------------------------------
	// 默认：标识符、数字或非法字符
	// ----------------------------------------------------------
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

// ============================================================================
// 空白字符处理
// ============================================================================

// skipWhitespace 批量跳过连续的空白字符
//
// 优化说明:
//   - 源代码中经常有连续的空格（如缩进、对齐）
//   - 一次性跳过所有连续空白比逐个处理更高效
//   - 遇到换行时需要更新行号
func (l *Lexer) skipWhitespace() {
	for !l.isAtEnd() {
		ch := l.peekByte() // 使用字节级别的 peek，更快

		switch ch {
		case ' ', '\t', '\r':
			l.advanceByte() // ASCII 字符，使用字节级别的 advance
		case '\n':
			l.advanceByte()
			l.newLine()
		default:
			return // 遇到非空白字符，结束
		}
	}
}

// ============================================================================
// 注释处理
// ============================================================================

// lineComment 处理单行注释 //
//
// 单行注释从 // 开始，到行尾结束。
// 注释内容被丢弃，不生成 Token。
func (l *Lexer) lineComment() {
	// 一直读取直到行尾或文件结束
	for !l.isAtEnd() && l.peekByte() != '\n' {
		l.advance()
	}
	// 注意：不消费换行符，让主循环处理（更新行号）
}

// blockComment 处理多行注释 /* */
//
// 支持嵌套注释，如 /* outer /* inner */ outer */
// 这对于临时注释掉包含注释的代码很有用。
func (l *Lexer) blockComment() {
	depth := 1 // 嵌套深度，支持嵌套注释

	for depth > 0 && !l.isAtEnd() {
		// 检查嵌套的开始 /*
		if l.peekByte() == '/' && l.peekNextByte() == '*' {
			l.advance()
			l.advance()
			depth++
			continue
		}

		// 检查注释结束 */
		if l.peekByte() == '*' && l.peekNextByte() == '/' {
			l.advance()
			l.advance()
			depth--
			continue
		}

		// 处理换行
		if l.peekByte() == '\n' {
			l.advance()
			l.newLine()
			continue
		}

		// 普通字符，跳过
		l.advance()
	}

	// 检查是否正确闭合
	if depth > 0 {
		l.error(i18n.T(i18n.ErrUnterminatedComment))
	}
}

// ============================================================================
// 字符串处理
// ============================================================================

// string 处理普通字符串字面量
//
// 支持单引号 'xxx' 和双引号 "xxx" 两种形式。
// 支持转义字符：\n \r \t \\ \' \" \0
//
// 优化说明:
//   - 快速路径：如果字符串不包含转义字符，直接切片返回
//   - 慢速路径：包含转义字符时，使用 strings.Builder 构建
func (l *Lexer) string(quote rune) {
	startOffset := l.current // 记录字符串内容的起始位置（引号后）

	// ==========================================================
	// 优化：快速扫描检查是否包含转义字符
	// 大多数字符串不包含转义，可以直接切片返回
	// ==========================================================
	hasEscape := false
	scanPos := l.current

	for scanPos < len(l.source) {
		b := l.source[scanPos]
		if b == '\\' {
			hasEscape = true
			break
		}
		if b == byte(quote) || b == '\n' {
			break
		}
		scanPos++
	}

	// ==========================================================
	// 快速路径：无转义字符，直接切片
	// ==========================================================
	if !hasEscape {
		// 找到结束引号的位置
		endPos := scanPos

		// 移动 lexer 位置
		for l.current < endPos {
			l.advance()
		}

		// 检查是否正确结束
		if l.isAtEnd() || l.peek() == '\n' {
			l.error(i18n.T(i18n.ErrUnterminatedString))
			return
		}

		// 提取字符串内容（不包含引号）
		value := l.source[startOffset:l.current]
		l.advance() // 跳过结束引号

		l.addTokenWithValue(token.STRING, value)
		return
	}

	// ==========================================================
	// 慢速路径：包含转义字符，需要处理转义
	// ==========================================================
	var sb strings.Builder
	sb.Grow(scanPos - startOffset + 16) // 预分配容量

	for !l.isAtEnd() {
		ch := l.peek()

		// 检查字符串结束
		if ch == quote {
			break
		}

		// 字符串不能跨行（除非使用转义）
		if ch == '\n' {
			l.error(i18n.T(i18n.ErrUnterminatedString))
			return
		}

		// 处理转义字符
		if ch == '\\' {
			l.advance() // 跳过反斜杠
			if l.isAtEnd() {
				l.error(i18n.T(i18n.ErrUnterminatedString))
				return
			}

			escaped := l.advance()
			switch escaped {
			case 'n':
				sb.WriteByte('\n') // 换行
			case 'r':
				sb.WriteByte('\r') // 回车
			case 't':
				sb.WriteByte('\t') // 制表符
			case '\\':
				sb.WriteByte('\\') // 反斜杠
			case '\'':
				sb.WriteByte('\'') // 单引号
			case '"':
				sb.WriteByte('"') // 双引号
			case '0':
				sb.WriteByte(0) // 空字符
			default:
				// 未知转义，保留原字符
				sb.WriteRune(escaped)
			}
		} else {
			sb.WriteRune(l.advance())
		}
	}

	// 检查是否正确结束
	if l.isAtEnd() {
		l.error(i18n.T(i18n.ErrUnterminatedString))
		return
	}

	l.advance() // 跳过结束引号
	l.addTokenWithValue(token.STRING, sb.String())
}

// interpString 处理插值字符串 #"..."
//
// 插值字符串支持在字符串中嵌入表达式，如 #"Hello, {name}!"
// 额外支持 \{ 和 \} 转义。
func (l *Lexer) interpString() {
	var sb strings.Builder

	for !l.isAtEnd() {
		ch := l.peek()

		// 检查字符串结束
		if ch == '"' {
			break
		}

		// 插值字符串不能跨行
		if ch == '\n' {
			l.error(i18n.T(i18n.ErrUnterminatedInterp))
			return
		}

		// 处理转义字符
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
				sb.WriteByte('{') // 转义的大括号
			case '}':
				sb.WriteByte('}') // 转义的大括号
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

// ============================================================================
// 变量处理
// ============================================================================

// variable 处理变量（$开头）
//
// 变量以 $ 开头，如 $name, $this
// $this 是特殊关键字，单独处理。
func (l *Lexer) variable() {
	// 特殊处理 $this
	if l.matchSequence("this") && !isAlphaNumeric(l.peek()) {
		l.addToken(token.THIS)
		return
	}

	// 普通变量：继续读取字母数字
	for isAlphaNumeric(l.peek()) {
		l.advance()
	}

	literal := l.source[l.start:l.current]

	// 检查是否只有 $ 没有变量名
	if literal == "$" {
		l.error(i18n.T(i18n.ErrExpectedVarName))
		return
	}

	l.addToken(token.VARIABLE)
}

// ============================================================================
// 数字处理
// ============================================================================

// number 处理数字字面量
//
// 支持以下格式：
//   - 十进制整数：123
//   - 十六进制整数：0x1A2B
//   - 二进制整数：0b1010
//   - 浮点数：3.14
//   - 科学计数法：1.5e10, 2E-3
//
// 优化说明:
//   - 单位数整数直接计算，避免 strconv.ParseInt 调用
//   - 这种情况在循环计数等场景非常常见
func (l *Lexer) number() {
	firstDigit := l.source[l.start] // 第一个数字字符

	// ==========================================================
	// 十六进制数 0x...
	// ==========================================================
	if firstDigit == '0' && (l.peekByte() == 'x' || l.peekByte() == 'X') {
		l.advance() // 跳过 'x' 或 'X'

		// 读取十六进制数字
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

	// ==========================================================
	// 二进制数 0b...
	// ==========================================================
	if firstDigit == '0' && (l.peekByte() == 'b' || l.peekByte() == 'B') {
		l.advance() // 跳过 'b' 或 'B'

		// 读取二进制数字
		for l.peekByte() == '0' || l.peekByte() == '1' {
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

	// ==========================================================
	// 十进制整数部分
	// ==========================================================
	for isDigit(l.peek()) {
		l.advance()
	}

	// ==========================================================
	// 检查小数部分
	// ==========================================================
	isFloat := false
	if l.peekByte() == '.' && isDigit(l.peekNextRune()) {
		isFloat = true
		l.advance() // 跳过 '.'

		for isDigit(l.peek()) {
			l.advance()
		}
	}

	// ==========================================================
	// 检查科学计数法 e/E
	// ==========================================================
	if l.peekByte() == 'e' || l.peekByte() == 'E' {
		isFloat = true
		l.advance() // 跳过 'e' 或 'E'

		// 可选的正负号
		if l.peekByte() == '+' || l.peekByte() == '-' {
			l.advance()
		}

		// 指数部分必须有数字
		if !isDigit(l.peek()) {
			l.error(i18n.T(i18n.ErrInvalidExponent))
			return
		}

		for isDigit(l.peek()) {
			l.advance()
		}
	}

	// ==========================================================
	// 解析数值
	// ==========================================================
	literal := l.source[l.start:l.current]

	if isFloat {
		// 浮点数
		value, err := strconv.ParseFloat(literal, 64)
		if err != nil {
			l.error(i18n.T(i18n.ErrInvalidFloat, literal))
			return
		}
		l.addTokenWithValue(token.FLOAT, value)
	} else {
		// =======================================================
		// 优化：单位数整数快速路径
		// 循环中的 i++ 等操作非常常见，避免 strconv 调用
		// =======================================================
		if len(literal) == 1 {
			l.addTokenWithValue(token.INT, int64(literal[0]-'0'))
			return
		}

		// 两位数整数快速路径
		if len(literal) == 2 {
			value := int64(literal[0]-'0')*10 + int64(literal[1]-'0')
			l.addTokenWithValue(token.INT, value)
			return
		}

		// 一般情况：使用 strconv
		value, err := strconv.ParseInt(literal, 10, 64)
		if err != nil {
			l.error(i18n.T(i18n.ErrInvalidInteger, literal))
			return
		}
		l.addTokenWithValue(token.INT, value)
	}
}

// ============================================================================
// 标识符处理
// ============================================================================

// identifier 处理标识符和关键字
//
// 标识符以字母或下划线开头，后跟字母、数字或下划线。
// 扫描完成后查找关键字表，确定是标识符还是关键字。
func (l *Lexer) identifier() {
	// 读取标识符的剩余部分
	for isAlphaNumeric(l.peek()) {
		l.advance()
	}

	text := l.source[l.start:l.current]

	// 查找是否为关键字
	tokenType := token.LookupIdent(text)

	// 特殊处理 as? (安全类型断言)
	// as 后面紧跟 ? 时，合并为 as? 运算符
	if tokenType == token.AS && l.peekByte() == '?' {
		l.advance() // 消费 ?
		tokenType = token.AS_SAFE
	}

	l.addToken(tokenType)
}

// ============================================================================
// 底层字符操作（带 ASCII 优化）
// ============================================================================

// isAtEnd 检查是否到达源代码末尾
//
// 内联提示：这个函数调用非常频繁，Go 编译器会自动内联
func (l *Lexer) isAtEnd() bool {
	return l.current >= len(l.source)
}

// advance 前进一个字符并返回它
//
// 这是通用版本，支持完整的 UTF-8 字符。
// 对于 ASCII 字符，会自动使用快速路径。
func (l *Lexer) advance() rune {
	if l.current >= len(l.source) {
		return 0
	}

	b := l.source[l.current]

	// ==========================================================
	// 优化：ASCII 快速路径
	// 大多数源代码字符是 ASCII（< 128），无需 UTF-8 解码
	// ==========================================================
	if b < utf8.RuneSelf {
		l.current++
		l.column++
		return rune(b)
	}

	// 非 ASCII：完整 UTF-8 解码
	r, size := utf8.DecodeRuneInString(l.source[l.current:])
	l.current += size
	l.column++
	return r
}

// advanceByte 前进一个字节（仅用于已知是 ASCII 的情况）
//
// 这是内部优化函数，调用者必须确保当前字符是 ASCII。
// 用于空白字符跳过等性能敏感场景。
func (l *Lexer) advanceByte() {
	l.current++
	l.column++
}

// peek 查看当前字符但不前进
//
// 返回当前位置的字符，支持完整 UTF-8。
func (l *Lexer) peek() rune {
	if l.current >= len(l.source) {
		return 0
	}

	b := l.source[l.current]

	// ASCII 快速路径
	if b < utf8.RuneSelf {
		return rune(b)
	}

	// 非 ASCII：完整 UTF-8 解码
	r, _ := utf8.DecodeRuneInString(l.source[l.current:])
	return r
}

// peekByte 查看当前字节（仅用于 ASCII 检查）
//
// 这是内部优化函数，用于检查已知是 ASCII 的字符。
// 如果当前位置是多字节 UTF-8 字符的一部分，返回的值可能没有意义。
func (l *Lexer) peekByte() byte {
	if l.current >= len(l.source) {
		return 0
	}
	return l.source[l.current]
}

// peekNext 查看下一个字符但不前进
//
// 返回当前位置之后的字符，支持完整 UTF-8。
func (l *Lexer) peekNext() rune {
	if l.current >= len(l.source) {
		return 0
	}

	// 先计算当前字符的大小
	b := l.source[l.current]
	var size int
	if b < utf8.RuneSelf {
		size = 1
	} else {
		_, size = utf8.DecodeRuneInString(l.source[l.current:])
	}

	// 检查是否还有下一个字符
	if l.current+size >= len(l.source) {
		return 0
	}

	// 读取下一个字符
	nextB := l.source[l.current+size]
	if nextB < utf8.RuneSelf {
		return rune(nextB)
	}

	r, _ := utf8.DecodeRuneInString(l.source[l.current+size:])
	return r
}

// peekNextByte 查看下一个字节（仅用于 ASCII 检查）
//
// 这是内部优化函数，用于检查 /* */ 等双字符序列。
func (l *Lexer) peekNextByte() byte {
	if l.current+1 >= len(l.source) {
		return 0
	}
	return l.source[l.current+1]
}

// peekNextRune 查看下一个 rune（用于浮点数检测）
//
// 专门用于检查 "." 后面是否是数字的场景。
func (l *Lexer) peekNextRune() rune {
	if l.current+1 >= len(l.source) {
		return 0
	}

	b := l.source[l.current+1]
	if b < utf8.RuneSelf {
		return rune(b)
	}

	r, _ := utf8.DecodeRuneInString(l.source[l.current+1:])
	return r
}

// match 如果当前字符匹配则前进
//
// 用于识别多字符运算符，如 == != <= 等。
func (l *Lexer) match(expected rune) bool {
	if l.current >= len(l.source) {
		return false
	}

	b := l.source[l.current]

	// ASCII 快速路径
	if b < utf8.RuneSelf {
		if rune(b) != expected {
			return false
		}
		l.current++
		l.column++
		return true
	}

	// 非 ASCII
	r, size := utf8.DecodeRuneInString(l.source[l.current:])
	if r != expected {
		return false
	}
	l.current += size
	l.column++
	return true
}

// matchSequence 匹配一个字符串序列
//
// 用于识别 $this 等特殊标识符。
// 注意：这个函数假设序列是 ASCII 字符串。
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

// ============================================================================
// 位置追踪
// ============================================================================

// newLine 处理换行
//
// 更新行号和列号计数器。
func (l *Lexer) newLine() {
	l.line++
	l.column = 1
	l.lineStart = l.current
}

// currentPos 获取当前 token 的位置
//
// 返回当前正在扫描的 token 的起始位置信息。
func (l *Lexer) currentPos() token.Position {
	return token.Position{
		Filename: l.filename,
		Line:     l.line,
		Column:   l.column - (l.current - l.start),
		Offset:   l.start,
	}
}

// ============================================================================
// Token 生成
// ============================================================================

// addToken 添加一个无值的 Token
func (l *Lexer) addToken(tokenType token.TokenType) {
	literal := l.source[l.start:l.current]
	l.tokens = append(l.tokens, token.Token{
		Type:    tokenType,
		Literal: literal,
		Pos:     l.currentPos(),
	})
}

// addTokenWithValue 添加一个带值的 Token
//
// 用于数字和字符串字面量，Value 字段存储解析后的值。
func (l *Lexer) addTokenWithValue(tokenType token.TokenType, value interface{}) {
	literal := l.source[l.start:l.current]
	l.tokens = append(l.tokens, token.Token{
		Type:    tokenType,
		Literal: literal,
		Value:   value,
		Pos:     l.currentPos(),
	})
}

// ============================================================================
// 错误处理
// ============================================================================

// error 记录一个词法错误
//
// 错误会被收集起来，不会中断扫描过程。
// 同时会生成一个 ILLEGAL token。
func (l *Lexer) error(message string) {
	l.errors = append(l.errors, Error{
		Pos:     l.currentPos(),
		Message: message,
	})
	l.addToken(token.ILLEGAL)
}

// ============================================================================
// 字符分类函数
// ============================================================================

// isDigit 判断是否为数字 0-9
func isDigit(ch rune) bool {
	return ch >= '0' && ch <= '9'
}

// isHexDigit 判断是否为十六进制数字 0-9, a-f, A-F
func isHexDigit(ch rune) bool {
	return isDigit(ch) || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')
}

// isAlpha 判断是否为字母或下划线
//
// 支持 Unicode 字母，允许标识符使用非 ASCII 字符。
func isAlpha(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') ||
		(ch >= 'A' && ch <= 'Z') ||
		ch == '_' ||
		unicode.IsLetter(ch)
}

// isAlphaNumeric 判断是否为字母、数字或下划线
func isAlphaNumeric(ch rune) bool {
	return isAlpha(ch) || isDigit(ch)
}
