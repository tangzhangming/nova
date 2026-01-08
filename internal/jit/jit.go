// Package jit 实现 JIT 编译器
package jit

import (
	"fmt"
	"runtime"
	"sync"

	"github.com/tangzhangming/nova/internal/bytecode"
	"github.com/tangzhangming/nova/internal/jit/platform"
	"github.com/tangzhangming/nova/internal/jit/types"
)

// ============================================================================
// JIT 编译器
// ============================================================================

// JITCompiler JIT 编译器
type JITCompiler struct {
	codeCache *CodeCache
	platform  string
	codeGen   types.CodeGenerator
	mutex     sync.Mutex
}

// NewJITCompiler 创建 JIT 编译器
func NewJITCompiler() *JITCompiler {
	arch := runtime.GOARCH
	
	var codeGen types.CodeGenerator
	switch arch {
	case "amd64":
		codeGen = platform.NewX64CodeGenerator()
	case "arm64":
		codeGen = platform.NewARM64CodeGenerator()
	default:
		return nil
	}
	
	return &JITCompiler{
		codeCache: NewCodeCache(16 * 1024 * 1024), // 16MB
		platform:  arch,
		codeGen:   codeGen,
	}
}

// CompileFunction 编译热点函数
func (jc *JITCompiler) CompileFunction(fn *bytecode.Function) (*CompiledFunction, error) {
	jc.mutex.Lock()
	defer jc.mutex.Unlock()
	
	// 检查缓存
	if compiled, ok := jc.codeCache.Get(fn.Name); ok {
		return compiled, nil
	}
	
	// 第一步：字节码到 IR
	builder := NewIRBuilder()
	irFn := builder.BuildFunction(fn)
	
	// 第二步：优化
	optimizer := NewOptimizer(irFn)
	optimizer.Optimize()
	
	// 第三步：寄存器分配
	regAlloc := NewRegisterAllocator(irFn)
	regAlloc.Allocate()
	
	// 第四步：代码生成
	machineCode, err := jc.codeGen.GenerateFunction(irFn, regAlloc.ToAllocation())
	if err != nil {
		return nil, err
	}
	
	// 第五步：创建可执行代码
	execCode, err := jc.codeCache.AllocateExecutable(len(machineCode))
	if err != nil {
		return nil, err
	}
	copy(execCode, machineCode)
	
	compiled := &CompiledFunction{
		Name:        fn.Name,
		MachineCode: execCode,
		StackSize:   regAlloc.GetStackSize(),
	}
	
	jc.codeCache.Put(fn.Name, compiled)
	return compiled, nil
}

// GetPlatform 获取平台
func (jc *JITCompiler) GetPlatform() string {
	return jc.platform
}

// CompiledFunction 已编译的函数
type CompiledFunction struct {
	Name        string
	MachineCode []byte
	StackSize   int
}

// ============================================================================
// IR 构建器
// ============================================================================

// IRBuilder IR 构建器
type IRBuilder struct {
	fn        *types.IRFunction
	current   *types.IRBlock
	vregs     int
	ipToBlock map[int]*types.IRBlock
	blockMap  map[*types.IRBlock]int
	stack     []int // 栈模拟
}

// NewIRBuilder 创建 IR 构建器
func NewIRBuilder() *IRBuilder {
	return &IRBuilder{
		ipToBlock: make(map[int]*types.IRBlock),
		blockMap:  make(map[*types.IRBlock]int),
		stack:     make([]int, 0),
	}
}

// BuildFunction 从字节码函数构建 IR
func (b *IRBuilder) BuildFunction(fn *bytecode.Function) *types.IRFunction {
	b.fn = &types.IRFunction{
		Name:       fn.Name,
		Constants:  fn.Chunk.Constants,
		LocalCount: fn.LocalCount,
	}
	
	// 创建入口块
	entry := &types.IRBlock{ID: 0, Entry: true}
	b.fn.Entry = entry
	b.fn.Blocks = append(b.fn.Blocks, entry)
	b.current = entry
	b.ipToBlock[0] = entry
	b.blockMap[entry] = 0
	
	// 识别基本块边界
	blockStarts := b.identifyBasicBlocks(fn.Chunk)
	
	// 创建基本块
	for i, ip := range blockStarts {
		if ip > 0 {
			block := &types.IRBlock{ID: i + 1}
			b.fn.Blocks = append(b.fn.Blocks, block)
			b.ipToBlock[ip] = block
			b.blockMap[block] = ip
		}
	}
	
	// 转换指令
	b.convertInstructions(fn.Chunk)
	
	return b.fn
}

