package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/directive"
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

func TestPrepareGivesAProjectItsDefaultDirectivesOnce(t *testing.T) {
	root := t.TempDir()
	first := prepareCI(t, root)
	for _, want := range []string{filepath.FromSlash(directive.Path), "codecov.yml", ".jscpd.json"} {
		if !contains(first.FilesCreated, want) {
			t.Errorf("first run did not create %s: %v", want, first.FilesCreated)
		}
	}
	set, err := directive.Load(root)
	if err != nil || set.Loc.MaxProductionLines != 500 {
		t.Fatalf("%+v, %v", set, err)
	}
	if again := prepareCI(t, root); contains(again.FilesCreated, filepath.FromSlash(directive.Path)) || contains(again.FilesCreated, "codecov.yml") {
		t.Errorf("a project that has directives keeps them: %v", again.FilesCreated)
	}
}

func TestEnsureDirectivesKeepsHandWrittenFilesAndHonoursDryRun(t *testing.T) {
	root := t.TempDir()
	created, notes, err := ensureDirectives(root, true)
	if err != nil || len(created) != 1 || len(notes) != 0 || directive.Exists(root) {
		t.Fatalf("a dry run only plans: %v %v %v", created, notes, err)
	}
	if err := os.WriteFile(filepath.Join(root, "codecov.yml"), []byte("coverage: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	created, notes, err = ensureDirectives(root, false)
	if err != nil || contains(created, "codecov.yml") || len(notes) != 1 || !strings.Contains(notes[0], "hand-written codecov.yml") {
		t.Fatalf("%v %v %v", created, notes, err)
	}
	if body, _ := os.ReadFile(filepath.Join(root, "codecov.yml")); string(body) != "coverage: {}\n" {
		t.Error("the hand-written codecov.yml must stay")
	}
	blocked := t.TempDir()
	if err := os.MkdirAll(filepath.Join(blocked, ".github"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(blocked, ".github", "dendrite"), []byte("a file"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ensureDirectives(blocked, false); err == nil {
		t.Error("directives that cannot be written are an error")
	}
}
