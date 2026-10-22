package plane

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAC2_EveryTicketGroupedByModuleAndOtherProjects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/workspaces/ws/projects/":
			w.WriteHeader(http.StatusOK)
			projects := []PlaneProject{
				{ID: "proj-den", Name: "Dendrite", Identifier: "DEN"},
				{ID: "proj-dcore", Name: "Dendrite Core", Identifier: "DCORE"},
				{ID: "proj-drag", Name: "Dendrite RAG", Identifier: "DRAG"},
				{ID: "proj-dgit", Name: "Dendrite Git", Identifier: "DGIT"},
				{ID: "proj-dmcp", Name: "Dendrite MCP", Identifier: "DMCP"},
			}
			data, _ := json.Marshal(map[string]interface{}{"results": projects})
			_, _ = w.Write(data)

		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/projects/proj-den/states/"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"results": [
				{"id": "st-done", "name": "Done", "group": "completed"},
				{"id": "st-prog", "name": "In Progress", "group": "started"},
				{"id": "st-verify", "name": "To Verify", "group": "started"},
				{"id": "st-todo", "name": "To Do", "group": "unstarted"}
			]}`))

		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/projects/proj-den/modules/"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"results": [
				{"id": "m-16", "name": "Milestone 16: Pluggable Multi-Provider Inference Engines (FEAT-020)"},
				{"id": "m-17", "name": "Milestone 17: Performance Testing, Inference Benchmarking & Gateway Load Harness (FEAT-021)"}
			]}`))

		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/modules/m-16/module-issues/"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"results": [{"id": "iss-35"}]}`))

		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/modules/m-17/module-issues/"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"results": [{"id": "iss-36"}]}`))

		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/projects/proj-den/issues/"):
			w.WriteHeader(http.StatusOK)
			issues := []PlaneIssue{
				{
					ID:                  "iss-35",
					Name:                "[DEN-035] Unified Inference Engine Gateway",
					SequenceID:          35,
					State:               "st-done",
					Priority:            "medium",
					DescriptionStripped: "Target Package: pkg/inference",
				},
				{
					ID:                  "iss-36",
					Name:                "[DEN-036] k6 Performance Testing",
					SequenceID:          36,
					State:               "st-prog",
					Priority:            "high",
					DescriptionStripped: "Target Package: pkg/perf",
				},
				{
					ID:                  "iss-984",
					Name:                "[DEN-984] Mirror Plane in DEV_STATUS",
					SequenceID:          984,
					State:               "st-verify",
					Priority:            "urgent",
					DescriptionStripped: "Target Package: pkg/tracker/plane",
				},
			}
			data, _ := json.Marshal(map[string]interface{}{"results": issues})
			_, _ = w.Write(data)

		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/projects/proj-dcore/issues/"):
			w.WriteHeader(http.StatusOK)
			issues := []PlaneIssue{
				{
					ID:                  "iss-dcore-1",
					Name:                "[DCORE-001] Distributed Core Bus",
					SequenceID:          1,
					StateDetail:         PlaneStateDetail{Name: "Done", Group: "completed"},
					Priority:            "urgent",
					DescriptionStripped: "Target Package: pkg/core",
				},
			}
			data, _ := json.Marshal(map[string]interface{}{"results": issues})
			_, _ = w.Write(data)

		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/projects/proj-drag/issues/"):
			w.WriteHeader(http.StatusOK)
			issues := []PlaneIssue{
				{
					ID:                  "iss-drag-1",
					Name:                "[DRAG-001] Semantic Index Pipeline",
					SequenceID:          1,
					StateDetail:         PlaneStateDetail{Name: "In Progress", Group: "started"},
					Priority:            "high",
					DescriptionStripped: "Target Package: pkg/rag",
				},
			}
			data, _ := json.Marshal(map[string]interface{}{"results": issues})
			_, _ = w.Write(data)

		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/projects/proj-dgit/issues/"):
			w.WriteHeader(http.StatusOK)
			issues := []PlaneIssue{
				{
					ID:                  "iss-dgit-1",
					Name:                "[DGIT-001] Worktree Orchestrator",
					SequenceID:          1,
					StateDetail:         PlaneStateDetail{Name: "To Do", Group: "unstarted"},
					Priority:            "medium",
					DescriptionStripped: "Target Package: pkg/git",
				},
			}
			data, _ := json.Marshal(map[string]interface{}{"results": issues})
			_, _ = w.Write(data)

		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/projects/proj-dmcp/issues/"):
			w.WriteHeader(http.StatusOK)
			issues := []PlaneIssue{
				{
					ID:                  "iss-dmcp-1",
					Name:                "[DMCP-001] Native MCP Server",
					SequenceID:          1,
					StateDetail:         PlaneStateDetail{Name: "To Verify", Group: "started"},
					Priority:            "urgent",
					DescriptionStripped: "Target Package: pkg/mcpserver",
				},
			}
			data, _ := json.Marshal(map[string]interface{}{"results": issues})
			_, _ = w.Write(data)

		default:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"results": []}`))
		}
	}))
	defer server.Close()

	client, err := NewClient(Config{
		BaseURL:       server.URL,
		WorkspaceSlug: "ws",
		ProjectID:     "proj-den",
		DirectToken:   "sec-key",
	}, server.Client())
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	rendered, rep, err := PullStatusFromPlane(ctx, client)
	if err != nil {
		t.Fatalf("PullStatusFromPlane failed: %v", err)
	}

	if rep.TotalTickets != 7 {
		t.Errorf("expected 7 tickets (3 DEN + 4 other), got %d", rep.TotalTickets)
	}

	// Verify all tickets appear
	for _, expectedID := range []string{"DEN-035", "DEN-036", "DEN-984", "DCORE-001", "DRAG-001", "DGIT-001", "DMCP-001"} {
		if !strings.Contains(rendered, expectedID) {
			t.Errorf("expected rendered output to contain ticket %s", expectedID)
		}
	}

	// Verify main modules
	if !strings.Contains(rendered, "### Milestone 16: Pluggable Multi-Provider Inference Engines (FEAT-020)") {
		t.Errorf("missing Milestone 16 section")
	}
	if !strings.Contains(rendered, "### Milestone 17: Performance Testing, Inference Benchmarking & Gateway Load Harness (FEAT-021)") {
		t.Errorf("missing Milestone 17 section")
	}
	if !strings.Contains(rendered, "### General") {
		t.Errorf("missing General section for DEN-984")
	}

	// Verify state and priority formatting
	if !strings.Contains(rendered, "[State: Done, Priority: Medium]") {
		t.Errorf("missing Done/Medium state tag for DEN-035")
	}
	if !strings.Contains(rendered, "[State: In Progress, Priority: High]") {
		t.Errorf("missing In Progress/High state tag for DEN-036")
	}
	if !strings.Contains(rendered, "[State: To Verify, Priority: Urgent]") {
		t.Errorf("missing To Verify/Urgent state tag for DEN-984")
	}

	// Verify Other Plane Projects section
	if !strings.Contains(rendered, "## Other Plane Projects") {
		t.Errorf("missing Other Plane Projects section header")
	}
	if !strings.Contains(rendered, "### DCORE") {
		t.Errorf("missing DCORE subsection")
	}
	if !strings.Contains(rendered, "### DRAG") {
		t.Errorf("missing DRAG subsection")
	}
	if !strings.Contains(rendered, "### DGIT") {
		t.Errorf("missing DGIT subsection")
	}
	if !strings.Contains(rendered, "### DMCP") {
		t.Errorf("missing DMCP subsection")
	}
}

func TestAC3_DeterministicReplicationNoDiff(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/states/"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"results": [
				{"id": "st-done", "name": "Done", "group": "completed"},
				{"id": "st-prog", "name": "In Progress", "group": "started"},
				{"id": "st-todo", "name": "To Do", "group": "unstarted"}
			]}`))

		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/modules/"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"results": [{"id": "m-17", "name": "Milestone 17"}]}`))

		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/issues/"):
			w.WriteHeader(http.StatusOK)
			issues := []PlaneIssue{
				{
					ID:         "iss-2",
					Name:       "[DEN-036] Performance",
					SequenceID: 36,
					State:      "st-prog",
					Priority:   "high",
				},
				{
					ID:         "iss-1",
					Name:       "[DEN-035] Unified",
					SequenceID: 35,
					State:      "st-done",
					Priority:   "medium",
				},
			}
			data, _ := json.Marshal(map[string]interface{}{"results": issues})
			_, _ = w.Write(data)

		default:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer server.Close()

	client, err := NewClient(Config{
		BaseURL:       server.URL,
		WorkspaceSlug: "ws",
		ProjectID:     "proj",
		DirectToken:   "sec-key",
	}, server.Client())
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	rendered1, rep1, err := PullStatusFromPlane(ctx, client)
	if err != nil {
		t.Fatalf("first pull failed: %v", err)
	}

	rendered2, rep2, err := PullStatusFromPlane(ctx, client)
	if err != nil {
		t.Fatalf("second pull failed: %v", err)
	}

	if rendered1 != rendered2 {
		t.Errorf("non-deterministic markdown rendering detected between runs")
	}
	if rep1.TotalTickets != rep2.TotalTickets {
		t.Errorf("report mismatch between runs")
	}
}
