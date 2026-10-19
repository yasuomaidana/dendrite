package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTriage(t *testing.T) {
	t.Setenv("OLLAMA_HOST", "http://127.0.0.1:1")
	file := filepath.Join(t.TempDir(), "main.go")
	if err := os.WriteFile(file, []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, code := cliOutput(t, "triage", "rename", "the", "main", "function", "--file", file, "--tier", "cloud"); code != ExitOK || !strings.Contains(out, "Assigned Tier:      CLOUD") {
		t.Fatalf("triage --tier cloud: exit %d\n%s", code, out)
	}
	if out, code := cliOutput(t, "inhibitory-triage", "summarize", "--engine", "hf", "--json"); code != ExitOK || !strings.Contains(out, "{") {
		t.Fatalf("triage --json: exit %d\n%s", code, out)
	}
	for _, args := range [][]string{
		{"triage"},
		{"triage", "x", "--tier", "orbital"},
		{"triage", "x", "--engine", "abacus"},
	} {
		if _, code := cliOutput(t, args...); code != ExitUsage {
			t.Errorf("%v: exit %d, want %d", args, code, ExitUsage)
		}
	}
	if _, code := cliOutput(t, "triage", "x", "--file", filepath.Join(t.TempDir(), "missing.go")); code != ExitError {
		t.Errorf("missing --file: exit %d", code)
	}
}
