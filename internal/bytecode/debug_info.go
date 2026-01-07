package bytecode

import (
	"fmt"
	"strings"
)

// ============================================================================
// B7. 字节码调试信息增强
// ============================================================================

// DebugInfo 调试信息
type DebugInfo struct {
	// 源文件信息
	SourceFile string // 源文件路径
	
	// 行号映射：字节码偏移 -> 源码行号
	LineMap map[int]int
	
	// 列号映射：字节码偏移 -> 源码列号
	ColumnMap map[int]int
	
	// 变量信息
	Variables []VariableInfo
	
	// 作用域信息
	Scopes []ScopeInfo
	
	// 函数信息
	Functions []FunctionDebugInfo
	
	// 断点信息
	Breakpoints []int // 可设置断点的字节码偏移
}

// VariableInfo 变量调试信息
type VariableInfo struct {
	Name       string // 变量名（不含 $）
	Type       string // 变量类型
	Slot       int    // 局部变量槽位（-1 表示全局变量）
	ScopeID    int    // 所属作用域 ID
	StartPC    int    // 变量生命周期开始的字节码偏移
	EndPC      int    // 变量生命周期结束的字节码偏移
	IsConstant bool   // 是否为常量
	IsCaptured bool   // 是否被闭包捕获
}

// ScopeInfo 作用域信息
type ScopeInfo struct {
	ID       int    // 作用域 ID
	ParentID int    // 父作用域 ID（-1 表示全局作用域）
	StartPC  int    // 作用域开始的字节码偏移
	EndPC    int    // 作用域结束的字节码偏移
	Name     string // 作用域名称（如函数名、"if"、"for" 等）
	Depth    int    // 嵌套深度
}

// FunctionDebugInfo 函数调试信息
type FunctionDebugInfo struct {
	Name       string   // 函数名
	ClassName  string   // 所属类名（空字符串表示顶层函数）
	StartPC    int      // 函数开始的字节码偏移
	EndPC      int      // 函数结束的字节码偏移
	Parameters []string // 参数名列表
	ReturnType string   // 返回类型
	IsStatic   bool     // 是否为静态方法
	IsPrivate  bool     // 是否为私有方法
}

// NewDebugInfo 创建调试信息
func NewDebugInfo(sourceFile string) *DebugInfo {
	return &DebugInfo{
		SourceFile:  sourceFile,
		LineMap:     make(map[int]int),
		ColumnMap:   make(map[int]int),
		Variables:   make([]VariableInfo, 0),
		Scopes:      make([]ScopeInfo, 0),
		Functions:   make([]FunctionDebugInfo, 0),
		Breakpoints: make([]int, 0),
	}
}

// AddLineMapping 添加行号映射
func (d *DebugInfo) AddLineMapping(pc, line int) {
	d.LineMap[pc] = line
}

// AddColumnMapping 添加列号映射
func (d *DebugInfo) AddColumnMapping(pc, column int) {
	d.ColumnMap[pc] = column
}

// AddVariable 添加变量信息
func (d *DebugInfo) AddVariable(info VariableInfo) {
	d.Variables = append(d.Variables, info)
}

// AddScope 添加作用域信息
func (d *DebugInfo) AddScope(info ScopeInfo) {
	d.Scopes = append(d.Scopes, info)
}

// AddFunction 添加函数信息
func (d *DebugInfo) AddFunction(info FunctionDebugInfo) {
	d.Functions = append(d.Functions, info)
}

// AddBreakpoint 添加断点位置
func (d *DebugInfo) AddBreakpoint(pc int) {
	d.Breakpoints = append(d.Breakpoints, pc)
}

// GetLine 获取指定字节码偏移的源码行号
func (d *DebugInfo) GetLine(pc int) int {
	if line, ok := d.LineMap[pc]; ok {
		return line
	}
	// 查找最近的前一个映射
	nearestPC := -1
	nearestLine := 0
	for p, l := range d.LineMap {
		if p <= pc && p > nearestPC {
			nearestPC = p
			nearestLine = l
		}
	}
	return nearestLine
}

// GetColumn 获取指定字节码偏移的源码列号
func (d *DebugInfo) GetColumn(pc int) int {
	if col, ok := d.ColumnMap[pc]; ok {
		return col
	}
	return 0
}

// GetLocation 获取完整的源码位置
func (d *DebugInfo) GetLocation(pc int) string {
	line := d.GetLine(pc)
	col := d.GetColumn(pc)
	if col > 0 {
		return fmt.Sprintf("%s:%d:%d", d.SourceFile, line, col)
	}
	return fmt.Sprintf("%s:%d", d.SourceFile, line)
}

// GetVariablesInScope 获取指定位置可见的变量
func (d *DebugInfo) GetVariablesInScope(pc int) []VariableInfo {
	var result []VariableInfo
	for _, v := range d.Variables {
		if v.StartPC <= pc && (v.EndPC == 0 || pc <= v.EndPC) {
			result = append(result, v)
		}
	}
	return result
}

