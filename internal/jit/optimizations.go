// optimizations.go - JIT 高级优化
//
// 本文件实现多种高级优化技术，用于提升 JIT 编译代码的执行效率。
//
// 优化技术：
// 1. 公共子表达式消除 (CSE) - 避免重复计算
// 2. 复制传播 (Copy Propagation) - 传播值以便后续优化
// 3. 条件分支优化 - 基于常量条件的分支简化
// 4. 边界检查消除 (BCE) - 消除冗余的数组边界检查
// 5. 窥孔优化 (Peephole) - 本地指令模式替换
// 6. 循环展开 (Loop Unrolling) - 减少循环开销
// 7. 全局值编号 (GVN) - 更强大的重复计算消除

package jit

import (
	"fmt"
)

// ============================================================================
// 优化统计
// ============================================================================

// OptimizationStats 优化统计信息
type OptimizationStats struct {
	CSEEliminations       int // 公共子表达式消除数
	CopyPropagations      int // 复制传播数
	BranchOptimizations   int // 条件分支优化数
	BoundsCheckEliminated int // 边界检查消除数
	PeepholeOptimizations int // 窥孔优化数
	LoopsUnrolled         int // 循环展开数
	GVNEliminations       int // GVN 消除数
}

// ============================================================================
// 1. 公共子表达式消除 (CSE)
// ============================================================================

// CommonSubexpressionElimination 公共子表达式消除
// 在基本块内查找相同的计算并重用结果
func (opt *Optimizer) CommonSubexpressionElimination(fn *IRFunc) bool {
	changed := false
	
	for _, block := range fn.Blocks {
		// 用于存储已见过的表达式：expr -> value
		exprMap := make(map[string]*IRValue)
		
		for i, instr := range block.Instrs {
			if instr.Op == OpNop {
				continue
			}
			
			// 跳过有副作用的指令
			if opt.hasSideEffect(instr) {
				continue
			}
			
			// 生成表达式的唯一键
			key := opt.exprKey(instr)
			if key == "" {
				continue
			}
			
			// 检查是否已经计算过相同的表达式
			if existing, found := exprMap[key]; found {
				// 找到公共子表达式，替换
				if instr.Dest != nil && existing != nil {
					opt.replaceAllUses(fn, instr.Dest, existing)
					block.Instrs[i].Op = OpNop
					block.Instrs[i].Dest = nil
					block.Instrs[i].Args = nil
					changed = true
				}
			} else {
				// 记录新表达式
				if instr.Dest != nil {
					exprMap[key] = instr.Dest
				}
			}
		}
	}
	
	return changed
}

// exprKey 生成表达式的唯一键
func (opt *Optimizer) exprKey(instr *IRInstr) string {
	switch instr.Op {
	case OpAdd, OpMul, OpBitAnd, OpBitOr, OpBitXor, OpEq, OpNe:
		// 可交换操作：对操作数排序
		if len(instr.Args) == 2 && instr.Args[0] != nil && instr.Args[1] != nil {
			arg0 := fmt.Sprintf("%d", instr.Args[0].ID)
			arg1 := fmt.Sprintf("%d", instr.Args[1].ID)
			if arg0 > arg1 {
				arg0, arg1 = arg1, arg0
			}
			return fmt.Sprintf("%s:%s:%s", instr.Op, arg0, arg1)
		}
	case OpSub, OpDiv, OpMod, OpLt, OpLe, OpGt, OpGe, OpShl, OpShr:
		// 不可交换操作
		if len(instr.Args) == 2 && instr.Args[0] != nil && instr.Args[1] != nil {
			return fmt.Sprintf("%s:%d:%d", instr.Op, instr.Args[0].ID, instr.Args[1].ID)
		}
	case OpNeg, OpNot, OpBitNot:
		// 一元操作
		if len(instr.Args) == 1 && instr.Args[0] != nil {
			return fmt.Sprintf("%s:%d", instr.Op, instr.Args[0].ID)
		}
	case OpLoadLocal:
		// 局部变量加载
		return fmt.Sprintf("%s:%d", instr.Op, instr.LocalIdx)
	}
	return ""
}

