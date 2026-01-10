package lsp

import (
	"path/filepath"
	"strings"

	"github.com/tangzhangming/nova/internal/ast"
	"go.lsp.dev/protocol"
)

// TestInfo 测试信息
type TestInfo struct {
	Name      string          // 测试名称
	Kind      TestKind        // 测试类型
	Range     protocol.Range  // 位置范围
	URI       string          // 文件URI
	ClassName string          // 类名（如果是测试类方法）
}

// TestKind 测试类型
type TestKind int

const (
	TestKindFunction TestKind = iota // 测试函数
	TestKindMethod                   // 测试方法（在测试类中）
	TestKindClass                    // 测试类
)

// TestResult 测试结果
type TestResult struct {
	Name     string       // 测试名称
	Passed   bool         // 是否通过
	Duration int64        // 执行时间（毫秒）
	Error    string       // 错误信息
	Output   string       // 输出内容
	Location TestLocation // 测试位置
}

// TestLocation 测试位置
type TestLocation struct {
	URI   string // 文件URI
	Line  int    // 行号
	Class string // 类名（可选）
}

// IsTestFile 检查文件是否是测试文件
func IsTestFile(filename string) bool {
	base := filepath.Base(filename)
	ext := filepath.Ext(base)
	if ext != ".sola" {
		return false
	}

	name := strings.TrimSuffix(base, ext)
	// 匹配模式: *_test.sola, test_*.sola, *Test.sola
	return strings.HasSuffix(name, "_test") ||
		strings.HasPrefix(name, "test_") ||
		strings.HasSuffix(name, "Test")
}

// IsTestClass 检查类是否是测试类
func IsTestClass(classDecl *ast.ClassDecl) bool {
	name := classDecl.Name.Name
	// 匹配模式: *Test, Test*, *TestCase
	return strings.HasSuffix(name, "Test") ||
		strings.HasPrefix(name, "Test") ||
		strings.HasSuffix(name, "TestCase")
}

// IsTestMethod 检查方法是否是测试方法
func IsTestMethod(method *ast.MethodDecl) bool {
	name := method.Name.Name
	// 匹配模式: test*, Test*, should*, it*, spec*
	return strings.HasPrefix(name, "test") ||
		strings.HasPrefix(name, "Test") ||
		strings.HasPrefix(name, "should") ||
		strings.HasPrefix(name, "it") ||
		strings.HasPrefix(name, "spec")
}

// IsSetupMethod 检查是否是 setup 方法
func IsSetupMethod(method *ast.MethodDecl) bool {
	name := method.Name.Name
	return name == "setUp" ||
		name == "setup" ||
		name == "beforeEach" ||
		name == "beforeAll"
}

// IsTeardownMethod 检查是否是 teardown 方法
func IsTeardownMethod(method *ast.MethodDecl) bool {
	name := method.Name.Name
	return name == "tearDown" ||
		name == "teardown" ||
		name == "afterEach" ||
		name == "afterAll"
}

// FindTests 在文档中查找所有测试
func (s *Server) FindTests(doc *Document) []TestInfo {
	var tests []TestInfo

	astFile := doc.GetAST()
	if astFile == nil {
		return tests
	}

	// 查找测试类和测试方法
	for _, decl := range astFile.Declarations {
		switch d := decl.(type) {
		case *ast.ClassDecl:
			if IsTestClass(d) {
				// 添加测试类
				tests = append(tests, TestInfo{
					Name:  d.Name.Name,
					Kind:  TestKindClass,
					Range: s.nodeRange(d.Name),
					URI:   doc.URI,
				})

				// 添加测试方法
				for _, method := range d.Methods {
					if IsTestMethod(method) && method.Body != nil {
						tests = append(tests, TestInfo{
							Name:      method.Name.Name,
							Kind:      TestKindMethod,
							Range:     s.nodeRange(method.Name),
							URI:       doc.URI,
							ClassName: d.Name.Name,
						})
					}
				}
			}
		}
	}

	return tests
}