// identifyBasicBlocks 识别基本块边界
func (b *IRBuilder) identifyBasicBlocks(chunk *bytecode.Chunk) []int {
	code := chunk.Code
	starts := make(map[int]bool)
	starts[0] = true
	
	i := 0
	for i < len(code) {
		op := bytecode.OpCode(code[i])
		size := b.instructionSize(op, i, code)
		
		switch op {
		case bytecode.OpJump, bytecode.OpJumpIfFalse, bytecode.OpJumpIfTrue:
			if i+2 < len(code) {
				offset := int(int16(code[i+1])<<8 | int16(code[i+2]))
				target := i + 3 + offset
				if target >= 0 && target < len(code) {
					starts[target] = true
				}
			}
			if i+size < len(code) {
				starts[i+size] = true
			}
		case bytecode.OpLoop:
			if i+2 < len(code) {
				offset := int(code[i+1])<<8 | int(code[i+2])
				target := i + 3 - offset
				if target >= 0 && target < len(code) {
					starts[target] = true
				}
			}
		case bytecode.OpReturn, bytecode.OpReturnNull:
			if i+size < len(code) {
				starts[i+size] = true
			}
		}
		
		i += size
	}
	
	result := make([]int, 0, len(starts))
	for ip := range starts {
		result = append(result, ip)
	}
	return result
}

// convertInstructions 转换指令
func (b *IRBuilder) convertInstructions(chunk *bytecode.Chunk) {
	code := chunk.Code
	i := 0
	
	for i < len(code) {
		// 检查是否需要切换基本块
		if block, ok := b.ipToBlock[i]; ok && block != b.current {
			b.current = block
		}
		
		op := bytecode.OpCode(code[i])
		size := b.instructionSize(op, i, code)
		
		b.convertInstruction(op, i, chunk)
		
		i += size
	}
}

