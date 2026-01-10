// optimizer.go - IR 优化器
//
// 本文件实现了多个优化 Pass，用于改进 IR 的质量和效率。
//
// 优化级别：
//   O0: 不优化（用于调试）
//   O1: 基本优化 - 常量传播、死代码消除
//   O2: 标准优化 - 增加代数简化、强度削减、CSE、复制传播、窥孔优化
//   O3: 激进优化 - 增加内联、边界检查消除、循环优化、GVN
//
// 优化 Pass (按执行顺序)：
//
// O1 基本优化：
// 1. ConstantPropagation - 常量传播和常量折叠
// 2. DeadCodeElimination - 死代码消除
//
// O2 标准优化：
// 3. AlgebraicSimplification - 代数简化
// 4. StrengthReduction - 强度削减（用移位代替乘除）
// 5. CommonSubexpressionElimination - 公共子表达式消除
// 6. CopyPropagation - 复制传播
// 7. ConditionalBranchOptimization - 条件分支优化
// 8. PeepholeOptimization - 窥孔优化（双重否定消除、连续移位合并等）
//
// O3 激进优化：
// 9. Inlining - 函数内联（由 inliner.go 实现）
// 10. LoopInvariantCodeMotion - 循环不变量外提
// 11. BoundsCheckElimination - 边界检查消除
// 12. GlobalValueNumbering - 全局值编号
// 13. LoopUnrolling - 循环展开
//
// 优化是迭代进行的，直到没有更多变化为止。

package jit

// ============================================================================
// 优化器
// ============================================================================

// Optimizer IR 优化器
type Optimizer struct {
	level    int            // 优化级别 (0-3)
	inliner  *Inliner       // 内联优化器
	resolver func(string) *IRFunc // 函数解析器
	
	// 优化统计
	stats    OptimizerStats
}

// OptimizerStats 优化器统计信息
type OptimizerStats struct {
	ConstantsFolded       int // 折叠的常量数
	ConstantsPropagated   int // 传播的常量数
	DeadInstructionsRemoved int // 消除的死指令数
	UnreachableBlocksRemoved int // 消除的不可达块数
	TotalIterations       int // 总优化迭代次数
}

// NewOptimizer 创建优化器
func NewOptimizer(level int) *Optimizer {
	if level < 0 {
		level = 0
	}
	if level > 3 {
		level = 3
	}
	opt := &Optimizer{level: level}
	
	// 为 O3 级别创建内联优化器
	if level >= 3 {
		opt.inliner = NewInliner(DefaultInlineConfig())
	}
	
	return opt
}

// SetFunctionResolver 设置函数解析器（用于内联）
func (opt *Optimizer) SetFunctionResolver(resolver func(string) *IRFunc) {
	opt.resolver = resolver
	if opt.inliner != nil {
		opt.inliner.SetResolver(resolver)
	}
}

// GetInlineStats 获取内联统计
func (opt *Optimizer) GetInlineStats() *InlineStats {
	if opt.inliner != nil {
		stats := opt.inliner.GetStats()
		return &stats
	}
	return nil
}

// GetStats 获取优化统计
func (opt *Optimizer) GetStats() OptimizerStats {
	return opt.stats
}

// ResetStats 重置优化统计
func (opt *Optimizer) ResetStats() {
	opt.stats = OptimizerStats{}
}

