package fsutiltest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileCreatesTheDirectoriesOnTheWay(t *testing.T) {
	dir := t.TempDir()
	WriteFile(t, dir, "a/b/c.txt", "hello")
	got, err := os.ReadFile(filepath.Join(dir, "a", "b", "c.txt"))
	if err != nil || string(got) != "hello" {
		t.Errorf("%q, %v", got, err)
	}
}
