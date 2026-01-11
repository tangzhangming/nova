package jit

import (
	"github.com/tangzhangming/nova/internal/bytecode"
)

// ============================================================================
// 优化 Pass 接口
// ============================================================================

// Pass 优化 Pass 接口
type Pass interface {
	Name() string
	Run(fn *IRFunction) bool // 返回是否有修改
}

// FunctionPass 函数级别的 Pass
type FunctionPass interface {
	Pass
	RunOnFunction(fn *IRFunction) bool
}

// BlockPass 基本块级别的 Pass
type BlockPass interface {
	Pass
	RunOnBlock(bb *BasicBlock) bool
}

// ============================================================================
// Pass 管理器
// ============================================================================

// PassManager Pass 管理器
type PassManager struct {
	passes []Pass
	stats  PassStats
}

// PassStats Pass 统计信息
type PassStats struct {
	PassesRun   int
	TotalChanges int
	PerPassChanges map[string]int
}

// NewPassManager 创建 Pass 管理器
func NewPassManager() *PassManager {
	return &PassManager{
		passes: make([]Pass, 0),
		stats: PassStats{
			PerPassChanges: make(map[string]int),
		},
	}
}

// AddPass 添加 Pass
func (pm *PassManager) AddPass(p Pass) {
	pm.passes = append(pm.passes, p)
}

// Run 运行所有 Pass
func (pm *PassManager) Run(fn *IRFunction) {
	for _, p := range pm.passes {
		pm.stats.PassesRun++
		if p.Run(fn) {
			pm.stats.TotalChanges++
			pm.stats.PerPassChanges[p.Name()]++
		}
	}
}

// RunUntilFixed 运行 Pass 直到不再有改变
func (pm *PassManager) RunUntilFixed(fn *IRFunction, maxIters int) {
	for i := 0; i < maxIters; i++ {
		changed := false
		for _, p := range pm.passes {
			if p.Run(fn) {
				changed = true
			}
		}
		if !changed {
			break
		}
	}
}

// Stats 获取统计信息
func (pm *PassManager) Stats() PassStats {
	return pm.stats
}

// ============================================================================
// 预置优化 Pipeline
// ============================================================================

// CreateStandardPipeline 创建标准优化 Pipeline
func CreateStandardPipeline() *PassManager {
	pm := NewPassManager()
	
	// 基本优化
	pm.AddPass(NewConstantFoldingPass())
	pm.AddPass(NewDeadCodeEliminationPass())
	pm.AddPass(NewConstantPropagationPass())
	
	// 强度削减
	pm.AddPass(NewStrengthReductionPass())
	
	// 循环优化
	pm.AddPass(NewLoopInvariantMotionPass())
	
	// 清理
	pm.AddPass(NewDeadCodeEliminationPass())
	
	return pm
}

// CreateFastPipeline 创建快速优化 Pipeline (用于 Tier-1 JIT)
func CreateFastPipeline() *PassManager {
	pm := NewPassManager()
	
	// 只做最基本的优化
	pm.AddPass(NewConstantFoldingPass())
	pm.AddPass(NewDeadCodeEliminationPass())
	
	return pm
}

// ============================================================================
// 常量折叠 Pass
// ============================================================================

// ConstantFoldingPass 常量折叠
type ConstantFoldingPass struct{}

// NewConstantFoldingPass 创建常量折叠 Pass
func NewConstantFoldingPass() *ConstantFoldingPass {
	return &ConstantFoldingPass{}
}

// Name 返回 Pass 名称
func (p *ConstantFoldingPass) Name() string {
	return "constant-folding"
}

// Run 运行 Pass
func (p *ConstantFoldingPass) Run(fn *IRFunction) bool {
	if fn.CFG == nil {
		return false
	}
	
	changed := false
	for _, bb := range fn.CFG.Blocks {
		if p.runOnBlock(bb) {
			changed = true
		}
	}
	return changed
}

