package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"Dendrite/pkg/tracker/github"
	"Dendrite/pkg/tracker/hitl"
	"Dendrite/pkg/tracker/hitl/hitltest"
	"Dendrite/pkg/tracker/plane"
)

// offlineHITL points the GitHub/HITL tools at a local Plane server and a scripted gh,
// so tests never reach the real Plane instance or GitHub.
func offlineHITL(t *testing.T) (*hitltest.PlaneServer, *hitltest.GHScript) {
	t.Helper()
	ps := hitltest.NewPlaneServer(
		plane.PlaneIssue{ID: "u-1", Name: "[DEN-1] feat(cli): new", State: "st-verify"},
		plane.PlaneIssue{ID: "u-2", Name: "[DEN-2] approved", State: "st-verify"},
	)
	t.Cleanup(ps.Close)
	gh := (&hitltest.GHScript{}).
		On("gh issue list", hitltest.IssueListJSON(hitltest.IssueRow{Number: 5, Title: "[DEN-2] approved", State: "CLOSED", StateReason: "COMPLETED", Labels: []string{github.LabelToVerify, github.LabelHITL}}), nil).
		On("gh issue create", "https://github.com/o/r/issues/6", nil)
	prev := newHITLSyncer
	newHITLSyncer = func(_ context.Context, _ Dependencies, repo string) (*hitl.Syncer, error) {
		if repo == "fail/me" {
			return nil, errors.New("no plane")
		}
		s := hitl.New(ps.Client(), github.NewGitHubClient("o/r").WithExecutor(gh), ".")
		s.Git = gh
		return s, nil
	}
	t.Cleanup(func() { newHITLSyncer = prev })
	return ps, gh
}

func callTool(t *testing.T, srv *Server, name, args string) (string, bool) {
	t.Helper()
	req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"` + name + `","arguments":` + args + `}}`
	resp := srv.HandleRequest(context.Background(), []byte(req))
	if resp.Error != nil {
		return resp.Error.Message, true
	}
	res, ok := resp.Result.(*CallToolResult)
	if !ok {
		v := resp.Result.(CallToolResult)
		res = &v
	}
	return res.Content[0].Text, res.IsError
}

func TestGitHubSyncAndHITLSyncTools(t *testing.T) {
	ps, gh := offlineHITL(t)
	srv := NewServer(ServerConfig{})
	srv.RegisterDefaultTools(Dependencies{})

	text, isErr := callTool(t, srv, "dendrite_github_sync", `{"dry_run":true}`)
	if isErr || !strings.Contains(text, `"kind": "create"`) || !strings.Contains(text, `"dry_run": true`) {
		t.Fatalf("dry run: %s", text)
	}
	if gh.Called("gh issue create") != 0 {
		t.Fatal("dry run created an issue")
	}
	if text, _ = callTool(t, srv, "dendrite_github_sync", `{"dry_run":true,"tickets":["DEN-2",7]}`); strings.Contains(text, `"ticket_id": "DEN-1"`) {
		t.Fatalf("tickets filter ignored: %s", text)
	}
	if text, isErr = callTool(t, srv, "dendrite_github_sync", `{}`); isErr || gh.Called("gh issue create") != 1 {
		t.Fatalf("apply: %s", text)
	}

	text, isErr = callTool(t, srv, "dendrite_hitl_sync", `{"dry_run":true}`)
	var results []hitl.HITLResult
	if isErr || json.Unmarshal([]byte(text), &results) != nil || len(results) != 2 || len(ps.Patches) != 0 {
		t.Fatalf("hitl dry run: %s", text)
	}
	if _, isErr = callTool(t, srv, "dendrite_hitl_sync", `{}`); isErr || ps.Patches["u-2"] != "st-done" {
		t.Fatalf("hitl apply: patches %v", ps.Patches)
	}

	for _, tool := range []string{"dendrite_github_sync", "dendrite_hitl_sync"} {
		if text, isErr := callTool(t, srv, tool, `{"repo":"fail/me"}`); !isErr || !strings.Contains(text, "no plane") {
			t.Errorf("%s setup failure: %s", tool, text)
		}
	}
	gh.On("gh issue list", "", errors.New("offline"))
	for _, tool := range []string{"dendrite_github_sync", "dendrite_hitl_sync"} {
		if text, isErr := callTool(t, srv, tool, `{}`); !isErr || !strings.Contains(text, "offline") {
			t.Errorf("%s sync failure: %s", tool, text)
		}
	}
}

func TestHITLSyncToolReturnsEmptyList(t *testing.T) {
	ps := hitltest.NewPlaneServer()
	defer ps.Close()
	prev := newHITLSyncer
	newHITLSyncer = func(context.Context, Dependencies, string) (*hitl.Syncer, error) {
		return hitl.New(ps.Client(), github.NewGitHubClient("o/r").WithExecutor(&hitltest.GHScript{}), "."), nil
	}
	defer func() { newHITLSyncer = prev }()
	srv := NewServer(ServerConfig{})
	srv.RegisterDefaultTools(Dependencies{})
	if text, isErr := callTool(t, srv, "dendrite_hitl_sync", `{}`); isErr || strings.TrimSpace(text) != "[]" {
		t.Fatalf("empty result = %q", text)
	}
}

func TestNewHITLSyncerUsesServerPlaneClient(t *testing.T) {
	ps := hitltest.NewPlaneServer()
	defer ps.Close()
	ctx := context.Background()
	s, err := newHITLSyncer(ctx, Dependencies{PlaneClient: ps.Client()}, "o/explicit")
	if err != nil || s.GitHub.Repo != "o/explicit" {
		t.Fatalf("explicit repo: %+v %v", s, err)
	}
	// The repository's own origin remote is detected when no repo is given.
	if s, err := newHITLSyncer(ctx, Dependencies{PlaneClient: ps.Client()}, ""); err != nil || !strings.Contains(s.GitHub.Repo, "/") {
		t.Fatalf("detected repo: %+v %v", s, err)
	}
	t.Chdir(t.TempDir())
	if _, err := newHITLSyncer(ctx, Dependencies{PlaneClient: ps.Client()}, ""); err == nil {
		t.Error("expected detection failure outside a git repository")
	}
	if s, err := newHITLSyncer(ctx, Dependencies{}, "o/r"); err != nil || s.GitHub.Repo != "o/r" {
		t.Fatalf("env Plane client: %v", err)
	}
}
