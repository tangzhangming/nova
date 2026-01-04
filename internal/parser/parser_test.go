package parser

import (
	"testing"

	"github.com/tangzhangming/nova/internal/ast"
)

func TestParseVariableDeclaration(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`int $count = 100;`, "int"},
		{`string $name = "test";`, "string"},
		{`$x := 42;`, ""},
	}

	for _, tt := range tests {
		p := New(tt.input, "test.nova")
		file := p.Parse()

		if p.HasErrors() {
			for _, err := range p.Errors() {
				t.Errorf("parser error: %v", err)
			}
			continue
		}

		if len(file.Statements) != 1 {
			t.Errorf("expected 1 statement, got %d", len(file.Statements))
			continue
		}

		stmt, ok := file.Statements[0].(*ast.VarDeclStmt)
		if !ok {
			t.Errorf("expected VarDeclStmt, got %T", file.Statements[0])
			continue
		}

		if tt.expected != "" {
			if stmt.Type == nil {
				t.Errorf("expected type %s, got nil", tt.expected)
			} else if stmt.Type.String() != tt.expected {
				t.Errorf("expected type %s, got %s", tt.expected, stmt.Type.String())
			}
		} else {
			if stmt.Type != nil {
				t.Errorf("expected no type, got %s", stmt.Type.String())
			}
		}
	}
}

func TestParseMultiVarDeclaration(t *testing.T) {
	input := `$a, $b := test();`

	p := New(input, "test.nova")
	file := p.Parse()

	if p.HasErrors() {
		for _, err := range p.Errors() {
			t.Errorf("parser error: %v", err)
		}
		return
	}

	if len(file.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(file.Statements))
	}

	stmt, ok := file.Statements[0].(*ast.MultiVarDeclStmt)
	if !ok {
		t.Fatalf("expected MultiVarDeclStmt, got %T", file.Statements[0])
	}

	if len(stmt.Names) != 2 {
		t.Errorf("expected 2 names, got %d", len(stmt.Names))
	}
}

func TestParseExpressions(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`$a = 1 + 2;`, "($a = (1 + 2))"},
		{`$a = 1 * 2 + 3;`, "($a = ((1 * 2) + 3))"},
		{`$a = 1 + 2 * 3;`, "($a = (1 + (2 * 3)))"},
		{`$a = (1 + 2) * 3;`, "($a = ((1 + 2) * 3))"},
		{`$a = $b > 0 ? 1 : 2;`, ""},
	}

	for _, tt := range tests {
		p := New(tt.input, "test.nova")
		file := p.Parse()

		if p.HasErrors() {
			for _, err := range p.Errors() {
				t.Errorf("input %q: parser error: %v", tt.input, err)
			}
			continue
		}

		if len(file.Statements) != 1 {
			t.Errorf("input %q: expected 1 statement, got %d", tt.input, len(file.Statements))
		}
	}
}

func TestParseIfStatement(t *testing.T) {
	input := `
	if ($a > 0) {
		$b = 1;
	} elseif ($a < 0) {
		$b = -1;
	} else {
		$b = 0;
	}
	`

	p := New(input, "test.nova")
	file := p.Parse()

	if p.HasErrors() {
		for _, err := range p.Errors() {
			t.Errorf("parser error: %v", err)
		}
		return
	}

	if len(file.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(file.Statements))
	}

	ifStmt, ok := file.Statements[0].(*ast.IfStmt)
	if !ok {
		t.Fatalf("expected IfStmt, got %T", file.Statements[0])
	}

	if len(ifStmt.ElseIfs) != 1 {
		t.Errorf("expected 1 elseif, got %d", len(ifStmt.ElseIfs))
	}

	if ifStmt.Else == nil {
		t.Errorf("expected else block")
	}
}

func TestParseForLoop(t *testing.T) {
	input := `
	for ($i := 0; $i < 10; $i++) {
		echo $i;
	}
	`

	p := New(input, "test.nova")
	file := p.Parse()

	if p.HasErrors() {
		for _, err := range p.Errors() {
			t.Errorf("parser error: %v", err)
		}
		return
	}

	if len(file.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(file.Statements))
	}

	_, ok := file.Statements[0].(*ast.ForStmt)
	if !ok {
		t.Fatalf("expected ForStmt, got %T", file.Statements[0])
	}
}

func TestParseForeachLoop(t *testing.T) {
	input := `
	foreach ($items as $key => $value) {
		echo $value;
	}
	`

	p := New(input, "test.nova")
	file := p.Parse()

	if p.HasErrors() {
		for _, err := range p.Errors() {
			t.Errorf("parser error: %v", err)
		}
		return
	}

	if len(file.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(file.Statements))
	}

	foreach, ok := file.Statements[0].(*ast.ForeachStmt)
	if !ok {
		t.Fatalf("expected ForeachStmt, got %T", file.Statements[0])
	}

	if foreach.Key == nil {
		t.Error("expected key variable")
	}
}

