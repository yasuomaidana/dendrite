package rag

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"Dendrite/pkg/sidecar"
)

// The dendrite-rag sidecar (repository dendrite-rag-rs) serves hybrid retrieval: code-aware
// BM25, dense vectors, symbol names and rank fusion over cAST chunks. Retriever uses it when
// it is installed and falls back to the in-process BM25 Indexer otherwise.
const (
	SidecarBinary = "dendrite-rag"
	SidecarMajor  = 1
	// DefaultEmbedBudget bounds the sidecar's embedding work per interactive call. The
	// lexical and symbol indexes are always brought up to date; vectors for the rest of the
	// project are added by later calls, or all at once by `dendrite-rag index <project>`.
	DefaultEmbedBudget = 3 * time.Second
	// SidecarEnv set to 0, off or false disables the sidecar.
	SidecarEnv = "DENDRITE_RAG_SIDECAR"
)

// Retriever indexes a project and returns the chunks most relevant to a question.
type Retriever struct {
	Locator     sidecar.Locator
	Indexer     *Indexer // in-process fallback
	EmbedBudget time.Duration
	Disabled    bool
}

// Retrieval is the outcome of one Retrieve call.
type Retrieval struct {
	Chunks []Chunk
	// IndexStatus is "cached", "incremental" or "refreshed".
	IndexStatus string
	// Outcome says whether the sidecar or the in-process fallback answered, and why.
	Outcome sidecar.Outcome
	// EmbedPending counts sidecar chunks still without vectors (0 when complete).
	EmbedPending int
}

// NewRetriever returns a Retriever that searches the default sidecar locations and falls
// back to idx.
func NewRetriever(idx *Indexer) *Retriever {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(SidecarEnv)))
	return &Retriever{
		Locator:     sidecar.DefaultLocator(),
		Indexer:     idx,
		EmbedBudget: DefaultEmbedBudget,
		Disabled:    v == "0" || v == "off" || v == "false",
	}
}

// Retrieve brings the index of projectPath up to date (rebuilding it when refresh is set)
// and returns the best chunks for question.
func (r *Retriever) Retrieve(ctx context.Context, projectPath, question string, opts SearchOptions, refresh bool) (Retrieval, error) {
	if projectPath == "" {
		projectPath = "."
	}
	reason := "disabled by " + SidecarEnv
	path := ""
	if !r.Disabled {
		got, p, err := r.viaSidecar(ctx, projectPath, question, opts, refresh)
		if err == nil {
			return got, nil
		}
		reason, path = err.Error(), p
	}
	got, err := r.inProcess(ctx, projectPath, question, opts, refresh)
	got.Outcome = sidecar.Outcome{Origin: sidecar.OriginFallback, Path: path, Reason: reason}
	return got, err
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

// viaSidecar runs index then search on one sidecar process. The returned path names the
// binary that was tried, for the fallback reason.
func (r *Retriever) viaSidecar(ctx context.Context, projectPath, question string, opts SearchOptions, refresh bool) (Retrieval, string, error) {
	root, err := filepath.Abs(projectPath)
	if err != nil {
		return Retrieval{}, "", err
	}
	bin, err := r.Locator.Locate(SidecarBinary)
	if err != nil {
		return Retrieval{}, "", err
	}
	c, err := sidecar.Start(ctx, bin, SidecarMajor)
	if err != nil {
		return Retrieval{}, bin, err
	}
	defer func() { _ = c.Close() }()

	indexParams := map[string]interface{}{"root": root, "full": refresh}
	if r.EmbedBudget > 0 {
		indexParams["embed_budget_ms"] = r.EmbedBudget.Milliseconds()
	}
	var report sidecarIndexReport
	if err := c.Call(ctx, "index", indexParams, &report); err != nil {
		return Retrieval{}, bin, fmt.Errorf("index: %w", err)
	}
	topK := opts.TopK
	if topK <= 0 {
		topK = DefaultSearchOptions().TopK
	}
	var res struct {
		Hits []sidecarHit `json:"hits"`
	}
	if err := c.Call(ctx, "search", map[string]interface{}{"root": root, "query": question, "top_k": topK}, &res); err != nil {
		return Retrieval{}, bin, fmt.Errorf("search: %w", err)
	}

	out := Retrieval{
		IndexStatus:  "cached",
		Outcome:      sidecar.Outcome{Origin: sidecar.OriginSidecar, Path: bin},
		EmbedPending: report.EmbedPending,
	}
	switch {
	case refresh:
		out.IndexStatus = "refreshed"
	case report.FilesChanged > 0 || report.FilesRemoved > 0:
		out.IndexStatus = "incremental"
	}
	for _, h := range res.Hits {
		out.Chunks = append(out.Chunks, Chunk{
			ID: h.ChunkID,
			// Same convention as the in-process Indexer: paths relative to projectPath as given.
			FilePath:  filepath.Join(projectPath, filepath.FromSlash(h.Path)),
			Heading:   strings.Join(h.Scope, " › "),
			Content:   h.Content,
			LineStart: h.StartLine,
			LineEnd:   h.EndLine,
			Score:     h.Score,
		})
	}
	return out, bin, nil
}

// inProcess is the BM25 fallback built into Dendrite.
func (r *Retriever) inProcess(ctx context.Context, projectPath, question string, opts SearchOptions, refresh bool) (Retrieval, error) {
	idx := r.Indexer
	if idx == nil {
		idx = NewIndexer()
	}
	status, indexErr := idx.IndexProjectIncremental(ctx, projectPath, refresh)
	chunks, err := idx.Retrieve(ctx, question, opts)
	if err == nil && indexErr != nil && len(chunks) == 0 {
		err = fmt.Errorf("indexing %s: %w", projectPath, indexErr)
	}
	return Retrieval{Chunks: chunks, IndexStatus: status}, err
}
