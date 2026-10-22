package rag

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"Dendrite/pkg/sidecar"
)

// Retrieval is served by the dendrite-rag sidecar (repository dendrite-rag-rs): cAST
// chunking, code-aware BM25, dense vectors from the embedder the project chose
// (.dendrite/rag.toml), symbol names, rank fusion and token-budgeted context packs.
// Dendrite only coordinates: it locates the sidecar, brings the index up to date and
// passes results on.
const (
	SidecarBinary = "dendrite-rag"
	SidecarMajor  = 1
	// DefaultEmbedBudget bounds the sidecar's embedding work per interactive call. The
	// lexical and symbol indexes are always brought up to date; vectors for the rest of the
	// project are added by later calls, or all at once by `dendrite-rag index <project>`.
	DefaultEmbedBudget = 3 * time.Second
	// InstallHint tells users how to get the sidecar when it is missing.
	InstallHint = "install it with `cargo install --locked --git https://github.com/yasuomaidana/dendrite-rag-rs --features candle-metal` " +
		"(Apple GPU; `candle-cuda` on NVIDIA, `candle` for CPU) or point DENDRITE_DENDRITE_RAG_BIN at a build"
)

// ErrSidecarUnavailable means the dendrite-rag binary could not be found or started.
var ErrSidecarUnavailable = errors.New("dendrite-rag sidecar unavailable")

// Retriever indexes a project through the sidecar and returns the chunks most relevant to
// a question.
type Retriever struct {
	Locator     sidecar.Locator
	EmbedBudget time.Duration
	// Env holds extra KEY=VALUE lines for the sidecar's environment.
	Env []string
}

// Retrieval is the outcome of one Retrieve or Context call.
type Retrieval struct {
	Chunks []Chunk
	// IndexStatus is "cached", "incremental" or "refreshed".
	IndexStatus string
	// Sidecar is the binary that answered.
	Sidecar string
	// EmbedPending counts chunks still without vectors (0 when complete).
	EmbedPending int
	// EmbedError reports why embedding failed in this call (lexical results still apply).
	EmbedError string
	// Tokens estimates the size of a context pack (Context only).
	Tokens int
	// EmbedCoverage is the share of chunks with a vector, reported when some are still
	// waiting (1 otherwise).
	EmbedCoverage float64
	// SpawnMS and IndexMS time starting the sidecar and the index call; EmbedMS is the part
	// of the index call spent embedding.
	SpawnMS, IndexMS, EmbedMS int64
}

// AskProtocolMinor is the sidecar protocol minor version `ask` needs (1.6: corpora, focus).
const AskProtocolMinor = 6

// DocsShareProtocolMinor is the first sidecar minor that takes `docs_share` on `context`
// and `ask`.
const DocsShareProtocolMinor = 7

// ErrSidecarTooOld means the dendrite-rag binary predates a method Dendrite needs.
var ErrSidecarTooOld = errors.New("dendrite-rag sidecar too old")

// NewRetriever returns a Retriever that searches the default sidecar locations.
func NewRetriever() *Retriever {
	return &Retriever{Locator: sidecar.DefaultLocator(), EmbedBudget: DefaultEmbedBudget}
}

// Retrieve brings the index of projectPath up to date (rebuilding it when refresh is set)
// and returns the best opts.TopK chunks for question. A projectPath naming a file
// searches only that file.
func (r *Retriever) Retrieve(ctx context.Context, projectPath, question string, opts SearchOptions, refresh bool) (Retrieval, error) {
	return r.run(ctx, projectPath, refresh, question, func(c *sidecar.Client, root string, only []string, out *Retrieval) error {
		hits, err := search(ctx, c, root, question, only, opts)
		out.Chunks = toChunks(projectPath, only, hits)
		return err
	})
}

// Context brings the index up to date and returns a context pack for question: the best
// spans plus their symbol-graph neighbours, packed into tokenBudget estimated tokens. For a
// single-file projectPath it searches within that file.
func (r *Retriever) Context(ctx context.Context, projectPath, question string, tokenBudget int, refresh bool) (Retrieval, error) {
	return r.ContextWith(ctx, projectPath, question, tokenBudget, ContextOptions{}, refresh)
}

