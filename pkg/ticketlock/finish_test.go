package ticketlock

import (
	"Dendrite/pkg/directive"
	"Dendrite/pkg/fsutil/fsutiltest"
	"Dendrite/pkg/pullrequest"
	"Dendrite/pkg/rag/ragtest"
	"Dendrite/pkg/testkit"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const twoCriteria = "<p>goal</p><ul><li>[AC-1] it works</li><li>[AC-2] it is tested</li></ul>"

// finishRig is a claimed session on a ticket with two acceptance criteria and checks that pass.
type finishRig struct {
	*session
	checks FinishChecks
}

func newFinishRig(t *testing.T, state, description string) *finishRig {
	t.Helper()
	s := newSession(t, map[string]string{"DEN-377": state})
	s.board.SetDescription("DEN-377", description)
	r := &finishRig{session: s}
	r.checks = FinishChecks{
		Gate: func(context.Context, string, string) (GateResult, error) {
			return GateResult{Passed: true, Report: "tests ok"}, nil
		},
		Review: func(_ context.Context, in ReviewInput) ([]ACVerdict, error) {
			var out []ACVerdict
			for _, c := range in.Criteria {
				out = append(out, ACVerdict{AC: acItem.FindString(c), Status: StatusMet, Evidence: "a line of the diff", Reason: "seen in the diff"})
			}
			return out, nil
		},
		Diff:     func(string, string) string { return "diff --git a/x b/x" },
		AutoDone: true,
	}
	if state != "Done" && state != "To Verify" {
		if _, err := s.Claim(context.Background(), "DEN-377", ClaimOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	return r
}

func (r *finishRig) finish() (FinishResult, error) {
	return r.Finish(context.Background(), "DEN-377", r.checks)
}

func (r *finishRig) lastComment() string {
	c := r.board.Texts("DEN-377")
	return c[len(c)-1]
}

func TestFinishSetsDoneWhenEveryCheckPasses(t *testing.T) {
	r := newFinishRig(t, "To Do", twoCriteria)
	res, err := r.finish()
	if err != nil || res.State != "Done" || res.Ticket != "DEN-377" || len(res.Verdicts) != 2 {
		t.Fatalf("%+v, %v", res, err)
	}
	if got := r.board.State("DEN-377"); got != "Done" {
		t.Errorf("Plane state = %q", got)
	}
	if _, err := r.Store.ClaimFor("agent/den-377"); !errors.Is(err, ErrNoClaim) {
		t.Errorf("a finished ticket releases its claim: %v", err)
	}
	if c := r.lastComment(); !strings.HasPrefix(c, "[dendrite:milestone] Finished: the quality gate passed.") || !strings.Contains(c, "[AC-1] met: seen in the diff | evidence: a line of the diff") {
		t.Errorf("report comment = %q", c)
	}
}

func TestFinishWaitsForAPersonWhenPolicyOrLabelsSayNot(t *testing.T) {
	cases := map[string]func(r *finishRig){
		"auto-Done is off": func(r *finishRig) { r.checks.AutoDone = false },
		"human review label": func(r *finishRig) {
			if _, _, err := r.Board.Svc.PatchLabels(context.Background(), r.Board.Client, "DEN-377", []string{HumanReviewLabel}, nil); err != nil {
				t.Fatal(err)
			}
		},
		"the review cannot run": func(r *finishRig) {
			r.checks.Review = func(context.Context, ReviewInput) ([]ACVerdict, error) { return nil, errors.New("gateway down") }
		},
	}
	for name, arrange := range cases {
		t.Run(name, func(t *testing.T) {
			r := newFinishRig(t, "To Do", twoCriteria)
			arrange(r)
			res, err := r.finish()
			if err != nil || res.State != "To Verify" || r.board.State("DEN-377") != "To Verify" {
				t.Fatalf("%+v, %v, Plane %q", res, err, r.board.State("DEN-377"))
			}
			if len(res.Notes) == 0 || !strings.Contains(r.lastComment(), "Note: ") {
				t.Errorf("the reason must be on the ticket: notes %v, last %q", res.Notes, r.lastComment())
			}
		})
	}
}

func TestFinishWithoutCriteriaWaitsForAPerson(t *testing.T) {
	r := newFinishRig(t, "To Do", "<p>goal only</p>")
	r.checks.Review = func(context.Context, ReviewInput) ([]ACVerdict, error) {
		t.Fatal("no criteria, no review")
		return nil, nil
	}
	res, err := r.finish()
	if err != nil || res.State != "To Verify" || r.board.State("DEN-377") != "To Verify" || !strings.Contains(strings.Join(res.Notes, " "), "no [AC-n] criteria") {
		t.Fatalf("an unchecked ticket is never Done: %+v, %v", res, err)
	}
}

func TestFinishHonoursTheConfiguredHumanReviewLabels(t *testing.T) {
	cases := map[string]struct {
		configured []string
		label      string
		want       string
	}{
		"a configured label holds the ticket":       {[]string{"security"}, "security", "To Verify"},
		"the case of the label does not matter":     {[]string{"Security"}, "security", "To Verify"},
		"a label that is not configured is ignored": {[]string{"security"}, HumanReviewLabel, "Done"},
		"no configuration keeps the default label":  {nil, HumanReviewLabel, "To Verify"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r := newFinishRig(t, "To Do", twoCriteria)
			r.checks.HumanReviewLabels = tc.configured
			if _, _, err := r.Board.Svc.PatchLabels(context.Background(), r.Board.Client, "DEN-377", []string{tc.label}, nil); err != nil {
				t.Fatal(err)
			}
			res, err := r.finish()
			if err != nil || res.State != tc.want {
				t.Fatalf("state = %q, want %q (%v)", res.State, tc.want, err)
			}
		})
	}
}

func TestFinishBlocksWithAHandoffWhenAStageFails(t *testing.T) {
	cases := map[string]struct {
		arrange func(r *finishRig)
		want    []string
	}{
		"the gate fails": {func(r *finishRig) {
			r.checks.Gate = func(context.Context, string, string) (GateResult, error) {
				return GateResult{Report: "FAIL pkg/x"}, nil
			}
		}, []string{"The quality gate failed.", "FAIL pkg/x"}},
		"the gate cannot run": {func(r *finishRig) {
			r.checks.Gate = func(context.Context, string, string) (GateResult, error) {
				return GateResult{}, errors.New("no toolchain")
			}
		}, []string{"the quality gate could not run: no toolchain"}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r := newFinishRig(t, "To Do", twoCriteria)
			tc.arrange(r)
			res, err := r.finish()
			if err != nil || res.State != "Blocked" || r.board.State("DEN-377") != "Blocked" {
				t.Fatalf("%+v, %v, Plane %q", res, err, r.board.State("DEN-377"))
			}
			handoff := ""
			for _, c := range r.board.Texts("DEN-377") {
				if strings.HasPrefix(c, "[dendrite:handoff]") {
					handoff = c
				}
			}
			for _, want := range append([]string{"Completed:", "Challenges:", "Missing:"}, tc.want...) {
				if !strings.Contains(handoff, want) {
					t.Errorf("handoff lacks %q: %q", want, handoff)
				}
			}
			if _, err := r.Store.ClaimFor("agent/den-377"); err != nil {
				t.Errorf("a blocked ticket keeps its claim for whoever resumes: %v", err)
			}
		})
	}
}

func TestFinishRefusals(t *testing.T) {
	r := newFinishRig(t, "To Do", twoCriteria)
	if _, err := r.Finish(context.Background(), "DEN-999", r.checks); err == nil {
		t.Error("an unknown ticket cannot be finished")
	}
	done := newFinishRig(t, "Done", twoCriteria)
	if _, err := done.finish(); !errors.Is(err, ErrNotInProgress) || !strings.Contains(err.Error(), "Done") {
		t.Errorf("a ticket that is not In Progress: %v", err)
	}
}

func TestFinishFailuresOfTheBoardAreReported(t *testing.T) {
	r := newFinishRig(t, "To Do", twoCriteria)
	r.board.Fail("labels", 0)
	res, err := r.finish()
	if err != nil || res.State != "To Verify" || !strings.Contains(strings.Join(res.Notes, " "), "labels could not be read") {
		t.Fatalf("unreadable labels mean a person verifies: %+v, %v", res, err)
	}
}

func TestACIDAndLabels(t *testing.T) {
	for _, in := range []string{"[AC-1]", "AC-1", " ac-1 "} {
		if acID(in) != "AC-1" {
			t.Errorf("acID(%q) = %q", in, acID(in))
		}
	}
	if !hasLabel([]string{"a", "Needs-Human-Review"}, HumanReviewLabel) || hasLabel(nil, "x") {
		t.Error("hasLabel")
	}
}

func TestFinishReportsBoardErrorsAtEachStep(t *testing.T) {
	cases := map[string]struct {
		method string
		after  int
		want   string
	}{
		"reading the ticket for its criteria": {"GET", 3, "list issues"},
		"posting the report":                  {"POST", 0, "add comment"},
		"setting Done":                        {"PATCH", 1, "to Done"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r := newFinishRig(t, "To Do", twoCriteria)
			r.board.Fail(tc.method, tc.after)
			if _, err := r.finish(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want one mentioning %q", err, tc.want)
			}
		})
	}
}