// replaceAllUses 替换所有使用
func (opt *Optimizer) replaceAllUses(fn *IRFunc, old, new *IRValue) {
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			for i, arg := range instr.Args {
				if arg == old {
					instr.Args[i] = new
				}
			}
		}
	}
}

// ============================================================================
// 2. 复制传播 (Copy Propagation)
// ============================================================================

// CopyPropagation 复制传播
// 当一个变量只是另一个变量的复制时，传播原始值
func (opt *Optimizer) CopyPropagation(fn *IRFunc) bool {
	changed := false
	
	// 构建复制映射：dest -> source
	copyMap := make(map[*IRValue]*IRValue)
	
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			// 检测简单的复制指令
			if opt.isCopyInstruction(instr) && len(instr.Args) > 0 && instr.Args[0] != nil {
				source := instr.Args[0]
				// 追踪复制链
				for {
					if mapped, ok := copyMap[source]; ok {
						source = mapped
					} else {
						break
					}
				}
				if instr.Dest != nil && source != nil {
					copyMap[instr.Dest] = source
				}
			}
		}
	}
	
	// 传播复制
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			for i, arg := range instr.Args {
				if arg != nil {
					if source, ok := copyMap[arg]; ok && source != nil {
						instr.Args[i] = source
						changed = true
					}
				}
			}
		}
	}
	
	return changed
}

// isCopyInstruction 检查是否是复制指令
func (opt *Optimizer) isCopyInstruction(instr *IRInstr) bool {
	// 只有当有参数且参数表示源值时才是复制指令
	switch instr.Op {
	case OpLoadLocal:
		// 加载局部变量不是复制（没有源参数）
		return false
	}
	// 检查是否是简单的复制模式：dest = arg[0]
	if len(instr.Args) == 1 && instr.Dest != nil && instr.Args[0] != nil {
		// 某些单参数指令是复制
		switch instr.Op {
		case OpIntToFloat, OpFloatToInt, OpBoolToInt:
			// 类型转换可以被追踪
			return true
		}
	}
	return false
}

// ============================================================================
// 3. 条件分支优化
// ============================================================================

// ConditionalBranchOptimization 条件分支优化
// 如果条件是常量，直接跳转到相应的目标
func (opt *Optimizer) ConditionalBranchOptimization(fn *IRFunc) bool {
	changed := false
	
	for _, block := range fn.Blocks {
		lastInstr := block.LastInstr()
		if lastInstr == nil || lastInstr.Op != OpBranch {
			continue
		}
		
		// 检查条件是否是常量
		if len(lastInstr.Args) > 0 && lastInstr.Args[0] != nil && lastInstr.Args[0].IsConst {
			cond := lastInstr.Args[0]
			
			var target *IRBlock
			if cond.Type == TypeBool {
				if cond.ConstVal.AsBool() {
					// 条件为 true，跳转到 true 分支
					if len(lastInstr.Targets) > 0 {
						target = lastInstr.Targets[0]
					}
				} else {
					// 条件为 false，跳转到 false 分支
					if len(lastInstr.Targets) > 1 {
						target = lastInstr.Targets[1]
					}
				}
			} else if cond.Type == TypeInt {
				if cond.ConstVal.AsInt() != 0 {
					if len(lastInstr.Targets) > 0 {
						target = lastInstr.Targets[0]
					}
				} else {
					if len(lastInstr.Targets) > 1 {
						target = lastInstr.Targets[1]
					}
				}
			}
			
			if target != nil {
				// 替换为无条件跳转
				lastInstr.Op = OpJump
				lastInstr.Args = nil
				lastInstr.Targets = []*IRBlock{target}
				changed = true
			}
		}
	}
	
	// 简化连续的无条件跳转
	changed = opt.simplifyJumpChains(fn) || changed
	
	return changed
}

