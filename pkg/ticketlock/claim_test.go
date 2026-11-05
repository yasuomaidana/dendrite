package ticketlock

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"Dendrite/pkg/tracker"
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
	for branch, want := range map[string]string{"agent/den-375": "den-375", "agent/": "", "feature/x": "", "develop": ""} {
		got, ok := TicketOfBranch(branch)
		if got != want || ok != (want != "") {
			t.Errorf("TicketOfBranch(%q) = %q, %v", branch, got, ok)
		}
	}
}
