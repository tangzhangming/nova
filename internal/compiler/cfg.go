package compiler

import (
	"github.com/tangzhangming/nova/internal/ast"
)

// BasicBlock 基本块
type BasicBlock struct {
	ID           int
	Statements   []ast.Statement
	Predecessors []*BasicBlock
	Successors   []*BasicBlock
	
	// 数据流信息
	VarsDefined  map[string]bool // 定义的变量
	VarsUsed     map[string]bool // 使用的变量
	VarsLiveIn   map[string]bool // 入口活跃变量
	VarsLiveOut  map[string]bool // 出口活跃变量
	
	// 返回信息
	HasReturn    bool
	ReturnType   string
}

// NewBasicBlock 创建新的基本块
func NewBasicBlock(id int) *BasicBlock {
	return &BasicBlock{
		ID:           id,
		Statements:   make([]ast.Statement, 0),
		Predecessors: make([]*BasicBlock, 0),
		Successors:   make([]*BasicBlock, 0),
		VarsDefined:  make(map[string]bool),
		VarsUsed:     make(map[string]bool),
		VarsLiveIn:   make(map[string]bool),
		VarsLiveOut:  make(map[string]bool),
		HasReturn:    false,
	}
}

// AddStatement 添加语句到基本块
func (bb *BasicBlock) AddStatement(stmt ast.Statement) {
	bb.Statements = append(bb.Statements, stmt)
	
	// 更新数据流信息
	bb.updateDataFlow(stmt)
}

// updateDataFlow 更新数据流信息
func (bb *BasicBlock) updateDataFlow(stmt ast.Statement) {
	switch s := stmt.(type) {
	case *ast.VarDeclStmt:
		bb.VarsDefined[s.Name.Name] = true
		if s.Value != nil {
			bb.collectUsedVars(s.Value)
		}
		
	case *ast.MultiVarDeclStmt:
		for _, name := range s.Names {
			bb.VarsDefined[name.Name] = true
		}
		bb.collectUsedVars(s.Value)
		
	case *ast.ExprStmt:
		bb.collectUsedVars(s.Expr)
		
	case *ast.ReturnStmt:
		bb.HasReturn = true
		for _, val := range s.Values {
			bb.collectUsedVars(val)
		}
	}
}

// collectUsedVars 收集表达式中使用的变量
func (bb *BasicBlock) collectUsedVars(expr ast.Expression) {
	if expr == nil {
		return
	}
	
	switch e := expr.(type) {
	case *ast.Variable:
		bb.VarsUsed[e.Name] = true
		
	case *ast.BinaryExpr:
		bb.collectUsedVars(e.Left)
		bb.collectUsedVars(e.Right)
		
	case *ast.UnaryExpr:
		bb.collectUsedVars(e.Operand)
		
	case *ast.AssignExpr:
		bb.collectUsedVars(e.Left)
		bb.collectUsedVars(e.Right)
		// 赋值左侧也算定义
		if v, ok := e.Left.(*ast.Variable); ok {
			bb.VarsDefined[v.Name] = true
		}
		
	case *ast.CallExpr:
		bb.collectUsedVars(e.Function)
		for _, arg := range e.Arguments {
			bb.collectUsedVars(arg)
		}
		
	case *ast.PropertyAccess:
		bb.collectUsedVars(e.Object)
		
	case *ast.MethodCall:
		bb.collectUsedVars(e.Object)
		for _, arg := range e.Arguments {
			bb.collectUsedVars(arg)
		}
		
	case *ast.IndexExpr:
		bb.collectUsedVars(e.Object)
		bb.collectUsedVars(e.Index)
		
	case *ast.ArrayLiteral:
		for _, elem := range e.Elements {
			bb.collectUsedVars(elem)
		}
		
	case *ast.MapLiteral:
		for _, pair := range e.Pairs {
			bb.collectUsedVars(pair.Key)
			bb.collectUsedVars(pair.Value)
		}
		
	case *ast.NewExpr:
		for _, arg := range e.Arguments {
			bb.collectUsedVars(arg)
		}
		
	case *ast.TernaryExpr:
		bb.collectUsedVars(e.Condition)
		bb.collectUsedVars(e.Then)
		bb.collectUsedVars(e.Else)
		
	case *ast.IsExpr:
		bb.collectUsedVars(e.Expr)
		
	case *ast.TypeCastExpr:
		bb.collectUsedVars(e.Expr)
	}
}

// AddSuccessor 添加后继块
func (bb *BasicBlock) AddSuccessor(succ *BasicBlock) {
	bb.Successors = append(bb.Successors, succ)
	succ.Predecessors = append(succ.Predecessors, bb)
}

// CFG 控制流图
type CFG struct {
	Entry   *BasicBlock
	Exit    *BasicBlock
	Blocks  []*BasicBlock
	blockID int
}

