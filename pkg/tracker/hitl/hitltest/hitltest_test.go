package hitltest

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"Dendrite/pkg/tracker"
	"Dendrite/pkg/tracker/plane"
)

func TestPlaneServer(t *testing.T) {
	ps := NewPlaneServer(plane.PlaneIssue{ID: "u-1", Name: "[DEN-1] one", State: "st-verify"})
	defer ps.Close()
	c := ps.Client()
	ctx := context.Background()

	states, err := c.ListStates(ctx)
	if err != nil || len(states) != len(States) {
		t.Fatalf("states = %v, %v", states, err)
	}
	issues, err := c.ListPlaneIssues(ctx)
	if err != nil || len(issues) != 1 || issues[0].Name != "[DEN-1] one" {
		t.Fatalf("issues = %v, %v", issues, err)
	}
	if err := c.UpdateStatus(ctx, "DEN-1", tracker.StatusDone); err != nil {
		t.Fatal(err)
	}
	if ps.Patches["u-1"] != "st-done" {
		t.Fatalf("patches = %v", ps.Patches)
	}

	ps.FailPatch = true
	if err := c.UpdateStatus(ctx, "DEN-1", tracker.StatusDone); err == nil {
		t.Error("FailPatch must fail updates")
	}
	ps.FailIssues = true
	if _, err := c.ListPlaneIssues(ctx); err == nil {
		t.Error("FailIssues must fail listing")
	}

	resp, err := http.Get(ps.URL + "/unknown")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown path = %d", resp.StatusCode)
	}
}

func TestGHScript(t *testing.T) {
	g := &GHScript{}
	g.On("gh issue", "generic", nil).On("gh issue list", "specific", nil).On("gh fail", "", errors.New("boom"))
	ctx := context.Background()

	if out, _ := g.Execute(ctx, "gh", "issue", "list", "--json", "x"); out != "specific" {
		t.Errorf("later registration must win, got %q", out)
	}
	if out, _ := g.Execute(ctx, "gh", "issue", "view", "1"); out != "generic" {
		t.Errorf("prefix fallback, got %q", out)
	}
	if _, err := g.Execute(ctx, "gh", "fail"); err == nil {
		t.Error("expected scripted error")
	}
	if out, err := g.Execute(ctx, "git", "status"); out != "" || err != nil {
		t.Errorf("unmatched = %q, %v", out, err)
	}
	if g.Called("gh issue") != 2 || g.Called("git") != 1 || len(g.Calls) != 4 {
		t.Errorf("calls = %v", g.Calls)
	}
}

func TestIssueListJSON(t *testing.T) {
	out := IssueListJSON(IssueRow{Number: 1, Title: "[DEN-1] a", State: "OPEN", Labels: []string{"x"}}, IssueRow{Number: 2, State: "CLOSED"})
	for _, want := range []string{`"number":1`, `"labels":[{"name":"x"}]`, `"labels":[]`, `"state":"CLOSED"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in %s", want, out)
		}
	}
	if IssueListJSON() != "[]" {
		t.Error("no rows must render []")
	}
}
