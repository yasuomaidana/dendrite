package plane

import (
	"encoding/json"
	"strings"
	"testing"
)

// issuesJSON is the body Plane answers an issue listing with.
func issuesJSON(issues ...PlaneIssue) string {
	data, _ := json.Marshal(map[string]any{"results": issues})
	return string(data)
}

// boardRules answers a replicate from one board. The project list rule is last because its path is
// a prefix of every other path; module issue lists are empty unless a test adds its own rules first.
func boardRules(projects, states, modules, issues string) []rule {
	return []rule{
		on("GET", "/module-issues/", 200, `{"results":[]}`),
		on("GET", "/states/", 200, states),
		on("GET", "/modules/", 200, modules),
		on("GET", "/issues/", 200, issues),
		on("GET", "/projects/", 200, projects),
	}
}

// expectIn fails for every text that md does not contain.
func expectIn(t *testing.T, md string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in:\n%s", want, md)
		}
	}
}

// expectNotIn fails for every text that md contains.
func expectNotIn(t *testing.T, md string, unwanted ...string) {
	t.Helper()
	for _, bad := range unwanted {
		if strings.Contains(md, bad) {
			t.Errorf("unexpected %q in:\n%s", bad, md)
		}
	}
}

// counts is the per-state summary of a report: total, backlog, to do, in progress, to verify, done.
func counts(r *StatusReplicationReport) [6]int {
	return [6]int{r.TotalTickets, r.BacklogTickets, r.TodoTickets, r.InProgressTickets, r.ToVerifyTickets, r.DoneTickets}
}

// planeFor is a client of the given project on a scripted Plane.
func planeFor(t *testing.T, baseURL, projectID string) *Client {
	t.Helper()
	c, err := NewClient(Config{BaseURL: baseURL, WorkspaceSlug: "ws", ProjectID: projectID, DirectToken: "token"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return c
}
