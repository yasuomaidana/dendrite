package audit

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/tracker/github"
	"Dendrite/pkg/vcs"
)

type mockAuditorRunner struct {
	history []string
}

func (m *mockAuditorRunner) RunGit(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := strings.Join(args, " ")
	m.history = append(m.history, cmd)
	if strings.HasPrefix(cmd, "diff") {
		return "diff --git a/file.go b/file.go\n+new line", nil
	}
	return "", nil
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

	// Verify local/parent comes before local/child
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

func TestAuditor_AuditChanges_DryRun(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "auditor-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	mock := &mockAuditorRunner{}
	guard := vcs.NewGitGuard(tmpDir, vcs.ModeAuditChanges).WithRunner(mock)
	manifest := filepath.Join(tmpDir, "queue.json")
	orch := vcs.NewStackedBranchOrchestrator(guard, manifest)

	_ = orch.SaveManifest([]*vcs.AuditQueueItem{
		{Branch: "local/mod-a", BaseBranch: "develop", Status: vcs.StatusAwaitingAudit},
	})

	auditor := NewAuditor(guard, orch, github.NewGitHubClient("test/repo"))
	summary, err := auditor.AuditChanges(context.Background(), AuditOptions{DryRun: true})
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

func TestAuditor_AuditChanges_Execution(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "auditor-exec-*")
	defer os.RemoveAll(tmpDir)

	mock := &mockAuditorRunner{}
	guard := vcs.NewGitGuard(tmpDir, vcs.ModeAuditChanges).WithRunner(mock)
	manifest := filepath.Join(tmpDir, "queue.json")
	orch := vcs.NewStackedBranchOrchestrator(guard, manifest)

	_ = orch.SaveManifest([]*vcs.AuditQueueItem{
		{Branch: "local/mod-1", BaseBranch: "develop", Status: vcs.StatusAwaitingAudit, HeadSHA: "headsha1"},
		{Branch: "local/mod-2", BaseBranch: "local/mod-1", Status: vcs.StatusAwaitingAudit},
	})

	auditor := NewAuditor(guard, orch, github.NewGitHubClient("test/repo"))
	summary, err := auditor.AuditChanges(context.Background(), AuditOptions{DryRun: false})
	if err != nil {
		t.Fatalf("audit failed: %v", err)
	}
	if summary.BranchesMerged != 2 {
		t.Fatalf("expected 2 branches merged, got %d", summary.BranchesMerged)
	}
}
