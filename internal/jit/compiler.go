package jit

import (
	"sync"

	"github.com/tangzhangming/nova/internal/bytecode"
)

// ============================================================================
// JIT 编译器框架
// ============================================================================

// JITCompiler JIT 编译器
type JITCompiler struct {
	config      *Config
	cache       sync.Map // map[*bytecode.Function]*CompiledCode
	hotspots    sync.Map // map[*bytecode.Function]int (执行计数)
	helperAddrs map[string]uintptr

	// 编译统计
	stats CompilerStats
	mu    sync.RWMutex
}

// CompiledCode 编译后的代码
type CompiledCode struct {
	// 机器码
	Code []byte

	// 入口点
	Entry uintptr

	// 元数据
	Function   *bytecode.Function
	Size       int
	IRInsts    []IRInst
	HelperRefs []HelperRef

	// 调试信息
	SourceMap []SourceMapping
}

// HelperRef Helper 函数引用
type HelperRef struct {
	Name   string
	Offset int    // 在机器码中的偏移
	Addr   uintptr
}

// SourceMapping 源码映射
type SourceMapping struct {
	CodeOffset int // 机器码偏移
	Line       int // 源码行号
}

// CompilerStats 编译器统计
type CompilerStats struct {
	TotalCompiled     int64  // 总编译数
	TotalIRInsts      int64  // 总 IR 指令数
	TotalCodeBytes    int64  // 总机器码字节数
	TotalCompileTime  int64  // 总编译时间 (ns)
	CacheHits         int64  // 缓存命中
	CacheMisses       int64  // 缓存未命中
	HotspotCompiles   int64  // 热点编译数
}

// ============================================================================
// 编译器生命周期
// ============================================================================

// NewCompiler 创建 JIT 编译器
func NewCompiler(config *Config) *JITCompiler {
	if config == nil {
		config = DefaultConfig()
	}
	return &JITCompiler{
		config:      config,
		helperAddrs: make(map[string]uintptr),
	}
}

// RegisterHelper 注册 Helper 函数地址
func (c *JITCompiler) RegisterHelper(name string, addr uintptr) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.helperAddrs[name] = addr
}

// GetHelperAddr 获取 Helper 函数地址
func (c *JITCompiler) GetHelperAddr(name string) uintptr {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.helperAddrs[name]
}

// ============================================================================
// 编译接口
// ============================================================================

// Compile 编译函数
func (c *JITCompiler) Compile(fn *bytecode.Function) (*CompiledCode, error) {
	if fn == nil {
		return nil, nil
	}

	// 检查缓存
	if cached, ok := c.cache.Load(fn); ok {
		c.mu.Lock()
		c.stats.CacheHits++
		c.mu.Unlock()
		return cached.(*CompiledCode), nil
	}

	c.mu.Lock()
	c.stats.CacheMisses++
	c.mu.Unlock()

	// 1. 字节码 -> IR
	ir, err := c.bytecodeToIR(fn)
	if err != nil {
		return nil, err
	}

	// 2. IR 优化
	ir = c.optimizeIR(ir)

	// 3. IR -> 机器码
	machineCode, err := c.generateMachineCode(fn, ir)
	if err != nil {
		// 如果机器码生成失败，仍然返回 CompiledCode 但不含机器码
		// 可以回退到解释器
		code := &CompiledCode{
			Function: fn,
			IRInsts:  ir,
		}
		c.cache.Store(fn, code)
		return code, nil
	}

	code := &CompiledCode{
		Function: fn,
		IRInsts:  ir,
		Code:     machineCode,
		Size:     len(machineCode),
	}

	// 缓存结果
	c.cache.Store(fn, code)

	c.mu.Lock()
	c.stats.TotalCompiled++
	c.stats.TotalIRInsts += int64(len(ir))
	c.stats.TotalCodeBytes += int64(len(machineCode))
	c.mu.Unlock()

	return code, nil
}

// generateMachineCode 生成机器码
func (c *JITCompiler) generateMachineCode(fn *bytecode.Function, ir []IRInst) ([]byte, error) {
	// 创建简单的 IRFunction
	irFn := &IRFunction{
		Name:       fn.Name,
		LocalCount: fn.LocalCount,
		ArgCount:   fn.Arity,
		CFG: &CFG{
			Blocks: []*BasicBlock{
				{
					ID:    0,
					Name:  "entry",
					Insts: ir,
				},
			},
		},
	}

	// 使用 IREmitter 生成机器码
	return GenerateMachineCode(irFn, c.helperAddrs)
}

