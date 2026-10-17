package plane

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSyncPlaneGanttAndRelations(t *testing.T) {
	mockModules := []PlaneModule{
		{ID: "mod-1", Name: "Milestone 1: Foundations", Status: "in-progress"},
		{ID: "mod-2", Name: "Milestone 2: Networking", Status: "in-progress"},
	}

	mockIssues := []PlaneIssue{
		{ID: "iss-1", Name: "[DEN-001] Setup Foundations", SequenceID: 1, Priority: "high"},
		{ID: "iss-2", Name: "[DEN-002] CLI Harness", SequenceID: 2, Priority: "medium"},
		{ID: "iss-3", Name: "[DEN-003] Multi-Mesh Abstraction", SequenceID: 3, Priority: "low"},
	}

	relationsCreated := 0
	modulesUpdated := 0
	issuesUpdated := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/modules/"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"results": mockModules})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/issues/"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"results": mockIssues})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/module-issues/"):
			if strings.Contains(r.URL.Path, "mod-1") {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"results": mockIssues[:2]})
			} else {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"results": mockIssues[2:]})
			}
		case r.Method == http.MethodPatch && strings.Contains(r.URL.Path, "/modules/"):
			modulesUpdated++
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(mockModules[0])
		case r.Method == http.MethodPatch && strings.Contains(r.URL.Path, "/issues/"):
			issuesUpdated++
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(mockIssues[0])
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/issue-relation/"):
			relationsCreated++
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode([]map[string]string{{"status": "linked"}})
		default:
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		}
	}))
	defer server.Close()

	client, err := NewClient(Config{
		BaseURL:       server.URL,
		WorkspaceSlug: "test-ws",
		ProjectID:     "test-proj",
		DirectToken:   "test-token",
	}, nil)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ctx := context.Background()
	report, err := SyncPlaneGanttAndRelations(ctx, client)
	if err != nil {
		t.Fatalf("SyncPlaneGanttAndRelations failed: %v", err)
	}

	if report.TotalModules != 2 {
		t.Errorf("expected 2 modules, got %d", report.TotalModules)
	}
	if report.TotalTickets != 3 {
		t.Errorf("expected 3 tickets, got %d", report.TotalTickets)
	}
	if modulesUpdated == 0 {
		t.Errorf("expected modules to be updated with dates")
	}
	if issuesUpdated == 0 {
		t.Errorf("expected issues to be updated with dates")
	}
	if relationsCreated == 0 {
		t.Errorf("expected relations to be created")
	}
}

func TestExtractMilestoneNumber(t *testing.T) {
	cases := []struct {
		name     string
		expected int
	}{
		{"Milestone 1: Core", 1},
		{"Milestone 39: CI/CD", 39},
		{"No milestone prefix", 999},
		{"QA-M3: Smart Commit", 3},
	}

	for _, c := range cases {
		got := extractMilestoneNumber(c.name)
		if got != c.expected {
			t.Errorf("for %q, expected %d, got %d", c.name, c.expected, got)
		}
	}
}
