package github

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func TestParseRepoURL(t *testing.T) {
	cases := map[string]string{
		"https://github.com/acme/widgets.git":      "acme/widgets",
		"https://github.com/acme/widgets":          "acme/widgets",
		"git@github.com:acme/widgets.git":          "acme/widgets",
		"ssh://git@github.com/acme/widgets.git/\n": "acme/widgets",
	}
	for in, want := range cases {
		if got, ok := ParseRepoURL(in); !ok || got != want {
			t.Errorf("ParseRepoURL(%q) = %q, %v", in, got, ok)
		}
	}
	if _, ok := ParseRepoURL("https://gitlab.com/acme/widgets.git"); ok {
		t.Error("non-GitHub remotes must not parse")
	}
}

func TestDetectRepo(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	exec := &DefaultCommandExecutor{}
	run("init", "-q")
	if _, err := DetectRepo(ctx, exec, dir); err == nil || !strings.Contains(err.Error(), "--repo") {
		t.Errorf("missing origin: %v", err)
	}
	run("remote", "add", "origin", "https://gitlab.com/a/b.git")
	if _, err := DetectRepo(ctx, exec, dir); err == nil || !strings.Contains(err.Error(), "not a GitHub repository") {
		t.Errorf("non-GitHub origin: %v", err)
	}
	run("remote", "set-url", "origin", "git@github.com:acme/widgets.git")
	if repo, err := DetectRepo(ctx, exec, dir); err != nil || repo != "acme/widgets" {
		t.Errorf("DetectRepo = %q, %v", repo, err)
	}

	script := newTestScriptExecutor()
	script.calls["git -C . remote get-url origin"] = "https://github.com/o/r"
	if repo, err := DetectRepo(ctx, script, ""); err != nil || repo != "o/r" {
		t.Errorf("default dir: %q, %v", repo, err)
	}
}

