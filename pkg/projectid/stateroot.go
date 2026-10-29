package projectid

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// StateRoot returns the checkout whose .dendrite/ directory holds the local state for dir.
// .dendrite/ is never committed, so a linked worktree (where .git is a file) has none of its
// own: it shares the state of the main checkout that owns its git directory. For a normal
// checkout, a directory without git, or when git cannot answer, dir itself is returned.
func StateRoot(dir string) string {
	info, err := os.Stat(filepath.Join(dir, ".git"))
	if err != nil || info.IsDir() {
		return dir
	}
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	if err != nil {
		return dir
	}
	common := strings.TrimSpace(string(out))
	if filepath.Base(common) != ".git" {
		return dir
	}
	return filepath.Dir(common)
}

// GitRootCommit returns the first commit of the repository at dir, the identity that stays the
// same across clones. A repository with several root commits yields the smallest SHA; "" when
// dir is not a repository or has no commit yet.
func GitRootCommit(dir string) string {
	out, err := exec.Command("git", "-C", dir, "rev-list", "--max-parents=0", "HEAD").Output()
	if err != nil {
		return ""
	}
	roots := strings.Fields(string(out))
	if len(roots) == 0 {
		return ""
	}
	sort.Strings(roots)
	return roots[0]
}