// NewCFG 创建新的控制流图
func NewCFG() *CFG {
	cfg := &CFG{
		Blocks:  make([]*BasicBlock, 0),
		blockID: 0,
	}
	cfg.Entry = cfg.NewBlock()
	cfg.Exit = cfg.NewBlock()
	return cfg
}

// NewBlock 创建新的基本块
func (cfg *CFG) NewBlock() *BasicBlock {
	block := NewBasicBlock(cfg.blockID)
	cfg.blockID++
	cfg.Blocks = append(cfg.Blocks, block)
	return block
}

// CFGBuilder CFG 构建器
type CFGBuilder struct {
	cfg          *CFG
	currentBlock *BasicBlock
	
	// 循环上下文（用于 break/continue）
	loopStack    []*loopContext
}

// loopContext 循环上下文
type loopContext struct {
	continueTarget *BasicBlock // continue 跳转目标
	breakTarget    *BasicBlock // break 跳转目标
}

// NewCFGBuilder 创建 CFG 构建器
func NewCFGBuilder() *CFGBuilder {
	return &CFGBuilder{
		loopStack: make([]*loopContext, 0),
	}
}

// Build 构建控制流图
func (cb *CFGBuilder) Build(stmt ast.Statement) *CFG {
	cb.cfg = NewCFG()
	cb.currentBlock = cb.cfg.Entry
	
	cb.buildStatement(stmt)
	
	// 连接到出口
	if cb.currentBlock != nil {
		cb.currentBlock.AddSuccessor(cb.cfg.Exit)
	}
	
	return cb.cfg
}

// buildStatement 构建语句的 CFG
func (cb *CFGBuilder) buildStatement(stmt ast.Statement) {
	if cb.currentBlock == nil {
		return
	}
	
	switch s := stmt.(type) {
	case *ast.BlockStmt:
		cb.buildBlockStmt(s)
		
	case *ast.IfStmt:
		cb.buildIfStmt(s)
		
	case *ast.WhileStmt:
		cb.buildWhileStmt(s)
		
	case *ast.DoWhileStmt:
		cb.buildDoWhileStmt(s)
		
	case *ast.ForStmt:
		cb.buildForStmt(s)
		
	case *ast.ForeachStmt:
		cb.buildForeachStmt(s)
		
	case *ast.SwitchStmt:
		cb.buildSwitchStmt(s)
		
	case *ast.ReturnStmt:
		cb.currentBlock.AddStatement(s)
		cb.currentBlock.HasReturn = true
		// return 后创建新块（不可达代码）
		cb.currentBlock = cb.cfg.NewBlock()
		
	case *ast.BreakStmt:
		if len(cb.loopStack) > 0 {
			ctx := cb.loopStack[len(cb.loopStack)-1]
			cb.currentBlock.AddSuccessor(ctx.breakTarget)
		}
		cb.currentBlock = cb.cfg.NewBlock()
		
	case *ast.ContinueStmt:
		if len(cb.loopStack) > 0 {
			ctx := cb.loopStack[len(cb.loopStack)-1]
			cb.currentBlock.AddSuccessor(ctx.continueTarget)
		}
		cb.currentBlock = cb.cfg.NewBlock()
		
	case *ast.TryStmt:
		cb.buildTryStmt(s)
		
	default:
		// 简单语句直接添加到当前块
		cb.currentBlock.AddStatement(s)
	}
}

// buildBlockStmt 构建块语句
func (cb *CFGBuilder) buildBlockStmt(stmt *ast.BlockStmt) {
	for _, s := range stmt.Statements {
		cb.buildStatement(s)
	}
}

