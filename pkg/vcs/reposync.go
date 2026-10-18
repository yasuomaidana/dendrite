package vcs

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// ErrDetachedHead is returned when a pull is requested on a checkout with no branch.
var ErrDetachedHead = errors.New("vcs: detached HEAD has no branch to update")

// ErrDirtyWorktree is returned when a pull would run over uncommitted tracked changes.
var ErrDirtyWorktree = errors.New("vcs: worktree has uncommitted changes (commit, stash or pass autostash)")

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
	Notes      []string        `json:"notes,omitempty"`
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
// aborting the merge on conflict. The checkout's branch is never switched.
func (r *RepoSync) Sync(ctx context.Context, dir string, opts RepoSyncOptions) (*RepoState, error) {
	st, err := r.Inspect(ctx, dir, opts)
	if err != nil {
		return nil, err
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
