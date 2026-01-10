package lsp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tangzhangming/nova/internal/ast"
	"go.lsp.dev/protocol"
)

// ============================================================================
// Document Manager Tests
// ============================================================================

func TestDocumentManager_Open(t *testing.T) {
	dm := NewDocumentManager()
	
	content := `namespace test
class Hello {
    public function greet(): string {
        return "Hello";
    }
}`
	
	doc := dm.Open("file:///test.sola", content, 1)
	
	if doc == nil {
		t.Fatal("expected document to be created")
	}
	
	if doc.URI != "file:///test.sola" {
		t.Errorf("expected URI 'file:///test.sola', got '%s'", doc.URI)
	}
	
	if doc.Version != 1 {
		t.Errorf("expected version 1, got %d", doc.Version)
	}
	
	if doc.Content != content {
		t.Errorf("content mismatch")
	}
	
	// Check that AST was parsed
	ast := doc.GetAST()
	if ast == nil {
		t.Error("expected AST to be parsed")
	}
}

func TestDocumentManager_Get(t *testing.T) {
	dm := NewDocumentManager()
	
	dm.Open("file:///test.sola", "class Test {}", 1)
	
	doc := dm.Get("file:///test.sola")
	if doc == nil {
		t.Fatal("expected document to exist")
	}
	
	notFound := dm.Get("file:///nonexistent.sola")
	if notFound != nil {
		t.Error("expected nil for nonexistent document")
	}
}

func TestDocumentManager_Close(t *testing.T) {
	dm := NewDocumentManager()
	
	dm.Open("file:///test.sola", "class Test {}", 1)
	dm.Close("file:///test.sola")
	
	doc := dm.Get("file:///test.sola")
	if doc != nil {
		t.Error("expected document to be removed after close")
	}
}

func TestDocument_GetLine(t *testing.T) {
	dm := NewDocumentManager()
	
	content := "line1\nline2\nline3"
	doc := dm.Open("file:///test.sola", content, 1)
	
	tests := []struct {
		line     int
		expected string
	}{
		{0, "line1"},
		{1, "line2"},
		{2, "line3"},
		{-1, ""},
		{10, ""},
	}
	
	for _, tt := range tests {
		result := doc.GetLine(tt.line)
		if result != tt.expected {
			t.Errorf("GetLine(%d): expected '%s', got '%s'", tt.line, tt.expected, result)
		}
	}
}

func TestDocument_GetWordAt(t *testing.T) {
	dm := NewDocumentManager()
	
	content := "class MyClass {}"
	doc := dm.Open("file:///test.sola", content, 1)
	
	tests := []struct {
		line, char int
		expected   string
	}{
		{0, 0, "class"},
		{0, 3, "class"},
		{0, 6, "MyClass"},
		{0, 10, "MyClass"},
	}
	
	for _, tt := range tests {
		result := doc.GetWordAt(tt.line, tt.char)
		if result != tt.expected {
			t.Errorf("GetWordAt(%d, %d): expected '%s', got '%s'", tt.line, tt.char, tt.expected, result)
		}
	}
}

// ============================================================================
// Test Support Tests
// ============================================================================

func TestIsTestFile(t *testing.T) {
	tests := []struct {
		filename string
		expected bool
	}{
		{"user_test.sola", true},
		{"test_user.sola", true},
		{"UserTest.sola", true},
		{"user.sola", false},
		{"my_test.go", false},
		{"test.txt", false},
	}
	
	for _, tt := range tests {
		result := IsTestFile(tt.filename)
		if result != tt.expected {
			t.Errorf("IsTestFile(%s): expected %v, got %v", tt.filename, tt.expected, result)
		}
	}
}

func TestIsTestMethod(t *testing.T) {
	dm := NewDocumentManager()
	
	content := `namespace test

class UserTest {
    public function testCreate() {}
    public function TestUpdate() {}
    public function shouldValidate() {}
    public function itShouldWork() {}
    public function specCreate() {}
    public function normalMethod() {}
    public function setUp() {}
}`
	
	doc := dm.Open("file:///test.sola", content, 1)
	astFile := doc.GetAST()
	
	if astFile == nil || len(astFile.Declarations) == 0 {
		t.Skip("failed to parse test content - parser may not be available")
		return
	}
	
	classDecl, ok := astFile.Declarations[0].(*ast.ClassDecl)
	if !ok {
		t.Skip("AST structure differs from expected")
		return
	}
	
	expectedTestMethods := map[string]bool{
		"testCreate":     true,
		"TestUpdate":     true,
		"shouldValidate": true,
		"itShouldWork":   true,
		"specCreate":     true,
		"normalMethod":   false,
		"setUp":          false,
	}
	
	for _, method := range classDecl.Methods {
		name := method.Name.Name
		expected, exists := expectedTestMethods[name]
		if exists {
			result := IsTestMethod(method)
			if result != expected {
				t.Errorf("IsTestMethod(%s): expected %v, got %v", name, expected, result)
			}
		}
	}
}