// simplifyJumpChains 简化跳转链
func (opt *Optimizer) simplifyJumpChains(fn *IRFunc) bool {
	changed := false
	
	for _, block := range fn.Blocks {
		lastInstr := block.LastInstr()
		if lastInstr == nil || lastInstr.Op != OpJump {
			continue
		}
		
		if len(lastInstr.Targets) != 1 {
			continue
		}
		
		target := lastInstr.Targets[0]
		
		// 检查目标块是否只有一个跳转指令
		if len(target.Instrs) == 1 && target.Instrs[0].Op == OpJump {
			if len(target.Instrs[0].Targets) == 1 {
				// 跳过中间块，直接跳转到最终目标
				lastInstr.Targets[0] = target.Instrs[0].Targets[0]
				changed = true
			}
		}
	}
	
	return changed
}

// ============================================================================
// 4. 边界检查消除 (BCE)
// ============================================================================

// BoundsCheckElimination 边界检查消除
// 在已知安全的情况下消除数组边界检查
func (opt *Optimizer) BoundsCheckElimination(fn *IRFunc) bool {
	changed := false
	
	// 分析每个基本块中的数组访问
	for _, block := range fn.Blocks {
		// 追踪已知安全的索引范围
		safeIndices := make(map[*IRValue]*IndexRange)
		
		for i, instr := range block.Instrs {
			switch instr.Op {
			case OpArrayBoundsCheck:
				// 检查是否可以消除边界检查
				if len(instr.Args) >= 2 {
					index := instr.Args[0]
					length := instr.Args[1]
					
					// 检查是否已知安全
					if opt.isIndexSafe(index, length, safeIndices) {
						// 消除边界检查
						block.Instrs[i].Op = OpNop
						block.Instrs[i].Dest = nil
						block.Instrs[i].Args = nil
						changed = true
					} else {
						// 记录此检查后索引是安全的
						if index != nil {
							safeIndices[index] = &IndexRange{MinVal: 0, MaxValue: length}
						}
					}
				}
				
			case OpLt, OpLe:
				// 比较操作可能建立索引的安全范围
				if len(instr.Args) == 2 && instr.Args[0] != nil {
					// index < length 成立后，index 是安全的
					safeIndices[instr.Args[0]] = &IndexRange{MinVal: 0, MaxValue: instr.Args[1]}
				}
			}
		}
	}
	
	return changed
}

// IndexRange 索引范围
type IndexRange struct {
	MinVal   int64     // 最小值
	MaxVal   int64     // 最大值
	MaxValue *IRValue  // 最大值（不含）作为 IR 值
}

// isIndexSafe 检查索引是否已知安全
func (opt *Optimizer) isIndexSafe(index, length *IRValue, safeIndices map[*IRValue]*IndexRange) bool {
	if index == nil {
		return false
	}
	
	// 检查是否是常量 0（总是安全的，只要数组非空）
	if index.IsConst && index.Type == TypeInt && index.ConstVal.AsInt() == 0 {
		// 需要确认数组长度 > 0
		if length != nil && length.IsConst && length.Type == TypeInt && length.ConstVal.AsInt() > 0 {
			return true
		}
	}
	
	// 检查是否已经被验证过
	if _, ok := safeIndices[index]; ok {
		return true
	}
	
	return false
}

// ============================================================================
// 5. 窥孔优化 (Peephole Optimization)
// ============================================================================

