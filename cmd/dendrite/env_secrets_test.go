package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func secretsProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "stacks"), 0o755); err != nil {
		t.Fatal(err)
	}
	tmpl := "POSTGRES_PASSWORD=pg_default\nLITELLM_MASTER_KEY=sk-default\nLITELLM_SALT_KEY=sk-salt\n"
	if err := os.WriteFile(filepath.Join(dir, "stacks", ".env.example"), []byte(tmpl), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	return dir
}

func TestSomaSecrets_EnsureStatusAndSet(t *testing.T) {
	dir := secretsProject(t)
	out, code := cliOutput(t, "soma", "secrets", "ensure")
	if code != ExitOK || !strings.Contains(out, "Created") || !strings.Contains(out, "published defaults") {
		t.Fatalf("ensure: exit %d\n%s", code, out)
	}
	if out, code = cliOutput(t, "soma", "secrets", "ensure", "--set", "POSTGRES_PASSWORD=mine"); code != ExitOK || !strings.Contains(out, "changed: POSTGRES_PASSWORD") {
		t.Fatalf("--set: exit %d\n%s", code, out)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, ".env"))
	if !strings.Contains(string(raw), "POSTGRES_PASSWORD=mine") {
		t.Errorf(".env = %q", raw)
	}

	out, code = cliOutput(t, "soma", "secrets", "status")
	if code != ExitOK || !strings.Contains(out, "POSTGRES_PASSWORD") || strings.Contains(out, "mine") || strings.Contains(out, "sk-default") {
		t.Fatalf("status leaked a value or failed: exit %d\n%s", code, out)
	}
	if _, code = cliOutput(t, "soma", "secrets", "ensure", "--set", "NOPE"); code != ExitUsage {
		t.Errorf("malformed --set: exit %d, want %d", code, ExitUsage)
	}
}
