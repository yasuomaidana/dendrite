package vcs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// ErrDetachedHead is returned when a pull is requested on a checkout with no branch.
var ErrDetachedHead = errors.New("vcs: detached HEAD has no branch to update")

// ErrDirtyWorktree is returned when a pull would run over uncommitted tracked changes.
var ErrDirtyWorktree = errors.New("vcs: worktree has uncommitted changes (commit, stash or pass autostash)")

// ErrOffTrackedBranch is returned when Enforce cannot move a checkout onto its tracked
// branch because it has uncommitted tracked changes.
var ErrOffTrackedBranch = errors.New("vcs: checkout is off its tracked branch and has uncommitted changes (commit or stash, then sync again)")

// WorkCopyDir is the directory holding model-owned working copies (dendrite work new);
// Enforce never switches a checkout inside it, since those sit on their own agent branches.
const WorkCopyDir = "dendrite-work"

// PendingBranch is a remote branch carrying commits the tracked target does not have yet.
type PendingBranch struct {
	Ref     string `json:"ref"`
	Commits int    `json:"commits"`
}

// RepoState is a snapshot of one checkout against the ref it tracks.
type RepoState struct {
	Dir      string `json:"dir"`
	Branch   string `json:"branch,omitempty"`
	Detached bool   `json:"detached,omitempty"`
	Head     string `json:"head"`
	// Target is the remote-tracking ref compared against (e.g. origin/develop); empty when none exists.
	Target string `json:"target,omitempty"`
	// OnTarget reports whether the checked-out branch is the branch Target tracks,
	// i.e. whether a fast-forward of the checkout would advance it.
	OnTarget   bool            `json:"on_target"`
	Ahead      int             `json:"ahead"`
	Behind     int             `json:"behind"`
	DirtyFiles int             `json:"dirty_files"`
	Pending    []PendingBranch `json:"pending,omitempty"`
	Updated    bool            `json:"updated"`
	Merged     string          `json:"merged,omitempty"`
	// Switched names the branch Enforce moved the checkout away from.
	Switched string   `json:"switched,omitempty"`
	Notes    []string `json:"notes,omitempty"`
}

// Dirty reports whether tracked files have uncommitted changes.
func (s *RepoState) Dirty() bool { return s.DirtyFiles > 0 }

// RepoSyncOptions configures InspectRepo and SyncRepo.
type RepoSyncOptions struct {
	// Remote is the remote name (default "origin").
	Remote string
	// Ref is the branch to track; empty follows the checkout's configured upstream,
	// falling back to <remote>/<current branch>.
	Ref string
	// NoFetch skips `git fetch` (offline inspection).
	NoFetch bool
	// Rebase pulls with --rebase instead of --ff-only.
	Rebase bool
	// Autostash stashes tracked changes around the pull instead of refusing.
	Autostash bool
	// Merge names a remote branch (with or without the "<remote>/" prefix) to merge
	// into the checkout after updating it; a conflicting merge is aborted.
	Merge string
	// Enforce switches a checkout that is not on the tracked branch Ref onto it (creating
	// the local branch from <remote>/<Ref> when missing) before updating. A checkout with
	// uncommitted tracked changes is refused with ErrOffTrackedBranch, and working copies
	// under WorkCopyDir are left alone. Without Ref, or when Ref is a tag or commit, it is a no-op.
	Enforce bool
}

func (o RepoSyncOptions) remote() string {
	if o.Remote == "" {
		return "origin"
	}
	return o.Remote
}

// RepoSync inspects and updates a checkout through a GitRunner.
type RepoSync struct {
	Runner GitRunner
}

// NewRepoSync returns a RepoSync backed by the system git binary.
func NewRepoSync() *RepoSync { return &RepoSync{Runner: &DefaultGitRunner{}} }

func (r *RepoSync) git(ctx context.Context, dir string, args ...string) (string, error) {
	runner := r.Runner
	if runner == nil {
		runner = &DefaultGitRunner{}
	}
	return runner.RunGit(ctx, dir, args...)
}