// runOnBlock 在基本块上运行
func (p *ConstantFoldingPass) runOnBlock(bb *BasicBlock) bool {
	changed := false
	newInsts := make([]IRInst, 0, len(bb.Insts))
	
	for i := 0; i < len(bb.Insts); i++ {
		inst := bb.Insts[i]
		
		// 尝试折叠连续的 CONST + CONST + OP
		if inst.Op == IR_CONST && i+2 < len(bb.Insts) {
			next := bb.Insts[i+1]
			if next.Op == IR_CONST {
				op := bb.Insts[i+2]
				if folded, ok := foldBinaryOp(inst.Value, next.Value, op.Op); ok {
					newInsts = append(newInsts, IRInst{Op: IR_CONST, Value: folded})
					i += 2 // 跳过后两条指令
					changed = true
					continue
				}
			}
		}
		
		newInsts = append(newInsts, inst)
	}
	
	if changed {
		bb.Insts = newInsts
	}
	return changed
}

// foldBinaryOp 折叠二元操作
func foldBinaryOp(a, b bytecode.Value, op IROp) (bytecode.Value, bool) {
	// 只处理整数常量
	if !a.IsInt() || !b.IsInt() {
		return bytecode.NullValue, false
	}
	
	ai, bi := a.AsInt(), b.AsInt()
	
	switch op {
	case IR_ADD:
		return bytecode.NewInt(ai + bi), true
	case IR_SUB:
		return bytecode.NewInt(ai - bi), true
	case IR_MUL:
		return bytecode.NewInt(ai * bi), true
	case IR_DIV:
		if bi != 0 {
			return bytecode.NewInt(ai / bi), true
		}
	case IR_MOD:
		if bi != 0 {
			return bytecode.NewInt(ai % bi), true
		}
	case IR_BAND:
		return bytecode.NewInt(ai & bi), true
	case IR_BOR:
		return bytecode.NewInt(ai | bi), true
	case IR_BXOR:
		return bytecode.NewInt(ai ^ bi), true
	case IR_SHL:
		if bi >= 0 && bi < 64 {
			return bytecode.NewInt(ai << uint(bi)), true
		}
	case IR_SHR:
		if bi >= 0 && bi < 64 {
			return bytecode.NewInt(ai >> uint(bi)), true
		}
	case IR_EQ:
		return bytecode.NewBool(ai == bi), true
	case IR_NE:
		return bytecode.NewBool(ai != bi), true
	case IR_LT:
		return bytecode.NewBool(ai < bi), true
	case IR_LE:
		return bytecode.NewBool(ai <= bi), true
	case IR_GT:
		return bytecode.NewBool(ai > bi), true
	case IR_GE:
		return bytecode.NewBool(ai >= bi), true
	}
	
	return bytecode.NullValue, false
}

// ============================================================================
// 死代码消除 Pass
// ============================================================================

// DeadCodeEliminationPass 死代码消除
type DeadCodeEliminationPass struct{}

// NewDeadCodeEliminationPass 创建死代码消除 Pass
func NewDeadCodeEliminationPass() *DeadCodeEliminationPass {
	return &DeadCodeEliminationPass{}
}

// Name 返回 Pass 名称
func (p *DeadCodeEliminationPass) Name() string {
	return "dead-code-elimination"
}

// Run 运行 Pass
func (p *DeadCodeEliminationPass) Run(fn *IRFunction) bool {
	if fn.CFG == nil {
		return false
	}
	
	changed := false
	
	// 移除 RETURN 后的指令
	for _, bb := range fn.CFG.Blocks {
		for i, inst := range bb.Insts {
			if inst.Op == IR_RETURN && i+1 < len(bb.Insts) {
				bb.Insts = bb.Insts[:i+1]
				changed = true
				break
			}
		}
	}
	
	// 移除不可达块
	reachable := make(map[*BasicBlock]bool)
	if fn.CFG.Entry != nil {
		markReachable(fn.CFG.Entry, reachable)
	}
	
	newBlocks := make([]*BasicBlock, 0, len(fn.CFG.Blocks))
	for _, bb := range fn.CFG.Blocks {
		if reachable[bb] {
			newBlocks = append(newBlocks, bb)
		} else {
			changed = true
		}
	}
	fn.CFG.Blocks = newBlocks
	
	// 移除无效的 PUSH + POP 对
	for _, bb := range fn.CFG.Blocks {
		if p.removeUselessPushPop(bb) {
			changed = true
		}
	}
	
	return changed
}