func TestFinishReportMarksAnExtraFailingVerdict(t *testing.T) {
	r := newFinishRig(t, "To Do", twoCriteria)
	inner := r.checks.Review
	r.checks.Review = func(ctx context.Context, in ReviewInput) ([]ACVerdict, error) {
		v, _ := inner(ctx, in)
		return append(v, ACVerdict{AC: "AC-9", Status: StatusNotMet, Reason: "not a criterion of this ticket"}), nil
	}
	if res, err := r.finish(); err != nil || res.State != "Done" || !strings.Contains(r.lastComment(), "AC-9 not_met") {
		t.Fatalf("%+v, %v, %q", res, err, r.lastComment())
	}
}

func TestFinishSendsUnprovenCriteriaToAPersonAndNamesThem(t *testing.T) {
	cases := map[string][]ACVerdict{
		"unclear with a reason":      {{AC: "[AC-1]", Status: StatusMet, Evidence: "x"}, {AC: "[AC-2]", Status: StatusUnproven, Reason: "the excerpts do not show it"}},
		"unproven without a reason":  {{AC: "[AC-1]", Status: StatusUnproven}, {AC: "[AC-2]", Status: StatusMet}},
		"no verdict for a criterion": {{AC: "[AC-1]", Status: StatusMet}},
		"an unknown status":          {{AC: "[AC-1]", Status: StatusMet}, {AC: "[AC-2]", Status: "maybe"}},
	}
	for name, verdicts := range cases {
		t.Run(name, func(t *testing.T) {
			r := newFinishRig(t, "To Do", twoCriteria)
			r.checks.Review = func(context.Context, ReviewInput) ([]ACVerdict, error) { return verdicts, nil }
			res, err := r.finish()
			if err != nil || res.State != "To Verify" || r.board.State("DEN-377") != "To Verify" {
				t.Fatalf("nothing is proven wrong, so a person verifies: %+v, %v", res, err)
			}
			if note := strings.Join(res.Notes, "\n"); !strings.Contains(note, "no verifiable evidence in the change for:") || !strings.Contains(note, "- [AC-") {
				t.Errorf("the criteria without proof are named: %q", note)
			}
		})
	}
}

