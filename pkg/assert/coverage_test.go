package assert

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func runGitCommand(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v, output: %s", args, err, string(out))
	}
}

func TestChangedPackages_RealGitRepo(t *testing.T) {
	repoDir := t.TempDir()

	runGitCommand(t, repoDir, "init")
	runGitCommand(t, repoDir, "config", "user.name", "Assertion Tester")
	runGitCommand(t, repoDir, "config", "user.email", "tester@example.com")

	if err := os.MkdirAll(filepath.Join(repoDir, "pkg", "alpha"), 0755); err != nil {
		t.Fatalf("failed creating pkg/alpha: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repoDir, "pkg", "beta"), 0755); err != nil {
		t.Fatalf("failed creating pkg/beta: %v", err)
	}

	fileA := filepath.Join(repoDir, "pkg", "alpha", "a.go")
	fileB := filepath.Join(repoDir, "pkg", "beta", "b.go")
	_ = os.WriteFile(fileA, []byte("package alpha\n\nfunc A() int { return 1 }\n"), 0644)
	_ = os.WriteFile(fileB, []byte("package beta\n\nfunc B() int { return 2 }\n"), 0644)

	runGitCommand(t, repoDir, "add", ".")
	runGitCommand(t, repoDir, "commit", "-m", "initial commit")

	// Commit 2: Modify pkg/alpha only
	_ = os.WriteFile(fileA, []byte("package alpha\n\nfunc A() int { return 1 }\nfunc Extra() int { return 10 }\n"), 0644)
	runGitCommand(t, repoDir, "add", ".")
	runGitCommand(t, repoDir, "commit", "-m", "update pkg/alpha")

	// Verify changed packages against previous commit
	pkgs, err := ChangedPackages(repoDir, "HEAD~1")
	if err != nil {
		t.Fatalf("ChangedPackages returned error: %v", err)
	}
	expected := []string{"pkg/alpha"}
	if !reflect.DeepEqual(pkgs, expected) {
		t.Fatalf("expected changed packages %v, got %v", expected, pkgs)
	}

	// Uncommitted working tree modification in pkg/beta
	_ = os.WriteFile(fileB, []byte("package beta\n\nfunc B() int { return 42 }\n"), 0644)
	workingPkgs, err := ChangedPackages(repoDir, "")
	if err != nil {
		t.Fatalf("ChangedPackages on uncommitted changes returned error: %v", err)
	}
	expectedWorking := []string{"pkg/beta"}
	if !reflect.DeepEqual(workingPkgs, expectedWorking) {
		t.Fatalf("expected uncommitted packages %v, got %v", expectedWorking, workingPkgs)
	}

	// Verify root-level Go file changes
	_ = os.WriteFile(filepath.Join(repoDir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0644)
	rootPkgs, err := ChangedPackages(repoDir, "")
	if err != nil {
		t.Fatalf("ChangedPackages on root package returned error: %v", err)
	}
	foundRoot := false
	for _, p := range rootPkgs {
		if p == "." {
			foundRoot = true
			break
		}
	}
	if !foundRoot {
		t.Fatalf("expected root package '.' in changed packages, got %v", rootPkgs)
	}
}

func TestChangedPackages_InvalidRepo(t *testing.T) {
	nonRepoDir := t.TempDir()
	_, err := ChangedPackages(nonRepoDir, "HEAD")
	if err == nil {
		t.Fatalf("expected error running git diff in non-git directory")
	}
}

func TestCoverageForPackages_ChangedPackagesOnly(t *testing.T) {
	profileContent := `mode: atomic
Dendrite/pkg/alpha/alpha.go:10.1,15.2 10 1
Dendrite/pkg/beta/beta.go:20.1,25.2 10 0
`

	// 1. Computed strictly for changed pkg/alpha (100%)
	covAlpha, err := CoverageForPackages(profileContent, []string{"pkg/alpha"})
	if err != nil {
		t.Fatalf("unexpected error parsing coverage: %v", err)
	}
	if covAlpha != 100.0 {
		t.Fatalf("expected 100%% coverage for pkg/alpha, got %.2f%%", covAlpha)
	}

	// 2. Computed strictly for changed pkg/beta (0%)
	covBeta, err := CoverageForPackages(profileContent, []string{"pkg/beta"})
	if err != nil {
		t.Fatalf("unexpected error parsing coverage: %v", err)
	}
	if covBeta != 0.0 {
		t.Fatalf("expected 0%% coverage for pkg/beta, got %.2f%%", covBeta)
	}

	// 3. Computed for both packages (50%)
	covBoth, err := CoverageForPackages(profileContent, []string{"pkg/alpha", "pkg/beta"})
	if err != nil {
		t.Fatalf("unexpected error parsing coverage: %v", err)
	}
	if covBoth != 50.0 {
		t.Fatalf("expected 50%% combined coverage, got %.2f%%", covBoth)
	}

	// 4. When no packages are specified, parses all packages
	covAll, err := CoverageForPackages(profileContent, nil)
	if err != nil {
		t.Fatalf("unexpected error parsing coverage with nil targets: %v", err)
	}
	if covAll != 50.0 {
		t.Fatalf("expected 50%% coverage across all packages, got %.2f%%", covAll)
	}

	// 5. When target package has 0 statements, returns 100.0
	covEmpty, err := CoverageForPackages(profileContent, []string{"pkg/nonexistent"})
	if err != nil {
		t.Fatalf("unexpected error parsing empty package coverage: %v", err)
	}
	if covEmpty != 100.0 {
		t.Fatalf("expected 100%% for package with zero statements, got %.2f%%", covEmpty)
	}
}

func TestParseCoverageReport_FromFile(t *testing.T) {
	covDir := t.TempDir()
	covFile := filepath.Join(covDir, "cov.out")
	content := `mode: set
Dendrite/pkg/alpha/alpha.go:10.1,15.2 8 5
Dendrite/pkg/alpha/extra.go:5.1,8.2 2 0
Dendrite/pkg/gamma/gamma.go:1.1,3.2 4 4
`
	if err := os.WriteFile(covFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed writing coverage file: %v", err)
	}

	// Filter with relative formatting: "./pkg/alpha/..."
	report, err := ParseCoverageReport(covFile, []string{"./pkg/alpha/..."})
	if err != nil {
		t.Fatalf("ParseCoverageReport returned error: %v", err)
	}

	if report.TotalStmts != 10 {
		t.Fatalf("expected 10 total stmts in pkg/alpha, got %d", report.TotalStmts)
	}
	if report.CoveredStmts != 8 {
		t.Fatalf("expected 8 covered stmts in pkg/alpha, got %d", report.CoveredStmts)
	}
	if report.Percentage != 80.0 {
		t.Fatalf("expected 80%% coverage, got %.2f%%", report.Percentage)
	}
	if len(report.Uncovered) != 1 {
		t.Fatalf("expected 1 uncovered statement block, got %d", len(report.Uncovered))
	}
}

func TestEvaluateCoverage_Gates(t *testing.T) {
	// Above gate
	verdict, reasons := EvaluateCoverage(87.5, 85.0)
	if verdict != VerifiedComplete || len(reasons) != 0 {
		t.Fatalf("expected VerifiedComplete at 87.5%%, got %s (%v)", verdict, reasons)
	}

	// Exact gate
	verdictExact, _ := EvaluateCoverage(85.0, 85.0)
	if verdictExact != VerifiedComplete {
		t.Fatalf("expected VerifiedComplete at exact 85.0%%, got %s", verdictExact)
	}

	// Below gate
	verdictBelow, reasonsBelow := EvaluateCoverage(84.9, 85.0)
	if verdictBelow != CoverageBelowGate || len(reasonsBelow) == 0 {
		t.Fatalf("expected CoverageBelowGate at 84.9%%, got %s (%v)", verdictBelow, reasonsBelow)
	}

	// Default gate fallback (85%)
	verdictDefault, _ := EvaluateCoverage(82.0, 0)
	if verdictDefault != CoverageBelowGate {
		t.Fatalf("expected default gate of 85%% to reject 82.0%%, got %s", verdictDefault)
	}
}

func TestChangedPackages_EmptyRepoDir(t *testing.T) {
	// Empty repoDir defaults to "."
	pkgs, err := ChangedPackages("", "HEAD")
	if err != nil {
		t.Fatalf("ChangedPackages with empty repoDir failed: %v", err)
	}
	_ = pkgs
}

func TestRunGitDiff_Branches(t *testing.T) {
	// Case 1: baseRef == "" on a repo with no commits (HEAD unavailable)
	// fallback git diff --name-only executes
	repoDir := t.TempDir()
	runGitCommand(t, repoDir, "init")
	runGitCommand(t, repoDir, "config", "user.name", "Tester")
	runGitCommand(t, repoDir, "config", "user.email", "tester@example.com")

	outFallback, errFallback := runGitDiff(repoDir, "")
	if errFallback != nil {
		t.Fatalf("runGitDiff fallback on empty repo failed: %v", errFallback)
	}
	_ = outFallback

	// Case 2: baseRef == "" on a non-git directory (both HEAD diff and fallback diff fail)
	nonRepo := t.TempDir()
	_, errNonRepo := runGitDiff(nonRepo, "")
	if errNonRepo == nil {
		t.Fatalf("expected error from runGitDiff on non-git dir with baseRef empty")
	}

	// Case 3: baseRef != ""
	outBase, errBase := runGitDiff(repoDir, "HEAD")
	if errBase == nil {
		t.Fatalf("expected error for non-existent baseRef HEAD on empty repo, got %s", outBase)
	}
}

func TestIsSourceFile(t *testing.T) {
	sourceFiles := []string{
		"code.go", "code.rs", "code.ts", "code.js",
		"code.py", "code.c", "code.cpp", "code.h",
	}
	for _, f := range sourceFiles {
		if !isSourceFile(f) {
			t.Errorf("expected %s to be recognized as source file", f)
		}
	}

	nonSourceFiles := []string{
		"README.md", "image.png", "archive.tar.gz", "license", "",
	}
	for _, f := range nonSourceFiles {
		if isSourceFile(f) {
			t.Errorf("expected %s NOT to be recognized as source file", f)
		}
	}
}

func TestCoverageForPackages_Error(t *testing.T) {
	// Line longer than bufio.MaxScanTokenSize (64KB) causes ParseCoverageReport to error
	longProfile := "mode: set\n" + strings.Repeat("x", 70000)
	_, err := CoverageForPackages(longProfile, []string{"pkg"})
	if err == nil {
		t.Fatalf("expected error from CoverageForPackages with invalid scanner line")
	}
}

func TestParseCoverageReport_ErrorsAndBlankLines(t *testing.T) {
	// Scanner error
	longProfile := "mode: set\n" + strings.Repeat("x", 70000)
	_, errScanner := ParseCoverageReport(longProfile, nil)
	if errScanner == nil {
		t.Fatalf("expected scanner error in ParseCoverageReport")
	}

	// Permission error from openProfileReader
	unreadableFile := filepath.Join(t.TempDir(), "unreadable.cov")
	if err := os.WriteFile(unreadableFile, []byte("mode: set\n"), 0000); err == nil {
		_, errUnreadable := ParseCoverageReport(unreadableFile, nil)
		if errUnreadable == nil {
			t.Logf("permission check skipped if root or file readable")
		}
	}

	// Blank lines and spaces
	content := "mode: atomic\n   \n\npkg/a/a.go:1.1,2.1 1 1\n"
	rep, err := ParseCoverageReport(content, nil)
	if err != nil {
		t.Fatalf("unexpected error parsing report with blank lines: %v", err)
	}
	if rep.TotalStmts != 1 {
		t.Errorf("expected 1 total stmt, got %d", rep.TotalStmts)
	}
}

func TestOpenProfileReader_Branches(t *testing.T) {
	// Mode prefix without newline
	rMode, cMode, errMode := openProfileReader("mode: set")
	if errMode != nil || rMode == nil || cMode != nil {
		t.Fatalf("unexpected result for mode without newline: %v", errMode)
	}

	// Newline string
	rNL, cNL, errNL := openProfileReader("foo\nbar")
	if errNL != nil || rNL == nil || cNL != nil {
		t.Fatalf("unexpected result for newline string: %v", errNL)
	}

	// Directory path
	rDir, cDir, errDir := openProfileReader(t.TempDir())
	if errDir != nil || rDir == nil || cDir != nil {
		t.Fatalf("unexpected result for directory path: %v", errDir)
	}

	// Non-existent path without newline
	rNon, cNon, errNon := openProfileReader(filepath.Join(t.TempDir(), "nonexistent"))
	if errNon != nil || rNon == nil || cNon != nil {
		t.Fatalf("unexpected result for non-existent path: %v", errNon)
	}

	// Valid readable file
	validFile := filepath.Join(t.TempDir(), "valid.cov")
	_ = os.WriteFile(validFile, []byte("mode: set\n"), 0644)
	rFile, cFile, errFile := openProfileReader(validFile)
	if errFile != nil || rFile == nil || cFile == nil {
		t.Fatalf("unexpected result for valid file: %v", errFile)
	}
	cFile.Close()

	// Unreadable file (chmod 0000)
	unreadable := filepath.Join(t.TempDir(), "unreadable.cov")
	_ = os.WriteFile(unreadable, []byte("mode: set\n"), 0000)
	_, cUnread, errUnread := openProfileReader(unreadable)
	if errUnread == nil && cUnread != nil {
		cUnread.Close()
	}
}

func TestProcessCoverageLine_MalformedAndUncovered(t *testing.T) {
	report := &CoverageReport{
		Packages: make(map[string]PackageCoverage),
	}

	// Less than 3 parts
	processCoverageLine("singlepart", nil, report)
	processCoverageLine("two parts", nil, report)

	// More than 3 parts
	processCoverageLine("one two three four", nil, report)

	// Non-integer numStmts
	processCoverageLine("pkg/foo.go:1.1,2.1 bad 1", nil, report)

	// Non-integer count
	processCoverageLine("pkg/foo.go:1.1,2.1 1 bad", nil, report)

	// Targets filtering non-matching
	processCoverageLine("pkg/other/foo.go:1.1,2.1 1 1", []string{"pkg/alpha"}, report)

	if report.TotalStmts != 0 {
		t.Fatalf("expected 0 total stmts from malformed and non-matching lines, got %d", report.TotalStmts)
	}

	// Uncovered block (count == 0)
	processCoverageLine("pkg/foo.go:1.1,2.1 5 0", nil, report)
	if report.TotalStmts != 5 || report.CoveredStmts != 0 {
		t.Fatalf("expected 5 total, 0 covered, got total %d covered %d", report.TotalStmts, report.CoveredStmts)
	}
	if len(report.Uncovered) != 1 || report.Uncovered[0] != "pkg/foo.go:1.1,2.1" {
		t.Fatalf("expected uncovered slice with 1 element, got %v", report.Uncovered)
	}

	// Covered block (count > 0)
	processCoverageLine("pkg/foo.go:3.1,4.1 3 2", nil, report)
	if report.TotalStmts != 8 || report.CoveredStmts != 3 {
		t.Fatalf("expected 8 total, 3 covered, got total %d covered %d", report.TotalStmts, report.CoveredStmts)
	}
}

func TestExtractFilePath(t *testing.T) {
	// With colon
	pathWithColon := extractFilePath("Dendrite/pkg/foo.go:10.1,20.2")
	if pathWithColon != "Dendrite/pkg/foo.go" {
		t.Errorf("expected Dendrite/pkg/foo.go, got %s", pathWithColon)
	}

	// Without colon
	pathWithoutColon := extractFilePath("Dendrite/pkg/foo.go")
	if pathWithoutColon != "Dendrite/pkg/foo.go" {
		t.Errorf("expected Dendrite/pkg/foo.go, got %s", pathWithoutColon)
	}
}

func TestFinalizeReport_ZeroStatementsAndPercentages(t *testing.T) {
	// Zero statements package
	repZero := &CoverageReport{
		Packages: map[string]PackageCoverage{
			"pkg/empty": {
				Package:      "pkg/empty",
				TotalStmts:   0,
				CoveredStmts: 0,
			},
		},
	}
	finalizeReport(repZero)
	if repZero.Percentage != 100.0 {
		t.Errorf("expected report percentage 100.0, got %f", repZero.Percentage)
	}
	if repZero.Packages["pkg/empty"].Percentage != 100.0 {
		t.Errorf("expected pkg percentage 100.0, got %f", repZero.Packages["pkg/empty"].Percentage)
	}

	// Non-zero statements package
	repNonZero := &CoverageReport{
		TotalStmts:   10,
		CoveredStmts: 7,
		Packages: map[string]PackageCoverage{
			"pkg/sub": {
				Package:      "pkg/sub",
				TotalStmts:   10,
				CoveredStmts: 7,
			},
		},
	}
	finalizeReport(repNonZero)
	if repNonZero.Percentage != 70.0 {
		t.Errorf("expected 70.0%% coverage, got %f", repNonZero.Percentage)
	}
	if repNonZero.Packages["pkg/sub"].Percentage != 70.0 {
		t.Errorf("expected 70.0%% pkg coverage, got %f", repNonZero.Packages["pkg/sub"].Percentage)
	}
}

func TestMatchesSingleTarget(t *testing.T) {
	target := "pkg/alpha"

	// dir == target
	if !matchesSingleTarget("pkg/alpha", target) {
		t.Errorf("expected exact match to be true")
	}

	// dir has suffix /target
	if !matchesSingleTarget("Dendrite/pkg/alpha", target) {
		t.Errorf("expected suffix match to be true")
	}

	// dir has prefix target/
	if !matchesSingleTarget("pkg/alpha/sub", target) {
		t.Errorf("expected prefix match to be true")
	}

	// dir contains /target/
	if !matchesSingleTarget("repo/pkg/alpha/sub", target) {
		t.Errorf("expected contains match to be true")
	}

	// dir does not match
	if matchesSingleTarget("pkg/beta", target) {
		t.Errorf("expected non-match to be false")
	}
}
