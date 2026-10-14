package devtool

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"Dendrite/pkg/ollama"
	"Dendrite/pkg/tracker"
)

func TestAuditPrompt(t *testing.T) {
	metrics := AuditPrompt("   Hello, this is a test prompt for Dendrite.   ")
	if metrics.CharacterCount == 0 || metrics.WordCount == 0 || metrics.EstimatedTokens == 0 {
		t.Errorf("unexpected zero metrics: %+v", metrics)
	}

	emptyMetrics := AuditPrompt("")
	if emptyMetrics.EstimatedTokens != 0 {
		t.Errorf("expected 0 tokens for empty prompt, got %d", emptyMetrics.EstimatedTokens)
	}

	shortMetrics := AuditPrompt("hi")
	if shortMetrics.EstimatedTokens != 1 {
		t.Errorf("expected 1 token for short prompt, got %d", shortMetrics.EstimatedTokens)
	}
}

func TestProbeOllama(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			resp := ollama.ListModelsResponse{
				Models: []ollama.ModelInfo{{Name: "qwen2.5-coder:14b"}},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	status, models, err := ProbeOllama(context.Background(), ts.URL)
	if err != nil {
		t.Fatalf("probe failed: %v", err)
	}
	if !status.Reachable || len(models) != 1 {
		t.Errorf("unexpected status or models: %+v, models: %v", status, models)
	}

	// Test unreachable server error path
	statusErr, _, err := ProbeOllama(context.Background(), "http://127.0.0.1:54321")
	if err == nil {
		t.Error("expected error for bad endpoint, got nil")
	}
	if statusErr.Reachable {
		t.Error("expected reachable false on failure")
	}

	// Test default endpoint fallback when empty string
	_, _, _ = ProbeOllama(context.Background(), "")
}

func TestUpdateTicketInMarkdown(t *testing.T) {
	tmpDir := t.TempDir()
	mdPath := filepath.Join(tmpDir, "DEV_STATUS.md")
	content := "# Status\n\n### Milestone 1\n- [ ] **DEN-001**: Task 1 (`pkg/test`) *(IN PROGRESS)*\n"
	if err := os.WriteFile(mdPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test md: %v", err)
	}

	ctx := context.Background()
	err := UpdateTicketInMarkdown(ctx, mdPath, "DEN-001", tracker.StatusDone)
	if err != nil {
		t.Fatalf("failed to update ticket in markdown: %v", err)
	}

	// Verify updated
	tr := tracker.NewMemoryTracker(mdPath)
	if err := tr.LoadFromMarkdown(mdPath); err != nil {
		t.Fatalf("failed to reload markdown: %v", err)
	}
	ticket, err := tr.GetTicket(ctx, "DEN-001")
	if err != nil {
		t.Fatalf("failed to get ticket: %v", err)
	}
	if ticket.Status != tracker.StatusDone {
		t.Errorf("expected StatusDone, got %s", ticket.Status)
	}

	// Test non-existent ticket update
	err = UpdateTicketInMarkdown(ctx, mdPath, "DEN-999", tracker.StatusDone)
	if err == nil {
		t.Error("expected error for non-existent ticket, got nil")
	}

	// Test non-existent file
	err = UpdateTicketInMarkdown(ctx, "nonexistent.md", "DEN-001", tracker.StatusDone)
	if err == nil {
		t.Error("expected error for non-existent file, got nil")
	}
}