// Optimize 优化 IR 函数
func (opt *Optimizer) Optimize(fn *IRFunc) {
	if opt.level == 0 {
		return
	}
	
	// O3: 先执行内联（在其他优化之前）
	// 这样内联后的代码可以享受其他优化的好处
	if opt.level >= 3 && opt.inliner != nil {
		opt.inlining(fn)
	}
	
	// 迭代优化直到稳定
	maxIterations := 10
	for i := 0; i < maxIterations; i++ {
		opt.stats.TotalIterations++
		changed := false
		
		// O1: 基本优化
		if opt.level >= 1 {
			changed = opt.constantPropagation(fn) || changed
			changed = opt.deadCodeElimination(fn) || changed
		}
		
		// O2: 标准优化
		if opt.level >= 2 {
			changed = opt.algebraicSimplification(fn) || changed
			changed = opt.strengthReduction(fn) || changed
			// 公共子表达式消除
			changed = opt.CommonSubexpressionElimination(fn) || changed
			// 复制传播
			changed = opt.CopyPropagation(fn) || changed
			// 条件分支优化
			changed = opt.ConditionalBranchOptimization(fn) || changed
			// 窥孔优化
			changed = opt.PeepholeOptimization(fn) || changed
		}
		
		// O3: 高级优化
		if opt.level >= 3 {
			changed = opt.loopInvariantCodeMotion(fn) || changed
			// 边界检查消除
			changed = opt.BoundsCheckElimination(fn) || changed
			// 全局值编号
			changed = opt.GlobalValueNumbering(fn) || changed
			// 循环展开
			changed = opt.LoopUnrolling(fn) || changed
		}
		
		if !changed {
			break
		}
	}
	
	// 最后的清理
	opt.removeNops(fn)
}

// ============================================================================
// O3 优化：内联
// ============================================================================

// inlining 执行函数内联优化
func (opt *Optimizer) inlining(fn *IRFunc) bool {
	if opt.inliner == nil {
		return false
	}
	return opt.inliner.Inline(fn)
}

// ============================================================================
// O3 优化：循环不变量外提 (LICM)
// ============================================================================

// loopInvariantCodeMotion 循环不变量外提
// 将循环内的不变计算移动到循环外
func (opt *Optimizer) loopInvariantCodeMotion(fn *IRFunc) bool {
	changed := false
	
	// 使用 detectLoops 检测循环（来自 optimizations.go）
	loops := opt.detectLoops(fn)
	if len(loops) == 0 {
		return false
	}
	
	// 对每个循环，识别并外提不变量
	for _, loop := range loops {
		// 找到循环的前导块（preheader）
		preheader := opt.findPreheader(loop)
		if preheader == nil {
			continue
		}
		
		// 收集循环内定义的所有值
		loopDefs := make(map[*IRValue]bool)
		for _, block := range loop.Body {
			for _, instr := range block.Instrs {
				if instr.Dest != nil {
					loopDefs[instr.Dest] = true
				}
			}
		}
		
		// 迭代查找并外提不变量
		for {
			moved := false
			for _, block := range loop.Body {
				newInstrs := make([]*IRInstr, 0, len(block.Instrs))
				for _, instr := range block.Instrs {
					// 检查是否为循环不变量
					if opt.isLoopInvariantInstr(instr, loopDefs) && opt.isSafeToHoist(instr) {
						// 移动到前导块（插入到最后一条指令之前）
						if len(preheader.Instrs) > 0 {
							lastIdx := len(preheader.Instrs) - 1
							preheader.Instrs = append(preheader.Instrs[:lastIdx], 
								instr, preheader.Instrs[lastIdx])
						} else {
							preheader.Instrs = append(preheader.Instrs, instr)
						}
						// 从循环定义中移除
						if instr.Dest != nil {
							delete(loopDefs, instr.Dest)
						}
						moved = true
						changed = true
					} else {
						newInstrs = append(newInstrs, instr)
					}
				}
				block.Instrs = newInstrs
			}
			if !moved {
				break
			}
		}
	}
	
	return changed
}

// findPreheader 查找循环的前导块
func (opt *Optimizer) findPreheader(loop *LoopInfo) *IRBlock {
	header := loop.Header
	
	// 检查是否已有唯一的非循环前驱
	var nonLoopPreds []*IRBlock
	loopBlockSet := make(map[int]bool)
	for _, b := range loop.Body {
		loopBlockSet[b.ID] = true
	}
	
	for _, pred := range header.Preds {
		if !loopBlockSet[pred.ID] {
			nonLoopPreds = append(nonLoopPreds, pred)
		}
	}
	
	// 如果只有一个非循环前驱，直接使用它
	if len(nonLoopPreds) == 1 {
		return nonLoopPreds[0]
	}
	
	return nil
}

