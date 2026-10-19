package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReflex(t *testing.T) {
	for _, args := range [][]string{
		{"reflex", "auto"},
		{"reflex", "auto", "a.go", "--file", "b.go"},
		{"test", "ui", "extra"},
	} {
		if _, code := cliOutput(t, args...); code != ExitUsage {
			t.Errorf("%v: exit %d, want %d", args, code, ExitUsage)
		}
	}
	if _, code := cliOutput(t, "reflex", "auto", filepath.Join(t.TempDir(), "missing.go")); code != ExitError {
		t.Errorf("missing source file: exit %d", code)
	}
	if out, code := cliOutput(t, "reflex", "ui", "--base-url", "http://127.0.0.1:1"); code == ExitUsage || !strings.Contains(out, "Executing Web UI Test Harness") {
		t.Errorf("ui against a closed port: exit %d\n%s", code, out)
	}
}

func TestReflex_AutoSynthesizes(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "calc.go")
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/calc\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("package calc\n\nfunc Add(a, b int) int { return a + b }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, code := cliOutput(t, "reflex", "auto", "--file", src, "--json")
	if code == ExitUsage {
		t.Fatalf("auto: usage error\n%s", out)
	}
	if !strings.Contains(out, "Launching AST Test Synthesizer on: "+src) {
		t.Fatalf("auto output:\n%s", out)
	}
}
