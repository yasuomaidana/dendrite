package agentjobs

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// shimToolchain replaces PATH with a directory holding real shell-script
// executables for git, gofmt and go, so Verify runs through ExecRunner against
// tools whose output and exit codes the test controls.
func shimToolchain(t *testing.T, scripts map[string]string) {
	t.Helper()
	binDir := t.TempDir()
	for _, name := range []string{"git", "gofmt", "go"} {
		body, ok := scripts[name]
		if !ok {
			body = "exit 0\n"
		}
		installExecutableScript(t, binDir, name, body)
	}
	t.Setenv("PATH", binDir)
}

func TestVerify_WithRealTempGoModule_Pass(t *testing.T) {
	worktree := t.TempDir()
	initGitRepo(t, worktree, "main")

	// Set up git user
	c1 := exec.Command("git", "config", "user.name", "TestUser")
	c1.Dir = worktree
	_ = c1.Run()
	c2 := exec.Command("git", "config", "user.email", "test@example.com")
	c2.Dir = worktree
	_ = c2.Run()

	// Initialize a tiny go module
	cMod := exec.Command("go", "mod", "init", "tinymod")
	cMod.Dir = worktree
	if out, err := cMod.CombinedOutput(); err != nil {
		t.Fatalf("go mod init failed: %v (%s)", err, string(out))
	}

	// Commit initial state
	cAdd := exec.Command("git", "add", "go.mod")
	cAdd.Dir = worktree
	_ = cAdd.Run()
	cCommit := exec.Command("git", "commit", "-m", "init mod")
	cCommit.Dir = worktree
	_ = cCommit.Run()

	// Create a valid, formatted Go package with a passing test
	pkgDir := filepath.Join(worktree, "pkg", "mathutil")
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		t.Fatalf("failed to create pkg dir: %v", err)
	}

	code := `package mathutil

func Add(a, b int) int {
	return a + b
}
`
	if err := os.WriteFile(filepath.Join(pkgDir, "mathutil.go"), []byte(code), 0644); err != nil {
		t.Fatalf("failed to write mathutil.go: %v", err)
	}

	testCode := `package mathutil

import "testing"

func TestAdd(t *testing.T) {
	if got := Add(2, 3); got != 5 {
		t.Errorf("Add(2, 3) = %d, want 5", got)
	}
}
`
	if err := os.WriteFile(filepath.Join(pkgDir, "mathutil_test.go"), []byte(testCode), 0644); err != nil {
		t.Fatalf("failed to write mathutil_test.go: %v", err)
	}

	ledgerDir := t.TempDir()
	ledger := NewLedger(ledgerDir)

	job := &Job{
		Ticket:   "DEN-991",
		Worktree: worktree,
		Scope:    []string{"pkg/mathutil"},
		Status:   StatusSucceeded,
	}
	created, err := ledger.Create(job)
	if err != nil {
		t.Fatalf("ledger.Create failed: %v", err)
	}

	ctx := context.Background()
	// Run Verify with the default real ExecRunner
	if err := Verify(ctx, ledger, created, &ExecRunner{}); err != nil {
		t.Fatalf("Verify failed: %v", err)
	}

	updated, err := ledger.Load(created.ID)
	if err != nil {
		t.Fatalf("failed to load updated job: %v", err)
	}
	if updated.Status != StatusVerified {
		t.Errorf("job.Status = %q, want %q", updated.Status, StatusVerified)
	}

	events, err := ledger.Events(created.ID)
	if err != nil {
		t.Fatalf("failed to load events: %v", err)
	}

	var foundVerified bool
	for _, ev := range events {
		if ev.Type == "verified" {
			foundVerified = true
			if ev.Data["gofmt"] != "ok" || ev.Data["build"] != "ok" || ev.Data["vet"] != "ok" || ev.Data["tests"] != "ok" {
				t.Errorf("unexpected verified event data: %+v", ev.Data)
			}
			cov, ok := ev.Data["coverage"].(map[string]any)
			if !ok {
				t.Errorf("expected coverage map in event data, got %T: %+v", ev.Data["coverage"], ev.Data)
			} else {
				if len(cov) == 0 {
					t.Errorf("expected non-empty coverage map")
				}
			}
		}
	}
	if !foundVerified {
		t.Errorf("verified event not found in event log")
	}
}

