package tokenopt

import (
	"errors"
	"time"
)

// Standard error definitions for token optimization operations.
var (
	ErrEmptySource    = errors.New("source code payload cannot be empty")
	ErrPrefixExpired  = errors.New("cached prefix has expired")
	ErrPrefixNotFound = errors.New("prefix key not found in cache")
)

// CompactedContext encapsulates the pruned AST source code and resulting token savings.
type CompactedContext struct {
	OriginalChars            int     `json:"original_chars"`
	CompactedChars           int     `json:"compacted_chars"`
	EstimatedOriginalTokens  int     `json:"estimated_original_tokens"`
	EstimatedCompactedTokens int     `json:"estimated_compacted_tokens"`
	CompactionRatio          float64 `json:"compaction_ratio"`
	CompactedContent         string  `json:"compacted_content"`
}

// CachedPrefix stores a high-volume prefix payload (such as an AST index) for LiteLLM prefix reuse.
type CachedPrefix struct {
	CacheKey    string        `json:"cache_key"`
	PrefixText  string        `json:"prefix_text"`
	TokenVolume int           `json:"token_volume"`
	CreatedAt   time.Time     `json:"created_at"`
	TTL         time.Duration `json:"ttl"`
	HitCount    int           `json:"hit_count"`
}

// IsExpired checks whether the cached prefix has outlived its time-to-live.
func (c *CachedPrefix) IsExpired() bool {
	return time.Since(c.CreatedAt) > c.TTL
}

// TokenEfficiencyMetrics measures the economic return on prompt token investment.
type TokenEfficiencyMetrics struct {
	AcceptedLOC          int     `json:"accepted_loc"`
	PromptTokensConsumed int     `json:"prompt_tokens_consumed"`
	EfficiencyRatio      float64 `json:"efficiency_ratio"` // LOC per token
}