// markReachable 标记可达块
func markReachable(bb *BasicBlock, reachable map[*BasicBlock]bool) {
	if reachable[bb] {
		return
	}
	reachable[bb] = true
	for _, succ := range bb.Succs {
		markReachable(succ, reachable)
	}
}

// removeUselessPushPop 移除无效的 PUSH+POP
func (p *DeadCodeEliminationPass) removeUselessPushPop(bb *BasicBlock) bool {
	changed := false
	newInsts := make([]IRInst, 0, len(bb.Insts))
	
	for i := 0; i < len(bb.Insts); i++ {
		inst := bb.Insts[i]
		
		// CONST + POP 模式
		if inst.Op == IR_CONST && i+1 < len(bb.Insts) && bb.Insts[i+1].Op == IR_POP {
			i++ // 跳过 POP
			changed = true
			continue
		}
		
		// DUP + POP 模式
		if inst.Op == IR_DUP && i+1 < len(bb.Insts) && bb.Insts[i+1].Op == IR_POP {
			i++ // 跳过 POP
			changed = true
			continue
		}
		
		newInsts = append(newInsts, inst)
	}
	
	if changed {
		bb.Insts = newInsts
	}
	return changed
}

// ============================================================================
// 常量传播 Pass
// ============================================================================

// ConstantPropagationPass 常量传播
type ConstantPropagationPass struct{}

// NewConstantPropagationPass 创建常量传播 Pass
func NewConstantPropagationPass() *ConstantPropagationPass {
	return &ConstantPropagationPass{}
}

// Name 返回 Pass 名称
func (p *ConstantPropagationPass) Name() string {
	return "constant-propagation"
}

// Run 运行 Pass
func (p *ConstantPropagationPass) Run(fn *IRFunction) bool {
	if fn.CFG == nil {
		return false
	}
	
	// 收集局部变量的常量值
	constants := make(map[int]bytecode.Value) // slot -> value
	changed := false
	
	for _, bb := range fn.CFG.Blocks {
		for i, inst := range bb.Insts {
			// 检测 STORE_LOCAL 常量
			if inst.Op == IR_STORE_LOCAL {
				if i > 0 && bb.Insts[i-1].Op == IR_CONST {
					constants[inst.Arg1] = bb.Insts[i-1].Value
				} else {
					// 非常量赋值，移除记录
					delete(constants, inst.Arg1)
				}
			}
			
			// 替换 LOAD_LOCAL 为 CONST
			if inst.Op == IR_LOAD_LOCAL {
				if val, ok := constants[inst.Arg1]; ok {
					bb.Insts[i] = IRInst{Op: IR_CONST, Value: val, BytecodeIP: inst.BytecodeIP}
					changed = true
				}
			}
		}
	}
	
	return changed
}

// ============================================================================
// 强度削减 Pass
// ============================================================================

// StrengthReductionPass 强度削减
type StrengthReductionPass struct{}

// NewStrengthReductionPass 创建强度削减 Pass
func NewStrengthReductionPass() *StrengthReductionPass {
	return &StrengthReductionPass{}
}

// Name 返回 Pass 名称
func (p *StrengthReductionPass) Name() string {
	return "strength-reduction"
}

// Run 运行 Pass
func (p *StrengthReductionPass) Run(fn *IRFunction) bool {
	if fn.CFG == nil {
		return false
	}
	
	changed := false
	for _, bb := range fn.CFG.Blocks {
		if p.runOnBlock(bb) {
			changed = true
		}
	}
	return changed
}