func TestAReviewThatDoubtsACriterionSendsTheTicketToAPersonWithItsReason(t *testing.T) {
	r := newFinishRig(t, "To Do", twoCriteria)
	r.checks.Review = func(context.Context, ReviewInput) ([]ACVerdict, error) {
		return []ACVerdict{{AC: "[AC-1]", Status: StatusNotMet, Reason: "nothing calls it"}, {AC: "[AC-2]", Status: StatusMet, Evidence: "x"}}, nil
	}
	res, err := r.finish()
	if err != nil || res.State != "To Verify" || r.board.State("DEN-377") != "To Verify" {
		t.Fatalf("only the quality gate blocks: %+v, %v", res, err)
	}
	if note := strings.Join(res.Notes, "\n"); !strings.Contains(note, "[AC-1] it works: the review believes it is not met: nothing calls it") {
		t.Errorf("the doubt and its reason reach the ticket: %q", note)
	}
}

func TestJudgeCriteriaListsWhatTheReviewDidNotProve(t *testing.T) {
	criteria := []string{"[AC-1] a", "[AC-2] b", "[AC-3] c", "[AC-4] d", "[AC-5] e"}
	got := judgeCriteria(criteria, []ACVerdict{
		{AC: "ac-1", Status: StatusMet}, {AC: "AC-2", Status: StatusNotMet, Reason: "missing"}, {AC: "[AC-3]", Status: StatusUnproven, Reason: "unclear"},
		{AC: "AC-5", Status: StatusNotMet},
	})
	want := []string{"- [AC-2] b: the review believes it is not met: missing", "- [AC-3] c: unclear", "- [AC-4] d: the review gave no verdict", "- [AC-5] e: the review believes it is not met: no reason given"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("got %q", got)
	}
	if got := judgeCriteria([]string{"[AC-1] a"}, []ACVerdict{{AC: "AC-1", Status: StatusUnproven}}); got[0] != "- [AC-1] a: unproven" {
		t.Errorf("a missing reason reads: %v", got)
	}
}