// convertInstruction 转换单条指令
func (b *IRBuilder) convertInstruction(op bytecode.OpCode, ip int, chunk *bytecode.Chunk) {
	switch op {
	case bytecode.OpLoadLocal:
		idx := int(chunk.ReadU16(ip + 1))
		dest := b.newVReg()
		b.current.Instrs = append(b.current.Instrs, &types.IRInstr{
			Op:   types.IRLoadLocal,
			Dest: dest,
			Args: []int{idx},
		})
		b.push(dest)
		
	case bytecode.OpStoreLocal:
		idx := int(chunk.ReadU16(ip + 1))
		src := b.pop()
		b.current.Instrs = append(b.current.Instrs, &types.IRInstr{
			Op:   types.IRStoreLocal,
			Dest: -1,
			Args: []int{src, idx},
		})
		
	case bytecode.OpPush:
		constIdx := chunk.ReadU16(ip + 1)
		if int(constIdx) < len(chunk.Constants) {
			dest := b.newVReg()
			b.current.Instrs = append(b.current.Instrs, &types.IRInstr{
				Op:        types.IRLoadConst,
				Dest:      dest,
				Immediate: chunk.Constants[constIdx],
			})
			b.push(dest)
		}
		
	case bytecode.OpZero:
		dest := b.newVReg()
		b.current.Instrs = append(b.current.Instrs, &types.IRInstr{
			Op:        types.IRLoadConst,
			Dest:      dest,
			Immediate: bytecode.NewInt(0),
		})
		b.push(dest)
		
	case bytecode.OpOne:
		dest := b.newVReg()
		b.current.Instrs = append(b.current.Instrs, &types.IRInstr{
			Op:        types.IRLoadConst,
			Dest:      dest,
			Immediate: bytecode.NewInt(1),
		})
		b.push(dest)
		
	case bytecode.OpTrue:
		dest := b.newVReg()
		b.current.Instrs = append(b.current.Instrs, &types.IRInstr{
			Op:        types.IRLoadConst,
			Dest:      dest,
			Immediate: bytecode.TrueValue,
		})
		b.push(dest)
		
	case bytecode.OpFalse:
		dest := b.newVReg()
		b.current.Instrs = append(b.current.Instrs, &types.IRInstr{
			Op:        types.IRLoadConst,
			Dest:      dest,
			Immediate: bytecode.FalseValue,
		})
		b.push(dest)
		
	case bytecode.OpAdd:
		right := b.pop()
		left := b.pop()
		dest := b.newVReg()
		b.current.Instrs = append(b.current.Instrs, &types.IRInstr{
			Op:   types.IRAdd,
			Dest: dest,
			Args: []int{left, right},
		})
		b.push(dest)
		
	case bytecode.OpSub:
		right := b.pop()
		left := b.pop()
		dest := b.newVReg()
		b.current.Instrs = append(b.current.Instrs, &types.IRInstr{
			Op:   types.IRSub,
			Dest: dest,
			Args: []int{left, right},
		})
		b.push(dest)
		
	case bytecode.OpMul:
		right := b.pop()
		left := b.pop()
		dest := b.newVReg()
		b.current.Instrs = append(b.current.Instrs, &types.IRInstr{
			Op:   types.IRMul,
			Dest: dest,
			Args: []int{left, right},
		})
		b.push(dest)
		
	case bytecode.OpDiv:
		right := b.pop()
		left := b.pop()
		dest := b.newVReg()
		b.current.Instrs = append(b.current.Instrs, &types.IRInstr{
			Op:   types.IRDiv,
			Dest: dest,
			Args: []int{left, right},
		})
		b.push(dest)
		
	case bytecode.OpEq:
		right := b.pop()
		left := b.pop()
		dest := b.newVReg()
		b.current.Instrs = append(b.current.Instrs, &types.IRInstr{
			Op:   types.IREq,
			Dest: dest,
			Args: []int{left, right},
		})
		b.push(dest)
		
	case bytecode.OpLt:
		right := b.pop()
		left := b.pop()
		dest := b.newVReg()
		b.current.Instrs = append(b.current.Instrs, &types.IRInstr{
			Op:   types.IRLt,
			Dest: dest,
			Args: []int{left, right},
		})
		b.push(dest)
		
	case bytecode.OpGt:
		right := b.pop()
		left := b.pop()
		dest := b.newVReg()
		b.current.Instrs = append(b.current.Instrs, &types.IRInstr{
			Op:   types.IRGt,
			Dest: dest,
			Args: []int{left, right},
		})
		b.push(dest)
		
	case bytecode.OpReturn:
		if len(b.stack) > 0 {
			ret := b.pop()
			b.current.Instrs = append(b.current.Instrs, &types.IRInstr{
				Op:   types.IRReturn,
				Dest: -1,
				Args: []int{ret},
			})
		} else {
			b.current.Instrs = append(b.current.Instrs, &types.IRInstr{
				Op:   types.IRReturn,
				Dest: -1,
			})
		}
		b.current.Exit = true
		
	case bytecode.OpReturnNull:
		b.current.Instrs = append(b.current.Instrs, &types.IRInstr{
			Op:   types.IRReturn,
			Dest: -1,
		})
		b.current.Exit = true
		
	case bytecode.OpJump:
		offset := int(int16(chunk.ReadU16(ip + 1)))
		target := ip + 3 + offset
		if targetBlock, ok := b.ipToBlock[target]; ok {
			b.current.Instrs = append(b.current.Instrs, &types.IRInstr{
				Op:        types.IRBranch,
				Dest:      -1,
				Immediate: targetBlock,
			})
			b.current.Succs = append(b.current.Succs, targetBlock)
			targetBlock.Preds = append(targetBlock.Preds, b.current)
		}
		
	case bytecode.OpJumpIfFalse:
		cond := b.pop()
		offset := int(int16(chunk.ReadU16(ip + 1)))
		target := ip + 3 + offset
		targetBlock := b.ipToBlock[target]
		nextBlock := b.ipToBlock[ip+3]
		if targetBlock != nil {
			b.current.Instrs = append(b.current.Instrs, &types.IRInstr{
				Op:        types.IRBranchIf,
				Dest:      -1,
				Args:      []int{cond},
				Immediate: []*types.IRBlock{targetBlock, nextBlock},
			})
			b.current.Succs = append(b.current.Succs, targetBlock, nextBlock)
			targetBlock.Preds = append(targetBlock.Preds, b.current)
			if nextBlock != nil {
				nextBlock.Preds = append(nextBlock.Preds, b.current)
			}
		}
	}
}

func (b *IRBuilder) newVReg() int {
	v := b.vregs
	b.vregs++
	if b.fn.NumVRegs < b.vregs {
		b.fn.NumVRegs = b.vregs
	}
	return v
}

func (b *IRBuilder) push(v int) { b.stack = append(b.stack, v) }

func (b *IRBuilder) pop() int {
	if len(b.stack) == 0 {
		return -1
	}
	v := b.stack[len(b.stack)-1]
	b.stack = b.stack[:len(b.stack)-1]
	return v
}