// buildIfStmt 构建 if 语句
func (cb *CFGBuilder) buildIfStmt(stmt *ast.IfStmt) {
	// 条件块
	condBlock := cb.currentBlock
	condBlock.AddStatement(&ast.ExprStmt{Expr: stmt.Condition})
	
	// then 分支
	thenBlock := cb.cfg.NewBlock()
	condBlock.AddSuccessor(thenBlock)
	cb.currentBlock = thenBlock
	cb.buildStatement(stmt.Then)
	thenExit := cb.currentBlock
	
	// else 分支
	var elseExit *BasicBlock
	if stmt.Else != nil {
		elseBlock := cb.cfg.NewBlock()
		condBlock.AddSuccessor(elseBlock)
		cb.currentBlock = elseBlock
		cb.buildStatement(stmt.Else)
		elseExit = cb.currentBlock
	} else if len(stmt.ElseIfs) > 0 {
		// elseif 分支
		elseIfEntry := cb.cfg.NewBlock()
		condBlock.AddSuccessor(elseIfEntry)
		cb.currentBlock = elseIfEntry
		
		for _, elseIf := range stmt.ElseIfs {
			elseIfCond := cb.currentBlock
			elseIfCond.AddStatement(&ast.ExprStmt{Expr: elseIf.Condition})
			
			elseIfThen := cb.cfg.NewBlock()
			elseIfCond.AddSuccessor(elseIfThen)
			cb.currentBlock = elseIfThen
			cb.buildStatement(elseIf.Body)
			
			// 下一个 elseif 或 else
			nextBlock := cb.cfg.NewBlock()
			elseIfCond.AddSuccessor(nextBlock)
			cb.currentBlock = nextBlock
		}
		
		elseExit = cb.currentBlock
	} else {
		// 没有 else，条件块直接连到合并点
		elseExit = condBlock
	}
	
	// 合并点
	mergeBlock := cb.cfg.NewBlock()
	if thenExit != nil {
		thenExit.AddSuccessor(mergeBlock)
	}
	if elseExit != nil && elseExit != condBlock {
		elseExit.AddSuccessor(mergeBlock)
	} else if elseExit == condBlock {
		condBlock.AddSuccessor(mergeBlock)
	}
	
	cb.currentBlock = mergeBlock
}

// buildWhileStmt 构建 while 语句
func (cb *CFGBuilder) buildWhileStmt(stmt *ast.WhileStmt) {
	// 循环头（条件）
	loopHead := cb.cfg.NewBlock()
	cb.currentBlock.AddSuccessor(loopHead)
	loopHead.AddStatement(&ast.ExprStmt{Expr: stmt.Condition})
	
	// 循环体
	loopBody := cb.cfg.NewBlock()
	loopHead.AddSuccessor(loopBody)
	
	// 循环出口
	loopExit := cb.cfg.NewBlock()
	loopHead.AddSuccessor(loopExit)
	
	// 构建循环体
	ctx := &loopContext{
		continueTarget: loopHead,
		breakTarget:    loopExit,
	}
	cb.loopStack = append(cb.loopStack, ctx)
	
	cb.currentBlock = loopBody
	cb.buildStatement(stmt.Body)
	
	// 循环体结束后跳回循环头
	if cb.currentBlock != nil {
		cb.currentBlock.AddSuccessor(loopHead)
	}
	
	cb.loopStack = cb.loopStack[:len(cb.loopStack)-1]
	cb.currentBlock = loopExit
}

// buildDoWhileStmt 构建 do-while 语句
func (cb *CFGBuilder) buildDoWhileStmt(stmt *ast.DoWhileStmt) {
	// 循环体
	loopBody := cb.cfg.NewBlock()
	cb.currentBlock.AddSuccessor(loopBody)
	
	// 条件块
	condBlock := cb.cfg.NewBlock()
	
	// 循环出口
	loopExit := cb.cfg.NewBlock()
	
	// 构建循环体
	ctx := &loopContext{
		continueTarget: condBlock,
		breakTarget:    loopExit,
	}
	cb.loopStack = append(cb.loopStack, ctx)
	
	cb.currentBlock = loopBody
	cb.buildStatement(stmt.Body)
	
	// 循环体结束后到条件块
	if cb.currentBlock != nil {
		cb.currentBlock.AddSuccessor(condBlock)
	}
	
	// 条件块
	condBlock.AddStatement(&ast.ExprStmt{Expr: stmt.Condition})
	condBlock.AddSuccessor(loopBody) // true 回到循环体
	condBlock.AddSuccessor(loopExit) // false 退出
	
	cb.loopStack = cb.loopStack[:len(cb.loopStack)-1]
	cb.currentBlock = loopExit
}

// buildForStmt 构建 for 语句
func (cb *CFGBuilder) buildForStmt(stmt *ast.ForStmt) {
	// 初始化
	if stmt.Init != nil {
		cb.buildStatement(stmt.Init)
	}
	
	// 循环头（条件）
	loopHead := cb.cfg.NewBlock()
	cb.currentBlock.AddSuccessor(loopHead)
	if stmt.Condition != nil {
		loopHead.AddStatement(&ast.ExprStmt{Expr: stmt.Condition})
	}
	
	// 循环体
	loopBody := cb.cfg.NewBlock()
	loopHead.AddSuccessor(loopBody)
	
	// Post 块
	postBlock := cb.cfg.NewBlock()
	
	// 循环出口
	loopExit := cb.cfg.NewBlock()
	loopHead.AddSuccessor(loopExit)
	
	// 构建循环体
	ctx := &loopContext{
		continueTarget: postBlock,
		breakTarget:    loopExit,
	}
	cb.loopStack = append(cb.loopStack, ctx)
	
	cb.currentBlock = loopBody
	cb.buildStatement(stmt.Body)
	
	// 循环体结束后到 post 块
	if cb.currentBlock != nil {
		cb.currentBlock.AddSuccessor(postBlock)
	}
	
	// Post 块
	if stmt.Post != nil {
		postBlock.AddStatement(&ast.ExprStmt{Expr: stmt.Post})
	}
	postBlock.AddSuccessor(loopHead)
	
	cb.loopStack = cb.loopStack[:len(cb.loopStack)-1]
	cb.currentBlock = loopExit
}

