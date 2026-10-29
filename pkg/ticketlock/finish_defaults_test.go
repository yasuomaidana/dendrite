package ticketlock

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/rag"
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

func TestAutoDoneReadsTheDoneTable(t *testing.T) {
	for name, tc := range map[string]struct {
		toml string
		want bool
	}{
		"no file":             {"", true},
		"off":                 {"[loc]\nauto_done = false\n[done]\nauto_done = false # humans sign off\n", false},
		"on":                  {"[done]\nauto_done = true\n", true},
		"other table only":    {"[loc]\nauto_done = false\n", true},
		"done without key":    {"[done]\nhuman_review_labels = []\n", true},
		"later table ignored": {"[done]\nx = 1\n[other]\nauto_done = false\n", true},
	} {
		dir := t.TempDir()
		if tc.toml != "" {
			writeFile(t, dir, DirectivesPath, tc.toml)
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
	if got := branchDiff(repo); got != "" {
		t.Errorf("not a repository: %q", got)
	}
	run("init", "-q", "-b", "develop")
	run("commit", "-q", "--allow-empty", "-m", "base")
	run("checkout", "-q", "-b", "agent/den-1")
	writeFile(t, repo, "new.txt", "hello\n")
	run("add", ".")
	run("commit", "-q", "-m", "work")
	if got := branchDiff(repo); !strings.Contains(got, "+hello") {
		t.Errorf("diff = %q", got)
	}
}

func TestDefaultFinishChecksUseTheGatewayAndTheDirective(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/model/info"):
			_, _ = w.Write([]byte(`{"data":[]}`))
		default:
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"[]"}}]}`))
		}
	}))
	defer srv.Close()
	t.Setenv("LITELLM_PROXY_URL", srv.URL+"/v1/")
	t.Setenv("LITELLM_MASTER_KEY", "sk-test")
	dir := t.TempDir()
	writeFile(t, dir, DirectivesPath, "[done]\nauto_done = false\n")

	checks := DefaultFinishChecks(dir)
	if checks.AutoDone || checks.Gate == nil || checks.Diff == nil || checks.Review == nil {
		t.Fatalf("checks = %+v", checks)
	}
	g := gatewayReviewer(dir)
	if g.URL != srv.URL+"/v1/chat/completions" || g.Key != "sk-test" || g.Model == "" {
		t.Errorf("reviewer = %+v", g)
	}
	prev := preferLocalModel
	preferLocalModel = func(context.Context, string) (rag.LocalChoice, bool) {
		return rag.LocalChoice{Model: "local-smart"}, true
	}
	defer func() { preferLocalModel = prev }()
	if g := gatewayReviewer(dir); g.Model != "local-smart" {
		t.Errorf("the gateway's preferred local model is used: %q", g.Model)
	}
	if v, err := checks.Review(context.Background(), ReviewInput{Ticket: "DEN-1"}); err != nil || len(v) != 0 {
		t.Errorf("review through the gateway: %v, %v", v, err)
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
