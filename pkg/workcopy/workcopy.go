// Package workcopy manages the model-owned working copies of a repository.
//
// Every isolated checkout an agent session works in (and every story-fork
// variant) is a git worktree placed under a single directory, dendrite-work/,
// next to the main checkout. This keeps model-made copies apart from human
// projects and lets Dendrite delete a copy once its work is merged and clean.
package workcopy

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

const (
	// DirName is the directory, sibling to the main checkout, holding every working copy.
	DirName = "dendrite-work"
	// EnvWorkDir overrides the working-copy root directory.
	EnvWorkDir = "DENDRITE_WORK_DIR"
	// metaDirName holds per-copy metadata inside the root.
	metaDirName = ".meta"
	// storiesSuffix names the per-repo directory holding story-fork variants.
	storiesSuffix = "-stories"
)

// mergeTargets are the integration branches a copy's branch is checked against.
var mergeTargets = []string{"main", "master", "develop"}

var unsafeNameChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

var (
	// isTestBinary is a seam: under `go test` without an explicit root, story
	// variants never land in the developer's real dendrite-work/.
	isTestBinary = testing.Testing
	// evalSymlinks is a seam so Canon's fallback for unresolvable paths is testable.
	evalSymlinks = filepath.EvalSymlinks
)

// GitRunner executes a git sub-command in dir and returns its trimmed stdout.
type GitRunner func(ctx context.Context, dir string, args ...string) (string, error)

// Copy describes one working copy under the root.
type Copy struct {
	Path         string `json:"path"`
	Branch       string `json:"branch"`
	Head         string `json:"head"`
	Dirty        bool   `json:"dirty"`
	Ahead        int    `json:"ahead"`
	MergedInto   string `json:"merged_into,omitempty"`
	UpstreamGone bool   `json:"upstream_gone"`
	Untouched    bool   `json:"untouched"`
	Story        bool   `json:"story"`
}

// Report records what happened to a copy during gc, done or adopt.
type Report struct {
	Path   string `json:"path"`
	Branch string `json:"branch"`
	Action string `json:"action"`
	Reason string `json:"reason"`
}

// GCOptions tunes a garbage-collection or adopt run.
type GCOptions struct {
	DryRun bool
	// InUse reports whether a copy path is referenced by an in-flight agent job.
	InUse func(path string) bool
}

// RemoveOptions tunes the removal of a single copy.
type RemoveOptions struct {
	// Force removes the copy even when it has uncommitted or unpushed work.
	Force bool
}

// meta is persisted per copy so gc can tell an untouched copy from a merged one.
type meta struct {
	Branch  string    `json:"branch"`
	Base    string    `json:"base"`
	Created time.Time `json:"created"`
}

// layout is the resolved location of a repository's working copies.
type layout struct {
	top      string // main checkout
	root     string // dendrite-work/
	repoName string
}

func (l layout) copyPath(name string) string {
	return filepath.Join(l.root, l.repoName+"-"+SanitizeName(name))
}

func (l layout) storiesDir() string {
	return filepath.Join(l.root, l.repoName+storiesSuffix)
}

// Manager creates, lists and removes the working copies of one repository.
type Manager struct {
	repoDir string
	git     GitRunner
	getwd   func() (string, error)
}

// New returns a Manager for the repository containing repoDir.
func New(repoDir string) *Manager {
	if repoDir == "" {
		repoDir = "."
	}
	return &Manager{repoDir: repoDir, git: RunGit, getwd: os.Getwd}
}

