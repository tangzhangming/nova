// function_table.go - JIT ????????
//
// ?????? JIT ?????????????????
// ?????
// 1. ???????????
// 2. ?????????????????????????
// 3. ?????????????
// 4. PLT??????????????

package jit

import (
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/tangzhangming/nova/internal/bytecode"
)

// ============================================================================
// ????
// ============================================================================

// FunctionState ????
type FunctionState int32

const (
	FuncStateNone      FunctionState = iota
	FuncStatePending
	FuncStateCompiling
	FuncStateCompiled
	FuncStateFailed
)

// JITFunctionEntry JIT ?????
type JITFunctionEntry struct {
	Name       string
	FullName   string
	State      FunctionState
	Bytecode   *bytecode.Function
	Compiled   *CompiledFunc
	EntryPoint uintptr
	
	CallCount  int64
	
	PatchSites []PatchSite
	
	NumArgs    int
	IsVariadic bool
	IsMethod   bool
	ClassName  string
}

// PatchSite ???????
type PatchSite struct {
	CodeAddr   uintptr
	PatchType  int
	CallerFunc string
}

const (
	PatchTypeCall = 1
	PatchTypeJump = 2
	PatchTypePLT  = 3
)

// ============================================================================
// ???
// ============================================================================

// FunctionTable ?????
type FunctionTable struct {
	mu        sync.RWMutex
	functions map[string]*JITFunctionEntry
	byAddr    map[uintptr]*JITFunctionEntry
	
	pltMu     sync.Mutex
	pltSlots  []uintptr
	pltIndex  map[string]int
	pltSize   int
	
	totalCompiled int64
	totalCalls    int64
}

const (
	PLTInitialSize = 256
	PLTGrowSize    = 128
)

var globalFunctionTable *FunctionTable
var functionTableOnce sync.Once

// GetFunctionTable ???????
func GetFunctionTable() *FunctionTable {
	functionTableOnce.Do(func() {
		globalFunctionTable = &FunctionTable{
			functions: make(map[string]*JITFunctionEntry),
			byAddr:    make(map[uintptr]*JITFunctionEntry),
			pltSlots:  make([]uintptr, PLTInitialSize),
			pltIndex:  make(map[string]int),
			pltSize:   PLTInitialSize,
		}
	})
	return globalFunctionTable
}

// Register ????
func (ft *FunctionTable) Register(name string, fn *bytecode.Function) *JITFunctionEntry {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	
	if entry, exists := ft.functions[name]; exists {
		entry.Bytecode = fn
		return entry
	}
	
	entry := &JITFunctionEntry{
		Name:       name,
		FullName:   name,
		State:      FuncStatePending,
		Bytecode:   fn,
		PatchSites: make([]PatchSite, 0),
	}
	
	if fn != nil {
		entry.NumArgs = fn.Arity
		entry.IsVariadic = fn.IsVariadic
	}
	
	ft.functions[name] = entry
	return entry
}

// RegisterMethod ????
func (ft *FunctionTable) RegisterMethod(className, methodName string, fn *bytecode.Function) *JITFunctionEntry {
	fullName := className + "::" + methodName
	
	ft.mu.Lock()
	defer ft.mu.Unlock()
	
	if entry, exists := ft.functions[fullName]; exists {
		entry.Bytecode = fn
		return entry
	}
	
	entry := &JITFunctionEntry{
		Name:       methodName,
		FullName:   fullName,
		State:      FuncStatePending,
		Bytecode:   fn,
		IsMethod:   true,
		ClassName:  className,
		PatchSites: make([]PatchSite, 0),
	}
	
	if fn != nil {
		entry.NumArgs = fn.Arity
		entry.IsVariadic = fn.IsVariadic
	}
	
	ft.functions[fullName] = entry
	return entry
}

// SetCompiled ??????
func (ft *FunctionTable) SetCompiled(name string, compiled *CompiledFunc) {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	
	entry, exists := ft.functions[name]
	if !exists {
		entry = &JITFunctionEntry{
			Name:       name,
			FullName:   name,
			PatchSites: make([]PatchSite, 0),
		}
		ft.functions[name] = entry
	}
	
	entry.Compiled = compiled
	entry.State = FuncStateCompiled
	
	if compiled != nil {
		entry.EntryPoint = compiled.EntryPoint()
		ft.byAddr[entry.EntryPoint] = entry
		atomic.AddInt64(&ft.totalCompiled, 1)
		
		ft.patchCallSites(entry)
	}
}

