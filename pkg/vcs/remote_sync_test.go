package vcs

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractTicketIDs(t *testing.T) {
	log := `
75eedc7 [DEN-701] chore: update files
a1b2c3d Fixes #42 - resolve memory leak
d4e5f6g [DEN-702] feat(tracker): add routing
`
	ids := ExtractTicketIDs(log)
	if len(ids) < 2 {
		t.Fatalf("expected at least 2 ticket IDs, got %d: %v", len(ids), ids)
	}

	has701 := false
	has702 := false
	for _, id := range ids {
		if id == "DEN-701" {
			has701 = true
		}
		if id == "DEN-702" {
			has702 = true
		}
	}
	if !has701 || !has702 {
		t.Fatalf("expected DEN-701 and DEN-702, got %v", ids)
	}
}

func TestRemoteSyncWatcher_FetchAndDetect_RealGit(t *testing.T) {
	repoDir, _ := setupRealRepo(t)
	ctx := context.Background()

	guard := NewGitGuard(repoDir, ModeStandard)
	watcher := NewRemoteSyncWatcher(guard, nil)

	status, err := watcher.FetchAndDetect(ctx, "develop")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.Behind != 0 || status.Ahead != 0 {
		t.Fatalf("expected 0/0, got ahead=%d behind=%d", status.Ahead, status.Behind)
	}
}

func TestRemoteSyncWatcher_SyncRemoteFixes_RealGit(t *testing.T) {
	repoDir, originDir := setupRealRepo(t)
	ctx := context.Background()

	// 1. Push a remote commit using a second clone
	tmp := t.TempDir()
	secondRepo := filepath.Join(tmp, "second_clone")
	runGitCmd(t, tmp, "clone", originDir, secondRepo)
	runGitCmd(t, secondRepo, "config", "user.name", "Remote Dev")
	runGitCmd(t, secondRepo, "config", "user.email", "remote@dendrite.ai")

	hotfixFile := filepath.Join(secondRepo, "hotfix.txt")
	_ = os.WriteFile(hotfixFile, []byte("hotfix applied remotely\n"), 0644)
	runGitCmd(t, secondRepo, "add", "hotfix.txt")
	runGitCmd(t, secondRepo, "commit", "-m", "[DEN-703] fix(remote): apply remote hotfix")
	runGitCmd(t, secondRepo, "push", "origin", "develop")

	// 2. In repoDir, detect divergence and sync fixes
	guard := NewGitGuard(repoDir, ModeStandard)
	watcher := NewRemoteSyncWatcher(guard, nil)

	status, err := watcher.SyncRemoteFixes(ctx, "develop")
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	if !status.Updated {
		t.Fatalf("expected updated = true")
	}
	if len(status.RemoteCommits) != 1 {
		t.Fatalf("expected 1 remote commit, got %d: %v", len(status.RemoteCommits), status.RemoteCommits)
	}
	if len(status.ReconciledTickets) != 1 || status.ReconciledTickets[0] != "DEN-703" {
		t.Fatalf("expected reconciled ticket DEN-703, got %v", status.ReconciledTickets)
	}
}
