package rag

import (
	"context"
	"fmt"
)

// Index triggers an explicit project indexing pass through dendrite-rag.
func (r *Retriever) Index(ctx context.Context, projectPath string, opts IndexOptions) (IndexReport, error) {
	params := map[string]interface{}{"full": opts.Full}
	setPositive(params, "embed_budget_ms", int(opts.EmbedBudgetMS))
	setPositive(params, "embed_budget_tokens", int(opts.EmbedBudgetTokens))
	if opts.Focus != "" {
		params["focus"] = opts.Focus
	}
	var rep IndexReport
	err := r.callRoot(ctx, projectPath, "index", params, &rep)
	return rep, err
}

// Status returns the indexing metrics, vector counts, and coverage for projectPath.
func (r *Retriever) Status(ctx context.Context, projectPath string) (StatusReport, error) {
	var st StatusReport
	err := r.callRoot(ctx, projectPath, "status", nil, &st)
	return st, err
}

// Settings returns the active configuration (file + env overrides) for projectPath.
func (r *Retriever) Settings(ctx context.Context, projectPath string) (SettingsReport, error) {
	var s SettingsReport
	err := r.callRoot(ctx, projectPath, "settings", nil, &s)
	return s, err
}

// Embedders lists the embedding backends supported by the current dendrite-rag build.
func (r *Retriever) Embedders(ctx context.Context) ([]EmbedderBackend, error) {
	c, err := r.dial(ctx)
	if err != nil {
		return nil, err
	}
	var res struct {
		Backends []EmbedderBackend `json:"backends"`
	}
	if err := c.Call(ctx, "embedders", nil, &res); err != nil {
		return nil, fmt.Errorf("embedders: %w", err)
	}
	return res.Backends, nil
}

// Purge removes the local index and cache directory for projectPath.
func (r *Retriever) Purge(ctx context.Context, projectPath string) error {
	var res map[string]interface{}
	return r.callRoot(ctx, projectPath, "purge", nil, &res)
}

// Symbols finds chunk definitions or references for a symbol name.
func (r *Retriever) Symbols(ctx context.Context, projectPath, name string, refs bool) ([]Chunk, error) {
	method := "symbols.definitions"
	if refs {
		method = "symbols.references"
	}
	var res struct {
		Hits []sidecarHit `json:"hits"`
	}
	if err := r.callRoot(ctx, projectPath, method, map[string]interface{}{"name": name}, &res); err != nil {
		return nil, err
	}
	return toChunks(projectPath, nil, res.Hits), nil
}

// Grep runs regex search over indexable repository files.
func (r *Retriever) Grep(ctx context.Context, projectPath, pattern string, paths []string, maxResults int) ([]GrepMatch, error) {
	params := map[string]interface{}{"pattern": pattern}
	if len(paths) > 0 {
		params["paths"] = paths
	}
	setPositive(params, "max_results", maxResults)
	var res struct {
		Matches []GrepMatch `json:"matches"`
	}
	if err := r.callRoot(ctx, projectPath, "grep", params, &res); err != nil {
		return nil, err
	}
	return res.Matches, nil
}

// History searches Git commit messages and per-file diffs.
func (r *Retriever) History(ctx context.Context, projectPath, query string, topK int) ([]HistoryHit, error) {
	if topK <= 0 {
		topK = 10
	}
	var res struct {
		Hits []HistoryHit `json:"hits"`
	}
	if err := r.callRoot(ctx, projectPath, "history.search", map[string]interface{}{"query": query, "top_k": topK}, &res); err != nil {
		return nil, err
	}
	return res.Hits, nil
}

// Sync replays commits journaled while the shared tier was unreachable (edge mode).
func (r *Retriever) Sync(ctx context.Context, projectPath string) (int, error) {
	var res struct {
		Pending int `json:"pending"`
	}
	if err := r.callRoot(ctx, projectPath, "sync", nil, &res); err != nil {
		return 0, err
	}
	return res.Pending, nil
}

// Pull fetches every missing embedding from the shared tier for offline use.
func (r *Retriever) Pull(ctx context.Context, projectPath string) (PullReport, error) {
	var rep PullReport
	err := r.callRoot(ctx, projectPath, "pull", nil, &rep)
	return rep, err
}

// Push uploads the local index to the PostgreSQL shared tier and compacts it.
func (r *Retriever) Push(ctx context.Context, projectPath string, dryRun bool) (PushReport, error) {
	var rep PushReport
	err := r.callRoot(ctx, projectPath, "push", map[string]interface{}{"dry_run": dryRun}, &rep)
	return rep, err
}
