package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRetro(t *testing.T) {
	if out, code := cliOutput(t, "retro", "list"); code != ExitOK || !strings.Contains(out, "Showcase Targets (5)") {
		t.Fatalf("list: exit %d\n%s", code, out)
	}
	if out, code := cliOutput(t, "rea", "targets", "--json"); code != ExitOK || !strings.HasPrefix(strings.TrimSpace(out), "[") {
		t.Fatalf("list --json: exit %d\n%s", code, out)
	}
	if out, code := cliOutput(t, "retro", "showcase", "aegis"); code != ExitOK || !strings.Contains(out, "SCORECARD: PASSED") {
		t.Fatalf("showcase text: exit %d\n%s", code, out)
	}
	file := filepath.Join(t.TempDir(), "card.md")
	if out, code := cliOutput(t, "retro", "showcase", "dxball", "--format", "md", "-o", file); code != ExitOK || !strings.Contains(out, "Scorecard written to") {
		t.Fatalf("showcase markdown file: exit %d\n%s", code, out)
	}
	if data, err := os.ReadFile(file); err != nil || !strings.Contains(string(data), "# Open-Source REA Showcase") {
		t.Fatalf("markdown file: %v", err)
	}
	for _, args := range [][]string{
		{"retro", "showcase", "aegis", "--format", "json"},
		{"retro", "showcase", "a", "b"},
	} {
		if _, code := cliOutput(t, args...); code != ExitUsage {
			t.Errorf("%v: exit %d, want %d", args, code, ExitUsage)
		}
	}
	if _, code := cliOutput(t, "retro", "showcase", "no-such-target"); code != ExitError {
		t.Errorf("unknown target: exit %d", code)
	}
	if _, code := cliOutput(t, "retro", "showcase", "ctf", "-o", filepath.Join(t.TempDir(), "missing", "x.md")); code != ExitError {
		t.Errorf("unwritable output: exit %d", code)
	}
}