// runOnBlock 在基本块上运行
func (p *StrengthReductionPass) runOnBlock(bb *BasicBlock) bool {
	changed := false
	
	for i := 0; i < len(bb.Insts); i++ {
		// 查找 CONST + MUL 模式
		if i+1 < len(bb.Insts) {
			constInst := bb.Insts[i]
			mulInst := bb.Insts[i+1]
			
			if constInst.Op == IR_CONST && mulInst.Op == IR_MUL {
				if constInst.Value.IsInt() {
					n := constInst.Value.AsInt()
					
					// 乘以 2 的幂可以用移位
					if shift := log2(n); shift >= 0 {
						bb.Insts[i] = IRInst{
							Op:    IR_CONST,
							Value: bytecode.NewInt(int64(shift)),
							BytecodeIP: constInst.BytecodeIP,
						}
						bb.Insts[i+1] = IRInst{
							Op:         IR_SHL,
							BytecodeIP: mulInst.BytecodeIP,
						}
						changed = true
					}
					
					// 乘以 0
					if n == 0 {
						// 替换为 POP + CONST 0
						bb.Insts[i] = IRInst{Op: IR_POP}
						bb.Insts[i+1] = IRInst{Op: IR_CONST, Value: bytecode.ZeroValue}
						changed = true
					}
					
					// 乘以 1
					if n == 1 {
						// 删除乘法
						bb.Insts = append(bb.Insts[:i], bb.Insts[i+2:]...)
						i--
						changed = true
					}
				}
			}
			
			// 查找 CONST + DIV 模式
			if constInst.Op == IR_CONST && mulInst.Op == IR_DIV {
				if constInst.Value.IsInt() {
					n := constInst.Value.AsInt()
					
					// 除以 2 的幂可以用移位
					if shift := log2(n); shift >= 0 && n > 0 {
						bb.Insts[i] = IRInst{
							Op:    IR_CONST,
							Value: bytecode.NewInt(int64(shift)),
							BytecodeIP: constInst.BytecodeIP,
						}
						bb.Insts[i+1] = IRInst{
							Op:         IR_SHR,
							BytecodeIP: mulInst.BytecodeIP,
						}
						changed = true
					}
					
					// 除以 1
					if n == 1 {
						bb.Insts = append(bb.Insts[:i], bb.Insts[i+2:]...)
						i--
						changed = true
					}
				}
			}
		}
	}
	
	return changed
}

// log2 计算以 2 为底的对数 (仅对 2 的幂)
func log2(n int64) int {
	if n <= 0 || (n&(n-1)) != 0 {
		return -1 // 不是 2 的幂
	}
	
	shift := 0
	for n > 1 {
		n >>= 1
		shift++
	}
	return shift
}

// ============================================================================
// 循环不变量外提 Pass
// ============================================================================

// LoopInvariantMotionPass 循环不变量外提
type LoopInvariantMotionPass struct{}

// NewLoopInvariantMotionPass 创建循环不变量外提 Pass
func NewLoopInvariantMotionPass() *LoopInvariantMotionPass {
	return &LoopInvariantMotionPass{}
}

// Name 返回 Pass 名称
func (p *LoopInvariantMotionPass) Name() string {
	return "loop-invariant-motion"
}

// Run 运行 Pass
func (p *LoopInvariantMotionPass) Run(fn *IRFunction) bool {
	if fn.CFG == nil {
		return false
	}
	
	// 检测循环
	loops := p.detectLoops(fn.CFG)
	if len(loops) == 0 {
		return false
	}
	
	changed := false
	
	for _, loop := range loops {
		// 标记循环深度
		for _, bb := range loop.Blocks {
			bb.LoopDepth++
		}
		
		// 识别循环不变量
		invariants := p.findInvariants(loop)
		
		// 移动不变量到 preheader
		if len(invariants) > 0 && loop.Preheader != nil {
			for _, inv := range invariants {
				loop.Preheader.AddInst(inv)
			}
			changed = true
		}
	}
	
	return changed
}

// Loop 循环结构
type Loop struct {
	Header    *BasicBlock   // 循环头
	Preheader *BasicBlock   // 循环前驱
	Blocks    []*BasicBlock // 循环体
	BackEdges []*BasicBlock // 回边源
}

// detectLoops 检测循环
func (p *LoopInvariantMotionPass) detectLoops(cfg *CFG) []*Loop {
	var loops []*Loop
	
	// 简单的回边检测
	for _, bb := range cfg.Blocks {
		for _, succ := range bb.Succs {
			// 如果后继在前驱之前出现，可能是回边
			if succ.ID < bb.ID {
				// 找到一个潜在循环
				loop := &Loop{
					Header:    succ,
					BackEdges: []*BasicBlock{bb},
					Blocks:    make([]*BasicBlock, 0),
				}
				
				// 收集循环体
				p.collectLoopBlocks(loop, cfg)
				
				if len(loop.Blocks) > 0 {
					loops = append(loops, loop)
				}
			}
		}
	}
	
	return loops
}

