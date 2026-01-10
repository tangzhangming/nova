package lsp

import (
	"strings"
	"sync"

	"github.com/tangzhangming/nova/internal/ast"
	"github.com/tangzhangming/nova/internal/compiler"
	"github.com/tangzhangming/nova/internal/parser"
	"github.com/tangzhangming/nova/internal/token"
	"go.lsp.dev/protocol"
)

// Document 表示一个打开的文档
type Document struct {
	URI     string
	Content string
	Version int
	Lines   []string // 按行分割的内容

	// 缓存的解析结果
	AST       *ast.File
	Symbols   *compiler.SymbolTable
	ParseErrs []parser.Error

	// 是否需要重新解析
	dirty bool
	
	// 文档专属的loader（用于解析该文档的导入）
	Loader interface {
		ResolveImport(importPath string) (string, error)
		GetProjectNamespace() string
		RootDir() string
	}
}

// DocumentManager 文档管理器
type DocumentManager struct {
	documents map[string]*Document
	mu        sync.RWMutex
	
	// 内存管理
	maxDocuments int // 最大文档数量限制
}

// NewDocumentManager 创建文档管理器
func NewDocumentManager() *DocumentManager {
	return &DocumentManager{
		documents:    make(map[string]*Document),
		maxDocuments: 10, // 紧急：最多10个文档
	}
}

// Open 打开文档
func (dm *DocumentManager) Open(uri, content string, version int) *Document {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	doc := &Document{
		URI:     uri,
		Content: content,
		Version: version,
		Lines:   splitLines(content),
		dirty:   true,
	}

	// 立即解析
	doc.parse()

	dm.documents[uri] = doc
	return doc
}

// Close 关闭文档
func (dm *DocumentManager) Close(uri string) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	delete(dm.documents, uri)
}

// Get 获取文档
func (dm *DocumentManager) Get(uri string) *Document {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.documents[uri]
}

// UpdateContent 更新文档内容
func (dm *DocumentManager) UpdateContent(uri, content string) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	doc, ok := dm.documents[uri]
	if !ok {
		return
	}

	doc.Content = content
	doc.Lines = splitLines(content)
	doc.Version++
	doc.dirty = true
	doc.parse()
}

// ApplyChange 应用增量变更
func (dm *DocumentManager) ApplyChange(uri string, change protocol.TextDocumentContentChangeEvent, version int) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	doc, ok := dm.documents[uri]
	if !ok {
		return
	}

	// 如果没有范围（Range 为零值），则是完整替换
	// LSP 规范：如果 range 被省略，新文本被认为是文档的完整内容
	isFullReplace := change.Range.Start.Line == 0 &&
		change.Range.Start.Character == 0 &&
		change.Range.End.Line == 0 &&
		change.Range.End.Character == 0 &&
		change.RangeLength == 0

	if isFullReplace {
		doc.Content = change.Text
		doc.Lines = splitLines(change.Text)
	} else {
		// 增量更新
		doc.Content = applyTextEdit(doc.Content, change.Range, change.Text)
		doc.Lines = splitLines(doc.Content)
	}

	doc.Version = version
	doc.dirty = true
	doc.parse()
}

// GetAll 获取所有文档
func (dm *DocumentManager) GetAll() []*Document {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	docs := make([]*Document, 0, len(dm.documents))
	for _, doc := range dm.documents {
		docs = append(docs, doc)
	}
	return docs
}

// maxDocumentSize 文档大小限制（500KB），防止内存暴涨
const maxDocumentSize = 500 * 1024

// parse 解析文档
func (doc *Document) parse() {
	if !doc.dirty {
		return
	}

	// 检查文档大小，防止内存暴涨
	if len(doc.Content) > maxDocumentSize {
		doc.AST = nil
		doc.ParseErrs = []parser.Error{{
			Pos:     token.Position{Line: 1, Column: 1},
			Message: "document too large to parse",
		}}
		doc.Symbols = nil
		doc.dirty = false
		return
	}

	// 获取文件名
	filename := uriToPath(doc.URI)

	// 解析
	p := parser.New(doc.Content, filename)
	doc.AST = p.Parse()
	doc.ParseErrs = p.Errors()

	// 收集符号
	if doc.AST != nil {
		doc.Symbols = compiler.NewSymbolTable()
		doc.Symbols.CollectFromFile(doc.AST)
	}

	doc.dirty = false
}

