package rag

import (
	"context"
	"fmt"

	"Dendrite/pkg/sidecar"
)

// Index triggers an explicit project indexing pass through dendrite-rag.
func (r *Retriever) Index(ctx context.Context, projectPath string, opts IndexOptions) (IndexReport, error) {
	if projectPath == "" {
		projectPath = "."
	}
	root, _, err := projectRoot(projectPath)
	if err != nil {
		return IndexReport{}, err
	}
	bin, err := r.Locator.Locate(SidecarBinary)
	if err != nil {
		return IndexReport{}, fmt.Errorf("%w: %v; %s", ErrSidecarUnavailable, err, InstallHint)
	}
	c, err := sidecar.StartWith(ctx, bin, SidecarMajor, r.Env)
	if err != nil {
		return IndexReport{}, fmt.Errorf("%w: %s: %v", ErrSidecarUnavailable, bin, err)
	}
	defer func() { _ = c.Close() }()

	params := map[string]interface{}{"root": root, "full": opts.Full}
	if opts.EmbedBudgetMS > 0 {
		params["embed_budget_ms"] = opts.EmbedBudgetMS
	}
	if opts.EmbedBudgetTokens > 0 {
		params["embed_budget_tokens"] = opts.EmbedBudgetTokens
	}
	if opts.Focus != "" {
		params["focus"] = opts.Focus
	}
	var rep IndexReport
	if err := c.Call(ctx, "index", params, &rep); err != nil {
		return IndexReport{}, fmt.Errorf("index: %w", err)
	}
	return rep, nil
}

// Status returns the indexing metrics, vector counts, and coverage for projectPath.
func (r *Retriever) Status(ctx context.Context, projectPath string) (StatusReport, error) {
	if projectPath == "" {
		projectPath = "."
	}
	root, _, err := projectRoot(projectPath)
	if err != nil {
		return StatusReport{}, err
	}
	bin, err := r.Locator.Locate(SidecarBinary)
	if err != nil {
		return StatusReport{}, fmt.Errorf("%w: %v; %s", ErrSidecarUnavailable, err, InstallHint)
	}
	c, err := sidecar.StartWith(ctx, bin, SidecarMajor, r.Env)
	if err != nil {
		return StatusReport{}, fmt.Errorf("%w: %s: %v", ErrSidecarUnavailable, bin, err)
	}
	defer func() { _ = c.Close() }()

	var st StatusReport
	if err := c.Call(ctx, "status", map[string]interface{}{"root": root}, &st); err != nil {
		return StatusReport{}, fmt.Errorf("status: %w", err)
	}
	return st, nil
}

// Settings returns the active configuration (file + env overrides) for projectPath.
func (r *Retriever) Settings(ctx context.Context, projectPath string) (SettingsReport, error) {
	if projectPath == "" {
		projectPath = "."
	}
	root, _, err := projectRoot(projectPath)
	if err != nil {
		return SettingsReport{}, err
	}
	bin, err := r.Locator.Locate(SidecarBinary)
	if err != nil {
		return SettingsReport{}, fmt.Errorf("%w: %v; %s", ErrSidecarUnavailable, err, InstallHint)
	}
	c, err := sidecar.StartWith(ctx, bin, SidecarMajor, r.Env)
	if err != nil {
		return SettingsReport{}, fmt.Errorf("%w: %s: %v", ErrSidecarUnavailable, bin, err)
	}
	defer func() { _ = c.Close() }()

	var s SettingsReport
	if err := c.Call(ctx, "settings", map[string]interface{}{"root": root}, &s); err != nil {
		return SettingsReport{}, fmt.Errorf("settings: %w", err)
	}
	return s, nil
}

// Embedders lists the embedding backends supported by the current dendrite-rag build.
func (r *Retriever) Embedders(ctx context.Context) ([]EmbedderBackend, error) {
	bin, err := r.Locator.Locate(SidecarBinary)
	if err != nil {
		return nil, fmt.Errorf("%w: %v; %s", ErrSidecarUnavailable, err, InstallHint)
	}
	c, err := sidecar.StartWith(ctx, bin, SidecarMajor, r.Env)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrSidecarUnavailable, bin, err)
	}
	defer func() { _ = c.Close() }()

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
	if projectPath == "" {
		projectPath = "."
	}
	root, _, err := projectRoot(projectPath)
	if err != nil {
		return err
	}
	bin, err := r.Locator.Locate(SidecarBinary)
	if err != nil {
		return fmt.Errorf("%w: %v; %s", ErrSidecarUnavailable, err, InstallHint)
	}
	c, err := sidecar.StartWith(ctx, bin, SidecarMajor, r.Env)
	if err != nil {
		return fmt.Errorf("%w: %s: %v", ErrSidecarUnavailable, bin, err)
	}
	defer func() { _ = c.Close() }()

	var res map[string]interface{}
	if err := c.Call(ctx, "purge", map[string]interface{}{"root": root}, &res); err != nil {
		return fmt.Errorf("purge: %w", err)
	}
	return nil
}

// Symbols finds chunk definitions or references for a symbol name.
func (r *Retriever) Symbols(ctx context.Context, projectPath, name string, refs bool) ([]Chunk, error) {
	if projectPath == "" {
		projectPath = "."
	}
	root, _, err := projectRoot(projectPath)
	if err != nil {
		return nil, err
	}
	bin, err := r.Locator.Locate(SidecarBinary)
	if err != nil {
		return nil, fmt.Errorf("%w: %v; %s", ErrSidecarUnavailable, err, InstallHint)
	}
	c, err := sidecar.StartWith(ctx, bin, SidecarMajor, r.Env)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrSidecarUnavailable, bin, err)
	}
	defer func() { _ = c.Close() }()

	method := "symbols.definitions"
	if refs {
		method = "symbols.references"
	}
	var res struct {
		Hits []sidecarHit `json:"hits"`
	}
	if err := c.Call(ctx, method, map[string]interface{}{"root": root, "name": name}, &res); err != nil {
		return nil, fmt.Errorf("%s: %w", method, err)
	}
	return toChunks(projectPath, nil, res.Hits), nil
}

