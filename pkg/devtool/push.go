package devtool

import (
	"Dendrite/pkg/branchpolicy"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
)

// PushOutcome reports what a push attempt did.
type PushOutcome struct {
	Pushed  bool   `json:"pushed"`
	Skipped bool   `json:"skipped,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Remote  string `json:"remote,omitempty"`
	Branch  string `json:"branch,omitempty"`
	Err     error  `json:"-"`
}

// RepoState is a snapshot of a working copy relative to its remote.
type RepoState struct {
	Dir         string   `json:"dir"`
	Branch      string   `json:"branch"`
	Dirty       []string `json:"dirty,omitempty"`
	HasUpstream bool     `json:"has_upstream"`
	Ahead       int      `json:"ahead"`
	Behind      int      `json:"behind"`
	FetchError  string   `json:"fetch_error,omitempty"`
}

func newGitRunner(ctx context.Context, dir string) func(args ...string) (string, error) {
	return func(args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, "git", args...)
		if dir != "" {
			cmd.Dir = dir
		}
		var out, errOut bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &errOut
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("git %s failed: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(errOut.String()))
		}
		return strings.TrimSpace(out.String()), nil
	}
}

// PushCurrentBranch pushes the checked-out branch of repoDir with the same guard rails as
// `dendrite commit`: origin is preferred over other remotes, a missing upstream is created
// with -u, main/master and detached HEADs are refused, force is never used, and
// DENDRITE_COMMIT_PUSH=false disables pushing entirely.
func PushCurrentBranch(ctx context.Context, repoDir string) PushOutcome {
	if strings.EqualFold(os.Getenv("DENDRITE_COMMIT_PUSH"), "false") {
		return PushOutcome{Skipped: true, Reason: "DENDRITE_COMMIT_PUSH=false"}
	}
	return pushBranch(newGitRunner(ctx, repoDir), pushRemoteFor(repoDir))
}

// pushRemoteFor is the remote the project's synapse member says branches go to (a fork); "" means the default
// choice, origin.
func pushRemoteFor(repoDir string) string {
	if repoDir == "" {
		repoDir = "."
	}
	if r := branchpolicy.For(repoDir).PushRemote; r != branchpolicy.DefaultRemote {
		return r
	}
	return ""
}

// remoteNames lists the repository's remotes; none when git cannot say.
func remoteNames(runGit func(args ...string) (string, error)) []string {
	out, err := runGit("remote")
	if err != nil {
		return nil
	}
	var remotes []string
	for _, r := range strings.Split(out, "\n") {
		if r = strings.TrimSpace(r); r != "" {
			remotes = append(remotes, r)
		}
	}
	return remotes
}

// branchRefusal is the outcome that stops a push of branch (detached HEAD, main, master), or nil.
func branchRefusal(branch string) *PushOutcome {
	switch branch {
	case "HEAD", "":
		return &PushOutcome{Skipped: true, Reason: "detached HEAD; nothing to push"}
	case "main", "master":
		refuseErr := fmt.Errorf("pushing %s directly is refused (branch guard)", branch)
		return &PushOutcome{Skipped: true, Reason: refuseErr.Error(), Branch: branch, Err: refuseErr}
	}
	return nil
}

// firstPresent is the first wanted remote that exists, or the first remote there is.
func firstPresent(remotes []string, wanted ...string) string {
	for _, w := range wanted {
		if w != "" && slices.Contains(remotes, w) {
			return w
		}
	}
	return remotes[0]
}

// pushBranch pushes the current branch. prefer names the remote to use when the repository has it; otherwise
// origin is preferred over the other remotes.
func pushBranch(runGit func(args ...string) (string, error), prefer string) PushOutcome {
	remotes := remoteNames(runGit)
	if len(remotes) == 0 {
		return PushOutcome{Skipped: true, Reason: "no remote configured"}
	}
	branch, _ := runGit("rev-parse", "--abbrev-ref", "HEAD")
	branch = strings.TrimSpace(branch)
	if refused := branchRefusal(branch); refused != nil {
		return *refused
	}

	targetRemote := firstPresent(remotes, prefer, "origin")

	out := PushOutcome{Branch: branch, Remote: targetRemote}
	var pushErr error
	// The destination is always the branch's own name: a bare "git push" follows the upstream and
	// push.default, so a branch tracking origin/main would land on main past the guard above.
	refspec := "HEAD:refs/heads/" + branch
	if _, uErr := runGit("rev-parse", "--abbrev-ref", "@{u}"); uErr == nil {
		if tracked, cErr := runGit("config", "--get", "branch."+branch+".remote"); cErr == nil && slices.Contains(remotes, tracked) {
			targetRemote, out.Remote = tracked, tracked
		}
		_, pushErr = runGit("push", targetRemote, refspec)
	} else {
		_, pushErr = runGit("push", "-u", targetRemote, refspec)
	}
	if pushErr != nil {
		out.Err = pushErr
		return out
	}
	out.Pushed = true
	return out
}

// InspectRepo reports the branch, uncommitted paths and ahead/behind counts of repoDir.
// With fetch, remotes are fetched first so Behind reflects the latest remote state; a fetch
// failure is recorded rather than returned. Without an upstream, Ahead counts commits that
// exist on no remote.
func InspectRepo(ctx context.Context, repoDir string, fetch bool) (*RepoState, error) {
	runGit := newGitRunner(ctx, repoDir)
	st := &RepoState{Dir: repoDir}

	// symbolic-ref also works on an unborn branch; rev-parse covers a detached HEAD.
	branch, err := runGit("symbolic-ref", "--short", "-q", "HEAD")
	if err != nil {
		if branch, err = runGit("rev-parse", "--abbrev-ref", "HEAD"); err != nil {
			return nil, err
		}
	}
	st.Branch = branch

	porcelain, err := runGit("status", "--porcelain")
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(porcelain, "\n") {
		if len(line) > 3 {
			st.Dirty = append(st.Dirty, strings.TrimSpace(line[3:]))
		}
	}

	if fetch {
		if _, ferr := runGit("fetch", "--all", "--prune", "--quiet"); ferr != nil {
			st.FetchError = ferr.Error()
		}
	}

	if _, uErr := runGit("rev-parse", "--abbrev-ref", "@{u}"); uErr == nil {
		st.HasUpstream = true
		counts, err := runGit("rev-list", "--left-right", "--count", "@{u}...HEAD")
		if err != nil {
			return nil, err
		}
		if f := strings.Fields(counts); len(f) == 2 {
			st.Behind, _ = strconv.Atoi(f[0])
			st.Ahead, _ = strconv.Atoi(f[1])
		}
		return st, nil
	}

	if _, hErr := runGit("rev-parse", "--verify", "--quiet", "HEAD"); hErr != nil {
		return st, nil // unborn branch: nothing committed yet
	}
	n, err := runGit("rev-list", "--count", "HEAD", "--not", "--remotes")
	if err != nil {
		return nil, err
	}
	st.Ahead, _ = strconv.Atoi(n)
	return st, nil
}