func TestAutoDoneFollowsTheDirectives(t *testing.T) {
	for name, tc := range map[string]struct {
		toml string
		want bool
	}{
		"no file":               {"", true},
		"on":                    {"[done]\nauto_done = true\n", true},
		"off":                   {"[done]\nauto_done = false\n", false},
		"table left out":        {"[loc]\nmax_production_lines = 300\nwarn_at = 250\n", true},
		"unreadable directives": {"[done\n", false},
		"invalid directive":     {"[coverage]\nmin_percent = 250.0\n", false},
	} {
		dir := t.TempDir()
		if tc.toml != "" {
			fsutiltest.WriteFile(t, dir, directive.Path, tc.toml)
		}
		if got := AutoDone(dir); got != tc.want {
			t.Errorf("%s: AutoDone = %v, want %v", name, got, tc.want)
		}
	}
}

func TestTail(t *testing.T) {
	if got := tail("a\nb\nc\n", 2); got != "b\nc" {
		t.Errorf("tail = %q", got)
	}
	if got := tail("only", 5); got != "only" {
		t.Errorf("tail = %q", got)
	}
}

func goModule(t *testing.T, test string) string {
	t.Helper()
	dir := t.TempDir()
	fsutiltest.WriteFile(t, dir, "go.mod", "module probe\n\ngo 1.22\n")
	fsutiltest.WriteFile(t, dir, "p_test.go", "package probe\n\nimport \"testing\"\n\n"+test)
	return dir
}

func TestToolchainGateRunsTheProjectsTests(t *testing.T) {
	ctx := context.Background()
	pass, err := toolchainGate(ctx, goModule(t, "func TestOK(t *testing.T) {}\n"))
	if err != nil || !pass.Passed || !strings.Contains(pass.Report, "go test") {
		t.Fatalf("passing project: %+v, %v", pass, err)
	}
	fail, err := toolchainGate(ctx, goModule(t, "func TestBad(t *testing.T) { t.Fatal(\"broken on purpose\") }\n"))
	if err != nil || fail.Passed || !strings.Contains(fail.Report, "broken on purpose") {
		t.Fatalf("failing project: %+v, %v", fail, err)
	}
	if _, err := toolchainGate(ctx, t.TempDir()); err == nil {
		t.Error("a directory with no toolchain cannot be gated")
	}
}

