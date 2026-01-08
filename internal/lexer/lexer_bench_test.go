package lexer

import (
	"strings"
	"testing"
)

// ============================================================================
// Lexer 基准测试
// ============================================================================
//
// 运行基准测试：
//   go test -bench=. -benchmem ./internal/lexer/...
//
// 对比优化前后：
//   go test -bench=. -benchmem -count=5 ./internal/lexer/... > new.txt
//   # 切换到优化前的代码
//   go test -bench=. -benchmem -count=5 ./internal/lexer/... > old.txt
//   benchstat old.txt new.txt
//
// ============================================================================

// 测试源码样本：模拟真实的 Sola 代码
var benchSource = `
// 这是一个基准测试用的示例代码
// 包含各种常见的语法结构

namespace App\Controllers;

use App\Models\User;
use App\Services\AuthService;

class UserController extends BaseController implements Authenticatable {
    private AuthService $authService;
    private int $maxRetries = 3;
    
    public function __construct(AuthService $authService) {
        $this->authService = $authService;
    }
    
    public function login(string $username, string $password) -> bool {
        // 验证输入
        if ($username == "" || $password == "") {
            return false;
        }
        
        // 尝试登录
        for ($i := 0; $i < $this->maxRetries; $i++) {
            $result := $this->authService->authenticate($username, $password);
            if ($result != null) {
                return true;
            }
        }
        
        return false;
    }
    
    public function getUser(int $id) -> User? {
        $user := User::find($id);
        return $user?.isActive() ? $user : null;
    }
    
    public function calculateScore(float $base, int $multiplier) -> float {
        $score := $base * $multiplier;
        $bonus := 1.5e2 + 0x10 + 0b1010;
        return $score + $bonus;
    }
    
    private function formatMessage(string $template, map<string, string> $params) -> string {
        $message := #"Hello, {$params['name']}! Your score is {$params['score']}.";
        return $message;
    }
}
`

// BenchmarkLexer 测试完整的词法分析性能
func BenchmarkLexer(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(int64(len(benchSource)))

	for i := 0; i < b.N; i++ {
		lexer := New(benchSource, "bench.sola")
		_ = lexer.ScanTokens()
	}
}

// BenchmarkLexerLargeFile 测试大文件的词法分析性能
func BenchmarkLexerLargeFile(b *testing.B) {
	// 重复源码创建一个较大的文件
	largeSource := strings.Repeat(benchSource, 100)

	b.ReportAllocs()
	b.SetBytes(int64(len(largeSource)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		lexer := New(largeSource, "large.sola")
		_ = lexer.ScanTokens()
	}
}

// BenchmarkLexerWhitespace 测试空白字符跳过性能
func BenchmarkLexerWhitespace(b *testing.B) {
	// 创建包含大量空白的源码
	source := strings.Repeat("    \t\t    \n", 1000) + "identifier"

	b.ReportAllocs()
	b.SetBytes(int64(len(source)))

	for i := 0; i < b.N; i++ {
		lexer := New(source, "whitespace.sola")
		_ = lexer.ScanTokens()
	}
}

// BenchmarkLexerStrings 测试字符串解析性能
func BenchmarkLexerStrings(b *testing.B) {
	// 创建包含多个字符串的源码
	source := `"simple string" "another string" "yet another"` +
		strings.Repeat(` "string with content number 123"`, 100)

	b.ReportAllocs()
	b.SetBytes(int64(len(source)))

	for i := 0; i < b.N; i++ {
		lexer := New(source, "strings.sola")
		_ = lexer.ScanTokens()
	}
}

// BenchmarkLexerStringsWithEscape 测试带转义的字符串解析性能
func BenchmarkLexerStringsWithEscape(b *testing.B) {
	// 创建包含转义字符的字符串
	source := strings.Repeat(`"hello\nworld\t\"escaped\""`, 100)

	b.ReportAllocs()
	b.SetBytes(int64(len(source)))

	for i := 0; i < b.N; i++ {
		lexer := New(source, "escape.sola")
		_ = lexer.ScanTokens()
	}
}

// BenchmarkLexerNumbers 测试数字解析性能
func BenchmarkLexerNumbers(b *testing.B) {
	// 创建包含各种数字的源码
	source := strings.Repeat("123 456 789 0 1 2 3 4 5 6 7 8 9 ", 50) +
		strings.Repeat("3.14 2.718 1.0e10 ", 30) +
		strings.Repeat("0xFF 0x1234 0b1010 ", 20)

	b.ReportAllocs()
	b.SetBytes(int64(len(source)))

	for i := 0; i < b.N; i++ {
		lexer := New(source, "numbers.sola")
		_ = lexer.ScanTokens()
	}
}

// BenchmarkLexerIdentifiers 测试标识符解析性能
func BenchmarkLexerIdentifiers(b *testing.B) {
	// 创建包含各种标识符的源码（包括关键字）
	source := strings.Repeat("foo bar baz qux identifier variable ", 50) +
		strings.Repeat("if else for while return function class ", 30) +
		strings.Repeat("int string bool float void ", 20)

	b.ReportAllocs()
	b.SetBytes(int64(len(source)))

	for i := 0; i < b.N; i++ {
		lexer := New(source, "idents.sola")
		_ = lexer.ScanTokens()
	}
}

// BenchmarkLexerOperators 测试运算符解析性能
func BenchmarkLexerOperators(b *testing.B) {
	// 创建包含各种运算符的源码
	source := strings.Repeat("+ - * / % = == != < <= > >= && || ", 50) +
		strings.Repeat("+= -= *= /= ++ -- ", 30) +
		strings.Repeat("& | ^ ~ << >> ", 20)

	b.ReportAllocs()
	b.SetBytes(int64(len(source)))

	for i := 0; i < b.N; i++ {
		lexer := New(source, "operators.sola")
		_ = lexer.ScanTokens()
	}
}

// BenchmarkLexerComments 测试注释跳过性能
func BenchmarkLexerComments(b *testing.B) {
	// 创建包含大量注释的源码
	source := strings.Repeat("// single line comment\n", 50) +
		strings.Repeat("/* block comment */ ", 30) +
		"/* nested /* comment */ */ identifier"

	b.ReportAllocs()
	b.SetBytes(int64(len(source)))

	for i := 0; i < b.N; i++ {
		lexer := New(source, "comments.sola")
		_ = lexer.ScanTokens()
	}
}
