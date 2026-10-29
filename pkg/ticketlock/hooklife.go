package ticketlock

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"Dendrite/pkg/quality/gate"
	"Dendrite/pkg/tracker"
)

// SavePlan is the plan-saved hook (after ExitPlanMode): the plan goes to the ticket the working copy has
// claimed. With no claim there is no ticket to put it on, so it is kept locally under the session and the
// message tells the agent to claim one; nothing is lost. board is only opened when there is a ticket.
func SavePlan(ctx context.Context, l *Lifecycle, board func() (Board, error), in HookInput) (string, error) {
	plan := strings.TrimSpace(in.ToolInput.Plan)
	if plan == "" {
		return "", nil
	}
	claim, claimed := l.Store.ClaimOfDir(in.Cwd)
	if !claimed {
		key := hookSessionKey(in)
		if err := l.Plans.Save(key, newPlan(plan, 1)); err != nil {
			return "", err
		}
		return fmt.Sprintf("plan kept locally (%s): no ticket is claimed here. Claim one with `dendrite chronos claim <ticket>` and push it with `dendrite chronos plan push <ticket> --file <plan>`", key), nil
	}
	b, err := board()
	if err != nil {
		return "", fmt.Errorf("plan not stored on %s: %w", claim.Ticket, err)
	}
	l.Board = b
	p, err := l.UploadPlan(ctx, claim.Ticket, plan)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("plan v%d stored on %s (%d step(s) left)", p.Version, claim.Ticket, p.PendingSteps), nil
}

// AutoHandoff is the session-end hook: a session that ends while its claimed ticket is still In Progress
// leaves the handoff it would have written, then blocks the ticket with the needs-input label so the next
// session sees where it stopped. A session that finished the ticket (To Verify, Blocked, ...) leaves nothing.
func AutoHandoff(ctx context.Context, l *Lifecycle, board func() (Board, error), in HookInput) (string, error) {
	claim, claimed := l.Store.ClaimOfDir(in.Cwd)
	if !claimed {
		return "", nil
	}
	b, err := board()
	if err != nil {
		return "", fmt.Errorf("handoff of %s not written: %w", claim.Ticket, err)
	}
	l.Board = b
	st, _, err := b.Status(ctx, claim.Ticket)
	if err != nil {
		return "", fmt.Errorf("handoff of %s not written: %w", claim.Ticket, err)
	}
	if st != tracker.StatusInProgress {
		return "", nil
	}

	h := Handoff{
		Completed:  l.completedWork(ctx, claim),
		Challenges: "The session ended before the ticket was finished; this handoff was written automatically.",
		Missing:    l.pendingWork(ctx, claim),
	}
	if _, err := l.Transition(ctx, claim.Ticket, "blocked", h); err != nil {
		return "", err
	}
	if _, _, err := b.Svc.PatchLabels(ctx, b.Client, claim.Ticket, []string{"needs-input"}, nil); err != nil {
		return "", fmt.Errorf("%s is blocked but not labelled needs-input: %w", claim.Ticket, err)
	}
	return fmt.Sprintf("%s was still In Progress: blocked with an automatic handoff (needs-input)", claim.Ticket), nil
}

// completedWork lists what the branch holds: commits not yet on develop and the uncommitted changes.
func (l *Lifecycle) completedWork(ctx context.Context, c Claim) string {
	dir := firstNonEmpty(c.Worktree, l.Dir)
	var parts []string
	for _, base := range []string{"origin/develop", "develop"} {
		if out, err := gitBytes(dir, "log", "--oneline", "--max-count=15", base+"..HEAD"); err == nil {
			if s := strings.TrimSpace(string(out)); s != "" {
				parts = append(parts, "Commits on "+c.Branch+" not on "+base+":\n"+s)
			}
			break
		}
	}
	if len(parts) == 0 { // the branch is merged: its commits are the ones that name the ticket
		if _, lines := gate.TicketCommits(dir, c.Ticket); len(lines) > 0 {
			parts = append(parts, "Commits for "+c.Ticket+" (already merged):\n"+strings.Join(lines[max(0, len(lines)-15):], "\n"))
		}
	}
	if out, err := gitBytes(dir, "diff", "--stat", "HEAD"); err == nil {
		if s := strings.TrimSpace(string(out)); s != "" {
			parts = append(parts, "Uncommitted changes:\n"+s)
		}
	}
	if p, err := l.GetPlan(ctx, c.Ticket); err == nil {
		if done := checkedSteps(p.Markdown); done != "" {
			parts = append(parts, "Plan steps done:\n"+done)
		}
	}
	if len(parts) == 0 {
		return "Nothing committed or changed on the branch yet."
	}
	return strings.Join(parts, "\n\n")
}

// pendingWork lists the unchecked plan steps, or points at the acceptance criteria when there is no plan.
func (l *Lifecycle) pendingWork(ctx context.Context, c Claim) string {
	if p, err := l.GetPlan(ctx, c.Ticket); err == nil && p.PendingSteps > 0 {
		return "Plan steps left:\n" + uncheckedSteps(p.Markdown)
	}
	return "No plan was stored: compare the work with the ticket's acceptance criteria (`dendrite chronos context " + c.Ticket + "`)."
}

func checkedSteps(markdown string) string   { return stepsMatching(markdown, true) }
func uncheckedSteps(markdown string) string { return stepsMatching(markdown, false) }

var stepLine = regexp.MustCompile(`^[-*]\s+\[([ xX])\]`)

// stepsMatching lists the checked (done) or unchecked plan steps, one trimmed line each.
func stepsMatching(markdown string, done bool) string {
	var out []string
	for _, line := range strings.Split(markdown, "\n") {
		t := strings.TrimSpace(line)
		if m := stepLine.FindStringSubmatch(t); m != nil && (m[1] != " ") == done {
			out = append(out, t)
		}
	}
	return strings.Join(out, "\n")
}
