package ticketlock

import (
	"context"
	"errors"
	"fmt"
	"html"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"Dendrite/pkg/planesvc"
	"Dendrite/pkg/projectid"
	"Dendrite/pkg/tracker"
	"Dendrite/pkg/tracker/plane"
)

// LiveClaimTTL is how long another session's claim on a ticket is respected without being renewed.
const LiveClaimTTL = 2 * time.Hour

// Board is the Plane project a ticket lives in, reached through the dendrite-plane sidecar.
type Board struct {
	Svc    *planesvc.Service
	Client *plane.Client
}

// Status reads a ticket and its lifecycle state.
func (b Board) Status(ctx context.Context, ref string) (tracker.Status, *plane.PlaneIssue, error) {
	ref = strings.TrimSpace(ref)
	if identN.MatchString(ref) {
		ref = strings.ToUpper(ref) // Plane matches the [IDENT-N] title prefix case-sensitively
	}
	issue, err := b.Svc.GetIssue(ctx, b.Client, ref)
	if err != nil {
		return "", nil, err
	}
	// An unreadable state must never be taken for Backlog: that would let a Done ticket be claimed.
	if issue.StateDetail.Name == "" && issue.StateDetail.Group == "" {
		return "", nil, fmt.Errorf("Plane did not report the workflow state of %s", ref)
	}
	return plane.StatusFor(issue.StateDetail.Group, issue.StateDetail.Name), issue, nil
}

// set moves the ticket to st.
func (b Board) set(ctx context.Context, ref string, st tracker.Status) error {
	_, err := b.Svc.SetStatus(ctx, b.Client, tracker.TicketID(ref), st)
	return err
}

// Lifecycle runs the ticket lifecycle for the session working in Dir: claim, comment, plan, handoff and
// the other transitions. The CLI (`dendrite chronos ...`) and the MCP tools call the same methods.
type Lifecycle struct {
	Board Board
	Store *Store
	Plans PlanStore
	Now   func() time.Time
	// Dir is the working copy of the session: claims are kept per branch of Dir.
	Dir string
	// Branch and Head describe Dir (seams for tests; the defaults ask git).
	Branch func(dir string) string
	Head   func(dir string) string
}

// NewLifecycle returns a lifecycle for the working copy in dir using the project's state directory and
// Dendrite's home for the plan store.
func NewLifecycle(board Board, dir string) *Lifecycle {
	home, _ := projectid.HomeDir()
	return &Lifecycle{
		Board: board, Store: NewStore(), Plans: PlanStore{Dir: filepath.Join(home, "engram", "plans")},
		Now: time.Now, Dir: dir,
		Branch: func(d string) string { b, _ := gitOut(d, "symbolic-ref", "--short", "HEAD"); return b },
		Head:   func(d string) string { h, _ := gitOut(d, "rev-parse", "--short", "HEAD"); return h },
	}
}

// ClaimOptions say who claims.
type ClaimOptions struct {
	Agent string
	Tier  string
	// Steal takes the ticket over from another live session.
	Steal bool
}

// ClaimResult is what a claim returns: the ticket now In Progress and, when one exists, its plan.
type ClaimResult struct {
	Ticket    string `json:"ticket"`
	State     string `json:"state"`
	Branch    string `json:"branch"`
	SessionID string `json:"session_id"`
	Plan      *Plan  `json:"plan,omitempty"`
}

var ticketID = regexp.MustCompile(`^\[([A-Za-z0-9]+-\d+)\]`)

// canonical is the IDENT-N of an issue, from its title; the reference itself when the title has none.
func canonical(issue *plane.PlaneIssue, ref string) string {
	if m := ticketID.FindStringSubmatch(strings.TrimSpace(issue.Name)); m != nil {
		return strings.ToUpper(m[1])
	}
	return strings.ToUpper(strings.TrimSpace(ref))
}

