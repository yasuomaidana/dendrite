// Package featurebranch names the branches of a ticket after what they are for. A ticket with no parent works
// on its own trunk, `<TICKET>-<slug>/main`, which merges into the integration branch. A sub-ticket works on
// `<ROOT>-<slug>/<TICKET>-<slug>` and starts from, and merges into, the trunk of its top-most parent; when the
// whole feature is done the trunk merges into the integration branch. A `feature/<ROOT>-<slug>` branch made
// before these names existed keeps serving as the trunk of its feature.
package featurebranch

import (
	"context"
	"fmt"
	"os/exec"
	"path"
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

// TrunkLeaf is the last segment of a trunk: the branch a feature folder merges through.
const TrunkLeaf = "main"

// Titled is a Source that also knows the title of a ticket itself, which names its own branch. A Source
// without it gives branches named by the ticket id alone.
type Titled interface {
	// Title is the title of ticket without its [ID] prefix.
	Title(ctx context.Context, ticket string) (string, error)
}

// Plan says which branch a ticket is worked on, and where that work starts from and merges into.
type Plan struct {
	// Ticket is the ticket the plan is for.
	Ticket string
	// Branch is the branch the ticket is worked on: its trunk when it has no parent, otherwise a branch in the
	// folder of its top-most parent.
	Branch string
	// Trunk is the branch the whole feature merges through: Branch for a ticket with no parent, Target otherwise.
	Trunk string
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

// Name is prefix followed by the ticket id and the slug of its title: the folder of a ticket's branches when
// prefix is empty, and the feature branch of older repositories when it is "feature/".
func Name(prefix string, t Ticket) string {
	if slug := Slug(t.Title); slug != "" {
		return prefix + t.ID + "-" + slug
	}
	return prefix + t.ID
}

// CopyName is the readable name of the working copy of a plan's branch: `<TICKET>-<slug>`.
func (p Plan) CopyName() string {
	return path.Base(strings.TrimSuffix(p.Branch, "/"+TrunkLeaf))
}

// Resolve decides the branch of ticket and where its work merges. The top-most parent (up to maxDepth levels)
// names the folder, so a whole tree of sub-tickets lands on one trunk. A trunk that already exists for a ticket
// keeps its name even if the ticket was renamed. Without a parent the ticket works on its own trunk and the plan
// targets the integration branch.
func Resolve(ctx context.Context, dir, ticket string, src Source) (Plan, error) {
	pol := branchpolicy.For(dir)
	p := Plan{Ticket: ticket, Target: pol.Integration, Integration: pol.Integration, Remote: pol.Remote, PushRemote: pol.PushRemote}
	self := Ticket{ID: ticket}
	if t, ok := src.(Titled); ok {
		title, err := t.Title(ctx, ticket)
		if err != nil {
			return p, fmt.Errorf("looking up the title of %s: %w", ticket, err)
		}
		self.Title = title
	}
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
		p.Branch = trunkOf(dir, pol, self)
		p.Trunk = p.Branch
		return p, nil
	}
	p.Feature = true
	p.Target = trunkOf(dir, pol, p.Parent)
	p.Trunk = p.Target
	folder, isFolder := strings.CutSuffix(p.Target, "/"+TrunkLeaf)
	if !isFolder {
		// A legacy feature/<ROOT>-<slug> trunk is a branch, so nothing can live below it.
		folder = Name("", p.Parent)
	}
	p.Branch = folder + "/" + Name("", self)
	return p, nil
}

// trunkOf is the trunk of the feature a ticket names: the one that exists already (a folder trunk first, then a
// legacy feature branch), otherwise `<ID>-<slug>/main`.
func trunkOf(dir string, pol branchpolicy.Target, t Ticket) string {
	if found := existing(dir, pol, t.ID, "/"+TrunkLeaf); found != "" {
		return found
	}
	if found := existing(dir, pol, pol.FeaturePrefix+t.ID, ""); found != "" {
		return found
	}
	return Name("", t) + "/" + TrunkLeaf
}

func git(dir string, args ...string) (string, bool) {
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	return strings.TrimSpace(string(out)), err == nil
}

// existing is the branch (local or on the remotes) named stem + suffix or stem + "-<anything>" + suffix, or "".
func existing(dir string, pol branchpolicy.Target, stem, suffix string) string {
	args := []string{"for-each-ref", "--format=%(refname:short)"}
	for _, root := range []string{"refs/heads/", "refs/remotes/" + pol.PushRemote + "/", "refs/remotes/" + pol.Remote + "/"} {
		args = append(args, root+stem+suffix, root+stem+"-*"+suffix)
	}
	out, ok := git(dir, args...)
	if !ok {
		return ""
	}
	for _, ref := range strings.Fields(out) {
		name := strings.TrimPrefix(strings.TrimPrefix(ref, pol.PushRemote+"/"), pol.Remote+"/")
		if name == stem+suffix || (strings.HasPrefix(name, stem+"-") && strings.HasSuffix(name, suffix)) {
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
