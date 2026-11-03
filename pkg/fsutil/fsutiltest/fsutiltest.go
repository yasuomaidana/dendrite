// Package fsutiltest holds the file helpers tests share, so each test file does not carry its own copy.
package fsutiltest

import (
	"os"
	"path/filepath"
	"testing"
)

// WriteFile writes content to rel (slash-separated) under dir, creating the directories on the way.
func WriteFile(t testing.TB, dir, rel, content string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