// ContextWith is Context with the corpus and detail of opts.
func (r *Retriever) ContextWith(ctx context.Context, projectPath, question string, tokenBudget int, opts ContextOptions, refresh bool) (Retrieval, error) {
	return r.run(ctx, projectPath, refresh, question, func(c *sidecar.Client, root string, only []string, out *Retrieval) error {
		if len(only) > 0 {
			so := DefaultSearchOptions()
			so.Corpus = opts.Corpus
			hits, err := search(ctx, c, root, question, only, so)
			out.Chunks = toChunks(projectPath, only, hits)
			return err
		}
		params := map[string]interface{}{"root": root, "query": question}
		if tokenBudget > 0 {
			params["token_budget"] = tokenBudget
		}
		if opts.Corpus != "" {
			params["corpus"] = opts.Corpus
		}
		if opts.Detail != "" {
			params["detail"] = opts.Detail
		}
		if opts.DocsShare != nil {
			if protocolMinor(c.Health.Protocol) < DocsShareProtocolMinor {
				return fmt.Errorf("%w: %s speaks protocol %s; a documentation share needs 1.%d or later; %s",
					ErrSidecarTooOld, c.Path, c.Health.Protocol, DocsShareProtocolMinor, InstallHint)
			}
			params["docs_share"] = *opts.DocsShare
		}
		var res struct {
			Spans  []sidecarHit `json:"spans"`
			Tokens int          `json:"tokens"`
		}
		if err := c.Call(ctx, "context", params, &res); err != nil {
			return fmt.Errorf("context: %w", err)
		}
		out.Chunks, out.Tokens = toChunks(projectPath, nil, res.Spans), res.Tokens
		return nil
	})
}

// Show returns the full text of chunks by id: the bodies behind a "map" context pack's
// outlines.
func (r *Retriever) Show(ctx context.Context, projectPath string, ids []string) ([]Chunk, error) {
	var chunks []Chunk
	_, err := r.run(ctx, projectPath, false, "", func(c *sidecar.Client, root string, only []string, out *Retrieval) error {
		var res struct {
			Hits []sidecarHit `json:"hits"`
		}
		if err := c.Call(ctx, "chunks.get", map[string]interface{}{"root": root, "ids": ids}, &res); err != nil {
			return fmt.Errorf("chunks.get: %w", err)
		}
		chunks = toChunks(projectPath, only, res.Hits)
		return nil
	})
	return chunks, err
}

type sidecarIndexReport struct {
	FilesChanged int    `json:"files_changed"`
	FilesRemoved int    `json:"files_removed"`
	EmbedPending int    `json:"embed_pending"`
	EmbedError   string `json:"embed_error"`
	EmbedMS      int64  `json:"embed_ms"`
}

type sidecarHit struct {
	ChunkID   string   `json:"chunk_id"`
	Path      string   `json:"path"`
	StartLine int      `json:"start_line"`
	EndLine   int      `json:"end_line"`
	Scope     []string `json:"scope"`
	Score     float32  `json:"score"`
	Content   string   `json:"content"`
	// Outline marks content that is the span's outline, not its text (detail "map").
	Outline bool `json:"outline"`
}

func search(ctx context.Context, c *sidecar.Client, root, question string, only []string, opts SearchOptions) ([]sidecarHit, error) {
	topK := opts.TopK
	if topK <= 0 {
		topK = DefaultSearchOptions().TopK
	}
	params := map[string]interface{}{"root": root, "query": question, "top_k": topK}
	if len(only) > 0 {
		params["paths"] = only
	}
	if opts.MinScore > 0 {
		params["min_score"] = opts.MinScore
	}
	if opts.Corpus != "" {
		params["corpus"] = opts.Corpus
	}
	var res struct {
		Hits []sidecarHit `json:"hits"`
	}
	if err := c.Call(ctx, "search", params, &res); err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	return res.Hits, nil
}

