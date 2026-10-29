package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/planesvc/planetest"
	"Dendrite/pkg/ticketlock"
	"Dendrite/pkg/tracker"
	"Dendrite/pkg/tracker/plane"
)

// hookRig is a Plane board plus a working copy on agent/den-377, with the environment pointing the hooks at it.
type hookRig struct {
	t     *testing.T
	board *planetest.Board
	repo  string
	deps  ticketlock.Deps
}

func newHookRig(t *testing.T, state string) *hookRig {
	t.Helper()
	jsonOutput(t, false)
	t.Setenv("DENDRITE_HOME", t.TempDir())
	t.Setenv("DENDRITE_STATE_DIR", t.TempDir())
	t.Setenv("DENDRITE_AGENT", "claude")
	board := planetest.NewBoard(t, map[string]string{"DEN-377": state})
	t.Setenv("PLANE_BASE_URL", board.URL)
	t.Setenv("PLANE_API_KEY", "k")
	t.Setenv("PLANE_WORKSPACE_SLUG", planetest.BoardWorkspace)
	t.Setenv("PLANE_PROJECT_ID", planetest.BoardProject)
	client, err := plane.NewClient(plane.Config{BaseURL: board.URL, WorkspaceSlug: planetest.BoardWorkspace, ProjectID: planetest.BoardProject, DirectToken: "k"}, board.Client())
	if err != nil {
		t.Fatal(err)
	}
	prev := newPlaneClient
	newPlaneClient = func() (*plane.Client, error) { return client, nil }
	t.Cleanup(func() { newPlaneClient = prev })

	repo := t.TempDir()
	for _, args := range [][]string{{"init", "-q", "-b", "develop"}, {"commit", "-q", "--allow-empty", "-m", "base"}, {"checkout", "-q", "-b", "agent/den-377"}} {
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	deps := ticketlock.DefaultDeps(func(ctx context.Context, top, ticket string) (tracker.Status, error) {
		return ticketlock.PlaneStatusFor(ctx, top, ticket)
	})
	deps.Linked = func(string) bool { return true }
	deps.Home = t.TempDir()
	return &hookRig{t: t, board: board, repo: repo, deps: deps}
}

func (r *hookRig) event(tool, file, plan string) *strings.Reader {
	body := `{"session_id":"s-1","cwd":"` + r.repo + `","tool_name":"` + tool + `","tool_input":{"file_path":"` + file + `","plan":` + jsonString(plan) + `}}`
	return strings.NewReader(body)
}

func jsonString(s string) string {
	var b bytes.Buffer
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"', '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '\n':
			b.WriteString(`\n`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func (r *hookRig) claim() {
	r.t.Helper()
	if _, err := lifecycleIn(mustClient(r.t), r.repo).Claim(context.Background(), "DEN-377", ticketlock.ClaimOptions{}); err != nil {
		r.t.Fatal(err)
	}
}

func mustClient(t *testing.T) *plane.Client {
	t.Helper()
	c, err := newPlaneClient()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestPreEditRefusesUntilTheTicketIsClaimed(t *testing.T) {
	r := newHookRig(t, "To Do")
	ctx := context.Background()
	file := filepath.Join(r.repo, "pkg", "new.go")

	err := runPreEdit(ctx, r.event("Edit", file, ""), r.deps)
	var usage *usageError
	if !errors.As(err, &usage) || !strings.Contains(err.Error(), "blocked: no Dendrite session/ticket claim") ||
		!strings.Contains(err.Error(), "dendrite_ticket_claim") {
		t.Fatalf("an unclaimed edit must be a usage error (exit 2) that says how to claim: %v", err)
	}

	r.claim()
	if err := runPreEdit(ctx, r.event("Edit", file, ""), r.deps); err != nil {
		t.Errorf("after the claim the edit is allowed: %v", err)
	}
	if err := runPreEdit(ctx, r.event("Write", filepath.Join(r.t.TempDir(), "elsewhere.txt"), ""), r.deps); err != nil {
		t.Errorf("a file outside any repository is never locked: %v", err)
	}
}

func TestPreEditAllowsWhatItCannotJudge(t *testing.T) {
	r := newHookRig(t, "To Do")
	ctx := context.Background()
	for name, in := range map[string]string{"empty": "", "garbage": "{not json", "nothing known": "{}"} {
		if err := runPreEdit(ctx, strings.NewReader(in), r.deps); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	if err := runPreEdit(ctx, strings.NewReader(`{"session_id":"s","tool_name":"ExitPlanMode","cwd":"`+r.repo+`"}`), r.deps); err != nil {
		t.Errorf("a tool with no file is allowed: %v", err)
	}
}

func TestPreEditHonoursTheTicketsState(t *testing.T) {
	r := newHookRig(t, "To Do")
	ctx := context.Background()
	r.claim()
	file := filepath.Join(r.repo, "a.go")
	if err := runPreEdit(ctx, r.event("Edit", file, ""), r.deps); err != nil {
		t.Fatal(err)
	}
	// The ticket is taken out of In Progress behind the session's back, after its cached state went stale.
	if _, err := lifecycleIn(mustClient(t), r.repo).Transition(ctx, "DEN-377", "backlog", ticketlock.Handoff{}); err != nil {
		t.Fatal(err)
	}
	claim := ticketlock.Claim{Ticket: "DEN-377", Branch: "agent/den-377", Worktree: r.repo, SessionID: "s-1", PlaneState: tracker.StatusInProgress}
	if err := ticketlock.NewStore().WriteClaim(claim); err != nil {
		t.Fatal(err)
	}
	if err := runPreEdit(ctx, r.event("Edit", file, ""), r.deps); err == nil || !strings.Contains(err.Error(), "is Backlog, not In Progress") {
		t.Errorf("a ticket that is no longer In Progress must lock the copy: %v", err)
	}
}

func TestPreCommitChecksTheStagedPaths(t *testing.T) {
	r := newHookRig(t, "To Do")
	ctx := context.Background()
	if err := os.WriteFile(filepath.Join(r.repo, "a.go"), []byte("package a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", r.repo, "add", "a.go").CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	var usage *usageError
	if err := runPreCommit(ctx, r.deps, r.repo); !errors.As(err, &usage) {
		t.Fatalf("staged work without a claim must be refused with a usage error: %v", err)
	}
	r.claim()
	if err := runPreCommit(ctx, r.deps, r.repo); err != nil {
		t.Errorf("with a claim: %v", err)
	}
}

func TestPlanSavedStoresThePlanOnTheClaimedTicketOrKeepsItLocally(t *testing.T) {
	r := newHookRig(t, "To Do")
	ctx := context.Background()
	var out bytes.Buffer

	if err := runPlanSaved(ctx, r.event("ExitPlanMode", "", "- [ ] one\n- [ ] two"), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "plan kept locally (unclaimed/s-1)") {
		t.Errorf("unclaimed: %s", out.String())
	}
	out.Reset()
	r.claim()
	if err := runPlanSaved(ctx, r.event("ExitPlanMode", "", "- [ ] one\n- [ ] two"), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "[✓] plan v2 stored on DEN-377 (2 step(s) left)") {
		t.Errorf("claimed: %s", out.String())
	}
	if plan, err := lifecycleIn(mustClient(t), r.repo).GetPlan(ctx, "DEN-377"); err != nil || plan.NextStep != "one" {
		t.Errorf("the ticket's plan = %+v, %v", plan, err)
	}

	out.Reset()
	if err := runPlanSaved(ctx, strings.NewReader("{broken"), &out); err != nil || out.Len() != 0 {
		t.Errorf("malformed input: %v %q", err, out.String())
	}
	t.Setenv("PLANE_API_KEY", "")
	if err := runPlanSaved(ctx, r.event("ExitPlanMode", "", "- [ ] one"), &out); err != nil || !strings.Contains(out.String(), "[!] plan not stored on DEN-377") {
		t.Errorf("a hook after the fact never fails the agent, it reports: %v %q", err, out.String())
	}
}

func TestSessionEndHandsOverATicketLeftInProgress(t *testing.T) {
	r := newHookRig(t, "To Do")
	ctx := context.Background()
	var out bytes.Buffer

	if err := runSessionEnd(ctx, r.event("", "", ""), &out); err != nil || out.Len() != 0 {
		t.Errorf("no claim, nothing to hand off: %v %q", err, out.String())
	}
	r.claim()
	if err := runSessionEnd(ctx, r.event("", "", ""), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "DEN-377 was still In Progress: blocked with an automatic handoff") || r.board.State("DEN-377") != "Blocked" {
		t.Errorf("state=%s output:\n%s", r.board.State("DEN-377"), out.String())
	}
	if labels := strings.Join(r.board.LabelNames("DEN-377"), ","); !strings.Contains(labels, "needs-input") {
		t.Errorf("labels = %s", labels)
	}

	out.Reset()
	if err := runSessionEnd(ctx, strings.NewReader("{broken"), &out); err != nil || out.Len() != 0 {
		t.Errorf("malformed input: %v %q", err, out.String())
	}
	if err := runSessionEnd(ctx, r.event("", "", ""), &out); err != nil || out.Len() != 0 {
		t.Errorf("a ticket already Blocked is left alone: %v %q", err, out.String())
	}
}

func TestHookDirAndRegistration(t *testing.T) {
	if hookDir(ticketlock.HookInput{}) != "." || hookDir(ticketlock.HookInput{Cwd: "/x"}) != "/x" {
		t.Error("hookDir")
	}
	var buf bytes.Buffer
	if err := runCLI(context.Background(), &buf, "hook", "--help"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"pre-tool", "pre-edit", "pre-commit", "plan-saved", "session-end"} {
		if !strings.Contains(buf.String(), name) {
			t.Errorf("`dendrite hook` does not list %s:\n%s", name, buf.String())
		}
	}
	if d := hookDeps(); d.PlaneStatus == nil {
		t.Error("hookDeps")
	}
}

func TestHookCommandsRunThroughTheCLI(t *testing.T) {
	r := newHookRig(t, "To Do")
	var buf bytes.Buffer
	prev := hookDeps
	hookDeps = func() ticketlock.Deps { return r.deps }
	t.Cleanup(func() { hookDeps = prev })

	t.Chdir(r.repo)
	if err := runCLI(context.Background(), &buf, "hook", "pre-commit"); err != nil {
		t.Errorf("nothing staged: %v", err)
	}
	if err := os.WriteFile(filepath.Join(r.repo, "a.go"), []byte("package a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", r.repo, "add", "a.go").CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if err := runCLI(context.Background(), &buf, "hook", "pre-commit"); err == nil || !strings.Contains(err.Error(), "no Dendrite session/ticket claim") {
		t.Errorf("staged work without a claim through the CLI: %v", err)
	}
}