// collectLoopBlocks 收集循环体块
func (p *LoopInvariantMotionPass) collectLoopBlocks(loop *Loop, cfg *CFG) {
	visited := make(map[*BasicBlock]bool)
	worklist := make([]*BasicBlock, 0)
	
	// 从回边开始反向搜索
	for _, back := range loop.BackEdges {
		if !visited[back] {
			visited[back] = true
			worklist = append(worklist, back)
			loop.Blocks = append(loop.Blocks, back)
		}
	}
	
	// 标记头部
	visited[loop.Header] = true
	loop.Blocks = append(loop.Blocks, loop.Header)
	
	// 反向遍历
	for len(worklist) > 0 {
		bb := worklist[len(worklist)-1]
		worklist = worklist[:len(worklist)-1]
		
		for _, pred := range bb.Preds {
			if !visited[pred] {
				visited[pred] = true
				worklist = append(worklist, pred)
				loop.Blocks = append(loop.Blocks, pred)
			}
		}
	}
	
	// 找 preheader
	for _, pred := range loop.Header.Preds {
		isBack := false
		for _, back := range loop.BackEdges {
			if pred == back {
				isBack = true
				break
			}
		}
		if !isBack {
			loop.Preheader = pred
			break
		}
	}
}

// findInvariants 查找循环不变量
func (p *LoopInvariantMotionPass) findInvariants(loop *Loop) []IRInst {
	var invariants []IRInst
	
	// 简单处理：只移动常量加载
	for _, bb := range loop.Blocks {
		newInsts := make([]IRInst, 0, len(bb.Insts))
		
		for _, inst := range bb.Insts {
			// 常量是循环不变的
			if inst.Op == IR_CONST {
				invariants = append(invariants, inst)
			} else {
				newInsts = append(newInsts, inst)
			}
		}
		
		if len(newInsts) < len(bb.Insts) {
			bb.Insts = newInsts
		}
	}
	
	return invariants
}

// ============================================================================
// 窥孔优化 Pass
// ============================================================================

// PeepholePass 窥孔优化
type PeepholePass struct{}

// NewPeepholePass 创建窥孔优化 Pass
func NewPeepholePass() *PeepholePass {
	return &PeepholePass{}
}

// Name 返回 Pass 名称
func (p *PeepholePass) Name() string {
	return "peephole"
}

// Run 运行 Pass
func (p *PeepholePass) Run(fn *IRFunction) bool {
	if fn.CFG == nil {
		return false
	}
	
	changed := false
	for _, bb := range fn.CFG.Blocks {
		if p.runOnBlock(bb) {
			changed = true
		}
	}
	return changed
}

// runOnBlock 在基本块上运行
func (p *PeepholePass) runOnBlock(bb *BasicBlock) bool {
	changed := false
	newInsts := make([]IRInst, 0, len(bb.Insts))
	
	for i := 0; i < len(bb.Insts); i++ {
		inst := bb.Insts[i]
		
		// NEG + NEG = 原值
		if inst.Op == IR_NEG && i+1 < len(bb.Insts) && bb.Insts[i+1].Op == IR_NEG {
			i++ // 跳过第二个 NEG
			changed = true
			continue
		}
		
		// NOT + NOT = 原值
		if inst.Op == IR_NOT && i+1 < len(bb.Insts) && bb.Insts[i+1].Op == IR_NOT {
			i++
			changed = true
			continue
		}
		
		// BNOT + BNOT = 原值
		if inst.Op == IR_BNOT && i+1 < len(bb.Insts) && bb.Insts[i+1].Op == IR_BNOT {
			i++
			changed = true
			continue
		}
		
		// ADD 0 = 无操作
		if inst.Op == IR_CONST && inst.Value.IsInt() && inst.Value.AsInt() == 0 {
			if i+1 < len(bb.Insts) && bb.Insts[i+1].Op == IR_ADD {
				i++ // 跳过 ADD
				changed = true
				continue
			}
		}
		
		// MUL 1 = 无操作
		if inst.Op == IR_CONST && inst.Value.IsInt() && inst.Value.AsInt() == 1 {
			if i+1 < len(bb.Insts) && bb.Insts[i+1].Op == IR_MUL {
				i++
				changed = true
				continue
			}
		}
		
		newInsts = append(newInsts, inst)
	}
	
	if changed {
		bb.Insts = newInsts
	}
	return changed
}
