// Package hitltest provides offline fixtures for the verification sync: a real
// HTTP server speaking the Plane API subset the sync uses, and a scripted gh
// executor that records every command instead of reaching GitHub.
package hitltest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"

	"Dendrite/pkg/tracker/plane"
)

const (
	// Workspace and Project are the identifiers the Plane server answers for.
	Workspace = "ws"
	Project   = "proj"
)

// Standard Plane states served by NewPlaneServer.
var States = []plane.PlaneState{
	{ID: "st-progress", Name: "In Progress", Group: "started"},
	{ID: "st-verify", Name: "To Verify", Group: "started"},
	{ID: "st-done", Name: "Done", Group: "completed"},
}

// PlaneServer is an in-memory Plane project served over HTTP.
type PlaneServer struct {
	*httptest.Server
	mu      sync.Mutex
	issues  []plane.PlaneIssue
	Patches map[string]string // issue UUID -> new state ID
	// FailIssues and FailPatch make the issue list and state updates answer 500.
	FailIssues, FailPatch bool
}

// NewPlaneServer serves issues and the standard states until Close.
func NewPlaneServer(issues ...plane.PlaneIssue) *PlaneServer {
	ps := &PlaneServer{issues: issues, Patches: make(map[string]string)}
	ps.Server = httptest.NewServer(http.HandlerFunc(ps.handle))
	return ps
}

// Client returns a Plane client bound to the server.
func (ps *PlaneServer) Client() *plane.Client {
	c, _ := plane.NewClient(plane.Config{
		BaseURL: ps.URL, WorkspaceSlug: Workspace, ProjectID: Project, DirectToken: "test-token",
	}, ps.Server.Client())
	return c
}

func (ps *PlaneServer) handle(w http.ResponseWriter, r *http.Request) {
	base := fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/", Workspace, Project)
	rest := strings.TrimPrefix(r.URL.Path, base)
	ps.mu.Lock()
	defer ps.mu.Unlock()
	switch {
	case r.Method == http.MethodGet && rest == "states/":
		_ = json.NewEncoder(w).Encode(States)
	case r.Method == http.MethodGet && rest == "issues/":
		if ps.FailIssues {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(ps.issues)
	case r.Method == http.MethodPatch && ps.FailPatch:
		w.WriteHeader(http.StatusInternalServerError)
	case r.Method == http.MethodPatch && strings.HasPrefix(rest, "issues/"):
		id := strings.Trim(strings.TrimPrefix(rest, "issues/"), "/")
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		ps.Patches[id] = body["state"]
		_, _ = w.Write([]byte(`{"id":"` + id + `"}`))
	default:
		http.NotFound(w, r)
	}
}

// GHScript answers gh (and git) commands from canned outputs keyed by command
// prefix, recording every call. Unmatched commands succeed with empty output.
type GHScript struct {
	mu        sync.Mutex
	responses []response
	Calls     []string
}

type response struct {
	prefix string
	out    string
	err    error
}

// On makes commands starting with prefix (e.g. "gh issue list") return out, err.
// Later registrations win over earlier ones.
func (g *GHScript) On(prefix, out string, err error) *GHScript {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.responses = append([]response{{prefix: prefix, out: out, err: err}}, g.responses...)
	return g
}

// Execute implements github.CommandExecutor.
func (g *GHScript) Execute(_ context.Context, name string, args ...string) (string, error) {
	cmd := strings.Join(append([]string{name}, args...), " ")
	g.mu.Lock()
	defer g.mu.Unlock()
	g.Calls = append(g.Calls, cmd)
	for _, r := range g.responses {
		if strings.HasPrefix(cmd, r.prefix) {
			return r.out, r.err
		}
	}
	return "", nil
}

// Called reports how many recorded commands start with prefix.
func (g *GHScript) Called(prefix string) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	n := 0
	for _, c := range g.Calls {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

// IssueListJSON renders `gh issue list --json number,title,state,stateReason,labels` output.
func IssueListJSON(issues ...IssueRow) string {
	type label struct {
		Name string `json:"name"`
	}
	type row struct {
		Number      int     `json:"number"`
		Title       string  `json:"title"`
		State       string  `json:"state"`
		StateReason string  `json:"stateReason"`
		Labels      []label `json:"labels"`
	}
	rows := make([]row, 0, len(issues))
	for _, i := range issues {
		r := row{Number: i.Number, Title: i.Title, State: i.State, StateReason: i.StateReason, Labels: []label{}}
		for _, l := range i.Labels {
			r.Labels = append(r.Labels, label{Name: l})
		}
		rows = append(rows, r)
	}
	data, _ := json.Marshal(rows)
	return string(data)
}

// IssueRow describes one issue for IssueListJSON.
type IssueRow struct {
	Number      int
	Title       string
	State       string
	StateReason string
	Labels      []string
}
