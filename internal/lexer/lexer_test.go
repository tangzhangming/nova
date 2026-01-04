package lexer

import (
	"testing"

	"github.com/tangzhangming/nova/internal/token"
)

func TestLexerBasicTokens(t *testing.T) {
	input := `+ - * / % = := == != < <= > >= && || ! ( ) { } [ ] , . ; : ? -> => :: @ #`

	expected := []token.TokenType{
		token.PLUS, token.MINUS, token.STAR, token.SLASH, token.PERCENT,
		token.ASSIGN, token.DECLARE, token.EQ, token.NE,
		token.LT, token.LE, token.GT, token.GE,
		token.AND, token.OR, token.NOT,
		token.LPAREN, token.RPAREN, token.LBRACE, token.RBRACE,
		token.LBRACKET, token.RBRACKET,
		token.COMMA, token.DOT, token.SEMICOLON, token.COLON, token.QUESTION,
		token.ARROW, token.DOUBLE_ARROW, token.DOUBLE_COLON,
		token.AT, token.HASH,
		token.EOF,
	}

	l := New(input, "test.nova")
	tokens := l.ScanTokens()

	if len(tokens) != len(expected) {
		t.Fatalf("token count mismatch: got %d, want %d", len(tokens), len(expected))
	}

	for i, tok := range tokens {
		if tok.Type != expected[i] {
			t.Errorf("token[%d] type mismatch: got %s, want %s", i, tok.Type, expected[i])
		}
	}
}

func TestLexerKeywords(t *testing.T) {
	input := `class interface abstract extends implements function const static
	public protected private if else elseif switch case default
	for foreach while do break continue return try catch finally throw
	new self parent as namespace use map int string bool float void null true false`

	l := New(input, "test.nova")
	tokens := l.ScanTokens()

	expectedKeywords := []token.TokenType{
		token.CLASS, token.INTERFACE, token.ABSTRACT, token.EXTENDS, token.IMPLEMENTS,
		token.FUNCTION, token.CONST, token.STATIC,
		token.PUBLIC, token.PROTECTED, token.PRIVATE,
		token.IF, token.ELSE, token.ELSEIF, token.SWITCH, token.CASE, token.DEFAULT,
		token.FOR, token.FOREACH, token.WHILE, token.DO, token.BREAK, token.CONTINUE, token.RETURN,
		token.TRY, token.CATCH, token.FINALLY, token.THROW,
		token.NEW, token.SELF, token.PARENT, token.AS, token.NAMESPACE, token.USE, token.MAP,
		token.INT_TYPE, token.STRING_TYPE, token.BOOL_TYPE, token.FLOAT_TYPE, token.VOID,
		token.NULL, token.TRUE, token.FALSE,
		token.EOF,
	}

	if len(tokens) != len(expectedKeywords) {
		t.Fatalf("token count mismatch: got %d, want %d", len(tokens), len(expectedKeywords))
	}

	for i, tok := range tokens {
		if tok.Type != expectedKeywords[i] {
			t.Errorf("token[%d] type mismatch: got %s, want %s (literal: %s)", 
				i, tok.Type, expectedKeywords[i], tok.Literal)
		}
	}
}

func TestLexerVariables(t *testing.T) {
	input := `$name $count $this $user123`

	l := New(input, "test.nova")
	tokens := l.ScanTokens()

	expected := []struct {
		typ     token.TokenType
		literal string
	}{
		{token.VARIABLE, "$name"},
		{token.VARIABLE, "$count"},
		{token.THIS, "$this"},
		{token.VARIABLE, "$user123"},
		{token.EOF, ""},
	}

	if len(tokens) != len(expected) {
		t.Fatalf("token count mismatch: got %d, want %d", len(tokens), len(expected))
	}

	for i, tok := range tokens {
		if tok.Type != expected[i].typ {
			t.Errorf("token[%d] type mismatch: got %s, want %s", i, tok.Type, expected[i].typ)
		}
		if expected[i].literal != "" && tok.Literal != expected[i].literal {
			t.Errorf("token[%d] literal mismatch: got %s, want %s", i, tok.Literal, expected[i].literal)
		}
	}
}

