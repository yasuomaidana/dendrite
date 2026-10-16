package audit

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/tracker/github"
	"Dendrite/pkg/vcs"
)

func setupRealAuditRepo(t *testing.T) (repoDir string, originDir string) {
	t.Helper()
	tmp := t.TempDir()
	originDir = filepath.Join(tmp, "origin.git")
	repoDir = filepath.Join(tmp, "repo")

	runGit(t, originDir, "init", "--bare", "-b", "develop")
	runGit(t, tmp, "clone", originDir, repoDir)
	runGit(t, repoDir, "config", "user.name", "Audit Agent")
	runGit(t, repoDir, "config", "user.email", "audit@dendrite.ai")

	readme := filepath.Join(repoDir, "README.md")
	_ = os.WriteFile(readme, []byte("# Audit Repo\n"), 0644)
	runGit(t, repoDir, "add", "README.md")
	runGit(t, repoDir, "commit", "-m", "initial commit on develop")
	runGit(t, repoDir, "push", "origin", "develop")

	return repoDir, originDir
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	_ = os.MkdirAll(dir, 0755)
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s failed: %v\nOutput: %s", args, dir, err, string(out))
	}
	return string(out)
}

func TestOrderTopological(t *testing.T) {
	items := []*vcs.AuditQueueItem{
		{Branch: "local/child", BaseBranch: "local/parent"},
		{Branch: "local/parent", BaseBranch: "develop"},
		{Branch: "local/independent", BaseBranch: "develop"},
	}

	ordered := OrderTopological(items)
	if len(ordered) != 3 {
		t.Fatalf("expected 3 items, got %d", len(ordered))
	}

	parentIdx := -1
	childIdx := -1
	for idx, it := range ordered {
		if it.Branch == "local/parent" {
			parentIdx = idx
		}
		if it.Branch == "local/child" {
			childIdx = idx
		}
	}
	if parentIdx >= childIdx {
		t.Fatalf("expected parent before child, got parentIdx=%d, childIdx=%d", parentIdx, childIdx)
	}
}

func TestAuditor_AuditChanges_DryRun_RealGit(t *testing.T) {
	repoDir, _ := setupRealAuditRepo(t)
	ctx := context.Background()

	// Create local branch with file
	runGit(t, repoDir, "checkout", "-b", "local/mod-a", "develop")
	f := filepath.Join(repoDir, "feature.go")
	_ = os.WriteFile(f, []byte("package mod_a\n"), 0644)
	runGit(t, repoDir, "add", "feature.go")
	runGit(t, repoDir, "commit", "-m", "feat(mod-a): new feature")
	runGit(t, repoDir, "push", "-u", "origin", "local/mod-a")
	runGit(t, repoDir, "checkout", "develop")

	guard := vcs.NewGitGuard(repoDir, vcs.ModeAuditChanges)
	manifest := filepath.Join(repoDir, ".dendrite", "audit_queue.json")
	orch := vcs.NewStackedBranchOrchestrator(guard, manifest)

	_ = orch.SaveManifest([]*vcs.AuditQueueItem{
		{Branch: "local/mod-a", BaseBranch: "develop", Status: vcs.StatusAwaitingAudit},
	})

	auditor := NewAuditor(guard, orch, github.NewGitHubClient("test/repo"))
	summary, err := auditor.AuditChanges(ctx, AuditOptions{DryRun: true})
	if err != nil {
		t.Fatalf("unexpected audit error: %v", err)
	}
	if summary.BranchesAudited != 1 {
		t.Fatalf("expected 1 branch audited, got %d", summary.BranchesAudited)
	}
	if summary.BranchesMerged != 0 {
		t.Fatalf("expected 0 branches merged in dry-run, got %d", summary.BranchesMerged)
	}
}

