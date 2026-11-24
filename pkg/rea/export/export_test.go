package export

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/coreclient"
	"Dendrite/pkg/rag"
	"Dendrite/pkg/rea/analysis"
	"Dendrite/pkg/rea/engine"
	"Dendrite/pkg/sidecar"
	"Dendrite/pkg/testkit"
)

// finished runs a real analysis of a file with the content against the stand-in sidecars.
func finished(t *testing.T, content string) (*analysis.Service, string) {
	t.Helper()
	t.Setenv("DENDRITE_HOME", t.TempDir())
	core := coreclient.New(sidecar.DefaultLocator())
	t.Cleanup(func() { _ = core.Close(); rag.CloseSidecars() })
	svc := &analysis.Service{Store: &analysis.Store{Root: filepath.Join(t.TempDir(), "rea")}, Inspector: &analysis.CoreInspector{Client: core}, Index: analysis.NewRagIndex()}
	path := filepath.Join(t.TempDir(), "sample.bin")
	testkit.WriteFile(t, path, content, 0o600)
	if _, err := svc.Analyze(context.Background(), path, analysis.Options{ID: "an-1"}); err != nil {
		t.Fatal(err)
	}
	return svc, "an-1"
}

// full replaces the saved image and engine result of analysis id with one that uses every field.
func full(t *testing.T, svc *analysis.Service, id string) {
	t.Helper()
	dir := svc.Store.Dir(id)
	image := `{"kind":"executable","format":"elf","arch":"x86_64","bits":64,"endian":"little","entry":"0x401000","base":"0x400000",
"sections":[{"name":".text\"x","address":"0x401000","size":4096,"raw_size":4096,"flags":["exec","read"]}],
"libraries":[{"name":"libc.so.6","symbols":["puts","exit"]}],"exports":["main"],"files":[{"path":"a/b.go","size":12,"kind":"source"}],
"notes":{"interpreter":"/lib/ld.so","a":"*/ \"\"\" ''' \n"},"truncated":true}`
	fns := `{"functions":[{"name":"main","address":"0x401000","size":64,"cyclomatic":3,"calls":["0x401040"]},{"name":"neg","address":"4198464","size":1,"cyclomatic":-1}]}`
	testkit.WriteFile(t, filepath.Join(dir, "image.json"), image, 0o600)
	testkit.WriteFile(t, filepath.Join(dir, "engine.json"), fns, 0o600)
}

func TestEveryLanguageWritesAModuleForAnyFormat(t *testing.T) {
	for name, content := range map[string]string{"pe": "MZ\x90\x00 pe image", "elf": "\x7fELF\x02\x01"} {
		svc, id := finished(t, content)
		for _, lang := range Languages() {
			out := t.TempDir()
			path, err := Write(svc, id, lang, out)
			if err != nil {
				t.Fatalf("%s %s: %v", name, lang, err)
			}
			if filepath.Dir(path) != out || !strings.HasPrefix(filepath.Base(path), id+".") {
				t.Errorf("%s %s: wrote %s", name, lang, path)
			}
			body, err := os.ReadFile(path)
			if err != nil || len(body) == 0 {
				t.Fatalf("%s %s: %v", name, lang, err)
			}
		}
	}
}

func TestEachLanguageRendersTheSameFacts(t *testing.T) {
	svc, id := finished(t, "\x7fELF\x02\x01")
	full(t, svc, id)
	want := map[string][]string{
		"rust":       {"pub struct Section", "pub const IMAGE: Image = Image {", `name: ".text\"x"`, "address: 0x401000,", "truncated: true,", "libraries: &[", "calls: &[\n"},
		"go":         {"type Section struct", "var Analysis = Image{", `Name: ".text\"x"`, "RawSize: 4096,", "Truncated: true,", "[]Function{"},
		"python":     {"class Section:", "@dataclass(frozen=True)", "IMAGE = Image(", "truncated=True,", "address=0x401000,", "tuple[str, ...]"},
		"typescript": {"export interface Section", "export const IMAGE: Image = {", "address: 0x401000n,", "truncated: true,", "readonly sections: readonly Section[];"},
	}
	for lang, facts := range want {
		path, err := Write(svc, id, lang, t.TempDir())
		if err != nil {
			t.Fatal(lang, err)
		}
		body, _ := os.ReadFile(path)
		testkit.Contains(t, string(body), append(facts, "libc.so.6", "interpreter", "sample.bin")...)
		if strings.Contains(string(body), "unsafe") {
			t.Errorf("%s: a data module holds no unsafe code", lang)
		}
		checkNoEscape(t, lang, string(body))
	}
}

// checkNoEscape checks that a hostile note value (comment closers and a newline) did not end a comment or string.
func checkNoEscape(t *testing.T, lang, body string) {
	t.Helper()
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "//") || strings.HasPrefix(line, `"""`) {
			if strings.Contains(strings.TrimPrefix(line, "//"), "*/") {
				t.Errorf("%s: a comment holds a closer: %s", lang, line)
			}
		}
	}
}

