package dispose

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"Dendrite/pkg/repair"
	"Dendrite/pkg/workcopy"
)

// Repairs lists what the repairs whose fix is merged left behind: the working copy and the pushed repair branch
// of each. A released repair that left nothing behind is forgotten: its link is dropped from the ledger.
func Repairs(ctx context.Context, ledger *repair.Ledger, o Options) ([]Item, error) {
	links, err := ledger.Links()
	if err != nil {
		return nil, err
	}
	var items []Item
	for _, k := range links {
		if k.Stage == repair.StageRepairing {
			continue
		}
		left := leftOf(ctx, k, o)
		if len(left) == 0 && k.Stage == repair.StageReleased {
			err = errors.Join(err, ledger.Remove(k.ID))
		}
		items = append(items, left...)
	}
	return items, err
}

// leftOf is the working copy and the remote branch of one repair, as far as they still exist.
func leftOf(ctx context.Context, k repair.Link, o Options) []Item {
	why := "repair " + k.ID + " is " + string(k.Stage)
	var items []Item
	if _, err := os.Stat(k.Copy); err == nil {
		items = append(items, Item{Kind: KindRepairCopy, Name: k.Copy, Why: why, Kept: unsaved(ctx, k.Copy), remove: func(ctx context.Context) error { return removeCopy(ctx, k.Copy) }})
	}
	if from := pushFrom(k); from != "" && onRemote(ctx, from, k, o) {
		items = append(items, Item{Kind: KindRepairBranch, Name: k.Branch + " on " + k.Remote, Why: why, remove: func(ctx context.Context) error {
			_, err := git(ctx, from, "push", k.Remote, "--delete", k.Branch)
			return err
		}})
	}
	return items
}

// pushFrom is a repository to run the branch deletion from: the working copy, else the consumer.
func pushFrom(k repair.Link) string {
	for _, dir := range []string{k.Copy, k.Consumer} {
		if _, err := os.Stat(dir); err == nil {
			return dir
		}
	}
	return ""
}

// onRemote reports whether the repair branch is still on its remote: asked of the remote, or, offline, read
// from what the working copy last saw.
func onRemote(ctx context.Context, from string, k repair.Link, o Options) bool {
	if o.Offline {
		return from == k.Copy && hasRef(ctx, from, "refs/remotes/origin/"+k.Branch)
	}
	out, err := git(ctx, from, "ls-remote", "--heads", k.Remote, k.Branch)
	return err == nil && out != ""
}

// unsaved is the reason a working copy must stay: uncommitted changes, or commits its remote does not have.
func unsaved(ctx context.Context, dir string) string {
	status, err := git(ctx, dir, "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return "its state cannot be read"
	}
	if status != "" {
		return "uncommitted changes"
	}
	local, err := git(ctx, dir, "rev-list", "--count", "HEAD", "--not", "--remotes")
	if err != nil || local != "0" {
		return fmt.Sprintf("%s commit(s) no remote has", local)
	}
	return ""
}

// removeCopy deletes a repair's working copy: a worktree through its repository (which also drops the merged
// branch), a clone as a directory.
func removeCopy(ctx context.Context, dir string) error {
	if info, err := os.Lstat(filepath.Join(dir, ".git")); err == nil && !info.IsDir() {
		top, err := workcopy.New(dir).MainTop(ctx) // run from the main checkout: the copy is about to go
		if err == nil {
			_, err = workcopy.New(top).Remove(ctx, dir, workcopy.RemoveOptions{})
		}
		return err
	}
	return os.RemoveAll(dir)
}