func TestAuditor_AuditChanges_Execution_RealGit(t *testing.T) {
	repoDir, originDir := setupRealAuditRepo(t)
	ctx := context.Background()

	// 1. Create local/mod-1 off develop
	runGit(t, repoDir, "checkout", "-b", "local/mod-1", "develop")
	f1 := filepath.Join(repoDir, "file1.txt")
	_ = os.WriteFile(f1, []byte("content 1\n"), 0644)
	runGit(t, repoDir, "add", "file1.txt")
	runGit(t, repoDir, "commit", "-m", "feat(mod-1): add file1")
	runGit(t, repoDir, "push", "-u", "origin", "local/mod-1")

	// 2. Create local/mod-2 stacked on local/mod-1
	runGit(t, repoDir, "checkout", "-b", "local/mod-2", "local/mod-1")
	f2 := filepath.Join(repoDir, "file2.txt")
	_ = os.WriteFile(f2, []byte("content 2\n"), 0644)
	runGit(t, repoDir, "add", "file2.txt")
	runGit(t, repoDir, "commit", "-m", "feat(mod-2): add file2")
	runGit(t, repoDir, "push", "-u", "origin", "local/mod-2")
	runGit(t, repoDir, "checkout", "develop")

	guard := vcs.NewGitGuard(repoDir, vcs.ModeAuditChanges)
	manifest := filepath.Join(repoDir, ".dendrite", "audit_queue.json")
	orch := vcs.NewStackedBranchOrchestrator(guard, manifest)

	_ = orch.SaveManifest([]*vcs.AuditQueueItem{
		{Branch: "local/mod-1", BaseBranch: "develop", Status: vcs.StatusAwaitingAudit},
		{Branch: "local/mod-2", BaseBranch: "local/mod-1", Status: vcs.StatusAwaitingAudit},
	})

	auditor := NewAuditor(guard, orch, github.NewGitHubClient("test/repo"))
	summary, err := auditor.AuditChanges(ctx, AuditOptions{DryRun: false})
	if err != nil {
		t.Fatalf("audit failed: %v", err)
	}
	if summary.BranchesMerged != 2 {
		t.Fatalf("expected 2 branches merged, got %d", summary.BranchesMerged)
	}

	// Verify both branches merged into develop on origin
	logOut := runGit(t, originDir, "log", "--oneline", "develop")
	if !contains(logOut, "validate and merge local/mod-1") || !contains(logOut, "validate and merge local/mod-2") {
		t.Fatalf("expected both merges on origin develop: %s", logOut)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle || (len(needle) > 0 && len(haystack) > 0 && (haystack[:len(needle)] == needle || contains(haystack[1:], needle))))
}

func TestAuditor_TargetBranchFilter(t *testing.T) {
	repoDir, _ := setupRealAuditRepo(t)
	ctx := context.Background()

	runGit(t, repoDir, "checkout", "-b", "local/mod-x", "develop")
	f := filepath.Join(repoDir, "x.txt")
	_ = os.WriteFile(f, []byte("x\n"), 0644)
	runGit(t, repoDir, "add", "x.txt")
	runGit(t, repoDir, "commit", "-m", "feat(mod-x): add x")
	runGit(t, repoDir, "push", "-u", "origin", "local/mod-x")
	runGit(t, repoDir, "checkout", "develop")

	guard := vcs.NewGitGuard(repoDir, vcs.ModeAuditChanges)
	manifest := filepath.Join(repoDir, ".dendrite", "audit_queue.json")
	orch := vcs.NewStackedBranchOrchestrator(guard, manifest)

	_ = orch.SaveManifest([]*vcs.AuditQueueItem{
		{Branch: "local/mod-x", BaseBranch: "develop", Status: vcs.StatusAwaitingAudit},
	})

	auditor := NewAuditor(guard, nil, nil)
	summary, err := auditor.AuditChanges(ctx, AuditOptions{TargetBranch: "mod-x", DryRun: false})
	if err != nil {
		t.Fatalf("audit failed: %v", err)
	}
	if summary.BranchesMerged != 1 {
		t.Fatalf("expected 1 merged, got %d", summary.BranchesMerged)
	}
}

func TestAuditor_LoadManifest_Error(t *testing.T) {
	repoDir, _ := setupRealAuditRepo(t)
	ctx := context.Background()

	guard := vcs.NewGitGuard(repoDir, vcs.ModeAuditChanges)
	manifest := filepath.Join(repoDir, ".dendrite", "audit_queue.json")
	_ = os.MkdirAll(filepath.Dir(manifest), 0755)
	_ = os.WriteFile(manifest, []byte("invalid JSON format"), 0644)
	orch := vcs.NewStackedBranchOrchestrator(guard, manifest)

	auditor := NewAuditor(guard, orch, nil)
	summary, err := auditor.AuditChanges(ctx, AuditOptions{})
	if err == nil {
		t.Fatalf("expected LoadManifest error, got nil")
	}
	if summary != nil {
		t.Fatalf("expected nil summary on error, got %v", summary)
	}
}

