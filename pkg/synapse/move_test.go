package synapse

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"Dendrite/pkg/pool/dag"
	"Dendrite/pkg/projectid"
)

func TestMoveHappyPath(t *testing.T) {
	e := newHealEnv(t)
	// Two synapses share member 'core'. Member 'api' is only in syn1, member 'web' is only in syn2.
	coreDir, coreID := e.project("ws/core")
	apiDir, apiID := e.project("ws/api")
	webDir, webID := e.project("ws/web")

	e.synapse("core-api",
		Member{ProjectID: coreID, Alias: "core", Role: RoleProducer, PathHint: coreDir},
		Member{ProjectID: apiID, Alias: "api", Role: RoleConsumer, PathHint: apiDir},
	)
	e.synapse("core-web",
		Member{ProjectID: coreID, Alias: "core", Role: RoleProducer, PathHint: coreDir},
		Member{ProjectID: webID, Alias: "web", Role: RoleConsumer, PathHint: webDir},
	)
	if err := e.index.Save(); err != nil {
		t.Fatal(err)
	}

	// web's pool references api and core by path
	pool := filepath.Join(webDir, filepath.FromSlash(dag.DefaultPoolConfigFile))
	if err := os.MkdirAll(filepath.Dir(pool), 0o755); err != nil {
		t.Fatal(err)
	}
	poolJSON, _ := json.Marshal(dag.PoolConfig{
		Version: "1.0.0",
		Projects: []dag.ProjectInfo{
			{Name: "core", Path: coreDir},
			{Name: "api", Path: apiDir},
		},
	})
	if err := os.WriteFile(pool, poolJSON, 0o644); err != nil {
		t.Fatal(err)
	}

	// 1. Move by alias of a non-shared member ('api')
	newAPIDir := filepath.Join(e.root, "ws", "services", "api")
	res1, err := Move(e.store, "api", newAPIDir, MoveOptions{Index: e.index})
	if err != nil {
		t.Fatalf("Move by alias failed: %v", err)
	}
	if res1.ProjectID != apiID {
		t.Errorf("res1.ProjectID = %q, want %q", res1.ProjectID, apiID)
	}
	if res1.OldPath != apiDir {
		t.Errorf("res1.OldPath = %q, want %q", res1.OldPath, apiDir)
	}
	if res1.NewPath != newAPIDir {
		t.Errorf("res1.NewPath = %q, want %q", res1.NewPath, newAPIDir)
	}
	if len(res1.Synapses) != 1 || res1.Synapses[0] != "core-api" {
		t.Errorf("res1.Synapses = %v, want [core-api]", res1.Synapses)
	}
	if res1.CrossDevice {
		t.Errorf("res1.CrossDevice = true, want false")
	}

	// Verify apiDir is gone, newAPIDir exists, project.id intact
	if _, err := os.Stat(apiDir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected old apiDir to be gone, got err: %v", err)
	}
	if gotID, err := projectid.Read(newAPIDir); err != nil || gotID != apiID {
		t.Errorf("projectid.Read(newAPIDir) = %q, %v, want %q", gotID, err, apiID)
	}

	// Verify manifest
	s1, err := e.store.Load("core-api")
	if err != nil {
		t.Fatal(err)
	}
	if m, _ := s1.Member(apiID); m.PathHint != newAPIDir {
		t.Errorf("manifest core-api api PathHint = %q, want %q", m.PathHint, newAPIDir)
	}

	// Verify index
	if entry, ok := e.index.Lookup(apiID); !ok || entry.Path != newAPIDir {
		t.Errorf("index api entry = %+v, ok = %v", entry, ok)
	}

	// Verify pool.json
	pm := dag.NewPoolManager(pool)
	if err := pm.Load(); err != nil {
		t.Fatal(err)
	}
	for _, p := range pm.List() {
		if p.Name == "api" && p.Path != newAPIDir {
			t.Errorf("pool.json api path = %q, want %q", p.Path, newAPIDir)
		}
	}

	// 2. Move by project ID of the shared member ('core')
	newCoreDir := filepath.Join(e.root, "ws", "shared", "core")
	res2, err := Move(e.store, coreID, newCoreDir, MoveOptions{Index: e.index})
	if err != nil {
		t.Fatalf("Move by project ID failed: %v", err)
	}
	if res2.ProjectID != coreID {
		t.Errorf("res2.ProjectID = %q, want %q", res2.ProjectID, coreID)
	}
	if res2.OldPath != coreDir {
		t.Errorf("res2.OldPath = %q, want %q", res2.OldPath, coreDir)
	}
	if res2.NewPath != newCoreDir {
		t.Errorf("res2.NewPath = %q, want %q", res2.NewPath, newCoreDir)
	}
	if len(res2.Synapses) != 2 || res2.Synapses[0] != "core-api" || res2.Synapses[1] != "core-web" {
		t.Errorf("res2.Synapses = %v, want [core-api, core-web]", res2.Synapses)
	}

	// Verify coreDir is gone, newCoreDir exists, project.id intact
	if _, err := os.Stat(coreDir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected old coreDir to be gone, got err: %v", err)
	}
	if gotID, err := projectid.Read(newCoreDir); err != nil || gotID != coreID {
		t.Errorf("projectid.Read(newCoreDir) = %q, %v, want %q", gotID, err, coreID)
	}

	// Verify both manifests
	s1, _ = e.store.Load("core-api")
	if m, _ := s1.Member(coreID); m.PathHint != newCoreDir {
		t.Errorf("manifest core-api core PathHint = %q, want %q", m.PathHint, newCoreDir)
	}
	s2, _ := e.store.Load("core-web")
	if m, _ := s2.Member(coreID); m.PathHint != newCoreDir {
		t.Errorf("manifest core-web core PathHint = %q, want %q", m.PathHint, newCoreDir)
	}

	// Verify index
	if entry, ok := e.index.Lookup(coreID); !ok || entry.Path != newCoreDir {
		t.Errorf("index core entry = %+v, ok = %v", entry, ok)
	}

	// Verify pool.json
	pm = dag.NewPoolManager(pool)
	if err := pm.Load(); err != nil {
		t.Fatal(err)
	}
	for _, p := range pm.List() {
		if p.Name == "core" && p.Path != newCoreDir {
			t.Errorf("pool.json core path = %q, want %q", p.Path, newCoreDir)
		}
	}
}

