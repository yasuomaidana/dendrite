package tokenopt

import (
	"testing"
	"time"
)

func TestPrefixCacheHarness_StoreAndGet(t *testing.T) {
	cache := NewPrefixCacheHarness()

	// Store empty prefix should error
	if _, err := cache.StorePrefix("", time.Minute); err == nil {
		t.Fatal("expected error storing empty prefix")
	}

	// Store valid prefix
	entry, err := cache.StorePrefix("package main\n\ntype Config struct{}", time.Hour)
	if err != nil {
		t.Fatalf("StorePrefix failed: %v", err)
	}
	if entry.CacheKey == "" {
		t.Fatal("expected non-empty cache key")
	}
	if entry.TokenVolume <= 0 {
		t.Fatalf("expected positive token volume, got %d", entry.TokenVolume)
	}

	if count := cache.Count(); count != 1 {
		t.Fatalf("expected count 1, got %d", count)
	}

	// Retrieve prefix
	retrieved, err := cache.GetPrefix(entry.CacheKey)
	if err != nil {
		t.Fatalf("GetPrefix failed: %v", err)
	}
	if retrieved.CacheKey != entry.CacheKey {
		t.Errorf("expected cache key %s, got %s", entry.CacheKey, retrieved.CacheKey)
	}
	if retrieved.HitCount != 1 {
		t.Errorf("expected hit count 1, got %d", retrieved.HitCount)
	}

	// Second get increments hit count
	retrieved2, err := cache.GetPrefix(entry.CacheKey)
	if err != nil {
		t.Fatalf("second GetPrefix failed: %v", err)
	}
	if retrieved2.HitCount != 2 {
		t.Errorf("expected hit count 2, got %d", retrieved2.HitCount)
	}

	// Non-existent key
	if _, err := cache.GetPrefix("non-existent"); err != ErrPrefixNotFound {
		t.Fatalf("expected ErrPrefixNotFound, got %v", err)
	}
}

func TestPrefixCacheHarness_ExpirationAndPurge(t *testing.T) {
	cache := NewPrefixCacheHarness()

	// Store prefix with negative TTL so it expires immediately
	entry, err := cache.StorePrefix("expired prefix data", -time.Second)
	if err != nil {
		t.Fatalf("StorePrefix failed: %v", err)
	}

	// Overwrite CreatedAt to guarantee expiration
	cache.mu.Lock()
	entry.CreatedAt = time.Now().Add(-2 * time.Hour)
	cache.mu.Unlock()

	if !entry.IsExpired() {
		t.Fatal("expected entry to be expired")
	}

	// GetPrefix on expired entry returns ErrPrefixExpired
	if _, err := cache.GetPrefix(entry.CacheKey); err != ErrPrefixExpired {
		t.Fatalf("expected ErrPrefixExpired, got %v", err)
	}

	// Add another expired entry and test PurgeExpired
	entry2, err := cache.StorePrefix("second expired prefix", time.Millisecond)
	if err != nil {
		t.Fatalf("StorePrefix failed: %v", err)
	}
	cache.mu.Lock()
	entry2.CreatedAt = time.Now().Add(-2 * time.Hour)
	cache.mu.Unlock()

	purged := cache.PurgeExpired()
	if purged < 1 {
		t.Fatalf("expected at least 1 purged entry, got %d", purged)
	}
	if count := cache.Count(); count != 0 {
		t.Fatalf("expected cache count 0 after purge, got %d", count)
	}
}
