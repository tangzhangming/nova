package lsp2

import (
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/tangzhangming/nova/internal/ast"
	"github.com/tangzhangming/nova/internal/parser"
)

// Document 表示一个打开的文档
type Document struct {
	URI     string
	Content string
	Version int
	Lines   []string

	// 延迟解析的 AST
	ast    *ast.File
	parsed bool
	mu     sync.Mutex
}

// GetAST 获取文档的 AST（延迟解析）
func (d *Document) GetAST() *ast.File {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.parsed {
		d.parse()
	}
	return d.ast
}

// parse 解析文档（内部方法，不加锁）
func (d *Document) parse() {
	if d.parsed {
		return
	}

	// 检查文档大小限制（500KB）
	if len(d.Content) > 500*1024 {
		d.ast = nil
		d.parsed = true
		return
	}

	// 快速检查：如果文档包含明显不完整的语法，跳过解析
	if hasIncompleteMethodSignature(d.Content) {
		d.ast = nil
		d.parsed = true
		return
	}

	// 使用 goroutine 和超时机制解析文档，防止 parser 卡住
	done := make(chan *ast.File, 1)
	go func() {
		defer func() {
			// 捕获 panic，防止 parser 崩溃导致 goroutine 泄漏
			if r := recover(); r != nil {
				done <- nil
			}
		}()
		p := parser.New(d.Content, uriToPath(d.URI))
		done <- p.Parse()
	}()

	// 设置 1 秒超时（降低超时时间）
	select {
	case result := <-done:
		d.ast = result
	case <-time.After(1 * time.Second):
		// 解析超时，返回 nil
		d.ast = nil
	}
	d.parsed = true
}

// hasIncompleteMethodSignature 检查是否有不完整的方法签名
// 这些情况会导致 parser 卡住或变慢
func hasIncompleteMethodSignature(content string) bool {
	lines := SplitLines(content)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// 检查方法定义行
		if strings.Contains(trimmed, "function ") && strings.Contains(trimmed, "(") {
			// 检查是否有未闭合的括号
			openCount := strings.Count(trimmed, "(")
			closeCount := strings.Count(trimmed, ")")
			if openCount > closeCount {
				// 括号未闭合，可能导致问题
				return true
			}

			// 检查返回类型位置是否有 $ 符号
			// 匹配模式: ) 后面跟着 : 和 $（可能有空格）
			rparen := strings.LastIndex(trimmed, ")")
			if rparen != -1 && rparen < len(trimmed)-1 {
				afterParen := trimmed[rparen+1:]
				// 移除空格后检查是否以 : 开头
				afterParen = strings.TrimSpace(afterParen)
				if strings.HasPrefix(afterParen, ":") {
					afterColon := strings.TrimSpace(afterParen[1:])
					if strings.HasPrefix(afterColon, "$") {
						// 返回类型位置出现变量，这是语法错误
						return true
					}
				}
			}

			// 检查括号内的参数是否完整
			// 提取括号内的内容
			lparen := strings.Index(trimmed, "(")
			if lparen != -1 && rparen != -1 && rparen > lparen {
				params := trimmed[lparen+1 : rparen]
				params = strings.TrimSpace(params)

				// 空参数列表是正常的
				if params == "" {
					continue
				}

				// 检查参数是否完整
				// 正确的参数格式: type $name 或 type $name = default
				// 不完整的情况: 只有类型没有变量名, 或只有部分输入
				if isIncompleteParams(params) {
					return true
				}
			}
		}
	}
	return false
}

// isIncompleteParams 检查参数列表是否不完整
func isIncompleteParams(params string) bool {
	// 检查是否以逗号结尾（表示还在输入下一个参数）
	if strings.HasSuffix(strings.TrimSpace(params), ",") {
		return true
	}

	// 按逗号分割参数
	paramList := strings.Split(params, ",")

	for _, param := range paramList {
		param = strings.TrimSpace(param)
		if param == "" {
			// 空参数（如 "a, , b" 中间的空）表示不完整
			return true
		}

		// 检查参数是否包含变量名（以$开头的部分）
		// 正确的参数: string $name, int $age = 10, ...$args
		// 不完整: string, str, s, string $

		// 如果参数以 ... 开头（可变参数）
		if strings.HasPrefix(param, "...") {
			param = param[3:]
		}

		// 检查是否有 $ 符号（表示变量名）
		dollarIdx := strings.Index(param, "$")
		if dollarIdx == -1 {
			// 没有 $ 符号，可能是不完整的参数
			// 但需要排除一些特殊情况

			// 如果整个参数是空白或特殊字符，跳过
			if len(strings.TrimSpace(param)) == 0 {
				continue
			}

			// 没有变量名，这是不完整的参数
			return true
		}

		// 检查 $ 后面是否有变量名
		afterDollar := param[dollarIdx+1:]
		// 移除可能的默认值部分
		if eqIdx := strings.Index(afterDollar, "="); eqIdx != -1 {
			afterDollar = afterDollar[:eqIdx]
		}
		afterDollar = strings.TrimSpace(afterDollar)

		if len(afterDollar) == 0 {
			// $ 后面没有变量名，不完整
			return true
		}
	}

	return false
}

