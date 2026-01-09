// Package vm 实现了 Sola 编程语言的字节码虚拟机。
//
// 本文件实现多线程 VM 的共享状态管理，确保并发安全。
package vm

import (
	"sync"
	"sync/atomic"

	"github.com/tangzhangming/nova/internal/bytecode"
)

// ============================================================================
// 共享状态管理
// ============================================================================
//
// BUG FIX 2026-01-10: 多线程 VM - 共享状态
// 防止反复引入的问题:
// 1. 读多写少的数据（classes, enums）使用 sync.RWMutex
// 2. 频繁修改的数据（globals）使用 sync.Map
// 3. 初始化完成后 classes 和 enums 应该是只读的
// 4. 必须在 VM 启动执行前完成所有类和枚举的注册

// SharedState 封装多线程 VM 中需要并发安全访问的共享状态
//
// 在多线程 VM 架构中，多个工作线程（Worker）共享同一份 SharedState：
//   - globals: 全局变量，可能在运行时被修改
//   - classes: 类定义，初始化后只读
//   - enums: 枚举定义，初始化后只读
//
// 线程安全保证：
//   - globals 使用 sync.Map 提供无锁读取（读多写少场景优化）
//   - classes 和 enums 使用 RWMutex，初始化后设置 frozen 标志禁止写入
type SharedState struct {
	// =========================================================================
	// 全局变量存储（并发安全）
	// =========================================================================

	// globals 使用 sync.Map 存储全局变量
	// sync.Map 对于读多写少的场景有优秀的性能
	// 键: string（变量名）
	// 值: bytecode.Value
	globals sync.Map

	// =========================================================================
	// 类型定义存储（初始化后只读）
	// =========================================================================

	// classesMu 保护 classes 的读写
	classesMu sync.RWMutex

	// classes 存储所有类定义
	// 初始化完成后（frozen=true），只允许读取
	classes map[string]*bytecode.Class

	// enumsMu 保护 enums 的读写
	enumsMu sync.RWMutex

	// enums 存储所有枚举定义
	// 初始化完成后（frozen=true），只允许读取
	enums map[string]*bytecode.Enum

	// =========================================================================
	// 状态标志
	// =========================================================================

	// frozen 标记类型定义是否已冻结
	// 冻结后，classes 和 enums 不能再被修改
	// 使用原子操作保证可见性
	frozen int32
}

// NewSharedState 创建新的共享状态实例
func NewSharedState() *SharedState {
	return &SharedState{
		classes: make(map[string]*bytecode.Class),
		enums:   make(map[string]*bytecode.Enum),
	}
}

// ============================================================================
// 全局变量操作（并发安全）
// ============================================================================

// GetGlobal 获取全局变量
//
// 此方法是并发安全的，可以从多个工作线程同时调用。
//
// 参数:
//   - name: 变量名
//
// 返回值:
//   - value: 变量值
//   - ok: 变量是否存在
func (s *SharedState) GetGlobal(name string) (bytecode.Value, bool) {
	if val, ok := s.globals.Load(name); ok {
		return val.(bytecode.Value), true
	}
	return bytecode.NullValue, false
}

// SetGlobal 设置全局变量
//
// 此方法是并发安全的，可以从多个工作线程同时调用。
// 注意：频繁的写入可能影响性能，建议只在必要时修改全局变量。
//
// 参数:
//   - name: 变量名
//   - value: 变量值
func (s *SharedState) SetGlobal(name string, value bytecode.Value) {
	s.globals.Store(name, value)
}

// DeleteGlobal 删除全局变量
//
// 参数:
//   - name: 变量名
func (s *SharedState) DeleteGlobal(name string) {
	s.globals.Delete(name)
}

// RangeGlobals 遍历所有全局变量
//
// 参数:
//   - f: 回调函数，返回 false 停止遍历
func (s *SharedState) RangeGlobals(f func(name string, value bytecode.Value) bool) {
	s.globals.Range(func(key, value interface{}) bool {
		return f(key.(string), value.(bytecode.Value))
	})
}

// ============================================================================
// 类定义操作（初始化后只读）
// ============================================================================

// DefineClass 注册类定义
//
// 此方法只能在 VM 启动执行前调用（frozen=false 时）。
// 调用 Freeze() 后，此方法将返回 false。
//
// 参数:
//   - class: 类定义
//
// 返回值:
//   - ok: 是否成功注册
func (s *SharedState) DefineClass(class *bytecode.Class) bool {
	if s.IsFrozen() {
		return false
	}

	s.classesMu.Lock()
	defer s.classesMu.Unlock()

	s.classes[class.Name] = class
	return true
}

// GetClass 获取类定义
//
// 此方法是并发安全的。
//
// 参数:
//   - name: 类名
//
// 返回值:
//   - 类定义，不存在时返回 nil
func (s *SharedState) GetClass(name string) *bytecode.Class {
	s.classesMu.RLock()
	defer s.classesMu.RUnlock()

	return s.classes[name]
}

