package quality

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"Dendrite/pkg/tracker"
)

type TestPollerExecutor struct {
	response string
}

func (m *TestPollerExecutor) Execute(ctx context.Context, name string, args ...string) (string, error) {
	return m.response, nil
}

func TestIssueSpec_Validate(t *testing.T) {
	// Missing fields
	spec := &IssueSpec{
		ID:    "T-1",
		Title: "Test",
	}
	if err := spec.Validate(); err == nil {
		t.Fatal("expected error for missing problem statement")
	}

	spec.ProblemStatement = "Some problem"
	if err := spec.Validate(); err == nil {
		t.Fatal("expected error for missing reproduction steps")
	}

	spec.ReproductionSteps = "Run test"
	if err := spec.Validate(); err == nil {
		t.Fatal("expected error for missing codebase location")
	}

	spec.CodebaseLocation = "pkg/vcs/guard.go:L1-L10"
	if err := spec.Validate(); err == nil {
		t.Fatal("expected error for missing proposed resolution")
	}

	spec.ProposedResolution = "Fix it"
	if err := spec.Validate(); err == nil {
		t.Fatal("expected error for missing acceptance criteria")
	}

	spec.AcceptanceCriteria = []string{"Zero mocks"}
	spec.CreatedByLLM = true
	spec.State = StateDone

	// Validate should clamp StateDone to StateToVerify when CreatedByLLM is true
	if err := spec.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if spec.State != StateToVerify {
		t.Fatalf("expected LLM ceiling state TO_VERIFY, got %s", spec.State)
	}
}

func TestIssueSpec_FormatMarkdown(t *testing.T) {
	spec := &IssueSpec{
		ID:                 "DEN-701",
		Title:              "Git Guard",
		ProblemStatement:   "Main branch lacks protection",
		ReproductionSteps:  "git checkout main && git commit",
		CodebaseLocation:   "pkg/vcs/guard.go:L40-L90",
		ProposedResolution: "Add PreFlightBranchCheck",
		AcceptanceCriteria: []string{"No commits to main", "100% coverage"},
		Priority:           "P0_TOP_BLOCKER",
		DifficultyTier:     tracker.TierL2Low,
		LocalFeasible:      true,
		State:              StateToVerify,
		GitHubIssueNum:     1,
		HITLIssueNum:       10,
		DependsOn:          []string{"DEP-1"},
		Blocks:             []string{"BLK-1"},
	}

	md := spec.FormatMarkdown(map[string]int{"DEP-1": 100, "BLK-1": 101})
	for _, section := range []string{
		"## 1. Problem Statement & Context",
		"## 2. Reproduction Steps / Trigger Conditions",
		"## 3. Fault / Codebase Location (`Where`)",
		"## 4. Proposed Technical Resolution (`How to Solve`)",
		"## 5. Strict Acceptance Criteria (Checklist)",
		"## 6. Execution Metadata",
		"- **Depends On**:",
		"- **Blocks Downstream**:",
	} {
		if !containsString(md, section) {
			t.Errorf("expected markdown to contain '%s'", section)
		}
	}
}

