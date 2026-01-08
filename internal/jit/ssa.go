// ssa.go - SSA 转换算法
//
// 本文件实现了从非 SSA IR 到完整 SSA 形式的转换。
//
// 算法概述：
// 1. 计算支配树（使用 Lengauer-Tarjan 算法的简化版本）
// 2. 计算支配边界
// 3. 在支配边界处插入 Phi 节点
// 4. 重命名变量使其符合 SSA 形式
//
// 参考文献：
// - "A Simple, Fast Dominance Algorithm" - Keith D. Cooper, Timothy J. Harvey, Ken Kennedy
// - "Efficiently Computing Static Single Assignment Form and the Control Dependence Graph"

package jit

// ============================================================================
// SSA 转换器
// ============================================================================

// SSABuilder SSA 转换器
type SSABuilder struct {
	fn *IRFunc
	
	// 支配树信息
	idom     map[*IRBlock]*IRBlock   // 直接支配者
	domTree  map[*IRBlock][]*IRBlock // 支配树（支配者 -> 被支配者列表）
	domFront map[*IRBlock][]*IRBlock // 支配边界
	
	// 变量信息
	// localDefs[block][localIdx] = 该块中对 local[localIdx] 的所有定义
	localDefs map[*IRBlock]map[int][]*IRValue
	
	// 变量重命名栈
	varStack map[int][]*IRValue // localIdx -> 定义栈
	varCount map[int]int        // localIdx -> 计数器（用于生成唯一名称）
}

// NewSSABuilder 创建 SSA 转换器
func NewSSABuilder(fn *IRFunc) *SSABuilder {
	return &SSABuilder{
		fn:        fn,
		idom:      make(map[*IRBlock]*IRBlock),
		domTree:   make(map[*IRBlock][]*IRBlock),
		domFront:  make(map[*IRBlock][]*IRBlock),
		localDefs: make(map[*IRBlock]map[int][]*IRValue),
		varStack:  make(map[int][]*IRValue),
		varCount:  make(map[int]int),
	}
}

// Build 执行 SSA 转换
func (s *SSABuilder) Build() {
	if len(s.fn.Blocks) == 0 {
		return
	}
	
	// 第一步：计算支配树
	s.computeDominators()
	
	// 第二步：计算支配边界
	s.computeDominanceFrontier()
	
	// 第三步：收集变量定义信息
	s.collectLocalDefs()
	
	// 第四步：插入 Phi 节点
	s.insertPhis()
	
	// 第五步：重命名变量
	s.renameVariables()
}

// ============================================================================
// 支配树计算
// ============================================================================

// computeDominators 计算支配树
// 使用 Cooper 等人的简化算法
func (s *SSABuilder) computeDominators() {
	// 按逆后序遍历块
	order := s.fn.ComputeBlockOrder()
	
	// 初始化：入口块支配自己
	s.idom[s.fn.Entry] = s.fn.Entry
	
	// 迭代直到不动点
	changed := true
	for changed {
		changed = false
		
		for _, b := range order {
			if b == s.fn.Entry {
				continue
			}
			
			// 找到第一个已处理的前驱
			var newIdom *IRBlock
			for _, pred := range b.Preds {
				if s.idom[pred] != nil {
					newIdom = pred
					break
				}
			}
			
			if newIdom == nil {
				continue
			}
			
			// 与其他已处理的前驱求交集
			for _, pred := range b.Preds {
				if pred == newIdom {
					continue
				}
				if s.idom[pred] != nil {
					newIdom = s.intersect(pred, newIdom)
				}
			}
			
			// 检查是否有变化
			if s.idom[b] != newIdom {
				s.idom[b] = newIdom
				changed = true
			}
		}
	}
	
	// 构建支配树
	for b, dom := range s.idom {
		if b != dom { // 排除入口块自我支配的情况
			s.domTree[dom] = append(s.domTree[dom], b)
		}
	}
	
	// 将支配信息存储到块中
	for b, dom := range s.idom {
		b.IDom = dom
	}
}