// isLoopInvariantInstr 检查指令是否为循环不变量
func (opt *Optimizer) isLoopInvariantInstr(instr *IRInstr, loopDefs map[*IRValue]bool) bool {
	// 跳过控制流指令和副作用指令
	switch instr.Op {
	case OpJump, OpBranch, OpReturn, OpCall, OpCallDirect, OpCallIndirect,
		OpCallBuiltin, OpCallMethod, OpCallVirtual, OpTailCall,
		OpStoreLocal, OpSetField, OpArraySet, OpPhi, OpNop:
		return false
	}
	
	// 检查所有操作数是否都是循环不变的
	for _, arg := range instr.Args {
		if arg == nil {
			continue
		}
		// 常量是不变的
		if arg.IsConst {
			continue
		}
		// 如果操作数在循环内定义，则不是不变量
		if loopDefs[arg] {
			return false
		}
	}
	
	return true
}

// isSafeToHoist 检查是否可以安全地外提指令
func (opt *Optimizer) isSafeToHoist(instr *IRInstr) bool {
	// 纯计算指令可以安全外提
	switch instr.Op {
	case OpConst, OpAdd, OpSub, OpMul, OpDiv, OpMod, OpNeg,
		OpEq, OpNe, OpLt, OpLe, OpGt, OpGe,
		OpNot, OpAnd, OpOr,
		OpBitAnd, OpBitOr, OpBitXor, OpBitNot, OpShl, OpShr,
		OpIntToFloat, OpFloatToInt:
		return true
	}
	return false
}

// ============================================================================
// 常量传播
// ============================================================================

// constantPropagation 常量传播和常量折叠
// 如果一个操作的所有操作数都是常量，则可以在编译时计算结果
// 增强版本支持跨基本块的常量传播
func (opt *Optimizer) constantPropagation(fn *IRFunc) bool {
	changed := false
	
	// 第一阶段：块内常量折叠
	for _, block := range fn.Blocks {
		for i, instr := range block.Instrs {
			if instr.Op == OpNop {
				continue
			}
			
			// 尝试常量折叠
			if result := opt.tryFold(instr); result != nil {
				// 将指令替换为常量加载
				newInstr := NewInstr(OpConst, instr.Dest)
				instr.Dest.IsConst = true
				instr.Dest.ConstVal = result.ConstVal
				block.Instrs[i] = newInstr
				opt.stats.ConstantsFolded++
				changed = true
			}
		}
	}
	
	// 第二阶段：跨块常量传播（数据流分析）
	// 构建常量映射表
	constMap := make(map[*IRValue]bool)
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			if instr.Dest != nil && instr.Dest.IsConst {
				constMap[instr.Dest] = true
			}
		}
	}
	
	// 传播常量到所有使用点
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			for i, arg := range instr.Args {
				if arg != nil && arg.IsConst {
					// 直接使用常量值
					instr.Args[i] = arg
					opt.stats.ConstantsPropagated++
				}
			}
		}
	}
	
	// 第三阶段：条件分支优化（当条件是常量时）
	changed = opt.foldConstantBranches(fn) || changed
	
	return changed
}

