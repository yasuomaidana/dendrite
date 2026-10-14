package tokenopt

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"
)

// PrefixCacheHarness provides in-memory prompt prefix caching compatible with LiteLLM and Gemini/Claude.
type PrefixCacheHarness struct {
	mu      sync.RWMutex
	entries map[string]*CachedPrefix
}

// NewPrefixCacheHarness creates an active prefix cache repository.
func NewPrefixCacheHarness() *PrefixCacheHarness {
	return &PrefixCacheHarness{
		entries: make(map[string]*CachedPrefix),
	}
}

// StorePrefix caches a prefix payload and returns the derived SHA-256 cache key and metrics.
func (h *PrefixCacheHarness) StorePrefix(prefixText string, ttl time.Duration) (*CachedPrefix, error) {
	trimmed := strings.TrimSpace(prefixText)
	if trimmed == "" {
		return nil, fmt.Errorf("cannot cache empty prefix")
	}

	if ttl <= 0 {
		ttl = 1 * time.Hour
	}

	hash := sha256.Sum256([]byte(trimmed))
	cacheKey := hex.EncodeToString(hash[:])

	tokens := EstimateTokens(trimmed)

	entry := &CachedPrefix{
		CacheKey:    cacheKey,
		PrefixText:  trimmed,
		TokenVolume: tokens,
		CreatedAt:   time.Now(),
		TTL:         ttl,
		HitCount:    0,
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	h.entries[cacheKey] = entry

	return entry, nil
}

// GetPrefix retrieves a cached prefix if valid, incrementing its hit count.
func (h *PrefixCacheHarness) GetPrefix(cacheKey string) (*CachedPrefix, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	entry, exists := h.entries[cacheKey]
	if !exists {
		return nil, ErrPrefixNotFound
	}

	if entry.IsExpired() {
		delete(h.entries, cacheKey)
		return nil, ErrPrefixExpired
	}

	entry.HitCount++
	cpy := *entry
	return &cpy, nil
}

// PurgeExpired sweeps the cache and eliminates stale items.
func (h *PrefixCacheHarness) PurgeExpired() int {
	h.mu.Lock()
	defer h.mu.Unlock()

	purged := 0
	for key, entry := range h.entries {
		if entry.IsExpired() {
			delete(h.entries, key)
			purged++
		}
	}
	return purged
}

// Count returns the number of active cached prefixes.
func (h *PrefixCacheHarness) Count() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.entries)
}
