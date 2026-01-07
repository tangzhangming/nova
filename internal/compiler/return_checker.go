package compiler

import (
	"fmt"

	"github.com/tangzhangming/nova/internal/token"
)

// ReturnChecker 返回值检查器
type ReturnChecker struct {
	cfg        *CFG
	returnType string
	errors     []TypeError
}

// NewReturnChecker 创建返回值检查器
func NewReturnChecker(cfg *CFG, returnType string) *ReturnChecker {
	return &ReturnChecker{
		cfg:        cfg,
		returnType: returnType,
		errors:     make([]TypeError, 0),
	}
}

// CheckAllPathsReturn 检查所有路径是否都有返回值
func (rc *ReturnChecker) CheckAllPathsReturn() bool {
	if rc.cfg == nil || rc.cfg.Entry == nil {
		return false
	}
	
	// 从入口开始 DFS
	visited := make(map[int]bool)
	return rc.dfsCheckReturn(rc.cfg.Entry, visited)
}

// dfsCheckReturn DFS 检查返回值
func (rc *ReturnChecker) dfsCheckReturn(block *BasicBlock, visited map[int]bool) bool {
	if block == nil {
		return false
	}
	
	// 避免循环
	if visited[block.ID] {
		return true // 已访问过的路径假设为 true（避免无限循环）
	}
	visited[block.ID] = true
	
	// 如果当前块有 return，这条路径有返回
	if block.HasReturn {
		return true
	}
	
	// 如果到达出口块但没有 return，这条路径没有返回
	if block == rc.cfg.Exit {
		return false
	}
	
	// 如果没有后继（死路），认为没有返回
	if len(block.Successors) == 0 {
		return false
	}
	
	// 所有后继路径都必须有返回
	for _, succ := range block.Successors {
		if !rc.dfsCheckReturn(succ, visited) {
			return false
		}
	}
	
	return true
}

// CheckReturnTypes 检查返回值类型（预留接口）
func (rc *ReturnChecker) CheckReturnTypes() []TypeError {
	// 这个功能已经在 TypeChecker 中实现
	return rc.errors
}

// addError 添加错误
func (rc *ReturnChecker) addError(pos token.Position, code, message string) {
	rc.errors = append(rc.errors, TypeError{
		Pos:     pos,
		Code:    code,
		Message: message,
	})
}

// GetErrors 获取错误列表
func (rc *ReturnChecker) GetErrors() []TypeError {
	return rc.errors
}

// UninitializedChecker 未初始化变量检查器
type UninitializedChecker struct {
	cfg    *CFG
	errors []TypeError
}

// NewUninitializedChecker 创建未初始化变量检查器
func NewUninitializedChecker(cfg *CFG) *UninitializedChecker {
	return &UninitializedChecker{
		cfg:    cfg,
		errors: make([]TypeError, 0),
	}
}

// Check 检查未初始化变量使用
func (uc *UninitializedChecker) Check() {
	if uc.cfg == nil || len(uc.cfg.Blocks) == 0 {
		return
	}
	
	// 前向数据流分析
	// Gen: 变量定义
	// Kill: 无
	// In[B] = ∩ Out[P] for all predecessors P
	// Out[B] = Gen[B] ∪ In[B]
	
	// 初始化
	for _, block := range uc.cfg.Blocks {
		block.VarsLiveIn = make(map[string]bool)
		block.VarsLiveOut = make(map[string]bool)
	}
	
	// 迭代直到不动点
	changed := true
	maxIterations := 100 // 防止无限循环
	iteration := 0
	
	for changed && iteration < maxIterations {
		changed = false
		iteration++
		
		for _, block := range uc.cfg.Blocks {
			oldIn := copySet(block.VarsLiveIn)
			
			// 计算 In: 所有前驱的 Out 的交集
			if len(block.Predecessors) > 0 {
				block.VarsLiveIn = uc.intersectOuts(block.Predecessors)
			} else {
				// 入口块，所有变量都未初始化
				block.VarsLiveIn = make(map[string]bool)
			}
			
			// 检查使用的变量是否在 In 中
			for varName := range block.VarsUsed {
				// 如果变量在本块定义，不检查
				if block.VarsDefined[varName] {
					continue
				}
				
				// 如果变量不在 In 中，说明可能未初始化
				if !block.VarsLiveIn[varName] {
					uc.addError(varName, block)
				}
			}
			
			// 计算 Out: Gen ∪ In
			block.VarsLiveOut = uc.union(block.VarsDefined, block.VarsLiveIn)
			
			if !equalSet(oldIn, block.VarsLiveIn) {
				changed = true
			}
		}
	}
}

