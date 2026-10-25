package rag

import (
	"io"
	"time"
)

// Chunk represents a segmented unit of document or code text indexed for retrieval.
type Chunk struct {
	ID        string  `json:"id"`
	FilePath  string  `json:"file_path"`
	Heading   string  `json:"heading,omitempty"`
	Content   string  `json:"content"`
	LineStart int     `json:"line_start"`
	LineEnd   int     `json:"line_end"`
	Score     float32 `json:"score,omitempty"`
	// Outline marks Content that is the chunk's outline (doc comment or heading and
	// declaration), not its text; Retriever.Show returns the text.
	Outline bool `json:"outline,omitempty"`
}

// SearchOptions governs the retrieval parameters.
type SearchOptions struct {
	TopK      int     `json:"top_k"`
	MinScore  float32 `json:"min_score"`
	MaxTokens int     `json:"max_tokens"`
	// Corpus is "code", "docs" or "all" (empty: the sidecar's default, "all").
	Corpus string `json:"corpus,omitempty"`
}

// ContextOptions shapes a context pack.
type ContextOptions struct {
	// Corpus is "code", "docs" or "all" (empty: the sidecar's default, "all").
	Corpus string `json:"corpus,omitempty"`
	// Detail is "auto" (text of retrieved spans, outlines of graph neighbours), "full" or
	// "map" (outlines only, bodies fetched with Retriever.Show); empty means "auto".
	Detail string `json:"detail,omitempty"`
	// DocsShare is the share of an "all" pack's budget kept for documentation, 0 to 1 (nil:
	// the project's [ask] docs_share or DENDRITE_RAG_ASK_DOCS_SHARE, else 0.2).
	DocsShare *float64 `json:"docs_share,omitempty"`
}

// DefaultSearchOptions returns sensible defaults for RAG snippet search.
func DefaultSearchOptions() SearchOptions {
	return SearchOptions{
		TopK:      4,
		MinScore:  0.01,
		MaxTokens: 2000,
	}
}

// AskRequest specifies the parameters for querying Dendrite about a project.
type AskRequest struct {
	Question        string `json:"question"`
	ProjectPath     string `json:"project_path,omitempty"`
	AutoApprovePaid bool   `json:"auto_approve_paid"`    // -y / --yes flag: bypass interactive human prompt upon failure
	ForcePaid       bool   `json:"force_paid"`           // --paid / --force-paid: bypass local LLM and query paid model directly
	HybridMode      bool   `json:"hybrid_mode"`          // --hybrid: local draft first, then paid refinement
	ServerURL       string `json:"server_url,omitempty"` // --server: query centralized remote RAG service
	UpdateIndex     bool   `json:"update_index"`         // --update-index: force refresh of incremental RAG cache
	PaidModel       string `json:"paid_model,omitempty"`
	LocalModel      string `json:"local_model,omitempty"`
	TopK            int    `json:"top_k,omitempty"`
	// Corpus is "code" (dendrite ask without -d), "docs", or "all" (the default: code and
	// documentation, with DocsShare of the context pack kept for documentation).
	Corpus string `json:"corpus,omitempty"`
	// DocsShare is the share of an "all" context pack kept for documentation, 0 to 1 (nil:
	// the project's [ask] docs_share or DENDRITE_RAG_ASK_DOCS_SHARE, else 0.2).
	DocsShare *float64  `json:"docs_share,omitempty"`
	Input     io.Reader `json:"-"` // for interactive prompting (defaults to os.Stdin)
	Output    io.Writer `json:"-"` // for interactive messages (defaults to os.Stdout)
}

// AskResult contains the answer synthesized by the RAG workflow.
type AskResult struct {
	Question           string        `json:"question"`
	Answer             string        `json:"answer"`
	LocalDraft         string        `json:"local_draft,omitempty"` // Captured during hybrid mode
	SourceTier         string        `json:"source_tier"`           // "local-rag", "paid-cloud", "hybrid", or "central-rag"
	Model              string        `json:"model"`
	Engine             string        `json:"engine"`
	Escalated          bool          `json:"escalated"`
	EscalationApproved bool          `json:"escalation_approved"`
	RetrievedChunks    []Chunk       `json:"retrieved_chunks"`
	PromptTokens       int           `json:"prompt_tokens"`
	OutputTokens       int           `json:"output_tokens"`
	Duration           time.Duration `json:"duration"`
	IndexStatus        string        `json:"index_status,omitempty"` // "cached", "refreshed", "incremental"
	Error              string        `json:"error,omitempty"`
	// Cached is set when the answer came from the sidecar's cache of identical prompts.
	Cached bool `json:"cached,omitempty"`
	// Abstained is set when [rerank] abstain found nothing likely relevant.
	Abstained bool `json:"abstained,omitempty"`
	// Timings break Duration down by stage (local runs only).
	Timings *AskTimings `json:"timings,omitempty"`
}

