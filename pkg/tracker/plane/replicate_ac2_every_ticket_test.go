package plane

import (
	"context"
	"testing"
)

func TestAC2_EveryTicketOfTheProjectGroupedByModule(t *testing.T) {
	rules := append([]rule{
		on("GET", "/modules/m-16/module-issues/", 200, `{"results":[{"id":"iss-35"}]}`),
		on("GET", "/modules/m-17/module-issues/", 200, `{"results":[{"id":"iss-36"}]}`),
		// Another project's board is never read.
		on("GET", "/projects/proj-dcore/issues/", 200, issuesJSON(PlaneIssue{ID: "iss-dcore-1", Name: "[DCORE-001] Distributed Core Bus", SequenceID: 1, State: "st-done"})),
	}, boardRules(
		`[{"id":"proj-den","identifier":"DEN"},{"id":"proj-dcore","identifier":"DCORE"},{"id":"proj-drag","identifier":"DRAG"}]`,
		`[{"id":"st-done","name":"Done","group":"completed"},{"id":"st-prog","name":"In Progress","group":"started"},{"id":"st-verify","name":"To Verify","group":"started"},{"id":"st-todo","name":"To Do","group":"unstarted"}]`,
		`[{"id":"m-16","name":"Milestone 16: Pluggable Multi-Provider Inference Engines (FEAT-020)"},{"id":"m-17","name":"Milestone 17: Performance Testing, Inference Benchmarking & Gateway Load Harness (FEAT-021)"}]`,
		issuesJSON(
			PlaneIssue{ID: "iss-35", Name: "[DEN-035] Unified Inference Engine Gateway", SequenceID: 35, State: "st-done", Priority: "medium", DescriptionStripped: "Target Package: pkg/inference"},
			PlaneIssue{ID: "iss-36", Name: "[DEN-036] k6 Performance Testing", SequenceID: 36, State: "st-prog", Priority: "high", DescriptionStripped: "Target Package: pkg/perf"},
			PlaneIssue{ID: "iss-984", Name: "[DEN-984] Mirror Plane in DEV_STATUS", SequenceID: 984, State: "st-verify", Priority: "urgent", DescriptionStripped: "Target Package: pkg/tracker/plane"},
			// A copy of a peer's ticket on the DEN board belongs to the peer's file.
			PlaneIssue{ID: "iss-copy", Name: "[DRAG-001] Semantic Index Pipeline", SequenceID: 985, State: "st-prog"}),
	)...)
	sp := newScriptedPlane(t, rules...)

	rendered, rep, err := PullStatusFromPlane(context.Background(), planeFor(t, sp.URL, "proj-den"))
	if err != nil {
		t.Fatalf("PullStatusFromPlane failed: %v", err)
	}
	if rep.TotalTickets != 3 {
		t.Errorf("expected the 3 DEN tickets and nothing from the other projects, got %d", rep.TotalTickets)
	}
	expectIn(t, rendered,
		"DEN-035", "DEN-036", "DEN-984",
		"### Milestone 16: Pluggable Multi-Provider Inference Engines (FEAT-020)",
		"### Milestone 17: Performance Testing, Inference Benchmarking & Gateway Load Harness (FEAT-021)",
		"### General",
		"[State: Done, Priority: Medium]", "[State: In Progress, Priority: High]", "[State: To Verify, Priority: Urgent]")
	expectNotIn(t, rendered, "DCORE", "DRAG", "DGIT", "DMCP", "Other Plane Projects")
}

func TestAC3_DeterministicReplicationNoDiff(t *testing.T) {
	sp := newScriptedPlane(t, boardRules(
		`[{"id":"proj","identifier":"DEN"}]`,
		`[{"id":"st-done","name":"Done","group":"completed"},{"id":"st-prog","name":"In Progress","group":"started"},{"id":"st-todo","name":"To Do","group":"unstarted"}]`,
		`[{"id":"m-17","name":"Milestone 17"}]`,
		issuesJSON(
			PlaneIssue{ID: "iss-2", Name: "[DEN-036] Performance", SequenceID: 36, State: "st-prog", Priority: "high"},
			PlaneIssue{ID: "iss-1", Name: "[DEN-035] Unified", SequenceID: 35, State: "st-done", Priority: "medium"}),
	)...)
	client := planeAt(t, sp.URL)

	rendered1, rep1, err := PullStatusFromPlane(context.Background(), client)
	if err != nil {
		t.Fatalf("first pull failed: %v", err)
	}
	rendered2, rep2, err := PullStatusFromPlane(context.Background(), client)
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
