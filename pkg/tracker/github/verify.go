package github

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Labels carried by Human-in-the-Loop verification issues.
const (
	LabelToVerify = "status:to-verify"
	LabelHITL     = "hitl-validation"
)

// verificationLabels are provisioned (when missing) before verification issues are created.
var verificationLabels = map[string]LabelDefinition{
	LabelToVerify: {Name: LabelToVerify, Color: "0e8a16", Description: "Awaiting Human-in-the-Loop review sign-off"},
	LabelHITL:     {Name: LabelHITL, Color: "5319e7", Description: "Human in the loop validation gate"},
}

// VerifyState is where a ticket stands in the verification lifecycle.
type VerifyState string

const (
	VerifyPending VerifyState = "to_verify" // implemented, awaiting human sign-off
	VerifyDone    VerifyState = "done"      // signed off
	VerifyOther   VerifyState = "other"     // not (or no longer) awaiting verification
)

// VerificationTicket is a tracker ticket as seen by the verification sync.
type VerificationTicket struct {
	ID          string      `json:"id"`
	Title       string      `json:"title"`
	Summary     string      `json:"summary,omitempty"`
	SequenceID  int         `json:"sequence_id,omitempty"`
	State       VerifyState `json:"state"`
	Module      string      `json:"module,omitempty"`
	Commits     []string    `json:"commits,omitempty"` // "<short sha> <subject>", newest first
	Branch      string      `json:"branch,omitempty"`
	TrackerName string      `json:"tracker_name,omitempty"`
}

// ActionKind is the GitHub mutation a sync action performs.
type ActionKind string

const (
	ActionCreate      ActionKind = "create"       // open a verification issue
	ActionAddLabels   ActionKind = "add-labels"   // add missing verification labels
	ActionRemoveLabel ActionKind = "remove-label" // ticket left verification: drop status:to-verify
	ActionClose       ActionKind = "close"        // ticket signed off: close the issue
)

// Action is one planned GitHub change.
type Action struct {
	Kind        ActionKind `json:"kind"`
	TicketID    string     `json:"ticket_id"`
	IssueNumber int        `json:"issue_number,omitempty"`
	Title       string     `json:"title,omitempty"`
	Body        string     `json:"-"`
	Labels      []string   `json:"labels,omitempty"`
	Comment     string     `json:"comment,omitempty"`
	Reason      string     `json:"reason"`
	Error       string     `json:"error,omitempty"`
}

// PlanVerificationSync decides which GitHub changes bring issues in line with the
// tracker. It never rewrites existing issue titles or bodies, so a second run
// with unchanged inputs plans nothing.
func PlanVerificationSync(tickets []VerificationTicket, issues map[string]Issue) []Action {
	sorted := append([]VerificationTicket(nil), tickets...)
	sort.Slice(sorted, func(i, j int) bool { return lessTicketID(sorted[i].ID, sorted[j].ID) })

	var actions []Action
	for _, t := range sorted {
		iss, has := issues[t.ID]
		switch {
		case t.State == VerifyPending && !has:
			actions = append(actions, NewCreateAction(t))
		case t.State == VerifyPending && iss.Open():
			if missing := missingLabels(iss, LabelToVerify, LabelHITL); len(missing) > 0 {
				actions = append(actions, Action{
					Kind: ActionAddLabels, TicketID: t.ID, IssueNumber: iss.Number, Labels: missing,
					Reason: "verification issue is missing its labels",
				})
			}
		case t.State == VerifyDone && iss.Open() && iss.HasLabel(LabelHITL):
			actions = append(actions, Action{
				Kind: ActionClose, TicketID: t.ID, IssueNumber: iss.Number,
				Comment: fmt.Sprintf("%s is Done in %s; closing the verification issue.", t.ID, trackerName(t)),
				Reason:  "ticket was signed off in the tracker",
			})
		case t.State == VerifyOther && iss.Open() && iss.HasLabel(LabelToVerify):
			actions = append(actions, Action{
				Kind: ActionRemoveLabel, TicketID: t.ID, IssueNumber: iss.Number, Labels: []string{LabelToVerify},
				Reason: "ticket is no longer awaiting verification",
			})
		}
	}
	return actions
}

// NewCreateAction plans the verification issue of a To Verify ticket.
func NewCreateAction(t VerificationTicket) Action {
	labels := []string{LabelToVerify, LabelHITL}
	if t.Module != "" {
		labels = append(labels, "module:"+t.Module)
	}
	return Action{
		Kind: ActionCreate, TicketID: t.ID,
		Title:  fmt.Sprintf("[%s] %s", t.ID, t.Title),
		Body:   BuildVerificationBody(t),
		Labels: labels,
		Reason: "ticket is To Verify and has no verification issue",
	}
}