// foldConstantBranches 折叠常量条件分支
// 当分支条件是常量时，可以确定性地选择一个分支
func (opt *Optimizer) foldConstantBranches(fn *IRFunc) bool {
	changed := false
	
	for _, block := range fn.Blocks {
		lastInstr := block.LastInstr()
		if lastInstr == nil || lastInstr.Op != OpBranch {
			continue
		}
		
		// 检查条件是否为常量
		if len(lastInstr.Args) == 0 || lastInstr.Args[0] == nil {
			continue
		}
		
		cond := lastInstr.Args[0]
		if !cond.IsConst {
			continue
		}
		
		// 确定选择哪个分支
		var targetBlock *IRBlock
		var deadBlock *IRBlock
		
		condValue := cond.ConstVal.AsBool()
		if len(lastInstr.Targets) >= 2 {
			if condValue {
				targetBlock = lastInstr.Targets[0] // then 分支
				deadBlock = lastInstr.Targets[1]   // else 分支
			} else {
				targetBlock = lastInstr.Targets[1] // else 分支
				deadBlock = lastInstr.Targets[0]   // then 分支
			}
			
			// 将条件分支替换为无条件跳转
			lastInstr.Op = OpJump
			lastInstr.Args = nil
			lastInstr.Targets = []*IRBlock{targetBlock}
			
			// 从死分支的前驱列表中移除当前块
			if deadBlock != nil {
				newPreds := make([]*IRBlock, 0, len(deadBlock.Preds))
				for _, pred := range deadBlock.Preds {
					if pred != block {
						newPreds = append(newPreds, pred)
					}
				}
				deadBlock.Preds = newPreds
			}
			
			changed = true
		}
	}
	
	return changed
}

// tryFold 尝试常量折叠
func (opt *Optimizer) tryFold(instr *IRInstr) *IRValue {
	// 检查所有操作数是否都是常量
	for _, arg := range instr.Args {
		if arg == nil || !arg.IsConst {
			return nil
		}
	}
	
	switch instr.Op {
	case OpAdd, OpSub, OpMul, OpDiv, OpMod:
		if len(instr.Args) != 2 {
			return nil
		}
		return opt.foldArithmetic(instr.Op, instr.Args[0], instr.Args[1])
		
	case OpEq, OpNe, OpLt, OpLe, OpGt, OpGe:
		if len(instr.Args) != 2 {
			return nil
		}
		return opt.foldComparison(instr.Op, instr.Args[0], instr.Args[1])
		
	case OpNeg:
		if len(instr.Args) != 1 {
			return nil
		}
		return opt.foldNeg(instr.Args[0])
		
	case OpNot:
		if len(instr.Args) != 1 {
			return nil
		}
		return opt.foldNot(instr.Args[0])
		
	case OpBitAnd, OpBitOr, OpBitXor, OpShl, OpShr:
		if len(instr.Args) != 2 {
			return nil
		}
		return opt.foldBitwise(instr.Op, instr.Args[0], instr.Args[1])
	}
	
	return nil
}

// foldArithmetic 折叠算术运算
// 增强版本：包含溢出检测
func (opt *Optimizer) foldArithmetic(op Opcode, left, right *IRValue) *IRValue {
	// 处理整数
	if left.Type == TypeInt && right.Type == TypeInt {
		l := left.ConstVal.AsInt()
		r := right.ConstVal.AsInt()
		var result int64
		var overflow bool
		
		switch op {
		case OpAdd:
			result, overflow = safeAddInt64(l, r)
			if overflow {
				return nil // 溢出时不折叠，保持运行时行为
			}
		case OpSub:
			result, overflow = safeSubInt64(l, r)
			if overflow {
				return nil
			}
		case OpMul:
			result, overflow = safeMulInt64(l, r)
			if overflow {
				return nil
			}
		case OpDiv:
			if r == 0 {
				return nil // 避免除零
			}
			// 检测 MinInt64 / -1 溢出
			if l == minInt64 && r == -1 {
				return nil
			}
			result = l / r
		case OpMod:
			if r == 0 {
				return nil
			}
			result = l % r
		default:
			return nil
		}
		
		return NewConstInt(-1, result)
	}
	
	// 处理浮点数
	if left.Type == TypeFloat || right.Type == TypeFloat {
		l := left.ConstVal.AsFloat()
		r := right.ConstVal.AsFloat()
		var result float64
		
		switch op {
		case OpAdd:
			result = l + r
		case OpSub:
			result = l - r
		case OpMul:
			result = l * r
		case OpDiv:
			if r == 0 {
				return nil
			}
			result = l / r
		default:
			return nil
		}
		
		return NewConstFloat(-1, result)
	}
	
	return nil
}