// Inspect fetches (unless opts.NoFetch) and reports where dir stands against its target,
// plus the remote branches that carry commits the target lacks. It never changes the checkout.
func (r *RepoSync) Inspect(ctx context.Context, dir string, opts RepoSyncOptions) (*RepoState, error) {
	st := &RepoState{Dir: dir}
	head, err := r.git(ctx, dir, "rev-parse", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("%s is not a git checkout with commits: %w", dir, err)
	}
	st.Head = head

	if branch, err := r.git(ctx, dir, "symbolic-ref", "--short", "-q", "HEAD"); err == nil && branch != "" {
		st.Branch = branch
	} else {
		st.Detached = true
	}

	if !opts.NoFetch {
		if _, err := r.git(ctx, dir, "fetch", "--prune", opts.remote()); err != nil {
			st.Notes = append(st.Notes, "fetch failed; using last known remote state: "+firstLine(err.Error()))
		}
	}

	st.Target, st.OnTarget = r.target(ctx, dir, st.Branch, opts)
	if st.Target != "" {
		counts, err := r.git(ctx, dir, "rev-list", "--left-right", "--count", "HEAD..."+st.Target)
		if err == nil {
			st.Ahead, st.Behind = parseCounts(counts)
		}
	}

	porcelain, err := r.git(ctx, dir, "status", "--porcelain", "--untracked-files=no")
	if err == nil {
		for _, line := range strings.Split(porcelain, "\n") {
			if strings.TrimSpace(line) != "" {
				st.DirtyFiles++
			}
		}
	}

	st.Pending = r.pending(ctx, dir, st.Target, opts.remote())
	return st, nil
}

// target resolves the remote-tracking ref to compare against and whether branch is the one it tracks.
func (r *RepoSync) target(ctx context.Context, dir, branch string, opts RepoSyncOptions) (string, bool) {
	remote := opts.remote()
	if opts.Ref != "" {
		ref := strings.TrimPrefix(opts.Ref, remote+"/")
		candidate := remote + "/" + ref
		if r.refExists(ctx, dir, "refs/remotes/"+candidate) {
			return candidate, branch == ref
		}
		// A tag or commit: compare against it directly; nothing to fast-forward onto.
		if r.refExists(ctx, dir, ref) {
			return ref, false
		}
		return "", false
	}
	if branch == "" {
		return "", false
	}
	if up, err := r.git(ctx, dir, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}"); err == nil && up != "" {
		return up, true
	}
	candidate := remote + "/" + branch
	if r.refExists(ctx, dir, "refs/remotes/"+candidate) {
		return candidate, true
	}
	return "", false
}

func (r *RepoSync) refExists(ctx context.Context, dir, ref string) bool {
	_, err := r.git(ctx, dir, "rev-parse", "--verify", "-q", ref+"^{commit}")
	return err == nil
}

// pending lists remote branches carrying commits that neither the checkout (HEAD) nor
// target contain, i.e. work published on other branches that has not reached this checkout.
func (r *RepoSync) pending(ctx context.Context, dir, target, remote string) []PendingBranch {
	exclude := []string{"^HEAD"}
	if target != "" {
		exclude = append(exclude, "^"+target)
	}
	out, err := r.git(ctx, dir, "for-each-ref", "--format=%(refname:short)", "refs/remotes/"+remote+"/")
	if err != nil || out == "" {
		return nil
	}
	var res []PendingBranch
	for _, ref := range strings.Split(out, "\n") {
		ref = strings.TrimSpace(ref)
		if ref == "" || ref == target || ref == remote || ref == remote+"/HEAD" {
			continue
		}
		n, err := r.git(ctx, dir, append([]string{"rev-list", "--count", ref}, exclude...)...)
		if err != nil {
			continue
		}
		if c, _ := strconv.Atoi(n); c > 0 {
			res = append(res, PendingBranch{Ref: ref, Commits: c})
		}
	}
	sort.Slice(res, func(i, j int) bool { return res[i].Ref < res[j].Ref })
	return res
}