func TestMoveAmbiguousAlias(t *testing.T) {
	e := newHealEnv(t)
	dir1, id1 := e.project("p1")
	dir2, id2 := e.project("p2")
	e.synapse("syn-1",
		Member{ProjectID: id1, Alias: "dup", Role: RolePeer, PathHint: dir1},
		Member{ProjectID: id2, Alias: "other1", Role: RolePeer, PathHint: dir2},
	)
	e.synapse("syn-2",
		Member{ProjectID: id2, Alias: "dup", Role: RolePeer, PathHint: dir2},
		Member{ProjectID: id1, Alias: "other2", Role: RolePeer, PathHint: dir1},
	)

	newDst := filepath.Join(e.root, "new-p1")
	_, err := Move(e.store, "dup", newDst, MoveOptions{Index: e.index})
	if err == nil {
		t.Fatal("expected error for ambiguous alias, got nil")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "syn-1") || !strings.Contains(errStr, "syn-2") {
		t.Fatalf("expected error mentioning syn-1 and syn-2, got: %v", err)
	}
	if _, err := os.Stat(newDst); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("destination should not exist after error")
	}
}

func TestMoveErrorsLeaveNothingChanged(t *testing.T) {
	e := newHealEnv(t)
	dir, id := e.project("proj")
	peerDir := filepath.Join(e.root, "peer")
	_ = os.MkdirAll(peerDir, 0o755)
	peerID := "dpid_00000000000000000000000000000001"
	_ = writeProjectID(peerDir, peerID)
	e.synapse("syn",
		Member{ProjectID: id, Alias: "my-alias", Role: RolePeer, PathHint: dir},
		Member{ProjectID: peerID, Alias: "peer", Role: RolePeer, PathHint: peerDir},
	)
	manifestPath := filepath.Join(e.store.Dir(), "syn.toml")
	beforeManifest, _ := os.ReadFile(manifestPath)

	t.Run("unknown ref", func(t *testing.T) {
		dst := filepath.Join(e.root, "dst1")
		_, err := Move(e.store, "unknown", dst, MoveOptions{Index: e.index})
		if err == nil || !strings.Contains(err.Error(), "unknown") {
			t.Fatalf("expected unknown ref error, got: %v", err)
		}
		if _, err := os.Stat(dst); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("dst should not exist")
		}
	})

	t.Run("destination exists", func(t *testing.T) {
		dst := filepath.Join(e.root, "existing")
		if err := os.MkdirAll(dst, 0o755); err != nil {
			t.Fatal(err)
		}
		_, err := Move(e.store, "my-alias", dst, MoveOptions{Index: e.index})
		if err == nil || !strings.Contains(err.Error(), "already exists") {
			t.Fatalf("expected destination exists error, got: %v", err)
		}
		if _, err := os.Stat(dir); err != nil {
			t.Fatal("source should still exist")
		}
	})

	t.Run("destination inside source", func(t *testing.T) {
		dst := filepath.Join(dir, "nested", "dest")
		_, err := Move(e.store, "my-alias", dst, MoveOptions{Index: e.index})
		if err == nil || !strings.Contains(err.Error(), "inside source") {
			t.Fatalf("expected inside source error, got: %v", err)
		}
		if _, err := os.Stat(dst); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("dst should not exist")
		}
	})

	t.Run("same path", func(t *testing.T) {
		_, err := Move(e.store, "my-alias", dir, MoveOptions{Index: e.index})
		if err == nil || !strings.Contains(err.Error(), "same path") {
			t.Fatalf("expected same path error, got: %v", err)
		}
	})

	t.Run("unresolved member", func(t *testing.T) {
		ghostDir := filepath.Join(e.root, "ghost")
		_ = os.MkdirAll(ghostDir, 0o755)
		ghostID, _ := projectid.Ensure(ghostDir)
		e.synapse("syn-ghost",
			Member{ProjectID: ghostID, Alias: "ghost", Role: RolePeer, PathHint: ghostDir},
			Member{ProjectID: id, Alias: "p", Role: RolePeer, PathHint: dir},
		)
		// Remove ghostDir so it becomes unresolved
		_ = os.RemoveAll(ghostDir)

		dst := filepath.Join(e.root, "ghost-new")
		_, err := Move(e.store, "ghost", dst, MoveOptions{Index: e.index})
		if err == nil || !strings.Contains(err.Error(), "heal") {
			t.Fatalf("expected error mentioning heal, got: %v", err)
		}
		if _, err := os.Stat(dst); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("dst should not exist")
		}
	})

	// Assert manifest was unchanged throughout
	afterManifest, _ := os.ReadFile(manifestPath)
	if !bytes.Equal(beforeManifest, afterManifest) {
		t.Fatal("manifest should not have changed during error cases")
	}
}