// GetVariableByName 按名称查找变量
func (d *DebugInfo) GetVariableByName(name string, pc int) *VariableInfo {
	for i := len(d.Variables) - 1; i >= 0; i-- {
		v := &d.Variables[i]
		if v.Name == name && v.StartPC <= pc && (v.EndPC == 0 || pc <= v.EndPC) {
			return v
		}
	}
	return nil
}

// GetCurrentScope 获取指定位置的当前作用域
func (d *DebugInfo) GetCurrentScope(pc int) *ScopeInfo {
	var current *ScopeInfo
	maxDepth := -1
	for i := range d.Scopes {
		s := &d.Scopes[i]
		if s.StartPC <= pc && (s.EndPC == 0 || pc <= s.EndPC) {
			if s.Depth > maxDepth {
				maxDepth = s.Depth
				current = s
			}
		}
	}
	return current
}

// GetCurrentFunction 获取指定位置所在的函数
func (d *DebugInfo) GetCurrentFunction(pc int) *FunctionDebugInfo {
	for i := range d.Functions {
		f := &d.Functions[i]
		if f.StartPC <= pc && (f.EndPC == 0 || pc <= f.EndPC) {
			return f
		}
	}
	return nil
}

// IsBreakpoint 检查指定位置是否可以设置断点
func (d *DebugInfo) IsBreakpoint(pc int) bool {
	for _, bp := range d.Breakpoints {
		if bp == pc {
			return true
		}
	}
	return false
}

// Dump 输出调试信息摘要
func (d *DebugInfo) Dump() string {
	var sb strings.Builder
	
	sb.WriteString(fmt.Sprintf("=== Debug Info: %s ===\n", d.SourceFile))
	
	// 函数信息
	if len(d.Functions) > 0 {
		sb.WriteString("\nFunctions:\n")
		for _, f := range d.Functions {
			if f.ClassName != "" {
				sb.WriteString(fmt.Sprintf("  %s::%s", f.ClassName, f.Name))
			} else {
				sb.WriteString(fmt.Sprintf("  %s", f.Name))
			}
			sb.WriteString(fmt.Sprintf(" [%d-%d]", f.StartPC, f.EndPC))
			if len(f.Parameters) > 0 {
				sb.WriteString(fmt.Sprintf(" params: %s", strings.Join(f.Parameters, ", ")))
			}
			if f.ReturnType != "" {
				sb.WriteString(fmt.Sprintf(" -> %s", f.ReturnType))
			}
			sb.WriteString("\n")
		}
	}
	
	// 作用域信息
	if len(d.Scopes) > 0 {
		sb.WriteString("\nScopes:\n")
		for _, s := range d.Scopes {
			indent := strings.Repeat("  ", s.Depth+1)
			sb.WriteString(fmt.Sprintf("%s%s (ID=%d, parent=%d) [%d-%d]\n",
				indent, s.Name, s.ID, s.ParentID, s.StartPC, s.EndPC))
		}
	}
	
	// 变量信息
	if len(d.Variables) > 0 {
		sb.WriteString("\nVariables:\n")
		for _, v := range d.Variables {
			varType := "var"
			if v.IsConstant {
				varType = "const"
			}
			location := "global"
			if v.Slot >= 0 {
				location = fmt.Sprintf("slot %d", v.Slot)
			}
			captured := ""
			if v.IsCaptured {
				captured = " (captured)"
			}
			sb.WriteString(fmt.Sprintf("  %s $%s: %s @ %s [%d-%d]%s\n",
				varType, v.Name, v.Type, location, v.StartPC, v.EndPC, captured))
		}
	}
	
	// 断点信息
	if len(d.Breakpoints) > 0 {
		sb.WriteString("\nBreakpoints:\n")
		sb.WriteString(fmt.Sprintf("  %v\n", d.Breakpoints))
	}
	
	return sb.String()
}

// ============================================================================
// 增强的 Chunk（带调试信息）
// ============================================================================

// ChunkWithDebug 带调试信息的字节码块
type ChunkWithDebug struct {
	*Chunk
	Debug *DebugInfo
}

// NewChunkWithDebug 创建带调试信息的字节码块
func NewChunkWithDebug(sourceFile string) *ChunkWithDebug {
	return &ChunkWithDebug{
		Chunk: NewChunk(),
		Debug: NewDebugInfo(sourceFile),
	}
}

// WriteWithLocation 写入字节并记录位置信息
func (c *ChunkWithDebug) WriteWithLocation(b byte, line, column int) {
	pc := len(c.Code)
	c.Chunk.Write(b, line)
	c.Debug.AddLineMapping(pc, line)
	if column > 0 {
		c.Debug.AddColumnMapping(pc, column)
	}
}