// PeepholeOptimization 窥孔优化
// 查找并替换低效的指令模式
func (opt *Optimizer) PeepholeOptimization(fn *IRFunc) bool {
	changed := false
	
	for _, block := range fn.Blocks {
		// 模式1: 双重否定消除
		changed = opt.eliminateDoubleNegation(block) || changed
		
		// 模式2: 加零消除
		changed = opt.eliminateAddZero(block) || changed
		
		// 模式3: 乘1消除
		changed = opt.eliminateMultiplyOne(block) || changed
		
		// 模式4: 连续移位合并
		changed = opt.mergeConsecutiveShifts(block) || changed
		
		// 模式5: 冗余加载消除
		changed = opt.eliminateRedundantLoads(block) || changed
		
		// 模式6: 减法转加法（减负数）
		changed = opt.convertSubNegToAdd(block) || changed
		
		// 模式7: 位与掩码优化
		changed = opt.optimizeBitMask(block) || changed
	}
	
	return changed
}

// eliminateDoubleNegation 消除双重否定 (!!x -> x, --x -> x)
func (opt *Optimizer) eliminateDoubleNegation(block *IRBlock) bool {
	changed := false
	
	for i, instr := range block.Instrs {
		if (instr.Op == OpNot || instr.Op == OpNeg) && len(instr.Args) == 1 {
			arg := instr.Args[0]
			if arg != nil && arg.Def != nil && arg.Def.Op == instr.Op {
				// 找到双重否定
				if len(arg.Def.Args) == 1 && arg.Def.Args[0] != nil {
					// 替换为原始值
					original := arg.Def.Args[0]
					opt.replaceWith(instr, original)
					block.Instrs[i].Op = OpNop
					changed = true
				}
			}
		}
	}
	
	return changed
}

// eliminateAddZero 消除加零 (x + 0 -> x, 0 + x -> x)
func (opt *Optimizer) eliminateAddZero(block *IRBlock) bool {
	changed := false
	
	for i, instr := range block.Instrs {
		if instr.Op == OpAdd && len(instr.Args) == 2 {
			if opt.isZero(instr.Args[0]) {
				opt.replaceWith(instr, instr.Args[1])
				block.Instrs[i].Op = OpNop
				changed = true
			} else if opt.isZero(instr.Args[1]) {
				opt.replaceWith(instr, instr.Args[0])
				block.Instrs[i].Op = OpNop
				changed = true
			}
		}
	}
	
	return changed
}

// eliminateMultiplyOne 消除乘1 (x * 1 -> x, 1 * x -> x)
func (opt *Optimizer) eliminateMultiplyOne(block *IRBlock) bool {
	changed := false
	
	for i, instr := range block.Instrs {
		if instr.Op == OpMul && len(instr.Args) == 2 {
			if opt.isOne(instr.Args[0]) {
				opt.replaceWith(instr, instr.Args[1])
				block.Instrs[i].Op = OpNop
				changed = true
			} else if opt.isOne(instr.Args[1]) {
				opt.replaceWith(instr, instr.Args[0])
				block.Instrs[i].Op = OpNop
				changed = true
			}
		}
	}
	
	return changed
}

// mergeConsecutiveShifts 合并连续移位 (x << a) << b -> x << (a+b)
func (opt *Optimizer) mergeConsecutiveShifts(block *IRBlock) bool {
	changed := false
	
	for _, instr := range block.Instrs {
		if (instr.Op == OpShl || instr.Op == OpShr) && len(instr.Args) == 2 {
			arg := instr.Args[0]
			shift1 := instr.Args[1]
			
			if arg != nil && arg.Def != nil && arg.Def.Op == instr.Op {
				// 找到连续移位
				if len(arg.Def.Args) == 2 {
					shift2 := arg.Def.Args[1]
					
					// 两个移位量都是常量
					if shift1 != nil && shift1.IsConst && shift2 != nil && shift2.IsConst {
						totalShift := shift1.ConstVal.AsInt() + shift2.ConstVal.AsInt()
						// 确保移位量合理
						if totalShift >= 0 && totalShift < 64 {
							// 合并移位
							instr.Args[0] = arg.Def.Args[0]
							instr.Args[1] = NewConstInt(-1, totalShift)
							changed = true
						}
					}
				}
			}
		}
	}
	
	return changed
}

