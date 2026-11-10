package switcher

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// manifests writes one manifest of each kind naming dep the published way, plus a go.mod replace.
func manifests(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range map[string]string{
		"go.mod":         "module example.com/app\n\ngo 1.24\n\nrequire (\n\texample.com/dep v1.2.3\n)\nreplace example.com/dep => ../dep\n",
		"Cargo.toml":     "[package]\nname = \"app\"\n\n[dependencies]\ndep = \"1.2\"\nother = { path = \"../o\" }\n",
		"package.json":   "{\n  \"dependencies\": {\n    \"dep\": \"^1.2.0\",\n    \"linked\": \"link:../l\",\n    \"forked\": \"git+https://x/y.git#b\"\n  }\n}",
		"pyproject.toml": "[tool.poetry.dependencies]\ndep = \"==1.2\"\ngitdep = { git = \"https://x/y.git\" }\nbranched = { git = \"https://x/y.git\", branch = \"b\" }\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func modeOf(t *testing.T, dir, dep, manifest string) DependencyMode {
	t.Helper()
	entries, err := Current(dir, dep)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Manifest == manifest {
			return e.Mode
		}
	}
	t.Fatalf("%s is not named in %s", dep, manifest)
	return ""
}

func TestCurrentReadsEveryManifestThatNamesTheDependency(t *testing.T) {
	dir := manifests(t)
	for _, c := range []struct {
		dep, manifest string
		want          DependencyMode
	}{
		{"dep", "Cargo.toml", ModePublished}, {"other", "Cargo.toml", ModeLocal},
		{"dep", "package.json", ModePublished}, {"linked", "package.json", ModeLocal}, {"forked", "package.json", ModeBranch},
		{"dep", "pyproject.toml", ModePublished}, {"gitdep", "pyproject.toml", ModeFork}, {"branched", "pyproject.toml", ModeBranch},
		{"example.com/dep", "go.mod", ModeLocal},
	} {
		if got := modeOf(t, dir, c.dep, c.manifest); got != c.want {
			t.Errorf("%s in %s = %s, want %s", c.dep, c.manifest, got, c.want)
		}
	}
	entries, _ := Current(dir, "example.com/dep")
	if e := entries[0]; e.Raw != "example.com/dep v1.2.3" || e.Replace != "replace example.com/dep => ../dep" {
		t.Errorf("go.mod entry = %+v", e)
	}
	if _, err := Current(dir, "absent"); !errors.Is(err, ErrDependencyNotFound) {
		t.Errorf("a dependency no manifest names: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module m\nrequire example.com/dep v1.0.0\nreplace example.com/dep => example.com/fork v1.0.1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := modeOf(t, dir, "example.com/dep", "go.mod"); got != ModeFork {
		t.Errorf("a replacement module is a fork, got %s", got)
	}
}

// content reads the manifests of dir by name.
func content(t *testing.T, dir string, names ...string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		out[name] = string(data)
	}
	return out
}

func TestRestorePutsEveryManifestBackAfterASwitch(t *testing.T) {
	dir := manifests(t)
	// go.mod names another module path, so the switch of "dep" touches the three other manifests only.
	if err := os.Remove(filepath.Join(dir, "go.mod")); err != nil {
		t.Fatal(err)
	}
	before := content(t, dir, "Cargo.toml", "pyproject.toml")
	original, err := Current(dir, "dep")
	if err != nil || len(original) != 3 {
		t.Fatalf("entries = %+v, %v", original, err)
	}
	req := SwitchRequest{TargetProjectDir: dir, DependencyName: "dep", Mode: ModeFork, ForkURL: "https://x/fork.git", ForkBranch: "DEN-1-fix/main"}
	if _, err := NewDependencySwitcher().Switch(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if got := modeOf(t, dir, "dep", "Cargo.toml"); got != ModeBranch {
		t.Fatalf("after the switch Cargo.toml is %s", got)
	}
	for _, e := range original {
		if err := Restore(dir, "dep", e); err != nil {
			t.Fatal(err)
		}
	}
	if after := content(t, dir, "Cargo.toml", "pyproject.toml"); !reflect.DeepEqual(after, before) {
		t.Errorf("not restored: %v", after)
	}
	if got := modeOf(t, dir, "dep", "package.json"); got != ModePublished {
		t.Errorf("package.json = %s", got)
	}
}

func TestRestoreGoModPutsBackTheRequireAndTheReplace(t *testing.T) {
	dir := manifests(t)
	entries, _ := Current(dir, "example.com/dep")
	changed := "module example.com/app\n\ngo 1.24\n\nrequire (\n\texample.com/dep v1.9.9-0.2026\n)\nrequire example.com/dep v0.0.1\nreplace example.com/dep => /tmp/copy\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(changed), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Restore(dir, "example.com/dep", entries[0]); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "go.mod"))
	got := string(data)
	if !strings.Contains(got, "\texample.com/dep v1.2.3") || !strings.Contains(got, "require example.com/dep v1.2.3") || !strings.HasSuffix(got, "replace example.com/dep => ../dep") || strings.Contains(got, "/tmp/copy") {
		t.Errorf("go.mod = %q", got)
	}
}

func TestRestoreReportsWhatItCannotPutBack(t *testing.T) {
	dir := manifests(t)
	if err := Restore(dir, "dep", Entry{Manifest: "missing.toml"}); err == nil {
		t.Error("a manifest that is gone")
	}
	if err := Restore(dir, "absent", Entry{Manifest: "Cargo.toml", Raw: "absent = \"1\""}); !errors.Is(err, ErrDependencyNotFound) {
		t.Errorf("a line that is gone: %v", err)
	}
	for _, content := range []string{"not json", "{}"} {
		if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := Restore(dir, "dep", Entry{Manifest: "package.json", Raw: "1"}); err == nil {
			t.Errorf("package.json %q", content)
		}
		if _, ok := currentPackageJSON(content, "dep"); ok {
			t.Errorf("package.json %q names nothing", content)
		}
	}
}

func TestABranchSwitchWithACommitPinsTheGitDependencyToIt(t *testing.T) {
	dir := manifests(t)
	if err := os.Remove(filepath.Join(dir, "go.mod")); err != nil {
		t.Fatal(err)
	}
	req := SwitchRequest{TargetProjectDir: dir, DependencyName: "dep", Mode: ModeBranch, ForkURL: "https://x/up.git", BranchName: "main", CommitSHA: "abc123"}
	if _, err := NewDependencySwitcher().Switch(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	for name, text := range content(t, dir, "Cargo.toml", "pyproject.toml") {
		if !strings.Contains(text, `dep = { git = "https://x/up.git", rev = "abc123" }`) {
			t.Errorf("%s is not pinned:\n%s", name, text)
		}
	}
	if got := modeOf(t, dir, "dep", "Cargo.toml"); got != ModeBranch {
		t.Errorf("a pinned dependency reads as %s", got)
	}
}
