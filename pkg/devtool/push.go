package devtool

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
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
	return pushBranch(newGitRunner(ctx, repoDir))
}

func pushBranch(runGit func(args ...string) (string, error)) PushOutcome {
	remotesOut, err := runGit("remote")
	if err != nil {
		return PushOutcome{Skipped: true, Reason: "no remote configured"}
	}
	var remotes []string
	for _, r := range strings.Split(remotesOut, "\n") {
		if r = strings.TrimSpace(r); r != "" {
			remotes = append(remotes, r)
		}
	}
	if len(remotes) == 0 {
		return PushOutcome{Skipped: true, Reason: "no remote configured"}
	}

	branch, _ := runGit("rev-parse", "--abbrev-ref", "HEAD")
	branch = strings.TrimSpace(branch)
	if branch == "HEAD" || branch == "" {
		return PushOutcome{Skipped: true, Reason: "detached HEAD; nothing to push"}
	}
	if branch == "main" || branch == "master" {
		refuseErr := fmt.Errorf("pushing %s directly is refused (branch guard)", branch)
		return PushOutcome{Skipped: true, Reason: refuseErr.Error(), Branch: branch, Err: refuseErr}
	}

	targetRemote := remotes[0]
	for _, r := range remotes {
		if r == "origin" {
			targetRemote = "origin"
			break
		}
	}

	out := PushOutcome{Branch: branch, Remote: targetRemote}
	var pushErr error
	if _, uErr := runGit("rev-parse", "--abbrev-ref", "@{u}"); uErr == nil {
		_, pushErr = runGit("push")
	} else {
		_, pushErr = runGit("push", "-u", targetRemote, branch)
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
