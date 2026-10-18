package synapse

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"Dendrite/pkg/pool/dag"
	"Dendrite/pkg/projectid"
)

// StatusAmbiguous marks a member whose new location could not be pinned to a single candidate.
const StatusAmbiguous MemberStatus = "AMBIGUOUS"

// DefaultHealDepth bounds how deep Locate descends below each search root.
const DefaultHealDepth = 4

// MatchKind records which evidence tied a member to a discovered directory.
type MatchKind string

const (
	MatchNone   MatchKind = ""
	MatchID     MatchKind = "id"
	MatchRemote MatchKind = "remote"
	MatchName   MatchKind = "name"
)

// skipDirs are never descended into while scanning for relocated projects.
var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "target": true, "vendor": true,
	"dist": true, ".idea": true, ".dendrite": true, "build": true,
}

// HealOptions configures Locate and Heal.
type HealOptions struct {
	// Roots are extra directories to search in addition to the defaults.
	Roots []string
	// MaxDepth bounds directory descent below each root (DefaultHealDepth when <= 0).
	MaxDepth int
	// Apply rewrites manifests, the project index and pool files for unambiguous matches.
	Apply bool
	// Index is the global project index; used for remote/name evidence and rewritten on Apply.
	Index *projectid.Index
	// NoDefaultRoots disables the IDE project folders under the user's home directory.
	NoDefaultRoots bool
}

// Location is the outcome of locating one member.
type Location struct {
	Path       string
	Match      MatchKind
	Candidates []string
}

// HealRow is one line of the health matrix.
type HealRow struct {
	Synapse    string       `json:"synapse"`
	Alias      string       `json:"alias"`
	ProjectID  string       `json:"project_id"`
	Status     MemberStatus `json:"status"`
	OldPath    string       `json:"old_path"`
	NewPath    string       `json:"new_path,omitempty"`
	Match      MatchKind    `json:"match,omitempty"`
	Candidates []string     `json:"candidates,omitempty"`
	Applied    bool         `json:"applied"`
}

// candidate is a directory discovered during a scan.
type candidate struct {
	path string
	id   string
}

// searchRoots returns the ordered, de-duplicated directories to scan for member m.
func searchRoots(m Member, opts HealOptions, extra ...string) []string {
	var roots []string
	roots = append(roots, extra...)
	if m.PathHint != "" {
		parent := filepath.Dir(filepath.Clean(m.PathHint))
		roots = append(roots, parent, filepath.Dir(parent))
	}
	roots = append(roots, opts.Roots...)
	if !opts.NoDefaultRoots {
		if home, err := os.UserHomeDir(); err == nil {
			for _, d := range []string{"GoLandProjects", "RustRoverProjects", "WebstormProjects", "PycharmProjects", "IdeaProjects", "src", "code"} {
				roots = append(roots, filepath.Join(home, d))
			}
		}
	}

	seen := map[string]bool{}
	var out []string
	for _, r := range roots {
		if r == "" {
			continue
		}
		abs, err := filepath.Abs(r)
		if err != nil || seen[abs] {
			continue
		}
		if info, err := os.Stat(abs); err != nil || !info.IsDir() {
			continue
		}
		seen[abs] = true
		out = append(out, abs)
	}
	return out
}

// scan walks the roots and returns every directory that looks like a project root
// (has a .dendrite/project.id or a .git entry).
func scan(roots []string, maxDepth int) []candidate {
	if maxDepth <= 0 {
		maxDepth = DefaultHealDepth
	}
	seen := map[string]bool{}
	var out []candidate
	for _, root := range roots {
		base := strings.Count(root, string(os.PathSeparator))
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || !d.IsDir() {
				return nil
			}
			if path != root && (skipDirs[d.Name()] || strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			if strings.Count(path, string(os.PathSeparator))-base > maxDepth {
				return filepath.SkipDir
			}
			if seen[path] {
				return filepath.SkipDir
			}
			id, idErr := projectid.Read(path)
			_, gitErr := os.Stat(filepath.Join(path, ".git"))
			if idErr == nil || gitErr == nil {
				seen[path] = true
				out = append(out, candidate{path: path, id: id})
			}
			return nil
		})
	}
	return out
}

