package ticketlock

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"Dendrite/pkg/tracker"
	"Dendrite/pkg/tracker/plane"
)

func sampleClaim(branch string) Claim {
	return Claim{Ticket: "DEN-375", SessionID: "s-1", Branch: branch, ClaimedAt: t0, PlaneState: tracker.StatusInProgress, CheckedAt: t0}
}

func TestClaimRoundTripPerBranch(t *testing.T) {
	s := &Store{Dir: t.TempDir()}
	if _, err := s.ClaimFor("agent/den-375"); !errors.Is(err, ErrNoClaim) {
		t.Fatalf("no claim yet: %v", err)
	}
	if err := s.WriteClaim(sampleClaim("agent/den-375")); err != nil {
		t.Fatal(err)
	}
	other := sampleClaim("agent/den-376")
	other.Ticket = "DEN-376"
	if err := s.WriteClaim(other); err != nil {
		t.Fatal(err)
	}
	got, err := s.ClaimFor("agent/den-375")
	if err != nil || got.Ticket != "DEN-375" || got.PlaneState != tracker.StatusInProgress {
		t.Fatalf("claim = %+v, err = %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(s.Dir, "claims", "agent-den-375.json")); err != nil {
		t.Errorf("claims are keyed by the branch slug: %v", err)
	}
	if err := s.ReleaseClaim("agent/den-375"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimFor("agent/den-375"); !errors.Is(err, ErrNoClaim) {
		t.Errorf("released claim still there: %v", err)
	}
	if _, err := s.ClaimFor("agent/den-376"); err != nil {
		t.Errorf("releasing one branch touched another: %v", err)
	}
	if err := s.ReleaseClaim("agent/never-claimed"); err != nil {
		t.Errorf("releasing nothing must not fail: %v", err)
	}
}

func TestClaimErrors(t *testing.T) {
	s := &Store{Dir: t.TempDir()}
	for _, c := range []Claim{{Branch: "b"}, {Ticket: "DEN-1"}} {
		if err := s.WriteClaim(c); err == nil {
			t.Errorf("claim %+v must be refused", c)
		}
	}
	if err := brokenStore(t).WriteClaim(sampleClaim("b")); err == nil {
		t.Error("a store that cannot be written must fail")
	}

	path := s.claimPath("bad")
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	_ = os.WriteFile(path, []byte("{"), 0o644)
	if _, err := s.ClaimFor("bad"); err == nil || errors.Is(err, ErrNoClaim) {
		t.Errorf("a corrupt claim must be an error, not 'no claim': %v", err)
	}

	// A directory where the claim file should be cannot be removed with Remove.
	dir := s.claimPath("stuck")
	_ = os.MkdirAll(filepath.Join(dir, "child"), 0o755)
	if err := s.ReleaseClaim("stuck"); err == nil {
		t.Error("an unremovable claim must be reported")
	}
}

func TestClaimOfDirFollowsTheCheckedOutBranch(t *testing.T) {
	s := &Store{Dir: t.TempDir()}
	if _, ok := s.ClaimOfDir(t.TempDir()); ok {
		t.Error("a directory outside git has no claim")
	}
	repo := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("checkout", "-q", "-b", "agent/den-375")
	if _, ok := s.ClaimOfDir(repo); ok {
		t.Error("no claim written yet")
	}
	_ = s.WriteClaim(sampleClaim("agent/den-375"))
	if c, ok := s.ClaimOfDir(repo); !ok || c.Ticket != "DEN-375" {
		t.Errorf("claim = %+v, %v", c, ok)
	}
}

func TestTicketOfBranch(t *testing.T) {
	for _, c := range []struct{ branch, want string }{
		{"agent/den-375", "den-375"}, {"agent/", ""}, {"feature/x", ""}, {"develop", ""}, {"spike/main", ""}, {"main", ""},
		{"DEN-890-fix-login-crash/main", "DEN-890"}, {"DEN-890/main", "DEN-890"}, {"DEN-8800x/main", ""},
		{"DEN-880-branch-naming/DEN-881-slug-names", "DEN-881"}, {"den-880-branch-naming/den-881", "DEN-881"},
		{"feature/DEN-840-work", "DEN-840"},
	} {
		got, ok := TicketOfBranch(c.branch)
		if got != c.want || ok != (c.want != "") {
			t.Errorf("TicketOfBranch(%q) = %q, %v", c.branch, got, ok)
		}
	}
}

// handOverToAGY labels the ticket the way the REA ownership transfer does.
func handOverToAGY(t *testing.T, s *session, ticket string) {
	t.Helper()
	if _, _, err := s.Board.Svc.PatchLabels(context.Background(), s.Board.Client, ticket, []string{OwnerAGYLabel}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestClaudeCannotClaimATicketAGYOwnsUnlessItSteals(t *testing.T) {
	s := newSession(t, map[string]string{"DEN-377": "In Progress"})
	handOverToAGY(t, s, "DEN-377")

	// the agent comes from DENDRITE_AGENT (claude, set by newSession) when the claim names none
	_, err := s.Claim(context.Background(), "DEN-377", ClaimOptions{})
	if err == nil || !strings.Contains(err.Error(), "owned by AGY") || !strings.Contains(err.Error(), "--steal") {
		t.Fatalf("a Claude claim on an AGY-owned ticket: %v", err)
	}
	if _, err := s.Store.ClaimFor(s.branch); err != ErrNoClaim {
		t.Errorf("a refused claim must leave no claim: %v", err)
	}

	if _, err := s.Claim(context.Background(), "DEN-377", ClaimOptions{Agent: "agy"}); err != nil {
		t.Errorf("AGY owns it and may claim it: %v", err)
	}
	if got := s.board.LabelNames("DEN-377"); !reflect.DeepEqual(got, []string{OwnerAGYLabel, "agent:agy", "tier:7"}) {
		t.Errorf("AGY keeps the owner label: %v", got)
	}

	if _, err := s.Claim(context.Background(), "DEN-377", ClaimOptions{Agent: "claude", Steal: true}); err != nil {
		t.Fatalf("a steal takes it back: %v", err)
	}
	if got := s.board.LabelNames("DEN-377"); !reflect.DeepEqual(got, []string{"agent:claude", "tier:7"}) {
		t.Errorf("stealing it back clears the owner label so a later handoff can run: %v", got)
	}
}

func TestOwnerGuardLetsThroughWhatItCannotRead(t *testing.T) {
	s := newSession(t, map[string]string{"DEN-377": "To Do"})
	s.board.Fail("PATCH", 0)
	// the label lookup (a PATCH) fails, then the claim's own start fails: the guard itself must not be what stops it
	_, err := s.Claim(context.Background(), "DEN-377", ClaimOptions{Agent: "claude"})
	if err == nil || strings.Contains(err.Error(), "owned by AGY") {
		t.Errorf("an unreadable label list must not read as ownership: %v", err)
	}
}

func TestCanonicalTicketComesFromTheTitleOrTheReference(t *testing.T) {
	if got := CanonicalTicket(&plane.PlaneIssue{Name: "[den-12] x"}, "ref"); got != "DEN-12" {
		t.Errorf("from the title: %q", got)
	}
	if got := CanonicalTicket(&plane.PlaneIssue{Name: "no id"}, " den-7 "); got != "DEN-7" {
		t.Errorf("from the reference: %q", got)
	}
}
