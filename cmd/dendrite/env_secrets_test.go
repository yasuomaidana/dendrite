package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func secretsProject(t *testing.T) string {
	t.Helper()
	t.Setenv("DENDRITE_HOME", t.TempDir())
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "stacks"), 0o755); err != nil {
		t.Fatal(err)
	}
	tmpl := "POSTGRES_PASSWORD=pg_default\nLITELLM_MASTER_KEY=sk-default\nLITELLM_SALT_KEY=sk-salt\n"
	for _, f := range []string{filepath.Join("stacks", ".env.example"), ".env"} { // template and committed defaults
		if err := os.WriteFile(filepath.Join(dir, f), []byte(tmpl), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(dir)
	return dir
}

func TestSomaSecrets_EnsureStatusAndSet(t *testing.T) {
	dir := secretsProject(t)
	out, code := cliOutput(t, "soma", "secrets", "ensure")
	if code != ExitOK || !strings.Contains(out, "Nothing to change") || !strings.Contains(out, "published defaults") {
		t.Fatalf("ensure: exit %d\n%s", code, out)
	}
	if out, code = cliOutput(t, "soma", "secrets", "ensure", "--set", "POSTGRES_PASSWORD=mine"); code != ExitOK || !strings.Contains(out, "changed: POSTGRES_PASSWORD") {
		t.Fatalf("--set: exit %d\n%s", code, out)
	}
	home := os.Getenv("DENDRITE_HOME")
	raw, _ := os.ReadFile(filepath.Join(home, ".env"))
	if !strings.Contains(string(raw), "POSTGRES_PASSWORD=mine") {
		t.Errorf("global .env = %q, want the value configured once in DENDRITE_HOME", raw)
	}
	if _, err := os.Stat(filepath.Join(dir, ".env.local")); err == nil {
		t.Error("a checkout-local file was written without --local")
	}
	if committed, _ := os.ReadFile(filepath.Join(dir, ".env")); strings.Contains(string(committed), "mine") {
		t.Error("the committed .env was edited")
	}
	if out, code = cliOutput(t, "soma", "secrets", "ensure", "--local", "--set", "LITELLM_MASTER_KEY=sk-here-only"); code != ExitOK {
		t.Fatalf("--local: exit %d\n%s", code, out)
	}
	if local, _ := os.ReadFile(filepath.Join(dir, ".env.local")); !strings.Contains(string(local), "LITELLM_MASTER_KEY=sk-here-only") {
		t.Errorf(".env.local = %q", local)
	}

	out, code = cliOutput(t, "soma", "secrets", "status")
	if code != ExitOK || !strings.Contains(out, "POSTGRES_PASSWORD") || strings.Contains(out, "mine") || strings.Contains(out, "sk-default") {
		t.Fatalf("status leaked a value or failed: exit %d\n%s", code, out)
	}
	if _, code = cliOutput(t, "soma", "secrets", "ensure", "--set", "NOPE"); code != ExitUsage {
		t.Errorf("malformed --set: exit %d, want %d", code, ExitUsage)
	}
}
