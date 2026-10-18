package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"Dendrite/pkg/projectid"
)

func TestRegistry_AddProject_MintsProjectID(t *testing.T) {
	base := t.TempDir()
	reg, err := NewRegistry(filepath.Join(base, "config.json"), filepath.Join(base, "projects"))
	if err != nil {
		t.Fatal(err)
	}
	proj := filepath.Join(base, "p1")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	entry, err := reg.AddProject("p1", proj)
	if err != nil {
		t.Fatalf("AddProject: %v", err)
	}
	onDisk, err := projectid.Read(proj)
	if err != nil {
		t.Fatalf("project.id not written: %v", err)
	}
	if entry.ID != onDisk {
		t.Fatalf("ProjectEntry.ID = %q, want %q", entry.ID, onDisk)
	}
}

func TestRegistry_AddProject_ProjectIDFailure(t *testing.T) {
	base := t.TempDir()
	reg, err := NewRegistry(filepath.Join(base, "config.json"), filepath.Join(base, "projects"))
	if err != nil {
		t.Fatal(err)
	}
	proj := filepath.Join(base, "p1")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, ".dendrite"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.AddProject("p1", proj); err == nil {
		t.Fatal("expected AddProject to fail when the project id cannot be minted")
	}
	if _, err := reg.GetProject("p1"); err == nil {
		t.Fatal("failed AddProject must not register the project")
	}
}
