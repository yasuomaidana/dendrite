package statedir

import (
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestResolve_AbsolutePath(t *testing.T) {
	abs, err := filepath.Abs("some/file.txt")
	if err != nil {
		t.Fatalf("filepath.Abs failed: %v", err)
	}

	if got := Resolve(abs); got != abs {
		t.Errorf("Resolve(%q) = %q; want %q", abs, got, abs)
	}

	absDendrite := filepath.Join(abs, ".dendrite", "state.json")
	if got := Resolve(absDendrite); got != absDendrite {
		t.Errorf("Resolve(%q) = %q; want %q", absDendrite, got, absDendrite)
	}
}

func TestResolve_NonDendriteRelativePath(t *testing.T) {
	paths := []string{
		"config.json",
		filepath.Join("pkg", "statedir", "foo.go"),
		"nested/path/.dendrite/bar",
		".dendrite_custom/file.txt",
		"",
	}

	for _, p := range paths {
		if got := Resolve(p); got != p {
			t.Errorf("Resolve(%q) = %q; want %q", p, got, p)
		}
	}
}

func TestResolve_EnvVarSet(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DENDRITE_STATE_DIR", dir)

	input := ".dendrite/a/b.json"
	expected := filepath.Join(dir, "a", "b.json")
	got := Resolve(input)
	if got != expected {
		t.Errorf("Resolve(%q) = %q; want %q", input, got, expected)
	}
}

func TestResolve_EnvVarUnsetUsesTestIsolation(t *testing.T) {
	t.Setenv("DENDRITE_STATE_DIR", "")

	got := Resolve(".dendrite/x.json")
	want := filepath.Join(testStateDir(), "x.json")
	if got != want {
		t.Errorf("Resolve(\".dendrite/x.json\") = %q; want %q", got, want)
	}
	if !strings.Contains(got, "dendrite-test-state-") {
		t.Errorf("test binaries must not resolve into the repository state dir, got %q", got)
	}
	if Resolve(".dendrite") != testStateDir() {
		t.Errorf("Resolve(\".dendrite\") = %q; want %q", Resolve(".dendrite"), testStateDir())
	}
}

func TestGitStateDir(t *testing.T) {
	got := gitStateDir()
	if !filepath.IsAbs(got) || filepath.Base(got) != ".dendrite" {
		t.Errorf("gitStateDir() = %q; want an absolute <repo>/.dendrite path", got)
	}
}

func TestResolve_RepositoryAndCWDFallbacks(t *testing.T) {
	t.Setenv("DENDRITE_STATE_DIR", "")
	origTest, origGit := isTestBinary, gitTopLevel
	t.Cleanup(func() {
		isTestBinary, gitTopLevel = origTest, origGit
		gitOnce, gitRoot = sync.Once{}, ""
	})
	isTestBinary = func() bool { return false }

	repo := t.TempDir()
	gitOnce, gitRoot = sync.Once{}, ""
	gitTopLevel = func() ([]byte, error) { return []byte(repo + "\n"), nil }
	if got, want := Resolve(".dendrite/a.json"), filepath.Join(repo, ".dendrite", "a.json"); got != want {
		t.Errorf("inside a repository: Resolve = %q; want %q", got, want)
	}

	gitOnce, gitRoot = sync.Once{}, ""
	gitTopLevel = func() ([]byte, error) { return nil, errors.New("not a git repository") }
	if got := Resolve(".dendrite/a.json"); got != ".dendrite/a.json" {
		t.Errorf("outside a repository: Resolve = %q; want the CWD-relative path unchanged", got)
	}
}