// intersectOuts 计算所有前驱的 Out 的交集
func (uc *UninitializedChecker) intersectOuts(predecessors []*BasicBlock) map[string]bool {
	if len(predecessors) == 0 {
		return make(map[string]bool)
	}
	
	// 从第一个前驱开始
	result := copySet(predecessors[0].VarsLiveOut)
	
	// 与其他前驱求交集
	for i := 1; i < len(predecessors); i++ {
		result = intersect(result, predecessors[i].VarsLiveOut)
	}
	
	return result
}

// union 计算两个集合的并集
func (uc *UninitializedChecker) union(a, b map[string]bool) map[string]bool {
	result := make(map[string]bool)
	for k := range a {
		result[k] = true
	}
	for k := range b {
		result[k] = true
	}
	return result
}

// addError 添加错误
func (uc *UninitializedChecker) addError(varName string, block *BasicBlock) {
	// 找到第一个使用该变量的语句
	var pos token.Position
	if len(block.Statements) > 0 {
		pos = block.Statements[0].Pos()
	}
	
	uc.errors = append(uc.errors, TypeError{
		Pos:     pos,
		Code:    "compiler.uninitialized_variable",
		Message: fmt.Sprintf("variable '%s' may not have been initialized", varName),
	})
}

// UnreachableChecker 不可达代码检测器
type UnreachableChecker struct {
	cfg      *CFG
	warnings []TypeWarning
}

// NewUnreachableChecker 创建不可达代码检测器
func NewUnreachableChecker(cfg *CFG) *UnreachableChecker {
	return &UnreachableChecker{
		cfg:      cfg,
		warnings: make([]TypeWarning, 0),
	}
}

// Check 检测不可达代码
func (urc *UnreachableChecker) Check() {
	if urc.cfg == nil || urc.cfg.Entry == nil {
		return
	}
	
	// 从入口开始标记可达块
	reachable := make(map[int]bool)
	urc.markReachable(urc.cfg.Entry, reachable)
	
	// 检查不可达块
	for _, block := range urc.cfg.Blocks {
		if !reachable[block.ID] && len(block.Statements) > 0 {
			// 跳过入口和出口块
			if block == urc.cfg.Entry || block == urc.cfg.Exit {
				continue
			}
			
			urc.addWarning(block.Statements[0].Pos(), "compiler.unreachable_code",
				"unreachable code detected")
		}
	}
}

// markReachable 标记可达块
func (urc *UnreachableChecker) markReachable(block *BasicBlock, reachable map[int]bool) {
	if block == nil || reachable[block.ID] {
		return
	}
	
	reachable[block.ID] = true
	
	for _, succ := range block.Successors {
		urc.markReachable(succ, reachable)
	}
}

// addWarning 添加警告
func (urc *UnreachableChecker) addWarning(pos token.Position, code, message string) {
	urc.warnings = append(urc.warnings, TypeWarning{
		Pos:     pos,
		Code:    code,
		Message: message,
	})
}

// Helper functions

// copySet 复制集合
func copySet(src map[string]bool) map[string]bool {
	dst := make(map[string]bool)
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// equalSet 比较两个集合是否相等
func equalSet(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

// intersect 计算两个集合的交集
func intersect(a, b map[string]bool) map[string]bool {
	result := make(map[string]bool)
	for k := range a {
		if b[k] {
			result[k] = true
		}
	}
	return result
}

// union 计算两个集合的并集
func union(a, b map[string]bool) map[string]bool {
	result := make(map[string]bool)
	for k := range a {
		result[k] = true
	}
	for k := range b {
		result[k] = true
	}
	return result
}