// AskTimings is where an answer spent its time, in milliseconds.
type AskTimings struct {
	// Spawn is starting the sidecar; Index is bringing the index up to date, of which
	// Embed was spent embedding.
	Spawn int64 `json:"spawn_ms"`
	Index int64 `json:"index_ms"`
	Embed int64 `json:"embed_ms"`
	// Retrieval is finding the context; Local and Paid are the model calls.
	Retrieval int64 `json:"retrieval_ms"`
	Local     int64 `json:"local_ms"`
	Paid      int64 `json:"paid_ms"`
}

// IndexOptions configures an index run.
type IndexOptions struct {
	Full              bool   `json:"full,omitempty"`
	EmbedBudgetMS     int64  `json:"embed_budget_ms,omitempty"`
	EmbedBudgetTokens int    `json:"embed_budget_tokens,omitempty"`
	Focus             string `json:"focus,omitempty"`
}

// IndexReport summarizes indexing work.
type IndexReport struct {
	FilesScanned          int    `json:"files_scanned"`
	FilesChanged          int    `json:"files_changed"`
	FilesRemoved          int    `json:"files_removed"`
	ChunksAdded           int    `json:"chunks_added"`
	ChunksRemoved         int    `json:"chunks_removed"`
	Embedded              int    `json:"embedded"`
	EmbedPending          int    `json:"embed_pending"`
	EmbedTokens           int    `json:"embed_tokens"`
	BudgetTokensRemaining *int   `json:"budget_tokens_remaining,omitempty"`
	EmbedError            string `json:"embed_error,omitempty"`
	OpenMS                int64  `json:"open_ms"`
	WalkMS                int64  `json:"walk_ms"`
	GraphMS               int64  `json:"graph_ms"`
	EmbedMS               int64  `json:"embed_ms"`
	DurationMS            int64  `json:"duration_ms"`
}

// StatusReport reports project index metrics.
type StatusReport struct {
	Files         int      `json:"files"`
	Chunks        int      `json:"chunks"`
	Vectors       int      `json:"vectors"`
	EmbedModel    string   `json:"embed_model"`
	EmbedCoverage *float64 `json:"embed_coverage,omitempty"`
	Warnings      []string `json:"warnings,omitempty"`
	MerkleRoot    string   `json:"merkle_root,omitempty"`
	IndexDir      string   `json:"index_dir,omitempty"`
	LastIndexed   *int64   `json:"last_indexed,omitempty"`
}

// SettingsReport reports the settings in effect for a project.
type SettingsReport struct {
	File       string                 `json:"file"`
	FileExists bool                   `json:"file_exists"`
	Settings   map[string]interface{} `json:"settings"`
}

// EmbedderBackend describes a supported embedding backend.
type EmbedderBackend struct {
	Backend      string  `json:"backend"`
	Compiled     bool    `json:"compiled"`
	DefaultModel *string `json:"default_model,omitempty"`
	DefaultURL   *string `json:"default_url,omitempty"`
	Description  string  `json:"description"`
}

// GrepMatch is one line matching a grep query.
type GrepMatch struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

// HistoryHit is one commit matching a history search.
type HistoryHit struct {
	SHA     string  `json:"sha"`
	Time    int64   `json:"time"`
	Author  string  `json:"author"`
	Subject string  `json:"subject"`
	Path    string  `json:"path"`
	Diff    string  `json:"diff"`
	Score   float32 `json:"score"`
}

// PushReport reports an index push to the shared tier.
type PushReport struct {
	DryRun             bool   `json:"dry_run"`
	Commit             string `json:"commit"`
	Branch             string `json:"branch"`
	Files              int    `json:"files"`
	Chunks             int    `json:"chunks"`
	EmbeddingsUploaded int    `json:"embeddings_uploaded"`
	Vectors            int    `json:"vectors"`
	Compaction         string `json:"compaction"`
}

// PullReport reports missing embeddings fetched from the shared tier.
type PullReport struct {
	Chunks  int `json:"chunks"`
	Cached  int `json:"cached"`
	Vectors int `json:"vectors"`
}
