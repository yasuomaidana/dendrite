package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/projectid"
)

func TestPlaneProjectRefresh_RelocatesMovedProjectsAndPrunesVanishedOnes(t *testing.T) {
	plainOutput(t)
	t.Setenv("DENDRITE_HOME", t.TempDir())
	t.Setenv("DENDRITE_SEARCH_ROOTS", t.TempDir()) // never scan the real home folders

	parent := t.TempDir()
	moved := filepath.Join(parent, "old", "app")
	gone := filepath.Join(parent, "old", "gone")
	for _, d := range []string{moved, gone} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		migrateGit(t, d, "init", "-q")
	}
	indexPath, _ := projectid.DefaultIndexPath()
	ix, _ := projectid.LoadIndex(indexPath)
	movedEntry, err := ix.Register(moved)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ix.Register(gone); err != nil {
		t.Fatal(err)
	}
	if err := ix.Save(); err != nil {
		t.Fatal(err)
	}

	newHome := filepath.Join(parent, "new", "app")
	if err := os.MkdirAll(filepath.Dir(newHome), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(moved, newHome); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}

	// Dry run reports and changes nothing.
	var out bytes.Buffer
	roots := []string{filepath.Join(parent, "new")}
	if err := planeProjectRefresh(planeProjectRefreshOptions{Roots: roots, DryRun: true}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "1 relocated") || !strings.Contains(out.String(), "1 missing") || !strings.Contains(out.String(), "dry run") {
		t.Fatalf("dry run output:\n%s", out.String())
	}
	ix, _ = projectid.LoadIndex(indexPath)
	if e, _ := ix.Lookup(movedEntry.ID); e.Path != movedEntry.Path {
		t.Fatalf("dry run must not touch the index, got %q", e.Path)
	}

	// Apply without --prune relocates but keeps the missing entry; --prune then drops it.
	out.Reset()
	if err := planeProjectRefresh(planeProjectRefreshOptions{Roots: roots}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "use --prune") {
		t.Fatalf("expected the --prune hint:\n%s", out.String())
	}
	ix, _ = projectid.LoadIndex(indexPath)
	want, _ := filepath.EvalSymlinks(newHome)
	if e, _ := ix.Lookup(movedEntry.ID); func() string { p, _ := filepath.EvalSymlinks(e.Path); return p }() != want {
		t.Fatalf("index not relocated: %q, want %q", e.Path, want)
	}
	if len(ix.List()) != 2 {
		t.Fatalf("the missing entry must survive without --prune, got %d entries", len(ix.List()))
	}

	out.Reset()
	if err := planeProjectRefresh(planeProjectRefreshOptions{Roots: roots, Prune: true}, &out); err != nil {
		t.Fatal(err)
	}
	ix, _ = projectid.LoadIndex(indexPath)
	if len(ix.List()) != 1 || !strings.Contains(out.String(), "1 pruned") {
		t.Fatalf("prune: %d entries\n%s", len(ix.List()), out.String())
	}
}
