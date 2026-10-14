package tokenopt_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"Dendrite/pkg/ai/tokenopt"
)

func TestCompactGoSource(t *testing.T) {
	// 1. Error on empty source
	if _, err := tokenopt.CompactGoSource("", nil); !errors.Is(err, tokenopt.ErrEmptySource) {
		t.Errorf("expected ErrEmptySource, got: %v", err)
	}

	// 2. Full Go source with types, interfaces, and heavy implementations
	heavyBody1 := strings.Repeat("\t\tfmt.Println(\"processing extensive data record batch item\")\n", 50)
	heavyBody2 := strings.Repeat("\t\tres += (x * i) + (y * i)\n\t\tfmt.Println(\"matrix multiplication step\", res)\n", 50)

	sampleSource := `package service

import (
	"context"
	"fmt"
)

type User struct {
	ID   string
	Name string
}

type UserService interface {
	GetUser(ctx context.Context, id string) (*User, error)
	DeleteUser(ctx context.Context, id string) error
}

func (s *server) GetUser(ctx context.Context, id string) (*User, error) {
	// We want to keep this implementation
	if id == "" {
		return nil, fmt.Errorf("empty id")
	}
	return &User{ID: id, Name: "Alice"}, nil
}

func (s *server) DeleteUser(ctx context.Context, id string) error {
` + heavyBody1 + `	return nil
}

func helperCalculation(x, y int) int {
	res := 0
` + heavyBody2 + `	return res
}
`

	// Keep only "GetUser"
	compacted, err := tokenopt.CompactGoSource(sampleSource, []string{"GetUser"})
	if err != nil {
		t.Fatalf("unexpected error compacting Go source: %v", err)
	}

	content := compacted.CompactedContent

	// Must preserve package, imports, structs, interfaces
	if !strings.Contains(content, "package service") ||
		!strings.Contains(content, "type User struct") ||
		!strings.Contains(content, "type UserService interface") {
		t.Errorf("expected type definitions and interfaces to be preserved")
	}

	// Must preserve GetUser implementation
	if !strings.Contains(content, "We want to keep this implementation") {
		t.Errorf("expected GetUser implementation to be preserved")
	}

	// Must prune DeleteUser and helperCalculation bodies
	if strings.Contains(content, "processing extensive data record batch item") ||
		strings.Contains(content, "matrix multiplication step") {
		t.Errorf("expected heavy function bodies to be pruned")
	}

	// Check pruning comment
	if !strings.Contains(content, "implementation pruned for token economy") {
		t.Errorf("expected pruning placeholder comment in compacted source")
	}

	// Verify compaction ratio meets >= 50% AC-1 goal
	if compacted.CompactionRatio < 0.50 {
		t.Errorf("expected >= 50%% compaction ratio, got %.2f%%", compacted.CompactionRatio*100)
	}
}

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