// eliminateRedundantLoads 消除冗余加载
func (opt *Optimizer) eliminateRedundantLoads(block *IRBlock) bool {
	changed := false
	
	// 追踪已加载的局部变量
	loadedLocals := make(map[int]*IRValue)
	
	for i, instr := range block.Instrs {
		switch instr.Op {
		case OpLoadLocal:
			localIdx := instr.LocalIdx
			if existing, ok := loadedLocals[localIdx]; ok {
				// 已经加载过，重用
				if instr.Dest != nil {
					opt.replaceAllUsesInBlock(block, instr.Dest, existing)
					block.Instrs[i].Op = OpNop
					block.Instrs[i].Dest = nil
					block.Instrs[i].Args = nil
					changed = true
				}
			} else {
				// 记录加载
				if instr.Dest != nil {
					loadedLocals[localIdx] = instr.Dest
				}
			}
			
		case OpStoreLocal:
			// 存储使之前的加载失效
			localIdx := instr.LocalIdx
			delete(loadedLocals, localIdx)
			// 存储的值可以作为新的"已加载"值
			if len(instr.Args) > 0 && instr.Args[0] != nil {
				loadedLocals[localIdx] = instr.Args[0]
			}
			
		case OpCall, OpCallDirect, OpCallIndirect, OpCallMethod:
			// 函数调用可能修改任何局部变量，清空缓存
			loadedLocals = make(map[int]*IRValue)
		}
	}
	
	return changed
}

// replaceAllUsesInBlock 在基本块内替换所有使用
func (opt *Optimizer) replaceAllUsesInBlock(block *IRBlock, old, new *IRValue) {
	for _, instr := range block.Instrs {
		for i, arg := range instr.Args {
			if arg == old {
				instr.Args[i] = new
			}
		}
	}
}

// convertSubNegToAdd 将减负数转为加法 (x - (-y) -> x + y)
func (opt *Optimizer) convertSubNegToAdd(block *IRBlock) bool {
	changed := false
	
	for _, instr := range block.Instrs {
		if instr.Op == OpSub && len(instr.Args) == 2 {
			right := instr.Args[1]
			
			// 检查右操作数是否是负数常量
			if right != nil && right.IsConst && right.Type == TypeInt {
				val := right.ConstVal.AsInt()
				if val < 0 {
					// x - (-y) -> x + y
					instr.Op = OpAdd
					instr.Args[1] = NewConstInt(-1, -val)
					changed = true
				}
			}
			
			// 检查右操作数是否是取负操作
			if right != nil && right.Def != nil && right.Def.Op == OpNeg {
				if len(right.Def.Args) == 1 && right.Def.Args[0] != nil {
					// x - neg(y) -> x + y
					instr.Op = OpAdd
					instr.Args[1] = right.Def.Args[0]
					changed = true
				}
			}
		}
	}
	
	return changed
}

// optimizeBitMask 优化位掩码操作
func (opt *Optimizer) optimizeBitMask(block *IRBlock) bool {
	changed := false
	
	for _, instr := range block.Instrs {
		if instr.Op == OpBitAnd && len(instr.Args) == 2 {
			// x & 0xFFFFFFFF -> x (对于32位值)
			// x & 0 -> 0
			// x & (-1) -> x
			
			right := instr.Args[1]
			if right != nil && right.IsConst && right.Type == TypeInt {
				val := right.ConstVal.AsInt()
				
				if val == 0 {
					// x & 0 -> 0
					instr.Op = OpConst
					instr.Dest.IsConst = true
					instr.Dest.ConstVal = NewConstInt(-1, 0).ConstVal
					instr.Dest.Type = TypeInt
					instr.Args = nil
					changed = true
				} else if val == -1 {
					// x & -1 -> x
					opt.replaceWith(instr, instr.Args[0])
					instr.Op = OpNop
					changed = true
				}
			}
		}
	}
	
	return changed
}

