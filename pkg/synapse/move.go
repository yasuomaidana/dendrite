package synapse

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"Dendrite/pkg/fsutil"
	"Dendrite/pkg/projectid"
)

// osRename is an internal hook for filesystem renames, overridden in tests
// to simulate cross-device (syscall.EXDEV) moves and rename failures.
var osRename = os.Rename

// MoveOptions configures Move.
type MoveOptions struct {
	Index *projectid.Index // optional; updated and saved when non-nil
}

// MoveResult reports what Move changed.
type MoveResult struct {
	ProjectID   string   `json:"project_id"`
	OldPath     string   `json:"old_path"`
	NewPath     string   `json:"new_path"`
	Synapses    []string `json:"synapses"` // ids whose manifests were rewritten, sorted
	CrossDevice bool     `json:"cross_device"`
}

// Move relocates the project identified by ref (a member alias — must be unique across all synapses,
// else error listing the synapse ids — or a project ID) to newPath and rebinds every reference:
// synapse manifests (PathHint), the project index, and pool.json files (via RebindPoolFiles) —
// reuse applyMoves with a single-entry moves map {id: Location{Path: abs(newPath), Match: MatchID}}.
func Move(st *Store, ref, newPath string, opts MoveOptions) (MoveResult, error) {
	syns, err := st.List()
	if err != nil {
		return MoveResult{}, fmt.Errorf("listing synapses: %w", err)
	}

	projectID, currentPath, err := resolveRef(syns, ref)
	if err != nil {
		return MoveResult{}, err
	}

	absNew, _ := filepath.Abs(newPath)
	absNew = filepath.Clean(absNew)

	absOld, _ := filepath.Abs(currentPath)
	absOld = filepath.Clean(absOld)

	if absNew == absOld {
		return MoveResult{}, fmt.Errorf("cannot move project to the same path: %s", absNew)
	}

	if _, err := os.Lstat(absNew); err == nil {
		return MoveResult{}, fmt.Errorf("destination path %s already exists", absNew)
	}

	rel, err := filepath.Rel(absOld, absNew)
	if err == nil && !strings.HasPrefix(rel, "..") && rel != "." {
		return MoveResult{}, fmt.Errorf("destination %s is inside source %s", absNew, absOld)
	}

	snap := takeSnapshot(st, opts.Index, poolFileCandidates(syns, opts.Index))

	parent := filepath.Dir(absNew)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return MoveResult{}, fmt.Errorf("creating parent directory %s: %w", parent, err)
	}

	crossDevice := false
	if err := osRename(absOld, absNew); err != nil {
		if errors.Is(err, syscall.EXDEV) {
			if err := copyTree(absOld, absNew); err != nil {
				_ = os.RemoveAll(absNew)
				return MoveResult{}, fmt.Errorf("cross-device copy: %w", err)
			}
			_ = os.RemoveAll(absOld)
			crossDevice = true
		} else {
			return MoveResult{}, fmt.Errorf("renaming directory: %w", err)
		}
	}

	moves := map[string]Location{
		projectID: {Path: absNew, Match: MatchID},
	}
	if err := applyMoves(st, syns, moves, opts.Index); err != nil {
		snap.rollback(st, opts.Index, absOld, absNew, crossDevice)
		return MoveResult{}, fmt.Errorf("apply moves failed (rolled back): %w", err)
	}

	rewrittenSynapses := make([]string, 0, len(syns))
	for _, s := range syns {
		if _, ok := s.Member(projectID); ok {
			rewrittenSynapses = append(rewrittenSynapses, s.ID)
		}
	}
	sort.Strings(rewrittenSynapses)

	return MoveResult{
		ProjectID:   projectID,
		OldPath:     absOld,
		NewPath:     absNew,
		Synapses:    rewrittenSynapses,
		CrossDevice: crossDevice,
	}, nil
}

// resolveRef identifies target project ID and its current path from ref.
func resolveRef(syns []*Synapse, ref string) (string, string, error) {
	if ref == "" {
		return "", "", errors.New("empty project ref")
	}

	var idMatches []Member
	for _, s := range syns {
		for _, m := range s.Members {
			if m.ProjectID == ref {
				idMatches = append(idMatches, m)
			}
		}
	}

	var targetID string
	if len(idMatches) > 0 {
		targetID = ref
	} else {
		// The same project may carry the same alias in several synapses; the alias is
		// only ambiguous when it names different projects.
		synapsesByProject := map[string][]string{}
		for _, s := range syns {
			for _, m := range s.Members {
				if m.Alias == ref {
					synapsesByProject[m.ProjectID] = append(synapsesByProject[m.ProjectID], s.ID)
					targetID = m.ProjectID
				}
			}
		}

		if len(synapsesByProject) == 0 {
			return "", "", fmt.Errorf("unknown project ref %q", ref)
		}

		if len(synapsesByProject) > 1 {
			var ids []string
			for _, synIDs := range synapsesByProject {
				ids = append(ids, synIDs...)
			}
			sort.Strings(ids)
			return "", "", fmt.Errorf("ambiguous alias %q found in synapses: %s", ref, strings.Join(ids, ", "))
		}
	}

	var currentPath string
	for _, s := range syns {
		for _, m := range s.Members {
			if m.ProjectID == targetID {
				resPath, err := Resolve(m)
				if err != nil {
					return "", "", fmt.Errorf("member %q (%s) does not resolve at %q: run 'dendrite synapse heal' first: %w", m.Alias, m.ProjectID, m.PathHint, err)
				}
				if currentPath == "" {
					currentPath = resPath
				}
			}
		}
	}

	return targetID, currentPath, nil
}

