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
