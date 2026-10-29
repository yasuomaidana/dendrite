package ticketlock

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"Dendrite/pkg/tracker"
)

// HumanReviewLabel on a ticket keeps `finish` from setting Done: a person verifies it.
const HumanReviewLabel = "needs-human-review"

// MaxReviewDiff is how much of the branch's diff the acceptance review is shown.
const MaxReviewDiff = 60000

// GateResult is the outcome of the deterministic quality gate, the first stage of `finish`.
type GateResult struct {
	Passed bool   `json:"passed"`
	Report string `json:"report"`
}

// ACVerdict is the acceptance review's judgement of one acceptance criterion.
type ACVerdict struct {
	AC     string `json:"ac"`
	Pass   bool   `json:"pass"`
	Reason string `json:"reason,omitempty"`
}

// ReviewInput is what the acceptance review sees.
type ReviewInput struct {
	Ticket      string
	Description string
	Criteria    []string
	Diff        string
}

// FinishChecks are the two stages `finish` runs and the policy it applies to the result.
type FinishChecks struct {
	// Gate runs tests, coverage and the quality gates in dir.
	Gate func(ctx context.Context, dir string) (GateResult, error)
	// Review judges the diff against every acceptance criterion.
	Review func(ctx context.Context, in ReviewInput) ([]ACVerdict, error)
	// Diff is the work done for the ticket, as shown to Review (it is cut to MaxReviewDiff).
	Diff func(dir, ticket string) string
	// AutoDone lets a clean finish set Done; without it every clean finish waits in To Verify.
	AutoDone bool
}

// FinishResult says where `finish` left the ticket and why.
type FinishResult struct {
	Ticket   string      `json:"ticket"`
	State    string      `json:"state"`
	Gate     GateResult  `json:"gate"`
	Verdicts []ACVerdict `json:"verdicts,omitempty"`
	Notes    []string    `json:"notes,omitempty"`
}

// ErrNotInProgress is returned when a ticket that is not In Progress is finished.
var ErrNotInProgress = errors.New("only a ticket that is In Progress can be finished")

// Finish ends the session's work on a ticket. Stage 1 is the quality gate, stage 2 the review of the branch's
// diff against each [AC-n]. A failed stage blocks the ticket with the three-part handoff. A clean finish sets
// Done when the policy allows it and the ticket carries no needs-human-review label, and To Verify (with the
// report as a comment) otherwise, including when the review could not run: an unchecked ticket is never Done.
func (l *Lifecycle) Finish(ctx context.Context, ref string, c FinishChecks) (FinishResult, error) {
	st, issue, err := l.Board.Status(ctx, ref)
	if err != nil {
		return FinishResult{}, err
	}
	ticket := canonical(issue, ref)
	if st != tracker.StatusInProgress {
		return FinishResult{}, fmt.Errorf("%w: %s is %s", ErrNotInProgress, ticket, tracker.StatusDisplayName(st))
	}
	res := FinishResult{Ticket: ticket}

	gate, gErr := c.Gate(ctx, l.Dir)
	if gErr != nil {
		gate = GateResult{Report: "the quality gate could not run: " + gErr.Error()}
	}
	res.Gate = gate
	if !gate.Passed {
		return l.blockFinish(ctx, res, "The quality gate failed.\n"+gate.Report, "Make the quality gate pass, then finish again.")
	}

	qc, err := l.QueryContext(ctx, ticket)
	if err != nil {
		return FinishResult{}, err
	}
	human := !c.AutoDone
	if !c.AutoDone {
		res.Notes = append(res.Notes, "automatic Done is off for this project")
	}
	switch {
	case len(qc.AcceptanceCriteria) == 0:
		res.Notes = append(res.Notes, "the ticket lists no [AC-n] criteria, so there was nothing to review")
	default:
		verdicts, rErr := c.Review(ctx, ReviewInput{Ticket: ticket, Description: qc.Description, Criteria: qc.AcceptanceCriteria, Diff: cutDiff(c.Diff(l.Dir, ticket))})
		if rErr != nil {
			human = true
			res.Notes = append(res.Notes, "the acceptance review could not run ("+rErr.Error()+"), so a person has to verify")
			break
		}
		res.Verdicts = verdicts
		if failed := failedCriteria(qc.AcceptanceCriteria, verdicts); len(failed) > 0 {
			return l.blockFinish(ctx, res, "The acceptance review failed:\n"+strings.Join(failed, "\n"), "Acceptance criteria not met:\n"+strings.Join(failed, "\n"))
		}
	}

	if _, labels, err := l.Board.Svc.PatchLabels(ctx, l.Board.Client, ticket, nil, nil); err != nil {
		human = true
		res.Notes = append(res.Notes, "the ticket's labels could not be read ("+err.Error()+"), so a person has to verify")
	} else if hasLabel(labels, HumanReviewLabel) {
		human = true
		res.Notes = append(res.Notes, "the ticket is labelled "+HumanReviewLabel)
	}

	claim, _ := l.Store.ClaimFor(l.Branch(l.Dir))
	if err := l.post(ctx, ticket, "milestone", finishReport(res), claim); err != nil {
		return FinishResult{}, err
	}
	if human {
		out, err := l.Transition(ctx, ticket, "verify", Handoff{})
		res.State = out.State
		return res, err
	}
	if err := l.Board.set(ctx, ticket, tracker.StatusDone); err != nil {
		return FinishResult{}, fmt.Errorf("set %s to Done: %w", ticket, err)
	}
	res.State = tracker.StatusDisplayName(tracker.StatusDone)
	return res, l.Store.ReleaseClaim(l.Branch(l.Dir))
}

