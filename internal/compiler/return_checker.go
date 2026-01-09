package compiler

import (
	"fmt"

	"github.com/tangzhangming/nova/internal/ast"
	"github.com/tangzhangming/nova/internal/i18n"
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
// 使用前向数据流分析检测可能未初始化就使用的变量
type UninitializedChecker struct {
	cfg       *CFG
	errors    []TypeError
	reported  map[string]bool // 已报告的错误（避免重复）
}

// NewUninitializedChecker 创建未初始化变量检查器
func NewUninitializedChecker(cfg *CFG) *UninitializedChecker {
	return &UninitializedChecker{
		cfg:      cfg,
		errors:   make([]TypeError, 0),
		reported: make(map[string]bool),
	}
}

// Check 检查未初始化变量使用
// 分两个阶段：
// 1. 数据流分析：计算每个块入口处"确定已初始化"的变量集合
// 2. 块内检查：按语句顺序检查变量使用是否在定义之前
func (uc *UninitializedChecker) Check() {
	if uc.cfg == nil || len(uc.cfg.Blocks) == 0 {
		return
	}
	
	// 阶段1：前向数据流分析
	// In[B] = ∩ Out[P] for all predecessors P (所有前驱 Out 的交集)
	// Out[B] = Gen[B] ∪ In[B] (本块定义 ∪ 入口已定义)
	uc.computeDataFlow()
	
	// 阶段2：检查每个块内的变量使用
	uc.checkAllBlocks()
}

// computeDataFlow 执行前向数据流分析
func (uc *UninitializedChecker) computeDataFlow() {
	// 初始化：入口块的 In 为空（只包含函数参数，由调用方设置），其他块待计算
	for _, block := range uc.cfg.Blocks {
		if block == uc.cfg.Entry {
			// 入口块的 VarsLiveIn 已由 TypeChecker 设置（包含函数参数）
			if block.VarsLiveIn == nil {
				block.VarsLiveIn = make(map[string]bool)
			}
			// 入口块的 VarsLiveOut = VarsLiveIn ∪ VarsDefined
			block.VarsLiveOut = make(map[string]bool)
			for v := range block.VarsLiveIn {
				block.VarsLiveOut[v] = true
			}
			for v := range block.VarsDefined {
				block.VarsLiveOut[v] = true
			}
		} else {
			block.VarsLiveIn = nil // 标记为未计算
			block.VarsLiveOut = make(map[string]bool)
		}
	}
	
	// 迭代直到不动点
	changed := true
	maxIterations := 100
	iteration := 0
	
	// 保存入口块的函数参数（在迭代前）
	var entryParams map[string]bool
	if uc.cfg.Entry != nil && uc.cfg.Entry.VarsLiveIn != nil {
		entryParams = copySet(uc.cfg.Entry.VarsLiveIn)
	} else {
		entryParams = make(map[string]bool)
	}
	
	for changed && iteration < maxIterations {
		changed = false
		iteration++
		
		for _, block := range uc.cfg.Blocks {
			// 计算 In
			var newIn map[string]bool
			if block == uc.cfg.Entry {
				// 入口块：函数参数始终可用
				// 如果有前驱（循环回边），与前驱的 Out 合并
				if len(block.Predecessors) > 0 {
					predOuts := uc.unionOuts(block.Predecessors)
					newIn = uc.union(entryParams, predOuts)
				} else {
					newIn = copySet(entryParams)
				}
			} else if len(block.Predecessors) > 0 {
				// 对于普通块，使用前驱 Out 的并集
				// 这是保守分析：只要变量在任一路径上被定义，就认为它已定义
				newIn = uc.unionOuts(block.Predecessors)
			} else {
				// 无前驱的非入口块（不可达代码），假设没有已初始化变量
				newIn = make(map[string]bool)
			}
			
			// 计算 Out: 块内定义 ∪ 入口已定义
			newOut := uc.union(block.VarsDefined, newIn)
			
			// 检查是否有变化
			if block.VarsLiveIn == nil || !equalSet(block.VarsLiveIn, newIn) {
				block.VarsLiveIn = newIn
				changed = true
			}
			if !equalSet(block.VarsLiveOut, newOut) {
				block.VarsLiveOut = newOut
				changed = true
			}
		}
	}
}

// checkAllBlocks 检查所有块内的变量使用
func (uc *UninitializedChecker) checkAllBlocks() {
	for _, block := range uc.cfg.Blocks {
		uc.checkBlockStatements(block)
	}
}

// checkBlockStatements 按语句顺序检查块内的变量使用
// 关键：先检查使用，再更新定义，保证正确检测"使用在定义之前"的情况
func (uc *UninitializedChecker) checkBlockStatements(block *BasicBlock) {
	// 从块入口的已定义变量集合开始
	defined := copySet(block.VarsLiveIn)
	
	for _, stmt := range block.Statements {
		// 先收集并检查此语句使用的变量
		usedVars := uc.collectStmtUsedVars(stmt)
		for varName, pos := range usedVars {
			if !defined[varName] {
				uc.reportError(varName, pos)
			}
		}
		
		// 再更新此语句定义的变量
		definedVars := uc.collectStmtDefinedVars(stmt)
		for varName := range definedVars {
			defined[varName] = true
		}
	}
}

// collectStmtUsedVars 收集语句中使用的变量及其位置
func (uc *UninitializedChecker) collectStmtUsedVars(stmt ast.Statement) map[string]token.Position {
	result := make(map[string]token.Position)
	
	switch s := stmt.(type) {
	case *ast.VarDeclStmt:
		// 变量声明：初值表达式中的变量
		if s.Value != nil {
			uc.collectExprUsedVars(s.Value, result)
		}
		
	case *ast.MultiVarDeclStmt:
		// 多变量声明：值表达式中的变量
		uc.collectExprUsedVars(s.Value, result)
		
	case *ast.ExprStmt:
		uc.collectExprUsedVars(s.Expr, result)
		
	case *ast.ReturnStmt:
		for _, val := range s.Values {
			uc.collectExprUsedVars(val, result)
		}
		
	case *ast.EchoStmt:
		uc.collectExprUsedVars(s.Value, result)
		
	case *ast.ThrowStmt:
		uc.collectExprUsedVars(s.Exception, result)
	}
	
	return result
}

// collectExprUsedVars 收集表达式中使用的变量
func (uc *UninitializedChecker) collectExprUsedVars(expr ast.Expression, result map[string]token.Position) {
	if expr == nil {
		return
	}
	
	switch e := expr.(type) {
	case *ast.Variable:
		// 记录变量名和位置
		if _, exists := result[e.Name]; !exists {
			result[e.Name] = e.Pos()
		}
		
	case *ast.BinaryExpr:
		uc.collectExprUsedVars(e.Left, result)
		uc.collectExprUsedVars(e.Right, result)
		
	case *ast.UnaryExpr:
		uc.collectExprUsedVars(e.Operand, result)
		
	case *ast.AssignExpr:
		// 赋值表达式：先检查右侧，再检查左侧（如果是复合赋值）
		uc.collectExprUsedVars(e.Right, result)
		// 对于简单赋值 $x = expr，左侧变量是被定义而非使用
		// 对于复合赋值 $x += expr，左侧变量既被使用又被定义
		// 这里只处理复合赋值的情况
		if e.Operator.Type != token.ASSIGN {
			uc.collectExprUsedVars(e.Left, result)
		}
		
	case *ast.CallExpr:
		uc.collectExprUsedVars(e.Function, result)
		for _, arg := range e.Arguments {
			uc.collectExprUsedVars(arg, result)
		}
		
	case *ast.PropertyAccess:
		uc.collectExprUsedVars(e.Object, result)
		
	case *ast.MethodCall:
		uc.collectExprUsedVars(e.Object, result)
		for _, arg := range e.Arguments {
			uc.collectExprUsedVars(arg, result)
		}
		
	case *ast.IndexExpr:
		uc.collectExprUsedVars(e.Object, result)
		uc.collectExprUsedVars(e.Index, result)
		
	case *ast.ArrayLiteral:
		for _, elem := range e.Elements {
			uc.collectExprUsedVars(elem, result)
		}
		
	case *ast.MapLiteral:
		for _, pair := range e.Pairs {
			uc.collectExprUsedVars(pair.Key, result)
			uc.collectExprUsedVars(pair.Value, result)
		}
		
	case *ast.NewExpr:
		for _, arg := range e.Arguments {
			uc.collectExprUsedVars(arg, result)
		}
		
	case *ast.TernaryExpr:
		uc.collectExprUsedVars(e.Condition, result)
		uc.collectExprUsedVars(e.Then, result)
		uc.collectExprUsedVars(e.Else, result)
		
	case *ast.IsExpr:
		uc.collectExprUsedVars(e.Expr, result)
		
	case *ast.TypeCastExpr:
		uc.collectExprUsedVars(e.Expr, result)
		
	case *ast.NullCoalesceExpr:
		uc.collectExprUsedVars(e.Left, result)
		uc.collectExprUsedVars(e.Right, result)
		
	case *ast.SafePropertyAccess:
		uc.collectExprUsedVars(e.Object, result)
		
	case *ast.SafeMethodCall:
		uc.collectExprUsedVars(e.Object, result)
		for _, arg := range e.Arguments {
			uc.collectExprUsedVars(arg, result)
		}
	}
}

// collectStmtDefinedVars 收集语句中定义的变量
func (uc *UninitializedChecker) collectStmtDefinedVars(stmt ast.Statement) map[string]bool {
	result := make(map[string]bool)
	
	switch s := stmt.(type) {
	case *ast.VarDeclStmt:
		// 变量声明：只有当有初值时才算定义
		if s.Value != nil {
			result[s.Name.Name] = true
		}
		
	case *ast.MultiVarDeclStmt:
		// 多变量声明：所有变量都被定义
		for _, name := range s.Names {
			result[name.Name] = true
		}
		
	case *ast.ExprStmt:
		// 赋值表达式中的定义
		uc.collectExprDefinedVars(s.Expr, result)
	}
	
	return result
}

// collectExprDefinedVars 收集表达式中定义的变量
func (uc *UninitializedChecker) collectExprDefinedVars(expr ast.Expression, result map[string]bool) {
	if expr == nil {
		return
	}
	
	switch e := expr.(type) {
	case *ast.AssignExpr:
		// 赋值表达式：左侧变量被定义
		if v, ok := e.Left.(*ast.Variable); ok {
			result[v.Name] = true
		}
		// 右侧可能也有赋值表达式（链式赋值）
		uc.collectExprDefinedVars(e.Right, result)
	}
}

// unionOuts 计算所有前驱的 Out 的并集
// 对于变量初始化检查，使用并集是正确的：只要变量在任一路径上被定义，
// 就应该认为它在合并点是已定义的（保守分析）
func (uc *UninitializedChecker) unionOuts(predecessors []*BasicBlock) map[string]bool {
	result := make(map[string]bool)
	
	for _, pred := range predecessors {
		if pred.VarsLiveOut != nil {
			for v := range pred.VarsLiveOut {
				result[v] = true
			}
		}
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

// reportError 报告错误（避免重复）
func (uc *UninitializedChecker) reportError(varName string, pos token.Position) {
	// 使用 位置+变量名 作为唯一键避免重复报告
	key := fmt.Sprintf("%s:%d:%d:%s", pos.Filename, pos.Line, pos.Column, varName)
	if uc.reported[key] {
		return
	}
	uc.reported[key] = true
	
	uc.errors = append(uc.errors, TypeError{
		Pos:     pos,
		Code:    i18n.WarnUninitializedVariable,
		Message: i18n.T(i18n.WarnUninitializedVariable, varName),
	})
}

// GetErrors 获取错误列表
func (uc *UninitializedChecker) GetErrors() []TypeError {
	return uc.errors
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