func TestMoveRollback(t *testing.T) {
	e := newHealEnv(t)
	dir, id := e.project("proj")
	peerDir, peerID := e.project("peer")
	e.synapse("syn",
		Member{ProjectID: id, Alias: "proj", Role: RolePeer, PathHint: dir},
		Member{ProjectID: peerID, Alias: "peer", Role: RolePeer, PathHint: peerDir},
	)
	if err := e.index.Save(); err != nil {
		t.Fatal(err)
	}

	manifestPath := filepath.Join(e.store.Dir(), "syn.toml")
	beforeManifest, _ := os.ReadFile(manifestPath)
	indexPath := e.index.Path()
	beforeIndex, _ := os.ReadFile(indexPath)

	// Make store dir read-only so st.Save fails in applyMoves
	if err := os.Chmod(e.store.Dir(), 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(e.store.Dir(), 0o755) })

	dst := filepath.Join(e.root, "moved-proj")
	_, err := Move(e.store, "proj", dst, MoveOptions{Index: e.index})
	if err == nil || !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("expected rolled back error, got: %v", err)
	}

	// Verify directory is back at old path
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("directory should be back at old path: %v", err)
	}
	if _, err := os.Stat(dst); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("new path should not exist after rollback: %v", err)
	}
	if gotID, err := projectid.Read(dir); err != nil || gotID != id {
		t.Fatalf("project id after rollback = %q, want %q", gotID, id)
	}

	// Restore chmod so we can inspect
	_ = os.Chmod(e.store.Dir(), 0o755)
	afterManifest, _ := os.ReadFile(manifestPath)
	if !bytes.Equal(beforeManifest, afterManifest) {
		t.Fatalf("manifest should be byte-identical to before:\nbefore: %s\nafter:  %s", beforeManifest, afterManifest)
	}

	afterIndex, _ := os.ReadFile(indexPath)
	if !bytes.Equal(beforeIndex, afterIndex) {
		t.Fatalf("index should be byte-identical to before")
	}
}

