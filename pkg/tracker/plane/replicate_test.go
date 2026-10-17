package plane

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"Dendrite/pkg/tracker"
)

const sampleDevStatusMD = `# Dendrite Project Development Status & Roadmap

## Current Status Overview

- **Active Milestone**: Milestone 17 — Performance Testing, Inference Benchmarking & Gateway Load Harness (FEAT-021)
- **Current Focus**: ` + "`DEN-036: k6 Performance Testing, TTFT/TPS Benchmarking & Cascade Failover Load Harness`" + `
- **Last Updated**: 2026-10-14
- **Overall Progress**: 36 / 40 Tickets Completed (90.0%)

---

## Milestone Execution Pipeline

### Milestone 17: Performance Testing, Inference Benchmarking & Gateway Load Harness (FEAT-021)

- [ ] **DEN-036**: k6 Performance Testing, TTFT/TPS Benchmarking & Cascade Failover Load Harness (pkg/perf, cmd/dendrite, tests/perf) *(IN PROGRESS)*

### Milestone 16: Pluggable Multi-Provider Inference Engines (FEAT-020)

- [x] **DEN-035**: Unified Inference Engine Gateway: OxiLLaMa, Hugging Face TGI/TEI, vLLM, llama.cpp & Ollama Metal (pkg/inference, pkg/gateway)

### Milestone 1: Core Foundation, Developer Tools & Ollama Engine

- [x] **DEN-001**: Initialize Development Status Ledger & Execution Roadmap (pkg/devtool)

---

## Detailed Specifications for Active & Queued Tickets

### Ticket ` + "`DEN-036`" + `: Performance Testing, Inference Benchmarking & Gateway Load Harness (k6)

- **Target Package**: ` + "pkg/perf, cmd/dendrite, tests/perf" + `
- **Governing Specs**: ` + "docs/features/FEAT-021.md" + `
- **Goal**: Implement k6 load harness and TTFT benchmarks.
- **Acceptance Criteria**:
    - [ ] Implement k6 runner with TTFT parsing.
    - [ ] Statement coverage >= 85%.
`

func TestParseDevStatus(t *testing.T) {
	data, err := ParseDevStatus(sampleDevStatusMD)
	if err != nil {
		t.Fatalf("unexpected error parsing dev status: %v", err)
	}

	if !strings.Contains(data.ActiveMilestone, "Milestone 17") {
		t.Errorf("unexpected active milestone: %s", data.ActiveMilestone)
	}
	if !strings.Contains(data.CurrentFocus, "DEN-036") {
		t.Errorf("unexpected current focus: %s", data.CurrentFocus)
	}
	if len(data.Tickets) != 3 {
		t.Fatalf("expected 3 tickets, got: %d", len(data.Tickets))
	}

	t0 := data.Tickets[0]
	if t0.ID != "DEN-036" || t0.Status != tracker.StatusInProgress {
		t.Errorf("ticket 0 mismatch: %+v", t0)
	}
	if len(t0.AcceptanceCriteria) != 2 {
		t.Errorf("expected 2 acceptance criteria, got: %v", t0.AcceptanceCriteria)
	}
	if t0.Goal == "" {
		t.Errorf("expected goal to be parsed for DEN-036")
	}

	t1 := data.Tickets[1]
	if t1.ID != "DEN-035" || t1.Status != tracker.StatusDone {
		t.Errorf("ticket 1 mismatch: %+v", t1)
	}

	t2 := data.Tickets[2]
	if t2.ID != "DEN-001" || t2.Status != tracker.StatusDone {
		t.Errorf("ticket 2 mismatch: %+v", t2)
	}
}