// GetTestCodeLenses 获取测试相关的 Code Lenses
func (s *Server) GetTestCodeLenses(doc *Document) []CodeLens {
	var lenses []CodeLens

	// 检查是否是测试文件
	path := uriToPath(doc.URI)
	if !IsTestFile(path) {
		return lenses
	}

	astFile := doc.GetAST()
	if astFile == nil {
		return lenses
	}

	// 文件级别：运行所有测试
	lenses = append(lenses, CodeLens{
		Range: protocol.Range{
			Start: protocol.Position{Line: 0, Character: 0},
			End:   protocol.Position{Line: 0, Character: 0},
		},
		Command: &Command{
			Title:   "▶ Run All Tests",
			Command: "sola.runTests",
			Arguments: []interface{}{
				doc.URI,
				"file",
				"",
			},
		},
	})

	// 查找测试类和方法
	for _, decl := range astFile.Declarations {
		switch d := decl.(type) {
		case *ast.ClassDecl:
			if IsTestClass(d) {
				// 类级别：运行类中所有测试
				lenses = append(lenses, CodeLens{
					Range: s.nodeRange(d.Name),
					Command: &Command{
						Title:   "▶ Run Tests",
						Command: "sola.runTests",
						Arguments: []interface{}{
							doc.URI,
							"class",
							d.Name.Name,
						},
					},
				})

				// 添加调试按钮
				lenses = append(lenses, CodeLens{
					Range: s.nodeRange(d.Name),
					Command: &Command{
						Title:   "🐛 Debug Tests",
						Command: "sola.debugTests",
						Arguments: []interface{}{
							doc.URI,
							"class",
							d.Name.Name,
						},
					},
				})

				// 测试方法级别
				for _, method := range d.Methods {
					if IsTestMethod(method) && method.Body != nil {
						// 运行单个测试
						lenses = append(lenses, CodeLens{
							Range: s.nodeRange(method.Name),
							Command: &Command{
								Title:   "▶ Run Test",
								Command: "sola.runTests",
								Arguments: []interface{}{
									doc.URI,
									"method",
									d.Name.Name + "::" + method.Name.Name,
								},
							},
						})

						// 调试单个测试
						lenses = append(lenses, CodeLens{
							Range: s.nodeRange(method.Name),
							Command: &Command{
								Title:   "🐛 Debug",
								Command: "sola.debugTests",
								Arguments: []interface{}{
									doc.URI,
									"method",
									d.Name.Name + "::" + method.Name.Name,
								},
							},
						})
					}
				}
			}
		}
	}

	return lenses
}

// nodeRange 获取节点的范围
func (s *Server) nodeRange(node ast.Node) protocol.Range {
	startPos := node.Pos()
	endPos := node.End()

	return protocol.Range{
		Start: protocol.Position{
			Line:      uint32(startPos.Line - 1),
			Character: uint32(startPos.Column - 1),
		},
		End: protocol.Position{
			Line:      uint32(endPos.Line - 1),
			Character: uint32(endPos.Column - 1),
		},
	}
}

// TestRunConfig 测试运行配置
type TestRunConfig struct {
	URI       string   // 文件URI
	Scope     string   // 范围: file, class, method
	Target    string   // 目标: 类名或方法名 (类名::方法名)
	Verbose   bool     // 详细输出
	Coverage  bool     // 收集覆盖率
	Timeout   int      // 超时时间（秒）
	Filter    string   // 测试过滤器
	Tags      []string // 测试标签
}

// TestCoverage 测试覆盖率信息
type TestCoverage struct {
	URI           string           // 文件URI
	Lines         []LineCoverage   // 行覆盖率
	BranchCoverage float64         // 分支覆盖率
	LineCoverage   float64         // 行覆盖率
	FunctionCoverage float64       // 函数覆盖率
}

// LineCoverage 行覆盖率
type LineCoverage struct {
	Line     int  // 行号
	Covered  bool // 是否被覆盖
	HitCount int  // 执行次数
}

// TestSuite 测试套件信息
type TestSuite struct {
	Name      string       // 套件名称
	Tests     []TestInfo   // 包含的测试
	SetUp     *TestInfo    // setUp 方法
	TearDown  *TestInfo    // tearDown 方法
	Duration  int64        // 总执行时间
	Passed    int          // 通过数量
	Failed    int          // 失败数量
	Skipped   int          // 跳过数量
}

