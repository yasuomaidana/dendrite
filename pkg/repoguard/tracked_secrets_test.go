// Package repoguard holds repository-level guards that run with go test.
package repoguard

import (
	"os/exec"
	"path"
	"strings"
	"testing"
)

// A .gitignore rule never untracks a file that is already tracked, and the commit tool stages
// edits to tracked files, so a secret file that slips in once keeps being committed. This test
// fails the build the moment any environment file with real values is tracked.
func TestNoEnvironmentFileIsTracked(t *testing.T) {
	out, err := exec.Command("git", "ls-files", "--", ":/").Output()
	if err != nil {
		t.Skipf("not a git checkout: %v", err)
	}
	for _, f := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		base := path.Base(f)
		isEnv := base == ".env" || strings.HasPrefix(base, ".env.")
		if isEnv && !strings.HasSuffix(base, ".example") && !strings.HasSuffix(base, ".sample") {
			t.Errorf("%s is tracked: run `git rm --cached %s` (it is gitignored, and its values are public once pushed)", f, f)
		}
	}
}
