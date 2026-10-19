package main

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func TestCortex_ServeUntilCancelled(t *testing.T) {
	useTempRegistry(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var buf bytes.Buffer
	if err := runCortexServe(ctx, freePort(t), &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "Dendrite Web UI listening") {
		t.Fatalf("unexpected output %q", buf.String())
	}
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	if err := runCortexServe(ctx, busy.Addr().(*net.TCPAddr).Port, &buf); err == nil {
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
	serveBoard(t)
	file := writeSvelte(t)
	if out, code := cliOutput(t, "cortex", "sync", "--file", file, "--dry-run", "--endpoint", "http://127.0.0.1:1"); code != ExitOK || !strings.Contains(out, "Successfully synchronized UI component") {
		t.Fatalf("sync: exit %d\n%s", code, out)
	}
	if out, code := cliOutput(t, "ui", "penpot", "-f", file, "--dry-run", "--json"); code != ExitOK || !strings.Contains(out, "{") {
		t.Fatalf("sync --json: exit %d\n%s", code, out)
	}
	if _, code := cliOutput(t, "cortex", "sync"); code != ExitUsage {
		t.Fatalf("sync without --file: exit %d", code)
	}
	if _, code := cliOutput(t, "cortex", "sync", "--file", filepath.Join(t.TempDir(), "missing.svelte")); code != ExitError {
		t.Fatalf("sync of a missing file: exit %d", code)
	}
}

func TestCortex_Canvas(t *testing.T) {
	serveBoard(t)
	state := t.TempDir()
	t.Setenv("DENDRITE_STATE_DIR", state)
	file := writeSvelte(t)
	steps := []struct {
		args []string
		code int
		want string
	}{
		{[]string{"cortex", "canvas", "actions"}, ExitOK, "Supported Canvas Creative Actions"},
		{[]string{"cortex", "canvas", "list-actions", "--json"}, ExitOK, "WRITE_PLAN"},
		{[]string{"cortex", "canvas", "dispatch", "--prompt", "Q4 roadmap", "--title", "Plan"}, ExitOK, "Action WRITE_PLAN [DRAFT] executed"},
		{[]string{"cortex", "canvas", "run", "--content", "draft text", "--mode", "CRITIQUE", "--json"}, ExitOK, "{"},
		{[]string{"cortex", "canvas", "dispatch"}, ExitUsage, ""},
		{[]string{"cortex", "canvas", "dispatch", "--action", "NO_SUCH_ACTION", "--prompt", "x"}, ExitError, ""},
		{[]string{"cortex", "canvas", "relate", "--ticket", "DEN-100", "--feature", "login", "--frame", "login-v2", "--component", file}, ExitOK, "Bound Plane ticket 'DEN-100' to feature 'login'"},
		{[]string{"cortex", "canvas", "relate", "--ticket", "DEN-100", "--feature", "login", "--json"}, ExitOK, `"login"`},
		{[]string{"cortex", "canvas", "relate", "--ticket", "DEN-100"}, ExitUsage, ""},
		{[]string{"cortex", "canvas", "list"}, ExitOK, "[DEN-100] login"},
		{[]string{"cortex", "canvas", "ls", "--json"}, ExitOK, `"DEN-100"`},
		{[]string{"cortex", "canvas", "sync", "--file", file, "--ticket", "DEN-100", "--dry-run"}, ExitOK, "Synchronized component"},
		{[]string{"cortex", "canvas", "sync", "--file", file, "--dry-run", "--json"}, ExitOK, "{"},
		{[]string{"cortex", "canvas", "sync"}, ExitUsage, ""},
		{[]string{"cortex", "canvas", "update-state", "--ticket", "DEN-100", "--state", "In Progress", "--no-plane"}, ExitOK, "Ticket ID:    DEN-100"},
		{[]string{"cortex", "canvas", "update-state", "--ticket", "DEN-100", "--state", "To Verify", "--no-plane", "--json"}, ExitOK, "{"},
		{[]string{"cortex", "canvas", "update-state", "--ticket", "DEN-100"}, ExitUsage, ""},
		{[]string{"canvas", "list"}, ExitUsage, ""},
	}
	for _, s := range steps {
		out, code := cliOutput(t, s.args...)
		if code != s.code || !strings.Contains(out, s.want) {
			t.Errorf("%v: exit %d (want %d)\n%s", s.args, code, s.code, out)
		}
	}
}
