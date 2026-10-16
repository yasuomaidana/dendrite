package rag

import (
	"context"
	"io"
	"time"

	"Dendrite/pkg/inference"
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
}

// SearchOptions governs the retrieval parameters.
type SearchOptions struct {
	TopK      int     `json:"top_k"`
	MinScore  float32 `json:"min_score"`
	MaxTokens int     `json:"max_tokens"`
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
	Question        string    `json:"question"`
	ProjectPath     string    `json:"project_path,omitempty"`
	AutoApprovePaid bool      `json:"auto_approve_paid"` // -y / --yes flag: bypass interactive human prompt upon failure
	ForcePaid       bool      `json:"force_paid"`        // --paid / --force-paid: bypass local LLM and query paid model directly
	HybridMode      bool      `json:"hybrid_mode"`       // --hybrid: local draft first, then paid refinement
	ServerURL       string    `json:"server_url,omitempty"` // --server: query centralized remote RAG service
	UpdateIndex     bool      `json:"update_index"`      // --update-index: force refresh of incremental RAG cache
	PaidModel       string    `json:"paid_model,omitempty"`
	LocalModel      string    `json:"local_model,omitempty"`
	TopK            int       `json:"top_k,omitempty"`
	Input           io.Reader `json:"-"` // for interactive prompting (defaults to os.Stdin)
	Output          io.Writer `json:"-"` // for interactive messages (defaults to os.Stdout)
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
}

// PaidCallerFunc defines the signature for invoking paid cloud LLM models.
type PaidCallerFunc func(ctx context.Context, prompt string, paidModel string) (*inference.CompletionResponse, error)
