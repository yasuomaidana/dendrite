package hitl

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"Dendrite/pkg/tracker/github"
	"Dendrite/pkg/tracker/hitl/hitltest"
	"Dendrite/pkg/tracker/plane"
)

func planeIssue(uuid, name, state string) plane.PlaneIssue {
	return plane.PlaneIssue{ID: uuid, Name: name, State: state, SequenceID: len(uuid), DescriptionStripped: "Spec of " + name}
}

// fixture: one ticket per sync rule, plus entries the sync must ignore.
func fixture(t *testing.T) (*hitltest.PlaneServer, *hitltest.GHScript, *Syncer) {
	t.Helper()
	archived := time.Now()
	gone := planeIssue("u-arch", "[DEN-9] archived", "st-verify")
	gone.ArchivedAt = &archived
	ps := hitltest.NewPlaneServer(
		planeIssue("u-1", "[DEN-1] feat(work): new copies", "st-verify"),
		planeIssue("u-2", "[DEN-2] fix(cli): labels", "st-verify"),
		planeIssue("u-3", "[DEN-3] signed off", "st-done"),
		planeIssue("u-4", "[DEN-4] reopened work", "st-progress"),
		planeIssue("u-5", "[DEN-5] in sync", "st-verify"),
		planeIssue("u-6", "[DEN-6] approved", "st-verify"),
		planeIssue("u-7", "[DEN-7] rejected", "st-verify"),
		planeIssue("u-x", "No ticket id", "st-verify"),
		gone,
		// Same ID as DEN-1 (Plane really has such duplicates): the lower sequence ID wins.
		plane.PlaneIssue{ID: "u-dup", Name: "[DEN-1] duplicate entry", State: "st-progress", SequenceID: 99},
	)
	t.Cleanup(ps.Close)

	gh := &hitltest.GHScript{}
	full := []string{github.LabelToVerify, github.LabelHITL}
	gh.On("gh issue list", hitltest.IssueListJSON(
		hitltest.IssueRow{Number: 12, Title: "[DEN-2] labels", State: "OPEN", Labels: []string{github.LabelToVerify}},
		hitltest.IssueRow{Number: 13, Title: "[DEN-3] signed off", State: "OPEN", Labels: full},
		hitltest.IssueRow{Number: 14, Title: "[DEN-4] reopened", State: "OPEN", Labels: full},
		hitltest.IssueRow{Number: 15, Title: "[DEN-5] in sync", State: "OPEN", Labels: full},
		hitltest.IssueRow{Number: 16, Title: "[DEN-6] approved", State: "CLOSED", StateReason: "COMPLETED", Labels: full},
		hitltest.IssueRow{Number: 17, Title: "[DEN-7] rejected", State: "CLOSED", StateReason: "NOT_PLANNED", Labels: full},
		hitltest.IssueRow{Number: 18, Title: "Unrelated issue", State: "OPEN"},
	), nil)
	gh.On("gh issue create", "https://github.com/o/r/issues/77", nil)
	gh.On("git -C repo log", "abc1234 [DEN-1] feat(workcopy): add copies\nold9999 [DEN-1] docs: notes", nil)
	gh.On("git -C repo branch", "origin/HEAD\norigin/feat/copies\nmain", nil)

	s := New(ps.Client(), github.NewGitHubClient("o/r").WithExecutor(gh), "repo")
	s.Git = gh
	return ps, gh, s
}

func kinds(actions []github.Action) map[string]github.ActionKind {
	m := make(map[string]github.ActionKind)
	for _, a := range actions {
		m[a.TicketID] = a.Kind
	}
	return m
}

func TestTicketsReadsPlaneStates(t *testing.T) {
	_, _, s := fixture(t)
	tickets, err := s.Tickets(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tickets) != 7 {
		t.Fatalf("want 7 tickets (no id and archived skipped), got %d: %+v", len(tickets), tickets)
	}
	first := tickets[0]
	if first.ID != "DEN-1" || first.Title != "feat(work): new copies" || first.State != github.VerifyPending || first.Module != "work" {
		t.Errorf("ticket 1 = %+v", first)
	}
	states := map[string]github.VerifyState{}
	for _, tk := range tickets {
		states[tk.ID] = tk.State
	}
	if states["DEN-3"] != github.VerifyDone || states["DEN-4"] != github.VerifyOther {
		t.Errorf("states = %v", states)
	}
}

