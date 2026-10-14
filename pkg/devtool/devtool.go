package devtool

import (
	"context"
	"fmt"
	"strings"
	"time"

	"Dendrite/pkg/ollama"
	"Dendrite/pkg/tracker"
)

// PromptMetrics represents character and estimated token volume for a prompt.
type PromptMetrics struct {
	CharacterCount  int     `json:"character_count"`
	WordCount       int     `json:"word_count"`
	EstimatedTokens int     `json:"estimated_tokens"`
	CompactionRatio float64 `json:"compaction_ratio"`
}

// AuditPrompt analyzes a prompt payload and returns size and token estimates.
func AuditPrompt(prompt string) PromptMetrics {
	trimmed := strings.TrimSpace(prompt)
	chars := len(trimmed)
	words := len(strings.Fields(trimmed))
	// Standard rule-of-thumb: ~4 characters per token for English text
	estimated := chars / 4
	if chars > 0 && estimated == 0 {
		estimated = 1
	}

	return PromptMetrics{
		CharacterCount:  chars,
		WordCount:       words,
		EstimatedTokens: estimated,
		CompactionRatio: 1.0,
	}
}

// ProbeOllama performs an on-the-fly health probe and model discovery on an Ollama endpoint.
func ProbeOllama(ctx context.Context, endpoint string) (*ollama.HealthStatus, []ollama.ModelInfo, error) {
	if endpoint == "" {
		endpoint = ollama.DefaultBaseURL
	}
	client := ollama.NewClient(
		ollama.WithBaseURL(endpoint),
		ollama.WithTimeout(5*time.Second),
	)

	status, err := client.CheckHealth(ctx)
	if err != nil {
		return status, nil, err
	}

	models, err := client.ListModels(ctx)
	if err != nil {
		return status, nil, err
	}

	return status, models, nil
}

// UpdateTicketInMarkdown updates a ticket status programmatically in DEV_STATUS.md.
func UpdateTicketInMarkdown(ctx context.Context, mdPath string, id string, status tracker.Status) error {
	tr := tracker.NewMemoryTracker(mdPath)
	if err := tr.LoadFromMarkdown(mdPath); err != nil {
		return fmt.Errorf("failed to load status file: %w", err)
	}

	if err := tr.UpdateStatus(ctx, tracker.TicketID(id), status); err != nil {
		return fmt.Errorf("failed to update ticket status: %w", err)
	}

	if err := tr.Sync(ctx); err != nil {
		return fmt.Errorf("failed to sync markdown file: %w", err)
	}

	return nil
}
