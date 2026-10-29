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

	"Dendrite/pkg/agentjobs"
	"Dendrite/pkg/devtool"
	"Dendrite/pkg/ticketlock"
	"Dendrite/pkg/tracker/plane"
)

func TestPreCommitHonoursTheGuardBypass(t *testing.T) {
	r := newHookRig(t, "To Do")
	if err := os.WriteFile(filepath.Join(r.repo, "a.go"), []byte("package a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", r.repo, "add", "a.go").CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if err := runPreCommit(context.Background(), r.deps, r.repo); err == nil {
		t.Fatal("staged work without a claim must be refused while the guard is on")
	}
	t.Setenv(devtool.TicketGuardEnv, "off")
	if err := runPreCommit(context.Background(), r.deps, r.repo); err != nil {
		t.Errorf("--no-ticket-guard / %s=off is the deliberate bypass: %v", devtool.TicketGuardEnv, err)
	}
}

func TestCommitProgressIsTheSubjectAndShortSHA(t *testing.T) {
	for name, tc := range map[string]struct {
		res  devtool.CommitResult
		want string
	}{
		"long sha, body dropped": {devtool.CommitResult{Message: "[DEN-1] feat: a thing\n\nlong body", CommitSHA: "0123456789abcdef"}, "[DEN-1] feat: a thing (0123456)"},
		"short sha kept":         {devtool.CommitResult{Message: "fix: x", CommitSHA: "abc"}, "fix: x (abc)"},
	} {
		if got := commitProgress(&tc.res); got != tc.want {
			t.Errorf("%s: %q, want %q", name, got, tc.want)
		}
	}
}

func TestCheckpointSummary(t *testing.T) {
	full := checkpointSummary(&agentjobs.Checkpoint{Branch: "agent/den-1", PendingDecisions: []string{"a", "b"}, Next: []string{"DEN-2", "DEN-3"}, Notes: "n1"})
	for _, want := range []string{"Checkpoint on agent/den-1", "Decisions:\n- a\n- b", "Next: DEN-2, DEN-3", "Notes:\nn1"} {
		if !strings.Contains(full, want) {
			t.Errorf("summary %q lacks %q", full, want)
		}
	}
	if got := checkpointSummary(&agentjobs.Checkpoint{Branch: "develop"}); got != "Checkpoint on develop" {
		t.Errorf("an empty checkpoint is just its branch: %q", got)
	}
}

func TestPostTicketCommentIsSilentWithoutAClaimAndWarnsWhenPlaneFails(t *testing.T) {
	r := newHookRig(t, "To Do")
	ctx := context.Background()
	var out bytes.Buffer

	postTicketComment(ctx, &out, r.repo, "progress", "nobody claimed")
	postTicketComment(ctx, &out, "", "progress", "no dir either")
	if out.Len() != 0 || len(r.board.Comments("DEN-377")) != 0 {
		t.Fatalf("a copy with no claim posts and says nothing: %q", out.String())
	}

	r.claim()
	before := len(r.board.Comments("DEN-377"))
	postTicketComment(ctx, &out, r.repo, "progress", "did a thing (abc1234)")
	comments := r.board.Comments("DEN-377")
	if out.Len() != 0 || len(comments) != before+1 || !strings.HasPrefix(comments[before].Text(), "[dendrite:progress] did a thing (abc1234)") {
		t.Fatalf("claimed copy: out %q, comments %d", out.String(), len(comments))
	}

	prev := newPlaneClient
	newPlaneClient = func() (*plane.Client, error) { return nil, errors.New("no Plane here") }
	defer func() { newPlaneClient = prev }()
	postTicketComment(ctx, &out, r.repo, "progress", "lost")
	if !strings.Contains(out.String(), "[!] ticket comment not posted: no Plane here") {
		t.Errorf("a failure is a warning: %q", out.String())
	}
}

func TestQuantalAndCheckpointCommentOnTheClaimedTicket(t *testing.T) {
	r := newHookRig(t, "To Do")
	ctx := context.Background()
	for k, v := range map[string]string{"GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@example.com", "GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@example.com"} {
		t.Setenv(k, v)
	}
	r.claim()
	if err := os.WriteFile(filepath.Join(r.repo, "a.go"), []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	opts := devtool.CommitOptions{RepoDir: r.repo, TicketID: "DEN-377", Message: "feat: add a", StageAll: true, NoPush: true, NoSmart: true, NoTicketGuard: true}
	if err := runQuantal(ctx, opts, &out); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if !lastCommentHas(r, "[dendrite:progress] [DEN-377] feat: add a (") {
		t.Errorf("the commit must be reported on the ticket: %v", r.board.Comments("DEN-377"))
	}

	opts.DryRun = true
	n := len(r.board.Comments("DEN-377"))
	if err := runQuantal(ctx, opts, &out); err == nil || len(r.board.Comments("DEN-377")) != n {
		t.Errorf("a dry run commits nothing and so reports nothing (err %v)", err)
	}

	cp := sessionCheckpointOptions{Worktree: r.repo, Path: filepath.Join(t.TempDir(), "handoff.md"), Decisions: []string{"keep it small"}, Next: []string{"DEN-379"}}
	if err := runSessionCheckpoint(ctx, cp, &out); err != nil {
		t.Fatal(err)
	}
	if !lastCommentHas(r, "[dendrite:milestone] Checkpoint on agent/den-377") {
		t.Errorf("the checkpoint must be reported on the ticket: %v", r.board.Comments("DEN-377"))
	}
}

func lastCommentHas(r *hookRig, prefix string) bool {
	c := r.board.Comments("DEN-377")
	return len(c) > 0 && strings.HasPrefix(c[len(c)-1].Text(), prefix)
}

func finishChecks(gatePasses bool) ticketlock.FinishChecks {
	return ticketlock.FinishChecks{
		Gate: func(context.Context, string) (ticketlock.GateResult, error) {
			return ticketlock.GateResult{Passed: gatePasses, Report: "gate report"}, nil
		},
		Review:   func(context.Context, ticketlock.ReviewInput) ([]ticketlock.ACVerdict, error) { return nil, nil },
		Diff:     func(string) string { return "" },
		AutoDone: true,
	}
}

func TestChronosFinishReportsTheOutcome(t *testing.T) {
	ctx := context.Background()
	r := newHookRig(t, "To Do")
	r.claim()
	var out bytes.Buffer
	if err := runChronosFinish(ctx, lifecycleIn(mustClient(t), r.repo), "DEN-377", finishChecks(true), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "[✓] DEN-377 is now Done") || !strings.Contains(out.String(), "no [AC-n] criteria") {
		t.Errorf("output = %q", out.String())
	}

	r2 := newHookRig(t, "To Do")
	r2.claim()
	jsonOutput(t, true)
	out.Reset()
	if err := runChronosFinish(ctx, lifecycleIn(mustClient(t), r2.repo), "DEN-377", finishChecks(true), &out); err != nil || !strings.Contains(out.String(), `"state": "Done"`) {
		t.Errorf("json output = %q, %v", out.String(), err)
	}
	jsonOutput(t, false)
}

func TestChronosFinishExitsNonZeroWhenBlockedAndRefusesAWrongState(t *testing.T) {
	ctx := context.Background()
	r := newHookRig(t, "To Do")
	r.claim()
	var out bytes.Buffer
	err := runChronosFinish(ctx, lifecycleIn(mustClient(t), r.repo), "DEN-377", finishChecks(false), &out)
	var usage *usageError
	if !errors.As(err, &usage) || !strings.Contains(err.Error(), "DEN-377 was blocked") {
		t.Fatalf("a blocked finish is a usage error: %v", err)
	}
	if err := runChronosFinish(ctx, lifecycleIn(mustClient(t), r.repo), "DEN-999", finishChecks(true), &out); err == nil {
		t.Error("an unknown ticket is an error")
	}
}

func TestChronosFinishCommandIsRegistered(t *testing.T) {
	cmd := newChronosFinishCmd()
	if cmd.Name() != "finish" || cmd.Flags().Lookup("worktree") == nil {
		t.Errorf("command = %s", cmd.Name())
	}
}