// ============================================================================
// 6. 循环展开 (Loop Unrolling)
// ============================================================================

// LoopUnrolling 循环展开
// 对小循环进行展开以减少循环开销
func (opt *Optimizer) LoopUnrolling(fn *IRFunc) bool {
	changed := false
	
	// 检测循环
	loops := opt.detectLoops(fn)
	
	for _, loop := range loops {
		// 检查是否适合展开
		if opt.shouldUnrollLoop(loop) {
			if opt.unrollLoop(fn, loop) {
				changed = true
			}
		}
	}
	
	return changed
}

// LoopInfo 循环信息
type LoopInfo struct {
	Header    *IRBlock   // 循环头
	Body      []*IRBlock // 循环体
	Exit      *IRBlock   // 循环出口
	BackEdge  *IRBlock   // 回边块
	TripCount int        // 迭代次数（如果已知）
}

// detectLoops 检测函数中的循环
func (opt *Optimizer) detectLoops(fn *IRFunc) []*LoopInfo {
	var loops []*LoopInfo
	
	// 简单的循环检测：查找回边
	for _, block := range fn.Blocks {
		lastInstr := block.LastInstr()
		if lastInstr == nil {
			continue
		}
		
		if lastInstr.Op == OpJump && len(lastInstr.Targets) == 1 {
			target := lastInstr.Targets[0]
			// 如果目标块在当前块之前，这是一个回边
			if target.ID < block.ID {
				loop := &LoopInfo{
					Header:   target,
					BackEdge: block,
				}
				
				// 收集循环体
				for _, b := range fn.Blocks {
					if b.ID >= target.ID && b.ID <= block.ID {
						loop.Body = append(loop.Body, b)
					}
				}
				
				loops = append(loops, loop)
			}
		}
	}
	
	return loops
}

// shouldUnrollLoop 检查是否应该展开循环
func (opt *Optimizer) shouldUnrollLoop(loop *LoopInfo) bool {
	// 循环体太大不展开
	totalInstrs := 0
	for _, block := range loop.Body {
		totalInstrs += len(block.Instrs)
	}
	
	if totalInstrs > 20 {
		return false
	}
	
	// 如果迭代次数已知且较小，可以完全展开
	if loop.TripCount > 0 && loop.TripCount <= 4 {
		return true
	}
	
	// 对于小循环体，部分展开
	if totalInstrs <= 5 {
		return true
	}
	
	return false
}

// unrollLoop 展开循环
func (opt *Optimizer) unrollLoop(fn *IRFunc, loop *LoopInfo) bool {
	// 简化实现：标记循环已被识别用于优化
	// 完整实现需要：
	// 1. 复制循环体
	// 2. 更新 phi 节点
	// 3. 调整控制流
	
	// 目前只对循环体设置较高的优先级用于后续优化
	for _, block := range loop.Body {
		block.LoopDepth = 1
	}
	
	return false // 实际展开未实现
}

// ============================================================================
// 7. 全局值编号 (GVN)
// ============================================================================