// Locate searches for the current directory of member m. Evidence is ranked:
// an exact project ID match wins; otherwise a unique origin-remote match; a
// basename match is only ever reported as a candidate list (never a Path).
func Locate(m Member, opts HealOptions, hints ...string) Location {
	if StatusOf(m) == StatusOK {
		return Location{Path: m.PathHint, Match: MatchID}
	}
	cands := scan(searchRoots(m, opts, hints...), opts.MaxDepth)

	for _, c := range cands {
		if c.id == m.ProjectID {
			return Location{Path: c.path, Match: MatchID}
		}
	}

	var remote, name string
	if opts.Index != nil {
		if e, ok := opts.Index.Lookup(m.ProjectID); ok {
			remote, name = e.Remote, e.Name
		}
	}
	if remote != "" {
		var hits []string
		for _, c := range cands {
			// A directory that already carries a different identity is another project.
			if c.id != "" {
				continue
			}
			if projectid.NormalizeRemote(projectid.GitRemote(c.path)) == remote {
				hits = append(hits, c.path)
			}
		}
		if len(hits) == 1 {
			return Location{Path: hits[0], Match: MatchRemote}
		}
		if len(hits) > 1 {
			sort.Strings(hits)
			return Location{Match: MatchRemote, Candidates: hits}
		}
	}

	names := map[string]bool{m.Alias: true, filepath.Base(m.PathHint): true}
	if name != "" {
		names[name] = true
	}
	var hits []string
	for _, c := range cands {
		if c.id == "" && names[filepath.Base(c.path)] {
			hits = append(hits, c.path)
		}
	}
	if len(hits) > 0 {
		sort.Strings(hits)
		return Location{Match: MatchName, Candidates: hits}
	}
	return Location{}
}

// Heal checks every member of every synapse in st, locates moved members and,
// when opts.Apply is set, rebinds unambiguous matches everywhere they are referenced.
func Heal(st *Store, opts HealOptions) ([]HealRow, error) {
	syns, err := st.List()
	if err != nil {
		return nil, err
	}
	// Locate each distinct project once even when it belongs to several synapses.
	located := map[string]Location{}
	var rows []HealRow
	for _, s := range syns {
		for _, m := range s.Members {
			row := HealRow{Synapse: s.ID, Alias: m.Alias, ProjectID: m.ProjectID, OldPath: m.PathHint}
			status := StatusOf(m)
			if status == StatusOK {
				row.Status = StatusOK
				rows = append(rows, row)
				continue
			}
			loc, ok := located[m.ProjectID]
			if !ok {
				loc = Locate(m, opts)
				located[m.ProjectID] = loc
			}
			row.Match, row.Candidates = loc.Match, loc.Candidates
			switch {
			case loc.Path != "":
				row.Status, row.NewPath = StatusMoved, loc.Path
			case len(loc.Candidates) > 0:
				row.Status = StatusAmbiguous
			default:
				row.Status = status
			}
			rows = append(rows, row)
		}
	}

	if !opts.Apply {
		return rows, nil
	}
	moves := map[string]Location{}
	for id, loc := range located {
		if loc.Path != "" {
			moves[id] = loc
		}
	}
	if err := applyMoves(st, syns, moves, opts.Index); err != nil {
		return rows, err
	}
	for i := range rows {
		if _, ok := moves[rows[i].ProjectID]; ok && rows[i].Status == StatusMoved {
			rows[i].Applied = true
		}
	}
	return rows, nil
}