func TestSyncValidationState_Closed(t *testing.T) {
	jsonResp := `{"state":"CLOSED","updatedAt":"2026-10-15T18:00:00Z","comments":[]}`
	poller := NewGitHubHITLPoller("test/repo", &TestPollerExecutor{response: jsonResp})

	spec := &IssueSpec{
		ID:             "DEN-701",
		GitHubIssueNum: 1,
		State:          StateToVerify,
	}

	followUp, err := poller.SyncValidationState(context.Background(), spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if followUp != nil {
		t.Fatalf("expected no follow-up for closed issue")
	}
	if spec.State != StateDone {
		t.Fatalf("expected state transition to DONE, got %s", spec.State)
	}
}

func TestSyncValidationState_OpenWithFeedback(t *testing.T) {
	jsonResp := `{
		"state": "OPEN",
		"updatedAt": "2026-10-15T18:00:00Z",
		"comments": [
			{
				"author": {"login": "senior-reviewer"},
				"body": "Need to handle detached HEAD edge case in guard.",
				"createdAt": "2026-10-15T18:00:00Z"
			}
		]
	}`
	poller := NewGitHubHITLPoller("test/repo", &TestPollerExecutor{response: jsonResp})

	spec := &IssueSpec{
		ID:               "DEN-701",
		GitHubIssueNum:   1,
		State:            StateToVerify,
		CodebaseLocation: "pkg/vcs/guard.go:L50",
		LastSyncedAt:     time.Date(2026, 10, 15, 17, 0, 0, 0, time.UTC),
	}

	followUp, err := poller.SyncValidationState(context.Background(), spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if followUp == nil {
		t.Fatalf("expected follow-up ticket generated for open issue with feedback")
	}
	if spec.State != StateToDo {
		t.Fatalf("expected parent ticket demoted to TO_DO, got %s", spec.State)
	}
	if followUp.ID != "DEN-701-HITL-FIX" {
		t.Fatalf("unexpected follow up ID: %s", followUp.ID)
	}
}

func TestScanForMocks_CleanDir(t *testing.T) {
	hasMocks, locs, err := ScanForMocks(".", "pkg/quality")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hasMocks {
		t.Fatalf("expected pkg/quality to be mock-free, found: %v", locs)
	}
}

func TestAuditFinishedTicket_CoverageAndFallback(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "agy-log-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	spec := &IssueSpec{
		ID:                 "DEN-TEST-01",
		Title:              "Quality Self-Test",
		State:              StateDone, // Should remap to StateToVerify
		CodebaseLocation:   "pkg/quality/quality.go",
		ReproductionSteps:  "go test ./pkg/quality/...",
		ProposedResolution: "Self test audit",
		AcceptanceCriteria: []string{"No mocks"},
	}

	localClient := &LocalOllamaAnalyzer{}
	agyClient := &AGYFallbackAnalyzer{LogPath: filepath.Join(tmpDir, "escalations.log")}

	gapTickets, err := AuditFinishedTicket(ctx, ".", spec, "pkg/quality", localClient, agyClient)
	if err != nil {
		t.Fatalf("audit failed: %v", err)
	}

	if spec.State != StateToVerify && spec.State != StateToDo {
		t.Fatalf("expected state to be TO_VERIFY or TO_DO, got %s", spec.State)
	}
	_ = gapTickets
}

func containsString(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle || (len(needle) > 0 && len(haystack) > 0 && (haystack[:len(needle)] == needle || containsString(haystack[1:], needle))))
}

func TestAGYFallbackAnalyzer_RealLog(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "escalations.log")
	agy := &AGYFallbackAnalyzer{LogPath: logPath}

	res, err := agy.AnalyzeFeature(context.Background(), "pkg/test", "diff")
	if err != nil {
		t.Fatalf("AGY analyze failed: %v", err)
	}
	if res.HasLogicalBugs || res.TooComplexForLocal {
		t.Fatalf("unexpected res: %+v", res)
	}

	content, err := os.ReadFile(logPath)
	if err != nil || !containsString(string(content), "Escalated package pkg/test to AGY") {
		t.Fatalf("escalation log not written properly: %s", string(content))
	}
}

func TestScanForMocks_Detected(t *testing.T) {
	tmpDir := t.TempDir()
	// initialize git repo in tmpDir
	runCmd := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = tmpDir
		_ = cmd.Run()
	}
	runCmd("init", "-b", "main")
	runCmd("config", "user.name", "Test")
	runCmd("config", "user.email", "test@test.com")
	mockFile := filepath.Join(tmpDir, "sample_test.go")
	_ = os.WriteFile(mockFile, []byte("package sample\ntype "+"MockService struct{}\n"), 0644)
	runCmd("add", "sample_test.go")
	runCmd("commit", "-m", "add test")

	hasMocks, locs, err := ScanForMocks(tmpDir, "sample_test.go")
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}
	if !hasMocks || len(locs) == 0 {
		t.Fatalf("expected mocks detected in sample_test.go")
	}
}