// blockFinish blocks the ticket with a handoff built from what the branch holds and why finishing failed.
func (l *Lifecycle) blockFinish(ctx context.Context, res FinishResult, challenges, missing string) (FinishResult, error) {
	claim, _ := l.Store.ClaimFor(l.Branch(l.Dir))
	claim.Worktree = firstNonEmpty(claim.Worktree, l.Dir)
	claim.Branch = firstNonEmpty(claim.Branch, l.Branch(l.Dir))
	h := Handoff{Completed: l.completedWork(ctx, claim), Challenges: challenges, Missing: missing + "\n" + l.pendingWork(ctx, claim)}
	out, err := l.Transition(ctx, res.Ticket, "blocked", h)
	res.State = out.State
	return res, err
}

// failedCriteria lists the criteria with no passing verdict. A verdict matches a criterion by its AC-n id.
func failedCriteria(criteria []string, verdicts []ACVerdict) []string {
	byID := map[string]ACVerdict{}
	for _, v := range verdicts {
		byID[acID(v.AC)] = v
	}
	var failed []string
	for _, c := range criteria {
		v, ok := byID[acID(acItem.FindString(c))]
		switch {
		case !ok:
			failed = append(failed, "- "+c+": the review gave no verdict")
		case !v.Pass:
			failed = append(failed, "- "+c+": "+firstNonEmpty(v.Reason, "not met"))
		}
	}
	return failed
}

// acID normalises "[AC-1]", "AC-1" and "ac-1" to "AC-1".
func acID(s string) string {
	return strings.ToUpper(strings.Trim(strings.TrimSpace(s), "[]"))
}

func hasLabel(labels []string, want string) bool {
	for _, l := range labels {
		if strings.EqualFold(l, want) {
			return true
		}
	}
	return false
}

// cutDiff keeps a diff within MaxReviewDiff by keeping the start of every file, so one large file does not hide
// the rest from the review.
func cutDiff(diff string) string {
	if len(diff) <= MaxReviewDiff {
		return diff
	}
	files := strings.Split(diff, "\ndiff --git ")
	per := max(2000, MaxReviewDiff/len(files))
	var b strings.Builder
	for i, f := range files {
		if i > 0 {
			f = "\ndiff --git " + f
		}
		if len(f) > per {
			f = f[:per] + "\n[file cut]\n"
		}
		b.WriteString(f)
	}
	out := b.String()
	if len(out) > MaxReviewDiff {
		out = out[:MaxReviewDiff]
	}
	return out + "\n[diff cut at " + fmt.Sprint(MaxReviewDiff) + " bytes]"
}

// finishReport is the comment a clean finish leaves: the gate report, one line per criterion and the notes.
func finishReport(res FinishResult) string {
	lines := []string{"Finished: the quality gate passed.", strings.TrimSpace(res.Gate.Report)}
	for _, v := range res.Verdicts {
		mark := "pass"
		if !v.Pass {
			mark = "FAIL"
		}
		lines = append(lines, fmt.Sprintf("- %s %s %s", v.AC, mark, v.Reason))
	}
	for _, n := range res.Notes {
		lines = append(lines, "Note: "+n)
	}
	return strings.Join(lines, "\n")
}