// applyMoves rewrites every reference to the moved projects and re-derives each
// synapse's broken/active state from the healed member set.
func applyMoves(st *Store, syns []*Synapse, moves map[string]Location, ix *projectid.Index) error {
	oldToNew := map[string]string{}
	for id, loc := range moves {
		// A remote match is a fresh clone or a copy without identity: restore it first
		// so the member resolves when the synapse state is re-derived below.
		if loc.Match == MatchRemote {
			if err := writeProjectID(loc.Path, id); err != nil {
				return err
			}
		}
		if ix != nil {
			e, ok := ix.Lookup(id)
			if !ok {
				e = projectid.Entry{ID: id, Name: filepath.Base(loc.Path)}
			}
			if e.Path != "" && filepath.Clean(e.Path) != loc.Path {
				oldToNew[filepath.Clean(e.Path)] = loc.Path
			}
			e.Path = loc.Path
			ix.Upsert(e)
		}
	}

	for _, s := range syns {
		changed := false
		for i := range s.Members {
			loc, ok := moves[s.Members[i].ProjectID]
			if !ok || s.Members[i].PathHint == loc.Path {
				continue
			}
			if s.Members[i].PathHint != "" {
				oldToNew[filepath.Clean(s.Members[i].PathHint)] = loc.Path
			}
			s.Members[i].PathHint = loc.Path
			changed = true
		}
		healthy := allResolve(s)
		switch {
		case healthy && s.State == StateBroken:
			changed = s.Transition(StateActive) == nil || changed
		case !healthy && (s.State == StateActive || s.State == StateDrifted):
			changed = s.Transition(StateBroken) == nil || changed
		}
		if !changed {
			continue
		}
		if err := st.Save(s); err != nil {
			return fmt.Errorf("saving synapse %s: %w", s.ID, err)
		}
	}

	if ix != nil {
		if err := ix.Save(); err != nil {
			return fmt.Errorf("saving project index: %w", err)
		}
	}
	return RebindPoolFiles(poolFileCandidates(syns, ix), oldToNew)
}

// allResolve reports whether every member of s currently resolves.
func allResolve(s *Synapse) bool {
	for _, m := range s.Members {
		if StatusOf(m) != StatusOK {
			return false
		}
	}
	return true
}

// writeProjectID stamps an existing identity into dir.
func writeProjectID(dir, id string) error {
	if got, err := projectid.Read(dir); err == nil {
		if got != id {
			return fmt.Errorf("refusing to overwrite project id %s at %s", got, dir)
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Join(dir, ".dendrite"), 0o755); err != nil {
		return err
	}
	return os.WriteFile(projectid.Path(dir), []byte(id+"\n"), 0o644)
}

// poolFileCandidates lists the pool.json files of every known project.
func poolFileCandidates(syns []*Synapse, ix *projectid.Index) []string {
	seen := map[string]bool{}
	var out []string
	add := func(dir string) {
		if dir == "" {
			return
		}
		f := filepath.Join(dir, filepath.FromSlash(dag.DefaultPoolConfigFile))
		if !seen[f] {
			seen[f] = true
			out = append(out, f)
		}
	}
	for _, s := range syns {
		for _, m := range s.Members {
			add(m.PathHint)
		}
	}
	if ix != nil {
		for _, e := range ix.List() {
			add(e.Path)
		}
	}
	sort.Strings(out)
	return out
}

// RebindPoolFiles rewrites pool entries whose path appears in oldToNew. Missing files are skipped.
func RebindPoolFiles(files []string, oldToNew map[string]string) error {
	if len(oldToNew) == 0 {
		return nil
	}
	for _, f := range files {
		if _, err := os.Stat(f); errors.Is(err, fs.ErrNotExist) {
			continue
		}
		pm := dag.NewPoolManager(f)
		if err := pm.Load(); err != nil {
			return fmt.Errorf("loading %s: %w", f, err)
		}
		n := 0
		for old, nw := range oldToNew {
			n += pm.RebindPath(old, nw)
		}
		if n == 0 {
			continue
		}
		if err := pm.Save(); err != nil {
			return fmt.Errorf("saving %s: %w", f, err)
		}
	}
	return nil
}