func TestGitHubSyncDryRunPlansWithoutWriting(t *testing.T) {
	_, gh, s := fixture(t)
	res, err := s.GitHubSync(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]github.ActionKind{
		"DEN-1": github.ActionCreate,
		"DEN-2": github.ActionAddLabels,
		"DEN-3": github.ActionClose,
		"DEN-4": github.ActionRemoveLabel,
	}
	if got := kinds(res.Actions); len(got) != len(want) {
		t.Fatalf("actions = %v", got)
	} else {
		for id, k := range want {
			if got[id] != k {
				t.Errorf("%s: %s, want %s", id, got[id], k)
			}
		}
	}
	create := res.Actions[0]
	for _, want := range []string{"Spec of [DEN-1]", "abc1234 [DEN-1] feat(workcopy)", "**Git Commit**: `abc1234`", "**Branch**: `feat/copies`", "**Plane Sequence ID**: #3"} {
		if !strings.Contains(create.Body, want) {
			t.Errorf("create body missing %q:\n%s", want, create.Body)
		}
	}
	if strings.Join(create.Labels, ",") != "status:to-verify,hitl-validation,module:workcopy" {
		t.Errorf("labels = %v (commit scope must win over the title scope)", create.Labels)
	}
	if len(res.Duplicates) != 1 || res.Duplicates[0] != "DEN-1 (Plane #99)" {
		t.Errorf("duplicates = %v", res.Duplicates)
	}
	if res.Repo != "o/r" || !res.DryRun || res.Tickets != 7 || len(res.Pending) != 5 {
		t.Errorf("result = %+v", res)
	}
	for _, mutating := range []string{"gh issue create", "gh issue edit", "gh issue close", "gh label create"} {
		if n := gh.Called(mutating); n != 0 {
			t.Errorf("dry run ran %q %d times", mutating, n)
		}
	}
}