// RunGit is the default GitRunner, shelling out to the git CLI.
func RunGit(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s failed: %w (stderr: %s)", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

// layout resolves the main checkout (so commands run from inside a copy still
// find the same root) and the working-copy root ($DENDRITE_WORK_DIR overrides).
func (m *Manager) layout(ctx context.Context) (layout, error) {
	common, err := m.git(ctx, m.repoDir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return layout{}, err
	}
	top := Canon(filepath.Dir(common))
	root := filepath.Join(filepath.Dir(top), DirName)
	if dir := os.Getenv(EnvWorkDir); dir != "" {
		abs, _ := filepath.Abs(dir)
		root = Canon(abs)
	}
	return layout{top: top, root: root, repoName: filepath.Base(top)}, nil
}

// MainTop returns the top-level directory of the main (non-linked) worktree.
func (m *Manager) MainTop(ctx context.Context) (string, error) {
	l, err := m.layout(ctx)
	return l.top, err
}

// Root returns the directory holding every working copy of this repository.
func (m *Manager) Root(ctx context.Context) (string, error) {
	l, err := m.layout(ctx)
	return l.root, err
}

// StoriesDir returns the base directory for story-fork variant worktrees.
func (m *Manager) StoriesDir(ctx context.Context) (string, error) {
	l, err := m.layout(ctx)
	if err != nil {
		return "", err
	}
	return l.storiesDir(), nil
}

// StoriesDirFor returns the story-variant base directory for the repository at
// repoDir, or fallback when it cannot be resolved (outside a git repository, or
// inside a test binary with no explicit $DENDRITE_WORK_DIR).
func StoriesDirFor(repoDir, fallback string) string {
	if os.Getenv(EnvWorkDir) == "" && isTestBinary() {
		return fallback
	}
	dir, err := New(repoDir).StoriesDir(context.Background())
	if err != nil {
		return fallback
	}
	return dir
}

// SanitizeName turns an arbitrary label (ticket, story) into a directory-safe name.
func SanitizeName(name string) string {
	clean := strings.Trim(unsafeNameChars.ReplaceAllString(strings.TrimSpace(name), "-"), "-.")
	if clean == "" {
		return "copy"
	}
	return clean
}

// Create adds a new working copy called name on branch (default agent/<name>)
// starting at base (default HEAD), and returns its path.
func (m *Manager) Create(ctx context.Context, name, branch, base string) (string, error) {
	l, err := m.layout(ctx)
	if err != nil {
		return "", err
	}
	path := l.copyPath(name)
	if _, statErr := os.Stat(path); statErr == nil {
		return "", fmt.Errorf("workcopy: %s already exists", path)
	}
	if branch == "" {
		branch = "agent/" + strings.ToLower(SanitizeName(name))
	}
	if base == "" {
		base = "HEAD"
	}
	baseCommit, err := m.git(ctx, m.repoDir, "rev-parse", "--verify", base+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("workcopy: resolve base %q: %w", base, err)
	}
	if err := os.MkdirAll(l.root, 0o755); err != nil {
		return "", err
	}
	if _, err := m.git(ctx, m.repoDir, "worktree", "add", "-b", branch, path, baseCommit); err != nil {
		return "", fmt.Errorf("workcopy: %w", err)
	}
	if err := writeMeta(path, meta{Branch: branch, Base: baseCommit, Created: time.Now().UTC()}); err != nil {
		return path, fmt.Errorf("workcopy: copy created but metadata not saved: %w", err)
	}
	return path, nil
}

type worktreeEntry struct {
	path   string
	head   string
	branch string
}

func (m *Manager) worktrees(ctx context.Context) ([]worktreeEntry, error) {
	out, err := m.git(ctx, m.repoDir, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	var entries []worktreeEntry
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			entries = append(entries, worktreeEntry{path: Canon(strings.TrimPrefix(line, "worktree "))})
		case strings.HasPrefix(line, "HEAD "):
			entries[len(entries)-1].head = strings.TrimPrefix(line, "HEAD ")
		case strings.HasPrefix(line, "branch "):
			entries[len(entries)-1].branch = strings.TrimPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/")
		}
	}
	return entries, nil
}

// List returns every working copy under the root with its status.
func (m *Manager) List(ctx context.Context) ([]Copy, error) {
	copies, _, err := m.list(ctx)
	return copies, err
}

func (m *Manager) list(ctx context.Context) ([]Copy, layout, error) {
	l, err := m.layout(ctx)
	if err != nil {
		return nil, l, err
	}
	entries, err := m.worktrees(ctx)
	if err != nil {
		return nil, l, err
	}
	var copies []Copy
	for _, e := range entries {
		if !isWithin(l.root, e.path) {
			continue
		}
		c := m.inspect(ctx, e)
		c.Story = isWithin(l.storiesDir(), e.path)
		copies = append(copies, c)
	}
	return copies, l, nil
}

func (m *Manager) inspect(ctx context.Context, e worktreeEntry) Copy {
	c := Copy{Path: e.path, Branch: e.branch, Head: e.head}
	status, err := m.git(ctx, e.path, "status", "--porcelain", "--untracked-files=all")
	// Unreadable state is never treated as safe to delete.
	c.Dirty = err != nil || hasUserChanges(status)
	if md, err := readMeta(e.path); err == nil && md.Base == e.head {
		c.Untouched = true
	}
	c.MergedInto = m.mergedInto(ctx, e.head)
	if e.branch == "" {
		return c // detached HEAD: no upstream to inspect
	}
	track, _ := m.git(ctx, m.repoDir, "for-each-ref", "--format=%(upstream:track)", "refs/heads/"+e.branch)
	c.UpstreamGone = strings.Contains(track, "[gone]")
	if ahead, err := m.git(ctx, e.path, "rev-list", "--count", "@{upstream}..HEAD"); err == nil && !c.UpstreamGone {
		fmt.Sscanf(ahead, "%d", &c.Ahead)
	}
	return c
}

// hasUserChanges reports porcelain output other than Dendrite's own untracked runtime state.
func hasUserChanges(status string) bool {
	for _, line := range strings.Split(status, "\n") {
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "?? .dendrite/") {
			continue
		}
		return true
	}
	return false
}

func (m *Manager) mergedInto(ctx context.Context, head string) string {
	for _, target := range mergeTargets {
		for _, ref := range []string{"refs/heads/" + target, "refs/remotes/origin/" + target} {
			if _, err := m.git(ctx, m.repoDir, "rev-parse", "--verify", "--quiet", ref); err != nil {
				continue
			}
			if _, err := m.git(ctx, m.repoDir, "merge-base", "--is-ancestor", head, ref); err == nil {
				return target
			}
		}
	}
	return ""
}

