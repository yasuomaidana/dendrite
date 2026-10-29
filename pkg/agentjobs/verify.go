package agentjobs

import (
	"bytes"
	"context"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// CommandRunner abstracts command execution for testability and hermetic fixtures.
type CommandRunner interface {
	Run(ctx context.Context, dir string, name string, args ...string) (stdout []byte, stderr []byte, exitCode int, err error)
}

// ExecRunner is the default CommandRunner using os/exec.
type ExecRunner struct{}

// Run executes the command using os/exec.CommandContext.
func (e *ExecRunner) Run(ctx context.Context, dir string, name string, args ...string) ([]byte, []byte, int, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}
	return stdout.Bytes(), stderr.Bytes(), exitCode, err
}

// VerificationResult holds the structured outcome of a successful verification.
type VerificationResult struct {
	Gofmt    string            `json:"gofmt"`
	Build    string            `json:"build"`
	Vet      string            `json:"vet"`
	Tests    string            `json:"tests"`
	Coverage map[string]string `json:"coverage"`
}

// Verify verifies an agent job in its worktree by running gofmt -l on changed in-scope Go files,
// go build ./..., go vet, and go test -race -cover on the affected packages.
func Verify(ctx context.Context, ledger *Ledger, job *Job, runner CommandRunner) error {
	if ledger == nil {
		ledger = NewLedger("")
	}
	if runner == nil {
		runner = &ExecRunner{}
	}

	worktree := job.Worktree
	if worktree == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return recordFailure(ledger, job, fmt.Sprintf("failed to get current working directory: %v", err))
		}
		worktree = cwd
	}

	goFiles, err := findChangedInScopeGoFiles(ctx, runner, worktree, job.Scope)
	if err != nil {
		return recordFailure(ledger, job, fmt.Sprintf("scope inspection failed: %v", err))
	}

	// 1. gofmt -l on changed in-scope Go files
	if len(goFiles) > 0 {
		args := append([]string{"-l"}, goFiles...)
		stdout, stderr, code, err := runner.Run(ctx, worktree, "gofmt", args...)
		if code != 0 || err != nil {
			out := tailString(string(stdout)+"\n"+string(stderr)+"\n"+errStr(err), 4000)
			return recordFailure(ledger, job, "gofmt execution error:\n"+out)
		}
		unformatted := strings.TrimSpace(string(stdout))
		if unformatted != "" {
			return recordFailure(ledger, job, fmt.Sprintf("gofmt -l found unformatted files:\n%s", tailString(unformatted, 4000)))
		}
	}

	// 2. go build ./...
	stdout, stderr, code, err := runner.Run(ctx, worktree, "go", "build", "./...")
	if code != 0 || err != nil {
		out := tailString(string(stdout)+"\n"+string(stderr)+"\n"+errStr(err), 4000)
		return recordFailure(ledger, job, "go build ./... failed:\n"+out)
	}

	// 3. Derive affected packages
	packages := derivePackages(worktree, goFiles)
	if len(packages) == 0 {
		packages = []string{"./..."}
	}

	// 4. go vet on affected packages
	vetArgs := append([]string{"vet"}, packages...)
	stdout, stderr, code, err = runner.Run(ctx, worktree, "go", vetArgs...)
	if code != 0 || err != nil {
		out := tailString(string(stdout)+"\n"+string(stderr)+"\n"+errStr(err), 4000)
		return recordFailure(ledger, job, "go vet failed:\n"+out)
	}

	// 5. go test -race -cover on affected packages
	testArgs := append([]string{"test", "-race", "-cover"}, packages...)
	stdout, stderr, code, err = runner.Run(ctx, worktree, "go", testArgs...)
	testCombined := string(stdout) + "\n" + string(stderr)
	if code != 0 || err != nil {
		out := tailString(testCombined+"\n"+errStr(err), 4000)
		return recordFailure(ledger, job, "go test -race -cover failed:\n"+out)
	}

	coverage := parseCoverage(testCombined)

	// Record verified event and status
	res := VerificationResult{
		Gofmt:    "ok",
		Build:    "ok",
		Vet:      "ok",
		Tests:    "ok",
		Coverage: coverage,
	}

	job.Status = StatusVerified
	if err := ledger.Save(job); err != nil {
		return fmt.Errorf("verify: failed to save job: %w", err)
	}

	return ledger.Append(job.ID, Event{
		Type: "verified",
		Data: map[string]any{
			"gofmt":    res.Gofmt,
			"build":    res.Build,
			"vet":      res.Vet,
			"tests":    res.Tests,
			"coverage": res.Coverage,
		},
	})
}

