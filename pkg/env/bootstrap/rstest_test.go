package bootstrap

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeCrate(t *testing.T, dir, manifest, source string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"Cargo.toml": manifest, "src/lib.rs": source} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

const withTests = "#[test]\nfn works() {}\n"

func TestRstestDeclaredReadsEveryShape(t *testing.T) {
	pkg := "[package]\nname = \"a\"\n[dev-dependencies]\n"
	for name, c := range map[string]struct {
		manifest     string
		ok, inherits bool
	}{
		"absent":      {pkg, false, false},
		"current":     {pkg + `rstest = "0.28.0"`, true, false},
		"caret old":   {pkg + `rstest = "^0.27"`, false, false},
		"newer":       {pkg + `rstest = ">=0.29, <1"`, true, false},
		"one":         {pkg + `rstest = "1"`, true, false},
		"table old":   {pkg + `rstest = { version = "0.18.2" }`, false, false},
		"path":        {pkg + `rstest = { path = "../rstest" }`, true, false},
		"workspace":   {pkg + `rstest = { workspace = true }`, true, true},
		"no digits":   {pkg + `rstest = "*"`, true, false},
		"wrong shape": {pkg + `rstest = 5`, false, false},
		"not toml":    {"[[[", false, false},
	} {
		if ok, inh := rstestDeclared([]byte(c.manifest)); ok != c.ok || inh != c.inherits {
			t.Errorf("%s: got (%v, %v), want (%v, %v)", name, ok, inh, c.ok, c.inherits)
		}
	}
}

func TestEnsureRstestAddsItWhereTestsNeedIt(t *testing.T) {
	root := t.TempDir()
	writeCrate(t, filepath.Join(root, "needs"), "[package]\nname = \"needs\"\n", withTests)
	writeCrate(t, filepath.Join(root, "has"), "[package]\nname = \"has\"\n[dev-dependencies]\nrstest = \"0.28.0\"\n", withTests)
	writeCrate(t, filepath.Join(root, "inherits"), "[package]\nname = \"i\"\n[dev-dependencies]\nrstest = { workspace = true }\n", withTests)
	writeCrate(t, filepath.Join(root, "untested"), "[package]\nname = \"u\"\n", "pub fn f() {}\n")
	writeCrate(t, filepath.Join(root, "target", "skipped"), "[package]\nname = \"s\"\n", withTests)
	if err := os.WriteFile(filepath.Join(root, "Cargo.toml"), []byte("[workspace]\nmembers = []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var asked []string
	prev := cargoAddRstest
	t.Cleanup(func() { cargoAddRstest = prev })
	cargoAddRstest = func(dir string) error { asked = append(asked, filepath.Base(dir)); return nil }

	dry, _ := ensureRstest(root, true)
	if len(dry) != 1 || dry[0] != filepath.Join("needs", "Cargo.toml") || len(asked) != 0 {
		t.Errorf("dry run: changed %v, asked %v", dry, asked)
	}
	changed, notes := ensureRstest(root, false)
	if len(changed) != 1 || len(asked) != 1 || asked[0] != "needs" {
		t.Errorf("changed %v, asked %v", changed, asked)
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "comes from the workspace") {
		t.Errorf("notes = %v", notes)
	}

	cargoAddRstest = func(string) error { return errors.New("no cargo") }
	changed, notes = ensureRstest(root, false)
	if len(changed) != 0 || len(notes) != 2 || !strings.Contains(notes[0]+notes[1], `rstest = "0.28.0"`) {
		t.Errorf("a failing cargo: changed %v, notes %v", changed, notes)
	}
}

func TestCargoAddRstestReportsAFailure(t *testing.T) {
	if err := cargoAddRstest(t.TempDir()); err == nil || !strings.Contains(err.Error(), "cargo add rstest") {
		t.Errorf("a directory with no manifest: %v", err)
	}
}

func TestRstestRecentComparesMajorAndMinor(t *testing.T) {
	for req, want := range map[string]bool{"0.28.0": true, "0.27.9": false, "^0.28": true, "1.0": true, "0": false, "=0.9.1": false} {
		if got := rstestRecent(req); got != want {
			t.Errorf("rstestRecent(%q) = %v", req, got)
		}
	}
}
