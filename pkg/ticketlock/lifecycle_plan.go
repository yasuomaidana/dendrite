package ticketlock

import (
	"context"
	"errors"
	"fmt"
	"html"
	"regexp"
	"sort"
	"strings"

	"Dendrite/pkg/tracker"
)

var identN = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*-\d+$`)

// keyOf is the IDENT-N a ticket's plan is stored under. A reference that already has that shape is used as
// is, so a plan can be read while Plane is down; anything else (a Plane number, a UUID) is looked up.
func (l *Lifecycle) keyOf(ctx context.Context, ref string) string {
	if identN.MatchString(strings.TrimSpace(ref)) {
		return strings.ToUpper(strings.TrimSpace(ref))
	}
	if _, issue, err := l.Board.Status(ctx, ref); err == nil {
		return canonical(issue, ref)
	}
	return strings.ToUpper(strings.TrimSpace(ref))
}

// planComment finds the plan comment of a ticket: the comment with the highest plan version.
func (l *Lifecycle) planComment(ctx context.Context, key string) (id string, p Plan, found bool, err error) {
	comments, err := l.Board.Svc.Comments(ctx, l.Board.Client, key)
	if err != nil {
		return "", Plan{}, false, err
	}
	for _, c := range comments {
		if cand, ok := parsePlanHTML(c.CommentHTML); ok && (!found || cand.Version >= p.Version) {
			id, p, found = c.ID, cand, true
		}
	}
	return id, p, found, nil
}

// UploadPlan stores the plan of a ticket. The newest version is always kept on disk first, so it survives
// Plane being down; in Plane one comment holds the plan and is edited in place, its version going up.
func (l *Lifecycle) UploadPlan(ctx context.Context, ref, markdown string) (Plan, error) {
	if strings.TrimSpace(markdown) == "" {
		return Plan{}, errors.New("a plan needs text")
	}
	key := l.keyOf(ctx, ref)
	id, prev, found, err := l.planComment(ctx, key)
	if err != nil {
		return Plan{}, fmt.Errorf("read the plan comments of %s: %w", key, err)
	}
	version := 1
	if found {
		version = prev.Version + 1
	}
	p := newPlan(markdown, version)
	if err := l.Plans.Save(key, p); err != nil {
		return Plan{}, err
	}
	if found {
		err = l.Board.Svc.UpdateComment(ctx, l.Board.Client, key, id, planHTML(p))
	} else {
		// The sidecar wraps a new comment in a paragraph, so the first one is opened for it.
		err = l.Board.Svc.Comment(ctx, l.Board.Client, key, strings.TrimPrefix(planHTML(p), "<p>")+"<p>")
	}
	if err != nil {
		return Plan{}, fmt.Errorf("the plan is saved locally but not in Plane: %w", err)
	}
	p.Source = "plane"
	return p, nil
}

// GetPlan returns the plan of a ticket, preferring Plane and falling back to the local store when Plane is
// down or holds no plan. ErrNoPlan when neither has one.
func (l *Lifecycle) GetPlan(ctx context.Context, ref string) (Plan, error) {
	key := l.keyOf(ctx, ref)
	if _, p, found, err := l.planComment(ctx, key); err == nil && found {
		p.Source = "plane"
		return p, nil
	}
	return l.Plans.Load(key)
}

// Handoff is the account a stopped ticket leaves for whoever picks it up.
type Handoff struct {
	Completed  string `json:"completed"`
	Challenges string `json:"challenges"`
	Missing    string `json:"missing"`
}

// ErrHandoffIncomplete is returned when a ticket is blocked without all three handoff sections.
var ErrHandoffIncomplete = errors.New("a blocked ticket needs a handoff")

// ErrUseFinish is returned for the outcome done, which only `chronos finish` may set.
var ErrUseFinish = errors.New("done is set by `dendrite chronos finish`, which runs the quality gate and the acceptance review first")

// TransitionResult is the state a ticket was moved to.
type TransitionResult struct {
	Ticket string `json:"ticket"`
	State  string `json:"state"`
}

// Outcomes are the transitions `Transition` accepts.
var Outcomes = []string{"verify", "blocked", "cancelled", "backlog"}

// Transition ends or pauses the session's work on a ticket. blocked needs the three handoff sections (what
// is done, what got in the way, what is missing against the acceptance criteria) and keeps the claim and the
// plan for whoever resumes; verify, cancelled and backlog release the claim.
func (l *Lifecycle) Transition(ctx context.Context, ref, outcome string, h Handoff) (TransitionResult, error) {
	var to tracker.Status
	switch outcome {
	case "done":
		return TransitionResult{}, ErrUseFinish
	case "verify":
		to = tracker.StatusInReview
	case "blocked":
		to = tracker.StatusBlocked
		if missing := h.missing(); len(missing) > 0 {
			return TransitionResult{}, fmt.Errorf("%w: write %s", ErrHandoffIncomplete, strings.Join(missing, ", "))
		}
	case "cancelled":
		to = tracker.StatusCancelled
	case "backlog":
		to = tracker.StatusBacklog
	default:
		return TransitionResult{}, fmt.Errorf("unknown outcome %q (use %s)", outcome, strings.Join(Outcomes, ", "))
	}
	_, issue, err := l.Board.Status(ctx, ref)
	if err != nil {
		return TransitionResult{}, err
	}
	ticket := canonical(issue, ref)
	branch := l.Branch(l.Dir)
	claim, _ := l.Store.ClaimFor(branch)
	if outcome == "blocked" {
		if err := l.post(ctx, ticket, "handoff", h.text(), claim); err != nil {
			return TransitionResult{}, err
		}
	}
	if err := l.Board.set(ctx, ticket, to); err != nil {
		return TransitionResult{}, fmt.Errorf("set %s to %s: %w", ticket, tracker.StatusDisplayName(to), err)
	}
	if outcome != "blocked" && strings.EqualFold(claim.Ticket, ticket) {
		if err := l.Store.ReleaseClaim(branch); err != nil {
			return TransitionResult{}, err
		}
	}
	return TransitionResult{Ticket: ticket, State: tracker.StatusDisplayName(to)}, nil
}

func (h Handoff) missing() []string {
	var out []string
	for _, s := range []struct{ name, text string }{{"completed", h.Completed}, {"challenges", h.Challenges}, {"missing", h.Missing}} {
		if strings.TrimSpace(s.text) == "" {
			out = append(out, s.name)
		}
	}
	return out
}

func (h Handoff) text() string {
	return "Completed:\n" + strings.TrimSpace(h.Completed) + "\n\nChallenges:\n" + strings.TrimSpace(h.Challenges) + "\n\nMissing:\n" + strings.TrimSpace(h.Missing)
}

// Context is everything an agent needs to start on a ticket.
type Context struct {
	Ticket             string   `json:"ticket"`
	Name               string   `json:"name"`
	State              string   `json:"state"`
	Description        string   `json:"description"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
	Plan               *Plan    `json:"plan,omitempty"`
	// Files are the paths the description and the plan mention, to search the code index for.
	Files []string `json:"files"`
}