func recordFailure(ledger *Ledger, job *Job, failingOutput string) error {
	job.Status = StatusFailed
	if err := ledger.Save(job); err != nil {
		return fmt.Errorf("record failure: failed to save job: %w", err)
	}
	return ledger.Append(job.ID, Event{
		Type: "failed",
		Data: map[string]any{
			"output": tailString(failingOutput, 4000),
		},
	})
}

var coverageRe = regexp.MustCompile(`(?:^|\n)(?:ok\s+|\?   \s+)(\S+)\s+(?:.*?coverage:\s+([0-9.]+% of statements|[^\n\r]+)|\[no test files\])`)

func parseCoverage(output string) map[string]string {
	coverage := make(map[string]string)
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "ok ") && !strings.HasPrefix(line, "? ") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			pkg := parts[1]
			covIdx := -1
			for i, p := range parts {
				if p == "coverage:" && i+1 < len(parts) {
					covIdx = i + 1
					break
				}
			}
			if covIdx != -1 {
				coverage[pkg] = parts[covIdx]
			} else if strings.Contains(line, "[no test files]") {
				coverage[pkg] = "[no test files]"
			}
		}
	}
	return coverage
}

func cleanScopeList(scope []string) []string {
	cleanScopes := make([]string, 0, len(scope))
	for _, s := range scope {
		s = strings.TrimSpace(s)
		if s != "" {
			cleanScopes = append(cleanScopes, filepath.Clean(s))
		}
	}
	return cleanScopes
}

func fileMatchesScope(path string, cleanScopes []string) bool {
	for _, s := range cleanScopes {
		if s == "." || path == s || strings.HasPrefix(path, s+string(filepath.Separator)) || strings.HasPrefix(path, s+"/") {
			return true
		}
	}
	return false
}

func findChangedInScopeGoFiles(ctx context.Context, runner CommandRunner, worktree string, scope []string) ([]string, error) {
	stdout, stderr, code, err := runner.Run(ctx, worktree, "git", "status", "--porcelain", "--untracked-files=all")
	if code != 0 || err != nil {
		return nil, fmt.Errorf("git status error (%s): %w", string(stderr), err)
	}

	cleanScopes := cleanScopeList(scope)

	var goFiles []string
	lines := strings.Split(string(stdout), "\n")
	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r")
		if len(trimmed) < 4 {
			continue
		}
		pathPart := trimmed[3:]
		if strings.Contains(pathPart, " -> ") {
			parts := strings.Split(pathPart, " -> ")
			pathPart = parts[len(parts)-1]
		}
		pathPart = strings.TrimSpace(pathPart)
		pathPart = strings.Trim(pathPart, "\"")
		if pathPart == "" {
			continue
		}
		cleanFile := filepath.Clean(pathPart)
		if !strings.HasSuffix(cleanFile, ".go") {
			continue
		}

		inScope := len(cleanScopes) == 0 || fileMatchesScope(cleanFile, cleanScopes)
		if inScope {
			// verify file exists (e.g. wasn't deleted)
			fullPath := filepath.Join(worktree, cleanFile)
			if fi, err := os.Stat(fullPath); err == nil && !fi.IsDir() {
				goFiles = append(goFiles, cleanFile)
			}
		}
	}

	sort.Strings(goFiles)
	return goFiles, nil
}

func derivePackages(worktree string, goFiles []string) []string {
	if len(goFiles) == 0 {
		return nil
	}
	pkgMap := make(map[string]bool)
	for _, f := range goFiles {
		dir := filepath.Dir(f)
		rel := filepath.ToSlash(dir)
		if rel == "." || rel == "" {
			rel = "."
		} else if !strings.HasPrefix(rel, "./") {
			rel = "./" + rel
		}
		pkgMap[rel] = true
	}

	var packages []string
	for p := range pkgMap {
		packages = append(packages, p)
	}
	sort.Strings(packages)
	return packages
}

// HasGoPackageDecl is a helper to verify a file is a valid go file.
func HasGoPackageDecl(path string) bool {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.PackageClauseOnly)
	return err == nil && f != nil
}

func tailString(s string, maxChars int) string {
	if len(s) <= maxChars {
		return s
	}
	return s[len(s)-maxChars:]
}

func errStr(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