func TestCopyTreeDirectly(t *testing.T) {
	temp := t.TempDir()
	src := filepath.Join(temp, "src")
	dst := filepath.Join(temp, "dst")

	// Nested dirs
	nestedDir := filepath.Join(src, "a", "b", "c")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	nestedFile := filepath.Join(nestedDir, "nested.txt")
	if err := os.WriteFile(nestedFile, []byte("hello nested"), 0o644); err != nil {
		t.Fatal(err)
	}

	// File with mode 0o600 preserved
	secretFile := filepath.Join(src, "secret.txt")
	if err := os.WriteFile(secretFile, []byte("super secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(secretFile, 0o600); err != nil {
		t.Fatal(err)
	}

	// Symlink preserved as a symlink
	symlinkPath := filepath.Join(src, "link.txt")
	if err := os.Symlink("secret.txt", symlinkPath); err != nil {
		t.Fatal(err)
	}

	// Run copyTree
	if err := copyTree(src, dst); err != nil {
		t.Fatalf("copyTree failed: %v", err)
	}

	// Verify nested file
	gotData, err := os.ReadFile(filepath.Join(dst, "a", "b", "c", "nested.txt"))
	if err != nil || string(gotData) != "hello nested" {
		t.Fatalf("nested file copy mismatch: %q, %v", gotData, err)
	}

	// Verify mode 0o600 preserved
	fi, err := os.Stat(filepath.Join(dst, "secret.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("secret.txt mode = %o, want 0600", fi.Mode().Perm())
	}

	// Verify symlink preserved as symlink
	dstLink := filepath.Join(dst, "link.txt")
	lfi, err := os.Lstat(dstLink)
	if err != nil {
		t.Fatal(err)
	}
	if lfi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("dst link is not a symlink: mode = %v", lfi.Mode())
	}
	target, err := os.Readlink(dstLink)
	if err != nil || target != "secret.txt" {
		t.Errorf("dst link target = %q, %v, want secret.txt", target, err)
	}

	// Error when dst parent is a file
	parentFile := filepath.Join(temp, "regular_file")
	if err := os.WriteFile(parentFile, []byte("file content"), 0o644); err != nil {
		t.Fatal(err)
	}
	badDst := filepath.Join(parentFile, "child")
	if err := copyTree(src, badDst); err == nil {
		t.Fatal("expected error when dst parent is a file, got nil")
	}
}

func TestMoveCrossDeviceAndRollback(t *testing.T) {
	e := newHealEnv(t)
	dir, id := e.project("proj")
	peerDir, peerID := e.project("peer")
	e.synapse("syn",
		Member{ProjectID: id, Alias: "proj", Role: RolePeer, PathHint: dir},
		Member{ProjectID: peerID, Alias: "peer", Role: RolePeer, PathHint: peerDir},
	)
	if err := e.index.Save(); err != nil {
		t.Fatal(err)
	}

	// Simulate cross-device: osRename returns syscall.EXDEV
	origRename := osRename
	defer func() { osRename = origRename }()

	osRename = func(oldpath, newpath string) error {
		return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: syscall.EXDEV}
	}

	dst := filepath.Join(e.root, "moved-cross")
	res, err := Move(e.store, "proj", dst, MoveOptions{Index: e.index})
	if err != nil {
		t.Fatalf("cross-device Move failed: %v", err)
	}
	if !res.CrossDevice {
		t.Errorf("res.CrossDevice = false, want true")
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("old dir should be removed after cross-device copy")
	}
	if gotID, err := projectid.Read(dst); err != nil || gotID != id {
		t.Errorf("new dir projectid = %q, want %q", gotID, id)
	}

	// Test cross-device rollback when applyMoves fails
	dir2, id2 := e.project("proj2")
	e.synapse("syn2",
		Member{ProjectID: id2, Alias: "proj2", Role: RolePeer, PathHint: dir2},
		Member{ProjectID: peerID, Alias: "peer2", Role: RolePeer, PathHint: peerDir},
	)
	if err := os.Chmod(e.store.Dir(), 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(e.store.Dir(), 0o755) })

	dst2 := filepath.Join(e.root, "moved-cross-fail")
	_, err = Move(e.store, "proj2", dst2, MoveOptions{Index: e.index})
	if err == nil || !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("expected cross-device rollback error, got: %v", err)
	}
	if _, err := os.Stat(dir2); err != nil {
		t.Errorf("dir2 should be restored at old path: %v", err)
	}
	if _, err := os.Stat(dst2); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("dst2 should be removed after rollback: %v", err)
	}
}