func TestPushStatusToPlane(t *testing.T) {
	patchCalls := 0
	postCalls := 0
	moduleIssueCalls := 0

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
			_, _ = w.Write([]byte(`{"results": []}`))

		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/modules/"):
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id": "mod-1", "name": "M17"}`))

		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/issues/"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"results": [
				{"id": "iss-1", "name": "[DEN-035] Unified Inference Engine", "sequence_id": 35, "state": "st-prog"}
			]}`))

		case r.Method == http.MethodPatch && strings.Contains(r.URL.Path, "/issues/iss-1/"):
			patchCalls++
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id": "iss-1", "name": "[DEN-035] Unified Inference Engine", "state": "st-done"}`))

		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/modules/") && strings.HasSuffix(r.URL.Path, "/issues/"):
			moduleIssueCalls++
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status": "ok"}`))

		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/issues/"):
			postCalls++
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id": "iss-new", "name": "[DEN-036] New", "sequence_id": 36}`))

		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/pages/"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))

		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/pages/"):
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id": "page-1", "name": "Dendrite Status"}`))

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
	rep, err := PushStatusToPlane(ctx, client, sampleDevStatusMD)
	if err != nil {
		t.Fatalf("PushStatusToPlane failed: %v", err)
	}

	if rep.TotalTickets != 3 {
		t.Errorf("expected 3 total tickets, got %d", rep.TotalTickets)
	}
	if rep.CompletedTickets != 2 {
		t.Errorf("expected 2 completed tickets, got %d", rep.CompletedTickets)
	}
	if rep.InProgressTickets != 1 {
		t.Errorf("expected 1 in-progress ticket, got %d", rep.InProgressTickets)
	}
	if patchCalls != 1 {
		t.Errorf("expected 1 patch call for existing DEN-035, got %d", patchCalls)
	}
	if postCalls != 2 {
		t.Errorf("expected 2 post calls for DEN-036 and DEN-001, got %d", postCalls)
	}
	if moduleIssueCalls == 0 {
		t.Errorf("expected at least 1 module issue assignment call")
	}
}

func TestReplicateStatusFromPlane(t *testing.T) {
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
					ID:                  "iss-35",
					Name:                "[DEN-035] Unified Inference Engine Gateway",
					SequenceID:          35,
					State:               "st-done",
					DescriptionStripped: "Target Package: pkg/inference",
				},
				{
					ID:                  "iss-36",
					Name:                "[DEN-036] k6 Performance Testing",
					SequenceID:          36,
					State:               "st-prog",
					DescriptionStripped: "Target Package: pkg/perf",
				},
				{
					ID:                  "iss-39",
					Name:                "[DEN-039] Reverse Engineering Showcases",
					SequenceID:          39,
					State:               "st-todo",
					DescriptionStripped: "Target Package: pkg/rea",
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
	rendered, rep, err := ReplicateStatusFromPlane(ctx, client)
	if err != nil {
		t.Fatalf("ReplicateStatusFromPlane failed: %v", err)
	}

	if rep.TotalTickets != 3 {
		t.Errorf("expected 3 tickets, got %d", rep.TotalTickets)
	}
	if rep.CompletedTickets != 1 {
		t.Errorf("expected 1 completed ticket, got %d", rep.CompletedTickets)
	}
	if rep.InProgressTickets != 1 {
		t.Errorf("expected 1 in-progress ticket, got %d", rep.InProgressTickets)
	}
	if !strings.Contains(rep.CurrentFocus, "DEN-036") {
		t.Errorf("expected DEN-036 in current focus, got %s", rep.CurrentFocus)
	}
	if !strings.Contains(rendered, "# Dendrite Project Development Status & Roadmap") {
		t.Errorf("expected header in rendered markdown")
	}
	if !strings.Contains(rendered, "- [x] **DEN-035**") {
		t.Errorf("expected completed DEN-035 in rendered markdown")
	}
	if !strings.Contains(rendered, "- [ ] **DEN-036**: k6 Performance Testing") {
		t.Errorf("expected in-progress DEN-036 in rendered markdown")
	}
}

