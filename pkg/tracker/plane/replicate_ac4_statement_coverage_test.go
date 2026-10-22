package plane

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"Dendrite/pkg/tracker"
)

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

func TestPullStatusFromPlane_RelocatedCopiesUseSubProjectIDsAndStates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/workspaces/ws/projects/"):
			_, _ = w.Write([]byte(`{"results": [
				{"id": "proj-den", "identifier": "DEN"},
				{"id": "proj-dcore", "identifier": "DCORE"}
			]}`))
		case strings.Contains(r.URL.Path, "/projects/proj-den/states/"):
			_, _ = w.Write([]byte(`{"results": [{"id": "den-todo", "name": "To Do", "group": "unstarted"}]}`))
		case strings.Contains(r.URL.Path, "/projects/proj-dcore/states/"):
			_, _ = w.Write([]byte(`{"results": [{"id": "dcore-verify", "name": "To Verify", "group": "started"}]}`))
		case strings.Contains(r.URL.Path, "/projects/proj-den/issues/"):
			_, _ = w.Write([]byte(`{"results": [
				{"id": "d1", "name": "[DEND-302] JetBrains Plugin", "sequence_id": 110, "state": "den-todo"},
				{"id": "d2", "name": "[DEN-942] Truncation splicer (other languages after DEN-972)", "sequence_id": 174, "state": "den-todo"},
				{"id": "d3", "name": "[DEN-701-COV-1] Coverage Gap: pkg/vcs/guard.go", "sequence_id": 124, "state": "den-todo"}
			]}`))
		case strings.Contains(r.URL.Path, "/projects/proj-dcore/issues/"):
			_, _ = w.Write([]byte(`{"results": [
				{"id": "c6", "name": "[DEN-018] AST Context Compaction", "sequence_id": 6, "state": "dcore-verify"}
			]}`))
		default:
			_, _ = w.Write([]byte(`{"results": []}`))
		}
	}))
	defer server.Close()

	client, err := NewClient(Config{BaseURL: server.URL, WorkspaceSlug: "ws", ProjectID: "proj-den", DirectToken: "k"}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	rendered, rep, err := PullStatusFromPlane(context.Background(), client)
	if err != nil {
		t.Fatalf("PullStatusFromPlane failed: %v", err)
	}

	if !strings.Contains(rendered, "**DCORE-6**: [DEN-018] AST Context Compaction [State: To Verify") {
		t.Errorf("relocated copy not listed under its DCORE key with the DCORE state:\n%s", rendered)
	}
	if strings.Contains(rendered, "### DEND") {
		t.Errorf("DEND-* tickets living in DEN must not get their own project section:\n%s", rendered)
	}
	if !strings.Contains(rendered, "**DEN-942**: Truncation splicer (other languages after DEN-972)") {
		t.Errorf("IDs referenced later in a title must be kept:\n%s", rendered)
	}
	if !strings.Contains(rendered, "**DEN-701-COV-1**: Coverage Gap") {
		t.Errorf("suffixed follow-up IDs must be kept whole:\n%s", rendered)
	}
	if rep.ToVerifyTickets != 1 || rep.TodoTickets != 3 {
		t.Errorf("state counts = verify %d, todo %d; want 1, 3", rep.ToVerifyTickets, rep.TodoTickets)
	}
}