// GetTestSuites 获取文档中的测试套件
func (s *Server) GetTestSuites(doc *Document) []TestSuite {
	var suites []TestSuite

	astFile := doc.GetAST()
	if astFile == nil {
		return suites
	}

	for _, decl := range astFile.Declarations {
		if classDecl, ok := decl.(*ast.ClassDecl); ok && IsTestClass(classDecl) {
			suite := TestSuite{
				Name:  classDecl.Name.Name,
				Tests: []TestInfo{},
			}

			for _, method := range classDecl.Methods {
				if method.Body == nil {
					continue
				}

				info := TestInfo{
					Name:      method.Name.Name,
					Range:     s.nodeRange(method.Name),
					URI:       doc.URI,
					ClassName: classDecl.Name.Name,
				}

				if IsSetupMethod(method) {
					info.Kind = TestKindFunction
					suite.SetUp = &info
				} else if IsTeardownMethod(method) {
					info.Kind = TestKindFunction
					suite.TearDown = &info
				} else if IsTestMethod(method) {
					info.Kind = TestKindMethod
					suite.Tests = append(suite.Tests, info)
				}
			}

			suites = append(suites, suite)
		}
	}

	return suites
}

// DiagnoseTestIssues 诊断测试问题
func (s *Server) DiagnoseTestIssues(doc *Document) []protocol.Diagnostic {
	var diagnostics []protocol.Diagnostic

	path := uriToPath(doc.URI)
	if !IsTestFile(path) {
		return diagnostics
	}

	astFile := doc.GetAST()
	if astFile == nil {
		return diagnostics
	}

	for _, decl := range astFile.Declarations {
		if classDecl, ok := decl.(*ast.ClassDecl); ok && IsTestClass(classDecl) {
			hasTests := false
			for _, method := range classDecl.Methods {
				if IsTestMethod(method) {
					hasTests = true
					
					// 检查测试方法是否有断言
					if method.Body != nil && !hasAssertions(method.Body) {
						diagnostics = append(diagnostics, protocol.Diagnostic{
							Range:    s.nodeRange(method.Name),
							Severity: protocol.DiagnosticSeverityWarning,
							Source:   "sola-test",
							Message:  "Test method has no assertions",
							Tags:     []protocol.DiagnosticTag{},
						})
					}
					
					// 检查测试方法是否为空
					if method.Body != nil && len(method.Body.Statements) == 0 {
						diagnostics = append(diagnostics, protocol.Diagnostic{
							Range:    s.nodeRange(method.Name),
							Severity: protocol.DiagnosticSeverityWarning,
							Source:   "sola-test",
							Message:  "Test method is empty",
							Tags:     []protocol.DiagnosticTag{},
						})
					}
				}
			}

			// 检查测试类是否有测试方法
			if !hasTests {
				diagnostics = append(diagnostics, protocol.Diagnostic{
					Range:    s.nodeRange(classDecl.Name),
					Severity: protocol.DiagnosticSeverityWarning,
					Source:   "sola-test",
					Message:  "Test class has no test methods",
					Tags:     []protocol.DiagnosticTag{},
				})
			}
		}
	}

	return diagnostics
}

// hasAssertions 检查代码块是否包含断言调用
func hasAssertions(block *ast.BlockStmt) bool {
	if block == nil {
		return false
	}

	found := false
	ast.Walk(block, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.MethodCall:
			// 检查常见的断言方法名
			if isAssertionMethod(n.Method.Name) {
				found = true
				return false
			}
		case *ast.CallExpr:
			// 检查函数调用形式的断言
			if ident, ok := n.Function.(*ast.Identifier); ok {
				if isAssertionFunction(ident.Name) {
					found = true
					return false
				}
			}
		case *ast.StaticAccess:
			// 检查静态方法调用形式的断言 Assert::equals(...)
			if call, ok := n.Member.(*ast.CallExpr); ok {
				if ident, ok := call.Function.(*ast.Identifier); ok {
					if isAssertionMethod(ident.Name) {
						found = true
						return false
					}
				}
			}
		}
		return !found
	})

	return found
}