// CompileWithProfile 使用 Profile 信息编译
func (c *JITCompiler) CompileWithProfile(fn *bytecode.Function, profile *FunctionProfile) (*CompiledCode, error) {
	if fn == nil {
		return nil, nil
	}

	// 1. 字节码 -> IR (使用 Profile 信息)
	builder := NewFunctionBuilder(fn)
	irFn := builder.BuildFromBytecode(fn)

	// 2. 应用 Profile 指导的优化
	if profile != nil {
		c.applyProfileOptimizations(irFn, profile)
	}

	// 3. 运行优化 Pass
	pm := CreateStandardPipeline()
	pm.Run(irFn)

	// 4. 收集所有 IR 指令
	var allIR []IRInst
	if irFn.CFG != nil {
		for _, bb := range irFn.CFG.Blocks {
			allIR = append(allIR, bb.Insts...)
		}
	}

	code := &CompiledCode{
		Function: fn,
		IRInsts:  allIR,
	}

	c.cache.Store(fn, code)

	c.mu.Lock()
	c.stats.TotalCompiled++
	c.stats.TotalIRInsts += int64(len(allIR))
	c.mu.Unlock()

	return code, nil
}

// FunctionProfile Profile 信息 (从 vm 包引用)
type FunctionProfile struct {
	Name           string
	ExecutionCount int64
	IsHot          bool
	TypeProfiles   map[int]*TypeProfileData
}

// TypeProfileData 类型 Profile 数据
type TypeProfileData struct {
	IntCount   int64
	FloatCount int64
	OtherCount int64
}

// applyProfileOptimizations 应用 Profile 指导的优化
func (c *JITCompiler) applyProfileOptimizations(fn *IRFunction, profile *FunctionProfile) {
	if fn.CFG == nil {
		return
	}

	for _, bb := range fn.CFG.Blocks {
		for i, inst := range bb.Insts {
			// 获取该 IP 的类型 Profile
			tp := profile.TypeProfiles[inst.BytecodeIP]
			if tp == nil {
				continue
			}

			// 计算类型占比
			total := tp.IntCount + tp.FloatCount + tp.OtherCount
			if total == 0 {
				continue
			}

			intRatio := float64(tp.IntCount) / float64(total)

			// 如果 95%+ 是整数，标记为整数特化
			if intRatio >= 0.95 {
				switch inst.Op {
				case IR_ADD, IR_SUB, IR_MUL, IR_DIV, IR_MOD:
					// 可以生成整数特化代码
					bb.Insts[i].Arg2 = 1 // 标记为整数特化
				}
			}
		}
	}
}

// CanCompile 检查是否可以编译函数
func (c *JITCompiler) CanCompile(fn *bytecode.Function) bool {
	if fn == nil || fn.Chunk == nil {
		return false
	}

	// 检查是否包含不支持的操作码
	for _, op := range fn.Chunk.Code {
		if !c.isSupportedOpcode(bytecode.OpCode(op)) {
			return false
		}
	}

	return true
}

// isSupportedOpcode 检查操作码是否支持
func (c *JITCompiler) isSupportedOpcode(op bytecode.OpCode) bool {
	switch op {
	case bytecode.OpPush, bytecode.OpPop, bytecode.OpDup,
		bytecode.OpNull, bytecode.OpTrue, bytecode.OpFalse, bytecode.OpZero, bytecode.OpOne,
		bytecode.OpAdd, bytecode.OpSub, bytecode.OpMul, bytecode.OpDiv, bytecode.OpMod, bytecode.OpNeg,
		bytecode.OpBitAnd, bytecode.OpBitOr, bytecode.OpBitXor, bytecode.OpBitNot, bytecode.OpShl, bytecode.OpShr,
		bytecode.OpEq, bytecode.OpNe, bytecode.OpLt, bytecode.OpLe, bytecode.OpGt, bytecode.OpGe,
		bytecode.OpNot, bytecode.OpAnd, bytecode.OpOr,
		bytecode.OpLoadLocal, bytecode.OpStoreLocal, bytecode.OpLoadGlobal, bytecode.OpStoreGlobal,
		bytecode.OpJump, bytecode.OpJumpIfFalse, bytecode.OpJumpIfTrue, bytecode.OpLoop,
		bytecode.OpCall, bytecode.OpReturn,
		bytecode.OpSuperArrayNew, bytecode.OpSuperArrayGet, bytecode.OpSuperArraySet,
		bytecode.OpNewArray, bytecode.OpArrayGet, bytecode.OpArraySet, bytecode.OpArrayLen:
		return true
	default:
		return false
	}
}

