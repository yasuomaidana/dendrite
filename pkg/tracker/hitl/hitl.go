// Package hitl keeps Human-in-the-Loop verification in step between the Plane
// tracker and GitHub issues. Every input is discovered at run time: tickets and
// their states come from Plane, commits and the target repository from git,
// and verification issues from GitHub.
package hitl

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"Dendrite/pkg/tracker"
	"Dendrite/pkg/tracker/github"
	"Dendrite/pkg/tracker/plane"
)

// toVerifyStateName is the Plane state holding implemented work awaiting human sign-off.
const toVerifyStateName = "to verify"

// newPlaneClient is a seam over the Plane client constructor.
var newPlaneClient = plane.NewClientFromEnv

// scopeRe extracts the conventional-commit scope of "[DEN-1] feat(scope): ..." or "fix(scope)!: ...".
var scopeRe = regexp.MustCompile(`^(?:\[[^\]]+\]\s*)?[a-z]+\(([^)]+)\)!?:`)

// PlaneTracker is the subset of the Plane client the sync needs.
type PlaneTracker interface {
	ListStates(ctx context.Context) ([]plane.PlaneState, error)
	ListPlaneIssues(ctx context.Context) ([]plane.PlaneIssue, error)
	UpdateStatus(ctx context.Context, id tracker.TicketID, state tracker.Status) error
}

// Syncer reconciles Plane verification state with GitHub issues.
type Syncer struct {
	Plane   PlaneTracker
	GitHub  *github.GitHubClient
	Git     github.CommandExecutor // runs git for commit and branch discovery
	RepoDir string
	// Only restricts both syncs to these ticket IDs; empty means every ticket.
	Only []string
}

// New returns a Syncer for the git repository at repoDir.
func New(planeClient PlaneTracker, gh *github.GitHubClient, repoDir string) *Syncer {
	if repoDir == "" {
		repoDir = "."
	}
	return &Syncer{Plane: planeClient, GitHub: gh, Git: &github.DefaultCommandExecutor{}, RepoDir: repoDir}
}

// Tickets lists Plane tickets with their verification state, one per ticket ID.
func (s *Syncer) Tickets(ctx context.Context) ([]github.VerificationTicket, error) {
	tickets, _, err := s.tickets(ctx)
	return tickets, err
}

// tickets also returns the IDs shared by several Plane issues. Each ID keeps the
// issue with the lowest sequence ID so a duplicate never yields a second GitHub issue.
func (s *Syncer) tickets(ctx context.Context) ([]github.VerificationTicket, []string, error) {
	states, err := s.Plane.ListStates(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("hitl: list Plane states: %w", err)
	}
	byID := make(map[string]plane.PlaneState, len(states))
	for _, st := range states {
		byID[st.ID] = st
	}
	issues, err := s.Plane.ListPlaneIssues(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("hitl: list Plane issues: %w", err)
	}
	sort.SliceStable(issues, func(i, j int) bool { return issues[i].SequenceID < issues[j].SequenceID })
	var tickets []github.VerificationTicket
	var duplicates []string
	seen := make(map[string]bool)
	for _, iss := range issues {
		id := github.TicketIDFromTitle(iss.Name)
		if id == "" || iss.ArchivedAt != nil {
			continue
		}
		if len(s.Only) > 0 && !contains(s.Only, id) {
			continue
		}
		if seen[id] {
			duplicates = append(duplicates, fmt.Sprintf("%s (Plane #%d)", id, iss.SequenceID))
			continue
		}
		seen[id] = true
		title := strings.TrimSpace(iss.Name[strings.Index(iss.Name, "]")+1:])
		tickets = append(tickets, github.VerificationTicket{
			ID:          id,
			Title:       title,
			Summary:     strings.TrimSpace(iss.DescriptionStripped),
			SequenceID:  iss.SequenceID,
			State:       verifyState(byID[iss.State]),
			Module:      scopeOf(title),
			TrackerName: "Plane",
		})
	}
	return tickets, duplicates, nil
}