func TestAHostileTargetNameCannotEndTheComment(t *testing.T) {
	m := NewModel(`i"""d*/`, "dir/na\nme'''.bin", &coreclient.Image{Kind: "executable", Format: "elf"}, nil)
	tree, err := m.tree()
	if err != nil {
		t.Fatal(err)
	}
	for lang, l := range languages {
		body := l.module(m, tree)
		first := strings.SplitN(body, "\n", 3)[0]
		if strings.Contains(first, "\n") || strings.Count(first, `"""`) > 2 || strings.Contains(first, "*/") || strings.Contains(first, "'''") {
			t.Errorf("%s: %q", lang, first)
		}
	}
}

func TestRealCompilersAcceptTheModulesWhereInstalled(t *testing.T) {
	svc, id := finished(t, "\x7fELF\x02\x01")
	full(t, svc, id)
	checks := map[string][]string{
		"rust":   {"rustc", "--crate-type", "lib", "--edition", "2021", "-D", "warnings", "--out-dir", t.TempDir()},
		"go":     {"gofmt", "-e", "-l"},
		"python": {"python3", "-I", "-c", "import ast,sys; ast.parse(open(sys.argv[1]).read())"},
	}
	for lang, argv := range checks {
		bin, err := exec.LookPath(argv[0])
		if err != nil {
			continue // not installed here: the content checks stand alone
		}
		path, err := Write(svc, id, lang, t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(bin, append(argv[1:], path)...)
		if lang == "python" {
			cmd = exec.Command(bin, argv[1], argv[2], argv[3], path)
		}
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("%s: %v\n%s", lang, err, out)
		}
	}
}

func TestWriteRefusesWhatItCannotExport(t *testing.T) {
	svc, id := finished(t, "MZ\x90\x00 pe image")
	if _, err := Write(svc, id, "rust", " "); !errors.Is(err, ErrNeedsOutput) {
		t.Errorf("no output: %v", err)
	}
	if _, err := Write(svc, id, "cobol", t.TempDir()); !errors.Is(err, ErrUnknownLanguage) || !strings.Contains(err.Error(), "go, python, rust, typescript") {
		t.Errorf("unknown language: %v", err)
	}
	if _, err := Write(svc, "missing", "rust", t.TempDir()); !errors.Is(err, analysis.ErrNotFound) {
		t.Errorf("unknown analysis: %v", err)
	}
	st, _ := svc.Store.Load(id)
	st.Stage = analysis.StageParsed
	if err := svc.Store.Save(&st); err != nil {
		t.Fatal(err)
	}
	if _, err := Write(svc, id, "rust", t.TempDir()); !errors.Is(err, analysis.ErrNotDone) {
		t.Errorf("an unfinished analysis: %v", err)
	}
}

func TestWriteReportsADamagedAnalysis(t *testing.T) {
	svc, id := finished(t, "MZ\x90\x00 pe image")
	dir := svc.Store.Dir(id)
	for name, c := range map[string]struct{ file, content string }{
		"bad entry":    {"image.json", `{"kind":"executable","format":"pe","entry":"nope"}`},
		"bad base":     {"image.json", `{"kind":"executable","format":"pe","base":"nope"}`},
		"bad section":  {"image.json", `{"kind":"executable","format":"pe","sections":[{"name":".x","address":"?"}]}`},
		"bad function": {"engine.json", `{"functions":[{"name":"f","address":"?"}]}`},
		"bad engine":   {"engine.json", `{`},
	} {
		full(t, svc, id)
		testkit.WriteFile(t, filepath.Join(dir, c.file), c.content, 0o600)
		if _, err := Write(svc, id, "rust", t.TempDir()); err == nil {
			t.Errorf("%s must be refused", name)
		}
	}
	if err := os.Remove(filepath.Join(dir, "image.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := Write(svc, id, "rust", t.TempDir()); err == nil {
		t.Error("a missing structure must be refused")
	}
}

func TestLanguageNamesAndStrings(t *testing.T) {
	for in, want := range map[string]string{"Rust": "rust", " GOLANG ": "go", "py": "python", "ts": "typescript"} {
		if got, err := Normalize(in); err != nil || got != want {
			t.Errorf("%q -> %q, %v", in, got, err)
		}
	}
	if got := rustString("a\"b\\c\né"); got != `"a\"b\\c\u{a}\u{e9}"` {
		t.Errorf("rustString = %s", got)
	}
	if got := camel("raw_size"); got != "RawSize" {
		t.Errorf("camel = %s", got)
	}
	if a, err := parseAddr(""); err != nil || a != 0 {
		t.Errorf("an empty address is zero: %d, %v", a, err)
	}
	if _, err := parseAddr("zz"); err == nil {
		t.Error("a bad address must be refused")
	}
	_ = engine.Function{}
}
