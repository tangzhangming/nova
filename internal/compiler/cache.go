// cache.go - 字节码缓存管理
//
// 实现字节码的序列化缓存，用于增量编译。
//
// 功能：
// 1. 文件哈希计算和变化检测
// 2. 字节码序列化/反序列化
// 3. 缓存目录管理
// 4. LRU 缓存清理策略

package compiler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/tangzhangming/nova/internal/bytecode"
)

const (
	// CacheVersion 缓存版本，版本不匹配时需要重新编译
	CacheVersion = "1.0.0"
	
	// DefaultCacheDir 默认缓存目录
	DefaultCacheDir = ".sola-cache"
	
	// MaxCacheEntries 最大缓存条目数
	MaxCacheEntries = 1000
	
	// MaxCacheSize 最大缓存大小（字节）
	MaxCacheSize = 100 * 1024 * 1024 // 100MB
)

// CacheManager 缓存管理器
type CacheManager struct {
	mu       sync.RWMutex
	cacheDir string
	index    *CacheIndex
	enabled  bool
}

// CacheIndex 缓存索引
type CacheIndex struct {
	Version   string                 `json:"version"`
	Entries   map[string]*CacheEntry `json:"entries"`
	TotalSize int64                  `json:"total_size"`
	UpdatedAt time.Time              `json:"updated_at"`
}

// CacheEntry 缓存条目
type CacheEntry struct {
	SourcePath   string    `json:"source_path"`
	SourceHash   string    `json:"source_hash"`
	CacheFile    string    `json:"cache_file"`
	Size         int64     `json:"size"`
	Dependencies []string  `json:"dependencies"`
	CompiledAt   time.Time `json:"compiled_at"`
	AccessedAt   time.Time `json:"accessed_at"`
	AccessCount  int       `json:"access_count"`
}

// NewCacheManager 创建缓存管理器
func NewCacheManager(workDir string) (*CacheManager, error) {
	cacheDir := filepath.Join(workDir, DefaultCacheDir)
	
	cm := &CacheManager{
		cacheDir: cacheDir,
		enabled:  true,
	}
	
	// 确保缓存目录存在
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}
	
	// 加载索引
	if err := cm.loadIndex(); err != nil {
		// 索引加载失败，创建新索引
		cm.index = &CacheIndex{
			Version: CacheVersion,
			Entries: make(map[string]*CacheEntry),
		}
	}
	
	// 验证版本
	if cm.index.Version != CacheVersion {
		// 版本不匹配，清空缓存
		cm.Clear()
	}
	
	return cm, nil
}

// Enable 启用缓存
func (cm *CacheManager) Enable() {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.enabled = true
}

// Disable 禁用缓存
func (cm *CacheManager) Disable() {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.enabled = false
}

// IsEnabled 检查是否启用
func (cm *CacheManager) IsEnabled() bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.enabled
}

// Get 从缓存获取编译结果
func (cm *CacheManager) Get(sourcePath string) (*bytecode.CompiledFile, bool) {
	if !cm.IsEnabled() {
		return nil, false
	}
	
	cm.mu.Lock()
	defer cm.mu.Unlock()
	
	// 计算源文件哈希
	hash, err := cm.computeFileHash(sourcePath)
	if err != nil {
		return nil, false
	}
	
	// 查找缓存条目
	entry, ok := cm.index.Entries[sourcePath]
	if !ok {
		return nil, false
	}
	
	// 验证哈希
	if entry.SourceHash != hash {
		// 文件已更改，删除缓存
		cm.removeEntryUnsafe(sourcePath)
		return nil, false
	}
	
	// 检查依赖项是否更改
	if cm.dependenciesChanged(entry) {
		cm.removeEntryUnsafe(sourcePath)
		return nil, false
	}
	
	// 加载缓存文件
	cf, err := cm.loadCacheFile(entry.CacheFile)
	if err != nil {
		cm.removeEntryUnsafe(sourcePath)
		return nil, false
	}
	
	// 更新访问信息
	entry.AccessedAt = time.Now()
	entry.AccessCount++
	
	return cf, true
}

