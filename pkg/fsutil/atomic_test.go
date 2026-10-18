package fsutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteFileAtomicErrors(t *testing.T) {
	t.Run("parent is a file", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "file")
		if err := os.WriteFile(file, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := WriteFileAtomic(filepath.Join(file, "child"), []byte("x")); err == nil {
			t.Fatal("expected MkdirAll error when parent is a regular file")
		}
	})

	t.Run("read-only directory", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.Chmod(dir, 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
		if err := WriteFileAtomic(filepath.Join(dir, "f"), []byte("x")); err == nil {
			t.Fatal("expected write error in read-only directory")
		}
	})

	t.Run("target is a non-empty directory", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "target")
		if err := os.MkdirAll(filepath.Join(target, "inner"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := WriteFileAtomic(target, []byte("x")); err == nil {
			t.Fatal("expected rename error over a non-empty directory")
		}
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if strings.Contains(e.Name(), ".tmp.") {
				t.Fatalf("temp file %s was not cleaned up", e.Name())
			}
		}
	})
}

func TestWriteFileAtomicSuccess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "f.txt")
	if err := WriteFileAtomic(path, []byte("one")); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(path, []byte("two")); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "two" {
		t.Fatalf("content = %q, %v; want \"two\"", got, err)
	}
}