// copyTree recursively copies the directory tree from src to dst, preserving file modes
// and symlinks (without following symlinks).
func copyTree(src, dst string) error {
	srcInfo, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if !srcInfo.IsDir() {
		return copyFileOrSymlink(src, dst, srcInfo)
	}

	if err := os.MkdirAll(dst, srcInfo.Mode().Perm()); err != nil {
		return err
	}
	_ = os.Chmod(dst, srcInfo.Mode().Perm())

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		s := filepath.Join(src, entry.Name())
		d := filepath.Join(dst, entry.Name())
		if err := copyTree(s, d); err != nil {
			return err
		}
	}
	return nil
}

// copyFileOrSymlink copies a single regular file or symlink.
func copyFileOrSymlink(src, dst string, info fs.FileInfo) error {
	if info.Mode()&os.ModeSymlink != 0 {
		target, _ := os.Readlink(src)
		return os.Symlink(target, dst)
	}

	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dst, data, info.Mode().Perm()); err != nil {
		return err
	}
	return os.Chmod(dst, info.Mode().Perm())
}

// snapshot stores in-memory snapshots of manifests, index and pool files prior to relocation.
type snapshot struct {
	manifests map[string][]byte
	// pools holds pool.json contents keyed by their pre-move path; files inside the
	// moved project are restored after the directory itself is moved back.
	pools        map[string][]byte
	indexPath    string
	indexBytes   []byte
	indexExisted bool
}

// takeSnapshot reads all store manifests, the index file and poolFiles into memory.
func takeSnapshot(st *Store, ix *projectid.Index, poolFiles []string) *snapshot {
	snap := &snapshot{
		manifests: make(map[string][]byte),
		pools:     make(map[string][]byte),
	}
	for _, f := range poolFiles {
		if data, err := os.ReadFile(f); err == nil {
			snap.pools[f] = data
		}
	}
	entries, err := os.ReadDir(st.Dir())
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".toml") {
				continue
			}
			path := filepath.Join(st.Dir(), entry.Name())
			if data, err := os.ReadFile(path); err == nil {
				snap.manifests[path] = data
			}
		}
	}

	if ix != nil {
		p := ix.Path()
		snap.indexPath = p
		if p != "" {
			if data, err := os.ReadFile(p); err == nil {
				snap.indexExisted = true
				snap.indexBytes = data
			}
		}
	}
	return snap
}

// rollback restores the directory to oldPath and manifests, index and pool files to their snapshotted state.
func (snap *snapshot) rollback(st *Store, ix *projectid.Index, oldPath, newPath string, crossDevice bool) {
	if crossDevice {
		if err := copyTree(newPath, oldPath); err == nil {
			_ = os.RemoveAll(newPath)
		}
	} else {
		_ = osRename(newPath, oldPath)
	}

	if entries, err := os.ReadDir(st.Dir()); err == nil {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".toml") {
				continue
			}
			path := filepath.Join(st.Dir(), entry.Name())
			if _, ok := snap.manifests[path]; !ok {
				_ = os.Remove(path)
			}
		}
	}
	for path, data := range snap.manifests {
		current, err := os.ReadFile(path)
		if err == nil && bytes.Equal(current, data) {
			continue
		}
		_ = fsutil.WriteFileAtomic(path, data)
	}

	for path, data := range snap.pools {
		current, err := os.ReadFile(path)
		if err == nil && bytes.Equal(current, data) {
			continue
		}
		_ = fsutil.WriteFileAtomic(path, data)
	}

	if ix != nil && snap.indexPath != "" {
		if snap.indexExisted {
			_ = fsutil.WriteFileAtomic(snap.indexPath, snap.indexBytes)
			var restored projectid.Index
			if err := json.Unmarshal(snap.indexBytes, &restored); err == nil {
				ix.Projects = restored.Projects
				ix.Version = restored.Version
			}
		} else {
			_ = os.Remove(snap.indexPath)
			ix.Projects = make(map[string]projectid.Entry)
		}
	}
}