func (b *IRBuilder) instructionSize(op bytecode.OpCode, offset int, code []byte) int {
	switch op {
	case bytecode.OpPush, bytecode.OpLoadLocal, bytecode.OpStoreLocal,
		bytecode.OpLoadGlobal, bytecode.OpStoreGlobal,
		bytecode.OpNewObject, bytecode.OpGetField, bytecode.OpSetField,
		bytecode.OpNewArray, bytecode.OpNewMap,
		bytecode.OpCheckType, bytecode.OpCast, bytecode.OpCastSafe:
		return 3
	case bytecode.OpNewFixedArray:
		return 5
	case bytecode.OpJump, bytecode.OpJumpIfFalse, bytecode.OpJumpIfTrue, bytecode.OpLoop:
		return 3
	case bytecode.OpCall, bytecode.OpTailCall:
		return 2
	case bytecode.OpCallMethod:
		return 4
	case bytecode.OpGetStatic, bytecode.OpSetStatic:
		return 5
	case bytecode.OpCallStatic:
		return 6
	default:
		return 1
	}
}

// ============================================================================
// 优化器
// ============================================================================

// Optimizer IR 优化器
type Optimizer struct {
	fn *types.IRFunction
}

// NewOptimizer 创建优化器
func NewOptimizer(fn *types.IRFunction) *Optimizer {
	return &Optimizer{fn: fn}
}

// Optimize 执行优化
func (o *Optimizer) Optimize() {
	o.constantFolding()
	o.deadCodeElimination()
}

// constantFolding 常量折叠
func (o *Optimizer) constantFolding() {
	for _, block := range o.fn.Blocks {
		for _, instr := range block.Instrs {
			if o.canFold(instr) {
				o.foldConstant(instr)
			}
		}
	}
}

func (o *Optimizer) canFold(instr *types.IRInstr) bool {
	switch instr.Op {
	case types.IRAdd, types.IRSub, types.IRMul, types.IRDiv:
		return len(instr.Args) >= 2 && o.isConstant(instr.Args[0]) && o.isConstant(instr.Args[1])
	}
	return false
}

func (o *Optimizer) isConstant(vreg int) bool {
	for _, block := range o.fn.Blocks {
		for _, instr := range block.Instrs {
			if instr.Dest == vreg && instr.Op == types.IRLoadConst {
				return true
			}
		}
	}
	return false
}

func (o *Optimizer) foldConstant(instr *types.IRInstr) {
	// 简化：标记为已折叠
}

// deadCodeElimination 死代码消除
func (o *Optimizer) deadCodeElimination() {
	used := make(map[int]bool)
	
	// 标记被使用的寄存器
	for _, block := range o.fn.Blocks {
		for _, instr := range block.Instrs {
			for _, arg := range instr.Args {
				used[arg] = true
			}
		}
	}
	
	// 删除未使用的定义
	for _, block := range o.fn.Blocks {
		newInstrs := make([]*types.IRInstr, 0)
		for _, instr := range block.Instrs {
			if instr.Dest < 0 || used[instr.Dest] || o.hasSideEffect(instr) {
				newInstrs = append(newInstrs, instr)
			}
		}
		block.Instrs = newInstrs
	}
}

func (o *Optimizer) hasSideEffect(instr *types.IRInstr) bool {
	switch instr.Op {
	case types.IRStoreLocal, types.IRStoreGlobal, types.IRCall, types.IRReturn:
		return true
	}
	return false
}

// ============================================================================
// 寄存器分配
// ============================================================================

// RegisterAllocator 寄存器分配器
type RegisterAllocator struct {
	fn            *types.IRFunction
	liveIntervals []*types.LiveInterval
	allocated     map[int]int
	spilled       map[int]int
	nextSlot      int
}

// NewRegisterAllocator 创建寄存器分配器
func NewRegisterAllocator(fn *types.IRFunction) *RegisterAllocator {
	return &RegisterAllocator{
		fn:        fn,
		allocated: make(map[int]int),
		spilled:   make(map[int]int),
	}
}

// Allocate 执行寄存器分配
func (ra *RegisterAllocator) Allocate() {
	ra.computeLiveIntervals()
	ra.linearScan()
}