func TestMoveEdgeCasesForCoverage(t *testing.T) {
	e := newHealEnv(t)
	dir, id := e.project("p")
	peerDir, peerID := e.project("peer")
	e.synapse("s",
		Member{ProjectID: id, Alias: "p", Role: RolePeer, PathHint: dir},
		Member{ProjectID: peerID, Alias: "peer", Role: RolePeer, PathHint: peerDir},
	)

	t.Run("empty ref", func(t *testing.T) {
		dst := filepath.Join(e.root, "dst-empty")
		_, err := Move(e.store, "", dst, MoveOptions{Index: e.index})
		if err == nil || !strings.Contains(err.Error(), "empty project ref") {
			t.Fatalf("expected empty project ref error, got: %v", err)
		}
	})

	t.Run("move without index", func(t *testing.T) {
		dst := filepath.Join(e.root, "dst-no-index")
		res, err := Move(e.store, "p", dst, MoveOptions{})
		if err != nil {
			t.Fatalf("Move without index failed: %v", err)
		}
		if res.NewPath != dst {
			t.Errorf("res.NewPath = %q, want %q", res.NewPath, dst)
		}
		// Move back for subsequent tests
		_, err = Move(e.store, "p", dir, MoveOptions{})
		if err != nil {
			t.Fatalf("Move back failed: %v", err)
		}
	})

	t.Run("store list error", func(t *testing.T) {
		fileStore := NewStore(filepath.Join(t.TempDir(), "store_is_a_file"))
		_ = os.WriteFile(fileStore.Dir(), []byte("not a dir"), 0o644)
		_, err := Move(fileStore, "p", filepath.Join(e.root, "nowhere"), MoveOptions{})
		if err == nil || !strings.Contains(err.Error(), "listing synapses") {
			t.Fatalf("expected listing synapses error, got: %v", err)
		}
	})

	t.Run("parent directory cannot be created", func(t *testing.T) {
		parentFile := filepath.Join(e.root, "file_blocking_parent")
		_ = os.WriteFile(parentFile, []byte("blocker"), 0o644)
		badDst := filepath.Join(parentFile, "child", "dest")
		_, err := Move(e.store, "p", badDst, MoveOptions{Index: e.index})
		if err == nil || !strings.Contains(err.Error(), "creating parent directory") {
			t.Fatalf("expected creating parent directory error, got: %v", err)
		}
	})

	t.Run("rename generic error", func(t *testing.T) {
		origRename := osRename
		defer func() { osRename = origRename }()
		osRename = func(oldpath, newpath string) error {
			return errors.New("simulated disk error")
		}
		dst := filepath.Join(e.root, "dst-rename-fail")
		_, err := Move(e.store, "p", dst, MoveOptions{Index: e.index})
		if err == nil || !strings.Contains(err.Error(), "renaming directory") {
			t.Fatalf("expected renaming directory error, got: %v", err)
		}
	})

	t.Run("cross device copy error", func(t *testing.T) {
		origRename := osRename
		defer func() { osRename = origRename }()
		osRename = func(oldpath, newpath string) error {
			return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: syscall.EXDEV}
		}
		// An unreadable file inside the source makes the recursive copy fail.
		unreadable := filepath.Join(dir, "secret.bin")
		if err := os.WriteFile(unreadable, []byte("x"), 0o000); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(unreadable)
		badDst := filepath.Join(e.root, "dst-cross-fail")
		_, err := Move(e.store, "p", badDst, MoveOptions{Index: e.index})
		if _, statErr := os.Stat(badDst); !os.IsNotExist(statErr) {
			t.Fatalf("partial copy at %s was not cleaned up", badDst)
		}
		if _, statErr := os.Stat(dir); statErr != nil {
			t.Fatalf("source must survive a failed cross-device copy: %v", statErr)
		}
		if err == nil || !strings.Contains(err.Error(), "cross-device copy") {
			t.Fatalf("expected cross-device copy error, got: %v", err)
		}
	})

	t.Run("rollback without preexisting index file", func(t *testing.T) {
		freshIndexPath := filepath.Join(e.root, "fresh_projects.json")
		freshIx, err := projectid.LoadIndex(freshIndexPath)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(e.store.Dir(), 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(e.store.Dir(), 0o755) })

		dst := filepath.Join(e.root, "dst-rollback-fresh-ix")
		_, err = Move(e.store, "p", dst, MoveOptions{Index: freshIx})
		if err == nil || !strings.Contains(err.Error(), "rolled back") {
			t.Fatalf("expected rolled back error, got: %v", err)
		}
		_ = os.Chmod(e.store.Dir(), 0o755)
		if _, err := os.Stat(freshIndexPath); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("fresh index path should not exist after rollback: %v", err)
		}
		if len(freshIx.Projects) != 0 {
			t.Errorf("fresh index projects should be empty after rollback, got %d", len(freshIx.Projects))
		}
	})

	t.Run("snapshot with non-toml and subdirectory in store", func(t *testing.T) {
		subDir := filepath.Join(e.store.Dir(), "sub_ignored")
		_ = os.MkdirAll(subDir, 0o755)
		readme := filepath.Join(e.store.Dir(), "README.md")
		_ = os.WriteFile(readme, []byte("docs"), 0o644)
		defer func() {
			_ = os.RemoveAll(subDir)
			_ = os.Remove(readme)
		}()

		dst := filepath.Join(e.root, "dst-snapshot-ignore")
		res, err := Move(e.store, "p", dst, MoveOptions{Index: e.index})
		if err != nil {
			t.Fatalf("Move failed with extra store files: %v", err)
		}
		// Move back
		_, _ = Move(e.store, "p", dir, MoveOptions{Index: e.index})
		if res.ProjectID != id {
			t.Errorf("ProjectID = %q, want %q", res.ProjectID, id)
		}
	})

	t.Run("rollback restores changed manifest and removes untracked manifest", func(t *testing.T) {
		snap := takeSnapshot(e.store, e.index)
		manifestPath := filepath.Join(e.store.Dir(), "s.toml")
		_ = os.WriteFile(manifestPath, []byte("corrupted"), 0o644)
		extraManifest := filepath.Join(e.store.Dir(), "extra.toml")
		_ = os.WriteFile(extraManifest, []byte("id = 'extra'"), 0o644)

		snap.rollback(e.store, e.index, dir, filepath.Join(e.root, "nowhere"), false)
		if _, err := os.Stat(extraManifest); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("extra manifest should be removed by rollback: %v", err)
		}
		after, _ := os.ReadFile(manifestPath)
		if !bytes.Equal(after, snap.manifests[manifestPath]) {
			t.Errorf("manifest not restored by rollback")
		}
	})
}

