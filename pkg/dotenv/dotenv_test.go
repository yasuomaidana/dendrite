package dotenv

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// setup makes a global .env and a git project; it returns the project's directory.
func setup(t *testing.T, global, project string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("DENDRITE_HOME", home)
	t.Setenv(EnvDisable, "")
	write(t, filepath.Join(home, FileName), global)
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if project != "" {
		write(t, filepath.Join(repo, FileName), project)
	}
	sub := filepath.Join(repo, "pkg", "x")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { loaded = map[string]bool{} })
	return sub
}

func TestParse(t *testing.T) {
	got := Parse("# c\n\nexport A=1\nB = \"two words\"\nC='x'\nD=\nbroken\nE=a=b\n")
	want := map[string]string{"A": "1", "B": "two words", "C": "x", "D": "", "E": "a=b"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("%s = %q, want %q", k, got[k], v)
		}
	}
}

func TestLoadGlobalAppliesWhenTheProjectHasNoEnv(t *testing.T) {
	dir := setup(t, "DOTENV_T_KEY=global\n", "")
	t.Setenv("DOTENV_T_KEY", "")
	found, err := Load(dir)
	if err != nil || len(found) != 1 {
		t.Fatalf("found %v, err %v", found, err)
	}
	if got := os.Getenv("DOTENV_T_KEY"); got != "global" {
		t.Fatalf("got %q", got)
	}
}

func TestLoadPrecedence(t *testing.T) {
	dir := setup(t, "DOTENV_T_A=global\nDOTENV_T_B=global\nDOTENV_T_C=global\n", "DOTENV_T_A=project\nDOTENV_T_B=\n")
	t.Setenv("DOTENV_T_A", "")
	t.Setenv("DOTENV_T_B", "")
	t.Setenv("DOTENV_T_C", "exported")
	if _, err := Load(dir); err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]string{"DOTENV_T_A": "project", "DOTENV_T_B": "global", "DOTENV_T_C": "exported"} {
		if got := os.Getenv(k); got != want {
			t.Fatalf("%s = %q, want %q", k, got, want)
		}
	}
}

func TestLookupResolvesForAnotherProject(t *testing.T) {
	cwd := setup(t, "DOTENV_T_KEY=global\n", "DOTENV_T_KEY=cwd-project\n")
	t.Setenv("DOTENV_T_KEY", "")
	if _, err := Load(cwd); err != nil {
		t.Fatal(err)
	}
	other := t.TempDir()
	if got := Lookup(other, "DOTENV_T_KEY"); got != "global" {
		t.Fatalf("a project without .env gets %q, want the global key", got)
	}
	write(t, filepath.Join(other, FileName), "DOTENV_T_KEY=other\n")
	if got := Lookup(other, "DOTENV_T_KEY"); got != "other" {
		t.Fatalf("got %q, want the project's own key", got)
	}
	t.Setenv("DOTENV_T_KEY", "exported")
	delete(loaded, "DOTENV_T_KEY")
	if got := Lookup(other, "DOTENV_T_KEY"); got != "exported" {
		t.Fatalf("got %q, an exported key must win", got)
	}
}

func TestDisabled(t *testing.T) {
	dir := setup(t, "DOTENV_T_KEY=global\n", "")
	t.Setenv("DOTENV_T_KEY", "")
	t.Setenv(EnvDisable, "off")
	if found, _ := Load(dir); found != nil || os.Getenv("DOTENV_T_KEY") != "" {
		t.Fatalf("loaded %v", found)
	}
}

func TestLocalFileOutranksTheCommittedProjectEnvAndGlobal(t *testing.T) {
	dir := setup(t, "DOTENV_T_A=global\nDOTENV_T_B=global\nDOTENV_T_C=global\n", "DOTENV_T_A=project\nDOTENV_T_B=project\n")
	root := ProjectRoot(dir)
	write(t, filepath.Join(root, LocalFileName), "DOTENV_T_A=local\n")
	for _, k := range []string{"DOTENV_T_A", "DOTENV_T_B", "DOTENV_T_C"} {
		t.Setenv(k, "")
	}
	if _, err := Load(dir); err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]string{"DOTENV_T_A": "local", "DOTENV_T_B": "project", "DOTENV_T_C": "global"} {
		if got := os.Getenv(k); got != want {
			t.Fatalf("%s = %q, want %q", k, got, want)
		}
	}
	if got := Lookup(dir, "DOTENV_T_A"); got != "local" {
		t.Fatalf("Lookup = %q, want the .env.local value", got)
	}
}