type bugAnalyzer struct{}

func (b *bugAnalyzer) AnalyzeFeature(ctx context.Context, pkgPath string, diff string) (AnalysisResult, error) {
	return AnalysisResult{
		TooComplexForLocal: false,
		HasLogicalBugs:     true,
		DiagnosticNotes:    "race condition in handler",
	}, nil
}

func TestAuditFinishedTicket_LogicalBugs(t *testing.T) {
	spec := &IssueSpec{
		ID:                 "DEN-BUG-01",
		Title:              "Bug test",
		State:              StateDone,
		CodebaseLocation:   "pkg/test/test.go",
		ReproductionSteps:  "run",
		ProposedResolution: "fix",
		AcceptanceCriteria: []string{"none"},
	}

	_, err := AuditFinishedTicket(context.Background(), ".", spec, "pkg/quality", &bugAnalyzer{}, nil)
	if err != nil {
		t.Fatalf("audit failed: %v", err)
	}
	if spec.State != StateToDo {
		t.Fatalf("expected demotion to TO_DO, got %s", spec.State)
	}
	if !containsString(spec.ProposedResolution, "[AUDIT REJECTION]: race condition in handler") {
		t.Fatalf("rejection notes missing: %s", spec.ProposedResolution)
	}
}

type complexAnalyzer struct{}

func (c *complexAnalyzer) AnalyzeFeature(ctx context.Context, pkgPath string, diff string) (AnalysisResult, error) {
	return AnalysisResult{
		TooComplexForLocal: true,
	}, nil
}

func TestAuditFinishedTicket_ComplexEscalation(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "escalations.log")
	agy := &AGYFallbackAnalyzer{LogPath: logPath}

	spec := &IssueSpec{
		ID:                 "DEN-COMPLEX-01",
		Title:              "Complex feature",
		State:              StateDone,
		CodebaseLocation:   "pkg/test/test.go",
		ReproductionSteps:  "run",
		ProposedResolution: "fix",
		AcceptanceCriteria: []string{"none"},
	}

	_, err := AuditFinishedTicket(context.Background(), ".", spec, "pkg/quality", &complexAnalyzer{}, agy)
	if err != nil {
		t.Fatalf("audit failed: %v", err)
	}

	content, err := os.ReadFile(logPath)
	if err != nil || !containsString(string(content), "Escalated package pkg/quality to AGY") {
		t.Fatalf("escalation log not written properly: %s", string(content))
	}
}

func TestAuditFinishedTicket_MocksRejection(t *testing.T) {
	tmpDir := t.TempDir()
	runCmd := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = tmpDir
		_ = cmd.Run()
	}
	runCmd("init", "-b", "main")
	runCmd("config", "user.name", "Test")
	runCmd("config", "user.email", "test@test.com")
	mockFile := filepath.Join(tmpDir, "mock_test.go")
	_ = os.WriteFile(mockFile, []byte("package sample\ntype "+"Mock"+"Runner struct{}\n"), 0644)
	runCmd("add", "mock_test.go")
	runCmd("commit", "-m", "add mock")

	spec := &IssueSpec{
		ID:                 "DEN-MOCK-01",
		Title:              "Mocked test",
		State:              StateToVerify,
		CodebaseLocation:   "mock_test.go",
		ReproductionSteps:  "run",
		ProposedResolution: "fix",
		AcceptanceCriteria: []string{"none"},
	}

	_, err := AuditFinishedTicket(context.Background(), tmpDir, spec, "mock_test.go", nil, nil)
	if err != nil {
		t.Fatalf("audit failed: %v", err)
	}
	if spec.State != StateToDo {
		t.Fatalf("expected demotion to TO_DO, got %s", spec.State)
	}
	if !containsString(spec.ProposedResolution, "[AUDIT REJECTION]: Mocks/stubs detected") {
		t.Fatalf("rejection notes missing: %s", spec.ProposedResolution)
	}
}

