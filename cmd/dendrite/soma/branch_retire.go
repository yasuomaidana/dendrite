package soma

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"Dendrite/cmd/dendrite/internal/cli"
	"Dendrite/pkg/branchpolicy"
	"Dendrite/pkg/pullrequest"
)

// retirement is where a branch that is being retired stands.
type retirement struct {
	Branch      string `json:"branch"`
	Integration string `json:"integration"`
	// Ahead is how many commits of Branch the integration branch does not have.
	Ahead int `json:"ahead"`
	// PullRequest is the pull request of Branch into the integration branch ("" when none was needed or opened).
	PullRequest string `json:"pull_request,omitempty"`
	Merged      bool   `json:"merged"`
	// Dependants are the local branches whose work still merges into Branch.
	Dependants []string `json:"dependants"`
	// Retired says nothing is left to do: every commit is in the integration branch and no branch targets it.
	Retired bool `json:"retired"`
}

func newBranchRetireCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "retire <branch>",
		Short: "Open the last pull request of a branch (e.g. develop) into the integration branch and list what still targets it",
		Long: "Retires a branch that work no longer integrates on. Its remaining commits are proposed to the integration\n" +
			"branch by pull request (a person merges it), and the local branches that still merge into it are listed.\n" +
			"Run it again after the merge: when nothing is left, `dendrite work dispose` offers to delete the branch.",
		Example: "dendrite branch retire develop",
		Args:    cli.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := retireBranch(cmd.Context(), ".", args[0])
			if err != nil {
				return err
			}
			return writeRetirement(cmd.OutOrStdout(), r)
		},
	}
}

func gitOut(dir string, args ...string) (string, error) {
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	return strings.TrimSpace(string(out)), err
}

// retireBranch proposes what branch still holds to the integration branch and reports who still targets it.
func retireBranch(ctx context.Context, dir, branch string) (retirement, error) {
	pol := branchpolicy.Resolve(dir, branchpolicy.MemberOverride(dir))
	r := retirement{Branch: branch, Integration: pol.Integration, Dependants: []string{}}
	if branch == pol.Integration {
		return r, cli.UsageErrorf("%s is the integration branch of this project; name another with `dendrite directive` before retiring it", branch)
	}
	ref := branch
	if _, err := gitOut(dir, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch); err != nil {
		ref = pol.PushRemote + "/" + branch
		if _, err := gitOut(dir, "rev-parse", "--verify", "--quiet", "refs/remotes/"+ref); err != nil {
			return r, cli.UsageErrorf("there is no branch %s here or on %s", branch, pol.PushRemote)
		}
	}
	heads, _ := gitOut(dir, "for-each-ref", "--format=%(refname:short)", "refs/heads")
	for _, b := range strings.Fields(heads) {
		if b != branch && branchpolicy.ForBranch(dir, b).Branch == branch {
			r.Dependants = append(r.Dependants, b)
		}
	}
	base := pol.Integration
	if _, err := gitOut(dir, "rev-parse", "--verify", "--quiet", "refs/remotes/"+pol.Ref()); err == nil {
		base = pol.Ref()
	}
	count, err := gitOut(dir, "rev-list", "--count", base+".."+ref)
	if err != nil {
		return r, fmt.Errorf("comparing %s with %s: %w", ref, base, err)
	}
	fmt.Sscanf(count, "%d", &r.Ahead)
	if r.Ahead > 0 {
		pr, err := pullrequest.Open(ctx, pullrequest.Request{
			Dir: dir, Base: pol.Integration, Head: branch,
			Title: "Retire " + branch + ": merge it into " + pol.Integration,
			Body:  "Work no longer integrates on `" + branch + "`. This is its last merge into `" + pol.Integration + "`; the branch is deleted once nothing targets it.",
		})
		if err != nil {
			return r, fmt.Errorf("opening the pull request of %s into %s: %w", branch, pol.Integration, err)
		}
		r.PullRequest, r.Merged = pr.URL, pr.Merged()
	}
	r.Retired = r.Ahead == 0 && len(r.Dependants) == 0
	return r, nil
}

func writeRetirement(out io.Writer, r retirement) error {
	if cli.Globals.JSON {
		return cli.WriteJSON(out, r)
	}
	switch {
	case r.Ahead == 0:
		fmt.Fprintf(out, "[✓] %s holds every commit of %s\n", r.Integration, r.Branch)
	case r.Merged:
		fmt.Fprintf(out, "[✓] %s was merged (%s); fetch to see it: %d commit(s) are not in the local %s yet\n", r.Branch, r.PullRequest, r.Ahead, r.Integration)
	default:
		fmt.Fprintf(out, "[+] %s has %d commit(s) %s does not: pull request %s (a person merges it)\n", r.Branch, r.Ahead, r.Integration, r.PullRequest)
	}
	for _, b := range r.Dependants {
		fmt.Fprintf(out, "    - %s still merges into %s\n", b, r.Branch)
	}
	if r.Retired {
		fmt.Fprintf(out, "[✓] Nothing uses %s any more: `dendrite work dispose` offers to delete it. If a synapse tracks it for this project, set the member's branch to %s.\n", r.Branch, r.Integration)
	}
	return nil
}