// IsCompiled 检查函数是否已编译
func (c *JITCompiler) IsCompiled(fn *bytecode.Function) bool {
	_, ok := c.cache.Load(fn)
	return ok
}

// GetCompiled 获取编译后的代码
func (c *JITCompiler) GetCompiled(fn *bytecode.Function) *CompiledCode {
	if cached, ok := c.cache.Load(fn); ok {
		return cached.(*CompiledCode)
	}
	return nil
}

// ============================================================================
// 热点检测
// ============================================================================

// RecordExecution 记录函数执行
func (c *JITCompiler) RecordExecution(fn *bytecode.Function) bool {
	if !c.config.Enabled {
		return false
	}

	// 增加执行计数
	countAny, _ := c.hotspots.LoadOrStore(fn, 0)
	count := countAny.(int) + 1
	c.hotspots.Store(fn, count)

	// 检查是否达到热点阈值
	if count >= c.config.HotspotThreshold && !c.IsCompiled(fn) {
		// 触发编译
		go func() {
			if c.CanCompile(fn) {
				c.Compile(fn)
				c.mu.Lock()
				c.stats.HotspotCompiles++
				c.mu.Unlock()
			}
		}()
		return true
	}

	return false
}

// ============================================================================
// 字节码 -> IR 转换
// ============================================================================