func (ft *FunctionTable) patchCallSites(entry *JITFunctionEntry) {
	if len(entry.PatchSites) == 0 || entry.EntryPoint == 0 {
		return
	}
	
	for _, site := range entry.PatchSites {
		switch site.PatchType {
		case PatchTypeCall:
			patchDirectCall(site.CodeAddr, entry.EntryPoint)
		case PatchTypePLT:
			if idx, ok := ft.pltIndex[entry.FullName]; ok {
				ft.pltSlots[idx] = entry.EntryPoint
			}
		}
	}
	
	entry.PatchSites = entry.PatchSites[:0]
}

func patchDirectCall(callAddr uintptr, targetAddr uintptr) {
	offset := int32(targetAddr - (callAddr + 5))
	ptr := (*int32)(unsafe.Pointer(callAddr + 1))
	*ptr = offset
}

// GetAddress ??????
func (ft *FunctionTable) GetAddress(name string) uintptr {
	ft.mu.RLock()
	defer ft.mu.RUnlock()
	
	if entry, ok := ft.functions[name]; ok && entry.State == FuncStateCompiled {
		return entry.EntryPoint
	}
	return 0
}

// GetEntry ??????
func (ft *FunctionTable) GetEntry(name string) (*JITFunctionEntry, bool) {
	ft.mu.RLock()
	defer ft.mu.RUnlock()
	
	entry, ok := ft.functions[name]
	return entry, ok
}

// GetByAddress ????????
func (ft *FunctionTable) GetByAddress(addr uintptr) (*JITFunctionEntry, bool) {
	ft.mu.RLock()
	defer ft.mu.RUnlock()
	
	entry, ok := ft.byAddr[addr]
	return entry, ok
}

// AddPatchSite ?????????
func (ft *FunctionTable) AddPatchSite(targetName string, site PatchSite) {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	
	entry, exists := ft.functions[targetName]
	if !exists {
		entry = &JITFunctionEntry{
			Name:       targetName,
			FullName:   targetName,
			State:      FuncStatePending,
			PatchSites: make([]PatchSite, 0),
		}
		ft.functions[targetName] = entry
	}
	
	if entry.State == FuncStateCompiled && entry.EntryPoint != 0 {
		switch site.PatchType {
		case PatchTypeCall:
			patchDirectCall(site.CodeAddr, entry.EntryPoint)
		}
		return
	}
	
	entry.PatchSites = append(entry.PatchSites, site)
}

// ============================================================================
// PLT (?????)
// ============================================================================

// GetOrCreatePLTSlot ????? PLT ?
func (ft *FunctionTable) GetOrCreatePLTSlot(name string) int {
	ft.pltMu.Lock()
	defer ft.pltMu.Unlock()
	
	if idx, ok := ft.pltIndex[name]; ok {
		return idx
	}
	
	idx := len(ft.pltIndex)
	if idx >= ft.pltSize {
		newSize := ft.pltSize + PLTGrowSize
		newSlots := make([]uintptr, newSize)
		copy(newSlots, ft.pltSlots)
		ft.pltSlots = newSlots
		ft.pltSize = newSize
	}
	
	ft.pltIndex[name] = idx
	
	ft.mu.RLock()
	if entry, ok := ft.functions[name]; ok && entry.EntryPoint != 0 {
		ft.pltSlots[idx] = entry.EntryPoint
	}
	ft.mu.RUnlock()
	
	return idx
}

// GetPLTSlotAddr ?? PLT ???
func (ft *FunctionTable) GetPLTSlotAddr(index int) uintptr {
	if index < 0 || index >= ft.pltSize {
		return 0
	}
	return uintptr(unsafe.Pointer(&ft.pltSlots[index]))
}

// GetPLTValue ?? PLT ????
func (ft *FunctionTable) GetPLTValue(index int) uintptr {
	if index < 0 || index >= ft.pltSize {
		return 0
	}
	return ft.pltSlots[index]
}

// ============================================================================
// ????
// ============================================================================

// IncrementCallCount ??????
func (ft *FunctionTable) IncrementCallCount(name string) int64 {
	ft.mu.RLock()
	entry, ok := ft.functions[name]
	ft.mu.RUnlock()
	
	if ok {
		count := atomic.AddInt64(&entry.CallCount, 1)
		atomic.AddInt64(&ft.totalCalls, 1)
		return count
	}
	return 0
}

