package soma

import (
	"Dendrite/cmd/dendrite/internal/planecli"
	"cmp"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"Dendrite/pkg/featurebranch"
	"Dendrite/pkg/planesvc"
	"Dendrite/pkg/ticketlock"
)

// workTarget is the branch of a new working copy, where it starts and where its work will merge.
type workTarget struct {
	// Work is the branch the copy is created on ("" for the default of its name).
	Work string
	// Copy names the copy's directory ("" for the name it was asked by).
	Copy string
	// Base is the commit-ish the copy starts from ("" keeps the default, HEAD).
	Base string
	// Branch is the branch its work merges into ("" for the integration branch).
	Branch string
	// Integration is the integration branch, so a Branch equal to it is not recorded.
	Integration string
}

// prepareWorkTarget decides the branch of a ticket's copy and where it starts and merges. Plane names it (lookup
// says whether Plane may be asked: a ticket-shaped name with the ticket guard on): a ticket works on its own
// trunk <TICKET>-<slug>/main, and a ticket with a Plane parent on <ROOT>-<slug>/<TICKET>-<slug>, starting from and
// merging into the parent's trunk, which is created from the integration branch on its first use. An explicit
// --target wins over that trunk. Plane being unreachable only costs the names: the copy then starts from HEAD on
// the default branch of its name.
func prepareWorkTarget(ctx context.Context, ticket, base, target string, lookup bool, out io.Writer) (workTarget, error) {
	wt := workTarget{Base: base}
	if lookup {
		plan, ok := lookUpPlan(ctx, ticket, out)
		if ok {
			wt.Work, wt.Copy, wt.Integration = plan.Branch, plan.CopyName(), plan.Integration
			wt.Base = cmp.Or(base, remoteTrunk(plan))
		}
		if ok && plan.Feature && target == "" {
			pushErr, err := featurebranch.Ensure(".", plan)
			if err != nil {
				return wt, fmt.Errorf("feature branch %s of %s: %w", plan.Target, plan.Parent.ID, err)
			}
			if pushErr != nil {
				fmt.Fprintf(out, "[!] %s exists locally but was not pushed: %s\n", plan.Target, firstLine(pushErr.Error()))
			}
			fmt.Fprintf(out, "[✓] %s is a sub-ticket of %s: its work starts from and merges into %s\n", ticket, plan.Parent.ID, plan.Target)
			target = plan.Target
		}
	}
	if target != "" {
		wt.Branch = target
		if base == "" {
			wt.Base = target
		}
	}
	return wt, nil
}

// remoteTrunk is the remote-tracking ref of a ticket's own trunk when only the remote has it (its sub-tickets
// were started on another machine): the copy then continues that trunk instead of starting a second one.
func remoteTrunk(plan featurebranch.Plan) string {
	ref := plan.PushRemote + "/" + plan.Branch
	if plan.Feature || exec.Command("git", "rev-parse", "--verify", "--quiet", "refs/heads/"+plan.Branch).Run() == nil ||
		exec.Command("git", "rev-parse", "--verify", "--quiet", "refs/remotes/"+ref).Run() != nil {
		return ""
	}
	return ref
}

// lookUpPlan asks Plane for the branch plan of ticket; a failure is a warning, and ok is false.
func lookUpPlan(ctx context.Context, ticket string, out io.Writer) (featurebranch.Plan, bool) {
	client, err := planecli.NewPlaneClient()
	if err != nil {
		fmt.Fprintf(out, "[!] feature branch not looked up: Plane is not configured (%s)\n", firstLine(err.Error()))
		return featurebranch.Plan{}, false
	}
	plan, err := featurebranch.Resolve(ctx, ".", ticket, ticketlock.Board{Svc: planesvc.Default(), Client: client})
	if err != nil {
		fmt.Fprintf(out, "[!] feature branch not looked up: %s\n", firstLine(err.Error()))
		return featurebranch.Plan{}, false
	}
	return plan, true
}

// recordWorkTarget remembers the target of the new copy's branch so the guard, the finish diff, the gate and
// `work gc` use it.
func recordWorkTarget(branch string, wt workTarget, out io.Writer) {
	if wt.Branch == "" {
		return
	}
	if err := featurebranch.Record(".", branch, wt.Branch, wt.Integration); err != nil {
		fmt.Fprintf(out, "[!] the target %s of %s was not recorded: %s\n", wt.Branch, branch, firstLine(err.Error()))
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
