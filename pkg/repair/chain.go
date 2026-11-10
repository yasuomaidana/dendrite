package repair

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"strings"

	"Dendrite/pkg/pullrequest"
)

// Row is one link of a chain and how deep it sits below the consumer that started the chain.
type Row struct {
	Depth int  `json:"depth"`
	Link  Link `json:"link"`
}

// Tree lists links chain by chain, each parent before its children. A link whose parent is gone starts a chain.
func Tree(links []Link) []Row {
	known := map[string]bool{}
	for _, k := range links {
		known[k.ID] = true
	}
	var rows []Row
	var walk func(k Link, depth int)
	walk = func(k Link, depth int) {
		rows = append(rows, Row{Depth: depth, Link: k})
		for _, child := range Children(links, k.ID) {
			walk(child, depth+1)
		}
	}
	for _, k := range links {
		if !known[k.Parent] {
			walk(k, 0)
		}
	}
	return rows
}

// LeafFirst lists links so that every link comes after the links below it: the order fixes are proposed and
// merged in.
func LeafFirst(links []Link) []Link {
	rows := Tree(links)
	out := make([]Link, 0, len(rows))
	for depth := deepest(rows); depth >= 0; depth-- {
		for _, r := range rows {
			if r.Depth == depth {
				out = append(out, r.Link)
			}
		}
	}
	return out
}

func deepest(rows []Row) int {
	d := 0
	for _, r := range rows {
		d = max(d, r.Depth)
	}
	return d
}

// upstreamRemote is the git remote of the link's working copy that is the dependency's own repository.
func (k Link) upstreamRemote() string {
	if k.Fork {
		return "upstream"
	}
	return "origin"
}

// baseBranch is the dependency's default branch, as the working copy knows it; main when it does not.
func (k Link) baseBranch(ctx context.Context) string {
	ref, err := git(ctx, k.Copy, "symbolic-ref", "--short", "refs/remotes/"+k.upstreamRemote()+"/HEAD")
	if err != nil {
		return "main"
	}
	return strings.TrimPrefix(ref, k.upstreamRemote()+"/")
}

// Propose opens the pull request of every link that is still being repaired and has none, the links at the
// bottom of a chain first, so each pull request can name the ones it waits for. It returns the links it opened
// a pull request for; a link whose pull request cannot be opened stops it.
func Propose(ctx context.Context, ledger *Ledger) ([]Link, error) {
	links, err := ledger.Links()
	if err != nil {
		return nil, err
	}
	urls := map[string]string{}
	for _, k := range links {
		urls[k.ID] = k.PullRequest
	}
	var opened []Link
	for _, k := range LeafFirst(links) {
		if k.Stage != StageRepairing || k.PullRequest != "" {
			continue
		}
		body := fmt.Sprintf("Repairs `%s` for %s (consumer: `%s`).", k.Dependency, k.Ticket, k.Consumer)
		for _, child := range Children(links, k.ID) {
			body += fmt.Sprintf("\n\nWaits for the repair of `%s`: %s", child.Dependency, urls[child.ID])
		}
		pr, err := pullrequest.Open(ctx, pullrequest.Request{
			Dir: k.Copy, Base: k.baseBranch(ctx), Head: k.Branch,
			Title: "[" + cmp.Or(k.DepTicket, k.Ticket) + "] Repair for " + k.Ticket + ": " + k.Dependency, Body: body,
		})
		if err != nil {
			return opened, errors.Join(fmt.Errorf("repair %s: %w", k.ID, err), ledger.Put(opened...))
		}
		k.PullRequest, urls[k.ID] = pr.URL, pr.URL
		opened = append(opened, k)
	}
	return opened, ledger.Put(opened...)
}
