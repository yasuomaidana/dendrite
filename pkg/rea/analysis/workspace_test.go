package analysis

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/coreclient"
)

func TestDocNameIsSafeAndUnique(t *testing.T) {
	used := map[string]int{}
	got := []string{docName("KERNEL32.dll", used), docName("kernel32.DLL", used), docName("??.", used), docName("a b/c.dll", used)}
	want := []string{"kernel32.dll.md", "kernel32.dll_2.md", "dll.md", "a_b_c.dll.md"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("name %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestWorkspaceDocumentsDescribeTheImage(t *testing.T) {
	st := State{ID: "pe-w", Binary: "/x/sample.exe", Format: "pe64"}
	empty := &coreclient.PEInfo{Machine: "x86_64", Truncated: true}
	if text := overview(st, empty); !strings.Contains(text, "cut by the parser") || !strings.Contains(text, "sample.exe") {
		t.Errorf("overview:\n%s", text)
	}
	if text := exportsDoc(empty); !strings.Contains(text, "exports nothing") {
		t.Errorf("exports:\n%s", text)
	}
	full := &coreclient.PEInfo{Sections: []coreclient.PESection{{Name: ".text", VirtualAddress: "0x1000", VirtualSize: 4096, RawSize: 512, Flags: []string{"code", "read"}}}}
	if text := sectionsDoc(full); !strings.Contains(text, "| .text | 0x1000 | 4096 | 512 | code, read |") {
		t.Errorf("sections:\n%s", text)
	}
	b := Bundle(st, empty)
	if b.TargetBinary != "/x/sample.exe" || len(b.CrossReferences) != 0 {
		t.Errorf("bundle %+v", b)
	}
}

func TestWriteWorkspaceReportsAnUnwritableDirectory(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "in-the-way")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	info := &coreclient.PEInfo{Imports: []coreclient.PEImport{{DLL: "A.dll"}}}
	if err := writeWorkspace(filepath.Join(file, "workspace"), State{ID: "x"}, info); err == nil {
		t.Error("a workspace below a regular file cannot be written")
	}
}
