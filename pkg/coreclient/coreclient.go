// Package coreclient connects to the dendrite-core Rust sidecar (JSON-RPC 2.0 over stdio)
// for deterministic syntax-aware AST parsing, complexity scoring, signature folding,
// chunk hashing, and token estimation.
package coreclient

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"Dendrite/pkg/sidecar"
)

const (
	// BinaryName is the sidecar binary name located by Locator.
	BinaryName = "dendrite-core"
	// Protocol is the protocol major version this client speaks.
	Protocol = 1
)

var (
	// ErrBinaryMissing is returned when the dendrite-core executable cannot be found or started.
	ErrBinaryMissing = errors.New("dendrite-core: binary not found: run `dendrite install --dev` (or `dendrite update --dev`)")
	// ErrNoCompleteNode is returned by LastCompleteNode when no complete node could be parsed.
	ErrNoCompleteNode = errors.New("no complete node")
)

// FunctionMetrics captures cyclomatic and cognitive complexity for a function.
type FunctionMetrics struct {
	Symbol     string `json:"symbol"`
	StartLine  int    `json:"start_line"`
	EndLine    int    `json:"end_line"`
	Cyclomatic int    `json:"cyclomatic"`
	Cognitive  int    `json:"cognitive"`
}

// Folded captures folded source code and prompt token reduction metrics.
type Folded struct {
	Source       string   `json:"source"`
	Folded       []string `json:"folded"`
	TokensBefore int      `json:"tokens_before"`
	TokensAfter  int      `json:"tokens_after"`
}

// Hunk represents a 1-based inclusive range of lines modified by a diff hunk.
type Hunk struct {
	StartLine int `json:"start_line"`
	EndLine   int `json:"end_line"`
}

