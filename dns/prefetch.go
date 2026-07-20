package dns

import (
	"sync"
	"time"
)

// PrefetchMetadata 存储单个 DNS key 的预刷新元数据
type PrefetchMetadata struct {
	lastAccess   time.Time
	nextPrefetch time.Time
	failureCount int
	lastFailTime time.Time
}

// PrefetchManager 管理 DNS hot-key 预刷新
type PrefetchManager struct {
	mu                sync.RWMutex
	metadata          map[dnsCacheKey]*PrefetchMetadata
	maxMetadataSize   int
	prefetchQPS       int
	prefetchWindow    time.Duration
	hotKeyThreshold   time.Duration
	backoffMultiplier float64
	maxBackoff        time.Duration
	jitterFraction    float64
}

// NewPrefetchManager 创建预刷新管理器
func NewPrefetchManager(maxMetadataSize int, prefetchQPS int) *PrefetchManager {
	return &PrefetchManager{
		metadata:          make(map[dnsCacheKey]*PrefetchMetadata),
		maxMetadataSize:   maxMetadataSize,
		prefetchQPS:       prefetchQPS,
		prefetchWindow:    time.Second,
		hotKeyThreshold:   5 * time.Minute,
		backoffMultiplier: 2.0,
		maxBackoff:        5 * time.Minute,
		jitterFraction:    0.2,
	}
}

// ShouldPrefetch 判断是否应该预刷新该 key
func (pm *PrefetchManager) ShouldPrefetch(key dnsCacheKey, ttl uint32, now time.Time) bool {
	pm.mu.RLock()
	meta, exists := pm.metadata[key]
	pm.mu.RUnlock()

	if !exists {
		return false
	}

	// 检查是否已到预刷新时间
	if now.Before(meta.nextPrefetch) {
		return false
	}

	// 检查失败退避
	if meta.failureCount > 0 {
		backoff := time.Duration(float64(time.Second) * (pm.backoffMultiplier * float64(meta.failureCount)))
		if backoff > pm.maxBackoff {
			backoff = pm.maxBackoff
		}
		if now.Sub(meta.lastFailTime) < backoff {
			return false
		}
	}

	return true
}

// RecordAccess 记录 key 被访问，用于判断是否为 hot-key
func (pm *PrefetchManager) RecordAccess(key dnsCacheKey, ttl uint32, now time.Time) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	meta, exists := pm.metadata[key]
	if !exists {
		// 新 key，检查容量
		if len(pm.metadata) >= pm.maxMetadataSize {
			// 简单淘汰：删除最久未访问的
			pm.evictOldest()
		}
		meta = &PrefetchMetadata{
			lastAccess:   now,
			nextPrefetch: now, // 立即可预刷新
		}
		pm.metadata[key] = meta
		return
	}

	// 更新访问时间
	meta.lastAccess = now

	// 计算下次预刷新时间（TTL 到期前 10% 时刻）
	prefetchLeadTime := time.Duration(float64(ttl) * 0.1 * float64(time.Second))
	if prefetchLeadTime < 5*time.Second {
		prefetchLeadTime = 5 * time.Second
	}
	if prefetchLeadTime > 1*time.Minute {
		prefetchLeadTime = 1 * time.Minute
	}

	// 加入抖动
	jitter := time.Duration(float64(prefetchLeadTime) * pm.jitterFraction * 2)
	meta.nextPrefetch = now.Add(time.Duration(ttl)*time.Second - prefetchLeadTime - jitter)
}

// RecordFailure 记录预刷新失败
func (pm *PrefetchManager) RecordFailure(key dnsCacheKey, now time.Time) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	meta, exists := pm.metadata[key]
	if !exists {
		return
	}

	meta.failureCount++
	meta.lastFailTime = now
}

// RecordSuccess 记录预刷新成功，重置失败计数
func (pm *PrefetchManager) RecordSuccess(key dnsCacheKey) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	meta, exists := pm.metadata[key]
	if !exists {
		return
	}

	meta.failureCount = 0
	// 重置预刷新时间为立即可预刷新（设置为过去时间）
	meta.nextPrefetch = time.Time{}
}

// evictOldest 淘汰最久未访问的元数据
func (pm *PrefetchManager) evictOldest() {
	var oldestKey dnsCacheKey
	var oldestTime time.Time

	for key, meta := range pm.metadata {
		if oldestTime.IsZero() || meta.lastAccess.Before(oldestTime) {
			oldestKey = key
			oldestTime = meta.lastAccess
		}
	}

	if !oldestTime.IsZero() {
		delete(pm.metadata, oldestKey)
	}
}

// Cleanup 清理过期的元数据
func (pm *PrefetchManager) Cleanup(now time.Time, maxAge time.Duration) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for key, meta := range pm.metadata {
		if now.Sub(meta.lastAccess) > maxAge {
			delete(pm.metadata, key)
		}
	}
}
