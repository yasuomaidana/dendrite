package bootstrap

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	// preCommitHookPath is the tracked git hook that applies the ticket lock to staged paths, so an agent that
	// does not go through Claude Code (AGY, Copilot, a script) cannot get around it by committing.
	preCommitHookPath = ".githooks/pre-commit"
	gitHooksDir       = ".githooks"
	hookBlockBegin    = "# >>> dendrite managed: ticket lock >>>"
	hookBlockEnd      = "# <<< dendrite managed: ticket lock <<<"
	// preCommitBlock fails closed: with no dendrite on PATH the commit is refused. `git commit --no-verify`
	// remains the human's deliberate escape hatch.
	preCommitBlock = hookBlockBegin + "\n" +
		"if command -v dendrite >/dev/null 2>&1; then\n" +
		"  dendrite hook pre-commit || exit 1\n" +
		"else\n" +
		"  echo \"blocked: dendrite is not on PATH, so this commit cannot be checked against a ticket claim\" >&2\n" +
		"  exit 1\n" +
		"fi\n" +
		hookBlockEnd + "\n"
)

// mergeManagedBlock returns the hook script with Dendrite's block in it: a new script when there is none, the
// block replaced in place when the script has one, and the block appended after the user's own commands
// otherwise. changed is false when the script already holds exactly this block.
func mergeManagedBlock(existing string) (merged string, changed bool) {
	if strings.TrimSpace(existing) == "" {
		return "#!/bin/sh\n" + preCommitBlock, true
	}
	if begin := strings.Index(existing, hookBlockBegin); begin >= 0 {
		if end := strings.Index(existing[begin:], hookBlockEnd); end >= 0 {
			stop := begin + end + len(hookBlockEnd)
			stop += len(existing[stop:]) - len(strings.TrimPrefix(existing[stop:], "\n"))
			merged = existing[:begin] + preCommitBlock + existing[stop:]
			return merged, merged != existing
		}
	}
	if !strings.HasSuffix(existing, "\n") {
		existing += "\n"
	}
	return existing + "\n" + preCommitBlock, true
}

// installGitHook writes .githooks/pre-commit (executable, tracked) and points git at it when the repository
// has no hooks path of its own. A repository with a different core.hooksPath keeps it; the note says what to add
// there. Directories that are not git repositories are left alone.
func installGitHook(root string, dryRun bool) (created, updated []string, note string, err error) {
	if _, statErr := os.Stat(filepath.Join(root, ".git")); statErr != nil {
		return nil, nil, "", nil
	}
	path := filepath.Join(root, filepath.FromSlash(preCommitHookPath))
	existing, _ := os.ReadFile(path)
	merged, changed := mergeManagedBlock(string(existing))
	if changed && !dryRun {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, nil, "", err
		}
		if err := os.WriteFile(path, []byte(merged), 0o755); err != nil {
			return nil, nil, "", err
		}
	}
	if !dryRun {
		if err := os.Chmod(path, 0o755); err != nil {
			return nil, nil, "", err
		}
	}
	switch {
	case changed && len(existing) == 0:
		created = append(created, preCommitHookPath)
	case changed:
		updated = append(updated, preCommitHookPath)
	}

	out, _ := exec.Command("git", "-C", root, "config", "--get", "core.hooksPath").Output()
	switch current := strings.TrimSpace(string(out)); current {
	case gitHooksDir:
	case "":
		if !dryRun {
			if err := exec.Command("git", "-C", root, "config", "core.hooksPath", gitHooksDir).Run(); err != nil {
				return created, updated, "", fmt.Errorf("set core.hooksPath: %w", err)
			}
		}
	default:
		note = fmt.Sprintf("core.hooksPath is %q, so git does not run %s here: add `dendrite hook pre-commit || exit 1` to %s/pre-commit", current, preCommitHookPath, current)
	}
	return created, updated, note, nil
}
