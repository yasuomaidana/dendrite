package main

import (
	"Dendrite/cmd/dendrite/internal/cli"
	"strings"
	"testing"
)

func TestMyelin(t *testing.T) {
	repo := setupCommitRepo(t)
	if out, code := cliOutput(t, "myelin", "check"); code != cli.ExitOK || !strings.Contains(out, "VCS Guard Active") {
		t.Fatalf("check on a feature branch: exit %d\n%s", code, out)
	}
	runGitT(t, repo, "checkout", "-q", "-b", "main")
	if out, code := cliOutput(t, "vcs", "guard"); code != cli.ExitError || !strings.Contains(out, "VCS Guard Alert") {
		t.Fatalf("check on main: exit %d\n%s", code, out)
	}
	if out, code := cliOutput(t, "myelin", "evacuate"); code != cli.ExitOK || !strings.Contains(out, "evacuated to develop") {
		t.Fatalf("evacuate: exit %d\n%s", code, out)
	}
	if out, code := cliOutput(t, "myelin", "sync-remote"); code == cli.ExitUsage || !strings.Contains(out, "synchronizing remote fixes") {
		t.Fatalf("sync-remote: exit %d\n%s", code, out)
	}
	if _, code := cliOutput(t, "sync-remote"); code != cli.ExitUsage {
		t.Fatalf("top-level sync-remote is gone: exit %d", code)
	}
}

func TestMyelin_GitHubSyncWiring(t *testing.T) {
	offlineSyncer(t)
	if out, code := cliOutput(t, "myelin", "github-sync", "--repo", "fail/me"); code == cli.ExitUsage || code == cli.ExitOK {
		t.Fatalf("github-sync with no Plane must fail at runtime: exit %d\n%s", code, out)
	}
	if out, code := cliOutput(t, "quality", "hitl-sync", "--dry-run"); code != cli.ExitOK || !strings.Contains(out, "Human-in-the-Loop verification") {
		t.Fatalf("quality hitl-sync alias: exit %d\n%s", code, out)
	}
	if _, code := cliOutput(t, "github-sync"); code != cli.ExitUsage {
		t.Fatalf("top-level github-sync is gone: exit %d", code)
	}
}

func TestPrune(t *testing.T) {
	setupCommitRepo(t)
	if out, code := cliOutput(t, "prune", "--dry-run"); code != cli.ExitOK || !strings.Contains(out, "Audit Process Completed") {
		t.Fatalf("prune --dry-run: exit %d\n%s", code, out)
	}
	if out, code := cliOutput(t, "audit-changes", "--branch", "local/x", "--dry-run", "--json"); code == cli.ExitUsage {
		t.Fatalf("prune --branch: usage error\n%s", out)
	}
	if _, code := cliOutput(t, "prune", "--max-tokens", "0"); code != cli.ExitUsage {
		t.Fatalf("--max-tokens 0: exit %d", code)
	}
}