// GlobalValueNumbering 全局值编号
// 跨基本块的公共子表达式消除
func (opt *Optimizer) GlobalValueNumbering(fn *IRFunc) bool {
	changed := false
	
	// 为每个值分配唯一编号
	valueNumbers := make(map[*IRValue]int)
	exprNumbers := make(map[string]int)
	nextNumber := 1
	
	// 第一遍：为常量和参数分配编号
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			if instr.Dest != nil {
				if instr.Op == OpConst {
					// 相同的常量获得相同的编号
					key := fmt.Sprintf("const:%s:%v", instr.Dest.Type, instr.Dest.ConstVal)
					if num, ok := exprNumbers[key]; ok {
						valueNumbers[instr.Dest] = num
					} else {
						exprNumbers[key] = nextNumber
						valueNumbers[instr.Dest] = nextNumber
						nextNumber++
					}
				} else {
					// 非常量，分配新编号
					valueNumbers[instr.Dest] = nextNumber
					nextNumber++
				}
			}
		}
	}
	
	// 第二遍：基于操作数编号计算表达式编号
	for _, block := range fn.Blocks {
		for i, instr := range block.Instrs {
			if instr.Dest == nil || instr.Op == OpConst {
				continue
			}
			
			// 生成基于值编号的表达式键
			key := opt.gvnExprKey(instr, valueNumbers)
			if key == "" {
				continue
			}
			
			if existingNum, ok := exprNumbers[key]; ok {
				// 找到相同的表达式
				// 查找具有此编号的值
				var existing *IRValue
				for v, num := range valueNumbers {
					if num == existingNum && v != instr.Dest {
						existing = v
						break
					}
				}
				
				if existing != nil {
					// 替换为现有值
					opt.replaceAllUses(fn, instr.Dest, existing)
					block.Instrs[i].Op = OpNop
					block.Instrs[i].Dest = nil
					block.Instrs[i].Args = nil
					changed = true
				}
			} else {
				// 新表达式，记录编号
				exprNumbers[key] = valueNumbers[instr.Dest]
			}
		}
	}
	
	return changed
}

// gvnExprKey 生成基于值编号的表达式键
func (opt *Optimizer) gvnExprKey(instr *IRInstr, valueNumbers map[*IRValue]int) string {
	switch instr.Op {
	case OpAdd, OpMul, OpBitAnd, OpBitOr, OpBitXor, OpEq, OpNe:
		// 可交换操作
		if len(instr.Args) == 2 && instr.Args[0] != nil && instr.Args[1] != nil {
			num0, ok0 := valueNumbers[instr.Args[0]]
			num1, ok1 := valueNumbers[instr.Args[1]]
			if ok0 && ok1 {
				if num0 > num1 {
					num0, num1 = num1, num0
				}
				return fmt.Sprintf("%s:%d:%d", instr.Op, num0, num1)
			}
		}
	case OpSub, OpDiv, OpMod, OpLt, OpLe, OpGt, OpGe, OpShl, OpShr:
		// 不可交换操作
		if len(instr.Args) == 2 && instr.Args[0] != nil && instr.Args[1] != nil {
			num0, ok0 := valueNumbers[instr.Args[0]]
			num1, ok1 := valueNumbers[instr.Args[1]]
			if ok0 && ok1 {
				return fmt.Sprintf("%s:%d:%d", instr.Op, num0, num1)
			}
		}
	case OpNeg, OpNot, OpBitNot:
		// 一元操作
		if len(instr.Args) == 1 && instr.Args[0] != nil {
			if num, ok := valueNumbers[instr.Args[0]]; ok {
				return fmt.Sprintf("%s:%d", instr.Op, num)
			}
		}
	}
	return ""
}

// ============================================================================
// 集成到优化器
// ============================================================================

// RunAdvancedOptimizations 运行高级优化
func (opt *Optimizer) RunAdvancedOptimizations(fn *IRFunc) *OptimizationStats {
	stats := &OptimizationStats{}
	
	if opt.level < 2 {
		return stats
	}
	
	// O2 优化
	if opt.CommonSubexpressionElimination(fn) {
		stats.CSEEliminations++
	}
	
	if opt.CopyPropagation(fn) {
		stats.CopyPropagations++
	}
	
	if opt.ConditionalBranchOptimization(fn) {
		stats.BranchOptimizations++
	}
	
	if opt.PeepholeOptimization(fn) {
		stats.PeepholeOptimizations++
	}
	
	// O3 优化
	if opt.level >= 3 {
		if opt.BoundsCheckElimination(fn) {
			stats.BoundsCheckEliminated++
		}
		
		if opt.LoopUnrolling(fn) {
			stats.LoopsUnrolled++
		}
		
		if opt.GlobalValueNumbering(fn) {
			stats.GVNEliminations++
		}
	}
	
	return stats
}