// Grep runs regex search over indexable repository files.
func (r *Retriever) Grep(ctx context.Context, projectPath, pattern string, paths []string, maxResults int) ([]GrepMatch, error) {
	if projectPath == "" {
		projectPath = "."
	}
	root, _, err := projectRoot(projectPath)
	if err != nil {
		return nil, err
	}
	bin, err := r.Locator.Locate(SidecarBinary)
	if err != nil {
		return nil, fmt.Errorf("%w: %v; %s", ErrSidecarUnavailable, err, InstallHint)
	}
	c, err := sidecar.StartWith(ctx, bin, SidecarMajor, r.Env)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrSidecarUnavailable, bin, err)
	}
	defer func() { _ = c.Close() }()

	params := map[string]interface{}{"root": root, "pattern": pattern}
	if len(paths) > 0 {
		params["paths"] = paths
	}
	if maxResults > 0 {
		params["max_results"] = maxResults
	}
	var res struct {
		Matches []GrepMatch `json:"matches"`
	}
	if err := c.Call(ctx, "grep", params, &res); err != nil {
		return nil, fmt.Errorf("grep: %w", err)
	}
	return res.Matches, nil
}

// History searches Git commit messages and per-file diffs.
func (r *Retriever) History(ctx context.Context, projectPath, query string, topK int) ([]HistoryHit, error) {
	if projectPath == "" {
		projectPath = "."
	}
	root, _, err := projectRoot(projectPath)
	if err != nil {
		return nil, err
	}
	bin, err := r.Locator.Locate(SidecarBinary)
	if err != nil {
		return nil, fmt.Errorf("%w: %v; %s", ErrSidecarUnavailable, err, InstallHint)
	}
	c, err := sidecar.StartWith(ctx, bin, SidecarMajor, r.Env)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrSidecarUnavailable, bin, err)
	}
	defer func() { _ = c.Close() }()

	if topK <= 0 {
		topK = 10
	}
	var res struct {
		Hits []HistoryHit `json:"hits"`
	}
	if err := c.Call(ctx, "history.search", map[string]interface{}{"root": root, "query": query, "top_k": topK}, &res); err != nil {
		return nil, fmt.Errorf("history.search: %w", err)
	}
	return res.Hits, nil
}

// Sync replays commits journaled while the shared tier was unreachable (edge mode).
func (r *Retriever) Sync(ctx context.Context, projectPath string) (int, error) {
	if projectPath == "" {
		projectPath = "."
	}
	root, _, err := projectRoot(projectPath)
	if err != nil {
		return 0, err
	}
	bin, err := r.Locator.Locate(SidecarBinary)
	if err != nil {
		return 0, fmt.Errorf("%w: %v; %s", ErrSidecarUnavailable, err, InstallHint)
	}
	c, err := sidecar.StartWith(ctx, bin, SidecarMajor, r.Env)
	if err != nil {
		return 0, fmt.Errorf("%w: %s: %v", ErrSidecarUnavailable, bin, err)
	}
	defer func() { _ = c.Close() }()

	var res struct {
		Pending int `json:"pending"`
	}
	if err := c.Call(ctx, "sync", map[string]interface{}{"root": root}, &res); err != nil {
		return 0, fmt.Errorf("sync: %w", err)
	}
	return res.Pending, nil
}

// Pull fetches every missing embedding from the shared tier for offline use.
func (r *Retriever) Pull(ctx context.Context, projectPath string) (PullReport, error) {
	if projectPath == "" {
		projectPath = "."
	}
	root, _, err := projectRoot(projectPath)
	if err != nil {
		return PullReport{}, err
	}
	bin, err := r.Locator.Locate(SidecarBinary)
	if err != nil {
		return PullReport{}, fmt.Errorf("%w: %v; %s", ErrSidecarUnavailable, err, InstallHint)
	}
	c, err := sidecar.StartWith(ctx, bin, SidecarMajor, r.Env)
	if err != nil {
		return PullReport{}, fmt.Errorf("%w: %s: %v", ErrSidecarUnavailable, bin, err)
	}
	defer func() { _ = c.Close() }()

	var rep PullReport
	if err := c.Call(ctx, "pull", map[string]interface{}{"root": root}, &rep); err != nil {
		return PullReport{}, fmt.Errorf("pull: %w", err)
	}
	return rep, nil
}

// Push uploads the local index to the PostgreSQL shared tier and compacts it.
func (r *Retriever) Push(ctx context.Context, projectPath string, dryRun bool) (PushReport, error) {
	if projectPath == "" {
		projectPath = "."
	}
	root, _, err := projectRoot(projectPath)
	if err != nil {
		return PushReport{}, err
	}
	bin, err := r.Locator.Locate(SidecarBinary)
	if err != nil {
		return PushReport{}, fmt.Errorf("%w: %v; %s", ErrSidecarUnavailable, err, InstallHint)
	}
	c, err := sidecar.StartWith(ctx, bin, SidecarMajor, r.Env)
	if err != nil {
		return PushReport{}, fmt.Errorf("%w: %s: %v", ErrSidecarUnavailable, bin, err)
	}
	defer func() { _ = c.Close() }()

	var rep PushReport
	if err := c.Call(ctx, "push", map[string]interface{}{"root": root, "dry_run": dryRun}, &rep); err != nil {
		return PushReport{}, fmt.Errorf("push: %w", err)
	}
	return rep, nil
}