func TestReplicate_ErrorHandling(t *testing.T) {
	errServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer errServer.Close()

	client, _ := NewClient(Config{
		BaseURL:       errServer.URL,
		WorkspaceSlug: "ws",
		ProjectID:     "p",
		DirectToken:   "tok",
	}, errServer.Client())

	ctx := context.Background()
	_, err := PushStatusToPlane(ctx, client, sampleDevStatusMD)
	if err == nil {
		t.Errorf("expected error from PushStatusToPlane on server failure")
	}

	_, _, err = ReplicateStatusFromPlane(ctx, client)
	if err == nil {
		t.Errorf("expected error from ReplicateStatusFromPlane on server failure")
	}

	_, _, err = PullStatusFromPlane(ctx, client)
	if err == nil {
		t.Errorf("expected error from PullStatusFromPlane on server failure")
	}
}

func TestAC1_KnownStatesAndSummaryCounts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/states/"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"results": [
				{"id": "st-done", "name": "Done", "group": "completed"},
				{"id": "st-verify", "name": "To Verify", "group": "started"},
				{"id": "st-prog", "name": "In Progress", "group": "started"},
				{"id": "st-todo", "name": "To Do", "group": "unstarted"},
				{"id": "st-backlog", "name": "Backlog", "group": "backlog"}
			]}`))

		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/modules/"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"results": [{"id": "m-17", "name": "Milestone 17"}]}`))

		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/issues/"):
			w.WriteHeader(http.StatusOK)
			issues := []PlaneIssue{
				{
					ID:         "iss-1",
					Name:       "[DEN-101] Backlog item",
					SequenceID: 101,
					State:      "st-backlog",
					Priority:   "low",
				},
				{
					ID:         "iss-2",
					Name:       "[DEN-102] To Do item",
					SequenceID: 102,
					State:      "st-todo",
					Priority:   "medium",
				},
				{
					ID:         "iss-3",
					Name:       "[DEN-103] In Progress item",
					SequenceID: 103,
					State:      "st-prog",
					Priority:   "high",
				},
				{
					ID:         "iss-4",
					Name:       "[DEN-104] To Verify item",
					SequenceID: 104,
					State:      "st-verify",
					Priority:   "urgent",
				},
				{
					ID:         "iss-5",
					Name:       "[DEN-105] Done item",
					SequenceID: 105,
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
	rendered, rep, err := PullStatusFromPlane(ctx, client)
	if err != nil {
		t.Fatalf("PullStatusFromPlane failed: %v", err)
	}

	if rep.TotalTickets != 5 {
		t.Errorf("expected 5 total tickets, got %d", rep.TotalTickets)
	}
	if rep.BacklogTickets != 1 {
		t.Errorf("expected 1 backlog ticket, got %d", rep.BacklogTickets)
	}
	if rep.TodoTickets != 1 {
		t.Errorf("expected 1 todo ticket, got %d", rep.TodoTickets)
	}
	if rep.InProgressTickets != 1 {
		t.Errorf("expected 1 in-progress ticket, got %d", rep.InProgressTickets)
	}
	if rep.ToVerifyTickets != 1 {
		t.Errorf("expected 1 to-verify ticket, got %d", rep.ToVerifyTickets)
	}
	if rep.DoneTickets != 1 {
		t.Errorf("expected 1 done ticket, got %d", rep.DoneTickets)
	}

	sum := rep.BacklogTickets + rep.TodoTickets + rep.InProgressTickets + rep.ToVerifyTickets + rep.DoneTickets
	if sum != rep.TotalTickets {
		t.Errorf("sum of states (%d) does not match total tickets (%d)", sum, rep.TotalTickets)
	}

	expectedSummary := "- **Summary**: Backlog: 1 | To Do: 1 | In Progress: 1 | To Verify: 1 | Done: 1 (Total: 5)"
	if !strings.Contains(rendered, expectedSummary) {
		t.Errorf("rendered markdown missing expected summary line. Got:\n%s", rendered)
	}
}

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

func TestAC4_StatementCoverageEdgeCases(t *testing.T) {
	// 1. resolvePlaneState coverage
	if st := resolvePlaneState("completed", "custom"); st != tracker.StatusDone {
		t.Errorf("expected StatusDone, got %s", st)
	}
	if st := resolvePlaneState("started", "ready for review"); st != tracker.StatusInReview {
		t.Errorf("expected StatusInReview, got %s", st)
	}
	if st := resolvePlaneState("started", "in verification"); st != tracker.StatusInReview {
		t.Errorf("expected StatusInReview, got %s", st)
	}
	if st := resolvePlaneState("started", "in active development"); st != tracker.StatusInProgress {
		t.Errorf("expected StatusInProgress, got %s", st)
	}
	if st := resolvePlaneState("unstarted", "backlog task"); st != tracker.StatusBacklog {
		t.Errorf("expected StatusBacklog, got %s", st)
	}
	if st := resolvePlaneState("backlog", "future item"); st != tracker.StatusBacklog {
		t.Errorf("expected StatusBacklog, got %s", st)
	}
	if st := resolvePlaneState("unstarted", "todo"); st != tracker.StatusTodo {
		t.Errorf("expected StatusTodo, got %s", st)
	}
	if st := resolvePlaneState("unknown_group", "unknown_name"); st != tracker.StatusBacklog {
		t.Errorf("expected StatusBacklog for unknown state, got %s", st)
	}

	// 2. formatStatusName coverage
	if s := formatStatusName(tracker.StatusDone); s != "Done" {
		t.Errorf("expected Done, got %s", s)
	}
	if s := formatStatusName(tracker.StatusInReview); s != "To Verify" {
		t.Errorf("expected To Verify, got %s", s)
	}
	if s := formatStatusName(tracker.StatusInProgress); s != "In Progress" {
		t.Errorf("expected In Progress, got %s", s)
	}
	if s := formatStatusName(tracker.StatusTodo); s != "To Do" {
		t.Errorf("expected To Do, got %s", s)
	}
	if s := formatStatusName(tracker.StatusBacklog); s != "Backlog" {
		t.Errorf("expected Backlog, got %s", s)
	}
	if s := formatStatusName(tracker.Status("OTHER")); s != "Backlog" {
		t.Errorf("expected Backlog, got %s", s)
	}

	// 3. normalizePriority coverage
	if p := normalizePriority("URGENT"); p != "Urgent" {
		t.Errorf("expected Urgent, got %s", p)
	}
	if p := normalizePriority("high"); p != "High" {
		t.Errorf("expected High, got %s", p)
	}
	if p := normalizePriority("medium"); p != "Medium" {
		t.Errorf("expected Medium, got %s", p)
	}
	if p := normalizePriority("low"); p != "Low" {
		t.Errorf("expected Low, got %s", p)
	}
	if p := normalizePriority("none"); p != "Medium" {
		t.Errorf("expected Medium, got %s", p)
	}
	if p := normalizePriority(""); p != "Medium" {
		t.Errorf("expected Medium, got %s", p)
	}

	// 4. extractMilestoneNumber coverage
	if n := extractMilestoneNumber("Milestone 18: Agent Search"); n != 18 {
		t.Errorf("expected 18, got %d", n)
	}
	if n := extractMilestoneNumber("M19"); n != 19 {
		t.Errorf("expected 19, got %d", n)
	}
	if n := extractMilestoneNumber("General"); n != 999 {
		t.Errorf("expected 999, got %d", n)
	}

	// 5. normalizeMilestoneName coverage
	if m := normalizeMilestoneName(""); m != "General" {
		t.Errorf("expected General, got %s", m)
	}
	if m := normalizeMilestoneName("Milestone 16"); !strings.Contains(m, "Pluggable Multi-Provider") {
		t.Errorf("expected expanded name, got %s", m)
	}
	if m := normalizeMilestoneName("Milestone 16: Custom"); m != "Milestone 16: Custom" {
		t.Errorf("expected unchanged, got %s", m)
	}
	if m := normalizeMilestoneName("Milestone 99: Unknown"); m != "Milestone 99: Unknown" {
		t.Errorf("expected unchanged, got %s", m)
	}

	// 6. extractMilestone coverage
	if em := extractMilestone("<p><strong>Milestone:</strong> Milestone 14</p>"); em != "Milestone 14" {
		t.Errorf("expected Milestone 14, got %s", em)
	}
	if em := extractMilestone("No milestone here"); em != "" {
		t.Errorf("expected empty, got %s", em)
	}

	// 7. extractProjectIdentifier coverage
	if proj := extractProjectIdentifier("DCORE-123"); proj != "DCORE" {
		t.Errorf("expected DCORE, got %s", proj)
	}
	if proj := extractProjectIdentifier("DEN"); proj != "DEN" {
		t.Errorf("expected DEN, got %s", proj)
	}

	// 8. ListPlaneIssuesForProject & ListStatesForProject coverage
	cursorCount := 0
	paginatedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/issues/"):
			if r.URL.Query().Get("cursor") == "" {
				cursorCount++
				_, _ = w.Write([]byte(`{
					"next_page_results": true,
					"next_cursor": "cur-1",
					"results": [{"id": "iss-p1", "name": "[P1] Issue 1", "sequence_id": 1}]
				}`))
			} else {
				_, _ = w.Write([]byte(`{
					"next_page_results": false,
					"next_cursor": "",
					"results": [{"id": "iss-p2", "name": "[P1] Issue 2", "sequence_id": 2}]
				}`))
			}
		case strings.Contains(r.URL.Path, "/states/"):
			_, _ = w.Write([]byte(`{
				"results": [{"id": "st-1", "name": "Active", "group": "started"}]
			}`))
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer paginatedServer.Close()

	client, _ := NewClient(Config{
		BaseURL:       paginatedServer.URL,
		WorkspaceSlug: "ws",
		ProjectID:     "proj-main",
		DirectToken:   "tok",
	}, paginatedServer.Client())

	ctx := context.Background()
	pIssues, err := client.ListPlaneIssuesForProject(ctx, "")
	if err != nil || len(pIssues) != 2 {
		t.Fatalf("expected 2 paginated issues, got %d (err: %v)", len(pIssues), err)
	}

	pStates, err := client.ListStatesForProject(ctx, "")
	if err != nil || len(pStates) != 1 {
		t.Fatalf("expected 1 state, got %d (err: %v)", len(pStates), err)
	}

	// Direct list test for ListPlaneIssuesForProject and ListStatesForProject
	directServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/issues/") {
			_, _ = w.Write([]byte(`[{"id": "iss-d1", "name": "[D1] Direct", "sequence_id": 1}]`))
		} else if strings.Contains(r.URL.Path, "/states/") {
			_, _ = w.Write([]byte(`[{"id": "st-d1", "name": "Direct State", "group": "started"}]`))
		}
	}))
	defer directServer.Close()

	clientDirect, _ := NewClient(Config{
		BaseURL:       directServer.URL,
		WorkspaceSlug: "ws",
		ProjectID:     "p",
		DirectToken:   "tok",
	}, directServer.Client())

	if issues, err := clientDirect.ListPlaneIssuesForProject(ctx, "p"); err != nil || len(issues) != 1 {
		t.Errorf("expected 1 direct issue, got %d (err: %v)", len(issues), err)
	}
	if states, err := clientDirect.ListStatesForProject(ctx, "p"); err != nil || len(states) != 1 {
		t.Errorf("expected 1 direct state, got %d (err: %v)", len(states), err)
	}

	// Error & unauth tests for ListPlaneIssuesForProject and ListStatesForProject
	errServ := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/unauth/") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer errServ.Close()

	errClient, _ := NewClient(Config{
		BaseURL:       errServ.URL,
		WorkspaceSlug: "ws",
		ProjectID:     "p",
		DirectToken:   "tok",
	}, errServ.Client())

	unauthClient, _ := NewClient(Config{
		BaseURL:       errServ.URL,
		WorkspaceSlug: "unauth",
		ProjectID:     "p",
		DirectToken:   "tok",
	}, errServ.Client())

	if _, err := unauthClient.ListPlaneIssuesForProject(ctx, "p"); err != ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized on ListPlaneIssuesForProject, got %v", err)
	}
	if _, err := errClient.ListPlaneIssuesForProject(ctx, "p"); err == nil {
		t.Errorf("expected error on 500 for ListPlaneIssuesForProject")
	}

	if _, err := unauthClient.ListStatesForProject(ctx, "p"); err != ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized on ListStatesForProject, got %v", err)
	}
	if _, err := errClient.ListStatesForProject(ctx, "p"); err == nil {
		t.Errorf("expected error on 500 for ListStatesForProject")
	}

	// 9. ParseDevStatus with Summary and various status tags
	testMD := `# Dendrite
## Current Status Overview
- **Active Milestone**: M1
- **Current Focus**: DEN-001
- **Last Updated**: 2026-10-17
- **Summary**: Backlog: 1 | To Do: 1 | In Progress: 1 | To Verify: 1 | Done: 1 (Total: 5)
## Milestone Execution Pipeline
### M1
- [ ] **DEN-001**: Task 1 (` + "`pkg/a`" + `) [State: To Verify, Priority: High]
- [ ] **DEN-002**: Task 2 (` + "`pkg/b`" + `) *(BACKLOG)*
- [ ] **DEN-003**: Task 3 *(TODO)*
- [x] **DEN-004**: Task 4
`
	parsed, err := ParseDevStatus(testMD)
	if err != nil {
		t.Fatalf("ParseDevStatus failed: %v", err)
	}
	if parsed.Summary != "Backlog: 1 | To Do: 1 | In Progress: 1 | To Verify: 1 | Done: 1 (Total: 5)" {
		t.Errorf("unexpected summary: %s", parsed.Summary)
	}
	if len(parsed.Tickets) != 4 {
		t.Fatalf("expected 4 tickets, got %d", len(parsed.Tickets))
	}
	if parsed.Tickets[0].Status != tracker.StatusInReview {
		t.Errorf("expected StatusInReview for DEN-001, got %s", parsed.Tickets[0].Status)
	}
	if parsed.Tickets[1].Status != tracker.StatusBacklog {
		t.Errorf("expected StatusBacklog for DEN-002, got %s", parsed.Tickets[1].Status)
	}
	if parsed.Tickets[2].Status != tracker.StatusTodo {
		t.Errorf("expected StatusTodo for DEN-003, got %s", parsed.Tickets[2].Status)
	}
	if parsed.Tickets[3].Status != tracker.StatusDone {
		t.Errorf("expected StatusDone for DEN-004, got %s", parsed.Tickets[3].Status)
	}

	// 10. buildDevStatusPageHTML with various statuses
	data := &DevStatusData{
		ActiveMilestone: "M1",
		CurrentFocus:    "DEN-001",
		LastUpdated:     "2026-10-17",
		Milestones:      []string{"M1"},
		Tickets: []ParsedDevTicket{
			{ID: "DEN-001", Title: "T1", Milestone: "M1", Status: tracker.StatusDone},
			{ID: "DEN-002", Title: "T2", Milestone: "M1", Status: tracker.StatusInProgress},
			{ID: "DEN-003", Title: "T3", Milestone: "M1", Status: tracker.StatusInReview},
			{ID: "DEN-004", Title: "T4", Milestone: "M1", Status: tracker.StatusBacklog},
			{ID: "DEN-005", Title: "T5", Milestone: "M1", Status: tracker.StatusTodo},
		},
	}
	htmlOut := buildDevStatusPageHTML(data)
	if !strings.Contains(htmlOut, "[DONE]") || !strings.Contains(htmlOut, "[IN PROGRESS]") ||
		!strings.Contains(htmlOut, "[TO VERIFY]") || !strings.Contains(htmlOut, "[BACKLOG]") ||
		!strings.Contains(htmlOut, "[TODO]") {
		t.Errorf("buildDevStatusPageHTML missing expected status badges: %s", htmlOut)
	}
}
