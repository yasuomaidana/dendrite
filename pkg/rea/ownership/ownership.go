package ownership

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"Dendrite/pkg/agentjobs"
	"Dendrite/pkg/directive"
	"Dendrite/pkg/ticketlock"
)

// OwnerLabel marks a ticket AGY took over from a Claude session; a Claude session may not claim it again.
const OwnerLabel = ticketlock.OwnerAGYLabel

// Errors Transfer returns when the rule does not apply.
var (
	ErrDisabled = errors.New("the [rea] directive keeps refused tickets with their session (rea.refusal_owner = none)")
	ErrNotREA   = errors.New("not an REA ticket (see rea.labels and rea.title_prefixes in the directives)")
)

// Issue is what Transfer needs to know about a ticket.
type Issue struct {
	Ticket      string
	Title       string
	Labels      []string
	Description string
	// Estimate is the ticket's estimate in points; 0 when it has none.
	Estimate int
}

// Ports are the effects of a transfer, so the rule can be exercised without Plane or a real agent.
type Ports struct {
	Issue         func(ctx context.Context, ref string) (Issue, error)
	Comment       func(ctx context.Context, ticket, kind, body string) error
	AddLabel      func(ctx context.Context, ticket, label string) error
	ReleaseClaims func(ticket string) error
	Submit        func(spec agentjobs.SubmitSpec) (string, error)
}

// Result says what a transfer did.
type Result struct {
	Ticket string `json:"ticket"`
	// AlreadyOwned is true when AGY already owns the ticket; nothing was done.
	AlreadyOwned bool   `json:"already_owned,omitempty"`
	JobID        string `json:"job_id,omitempty"`
}

// IsREA reports whether a ticket is reverse-engineering work: it carries one of the directive's labels or its
// title (after any `[DEN-n]` id) starts with one of its prefixes.
func IsREA(title string, labels []string, r directive.Rea) bool {
	for _, l := range labels {
		for _, want := range r.Labels {
			if strings.EqualFold(l, want) {
				return true
			}
		}
	}
	t := strings.TrimSpace(title)
	if strings.HasPrefix(t, "[") { // skip a leading ticket id such as [DEN-1061]
		if end := strings.Index(t, "]"); end > 0 && strings.Contains(t[:end], "-") {
			t = strings.TrimSpace(t[end+1:])
		}
	}
	for _, p := range r.TitlePrefixes {
		if p != "" && strings.HasPrefix(strings.ToLower(t), strings.ToLower(p)) {
			return true
		}
	}
	return false
}

// HasOwner reports whether the labels say AGY owns the ticket.
func HasOwner(labels []string) bool {
	for _, l := range labels {
		if strings.EqualFold(l, OwnerLabel) {
			return true
		}
	}
	return false
}

// Transfer hands an REA ticket a Claude session refused to AGY: it records the refusal on the ticket, starts
// an AGY job in an isolated copy, labels the ticket owner:agy and releases the Claude claims. Doing it again
// for a ticket AGY already owns changes nothing.
func Transfer(ctx context.Context, p Ports, rule directive.Rea, ref string, why Refusal) (Result, error) {
	if !strings.EqualFold(rule.RefusalOwner, directive.RefusalOwnerAGY) {
		return Result{}, ErrDisabled
	}
	issue, err := p.Issue(ctx, ref)
	if err != nil {
		return Result{}, fmt.Errorf("read %s: %w", ref, err)
	}
	if !IsREA(issue.Title, issue.Labels, rule) {
		return Result{}, fmt.Errorf("%s: %w", issue.Ticket, ErrNotREA)
	}
	if HasOwner(issue.Labels) {
		return Result{Ticket: issue.Ticket, AlreadyOwned: true}, nil
	}
	if err := p.Comment(ctx, issue.Ticket, "decision", decisionNote(why)); err != nil {
		return Result{}, err
	}
	id, err := p.Submit(jobSpec(issue, rule, why))
	if err != nil {
		return Result{}, fmt.Errorf("start the AGY job for %s: %w", issue.Ticket, err)
	}
	res := Result{Ticket: issue.Ticket, JobID: id}
	if err := p.AddLabel(ctx, issue.Ticket, OwnerLabel); err != nil {
		return res, fmt.Errorf("AGY job %s started but %s is not labelled %s: %w", id, issue.Ticket, OwnerLabel, err)
	}
	if err := p.ReleaseClaims(issue.Ticket); err != nil {
		return res, fmt.Errorf("AGY job %s started but the Claude claim on %s was not released: %w", id, issue.Ticket, err)
	}
	return res, nil
}

func decisionNote(why Refusal) string {
	note := "A Claude session refused this REA ticket; per the [rea] directive AGY takes ownership."
	if why.Reason != "" {
		note += "\nSignal: " + why.Reason + "."
	}
	if why.Excerpt != "" {
		note += "\nWhat Claude said: " + why.Excerpt
	}
	return note
}

// jobSpec is the AGY job for a refused ticket. The estimate in points picks the model tier (1 to 10) unless the
// directive names a model.
func jobSpec(issue Issue, rule directive.Rea, why Refusal) agentjobs.SubmitSpec {
	complexity := min(max(issue.Estimate, 1), 10)
	var b strings.Builder
	fmt.Fprintf(&b, "You now own %s: %s\n\n", issue.Ticket, issue.Title)
	b.WriteString("A Claude session declined this reverse-engineering ticket, so it was handed to you. Work it to its acceptance criteria.\n")
	if why.Excerpt != "" {
		fmt.Fprintf(&b, "\nWhat Claude said when it declined:\n%s\n", why.Excerpt)
	}
	if d := strings.TrimSpace(issue.Description); d != "" {
		fmt.Fprintf(&b, "\n## Ticket\n%s\n", d)
	}
	return agentjobs.SubmitSpec{
		Ticket: issue.Ticket, Agent: "agy", Model: rule.RefusalModel,
		Prompt: b.String(), Complexity: complexity, Isolate: true,
	}
}
