package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"Dendrite/pkg/ticketlock"
	"Dendrite/pkg/tracker"
)

// inRepoOnBranch makes a git repository on branch, runs the test inside it and isolates the state
// directory, so claims and tokens never touch the real project.
func inRepoOnBranch(t *testing.T, branch string) (repo, state string) {
	t.Helper()
	state = t.TempDir()
	t.Setenv("DENDRITE_STATE_DIR", state)
	repo = t.TempDir()
	initTestGitRepo(t, repo, branch)
	old, _ := os.Getwd()
	if err := os.Chdir(repo); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	return repo, state
}

func claimBranch(t *testing.T, branch, ticket string) {
	t.Helper()
	c := ticketlock.Claim{Ticket: ticket, SessionID: "s-1", Branch: branch, ClaimedAt: time.Now(), PlaneState: tracker.StatusInProgress, CheckedAt: time.Now()}
	if err := ticketlock.NewStore().WriteClaim(c); err != nil {
		t.Fatal(err)
	}
}

func TestSessionResumeMintsTheSessionToken(t *testing.T) {
	_, state := inRepoOnBranch(t, "agent/den-1")
	t.Setenv(ticketlock.SessionIDEnv, "")
	t.Setenv(ticketlock.AgentEnv, "claude")
	t.Setenv(ticketlock.TierEnv, "9")
	ctx := context.Background()
	var buf bytes.Buffer

	if err := runSessionCommand(ctx, []string{"checkpoint"}, &buf); err != nil {
		t.Fatal(err)
	}
	buf.Reset()
	if err := runSessionCommand(ctx, []string{"resume"}, &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "session: s-") || !strings.Contains(buf.String(), "(agent claude, tier 9)") {
		t.Errorf("resume must print the session line:\n%s", buf.String())
	}
	if strings.Contains(buf.String(), "claimed ticket") {
		t.Errorf("no claim yet:\n%s", buf.String())
	}
	files, _ := filepath.Glob(filepath.Join(state, "session", "tokens", "*.json"))
	if len(files) != 1 {
		t.Fatalf("token files = %v", files)
	}

	claimBranch(t, "agent/den-1", "DEN-1")
	buf.Reset()
	if err := runSessionCommand(ctx, []string{"resume"}, &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "claimed ticket DEN-1") {
		t.Errorf("resume must name the claimed ticket:\n%s", buf.String())
	}
}

func TestSessionResumeJSONStaysPureJSONButStillMintsAToken(t *testing.T) {
	_, state := inRepoOnBranch(t, "agent/den-2")
	ctx := context.Background()
	var buf bytes.Buffer
	if err := runSessionCommand(ctx, []string{"checkpoint"}, &buf); err != nil {
		t.Fatal(err)
	}
	buf.Reset()
	if err := runSessionCommand(ctx, []string{"resume", "--json"}, &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(strings.TrimSpace(buf.String()), "{") || strings.Contains(buf.String(), "session: s-") {
		t.Errorf("JSON output must not carry the session line:\n%s", buf.String())
	}
	if files, _ := filepath.Glob(filepath.Join(state, "session", "tokens", "*.json")); len(files) != 1 {
		t.Errorf("token files = %v", files)
	}
}

func TestSessionResumeReportsATokenItCannotWrite(t *testing.T) {
	_, state := inRepoOnBranch(t, "agent/den-3")
	ctx := context.Background()
	var buf bytes.Buffer
	if err := runSessionCommand(ctx, []string{"checkpoint"}, &buf); err != nil {
		t.Fatal(err)
	}
	// A regular file where the tokens directory should be: only the token write fails.
	if err := os.MkdirAll(filepath.Join(state, "session"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state, "session", "tokens"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	buf.Reset()
	if err := runSessionCommand(ctx, []string{"resume"}, &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "[!] session token not written") {
		t.Errorf("the failure must be visible:\n%s", buf.String())
	}
}

func TestCheckpointRecordsTheClaimedTicket(t *testing.T) {
	repo, state := inRepoOnBranch(t, "agent/den-4")
	claimBranch(t, "agent/den-4", "DEN-4")
	if got := claimedTicket(repo); got != "DEN-4" {
		t.Errorf("claimedTicket = %q", got)
	}
	var buf bytes.Buffer
	if err := runSessionCommand(context.Background(), []string{"checkpoint", "--worktree", repo}, &buf); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(state, "session", "handoff.md"))
	if err != nil || !strings.Contains(string(data), "- **Ticket**: DEN-4") {
		t.Errorf("handoff must name the ticket: %v\n%s", err, data)
	}
}

func TestClaimedTicketIsEmptyWithoutAClaim(t *testing.T) {
	inRepoOnBranch(t, "agent/den-5")
	if got := claimedTicket(""); got != "" {
		t.Errorf("claimedTicket = %q", got)
	}
}
