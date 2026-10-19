package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMyelin(t *testing.T) {
	repo := setupCommitRepo(t)
	if out, code := cliOutput(t, "myelin", "check"); code != ExitOK || !strings.Contains(out, "VCS Guard Active") {
		t.Fatalf("check on a feature branch: exit %d\n%s", code, out)
	}
	runGitT(t, repo, "checkout", "-q", "-b", "main")
	if out, code := cliOutput(t, "vcs", "guard"); code != ExitError || !strings.Contains(out, "VCS Guard Alert") {
		t.Fatalf("check on main: exit %d\n%s", code, out)
	}
	if out, code := cliOutput(t, "myelin", "evacuate"); code != ExitOK || !strings.Contains(out, "evacuated to develop") {
		t.Fatalf("evacuate: exit %d\n%s", code, out)
	}
	if out, code := cliOutput(t, "myelin", "sync-remote"); code == ExitUsage || !strings.Contains(out, "synchronizing remote fixes") {
		t.Fatalf("sync-remote: exit %d\n%s", code, out)
	}
	if _, code := cliOutput(t, "sync-remote"); code != ExitUsage {
		t.Fatalf("top-level sync-remote is gone: exit %d", code)
	}
}

func TestMyelin_GitHubSyncWithoutGH(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if out, code := cliOutput(t, "myelin", "github-sync", "--repo", "o/r"); code == ExitUsage || !strings.Contains(out, "Synchronizing Sequenced P0 Blockers to GitHub Issues (o/r)") {
		t.Fatalf("github-sync: exit %d\n%s", code, out)
	}
	if _, code := cliOutput(t, "github-sync"); code != ExitUsage {
		t.Fatalf("top-level github-sync is gone: exit %d", code)
	}
}

func TestMyelin_HITLSyncWithGH(t *testing.T) {
	bin := t.TempDir()
	installTestExecutableScript(t, bin, "gh", "echo '{\"state\":\"OPEN\",\"comments\":[]}'\n")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	var buf strings.Builder
	if err := runHITLSync(t.Context(), "o/r", &buf); err != nil && !strings.Contains(buf.String(), "Polling GitHub Issues") {
		t.Fatalf("hitl: %v\n%s", err, buf.String())
	}
	if len(p0Blockers()) != 5 || filepath.Base(p0Blockers()[0].ID) != "DEN-701" {
		t.Fatal("unexpected P0 blocker chain")
	}
}

func TestPrune(t *testing.T) {
	setupCommitRepo(t)
	if out, code := cliOutput(t, "prune", "--dry-run"); code != ExitOK || !strings.Contains(out, "Audit Process Completed") {
		t.Fatalf("prune --dry-run: exit %d\n%s", code, out)
	}
	if out, code := cliOutput(t, "audit-changes", "--branch", "local/x", "--dry-run", "--json"); code == ExitUsage {
		t.Fatalf("prune --branch: usage error\n%s", out)
	}
	if _, code := cliOutput(t, "prune", "--max-tokens", "0"); code != ExitUsage {
		t.Fatalf("--max-tokens 0: exit %d", code)
	}
}