// 溢出检测辅助函数
const (
	maxInt64 = int64(1<<63 - 1)
	minInt64 = int64(-1 << 63)
)

// safeAddInt64 安全加法，检测溢出
func safeAddInt64(a, b int64) (int64, bool) {
	if b > 0 && a > maxInt64-b {
		return 0, true // 正溢出
	}
	if b < 0 && a < minInt64-b {
		return 0, true // 负溢出
	}
	return a + b, false
}

// safeSubInt64 安全减法，检测溢出
func safeSubInt64(a, b int64) (int64, bool) {
	if b < 0 && a > maxInt64+b {
		return 0, true // 正溢出
	}
	if b > 0 && a < minInt64+b {
		return 0, true // 负溢出
	}
	return a - b, false
}

// safeMulInt64 安全乘法，检测溢出
func safeMulInt64(a, b int64) (int64, bool) {
	if a == 0 || b == 0 {
		return 0, false
	}
	result := a * b
	if a == minInt64 || b == minInt64 {
		// 特殊处理 MinInt64
		if (a == minInt64 && b != 1) || (b == minInt64 && a != 1) {
			return 0, true
		}
	}
	if result/b != a {
		return 0, true // 溢出
	}
	return result, false
}

// foldComparison 折叠比较运算
func (opt *Optimizer) foldComparison(op Opcode, left, right *IRValue) *IRValue {
	// 整数比较
	if left.Type == TypeInt && right.Type == TypeInt {
		l := left.ConstVal.AsInt()
		r := right.ConstVal.AsInt()
		var result bool
		
		switch op {
		case OpEq:
			result = l == r
		case OpNe:
			result = l != r
		case OpLt:
			result = l < r
		case OpLe:
			result = l <= r
		case OpGt:
			result = l > r
		case OpGe:
			result = l >= r
		default:
			return nil
		}
		
		return NewConstBool(-1, result)
	}
	
	// 浮点数比较
	if left.Type == TypeFloat || right.Type == TypeFloat {
		l := left.ConstVal.AsFloat()
		r := right.ConstVal.AsFloat()
		var result bool
		
		switch op {
		case OpEq:
			result = l == r
		case OpNe:
			result = l != r
		case OpLt:
			result = l < r
		case OpLe:
			result = l <= r
		case OpGt:
			result = l > r
		case OpGe:
			result = l >= r
		default:
			return nil
		}
		
		return NewConstBool(-1, result)
	}
	
	return nil
}

// foldNeg 折叠取负
func (opt *Optimizer) foldNeg(operand *IRValue) *IRValue {
	if operand.Type == TypeInt {
		return NewConstInt(-1, -operand.ConstVal.AsInt())
	}
	if operand.Type == TypeFloat {
		return NewConstFloat(-1, -operand.ConstVal.AsFloat())
	}
	return nil
}

// foldNot 折叠逻辑非
func (opt *Optimizer) foldNot(operand *IRValue) *IRValue {
	if operand.Type == TypeBool {
		return NewConstBool(-1, !operand.ConstVal.AsBool())
	}
	return nil
}

// foldBitwise 折叠位运算
func (opt *Optimizer) foldBitwise(op Opcode, left, right *IRValue) *IRValue {
	if left.Type != TypeInt || right.Type != TypeInt {
		return nil
	}
	
	l := left.ConstVal.AsInt()
	r := right.ConstVal.AsInt()
	var result int64
	
	switch op {
	case OpBitAnd:
		result = l & r
	case OpBitOr:
		result = l | r
	case OpBitXor:
		result = l ^ r
	case OpShl:
		result = l << uint(r)
	case OpShr:
		result = l >> uint(r)
	default:
		return nil
	}
	
	return NewConstInt(-1, result)
}

// ============================================================================
// 死代码消除
// ============================================================================