// Claim takes a ticket for this session: it must be in a state that allows work, not held by another live
// session, and it ends In Progress, assigned to the API key's user and labelled agent:/tier:. The claim is
// recorded for the branch checked out in Dir, which is what unlocks edits there.
func (l *Lifecycle) Claim(ctx context.Context, ref string, o ClaimOptions) (ClaimResult, error) {
	if strings.TrimSpace(ref) == "" {
		return ClaimResult{}, errors.New("a ticket is required")
	}
	st, issue, err := l.Board.Status(ctx, ref)
	if err != nil {
		return ClaimResult{}, err
	}
	ticket := canonical(issue, ref)
	if _, hint := Allowed(st, ModeStart, ticket); hint != "" {
		return ClaimResult{}, fmt.Errorf("cannot claim %s, it is %s: %s", ticket, tracker.StatusDisplayName(st), hint)
	}
	branch := l.Branch(l.Dir)
	if branch == "" {
		return ClaimResult{}, fmt.Errorf("%s is not a git working copy, so there is no branch to claim for", l.Dir)
	}
	if want, ok := TicketOfBranch(branch); ok && !strings.EqualFold(want, ticket) {
		return ClaimResult{}, fmt.Errorf("branch %s belongs to %s, not %s: work on %s in its own copy (`dendrite work new %s`)", branch, want, ticket, ticket, ticket)
	}
	now := l.Now()
	if err := l.refuseOtherSession(ticket, branch, now, o.Steal); err != nil {
		return ClaimResult{}, err
	}

	tok, ok := l.Store.LatestToken(now)
	if !ok {
		if tok, err = l.Store.MintToken(TokenOptions{Agent: o.Agent, ModelTier: o.Tier, Worktree: l.Dir}, now); err != nil {
			return ClaimResult{}, err
		}
	}
	agent, tier := firstNonEmpty(o.Agent, tok.Agent), firstNonEmpty(o.Tier, tok.ModelTier)

	if st != tracker.StatusInProgress {
		if err := l.Board.set(ctx, ticket, tracker.StatusInProgress); err != nil {
			return ClaimResult{}, fmt.Errorf("start %s: %w", ticket, err)
		}
	}
	me, err := l.Board.Svc.Me(ctx, l.Board.Client)
	if err != nil {
		return ClaimResult{}, fmt.Errorf("find the Plane user to assign: %w", err)
	}
	if _, err := l.Board.Svc.UpdateIssue(ctx, l.Board.Client, ticket, plane.UpdateIssueRequest{Assignees: []string{me.ID}}); err != nil {
		return ClaimResult{}, fmt.Errorf("assign %s: %w", ticket, err)
	}
	labels := []string{"agent:" + agent}
	if tier != "" {
		labels = append(labels, "tier:"+tier)
	}
	if _, _, err := l.Board.Svc.PatchLabels(ctx, l.Board.Client, ticket, labels, []string{"agent:", "tier:"}); err != nil {
		return ClaimResult{}, fmt.Errorf("label %s: %w", ticket, err)
	}
	claim := Claim{Ticket: ticket, SessionID: tok.SessionID, Agent: agent, ModelTier: tier, Branch: branch, Worktree: l.Dir,
		ClaimedAt: now, PlaneState: tracker.StatusInProgress, CheckedAt: now}
	if err := l.Store.WriteClaim(claim); err != nil {
		return ClaimResult{}, err
	}
	if err := l.post(ctx, ticket, "claim", fmt.Sprintf("Claimed by %s (session %s, tier %s) on branch %s.", agent, tok.SessionID, firstNonEmpty(tier, "-"), branch), claim); err != nil {
		return ClaimResult{}, err
	}

	res := ClaimResult{Ticket: ticket, State: tracker.StatusDisplayName(tracker.StatusInProgress), Branch: branch, SessionID: tok.SessionID}
	if p, err := l.GetPlan(ctx, ticket); err == nil {
		res.Plan = &p
	}
	return res, nil
}

// refuseOtherSession fails when a different branch holds a live claim on the ticket, unless steal is set,
// in which case those claims are released.
func (l *Lifecycle) refuseOtherSession(ticket, branch string, now time.Time, steal bool) error {
	for _, c := range l.Store.ClaimsOf(ticket) {
		if c.Branch == branch || (now.Sub(c.ClaimedAt) > LiveClaimTTL && now.Sub(c.CheckedAt) > LiveClaimTTL) {
			continue
		}
		if !steal {
			return fmt.Errorf("%s is claimed by %s (session %s) on branch %s since %s; pass --steal to take it over",
				ticket, firstNonEmpty(c.Agent, "another agent"), c.SessionID, c.Branch, c.ClaimedAt.Format(time.RFC3339))
		}
		if err := l.Store.ReleaseClaim(c.Branch); err != nil {
			return err
		}
	}
	return nil
}

// CommentKinds are the kinds `Comment` accepts.
var CommentKinds = []string{"progress", "milestone", "note", "handoff"}

// Comment posts a structured comment on the ticket: a visible [dendrite:<kind>] marker, the text, and a
// footer naming the session, agent, tier, branch and commit.
func (l *Lifecycle) Comment(ctx context.Context, ticket, kind, body string) error {
	valid := false
	for _, k := range CommentKinds {
		valid = valid || k == kind
	}
	if !valid {
		return fmt.Errorf("unknown comment kind %q (use %s)", kind, strings.Join(CommentKinds, ", "))
	}
	if strings.TrimSpace(body) == "" {
		return errors.New("a comment needs text")
	}
	claim, _ := l.Store.ClaimFor(l.Branch(l.Dir))
	return l.post(ctx, ticket, kind, body, claim)
}

// post writes one marker comment. The sidecar wraps the text in a paragraph and does not escape it.
func (l *Lifecycle) post(ctx context.Context, ticket, kind, body string, c Claim) error {
	text := "[dendrite:" + kind + "] " + strings.ReplaceAll(html.EscapeString(strings.TrimSpace(body)), "\n", "<br>") +
		"<br><sub>" + html.EscapeString(l.footer(c)) + "</sub>"
	if err := l.Board.Svc.Comment(ctx, l.Board.Client, ticket, text); err != nil {
		return fmt.Errorf("comment on %s: %w", ticket, err)
	}
	return nil
}

func (l *Lifecycle) footer(c Claim) string {
	parts := []string{"session " + firstNonEmpty(c.SessionID, "-"), "agent " + firstNonEmpty(c.Agent, "-"), "tier " + firstNonEmpty(c.ModelTier, "-"), "branch " + firstNonEmpty(c.Branch, l.Branch(l.Dir), "-")}
	if h := l.Head(l.Dir); h != "" {
		parts = append(parts, "commit "+h)
	}
	return strings.Join(parts, " · ")
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
