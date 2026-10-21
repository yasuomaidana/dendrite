package rag

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	InstallHint = "install it with `cargo install --locked --git https://github.com/yasuomaidana/dendrite-rag-rs` " +
		"(add `--features candle`, `oxillama` or `model2vec` for in-process embedders) or point DENDRITE_DENDRITE_RAG_BIN at a build"
)

// ErrSidecarUnavailable means the dendrite-rag binary could not be found or started.
var ErrSidecarUnavailable = errors.New("dendrite-rag sidecar unavailable")

// Retriever indexes a project through the sidecar and returns the chunks most relevant to
// a question.
type Retriever struct {
	Locator     sidecar.Locator
	EmbedBudget time.Duration
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
}

// NewRetriever returns a Retriever that searches the default sidecar locations.
func NewRetriever() *Retriever {
	return &Retriever{Locator: sidecar.DefaultLocator(), EmbedBudget: DefaultEmbedBudget}
}

// Retrieve brings the index of projectPath up to date (rebuilding it when refresh is set)
// and returns the best opts.TopK chunks for question. A projectPath naming a file
// searches only that file.
func (r *Retriever) Retrieve(ctx context.Context, projectPath, question string, opts SearchOptions, refresh bool) (Retrieval, error) {
	return r.run(ctx, projectPath, refresh, func(c *sidecar.Client, root string, only []string, out *Retrieval) error {
		hits, err := search(ctx, c, root, question, only, opts)
		out.Chunks = toChunks(projectPath, only, hits)
		return err
	})
}

// Context brings the index up to date and returns a context pack for question: the best
// spans plus their symbol-graph neighbours, packed into tokenBudget estimated tokens. For a
// single-file projectPath it searches within that file.
func (r *Retriever) Context(ctx context.Context, projectPath, question string, tokenBudget int, refresh bool) (Retrieval, error) {
	return r.run(ctx, projectPath, refresh, func(c *sidecar.Client, root string, only []string, out *Retrieval) error {
		if len(only) > 0 {
			hits, err := search(ctx, c, root, question, only, DefaultSearchOptions())
			out.Chunks = toChunks(projectPath, only, hits)
			return err
		}
		params := map[string]interface{}{"root": root, "query": question}
		if tokenBudget > 0 {
			params["token_budget"] = tokenBudget
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

type sidecarIndexReport struct {
	FilesChanged int    `json:"files_changed"`
	FilesRemoved int    `json:"files_removed"`
	EmbedPending int    `json:"embed_pending"`
	EmbedError   string `json:"embed_error"`
}

type sidecarHit struct {
	ChunkID   string   `json:"chunk_id"`
	Path      string   `json:"path"`
	StartLine int      `json:"start_line"`
	EndLine   int      `json:"end_line"`
	Scope     []string `json:"scope"`
	Score     float32  `json:"score"`
	Content   string   `json:"content"`
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
	var res struct {
		Hits []sidecarHit `json:"hits"`
	}
	if err := c.Call(ctx, "search", params, &res); err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	return res.Hits, nil
}

// run starts the sidecar, indexes the project and hands the client to query.
func (r *Retriever) run(ctx context.Context, projectPath string, refresh bool, query func(c *sidecar.Client, root string, only []string, out *Retrieval) error) (Retrieval, error) {
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
	c, err := sidecar.Start(ctx, bin, SidecarMajor)
	if err != nil {
		return Retrieval{}, fmt.Errorf("%w: %s: %v", ErrSidecarUnavailable, bin, err)
	}
	defer func() { _ = c.Close() }()

	indexParams := map[string]interface{}{"root": root, "full": refresh}
	if r.EmbedBudget > 0 {
		indexParams["embed_budget_ms"] = r.EmbedBudget.Milliseconds()
	}
	var report sidecarIndexReport
	if err := c.Call(ctx, "index", indexParams, &report); err != nil {
		return Retrieval{}, fmt.Errorf("index %s: %w", root, err)
	}
	out := Retrieval{
		IndexStatus:  "cached",
		Sidecar:      bin,
		EmbedPending: report.EmbedPending,
		EmbedError:   report.EmbedError,
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
		})
	}
	return chunks
}
