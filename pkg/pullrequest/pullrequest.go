// Package pullrequest opens and reads pull requests through the GitHub CLI (`gh`). Finished work reaches a trunk
// that agents never push (main, master) only this way: Dendrite pushes the branch and proposes it, a person merges.
package pullrequest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// The states GitHub reports for a pull request.
const (
	StateOpen   = "OPEN"
	StateMerged = "MERGED"
	StateClosed = "CLOSED"
)

// ErrNone is returned by View when the branch has no pull request.
var ErrNone = errors.New("no pull request for this branch")

// Request is a pull request to open: Head proposed into Base, from the repository checked out in Dir.
type Request struct {
	Dir string
	// Remote is where PushAndOpen pushes Head before proposing it.
	Remote string
	Base   string
	Head   string
	Title  string
	Body   string
}

// PullRequest is a pull request as GitHub reports it.
type PullRequest struct {
	URL    string `json:"url"`
	Number int    `json:"number"`
	State  string `json:"state"`
	Base   string `json:"baseRefName"`
	// MergeCommit is the commit the base branch got when the pull request was merged.
	MergeCommit string `json:"mergeCommit"`
}

// Merged reports whether the pull request was merged.
func (p PullRequest) Merged() bool { return p.State == StateMerged }

// gh runs the GitHub CLI in dir and returns its standard output; a failure carries what it printed.
func gh(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("gh %s: %w: %s", strings.Join(args[:min(2, len(args))], " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

// View reads the pull request of branch head in the repository at dir: the open one, else the latest. It returns
// ErrNone when the branch has none.
func View(ctx context.Context, dir, head string) (PullRequest, error) {
	out, err := gh(ctx, dir, "pr", "view", head, "--json", "url,number,state,baseRefName,mergeCommit")
	if err != nil {
		if strings.Contains(err.Error(), "no pull requests found") {
			return PullRequest{}, ErrNone
		}
		return PullRequest{}, err
	}
	var raw struct {
		PullRequest
		MergeCommit *struct {
			OID string `json:"oid"`
		} `json:"mergeCommit"`
	}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return PullRequest{}, fmt.Errorf("gh pr view: unreadable answer: %w", err)
	}
	pr := raw.PullRequest
	if raw.MergeCommit != nil {
		pr.MergeCommit = raw.MergeCommit.OID
	}
	return pr, nil
}

// Open proposes r.Head into r.Base and returns the pull request. A branch that already has an open or merged
// pull request into the same base keeps it: nothing new is opened.
func Open(ctx context.Context, r Request) (PullRequest, error) {
	if pr, err := View(ctx, r.Dir, r.Head); err == nil && pr.State != StateClosed && pr.Base == r.Base {
		return pr, nil
	}
	url, err := gh(ctx, r.Dir, "pr", "create", "--base", r.Base, "--head", r.Head, "--title", r.Title, "--body", r.Body)
	if err != nil {
		return PullRequest{}, err
	}
	// gh prints the URL of the new pull request on its last line.
	lines := strings.Split(url, "\n")
	return PullRequest{URL: strings.TrimSpace(lines[len(lines)-1]), State: StateOpen, Base: r.Base}, nil
}

// PushAndOpen pushes r.Head to r.Remote and opens its pull request; it returns the URL.
func PushAndOpen(ctx context.Context, r Request) (string, error) {
	if out, err := exec.CommandContext(ctx, "git", "-C", r.Dir, "push", "-u", r.Remote, r.Head).CombinedOutput(); err != nil {
		line, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
		return "", fmt.Errorf("git push %s %s: %s", r.Remote, r.Head, line)
	}
	pr, err := Open(ctx, r)
	return pr.URL, err
}
