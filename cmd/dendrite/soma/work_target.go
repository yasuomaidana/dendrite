package soma

import (
	"Dendrite/cmd/dendrite/internal/planecli"
	"context"
	"fmt"
	"io"
	"strings"

	"Dendrite/pkg/featurebranch"
	"Dendrite/pkg/planesvc"
	"Dendrite/pkg/ticketlock"
	"Dendrite/pkg/workcopy"
)

// workTarget is where a new working copy starts and where its work will merge.
type workTarget struct {
	// Base is the commit-ish the copy starts from ("" keeps the default, HEAD).
	Base string
	// Branch is the branch its work merges into ("" for the integration branch).
	Branch string
	// Integration is the integration branch, so a Branch equal to it is not recorded.
	Integration string
}

// prepareWorkTarget decides where the copy of a ticket starts and merges. An explicit --target wins; otherwise a
// ticket with a Plane parent works on the parent's feature branch, created from the integration branch on its
// first use (lookup says whether Plane may be asked: a ticket-shaped name with the ticket guard on). Plane being unreachable only costs the feature branch: the copy then starts from HEAD as before.
func prepareWorkTarget(ctx context.Context, ticket, base, target string, lookup bool, out io.Writer) (workTarget, error) {
	wt := workTarget{Base: base}
	if target != "" {
		wt.Branch = target
		if base == "" {
			wt.Base = target
		}
		return wt, nil
	}
	if !lookup {
		return wt, nil
	}
	client, err := planecli.NewPlaneClient()
	if err != nil {
		fmt.Fprintf(out, "[!] feature branch not looked up: Plane is not configured (%s)\n", firstLine(err.Error()))
		return wt, nil
	}
	plan, err := featurebranch.Resolve(ctx, ".", ticket, ticketlock.Board{Svc: planesvc.Default(), Client: client})
	if err != nil {
		fmt.Fprintf(out, "[!] feature branch not looked up: %s\n", firstLine(err.Error()))
		return wt, nil
	}
	wt.Integration = plan.Integration
	if !plan.Feature {
		return wt, nil
	}
	pushErr, err := featurebranch.Ensure(".", plan)
	if err != nil {
		return wt, fmt.Errorf("feature branch %s of %s: %w", plan.Target, plan.Parent.ID, err)
	}
	if pushErr != nil {
		fmt.Fprintf(out, "[!] %s exists locally but was not pushed: %s\n", plan.Target, firstLine(pushErr.Error()))
	}
	fmt.Fprintf(out, "[✓] %s is a sub-ticket of %s: its work starts from and merges into %s\n", ticket, plan.Parent.ID, plan.Target)
	wt.Branch = plan.Target
	if base == "" {
		wt.Base = plan.Target
	}
	return wt, nil
}

// recordWorkTarget remembers the target of the new copy's branch so the guard, the finish diff, the gate and
// `work gc` use it.
func recordWorkTarget(name, branch string, wt workTarget, out io.Writer) {
	if wt.Branch == "" {
		return
	}
	if branch == "" {
		branch = "agent/" + strings.ToLower(workcopy.SanitizeName(name))
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
