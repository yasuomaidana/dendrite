package fsutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadIfPresent(t *testing.T) {
	dir := t.TempDir()
	if data, present, err := ReadIfPresent(filepath.Join(dir, "none")); data != nil || present || err != nil {
		t.Errorf("a missing file is absent, not an error: %q %v %v", data, present, err)
	}
	path := filepath.Join(dir, "f")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if data, present, err := ReadIfPresent(path); string(data) != "x" || !present || err != nil {
		t.Errorf("%q %v %v", data, present, err)
	}
	if _, present, err := ReadIfPresent(dir); present || err == nil {
		t.Errorf("a directory is an error, not a file: %v %v", present, err)
	}
}
