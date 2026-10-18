package dag

import (
	"os"
	"path/filepath"
	"testing"

	"Dendrite/pkg/projectid"
)

func TestAddMintsProjectID(t *testing.T) {
	dir := t.TempDir()
	proj := filepath.Join(dir, "svc")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "go.mod"), []byte("module svc\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	pm := NewPoolManager(filepath.Join(dir, "pool.json"))
	added, err := pm.Add(proj)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	onDisk, err := projectid.Read(proj)
	if err != nil {
		t.Fatalf("project.id not written: %v", err)
	}
	if added[0].ID != onDisk {
		t.Fatalf("ProjectInfo.ID = %q, want %q", added[0].ID, onDisk)
	}
}

func TestAddFailsWhenProjectIDCannotBeWritten(t *testing.T) {
	dir := t.TempDir()
	proj := filepath.Join(dir, "svc")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	// A regular file named .dendrite blocks the id directory.
	if err := os.WriteFile(filepath.Join(proj, ".dendrite"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	pm := NewPoolManager(filepath.Join(dir, "pool.json"))
	if _, err := pm.Add(proj); err == nil {
		t.Fatal("expected Add to fail when the project id cannot be minted")
	}
	if len(pm.List()) != 0 {
		t.Fatal("failed Add must not register the project")
	}
}

func TestLoadLegacyPoolWithoutIDs(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "pool.json")
	legacy := `{"version":"1.0.0","projects":[{"name":"a","path":"/x/a","ecosystems":["go"],"manifests":[],"dependencies":[]}]}`
	if err := os.WriteFile(cfg, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	pm := NewPoolManager(cfg)
	got := pm.List()
	if len(got) != 1 || got[0].Name != "a" || got[0].ID != "" {
		t.Fatalf("legacy pool not loaded as-is: %+v", got)
	}
}

func TestRebindPath(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "pool.json")
	legacy := `{"version":"1.0.0","projects":[{"name":"a","path":"/old/a/"},{"name":"b","path":"/old/b"}]}`
	if err := os.WriteFile(cfg, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	pm := NewPoolManager(cfg)
	if n := pm.RebindPath("/old/a", "/new/a"); n != 1 {
		t.Fatalf("RebindPath changed %d entries, want 1", n)
	}
	if n := pm.RebindPath("/missing", "/new/x"); n != 0 {
		t.Fatalf("RebindPath on unknown path changed %d entries", n)
	}
	if err := pm.Save(); err != nil {
		t.Fatal(err)
	}
	reloaded := NewPoolManager(cfg).List()
	if reloaded[0].Path != "/new/a" || reloaded[1].Path != "/old/b" {
		t.Fatalf("unexpected paths after rebind: %+v", reloaded)
	}
}