func verifyState(st plane.PlaneState) github.VerifyState {
	switch {
	case strings.EqualFold(strings.TrimSpace(st.Name), toVerifyStateName):
		return github.VerifyPending
	case st.Group == "completed":
		return github.VerifyDone
	default:
		return github.VerifyOther
	}
}

// scopeOf returns the sanitized conventional-commit scope of subject, or "".
func scopeOf(subject string) string {
	m := scopeRe.FindStringSubmatch(strings.TrimSpace(subject))
	if m == nil {
		return ""
	}
	scope := strings.TrimSpace(strings.Split(m[1], ",")[0])
	return strings.ToLower(strings.ReplaceAll(scope, " ", "-"))
}

// enrich adds the commits that reference the ticket, the branch holding the latest
// one, and (preferring the commit's scope) the module.
func (s *Syncer) enrich(ctx context.Context, t *github.VerificationTicket) {
	out, err := s.Git.Execute(ctx, "git", "-C", s.RepoDir, "log", "--all", "--fixed-strings",
		"--grep", "["+t.ID+"]", "--format=%h %s")
	if err != nil || strings.TrimSpace(out) == "" {
		return
	}
	t.Commits = strings.Split(strings.TrimSpace(out), "\n")
	latest := strings.Fields(t.Commits[0])
	if subject := strings.TrimPrefix(t.Commits[0], latest[0]+" "); scopeOf(subject) != "" {
		t.Module = scopeOf(subject)
	}
	branches, err := s.Git.Execute(ctx, "git", "-C", s.RepoDir, "branch", "--all", "--contains", latest[0], "--format=%(refname:short)")
	if err == nil {
		for _, b := range strings.Split(branches, "\n") {
			if b = strings.TrimSpace(b); b != "" && !strings.HasSuffix(b, "/HEAD") {
				t.Branch = strings.TrimPrefix(b, "origin/")
				break
			}
		}
	}
}

// GitHubSyncResult reports what github-sync did (or would do, in a dry run).
type GitHubSyncResult struct {
	Repo    string          `json:"repo"`
	DryRun  bool            `json:"dry_run"`
	Tickets int             `json:"tickets"`
	Actions []github.Action `json:"actions"`
	Pending []string        `json:"pending_verification"`
	// Duplicates are Plane issues skipped because another issue already uses their ticket ID.
	Duplicates []string `json:"duplicate_ticket_ids,omitempty"`
	Errors     []string `json:"errors,omitempty"`
}

// GitHubSync opens verification issues for To Verify tickets, labels them, closes
// issues of tickets signed off in Plane, and unlabels tickets that left verification.
func (s *Syncer) GitHubSync(ctx context.Context, dryRun bool) (*GitHubSyncResult, error) {
	tickets, duplicates, err := s.tickets(ctx)
	if err != nil {
		return nil, err
	}
	issues, err := s.GitHub.ListTicketIssues(ctx)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]github.VerificationTicket, len(tickets))
	for _, t := range tickets {
		byID[t.ID] = t
	}
	actions := github.PlanVerificationSync(tickets, issues)
	for i := range actions {
		if actions[i].Kind != github.ActionCreate {
			continue
		}
		// Only new issues need commits, branch and module, so git runs just for them.
		t := byID[actions[i].TicketID]
		s.enrich(ctx, &t)
		actions[i] = github.NewCreateAction(t)
	}
	res := &GitHubSyncResult{Repo: s.GitHub.Repo, DryRun: dryRun, Tickets: len(tickets), Actions: actions, Duplicates: duplicates}
	for _, t := range tickets {
		if t.State == github.VerifyPending {
			res.Pending = append(res.Pending, t.ID)
		}
	}
	if !dryRun {
		res.Actions = s.GitHub.ApplyVerificationActions(ctx, actions)
		for _, a := range res.Actions {
			if a.Error != "" {
				res.Errors = append(res.Errors, fmt.Sprintf("%s %s: %s", a.Kind, a.TicketID, a.Error))
			}
		}
	}
	return res, nil
}

