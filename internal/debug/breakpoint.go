// breakpoint.go - 断点管理
//
// 实现断点功能：
// 1. 行断点
// 2. 条件断点
// 3. 命中计数断点
// 4. 日志断点

package debug

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// BreakpointType 断点类型
type BreakpointType int

const (
	// BreakpointLine 行断点
	BreakpointLine BreakpointType = iota
	// BreakpointConditional 条件断点
	BreakpointConditional
	// BreakpointHitCount 命中计数断点
	BreakpointHitCount
	// BreakpointLog 日志断点
	BreakpointLog
)

// Breakpoint 断点
type Breakpoint struct {
	// ID 唯一标识
	ID int
	
	// Type 断点类型
	Type BreakpointType
	
	// 位置信息
	File   string
	Line   int
	Column int
	
	// 条件（用于条件断点）
	Condition string
	
	// 命中计数
	HitCondition string // 例如 ">=5"
	HitCount     int64
	
	// 日志消息（用于日志断点）
	LogMessage string
	
	// 状态
	Enabled  bool
	Verified bool
	
	// 源码信息
	Source string
}

// BreakpointManager 断点管理器
type BreakpointManager struct {
	mu sync.RWMutex
	
	// 断点存储
	breakpoints map[int]*Breakpoint
	
	// 按文件索引
	byFile map[string]map[int]*Breakpoint
	
	// 下一个 ID
	nextID int32
}

// NewBreakpointManager 创建断点管理器
func NewBreakpointManager() *BreakpointManager {
	return &BreakpointManager{
		breakpoints: make(map[int]*Breakpoint),
		byFile:      make(map[string]map[int]*Breakpoint),
	}
}

// Add 添加行断点
func (m *BreakpointManager) Add(file string, line int) (*Breakpoint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// 检查是否已存在
	if fileBreakpoints, ok := m.byFile[file]; ok {
		for _, bp := range fileBreakpoints {
			if bp.Line == line {
				return nil, fmt.Errorf("breakpoint already exists at %s:%d", file, line)
			}
		}
	}
	
	// 创建断点
	bp := &Breakpoint{
		ID:       int(atomic.AddInt32(&m.nextID, 1)),
		Type:     BreakpointLine,
		File:     file,
		Line:     line,
		Enabled:  true,
		Verified: true,
	}
	
	// 添加到存储
	m.breakpoints[bp.ID] = bp
	
	// 添加到文件索引
	if m.byFile[file] == nil {
		m.byFile[file] = make(map[int]*Breakpoint)
	}
	m.byFile[file][bp.ID] = bp
	
	return bp, nil
}

// AddConditional 添加条件断点
func (m *BreakpointManager) AddConditional(file string, line int, condition string) (*Breakpoint, error) {
	bp, err := m.Add(file, line)
	if err != nil {
		return nil, err
	}
	
	m.mu.Lock()
	defer m.mu.Unlock()
	
	bp.Type = BreakpointConditional
	bp.Condition = condition
	
	return bp, nil
}

// AddHitCount 添加命中计数断点
func (m *BreakpointManager) AddHitCount(file string, line int, hitCondition string) (*Breakpoint, error) {
	bp, err := m.Add(file, line)
	if err != nil {
		return nil, err
	}
	
	m.mu.Lock()
	defer m.mu.Unlock()
	
	bp.Type = BreakpointHitCount
	bp.HitCondition = hitCondition
	
	return bp, nil
}

// AddLog 添加日志断点
func (m *BreakpointManager) AddLog(file string, line int, logMessage string) (*Breakpoint, error) {
	bp, err := m.Add(file, line)
	if err != nil {
		return nil, err
	}
	
	m.mu.Lock()
	defer m.mu.Unlock()
	
	bp.Type = BreakpointLog
	bp.LogMessage = logMessage
	
	return bp, nil
}

// Remove 移除断点
func (m *BreakpointManager) Remove(id int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	bp, ok := m.breakpoints[id]
	if !ok {
		return fmt.Errorf("breakpoint not found: %d", id)
	}
	
	// 从存储移除
	delete(m.breakpoints, id)
	
	// 从文件索引移除
	if fileBreakpoints, ok := m.byFile[bp.File]; ok {
		delete(fileBreakpoints, id)
		if len(fileBreakpoints) == 0 {
			delete(m.byFile, bp.File)
		}
	}
	
	return nil
}

// RemoveAll 移除所有断点
func (m *BreakpointManager) RemoveAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.breakpoints = make(map[int]*Breakpoint)
	m.byFile = make(map[string]map[int]*Breakpoint)
}

// Get 获取断点
func (m *BreakpointManager) Get(id int) (*Breakpoint, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	bp, ok := m.breakpoints[id]
	return bp, ok
}