// GetCallCount ??????
func (ft *FunctionTable) GetCallCount(name string) int64 {
	ft.mu.RLock()
	defer ft.mu.RUnlock()
	
	if entry, ok := ft.functions[name]; ok {
		return atomic.LoadInt64(&entry.CallCount)
	}
	return 0
}

// ============================================================================
// ????
// ============================================================================

// InlineCache ??????
type InlineCache struct {
	ClassID    int32
	MethodAddr uintptr
	VTableIdx  int
	Hits       int64
}

// MethodCache ????
type MethodCache struct {
	mu      sync.RWMutex
	entries map[string]*InlineCache
}

var globalMethodCache *MethodCache
var methodCacheOnce sync.Once

// GetMethodCache ??????
func GetMethodCache() *MethodCache {
	methodCacheOnce.Do(func() {
		globalMethodCache = &MethodCache{
			entries: make(map[string]*InlineCache),
		}
	})
	return globalMethodCache
}

// Get ?????????
func (mc *MethodCache) Get(className, methodName string) (*InlineCache, bool) {
	key := className + "::" + methodName
	
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	
	ic, ok := mc.entries[key]
	if ok {
		atomic.AddInt64(&ic.Hits, 1)
	}
	return ic, ok
}

// Set ??????
func (mc *MethodCache) Set(className, methodName string, classID int32, addr uintptr, vtableIdx int) {
	key := className + "::" + methodName
	
	mc.mu.Lock()
	defer mc.mu.Unlock()
	
	mc.entries[key] = &InlineCache{
		ClassID:    classID,
		MethodAddr: addr,
		VTableIdx:  vtableIdx,
	}
}

// ============================================================================
// ????
// ============================================================================

// IsCompiled ?????????
func (ft *FunctionTable) IsCompiled(name string) bool {
	ft.mu.RLock()
	defer ft.mu.RUnlock()
	
	if entry, ok := ft.functions[name]; ok {
		return entry.State == FuncStateCompiled
	}
	return false
}

// GetStats ??????
func (ft *FunctionTable) GetStats() (totalFuncs, compiled, pending int64) {
	ft.mu.RLock()
	defer ft.mu.RUnlock()
	
	totalFuncs = int64(len(ft.functions))
	for _, entry := range ft.functions {
		switch entry.State {
		case FuncStateCompiled:
			compiled++
		case FuncStatePending:
			pending++
		}
	}
	return
}

// Reset ?????
func (ft *FunctionTable) Reset() {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	
	ft.functions = make(map[string]*JITFunctionEntry)
	ft.byAddr = make(map[uintptr]*JITFunctionEntry)
	ft.pltSlots = make([]uintptr, PLTInitialSize)
	ft.pltIndex = make(map[string]int)
	ft.totalCompiled = 0
	ft.totalCalls = 0
}

// ============================================================================
// ???????
// ============================================================================

// ExternalFunction ??????
type ExternalFunction struct {
	Name      string
	Module    string
	Address   uintptr
	Resolved  bool
}

// ExternalFunctionTable ?????
type ExternalFunctionTable struct {
	mu        sync.RWMutex
	functions map[string]*ExternalFunction
}

var globalExternTable *ExternalFunctionTable
var externTableOnce sync.Once

// GetExternalFunctionTable ???????
func GetExternalFunctionTable() *ExternalFunctionTable {
	externTableOnce.Do(func() {
		globalExternTable = &ExternalFunctionTable{
			functions: make(map[string]*ExternalFunction),
		}
	})
	return globalExternTable
}

// Register ??????
func (eft *ExternalFunctionTable) Register(module, name string, addr uintptr) {
	fullName := module + "." + name
	
	eft.mu.Lock()
	defer eft.mu.Unlock()
	
	eft.functions[fullName] = &ExternalFunction{
		Name:     name,
		Module:   module,
		Address:  addr,
		Resolved: addr != 0,
	}
}

// Resolve ????????
func (eft *ExternalFunctionTable) Resolve(module, name string) (uintptr, bool) {
	fullName := module + "." + name
	
	eft.mu.RLock()
	defer eft.mu.RUnlock()
	
	if ef, ok := eft.functions[fullName]; ok && ef.Resolved {
		return ef.Address, true
	}
	return 0, false
}
