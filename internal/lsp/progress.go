package lsp

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// ProgressReporter 进度报告器
type ProgressReporter struct {
	server    *Server
	tokenID   int64
	activeOps map[string]*progressOperation
	mu        sync.Mutex
}

// progressOperation 进度操作
type progressOperation struct {
	token   string
	title   string
	message string
	percent uint32
	done    bool
}

// NewProgressReporter 创建进度报告器
func NewProgressReporter(server *Server) *ProgressReporter {
	return &ProgressReporter{
		server:    server,
		activeOps: make(map[string]*progressOperation),
	}
}

// generateToken 生成唯一的进度令牌
func (pr *ProgressReporter) generateToken() string {
	id := atomic.AddInt64(&pr.tokenID, 1)
	return fmt.Sprintf("sola-progress-%d", id)
}

// BeginProgress 开始进度报告
func (pr *ProgressReporter) BeginProgress(title, message string) string {
	token := pr.generateToken()

	pr.mu.Lock()
	pr.activeOps[token] = &progressOperation{
		token:   token,
		title:   title,
		message: message,
		percent: 0,
	}
	pr.mu.Unlock()

	// 发送 window/workDoneProgress/create 请求
	pr.server.sendNotification("$/progress", map[string]interface{}{
		"token": token,
		"value": map[string]interface{}{
			"kind":       "begin",
			"title":      title,
			"message":    message,
			"percentage": 0,
			"cancellable": false,
		},
	})

	return token
}

// ReportProgress 报告进度
func (pr *ProgressReporter) ReportProgress(token string, message string, percent uint32) {
	pr.mu.Lock()
	op, exists := pr.activeOps[token]
	if exists {
		op.message = message
		op.percent = percent
	}
	pr.mu.Unlock()

	if !exists {
		return
	}

	pr.server.sendNotification("$/progress", map[string]interface{}{
		"token": token,
		"value": map[string]interface{}{
			"kind":       "report",
			"message":    message,
			"percentage": percent,
		},
	})
}

// EndProgress 结束进度报告
func (pr *ProgressReporter) EndProgress(token string, message string) {
	pr.mu.Lock()
	op, exists := pr.activeOps[token]
	if exists {
		op.done = true
		delete(pr.activeOps, token)
	}
	pr.mu.Unlock()

	if !exists {
		return
	}

	pr.server.sendNotification("$/progress", map[string]interface{}{
		"token": token,
		"value": map[string]interface{}{
			"kind":    "end",
			"message": message,
		},
	})
}

// WithProgress 执行带进度报告的操作
func (pr *ProgressReporter) WithProgress(title string, fn func(report func(message string, percent uint32))) {
	token := pr.BeginProgress(title, "正在初始化...")

	report := func(message string, percent uint32) {
		pr.ReportProgress(token, message, percent)
	}

	fn(report)

	pr.EndProgress(token, "完成")
}

// IndexingProgress 索引进度
func (pr *ProgressReporter) IndexingProgress(current, total int, filename string) {
	percent := uint32(0)
	if total > 0 {
		percent = uint32(current * 100 / total)
	}

	// 如果没有活动的索引操作，创建一个
	pr.mu.Lock()
	var indexToken string
	for token, op := range pr.activeOps {
		if op.title == "索引工作区" {
			indexToken = token
			break
		}
	}
	pr.mu.Unlock()

	if indexToken == "" {
		indexToken = pr.BeginProgress("索引工作区", "正在索引...")
	}

	message := fmt.Sprintf("正在索引: %s (%d/%d)", filename, current, total)
	pr.ReportProgress(indexToken, message, percent)

	if current >= total {
		pr.EndProgress(indexToken, fmt.Sprintf("已索引 %d 个文件", total))
	}
}