// Outcome classifies a To Verify ticket after polling its verification issue.
type Outcome string

const (
	OutcomeApproved Outcome = "approved" // issue closed as completed: ticket moved to Done
	OutcomeRejected Outcome = "rejected" // issue closed as not planned: ticket left in To Verify
	OutcomeFeedback Outcome = "feedback" // open issue with reviewer comments
	OutcomeWaiting  Outcome = "waiting"  // open issue, no comments yet
	OutcomeNoIssue  Outcome = "no-issue" // run github-sync to open one
)

// HITLResult is the verification status of one To Verify ticket.
type HITLResult struct {
	TicketID    string           `json:"ticket_id"`
	Title       string           `json:"title"`
	IssueNumber int              `json:"issue_number,omitempty"`
	Outcome     Outcome          `json:"outcome"`
	Feedback    []github.Comment `json:"feedback,omitempty"`
	Applied     bool             `json:"applied"`
	Error       string           `json:"error,omitempty"`
}

// HITLSync polls the verification issue of every To Verify ticket. Issues closed as
// completed move their ticket to Done in Plane (unless dryRun); reviewer comments on
// open issues are reported for follow-up.
func (s *Syncer) HITLSync(ctx context.Context, dryRun bool) ([]HITLResult, error) {
	tickets, err := s.Tickets(ctx)
	if err != nil {
		return nil, err
	}
	issues, err := s.GitHub.ListTicketIssues(ctx)
	if err != nil {
		return nil, err
	}
	var results []HITLResult
	for _, t := range tickets {
		if t.State != github.VerifyPending {
			continue
		}
		r := HITLResult{TicketID: t.ID, Title: t.Title, Outcome: OutcomeNoIssue}
		if iss, ok := issues[t.ID]; ok {
			r.IssueNumber = iss.Number
			s.pollIssue(ctx, iss, &r, dryRun)
		}
		results = append(results, r)
	}
	return results, nil
}

func (s *Syncer) pollIssue(ctx context.Context, iss github.Issue, r *HITLResult, dryRun bool) {
	if !iss.Open() {
		if iss.StateReason == "NOT_PLANNED" {
			r.Outcome = OutcomeRejected
			return
		}
		r.Outcome = OutcomeApproved
		if dryRun {
			return
		}
		if err := s.Plane.UpdateStatus(ctx, tracker.TicketID(r.TicketID), tracker.StatusDone); err != nil {
			r.Error = err.Error()
			return
		}
		r.Applied = true
		return
	}
	comments, err := s.GitHub.IssueComments(ctx, iss.Number)
	if err != nil {
		r.Outcome, r.Error = OutcomeWaiting, err.Error()
		return
	}
	r.Outcome = OutcomeWaiting
	if len(comments) > 0 {
		r.Outcome, r.Feedback = OutcomeFeedback, comments
	}
}

// NewFromEnv builds a Syncer from the Plane environment configuration and the GitHub
// repository repo, detected from the origin remote of repoDir when empty.
func NewFromEnv(ctx context.Context, repo, repoDir string) (*Syncer, error) {
	planeClient, err := newPlaneClient()
	if err != nil {
		return nil, fmt.Errorf("hitl: Plane is not configured: %w", err)
	}
	if repo == "" {
		if repo, err = github.DetectRepo(ctx, &github.DefaultCommandExecutor{}, repoDir); err != nil {
			return nil, err
		}
	}
	return New(planeClient, github.NewGitHubClient(repo), repoDir), nil
}

func contains(ids []string, id string) bool {
	for _, x := range ids {
		if strings.EqualFold(strings.TrimSpace(x), id) {
			return true
		}
	}
	return false
}