// GetAllClasses 获取所有类定义的副本
//
// 返回一个只读副本，修改不会影响原始数据。
func (s *SharedState) GetAllClasses() map[string]*bytecode.Class {
	s.classesMu.RLock()
	defer s.classesMu.RUnlock()

	result := make(map[string]*bytecode.Class, len(s.classes))
	for k, v := range s.classes {
		result[k] = v
	}
	return result
}

// ============================================================================
// 枚举定义操作（初始化后只读）
// ============================================================================

// DefineEnum 注册枚举定义
//
// 此方法只能在 VM 启动执行前调用（frozen=false 时）。
// 调用 Freeze() 后，此方法将返回 false。
//
// 参数:
//   - enum: 枚举定义
//
// 返回值:
//   - ok: 是否成功注册
func (s *SharedState) DefineEnum(enum *bytecode.Enum) bool {
	if s.IsFrozen() {
		return false
	}

	s.enumsMu.Lock()
	defer s.enumsMu.Unlock()

	s.enums[enum.Name] = enum
	return true
}

// GetEnum 获取枚举定义
//
// 此方法是并发安全的。
//
// 参数:
//   - name: 枚举名
//
// 返回值:
//   - 枚举定义，不存在时返回 nil
func (s *SharedState) GetEnum(name string) *bytecode.Enum {
	s.enumsMu.RLock()
	defer s.enumsMu.RUnlock()

	return s.enums[name]
}

// GetAllEnums 获取所有枚举定义的副本
func (s *SharedState) GetAllEnums() map[string]*bytecode.Enum {
	s.enumsMu.RLock()
	defer s.enumsMu.RUnlock()

	result := make(map[string]*bytecode.Enum, len(s.enums))
	for k, v := range s.enums {
		result[k] = v
	}
	return result
}

// ============================================================================
// 冻结控制
// ============================================================================

// Freeze 冻结类型定义
//
// 调用此方法后，不能再通过 DefineClass 和 DefineEnum 添加新的类型定义。
// 这是多线程 VM 启动前的必要步骤，确保运行时类型定义不会被意外修改。
//
// 冻结是单向的，一旦冻结不能解冻。
func (s *SharedState) Freeze() {
	atomic.StoreInt32(&s.frozen, 1)
}

// IsFrozen 检查类型定义是否已冻结
func (s *SharedState) IsFrozen() bool {
	return atomic.LoadInt32(&s.frozen) == 1
}

// ============================================================================
// 从传统 VM 迁移的辅助方法
// ============================================================================

// ImportFromLegacyVM 从传统单线程 VM 导入状态
//
// 这是一个过渡方法，用于将现有的 map 数据迁移到并发安全的 SharedState。
// 注意：此方法不是并发安全的，只应在初始化阶段调用。
//
// 参数:
//   - globals: 全局变量 map
//   - classes: 类定义 map
//   - enums: 枚举定义 map
func (s *SharedState) ImportFromLegacyVM(
	globals map[string]bytecode.Value,
	classes map[string]*bytecode.Class,
	enums map[string]*bytecode.Enum,
) {
	// 导入全局变量
	for name, value := range globals {
		s.globals.Store(name, value)
	}

	// 导入类定义
	s.classesMu.Lock()
	for name, class := range classes {
		s.classes[name] = class
	}
	s.classesMu.Unlock()

	// 导入枚举定义
	s.enumsMu.Lock()
	for name, enum := range enums {
		s.enums[name] = enum
	}
	s.enumsMu.Unlock()
}

// ExportToLegacyVM 导出状态到传统 map 格式
//
// 用于调试或与不支持 SharedState 的代码交互。
// 注意：返回的是副本，修改不会影响原始数据。
func (s *SharedState) ExportToLegacyVM() (
	globals map[string]bytecode.Value,
	classes map[string]*bytecode.Class,
	enums map[string]*bytecode.Enum,
) {
	// 导出全局变量
	globals = make(map[string]bytecode.Value)
	s.globals.Range(func(key, value interface{}) bool {
		globals[key.(string)] = value.(bytecode.Value)
		return true
	})

	// 导出类定义
	classes = s.GetAllClasses()

	// 导出枚举定义
	enums = s.GetAllEnums()

	return
}

// ============================================================================
// 统计信息
// ============================================================================

// Stats 返回共享状态的统计信息
func (s *SharedState) Stats() SharedStateStats {
	var globalCount int
	s.globals.Range(func(key, value interface{}) bool {
		globalCount++
		return true
	})

	s.classesMu.RLock()
	classCount := len(s.classes)
	s.classesMu.RUnlock()

	s.enumsMu.RLock()
	enumCount := len(s.enums)
	s.enumsMu.RUnlock()

	return SharedStateStats{
		GlobalCount: globalCount,
		ClassCount:  classCount,
		EnumCount:   enumCount,
		IsFrozen:    s.IsFrozen(),
	}
}

// SharedStateStats 共享状态统计信息
type SharedStateStats struct {
	GlobalCount int  // 全局变量数量
	ClassCount  int  // 类定义数量
	EnumCount   int  // 枚举定义数量
	IsFrozen    bool // 是否已冻结
}