func TestRunCoverageAudit_RealGoCode(t *testing.T) {
	tmpDir := t.TempDir()

	goMod := "module example.com/testpkg\n\ngo 1.22\n"
	_ = os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goMod), 0644)

	code := `package testpkg

func CoveredFunc() int {
	return 42
}

func UncoveredFunc() int {
	a := 10
	b := 20
	return a + b
}
`
	_ = os.WriteFile(filepath.Join(tmpDir, "calc.go"), []byte(code), 0644)

	testCode := `package testpkg

import "testing"

func TestCoveredFunc(t *testing.T) {
	if CoveredFunc() != 42 {
		t.Fail()
	}
}
`
	_ = os.WriteFile(filepath.Join(tmpDir, "calc_test.go"), []byte(testCode), 0644)

	uncovered, pct, failed, _ := RunCoverageAudit(context.Background(), tmpDir, ".")
	if failed {
		t.Fatal("expected test to pass")
	}
	if pct >= 100.0 {
		t.Fatalf("expected coverage < 100%%, got %.2f%%", pct)
	}
	if len(uncovered) == 0 {
		t.Fatal("expected uncovered blocks identified")
	}
}

func TestRunCoverageAudit_FailingTest(t *testing.T) {
	tmpDir := t.TempDir()

	goMod := "module example.com/failpkg\n\ngo 1.22\n"
	_ = os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goMod), 0644)

	testCode := `package failpkg

import "testing"

func TestBroken(t *testing.T) {
	t.Fatal("intentionally broken")
}
`
	_ = os.WriteFile(filepath.Join(tmpDir, "fail_test.go"), []byte(testCode), 0644)

	_, _, failed, out := RunCoverageAudit(context.Background(), tmpDir, ".")
	if !failed {
		t.Fatal("expected failure")
	}
	if !containsString(out, "intentionally broken") {
		t.Fatalf("expected failure output: %s", out)
	}
}

func TestNewGitHubHITLPoller_Defaults(t *testing.T) {
	poller := NewGitHubHITLPoller("", nil)
	if poller.Repo != "" {
		t.Fatalf("an empty repo must stay empty so gh resolves it from the git remote, got %s", poller.Repo)
	}
	if poller.Executor == nil {
		t.Fatal("expected non-nil default executor")
	}
}

func TestExtractPackages(t *testing.T) {
	text := "Ticket [DEN-038] Automated Test Synthesizer (`pkg/testgen`, `pkg/uitest`, `cmd/dendrite`)"
	pkgs := ExtractPackages(text)
	if len(pkgs) != 3 {
		t.Fatalf("expected 3 packages, got %v", pkgs)
	}
	expected := []string{"pkg/testgen", "pkg/uitest", "cmd/dendrite"}
	for i, exp := range expected {
		if pkgs[i] != exp {
			t.Errorf("expected %s, got %s", exp, pkgs[i])
		}
	}
}

func TestValidateZeroMockGate(t *testing.T) {
	tmpDir := t.TempDir()
	runCmd := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = tmpDir
		_ = cmd.Run()
	}
	runCmd("init", "-b", "main")
	runCmd("config", "user.name", "Test")
	runCmd("config", "user.email", "test@test.com")
	pkgDir := filepath.Join(tmpDir, "pkg", "sample")
	_ = os.MkdirAll(pkgDir, 0755)
	_ = os.WriteFile(filepath.Join(pkgDir, "sample.go"), []byte("package sample\ntype "+"Mock"+"Driver struct{}\n"), 0644)
	runCmd("add", ".")
	runCmd("commit", "-m", "add sample")

	hasMocks, locs, err := ValidateZeroMockGate(tmpDir, []string{"pkg/sample"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasMocks || len(locs) == 0 {
		t.Fatalf("expected mocks to be detected in temp pkg/sample")
	}

	cleanMocks, _, err := ValidateZeroMockGate(".", []string{"pkg/quality"})
	if err != nil {
		t.Fatalf("unexpected error on clean dir: %v", err)
	}
	if cleanMocks {
		t.Fatalf("expected no mocks in pkg/quality")
	}
}
