package analysis

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/coreclient"
)

// TestExportRustWritesAModuleThatCompiles is AC-1 of DEN-1065. Where rustc is installed the module must compile as a
// library; elsewhere the check of its content stands alone.
func TestExportRustWritesAModuleThatCompiles(t *testing.T) {
	r := newRig(t)
	if _, err := r.Analyze(ctx(), r.binary, Options{ID: "pe-rs"}); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	path, err := r.ExportRust("pe-rs", out)
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"pub struct Section", "pub struct Import", "pub const IMAGE: Image", `dll: "KERNEL32.dll"`} {
		if !strings.Contains(string(body), want) {
			t.Errorf("module lacks %q:\n%s", want, body)
		}
	}
	if strings.Contains(string(body), "unsafe {") || strings.Contains(string(body), "unsafe fn") {
		t.Error("the module must hold no unsafe code")
	}
	if rustc, err := exec.LookPath("rustc"); err == nil {
		cmd := exec.Command(rustc, "--crate-type", "lib", "--edition", "2021", "--out-dir", t.TempDir(), "-D", "warnings", path)
		if combined, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("rustc: %v\n%s", err, combined)
		}
	}
}

// TestExportRustStaysUnderOutput is AC-2: the file lands in the given directory, created when it does not exist.
func TestExportRustStaysUnderOutput(t *testing.T) {
	r := newRig(t)
	if _, err := r.Analyze(ctx(), r.binary, Options{ID: "pe-out"}); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "nested", "gen")
	path, err := r.ExportRust("pe-out", out)
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(out, "pe-out.rs") {
		t.Errorf("exported to %s, want it under %s", path, out)
	}
	if _, err := r.ExportRust("pe-out", " "); !errors.Is(err, ErrNeedsOutput) {
		t.Errorf("no output directory: %v", err)
	}
}

// TestExportRustNeedsACompletedAnalysis is AC-3.
func TestExportRustNeedsACompletedAnalysis(t *testing.T) {
	r := pausedAtParsed(t, "pe-half")
	if _, err := r.ExportRust("pe-half", t.TempDir()); !errors.Is(err, ErrNotDone) {
		t.Errorf("paused analysis: %v", err)
	}
	if _, err := r.ExportRust("pe-missing", t.TempDir()); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown analysis: %v", err)
	}
}

func TestRustLiteralsEscapeWhatRustWouldRead(t *testing.T) {
	if got := rustString("a\"b\\c\n\u00e9"); got != `"a\"b\\c\u{a}\u{e9}"` {
		t.Errorf("rustString = %s", got)
	}
	if _, err := rustAddress("zz"); err == nil {
		t.Error("a bad address must be refused")
	}
	if _, err := rustModule(State{ID: "x"}, &coreclient.PEInfo{EntryPoint: "0x1", ImageBase: "nope"}); err == nil {
		t.Error("a bad image base must be refused")
	}
	bad := &coreclient.PEInfo{EntryPoint: "0x1", ImageBase: "0x2", Sections: []coreclient.PESection{{Name: ".x", VirtualAddress: "?"}}}
	if _, err := rustModule(State{ID: "x"}, bad); err == nil {
		t.Error("a bad section address must be refused")
	}
	if _, err := rustModule(State{ID: "x"}, &coreclient.PEInfo{EntryPoint: "?"}); err == nil {
		t.Error("a bad entry point must be refused")
	}
}

func TestExportRustReportsADamagedAnalysis(t *testing.T) {
	r := newRig(t)
	if _, err := r.Analyze(ctx(), r.binary, Options{ID: "pe-bad"}); err != nil {
		t.Fatal(err)
	}
	info := filepath.Join(r.Store.Dir("pe-bad"), "image.json")
	if err := os.WriteFile(info, []byte(`{"kind":"executable","format":"pe","entry":"nope","base":"0x1"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := r.ExportRust("pe-bad", t.TempDir()); err == nil || !strings.Contains(err.Error(), "pe-bad") {
		t.Errorf("an unreadable address must name the analysis: %v", err)
	}
	if err := os.Remove(info); err != nil {
		t.Fatal(err)
	}
	if _, err := r.ExportRust("pe-bad", t.TempDir()); err == nil {
		t.Error("a missing structure must be refused")
	}
}