func TestGitHubSyncAppliesAndReportsErrors(t *testing.T) {
	_, gh, s := fixture(t)
	gh.On("gh issue close", "", errors.New("closed already"))
	res, err := s.GitHubSync(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Actions[0].IssueNumber != 77 {
		t.Errorf("created issue number = %d", res.Actions[0].IssueNumber)
	}
	if len(res.Errors) != 1 || !strings.Contains(res.Errors[0], "close DEN-3") {
		t.Errorf("errors = %v", res.Errors)
	}
	if gh.Called("gh issue create") != 1 || gh.Called("gh issue edit 12") != 1 || gh.Called("gh issue edit 14") != 1 {
		t.Errorf("calls = %v", gh.Calls)
	}
}

func TestEnrichWithoutCommits(t *testing.T) {
	_, gh, s := fixture(t)
	ctx := context.Background()

	gh.On("git -C repo log", "", errors.New("not a repository"))
	tk := github.VerificationTicket{ID: "DEN-1", Module: "work"}
	s.enrich(ctx, &tk)
	if tk.Commits != nil || tk.Module != "work" {
		t.Errorf("git failure must leave the ticket untouched: %+v", tk)
	}

	gh.On("git -C repo log", "def5678 [DEN-1] no scope here", nil)
	gh.On("git -C repo branch", "", errors.New("detached"))
	s.enrich(ctx, &tk)
	if len(tk.Commits) != 1 || tk.Module != "work" || tk.Branch != "" {
		t.Errorf("scopeless commit: %+v", tk)
	}
}

func TestHITLSyncOutcomes(t *testing.T) {
	ps, gh, s := fixture(t)
	gh.On("gh issue view 12", `{"comments":[{"author":{"login":"rev"},"body":"Please rename","createdAt":"2026-10-18T00:00:00Z"}]}`, nil)
	gh.On("gh issue view 15", `{"comments":[]}`, nil)
	ctx := context.Background()

	dry, err := s.HITLSync(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	outcomes := map[string]Outcome{}
	for _, r := range dry {
		outcomes[r.TicketID] = r.Outcome
	}
	want := map[string]Outcome{
		"DEN-1": OutcomeNoIssue, "DEN-2": OutcomeFeedback, "DEN-5": OutcomeWaiting,
		"DEN-6": OutcomeApproved, "DEN-7": OutcomeRejected,
	}
	if len(outcomes) != len(want) {
		t.Fatalf("outcomes = %v", outcomes)
	}
	for id, o := range want {
		if outcomes[id] != o {
			t.Errorf("%s: %s, want %s", id, outcomes[id], o)
		}
	}
	if len(ps.Patches) != 0 {
		t.Fatalf("dry run patched Plane: %v", ps.Patches)
	}

	results, err := s.HITLSync(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if ps.Patches["u-6"] != "st-done" || len(ps.Patches) != 1 {
		t.Errorf("approved ticket must move to Done, patches = %v", ps.Patches)
	}
	for _, r := range results {
		if r.TicketID == "DEN-6" && !r.Applied {
			t.Errorf("DEN-6 not applied: %+v", r)
		}
		if r.TicketID == "DEN-2" && (len(r.Feedback) != 1 || r.Feedback[0].Author != "rev") {
			t.Errorf("DEN-2 feedback = %+v", r.Feedback)
		}
	}
}

func TestHITLSyncErrors(t *testing.T) {
	ps, gh, s := fixture(t)
	ctx := context.Background()

	gh.On("gh issue view", "", errors.New("rate limited"))
	ps.FailPatch = true
	results, err := s.HITLSync(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range results {
		switch r.TicketID {
		case "DEN-2", "DEN-5":
			if r.Outcome != OutcomeWaiting || !strings.Contains(r.Error, "rate limited") {
				t.Errorf("%s: %+v", r.TicketID, r)
			}
		case "DEN-6":
			if r.Applied || r.Error == "" {
				t.Errorf("failed Plane update must be reported: %+v", r)
			}
		}
	}

	gh.On("gh issue list", "", errors.New("offline"))
	if _, err := s.HITLSync(ctx, true); err == nil {
		t.Error("HITLSync must surface GitHub list failures")
	}
	if _, err := s.GitHubSync(ctx, true); err == nil {
		t.Error("GitHubSync must surface GitHub list failures")
	}

	ps.FailIssues = true
	if _, err := s.HITLSync(ctx, true); err == nil || !strings.Contains(err.Error(), "list Plane issues") {
		t.Errorf("Plane issue failure: %v", err)
	}
	if _, err := s.GitHubSync(ctx, true); err == nil {
		t.Error("GitHubSync must surface Plane failures")
	}
	ps.Close()
	if _, err := s.Tickets(ctx); err == nil || !strings.Contains(err.Error(), "list Plane states") {
		t.Errorf("Plane state failure: %v", err)
	}
}

func TestScopeOf(t *testing.T) {
	cases := map[string]string{
		"[DEN-1] feat(work): x":        "work",
		"fix(CLI Tools, docs)!: y":     "cli-tools",
		"docs: no scope":               "",
		"refactor(agentjobs): z":       "agentjobs",
		"[DEN-2] Plain title (parens)": "",
	}
	for in, want := range cases {
		if got := scopeOf(in); got != want {
			t.Errorf("scopeOf(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNewDefaultsAndNewFromEnv(t *testing.T) {
	if s := New(nil, nil, ""); s.RepoDir != "." || s.Git == nil {
		t.Errorf("defaults: %+v", s)
	}

	ctx := context.Background()
	repo := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"remote", "add", "origin", "git@github.com:acme/widgets.git"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	s, err := NewFromEnv(ctx, "", repo)
	if err != nil || s.GitHub.Repo != "acme/widgets" || s.RepoDir != repo {
		t.Fatalf("detected repo: %+v %v", s, err)
	}
	if s, err := NewFromEnv(ctx, "o/explicit", repo); err != nil || s.GitHub.Repo != "o/explicit" {
		t.Fatalf("explicit repo: %v", err)
	}
	if _, err := NewFromEnv(ctx, "", filepath.Join(repo, "missing")); err == nil {
		t.Error("expected repo detection failure outside a git repository")
	}

	prev := newPlaneClient
	newPlaneClient = func() (*plane.Client, error) { return nil, errors.New("no plane") }
	defer func() { newPlaneClient = prev }()
	if _, err := NewFromEnv(ctx, "o/r", repo); err == nil || !strings.Contains(err.Error(), "Plane is not configured") {
		t.Errorf("plane failure: %v", err)
	}
	_ = os.Remove(repo)
}

func TestOnlyRestrictsTickets(t *testing.T) {
	_, gh, s := fixture(t)
	s.Only = []string{" den-3 ", "DEN-4"}
	res, err := s.GitHubSync(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if got := kinds(res.Actions); len(got) != 2 || got["DEN-3"] != github.ActionClose || got["DEN-4"] != github.ActionRemoveLabel {
		t.Errorf("filtered actions = %v", got)
	}
	if gh.Called("git") != 0 {
		t.Error("filtered sync must not enrich other tickets")
	}
}
