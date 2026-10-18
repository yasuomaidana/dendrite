package tokenopt

import (
	"strings"
	"testing"
)

func TestEstimateTokens(t *testing.T) {
	if got := EstimateTokens(""); got != 0 {
		t.Fatalf("expected 0 tokens for empty string, got %d", got)
	}
	if got := EstimateTokens("a"); got != 1 {
		t.Fatalf("expected 1 token for single char, got %d", got)
	}
	if got := EstimateTokens("12345678"); got != 2 {
		t.Fatalf("expected 2 tokens for 8 chars, got %d", got)
	}
}

func TestCompactGoSource_Empty(t *testing.T) {
	if _, err := CompactGoSource("", nil); err != ErrEmptySource {
		t.Fatalf("expected ErrEmptySource for empty source, got %v", err)
	}
	if _, err := CompactGoSource("   \n\t  ", nil); err != ErrEmptySource {
		t.Fatalf("expected ErrEmptySource for whitespace source, got %v", err)
	}
}

func TestCompactGoSource_ValidGoSource(t *testing.T) {
	source := `package service

type Client struct {
	Endpoint string
}

func NewClient(ep string) *Client {
	return &Client{Endpoint: ep}
}

func (c *Client) CallAPI() error {
	println("heavy logic")
	for i := 0; i < 3; i++ {
		println("retrying request against", c.Endpoint, "attempt", i)
		if c.Endpoint == "" {
			return nil
		}
	}
	println("request completed successfully after retries")
	return nil
}
`
	ctx, err := CompactGoSource(source, []string{"NewClient"})
	if err != nil {
		t.Fatalf("unexpected CompactGoSource error: %v", err)
	}

	if ctx.OriginalChars != len(source) {
		t.Errorf("expected OriginalChars %d, got %d", len(source), ctx.OriginalChars)
	}
	if ctx.CompactedChars >= ctx.OriginalChars {
		t.Errorf("expected CompactedChars < OriginalChars, got %d >= %d", ctx.CompactedChars, ctx.OriginalChars)
	}
	if ctx.CompactionRatio <= 0 {
		t.Errorf("expected positive compaction ratio, got %.4f", ctx.CompactionRatio)
	}

	// NewClient was kept
	if !strings.Contains(ctx.CompactedContent, "return &Client{Endpoint: ep}") {
		t.Errorf("expected NewClient body to be kept in compacted content")
	}
	// CallAPI body was pruned
	if strings.Contains(ctx.CompactedContent, "heavy logic") {
		t.Errorf("expected CallAPI body to be pruned in compacted content")
	}
}

func TestCompactGoSource_SnippetFallback(t *testing.T) {
	// A snippet without a package clause triggers the line-based fallback
	snippet := `func StandaloneHelper(x int) int {
	y := x * 2
	return y
}

func KeptHelper() string {
	return "kept"
}
`
	ctx, err := CompactGoSource(snippet, []string{"KeptHelper"})
	if err != nil {
		t.Fatalf("unexpected CompactGoSource error on snippet: %v", err)
	}

	if !strings.Contains(ctx.CompactedContent, "return \"kept\"") {
		t.Errorf("expected KeptHelper body to be preserved")
	}
	if strings.Contains(ctx.CompactedContent, "y := x * 2") {
		t.Errorf("expected StandaloneHelper body to be pruned")
	}
}

func TestCompactGoSource_MethodReceiverSnippet(t *testing.T) {
	snippet := `func (r *Runner) Step() {
	println("stepping")
}
`
	ctx, err := CompactGoSource(snippet, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(ctx.CompactedContent, "stepping") {
		t.Errorf("expected Step body to be pruned")
	}
}