func TestCopyTreeEdgeCases(t *testing.T) {
	temp := t.TempDir()

	t.Run("copy single file directly", func(t *testing.T) {
		srcFile := filepath.Join(temp, "file.txt")
		_ = os.WriteFile(srcFile, []byte("single file"), 0o644)
		dstFile := filepath.Join(temp, "file_copy.txt")
		if err := copyTree(srcFile, dstFile); err != nil {
			t.Fatalf("copyTree on single file failed: %v", err)
		}
		got, _ := os.ReadFile(dstFile)
		if string(got) != "single file" {
			t.Errorf("copyTree file content = %q, want 'single file'", string(got))
		}
	})

	t.Run("copy nonexistent source", func(t *testing.T) {
		err := copyTree(filepath.Join(temp, "does_not_exist"), filepath.Join(temp, "dst"))
		if err == nil {
			t.Fatal("expected error for nonexistent source, got nil")
		}
	})

	t.Run("copy unreadable directory", func(t *testing.T) {
		unreadable := filepath.Join(temp, "unreadable_dir")
		_ = os.MkdirAll(unreadable, 0o755)
		_ = os.WriteFile(filepath.Join(unreadable, "child.txt"), []byte("c"), 0o644)
		if err := os.Chmod(unreadable, 0o000); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(unreadable, 0o755) })

		err := copyTree(unreadable, filepath.Join(temp, "dst_unreadable"))
		if err == nil {
			t.Fatal("expected error for unreadable directory, got nil")
		}
	})
}

func TestCopyFileIntoReadOnlyDir(t *testing.T) {
	src := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	ro := t.TempDir()
	if err := os.Chmod(ro, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(ro, 0o755) })
	if err := copyTree(src, filepath.Join(ro, "f")); err == nil {
		t.Fatal("expected write error copying into a read-only directory")
	}
}

func TestRollbackIgnoresNonManifestEntries(t *testing.T) {
	e := newHealEnv(t)
	aDir, aID := e.project("a")
	bDir, bID := e.project("b")
	e.synapse("ab", Member{ProjectID: aID, Alias: "a", Role: RolePeer, PathHint: aDir}, Member{ProjectID: bID, Alias: "b", Role: RolePeer, PathHint: bDir})
	snap := takeSnapshot(e.store, nil)

	notes := filepath.Join(e.store.Dir(), "README.md")
	sub := filepath.Join(e.store.Dir(), "archive.toml")
	if err := os.WriteFile(notes, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(e.root, "moved-a")
	if err := os.Rename(aDir, moved); err != nil {
		t.Fatal(err)
	}
	snap.rollback(e.store, nil, aDir, moved, false)
	for _, p := range []string{notes, sub, aDir} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("%s should survive rollback: %v", p, err)
		}
	}
}