// bytecodeToIR 将字节码转换为 IR
func (c *JITCompiler) bytecodeToIR(fn *bytecode.Function) ([]IRInst, error) {
	if fn.Chunk == nil {
		return nil, nil
	}

	code := fn.Chunk.Code
	consts := fn.Chunk.Constants
	
	var ir []IRInst
	ip := 0

	for ip < len(code) {
		op := bytecode.OpCode(code[ip])
		startIP := ip
		ip++

		switch op {
		case bytecode.OpPush:
			if ip+1 < len(code) {
				constIdx := int(code[ip])<<8 | int(code[ip+1])
				ip += 2
				if constIdx < len(consts) {
					ir = append(ir, IRInst{Op: IR_CONST, Value: consts[constIdx], BytecodeIP: startIP})
				}
			}

		case bytecode.OpPop:
			ir = append(ir, IRInst{Op: IR_POP, BytecodeIP: startIP})

		case bytecode.OpDup:
			ir = append(ir, IRInst{Op: IR_DUP, BytecodeIP: startIP})

		case bytecode.OpNull:
			ir = append(ir, IRInst{Op: IR_CONST, Value: bytecode.NullValue, BytecodeIP: startIP})

		case bytecode.OpTrue:
			ir = append(ir, IRInst{Op: IR_CONST, Value: bytecode.TrueValue, BytecodeIP: startIP})

		case bytecode.OpFalse:
			ir = append(ir, IRInst{Op: IR_CONST, Value: bytecode.FalseValue, BytecodeIP: startIP})

		case bytecode.OpZero:
			ir = append(ir, IRInst{Op: IR_CONST, Value: bytecode.ZeroValue, BytecodeIP: startIP})

		case bytecode.OpOne:
			ir = append(ir, IRInst{Op: IR_CONST, Value: bytecode.OneValue, BytecodeIP: startIP})

		case bytecode.OpAdd:
			ir = append(ir, IRInst{Op: IR_ADD, BytecodeIP: startIP})

		case bytecode.OpSub:
			ir = append(ir, IRInst{Op: IR_SUB, BytecodeIP: startIP})

		case bytecode.OpMul:
			ir = append(ir, IRInst{Op: IR_MUL, BytecodeIP: startIP})

		case bytecode.OpDiv:
			ir = append(ir, IRInst{Op: IR_DIV, BytecodeIP: startIP})

		case bytecode.OpMod:
			ir = append(ir, IRInst{Op: IR_MOD, BytecodeIP: startIP})

		case bytecode.OpNeg:
			ir = append(ir, IRInst{Op: IR_NEG, BytecodeIP: startIP})

		case bytecode.OpBitAnd:
			ir = append(ir, IRInst{Op: IR_BAND, BytecodeIP: startIP})

		case bytecode.OpBitOr:
			ir = append(ir, IRInst{Op: IR_BOR, BytecodeIP: startIP})

		case bytecode.OpBitXor:
			ir = append(ir, IRInst{Op: IR_BXOR, BytecodeIP: startIP})

		case bytecode.OpBitNot:
			ir = append(ir, IRInst{Op: IR_BNOT, BytecodeIP: startIP})

		case bytecode.OpShl:
			ir = append(ir, IRInst{Op: IR_SHL, BytecodeIP: startIP})

		case bytecode.OpShr:
			ir = append(ir, IRInst{Op: IR_SHR, BytecodeIP: startIP})

		case bytecode.OpEq:
			ir = append(ir, IRInst{Op: IR_EQ, BytecodeIP: startIP})

		case bytecode.OpNe:
			ir = append(ir, IRInst{Op: IR_NE, BytecodeIP: startIP})

		case bytecode.OpLt:
			ir = append(ir, IRInst{Op: IR_LT, BytecodeIP: startIP})

		case bytecode.OpLe:
			ir = append(ir, IRInst{Op: IR_LE, BytecodeIP: startIP})

		case bytecode.OpGt:
			ir = append(ir, IRInst{Op: IR_GT, BytecodeIP: startIP})

		case bytecode.OpGe:
			ir = append(ir, IRInst{Op: IR_GE, BytecodeIP: startIP})

		case bytecode.OpNot:
			ir = append(ir, IRInst{Op: IR_NOT, BytecodeIP: startIP})

		case bytecode.OpLoadLocal:
			if ip+1 < len(code) {
				slot := int(code[ip])<<8 | int(code[ip+1])
				ip += 2
				ir = append(ir, IRInst{Op: IR_LOAD_LOCAL, Arg1: slot, BytecodeIP: startIP})
			}

		case bytecode.OpStoreLocal:
			if ip+1 < len(code) {
				slot := int(code[ip])<<8 | int(code[ip+1])
				ip += 2
				ir = append(ir, IRInst{Op: IR_STORE_LOCAL, Arg1: slot, BytecodeIP: startIP})
			}

		case bytecode.OpLoadGlobal:
			if ip+1 < len(code) {
				idx := int(code[ip])<<8 | int(code[ip+1])
				ip += 2
				ir = append(ir, IRInst{Op: IR_LOAD_GLOBAL, Arg1: idx, BytecodeIP: startIP})
			}

		case bytecode.OpStoreGlobal:
			if ip+1 < len(code) {
				idx := int(code[ip])<<8 | int(code[ip+1])
				ip += 2
				ir = append(ir, IRInst{Op: IR_STORE_GLOBAL, Arg1: idx, BytecodeIP: startIP})
			}

		case bytecode.OpJump:
			if ip+1 < len(code) {
				offset := int(code[ip])<<8 | int(code[ip+1])
				ip += 2
				ir = append(ir, IRInst{Op: IR_JUMP, Arg1: offset, BytecodeIP: startIP})
			}

		case bytecode.OpJumpIfFalse:
			if ip+1 < len(code) {
				offset := int(code[ip])<<8 | int(code[ip+1])
				ip += 2
				ir = append(ir, IRInst{Op: IR_JUMP_FALSE, Arg1: offset, BytecodeIP: startIP})
			}

		case bytecode.OpJumpIfTrue:
			if ip+1 < len(code) {
				offset := int(code[ip])<<8 | int(code[ip+1])
				ip += 2
				ir = append(ir, IRInst{Op: IR_JUMP_TRUE, Arg1: offset, BytecodeIP: startIP})
			}

		case bytecode.OpLoop:
			if ip+1 < len(code) {
				offset := int(code[ip])<<8 | int(code[ip+1])
				ip += 2
				ir = append(ir, IRInst{Op: IR_LOOP, Arg1: offset, BytecodeIP: startIP})
			}

		case bytecode.OpCall:
			argCount := int(code[ip])
			ip++
			ir = append(ir, IRInst{Op: IR_CALL, Arg1: argCount, BytecodeIP: startIP})

		case bytecode.OpReturn:
			ir = append(ir, IRInst{Op: IR_RETURN, BytecodeIP: startIP})

		case bytecode.OpSuperArrayNew:
			if ip+1 < len(code) {
				count := int(code[ip])<<8 | int(code[ip+1])
				ip += 2
				ir = append(ir, IRInst{Op: IR_CALL_HELPER, HelperName: "SA_New", Arg1: count, BytecodeIP: startIP})
			}

		case bytecode.OpSuperArrayGet:
			ir = append(ir, IRInst{Op: IR_CALL_HELPER, HelperName: "SA_Get", BytecodeIP: startIP})

		case bytecode.OpSuperArraySet:
			ir = append(ir, IRInst{Op: IR_CALL_HELPER, HelperName: "SA_Set", BytecodeIP: startIP})

		default:
			// 未知操作码，跳过
		}
	}

	return ir, nil
}

