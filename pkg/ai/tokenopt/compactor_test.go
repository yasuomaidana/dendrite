package tokenopt

import (
	"Dendrite/pkg/coreclient/coretest"
	"strings"
	"testing"

	"Dendrite/pkg/testkit"
)

func TestMain(m *testing.M) { coretest.Main(m) }

func TestEstimateTokens(t *testing.T) {
	testkit.Equal(t, "empty", EstimateTokens(""), 0)
	testkit.Equal(t, "single char", EstimateTokens("a"), 1)
	testkit.Equal(t, "8 chars", EstimateTokens("12345678"), 2)
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
	testkit.Check(t, err)

	testkit.Equal(t, "OriginalChars", ctx.OriginalChars, len(source))
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
	snippet := `func StandaloneHelper(x int) int {
	y := x * 2
	return y
}

func KeptHelper() string {
	return "kept"
}
`
	ctx, err := CompactGoSource(snippet, []string{"KeptHelper"})
	testkit.Check(t, err)

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
	testkit.Check(t, err)

	if strings.Contains(ctx.CompactedContent, "stepping") {
		t.Errorf("expected Step body to be pruned")
	}
}

func TestCompactGoSource_FoldError(t *testing.T) {
	_, err := CompactGoSource("FAIL", nil)
	if err == nil {
		t.Fatal("expected error on fold failure")
	}
}