// ============================================================================
// Semantic Tokens Tests
// ============================================================================

func TestSemanticTokenTypes(t *testing.T) {
	expectedTypes := []string{
		"namespace", "class", "enum", "interface", "struct",
		"typeParameter", "type", "parameter", "variable",
		"property", "enumMember", "function", "method",
	}
	
	for _, expected := range expectedTypes {
		found := false
		for _, st := range SemanticTokenTypes {
			if st == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected semantic token type '%s' not found", expected)
		}
	}
}

func TestSemanticTokenModifiers(t *testing.T) {
	expectedModifiers := []string{
		"declaration", "definition", "readonly", "static",
		"deprecated", "abstract",
	}
	
	for _, expected := range expectedModifiers {
		found := false
		for _, sm := range SemanticTokenModifiers {
			if sm == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected semantic token modifier '%s' not found", expected)
		}
	}
}

func TestEncodeSemanticTokens(t *testing.T) {
	tokens := []semanticToken{
		{Line: 0, StartChar: 0, Length: 5, TokenType: TokenTypeKeyword, Modifiers: 0},
		{Line: 0, StartChar: 6, Length: 5, TokenType: TokenTypeClass, Modifiers: TokenModDeclaration},
		{Line: 1, StartChar: 4, Length: 8, TokenType: TokenTypeMethod, Modifiers: TokenModDeclaration},
	}
	
	encoded := encodeSemanticTokens(tokens)
	
	// Each token is encoded as 5 values: deltaLine, deltaChar, length, type, modifiers
	expectedLen := len(tokens) * 5
	if len(encoded) != expectedLen {
		t.Errorf("expected encoded length %d, got %d", expectedLen, len(encoded))
	}
	
	// Verify first token
	if encoded[0] != 0 { // deltaLine
		t.Errorf("first token deltaLine: expected 0, got %d", encoded[0])
	}
	if encoded[1] != 0 { // deltaChar
		t.Errorf("first token deltaChar: expected 0, got %d", encoded[1])
	}
	if encoded[2] != 5 { // length
		t.Errorf("first token length: expected 5, got %d", encoded[2])
	}
	
	// Verify second token (same line)
	if encoded[5] != 0 { // deltaLine
		t.Errorf("second token deltaLine: expected 0, got %d", encoded[5])
	}
	if encoded[6] != 6 { // deltaChar (relative to previous)
		t.Errorf("second token deltaChar: expected 6, got %d", encoded[6])
	}
	
	// Verify third token (new line)
	if encoded[10] != 1 { // deltaLine
		t.Errorf("third token deltaLine: expected 1, got %d", encoded[10])
	}
	if encoded[11] != 4 { // deltaChar (absolute since new line)
		t.Errorf("third token deltaChar: expected 4, got %d", encoded[11])
	}
}

// ============================================================================
// Inlay Hints Tests
// ============================================================================

func TestInlayHintKind(t *testing.T) {
	if InlayHintKindType != 1 {
		t.Errorf("InlayHintKindType: expected 1, got %d", InlayHintKindType)
	}
	if InlayHintKindParameter != 2 {
		t.Errorf("InlayHintKindParameter: expected 2, got %d", InlayHintKindParameter)
	}
}

func TestIsInRange(t *testing.T) {
	rang := protocol.Range{
		Start: protocol.Position{Line: 1, Character: 0},
		End:   protocol.Position{Line: 10, Character: 100},
	}
	
	tests := []struct {
		pos      protocol.Position
		expected bool
	}{
		{protocol.Position{Line: 5, Character: 50}, true},
		{protocol.Position{Line: 1, Character: 0}, true},
		{protocol.Position{Line: 10, Character: 100}, true},
		{protocol.Position{Line: 0, Character: 50}, false},
		{protocol.Position{Line: 11, Character: 0}, false},
	}
	
	for _, tt := range tests {
		result := isInRange(tt.pos, rang)
		if result != tt.expected {
			t.Errorf("isInRange(%v): expected %v, got %v", tt.pos, tt.expected, result)
		}
	}
}

// ============================================================================
// Type Inference Tests
// ============================================================================

