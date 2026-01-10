package lsp2

import (
	"runtime"
	"sync"

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

	// 解析文档
	p := parser.New(d.Content, uriToPath(d.URI))
	d.ast = p.Parse()
	d.parsed = true
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
