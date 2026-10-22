package synapse

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"Dendrite/pkg/pool/dag"
	"Dendrite/pkg/projectid"
)

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
		snap := takeSnapshot(e.store, e.index, nil)
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
	snap := takeSnapshot(e.store, nil, nil)

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

func TestMoveRollbackRestoresRewrittenPoolFiles(t *testing.T) {
	e := newHealEnv(t)
	dir, id := e.project("proj")
	peerDir, peerID := e.project("peer")
	e.synapse("syn",
		Member{ProjectID: id, Alias: "proj", Role: RolePeer, PathHint: dir},
		Member{ProjectID: peerID, Alias: "peer", Role: RolePeer, PathHint: peerDir},
	)

	writePool := func(root, content string) string {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(dag.DefaultPoolConfigFile))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	// The peer's pool references proj and is rebound first; the malformed pool
	// inside proj (rebound after the move, under its new path) then fails.
	peerPool := writePool(peerDir, `{"version":"1.0.0","projects":[{"name":"proj","path":"`+dir+`"}]}`)
	writePool(dir, "{")
	before, _ := os.ReadFile(peerPool)

	_, err := Move(e.store, "proj", filepath.Join(e.root, "zz-moved"), MoveOptions{Index: e.index})
	if err == nil || !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("expected rolled back error, got: %v", err)
	}
	if after, _ := os.ReadFile(peerPool); !bytes.Equal(before, after) {
		t.Fatalf("peer pool.json not restored:\nbefore: %s\nafter:  %s", before, after)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("directory should be back at old path: %v", err)
	}
}