func TestVerify_WithRealTempGoModule_FailureCases(t *testing.T) {
	t.Run("gofmt unformatted failure", func(t *testing.T) {
		worktree := t.TempDir()
		initGitRepo(t, worktree, "main")

		c1 := exec.Command("git", "config", "user.name", "TestUser")
		c1.Dir = worktree
		_ = c1.Run()
		c2 := exec.Command("git", "config", "user.email", "test@example.com")
		c2.Dir = worktree
		_ = c2.Run()

		cMod := exec.Command("go", "mod", "init", "tinymod")
		cMod.Dir = worktree
		_ = cMod.Run()

		cAdd := exec.Command("git", "add", "go.mod")
		cAdd.Dir = worktree
		_ = cAdd.Run()
		cCommit := exec.Command("git", "commit", "-m", "init mod")
		cCommit.Dir = worktree
		_ = cCommit.Run()

		// Badly formatted Go code (tabs missing, spacing messy)
		unformattedCode := `package main

func main() {
var x = 1
        var y = 2
  _ = x + y
}
`
		if err := os.WriteFile(filepath.Join(worktree, "main.go"), []byte(unformattedCode), 0644); err != nil {
			t.Fatalf("failed to write main.go: %v", err)
		}

		ledger := NewLedger(t.TempDir())
		job, _ := ledger.Create(&Job{
			Ticket:   "DEN-991",
			Worktree: worktree,
			Scope:    []string{"main.go"},
			Status:   StatusSucceeded,
		})

		err := Verify(context.Background(), ledger, job, &ExecRunner{})
		if err != nil {
			t.Fatalf("Verify returned unexpected error: %v", err)
		}

		updated, _ := ledger.Load(job.ID)
		if updated.Status != StatusFailed {
			t.Errorf("updated.Status = %q, want %q", updated.Status, StatusFailed)
		}

		events, _ := ledger.Events(job.ID)
		var foundFailed bool
		for _, ev := range events {
			if ev.Type == "failed" {
				foundFailed = true
				out, _ := ev.Data["output"].(string)
				if !strings.Contains(out, "gofmt -l found unformatted files") {
					t.Errorf("failed event output does not mention gofmt: %q", out)
				}
			}
		}
		if !foundFailed {
			t.Errorf("expected failed event")
		}
	})

	t.Run("build failure", func(t *testing.T) {
		worktree := t.TempDir()
		initGitRepo(t, worktree, "main")

		c1 := exec.Command("git", "config", "user.name", "TestUser")
		c1.Dir = worktree
		_ = c1.Run()
		c2 := exec.Command("git", "config", "user.email", "test@example.com")
		c2.Dir = worktree
		_ = c2.Run()

		cMod := exec.Command("go", "mod", "init", "tinymod")
		cMod.Dir = worktree
		_ = cMod.Run()

		cAdd := exec.Command("git", "add", "go.mod")
		cAdd.Dir = worktree
		_ = cAdd.Run()
		cCommit := exec.Command("git", "commit", "-m", "init mod")
		cCommit.Dir = worktree
		_ = cCommit.Run()

		// Valid format, but compiler error
		badCode := `package main

func main() {
	undefinedFunctionCall()
}
`
		if err := os.WriteFile(filepath.Join(worktree, "main.go"), []byte(badCode), 0644); err != nil {
			t.Fatalf("failed to write main.go: %v", err)
		}

		ledger := NewLedger(t.TempDir())
		job, _ := ledger.Create(&Job{
			Ticket:   "DEN-991",
			Worktree: worktree,
			Scope:    []string{"main.go"},
			Status:   StatusSucceeded,
		})

		err := Verify(context.Background(), ledger, job, &ExecRunner{})
		if err != nil {
			t.Fatalf("Verify returned unexpected error: %v", err)
		}

		updated, _ := ledger.Load(job.ID)
		if updated.Status != StatusFailed {
			t.Errorf("updated.Status = %q, want %q", updated.Status, StatusFailed)
		}

		events, _ := ledger.Events(job.ID)
		var foundFailed bool
		for _, ev := range events {
			if ev.Type == "failed" {
				foundFailed = true
				out, _ := ev.Data["output"].(string)
				if !strings.Contains(out, "go build ./... failed") {
					t.Errorf("failed event output does not mention build failure: %q", out)
				}
			}
		}
		if !foundFailed {
			t.Errorf("expected failed event")
		}
	})

	t.Run("test failure", func(t *testing.T) {
		worktree := t.TempDir()
		initGitRepo(t, worktree, "main")

		c1 := exec.Command("git", "config", "user.name", "TestUser")
		c1.Dir = worktree
		_ = c1.Run()
		c2 := exec.Command("git", "config", "user.email", "test@example.com")
		c2.Dir = worktree
		_ = c2.Run()

		cMod := exec.Command("go", "mod", "init", "tinymod")
		cMod.Dir = worktree
		_ = cMod.Run()

		cAdd := exec.Command("git", "add", "go.mod")
		cAdd.Dir = worktree
		_ = cAdd.Run()
		cCommit := exec.Command("git", "commit", "-m", "init mod")
		cCommit.Dir = worktree
		_ = cCommit.Run()

		pkgDir := filepath.Join(worktree, "pkg", "failing")
		_ = os.MkdirAll(pkgDir, 0755)

		_ = os.WriteFile(filepath.Join(pkgDir, "failing.go"), []byte("package failing\n"), 0644)
		failingTest := `package failing

import "testing"

func TestFail(t *testing.T) {
	t.Fatalf("intentional failure")
}
`
		_ = os.WriteFile(filepath.Join(pkgDir, "failing_test.go"), []byte(failingTest), 0644)

		ledger := NewLedger(t.TempDir())
		job, _ := ledger.Create(&Job{
			Ticket:   "DEN-991",
			Worktree: worktree,
			Scope:    []string{"pkg/failing"},
			Status:   StatusSucceeded,
		})

		err := Verify(context.Background(), ledger, job, &ExecRunner{})
		if err != nil {
			t.Fatalf("Verify returned unexpected error: %v", err)
		}

		updated, _ := ledger.Load(job.ID)
		if updated.Status != StatusFailed {
			t.Errorf("updated.Status = %q, want %q", updated.Status, StatusFailed)
		}

		events, _ := ledger.Events(job.ID)
		var foundFailed bool
		for _, ev := range events {
			if ev.Type == "failed" {
				foundFailed = true
				out, _ := ev.Data["output"].(string)
				if !strings.Contains(out, "go test -race -cover failed") {
					t.Errorf("failed event output: %q", out)
				}
			}
		}
		if !foundFailed {
			t.Errorf("expected failed event")
		}
	})
}

