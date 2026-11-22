package coreclient

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"Dendrite/pkg/testkit"
)

// target writes a file named name with the content in a temporary directory and returns its path.
func target(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	testkit.WriteFile(t, path, content, 0o600)
	return path
}

func TestInspectRecognisesEveryFormat(t *testing.T) {
	for name, tc := range map[string]struct {
		file, content string
		kind, format  string
	}{
		"pe":         {"a.exe", "MZ\x90\x00", KindExecutable, FormatPE},
		"elf":        {"a.out", "\x7fELF\x02\x01", KindExecutable, FormatELF},
		"mach-o":     {"a", "\xcf\xfa\xed\xfe\x0c", KindExecutable, FormatMachO},
		"fat mach-o": {"a", "\xca\xfe\xba\xbe\x00", KindExecutable, FormatFatMachO},
		"zip":        {"a.apk", "PK\x03\x04rest", KindPackage, FormatZIP},
		"asar":       {"app.asar", "\x04\x00\x00\x00 pickle", KindPackage, FormatASAR},
		"js bundle":  {"main.js", "var a = 1;", KindBundle, FormatJSBundle},
		"unknown":    {"blob.bin", "just some bytes", KindData, FormatUnknown},
	} {
		img, err := testClient(t).Inspect(context.Background(), target(t, tc.file, tc.content))
		if err != nil || img.Kind != tc.kind || img.Format != tc.format {
			t.Errorf("%s: %+v, %v; want %s/%s", name, img, err, tc.kind, tc.format)
		}
	}
}

func TestInspectReadsTheDetailsOfAnImage(t *testing.T) {
	elf, err := testClient(t).Inspect(context.Background(), target(t, "a.out", "\x7fELF"))
	testkit.Check(t, err)
	testkit.Equal(t, "arch", elf.Arch, "x86_64")
	testkit.Equal(t, "bits", elf.Bits, 64)
	testkit.Equal(t, "library", elf.Libraries[0].Name, "libc.so.6")
	testkit.Equal(t, "symbol", elf.Libraries[0].Symbols[1], "strcmp")
	testkit.Equal(t, "interpreter", elf.Notes["interpreter"], "/lib64/ld-linux-x86-64.so.2")
	testkit.Equal(t, "section", elf.Sections[0].Name, ".text")

	pkg, err := testClient(t).Inspect(context.Background(), target(t, "a.apk", "PK\x03\x04"))
	testkit.Check(t, err)
	testkit.Equal(t, "files", len(pkg.Files), 2)
	testkit.Equal(t, "file kind", pkg.Files[0].Kind, "bytecode")
}

func TestInspectListsASourceDirectory(t *testing.T) {
	dir := t.TempDir()
	testkit.WriteFile(t, filepath.Join(dir, "b.go"), "package b", 0o600)
	testkit.WriteFile(t, filepath.Join(dir, "a.go"), "package a", 0o600)
	testkit.WriteFile(t, filepath.Join(dir, "sub", "c.go"), "package c", 0o600)
	img, err := testClient(t).Inspect(context.Background(), dir)
	testkit.Check(t, err)
	testkit.Equal(t, "kind", img.Kind, KindSource)
	testkit.Equal(t, "format", img.Format, FormatSourceTree)
	testkit.Equal(t, "files", len(img.Files), 2)
	testkit.Equal(t, "sorted", img.Files[0].Path, "a.go")
}

func TestInspectFailures(t *testing.T) {
	ctx := context.Background()
	if _, err := testClient(t).Inspect(ctx, " "); err == nil {
		t.Error("an empty path is an error")
	}
	if _, err := testClient(t).Inspect(ctx, filepath.Join(t.TempDir(), "missing")); !errors.Is(err, ErrNotRecognised) {
		t.Errorf("a path that does not exist: %v", err)
	}

	t.Setenv("TESTSIDECAR_MODE", "noinspect")
	if _, err := testClient(t).Inspect(ctx, target(t, "a.exe", "MZ")); !errors.Is(err, ErrInspectUnsupported) {
		t.Errorf("an old sidecar: %v", err)
	}
	t.Setenv("TESTSIDECAR_MODE", "rpcfail")
	if _, err := testClient(t).Inspect(ctx, target(t, "a.exe", "MZ")); err == nil || errors.Is(err, ErrInspectUnsupported) || errors.Is(err, ErrNotRecognised) {
		t.Errorf("another sidecar failure passes through: %v", err)
	}
	t.Setenv("TESTSIDECAR_MODE", "exit")
	if _, err := testClient(t).Inspect(ctx, target(t, "a.exe", "MZ")); err == nil || errors.Is(err, ErrInspectUnsupported) || errors.Is(err, ErrNotRecognised) {
		t.Errorf("a sidecar that exits: %v", err)
	}
	var nilClient *Client
	if _, err := nilClient.Inspect(ctx, target(t, "a.exe", "MZ")); !errors.Is(err, ErrBinaryMissing) {
		t.Errorf("a nil client must report ErrBinaryMissing, got %v", err)
	}
}

func TestFromPEMapsAPEInfoToAnImage(t *testing.T) {
	pe := &PEInfo{Machine: "x86_64", Subsystem: "gui", PE32Plus: true, EntryPoint: "0x1000", ImageBase: "0x400000", Exports: []string{"E"}, Truncated: true,
		Sections: []PESection{{Name: ".text", VirtualAddress: "0x1000", VirtualSize: 10, RawSize: 8, Flags: []string{"code"}}},
		Imports:  []PEImport{{DLL: "A.dll", Functions: []string{"F"}}}}
	img := FromPE(pe)
	testkit.Equal(t, "format", img.Format, FormatPE)
	testkit.Equal(t, "bits", img.Bits, 64)
	testkit.Equal(t, "base", img.Base, "0x400000")
	testkit.Equal(t, "subsystem", img.Notes["subsystem"], "gui")
	testkit.Equal(t, "section size", img.Sections[0].Size, uint64(10))
	testkit.Equal(t, "library", img.Libraries[0].Symbols[0], "F")
	testkit.Equal(t, "truncated", img.Truncated, true)

	pe.PE32Plus = false
	testkit.Equal(t, "pe32 bits", FromPE(pe).Bits, 32)
}
