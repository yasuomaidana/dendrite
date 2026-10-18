package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/devtool"
)

func TestCommitSynapsePeers(t *testing.T) {
	t.Setenv("DENDRITE_HOME", t.TempDir())
	t.Setenv("DENDRITE_COMMIT_PUSH", "")
	t.Setenv("PLANE_BASE_URL", "http://127.0.0.1:1")
	ctx := context.Background()
	base := devtool.CommitOptions{UseLocalModel: false, NoSmart: true, TicketID: "DEN-1", Message: "feat: only for current"}

	den, _ := syncTestRepo(t, "den")
	gitDir, gitBare := syncTestRepo(t, "git")
	rag, ragBare := syncTestRepo(t, "rag")
	saveTestSynapse(t, "mesh", map[string]string{"den": den, "git": gitDir, "rag": rag})

	// git: dirty; rag: clean but one commit ahead.
	syncTestFile(t, gitDir, "change.txt", false)
	syncTestFile(t, rag, "ahead.txt", true)

	var out bytes.Buffer
	// Report-only mode flags the dirty peer but still pushes the ahead one.
	err := commitSynapsePeers(ctx, &out, den, "", base, nil)
	if err == nil || !strings.Contains(out.String(), "git has 1 uncommitted path(s)") {
		t.Fatalf("report-only: err=%v\n%s", err, out.String())
	}
	if runGitT(t, ragBare, "rev-parse", "feature") != runGitT(t, rag, "rev-parse", "HEAD") {
		t.Error("ahead peer rag should have been pushed")
	}

	// Declined peer is skipped without error.
	out.Reset()
	if err := commitSynapsePeers(ctx, &out, den, "mesh", base, func(string, int) bool { return false }); err != nil {
		t.Fatalf("declined: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "git: skipped") || !strings.Contains(out.String(), "rag: clean") {
		t.Errorf("unexpected output:\n%s", out.String())
	}

	// Approved peer is committed with its own message (ticket/message not carried over) and pushed.
	out.Reset()
	if err := commitSynapsePeers(ctx, &out, den, "", base, linePrompter(strings.NewReader("a\n"), &out)); err != nil {
		t.Fatalf("approved: %v\n%s", err, out.String())
	}
	msg := runGitT(t, gitDir, "log", "-1", "--format=%s")
	if strings.Contains(msg, "DEN-1") || strings.Contains(msg, "only for current") {
		t.Errorf("peer commit must not reuse the current repo's ticket/message: %q", msg)
	}
	if runGitT(t, gitBare, "rev-parse", "feature") != runGitT(t, gitDir, "rev-parse", "HEAD") {
		t.Error("committed peer should be pushed")
	}

	// Peer behind upstream with local commits is reported, not pushed.
	other := filepath.Join(t.TempDir(), "other")
	runGitT(t, filepath.Dir(other), "clone", "-q", "-b", "feature", ragBare, other)
	runGitT(t, other, "config", "user.email", "o@x")
	runGitT(t, other, "config", "user.name", "o")
	syncTestFile(t, other, "remote.txt", true)
	runGitT(t, other, "push", "-q")
	runGitT(t, rag, "fetch", "-q")
	syncTestFile(t, rag, "local.txt", true)
	out.Reset()
	_ = commitSynapsePeers(ctx, &out, den, "", base, nil)
	if !strings.Contains(out.String(), "rag: 1 unpushed commit(s), 1 behind upstream") {
		t.Errorf("expected diverged report:\n%s", out.String())
	}

	// Broken peers need attention; a peer that is no longer a git repo is reported.
	if err := os.RemoveAll(filepath.Join(rag, ".git")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(gitDir); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := commitSynapsePeers(ctx, &out, den, "", base, nil); err == nil || !strings.Contains(err.Error(), "2 synapse peer(s)") {
		t.Fatalf("expected 2 peers needing attention, got %v\n%s", err, out.String())
	}

	// Standalone repo: no peers.
	solo, _ := syncTestRepo(t, "solo")
	out.Reset()
	if err := commitSynapsePeers(ctx, &out, solo, "", base, nil); err != nil || !strings.Contains(out.String(), "No synapse peers") {
		t.Fatalf("standalone: %v\n%s", err, out.String())
	}
	if err := commitSynapsePeers(ctx, &out, solo, "nope", base, nil); err == nil {
		t.Error("expected unknown synapse error")
	}
}

func TestLinePrompter(t *testing.T) {
	var out bytes.Buffer
	ask := linePrompter(strings.NewReader("y\nn\n\n"), &out)
	if !ask("a", 1) || ask("b", 2) || ask("c", 3) {
		t.Error("expected y, n, default-no")
	}
	if strings.Count(out.String(), "[y/N/a]") != 3 {
		t.Errorf("expected 3 prompts, got %q", out.String())
	}
	_ = stdinIsTerminal()
}