func TestParseClass(t *testing.T) {
	input := `
	public class User extends BaseModel implements Serializable {
		public const int MAX_AGE = 100;
		
		private string $name;
		public ?string $email = null;
		
		public function __construct(string $name) {
			$this->name = $name;
		}
		
		public function getName(): string {
			return $this->name;
		}
		
		public static function create(string $name): User {
			return new User($name);
		}
	}
	`

	p := New(input, "test.nova")
	file := p.Parse()

	if p.HasErrors() {
		for _, err := range p.Errors() {
			t.Errorf("parser error: %v", err)
		}
		return
	}

	if len(file.Declarations) != 1 {
		t.Fatalf("expected 1 declaration, got %d", len(file.Declarations))
	}

	class, ok := file.Declarations[0].(*ast.ClassDecl)
	if !ok {
		t.Fatalf("expected ClassDecl, got %T", file.Declarations[0])
	}

	if class.Name.Name != "User" {
		t.Errorf("expected class name 'User', got '%s'", class.Name.Name)
	}

	if class.Extends == nil || class.Extends.Name != "BaseModel" {
		t.Error("expected extends BaseModel")
	}

	if len(class.Implements) != 1 || class.Implements[0].Name != "Serializable" {
		t.Error("expected implements Serializable")
	}

	if len(class.Constants) != 1 {
		t.Errorf("expected 1 constant, got %d", len(class.Constants))
	}

	if len(class.Properties) != 2 {
		t.Errorf("expected 2 properties, got %d", len(class.Properties))
	}

	if len(class.Methods) != 3 {
		t.Errorf("expected 3 methods, got %d", len(class.Methods))
	}
}

func TestParseInterface(t *testing.T) {
	input := `
	interface Repository {
		public function find(int $id): ?User;
		public function save(User $user): bool;
		public function delete(int $id): bool;
	}
	`

	p := New(input, "test.nova")
	file := p.Parse()

	if p.HasErrors() {
		for _, err := range p.Errors() {
			t.Errorf("parser error: %v", err)
		}
		return
	}

	if len(file.Declarations) != 1 {
		t.Fatalf("expected 1 declaration, got %d", len(file.Declarations))
	}

	iface, ok := file.Declarations[0].(*ast.InterfaceDecl)
	if !ok {
		t.Fatalf("expected InterfaceDecl, got %T", file.Declarations[0])
	}

	if iface.Name.Name != "Repository" {
		t.Errorf("expected interface name 'Repository', got '%s'", iface.Name.Name)
	}

	if len(iface.Methods) != 3 {
		t.Errorf("expected 3 methods, got %d", len(iface.Methods))
	}
}

func TestParseAbstractClass(t *testing.T) {
	input := `
	public abstract class Shape {
		abstract public function area(): float;
		
		public function describe(): string {
			return "Shape";
		}
	}
	`

	p := New(input, "test.nova")
	file := p.Parse()

	if p.HasErrors() {
		for _, err := range p.Errors() {
			t.Errorf("parser error: %v", err)
		}
		return
	}

	if len(file.Declarations) != 1 {
		t.Fatalf("expected 1 declaration, got %d", len(file.Declarations))
	}

	class, ok := file.Declarations[0].(*ast.ClassDecl)
	if !ok {
		t.Fatalf("expected ClassDecl, got %T", file.Declarations[0])
	}

	if !class.Abstract {
		t.Error("expected abstract class")
	}

	if len(class.Methods) != 2 {
		t.Errorf("expected 2 methods, got %d", len(class.Methods))
	}

	// 第一个方法应该是抽象的
	if !class.Methods[0].Abstract {
		t.Error("expected first method to be abstract")
	}
}

func TestParseClosure(t *testing.T) {
	input := `
	$fn = function(int $x): int {
		return $x * 2;
	};
	`

	p := New(input, "test.nova")
	file := p.Parse()

	if p.HasErrors() {
		for _, err := range p.Errors() {
			t.Errorf("parser error: %v", err)
		}
		return
	}

	if len(file.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(file.Statements))
	}
}

func TestParseArrayLiteral(t *testing.T) {
	input := `$arr := [1, 2, 3, 4];`

	p := New(input, "test.nova")
	file := p.Parse()

	if p.HasErrors() {
		for _, err := range p.Errors() {
			t.Errorf("parser error: %v", err)
		}
		return
	}

	if len(file.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(file.Statements))
	}

	decl, ok := file.Statements[0].(*ast.VarDeclStmt)
	if !ok {
		t.Fatalf("expected VarDeclStmt, got %T", file.Statements[0])
	}

	arr, ok := decl.Value.(*ast.ArrayLiteral)
	if !ok {
		t.Fatalf("expected ArrayLiteral, got %T", decl.Value)
	}

	if len(arr.Elements) != 4 {
		t.Errorf("expected 4 elements, got %d", len(arr.Elements))
	}
}