// deadCodeElimination 死代码消除
// 增强版本：包含活性分析和不可达块消除
func (opt *Optimizer) deadCodeElimination(fn *IRFunc) bool {
	changed := false
	
	// 第一阶段：消除不可达的基本块
	changed = opt.removeUnreachableBlocks(fn) || changed
	
	// 第二阶段：活性分析
	liveValues := opt.livenessAnalysis(fn)
	
	// 第三阶段：删除死指令
	for _, block := range fn.Blocks {
		newInstrs := make([]*IRInstr, 0, len(block.Instrs))
		
		for _, instr := range block.Instrs {
			// 使用活性信息判断是否可以消除
			if opt.canEliminateWithLiveness(instr, liveValues) {
				opt.stats.DeadInstructionsRemoved++
				changed = true
				continue
			}
			newInstrs = append(newInstrs, instr)
		}
		
		if len(newInstrs) != len(block.Instrs) {
			block.Instrs = newInstrs
		}
	}
	
	return changed
}

// removeUnreachableBlocks 删除不可达的基本块
func (opt *Optimizer) removeUnreachableBlocks(fn *IRFunc) bool {
	if len(fn.Blocks) == 0 {
		return false
	}
	
	// 从入口块开始，标记所有可达的块
	reachable := make(map[int]bool)
	var markReachable func(block *IRBlock)
	markReachable = func(block *IRBlock) {
		if block == nil || reachable[block.ID] {
			return
		}
		reachable[block.ID] = true
		for _, succ := range block.Succs {
			markReachable(succ)
		}
	}
	
	// 从第一个块开始标记
	markReachable(fn.Blocks[0])
	
	// 删除不可达的块
	newBlocks := make([]*IRBlock, 0, len(fn.Blocks))
	removedCount := 0
	for _, block := range fn.Blocks {
		if reachable[block.ID] {
			newBlocks = append(newBlocks, block)
		} else {
			removedCount++
		}
	}
	
	if removedCount > 0 {
		fn.Blocks = newBlocks
		opt.stats.UnreachableBlocksRemoved += removedCount
		return true
	}
	
	return false
}

// livenessAnalysis 活性分析
// 返回所有活跃值的集合（在程序某点之后会被使用的值）
func (opt *Optimizer) livenessAnalysis(fn *IRFunc) map[*IRValue]bool {
	liveOut := make(map[*IRValue]bool)
	
	// 反向遍历：从最后一个块到第一个块
	// 简化版本：收集所有被使用的值
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			// 所有参数都是活跃的
			for _, arg := range instr.Args {
				if arg != nil && !arg.IsConst {
					liveOut[arg] = true
				}
			}
			// 跳转目标中的 Phi 参数也是活跃的
			if instr.Op == OpPhi {
				for _, arg := range instr.Args {
					if arg != nil {
						liveOut[arg] = true
					}
				}
			}
		}
	}
	
	// 返回和分支目标中使用的值也是活跃的
	for _, block := range fn.Blocks {
		lastInstr := block.LastInstr()
		if lastInstr != nil && lastInstr.Op == OpReturn {
			for _, arg := range lastInstr.Args {
				if arg != nil {
					liveOut[arg] = true
				}
			}
		}
	}
	
	return liveOut
}

// canEliminateWithLiveness 使用活性信息判断指令是否可以消除
func (opt *Optimizer) canEliminateWithLiveness(instr *IRInstr, liveValues map[*IRValue]bool) bool {
	// NOP 可以消除
	if instr.Op == OpNop {
		return true
	}
	
	// 有副作用的指令不能消除
	if opt.hasSideEffect(instr) {
		return false
	}
	
	// 没有目标值的指令不能通过 DCE 消除
	if instr.Dest == nil {
		return false
	}
	
	// 如果目标值不活跃（不会被使用），可以消除
	return !liveValues[instr.Dest] && !instr.Dest.HasUses()
}

// canEliminate 检查指令是否可以被消除
func (opt *Optimizer) canEliminate(instr *IRInstr) bool {
	// NOP 可以消除
	if instr.Op == OpNop {
		return true
	}
	
	// 有副作用的指令不能消除
	if opt.hasSideEffect(instr) {
		return false
	}
	
	// 没有目标值的指令不能通过 DCE 消除
	if instr.Dest == nil {
		return false
	}
	
	// 如果目标值没有使用者，可以消除
	return !instr.Dest.HasUses()
}

