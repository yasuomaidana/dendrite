package workcopy

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Adopt moves the repository's linked worktrees that live outside the root into it.
func (m *Manager) Adopt(ctx context.Context, opts GCOptions) ([]Report, error) {
	l, err := m.layout(ctx)
	if err != nil {
		return nil, err
	}
	entries, err := m.worktrees(ctx)
	if err != nil {
		return nil, err
	}
	cwd := m.cwd()
	var reports []Report
	for _, e := range entries {
		if isWithin(l.root, e.path) || isWithin(l.top, e.path) {
			continue // managed copies, the main checkout and in-repo state dirs stay put
		}
		r := Report{Path: e.path, Branch: e.branch, Action: "kept"}
		name := filepath.Base(e.path)
		if !strings.HasPrefix(name, l.repoName+"-") {
			name = l.repoName + "-" + name
		}
		dest := filepath.Join(l.root, name)
		switch {
		case cwd != "" && isWithin(e.path, cwd):
			r.Reason = "current working directory"
		case opts.InUse != nil && opts.InUse(e.path):
			r.Reason = "used by an in-flight agent job"
		case dirExists(dest):
			r.Reason = dest + " already exists"
		case opts.DryRun:
			r.Action, r.Reason = "would move", "to "+dest
		default:
			r.Action, r.Reason = m.move(ctx, l.root, e.path, dest)
		}
		reports = append(reports, r)
	}
	return reports, nil
}

func (m *Manager) move(ctx context.Context, root, src, dest string) (action, reason string) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "kept", err.Error()
	}
	if _, err := m.git(ctx, m.repoDir, "worktree", "move", src, dest); err != nil {
		return "kept", err.Error()
	}
	return "moved", "to " + dest
}

func metaPath(copyPath string) string {
	return filepath.Join(filepath.Dir(copyPath), metaDirName, filepath.Base(copyPath)+".json")
}

func writeMeta(copyPath string, md meta) error {
	p := metaPath(copyPath)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(md, "", "  ")
	return os.WriteFile(p, data, 0o644)
}

func readMeta(copyPath string) (meta, error) {
	var md meta
	data, err := os.ReadFile(metaPath(copyPath))
	if err != nil {
		return md, err
	}
	err = json.Unmarshal(data, &md)
	return md, err
}

// isWithin reports whether path equals dir or lies beneath it.
func isWithin(dir, path string) bool {
	rel, err := filepath.Rel(Canon(dir), Canon(path))
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

// Canon resolves symlinks (e.g. /tmp -> /private/tmp) for the longest existing prefix.
func Canon(p string) string {
	p = filepath.Clean(p)
	if resolved, err := evalSymlinks(p); err == nil {
		return resolved
	}
	parent := filepath.Dir(p)
	if parent == p {
		return p
	}
	return filepath.Join(Canon(parent), filepath.Base(p))
}