// computeLiveIntervals 计算活跃区间
func (ra *RegisterAllocator) computeLiveIntervals() {
	intervals := make(map[int]*types.LiveInterval)
	instrNum := 0
	
	for _, block := range ra.fn.Blocks {
		for _, instr := range block.Instrs {
			if instr.Dest >= 0 {
				if _, ok := intervals[instr.Dest]; !ok {
					intervals[instr.Dest] = &types.LiveInterval{
						VReg:  instr.Dest,
						Start: instrNum,
						End:   instrNum,
						PReg:  -1,
						Spill: -1,
					}
				} else {
					intervals[instr.Dest].End = instrNum
				}
			}
			for _, arg := range instr.Args {
				if arg >= 0 {
					if _, ok := intervals[arg]; !ok {
						intervals[arg] = &types.LiveInterval{
							VReg:  arg,
							Start: instrNum,
							End:   instrNum,
							PReg:  -1,
							Spill: -1,
						}
					} else {
						intervals[arg].End = instrNum
					}
				}
			}
			instrNum++
		}
	}
	
	for _, interval := range intervals {
		ra.liveIntervals = append(ra.liveIntervals, interval)
	}
}

// linearScan 线性扫描分配
func (ra *RegisterAllocator) linearScan() {
	// 按开始位置排序
	for i := 1; i < len(ra.liveIntervals); i++ {
		key := ra.liveIntervals[i]
		j := i - 1
		for j >= 0 && ra.liveIntervals[j].Start > key.Start {
			ra.liveIntervals[j+1] = ra.liveIntervals[j]
			j--
		}
		ra.liveIntervals[j+1] = key
	}
	
	// 可用寄存器（保留 RAX 用于临时操作）
	numRegs := 14 // R8-R15, RBX, RCX, RDX, RSI, RDI
	usedRegs := make([]bool, numRegs)
	active := make([]*types.LiveInterval, 0)
	
	for _, interval := range ra.liveIntervals {
		// 过期区间
		newActive := make([]*types.LiveInterval, 0)
		for _, act := range active {
			if act.End < interval.Start {
				if act.PReg >= 0 {
					usedRegs[act.PReg] = false
				}
			} else {
				newActive = append(newActive, act)
			}
		}
		active = newActive
		
		// 尝试分配寄存器
		allocated := false
		for i := 0; i < numRegs; i++ {
			if !usedRegs[i] {
				interval.PReg = i
				usedRegs[i] = true
				ra.allocated[interval.VReg] = i
				active = append(active, interval)
				allocated = true
				break
			}
		}
		
		if !allocated {
			// 溢出
			interval.Spill = ra.nextSlot
			ra.spilled[interval.VReg] = ra.nextSlot
			ra.nextSlot++
		}
	}
}

// GetStackSize 获取栈大小
func (ra *RegisterAllocator) GetStackSize() int {
	return ra.nextSlot * 8
}

// ToAllocation 转换为分配结果
func (ra *RegisterAllocator) ToAllocation() *types.RegisterAllocation {
	return &types.RegisterAllocation{
		Allocated:     ra.allocated,
		Spilled:       ra.spilled,
		StackSize:     ra.GetStackSize(),
		LiveIntervals: ra.liveIntervals,
	}
}

// ============================================================================
// 代码缓存
// ============================================================================

// CodeCache 代码缓存
type CodeCache struct {
	maxSize  int
	usedSize int
	entries  map[string]*CompiledFunction
	memory   []byte
	nextFree int
	mutex    sync.RWMutex
}

// NewCodeCache 创建代码缓存
func NewCodeCache(maxSize int) *CodeCache {
	return &CodeCache{
		maxSize: maxSize,
		entries: make(map[string]*CompiledFunction),
		memory:  make([]byte, maxSize),
	}
}

// AllocateExecutable 分配可执行内存
func (cc *CodeCache) AllocateExecutable(size int) ([]byte, error) {
	cc.mutex.Lock()
	defer cc.mutex.Unlock()
	
	if cc.nextFree+size > cc.maxSize {
		return nil, fmt.Errorf("code cache full")
	}
	
	start := cc.nextFree
	cc.nextFree += size
	cc.usedSize += size
	
	return cc.memory[start : start+size], nil
}

// Put 存储已编译函数
func (cc *CodeCache) Put(name string, compiled *CompiledFunction) {
	cc.mutex.Lock()
	defer cc.mutex.Unlock()
	cc.entries[name] = compiled
}

// Get 获取已编译函数
func (cc *CodeCache) Get(name string) (*CompiledFunction, bool) {
	cc.mutex.RLock()
	defer cc.mutex.RUnlock()
	compiled, ok := cc.entries[name]
	return compiled, ok
}