// Put 存储编译结果到缓存
func (cm *CacheManager) Put(sourcePath string, cf *bytecode.CompiledFile, deps []string) error {
	if !cm.IsEnabled() {
		return nil
	}
	
	cm.mu.Lock()
	defer cm.mu.Unlock()
	
	// 计算源文件哈希
	hash, err := cm.computeFileHash(sourcePath)
	if err != nil {
		return err
	}
	
	// 生成缓存文件名
	cacheFile := cm.generateCacheFileName(sourcePath, hash)
	
	// 序列化并写入缓存文件
	size, err := cm.saveCacheFile(cacheFile, cf)
	if err != nil {
		return err
	}
	
	// 创建缓存条目
	entry := &CacheEntry{
		SourcePath:   sourcePath,
		SourceHash:   hash,
		CacheFile:    cacheFile,
		Size:         size,
		Dependencies: deps,
		CompiledAt:   time.Now(),
		AccessedAt:   time.Now(),
		AccessCount:  1,
	}
	
	// 更新索引
	cm.index.Entries[sourcePath] = entry
	cm.index.TotalSize += size
	cm.index.UpdatedAt = time.Now()
	
	// 检查是否需要清理
	cm.cleanupIfNeeded()
	
	// 保存索引
	return cm.saveIndex()
}

// Invalidate 使缓存条目失效
func (cm *CacheManager) Invalidate(sourcePath string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.removeEntryUnsafe(sourcePath)
}

// InvalidateDependents 使依赖指定文件的所有缓存失效
func (cm *CacheManager) InvalidateDependents(sourcePath string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	
	toRemove := []string{}
	for path, entry := range cm.index.Entries {
		for _, dep := range entry.Dependencies {
			if dep == sourcePath {
				toRemove = append(toRemove, path)
				break
			}
		}
	}
	
	for _, path := range toRemove {
		cm.removeEntryUnsafe(path)
	}
}

// Clear 清空所有缓存
func (cm *CacheManager) Clear() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	
	// 删除所有缓存文件
	entries, err := os.ReadDir(cm.cacheDir)
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && filepath.Ext(entry.Name()) == ".cache" {
				os.Remove(filepath.Join(cm.cacheDir, entry.Name()))
			}
		}
	}
	
	// 重置索引
	cm.index = &CacheIndex{
		Version: CacheVersion,
		Entries: make(map[string]*CacheEntry),
	}
	
	return cm.saveIndex()
}

// Stats 获取缓存统计
func (cm *CacheManager) Stats() CacheStats {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	
	return CacheStats{
		TotalEntries: len(cm.index.Entries),
		TotalSize:    cm.index.TotalSize,
		CacheDir:     cm.cacheDir,
		UpdatedAt:    cm.index.UpdatedAt,
	}
}

// CacheStats 缓存统计信息
type CacheStats struct {
	TotalEntries int
	TotalSize    int64
	CacheDir     string
	UpdatedAt    time.Time
}

// ============================================================================
// 内部方法
// ============================================================================

// loadIndex 加载缓存索引
func (cm *CacheManager) loadIndex() error {
	indexPath := filepath.Join(cm.cacheDir, "index.json")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return err
	}
	
	cm.index = &CacheIndex{}
	return json.Unmarshal(data, cm.index)
}

// saveIndex 保存缓存索引
func (cm *CacheManager) saveIndex() error {
	indexPath := filepath.Join(cm.cacheDir, "index.json")
	data, err := json.MarshalIndent(cm.index, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(indexPath, data, 0644)
}

// computeFileHash 计算文件哈希
func (cm *CacheManager) computeFileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	
	return hex.EncodeToString(h.Sum(nil)), nil
}