var (
	listItem = regexp.MustCompile(`(?s)<li[^>]*>(.*?)</li>`)
	htmlTags = regexp.MustCompile(`<[^>]*>`)
	acItem   = regexp.MustCompile(`^\[AC-\d+\]`)
	pathLike = regexp.MustCompile("(?:[\\w.-]+/)+[\\w.-]+\\.\\w+|\\b[\\w-]+\\.(?:go|rs|md|toml|json|ts|py)\\b")
)

// QueryContext gathers a ticket's description, acceptance criteria, plan and the files it names.
func (l *Lifecycle) QueryContext(ctx context.Context, ref string) (Context, error) {
	st, issue, err := l.Board.Status(ctx, ref)
	if err != nil {
		return Context{}, err
	}
	ticket := canonical(issue, ref)
	out := Context{Ticket: ticket, Name: issue.Name, State: tracker.StatusDisplayName(st),
		Description: issue.DescriptionStripped, AcceptanceCriteria: []string{}, Files: []string{}}
	for _, m := range listItem.FindAllStringSubmatch(issue.DescriptionHTML, -1) {
		if item := strings.TrimSpace(htmlTags.ReplaceAllString(html.UnescapeString(m[1]), "")); acItem.MatchString(item) {
			out.AcceptanceCriteria = append(out.AcceptanceCriteria, item)
		}
	}
	text := issue.DescriptionStripped
	if p, err := l.GetPlan(ctx, ticket); err == nil {
		out.Plan = &p
		text += "\n" + p.Markdown
	}
	seen := map[string]bool{}
	for _, f := range pathLike.FindAllString(text, -1) {
		if !seen[f] {
			seen[f] = true
			out.Files = append(out.Files, f)
		}
	}
	sort.Strings(out.Files)
	if len(out.Files) > 30 {
		out.Files = out.Files[:30]
	}
	return out, nil
}
