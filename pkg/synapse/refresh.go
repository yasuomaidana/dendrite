package synapse

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"Dendrite/pkg/projectid"
)

// MatchRootCommit marks a directory tied to an indexed project by its first commit.
const MatchRootCommit MatchKind = "root_commit"

// RefreshStatus is the outcome of refreshing one entry of the project index.
type RefreshStatus string

const (
	RefreshOK        RefreshStatus = "ok"
	RefreshRelocated RefreshStatus = "relocated"
	RefreshAmbiguous RefreshStatus = "ambiguous"
	RefreshMissing   RefreshStatus = "missing"
	RefreshPruned    RefreshStatus = "pruned"
)

// RefreshOptions configures RefreshIndex.
type RefreshOptions struct {
	// Roots are extra directories to search in addition to the defaults.
	Roots []string
	// MaxDepth bounds directory descent below each root (DefaultHealDepth when <= 0).
	MaxDepth int
	// NoDefaultRoots disables the IDE project folders under the user's home directory.
	NoDefaultRoots bool
	// Apply writes the relocations (index, synapse path hints, pool files); false only reports.
	Apply bool
	// Prune removes entries whose project cannot be found. It needs Apply.
	Prune bool
}

// RefreshRow is one entry of the refresh report.
type RefreshRow struct {
	ID         string        `json:"id"`
	Name       string        `json:"name"`
	OldPath    string        `json:"old_path"`
	NewPath    string        `json:"new_path,omitempty"`
	Status     RefreshStatus `json:"status"`
	Match      MatchKind     `json:"match,omitempty"`
	Candidates []string      `json:"candidates,omitempty"`
	Applied    bool          `json:"applied"`
}

// RefreshIndex finds every entry of the global project index whose directory moved or vanished.
// Unlike Heal it covers all indexed projects, not only synapse members. An entry is stale when
// its path is gone, is a linked worktree, or no longer carries the entry's project ID. A moved
// project is found by project ID, then by root commit, then by a unique origin remote among
// directories that have no identity (a fresh clone); several candidates are reported, never
// guessed. With opts.Apply the relocations rewrite the index, the synapse path hints and the pool
// files through the same code as Heal; with opts.Prune the entries that cannot be found are removed.
func RefreshIndex(st *Store, ix *projectid.Index, opts RefreshOptions) ([]RefreshRow, error) {
	var stale []projectid.Entry
	var rows []RefreshRow
	for _, e := range ix.List() {
		if entryResolves(e) {
			rows = append(rows, RefreshRow{ID: e.ID, Name: e.Name, OldPath: e.Path, Status: RefreshOK})
			continue
		}
		stale = append(stale, e)
	}
	if opts.Apply {
		if _, err := BackfillPlane(ix); err != nil {
			return rows, fmt.Errorf("saving project index: %w", err)
		}
	}
	if len(stale) == 0 {
		return rows, nil
	}

	// One scan covers every stale entry: the roots are the union of what each one needs.
	var roots []string
	for _, e := range stale {
		roots = append(roots, searchRoots(Member{PathHint: e.Path}, HealOptions{Roots: opts.Roots, NoDefaultRoots: opts.NoDefaultRoots})...)
	}
	cands := scan(dedupe(roots), opts.MaxDepth)

	moves := map[string]Location{}
	var prune []string
	for _, e := range stale {
		row := RefreshRow{ID: e.ID, Name: e.Name, OldPath: e.Path}
		loc := locateIndexed(cands, e)
		row.Match, row.Candidates = loc.Match, loc.Candidates
		switch {
		case loc.Path != "":
			row.Status, row.NewPath = RefreshRelocated, loc.Path
			moves[e.ID] = loc
		case len(loc.Candidates) > 0:
			row.Status = RefreshAmbiguous
		default:
			row.Status = RefreshMissing
			if opts.Prune {
				prune = append(prune, e.ID)
			}
		}
		rows = append(rows, row)
	}

	if opts.Apply {
		syns, err := st.List()
		if err != nil {
			return rows, err
		}
		if err := applyMoves(st, syns, moves, ix); err != nil {
			return rows, err
		}
		for _, id := range prune {
			ix.Remove(id)
		}
		if len(prune) > 0 {
			if err := ix.Save(); err != nil {
				return rows, fmt.Errorf("saving project index: %w", err)
			}
		}
		pruned := map[string]bool{}
		for _, id := range prune {
			pruned[id] = true
		}
		for i := range rows {
			switch {
			case rows[i].Status == RefreshRelocated:
				rows[i].Applied = true
			case pruned[rows[i].ID]:
				rows[i].Status, rows[i].Applied = RefreshPruned, true
			}
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
	return rows, nil
}

// entryResolves reports whether the entry's directory still exists, is a main checkout and
// carries the entry's project ID.
func entryResolves(e projectid.Entry) bool {
	info, err := os.Stat(e.Path)
	if err != nil || !info.IsDir() || isLinkedWorktree(e.Path) {
		return false
	}
	id, err := projectid.Read(e.Path)
	return err == nil && id == e.ID
}

// locateIndexed ties a stale index entry to one of the scanned directories.
func locateIndexed(cands []candidate, e projectid.Entry) Location {
	if loc, ok := locateByID(cands, e.ID); ok {
		return loc
	}
	// The remaining evidence only applies to directories that carry no identity of their own
	// (a fresh clone, as .dendrite/ is not committed) and are not linked worktrees.
	var byRoot, byRemote []string
	for _, c := range cands {
		if c.id != "" || c.worktree {
			continue
		}
		remoteOK := e.Remote != "" && projectid.NormalizeRemote(projectid.GitRemote(c.path)) == e.Remote
		if e.RootCommit != "" && projectid.GitRootCommit(c.path) == e.RootCommit && (e.Remote == "" || remoteOK) {
			byRoot = append(byRoot, c.path)
		} else if remoteOK {
			byRemote = append(byRemote, c.path)
		}
	}
	for _, step := range []struct {
		hits  []string
		match MatchKind
	}{{byRoot, MatchRootCommit}, {byRemote, MatchRemote}} {
		sort.Strings(step.hits)
		switch len(step.hits) {
		case 0:
		case 1:
			return Location{Path: step.hits[0], Match: step.match}
		default:
			return Location{Match: step.match, Candidates: step.hits}
		}
	}
	return Location{}
}

func dedupe(paths []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range paths {
		if p = filepath.Clean(p); !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}