// Sync inspects dir and, when the checkout is on its tracked branch and behind, updates it
// (fast-forward only unless opts.Rebase). It refuses on a detached HEAD and on uncommitted
// tracked changes unless opts.Autostash. With opts.Merge it then merges that remote branch,
// aborting the merge on conflict. The checkout's branch is only switched with opts.Enforce.
func (r *RepoSync) Sync(ctx context.Context, dir string, opts RepoSyncOptions) (*RepoState, error) {
	st, err := r.Inspect(ctx, dir, opts)
	if err != nil {
		return nil, err
	}
	if opts.Enforce {
		if st, err = r.enforce(ctx, dir, st, opts); err != nil {
			return st, err
		}
	}
	needPull := st.Behind > 0 && st.OnTarget
	if st.Behind > 0 && !st.OnTarget && st.Target != "" {
		st.Notes = append(st.Notes, fmt.Sprintf("checkout is on %q, not the tracked %s; not switching branches", displayBranch(st), st.Target))
	}
	if !needPull && opts.Merge == "" {
		return st, nil
	}
	if st.Detached {
		return st, ErrDetachedHead
	}
	if st.Dirty() && !opts.Autostash {
		return st, ErrDirtyWorktree
	}

	if needPull {
		args := []string{"merge", "--ff-only"}
		if opts.Rebase {
			args = []string{"rebase"}
		}
		if opts.Autostash {
			args = append(args, "--autostash")
		}
		args = append(args, st.Target)
		if _, err := r.git(ctx, dir, args...); err != nil {
			if opts.Rebase {
				_, _ = r.git(ctx, dir, "rebase", "--abort")
			}
			return st, fmt.Errorf("updating %s from %s: %w", displayBranch(st), st.Target, err)
		}
		st.Updated = true
	}

	if opts.Merge != "" {
		branch := opts.Merge
		if !strings.HasPrefix(branch, opts.remote()+"/") {
			branch = opts.remote() + "/" + branch
		}
		args := []string{"merge", "--no-edit"}
		if opts.Autostash {
			args = append(args, "--autostash")
		}
		if _, err := r.git(ctx, dir, append(args, branch)...); err != nil {
			_, _ = r.git(ctx, dir, "merge", "--abort")
			return st, fmt.Errorf("merging %s into %s (aborted): %w", branch, displayBranch(st), err)
		}
		st.Merged = branch
		st.Updated = true
	}

	if head, err := r.git(ctx, dir, "rev-parse", "HEAD"); err == nil {
		st.Head = head
	}
	if st.Target != "" {
		if counts, err := r.git(ctx, dir, "rev-list", "--left-right", "--count", "HEAD..."+st.Target); err == nil {
			st.Ahead, st.Behind = parseCounts(counts)
		}
	}
	st.Pending = r.pending(ctx, dir, st.Target, opts.remote())
	return st, nil
}

// Clone checks url out into dir on ref (a branch or tag; the remote's default branch when
// empty). A commit ref is cloned from the default branch and then checked out detached.
func (r *RepoSync) Clone(ctx context.Context, url, ref, dir string) error {
	args := []string{"clone"}
	if ref != "" && !isFullSHA(ref) {
		args = append(args, "--branch", ref)
	}
	if _, err := r.git(ctx, "", append(args, url, dir)...); err != nil {
		return fmt.Errorf("git clone %s (%s): %w", url, ref, err)
	}
	if isFullSHA(ref) {
		if _, err := r.git(ctx, dir, "checkout", "--detach", ref); err != nil {
			return fmt.Errorf("checking out %s in %s: %w", ref, dir, err)
		}
	}
	return nil
}

// Exclude adds pattern to the checkout's local exclude file (info/exclude, shared by its
// worktrees) unless it is already listed. Git treats excluded files as expendable, so a
// later checkout of a branch that tracks the same path replaces it instead of refusing.
func (r *RepoSync) Exclude(ctx context.Context, dir, pattern string) error {
	path, err := r.git(ctx, dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return err
	}
	path = filepath.Join(path, "info", "exclude")
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == pattern {
			return nil
		}
	}
	if len(data) > 0 && !strings.HasSuffix(string(data), "\n") {
		data = append(data, '\n')
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, []byte(pattern+"\n")...), 0o644)
}