// GetAST 获取 AST（如果需要会重新解析）
func (doc *Document) GetAST() *ast.File {
	if doc.dirty {
		doc.parse()
	}
	return doc.AST
}

// GetSymbols 获取符号表
func (doc *Document) GetSymbols() *compiler.SymbolTable {
	if doc.dirty {
		doc.parse()
	}
	return doc.Symbols
}

// GetLine 获取指定行内容
func (doc *Document) GetLine(line int) string {
	if line < 0 || line >= len(doc.Lines) {
		return ""
	}
	return doc.Lines[line]
}

// GetWordAt 获取指定位置的单词
func (doc *Document) GetWordAt(line, character int) string {
	if line < 0 || line >= len(doc.Lines) {
		return ""
	}

	lineText := doc.Lines[line]
	if character < 0 || character > len(lineText) {
		return ""
	}

	// 向前查找单词开始
	start := character
	for start > 0 && isWordChar(lineText[start-1]) {
		start--
	}

	// 向后查找单词结束
	end := character
	for end < len(lineText) && isWordChar(lineText[end]) {
		end++
	}

	return lineText[start:end]
}

// GetWordRangeAt 获取指定位置的单词及其范围
func (doc *Document) GetWordRangeAt(line, character int) (word string, startCol, endCol int) {
	if line < 0 || line >= len(doc.Lines) {
		return "", 0, 0
	}

	lineText := doc.Lines[line]
	if character < 0 || character > len(lineText) {
		return "", 0, 0
	}

	// 向前查找单词开始
	start := character
	for start > 0 && isWordChar(lineText[start-1]) {
		start--
	}

	// 向后查找单词结束
	end := character
	for end < len(lineText) && isWordChar(lineText[end]) {
		end++
	}

	return lineText[start:end], start, end
}

// GetOffset 获取指定位置的字节偏移
func (doc *Document) GetOffset(line, character int) int {
	offset := 0
	for i := 0; i < line && i < len(doc.Lines); i++ {
		offset += len(doc.Lines[i]) + 1 // +1 for newline
	}
	offset += character
	return offset
}

// GetPosition 从字节偏移获取位置
func (doc *Document) GetPosition(offset int) (line, character int) {
	currentOffset := 0
	for i, lineText := range doc.Lines {
		lineLen := len(lineText) + 1 // +1 for newline
		if currentOffset+lineLen > offset {
			return i, offset - currentOffset
		}
		currentOffset += lineLen
	}
	return len(doc.Lines) - 1, 0
}

// splitLines 将内容按行分割
func splitLines(content string) []string {
	// 处理不同的换行符
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	return strings.Split(content, "\n")
}

// applyTextEdit 应用文本编辑
func applyTextEdit(content string, rang protocol.Range, newText string) string {
	lines := splitLines(content)

	// 获取开始和结束位置
	startLine := int(rang.Start.Line)
	startChar := int(rang.Start.Character)
	endLine := int(rang.End.Line)
	endChar := int(rang.End.Character)

	// 确保行号有效
	if startLine >= len(lines) {
		startLine = len(lines) - 1
	}
	if endLine >= len(lines) {
		endLine = len(lines) - 1
	}
	if startLine < 0 {
		startLine = 0
	}
	if endLine < 0 {
		endLine = 0
	}

	// 获取开始行和结束行
	startLineText := ""
	endLineText := ""
	if startLine < len(lines) {
		startLineText = lines[startLine]
	}
	if endLine < len(lines) {
		endLineText = lines[endLine]
	}

	// 确保字符位置有效
	if startChar > len(startLineText) {
		startChar = len(startLineText)
	}
	if endChar > len(endLineText) {
		endChar = len(endLineText)
	}

	// 构建新内容
	var result strings.Builder

	// 添加开始位置之前的内容
	for i := 0; i < startLine; i++ {
		result.WriteString(lines[i])
		result.WriteString("\n")
	}
	result.WriteString(startLineText[:startChar])

	// 添加新文本
	result.WriteString(newText)

	// 添加结束位置之后的内容
	result.WriteString(endLineText[endChar:])
	for i := endLine + 1; i < len(lines); i++ {
		result.WriteString("\n")
		result.WriteString(lines[i])
	}

	return result.String()
}

// isWordChar 判断是否是单词字符
func isWordChar(c byte) bool {
	return (c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') ||
		c == '_' || c == '$'
}