func TestParseMapLiteral(t *testing.T) {
	input := `$map := ["a" => 1, "b" => 2];`

	p := New(input, "test.nova")
	file := p.Parse()

	if p.HasErrors() {
		for _, err := range p.Errors() {
			t.Errorf("parser error: %v", err)
		}
		return
	}

	if len(file.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(file.Statements))
	}

	decl, ok := file.Statements[0].(*ast.VarDeclStmt)
	if !ok {
		t.Fatalf("expected VarDeclStmt, got %T", file.Statements[0])
	}

	mapLit, ok := decl.Value.(*ast.MapLiteral)
	if !ok {
		t.Fatalf("expected MapLiteral, got %T", decl.Value)
	}

	if len(mapLit.Pairs) != 2 {
		t.Errorf("expected 2 pairs, got %d", len(mapLit.Pairs))
	}
}

func TestParseMethodCall(t *testing.T) {
	input := `$user->getName();`

	p := New(input, "test.nova")
	file := p.Parse()

	if p.HasErrors() {
		for _, err := range p.Errors() {
			t.Errorf("parser error: %v", err)
		}
		return
	}

	if len(file.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(file.Statements))
	}

	exprStmt, ok := file.Statements[0].(*ast.ExprStmt)
	if !ok {
		t.Fatalf("expected ExprStmt, got %T", file.Statements[0])
	}

	_, ok = exprStmt.Expr.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected MethodCall, got %T", exprStmt.Expr)
	}
}

func TestParseStaticAccess(t *testing.T) {
	tests := []string{
		`User::MAX_AGE;`,
		`self::$count;`,
		`parent::create();`,
	}

	for _, input := range tests {
		p := New(input, "test.nova")
		file := p.Parse()

		if p.HasErrors() {
			for _, err := range p.Errors() {
				t.Errorf("input %q: parser error: %v", input, err)
			}
			continue
		}

		if len(file.Statements) != 1 {
			t.Errorf("input %q: expected 1 statement, got %d", input, len(file.Statements))
		}
	}
}

func TestParseTryCatch(t *testing.T) {
	input := `
	try {
		$result = riskyOperation();
	} catch (Exception $e) {
		echo $e->getMessage();
	} finally {
		cleanup();
	}
	`

	p := New(input, "test.nova")
	file := p.Parse()

	if p.HasErrors() {
		for _, err := range p.Errors() {
			t.Errorf("parser error: %v", err)
		}
		return
	}

	if len(file.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(file.Statements))
	}

	tryStmt, ok := file.Statements[0].(*ast.TryStmt)
	if !ok {
		t.Fatalf("expected TryStmt, got %T", file.Statements[0])
	}

	if len(tryStmt.Catches) != 1 {
		t.Errorf("expected 1 catch, got %d", len(tryStmt.Catches))
	}

	if tryStmt.Finally == nil {
		t.Error("expected finally block")
	}
}

func TestParseNamespaceAndUse(t *testing.T) {
	input := `
	namespace company.project
	
	use company.project.models.User;
	use external.lib.Helper as H;
	
	public class Service {
		public function run() {
			$user := new User();
		}
	}
	`

	p := New(input, "test.nova")
	file := p.Parse()

	if p.HasErrors() {
		for _, err := range p.Errors() {
			t.Errorf("parser error: %v", err)
		}
		return
	}

	if file.Namespace == nil {
		t.Fatal("expected namespace")
	}

	if file.Namespace.Name != "company.project" {
		t.Errorf("expected namespace 'company.project', got '%s'", file.Namespace.Name)
	}

	if len(file.Uses) != 2 {
		t.Fatalf("expected 2 use statements, got %d", len(file.Uses))
	}

	if file.Uses[1].Alias == nil || file.Uses[1].Alias.Name != "H" {
		t.Error("expected second use to have alias 'H'")
	}
}

func TestParseAnnotation(t *testing.T) {
	input := `
	public class Controller {
		@Route("/users")
		@Middleware("auth")
		public function getUsers() {
			return [];
		}
	}
	`

	p := New(input, "test.nova")
	file := p.Parse()

	if p.HasErrors() {
		for _, err := range p.Errors() {
			t.Errorf("parser error: %v", err)
		}
		return
	}

	if len(file.Declarations) != 1 {
		t.Fatalf("expected 1 declaration, got %d", len(file.Declarations))
	}

	class := file.Declarations[0].(*ast.ClassDecl)
	if len(class.Methods) != 1 {
		t.Fatalf("expected 1 method, got %d", len(class.Methods))
	}

	method := class.Methods[0]
	if len(method.Annotations) != 2 {
		t.Errorf("expected 2 annotations, got %d", len(method.Annotations))
	}
}
