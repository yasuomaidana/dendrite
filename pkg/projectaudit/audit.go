// Package projectaudit checks the project rule: every editable repository is a
// Plane project, and repositories that depend on each other share a synapse.
package projectaudit

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"Dendrite/pkg/projectid"
	"Dendrite/pkg/synapse"
	"Dendrite/pkg/tracker/plane"
)

// Repo is one git repository considered by the audit.
type Repo struct {
	Root     string
	Name     string
	Remote   string // normalized origin, empty for local-only repositories
	Editable bool
	Link     *plane.ProjectLink
	Synapses []string
}

// Edge is a dependency of one repository on another.
type Edge struct {
	From string // dependent repository root
	To   string // dependency repository root
	Via  string // manifest that declares it, relative to From
}

// Report lists the rule violations among the audited repositories.
type Report struct {
	Repos          []Repo
	Edges          []Edge
	MissingProject []Repo // editable but without .dendrite/plane.json
	Unsynapsed     []Edge // related editable repositories that share no synapse
}

var (
	skipDirs       = map[string]bool{"target": true, "node_modules": true, "vendor": true, "dist": true, "build": true}
	pathDepRegex   = regexp.MustCompile(`\bpath\s*=\s*"([^"]+)"`)
	goReplaceRegex = regexp.MustCompile(`=>\s*(\.{1,2}/\S*|/\S+)`)
	goModuleRegex  = regexp.MustCompile(`^module\s+(\S+)`)
	goRequireRegex = regexp.MustCompile(`^\s*(?:require\s+)?([a-zA-Z0-9][^\s]*\.[^\s]+/[^\s]+)\s+v`)
	npmFileRegex   = regexp.MustCompile(`"(?:file|link):([^"]+)"`)
)

// Discover returns the git repositories that are direct children of the given roots.
// Linked worktrees (where .git is a file, e.g. dendrite-work/ copies) are working copies of
// another repository, not projects of their own, and are skipped.
func Discover(roots []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			dir := filepath.Join(root, e.Name())
			if info, err := os.Stat(filepath.Join(dir, ".git")); err == nil && info.IsDir() && !seen[dir] {
				seen[dir] = true
				out = append(out, dir)
			}
		}
	}
	sort.Strings(out)
	return out
}

// OwnerOf returns the account part of a normalized remote ("github.com/acme/x" -> "acme").
func OwnerOf(remote string) string {
	parts := strings.Split(remote, "/")
	if len(parts) < 3 {
		return ""
	}
	return strings.ToLower(parts[len(parts)-2])
}

// Inspect loads a repository's remote, editability, Plane link and synapse membership.
// A repository is editable when it has no remote (local work) or its origin belongs to one of owners.
func Inspect(root string, owners []string, syns []*synapse.Synapse) Repo {
	r := Repo{Root: root, Name: filepath.Base(root), Remote: projectid.NormalizeRemote(projectid.GitRemote(root))}
	r.Editable = r.Remote == ""
	for _, o := range owners {
		if r.Remote != "" && strings.EqualFold(OwnerOf(r.Remote), o) {
			r.Editable = true
		}
	}
	if l, err := plane.ReadLink(root); err == nil {
		r.Link = l
	}
	for _, s := range syns {
		for _, m := range s.Members {
			if dir, err := synapse.Resolve(m); err == nil && filepath.Clean(dir) == filepath.Clean(root) {
				r.Synapses = append(r.Synapses, s.ID)
				break
			}
		}
	}
	return r
}

// Dependencies finds manifests in root that point into one of the other repositories:
// path dependencies (Cargo, pyproject), go.mod replaces and requires, and npm file:/link: specs.
func Dependencies(root string, others []string) []Edge {
	modules := map[string]string{} // go module path -> repo root
	for _, o := range others {
		if mod := goModulePath(filepath.Join(o, "go.mod")); mod != "" {
			modules[mod] = o
		}
	}
	seen := map[string]bool{}
	var edges []Edge
	add := func(target, manifest string) {
		owner := containingRepo(target, others)
		if owner == "" || owner == root || seen[owner] {
			return
		}
		seen[owner] = true
		rel, _ := filepath.Rel(root, manifest)
		edges = append(edges, Edge{From: root, To: owner, Via: rel})
	}
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != root && (skipDirs[d.Name()] || strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			if strings.Count(strings.TrimPrefix(path, root), string(os.PathSeparator)) > 3 {
				return filepath.SkipDir
			}
			return nil
		}
		dir := filepath.Dir(path)
		switch d.Name() {
		case "Cargo.toml", "pyproject.toml":
			for _, m := range matchLines(path, pathDepRegex) {
				add(resolve(dir, m), path)
			}
		case "go.mod":
			for _, m := range matchLines(path, goReplaceRegex) {
				add(resolve(dir, m), path)
			}
			for _, m := range matchLines(path, goRequireRegex) {
				if target, ok := modules[m]; ok {
					add(target, path)
				}
			}
		case "package.json":
			for _, m := range matchLines(path, npmFileRegex) {
				add(resolve(dir, m), path)
			}
		}
		return nil
	})
	return edges
}

// Audit applies the rule to the given repositories.
func Audit(roots []string, owners []string, syns []*synapse.Synapse) Report {
	var rep Report
	byRoot := map[string]Repo{}
	for _, root := range roots {
		r := Inspect(root, owners, syns)
		rep.Repos = append(rep.Repos, r)
		byRoot[root] = r
		if r.Editable && r.Link == nil {
			rep.MissingProject = append(rep.MissingProject, r)
		}
	}
	for _, root := range roots {
		for _, e := range Dependencies(root, roots) {
			rep.Edges = append(rep.Edges, e)
			from, to := byRoot[e.From], byRoot[e.To]
			if from.Editable && to.Editable && !shareSynapse(from, to) {
				rep.Unsynapsed = append(rep.Unsynapsed, e)
			}
		}
	}
	return rep
}

func shareSynapse(a, b Repo) bool {
	for _, x := range a.Synapses {
		for _, y := range b.Synapses {
			if x == y {
				return true
			}
		}
	}
	return false
}

func containingRepo(path string, roots []string) string {
	best := ""
	for _, r := range roots {
		if (path == r || strings.HasPrefix(path, r+string(os.PathSeparator))) && len(r) > len(best) {
			best = r
		}
	}
	return best
}

func resolve(dir, p string) string {
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Clean(filepath.Join(dir, p))
}

func goModulePath(path string) string {
	if m := matchLines(path, goModuleRegex); len(m) > 0 {
		return m[0]
	}
	return ""
}

func matchLines(path string, re *regexp.Regexp) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		for _, m := range re.FindAllStringSubmatch(sc.Text(), -1) {
			out = append(out, m[1])
		}
	}
	return out
}