func trackerName(t VerificationTicket) string {
	if t.TrackerName != "" {
		return t.TrackerName
	}
	return "the tracker"
}

func missingLabels(iss Issue, labels ...string) []string {
	var missing []string
	for _, l := range labels {
		if !iss.HasLabel(l) {
			missing = append(missing, l)
		}
	}
	return missing
}

// lessTicketID orders "DEN-9" before "DEN-10": by key, then numerically.
func lessTicketID(a, b string) bool {
	ka, na := splitTicketID(a)
	kb, nb := splitTicketID(b)
	if ka != kb {
		return ka < kb
	}
	if na != nb {
		return na < nb
	}
	return a < b
}

func splitTicketID(id string) (string, int) {
	i := strings.LastIndex(id, "-")
	if i < 0 {
		return id, 0
	}
	n, _ := strconv.Atoi(id[i+1:])
	return id[:i], n
}

// BuildVerificationBody renders the body of a new verification issue.
func BuildVerificationBody(t VerificationTicket) string {
	var sb strings.Builder
	sb.WriteString("## Summary\n")
	summary := strings.TrimSpace(t.Summary)
	if summary == "" {
		summary = t.Title
	}
	sb.WriteString(summary + "\n\n")
	sb.WriteString("### How to verify\n")
	sb.WriteString("Check the acceptance criteria above against the commits below, then close this issue as **completed** to sign off ")
	sb.WriteString("(`dendrite quality hitl-sync` moves the ticket to Done), or comment with the changes needed.\n")
	if len(t.Commits) > 0 {
		sb.WriteString("\n### Commits\n")
		for _, c := range t.Commits {
			sb.WriteString("- " + c + "\n")
		}
	}
	sb.WriteString("\n---\n")
	if t.SequenceID > 0 {
		sb.WriteString(fmt.Sprintf("- **Plane Sequence ID**: #%d\n", t.SequenceID))
	}
	if len(t.Commits) > 0 {
		sb.WriteString(fmt.Sprintf("- **Git Commit**: `%s`\n", strings.Fields(t.Commits[0])[0]))
	}
	if t.Branch != "" {
		sb.WriteString(fmt.Sprintf("- **Branch**: `%s`\n", t.Branch))
	}
	return sb.String()
}

// ApplyVerificationActions performs planned actions, recording per-action errors
// instead of stopping, and returns the actions with created issue numbers filled in.
func (c *GitHubClient) ApplyVerificationActions(ctx context.Context, actions []Action) []Action {
	provisioned := make(map[string]bool)
	applied := make([]Action, 0, len(actions))
	for _, a := range actions {
		if err := c.applyAction(ctx, &a, provisioned); err != nil {
			a.Error = err.Error()
		}
		applied = append(applied, a)
	}
	return applied
}

func (c *GitHubClient) applyAction(ctx context.Context, a *Action, provisioned map[string]bool) error {
	switch a.Kind {
	case ActionCreate, ActionAddLabels:
		c.provisionVerificationLabels(ctx, a.Labels, provisioned)
	}
	switch a.Kind {
	case ActionCreate:
		args := c.repoArgs("issue", "create", "--title", a.Title, "--body", a.Body)
		for _, l := range a.Labels {
			args = append(args, "--label", l)
		}
		out, err := c.Executor.Execute(ctx, "gh", args...)
		if err != nil {
			return err
		}
		a.IssueNumber, _ = issueNumberFromURL(out)
		return nil
	case ActionAddLabels:
		_, err := c.Executor.Execute(ctx, "gh", c.repoArgs("issue", "edit", strconv.Itoa(a.IssueNumber), "--add-label", strings.Join(a.Labels, ","))...)
		return err
	case ActionRemoveLabel:
		return c.RemoveLabel(ctx, a.IssueNumber, a.Labels[0])
	case ActionClose:
		_, err := c.Executor.Execute(ctx, "gh", c.repoArgs("issue", "close", strconv.Itoa(a.IssueNumber), "--reason", "completed", "--comment", a.Comment)...)
		return err
	default:
		return fmt.Errorf("github: unknown action %q", a.Kind)
	}
}

func (c *GitHubClient) provisionVerificationLabels(ctx context.Context, labels []string, provisioned map[string]bool) {
	for _, name := range labels {
		if provisioned[name] {
			continue
		}
		provisioned[name] = true
		def, ok := verificationLabels[name]
		if !ok {
			def = LabelDefinition{Name: name, Color: "1d76db", Description: "Belongs to module " + strings.TrimPrefix(name, "module:")}
		}
		c.ensureLabel(ctx, def)
	}
}