func TestBranchDiffIsAgainstDevelop(t *testing.T) {
	repo := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@e.x", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@e.x")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if got := branchDiff(repo, "DEN-1"); got != "" {
		t.Errorf("not a repository: %q", got)
	}
	run("init", "-q", "-b", "develop")
	run("commit", "-q", "--allow-empty", "-m", "base")
	run("checkout", "-q", "-b", "agent/den-1")
	fsutiltest.WriteFile(t, repo, "new.txt", "hello\n")
	run("add", ".")
	run("commit", "-q", "-m", "work")
	if got := branchDiff(repo, "DEN-1"); !strings.Contains(got, "+hello") {
		t.Errorf("diff = %q", got)
	}
}

func TestDefaultFinishChecksAreWiredToTheProject(t *testing.T) {
	dir := t.TempDir()
	fsutiltest.WriteFile(t, dir, directive.Path, "[done]\nauto_done = false\n")
	checks := DefaultFinishChecks(dir)
	if checks.AutoDone || checks.Gate == nil || checks.Diff == nil || checks.Review == nil {
		t.Fatalf("checks = %+v", checks)
	}
	if r := proofReviewer(dir); r.Chat == nil || r.Rank == nil {
		t.Error("the reviewer talks to the gateway and ranks hunks")
	}
}

func TestHumanReviewLabelsComeFromTheDirectives(t *testing.T) {
	dir := t.TempDir()
	fsutiltest.WriteFile(t, dir, directive.Path, "[done]\nhuman_review_labels = [\"security\", \"legal\"]\n")
	if got := DefaultFinishChecks(dir).HumanReviewLabels; strings.Join(got, ",") != "security,legal" {
		t.Errorf("labels = %v", got)
	}
	broken := t.TempDir()
	fsutiltest.WriteFile(t, broken, directive.Path, "[done\n")
	if got := HumanReviewLabels(broken); got != nil {
		t.Errorf("an unreadable file falls back to the default label: %v", got)
	}
}

func TestGateReportListsFailuresBeforeTheTail(t *testing.T) {
	argv := []string{"go", "test", "./..."}
	var lines []string
	lines = append(lines, "ok  \tx/a\t0.1s", "--- FAIL: TestBad (0.00s)", "WARNING: DATA RACE", "FAIL\tx/b\t0.2s")
	for i := 0; i < 60; i++ {
		lines = append(lines, "ok  \tx/pkg"+string(rune('a'+i%26))+"\t0.1s")
	}
	failed := gateReport(argv, strings.Join(lines, "\n"), false)
	for _, want := range []string{"`go test ./...`", "Failures:", "--- FAIL: TestBad", "WARNING: DATA RACE", "FAIL\tx/b", "End of the output:"} {
		if !strings.Contains(failed, want) {
			t.Errorf("report lacks %q:\n%s", want, failed)
		}
	}
	if strings.Count(failed, "\n") > 60 {
		t.Errorf("the tail is short: %d lines", strings.Count(failed, "\n"))
	}
	if passed := gateReport(argv, strings.Join(lines, "\n"), true); strings.Contains(passed, "Failures:") {
		t.Error("a passing run lists no failures")
	}
	if noMatch := gateReport(argv, "exit status 2", false); !strings.Contains(noMatch, "exit status 2") || strings.Contains(noMatch, "Failures:") {
		t.Errorf("a failure with no recognisable line keeps the tail: %q", noMatch)
	}
	many := strings.Repeat("--- FAIL: TestX\n", maxFailureLines+10)
	if got := gateReport(argv, many, false); strings.Count(got, "--- FAIL: TestX") < maxFailureLines || strings.Count(got, "Failures:") != 1 {
		t.Errorf("failures are capped at %d", maxFailureLines)
	}
}