// Resolve maps a copy name (as given to Create) or path to an existing copy.
func (m *Manager) Resolve(ctx context.Context, target string) (Copy, error) {
	c, _, err := m.resolve(ctx, target)
	return c, err
}

func (m *Manager) resolve(ctx context.Context, target string) (Copy, layout, error) {
	copies, l, err := m.list(ctx)
	if err != nil {
		return Copy{}, l, err
	}
	abs, _ := filepath.Abs(target)
	for _, c := range copies {
		if c.Path == Canon(abs) || c.Path == l.copyPath(target) {
			return c, l, nil
		}
	}
	return Copy{}, l, fmt.Errorf("workcopy: no working copy matches %q", target)
}

// Remove deletes one copy. Without Force it refuses copies with uncommitted or
// unpushed work; the branch is deleted only when its commits are merged (or Force).
func (m *Manager) Remove(ctx context.Context, target string, opts RemoveOptions) (Report, error) {
	c, l, err := m.resolve(ctx, target)
	if err != nil {
		return Report{}, err
	}
	if !opts.Force && c.Dirty {
		return Report{}, fmt.Errorf("workcopy: %s has uncommitted changes (use --force)", c.Path)
	}
	if !opts.Force && c.Ahead > 0 {
		return Report{}, fmt.Errorf("workcopy: %s has %d unpushed commit(s) (use --force)", c.Path, c.Ahead)
	}
	deleteBranch := opts.Force || c.MergedInto != "" || c.UpstreamGone || c.Untouched
	reason := "removed on request"
	if !deleteBranch {
		reason = "removed on request; branch " + c.Branch + " kept (not merged)"
	}
	if err := m.removeCopy(ctx, l, c, deleteBranch); err != nil {
		return Report{}, err
	}
	return Report{Path: c.Path, Branch: c.Branch, Action: "removed", Reason: reason}, nil
}

// removeCopy deletes a copy whose safety the caller has already checked. Untracked
// runtime state (.dendrite/) does not count as work, so git is told to drop it.
func (m *Manager) removeCopy(ctx context.Context, l layout, c Copy, deleteBranch bool) error {
	if _, err := m.git(ctx, m.repoDir, "worktree", "remove", "--force", c.Path); err != nil {
		return fmt.Errorf("workcopy: %w", err)
	}
	_, _ = m.git(ctx, m.repoDir, "worktree", "prune")
	_ = os.Remove(metaPath(c.Path))
	_ = os.Remove(filepath.Join(l.root, metaDirName)) // only succeeds when empty
	_ = os.Remove(l.root)
	if deleteBranch && c.Branch != "" {
		if _, err := m.git(ctx, m.repoDir, "branch", "-D", c.Branch); err != nil {
			return fmt.Errorf("workcopy: copy removed but branch not deleted: %w", err)
		}
	}
	return nil
}

// GC removes every copy whose work is merged (or whose upstream branch is gone)
// and that has no uncommitted or unpushed work. Story variants, the copy the
// current process runs in, and copies used by in-flight jobs are always kept.
func (m *Manager) GC(ctx context.Context, opts GCOptions) ([]Report, error) {
	copies, l, err := m.list(ctx)
	if err != nil {
		return nil, err
	}
	cwd := m.cwd()
	var reports []Report
	for _, c := range copies {
		keep, reason := gcVerdict(c, cwd, opts.InUse)
		r := Report{Path: c.Path, Branch: c.Branch, Action: "removed", Reason: reason}
		switch {
		case keep:
			r.Action = "kept"
		case opts.DryRun:
			r.Action = "would remove"
		default:
			if err := m.removeCopy(ctx, l, c, true); err != nil {
				r.Action, r.Reason = "kept", err.Error()
			}
		}
		reports = append(reports, r)
	}
	return reports, nil
}

// cwd returns the canonical working directory, or "" when it cannot be read.
func (m *Manager) cwd() string {
	wd, err := m.getwd()
	if err != nil {
		return ""
	}
	return Canon(wd)
}

// gcVerdict decides whether a copy is kept, and why.
func gcVerdict(c Copy, cwd string, inUse func(string) bool) (bool, string) {
	switch {
	case c.Story:
		return true, "story variant (managed by dendrite story)"
	case cwd != "" && isWithin(c.Path, cwd):
		return true, "current working directory"
	case inUse != nil && inUse(c.Path):
		return true, "used by an in-flight agent job"
	case c.Dirty:
		return true, "uncommitted changes"
	case c.Ahead > 0:
		return true, fmt.Sprintf("%d unpushed commit(s)", c.Ahead)
	case c.UpstreamGone:
		return false, "upstream branch deleted"
	case c.Untouched:
		return true, "no commits yet"
	case c.MergedInto != "":
		return false, "merged into " + c.MergedInto
	default:
		return true, "not merged"
	}
}
