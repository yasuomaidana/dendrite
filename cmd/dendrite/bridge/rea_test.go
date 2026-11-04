package bridge

import (
	"Dendrite/cmd/dendrite/internal/cli"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRetro(t *testing.T) {
	if out, code := cliOutput(t, "retro", "list"); code != cli.ExitOK || !strings.Contains(out, "Showcase Targets (5)") {
		t.Fatalf("list: exit %d\n%s", code, out)
	}
	if out, code := cliOutput(t, "rea", "targets", "--json"); code != cli.ExitOK || !strings.HasPrefix(strings.TrimSpace(out), "[") {
		t.Fatalf("list --json: exit %d\n%s", code, out)
	}
	if out, code := cliOutput(t, "retro", "showcase", "aegis"); code != cli.ExitOK || !strings.Contains(out, "SCORECARD: PASSED") {
		t.Fatalf("showcase text: exit %d\n%s", code, out)
	}
	file := filepath.Join(t.TempDir(), "card.md")
	if out, code := cliOutput(t, "retro", "showcase", "dxball", "--format", "md", "-o", file); code != cli.ExitOK || !strings.Contains(out, "Scorecard written to") {
		t.Fatalf("showcase markdown file: exit %d\n%s", code, out)
	}
	if data, err := os.ReadFile(file); err != nil || !strings.Contains(string(data), "# Open-Source REA Showcase") {
		t.Fatalf("markdown file: %v", err)
	}
	for _, args := range [][]string{
		{"retro", "showcase", "aegis", "--format", "json"},
		{"retro", "showcase", "a", "b"},
	} {
		if _, code := cliOutput(t, args...); code != cli.ExitUsage {
			t.Errorf("%v: exit %d, want %d", args, code, cli.ExitUsage)
		}
	}
	if _, code := cliOutput(t, "retro", "showcase", "no-such-target"); code != cli.ExitError {
		t.Errorf("unknown target: exit %d", code)
	}
	if _, code := cliOutput(t, "retro", "showcase", "ctf", "-o", filepath.Join(t.TempDir(), "missing", "x.md")); code != cli.ExitError {
		t.Errorf("unwritable output: exit %d", code)
	}
}