func TestToolchainGateRunsTestsWithoutTheLiveEnvironment(t *testing.T) {
	t.Setenv("PLANE_API_KEY", "live-key")
	t.Setenv("DENDRITE_HOME", "/live/home")
	dir := goModule(t, "func TestIsolated(t *testing.T) {\n\tif os.Getenv(\"PLANE_API_KEY\") != \"\" || os.Getenv(\"DENDRITE_HOME\") != \"\" {\n\t\tt.Fatal(\"the gate leaked the live environment\")\n\t}\n}\n")
	fsutiltest.WriteFile(t, dir, "p_test.go", strings.Replace(mustRead(t, dir, "p_test.go"), "import \"testing\"", "import (\n\t\"os\"\n\t\"testing\"\n)", 1))
	res, err := toolchainGate(context.Background(), dir)
	if err != nil || !res.Passed {
		t.Fatalf("%+v, %v", res, err)
	}
}

func mustRead(t *testing.T, dir, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// repoWithMergedWork is a repository whose develop already holds the ticket's two commits (and a lockfile
// change), the state in which a ticket is finished after its branch was merged.
func repoWithMergedWork(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@e.x", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@e.x")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "develop")
	run("commit", "-q", "--allow-empty", "-m", "base")
	fsutiltest.WriteFile(t, repo, "a.go", "package a\n// json output\n")
	fsutiltest.WriteFile(t, repo, "Cargo.lock", "huge lock noise\n")
	run("add", ".")
	run("commit", "-q", "-m", "[DEN-9] feat: json output")
	fsutiltest.WriteFile(t, repo, "b.go", "package b\n// github annotations\n")
	run("add", ".")
	run("commit", "-q", "-m", "[DEN-9] feat: github annotations")
	fsutiltest.WriteFile(t, repo, "other.go", "package other\n")
	run("add", ".")
	run("commit", "-q", "-m", "[DEN-10] unrelated work")
	return repo
}