// hasSideEffect 检查指令是否有副作用
func (opt *Optimizer) hasSideEffect(instr *IRInstr) bool {
	switch instr.Op {
	case OpStoreLocal, OpCall, OpCallMethod, OpReturn, 
		OpJump, OpBranch, OpArraySet:
		return true
	}
	return false
}

// ============================================================================
// 代数简化
// ============================================================================

// algebraicSimplification 代数简化
// 应用代数恒等式简化表达式
func (opt *Optimizer) algebraicSimplification(fn *IRFunc) bool {
	changed := false
	
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			if simplified := opt.simplify(instr); simplified {
				changed = true
			}
		}
	}
	
	return changed
}

// simplify 简化单条指令
func (opt *Optimizer) simplify(instr *IRInstr) bool {
	switch instr.Op {
	case OpAdd:
		// x + 0 = x
		if opt.isZero(instr.Args[1]) {
			opt.replaceWith(instr, instr.Args[0])
			return true
		}
		// 0 + x = x
		if opt.isZero(instr.Args[0]) {
			opt.replaceWith(instr, instr.Args[1])
			return true
		}
		
	case OpSub:
		// x - 0 = x
		if opt.isZero(instr.Args[1]) {
			opt.replaceWith(instr, instr.Args[0])
			return true
		}
		// x - x = 0
		if instr.Args[0] == instr.Args[1] {
			instr.Op = OpConst
			instr.Dest.IsConst = true
			instr.Dest.ConstVal = NewConstInt(-1, 0).ConstVal
			instr.Args = nil
			return true
		}
		
	case OpMul:
		// x * 0 = 0
		if opt.isZero(instr.Args[0]) || opt.isZero(instr.Args[1]) {
			instr.Op = OpConst
			instr.Dest.IsConst = true
			instr.Dest.ConstVal = NewConstInt(-1, 0).ConstVal
			instr.Args = nil
			return true
		}
		// x * 1 = x
		if opt.isOne(instr.Args[1]) {
			opt.replaceWith(instr, instr.Args[0])
			return true
		}
		// 1 * x = x
		if opt.isOne(instr.Args[0]) {
			opt.replaceWith(instr, instr.Args[1])
			return true
		}
		
	case OpDiv:
		// x / 1 = x
		if opt.isOne(instr.Args[1]) {
			opt.replaceWith(instr, instr.Args[0])
			return true
		}
		// 0 / x = 0 (x != 0)
		if opt.isZero(instr.Args[0]) && !opt.isZero(instr.Args[1]) {
			instr.Op = OpConst
			instr.Dest.IsConst = true
			instr.Dest.ConstVal = NewConstInt(-1, 0).ConstVal
			instr.Args = nil
			return true
		}
		
	case OpBitAnd:
		// x & 0 = 0
		if opt.isZero(instr.Args[0]) || opt.isZero(instr.Args[1]) {
			instr.Op = OpConst
			instr.Dest.IsConst = true
			instr.Dest.ConstVal = NewConstInt(-1, 0).ConstVal
			instr.Args = nil
			return true
		}
		// x & x = x
		if instr.Args[0] == instr.Args[1] {
			opt.replaceWith(instr, instr.Args[0])
			return true
		}
		
	case OpBitOr:
		// x | 0 = x
		if opt.isZero(instr.Args[1]) {
			opt.replaceWith(instr, instr.Args[0])
			return true
		}
		// 0 | x = x
		if opt.isZero(instr.Args[0]) {
			opt.replaceWith(instr, instr.Args[1])
			return true
		}
		// x | x = x
		if instr.Args[0] == instr.Args[1] {
			opt.replaceWith(instr, instr.Args[0])
			return true
		}
		
	case OpBitXor:
		// x ^ 0 = x
		if opt.isZero(instr.Args[1]) {
			opt.replaceWith(instr, instr.Args[0])
			return true
		}
		// 0 ^ x = x
		if opt.isZero(instr.Args[0]) {
			opt.replaceWith(instr, instr.Args[1])
			return true
		}
		// x ^ x = 0
		if instr.Args[0] == instr.Args[1] {
			instr.Op = OpConst
			instr.Dest.IsConst = true
			instr.Dest.ConstVal = NewConstInt(-1, 0).ConstVal
			instr.Args = nil
			return true
		}
		
	case OpShl, OpShr:
		// x << 0 = x, x >> 0 = x
		if opt.isZero(instr.Args[1]) {
			opt.replaceWith(instr, instr.Args[0])
			return true
		}
	}
	
	return false
}