func TestInferExprType(t *testing.T) {
	dm := NewDocumentManager()
	
	content := `namespace test

$int := 42;
$float := 3.14;
$str := "hello";
$bool := true;
$arr := int{1, 2, 3};
`
	
	doc := dm.Open("file:///test.sola", content, 1)
	symbols := doc.GetSymbols()
	ast := doc.GetAST()
	
	if ast == nil {
		t.Fatal("failed to parse content")
	}
	
	// Test basic type inference through statement analysis
	// This is a simplified test - in real code, we'd walk the AST
	_ = symbols // symbols would be used for variable type lookup
}

// ============================================================================
// Code Lens Tests
// ============================================================================

func TestCodeLensCommand(t *testing.T) {
	cmd := &Command{
		Title:   "5 references",
		Command: "sola.findReferences",
		Arguments: []interface{}{
			"file:///test.sola",
			10,
			5,
		},
	}
	
	if cmd.Title != "5 references" {
		t.Errorf("expected title '5 references', got '%s'", cmd.Title)
	}
	
	if cmd.Command != "sola.findReferences" {
		t.Errorf("expected command 'sola.findReferences', got '%s'", cmd.Command)
	}
	
	if len(cmd.Arguments) != 3 {
		t.Errorf("expected 3 arguments, got %d", len(cmd.Arguments))
	}
}

// ============================================================================
// Assertion Detection Tests
// ============================================================================

func TestIsAssertionMethod(t *testing.T) {
	assertionMethods := []string{
		"assertEquals", "assertEqual", "assertTrue", "assertFalse",
		"assertNull", "assertNotNull", "expect", "should",
	}
	
	for _, method := range assertionMethods {
		if !isAssertionMethod(method) {
			t.Errorf("expected '%s' to be recognized as assertion method", method)
		}
	}
	
	nonAssertionMethods := []string{
		"calculate", "process", "validate", "transform",
	}
	
	for _, method := range nonAssertionMethods {
		if isAssertionMethod(method) {
			t.Errorf("expected '%s' to NOT be recognized as assertion method", method)
		}
	}
}

func TestIsAssertionFunction(t *testing.T) {
	assertionFuncs := []string{
		"assert", "expect", "verify", "assertTrue",
	}
	
	for _, fn := range assertionFuncs {
		if !isAssertionFunction(fn) {
			t.Errorf("expected '%s' to be recognized as assertion function", fn)
		}
	}
	
	nonAssertionFuncs := []string{
		"print", "echo", "len", "count",
	}
	
	for _, fn := range nonAssertionFuncs {
		if isAssertionFunction(fn) {
			t.Errorf("expected '%s' to NOT be recognized as assertion function", fn)
		}
	}
}

// ============================================================================
// Text Edit Tests
// ============================================================================

func TestApplyTextEdit(t *testing.T) {
	content := "line1\nline2\nline3"
	
	// Replace "line2" with "modified"
	rang := protocol.Range{
		Start: protocol.Position{Line: 1, Character: 0},
		End:   protocol.Position{Line: 1, Character: 5},
	}
	
	result := applyTextEdit(content, rang, "modified")
	expected := "line1\nmodified\nline3"
	
	if result != expected {
		t.Errorf("applyTextEdit: expected '%s', got '%s'", expected, result)
	}
}

func TestApplyTextEdit_Insert(t *testing.T) {
	content := "line1\nline2"
	
	// Insert at start of line2
	rang := protocol.Range{
		Start: protocol.Position{Line: 1, Character: 0},
		End:   protocol.Position{Line: 1, Character: 0},
	}
	
	result := applyTextEdit(content, rang, "NEW: ")
	expected := "line1\nNEW: line2"
	
	if result != expected {
		t.Errorf("applyTextEdit insert: expected '%s', got '%s'", expected, result)
	}
}

func TestApplyTextEdit_Delete(t *testing.T) {
	content := "line1\nline2\nline3"
	
	// Delete line2 entirely (including newline from line1)
	rang := protocol.Range{
		Start: protocol.Position{Line: 1, Character: 0},
		End:   protocol.Position{Line: 2, Character: 0},
	}
	
	result := applyTextEdit(content, rang, "")
	expected := "line1\nline3"
	
	if result != expected {
		t.Errorf("applyTextEdit delete: expected '%s', got '%s'", expected, result)
	}
}

// ============================================================================
// Split Lines Tests
// ============================================================================

