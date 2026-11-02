package tokenopt_test

import (
	"errors"
	"testing"
	"time"

	"Dendrite/pkg/ai/tokenopt"
)

func TestPrefixCacheHarness_Lifecycle(t *testing.T) {
	harness := tokenopt.NewPrefixCacheHarness()

	// 1. Error on empty prefix
	if _, err := harness.StorePrefix("", 1*time.Hour); err == nil {
		t.Errorf("expected error caching empty prefix")
	}

	// 2. Store valid prefix
	prefixContent := "SYSTEM INSTRUCTIONS: You are an autonomous coding assistant for Dendrite."
	entry, err := harness.StorePrefix(prefixContent, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error storing prefix: %v", err)
	}

	if entry.CacheKey == "" || entry.TokenVolume <= 0 {
		t.Errorf("expected cache key and token volume, got key=%s, tokens=%d", entry.CacheKey, entry.TokenVolume)
	}

	if harness.Count() != 1 {
		t.Errorf("expected 1 cached prefix, got %d", harness.Count())
	}

	// 3. Cache Hit
	retrieved, err := harness.GetPrefix(entry.CacheKey)
	if err != nil {
		t.Fatalf("unexpected error getting prefix: %v", err)
	}
	if retrieved.HitCount != 1 {
		t.Errorf("expected hit count to be 1, got %d", retrieved.HitCount)
	}

	// 4. Missing key
	if _, err := harness.GetPrefix("nonexistent-key"); !errors.Is(err, tokenopt.ErrPrefixNotFound) {
		t.Errorf("expected ErrPrefixNotFound for missing key")
	}

	// 5. Expiration & Purge
	time.Sleep(60 * time.Millisecond)

	// Fetching expired prefix returns ErrPrefixExpired
	if _, err := harness.GetPrefix(entry.CacheKey); !errors.Is(err, tokenopt.ErrPrefixExpired) {
		t.Errorf("expected ErrPrefixExpired for stale prefix, got: %v", err)
	}

	// Store another short-lived prefix for PurgeExpired test
	entry2, _ := harness.StorePrefix("another prefix", 10*time.Millisecond)
	time.Sleep(15 * time.Millisecond)
	purged := harness.PurgeExpired()
	if purged != 1 {
		t.Errorf("expected 1 entry purged, got %d", purged)
	}
	_ = entry2
}

func TestCalculateEfficiency(t *testing.T) {
	// 500 lines accepted for 1000 tokens consumed -> ratio 0.50 LOC/token
	metrics := tokenopt.CalculateEfficiency(500, 1000)
	if metrics.EfficiencyRatio != 0.50 {
		t.Errorf("expected efficiency ratio 0.50, got %.2f", metrics.EfficiencyRatio)
	}

	// Zero tokens consumed guard
	metricsZero := tokenopt.CalculateEfficiency(100, 0)
	if metricsZero.EfficiencyRatio != 0.0 {
		t.Errorf("expected 0.0 ratio when 0 tokens consumed")
	}
}