func TestVerify_RunnerErrorCases(t *testing.T) {
	worktree := t.TempDir()
	ledger := NewLedger(t.TempDir())

	t.Run("git status fails", func(t *testing.T) {
		job, _ := ledger.Create(&Job{
			Ticket:   "DEN-991",
			Worktree: worktree,
			Status:   StatusSucceeded,
		})
		shimToolchain(t, map[string]string{
			"git": "echo 'fatal: not a git repo' >&2\nexit 128\n",
		})
		err := Verify(context.Background(), ledger, job, &ExecRunner{})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		updated, _ := ledger.Load(job.ID)
		if updated.Status != StatusFailed {
			t.Errorf("want StatusFailed, got %q", updated.Status)
		}
	})

	t.Run("gofmt runner returns error", func(t *testing.T) {
		// Create a go file in worktree
		initGitRepo(t, worktree, "main")
		_ = os.WriteFile(filepath.Join(worktree, "foo.go"), []byte("package main\n"), 0644)

		job, _ := ledger.Create(&Job{
			Ticket:   "DEN-991",
			Worktree: worktree,
			Scope:    []string{"foo.go"},
			Status:   StatusSucceeded,
		})
		shimToolchain(t, map[string]string{
			"git":   "echo '?? foo.go'\n",
			"gofmt": "echo 'gofmt: binary broken' >&2\nexit 1\n",
		})
		err := Verify(context.Background(), ledger, job, &ExecRunner{})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		updated, _ := ledger.Load(job.ID)
		if updated.Status != StatusFailed {
			t.Errorf("want StatusFailed, got %q", updated.Status)
		}
	})

	t.Run("go vet fails", func(t *testing.T) {
		job, _ := ledger.Create(&Job{
			Ticket:   "DEN-991",
			Worktree: worktree,
			Scope:    []string{"foo.go"},
			Status:   StatusSucceeded,
		})
		shimToolchain(t, map[string]string{
			"git": "echo '?? foo.go'\n",
			"go":  "if [ \"$1\" = vet ]; then echo 'vet error' >&2; exit 1; fi\nexit 0\n",
		})
		err := Verify(context.Background(), ledger, job, &ExecRunner{})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		updated, _ := ledger.Load(job.ID)
		if updated.Status != StatusFailed {
			t.Errorf("want StatusFailed, got %q", updated.Status)
		}
	})
}

func TestVerify_DefaultLedgerAndWorktree(t *testing.T) {
	// Exercise nil ledger and empty worktree fallback
	job := &Job{
		Ticket: "DEN-991",
		Status: StatusSucceeded,
	}
	shimToolchain(t, nil)
	// Verify should not panic
	_ = Verify(context.Background(), nil, job, &ExecRunner{})
}

func TestParseCoverage(t *testing.T) {
	sampleOutput := `
ok  	Dendrite/pkg/agentjobs	(cached)	coverage: 92.5% of statements
?   	Dendrite/cmd/dendrite	[no test files]
ok  	Dendrite/pkg/statedir	0.012s	coverage: 100.0% of statements
`
	cov := parseCoverage(sampleOutput)
	if cov["Dendrite/pkg/agentjobs"] != "92.5%" {
		t.Errorf("expected 92.5%%, got %q", cov["Dendrite/pkg/agentjobs"])
	}
	if cov["Dendrite/cmd/dendrite"] != "[no test files]" {
		t.Errorf("expected [no test files], got %q", cov["Dendrite/cmd/dendrite"])
	}
	if cov["Dendrite/pkg/statedir"] != "100.0%" {
		t.Errorf("expected 100.0%%, got %q", cov["Dendrite/pkg/statedir"])
	}
}

func TestHasGoPackageDecl(t *testing.T) {
	tmp := t.TempDir()
	validGo := filepath.Join(tmp, "valid.go")
	_ = os.WriteFile(validGo, []byte("package foo\n"), 0644)
	if !HasGoPackageDecl(validGo) {
		t.Errorf("expected validGo to have go package decl")
	}

	invalidGo := filepath.Join(tmp, "invalid.go")
	_ = os.WriteFile(invalidGo, []byte("not a go file"), 0644)
	if HasGoPackageDecl(invalidGo) {
		t.Errorf("expected invalidGo to NOT have go package decl")
	}
}
