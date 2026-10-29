package ticketlock

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"Dendrite/pkg/tracker"
	"Dendrite/pkg/tracker/plane"
)

const (
	// CacheTTL is how long a claim's Plane state is trusted before Plane is asked again.
	CacheTTL = 10 * time.Minute
	// OfflineTTL is how long a cached In Progress state is accepted when Plane does not answer.
	OfflineTTL = 24 * time.Hour

	// Instruction is what every refusal ends with: how to get a session and a claim.
	Instruction = "call dendrite_ticket_claim and dendrite_query_context first (or run: dendrite session resume && dendrite work new <ticket>)"
)

// ErrLocked is returned by Require when a write is refused.
type ErrLocked struct {
	// Reason says why, in one phrase.
	Reason string
}

func (e *ErrLocked) Error() string {
	return "blocked: " + e.Reason + " — " + Instruction
}

func locked(format string, args ...any) error {
	return &ErrLocked{Reason: fmt.Sprintf(format, args...)}
}

// Deps are the outside world the guard looks at; tests replace the fields they care about.
type Deps struct {
	Store *Store
	Now   func() time.Time
	// Home is the user's home directory; plans written under Home/.claude/plans are always allowed.
	Home string
	// RepoTop returns the top level of the git repository that contains dir.
	RepoTop func(dir string) (string, error)
	// Branch returns the checked-out branch of the repository at top.
	Branch func(top string) string
	// Linked reports whether the repository at top is a Plane project.
	Linked func(top string) bool
	// PlaneStatus asks Plane for the state of a ticket of the repository at top.
	PlaneStatus func(ctx context.Context, top, ticket string) (tracker.Status, error)
}

func gitOut(dir string, args ...string) (string, error) {
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	return strings.TrimSpace(string(out)), err
}

// DefaultDeps wires the guard to the real store, git and Plane link. PlaneStatus is left to the
// caller, because the Plane client is built differently by the CLI and by tests.
func DefaultDeps(planeStatus func(ctx context.Context, top, ticket string) (tracker.Status, error)) Deps {
	home, _ := os.UserHomeDir()
	return Deps{
		Store: NewStore(),
		Now:   time.Now,
		Home:  home,
		RepoTop: func(dir string) (string, error) {
			return gitOut(dir, "rev-parse", "--show-toplevel")
		},
		Branch: func(top string) string {
			b, _ := gitOut(top, "symbolic-ref", "--short", "HEAD")
			return b
		},
		Linked: func(top string) bool {
			_, _, err := plane.LinkForDir(top)
			return err == nil
		},
		PlaneStatus: planeStatus,
	}
}

// nearestDir returns the closest existing directory at or above path, so a file that is about to be
// created can still be placed in its repository.
func nearestDir(path string) string {
	dir := path
	for {
		st, err := os.Stat(dir)
		if (err == nil && st.IsDir()) || filepath.Dir(dir) == dir {
			return dir
		}
		dir = filepath.Dir(dir)
	}
}

func within(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func isMainCheckout(top string) bool {
	st, err := os.Stat(filepath.Join(top, ".git"))
	return err == nil && st.IsDir()
}

// Require is the single write guard. It returns nil when a write to target (an absolute path) is
// allowed and an *ErrLocked when it is not. It fails closed: a missing token or claim, a ticket that
// is not In Progress, and an unreadable claim all refuse. Repositories that are not Plane projects,
// paths outside any repository, Dendrite's own state and Claude's plan files are never locked.
func Require(ctx context.Context, d Deps, target string) error {
	if d.Home != "" && within(target, filepath.Join(d.Home, ".claude", "plans")) {
		return nil
	}
	top, err := d.RepoTop(nearestDir(target))
	if err != nil || top == "" {
		return nil
	}
	if within(target, filepath.Join(top, ".dendrite")) || !d.Linked(top) {
		return nil
	}

	branch := d.Branch(top)
	switch branch {
	case "develop", "main", "master":
		if isMainCheckout(top) {
			return locked("%s is checked out in the main checkout; work in a dendrite-work/ copy", branch)
		}
	}

	now := d.Now()
	if _, ok := d.Store.LatestToken(now); !ok {
		return locked("no Dendrite session/ticket claim")
	}
	claim, err := d.Store.ClaimFor(branch)
	if errors.Is(err, ErrNoClaim) {
		return locked("no Dendrite session/ticket claim")
	}
	if err != nil {
		return locked("the claim of %s is unreadable (%v)", branch, err)
	}
	if want, ok := TicketOfBranch(branch); ok && !strings.EqualFold(want, claim.Ticket) {
		return locked("branch %s belongs to %s but the claim is for %s", branch, want, claim.Ticket)
	}
	return checkState(ctx, d, top, claim, now)
}

// checkState confirms the claimed ticket is In Progress, from the claim's cache when it is fresh and
// from Plane otherwise.
func checkState(ctx context.Context, d Deps, top string, c Claim, now time.Time) error {
	if c.PlaneState == tracker.StatusInProgress && now.Sub(c.CheckedAt) < CacheTTL {
		return nil
	}
	st, err := d.PlaneStatus(ctx, top, c.Ticket)
	if err != nil {
		if c.PlaneState == tracker.StatusInProgress && now.Sub(c.CheckedAt) < OfflineTTL {
			return nil
		}
		return locked("Plane did not answer and %s has no recent In Progress state (%v)", c.Ticket, err)
	}
	c.PlaneState, c.CheckedAt = st, now
	if err := d.Store.WriteClaim(c); err != nil {
		return locked("cannot record the claim check (%v)", err)
	}
	if st != tracker.StatusInProgress {
		return locked("%s is %s, not In Progress", c.Ticket, tracker.StatusDisplayName(st))
	}
	return nil
}
