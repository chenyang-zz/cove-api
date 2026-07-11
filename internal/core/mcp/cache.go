package mcp

import (
	"context"
	"sync"
	"time"
)

type ToolCache interface {
	Get(ctx context.Context, key string) (CacheEntry, bool, error)
	Set(ctx context.Context, key string, entry CacheEntry) error
	Valid(server ServerConfig, entry CacheEntry) bool
}

type MemoryToolCache struct {
	mu      sync.RWMutex
	entries map[string]CacheEntry
	now     func() time.Time
}

func NewMemoryToolCache() *MemoryToolCache {
	return &MemoryToolCache{
		entries: map[string]CacheEntry{},
		now:     time.Now,
	}
}

func (c *MemoryToolCache) Get(ctx context.Context, key string) (CacheEntry, bool, error) {
	if c == nil {
		return CacheEntry{}, false, nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.entries[key]
	entry.Tools = cloneTools(entry.Tools)
	return entry, ok, nil
}

func (c *MemoryToolCache) Set(ctx context.Context, key string, entry CacheEntry) error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = map[string]CacheEntry{}
	}
	entry.Tools = cloneTools(entry.Tools)
	c.entries[key] = entry
	return nil
}

func (c *MemoryToolCache) Valid(server ServerConfig, entry CacheEntry) bool {
	now := time.Now
	if c != nil && c.now != nil {
		now = c.now
	}
	return entry.Fingerprint == Fingerprint(server) && now().Before(entry.ExpiresAt)
}

// Stale reports whether entry 的指纹仍匹配且包含可复用工具列表。
//
// 与 Valid 不同，Stale 忽略 TTL，用于远端发现失败时的 stale-if-error 降级。
func Stale(server ServerConfig, entry CacheEntry) bool {
	return entry.Fingerprint == Fingerprint(server) && len(entry.Tools) > 0
}
