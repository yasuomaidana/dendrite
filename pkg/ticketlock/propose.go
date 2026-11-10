package ticketlock

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"Dendrite/pkg/branchpolicy"
	"Dendrite/pkg/pullrequest"
)

// Proposal is a finished branch on its way to a trunk that agents never push: it goes there by pull request.
type Proposal struct {
	// Dir is the working copy the branch is checked out in.
	Dir string
	// Remote is where the branch is pushed.
	Remote string
	// Base is the trunk the branch is proposed into; Head is the branch.
	Base, Head string
	// Title and Body describe the pull request.
	Title, Body string
}

// propose is the last stage of a clean finish for a branch that merges into main or master: the branch is pushed
// and a pull request is opened, and the ticket waits for a person to merge it. It reports whether the ticket has
// to wait. A pull request that could not be opened is a note that says how to open it, never a blocked ticket.
func (l *Lifecycle) propose(ctx context.Context, c FinishChecks, qc Context, res *FinishResult) bool {
	pol := branchpolicy.For(l.Dir)
	if c.Propose == nil || !pol.ByPullRequest() {
		return false
	}
	p := Proposal{
		Dir: l.Dir, Remote: pol.PushRemote, Base: pol.Branch, Head: l.Branch(l.Dir),
		Title: "[" + res.Ticket + "] " + strings.TrimSpace(namePrefix.ReplaceAllString(qc.Name, "$2")),
		Body:  "Ticket " + res.Ticket + "\n\n" + strings.Join(qc.AcceptanceCriteria, "\n"),
	}
	url, err := c.Propose(ctx, p)
	if err != nil {
		res.Notes = append(res.Notes, fmt.Sprintf("the pull request of %s into %s could not be opened (%s): push the branch and run `gh pr create --base %s --head %s`", p.Head, p.Base, firstLineOf(err.Error()), p.Base, p.Head))
		return true
	}
	res.Notes = append(res.Notes, fmt.Sprintf("pull request of %s into %s: %s (a person merges it; %s is never pushed directly)", p.Head, p.Base, url, p.Base))
	return true
}

// proposeByPullRequest pushes the branch and opens its pull request with the GitHub CLI.
func proposeByPullRequest(ctx context.Context, p Proposal) (string, error) {
	if out, err := exec.CommandContext(ctx, "git", "-C", p.Dir, "push", "-u", p.Remote, p.Head).CombinedOutput(); err != nil {
		return "", fmt.Errorf("git push %s %s: %s", p.Remote, p.Head, firstLineOf(strings.TrimSpace(string(out))))
	}
	pr, err := pullrequest.Open(ctx, pullrequest.Request{Dir: p.Dir, Base: p.Base, Head: p.Head, Title: p.Title, Body: p.Body})
	return pr.URL, err
}

func firstLineOf(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}
