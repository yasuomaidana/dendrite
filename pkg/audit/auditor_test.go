package audit

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
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