// ChangedSymbol maps a diff hunk to its enclosing code declaration.
type ChangedSymbol struct {
	Hunk      int    `json:"hunk"`
	Symbol    string `json:"symbol"`
	Kind      string `json:"kind"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
}

// TokenCount holds the token count for a text and whether it was exact.
type TokenCount struct {
	Count int  `json:"count"`
	Exact bool `json:"exact"`
}

// Client manages communication with the dendrite-core sidecar process.
type Client struct {
	pool *sidecar.Pool
}

// New creates a Client backed by a long-lived sidecar process pool located by loc.
func New(loc sidecar.Locator) *Client {
	return &Client{
		pool: sidecar.NewPool(sidecar.PoolConfig{
			Binary:  BinaryName,
			Major:   Protocol,
			Locator: loc,
		}),
	}
}

// NewWithPool creates a Client backed by an existing sidecar Pool.
func NewWithPool(p *sidecar.Pool) *Client {
	return &Client{pool: p}
}

var (
	defaultMu     sync.Mutex
	defaultClient *Client
)

// Default returns the process-wide default Client searching standard sidecar locations.
func Default() *Client {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	if defaultClient == nil {
		defaultClient = New(sidecar.DefaultLocator())
	}
	return defaultClient
}

// SetDefault replaces the default client and returns a cleanup function to restore the previous client.
func SetDefault(c *Client) func() {
	defaultMu.Lock()
	prev := defaultClient
	defaultClient = c
	defaultMu.Unlock()
	return func() {
		defaultMu.Lock()
		defaultClient = prev
		defaultMu.Unlock()
	}
}

// Close terminates the running sidecar process and drains queued calls.
func (c *Client) Close() error {
	if c == nil || c.pool == nil {
		return nil
	}
	return c.pool.Close()
}

func (c *Client) call(ctx context.Context, method string, params, result interface{}) error {
	if c == nil || c.pool == nil {
		return ErrBinaryMissing
	}
	err := c.pool.Call(ctx, method, params, result)
	if err == nil {
		return nil
	}
	if errors.Is(err, sidecar.ErrUnavailable) || errors.Is(err, sidecar.ErrNotFound) {
		return fmt.Errorf("%w: %v", ErrBinaryMissing, err)
	}
	var rpcErr *sidecar.RPCError
	if errors.As(err, &rpcErr) && rpcErr.Kind() == "no_complete_node" {
		return ErrNoCompleteNode
	}
	return err
}

// Complexity computes cyclomatic and cognitive complexity metrics for every function in source.
func (c *Client) Complexity(ctx context.Context, lang, source string) ([]FunctionMetrics, error) {
	params := map[string]string{
		"lang":   lang,
		"source": source,
	}
	var res []FunctionMetrics
	if err := c.call(ctx, "ast.complexity", params, &res); err != nil {
		return nil, err
	}
	return res, nil
}

// FoldSignatures prunes function bodies of symbols not in keepSymbols, replacing them with a placeholder.
func (c *Client) FoldSignatures(ctx context.Context, lang, source string, keepSymbols []string) (*Folded, error) {
	if keepSymbols == nil {
		keepSymbols = []string{}
	}
	params := map[string]interface{}{
		"lang":         lang,
		"source":       source,
		"keep_symbols": keepSymbols,
	}
	var res Folded
	if err := c.call(ctx, "ast.fold_signatures", params, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// LastCompleteNode finds the end byte offset of the last complete node in buffer.
func (c *Client) LastCompleteNode(ctx context.Context, lang, buffer string) (int, error) {
	params := map[string]string{
		"lang":   lang,
		"buffer": buffer,
	}
	var res struct {
		Offset int `json:"offset"`
	}
	if err := c.call(ctx, "ast.last_complete_node", params, &res); err != nil {
		return 0, err
	}
	return res.Offset, nil
}

// ChangedSymbols maps diff hunks to enclosing code declarations in source.
func (c *Client) ChangedSymbols(ctx context.Context, lang, source string, hunks []Hunk) ([]ChangedSymbol, error) {
	if hunks == nil {
		hunks = []Hunk{}
	}
	params := map[string]interface{}{
		"lang":   lang,
		"source": source,
		"hunks":  hunks,
	}
	var res []ChangedSymbol
	if err := c.call(ctx, "ast.changed_symbols", params, &res); err != nil {
		return nil, err
	}
	return res, nil
}

// CountTokens estimates or counts tokens for the specified model and text.
func (c *Client) CountTokens(ctx context.Context, model, text string) (*TokenCount, error) {
	params := map[string]string{
		"model": model,
		"text":  text,
	}
	var res TokenCount
	if err := c.call(ctx, "tokens.count", params, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// ChunkID derives the BLAKE3 chunk identity hash from path, symbol, and content.
func (c *Client) ChunkID(ctx context.Context, path, symbol, content string) (string, error) {
	params := map[string]string{
		"path":    path,
		"symbol":  symbol,
		"content": content,
	}
	var res struct {
		Hex string `json:"hex"`
	}
	if err := c.call(ctx, "hash.chunk_id", params, &res); err != nil {
		return "", err
	}
	return res.Hex, nil
}

// Health checks the sidecar connection and returns health metadata.
func (c *Client) Health(ctx context.Context) (*sidecar.Health, error) {
	var h sidecar.Health
	if err := c.call(ctx, "health", nil, &h); err != nil {
		return nil, err
	}
	return &h, nil
}

// Complexity computes cyclomatic and cognitive complexity using the default client.
func Complexity(ctx context.Context, lang, source string) ([]FunctionMetrics, error) {
	return Default().Complexity(ctx, lang, source)
}

// FoldSignatures folds function signatures using the default client.
func FoldSignatures(ctx context.Context, lang, source string, keepSymbols []string) (*Folded, error) {
	return Default().FoldSignatures(ctx, lang, source, keepSymbols)
}

// LastCompleteNode locates the last complete node using the default client.
func LastCompleteNode(ctx context.Context, lang, buffer string) (int, error) {
	return Default().LastCompleteNode(ctx, lang, buffer)
}

// ChangedSymbols maps hunks to symbols using the default client.
func ChangedSymbols(ctx context.Context, lang, source string, hunks []Hunk) ([]ChangedSymbol, error) {
	return Default().ChangedSymbols(ctx, lang, source, hunks)
}

// CountTokens counts tokens using the default client.
func CountTokens(ctx context.Context, model, text string) (*TokenCount, error) {
	return Default().CountTokens(ctx, model, text)
}

// ChunkID computes the BLAKE3 chunk ID using the default client.
func ChunkID(ctx context.Context, path, symbol, content string) (string, error) {
	return Default().ChunkID(ctx, path, symbol, content)
}