// isZero 检查值是否为 0
func (opt *Optimizer) isZero(v *IRValue) bool {
	if v == nil || !v.IsConst {
		return false
	}
	if v.Type == TypeInt {
		return v.ConstVal.AsInt() == 0
	}
	if v.Type == TypeFloat {
		return v.ConstVal.AsFloat() == 0
	}
	return false
}

// isOne 检查值是否为 1
func (opt *Optimizer) isOne(v *IRValue) bool {
	if v == nil || !v.IsConst {
		return false
	}
	if v.Type == TypeInt {
		return v.ConstVal.AsInt() == 1
	}
	if v.Type == TypeFloat {
		return v.ConstVal.AsFloat() == 1
	}
	return false
}

// replaceWith 将指令替换为使用另一个值
func (opt *Optimizer) replaceWith(instr *IRInstr, replacement *IRValue) {
	// 将所有使用 instr.Dest 的地方替换为 replacement
	if instr.Dest != nil {
		for _, use := range instr.Dest.Uses {
			use.ReplaceArg(instr.Dest, replacement)
		}
	}
	// 将指令标记为 NOP
	instr.Op = OpNop
	instr.Dest = nil
	instr.Args = nil
}

// ============================================================================
// 强度削减
// ============================================================================

// strengthReduction 强度削减
// 用更廉价的操作替代昂贵的操作
func (opt *Optimizer) strengthReduction(fn *IRFunc) bool {
	changed := false
	
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			switch instr.Op {
			case OpMul:
				// 乘以 2 的幂次 -> 左移
				if opt.isPowerOfTwo(instr.Args[1]) {
					shift := opt.log2(instr.Args[1].ConstVal.AsInt())
					instr.Op = OpShl
					instr.Args[1] = NewConstInt(-1, int64(shift))
					changed = true
				} else if opt.isPowerOfTwo(instr.Args[0]) {
					shift := opt.log2(instr.Args[0].ConstVal.AsInt())
					instr.Op = OpShl
					instr.Args[0] = instr.Args[1]
					instr.Args[1] = NewConstInt(-1, int64(shift))
					changed = true
				}
				
			case OpDiv:
				// 除以 2 的幂次 -> 右移（仅对无符号整数安全）
				// 暂时不实现，因为我们没有无符号整数类型
			}
		}
	}
	
	return changed
}

// isPowerOfTwo 检查是否是 2 的幂次
func (opt *Optimizer) isPowerOfTwo(v *IRValue) bool {
	if v == nil || !v.IsConst || v.Type != TypeInt {
		return false
	}
	n := v.ConstVal.AsInt()
	return n > 0 && (n&(n-1)) == 0
}

// log2 计算 log2(n)
func (opt *Optimizer) log2(n int64) int {
	result := 0
	for n > 1 {
		n >>= 1
		result++
	}
	return result
}

// ============================================================================
// 清理
// ============================================================================

// removeNops 移除 NOP 指令
func (opt *Optimizer) removeNops(fn *IRFunc) {
	for _, block := range fn.Blocks {
		newInstrs := make([]*IRInstr, 0, len(block.Instrs))
		for _, instr := range block.Instrs {
			if instr.Op != OpNop {
				newInstrs = append(newInstrs, instr)
			}
		}
		block.Instrs = newInstrs
	}
}
