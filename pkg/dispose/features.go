package dispose

import (
	"context"
	"sort"
	"strings"

	"Dendrite/pkg/branchpolicy"
	"Dendrite/pkg/featurebranch"
	"Dendrite/pkg/pullrequest"
	"Dendrite/pkg/repair"
)

// repo is the repository whose branches are looked at, with what every check needs.
type repo struct {
	dir string
	pol branchpolicy.Target
	// target is the ref finished work is in: the remote-tracking integration branch, else the local one.
	target string
	// heads are the local branches; busy maps a checked-out branch to its working copy.
	heads []string
	busy  map[string]string
	o     Options
}

func open(ctx context.Context, o Options) repo {
	r := repo{dir: o.Dir, o: o, busy: map[string]string{}}
	r.pol = branchpolicy.Resolve(o.Dir, branchpolicy.MemberOverride(o.Dir))
	r.target = r.pol.Integration
	if hasRef(ctx, o.Dir, "refs/remotes/"+r.pol.Ref()) {
		r.target = r.pol.Ref()
	}
	heads, _ := git(ctx, o.Dir, "for-each-ref", "--format=%(refname:short)", "refs/heads")
	r.heads = strings.Fields(heads)
	worktrees, _ := git(ctx, o.Dir, "worktree", "list", "--porcelain")
	path := ""
	for _, line := range strings.Split(worktrees, "\n") {
		if p, ok := strings.CutPrefix(line, "worktree "); ok {
			path = p
		}
		if b, ok := strings.CutPrefix(line, "branch refs/heads/"); ok {
			r.busy[b] = path
		}
	}
	return r
}

// merged says why branch counts as merged into the integration branch: its commits are there, or (asked of
// GitHub, so not offline) its pull request was merged, which is how a squash merge shows. "" when it is not.
func (r repo) merged(ctx context.Context, branch string) string {
	if isAncestor(ctx, r.dir, "refs/heads/"+branch, r.target) {
		return "merged into " + r.pol.Integration
	}
	if r.o.Offline {
		return ""
	}
	if pr, err := pullrequest.View(ctx, r.dir, branch); err == nil && pr.Merged() && pr.Base == r.pol.Integration {
		return "its pull request into " + r.pol.Integration + " was merged"
	}
	return ""
}

// removeBranches deletes branches here and, where the push remote has them, there.
func (r repo) removeBranches(branches []string) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		for _, b := range branches {
			if hasRef(ctx, r.dir, "refs/remotes/"+r.pol.PushRemote+"/"+b) {
				if _, err := git(ctx, r.dir, "push", r.pol.PushRemote, "--delete", b); err != nil && !strings.Contains(err.Error(), "remote ref does not exist") {
					return err
				}
			}
			if _, err := git(ctx, r.dir, "branch", "-D", b); err != nil {
				return err
			}
		}
		return nil
	}
}

// Features lists the feature folders of the repository at o.Dir whose trunk is merged into the integration
// branch: the trunk and every branch beside it go together. A folder stays while one of its branches holds
// work the trunk does not have, or is checked out somewhere.
func Features(ctx context.Context, o Options) []Item {
	r := open(ctx, o)
	var items []Item
	for _, trunk := range r.heads {
		folder, isTrunk := strings.CutSuffix(trunk, "/"+featurebranch.TrunkLeaf)
		if !isTrunk {
			continue
		}
		why := r.merged(ctx, trunk)
		if why == "" {
			continue
		}
		it := Item{Kind: KindFeature, Name: folder, Why: why}
		var branches []string
		for _, b := range r.heads {
			if !strings.HasPrefix(b, folder+"/") {
				continue
			}
			branches = append(branches, b)
			switch {
			case r.busy[b] != "":
				it.Kept = b + " is checked out in " + r.busy[b]
			case b != trunk && !isAncestor(ctx, r.dir, "refs/heads/"+b, "refs/heads/"+trunk):
				it.Kept = b + " holds work its trunk does not have"
			}
		}
		sort.Strings(branches)
		it.remove = r.removeBranches(branches)
		items = append(items, it)
	}
	return items
}

// Retired lists the legacy integration branch (develop) of the repository at o.Dir once the project integrates
// elsewhere and holds every commit of it. It stays while a branch still merges into it, it is checked out, or
// o.Tracked names something that still follows it.
func Retired(ctx context.Context, o Options) []Item {
	r := open(ctx, o)
	legacy := branchpolicy.LegacyIntegration
	if r.pol.Integration == legacy || !hasRef(ctx, r.dir, "refs/heads/"+legacy) {
		return nil
	}
	why := r.merged(ctx, legacy)
	if why == "" {
		return nil
	}
	it := Item{Kind: KindRetired, Name: legacy, Why: why, remove: r.removeBranches([]string{legacy})}
	switch {
	case r.busy[legacy] != "":
		it.Kept = "it is checked out in " + r.busy[legacy]
	case r.dependant(legacy) != "":
		it.Kept = r.dependant(legacy) + " still merges into it"
	case o.Tracked != nil:
		it.Kept = o.Tracked(legacy)
	}
	return []Item{it}
}

// dependant names a local branch whose work merges into branch, or "".
func (r repo) dependant(branch string) string {
	for _, b := range r.heads {
		if b != branch && branchpolicy.ForBranch(r.dir, b).Branch == branch {
			return b
		}
	}
	return ""
}

// Find lists everything disposable or kept: the leftovers of merged repairs, then the finished features and a
// retired branch of the repository at o.Dir.
func Find(ctx context.Context, ledger *repair.Ledger, o Options) ([]Item, error) {
	items, err := Repairs(ctx, ledger, o)
	if err != nil {
		return nil, err
	}
	items = append(items, Features(ctx, o)...)
	return append(items, Retired(ctx, o)...), nil
}
