package ticketlock

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/directive"
)

func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
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
			writeFile(t, dir, directive.Path, tc.toml)
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
	writeFile(t, dir, "go.mod", "module probe\n\ngo 1.22\n")
	writeFile(t, dir, "p_test.go", "package probe\n\nimport \"testing\"\n\n"+test)
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
	writeFile(t, repo, "new.txt", "hello\n")
	run("add", ".")
	run("commit", "-q", "-m", "work")
	if got := branchDiff(repo, "DEN-1"); !strings.Contains(got, "+hello") {
		t.Errorf("diff = %q", got)
	}
}

func TestDefaultFinishChecksAreWiredToTheProject(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, directive.Path, "[done]\nauto_done = false\n")
	checks := DefaultFinishChecks(dir)
	if checks.AutoDone || checks.Gate == nil || checks.Diff == nil || checks.Review == nil {
		t.Fatalf("checks = %+v", checks)
	}
	if r := proofReviewer(dir); r.Chat == nil || r.Rank == nil {
		t.Error("the reviewer talks to the gateway and ranks hunks")
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

func TestCleanEnvDropsTheLiveServiceVariables(t *testing.T) {
	got := cleanEnv([]string{"PATH=/bin", "PLANE_API_KEY=k", "DENDRITE_HOME=/h", "LITELLM_MASTER_KEY=m", "HOME=/u", "PLANEX=keep"})
	if strings.Join(got, " ") != "PATH=/bin HOME=/u PLANEX=keep" {
		t.Errorf("env = %v", got)
	}
}

func TestToolchainGateRunsTestsWithoutTheLiveEnvironment(t *testing.T) {
	t.Setenv("PLANE_API_KEY", "live-key")
	t.Setenv("DENDRITE_HOME", "/live/home")
	dir := goModule(t, "func TestIsolated(t *testing.T) {\n\tif os.Getenv(\"PLANE_API_KEY\") != \"\" || os.Getenv(\"DENDRITE_HOME\") != \"\" {\n\t\tt.Fatal(\"the gate leaked the live environment\")\n\t}\n}\n")
	writeFile(t, dir, "p_test.go", strings.Replace(mustRead(t, dir, "p_test.go"), "import \"testing\"", "import (\n\t\"os\"\n\t\"testing\"\n)", 1))
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
	writeFile(t, repo, "a.go", "package a\n// json output\n")
	writeFile(t, repo, "Cargo.lock", "huge lock noise\n")
	run("add", ".")
	run("commit", "-q", "-m", "[DEN-9] feat: json output")
	writeFile(t, repo, "b.go", "package b\n// github annotations\n")
	run("add", ".")
	run("commit", "-q", "-m", "[DEN-9] feat: github annotations")
	writeFile(t, repo, "other.go", "package other\n")
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

func TestTicketCommitsMatchOnTheSubjectPrefix(t *testing.T) {
	repo := repoWithMergedWork(t)
	shas, lines := ticketCommits(repo, "DEN-9")
	if len(shas) != 2 || len(lines) != 2 || !strings.HasSuffix(lines[0], " [DEN-9] feat: json output") || !strings.HasSuffix(lines[1], "github annotations") {
		t.Errorf("%v %v", shas, lines)
	}
	if s, l := ticketCommits(t.TempDir(), "DEN-9"); s != nil || l != nil {
		t.Error("not a repository")
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
	writeFile(t, repo, "docs/cli/x.md", "generated reference\n")
	writeFile(t, repo, ".claude/skills/s/SKILL.md", "generated skill\n")
	writeFile(t, repo, "real.go", "package real\n")
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