func TestLexerNumbers(t *testing.T) {
	tests := []struct {
		input   string
		tokType token.TokenType
		value   interface{}
	}{
		{"123", token.INT, int64(123)},
		{"0", token.INT, int64(0)},
		{"3.14", token.FLOAT, 3.14},
		{"1e10", token.FLOAT, 1e10},
		{"2.5e-3", token.FLOAT, 2.5e-3},
		{"0xFF", token.INT, int64(255)},
		{"0b1010", token.INT, int64(10)},
	}

	for _, tt := range tests {
		l := New(tt.input, "test.nova")
		tokens := l.ScanTokens()

		if len(tokens) != 2 { // number + EOF
			t.Errorf("input %q: expected 2 tokens, got %d", tt.input, len(tokens))
			continue
		}

		tok := tokens[0]
		if tok.Type != tt.tokType {
			t.Errorf("input %q: type mismatch: got %s, want %s", tt.input, tok.Type, tt.tokType)
		}

		switch v := tt.value.(type) {
		case int64:
			if tok.Value.(int64) != v {
				t.Errorf("input %q: value mismatch: got %v, want %v", tt.input, tok.Value, v)
			}
		case float64:
			if tok.Value.(float64) != v {
				t.Errorf("input %q: value mismatch: got %v, want %v", tt.input, tok.Value, v)
			}
		}
	}
}

func TestLexerStrings(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`"hello"`, "hello"},
		{`'world'`, "world"},
		{`"hello\nworld"`, "hello\nworld"},
		{`"tab\there"`, "tab\there"},
		{`"quote\"here"`, `quote"here`},
	}

	for _, tt := range tests {
		l := New(tt.input, "test.nova")
		tokens := l.ScanTokens()

		if len(tokens) != 2 {
			t.Errorf("input %q: expected 2 tokens, got %d", tt.input, len(tokens))
			continue
		}

		tok := tokens[0]
		if tok.Type != token.STRING {
			t.Errorf("input %q: type mismatch: got %s, want STRING", tt.input, tok.Type)
		}
		if tok.Value.(string) != tt.expected {
			t.Errorf("input %q: value mismatch: got %q, want %q", tt.input, tok.Value, tt.expected)
		}
	}
}

func TestLexerInterpString(t *testing.T) {
	input := `#"hello {$name}"`

	l := New(input, "test.nova")
	tokens := l.ScanTokens()

	if len(tokens) != 2 {
		t.Fatalf("expected 2 tokens, got %d", len(tokens))
	}

	tok := tokens[0]
	if tok.Type != token.INTERP_STRING {
		t.Errorf("type mismatch: got %s, want INTERP_STRING", tok.Type)
	}
	if tok.Value.(string) != "hello {$name}" {
		t.Errorf("value mismatch: got %q, want %q", tok.Value, "hello {$name}")
	}
}

func TestLexerComments(t *testing.T) {
	input := `
	// single line comment
	$a = 1;
	/* multi
	   line
	   comment */
	$b = 2;
	`

	l := New(input, "test.nova")
	tokens := l.ScanTokens()

	// 应该只有 $a = 1 ; $b = 2 ; EOF
	expectedTypes := []token.TokenType{
		token.VARIABLE, token.ASSIGN, token.INT, token.SEMICOLON,
		token.VARIABLE, token.ASSIGN, token.INT, token.SEMICOLON,
		token.EOF,
	}

	if len(tokens) != len(expectedTypes) {
		t.Fatalf("token count mismatch: got %d, want %d", len(tokens), len(expectedTypes))
	}

	for i, tok := range tokens {
		if tok.Type != expectedTypes[i] {
			t.Errorf("token[%d] type mismatch: got %s, want %s", i, tok.Type, expectedTypes[i])
		}
	}
}

func TestLexerClassDeclaration(t *testing.T) {
	input := `
	public class User extends BaseModel implements Serializable {
		public const int MAX_AGE = 100;
		private string $name;
		
		public function __construct() {
			$this->name = "test";
		}
	}
	`

	l := New(input, "test.nova")
	tokens := l.ScanTokens()

	if l.HasErrors() {
		for _, err := range l.Errors() {
			t.Errorf("lexer error: %v", err)
		}
	}

	// 验证第一批 tokens
	expectedStart := []token.TokenType{
		token.PUBLIC, token.CLASS, token.IDENT, token.EXTENDS, token.IDENT,
		token.IMPLEMENTS, token.IDENT, token.LBRACE,
	}

	for i, expected := range expectedStart {
		if i >= len(tokens) {
			t.Fatalf("not enough tokens")
		}
		if tokens[i].Type != expected {
			t.Errorf("token[%d] type mismatch: got %s, want %s", i, tokens[i].Type, expected)
		}
	}
}

func TestLexerEllipsis(t *testing.T) {
	input := `int ...$args`

	l := New(input, "test.nova")
	tokens := l.ScanTokens()

	expected := []token.TokenType{
		token.INT_TYPE, token.ELLIPSIS, token.VARIABLE, token.EOF,
	}

	if len(tokens) != len(expected) {
		t.Fatalf("token count mismatch: got %d, want %d", len(tokens), len(expected))
	}

	for i, tok := range tokens {
		if tok.Type != expected[i] {
			t.Errorf("token[%d] type mismatch: got %s, want %s", i, tok.Type, expected[i])
		}
	}
}
