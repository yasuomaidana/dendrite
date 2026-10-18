package projectid

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHomeDir(t *testing.T) {
	t.Run("honours DENDRITE_HOME via t.Setenv", func(t *testing.T) {
		custom := filepath.Join(t.TempDir(), "custom_home")
		t.Setenv("DENDRITE_HOME", custom)

		home, err := HomeDir()
		if err != nil {
			t.Fatalf("HomeDir() returned error: %v", err)
		}
		if home != custom {
			t.Errorf("HomeDir() = %q, want %q", home, custom)
		}
	})

	t.Run("falls back to user home directory when DENDRITE_HOME is unset", func(t *testing.T) {
		t.Setenv("DENDRITE_HOME", "")

		home, err := HomeDir()
		if err != nil {
			t.Fatalf("HomeDir() returned error: %v", err)
		}
		userHome, err := os.UserHomeDir()
		if err != nil {
			t.Fatalf("os.UserHomeDir() error: %v", err)
		}
		expected := filepath.Join(userHome, ".dendrite")
		if home != expected {
			t.Errorf("HomeDir() = %q, want %q", home, expected)
		}
	})
}

func TestDefaultIndexPath(t *testing.T) {
	custom := filepath.Join(t.TempDir(), "custom_home")
	t.Setenv("DENDRITE_HOME", custom)

	indexPath, err := DefaultIndexPath()
	if err != nil {
		t.Fatalf("DefaultIndexPath() error: %v", err)
	}
	expected := filepath.Join(custom, "projects.json")
	if indexPath != expected {
		t.Errorf("DefaultIndexPath() = %q, want %q", indexPath, expected)
	}
}

func TestLoadIndex(t *testing.T) {
	t.Run("missing file yields empty index with Version 1", func(t *testing.T) {
		missingPath := filepath.Join(t.TempDir(), "does_not_exist", "projects.json")

		ix, err := LoadIndex(missingPath)
		if err != nil {
			t.Fatalf("LoadIndex(missing) error: %v", err)
		}
		if ix == nil {
			t.Fatal("LoadIndex(missing) returned nil")
		}
		if ix.Version != 1 {
			t.Errorf("ix.Version = %d, want 1", ix.Version)
		}
		if len(ix.Projects) != 0 {
			t.Errorf("len(ix.Projects) = %d, want 0", len(ix.Projects))
		}
		if ix.path != missingPath {
			t.Errorf("ix.path = %q, want %q", ix.path, missingPath)
		}
	})

	t.Run("malformed json returns error", func(t *testing.T) {
		dir := t.TempDir()
		badFile := filepath.Join(dir, "projects.json")
		if err := os.WriteFile(badFile, []byte("{invalid-json"), 0644); err != nil {
			t.Fatal(err)
		}

		_, err := LoadIndex(badFile)
		if err == nil {
			t.Fatal("expected error on malformed json, got nil")
		}
	})
}

func TestIndexSaveAndLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "sub", "projects.json")

	ix, err := LoadIndex(indexPath)
	if err != nil {
		t.Fatalf("LoadIndex error: %v", err)
	}

	testTime := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	entry := Entry{
		ID:       "dpid_11112222333344445555666677778888",
		Path:     filepath.Join(dir, "p1"),
		Remote:   "git@github.com:User/Repo.git",
		Name:     "p1",
		LastSeen: testTime,
	}
	ix.Upsert(entry)

	if err := ix.Save(); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	loaded, err := LoadIndex(indexPath)
	if err != nil {
		t.Fatalf("LoadIndex after Save() failed: %v", err)
	}

	if loaded.Version != 1 {
		t.Errorf("loaded.Version = %d, want 1", loaded.Version)
	}

	got, ok := loaded.Lookup(entry.ID)
	if !ok {
		t.Fatalf("Lookup(%q) not found after round trip", entry.ID)
	}
	if got.ID != entry.ID {
		t.Errorf("got.ID = %q, want %q", got.ID, entry.ID)
	}
	if got.Path != entry.Path {
		t.Errorf("got.Path = %q, want %q", got.Path, entry.Path)
	}
	if got.Remote != "github.com/user/repo" {
		t.Errorf("got.Remote = %q, want %q", got.Remote, "github.com/user/repo")
	}
	if got.Name != entry.Name {
		t.Errorf("got.Name = %q, want %q", got.Name, entry.Name)
	}
	if !got.LastSeen.Equal(testTime) {
		t.Errorf("got.LastSeen = %v, want %v", got.LastSeen, testTime)
	}
}

