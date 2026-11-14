// Package dispose finds what a finished change left behind and deletes it when asked: the working copies and
// remote branches of repairs whose fix is merged, the branches of features whose trunk is merged, and an
// integration branch that was retired. It never touches a protected branch, a fork repository, or anything with
// uncommitted, unpushed or unmerged work: those are listed as kept, with the reason.
package dispose

// TODO(move:dendrite-git-rs,DGIT-7): deleting branches and worktrees is repository plumbing that dendrite-git
//   owns (git.worktree.remove is DGIT-7; deleting a remote branch has no method yet).
//   When: the sidecar exposes them; then the git calls here go through it.
//   Stays in Go: deciding what is disposable, the kept reasons and the prompt.
//   Until then: keep these calls thin; add no git features here.

import (
	"context"
	"fmt"

	"Dendrite/pkg/repair"
)

// Kind groups items for the prompt: a person answers once per kind.
type Kind string

const (
	KindRepairCopy   Kind = "repair working copy"
	KindRepairBranch Kind = "remote repair branch"
	KindFeature      Kind = "finished feature"
	KindRetired      Kind = "retired branch"
)

// Kinds lists the kinds in the order they are asked about.
var Kinds = []Kind{KindRepairCopy, KindRepairBranch, KindFeature, KindRetired}

// Item is one thing that could be deleted.
type Item struct {
	Kind Kind `json:"kind"`
	// Name is what it is: a path, or a branch (with its remote).
	Name string `json:"name"`
	// Why says what makes it disposable, e.g. "merged into main".
	Why string `json:"why"`
	// Kept is the reason it must stay; empty when it can be deleted.
	Kept string `json:"kept,omitempty"`
	// remove deletes it.
	remove func(ctx context.Context) error
}

// Disposable reports whether the item can be deleted.
func (i Item) Disposable() bool { return i.Kept == "" }

// Delete removes a disposable item; an item that must stay is refused.
func (i Item) Delete(ctx context.Context) error {
	if !i.Disposable() {
		return fmt.Errorf("%s %s is kept: %s", i.Kind, i.Name, i.Kept)
	}
	return i.remove(ctx)
}

// Options says where to look.
type Options struct {
	// Dir is the repository whose features and retired branches are looked at.
	Dir string
	// Offline skips every question that needs the network (pull request state, remote listings), so only what
	// the local repository already knows is offered. It is what commands that run often use.
	Offline bool
	// Tracked reports why a branch of the repository at Dir must stay although nothing merges into it, e.g. a
	// synapse that still tracks it; "" when nothing does. Nil means nothing does.
	Tracked func(branch string) string
}

func git(ctx context.Context, dir string, args ...string) (string, error) {
	return repair.Git(ctx, dir, args...)
}

func hasRef(ctx context.Context, dir, ref string) bool {
	_, err := git(ctx, dir, "rev-parse", "--verify", "--quiet", ref)
	return err == nil
}

// isAncestor reports whether every commit of ref is in target.
func isAncestor(ctx context.Context, dir, ref, target string) bool {
	_, err := git(ctx, dir, "merge-base", "--is-ancestor", ref, target)
	return err == nil
}

// Group is the disposable and the kept items of one kind.
type Group struct {
	Kind       Kind
	Disposable []Item
	Kept       []Item
}

// Grouped sorts items into one group per kind that has any, in the order of Kinds.
func Grouped(items []Item) []Group {
	var groups []Group
	for _, kind := range Kinds {
		g := Group{Kind: kind}
		for _, it := range items {
			switch {
			case it.Kind != kind:
			case it.Disposable():
				g.Disposable = append(g.Disposable, it)
			default:
				g.Kept = append(g.Kept, it)
			}
		}
		if len(g.Disposable)+len(g.Kept) > 0 {
			groups = append(groups, g)
		}
	}
	return groups
}
