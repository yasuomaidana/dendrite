package fsutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAppendLineAddsOneLinePerCallAndCreatesTheDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a", "b", "log.jsonl")
	for _, line := range []string{`{"n":1}`, `{"n":2}`} {
		if err := AppendLine(path, []byte(line)); err != nil {
			t.Fatalf("AppendLine: %v", err)
		}
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := "{\"n\":1}\n{\"n\":2}\n"; string(got) != want {
		t.Fatalf("file = %q, want %q", got, want)
	}
}

func TestAppendLineReportsAnUnwritablePath(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := AppendLine(filepath.Join(file, "child", "log"), []byte("x")); err == nil {
		t.Fatal("expected an error when the parent is a regular file")
	}
	dir := t.TempDir()
	if err := AppendLine(dir, []byte("x")); err == nil {
		t.Fatal("expected an error when the path is a directory")
	}
}