// Invalidate 标记文档需要重新解析
func (d *Document) Invalidate() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.parsed = false
	d.ast = nil
}

// DocumentManager 文档管理器
type DocumentManager struct {
	docs      map[string]*Document // URI -> Document
	openOrder []string             // LRU 顺序（最近使用的在最后）
	maxDocs   int                  // 最多缓存的文档数量
	mu        sync.Mutex
	logger    *Logger
}

// NewDocumentManager 创建文档管理器
func NewDocumentManager(logger *Logger) *DocumentManager {
	return &DocumentManager{
		docs:      make(map[string]*Document),
		openOrder: make([]string, 0, 10),
		maxDocs:   10, // 最多缓存10个文档
		logger:    logger,
	}
}

// Open 打开文档
func (dm *DocumentManager) Open(uri, content string, version int) *Document {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	// 如果文档已经打开，更新内容
	if doc, exists := dm.docs[uri]; exists {
		doc.Content = content
		doc.Version = version
		doc.Lines = SplitLines(content)
		doc.Invalidate()
		dm.updateLRU(uri)
		dm.logger.Debug("Document updated: %s (version %d)", uri, version)
		return doc
	}

	// 检查是否需要淘汰旧文档
	if len(dm.docs) >= dm.maxDocs {
		dm.evictOldest()
	}

	// 创建新文档
	doc := &Document{
		URI:     uri,
		Content: content,
		Version: version,
		Lines:   SplitLines(content),
		parsed:  false,
	}

	dm.docs[uri] = doc
	dm.openOrder = append(dm.openOrder, uri)
	dm.logger.Debug("Document opened: %s (version %d, size %d bytes)", uri, version, len(content))

	return doc
}

// Close 关闭文档
func (dm *DocumentManager) Close(uri string) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	doc, exists := dm.docs[uri]
	if !exists {
		return
	}

	// 删除文档
	delete(dm.docs, uri)

	// 从 LRU 列表中删除
	for i, u := range dm.openOrder {
		if u == uri {
			dm.openOrder = append(dm.openOrder[:i], dm.openOrder[i+1:]...)
			break
		}
	}

	// 清理 AST
	doc.ast = nil
	doc.Lines = nil
	doc.Content = ""

	dm.logger.Debug("Document closed: %s (remaining: %d)", uri, len(dm.docs))

	// 强制垃圾回收
	runtime.GC()
}

// Get 获取文档
func (dm *DocumentManager) Get(uri string) *Document {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	doc, exists := dm.docs[uri]
	if !exists {
		return nil
	}

	// 更新 LRU
	dm.updateLRU(uri)
	return doc
}

// Update 更新文档内容
func (dm *DocumentManager) Update(uri, content string, version int) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	doc, exists := dm.docs[uri]
	if !exists {
		return
	}

	doc.Content = content
	doc.Version = version
	doc.Lines = SplitLines(content)
	doc.Invalidate()
	dm.updateLRU(uri)

	dm.logger.Debug("Document content updated: %s (version %d)", uri, version)
}

// GetAll 获取所有打开的文档
func (dm *DocumentManager) GetAll() []*Document {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	docs := make([]*Document, 0, len(dm.docs))
	for _, doc := range dm.docs {
		docs = append(docs, doc)
	}
	return docs
}

// Count 返回当前打开的文档数量
func (dm *DocumentManager) Count() int {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	return len(dm.docs)
}

// updateLRU 更新 LRU 顺序（内部方法，调用者需持有锁）
func (dm *DocumentManager) updateLRU(uri string) {
	// 从列表中移除
	for i, u := range dm.openOrder {
		if u == uri {
			dm.openOrder = append(dm.openOrder[:i], dm.openOrder[i+1:]...)
			break
		}
	}
	// 添加到末尾（最近使用）
	dm.openOrder = append(dm.openOrder, uri)
}

// evictOldest 淘汰最旧的文档（内部方法，调用者需持有锁）
func (dm *DocumentManager) evictOldest() {
	if len(dm.openOrder) == 0 {
		return
	}

	// 淘汰最旧的文档（列表开头）
	oldestURI := dm.openOrder[0]
	doc := dm.docs[oldestURI]

	// 删除文档
	delete(dm.docs, oldestURI)
	dm.openOrder = dm.openOrder[1:]

	// 清理 AST
	if doc != nil {
		doc.ast = nil
		doc.Lines = nil
		doc.Content = ""
	}

	dm.logger.Info("Evicted oldest document (LRU): %s", oldestURI)

	// 强制垃圾回收
	runtime.GC()
}
