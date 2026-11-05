package cortex

import (
	"Dendrite/cmd/dendrite/internal/cli"
	"Dendrite/cmd/dendrite/internal/clitest"
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCortex_ServeUntilCancelled(t *testing.T) {
	clitest.UseTempRegistry(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var buf bytes.Buffer
	if err := runCortexServe(ctx, clitest.FreePort(t), false, &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "Dendrite Web UI listening") || !strings.Contains(buf.String(), "?token=") {
		t.Fatalf("serve must print a tokenised URL, got %q", buf.String())
	}
	var open bytes.Buffer
	if err := runCortexServe(ctx, clitest.FreePort(t), true, &open); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(open.String(), "token=") || !strings.Contains(open.String(), "--no-auth") {
		t.Fatalf("--no-auth must say so and print no token, got %q", open.String())
	}
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	if err := runCortexServe(ctx, busy.Addr().(*net.TCPAddr).Port, false, &buf); err == nil {
		t.Fatal("a busy port should fail")
	}
}

func writeSvelte(t *testing.T) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "Button.svelte")
	src := "<script>\n  export let label = 'Go';\n</script>\n<button style=\"color: #ff0000\">{label}</button>\n"
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return file
}

func TestCortex_Sync(t *testing.T) {
	clitest.ServeBoard(t)
	file := writeSvelte(t)
	if out, code := cliOutput(t, "cortex", "sync", "--file", file, "--dry-run", "--endpoint", "http://127.0.0.1:1"); code != cli.ExitOK || !strings.Contains(out, "Successfully synchronized UI component") {
		t.Fatalf("sync: exit %d\n%s", code, out)
	}
	if out, code := cliOutput(t, "ui", "penpot", "-f", file, "--dry-run", "--json"); code != cli.ExitOK || !strings.Contains(out, "{") {
		t.Fatalf("sync --json: exit %d\n%s", code, out)
	}
	if _, code := cliOutput(t, "cortex", "sync"); code != cli.ExitUsage {
		t.Fatalf("sync without --file: exit %d", code)
	}
	if _, code := cliOutput(t, "cortex", "sync", "--file", filepath.Join(t.TempDir(), "missing.svelte")); code != cli.ExitError {
		t.Fatalf("sync of a missing file: exit %d", code)
	}
}

func TestCortex_Canvas(t *testing.T) {
	clitest.ServeBoard(t)
	state := t.TempDir()
	t.Setenv("DENDRITE_STATE_DIR", state)
	file := writeSvelte(t)
	steps := []struct {
		args []string
		code int
		want string
	}{
		{[]string{"cortex", "canvas", "actions"}, cli.ExitOK, "Supported Canvas Creative Actions"},
		{[]string{"cortex", "canvas", "list-actions", "--json"}, cli.ExitOK, "WRITE_PLAN"},
		{[]string{"cortex", "canvas", "dispatch", "--prompt", "Q4 roadmap", "--title", "Plan"}, cli.ExitOK, "Action WRITE_PLAN [DRAFT] executed"},
		{[]string{"cortex", "canvas", "run", "--content", "draft text", "--mode", "CRITIQUE", "--json"}, cli.ExitOK, "{"},
		{[]string{"cortex", "canvas", "dispatch"}, cli.ExitUsage, ""},
		{[]string{"cortex", "canvas", "dispatch", "--action", "NO_SUCH_ACTION", "--prompt", "x"}, cli.ExitError, ""},
		{[]string{"cortex", "canvas", "relate", "--ticket", "DEN-100", "--feature", "login", "--frame", "login-v2", "--component", file}, cli.ExitOK, "Bound Plane ticket 'DEN-100' to feature 'login'"},
		{[]string{"cortex", "canvas", "relate", "--ticket", "DEN-100", "--feature", "login", "--json"}, cli.ExitOK, `"login"`},
		{[]string{"cortex", "canvas", "relate", "--ticket", "DEN-100"}, cli.ExitUsage, ""},
		{[]string{"cortex", "canvas", "list"}, cli.ExitOK, "[DEN-100] login"},
		{[]string{"cortex", "canvas", "ls", "--json"}, cli.ExitOK, `"DEN-100"`},
		{[]string{"cortex", "canvas", "sync", "--file", file, "--ticket", "DEN-100", "--dry-run"}, cli.ExitOK, "Synchronized component"},
		{[]string{"cortex", "canvas", "sync", "--file", file, "--dry-run", "--json"}, cli.ExitOK, "{"},
		{[]string{"cortex", "canvas", "sync"}, cli.ExitUsage, ""},
		{[]string{"cortex", "canvas", "update-state", "--ticket", "DEN-100", "--state", "In Progress", "--no-plane"}, cli.ExitOK, "Ticket ID:    DEN-100"},
		{[]string{"cortex", "canvas", "update-state", "--ticket", "DEN-100", "--state", "To Verify", "--no-plane", "--json"}, cli.ExitOK, "{"},
		{[]string{"cortex", "canvas", "update-state", "--ticket", "DEN-100"}, cli.ExitUsage, ""},
		{[]string{"canvas", "list"}, cli.ExitUsage, ""},
	}
	for _, s := range steps {
		out, code := cliOutput(t, s.args...)
		if code != s.code || !strings.Contains(out, s.want) {
			t.Errorf("%v: exit %d (want %d)\n%s", s.args, code, s.code, out)
		}
	}
}