// ============================================================================
// IR 优化
// ============================================================================

// optimizeIR 优化 IR
func (c *JITCompiler) optimizeIR(ir []IRInst) []IRInst {
	if len(ir) == 0 {
		return ir
	}

	// 优化级别 0: 不优化
	if c.config.OptimizationLevel == 0 {
		return ir
	}

	// 优化级别 1: 基本优化
	ir = c.constantFolding(ir)
	ir = c.deadCodeElimination(ir)

	// 优化级别 2+: 更多优化
	if c.config.OptimizationLevel >= 2 {
		ir = c.peepholeOptimization(ir)
	}

	return ir
}

// constantFolding 常量折叠
func (c *JITCompiler) constantFolding(ir []IRInst) []IRInst {
	result := make([]IRInst, 0, len(ir))

	for i := 0; i < len(ir); i++ {
		inst := ir[i]

		// 查找 CONST CONST OP 模式
		if inst.Op == IR_CONST && i+2 < len(ir) {
			next := ir[i+1]
			if next.Op == IR_CONST {
				op := ir[i+2]
				if folded, ok := c.foldConstants(inst.Value, next.Value, op.Op); ok {
					result = append(result, IRInst{Op: IR_CONST, Value: folded})
					i += 2
					continue
				}
			}
		}

		result = append(result, inst)
	}

	return result
}

// foldConstants 折叠常量
func (c *JITCompiler) foldConstants(a, b bytecode.Value, op IROp) (bytecode.Value, bool) {
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
	}

	return bytecode.NullValue, false
}

// deadCodeElimination 死代码消除
func (c *JITCompiler) deadCodeElimination(ir []IRInst) []IRInst {
	// 简单实现：移除 RETURN 后的代码
	for i, inst := range ir {
		if inst.Op == IR_RETURN {
			return ir[:i+1]
		}
	}
	return ir
}

// peepholeOptimization 窥孔优化
func (c *JITCompiler) peepholeOptimization(ir []IRInst) []IRInst {
	result := make([]IRInst, 0, len(ir))

	for i := 0; i < len(ir); i++ {
		inst := ir[i]

		// 优化 PUSH + POP 为空
		if inst.Op == IR_CONST && i+1 < len(ir) && ir[i+1].Op == IR_POP {
			i++
			continue
		}

		// 优化 DUP + POP 为空
		if inst.Op == IR_DUP && i+1 < len(ir) && ir[i+1].Op == IR_POP {
			i++
			continue
		}

		result = append(result, inst)
	}

	return result
}

// ============================================================================
// 统计和调试
// ============================================================================

// GetStats 获取编译器统计
func (c *JITCompiler) GetStats() CompilerStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.stats
}

// Reset 重置编译器
func (c *JITCompiler) Reset() {
	c.cache = sync.Map{}
	c.hotspots = sync.Map{}
	c.mu.Lock()
	c.stats = CompilerStats{}
	c.mu.Unlock()
}

// Invalidate 使缓存失效
func (c *JITCompiler) Invalidate(fn *bytecode.Function) {
	c.cache.Delete(fn)
	c.hotspots.Delete(fn)
}