func TestTicketIDFromTitle(t *testing.T) {
	cases := map[string]string{
		"[DEN-1014] feat: x":     "DEN-1014",
		"  [DMCP-7] hub":         "DMCP-7",
		"[DEN-703-COV-1] gap":    "",
		"DEN-1 without brackets": "",
		"Fix [DEN-2] later":      "",
	}
	for in, want := range cases {
		if got := TicketIDFromTitle(in); got != want {
			t.Errorf("TicketIDFromTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

const listCmd = "gh issue list --state all --limit 5000 --json number,title,state,stateReason,labels,comments --repo o/r"

func TestListTicketIssues(t *testing.T) {
	script := newTestScriptExecutor()
	script.calls[listCmd] = `[
		{"number":3,"title":"[DEN-1] old","state":"CLOSED","stateReason":"COMPLETED","labels":[]},
		{"number":9,"title":"[DEN-1] newer closed","state":"CLOSED","stateReason":"NOT_PLANNED","labels":[]},
		{"number":7,"title":"[DEN-2] open dup","state":"OPEN","labels":[]},
		{"number":5,"title":"[DEN-2] open lower","state":"OPEN","labels":[{"name":"hitl-validation"}]},
		{"number":8,"title":"[DEN-2] closed","state":"CLOSED","labels":[]},
		{"number":4,"title":"[DEN-3] closed first","state":"CLOSED","labels":[]},
		{"number":6,"title":"[DEN-3] open later","state":"OPEN","labels":[]},
		{"number":1,"title":"Not a ticket","state":"OPEN","labels":[]}
	]`
	c := NewGitHubClient("o/r").WithExecutor(script)
	issues, err := c.ListTicketIssues(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 3 {
		t.Fatalf("issues = %+v", issues)
	}
	if issues["DEN-1"].Number != 9 || issues["DEN-2"].Number != 5 || issues["DEN-3"].Number != 6 {
		t.Errorf("preference wrong: %+v", issues)
	}
	if !issues["DEN-2"].HasLabel(LabelHITL) || issues["DEN-2"].TicketID != "DEN-2" {
		t.Errorf("DEN-2 = %+v", issues["DEN-2"])
	}

	script.calls[listCmd] = "not json"
	if _, err := c.ListTicketIssues(context.Background()); err == nil {
		t.Error("expected parse error")
	}
	script.failCmd = "issue list"
	if _, err := c.ListTicketIssues(context.Background()); err == nil {
		t.Error("expected list error")
	}

	empty := NewGitHubClient("o/r").WithExecutor(newTestScriptExecutor())
	if issues, err := empty.ListTicketIssues(context.Background()); err != nil || len(issues) != 0 {
		t.Errorf("empty output: %v, %v", issues, err)
	}
}

func TestFindTicketIssueEdgeCases(t *testing.T) {
	script := newTestScriptExecutor()
	c := NewGitHubClient("").WithExecutor(script)
	ctx := context.Background()
	search := `gh issue list --state all --search "[DEN-5]" in:title --json number,title --limit 50`
	if _, ok := c.findTicketIssue(ctx, "DEN-5"); ok {
		t.Error("empty output must not match")
	}
	script.calls[search] = "garbage"
	if _, ok := c.findTicketIssue(ctx, "DEN-5"); ok {
		t.Error("bad json must not match")
	}
	script.calls[search] = `[{"number":1,"title":"[DEN-50] near miss"}]`
	if _, ok := c.findTicketIssue(ctx, "DEN-5"); ok {
		t.Error("near-miss IDs must not match")
	}
}

func TestIssueComments(t *testing.T) {
	script := newTestScriptExecutor()
	c := NewGitHubClient("o/r").WithExecutor(script)
	ctx := context.Background()
	view := "gh issue view 4 --json comments --repo o/r"
	script.calls[view] = `{"comments":[{"author":{"login":"rev"},"body":"looks good","createdAt":"2026-10-18T00:00:00Z"}]}`
	comments, err := c.IssueComments(ctx, 4)
	if err != nil || len(comments) != 1 || comments[0] != (Comment{Author: "rev", Body: "looks good", CreatedAt: "2026-10-18T00:00:00Z"}) {
		t.Fatalf("comments = %+v, %v", comments, err)
	}
	script.calls[view] = "{"
	if _, err := c.IssueComments(ctx, 4); err == nil {
		t.Error("expected parse error")
	}
	script.failCmd = "issue view"
	if _, err := c.IssueComments(ctx, 4); err == nil {
		t.Error("expected command error")
	}
}

func verifyFixture() ([]VerificationTicket, map[string]Issue) {
	full := []string{LabelToVerify, LabelHITL}
	tickets := []VerificationTicket{
		{ID: "DEN-10", Title: "ten", State: VerifyPending, Module: "cli"},
		{ID: "DEN-9", Title: "nine", State: VerifyPending},
		{ID: "DEN-2", Title: "two", State: VerifyPending},
		{ID: "DEN-3", Title: "three", State: VerifyDone, TrackerName: "Plane"},
		{ID: "DEN-4", Title: "four", State: VerifyOther},
		{ID: "DEN-5", Title: "five", State: VerifyPending},
		{ID: "DEN-6", Title: "six", State: VerifyDone},
		{ID: "DEN-7", Title: "seven", State: VerifyPending},
		{ID: "DMCP-1", Title: "hub", State: VerifyOther},
	}
	issues := map[string]Issue{
		"DEN-2": {Number: 2, State: "OPEN", Labels: []string{LabelToVerify}},
		"DEN-3": {Number: 3, State: "OPEN", Labels: full},
		"DEN-4": {Number: 4, State: "OPEN", Labels: full},
		"DEN-5": {Number: 5, State: "OPEN", Labels: full},
		"DEN-6": {Number: 6, State: "OPEN"}, // no HITL label: not a verification issue
		"DEN-7": {Number: 7, State: "CLOSED", Labels: full},
	}
	return tickets, issues
}

func TestPlanVerificationSync(t *testing.T) {
	tickets, issues := verifyFixture()
	actions := PlanVerificationSync(tickets, issues)
	var got []string
	for _, a := range actions {
		got = append(got, string(a.Kind)+":"+a.TicketID)
	}
	want := "add-labels:DEN-2,close:DEN-3,remove-label:DEN-4,create:DEN-9,create:DEN-10"
	if strings.Join(got, ",") != want {
		t.Fatalf("actions = %s\nwant      %s", strings.Join(got, ","), want)
	}
	if strings.Join(actions[0].Labels, ",") != LabelHITL {
		t.Errorf("only the missing label is added: %v", actions[0].Labels)
	}
	if !strings.Contains(actions[1].Comment, "Done in Plane") {
		t.Errorf("close comment = %q", actions[1].Comment)
	}
	if actions[4].Title != "[DEN-10] ten" || strings.Join(actions[4].Labels, ",") != "status:to-verify,hitl-validation,module:cli" {
		t.Errorf("create = %+v", actions[4])
	}

	// Once applied, a second run plans nothing.
	issues["DEN-2"] = Issue{Number: 2, State: "OPEN", Labels: []string{LabelToVerify, LabelHITL}}
	issues["DEN-3"] = Issue{Number: 3, State: "CLOSED", Labels: []string{LabelToVerify, LabelHITL}}
	issues["DEN-4"] = Issue{Number: 4, State: "OPEN", Labels: []string{LabelHITL}}
	issues["DEN-9"] = Issue{Number: 90, State: "OPEN", Labels: []string{LabelToVerify, LabelHITL}}
	issues["DEN-10"] = Issue{Number: 100, State: "OPEN", Labels: []string{LabelToVerify, LabelHITL, "module:cli"}}
	if again := PlanVerificationSync(tickets, issues); len(again) != 0 {
		t.Errorf("second run must be a no-op, got %+v", again)
	}

	if a := PlanVerificationSync([]VerificationTicket{{ID: "X-1", State: VerifyDone}}, map[string]Issue{"X-1": {State: "OPEN", Labels: []string{LabelHITL}}}); !strings.Contains(a[0].Comment, "the tracker") {
		t.Errorf("default tracker name: %+v", a)
	}
}

func TestLessTicketID(t *testing.T) {
	if !lessTicketID("DEN-9", "DEN-10") || lessTicketID("DEN-10", "DEN-9") {
		t.Error("numeric order")
	}
	if !lessTicketID("DCORE-5", "DEN-1") {
		t.Error("key order")
	}
	if !lessTicketID("A", "B") || !lessTicketID("DEN-01", "DEN-1") {
		t.Error("fallback order")
	}
}

func TestBuildVerificationBody(t *testing.T) {
	body := BuildVerificationBody(VerificationTicket{
		ID: "DEN-1", Title: "t", Summary: "  The summary  ", SequenceID: 12,
		Commits: []string{"abc123 [DEN-1] feat: x", "def456 [DEN-1] fix: y"}, Branch: "feat/x",
	})
	for _, want := range []string{"## Summary\nThe summary\n", "### How to verify", "- abc123 [DEN-1] feat: x\n- def456", "**Plane Sequence ID**: #12", "**Git Commit**: `abc123`", "**Branch**: `feat/x`"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q:\n%s", want, body)
		}
	}
	bare := BuildVerificationBody(VerificationTicket{ID: "DEN-2", Title: "Only a title"})
	if !strings.Contains(bare, "## Summary\nOnly a title") || strings.Contains(bare, "Commits") || strings.Contains(bare, "Sequence") || strings.Contains(bare, "Branch") {
		t.Errorf("bare body:\n%s", bare)
	}
}

func TestApplyVerificationActions(t *testing.T) {
	script := newTestScriptExecutor()
	c := NewGitHubClient("o/r").WithExecutor(script)
	actions := []Action{
		{Kind: ActionCreate, TicketID: "DEN-1", Title: "[DEN-1] a", Body: "b", Labels: []string{LabelToVerify, LabelHITL, "module:cli"}},
		{Kind: ActionAddLabels, TicketID: "DEN-2", IssueNumber: 2, Labels: []string{LabelHITL}},
		{Kind: ActionRemoveLabel, TicketID: "DEN-3", IssueNumber: 3, Labels: []string{LabelToVerify}},
		{Kind: ActionClose, TicketID: "DEN-4", IssueNumber: 4, Comment: "done"},
		{Kind: "bogus", TicketID: "DEN-5"},
	}
	applied := c.ApplyVerificationActions(context.Background(), actions)
	if applied[0].IssueNumber != 42 {
		t.Errorf("created number = %d", applied[0].IssueNumber)
	}
	if applied[4].Error == "" {
		t.Error("unknown action must report an error")
	}
	history := strings.Join(script.history, "\n")
	for _, want := range []string{
		"gh label create status:to-verify --color 0e8a16",
		"gh label create module:cli --color 1d76db --description Belongs to module cli --repo o/r",
		"gh issue create --title [DEN-1] a --body b --repo o/r --label status:to-verify --label hitl-validation --label module:cli",
		"gh issue edit 2 --add-label hitl-validation --repo o/r",
		"gh issue edit 3 --remove-label status:to-verify --repo o/r",
		"gh issue close 4 --reason completed --comment done --repo o/r",
	} {
		if !strings.Contains(history, want) {
			t.Errorf("missing command %q in\n%s", want, history)
		}
	}
	if n := strings.Count(history, "gh label create hitl-validation"); n != 1 {
		t.Errorf("labels must be provisioned once per run, got %d", n)
	}
	if strings.Contains(history, "--force") {
		t.Error("existing labels must never be overwritten with --force")
	}

	failing := newTestScriptExecutor()
	failing.createErr = errors.New("quota")
	failing.failCmd = "issue edit"
	out := NewGitHubClient("o/r").WithExecutor(failing).ApplyVerificationActions(context.Background(), actions[:2])
	if out[0].Error == "" || out[1].Error == "" {
		t.Errorf("errors must be recorded per action: %+v", out)
	}
}

func TestListTicketIssuesCarriesCommentsOnlyWhenTheListingDoes(t *testing.T) {
	script := newTestScriptExecutor()
	script.calls[listCmd] = `[
		{"number":1,"title":"[DEN-1] with comments","state":"OPEN","labels":[],"comments":[{"author":{"login":"rev"},"body":"looks wrong","createdAt":"2026-10-28T10:00:00Z"}]},
		{"number":2,"title":"[DEN-2] none yet","state":"OPEN","labels":[],"comments":[]},
		{"number":3,"title":"[DEN-3] field absent","state":"OPEN","labels":[]}
	]`
	issues, err := NewGitHubClient("o/r").WithExecutor(script).ListTicketIssues(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if i := issues["DEN-1"]; !i.CommentsKnown || len(i.Comments) != 1 || i.Comments[0].Author != "rev" || i.Comments[0].Body != "looks wrong" {
		t.Errorf("DEN-1 = %+v", i)
	}
	if i := issues["DEN-2"]; !i.CommentsKnown || len(i.Comments) != 0 {
		t.Errorf("DEN-2 = %+v: an empty list is known to be empty", i)
	}
	if i := issues["DEN-3"]; i.CommentsKnown {
		t.Errorf("DEN-3 = %+v: a missing field must not read as 'no comments'", i)
	}
}
