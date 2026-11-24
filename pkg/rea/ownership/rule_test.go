package ownership

import (
	"os"
	"path/filepath"
	"testing"

	"Dendrite/pkg/directive"
)

func TestLoadRuleReadsTheRepositoryOrTheDirectoryItself(t *testing.T) {
	if set, err := LoadRule("."); err != nil || set.Rea.RefusalOwner != directive.RefusalOwnerAGY {
		t.Errorf("inside this repository: %+v, %v", set.Rea, err)
	}

	dir := t.TempDir() // not a repository
	file := filepath.Join(dir, filepath.FromSlash(directive.Path))
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("schema = 1\n[rea]\nrefusal_owner = \"none\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if set, err := LoadRule(dir); err != nil || set.Rea.RefusalOwner != directive.RefusalOwnerNone {
		t.Errorf("its own directives: %+v, %v", set.Rea, err)
	}
	if err := os.WriteFile(file, []byte("not = [valid"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRule(dir); err == nil {
		t.Error("unreadable directives are an error")
	}
}