// intersect 计算两个块的最近公共支配者
func (s *SSABuilder) intersect(b1, b2 *IRBlock) *IRBlock {
	finger1 := b1
	finger2 := b2
	
	for finger1 != finger2 {
		// 比较块的 ID（假设 ID 按逆后序分配）
		for finger1.ID > finger2.ID {
			finger1 = s.idom[finger1]
			if finger1 == nil {
				return s.fn.Entry
			}
		}
		for finger2.ID > finger1.ID {
			finger2 = s.idom[finger2]
			if finger2 == nil {
				return s.fn.Entry
			}
		}
	}
	
	return finger1
}

// ============================================================================
// 支配边界计算
// ============================================================================

// computeDominanceFrontier 计算支配边界
func (s *SSABuilder) computeDominanceFrontier() {
	for _, b := range s.fn.Blocks {
		s.domFront[b] = make([]*IRBlock, 0)
	}
	
	for _, b := range s.fn.Blocks {
		// 只有当块有多个前驱时才需要计算
		if len(b.Preds) < 2 {
			continue
		}
		
		for _, pred := range b.Preds {
			runner := pred
			for runner != nil && runner != s.idom[b] {
				// b 在 runner 的支配边界中
				s.addToDomFront(runner, b)
				runner = s.idom[runner]
			}
		}
	}
	
	// 将支配边界存储到块中
	for b, front := range s.domFront {
		b.DomFront = front
	}
}

// addToDomFront 添加块到支配边界（避免重复）
func (s *SSABuilder) addToDomFront(block, frontier *IRBlock) {
	for _, f := range s.domFront[block] {
		if f == frontier {
			return
		}
	}
	s.domFront[block] = append(s.domFront[block], frontier)
}

// ============================================================================
// 变量定义收集
// ============================================================================

// collectLocalDefs 收集每个块中的局部变量定义
func (s *SSABuilder) collectLocalDefs() {
	for _, b := range s.fn.Blocks {
		s.localDefs[b] = make(map[int][]*IRValue)
		
		for _, instr := range b.Instrs {
			// StoreLocal 指令定义了一个局部变量
			if instr.Op == OpStoreLocal && len(instr.Args) > 0 {
				localIdx := instr.LocalIdx
				s.localDefs[b][localIdx] = append(s.localDefs[b][localIdx], instr.Args[0])
			}
			
			// LoadLocal 指令的目标值也是一个定义
			if instr.Op == OpLoadLocal && instr.Dest != nil {
				// 这是参数加载，不需要 Phi
			}
		}
	}
}

// ============================================================================
// Phi 节点插入
// ============================================================================

// insertPhis 在需要的地方插入 Phi 节点
func (s *SSABuilder) insertPhis() {
	// 对每个局部变量
	for localIdx := 0; localIdx < s.fn.LocalCount; localIdx++ {
		// 收集定义该变量的所有块
		defBlocks := make([]*IRBlock, 0)
		for b, defs := range s.localDefs {
			if len(defs[localIdx]) > 0 {
				defBlocks = append(defBlocks, b)
			}
		}
		
		if len(defBlocks) == 0 {
			continue
		}
		
		// 使用工作表算法插入 Phi 节点
		hasAlready := make(map[*IRBlock]bool)
		everOnWorklist := make(map[*IRBlock]bool)
		worklist := make([]*IRBlock, 0)
		
		// 初始化工作表
		for _, b := range defBlocks {
			everOnWorklist[b] = true
			worklist = append(worklist, b)
		}
		
		for len(worklist) > 0 {
			// 取出一个块
			n := worklist[len(worklist)-1]
			worklist = worklist[:len(worklist)-1]
			
			// 遍历 n 的支配边界
			for _, d := range s.domFront[n] {
				if !hasAlready[d] {
					// 在 d 的开头插入 Phi 节点
					s.insertPhiAt(d, localIdx)
					hasAlready[d] = true
					
					if !everOnWorklist[d] {
						everOnWorklist[d] = true
						worklist = append(worklist, d)
					}
				}
			}
		}
	}
}