func TestBranchDiffOfAMergedTicketIsItsOwnCommitsWithoutLockfiles(t *testing.T) {
	repo := repoWithMergedWork(t)
	got := branchDiff(repo, "DEN-9")
	for _, want := range []string{"+// json output", "+// github annotations"} {
		if !strings.Contains(got, want) {
			t.Errorf("diff lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Cargo.lock") || strings.Contains(got, "other.go") {
		t.Errorf("lockfiles and other tickets' work stay out:\n%s", got)
	}
	if got := branchDiff(repo, "DEN-404"); got != "" {
		t.Errorf("a ticket with no commits has no diff: %q", got)
	}
}

func TestCompletedWorkListsTheTicketsCommitsOnceMerged(t *testing.T) {
	s := newSession(t, map[string]string{"DEN-377": "To Do"})
	repo := repoWithMergedWork(t)
	got := s.completedWork(context.Background(), Claim{Ticket: "DEN-9", Worktree: repo, Branch: "develop"})
	if !strings.Contains(got, "Commits for DEN-9 (already merged):") || !strings.Contains(got, "feat: json output") || !strings.Contains(got, "feat: github annotations") || strings.Contains(got, "unrelated") {
		t.Errorf("completed work = %q", got)
	}
}

func TestBranchDiffLeavesOutGeneratedDocsAndAgentFiles(t *testing.T) {
	repo := repoWithMergedWork(t)
	fsutiltest.WriteFile(t, repo, "docs/cli/x.md", "generated reference\n")
	fsutiltest.WriteFile(t, repo, ".claude/skills/s/SKILL.md", "generated skill\n")
	fsutiltest.WriteFile(t, repo, "real.go", "package real\n")
	run := func(args ...string) {
		t.Helper()
		if out, err := gitCommand(repo, args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("add", ".")
	run("-c", "user.name=t", "-c", "user.email=t@e.x", "commit", "-q", "-m", "[DEN-9] generated files too")
	got := branchDiff(repo, "DEN-9")
	if !strings.Contains(got, "real.go") || strings.Contains(got, "docs/cli/x.md") || strings.Contains(got, ".claude/skills") {
		t.Errorf("generated files are not shown:\n%s", got)
	}
}

// moduleBranch is a Go module on develop with one tested function, and a ticket branch that adds another.
func moduleBranch(t *testing.T, directives string) string {
	t.Helper()
	repo := t.TempDir()
	fsutiltest.WriteFile(t, repo, "go.mod", "module probe\n\ngo 1.22\n")
	fsutiltest.WriteFile(t, repo, "p.go", "package probe\n\nfunc Add(a, b int) int { return a + b }\n")
	fsutiltest.WriteFile(t, repo, "p_test.go", "package probe\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(1, 2) != 3 {\n\t\tt.Fatal(\"add\")\n\t}\n}\n")
	if directives != "" {
		fsutiltest.WriteFile(t, repo, directive.Path, directives)
	}
	run := func(args ...string) {
		t.Helper()
		if out, err := gitCommand(repo, args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "develop")
	run("add", ".")
	run("-c", "user.name=t", "-c", "user.email=t@e.x", "commit", "-q", "-m", "base")
	run("checkout", "-q", "-b", "agent/den-5")
	fsutiltest.WriteFile(t, repo, "p.go", "package probe\n\nfunc Add(a, b int) int { return a + b }\n\nfunc Sub(a, b int) int { return a - b }\n")
	fsutiltest.WriteFile(t, repo, "p_test.go", "package probe\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(1, 2) != 3 || Sub(3, 1) != 2 {\n\t\tt.Fatal(\"math\")\n\t}\n}\n")
	run("add", ".")
	run("-c", "user.name=t", "-c", "user.email=t@e.x", "commit", "-q", "-m", "[DEN-5] add Sub")
	return repo
}

func gateTicket(t *testing.T, repo string) (GateResult, error) {
	t.Helper()
	ragtest.UseStandin(t, "1.12.0")
	return qualityGate(context.Background(), repo, "DEN-5")
}

func TestQualityGatePassesATestedChangeWithinTheLimits(t *testing.T) {
	res, err := gateTicket(t, moduleBranch(t, ""))
	if err != nil || !res.Passed || !strings.Contains(res.Report, "go test -race") || !strings.Contains(res.Report, " passed.\n\nQuality gate (changes): **passed**") || strings.Contains(res.Report, "coverage:") {
		t.Fatalf("a tested change within the limits: %+v, %v", res, err)
	}
}

func TestQualityGateFailsATouchedFileOverTheLimit(t *testing.T) {
	strict := moduleBranch(t, "schema = 1\n[loc]\nmax_production_lines = 50\nwarn_at = 40\n")
	var vars strings.Builder
	for i := 0; i < 60; i++ {
		fmt.Fprintf(&vars, "var v%d = %d\n", i, i)
	}
	fsutiltest.WriteFile(t, strict, "p.go", "package probe\n\nfunc Add(a, b int) int { return a + b }\n\nfunc Sub(a, b int) int { return a - b }\n"+vars.String())
	res, err := gateTicket(t, strict)
	if err != nil || res.Passed || !strings.Contains(res.Report, "p.go: 63 production lines, the limit is 50") || !strings.Contains(res.Report, "Quality gate (changes): **failed**") {
		t.Errorf("a touched file over the limit fails the gate: %+v, %v", res, err)
	}
}

func TestQualityGateFailsUnreadableDirectivesAndSaysWhy(t *testing.T) {
	res, err := gateTicket(t, moduleBranch(t, "[loc\n"))
	if err != nil || res.Passed || !strings.Contains(res.Report, "could not read the directives") {
		t.Errorf("unreadable directives fail the gate and say why: %+v, %v", res, err)
	}
}

func TestQualityGateSkipsTheDirectivesWhenTheTestsFail(t *testing.T) {
	failing := moduleBranch(t, "")
	fsutiltest.WriteFile(t, failing, "p_test.go", "package probe\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) { t.Fatal(\"broken\") }\n")
	res, err := gateTicket(t, failing)
	if err != nil || res.Passed || strings.Contains(res.Report, "Quality gate") {
		t.Errorf("when the tests fail the quality gate is not run: %+v, %v", res, err)
	}
}

func TestQualityGateCannotGateADirectoryWithNoToolchain(t *testing.T) {
	if _, err := gateTicket(t, t.TempDir()); err == nil {
		t.Error("a directory with no toolchain cannot be gated")
	}
}

// proposing is a finish rig whose working copy is a repository on a ticket trunk, with integration as the only
// other branch and a bare origin that has it.
func proposing(t *testing.T, integration string) (*finishRig, *[]pullrequest.Request) {
	t.Helper()
	r := newFinishRig(t, "To Do", twoCriteria)
	origin := t.TempDir()
	testkit.Git(t, origin, "init", "-q", "--bare", "-b", integration)
	testkit.GitInit(t, r.Dir, integration)
	testkit.Git(t, r.Dir, "commit", "-q", "--allow-empty", "-m", "base")
	testkit.Git(t, r.Dir, "remote", "add", "origin", origin)
	testkit.Git(t, r.Dir, "push", "-q", "origin", integration)
	testkit.Git(t, r.Dir, "checkout", "-q", "-b", "DEN-377-work/main")
	r.branch = "DEN-377-work/main"
	var seen []pullrequest.Request
	r.checks.Propose = func(_ context.Context, p pullrequest.Request) (string, error) {
		seen = append(seen, p)
		return "https://github.example/o/r/pull/9", nil
	}
	return r, &seen
}

func TestFinishProposesABranchThatMergesIntoMainAndWaits(t *testing.T) {
	r, seen := proposing(t, "main")
	res, err := r.finish()
	testkit.Check(t, err)
	testkit.Equal(t, "state", res.State, "To Verify")
	testkit.Equal(t, "proposed once", len(*seen), 1)
	p := (*seen)[0]
	testkit.Equal(t, "base", p.Base, "main")
	testkit.Equal(t, "head", p.Head, "DEN-377-work/main")
	testkit.Equal(t, "remote", p.Remote, "origin")
	testkit.Equal(t, "title", p.Title, "[DEN-377] work")
	testkit.Contains(t, p.Body, "Ticket DEN-377", "[AC-1] it works", "[AC-2] it is tested")
	testkit.Contains(t, strings.Join(res.Notes, "\n"), "pull request of DEN-377-work/main into main: https://github.example/o/r/pull/9")
	testkit.Contains(t, r.lastComment(), "pull/9")
}

func TestFinishProposesNothingForABranchThatMergesElsewhere(t *testing.T) {
	r, seen := proposing(t, "develop")
	res, err := r.finish()
	testkit.Check(t, err)
	testkit.Equal(t, "state", res.State, "Done")
	testkit.Equal(t, "not proposed", len(*seen), 0)
}

func TestAPullRequestThatCannotBeOpenedIsANoteThatSaysHow(t *testing.T) {
	r, _ := proposing(t, "main")
	r.checks.Propose = func(context.Context, pullrequest.Request) (string, error) {
		return "", errors.New("gh: not logged in\nrun gh auth login")
	}
	res, err := r.finish()
	testkit.Check(t, err)
	testkit.Equal(t, "still waits for a person", res.State, "To Verify")
	notes := strings.Join(res.Notes, "\n")
	testkit.Contains(t, notes, "could not be opened (gh: not logged in)", "gh pr create --base main --head DEN-377-work/main")
	testkit.NotContains(t, notes, "run gh auth login")
}

func TestTheDefaultChecksProposeByPullRequest(t *testing.T) {
	testkit.Equal(t, "propose", DefaultFinishChecks(t.TempDir()).Propose != nil, true)
}
