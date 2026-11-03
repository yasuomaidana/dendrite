package plane

import (
	"encoding/json"
	"testing"

	"Dendrite/pkg/testkit"
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

// expectIn and expectNotIn fail for every text md lacks or holds.
var (
	expectIn    = testkit.Contains
	expectNotIn = testkit.NotContains
)

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