// insertPhiAt 在指定块中为指定变量插入 Phi 节点
func (s *SSABuilder) insertPhiAt(block *IRBlock, localIdx int) {
	// 创建 Phi 节点
	dest := s.fn.NewValue(TypeUnknown)
	phi := NewInstr(OpPhi, dest)
	phi.LocalIdx = localIdx
	phi.Block = block
	
	// Phi 节点的参数数量等于前驱块数量
	// 参数值稍后在重命名阶段填充
	phi.Args = make([]*IRValue, len(block.Preds))
	phi.Targets = make([]*IRBlock, len(block.Preds))
	for i, pred := range block.Preds {
		phi.Targets[i] = pred
		// Args[i] 将在重命名阶段设置
	}
	
	// 在块的开头插入 Phi 节点
	block.Instrs = append([]*IRInstr{phi}, block.Instrs...)
}

// ============================================================================
// 变量重命名
// ============================================================================

// renameVariables 重命名变量使其符合 SSA 形式
func (s *SSABuilder) renameVariables() {
	// 初始化变量栈
	for i := 0; i < s.fn.LocalCount; i++ {
		s.varStack[i] = make([]*IRValue, 0)
		s.varCount[i] = 0
	}
	
	// 从入口块开始 DFS
	s.renameBlock(s.fn.Entry)
}

// renameBlock 重命名单个块中的变量
func (s *SSABuilder) renameBlock(block *IRBlock) {
	// 记录每个变量在进入此块前栈的大小（用于恢复）
	stackSizes := make(map[int]int)
	for i := range s.varStack {
		stackSizes[i] = len(s.varStack[i])
	}
	
	// 处理 Phi 节点（作为定义）
	for _, instr := range block.Instrs {
		if instr.Op == OpPhi {
			localIdx := instr.LocalIdx
			// Phi 节点的目标是一个新的定义
			s.pushDef(localIdx, instr.Dest)
		}
	}
	
	// 处理其他指令
	for _, instr := range block.Instrs {
		if instr.Op == OpPhi {
			continue // Phi 节点已处理
		}
		
		switch instr.Op {
		case OpLoadLocal:
			// 使用最新的定义
			localIdx := instr.LocalIdx
			if def := s.currentDef(localIdx); def != nil {
				// 替换 LoadLocal 的结果为当前定义
				// 实际上我们应该将此指令的使用点更新为使用 def
				// 但这里简化处理：保持 LoadLocal 但记录对应关系
				instr.Args = []*IRValue{def}
			}
			
		case OpStoreLocal:
			// 这是一个新的定义
			localIdx := instr.LocalIdx
			if len(instr.Args) > 0 {
				s.pushDef(localIdx, instr.Args[0])
			}
		}
	}
	
	// 填充后继块中 Phi 节点的参数
	for _, succ := range block.Succs {
		// 找到 block 在 succ.Preds 中的索引
		predIdx := -1
		for i, pred := range succ.Preds {
			if pred == block {
				predIdx = i
				break
			}
		}
		
		if predIdx == -1 {
			continue
		}
		
		// 更新 succ 中所有 Phi 节点的对应参数
		for _, instr := range succ.Instrs {
			if instr.Op != OpPhi {
				break // Phi 节点总是在块的开头
			}
			
			localIdx := instr.LocalIdx
			def := s.currentDef(localIdx)
			if def == nil {
				// 没有定义，使用 0 值
				def = s.fn.NewConstIntValue(0)
			}
			
			instr.Args[predIdx] = def
			def.AddUse(instr)
		}
	}
	
	// 递归处理支配树中的子节点
	for _, child := range s.domTree[block] {
		s.renameBlock(child)
	}
	
	// 恢复变量栈
	for i := range s.varStack {
		s.varStack[i] = s.varStack[i][:stackSizes[i]]
	}
}

// pushDef 将新定义压入变量栈
func (s *SSABuilder) pushDef(localIdx int, def *IRValue) {
	s.varStack[localIdx] = append(s.varStack[localIdx], def)
}

// currentDef 获取变量的当前定义
func (s *SSABuilder) currentDef(localIdx int) *IRValue {
	stack := s.varStack[localIdx]
	if len(stack) == 0 {
		return nil
	}
	return stack[len(stack)-1]
}