func TestIndexUpsert(t *testing.T) {
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "projects.json")
	ix, err := LoadIndex(indexPath)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("upsert sets LastSeen when zero", func(t *testing.T) {
		before := time.Now().UTC().Add(-time.Second)
		e := Entry{
			ID:     "dpid_00000000000000000000000000000001",
			Path:   "/path/1",
			Remote: "git@github.com:A/B.git",
			Name:   "p1",
		}
		ix.Upsert(e)
		after := time.Now().UTC().Add(time.Second)

		stored, ok := ix.Lookup(e.ID)
		if !ok {
			t.Fatal("entry not found")
		}
		if stored.LastSeen.Before(before) || stored.LastSeen.After(after) {
			t.Errorf("LastSeen %v not between %v and %v", stored.LastSeen, before, after)
		}
		if stored.Remote != "github.com/a/b" {
			t.Errorf("stored.Remote = %q, want %q", stored.Remote, "github.com/a/b")
		}
	})

	t.Run("upsert preserves non-zero LastSeen", func(t *testing.T) {
		fixedTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		e := Entry{
			ID:       "dpid_00000000000000000000000000000002",
			Path:     "/path/2",
			Remote:   "https://github.com/A/B",
			Name:     "p2",
			LastSeen: fixedTime,
		}
		ix.Upsert(e)

		stored, ok := ix.Lookup(e.ID)
		if !ok {
			t.Fatal("entry not found")
		}
		if !stored.LastSeen.Equal(fixedTime) {
			t.Errorf("stored.LastSeen = %v, want %v", stored.LastSeen, fixedTime)
		}
	})
}

func TestLookupByRemote(t *testing.T) {
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "projects.json")
	ix, err := LoadIndex(indexPath)
	if err != nil {
		t.Fatal(err)
	}

	e1 := Entry{
		ID:     "dpid_00000000000000000000000000000002",
		Path:   "/path/repo2",
		Remote: "ssh://git@github.com:22/org/repo.git",
		Name:   "repo2",
	}
	e2 := Entry{
		ID:     "dpid_00000000000000000000000000000001",
		Path:   "/path/repo1",
		Remote: "git@github.com:org/repo.git",
		Name:   "repo1",
	}
	e3 := Entry{
		ID:     "dpid_00000000000000000000000000000003",
		Path:   "/path/repo3",
		Remote: "https://gitlab.com/other/project",
		Name:   "repo3",
	}

	ix.Upsert(e1)
	ix.Upsert(e2)
	ix.Upsert(e3)

	t.Run("matches ssh vs https forms", func(t *testing.T) {
		results := ix.LookupByRemote("https://github.com/org/repo")
		if len(results) != 2 {
			t.Fatalf("expected 2 matches, got %d", len(results))
		}
		// Must be sorted by ID: e2 (ending in 001) then e1 (ending in 002)
		if results[0].ID != e2.ID || results[1].ID != e1.ID {
			t.Errorf("unexpected order or IDs: got [%s, %s], want [%s, %s]",
				results[0].ID, results[1].ID, e2.ID, e1.ID)
		}
	})

	t.Run("matches gitlab ssh vs https", func(t *testing.T) {
		results := ix.LookupByRemote("git@gitlab.com:other/project.git")
		if len(results) != 1 {
			t.Fatalf("expected 1 match, got %d", len(results))
		}
		if results[0].ID != e3.ID {
			t.Errorf("got ID %q, want %q", results[0].ID, e3.ID)
		}
	})

	t.Run("returns empty for empty remote", func(t *testing.T) {
		results := ix.LookupByRemote("")
		if len(results) != 0 {
			t.Errorf("expected 0 matches for empty remote, got %d", len(results))
		}
	})

	t.Run("returns empty for non-matching remote", func(t *testing.T) {
		results := ix.LookupByRemote("https://unknown.com/x/y")
		if len(results) != 0 {
			t.Errorf("expected 0 matches, got %d", len(results))
		}
	})
}

func TestLookupByPath(t *testing.T) {
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "projects.json")
	ix, err := LoadIndex(indexPath)
	if err != nil {
		t.Fatal(err)
	}

	projectPath := filepath.Join(dir, "myproject")
	entry := Entry{
		ID:   "dpid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Path: projectPath,
		Name: "myproject",
	}
	ix.Upsert(entry)

	tests := []struct {
		name      string
		query     string
		wantFound bool
	}{
		{
			name:      "exact path",
			query:     projectPath,
			wantFound: true,
		},
		{
			name:      "path with trailing slash",
			query:     projectPath + string(filepath.Separator),
			wantFound: true,
		},
		{
			name:      "path with dot element",
			query:     filepath.Join(dir, ".", "myproject"),
			wantFound: true,
		},
		{
			name:      "non-existent path",
			query:     filepath.Join(dir, "unknown"),
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ix.LookupByPath(tt.query)
			if ok != tt.wantFound {
				t.Fatalf("LookupByPath(%q) found = %v, want %v", tt.query, ok, tt.wantFound)
			}
			if ok && got.ID != entry.ID {
				t.Errorf("LookupByPath(%q) got ID %q, want %q", tt.query, got.ID, entry.ID)
			}
		})
	}
}

