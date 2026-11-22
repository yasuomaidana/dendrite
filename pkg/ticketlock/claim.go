package ticketlock

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"Dendrite/pkg/directive"
	"Dendrite/pkg/featurebranch"
	"Dendrite/pkg/fsutil"
	"Dendrite/pkg/tracker"
	"Dendrite/pkg/tracker/plane"
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
	// RefusalSuspected is set by the Stop hook when the session's last reply only declined in words (an REA
	// ticket): the session may still go on, so nothing is handed over yet. A later reply that does not decline
	// clears it, and the session-end hook hands the ticket over while it stands.
	RefusalSuspected *SuspectedRefusal `json:"refusal_suspected,omitempty"`
}

// SuspectedRefusal is the mark a claim carries while its last reply looks like a refusal.
type SuspectedRefusal struct {
	// Excerpt is the start of the reply, kept on the ticket as evidence if it is handed over.
	Excerpt string    `json:"excerpt"`
	At      time.Time `json:"at"`
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

// ticketLead matches the ticket id that starts a branch segment such as DEN-880-slug-names.
var ticketLead = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9]*-[0-9]+)(-|$)`)

// TicketOfBranch returns the ticket a dendrite working-copy branch is named after: the last segment of
// `<ROOT>-<slug>/<TICKET>-<slug>`, the folder of a trunk `<TICKET>-<slug>/main`, or the rest of a legacy
// `agent/<ticket>`.
func TicketOfBranch(branch string) (string, bool) {
	if t, ok := strings.CutPrefix(branch, "agent/"); ok {
		return t, t != ""
	}
	segments := strings.Split(branch, "/")
	last := segments[len(segments)-1]
	if last == featurebranch.TrunkLeaf && len(segments) > 1 {
		last = segments[len(segments)-2]
	}
	if m := ticketLead.FindStringSubmatch(last); m != nil {
		return strings.ToUpper(m[1]), true
	}
	return "", false
}

// OwnerAGYLabel marks a ticket AGY took over from a Claude session that refused it (the `[rea]` directive).
// Claude may not claim it again unless the claim steals it.
const OwnerAGYLabel = directive.ReaOwnerLabel

// ownerPrefix is the label family a steal clears, so a later handoff can run again.
const ownerPrefix = "owner:"

// CanonicalTicket is the IDENT-N of an issue, from its title; the reference itself when the title has none.
func CanonicalTicket(issue *plane.PlaneIssue, ref string) string { return canonical(issue, ref) }

// claudeClaims reports whether the claim is made by a Claude session: the named agent, else the detected one.
func claudeClaims(o ClaimOptions) bool {
	agent := o.Agent
	if agent == "" {
		agent = DetectAgent()
	}
	return strings.EqualFold(agent, "claude")
}

// refuseAGYOwned stops a Claude claim on a ticket AGY owns. Other agents and a steal pass. A ticket whose
// labels cannot be read is let through: the guard must not block work it cannot judge.
func (l *Lifecycle) refuseAGYOwned(ctx context.Context, ticket string, o ClaimOptions) error {
	if o.Steal || !claudeClaims(o) {
		return nil
	}
	// PatchLabels with nothing to add or drop is the only call that returns label names; it changes nothing.
	_, names, err := l.Board.Svc.PatchLabels(ctx, l.Board.Client, ticket, nil, nil)
	if err != nil {
		return nil
	}
	for _, n := range names {
		if strings.EqualFold(n, OwnerAGYLabel) {
			return fmt.Errorf("%s is owned by AGY: a Claude session refused it and it was handed over (label %s); pass --steal to take it back", ticket, OwnerAGYLabel)
		}
	}
	return nil
}

// claimRemovals are the label prefixes a claim replaces: its own agent and tier labels, the needs-input mark and,
// when Claude steals a ticket back from AGY, the owner label.
func claimRemovals(o ClaimOptions) []string {
	out := []string{"agent:", "tier:", NeedsInputLabel}
	if o.Steal && claudeClaims(o) {
		out = append(out, ownerPrefix)
	}
	return out
}