func TestAuditor_AuditChanges_StatusMerged_Skip(t *testing.T) {
	repoDir, _ := setupRealAuditRepo(t)
	ctx := context.Background()

	guard := vcs.NewGitGuard(repoDir, vcs.ModeAuditChanges)
	manifest := filepath.Join(repoDir, ".dendrite", "audit_queue.json")
	orch := vcs.NewStackedBranchOrchestrator(guard, manifest)

	_ = orch.SaveManifest([]*vcs.AuditQueueItem{
		{Branch: "local/mod-done", BaseBranch: "develop", Status: vcs.StatusMerged},
	})

	auditor := NewAuditor(guard, orch, nil)
	summary, err := auditor.AuditChanges(ctx, AuditOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.BranchesAudited != 0 {
		t.Fatalf("expected 0 branches audited because status was merged, got %d", summary.BranchesAudited)
	}
}

func TestAuditor_AuditChanges_AuditBranch_Error(t *testing.T) {
	repoDir, _ := setupRealAuditRepo(t)
	ctx := context.Background()

	guard := vcs.NewGitGuard(repoDir, vcs.ModeAuditChanges)
	manifest := filepath.Join(repoDir, ".dendrite", "audit_queue.json")
	orch := vcs.NewStackedBranchOrchestrator(guard, manifest)

	_ = orch.SaveManifest([]*vcs.AuditQueueItem{
		{Branch: "local/does-not-exist", BaseBranch: "develop", Status: vcs.StatusAwaitingAudit},
	})

	auditor := NewAuditor(guard, orch, nil)
	summary, err := auditor.AuditChanges(ctx, AuditOptions{})
	if err == nil {
		t.Fatalf("expected audit error on nonexistent branch, got nil")
	}
	if summary == nil || summary.Success {
		t.Fatalf("expected summary.Success to be false")
	}
}

func TestAuditor_RebaseDownstream_DirectDevelop_And_Errors(t *testing.T) {
	repoDir, _ := setupRealAuditRepo(t)
	ctx := context.Background()

	runGit(t, repoDir, "checkout", "-b", "local/sub-branch", "develop")
	f := filepath.Join(repoDir, "sub.txt")
	_ = os.WriteFile(f, []byte("sub\n"), 0644)
	runGit(t, repoDir, "add", "sub.txt")
	runGit(t, repoDir, "commit", "-m", "feat: sub commit")
	runGit(t, repoDir, "push", "-u", "origin", "local/sub-branch")

	guard := vcs.NewGitGuard(repoDir, vcs.ModeAuditChanges)
	auditor := NewAuditor(guard, nil, nil)

	// Case 1: empty oldParentSHA (rebases develop directly)
	err := auditor.RebaseDownstream(ctx, "local/sub-branch", "")
	if err != nil {
		t.Fatalf("expected successful rebase with empty parent sha: %v", err)
	}

	// Case 1b: non-empty oldParentSHA (rebases --onto develop oldParentSHA childBranch)
	headSHA := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	err = auditor.RebaseDownstream(ctx, "local/sub-branch", headSHA)
	if err != nil {
		t.Fatalf("expected successful rebase with non-empty parent sha: %v", err)
	}

	// Case 2: nonexistent branch error
	err = auditor.RebaseDownstream(ctx, "local/ghost-branch", "")
	if err == nil {
		t.Fatalf("expected checkout failure for ghost-branch, got nil")
	}
}

func TestAuditor_AuditBranch_PushDivergence_AutoRebase(t *testing.T) {
	repoDir, originDir := setupRealAuditRepo(t)
	ctx := context.Background()

	// 1. Create local/feat-div in repoDir
	runGit(t, repoDir, "checkout", "-b", "local/feat-div", "develop")
	f1 := filepath.Join(repoDir, "f1.txt")
	_ = os.WriteFile(f1, []byte("feature 1\n"), 0644)
	runGit(t, repoDir, "add", "f1.txt")
	runGit(t, repoDir, "commit", "-m", "feat: feat-div")
	runGit(t, repoDir, "push", "-u", "origin", "local/feat-div")
	runGit(t, repoDir, "checkout", "develop")

	// 2. Clone a second repo and push a concurrent commit to develop
	tmp := t.TempDir()
	secondRepo := filepath.Join(tmp, "second")
	runGit(t, tmp, "clone", originDir, secondRepo)
	runGit(t, secondRepo, "config", "user.name", "Second Agent")
	runGit(t, secondRepo, "config", "user.email", "second@dendrite.ai")
	remoteFile := filepath.Join(secondRepo, "remote.txt")
	_ = os.WriteFile(remoteFile, []byte("remote\n"), 0644)
	runGit(t, secondRepo, "add", "remote.txt")
	runGit(t, secondRepo, "commit", "-m", "remote commit")
	runGit(t, secondRepo, "push", "origin", "develop")

	// 3. Now run AuditBranch on feat-div. Push will fail initially due to divergence, then auto pull --rebase & push!
	guard := vcs.NewGitGuard(repoDir, vcs.ModeAuditChanges)
	manifest := filepath.Join(repoDir, ".dendrite", "audit_queue.json")
	orch := vcs.NewStackedBranchOrchestrator(guard, manifest)
	auditor := NewAuditor(guard, orch, nil)

	item := &vcs.AuditQueueItem{
		Branch:     "local/feat-div",
		BaseBranch: "develop",
		Status:     vcs.StatusAwaitingAudit,
	}

	res, err := auditor.AuditBranch(ctx, item, AuditOptions{})
	if err != nil {
		t.Fatalf("expected divergence recovery in AuditBranch, got err: %v", err)
	}
	if !res.Merged {
		t.Fatalf("expected branch to be merged after divergence recovery")
	}
}

func TestAuditor_AuditBranch_FaultErrors(t *testing.T) {
	repoDir, _ := setupRealAuditRepo(t)
	ctx := context.Background()

	runGit(t, repoDir, "checkout", "-b", "local/feat-err", "develop")
	f := filepath.Join(repoDir, "err.txt")
	_ = os.WriteFile(f, []byte("err\n"), 0644)
	runGit(t, repoDir, "add", "err.txt")
	runGit(t, repoDir, "commit", "-m", "feat: err")
	runGit(t, repoDir, "push", "-u", "origin", "local/feat-err")
	runGit(t, repoDir, "checkout", "develop")

	item := &vcs.AuditQueueItem{
		Branch:     "local/feat-err",
		BaseBranch: "develop",
		Status:     vcs.StatusAwaitingAudit,
	}

	// Case 1: Status command failure
	runner1 := &interceptedGitRunner{
		base: &vcs.DefaultGitRunner{},
		failOnMatch: func(args []string) bool {
			return len(args) > 0 && args[0] == "status"
		},
	}
	guard1 := vcs.NewGitGuard(repoDir, vcs.ModeAuditChanges).WithRunner(runner1)
	auditor1 := NewAuditor(guard1, nil, nil)
	_, err := auditor1.AuditBranch(ctx, item, AuditOptions{})
	if err == nil || !strings.Contains(err.Error(), "git status error") {
		t.Fatalf("expected status error, got: %v", err)
	}

	// Case 2: Checkout develop failure
	runner2 := &interceptedGitRunner{
		base: &vcs.DefaultGitRunner{},
		failOnMatch: func(args []string) bool {
			return len(args) > 1 && args[0] == "checkout" && args[1] == "develop"
		},
	}
	guard2 := vcs.NewGitGuard(repoDir, vcs.ModeAuditChanges).WithRunner(runner2)
	auditor2 := NewAuditor(guard2, nil, nil)
	_, err = auditor2.AuditBranch(ctx, item, AuditOptions{})
	if err == nil || !strings.Contains(err.Error(), "failed to checkout develop") {
		t.Fatalf("expected checkout develop error, got: %v", err)
	}

	// Case 3: Merge failure
	runner3 := &interceptedGitRunner{
		base: &vcs.DefaultGitRunner{},
		failOnMatch: func(args []string) bool {
			return len(args) > 0 && args[0] == "merge"
		},
	}
	guard3 := vcs.NewGitGuard(repoDir, vcs.ModeAuditChanges).WithRunner(runner3)
	auditor3 := NewAuditor(guard3, nil, nil)
	_, err = auditor3.AuditBranch(ctx, item, AuditOptions{})
	if err == nil || !strings.Contains(err.Error(), "failed to merge") {
		t.Fatalf("expected merge error, got: %v", err)
	}
}

type interceptedGitRunner struct {
	base        vcs.GitRunner
	failOnMatch func(args []string) bool
}

func (r *interceptedGitRunner) RunGit(ctx context.Context, dir string, args ...string) (string, error) {
	if r.failOnMatch != nil && r.failOnMatch(args) {
		return "", errors.New("simulated git execution failure")
	}
	return r.base.RunGit(ctx, dir, args...)
}
