package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"Dendrite/pkg/tracker/github"
	"Dendrite/pkg/tracker/hitl"
	"Dendrite/pkg/tracker/hitl/hitltest"
	"Dendrite/pkg/tracker/plane"
)

// offlineSyncer points github-sync and hitl-sync at a local Plane server and a
// scripted gh so tests never reach the real Plane instance or GitHub.
func offlineSyncer(t *testing.T, issues ...plane.PlaneIssue) (*hitltest.PlaneServer, *hitltest.GHScript) {
	t.Helper()
	ps := hitltest.NewPlaneServer(issues...)
	t.Cleanup(ps.Close)
	gh := &hitltest.GHScript{}
	prev := newHITLSyncer
	newHITLSyncer = func(_ context.Context, repo string) (*hitl.Syncer, error) {
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

func TestGitHubSyncCommand(t *testing.T) {
	_, gh := offlineSyncer(t,
		plane.PlaneIssue{ID: "u-1", Name: "[DEN-1] feat(cli): new", State: "st-verify"},
		plane.PlaneIssue{ID: "u-2", Name: "[DEN-2] done", State: "st-done"},
		plane.PlaneIssue{ID: "u-3", Name: "[DEN-2] duplicate", State: "st-verify", SequenceID: 5},
	)
	full := []string{github.LabelToVerify, github.LabelHITL}
	gh.On("gh issue list", hitltest.IssueListJSON(hitltest.IssueRow{Number: 9, Title: "[DEN-2] done", State: "OPEN", Labels: full}), nil)
	gh.On("gh issue create", "https://github.com/o/r/issues/10", nil)
	ctx := context.Background()
	var out bytes.Buffer

	if err := runGitHubSync(ctx, []string{"--dry-run"}, &out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Dry run for verification issues in o/r (2 tickets, 1 To Verify)", "Skipped duplicate ticket ID DEN-2 (Plane #5)", "would create DEN-1 (new issue)", "would close DEN-2 (#9)"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("dry run output missing %q:\n%s", want, out.String())
		}
	}
	if gh.Called("gh issue create")+gh.Called("gh issue close") != 0 {
		t.Fatal("dry run changed GitHub")
	}

	out.Reset()
	gh.On("gh issue close", "", errors.New("forbidden"))
	err := runGitHubSync(ctx, nil, &out)
	if err == nil || !strings.Contains(err.Error(), "1 of 2 GitHub changes failed") {
		t.Fatalf("expected partial failure, got %v", err)
	}
	if !strings.Contains(out.String(), "create DEN-1 (#10)") || !strings.Contains(out.String(), "[failed: forbidden]") {
		t.Errorf("apply output:\n%s", out.String())
	}

	out.Reset()
	if err := runGitHubSync(ctx, []string{"--json", "--dry-run"}, &out); err != nil || !strings.Contains(out.String(), `"repo": "o/r"`) {
		t.Fatalf("json: %v\n%s", err, out.String())
	}
}

func TestGitHubSyncCommandInSyncAndErrors(t *testing.T) {
	offlineSyncer(t)
	ctx := context.Background()
	var out bytes.Buffer
	if err := runGitHubSync(ctx, nil, &out); err != nil || !strings.Contains(out.String(), "already in sync") {
		t.Fatalf("in sync: %v\n%s", err, out.String())
	}
	if err := runGitHubSync(ctx, []string{"--bogus"}, &out); err == nil {
		t.Error("expected flag error")
	}
	if err := runGitHubSync(ctx, []string{"-r", "fail/me"}, &out); err == nil {
		t.Error("expected setup error")
	}

	_, gh := offlineSyncer(t)
	gh.On("gh issue list", "", errors.New("offline"))
	if err := runGitHubSync(ctx, nil, &out); err == nil {
		t.Error("expected sync error")
	}
	handleGitHubSyncCommand(ctx, nil) // prints the error instead of returning it
}

func TestHITLSyncCommand(t *testing.T) {
	ps, gh := offlineSyncer(t,
		plane.PlaneIssue{ID: "u-1", Name: "[DEN-1] no issue yet", State: "st-verify"},
		plane.PlaneIssue{ID: "u-2", Name: "[DEN-2] approved", State: "st-verify"},
		plane.PlaneIssue{ID: "u-3", Name: "[DEN-3] has feedback", State: "st-verify"},
		plane.PlaneIssue{ID: "u-4", Name: "[DEN-4] waiting", State: "st-verify"},
	)
	full := []string{github.LabelToVerify, github.LabelHITL}
	gh.On("gh issue list", hitltest.IssueListJSON(
		hitltest.IssueRow{Number: 2, Title: "[DEN-2] approved", State: "CLOSED", StateReason: "COMPLETED", Labels: full},
		hitltest.IssueRow{Number: 3, Title: "[DEN-3] has feedback", State: "OPEN", Labels: full},
		hitltest.IssueRow{Number: 4, Title: "[DEN-4] waiting", State: "OPEN", Labels: full},
	), nil)
	gh.On("gh issue view 3", `{"comments":[{"author":{"login":"rev"},"body":"Rename the flag\nand more","createdAt":"2026-10-18T00:00:00Z"}]}`, nil)
	gh.On("gh issue view 4", `{"comments":[]}`, nil)
	ctx := context.Background()
	var out bytes.Buffer

	if err := runHITLSync(ctx, []string{"--dry-run"}, &out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"(4 tickets To Verify)",
		"[DEN-1] no-issue (no issue): no issue yet -> run `dendrite github-sync`",
		"[DEN-2] approved (#2): approved -> would move to Done",
		"[DEN-3] feedback (#3): has feedback -> 1 comment(s), latest by @rev: Rename the flag",
		"[DEN-4] waiting (#4)",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in:\n%s", want, out.String())
		}
	}
	if len(ps.Patches) != 0 {
		t.Fatal("dry run moved tickets")
	}

	out.Reset()
	if err := runHITLSync(ctx, nil, &out); err != nil || !strings.Contains(out.String(), "-> moved to Done in Plane") || ps.Patches["u-2"] != "st-done" {
		t.Fatalf("apply: %v %v\n%s", err, ps.Patches, out.String())
	}

	out.Reset()
	ps.FailPatch = true
	if err := runHITLSync(ctx, []string{"--json"}, &out); err != nil || !strings.Contains(out.String(), `"outcome": "approved"`) {
		t.Fatalf("json: %v\n%s", err, out.String())
	}
	out.Reset()
	if err := runHITLSync(ctx, nil, &out); err != nil || !strings.Contains(out.String(), "[failed:") {
		t.Fatalf("failed update must be shown: %v\n%s", err, out.String())
	}
}

func TestHITLSyncCommandEmptyAndErrors(t *testing.T) {
	_, gh := offlineSyncer(t)
	ctx := context.Background()
	var out bytes.Buffer
	if err := runHITLSync(ctx, []string{"--json"}, &out); err != nil || strings.TrimSpace(out.String()) != "[]" {
		t.Fatalf("empty json = %q, %v", out.String(), err)
	}
	if err := runHITLSync(ctx, []string{"--bogus"}, &out); err == nil {
		t.Error("expected flag error")
	}
	if err := runHITLSync(ctx, []string{"--repo", "fail/me"}, &out); err == nil {
		t.Error("expected setup error")
	}
	gh.On("gh issue list", "", errors.New("offline"))
	if err := runHITLSync(ctx, nil, &out); err == nil {
		t.Error("expected sync error")
	}
	handleQualityCommand(ctx, []string{"hitl-sync"}) // prints the error instead of returning it
}

func TestFirstLine(t *testing.T) {
	if got := firstLine("  one\ntwo"); got != "one" {
		t.Errorf("got %q", got)
	}
	long := strings.Repeat("x", 120)
	if got := firstLine(long); len([]rune(got)) != 101 || !strings.HasSuffix(got, "…") {
		t.Errorf("long line not truncated: %q", got)
	}
}

func TestGitHubSyncTicketFilter(t *testing.T) {
	_, gh := offlineSyncer(t,
		plane.PlaneIssue{ID: "u-1", Name: "[DEN-1] one", State: "st-verify"},
		plane.PlaneIssue{ID: "u-2", Name: "[DEN-2] two", State: "st-verify"},
	)
	var out bytes.Buffer
	if err := runGitHubSync(context.Background(), []string{"--dry-run", "--ticket", "DEN-2, ,"}, &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "DEN-1 ") || !strings.Contains(out.String(), "would create DEN-2") {
		t.Errorf("filter output:\n%s", out.String())
	}
	out.Reset()
	if err := runHITLSync(context.Background(), []string{"--ticket", "DEN-1"}, &out); err != nil || strings.Contains(out.String(), "DEN-2") {
		t.Errorf("hitl filter: %v\n%s", err, out.String())
	}
	_ = gh
}
