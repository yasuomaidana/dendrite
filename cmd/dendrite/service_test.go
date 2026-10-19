package main

import (
	"Dendrite/pkg/node/migrate"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func useCompose(t *testing.T, err error) *[]string {
	t.Helper()
	var calls []string
	prev := runCompose
	runCompose = func(_ context.Context, _ io.Writer, file string, args ...string) error {
		calls = append(calls, file+" "+strings.Join(args, " "))
		return err
	}
	t.Cleanup(func() { runCompose = prev })
	return &calls
}

func TestMotor(t *testing.T) {
	calls := useCompose(t, nil)
	if out, code := cliOutput(t, "motor", "list"); code != ExitOK || !strings.Contains(out, "Registered Service Catalog") {
		t.Fatalf("list: exit %d\n%s", code, out)
	}
	if out, code := cliOutput(t, "service", "ls", "--json"); code != ExitOK || !strings.Contains(out, "{") {
		t.Fatalf("list --json: exit %d\n%s", code, out)
	}
	if out, code := cliOutput(t, "motor", "health", "litellm", "--deep"); code != ExitOK || !strings.Contains(out, "Inspecting Health for Service 'litellm'") {
		t.Fatalf("health: exit %d\n%s", code, out)
	}
	if out, code := cliOutput(t, "motor", "health", "plane", "--json"); code != ExitOK || !strings.Contains(out, `"healthy"`) {
		t.Fatalf("health --json: exit %d\n%s", code, out)
	}
	if _, code := cliOutput(t, "motor", "stop", "plane"); code != ExitOK || len(*calls) != 1 || !strings.HasSuffix((*calls)[0], " down") {
		t.Fatalf("stop: exit %d, calls %v", code, *calls)
	}
	if _, code := cliOutput(t, "motor", "restart", "plane"); code != ExitOK {
		t.Fatalf("restart: exit %d", code)
	}
	for _, args := range [][]string{{"motor", "start", "nope"}, {"motor", "health", "a", "b"}, {"motor", "ui-sync"}} {
		if _, code := cliOutput(t, args...); code != ExitUsage {
			t.Errorf("%v: exit %d, want %d", args, code, ExitUsage)
		}
	}
}

func TestMotor_StartAndComposeFailures(t *testing.T) {
	calls := useCompose(t, nil)
	t.Chdir(t.TempDir())
	if out, code := cliOutput(t, "motor", "start", "plane", "--target", "local"); code != ExitOK || !strings.Contains(out, "not found; skipping container spinup") {
		t.Fatalf("start without a compose file: exit %d\n%s", code, out)
	}

	file := migrate.DefaultServiceCatalog()["plane"].ComposeFile
	if file == "" {
		t.Fatal("the plane profile must name a compose file")
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := cliOutput(t, "motor", "start", "plane", "--verify-mcp"); code != ExitOK || len(*calls) == 0 {
		t.Fatalf("start with a compose file: exit %d, calls %v", code, *calls)
	}
	useCompose(t, errors.New("docker not running"))
	if out, code := cliOutput(t, "motor", "start", "plane"); code != ExitError || !strings.Contains(out, "docker not running") {
		t.Fatalf("compose failure: exit %d\n%s", code, out)
	}
	if _, code := cliOutput(t, "motor", "stop", "plane"); code != ExitError {
		t.Fatalf("stop failure: exit %d", code)
	}
}