// WriteOpWithLocation 写入操作码并记录位置信息
func (c *ChunkWithDebug) WriteOpWithLocation(op OpCode, line, column int) {
	pc := len(c.Code)
	c.Chunk.WriteOp(op, line)
	c.Debug.AddLineMapping(pc, line)
	if column > 0 {
		c.Debug.AddColumnMapping(pc, column)
	}
	// 语句开始位置可以设置断点
	c.Debug.AddBreakpoint(pc)
}

// EnterScope 进入新作用域
func (c *ChunkWithDebug) EnterScope(name string, parentID, depth int) int {
	id := len(c.Debug.Scopes)
	c.Debug.AddScope(ScopeInfo{
		ID:       id,
		ParentID: parentID,
		StartPC:  len(c.Code),
		Name:     name,
		Depth:    depth,
	})
	return id
}

// LeaveScope 离开作用域
func (c *ChunkWithDebug) LeaveScope(scopeID int) {
	if scopeID >= 0 && scopeID < len(c.Debug.Scopes) {
		c.Debug.Scopes[scopeID].EndPC = len(c.Code)
	}
}

// DeclareVariable 声明变量
func (c *ChunkWithDebug) DeclareVariable(name, typeName string, slot, scopeID int, isConstant bool) {
	c.Debug.AddVariable(VariableInfo{
		Name:       name,
		Type:       typeName,
		Slot:       slot,
		ScopeID:    scopeID,
		StartPC:    len(c.Code),
		IsConstant: isConstant,
	})
}

// EndVariable 结束变量生命周期
func (c *ChunkWithDebug) EndVariable(name string) {
	for i := len(c.Debug.Variables) - 1; i >= 0; i-- {
		if c.Debug.Variables[i].Name == name && c.Debug.Variables[i].EndPC == 0 {
			c.Debug.Variables[i].EndPC = len(c.Code)
			break
		}
	}
}

// MarkVariableCaptured 标记变量被闭包捕获
func (c *ChunkWithDebug) MarkVariableCaptured(name string) {
	for i := len(c.Debug.Variables) - 1; i >= 0; i-- {
		if c.Debug.Variables[i].Name == name {
			c.Debug.Variables[i].IsCaptured = true
			break
		}
	}
}

// BeginFunction 开始函数
func (c *ChunkWithDebug) BeginFunction(name, className string, params []string, returnType string, isStatic, isPrivate bool) {
	c.Debug.AddFunction(FunctionDebugInfo{
		Name:       name,
		ClassName:  className,
		StartPC:    len(c.Code),
		Parameters: params,
		ReturnType: returnType,
		IsStatic:   isStatic,
		IsPrivate:  isPrivate,
	})
}

// EndFunction 结束函数
func (c *ChunkWithDebug) EndFunction(name string) {
	for i := len(c.Debug.Functions) - 1; i >= 0; i-- {
		if c.Debug.Functions[i].Name == name && c.Debug.Functions[i].EndPC == 0 {
			c.Debug.Functions[i].EndPC = len(c.Code)
			break
		}
	}
}

// DisassembleWithDebug 带调试信息的反汇编
func (c *ChunkWithDebug) DisassembleWithDebug(name string) string {
	var result strings.Builder
	result.WriteString(fmt.Sprintf("=== %s ===\n", name))
	result.WriteString(fmt.Sprintf("Source: %s\n\n", c.Debug.SourceFile))
	
	offset := 0
	for offset < len(c.Code) {
		// 显示位置信息
		line := c.Debug.GetLine(offset)
		col := c.Debug.GetColumn(offset)
		
		result.WriteString(fmt.Sprintf("%04d ", offset))
		
		// 显示行号
		if offset > 0 && c.Lines[offset] == c.Lines[offset-1] {
			result.WriteString("   | ")
		} else {
			result.WriteString(fmt.Sprintf("%4d ", line))
		}
		
		// 断点标记
		if c.Debug.IsBreakpoint(offset) {
			result.WriteString("* ")
		} else {
			result.WriteString("  ")
		}
		
		op := OpCode(c.Code[offset])
		
		// 显示指令
		switch op {
		case OpLoadLocal, OpStoreLocal:
			slot := c.Chunk.ReadU16(offset + 1)
			varName := ""
			for _, v := range c.Debug.Variables {
				if v.Slot == int(slot) && v.StartPC <= offset && (v.EndPC == 0 || offset <= v.EndPC) {
					varName = v.Name
					break
				}
			}
			if varName != "" {
				result.WriteString(fmt.Sprintf("%-16s %4d  ; $%s\n", op, slot, varName))
			} else {
				result.WriteString(fmt.Sprintf("%-16s %4d\n", op, slot))
			}
			offset += 3
			
		default:
			// 使用原有的反汇编逻辑
			var instrStr string
			offset = c.Chunk.disassembleInstruction(&instrStr, offset)
			result.WriteString(instrStr)
		}
		
		_ = col // 可以用于更详细的调试信息
	}
	
	// 附加调试信息摘要
	result.WriteString("\n")
	result.WriteString(c.Debug.Dump())
	
	return result.String()
}

