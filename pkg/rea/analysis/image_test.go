package analysis

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/coreclient"
	"Dendrite/pkg/rea"
	"Dendrite/pkg/sidecar"
	"Dendrite/pkg/testkit"
)

func TestNormalizeFormat(t *testing.T) {
	for in, want := range map[string]string{"": "", "auto": "", " AUTO ": "", "pe32": "pe", "PE64": "pe", "pe": "pe", "elf": "elf", "Mach-O": "", "fat-macho": "fat-macho"} {
		got, err := normalizeFormat(in)
		if in == "Mach-O" {
			if err == nil || !strings.Contains(err.Error(), "unknown format") {
				t.Errorf("%q: %v", in, err)
			}
			continue
		}
		if err != nil || got != want {
			t.Errorf("normalizeFormat(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}

func TestReaFormatMapsImageFormatsToBundleFormats(t *testing.T) {
	for format, want := range map[string]rea.BinaryFormat{
		"pe": rea.FormatPE, "elf": rea.FormatELF, "macho": rea.FormatMachO, "fat-macho": rea.FormatMachO, "asar": rea.FormatASAR, "zip": "zip", "source-tree": "source-tree",
	} {
		if got := reaFormat(format); got != want {
			t.Errorf("%s: %q, want %q", format, got, want)
		}
	}
}

// inspectorIn is a CoreInspector over a new sidecar that plays the given stand-in mode (a running sidecar keeps the
// mode it started with).
func inspectorIn(t *testing.T, mode string) *CoreInspector {
	t.Helper()
	t.Setenv("TESTSIDECAR_MODE", mode)
	core := coreclient.New(sidecar.DefaultLocator())
	t.Cleanup(func() { _ = core.Close() })
	return &CoreInspector{Client: core}
}

func TestCoreInspectorFallsBackToPEParseForAnOlderSidecar(t *testing.T) {
	pe, elf := writeBinary(t, "MZ pe"), writeBinary(t, "\x7fELF")
	img, err := inspectorIn(t, "noinspect").Inspect(context.Background(), pe)
	if err != nil || img.Format != "pe" || len(img.Libraries) != 2 {
		t.Errorf("a PE image through pe.parse: %+v, %v", img, err)
	}
	if _, err := inspectorIn(t, "noinspect").Inspect(context.Background(), elf); !errors.Is(err, ErrFormatUnsupported) {
		t.Errorf("another format through an older sidecar: %v", err)
	}
	if _, err := inspectorIn(t, "old").Inspect(context.Background(), pe); !errors.Is(err, ErrFormatUnsupported) {
		t.Errorf("a sidecar with neither method: %v", err)
	}
	for _, mode := range []string{"exit", "pefail"} {
		if _, err := inspectorIn(t, mode).Inspect(context.Background(), pe); err == nil || errors.Is(err, ErrFormatUnsupported) {
			t.Errorf("mode %s: a failing sidecar is not an unsupported format: %v", mode, err)
		}
	}
}

func TestAnInspectionThatFailsIsRecordedAtTheNewStage(t *testing.T) {
	r := newRig(t)
	r.Inspector = inspectorIn(t, "rpcfail")
	res, err := r.Analyze(ctx(), r.binary, Options{ID: "pe-fail"})
	if err == nil || res.State.Stage != StageNew || res.State.Error == "" {
		t.Errorf("%+v, %v", res, err)
	}
}

// analyzeContent analyzes a new file named file with the content.
func analyzeContent(t *testing.T, r *rig, file, content, id string) (*Result, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), file)
	testkit.WriteFile(t, path, content, 0o600)
	return r.Analyze(ctx(), path, Options{ID: id})
}

func TestEveryFormatIsAnalyzedEndToEnd(t *testing.T) {
	type formatCase struct{ name, file, content, format, doc string }
	for _, tc := range []formatCase{
		{name: "pe", file: "a.exe", content: "MZ", format: "pe", doc: "libraries/kernel32.dll.md"},
		{name: "elf", file: "a.out", content: "\x7fELF", format: "elf", doc: "libraries/libc.so.6.md"},
		{name: "mach-o", file: "a", content: "\xcf\xfa\xed\xfe", format: "macho", doc: "libraries/libsystem.b.dylib.md"},
		{name: "fat mach-o", file: "b", content: "\xca\xfe\xba\xbe", format: "fat-macho", doc: "libraries/libsystem.b.dylib.md"},
		{name: "zip", file: "a.apk", content: "PK\x03\x04", format: "zip", doc: "files.md"},
		{name: "asar", file: "app.asar", content: "x", format: "asar", doc: "files.md"},
		{name: "js bundle", file: "main.js", content: "var a;", format: "js-bundle", doc: "strings.md"},
		{name: "unknown", file: "blob", content: "bytes", format: "unknown", doc: "strings.md"},
	} {
		r := newRig(t)
		res, err := analyzeContent(t, r, tc.file, tc.content, "fmt-"+strings.ReplaceAll(tc.name, " ", "-"))
		if err != nil || res.State.Format != tc.format || res.State.Stage != StageDone || res.Bundle == nil {
			t.Errorf("%s: %+v, %v", tc.name, res, err)
			continue
		}
		if _, err := os.Stat(filepath.Join(r.Store.WorkspaceDir(res.State.ID), filepath.FromSlash(tc.doc))); err != nil {
			t.Errorf("%s: document %s: %v", tc.name, tc.doc, err)
		}
	}
}

func TestADirectoryOfSourceIsAnalyzedAndSearched(t *testing.T) {
	r := newRig(t)
	src := t.TempDir()
	testkit.WriteFile(t, filepath.Join(src, "check.go"), "package main", 0o600)
	testkit.WriteFile(t, filepath.Join(src, "util.go"), "package main", 0o600)
	res, err := r.Analyze(ctx(), src, Options{ID: "src-tree", Format: "source-tree"})
	if err != nil || res.State.Kind != "source" || res.State.Format != "source-tree" || res.State.Stage != StageDone {
		t.Fatalf("%+v, %v", res, err)
	}
	if hits, err := r.Search(ctx(), "src-tree", "check.go", 3); err != nil || len(hits) == 0 {
		t.Errorf("the file list is searchable: %+v, %v", hits, err)
	}
}

// legacyPE is the structure an analysis made before the format-neutral model kept in pe.json.
const legacyPE = `{"machine":"x86_64","subsystem":"console","pe32_plus":true,"entry_point":"0x1","image_base":"0x2","sections":[],"imports":[{"dll":"OLD.dll","functions":["F"]}],"exports":[],"truncated":false}`

func TestAnAnalysisSavedBeforeTheFormatNeutralModelStillLoads(t *testing.T) {
	r := newRig(t)
	finished(t, r, "pe-old")
	if err := os.Remove(r.imagePath("pe-old")); err != nil {
		t.Fatal(err)
	}
	testkit.WriteFile(t, r.legacyPEPath("pe-old"), legacyPE, 0o600)
	st, err := r.Store.Load("pe-old")
	if err != nil {
		t.Fatal(err)
	}
	st.Format = "pe64" // the format name those analyses recorded
	if err := r.Store.Save(&st); err != nil {
		t.Fatal(err)
	}
	res, err := r.Resume(ctx(), "pe-old", Options{})
	if err != nil || res.Bundle == nil || res.Bundle.CrossReferences[0] != "OLD.dll" || res.State.Format != "pe64" {
		t.Errorf("%+v, %v", res, err)
	}
}

func TestADamagedInspectedStructureIsReported(t *testing.T) {
	for name, damage := range map[string]func(t *testing.T, r *rig, id string){
		"corrupt legacy structure": func(t *testing.T, r *rig, id string) {
			_ = os.Remove(r.imagePath(id))
			testkit.WriteFile(t, r.legacyPEPath(id), "{", 0o600)
		},
		"corrupt image": func(t *testing.T, r *rig, id string) { testkit.WriteFile(t, r.imagePath(id), "{", 0o600) },
		"image that cannot be read": func(t *testing.T, r *rig, id string) {
			_ = os.Remove(r.imagePath(id))
			if err := os.MkdirAll(r.imagePath(id), 0o755); err != nil {
				t.Fatal(err)
			}
		},
	} {
		r := newRig(t)
		finished(t, r, "pe-dmg")
		damage(t, r, "pe-dmg")
		if _, err := r.Resume(ctx(), "pe-dmg", Options{}); err == nil || !strings.Contains(err.Error(), "unreadable") {
			t.Errorf("%s: %v", name, err)
		}
	}
}