// GetAll 获取所有断点
func (m *BreakpointManager) GetAll() []*Breakpoint {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	result := make([]*Breakpoint, 0, len(m.breakpoints))
	for _, bp := range m.breakpoints {
		result = append(result, bp)
	}
	return result
}

// GetForFile 获取文件的所有断点
func (m *BreakpointManager) GetForFile(file string) []*Breakpoint {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	fileBreakpoints, ok := m.byFile[file]
	if !ok {
		return nil
	}
	
	result := make([]*Breakpoint, 0, len(fileBreakpoints))
	for _, bp := range fileBreakpoints {
		result = append(result, bp)
	}
	return result
}

// Enable 启用断点
func (m *BreakpointManager) Enable(id int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	bp, ok := m.breakpoints[id]
	if !ok {
		return fmt.Errorf("breakpoint not found: %d", id)
	}
	
	bp.Enabled = true
	return nil
}

// Disable 禁用断点
func (m *BreakpointManager) Disable(id int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	bp, ok := m.breakpoints[id]
	if !ok {
		return fmt.Errorf("breakpoint not found: %d", id)
	}
	
	bp.Enabled = false
	return nil
}

// ShouldBreak 检查是否应该在指定位置中断
func (m *BreakpointManager) ShouldBreak(file string, line int) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	fileBreakpoints, ok := m.byFile[file]
	if !ok {
		return false
	}
	
	for _, bp := range fileBreakpoints {
		if bp.Line == line && bp.Enabled {
			// 增加命中计数
			atomic.AddInt64(&bp.HitCount, 1)
			
			// 检查断点类型
			switch bp.Type {
			case BreakpointLine:
				return true
				
			case BreakpointConditional:
				// TODO: 评估条件表达式
				// 目前简单返回 true
				return true
				
			case BreakpointHitCount:
				return m.checkHitCondition(bp)
				
			case BreakpointLog:
				// 日志断点不中断，只输出日志
				// TODO: 输出日志消息
				return false
			}
		}
	}
	
	return false
}

// checkHitCondition 检查命中条件
func (m *BreakpointManager) checkHitCondition(bp *Breakpoint) bool {
	// 简单实现：解析 ">=N" 或 "==N" 或 "%N" 格式
	condition := bp.HitCondition
	hitCount := atomic.LoadInt64(&bp.HitCount)
	
	if len(condition) < 2 {
		return true
	}
	
	var op string
	var value int64
	
	switch {
	case condition[0] == '>' && condition[1] == '=':
		op = ">="
		fmt.Sscanf(condition[2:], "%d", &value)
	case condition[0] == '=' && condition[1] == '=':
		op = "=="
		fmt.Sscanf(condition[2:], "%d", &value)
	case condition[0] == '%':
		op = "%"
		fmt.Sscanf(condition[1:], "%d", &value)
	default:
		// 默认作为等于处理
		fmt.Sscanf(condition, "%d", &value)
		op = "=="
	}
	
	switch op {
	case ">=":
		return hitCount >= value
	case "==":
		return hitCount == value
	case "%":
		return value > 0 && hitCount%value == 0
	default:
		return true
	}
}

// SetCondition 设置断点条件
func (m *BreakpointManager) SetCondition(id int, condition string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	bp, ok := m.breakpoints[id]
	if !ok {
		return fmt.Errorf("breakpoint not found: %d", id)
	}
	
	bp.Condition = condition
	if condition != "" {
		bp.Type = BreakpointConditional
	} else {
		bp.Type = BreakpointLine
	}
	
	return nil
}

// SetHitCondition 设置命中条件
func (m *BreakpointManager) SetHitCondition(id int, hitCondition string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	bp, ok := m.breakpoints[id]
	if !ok {
		return fmt.Errorf("breakpoint not found: %d", id)
	}
	
	bp.HitCondition = hitCondition
	if hitCondition != "" {
		bp.Type = BreakpointHitCount
	}
	
	return nil
}

// SetLogMessage 设置日志消息
func (m *BreakpointManager) SetLogMessage(id int, logMessage string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	bp, ok := m.breakpoints[id]
	if !ok {
		return fmt.Errorf("breakpoint not found: %d", id)
	}
	
	bp.LogMessage = logMessage
	if logMessage != "" {
		bp.Type = BreakpointLog
	}
	
	return nil
}

// Count 获取断点数量
func (m *BreakpointManager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.breakpoints)
}

// ClearHitCounts 清除所有命中计数
func (m *BreakpointManager) ClearHitCounts() {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	for _, bp := range m.breakpoints {
		atomic.StoreInt64(&bp.HitCount, 0)
	}
}
