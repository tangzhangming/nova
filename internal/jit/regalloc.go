// regalloc.go - 寄存器分配器
//
// 本文件实现了线性扫描寄存器分配算法 (Linear Scan Register Allocation)。
// 这是一种高效的寄存器分配算法，被广泛用于 JIT 编译器中（如 HotSpot）。
//
// 算法概述：
// 1. 计算每个值的活跃区间（从定义到最后使用）
// 2. 按起始位置排序活跃区间
// 3. 线性扫描，为每个区间分配寄存器
// 4. 如果没有可用寄存器，选择一个区间溢出到栈
//
// 时间复杂度：O(n log n)，其中 n 是活跃区间数量
// 空间复杂度：O(n)

package jit

import (
	"sort"
)

// ============================================================================
// 寄存器分配结果
// ============================================================================

// RegAllocation 寄存器分配结果
type RegAllocation struct {
	// ValueRegs 值到寄存器的映射
	// key: 值 ID, value: 物理寄存器编号（-1 表示溢出到栈）
	ValueRegs map[int]int
	
	// SpillSlots 溢出槽分配
	// key: 值 ID, value: 栈槽编号
	SpillSlots map[int]int
	
	// StackSize 所需的栈空间大小（字节）
	StackSize int
	
	// 活跃区间信息（用于调试）
	Intervals []*LiveInterval
}

// GetReg 获取值对应的物理寄存器
// 返回 -1 表示值被溢出到栈
func (alloc *RegAllocation) GetReg(valueID int) int {
	if reg, ok := alloc.ValueRegs[valueID]; ok {
		return reg
	}
	return -1
}

// GetSpillSlot 获取值的溢出槽
// 返回 -1 表示值没有被溢出
func (alloc *RegAllocation) GetSpillSlot(valueID int) int {
	if slot, ok := alloc.SpillSlots[valueID]; ok {
		return slot
	}
	return -1
}

// IsSpilled 检查值是否被溢出
func (alloc *RegAllocation) IsSpilled(valueID int) bool {
	_, ok := alloc.SpillSlots[valueID]
	return ok
}

// ============================================================================
// 活跃区间
// ============================================================================

// LiveInterval 活跃区间
// 表示一个值从定义到最后使用的范围
type LiveInterval struct {
	ValueID  int   // 对应的值 ID
	Start    int   // 开始位置（指令编号）
	End      int   // 结束位置（指令编号）
	Reg      int   // 分配的物理寄存器（-1 表示未分配或溢出）
	SpillSlot int  // 溢出槽（-1 表示未溢出）
	
	// 值的类型信息（用于选择寄存器类）
	IsFloat  bool
	
	// 约束
	FixedReg int   // 固定寄存器约束（-1 表示无约束）
}

// NewLiveInterval 创建活跃区间
func NewLiveInterval(valueID, start int) *LiveInterval {
	return &LiveInterval{
		ValueID:   valueID,
		Start:     start,
		End:       start,
		Reg:       -1,
		SpillSlot: -1,
		FixedReg:  -1,
	}
}

// Extend 扩展区间终点
func (li *LiveInterval) Extend(pos int) {
	if pos > li.End {
		li.End = pos
	}
}

// Overlaps 检查两个区间是否重叠
func (li *LiveInterval) Overlaps(other *LiveInterval) bool {
	return li.Start < other.End && other.Start < li.End
}

// ============================================================================
// 寄存器分配器
// ============================================================================

// RegisterAllocator 寄存器分配器
type RegisterAllocator struct {
	numRegs   int            // 可用寄存器数量
	intervals []*LiveInterval // 所有活跃区间
	active    []*LiveInterval // 当前活跃的区间（按结束位置排序）
	
	// 寄存器状态
	freeRegs  []bool         // 哪些寄存器是空闲的
	usedRegs  []bool         // 哪些寄存器被使用过
	
	// 溢出管理
	nextSpillSlot int
	
	// 结果
	allocation *RegAllocation
}

// NewRegisterAllocator 创建寄存器分配器
func NewRegisterAllocator(numRegs int) *RegisterAllocator {
	if numRegs <= 0 {
		numRegs = 14 // 默认使用 14 个寄存器
	}
	
	return &RegisterAllocator{
		numRegs:  numRegs,
		freeRegs: make([]bool, numRegs),
		usedRegs: make([]bool, numRegs),
	}
}

// Allocate 执行寄存器分配
func (ra *RegisterAllocator) Allocate(fn *IRFunc) *RegAllocation {
	ra.allocation = &RegAllocation{
		ValueRegs:  make(map[int]int),
		SpillSlots: make(map[int]int),
	}
	
	// 初始化所有寄存器为空闲
	for i := range ra.freeRegs {
		ra.freeRegs[i] = true
	}
	
	// 第一步：计算活跃区间
	ra.computeLiveIntervals(fn)
	
	// 第二步：线性扫描分配
	ra.linearScan()
	
	// 计算栈大小
	// 包括：参数保存区 + 溢出槽
	paramSpace := fn.NumArgs * 8
	if paramSpace < 32 {
		paramSpace = 32 // Windows x64 需要至少 32 字节的 shadow space
	}
	spillSpace := ra.nextSpillSlot * 8
	ra.allocation.StackSize = paramSpace + spillSpace
	// 16 字节对齐
	ra.allocation.StackSize = (ra.allocation.StackSize + 15) &^ 15
	
	// 保存活跃区间信息
	ra.allocation.Intervals = ra.intervals
	
	return ra.allocation
}

// ============================================================================
// 活跃区间计算
// ============================================================================