// isAssertionMethod 检查是否是断言方法
func isAssertionMethod(name string) bool {
	assertionMethods := []string{
		"assertEquals", "assertEqual", "equals", "equal",
		"assertNotEquals", "assertNotEqual", "notEquals", "notEqual",
		"assertTrue", "isTrue", "true",
		"assertFalse", "isFalse", "false",
		"assertNull", "isNull", "null",
		"assertNotNull", "isNotNull", "notNull",
		"assertSame", "same",
		"assertNotSame", "notSame",
		"assertContains", "contains",
		"assertNotContains", "notContains",
		"assertEmpty", "isEmpty", "empty",
		"assertNotEmpty", "isNotEmpty", "notEmpty",
		"assertInstanceOf", "isInstanceOf", "instanceOf",
		"assertThrows", "throws", "expectException",
		"assertCount", "count",
		"assertGreaterThan", "greaterThan",
		"assertLessThan", "lessThan",
		"assertGreaterThanOrEqual", "greaterThanOrEqual",
		"assertLessThanOrEqual", "lessThanOrEqual",
		"assertArrayHasKey", "arrayHasKey",
		"assertStringContains", "stringContains",
		"assertMatchesRegex", "matchesRegex",
		"expect", "should", "must",
	}

	for _, method := range assertionMethods {
		if strings.EqualFold(name, method) {
			return true
		}
	}
	return false
}

// isAssertionFunction 检查是否是断言函数
func isAssertionFunction(name string) bool {
	assertionFunctions := []string{
		"assert", "expect", "verify",
		"assertTrue", "assertFalse",
		"assertEquals", "assertNotEquals",
	}

	for _, fn := range assertionFunctions {
		if strings.EqualFold(name, fn) {
			return true
		}
	}
	return false
}

// TestAnnotation 测试注解信息
type TestAnnotation struct {
	Name   string            // 注解名称
	Args   map[string]string // 注解参数
}

// GetTestAnnotations 获取测试方法上的注解
func GetTestAnnotations(method *ast.MethodDecl) []TestAnnotation {
	var annotations []TestAnnotation

	for _, ann := range method.Annotations {
		testAnn := TestAnnotation{
			Name: ann.Name.Name,
			Args: make(map[string]string),
		}

		// 解析注解参数
		for i, arg := range ann.Args {
			if str, ok := arg.(*ast.StringLiteral); ok {
				testAnn.Args[string(rune('0'+i))] = str.Value
			}
		}

		// 识别测试相关注解
		switch testAnn.Name {
		case "Test", "test":
			annotations = append(annotations, testAnn)
		case "Skip", "skip", "Ignore", "ignore":
			annotations = append(annotations, testAnn)
		case "Timeout", "timeout":
			annotations = append(annotations, testAnn)
		case "DataProvider", "dataProvider":
			annotations = append(annotations, testAnn)
		case "DependsOn", "dependsOn":
			annotations = append(annotations, testAnn)
		case "Group", "group", "Tag", "tag":
			annotations = append(annotations, testAnn)
		case "Before", "before", "BeforeEach", "beforeEach":
			annotations = append(annotations, testAnn)
		case "After", "after", "AfterEach", "afterEach":
			annotations = append(annotations, testAnn)
		case "BeforeAll", "beforeAll", "BeforeClass", "beforeClass":
			annotations = append(annotations, testAnn)
		case "AfterAll", "afterAll", "AfterClass", "afterClass":
			annotations = append(annotations, testAnn)
		}
	}

	return annotations
}

// ShouldSkipTest 检查测试是否应该跳过
func ShouldSkipTest(method *ast.MethodDecl) bool {
	annotations := GetTestAnnotations(method)
	for _, ann := range annotations {
		if ann.Name == "Skip" || ann.Name == "skip" || 
		   ann.Name == "Ignore" || ann.Name == "ignore" {
			return true
		}
	}
	return false
}
