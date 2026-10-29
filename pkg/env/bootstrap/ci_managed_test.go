package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// prepareCI runs Prepare with the CI/CD step only, in a Go project at root.
func prepareCI(t *testing.T, root string) *PrepareReport {
	t.Helper()
	t.Setenv("DENDRITE_HOME", t.TempDir())
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module x\n\ngo 1.25\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := NewPreparer()
	p.SetLookPath(func(string) (string, error) { return "", os.ErrNotExist })
	report, err := p.Prepare(context.Background(), PrepareOptions{ProjectPath: root, EnforceCICD: true})
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func readCI(t *testing.T, root string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func writeCI(t *testing.T, root, content string) {
	t.Helper()
	path := filepath.Join(root, ".github", "workflows", "ci.yml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPrepareWritesAManagedCIWorkflowAndLeavesItAloneWhenCurrent(t *testing.T) {
	root := t.TempDir()
	first := prepareCI(t, root)
	if got := readCI(t, root); !strings.HasPrefix(got, ManagedHeader+"\n") {
		t.Fatalf("a generated workflow starts with the managed header: %.80q", got)
	}
	if !contains(first.FilesCreated, filepath.Join(".github", "workflows", "ci.yml")) || len(first.Notes) != 0 {
		t.Errorf("first run: created %v, notes %v", first.FilesCreated, first.Notes)
	}
	second := prepareCI(t, root)
	if contains(second.FilesUpdated, filepath.Join(".github", "workflows", "ci.yml")) || contains(second.FilesCreated, filepath.Join(".github", "workflows", "ci.yml")) || len(second.Notes) != 0 {
		t.Errorf("second run changes nothing: %+v", second)
	}
}

func TestPrepareRewritesAStaleManagedWorkflow(t *testing.T) {
	root := t.TempDir()
	writeCI(t, root, ManagedHeader+"\nname: old template\n")
	report := prepareCI(t, root)
	if got := readCI(t, root); strings.Contains(got, "old template") || !strings.HasPrefix(got, ManagedHeader) {
		t.Errorf("a managed workflow follows the template: %.120q", got)
	}
	if !contains(report.FilesUpdated, filepath.Join(".github", "workflows", "ci.yml")) {
		t.Errorf("updated = %v", report.FilesUpdated)
	}
}

func TestPrepareKeepsAHandWrittenWorkflowAndSaysSo(t *testing.T) {
	root := t.TempDir()
	const mine = "name: CI\non: [push]\njobs:\n  verify:\n    runs-on: ubuntu-latest\n    steps:\n      - run: go run ./cmd/dendrite allostasis coverage --ratchet\n"
	writeCI(t, root, mine)
	report := prepareCI(t, root)
	if got := readCI(t, root); got != mine {
		t.Fatalf("a hand-written workflow must stay byte for byte: %q", got)
	}
	if contains(report.FilesUpdated, filepath.Join(".github", "workflows", "ci.yml")) || len(report.Notes) != 1 || !strings.Contains(report.Notes[0], "hand-written") {
		t.Errorf("updated %v, notes %v", report.FilesUpdated, report.Notes)
	}
}

func TestWriteManagedFileDryRunWritesNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ci.yml")
	created, updated, kept, err := writeManagedFile(path, ManagedHeader+"\nx\n", true)
	if err != nil || !created || updated || kept {
		t.Fatalf("created %v updated %v kept %v err %v", created, updated, kept, err)
	}
	if _, statErr := os.Stat(path); statErr == nil {
		t.Error("a dry run must not create the file")
	}
}