// generateCacheFileName 生成缓存文件名
func (cm *CacheManager) generateCacheFileName(sourcePath, hash string) string {
	// 使用源文件路径的哈希和内容哈希组合
	pathHash := sha256.Sum256([]byte(sourcePath))
	name := hex.EncodeToString(pathHash[:8]) + "_" + hash[:16] + ".cache"
	return filepath.Join(cm.cacheDir, name)
}

// saveCacheFile 保存缓存文件
func (cm *CacheManager) saveCacheFile(cacheFile string, cf *bytecode.CompiledFile) (int64, error) {
	data, err := bytecode.SerializeToBytes(cf)
	if err != nil {
		return 0, err
	}
	
	if err := os.WriteFile(cacheFile, data, 0644); err != nil {
		return 0, err
	}
	
	return int64(len(data)), nil
}

// loadCacheFile 加载缓存文件
func (cm *CacheManager) loadCacheFile(cacheFile string) (*bytecode.CompiledFile, error) {
	data, err := os.ReadFile(cacheFile)
	if err != nil {
		return nil, err
	}
	
	if err := bytecode.ValidateHeader(data); err != nil {
		return nil, err
	}
	
	return bytecode.DeserializeFromBytes(data)
}

// removeEntryUnsafe 删除缓存条目（不加锁）
func (cm *CacheManager) removeEntryUnsafe(sourcePath string) {
	entry, ok := cm.index.Entries[sourcePath]
	if !ok {
		return
	}
	
	// 删除缓存文件
	os.Remove(entry.CacheFile)
	
	// 更新索引
	cm.index.TotalSize -= entry.Size
	delete(cm.index.Entries, sourcePath)
}

// dependenciesChanged 检查依赖项是否更改
func (cm *CacheManager) dependenciesChanged(entry *CacheEntry) bool {
	for _, dep := range entry.Dependencies {
		depEntry, ok := cm.index.Entries[dep]
		if !ok {
			// 依赖项不在缓存中，可能已更改
			return true
		}
		
		// 检查依赖项的编译时间
		if depEntry.CompiledAt.After(entry.CompiledAt) {
			return true
		}
	}
	return false
}

// cleanupIfNeeded 如果需要则清理缓存
func (cm *CacheManager) cleanupIfNeeded() {
	// 检查条目数量
	if len(cm.index.Entries) > MaxCacheEntries {
		cm.evictLRU(len(cm.index.Entries) - MaxCacheEntries)
	}
	
	// 检查总大小
	if cm.index.TotalSize > MaxCacheSize {
		cm.evictBySize(cm.index.TotalSize - MaxCacheSize)
	}
}

// evictLRU 使用 LRU 策略驱逐条目
func (cm *CacheManager) evictLRU(count int) {
	// 按访问时间排序
	entries := make([]*CacheEntry, 0, len(cm.index.Entries))
	for _, entry := range cm.index.Entries {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].AccessedAt.Before(entries[j].AccessedAt)
	})
	
	// 删除最久未访问的条目
	for i := 0; i < count && i < len(entries); i++ {
		cm.removeEntryUnsafe(entries[i].SourcePath)
	}
}

// evictBySize 按大小驱逐条目直到满足大小限制
func (cm *CacheManager) evictBySize(targetReduction int64) {
	// 按访问频率和时间综合排序
	entries := make([]*CacheEntry, 0, len(cm.index.Entries))
	for _, entry := range cm.index.Entries {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		// 优先删除访问次数少且时间久的
		scoreI := float64(entries[i].AccessCount) / float64(time.Since(entries[i].AccessedAt).Hours()+1)
		scoreJ := float64(entries[j].AccessCount) / float64(time.Since(entries[j].AccessedAt).Hours()+1)
		return scoreI < scoreJ
	})
	
	// 删除条目直到满足大小限制
	var reduced int64
	for _, entry := range entries {
		if reduced >= targetReduction {
			break
		}
		reduced += entry.Size
		cm.removeEntryUnsafe(entry.SourcePath)
	}
}

// ComputeContentHash 计算内容哈希（用于字符串内容）
func ComputeContentHash(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])
}
