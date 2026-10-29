package ticketlock

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"Dendrite/pkg/fsutil"
	"Dendrite/pkg/tracker"
)

// Claim ties one working copy (identified by its branch) to one Plane ticket. It lives in
// .dendrite/session/claims/<branch-slug>.json, because statedir resolves every worktree of a
// repository to the main checkout's .dendrite and one claim file would be shared by all of them.
type Claim struct {
	Ticket     string         `json:"ticket"`
	SessionID  string         `json:"session_id"`
	Agent      string         `json:"agent,omitempty"`
	ModelTier  string         `json:"model_tier,omitempty"`
	Branch     string         `json:"branch"`
	Worktree   string         `json:"worktree,omitempty"`
	ClaimedAt  time.Time      `json:"claimed_at"`
	PlaneState tracker.Status `json:"plane_state"`
	// CheckedAt is when PlaneState was last confirmed against Plane; the guard trusts it for a while.
	CheckedAt time.Time `json:"checked_at"`
}

func (s *Store) claimPath(branch string) string {
	return filepath.Join(s.Dir, "claims", slug(branch)+".json")
}

// WriteClaim stores the claim of c.Branch.
func (s *Store) WriteClaim(c Claim) error {
	if c.Ticket == "" || c.Branch == "" {
		return errors.New("a claim needs a ticket and a branch")
	}
	data, _ := json.MarshalIndent(c, "", "  ")
	if err := fsutil.WriteFileAtomic(s.claimPath(c.Branch), append(data, '\n')); err != nil {
		return fmt.Errorf("write claim: %w", err)
	}
	return nil
}

// ClaimFor reads the claim of a branch; ErrNoClaim when there is none.
func (s *Store) ClaimFor(branch string) (Claim, error) {
	c, err := readJSON[Claim](s.claimPath(branch))
	if errors.Is(err, os.ErrNotExist) {
		return Claim{}, ErrNoClaim
	}
	return c, err
}

// ClaimsOf returns every claim on ticket, whichever branch holds it. Unreadable claim files are skipped.
func (s *Store) ClaimsOf(ticket string) []Claim {
	entries, _ := os.ReadDir(filepath.Join(s.Dir, "claims"))
	var out []Claim
	for _, e := range entries {
		if c, err := readJSON[Claim](filepath.Join(s.Dir, "claims", e.Name())); err == nil && strings.EqualFold(c.Ticket, ticket) {
			out = append(out, c)
		}
	}
	return out
}

// ReleaseClaim removes the claim of a branch; releasing a branch with no claim is not an error.
func (s *Store) ReleaseClaim(branch string) error {
	if err := os.Remove(s.claimPath(branch)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// ClaimOfDir returns the claim of the branch checked out in dir, if there is one.
func (s *Store) ClaimOfDir(dir string) (Claim, bool) {
	branch, err := gitOut(dir, "symbolic-ref", "--short", "HEAD")
	if err != nil {
		return Claim{}, false
	}
	c, err := s.ClaimFor(branch)
	return c, err == nil
}

// TicketOfBranch returns the ticket a dendrite working-copy branch (agent/<ticket>) is named after.
func TicketOfBranch(branch string) (string, bool) {
	t, ok := strings.CutPrefix(branch, "agent/")
	if !ok || t == "" {
		return "", false
	}
	return t, true
}
