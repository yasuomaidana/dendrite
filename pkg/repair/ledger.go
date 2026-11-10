// Package repair tracks temporary repairs of dependencies. A repair points a consumer at a branch of a
// dependency (in the dependency's own repository, or in a fork) while a bug or a security hole is fixed there,
// remembers how the consumer named the dependency before, and puts that back when the fix is merged and
// released. Repairs chain: the working copy of one repair can itself start a repair of its own dependency.
//
// The ledger is one file in the Dendrite home, not in any repository, because a chain spans repositories.
package repair

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"Dendrite/pkg/pool/switcher"
	"Dendrite/pkg/projectid"
)

// Stage is how far a repair has come.
type Stage string

const (
	// StageRepairing: the consumer points at the repair branch.
	StageRepairing Stage = "repairing"
	// StageMerged: the fix is in the dependency's default branch and the consumer is pinned to that commit.
	StageMerged Stage = "merged"
	// StageReleased: a release holds the fix and the consumer names the dependency the normal way again.
	StageReleased Stage = "released"
)

// Link is one repair: Consumer uses Dependency from Branch until the fix is merged and released.
type Link struct {
	// ID names the link: "<consumer ticket>/<dependency>".
	ID string `json:"id"`
	// Parent is the ID of the link whose working copy is this link's consumer ("" at the top of a chain).
	Parent string `json:"parent,omitempty"`
	// Consumer is the directory of the project whose manifest was switched; Ticket is the ticket it is worked for.
	Consumer string `json:"consumer"`
	Ticket   string `json:"ticket"`
	// Dependency is the name the consumer's manifest uses.
	Dependency string `json:"dependency"`
	// DepTicket is the dependency project's own ticket for the fix, when it has one.
	DepTicket string `json:"dep_ticket,omitempty"`
	// Copy is the working copy the fix is made in.
	Copy string `json:"copy"`
	// Upstream is the dependency's repository; Remote is where Branch is pushed (Upstream, or a fork of it).
	Upstream string `json:"upstream"`
	Remote   string `json:"remote"`
	Fork     bool   `json:"fork"`
	Branch   string `json:"branch"`
	// Original is how the consumer named the dependency before the repair, one entry per manifest.
	Original []switcher.Entry `json:"original"`
	Stage    Stage            `json:"stage"`
	// PullRequest is the URL of the fix's pull request; MergeSHA the commit it was merged as; Version the
	// release that holds it.
	PullRequest string    `json:"pull_request,omitempty"`
	MergeSHA    string    `json:"merge_sha,omitempty"`
	Version     string    `json:"version,omitempty"`
	StartedAt   time.Time `json:"started_at"`
}

// ErrNoLink is returned when the ledger has no link with the asked ID.
var ErrNoLink = errors.New("repair: no such link")

// Ledger is the file that holds every link.
type Ledger struct{ Path string }

// Open is the ledger of the Dendrite home ($DENDRITE_HOME, else ~/.dendrite).
func Open() (*Ledger, error) {
	home, err := projectid.HomeDir()
	if err != nil {
		return nil, err
	}
	return &Ledger{Path: filepath.Join(home, "repairs.json")}, nil
}

// Links lists every link, oldest first. A ledger that does not exist yet is empty.
func (l *Ledger) Links() ([]Link, error) {
	data, err := os.ReadFile(l.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var links []Link
	if err := json.Unmarshal(data, &links); err != nil {
		return nil, fmt.Errorf("repair: %s is not readable: %w", l.Path, err)
	}
	sort.SliceStable(links, func(i, j int) bool { return links[i].StartedAt.Before(links[j].StartedAt) })
	return links, nil
}

// Get returns the link with id.
func (l *Ledger) Get(id string) (Link, error) {
	links, err := l.Links()
	if err != nil {
		return Link{}, err
	}
	for _, k := range links {
		if k.ID == id {
			return k, nil
		}
	}
	return Link{}, fmt.Errorf("%w: %s", ErrNoLink, id)
}

// Put stores the links, each replacing the one with the same ID.
func (l *Ledger) Put(put ...Link) error {
	links, err := l.Links()
	if err != nil {
		return err
	}
	for _, link := range put {
		replaced := false
		for i := range links {
			if links[i].ID == link.ID {
				links[i], replaced = link, true
			}
		}
		if !replaced {
			links = append(links, link)
		}
	}
	return l.save(links)
}

// Remove drops the link with id; a link that is not there is not an error.
func (l *Ledger) Remove(id string) error {
	links, err := l.Links()
	if err != nil {
		return err
	}
	kept := links[:0]
	for _, k := range links {
		if k.ID != id {
			kept = append(kept, k)
		}
	}
	return l.save(kept)
}

func (l *Ledger) save(links []Link) error {
	if links == nil {
		links = []Link{}
	}
	data, _ := json.MarshalIndent(links, "", "  ") // a slice of links always marshals
	if err := os.MkdirAll(filepath.Dir(l.Path), 0o755); err != nil {
		return err
	}
	tmp := l.Path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, l.Path)
}

// Children lists the links whose consumer is the working copy of the link with id.
func Children(links []Link, id string) []Link {
	var out []Link
	for _, k := range links {
		if k.Parent == id {
			out = append(out, k)
		}
	}
	return out
}