// run starts the sidecar, indexes the project and hands the client to query. Under the
// embedding budget, the chunks focus (the question) matches are embedded first.
func (r *Retriever) run(ctx context.Context, projectPath string, refresh bool, focus string, query func(c *sidecar.Client, root string, only []string, out *Retrieval) error) (Retrieval, error) {
	if projectPath == "" {
		projectPath = "."
	}
	root, only, err := projectRoot(projectPath)
	if err != nil {
		return Retrieval{}, err
	}
	bin, err := r.Locator.Locate(SidecarBinary)
	if err != nil {
		return Retrieval{}, fmt.Errorf("%w: %v; %s", ErrSidecarUnavailable, err, InstallHint)
	}
	started := time.Now()
	c, err := sidecar.StartWith(ctx, bin, SidecarMajor, r.Env)
	if err != nil {
		return Retrieval{}, fmt.Errorf("%w: %s: %v", ErrSidecarUnavailable, bin, err)
	}
	defer func() { _ = c.Close() }()
	spawn := time.Since(started).Milliseconds()

	indexParams := map[string]interface{}{"root": root, "full": refresh}
	if r.EmbedBudget > 0 {
		indexParams["embed_budget_ms"] = r.EmbedBudget.Milliseconds()
		if focus != "" && protocolMinor(c.Health.Protocol) >= AskProtocolMinor {
			indexParams["focus"] = focus
		}
	}
	started = time.Now()
	var report sidecarIndexReport
	if err := c.Call(ctx, "index", indexParams, &report); err != nil {
		return Retrieval{}, fmt.Errorf("index %s: %w", root, err)
	}
	out := Retrieval{
		IndexStatus:   "cached",
		Sidecar:       bin,
		EmbedPending:  report.EmbedPending,
		EmbedError:    report.EmbedError,
		EmbedCoverage: 1,
		SpawnMS:       spawn,
		IndexMS:       time.Since(started).Milliseconds(),
		EmbedMS:       report.EmbedMS,
	}
	if report.EmbedPending > 0 {
		var st struct {
			EmbedCoverage *float64 `json:"embed_coverage"`
		}
		if err := c.Call(ctx, "status", map[string]interface{}{"root": root}, &st); err == nil && st.EmbedCoverage != nil {
			out.EmbedCoverage = *st.EmbedCoverage
		}
	}
	switch {
	case refresh:
		out.IndexStatus = "refreshed"
	case report.FilesChanged > 0 || report.FilesRemoved > 0:
		out.IndexStatus = "incremental"
	}
	if err := query(c, root, only, &out); err != nil {
		return Retrieval{}, err
	}
	return out, nil
}

// singleFile is the directory-relative path a single-file projectPath restricts retrieval
// to (nil for a directory).
func singleFile(projectPath string) []string {
	_, only, err := projectRoot(projectPath)
	if err != nil {
		return nil
	}
	return only
}

// protocolMinor is the minor version of a semver protocol string (0 when unparsable).
func protocolMinor(semver string) int {
	parts := strings.SplitN(semver, ".", 3)
	if len(parts) < 2 {
		return 0
	}
	n, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0
	}
	return n
}

// projectRoot returns the absolute directory to index and, when projectPath is a file,
// the directory-relative path that restricts the search to it.
func projectRoot(projectPath string) (string, []string, error) {
	abs, err := filepath.Abs(projectPath)
	if err != nil {
		return "", nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", nil, fmt.Errorf("project path: %w", err)
	}
	if info.IsDir() {
		return abs, nil, nil
	}
	return filepath.Dir(abs), []string{filepath.Base(abs)}, nil
}

// toChunks maps sidecar hits to chunks whose FilePath is relative to projectPath as given
// (or to the directory of a single-file projectPath).
func toChunks(projectPath string, only []string, hits []sidecarHit) []Chunk {
	base := projectPath
	if len(only) > 0 {
		base = filepath.Dir(projectPath)
	}
	chunks := make([]Chunk, 0, len(hits))
	for _, h := range hits {
		chunks = append(chunks, Chunk{
			ID:        h.ChunkID,
			FilePath:  filepath.Join(base, filepath.FromSlash(h.Path)),
			Heading:   strings.Join(h.Scope, " › "),
			Content:   h.Content,
			LineStart: h.StartLine,
			LineEnd:   h.EndLine,
			Score:     h.Score,
			Outline:   h.Outline,
		})
	}
	return chunks
}