// buildForeachStmt 构建 foreach 语句
func (cb *CFGBuilder) buildForeachStmt(stmt *ast.ForeachStmt) {
	// 简化处理：foreach 类似 while
	loopHead := cb.cfg.NewBlock()
	cb.currentBlock.AddSuccessor(loopHead)
	
	loopBody := cb.cfg.NewBlock()
	loopHead.AddSuccessor(loopBody)
	
	loopExit := cb.cfg.NewBlock()
	loopHead.AddSuccessor(loopExit)
	
	ctx := &loopContext{
		continueTarget: loopHead,
		breakTarget:    loopExit,
	}
	cb.loopStack = append(cb.loopStack, ctx)
	
	cb.currentBlock = loopBody
	cb.buildStatement(stmt.Body)
	
	if cb.currentBlock != nil {
		cb.currentBlock.AddSuccessor(loopHead)
	}
	
	cb.loopStack = cb.loopStack[:len(cb.loopStack)-1]
	cb.currentBlock = loopExit
}

// buildSwitchStmt 构建 switch 语句
func (cb *CFGBuilder) buildSwitchStmt(stmt *ast.SwitchStmt) {
	// switch 表达式
	switchBlock := cb.currentBlock
	switchBlock.AddStatement(&ast.ExprStmt{Expr: stmt.Expr})
	
	// 合并点
	mergeBlock := cb.cfg.NewBlock()
	
	// 每个 case
	for _, caseClause := range stmt.Cases {
		caseBlock := cb.cfg.NewBlock()
		switchBlock.AddSuccessor(caseBlock)
		
		cb.currentBlock = caseBlock
		// Body 可能是 []Statement 或 Expression
		if stmts, ok := caseClause.Body.([]ast.Statement); ok {
			for _, s := range stmts {
				cb.buildStatement(s)
			}
		}
		
		// case 结束后到合并点
		if cb.currentBlock != nil {
			cb.currentBlock.AddSuccessor(mergeBlock)
		}
	}
	
	// default
	if stmt.Default != nil {
		defaultBlock := cb.cfg.NewBlock()
		switchBlock.AddSuccessor(defaultBlock)
		
		cb.currentBlock = defaultBlock
		// Body 可能是 []Statement 或 Expression
		if stmts, ok := stmt.Default.Body.([]ast.Statement); ok {
			for _, s := range stmts {
				cb.buildStatement(s)
			}
		}
		
		if cb.currentBlock != nil {
			cb.currentBlock.AddSuccessor(mergeBlock)
		}
	} else {
		// 没有 default，switch 直接到合并点
		switchBlock.AddSuccessor(mergeBlock)
	}
	
	cb.currentBlock = mergeBlock
}

// buildTryStmt 构建 try 语句
func (cb *CFGBuilder) buildTryStmt(stmt *ast.TryStmt) {
	// try 块
	tryBlock := cb.currentBlock
	cb.buildStatement(stmt.Try)
	tryExit := cb.currentBlock
	
	// catch 块
	var catchExits []*BasicBlock
	for _, catchClause := range stmt.Catches {
		catchBlock := cb.cfg.NewBlock()
		tryBlock.AddSuccessor(catchBlock) // try 可能跳到任何 catch
		
		cb.currentBlock = catchBlock
		cb.buildStatement(catchClause.Body)
		catchExits = append(catchExits, cb.currentBlock)
	}
	
	// finally 块
	var finallyExit *BasicBlock
	if stmt.Finally != nil {
		finallyBlock := cb.cfg.NewBlock()
		
		// try 和所有 catch 都到 finally
		if tryExit != nil {
			tryExit.AddSuccessor(finallyBlock)
		}
		for _, catchExit := range catchExits {
			if catchExit != nil {
				catchExit.AddSuccessor(finallyBlock)
			}
		}
		
		cb.currentBlock = finallyBlock
		cb.buildStatement(stmt.Finally.Body)
		finallyExit = cb.currentBlock
	} else {
		finallyExit = tryExit
	}
	
	// 合并点
	mergeBlock := cb.cfg.NewBlock()
	if finallyExit != nil {
		finallyExit.AddSuccessor(mergeBlock)
	}
	for _, catchExit := range catchExits {
		if catchExit != nil && stmt.Finally == nil {
			catchExit.AddSuccessor(mergeBlock)
		}
	}
	
	cb.currentBlock = mergeBlock
}