// computeLiveIntervals 计算所有值的活跃区间
func (ra *RegisterAllocator) computeLiveIntervals(fn *IRFunc) {
	intervals := make(map[int]*LiveInterval)
	instrNum := 0
	
	// 遍历所有基本块
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			// 处理目标值（定义）
			if instr.Dest != nil {
				interval, ok := intervals[instr.Dest.ID]
				if !ok {
					interval = NewLiveInterval(instr.Dest.ID, instrNum)
					interval.IsFloat = instr.Dest.Type == TypeFloat
					intervals[instr.Dest.ID] = interval
				}
				interval.Extend(instrNum)
			}
			
			// 处理操作数（使用）
			for _, arg := range instr.Args {
				if arg != nil && !arg.IsConst {
					interval, ok := intervals[arg.ID]
					if !ok {
						// 使用前没有定义（可能是函数参数）
						interval = NewLiveInterval(arg.ID, 0)
						interval.IsFloat = arg.Type == TypeFloat
						intervals[arg.ID] = interval
					}
					interval.Extend(instrNum)
				}
			}
			
			instrNum++
		}
	}
	
	// 转换为切片并排序
	ra.intervals = make([]*LiveInterval, 0, len(intervals))
	for _, interval := range intervals {
		ra.intervals = append(ra.intervals, interval)
	}
	
	// 按起始位置排序
	sort.Slice(ra.intervals, func(i, j int) bool {
		return ra.intervals[i].Start < ra.intervals[j].Start
	})
}

// ============================================================================
// 线性扫描
// ============================================================================

// linearScan 线性扫描分配算法
func (ra *RegisterAllocator) linearScan() {
	ra.active = make([]*LiveInterval, 0)
	
	for _, current := range ra.intervals {
		// 释放已经过期的区间
		ra.expireOldIntervals(current)
		
		// 尝试分配寄存器
		if len(ra.active) >= ra.numRegs {
			// 没有空闲寄存器，需要溢出
			ra.spillAtInterval(current)
		} else {
			// 分配一个空闲寄存器
			reg := ra.allocateFreeReg(current)
			if reg >= 0 {
				current.Reg = reg
				ra.allocation.ValueRegs[current.ValueID] = reg
				ra.addToActive(current)
			} else {
				// 没有空闲寄存器
				ra.spillAtInterval(current)
			}
		}
	}
}

// expireOldIntervals 释放已经结束的区间
func (ra *RegisterAllocator) expireOldIntervals(current *LiveInterval) {
	newActive := make([]*LiveInterval, 0, len(ra.active))
	
	for _, active := range ra.active {
		if active.End <= current.Start {
			// 区间已结束，释放寄存器
			if active.Reg >= 0 {
				ra.freeRegs[active.Reg] = true
			}
		} else {
			newActive = append(newActive, active)
		}
	}
	
	ra.active = newActive
}

// allocateFreeReg 分配一个空闲寄存器
func (ra *RegisterAllocator) allocateFreeReg(interval *LiveInterval) int {
	// 如果有固定寄存器约束
	if interval.FixedReg >= 0 && interval.FixedReg < ra.numRegs {
		if ra.freeRegs[interval.FixedReg] {
			ra.freeRegs[interval.FixedReg] = false
			ra.usedRegs[interval.FixedReg] = true
			return interval.FixedReg
		}
		// 固定寄存器被占用，需要特殊处理
		return -1
	}
	
	// 寻找任意空闲寄存器
	for i := 0; i < ra.numRegs; i++ {
		if ra.freeRegs[i] {
			ra.freeRegs[i] = false
			ra.usedRegs[i] = true
			return i
		}
	}
	
	return -1
}

// spillAtInterval 在当前位置执行溢出
func (ra *RegisterAllocator) spillAtInterval(current *LiveInterval) {
	// 策略：如果活跃区间中有一个结束更晚的区间，溢出它；
	// 否则溢出当前区间
	
	if len(ra.active) == 0 {
		// 没有活跃区间，直接溢出当前
		ra.spillInterval(current)
		return
	}
	
	// 找到结束最晚的活跃区间
	latest := ra.active[len(ra.active)-1]
	
	if latest.End > current.End {
		// 溢出结束更晚的区间，把它的寄存器给当前区间
		current.Reg = latest.Reg
		ra.allocation.ValueRegs[current.ValueID] = current.Reg
		
		// 溢出 latest
		ra.spillInterval(latest)
		
		// 从 active 中移除 latest
		ra.active = ra.active[:len(ra.active)-1]
		
		// 将 current 加入 active
		ra.addToActive(current)
	} else {
		// 溢出当前区间
		ra.spillInterval(current)
	}
}

// spillInterval 溢出一个区间到栈
func (ra *RegisterAllocator) spillInterval(interval *LiveInterval) {
	// 分配溢出槽
	slot := ra.nextSpillSlot
	ra.nextSpillSlot++
	
	interval.SpillSlot = slot
	interval.Reg = -1
	
	ra.allocation.SpillSlots[interval.ValueID] = slot
	delete(ra.allocation.ValueRegs, interval.ValueID)
}

// addToActive 将区间加入活跃列表（保持按结束位置排序）
func (ra *RegisterAllocator) addToActive(interval *LiveInterval) {
	// 二分查找插入位置
	i := sort.Search(len(ra.active), func(i int) bool {
		return ra.active[i].End >= interval.End
	})
	
	// 插入
	ra.active = append(ra.active, nil)
	copy(ra.active[i+1:], ra.active[i:])
	ra.active[i] = interval
}
