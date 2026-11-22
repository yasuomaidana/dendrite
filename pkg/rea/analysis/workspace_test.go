package analysis

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/coreclient"
	"Dendrite/pkg/rea"
)

func TestDocNameIsSafeAndUnique(t *testing.T) {
	used := map[string]int{}
	got := []string{docName("KERNEL32.dll", used), docName("kernel32.DLL", used), docName("??.", used), docName("a b/c.dll", used),
		docName("/usr/lib/libSystem.B.dylib", used)}
	want := []string{"kernel32.dll.md", "kernel32.dll_2.md", "library.md", "c.dll.md", "libsystem.b.dylib.md"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("name %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestLibraryLabelFollowsTheFormat(t *testing.T) {
	for format, want := range map[string]string{
		"pe": "Imported DLLs", "elf": "Shared libraries (DT_NEEDED)", "macho": "Dylibs", "fat-macho": "Dylibs",
		"asar": "Dependencies", "zip": "Dependencies", "js-bundle": "Dependencies", "source-tree": "Dependencies", "unknown": "Libraries",
	} {
		if got := libraryLabel(format); got != want {
			t.Errorf("%s: %q, want %q", format, got, want)
		}
	}
}

func TestOverviewDescribesWhatTheTargetIs(t *testing.T) {
	st := State{ID: "a-1", Binary: "/x/sample"}
	elf := &Image{Kind: "executable", Format: "elf", Arch: "x86_64", Bits: 64, Endian: "little", Entry: "0x401040", Entropy: 5.123,
		Notes: map[string]string{"rpath": "$ORIGIN", "interpreter": "/lib64/ld.so"}, Truncated: true,
		Libraries: []coreclient.ImageLibrary{{Name: "libc.so.6"}}}
	text := overview(st, elf)
	for _, want := range []string{"# Analysis a-1: sample", "- Kind: executable", "- Format: elf", "- Architecture: x86_64", "- Bits: 64", "- Entropy: 5.12",
		"- interpreter: /lib64/ld.so", "- rpath: $ORIGIN", "shared libraries (dt_needed): 1", "cut by the inspector"} {
		if !strings.Contains(text, want) {
			t.Errorf("overview lacks %q:\n%s", want, text)
		}
	}
	if strings.Index(text, "interpreter") > strings.Index(text, "rpath") {
		t.Errorf("notes are listed in a stable order:\n%s", text)
	}
	bare := overview(st, &Image{Kind: "data", Format: "unknown"})
	for _, absent := range []string{"Architecture", "Bits", "Entropy", "cut by"} {
		if strings.Contains(bare, absent) {
			t.Errorf("an unknown file has no %s:\n%s", absent, bare)
		}
	}
}

func TestWorkspaceDocumentsFollowTheContentOfTheTarget(t *testing.T) {
	full := &Image{Sections: []coreclient.ImageSection{{Name: ".text", Address: "0x1000", Size: 4096, RawSize: 512, Flags: []string{"code", "read"}}},
		Files:   []coreclient.ImageFile{{Path: "main.js", Size: 10, Kind: "script"}, {Path: "LICENSE", Size: 5}},
		Strings: []string{"https://x"}}
	if text := sectionsDoc(full); !strings.Contains(text, "| .text | 0x1000 | 4096 | 512 | code, read |") {
		t.Errorf("sections:\n%s", text)
	}
	if text := filesDoc(full); !strings.Contains(text, "- main.js (10 bytes, script)") || !strings.Contains(text, "- LICENSE (5 bytes)") {
		t.Errorf("files:\n%s", text)
	}
	if text := listDoc("Strings", "none", full.Strings); !strings.Contains(text, "- https://x") {
		t.Errorf("strings:\n%s", text)
	}
	if text := filesDoc(&Image{}); !strings.Contains(text, "holds no listed files") {
		t.Errorf("no files:\n%s", text)
	}
	if text := libraryDoc("Dylibs", coreclient.ImageLibrary{Name: "libSystem"}); !strings.Contains(text, "# Dylibs: libSystem") || !strings.Contains(text, "No symbols are recorded") {
		t.Errorf("library:\n%s", text)
	}
}

func TestWriteWorkspaceWritesTheDocumentsOfAnyFormat(t *testing.T) {
	dir := t.TempDir()
	img := &Image{Kind: "package", Format: "asar", Files: []coreclient.ImageFile{{Path: "main.js", Size: 1}},
		Libraries: []coreclient.ImageLibrary{{Name: "lodash", Symbols: []string{"map"}}}}
	if err := writeWorkspace(dir, State{ID: "w", Binary: "/x/app.asar"}, img); err != nil {
		t.Fatal(err)
	}
	for _, doc := range []string{"overview.md", "sections.md", "exports.md", "files.md", "strings.md", "libraries/lodash.md"} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(doc))); err != nil {
			t.Errorf("document %s: %v", doc, err)
		}
	}
}

func TestWriteWorkspaceReportsAnUnwritableDirectory(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "in-the-way")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	img := &Image{Libraries: []coreclient.ImageLibrary{{Name: "A.dll"}}}
	if err := writeWorkspace(filepath.Join(file, "workspace"), State{ID: "x"}, img); err == nil {
		t.Error("a workspace below a regular file cannot be written")
	}
}

func TestBundleCarriesDependenciesAndExports(t *testing.T) {
	b := Bundle(State{ID: "b", Binary: "/x/a.out"}, &Image{Kind: "executable", Format: "elf", Exports: []string{"main"},
		Libraries: []coreclient.ImageLibrary{{Name: "libc.so.6"}}})
	if b.Format != rea.FormatELF || len(b.CrossReferences) != 1 || b.CrossReferences[0] != "libc.so.6" || b.StringConstants[0] != "main" || b.TokenEstimate == 0 {
		t.Errorf("bundle %+v", b)
	}
}

func TestDocumentsListEveryFileWithItsEdges(t *testing.T) {
	st := State{ID: "d", Binary: "/x/app.exe"}
	img := &Image{Format: "pe", Libraries: []coreclient.ImageLibrary{{Name: "A.dll", Symbols: []string{"F", "G"}}, {Name: "B.dll"}}, Exports: []string{"E"}}
	byPath := map[string]Document{}
	for _, d := range Documents(st, img) {
		byPath[d.Path] = d
	}
	for path, edges := range map[string]int{"libraries/a.dll.md": 2, "libraries/b.dll.md": 0, "exports.md": 1, "graph.md": 4, "overview.md": 0} {
		if d, ok := byPath[path]; !ok || d.Edges != edges || d.Why == "" || d.Text == "" {
			t.Errorf("%s: %+v (present %t), want %d edges", path, d, ok, edges)
		}
	}
	if text := byPath["graph.md"].Text; !strings.Contains(text, "- app.exe -> A.dll") || !strings.Contains(text, "- A.dll -> G") {
		t.Errorf("graph:\n%s", text)
	}
	if text := graphDoc(st, &Image{}); !strings.Contains(text, "no recorded dependencies") {
		t.Errorf("an empty graph says so:\n%s", text)
	}
}