func TestRemove(t *testing.T) {
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "projects.json")
	ix, err := LoadIndex(indexPath)
	if err != nil {
		t.Fatal(err)
	}

	id := "dpid_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	ix.Upsert(Entry{ID: id, Path: "/tmp/foo"})

	if !ix.Remove(id) {
		t.Fatalf("Remove(%q) returned false, want true", id)
	}
	if _, ok := ix.Lookup(id); ok {
		t.Errorf("Lookup after Remove returned true, want false")
	}
	if ix.Remove(id) {
		t.Errorf("second Remove(%q) returned true, want false", id)
	}
}

func TestList(t *testing.T) {
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "projects.json")
	ix, err := LoadIndex(indexPath)
	if err != nil {
		t.Fatal(err)
	}

	if len(ix.List()) != 0 {
		t.Errorf("empty index List() should be empty, got %d", len(ix.List()))
	}

	entries := []Entry{
		{ID: "dpid_cccccccccccccccccccccccccccccccc", Path: "/c"},
		{ID: "dpid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Path: "/a"},
		{ID: "dpid_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Path: "/b"},
	}
	for _, e := range entries {
		ix.Upsert(e)
	}

	list := ix.List()
	if len(list) != 3 {
		t.Fatalf("List() returned %d items, want 3", len(list))
	}
	if list[0].ID != "dpid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" ||
		list[1].ID != "dpid_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" ||
		list[2].ID != "dpid_cccccccccccccccccccccccccccccccc" {
		t.Errorf("List() not sorted by ID: [%s, %s, %s]", list[0].ID, list[1].ID, list[2].ID)
	}
}

func TestRegister(t *testing.T) {
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "projects.json")
	ix, err := LoadIndex(indexPath)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("register real repo with origin then Save/LoadIndex round-trip", func(t *testing.T) {
		repoOrigin := "git@github.com:Dendrite/synapse-mesh.git"
		repoDir := createGitRepo(t, repoOrigin)

		entry, err := ix.Register(repoDir)
		if err != nil {
			t.Fatalf("Register failed: %v", err)
		}

		if !Valid(entry.ID) {
			t.Errorf("entry.ID invalid: %q", entry.ID)
		}
		expectedRemote := "github.com/dendrite/synapse-mesh"
		if entry.Remote != expectedRemote {
			t.Errorf("entry.Remote = %q, want %q", entry.Remote, expectedRemote)
		}
		expectedName := filepath.Base(repoDir)
		if entry.Name != expectedName {
			t.Errorf("entry.Name = %q, want %q", entry.Name, expectedName)
		}

		// Ensure persistent ID file was created
		idFromFile, err := Read(repoDir)
		if err != nil {
			t.Fatalf("Read(repoDir) failed: %v", err)
		}
		if idFromFile != entry.ID {
			t.Errorf("Read(repoDir) = %q, want %q", idFromFile, entry.ID)
		}

		// Second register returns identical ID
		entry2, err := ix.Register(repoDir)
		if err != nil {
			t.Fatalf("second Register failed: %v", err)
		}
		if entry2.ID != entry.ID {
			t.Errorf("second Register ID = %q, want %q", entry2.ID, entry.ID)
		}

		// Save and verify round-trip
		if err := ix.Save(); err != nil {
			t.Fatalf("Save failed: %v", err)
		}

		loaded, err := LoadIndex(indexPath)
		if err != nil {
			t.Fatalf("LoadIndex failed: %v", err)
		}

		// Lookup by remote matches ssh and https forms
		matches := loaded.LookupByRemote("https://github.com/Dendrite/synapse-mesh")
		if len(matches) != 1 {
			t.Fatalf("expected 1 match via https lookup, got %d", len(matches))
		}
		if matches[0].ID != entry.ID {
			t.Errorf("matches[0].ID = %q, want %q", matches[0].ID, entry.ID)
		}

		// Lookup by path
		foundByPath, ok := loaded.LookupByPath(repoDir)
		if !ok {
			t.Fatalf("LookupByPath(%q) not found", repoDir)
		}
		if foundByPath.ID != entry.ID {
			t.Errorf("foundByPath.ID = %q, want %q", foundByPath.ID, entry.ID)
		}
	})

	t.Run("register non-repo directory", func(t *testing.T) {
		plainDir := t.TempDir()

		entry, err := ix.Register(plainDir)
		if err != nil {
			t.Fatalf("Register non-repo failed: %v", err)
		}

		if !Valid(entry.ID) {
			t.Errorf("entry.ID invalid: %q", entry.ID)
		}
		if entry.Remote != "" {
			t.Errorf("entry.Remote = %q, want empty", entry.Remote)
		}
		if entry.Name != filepath.Base(plainDir) {
			t.Errorf("entry.Name = %q, want %q", entry.Name, filepath.Base(plainDir))
		}
	})
}