// enforce moves dir onto its tracked branch when it sits elsewhere, and returns the
// re-inspected state. It does nothing when the checkout is already there, the tracked ref
// is not a remote branch, or dir is a working copy under WorkCopyDir.
func (r *RepoSync) enforce(ctx context.Context, dir string, st *RepoState, opts RepoSyncOptions) (*RepoState, error) {
	remote := opts.remote()
	ref := strings.TrimPrefix(opts.Ref, remote+"/")
	if ref == "" || st.OnTarget || st.Target != remote+"/"+ref {
		return st, nil
	}
	if inWorkCopy(dir) {
		st.Notes = append(st.Notes, fmt.Sprintf("working copy in %s/ stays on %q", WorkCopyDir, displayBranch(st)))
		return st, nil
	}
	if st.Dirty() {
		return st, fmt.Errorf("%w: on %q, tracks %s", ErrOffTrackedBranch, displayBranch(st), st.Target)
	}
	from := displayBranch(st)
	args := []string{"switch", ref}
	if !r.refExists(ctx, dir, "refs/heads/"+ref) {
		args = []string{"switch", "-c", ref, "--track", st.Target}
	}
	if _, err := r.git(ctx, dir, args...); err != nil {
		return st, fmt.Errorf("switching %s from %q to %q: %w", dir, from, ref, err)
	}
	next, err := r.Inspect(ctx, dir, RepoSyncOptions{Remote: opts.Remote, Ref: opts.Ref, NoFetch: true})
	if err != nil {
		return st, err
	}
	next.Switched = from
	next.Notes = append(st.Notes, fmt.Sprintf("switched from %q to tracked branch %q", from, ref))
	return next, nil
}

// inWorkCopy reports whether dir lies inside a WorkCopyDir directory.
func inWorkCopy(dir string) bool {
	for _, part := range strings.Split(filepath.ToSlash(filepath.Clean(dir)), "/") {
		if part == WorkCopyDir {
			return true
		}
	}
	return false
}

// ResolveRemoteRef returns the commit ref points to on the remote at url without a local
// checkout (`git ls-remote`). An empty ref resolves the remote's HEAD; a full 40-hex SHA
// is returned unchanged.
func (r *RepoSync) ResolveRemoteRef(ctx context.Context, url, ref string) (string, error) {
	if isFullSHA(ref) {
		return ref, nil
	}
	pattern := ref
	if pattern == "" {
		pattern = "HEAD"
	}
	out, err := r.git(ctx, "", "ls-remote", url, pattern, "refs/heads/"+pattern, "refs/tags/"+pattern, "refs/tags/"+pattern+"^{}")
	if err != nil {
		return "", fmt.Errorf("resolving %s at %s: %w", pattern, url, err)
	}
	// Prefer a peeled tag, then a branch, then whatever matched.
	var sha, branchSHA, peeledSHA string
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		switch {
		case strings.HasSuffix(fields[1], "^{}"):
			peeledSHA = fields[0]
		case strings.HasPrefix(fields[1], "refs/heads/"):
			branchSHA = fields[0]
		case sha == "":
			sha = fields[0]
		}
	}
	for _, s := range []string{peeledSHA, branchSHA, sha} {
		if s != "" {
			return s, nil
		}
	}
	return "", fmt.Errorf("ref %q not found at %s", pattern, url)
}

func isFullSHA(s string) bool {
	if len(s) != 40 {
		return false
	}
	for _, c := range s {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return false
		}
	}
	return true
}

func parseCounts(out string) (ahead, behind int) {
	parts := strings.Fields(out)
	if len(parts) >= 2 {
		ahead, _ = strconv.Atoi(parts[0])
		behind, _ = strconv.Atoi(parts[1])
	}
	return ahead, behind
}

func displayBranch(st *RepoState) string {
	if st.Detached {
		return "detached HEAD"
	}
	return st.Branch
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
