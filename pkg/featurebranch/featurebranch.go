// Package featurebranch groups the sub-tickets of a Plane parent ticket on one feature branch. A ticket with a
// parent starts from, and merges into, `feature/<PARENT>-<slug>` instead of the integration branch; when the
// whole feature is done the parent's own work merges that branch into the integration branch.
package featurebranch

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"Dendrite/pkg/branchpolicy"
)

// maxDepth stops a loop of parents (a ticket that is its own ancestor).
const maxDepth = 8

// Ticket is a Plane ticket as the branch name needs it.
type Ticket struct {
	ID    string // DEN-840
	Title string // without the [DEN-840] prefix
}

// Source answers which ticket is the parent of another.
type Source interface {
	// Parent reports the parent of ticket, ok false when it has none.
	Parent(ctx context.Context, ticket string) (parent Ticket, ok bool, err error)
}

// Plan says where work on a ticket starts from and merges into.
type Plan struct {
	// Ticket is the ticket the plan is for.
	Ticket string
	// Target is the branch the ticket's work merges into: a feature branch, or the integration branch.
	Target string
	// Feature is true when Target is a feature branch.
	Feature bool
	// Parent is the top-most parent that names the feature branch (zero when there is none).
	Parent Ticket
	// Integration, Remote and PushRemote are the project's (see branchpolicy.Target).
	Integration, Remote, PushRemote string
}

var nonWord = regexp.MustCompile(`[^a-z0-9]+`)

// Slug turns a ticket title into the readable tail of a branch name: lower case words joined by dashes, at most
// 40 characters, never ending in a dash.
func Slug(title string) string {
	s := strings.Trim(nonWord.ReplaceAllString(strings.ToLower(title), "-"), "-")
	if len(s) > 40 {
		s = strings.TrimRight(s[:40], "-")
	}
	return s
}

// Name is the feature branch of a parent ticket.
func Name(prefix string, parent Ticket) string {
	if slug := Slug(parent.Title); slug != "" {
		return prefix + parent.ID + "-" + slug
	}
	return prefix + parent.ID
}

// Resolve decides where work on ticket merges. The top-most parent (up to maxDepth levels) names the feature
// branch, so a whole tree of sub-tickets lands on one branch. A branch that already exists for that parent keeps
// its name even if the parent was renamed. Without a parent the plan targets the integration branch.
func Resolve(ctx context.Context, dir, ticket string, src Source) (Plan, error) {
	pol := branchpolicy.For(dir)
	p := Plan{Ticket: ticket, Target: pol.Integration, Integration: pol.Integration, Remote: pol.Remote, PushRemote: pol.PushRemote}
	top := Ticket{ID: ticket}
	for depth := 0; depth < maxDepth; depth++ {
		parent, ok, err := src.Parent(ctx, top.ID)
		if err != nil {
			return p, fmt.Errorf("looking up the parent of %s: %w", top.ID, err)
		}
		if !ok {
			break
		}
		top = parent
		p.Parent = parent
	}
	if p.Parent.ID == "" {
		return p, nil
	}
	p.Feature = true
	p.Target = existing(dir, pol, pol.FeaturePrefix+p.Parent.ID)
	if p.Target == "" {
		p.Target = Name(pol.FeaturePrefix, p.Parent)
	}
	return p, nil
}

func git(dir string, args ...string) (string, bool) {
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	return strings.TrimSpace(string(out)), err == nil
}

// existing is the branch (local or on the remotes) whose name is stem or starts with stem + "-", or "".
func existing(dir string, pol branchpolicy.Target, stem string) string {
	out, ok := git(dir, "for-each-ref", "--format=%(refname:short)", "refs/heads/"+stem+"*", "refs/remotes/"+pol.PushRemote+"/"+stem+"*", "refs/remotes/"+pol.Remote+"/"+stem+"*")
	if !ok {
		return ""
	}
	for _, ref := range strings.Fields(out) {
		name := strings.TrimPrefix(strings.TrimPrefix(ref, pol.PushRemote+"/"), pol.Remote+"/")
		if name == stem || strings.HasPrefix(name, stem+"-") {
			return name
		}
	}
	return ""
}

// Ensure makes the plan's feature branch exist locally: from the remote when it is there, otherwise from the
// integration branch, which is pushed to the push remote (a failed push is returned as pushErr, because the
// branch is usable without it). A plan with no feature branch needs nothing.
func Ensure(dir string, p Plan) (pushErr error, err error) {
	if !p.Feature || hasRef(dir, "refs/heads/"+p.Target) {
		return nil, nil
	}
	if hasRef(dir, "refs/remotes/"+p.PushRemote+"/"+p.Target) {
		return nil, run(dir, "branch", p.Target, p.PushRemote+"/"+p.Target)
	}
	start := p.Integration
	if hasRef(dir, "refs/remotes/"+p.Remote+"/"+p.Integration) {
		start = p.Remote + "/" + p.Integration
	}
	if err := run(dir, "branch", p.Target, start); err != nil {
		return nil, err
	}
	return run(dir, "push", p.PushRemote, p.Target), nil
}

func hasRef(dir, ref string) bool {
	_, ok := git(dir, "rev-parse", "--verify", "--quiet", ref)
	return ok
}

func run(dir string, args ...string) error {
	if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
		return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Record remembers that work on branch merges into target, so the guard, the finish diff, the gate and
// `work gc` all use it. A target equal to the integration branch is not recorded (and clears an old record).
func Record(dir, branch, target, integration string) error {
	if target == "" || target == integration {
		_ = run(dir, "config", "--unset", branchpolicy.TargetKey(branch))
		return nil
	}
	return run(dir, "config", branchpolicy.TargetKey(branch), target)
}