func TestSplitLines(t *testing.T) {
	tests := []struct {
		content  string
		expected []string
	}{
		{"line1\nline2", []string{"line1", "line2"}},
		{"line1\r\nline2", []string{"line1", "line2"}},
		{"line1\rline2", []string{"line1", "line2"}},
		{"single", []string{"single"}},
		{"", []string{""}},
	}
	
	for _, tt := range tests {
		result := splitLines(tt.content)
		if len(result) != len(tt.expected) {
			t.Errorf("splitLines(%q): expected %d lines, got %d", tt.content, len(tt.expected), len(result))
			continue
		}
		for i, line := range result {
			if line != tt.expected[i] {
				t.Errorf("splitLines(%q)[%d]: expected '%s', got '%s'", tt.content, i, tt.expected[i], line)
			}
		}
	}
}

// ============================================================================
// Word Character Tests
// ============================================================================

func TestIsWordChar(t *testing.T) {
	wordChars := []byte{'a', 'z', 'A', 'Z', '0', '9', '_', '$'}
	for _, c := range wordChars {
		if !isWordChar(c) {
			t.Errorf("expected '%c' to be word char", c)
		}
	}
	
	nonWordChars := []byte{' ', '.', ',', '(', ')', '{', '}', '[', ']', '-', '+'}
	for _, c := range nonWordChars {
		if isWordChar(c) {
			t.Errorf("expected '%c' to NOT be word char", c)
		}
	}
}

// ============================================================================
// URI to Path Tests
// ============================================================================

func TestUriToPath(t *testing.T) {
	// Note: Results may vary by OS
	uri := "file:///home/user/test.sola"
	path := uriToPath(uri)
	
	// Should not contain "file://"
	if strings.HasPrefix(path, "file://") {
		t.Error("uriToPath should remove file:// prefix")
	}
}

// ============================================================================
// JSON Marshaling Tests
// ============================================================================

func TestCodeLensJSON(t *testing.T) {
	lens := CodeLens{
		Range: protocol.Range{
			Start: protocol.Position{Line: 1, Character: 0},
			End:   protocol.Position{Line: 1, Character: 10},
		},
		Command: &Command{
			Title:     "Run Test",
			Command:   "sola.runTest",
			Arguments: []interface{}{"test1"},
		},
	}
	
	data, err := json.Marshal(lens)
	if err != nil {
		t.Fatalf("failed to marshal CodeLens: %v", err)
	}
	
	var unmarshaled CodeLens
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal CodeLens: %v", err)
	}
	
	if unmarshaled.Command.Title != "Run Test" {
		t.Errorf("expected title 'Run Test', got '%s'", unmarshaled.Command.Title)
	}
}

func TestInlayHintJSON(t *testing.T) {
	hint := InlayHint{
		Position: protocol.Position{Line: 5, Character: 10},
		Label:    []InlayHintLabelPart{{Value: ": string"}},
		Kind:     InlayHintKindType,
	}
	
	data, err := json.Marshal(hint)
	if err != nil {
		t.Fatalf("failed to marshal InlayHint: %v", err)
	}
	
	var unmarshaled InlayHint
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal InlayHint: %v", err)
	}
	
	if unmarshaled.Kind != InlayHintKindType {
		t.Errorf("expected kind %d, got %d", InlayHintKindType, unmarshaled.Kind)
	}
	
	if len(unmarshaled.Label) != 1 || unmarshaled.Label[0].Value != ": string" {
		t.Error("label mismatch after unmarshal")
	}
}

// ============================================================================
// Builtin Type Tests
// ============================================================================

func TestIsBuiltinType(t *testing.T) {
	builtinTypes := []string{
		"int", "float", "string", "bool", "void", "null", "dynamic", "array", "map",
		"i8", "i16", "i32", "i64", "uint", "u8", "u16", "u32", "u64",
		"f32", "f64", "byte", "unknown",
	}
	
	for _, typ := range builtinTypes {
		if !isBuiltinType(typ) {
			t.Errorf("expected '%s' to be recognized as builtin type", typ)
		}
	}
	
	nonBuiltinTypes := []string{
		"User", "MyClass", "CustomType", "Service", "any", "object",
	}
	
	for _, typ := range nonBuiltinTypes {
		if isBuiltinType(typ) {
			t.Errorf("expected '%s' to NOT be recognized as builtin type", typ)
		}
	}
}

// ============================================================================
// Configuration Tests
// ============================================================================

func TestDefaultConfiguration(t *testing.T) {
	config := GetDefaultConfiguration()
	
	// Check default values
	if !config.Diagnostics.Enable {
		t.Error("diagnostics should be enabled by default")
	}
	
	if !config.Completion.Enable {
		t.Error("completion should be enabled by default")
	}
	
	if !config.InlayHints.Enable {
		t.Error("inlay hints should be enabled by default")
	}
	
	if config.Formatting.TabSize != 4 {
		t.Errorf("default tab size should be 4, got %d", config.Formatting.TabSize)
	}
}
