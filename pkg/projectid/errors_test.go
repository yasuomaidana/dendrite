package projectid

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHomeDirWithoutHome(t *testing.T) {
	t.Setenv("DENDRITE_HOME", "")
	t.Setenv("HOME", "")
	if _, err := HomeDir(); err == nil {
		t.Fatal("expected error when neither DENDRITE_HOME nor HOME is set")
	}
	if _, err := DefaultIndexPath(); err == nil {
		t.Fatal("expected DefaultIndexPath to propagate the HomeDir error")
	}
}

func TestReadAndEnsureErrors(t *testing.T) {
	dir := t.TempDir()
	// project.id is a directory: ReadFile fails with something other than ErrNotExist.
	if err := os.MkdirAll(Path(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(dir); err == nil || errors.Is(err, ErrNoProjectID) {
		t.Fatalf("Read on a directory: got %v, want non-ErrNoProjectID error", err)
	}
	if _, err := Ensure(dir); err == nil || errors.Is(err, ErrNoProjectID) {
		t.Fatalf("Ensure must surface the read error, got %v", err)
	}

	// .dendrite is a file: Ensure cannot create the id file.
	blocked := t.TempDir()
	if err := os.WriteFile(filepath.Join(blocked, ".dendrite"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Ensure(blocked); err == nil {
		t.Fatal("expected Ensure to fail when .dendrite is a regular file")
	}
	if _, err := LoadIndexOrFail(t).Register(blocked); err == nil {
		t.Fatal("expected Register to propagate the Ensure error")
	}

	// .dendrite exists but is read-only: the id is absent and cannot be written.
	ro := t.TempDir()
	roState := filepath.Join(ro, ".dendrite")
	if err := os.Mkdir(roState, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(roState, 0o755) })
	if _, err := Ensure(ro); err == nil || !strings.Contains(err.Error(), "projectid: write") {
		t.Fatalf("Ensure in read-only .dendrite: got %v, want write error", err)
	}
}

// LoadIndexOrFail loads an empty index from a fresh temp path.
func LoadIndexOrFail(t *testing.T) *Index {
	t.Helper()
	ix, err := LoadIndex(filepath.Join(t.TempDir(), "projects.json"))
	if err != nil {
		t.Fatal(err)
	}
	return ix
}

func TestLoadIndexErrorsAndDefaults(t *testing.T) {
	dir := t.TempDir()
	if _, err := LoadIndex(dir); err == nil {
		t.Fatal("expected error loading a directory as index")
	}

	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadIndex(bad); err == nil {
		t.Fatal("expected JSON error")
	}

	empty := filepath.Join(dir, "empty.json")
	if err := os.WriteFile(empty, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	ix, err := LoadIndex(empty)
	if err != nil {
		t.Fatal(err)
	}
	if ix.Version != 1 || ix.Projects == nil {
		t.Fatalf("defaults not applied: version=%d projects=%v", ix.Version, ix.Projects)
	}
}

func TestUpsertZeroValueIndexAndExplicitLastSeen(t *testing.T) {
	var ix Index
	seen := time.Date(2026, 10, 18, 9, 0, 0, 0, time.FixedZone("CST", -6*3600))
	ix.Upsert(Entry{ID: "dpid_00000000000000000000000000000001", LastSeen: seen})
	got, ok := ix.Lookup("dpid_00000000000000000000000000000001")
	if !ok {
		t.Fatal("entry missing after Upsert on zero-value Index")
	}
	if !got.LastSeen.Equal(seen) || got.LastSeen.Location() != time.UTC {
		t.Fatalf("LastSeen = %v, want %v in UTC", got.LastSeen, seen)
	}
}

func TestNormalizeRemoteUnparseableURL(t *testing.T) {
	raw := "https://[::1/a/b"
	if got := NormalizeRemote(raw); got != raw {
		t.Fatalf("NormalizeRemote(%q) = %q, want input unchanged", raw, got)
	}
}
