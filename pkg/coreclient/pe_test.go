package coreclient

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"Dendrite/pkg/testkit"
)

// peFile writes a file with the given content and returns its path.
func peFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sample.exe")
	testkit.Check(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestPEParse(t *testing.T) {
	info, err := testClient(t).PEParse(context.Background(), peFile(t, "MZ\x90\x00 header"))
	testkit.Check(t, err)
	testkit.Equal(t, "machine", info.Machine, "x86_64")
	testkit.Equal(t, "pe32+", info.PE32Plus, true)
	testkit.Equal(t, "entry point", info.EntryPoint, "0x140001000")
	testkit.Equal(t, "sections", len(info.Sections), 2)
	testkit.Equal(t, "first section", info.Sections[0].Name, ".text")
	testkit.Equal(t, "imports", info.Imports[0].DLL, "KERNEL32.dll")
	testkit.Equal(t, "exports", info.Exports[0], "ComputeChecksum")
}

func TestPEParse_Failures(t *testing.T) {
	ctx := context.Background()
	if _, err := testClient(t).PEParse(ctx, "  "); err == nil {
		t.Error("an empty path is an error")
	}
	if _, err := testClient(t).PEParse(ctx, peFile(t, "ELF")); !errors.Is(err, ErrNotPE) {
		t.Errorf("a file that is not a PE image: %v", err)
	}
	if _, err := testClient(t).PEParse(ctx, filepath.Join(t.TempDir(), "missing.exe")); !errors.Is(err, ErrNotPE) {
		t.Errorf("an unreadable path is reported as a rejected request: %v", err)
	}

	t.Setenv("TESTSIDECAR_MODE", "nope")
	if _, err := testClient(t).PEParse(ctx, peFile(t, "MZ")); !errors.Is(err, ErrPEUnsupported) {
		t.Errorf("an old sidecar: %v", err)
	}
	t.Setenv("TESTSIDECAR_MODE", "exit")
	if _, err := testClient(t).PEParse(ctx, peFile(t, "MZ")); err == nil || errors.Is(err, ErrPEUnsupported) || errors.Is(err, ErrNotPE) {
		t.Errorf("a sidecar that exits: %v", err)
	}

	var nilClient *Client
	if _, err := nilClient.PEParse(ctx, peFile(t, "MZ")); !errors.Is(err, ErrBinaryMissing) {
		t.Errorf("a nil client must report ErrBinaryMissing, got %v", err)
	}
}
